import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { RefreshCw, Save } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { api, type BucketSettings } from "@/lib/api";
import {
  type ApplyMode,
  type EditableBucketField,
  buildUpdatePayloadForBucket,
  isBucketConfigured,
  mergeBucketSettings,
} from "@/lib/bucket-settings-merge";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { BucketSettingsTabs } from "@/components/settings/BucketSettingsTabs";
import { BucketSettingsPicker } from "@/components/settings/BucketSettingsPicker";
import { ConfirmDialog } from "@/components/confirm-dialog";

function parseBucketsFromQuery(searchParams: URLSearchParams, known: Set<string>): string[] {
  const multi = searchParams.get("buckets");
  if (multi) {
    return multi
      .split(",")
      .map((s) => s.trim())
      .filter((name) => name && known.has(name));
  }
  const single = searchParams.get("bucket");
  if (single && known.has(single)) return [single];
  return [];
}

function syncBucketsQuery(
  searchParams: URLSearchParams,
  setSearchParams: (next: URLSearchParams, opts?: { replace?: boolean }) => void,
  names: string[]
) {
  const next = new URLSearchParams(searchParams);
  next.delete("bucket");
  next.delete("buckets");
  if (names.length === 1) {
    next.set("bucket", names[0]);
  } else if (names.length > 1) {
    next.set("buckets", names.join(","));
  }
  setSearchParams(next, { replace: true });
}

