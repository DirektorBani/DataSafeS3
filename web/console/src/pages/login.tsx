import { useCallback, useEffect, useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { ArrowLeft, Database, Loader2, ShieldCheck } from "lucide-react";
import { api, getToken, login, loginMFA, setSessionProfile } from "@/lib/api";
import { useAuth } from "@/hooks/use-auth";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { OtpInput, useTotpCountdown } from "@/components/otp-input";
import { LanguageSwitcher } from "@/components/language-switcher";

type LoginForm = { username: string; password: string };

export function LoginPage() {
  const { t } = useTranslation("login");
  const { login: onLogin } = useAuth();
  const [searchParams, setSearchParams] = useSearchParams();
  const [error, setError] = useState("");
  const [mfaStep, setMfaStep] = useState(false);
  const [mfaToken, setMfaToken] = useState("");
  const [mfaCode, setMfaCode] = useState("");
  const [recoveryMode, setRecoveryMode] = useState(false);
  const [recoveryCode, setRecoveryCode] = useState("");
  const [mfaSubmitting, setMfaSubmitting] = useState(false);
  const totpRemaining = useTotpCountdown();
  const oidc = useQueryOIDC();
  const setupStatus = useQuery({
    queryKey: ["setup-status"],
    queryFn: () => api.getSetupStatus(),
    staleTime: 30_000,
  });
  const showDefaultCredentials =
    !setupStatus.data?.admin_first_login_completed &&
    !setupStatus.data?.initial_setup_completed;

  const loginSchema = useMemo(
    () =>
      z.object({
        username: z.string().min(1, t("validation.usernameRequired")),
        password: z.string().min(1, t("validation.passwordRequired")),
      }),
    [t]
  );

  useEffect(() => {
    const exchangeCode = searchParams.get("exchange_code");
    const authSource = searchParams.get("auth_source") ?? undefined;
    if (exchangeCode) {
      api.exchangeOidcCode(exchangeCode).then((data) => {
        setSessionProfile(data.token, "user", "", { authSource: data.auth_source ?? authSource });
        setSearchParams({}, { replace: true });
        onLogin();
        return api.getMe();
      }).then((me) => {
        if (!me) return;
        setSessionProfile(getToken() ?? "", me.role, me.username, {
          authSource: me.auth_source ?? authSource,
          tenantMemberships: me.tenant_memberships,
          isTenantAdmin: me.is_tenant_admin,
        });
        onLogin();
      }).catch(() => setError(t("error.loginFailed")));
      return;
    }
    const token = searchParams.get("token");
    if (!token) return;

    setSessionProfile(token, "user", "", { authSource });
    setSearchParams({}, { replace: true });
    onLogin();

    api.getMe().then((me) => {
      setSessionProfile(token, me.role, me.username, {
        authSource: me.auth_source ?? authSource,
        tenantMemberships: me.tenant_memberships,
        isTenantAdmin: me.is_tenant_admin,
      });
      onLogin();
    }).catch(() => {});
  }, [searchParams, setSearchParams, onLogin]);

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<LoginForm>({
    resolver: zodResolver(loginSchema),
    defaultValues: { username: "admin", password: "admin" },
  });

  async function onSubmit(data: LoginForm) {
    setError("");
    try {
      const result = await login(data.username, data.password);
      if (result.mfa_required && result.mfa_token) {
        setMfaStep(true);
        setMfaToken(result.mfa_token);
        setMfaCode("");
        setRecoveryCode("");
        setRecoveryMode(false);
        return;
      }
      onLogin();
    } catch (err) {
      setError(err instanceof Error ? err.message : t("error.loginFailed"));
    }
  }

  const submitMFA = useCallback(async (code: string) => {
    if (!code.trim() || mfaSubmitting) return;
    setError("");
    setMfaSubmitting(true);
    try {
      await loginMFA(mfaToken, code.trim());
      onLogin();
    } catch (err) {
      setError(err instanceof Error ? err.message : t("error.invalidCode"));
      if (!recoveryMode) setMfaCode("");
    } finally {
      setMfaSubmitting(false);
    }
  }, [mfaSubmitting, mfaToken, onLogin, recoveryMode, t]);

  function backToLogin() {
    setMfaStep(false);
    setMfaToken("");
    setMfaCode("");
    setRecoveryCode("");
    setRecoveryMode(false);
    setError("");
  }

  return (
    <div className="relative flex min-h-screen items-center justify-center bg-background p-4">
      <div className="absolute right-4 top-4">
        <LanguageSwitcher />
      </div>
      <Card className="w-full max-w-md">
        <CardHeader className="space-y-3 text-center">
          <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-lg bg-primary/10">
            {mfaStep ? (
              <ShieldCheck className="h-6 w-6 text-primary" />
            ) : (
              <Database className="h-6 w-6 text-primary" />
            )}
          </div>
          <CardTitle className="text-2xl">{mfaStep ? t("title.mfa") : t("title.brand")}</CardTitle>
          <CardDescription>
            {mfaStep
              ? recoveryMode
                ? t("description.mfaRecovery")
                : t("description.mfa")
              : t("description.signIn")}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {mfaStep ? (
            <div className="space-y-4">
              {!recoveryMode ? (
                <>
                  <OtpInput
                    value={mfaCode}
                    onChange={setMfaCode}
                    onComplete={submitMFA}
                    disabled={mfaSubmitting}
                    autoFocus
                  />
                  <p className="text-center text-xs text-muted-foreground">
                    {t("mfa.codeRefreshes", { seconds: totpRemaining })}
                  </p>
                </>
              ) : (
                <div className="space-y-2">
                  <Label htmlFor="recovery-code">{t("fields.recoveryCode")}</Label>
                  <Input
                    id="recovery-code"
                    value={recoveryCode}
                    onChange={(e) => setRecoveryCode(e.target.value.trim())}
                    placeholder={t("placeholder.recoveryCode")}
                    autoComplete="off"
                    autoFocus
                  />
                </div>
              )}

              {error && <p className="text-sm text-destructive text-center">{error}</p>}

              {recoveryMode ? (
                <Button
                  className="w-full"
                  onClick={() => submitMFA(recoveryCode)}
                  disabled={!recoveryCode || mfaSubmitting}
                >
                  {mfaSubmitting ? (
                    <>
                      <Loader2 className="h-4 w-4 animate-spin" />
                      {t("actions.verifying")}
                    </>
                  ) : (
                    t("actions.verifyRecovery")
                  )}
                </Button>
              ) : (
                <Button
                  className="w-full"
                  onClick={() => submitMFA(mfaCode)}
                  disabled={mfaCode.length !== 6 || mfaSubmitting}
                >
                  {mfaSubmitting ? (
                    <>
                      <Loader2 className="h-4 w-4 animate-spin" />
                      {t("actions.verifying")}
                    </>
                  ) : (
                    t("actions.verify")
                  )}
                </Button>
              )}

              <div className="flex flex-col gap-2">
                <Button
                  variant="link"
                  className="h-auto p-0 text-sm"
                  onClick={() => {
                    setRecoveryMode((v) => !v);
                    setMfaCode("");
                    setRecoveryCode("");
                    setError("");
                  }}
                >
                  {recoveryMode ? t("actions.useAuthenticator") : t("actions.useRecovery")}
                </Button>
                <Button variant="ghost" className="w-full" onClick={backToLogin} disabled={mfaSubmitting}>
                  <ArrowLeft className="h-4 w-4 mr-1" />
                  {t("actions.backToLogin")}
                </Button>
              </div>
            </div>
          ) : (
            <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="username">{t("fields.username")}</Label>
                <Input id="username" autoComplete="username" {...register("username")} />
                {errors.username && (
                  <p className="text-sm text-destructive">{errors.username.message}</p>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="password">{t("fields.password")}</Label>
                <Input
                  id="password"
                  type="password"
                  autoComplete="current-password"
                  {...register("password")}
                />
                {errors.password && (
                  <p className="text-sm text-destructive">{errors.password.message}</p>
                )}
              </div>
              {error && <p className="text-sm text-destructive">{error}</p>}
              <Button type="submit" className="w-full" disabled={isSubmitting}>
                {isSubmitting ? (
                  <>
                    <Loader2 className="h-4 w-4 animate-spin" />
                    {t("actions.signingIn")}
                  </>
                ) : (
                  t("actions.signIn")
                )}
              </Button>
            </form>
          )}
          {oidc.enabled && !mfaStep && (
            <>
              {oidc.issuerError && (
                <p className="mt-4 text-sm text-destructive">{t("error.oidcIssuerUnreachable")}</p>
              )}
              <Button
                variant="outline"
                className="mt-4 w-full"
                disabled={!!oidc.issuerError}
                onClick={() => { window.location.href = "/api/v1/auth/oidc/login"; }}
              >
                {t("actions.signInSso")}
              </Button>
            </>
          )}
          {showDefaultCredentials && !mfaStep && (
            <p className="mt-4 text-center text-xs text-muted-foreground">
              {t("hint.defaultCredentials")}
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function useQueryOIDC() {
  const [state, setState] = useState<{ enabled: boolean; issuerError?: string }>({ enabled: false });
  useEffect(() => {
    api.getOIDCConfig()
      .then((c) => {
        setState({
          enabled: c.enabled,
          issuerError:
            c.enabled && c.issuer_reachable === false
              ? c.issuer_error ?? "unreachable"
              : undefined,
        });
      })
      .catch(() => {});
  }, []);
  return state;
}
