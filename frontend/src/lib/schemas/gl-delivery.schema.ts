import { z } from "zod";

// ---------------------------------------------------------------------------
// Enums — derived from OpenAPI app-d-gl-delivery.yaml
// ---------------------------------------------------------------------------

export const glHostStatusEnum = z.enum([
  "PENDING_DELIVERY",
  "DELIVERY_IN_FLIGHT",
  "DELIVERED",
  "RETRYING",
  "FAILED",
  "DEAD_LETTER",
]);
export type GlHostStatus = z.infer<typeof glHostStatusEnum>;

export const glFailureCategoryEnum = z.enum(["DOMAIN", "INFRA"]);
export type GlFailureCategory = z.infer<typeof glFailureCategoryEnum>;

export const glDeliveryModeEnum = z.enum(["API", "BATCH_FILE"]);
export type GlDeliveryMode = z.infer<typeof glDeliveryModeEnum>;

export const reconReportStatusEnum = z.enum([
  "PENDING",
  "RUNNING",
  "COMPLETED",
  "COMPLETED_WITH_MISMATCH",
  "FAILED",
]);
export type ReconReportStatus = z.infer<typeof reconReportStatusEnum>;

export const mismatchTypeEnum = z.enum(["BLIPS_ONLY", "GL_ONLY", "AMOUNT_DIFF"]);
export type MismatchType = z.infer<typeof mismatchTypeEnum>;

// ---------------------------------------------------------------------------
// Error codes — P5-M3 (16 error codes from state machine doc)
// ---------------------------------------------------------------------------

export const glDeliveryErrorCodeEnum = z.enum([
  "GL_DELIVERY_JURNAL_NOT_FOUND",
  "GL_DELIVERY_REASON_TOO_SHORT",
  "GL_DELIVERY_INVALID_TRANSITION",
  "GL_DELIVERY_MAX_ATTEMPTS_EXCEEDED",
  "GL_DELIVERY_PERMISSION_DENIED",
  "GL_DELIVERY_HOST_4XX",
  "GL_DELIVERY_HOST_UNREACHABLE",
  "GL_DLQ_REPLAY_INVALID_STATE",
  "GL_RECONCILIATION_REPORT_NOT_FOUND",
  "GL_RECONCILIATION_DATE_INVALID",
  "GL_RECONCILIATION_IN_PROGRESS",
  "GL_RECONCILIATION_HOST_FETCH_FAILED",
  "WORKFLOW_INVALID_TRANSITION",
  "VALIDATION_FAILED",
  "FORBIDDEN",
  "NOT_FOUND",
]);
export type GlDeliveryErrorCode = z.infer<typeof glDeliveryErrorCodeEnum>;

// ---------------------------------------------------------------------------
// API Response shapes — S2: GL Delivery Status
// ---------------------------------------------------------------------------

export const manualRetryHistoryItemSchema = z.object({
  retriedBy: z.string().uuid(),
  retriedAt: z.string().datetime({ offset: true }),
  reason: z.string(),
});
export type ManualRetryHistoryItem = z.infer<typeof manualRetryHistoryItemSchema>;

export const glDeliveryStatusSchema = z.object({
  glStatusId: z.string().uuid(),
  glHostStatus: glHostStatusEnum,
  glHostJournalId: z.string().nullable().optional(),
  deliveredAt: z.string().datetime({ offset: true }).nullable().optional(),
  retryCount: z.number().int().min(0),
  lastRetryAt: z.string().datetime({ offset: true }).nullable().optional(),
  lastError: z.string().nullable().optional(),
  failureCategory: glFailureCategoryEnum.nullable().optional(),
  deliveryMode: glDeliveryModeEnum,
  payloadSentAt: z.string().datetime({ offset: true }).nullable().optional(),
  canRetry: z.boolean(),
  glResponsePayloadJsonb: z.record(z.string(), z.unknown()).nullable().optional(),
  manualRetryHistory: z.array(manualRetryHistoryItemSchema).nullable().optional(),
});
export type GlDeliveryStatus = z.infer<typeof glDeliveryStatusSchema>;

// ---------------------------------------------------------------------------
// S3: Retry GL Delivery
// ---------------------------------------------------------------------------

export const retryGlDeliveryRequestSchema = z.object({
  reason: z
    .string()
    .min(30, "Alasan retry minimal 30 karakter")
    .max(1000, "Alasan retry maksimal 1000 karakter"),
});
export type RetryGlDeliveryRequest = z.infer<typeof retryGlDeliveryRequestSchema>;

export const retryGlDeliveryResponseSchema = z.object({
  jobId: z.string(),
  statusUrl: z.string(),
  glStatusId: z.string().uuid(),
  previousStatus: glHostStatusEnum,
  newStatus: glHostStatusEnum,
  retryAttemptNumber: z.number().int().min(1),
});
export type RetryGlDeliveryResponse = z.infer<typeof retryGlDeliveryResponseSchema>;

