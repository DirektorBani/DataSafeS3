import { test, expect } from "@playwright/test";
import { adminToken, createBucket, e2eSuffix, loginAsAdmin } from "./helpers";

const secret = "e2e-kms-secret-do-not-leak";

test("vault settings mask the secret and keep the bucket key disabled until an address is saved", async ({
  page,
  request,
  baseURL,
}) => {
  const tok = await adminToken(request, baseURL!);
  const bucket = `e2e-kms-${e2eSuffix()}`;
  await createBucket(request, baseURL!, tok, bucket);

  const clear = async () => {
    await request.put(`${baseURL}/api/v1/kms/settings`, {
      headers: { Authorization: `Bearer ${tok}` },
      data: { addr: "" },
    });
  };

  try {
    await loginAsAdmin(page);
    await page.goto("/admin/settings/kms");
    await expect(page.getByTestId("kms-settings")).toBeVisible({ timeout: 15_000 });
    await expect(page.getByTestId("kms-status")).toContainText(/Not configured|Не настроено/);

    await page.goto(`/buckets/${encodeURIComponent(bucket)}?tab=settings`);
    await expect(page.getByTestId("encryption-card")).toBeVisible({ timeout: 15_000 });
    const toggle = page.getByTestId("encryption-toggle");
    if (await toggle.isEnabled()) {
      await toggle.check();
      await expect(page.getByTestId("encryption-provider-vault")).toBeDisabled();
    }

    await page.goto("/admin/settings/kms");
    await page.getByTestId("kms-addr").fill("http://127.0.0.1:9");
    await page.getByTestId("kms-role-id").fill("e2e-role");
    await page.getByTestId("kms-secret").fill(secret);
    await page.getByTestId("kms-test").click();
    const result = page.getByTestId("kms-test-result");
    await expect(result).toBeVisible({ timeout: 20_000 });
    await expect(result).not.toContainText(/Connected, Transit works|Подключено, Transit работает/);
    await expect(result).not.toContainText(secret);

    await page.getByTestId("kms-save").click();
    await expect(page.getByText(/Vault settings saved|Настройки Vault сохранены/)).toBeVisible({
      timeout: 10_000,
    });

    await page.reload();
    await expect(page.getByTestId("kms-secret-set")).toBeVisible({ timeout: 15_000 });
    await expect(page.getByTestId("kms-secret")).toHaveValue("");

    const saved = await request.get(`${baseURL}/api/v1/kms/settings`, {
      headers: { Authorization: `Bearer ${tok}` },
    });
    expect(saved.ok()).toBeTruthy();
    const raw = await saved.text();
    expect(raw).not.toContain(secret);
    expect(raw).toContain('"secret_id_set":true');

    await page.goto(`/buckets/${encodeURIComponent(bucket)}?tab=settings`);
    await expect(page.getByTestId("encryption-card")).toBeVisible({ timeout: 15_000 });
    if (await page.getByTestId("encryption-toggle").isEnabled()) {
      await page.getByTestId("encryption-toggle").check();
      await expect(page.getByTestId("encryption-provider-vault")).toBeEnabled();
    }
  } finally {
    await clear();
  }
});
