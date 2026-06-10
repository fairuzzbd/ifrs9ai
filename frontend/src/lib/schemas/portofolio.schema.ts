import { z } from "zod";

// ---------------------------------------------------------------------------
// Shared enums
// ---------------------------------------------------------------------------

export const bmCategoryEnum = z.enum(["HTC", "HTCS", "OTHER"], {
  error: () => ({
    message: "Pilih kategori Business Model yang valid (HTC, HTCS, atau OTHER)",
  }),
});

export type BmCategory = z.infer<typeof bmCategoryEnum>;

export const workflowStatusEnum = z.enum([
  "DRAFT",
  "PENDING_REVIEW",
  "PENDING_APPROVAL",
  "APPROVED",
  "RETURNED",
]);

export type PortofolioWorkflowState = z.infer<typeof workflowStatusEnum>;

// ---------------------------------------------------------------------------
// Create schema
// ---------------------------------------------------------------------------

export const portofolioCreateSchema = z.object({
  kodePortofolio: z
    .string()
    .min(1, "Kode portofolio wajib diisi")
    .max(20, "Kode portofolio maksimal 20 karakter")
    .regex(
      /^[A-Z0-9_]{1,20}$/,
      "Kode portofolio hanya boleh huruf kapital, angka, dan underscore (contoh: BOND_HTM_IDR)",
    ),
  nama: z
    .string()
    .min(3, "Nama portofolio minimal 3 karakter")
    .max(200, "Nama portofolio maksimal 200 karakter"),
  tujuanPengelolaan: z
    .string()
    .min(10, "Tujuan pengelolaan minimal 10 karakter")
    .max(2000, "Tujuan pengelolaan maksimal 2000 karakter"),
  bmCategoryDefault: bmCategoryEnum,
  benchmark: z
    .string()
    .max(500, "Benchmark maksimal 500 karakter")
    .optional()
    .nullable(),
  kompensasiManagerBasis: z
    .string()
    .max(500, "Kompensasi manager basis maksimal 500 karakter")
    .optional()
    .nullable(),
  periodeReviewTerakhir: z
    .string()
    .date("Format tanggal tidak valid (YYYY-MM-DD)")
    .optional()
    .nullable(),
  aktifFlag: z.boolean(),
});

export type PortofolioCreateInput = z.infer<typeof portofolioCreateSchema>;

// ---------------------------------------------------------------------------
// Update schema (add rowVersion for optimistic lock)
// ---------------------------------------------------------------------------

export const portofolioUpdateSchema = portofolioCreateSchema
  .omit({ kodePortofolio: true })
  .extend({
    rowVersion: z.number().int().positive("rowVersion diperlukan untuk update"),
  });

export type PortofolioUpdateInput = z.infer<typeof portofolioUpdateSchema>;

// ---------------------------------------------------------------------------
// Workflow action schemas (same pattern as mata-uang)
// ---------------------------------------------------------------------------

export const workflowApproveSchema = z.object({
  comment: z.string().max(1000).optional(),
  signatureMethod: z
    .enum(["JWT_STANDARD", "JWT_STEP_UP"])
    .default("JWT_STANDARD"),
  rowVersion: z.number().int().positive("rowVersion diperlukan"),
  attestChecked: z.literal(true, {
    error: () => ({
      message:
        "Anda harus mencentang pernyataan ini sebelum melanjutkan",
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
      message:
        "Anda harus mencentang pernyataan ini sebelum melanjutkan",
    }),
  }),
});

export type WorkflowRejectInput = z.infer<typeof workflowRejectSchema>;

// ---------------------------------------------------------------------------
// API response types
// ---------------------------------------------------------------------------

export interface WorkflowHistoryEntry {
  action: "SUBMIT" | "REVIEW" | "APPROVE" | "REJECT";
  userId: string;
  username: string;
  role: string;
  signedAt: string;
  signatureHash: string;
  comment: string | null;
}

export interface WorkflowStatus {
  currentState: PortofolioWorkflowState;
  workflowEyes: 4 | 6;
  makerId: string | null;
  reviewerId: string | null;
  approverId: string | null;
  history: WorkflowHistoryEntry[];
}

export interface PortofolioItem {
  id: string;
  kodePortofolio: string;
  nama: string;
  tujuanPengelolaan: string;
  bmCategoryDefault: BmCategory;
  benchmark: string | null;
  kompensasiManagerBasis: string | null;
  periodeReviewTerakhir: string | null;
  aktifFlag: boolean;
  workflowStatus: PortofolioWorkflowState;
  workflowInstanceId: string | null;
  rowVersion: number;
  createdAt: string;
  createdBy: string;
  updatedAt: string;
  updatedBy: string;
  deletedAt: string | null;
}

export interface PortofolioDetail extends PortofolioItem {
  workflow: WorkflowStatus | null;
}

// ---------------------------------------------------------------------------
// BM category display helpers
// ---------------------------------------------------------------------------

export const BM_CATEGORY_LABEL: Record<BmCategory, string> = {
  HTC: "HTC — Hold-to-Collect",
  HTCS: "HTCS — Hold-to-Collect-and-Sell",
  OTHER: "OTHER — Trading/Lain-lain",
};

export const BM_CATEGORY_PSAK71: Record<BmCategory, string> = {
  HTC: "Amortised Cost (AC)",
  HTCS: "FVOCI debt",
  OTHER: "FVTPL",
};
