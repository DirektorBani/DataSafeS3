import { invoke } from "@tauri-apps/api/core";

export type AuthMethod = "password" | "oidc" | "api_token" | "access_key";

export type ServerCard = {
  profile?: string;
  warning?: string;
  deprecation?: string;
  blocked?: boolean;
  rejected?: boolean;
  browse_allowed?: boolean;
  advisory_url?: string;
  server_version?: string;
  login_methods?: string[];
  expires_soon?: boolean;
  mfa_setup?: boolean;
};

export type Profile = {
  id: string;
  name: string;
  server_url: string;
  auth_method: AuthMethod;
  username?: string;
  tls?: { insecure_dev?: boolean };
  session?: string;
  server?: ServerCard;
};

export type LoginView = {
  status?: string;
  message?: string;
  username?: string;
  secrets_persisted?: boolean;
  password_revoked?: boolean;
  mfa_setup?: boolean;
  server?: ServerCard;
};

export async function listProfiles(): Promise<Profile[]> {
  const result = await invoke<{ profiles: Profile[] }>("profile_list");
  return result.profiles ?? [];
}

export function saveProfile(profile: {
  name: string;
  server_url: string;
  auth_method: AuthMethod;
  username?: string;
  tls?: { insecure_dev?: boolean };
}): Promise<{ profile: Profile }> {
  return invoke("profile_save", { profile });
}

export function forgetProfile(id: string): Promise<unknown> {
  return invoke("profile_forget", { id });
}

export function serverInfo(id: string): Promise<{ server: ServerCard }> {
  return invoke("server_info", { id });
}

export function login(id: string, username: string, password: string, remember: boolean): Promise<LoginView> {
  return invoke("auth_login", { id, username, password, remember });
}

export function confirmMfa(id: string, code: string): Promise<LoginView> {
  return invoke("auth_mfa", { id, code });
}

export function logout(id: string): Promise<LoginView> {
  return invoke("auth_logout", { id });
}

export function oidcPassword(id: string, username: string, password: string, remember: boolean): Promise<LoginView> {
  return invoke("auth_oidc_password", { id, username, password, remember });
}

export function oidcExchange(id: string, exchangeCode: string): Promise<LoginView> {
  return invoke("auth_oidc_exchange", { id, exchangeCode });
}

export function saveApiToken(id: string, token: string): Promise<LoginView> {
  return invoke("auth_api_token", { id, token });
}

export function saveAccessKey(id: string, secret: string): Promise<LoginView> {
  return invoke("auth_access_key", { id, secret });
}
