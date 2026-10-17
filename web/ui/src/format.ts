// Shared with the web console. Size keeps one decimal and drops a trailing .0.
// Date is the locale default, for example "10/17/2026, 1:02:54 PM".

export type ByteUnits = {
  bytes: string;
  kilobytes: string;
  megabytes: string;
  gigabytes: string;
  terabytes: string;
};

export function formatBytes(bytes: number, units: ByteUnits): string {
  const sizes = [units.bytes, units.kilobytes, units.megabytes, units.gigabytes, units.terabytes];
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return `0 ${sizes[0]}`;
  }
  const k = 1024;
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(k)), sizes.length - 1);
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
}

export function formatDate(iso: string, locale: string): string {
  try {
    return new Date(iso).toLocaleString(locale);
  } catch {
    return iso;
  }
}
