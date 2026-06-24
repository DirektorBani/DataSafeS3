import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { Plus, RefreshCw, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { api, type ConsoleUser, type UserRole, bytesFromQuota, quotaFromBytes, type StorageQuotaUnit } from "@/lib/api";
import { formatDate, formatBytes } from "@/lib/utils";
import { useObjectQuotaPresets } from "@/hooks/use-quota-presets";
import { PageHeader } from "@/components/page-header";
import { DataTable } from "@/components/data-table";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

export function UsersPage() {
  const { t, i18n } = useTranslation(["users", "common"]);
  const objectQuotaPresets = useObjectQuotaPresets();
  const queryClient = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<ConsoleUser | null>(null);
  const [resetTarget, setResetTarget] = useState<ConsoleUser | null>(null);
  const [quotaTarget, setQuotaTarget] = useState<ConsoleUser | null>(null);
  const [quotaUnlimited, setQuotaUnlimited] = useState(true);
  const [quotaValue, setQuotaValue] = useState("10");
  const [quotaUnit, setQuotaUnit] = useState<StorageQuotaUnit>("GB");
  const [quotaObjects, setQuotaObjects] = useState("0");
  const [newPassword, setNewPassword] = useState("");
  const [form, setForm] = useState({
    username: "",
    email: "",
    password: "",
    role: "user" as UserRole,
  });

  const users = useQuery({
    queryKey: ["users"],
    queryFn: async () => (await api.listUsers()).users,
  });

  const createMutation = useMutation({
    mutationFn: () => api.createUser(form),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["users"] });
      setCreateOpen(false);
      setForm({ username: "", email: "", password: "", role: "user" });
      toast.success(t("users:toast.created"));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteUser(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["users"] });
      setDeleteTarget(null);
      toast.success(t("users:toast.deleted"));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const resetMutation = useMutation({
    mutationFn: ({ id, password }: { id: string; password: string }) =>
      api.resetPassword(id, password),
    onSuccess: () => {
      setResetTarget(null);
      setNewPassword("");
      toast.success(t("users:toast.passwordReset"));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const updateStatus = useMutation({
    mutationFn: ({ id, status }: { id: string; status: string }) =>
      api.updateUser(id, { status: status as "active" | "suspended" }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["users"] });
      toast.success(t("users:toast.updated"));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const updateQuota = useMutation({
    mutationFn: ({ id, max_size_bytes, max_objects }: { id: string; max_size_bytes: number; max_objects: number }) =>
      api.updateUser(id, { max_size_bytes, max_objects }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["users"] });
      setQuotaTarget(null);
      toast.success(t("users:toast.quotaUpdated"));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const columns: ColumnDef<ConsoleUser>[] = [
    { accessorKey: "username", header: t("users:columns.username") },
    { accessorKey: "email", header: t("users:columns.email") },
    { accessorKey: "role", header: t("users:columns.role") },
    {
      accessorKey: "max_size_bytes",
      header: t("users:columns.storageQuota"),
      cell: ({ row }) =>
        row.original.max_size_bytes
          ? formatBytes(row.original.max_size_bytes, i18n.language)
          : t("common:unlimited"),
    },
    {
      accessorKey: "max_objects",
      header: t("users:columns.objectQuota"),
      cell: ({ row }) =>
        row.original.max_objects
          ? row.original.max_objects.toLocaleString()
          : t("common:unlimited"),
    },
    {
      accessorKey: "status",
      header: t("users:columns.status"),
      cell: ({ row }) => (
        <Select
          value={row.original.status}
          onValueChange={(v) =>
            updateStatus.mutate({ id: row.original.id, status: v })
          }
        >
          <SelectTrigger className="h-8 w-28">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="active">{t("users:status.active")}</SelectItem>
            <SelectItem value="suspended">{t("users:status.suspended")}</SelectItem>
          </SelectContent>
        </Select>
      ),
    },
    {
      accessorKey: "last_login",
      header: t("users:columns.lastLogin"),
      cell: ({ row }) =>
        row.original.last_login ? formatDate(row.original.last_login, i18n.language) : t("common:duration.empty"),
    },
    {
      id: "actions",
      header: "",
      cell: ({ row }) => (
        <div className="flex gap-1">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              setQuotaTarget(row.original);
              const q = quotaFromBytes(row.original.max_size_bytes ?? 0);
              setQuotaUnlimited(q.unlimited);
              setQuotaValue(q.unlimited ? "10" : String(q.value));
              setQuotaUnit(q.unit);
              setQuotaObjects(String(row.original.max_objects ?? 0));
            }}
          >
            {t("users:actions.edit")}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setResetTarget(row.original)}
          >
            {t("users:actions.resetPwd")}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="text-destructive"
            onClick={() => setDeleteTarget(row.original)}
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      ),
    },
  ];

  return (
    <div>
      <PageHeader
        title={t("users:title")}
        description={t("users:description")}
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => users.refetch()} disabled={users.isFetching}>
              <RefreshCw className={`h-4 w-4 ${users.isFetching ? "animate-spin" : ""}`} />
              {t("common:refresh")}
            </Button>
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus className="h-4 w-4" />
              {t("users:create.title")}
            </Button>
          </>
        }
      />

      {users.isLoading ? (
        <p className="text-muted-foreground">{t("users:loading")}</p>
      ) : (
        <DataTable
          columns={columns}
          data={users.data ?? []}
          searchKey="username"
          searchPlaceholder={t("users:searchPlaceholder")}
          emptyMessage={t("users:empty")}
        />
      )}

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("users:create.title")}</DialogTitle>
            <DialogDescription>{t("users:create.description")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-2">
              <Label>{t("users:columns.username")}</Label>
              <Input value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} />
            </div>
            <div className="space-y-2">
              <Label>{t("users:columns.email")}</Label>
              <Input type="email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} />
            </div>
            <div className="space-y-2">
              <Label>{t("users:fields.password")}</Label>
              <Input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} />
            </div>
            <div className="space-y-2">
              <Label>{t("users:columns.role")}</Label>
              <Select value={form.role} onValueChange={(v) => setForm({ ...form, role: v as UserRole })}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="administrator">{t("common:roles.administrator")}</SelectItem>
                  <SelectItem value="operator">{t("common:roles.operator")}</SelectItem>
                  <SelectItem value="user">{t("common:roles.user")}</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>{t("common:cancel")}</Button>
            <Button onClick={() => createMutation.mutate()} disabled={createMutation.isPending}>
              {t("users:create.action")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!resetTarget} onOpenChange={(o) => !o && setResetTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("users:reset.title")}</DialogTitle>
            <DialogDescription>{t("users:reset.description", { username: resetTarget?.username ?? "" })}</DialogDescription>
          </DialogHeader>
          <Input type="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} />
          <DialogFooter>
            <Button variant="outline" onClick={() => setResetTarget(null)}>{t("common:cancel")}</Button>
            <Button
              onClick={() => resetTarget && resetMutation.mutate({ id: resetTarget.id, password: newPassword })}
              disabled={!newPassword || resetMutation.isPending}
            >
              {t("users:reset.action")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!quotaTarget} onOpenChange={(o) => !o && setQuotaTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("users:quota.title")}</DialogTitle>
            <DialogDescription>{t("users:quota.description", { username: quotaTarget?.username ?? "" })}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>{t("users:quota.storage")}</Label>
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  className="h-4 w-4"
                  checked={quotaUnlimited}
                  onChange={(e) => setQuotaUnlimited(e.target.checked)}
                />
                {t("common:unlimited")}
              </label>
              {!quotaUnlimited && (
                <div className="flex gap-2">
                  <Input
                    type="number"
                    min={1}
                    value={quotaValue}
                    onChange={(e) => setQuotaValue(e.target.value)}
                    className="flex-1"
                  />
                  <Select value={quotaUnit} onValueChange={(v) => setQuotaUnit(v as StorageQuotaUnit)}>
                    <SelectTrigger className="w-24"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="MB">MB</SelectItem>
                      <SelectItem value="GB">GB</SelectItem>
                      <SelectItem value="TB">TB</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              )}
            </div>
            <div className="space-y-2">
              <Label>{t("users:quota.objectCount")}</Label>
              <Select value={quotaObjects} onValueChange={setQuotaObjects}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  {objectQuotaPresets.map((p) => (
                    <SelectItem key={p.label} value={String(p.count)}>{p.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setQuotaTarget(null)}>{t("common:cancel")}</Button>
            <Button
              onClick={() => {
                if (!quotaTarget) return;
                const maxSize = quotaUnlimited
                  ? 0
                  : bytesFromQuota(parseFloat(quotaValue) || 0, quotaUnit);
                updateQuota.mutate({
                  id: quotaTarget.id,
                  max_size_bytes: maxSize,
                  max_objects: parseInt(quotaObjects, 10) || 0,
                });
              }}
              disabled={updateQuota.isPending || (!quotaUnlimited && !(parseFloat(quotaValue) > 0))}
            >
              {t("common:save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(o) => !o && setDeleteTarget(null)}
        title={t("users:delete.title")}
        description={t("users:delete.description", { username: deleteTarget?.username ?? "" })}
        confirmLabel={t("common:delete")}
        destructive
        loading={deleteMutation.isPending}
        onConfirm={() => deleteTarget && deleteMutation.mutate(deleteTarget.id)}
      />
    </div>
  );
}
