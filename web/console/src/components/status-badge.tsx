import { Badge } from "@/components/ui/badge";
import { useTranslation } from "react-i18next";

type Status = "ok" | "warning" | "error" | "info" | "demo";

export function StatusBadge({ status, label }: { status: Status; label?: string }) {
  const { t } = useTranslation("common");
  const statusLabels: Record<Status, string> = {
    ok: t("status.healthy"),
    warning: t("status.warning"),
    error: t("status.error"),
    info: t("status.info"),
    demo: t("status.demo"),
  };
  const variants: Record<Status, "success" | "warning" | "destructive" | "secondary" | "outline"> = {
    ok: "success",
    warning: "warning",
    error: "destructive",
    info: "secondary",
    demo: "outline",
  };
  return <Badge variant={variants[status]}>{label ?? statusLabels[status]}</Badge>;
}
