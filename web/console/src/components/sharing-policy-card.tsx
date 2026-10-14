import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { api, type SharingPolicy } from "@/lib/api";
import { DurationFields } from "@/components/duration-fields";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

type Scope = "global" | "tenant" | "bucket";

function formatPolicyTTL(seconds: number, t: (key: string, opts?: { count: number }) => string): string {
  if (!seconds || seconds <= 0) return t("sharing.unlimited");
  if (seconds % 86400 === 0) return t("sharing.ttlDays", { count: seconds / 86400 });
  if (seconds % 3600 === 0) return t("sharing.ttlHours", { count: seconds / 3600 });
  if (seconds % 60 === 0) return t("sharing.ttlMinutes", { count: seconds / 60 });
  return t("sharing.ttlSeconds", { count: seconds });
}

export function SharingPolicyCard({
  scope,
  scopeId = "",
  canEdit,
}: {
  scope: Scope;
  scopeId?: string;
  canEdit: boolean;
}) {
  const { t } = useTranslation("settings");
  const queryClient = useQueryClient();
  const queryKey = ["sharing", scope, scopeId];
  const sharing = useQuery({
    queryKey,
    queryFn: () => {
      if (scope === "global") return api.getGlobalSharing();
      if (scope === "tenant") return api.getTenantSharing(scopeId);
      return api.getBucketSharing(scopeId);
    },
  });
  const policy = sharing.data?.policy;
  const effective = sharing.data?.effective;
  const locked = new Set(sharing.data?.locked ?? []);

  const [draft, setDraft] = useState<SharingPolicy | null>(null);
  const [confirmOff, setConfirmOff] = useState(false);
  const current = draft ?? policy ?? {};

  const save = useMutation({
    mutationFn: (body: SharingPolicy) => {
      if (scope === "global") return api.putGlobalSharing(body);
      if (scope === "tenant") return api.putTenantSharing(scopeId, body);
      return api.putBucketSharing(scopeId, body);
    },
    onSuccess: () => {
      setDraft(null);
      queryClient.invalidateQueries({ queryKey });
      toast.success(t("sharing.saved"));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const revokeAll = useMutation({
    mutationFn: () => api.revokeAllShares(scopeId),
    onSuccess: (res) => toast.success(t("sharing.revoked", { count: res.revoked })),
    onError: (err: Error) => toast.error(err.message),
  });

  const setBool = (key: keyof SharingPolicy, value: boolean) => {
    if (key === "share_links_allowed" && value === false && current.share_links_allowed !== false) {
      setConfirmOff(true);
      return;
    }
    setDraft({ ...current, [key]: value });
  };

  const by = (field: string | undefined) =>
    field ? t("sharing.limitedBy", { scope: t(`sharing.scope.${field}`, field) }) : "";

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("sharing.title")}</CardTitle>
        <CardDescription>{t("sharing.description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {sharing.isLoading && <p className="text-sm text-muted-foreground">{t("sharing.loading")}</p>}
        {sharing.isError && <p className="text-sm text-destructive">{t("sharing.loadError")}</p>}
        {effective && (
          <p className="text-sm text-muted-foreground">
            {t("sharing.effective", {
              publicAccess: effective.public_access_blocked ? t("sharing.publicBlocked") : t("sharing.publicAllowed"),
              links: effective.share_links_allowed ? t("sharing.linksOn") : t("sharing.linksOff"),
              ttl: formatPolicyTTL(effective.max_share_ttl_sec, t),
            })}
          </p>
        )}
        <Toggle
          label={t("sharing.publicAccess")}
          hint={t("sharing.publicAccessHint")}
          extra={by(effective?.public_access_blocked_by)}
          checked={!!current.public_access_blocked}
          disabled={!canEdit || locked.has("public_access_blocked")}
          onChange={(v) => setBool("public_access_blocked", v)}
        />
        <Toggle
          label={t("sharing.shareLinks")}
          hint={t("sharing.shareLinksHint")}
          extra={by(effective?.share_links_blocked_by)}
          checked={current.share_links_allowed !== false}
          disabled={!canEdit || locked.has("share_links_allowed")}
          onChange={(v) => setBool("share_links_allowed", v)}
        />
        <Toggle
          label={t("sharing.presign")}
          hint={t("sharing.presignHint")}
          extra={by(effective?.presign_blocked_by)}
          checked={current.presign_allowed !== false}
          disabled={!canEdit || locked.has("presign_allowed")}
          onChange={(v) => setBool("presign_allowed", v)}
        />
        <Toggle
          label={t("sharing.passwordRequired")}
          hint={t("sharing.passwordHint")}
          extra={by(effective?.password_required_by)}
          checked={!!current.share_password_required}
          disabled={!canEdit || locked.has("share_password_required")}
          onChange={(v) => setBool("share_password_required", v)}
        />
        <div className="grid gap-3 sm:grid-cols-2">
          <DurationFields
            idPrefix="share-ttl"
            label={t("sharing.maxShareTtl")}
            hint={t("sharing.ttlHint")}
            seconds={current.max_share_ttl_sec}
            disabled={!canEdit}
            onChange={(seconds) => setDraft({ ...current, max_share_ttl_sec: seconds })}
          />
          <DurationFields
            idPrefix="presign-ttl"
            label={t("sharing.maxPresignTtl")}
            hint={t("sharing.presignTtlHint")}
            seconds={current.max_presign_ttl_sec}
            disabled={!canEdit}
            onChange={(seconds) => setDraft({ ...current, max_presign_ttl_sec: seconds })}
          />
        </div>
        {canEdit && (
          <div className="flex flex-wrap gap-2">
            <Button size="sm" disabled={!draft || save.isPending} onClick={() => draft && save.mutate(draft)}>
              {t("sharing.save")}
            </Button>
            {scope === "bucket" && (
              <Button size="sm" variant="outline" disabled={revokeAll.isPending} onClick={() => revokeAll.mutate()}>
                {t("sharing.revokeAll")}
              </Button>
            )}
          </div>
        )}
      </CardContent>
      <Dialog open={confirmOff} onOpenChange={setConfirmOff}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("sharing.disableTitle")}</DialogTitle>
            <DialogDescription>{t("sharing.disableBody")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmOff(false)}>
              {t("sharing.cancel")}
            </Button>
            <Button
              onClick={() => {
                setDraft({ ...current, share_links_allowed: false });
                setConfirmOff(false);
              }}
            >
              {t("sharing.disableConfirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}

function Toggle({
  label,
  hint,
  extra,
  checked,
  disabled,
  onChange,
}: {
  label: string;
  hint: string;
  extra?: string;
  checked: boolean;
  disabled: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <label className="flex items-start gap-3 text-sm">
      <input
        type="checkbox"
        className="mt-1"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
      />
      <span>
        <span className="font-medium">{label}</span>
        <span className="block text-xs text-muted-foreground">{hint}</span>
        {extra ? <span className="block text-xs text-muted-foreground">{extra}</span> : null}
      </span>
    </label>
  );
}
