import { z } from "zod";

// ---------------------------------------------------------------------------
// Workflow status
// ---------------------------------------------------------------------------

export const impactPdWorkflowStatusEnum = z.enum([
  "DRAFT",
  "PENDING_REVIEW",
  "PENDING_APPROVAL",
  "PENDING_APPROVAL_2",
  "APPROVED",
  "REJECTED",
]);

/** String union of valid workflow states for impact_pd. */
export type ImpactPdStatusString = z.infer<typeof impactPdWorkflowStatusEnum>;

// ---------------------------------------------------------------------------
// Multiplier decimal validation
// DB-enforced range: [0.5, 2.0] — hard reject outside this range (service returns 422).
// We mirror this in client-side Zod so users see the error immediately.
// ---------------------------------------------------------------------------

function isValidDecimalString(val: string): boolean {
  const n = Number(val);
  return !Number.isNaN(n) && Number.isFinite(n);
}

/** range [0.5, 2.0] — mirrors DB CHECK constraint */
function isInRange(val: string): boolean {
  const n = Number(val);
  return n >= 0.5 && n <= 2.0;
}

// ---------------------------------------------------------------------------
// Create schema
// ---------------------------------------------------------------------------

export const impactPdCreateSchema = z.object({
  periodeId: z
    .string()
    .uuid("Pilih periode buku yang valid")
    .min(1, "Periode buku wajib dipilih"),

  impactMultiplier: z
    .string()
    .min(1, "Multiplier wajib diisi")
    .refine(isValidDecimalString, "Multiplier harus berupa angka")
    .refine(isInRange, "Multiplier harus berada dalam rentang 0.5 – 2.0 (DEC-010)"),

  catatan: z
    .string()
    .max(1000, "Catatan maksimal 1000 karakter")
    .optional(),
});

export type ImpactPdCreateInput = z.infer<typeof impactPdCreateSchema>;

// ---------------------------------------------------------------------------
// Update schema
// ---------------------------------------------------------------------------

export const impactPdUpdateSchema = impactPdCreateSchema
  .omit({ periodeId: true })
  .extend({
    rowVersion: z.number().int().positive("rowVersion diperlukan untuk update"),
  });

export type ImpactPdUpdateInput = z.infer<typeof impactPdUpdateSchema>;

// ---------------------------------------------------------------------------
// API response types (derived from backend domain.go)
// ---------------------------------------------------------------------------

export interface ImpactPdWorkflowHistoryEntry {
  action: "SUBMIT" | "REVIEW" | "APPROVE" | "APPROVE2" | "REJECT";
  userId: string;
  username: string;
  role: string;
  signedAt: string;
  signatureHash: string;
  comment: string | null;
}

/** Full workflow status object returned from /workflow sub-endpoint. */
export interface ImpactPdWorkflowInfo {
  currentState: ImpactPdStatusString;
  workflowEyes: 6;
  makerId: string | null;
  reviewerId: string | null;
  approverId: string | null;
  approver2Id: string | null;
  history: ImpactPdWorkflowHistoryEntry[];
}

export interface ImpactPdItem {
  id: string;
  periodeId: string;
  /** Decimal string — DEC-016 */
  impactMultiplier: string;
  catatan: string | null;
  workflowStatus: ImpactPdStatusString;
  workflowInstanceId: string | null;
  rowVersion: number;
  createdAt: string;
  createdBy: string | null;
  updatedAt: string | null;
  updatedBy: string | null;
  deletedAt: string | null;
}

export interface ImpactPdDetail extends ImpactPdItem {
  workflow: ImpactPdWorkflowInfo | null;
}
