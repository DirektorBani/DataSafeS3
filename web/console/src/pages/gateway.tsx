import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { CheckCircle2, Plus, RefreshCw, Trash2, XCircle } from "lucide-react";
import { toast } from "sonner";
import { api, type GatewayConnection, type ReplicationRule } from "@/lib/api";
import { formatBytes } from "@/lib/utils";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";

export function GatewayPage() {
  const { t, i18n } = useTranslation(["gateway", "common"]);
  const queryClient = useQueryClient();
  const formRef = useRef<HTMLDivElement>(null);
  const [tab, setTab] = useState("connections");
  const [connForm, setConnForm] = useState({
    name: "",
    endpoint: "",
    region: "us-east-1",
    access_key: "",
    secret_key: "",
    path_style: true,
    tls_verify: false,
  });
  const [ruleForm, setRuleForm] = useState({ source_bucket: "", dest_connection_id: "", dest_bucket: "" });

  const health = useQuery({ queryKey: ["gateway-health"], queryFn: () => api.gatewayHealth(), refetchInterval: 5000 });
  const connections = useQuery({ queryKey: ["gateway-connections"], queryFn: async () => (await api.listGatewayConnections()).connections });
  const buckets = useQuery({ queryKey: ["buckets"], queryFn: async () => (await api.listBuckets()).buckets });
  const rules = useQuery({ queryKey: ["replication-rules"], queryFn: async () => (await api.listReplicationRules()).rules });
  const jobs = useQuery({ queryKey: ["sync-jobs"], queryFn: async () => (await api.listSyncJobs()).jobs });
  const queue = useQuery({ queryKey: ["repl-queue"], queryFn: async () => (await api.listReplicationQueue("pending")).tasks, refetchInterval: 5000 });
  const failedQueue = useQuery({ queryKey: ["repl-queue-failed"], queryFn: async () => (await api.listReplicationQueue("failed")).tasks, refetchInterval: 5000 });

  const retryFailed = useMutation({
    mutationFn: () => api.retryFailedReplication(),
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: ["repl-queue"] });
      queryClient.invalidateQueries({ queryKey: ["repl-queue-failed"] });
      queryClient.invalidateQueries({ queryKey: ["gateway-health"] });
      toast.success(res.retried > 0 ? t("gateway:toast.requeued") : t("gateway:toast.noFailed"));
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const clearErrors = useMutation({
    mutationFn: () => api.clearReplicationErrors(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["repl-queue-failed"] });
      queryClient.invalidateQueries({ queryKey: ["gateway-health"] });
      toast.success(t("gateway:toast.errorsCleared"));
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const createConn = useMutation({
    mutationFn: () => api.createGatewayConnection(connForm),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["gateway-connections"] });
      setConnForm({ name: "", endpoint: "", region: "us-east-1", access_key: "", secret_key: "", path_style: true, tls_verify: false });
      toast.success(t("gateway:toast.connectionCreated"));
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const createRule = useMutation({
    mutationFn: () => api.createReplicationRule(ruleForm),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["replication-rules"] });
      queryClient.invalidateQueries({ queryKey: ["repl-queue"] });
      setRuleForm({ source_bucket: "", dest_connection_id: "", dest_bucket: "" });
      toast.success(t("gateway:toast.ruleCreated"));
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const connName = (id: string) => connections.data?.find((c) => c.id === id)?.name ?? id.slice(0, 8);
  const connList = connections.data ?? [];
  const hasConnections = connList.length > 0;

  const scrollToCreate = () => {
    setTab("connections");
    setTimeout(() => formRef.current?.scrollIntoView({ behavior: "smooth" }), 50);
  };

  const apiError = connections.error || health.error || rules.error;

  return (
    <div>
      <PageHeader
        title={t("gateway:title", { brand: t("common:brand") })}
        description={t("gateway:description")}
        actions={
          <Button variant="outline" size="sm" onClick={() => { health.refetch(); connections.refetch(); queue.refetch(); }}>
            <RefreshCw className="h-4 w-4" />
            {t("common:refresh")}
          </Button>
        }
      />

      {apiError && (
        <Card className="mb-4 border-destructive/50">
          <CardContent className="py-3 text-sm text-destructive">
            {t("gateway:error.load", { message: (apiError as Error).message })}
          </CardContent>
        </Card>
      )}

      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="connections">{t("gateway:tabs.connections")}</TabsTrigger>
          <TabsTrigger value="rules">{t("gateway:tabs.rules")}</TabsTrigger>
          <TabsTrigger value="jobs">{t("gateway:tabs.jobs")}</TabsTrigger>
          <TabsTrigger value="health">{t("gateway:tabs.health")}</TabsTrigger>
        </TabsList>

        <TabsContent value="connections" className="space-y-4">
          {!hasConnections && !connections.isLoading && (
            <Card className="border-dashed">
              <CardContent className="flex flex-col items-center justify-center py-12 text-center gap-3">
                <p className="text-muted-foreground">{t("gateway:connections.empty")}</p>
                <Button onClick={scrollToCreate}>
                  <Plus className="h-4 w-4" /> {t("gateway:connections.create")}
                </Button>
              </CardContent>
            </Card>
          )}

          <Card ref={formRef}>
            <CardHeader>
              <CardTitle className="text-base">{t("gateway:connections.add.title")}</CardTitle>
              <CardDescription>{t("gateway:connections.add.description")}</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-3 sm:grid-cols-2">
              <div><Label>{t("gateway:fields.name")}</Label><Input value={connForm.name} onChange={(e) => setConnForm({ ...connForm, name: e.target.value })} /></div>
              <div><Label>{t("gateway:fields.endpoint")}</Label><Input value={connForm.endpoint} onChange={(e) => setConnForm({ ...connForm, endpoint: e.target.value })} /></div>
              <div><Label>{t("gateway:fields.region")}</Label><Input value={connForm.region} onChange={(e) => setConnForm({ ...connForm, region: e.target.value })} /></div>
              <div><Label>{t("gateway:fields.accessKey")}</Label><Input value={connForm.access_key} onChange={(e) => setConnForm({ ...connForm, access_key: e.target.value })} /></div>
              <div><Label>{t("gateway:fields.secretKey")}</Label><Input type="password" value={connForm.secret_key} onChange={(e) => setConnForm({ ...connForm, secret_key: e.target.value })} /></div>
              <div className="flex flex-col gap-3 justify-end">
                <label className="flex items-center gap-2 text-sm cursor-pointer">
                  <input type="checkbox" checked={connForm.path_style} onChange={(e) => setConnForm({ ...connForm, path_style: e.target.checked })} className="rounded" />
                  {t("gateway:pathStyle")}
                </label>
                <label className="flex items-center gap-2 text-sm cursor-pointer">
                  <input type="checkbox" checked={connForm.tls_verify} onChange={(e) => setConnForm({ ...connForm, tls_verify: e.target.checked })} className="rounded" />
                  {t("gateway:tlsVerify")}
                </label>
              </div>
              <div className="sm:col-span-2">
                <Button onClick={() => createConn.mutate()} disabled={createConn.isPending || !connForm.name || !connForm.endpoint}>
                  <Plus className="h-4 w-4" /> {t("gateway:connections.addAction")}
                </Button>
              </div>
            </CardContent>
          </Card>

          {connections.isLoading && <p className="text-sm text-muted-foreground">{t("gateway:connections.loading")}</p>}
          {connList.map((c: GatewayConnection) => (
            <Card key={c.id}>
              <CardContent className="flex items-center justify-between py-4">
                <div>
                  <p className="font-medium flex items-center gap-2">
                    {c.name}
                    {c.status === "ok" ? <CheckCircle2 className="h-4 w-4 text-green-600" /> : c.status === "error" ? <XCircle className="h-4 w-4 text-destructive" /> : null}
                  </p>
                  <p className="text-xs text-muted-foreground">{c.endpoint} · {c.region}</p>
                  <p className="text-xs text-muted-foreground font-mono">ID: {c.id}</p>
                </div>
                <div className="flex gap-2">
                  <Button size="sm" variant="outline" onClick={async () => {
                    const r = await api.testGatewayConnection(c.id);
                    toast[r.ok ? "success" : "error"](r.message);
                    queryClient.invalidateQueries({ queryKey: ["gateway-connections"] });
                    queryClient.invalidateQueries({ queryKey: ["gateway-health"] });
                  }}>{t("gateway:connections.test")}</Button>
                  <Button size="sm" variant="ghost" onClick={async () => {
                    await api.deleteGatewayConnection(c.id);
                    queryClient.invalidateQueries({ queryKey: ["gateway-connections"] });
                  }}><Trash2 className="h-4 w-4" /></Button>
                </div>
              </CardContent>
            </Card>
          ))}
        </TabsContent>

        <TabsContent value="rules" className="space-y-4">
          {!hasConnections && (
            <Card className="border-amber-500/50 bg-amber-500/5">
              <CardContent className="py-4 text-sm">{t("gateway:rules.hint")}</CardContent>
            </Card>
          )}
          <Card>
            <CardHeader>
              <CardTitle className="text-base">{t("gateway:rules.add.title")}</CardTitle>
              <CardDescription>{t("gateway:rules.add.description")}</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-3 sm:grid-cols-3">
              <div>
                <Label>{t("gateway:fields.localBucket")}</Label>
                <Select value={ruleForm.source_bucket || "__none__"} onValueChange={(v) => setRuleForm({ ...ruleForm, source_bucket: v === "__none__" ? "" : v })}>
                  <SelectTrigger><SelectValue placeholder={t("gateway:rules.selectBucket")} /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__none__">{t("gateway:rules.selectBucket")}</SelectItem>
                    {buckets.data?.map((b) => (
                      <SelectItem key={b.name} value={b.name}>{b.name}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div>
                <Label>{t("gateway:fields.remoteConnection")}</Label>
                <Select value={ruleForm.dest_connection_id || "__none__"} onValueChange={(v) => setRuleForm({ ...ruleForm, dest_connection_id: v === "__none__" ? "" : v })}>
                  <SelectTrigger><SelectValue placeholder={t("gateway:rules.selectConnection")} /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__none__">{t("gateway:rules.selectConnection")}</SelectItem>
                    {connList.map((c) => (
                      <SelectItem key={c.id} value={c.id}>{c.name}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div>
                <Label>{t("gateway:fields.remoteBucket")}</Label>
                <Input value={ruleForm.dest_bucket} onChange={(e) => setRuleForm({ ...ruleForm, dest_bucket: e.target.value })} />
              </div>
              <div className="sm:col-span-3">
                <Button
                  onClick={() => createRule.mutate()}
                  disabled={createRule.isPending || !ruleForm.source_bucket || !ruleForm.dest_connection_id || !ruleForm.dest_bucket}
                >
                  <Plus className="h-4 w-4" /> {t("gateway:rules.addAction")}
                </Button>
              </div>
            </CardContent>
          </Card>
          {(rules.data ?? []).length === 0 && !rules.isLoading && (
            <p className="text-sm text-muted-foreground text-center py-4">{t("gateway:rules.empty")}</p>
          )}
          {(rules.data ?? []).map((r: ReplicationRule) => (
            <Card key={r.id}>
              <CardContent className="flex items-center justify-between py-4">
                <div>
                  <p className="font-medium">{r.source_bucket} → {r.dest_bucket}</p>
                  <p className="text-xs text-muted-foreground">via {connName(r.dest_connection_id)}</p>
                </div>
                <div className="flex gap-2">
                  <Button size="sm" variant="outline" onClick={async () => {
                    const res = await api.triggerSyncJob(r.id);
                    toast.success(res.job.message ?? String(res.job.objects_synced));
                    queryClient.invalidateQueries({ queryKey: ["sync-jobs"] });
                    queryClient.invalidateQueries({ queryKey: ["gateway-health"] });
                  }}>{t("gateway:rules.syncNow")}</Button>
                  <Button size="sm" variant="ghost" onClick={async () => {
                    await api.deleteReplicationRule(r.id);
                    queryClient.invalidateQueries({ queryKey: ["replication-rules"] });
                  }}><Trash2 className="h-4 w-4" /></Button>
                </div>
              </CardContent>
            </Card>
          ))}
        </TabsContent>

        <TabsContent value="jobs" className="space-y-2">
          {(jobs.data ?? []).length === 0 && !jobs.isLoading && (
            <p className="text-sm text-muted-foreground py-4">{t("gateway:jobs.empty")}</p>
          )}
          {(jobs.data ?? []).map((j) => (
            <Card key={j.id}>
              <CardContent className="py-3 text-sm">
                <p className="font-medium">{j.status} — {j.objects_synced} objects, {j.errors} errors</p>
                <p className="text-xs text-muted-foreground">{j.message}</p>
                <p className="text-xs text-muted-foreground">{new Date(j.started_at).toLocaleString(i18n.language)}{j.ended_at && ` → ${new Date(j.ended_at).toLocaleString(i18n.language)}`}</p>
              </CardContent>
            </Card>
          ))}
        </TabsContent>

        <TabsContent value="health" className="space-y-4">
          {health.isLoading && <p className="text-sm text-muted-foreground">{t("gateway:health.loading")}</p>}
          {health.isError && (
            <p className="text-sm text-destructive">{t("gateway:health.failed")} {(health.error as Error).message}</p>
          )}
          {health.data && (
            <>
              <Card>
                <CardHeader><CardTitle className="text-base">{t("gateway:health.title")}</CardTitle></CardHeader>
                <CardContent className="text-sm grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                  <div>
                    <p className="text-muted-foreground">{t("gateway:health.connections")}</p>
                    <p className="font-medium">{health.data.connections_ok}/{health.data.connections_total} OK</p>
                  </div>
                  <div>
                    <p className="text-muted-foreground">{t("gateway:health.rules")}</p>
                    <p className="font-medium">{health.data.rules_total}</p>
                  </div>
                  <div>
                    <p className="text-muted-foreground">{t("gateway:health.queuePending")}</p>
                    <p className="font-medium">{health.data.queue_pending}</p>
                  </div>
                  <div>
                    <p className="text-muted-foreground">{t("gateway:health.replicated")}</p>
                    <p className="font-medium">{formatBytes(health.data.bytes_replicated, i18n.language)} · {health.data.tasks_completed} tasks</p>
                  </div>
                  {health.data.replication_errors > 0 && (
                    <div className="sm:col-span-2 lg:col-span-4 space-y-3">
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge variant="destructive">{health.data.replication_errors} {t("gateway:health.replicationErrors")}</Badge>
                        {health.data.rules_broken ? (
                          <Badge variant="outline">{health.data.rules_broken} {t("gateway:health.missingConnections")}</Badge>
                        ) : null}
                        <Button size="sm" variant="outline" onClick={() => retryFailed.mutate()} disabled={retryFailed.isPending}>
                          {t("gateway:health.retryFailed")}
                        </Button>
                        <Button size="sm" variant="ghost" onClick={() => clearErrors.mutate()} disabled={clearErrors.isPending}>
                          {t("gateway:health.clearErrors")}
                        </Button>
                      </div>
                    </div>
                  )}
                </CardContent>
              </Card>

              <Card>
                <CardHeader><CardTitle className="text-base">{t("gateway:queue.title")}</CardTitle></CardHeader>
                <CardContent className="space-y-2">
                  <p className="text-sm text-muted-foreground">{t("gateway:queue.description")}</p>
                  {(queue.data ?? []).length === 0 && (failedQueue.data ?? []).length === 0 && (
                    <p className="text-sm text-muted-foreground py-4">{t("gateway:queue.empty")}</p>
                  )}
                  {(queue.data ?? []).slice(0, 50).map((task) => (
                    <Card key={task.id}>
                      <CardContent className="py-2 text-sm flex justify-between">
                        <span><Badge variant="outline" className="mr-2">{task.event}</Badge>{task.source_bucket}/{task.key}</span>
                        <span className="text-muted-foreground">{task.attempts > 0 && t("gateway:queue.retry", { n: task.attempts })}</span>
                      </CardContent>
                    </Card>
                  ))}
                  {(failedQueue.data ?? []).length > 0 && (
                    <>
                      <p className="text-sm font-medium text-destructive pt-2">{t("gateway:queue.failed")}</p>
                      {(failedQueue.data ?? []).slice(0, 50).map((task) => (
                        <Card key={task.id} className="border-destructive/30">
                          <CardContent className="py-2 text-sm">
                            <p>
                              <Badge variant="destructive" className="mr-2">{task.event}</Badge>
                              {task.source_bucket}/{task.key}
                            </p>
                            {task.error && <p className="text-xs text-muted-foreground mt-1">{task.error}</p>}
                          </CardContent>
                        </Card>
                      ))}
                    </>
                  )}
                </CardContent>
              </Card>
            </>
          )}
        </TabsContent>
      </Tabs>
    </div>
  );
}
