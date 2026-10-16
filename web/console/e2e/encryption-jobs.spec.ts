import { test, expect } from "@playwright/test";
import { adminToken, createBucket, e2eSuffix, loginAsAdmin } from "./helpers";

test("rotate key, job progress, and destroy requires the bucket name", async ({ page, request, baseURL }) => {
  const bucket = `e2e-job-${e2eSuffix()}`;
  const tok = await adminToken(request, baseURL!);
  await createBucket(request, baseURL!, tok, bucket);
  const uploaded = await request.put(`${baseURL}/api/v1/buckets/${encodeURIComponent(bucket)}/objects/hello.txt`, {
    headers: { Authorization: `Bearer ${tok}`, "Content-Type": "text/plain" },
    data: "hello",
  });
  expect(uploaded.ok()).toBeTruthy();

  const viewRes = await request.get(`${baseURL}/api/v1/buckets/${encodeURIComponent(bucket)}/encryption`, {
    headers: { Authorization: `Bearer ${tok}` },
  });
  expect(viewRes.ok()).toBeTruthy();
  const view = (await viewRes.json()) as {
    server_key_configured?: boolean;
    metadata_backend?: string;
    ha_enabled?: boolean;
  };
  if (!view.server_key_configured) {
    return;
  }

  await loginAsAdmin(page);
  await page.goto(`/buckets/${encodeURIComponent(bucket)}?tab=settings`);
  const card = page.getByTestId("encryption-card");
  await expect(card).toBeVisible({ timeout: 15_000 });
  await page.getByTestId("encryption-toggle").check();
  await page.getByTestId("encryption-save").click();
  if (view.metadata_backend === "postgres" || view.ha_enabled) {
    await page.getByTestId("encryption-upgrade-ack").check();
    await page.getByTestId("encryption-upgrade-confirm").click();
  }
  await expect(page.getByText(/encryption saved|шифрование сохранено/i)).toBeVisible({ timeout: 10_000 });

  await page.getByTestId("encryption-rotate").click();
  await expect(page.getByText(/new key version|новой версией ключа/i)).toBeVisible();
  await page.getByTestId("encryption-rotate-confirm").click();
  await expect(page.getByText(/key rotated|ключ повёрнут/i)).toBeVisible({ timeout: 10_000 });

  await page.getByTestId("encryption-encrypt-existing").click();
  await expect(page.getByTestId("encryption-job-progress")).toHaveAttribute("data-progress", "100", {
    timeout: 30_000,
  });
  await expect(page.getByTestId("encryption-job-report")).toBeVisible();

  await page.getByTestId("encryption-destroy").click();
  await expect(page.getByText(/unreadable forever|нечитаемыми навсегда/i)).toBeVisible();
  await expect(page.getByTestId("encryption-destroy-backup")).toBeVisible();
  await page.getByTestId("encryption-destroy-next").click();
  await page.getByTestId("encryption-destroy-name").fill("wrong-name");
  await page.getByTestId("encryption-destroy-confirm").click();
  await expect(page.getByTestId("encryption-destroy-mismatch")).toBeVisible();
  await expect(page.getByTestId("encryption-key-destroyed")).toHaveCount(0);

  await page.getByTestId("encryption-destroy-name").fill(bucket);
  await page.getByTestId("encryption-destroy-confirm").click();
  await expect(page.getByTestId("encryption-key-destroyed")).toBeVisible({ timeout: 10_000 });
  await page.screenshot({ path: "test-results/e08/05-key-destroyed.png", fullPage: true });

  const downloaded = await request.get(
    `${baseURL}/api/v1/buckets/${encodeURIComponent(bucket)}/objects/hello.txt`,
    { headers: { Authorization: `Bearer ${tok}` } }
  );
  expect(downloaded.status()).toBe(403);
  expect(await downloaded.text()).toContain("Ключ шифрования уничтожен, файл недоступен навсегда");

  await page.goto(`/buckets/${encodeURIComponent(bucket)}`);
  await page.getByRole("button", { name: "hello.txt" }).click();
  await page.getByTestId("object-download").click();
  await expect(page.getByText("Ключ шифрования уничтожен, файл недоступен навсегда")).toBeVisible({
    timeout: 10_000,
  });
});
