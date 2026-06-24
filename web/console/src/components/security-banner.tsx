import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { AlertTriangle } from "lucide-react";
import { api } from "@/lib/api";
import { useAuth } from "@/hooks/use-auth";

export function SecurityBanner() {
  const { t } = useTranslation("layout");
  const { isAdmin } = useAuth();
  const { data } = useQuery({
    queryKey: ["security-status"],
    queryFn: () => api.getSecurityStatus(),
    enabled: isAdmin,
    staleTime: 60_000,
  });

  if (!isAdmin || !data?.weak_secrets?.length) {
    return null;
  }

  return (
    <div className="border-b border-amber-500/40 bg-amber-500/10 px-6 py-2 text-sm text-amber-950 dark:text-amber-100 lg:px-8">
      <div className="flex items-start gap-2">
        <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
        <p>{t("securityBanner.message", { vars: data.weak_secrets.join(", ") })}</p>
      </div>
    </div>
  );
}
