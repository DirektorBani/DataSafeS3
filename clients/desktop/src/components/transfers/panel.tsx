import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { getCurrentWebview } from "@tauri-apps/api/webview";
import { Button } from "@datasafe/ui/ui/button";
import { Input } from "@datasafe/ui/ui/input";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@datasafe/ui/ui/card";
import { cn } from "@datasafe/ui/cn";
import { transfer, type TransferDetail, type TransferTask } from "../../desktop";

function TransferMeter(props: { done: number; size: number; label: string; error?: boolean }) {
  const pct = props.size > 0 ? Math.min(100, Math.round((props.done / props.size) * 100)) : 0;
  const announced = useAnnounced(pct);
  return (
    <div className="h-2 overflow-hidden rounded-full bg-muted" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={announced} aria-label={props.label}>
      <div className={cn("h-full transition-all", props.error ? "bg-destructive" : "bg-primary")} style={{ width: `${pct}%` }} />
    </div>
  );
}

function useAnnounced(value: number) {
  const [shown, setShown] = useState(value);
  const at = useRef(0);
  useEffect(() => {
    const now = Date.now();
    if (at.current === 0 || now - at.current >= 5000 || value <= 0 || value >= 100) {
      at.current = now;
      setShown(value);
    }
  }, [value]);
  return shown;
}

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
  "multipart.upload_not_found": "uploadNotFound",
  etag_mismatch: "etagMismatch",
  presign_unavailable: "presignUnavailable",
  resume_after_restart: "resumeAfterRestart",
  paused_signed_out: "pausedSignedOut",
  paused_user: "pausedUser",
  "tls.untrusted": "tlsUntrusted",
  "tls.failed": "tlsFailed",
  "request.invalid": "codeRequestInvalid",
  "local.file_missing": "fileMissing",
  "auth.unauthorized": "codeAuth",
  "auth.session_expired": "sessionExpired",
  "access.forbidden": "forbidden",
  "resource.not_found": "codeNotFound",
  internal: "codeInternal",
  rate_limited: "codeRate",
};

function pathLike(text: string): boolean {
  return /[A-Za-z]:\\/.test(text) || text.includes("\\\\") || /\bmkdir\b/i.test(text);
}

function explain(task: TransferTask, t: (key: string, options?: { code: string }) => string): string {
  const code = (task.message || "").trim();
  const key = code ? messages[code] : "";
  if (key) {
    const text = t(key, { code });
    if (task.state === "error" && code && !text.includes(code)) {
      return `${text} (${code})`;
    }
    return text;
  }
  if (pathLike(code)) {
    return t("codeRequestInvalid", { code: "request.invalid" });
  }
  if (task.state === "waiting_network") {
    return t("waitingNetwork");
  }
  if (task.state === "interrupted") {
    return t("resumeAfterRestart");
  }
  if (task.state === "error" && code && /^[A-Za-z0-9_.-]+$/.test(code)) {
    return t("codeGeneric", { code });
  }
  if (task.state === "error") {
    return t("codeGeneric", { code: "error" });
  }
  return task.state;
}

function transferSummary(tasks: TransferTask[], t: (key: string) => string): string {
  if (tasks.some((task) => task.message === "paused_user")) {
    return t("pausedUser");
  }
  if (tasks.some((task) => task.message === "paused_signed_out")) {
    return t("pausedSignedOut");
  }
  if (tasks.some((task) => task.state === "interrupted" || task.message === "resume_after_restart")) {
    return t("resumeAfterRestart");
  }
  if (tasks.some((task) => task.state === "error")) {
    return t("transferFailed");
  }
  if (tasks.some((task) => task.state === "running" || task.state === "queued" || task.state === "waiting_network" || task.state === "conflict")) {
    return t("transferActive");
  }
  return t("transferIdle");
}

function canCancel(state: string): boolean {
  return state === "error" || state === "queued" || state === "running" || state === "waiting_network" || state === "conflict";
}

