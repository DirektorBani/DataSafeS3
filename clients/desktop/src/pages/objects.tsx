import { useCallback, useEffect, useMemo, useState, type KeyboardEvent } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@datasafe/ui/ui/button";
import { ObjectInfo, type ListedObject } from "../components/object-info";
import { TransferPanel } from "../components/transfers/panel";
import { browse, type BrowseResult } from "../desktop";

type SortKey = "name" | "size" | "modified";

type Row = {
  kind: "folder" | "object";
  name: string;
  key: string;
  object?: ListedObject;
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

function displayName(key: string): string {
  const trimmed = key.endsWith("/") ? key.slice(0, -1) : key;
  const cut = trimmed.lastIndexOf("/");
  return cut >= 0 ? trimmed.slice(cut + 1) : trimmed;
}

function parentPrefix(prefix: string): string {
  const trimmed = prefix.endsWith("/") ? prefix.slice(0, -1) : prefix;
  const cut = trimmed.lastIndexOf("/");
  return cut >= 0 ? trimmed.slice(0, cut + 1) : "";
}

export function ObjectBrowser(props: {
  profileId: string;
  bucket: string;
  canWrite: boolean;
  encryption: string;
  onError: (message: string) => void;
}) {
  const { t } = useTranslation();
  const { profileId, bucket, onError } = props;
  const [prefix, setPrefix] = useState("");
  const [rows, setRows] = useState<Row[]>([]);
  const [marker, setMarker] = useState("");
  const [truncated, setTruncated] = useState(false);
  const [markers, setMarkers] = useState<string[]>([""]);
  const [filter, setFilter] = useState("");
  const [sort, setSort] = useState<SortKey>("name");
  const [selected, setSelected] = useState(0);
  const [firstPageMs, setFirstPageMs] = useState<number | null>(null);
  const [confirmKey, setConfirmKey] = useState("");
  const [renaming, setRenaming] = useState("");
  const [folderName, setFolderName] = useState("");
  const [trash, setTrash] = useState<Array<Record<string, unknown>>>([]);
  const [showTrash, setShowTrash] = useState(false);
  const [selectKey, setSelectKey] = useState("");

  const load = useCallback(
    async (nextPrefix: string, startAfter: string, recordFirst: boolean) => {
      const started = performance.now();
      const result = await browse(profileId, "object.list", {
        bucket,
        prefix: nextPrefix,
        start_after: startAfter,
        max_keys: 100,
      });
      const message = problem(result);
      if (message) {
        onError(message);
        return;
      }
      if (recordFirst) {
        setFirstPageMs(Math.round(performance.now() - started));
      }
      const folders = Array.isArray(result.body?.folders) ? (result.body?.folders as string[]) : [];
      const objects = Array.isArray(result.body?.objects) ? (result.body?.objects as ListedObject[]) : [];
      const nextRows: Row[] = [
        ...folders.map((key) => ({ kind: "folder" as const, name: displayName(key), key })),
        ...objects
          .filter((item) => item.key && !item.key.endsWith("/"))
          .map((item) => ({ kind: "object" as const, name: displayName(item.key), key: item.key, object: item })),
      ];
      setRows(nextRows);
      setSelected(0);
      setTruncated(result.body?.truncated === true);
      setMarker(typeof result.body?.next_marker === "string" ? result.body.next_marker : "");
    },
    [bucket, onError, profileId],
  );

  useEffect(() => {
    setPrefix("");
    setMarkers([""]);
    setFirstPageMs(null);
    setShowTrash(false);
    setTrash([]);
    setSelectKey("");
    void load("", "", true);
  }, [bucket, load, profileId]);

  const visible = useMemo(() => {
    const needle = filter.trim().toLowerCase();
    const filtered = rows.filter((row) => (needle ? row.name.toLowerCase().includes(needle) : true));
    const folders = filtered.filter((row) => row.kind === "folder");
    const files = filtered.filter((row) => row.kind === "object");
    const byName = (a: Row, b: Row) => a.name.localeCompare(b.name);
    const bySize = (a: Row, b: Row) => (a.object?.size ?? 0) - (b.object?.size ?? 0);
    const byTime = (a: Row, b: Row) => (a.object?.last_modified || "").localeCompare(b.object?.last_modified || "");
    const cmp = sort === "size" ? bySize : sort === "modified" ? byTime : byName;
    return [...folders.sort(byName), ...files.sort(cmp)];
  }, [rows, filter, sort]);

  useEffect(() => {
    if (!selectKey) {
      return;
    }
    const index = visible.findIndex((row) => row.key === selectKey);
    if (index >= 0) {
      setSelected(index);
      setSelectKey("");
    }
  }, [selectKey, visible]);

  const current = visible[selected];

  async function remove(key: string, folder: boolean) {
    const result = await browse(props.profileId, folder ? "folder.delete" : "object.delete", {
      bucket: props.bucket,
      prefix: key,
      key,
      keys: [key],
    });
    const message = problem(result);
    if (message) {
      props.onError(message);
      return;
    }
    setConfirmKey("");
    await load(prefix, markers[markers.length - 1] || "", false);
  }

  async function rename(row: Row) {
    const dest = prefix + renaming;
    const result = await browse(props.profileId, "object.rename", { bucket: props.bucket, key: row.key, dest_key: dest });
    const message = problem(result);
    if (message) {
      props.onError(message);
      return;
    }
    setRenaming("");
    await load(prefix, markers[markers.length - 1] || "", false);
  }

  async function createFolder() {
    const name = prefix + folderName;
    const result = await browse(props.profileId, "folder.create", { bucket: props.bucket, name });
    const message = problem(result);
    if (message) {
      props.onError(message);
      return;
    }
    setFolderName("");
    const key = name.replace(/\/+$/, "") + "/";
    setRows((current) => (current.some((row) => row.key === key) ? current : [...current, { kind: "folder", name: displayName(key), key }]));
    setSelectKey(key);
  }

  async function openTrash() {
    setShowTrash(true);
    const result = await browse(props.profileId, "trash.list", { bucket: props.bucket });
    const message = problem(result);
    if (message) {
      props.onError(message);
      return;
    }
    setTrash(Array.isArray(result.body?.items) ? (result.body.items as Array<Record<string, unknown>>) : []);
  }

  async function trashAction(method: "trash.restore" | "trash.purge", id: string) {
    const result = await browse(props.profileId, method, { trash_id: id });
    const message = problem(result);
    if (message) {
      props.onError(message);
      return;
    }
    await openTrash();
  }

  function onKey(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setSelected((index) => Math.min(index + 1, Math.max(visible.length - 1, 0)));
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      setSelected((index) => Math.max(index - 1, 0));
    } else if (event.key === "Enter" && current?.kind === "folder") {
      event.preventDefault();
      setPrefix(current.key);
      setMarkers([""]);
      void load(current.key, "", true);
    } else if (event.key === "Delete" && props.canWrite && current) {
      event.preventDefault();
      setConfirmKey(current.key);
    } else if (event.key === "F2" && props.canWrite && current?.kind === "object") {
      event.preventDefault();
      setRenaming(current.name);
    } else if (event.key === "Home") {
      event.preventDefault();
      setSelected(0);
    } else if (event.key === "End") {
      event.preventDefault();
      setSelected(Math.max(visible.length - 1, 0));
    } else if (event.key === "PageUp") {
      event.preventDefault();
      setSelected((index) => Math.max(index - 10, 0));
    } else if (event.key === "PageDown") {
      event.preventDefault();
      setSelected((index) => Math.min(index + 10, Math.max(visible.length - 1, 0)));
    } else if (event.key === "Backspace" && prefix) {
      event.preventDefault();
      const next = parentPrefix(prefix);
      setPrefix(next);
      setMarkers([""]);
      void load(next, "", true);
    }
  }

  return (
    <section className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          variant="outline"
          disabled={!prefix}
          onClick={() => {
            const next = parentPrefix(prefix);
            setPrefix(next);
            setMarkers([""]);
            void load(next, "", true);
          }}
        >
          {t("back")}
        </Button>
        <p className="text-sm text-muted-foreground">
          {props.bucket}/{prefix}
          {props.encryption ? ` · ${t("encryption")}: ${props.encryption}` : ""}
          {firstPageMs !== null ? ` · ${t("firstPage", { ms: firstPageMs })}` : ""}
        </p>
        <Button type="button" variant="outline" onClick={() => void openTrash()}>
          {t("trash")}
        </Button>
      </div>
      {!props.canWrite ? <p className="text-sm">{t("noWrite")}</p> : null}
      <div className="flex flex-wrap gap-2">
        <input className="rounded-md border bg-transparent px-2 py-1" placeholder={t("filter")} value={filter} onChange={(event) => setFilter(event.target.value)} />
        <label className="flex items-center gap-2 text-sm">
          {t("sort")}
          <select className="rounded-md border bg-transparent px-2 py-1" value={sort} onChange={(event) => setSort(event.target.value as SortKey)}>
            <option value="name">{t("sortName")}</option>
            <option value="size">{t("sortSize")}</option>
            <option value="modified">{t("sortModified")}</option>
          </select>
        </label>
        {props.canWrite ? (
          <>
            <input className="rounded-md border bg-transparent px-2 py-1" placeholder={t("folder")} value={folderName} onChange={(event) => setFolderName(event.target.value)} />
            <Button type="button" variant="outline" onClick={() => void createFolder()}>
              {t("folder")}
            </Button>
          </>
        ) : null}
      </div>
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_20rem]">
        <div role="listbox" tabIndex={0} aria-label={t("objects")} className="max-h-[28rem] overflow-auto rounded-md border outline-none focus-visible:ring-2" onKeyDown={onKey}>
          {visible.length === 0 ? <p className="p-3 text-sm text-muted-foreground">{t("emptyBucket")}</p> : null}
          {visible.map((row, index) => (
            <div
              key={row.key}
              role="option"
              aria-selected={index === selected}
              className={`flex cursor-pointer items-center justify-between px-3 py-2 text-sm ${index === selected ? "bg-muted" : ""}`}
              onClick={() => {
                setSelected(index);
                if (row.kind === "folder") {
                  setPrefix(row.key);
                  setMarkers([""]);
                  void load(row.key, "", true);
                }
              }}
            >
              <span>{row.kind === "folder" ? `${row.name}/` : row.name}</span>
              <span className="text-muted-foreground">{row.object ? row.object.size ?? 0 : ""}</span>
            </div>
          ))}
        </div>
        {current?.kind === "object" && current.object ? (
          <ObjectInfo profileId={props.profileId} bucket={props.bucket} object={current.object} canWrite={props.canWrite} onChanged={() => void load(prefix, markers[markers.length - 1] || "", false)} onError={props.onError} />
        ) : (
          <aside className="rounded-md border p-3 text-sm text-muted-foreground">{t("info")}</aside>
        )}
      </div>
      <div className="flex flex-wrap gap-2">
        {markers.length > 1 ? (
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              const next = markers.slice(0, -1);
              setMarkers(next);
              void load(prefix, next[next.length - 1] || "", false);
            }}
          >
            {t("back")}
          </Button>
        ) : null}
        {truncated ? (
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setMarkers((stack) => [...stack, marker]);
              void load(prefix, marker, false);
            }}
          >
            {t("loadMore")}
          </Button>
        ) : null}
      </div>
      {confirmKey && props.canWrite ? (
        <div className="flex items-center gap-2 text-sm">
          <span>{confirmKey}</span>
          <Button type="button" onClick={() => void remove(confirmKey, confirmKey.endsWith("/"))}>
            {t("confirmDelete")}
          </Button>
          <Button type="button" variant="outline" onClick={() => setConfirmKey("")}>
            {t("cancel")}
          </Button>
        </div>
      ) : null}
      {renaming && current?.kind === "object" && props.canWrite ? (
        <div className="flex items-center gap-2">
          <input className="rounded-md border bg-transparent px-2 py-1" value={renaming} onChange={(event) => setRenaming(event.target.value)} />
          <Button type="button" onClick={() => void rename(current)}>
            {t("rename")}
          </Button>
        </div>
      ) : null}
      {showTrash ? (
        <div className="space-y-2 rounded-md border p-3">
          <h3 className="text-sm font-medium">{t("trash")}</h3>
          {trash.length === 0 ? <p className="text-sm text-muted-foreground">{t("emptyTrash")}</p> : null}
          <ul className="space-y-2 text-sm">
            {trash.map((item) => {
              const id = String(item.id || "");
              return (
                <li key={id} className="flex flex-wrap items-center justify-between gap-2">
                  <span className="break-all">{String(item.original_key || id)}</span>
                  {props.canWrite ? (
                    <span className="flex gap-2">
                      <Button type="button" variant="outline" onClick={() => void trashAction("trash.restore", id)}>
                        {t("restoreTrash")}
                      </Button>
                      <Button type="button" variant="outline" onClick={() => void trashAction("trash.purge", id)}>
                        {t("purgeTrash")}
                      </Button>
                    </span>
                  ) : null}
                </li>
              );
            })}
          </ul>
        </div>
      ) : null}
      <TransferPanel profileId={props.profileId} bucket={props.bucket} prefix={prefix} canWrite={props.canWrite} selectedKey={current?.key || ""} selectedFolder={current?.kind === "folder"} />
    </section>
  );
}
