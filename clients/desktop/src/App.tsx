import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@datasafe/ui/ui/button";
import { setDesktopLocale, THEME_KEY, type DesktopLocale } from "./i18n";

const VERSION = "1.0.0-dev";

export default function App() {
  const { t, i18n } = useTranslation();
  const [dark, setDark] = useState(() => document.documentElement.classList.contains("dark"));
  const locale: DesktopLocale = i18n.language === "ru" ? "ru" : "en";

  function chooseLocale(next: DesktopLocale) {
    setDesktopLocale(next);
  }

  function chooseTheme(nextDark: boolean) {
    document.documentElement.classList.toggle("dark", nextDark);
    localStorage.setItem(THEME_KEY, nextDark ? "dark" : "light");
    setDark(nextDark);
  }

  return (
    <main className="mx-auto flex min-h-screen max-w-xl flex-col gap-6 p-8">
      <header className="space-y-1">
        <h1 className="text-2xl font-semibold">{t("title")}</h1>
        <p className="text-sm text-muted-foreground">{t("version", { version: VERSION })}</p>
        <p>{t("subtitle")}</p>
      </header>
      <section className="space-y-2" aria-label={t("language")}>
        <h2 className="text-sm font-medium">{t("language")}</h2>
        <div className="flex gap-2">
          <Button type="button" variant={locale === "en" ? "default" : "outline"} onClick={() => chooseLocale("en")}>
            EN
          </Button>
          <Button type="button" variant={locale === "ru" ? "default" : "outline"} onClick={() => chooseLocale("ru")}>
            RU
          </Button>
        </div>
      </section>
      <section className="space-y-2" aria-label={t("theme")}>
        <h2 className="text-sm font-medium">{t("theme")}</h2>
        <div className="flex gap-2">
          <Button type="button" variant={dark ? "outline" : "default"} onClick={() => chooseTheme(false)}>
            {t("themeLight")}
          </Button>
          <Button type="button" variant={dark ? "default" : "outline"} onClick={() => chooseTheme(true)}>
            {t("themeDark")}
          </Button>
        </div>
      </section>
    </main>
  );
}
