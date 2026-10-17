import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { ShieldAlert } from "lucide-react";
import { Button } from "@datasafe/ui/ui/button";
import { Input } from "@datasafe/ui/ui/input";
import { Card, CardContent, CardHeader, CardTitle } from "@datasafe/ui/ui/card";
import { PageHeader } from "@datasafe/ui/page-header";
import { browse, diagExport, diagPreview, listProfiles, loadSettings, logout, saveSettings, transfer, type DiagPreview, type Profile } from "./desktop";
import { setDesktopLocale, THEME_KEY } from "./i18n";
import { ResumeProgress } from "./components/transfers/panel";
import { DesktopShell, type DesktopTheme, type DesktopView } from "./components/shell";
import { BucketBrowser } from "./pages/buckets";
import { LoginPanel } from "./pages/login";
import { ProfileList } from "./pages/profiles";

const VERSION = "1.0.0-dev";

function storedTheme(): DesktopTheme {
  const saved = localStorage.getItem(THEME_KEY);
  if (saved === "light" || saved === "dark" || saved === "system") {
    return saved;
  }
  return "dark";
}

function applyTheme(theme: DesktopTheme, persist = true) {
  const dark = theme === "dark" || (theme === "system" && window.matchMedia("(prefers-color-scheme: dark)").matches);
  document.documentElement.classList.toggle("dark", dark);
  localStorage.setItem(THEME_KEY, theme);
  if (persist) {
    void saveSettings({ theme }).catch(() => undefined);
  }
}

