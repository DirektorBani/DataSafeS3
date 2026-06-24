/**
 * Capture real UI screenshots for docs/user-guide using Playwright.
 * Usage: node scripts/capture-screenshots.mjs
 */
import { chromium } from "playwright";
import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(__dirname, "..");
const OUT_DIR = path.join(ROOT, "docs", "user-guide", "images");

const CONSOLE_URL = process.env.CONSOLE_URL || "http://localhost:8080";
const GRAFANA_URL = process.env.GRAFANA_URL || "http://localhost:3000";
const USER = process.env.STORAGE_ADMIN_USER || "admin";
const PASS = process.env.STORAGE_ADMIN_PASSWORD || "admin";
const DEMO_BUCKET = "user-guide-demo";
const VIEWPORT = { width: 1280, height: 720 };

const captured = [];
const failed = [];

async function shot(page, name, opts = {}) {
  const filePath = path.join(OUT_DIR, name);
  await page.screenshot({ path: filePath, fullPage: false, ...opts });
  captured.push(filePath);
  console.log(`  ✓ ${name}`);
  return filePath;
}

async function loginViaApi(request) {
  const res = await request.post(`${CONSOLE_URL}/api/v1/admin/login`, {
    data: { username: USER, password: PASS },
  });
  if (!res.ok()) {
    throw new Error(`Login API failed: ${res.status()} ${await res.text()}`);
  }
  const data = await res.json();
  if (data.mfa_required) {
    throw new Error("Admin account has MFA enabled; disable MFA or use recovery code for capture.");
  }
  return data;
}

async function seedSession(page, login) {
  await page.goto(`${CONSOLE_URL}/login`, { waitUntil: "domcontentloaded" });
  await page.evaluate(
    ({ token, username, role }) => {
      sessionStorage.setItem("datasafe_admin_token", token);
      sessionStorage.setItem("datasafe_admin_user", username);
      sessionStorage.setItem("datasafe_admin_role", role);
      sessionStorage.removeItem("datasafe_tenant_memberships");
      sessionStorage.removeItem("datasafe_is_tenant_admin");
    },
    {
      token: login.token,
      username: login.username || USER,
      role: login.role || "administrator",
    }
  );
}

async function loginInBrowser(page, login) {
  await seedSession(page, login);
  await page.goto(`${CONSOLE_URL}/`, { waitUntil: "networkidle" });
  await page.waitForURL((url) => !url.pathname.includes("/login"), { timeout: 15000 });
  await page.waitForLoadState("networkidle");
}

async function ensureDemoBucket(request, token) {
  const headers = { Authorization: `Bearer ${token}` };
  const list = await request.get(`${CONSOLE_URL}/api/v1/buckets`, { headers });
  if (!list.ok()) throw new Error(`List buckets failed: ${list.status()}`);

  const { buckets = [] } = await list.json();
  if (!buckets.some((b) => b.name === DEMO_BUCKET)) {
    const create = await request.post(
      `${CONSOLE_URL}/api/v1/buckets/${encodeURIComponent(DEMO_BUCKET)}`,
      { headers }
    );
    if (!create.ok() && create.status() !== 409) {
      throw new Error(`Create bucket failed: ${create.status()} ${await create.text()}`);
    }
  }

  const sample = Buffer.from(
    "DataSafeS3 user guide demo file.\nGenerated for documentation screenshots.\n",
    "utf8"
  );
  const upload = await request.put(
    `${CONSOLE_URL}/api/v1/buckets/${encodeURIComponent(DEMO_BUCKET)}/objects/${encodeURIComponent("sample.txt")}`,
    {
      headers: { ...headers, "Content-Type": "text/plain" },
      data: sample,
    }
  );
  if (!upload.ok()) {
    throw new Error(`Upload object failed: ${upload.status()} ${await upload.text()}`);
  }
}

