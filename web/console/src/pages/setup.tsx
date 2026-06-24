import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { CheckCircle2, Cloud, Database, KeyRound, Loader2, PartyPopper } from "lucide-react";
import { toast } from "sonner";
import { api, type ExternalS3Config } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { LanguageSwitcher } from "@/components/language-switcher";

const defaultS3: ExternalS3Config = {
  endpoint: "",
  access_key_id: "",
  secret_access_key: "",
  bucket: "",
  region: "us-east-1",
  use_ssl: true,
};

type SetupStep = "password" | "welcome" | "s3" | "finish";

const SETUP_STEP_IDS: SetupStep[] = ["password", "welcome", "s3", "finish"];

function SetupProgressBar({ step }: { step: SetupStep }) {
  const { t } = useTranslation("setup");
  const currentIndex = SETUP_STEP_IDS.findIndex((s) => s === step);
  const progress = ((currentIndex + 1) / SETUP_STEP_IDS.length) * 100;

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span>
          {t("progress.stepOf", { current: currentIndex + 1, total: SETUP_STEP_IDS.length })}
        </span>
        <span>{Math.round(progress)}%</span>
      </div>
      <div className="h-2 overflow-hidden rounded-full bg-muted">
        <div
          className="h-full rounded-full bg-primary transition-all duration-300 ease-out"
          style={{ width: `${progress}%` }}
        />
      </div>
      <div className="flex flex-wrap gap-2">
        {SETUP_STEP_IDS.map((id, index) => {
          const done = index < currentIndex;
          const active = id === step;
          return (
            <span
              key={id}
              className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${
                active
                  ? "bg-primary text-primary-foreground"
                  : done
                    ? "bg-primary/15 text-primary"
                    : "bg-muted text-muted-foreground"
              }`}
            >
              {t(`steps.${id}`)}
            </span>
          );
        })}
      </div>
    </div>
  );
}

export function SetupPage() {
  const { t } = useTranslation("setup");
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const setupStatus = useQuery({
    queryKey: ["setup-status"],
    queryFn: () => api.getSetupStatus(),
  });

  const [step, setStep] = useState<SetupStep>("welcome");
  const [form, setForm] = useState<ExternalS3Config>(defaultS3);
  const [testMessage, setTestMessage] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");

  const needsPasswordChange = setupStatus.data?.needs_password_change ?? false;

  useEffect(() => {
    if (setupStatus.isLoading) return;
    setStep(needsPasswordChange ? "password" : "welcome");
  }, [needsPasswordChange, setupStatus.isLoading]);

  const invalidateSetup = () => {
    void queryClient.invalidateQueries({ queryKey: ["setup-status"] });
  };

  const passwordMutation = useMutation({
    mutationFn: () => api.changePassword(currentPassword, newPassword),
    onSuccess: () => {
      toast.success(t("toast.passwordChanged"));
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      invalidateSetup();
      setStep("welcome");
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const testMutation = useMutation({
    mutationFn: () => api.testSetupS3(form),
    onSuccess: (data) => {
      setTestMessage(data.message);
      if (data.ok) {
        toast.success(t("toast.connectionSuccess"));
      } else {
        toast.error(data.message || t("toast.connectionFailed"));
      }
    },
    onError: (err: Error) => {
      setTestMessage(err.message);
      toast.error(err.message);
    },
  });

  const saveMutation = useMutation({
    mutationFn: () => api.saveSetupS3(form),
    onSuccess: () => {
      setStep("finish");
      toast.success(t("toast.setupComplete"));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const skipMutation = useMutation({
    mutationFn: () => api.completeSetup(),
    onSuccess: () => {
      setStep("finish");
      toast.success(t("toast.setupComplete"));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  function updateField<K extends keyof ExternalS3Config>(key: K, value: ExternalS3Config[K]) {
    setForm((prev) => ({ ...prev, [key]: value }));
    setTestMessage("");
  }

  const formValid =
    form.endpoint.trim() &&
    form.access_key_id.trim() &&
    form.secret_access_key?.trim() &&
    form.bucket.trim() &&
    form.region.trim();

  const stepMeta = useMemo(() => {
    const icons = {
      password: KeyRound,
      welcome: Database,
      s3: Cloud,
      finish: PartyPopper,
    } as const;
    return {
      icon: icons[step],
      title: t(`stepMeta.${step}.title`),
      description: t(`stepMeta.${step}.description`),
    };
  }, [step, t]);

  const StepIcon = stepMeta.icon;

  if (setupStatus.isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <>
      <div className="fixed right-4 top-4 z-50">
        <LanguageSwitcher />
      </div>
      <Dialog open={step === "password"}>
        <DialogContent
          className="[&>button]:hidden sm:max-w-md"
          onPointerDownOutside={(e) => e.preventDefault()}
          onEscapeKeyDown={(e) => e.preventDefault()}
        >
          <DialogHeader>
            <SetupProgressBar step="password" />
            <DialogTitle>{t("passwordDialog.title")}</DialogTitle>
            <DialogDescription>{t("passwordDialog.description")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="setup-current-password">{t("fields.currentPassword")}</Label>
              <Input
                id="setup-current-password"
                type="password"
                value={currentPassword}
                onChange={(e) => setCurrentPassword(e.target.value)}
                autoComplete="current-password"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="setup-new-password">{t("fields.newPassword")}</Label>
              <Input
                id="setup-new-password"
                type="password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                autoComplete="new-password"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="setup-confirm-password">{t("fields.confirmPassword")}</Label>
              <Input
                id="setup-confirm-password"
                type="password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                autoComplete="new-password"
              />
            </div>
            <Button
              className="w-full"
              disabled={
                !currentPassword ||
                !newPassword ||
                newPassword !== confirmPassword ||
                passwordMutation.isPending
              }
              onClick={() => passwordMutation.mutate()}
            >
              {passwordMutation.isPending ? (
                <>
                  <Loader2 className="h-4 w-4 animate-spin" />
                  {t("actions.saving")}
                </>
              ) : (
                t("actions.savePassword")
              )}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      {step !== "password" && (
      <div className="flex min-h-screen items-center justify-center bg-background p-4">
        <Card className="w-full max-w-lg">
          <CardHeader className="space-y-4 text-center">
            <SetupProgressBar step={step} />
            <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-lg bg-primary/10">
              <StepIcon className="h-6 w-6 text-primary" />
            </div>
            <CardTitle className="text-2xl">{stepMeta.title}</CardTitle>
            <CardDescription>{stepMeta.description}</CardDescription>
          </CardHeader>
          <CardContent>
            {step === "welcome" && (
              <div className="space-y-2">
                <Button className="w-full" onClick={() => setStep("s3")}>
                  {t("actions.startSetup")}
                </Button>
                <Button
                  variant="ghost"
                  className="w-full"
                  disabled={skipMutation.isPending}
                  onClick={() => skipMutation.mutate()}
                >
                  {skipMutation.isPending ? (
                    <>
                      <Loader2 className="h-4 w-4 animate-spin" />
                      {t("actions.skipping")}
                    </>
                  ) : (
                    t("actions.skip")
                  )}
                </Button>
              </div>
            )}

            {step === "s3" && (
              <div className="space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="endpoint">{t("fields.endpoint")}</Label>
                  <Input
                    id="endpoint"
                    placeholder={t("placeholder.endpoint")}
                    value={form.endpoint}
                    onChange={(e) => updateField("endpoint", e.target.value)}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="access_key">{t("fields.accessKey")}</Label>
                  <Input
                    id="access_key"
                    value={form.access_key_id}
                    onChange={(e) => updateField("access_key_id", e.target.value)}
                    autoComplete="off"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="secret_key">{t("fields.secretKey")}</Label>
                  <Input
                    id="secret_key"
                    type="password"
                    value={form.secret_access_key ?? ""}
                    onChange={(e) => updateField("secret_access_key", e.target.value)}
                    autoComplete="new-password"
                  />
                </div>
                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="space-y-2">
                    <Label htmlFor="bucket">{t("fields.bucket")}</Label>
                    <Input
                      id="bucket"
                      value={form.bucket}
                      onChange={(e) => updateField("bucket", e.target.value)}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="region">{t("fields.region")}</Label>
                    <Input
                      id="region"
                      value={form.region}
                      onChange={(e) => updateField("region", e.target.value)}
                    />
                  </div>
                </div>
                <label className="flex items-center gap-2 text-sm">
                  <input
                    type="checkbox"
                    checked={form.use_ssl}
                    onChange={(e) => updateField("use_ssl", e.target.checked)}
                    className="rounded border-input"
                  />
                  {t("fields.useSsl")}
                </label>

                {testMessage && (
                  <p
                    className={`text-sm ${testMutation.data?.ok ? "text-green-600" : "text-destructive"}`}
                  >
                    {testMessage}
                  </p>
                )}

                <div className="flex flex-col gap-2 sm:flex-row">
                  <Button
                    variant="outline"
                    className="flex-1"
                    disabled={!formValid || testMutation.isPending}
                    onClick={() => testMutation.mutate()}
                  >
                    {testMutation.isPending ? (
                      <>
                        <Loader2 className="h-4 w-4 animate-spin" />
                        {t("actions.testing")}
                      </>
                    ) : (
                      t("actions.testConnection")
                    )}
                  </Button>
                  <Button
                    className="flex-1"
                    disabled={!formValid || saveMutation.isPending}
                    onClick={() => saveMutation.mutate()}
                  >
                    {saveMutation.isPending ? (
                      <>
                        <Loader2 className="h-4 w-4 animate-spin" />
                        {t("actions.saving")}
                      </>
                    ) : (
                      <>
                        <CheckCircle2 className="h-4 w-4" />
                        {t("actions.save")}
                      </>
                    )}
                  </Button>
                </div>
                <div className="flex flex-col gap-2 sm:flex-row">
                  <Button variant="ghost" className="flex-1" onClick={() => setStep("welcome")}>
                    {t("actions.back")}
                  </Button>
                  <Button
                    variant="ghost"
                    className="flex-1"
                    disabled={skipMutation.isPending}
                    onClick={() => skipMutation.mutate()}
                  >
                    {skipMutation.isPending ? (
                      <>
                        <Loader2 className="h-4 w-4 animate-spin" />
                        {t("actions.skipping")}
                      </>
                    ) : (
                      t("actions.skipS3")
                    )}
                  </Button>
                </div>
              </div>
            )}

            {step === "finish" && (
              <Button
                className="w-full"
                onClick={() => {
                  invalidateSetup();
                  navigate("/", { replace: true });
                }}
              >
                {t("actions.goToConsole")}
              </Button>
            )}
          </CardContent>
        </Card>
      </div>
      )}
    </>
  );
}
