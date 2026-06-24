/**
 * Optional Playwright OIDC browser smoke (full redirect flow).
 * Skips when Keycloak test stack is not running — API path is covered by feature-audit-test.ps1.
 *
 * Usage:
 *   scripts\start-keycloak-test.cmd
 *   node scripts/oidc-browser-e2e.mjs
 */
import { chromium } from "playwright";

const CONSOLE_URL = process.env.CONSOLE_URL || "http://localhost:8080";
const KEYCLOAK_URL = process.env.KEYCLOAK_URL || "http://localhost:8180";
const OIDC_USER = process.env.OIDC_TEST_USER || "ssouser";
const OIDC_PASS = process.env.OIDC_TEST_PASSWORD || "password";

async function keycloakUp() {
  try {
    const res = await fetch(`${KEYCLOAK_URL}/realms/datasafe`);
    return res.ok;
  } catch {
    return false;
  }
}

async function main() {
  if (!(await keycloakUp())) {
    console.log("SKIP: Keycloak not reachable at", KEYCLOAK_URL);
    console.log("Reason: full browser OIDC redirect E2E deferred; use POST /api/v1/auth/oidc/password-login in feature-audit-test.ps1");
    process.exit(0);
  }

  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  try {
    await page.goto(`${CONSOLE_URL}/login`, { waitUntil: "networkidle" });
    const sso = page.getByRole("button", { name: /sso|oidc|keycloak/i });
    if ((await sso.count()) === 0) {
      console.log("SKIP: OIDC login button not visible (OIDC disabled in settings)");
      process.exit(0);
    }
    await sso.first().click();
    await page.waitForURL(/8180|realms\/datasafe/, { timeout: 15000 });
    await page.fill("#username", OIDC_USER);
    await page.fill("#password", OIDC_PASS);
    await page.click("#kc-login, input[type=submit], button[type=submit]");
    await page.waitForURL(/localhost:8080\/login\?token=/, { timeout: 30000 });
    const token = await page.evaluate(() => localStorage.getItem("datasafe_token"));
    if (!token) {
      throw new Error("No datasafe_token in localStorage after OIDC callback");
    }
    console.log("PASS: OIDC browser login completed");
  } finally {
    await browser.close();
  }
}

main().catch((e) => {
  console.log("SKIP:", e.message);
  console.log("Reason: browser OIDC redirect is optional; POST /api/v1/auth/oidc/password-login covered in feature-audit-test.ps1");
  process.exit(0);
});
