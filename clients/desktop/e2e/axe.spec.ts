import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

type MockProfile = {
  id: string;
  name: string;
  server_url: string;
  auth_method: string;
  username?: string;
  session?: string;
  server?: { browse_allowed?: boolean; features?: Record<string, number> };
};

type ObjectList = { status?: number; body?: unknown; error?: string; cached?: boolean; cached_at?: number };

async function boot(page: Page, profile: MockProfile, transfers: unknown[] = [], theme = "light", objectLists?: Record<string, ObjectList>, writable = false) {
  await page.addInitScript(
    ({ profile, transfers, theme, objectLists, writable }) => {
      localStorage.setItem("datasafe-desktop.theme", theme);
      const removed = new Set<string>();
      const invoke = async (cmd: string, args?: { method?: string; params?: { prefix?: string; key?: string } }) => {
        if (cmd === "profile_list") {
          return { profiles: [profile] };
        }
        if (cmd === "settings_load") {
          return { locale: "en", theme };
        }
        if (cmd === "transfer") {
          if (args?.method === "transfer.list") {
            return { transfers };
          }
          return {};
        }
        if (cmd === "browse") {
          if (args?.method === "account.me") {
            return { status: 200, body: { username: profile.username ?? "" } };
          }
          if (args?.method === "bucket.list") {
            return { status: 200, body: { buckets: [{ name: "photos", access: { can_write: writable, ownership: "owner" } }] } };
          }
          if (args?.method === "object.delete" || args?.method === "folder.delete") {
            const key = args.params?.key || args.params?.prefix;
            if (key) {
              removed.add(key);
            }
            return { status: 200, body: {} };
          }
          if (args?.method === "object.list") {
            const prefix = args.params?.prefix ?? "";
            const listed = objectLists?.[prefix];
            if (listed?.error) {
              throw new Error(listed.error);
            }
            if (listed) {
              const body = listed.body && typeof listed.body === "object" ? { ...(listed.body as Record<string, unknown>) } : listed.body;
              if (body && typeof body === "object") {
                const record = body as { objects?: Array<{ key?: string }>; folders?: string[] };
                if (Array.isArray(record.objects)) {
                  record.objects = record.objects.filter((item) => !item.key || !removed.has(item.key));
                }
                if (Array.isArray(record.folders)) {
                  record.folders = record.folders.filter((key) => !removed.has(key));
                }
              }
              return { status: listed.status ?? 200, body, cached: listed.cached, cached_at: listed.cached_at };
            }
            return { status: 200, body: { objects: [{ key: "readme.txt", size: 12 }], folders: [], truncated: false } };
          }
          return { status: 200, body: {} };
        }
        return {};
      };
      Object.assign(window, {
        __TAURI_INTERNALS__: {
          invoke,
          transformCallback: () => 0,
          convertFileSrc: (path: string) => path,
        },
      });
    },
    { profile, transfers, theme, objectLists, writable },
  );
  await page.goto("/");
}

async function serious(page: Page) {
  const results = await new AxeBuilder({ page }).analyze();
  return results.violations.filter((item) => item.impact === "serious" || item.impact === "critical");
}

test("session expired is shown once and the shell has no serious axe violations", async ({ page }) => {
  await boot(page, {
    id: "p1",
    name: "OIDC",
    server_url: "https://example.invalid",
    auth_method: "oidc",
    username: "oidcuser",
    session: "reauth",
  });
  await expect(page.getByText("The session expired. Sign in again.")).toHaveCount(1);
  await expect(page.getByRole("button", { name: "Sign in with the browser" })).toBeVisible();
  expect(await serious(page)).toEqual([]);
});

test("the dark theme has no serious axe violations", async ({ page }) => {
  await boot(
    page,
    {
      id: "p1",
      name: "OIDC",
      server_url: "https://example.invalid",
      auth_method: "oidc",
      username: "oidcuser",
      session: "reauth",
    },
    [],
    "dark",
  );
  await expect(page.getByText("The session expired. Sign in again.")).toHaveCount(1);
  expect(await serious(page)).toEqual([]);
});

