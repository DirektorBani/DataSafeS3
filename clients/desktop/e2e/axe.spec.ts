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

async function boot(page: Page, profile: MockProfile, transfers: unknown[] = [], theme = "light", objectLists?: Record<string, ObjectList>) {
  await page.addInitScript(
    ({ profile, transfers, theme, objectLists }) => {
      localStorage.setItem("datasafe-desktop.theme", theme);
      const invoke = async (cmd: string, args?: { method?: string; params?: { prefix?: string } }) => {
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
            return { status: 200, body: { buckets: [{ name: "photos", access: { can_write: false, ownership: "owner" } }] } };
          }
          if (args?.method === "object.list") {
            const prefix = args.params?.prefix ?? "";
            const listed = objectLists?.[prefix];
            if (listed?.error) {
              throw new Error(listed.error);
            }
            if (listed) {
              return { status: listed.status ?? 200, body: listed.body, cached: listed.cached, cached_at: listed.cached_at };
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
    { profile, transfers, theme, objectLists },
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
  await expect(empty).toHaveAttribute("role", "option");
  await expect(empty).toHaveAttribute("aria-selected", "true");
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
