import { test, expect } from "@playwright/test";

test("OIDC exchange code login clears URL and stores session", async ({ page }) => {
  const mockToken = "eyJhbGciOiJIUzI1NiJ9.security-oidc-e2e.mock";

  await page.route("**/api/v1/auth/oidc/exchange", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ token: mockToken, auth_source: "oidc" }),
    });
  });

  await page.route("**/api/v1/me", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        username: "oidc-user",
        role: "admin",
        auth_source: "oidc",
      }),
    });
  });

  await page.route("**/api/v1/setup/status", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        admin_first_login_completed: true,
        initial_setup_completed: true,
      }),
    });
  });

  await page.goto("/login?exchange_code=test-exchange-code&auth_source=oidc");
  await page.waitForURL((url) => !url.searchParams.has("exchange_code"), { timeout: 15_000 });

  const token = await page.evaluate(() => sessionStorage.getItem("datasafe_admin_token"));
  expect(token).toBe(mockToken);
  expect(page.url()).not.toContain("exchange_code=");
});
