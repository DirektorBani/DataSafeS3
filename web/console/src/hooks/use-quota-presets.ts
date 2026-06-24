import { useMemo } from "react";
import { useTranslation } from "react-i18next";

export function useQuotaPresets() {
  const { t } = useTranslation("common");
  return useMemo(
    () =>
      [
        { label: t("quotaPresets.unlimited"), bytes: 0 },
        { label: t("quotaPresets.10gb"), bytes: 10 * 1024 * 1024 * 1024 },
        { label: t("quotaPresets.100gb"), bytes: 100 * 1024 * 1024 * 1024 },
        { label: t("quotaPresets.1tb"), bytes: 1024 * 1024 * 1024 * 1024 },
      ] as const,
    [t]
  );
}

export function useObjectQuotaPresets() {
  const { t } = useTranslation("common");
  return useMemo(
    () =>
      [
        { label: t("objectQuotaPresets.unlimited"), count: 0 },
        { label: t("objectQuotaPresets.1k"), count: 1000 },
        { label: t("objectQuotaPresets.10k"), count: 10000 },
        { label: t("objectQuotaPresets.100k"), count: 100000 },
        { label: t("objectQuotaPresets.1m"), count: 1000000 },
      ] as const,
    [t]
  );
}
