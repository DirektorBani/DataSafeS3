import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api } from "@/lib/api";
import { PageHeader } from "@/components/page-header";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

export function ClusterPage() {
  const { t } = useTranslation("cluster");
  const status = useQuery({ queryKey: ["cluster-status"], queryFn: () => api.clusterStatus() });

  return (
    <div>
      <PageHeader title={t("title")} description={t("description")} />
      {status.data && (
        <>
          <Card className="mb-6">
            <CardHeader>
              <CardTitle className="text-base">{t("config.title")}</CardTitle>
              <CardDescription>{t("config.description")}</CardDescription>
            </CardHeader>
            <CardContent className="text-sm space-y-2">
              <p>{t("distributedMode")} {status.data.distributed_mode ? t("distributed.enabled") : t("distributed.single")}</p>
              <p>{t("erasureCoding")} {status.data.erasure_coding_planned ? t("erasure.planned") : t("erasure.notConfigured")}</p>
              {status.data.disk_paths?.length > 0 && (
                <p>{t("diskPaths")} {status.data.disk_paths.join(", ")}</p>
              )}
            </CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle className="text-base">{t("nodes.title")}</CardTitle></CardHeader>
            <CardContent className="space-y-2">
              {status.data.nodes.map((n) => (
                <div key={n.id} className="flex items-center justify-between rounded border p-3">
                  <div>
                    <p className="font-medium">{n.id}</p>
                    <p className="text-xs text-muted-foreground">{n.address} — {n.role}</p>
                  </div>
                  <Badge variant={n.status === "healthy" ? "default" : "secondary"}>{n.status ?? t("node.unknown")}</Badge>
                </div>
              ))}
            </CardContent>
          </Card>
        </>
      )}
    </div>
  );
}
