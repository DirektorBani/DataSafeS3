import { test, expect } from "@playwright/test";
import { adminToken, loginAsAdmin } from "./helpers";

test("admin sees clusters page, federation nav, and cluster selector", async ({ page, request, baseURL }) => {
  await loginAsAdmin(page);

  await page.goto("/cluster");
  await expect(page.locator("body")).toContainText(/clusters|кластер/i);
  await expect(page.locator("body")).toContainText(/trusted clusters|доверенн/i);

  const token = await adminToken(request, baseURL!);
  const clustersRes = await request.get(`${baseURL}/api/v1/clusters`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(clustersRes.ok()).toBeTruthy();
  const clustersBody = (await clustersRes.json()) as {
    clusters?: { is_local?: boolean; name?: string }[];
  };
  expect(clustersBody.clusters?.some((c) => c.is_local)).toBeTruthy();

  await expect(page.getByRole("link", { name: /federation|федерац/i })).toBeVisible();

  await page.goto("/federation");
  await expect(page.locator("body")).toContainText(/federation|федерац/i);
  await expect(page.getByText(/cluster|кластер/i).first()).toBeVisible();
  await expect(page.getByRole("button", { name: /register|зарегистр/i })).toBeVisible();
});
