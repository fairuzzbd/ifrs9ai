/**
 * Zod schemas — P5-M11 Bulk Upload Master Instrumen
 * Derived from api/openapi/app-b-bulk-upload.yaml
 * DEC-016: money as string NUMERIC(20,4); DEC-017: SoD; DEC-018: soft-delete; DEC-021: idempotency
 */

import { z } from "zod";

// ---------------------------------------------------------------------------
// Batch status enum — 9 states per state machine §1
// ---------------------------------------------------------------------------

export const bulkBatchStatusEnum = z.enum([
  "PARSED",
  "DRY_RUN_PASSED",
  "DRY_RUN_FAILED",
  "COMMITTING",
  "COMMITTED",
  "PARTIAL_COMMIT",
  "APPROVED",
  "ROLLBACK_PENDING",
  "ROLLED_BACK",
]);
export type BulkBatchStatus = z.infer<typeof bulkBatchStatusEnum>;

// ---------------------------------------------------------------------------
// Row status enum — 5 states
// ---------------------------------------------------------------------------

export const bulkRowStatusEnum = z.enum([
  "PENDING",
  "COMMITTED",
  "FAILED",
  "ROLLED_BACK",
  "FLAGGED_MANUAL_REVIEW",
]);
export type BulkRowStatus = z.infer<typeof bulkRowStatusEnum>;

// ---------------------------------------------------------------------------
// Rollback status (separate field on batch)
// ---------------------------------------------------------------------------

export const rollbackStatusEnum = z.enum([
  "NOT_REQUESTED",
  "PENDING",
  "APPROVED",
  "EXPIRED",
]);
export type RollbackStatus = z.infer<typeof rollbackStatusEnum>;

// ---------------------------------------------------------------------------
// Label maps
// ---------------------------------------------------------------------------

export const BULK_BATCH_STATUS_LABELS: Record<BulkBatchStatus, string> = {
  PARSED: "Diparsing",
  DRY_RUN_PASSED: "Validasi Lulus",
  DRY_RUN_FAILED: "Validasi Gagal",
  COMMITTING: "Sedang Commit",
  COMMITTED: "Berhasil Commit",
  PARTIAL_COMMIT: "Commit Sebagian",
  APPROVED: "Disetujui",
  ROLLBACK_PENDING: "Menunggu Rollback",
  ROLLED_BACK: "Di-rollback",
};

export const BULK_ROW_STATUS_LABELS: Record<BulkRowStatus, string> = {
  PENDING: "Menunggu",
  COMMITTED: "Berhasil",
  FAILED: "Gagal",
  ROLLED_BACK: "Di-rollback",
  FLAGGED_MANUAL_REVIEW: "Perlu Review Manual",
};

// ---------------------------------------------------------------------------
// Sub-schemas
// ---------------------------------------------------------------------------

export const parseErrorSchema = z.object({
  sheet: z.string(),
  row: z.number().int(),
  col: z.string().optional(),
  error: z.string(),
});
export type ParseError = z.infer<typeof parseErrorSchema>;

export const rowValidationErrorSchema = z.object({
  sheet: z.string(),
  row: z.number().int(),
  stage: z.number().int().min(1).max(4),
  col: z.string().nullable().optional(),
  error: z.string(),
  klasifikasiPsak71: z.string().nullable().optional(),
});
export type RowValidationError = z.infer<typeof rowValidationErrorSchema>;

export const stageSummaryItemSchema = z.object({
  status: z.enum(["PASS", "FAIL"]),
  errorCount: z.number().int().optional(),
});

export const stage4SummarySchema = z.object({
  status: z.enum(["PASS", "PARTIAL", "UNAVAILABLE"]),
  evaluated: z.number().int().optional(),
  classified: z.number().int().optional(),
  flagged: z.number().int().optional(),
  sppiServiceUnavailable: z.boolean().default(false),
});

export const stageSummarySchema = z.object({
  stage1: stageSummaryItemSchema.optional(),
  stage2: stageSummaryItemSchema.optional(),
  stage3: stageSummaryItemSchema.optional(),
  stage4: stage4SummarySchema.optional(),
});
export type StageSummary = z.infer<typeof stageSummarySchema>;

// ---------------------------------------------------------------------------
// Batch summary (upload response)
// ---------------------------------------------------------------------------

export const bulkUploadBatchSummarySchema = z.object({
  batchId: z.string(),
  status: bulkBatchStatusEnum,
  totalRows: z.number().int(),
  parseErrors: z.array(parseErrorSchema).default([]),
  sheets: z.record(z.string(), z.number().int()),
  createdAt: z.string(),
});
export type BulkUploadBatchSummary = z.infer<typeof bulkUploadBatchSummarySchema>;

// ---------------------------------------------------------------------------
// Batch detail (GET /batch/{id})
// ---------------------------------------------------------------------------

