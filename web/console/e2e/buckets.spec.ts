import { test, expect } from "@playwright/test";
import { e2eSuffix, loginAsAdmin } from "./helpers";

test("admin can list buckets and create a disposable bucket", async ({ page }) => {
  const bucketName = `e2e-bkt-${e2eSuffix()}`;

  await loginAsAdmin(page);
  await page.goto("/buckets");

  await expect(page.locator("body")).toContainText(/bucket|бакет/i);
  await expect(page.getByRole("tab", { name: /my files|мои файлы/i })).toBeVisible();

  await page.getByRole("button", { name: /create storage|создать хранилище/i }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  const dialog = page.getByRole("dialog");
  await dialog.locator("#bucket-name").fill(bucketName);
  await dialog.getByRole("button", { name: /^create$|^создать$/i }).click();

  await expect(page.locator("body")).toContainText(/bucket created|бакет создан/i, { timeout: 15_000 });

  const refresh = page.getByRole("button", { name: /refresh|обновить/i }).first();
  await Promise.all([
    page.waitForResponse(
      (r) => r.url().includes("/api/v1/buckets") && r.request().method() === "GET" && r.ok()
    ),
    refresh.click(),
  ]);

  await page.getByPlaceholder(/^search buckets\.\.\.$|^поиск бакетов/i).fill(bucketName);
  await expect(page.getByRole("link", { name: bucketName })).toBeVisible({ timeout: 15_000 });
});
