import { test, expect } from "@playwright/test";
import { loginAsAdmin } from "./helpers";

test("gateway page loads connections tab without error", async ({ page }) => {
  await loginAsAdmin(page);
  await page.goto("/gateway");

  await expect(page.locator("body")).toContainText(/gateway/i);
  await expect(page.getByRole("tab", { name: /connections|подключения/i })).toBeVisible();
  await expect(page.locator(".border-destructive\\/50")).toHaveCount(0);
  await expect(page.locator("body")).toContainText(
    /no gateway connections|add connection|нет подключений|добавить подключение/i
  );
});
