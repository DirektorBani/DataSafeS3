import fs from "fs";
import path from "path";

const srcDir = "src";

function walk(d, acc = []) {
  for (const f of fs.readdirSync(d)) {
    const p = path.join(d, f);
    if (fs.statSync(p).isDirectory()) walk(p, acc);
    else if (/\.(tsx?|jsx?)$/.test(f)) acc.push(p);
  }
  return acc;
}

function flatten(obj, prefix = "") {
  const out = {};
  for (const [k, v] of Object.entries(obj)) {
    const nk = prefix ? `${prefix}.${k}` : k;
    if (v && typeof v === "object" && !Array.isArray(v)) Object.assign(out, flatten(v, nk));
    else out[nk] = v;
  }
  return out;
}

function parseDefaultNamespaces(content) {
  const m = content.match(/useTranslation\(\s*\[?\s*["']([^"']+)["']/);
  if (m) return [m[1]];
  const m2 = content.match(/useTranslation\(\s*["']([^"']+)["']/);
  if (m2) return [m2[1]];
  return ["common"];
}

function resolveKey(key, defaultNs) {
  if (key.includes(":")) {
    const [ns, ...rest] = key.split(":");
    return { ns, actualKey: rest.join(":") };
  }
  return { ns: defaultNs, actualKey: key };
}

const files = walk(srcDir).filter((f) => !f.includes("locales") && !f.includes("audit-i18n"));
const keyRe = /\bt\(\s*["']([^"']+)["']/g;
const keys = new Map();

for (const f of files) {
  const c = fs.readFileSync(f, "utf8");
  const defaultNs = parseDefaultNamespaces(c)[0];
  let m;
  while ((m = keyRe.exec(c))) {
    const raw = m[1];
    const resolved = resolveKey(raw, defaultNs);
    const full = `${resolved.ns}:${resolved.actualKey}`;
    if (!keys.has(full)) keys.set(full, []);
    keys.get(full).push(f.replace(/\\/g, "/"));
  }
}

const locales = { en: {}, ru: {} };
for (const lang of ["en", "ru"]) {
  const dir = path.join("src/locales", lang);
  for (const f of fs.readdirSync(dir)) {
    if (!f.endsWith(".json")) continue;
    const ns = f.replace(".json", "");
    const raw = fs.readFileSync(path.join(dir, f), "utf8");
    const data = JSON.parse(raw);
    locales[lang][ns] = flatten(data);
    const topKeys = [...raw.matchAll(/^\s*"([^"]+)"\s*:/gm)].map((x) => x[1]);
    const dupes = topKeys.filter((k, i) => topKeys.indexOf(k) !== i);
    if (dupes.length) {
      console.log(`WARN duplicate top-level keys in ${lang}/${f}:`, [...new Set(dupes)].join(", "));
    }
  }
}

const missing = { en: [], ru: [] };
for (const [full, keyFiles] of keys) {
  const [ns, ...rest] = full.split(":");
  const actualKey = rest.join(":");
  for (const lang of ["en", "ru"]) {
    if (!locales[lang][ns] || !(actualKey in locales[lang][ns])) {
      missing[lang].push({ key: full, files: [...new Set(keyFiles)].slice(0, 2) });
    }
  }
}

console.log("Total resolved t() keys:", keys.size);
console.log("Missing EN:", missing.en.length);
missing.en.forEach((x) => console.log("  EN", x.key, "<-", x.files[0]));
console.log("Missing RU:", missing.ru.length);
missing.ru.forEach((x) => console.log("  RU", x.key, "<-", x.files[0]));
