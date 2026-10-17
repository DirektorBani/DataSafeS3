import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from "react";
import { useTranslation } from "react-i18next";
import { ArrowLeft, FolderOpen, Plus, Trash2 } from "lucide-react";
import { Button } from "@datasafe/ui/ui/button";
import { Input } from "@datasafe/ui/ui/input";
import { Card, CardContent } from "@datasafe/ui/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@datasafe/ui/ui/tabs";
import { PageHeader } from "@datasafe/ui/page-header";
import { StepUpDialog, useStepUp } from "../components/step-up";
import { browse, type BrowseResult } from "../desktop";
import { ObjectBrowser } from "./objects";

type BucketItem = {
  name: string;
  access?: { can_write?: boolean; ownership?: string };
  object_lock_enabled?: boolean;
};

function problem(result: BrowseResult): string {
  if (result.step_up) {
    return result.message || "step_up";
  }
  if (result.status >= 400) {
    return result.message || result.code || String(result.status);
  }
  return "";
}

export function BucketBrowser(props: { profileId: string; features?: Record<string, number>; resetToken: number; onError: (message: string) => void; onMfaSetup: () => void }) {
  const { t, i18n } = useTranslation(["shell", "buckets", "common"]);
  const step = useStepUp(props.profileId, props.onMfaSetup);
  const [buckets, setBuckets] = useState<BucketItem[]>([]);
  const [filter, setFilter] = useState("all");
  const [name, setName] = useState("");
  const [selected, setSelected] = useState("");
  const [confirm, setConfirm] = useState(false);
  const [encryption, setEncryption] = useState("");
  const [offline, setOffline] = useState(false);
  const [savedAt, setSavedAt] = useState(0);
  const [cursor, setCursor] = useState(0);
  const bucketRefs = useRef<Array<HTMLTableRowElement | null>>([]);

  const { profileId, onError } = props;
  const load = useCallback(async () => {
    const result = await browse(profileId, "bucket.list", { filter });
    const message = problem(result);
    if (message) {
      onError(message);
      return;
    }
    setOffline(Boolean(result.cached));
    setSavedAt(typeof result.cached_at === "number" ? result.cached_at : 0);
    const items = Array.isArray(result.body?.buckets) ? (result.body.buckets as BucketItem[]) : [];
    setBuckets(items);
  }, [filter, onError, profileId]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    setSelected("");
  }, [props.resetToken]);

  const current = buckets.find((bucket) => bucket.name === selected);
  const canWrite = current?.access?.can_write === true && !offline;
  const savedLabel = savedAt > 0 ? new Intl.DateTimeFormat(i18n.language, { dateStyle: "medium", timeStyle: "short" }).format(new Date(savedAt * 1000)) : "";

  useEffect(() => {
    if (!selected) {
      setEncryption("");
      return;
    }
    let cancel = false;
    void browse(props.profileId, "bucket.encryption", { bucket: selected }).then((result) => {
      if (cancel) {
        return;
      }
      if (result.status === 200 && result.body?.enabled === true) {
        const algorithm = typeof result.body.algorithm === "string" ? result.body.algorithm : "on";
        setEncryption(algorithm);
      } else {
        setEncryption("");
      }
    });
    return () => {
      cancel = true;
    };
  }, [props.profileId, selected]);

  async function createBucket() {
    const result = await browse(props.profileId, "bucket.create", { bucket: name.trim() });
    const message = problem(result);
    if (message) {
      props.onError(message);
      return;
    }
    setName("");
    await load();
  }

  async function deleteBucket() {
    setConfirm(false);
    const result = await step.call("bucket.delete", { bucket: selected });
    if (result.code === "cancelled" || result.mfa_setup) {
      return;
    }
    const message = problem(result);
    if (message) {
      props.onError(message);
      return;
    }
    setSelected("");
    await load();
  }

  function onBucketKey(event: KeyboardEvent<HTMLTableRowElement>, index: number) {
    if (event.target !== event.currentTarget) {
      return;
    }
    const last = Math.max(buckets.length - 1, 0);
    const go = (next: number) => {
      const clamped = Math.max(0, Math.min(next, last));
      setCursor(clamped);
      bucketRefs.current[clamped]?.focus();
    };
    if (event.key === "ArrowDown") {
      event.preventDefault();
      go(index + 1);
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      go(index - 1);
    } else if (event.key === "Home") {
      event.preventDefault();
      go(0);
    } else if (event.key === "End") {
      event.preventDefault();
      go(last);
    } else if (event.key === "Enter") {
      event.preventDefault();
      const bucket = buckets[index];
      if (bucket) {
        setCursor(index);
        setSelected(bucket.name);
      }
    }
  }

  return (
    <section className="space-y-4">
      <StepUpDialog open={step.open} error={step.error} onCancel={step.cancel} onSubmit={(code) => void step.submit(code)} />
      {offline ? (
        <p className="text-sm text-muted-foreground" role="status">
          {t("offlineSaved", { time: savedLabel })}
        </p>
      ) : null}
      {selected ? (
        <>
          <div className="mb-4">
            <Button variant="ghost" size="sm" type="button" onClick={() => setSelected("")}>
              <ArrowLeft className="h-4 w-4" aria-hidden="true" />
              {t("bucketDetail:backToBuckets")}
            </Button>
          </div>
          {current && current.access?.can_write !== true ? <p className="mb-4 text-sm text-muted-foreground">{t("readOnly")}</p> : null}
          <ObjectBrowser
            profileId={props.profileId}
            bucket={selected}
            canWrite={canWrite}
            offline={offline}
            encryption={encryption}
            features={props.features}
            onError={props.onError}
            confirmDangerous={step.call}
            headerActions={
              canWrite ? (
                confirm ? (
                  <>
                    <Button type="button" variant="destructive" size="sm" onClick={() => void deleteBucket()}>
                      {t("confirmDelete")}
                    </Button>
                    <Button type="button" variant="outline" size="sm" onClick={() => setConfirm(false)}>
                      {t("common:cancel")}
                    </Button>
                  </>
                ) : (
                  <Button type="button" variant="outline" size="sm" onClick={() => setConfirm(true)}>
                    <Trash2 className="h-4 w-4" aria-hidden="true" />
                    {t("deleteBucket")}
                  </Button>
                )
              ) : null
            }
          />
        </>
      ) : (
        <>
          <PageHeader
            title={t("buckets:title")}
            description={t("buckets:description")}
            actions={
              <>
                <Input className="w-48" aria-label={t("buckets:fields.bucketName")} placeholder={t("buckets:placeholder.bucketName")} value={name} onChange={(event) => setName(event.target.value)} />
                <Button type="button" onClick={() => void createBucket()} disabled={!name.trim() || offline}>
                  <Plus className="h-4 w-4" aria-hidden="true" />
                  {t("buckets:actions.create")}
                </Button>
              </>
            }
          />
          <Tabs value={filter} onValueChange={setFilter}>
            <TabsList>
              <TabsTrigger value="all">{t("filterAll")}</TabsTrigger>
              <TabsTrigger value="owned">{t("buckets:tabs.myFiles")}</TabsTrigger>
              <TabsTrigger value="shared">{t("buckets:tabs.shared")}</TabsTrigger>
            </TabsList>
            <TabsContent value="all" className="hidden" />
            <TabsContent value="owned" className="hidden" />
            <TabsContent value="shared" className="hidden" />
          </Tabs>
          <Card>
            <CardContent className="p-0">
              <table
                className="w-full text-sm"
                role="grid"
                aria-label={t("buckets:title")}
                aria-multiselectable="false"
                aria-activedescendant={buckets.length > 0 ? `bucket-row-${Math.min(cursor, buckets.length - 1)}` : undefined}
              >
                <thead>
                  <tr role="row" className="border-b text-left text-muted-foreground">
                    <th role="columnheader" className="p-3">{t("buckets:columns.name")}</th>
                    <th role="columnheader" className="p-3">{t("buckets:columns.access")}</th>
                  </tr>
                </thead>
                <tbody>
                  {buckets.length === 0 ? (
                    <tr role="row">
                      <td role="gridcell" colSpan={2} className="p-8 text-center text-muted-foreground">
                        {t("buckets:empty.owned")}
                      </td>
                    </tr>
                  ) : null}
                  {buckets.map((bucket, index) => {
                    const active = index === Math.min(cursor, buckets.length - 1);
                    return (
                      <tr
                        key={bucket.name}
                        id={`bucket-row-${index}`}
                        role="row"
                        tabIndex={active ? 0 : -1}
                        aria-selected={active}
                        ref={(node) => {
                          bucketRefs.current[index] = node;
                        }}
                        className={`cursor-pointer border-b hover:bg-muted/50 outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring${active ? " bg-muted/30" : ""}`}
                        onClick={() => {
                          setCursor(index);
                          setSelected(bucket.name);
                        }}
                        onKeyDown={(event) => onBucketKey(event, index)}
                      >
                        <td role="gridcell" aria-selected={active} className="p-3">
                          <span className="inline-flex items-center gap-2 font-medium text-primary">
                            <FolderOpen className="h-4 w-4" aria-hidden="true" />
                            {bucket.name}
                            {bucket.object_lock_enabled ? " · lock" : ""}
                          </span>
                        </td>
                        <td role="gridcell" aria-selected={active} className="p-3 capitalize text-muted-foreground">{bucket.access?.ownership || "—"}</td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </CardContent>
          </Card>
        </>
      )}
    </section>
  );
}
