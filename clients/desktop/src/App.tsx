import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@datasafe/ui/ui/button";
import { listProfiles, type Profile } from "./desktop";
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
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
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
        <BucketBrowser profileId={current.id} onError={setError} />
      ) : null}
    </main>
  );
}
