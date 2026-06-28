import { test, expect } from "@playwright/test";

test("login and buckets page smoke", async ({ page }) => {
  await page.goto("/login");
  await page.fill('input[name="username"], input[type="text"]', "admin");
  await page.fill('input[name="password"], input[type="password"]', "admin");
  await page.click('button[type="submit"]');
  await page.waitForURL((url) => !url.pathname.startsWith("/login"), { timeout: 30_000 });
  await page.goto("/buckets");
  await expect(page.locator("body")).toContainText(/bucket|бакет/i);
});
