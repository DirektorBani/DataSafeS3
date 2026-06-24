import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { FolderOpen, HardDrive, KeyRound, Activity, Pin } from "lucide-react";
import { Link } from "react-router-dom";
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { api, fetchMetricsSummary } from "@/lib/api";
import { formatBytes, formatBytesAxis } from "@/lib/utils";
import { PageHeader } from "@/components/page-header";
import { MetricsCard } from "@/components/metrics-card";
import { StatusBadge } from "@/components/status-badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";

export function DashboardPage() {
  const { t, i18n } = useTranslation(["dashboard", "common"]);
  const locale = i18n.language;
  const health = useQuery({ queryKey: ["health"], queryFn: api.health, refetchInterval: 30000 });
  const buckets = useQuery({ queryKey: ["buckets"], queryFn: async () => (await api.listBuckets()).buckets });
  const keys = useQuery({ queryKey: ["keys"], queryFn: async () => (await api.listKeys()).keys });
  const metrics = useQuery({ queryKey: ["metrics"], queryFn: fetchMetricsSummary, refetchInterval: 30000 });
  const usage = useQuery({ queryKey: ["usage"], queryFn: api.getUsage, refetchInterval: 60000 });
  const favorites = useQuery({ queryKey: ["favorites"], queryFn: async () => (await api.listFavorites()).favorites });

  const systemWide = usage.data?.scope?.system_wide ?? false;
  const bucketCount = buckets.data?.length ?? usage.data?.summary.bucket_count ?? (systemWide ? metrics.data?.buckets : 0) ?? 0;
  const storageBytes = usage.data?.summary.total_size ?? (systemWide ? metrics.data?.storageBytes : 0) ?? 0;
  const keyCount = keys.data?.length ?? 0;
  const loading = buckets.isLoading || keys.isLoading || usage.isLoading;

  const chartData = (usage.data?.storage_growth ?? []).map((p) => ({
    day: p.date.slice(5),
    storage: p.bytes,
  }));

  return (
    <div>
      <PageHeader
        title={t("dashboard:title")}
        description={
          systemWide
            ? t("dashboard:description.system", { brand: t("common:brand") })
            : t("dashboard:description.user")
        }
        badge={
          health.isLoading ? null : (
            <StatusBadge status={health.data?.status === "ok" ? "ok" : "error"} />
          )
        }
      />

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4 mb-8">
        <MetricsCard
          title={t("dashboard:metrics.buckets")}
          value={bucketCount}
          description={t("dashboard:metrics.bucketsDesc")}
          icon={FolderOpen}
          loading={loading}
        />
        {usage.data?.quota?.max_size_bytes ? (
          <>
            <MetricsCard
              title={t("dashboard:metrics.storageUsed")}
              value={formatBytes(usage.data.quota.used_size ?? storageBytes, locale)}
              description={t("dashboard:metrics.againstQuota")}
              icon={HardDrive}
              loading={usage.isLoading}
            />
            <MetricsCard
              title={t("dashboard:metrics.quota")}
              value={formatBytes(usage.data.quota.max_size_bytes, locale)}
              description={t("dashboard:metrics.maxStorage")}
              icon={HardDrive}
              loading={usage.isLoading}
            />
            <MetricsCard
              title={t("dashboard:metrics.remaining")}
              value={formatBytes(Math.max(0, usage.data.quota.remaining_size ?? 0), locale)}
              description={t("dashboard:metrics.availableStorage")}
              icon={HardDrive}
              loading={usage.isLoading}
            />
          </>
        ) : (
          <>
            <MetricsCard
              title={t("dashboard:metrics.storageUsed")}
              value={formatBytes(storageBytes, locale)}
              description={t("dashboard:metrics.fromUsageApi")}
              icon={HardDrive}
              loading={loading}
            />
            <MetricsCard
              title={t("dashboard:metrics.objects")}
              value={usage.data?.summary.object_count ?? 0}
              description={t("dashboard:metrics.latestObjects")}
              icon={HardDrive}
              loading={usage.isLoading}
            />
          </>
        )}
        <MetricsCard
          title={t("dashboard:metrics.accessKeys")}
          value={keyCount}
          description={t("dashboard:metrics.activeCredentials")}
          icon={KeyRound}
          loading={loading}
        />
        <MetricsCard
          title={t("dashboard:metrics.apiHealth")}
          value={health.data?.status === "ok" ? t("dashboard:metrics.online") : health.isLoading ? "..." : t("dashboard:metrics.offline")}
          description="/api/v1/health"
          icon={Activity}
          loading={health.isLoading}
        />
      </div>

      {(favorites.data ?? []).length > 0 && (
        <Card className="mb-8">
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2"><Pin className="h-4 w-4" /> {t("dashboard:favorites.title")}</CardTitle>
            <CardDescription>{t("dashboard:favorites.description")}</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-wrap gap-2">
            {(favorites.data ?? []).map((f) => (
              <Button key={f.id} variant="outline" size="sm" asChild>
                <Link to={`/buckets/${encodeURIComponent(f.bucket)}${f.prefix ? `?prefix=${encodeURIComponent(f.prefix)}` : ""}`}>
                  <FolderOpen className="h-3.5 w-3.5" />
                  {f.bucket}{f.prefix ? ` / ${f.prefix}` : ""}
                </Link>
              </Button>
            ))}
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <div>
            <CardTitle className="text-base">{t("dashboard:chart.title")}</CardTitle>
            <CardDescription>{t("dashboard:chart.description")}</CardDescription>
          </div>
        </CardHeader>
        <CardContent className="h-64">
          {loading || usage.isLoading ? (
            <Skeleton className="h-full w-full" />
          ) : chartData.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("dashboard:chart.empty")}</p>
          ) : (
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={chartData}>
                <defs>
                  <linearGradient id="reqFill" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="hsl(var(--primary))" stopOpacity={0.3} />
                    <stop offset="95%" stopColor="hsl(var(--primary))" stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
                <XAxis dataKey="day" className="text-xs" tick={{ fill: "hsl(var(--muted-foreground))" }} />
                <YAxis
                  className="text-xs"
                  tick={{ fill: "hsl(var(--muted-foreground))" }}
                  tickFormatter={(v) => formatBytesAxis(v, locale)}
                  width={72}
                />
                <Tooltip
                  formatter={(value: number) => formatBytes(value, locale)}
                  contentStyle={{
                    background: "hsl(var(--popover))",
                    border: "1px solid hsl(var(--border))",
                    borderRadius: "var(--radius)",
                  }}
                />
                <Area
                  type="monotone"
                  dataKey="storage"
                  stroke="hsl(var(--primary))"
                  fill="url(#reqFill)"
                  name={t("dashboard:chart.series")}
                />
              </AreaChart>
            </ResponsiveContainer>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