test("a paused transfer exposes a progress bar without serious axe violations", async ({ page }) => {
  await boot(
    page,
    {
      id: "p1",
      name: "Demo",
      server_url: "https://example.invalid",
      auth_method: "password",
      username: "demo",
      session: "signed_in",
      server: { browse_allowed: true },
    },
    [{ id: "t1", profile_id: "p1", direction: "upload", state: "interrupted", key: "big.bin", bytes_done: 524288, size: 1048576, message: "paused_signed_out" }],
  );
  const bar = page.getByRole("progressbar", { name: "Transfer progress" });
  await expect(bar).toBeVisible();
  await expect(bar).toHaveAttribute("aria-valuenow", "50");
  await expect(page.getByText(/paused when you signed out/)).toBeVisible();
  await expect(page.getByText("(paused_signed_out)")).toHaveCount(0);
  await page.getByRole("button", { name: "Buckets" }).click();
  await expect(page.getByText("photos")).toBeVisible();
  await expect(page.getByRole("button", { name: "Buckets" })).toHaveAttribute("aria-current", "page");
  expect(await serious(page)).toEqual([]);
});

test("a bucket row and an object row open from the keyboard", async ({ page }) => {
  await boot(
    page,
    {
      id: "p1",
      name: "Demo",
      server_url: "https://example.invalid",
      auth_method: "password",
      username: "demo",
      session: "signed_in",
      server: { browse_allowed: true },
    },
    [],
    "light",
    {
      "": { status: 200, body: { folders: ["docs/"], objects: [{ key: "readme.txt", size: 12 }], truncated: false } },
      "docs/": { status: 200, body: { folders: [], objects: [{ key: "docs/inside.txt", size: 4 }], truncated: false } },
    },
  );
  await page.getByRole("button", { name: "Buckets" }).click();
  const buckets = page.getByRole("listbox", { name: "Buckets" });
  await expect(buckets).toHaveAttribute("aria-multiselectable", "false");
  const bucket = page.getByRole("option", { name: /photos/ });
  await bucket.focus();
  await expect(bucket).toBeFocused();
  await expect(bucket).toHaveAttribute("aria-selected", "true");
  await page.keyboard.press("Enter");
  await expect(page.getByRole("heading", { name: "photos" })).toBeVisible();
  const objects = page.getByRole("listbox", { name: "Objects" });
  await expect(objects).toHaveAttribute("aria-multiselectable", "false");
  const folder = page.getByRole("option", { name: /docs/ });
  await folder.focus();
  await expect(folder).toBeFocused();
  await expect(folder).toHaveAttribute("aria-selected", "true");
  const readme = page.getByRole("option", { name: /readme\.txt/ });
  await expect(readme).toHaveAttribute("aria-selected", "false");
  await page.keyboard.press("Enter");
  const inside = page.getByRole("option", { name: /inside\.txt/ });
  await expect(inside).toBeVisible();
  await expect(inside).toBeFocused();
  await expect(inside).toHaveAttribute("aria-selected", "true");
  await expect(objects).toHaveAttribute("aria-activedescendant", "object-row-0");
  expect(await serious(page)).toEqual([]);
});

test("an uncached folder clears the parent rows", async ({ page }) => {
  await boot(
    page,
    {
      id: "p1",
      name: "Demo",
      server_url: "https://example.invalid",
      auth_method: "password",
      username: "demo",
      session: "signed_in",
      server: { browse_allowed: true },
    },
    [],
    "light",
    {
      "": {
        status: 200,
        cached: true,
        cached_at: 1700000000,
        body: { folders: ["docs/"], objects: [{ key: "readme.txt", size: 12 }], truncated: false },
      },
      "docs/": { error: 'Get "http://127.0.0.1:9/objects": dial tcp 127.0.0.1:9: connectex: No connection could be made because the target machine actively refused it.' },
    },
  );
  await page.getByRole("button", { name: "Buckets" }).click();
  await page.getByRole("option", { name: /photos/ }).click();
  await expect(page.getByRole("option", { name: /readme\.txt/ })).toBeVisible();
  await page.getByRole("option", { name: /docs/ }).click();
  await expect(page.getByRole("status").filter({ hasText: "This folder is not available offline." })).toBeVisible();
  const empty = page.locator("#object-row-empty");
  await expect(empty).toHaveAttribute("role", "status");
  await expect(empty).not.toHaveAttribute("aria-selected");
  await expect(page.getByRole("listbox", { name: "Objects" })).toHaveCount(0);
  await expect(page.getByRole("option", { name: /not available offline/ })).toHaveCount(0);
  await expect(page.getByRole("option", { name: /readme\.txt/ })).toHaveCount(0);
  await expect(page.getByText("docs", { exact: true })).toBeVisible();
  expect(await serious(page)).toEqual([]);
});

