import { z } from "zod";

// ---------------------------------------------------------------------------
// Enums — from OpenAPI app-d-fx-rate.yaml
// ---------------------------------------------------------------------------

export const kursWorkflowStatusP5Enum = z.enum([
  "PENDING_APPROVAL",
  "APPROVED",
  "REJECTED",
]);
export type KursWorkflowStatusP5 = z.infer<typeof kursWorkflowStatusP5Enum>;

export const sumberKursEnum = z.enum([
  "BI_JISDOR",
  "BI_KURS_TENGAH",
  "INTERNAL",
  "MANUAL",
]);
export type SumberKurs = z.infer<typeof sumberKursEnum>;

export const fxTreatmentRoutingEnum = z.enum([
  "P&L_FOREIGN_EXCHANGE",
  "OCI_FOREIGN_EXCHANGE_RESERVE",
  "OCI_FOREIGN_EXCHANGE_RESERVE_NO_RECYCLING",
  "NO_FX_TREATMENT",
]);
export type FxTreatmentRouting = z.infer<typeof fxTreatmentRoutingEnum>;

export const klasifikasiPsak71Enum = z.enum([
  "AC",
  "FVOCI_DEBT",
  "FVOCI_ELECTION",
  "FVTPL",
  "POCI",
]);
export type KlasifikasiPsak71 = z.infer<typeof klasifikasiPsak71Enum>;

// ---------------------------------------------------------------------------
// Error codes — P5-M5 (5 new codes)
// ---------------------------------------------------------------------------

export const fxRateErrorCodeEnum = z.enum([
  "FX_RATE_LOCKED",
  "KURS_DUPLICATE_DATE",
  "KURS_UPLOAD_VALIDATION_FAILED",
  "KLASIFIKASI_NOT_LOCKED",
  "KURS_PERIODE_MISMATCH",
  // existing cross-module codes also used in P5-M5
  "SOD_VIOLATION",
  "PERIODE_CLOSED",
  "WORKFLOW_INVALID_TRANSITION",
  "VALIDATION_FAILED",
  "FORBIDDEN",
  "NOT_FOUND",
]);
export type FxRateErrorCode = z.infer<typeof fxRateErrorCodeEnum>;

export const FX_RATE_ERROR_MESSAGES: Record<string, string> = {
  FX_RATE_LOCKED:
    "Kurs sudah dikunci karena periode hard-closed. Tidak bisa ditambah atau diubah. Hubungi CFO untuk reopen dalam grace window.",
  KURS_DUPLICATE_DATE:
    "Kurs untuk tanggal dan mata uang ini sudah ada (APPROVED atau PENDING_APPROVAL). Tidak bisa di-override via manual upload.",
  KURS_UPLOAD_VALIDATION_FAILED:
    "File upload gagal validasi. Periksa detail per baris — perbaiki lalu upload ulang.",
  KLASIFIKASI_NOT_LOCKED:
    "Instrumen belum memiliki klasifikasi PSAK 71 yang final (locked). FX treatment tidak dapat ditentukan. Selesaikan SPPI Test + BM Assessment + Klasifikasi Approval terlebih dahulu.",
  KURS_PERIODE_MISMATCH:
    "Tanggal berlaku kurs tidak berada dalam periode buku manapun yang OPEN. Pastikan tanggal benar atau hubungi Finance Controller untuk membuka periode.",
};

// ---------------------------------------------------------------------------
// Label maps
// ---------------------------------------------------------------------------

export const WORKFLOW_STATUS_P5_LABELS: Record<KursWorkflowStatusP5, string> = {
  PENDING_APPROVAL: "Menunggu Approval",
  APPROVED: "Disetujui",
  REJECTED: "Ditolak",
};

export const SUMBER_KURS_LABELS: Record<SumberKurs, string> = {
  BI_JISDOR: "BI JISDOR",
  BI_KURS_TENGAH: "BI Kurs Tengah",
  INTERNAL: "Internal",
  MANUAL: "Manual",
};

export const FX_TREATMENT_LABELS: Record<FxTreatmentRouting, string> = {
  "P&L_FOREIGN_EXCHANGE": "P&L",
  OCI_FOREIGN_EXCHANGE_RESERVE: "OCI (dengan recycling)",
  OCI_FOREIGN_EXCHANGE_RESERVE_NO_RECYCLING: "OCI (tanpa recycling)",
  NO_FX_TREATMENT: "Tidak ada FX",
};

