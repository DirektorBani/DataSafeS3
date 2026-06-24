import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { type BucketSettings, RETENTION_PRESETS, STORAGE_CLASSES } from "@/lib/api";
import {
  type EditableBucketField,
  MIXED_SELECT,
} from "@/lib/bucket-settings-merge";
import { formatBytes } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

export type BucketSettingsTabsProps = {
  draft: BucketSettings;
  onDraftChange: (draft: BucketSettings) => void;
  mixedFields?: Set<EditableBucketField>;
  selectionCount?: number;
  onFieldCommit?: (field: EditableBucketField) => void;
  readOnly?: boolean;
  tagsDraft?: string;
  onTagsDraftChange?: (value: string) => void;
  onSaveTags?: () => void;
  tagsSaving?: boolean;
  showTags?: boolean;
};

function MixedHint({ count, label }: { count: number; label: string }) {
  if (count < 2) return null;
  return <p className="text-xs text-amber-600 dark:text-amber-400">{label}</p>;
}

function MixedCheckbox({
  checked,
  mixed,
  onChange,
  disabled,
  label,
  mixedLabel,
  count,
  mixedCountLabel,
}: {
  checked: boolean;
  mixed: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
  label: string;
  mixedLabel: string;
  count: number;
  mixedCountLabel: string;
}) {
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (ref.current) ref.current.indeterminate = mixed;
  }, [mixed]);

  return (
    <div className="space-y-1">
      <label className="flex items-center gap-3">
        <input
          ref={ref}
          type="checkbox"
          className="h-4 w-4"
          checked={mixed ? false : checked}
          onChange={(e) => onChange(e.target.checked)}
          disabled={disabled}
        />
        <Label>{mixed ? mixedLabel : label}</Label>
      </label>
      {mixed && <MixedHint count={count} label={mixedCountLabel} />}
    </div>
  );
}

