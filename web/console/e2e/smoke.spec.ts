import { test, expect } from "@playwright/test";

test("language switcher is Russian and English", async ({ page }) => {
  await page.goto("/login");
  const group = page.getByRole("group", { name: /language|язык/i });
  await expect(group.getByRole("button", { name: "RU" })).toBeVisible();
  await expect(group.getByRole("button", { name: "EN" })).toBeVisible();
  await expect(group.getByRole("button", { name: "DE" })).toHaveCount(0);
  await expect(group.getByRole("button", { name: "FR" })).toHaveCount(0);
  await expect(page.getByText("Sign in to manage buckets, objects, and access keys.")).toBeVisible();
});

test("a saved French locale falls back to English", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("datasafe.locale", "fr"));
  await page.goto("/login");
  await expect(page.getByRole("button", { name: "EN" })).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByText("Sign in to manage buckets, objects, and access keys.")).toBeVisible();
  await expect.poll(() => page.evaluate(() => localStorage.getItem("datasafe.locale"))).toBeNull();
});

test.describe("russian browser", () => {
  test.use({ locale: "ru-RU" });

  test("opens in Russian when the browser is Russian", async ({ page }) => {
    await page.goto("/login");
    await expect(page.getByRole("button", { name: "RU" })).toHaveAttribute("aria-pressed", "true");
    await expect(page.getByText("Войдите для управления бакетами, объектами и ключами доступа.")).toBeVisible();
  });
});

test("login and buckets page smoke", async ({ page }) => {
  await page.goto("/login");
  await page.fill('input[name="username"], input[type="text"]', "admin");
  await page.fill('input[name="password"], input[type="password"]', "admin");
  await page.click('button[type="submit"]');
  await page.waitForURL((url) => !url.pathname.startsWith("/login"), { timeout: 30_000 });
  await page.goto("/buckets");
  await expect(page.locator("body")).toContainText(/bucket|бакет/i);
});
