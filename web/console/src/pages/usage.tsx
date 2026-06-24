import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { useTranslation } from "react-i18next";
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { FolderOpen, HardDrive, RefreshCw, Upload } from "lucide-react";
import { useAuth } from "@/hooks/use-auth";
import { api, type BucketUsage } from "@/lib/api";
import { formatBytes, formatBytesAxis, formatDate } from "@/lib/utils";
import { PageHeader } from "@/components/page-header";
import { MetricsCard } from "@/components/metrics-card";
import { DataTable } from "@/components/data-table";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

export function UsagePage() {
  const { t, i18n } = useTranslation(["usage", "common", "dashboard"]);
  const locale = i18n.language;
  const { isAdmin } = useAuth();
  const usage = useQuery({
    queryKey: ["usage"],
    queryFn: api.getUsage,
    refetchInterval: 60000,
  });

  const summary = usage.data?.summary;
  const chartData = (usage.data?.buckets ?? []).map((b) => ({
    bucket: b.name,
    storage: b.total_size,
    objects: b.object_count,
  }));

  const storageGrowth = (usage.data?.storage_growth ?? []).map((p) => ({
    date: p.date.slice(5),
    bytes: p.bytes,
  }));

  const columns: ColumnDef<BucketUsage>[] = useMemo(() => [
    { accessorKey: "name", header: t("usage:columns.bucket") },
    ...(isAdmin ? [{ accessorKey: "owner", header: t("usage:columns.owner") } as ColumnDef<BucketUsage>] : []),
    { accessorKey: "object_count", header: t("usage:columns.objects") },
    {
      accessorKey: "total_size",
      header: t("usage:columns.size"),
      cell: ({ row }) => formatBytes(row.original.total_size, locale),
    },
    {
      accessorKey: "last_activity",
      header: t("usage:columns.lastActivity"),
      cell: ({ row }) =>
        row.original.last_activity ? formatDate(row.original.last_activity, locale) : t("common:duration.empty"),
    },
  ], [t, locale, isAdmin]);

  return (
    <div>
      <PageHeader
        title={t("usage:title")}
        description={isAdmin ? t("usage:description.admin") : t("usage:description.user")}
        actions={
          <Button variant="outline" size="sm" onClick={() => usage.refetch()} disabled={usage.isFetching}>
            <RefreshCw className={`h-4 w-4 ${usage.isFetching ? "animate-spin" : ""}`} />
            {t("common:refresh")}
          </Button>
        }
      />

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4 mb-8">
        <MetricsCard
          title={isAdmin ? t("usage:metrics.bucketsAll") : t("usage:metrics.yourBuckets")}
          value={summary?.bucket_count ?? 0}
          icon={FolderOpen}
          loading={usage.isLoading}
        />
        <MetricsCard
          title={t("usage:metrics.objects")}
          value={summary?.object_count ?? 0}
          icon={HardDrive}
          loading={usage.isLoading}
        />
        <MetricsCard
          title={t("usage:metrics.totalStorage")}
          value={formatBytes(summary?.total_size ?? 0, locale)}
          icon={HardDrive}
          loading={usage.isLoading}
        />
        <MetricsCard
          title={t("usage:metrics.uploadBytes")}
          value={formatBytes(summary?.upload_bytes ?? 0, locale)}
          description={t("usage:metrics.download", { bytes: formatBytes(summary?.download_bytes ?? 0, locale) })}
          icon={Upload}
          loading={usage.isLoading}
        />
      </div>

      <div className="grid gap-6 lg:grid-cols-2 mb-8">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">
              {isAdmin ? t("usage:chart.perBucket") : t("usage:chart.yourBucketStorage")}
            </CardTitle>
            <CardDescription>{t("usage:chart.totalByBucket")}</CardDescription>
          </CardHeader>
          <CardContent className="h-64">
            {usage.isLoading ? (
              <Skeleton className="h-full w-full" />
            ) : (
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={chartData}>
                  <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
                  <XAxis dataKey="bucket" tick={{ fill: "hsl(var(--muted-foreground))" }} />
                  <YAxis
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
                  <Bar dataKey="storage" fill="hsl(var(--primary))" name={t("dashboard:chart.series")} radius={[4, 4, 0, 0]} />
                </BarChart>
              </ResponsiveContainer>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("usage:chart.growth")}</CardTitle>
            <CardDescription>{t("usage:chart.last30Days")}</CardDescription>
          </CardHeader>
          <CardContent className="h-64">
            {usage.isLoading ? (
              <Skeleton className="h-full w-full" />
            ) : (
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={storageGrowth}>
                  <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
                  <XAxis dataKey="date" tick={{ fill: "hsl(var(--muted-foreground))" }} />
                  <YAxis
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
                    dataKey="bytes"
                    stroke="hsl(var(--primary))"
                    fill="hsl(var(--primary))"
                    fillOpacity={0.2}
                    name={t("dashboard:chart.series")}
                  />
                </AreaChart>
              </ResponsiveContainer>
            )}
          </CardContent>
        </Card>
      </div>

      {usage.isLoading ? (
        <p className="text-muted-foreground">{t("usage:loading")}</p>
      ) : (
        <DataTable
          columns={columns}
          data={usage.data?.buckets ?? []}
          searchKey="name"
          searchPlaceholder={t("usage:searchPlaceholder")}
          emptyMessage={t("usage:empty")}
        />
      )}
    </div>
  );
}