test("switching language keeps the open folder", async ({ page }) => {
  await boot(
    page,
    {
      id: "p1",
      name: "Demo",
      server_url: "https://example.invalid",
      auth_method: "password",
      username: "demo",
      session: "signed_in",
      server: { browse_allowed: true },
    },
    [],
    "light",
    {
      "": { status: 200, body: { folders: ["docs/"], objects: [{ key: "readme.txt", size: 12 }], truncated: false } },
      "docs/": { status: 200, body: { folders: [], objects: [{ key: "docs/inside.txt", size: 4 }], truncated: false } },
    },
  );
  await page.getByRole("button", { name: "Buckets" }).click();
  await page.getByRole("option", { name: /photos/ }).click();
  await page.getByRole("option", { name: /docs/ }).click();
  await expect(page.getByRole("option", { name: /inside\.txt/ })).toBeVisible();
  await page.getByRole("button", { name: "RU", exact: true }).click();
  await expect(page.getByRole("button", { name: "Корень" })).toBeVisible();
  await expect(page.getByRole("button", { name: "docs" })).toBeVisible();
  await expect(page.getByRole("option", { name: /inside\.txt/ })).toBeVisible();
  await expect(page.getByRole("option", { name: /readme\.txt/ })).toHaveCount(0);
  await page.getByRole("button", { name: "EN", exact: true }).click();
  await expect(page.getByRole("button", { name: "Root" })).toBeVisible();
  await expect(page.getByRole("option", { name: /inside\.txt/ })).toBeVisible();
  expect(await serious(page)).toEqual([]);
});

test("a missing local file is not a server rejection", async ({ page }) => {
  await boot(page, {
    id: "p1",
    name: "Demo",
    server_url: "https://example.invalid",
    auth_method: "password",
    username: "demo",
    session: "signed_in",
    server: { browse_allowed: true },
  }, [{ id: "t1", profile_id: "p1", direction: "upload", state: "error", key: "does-not-exist.bin", bytes_done: 0, size: 12, message: "local.file_missing" }]);
  await expect(page.getByText(/not on this computer/)).toBeVisible();
  await expect(page.getByText("(local.file_missing)")).toBeVisible();
  await expect(page.getByText(/server rejected the upload/)).toHaveCount(0);
  expect(await serious(page)).toEqual([]);
});

