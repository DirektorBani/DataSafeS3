import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { AlertTriangle, CheckCircle2 } from "lucide-react";
import { api } from "@/lib/api";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

export function SecurityStatusPanel() {
  const { t } = useTranslation("settings");
  const { data, isLoading, isError } = useQuery({
    queryKey: ["security-status"],
    queryFn: () => api.getSecurityStatus(),
    staleTime: 60_000,
  });

  const weak = data?.weak_secrets ?? [];
  const hasWeak = weak.length > 0;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("security.title")}</CardTitle>
        <CardDescription>{t("security.description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {isLoading && <p className="text-sm text-muted-foreground">{t("security.loading")}</p>}
        {isError && <p className="text-sm text-destructive">{t("security.loadError")}</p>}
        {!isLoading && !isError && data && (
          <>
            {hasWeak ? (
              <div className="flex items-start gap-2 rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-sm text-amber-950 dark:text-amber-100">
                <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
                <div className="space-y-2">
                  <p>{t("security.weakSecrets", { vars: weak.join(", ") })}</p>
                  <ul className="list-disc space-y-1 pl-4 text-muted-foreground">
                    <li>{t("security.recommendations.rotate")}</li>
                    <li>{t("security.recommendations.strict")}</li>
                  </ul>
                </div>
              </div>
            ) : (
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <CheckCircle2 className="h-4 w-4 text-green-600 dark:text-green-400" aria-hidden />
                <span>{t("security.ok")}</span>
              </div>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}
