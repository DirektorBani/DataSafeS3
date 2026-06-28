const API_BASE = "/api/v1";
const TOKEN_KEY = "datasafe_admin_token";
const ROLE_KEY = "datasafe_admin_role";
const USER_KEY = "datasafe_admin_user";
const AUTH_SOURCE_KEY = "datasafe_auth_source";
const TENANT_MEMBERSHIPS_KEY = "datasafe_tenant_memberships";
const IS_TENANT_ADMIN_KEY = "datasafe_is_tenant_admin";

export type UserRole = "administrator" | "operator" | "user";
export type TenantRole = "tenant_admin" | "member" | "viewer";

export type TenantMembership = {
  tenant_id: string;
  tenant_name?: string;
  role: TenantRole | string;
};

export type Bucket = {
  name: string;
  created_at: string;
  owner?: string;
  owner_id?: string;
  visibility?: string;
  tenant_id?: string;
  storage_key?: string;
  access?: {
    ownership: "owned" | "shared" | "tenant";
    can_read: boolean;
    can_write: boolean;
    shared_by?: string | null;
    shared_prefixes?: string[];
  };
};
export type ObjectRow = {
  key: string;
  size: number;
  etag: string;
  last_modified: string;
  version_id?: string;
  content_type?: string;
  scheduled_delete_at?: string;
  legal_hold?: boolean;
  retention_until?: string;
  storage_class?: string;
  metadata?: Record<string, string>;
  tags?: Record<string, string>;
  created_at?: string;
};

export type LifecycleRule = {
  id: string;
  name?: string;
  prefix?: string;
  action?: string;
  expiration_days: number;
  enabled: boolean;
};

export type QuotaInfo = {
  max_size_bytes: number;
  max_objects: number;
  used_size: number;
  used_objects: number;
  remaining_size?: number;
  remaining_objects?: number;
};
export type AccessKey = { access_key: string; label: string; created_at: string; owner?: string };
export type CreatedKey = AccessKey & { secret_key: string };

export type ActivityEvent = {
  id: string;
  timestamp: string;
  user: string;
  action: string;
  resource_type: string;
  resource_name: string;
  ip_address: string;
};

export type UsageSummary = {
  bucket_count: number;
  object_count: number;
  total_size: number;
  upload_bytes: number;
  download_bytes: number;
};

export type BucketUsage = {
  name: string;
  object_count: number;
  total_size: number;
  last_activity?: string;
  owner: string;
};

export type UsageResponse = {
  scope?: { system_wide: boolean };
  summary: UsageSummary;
  quota?: QuotaInfo;
  buckets: BucketUsage[];
  storage_growth: { date: string; bytes: number }[];
  objects_growth: { date: string; objects: number }[];
};

export type ConsoleUser = {
  id: string;
  username: string;
  email: string;
  role: UserRole;
  status: "active" | "suspended";
  max_size_bytes?: number;
  max_objects?: number;
  last_login?: string;
  created_at: string;
};

export type BucketSettings = {
  name: string;
  owner: string;
  owner_id?: string;
  description: string;
  versioning_enabled: boolean;
  object_lock_enabled: boolean;
  retention_days?: number;
  storage_class?: string;
  tenant_id?: string;
  visibility: string;
  max_size_bytes: number;
  max_objects: number;
  lifecycle_rules: LifecycleRule[];
  tags?: Record<string, string>;
};

export const QUOTA_PRESETS = [
  { label: "Unlimited", bytes: 0 },
  { label: "10 GB", bytes: 10 * 1024 * 1024 * 1024 },
  { label: "100 GB", bytes: 100 * 1024 * 1024 * 1024 },
  { label: "1 TB", bytes: 1024 * 1024 * 1024 * 1024 },
] as const;

export const OBJECT_QUOTA_PRESETS = [
  { label: "Unlimited", count: 0 },
  { label: "1 000", count: 1000 },
  { label: "10 000", count: 10000 },
  { label: "100 000", count: 100000 },
  { label: "1 000 000", count: 1000000 },
] as const;

export type TrashItem = {
  id: string;
  original_bucket: string;
  original_key: string;
  trash_key: string;
  size: number;
  deleted_by?: string;
  deleted_at: string;
};

export type APIToken = {
  id: string;
  name: string;
  username: string;
  scopes: string[];
  expires_at: string;
  created_at: string;
};

export type Webhook = {
  id: string;
  name: string;
  url: string;
  events: string[];
  headers?: Record<string, string>;
  enabled: boolean;
  created_at: string;
};

export const WEBHOOK_EVENTS = [
  "ObjectCreated",
  "ObjectDeleted",
  "BucketCreated",
  "BucketDeleted",
  "UserCreated",
] as const;

export type StorageQuotaUnit = "MB" | "GB" | "TB";

export function bytesFromQuota(value: number, unit: StorageQuotaUnit): number {
  if (value <= 0) return 0;
  const mult = unit === "TB" ? 1024 ** 4 : unit === "GB" ? 1024 ** 3 : 1024 ** 2;
  return Math.round(value * mult);
}

export function quotaFromBytes(bytes: number): { value: number; unit: StorageQuotaUnit; unlimited: boolean } {
  if (!bytes || bytes <= 0) return { value: 0, unit: "GB", unlimited: true };
  if (bytes % 1024 ** 4 === 0) return { value: bytes / 1024 ** 4, unit: "TB", unlimited: false };
  if (bytes % 1024 ** 3 === 0) return { value: bytes / 1024 ** 3, unit: "GB", unlimited: false };
  return { value: Math.max(1, Math.round(bytes / 1024 ** 2)), unit: "MB", unlimited: false };
}

export type LogSinkEndpoint = {
  enabled?: boolean;
  address?: string;
  username?: string;
  password?: string;
  token?: string;
  tls?: boolean;
  headers?: Record<string, string>;
  index?: string;
};

export type LoggingConfig = {
  syslog?: LogSinkEndpoint;
  loki?: LogSinkEndpoint;
  elasticsearch?: LogSinkEndpoint;
  webhook?: LogSinkEndpoint;
};

export type SystemConfig = {
  initial_setup_completed?: boolean;
  admin_first_login_completed?: boolean;
  admin_password_changed?: boolean;
  soft_delete_enabled: boolean;
  trash_retention_days: number;
  external_s3?: ExternalS3Config;
  ldap?: LDAPConfig;
  oidc?: OIDCConfig;
  mfa?: MFASettings;
  cluster?: ClusterConfig;
  logging?: LoggingConfig;
};

