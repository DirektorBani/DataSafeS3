import { Navigate, Route, Routes, useLocation } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/hooks/use-auth";
import { AppLayout } from "@/layouts/app-layout";
import { LoginPage } from "@/pages/login";
import { SetupPage } from "@/pages/setup";
import { DashboardPage } from "@/pages/dashboard";
import { BucketsPage } from "@/pages/buckets";
import { BucketDetailPage } from "@/pages/bucket-detail";
import { AccessKeysPage } from "@/pages/access-keys";
import { PolicyPage } from "@/pages/policy";
import { ActivityPage } from "@/pages/activity";
import { UsagePage } from "@/pages/usage";
import { UsersPage } from "@/pages/users";
import { WebhooksPage } from "@/pages/webhooks";
import { SettingsLayout } from "@/components/settings/SettingsLayout";
import { BucketSettingsPage } from "@/pages/settings-buckets";
import { AdministratorSettingsPage } from "@/pages/settings-system";
import { ProfilePage } from "@/pages/profile";
import { GatewayPage } from "@/pages/gateway";
import { FederationPage } from "@/pages/federation";
import { ClusterPage } from "@/pages/cluster";
import { TenantsPage } from "@/pages/tenants";
import { PublicSharePage } from "@/pages/public-share";
import { api, isMfaSetupRequired } from "@/lib/api";

function AdminRoute({ children }: { children: React.ReactNode }) {
  const { isAdmin } = useAuth();
  if (!isAdmin) return <Navigate to="/" replace />;
  return <>{children}</>;
}

function TenantAdminRoute({ children }: { children: React.ReactNode }) {
  const { isAdmin, isTenantAdmin } = useAuth();
  if (!isAdmin && !isTenantAdmin) return <Navigate to="/" replace />;
  return <>{children}</>;
}

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { authed } = useAuth();
  if (!authed) return <Navigate to="/login" replace />;
  return <>{children}</>;
}

function useSetupStatus() {
  return useQuery({
    queryKey: ["setup-status"],
    queryFn: () => api.getSetupStatus(),
    staleTime: 10_000,
  });
}

function RequireMfaSetup({ children }: { children: React.ReactNode }) {
  const location = useLocation();
  const { data: me, isLoading } = useQuery({
    queryKey: ["me"],
    queryFn: () => api.getMe(),
    staleTime: 15_000,
  });
  if (isLoading) return null;
  const required = isMfaSetupRequired() || me?.mfa_setup_required;
  if (required && !me?.mfa_enabled && location.pathname !== "/profile") {
    return <Navigate to="/profile" replace />;
  }
  return <>{children}</>;
}

function RequireSetupComplete({ children }: { children: React.ReactNode }) {
  const { isAdmin } = useAuth();
  const { data, isLoading } = useSetupStatus();
  if (isLoading) return null;
  if (isAdmin && data?.needs_setup) {
    return <Navigate to="/setup" replace />;
  }
  return <>{children}</>;
}

function PostLoginRedirect() {
  const { isAdmin } = useAuth();
  const { data, isLoading } = useSetupStatus();
  if (isLoading) return null;
  if (isAdmin && data?.needs_setup) {
    return <Navigate to="/setup" replace />;
  }
  return <Navigate to="/" replace />;
}

function SetupRoute() {
  const { data, isLoading } = useSetupStatus();
  if (isLoading) return null;
  if (!data?.needs_setup && !data?.needs_password_change) {
    return <Navigate to="/" replace />;
  }
  return <SetupPage />;
}

export function App() {
  const { authed } = useAuth();

  return (
    <Routes>
      <Route
        path="/login"
        element={authed ? <PostLoginRedirect /> : <LoginPage />}
      />
      <Route
        path="/setup"
        element={
          <ProtectedRoute>
            <SetupRoute />
          </ProtectedRoute>
        }
      />
      <Route path="/share/:token" element={<PublicSharePage />} />
      <Route
        element={
          <ProtectedRoute>
            <RequireSetupComplete>
              <RequireMfaSetup>
                <AppLayout />
              </RequireMfaSetup>
            </RequireSetupComplete>
          </ProtectedRoute>
        }
      >
        <Route index element={<DashboardPage />} />
        <Route path="buckets" element={<BucketsPage />} />
        <Route path="buckets/:bucketName" element={<BucketDetailPage />} />
        <Route path="keys" element={<AccessKeysPage />} />
        <Route path="usage" element={<UsagePage />} />
        <Route path="profile" element={<ProfilePage />} />
        <Route path="federation" element={<FederationPage />} />
        <Route path="cluster" element={<ClusterPage />} />
        <Route
          path="admin/policy"
          element={
            <AdminRoute>
              <PolicyPage />
            </AdminRoute>
          }
        />
        <Route
          path="admin/activity"
          element={
            <AdminRoute>
              <ActivityPage />
            </AdminRoute>
          }
        />
        <Route path="policy" element={<Navigate to="/admin/policy" replace />} />
        <Route path="activity" element={<Navigate to="/admin/activity" replace />} />
        <Route
          path="admin/users"
          element={
            <AdminRoute>
              <UsersPage />
            </AdminRoute>
          }
        />
        <Route
          path="admin/webhooks"
          element={
            <AdminRoute>
              <WebhooksPage />
            </AdminRoute>
          }
        />
        <Route
          path="admin/tenants"
          element={
            <TenantAdminRoute>
              <TenantsPage />
            </TenantAdminRoute>
          }
        />
        <Route
          path="gateway"
          element={
            <AdminRoute>
              <GatewayPage />
            </AdminRoute>
          }
        />
        <Route
          path="admin/gateway"
          element={<Navigate to="/gateway" replace />}
        />
        <Route
          path="admin/settings"
          element={
            <AdminRoute>
              <SettingsLayout />
            </AdminRoute>
          }
        >
          <Route index element={<Navigate to="buckets" replace />} />
          <Route path="buckets" element={<BucketSettingsPage />} />
          <Route path="system" element={<AdministratorSettingsPage />} />
        </Route>
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
