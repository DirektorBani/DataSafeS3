import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Info } from "lucide-react";
import { Button } from "@datasafe/ui/ui/button";
import { Input } from "@datasafe/ui/ui/input";
import { Textarea } from "@datasafe/ui/ui/textarea";
import { Card, CardContent, CardHeader, CardTitle } from "@datasafe/ui/ui/card";
import { browse, type BrowseResult } from "../desktop";
import { ShareDialog } from "./share-dialog";

type MapValue = Record<string, string>;

export type ListedObject = {
  key: string;
  size?: number;
  etag?: string;
  content_type?: string;
  last_modified?: string;
  version_id?: string;
  legal_hold?: boolean;
  retention_until?: string;
  retention_mode?: string;
  encryption?: { sse_algorithm?: string; plaintext?: boolean };
};

function asMap(value: unknown): MapValue {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return {};
  }
  const out: MapValue = {};
  for (const [key, item] of Object.entries(value as Record<string, unknown>)) {
    if (key === "wrapped_dek" || typeof item !== "string") {
      continue;
    }
    out[key] = item;
  }
  return out;
}

function lines(value: MapValue): string {
  return Object.entries(value)
    .map(([key, item]) => `${key}=${item}`)
    .join("\n");
}

function parseLines(text: string): MapValue {
  const out: MapValue = {};
  for (const line of text.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed) {
      continue;
    }
    const cut = trimmed.indexOf("=");
    if (cut <= 0) {
      continue;
    }
    out[trimmed.slice(0, cut).trim()] = trimmed.slice(cut + 1).trim();
  }
  return out;
}

function failed(result: BrowseResult): string {
  if (result.step_up) {
    return result.message || "step_up";
  }
  if (result.status >= 400) {
    return result.message || result.code || String(result.status);
  }
  return "";
}

