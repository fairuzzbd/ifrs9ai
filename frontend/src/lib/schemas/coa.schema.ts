import { z } from "zod";

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

export const tipeAkunEnum = z.enum(
  ["ASET", "LIABILITAS", "EKUITAS", "PENDAPATAN", "BEBAN", "KONTINJEN"],
  { error: () => ({ message: "Pilih tipe akun yang valid" }) },
);

export const posisiNormalEnum = z.enum(["DEBIT", "KREDIT"], {
  error: () => ({ message: "Pilih posisi normal yang valid" }),
});

export const workflowStatusEnum = z.enum([
  "DRAFT",
  "PENDING_REVIEW",
  "PENDING_APPROVAL",
  "PENDING_APPROVAL_2",
  "APPROVED",
  "RETURNED",
]);

export type TipeAkun = z.infer<typeof tipeAkunEnum>;
export type PosisiNormal = z.infer<typeof posisiNormalEnum>;
export type CoAWorkflowState = z.infer<typeof workflowStatusEnum>;

// ---------------------------------------------------------------------------
// Create / update
// ---------------------------------------------------------------------------

export const coaCreateSchema = z.object({
  kodeAkun: z
    .string()
    .min(1, "Kode akun wajib diisi")
    .max(30, "Kode akun maksimal 30 karakter")
    .regex(
      /^\d+(\.\d+)*$/,
      "Format kode akun tidak valid. Gunakan angka dengan pemisah titik, contoh: 1.1.01.001",
    ),
  namaAkun: z
    .string()
    .min(3, "Nama akun minimal 3 karakter")
    .max(120, "Nama akun maksimal 120 karakter"),
  tipeAkun: tipeAkunEnum,
  subTipeAkun: z
    .string()
    .max(80, "Sub tipe akun maksimal 80 karakter")
    .optional(),
  kategoriInvestasi: z
    .string()
    .max(80, "Kategori investasi maksimal 80 karakter")
    .optional(),
  matauangNative: z
    .string()
    .length(3, "Kode mata uang harus 3 karakter")
    .regex(/^[A-Z]{3}$/, "Kode mata uang harus 3 huruf kapital (ISO 4217)")
    .default("IDR"),
  posisiNormal: posisiNormalEnum,
  aktifFlag: z.boolean().default(true),
  parentAkunId: z.string().uuid("ID parent akun tidak valid").nullable().optional(),
  sumberCoa: z
    .string()
    .min(2, "Sumber CoA minimal 2 karakter")
    .max(100, "Sumber CoA maksimal 100 karakter"),
  tanggalMulaiAktif: z
    .string()
    .date("Format tanggal tidak valid (YYYY-MM-DD)")
    .refine(
      (val) => new Date(val) <= new Date(),
      "Tanggal mulai aktif tidak boleh di masa depan",
    ),
});

export type CoACreateInput = z.infer<typeof coaCreateSchema>;

export const coaUpdateSchema = coaCreateSchema
  .omit({ kodeAkun: true })
  .extend({
    rowVersion: z
      .number()
      .int()
      .positive("rowVersion diperlukan untuk update"),
  });

export type CoAUpdateInput = z.infer<typeof coaUpdateSchema>;

// ---------------------------------------------------------------------------
// Workflow action schemas (reusing same pattern as mata-uang)
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
// Import-related types
// ---------------------------------------------------------------------------

export const coaImportSchema = z.object({
  file: z
    .instanceof(typeof window !== "undefined" ? File : Object, {
      message: "File XLSX wajib dipilih",
    })
    .refine(
      (f) =>
        f instanceof File &&
        (f.type ===
          "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" ||
          f.name.endsWith(".xlsx")),
      "Hanya file XLSX yang diizinkan",
    )
    .refine(
      (f) => f instanceof File && f.size <= 10 * 1024 * 1024,
      "Ukuran file maksimal 10MB",
    ),
  sumberCoa: z
    .string()
    .min(2, "Sumber CoA minimal 2 karakter")
    .max(100, "Sumber CoA maksimal 100 karakter"),
});

export type CoAImportInput = z.infer<typeof coaImportSchema>;

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
  currentState: CoAWorkflowState;
  workflowEyes: 4 | 6;
  makerId: string | null;
  reviewerId: string | null;
  approverId: string | null;
  history: WorkflowHistoryEntry[];
}

export interface CoAItem {
  id: string;
  kodeAkun: string;
  namaAkun: string;
  tipeAkun: TipeAkun;
  subTipeAkun: string | null;
  kategoriInvestasi: string | null;
  matauangNative: string;
  posisiNormal: PosisiNormal;
  aktifFlag: boolean;
  parentAkunId: string | null;
  parentKodeAkun: string | null;
  parentNamaAkun: string | null;
  sumberCoa: string;
  tanggalMulaiAktif: string;
  workflowStatus: CoAWorkflowState;
  workflowInstanceId: string | null;
  rowVersion: number;
  createdAt: string;
  createdBy: string;
  updatedAt: string;
  updatedBy: string;
  deletedAt: string | null;
}

export interface CoADetail extends CoAItem {
  workflow: WorkflowStatus | null;
  ancestors: Array<{ id: string; kodeAkun: string; namaAkun: string }>;
}

export interface CoAImportJobResult {
  rowsCreated: number;
  rowsSkipped: number;
  errors: Array<{ row: number; kodeAkun: string; reason: string }>;
}

export interface CoAImportJob {
  jobId: string;
  type: "COA_IMPORT_XLSX";
  status: "queued" | "running" | "completed" | "failed" | "cancelled";
  progress: number;
  currentStep: string | null;
  startedAt: string | null;
  estimatedCompletionAt: string | null;
  result: CoAImportJobResult | null;
  error: { code: string; message: string } | null;
  canCancel: boolean;
  createdBy: string;
}
