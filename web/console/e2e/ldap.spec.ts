import { test, expect } from "@playwright/test";
import { login, loginAsAdmin } from "./helpers";

const LDAP_USER = process.env.E2E_LDAP_USER ?? "ldapuser";
const LDAP_PASS = process.env.E2E_LDAP_PASSWORD ?? "password";

test("administrator settings shows LDAP configuration", async ({ page }) => {
  await loginAsAdmin(page);
  await page.goto("/admin/settings/system");
  await expect(page.locator("body")).toContainText(/LDAP|Active Directory/i);
  await expect(page.getByRole("button", { name: /test connection|проверить соединение/i })).toBeVisible();
});

test("LDAP user can sign in via console login form", async ({ page, request, baseURL }) => {
  const probe = await request.post(`${baseURL}/api/v1/admin/login`, {
    data: { username: LDAP_USER, password: LDAP_PASS },
  });
  if (!probe.ok()) {
    test.skip(true, `LDAP login not available (${probe.status()}) — start scripts/start-ldap-test.cmd`);
  }

  await login(page, LDAP_USER, LDAP_PASS);
  await expect(page).not.toHaveURL(/\/login/);
  await page.goto("/buckets");
  await expect(page.locator("body")).toContainText(/bucket|бакет|files|файл/i);
});
