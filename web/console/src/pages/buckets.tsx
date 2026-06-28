import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { type ColumnDef } from "@tanstack/react-table";
import { useTranslation } from "react-i18next";
import { FolderOpen, Plus, RefreshCw, Trash2, Pin } from "lucide-react";
import { toast } from "sonner";
import { api, type Bucket } from "@/lib/api";
import { formatDate } from "@/lib/utils";
import { PageHeader } from "@/components/page-header";
import { DataTable } from "@/components/data-table";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useAuth } from "@/hooks/use-auth";

type FilesTab = "owned" | "shared" | "tenant";

export function BucketsPage() {
  const { t, i18n } = useTranslation(["buckets", "common"]);
  const { role } = useAuth();
  const isEndUser = role === "user";
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<FilesTab>(isEndUser ? "owned" : "owned");
  const [createOpen, setCreateOpen] = useState(false);
  const [newName, setNewName] = useState("");
  const [newVisibility, setNewVisibility] = useState("private");
  const [deleteTarget, setDeleteTarget] = useState<Bucket | null>(null);

  const buckets = useQuery({
    queryKey: ["buckets", tab],
    queryFn: async () => (await api.listBuckets(tab)).buckets,
  });

  const allBuckets = useQuery({
    queryKey: ["buckets", "all"],
    queryFn: async () => (await api.listBuckets("all")).buckets,
  });

  const favorites = useQuery({
    queryKey: ["favorites"],
    queryFn: async () => (await api.listFavorites()).favorites,
  });

  const recent = useQuery({
    queryKey: ["recent"],
    queryFn: async () => (await api.listRecent()).items,
  });

  const showTeamTab = (allBuckets.data ?? []).some((b) => b.access?.ownership === "tenant");

  const createMutation = useMutation({
    mutationFn: (payload: { name: string; visibility: string }) =>
      api.createBucket(payload.name, { visibility: payload.visibility }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["buckets"] });
      toast.success(t("buckets:toast.created"));
      setCreateOpen(false);
      setNewName("");
      setNewVisibility("private");
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (name: string) => api.deleteBucket(name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["buckets"] });
      toast.success(t("buckets:toast.deleted"));
      setDeleteTarget(null);
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const columns: ColumnDef<Bucket>[] = useMemo(() => {
    const cols: ColumnDef<Bucket>[] = [
      {
        accessorKey: "name",
        header: t("buckets:columns.name"),
        cell: ({ row }) => (
          <Link
            to={`/buckets/${encodeURIComponent(row.original.name)}`}
            className="inline-flex items-center gap-2 font-medium text-primary hover:underline"
          >
            <FolderOpen className="h-4 w-4" />
            {row.original.name}
          </Link>
        ),
      },
    ];
    if (tab === "shared") {
      cols.push({
        id: "shared_by",
        header: t("buckets:columns.sharedBy"),
        cell: ({ row }) => {
          const prefixes = row.original.access?.shared_prefixes;
          const by = row.original.access?.shared_by ?? "—";
          if (prefixes?.length) {
            return (
              <span className="text-muted-foreground">
                {by} · {prefixes.join(", ")}
              </span>
            );
          }
          return <span className="text-muted-foreground">{by}</span>;
        },
      });
    }
    if (tab === "tenant") {
      cols.push({
        id: "ownership",
        header: t("buckets:columns.type"),
        cell: () => <span className="text-muted-foreground">{t("buckets:ownership.tenant")}</span>,
      });
    }
    cols.push(
      {
        accessorKey: "visibility",
        header: t("buckets:columns.access"),
        cell: ({ row }) => (
          <span className="text-muted-foreground capitalize">
            {row.original.visibility === "public-read"
              ? t("buckets:visibility.publicRead")
              : t("buckets:visibility.private")}
          </span>
        ),
      },
      {
        accessorKey: "created_at",
        header: t("buckets:columns.created"),
        cell: ({ row }) => formatDate(row.original.created_at, i18n.language),
      },
      {
        id: "actions",
        header: "",
        cell: ({ row }) => (
          <div className="flex gap-1">
            <Button
              variant="ghost"
              size="sm"
              title={t("buckets:actions.pinTitle")}
              onClick={async () => {
                const fav = favorites.data?.find((f) => f.type === "bucket" && f.bucket === row.original.name);
                try {
                  if (fav) {
                    await api.deleteFavorite(fav.id);
                    toast.success(t("buckets:toast.unpinned"));
                  } else {
                    await api.createFavorite({ type: "bucket", bucket: row.original.name });
                    toast.success(t("buckets:toast.pinned"));
                  }
                  favorites.refetch();
                } catch (e) {
                  toast.error(e instanceof Error ? e.message : t("buckets:toast.failed"));
                }
              }}
            >
              <Pin
                className={`h-4 w-4 ${favorites.data?.some((f) => f.type === "bucket" && f.bucket === row.original.name) ? "fill-primary text-primary" : ""}`}
              />
            </Button>
            {row.original.access?.ownership === "owned" && (
              <Button
                variant="ghost"
                size="sm"
                className="text-destructive hover:text-destructive"
                onClick={() => setDeleteTarget(row.original)}
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            )}
          </div>
        ),
      }
    );
    return cols;
  }, [t, i18n.language, favorites, tab]);

  const emptyMessage =
    tab === "owned"
      ? isEndUser
        ? t("buckets:empty.ownedUser")
        : t("buckets:empty.owned")
      : tab === "shared"
        ? t("buckets:empty.shared")
        : t("buckets:empty.tenant");

  const homeBucket = (allBuckets.data ?? []).find(
    (b) => b.access?.ownership === "owned" && b.name === "files"
  );

  return (
    <div>
      <PageHeader
        title={isEndUser ? t("buckets:titleFiles") : t("buckets:title")}
        description={isEndUser ? t("buckets:descriptionFiles") : t("buckets:description")}
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => buckets.refetch()} disabled={buckets.isFetching}>
              <RefreshCw className={`h-4 w-4 ${buckets.isFetching ? "animate-spin" : ""}`} />
              {t("common:refresh")}
            </Button>
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus className="h-4 w-4" />
              {t("buckets:actions.create")}
            </Button>
          </>
        }
      />

      <Tabs value={tab} onValueChange={(v) => setTab(v as FilesTab)} className="mb-4">
        <TabsList>
          <TabsTrigger value="owned">{t("buckets:tabs.myFiles")}</TabsTrigger>
          <TabsTrigger value="shared">{t("buckets:tabs.shared")}</TabsTrigger>
          {showTeamTab && <TabsTrigger value="tenant">{t("buckets:tabs.team")}</TabsTrigger>}
        </TabsList>
      </Tabs>

      {tab === "owned" && homeBucket && isEndUser && (buckets.data?.length ?? 0) > 0 && (
        <p className="mb-4 text-sm text-muted-foreground">
          {t("buckets:homeReady")}{" "}
          <Link to={`/buckets/${encodeURIComponent(homeBucket.name)}`} className="text-primary hover:underline">
            {homeBucket.name}
          </Link>
        </p>
      )}

      {(recent.data?.length ?? 0) > 0 && (
        <div className="mb-6">
          <h2 className="mb-2 text-sm font-medium text-muted-foreground">{t("buckets:recent.title")}</h2>
          <div className="flex flex-wrap gap-2">
            {recent.data!.map((item) => (
              <Link
                key={item.id}
                to={`/buckets/${encodeURIComponent(item.bucket)}${item.prefix ? `?prefix=${encodeURIComponent(item.prefix)}` : ""}`}
                className="inline-flex items-center gap-2 rounded-md border px-3 py-1.5 text-sm hover:bg-muted"
              >
                <FolderOpen className="h-3.5 w-3.5 text-muted-foreground" />
                {item.bucket}
                {item.prefix ? ` / ${item.prefix}` : ""}
              </Link>
            ))}
          </div>
        </div>
      )}

      {buckets.isError ? (
        <p className="text-destructive" role="alert">
          {t("buckets:listError")}{" "}
          {buckets.error instanceof Error ? buckets.error.message : t("buckets:toast.failed")}
        </p>
      ) : buckets.isLoading ? (
        <p className="text-muted-foreground">{t("buckets:loading")}</p>
      ) : (
        <DataTable
          columns={columns}
          data={buckets.data ?? []}
          searchKey="name"
          searchPlaceholder={t("buckets:searchPlaceholder")}
          emptyMessage={emptyMessage}
        />
      )}

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("buckets:create.title")}</DialogTitle>
            <DialogDescription>{t("buckets:create.description")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="bucket-name">{t("buckets:fields.bucketName")}</Label>
            <Input
              id="bucket-name"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              placeholder={t("buckets:placeholder.bucketName")}
              onKeyDown={(e) =>
                e.key === "Enter" &&
                newName.trim() &&
                createMutation.mutate({ name: newName.trim(), visibility: newVisibility })
              }
            />
          </div>
          <div className="space-y-2">
            <Label>{t("buckets:fields.storageType")}</Label>
            <Select value={newVisibility} onValueChange={setNewVisibility}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="private">{t("buckets:visibility.private")}</SelectItem>
                <SelectItem value="public-read">{t("buckets:visibility.publicRead")}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              {t("common:cancel")}
            </Button>
            <Button
              disabled={!newName.trim() || createMutation.isPending}
              onClick={() => createMutation.mutate({ name: newName.trim(), visibility: newVisibility })}
            >
              {t("common:create")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title={t("buckets:delete.title")}
        description={t("buckets:delete.description", { name: deleteTarget?.name ?? "" })}
        confirmLabel={t("common:delete")}
        destructive
        onConfirm={() => deleteTarget && deleteMutation.mutate(deleteTarget.name)}
      />
    </div>
  );
}
