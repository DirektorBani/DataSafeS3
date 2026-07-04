import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Copy, Link2, Plus, ShieldOff, Trash2 } from "lucide-react";
import { api, type SiteReplicationRule, type TrustedCluster } from "@/lib/api";
import { PageHeader } from "@/components/page-header";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

function formatTime(iso?: string) {
  if (!iso) return "—";
  return new Date(iso).toLocaleString();
}

export function ClusterPage() {
  const { t } = useTranslation("cluster");
  const qc = useQueryClient();
  const [pairToken, setPairToken] = useState<string | null>(null);
  const [pairExpires, setPairExpires] = useState<string | null>(null);
  const [joinForm, setJoinForm] = useState({ initiator_url: "", token: "", name: "" });
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [replForm, setReplForm] = useState({ source_bucket: "", dest_bucket: "", direction: "one-way" });

  const status = useQuery({ queryKey: ["cluster-status"], queryFn: () => api.clusterStatus() });
  const trusted = useQuery({ queryKey: ["trusted-clusters"], queryFn: () => api.listTrustedClusters() });
  const detail = useQuery({
    queryKey: ["trusted-cluster", selectedId],
    queryFn: () => api.getTrustedCluster(selectedId!),
    enabled: !!selectedId,
  });
  const replRules = useQuery({
    queryKey: ["cluster-repl-rules", selectedId],
    queryFn: () => api.listClusterReplicationRules(selectedId!),
    enabled: !!selectedId,
  });

  const createCode = useMutation({
    mutationFn: () => api.createClusterPairingCode(),
    onSuccess: (data) => {
      setPairToken(data.token);
      setPairExpires(data.expires_at);
      toast.success(t("pairing.tokenCreated"));
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const joinCluster = useMutation({
    mutationFn: () => api.joinTrustedCluster(joinForm),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["trusted-clusters"] });
      setJoinForm({ initiator_url: "", token: "", name: "" });
      toast.success(t("pairing.joinSuccess"));
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const revoke = useMutation({
    mutationFn: (id: string) => api.revokeTrustedCluster(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["trusted-clusters"] });
      setSelectedId(null);
      toast.success(t("revoke.success"));
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const addReplRule = useMutation({
    mutationFn: () =>
      api.createClusterReplicationRule(selectedId!, {
        source_bucket: replForm.source_bucket,
        dest_bucket: replForm.dest_bucket,
        direction: replForm.direction,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["cluster-repl-rules", selectedId] });
      setReplForm({ source_bucket: "", dest_bucket: "", direction: "one-way" });
      toast.success(t("replication.ruleCreated"));
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const deleteReplRule = useMutation({
    mutationFn: (ruleId: string) => api.deleteClusterReplicationRule(selectedId!, ruleId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["cluster-repl-rules", selectedId] });
      toast.success(t("replication.ruleDeleted"));
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const remoteClusters = (trusted.data?.clusters ?? []).filter((c) => !c.is_local);
  const selected = detail.data?.cluster;

  return (
    <div>
      <PageHeader title={t("title")} description={t("description")} />

      {status.data && (
        <>
          <Card className="mb-6">
            <CardHeader>
              <CardTitle className="text-base">{t("config.title")}</CardTitle>
              <CardDescription>{t("config.description")}</CardDescription>
            </CardHeader>
            <CardContent className="text-sm space-y-2">
              <p>{t("distributedMode")} {status.data.distributed_mode ? t("distributed.enabled") : t("distributed.single")}</p>
              <p>{t("erasureCoding")} {status.data.erasure_coding_planned ? t("erasure.planned") : t("erasure.notConfigured")}</p>
              {status.data.object_backend && (
                <p>{t("objectBackend")}: {status.data.object_backend}{status.data.erasure_degraded ? ` (${t("erasure.degraded")})` : ""}</p>
              )}
              {status.data.ha_enabled && (
                <p>{t("haLeader")}: {status.data.is_leader ? t("leader.yes") : t("leader.no")} ({status.data.node_id})</p>
              )}
              {status.data.disk_paths?.length > 0 && (
                <p>{t("diskPaths")} {status.data.disk_paths.join(", ")}</p>
              )}
            </CardContent>
          </Card>
          <Card className="mb-6">
            <CardHeader><CardTitle className="text-base">{t("nodes.title")}</CardTitle></CardHeader>
            <CardContent className="space-y-2">
              {status.data.nodes.map((n) => (
                <div key={n.id} className="flex items-center justify-between rounded border p-3">
                  <div>
                    <p className="font-medium">{n.id}</p>
                    <p className="text-xs text-muted-foreground">{n.address} — {n.role}</p>
                  </div>
                  <Badge variant={n.status === "healthy" ? "default" : "secondary"}>{n.status ?? t("node.unknown")}</Badge>
                </div>
              ))}
            </CardContent>
          </Card>
        </>
      )}

      <Card className="mb-6">
        <CardHeader>
          <CardTitle className="text-base">{t("trusted.title")}</CardTitle>
          <CardDescription>{t("trusted.description")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {(trusted.data?.clusters ?? []).map((c) => (
            <ClusterRow key={c.id} cluster={c} selected={selectedId === c.id} onSelect={() => setSelectedId(c.id)} t={t} />
          ))}
        </CardContent>
      </Card>

      <div className="grid gap-6 md:grid-cols-2 mb-6">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("pairing.addTitle")}</CardTitle>
            <CardDescription>{t("pairing.addDescription")}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <Button onClick={() => createCode.mutate()} disabled={createCode.isPending}>
              <Plus className="h-4 w-4 mr-2" /> {t("pairing.generate")}
            </Button>
            {pairToken && (
              <div className="rounded border p-3 text-sm space-y-2">
                <p className="font-mono break-all">{pairToken}</p>
                <p className="text-muted-foreground text-xs">{t("pairing.expires")}: {formatTime(pairExpires ?? undefined)}</p>
                <Button variant="outline" size="sm" onClick={() => { void navigator.clipboard.writeText(pairToken); toast.success(t("pairing.copied")); }}>
                  <Copy className="h-3 w-3 mr-1" /> {t("pairing.copy")}
                </Button>
              </div>
            )}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("pairing.joinTitle")}</CardTitle>
            <CardDescription>{t("pairing.joinDescription")}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div><Label>{t("pairing.initiatorUrl")}</Label><Input value={joinForm.initiator_url} onChange={(e) => setJoinForm({ ...joinForm, initiator_url: e.target.value })} placeholder="https://site-a:9000" /></div>
            <div><Label>{t("pairing.token")}</Label><Input value={joinForm.token} onChange={(e) => setJoinForm({ ...joinForm, token: e.target.value })} /></div>
            <div><Label>{t("pairing.name")}</Label><Input value={joinForm.name} onChange={(e) => setJoinForm({ ...joinForm, name: e.target.value })} /></div>
            <Button onClick={() => joinCluster.mutate()} disabled={joinCluster.isPending}>
              <Link2 className="h-4 w-4 mr-2" /> {t("pairing.join")}
            </Button>
          </CardContent>
        </Card>
      </div>

      {selected && !selected.is_local && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{selected.name}</CardTitle>
            <CardDescription>{selected.endpoint}</CardDescription>
          </CardHeader>
          <CardContent className="text-sm space-y-2">
            <p>{t("trusted.status")}: <Badge>{selected.status}</Badge></p>
            <p>{t("trusted.certExpires")}: {formatTime(selected.cert_expires_at)}</p>
            <p>{t("trusted.nextRotation")}: {formatTime(selected.next_rotation_at)}</p>
            <p>{t("trusted.safetyNumber")}: <code>{selected.safety_number ?? "—"}</code></p>
            {detail.data?.certificates?.length ? (
              <div className="pt-2">
                <p className="font-medium mb-1">{t("trusted.certTimeline")}</p>
                {detail.data.certificates.map((c) => (
                  <p key={c.serial} className="text-xs text-muted-foreground">
                    {c.serial.slice(0, 12)}… — {formatTime(c.not_before)} → {formatTime(c.not_after)}
                    {c.revoked_at ? ` (${t("revoke.revoked")})` : ""}
                  </p>
                ))}
              </div>
            ) : null}
            <ReplicationRulesSection
              t={t}
              rules={replRules.data?.rules ?? []}
              status={replRules.data?.status}
              form={replForm}
              onFormChange={setReplForm}
              onAdd={() => addReplRule.mutate()}
              onDelete={(id) => deleteReplRule.mutate(id)}
              adding={addReplRule.isPending}
            />
            <Button variant="destructive" size="sm" className="mt-2" onClick={() => revoke.mutate(selected.id)} disabled={revoke.isPending}>
              <ShieldOff className="h-3 w-3 mr-1" /> {t("revoke.action")}
            </Button>
          </CardContent>
        </Card>
      )}

      {remoteClusters.length === 0 && !trusted.isLoading && (
        <p className="text-sm text-muted-foreground">{t("trusted.empty")}</p>
      )}
    </div>
  );
}

function ReplicationRulesSection({
  t,
  rules,
  status,
  form,
  onFormChange,
  onAdd,
  onDelete,
  adding,
}: {
  t: (key: string, opts?: Record<string, unknown>) => string;
  rules: SiteReplicationRule[];
  status?: { pending_count: number; lag_seconds: number; last_error?: string };
  form: { source_bucket: string; dest_bucket: string; direction: string };
  onFormChange: (v: { source_bucket: string; dest_bucket: string; direction: string }) => void;
  onAdd: () => void;
  onDelete: (id: string) => void;
  adding: boolean;
}) {
  return (
    <div className="pt-4 mt-4 border-t space-y-3">
      <div>
        <p className="font-medium">{t("replication.title")}</p>
        <p className="text-xs text-muted-foreground">{t("replication.description")}</p>
        {status && (
          <p className="text-xs text-muted-foreground mt-1">
            {t("replication.lag")}: {status.lag_seconds.toFixed(1)}s
            {status.pending_count > 0 ? ` · ${t("replication.pending", { count: status.pending_count })}` : ""}
          </p>
        )}
      </div>
      {rules.length > 0 && (
        <ul className="space-y-2">
          {rules.map((r) => (
            <li key={r.id} className="flex items-center justify-between rounded border p-2 text-xs">
              <span>
                {r.source_bucket} → {r.dest_bucket}
                {r.direction === "bidirectional" ? ` (${t("replication.bidirectional")})` : ""}
              </span>
              <Button variant="ghost" size="sm" onClick={() => onDelete(r.id)}>
                <Trash2 className="h-3 w-3" />
              </Button>
            </li>
          ))}
        </ul>
      )}
      <div className="grid gap-2 sm:grid-cols-2">
        <div>
          <Label>{t("replication.sourceBucket")}</Label>
          <Input
            value={form.source_bucket}
            onChange={(e) => onFormChange({ ...form, source_bucket: e.target.value })}
          />
        </div>
        <div>
          <Label>{t("replication.destBucket")}</Label>
          <Input
            value={form.dest_bucket}
            onChange={(e) => onFormChange({ ...form, dest_bucket: e.target.value })}
          />
        </div>
      </div>
      <div>
        <Label>{t("replication.direction")}</Label>
        <select
          className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm"
          value={form.direction}
          onChange={(e) => onFormChange({ ...form, direction: e.target.value })}
        >
          <option value="one-way">{t("replication.oneWay")}</option>
          <option value="bidirectional">{t("replication.bidirectional")}</option>
        </select>
      </div>
      <Button size="sm" onClick={onAdd} disabled={adding || !form.source_bucket || !form.dest_bucket}>
        <Plus className="h-3 w-3 mr-1" /> {t("replication.addRule")}
      </Button>
    </div>
  );
}

function ClusterRow({
  cluster,
  selected,
  onSelect,
  t,
}: {
  cluster: TrustedCluster;
  selected: boolean;
  onSelect: () => void;
  t: (key: string) => string;
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      className={`flex w-full items-center justify-between rounded border p-3 text-left hover:bg-muted/50 ${selected ? "ring-2 ring-primary" : ""}`}
    >
      <div>
        <p className="font-medium">{cluster.name}{cluster.is_local ? ` (${t("trusted.local")})` : ""}</p>
        <p className="text-xs text-muted-foreground">{cluster.endpoint || "—"}</p>
      </div>
      <Badge variant={cluster.status === "healthy" ? "default" : "secondary"}>{cluster.status}</Badge>
    </button>
  );
}
