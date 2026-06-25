import { z } from "zod";

// ---------------------------------------------------------------------------
// Enums — from OpenAPI app-b-mtm.yaml
// ---------------------------------------------------------------------------

export const mtmStatusEnum = z.enum([
  "AUTO_POSTED",
  "PENDING_REVIEW",
  "APPROVED",
  "REJECTED",
  "STALE_PRICE",
]);
export type MtmStatus = z.infer<typeof mtmStatusEnum>;

export const hargaSumberEnum = z.enum([
  "IBPA",
  "BEI",
  "KSEI",
  "MANUAL",
  "IBPA_MANUAL",
  "BEI_MANUAL",
]);
export type HargaSumber = z.infer<typeof hargaSumberEnum>;

export const mtmKlasifikasiEnum = z.enum([
  "FVOCI_DEBT",
  "FVTPL",
  "FVOCI_ELECTION",
  "POCI",
]);
export type MtmKlasifikasi = z.infer<typeof mtmKlasifikasiEnum>;

export const stalePriceReasonEnum = z.enum([
  "HARGA_TIDAK_TERSEDIA",
  "KURS_FCY_TIDAK_TERSEDIA",
]);
export type StalePriceReason = z.infer<typeof stalePriceReasonEnum>;

// ---------------------------------------------------------------------------
// Error codes — P5-M6 (6 new codes)
// ---------------------------------------------------------------------------

export const mtmErrorCodeEnum = z.enum([
  "MTM_PRICE_STALE",
  "MTM_PRICE_DEVIATION_REJECTED",
  "MTM_BATCH_NOT_FOUND",
  "MTM_OVERRIDE_SOD_VIOLATION",
  "MTM_INSTRUMEN_AC_SKIP",
  "MTM_PERIODE_LOCKED",
  // shared cross-module codes
  "SOD_VIOLATION",
  "WORKFLOW_INVALID_TRANSITION",
  "VALIDATION_FAILED",
  "FORBIDDEN",
  "NOT_FOUND",
  "CONFLICT",
  "RATE_LIMITED",
]);
export type MtmErrorCode = z.infer<typeof mtmErrorCodeEnum>;

// ---------------------------------------------------------------------------
// Label maps
// ---------------------------------------------------------------------------

export const MTM_STATUS_LABELS: Record<MtmStatus, string> = {
  AUTO_POSTED: "Auto Diposting",
  PENDING_REVIEW: "Menunggu Review",
  APPROVED: "Disetujui",
  REJECTED: "Ditolak",
  STALE_PRICE: "Harga Kedaluwarsa",
};

export const HARGA_SUMBER_LABELS: Record<HargaSumber, string> = {
  IBPA: "IBPA",
  BEI: "BEI",
  KSEI: "KSEI",
  MANUAL: "Manual",
  IBPA_MANUAL: "IBPA (Manual)",
  BEI_MANUAL: "BEI (Manual)",
};

export const MTM_KLASIFIKASI_LABELS: Record<MtmKlasifikasi, string> = {
  FVOCI_DEBT: "FVOCI Utang",
  FVTPL: "FVTPL",
  FVOCI_ELECTION: "FVOCI Ekuitas",
  POCI: "POCI",
};

export const JURNAL_EVENT_CODE_LABELS: Record<string, string> = {
  MTM_FVOCI: "OCI Nilai Wajar",
  MTM_FX_OCI_RESERVE: "OCI FX Reserve",
  MTM_FVOCI_ELECTION: "OCI Ekuitas",
  MTM_FVTPL: "P&L Fair Value",
  MTM_FVTPL_POCI: "P&L POCI",
};

// ---------------------------------------------------------------------------
// MtmListItem — from GET /trx/mtm
// ---------------------------------------------------------------------------

export const mtmListItemSchema = z.object({
  id: z.string().uuid(),
  instrumenId: z.string().uuid(),
  instrumenKode: z.string(),
  instrumenNama: z.string(),
  tanggalMtm: z.string(), // date string "YYYY-MM-DD"
  hargaSumber: hargaSumberEnum,
  hargaPasarIdr: z.number(),
  hargaBukuIdr: z.number(),
  deltaIdr: z.number(),
  deltaPct: z.number(),
  hargaAgeDays: z.number().int(),
  stalePriceFlag: z.boolean(),
  deviationFlag: z.boolean(),
  status: mtmStatusEnum,
  klasifikasiSnapshot: mtmKlasifikasiEnum,
  jurnalEventCode: z.string().nullable(),
  jurnalEntryId: z.string().uuid().nullable(),
  uploaderId: z.string().uuid().nullable(),
  overrideApproverId: z.string().uuid().nullable(),
  overrideAt: z.string().nullable(),
  lockedFlag: z.boolean(),
  createdAt: z.string(),
});
export type MtmListItem = z.infer<typeof mtmListItemSchema>;

