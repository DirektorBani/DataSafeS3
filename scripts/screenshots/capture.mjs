#!/usr/bin/env node
/**
 * Seed DataSafeS3 demo data and capture console screenshots for docs.
 * Usage: npm install && npx playwright install chromium && npm run capture
 */
import { chromium } from "playwright";
import { mkdir } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const OUT_DIR = path.resolve(__dirname, "../../docs/images/screenshots");
const BASE = process.env.DATASAFE_URL || "http://localhost:8080";
const API = `${BASE}/api/v1`;
const ADMIN_USER = "admin";
const ADMIN_PASS_OLD = "admin";
const ADMIN_PASS = process.env.DATASAFE_ADMIN_PASS || "Admin123!";

async function api(method, path, token, body) {
  const headers = { "Content-Type": "application/json" };
  if (token) headers.Authorization = `Bearer ${token}`;
  const res = await fetch(`${API}${path}`, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  let json;
  try {
    json = text ? JSON.parse(text) : null;
  } catch {
    json = { raw: text };
  }
  if (!res.ok) {
    throw new Error(`${method} ${path} -> ${res.status}: ${text}`);
  }
  return json;
}

async function uploadObject(token, bucket, key, content) {
  const res = await fetch(`${API}/buckets/${bucket}/objects/${key}`, {
    method: "PUT",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "text/plain",
    },
    body: content,
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`upload ${bucket}/${key} -> ${res.status}: ${text}`);
  }
}

async function seed(token) {
  console.log("==> Seeding buckets, objects, tenants, users...");
  for (const name of ["documents", "backups", "media-assets"]) {
    try {
      await api("POST", `/buckets/${name}`, token, { visibility: "private" });
    } catch (e) {
      if (!String(e.message).includes("409")) throw e;
    }
  }
  await uploadObject(token, "documents", "readme.txt", "DataSafeS3 sample document for screenshots.");
  await uploadObject(token, "documents", "reports/q1-summary.pdf", "%PDF-1.4 sample");
  await uploadObject(token, "backups", "db-snapshot-2026-06-21.tar.gz", "backup placeholder");
  await uploadObject(token, "media-assets", "logo.png", "PNG placeholder");

  let tenant;
  try {
    tenant = await api("POST", "/tenants", token, { name: "Acme Corp" });
  } catch (e) {
    const list = await api("GET", "/tenants", token);
    tenant = { tenant: list.tenants?.find((t) => t.name === "Acme Corp") || list.tenants?.[0] };
  }
  const tenantId = tenant.tenant?.id || tenant.id;

  for (const u of [
    { username: "operator1", password: "Operator123!", role: "operator", email: "operator@acme.local" },
    { username: "alice", password: "User12345!", role: "user", email: "alice@acme.local" },
  ]) {
    try {
      await api("POST", "/users", token, u);
    } catch (e) {
      if (!String(e.message).includes("409")) throw e;
    }
  }

  if (tenantId) {
    try {
      await api("POST", `/tenants/${tenantId}/members`, token, {
        username: "alice",
        role: "member",
      });
    } catch {
      /* may already exist */
    }
  }
  console.log("    Seed complete.");
}

async function setupAdmin() {
  console.log("==> Completing initial setup via API...");
  const login = await api("POST", "/admin/login", null, {
    username: ADMIN_USER,
    password: ADMIN_PASS_OLD,
  });
  const token = login.token;

  await api("POST", "/me/password", token, {
    current_password: ADMIN_PASS_OLD,
    new_password: ADMIN_PASS,
  });

  const status = await api("GET", "/setup/status", null);
  if (status.needs_setup) {
    await api("POST", "/setup/complete", token, {});
  }

  await seed(token);
  return ADMIN_PASS;
}

async function screenshot(page, name) {
  const file = path.join(OUT_DIR, `${name}.png`);
  await page.screenshot({ path: file, fullPage: false });
  console.log(`    Saved ${file}`);
  return file;
}

async function loginUI(page, password) {
  await page.goto(`${BASE}/login`, { waitUntil: "networkidle" });
  await page.fill('input[name="username"], input#username', ADMIN_USER);
  await page.fill('input[name="password"], input#password', password);
  await page.click('form button[type="submit"], button[type="submit"]');
  await page.waitForURL((url) => !url.pathname.includes("/login"), { timeout: 15000 });
}

async function captureConsole(password) {
  await mkdir(OUT_DIR, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: 1280, height: 800 },
    locale: "ru-RU",
  });
  const page = await context.newPage();

  console.log("==> Capturing console screenshots...");
  await loginUI(page, password);
  await page.waitForTimeout(1500);

  const shots = [];

  await page.goto(`${BASE}/`, { waitUntil: "networkidle" });
  await page.waitForTimeout(1000);
  shots.push(await screenshot(page, "dashboard"));

  await page.goto(`${BASE}/buckets`, { waitUntil: "networkidle" });
  await page.waitForTimeout(1000);
  shots.push(await screenshot(page, "buckets"));

  await page.goto(`${BASE}/buckets/documents`, { waitUntil: "networkidle" });
  await page.waitForTimeout(1000);
  shots.push(await screenshot(page, "object-browser"));

  await page.goto(`${BASE}/admin/tenants`, { waitUntil: "networkidle" });
  await page.waitForTimeout(1000);
  shots.push(await screenshot(page, "tenants"));

  await page.goto(`${BASE}/admin/users`, { waitUntil: "networkidle" });
  await page.waitForTimeout(1000);
  shots.push(await screenshot(page, "users"));

  await page.goto(`${BASE}/admin/settings/system`, { waitUntil: "networkidle" });
  await page.waitForTimeout(1000);
  shots.push(await screenshot(page, "settings"));

  await page.goto(`${BASE}/gateway`, { waitUntil: "networkidle" });
  await page.waitForTimeout(1000);
  shots.push(await screenshot(page, "gateway"));

  await page.goto(`${BASE}/admin/activity`, { waitUntil: "networkidle" });
  await page.waitForTimeout(1000);
  shots.push(await screenshot(page, "activity"));

  // Grafana dashboard
  const grafana = await context.newPage();
  await grafana.setViewportSize({ width: 1280, height: 800 });
  try {
    await grafana.goto("http://localhost:3000/login", { waitUntil: "networkidle", timeout: 10000 });
    await grafana.fill('input[name="user"]', "admin");
    await grafana.fill('input[name="password"]', "admin");
    await grafana.click('button[type="submit"]');
    await grafana.waitForTimeout(2000);
    await grafana.goto("http://localhost:3000/dashboards", { waitUntil: "networkidle" });
    await grafana.waitForTimeout(1500);
    const grafanaFile = path.join(OUT_DIR, "monitoring.png");
    await grafana.screenshot({ path: grafanaFile, fullPage: false });
    console.log(`    Saved ${grafanaFile}`);
    shots.push(grafanaFile);
  } catch (e) {
    console.warn("    Grafana screenshot skipped:", e.message);
  }

  await browser.close();
  return shots;
}

async function main() {
  const password = await setupAdmin();
  const shots = await captureConsole(password);
  console.log("\n==> Screenshots captured:");
  for (const s of shots) console.log(`  - ${s}`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
