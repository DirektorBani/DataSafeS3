import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { ShieldAlert } from "lucide-react";
import { api } from "@/lib/api";
import { Button } from "@datasafe/ui/ui/button";

export function MfaPolicyBanner() {
  const { t } = useTranslation("layout");
  const { data: me } = useQuery({
    queryKey: ["me"],
    queryFn: () => api.getMe(),
    staleTime: 15_000,
  });

  if (!me?.mfa_policy?.banner) {
    return null;
  }

  return (
    <div
      className="border-b border-amber-500/40 bg-amber-500/10 px-6 py-3 text-sm lg:px-8"
      data-testid="mfa-policy-banner"
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-start gap-2">
          <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0 text-amber-600" aria-hidden />
          <p>{t("mfaPolicyBanner.message")}</p>
        </div>
        <Button asChild size="sm" variant="outline">
          <Link to="/profile">{t("mfaPolicyBanner.action")}</Link>
        </Button>
      </div>
    </div>
  );
}
