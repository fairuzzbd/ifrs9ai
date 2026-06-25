/**
 * P5-M13 — Reporting MV Foundation Zod schemas
 * Mirrors api/openapi/app-e-reporting-mv.yaml
 *
 * Covers:
 *  - MVStatusItem (IDLE/REFRESHING/FAILED)
 *  - ExportLogItem (REQUESTED/COMPUTING/COMPLETED/FAILED, CSV/XLSX/PDF)
 *  - ScheduledEmailRequest / ScheduledEmailItem
 *  - AsyncJobRef
 *  - 6 new error codes (EXPORT_*, MV_*, SCHEDULED_EMAIL_*)
 */

import { z } from "zod";

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

export const mvStatusEnum = z.enum(["IDLE", "REFRESHING", "FAILED"]);
export type MVStatus = z.infer<typeof mvStatusEnum>;

export const mvTriggerEnum = z.enum(["CRON", "HARD_CLOSE", "MANUAL"]);
export type MVTrigger = z.infer<typeof mvTriggerEnum>;

export const exportFormatEnum = z.enum(["csv", "xlsx", "pdf"]);
export type ExportFormat = z.infer<typeof exportFormatEnum>;

export const exportStatusEnum = z.enum(["REQUESTED", "QUEUED", "COMPUTING", "COMPLETED", "FAILED"]);
export type ExportStatus = z.infer<typeof exportStatusEnum>;

export const schedEmailFreqEnum = z.enum(["daily", "weekly", "monthly"]);
export type SchedEmailFreq = z.infer<typeof schedEmailFreqEnum>;

export const schedEmailStatusEnum = z.enum(["SENT", "FAILED", "PENDING"]);
export type SchedEmailStatus = z.infer<typeof schedEmailStatusEnum>;

export const reportSlugEnum = z.enum([
  "mv-status-periode",
  "mv-jurnal-summary",
  "mv-gl-delivery-status",
  "mv-mtm-daily-summary",
  "mv-akrual-summary",
  "mv-renewal-summary",
  "mv-penjualan-summary",
  "mv-poci-delta-summary",
]);
export type ReportSlug = z.infer<typeof reportSlugEnum>;

// ---------------------------------------------------------------------------
// Labels
// ---------------------------------------------------------------------------

export const MV_STATUS_LABELS: Record<MVStatus, string> = {
  IDLE: "Selesai",
  REFRESHING: "Sedang Refresh",
  FAILED: "Gagal",
};

export const EXPORT_STATUS_LABELS: Record<ExportStatus, string> = {
  REQUESTED: "Diminta",
  QUEUED: "Dalam Antrean",
  COMPUTING: "Sedang Diproses",
  COMPLETED: "Selesai",
  FAILED: "Gagal",
};

export const EXPORT_FORMAT_LABELS: Record<ExportFormat, string> = {
  csv: "CSV",
  xlsx: "XLSX",
  pdf: "PDF",
};

export const REPORT_SLUG_LABELS: Record<ReportSlug, string> = {
  "mv-status-periode": "Status Periode",
  "mv-jurnal-summary": "Ringkasan Jurnal",
  "mv-gl-delivery-status": "Status GL Delivery",
  "mv-mtm-daily-summary": "Ringkasan MTM Harian",
  "mv-akrual-summary": "Ringkasan Akrual",
  "mv-renewal-summary": "Ringkasan Renewal",
  "mv-penjualan-summary": "Ringkasan Penjualan",
  "mv-poci-delta-summary": "Delta POCI",
};

// ---------------------------------------------------------------------------
// API response schemas
// ---------------------------------------------------------------------------

export const mvStatusItemSchema = z.object({
  mvName: z.string(),
  status: mvStatusEnum,
  lastRefreshAt: z.string().nullable(),
  rowCount: z.number().int().nullable(),
  lastError: z.string().nullable(),
  triggeredBy: mvTriggerEnum.nullable(),
});
export type MVStatusItem = z.infer<typeof mvStatusItemSchema>;

export const asyncJobRefSchema = z.object({
  jobId: z.string(),
  statusUrl: z.string(),
  streamUrl: z.string(),
});
export type AsyncJobRef = z.infer<typeof asyncJobRefSchema>;

export const exportLogItemSchema = z.object({
  id: z.string().uuid(),
  reportSlug: z.string(),
  format: exportFormatEnum,
  status: exportStatusEnum,
  rowCount: z.number().int().nullable(),
  fileSha256: z.string().nullable(),
  minioPath: z.string().nullable(),
  expiresAt: z.string().nullable(),
  requestedBy: z.string().uuid(),
  requestedAt: z.string(),
  completedAt: z.string().nullable(),
  downloadedAt: z.string().nullable(),
});
export type ExportLogItem = z.infer<typeof exportLogItemSchema>;

export const scheduledEmailItemSchema = z.object({
  id: z.string().uuid(),
  reportSlug: z.string(),
  format: exportFormatEnum,
  frequency: schedEmailFreqEnum,
  sendTime: z.string(),
  recipients: z.array(z.string().email()),
  active: z.boolean(),
  lastSentAt: z.string().nullable(),
  lastStatus: schedEmailStatusEnum.nullable(),
  optOutCount: z.number().int(),
  createdAt: z.string(),
  createdBy: z.string().uuid(),
});
export type ScheduledEmailItem = z.infer<typeof scheduledEmailItemSchema>;

// ---------------------------------------------------------------------------
// Form schemas (React Hook Form + Zod)
// ---------------------------------------------------------------------------

/** Manual MV refresh trigger request */
export const mvRefreshRequestSchema = z.object({
  mvName: z.string().nullable().optional(),
});
export type MVRefreshRequest = z.infer<typeof mvRefreshRequestSchema>;

/** Scheduled email create/edit form */
export const scheduledEmailFormSchema = z.object({
  reportSlug: reportSlugEnum,
  format: exportFormatEnum,
  frequency: schedEmailFreqEnum,
  sendTime: z
    .string()
    .min(1, "Waktu kirim wajib diisi")
    .regex(/^\d{2}:\d{2}\+07:00$/, "Format waktu harus HH:MM+07:00, contoh: 07:00+07:00"),
  recipients: z
    .string()
    .min(1, "Minimal satu penerima email")
    .refine(
      (v) =>
        v
          .split(",")
          .map((e) => e.trim())
          .every((e) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(e)),
      "Satu atau lebih email tidak valid (pisahkan dengan koma)",
    ),
  active: z.boolean().default(true),
  subjectTemplate: z
    .string()
    .max(200, "Subjek template maks 200 karakter")
    .optional()
    .or(z.literal("")),
  bodyTemplate: z.string().max(2000, "Body template maks 2000 karakter").optional().or(z.literal("")),
});
export type ScheduledEmailFormInput = z.infer<typeof scheduledEmailFormSchema>;

/** Parses comma-separated recipients string to array */
export function parseRecipients(raw: string): string[] {
  return raw.split(",").map((e) => e.trim()).filter(Boolean);
}

// ---------------------------------------------------------------------------
// 6 new error codes (P5-M13)
// ---------------------------------------------------------------------------

export const REPORTING_ERROR_CODES = [
  "EXPORT_TOO_LARGE",
  "EXPORT_PERMISSION_DENIED",
  "EXPORT_FORMAT_UNSUPPORTED",
  "MV_REFRESH_LOCKED",
  "MV_REFRESH_FAILED",
  "SCHEDULED_EMAIL_SMTP_FAILED",
] as const;

export type ReportingErrorCode = (typeof REPORTING_ERROR_CODES)[number];
