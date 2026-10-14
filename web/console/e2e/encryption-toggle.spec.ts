import { test, expect } from "@playwright/test";
import { adminToken, createBucket, e2eSuffix, loginAsAdmin } from "./helpers";

test("bucket encryption card toggles and list shows a badge", async ({ page, request, baseURL }) => {
  const bucket = `e2e-enc-${e2eSuffix()}`;
  const tok = await adminToken(request, baseURL!);
  await createBucket(request, baseURL!, tok, bucket);

  await loginAsAdmin(page);
  await page.goto(`/buckets/${encodeURIComponent(bucket)}?tab=settings`);
  const card = page.getByTestId("encryption-card");
  await expect(card).toBeVisible({ timeout: 15_000 });

  const toggle = page.getByTestId("encryption-toggle");
  if (await toggle.isDisabled()) {
    await expect(card.getByText(/STORAGE_SSE_MASTER_KEY/)).toBeVisible();
    return;
  }

  await toggle.check();
  await page.getByTestId("encryption-save").click();
  await expect(page.getByText(/encryption saved|шифрование сохранено/i)).toBeVisible({ timeout: 10_000 });

  const listed = await request.get(`${baseURL}/api/v1/buckets/${encodeURIComponent(bucket)}/encryption`, {
    headers: { Authorization: `Bearer ${tok}` },
  });
  expect(listed.ok()).toBeTruthy();
  const body = (await listed.json()) as { enabled: boolean; algorithm: string };
  expect(body.enabled).toBe(true);
  expect(body.algorithm).toBe("AES256");

  await page.goto("/buckets");
  await expect(page.getByTestId("bucket-encryption-badge").first()).toBeVisible({ timeout: 15_000 });

  await page.goto(`/buckets/${encodeURIComponent(bucket)}?tab=settings`);
  await page.getByTestId("encryption-toggle").uncheck();
  await page.getByTestId("encryption-disable-confirm").click();
  await expect(page.getByText(/encryption saved|шифрование сохранено/i)).toBeVisible({ timeout: 10_000 });
});
