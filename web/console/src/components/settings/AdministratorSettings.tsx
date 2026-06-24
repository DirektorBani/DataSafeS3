import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { api, type SystemConfig } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { type TrashRetentionUnit } from "./trash-retention";

export type AdministratorSettingsProps = {
  draft: SystemConfig;
  onDraftChange: (draft: SystemConfig) => void;
  trashRetentionValue: string;
  trashRetentionUnit: TrashRetentionUnit;
  onTrashRetentionValueChange: (value: string) => void;
  onTrashRetentionUnitChange: (unit: TrashRetentionUnit) => void;
  trashRetentionDays: number;
};

export function AdministratorSettings({
  draft,
  onDraftChange,
  trashRetentionValue,
  trashRetentionUnit,
  onTrashRetentionValueChange,
  onTrashRetentionUnitChange,
  trashRetentionDays,
}: AdministratorSettingsProps) {
  const { t } = useTranslation(["adminSettings", "common"]);
  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <div>
            <CardTitle className="text-base">{t("adminSettings:trash.title")}</CardTitle>
            <CardDescription>{t("adminSettings:trash.description")}</CardDescription>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              className="h-4 w-4"
              checked={draft.soft_delete_enabled}
              onChange={(e) => onDraftChange({ ...draft, soft_delete_enabled: e.target.checked })}
            />
            {t("adminSettings:trash.enable")}
          </label>
          <div className="space-y-2 max-w-xs">
            <Label>{t("adminSettings:trash.autoPurge")}</Label>
            <div className="flex gap-2">
              <Input
                type="number"
                min={1}
                max={trashRetentionUnit === "months" ? 120 : 3650}
                value={trashRetentionValue}
                onChange={(e) => onTrashRetentionValueChange(e.target.value)}
              />
              <Select
                value={trashRetentionUnit}
                onValueChange={(v) => onTrashRetentionUnitChange(v as TrashRetentionUnit)}
              >
                <SelectTrigger className="w-28"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="days">{t("adminSettings:trash.days")}</SelectItem>
                  <SelectItem value="months">{t("adminSettings:trash.months")}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <p className="text-xs text-muted-foreground">
              {t("adminSettings:trash.daysTotal", { days: trashRetentionDays })}
            </p>
          </div>
        </CardContent>
      </Card>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader><CardTitle className="text-base">{t("adminSettings:ldap.title")}</CardTitle></CardHeader>
          <CardContent className="space-y-3 text-sm">
            <label className="flex items-center gap-2">
              <input
                type="checkbox"
                className="h-4 w-4"
                checked={draft.ldap?.enabled ?? false}
                onChange={(e) => {
                  const enabled = e.target.checked;
                  const ldap = { ...draft.ldap, enabled };
                  if (enabled && (ldap.sync_interval_minutes ?? 0) === 0) {
                    ldap.sync_interval_minutes = 60;
                  }
                  onDraftChange({ ...draft, ldap });
                }}
              />
              {t("adminSettings:ldap.enable")}
            </label>
            <Input
              placeholder={t("adminSettings:ldap.placeholder.url")}
              value={draft.ldap?.url ?? ""}
              onChange={(e) => onDraftChange({ ...draft, ldap: { ...draft.ldap, url: e.target.value } })}
            />
            <p className="text-xs text-muted-foreground">{t("adminSettings:ldap.dockerHint")}</p>
            <Input
              placeholder={t("adminSettings:ldap.placeholder.bindDn")}
              value={draft.ldap?.bind_dn ?? ""}
              onChange={(e) => onDraftChange({ ...draft, ldap: { ...draft.ldap, bind_dn: e.target.value } })}
            />
            <Input
              placeholder={t("adminSettings:ldap.placeholder.bindPassword")}
              type="password"
              value={draft.ldap?.bind_password ?? ""}
              onChange={(e) => onDraftChange({ ...draft, ldap: { ...draft.ldap, bind_password: e.target.value } })}
            />
            <Input
              placeholder={t("adminSettings:ldap.placeholder.baseDn")}
              value={draft.ldap?.base_dn ?? ""}
              onChange={(e) => onDraftChange({ ...draft, ldap: { ...draft.ldap, base_dn: e.target.value } })}
            />
            <Input
              placeholder={t("adminSettings:ldap.placeholder.groupAttr")}
              value={draft.ldap?.group_attr ?? ""}
              onChange={(e) => onDraftChange({ ...draft, ldap: { ...draft.ldap, group_attr: e.target.value } })}
            />
            <p className="text-xs text-muted-foreground">{t("adminSettings:ldap.groupsHint")}</p>
            <div className="space-y-2 max-w-xs">
              <Label>{t("adminSettings:ldap.syncInterval")}</Label>
              <Input
                type="number"
                min={0}
                placeholder="60"
                value={draft.ldap?.sync_interval_minutes ?? ""}
                onChange={(e) =>
                  onDraftChange({
                    ...draft,
                    ldap: { ...draft.ldap, sync_interval_minutes: parseInt(e.target.value, 10) || 0 },
                  })
                }
              />
              <p className="text-xs text-muted-foreground">{t("adminSettings:ldap.syncIntervalHint")}</p>
            </div>
            <div className="flex gap-2">
              <Button
                size="sm"
                variant="outline"
                onClick={async () => {
                  try {
                    const r = await api.testLDAP(draft.ldap ?? {});
                    toast[r.ok ? "success" : "error"](r.message ?? r.error ?? "");
                  } catch (e) {
                    toast.error((e as Error).message);
                  }
                }}
              >
                {t("adminSettings:ldap.test")}
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={async () => {
                  try {
                    const r = await api.syncLDAP();
                    toast.success(t("adminSettings:ldap.synced", { count: r.synced }));
                  } catch (e) {
                    toast.error((e as Error).message);
                  }
                }}
              >
                {t("adminSettings:ldap.sync")}
              </Button>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle className="text-base">{t("adminSettings:oidc.title")}</CardTitle></CardHeader>
          <CardContent className="space-y-3 text-sm">
            <label className="flex items-center gap-2">
              <input
                type="checkbox"
                className="h-4 w-4"
                checked={draft.oidc?.enabled ?? false}
                onChange={(e) => onDraftChange({ ...draft, oidc: { ...draft.oidc, enabled: e.target.checked } })}
              />
              {t("adminSettings:oidc.enable")}
            </label>
            <Input
              placeholder={t("adminSettings:oidc.placeholder.issuer")}
              value={draft.oidc?.issuer ?? ""}
              onChange={(e) => onDraftChange({ ...draft, oidc: { ...draft.oidc, issuer: e.target.value } })}
            />
            <Input
              placeholder={t("adminSettings:oidc.placeholder.internalIssuer")}
              value={draft.oidc?.internal_issuer ?? ""}
              onChange={(e) => onDraftChange({ ...draft, oidc: { ...draft.oidc, internal_issuer: e.target.value } })}
            />
            <p className="text-xs text-muted-foreground">{t("adminSettings:oidc.dockerHint")}</p>
            <Input
              placeholder={t("adminSettings:oidc.placeholder.clientId")}
              value={draft.oidc?.client_id ?? ""}
              onChange={(e) => onDraftChange({ ...draft, oidc: { ...draft.oidc, client_id: e.target.value } })}
            />
            <Input
              placeholder={t("adminSettings:oidc.placeholder.clientSecret")}
              type="password"
              value={draft.oidc?.client_secret ?? ""}
              onChange={(e) => onDraftChange({ ...draft, oidc: { ...draft.oidc, client_secret: e.target.value } })}
            />
            <Input
              placeholder={t("adminSettings:oidc.placeholder.redirectUrl")}
              value={draft.oidc?.redirect_url ?? ""}
              onChange={(e) => onDraftChange({ ...draft, oidc: { ...draft.oidc, redirect_url: e.target.value } })}
            />
            <Input
              placeholder={t("adminSettings:oidc.placeholder.groupsClaim")}
              value={draft.oidc?.groups_claim ?? ""}
              onChange={(e) => onDraftChange({ ...draft, oidc: { ...draft.oidc, groups_claim: e.target.value } })}
            />
            <p className="text-xs text-muted-foreground">{t("adminSettings:oidc.groupsHint")}</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle className="text-base">{t("adminSettings:mfa.title")}</CardTitle></CardHeader>
          <CardContent className="space-y-2">
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                className="h-4 w-4"
                checked={draft.mfa?.require_admin_mfa ?? false}
                onChange={(e) => onDraftChange({ ...draft, mfa: { require_admin_mfa: e.target.checked } })}
              />
              {t("adminSettings:mfa.requireAdmin")}
            </label>
            <p className="text-xs text-muted-foreground">{t("adminSettings:mfa.hint")}</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle className="text-base">{t("adminSettings:cluster.title")}</CardTitle></CardHeader>
          <CardContent className="space-y-3 text-sm">
            <label className="flex items-center gap-2">
              <input
                type="checkbox"
                className="h-4 w-4"
                checked={draft.cluster?.distributed_mode ?? false}
                onChange={(e) =>
                  onDraftChange({ ...draft, cluster: { ...draft.cluster, distributed_mode: e.target.checked } })
                }
              />
              {t("adminSettings:cluster.distributed")}
            </label>
            <label className="flex items-center gap-2">
              <input
                type="checkbox"
                className="h-4 w-4"
                checked={draft.cluster?.erasure_coding_planned ?? true}
                onChange={(e) =>
                  onDraftChange({
                    ...draft,
                    cluster: { ...draft.cluster, erasure_coding_planned: e.target.checked },
                  })
                }
              />
              {t("adminSettings:cluster.erasure")}
            </label>
            <Textarea
              placeholder={t("adminSettings:cluster.diskPaths")}
              rows={3}
              value={(draft.cluster?.disk_paths ?? []).join("\n")}
              onChange={(e) =>
                onDraftChange({
                  ...draft,
                  cluster: { ...draft.cluster, disk_paths: e.target.value.split("\n").filter(Boolean) },
                })
              }
            />
          </CardContent>
        </Card>

        <Card className="lg:col-span-2">
          <CardHeader>
            <div>
              <CardTitle className="text-base">{t("adminSettings:logging.title")}</CardTitle>
              <CardDescription>{t("adminSettings:logging.description")}</CardDescription>
            </div>
          </CardHeader>
          <CardContent className="grid gap-4 md:grid-cols-2">
            {(["syslog", "loki", "elasticsearch", "webhook"] as const).map((name) => {
              const sink = draft.logging?.[name] ?? {};
              return (
                <div key={name} className="space-y-2 rounded border p-3 text-sm">
                  <label className="flex items-center gap-2 font-medium capitalize">
                    <input
                      type="checkbox"
                      className="h-4 w-4"
                      checked={sink.enabled ?? false}
                      onChange={(e) =>
                        onDraftChange({
                          ...draft,
                          logging: { ...draft.logging, [name]: { ...sink, enabled: e.target.checked } },
                        })
                      }
                    />
                    {name}
                  </label>
                  <Input
                    placeholder={t("adminSettings:logging.address")}
                    value={sink.address ?? ""}
                    onChange={(e) =>
                      onDraftChange({
                        ...draft,
                        logging: { ...draft.logging, [name]: { ...sink, address: e.target.value } },
                      })
                    }
                  />
                  {(name === "elasticsearch" || name === "loki") && (
                    <Input
                      placeholder={
                        name === "elasticsearch"
                          ? t("adminSettings:logging.index")
                          : t("adminSettings:logging.bearerToken")
                      }
                      value={name === "elasticsearch" ? (sink.index ?? "") : (sink.token ?? "")}
                      onChange={(e) =>
                        onDraftChange({
                          ...draft,
                          logging: {
                            ...draft.logging,
                            [name]:
                              name === "elasticsearch"
                                ? { ...sink, index: e.target.value }
                                : { ...sink, token: e.target.value },
                          },
                        })
                      }
                    />
                  )}
                  {name === "elasticsearch" && (
                    <>
                      <Input
                        placeholder={t("adminSettings:logging.username")}
                        value={sink.username ?? ""}
                        onChange={(e) =>
                          onDraftChange({
                            ...draft,
                            logging: {
                              ...draft.logging,
                              elasticsearch: { ...sink, username: e.target.value },
                            },
                          })
                        }
                      />
                      <Input
                        type="password"
                        placeholder={t("adminSettings:logging.password")}
                        value={sink.password ?? ""}
                        onChange={(e) =>
                          onDraftChange({
                            ...draft,
                            logging: {
                              ...draft.logging,
                              elasticsearch: { ...sink, password: e.target.value },
                            },
                          })
                        }
                      />
                      <Input
                        placeholder={t("adminSettings:logging.apiKey")}
                        value={sink.token ?? ""}
                        onChange={(e) =>
                          onDraftChange({
                            ...draft,
                            logging: {
                              ...draft.logging,
                              elasticsearch: { ...sink, token: e.target.value },
                            },
                          })
                        }
                      />
                    </>
                  )}
                  <label className="flex items-center gap-2 text-xs text-muted-foreground">
                    <input
                      type="checkbox"
                      className="h-3 w-3"
                      checked={sink.tls ?? false}
                      onChange={(e) =>
                        onDraftChange({
                          ...draft,
                          logging: { ...draft.logging, [name]: { ...sink, tls: e.target.checked } },
                        })
                      }
                    />
                    {t("adminSettings:logging.tlsVerify")}
                  </label>
                </div>
              );
            })}
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t("adminSettings:swagger.title")}</CardTitle>
          <CardDescription>{t("adminSettings:swagger.description")}</CardDescription>
        </CardHeader>
        <CardContent>
          <Button variant="outline" size="sm" asChild>
            <a href="/api/v1/docs" target="_blank" rel="noopener noreferrer">
              {t("adminSettings:swagger.open")}
            </a>
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
