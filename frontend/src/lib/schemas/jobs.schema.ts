/**
 * P5-M15 — Zod schemas for Job list + detail.
 * Extends M13 job status types with list-specific fields.
 */

import { z } from "zod";

// ---------------------------------------------------------------------------
// Job status enum
// ---------------------------------------------------------------------------

export const jobStatusEnum = z.enum([
  "queued",
  "running",
  "completed",
  "failed",
  "cancelled",
]);
export type JobStatus = z.infer<typeof jobStatusEnum>;

// ---------------------------------------------------------------------------
// Job type label map (Bahasa Indonesia per design §7.5)
// ---------------------------------------------------------------------------

export const JOB_TYPE_LABELS: Record<string, string> = {
  ECL_CALC_RUN: "ECL Calc Run",
  EIR_RE_ESTIMATION: "Re-estimasi EIR",
  PEFINDO_IMPORT: "Import Rating Pefindo",
  IBPA_IMPORT: "Import Harga IBPA",
  KSEI_NAB_IMPORT: "Import NAB KSEI",
  BEI_PRICE_IMPORT: "Import Harga BEI",
  EXPORT_CSV: "Export CSV",
  EXPORT_XLSX: "Export XLSX",
  MV_REFRESH: "Refresh Materialized View",
  HASH_CHAIN_VERIFY: "Verifikasi Hash-Chain",
  MTM_DAILY_RUN: "MTM Harian",
  GL_RECON: "Rekonsiliasi GL",
  JURNAL_BATCH_POST: "Posting Jurnal Batch",
};

export function getJobTypeLabel(type: string): string {
  return JOB_TYPE_LABELS[type] ?? type;
}

// ---------------------------------------------------------------------------
// Job status labels
// ---------------------------------------------------------------------------

export const JOB_STATUS_LABELS: Record<JobStatus, string> = {
  queued: "Antri",
  running: "Berjalan",
  completed: "Selesai",
  failed: "Gagal",
  cancelled: "Dibatalkan",
};

// ---------------------------------------------------------------------------
// Job list item schema (GET /api/v1/jobs list response)
// ---------------------------------------------------------------------------

export const jobListItemSchema = z.object({
  jobId: z.string(),
  type: z.string(),
  status: jobStatusEnum,
  progress: z.number().int().min(0).max(100),
  currentStep: z.string().nullable().optional(),
  startedAt: z.string().nullable().optional(),
  estimatedCompletionAt: z.string().nullable().optional(),
  completedAt: z.string().nullable().optional(),
  createdAt: z.string().nullable().optional(),
  createdBy: z.string().nullable().optional(),
  createdByUsername: z.string().nullable().optional(),
  canCancel: z.boolean().default(false).optional(),
  resultUrl: z.string().nullable().optional(),
  result: z.unknown().optional(),
  error: z.unknown().optional(),
  durationSeconds: z.number().nullable().optional(),
});
export type JobListItem = z.infer<typeof jobListItemSchema>;

// ---------------------------------------------------------------------------
// Job list query params
// ---------------------------------------------------------------------------

export interface JobListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[status]"?: string;
  "filter[type]"?: string;
  "filter[created_by]"?: string;
  "filter[started_at]"?: string;
}

// ---------------------------------------------------------------------------
// Zod for filters form
// ---------------------------------------------------------------------------

export const jobFiltersSchema = z.object({
  q: z.string().optional(),
  status: z.array(jobStatusEnum).optional(),
  type: z.string().optional(),
  dateFrom: z.string().optional(),
  dateTo: z.string().optional(),
  createdBy: z.string().optional(),
});
export type JobFiltersInput = z.infer<typeof jobFiltersSchema>;
