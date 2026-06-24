import { Construction } from "lucide-react";
import { useTranslation } from "react-i18next";
import { PageHeader } from "@/components/page-header";
import { StatusBadge } from "@/components/status-badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

type ComingSoonProps = {
  title: string;
  description: string;
};

export function ComingSoonPage({ title, description }: ComingSoonProps) {
  const { t } = useTranslation("comingSoon");

  return (
    <div>
      <PageHeader title={title} description={description} badge={<StatusBadge status="info" label={t("badge")} />} />
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Construction className="h-4 w-4" />
            {t("title")}
          </CardTitle>
          <CardDescription>{t("description")}</CardDescription>
        </CardHeader>
        <CardContent className="text-sm text-muted-foreground">
          {t("body")}
        </CardContent>
      </Card>
    </div>
  );
}
