/**
 * P5-M15 — Zod schemas for dashboard widget data shapes.
 * Mirrors M14 report response column sets per slug.
 * Parsed at API boundaries in reports.api.ts + useReportData.ts.
 */

import { z } from "zod";

// ---------------------------------------------------------------------------
// Shared primitives
// ---------------------------------------------------------------------------

const decimalString = z.string(); // shopspring/decimal → JSON as string or number
const dateString = z.string(); // ISO 8601 date string

// ---------------------------------------------------------------------------
// RPT-01 — Daftar Instrumen (used by Treasury + CFO)
// ---------------------------------------------------------------------------

export const rpt01RowSchema = z.object({
  instrumen_id: z.string().uuid(),
  kode_instrumen: z.string(),
  nama: z.string(),
  jenis_instrumen: z.string(),
  bank_id: z.string().nullable().optional(),
  nama_bank: z.string().nullable().optional(),
  counterparty: z.string().nullable().optional(),
  status: z.string(),
  klasifikasi: z.string().nullable().optional(),
  ead_idr: decimalString,
  tanggal_jatuh_tempo: dateString.nullable().optional(),
  currency: z.string().default("IDR"),
});
export type Rpt01Row = z.infer<typeof rpt01RowSchema>;

// ---------------------------------------------------------------------------
// RPT-06 — Penempatan Log (Treasury: Recent Transactions)
// ---------------------------------------------------------------------------

export const rpt06RowSchema = z.object({
  penempatan_id: z.string().uuid(),
  kode: z.string(),
  jenis_instrumen: z.string(),
  counterparty: z.string().nullable().optional(),
  bank: z.string().nullable().optional(),
  nominal_idr: decimalString,
  tanggal_penempatan: dateString,
  status: z.string(),
  klasifikasi: z.string().nullable().optional(),
});
export type Rpt06Row = z.infer<typeof rpt06RowSchema>;

// ---------------------------------------------------------------------------
// RPT-10 — Jatuh Tempo Log (Treasury: Upcoming Maturities)
// ---------------------------------------------------------------------------

export const rpt10RowSchema = z.object({
  instrumen_id: z.string().uuid(),
  kode_instrumen: z.string(),
  nama: z.string().nullable().optional(),
  counterparty: z.string().nullable().optional(),
  nominal_idr: decimalString,
  tanggal_jatuh_tempo: dateString,
  jenis_instrumen: z.string().nullable().optional(),
});
export type Rpt10Row = z.infer<typeof rpt10RowSchema>;

// ---------------------------------------------------------------------------
// RPT-13 — ECL Calc Run Detail (Risk + CFO)
// ---------------------------------------------------------------------------

export const rpt13RowSchema = z.object({
  instrumen_id: z.string().uuid(),
  kode_instrumen: z.string(),
  nama: z.string().nullable().optional(),
  calc_run_id: z.string().uuid(),
  stage: z.number().int().min(1).max(3),
  ead_idr: decimalString,
  ecl_weighted: decimalString,
  ecl_fl_good: decimalString.optional(),
  ecl_fl_normal: decimalString.optional(),
  ecl_fl_bad: decimalString.optional(),
  fl_multiplier: decimalString.nullable().optional(),
  periode_id: z.string().nullable().optional(),
});
export type Rpt13Row = z.infer<typeof rpt13RowSchema>;

// ---------------------------------------------------------------------------
// RPT-14 — Stage Movement (Risk: Trend)
// ---------------------------------------------------------------------------

export const rpt14RowSchema = z.object({
  periode_id: z.string(),
  periode_label: z.string().nullable().optional(),
  tanggal_transisi: dateString,
  stage_from: z.number().int().nullable().optional(),
  stage_to: z.number().int(),
  count_instrumen: z.number().int(),
});
export type Rpt14Row = z.infer<typeof rpt14RowSchema>;

// ---------------------------------------------------------------------------
// RPT-15 — SICR Trigger Log (Risk: SICR Counters)
// ---------------------------------------------------------------------------

export const rpt15RowSchema = z.object({
  trigger_id: z.string().uuid().optional(),
  instrumen_id: z.string().uuid(),
  kode_instrumen: z.string().nullable().optional(),
  trigger_type: z.enum(["rating_downgrade", "ig_to_nonig", "dpd_30"]),
  tanggal_trigger: dateString,
  periode_id: z.string().nullable().optional(),
  detail: z.string().nullable().optional(),
});
export type Rpt15Row = z.infer<typeof rpt15RowSchema>;

// ---------------------------------------------------------------------------
// RPT-18 — ECL Roll-Forward (CFO: P&L Impact)
// ---------------------------------------------------------------------------

export const rpt18RowSchema = z.object({
  tanggal: dateString,
  periode_id: z.string().nullable().optional(),
  opening_ecl: decimalString.nullable().optional(),
  new_originations: decimalString.nullable().optional(),
  derecognitions: decimalString.nullable().optional(),
  stage_transfers: decimalString.nullable().optional(),
  remeasurements: decimalString.nullable().optional(),
  closing_ecl: decimalString,
  ecl_movement: decimalString.nullable().optional(),
});
export type Rpt18Row = z.infer<typeof rpt18RowSchema>;

// ---------------------------------------------------------------------------
// RPT-22 — Jurnal Posting Log (Akuntansi: Recent Jurnal)
// ---------------------------------------------------------------------------

export const rpt22RowSchema = z.object({
  jurnal_id: z.string().uuid(),
  event_code: z.string(),
  instrumen_id: z.string().uuid().nullable().optional(),
  kode_instrumen: z.string().nullable().optional(),
  nominal_idr: decimalString,
  posted_at: dateString.nullable().optional(),
  status_posting: z.string(),
  posting_batch_id: z.string().nullable().optional(),
});
export type Rpt22Row = z.infer<typeof rpt22RowSchema>;

