import type { BucketSettings } from "@/lib/api";

export type ApplyMode = "overwrite" | "only_empty";

export const EDITABLE_BUCKET_FIELDS = [
  "description",
  "versioning_enabled",
  "object_lock_enabled",
  "retention_days",
  "storage_class",
  "visibility",
  "max_size_bytes",
  "max_objects",
  "lifecycle_rules",
] as const;

export type EditableBucketField = (typeof EDITABLE_BUCKET_FIELDS)[number];

export type BucketSettingsUpdate = Pick<
  BucketSettings,
  | "description"
  | "versioning_enabled"
  | "object_lock_enabled"
  | "retention_days"
  | "storage_class"
  | "visibility"
  | "max_size_bytes"
  | "max_objects"
  | "lifecycle_rules"
>;

const MIXED_SELECT = "__mixed__";

export { MIXED_SELECT };

function fieldValuesEqual(field: EditableBucketField, a: BucketSettings, b: BucketSettings): boolean {
  if (field === "lifecycle_rules") {
    return JSON.stringify(a.lifecycle_rules ?? []) === JSON.stringify(b.lifecycle_rules ?? []);
  }
  const av = a[field];
  const bv = b[field];
  if (field === "storage_class") {
    return (av || "hot") === (bv || "hot");
  }
  if (field === "retention_days") {
    return (av ?? 30) === (bv ?? 30);
  }
  return av === bv;
}

export function isBucketConfigured(bucket: BucketSettings): boolean {
  return (
    Boolean(bucket.description?.trim()) ||
    bucket.versioning_enabled ||
    bucket.object_lock_enabled ||
    (bucket.storage_class != null && bucket.storage_class !== "" && bucket.storage_class !== "hot") ||
    bucket.visibility !== "private" ||
    bucket.max_size_bytes > 0 ||
    bucket.max_objects > 0 ||
    (bucket.lifecycle_rules?.length ?? 0) > 0
  );
}

export function isFieldEmpty(bucket: BucketSettings, field: EditableBucketField): boolean {
  switch (field) {
    case "description":
      return !bucket.description?.trim();
    case "versioning_enabled":
      return !bucket.versioning_enabled;
    case "object_lock_enabled":
      return !bucket.object_lock_enabled;
    case "retention_days":
      return (bucket.retention_days ?? 30) === 30;
    case "storage_class":
      return !bucket.storage_class || bucket.storage_class === "hot";
    case "visibility":
      return bucket.visibility === "private" || !bucket.visibility;
    case "max_size_bytes":
      return !bucket.max_size_bytes;
    case "max_objects":
      return !bucket.max_objects;
    case "lifecycle_rules":
      return (bucket.lifecycle_rules?.length ?? 0) === 0;
    default:
      return true;
  }
}

export function computeMixedFields(buckets: BucketSettings[]): Set<EditableBucketField> {
  const mixed = new Set<EditableBucketField>();
  if (buckets.length < 2) return mixed;

  for (const field of EDITABLE_BUCKET_FIELDS) {
    for (let i = 1; i < buckets.length; i++) {
      if (!fieldValuesEqual(field, buckets[0], buckets[i])) {
        mixed.add(field);
        break;
      }
    }
  }
  return mixed;
}

export function mergeBucketSettings(buckets: BucketSettings[]): {
  draft: BucketSettings;
  mixed: Set<EditableBucketField>;
} {
  if (buckets.length === 0) {
    throw new Error("At least one bucket required");
  }

  const mixed = computeMixedFields(buckets);
  const draft: BucketSettings = { ...buckets[0] };

  if (buckets.length > 1) {
    draft.name = buckets.map((b) => b.name).join(", ");
    const owners = new Set(buckets.map((b) => b.owner));
    draft.owner = owners.size === 1 ? buckets[0].owner : `${owners.size} owners`;
  }

  return { draft, mixed };
}

export function draftToUpdateBody(draft: BucketSettings): BucketSettingsUpdate {
  return {
    description: draft.description,
    versioning_enabled: draft.versioning_enabled,
    object_lock_enabled: draft.object_lock_enabled,
    retention_days: draft.retention_days,
    storage_class: draft.storage_class,
    visibility: draft.visibility,
    max_size_bytes: draft.max_size_bytes,
    max_objects: draft.max_objects,
    lifecycle_rules: draft.lifecycle_rules,
  };
}

export function buildUpdatePayloadForBucket(
  draft: BucketSettings,
  bucket: BucketSettings,
  applyMode: ApplyMode,
  mixed: Set<EditableBucketField>,
  clearedMixed: Set<EditableBucketField>
): BucketSettingsUpdate {
  const full = draftToUpdateBody(draft);
  if (applyMode === "overwrite") {
    return full;
  }

  const partial: Partial<BucketSettingsUpdate> = {};
  for (const field of EDITABLE_BUCKET_FIELDS) {
    const userSetValue = !mixed.has(field) || clearedMixed.has(field);
    if (userSetValue && (isFieldEmpty(bucket, field) || clearedMixed.has(field))) {
      (partial as Record<string, unknown>)[field] = full[field];
    }
  }
  return partial as BucketSettingsUpdate;
}

export type BatchUpdateResult = {
  ok: number;
  failed: { name: string; error: string }[];
};
