import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Save } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { api, type SystemConfig } from "@/lib/api";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import { AdministratorSettings } from "@/components/settings/AdministratorSettings";
import {
  initTrashRetentionFromDays,
  trashRetentionDays,
  type TrashRetentionUnit,
} from "@/components/settings/trash-retention";

export function AdministratorSettingsPage() {
  const { t } = useTranslation(["settings", "adminSettings", "common"]);
  const queryClient = useQueryClient();
  const [systemDraft, setSystemDraft] = useState<SystemConfig | null>(null);
  const [trashRetentionValue, setTrashRetentionValue] = useState("30");
  const [trashRetentionUnit, setTrashRetentionUnit] = useState<TrashRetentionUnit>("days");

  const systemConfig = useQuery({
    queryKey: ["system-config"],
    queryFn: () => api.getSystemConfig(),
  });

  useEffect(() => {
    if (systemConfig.data) {
      setSystemDraft({ ...systemConfig.data });
      const { value, unit } = initTrashRetentionFromDays(systemConfig.data.trash_retention_days || 30);
      setTrashRetentionValue(value);
      setTrashRetentionUnit(unit);
    }
  }, [systemConfig.data]);

  const retentionDays = trashRetentionDays(trashRetentionValue, trashRetentionUnit);

  const saveSystemMutation = useMutation({
    mutationFn: () => {
      if (!systemDraft) throw new Error("No config");
      if (retentionDays < 1 || retentionDays > 3650) {
        throw new Error(t("settings:system.error.retentionRange"));
      }
      return api.updateSystemConfig({ ...systemDraft, trash_retention_days: retentionDays });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["system-config"] });
      toast.success(t("settings:system.toast.saved"));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  if (systemConfig.isLoading) {
    return <p className="text-muted-foreground">{t("settings:system.loading")}</p>;
  }

  if (!systemDraft) {
    return <p className="text-muted-foreground">{t("settings:system.loadError")}</p>;
  }

  return (
    <div>
      <PageHeader
        title={t("settings:system.title")}
        description={t("settings:system.description")}
        actions={
          <Button
            size="sm"
            onClick={() => saveSystemMutation.mutate()}
            disabled={saveSystemMutation.isPending}
          >
            <Save className="h-4 w-4" />
            Save
          </Button>
        }
      />

      <AdministratorSettings
        draft={systemDraft}
        onDraftChange={setSystemDraft}
        trashRetentionValue={trashRetentionValue}
        trashRetentionUnit={trashRetentionUnit}
        onTrashRetentionValueChange={setTrashRetentionValue}
        onTrashRetentionUnitChange={setTrashRetentionUnit}
        trashRetentionDays={retentionDays}
      />
    </div>
  );
}
