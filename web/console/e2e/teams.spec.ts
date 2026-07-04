import { test, expect } from "@playwright/test";
import { adminToken, e2eSuffix, loginAsAdmin } from "./helpers";

test("admin can create a team and assign a member", async ({ page, request, baseURL }) => {
  const teamName = `e2e-team-${e2eSuffix()}`;

  await loginAsAdmin(page);
  await page.goto("/admin/teams");

  await expect(page.locator("body")).toContainText(/teams|команды/i);
  await expect(page.getByPlaceholder(/team name|название команды/i)).toBeVisible();

  await page.getByPlaceholder(/team name|название команды/i).fill(teamName);
  await page.getByRole("button", { name: /create team|создать команду/i }).click();
  await expect(page.locator("body")).toContainText(/team created|команда создана/i, {
    timeout: 15_000,
  });

  const teamRow = page.locator("li").filter({ hasText: teamName });
  await expect(teamRow).toBeVisible({ timeout: 15_000 });
  await teamRow.getByRole("button", { name: teamName }).click();

  const adminRow = page.locator("div.flex.items-center").filter({ hasText: /^admin$/i });
  await expect(adminRow).toBeVisible({ timeout: 15_000 });
  await adminRow.getByRole("checkbox").check();

  await page.getByRole("button", { name: /save members|сохранить участников/i }).click();
  await expect(page.locator("body")).toContainText(/members updated|участники обновлены/i, {
    timeout: 15_000,
  });

  await page.getByRole("button", { name: /^members$|^участники$/i }).click();
  await expect(adminRow.getByRole("checkbox")).toBeChecked();

  const token = await adminToken(request, baseURL!);
  const teamsRes = await request.get(`${baseURL}/api/v1/teams`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(teamsRes.ok()).toBeTruthy();
  const teamsBody = (await teamsRes.json()) as { teams?: { id: string; name: string }[] };
  const team = (teamsBody.teams ?? []).find((t) => t.name === teamName);
  expect(team).toBeDefined();

  const membersRes = await request.get(`${baseURL}/api/v1/teams/${team!.id}/members`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(membersRes.ok()).toBeTruthy();
  const membersBody = (await membersRes.json()) as { members?: { username: string }[] };
  expect(membersBody.members?.some((m) => m.username === "admin")).toBeTruthy();
});
