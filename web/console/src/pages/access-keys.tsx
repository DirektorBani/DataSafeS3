import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { AlertTriangle, KeyRound, Plus, RefreshCw, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { useAuth } from "@/hooks/use-auth";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { api, type AccessKey, type APIToken, type CreatedKey } from "@/lib/api";
import { formatDate } from "@/lib/utils";
import { PageHeader } from "@/components/page-header";
import { DataTable } from "@/components/data-table";
import { CopyButton } from "@/components/copy-button";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

export function AccessKeysPage() {
  const { t, i18n } = useTranslation(["access", "common"]);
  const { isAdmin } = useAuth();
  const queryClient = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);
  const [label, setLabel] = useState("console");
  const [createdKey, setCreatedKey] = useState<CreatedKey | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<AccessKey | null>(null);
  const [tokenCreateOpen, setTokenCreateOpen] = useState(false);
  const [tokenName, setTokenName] = useState("");
  const [createdToken, setCreatedToken] = useState<{ name: string; token: string } | null>(null);
  const [deleteTokenTarget, setDeleteTokenTarget] = useState<APIToken | null>(null);

  const tokens = useQuery({
    queryKey: ["api-tokens"],
    queryFn: async () => (await api.listAPITokens()).tokens,
  });

  const createTokenMutation = useMutation({
    mutationFn: (name: string) => api.createAPIToken({ name, expires_days: 90, scopes: ["read", "write"] }),
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ["api-tokens"] });
      setCreatedToken({ name: data.name, token: data.token });
      setTokenCreateOpen(false);
      setTokenName("");
      toast.success(t("access:toast.tokenCreated"));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const deleteTokenMutation = useMutation({
    mutationFn: (id: string) => api.deleteAPIToken(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["api-tokens"] });
      setDeleteTokenTarget(null);
      toast.success(t("access:toast.tokenDeleted"));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const keys = useQuery({
    queryKey: ["keys"],
    queryFn: async () => (await api.listKeys()).keys,
  });

  const createMutation = useMutation({
    mutationFn: (lbl: string) => api.createKey(lbl),
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ["keys"] });
      setCreatedKey(data);
      setCreateOpen(false);
      setLabel("console");
      toast.success(t("access:toast.keyCreated"));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (accessKey: string) => api.deleteKey(accessKey),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["keys"] });
      toast.success(t("access:toast.keyDeleted"));
      setDeleteTarget(null);
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const columns: ColumnDef<AccessKey>[] = [
    {
      accessorKey: "access_key",
      header: t("access:columns.accessKeyId"),
      cell: ({ row }) => (
        <div className="flex items-center gap-2">
          <code className="rounded bg-muted px-2 py-1 text-xs font-mono">{row.original.access_key}</code>
          <CopyButton value={row.original.access_key} label={t("access:copy.keyId")} size="icon" />
        </div>
      ),
    },
    {
      accessorKey: "label",
      header: t("access:columns.label"),
    },
    ...(isAdmin
      ? [
          {
            accessorKey: "owner",
            header: t("access:columns.owner"),
            cell: ({ row }: { row: { original: AccessKey } }) => row.original.owner || t("common:duration.empty"),
          } as ColumnDef<AccessKey>,
        ]
      : []),
    {
      accessorKey: "created_at",
      header: t("access:columns.created"),
      cell: ({ row }) => formatDate(row.original.created_at, i18n.language),
    },
    {
      id: "actions",
      header: "",
      cell: ({ row }) => (
        <Button
          variant="ghost"
          size="sm"
          className="text-destructive hover:text-destructive"
          onClick={() => setDeleteTarget(row.original)}
        >
          <Trash2 className="h-4 w-4" />
        </Button>
      ),
    },
  ];

  return (
    <div>
      <PageHeader
        title={t("access:title")}
        description={t("access:description")}
      />

      <Tabs defaultValue="s3-keys" className="space-y-4">
        <TabsList>
          <TabsTrigger value="s3-keys">{t("access:tabs.s3")}</TabsTrigger>
          <TabsTrigger value="api-tokens">{t("access:tabs.api")}</TabsTrigger>
        </TabsList>

        <TabsContent value="s3-keys">
          <div className="flex flex-wrap gap-2 mb-4">
            <Button variant="outline" size="sm" onClick={() => keys.refetch()} disabled={keys.isFetching}>
              <RefreshCw className={`h-4 w-4 ${keys.isFetching ? "animate-spin" : ""}`} />
              {t("common:refresh")}
            </Button>
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus className="h-4 w-4" />
              {t("access:actions.createKey")}
            </Button>
          </div>
          {keys.isLoading ? (
            <p className="text-muted-foreground">{t("access:loading.keys")}</p>
          ) : (
            <DataTable
              columns={columns}
              data={keys.data ?? []}
              searchKey="access_key"
              searchPlaceholder={t("access:searchPlaceholder")}
              emptyMessage={t("access:empty.keys")}
            />
          )}
        </TabsContent>

        <TabsContent value="api-tokens">
          <div className="flex flex-wrap gap-2 mb-4">
            <Button variant="outline" size="sm" onClick={() => tokens.refetch()} disabled={tokens.isFetching}>
              <RefreshCw className={`h-4 w-4 ${tokens.isFetching ? "animate-spin" : ""}`} />
              {t("common:refresh")}
            </Button>
            <Button size="sm" onClick={() => setTokenCreateOpen(true)}>
              <Plus className="h-4 w-4" />
              {t("access:actions.createToken")}
            </Button>
          </div>
          {tokens.isLoading ? (
            <p className="text-muted-foreground">{t("access:loading.tokens")}</p>
          ) : (
            <DataTable
              columns={[
                { accessorKey: "name", header: t("access:columns.name") },
                { accessorKey: "username", header: t("access:columns.user") },
                { accessorKey: "scopes", header: t("access:columns.scopes"), cell: ({ row }) => row.original.scopes.join(", ") },
                { accessorKey: "expires_at", header: t("access:columns.expires"), cell: ({ row }) => formatDate(row.original.expires_at, i18n.language) },
                {
                  id: "actions",
                  header: "",
                  cell: ({ row }) => (
                    <Button variant="ghost" size="sm" className="text-destructive" onClick={() => setDeleteTokenTarget(row.original)}>
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  ),
                },
              ]}
              data={tokens.data ?? []}
              emptyMessage={t("access:empty.tokens")}
            />
          )}
        </TabsContent>
      </Tabs>

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("access:createKey.title")}</DialogTitle>
            <DialogDescription>
              {t("access:createKey.description")}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="key-label">{t("access:columns.label")}</Label>
            <Input
              id="key-label"
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              placeholder={t("access:placeholder.keyLabel")}
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              {t("common:cancel")}
            </Button>
            <Button
              onClick={() => createMutation.mutate(label.trim() || t("access:placeholder.keyLabel"))}
              disabled={createMutation.isPending}
            >
              {createMutation.isPending ? t("common:creating") : t("common:create")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!createdKey} onOpenChange={(open) => !open && setCreatedKey(null)}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <KeyRound className="h-5 w-5 text-primary" />
              {t("access:created.title")}
            </DialogTitle>
            <DialogDescription>
              {t("access:created.description")}
            </DialogDescription>
          </DialogHeader>

          <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 flex gap-2 text-sm">
            <AlertTriangle className="h-4 w-4 shrink-0 text-amber-500 mt-0.5" />
            <p>{t("access:created.warning")}</p>
          </div>

          <div className="space-y-4">
            <div className="space-y-2">
              <Label>{t("access:fields.accessKeyId")}</Label>
              <div className="flex items-center gap-2">
                <code className="flex-1 rounded-md border bg-muted px-3 py-2 text-sm font-mono break-all">
                  {createdKey?.access_key}
                </code>
                <CopyButton value={createdKey?.access_key ?? ""} label={t("access:copy.keyId")} />
              </div>
            </div>
            <div className="space-y-2">
              <Label>{t("access:fields.secretAccessKey")}</Label>
              <div className="flex items-center gap-2">
                <code className="flex-1 rounded-md border bg-muted px-3 py-2 text-sm font-mono break-all">
                  {createdKey?.secret_key}
                </code>
                <CopyButton value={createdKey?.secret_key ?? ""} label={t("access:copy.secret")} />
              </div>
            </div>
          </div>

          <DialogFooter>
            <Button onClick={() => setCreatedKey(null)}>{t("common:done")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title={t("access:deleteKey.title")}
        description={t("access:deleteKey.description", { key: deleteTarget?.access_key ?? "" })}
        confirmLabel={t("common:delete")}
        destructive
        loading={deleteMutation.isPending}
        onConfirm={() => deleteTarget && deleteMutation.mutate(deleteTarget.access_key)}
      />

      <Dialog open={tokenCreateOpen} onOpenChange={setTokenCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("access:createToken.title")}</DialogTitle>
            <DialogDescription>{t("access:createToken.description")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label>{t("access:columns.name")}</Label>
            <Input value={tokenName} onChange={(e) => setTokenName(e.target.value)} placeholder={t("access:placeholder.ciPipeline")} />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setTokenCreateOpen(false)}>{t("common:cancel")}</Button>
            <Button onClick={() => createTokenMutation.mutate(tokenName.trim() || "token")} disabled={createTokenMutation.isPending}>{t("common:create")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!createdToken} onOpenChange={(o) => !o && setCreatedToken(null)}>
        <DialogContent>
          <DialogHeader><DialogTitle>{t("access:tokenCreated")}</DialogTitle></DialogHeader>
          <div className="flex gap-2">
            <Input readOnly value={createdToken?.token ?? ""} className="font-mono text-xs" />
            <CopyButton value={createdToken?.token ?? ""} label={t("access:copy.token")} />
          </div>
          <DialogFooter><Button onClick={() => setCreatedToken(null)}>{t("common:done")}</Button></DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={!!deleteTokenTarget}
        onOpenChange={(o) => !o && setDeleteTokenTarget(null)}
        title={t("access:deleteToken.title")}
        description={t("access:deleteToken.description", { name: deleteTokenTarget?.name ?? "" })}
        confirmLabel={t("common:delete")}
        destructive
        loading={deleteTokenMutation.isPending}
        onConfirm={() => deleteTokenTarget && deleteTokenMutation.mutate(deleteTokenTarget.id)}
      />
    </div>
  );
}
