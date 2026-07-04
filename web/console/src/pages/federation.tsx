import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Plus, Trash2, Wifi } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

export function FederationPage() {
  const { t } = useTranslation(["federation", "common"]);
  const queryClient = useQueryClient();
  const [form, setForm] = useState({ name: "", endpoint: "", region: "us-east-1", cluster_id: "local" });

  const trusted = useQuery({
    queryKey: ["trusted-clusters"],
    queryFn: () => api.listTrustedClusters(),
  });

  const clusterOptions = useMemo(() => {
    const list = trusted.data?.clusters ?? [];
    const local = list.find((c) => c.is_local);
    const localId = local?.id ?? "local";
    const opts = [{ id: localId, name: local?.name ?? t("federation:cluster.local") }];
    for (const c of list) {
      if (!c.is_local && c.active) {
        opts.push({ id: c.id, name: c.name });
      }
    }
    return opts;
  }, [trusted.data, t]);

  const defaultClusterId = clusterOptions[0]?.id ?? "local";

  const clusters = useQuery({
    queryKey: ["federation-clusters"],
    queryFn: async () => (await api.listFederationClusters()).clusters,
  });

  const clusterLabel = (id: string) => clusterOptions.find((c) => c.id === id)?.name ?? id;

  const create = useMutation({
    mutationFn: () =>
      api.createFederationCluster({
        ...form,
        cluster_id: form.cluster_id || defaultClusterId,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["federation-clusters"] });
      setForm({ name: "", endpoint: "", region: "us-east-1", cluster_id: defaultClusterId });
      toast.success(t("federation:toast.registered"));
    },
    onError: (e: Error) => toast.error(e.message),
  });

  return (
    <div>
      <PageHeader title={t("federation:title")} description={t("federation:description", { brand: t("common:brand") })} />
      <Card className="mb-6">
        <CardContent className="grid gap-3 pt-6 sm:grid-cols-2 lg:grid-cols-4">
          <div>
            <Label>{t("federation:fields.name")}</Label>
            <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          </div>
          <div>
            <Label>{t("federation:fields.endpoint")}</Label>
            <Input
              value={form.endpoint}
              onChange={(e) => setForm({ ...form, endpoint: e.target.value })}
              placeholder={t("federation:placeholder.endpoint")}
            />
          </div>
          <div>
            <Label>{t("federation:fields.cluster")}</Label>
            <Select
              value={form.cluster_id || defaultClusterId}
              onValueChange={(v) => setForm({ ...form, cluster_id: v })}
            >
              <SelectTrigger>
                <SelectValue placeholder={t("federation:fields.cluster")} />
              </SelectTrigger>
              <SelectContent>
                {clusterOptions.map((c) => (
                  <SelectItem key={c.id} value={c.id}>
                    {c.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex items-end">
            <Button onClick={() => create.mutate()} disabled={create.isPending}>
              <Plus className="h-4 w-4" /> {t("federation:actions.register")}
            </Button>
          </div>
        </CardContent>
      </Card>
      {clusters.data?.map((c) => (
        <Card key={c.id} className="mb-2">
          <CardContent className="flex justify-between py-4">
            <div>
              <p className="font-medium">{c.name}</p>
              <p className="text-xs text-muted-foreground">
                {c.endpoint} — {c.status}
              </p>
              <p className="text-xs text-muted-foreground">
                {t("federation:fields.cluster")}: {clusterLabel(c.cluster_id)}
              </p>
            </div>
            <div className="flex gap-2">
              <Button
                size="sm"
                variant="outline"
                onClick={async () => {
                  try {
                    const res = await api.testFederationCluster(c.id);
                    toast.success(res.detail || res.status);
                    queryClient.invalidateQueries({ queryKey: ["federation-clusters"] });
                  } catch (e: unknown) {
                    toast.error(e instanceof Error ? e.message : "test failed");
                  }
                }}
              >
                <Wifi className="h-4 w-4" />
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={async () => {
                  await api.deleteFederationCluster(c.id);
                  queryClient.invalidateQueries({ queryKey: ["federation-clusters"] });
                }}
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}
