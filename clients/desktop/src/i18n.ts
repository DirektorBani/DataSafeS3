import i18n from "i18next";
import { initReactI18next } from "react-i18next";

export const LOCALE_KEY = "datasafe-desktop.locale";
export const THEME_KEY = "datasafe-desktop.theme";

const resources = {
  en: {
    shell: {
      title: "DataSafeS3 Desktop",
      version: "Version {{version}}",
      subtitle: "Profiles and sign-in. The window does not call the server itself.",
      language: "Language",
      theme: "Theme",
      themeLight: "Light",
      themeDark: "Dark",
      profiles: "Profiles",
      addProfile: "Add profile",
      name: "Name",
      server: "Server URL",
      method: "Sign-in method",
      username: "Username",
      accessKey: "Access key",
      insecure: "Skip TLS verification (dev only)",
      save: "Save profile",
      forget: "Forget profile",
      refresh: "Check server",
      logout: "Sign out",
      password: "Password",
      remember: "Remember password",
      signIn: "Sign in",
      code: "Verification code",
      confirmCode: "Confirm code",
      exchange: "OIDC exchange code",
      exchangeHint: "Paste the one-time code from the server. The system-browser redirect is a later step.",
      oidcPassword: "OIDC username and password",
      apiToken: "API token",
      secret: "Secret key",
      saveSecret: "Save credentials",
      empty: "No profiles yet.",
      desktopOnly: "Open this screen in the DataSafeS3 Desktop window.",
      session: "Session",
      secretsMemory: "This system has no secret store. The secret stays in memory until you close the app.",
      passwordStays: "A password session stays valid on the server until it expires.",
      mfaSetup: "Set up 2FA in the web console.",
      expiresSoon: "The session expires soon. Sign in again before a long transfer.",
      blocked: "This server refuses this version of the app.",
      advisory: "Details",
      rejected: "This server is older than 1.5.0.",
      basic: "The server version is unknown. Browsing stays available.",
      methods: {
        password: "Password",
        oidc: "OIDC",
        api_token: "API token",
        access_key: "Access key",
      },
    },
  },
  ru: {
    shell: {
      title: "DataSafeS3 Desktop",
      version: "Версия {{version}}",
      subtitle: "Профили и вход. Окно само к серверу не обращается.",
      language: "Язык",
      theme: "Тема",
      themeLight: "Светлая",
      themeDark: "Тёмная",
      profiles: "Профили",
      addProfile: "Добавить профиль",
      name: "Имя",
      server: "Адрес сервера",
      method: "Способ входа",
      username: "Имя пользователя",
      accessKey: "Ключ доступа",
      insecure: "Не проверять TLS (только для разработки)",
      save: "Сохранить профиль",
      forget: "Забыть профиль",
      refresh: "Проверить сервер",
      logout: "Выйти",
      password: "Пароль",
      remember: "Запомнить пароль",
      signIn: "Войти",
      code: "Код подтверждения",
      confirmCode: "Подтвердить код",
      exchange: "Код обмена OIDC",
      exchangeHint: "Вставьте одноразовый код сервера. Переход через системный браузер будет позже.",
      oidcPassword: "Имя и пароль OIDC",
      apiToken: "Токен API",
      secret: "Секретный ключ",
      saveSecret: "Сохранить учётные данные",
      empty: "Профилей пока нет.",
      desktopOnly: "Откройте этот экран в окне DataSafeS3 Desktop.",
      session: "Сессия",
      secretsMemory: "На этой системе нет хранилища секретов. Секрет останется в памяти, пока приложение открыто.",
      passwordStays: "Сессия по паролю действует на сервере до истечения срока.",
      mfaSetup: "Настройте 2FA в веб-консоли.",
      expiresSoon: "Сессия скоро истечёт. Войдите снова перед длинной передачей.",
      blocked: "Сервер отказывается работать с этой версией приложения.",
      advisory: "Подробности",
      rejected: "Сервер старше 1.5.0.",
      basic: "Версия сервера не определена. Просмотр остаётся доступным.",
      methods: {
        password: "Пароль",
        oidc: "OIDC",
        api_token: "Токен API",
        access_key: "Ключ доступа",
      },
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
