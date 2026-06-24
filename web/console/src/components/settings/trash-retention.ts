export type TrashRetentionUnit = "days" | "months";

export function trashRetentionDays(value: string, unit: TrashRetentionUnit): number {
  const n = parseInt(value, 10) || 1;
  return unit === "months" ? n * 30 : n;
}

export function initTrashRetentionFromDays(days: number): { value: string; unit: TrashRetentionUnit } {
  if (days % 30 === 0 && days >= 30 && days <= 360) {
    return { value: String(days / 30), unit: "months" };
  }
  return { value: String(days || 30), unit: "days" };
}
