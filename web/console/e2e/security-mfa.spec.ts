import { test, expect } from "@playwright/test";

test("login smoke still works under security-hardened stack", async ({ page }) => {
  await page.goto("/login");
  await page.fill('input[name="username"], input[type="text"]', "admin");
  await page.fill('input[name="password"], input[type="password"]', "admin");
  await page.click('button[type="submit"]');
  await page.waitForURL((url) => !url.pathname.startsWith("/login"), { timeout: 30_000 });
  const token = await page.evaluate(() => sessionStorage.getItem("datasafe_admin_token"));
  expect(token).toBeTruthy();
});
