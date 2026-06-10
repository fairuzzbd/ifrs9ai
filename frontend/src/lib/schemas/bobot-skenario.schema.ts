import { z } from "zod";
import type { WorkflowHistoryEntry, WorkflowStatus } from "@/lib/schemas/mata-uang.schema";

// Re-export shared workflow types
export type { WorkflowHistoryEntry, WorkflowStatus };
export type { MasterWorkflowState } from "@/lib/schemas/mata-uang.schema";

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

export const skenarioEclEnum = z.enum(["GOOD", "NORMAL", "BAD"]);
export type SkenarioEcl = z.infer<typeof skenarioEclEnum>;

export const SKENARIO_ECL_LABELS: Record<SkenarioEcl, string> = {
  GOOD: "Optimis (Good)",
  NORMAL: "Dasar (Normal)",
  BAD: "Pesimis (Bad)",
};

export const SKENARIO_ECL_SHORT: Record<SkenarioEcl, string> = {
  GOOD: "G",
  NORMAL: "N",
  BAD: "B",
};

/**
 * Default DEC-010 weights: Good 0.25 / Normal 0.50 / Bad 0.25
 */
export const DEFAULT_WEIGHTS: Record<SkenarioEcl, string> = {
  GOOD: "25.00",
  NORMAL: "50.00",
  BAD: "25.00",
};

// ---------------------------------------------------------------------------
// Weight string helpers (DEC-016 — no float64)
// ---------------------------------------------------------------------------

/**
 * Convert display percentage string (e.g. "25.00") to API decimal string (e.g. "0.25000000").
 * Returns null if input is empty/invalid.
 */
export function bobotPercentToDecimal(pct: string): string | null {
  const trimmed = pct.trim();
  if (trimmed === "") return null;
  const num = parseFloat(trimmed);
  if (isNaN(num)) return null;
  // Divide by 100 and round to 8 decimal places (NUMERIC(10,8))
  const decimal = (num / 100).toFixed(8);
  return decimal;
}

/**
 * Convert API decimal string (e.g. "0.25000000") to display percentage string (e.g. "25.00").
 */
export function bobotDecimalToPercent(dec: string | null | undefined): string {
  if (!dec) return "";
  const num = parseFloat(dec);
  if (isNaN(num)) return "";
  return (num * 100).toFixed(2);
}

/**
 * Sum three bobot decimal strings. Returns a string with 8 decimal places.
 * Returns null if any value is invalid.
 */
export function sumBobotDecimals(
  good: string | null,
  normal: string | null,
  bad: string | null,
): string | null {
  const values = [good, normal, bad];
  let sum = 0;
  for (const v of values) {
    if (!v) return null;
    const n = parseFloat(v);
    if (isNaN(n)) return null;
    sum += n;
  }
  return sum.toFixed(8);
}

/**
 * Check if sum equals 1.0 within a small tolerance (float comparison).
 */
export function isSumValid(sum: string | null): boolean {
  if (!sum) return false;
  const n = parseFloat(sum);
  return Math.abs(n - 1.0) < 1e-7;
}

// ---------------------------------------------------------------------------
// Create schema (UI form input) — one row at a time
// ---------------------------------------------------------------------------

export const bobotSkenarioCreateSchema = z
  .object({
    skenario: skenarioEclEnum,
    // bobot state = string percent, validated 0-100 (DEC-016)
    bobotPersen: z
      .string()
      .min(1, "Bobot wajib diisi")
      .refine((v) => {
        const n = parseFloat(v);
        return !isNaN(n) && n >= 0 && n <= 100;
      }, "Bobot harus antara 0 dan 100 persen"),
    periodeBerlakuDari: z
      .string()
      .date("Format tanggal tidak valid (YYYY-MM-DD)")
      .min(1, "Periode berlaku dari wajib diisi"),
    periodeBerlakuSampai: z
      .string()
      .date("Format tanggal tidak valid (YYYY-MM-DD)")
      .optional()
      .or(z.literal("")),
    catatan: z.string().max(2000, "Maksimal 2000 karakter").optional(),
  })
  .superRefine((data, ctx) => {
    if (
      data.periodeBerlakuSampai &&
      data.periodeBerlakuSampai !== "" &&
      data.periodeBerlakuDari
    ) {
      if (data.periodeBerlakuSampai < data.periodeBerlakuDari) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ["periodeBerlakuSampai"],
          message:
            "Periode berlaku sampai harus sama atau setelah periode berlaku dari",
        });
      }
    }
  });

