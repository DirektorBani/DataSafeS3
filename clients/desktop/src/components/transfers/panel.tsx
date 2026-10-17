import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { getCurrentWebview } from "@tauri-apps/api/webview";
import { Button } from "@datasafe/ui/ui/button";
import { transfer, type TransferDetail, type TransferTask } from "../../desktop";

function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) {
    return "0 B";
  }
  if (n < 1024) {
    return `${Math.round(n)} B`;
  }
  const units = ["KiB", "MiB", "GiB", "TiB"];
  let value = n / 1024;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value >= 10 ? value.toFixed(0) : value.toFixed(1)} ${units[unit]}`;
}

const messages: Record<string, string> = {
  waiting_network: "waitingNetwork",
  file_changed: "fileChanged",
  upload_not_found: "uploadNotFound",
  etag_mismatch: "etagMismatch",
  presign_unavailable: "presignUnavailable",
  resume_after_restart: "resumeAfterRestart",
};

export function TransferPanel(props: {
  profileId: string;
  bucket: string;
  prefix: string;
  canWrite: boolean;
  selectedKey: string;
  selectedFolder: boolean;
}) {
  const { t } = useTranslation();
  const [localPath, setLocalPath] = useState("");
  const [destPath, setDestPath] = useState("");
  const [follow, setFollow] = useState(false);
  const [tasks, setTasks] = useState<TransferTask[]>([]);
  const [error, setError] = useState("");
  const [applyAll, setApplyAll] = useState(false);
  const [detailId, setDetailId] = useState("");
  const [detail, setDetail] = useState<TransferDetail | null>(null);

  useEffect(() => {
    let stop = false;
    async function tick() {
      try {
        const result = await transfer(props.profileId, "transfer.list");
        if (!stop) {
          setTasks((result.transfers ?? []).filter((task) => !task.profile_id || task.profile_id === props.profileId));
          setError("");
        }
      } catch (err) {
        if (!stop) {
          const message = err instanceof Error ? err.message : String(err);
          if (!message.includes("invoke") && !message.includes("tauri")) {
            setError(message);
          }
        }
      }
    }
    void tick();
    const timer = window.setInterval(() => void tick(), 1000);
    return () => {
      stop = true;
      window.clearInterval(timer);
    };
  }, [props.profileId]);

  useEffect(() => {
    let unlisten: (() => void) | undefined;
    if (!props.canWrite) {
      return;
    }
    void getCurrentWebview()
      .onDragDropEvent((event) => {
        if (event.payload.type !== "drop") {
          return;
        }
        for (const path of event.payload.paths) {
          void enqueue(path);
        }
      })
      .then((fn) => {
        unlisten = fn;
      })
      .catch(() => {
        unlisten = undefined;
      });
    return () => {
      unlisten?.();
    };
    // enqueue closes over the current prefix; a new listener is registered when it changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [props.profileId, props.bucket, props.prefix, props.canWrite, follow]);

  async function enqueue(path: string) {
    setError("");
    try {
      await transfer(props.profileId, "transfer.enqueue", {
        direction: "upload",
        bucket: props.bucket,
        key: props.prefix,
        local_path: path,
        follow_symlinks: follow,
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  async function download() {
    if (!props.selectedKey || !destPath.trim()) {
      return;
    }
    const key = props.selectedFolder && !props.selectedKey.endsWith("/") ? `${props.selectedKey}` : props.selectedKey;
    setError("");
    try {
      await transfer(props.profileId, "transfer.enqueue", {
        direction: "download",
        bucket: props.bucket,
        key,
        dest_path: destPath.trim(),
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  async function act(method: "transfer.cancel" | "transfer.resolve", id: string, extra: Record<string, unknown> = {}) {
    setError("");
    try {
      await transfer(props.profileId, method, { transfer_id: id, ...extra });
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  async function toggleDetails(id: string) {
    if (detailId === id) {
      setDetailId("");
      setDetail(null);
      return;
    }
    setError("");
    try {
      const result = await transfer(props.profileId, "transfer.files", { transfer_id: id });
      setDetailId(id);
      setDetail(result.detail ?? null);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  function explain(task: TransferTask): string {
    const key = task.message ? messages[task.message] : "";
    if (key) {
      return t(key);
    }
    if (task.state === "waiting_network") {
      return t("waitingNetwork");
    }
    return task.message || task.state;
  }

  return (
    <section className="space-y-3 rounded-md border p-3">
      <h3 className="text-sm font-medium">{t("transfers")}</h3>
      <p className="text-sm text-muted-foreground">{t("dragHint")}</p>
      <p className="text-sm text-muted-foreground">{t("dragOut")}</p>
      {props.canWrite ? (
        <div className="flex flex-wrap items-center gap-2">
          <input className="min-w-64 rounded-md border bg-transparent px-2 py-1" placeholder={t("localPath")} value={localPath} onChange={(event) => setLocalPath(event.target.value)} />
          <Button type="button" variant="outline" disabled={!localPath.trim()} onClick={() => void enqueue(localPath.trim())}>
            {t("upload")}
          </Button>
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={follow} onChange={(event) => setFollow(event.target.checked)} />
            {t("followLinks")}
          </label>
        </div>
      ) : null}
      <div className="flex flex-wrap items-center gap-2">
        <label className="flex items-center gap-2 text-sm">
          <span>{t("destFolder")}</span>
          <input className="min-w-64 rounded-md border bg-transparent px-2 py-1" aria-label={t("destFolder")} placeholder={t("destFolderHint")} value={destPath} onChange={(event) => setDestPath(event.target.value)} />
        </label>
        <Button type="button" variant="outline" disabled={!props.selectedKey || !destPath.trim()} onClick={() => void download()}>
          {t("download")}
        </Button>
      </div>
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
      {tasks.length === 0 ? <p className="text-sm text-muted-foreground">{t("noTransfers")}</p> : null}
      <ul className="space-y-2 text-sm">
        {tasks.map((task) => (
          <li key={task.id} className="space-y-1 rounded-md border px-3 py-2">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <span className="break-all">
                {task.direction} · {task.key || task.local_path} · {explain(task)}
                {task.size ? ` · ${formatBytes(task.bytes_done ?? 0)} / ${formatBytes(task.size)}` : task.bytes_done ? ` · ${formatBytes(task.bytes_done)}` : ""}
                {task.files_total && task.files_total > 1 ? ` · ${task.files_done ?? 0}/${task.files_total}` : ""}
              </span>
              <span className="flex gap-2">
                <Button type="button" variant="outline" onClick={() => void toggleDetails(task.id)}>
                  {detailId === task.id ? t("hideDetails") : t("details")}
                </Button>
                {task.state === "queued" || task.state === "running" || task.state === "waiting_network" || task.state === "conflict" ? (
                  <Button type="button" variant="outline" onClick={() => void act("transfer.cancel", task.id)}>
                    {t("cancel")}
                  </Button>
                ) : null}
              </span>
            </div>
            {task.state === "conflict" ? (
              <div className="flex flex-wrap gap-2">
                <span className="break-all">{t("conflict")}: {task.pending_local}</span>
                <Button type="button" variant="outline" onClick={() => void act("transfer.resolve", task.id, { choice: "replace", apply_all: applyAll })}>
                  {t("replace")}
                </Button>
                <Button type="button" variant="outline" onClick={() => void act("transfer.resolve", task.id, { choice: "skip", apply_all: applyAll })}>
                  {t("skip")}
                </Button>
                <Button type="button" variant="outline" onClick={() => void act("transfer.resolve", task.id, { choice: "rename", apply_all: applyAll })}>
                  {t("renameAll")}
                </Button>
                <label className="flex items-center gap-2">
                  <input type="checkbox" checked={applyAll} onChange={(event) => setApplyAll(event.target.checked)} />
                  {t("applyAll")}
                </label>
              </div>
            ) : null}
            {detailId === task.id && detail ? (
              <div className="space-y-1 text-muted-foreground">
                {detail.report && detail.report.length > 0 ? (
                  <ul>
                    <li>{t("report")}</li>
                    {detail.report.map((note, index) => (
                      <li key={`${note.from}-${index}`} className="break-all">
                        {note.action} {note.from}
                        {note.to ? ` → ${note.to}` : ""} ({note.reason})
                      </li>
                    ))}
                  </ul>
                ) : null}
                {detail.files && detail.files.length > 0 ? (
                  <ul>
                    {detail.files.slice(0, 200).map((file, index) => (
                      <li key={`${file.key ?? file.local ?? "file"}-${index}`} className="break-all">
                        {file.key || file.local} · {file.done ? t("fileDone") : t("filePending")}
                        {file.size ? ` · ${formatBytes(file.size)}` : ""}
                      </li>
                    ))}
                    {detail.files.length > 200 ? <li>{t("filesShown", { shown: 200, total: detail.files.length })}</li> : null}
                  </ul>
                ) : null}
              </div>
            ) : null}
          </li>
        ))}
      </ul>
    </section>
  );
}
