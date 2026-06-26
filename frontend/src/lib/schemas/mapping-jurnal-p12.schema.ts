/**
 * P5-M12 — Mapping Jurnal Zod schemas (6-eyes workflow, version chain, RPT-19/20/21)
 * Mirrors api/openapi/app-d-mapping-jurnal.yaml
 *
 * Extends existing mapping-jurnal.schema.ts with P5-M12-specific contracts:
 * - MappingWorkflowStatus 5-state enum (DRAFT/PENDING_REVIEW/PENDING_APPROVAL/PENDING_APPROVAL_2/APPROVED_ACTIVE)
 * - WorkflowActionRequest / ReviewRequest / ApproveRequest / RejectRequest
 * - NewVersionRequest
 * - BulkImportResult
 * - RPT-19 Coverage, RPT-20 Validation, RPT-21 AuditLogEntry
 */

import { z } from "zod";

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

export const mappingP12WorkflowStatusEnum = z.enum([
  "DRAFT",
  "PENDING_REVIEW",
  "PENDING_APPROVAL",
  "PENDING_APPROVAL_2",
  "APPROVED_ACTIVE",
]);
export type MappingP12WorkflowStatus = z.infer<typeof mappingP12WorkflowStatusEnum>;

export const MAPPING_WORKFLOW_STATUS_LABELS: Record<MappingP12WorkflowStatus, string> = {
  DRAFT: "Draf",
  PENDING_REVIEW: "Menunggu Review",
  PENDING_APPROVAL: "Menunggu Approval",
  PENDING_APPROVAL_2: "Menunggu Approval RISK",
  APPROVED_ACTIVE: "Aktif",
};

export const mappingWorkflowPathEnum = z.enum(["4-eyes", "6-eyes"]);
export type MappingWorkflowPath = z.infer<typeof mappingWorkflowPathEnum>;

export const gapCoverageEnum = z.enum(["OK", "MISSING", "INCOMPLETE"]);
export type GapCoverage = z.infer<typeof gapCoverageEnum>;

// ---------------------------------------------------------------------------
// Header summary (list item)
// ---------------------------------------------------------------------------

export const mappingP12HeaderSummarySchema = z.object({
  id: z.string().uuid(),
  eventCode: z.string(),
  namaEvent: z.string(),
  kategoriEvent: z.string(),
  workflowStatus: mappingP12WorkflowStatusEnum,
  workflowPath: mappingWorkflowPathEnum,
  regulatedFlag: z.boolean(),
  aktifFlag: z.boolean(),
  parentId: z.string().uuid().nullable(),
  effectiveFrom: z.string().nullable(),
  effectiveTo: z.string().nullable(),
  makerId: z.string().uuid().nullable(),
  reviewerId: z.string().uuid().nullable(),
  approverId: z.string().uuid().nullable(),
  approver2Id: z.string().uuid().nullable(),
  updatedAt: z.string(),
});
export type MappingP12HeaderSummary = z.infer<typeof mappingP12HeaderSummarySchema>;

// ---------------------------------------------------------------------------
// Detail row (akun_debit/kredit, debit_kredit, jumlah_calc)
// ---------------------------------------------------------------------------

export const mappingP12DetailRowSchema = z.object({
  id: z.string().uuid(),
  headerId: z.string().uuid(),
  akunDebit: z.string().nullable(),
  akunKredit: z.string().nullable(),
  debitKredit: z.enum(["D", "K"]),
  jumlahCalc: z.string().nullable(),
  urutan: z.number().int(),
});
export type MappingP12DetailRow = z.infer<typeof mappingP12DetailRowSchema>;

// ---------------------------------------------------------------------------
// Header detail (single event full view)
// ---------------------------------------------------------------------------

