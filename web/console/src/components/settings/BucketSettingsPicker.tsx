import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import type { BucketSettings } from "@/lib/api";
import { isBucketConfigured } from "@/lib/bucket-settings-merge";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

type BucketSettingsPickerProps = {
  buckets: BucketSettings[];
  selected: Set<string>;
  onSelectionChange: (selected: Set<string>) => void;
};

export function BucketSettingsPicker({
  buckets,
  selected,
  onSelectionChange,
}: BucketSettingsPickerProps) {
  const { t } = useTranslation("settings");
  const [search, setSearch] = useState("");

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return buckets;
    return buckets.filter((b) => b.name.toLowerCase().includes(q));
  }, [buckets, search]);

  const toggle = (name: string) => {
    const next = new Set(selected);
    if (next.has(name)) next.delete(name);
    else next.add(name);
    onSelectionChange(next);
  };

  const selectVisible = () => {
    const next = new Set(selected);
    for (const b of filtered) next.add(b.name);
    onSelectionChange(next);
  };

  const clearAll = () => onSelectionChange(new Set());

  return (
    <div className="flex h-full flex-col gap-3 rounded-lg border bg-card p-4">
      <div className="space-y-1">
        <Label>{t("picker.label")}</Label>
        <p className="text-xs text-muted-foreground">
          {t("picker.selected", { count: selected.size })}
        </p>
      </div>

      <Input
        placeholder={t("picker.searchPlaceholder")}
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        className="h-8"
      />

      <div className="flex gap-2">
        <Button type="button" variant="outline" size="sm" className="h-7 text-xs" onClick={selectVisible}>
          {t("picker.selectVisible")}
        </Button>
        <Button type="button" variant="ghost" size="sm" className="h-7 text-xs" onClick={clearAll}>
          {t("picker.clear")}
        </Button>
      </div>

      <div className="min-h-0 flex-1 space-y-1 overflow-y-auto">
        {filtered.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("picker.noMatch")}</p>
        ) : (
          filtered.map((b) => {
            const configured = isBucketConfigured(b);
            return (
              <label
                key={b.name}
                className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 hover:bg-muted/60"
              >
                <input
                  type="checkbox"
                  className="h-4 w-4"
                  checked={selected.has(b.name)}
                  onChange={() => toggle(b.name)}
                />
                <span className="flex-1 truncate font-mono text-sm">{b.name}</span>
                <Badge variant={configured ? "default" : "secondary"} className="text-[10px]">
                  {configured ? t("picker.configured") : t("picker.default")}
                </Badge>
              </label>
            );
          })
        )}
      </div>
    </div>
  );
}
