import { test, expect } from "@playwright/test";
import { loginAsAdmin } from "./helpers";

test("settings page loads bucket settings without error", async ({ page }) => {
  await loginAsAdmin(page);
  await page.goto("/admin/settings");

  await expect(page.getByRole("tab", { name: /bucket settings|настройки бакетов/i })).toBeVisible();
  await expect(page.getByRole("tab", { name: /administrator settings|настройки администратора/i })).toBeVisible();
  await expect(page.locator("body")).toContainText(
    /bucket settings|configure bucket|настройки бакетов|свойств бакетов/i
  );
  await expect(page.locator("body")).not.toContainText(/unable to load|не удалось загрузить/i);
});
