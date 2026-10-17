import { useCallback, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@datasafe/ui/ui/button";
import { Input } from "@datasafe/ui/ui/input";
import { Label } from "@datasafe/ui/ui/label";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@datasafe/ui/ui/dialog";
import { browse, type BrowseResult } from "../desktop";

type Job = {
  method: string;
  params: Record<string, unknown>;
  resolve: (result: BrowseResult) => void;
};

export function useStepUp(profileId: string, onSetup: () => void) {
  const [open, setOpen] = useState(false);
  const [error, setError] = useState("");
  const pending = useRef<Job | null>(null);
  const busy = useRef(false);

  const call = useCallback(
    (method: string, params: Record<string, unknown>) => {
      return new Promise<BrowseResult>((resolve) => {
        if (busy.current) {
          resolve({ status: 0, code: "cancelled" });
          return;
        }
        busy.current = true;
        void browse(profileId, method, params)
          .then((result) => {
            if (result.mfa_setup) {
              onSetup();
              busy.current = false;
              resolve(result);
              return;
            }
            if (!result.step_up) {
              busy.current = false;
              resolve(result);
              return;
            }
            pending.current = { method, params, resolve };
            setError("");
            setOpen(true);
          })
          .catch((err: unknown) => {
            busy.current = false;
            resolve({ status: 0, message: err instanceof Error ? err.message : String(err) });
          });
      });
    },
    [onSetup, profileId],
  );

  const finish = useCallback((result: BrowseResult) => {
    const job = pending.current;
    pending.current = null;
    busy.current = false;
    setOpen(false);
    setError("");
    job?.resolve(result);
  }, []);

  async function submit(code: string) {
    const job = pending.current;
    const trimmed = code.trim();
    if (!job || trimmed === "") {
      return;
    }
    let result: BrowseResult;
    try {
      result = await browse(profileId, job.method, { ...job.params, step_up_code: trimmed });
    } catch (err) {
      finish({ status: 0, message: err instanceof Error ? err.message : String(err) });
      return;
    }
    if (result.mfa_setup) {
      onSetup();
      finish(result);
      return;
    }
    if (result.step_up) {
      setError(result.code === "auth.mfa_token_invalid" ? "again" : "invalid");
      return;
    }
    finish(result);
  }

  function cancel() {
    finish({ status: 0, code: "cancelled" });
  }

  return { call, open, error, submit, cancel };
}

export function StepUpDialog(props: {
  open: boolean;
  error: string;
  onCancel: () => void;
  onSubmit: (code: string) => void;
}) {
  const { t } = useTranslation();
  const [code, setCode] = useState("");
  return (
    <Dialog open={props.open} onOpenChange={(open) => { if (!open) props.onCancel(); }}>
      <DialogContent>
        <form
          className="space-y-3"
          onSubmit={(event) => {
            event.preventDefault();
            const next = code.trim();
            if (next === "") {
              return;
            }
            setCode("");
            props.onSubmit(next);
          }}
        >
          <DialogHeader>
            <DialogTitle>{t("stepUpTitle")}</DialogTitle>
            <DialogDescription>{t("stepUpHint")}</DialogDescription>
          </DialogHeader>
          {props.error === "again" ? <p className="text-sm text-destructive">{t("stepUpAgain")}</p> : null}
          {props.error === "invalid" ? <p className="text-sm text-destructive">{t("stepUpInvalid")}</p> : null}
          <div className="space-y-1">
            <Label htmlFor="step-up-code">{t("code")}</Label>
            <Input id="step-up-code" value={code} onChange={(event) => setCode(event.target.value)} autoComplete="off" autoFocus />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={props.onCancel}>
              {t("common:cancel")}
            </Button>
            <Button type="submit">{t("stepUpConfirm")}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
