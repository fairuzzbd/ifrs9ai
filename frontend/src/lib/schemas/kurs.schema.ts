import { z } from "zod";

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

export const sumberKursKursEnum = z.enum(
  ["BI_JISDOR", "BI_KURS_TENGAH", "INTERNAL", "MANUAL"],
  { error: () => ({ message: "Pilih sumber kurs yang valid" }) },
);

export const kursWorkflowStatusEnum = z.enum([
  "DRAFT",
  "PENDING_REVIEW",
  "PENDING_APPROVAL",
  "PENDING_APPROVAL_2",
  "APPROVED",
  "RETURNED",
]);

export type KursWorkflowState = z.infer<typeof kursWorkflowStatusEnum>;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * Parse a decimal string.  Returns undefined if the value is undefined/null/empty.
 * Returns a number if parseable, else undefined (caller validates).
 */
function parseOptionalDecimal(val: string | undefined | null): number | undefined {
  if (val === undefined || val === null || val.trim() === "") return undefined;
  const n = parseFloat(val.replace(",", "."));
  return isNaN(n) ? undefined : n;
}

// ---------------------------------------------------------------------------
// Create schema
// ---------------------------------------------------------------------------

export const kursCreateSchema = z
  .object({
    kodeMataUang: z
      .string()
      .regex(/^[A-Z]{3}$/, "Kode mata uang harus 3 huruf kapital (ISO 4217)")
      .refine((v) => v !== "IDR", {
        message: "Kurs IDR tidak diperlukan — IDR adalah mata uang fungsional",
      }),

    tanggalBerlaku: z
      .string()
      .date("Format tanggal tidak valid (YYYY-MM-DD)")
      .refine(
        (val) => {
          const d = new Date(val);
          const maxDate = new Date();
          maxDate.setDate(maxDate.getDate() + 1);
          return d <= maxDate;
        },
        "Tanggal berlaku maksimal besok (H+1)",
      ),

    /** Raw decimal string, optional */
    kursBeli: z.string().optional(),

    /** Raw decimal string, optional */
    kursJual: z.string().optional(),

    /** Raw decimal string, required */
    kursTengah: z
      .string()
      .min(1, "Kurs tengah wajib diisi")
      .refine(
        (v) => {
          const n = parseFloat(v.replace(",", "."));
          return !isNaN(n) && n > 0;
        },
        "Kurs tengah harus angka desimal > 0",
      ),

    sumberKurs: sumberKursKursEnum,
  })
  .superRefine((data, ctx) => {
    const beli = parseOptionalDecimal(data.kursBeli);
    const jual = parseOptionalDecimal(data.kursJual);
    const tengah = parseOptionalDecimal(data.kursTengah);

    if (tengah === undefined) return; // already caught above

    if (beli !== undefined && beli > tengah) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ["kursBeli"],
        message: "Kurs beli harus ≤ kurs tengah",
      });
    }

    if (jual !== undefined && jual < tengah) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ["kursJual"],
        message: "Kurs jual harus ≥ kurs tengah",
      });
    }

    if (beli !== undefined && jual !== undefined && beli > jual) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ["kursBeli"],
        message: "Kurs beli harus ≤ kurs jual",
      });
    }
  });

export type KursCreateInput = z.infer<typeof kursCreateSchema>;

// ---------------------------------------------------------------------------
// Update schema
// ---------------------------------------------------------------------------

export const kursUpdateSchema = z
  .object({
    kursBeli: z.string().optional(),
    kursJual: z.string().optional(),
    kursTengah: z
      .string()
      .min(1, "Kurs tengah wajib diisi")
      .refine(
        (v) => {
          const n = parseFloat(v.replace(",", "."));
          return !isNaN(n) && n > 0;
        },
        "Kurs tengah harus angka desimal > 0",
      ),
    sumberKurs: sumberKursKursEnum,
    rowVersion: z.number().int().positive("rowVersion diperlukan untuk update"),
  })
  .superRefine((data, ctx) => {
    const beli = parseOptionalDecimal(data.kursBeli);
    const jual = parseOptionalDecimal(data.kursJual);
    const tengah = parseOptionalDecimal(data.kursTengah);

    if (tengah === undefined) return;

    if (beli !== undefined && beli > tengah) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ["kursBeli"],
        message: "Kurs beli harus ≤ kurs tengah",
      });
    }

    if (jual !== undefined && jual < tengah) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ["kursJual"],
        message: "Kurs jual harus ≥ kurs tengah",
      });
    }
  });

