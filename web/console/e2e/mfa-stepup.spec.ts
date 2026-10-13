import { createHmac } from "node:crypto";
import { expect, test, type Page } from "@playwright/test";
import { adminToken, createBucket, createTenantWithMembers, createUser, e2eSuffix, login, loginAsAdmin, userToken } from "./helpers";

const B32 = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";

function base32Decode(input: string): Buffer {
  const clean = input.replace(/=+$/g, "").toUpperCase().replace(/[^A-Z2-7]/g, "");
  let bits = 0;
  let value = 0;
  const out: number[] = [];
  for (const ch of clean) {
    const idx = B32.indexOf(ch);
    if (idx < 0) continue;
    value = (value << 5) | idx;
    bits += 5;
    if (bits >= 8) {
      bits -= 8;
      out.push((value >>> bits) & 0xff);
    }
  }
  return Buffer.from(out);
}

function totp(secret: string, at = Date.now()): string {
  const counter = Math.floor(at / 1000 / 30);
  const buf = Buffer.alloc(8);
  buf.writeBigUInt64BE(BigInt(counter));
  const hmac = createHmac("sha1", base32Decode(secret)).update(buf).digest();
  const offset = hmac[hmac.length - 1] & 0x0f;
  const bin =
    ((hmac[offset] & 0x7f) << 24) |
    ((hmac[offset + 1] & 0xff) << 16) |
    ((hmac[offset + 2] & 0xff) << 8) |
    (hmac[offset + 3] & 0xff);
  return String(bin % 1_000_000).padStart(6, "0");
}

async function putPolicy(
  request: import("@playwright/test").APIRequestContext,
  baseURL: string,
  token: string,
  on: boolean,
  graceDays = 0,
) {
  const res = await request.put(`${baseURL}/api/v1/settings/mfa-policy`, {
    headers: { Authorization: `Bearer ${token}` },
    data: {
      require_bucket_admin_mfa: on,
      grace_period_days: graceDays,
      apply_to_tenant_admins: false,
      apply_to_global_admins: false,
      require_admin_mfa: false,
    },
  });
  if (!res.ok()) {
    throw new Error(`mfa policy failed: ${res.status()} ${await res.text()}`);
  }
}

async function confirmStepUp(page: Page, code: string, action: RegExp) {
  const dialog = page.getByTestId("mfa-step-up-dialog");
  await expect(dialog).toBeVisible({ timeout: 10_000 });
  await expect(dialog).toContainText(action);
  await dialog.getByTestId("mfa-step-up-code").fill(code);
  await dialog.getByTestId("mfa-step-up-confirm").click();
  await expect(dialog).toBeHidden({ timeout: 10_000 });
}

test.describe.configure({ mode: "serial" });

test.afterAll(async ({ request, baseURL }) => {
  const tok = await adminToken(request, baseURL!);
  await putPolicy(request, baseURL!, tok, false);
});