export default function App() {
  const { t } = useTranslation();
  const [theme, setTheme] = useState<DesktopTheme>(storedTheme);
  const [view, setView] = useState<DesktopView>("profiles");
  const [bucketListToken, setBucketListToken] = useState(0);
  const [transferNote, setTransferNote] = useState("");
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [selected, setSelected] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [mfaBanner, setMfaBanner] = useState(false);
  const resumed = useRef(new Set<string>());
  const [diag, setDiag] = useState<DiagPreview | null>(null);
  const [diagFolder, setDiagFolder] = useState("");
  const [diagSaved, setDiagSaved] = useState("");

  const reload = useCallback(async () => {
    try {
      const next = await listProfiles();
      setProfiles(next);
      setSelected((current) => current || next[0]?.id || "");
      setError("");
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      setError(message.includes("invoke") || message.includes("tauri") ? t("desktopOnly") : message);
    }
  }, [t]);

  useEffect(() => {
    void reload();
  }, [reload]);

  useEffect(() => {
    let cancel = false;
    void loadSettings()
      .then((saved) => {
        if (cancel) {
          return;
        }
        if (saved.locale === "en" || saved.locale === "ru") {
          setDesktopLocale(saved.locale, false);
        }
        if (saved.theme === "light" || saved.theme === "dark" || saved.theme === "system") {
          applyTheme(saved.theme, false);
          setTheme(saved.theme);
        }
      })
      .catch(() => undefined);
    return () => {
      cancel = true;
    };
  }, []);

  const current = profiles.find((profile) => profile.id === selected);
  const signedIn = Boolean(current && current.session === "signed_in" && !current.server?.rejected);
  const canBrowse = Boolean(signedIn && current && !current.server?.blocked && current.server?.browse_allowed !== false);

  useEffect(() => {
    if (!current) {
      setTransferNote("");
    }
  }, [current]);

  useEffect(() => {
    const signed = new Set(
      profiles.filter((profile) => profile.session === "signed_in" && !profile.server?.rejected).map((profile) => profile.id),
    );
    for (const id of [...resumed.current]) {
      if (!signed.has(id)) {
        resumed.current.delete(id);
      }
    }
    for (const id of signed) {
      if (resumed.current.has(id)) {
        continue;
      }
      resumed.current.add(id);
      void transfer(id, "transfer.resume").catch(() => undefined);
    }
  }, [profiles]);

  useEffect(() => {
    if (!current || current.session !== "signed_in" || current.server?.rejected) {
      setMfaBanner(false);
      return;
    }
    let cancel = false;
    void browse(current.id, "account.me")
      .then((result) => {
        if (cancel) {
          return;
        }
        if (result.status === 401 && !result.step_up) {
          void reload();
          return;
        }
        setMfaBanner(Boolean(result.mfa_setup));
      })
      .catch(() => undefined);
    return () => {
      cancel = true;
    };
  }, [current, reload]);

  async function previewDiagnostics() {
    setError("");
    setDiagSaved("");
    try {
      setDiag(await diagPreview(selected));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  async function saveDiagnostics() {
    if (!diag || !diagFolder.trim()) {
      return;
    }
    setError("");
    try {
      const saved = await diagExport(diagFolder.trim(), selected);
      setDiagSaved(saved.path ?? "");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  function reportError(message: string) {
    if (message === "auth.session_expired" || message === "auth.unauthorized") {
      void reload();
      return;
    }
    setError(message);
  }

  async function signOut() {
    if (!current) {
      return;
    }
    try {
      await logout(current.id);
      await reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  const banner = (
    <>
      {current?.session === "reauth" ? (
        <div className="border-b px-6 py-3 text-sm lg:px-8" role="status">
          <p>{t("sessionExpired")}</p>
        </div>
      ) : null}
      {mfaBanner ? (
        <div className="border-b border-destructive/40 bg-destructive/10 px-6 py-3 text-sm lg:px-8" role="status">
          <div className="flex items-start gap-2">
            <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0 text-destructive" aria-hidden />
            <p>{t("mfaSetupBanner")}</p>
          </div>
        </div>
      ) : null}
      {current?.tls?.insecure_dev ? <p className="border-b px-6 py-2 text-sm text-destructive lg:px-8">{t("insecureWarning")}</p> : null}
      {error ? <p className="border-b px-6 py-2 text-sm text-destructive lg:px-8">{error}</p> : null}
    </>
  );

  return (
    <DesktopShell
      view={view}
      onView={(next) => {
        if (next === "buckets") {
          setBucketListToken((n) => n + 1);
        }
        setView(next);
      }}
      theme={theme}
      onTheme={(next) => {
        applyTheme(next);
        setTheme(next);
      }}
      version={VERSION}
      signedIn={signedIn}
      onSignOut={() => void signOut()}
      banner={banner}
    >
      {view === "profiles" ? (
        <div className="space-y-6">
          <PageHeader title={t("profiles")} description={t("subtitle")} />
          <div className="grid gap-6 lg:grid-cols-2">
            <ProfileList profiles={profiles} selected={selected} onSelect={setSelected} onCreated={() => void reload()} onError={setError} />
            {current ? (
              <LoginPanel profile={current} notice={notice} onNotice={setNotice} onChanged={() => void reload()} onError={setError} />
            ) : null}
          </div>
          {current ? <ResumeProgress profileId={current.id} onSessionExpired={() => void reload()} /> : null}
        </div>
      ) : null}
      {view === "buckets" ? (
        canBrowse && current ? (
          <BucketBrowser profileId={current.id} features={current.server?.features} resetToken={bucketListToken} onError={reportError} onMfaSetup={() => setMfaBanner(true)} />
        ) : (
          <PageHeader title={t("nav:buckets")} description={t("subtitle")} />
        )
      ) : null}
      {view === "transfers" ? (
        <div className="space-y-4">
          <PageHeader
            title={t("transfers")}
            description={transferNote || t("transferIdle")}
            actions={
              current ? (
                <Button type="button" variant="outline" onClick={() => void transfer(current.id, "transfer.clear_finished").catch((err: unknown) => setError(err instanceof Error ? err.message : String(err)))}>
                  {t("clearFinished")}
                </Button>
              ) : null
            }
          />
          {current ? (
            <ResumeProgress profileId={current.id} onSummary={setTransferNote} onSessionExpired={() => void reload()} />
          ) : (
            <p className="text-sm text-muted-foreground">{t("empty")}</p>
          )}
        </div>
      ) : null}
      {view === "diagnostics" ? (
        <div className="space-y-4">
          <PageHeader title={t("diagnostics")} description={t("diagHint")} />
          <Card>
            <CardHeader>
              <CardTitle className="text-base">{t("diagPreview")}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="flex flex-wrap items-center gap-2">
                <Button type="button" variant="outline" onClick={() => void previewDiagnostics()}>
                  {t("diagPreview")}
                </Button>
                <Input aria-label={t("diagFolder")} placeholder={t("diagFolder")} value={diagFolder} onChange={(event) => setDiagFolder(event.target.value)} className="max-w-sm" />
                <Button type="button" variant="outline" disabled={!diag || !diagFolder.trim()} onClick={() => void saveDiagnostics()}>
                  {t("diagSave")}
                </Button>
              </div>
              {diag ? (
                <pre className="max-h-48 overflow-auto whitespace-pre-wrap rounded-md border p-3 text-xs">{`${diag.versions ?? ""}${diag.files?.length ? `\n${diag.files.map((file) => `${file.name} ${file.bytes}`).join("\n")}` : ""}`}</pre>
              ) : null}
              {diagSaved ? <p className="text-sm text-muted-foreground">{diagSaved}</p> : null}
            </CardContent>
          </Card>
        </div>
      ) : null}
      {view === "settings" ? (
        <div className="space-y-4">
          <PageHeader title={t("nav:settings")} description={t("subtitle")} />
          <Card>
            <CardHeader>
              <CardTitle className="text-base">{t("common:theme.appearance")}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <p className="text-sm text-muted-foreground">{t("prefsKept")}</p>
              <div className="flex flex-wrap gap-2">
                {(["light", "dark", "system"] as const).map((item) => (
                  <Button key={item} type="button" variant={theme === item ? "default" : "outline"} onClick={() => { applyTheme(item); setTheme(item); }}>
                    {t(`common:theme.${item}`)}
                  </Button>
                ))}
              </div>
            </CardContent>
          </Card>
        </div>
      ) : null}
    </DesktopShell>
  );
}