export type ExternalS3Config = {
  endpoint: string;
  access_key_id: string;
  secret_access_key?: string;
  bucket: string;
  region: string;
  use_ssl: boolean;
};

export type SetupStatus = {
  initial_setup_completed: boolean;
  admin_first_login_completed: boolean;
  admin_password_changed: boolean;
  needs_password_change: boolean;
  needs_setup: boolean;
};

export type SharedLink = {
  id: string;
  bucket: string;
  key: string;
  token: string;
  expires_at?: string;
  max_downloads: number;
  download_count: number;
  created_by: string;
  created_at: string;
};

export type TenantMember = {
  user_id: string;
  username?: string;
  email?: string;
  role: string;
  groups?: { id: string; name: string }[];
};

export type TenantGroup = {
  id: string;
  tenant_id: string;
  name: string;
  external_name?: string;
  description?: string;
  access_level: "read" | "read_write";
  created_at: string;
  bucket_count?: number;
  member_count?: number;
};

export type BucketAccessGrant = {
  user_id: string;
  username?: string;
  prefix?: string;
  can_read: boolean;
  can_write: boolean;
};

export type UserNotification = {
  id: string;
  kind: string;
  title: string;
  body?: string;
  link?: string;
  read_at?: string;
  created_at: string;
};

export type RecentItem = {
  id: string;
  bucket: string;
  prefix?: string;
  accessed_at: string;
};

export type LDAPConfig = {
  enabled?: boolean;
  url?: string;
  bind_dn?: string;
  bind_password?: string;
  base_dn?: string;
  group_dn?: string;
  user_attr?: string;
  group_attr?: string;
  group_role_map?: Record<string, string>;
  sync_on_login?: boolean;
  sync_interval_minutes?: number;
};

export type SecurityStatus = {
  weak_secrets: string[];
  doc?: string;
};

export type OIDCConfig = {
  enabled?: boolean;
  issuer?: string;
  internal_issuer?: string;
  client_id?: string;
  client_secret?: string;
  redirect_url?: string;
  groups_claim?: string;
};

export type MFASettings = {
  require_admin_mfa?: boolean;
};

export type ClusterNode = {
  id: string;
  address: string;
  role: string;
  status?: string;
};

export type ClusterConfig = {
  distributed_mode?: boolean;
  nodes?: ClusterNode[];
  erasure_coding_planned?: boolean;
  disk_paths?: string[];
};

export type Tenant = {
  id: string;
  name: string;
  status: string;
  created_at: string;
};

export type GatewayConnection = {
  id: string;
  name: string;
  endpoint: string;
  region: string;
  access_key: string;
  path_style: boolean;
  tls_verify: boolean;
  status?: string;
  last_check?: string;
  created_at: string;
};

export type ReplicationRule = {
  id: string;
  name?: string;
  source_bucket: string;
  dest_connection_id: string;
  dest_bucket: string;
  enabled: boolean;
  created_at: string;
};

export type ReplicationTask = {
  id: string;
  rule_id: string;
  event: string;
  source_bucket: string;
  key: string;
  status: string;
  attempts: number;
  bytes: number;
  error?: string;
  created_at: string;
  processed_at?: string;
};

export type ReplicationError = {
  id: string;
  task_id?: string;
  rule_id: string;
  event: string;
  source_bucket: string;
  key: string;
  message: string;
  created_at: string;
};

export type SyncJob = {
  id: string;
  rule_id: string;
  status: string;
  objects_synced: number;
  errors: number;
  message?: string;
  started_at: string;
  ended_at?: string;
};

export type GatewayHealth = {
  connections_total: number;
  connections_ok: number;
  rules_total: number;
  rules_broken?: number;
  recent_jobs: SyncJob[];
  recent_errors?: ReplicationError[];
  queue_pending: number;
  queue_lag_seconds: number;
  bytes_replicated: number;
  replication_errors: number;
  tasks_completed: number;
  last_processed_at?: string;
};

export type FederationCluster = {
  id: string;
  name: string;
  endpoint: string;
  region?: string;
  status?: string;
  created_at: string;
};

export const RETENTION_PRESETS = [
  { label: "30 days", days: 30 },
  { label: "90 days", days: 90 },
  { label: "1 year", days: 365 },
] as const;

export const STORAGE_CLASSES = [
  { value: "hot", label: "Hot" },
  { value: "warm", label: "Warm" },
  { value: "cold", label: "Cold" },
] as const;

export type SearchResult = {
  type: "bucket" | "object" | "user";
  name: string;
  bucket?: string;
  key?: string;
  size?: number;
  owner?: string;
  username?: string;
  email?: string;
  last_modified?: string;
};

export type Favorite = {
  id: string;
  user_id: string;
  type: "bucket" | "folder";
  bucket: string;
  prefix?: string;
  created_at: string;
};

export type WebhookDelivery = {
  id: string;
  webhook_id: string;
  event: string;
  url: string;
  status_code: number;
  success: boolean;
  error?: string;
  attempts: number;
  payload: string;
  created_at: string;
  last_attempt: string;
};

export type MultipartUploadProgress = {
  loaded: number;
  total: number;
  partsDone: number;
  partsTotal: number;
  speed: number;
  eta: number;
};

export const MULTIPART_THRESHOLD = 64 * 1024 * 1024;
export const MULTIPART_PART_SIZE = 64 * 1024 * 1024;

export function getToken(): string | null {
  return sessionStorage.getItem(TOKEN_KEY);
}

export function getRole(): UserRole | null {
  const r = sessionStorage.getItem(ROLE_KEY);
  return r as UserRole | null;
}

export function getUsername(): string | null {
  return sessionStorage.getItem(USER_KEY);
}

export function setToken(token: string): void {
  sessionStorage.setItem(TOKEN_KEY, token);
}

export function setSession(token: string, role: UserRole, username: string, authSource?: string): void {
  sessionStorage.setItem(TOKEN_KEY, token);
  sessionStorage.setItem(ROLE_KEY, role);
  sessionStorage.setItem(USER_KEY, username);
  if (authSource) {
    sessionStorage.setItem(AUTH_SOURCE_KEY, authSource);
  } else {
    sessionStorage.removeItem(AUTH_SOURCE_KEY);
  }
}

export function getTenantMemberships(): TenantMembership[] {
  const raw = sessionStorage.getItem(TENANT_MEMBERSHIPS_KEY);
  if (!raw) return [];
  try {
    return JSON.parse(raw) as TenantMembership[];
  } catch {
    return [];
  }
}

