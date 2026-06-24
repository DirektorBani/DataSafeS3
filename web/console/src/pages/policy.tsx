import { useEffect, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Save, Shield } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { emptyPolicy, parsePolicy, serializePolicy, type PolicyDocument } from "@/lib/policy";
import { PolicyBuilder } from "@/components/policy-builder";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Label } from "@/components/ui/label";

export function PolicyPage() {
  const { t } = useTranslation(["policy", "common"]);
  const [selectedBucket, setSelectedBucket] = useState("");
  const [policyDoc, setPolicyDoc] = useState<PolicyDocument>(emptyPolicy());
  const [policyJson, setPolicyJson] = useState(serializePolicy(emptyPolicy()));
  const [tab, setTab] = useState("visual");

  const buckets = useQuery({
    queryKey: ["buckets"],
    queryFn: async () => (await api.listBuckets()).buckets,
  });

  const policyQuery = useQuery({
    queryKey: ["policy", selectedBucket],
    queryFn: async () => (await api.getPolicy(selectedBucket)).policy,
    enabled: !!selectedBucket,
  });

  useEffect(() => {
    if (policyQuery.data !== undefined) {
      try {
        const doc = parsePolicy(policyQuery.data || serializePolicy(emptyPolicy()));
        setPolicyDoc(doc);
        setPolicyJson(serializePolicy(doc));
      } catch {
        setPolicyDoc(emptyPolicy());
        setPolicyJson(policyQuery.data || serializePolicy(emptyPolicy()));
      }
    }
  }, [policyQuery.data]);

  const saveMutation = useMutation({
    mutationFn: () => {
      const json = tab === "json" ? policyJson : serializePolicy(policyDoc);
      return api.putPolicy(selectedBucket, json);
    },
    onSuccess: () => toast.success(t("policy:toast.saved")),
    onError: (err: Error) => toast.error(err.message),
  });

  useEffect(() => {
    if (!selectedBucket && buckets.data?.length) {
      setSelectedBucket(buckets.data[0].name);
    }
  }, [buckets.data, selectedBucket]);

  const syncFromVisual = () => setPolicyJson(serializePolicy(policyDoc));

  const syncFromJson = () => {
    try {
      const doc = parsePolicy(policyJson);
      setPolicyDoc(doc);
      toast.success(t("policy:toast.jsonParsed"));
    } catch {
      toast.error(t("policy:toast.invalidJson"));
    }
  };

  return (
    <div>
      <PageHeader
        title={t("policy:title")}
        description={t("policy:description")}
        actions={
          <Button
            size="sm"
            onClick={() => saveMutation.mutate()}
            disabled={!selectedBucket || saveMutation.isPending}
          >
            <Save className="h-4 w-4" />
            {saveMutation.isPending ? t("policy:saving") : t("policy:save")}
          </Button>
        }
      />

      <div className="mb-6 max-w-xs">
        <Label className="mb-2 block">{t("policy:fields.bucket")}</Label>
        <Select value={selectedBucket} onValueChange={setSelectedBucket}>
          <SelectTrigger>
            <SelectValue placeholder={t("policy:placeholder.selectBucket")} />
          </SelectTrigger>
          <SelectContent>
            {(buckets.data ?? []).map((b) => (
              <SelectItem key={b.name} value={b.name}>
                {b.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="visual">
            <Shield className="h-4 w-4 mr-1" />
            {t("policy:tabs.visual")}
          </TabsTrigger>
          <TabsTrigger value="json">{t("policy:tabs.json")}</TabsTrigger>
        </TabsList>
        <TabsContent value="visual" className="mt-4">
          {policyQuery.isLoading ? (
            <p className="text-muted-foreground">{t("policy:loading")}</p>
          ) : (
            <PolicyBuilder
              bucket={selectedBucket}
              document={policyDoc}
              onChange={(doc) => {
                setPolicyDoc(doc);
                setPolicyJson(serializePolicy(doc));
              }}
            />
          )}
        </TabsContent>
        <TabsContent value="json" className="mt-4 space-y-3">
          {policyQuery.isLoading ? (
            <p className="text-muted-foreground">{t("policy:loading")}</p>
          ) : (
            <>
              <Textarea
                value={policyJson}
                onChange={(e) => setPolicyJson(e.target.value)}
                rows={18}
                className="font-mono text-sm"
              />
              <Button type="button" variant="outline" size="sm" onClick={syncFromJson}>
                {t("policy:json.apply")}
              </Button>
            </>
          )}
        </TabsContent>
      </Tabs>

      {tab === "visual" && (
        <p className="mt-4 text-xs text-muted-foreground">
          {t("policy:hint.visual")}
        </p>
      )}
      {tab === "json" && (
        <Button type="button" variant="ghost" size="sm" className="mt-2" onClick={syncFromVisual}>
          {t("policy:json.refresh")}
        </Button>
      )}
    </div>
  );
}
