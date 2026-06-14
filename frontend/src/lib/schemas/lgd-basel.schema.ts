import { z } from "zod";
import type { WorkflowHistoryEntry, WorkflowStatus } from "@/lib/schemas/mata-uang.schema";

// Re-export shared workflow types
export type { WorkflowHistoryEntry, WorkflowStatus };
export type { MasterWorkflowState } from "@/lib/schemas/mata-uang.schema";

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

export const tipeEksposurEnum = z.enum([
  "SOVEREIGN",
  "BANK",
  "CORPORATE",
  "RETAIL",
  "EQUITY",
  "REINSURANCE",
]);

export type TipeEksposur = z.infer<typeof tipeEksposurEnum>;

export const TIPE_EKSPOSUR_LABELS: Record<TipeEksposur, string> = {
  SOVEREIGN: "Pemerintah",
  BANK: "Bank",
  CORPORATE: "Korporasi",
  RETAIL: "Ritel",
  EQUITY: "Ekuitas",
  REINSURANCE: "Reasuransi",
};

// ---------------------------------------------------------------------------
// LGD decimal helpers
// ---------------------------------------------------------------------------
// LGD stored as NUMERIC(8,4) decimal 0–1 (e.g. 0.4550).
// UI shows as percentage 0–100 (e.g. 45.50).
// We keep lgd as string internally to avoid float precision issues (DEC-016).

/**
 * Convert display percentage string (e.g. "45.50") to API decimal string (e.g. "0.4550").
 * Returns null if input is empty/invalid.
 */
export function lgdPercentToDecimal(pct: string): string | null {
  const trimmed = pct.trim();
  if (trimmed === "") return null;
  const num = parseFloat(trimmed);
  if (isNaN(num)) return null;
  // Divide by 100 and round to 4 decimal places
  const decimal = (num / 100).toFixed(4);
  return decimal;
}

/**
 * Convert API decimal string (e.g. "0.4550") to display percentage string (e.g. "45.50").
 */
export function lgdDecimalToPercent(dec: string | null | undefined): string {
  if (!dec) return "";
  const num = parseFloat(dec);
  if (isNaN(num)) return "";
  return (num * 100).toFixed(2);
}

// ---------------------------------------------------------------------------
// Create schema (UI form input)
// ---------------------------------------------------------------------------

export const lgdBaselCreateSchema = z
  .object({
    tipeEksposur: tipeEksposurEnum,
    // lgd: stored as string %, validated 0-100
    lgdPersen: z
      .string()
      .min(1, "LGD wajib diisi")
      .refine((v) => {
        const n = parseFloat(v);
        return !isNaN(n) && n >= 0 && n <= 100;
      }, "LGD harus antara 0 dan 100 persen"),
    karakteristik: z.string().max(2000, "Maksimal 2000 karakter").optional(),
    periodeBerlakuDari: z
      .string()
      .date("Format tanggal tidak valid (YYYY-MM-DD)")
      .min(1, "Periode berlaku dari wajib diisi"),
    periodeBerlakuSampai: z
      .string()
      .date("Format tanggal tidak valid (YYYY-MM-DD)")
      .optional()
      .or(z.literal("")),
    sumber: z.string().min(1, "Sumber wajib diisi").max(100, "Maksimal 100 karakter"),
    dokumenPendukungId: z
      .string()
      .uuid("Format UUID tidak valid")
      .optional()
      .or(z.literal("")),
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

export type LGDBaselCreateInput = z.infer<typeof lgdBaselCreateSchema>;

// ---------------------------------------------------------------------------
// Update schema
// ---------------------------------------------------------------------------

export const lgdBaselUpdateSchema = lgdBaselCreateSchema.extend({
  rowVersion: z.number().int().positive("rowVersion diperlukan untuk update"),
});

export type LGDBaselUpdateInput = z.infer<typeof lgdBaselUpdateSchema>;

// ---------------------------------------------------------------------------
// Filter schema (URL state)
// ---------------------------------------------------------------------------

export const lgdBaselFilterSchema = z.object({
  q: z.string().optional(),
  sort: z.string().optional(),
  "filter[tipe_eksposur]": z.string().optional(),
  "filter[sumber]": z.string().optional(),
  "filter[workflow_status]": z.string().optional(),
  "filter[aktif]": z.string().optional(),
  cursor: z.string().optional(),
  limit: z.number().int().min(1).max(200).optional(),
});

export type LGDBaselFilterInput = z.infer<typeof lgdBaselFilterSchema>;

// ---------------------------------------------------------------------------
// API response types
// ---------------------------------------------------------------------------

export interface LGDBaselItem {
  id: string;
  tipeEksposur: TipeEksposur;
  lgd: string; // decimal string e.g. "0.4550"
  karakteristik: string | null;
  periodeBerlakuDari: string; // date YYYY-MM-DD
  periodeBerlakuSampai: string | null; // date YYYY-MM-DD or null
  sumber: string;
  dokumenPendukungId: string | null;
  workflowStatus: import("@/lib/schemas/mata-uang.schema").MasterWorkflowState;
  workflowInstanceId: string | null;
  rowVersion: number;
  createdAt: string;
  createdBy: string;
  updatedAt: string;
  updatedBy: string;
  deletedAt: string | null;
}

export interface LGDBaselDetail extends LGDBaselItem {
  workflow: WorkflowStatus | null;
}
