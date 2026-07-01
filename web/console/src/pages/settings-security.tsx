import { PageHeader } from "@/components/page-header";
import { SecurityStatusPanel } from "@/components/settings/SecurityStatusPanel";
import { useTranslation } from "react-i18next";

export function SecuritySettingsPage() {
  const { t } = useTranslation("settings");

  return (
    <div className="space-y-4">
      <PageHeader title={t("security.title")} description={t("security.description")} />
      <SecurityStatusPanel />
    </div>
  );
}
