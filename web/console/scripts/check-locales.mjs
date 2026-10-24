/**
 * Fails when the web console locale set is anything other than Russian and English
 * with the same files, the same keys, non-empty strings, and the same placeholders.
 * Wired into `npm run build`, which the web-console CI job runs.
 */
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../src/locales");
const allowed = ["ru", "en"];
const errors = [];

function readJSON(file) {
  let raw = fs.readFileSync(file);
  if (raw[0] === 0xef && raw[1] === 0xbb && raw[2] === 0xbf) raw = raw.subarray(3);
  return JSON.parse(raw.toString("utf8"));
}

function flatten(value, prefix, out) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    errors.push(`${prefix || "(root)"}: locale files must be objects of strings`);
    return;
  }
  for (const [key, child] of Object.entries(value)) {
    const full = prefix ? `${prefix}.${key}` : key;
    if (child && typeof child === "object" && !Array.isArray(child)) flatten(child, full, out);
    else if (typeof child === "string") out[full] = child;
    else errors.push(`${full}: value must be a string`);
  }
}

function placeholders(text) {
  return [...text.matchAll(/\{\{\s*([^{}]+?)\s*\}\}/g)].map((m) => m[1].trim()).sort();
}

if (!fs.existsSync(root)) {
  console.error(`locale directory missing: ${root}`);
  process.exit(1);
}

const dirs = fs.readdirSync(root, { withFileTypes: true });
for (const entry of dirs) {
  if (!entry.isDirectory() || !allowed.includes(entry.name)) {
    errors.push(`unexpected locale entry ${entry.name}; only ru and en are allowed`);
  }
}
for (const lang of allowed) {
  if (!dirs.some((entry) => entry.isDirectory() && entry.name === lang)) {
    errors.push(`missing locale directory ${lang}`);
  }
}

const filesByLang = {};
for (const lang of allowed) {
  const dir = path.join(root, lang);
  if (!fs.existsSync(dir)) continue;
  filesByLang[lang] = fs
    .readdirSync(dir)
    .filter((name) => name.endsWith(".json"))
    .sort();
  for (const name of fs.readdirSync(dir)) {
    if (!name.endsWith(".json")) errors.push(`${lang}/${name}: locale directories contain JSON only`);
  }
}

if (filesByLang.ru && filesByLang.en) {
  const ruSet = new Set(filesByLang.ru);
  const enSet = new Set(filesByLang.en);
  for (const name of filesByLang.en) if (!ruSet.has(name)) errors.push(`ru is missing ${name}`);
  for (const name of filesByLang.ru) if (!enSet.has(name)) errors.push(`en is missing ${name}`);

  for (const name of filesByLang.en) {
    if (!ruSet.has(name)) continue;
    const enFlat = {};
    const ruFlat = {};
    flatten(readJSON(path.join(root, "en", name)), "", enFlat);
    flatten(readJSON(path.join(root, "ru", name)), "", ruFlat);
    const enKeys = Object.keys(enFlat).sort();
    const ruKeys = Object.keys(ruFlat).sort();
    for (const key of enKeys) {
      if (!(key in ruFlat)) errors.push(`ru/${name} missing key ${key}`);
    }
    for (const key of ruKeys) {
      if (!(key in enFlat)) errors.push(`en/${name} missing key ${key}`);
    }
    for (const key of enKeys) {
      if (!(key in ruFlat)) continue;
      if (!enFlat[key].trim()) errors.push(`en/${name} ${key} is empty`);
      if (!ruFlat[key].trim()) errors.push(`ru/${name} ${key} is empty`);
      const enPh = placeholders(enFlat[key]).join("|");
      const ruPh = placeholders(ruFlat[key]).join("|");
      if (enPh !== ruPh) {
        errors.push(`placeholder mismatch ${name} ${key}: en [${enPh}] ru [${ruPh}]`);
      }
    }
  }
}

if (errors.length) {
  console.error(`locale check failed (${errors.length}):`);
  for (const error of errors) console.error(`  ${error}`);
  process.exit(1);
}

const count = filesByLang.en?.length ?? 0;
console.log(`locale check ok: ru and en, ${count} files, keys match`);