async function expectConsoleColumns(page: Page) {
  await page.getByRole("button", { name: "Buckets" }).click();
  const bucketHeader = page.locator("#buckets-col-0");
  await expect(bucketHeader).toBeVisible();
  const bucketWeight = await bucketHeader.evaluate((el) => Number(getComputedStyle(el).fontWeight));
  expect(bucketWeight).toBeGreaterThanOrEqual(700);
  expect(await bucketHeader.evaluate((el) => el.closest("[aria-hidden='true']") !== null)).toBe(false);
  await page.getByRole("option", { name: /photos/ }).click();
  const modifiedHeader = page.locator("#objects-col-2");
  await expect(modifiedHeader).toHaveText("Modified");
  const weight = await modifiedHeader.evaluate((el) => Number(getComputedStyle(el).fontWeight));
  expect(weight).toBeGreaterThanOrEqual(700);
  expect(await modifiedHeader.evaluate((el) => el.closest("[aria-hidden='true']") !== null)).toBe(false);
  const metrics = await page.evaluate(() => {
    const box = (selector: string) => document.querySelector(selector)!.getBoundingClientRect();
    const name = box("[data-column='name']");
    const size = box("[data-column='size']");
    const modified = box("[data-column='modified']");
    const headerSize = document.getElementById("objects-col-1")!.getBoundingClientRect();
    const date = document.querySelector("[data-column='modified']")!;
    return {
      nameTop: name.top,
      sizeTop: size.top,
      modifiedTop: modified.top,
      modifiedHeight: modified.height,
      headerSizeLeft: headerSize.left,
      rowSizeLeft: size.left,
      nowrap: getComputedStyle(date).whiteSpace,
    };
  });
  expect(metrics.nowrap).toBe("nowrap");
  expect(metrics.modifiedHeight).toBeLessThan(56);
  expect(Math.abs(metrics.nameTop - metrics.sizeTop)).toBeLessThan(4);
  expect(Math.abs(metrics.sizeTop - metrics.modifiedTop)).toBeLessThan(4);
  expect(Math.abs(metrics.headerSizeLeft - metrics.rowSizeLeft)).toBeLessThan(2);
  const option = page.getByRole("option", { name: /readme\.txt/ });
  await expect(option).toHaveAccessibleName(/readme\.txt/);
  await expect(option).toHaveAccessibleName(/15 B/);
  await expect(option).not.toHaveAccessibleName(/2026-10-17T/);
  await expect(option).not.toHaveAccessibleName(/Delete/);
  await expect(page.getByRole("listbox", { name: "Objects" })).toHaveAttribute("aria-describedby", /objects-col-0/);
  expect(await serious(page)).toEqual([]);
}

const longObject = {
  "": {
    status: 200,
    body: {
      folders: [],
      objects: [{ key: "readme.txt", size: 15, last_modified: "2026-10-17T21:23:44.3860237Z" }],
      truncated: false,
    },
  },
};

test("object columns keep the console grid in the light theme", async ({ page }) => {
  await boot(
    page,
    { id: "p1", name: "Demo", server_url: "https://example.invalid", auth_method: "password", username: "demo", session: "signed_in", server: { browse_allowed: true } },
    [],
    "light",
    longObject,
    true,
  );
  await expectConsoleColumns(page);
});

test("object columns keep the console grid in the dark theme", async ({ page }) => {
  await boot(
    page,
    { id: "p1", name: "Demo", server_url: "https://example.invalid", auth_method: "password", username: "demo", session: "signed_in", server: { browse_allowed: true } },
    [],
    "dark",
    longObject,
    true,
  );
  await expectConsoleColumns(page);
});

test("delete confirmation takes focus and the trash icon is reached with Tab", async ({ page }) => {
  await boot(
    page,
    { id: "p1", name: "Demo", server_url: "https://example.invalid", auth_method: "password", username: "demo", session: "signed_in", server: { browse_allowed: true } },
    [],
    "light",
    {
      "": {
        status: 200,
        body: {
          folders: [],
          objects: [
            { key: "a.txt", size: 1, last_modified: "2026-10-17T21:23:44.3860237Z" },
            { key: "b.txt", size: 2, last_modified: "2026-10-17T22:23:44.3860237Z" },
          ],
          truncated: false,
        },
      },
    },
    true,
  );
  await page.getByRole("button", { name: "Buckets" }).click();
  await page.getByRole("option", { name: /photos/ }).click();
  const first = page.getByRole("option", { name: /a\.txt/ });
  await first.focus();
  expect(await serious(page)).toEqual([]);
  await page.keyboard.press("Tab");
  await expect(page.getByRole("button", { name: "Delete a.txt" })).toBeFocused();
  await page.keyboard.press("Shift+Tab");
  await expect(first).toBeFocused();
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Delete");
  const confirm = page.getByRole("group", { name: "Delete b.txt" }).getByRole("button", { name: "Delete b.txt" });
  await expect(confirm).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("option", { name: /b\.txt/ })).toHaveCount(0);
  await expect(page.getByRole("option", { name: /a\.txt/ })).toBeFocused();
  expect(await serious(page)).toEqual([]);
});
