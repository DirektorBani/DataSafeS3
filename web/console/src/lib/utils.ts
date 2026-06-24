import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";
import i18n, { normalizeLocale } from "@/i18n";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

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
  if (!Number.isFinite(bytes) || bytes <= 0) return `0 ${sizes[0]}`;
  const k = 1024;
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(k)), sizes.length - 1);
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
}

/** Recharts Y-axis tick formatter — keeps chart data in raw bytes. */
export function formatBytesAxis(value: number, locale?: string): string {
  return formatBytes(Number(value), locale);
}

export function formatDate(iso: string, locale?: string): string {
  try {
    return new Date(iso).toLocaleString(resolveLocale(locale));
  } catch {
    return iso;
  }
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
