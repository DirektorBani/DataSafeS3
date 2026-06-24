import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { useTranslation } from "react-i18next";
import { Circle, RefreshCw } from "lucide-react";
import { api, type ActivityEvent } from "@/lib/api";
import { formatDate } from "@/lib/utils";
import { PageHeader } from "@/components/page-header";
import { DataTable } from "@/components/data-table";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";

const ACTION_VALUES = [
  "",
  "login",
  "logout",
  "bucket_created",
  "bucket_deleted",
  "object_uploaded",
  "object_downloaded",
  "object_deleted",
  "policy_changed",
  "trash_restored",
  "trash_purged",
  "object_scheduled_delete",
  "access_key_created",
  "access_key_deleted",
  "user_created",
  "user_deleted",
  "settings_changed",
] as const;

const ACTION_LOCALE_KEYS: Record<string, string> = {
  "": "all",
  login: "login",
  logout: "logout",
  bucket_created: "bucketCreated",
  bucket_deleted: "bucketDeleted",
  object_uploaded: "objectUploaded",
  object_deleted: "objectDeleted",
  policy_changed: "policyChanged",
  access_key_created: "keyCreated",
  access_key_deleted: "keyDeleted",
  user_created: "userCreated",
  user_deleted: "userDeleted",
  settings_changed: "settingsChanged",
};

function actionLabel(value: string, t: (key: string) => string): string {
  const key = ACTION_LOCALE_KEYS[value];
  if (key) return t(`actions.${key}`);
  if (!value) return t("actions.all");
  return value.replace(/_/g, " ");
}

export function ActivityPage() {
  const { t, i18n } = useTranslation(["activity", "common"]);
  const [period, setPeriod] = useState("7d");
  const [action, setAction] = useState("");
  const [userFilter, setUserFilter] = useState("");
  const [bucketFilter, setBucketFilter] = useState("");
  const [ipFilter, setIpFilter] = useState("");
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const pageSize = 20;

  const activity = useQuery({
    queryKey: ["activity", period, action, userFilter, bucketFilter, ipFilter, search, page],
    queryFn: () =>
      api.listActivity({
        period: period || undefined,
        action: action || undefined,
        user: userFilter || undefined,
        bucket: bucketFilter || undefined,
        ip: ipFilter || undefined,
        search: search || undefined,
        offset: page * pageSize,
        limit: pageSize,
      }),
    refetchInterval: 5000,
  });

  const [live, setLive] = useState(true);
  useEffect(() => {
    setLive(!activity.isFetching);
  }, [activity.isFetching, activity.dataUpdatedAt]);

  const columns: ColumnDef<ActivityEvent>[] = useMemo(() => [
    {
      accessorKey: "timestamp",
      header: t("activity:columns.time"),
      cell: ({ row }) => formatDate(row.original.timestamp, i18n.language),
    },
    { accessorKey: "user", header: t("activity:columns.user") },
    { accessorKey: "action", header: t("activity:columns.action") },
    { accessorKey: "resource_type", header: t("activity:columns.resourceType") },
    {
      accessorKey: "resource_name",
      header: t("activity:columns.resource"),
      cell: ({ row }) => (
        <code className="text-xs font-mono">{row.original.resource_name}</code>
      ),
    },
    { accessorKey: "ip_address", header: t("activity:columns.ip") },
  ], [t, i18n.language]);

  const total = activity.data?.total ?? 0;
  const pageCount = Math.max(1, Math.ceil(total / pageSize));

  return (
    <div>
      <PageHeader
        title={t("activity:title")}
        description={t("activity:description")}
        badge={
          <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
            <Circle className={cn("h-2 w-2 fill-current", live ? "text-emerald-500" : "text-amber-500")} />
            {live ? t("activity:live") : t("activity:updating")}
          </span>
        }
        actions={
          <Button
            variant="outline"
            size="sm"
            onClick={() => activity.refetch()}
            disabled={activity.isFetching}
          >
            <RefreshCw className={`h-4 w-4 ${activity.isFetching ? "animate-spin" : ""}`} />
            {t("common:refresh")}
          </Button>
        }
      />

      <div className="mb-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <div className="space-y-2">
          <Label>{t("activity:filters.period")}</Label>
          <Select value={period} onValueChange={(v) => { setPeriod(v); setPage(0); }}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="24h">{t("activity:period.24h")}</SelectItem>
              <SelectItem value="7d">{t("activity:period.7d")}</SelectItem>
              <SelectItem value="30d">{t("activity:period.30d")}</SelectItem>
              <SelectItem value="all">{t("activity:period.all")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label>{t("activity:filters.action")}</Label>
          <Select value={action || "all"} onValueChange={(v) => { setAction(v === "all" ? "" : v); setPage(0); }}>
            <SelectTrigger>
              <SelectValue placeholder={t("activity:actions.all")} />
            </SelectTrigger>
            <SelectContent>
              {ACTION_VALUES.map((value) => (
                <SelectItem key={value || "all"} value={value || "all"}>
                  {actionLabel(value, t)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label>{t("activity:filters.user")}</Label>
          <Input placeholder={t("activity:placeholder.username")} value={userFilter} onChange={(e) => { setUserFilter(e.target.value); setPage(0); }} />
        </div>
        <div className="space-y-2">
          <Label>{t("activity:filters.bucket")}</Label>
          <Input placeholder={t("activity:placeholder.bucket")} value={bucketFilter} onChange={(e) => { setBucketFilter(e.target.value); setPage(0); }} />
        </div>
        <div className="space-y-2">
          <Label>{t("activity:filters.ip")}</Label>
          <Input placeholder={t("activity:placeholder.ip")} value={ipFilter} onChange={(e) => { setIpFilter(e.target.value); setPage(0); }} />
        </div>
        <div className="space-y-2">
          <Label>{t("activity:filters.search")}</Label>
          <Input
            placeholder={t("activity:placeholder.search")}
            value={search}
            onChange={(e) => { setSearch(e.target.value); setPage(0); }}
          />
        </div>
      </div>

      {activity.isLoading ? (
        <p className="text-muted-foreground">{t("activity:loading")}</p>
      ) : (
        <>
          <DataTable
            columns={columns}
            data={activity.data?.events ?? []}
            emptyMessage={t("activity:empty")}
            pageSize={pageSize}
          />
          {pageCount > 1 && (
            <div className="mt-4 flex items-center justify-between">
              <p className="text-sm text-muted-foreground">
                {t("activity:pagination", { total, page: page + 1, pages: pageCount })}
              </p>
              <div className="flex gap-2">
                <Button variant="outline" size="sm" disabled={page === 0} onClick={() => setPage((p) => p - 1)}>
                  {t("common:previous")}
                </Button>
                <Button variant="outline" size="sm" disabled={page + 1 >= pageCount} onClick={() => setPage((p) => p + 1)}>
                  {t("common:next")}
                </Button>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}
