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

async function boot(page: Page, profile: MockProfile, transfers: unknown[] = [], theme = "light") {
  await page.addInitScript(
    ({ profile, transfers, theme }) => {
      localStorage.setItem("datasafe-desktop.theme", theme);
      const invoke = async (cmd: string, args?: { method?: string }) => {
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
    { profile, transfers, theme },
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
  await page.getByRole("button", { name: "Buckets" }).click();
  await expect(page.getByText("photos")).toBeVisible();
  await expect(page.getByRole("button", { name: "Buckets" })).toHaveAttribute("aria-current", "page");
  expect(await serious(page)).toEqual([]);
});

test("a bucket row and an object row open from the keyboard", async ({ page }) => {
  await boot(page, {
    id: "p1",
    name: "Demo",
    server_url: "https://example.invalid",
    auth_method: "password",
    username: "demo",
    session: "signed_in",
    server: { browse_allowed: true },
  });
  await page.getByRole("button", { name: "Buckets" }).click();
  const bucket = page.getByRole("row", { name: /photos/ });
  await bucket.focus();
  await expect(bucket).toBeFocused();
  await expect(bucket).toHaveAttribute("aria-selected", "true");
  await page.keyboard.press("Enter");
  await expect(page.getByRole("heading", { name: "photos" })).toBeVisible();
  const objectRow = page.getByRole("row", { name: /readme\.txt/ });
  await objectRow.focus();
  await expect(objectRow).toBeFocused();
  await expect(objectRow).toHaveAttribute("aria-selected", "true");
  expect(await serious(page)).toEqual([]);
});
