import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api } from "@/lib/api";
import { registerStepUpHandler } from "@/lib/step-up";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "./ui/dialog";

type Pending = {
  action: string;
  target: string;
  resolve: (token: string) => void;
  reject: (err: Error) => void;
  settled: boolean;
};

export function StepUpDialog() {
  const { t } = useTranslation("layout");
  const pendingRef = useRef<Pending | null>(null);
  const [view, setView] = useState<{ action: string; target: string } | null>(null);
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const me = useQuery({
    queryKey: ["me"],
    queryFn: () => api.getMe(),
    staleTime: 30_000,
  });

  useEffect(() => {
    registerStepUpHandler(
      (req) =>
        new Promise<string>((resolve, reject) => {
          pendingRef.current = { ...req, resolve, reject, settled: false };
          setCode("");
          setError("");
          setSubmitting(false);
          setView({ action: req.action, target: req.target });
        }),
    );
    return () => {
      const pending = pendingRef.current;
      pendingRef.current = null;
      if (pending && !pending.settled) {
        pending.settled = true;
        pending.reject(new Error("cancelled"));
      }
      registerStepUpHandler(null);
    };
  }, []);

  function actionLabel(action: string): string {
    return t(`mfaStepUp.actions.${action}`, { defaultValue: action });
  }

  function cancel() {
    const pending = pendingRef.current;
    pendingRef.current = null;
    setView(null);
    setSubmitting(false);
    if (pending && !pending.settled) {
      pending.settled = true;
      pending.reject(new Error(t("mfaStepUp.cancelled")));
    }
  }

  function succeed(token: string) {
    const pending = pendingRef.current;
    pendingRef.current = null;
    if (pending && !pending.settled) {
      pending.settled = true;
      pending.resolve(token);
    }
    setView(null);
    setSubmitting(false);
  }

  async function confirm() {
    const pending = pendingRef.current;
    if (!pending || submitting) return;
    setSubmitting(true);
    setError("");
    try {
      const res = await api.stepUp(pending.action, pending.target, code.trim());
      succeed(res.token);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("mfaStepUp.invalid"));
      setSubmitting(false);
    }
  }

  async function confirmWebAuthn() {
    const pending = pendingRef.current;
    if (!pending || submitting || !window.PublicKeyCredential) return;
    setSubmitting(true);
    setError("");
    try {
      const options = await api.stepUpWebAuthnBegin(pending.action, pending.target);
      const publicKey = PublicKeyCredential.parseRequestOptionsFromJSON(
        options as unknown as PublicKeyCredentialRequestOptionsJSON,
      );
      const cred = (await navigator.credentials.get({ publicKey })) as PublicKeyCredential | null;
      if (!cred) {
        throw new Error(t("mfaStepUp.cancelled"));
      }
      const body = typeof cred.toJSON === "function" ? cred.toJSON() : { id: cred.id, type: cred.type };
      const res = await api.stepUpWebAuthnFinish(body);
      succeed(res.token);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("mfaStepUp.invalid"));
      setSubmitting(false);
    }
  }

  const target = view?.target?.trim() ? view.target : t("mfaStepUp.noTarget");

  return (
    <Dialog open={!!view} onOpenChange={(open) => !open && cancel()}>
      <DialogContent data-testid="mfa-step-up-dialog">
        <DialogHeader>
          <DialogTitle>{t("mfaStepUp.title")}</DialogTitle>
          <DialogDescription>
            {t("mfaStepUp.description", {
              action: view ? actionLabel(view.action) : "",
              target,
            })}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <Label htmlFor="mfa-step-up-code">{t("mfaStepUp.code")}</Label>
          <Input
            id="mfa-step-up-code"
            data-testid="mfa-step-up-code"
            value={code}
            autoComplete="one-time-code"
            autoFocus
            onChange={(e) => setCode(e.target.value.trim())}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                void confirm();
              }
            }}
          />
          {error && <p className="text-sm text-destructive">{error}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={cancel} disabled={submitting}>
            {t("mfaStepUp.cancel")}
          </Button>
          {me.data?.webauthn_enabled && (
            <Button variant="outline" onClick={() => void confirmWebAuthn()} disabled={submitting}>
              {t("mfaStepUp.useKey")}
            </Button>
          )}
          <Button data-testid="mfa-step-up-confirm" onClick={() => void confirm()} disabled={code.length < 6 || submitting}>
            {t("mfaStepUp.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