// ---------------------------------------------------------------------------
// S4: Reconciliation
// ---------------------------------------------------------------------------

export const runReconciliationRequestSchema = z.object({
  date: z
    .string()
    .regex(/^\d{4}-\d{2}-\d{2}$/, "Format tanggal harus YYYY-MM-DD"),
  reason: z.string().max(500).optional(),
});
export type RunReconciliationRequest = z.infer<typeof runReconciliationRequestSchema>;

export const runReconciliationResponseSchema = z.object({
  jobId: z.string(),
  statusUrl: z.string(),
  streamUrl: z.string().optional(),
  tanggalRekonsiliasi: z.string(),
});
export type RunReconciliationResponse = z.infer<typeof runReconciliationResponseSchema>;

export const glReconMismatchLineSchema = z.object({
  kodeAkun: z.string(),
  namaAkun: z.string().nullable().optional(),
  blipsAmountIdr: z.number(),
  glHostAmountIdr: z.number(),
  deltaIdr: z.number(),
  mismatchType: mismatchTypeEnum,
  jurnalHeaderIds: z.array(z.string().uuid()),
});
export type GlReconMismatchLine = z.infer<typeof glReconMismatchLineSchema>;

export const glReconciliationReportSchema = z.object({
  reportId: z.string().uuid(),
  tanggalRekonsiliasi: z.string(),
  status: reconReportStatusEnum,
  totalAkunChecked: z.number().int().min(0),
  totalMismatchCount: z.number().int().min(0),
  totalMismatchAmountIdr: z.number().optional().nullable(),
  blipsTotalIdr: z.number(),
  glHostTotalIdr: z.number(),
  deltaIdr: z.number(),
  toleranceIdr: z.number(),
  generatedAt: z.string().datetime({ offset: true }),
  jobId: z.string().nullable().optional(),
  mismatchLines: z.array(glReconMismatchLineSchema).optional(),
});
export type GlReconciliationReport = z.infer<typeof glReconciliationReportSchema>;

export const glReconciliationSummaryItemSchema = z.object({
  reportId: z.string().uuid(),
  tanggalRekonsiliasi: z.string(),
  status: reconReportStatusEnum,
  totalAkunChecked: z.number().int().min(0).optional(),
  totalMismatchCount: z.number().int().min(0),
  totalMismatchAmountIdr: z.number().nullable().optional(),
  deltaIdr: z.number().nullable().optional(),
  generatedAt: z.string().datetime({ offset: true }).nullable().optional(),
  jobId: z.string().nullable().optional(),
});
export type GlReconciliationSummaryItem = z.infer<typeof glReconciliationSummaryItemSchema>;

// ---------------------------------------------------------------------------
// S5: GL Delivery DLQ (uses GlHostStatus, not a separate DlqStatus)
// ---------------------------------------------------------------------------

export const dlqActionRequestSchema = z.object({
  reason: z
    .string()
    .min(30, "Alasan minimal 30 karakter")
    .max(1000, "Alasan maksimal 1000 karakter"),
});
export type DlqActionRequest = z.infer<typeof dlqActionRequestSchema>;

export const glDeliveryDlqListItemSchema = z.object({
  dlqEntryId: z.string().uuid(),
  jurnalHeaderId: z.string().uuid(),
  noJurnal: z.string(),
  eventCode: z.string().optional(),
  tanggalPosting: z.string().optional(),
  glHostStatus: glHostStatusEnum,     // FAILED or DEAD_LETTER for DLQ entries
  failureCategory: glFailureCategoryEnum,
  errorCode: z.string(),
  errorMessage: z.string().nullable().optional(),
  retryCount: z.number().int().min(0),
  lastRetryAt: z.string().datetime({ offset: true }).nullable().optional(),
  createdAt: z.string().datetime({ offset: true }),
  canReplay: z.boolean(),
  canDiscard: z.boolean(),
});
export type GlDeliveryDlqListItem = z.infer<typeof glDeliveryDlqListItemSchema>;

export const dlqErrorHistoryItemSchema = z.object({
  eventTime: z.string().datetime({ offset: true }),
  action: z.enum(["GL_DELIVERY.FAILED", "GL_DELIVERY.RETRY"]),
  attempt: z.number().int().optional(),
  errorCode: z.string().optional(),
  errorMessage: z.string().optional(),
});
export type DlqErrorHistoryItem = z.infer<typeof dlqErrorHistoryItemSchema>;

export const dlqDiscardInfoSchema = z.object({
  discardedBy: z.string().uuid(),
  discardedAt: z.string().datetime({ offset: true }),
  discardReason: z.string(),
});
export type DlqDiscardInfo = z.infer<typeof dlqDiscardInfoSchema>;

