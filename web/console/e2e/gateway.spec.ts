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

test("gateway shows public-read policy warning when health reports rules", async ({ page }) => {
  await page.route("**/api/v1/gateway/health", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        status: "ok",
        pending_tasks: 0,
        failed_tasks: 0,
        public_read_rules: 2,
      }),
    });
  });

  await loginAsAdmin(page);
  await page.goto("/gateway");

  await expect(page.locator("body")).toContainText(
    /public-read|public read|публичн/i
  );
});
