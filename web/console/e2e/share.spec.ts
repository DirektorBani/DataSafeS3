import { test, expect } from "@playwright/test";
import {
  adminToken,
  createBucket,
  createTenantWithMembers,
  createUser,
  e2eSuffix,
  login,
  userToken,
} from "./helpers";

test("owner grants bucket read access and grantee sees it under Shared with me", async ({
  page,
  request,
  baseURL,
}) => {
  const suffix = e2eSuffix();
  const bucketName = `e2e-share-${suffix}`;
  const owner = `e2e-owner-${suffix}`;
  const grantee = `e2e-grantee-${suffix}`;
  const password = `e2e-pass-${suffix}`;

  const adminTok = await adminToken(request, baseURL!);
  const ownerId = await createUser(request, baseURL!, adminTok, owner, password);
  const granteeId = await createUser(request, baseURL!, adminTok, grantee, password);
  await createTenantWithMembers(request, baseURL!, adminTok, `E2E Grant Co ${suffix}`, [
    ownerId,
    granteeId,
  ]);

  const ownerTok = await userToken(request, baseURL!, owner, password);
  await createBucket(request, baseURL!, ownerTok, bucketName);

  await login(page, owner, password);
  await page.goto(`/buckets/${encodeURIComponent(bucketName)}?tab=access`);

  await expect(page.getByRole("button", { name: /save access|сохранить доступ/i })).toBeVisible({
    timeout: 15_000,
  });
  const granteeRow = page.locator("div.rounded.border").filter({ hasText: grantee });
  await expect(granteeRow).toBeVisible({ timeout: 15_000 });
  await granteeRow.getByRole("checkbox").first().check();
  await page.getByRole("button", { name: /save access|сохранить доступ/i }).click();
  await expect(page.locator("body")).toContainText(/access rules saved|правила доступа сохранены/i, {
    timeout: 15_000,
  });

  await page.getByRole("button", { name: /sign out|выйти/i }).click();
  await page.waitForURL(/\/login/, { timeout: 15_000 });

  await login(page, grantee, password);
  await page.goto("/buckets");
  await page.getByRole("tab", { name: /shared with me|доступные мне/i }).click();
  await expect(page.getByRole("link", { name: bucketName })).toBeVisible({ timeout: 15_000 });
});