export type BobotSkenarioCreateInput = z.infer<typeof bobotSkenarioCreateSchema>;

// ---------------------------------------------------------------------------
// Update schema
// ---------------------------------------------------------------------------

export const bobotSkenarioUpdateSchema = bobotSkenarioCreateSchema.extend({
  rowVersion: z.number().int().positive("rowVersion diperlukan untuk update"),
});

export type BobotSkenarioUpdateInput = z.infer<typeof bobotSkenarioUpdateSchema>;

// ---------------------------------------------------------------------------
// Filter schema (URL state)
// ---------------------------------------------------------------------------

export const bobotSkenarioFilterSchema = z.object({
  q: z.string().optional(),
  sort: z.string().optional(),
  "filter[skenario]": z.string().optional(),
  "filter[workflow_status]": z.string().optional(),
  "filter[periode_berlaku_dari]": z.string().optional(),
  cursor: z.string().optional(),
  limit: z.number().int().min(1).max(200).optional(),
});

export type BobotSkenarioFilterInput = z.infer<typeof bobotSkenarioFilterSchema>;

// ---------------------------------------------------------------------------
// API response types
// ---------------------------------------------------------------------------

export interface BobotSkenarioItem {
  id: string;
  skenario: SkenarioEcl;
  bobot: string; // decimal string e.g. "0.25000000" (NUMERIC(10,8))
  periodeBerlakuDari: string; // date YYYY-MM-DD
  periodeBerlakuSampai: string | null; // date YYYY-MM-DD or null
  catatan: string | null;
  workflowStatus: import("@/lib/schemas/mata-uang.schema").MasterWorkflowState;
  workflowInstanceId: string | null;
  rowVersion: number;
  createdAt: string;
  createdBy: string;
  updatedAt: string;
  updatedBy: string;
  deletedAt: string | null;
}

export interface BobotSkenarioDetail extends BobotSkenarioItem {
  workflow: WorkflowStatus | null;
}

// ---------------------------------------------------------------------------
// Trio analysis types (for sum=1.0 indicator)
// ---------------------------------------------------------------------------

export interface TrioSkenario {
  periodeBerlakuDari: string;
  good: BobotSkenarioItem | null;
  normal: BobotSkenarioItem | null;
  bad: BobotSkenarioItem | null;
  sum: string | null;
  isValid: boolean;
  isComplete: boolean;
}

/**
 * Group a flat list of BobotSkenarioItem by periodeBerlakuDari,
 * returning the latest trio (sorted by periodeBerlakuDari desc).
 */
export function groupIntoTrios(items: BobotSkenarioItem[]): TrioSkenario[] {
  const map = new Map<string, TrioSkenario>();

  for (const item of items) {
    const key = item.periodeBerlakuDari;
    if (!map.has(key)) {
      map.set(key, {
        periodeBerlakuDari: key,
        good: null,
        normal: null,
        bad: null,
        sum: null,
        isValid: false,
        isComplete: false,
      });
    }
    const trio = map.get(key)!;
    if (item.skenario === "GOOD") trio.good = item;
    else if (item.skenario === "NORMAL") trio.normal = item;
    else if (item.skenario === "BAD") trio.bad = item;
  }

  // Compute sum + validity for each trio
  for (const trio of map.values()) {
    const goodDec = trio.good?.bobot ?? null;
    const normalDec = trio.normal?.bobot ?? null;
    const badDec = trio.bad?.bobot ?? null;
    trio.isComplete = !!trio.good && !!trio.normal && !!trio.bad;
    trio.sum = sumBobotDecimals(goodDec, normalDec, badDec);
    trio.isValid = trio.isComplete && isSumValid(trio.sum);
  }

  // Sort by periodeBerlakuDari descending
  return Array.from(map.values()).sort(
    (a, b) =>
      b.periodeBerlakuDari.localeCompare(a.periodeBerlakuDari),
  );
}
