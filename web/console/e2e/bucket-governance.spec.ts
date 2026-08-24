import { test, expect } from "@playwright/test";
import { createBucket, e2eSuffix, loginAsAdmin, adminToken } from "./helpers";

test("bucket settings expose versioning Suspend and Object Lock retention_mode", async ({
  page,
  request,
  baseURL,
}) => {
  const token = await adminToken(request, baseURL!);
  const bucketName = `e2e-gov-${e2eSuffix()}`;
  await createBucket(request, baseURL!, token, bucketName);

  await loginAsAdmin(page);
  await page.goto(`/admin/settings/buckets?bucket=${encodeURIComponent(bucketName)}`);

  await page.getByRole("tab", { name: /^versioning$|^версионирование$/i }).click();
  await expect(page.getByText(/enable object versioning|включить версионирование/i)).toBeVisible();
  const enableVersioning = page.getByRole("checkbox", {
    name: /enable object versioning|включить версионирование/i,
  });
  if (!(await enableVersioning.isChecked())) {
    await enableVersioning.click();
  }
  await expect(
    page.getByText(/suspend versioning|приостановить версионирование/i)
  ).toBeVisible();

  await page.getByRole("tab", { name: /object lock|блокировк/i }).click();
  await expect(page.getByRole("heading", { name: /object lock|блокировк/i })).toBeVisible();

  const enableWorm = page.getByRole("checkbox", {
    name: /enable immutable storage|неизменяемое хранение/i,
  });
  await expect(enableWorm).toBeVisible();
  if (!(await enableWorm.isChecked())) {
    await enableWorm.click();
  }

  await expect(page.getByText(/^retention mode$|^режим удержания$/i)).toBeVisible();
  await expect(page.getByText(/governance/i).first()).toBeVisible();
});