export const FX_TREATMENT_PSAK_REF: Record<FxTreatmentRouting, string> = {
  "P&L_FOREIGN_EXCHANGE": "PSAK 71 — FX ke P&L",
  OCI_FOREIGN_EXCHANGE_RESERVE: "PSAK 71 §5.7.10 — FX ke OCI, di-recycle ke P&L saat derecognition",
  OCI_FOREIGN_EXCHANGE_RESERVE_NO_RECYCLING: "PSAK 71 §5.7.5 — FX ke OCI, irrevocable, tidak di-recycle",
  NO_FX_TREATMENT: "IDR — tidak ada FX exposure",
};

// ---------------------------------------------------------------------------
// KursListItem — from GET /master/kurs response (P5-M5 shape)
// ---------------------------------------------------------------------------

export const kursListItemP5Schema = z.object({
  id: z.string().uuid(),
  fxRateIdKode: z.string(),
  kodeMataUang: z.string().length(3),
  tanggalBerlaku: z.string(), // date string "YYYY-MM-DD"
  kursTengah: z.number(),
  kursBeli: z.number().nullable(),
  kursJual: z.number().nullable(),
  sumberKurs: sumberKursEnum,
  workflowStatus: kursWorkflowStatusP5Enum,
  lockedFlag: z.boolean(),
  deviationFlag: z.boolean(),
  rateDeviationPct: z.number().nullable(),
  periodeKode: z.string().nullable(),
  makerId: z.string().uuid().nullable(),
  approverId: z.string().uuid().nullable(),
  approvedAt: z.string().nullable(),
  rejectReason: z.string().nullable(),
  uploadBatchId: z.string().uuid().nullable(),
  createdAt: z.string(),
  createdBy: z.string().uuid(),
});
export type KursListItemP5 = z.infer<typeof kursListItemP5Schema>;

// ---------------------------------------------------------------------------
// KursDetail — full detail including JISDOR metadata
// ---------------------------------------------------------------------------

export const jisdorFetchMetadataSchema = z.object({
  url: z.string(),
  fetchedAt: z.string(),
  httpStatus: z.number().int(),
  responseHash: z.string().nullable(),
  retryCount: z.number().int(),
}).nullable();

export const kursDetailP5Schema = kursListItemP5Schema.extend({
  periodeBulananId: z.string().uuid().nullable(),
  jisdorFetchMetadata: jisdorFetchMetadataSchema,
  updatedAt: z.string().nullable(),
  updatedBy: z.string().uuid().nullable(),
  rowVersion: z.number().int().positive(),
});
export type KursDetailP5 = z.infer<typeof kursDetailP5Schema>;

// ---------------------------------------------------------------------------
// Upload form — POST /master/kurs/upload (multipart)
// ---------------------------------------------------------------------------

export const kursUploadFormSchema = z.object({
  file: z
    .instanceof(File, { message: "File XLSX atau CSV wajib dipilih" })
    .refine((f) => f.size > 0, "File tidak boleh kosong")
    .refine(
      (f) =>
        [
          "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
          "text/csv",
          "application/csv",
          "application/vnd.ms-excel",
        ].includes(f.type) ||
        f.name.endsWith(".xlsx") ||
        f.name.endsWith(".csv"),
      "Format harus XLSX atau CSV",
    )
    .refine((f) => f.size <= 5 * 1024 * 1024, "Ukuran file maksimal 5 MB"),
  catatanUpload: z.string().max(1000).optional(),
  tanggalBerlakuOverride: z.string().date().optional().or(z.literal("")),
});
export type KursUploadFormInput = z.infer<typeof kursUploadFormSchema>;

// ---------------------------------------------------------------------------
// Upload response
// ---------------------------------------------------------------------------

export const kursCreatedRowSchema = z.object({
  kodeMataUang: z.string(),
  tanggalBerlaku: z.string(),
  kursTengah: z.number(),
  deviationFlag: z.boolean(),
  rateDeviationPct: z.number().nullable(),
});

export const deviationWarningSchema = z.object({
  kodeMataUang: z.string(),
  rateDeviationPct: z.number(),
  previousKursTengah: z.number(),
  message: z.string(),
});

