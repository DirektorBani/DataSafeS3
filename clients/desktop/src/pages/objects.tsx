import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { ChevronRight, File, Folder, FolderPlus, Trash2 } from "lucide-react";
import { Button } from "@datasafe/ui/ui/button";
import { Input } from "@datasafe/ui/ui/input";
import { Card, CardContent, CardHeader, CardTitle } from "@datasafe/ui/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@datasafe/ui/ui/tabs";
import { PageHeader } from "@datasafe/ui/page-header";
import { cn } from "@datasafe/ui/cn";
import { formatBytes, formatDate, type ByteUnits } from "@datasafe/ui/format";
import { ObjectInfo, type ListedObject } from "../components/object-info";
import { SelectionList, SelectionOption, objectColumns } from "../components/selection-list";
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

function listedSize(bytes: number | undefined, units: ByteUnits): string {
  if (typeof bytes !== "number" || !Number.isFinite(bytes) || bytes < 0) {
    return "";
  }
  return formatBytes(bytes, units);
}

function listedDate(iso: string, locale: string): string {
  if (!iso) {
    return "";
  }
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  return formatDate(iso, locale);
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

function isTransport(message: string): boolean {
  const text = message.toLowerCase();
  return (
    text.includes("connection refused") ||
    text.includes("connectex") ||
    text.includes("actively refused") ||
    text.includes("dial tcp") ||
    text.includes("no such host") ||
    text.includes("i/o timeout") ||
    text.includes("timeout") ||
    text.includes("network is unreachable") ||
    text.includes("connection reset") ||
    text.includes("wsarecv") ||
    text.includes("eof")
  );
}

export function ObjectBrowser(props: {
  profileId: string;
  bucket: string;
  canWrite: boolean;
  offline?: boolean;
  encryption: string;
  features?: Record<string, number>;
  onError: (message: string) => void;
  confirmDangerous: (method: string, params: Record<string, unknown>) => Promise<BrowseResult>;
  headerActions?: ReactNode;
}) {
  const { t, i18n } = useTranslation(["shell", "bucketDetail", "common"]);
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
  const [actionKey, setActionKey] = useState("");
  const [renaming, setRenaming] = useState("");
  const [folderName, setFolderName] = useState("");
  const [trash, setTrash] = useState<Array<Record<string, unknown>>>([]);
  const [showTrash, setShowTrash] = useState(false);
  const [selectKey, setSelectKey] = useState("");
  const [listOffline, setListOffline] = useState(false);
  const [savedAt, setSavedAt] = useState(0);
  const [folderOffline, setFolderOffline] = useState(false);
  const [listError, setListError] = useState("");
  const [focusNonce, setFocusNonce] = useState(0);
  const rowRefs = useRef<Array<HTMLDivElement | null>>([]);
  const emptyRef = useRef<HTMLDivElement | null>(null);
  const actionRef = useRef<HTMLSpanElement | null>(null);
  const confirmDeleteRef = useRef<HTMLButtonElement | null>(null);
  const focusAfterDelete = useRef("");
  const listOfflineRef = useRef(false);
  const keyboardOpen = useRef(false);
  if (props.offline) {
    listOfflineRef.current = true;
  }

  const load = useCallback(
    async (nextPrefix: string, startAfter: string, recordFirst: boolean) => {
      const focusAfter = keyboardOpen.current;
      keyboardOpen.current = false;
      const miss = (message: string) => {
        setRows([]);
        setSelected(0);
        setTruncated(false);
        setMarker("");
        const offlineMiss = listOfflineRef.current || Boolean(props.offline) || isTransport(message);
        if (offlineMiss) {
          setFolderOffline(true);
          setListError("");
        } else {
          setFolderOffline(false);
          setListError(message);
          onError(message);
        }
        if (focusAfter) {
          setFocusNonce((n) => n + 1);
        }
      };
      const started = performance.now();
      let result: BrowseResult;
      try {
        result = await browse(profileId, "object.list", {
          bucket,
          prefix: nextPrefix,
          start_after: startAfter,
          max_keys: 100,
        });
      } catch (err) {
        miss(err instanceof Error ? err.message : String(err));
        return;
      }
      const message = problem(result);
      if (message) {
        miss(message);
        return;
      }
      const cached = Boolean(result.cached);
      setListOffline(cached);
      listOfflineRef.current = cached || Boolean(props.offline);
      setSavedAt(typeof result.cached_at === "number" ? result.cached_at : 0);
      setFolderOffline(false);
      setListError("");
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
      if (focusAfter) {
        setFocusNonce((n) => n + 1);
      }
    },
    [bucket, onError, profileId, props.offline],
  );
  const loadRef = useRef(load);
  loadRef.current = load;

  useEffect(() => {
    // A language change rebuilds load. The open folder stays. A new bucket
    // or profile still starts at the bucket root.
    setPrefix("");
    setMarkers([""]);
    setFirstPageMs(null);
    setShowTrash(false);
    setTrash([]);
    setSelectKey("");
    void loadRef.current("", "", true);
  }, [bucket, profileId]);

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

  useEffect(() => {
    if (focusNonce === 0) {
      return;
    }
    const node = visible.length > 0 ? rowRefs.current[0] : emptyRef.current;
    node?.focus();
  }, [focusNonce, visible]);

  useEffect(() => {
    const pending = focusAfterDelete.current;
    if (!pending) {
      return;
    }
    focusAfterDelete.current = "";
    if (pending === "empty") {
      emptyRef.current?.focus();
      return;
    }
    const index = visible.findIndex((row) => row.key === pending);
    if (index < 0) {
      return;
    }
    setSelected(index);
    rowRefs.current[index]?.focus();
  }, [visible]);

  useEffect(() => {
    if (!actionKey) {
      return;
    }
    actionRef.current?.focus();
  }, [actionKey]);

  useEffect(() => {
    if (!confirmKey) {
      return;
    }
    confirmDeleteRef.current?.focus();
  }, [confirmKey]);

  function openPrefix(next: string, fromKeyboard: boolean) {
    if (fromKeyboard) {
      keyboardOpen.current = true;
    }
    setPrefix(next);
    setMarkers([""]);
    void load(next, "", true);
  }

  async function remove(key: string, folder: boolean) {
    const index = visible.findIndex((row) => row.key === key);
    const rest = visible.filter((row) => row.key !== key);
    const neighbor = rest.length === 0 ? "empty" : rest[Math.min(Math.max(index, 0), rest.length - 1)].key;
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
    focusAfterDelete.current = neighbor;
    setConfirmKey("");
    setActionKey("");
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
    const result =
      method === "trash.purge"
        ? await props.confirmDangerous(method, { trash_id: id })
        : await browse(props.profileId, method, { trash_id: id });
    if (result.code === "cancelled" || result.mfa_setup) {
      return;
    }
    const message = problem(result);
    if (message) {
      props.onError(message);
      return;
    }
    await openTrash();
  }

  function onRowKey(event: KeyboardEvent<HTMLDivElement>, index: number) {
    if (event.target !== event.currentTarget) {
      return;
    }
    const last = Math.max(visible.length - 1, 0);
    const go = (next: number) => {
      const clamped = Math.max(0, Math.min(next, last));
      setSelected(clamped);
      rowRefs.current[clamped]?.focus();
    };
    const row = visible[index];
    if (event.key === "ArrowDown") {
      event.preventDefault();
      go(index + 1);
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      go(index - 1);
    } else if (event.key === "Enter" && row?.kind === "folder") {
      event.preventDefault();
      openPrefix(row.key, true);
    } else if (event.key === "Delete" && canWrite && row) {
      event.preventDefault();
      setConfirmKey(row.key);
    } else if (event.key === "F2" && canWrite && row?.kind === "object") {
      event.preventDefault();
      setRenaming(row.name);
    } else if (event.key === "Home") {
      event.preventDefault();
      go(0);
    } else if (event.key === "End") {
      event.preventDefault();
      go(last);
    } else if (event.key === "PageUp") {
      event.preventDefault();
      go(index - 10);
    } else if (event.key === "PageDown") {
      event.preventDefault();
      go(index + 10);
    } else if (event.key === "Backspace" && prefix) {
      event.preventDefault();
      openPrefix(parentPrefix(prefix), true);
    } else if (event.key === "Tab" && !event.shiftKey && canWrite && row) {
      event.preventDefault();
      setActionKey(row.key);
    }
  }

  const offline = Boolean(props.offline || listOffline || folderOffline);
  const canWrite = props.canWrite && !offline;
  const units: ByteUnits = {
    bytes: t("common:units.bytes"),
    kilobytes: t("common:units.kilobytes"),
    megabytes: t("common:units.megabytes"),
    gigabytes: t("common:units.gigabytes"),
    terabytes: t("common:units.terabytes"),
  };
  const savedLabel = savedAt > 0 ? formatDate(new Date(savedAt * 1000).toISOString(), i18n.language) : "";
  const crumbItems = [{ label: t("bucketDetail:breadcrumb.root"), path: "" }];
  if (prefix) {
    let acc = "";
    for (const part of prefix.replace(/\/$/, "").split("/")) {
      acc += `${part}/`;
      crumbItems.push({ label: part, path: acc });
    }
  }

  return (
    <section className="space-y-4">
      <PageHeader
        title={props.bucket}
        description={t("bucketDetail:description")}
        badge={props.encryption ? <span className="text-xs text-muted-foreground">{props.encryption}</span> : undefined}
        actions={props.headerActions}
      />
      {listOffline && !props.offline ? (
        <p className="text-sm text-muted-foreground" role="status">
          {t("offlineSaved", { time: savedLabel })}
        </p>
      ) : null}
      {firstPageMs !== null ? <p className="text-xs text-muted-foreground">{t("firstPage", { ms: firstPageMs })}</p> : null}
      {!canWrite && !offline ? <p className="text-sm text-muted-foreground">{t("noWrite")}</p> : null}
      <Tabs
        value={showTrash ? "trash" : "objects"}
        onValueChange={(next) => {
          if (next === "trash") {
            void openTrash();
          } else {
            setShowTrash(false);
          }
        }}
      >
        <TabsList>
          <TabsTrigger value="objects">{t("bucketDetail:tabs.objects")}</TabsTrigger>
          <TabsTrigger value="trash">{t("bucketDetail:tabs.trash")}</TabsTrigger>
        </TabsList>
        <TabsContent value="objects" className="space-y-4">
          <div className="flex flex-wrap items-center gap-2 text-sm">
            {crumbItems.map((crumb, index) => (
              <span key={crumb.path || "root"} className="inline-flex items-center gap-1">
                {index > 0 ? <ChevronRight className="h-3 w-3 text-muted-foreground" aria-hidden="true" /> : null}
                <button
                  type="button"
                  className={cn("hover:text-primary", index === crumbItems.length - 1 ? "font-medium text-foreground" : "text-muted-foreground")}
                  onClick={() => openPrefix(crumb.path, false)}
                >
                  {crumb.label}
                </button>
              </span>
            ))}
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Input className="max-w-xs" placeholder={t("filter")} value={filter} onChange={(event) => setFilter(event.target.value)} />
            <label className="flex items-center gap-2 text-sm text-muted-foreground">
              {t("sort")}
              <select className="h-10 rounded-md border border-input bg-background px-3 text-sm" value={sort} onChange={(event) => setSort(event.target.value as SortKey)}>
                <option value="name">{t("bucketDetail:table.name")}</option>
                <option value="size">{t("bucketDetail:table.size")}</option>
                <option value="modified">{t("bucketDetail:table.modified")}</option>
              </select>
            </label>
            {canWrite ? (
              <>
                <Input className="max-w-xs" placeholder={t("bucketDetail:actions.newFolder")} value={folderName} onChange={(event) => setFolderName(event.target.value)} />
                <Button type="button" variant="outline" size="sm" onClick={() => void createFolder()}>
                  <FolderPlus className="h-4 w-4" aria-hidden="true" />
                  {t("bucketDetail:actions.newFolder")}
                </Button>
              </>
            ) : null}
          </div>
          <div className="grid gap-4 lg:grid-cols-[1fr_280px]">
            <Card>
              <CardContent className="p-0">
                <div className="max-h-[28rem] overflow-auto">
                  <SelectionList
                    label={t("objects")}
                    idPrefix="objects"
                    template={objectColumns}
                    activeId={visible.length > 0 ? `object-row-${Math.min(selected, visible.length - 1)}` : undefined}
                    columns={[t("bucketDetail:table.name"), t("bucketDetail:table.size"), t("bucketDetail:table.modified"), ""]}
                    empty={
                      visible.length === 0 ? (
                        <div id="object-row-empty" role="status" tabIndex={0} ref={emptyRef} className="p-8 text-center text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring">
                          {folderOffline ? t("folderOffline") : listError ? listError : t("bucketDetail:emptyFolder")}
                        </div>
                      ) : undefined
                    }
                  >
                    {visible.map((row, index) => {
                      const active = index === Math.min(selected, Math.max(visible.length - 1, 0));
                      const nameId = `object-name-${index}`;
                      const sizeId = `object-size-${index}`;
                      const dateId = `object-date-${index}`;
                      const sizeText = row.kind === "object" ? listedSize(row.object?.size, units) : "";
                      const dateText = row.kind === "object" ? listedDate(row.object?.last_modified || "", i18n.language) : "";
                      const labelledBy = [nameId, sizeText ? sizeId : "", dateText ? dateId : ""].filter(Boolean).join(" ");
                      return (
                        <SelectionOption
                          key={row.key}
                          id={`object-row-${index}`}
                          selected={active}
                          tabIndex={active ? 0 : -1}
                          labelledBy={labelledBy}
                          describedBy="objects-col-0 objects-col-1 objects-col-2"
                          keyShortcuts={canWrite ? "Delete" : undefined}
                          optionRef={(node) => {
                            rowRefs.current[index] = node;
                          }}
                          onClick={() => {
                            setSelected(index);
                            if (row.kind === "folder") {
                              openPrefix(row.key, false);
                            }
                          }}
                          onKeyDown={(event) => onRowKey(event, index)}
                        >
                          {/* The trailing space is a word boundary. Adjacent cells have no text node between them, so without it the visible text is "readme.txt12 BOct" while the name is "readme.txt 12 B Oct". */}
                          <span id={nameId} data-column="name" className="inline-flex items-center gap-2 p-3 font-mono">
                            {row.kind === "folder" ? <Folder className="h-4 w-4 text-amber-500" aria-hidden="true" /> : <File className="h-4 w-4 text-muted-foreground" aria-hidden="true" />}
                            {row.name}{" "}
                          </span>
                          <span id={sizeText ? sizeId : undefined} data-column="size" className="whitespace-nowrap p-3">{sizeText}{sizeText ? " " : ""}</span>
                          <span id={dateText ? dateId : undefined} data-column="modified" className="whitespace-nowrap p-3">{dateText}{dateText ? " " : ""}</span>
                          <span className="p-3">
                            {canWrite ? (
                              <span
                                role="button"
                                tabIndex={actionKey === row.key ? 0 : undefined}
                                ref={actionKey === row.key ? actionRef : undefined}
                                className="inline-flex h-7 w-7 items-center justify-center rounded-md text-destructive hover:bg-accent outline-none focus-visible:ring-2 focus-visible:ring-ring"
                                aria-label={t("deleteObject", { name: row.name })}
                                aria-keyshortcuts="Delete"
                                onClick={(event) => {
                                  event.stopPropagation();
                                  setSelected(index);
                                  setActionKey("");
                                  setConfirmKey(row.key);
                                }}
                                onBlur={() => {
                                  setActionKey((current) => (current === row.key ? "" : current));
                                }}
                                onKeyDown={(event) => {
                                  if (event.key === "Tab" && event.shiftKey) {
                                    event.preventDefault();
                                    setActionKey("");
                                    rowRefs.current[index]?.focus();
                                  } else if (event.key === "Enter" || event.key === " ") {
                                    event.preventDefault();
                                    setSelected(index);
                                    setActionKey("");
                                    setConfirmKey(row.key);
                                  }
                                }}
                              >
                                <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
                              </span>
                            ) : null}
                          </span>
                        </SelectionOption>
                      );
                    })}
                  </SelectionList>
                </div>
                {markers.length > 1 || truncated ? (
                  <div className="flex justify-center gap-2 border-t p-3">
                    {markers.length > 1 ? (
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        onClick={() => {
                          const next = markers.slice(0, -1);
                          setMarkers(next);
                          void load(prefix, next[next.length - 1] || "", false);
                        }}
                      >
                        {t("common:back")}
                      </Button>
                    ) : null}
                    {truncated ? (
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        onClick={() => {
                          setMarkers((stack) => [...stack, marker]);
                          void load(prefix, marker, false);
                        }}
                      >
                        {t("bucketDetail:loadMore")}
                      </Button>
                    ) : null}
                  </div>
                ) : null}
              </CardContent>
            </Card>
            {current?.kind === "object" && current.object ? (
              <ObjectInfo
                profileId={props.profileId}
                bucket={props.bucket}
                object={current.object}
                canWrite={canWrite}
                offline={offline}
                features={props.features}
                confirmDangerous={props.confirmDangerous}
                onChanged={() => void load(prefix, markers[markers.length - 1] || "", false)}
                onError={props.onError}
              />
            ) : (
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm">{t("bucketDetail:metadata.title")}</CardTitle>
                </CardHeader>
                <CardContent className="text-sm text-muted-foreground">{t("bucketDetail:metadata.selectObject")}</CardContent>
              </Card>
            )}
          </div>
          {confirmKey && canWrite ? (
            <div role="group" aria-label={t("confirmDeleteObject", { name: displayName(confirmKey) })} className="flex flex-wrap items-center gap-2 text-sm">
              <span className="font-mono">{confirmKey}</span>
              <Button ref={confirmDeleteRef} type="button" variant="destructive" size="sm" aria-label={t("confirmDeleteObject", { name: displayName(confirmKey) })} onClick={() => void remove(confirmKey, confirmKey.endsWith("/"))}>
                {t("common:delete")}
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => {
                  const key = confirmKey;
                  setConfirmKey("");
                  const index = visible.findIndex((row) => row.key === key);
                  if (index >= 0) {
                    rowRefs.current[index]?.focus();
                  }
                }}
              >
                {t("common:cancel")}
              </Button>
            </div>
          ) : null}
          {renaming && current?.kind === "object" && canWrite ? (
            <div className="flex flex-wrap items-center gap-2">
              <Input className="max-w-xs" value={renaming} onChange={(event) => setRenaming(event.target.value)} />
              <Button type="button" size="sm" onClick={() => void rename(current)}>
                {t("rename")}
              </Button>
            </div>
          ) : null}
        </TabsContent>
        <TabsContent value="trash">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">{t("bucketDetail:tabs.trash")}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2 text-sm">
              {trash.length === 0 ? <p className="text-muted-foreground">{t("emptyTrash")}</p> : null}
              {trash.map((item) => {
                const id = String(item.id || "");
                return (
                  <div key={id} className="flex flex-wrap items-center justify-between gap-2 border-b py-2">
                    <span className="break-all font-mono">{String(item.original_key || id)}</span>
                    {canWrite ? (
                      <span className="flex gap-2">
                        <Button type="button" variant="outline" size="sm" onClick={() => void trashAction("trash.restore", id)}>
                          {t("restoreTrash")}
                        </Button>
                        <Button type="button" variant="outline" size="sm" onClick={() => void trashAction("trash.purge", id)}>
                          {t("purgeTrash")}
                        </Button>
                      </span>
                    ) : null}
                  </div>
                );
              })}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
      <TransferPanel profileId={props.profileId} bucket={props.bucket} prefix={prefix} canWrite={canWrite} offline={offline} selectedKey={current?.key || ""} selectedFolder={current?.kind === "folder"} />
    </section>
  );
}
