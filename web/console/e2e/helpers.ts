import type { APIRequestContext, Page } from "@playwright/test";

const ADMIN_USER = "admin";
const ADMIN_PASS = "admin";

export function e2eSuffix(): string {
  return Date.now().toString(36);
}

export async function login(page: Page, username: string, password: string): Promise<void> {
  await page.goto("/login");
  await page.fill('input[name="username"], input[type="text"]', username);
  await page.fill('input[name="password"], input[type="password"]', password);
  await page.click('button[type="submit"]');
  await page.waitForURL((url) => !url.pathname.startsWith("/login"), { timeout: 30_000 });
}

export async function loginAsAdmin(page: Page): Promise<void> {
  await login(page, ADMIN_USER, ADMIN_PASS);
}

export async function adminToken(request: APIRequestContext, baseURL: string): Promise<string> {
  const res = await request.post(`${baseURL}/api/v1/admin/login`, {
    data: { username: ADMIN_USER, password: ADMIN_PASS },
  });
  if (!res.ok()) {
    throw new Error(`admin login failed: ${res.status()} ${await res.text()}`);
  }
  const body = (await res.json()) as { token?: string };
  if (!body.token) {
    throw new Error("admin login response missing token");
  }
  return body.token;
}

export async function createUser(
  request: APIRequestContext,
  baseURL: string,
  token: string,
  username: string,
  password: string,
  role = "user"
): Promise<string> {
  const res = await request.post(`${baseURL}/api/v1/users`, {
    headers: { Authorization: `Bearer ${token}` },
    data: { username, password, role, email: `${username}@e2e.test` },
  });
  if (!res.ok()) {
    throw new Error(`create user ${username} failed: ${res.status()} ${await res.text()}`);
  }
  const body = (await res.json()) as { id?: string };
  if (!body.id) {
    throw new Error(`create user ${username} response missing id`);
  }
  return body.id;
}

export async function userToken(
  request: APIRequestContext,
  baseURL: string,
  username: string,
  password: string
): Promise<string> {
  const res = await request.post(`${baseURL}/api/v1/admin/login`, {
    data: { username, password },
  });
  if (!res.ok()) {
    throw new Error(`login ${username} failed: ${res.status()} ${await res.text()}`);
  }
  const body = (await res.json()) as { token?: string };
  if (!body.token) {
    throw new Error(`login ${username} response missing token`);
  }
  return body.token;
}

export async function createTenantWithMembers(
  request: APIRequestContext,
  baseURL: string,
  adminTok: string,
  name: string,
  userIds: string[]
): Promise<string> {
  const tenantRes = await request.post(`${baseURL}/api/v1/tenants`, {
    headers: { Authorization: `Bearer ${adminTok}` },
    data: { name },
  });
  if (!tenantRes.ok()) {
    throw new Error(`create tenant failed: ${tenantRes.status()} ${await tenantRes.text()}`);
  }
  const tenantBody = (await tenantRes.json()) as { tenant?: { id?: string } };
  const tenantId = tenantBody.tenant?.id;
  if (!tenantId) {
    throw new Error("create tenant response missing id");
  }
  for (const userId of userIds) {
    const memberRes = await request.post(`${baseURL}/api/v1/tenants/${tenantId}/members`, {
      headers: { Authorization: `Bearer ${adminTok}` },
      data: { user_id: userId, role: "member" },
    });
    if (!memberRes.ok()) {
      throw new Error(`add tenant member failed: ${memberRes.status()} ${await memberRes.text()}`);
    }
  }
  return tenantId;
}

export async function createBucket(
  request: APIRequestContext,
  baseURL: string,
  token: string,
  name: string
): Promise<void> {
  const res = await request.post(`${baseURL}/api/v1/buckets/${encodeURIComponent(name)}`, {
    headers: { Authorization: `Bearer ${token}` },
    data: { visibility: "private" },
  });
  if (!res.ok()) {
    throw new Error(`create bucket ${name} failed: ${res.status()} ${await res.text()}`);
  }
}