export const bulkUploadBatchDetailSchema = bulkUploadBatchSummarySchema.extend({
  committedRows: z.number().int().optional(),
  failedRows: z.number().int().optional(),
  flaggedRows: z.number().int().optional(),
  dryRunExpiresAt: z.string().nullable().optional(),
  rollbackStatus: rollbackStatusEnum.nullable().optional(),
  rollbackGraceExpiresAt: z.string().nullable().optional(),
  approverId: z.string().uuid().nullable().optional(),
  approvedAt: z.string().nullable().optional(),
});
export type BulkUploadBatchDetail = z.infer<typeof bulkUploadBatchDetailSchema>;

// ---------------------------------------------------------------------------
// Row item (paginated in batch detail)
// ---------------------------------------------------------------------------

export const bulkUploadRowItemSchema = z.object({
  rowId: z.string().uuid(),
  batchId: z.string(),
  sheetName: z.string(),
  rowNumber: z.number().int(),
  rowStatus: bulkRowStatusEnum,
  instrumenId: z.string().uuid().nullable().optional(),
  rowErrorJsonb: z.unknown().nullable().optional(),
  rowDataPreview: z.record(z.string(), z.unknown()).optional(),
});
export type BulkUploadRowItem = z.infer<typeof bulkUploadRowItemSchema>;

// ---------------------------------------------------------------------------
// DRY_RUN result
// ---------------------------------------------------------------------------

export const dryRunResultSchema = z.object({
  status: z.enum(["DRY_RUN_PASSED", "DRY_RUN_FAILED"]),
  totalRows: z.number().int().optional(),
  validRows: z.number().int().optional(),
  invalidRows: z.number().int().optional(),
  flaggedRows: z.number().int().optional(),
  stageSummary: stageSummarySchema.optional(),
  errorsPerRow: z.array(rowValidationErrorSchema).default([]),
  dryRunExpiresAt: z.string().nullable().optional(),
});
export type DryRunResult = z.infer<typeof dryRunResultSchema>;

// ---------------------------------------------------------------------------
// Commit job response (202)
// ---------------------------------------------------------------------------

export const commitJobResponseSchema = z.object({
  jobId: z.string(),
  type: z.string(),
  statusUrl: z.string(),
  streamUrl: z.string(),
  batchId: z.string(),
});
export type CommitJobResponse = z.infer<typeof commitJobResponseSchema>;

// ---------------------------------------------------------------------------
// Approve response
// ---------------------------------------------------------------------------

export const approveResultSchema = z.object({
  batchId: z.string(),
  status: z.literal("APPROVED"),
  activatedCount: z.number().int(),
  pendingManualCount: z.number().int(),
  approverId: z.string().uuid(),
  approvedAt: z.string(),
});
export type ApproveResult = z.infer<typeof approveResultSchema>;

// ---------------------------------------------------------------------------
// Rollback result
// ---------------------------------------------------------------------------

export const rollbackResultSchema = z.object({
  batchId: z.string(),
  status: z.literal("ROLLED_BACK"),
  rolledBackCount: z.number().int(),
  rolledBackAt: z.string(),
});
export type RollbackResult = z.infer<typeof rollbackResultSchema>;

// ---------------------------------------------------------------------------
// Form input schemas (React Hook Form + Zod)
// ---------------------------------------------------------------------------

export const approveFormSchema = z.object({
  comment: z.string().min(10, "Komentar minimal 10 karakter"),
  signatureMethod: z.literal("JWT_STEP_UP"),
});
export type ApproveFormInput = z.infer<typeof approveFormSchema>;

export const rollbackRequestFormSchema = z.object({
  reason: z.string().min(50, "Alasan minimal 50 karakter (audit trail requirement)"),
});
export type RollbackRequestFormInput = z.infer<typeof rollbackRequestFormSchema>;

export const rollbackApproveFormSchema = z.object({
  comment: z.string().min(10, "Komentar minimal 10 karakter"),
  signatureMethod: z.literal("JWT_STEP_UP"),
});
export type RollbackApproveFormInput = z.infer<typeof rollbackApproveFormSchema>;

// ---------------------------------------------------------------------------
// Error codes — 7 baru P5-M11
// ---------------------------------------------------------------------------

export const bulkUploadErrorCodes = [
  "BULK_FILE_TOO_LARGE",
  "BULK_MIME_INVALID",
  "BULK_DRY_RUN_EXPIRED",
  "BULK_DRY_RUN_FAILED",
  "BULK_PERIODE_LOCKED",
  "BULK_ROLLBACK_GRACE_EXPIRED",
  "BULK_APPROVE_SOD_VIOLATION",
] as const;
export type BulkUploadErrorCode = (typeof bulkUploadErrorCodes)[number];
