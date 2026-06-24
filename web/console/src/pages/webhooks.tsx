import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, RefreshCw, RotateCcw, Trash2, Webhook as WebhookIcon } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { api, WEBHOOK_EVENTS, type Webhook as WebhookConfig, type WebhookDelivery } from "@/lib/api";
import { formatDate } from "@/lib/utils";
import { PageHeader } from "@/components/page-header";
import { DataTable } from "@/components/data-table";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
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

const TEMPLATE_NONE = "__custom__";

type WebhookForm = {
  name: string;
  url: string;
  template: string;
  events: string[];
  headers: string;
  enabled: boolean;
};

const defaultForm = (): WebhookForm => ({
  name: "",
  url: "",
  template: TEMPLATE_NONE,
  events: ["ObjectCreated", "ObjectDeleted", "BucketCreated"],
  headers: "",
  enabled: true,
});

function parseHeaders(raw: string, invalidMessage: string): Record<string, string> | undefined {
  const trimmed = raw.trim();
  if (!trimmed) return undefined;
  const parsed = JSON.parse(trimmed) as Record<string, string>;
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    throw new Error(invalidMessage);
  }
  return parsed;
}

export function WebhooksPage() {
  const { t, i18n } = useTranslation(["webhooks", "common"]);
  const queryClient = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<WebhookConfig | null>(null);
  const [form, setForm] = useState<WebhookForm>(defaultForm);
  const [deliveryWebhook, setDeliveryWebhook] = useState<WebhookConfig | null>(null);

  const hooks = useQuery({
    queryKey: ["webhooks"],
    queryFn: async () => (await api.listWebhooks()).webhooks,
  });

  const templates = useQuery({
    queryKey: ["webhook-templates"],
    queryFn: async () => (await api.webhookTemplates()).templates,
    enabled: createOpen,
  });

  useEffect(() => {
    if (createOpen) setForm(defaultForm());
  }, [createOpen]);

  const createMutation = useMutation({
    mutationFn: () => {
      const headers = form.headers.trim() ? parseHeaders(form.headers, t("webhooks:error.headersJson")) : undefined;
      return api.createWebhook({
        name: form.name.trim() || t("webhooks:create.defaultName"),
        url: form.url.trim(),
        events: form.events.length ? form.events : [...WEBHOOK_EVENTS],
        headers,
        enabled: form.enabled,
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["webhooks"] });
      setCreateOpen(false);
      setForm(defaultForm());
      toast.success(t("webhooks:toast.created"));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteWebhook(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["webhooks"] });
      setDeleteTarget(null);
      toast.success(t("webhooks:toast.deleted"));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const toggleMutation = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      api.updateWebhook(id, { enabled }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["webhooks"] }),
  });

  const deliveries = useQuery({
    queryKey: ["webhook-deliveries", deliveryWebhook?.id],
    queryFn: async () => (await api.listWebhookDeliveries(deliveryWebhook!.id, 30)).deliveries,
    enabled: !!deliveryWebhook,
  });

  const retryMutation = useMutation({
    mutationFn: ({ webhookId, deliveryId }: { webhookId: string; deliveryId: string }) =>
      api.retryWebhookDelivery(webhookId, deliveryId),
    onSuccess: () => {
      deliveries.refetch();
      toast.success(t("webhooks:toast.retryQueued"));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const toggleEvent = (event: string) => {
    setForm((prev) => ({
      ...prev,
      events: prev.events.includes(event)
        ? prev.events.filter((e) => e !== event)
        : [...prev.events, event],
    }));
  };

  return (
    <div>
      <PageHeader
        title={t("webhooks:title")}
        description={t("webhooks:description")}
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => hooks.refetch()} disabled={hooks.isFetching}>
              <RefreshCw className={`h-4 w-4 ${hooks.isFetching ? "animate-spin" : ""}`} />
              {t("common:refresh")}
            </Button>
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus className="h-4 w-4" /> {t("webhooks:actions.add")}
            </Button>
          </>
        }
      />

      {hooks.isLoading ? (
        <p className="text-muted-foreground">{t("webhooks:loading")}</p>
      ) : (
        <DataTable
          columns={[
            { accessorKey: "name", header: t("webhooks:columns.name") },
            {
              accessorKey: "url",
              header: t("webhooks:columns.url"),
              cell: ({ row }) => <code className="text-xs font-mono truncate max-w-xs block">{row.original.url}</code>,
            },
            {
              accessorKey: "events",
              header: t("webhooks:columns.events"),
              cell: ({ row }) => row.original.events.join(", "),
            },
            {
              accessorKey: "enabled",
              header: t("webhooks:columns.status"),
              cell: ({ row }) => (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => toggleMutation.mutate({ id: row.original.id, enabled: !row.original.enabled })}
                >
                  {row.original.enabled ? t("webhooks:status.enabled") : t("webhooks:status.disabled")}
                </Button>
              ),
            },
            {
              accessorKey: "created_at",
              header: t("webhooks:columns.created"),
              cell: ({ row }) => formatDate(row.original.created_at, i18n.language),
            },
            {
              id: "actions",
              header: "",
              cell: ({ row }) => (
                <div className="flex gap-1">
                  <Button variant="ghost" size="sm" onClick={() => setDeliveryWebhook(row.original)}>{t("webhooks:actions.log")}</Button>
                  <Button variant="ghost" size="sm" className="text-destructive" onClick={() => setDeleteTarget(row.original)}>
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              ),
            },
          ]}
          data={hooks.data ?? []}
          emptyMessage={t("webhooks:empty")}
        />
      )}

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2"><WebhookIcon className="h-4 w-4" /> {t("webhooks:create.title")}</DialogTitle>
            <DialogDescription>{t("webhooks:create.description")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-3 max-h-[60vh] overflow-y-auto pr-1">
            <div className="space-y-2">
              <Label>{t("webhooks:fields.name")}</Label>
              <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder={t("webhooks:placeholder.name")} />
            </div>
            <div className="space-y-2">
              <Label>{t("webhooks:fields.template")}</Label>
              <Select
                value={form.template}
                onValueChange={(v) => {
                  const tmpl = templates.data?.find((x) => x.name === v);
                  setForm({ ...form, template: v, url: tmpl?.url ?? form.url });
                }}
              >
                <SelectTrigger><SelectValue placeholder={t("webhooks:placeholder.template")} /></SelectTrigger>
                <SelectContent>
                  <SelectItem value={TEMPLATE_NONE}>{t("webhooks:placeholder.template")}</SelectItem>
                  {(templates.data ?? []).map((tmpl) => (
                    <SelectItem key={tmpl.name} value={tmpl.name}>{tmpl.name}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>{t("webhooks:fields.targetUrl")}</Label>
              <Input value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} placeholder={t("webhooks:placeholder.url")} />
            </div>
            <div className="space-y-2">
              <Label>{t("webhooks:fields.eventTypes")}</Label>
              <div className="grid gap-2 sm:grid-cols-2">
                {WEBHOOK_EVENTS.map((event) => (
                  <label key={event} className="flex items-center gap-2 text-sm cursor-pointer">
                    <input
                      type="checkbox"
                      className="h-4 w-4 rounded"
                      checked={form.events.includes(event)}
                      onChange={() => toggleEvent(event)}
                    />
                    {event}
                  </label>
                ))}
              </div>
            </div>
            <div className="space-y-2">
              <Label>{t("webhooks:fields.headers")}</Label>
              <Textarea
                value={form.headers}
                onChange={(e) => setForm({ ...form, headers: e.target.value })}
                placeholder={t("webhooks:placeholder.headers")}
                rows={3}
                className="font-mono text-xs"
              />
            </div>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                className="h-4 w-4"
                checked={form.enabled}
                onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
              />
              {t("webhooks:fields.enabled")}
            </label>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>{t("common:cancel")}</Button>
            <Button
              onClick={() => createMutation.mutate()}
              disabled={!form.url.trim() || form.events.length === 0 || createMutation.isPending}
            >
              {t("common:create")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(o) => !o && setDeleteTarget(null)}
        title={t("webhooks:delete.title")}
        description={t("webhooks:delete.description", { name: deleteTarget?.name ?? "" })}
        confirmLabel={t("common:delete")}
        destructive
        loading={deleteMutation.isPending}
        onConfirm={() => deleteTarget && deleteMutation.mutate(deleteTarget.id)}
      />

      <Dialog open={!!deliveryWebhook} onOpenChange={(o) => !o && setDeliveryWebhook(null)}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t("webhooks:delivery.title", { name: deliveryWebhook?.name ?? "" })}</DialogTitle>
            <DialogDescription>{t("webhooks:delivery.description")}</DialogDescription>
          </DialogHeader>
          {deliveries.isLoading ? (
            <p className="text-sm text-muted-foreground">{t("webhooks:delivery.loading")}</p>
          ) : (deliveries.data ?? []).length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("webhooks:delivery.empty")}</p>
          ) : (
            <div className="max-h-80 overflow-y-auto space-y-2">
              {(deliveries.data ?? []).map((d: WebhookDelivery) => (
                <div key={d.id} className="rounded border p-3 text-sm space-y-1">
                  <div className="flex items-center justify-between gap-2">
                    <span className="font-mono text-xs">{d.event}</span>
                    <div className="flex items-center gap-2">
                      <Badge variant={d.success ? "default" : "destructive"}>
                        {d.status_code || "err"}
                      </Badge>
                      {!d.success && (
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={retryMutation.isPending}
                          onClick={() => deliveryWebhook && retryMutation.mutate({ webhookId: deliveryWebhook.id, deliveryId: d.id })}
                        >
                          <RotateCcw className="h-3.5 w-3.5" /> {t("webhooks:delivery.retry")}
                        </Button>
                      )}
                    </div>
                  </div>
                  <p className="text-xs text-muted-foreground">{formatDate(d.last_attempt, i18n.language)} · {t("webhooks:delivery.attempts")} {d.attempts}</p>
                  {d.error && <p className="text-xs text-destructive">{d.error}</p>}
                </div>
              ))}
            </div>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}
