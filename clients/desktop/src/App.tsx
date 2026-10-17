import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@datasafe/ui/ui/button";
import { browse, diagExport, diagPreview, listProfiles, transfer, type DiagPreview, type Profile } from "./desktop";
import { setDesktopLocale, THEME_KEY, type DesktopLocale } from "./i18n";
import { BucketBrowser } from "./pages/buckets";
import { LoginPanel } from "./pages/login";
import { ProfileList } from "./pages/profiles";

const VERSION = "1.0.0-dev";

export default function App() {
  const { t, i18n } = useTranslation();
  const [dark, setDark] = useState(() => document.documentElement.classList.contains("dark"));
  const locale: DesktopLocale = i18n.language === "ru" ? "ru" : "en";
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

  const current = profiles.find((profile) => profile.id === selected);

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
        if (!cancel) {
          setMfaBanner(Boolean(result.mfa_setup));
        }
      })
      .catch(() => undefined);
    return () => {
      cancel = true;
    };
  }, [current]);

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

  return (
    <main className="mx-auto flex min-h-screen max-w-6xl flex-col gap-6 p-8">
      <header className="space-y-1">
        <h1 className="text-2xl font-semibold">{t("title")}</h1>
        <p className="text-sm text-muted-foreground">{t("version", { version: VERSION })}</p>
        <p>{t("subtitle")}</p>
      </header>
      <section className="flex flex-wrap gap-6">
        <div className="space-y-2">
          <h2 className="text-sm font-medium">{t("language")}</h2>
          <div className="flex gap-2">
            <Button type="button" variant={locale === "en" ? "default" : "outline"} onClick={() => setDesktopLocale("en")}>
              EN
            </Button>
            <Button type="button" variant={locale === "ru" ? "default" : "outline"} onClick={() => setDesktopLocale("ru")}>
              RU
            </Button>
          </div>
        </div>
        <div className="space-y-2">
          <h2 className="text-sm font-medium">{t("theme")}</h2>
          <div className="flex gap-2">
            <Button
              type="button"
              variant={dark ? "outline" : "default"}
              onClick={() => {
                document.documentElement.classList.toggle("dark", false);
                localStorage.setItem(THEME_KEY, "light");
                setDark(false);
              }}
            >
              {t("themeLight")}
            </Button>
            <Button
              type="button"
              variant={dark ? "default" : "outline"}
              onClick={() => {
                document.documentElement.classList.toggle("dark", true);
                localStorage.setItem(THEME_KEY, "dark");
                setDark(true);
              }}
            >
              {t("themeDark")}
            </Button>
          </div>
        </div>
      </section>
      {current?.tls?.insecure_dev ? <p className="text-sm text-destructive">{t("insecureWarning")}</p> : null}
      <section className="space-y-2">
        <h2 className="text-sm font-medium">{t("diagnostics")}</h2>
        <p className="text-sm text-muted-foreground">{t("diagHint")}</p>
        <div className="flex flex-wrap items-center gap-2">
          <Button type="button" variant="outline" onClick={() => void previewDiagnostics()}>
            {t("diagPreview")}
          </Button>
          <input
            className="min-w-64 rounded-md border bg-transparent px-2 py-1"
            aria-label={t("diagFolder")}
            placeholder={t("diagFolder")}
            value={diagFolder}
            onChange={(event) => setDiagFolder(event.target.value)}
          />
          <Button type="button" variant="outline" disabled={!diag || !diagFolder.trim()} onClick={() => void saveDiagnostics()}>
            {t("diagSave")}
          </Button>
        </div>
        {diag ? (
          <pre className="max-h-48 overflow-auto whitespace-pre-wrap rounded-md border p-2 text-xs">{`${diag.versions ?? ""}${diag.files?.length ? `\n${diag.files.map((file) => `${file.name} ${file.bytes}`).join("\n")}` : ""}`}</pre>
        ) : null}
        {diagSaved ? <p className="text-sm text-muted-foreground">{diagSaved}</p> : null}
      </section>
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
      {mfaBanner ? (
        <p className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm" role="status">
          {t("mfaSetupBanner")}
        </p>
      ) : null}
      <div className="grid gap-8 md:grid-cols-2">
        <ProfileList
          profiles={profiles}
          selected={selected}
          onSelect={setSelected}
          onCreated={() => void reload()}
          onError={setError}
        />
        {current ? (
          <LoginPanel
            profile={current}
            notice={notice}
            onNotice={setNotice}
            onChanged={() => void reload()}
            onError={setError}
          />
        ) : null}
      </div>
      {current && current.session === "signed_in" && !current.server?.rejected && !current.server?.blocked && current.server?.browse_allowed !== false ? (
        <BucketBrowser profileId={current.id} onError={setError} onMfaSetup={() => setMfaBanner(true)} />
      ) : null}
    </main>
  );
}
