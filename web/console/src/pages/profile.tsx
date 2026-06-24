import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Download, Fingerprint, KeyRound, ShieldCheck } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { PageHeader } from "@/components/page-header";
import { CopyButton } from "@/components/copy-button";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { OtpInput, useTotpCountdown } from "@/components/otp-input";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";

type EnrollData = { secret: string; otpauth_uri: string; qr_code: string };

function roleLabel(role: string, t: (key: string) => string): string {
  if (role === "tenant_admin") return t("common:roles.tenantAdmin");
  if (role === "administrator" || role === "operator" || role === "user") {
    return t(`common:roles.${role}`);
  }
  return role;
}

export function ProfilePage() {
  const { t } = useTranslation(["profile", "common"]);
  const queryClient = useQueryClient();
  const [mfaCode, setMfaCode] = useState("");
  const [disablePassword, setDisablePassword] = useState("");
  const [disableCode, setDisableCode] = useState("");
  const [disableOpen, setDisableOpen] = useState(false);
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null);
  const [enrollData, setEnrollData] = useState<EnrollData | null>(null);
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const totpRemaining = useTotpCountdown();

  const me = useQuery({ queryKey: ["me"], queryFn: () => api.getMe() });

  const enrollMutation = useMutation({
    mutationFn: () => api.mfaEnroll(),
    onSuccess: (data) => {
      setEnrollData(data);
      toast.success(t("profile:toast.scanQr"));
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const verifyMutation = useMutation({
    mutationFn: (code: string) => api.mfaVerify(code),
    onSuccess: (data) => {
      setRecoveryCodes(data.recovery_codes);
      setEnrollData(null);
      setMfaCode("");
      localStorage.removeItem("datasafe_mfa_setup_required");
      queryClient.invalidateQueries({ queryKey: ["me"] });
      toast.success(t("profile:toast.mfaEnabled"));
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const disableMutation = useMutation({
    mutationFn: () => api.mfaDisable(disablePassword, disableCode),
    onSuccess: () => {
      setDisablePassword("");
      setDisableCode("");
      setDisableOpen(false);
      queryClient.invalidateQueries({ queryKey: ["me"] });
      toast.success(t("profile:toast.mfaDisabled"));
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const passwordMutation = useMutation({
    mutationFn: () => api.changePassword(currentPassword, newPassword),
    onSuccess: () => {
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      toast.success(t("profile:toast.passwordChanged"));
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const passkeyMutation = useMutation({
    mutationFn: async () => {
      if (!window.PublicKeyCredential) {
        throw new Error(t("profile:passkey.unsupported"));
      }
      const options = await api.webauthnRegisterBegin();
      const publicKey = PublicKeyCredential.parseCreationOptionsFromJSON(
        options as PublicKeyCredentialCreationOptionsJSON
      );
      const cred = (await navigator.credentials.create({ publicKey })) as PublicKeyCredential | null;
      if (!cred) {
        throw new Error(t("profile:passkey.cancelled"));
      }
      const body =
        typeof cred.toJSON === "function"
          ? cred.toJSON()
          : {
              id: cred.id,
              rawId: bufferToBase64URL(cred.rawId),
              type: cred.type,
              response: {
                clientDataJSON: bufferToBase64URL((cred.response as AuthenticatorAttestationResponse).clientDataJSON),
                attestationObject: bufferToBase64URL(
                  (cred.response as AuthenticatorAttestationResponse).attestationObject
                ),
              },
            };
      return api.webauthnRegisterFinish(body);
    },
    onSuccess: (data) => {
      localStorage.removeItem("datasafe_mfa_setup_required");
      queryClient.invalidateQueries({ queryKey: ["me"] });
      toast.success(t("profile:passkey.added", { count: data.passkeys }));
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const externalAuth = me.data?.auth_source === "ldap" || me.data?.auth_source === "oidc";

  function downloadRecoveryCodes() {
    if (!recoveryCodes) return;
    const blob = new Blob([recoveryCodes.join("\n")], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "datasafe-recovery-codes.txt";
    a.click();
    URL.revokeObjectURL(url);
  }

  return (
    <div>
      <PageHeader title={t("profile:title")} description={t("profile:description")} />
      {me.data && (
        <Card className="mb-6">
          <CardHeader>
            <CardTitle className="text-base">{t("profile:account.title")}</CardTitle>
            <CardDescription>{me.data.email ?? me.data.username}</CardDescription>
          </CardHeader>
          <CardContent className="text-sm space-y-1">
            <p>{t("profile:account.role")} {roleLabel(me.data.role, t)}</p>
            {me.data.tenant_id && <p>{t("profile:account.primaryTenant")} {me.data.tenant_id}</p>}
            {(me.data.tenant_memberships ?? []).length > 0 && (
              <div className="pt-2">
                <p className="font-medium mb-1">{t("profile:account.tenantMemberships")}</p>
                <ul className="list-disc pl-5 space-y-0.5">
                  {me.data.tenant_memberships!.map((m) => (
                    <li key={m.tenant_id}>
                      {m.tenant_name ?? m.tenant_id} — {roleLabel(m.role, t)}
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <ShieldCheck className="h-4 w-4" />
            {t("profile:mfa.title")}
          </CardTitle>
          <CardDescription>
            {me.data?.mfa_enabled ? t("profile:mfa.enabled") : t("profile:mfa.disabled")}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {!me.data?.mfa_enabled && !enrollData && (
            <Button onClick={() => enrollMutation.mutate()} disabled={enrollMutation.isPending}>
              {t("profile:mfa.enable")}
            </Button>
          )}
          {enrollData && (
            <div className="space-y-4 max-w-sm">
              <p className="text-sm text-muted-foreground">{t("profile:mfa.scanApps")}</p>
              <img
                src={`data:image/png;base64,${enrollData.qr_code}`}
                alt={t("profile:mfa.qrAlt")}
                className="mx-auto rounded border bg-white p-2"
                width={200}
                height={200}
              />
              <div>
                <p className="text-sm text-muted-foreground mb-1">{t("profile:mfa.manualSecret")}</p>
                <div className="flex items-center gap-2">
                  <code className="flex-1 rounded bg-muted p-2 text-xs break-all">{enrollData.secret}</code>
                  <CopyButton value={enrollData.secret} label={t("profile:mfa.secretLabel")} />
                </div>
              </div>
              <div className="space-y-3">
                <Label>{t("profile:mfa.verificationCode")}</Label>
                <OtpInput
                  value={mfaCode}
                  onChange={setMfaCode}
                  onComplete={(code) => {
                    if (!verifyMutation.isPending) verifyMutation.mutate(code);
                  }}
                  disabled={verifyMutation.isPending}
                  autoFocus
                />
                <p className="text-center text-xs text-muted-foreground">
                  {t("profile:mfa.confirmSetup", { seconds: totpRemaining })}
                </p>
                <Button
                  className="w-full"
                  onClick={() => verifyMutation.mutate(mfaCode)}
                  disabled={mfaCode.length !== 6 || verifyMutation.isPending}
                >
                  {t("profile:mfa.verifyAndEnable")}
                </Button>
                <Button
                  variant="ghost"
                  className="w-full"
                  onClick={() => {
                    setEnrollData(null);
                    setMfaCode("");
                  }}
                  disabled={verifyMutation.isPending}
                >
                  {t("profile:mfa.cancelSetup")}
                </Button>
              </div>
            </div>
          )}
          {recoveryCodes && (
            <div className="space-y-3 rounded-lg border border-amber-500/40 bg-amber-500/5 p-4">
              <p className="text-sm font-medium">{t("profile:recovery.title")}</p>
              <ul className="grid grid-cols-2 gap-1 font-mono text-xs">
                {recoveryCodes.map((c) => (
                  <li key={c}>{c}</li>
                ))}
              </ul>
              <div className="flex gap-2">
                <CopyButton value={recoveryCodes.join("\n")} label={t("profile:recovery.copyAll")} />
                <Button variant="outline" size="sm" onClick={downloadRecoveryCodes}>
                  <Download className="h-4 w-4 mr-1" />
                  {t("profile:recovery.download")}
                </Button>
              </div>
            </div>
          )}
          {me.data?.mfa_enabled && !enrollData && (
            <Button variant="destructive" onClick={() => setDisableOpen(true)}>
              {t("profile:mfa.disable")}
            </Button>
          )}
        </CardContent>
      </Card>

      <Card className="mt-6">
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <Fingerprint className="h-4 w-4" />
            {t("profile:passkey.title")}
          </CardTitle>
          <CardDescription>
            {me.data?.webauthn_enabled
              ? t("profile:passkey.enabled", { count: me.data.passkey_count ?? 1 })
              : t("profile:passkey.disabled")}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <p className="text-sm text-muted-foreground">{t("profile:passkey.hint")}</p>
          <Button onClick={() => passkeyMutation.mutate()} disabled={passkeyMutation.isPending}>
            {t("profile:passkey.add")}
          </Button>
        </CardContent>
      </Card>

      <Card className="mt-6">
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <KeyRound className="h-4 w-4" />
            {t("profile:password.title")}
          </CardTitle>
          <CardDescription>
            {me.data?.auth_source === "ldap"
              ? t("profile:password.ldap")
              : me.data?.auth_source === "oidc"
                ? t("profile:password.oidc")
                : t("profile:password.local")}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {externalAuth ? (
            <p className="text-sm text-muted-foreground">
              {me.data?.auth_source === "ldap" ? t("profile:password.ldapHint") : t("profile:password.oidcHint")}
            </p>
          ) : (
            <>
              <div className="space-y-2 max-w-sm">
                <Label htmlFor="current-password">{t("profile:fields.currentPassword")}</Label>
                <Input
                  id="current-password"
                  type="password"
                  value={currentPassword}
                  onChange={(e) => setCurrentPassword(e.target.value)}
                  autoComplete="current-password"
                />
              </div>
              <div className="space-y-2 max-w-sm">
                <Label htmlFor="new-password">{t("profile:fields.newPassword")}</Label>
                <Input
                  id="new-password"
                  type="password"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  autoComplete="new-password"
                />
              </div>
              <div className="space-y-2 max-w-sm">
                <Label htmlFor="confirm-password">{t("profile:fields.confirmPassword")}</Label>
                <Input
                  id="confirm-password"
                  type="password"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  autoComplete="new-password"
                />
              </div>
              <Button
                onClick={() => passwordMutation.mutate()}
                disabled={
                  !currentPassword ||
                  !newPassword ||
                  newPassword !== confirmPassword ||
                  passwordMutation.isPending
                }
              >
                {t("profile:actions.changePassword")}
              </Button>
            </>
          )}
        </CardContent>
      </Card>

      <AlertDialog open={disableOpen} onOpenChange={setDisableOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("profile:disableMfa.title")}</AlertDialogTitle>
            <AlertDialogDescription>{t("profile:disableMfa.description")}</AlertDialogDescription>
          </AlertDialogHeader>
          <div className="space-y-3 py-2">
            <div className="space-y-2">
              <Label htmlFor="disable-password">{t("profile:fields.password")}</Label>
              <Input
                id="disable-password"
                type="password"
                value={disablePassword}
                onChange={(e) => setDisablePassword(e.target.value)}
                autoComplete="current-password"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="disable-code">{t("profile:fields.authenticatorCode")}</Label>
              <Input
                id="disable-code"
                value={disableCode}
                onChange={(e) => setDisableCode(e.target.value)}
                autoComplete="one-time-code"
              />
            </div>
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={disableMutation.isPending}>{t("common:cancel")}</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault();
                disableMutation.mutate();
              }}
              disabled={!disablePassword || !disableCode || disableMutation.isPending}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {t("profile:mfa.disable")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function bufferToBase64URL(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  let binary = "";
  for (const b of bytes) {
    binary += String.fromCharCode(b);
  }
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}