async function captureGrafana(browser) {
  const context = await browser.newContext({ viewport: VIEWPORT });
  const page = await context.newPage();
  try {
    await page.goto(GRAFANA_URL, { waitUntil: "domcontentloaded", timeout: 15000 });
    const userField = page
      .locator(
        'input[name="user"], input[name="username"], input[placeholder*="email" i], input[placeholder*="username" i]'
      )
      .first();
    await userField.waitFor({ state: "visible", timeout: 10000 });
    await userField.fill("admin");
    const passField = page.locator('input[name="password"], input[type="password"]').first();
    await passField.fill("admin");
    await page.getByRole("button", { name: /log in|sign in/i }).click();
    await page.waitForLoadState("networkidle");

    await page.goto(`${GRAFANA_URL}/d/datasafe-overview/datasafe-overview`, {
      waitUntil: "networkidle",
      timeout: 30000,
    });
    await page.waitForTimeout(2000);
    await shot(page, "grafana.png");
  } catch (err) {
    failed.push({ name: "grafana.png", error: err.message });
    console.warn(`  ✗ grafana.png: ${err.message}`);
  } finally {
    await context.close();
  }
}

async function main() {
  await mkdir(OUT_DIR, { recursive: true });

  console.log(`Console: ${CONSOLE_URL}`);
  console.log(`Output:  ${OUT_DIR}\n`);

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: VIEWPORT });
  const page = await context.newPage();
  const request = context.request;

  try {
    await page.goto(`${CONSOLE_URL}/login`, { waitUntil: "networkidle" });
    await shot(page, "login.png");

    const login = await loginViaApi(request);
    await ensureDemoBucket(request, login.token);
    await loginInBrowser(page, login);

    await page.goto(`${CONSOLE_URL}/`, { waitUntil: "networkidle" });
    await page.getByRole("heading", { name: /dashboard/i }).waitFor({ timeout: 10000 }).catch(() => {});
    await page.waitForTimeout(500);
    await shot(page, "dashboard.png");

    await page.goto(`${CONSOLE_URL}/buckets`, { waitUntil: "networkidle" });
    await page.getByRole("heading", { name: /buckets|files|бакеты|файлы/i }).waitFor({ timeout: 10000 });
    await page.waitForTimeout(500);
    await shot(page, "buckets.png");

    await page.goto(`${CONSOLE_URL}/buckets/${encodeURIComponent(DEMO_BUCKET)}`, {
      waitUntil: "networkidle",
    });
    await page.getByText("sample.txt").waitFor({ timeout: 15000 });
    await page.waitForTimeout(500);
    await shot(page, "bucket-detail.png");

    await page.goto(`${CONSOLE_URL}/gateway`, { waitUntil: "networkidle" });
    await page.getByRole("tab", { name: /connections|подключения/i }).waitFor({ timeout: 10000 });
    await page.waitForTimeout(500);
    await shot(page, "gateway.png");

    await page.goto(`${CONSOLE_URL}/profile`, { waitUntil: "networkidle" });
    await page.getByRole("heading", { name: /profile|профиль/i }).waitFor({ timeout: 10000 });
    await page.getByText(/MFA|two-factor|authenticator|двухфактор/i).first().waitFor({ timeout: 10000 });
    await page.waitForTimeout(500);
    await shot(page, "mfa-profile.png");
  } catch (err) {
    console.error("\nFatal:", err.message);
    failed.push({ name: "main flow", error: err.message });
  } finally {
    await context.close();
  }

  await captureGrafana(browser);
  await browser.close();

  const summaryPath = path.join(OUT_DIR, "capture-summary.json");
  await writeFile(
    summaryPath,
    JSON.stringify({ captured, failed, timestamp: new Date().toISOString() }, null, 2)
  );

  console.log("\n--- Summary ---");
  console.log(`Captured: ${captured.length}`);
  captured.forEach((p) => console.log(`  ${p}`));
  if (failed.length) {
    console.log(`Failed: ${failed.length}`);
    failed.forEach((f) => console.log(`  ${f.name}: ${f.error}`));
  }
  process.exit(failed.length && captured.length === 0 ? 1 : 0);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
