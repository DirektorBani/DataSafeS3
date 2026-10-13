import { PageHeader } from "@/components/page-header";
import { MfaPolicyCard } from "@/components/mfa-policy-card";
import { SecurityStatusPanel } from "@/components/settings/SecurityStatusPanel";
import { SharingPolicyCard } from "@/components/sharing-policy-card";
import { useTranslation } from "react-i18next";

export function SecuritySettingsPage() {
  const { t } = useTranslation("settings");

  return (
    <div className="space-y-4">
      <PageHeader title={t("sharing.title")} description={t("sharing.description")} />
      <SharingPolicyCard scope="global" canEdit />
      <MfaPolicyCard />
      <SecurityStatusPanel />
    </div>
  );
}
