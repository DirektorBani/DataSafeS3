import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import {
  canManageTenant,
  clearToken,
  getRole,
  getTenantMemberships,
  getUsername,
  isAuthenticated,
  isTenantAdminSession,
  logout as apiLogout,
  refreshSessionFromMe,
  type TenantMembership,
  type UserRole,
} from "@/lib/api";

type AuthContextValue = {
  authed: boolean;
  role: UserRole | null;
  username: string | null;
  isAdmin: boolean;
  isTenantAdmin: boolean;
  tenantMemberships: TenantMembership[];
  canManageTenant: (tenantId: string) => boolean;
  login: () => void;
  logout: () => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [authed, setAuthed] = useState(isAuthenticated);
  const [role, setRole] = useState<UserRole | null>(getRole);
  const [username, setUsername] = useState<string | null>(getUsername);
  const [tenantMemberships, setTenantMemberships] = useState<TenantMembership[]>(getTenantMemberships);
  const [isTenantAdmin, setIsTenantAdmin] = useState(isTenantAdminSession);

  const syncFromSession = useCallback(() => {
    setAuthed(isAuthenticated());
    setRole(getRole());
    setUsername(getUsername());
    setTenantMemberships(getTenantMemberships());
    setIsTenantAdmin(isTenantAdminSession());
  }, []);

  const login = useCallback(() => {
    syncFromSession();
  }, [syncFromSession]);

  const logout = useCallback(async () => {
    const result = await apiLogout();
    clearToken();
    setAuthed(false);
    setRole(null);
    setUsername(null);
    setTenantMemberships([]);
    setIsTenantAdmin(false);
    if (result.oidc_logout_url) {
      window.location.href = result.oidc_logout_url;
    }
  }, []);

  useEffect(() => {
    const onStorage = () => syncFromSession();
    window.addEventListener("storage", onStorage);
    return () => window.removeEventListener("storage", onStorage);
  }, [syncFromSession]);

  useEffect(() => {
    const onUnauthorized = () => {
      setAuthed(false);
      setRole(null);
      setUsername(null);
      setTenantMemberships([]);
      setIsTenantAdmin(false);
      if (!window.location.pathname.startsWith("/login") && !window.location.pathname.startsWith("/setup")) {
        window.location.replace("/login");
      }
    };
    window.addEventListener("datasafe:unauthorized", onUnauthorized);
    return () => window.removeEventListener("datasafe:unauthorized", onUnauthorized);
  }, []);

  useEffect(() => {
    if (!isAuthenticated()) return;
    void refreshSessionFromMe().then(syncFromSession).catch(() => {});
  }, [syncFromSession]);

  return (
    <AuthContext.Provider
      value={{
        authed,
        role,
        username,
        isAdmin: role === "administrator",
        isTenantAdmin,
        tenantMemberships,
        canManageTenant,
        login,
        logout,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
