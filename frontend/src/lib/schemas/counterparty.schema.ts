import { z } from "zod";
import type { WorkflowStatus, WorkflowHistoryEntry, MasterWorkflowState } from "@/lib/schemas/mata-uang.schema";

// Re-export as type-only to share across modules (no runtime dependency)
export type { WorkflowStatus, WorkflowHistoryEntry, MasterWorkflowState };

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

export const tipeCounterpartyEnum = z.enum([
  "BANK", "PERUSAHAAN_ASURANSI", "PERUSAHAAN_SEKURITAS", "MANAJER_INVESTASI",
  "PEMERINTAH", "KORPORASI", "LAINNYA",
] as const);

export const tipeEksposurBaselEnum = z.enum([
  "CORPORATE", "FINANCIAL_INSTITUTION", "SOVEREIGN", "RETAIL", "OTHER",
] as const);

export const statusCounterpartyEnum = z.enum([
  "AKTIF", "TIDAK_AKTIF", "DIBLOKIR",
] as const);

export const kategoriMiEnum = z.enum([
  "REKSA_DANA", "MANAJER_INVESTASI", "NON_MI",
] as const);

export type TipeCounterparty = z.infer<typeof tipeCounterpartyEnum>;
export type TipeEksposurBasel = z.infer<typeof tipeEksposurBaselEnum>;
export type StatusCounterparty = z.infer<typeof statusCounterpartyEnum>;
export type KategoriMi = z.infer<typeof kategoriMiEnum>;

// ---------------------------------------------------------------------------
// Create schema — PII fields optional (may be empty on create)
// ---------------------------------------------------------------------------

export const counterpartyCreateSchema = z.object({
  kodeCounterparty: z
    .string()
    .min(3, "Kode minimal 3 karakter")
    .max(30, "Kode maksimal 30 karakter")
    .regex(/^[A-Z0-9_-]+$/, "Kode hanya boleh huruf kapital, angka, - atau _"),
  nama: z
    .string()
    .min(3, "Nama minimal 3 karakter")
    .max(200, "Nama maksimal 200 karakter"),
  tipe: tipeCounterpartyEnum,
  tipeEksposurBasel: tipeEksposurBaselEnum,
  eligibleLpsFlag: z.boolean().default(false),
  status: statusCounterpartyEnum.default("AKTIF"),
  nomorIzinOjk: z.string().max(50).optional(),
  aumTerakhir: z.string().max(30).optional(), // decimal as string to preserve precision
  kategoriMi: kategoriMiEnum.optional(),
  // PII — optional string, backend validates format
  npwp: z
    .string()
    .max(20)
    .optional(),
  nomorRekening: z
    .string()
    .max(30, "Nomor rekening maksimal 30 karakter")
    .optional(),
  ktp: z
    .string()
    .max(16)
    .optional(),
});

export type CounterpartyCreateInput = z.infer<typeof counterpartyCreateSchema>;

export const counterpartyUpdateSchema = counterpartyCreateSchema
  .omit({ kodeCounterparty: true })
  .extend({
    rowVersion: z.number().int().positive("rowVersion diperlukan untuk update"),
  });

export type CounterpartyUpdateInput = z.infer<typeof counterpartyUpdateSchema>;

// ---------------------------------------------------------------------------
// API response types — PII NEVER in list response (masked "***")
// ---------------------------------------------------------------------------

/**
 * List item — PII fields intentionally absent.
 * TypeScript strict mode will reject any attempt to access npwp/ktp/nomorRekening here.
 */
export interface CounterpartyListItem {
  id: string;
  kodeCounterparty: string;
  nama: string;
  tipe: TipeCounterparty;
  tipeEksposurBasel: TipeEksposurBasel;
  ratingPefindoCurrent: string | null;
  eligibleLpsFlag: boolean;
  status: StatusCounterparty;
  nomorIzinOjk: string | null;
  kategoriMi: KategoriMi | null;
  workflowStatus: MasterWorkflowState;
  workflowInstanceId: string | null;
  rowVersion: number;
  createdAt: string;
  createdBy: string;
  updatedAt: string;
  updatedBy: string;
  deletedAt: string | null;
}

/**
 * Detail response — PII present but masked ("***") unless decrypted via /pii.
 * aumTerakhir shown as string (decimal).
 */
export interface CounterpartyDetail extends CounterpartyListItem {
  aumTerakhir: string | null;
  // PII — always "***" in this type; use CounterpartyPII for plaintext
  npwp: "***";
  nomorRekening: "***";
  ktp: "***";
  workflow: WorkflowStatus | null;
}

/**
 * Decrypted PII — returned only from /pii endpoint for permitted users.
 * Never stored in list cache.
 */
export interface CounterpartyPII {
  id: string;
  npwp: string | null;
  nomorRekening: string | null;
  ktp: string | null;
  decryptedAt: string;
  decryptedBy: string;
}

// ---------------------------------------------------------------------------
// Rating history types (moved here for colocation)
// ---------------------------------------------------------------------------

export const ratingPefindoEnum = z.enum([
  "idAAA", "idAA+", "idAA", "idAA-", "idA+", "idA", "idA-", "idBBB+", "idBBB", "idBBB-",
  "idBB+", "idBB", "idBB-", "idB+", "idB", "idB-", "idCCC", "idD", "SD", "NR",
] as const);

export const ratingOutlookEnum = z.enum([
  "STABLE", "POSITIVE", "NEGATIVE", "WATCH_POSITIVE", "WATCH_NEGATIVE",
] as const);

export const actionTypeRatingEnum = z.enum([
  "INITIAL", "UPGRADE", "DOWNGRADE", "AFFIRM", "WATCH", "WITHDRAW",
] as const);

export type RatingPefindo = z.infer<typeof ratingPefindoEnum>;
export type RatingOutlook = z.infer<typeof ratingOutlookEnum>;
export type ActionTypeRating = z.infer<typeof actionTypeRatingEnum>;
