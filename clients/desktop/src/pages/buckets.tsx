import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@datasafe/ui/ui/button";
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

export function BucketBrowser(props: { profileId: string; onError: (message: string) => void; onMfaSetup: () => void }) {
  const { t } = useTranslation();
  const step = useStepUp(props.profileId, props.onMfaSetup);
  const [buckets, setBuckets] = useState<BucketItem[]>([]);
  const [filter, setFilter] = useState("all");
  const [name, setName] = useState("");
  const [selected, setSelected] = useState("");
  const [confirm, setConfirm] = useState(false);
  const [encryption, setEncryption] = useState("");

  const { profileId, onError } = props;
  const load = useCallback(async () => {
    const result = await browse(profileId, "bucket.list", { filter });
    const message = problem(result);
    if (message) {
      onError(message);
      return;
    }
    const items = Array.isArray(result.body?.buckets) ? (result.body.buckets as BucketItem[]) : [];
    setBuckets(items);
  }, [filter, onError, profileId]);

  useEffect(() => {
    void load();
  }, [load]);

  const current = buckets.find((bucket) => bucket.name === selected);
  const canWrite = current?.access?.can_write === true;

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

  return (
    <section className="space-y-4">
      <StepUpDialog open={step.open} error={step.error} onCancel={step.cancel} onSubmit={(code) => void step.submit(code)} />
      <div className="flex flex-wrap items-end gap-2">
        <label className="space-y-1 text-sm">
          <span className="block">{t("buckets")}</span>
          <select className="rounded-md border bg-transparent px-2 py-1" value={filter} onChange={(event) => setFilter(event.target.value)}>
            <option value="all">all</option>
            <option value="owned">owned</option>
            <option value="shared">shared</option>
          </select>
        </label>
        <ul className="flex flex-wrap gap-2">
          {buckets.map((bucket) => (
            <li key={bucket.name}>
              <Button type="button" variant={bucket.name === selected ? "default" : "outline"} onClick={() => setSelected(bucket.name)}>
                {bucket.name}
                {bucket.object_lock_enabled ? " · lock" : ""}
              </Button>
            </li>
          ))}
        </ul>
      </div>
      <div className="flex flex-wrap gap-2">
        <input className="rounded-md border bg-transparent px-2 py-1" placeholder={t("createBucket")} value={name} onChange={(event) => setName(event.target.value)} />
        <Button type="button" onClick={() => void createBucket()}>
          {t("createBucket")}
        </Button>
        {selected && canWrite ? (
          confirm ? (
            <>
              <Button type="button" onClick={() => void deleteBucket()}>
                {t("confirmDelete")}
              </Button>
              <Button type="button" variant="outline" onClick={() => setConfirm(false)}>
                {t("cancel")}
              </Button>
            </>
          ) : (
            <Button type="button" variant="outline" onClick={() => setConfirm(true)}>
              {t("deleteBucket")}
            </Button>
          )
        ) : null}
      </div>
      {current && current.access?.can_write !== true ? <p className="text-sm">{t("readOnly")}</p> : null}
      {selected ? (
        <ObjectBrowser
          profileId={props.profileId}
          bucket={selected}
          canWrite={canWrite}
          encryption={encryption}
          onError={props.onError}
          confirmDangerous={step.call}
        />
      ) : null}
    </section>
  );
}