// ---------------------------------------------------------------------------
// MtmDetail — full detail including FCY + override info
// ---------------------------------------------------------------------------

export const mtmDetailSchema = mtmListItemSchema.extend({
  periodeBulananId: z.string().uuid(),
  hargaTanggal: z.string(),
  hargaPasarFcy: z.number().nullable(),
  kursId: z.string().uuid().nullable(),
  kursTengah: z.number().nullable(),
  treatmentSnapshot: z.string().nullable(),
  jurnalEventCodes: z.array(z.string()).nullable(),
  uploadBatchId: z.string().uuid().nullable(),
  overrideComment: z.string().nullable(),
  cronJobId: z.string().nullable(),
  createdBy: z.string().uuid(),
  updatedAt: z.string(),
  updatedBy: z.string().uuid(),
  rowVersion: z.number().int().positive(),
});
export type MtmDetail = z.infer<typeof mtmDetailSchema>;

// ---------------------------------------------------------------------------
// MtmUploadForm — POST /trx/mtm/upload/batch (multipart)
// ---------------------------------------------------------------------------

export const mtmUploadFormSchema = z.object({
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
    .refine((f) => f.size <= 10 * 1024 * 1024, "Ukuran file maksimal 10 MB"),
  catatanUpload: z.string().max(1000).optional(),
  tanggalMtmOverride: z.string().date().optional().or(z.literal("")),
});
export type MtmUploadFormInput = z.infer<typeof mtmUploadFormSchema>;

// ---------------------------------------------------------------------------
// Upload response
// ---------------------------------------------------------------------------

export const mtmCreatedRowSchema = z.object({
  instrumenKode: z.string(),
  tanggalMtm: z.string(),
  hargaPasarFcy: z.number().nullable(),
  hargaPasarIdr: z.number(),
  hargaSumber: z.string(),
  deviationFlag: z.boolean(),
  deltaPct: z.number(),
  stalePriceFlag: z.boolean(),
});
export type MtmCreatedRow = z.infer<typeof mtmCreatedRowSchema>;

export const mtmDeviationWarningSchema = z.object({
  instrumenKode: z.string(),
  deltaPct: z.number(),
  thresholdPct: z.number(),
  message: z.string(),
});
export type MtmDeviationWarning = z.infer<typeof mtmDeviationWarningSchema>;

export const mtmUploadBatchResponseSchema = z.object({
  uploadBatchId: z.string().uuid(),
  rowsParsed: z.number().int(),
  rowsValid: z.number().int(),
  rowsInvalid: z.number().int(),
  status: z.literal("PENDING_REVIEW"),
  mtmIds: z.array(z.string().uuid()),
  rowsCreated: z.array(mtmCreatedRowSchema),
  stalePriceWarnings: z.array(z.string()),
  deviationWarnings: z.array(mtmDeviationWarningSchema),
  nextStep: z.string(),
});
export type MtmUploadBatchResponse = z.infer<typeof mtmUploadBatchResponseSchema>;

// ---------------------------------------------------------------------------
// Batch detail — GET /trx/mtm/upload/batch/{batch_id}
// ---------------------------------------------------------------------------

export const mtmBatchRowSchema = z.object({
  lineNumber: z.number().int(),
  mtmId: z.string().uuid(),
  instrumenKode: z.string(),
  instrumenId: z.string().uuid(),
  tanggalMtm: z.string(),
  hargaPasarFcy: z.number().nullable(),
  hargaPasarIdr: z.number(),
  deltaPct: z.number(),
  deviationFlag: z.boolean(),
  stalePriceFlag: z.boolean(),
  rowStatus: mtmStatusEnum,
  rowErrorMsg: z.string().nullable(),
});
export type MtmBatchRow = z.infer<typeof mtmBatchRowSchema>;

export const mtmUploadBatchDetailSchema = z.object({
  uploadBatchId: z.string().uuid(),
  uploaderId: z.string().uuid(),
  uploaderName: z.string(),
  catatanUpload: z.string().nullable(),
  rowsParsed: z.number().int(),
  rowsValid: z.number().int(),
  rowsInvalid: z.number().int(),
  status: mtmStatusEnum,
  createdAt: z.string(),
  rows: z.array(mtmBatchRowSchema),
});
export type MtmUploadBatchDetail = z.infer<typeof mtmUploadBatchDetailSchema>;