export function isTenantAdminSession(): boolean {
  return sessionStorage.getItem(IS_TENANT_ADMIN_KEY) === "1" || isAdministrator();
}

export function canManageTenant(tenantId: string): boolean {
  if (isAdministrator()) return true;
  return getTenantMemberships().some((m) => m.tenant_id === tenantId && m.role === "tenant_admin");
}

export function setSessionProfile(
  token: string,
  role: UserRole,
  username: string,
  opts?: { authSource?: string; tenantMemberships?: TenantMembership[]; isTenantAdmin?: boolean }
): void {
  setSession(token, role, username, opts?.authSource);
  if (opts?.tenantMemberships) {
    sessionStorage.setItem(TENANT_MEMBERSHIPS_KEY, JSON.stringify(opts.tenantMemberships));
  } else {
    sessionStorage.removeItem(TENANT_MEMBERSHIPS_KEY);
  }
  if (opts?.isTenantAdmin) {
    sessionStorage.setItem(IS_TENANT_ADMIN_KEY, "1");
  } else {
    sessionStorage.removeItem(IS_TENANT_ADMIN_KEY);
  }
}

export async function refreshSessionFromMe(): Promise<void> {
  const token = getToken();
  if (!token) return;
  const me = await fetchJSON<{
    username: string;
    role: UserRole;
    auth_source?: string;
    tenant_memberships?: TenantMembership[];
    is_tenant_admin?: boolean;
    mfa_setup_required?: boolean;
    mfa_enabled?: boolean;
  }>("/me");
  setSessionProfile(token, me.role, me.username, {
    authSource: me.auth_source,
    tenantMemberships: me.tenant_memberships,
    isTenantAdmin: me.is_tenant_admin,
  });
  if (me.mfa_setup_required) {
    localStorage.setItem("datasafe_mfa_setup_required", "1");
  } else if (me.mfa_enabled) {
    localStorage.removeItem("datasafe_mfa_setup_required");
  }
}

export function isMfaSetupRequired(): boolean {
  return localStorage.getItem("datasafe_mfa_setup_required") === "1";
}

export function getAuthSource(): string | null {
  return sessionStorage.getItem(AUTH_SOURCE_KEY);
}

export function clearToken(): void {
  sessionStorage.removeItem(TOKEN_KEY);
  sessionStorage.removeItem(ROLE_KEY);
  sessionStorage.removeItem(USER_KEY);
  sessionStorage.removeItem(AUTH_SOURCE_KEY);
  sessionStorage.removeItem(TENANT_MEMBERSHIPS_KEY);
  sessionStorage.removeItem(IS_TENANT_ADMIN_KEY);
  localStorage.removeItem("datasafe_mfa_setup_required");
}

export function isAuthenticated(): boolean {
  return !!getToken();
}

export function isAdministrator(): boolean {
  return getRole() === "administrator";
}

export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

async function fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  const token = getToken();
  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }
  const res = await fetch(`${API_BASE}${path}`, { ...init, headers });
  if (res.status === 401) {
    clearToken();
    window.dispatchEvent(new Event("datasafe:unauthorized"));
    throw new ApiError("Session expired. Please sign in again.", 401);
  }
  if (!res.ok) {
    const text = await res.text();
    let message = text || res.statusText;
    try {
      const json = JSON.parse(text);
      if (json.error) message = json.error;
    } catch {
      /* use raw text */
    }
    throw new ApiError(message, res.status);
  }
  if (res.status === 204) return {} as T;
  return res.json();
}

export async function login(username: string, password: string): Promise<{ mfa_required?: boolean; mfa_token?: string; mfa_setup_required?: boolean }> {
  const res = await fetch(`${API_BASE}/admin/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });
  const text = await res.text();
  let data: Record<string, unknown> = {};
  try {
    data = JSON.parse(text);
  } catch {
    /* */
  }
  if (!res.ok) {
    throw new ApiError((data.error as string) || text || "Login failed", res.status);
  }
  if (data.mfa_required) {
    return { mfa_required: true, mfa_token: data.mfa_token as string };
  }
  setSession(data.token as string, (data.role as UserRole) ?? "administrator", data.username as string);
  if (data.mfa_setup_required) {
    localStorage.setItem("datasafe_mfa_setup_required", "1");
  } else {
    localStorage.removeItem("datasafe_mfa_setup_required");
  }
  try {
    await refreshSessionFromMe();
  } catch {
    /* profile enrichment optional */
  }
  return {};
}

export async function exchangeOidcCode(exchangeCode: string): Promise<{ token: string; auth_source?: string }> {
  const res = await fetch(`${API_BASE}/auth/oidc/exchange`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ exchange_code: exchangeCode }),
  });
  const text = await res.text();
  let data: Record<string, unknown> = {};
  try {
    data = JSON.parse(text);
  } catch {
    /* */
  }
  if (!res.ok) {
    throw new ApiError((data.error as string) || text || "OIDC exchange failed", res.status);
  }
  return { token: data.token as string, auth_source: data.auth_source as string | undefined };
}

export async function loginMFA(mfaToken: string, code: string): Promise<void> {
  const res = await fetch(`${API_BASE}/mfa/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ mfa_token: mfaToken, totp_code: code }),
  });
  if (!res.ok) {
    const text = await res.text();
    let message = text || "MFA login failed";
    try {
      const json = JSON.parse(text);
      if (json.error) message = json.error;
    } catch {
      /* */
    }
    throw new ApiError(message, res.status);
  }
  const data = await res.json();
  setSession(data.token, data.role ?? "administrator", data.username);
  try {
    await refreshSessionFromMe();
  } catch {
    /* profile enrichment optional */
  }
}

export async function logout(): Promise<{ oidc_logout_url?: string }> {
  try {
    const res = await fetch(`${API_BASE}/admin/logout`, {
      method: "POST",
      headers: {
        Authorization: getToken() ? `Bearer ${getToken()}` : "",
      },
    });
    if (res.status === 401) {
      clearToken();
      return {};
    }
    if (res.status === 204) {
      clearToken();
      return {};
    }
    if (!res.ok) {
      clearToken();
      return {};
    }
    const data = (await res.json()) as { oidc_logout_url?: string };
    clearToken();
    return data;
  } catch {
    clearToken();
    return {};
  }
}