test("bucket admin banner, TOTP wizard, and two step-up dialogs", async ({ page, request, baseURL }) => {
  const suffix = e2eSuffix();
  const bucket = `e2e-mfa-${suffix}`;
  const owner = `e2e-mfaown-${suffix}`;
  const grantee = `e2e-mfaview-${suffix}`;
  const password = `e2e-pass-${suffix}`;
  const tok = await adminToken(request, baseURL!);
  const ownerId = await createUser(request, baseURL!, tok, owner, password);
  const viewerId = await createUser(request, baseURL!, tok, grantee, password);
  await createTenantWithMembers(request, baseURL!, tok, `e2e-mfa-${suffix}`, [ownerId, viewerId]);
  const ownerTok = await userToken(request, baseURL!, owner, password);
  await createBucket(request, baseURL!, ownerTok, bucket);
  await putPolicy(request, baseURL!, tok, true);

  await login(page, owner, password);
  await expect(page.getByTestId("mfa-policy-banner")).toBeVisible({ timeout: 15_000 });
  await expect(page.getByTestId("mfa-policy-banner")).toContainText(/two-factor authentication to administer|двухфакторную аутентификацию/i);
  await page.screenshot({ path: "/opt/cursor/artifacts/e00b/01-banner.png", fullPage: true });

  await page.goto("/profile");
  await page.getByRole("button", { name: /enable mfa|включить mfa/i }).click();
  const secret = (await page.locator("code.break-all").innerText()).replace(/\s+/g, "");
  expect(secret.length).toBeGreaterThan(8);
  const code = totp(secret);
  const inputs = page.locator('input[inputmode="numeric"]');
  for (let i = 0; i < code.length; i++) {
    await inputs.nth(i).fill(code[i]);
  }
  await expect(page.getByText(/recovery codes|коды восстановления/i)).toBeVisible({ timeout: 15_000 });
  const recovery = (await page.locator("ul.font-mono li").allTextContents()).map((c) => c.trim()).filter(Boolean);
  expect(recovery.length).toBeGreaterThan(1);
  await page.screenshot({ path: "/opt/cursor/artifacts/e00b/02-totp-enrolled.png", fullPage: true });
  await expect(page.getByTestId("mfa-policy-banner")).toHaveCount(0);

  await page.goto(`/buckets/${encodeURIComponent(bucket)}?tab=access`);
  const row = page.locator("div.rounded.border").filter({ hasText: grantee });
  await expect(row).toBeVisible({ timeout: 15_000 });
  await row.getByRole("checkbox").nth(2).check();
  await page.getByRole("button", { name: /save access|сохранить доступ/i }).click();
  await confirmStepUp(page, recovery[0], /change bucket grants|изменить доступ к бакету/i);
  await expect(page.getByText(/access rules saved|правила доступа сохранены/i)).toBeVisible({ timeout: 10_000 });

  await page.goto(`/buckets/${encodeURIComponent(bucket)}?tab=sharing`);
  await page.getByRole("button", { name: /revoke all links|отозвать все ссылки/i }).click();
  await confirmStepUp(page, recovery[1], /revoke every share link|отозвать все ссылки/i);
  await expect(page.getByText(/revoked|отозвано/i).first()).toBeVisible({ timeout: 10_000 });
  await page.screenshot({ path: "/opt/cursor/artifacts/e00b/03-step-up-done.png", fullPage: true });
});

test("global admin resets 2FA with a reason", async ({ page, request, baseURL }) => {
  const suffix = e2eSuffix();
  const username = `e2e-mfareset-${suffix}`;
  const password = `e2e-pass-${suffix}`;
  const tok = await adminToken(request, baseURL!);
  // Grace stays open so a global admin who already owns a bucket can reset 2FA
  // without a local factor. The denial check below closes the grace.
  await putPolicy(request, baseURL!, tok, true, 90);
  await createUser(request, baseURL!, tok, username, password);
  const userTok = await userToken(request, baseURL!, username, password);
  const enrolled = await request.post(`${baseURL}/api/v1/mfa/enroll`, {
    headers: { Authorization: `Bearer ${userTok}` },
  });
  expect(enrolled.ok()).toBeTruthy();
  const secret = ((await enrolled.json()) as { secret: string }).secret;
  const verified = await request.post(`${baseURL}/api/v1/mfa/verify-enroll`, {
    headers: { Authorization: `Bearer ${userTok}` },
    data: { code: totp(secret) },
  });
  expect(verified.ok()).toBeTruthy();

  await loginAsAdmin(page);
  await page.goto("/admin/users");
  await page.getByPlaceholder(/search users|поиск пользователей/i).fill(username);
  const row = page.getByRole("row", { name: new RegExp(username) });
  await expect(row).toBeVisible({ timeout: 15_000 });
  await row.getByRole("button", { name: /reset 2fa|сбросить 2fa/i }).click();
  await page.getByTestId("mfa-reset-reason").fill("lost device");
  await page.screenshot({ path: "/opt/cursor/artifacts/e00b/04-mfa-reset.png", fullPage: true });
  await page.getByRole("dialog").getByRole("button", { name: /reset 2fa|сбросить 2fa/i }).click();
  await expect(page.getByText(/2fa reset|2fa сброшена/i)).toBeVisible({ timeout: 10_000 });
  await page.goto("/admin/settings/security");
  await expect(page.getByTestId("mfa-policy-card")).toBeVisible({ timeout: 15_000 });
  await page.screenshot({ path: "/opt/cursor/artifacts/e00b/05-mfa-policy.png", fullPage: true });
  await page.goto("/keys");
  await expect(page.getByTestId("access-key-without-mfa").first()).toBeVisible({ timeout: 15_000 });
  await page.screenshot({ path: "/opt/cursor/artifacts/e00b/06-key-without-mfa.png", fullPage: true });

  const denied = await request.get(`${baseURL}/api/v1/buckets`, {
    headers: { Authorization: `Bearer ${userTok}` },
  });
  expect(denied.status()).toBe(403);
  const body = (await denied.json()) as { error?: string };
  expect(body.error).toBe("mfa_setup_required");
});