export const mappingP12HeaderDetailSchema = mappingP12HeaderSummarySchema.extend({
  detail: z.array(mappingP12DetailRowSchema),
  versionHistory: z.array(mappingP12HeaderSummarySchema),
  klasifikasiBerlaku: z.array(z.string()).nullable(),
  catatan: z.string().nullable(),
  rejectReason: z.string().nullable(),
  reviewerSignedAt: z.string().nullable(),
  approverSignedAt: z.string().nullable(),
  approver2SignedAt: z.string().nullable(),
});
export type MappingP12HeaderDetail = z.infer<typeof mappingP12HeaderDetailSchema>;

// ---------------------------------------------------------------------------
// Form schemas (React Hook Form + Zod)
// ---------------------------------------------------------------------------

/** New detail row form (used inside MappingNewForm for detail rows array) */
export const mappingDetailInputSchema = z.object({
  _clientKey: z.string().optional(),
  akunDebit: z
    .string()
    .min(1, "Akun debit wajib diisi")
    .max(20, "Kode akun maks 20 karakter"),
  akunKredit: z
    .string()
    .min(1, "Akun kredit wajib diisi")
    .max(20, "Kode akun maks 20 karakter"),
  debitKredit: z.enum(["D", "K"], {
    error: () => ({ message: "Pilih D (Debit) atau K (Kredit)" }),
  }),
  jumlahCalc: z.string().max(200, "Formula maks 200 karakter").nullable().optional(),
  urutan: z.number().int().min(1, "Urutan minimal 1"),
});
export type MappingDetailInput = z.infer<typeof mappingDetailInputSchema>;

/** New version form (POST /mapping-jurnal/{event_code}/new-version) */
export const newVersionFormSchema = z.object({
  reason: z.string().min(10, "Alasan minimal 10 karakter").max(500, "Alasan maks 500 karakter"),
  detail: z.array(mappingDetailInputSchema).min(1, "Minimal 1 baris detail"),
});
export type NewVersionFormInput = z.infer<typeof newVersionFormSchema>;

/** Submit dialog form */
export const submitDialogSchema = z.object({
  comment: z.string().min(1, "Komentar wajib diisi").max(500),
});
export type SubmitDialogInput = z.infer<typeof submitDialogSchema>;

/** Review dialog form (comment ≥ 30 chars, signature_method) */
export const reviewDialogSchema = z.object({
  comment: z
    .string()
    .min(30, "Komentar reviewer minimal 30 karakter")
    .max(1000, "Komentar maks 1000 karakter"),
  signatureMethod: z.literal("JWT_STEP_UP"),
});
export type ReviewDialogInput = z.infer<typeof reviewDialogSchema>;

/** Approve dialog form (comment ≥ 10 chars, signature_method) */
export const approveDialogSchema = z.object({
  comment: z
    .string()
    .min(10, "Komentar approver minimal 10 karakter")
    .max(1000, "Komentar maks 1000 karakter"),
  signatureMethod: z.literal("JWT_STEP_UP"),
  stepUpToken: z.string().optional(), // required for approve-2
});
export type ApproveDialogInput = z.infer<typeof approveDialogSchema>;

/** Reject dialog form (reason ≥ 30 chars) */
export const rejectDialogSchema = z.object({
  reason: z
    .string()
    .min(30, "Alasan penolakan minimal 30 karakter")
    .max(1000, "Alasan maks 1000 karakter"),
});
export type RejectDialogInput = z.infer<typeof rejectDialogSchema>;

// ---------------------------------------------------------------------------
// API request types (match OpenAPI)
// ---------------------------------------------------------------------------

export interface WorkflowActionRequest {
  comment: string;
}

export interface ReviewRequest {
  comment: string; // min 30
  signatureMethod: "JWT_STEP_UP";
}

export interface ApproveRequest {
  comment: string; // min 10
  signatureMethod: "JWT_STEP_UP";
}

export interface RejectRequest {
  reason: string; // min 30
}

export interface NewVersionRequest {
  reason: string; // min 10
  detail: {
    akunDebit: string;
    akunKredit: string;
    debitKredit: "D" | "K";
    jumlahCalc?: string | null;
    urutan: number;
  }[];
}

