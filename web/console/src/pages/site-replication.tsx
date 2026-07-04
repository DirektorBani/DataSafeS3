import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api } from "@/lib/api";
import { PageHeader } from "@/components/page-header";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export function SiteReplicationPage() {
  const { t } = useTranslation("siteRepl");
  const status = useQuery({ queryKey: ["site-repl-status"], queryFn: () => api.siteReplicationStatus() });
  const peers = useQuery({ queryKey: ["site-repl-peers"], queryFn: () => api.listSiteReplicationPeers() });
  const rules = useQuery({ queryKey: ["site-repl-rules"], queryFn: () => api.listSiteReplicationRules() });

  return (
    <div>
      <PageHeader title={t("title")} description={t("description")} />
      {status.data && (
        <Card className="mb-6">
          <CardHeader><CardTitle className="text-base">{t("status.title")}</CardTitle></CardHeader>
          <CardContent className="text-sm space-y-1">
            <p>{t("status.pending")}: {status.data.pending_count}</p>
            <p>{t("status.lag")}: {status.data.lag_seconds.toFixed(1)}s</p>
          </CardContent>
        </Card>
      )}
      <Card className="mb-6">
        <CardHeader><CardTitle className="text-base">{t("peers.title")}</CardTitle></CardHeader>
        <CardContent className="text-sm">
          {(peers.data?.peers ?? []).map((p) => (
            <p key={p.id}>{p.name} — {p.endpoint}</p>
          ))}
          {(peers.data?.peers?.length ?? 0) === 0 && <p className="text-muted-foreground">{t("peers.empty")}</p>}
        </CardContent>
      </Card>
      <Card>
        <CardHeader><CardTitle className="text-base">{t("rules.title")}</CardTitle></CardHeader>
        <CardContent className="text-sm">
          {(rules.data?.rules ?? []).map((r) => (
            <p key={r.id}>{r.source_bucket} → {r.dest_bucket} ({r.direction})</p>
          ))}
          {(rules.data?.rules?.length ?? 0) === 0 && <p className="text-muted-foreground">{t("rules.empty")}</p>}
        </CardContent>
      </Card>
    </div>
  );
}
