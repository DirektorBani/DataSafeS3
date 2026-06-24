import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { api, isAuthenticated } from "@/lib/api";
import {
  detectInitialLocale,
  normalizeLocale,
  setStoredLocale,
  type AppLocale,
} from "@/i18n";

type LocaleContextValue = {
  locale: AppLocale;
  setLocale: (locale: AppLocale) => void;
};

const LocaleContext = createContext<LocaleContextValue | null>(null);

export function LocaleProvider({ children }: { children: ReactNode }) {
  const { i18n } = useTranslation();
  const [locale, setLocaleState] = useState<AppLocale>(() => normalizeLocale(i18n.language));

  const setLocale = useCallback(
    (next: AppLocale) => {
      setStoredLocale(next);
      setLocaleState(next);
      void i18n.changeLanguage(next);
      if (isAuthenticated()) {
        void api.updateLocale(next).catch(() => {});
      }
    },
    [i18n]
  );

  useEffect(() => {
    const onLanguageChanged = (lng: string) => setLocaleState(normalizeLocale(lng));
    i18n.on("languageChanged", onLanguageChanged);
    return () => i18n.off("languageChanged", onLanguageChanged);
  }, [i18n]);

  useEffect(() => {
    if (!isAuthenticated()) return;
    void api
      .getMe()
      .then((me) => {
        if (me.locale) {
          const resolved = detectInitialLocale(me.locale);
          if (resolved !== normalizeLocale(i18n.language)) {
            setStoredLocale(resolved);
            void i18n.changeLanguage(resolved);
          }
        }
      })
      .catch(() => {});
  }, [i18n]);

  return <LocaleContext.Provider value={{ locale, setLocale }}>{children}</LocaleContext.Provider>;
}

export function useLocale() {
  const ctx = useContext(LocaleContext);
  if (!ctx) throw new Error("useLocale must be used within LocaleProvider");
  return ctx;
}