// ---------------------------------------------------------------------------
// Workflow action response
// ---------------------------------------------------------------------------

export interface WorkflowActionResponse {
  data: {
    id: string;
    eventCode: string;
    workflowStatus: MappingP12WorkflowStatus;
    aktifFlag: boolean;
    updatedAt: string;
  };
  meta: { traceId: string };
}

// ---------------------------------------------------------------------------
// Bulk import result
// ---------------------------------------------------------------------------

export const bulkImportErrorSchema = z.object({
  row: z.number().int(),
  col: z.string(),
  errorCode: z.string(),
  error: z.string(),
});

export const bulkImportResultSchema = z.object({
  batchId: z.string(),
  batchType: z.literal("MAPPING_BULK"),
  totalRows: z.number().int(),
  validRows: z.number().int(),
  invalidRows: z.number().int(),
  errors: z.array(bulkImportErrorSchema),
});
export type BulkImportResult = z.infer<typeof bulkImportResultSchema>;

// ---------------------------------------------------------------------------
// RPT-19 Coverage
// ---------------------------------------------------------------------------

export const rpt19CoverageEventSchema = z.object({
  eventCode: z.string(),
  namaEvent: z.string(),
  workflowStatus: mappingP12WorkflowStatusEnum.nullable(),
  activeDetailCount: z.number().int(),
  missingAkunCount: z.number().int(),
  lastDlqError: z.string().nullable(),
  gapCoverage: gapCoverageEnum,
});
export type Rpt19CoverageEvent = z.infer<typeof rpt19CoverageEventSchema>;

export const rpt19CoverageSchema = z.object({
  totalEvents: z.number().int(),
  activeEvents: z.number().int(),
  missingEvents: z.number().int(),
  gapEvents: z.array(rpt19CoverageEventSchema),
});
export type Rpt19Coverage = z.infer<typeof rpt19CoverageSchema>;

// ---------------------------------------------------------------------------
// RPT-20 Validation
// ---------------------------------------------------------------------------

export const rpt20IssueSchema = z.object({
  eventCode: z.string(),
  headerId: z.string().uuid(),
  errorCodes: z.array(z.string()),
  details: z.string(),
});
export type Rpt20Issue = z.infer<typeof rpt20IssueSchema>;

export const rpt20ValidationSchema = z.object({
  totalActiveMappings: z.number().int(),
  validMappings: z.number().int(),
  invalidMappings: z.number().int(),
  issues: z.array(rpt20IssueSchema),
});
export type Rpt20Validation = z.infer<typeof rpt20ValidationSchema>;

// ---------------------------------------------------------------------------
// RPT-21 Audit log entry
// ---------------------------------------------------------------------------

export const auditLogEntrySchema = z.object({
  eventId: z.string().uuid(),
  eventTime: z.string(),
  actorUserId: z.string().uuid(),
  actorRole: z.string(),
  action: z.string(),
  entityType: z.string(),
  entityId: z.string().uuid(),
  beforeJsonb: z.record(z.unknown()).nullable(),
  afterJsonb: z.record(z.unknown()).nullable(),
  traceId: z.string().nullable(),
});
export type AuditLogEntry = z.infer<typeof auditLogEntrySchema>;

// ---------------------------------------------------------------------------
// 7 new MAPPING_* error codes (for notify.ts)
// ---------------------------------------------------------------------------

export const MAPPING_P12_ERROR_CODES = [
  "MAPPING_EVENT_NOT_FOUND",
  "MAPPING_AKUN_INVALID",
  "MAPPING_UNBALANCED",
  "MAPPING_REGULATED_REQUIRES_RISK",
  "MAPPING_DUPLICATE_VERSION",
  "MAPPING_SOD_VIOLATION",
  "MAPPING_PERIODE_LOCKED",
] as const;

export type MappingP12ErrorCode = (typeof MAPPING_P12_ERROR_CODES)[number];
