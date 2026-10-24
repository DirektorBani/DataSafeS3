import assert from "node:assert/strict";
import test from "node:test";
import { detectInitialLocale, LOCALE_KEY, normalizeLocale } from "../src/i18n/locale.mjs";

function memoryStorage(initial = {}) {
  const data = { ...initial };
  return {
    getItem(key) {
      return Object.prototype.hasOwnProperty.call(data, key) ? data[key] : null;
    },
    setItem(key, value) {
      data[key] = value;
    },
    removeItem(key) {
      delete data[key];
    },
  };
}

test("normalizeLocale prefers Russian and falls back to English", () => {
  assert.equal(normalizeLocale(undefined), "ru");
  assert.equal(normalizeLocale(null), "ru");
  assert.equal(normalizeLocale(""), "ru");
  assert.equal(normalizeLocale("ru"), "ru");
  assert.equal(normalizeLocale("ru-RU"), "ru");
  assert.equal(normalizeLocale("RU"), "ru");
  assert.equal(normalizeLocale("en"), "en");
  assert.equal(normalizeLocale("en-US"), "en");
  assert.equal(normalizeLocale("de"), "en");
  assert.equal(normalizeLocale("fr-FR"), "en");
  assert.equal(normalizeLocale("zh-CN"), "en");
});

test("saved Russian or English wins over the browser", () => {
  const storage = memoryStorage({ [LOCALE_KEY]: "en" });
  assert.equal(detectInitialLocale({ profileLocale: "ru", browserLanguage: "ru-RU", storage }), "en");
  storage.setItem(LOCALE_KEY, "ru-RU");
  assert.equal(detectInitialLocale({ profileLocale: "en", browserLanguage: "de", storage }), "ru");
});

test("retired saved locale is dropped and the browser decides", () => {
  const storage = memoryStorage({ [LOCALE_KEY]: "de" });
  assert.equal(detectInitialLocale({ browserLanguage: "ru", storage }), "ru");
  assert.equal(storage.getItem(LOCALE_KEY), null);

  storage.setItem(LOCALE_KEY, "fr");
  assert.equal(detectInitialLocale({ browserLanguage: "de-DE", storage }), "en");
  assert.equal(storage.getItem(LOCALE_KEY), null);
});

test("a supported profile locale is used when nothing valid is stored", () => {
  const storage = memoryStorage({ [LOCALE_KEY]: "es" });
  assert.equal(detectInitialLocale({ profileLocale: "en", browserLanguage: "ru", storage }), "en");
  assert.equal(storage.getItem(LOCALE_KEY), null);
});

test("an unsupported profile locale follows the browser", () => {
  const storage = memoryStorage();
  assert.equal(detectInitialLocale({ profileLocale: "de", browserLanguage: "ru-RU", storage }), "ru");
  assert.equal(detectInitialLocale({ profileLocale: "fr", browserLanguage: "pt-BR", storage }), "en");
  assert.equal(detectInitialLocale({ profileLocale: null, browserLanguage: null, storage }), "ru");
});
