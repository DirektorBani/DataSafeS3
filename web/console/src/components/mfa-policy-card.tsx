import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { api, type MFASettings } from "@/lib/api";
import { formatDate } from "@/lib/utils";
import { Button } from "@datasafe/ui/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@datasafe/ui/ui/card";
import { Input } from "@datasafe/ui/ui/input";
import { Label } from "@datasafe/ui/ui/label";

const ACTION_IDS = [
  "crypto_shred",
  "key_rotate",
  "trash_purge",
  "sharing_policy_change",
  "grants_change",
  "encryption_disable",
  "object_lock_config",
  "compliance_retention",
  "bucket_delete",
  "mfa_reset",
  "revoke_all_shares",
  "access_key_create",
] as const;

export function MfaPolicyCard() {
  const { t, i18n } = useTranslation(["settings", "common"]);
  const queryClient = useQueryClient();
  const policy = useQuery({
    queryKey: ["mfa-policy"],
    queryFn: () => api.getMFAPolicy(),
  });
  const [draft, setDraft] = useState<MFASettings | null>(null);

  useEffect(() => {
    if (policy.data) setDraft(policy.data.policy);
  }, [policy.data]);

  const save = useMutation({
    mutationFn: (next: MFASettings) => api.putMFAPolicy(next),
    onSuccess: (data) => {
      setDraft(data.policy);
      queryClient.invalidateQueries({ queryKey: ["mfa-policy"] });
      queryClient.invalidateQueries({ queryKey: ["system-config"] });
      queryClient.invalidateQueries({ queryKey: ["me"] });
      toast.success(t("settings:mfaPolicy.saved"));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  if (policy.isLoading || !draft) {
    return <p className="text-sm text-muted-foreground">{t("settings:mfaPolicy.loading")}</p>;
  }
  if (policy.isError) {
    return <p className="text-sm text-destructive">{t("settings:mfaPolicy.loadError")}</p>;
  }

  const globalOn = Boolean(draft.apply_to_global_admins || draft.require_admin_mfa);
  const actions = policy.data?.dangerous_actions?.length ? policy.data.dangerous_actions : [...ACTION_IDS];
  const missing = policy.data?.users_without_mfa ?? [];

  function saveDraft() {
    const grace = draft?.grace_period_days ?? 7;
    save.mutate({
      ...draft,
      grace_period_days: grace,
      require_admin_mfa: globalOn,
      apply_to_global_admins: globalOn,
    });
  }

  return (
    <Card data-testid="mfa-policy-card">
      <CardHeader>
        <CardTitle className="text-base">{t("settings:mfaPolicy.title")}</CardTitle>
        <CardDescription>{t("settings:mfaPolicy.description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <Toggle
          label={t("settings:mfaPolicy.requireBucketAdmin")}
          hint={t("settings:mfaPolicy.requireBucketAdminHint")}
          checked={Boolean(draft.require_bucket_admin_mfa)}
          onChange={(checked) => setDraft({ ...draft, require_bucket_admin_mfa: checked })}
        />
        <div className="space-y-1">
          <Label htmlFor="mfa-grace-days">{t("settings:mfaPolicy.graceDays")}</Label>
          <Input
            id="mfa-grace-days"
            type="number"
            min={0}
            max={90}
            value={draft.grace_period_days ?? 7}
            onChange={(e) => {
              const n = Number(e.target.value);
              setDraft({ ...draft, grace_period_days: Number.isFinite(n) ? n : 0 });
            }}
          />
          <p className="text-xs text-muted-foreground">{t("settings:mfaPolicy.graceHint")}</p>
        </div>
        <Toggle
          label={t("settings:mfaPolicy.tenantAdmins")}
          hint={t("settings:mfaPolicy.tenantAdminsHint")}
          checked={Boolean(draft.apply_to_tenant_admins)}
          onChange={(checked) => setDraft({ ...draft, apply_to_tenant_admins: checked })}
        />
        <Toggle
          label={t("settings:mfaPolicy.globalAdmins")}
          hint={t("settings:mfaPolicy.globalAdminsHint")}
          checked={globalOn}
          onChange={(checked) =>
            setDraft({
              ...draft,
              apply_to_global_admins: checked,
              require_admin_mfa: checked,
            })
          }
        />
        <div className="space-y-1">
          <p className="text-sm font-medium">{t("settings:mfaPolicy.actionsTitle")}</p>
          <p className="text-xs text-muted-foreground">{t("settings:mfaPolicy.actionsHint")}</p>
          <ul className="list-disc space-y-0.5 pl-5 text-sm text-muted-foreground">
            {actions.map((id) => (
              <li key={id}>{t(`settings:mfaPolicy.actions.${id}`, { defaultValue: id })}</li>
            ))}
          </ul>
        </div>
        <div className="space-y-1">
          <p className="text-sm font-medium">{t("settings:mfaPolicy.missingTitle")}</p>
          {missing.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("settings:mfaPolicy.missingEmpty")}</p>
          ) : (
            <ul className="space-y-1 text-sm">
              {missing.map((user) => (
                <li key={user.user_id}>
                  {user.username}
                  {" — "}
                  {user.in_grace
                    ? t("settings:mfaPolicy.inGrace", {
                        until: user.grace_until ? formatDate(user.grace_until, i18n.language) : "",
                      })
                    : t("settings:mfaPolicy.graceOver")}
                </li>
              ))}
            </ul>
          )}
        </div>
        <Button size="sm" onClick={saveDraft} disabled={save.isPending}>
          {t("common:save")}
        </Button>
      </CardContent>
    </Card>
  );
}

function Toggle({
  label,
  hint,
  checked,
  onChange,
}: {
  label: string;
  hint: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <label className="flex items-start gap-2 text-sm">
      <input
        type="checkbox"
        className="mt-0.5 h-4 w-4"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
      />
      <span>
        <span className="font-medium">{label}</span>
        <span className="mt-0.5 block text-xs text-muted-foreground">{hint}</span>
      </span>
    </label>
  );
}
