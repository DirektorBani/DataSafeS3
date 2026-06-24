import { useState } from "react";
import { Check, Copy } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

type CopyButtonProps = {
  value: string;
  label?: string;
  className?: string;
  variant?: "default" | "outline" | "ghost" | "secondary";
  size?: "default" | "sm" | "icon";
};

export function CopyButton({
  value,
  label,
  className,
  variant = "outline",
  size = "sm",
}: CopyButtonProps) {
  const { t } = useTranslation("common");
  const [copied, setCopied] = useState(false);

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      toast.success(label ? t("copy.copied", { label }) : t("copy.copiedGeneric"));
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error(t("copy.failed"));
    }
  }

  return (
    <Button
      type="button"
      variant={variant}
      size={size}
      onClick={handleCopy}
      className={cn("shrink-0", className)}
      aria-label={label ? t("copy.aria", { label }) : t("copy.ariaGeneric")}
    >
      {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
      {size !== "icon" && (copied ? t("copy.buttonCopied") : (label ?? t("copy.buttonCopy")))}
    </Button>
  );
}
