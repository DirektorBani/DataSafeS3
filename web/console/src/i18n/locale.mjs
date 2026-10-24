export const LOCALE_KEY = "datasafe.locale";

/** Russian is first: it is the product default. English is the fallback. */
export const SUPPORTED_LOCALES = ["ru", "en"];

export function supportedLocale(lang) {
  if (!lang) return null;
  const base = String(lang).trim().toLowerCase().split("-")[0];
  if (base === "ru" || base === "en") return base;
  return null;
}

/**
 * Empty or missing language → Russian.
 * A Russian tag → Russian.
 * Any other tag, including retired codes such as de and fr → English.
 */
export function normalizeLocale(lang) {
  if (!lang || !String(lang).trim()) return "ru";
  return supportedLocale(lang) === "ru" ? "ru" : "en";
}

function defaultStorage() {
  try {
    if (typeof localStorage === "undefined") return null;
    return localStorage;
  } catch {
    return null;
  }
}

export function getStoredLocale(storage = defaultStorage()) {
  if (!storage) return null;
  try {
    return supportedLocale(storage.getItem(LOCALE_KEY));
  } catch {
    return null;
  }
}

export function setStoredLocale(locale, storage = defaultStorage()) {
  if (!storage) return;
  try {
    storage.setItem(LOCALE_KEY, locale);
  } catch {
    /* private mode or a blocked storage */
  }
}

function dropUnsupportedStoredLocale(storage) {
  if (!storage) return;
  try {
    const raw = storage.getItem(LOCALE_KEY);
    if (raw && !supportedLocale(raw)) storage.removeItem(LOCALE_KEY);
  } catch {
    /* ignore */
  }
}

/**
 * A supported value in localStorage wins, then a supported profile locale, then the browser.
 * A Russian browser opens Russian. Every other browser language opens English.
 * When there is no language signal, Russian is the default.
 * A saved code the console no longer supports is removed so later reads cannot render an empty catalog.
 */
export function detectInitialLocale(profileOrOptions) {
  let profile;
  let browser;
  let storage;

  if (profileOrOptions && typeof profileOrOptions === "object") {
    profile = profileOrOptions.profileLocale;
    browser = profileOrOptions.browserLanguage;
    storage = profileOrOptions.storage;
  } else {
    profile = profileOrOptions ?? null;
  }

  const store = storage === undefined ? defaultStorage() : storage;
  const browserLanguage =
    browser === undefined ? (typeof navigator !== "undefined" ? navigator.language : undefined) : browser;

  dropUnsupportedStoredLocale(store);
  const stored = getStoredLocale(store);
  if (stored) return stored;
  const fromProfile = supportedLocale(profile);
  if (fromProfile) return fromProfile;
  return normalizeLocale(browserLanguage);
}
