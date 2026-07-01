import { test, expect } from "@playwright/test";
import { adminToken, loginAsAdmin } from "./helpers";

test("settings page loads bucket settings without error", async ({ page }) => {
  await loginAsAdmin(page);
  await page.goto("/admin/settings");

  await expect(page.getByRole("tab", { name: /bucket settings|настройки бакетов/i })).toBeVisible();
  await expect(page.getByRole("tab", { name: /administrator settings|настройки администратора/i })).toBeVisible();
  await expect(page.getByRole("tab", { name: /security posture|безопасность/i })).toBeVisible();
  await expect(page.locator("body")).toContainText(
    /bucket settings|configure bucket|настройки бакетов|свойств бакетов/i
  );
  await expect(page.locator("body")).not.toContainText(/unable to load|не удалось загрузить/i);
});

test("security posture tab shows status panel", async ({ page }) => {
  await loginAsAdmin(page);
  await page.goto("/admin/settings/security");
  await expect(page.locator("body")).toContainText(
    /security status|security posture|состояние безопасности|безопасность/i
  );
});

test("security-status API includes field_encryption block", async ({ request, baseURL }) => {
  const token = await adminToken(request, baseURL!);
  const res = await request.get(`${baseURL}/api/v1/settings/security-status`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(res.ok()).toBeTruthy();
  const body = (await res.json()) as {
    field_encryption?: { enabled?: boolean; registry_count?: number };
  };
  expect(body.field_encryption).toBeDefined();
  expect(typeof body.field_encryption?.enabled).toBe("boolean");
  expect(typeof body.field_encryption?.registry_count).toBe("number");
});