export function BucketSettingsPage() {
  const { t } = useTranslation(["settings", "common"]);
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const [selectedNames, setSelectedNames] = useState<Set<string>>(new Set());
  const [draft, setDraft] = useState<BucketSettings | null>(null);
  const [mixedFields, setMixedFields] = useState<Set<EditableBucketField>>(new Set());
  const [clearedMixed, setClearedMixed] = useState<Set<EditableBucketField>>(new Set());
  const [applyMode, setApplyMode] = useState<ApplyMode>("overwrite");
  const [confirmOpen, setConfirmOpen] = useState(false);
  const dirtyRef = useRef(false);
  const serverSnapshotRef = useRef<string>("");

  const settings = useQuery({
    queryKey: ["bucket-settings"],
    queryFn: async () => (await api.listBucketSettings()).buckets,
  });

  const knownNames = useMemo(
    () => new Set(settings.data?.map((b) => b.name) ?? []),
    [settings.data]
  );

  const selectedBuckets = useMemo(() => {
    if (!settings.data?.length) return [];
    return settings.data.filter((b) => selectedNames.has(b.name));
  }, [settings.data, selectedNames]);

  useEffect(() => {
    if (!settings.data?.length) return;
    const fromQuery = parseBucketsFromQuery(searchParams, knownNames);
    if (fromQuery.length > 0) {
      setSelectedNames(new Set(fromQuery));
      return;
    }
    if (selectedNames.size === 0) {
      setSelectedNames(new Set([settings.data[0].name]));
    }
  }, [settings.data, searchParams, knownNames]);

  const selectionKey = useMemo(() => [...selectedNames].sort().join(","), [selectedNames]);

  useEffect(() => {
    if (selectedBuckets.length === 0) {
      setDraft(null);
      setMixedFields(new Set());
      setClearedMixed(new Set());
      dirtyRef.current = false;
      return;
    }
    const { draft: merged, mixed } = mergeBucketSettings(selectedBuckets);
    setDraft(merged);
    setMixedFields(mixed);
    setClearedMixed(new Set());
    serverSnapshotRef.current = JSON.stringify({ merged, mixed: [...mixed] });
    dirtyRef.current = false;
  }, [selectionKey, settings.data]);

  const handleSelectionChange = useCallback(
    (next: Set<string>) => {
      if (dirtyRef.current) {
        const ok = window.confirm(t("settings:buckets.confirmDiscard"));
        if (!ok) return;
      }
      setSelectedNames(next);
      syncBucketsQuery(searchParams, setSearchParams, [...next]);
    },
    [searchParams, setSearchParams]
  );

  const handleDraftChange = (next: BucketSettings) => {
    dirtyRef.current = true;
    setDraft(next);
  };

  const handleFieldCommit = (field: EditableBucketField) => {
    setClearedMixed((prev) => new Set(prev).add(field));
    setMixedFields((prev) => {
      const next = new Set(prev);
      next.delete(field);
      return next;
    });
  };

  const runSave = async () => {
    if (!draft || selectedBuckets.length === 0) return;

    const updates = selectedBuckets.map((bucket) => ({
      name: bucket.name,
      body: buildUpdatePayloadForBucket(draft, bucket, applyMode, mixedFields, clearedMixed),
    }));

    const stillMixed = [...mixedFields].filter((f) => !clearedMixed.has(f));
    if (stillMixed.length > 0) {
      toast.error(t("settings:buckets.toast.resolveMixed", { fields: stillMixed.join(", ") }));
      return;
    }

    const result = await api.batchUpdateBucketSettings(updates);
    queryClient.invalidateQueries({ queryKey: ["bucket-settings"] });
    dirtyRef.current = false;

    if (result.failed.length === 0) {
      toast.success(t("settings:buckets.toast.saved", { ok: result.ok, total: selectedBuckets.length }));
    } else if (result.ok > 0) {
      toast.warning(
        t("settings:buckets.toast.partial", {
          ok: result.ok,
          total: selectedBuckets.length,
          names: result.failed.map((f) => f.name).join(", "),
        })
      );
    } else {
      toast.error(result.failed[0]?.error ?? t("settings:buckets.toast.failed"));
    }
  };

  const saveMutation = useMutation({
    mutationFn: runSave,
    onError: (err: Error) => toast.error(err.message),
  });

  const requestSave = () => {
    if (!draft || selectedBuckets.length === 0) return;

    const needsConfirm =
      applyMode === "overwrite" &&
      selectedBuckets.length > 1 &&
      selectedBuckets.some((b) => isBucketConfigured(b));

    if (needsConfirm) {
      setConfirmOpen(true);
      return;
    }
    saveMutation.mutate();
  };

  if (settings.isLoading) {
    return <p className="text-muted-foreground">{t("settings:buckets.loading")}</p>;
  }

  if (!settings.data?.length) {
    return (
      <div>
        <PageHeader
          title={t("settings:buckets.title")}
          description={t("settings:buckets.description")}
        />
        <p className="text-muted-foreground">{t("settings:buckets.empty")}</p>
      </div>
    );
  }

  return (
    <div>
      <PageHeader
        title={t("settings:buckets.title")}
        description={t("settings:buckets.description")}
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => settings.refetch()} disabled={settings.isFetching}>
              <RefreshCw className={`h-4 w-4 ${settings.isFetching ? "animate-spin" : ""}`} />
              {t("common:refresh")}
            </Button>
            <Button
              size="sm"
              onClick={requestSave}
              disabled={saveMutation.isPending || !draft || selectedNames.size === 0}
            >
              <Save className="h-4 w-4" />
              {t("common:save")}
            </Button>
          </>
        }
      />

      <div className="grid gap-6 lg:grid-cols-[minmax(220px,280px)_1fr]">
        <BucketSettingsPicker
          buckets={settings.data}
          selected={selectedNames}
          onSelectionChange={handleSelectionChange}
        />

        <div className="min-w-0 space-y-4">
          {selectedNames.size === 0 ? (
            <p className="text-muted-foreground">{t("settings:buckets.selectOne")}</p>
          ) : (
            <>
              <div className="flex flex-wrap items-end gap-4">
                <div className="space-y-2 max-w-xs">
                  <Label>{t("settings:buckets.applyMode.label")}</Label>
                  <Select value={applyMode} onValueChange={(v) => setApplyMode(v as ApplyMode)}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="overwrite">{t("settings:buckets.applyMode.overwrite")}</SelectItem>
                      <SelectItem value="only_empty">{t("settings:buckets.applyMode.onlyEmpty")}</SelectItem>
                    </SelectContent>
                  </Select>
                  <p className="text-xs text-muted-foreground">
                    {applyMode === "overwrite"
                      ? t("settings:buckets.applyMode.overwriteHint")
                      : t("settings:buckets.applyMode.onlyEmptyHint")}
                  </p>
                </div>
              </div>

              {draft && (
                <BucketSettingsTabs
                  draft={draft}
                  onDraftChange={handleDraftChange}
                  mixedFields={mixedFields}
                  selectionCount={selectedNames.size}
                  onFieldCommit={handleFieldCommit}
                />
              )}
            </>
          )}
        </div>
      </div>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t("settings:buckets.confirm.title", { count: selectedNames.size })}
        description={t("settings:buckets.confirm.description")}
        confirmLabel={t("settings:buckets.confirm.apply")}
        loading={saveMutation.isPending}
        destructive
        onConfirm={() => {
          setConfirmOpen(false);
          saveMutation.mutate();
        }}
      />
    </div>
  );
}