export const kursUploadResponseSchema = z.object({
  uploadBatchId: z.string().uuid(),
  rowsParsed: z.number().int(),
  rowsValid: z.number().int(),
  rowsInvalid: z.number().int(),
  status: z.literal("PENDING_APPROVAL"),
  kursIds: z.array(z.string().uuid()),
  kursCreated: z.array(kursCreatedRowSchema),
  deviationWarnings: z.array(deviationWarningSchema),
  nextStep: z.string(),
  dokumenBuktiId: z.string().uuid().nullable(),
});
export type KursUploadResponse = z.infer<typeof kursUploadResponseSchema>;

// ---------------------------------------------------------------------------
// Approve batch — POST /master/kurs/upload/{batch_id}/approve
// ---------------------------------------------------------------------------

export const kursApproveBodySchema = z.object({
  comment: z
    .string()
    .min(5, "Komentar approval minimal 5 karakter")
    .max(2000, "Komentar maksimal 2000 karakter"),
  signatureMethod: z.literal("JWT_STEP_UP"),
});
export type KursApproveBody = z.infer<typeof kursApproveBodySchema>;

export const kursBatchApproveResponseSchema = z.object({
  uploadBatchId: z.string().uuid(),
  rowsApproved: z.number().int(),
  kursApproved: z.array(
    z.object({
      kursId: z.string().uuid(),
      kodeMataUang: z.string(),
      tanggalBerlaku: z.string(),
      kursTengah: z.number(),
      workflowStatus: z.literal("APPROVED"),
    }),
  ),
  approvedBy: z.string().uuid(),
  approvedAt: z.string(),
  message: z.string(),
});
export type KursBatchApproveResponse = z.infer<typeof kursBatchApproveResponseSchema>;

// ---------------------------------------------------------------------------
// Reject batch — POST /master/kurs/upload/{batch_id}/reject
// ---------------------------------------------------------------------------

export const kursRejectBodySchema = z.object({
  rejectReason: z
    .string()
    .min(20, "Alasan penolakan minimal 20 karakter")
    .max(2000, "Alasan penolakan maksimal 2000 karakter"),
  signatureMethod: z.literal("JWT_STEP_UP"),
});
export type KursRejectBody = z.infer<typeof kursRejectBodySchema>;

export const kursBatchRejectResponseSchema = z.object({
  uploadBatchId: z.string().uuid(),
  rowsRejected: z.number().int(),
  workflowStatus: z.literal("REJECTED"),
  rejectedBy: z.string().uuid(),
  rejectReason: z.string(),
  message: z.string(),
});
export type KursBatchRejectResponse = z.infer<typeof kursBatchRejectResponseSchema>;

// ---------------------------------------------------------------------------
// JISDOR sync trigger — POST /master/kurs/jisdor-sync
// ---------------------------------------------------------------------------

export const jisdorSyncTriggerBodySchema = z.object({
  tanggalTarget: z.string().date().optional().or(z.literal("")),
  forceRefetch: z.boolean().optional(),
});
export type JisdorSyncTriggerBody = z.infer<typeof jisdorSyncTriggerBodySchema>;

export const jisdorSyncJobResponseSchema = z.object({
  jobId: z.string(),
  type: z.literal("JISDOR_SYNC"),
  tanggalTarget: z.string(),
  statusUrl: z.string(),
  streamUrl: z.string(),
  estimatedCurrencies: z.number().int(),
  message: z.string(),
});
export type JisdorSyncJobResponse = z.infer<typeof jisdorSyncJobResponseSchema>;

// ---------------------------------------------------------------------------
// FX Treatment — GET /master/kurs/treatment/{instrumen_id}
// ---------------------------------------------------------------------------

export const fxTreatmentDetailSchema = z.object({
  routing: fxTreatmentRoutingEnum,
  accountType: z.enum(["P&L", "OCI"]).nullable(),
  ociRecycling: z.boolean().nullable(),
  jurnalEventCode: z.string().nullable(),
  psak71Reference: z.string().nullable(),
  notes: z.string().nullable(),
});
export type FxTreatmentDetail = z.infer<typeof fxTreatmentDetailSchema>;

export const kursTreatmentResponseSchema = z.object({
  instrumenId: z.string().uuid(),
  kodeInstrumen: z.string(),
  klasifikasiPsak71: klasifikasiPsak71Enum.nullable(),
  matauang: z.string(),
  klasifikasiLocked: z.boolean(),
  klasifikasiLockedAt: z.string().nullable(),
  fxTreatment: fxTreatmentDetailSchema,
});
export type KursTreatmentResponse = z.infer<typeof kursTreatmentResponseSchema>;
