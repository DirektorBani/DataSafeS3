import { useState } from "react";
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

export function FederationPage() {
  const { t } = useTranslation(["federation", "common"]);
  const queryClient = useQueryClient();
  const [form, setForm] = useState({ name: "", endpoint: "", region: "us-east-1" });

  const clusters = useQuery({
    queryKey: ["federation-clusters"],
    queryFn: async () => (await api.listFederationClusters()).clusters,
  });

  const create = useMutation({
    mutationFn: () => api.createFederationCluster(form),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["federation-clusters"] });
      setForm({ name: "", endpoint: "", region: "us-east-1" });
      toast.success(t("federation:toast.registered"));
    },
    onError: (e: Error) => toast.error(e.message),
  });

  return (
    <div>
      <PageHeader title={t("federation:title")} description={t("federation:description", { brand: t("common:brand") })} />
      <Card className="mb-6">
        <CardContent className="grid gap-3 pt-6 sm:grid-cols-3">
          <div><Label>{t("federation:fields.name")}</Label><Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></div>
          <div><Label>{t("federation:fields.endpoint")}</Label><Input value={form.endpoint} onChange={(e) => setForm({ ...form, endpoint: e.target.value })} placeholder={t("federation:placeholder.endpoint")} /></div>
          <div className="flex items-end"><Button onClick={() => create.mutate()} disabled={create.isPending}><Plus className="h-4 w-4" /> {t("federation:actions.register")}</Button></div>
        </CardContent>
      </Card>
      {clusters.data?.map((c) => (
        <Card key={c.id} className="mb-2">
          <CardContent className="flex justify-between py-4">
            <div>
              <p className="font-medium">{c.name}</p>
              <p className="text-xs text-muted-foreground">{c.endpoint} — {c.status}</p>
            </div>
            <div className="flex gap-2">
              <Button size="sm" variant="outline" onClick={async () => {
                try {
                  const res = await api.testFederationCluster(c.id);
                  toast.success(res.detail || res.status);
                  queryClient.invalidateQueries({ queryKey: ["federation-clusters"] });
                } catch (e: unknown) {
                  toast.error(e instanceof Error ? e.message : "test failed");
                }
              }}><Wifi className="h-4 w-4" /></Button>
              <Button size="sm" variant="ghost" onClick={async () => {
                await api.deleteFederationCluster(c.id);
                queryClient.invalidateQueries({ queryKey: ["federation-clusters"] });
              }}><Trash2 className="h-4 w-4" /></Button>
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}
