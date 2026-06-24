import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { ShieldAlert } from "lucide-react";
import { api, isMfaSetupRequired } from "@/lib/api";
import { Button } from "@/components/ui/button";

export function MfaSetupBanner() {
  const { t } = useTranslation("layout");
  const { data: me } = useQuery({
    queryKey: ["me"],
    queryFn: () => api.getMe(),
    staleTime: 30_000,
  });

  const required = isMfaSetupRequired() || me?.mfa_setup_required;
  if (!required || me?.mfa_enabled) {
    return null;
  }

  return (
    <div className="border-b border-destructive/40 bg-destructive/10 px-6 py-3 text-sm lg:px-8">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-start gap-2">
          <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0 text-destructive" aria-hidden />
          <p>{t("mfaSetupBanner.message")}</p>
        </div>
        <Button asChild size="sm" variant="destructive">
          <Link to="/profile">{t("mfaSetupBanner.action")}</Link>
        </Button>
      </div>
    </div>
  );
}
