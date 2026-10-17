import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link2 } from "lucide-react";
import { Button } from "@datasafe/ui/ui/button";
import { Input } from "@datasafe/ui/ui/input";
import { Label } from "@datasafe/ui/ui/label";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@datasafe/ui/ui/dialog";
import { browse, type BrowseResult } from "../desktop";

const PRESETS = [
  { id: "1h", sec: 3600 },
  { id: "24h", sec: 86400 },
  { id: "7d", sec: 604800 },
] as const;

type ShareRow = { id?: string; expires_at?: string; has_password?: boolean };

type Policy = {
  links: boolean;
  presign: boolean;
  password: boolean;
  maxShare: number;
  maxPresign: number;
};

function num(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function problem(result: BrowseResult, t: (key: string) => string, lang: string): string {
  if (result.status < 400 && !result.step_up) {
    return "";
  }
  const body = result.body ?? {};
  const legacy = typeof body.error === "string" ? body.error : "";
  if (legacy === "sharing_disabled" || result.code === "sharing_disabled") {
    return t("shareDisabled");
  }
  if (legacy === "presign_disabled" || result.code === "presign_disabled") {
    return t("presignDisabled");
  }
  if (legacy === "password_required" || result.code === "password_required") {
    return t("passwordRequired");
  }
  if (legacy === "max_share_ttl" || legacy === "max_presign_ttl" || result.code === "max_share_ttl" || result.code === "max_presign_ttl") {
    return t("ttlOver");
  }
  if (lang.startsWith("ru") && typeof body.message_ru === "string" && body.message_ru) {
    return body.message_ru;
  }
  if (typeof body.message === "string" && body.message) {
    return body.message;
  }
  return result.message || result.code || String(result.status || "");
}

export function ShareDialog(props: {
  profileId: string;
  bucket: string;
  objectKey: string;
  canWrite: boolean;
  features?: Record<string, number>;
  confirmDangerous: (method: string, params: Record<string, unknown>) => Promise<BrowseResult>;
  onError: (message: string) => void;
}) {
  const { t, i18n } = useTranslation(["shell", "bucketDetail", "common"]);
  const hasPolicy = props.features?.["sharing.policy"] === 1;
  const canRevokeAll = props.features?.["sharing.revoke_all"] === 1;
  const [open, setOpen] = useState(false);
  const [mode, setMode] = useState<"link" | "presign">("link");
  const [ttl, setTtl] = useState(3600);
  const [password, setPassword] = useState("");
  const [maxDownloads, setMaxDownloads] = useState("0");
  const [policy, setPolicy] = useState<Policy | null>(null);
  const [shares, setShares] = useState<ShareRow[]>([]);
  const [url, setUrl] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!open) {
      return;
    }
    let cancel = false;
    setUrl("");
    setNotice("");
    setPassword("");
    async function load() {
      if (hasPolicy) {
        const result = await browse(props.profileId, "share.policy", { bucket: props.bucket });
        if (cancel) {
          return;
        }
        if (result.status === 200) {
          const effective = (result.body?.effective ?? {}) as Record<string, unknown>;
          setPolicy({
            links: effective.share_links_allowed !== false,
            presign: effective.presign_allowed !== false,
            password: effective.share_password_required === true,
            maxShare: num(effective.max_share_ttl_sec),
            maxPresign: num(effective.max_presign_ttl_sec),
          });
        } else {
          setPolicy(null);
        }
      } else {
        setPolicy(null);
      }
      const listed = await browse(props.profileId, "share.list", { bucket: props.bucket, key: props.objectKey });
      if (cancel) {
        return;
      }
      const rows = Array.isArray(listed.body?.shares) ? (listed.body.shares as ShareRow[]) : [];
      setShares(rows.filter((row) => typeof row.id === "string" && row.id !== ""));
    }
    void load().catch((err: unknown) => {
      if (!cancel) {
        props.onError(err instanceof Error ? err.message : String(err));
      }
    });
    return () => {
      cancel = true;
    };
  }, [hasPolicy, open, props.bucket, props.objectKey, props.onError, props.profileId]);

  const active = policy;
  const linksAllowed = !active || active.links;
  const presignAllowed = !active || (active.presign && active.links);
  const passwordRequired = Boolean(active?.password);
  const max = mode === "link" ? active?.maxShare ?? 0 : active?.maxPresign ?? 0;
  const ttlBlocked = max > 0 && (ttl <= 0 || ttl > max);
  const modeBlocked = mode === "link" ? !linksAllowed : !presignAllowed;
  const needsPassword = mode === "link" && passwordRequired && password.trim() === "";
  const reason = mode === "link"
    ? !linksAllowed
      ? t("shareDisabled")
      : needsPassword
        ? t("passwordRequired")
        : ttlBlocked
          ? t("ttlOver")
          : ""
    : !presignAllowed
      ? active && !active.links
        ? t("shareDisabled")
        : t("presignDisabled")
      : ttlBlocked
        ? t("ttlOver")
        : "";

  async function create() {
    if (!props.canWrite || modeBlocked || needsPassword || ttlBlocked || busy) {
      return;
    }
    setBusy(true);
    setNotice("");
    setUrl("");
    const secret = password;
    setPassword("");
    try {
      const result = mode === "link"
        ? await browse(props.profileId, "share.create", {
            bucket: props.bucket,
            key: props.objectKey,
            expires_in_sec: ttl,
            max_downloads: Number(maxDownloads) || 0,
            ...(secret.trim() ? { share_password: secret } : {}),
          })
        : await browse(props.profileId, "presign.create", {
            bucket: props.bucket,
            key: props.objectKey,
            method: "GET",
            expires_seconds: ttl,
          });
      const message = problem(result, t, i18n.language);
      if (message) {
        setNotice(message);
        return;
      }
      const next = typeof result.body?.url === "string" ? result.body.url : "";
      setUrl(next);
      if (mode === "link") {
        const listed = await browse(props.profileId, "share.list", { bucket: props.bucket, key: props.objectKey });
        const rows = Array.isArray(listed.body?.shares) ? (listed.body.shares as ShareRow[]) : [];
        setShares(rows.filter((row) => typeof row.id === "string" && row.id !== ""));
      }
    } catch (err) {
      props.onError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function revoke(id: string) {
    setNotice("");
    const result = await browse(props.profileId, "share.revoke", { bucket: props.bucket, share_id: id });
    const message = problem(result, t, i18n.language);
    if (message) {
      setNotice(message);
      return;
    }
    setShares((rows) => rows.filter((row) => row.id !== id));
    setUrl("");
  }

  async function revokeAll() {
    setNotice("");
    const result = await props.confirmDangerous("share.revoke_all", { bucket: props.bucket });
    const message = problem(result, t, i18n.language);
    if (message) {
      setNotice(message);
      return;
    }
    setShares([]);
    setUrl("");
  }

  async function copy() {
    if (!url) {
      return;
    }
    try {
      await navigator.clipboard.writeText(url);
      setNotice(t("bucketDetail:toast.copied"));
    } catch {
      setNotice(url);
    }
  }

  return (
    <>
      <Button type="button" size="sm" variant="outline" onClick={() => setOpen(true)}>
        <Link2 className="h-4 w-4" />
        {t("bucketDetail:tabs.share")}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("bucketDetail:share.title", { key: props.objectKey })}</DialogTitle>
            <DialogDescription>{mode === "link" ? t("shareLinkHint") : t("presignHint")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="flex flex-wrap gap-2">
              <Button type="button" size="sm" variant={mode === "link" ? "default" : "outline"} onClick={() => setMode("link")}>
                {t("shareLink")}
              </Button>
              <Button type="button" size="sm" variant={mode === "presign" ? "default" : "outline"} onClick={() => setMode("presign")}>
                {t("presignUrl")}
              </Button>
            </div>
            <div className="space-y-1">
              <Label>{t("bucketDetail:share.expiration")}</Label>
              <div className="flex flex-wrap gap-2">
                {PRESETS.map((item) => {
                  const over = max > 0 && item.sec > max;
                  return (
                    <Button key={item.id} type="button" size="sm" variant={ttl === item.sec ? "default" : "outline"} disabled={over} onClick={() => setTtl(item.sec)}>
                      {t(`bucketDetail:presign.${item.id}`)}
                    </Button>
                  );
                })}
              </div>
            </div>
            {mode === "link" && hasPolicy ? (
              <div className="space-y-1">
                <Label htmlFor="share-password">{passwordRequired ? t("passwordRequired") : t("bucketDetail:share.password")}</Label>
                <Input id="share-password" type="password" autoComplete="off" value={password} onChange={(event) => setPassword(event.target.value)} />
              </div>
            ) : null}
            {mode === "link" ? (
              <div className="space-y-1">
                <Label htmlFor="share-downloads">{t("bucketDetail:share.maxDownloads")}</Label>
                <Input id="share-downloads" inputMode="numeric" value={maxDownloads} onChange={(event) => setMaxDownloads(event.target.value)} />
              </div>
            ) : null}
            {reason ? <p className="text-sm text-muted-foreground">{reason}</p> : null}
            {notice ? <p className="text-sm text-muted-foreground">{notice}</p> : null}
            {url ? (
              <div className="space-y-1">
                <Label htmlFor="share-url">{t("bucketDetail:share.url")}</Label>
                <div className="flex gap-2">
                  <Input id="share-url" readOnly value={url} />
                  <Button type="button" variant="outline" onClick={() => void copy()}>
                    {t("bucketDetail:toast.copied")}
                  </Button>
                </div>
              </div>
            ) : null}
            {mode === "link" && shares.length > 0 ? (
              <ul className="space-y-1">
                {shares.map((row) => (
                  <li key={row.id} className="flex items-center justify-between gap-2">
                    <span className="truncate text-xs text-muted-foreground">{row.expires_at || row.id}</span>
                    <Button type="button" size="sm" variant="outline" onClick={() => void revoke(row.id || "")}>
                      {t("revokeOne")}
                    </Button>
                  </li>
                ))}
              </ul>
            ) : null}
          </div>
          <DialogFooter className="gap-2 sm:justify-between">
            {canRevokeAll ? (
              <Button type="button" variant="destructive" onClick={() => void revokeAll()}>
                {t("revokeAll")}
              </Button>
            ) : (
              <span />
            )}
            <Button type="button" disabled={!props.canWrite || busy || modeBlocked || needsPassword || ttlBlocked} onClick={() => void create()}>
              {mode === "link" ? t("bucketDetail:share.createLink") : t("presignCreate")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
