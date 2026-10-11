import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { PageHeader } from "@/components/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { api } from "@/lib/api";

function statusVariant(status: string): "success" | "warning" | "secondary" {
  if (status === "connected") return "success";
  if (status === "degraded") return "warning";
  return "secondary";
}

export function EventDeliveryPage() {
  const { t } = useTranslation("settings");
  const queryClient = useQueryClient();
  const delivery = useQuery({
    queryKey: ["event-delivery"],
    queryFn: () => api.eventDelivery(),
    refetchInterval: 5000,
  });
  const sendTest = useMutation({
    mutationFn: () => api.sendTestEvent(),
    onSuccess: (res) => {
      toast.success(t("events.testSent", { id: res.event_id }));
      void queryClient.invalidateQueries({ queryKey: ["event-delivery"] });
    },
    onError: (err: Error) => {
      toast.error(err.message || t("events.testFailed"));
    },
  });

  return (
    <div className="space-y-4">
      <PageHeader title={t("events.title")} description={t("events.description")} />
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">
          {t("events.queueDepth", { count: delivery.data?.queue_depth ?? 0 })}
        </p>
        <Button
          type="button"
          onClick={() => sendTest.mutate()}
          disabled={sendTest.isPending || delivery.isLoading}
        >
          {sendTest.isPending ? t("events.sending") : t("events.sendTest")}
        </Button>
      </div>
      {delivery.isLoading ? (
        <p className="text-muted-foreground">{t("events.loading")}</p>
      ) : delivery.isError ? (
        <p className="text-destructive">{t("events.loadError")}</p>
      ) : (
        <div className="grid gap-3">
          {(delivery.data?.sinks ?? []).map((sink) => (
            <Card key={sink.name}>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-base">{t(`events.sinks.${sink.name}`, { defaultValue: sink.name })}</CardTitle>
                <Badge variant={statusVariant(sink.status)}>{t(`events.status.${sink.status}`, { defaultValue: sink.status })}</Badge>
              </CardHeader>
              <CardContent className="space-y-1 text-sm">
                <p>{t("events.sinkQueue", { count: sink.queue_depth })}</p>
                <p className="text-muted-foreground break-all">
                  {sink.last_error ? t("events.lastError", { error: sink.last_error }) : t("events.noError")}
                </p>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
