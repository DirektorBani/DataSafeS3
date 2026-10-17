import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@datasafe/ui/ui/button";
import { Input } from "@datasafe/ui/ui/input";
import { Label } from "@datasafe/ui/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@datasafe/ui/ui/card";
import {
  confirmMfa,
  forgetProfile,
  login,
  logout,
  oidcExchange,
  oidcPassword,
  saveAccessKey,
  saveApiToken,
  serverInfo,
  type LoginView,
  type Profile,
} from "../desktop";

export function LoginPanel(props: {
  profile: Profile;
  onChanged: () => void;
  onError: (message: string) => void;
  notice: string;
  onNotice: (message: string) => void;
}) {
  const { t } = useTranslation();
  const [password, setPassword] = useState("");
  const [remember, setRemember] = useState(false);
  const [code, setCode] = useState("");
  const [exchange, setExchange] = useState("");
  const [token, setToken] = useState("");
  const [secret, setSecret] = useState("");
  const [mfa, setMfa] = useState(false);
  const profile = props.profile;
  const server = profile.server;

  async function run(action: () => Promise<LoginView | unknown>) {
    try {
      const result = (await action()) as LoginView;
      setPassword("");
      setToken("");
      setSecret("");
      setExchange("");
      setCode("");
      if (result?.status === "mfa_required") {
        setMfa(true);
      } else {
        setMfa(false);
      }
      const notes = [
        result?.secrets_persisted === false &&
        result?.status !== "rejected" &&
        result?.status !== "blocked" &&
        result?.status !== "error"
          ? t("secretsMemory")
          : "",
        result?.message && result.status === "signed_out" ? t("passwordStays") : "",
        result?.mfa_setup ? t("mfaSetup") : "",
        result?.server?.expires_soon ? t("expiresSoon") : "",
      ].filter(Boolean);
      props.onNotice(notes.join(" "));
      props.onChanged();
    } catch (err) {
      props.onError(err instanceof Error ? err.message : String(err));
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{profile.name}</CardTitle>
        <CardDescription>
          {t("session")}: {profile.session || "signed_out"}
          {server?.server_version ? ` · ${server.server_version}` : ""}
          {server?.profile ? ` · ${server.profile}` : ""}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
      {server?.warning === "the server certificate could not be verified" ? <p>{t("tlsCertificate")}</p> : null}
      {server?.warning === "server is unreachable" ? <p>{t("unreachable")}</p> : null}
      {server?.rejected && server.warning !== "the server certificate could not be verified" && server.warning !== "server is unreachable" ? <p>{t("rejected")}</p> : null}
      {server?.blocked ? (
        <p>
          {t("blocked")}{" "}
          {server.advisory_url ? (
            <a className="underline" href={server.advisory_url}>
              {t("advisory")}
            </a>
          ) : null}
        </p>
      ) : null}
      {server?.profile === "basic" ? <p>{t("basic")}</p> : null}
      {server?.warning && !server.blocked && !server.rejected && server.profile !== "basic" ? <p>{server.warning}</p> : null}
      {server?.deprecation ? <p>{server.deprecation}</p> : null}
      {props.notice ? <p className="text-sm">{props.notice}</p> : null}

      {profile.auth_method === "password" ? (
        <form
          className="space-y-2"
          onSubmit={(event) => {
            event.preventDefault();
            void run(() => login(profile.id, profile.username || "", password, remember));
          }}
        >
          <div className="space-y-1">
            <Label htmlFor="login-password">{t("password")}</Label>
            <Input id="login-password" type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="current-password" />
          </div>
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={remember} onChange={(event) => setRemember(event.target.checked)} />
            {t("remember")}
          </label>
          <Button type="submit">{t("signIn")}</Button>
        </form>
      ) : null}

      {mfa || profile.session === "mfa_required" ? (
        <form
          className="space-y-2"
          onSubmit={(event) => {
            event.preventDefault();
            void run(() => confirmMfa(profile.id, code));
          }}
        >
          <div className="space-y-1">
            <Label htmlFor="login-code">{t("code")}</Label>
            <Input id="login-code" value={code} onChange={(event) => setCode(event.target.value)} inputMode="numeric" autoComplete="off" />
          </div>
          <Button type="submit">{t("confirmCode")}</Button>
        </form>
      ) : null}

      {profile.auth_method === "oidc" ? (
        <div className="space-y-3">
          <form
            className="space-y-2"
            onSubmit={(event) => {
              event.preventDefault();
              void run(() => oidcPassword(profile.id, profile.username || "", password, remember));
            }}
          >
            <p className="text-sm font-medium">{t("oidcPassword")}</p>
            <Input type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="current-password" />
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={remember} onChange={(event) => setRemember(event.target.checked)} />
              {t("remember")}
            </label>
            <Button type="submit">{t("signIn")}</Button>
          </form>
          <form
            className="space-y-2"
            onSubmit={(event) => {
              event.preventDefault();
              void run(() => oidcExchange(profile.id, exchange));
            }}
          >
            <p className="text-sm">{t("exchangeHint")}</p>
            <div className="space-y-1">
              <Label htmlFor="login-exchange">{t("exchange")}</Label>
              <Input id="login-exchange" value={exchange} onChange={(event) => setExchange(event.target.value)} autoComplete="off" />
            </div>
            <Button type="submit" variant="outline">
              {t("signIn")}
            </Button>
          </form>
        </div>
      ) : null}

      {profile.auth_method === "api_token" ? (
        <form
          className="space-y-2"
          onSubmit={(event) => {
            event.preventDefault();
            void run(() => saveApiToken(profile.id, token));
          }}
        >
          <div className="space-y-1">
            <Label htmlFor="login-token">{t("apiToken")}</Label>
            <Input id="login-token" type="password" value={token} onChange={(event) => setToken(event.target.value)} autoComplete="off" />
          </div>
          <Button type="submit">{t("saveSecret")}</Button>
        </form>
      ) : null}

      {profile.auth_method === "access_key" ? (
        <form
          className="space-y-2"
          onSubmit={(event) => {
            event.preventDefault();
            void run(() => saveAccessKey(profile.id, secret));
          }}
        >
          <div className="space-y-1">
            <Label htmlFor="login-secret">{t("secret")}</Label>
            <Input id="login-secret" type="password" value={secret} onChange={(event) => setSecret(event.target.value)} autoComplete="off" />
          </div>
          <Button type="submit">{t("saveSecret")}</Button>
        </form>
      ) : null}

      <div className="flex flex-wrap gap-2">
        <Button type="button" variant="outline" onClick={() => void run(() => serverInfo(profile.id))}>
          {t("refresh")}
        </Button>
        <Button type="button" variant="outline" onClick={() => void run(() => logout(profile.id))}>
          {t("logout")}
        </Button>
        <Button
          type="button"
          variant="outline"
          onClick={() =>
            void run(async () => {
              await forgetProfile(profile.id);
              return { status: "signed_out" };
            })
          }
        >
          {t("forget")}
        </Button>
      </div>
      </CardContent>
    </Card>
  );
}