// ---------------------------------------------------------------------------
// RPT-22b — GL Delivery Status (Akuntansi: GL Gauge)
// ---------------------------------------------------------------------------

export const rpt22bRowSchema = z.object({
  periode_id: z.string().nullable().optional(),
  delivered_count: z.number().int(),
  failed_count: z.number().int(),
  pending_count: z.number().int(),
  total_count: z.number().int(),
  success_rate_pct: decimalString.nullable().optional(),
});
export type Rpt22bRow = z.infer<typeof rpt22bRowSchema>;

// ---------------------------------------------------------------------------
// RPT-23 — Periode Close Audit (Akuntansi + CFO)
// ---------------------------------------------------------------------------

export const rpt23RowSchema = z.object({
  periode_id: z.string(),
  kode: z.string(),
  label: z.string().nullable().optional(),
  status: z.enum(["OPEN", "SOFT_CLOSED", "HARD_CLOSED"]),
  tanggal_open: dateString.nullable().optional(),
  tanggal_soft_close: dateString.nullable().optional(),
  tanggal_close: dateString.nullable().optional(),
  closed_by: z.string().nullable().optional(),
  is_current: z.boolean().default(false).optional(),
});
export type Rpt23Row = z.infer<typeof rpt23RowSchema>;

// ---------------------------------------------------------------------------
// RPT-25 — Audit Log Browser (Auditor)
// ---------------------------------------------------------------------------

export const rpt25RowSchema = z.object({
  event_id: z.string().uuid(),
  event_time: dateString,
  actor_user_id: z.string().uuid(),
  actor_username: z.string().nullable().optional(),
  actor_role: z.string(),
  action: z.string(),
  entity_type: z.string(),
  entity_id: z.string().uuid().nullable().optional(),
  detail: z.string().nullable().optional(),
  ip: z.string().nullable().optional(),
  trace_id: z.string().nullable().optional(),
  count: z.number().int().optional(), // for group_by=action queries
});
export type Rpt25Row = z.infer<typeof rpt25RowSchema>;

// ---------------------------------------------------------------------------
// RPT-26 — Workflow Pending Approval (shared)
// ---------------------------------------------------------------------------

export const rpt26RowSchema = z.object({
  workflow_id: z.string().uuid(),
  entity_type: z.string(),
  entity_id: z.string().uuid(),
  kode_entitas: z.string().nullable().optional(),
  status: z.string(),
  submitted_by: z.string().nullable().optional(),
  submitted_by_username: z.string().nullable().optional(),
  submitted_at: dateString.nullable().optional(),
  detail: z.string().nullable().optional(),
  jurnal_id: z.string().uuid().nullable().optional(),
  nominal_idr: decimalString.nullable().optional(),
  event_code: z.string().nullable().optional(),
});
export type Rpt26Row = z.infer<typeof rpt26RowSchema>;

// ---------------------------------------------------------------------------
// RPT-27 — ECL Sensitivity Analysis (CFO)
// ---------------------------------------------------------------------------

export const rpt27RowSchema = z.object({
  calc_run_id: z.string().uuid(),
  w_good: decimalString.nullable().optional(),
  w_normal: decimalString.nullable().optional(),
  w_bad: decimalString.nullable().optional(),
  ecl_fl_good_total: decimalString,
  ecl_fl_normal_total: decimalString,
  ecl_fl_bad_total: decimalString,
  ecl_weighted_total: decimalString,
});
export type Rpt27Row = z.infer<typeof rpt27RowSchema>;

// ---------------------------------------------------------------------------
// RPT-05 — FX Rate History (Akuntansi: FX Freshness)
// ---------------------------------------------------------------------------

export const rpt05RowSchema = z.object({
  kurs_id: z.string().uuid().optional(),
  kode_mata_uang: z.string(),
  tanggal: dateString,
  kurs_idr: decimalString,
  sumber: z.string().nullable().optional(), // "JISDOR" | "MANUAL"
  status: z.string().nullable().optional(),
});
export type Rpt05Row = z.infer<typeof rpt05RowSchema>;

// ---------------------------------------------------------------------------
// Typed response wrappers per slug
// ---------------------------------------------------------------------------

export type DashboardReportSlug =
  | "rpt-01"
  | "rpt-05"
  | "rpt-06"
  | "rpt-10"
  | "rpt-13"
  | "rpt-14"
  | "rpt-15"
  | "rpt-18"
  | "rpt-22"
  | "rpt-22b"
  | "rpt-23"
  | "rpt-25"
  | "rpt-26"
  | "rpt-27";

// Map slug → row schema (used for parsing in useReportData)
export const REPORT_SLUG_SCHEMA = {
  "rpt-01": rpt01RowSchema,
  "rpt-05": rpt05RowSchema,
  "rpt-06": rpt06RowSchema,
  "rpt-10": rpt10RowSchema,
  "rpt-13": rpt13RowSchema,
  "rpt-14": rpt14RowSchema,
  "rpt-15": rpt15RowSchema,
  "rpt-18": rpt18RowSchema,
  "rpt-22": rpt22RowSchema,
  "rpt-22b": rpt22bRowSchema,
  "rpt-23": rpt23RowSchema,
  "rpt-25": rpt25RowSchema,
  "rpt-26": rpt26RowSchema,
  "rpt-27": rpt27RowSchema,
} as const;

export type ReportRowType<S extends DashboardReportSlug> =
  z.infer<(typeof REPORT_SLUG_SCHEMA)[S]>;
