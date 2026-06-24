import { Outlet } from "react-router-dom";
import { Sidebar } from "@/layouts/sidebar";
import { GlobalSearch } from "@/components/global-search";
import { NotificationBell } from "@/components/notification-bell";
import { LanguageSwitcher } from "@/components/language-switcher";
import { SecurityBanner } from "@/components/security-banner";
import { MfaSetupBanner } from "@/components/mfa-setup-banner";

export function AppLayout() {
  return (
    <div className="flex h-screen overflow-hidden">
      <Sidebar />
      <main className="flex-1 overflow-y-auto">
        <SecurityBanner />
        <MfaSetupBanner />
        <div className="flex items-center justify-between gap-4 border-b px-6 py-3 lg:px-8">
          <GlobalSearch />
          <div className="flex items-center gap-2">
            <NotificationBell />
            <LanguageSwitcher />
          </div>
        </div>
        <div className="mx-auto max-w-6xl p-6 lg:p-8">
          <Outlet />
        </div>
      </main>
    </div>
  );
}
