import { test, expect } from "@playwright/test";
import { adminToken, createUser, e2eSuffix, login } from "./helpers";

test("user role sees Files tabs (My files / Shared with me)", async ({ page, request, baseURL }) => {
  const suffix = e2eSuffix();
  const username = `e2e-user-${suffix}`;
  const password = `e2e-pass-${suffix}`;
  const token = await adminToken(request, baseURL!);
  await createUser(request, baseURL!, token, username, password);

  await login(page, username, password);
  await page.goto("/buckets");

  await expect(page.locator("body")).toContainText(/files|файлы/i);
  await expect(page.getByRole("tab", { name: /my files|мои файлы/i })).toBeVisible();
  await expect(page.getByRole("tab", { name: /shared with me|доступные мне/i })).toBeVisible();

  await page.getByRole("tab", { name: /shared with me|доступные мне/i }).click();
  await expect(page.locator("body")).toContainText(/shared with you|поделились|no files have been shared/i);
});
