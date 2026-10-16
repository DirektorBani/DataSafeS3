import i18n from "i18next";
import { initReactI18next } from "react-i18next";

export const LOCALE_KEY = "datasafe-desktop.locale";
export const THEME_KEY = "datasafe-desktop.theme";

const resources = {
  en: {
    shell: {
      title: "DataSafeS3 Desktop",
      version: "Version {{version}}",
      subtitle: "No profile yet. Sign-in arrives in the next step.",
      language: "Language",
      theme: "Theme",
      themeLight: "Light",
      themeDark: "Dark",
    },
  },
  ru: {
    shell: {
      title: "DataSafeS3 Desktop",
      version: "Версия {{version}}",
      subtitle: "Профиля ещё нет. Вход появится на следующем шаге.",
      language: "Язык",
      theme: "Тема",
      themeLight: "Светлая",
      themeDark: "Тёмная",
    },
  },
} as const;

export type DesktopLocale = keyof typeof resources;

function storedLocale(): DesktopLocale {
  const saved = localStorage.getItem(LOCALE_KEY);
  return saved === "ru" ? "ru" : "en";
}

void i18n.use(initReactI18next).init({
  resources,
  lng: storedLocale(),
  fallbackLng: "en",
  ns: ["shell"],
  defaultNS: "shell",
  interpolation: { escapeValue: false },
});

export function setDesktopLocale(locale: DesktopLocale) {
  localStorage.setItem(LOCALE_KEY, locale);
  void i18n.changeLanguage(locale);
  document.documentElement.lang = locale;
}

export default i18n;
