import { useCallback, useEffect, useMemo, useState, type KeyboardEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { ChevronRight, File, Folder, FolderPlus, Trash2 } from "lucide-react";
import { Button } from "@datasafe/ui/ui/button";
import { Input } from "@datasafe/ui/ui/input";
import { Card, CardContent, CardHeader, CardTitle } from "@datasafe/ui/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@datasafe/ui/ui/tabs";
import { PageHeader } from "@datasafe/ui/page-header";
import { cn } from "@datasafe/ui/cn";
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
  features?: Record<string, number>;
  onError: (message: string) => void;
  confirmDangerous: (method: string, params: Record<string, unknown>) => Promise<BrowseResult>;
  headerActions?: ReactNode;
}) {
  const { t } = useTranslation(["shell", "bucketDetail", "common"]);
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
      {firstPageMs !== null ? <p className="text-xs text-muted-foreground">{t("firstPage", { ms: firstPageMs })}</p> : null}
      {!props.canWrite ? <p className="text-sm text-muted-foreground">{t("noWrite")}</p> : null}
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
                {index > 0 ? <ChevronRight className="h-3 w-3 text-muted-foreground" /> : null}
                <button
                  type="button"
                  className={cn("hover:text-primary", index === crumbItems.length - 1 ? "font-medium text-foreground" : "text-muted-foreground")}
                  onClick={() => {
                    setPrefix(crumb.path);
                    setMarkers([""]);
                    void load(crumb.path, "", true);
                  }}
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
            {props.canWrite ? (
              <>
                <Input className="max-w-xs" placeholder={t("bucketDetail:actions.newFolder")} value={folderName} onChange={(event) => setFolderName(event.target.value)} />
                <Button type="button" variant="outline" size="sm" onClick={() => void createFolder()}>
                  <FolderPlus className="h-4 w-4" />
                  {t("bucketDetail:actions.newFolder")}
                </Button>
              </>
            ) : null}
          </div>
          <div className="grid gap-4 lg:grid-cols-[1fr_280px]">
            <Card>
              <CardContent className="p-0">
                <div role="listbox" tabIndex={0} aria-label={t("objects")} className="max-h-[28rem] overflow-auto outline-none focus-visible:ring-2" onKeyDown={onKey}>
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b text-left text-muted-foreground">
                        <th className="p-3">{t("bucketDetail:table.name")}</th>
                        <th className="p-3">{t("bucketDetail:table.size")}</th>
                        <th className="p-3">{t("bucketDetail:table.modified")}</th>
                        <th className="w-24 p-3" />
                      </tr>
                    </thead>
                    <tbody>
                      {visible.length === 0 ? (
                        <tr>
                          <td colSpan={4} className="p-8 text-center text-muted-foreground">
                            {t("bucketDetail:emptyFolder")}
                          </td>
                        </tr>
                      ) : null}
                      {visible.map((row, index) => (
                        <tr
                          key={row.key}
                          role="option"
                          aria-selected={index === selected}
                          className={cn("cursor-pointer border-b hover:bg-muted/50", index === selected && "bg-muted/30")}
                          onClick={() => {
                            setSelected(index);
                            if (row.kind === "folder") {
                              setPrefix(row.key);
                              setMarkers([""]);
                              void load(row.key, "", true);
                            }
                          }}
                        >
                          <td className="p-3 font-mono">
                            <span className="inline-flex items-center gap-2">
                              {row.kind === "folder" ? <Folder className="h-4 w-4 text-amber-500" /> : <File className="h-4 w-4 text-muted-foreground" />}
                              {row.name}
                            </span>
                          </td>
                          <td className="p-3">{row.object?.size ?? ""}</td>
                          <td className="p-3">{row.object?.last_modified || ""}</td>
                          <td className="p-3">
                            {props.canWrite ? (
                              <Button
                                type="button"
                                variant="ghost"
                                size="icon"
                                className="h-7 w-7 text-destructive"
                                onClick={(event) => {
                                  event.stopPropagation();
                                  setSelected(index);
                                  setConfirmKey(row.key);
                                }}
                              >
                                <Trash2 className="h-3.5 w-3.5" />
                              </Button>
                            ) : null}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
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
                canWrite={props.canWrite}
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
          {confirmKey && props.canWrite ? (
            <div className="flex flex-wrap items-center gap-2 text-sm">
              <span className="font-mono">{confirmKey}</span>
              <Button type="button" variant="destructive" size="sm" onClick={() => void remove(confirmKey, confirmKey.endsWith("/"))}>
                {t("common:delete")}
              </Button>
              <Button type="button" variant="outline" size="sm" onClick={() => setConfirmKey("")}>
                {t("common:cancel")}
              </Button>
            </div>
          ) : null}
          {renaming && current?.kind === "object" && props.canWrite ? (
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
                    {props.canWrite ? (
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
      <TransferPanel profileId={props.profileId} bucket={props.bucket} prefix={prefix} canWrite={props.canWrite} selectedKey={current?.key || ""} selectedFolder={current?.kind === "folder"} />
    </section>
  );
}
