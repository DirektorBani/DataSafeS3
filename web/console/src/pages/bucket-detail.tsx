import { useCallback, useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams, useSearchParams } from "react-router-dom";
import {
  ArrowLeft,
  ChevronRight,
  Clock,
  Copy,
  File,
  Folder,
  FolderPlus,
  Info,
  Link2,
  Pencil,
  Pin,
  RefreshCw,
  RotateCcw,
  Save,
  Share2,
  Trash2,
  Upload,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import {
  api,
  MULTIPART_THRESHOLD,
  uploadObjectMultipart,
  type BucketAccessGrant,
  type BucketSettings,
  type LifecycleRule,
  type MultipartUploadProgress,
  type ObjectRow,
  type TrashItem,
} from "@/lib/api";
import { formatBytes, formatDate, formatDuration, formatSpeed } from "@/lib/utils";
import { PageHeader } from "@/components/page-header";
import { CopyButton } from "@/components/copy-button";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { useAuth } from "@/hooks/use-auth";
import { BucketSettingsTabs } from "@/components/settings/BucketSettingsTabs";

type UploadItem = {
  name: string;
  progress: number;
  status: "uploading" | "done" | "error";
  multipart?: boolean;
  partsDone?: number;
  partsTotal?: number;
  speed?: number;
  eta?: number;
};
type DeleteMode = "now" | "scheduled";

function breadcrumbParts(prefix: string, rootLabel: string): { label: string; path: string }[] {
  if (!prefix) return [{ label: rootLabel, path: "" }];
  const parts = prefix.replace(/\/$/, "").split("/");
  const crumbs = [{ label: rootLabel, path: "" }];
  let acc = "";
  for (const p of parts) {
    acc += p + "/";
    crumbs.push({ label: p, path: acc });
  }
  return crumbs;
}

export function BucketDetailPage() {
  const { t, i18n } = useTranslation(["bucketDetail", "common"]);
  const PRESIGN_PRESETS = [
    { label: t("bucketDetail:presign.1h"), seconds: 3600 },
    { label: t("bucketDetail:presign.24h"), seconds: 86400 },
    { label: t("bucketDetail:presign.7d"), seconds: 604800 },
    { label: t("bucketDetail:presign.30d"), seconds: 2592000 },
  ] as const;
  const LIFECYCLE_ACTIONS = [
    { value: "expire", label: t("bucketDetail:lifecycle.expire") },
    { value: "abort_multipart", label: t("bucketDetail:lifecycle.abortMultipart") },
    { value: "expire_noncurrent", label: t("bucketDetail:lifecycle.expireNoncurrent") },
  ] as const;
  const { bucketName } = useParams<{ bucketName: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const activeTab = searchParams.get("tab") || "objects";
  const { canManageTenant, isAdmin, username } = useAuth();
  const bucket = bucketName ? decodeURIComponent(bucketName) : "";
  const me = useQuery({
    queryKey: ["me"],
    queryFn: api.getMe,
    enabled: !!bucket,
  });
  const queryClient = useQueryClient();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const dropInputRef = useRef<HTMLInputElement>(null);

  const [prefix, setPrefix] = useState(() => searchParams.get("prefix") || "");

  useEffect(() => {
    const fromUrl = searchParams.get("prefix") || "";
    setPrefix(fromUrl);
  }, [searchParams]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [detailObject, setDetailObject] = useState<ObjectRow | null>(null);
  const [shareObject, setShareObject] = useState<ObjectRow | null>(null);
  const [shareUrl, setShareUrl] = useState("");
  const [createdShare, setCreatedShare] = useState<{ max_downloads: number; download_count: number } | null>(null);
  const [shareExpires, setShareExpires] = useState("3600");
  const [shareMaxDownloads, setShareMaxDownloads] = useState("0");
  const [customExpires, setCustomExpires] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<ObjectRow | null>(null);
  const [deleteMode, setDeleteMode] = useState<DeleteMode>("now");
  const [scheduleDuration, setScheduleDuration] = useState<"1d" | "1w" | "1m">("1d");
  const [uploads, setUploads] = useState<UploadItem[]>([]);
  const [dragOver, setDragOver] = useState(false);
  const [folderName, setFolderName] = useState("");
  const [folderOpen, setFolderOpen] = useState(false);
  const [moveTarget, setMoveTarget] = useState<ObjectRow | null>(null);
  const [moveDest, setMoveDest] = useState("");
  const [copyTarget, setCopyTarget] = useState<ObjectRow | null>(null);
  const [copyDestBucket, setCopyDestBucket] = useState("");
  const [copyDestKey, setCopyDestKey] = useState("");
  const [lifecycleDraft, setLifecycleDraft] = useState<LifecycleRule[]>([]);
  const [settingsDraft, setSettingsDraft] = useState<BucketSettings | null>(null);
  const [ruleDialog, setRuleDialog] = useState<LifecycleRule | null>(null);
  const [nextMarker, setNextMarker] = useState<string | undefined>();
  const [allFiles, setAllFiles] = useState<ObjectRow[]>([]);
  const [filesPrefix, setFilesPrefix] = useState("");
  const [renameTarget, setRenameTarget] = useState<ObjectRow | null>(null);
  const [renameDest, setRenameDest] = useState("");
  const [metaDraft, setMetaDraft] = useState<{ metadata: string; tags: string; contentType: string } | null>(null);
  const [bucketTagsDraft, setBucketTagsDraft] = useState("");
  const [folderPinned, setFolderPinned] = useState(false);
  const [deleteFolderTarget, setDeleteFolderTarget] = useState<string | null>(null);
  const [deleteFolderRecursive, setDeleteFolderRecursive] = useState(false);
  const [moveDestBucket, setMoveDestBucket] = useState("");

  const PAGE_SIZE = 50;

  const objects = useQuery({
    queryKey: ["objects", bucket, prefix, nextMarker],
    queryFn: async () => api.listObjects(bucket, prefix || undefined, "/", { startAfter: nextMarker, maxKeys: PAGE_SIZE }),
    enabled: !!bucket,
    placeholderData: undefined,
  });

  const favorites = useQuery({
    queryKey: ["favorites"],
    queryFn: async () => (await api.listFavorites()).favorites,
    enabled: !!bucket,
  });

  const versions = useQuery({
    queryKey: ["versions", bucket, prefix],
    queryFn: async () => (await api.listVersions(bucket, prefix || undefined)).versions,
    enabled: !!bucket,
  });

  const settings = useQuery({
    queryKey: ["bucket-settings", bucket],
    queryFn: () => api.getBucketSettings(bucket),
    enabled: !!bucket,
  });

  const lifecycle = useQuery({
    queryKey: ["lifecycle", bucket],
    queryFn: async () => (await api.getLifecycle(bucket)).rules as LifecycleRule[],
    enabled: !!bucket,
  });

  const trash = useQuery({
    queryKey: ["trash", bucket],
    queryFn: async () => (await api.listTrash(bucket)).items,
    enabled: !!bucket,
  });

  const usage = useQuery({
    queryKey: ["usage"],
    queryFn: api.getUsage,
    enabled: !!bucket,
  });

  const buckets = useQuery({
    queryKey: ["buckets"],
    queryFn: async () => (await api.listBuckets()).buckets,
    enabled: !!copyTarget || !!moveTarget,
  });

  const tenantId = settings.data?.tenant_id;
  const ownerId = settings.data?.owner_id;
  const isBucketOwner =
    (!!ownerId && me.data?.user_id === ownerId) ||
    (!!settings.data?.owner && !!username && settings.data.owner === username);
  const showAccessTab =
    !!bucket &&
    (isAdmin || (!!tenantId && tenantId !== "default" && canManageTenant(tenantId)) || isBucketOwner);
  const useTenantAccessApi = !!tenantId && tenantId !== "default" && canManageTenant(tenantId) && !isBucketOwner;
  const tenantMembers = useQuery({
    queryKey: ["tenant-members", tenantId],
    queryFn: async () => (await api.listTenantMembers(tenantId!)).members,
    enabled: showAccessTab && useTenantAccessApi,
  });
  const shareableUsers = useQuery({
    queryKey: ["shareable-users", bucket],
    queryFn: async () => (await api.listShareableUsers(bucket!)).users,
    enabled: showAccessTab && !useTenantAccessApi,
  });
  const bucketAccess = useQuery({
    queryKey: ["bucket-access", tenantId, bucket, useTenantAccessApi],
    queryFn: async () => {
      const res = useTenantAccessApi
        ? await api.listBucketAccess(tenantId!, bucket!)
        : await api.listBucketAccessByBucket(bucket!);
      return { grants: res.grants, prefix_grants: res.prefix_grants ?? [] };
    },
    enabled: showAccessTab && !!bucket,
  });
  const [accessDraft, setAccessDraft] = useState<BucketAccessGrant[]>([]);
  const [prefixAccessDraft, setPrefixAccessDraft] = useState<BucketAccessGrant[]>([]);
  useEffect(() => {
    if (bucketAccess.data) {
      setAccessDraft(bucketAccess.data.grants.map((g) => ({ ...g })));
      setPrefixAccessDraft(bucketAccess.data.prefix_grants.map((g) => ({ ...g })));
    }
  }, [bucketAccess.data]);

  useEffect(() => {
    if (settings.data) setSettingsDraft({ ...settings.data });
  }, [settings.data]);

  useEffect(() => {
    if (lifecycle.data) setLifecycleDraft(lifecycle.data);
  }, [lifecycle.data]);

  useEffect(() => {
    if (!objects.data || filesPrefix !== prefix) return;
    const files = (objects.data.objects ?? []).filter((o) => !o.key.endsWith("/") || o.size > 0);
    if (!nextMarker) setAllFiles(files);
    else setAllFiles((prev) => (filesPrefix === prefix ? [...prev, ...files] : files));
  }, [objects.data, nextMarker, prefix, filesPrefix]);

  useEffect(() => {
    setNextMarker(undefined);
    setAllFiles([]);
    setFilesPrefix(prefix);
    void queryClient.invalidateQueries({ queryKey: ["objects", bucket, prefix] });
  }, [prefix, bucket, queryClient]);

  useEffect(() => {
    const fav = favorites.data?.find((f) => f.type === "folder" && f.bucket === bucket && f.prefix === prefix);
    setFolderPinned(!!fav);
  }, [favorites.data, bucket, prefix]);

  useEffect(() => {
    if (!detailObject) {
      setMetaDraft(null);
      return;
    }
    const key = detailObject.key;
    const versionId = detailObject.version_id;
    api.getObjectMeta(bucket, key, versionId).then((res) => {
      const o = res.object;
      setMetaDraft({
        contentType: o.content_type ?? "",
        metadata: JSON.stringify(o.metadata ?? {}, null, 2),
        tags: JSON.stringify(o.tags ?? {}, null, 2),
      });
      setDetailObject((prev) => (prev?.key === key ? { ...prev, ...o } : prev));
    }).catch(() => {
      setMetaDraft({
        contentType: detailObject.content_type ?? "",
        metadata: JSON.stringify(detailObject.metadata ?? {}, null, 2),
        tags: JSON.stringify(detailObject.tags ?? {}, null, 2),
      });
    });
  }, [detailObject?.key, bucket]);

  useEffect(() => {
    if (settings.data?.tags) setBucketTagsDraft(JSON.stringify(settings.data.tags, null, 2));
  }, [settings.data?.tags]);

  const folders = filesPrefix === prefix ? (objects.data?.folders ?? []) : [];
  const files = filesPrefix === prefix ? allFiles : [];
  const listTruncated = objects.data?.truncated ?? false;
  const bucketUsage = usage.data?.buckets?.find((b) => b.name === bucket);
  const bucketSettings = settings.data;
  const usedBytes = bucketUsage?.total_size ?? 0;
  const limitBytes = bucketSettings?.max_size_bytes ?? 0;
  const usagePct = limitBytes > 0 ? Math.min(100, Math.round((usedBytes / limitBytes) * 100)) : 0;

  const invalidateObjects = () => {
    queryClient.invalidateQueries({ queryKey: ["objects", bucket] });
    queryClient.invalidateQueries({ queryKey: ["versions", bucket] });
  };

  const deleteMutation = useMutation({
    mutationFn: ({ key, mode, schedule }: { key: string; mode: DeleteMode; schedule?: "1d" | "1w" | "1m" }) =>
      mode === "scheduled" && schedule
        ? api.deleteObject(bucket, key, { schedule })
        : api.deleteObject(bucket, key),
    onSuccess: (data, vars) => {
      invalidateObjects();
      if (vars.mode === "scheduled") {
        const at = (data as { scheduled_delete_at?: string })?.scheduled_delete_at;
        toast.success(at ? t("bucketDetail:toast.deletionScheduled", { date: formatDate(at, i18n.language) }) : t("bucketDetail:toast.deletionScheduledShort"));
      } else {
        toast.success(t("bucketDetail:toast.objectDeleted"));
      }
      setDeleteTarget(null);
      setDeleteMode("now");
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const bulkDeleteMutation = useMutation({
    mutationFn: (keys: string[]) => api.bulkDeleteObjects(bucket, keys),
    onSuccess: (data) => {
      invalidateObjects();
      setSelected(new Set());
      toast.success(`Deleted ${data.deleted} object(s)`);
      if (data.errors?.length) toast.error(data.errors.join("; "));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const uploadFiles = useCallback(
    async (files: FileList | File[]) => {
      const list = Array.from(files);
      if (!list.length) return;
      for (const file of list) {
        const key = prefix ? `${prefix}${file.name}` : file.name;
        const useMultipart = file.size >= MULTIPART_THRESHOLD;
        setUploads((prev) => [
          ...prev,
          {
            name: key,
            progress: 0,
            status: "uploading",
            multipart: useMultipart,
            partsDone: 0,
            partsTotal: useMultipart ? Math.ceil(file.size / (64 * 1024 * 1024)) : undefined,
          },
        ]);
        try {
          if (useMultipart) {
            await uploadObjectMultipart(bucket, key, file, (p: MultipartUploadProgress) => {
              const pct = Math.round((p.loaded / p.total) * 100);
              setUploads((prev) =>
                prev.map((u) =>
                  u.name === key
                    ? { ...u, progress: pct, partsDone: p.partsDone, partsTotal: p.partsTotal, speed: p.speed, eta: p.eta }
                    : u
                )
              );
            });
          } else {
            await api.uploadObject(bucket, key, file, (pct) => {
              setUploads((prev) => prev.map((u) => (u.name === key ? { ...u, progress: pct } : u)));
            });
          }
          setUploads((prev) => prev.map((u) => (u.name === key ? { ...u, progress: 100, status: "done" } : u)));
        } catch (err) {
          setUploads((prev) => prev.map((u) => (u.name === key ? { ...u, status: "error" } : u)));
          toast.error(err instanceof Error ? err.message : t("bucketDetail:toast.uploadFailed"));
        }
      }
      setNextMarker(undefined);
      invalidateObjects();
      setTimeout(() => setUploads([]), 5000);
    },
    [bucket, prefix, queryClient]
  );

  const createFolderMutation = useMutation({
    mutationFn: (name: string) => api.createFolder(bucket, prefix ? `${prefix}${name}` : name),
    onSuccess: () => {
      invalidateObjects();
      setFolderOpen(false);
      setFolderName("");
      toast.success(t("bucketDetail:toast.folderCreated"));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const saveSettingsMutation = useMutation({
    mutationFn: () => {
      if (!settingsDraft) throw new Error("No settings");
      return api.updateBucketSettings(bucket, {
        description: settingsDraft.description,
        versioning_enabled: settingsDraft.versioning_enabled,
        object_lock_enabled: settingsDraft.object_lock_enabled,
        retention_days: settingsDraft.retention_days,
        storage_class: settingsDraft.storage_class,
        visibility: settingsDraft.visibility,
        max_size_bytes: settingsDraft.max_size_bytes,
        max_objects: settingsDraft.max_objects,
        lifecycle_rules: settingsDraft.lifecycle_rules,
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["bucket-settings", bucket] });
      toast.success(t("bucketDetail:toast.settingsSaved"));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const saveLifecycleMutation = useMutation({
    mutationFn: (rules: LifecycleRule[]) => api.putLifecycle(bucket, rules),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["lifecycle", bucket] });
      toast.success(t("bucketDetail:toast.lifecycleSaved"));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const shareLinkMutation = useMutation({
    mutationFn: async (obj: ObjectRow) => {
      const expires = shareExpires === "custom" ? parseInt(customExpires, 10) : parseInt(shareExpires, 10);
      const maxDl = parseInt(shareMaxDownloads, 10) || 0;
      return api.createSharedLink(bucket, {
        key: obj.key,
        expires_in_sec: expires || 3600,
        max_downloads: maxDl,
      });
    },
    onSuccess: (res) => {
      setShareUrl(res.url);
      setCreatedShare(res.share);
      toast.success(t("bucketDetail:toast.shareCreated"));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const toggleSelect = (key: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const crumbs = breadcrumbParts(prefix, t("bucketDetail:breadcrumb.root"));

  if (!bucket) return <p className="text-muted-foreground">{t("bucketDetail:invalidBucket")}</p>;

  return (
    <div>
      <div className="mb-4">
        <Button variant="ghost" size="sm" asChild>
          <Link to="/buckets">
            <ArrowLeft className="h-4 w-4" />
            {t("bucketDetail:backToBuckets")}
          </Link>
        </Button>
      </div>

      <PageHeader
        title={bucket}
        description={t("bucketDetail:description")}
        actions={
          <Button variant="outline" size="sm" onClick={() => objects.refetch()} disabled={objects.isFetching}>
            <RefreshCw className={`h-4 w-4 ${objects.isFetching ? "animate-spin" : ""}`} />
            {t("common:refresh")}
          </Button>
        }
      />

      {limitBytes > 0 && (
        <Card className="mb-4">
          <CardContent className="py-4 space-y-2">
            <div className="flex justify-between text-sm">
              <span className="text-muted-foreground">{t("bucketDetail:storageUsage")}</span>
              <span>
                {formatBytes(usedBytes, i18n.language)} / {formatBytes(limitBytes, i18n.language)} ({usagePct}%)
              </span>
            </div>
            <div className="h-2 rounded-full bg-muted overflow-hidden">
              <div className="h-full bg-primary transition-all" style={{ width: `${usagePct}%` }} />
            </div>
          </CardContent>
        </Card>
      )}

      <Tabs
        value={activeTab}
        onValueChange={(tab) => setSearchParams({ tab }, { replace: true })}
        className="space-y-4"
      >
        <TabsList>
          <TabsTrigger value="objects">{t("bucketDetail:tabs.objects")}</TabsTrigger>
          <TabsTrigger value="versions">{t("bucketDetail:tabs.versions")}</TabsTrigger>
          <TabsTrigger value="settings">{t("bucketDetail:tabs.settings")}</TabsTrigger>
          {showAccessTab && <TabsTrigger value="access">{t("bucketDetail:tabs.share")}</TabsTrigger>}
          <TabsTrigger value="lifecycle">{t("bucketDetail:tabs.lifecycle")}</TabsTrigger>
          <TabsTrigger value="trash">{t("bucketDetail:tabs.trash")}</TabsTrigger>
        </TabsList>

        <TabsContent value="objects" className="space-y-4">
          <div className="flex flex-wrap items-center gap-2 text-sm">
            {crumbs.map((c, i) => (
              <span key={c.path} className="inline-flex items-center gap-1">
                {i > 0 && <ChevronRight className="h-3 w-3 text-muted-foreground" />}
                <button
                  type="button"
                  className={cn(
                    "hover:text-primary",
                    i === crumbs.length - 1 ? "font-medium text-foreground" : "text-muted-foreground"
                  )}
                  onClick={() => {
                    setPrefix(c.path);
                    setSelected(new Set());
                  }}
                >
                  {c.label}
                </button>
              </span>
            ))}
            <Button
              size="sm"
              variant={folderPinned ? "default" : "outline"}
              className="ml-auto h-7"
              onClick={async () => {
                const fav = favorites.data?.find((f) => f.type === "folder" && f.bucket === bucket && f.prefix === prefix);
                if (fav) {
                  await api.deleteFavorite(fav.id);
                  toast.success(t("bucketDetail:toast.folderUnpinned"));
                } else {
                  await api.createFavorite({ type: "folder", bucket, prefix });
                  toast.success(t("bucketDetail:toast.folderPinned"));
                }
                favorites.refetch();
              }}
            >
              <Pin className="h-3.5 w-3.5" /> {folderPinned ? t("bucketDetail:pin.pinned") : t("bucketDetail:pin.pinFolder")}
            </Button>
          </div>

          <div className="flex flex-wrap gap-2">
            <Button size="sm" onClick={() => fileInputRef.current?.click()}>
              <Upload className="h-4 w-4" /> {t("bucketDetail:actions.upload")}
            </Button>
            <Button size="sm" variant="outline" onClick={() => setFolderOpen(true)}>
              <FolderPlus className="h-4 w-4" /> {t("bucketDetail:actions.newFolder")}
            </Button>
            {selected.size > 0 && (
              <Button
                size="sm"
                variant="destructive"
                onClick={() => bulkDeleteMutation.mutate([...selected])}
                disabled={bulkDeleteMutation.isPending}
              >
                <Trash2 className="h-4 w-4" /> {t("bucketDetail:actions.delete", { count: selected.size })}
              </Button>
            )}
            <input ref={fileInputRef} type="file" multiple className="hidden" onChange={(e) => e.target.files && uploadFiles(e.target.files)} />
          </div>

          <Card
            className={cn("border-dashed transition-colors cursor-pointer", dragOver && "border-primary bg-primary/5")}
            onDragOver={(e) => { e.preventDefault(); setDragOver(true); }}
            onDragLeave={() => setDragOver(false)}
            onDrop={(e) => { e.preventDefault(); setDragOver(false); uploadFiles(e.dataTransfer.files); }}
            onClick={() => dropInputRef.current?.click()}
          >
            <CardContent className="flex flex-col items-center py-8 text-center pointer-events-none">
              <Upload className="mb-2 h-7 w-7 text-muted-foreground" />
              <p className="text-sm font-medium">{t("bucketDetail:dropzone")}</p>
            </CardContent>
            <input ref={dropInputRef} type="file" multiple className="hidden" onChange={(e) => { if (e.target.files) uploadFiles(e.target.files); e.target.value = ""; }} />
          </Card>

          {uploads.length > 0 && (
            <div className="space-y-2">
              {uploads.map((u) => (
                <div key={u.name} className="rounded-md border p-3 text-sm space-y-1">
                  <div className="flex items-center gap-3">
                    <span className="w-48 truncate font-mono">{u.name}</span>
                    {u.multipart && (
                      <span className="text-xs text-muted-foreground">
                        {t("bucketDetail:upload.part", { done: u.partsDone ?? 0, total: u.partsTotal ?? 0 })}
                        {u.speed ? ` · ${formatSpeed(u.speed, i18n.language)}` : ""}
                        {u.eta ? ` · ${t("bucketDetail:upload.eta", { duration: formatDuration(u.eta, i18n.language) })}` : ""}
                      </span>
                    )}
                  </div>
                  <div className="flex-1 h-2 rounded-full bg-muted overflow-hidden">
                    <div className={cn("h-full transition-all", u.status === "error" ? "bg-destructive" : "bg-primary")} style={{ width: `${u.progress}%` }} />
                  </div>
                </div>
              ))}
            </div>
          )}

          <div className="grid gap-4 lg:grid-cols-[1fr_280px]">
            <Card>
              <CardContent className="p-0">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b text-left text-muted-foreground">
                      <th className="p-3 w-8" />
                      <th className="p-3">{t("bucketDetail:table.name")}</th>
                      <th className="p-3">{t("bucketDetail:table.size")}</th>
                      <th className="p-3">{t("bucketDetail:table.modified")}</th>
                      <th className="p-3 w-24" />
                    </tr>
                  </thead>
                  <tbody>
                    {folders.map((f) => (
                      <tr key={f} className="border-b hover:bg-muted/50 cursor-pointer" onClick={() => setPrefix(f)}>
                        <td className="p-3" />
                        <td className="p-3 font-mono inline-flex items-center gap-2">
                          <Folder className="h-4 w-4 text-amber-500" />
                          {f.replace(prefix, "").replace(/\/$/, "")}
                        </td>
                        <td className="p-3 text-muted-foreground">—</td>
                        <td className="p-3" />
                        <td className="p-3">
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-7 w-7 text-destructive"
                            onClick={(e) => {
                              e.stopPropagation();
                              setDeleteFolderTarget(f);
                              setDeleteFolderRecursive(false);
                            }}
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </Button>
                        </td>
                      </tr>
                    ))}
                    {files.map((obj) => (
                      <tr
                        key={obj.key}
                        className={cn("border-b hover:bg-muted/50", detailObject?.key === obj.key && "bg-muted/30")}
                      >
                        <td className="p-3">
                          <input type="checkbox" checked={selected.has(obj.key)} onChange={() => toggleSelect(obj.key)} />
                        </td>
                        <td className="p-3 font-mono">
                          <button type="button" className="inline-flex items-center gap-2 hover:text-primary" onClick={() => setDetailObject(obj)}>
                            <File className="h-4 w-4 text-muted-foreground" />
                            {obj.key.replace(prefix, "")}
                          </button>
                        </td>
                        <td className="p-3">{formatBytes(obj.size)}</td>
                        <td className="p-3">{formatDate(obj.last_modified, i18n.language)}</td>
                        <td className="p-3">
                          <div className="flex gap-1">
                            <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => { setShareObject(obj); setShareUrl(""); }}>
                              <Share2 className="h-3.5 w-3.5" />
                            </Button>
                            <Button variant="ghost" size="icon" className="h-7 w-7 text-destructive" onClick={() => setDeleteTarget(obj)}>
                              <Trash2 className="h-3.5 w-3.5" />
                            </Button>
                          </div>
                        </td>
                      </tr>
                    ))}
                    {!objects.isLoading && folders.length === 0 && files.length === 0 && (
                      <tr><td colSpan={5} className="p-8 text-center text-muted-foreground">{t("bucketDetail:emptyFolder")}</td></tr>
                    )}
                  </tbody>
                </table>
                {listTruncated && (
                  <div className="p-3 border-t text-center">
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => setNextMarker(objects.data?.next_marker)}
                      disabled={objects.isFetching}
                    >
                      {t("bucketDetail:loadMore")}
                    </Button>
                  </div>
                )}
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm flex items-center gap-2"><Info className="h-4 w-4" /> Metadata</CardTitle>
              </CardHeader>
              <CardContent className="text-sm space-y-2">
                {detailObject && metaDraft ? (
                  <>
                    <div><span className="text-muted-foreground">{t("bucketDetail:metadata.key")}</span><p className="font-mono break-all">{detailObject.key}</p></div>
                    <div><span className="text-muted-foreground">{t("bucketDetail:metadata.contentType")}</span>
                      <Input className="mt-1 h-8 text-xs" value={metaDraft.contentType} onChange={(e) => setMetaDraft({ ...metaDraft, contentType: e.target.value })} />
                    </div>
                    <div><span className="text-muted-foreground">{t("bucketDetail:metadata.size")}</span><p>{formatBytes(detailObject.size)}</p></div>
                    <div><span className="text-muted-foreground">{t("bucketDetail:metadata.etag")}</span><p className="font-mono text-xs">{detailObject.etag}</p></div>
                    <div><span className="text-muted-foreground">{t("bucketDetail:metadata.created")}</span><p>{formatDate(detailObject.created_at ?? detailObject.last_modified, i18n.language)}</p></div>
                    <div><span className="text-muted-foreground">{t("bucketDetail:metadata.modified")}</span><p>{formatDate(detailObject.last_modified, i18n.language)}</p></div>
                    {detailObject.version_id && <div><span className="text-muted-foreground">{t("bucketDetail:metadata.version")}</span><p className="font-mono text-xs">{detailObject.version_id}</p></div>}
                    {detailObject.retention_until && <div><span className="text-muted-foreground">{t("bucketDetail:metadata.retentionUntil")}</span><p>{formatDate(detailObject.retention_until, i18n.language)}</p></div>}
                    {detailObject.storage_class && <div><span className="text-muted-foreground">{t("bucketDetail:metadata.storageClass")}</span><p>{detailObject.storage_class}</p></div>}
                    <div className="flex items-center gap-2">
                      <input type="checkbox" className="h-4 w-4" checked={detailObject.legal_hold ?? false}
                        onChange={async (e) => {
                          try {
                            await api.setLegalHold(bucket, detailObject.key, e.target.checked, detailObject.version_id);
                            setDetailObject({ ...detailObject, legal_hold: e.target.checked });
                            toast.success(e.target.checked ? t("bucketDetail:toast.legalHoldEnabled") : t("bucketDetail:toast.legalHoldCleared"));
                          } catch (err) {
                            toast.error(err instanceof Error ? err.message : t("common:failed"));
                          }
                        }} />
                      <Label>{t("bucketDetail:metadata.legalHold")}</Label>
                    </div>
                    <div className="space-y-1">
                      <span className="text-muted-foreground">{t("bucketDetail:metadata.tagsJson")}</span>
                      <Textarea rows={2} className="font-mono text-xs" value={metaDraft.tags} onChange={(e) => setMetaDraft({ ...metaDraft, tags: e.target.value })} />
                    </div>
                    <div className="space-y-1">
                      <span className="text-muted-foreground">{t("bucketDetail:metadata.customMetadata")}</span>
                      <Textarea rows={3} className="font-mono text-xs" value={metaDraft.metadata} onChange={(e) => setMetaDraft({ ...metaDraft, metadata: e.target.value })} />
                    </div>
                    <div className="flex flex-wrap gap-2 pt-2">
                      <Button size="sm" variant="outline" onClick={async () => {
                        try {
                          const tags = JSON.parse(metaDraft.tags || "{}");
                          const metadata = JSON.parse(metaDraft.metadata || "{}");
                          await api.putObjectMeta(bucket, detailObject.key, {
                            tags, metadata, content_type: metaDraft.contentType,
                          }, detailObject.version_id);
                          toast.success(t("bucketDetail:toast.metadataSaved"));
                          invalidateObjects();
                        } catch (e) {
                          toast.error(e instanceof Error ? e.message : t("bucketDetail:toast.invalidJson"));
                        }
                      }}>{t("bucketDetail:metadata.save")}</Button>
                      <Button size="sm" variant="outline" onClick={async () => {
                        const blob = await api.downloadObject(bucket, detailObject.key, detailObject.version_id);
                        const a = document.createElement("a");
                        a.href = URL.createObjectURL(blob);
                        a.download = detailObject.key.split("/").pop() ?? "download";
                        a.click();
                      }}>{t("bucketDetail:metadata.download")}</Button>
                      <Button size="sm" variant="outline" onClick={() => { setRenameTarget(detailObject); setRenameDest(detailObject.key.replace(prefix, "")); }}><Pencil className="h-3.5 w-3.5" /> {t("bucketDetail:metadata.rename")}</Button>
                      <Button size="sm" variant="outline" onClick={() => { setMoveTarget(detailObject); setMoveDest(""); setMoveDestBucket(bucket); }}>{t("bucketDetail:metadata.move")}</Button>
                      <Button size="sm" variant="outline" onClick={() => { setCopyTarget(detailObject); setCopyDestBucket(bucket); setCopyDestKey(""); }}>{t("bucketDetail:metadata.copy")}</Button>
                      <Button size="sm" variant="outline" onClick={() => { setShareObject(detailObject); setShareUrl(""); }}>{t("bucketDetail:metadata.share")}</Button>
                    </div>
                  </>
                ) : (
                  <p className="text-muted-foreground">{t("bucketDetail:metadata.selectObject")}</p>
                )}
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="versions">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">{t("bucketDetail:versions.title")}</CardTitle>
              <CardDescription>{t("bucketDetail:versions.description")}</CardDescription>
            </CardHeader>
            <CardContent className="p-0">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left text-muted-foreground">
                    <th className="p-3">{t("bucketDetail:metadata.key")}</th>
                    <th className="p-3">{t("bucketDetail:versions.versionId")}</th>
                    <th className="p-3">{t("bucketDetail:table.size")}</th>
                    <th className="p-3">{t("bucketDetail:table.modified")}</th>
                    <th className="p-3" />
                  </tr>
                </thead>
                <tbody>
                  {(versions.data ?? []).map((v) => (
                    <tr key={`${v.key}-${v.version_id}`} className="border-b">
                      <td className="p-3 font-mono">{v.key}</td>
                      <td className="p-3 font-mono text-xs">{v.version_id || "—"}</td>
                      <td className="p-3">{formatBytes(v.size)}</td>
                      <td className="p-3">{formatDate(v.last_modified, i18n.language)}</td>
                      <td className="p-3">
                        <div className="flex gap-1">
                          <Button size="sm" variant="ghost" disabled={!v.version_id} onClick={async () => {
                            const blob = await api.downloadObject(bucket, v.key, v.version_id);
                            const a = document.createElement("a");
                            a.href = URL.createObjectURL(blob);
                            a.download = v.key.split("/").pop() ?? "download";
                            a.click();
                          }}>{t("bucketDetail:metadata.download")}</Button>
                          <Button size="sm" variant="ghost" disabled={!v.version_id} onClick={() => api.objectAction(bucket, { action: "restore", key: v.key, version_id: v.version_id }).then(() => { invalidateObjects(); toast.success(t("bucketDetail:toast.versionRestored")); })}>
                            <RotateCcw className="h-3.5 w-3.5" />
                          </Button>
                          <Button size="sm" variant="ghost" className="text-destructive" disabled={!v.version_id} onClick={() => api.deleteObject(bucket, v.key, { versionId: v.version_id }).then(() => { invalidateObjects(); toast.success(t("bucketDetail:toast.versionDeleted")); }).catch((e) => toast.error(e.message))}>
                            <Trash2 className="h-3.5 w-3.5" />
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                  {!versions.isLoading && (versions.data ?? []).length === 0 && (
                    <tr><td colSpan={5} className="p-8 text-center text-muted-foreground">{t("bucketDetail:versions.noVersions")}</td></tr>
                  )}
                </tbody>
              </table>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="settings">
          {settingsDraft && (
            <div className="space-y-4">
              <div className="flex flex-wrap items-center gap-2">
                {isAdmin && (
                  <Button size="sm" variant="outline" asChild>
                    <Link to={`/admin/settings/buckets?bucket=${encodeURIComponent(bucket)}`}>
                      {t("bucketDetail:settings.openAdmin")}
                    </Link>
                  </Button>
                )}
                {isAdmin && (
                  <Button
                    size="sm"
                    onClick={() => saveSettingsMutation.mutate()}
                    disabled={saveSettingsMutation.isPending}
                  >
                    <Save className="h-4 w-4" />
                    {t("common:save")}
                  </Button>
                )}
              </div>
              <BucketSettingsTabs
                draft={settingsDraft}
                onDraftChange={setSettingsDraft}
                readOnly={!isAdmin}
                showTags
                tagsDraft={bucketTagsDraft}
                onTagsDraftChange={setBucketTagsDraft}
                onSaveTags={async () => {
                  try {
                    const tags = JSON.parse(bucketTagsDraft || "{}");
                    await api.putBucketTags(bucket, tags);
                    toast.success(t("bucketDetail:toast.tagsSaved"));
                    queryClient.invalidateQueries({ queryKey: ["bucket-settings", bucket] });
                  } catch {
                    toast.error(t("bucketDetail:toast.invalidTagsJson"));
                  }
                }}
              />
            </div>
          )}
        </TabsContent>

        {showAccessTab && (
          <TabsContent value="access">
            <Card>
              <CardHeader className="flex flex-row items-center justify-between">
                <div>
                  <CardTitle className="text-base">{t("bucketDetail:access.title")}</CardTitle>
                  <CardDescription>{t("bucketDetail:access.description")}</CardDescription>
                </div>
                <Button
                  size="sm"
                  disabled={bucketAccess.isLoading}
                  onClick={async () => {
                    try {
                      const body = {
                        grants: accessDraft.map((g) => ({
                          user_id: g.user_id,
                          can_read: g.can_read,
                          can_write: g.can_write,
                        })),
                        prefix_grants: prefixAccessDraft
                          .filter((g) => g.user_id && g.prefix?.trim())
                          .map((g) => ({
                            user_id: g.user_id,
                            prefix: g.prefix!.trim(),
                            can_read: g.can_read,
                            can_write: g.can_write,
                          })),
                      };
                      if (useTenantAccessApi) {
                        await api.putBucketAccess(tenantId!, bucket!, body);
                      } else {
                        await api.putBucketAccessByBucket(bucket!, body);
                      }
                      queryClient.invalidateQueries({ queryKey: ["bucket-access", tenantId, bucket] });
                      toast.success(t("bucketDetail:toast.accessSaved"));
                    } catch (e) {
                      toast.error((e as Error).message);
                    }
                  }}
                >
                  {t("bucketDetail:access.save")}
                </Button>
              </CardHeader>
              <CardContent className="space-y-3">
                {(useTenantAccessApi ? tenantMembers.data ?? [] : shareableUsers.data ?? []).map((m) => {
                  const userId = m.user_id;
                  const uname = m.username ?? userId;
                  const row = accessDraft.find((g) => g.user_id === userId) ?? {
                    user_id: userId,
                    username: uname,
                    can_read: false,
                    can_write: false,
                  };
                  const inDraft = accessDraft.some((g) => g.user_id === userId);
                  return (
                    <div key={userId} className="flex flex-wrap items-center gap-4 rounded border p-3 text-sm">
                      <span className="min-w-[8rem] font-medium">{uname}</span>
                      {"role" in m && m.role ? (
                        <span className="text-muted-foreground">{m.role}</span>
                      ) : null}
                      <label className="flex items-center gap-2">
                        <input
                          type="checkbox"
                          checked={inDraft && row.can_read}
                          onChange={(e) => {
                            const on = e.target.checked;
                            setAccessDraft((prev) => {
                              const rest = prev.filter((g) => g.user_id !== userId);
                              if (!on && !row.can_write) return rest;
                              return [...rest, { ...row, can_read: on || row.can_write, can_write: row.can_write }];
                            });
                          }}
                        />
                        {t("bucketDetail:access.read")}
                      </label>
                      <label className="flex items-center gap-2">
                        <input
                          type="checkbox"
                          checked={inDraft && row.can_write}
                          onChange={(e) => {
                            const on = e.target.checked;
                            setAccessDraft((prev) => {
                              const rest = prev.filter((g) => g.user_id !== userId);
                              if (!on && !row.can_read) return rest;
                              return [...rest, { user_id: userId, username: uname, can_read: on || row.can_read, can_write: on }];
                            });
                          }}
                        />
                        {t("bucketDetail:access.write")}
                      </label>
                    </div>
                  );
                })}
                {bucketAccess.isError && (
                  <p className="text-sm text-muted-foreground">{t("bucketDetail:access.loadError")}</p>
                )}

                <div className="space-y-3 border-t pt-4">
                  <div className="flex items-center justify-between">
                    <div>
                      <p className="text-sm font-medium">{t("bucketDetail:access.folderTitle")}</p>
                      <p className="text-xs text-muted-foreground">{t("bucketDetail:access.folderDescription")}</p>
                    </div>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() =>
                        setPrefixAccessDraft((prev) => [
                          ...prev,
                          { user_id: "", username: "", prefix: "", can_read: true, can_write: false },
                        ])
                      }
                    >
                      {t("bucketDetail:access.addFolderShare")}
                    </Button>
                  </div>
                  {prefixAccessDraft.map((row, idx) => (
                    <div key={`${row.user_id}-${row.prefix}-${idx}`} className="flex flex-wrap items-center gap-3 rounded border p-3 text-sm">
                      <Input
                        className="max-w-[10rem] font-mono text-xs"
                        placeholder={t("bucketDetail:access.folderPrefix")}
                        value={row.prefix ?? ""}
                        onChange={(e) =>
                          setPrefixAccessDraft((prev) =>
                            prev.map((g, i) => (i === idx ? { ...g, prefix: e.target.value } : g))
                          )
                        }
                      />
                      <Select
                        value={row.user_id || undefined}
                        onValueChange={(userId) => {
                          const users = useTenantAccessApi ? tenantMembers.data ?? [] : shareableUsers.data ?? [];
                          const u = users.find((m) => m.user_id === userId);
                          setPrefixAccessDraft((prev) =>
                            prev.map((g, i) =>
                              i === idx ? { ...g, user_id: userId, username: u?.username ?? userId } : g
                            )
                          );
                        }}
                      >
                        <SelectTrigger className="w-[10rem]">
                          <SelectValue placeholder={t("bucketDetail:access.selectUser")} />
                        </SelectTrigger>
                        <SelectContent>
                          {(useTenantAccessApi ? tenantMembers.data ?? [] : shareableUsers.data ?? []).map((m) => (
                            <SelectItem key={m.user_id} value={m.user_id}>
                              {m.username ?? m.user_id}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <label className="flex items-center gap-2">
                        <input
                          type="checkbox"
                          checked={row.can_read}
                          onChange={(e) =>
                            setPrefixAccessDraft((prev) =>
                              prev.map((g, i) =>
                                i === idx ? { ...g, can_read: e.target.checked || g.can_write } : g
                              )
                            )
                          }
                        />
                        {t("bucketDetail:access.read")}
                      </label>
                      <label className="flex items-center gap-2">
                        <input
                          type="checkbox"
                          checked={row.can_write}
                          onChange={(e) =>
                            setPrefixAccessDraft((prev) =>
                              prev.map((g, i) =>
                                i === idx
                                  ? { ...g, can_write: e.target.checked, can_read: e.target.checked || g.can_read }
                                  : g
                              )
                            )
                          }
                        />
                        {t("bucketDetail:access.write")}
                      </label>
                      <Button
                        size="sm"
                        variant="ghost"
                        className="text-destructive"
                        onClick={() => setPrefixAccessDraft((prev) => prev.filter((_, i) => i !== idx))}
                      >
                        {t("common:delete")}
                      </Button>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>
          </TabsContent>
        )}

        <TabsContent value="lifecycle">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between">
              <div>
                <CardTitle className="text-base">{t("bucketDetail:lifecycle.title")}</CardTitle>
                <CardDescription>{t("bucketDetail:lifecycle.description")}</CardDescription>
              </div>
              <div className="flex gap-2">
                <Button size="sm" variant="outline" onClick={() => setRuleDialog({ id: crypto.randomUUID(), action: "expire", expiration_days: 30, enabled: true })}>{t("bucketDetail:lifecycle.addRule")}</Button>
                <Button size="sm" onClick={() => saveLifecycleMutation.mutate(lifecycleDraft)} disabled={saveLifecycleMutation.isPending}>{t("common:save")}</Button>
              </div>
            </CardHeader>
            <CardContent className="p-0">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left text-muted-foreground">
                    <th className="p-3">{t("bucketDetail:table.name")}</th>
                    <th className="p-3">{t("bucketDetail:lifecycle.columns.prefix")}</th>
                    <th className="p-3">{t("bucketDetail:lifecycle.columns.action")}</th>
                    <th className="p-3">{t("bucketDetail:lifecycle.columns.days")}</th>
                    <th className="p-3">{t("bucketDetail:lifecycle.columns.status")}</th>
                    <th className="p-3" />
                  </tr>
                </thead>
                <tbody>
                  {lifecycleDraft.map((rule) => (
                    <tr key={rule.id} className="border-b">
                      <td className="p-3">{rule.name || rule.id}</td>
                      <td className="p-3 font-mono">{rule.prefix || "*"}</td>
                      <td className="p-3">{LIFECYCLE_ACTIONS.find((a) => a.value === (rule.action || "expire"))?.label ?? rule.action}</td>
                      <td className="p-3">{rule.expiration_days}</td>
                      <td className="p-3">{rule.enabled ? t("common:enabled") : t("common:disabled")}</td>
                      <td className="p-3">
                        <Button size="sm" variant="ghost" onClick={() => setRuleDialog({ ...rule })}>{t("bucketDetail:lifecycle.edit")}</Button>
                        <Button size="sm" variant="ghost" className="text-destructive" onClick={() => setLifecycleDraft((r) => r.filter((x) => x.id !== rule.id))}>{t("common:delete")}</Button>
                      </td>
                    </tr>
                  ))}
                  {lifecycleDraft.length === 0 && (
                    <tr><td colSpan={6} className="p-8 text-center text-muted-foreground">{t("bucketDetail:lifecycle.empty")}</td></tr>
                  )}
                </tbody>
              </table>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="trash">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">{t("bucketDetail:trash.title")}</CardTitle>
              <CardDescription>{t("bucketDetail:trash.description")}</CardDescription>
            </CardHeader>
            <CardContent className="p-0">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left text-muted-foreground">
                    <th className="p-3">{t("bucketDetail:metadata.key")}</th>
                    <th className="p-3">{t("bucketDetail:table.size")}</th>
                    <th className="p-3">{t("bucketDetail:trash.columns.deleted")}</th>
                    <th className="p-3">{t("bucketDetail:trash.columns.by")}</th>
                    <th className="p-3" />
                  </tr>
                </thead>
                <tbody>
                  {(trash.data ?? []).map((tr: TrashItem) => (
                    <tr key={tr.id} className="border-b">
                      <td className="p-3 font-mono">{tr.original_key}</td>
                      <td className="p-3">{formatBytes(tr.size)}</td>
                      <td className="p-3">{formatDate(tr.deleted_at, i18n.language)}</td>
                      <td className="p-3">{tr.deleted_by || "—"}</td>
                      <td className="p-3">
                        <div className="flex gap-1">
                          <Button size="sm" variant="ghost" onClick={() => api.restoreTrash(tr.id).then(() => { trash.refetch(); invalidateObjects(); toast.success(t("bucketDetail:toast.restored")); }).catch((e) => toast.error(e.message))}>
                            <RotateCcw className="h-3.5 w-3.5" /> {t("bucketDetail:trash.restore")}
                          </Button>
                          <Button size="sm" variant="ghost" className="text-destructive" onClick={() => api.purgeTrash(tr.id).then(() => { trash.refetch(); toast.success(t("bucketDetail:toast.permanentlyDeleted")); }).catch((e) => toast.error(e.message))}>
                            <Trash2 className="h-3.5 w-3.5" />
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                  {!trash.isLoading && (trash.data ?? []).length === 0 && (
                    <tr><td colSpan={5} className="p-8 text-center text-muted-foreground">{t("bucketDetail:trash.empty")}</td></tr>
                  )}
                </tbody>
              </table>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      {/* Share dialog */}
      <Dialog open={!!shareObject} onOpenChange={(o) => !o && setShareObject(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2"><Link2 className="h-4 w-4" /> {t("bucketDetail:share.title")}</DialogTitle>
            <DialogDescription>{t("bucketDetail:share.description", { key: shareObject?.key })}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>{t("bucketDetail:share.expiration")}</Label>
              <Select value={shareExpires} onValueChange={setShareExpires}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  {PRESIGN_PRESETS.map((p) => (
                    <SelectItem key={p.seconds} value={String(p.seconds)}>{p.label}</SelectItem>
                  ))}
                  <SelectItem value="custom">{t("bucketDetail:share.customSeconds")}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            {shareExpires === "custom" && (
              <Input type="number" placeholder={t("bucketDetail:placeholder.seconds")} value={customExpires} onChange={(e) => setCustomExpires(e.target.value)} />
            )}
            <div className="space-y-2">
              <Label>{t("bucketDetail:share.maxDownloads")}</Label>
              <Input type="number" min={0} value={shareMaxDownloads} onChange={(e) => setShareMaxDownloads(e.target.value)} />
            </div>
            {shareUrl && (
              <div className="space-y-2">
                <div className="flex gap-2">
                  <Input readOnly value={shareUrl} className="font-mono text-xs" />
                  <CopyButton value={shareUrl} label={t("bucketDetail:share.url")} />
                </div>
                {createdShare && createdShare.max_downloads > 0 && (
                  <p className="text-xs text-muted-foreground">
                    {t("bucketDetail:share.downloadsRemaining", {
                      remaining: createdShare.max_downloads - createdShare.download_count,
                      max: createdShare.max_downloads,
                    })}
                  </p>
                )}
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShareObject(null)}>{t("common:close")}</Button>
            <Button onClick={() => shareObject && shareLinkMutation.mutate(shareObject)} disabled={shareLinkMutation.isPending}>{t("bucketDetail:share.createLink")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Folder dialog */}
      <Dialog open={folderOpen} onOpenChange={setFolderOpen}>
        <DialogContent>
          <DialogHeader><DialogTitle>{t("bucketDetail:folder.title")}</DialogTitle></DialogHeader>
          <Input placeholder={t("bucketDetail:folder.placeholder")} value={folderName} onChange={(e) => setFolderName(e.target.value)} />
          <DialogFooter>
            <Button variant="outline" onClick={() => setFolderOpen(false)}>{t("common:cancel")}</Button>
            <Button onClick={() => createFolderMutation.mutate(folderName)} disabled={!folderName.trim()}>{t("common:create")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Copy dialog */}
      <Dialog open={!!copyTarget} onOpenChange={(o) => !o && setCopyTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2"><Copy className="h-4 w-4" /> {t("bucketDetail:copy.title")}</DialogTitle>
            <DialogDescription>{t("bucketDetail:copy.description", { key: copyTarget?.key })}</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-2">
              <Label>{t("bucketDetail:fields.destBucket")}</Label>
              <Select value={copyDestBucket} onValueChange={setCopyDestBucket}>
                <SelectTrigger><SelectValue placeholder={t("bucketDetail:placeholder.bucket")} /></SelectTrigger>
                <SelectContent>
                  {(buckets.data ?? [{ name: bucket }]).map((b) => (
                    <SelectItem key={b.name} value={b.name}>{b.name}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>{t("bucketDetail:fields.destKey")}</Label>
              <Input placeholder={t("bucketDetail:placeholder.objectPath")} value={copyDestKey} onChange={(e) => setCopyDestKey(e.target.value)} />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCopyTarget(null)}>{t("common:cancel")}</Button>
            <Button onClick={() => copyTarget && copyDestKey && api.objectAction(bucket, { action: "copy", key: copyTarget.key, dest_bucket: copyDestBucket || bucket, dest_key: copyDestKey }).then(() => { invalidateObjects(); setCopyTarget(null); toast.success(t("bucketDetail:toast.copied")); }).catch((e) => toast.error(e.message))} disabled={!copyDestKey.trim()}>{t("bucketDetail:metadata.copy")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Move dialog */}
      <Dialog open={!!moveTarget} onOpenChange={(o) => !o && setMoveTarget(null)}>
        <DialogContent>
          <DialogHeader><DialogTitle>{t("bucketDetail:move.title")}</DialogTitle></DialogHeader>
          <div className="space-y-3">
            <div className="space-y-2">
              <Label>{t("bucketDetail:fields.destBucket")}</Label>
              <Select value={moveDestBucket || bucket} onValueChange={setMoveDestBucket}>
                <SelectTrigger><SelectValue placeholder={t("bucketDetail:placeholder.bucket")} /></SelectTrigger>
                <SelectContent>
                  {(buckets.data ?? [{ name: bucket }]).map((b) => (
                    <SelectItem key={b.name} value={b.name}>{b.name}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>{t("bucketDetail:fields.destKey")}</Label>
              <Input placeholder={t("bucketDetail:placeholder.objectPath")} value={moveDest} onChange={(e) => setMoveDest(e.target.value)} />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setMoveTarget(null)}>{t("common:cancel")}</Button>
            <Button onClick={() => moveTarget && moveDest && api.objectAction(bucket, {
              action: "move",
              key: moveTarget.key,
              dest_bucket: moveDestBucket || bucket,
              dest_key: moveDest,
            }).then(() => { invalidateObjects(); setMoveTarget(null); toast.success(t("bucketDetail:toast.moved")); }).catch((e) => toast.error(e.message))} disabled={!moveDest.trim()}>{t("bucketDetail:metadata.move")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete folder dialog */}
      <Dialog open={!!deleteFolderTarget} onOpenChange={(o) => !o && setDeleteFolderTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("bucketDetail:deleteFolder.title")}</DialogTitle>
            <DialogDescription>
              {t("bucketDetail:deleteFolder.description", { name: deleteFolderTarget?.replace(prefix, "").replace(/\/$/, "") })}
            </DialogDescription>
          </DialogHeader>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={deleteFolderRecursive}
              onChange={(e) => setDeleteFolderRecursive(e.target.checked)}
            />
            {t("bucketDetail:deleteFolder.recursive")}
          </label>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteFolderTarget(null)}>{t("common:cancel")}</Button>
            <Button
              variant="destructive"
              onClick={async () => {
                if (!deleteFolderTarget) return;
                try {
                  const res = await api.deleteFolder(bucket, deleteFolderTarget, deleteFolderRecursive);
                  invalidateObjects();
                  setDeleteFolderTarget(null);
                  toast.success(t("bucketDetail:toast.folderDeleted", { count: res.deleted }));
                } catch (e) {
                  const msg = e instanceof Error ? e.message : t("bucketDetail:toast.deleteFailed");
                  if (msg.includes("not empty") || msg.includes("409")) {
                    toast.error(t("bucketDetail:toast.folderNotEmpty"));
                  } else {
                    toast.error(msg);
                  }
                }
              }}
            >
              {t("common:delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Lifecycle rule dialog */}
      <Dialog open={!!ruleDialog} onOpenChange={(o) => !o && setRuleDialog(null)}>
        <DialogContent>
          <DialogHeader><DialogTitle>{lifecycleDraft.some((r) => r.id === ruleDialog?.id) ? t("bucketDetail:lifecycleRule.edit") : t("bucketDetail:lifecycleRule.new")}</DialogTitle></DialogHeader>
          {ruleDialog && (
            <div className="space-y-3">
              <div><Label>{t("common:name")}</Label><Input value={ruleDialog.name ?? ""} onChange={(e) => setRuleDialog({ ...ruleDialog, name: e.target.value })} /></div>
              <div><Label>{t("bucketDetail:lifecycle.columns.prefix")}</Label><Input value={ruleDialog.prefix ?? ""} onChange={(e) => setRuleDialog({ ...ruleDialog, prefix: e.target.value })} placeholder="logs/" /></div>
              <div><Label>{t("bucketDetail:lifecycle.columns.action")}</Label>
                <Select value={ruleDialog.action || "expire"} onValueChange={(v) => setRuleDialog({ ...ruleDialog, action: v })}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>{LIFECYCLE_ACTIONS.map((a) => <SelectItem key={a.value} value={a.value}>{a.label}</SelectItem>)}</SelectContent>
                </Select>
              </div>
              <div><Label>{t("bucketDetail:lifecycle.columns.days")}</Label><Input type="number" value={ruleDialog.expiration_days} onChange={(e) => setRuleDialog({ ...ruleDialog, expiration_days: parseInt(e.target.value, 10) || 0 })} /></div>
              <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={ruleDialog.enabled} onChange={(e) => setRuleDialog({ ...ruleDialog, enabled: e.target.checked })} />{t("common:enabled")}</label>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setRuleDialog(null)}>{t("common:cancel")}</Button>
            <Button onClick={() => {
              if (!ruleDialog) return;
              setLifecycleDraft((prev) => {
                const idx = prev.findIndex((r) => r.id === ruleDialog.id);
                if (idx >= 0) { const next = [...prev]; next[idx] = ruleDialog; return next; }
                return [...prev, ruleDialog];
              });
              setRuleDialog(null);
            }}>{t("bucketDetail:lifecycleRule.save")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Rename dialog */}
      <Dialog open={!!renameTarget} onOpenChange={(o) => !o && setRenameTarget(null)}>
        <DialogContent>
          <DialogHeader><DialogTitle>{t("bucketDetail:rename.title")}</DialogTitle></DialogHeader>
          <Input placeholder={t("bucketDetail:rename.placeholder")} value={renameDest} onChange={(e) => setRenameDest(e.target.value)} />
          <DialogFooter>
            <Button variant="outline" onClick={() => setRenameTarget(null)}>{t("common:cancel")}</Button>
            <Button onClick={() => renameTarget && renameDest && api.objectAction(bucket, {
              action: "rename",
              key: renameTarget.key,
              dest_key: prefix ? `${prefix}${renameDest}` : renameDest,
            }).then(() => { invalidateObjects(); setRenameTarget(null); setDetailObject(null); toast.success(t("bucketDetail:toast.renamed")); }).catch((e) => toast.error(e.message))}>{t("bucketDetail:metadata.rename")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete dialog */}
      <Dialog open={!!deleteTarget} onOpenChange={(o) => !o && setDeleteTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("bucketDetail:deleteObject.title")}</DialogTitle>
            <DialogDescription>{t("bucketDetail:deleteObject.description", { key: deleteTarget?.key })}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <label className="flex items-center gap-2 text-sm"><input type="radio" checked={deleteMode === "now"} onChange={() => setDeleteMode("now")} />{t("bucketDetail:deleteObject.now")}</label>
            <label className="flex items-center gap-2 text-sm"><input type="radio" checked={deleteMode === "scheduled"} onChange={() => setDeleteMode("scheduled")} />{t("bucketDetail:deleteObject.schedule")}</label>
            {deleteMode === "scheduled" && (
              <Select value={scheduleDuration} onValueChange={(v) => setScheduleDuration(v as "1d" | "1w" | "1m")}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="1d">{t("bucketDetail:schedule.1d")}</SelectItem>
                  <SelectItem value="1w">{t("bucketDetail:schedule.1w")}</SelectItem>
                  <SelectItem value="1m">{t("bucketDetail:schedule.1m")}</SelectItem>
                </SelectContent>
              </Select>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteTarget(null)}>{t("common:cancel")}</Button>
            <Button variant="destructive" onClick={() => deleteTarget && deleteMutation.mutate({ key: deleteTarget.key, mode: deleteMode, schedule: deleteMode === "scheduled" ? scheduleDuration : undefined })}>{t("common:delete")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
