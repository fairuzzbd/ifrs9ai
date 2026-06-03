import { z } from "zod";

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

export const skenarioEnum = z.enum(["GOOD", "BAD"], {
  error: () => ({ message: "Skenario harus GOOD atau BAD" }),
});

export type Skenario = z.infer<typeof skenarioEnum>;

export const impactMevPdWorkflowStatusEnum = z.enum([
  "DRAFT",
  "PENDING_REVIEW",
  "PENDING_APPROVAL",
  "PENDING_APPROVAL_2",
  "APPROVED",
  "REJECTED",
]);

/** String union of valid workflow states for impact_mev_pd. */
export type ImpactMevPdStatusString = z.infer<typeof impactMevPdWorkflowStatusEnum>;

// ---------------------------------------------------------------------------
// Multiplier decimal validation helper
// ---------------------------------------------------------------------------

/** Validates that a string is a finite positive decimal > 0. */
function isValidDecimalString(val: string): boolean {
  const n = Number(val);
  return !Number.isNaN(n) && Number.isFinite(n) && n > 0;
}

// ---------------------------------------------------------------------------
// Create schema
// ---------------------------------------------------------------------------

export const impactMevPdCreateSchema = z.object({
  periodeId: z
    .string()
    .uuid("Pilih periode buku yang valid")
    .min(1, "Periode buku wajib dipilih"),

  skenario: skenarioEnum,

  impactMultiplier: z
    .string()
    .min(1, "Multiplier wajib diisi")
    .refine(isValidDecimalString, "Multiplier harus berupa angka positif")
    .refine(
      (v) => {
        const n = Number(v);
        return n > 0;
      },
      "Multiplier harus lebih dari 0",
    ),

  mevComponentsJson: z
    .string()
    .optional()
    .refine((v) => {
      if (!v || v.trim() === "") return true;
      try {
        const parsed = JSON.parse(v);
        return parsed !== null && typeof parsed === "object" && !Array.isArray(parsed);
      } catch {
        return false;
      }
    }, "MEV Components harus berupa JSON object yang valid (mis. {\"gdp\": 0.4, \"inflation\": 0.6})"),

  catatan: z
    .string()
    .max(1000, "Catatan maksimal 1000 karakter")
    .optional(),
});

export type ImpactMevPdCreateInput = z.infer<typeof impactMevPdCreateSchema>;

// ---------------------------------------------------------------------------
// Update schema
// ---------------------------------------------------------------------------

export const impactMevPdUpdateSchema = impactMevPdCreateSchema
  .omit({ periodeId: true, skenario: true })
  .extend({
    rowVersion: z.number().int().positive("rowVersion diperlukan untuk update"),
  });

export type ImpactMevPdUpdateInput = z.infer<typeof impactMevPdUpdateSchema>;

// ---------------------------------------------------------------------------
// API response types (derived from backend domain.go)
// ---------------------------------------------------------------------------

export interface ImpactMevPdWorkflowHistoryEntry {
  action: "SUBMIT" | "REVIEW" | "APPROVE" | "APPROVE2" | "REJECT";
  userId: string;
  username: string;
  role: string;
  signedAt: string;
  signatureHash: string;
  comment: string | null;
}

/** Full workflow status object returned from /workflow sub-endpoint. */
export interface ImpactMevPdWorkflowInfo {
  currentState: ImpactMevPdStatusString;
  workflowEyes: 6;
  makerId: string | null;
  reviewerId: string | null;
  approverId: string | null;
  approver2Id: string | null;
  history: ImpactMevPdWorkflowHistoryEntry[];
}

export interface ImpactMevPdItem {
  id: string;
  periodeId: string;
  skenario: Skenario;
  /** Decimal string — DEC-016 */
  impactMultiplier: string;
  mevComponentsJson: string | null;
  catatan: string | null;
  workflowStatus: ImpactMevPdStatusString;
  workflowInstanceId: string | null;
  rowVersion: number;
  createdAt: string;
  createdBy: string | null;
  updatedAt: string | null;
  updatedBy: string | null;
  deletedAt: string | null;
}

export interface ImpactMevPdDetail extends ImpactMevPdItem {
  workflow: ImpactMevPdWorkflowInfo | null;
}