export function TransferPanel(props: {
  profileId: string;
  bucket: string;
  prefix: string;
  canWrite: boolean;
  offline?: boolean;
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
    void (async () => {
      try {
        await transfer(props.profileId, "transfer.resume");
      } catch {
        // The list still shows a transfer that could not continue.
      }
      if (!stop) {
        await tick();
      }
    })();
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
    let webview: ReturnType<typeof getCurrentWebview>;
    try {
      webview = getCurrentWebview();
    } catch {
      return;
    }
    void webview
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

  async function act(method: "transfer.cancel" | "transfer.resolve" | "transfer.retry" | "transfer.resume_user", id: string, extra: Record<string, unknown> = {}) {
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

  const visible = tasks.filter((task) => task.state !== "cancelled");

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("transfers")}</CardTitle>
        <CardDescription>
          {t("dragHint")} {t("dragOut")}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
      {props.canWrite ? (
        <div className="flex flex-wrap items-center gap-2">
          <Input className="max-w-sm" placeholder={t("localPath")} value={localPath} onChange={(event) => setLocalPath(event.target.value)} autoComplete="off" />
          <Button type="button" variant="outline" disabled={!localPath.trim()} onClick={() => void enqueue(localPath.trim())}>
            {t("bucketDetail:actions.upload")}
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
          <Input className="max-w-sm" aria-label={t("destFolder")} placeholder={t("destFolderHint")} value={destPath} onChange={(event) => setDestPath(event.target.value)} autoComplete="off" />
        </label>
        <Button type="button" variant="outline" disabled={props.offline || !props.selectedKey || !destPath.trim()} onClick={() => void download()}>
          {t("download")}
        </Button>
      </div>
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
      {visible.length === 0 ? <p className="text-sm text-muted-foreground">{t("noTransfers")}</p> : null}
      <ul className="space-y-2 text-sm">
        {visible.map((task) => (
          <li key={task.id} className="space-y-2 rounded-md border p-3 text-sm">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <span className="break-all">
                {task.direction} · {task.key || task.local_path} · {explain(task, t)}
                {task.size ? ` · ${formatBytes(task.bytes_done ?? 0)} / ${formatBytes(task.size)}` : task.bytes_done ? ` · ${formatBytes(task.bytes_done)}` : ""}
                {task.files_total && task.files_total > 1 ? ` · ${task.files_done ?? 0}/${task.files_total}` : ""}
              </span>
              <span className="flex gap-2">
                <Button type="button" variant="outline" onClick={() => void toggleDetails(task.id)}>
                  {detailId === task.id ? t("hideDetails") : t("details")}
                </Button>
                {task.message === "paused_user" ? (
                  <Button type="button" variant="outline" onClick={() => void act("transfer.resume_user", task.id)}>
                    {t("resume")}
                  </Button>
                ) : null}
                {task.state === "error" ? (
                  <Button type="button" variant="outline" onClick={() => void act("transfer.retry", task.id)}>
                    {t("retry")}
                  </Button>
                ) : null}
                {canCancel(task.state) ? (
                  <Button type="button" variant="outline" onClick={() => void act("transfer.cancel", task.id)}>
                    {t("cancel")}
                  </Button>
                ) : null}
              </span>
            </div>
            {task.size ? <TransferMeter done={task.bytes_done ?? 0} size={task.size} label={t("progress")} error={task.state === "error"} /> : null}
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
      </CardContent>
    </Card>
  );
}

export function ResumeProgress(props: { profileId: string; onSummary?: (text: string) => void; onSessionExpired?: () => void }) {
  const { t } = useTranslation();
  const [tasks, setTasks] = useState<TransferTask[]>([]);
  const onSummary = props.onSummary;
  const onSessionExpired = props.onSessionExpired;
  const toldSession = useRef(false);

  useEffect(() => {
    onSummary?.(transferSummary(tasks, t));
  }, [tasks, onSummary, t]);

  useEffect(() => {
    if (toldSession.current || !onSessionExpired) {
      return;
    }
    if (tasks.some((task) => task.message === "auth.session_expired")) {
      toldSession.current = true;
      onSessionExpired();
    }
  }, [onSessionExpired, tasks]);

  useEffect(() => {
    let stop = false;
    async function tick() {
      try {
        const result = await transfer(props.profileId, "transfer.list");
        if (stop) {
          return;
        }
        setTasks(
          (result.transfers ?? []).filter((task) => {
            if (task.profile_id && task.profile_id !== props.profileId) {
              return false;
            }
            return task.state !== "done" && task.state !== "cancelled";
          }),
        );
      } catch {
        // The start screen stays quiet when the agent is not there.
      }
    }
    void tick();
    const timer = window.setInterval(() => void tick(), 1000);
    return () => {
      stop = true;
      window.clearInterval(timer);
    };
  }, [props.profileId]);

  if (tasks.length === 0) {
    return null;
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("transfers")}</CardTitle>
      </CardHeader>
      <CardContent>
      <ul className="space-y-2 text-sm">
        {tasks.map((task) => (
          <li key={task.id} className="space-y-2 rounded-md border p-3">
            <span className="break-all">
              {task.direction} · {task.key || task.local_path} · {explain(task, t)}
              {task.size ? ` · ${formatBytes(task.bytes_done ?? 0)} / ${formatBytes(task.size)}` : task.bytes_done ? ` · ${formatBytes(task.bytes_done)}` : ""}
            </span>
            <span className="flex gap-2">
              {task.message === "paused_user" ? (
                <Button type="button" size="sm" variant="outline" onClick={() => void transfer(props.profileId, "transfer.resume_user", { transfer_id: task.id })}>
                  {t("resume")}
                </Button>
              ) : null}
              {task.state === "error" ? (
                <Button type="button" size="sm" variant="outline" onClick={() => void transfer(props.profileId, "transfer.retry", { transfer_id: task.id })}>
                  {t("retry")}
                </Button>
              ) : null}
              {canCancel(task.state) ? (
                <Button type="button" size="sm" variant="outline" onClick={() => void transfer(props.profileId, "transfer.cancel", { transfer_id: task.id })}>
                  {t("cancel")}
                </Button>
              ) : null}
            </span>
            {task.size ? <TransferMeter done={task.bytes_done ?? 0} size={task.size} label={t("progress")} /> : null}
          </li>
        ))}
      </ul>
      </CardContent>
    </Card>
  );
}
