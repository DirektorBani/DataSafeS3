import { test, expect } from "@playwright/test";

const keycloakURL = process.env.KEYCLOAK_URL ?? "http://127.0.0.1:8180";

test.describe("OIDC Keycloak browser flow @nightly", () => {
  test.beforeEach(async ({ request }) => {
    try {
      const res = await request.get(`${keycloakURL}/realms/datasafe`);
      if (!res.ok()) {
        test.skip(true, "Keycloak test realm not reachable");
      }
    } catch {
      test.skip(true, "Keycloak test realm not reachable");
    }
  });

  test("AUD-14 real SSO redirect stores session via exchange_code", async ({ page }) => {
    test.skip(
      process.env.E2E_OIDC_KEYCLOAK !== "1",
      "Set E2E_OIDC_KEYCLOAK=1 with OIDC enabled on storage-server and Keycloak running"
    );

    await page.goto("/login");
    const sso = page.getByRole("button", { name: /sso|oidc|keycloak|войти через/i });
    await sso.click();

    await page.waitForURL(/8180|keycloak|realms\/datasafe/i, { timeout: 30_000 });
    await page.fill("#username", "ssouser");
    await page.fill("#password", "password");
    await page.click("#kc-login");

    await page.waitForURL((url) => !url.searchParams.has("exchange_code") === false || !url.pathname.startsWith("/login"), {
      timeout: 45_000,
    });

    const token = await page.evaluate(() => sessionStorage.getItem("datasafe_admin_token"));
    expect(token).toBeTruthy();
    expect(page.url()).not.toContain("exchange_code=");
  });
});