// ---------------------------------------------------------------------------
// Override approve — POST /trx/mtm/{id}/override-approve
// ---------------------------------------------------------------------------

export const mtmOverrideApproveSchema = z.object({
  comment: z
    .string()
    .min(30, "Komentar persetujuan wajib minimal 30 karakter")
    .max(2000, "Komentar maksimal 2000 karakter"),
  signatureMethod: z.literal("JWT_STEP_UP"),
  attest: z.literal(true, {
    error: "Anda wajib mencentang pernyataan verifikasi harga",
  }),
});
export type MtmOverrideApproveInput = z.infer<typeof mtmOverrideApproveSchema>;

export const mtmOverrideApproveResponseSchema = z.object({
  mtmId: z.string().uuid(),
  instrumenKode: z.string(),
  status: z.literal("APPROVED"),
  jurnalEntryId: z.string().uuid().nullable(),
  jurnalEventCodes: z.array(z.string()).nullable(),
  approvedBy: z.string().uuid(),
  approvedAt: z.string(),
  message: z.string(),
});
export type MtmOverrideApproveResponse = z.infer<typeof mtmOverrideApproveResponseSchema>;

// ---------------------------------------------------------------------------
// Override reject — POST /trx/mtm/{id}/override-reject
// ---------------------------------------------------------------------------

export const mtmOverrideRejectSchema = z.object({
  comment: z
    .string()
    .min(30, "Alasan penolakan wajib minimal 30 karakter (S4-AC4)")
    .max(2000, "Alasan penolakan maksimal 2000 karakter"),
  signatureMethod: z.literal("JWT_STEP_UP"),
});
export type MtmOverrideRejectInput = z.infer<typeof mtmOverrideRejectSchema>;

export const mtmOverrideRejectResponseSchema = z.object({
  mtmId: z.string().uuid(),
  instrumenKode: z.string(),
  status: z.literal("REJECTED"),
  rejectedBy: z.string().uuid(),
  rejectedAt: z.string(),
  comment: z.string(),
  message: z.string(),
});
export type MtmOverrideRejectResponse = z.infer<typeof mtmOverrideRejectResponseSchema>;

// ---------------------------------------------------------------------------
// Cron trigger — POST /trx/mtm/cron/trigger
// ---------------------------------------------------------------------------

export const mtmCronTriggerSchema = z.object({
  tanggalTarget: z
    .string()
    .date("Format tanggal harus YYYY-MM-DD")
    .refine(
      (d) => new Date(d) <= new Date(),
      "Tanggal target tidak boleh tanggal yang akan datang",
    )
    .optional()
    .or(z.literal("")),
  forceRerun: z.boolean().default(false),
});
export type MtmCronTriggerInput = z.infer<typeof mtmCronTriggerSchema>;

export const mtmCronJobResponseSchema = z.object({
  jobId: z.string(),
  type: z.literal("MTM_DAILY_RUN"),
  tanggalTarget: z.string(),
  statusUrl: z.string(),
  streamUrl: z.string(),
  estimatedInstrumen: z.number().int(),
  message: z.string(),
});
export type MtmCronJobResponse = z.infer<typeof mtmCronJobResponseSchema>;

// ---------------------------------------------------------------------------
// Stale price alerts — GET /trx/mtm/alerts/stale-price
// ---------------------------------------------------------------------------

export const mtmStaleAlertItemSchema = z.object({
  id: z.string().uuid(),
  instrumenId: z.string().uuid(),
  instrumenKode: z.string(),
  instrumenNama: z.string(),
  klasifikasiSnapshot: mtmKlasifikasiEnum,
  tanggalMtm: z.string(),
  hargaTanggal: z.string(),
  hargaAgeDays: z.number().int(),
  status: mtmStatusEnum,
  stalePriceReason: stalePriceReasonEnum,
  esklasiasiFlag: z.boolean(),
  lockedFlag: z.boolean(),
  uploaderId: z.string().uuid().nullable(),
});
export type MtmStaleAlertItem = z.infer<typeof mtmStaleAlertItemSchema>;

// ---------------------------------------------------------------------------
// Async export job response
// ---------------------------------------------------------------------------

export const mtmAsyncExportJobResponseSchema = z.object({
  jobId: z.string(),
  type: z.literal("MTM_EXPORT"),
  statusUrl: z.string(),
  streamUrl: z.string(),
  message: z.string(),
});
export type MtmAsyncExportJobResponse = z.infer<typeof mtmAsyncExportJobResponseSchema>;
