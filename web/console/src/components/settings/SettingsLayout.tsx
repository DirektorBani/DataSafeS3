import { Outlet, useLocation, useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

export function SettingsLayout() {
  const { t } = useTranslation("settings");
  const { pathname } = useLocation();
  const navigate = useNavigate();
  const tab = pathname.includes("/system") ? "system" : "buckets";

  return (
    <div className="space-y-4">
      <Tabs
        value={tab}
        onValueChange={(value) => {
          navigate(value === "system" ? "/admin/settings/system" : "/admin/settings/buckets");
        }}
      >
        <TabsList>
          <TabsTrigger value="buckets">{t("tabs.buckets")}</TabsTrigger>
          <TabsTrigger value="system">{t("tabs.system")}</TabsTrigger>
        </TabsList>
      </Tabs>
      <Outlet />
    </div>
  );
}
