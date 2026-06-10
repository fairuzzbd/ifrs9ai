import { z } from "zod";

// ---------------------------------------------------------------------------
// Re-use workflow enums from mata-uang (they are domain-agnostic)
// ---------------------------------------------------------------------------

export const workflowStatusEnum = z.enum([
  "DRAFT",
  "PENDING_REVIEW",
  "PENDING_APPROVAL",
  "PENDING_APPROVAL_2",
  "APPROVED",
  "RETURNED",
]);

export type LPSWorkflowState = z.infer<typeof workflowStatusEnum>;

// ---------------------------------------------------------------------------
// Decimal string validation helper
// Keeps coverage_amount as string to avoid float64 precision loss (DEC-016).
// ---------------------------------------------------------------------------

const decimalPositiveString = z
  .string()
  .min(1, "Jumlah wajib diisi")
  .regex(
    /^\d+(\.\d{1,4})?$/,
    "Format tidak valid. Gunakan angka, contoh: 2000000000.0000",
  )
  .refine(
    (val) => parseFloat(val) > 0,
    "Jumlah coverage harus lebih dari 0",
  );

// ---------------------------------------------------------------------------
// Create schema
// ---------------------------------------------------------------------------

export const lpsCoverageCreateSchema = z
  .object({
    coverageAmount: decimalPositiveString,
    // mata_uang is always IDR per DEC-014, sent as read-only info
    periodeBerlakuDari: z
      .string()
      .date("Format tanggal tidak valid (YYYY-MM-DD)")
      .min(1, "Tanggal mulai berlaku wajib diisi"),
    periodeBerlakuSampai: z
      .string()
      .date("Format tanggal tidak valid (YYYY-MM-DD)")
      .nullable()
      .optional(),
    regulasiReferensi: z
      .string()
      .max(200, "Referensi regulasi maksimal 200 karakter")
      .optional()
      .nullable(),
  })
  .refine(
    (data) => {
      if (!data.periodeBerlakuSampai) return true;
      return data.periodeBerlakuDari <= data.periodeBerlakuSampai;
    },
    {
      message: "Tanggal akhir tidak boleh sebelum tanggal mulai",
      path: ["periodeBerlakuSampai"],
    },
  );

export type LPSCoverageCreateInput = z.infer<typeof lpsCoverageCreateSchema>;

// ---------------------------------------------------------------------------
// Update schema
// ---------------------------------------------------------------------------

export const lpsCoverageUpdateSchema = lpsCoverageCreateSchema.extend({
  rowVersion: z
    .number()
    .int()
    .positive("rowVersion diperlukan untuk update"),
});

export type LPSCoverageUpdateInput = z.infer<typeof lpsCoverageUpdateSchema>;

// ---------------------------------------------------------------------------
// Workflow action schemas (identical to mata-uang pattern)
// ---------------------------------------------------------------------------

export const workflowApproveSchema = z.object({
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

export type WorkflowApproveInput = z.infer<typeof workflowApproveSchema>;

export const workflowRejectSchema = z.object({
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

export type WorkflowRejectInput = z.infer<typeof workflowRejectSchema>;

export const workflowSubmitSchema = z.object({
  comment: z.string().max(500).optional(),
  rowVersion: z.number().int().positive("rowVersion diperlukan"),
});

export type WorkflowSubmitInput = z.infer<typeof workflowSubmitSchema>;

// ---------------------------------------------------------------------------
// API response types
// ---------------------------------------------------------------------------

export interface WorkflowHistoryEntry {
  action: "SUBMIT" | "REVIEW" | "APPROVE" | "APPROVE2" | "REJECT";
  userId: string;
  username: string;
  role: string;
  signedAt: string;
  signatureHash: string;
  comment: string | null;
}

export interface WorkflowStatus {
  currentState: LPSWorkflowState;
  workflowEyes: 4 | 6;
  makerId: string | null;
  reviewerId: string | null;
  approverId: string | null;
  approver2Id: string | null;
  history: WorkflowHistoryEntry[];
}

export interface LPSCoverageItem {
  id: string;
  coverageAmount: string;         // decimal string, never number
  mataUang: "IDR";               // always IDR per DEC-014
  periodeBerlakuDari: string;       // date string YYYY-MM-DD
  periodeBerlakuSampai: string | null; // null = currently active
  regulasiReferensi: string | null;
  workflowStatus: LPSWorkflowState;
  workflowInstanceId: string | null;
  rowVersion: number;
  createdAt: string;
  createdBy: string;
  updatedAt: string;
  updatedBy: string;
  deletedAt: string | null;
}

export interface LPSCoverageDetail extends LPSCoverageItem {
  workflow: WorkflowStatus | null;
}

// ---------------------------------------------------------------------------
// IDR formatter helpers — never use Number for monetary display
// ---------------------------------------------------------------------------

/**
 * Format a decimal string to IDR display: "Rp 2.000.000.000,00"
 * Uses Intl.NumberFormat id-ID with currency IDR.
 * Input: string like "2000000000.0000"
 */
export function formatIDR(amountStr: string): string {
  // Parse as float for display only (display precision, not computation)
  const parsed = parseFloat(amountStr);
  if (isNaN(parsed)) return amountStr;
  return new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(parsed);
}

/**
 * Strip formatting from a display string back to plain decimal.
 * Removes "Rp", dots (thousands), spaces, replaces comma with dot.
 */
export function parseIDRInput(displayVal: string): string {
  return displayVal
    .replace(/Rp\s?/g, "")
    .replace(/\./g, "")
    .replace(/,/g, ".")
    .trim();
}

/** Default coverage amount per DEC-014 */
export const DEFAULT_COVERAGE_AMOUNT = "2000000000.0000";
