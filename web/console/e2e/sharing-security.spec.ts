import { test, expect } from "@playwright/test";
import { adminToken, createBucket, createTenantWithMembers, createUser, e2eSuffix, login, loginAsAdmin, userToken } from "./helpers";

test("access security page and bucket sharing card", async ({ page, request, baseURL }) => {
  const suffix = e2eSuffix();
  const bucket = `e2e-sharesec-${suffix}`;
  const tok = await adminToken(request, baseURL!);
  await createBucket(request, baseURL!, tok, bucket);

  await loginAsAdmin(page);
  await page.goto("/admin/settings/security");
  await expect(page.getByText(/access security|безопасность доступа/i).first()).toBeVisible({
    timeout: 15_000,
  });
  await expect(page.getByText(/block public access|запретить публичный доступ/i)).toBeVisible();

  await page.goto(`/buckets/${encodeURIComponent(bucket)}?tab=sharing`);
  await expect(page.getByRole("button", { name: /save sharing policy|сохранить политику обмена/i })).toBeVisible({
    timeout: 15_000,
  });
  await expect(page.getByRole("button", { name: /revoke all links|отозвать все ссылки/i })).toBeVisible();
});

test("bucket admin grant level is saved", async ({ page, request, baseURL }) => {
  const suffix = e2eSuffix();
  const bucket = `e2e-grant-${suffix}`;
  const owner = `e2e-owner-${suffix}`;
  const grantee = `e2e-viewer-${suffix}`;
  const password = `e2e-pass-${suffix}`;
  const tok = await adminToken(request, baseURL!);
  const ownerId = await createUser(request, baseURL!, tok, owner, password);
  const viewerId = await createUser(request, baseURL!, tok, grantee, password);
  await createTenantWithMembers(request, baseURL!, tok, `e2e-${suffix}`, [ownerId, viewerId]);
  const ownerTok = await userToken(request, baseURL!, owner, password);
  await createBucket(request, baseURL!, ownerTok, bucket);

  await login(page, owner, password);
  await page.goto(`/buckets/${encodeURIComponent(bucket)}?tab=access`);
  const row = page.locator("div.rounded.border").filter({ hasText: grantee });
  await expect(row).toBeVisible({ timeout: 15_000 });
  await row.getByRole("checkbox").nth(2).check();
  await page.getByRole("button", { name: /save access|сохранить доступ/i }).click();
  await expect(page.getByText(/access rules saved|правила доступа сохранены/i)).toBeVisible({ timeout: 10_000 });

  const listed = await request.get(`${baseURL}/api/v1/buckets/${encodeURIComponent(bucket)}/access`, {
    headers: { Authorization: `Bearer ${ownerTok}` },
  });
  expect(listed.ok()).toBeTruthy();
  const body = (await listed.json()) as { grants: { user_id: string; level: string }[] };
  expect(body.grants.find((g) => g.user_id === viewerId)?.level).toBe("admin");
});
