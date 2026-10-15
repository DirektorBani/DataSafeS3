import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { ApiError, api, type KMSJob } from "@/lib/api";
import { Button } from "@/components/ui/button";

function errorText(err: Error): string {
  if (err instanceof ApiError && typeof err.body?.message === "string") return err.body.message;
  return err.message;
}

function terminal(status: string | undefined): boolean {
  return status === "completed" || status === "completed_with_errors" || status === "failed";
}

function percent(job: KMSJob | undefined): number {
  if (!job) return 0;
  if (terminal(job.status)) return 100;
  if (job.total > 0) return Math.min(100, Math.round((job.done / job.total) * 100));
  return 0;
}

export function EncryptExisting({
  bucket,
  disabled,
  activeJobId,
}: {
  bucket: string;
  disabled?: boolean;
  activeJobId?: string;
}) {
  const { t } = useTranslation("encryption");
  const queryClient = useQueryClient();
  const [jobID, setJobID] = useState(activeJobId ?? "");

  useEffect(() => {
    if (activeJobId) setJobID(activeJobId);
  }, [activeJobId]);

  const job = useQuery({
    queryKey: ["kms-job", jobID],
    queryFn: () => api.getKMSJob(jobID),
    enabled: !!jobID,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      if (status === "pending" || status === "running" || !status) return 1000;
      return false;
    },
  });

  const start = useMutation({
    mutationFn: () => api.startEncryptExisting(bucket),
    onSuccess: (data) => {
      setJobID(data.id);
      queryClient.invalidateQueries({ queryKey: ["bucket-encryption", bucket] });
    },
    onError: (err: Error) => {
      if (err instanceof ApiError && typeof err.body?.job_id === "string") {
        setJobID(err.body.job_id);
        return;
      }
      toast.error(errorText(err));
    },
  });

  const view = job.data;
  const pct = percent(view);
  const running = view?.status === "pending" || view?.status === "running" || start.isPending;

  async function downloadReport() {
    if (!jobID) return;
    const report = await api.getKMSJobReport(jobID);
    const blob = new Blob([JSON.stringify(report, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `kms-job-${jobID}.json`;
    anchor.click();
    URL.revokeObjectURL(url);
  }

  return (
    <div className="space-y-2">
      <Button
        size="sm"
        variant="outline"
        data-testid="encryption-encrypt-existing"
        disabled={disabled || running}
        onClick={() => start.mutate()}
      >
        {t("encryptExisting")}
      </Button>
      <p className="text-xs text-muted-foreground">{t("encryptExistingHint")}</p>
      {view && (
        <div className="space-y-1">
          <div
            className="h-2 w-full overflow-hidden rounded bg-muted"
            data-testid="encryption-job-progress"
            data-progress={pct}
            data-status={view.status}
          >
            <div className="h-full bg-primary" style={{ width: `${pct}%` }} />
          </div>
          <p className="text-xs text-muted-foreground" data-testid="encryption-job-counts">
            {t("jobProgress", { done: view.done, total: view.total })}
            {view.errors > 0 ? ` · ${t("jobErrors", { count: view.errors })}` : ""}
            {view.status === "completed_with_errors"
              ? ` · ${t("jobDoneWithErrors")}`
              : terminal(view.status)
                ? ` · ${t("jobDone")}`
                : view.eta_seconds > 0
                  ? ` · ${t("jobEta", { seconds: view.eta_seconds })}`
                  : ""}
          </p>
          {terminal(view.status) && (
            <Button size="sm" variant="link" className="h-auto px-0" data-testid="encryption-job-report" onClick={() => void downloadReport()}>
              {t("jobReport")}
            </Button>
          )}
        </div>
      )}
    </div>
  );
}