export const glDeliveryDlqDetailSchema = glDeliveryDlqListItemSchema.extend({
  payloadSnapshotJsonb: z.record(z.string(), z.unknown()).nullable().optional(),
  glResponsePayloadJsonb: z.record(z.string(), z.unknown()).nullable().optional(),
  errorHistory: z.array(dlqErrorHistoryItemSchema).optional(),
  manualRetryHistory: z.array(manualRetryHistoryItemSchema).optional(),
  discardInfo: dlqDiscardInfoSchema.nullable().optional(),
});
export type GlDeliveryDlqDetail = z.infer<typeof glDeliveryDlqDetailSchema>;

export const dlqReplayResponseSchema = z.object({
  jobId: z.string(),
  statusUrl: z.string(),
  dlqEntryId: z.string().uuid(),
  jurnalHeaderId: z.string().uuid(),
  noJurnal: z.string(),
  previousStatus: glHostStatusEnum,
  newStatus: glHostStatusEnum,
});
export type DlqReplayResponse = z.infer<typeof dlqReplayResponseSchema>;

export const dlqDiscardResponseSchema = z.object({
  dlqEntryId: z.string().uuid(),
  jurnalHeaderId: z.string().uuid(),
  noJurnal: z.string(),
  previousStatus: glHostStatusEnum,
  newStatus: glHostStatusEnum,
  discardedAt: z.string().datetime({ offset: true }),
  discardedBy: z.string().uuid(),
});
export type DlqDiscardResponse = z.infer<typeof dlqDiscardResponseSchema>;

// ---------------------------------------------------------------------------
// Human-readable labels (Bahasa Indonesia)
// ---------------------------------------------------------------------------

export const GL_HOST_STATUS_LABELS: Record<GlHostStatus, string> = {
  PENDING_DELIVERY: "Menunggu Pengiriman",
  DELIVERY_IN_FLIGHT: "Sedang Dikirim",
  DELIVERED: "Terkirim ke GL",
  RETRYING: "Sedang Retry",
  FAILED: "Gagal — Delivery",
  DEAD_LETTER: "Dihentikan — DLQ",
};

export const RECON_STATUS_LABELS: Record<ReconReportStatus, string> = {
  PENDING: "Menunggu",
  RUNNING: "Berjalan",
  COMPLETED: "Sesuai",
  COMPLETED_WITH_MISMATCH: "Ada Mismatch",
  FAILED: "Gagal",
};

export const MISMATCH_TYPE_LABELS: Record<MismatchType, string> = {
  BLIPS_ONLY: "Hanya di BLIPS",
  GL_ONLY: "Hanya di GL Host",
  AMOUNT_DIFF: "Selisih Jumlah",
};

export const FAILURE_CATEGORY_LABELS: Record<GlFailureCategory, string> = {
  DOMAIN: "Domain Error",
  INFRA: "Infra Error",
};

// ---------------------------------------------------------------------------
// Notify error messages (P5-M3 specific) — merged into lib/notify.ts ERROR_MESSAGE_MAP
// ---------------------------------------------------------------------------

export const GL_DELIVERY_ERROR_MESSAGES: Record<string, string> = {
  GL_DELIVERY_JURNAL_NOT_FOUND:
    "Jurnal header tidak ditemukan.",
  GL_DELIVERY_REASON_TOO_SHORT:
    "Alasan retry / discard wajib minimal 30 karakter.",
  GL_DELIVERY_INVALID_TRANSITION:
    "Jurnal memiliki status DEAD_LETTER dan tidak dapat di-retry. Hubungi ROLE-IT-ADMIN.",
  GL_DELIVERY_MAX_ATTEMPTS_EXCEEDED:
    "Batas maksimum percobaan delivery tercapai (5/5). Hubungi ROLE-IT-ADMIN untuk tindak lanjut.",
  GL_DELIVERY_PERMISSION_DENIED:
    "Anda tidak memiliki izin untuk tindakan ini. Hanya ROLE-IT-ADMIN yang dapat mendiscard entry GL Delivery DLQ.",
  GL_DLQ_REPLAY_INVALID_STATE:
    "DLQ entry tidak bisa di-replay dari status saat ini. Pastikan entry berstatus FAILED.",
  GL_RECONCILIATION_REPORT_NOT_FOUND:
    "Belum ada laporan rekonsiliasi untuk tanggal ini.",
  GL_RECONCILIATION_DATE_INVALID:
    "Tanggal tidak valid atau merupakan hari libur. Rekonsiliasi hanya untuk hari kerja.",
  GL_RECONCILIATION_IN_PROGRESS:
    "Rekonsiliasi untuk tanggal ini sedang berjalan. Tunggu proses selesai sebelum menjalankan ulang.",
  WORKFLOW_INVALID_TRANSITION:
    "Transisi status tidak valid dari status GL saat ini.",
};