export const api = {
  health: () => fetchJSON<{ status: string }>("/health"),

  listBuckets: (filter?: "owned" | "shared" | "tenant" | "all") => {
    const q = filter && filter !== "all" ? `?filter=${encodeURIComponent(filter)}` : "";
    return fetchJSON<{ buckets: Bucket[] }>(`/buckets${q}`);
  },
  createBucket: (name: string, opts?: { visibility?: string }) =>
    fetchJSON<{ bucket: string; visibility?: string }>(`/buckets/${encodeURIComponent(name)}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ visibility: opts?.visibility ?? "private" }),
    }),
  deleteBucket: (name: string) =>
    fetchJSON<void>(`/buckets/${encodeURIComponent(name)}`, { method: "DELETE" }),

  listObjects: (bucket: string, prefix?: string, delimiter?: string, opts?: { startAfter?: string; maxKeys?: number }) => {
    const q = new URLSearchParams();
    if (prefix) q.set("prefix", prefix);
    if (delimiter) q.set("delimiter", delimiter);
    if (opts?.startAfter) q.set("start_after", opts.startAfter);
    if (opts?.maxKeys) q.set("max_keys", String(opts.maxKeys));
    const qs = q.toString();
    return fetchJSON<{
      objects: ObjectRow[];
      folders?: string[];
      truncated?: boolean;
      next_marker?: string;
    }>(`/buckets/${encodeURIComponent(bucket)}/objects${qs ? `?${qs}` : ""}`);
  },

  listVersions: (bucket: string, prefix?: string) => {
    const q = prefix ? `?prefix=${encodeURIComponent(prefix)}` : "";
    return fetchJSON<{ versions: ObjectRow[] }>(
      `/buckets/${encodeURIComponent(bucket)}/versions${q}`
    );
  },

  getBucketSettings: (bucket: string) =>
    fetchJSON<BucketSettings>(`/buckets/${encodeURIComponent(bucket)}/settings`),

  createFolder: (bucket: string, name: string) =>
    fetchJSON<{ object: ObjectRow }>(`/buckets/${encodeURIComponent(bucket)}/folders`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name }),
    }),

  deleteFolder: (bucket: string, prefix: string, recursive: boolean) =>
    fetchJSON<{ deleted: number }>(`/buckets/${encodeURIComponent(bucket)}/folders`, {
      method: "DELETE",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ prefix, recursive }),
    }),

  bulkDeleteObjects: (bucket: string, keys: string[]) =>
    fetchJSON<{ deleted: number; errors: string[] }>(
      `/buckets/${encodeURIComponent(bucket)}/bulk-delete`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ keys }),
      }
    ),

  objectAction: (
    bucket: string,
    body: {
      action: "restore" | "copy" | "move" | "rename";
      key: string;
      dest_key?: string;
      dest_bucket?: string;
      version_id?: string;
    }
  ) =>
    fetchJSON<{ object: ObjectRow }>(`/buckets/${encodeURIComponent(bucket)}/object-actions`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),

  getObjectMeta: (bucket: string, key: string, versionId?: string) => {
    const q = new URLSearchParams({ key });
    if (versionId) q.set("versionId", versionId);
    return fetchJSON<{ object: ObjectRow }>(
      `/buckets/${encodeURIComponent(bucket)}/object-meta?${q}`
    );
  },

  downloadObjectUrl: (bucket: string, key: string, versionId?: string) => {
    const q = versionId ? `?versionId=${encodeURIComponent(versionId)}` : "";
    return `${API_BASE}/buckets/${encodeURIComponent(bucket)}/objects/${encodeURIComponent(key)}${q}`;
  },

  downloadObject: async (bucket: string, key: string, versionId?: string) => {
    const token = getToken();
    const q = versionId ? `?versionId=${encodeURIComponent(versionId)}` : "";
    const res = await fetch(
      `${API_BASE}/buckets/${encodeURIComponent(bucket)}/objects/${encodeURIComponent(key)}${q}`,
      { headers: token ? { Authorization: `Bearer ${token}` } : {} }
    );
    if (!res.ok) throw new ApiError(await res.text(), res.status);
    return res.blob();
  },

  presign: (body: {
    method?: string;
    bucket: string;
    key: string;
    expires_seconds: number;
    endpoint?: string;
  }) =>
    fetchJSON<{ url: string }>("/presign", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),

  putLifecycle: (bucket: string, rules: LifecycleRule[]) =>
    fetchJSON<{ ok: boolean }>(`/buckets/${encodeURIComponent(bucket)}/lifecycle`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ rules }),
    }),

  uploadObject: async (bucket: string, key: string, file: File, onProgress?: (pct: number) => void) => {
    const token = getToken();
    const headers: Record<string, string> = {
      "Content-Type": file.type || "application/octet-stream",
    };
    if (token) headers.Authorization = `Bearer ${token}`;

    return new Promise<ObjectRow>((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      xhr.open("PUT", `${API_BASE}/buckets/${encodeURIComponent(bucket)}/objects/${encodeURIComponent(key)}`);
      Object.entries(headers).forEach(([k, v]) => xhr.setRequestHeader(k, v));
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable && onProgress) {
          onProgress(Math.round((e.loaded / e.total) * 100));
        }
      };
      xhr.onload = () => {
        if (xhr.status === 401) {
          clearToken();
          window.dispatchEvent(new Event("datasafe:unauthorized"));
          reject(new ApiError("Session expired.", 401));
          return;
        }
        if (xhr.status >= 200 && xhr.status < 300) {
          try {
            const data = JSON.parse(xhr.responseText);
            resolve(data.object ?? { key, size: file.size, etag: "", last_modified: new Date().toISOString() });
          } catch {
            resolve({ key, size: file.size, etag: "", last_modified: new Date().toISOString() });
          }
          return;
        }
        reject(new ApiError(xhr.responseText || "Upload failed", xhr.status));
      };
      xhr.onerror = () => reject(new ApiError("Upload failed", 0));
      xhr.send(file);
    });
  },

  deleteObject: (bucket: string, key: string, options?: { schedule?: "1d" | "1w" | "1m"; versionId?: string }) => {
    const q = new URLSearchParams();
    if (options?.schedule) q.set("schedule", options.schedule);
    if (options?.versionId) q.set("versionId", options.versionId);
    const qs = q.toString();
    return fetchJSON<void | { scheduled_delete_at: string }>(
      `/buckets/${encodeURIComponent(bucket)}/objects/${encodeURIComponent(key)}${qs ? `?${qs}` : ""}`,
      { method: "DELETE" }
    );
  },

  listKeys: () => fetchJSON<{ keys: AccessKey[] }>("/keys"),
  createKey: (label: string) =>
    fetchJSON<CreatedKey>("/keys", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ label }),
    }),
  deleteKey: (accessKey: string) =>
    fetchJSON<void>(`/keys/${encodeURIComponent(accessKey)}`, { method: "DELETE" }),

  getPolicy: (bucket: string) =>
    fetchJSON<{ policy: string }>(`/buckets/${encodeURIComponent(bucket)}/policy`),
  putPolicy: (bucket: string, policy: string) =>
    fetchJSON<{ ok: boolean }>(`/buckets/${encodeURIComponent(bucket)}/policy`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ policy }),
    }),

  getLifecycle: (bucket: string) =>
    fetchJSON<{ rules: unknown[] }>(`/buckets/${encodeURIComponent(bucket)}/lifecycle`),

  me: () =>
    fetchJSON<{
      username: string;
      role: UserRole;
      user_id: string;
      email?: string;
      status?: string;
      tenant_id?: string;
      tenant_memberships?: TenantMembership[];
      is_tenant_admin?: boolean;
      auth_source?: string;
    }>("/me"),

  listActivity: (params?: {
    period?: string;
    user?: string;
    action?: string;
    bucket?: string;
    ip?: string;
    search?: string;
    offset?: number;
    limit?: number;
  }) => {
    const q = new URLSearchParams();
    if (params?.period) q.set("period", params.period);
    if (params?.user) q.set("user", params.user);
    if (params?.action) q.set("action", params.action);
    if (params?.bucket) q.set("bucket", params.bucket);
    if (params?.ip) q.set("ip", params.ip);
    if (params?.search) q.set("search", params.search);
    if (params?.offset != null) q.set("offset", String(params.offset));
    if (params?.limit != null) q.set("limit", String(params.limit));
    const qs = q.toString();
    return fetchJSON<{ events: ActivityEvent[]; total: number }>(
      `/activity${qs ? `?${qs}` : ""}`
    );
  },

  getUsage: () => fetchJSON<UsageResponse>("/usage"),

  listUsers: () => fetchJSON<{ users: ConsoleUser[] }>("/users"),
  createUser: (body: {
    username: string;
    email: string;
    password: string;
    role: UserRole;
    status?: string;
  }) =>
    fetchJSON<{ user_id: string }>("/users", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  updateUser: (
    id: string,
    body: {
      email?: string;
      role?: UserRole;
      status?: string;
      max_size_bytes?: number;
      max_objects?: number;
    }
  ) =>
    fetchJSON<{ ok: boolean }>(`/users/${encodeURIComponent(id)}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  deleteUser: (id: string) =>
    fetchJSON<void>(`/users/${encodeURIComponent(id)}`, { method: "DELETE" }),
  resetPassword: (id: string, password: string) =>
    fetchJSON<{ ok: boolean }>(`/users/${encodeURIComponent(id)}/reset-password`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ password }),
    }),

  listBucketSettings: () => fetchJSON<{ buckets: BucketSettings[] }>("/settings/buckets"),
  updateBucketSettings: (name: string, body: Partial<BucketSettings>) =>
    fetchJSON<{ ok: boolean }>(`/settings/buckets/${encodeURIComponent(name)}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  batchUpdateBucketSettings: async (
    updates: { name: string; body: Partial<BucketSettings> }[]
  ): Promise<{ ok: number; failed: { name: string; error: string }[] }> => {
    const results = await Promise.allSettled(
      updates.map(({ name, body }) =>
        fetchJSON<{ ok: boolean }>(`/settings/buckets/${encodeURIComponent(name)}`, {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        })
      )
    );
    let ok = 0;
    const failed: { name: string; error: string }[] = [];
    results.forEach((result, i) => {
      if (result.status === "fulfilled") {
        ok += 1;
      } else {
        const err = result.reason;
        failed.push({
          name: updates[i].name,
          error: err instanceof Error ? err.message : String(err),
        });
      }
    });
    return { ok, failed };
  },

  getSystemConfig: () => fetchJSON<SystemConfig>("/settings/system"),
  updateSystemConfig: (body: Partial<SystemConfig>) =>
    fetchJSON<{ ok: boolean }>("/settings/system", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),

  getSetupStatus: () => fetchJSON<SetupStatus>("/setup/status"),
  testSetupS3: (body: ExternalS3Config) =>
    fetchJSON<{ ok: boolean; message: string }>("/setup/s3/test", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  saveSetupS3: (body: ExternalS3Config) =>
    fetchJSON<{ initial_setup_completed: boolean; needs_setup: boolean }>("/setup/s3/save", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  completeSetup: () =>
    fetchJSON<{ initial_setup_completed: boolean; needs_setup: boolean }>("/setup/complete", {
      method: "POST",
    }),

  listTrash: (bucket?: string) => {
    const q = bucket ? `?bucket=${encodeURIComponent(bucket)}` : "";
    return fetchJSON<{ items: TrashItem[] }>(`/trash${q}`);
  },
  restoreTrash: (id: string) =>
    fetchJSON<{ object: ObjectRow }>(`/trash/${encodeURIComponent(id)}/restore`, { method: "POST" }),
  purgeTrash: (id: string) =>
    fetchJSON<void>(`/trash/${encodeURIComponent(id)}`, { method: "DELETE" }),

  listAPITokens: () => fetchJSON<{ tokens: APIToken[] }>("/tokens"),
  createAPIToken: (body: { name: string; expires_days?: number; scopes?: string[] }) =>
    fetchJSON<{ id: string; name: string; token: string; scopes: string[]; expires_at: string }>("/tokens", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  deleteAPIToken: (id: string) =>
    fetchJSON<void>(`/tokens/${encodeURIComponent(id)}`, { method: "DELETE" }),

  listWebhooks: () => fetchJSON<{ webhooks: Webhook[] }>("/webhooks"),
  createWebhook: (body: { name?: string; url: string; events?: string[]; headers?: Record<string, string>; enabled?: boolean }) =>
    fetchJSON<{ webhook: Webhook }>("/webhooks", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  updateWebhook: (id: string, body: Partial<Webhook>) =>
    fetchJSON<{ webhook: Webhook }>(`/webhooks/${encodeURIComponent(id)}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  deleteWebhook: (id: string) =>
    fetchJSON<void>(`/webhooks/${encodeURIComponent(id)}`, { method: "DELETE" }),
  webhookTemplates: () =>
    fetchJSON<{ templates: { name: string; url: string }[] }>("/webhooks/templates"),
  listWebhookDeliveries: (id: string, limit?: number) => {
    const q = limit ? `?limit=${limit}` : "";
    return fetchJSON<{ deliveries: WebhookDelivery[] }>(`/webhooks/${encodeURIComponent(id)}/deliveries${q}`);
  },
  retryWebhookDelivery: (webhookId: string, deliveryId: string) =>
    fetchJSON<{ ok: boolean }>(
      `/webhooks/${encodeURIComponent(webhookId)}/deliveries/${encodeURIComponent(deliveryId)}/retry`,
      { method: "POST" }
    ),

  search: (q: string, offset?: number, limit?: number) => {
    const params = new URLSearchParams({ q });
    if (offset != null) params.set("offset", String(offset));
    if (limit != null) params.set("limit", String(limit));
    return fetchJSON<{ results: SearchResult[]; total: number; offset: number; limit: number }>(
      `/search?${params}`
    );
  },

  listFavorites: () => fetchJSON<{ favorites: Favorite[] }>("/favorites"),
  createFavorite: (body: { type?: string; bucket: string; prefix?: string }) =>
    fetchJSON<{ favorite: Favorite }>("/favorites", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  deleteFavorite: (id: string) =>
    fetchJSON<void>(`/favorites/${encodeURIComponent(id)}`, { method: "DELETE" }),

  getBucketTags: (bucket: string) =>
    fetchJSON<{ tags: Record<string, string> }>(`/buckets/${encodeURIComponent(bucket)}/tags`),
  putBucketTags: (bucket: string, tags: Record<string, string>) =>
    fetchJSON<{ tags: Record<string, string> }>(`/buckets/${encodeURIComponent(bucket)}/tags`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ tags }),
    }),
  getObjectTags: (bucket: string, key: string, versionId?: string) => {
    const q = new URLSearchParams({ key });
    if (versionId) q.set("versionId", versionId);
    return fetchJSON<{ tags: Record<string, string> }>(
      `/buckets/${encodeURIComponent(bucket)}/object-tags?${q}`
    );
  },
  putObjectTags: (bucket: string, key: string, tags: Record<string, string>, versionId?: string) => {
    const q = new URLSearchParams({ key });
    if (versionId) q.set("versionId", versionId);
    return fetchJSON<{ tags: Record<string, string> }>(
      `/buckets/${encodeURIComponent(bucket)}/object-tags?${q}`,
      { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ tags }) }
    );
  },
  putObjectMeta: (
    bucket: string,
    key: string,
    body: { metadata?: Record<string, string>; content_type?: string; tags?: Record<string, string> },
    versionId?: string
  ) => {
    const q = new URLSearchParams({ key });
    if (versionId) q.set("versionId", versionId);
    return fetchJSON<{ object: ObjectRow }>(
      `/buckets/${encodeURIComponent(bucket)}/object-meta?${q}`,
      { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) }
    );
  },

  initiateMultipart: (bucket: string, key: string, contentType?: string) =>
    fetchJSON<{ upload_id: string; bucket: string; key: string }>(
      `/buckets/${encodeURIComponent(bucket)}/multipart`,
      { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ key, content_type: contentType }) }
    ),
  uploadMultipartPart: async (
    bucket: string,
    uploadId: string,
    partNumber: number,
    blob: Blob,
    onProgress?: (loaded: number) => void
  ) => {
    const token = getToken();
    return new Promise<{ etag: string; part_number: number }>((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      xhr.open(
        "PUT",
        `${API_BASE}/buckets/${encodeURIComponent(bucket)}/multipart/${encodeURIComponent(uploadId)}/parts/${partNumber}`
      );
      if (token) xhr.setRequestHeader("Authorization", `Bearer ${token}`);
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable && onProgress) onProgress(e.loaded);
      };
      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          try {
            resolve(JSON.parse(xhr.responseText));
          } catch {
            reject(new ApiError("Invalid response", xhr.status));
          }
        } else {
          reject(new ApiError(xhr.responseText || "Part upload failed", xhr.status));
        }
      };
      xhr.onerror = () => reject(new ApiError("Part upload failed", 0));
      xhr.send(blob);
    });
  },
  completeMultipart: (
    bucket: string,
    uploadId: string,
    parts: { part_number: number; etag: string }[]
  ) =>
    fetchJSON<{ object: ObjectRow }>(
      `/buckets/${encodeURIComponent(bucket)}/multipart/${encodeURIComponent(uploadId)}/complete`,
      { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ parts }) }
    ),
  abortMultipart: (bucket: string, uploadId: string) =>
    fetchJSON<void>(`/buckets/${encodeURIComponent(bucket)}/multipart/${encodeURIComponent(uploadId)}`, {
      method: "DELETE",
    }),

  getOIDCConfig: () =>
    fetchJSON<{ enabled: boolean; issuer?: string; issuer_reachable?: boolean; issuer_error?: string }>(
      "/auth/oidc/config"
    ),
  exchangeOidcCode: (exchangeCode: string) => exchangeOidcCode(exchangeCode),
  testLDAP: (body: LDAPConfig) =>
    fetchJSON<{ ok: boolean; message?: string; error?: string }>("/settings/ldap/test", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  syncLDAP: () =>
    fetchJSON<{ synced: number; created?: number; updated?: number; suspended?: number; total_found: number }>(
      "/settings/ldap/sync",
      { method: "POST" },
    ),
  getSecurityStatus: () => fetchJSON<SecurityStatus>("/settings/security-status"),
  mfaEnroll: () =>
    fetchJSON<{ secret: string; otpauth_uri: string; qr_url: string; qr_code: string }>("/mfa/enroll", { method: "POST" }),
  mfaVerify: (code: string) =>
    fetchJSON<{ ok: boolean; recovery_codes: string[] }>("/mfa/verify-enroll", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code }),
    }),
  mfaDisable: (password: string, code: string) =>
    fetchJSON<{ ok: boolean }>("/mfa/disable", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ password, code }),
    }),
  webauthnRegisterBegin: () =>
    fetchJSON<PublicKeyCredentialCreationOptionsJSON>("/me/mfa/webauthn/register/begin", { method: "POST" }),
  webauthnRegisterFinish: (credential: unknown) =>
    fetchJSON<{ ok: boolean; passkeys: number }>("/me/mfa/webauthn/register/finish", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(credential),
    }),
  setLegalHold: (bucket: string, key: string, hold: boolean, versionId?: string) =>
    fetchJSON<{ ok: boolean }>(`/buckets/${encodeURIComponent(bucket)}/legal-hold`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ key, hold, version_id: versionId }),
    }),
  listTenants: () => fetchJSON<{ tenants: Tenant[] }>("/tenants"),
  createTenant: (name: string) =>
    fetchJSON<{ tenant: Tenant }>("/tenants", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name }),
    }),
  deleteTenant: (id: string) => fetchJSON<void>(`/tenants/${encodeURIComponent(id)}`, { method: "DELETE" }),
  listTenantMembers: (tenantId: string) =>
    fetchJSON<{ members: TenantMember[] }>(`/tenants/${encodeURIComponent(tenantId)}/members`),
  addTenantMember: (tenantId: string, body: { user_id: string; role?: string; group_ids?: string[] }) =>
    fetchJSON<{ member: TenantMember }>(`/tenants/${encodeURIComponent(tenantId)}/members`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  createTenantUser: (
    tenantId: string,
    body: { username: string; password: string; email?: string; role?: TenantRole; group_ids?: string[] }
  ) =>
    fetchJSON<{ user: ConsoleUser; member: TenantMember }>(
      `/tenants/${encodeURIComponent(tenantId)}/users`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      }
    ),
  updateTenantMember: (tenantId: string, userId: string, role: string) =>
    fetchJSON<{ ok: boolean }>(`/tenants/${encodeURIComponent(tenantId)}/members/${encodeURIComponent(userId)}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ role }),
    }),
  removeTenantMember: (tenantId: string, userId: string) =>
    fetchJSON<void>(`/tenants/${encodeURIComponent(tenantId)}/members/${encodeURIComponent(userId)}`, {
      method: "DELETE",
    }),
  listTenantGroups: (tenantId: string) =>
    fetchJSON<{ groups: TenantGroup[] }>(`/tenants/${encodeURIComponent(tenantId)}/groups`),
  listTenantBuckets: (tenantId: string) =>
    fetchJSON<{ buckets: Bucket[]; tenant_id: string }>(`/tenants/${encodeURIComponent(tenantId)}/buckets`),
  getTenantGroup: (tenantId: string, groupId: string) =>
    fetchJSON<{ group: TenantGroup; bucket_keys: string[]; member_ids: string[] }>(
      `/tenants/${encodeURIComponent(tenantId)}/groups/${encodeURIComponent(groupId)}`
    ),
  createTenantGroup: (
    tenantId: string,
    body: { name: string; external_name?: string; description?: string; access_level?: "read" | "read_write" }
  ) =>
    fetchJSON<{ group: TenantGroup }>(`/tenants/${encodeURIComponent(tenantId)}/groups`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  updateTenantGroup: (
    tenantId: string,
    groupId: string,
    body: { name?: string; external_name?: string; description?: string; access_level?: "read" | "read_write" }
  ) =>
    fetchJSON<{ group: TenantGroup }>(
      `/tenants/${encodeURIComponent(tenantId)}/groups/${encodeURIComponent(groupId)}`,
      { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) }
    ),
  deleteTenantGroup: (tenantId: string, groupId: string) =>
    fetchJSON<void>(`/tenants/${encodeURIComponent(tenantId)}/groups/${encodeURIComponent(groupId)}`, {
      method: "DELETE",
    }),
  setTenantGroupBuckets: (tenantId: string, groupId: string, bucketKeys: string[]) =>
    fetchJSON<{ ok: boolean; bucket_keys: string[] }>(
      `/tenants/${encodeURIComponent(tenantId)}/groups/${encodeURIComponent(groupId)}/buckets`,
      { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ bucket_keys: bucketKeys }) }
    ),
  setMemberGroups: (tenantId: string, userId: string, groupIds: string[]) =>
    fetchJSON<{ ok: boolean; group_ids: string[] }>(
      `/tenants/${encodeURIComponent(tenantId)}/members/${encodeURIComponent(userId)}/groups`,
      { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ group_ids: groupIds }) }
    ),
  listBucketAccess: (tenantId: string, bucket: string) =>
    fetchJSON<{ grants: BucketAccessGrant[]; prefix_grants?: BucketAccessGrant[]; bucket: string; tenant_id: string }>(
      `/tenants/${encodeURIComponent(tenantId)}/buckets/${encodeURIComponent(bucket)}/access`
    ),
  listBucketAccessByBucket: (bucket: string) =>
    fetchJSON<{ grants: BucketAccessGrant[]; prefix_grants?: BucketAccessGrant[]; bucket: string; tenant_id: string }>(
      `/buckets/${encodeURIComponent(bucket)}/access`
    ),
  putBucketAccess: (
    tenantId: string,
    bucket: string,
    body: {
      grants: { user_id: string; can_read: boolean; can_write: boolean }[];
      prefix_grants?: { user_id: string; prefix: string; can_read: boolean; can_write: boolean }[];
    }
  ) =>
    fetchJSON<{ ok: boolean; grants: number; prefix_grants?: number }>(
      `/tenants/${encodeURIComponent(tenantId)}/buckets/${encodeURIComponent(bucket)}/access`,
      {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      }
    ),
  putBucketAccessByBucket: (
    bucket: string,
    body: {
      grants: { user_id: string; can_read: boolean; can_write: boolean }[];
      prefix_grants?: { user_id: string; prefix: string; can_read: boolean; can_write: boolean }[];
    }
  ) =>
    fetchJSON<{ ok: boolean; grants: number; prefix_grants?: number }>(
      `/buckets/${encodeURIComponent(bucket)}/access`,
      {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      }
    ),
  listNotifications: () =>
    fetchJSON<{ notifications: UserNotification[]; unread: number }>("/notifications"),
  markNotificationRead: (id: string) =>
    fetchJSON<void>(`/notifications/${encodeURIComponent(id)}/read`, { method: "POST" }),
  markAllNotificationsRead: () =>
    fetchJSON<void>("/notifications/read-all", { method: "POST" }),
  listRecent: () => fetchJSON<{ items: RecentItem[] }>("/recent"),
  listShareableUsers: (bucket: string, q?: string) => {
    const params = new URLSearchParams({ bucket });
    if (q?.trim()) params.set("q", q.trim());
    return fetchJSON<{ users: BucketAccessGrant[]; bucket: string }>(
      `/shareable-users?${params.toString()}`
    );
  },
  listSharedLinks: (bucket: string, key?: string) => {
    const q = key ? `?key=${encodeURIComponent(key)}` : "";
    return fetchJSON<{ shares: SharedLink[] }>(`/buckets/${encodeURIComponent(bucket)}/shares${q}`);
  },
  createSharedLink: (bucket: string, body: { key: string; expires_in_sec?: number; max_downloads?: number }) =>
    fetchJSON<{ share: SharedLink; url: string }>(`/buckets/${encodeURIComponent(bucket)}/shares`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  revokeSharedLink: (id: string) =>
    fetchJSON<void>(`/shares/${encodeURIComponent(id)}`, { method: "DELETE" }),
  listGatewayConnections: () => fetchJSON<{ connections: GatewayConnection[] }>("/gateway/connections"),
  createGatewayConnection: (body: Partial<GatewayConnection> & { secret_key?: string }) =>
    fetchJSON<{ connection: GatewayConnection }>("/gateway/connections", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  deleteGatewayConnection: (id: string) =>
    fetchJSON<void>(`/gateway/connections/${encodeURIComponent(id)}`, { method: "DELETE" }),
  testGatewayConnection: (id: string) =>
    fetchJSON<{ ok: boolean; message: string; status: string }>(
      `/gateway/connections/${encodeURIComponent(id)}/test`,
      { method: "POST" }
    ),
  listReplicationRules: () => fetchJSON<{ rules: ReplicationRule[] }>("/gateway/replication"),
  createReplicationRule: (body: Partial<ReplicationRule>) =>
    fetchJSON<{ rule: ReplicationRule }>("/gateway/replication", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  deleteReplicationRule: (id: string) =>
    fetchJSON<void>(`/gateway/replication/${encodeURIComponent(id)}`, { method: "DELETE" }),
  triggerSyncJob: (ruleId: string) =>
    fetchJSON<{ job: SyncJob }>(`/gateway/replication/${encodeURIComponent(ruleId)}/sync`, { method: "POST" }),
  listSyncJobs: (ruleId?: string) => {
    const q = ruleId ? `?rule_id=${encodeURIComponent(ruleId)}` : "";
    return fetchJSON<{ jobs: SyncJob[] }>(`/gateway/sync-jobs${q}`);
  },
  gatewayHealth: () => fetchJSON<GatewayHealth>("/gateway/health"),
  listReplicationQueue: (status?: string) => {
    const q = status ? `?status=${encodeURIComponent(status)}` : "";
    return fetchJSON<{ tasks: ReplicationTask[] }>(`/gateway/replication/queue${q}`);
  },
  retryFailedReplication: () =>
    fetchJSON<{ retried: number }>("/gateway/replication/retry-failed", { method: "POST" }),
  clearReplicationErrors: () =>
    fetchJSON<{ cleared: boolean }>("/gateway/replication/clear-errors", { method: "POST" }),
  listFederationClusters: () => fetchJSON<{ clusters: FederationCluster[] }>("/federation/clusters"),
  createFederationCluster: (body: { name: string; endpoint: string; region?: string }) =>
    fetchJSON<{ cluster: FederationCluster }>("/federation/clusters", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  deleteFederationCluster: (id: string) =>
    fetchJSON<void>(`/federation/clusters/${encodeURIComponent(id)}`, { method: "DELETE" }),
  testFederationCluster: (id: string) =>
    fetchJSON<{ status: string; detail?: string }>(`/federation/clusters/${encodeURIComponent(id)}/test`, {
      method: "POST",
    }),
  clusterStatus: () =>
    fetchJSON<{
      distributed_mode: boolean;
      erasure_coding_planned: boolean;
      disk_paths: string[];
      nodes: ClusterNode[];
    }>("/cluster/status"),
  getMe: () =>
    fetchJSON<{
      username: string;
      email?: string;
      role: UserRole;
      user_id: string;
      tenant_id?: string;
      locale?: string;
      mfa_enabled?: boolean;
      mfa_setup_required?: boolean;
      webauthn_enabled?: boolean;
      passkey_count?: number;
      auth_source?: string;
      tenant_memberships?: TenantMembership[];
      is_tenant_admin?: boolean;
    }>("/me"),

  updateLocale: (locale: string) =>
    fetchJSON<{ ok: boolean }>("/me/locale", {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ locale }),
    }),

  changePassword: (current_password: string, new_password: string) =>
    fetchJSON<{ ok: boolean }>("/me/password", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ current_password, new_password }),
    }),
};

export async function uploadObjectMultipart(
  bucket: string,
  key: string,
  file: File,
  onProgress?: (p: MultipartUploadProgress) => void
) {
  const partSize = MULTIPART_PART_SIZE;
  const partsTotal = Math.ceil(file.size / partSize);
  const { upload_id: uploadId } = await api.initiateMultipart(bucket, key, file.type || "application/octet-stream");
  const parts: { part_number: number; etag: string }[] = [];
  let loaded = 0;
  const startTime = Date.now();
  try {
    for (let i = 0; i < partsTotal; i++) {
      const start = i * partSize;
      const end = Math.min(start + partSize, file.size);
      const blob = file.slice(start, end);
      const partStart = loaded;
      const res = await api.uploadMultipartPart(bucket, uploadId, i + 1, blob, (partLoaded) => {
        const nowLoaded = partStart + partLoaded;
        const elapsed = (Date.now() - startTime) / 1000;
        const speed = elapsed > 0 ? nowLoaded / elapsed : 0;
        const remaining = file.size - nowLoaded;
        onProgress?.({
          loaded: nowLoaded,
          total: file.size,
          partsDone: i,
          partsTotal,
          speed,
          eta: speed > 0 ? remaining / speed : 0,
        });
      });
      loaded += blob.size;
      parts.push({ part_number: res.part_number, etag: res.etag });
      const elapsed = (Date.now() - startTime) / 1000;
      const speed = elapsed > 0 ? loaded / elapsed : 0;
      onProgress?.({
        loaded,
        total: file.size,
        partsDone: i + 1,
        partsTotal,
        speed,
        eta: speed > 0 ? (file.size - loaded) / speed : 0,
      });
    }
    return await api.completeMultipart(bucket, uploadId, parts);
  } catch (err) {
    try {
      await api.abortMultipart(bucket, uploadId);
    } catch {
      /* ignore */
    }
    throw err;
  }
}

export async function fetchMetricsSummary(): Promise<{
  buckets: number;
  storageBytes: number;
}> {
  try {
    const res = await fetch("/metrics");
    if (!res.ok) throw new Error("metrics unavailable");
    const text = await res.text();
    const buckets = parsePromGauge(text, "datasafe_buckets_total");
    const storageBytes = parsePromGauge(text, "datasafe_storage_bytes");
    return { buckets, storageBytes };
  } catch {
    return { buckets: 0, storageBytes: 0 };
  }
}

function parsePromGauge(text: string, name: string): number {
  const re = new RegExp(`^${name}\\s+(\\S+)`, "m");
  const match = text.match(re);
  return match ? parseFloat(match[1]) : 0;
}
