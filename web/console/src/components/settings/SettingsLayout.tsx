import { Outlet, useLocation, useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { Tabs, TabsList, TabsTrigger } from "@datasafe/ui/ui/tabs";

export function SettingsLayout() {
  const { t } = useTranslation("settings");
  const { pathname } = useLocation();
  const navigate = useNavigate();
  const tab = pathname.includes("/system")
    ? "system"
    : pathname.includes("/security")
      ? "security"
      : pathname.includes("/events")
        ? "events"
        : pathname.includes("/kms")
          ? "kms"
          : pathname.includes("/migrate")
            ? "migrate"
            : "buckets";

  return (
    <div className="space-y-4">
      <Tabs
        value={tab}
        onValueChange={(value) => {
          if (value === "system") {
            navigate("/admin/settings/system");
          } else if (value === "security") {
            navigate("/admin/settings/security");
          } else if (value === "events") {
            navigate("/admin/settings/events");
          } else if (value === "kms") {
            navigate("/admin/settings/kms");
          } else if (value === "migrate") {
            navigate("/admin/settings/migrate");
          } else {
            navigate("/admin/settings/buckets");
          }
        }}
      >
        <TabsList>
          <TabsTrigger value="buckets">{t("tabs.buckets")}</TabsTrigger>
          <TabsTrigger value="system">{t("tabs.system")}</TabsTrigger>
          <TabsTrigger value="security">{t("tabs.security")}</TabsTrigger>
          <TabsTrigger value="events">{t("tabs.events")}</TabsTrigger>
          <TabsTrigger value="kms" data-testid="settings-tab-kms">
            {t("tabs.kms")}
          </TabsTrigger>
          <TabsTrigger value="migrate" data-testid="settings-tab-migrate">
            {t("tabs.migrate")}
          </TabsTrigger>
        </TabsList>
      </Tabs>
      <Outlet />
    </div>
  );
}