export type KursUpdateInput = z.infer<typeof kursUpdateSchema>;

// ---------------------------------------------------------------------------
// Workflow action schemas (reuse pattern from mata-uang)
// ---------------------------------------------------------------------------

export const kursWorkflowApproveSchema = z.object({
  comment: z.string().max(1000).optional(),
  signatureMethod: z
    .enum(["JWT_STANDARD", "JWT_STEP_UP"])
    .default("JWT_STANDARD"),
  rowVersion: z.number().int().positive("rowVersion diperlukan"),
  attestChecked: z.literal(true, {
    error: () => ({
      message: "Anda harus mencentang pernyataan ini sebelum melanjutkan",
    }),
  }),
});

export const kursWorkflowRejectSchema = z.object({
  comment: z
    .string()
    .min(10, "Alasan penolakan minimal 10 karakter")
    .max(1000, "Alasan penolakan maksimal 1000 karakter"),
  signatureMethod: z
    .enum(["JWT_STANDARD", "JWT_STEP_UP"])
    .default("JWT_STANDARD"),
  rowVersion: z.number().int().positive("rowVersion diperlukan"),
  attestChecked: z.literal(true, {
    error: () => ({
      message: "Anda harus mencentang pernyataan ini sebelum melanjutkan",
    }),
  }),
});

// ---------------------------------------------------------------------------
// API response types
// ---------------------------------------------------------------------------

export interface KursItem {
  id: string;
  fxRateIdKode: string;
  kodeMataUang: string;
  tanggalBerlaku: string; // "YYYY-MM-DD"
  kursBeli: string | null;
  kursJual: string | null;
  kursTengah: string;
  sumberKurs: "BI_JISDOR" | "BI_KURS_TENGAH" | "INTERNAL" | "MANUAL";
  periodeBulananId: string;
  lockedFlag: boolean;
  workflowStatus: KursWorkflowState;
  workflowInstanceId: string | null;
  rowVersion: number;
  createdAt: string;
  createdBy: string | null;
  updatedAt: string | null;
  updatedBy: string | null;
  deletedAt: string | null;
}

export interface KursWorkflowHistoryEntry {
  action: "SUBMIT" | "REVIEW" | "APPROVE" | "REJECT";
  userId: string;
  username: string;
  role: string;
  signedAt: string;
  signatureHash: string;
  comment: string | null;
}

export interface KursWorkflowStatus {
  currentState: KursWorkflowState;
  workflowEyes: 4 | 6;
  makerId: string | null;
  reviewerId: string | null;
  approverId: string | null;
  history: KursWorkflowHistoryEntry[];
}

export interface KursDetail extends KursItem {
  workflow: KursWorkflowStatus | null;
}

// ---------------------------------------------------------------------------
// IDR formatting utilities
// ---------------------------------------------------------------------------

/**
 * Format a decimal string as IDR display: "Rp 15.432,1234"
 * Uses id-ID locale with 4 decimal places for kurs values.
 */
export function formatKursTengah(value: string | null | undefined): string {
  if (!value) return "—";
  const n = parseFloat(value);
  if (isNaN(n)) return value;
  return new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    minimumFractionDigits: 4,
    maximumFractionDigits: 4,
  }).format(n);
}

/**
 * Format a decimal string for table display: "15.432,1234" (no symbol for space saving)
 */
export function formatKursTable(value: string | null | undefined): string {
  if (!value) return "—";
  const n = parseFloat(value);
  if (isNaN(n)) return value;
  return new Intl.NumberFormat("id-ID", {
    minimumFractionDigits: 4,
    maximumFractionDigits: 4,
  }).format(n);
}

/**
 * Parse displayed IDR string back to raw decimal string for form input.
 * Strips "Rp", spaces, and converts id-ID decimal comma to dot.
 */
export function parseKursDisplay(display: string): string {
  return display
    .replace(/Rp\s*/g, "")
    .replace(/\./g, "")   // remove thousands separator (dot in id-ID)
    .replace(",", ".")    // decimal comma → dot
    .trim();
}

/** Sumber kurs labels */
export const SUMBER_KURS_LABELS: Record<string, string> = {
  BI_JISDOR: "BI JISDOR",
  BI_KURS_TENGAH: "BI Kurs Tengah",
  INTERNAL: "Internal",
  MANUAL: "Manual",
};
