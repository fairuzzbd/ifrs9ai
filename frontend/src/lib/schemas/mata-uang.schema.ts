import { z } from "zod";

export const sumberKursEnum = z.enum(
  ["BI_JISDOR", "BI_KURS_TENGAH", "INTERNAL"],
  { error: () => ({ message: "Pilih sumber kurs yang valid" }) },
);

export const frekuensiUpdateEnum = z.enum(
  ["HARIAN", "INTRA_DAY", "BULANAN"],
  { error: () => ({ message: "Pilih frekuensi update yang valid" }) },
);

export const workflowStatusEnum = z.enum([
  "DRAFT",
  "PENDING_REVIEW",
  "PENDING_APPROVAL",
  "PENDING_APPROVAL_2",
  "APPROVED",
  "RETURNED",
]);

export type MasterWorkflowState = z.infer<typeof workflowStatusEnum>;

export const mataUangCreateSchema = z.object({
  kodeMataUang: z
    .string()
    .regex(
      /^[A-Z]{3}$/,
      "Kode mata uang harus 3 huruf kapital sesuai ISO 4217 (contoh: IDR, USD, EUR)",
    ),
  namaMataUang: z
    .string()
    .min(3, "Nama mata uang minimal 3 karakter")
    .max(60, "Nama mata uang maksimal 60 karakter"),
  simbol: z
    .string()
    .min(1, "Simbol wajib diisi")
    .max(5, "Simbol maksimal 5 karakter"),
  decimalPlaces: z
    .number({ error: () => ({ message: "Decimal places harus berupa angka" }) })
    .int("Decimal places harus bilangan bulat")
    .min(0, "Decimal places minimal 0")
    .max(4, "Decimal places maksimal 4"),
  sumberKursDefault: sumberKursEnum,
  frekuensiUpdate: frekuensiUpdateEnum,
  tanggalMulaiAktif: z
    .string()
    .date("Format tanggal tidak valid (YYYY-MM-DD)")
    .refine(
      (val) => new Date(val) <= new Date(),
      "Tanggal mulai aktif tidak boleh di masa depan",
    ),
  aktifFlag: z.boolean(),
});

export type MataUangCreateInput = z.infer<typeof mataUangCreateSchema>;

export const mataUangUpdateSchema = mataUangCreateSchema
  .omit({ kodeMataUang: true })
  .extend({
    rowVersion: z
      .number()
      .int()
      .positive("rowVersion diperlukan untuk update"),
  });

export type MataUangUpdateInput = z.infer<typeof mataUangUpdateSchema>;

export const workflowApproveSchema = z.object({
  comment: z.string().max(1000).optional(),
  signatureMethod: z
    .enum(["JWT_STANDARD", "JWT_STEP_UP"])
    .default("JWT_STANDARD"),
  rowVersion: z.number().int().positive("rowVersion diperlukan"),
  attestChecked: z.literal(true, {
    error: () => ({ message: "Anda harus mencentang pernyataan ini sebelum melanjutkan" }),
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
    error: () => ({ message: "Anda harus mencentang pernyataan ini sebelum melanjutkan" }),
  }),
});

export type WorkflowRejectInput = z.infer<typeof workflowRejectSchema>;

export const workflowSubmitSchema = z.object({
  comment: z.string().max(500).optional(),
  rowVersion: z.number().int().positive("rowVersion diperlukan"),
});

export type WorkflowSubmitInput = z.infer<typeof workflowSubmitSchema>;

// ---------------------------------------------------------------------------
// API response types (derived from OpenAPI)
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
  currentState: MasterWorkflowState;
  workflowEyes: 4 | 6;
  makerId: string | null;
  reviewerId: string | null;
  approverId: string | null;
  history: WorkflowHistoryEntry[];
}

export interface MataUangItem {
  kodeMataUang: string;
  namaMataUang: string;
  simbol: string;
  decimalPlaces: number;
  sumberKursDefault: "BI_JISDOR" | "BI_KURS_TENGAH" | "INTERNAL";
  frekuensiUpdate: "HARIAN" | "INTRA_DAY" | "BULANAN";
  aktifFlag: boolean;
  tanggalMulaiAktif: string;
  isSystemCurrency: boolean;
  workflowStatus: MasterWorkflowState;
  workflowInstanceId: string | null;
  rowVersion: number;
  createdAt: string;
  createdBy: string;
  updatedAt: string;
  updatedBy: string;
  deletedAt: string | null;
}

export interface MataUangDetail extends MataUangItem {
  workflow: WorkflowStatus | null;
}
