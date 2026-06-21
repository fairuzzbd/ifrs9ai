/**
 * Zod schemas — P5-M9 Jatuh Tempo + Pendapatan Akrual Harian
 * Derived from api/openapi/app-b-jatuh-tempo-akrual.yaml
 * DEC-016: NUMERIC(20,4) IDR, NUMERIC(10,8) EIR — stored as string from API
 */

import { z } from "zod";

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

export const akrualStatusEnum = z.enum([
  "PENDING_STALE_REVIEW",
  "AUTO_POSTED",
  "OVERRIDE_APPROVED",
  "POSTED",
  "SKIPPED",
]);
export type AkrualStatus = z.infer<typeof akrualStatusEnum>;

export const akrualJenisEnum = z.enum([
  "BUNGA",
  "DIVIDEN",
  "AMORTISASI_PREMIUM",
  "AMORTISASI_DISKON",
  "DISTRIBUSI_REKSADANA",
]);
export type AkrualJenis = z.infer<typeof akrualJenisEnum>;

export const carryingBasisEnum = z.enum(["GROSS", "NET_CARRYING"]);
export type CarryingBasis = z.infer<typeof carryingBasisEnum>;

export const jatuhTempoStatusEnum = z.enum(["PENDING", "SETTLED", "FAILED", "SKIPPED"]);
export type JatuhTempoStatus = z.infer<typeof jatuhTempoStatusEnum>;

export const jatuhTempoJenisEnum = z.enum(["DEPOSITO", "BOND", "REKSADANA"]);
export type JatuhTempoJenis = z.infer<typeof jatuhTempoJenisEnum>;

// ---------------------------------------------------------------------------
// Label maps
// ---------------------------------------------------------------------------

export const AKRUAL_STATUS_LABELS: Record<AkrualStatus, string> = {
  PENDING_STALE_REVIEW: "Menunggu Review Stale",
  AUTO_POSTED: "Otomatis Diposting",
  OVERRIDE_APPROVED: "Override Disetujui",
  POSTED: "Diposting",
  SKIPPED: "Dilewati",
};

export const AKRUAL_JENIS_LABELS: Record<AkrualJenis, string> = {
  BUNGA: "Bunga EIR",
  DIVIDEN: "Dividen",
  AMORTISASI_PREMIUM: "Amortisasi Premium",
  AMORTISASI_DISKON: "Amortisasi Diskon",
  DISTRIBUSI_REKSADANA: "Distribusi Reksadana",
};

export const JATUH_TEMPO_STATUS_LABELS: Record<JatuhTempoStatus, string> = {
  PENDING: "Menunggu",
  SETTLED: "Diselesaikan",
  FAILED: "Gagal",
  SKIPPED: "Dilewati",
};

export const JATUH_TEMPO_JENIS_LABELS: Record<JatuhTempoJenis, string> = {
  DEPOSITO: "Deposito",
  BOND: "Obligasi",
  REKSADANA: "Reksa Dana",
};

// ---------------------------------------------------------------------------
// Override stale form (S5-AC4) — ROLE-AKUN-CTL
// ---------------------------------------------------------------------------

export const overrideStaleSchema = z.object({
  reason: z
    .string()
    .min(30, { message: "Alasan konfirmasi staging minimal 30 karakter (SoW §9.3)." }),
  signatureMethod: z.literal("JWT_STEP_UP"),
});
export type OverrideStaleInput = z.infer<typeof overrideStaleSchema>;

// ---------------------------------------------------------------------------
// API response shapes
// ---------------------------------------------------------------------------

export const akrualListItemSchema = z.object({
  id: z.string().uuid(),
  instrumenId: z.string().uuid(),
  instrumenKode: z.string(),
  klasifikasiSnapshot: z.string(),
  tanggalAkrual: z.string(),
  jenis: akrualJenisEnum,
  stage: z.number().int().min(1).max(3).nullable().optional(),
  carryingBasis: carryingBasisEnum,
  carryingIdr: z.string(),
  eirPersen: z.string().nullable().optional(),
  bungaKotor: z.string(),
  pph: z.string().nullable().optional(),
  bungaBersih: z.string(),
  mataUang: z.string().default("IDR"),
  fxRateId: z.string().uuid().nullable().optional(),
  staleStagingFlag: z.boolean(),
  eclRunIdUsed: z.string().uuid().nullable().optional(),
  status: akrualStatusEnum,
  jurnalHeaderId: z.string().uuid().nullable().optional(),
  createdAt: z.string(),
});
export type AkrualListItem = z.infer<typeof akrualListItemSchema>;

export const akrualDetailSchema = akrualListItemSchema.extend({
  overrideUserId: z.string().uuid().nullable().optional(),
  overrideComment: z.string().nullable().optional(),
  periodeBulananId: z.string().uuid().nullable().optional(),
  updatedAt: z.string().optional(),
  rowVersion: z.number().optional(),
});
export type AkrualDetail = z.infer<typeof akrualDetailSchema>;

export const akrualDashboardBreakdownSchema = z.object({
  jenis: z.string(),
  mtdIdr: z.string(),
  ytdIdr: z.string(),
});

export const akrualDashboardSchema = z.object({
  instrumenId: z.string().uuid().nullable().optional(),
  portofolioId: z.string().uuid().nullable().optional(),
  year: z.number().int(),
  month: z.number().int().min(1).max(12),
  akrualMtdIdr: z.string(),
  akrualYtdIdr: z.string(),
  stageSaatIni: z.number().int().nullable().optional(),
  stagingSource: z.string().nullable().optional(),
  eclRunSealedAt: z.string().nullable().optional(),
  staleCount: z.number().int(),
  breakdown: z.array(akrualDashboardBreakdownSchema),
});
export type AkrualDashboard = z.infer<typeof akrualDashboardSchema>;

export const jatuhTempoListItemSchema = z.object({
  id: z.string().uuid(),
  instrumenId: z.string().uuid(),
  instrumenKode: z.string(),
  tanggalJatuhTempo: z.string(),
  jenis: jatuhTempoJenisEnum,
  pokokIdr: z.string(),
  bungaLastIdr: z.string(),
  pphIdr: z.string(),
  netKasIdr: z.string(),
  klasifikasiSnapshot: z.string(),
  status: jatuhTempoStatusEnum,
  errorMessage: z.string().nullable().optional(),
  jurnalHeaderId: z.string().uuid().nullable().optional(),
  createdAt: z.string(),
});
export type JatuhTempoListItem = z.infer<typeof jatuhTempoListItemSchema>;

export const overrideStaleResponseSchema = z.object({
  akrualId: z.string().uuid(),
  status: z.literal("POSTED"),
  akrualIdr: z.string(),
  jurnalEntryId: z.string().uuid(),
});
export type OverrideStaleResponse = z.infer<typeof overrideStaleResponseSchema>;

// ---------------------------------------------------------------------------
// Error codes (7 baru dari OpenAPI P5-M9)
// ---------------------------------------------------------------------------

export const akrualErrorCodes = [
  "MATURITY_INSTRUMEN_NOT_ACTIVE",
  "AKRUAL_STAGING_STALE",
  "AKRUAL_FX_RATE_MISSING",
  "AKRUAL_PERIODE_LOCKED",
  "AKRUAL_DUPLICATE",
  "AKRUAL_EIR_NOT_FOUND",
  "DIVIDEN_VALIDATION_FAILED",
] as const;
export type AkrualErrorCode = (typeof akrualErrorCodes)[number];
