import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export type DurationParts = {
  days: string;
  hours: string;
  minutes: string;
  unlimited: boolean;
};

export function splitDuration(seconds: number | null | undefined): DurationParts {
  if (seconds == null) return { days: "", hours: "", minutes: "", unlimited: false };
  if (seconds < 0) return { days: "", hours: "", minutes: "", unlimited: true };
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  return {
    days: String(days),
    hours: String(hours),
    minutes: minutes > 0 ? String(minutes) : "",
    unlimited: false,
  };
}

export function joinDuration(parts: DurationParts): number | null {
  if (parts.unlimited) return -1;
  if (parts.days.trim() === "" && parts.hours.trim() === "" && parts.minutes.trim() === "") return null;
  const days = Number(parts.days || "0");
  const hours = Number(parts.hours || "0");
  const minutes = Number(parts.minutes || "0");
  if (![days, hours, minutes].every((n) => Number.isFinite(n) && n >= 0)) return null;
  const seconds = Math.round(days * 86400 + hours * 3600 + minutes * 60);
  return seconds > 0 ? seconds : null;
}

export function DurationFields({
  idPrefix,
  label,
  hint,
  seconds,
  disabled,
  onChange,
}: {
  idPrefix: string;
  label: string;
  hint: string;
  seconds: number | null | undefined;
  disabled?: boolean;
  onChange: (seconds: number | null) => void;
}) {
  const { t } = useTranslation("settings");
  const [parts, setParts] = useState<DurationParts>(() => splitDuration(seconds));

  useEffect(() => {
    setParts(splitDuration(seconds));
  }, [seconds]);

  function commit(next: DurationParts) {
    setParts(next);
    onChange(joinDuration(next));
  }

  return (
    <div className="space-y-1">
      <Label>{label}</Label>
      <div className="grid grid-cols-2 gap-2">
        <div className="space-y-1">
          <Label htmlFor={`${idPrefix}-days`} className="text-xs text-muted-foreground">
            {t("sharing.days")}
          </Label>
          <Input
            id={`${idPrefix}-days`}
            data-testid={`${idPrefix}-days`}
            type="number"
            min={0}
            disabled={disabled || parts.unlimited}
            value={parts.unlimited ? "" : parts.days}
            onChange={(e) => commit({ ...parts, days: e.target.value, unlimited: false })}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor={`${idPrefix}-hours`} className="text-xs text-muted-foreground">
            {t("sharing.hours")}
          </Label>
          <Input
            id={`${idPrefix}-hours`}
            data-testid={`${idPrefix}-hours`}
            type="number"
            min={0}
            disabled={disabled || parts.unlimited}
            value={parts.unlimited ? "" : parts.hours}
            onChange={(e) => commit({ ...parts, hours: e.target.value, unlimited: false })}
          />
        </div>
      </div>
      {parts.minutes !== "" && !parts.unlimited && (
        <div className="space-y-1">
          <Label htmlFor={`${idPrefix}-minutes`} className="text-xs text-muted-foreground">
            {t("sharing.minutes")}
          </Label>
          <Input
            id={`${idPrefix}-minutes`}
            data-testid={`${idPrefix}-minutes`}
            type="number"
            min={0}
            disabled={disabled}
            value={parts.minutes}
            onChange={(e) => commit({ ...parts, minutes: e.target.value })}
          />
        </div>
      )}
      <label className="flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          data-testid={`${idPrefix}-unlimited`}
          checked={parts.unlimited}
          disabled={disabled}
          onChange={(e) => commit({ ...parts, unlimited: e.target.checked })}
        />
        {t("sharing.noLimit")}
      </label>
      <p className="text-xs text-muted-foreground">{hint}</p>
    </div>
  );
}