export function BucketSettingsTabs({
  draft,
  onDraftChange,
  mixedFields,
  selectionCount = 1,
  onFieldCommit,
  readOnly = false,
  tagsDraft,
  onTagsDraftChange,
  onSaveTags,
  tagsSaving,
  showTags = false,
}: BucketSettingsTabsProps) {
  const { t } = useTranslation(["settings", "common"]);
  const disabled = readOnly;
  const mixed = mixedFields ?? new Set<EditableBucketField>();
  const count = selectionCount;
  const multipleValues = t("settings:bucketTabs.multipleValues");
  const multipleValuesCount = t("settings:bucketTabs.multipleValuesCount", { count });

  const patch = (field: EditableBucketField, next: BucketSettings) => {
    onFieldCommit?.(field);
    onDraftChange(next);
  };

  const isMixed = (field: EditableBucketField) => mixed.has(field);

  return (
    <Tabs defaultValue="general">
      <TabsList>
        <TabsTrigger value="general">{t("settings:bucketTabs.general")}</TabsTrigger>
        <TabsTrigger value="versioning">{t("settings:bucketTabs.versioning")}</TabsTrigger>
        <TabsTrigger value="object-lock">{t("settings:bucketTabs.objectLock")}</TabsTrigger>
        <TabsTrigger value="storage-class">{t("settings:bucketTabs.storageClass")}</TabsTrigger>
        <TabsTrigger value="lifecycle">{t("settings:bucketTabs.lifecycle")}</TabsTrigger>
        <TabsTrigger value="visibility">{t("settings:bucketTabs.visibility")}</TabsTrigger>
        <TabsTrigger value="quotas">{t("settings:bucketTabs.quotas")}</TabsTrigger>
      </TabsList>

      <TabsContent value="general">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("settings:bucketTabs.generalTitle")}</CardTitle>
            <CardDescription>{t("settings:bucketTabs.owner", { owner: draft.owner })}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label>{t("settings:bucketTabs.description")}</Label>
              <Textarea
                value={isMixed("description") ? "" : draft.description}
                placeholder={isMixed("description") ? multipleValues : undefined}
                onChange={(e) => patch("description", { ...draft, description: e.target.value })}
                rows={3}
                disabled={disabled}
              />
              {isMixed("description") && <MixedHint count={count} label={multipleValuesCount} />}
            </div>
            {showTags && tagsDraft !== undefined && onTagsDraftChange && (
              <div className="space-y-2">
                <Label>{t("settings:bucketTabs.tags")}</Label>
                <Textarea
                  rows={3}
                  className="font-mono text-xs"
                  value={tagsDraft}
                  onChange={(e) => onTagsDraftChange(e.target.value)}
                  disabled={disabled}
                />
                {onSaveTags && (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={onSaveTags}
                    disabled={disabled || tagsSaving}
                    title={disabled ? t("settings:bucketTabs.adminOnly") : undefined}
                  >
                    {t("settings:bucketTabs.saveTags")}
                  </Button>
                )}
              </div>
            )}
          </CardContent>
        </Card>
      </TabsContent>

      <TabsContent value="versioning">
        <Card>
          <CardHeader><CardTitle className="text-base">{t("settings:bucketTabs.versioning")}</CardTitle></CardHeader>
          <CardContent>
            <MixedCheckbox
              checked={draft.versioning_enabled}
              mixed={isMixed("versioning_enabled")}
              onChange={(v) => patch("versioning_enabled", { ...draft, versioning_enabled: v })}
              disabled={disabled}
              label={t("settings:bucketTabs.enableVersioning")}
              mixedLabel={multipleValues}
              count={count}
              mixedCountLabel={multipleValuesCount}
            />
          </CardContent>
        </Card>
      </TabsContent>

      <TabsContent value="object-lock">
        <Card>
          <CardHeader><CardTitle className="text-base">{t("settings:bucketTabs.objectLockTitle")}</CardTitle></CardHeader>
          <CardContent className="space-y-4">
            <MixedCheckbox
              checked={draft.object_lock_enabled}
              mixed={isMixed("object_lock_enabled")}
              onChange={(v) => patch("object_lock_enabled", { ...draft, object_lock_enabled: v })}
              disabled={disabled}
              label={t("settings:bucketTabs.enableWorm")}
              mixedLabel={multipleValues}
              count={count}
              mixedCountLabel={multipleValuesCount}
            />
            {(draft.object_lock_enabled || isMixed("object_lock_enabled")) && (
              <div className="space-y-2 max-w-xs">
                <Label>{t("settings:bucketTabs.defaultRetention")}</Label>
                <Select
                  value={
                    isMixed("retention_days")
                      ? MIXED_SELECT
                      : String(draft.retention_days || 30)
                  }
                  onValueChange={(v) => {
                    if (v === MIXED_SELECT) return;
                    patch("retention_days", { ...draft, retention_days: parseInt(v, 10) });
                  }}
                  disabled={disabled}
                >
                  <SelectTrigger><SelectValue placeholder={multipleValues} /></SelectTrigger>
                  <SelectContent>
                    {isMixed("retention_days") && (
                      <SelectItem value={MIXED_SELECT} disabled>
                        {multipleValues}
                      </SelectItem>
                    )}
                    {RETENTION_PRESETS.map((p) => (
                      <SelectItem key={p.days} value={String(p.days)}>{p.label}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {isMixed("retention_days") && <MixedHint count={count} label={multipleValuesCount} />}
              </div>
            )}
          </CardContent>
        </Card>
      </TabsContent>

      <TabsContent value="storage-class">
        <Card>
          <CardHeader><CardTitle className="text-base">{t("settings:bucketTabs.storageClassTitle")}</CardTitle></CardHeader>
          <CardContent>
            <Select
              value={isMixed("storage_class") ? MIXED_SELECT : (draft.storage_class || "hot")}
              onValueChange={(v) => {
                if (v === MIXED_SELECT) return;
                patch("storage_class", { ...draft, storage_class: v });
              }}
              disabled={disabled}
            >
              <SelectTrigger className="max-w-xs"><SelectValue placeholder={multipleValues} /></SelectTrigger>
              <SelectContent>
                {isMixed("storage_class") && (
                  <SelectItem value={MIXED_SELECT} disabled>
                    {multipleValues}
                  </SelectItem>
                )}
                {STORAGE_CLASSES.map((sc) => (
                  <SelectItem key={sc.value} value={sc.value}>{sc.label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
            {isMixed("storage_class") && <MixedHint count={count} label={multipleValuesCount} />}
          </CardContent>
        </Card>
      </TabsContent>

      <TabsContent value="lifecycle">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("settings:bucketTabs.lifecycleTitle")}</CardTitle>
            <CardDescription>{t("settings:bucketTabs.lifecycleDescription")}</CardDescription>
          </CardHeader>
          <CardContent>
            <Textarea
              className="font-mono text-xs"
              rows={8}
              value={
                isMixed("lifecycle_rules")
                  ? ""
                  : JSON.stringify(draft.lifecycle_rules ?? [], null, 2)
              }
              placeholder={isMixed("lifecycle_rules") ? t("settings:bucketTabs.lifecyclePlaceholder") : undefined}
              onChange={(e) => {
                try {
                  const rules = JSON.parse(e.target.value);
                  patch("lifecycle_rules", { ...draft, lifecycle_rules: rules });
                } catch {
                  /* invalid json while typing */
                }
              }}
              disabled={disabled}
            />
            {isMixed("lifecycle_rules") && <MixedHint count={count} label={multipleValuesCount} />}
          </CardContent>
        </Card>
      </TabsContent>

      <TabsContent value="visibility">
        <Card>
          <CardHeader><CardTitle className="text-base">{t("settings:bucketTabs.visibilityTitle")}</CardTitle></CardHeader>
          <CardContent>
            <Select
              value={isMixed("visibility") ? MIXED_SELECT : (draft.visibility || "private")}
              onValueChange={(v) => {
                if (v === MIXED_SELECT) return;
                patch("visibility", { ...draft, visibility: v });
              }}
              disabled={disabled}
            >
              <SelectTrigger className="max-w-xs"><SelectValue placeholder={multipleValues} /></SelectTrigger>
              <SelectContent>
                {isMixed("visibility") && (
                  <SelectItem value={MIXED_SELECT} disabled>
                    {multipleValues}
                  </SelectItem>
                )}
                <SelectItem value="private">{t("settings:bucketTabs.private")}</SelectItem>
                <SelectItem value="public-read">{t("settings:bucketTabs.publicRead")}</SelectItem>
              </SelectContent>
            </Select>
            {isMixed("visibility") && <MixedHint count={count} label={multipleValuesCount} />}
          </CardContent>
        </Card>
      </TabsContent>

      <TabsContent value="quotas">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("settings:bucketTabs.quotasTitle")}</CardTitle>
            <CardDescription>{t("settings:bucketTabs.quotasDescription")}</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label>{t("settings:bucketTabs.maxSize")}</Label>
              <Input
                type="number"
                value={isMixed("max_size_bytes") ? "" : (draft.max_size_bytes || "")}
                placeholder={isMixed("max_size_bytes") ? multipleValues : undefined}
                onChange={(e) =>
                  patch("max_size_bytes", {
                    ...draft,
                    max_size_bytes: parseInt(e.target.value, 10) || 0,
                  })
                }
                disabled={disabled}
              />
              {isMixed("max_size_bytes") ? (
                <MixedHint count={count} label={multipleValuesCount} />
              ) : (
                draft.max_size_bytes > 0 && (
                  <p className="text-xs text-muted-foreground">{formatBytes(draft.max_size_bytes)}</p>
                )
              )}
            </div>
            <div className="space-y-2">
              <Label>{t("settings:bucketTabs.maxObjects")}</Label>
              <Input
                type="number"
                value={isMixed("max_objects") ? "" : (draft.max_objects || "")}
                placeholder={isMixed("max_objects") ? multipleValues : undefined}
                onChange={(e) =>
                  patch("max_objects", {
                    ...draft,
                    max_objects: parseInt(e.target.value, 10) || 0,
                  })
                }
                disabled={disabled}
              />
              {isMixed("max_objects") && <MixedHint count={count} label={multipleValuesCount} />}
            </div>
          </CardContent>
        </Card>
      </TabsContent>
    </Tabs>
  );
}
