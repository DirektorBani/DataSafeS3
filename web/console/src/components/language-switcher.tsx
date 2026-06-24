import { useTranslation } from "react-i18next";
import { useLocale } from "@/hooks/use-locale";
import { cn } from "@/lib/utils";
import type { AppLocale } from "@/i18n";

const LOCALES: { code: AppLocale; label: string }[] = [
  { code: "en", label: "EN" },
  { code: "ru", label: "RU" },
  { code: "de", label: "DE" },
  { code: "fr", label: "FR" },
];

export function LanguageSwitcher({ className }: { className?: string }) {
  const { t } = useTranslation("common");
  const { locale, setLocale } = useLocale();

  return (
    <div
      className={cn("inline-flex items-center rounded-md border bg-background p-0.5 text-xs font-medium", className)}
      role="group"
      aria-label={t("languageSwitcher.ariaLabel")}
    >
      {LOCALES.map(({ code, label }, index) => (
        <span key={code} className="inline-flex items-center">
          {index > 0 && <span className="px-0.5 text-muted-foreground">|</span>}
          <button
            type="button"
            onClick={() => setLocale(code)}
            className={cn(
              "rounded px-2 py-1 transition-colors",
              locale === code
                ? "bg-primary text-primary-foreground"
                : "text-muted-foreground hover:text-foreground"
            )}
            aria-pressed={locale === code}
          >
            {label}
          </button>
        </span>
      ))}
    </div>
  );
}
