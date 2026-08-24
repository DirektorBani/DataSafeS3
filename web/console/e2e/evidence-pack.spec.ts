import { test, expect } from "@playwright/test";
import { e2eSuffix, loginAsAdmin, adminToken, createBucket } from "./helpers";

/**
 * Smoke: Activity export + Storage inventory UI surfaces.
 * Requires live console (PLAYWRIGHT_BASE_URL, default :8080) with admin/admin.
 */
test.describe("governance evidence pack", () => {
  test("activity page shows export buttons", async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto("/activity");
    await expect(page.getByRole("button", { name: /export csv|экспорт csv/i })).toBeVisible({
      timeout: 20_000,
    });
    await expect(page.getByRole("button", { name: /export json|экспорт json/i })).toBeVisible();
  });

  test("bucket settings shows storage inventory export", async ({ page, request, baseURL }) => {
    const root = baseURL ?? "http://127.0.0.1:8080";
    const tok = await adminToken(request, root);
    const bucket = `e2e-inv-${e2eSuffix()}`;
    await createBucket(request, root, tok, bucket);

    await loginAsAdmin(page);
    await page.goto(`/buckets/${encodeURIComponent(bucket)}`);
    await page.getByRole("tab", { name: /settings|настройки/i }).click();
    await expect(page.getByText(/storage inventory|состав хранения/i)).toBeVisible({
      timeout: 20_000,
    });
    await expect(page.getByRole("button", { name: /export csv|экспорт csv/i })).toBeVisible();
  });
});
