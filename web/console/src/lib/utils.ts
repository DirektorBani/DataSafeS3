import i18n, { normalizeLocale } from "@/i18n";
import { formatBytes as formatByteCount, formatDate as formatLocaleDate, type ByteUnits } from "@datasafe/ui/format";

export { cn } from "@datasafe/ui/cn";

function resolveLocale(locale?: string): string {
  return normalizeLocale(locale ?? i18n.language);
}

function byteUnits(locale: string): string[] {
  return [
    i18n.t("common:units.bytes", { lng: locale }),
    i18n.t("common:units.kilobytes", { lng: locale }),
    i18n.t("common:units.megabytes", { lng: locale }),
    i18n.t("common:units.gigabytes", { lng: locale }),
    i18n.t("common:units.terabytes", { lng: locale }),
  ];
}

export function formatBytes(bytes: number, locale?: string): string {
  const lng = resolveLocale(locale);
  const sizes = byteUnits(lng);
  const units: ByteUnits = {
    bytes: sizes[0],
    kilobytes: sizes[1],
    megabytes: sizes[2],
    gigabytes: sizes[3],
    terabytes: sizes[4],
  };
  return formatByteCount(bytes, units);
}

/** Recharts Y-axis tick formatter — keeps chart data in raw bytes. */
export function formatBytesAxis(value: number, locale?: string): string {
  return formatBytes(Number(value), locale);
}

export function formatDate(iso: string, locale?: string): string {
  return formatLocaleDate(iso, resolveLocale(locale));
}

export function formatSpeed(bytesPerSec: number, locale?: string): string {
  const lng = resolveLocale(locale);
  if (!Number.isFinite(bytesPerSec) || bytesPerSec <= 0) {
    return i18n.t("common:duration.empty", { lng });
  }
  return `${formatBytes(bytesPerSec, lng)}/s`;
}

export function formatDuration(seconds: number, locale?: string): string {
  const lng = resolveLocale(locale);
  if (!Number.isFinite(seconds) || seconds <= 0) {
    return i18n.t("common:duration.empty", { lng });
  }
  if (seconds < 60) {
    return i18n.t("common:duration.seconds", { lng, count: Math.round(seconds) });
  }
  const m = Math.floor(seconds / 60);
  const s = Math.round(seconds % 60);
  return i18n.t("common:duration.minutesSeconds", { lng, minutes: m, seconds: s });
}