export function ObjectInfo(props: {
  profileId: string;
  bucket: string;
  object: ListedObject;
  canWrite: boolean;
  offline?: boolean;
  features?: Record<string, number>;
  confirmDangerous: (method: string, params: Record<string, unknown>) => Promise<BrowseResult>;
  onChanged: () => void;
  onError: (message: string) => void;
}) {
  const { t } = useTranslation();
  const [meta, setMeta] = useState("");
  const [tags, setTags] = useState("");
  const [contentType, setContentType] = useState(props.object.content_type || "");
  const [retention, setRetention] = useState("");
  const [versions, setVersions] = useState<ListedObject[]>([]);

  useEffect(() => {
    let cancel = false;
    const key = props.object.key;
    async function load() {
      const [metaResult, tagResult, holdResult, versionResult] = await Promise.all([
        browse(props.profileId, "object.meta", { bucket: props.bucket, key }),
        browse(props.profileId, "object.tags", { bucket: props.bucket, key }),
        browse(props.profileId, "object.retention", { bucket: props.bucket, key }),
        browse(props.profileId, "object.versions", { bucket: props.bucket, prefix: key }),
      ]);
      if (cancel) {
        return;
      }
      const object = (metaResult.body?.object || {}) as ListedObject & { metadata?: MapValue; retention_mode?: string };
      setMeta(lines(asMap(object.metadata)));
      setContentType(object.content_type || props.object.content_type || "");
      setTags(lines(asMap(tagResult.body?.tags)));
      const until = holdResult.body?.retain_until;
      const hold = holdResult.body?.legal_hold ? "legal hold" : "";
      let mode = object.retention_mode || props.object.retention_mode || "";
      if (!mode && (typeof until === "string" && until || holdResult.body?.legal_hold)) {
        const settings = await browse(props.profileId, "bucket.settings", { bucket: props.bucket });
        if (typeof settings.body?.retention_mode === "string") {
          mode = settings.body.retention_mode;
        }
      }
      if (mode) {
        mode = mode.toUpperCase();
      }
      setRetention([mode, hold, typeof until === "string" ? until : ""].filter(Boolean).join(" · "));
      const rows = Array.isArray(versionResult.body?.versions) ? (versionResult.body?.versions as ListedObject[]) : [];
      setVersions(rows);
    }
    void load();
    return () => {
      cancel = true;
    };
  }, [props.profileId, props.bucket, props.object.key, props.object.content_type]);

  const sealed = props.object.encryption && props.object.encryption.plaintext !== true && props.object.encryption.sse_algorithm;

  async function save(method: "object.meta.put" | "object.tags.put") {
    const result = await browse(props.profileId, method, {
      bucket: props.bucket,
      key: props.object.key,
      metadata: parseLines(meta),
      content_type: contentType,
      tags: parseLines(tags),
    });
    const message = failed(result);
    if (message) {
      props.onError(message);
      return;
    }
    props.onChanged();
  }

  async function restore(versionId: string) {
    const result = await browse(props.profileId, "object.restore", {
      bucket: props.bucket,
      key: props.object.key,
      version_id: versionId,
    });
    const message = failed(result);
    if (message) {
      props.onError(message);
      return;
    }
    props.onChanged();
  }

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="flex items-center gap-2 text-sm">
          <Info className="h-4 w-4" /> {t("bucketDetail:metadata.title")}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-2 text-sm">
        <ShareDialog
          profileId={props.profileId}
          bucket={props.bucket}
          objectKey={props.object.key}
          canWrite={props.canWrite}
          offline={props.offline}
          features={props.features}
          confirmDangerous={props.confirmDangerous}
          onError={props.onError}
        />
        <div>
          <span className="text-muted-foreground">{t("bucketDetail:metadata.key")}</span>
          <p className="break-all font-mono">{props.object.key}</p>
        </div>
        <div>
          <span className="text-muted-foreground">{t("bucketDetail:metadata.size")}</span>
          <p>{props.object.size ?? 0}</p>
        </div>
        <div>
          <span className="text-muted-foreground">{t("bucketDetail:metadata.contentType")}</span>
          <Input className="mt-1 h-8 text-xs" value={contentType} readOnly={!props.canWrite} onChange={(event) => setContentType(event.target.value)} />
        </div>
        <div>
          <span className="text-muted-foreground">{t("bucketDetail:metadata.etag")}</span>
          <p className="break-all font-mono text-xs">{props.object.etag || "—"}</p>
        </div>
        <div>
          <span className="text-muted-foreground">{t("bucketDetail:metadata.modified")}</span>
          <p>{props.object.last_modified || "—"}</p>
        </div>
        {sealed ? (
          <p>
            {t("encryption")}: {props.object.encryption?.sse_algorithm}
          </p>
        ) : null}
        <p>
          {t("retention")}: {retention || props.object.retention_until || (props.object.legal_hold ? "legal hold" : "—")}
        </p>
        <div className="space-y-1">
          <span className="text-muted-foreground">{t("metadata")}</span>
          <Textarea rows={3} className="font-mono text-xs" value={meta} readOnly={!props.canWrite} onChange={(event) => setMeta(event.target.value)} />
        </div>
        {props.canWrite ? (
          <Button type="button" size="sm" variant="outline" onClick={() => void save("object.meta.put")}>
            {t("bucketDetail:metadata.save")}
          </Button>
        ) : null}
        <div className="space-y-1">
          <span className="text-muted-foreground">{t("tags")}</span>
          <Textarea rows={3} className="font-mono text-xs" value={tags} readOnly={!props.canWrite} onChange={(event) => setTags(event.target.value)} />
        </div>
        {props.canWrite ? (
          <Button type="button" size="sm" variant="outline" onClick={() => void save("object.tags.put")}>
            {t("saveTags")}
          </Button>
        ) : null}
        <div>
          <h4 className="font-medium">{t("bucketDetail:tabs.versions")}</h4>
          <ul className="space-y-1">
            {versions.map((version) => (
              <li key={version.version_id || version.last_modified} className="flex items-center justify-between gap-2">
                <span className="truncate font-mono text-xs">{version.version_id || version.last_modified}</span>
                {props.canWrite && version.version_id ? (
                  <Button type="button" size="sm" variant="outline" onClick={() => void restore(version.version_id || "")}>
                    {t("restore")}
                  </Button>
                ) : null}
              </li>
            ))}
          </ul>
        </div>
      </CardContent>
    </Card>
  );
}
