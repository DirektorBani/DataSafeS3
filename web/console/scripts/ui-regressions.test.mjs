// Regression checks for UI defects found on a full installation (v1.7.0).
// They read the sources, so no browser or running stack is needed.
import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const src = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../src");
const read = (p) => fs.readFileSync(path.join(src, p), "utf8").replace(/^\uFEFF/, "");

function* walk(dir) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, e.name);
    if (e.isDirectory()) yield* walk(full);
    else yield full;
  }
}

test("login form is not prefilled with the default credentials", () => {
  const login = read("pages/login.tsx");
  assert.doesNotMatch(login, /defaultValues:\s*\{[^}]*["']admin["']/);
});

test("theme button label uses the localised theme name", () => {
  const sidebar = read("layouts/sidebar.tsx");
  assert.doesNotMatch(sidebar, /theme\.label",\s*\{\s*theme\s*\}\s*\)/);
  assert.match(sidebar, /theme\.label",\s*\{\s*theme:\s*t\(/);
});

test("no doubled words such as 'S3 S3' in locale strings", () => {
  for (const file of walk(path.join(src, "locales"))) {
    if (!file.endsWith(".json")) continue;
    const text = fs.readFileSync(file, "utf8");
    assert.doesNotMatch(text, /\bS3 S3\b/, file);
    assert.doesNotMatch(text, /external S3 (S3|Gateway)/i, file);
  }
});

test("language switcher offers only ru and en", () => {
  const sw = read("components/language-switcher.tsx");
  const codes = [...sw.matchAll(/code:\s*"([a-z]{2})"/g)].map((m) => m[1]);
  assert.deepEqual(codes, ["ru", "en"]);
});
