import { test, expect } from "@playwright/test";
import { adminToken, createBucket, e2eSuffix, loginAsAdmin } from "./helpers";

test("bucket encryption card toggles and list shows a badge", async ({ page, request, baseURL }) => {
  const bucket = `e2e-enc-${e2eSuffix()}`;
  const tok = await adminToken(request, baseURL!);
  await createBucket(request, baseURL!, tok, bucket);

  const viewRes = await request.get(`${baseURL}/api/v1/buckets/${encodeURIComponent(bucket)}/encryption`, {
    headers: { Authorization: `Bearer ${tok}` },
  });
  expect(viewRes.ok()).toBeTruthy();
  const view = (await viewRes.json()) as {
    server_key_configured?: boolean;
    metadata_backend?: string;
    ha_enabled?: boolean;
  };

  await loginAsAdmin(page);
  await page.goto(`/buckets/${encodeURIComponent(bucket)}?tab=settings`);
  const card = page.getByTestId("encryption-card");
  await expect(card).toBeVisible({ timeout: 15_000 });

  const toggle = page.getByTestId("encryption-toggle");
  if (!view.server_key_configured) {
    await expect(toggle).toBeDisabled();
    const hint = page.getByTestId("encryption-key-missing");
    await expect(hint).toBeVisible();
    await expect(hint).toContainText("STORAGE_SSE_MASTER_KEY");
    return;
  }

  await expect(toggle).toBeEnabled();
  await toggle.check();
  await page.getByTestId("encryption-save").click();
  if (view.metadata_backend === "postgres" || view.ha_enabled) {
    const ack = page.getByTestId("encryption-upgrade-ack");
    await expect(ack).toBeVisible();
    await expect(page.getByTestId("encryption-upgrade-confirm")).toBeDisabled();
    await ack.check();
    await page.getByTestId("encryption-upgrade-confirm").click();
  }
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
  await page.screenshot({ path: "test-results/e08/04-encryption-badge.png", fullPage: true });

  await page.goto(`/buckets/${encodeURIComponent(bucket)}?tab=settings`);
  await page.getByTestId("encryption-toggle").click();
  const confirm = page.getByTestId("encryption-disable-confirm");
  await expect(confirm).toBeVisible({ timeout: 10_000 });
  await confirm.click();
  await expect(page.getByText(/encryption saved|шифрование сохранено/i)).toBeVisible({ timeout: 10_000 });
});
