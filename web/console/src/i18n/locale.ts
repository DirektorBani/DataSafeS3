// @ts-expect-error locale.mjs is plain JavaScript so Node 20 CI can load it.
import { detectInitialLocale as detect, getStoredLocale as readStored, LOCALE_KEY as localeKey, normalizeLocale as normalize, setStoredLocale as writeStored, SUPPORTED_LOCALES as supportedLocales, supportedLocale as supported } from "./locale.mjs";

export const LOCALE_KEY = localeKey as "datasafe.locale";
export const SUPPORTED_LOCALES = supportedLocales as readonly ["ru", "en"];
export type AppLocale = (typeof SUPPORTED_LOCALES)[number];

export type LocaleStorage = {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
};

export type LocaleDetection = {
  profileLocale?: string | null;
  browserLanguage?: string | null;
  storage?: LocaleStorage | null;
};

export function supportedLocale(lang: string | undefined | null): AppLocale | null {
  return supported(lang);
}

export function normalizeLocale(lang: string | undefined | null): AppLocale {
  return normalize(lang);
}

export function getStoredLocale(storage?: LocaleStorage | null): AppLocale | null {
  return readStored(storage);
}

export function setStoredLocale(locale: AppLocale, storage?: LocaleStorage | null) {
  writeStored(locale, storage);
}

export function detectInitialLocale(profileOrOptions?: string | null | LocaleDetection): AppLocale {
  return detect(profileOrOptions);
}
