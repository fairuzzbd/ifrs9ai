/**
 * Zod schemas for APP-A Master Instrumen.
 *
 * Mirrors backend domain.go validation rules:
 * - kodeInstrumen: 2-20 alphanumeric
 * - tipeInstrumen: enum whitelist
 * - Conditional required: manajerInvestasi ↔ REKSADANA, bankKustodian ↔ SAHAM|REKSADANA
 * - kupon/frekuensiBunga ↔ OBLIGASI|SBN|SPN|SUKUK
 * - tanggalJatuhTempo > tanggalPenempatan
 */

import { z } from "zod";

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

export const tipeInstrumenEnum = z.enum(
  ["DEPOSITO", "OBLIGASI", "SAHAM", "REKSADANA", "SBN", "SPN", "SUKUK"],
  { error: () => ({ message: "Pilih tipe instrumen yang valid" }) },
);
export type TipeInstrumen = z.infer<typeof tipeInstrumenEnum>;

export const frekuensiBungaEnum = z.enum(
  ["BULANAN", "TRIWULANAN", "SEMESTERAN", "TAHUNAN"],
  { error: () => ({ message: "Pilih frekuensi bunga yang valid" }) },
);

export const bmCategoryEnum = z.enum(["HTC", "HTC_S", "OTHER"], {
  error: () => ({ message: "Pilih kategori business model yang valid" }),
});

export const klasifikasiPsak71Enum = z.enum([
  "AC",
  "FVOCI",
  "FVOCI_ELECTION",
  "FVTPL",
]);

export const instrumenStatusEnum = z.enum([
  "AKTIF",
  "TIDAK_AKTIF",
  "MATURED",
  "SOLD",
]);

export const workflowStatusEnum = z.enum([
  "DRAFT",
  "PENDING_REVIEW",
  "PENDING_APPROVAL",
  "PENDING_APPROVAL_2",
  "APPROVED",
  "RETURNED",
]);
export type InstrumenWorkflowState = z.infer<typeof workflowStatusEnum>;

// ---------------------------------------------------------------------------
// Tipe → conditional requirement helpers
// ---------------------------------------------------------------------------

export const TIPE_REQUIRES_KUSTODIAN: TipeInstrumen[] = ["SAHAM", "REKSADANA"];
export const TIPE_REQUIRES_MANAJER_INVESTASI: TipeInstrumen[] = ["REKSADANA"];
export const TIPE_REQUIRES_KUPON: TipeInstrumen[] = [
  "OBLIGASI",
  "SBN",
  "SPN",
  "SUKUK",
];
export const TIPE_KUPON_LABEL: Record<string, string> = {
  OBLIGASI: "Obligasi",
  SBN: "SBN",
  SPN: "SPN",
  SUKUK: "Sukuk",
};

// ---------------------------------------------------------------------------
// Create schema — full conditional validation
// ---------------------------------------------------------------------------

export const instrumenCreateSchema = z
  .object({
    // Section 1: Identitas
    kodeInstrumen: z
      .string()
      .min(2, "Kode minimal 2 karakter")
      .max(20, "Kode maksimal 20 karakter")
      .regex(
        /^[A-Za-z0-9\-_]+$/,
        "Kode hanya boleh berisi huruf, angka, tanda hubung, atau garis bawah",
      ),
    tipeInstrumen: tipeInstrumenEnum,
    subTipe: z.string().max(50, "Sub tipe maksimal 50 karakter").optional().default(""),
    nama: z
      .string()
      .min(2, "Nama minimal 2 karakter")
      .max(200, "Nama maksimal 200 karakter"),
    isin: z
      .string()
      .max(20, "ISIN maksimal 20 karakter")
      .optional()
      .or(z.literal("")),

    // Section 2: Counterparty & Kustodian
    counterpartyId: z.string().uuid("Pilih counterparty yang valid"),
    manajerInvestasiId: z
      .string()
      .uuid("Pilih manajer investasi yang valid")
      .optional()
      .or(z.literal(""))
      .nullable(),
    bankKustodianId: z
      .string()
      .uuid("Pilih bank kustodian yang valid")
      .optional()
      .or(z.literal(""))
      .nullable(),
    mataUang: z
      .string()
      .length(3, "Kode mata uang harus 3 karakter")
      .regex(/^[A-Z]{3}$/, "Kode mata uang harus 3 huruf kapital"),
    portofolioId: z.string().uuid("Pilih portofolio yang valid"),

    // Financial
    nominal: z
      .string()
      .min(1, "Nominal wajib diisi")
      .refine(
        (v) => !isNaN(parseFloat(v.replace(/[,.]/g, ""))) && parseFloat(v.replace(/[,.]/g, "")) > 0,
        "Nominal harus berupa angka positif",
      ),
    jumlahLot: z
      .string()
      .optional()
      .or(z.literal(""))
      .refine(
        (v) => !v || !isNaN(parseInt(v, 10)),
        "Jumlah lot harus berupa angka bulat",
      ),

    // Section 3: Periode & Kupon
    tanggalPenempatan: z
      .string()
      .date("Format tanggal tidak valid (YYYY-MM-DD)"),
    tanggalJatuhTempo: z
      .string()
      .date("Format tanggal tidak valid (YYYY-MM-DD)")
      .optional()
      .or(z.literal(""))
      .nullable(),
    kupon: z
      .string()
      .optional()
      .or(z.literal(""))
      .nullable()
      .refine(
        (v) => !v || (parseFloat(v) >= 0 && parseFloat(v) <= 100),
        "Kupon harus antara 0 dan 100 persen",
      ),
    frekuensiBunga: frekuensiBungaEnum.optional().nullable(),
    autoRenewalFlag: z.boolean().default(false),

    // Section 4: PSAK 71 (Phase 3 read-only, set on create as defaults)
    fvociElection: z.boolean().default(false),
    bmCategory: bmCategoryEnum.optional().nullable(),

    // EIR (Phase 4 — optional on create)
    eirAwal: z
      .string()
      .optional()
      .or(z.literal(""))
      .nullable()
      .refine(
        (v) => !v || (parseFloat(v) >= 0 && parseFloat(v) <= 1),
        "EIR harus antara 0 dan 1 (cth: 0.0525 untuk 5.25%)",
      ),
    premiumDiskonto: z.string().optional().or(z.literal("")).default("0"),
    biayaTransaksi: z.string().optional().or(z.literal("")).default("0"),

    // Status
    status: instrumenStatusEnum.default("AKTIF"),
  })
  .superRefine((data, ctx) => {
    const tipe = data.tipeInstrumen as TipeInstrumen;

    // manajerInvestasiId required for REKSADANA
    if (TIPE_REQUIRES_MANAJER_INVESTASI.includes(tipe)) {
      if (!data.manajerInvestasiId) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ["manajerInvestasiId"],
          message: "Manajer Investasi wajib diisi untuk instrumen REKSADANA",
        });
      }
    }

    // bankKustodianId required for SAHAM and REKSADANA
    if (TIPE_REQUIRES_KUSTODIAN.includes(tipe)) {
      if (!data.bankKustodianId) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ["bankKustodianId"],
          message: `Bank Kustodian wajib diisi untuk instrumen ${tipe}`,
        });
      }
    }

    // tanggalJatuhTempo must be after tanggalPenempatan (if provided)
    if (data.tanggalJatuhTempo && data.tanggalPenempatan) {
      if (data.tanggalJatuhTempo <= data.tanggalPenempatan) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ["tanggalJatuhTempo"],
          message:
            "Tanggal jatuh tempo harus setelah tanggal penempatan",
        });
      }
    }

    // fvociElection only valid for equity (SAHAM)
    if (data.fvociElection && tipe !== "SAHAM") {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ["fvociElection"],
        message:
          "FVOCI Election hanya berlaku untuk instrumen SAHAM (equity)",
      });
    }

    // autoRenewalFlag only valid for DEPOSITO
    if (data.autoRenewalFlag && tipe !== "DEPOSITO") {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ["autoRenewalFlag"],
        message: "Auto Renewal hanya berlaku untuk instrumen DEPOSITO",
      });
    }
  });

export type InstrumenCreateInput = z.infer<typeof instrumenCreateSchema>;

// ---------------------------------------------------------------------------
// Update schema — partial, most fields mutable, with rowVersion
// ---------------------------------------------------------------------------

export const instrumenUpdateSchema = z
  .object({
    subTipe: z.string().max(50).optional(),
    nama: z.string().min(2).max(200).optional(),
    isin: z.string().max(20).optional().or(z.literal("")).nullable(),
    manajerInvestasiId: z
      .string()
      .uuid()
      .optional()
      .or(z.literal(""))
      .nullable(),
    bankKustodianId: z
      .string()
      .uuid()
      .optional()
      .or(z.literal(""))
      .nullable(),
    mataUang: z.string().length(3).regex(/^[A-Z]{3}$/).optional(),
    kupon: z.string().optional().or(z.literal("")).nullable(),
    frekuensiBunga: frekuensiBungaEnum.optional().nullable(),
    autoRenewalFlag: z.boolean().optional(),
    fvociElection: z.boolean().optional(),
    bmCategory: bmCategoryEnum.optional().nullable(),
    eirAwal: z.string().optional().or(z.literal("")).nullable(),
    status: instrumenStatusEnum.optional(),
    rowVersion: z.number().int().positive("rowVersion diperlukan untuk update"),
  });

export type InstrumenUpdateInput = z.infer<typeof instrumenUpdateSchema>;

// ---------------------------------------------------------------------------
// Workflow schemas (reuse pattern from mata-uang)
// ---------------------------------------------------------------------------

export const instrumenWorkflowApproveSchema = z.object({
  comment: z.string().max(1000).optional(),
  signatureMethod: z
    .enum(["JWT_STANDARD", "JWT_STEP_UP"])
    .default("JWT_STANDARD"),
  rowVersion: z.number().int().positive("rowVersion diperlukan"),
  attestChecked: z.literal(true, {
    error: () => ({
      message: "Anda harus mencentang pernyataan ini sebelum melanjutkan",
    }),
  }),
});

export type InstrumenWorkflowApproveInput = z.infer<
  typeof instrumenWorkflowApproveSchema
>;

export const instrumenWorkflowRejectSchema = z.object({
  comment: z
    .string()
    .min(10, "Alasan penolakan minimal 10 karakter")
    .max(1000, "Alasan penolakan maksimal 1000 karakter"),
  signatureMethod: z
    .enum(["JWT_STANDARD", "JWT_STEP_UP"])
    .default("JWT_STANDARD"),
  rowVersion: z.number().int().positive("rowVersion diperlukan"),
  attestChecked: z.literal(true, {
    error: () => ({
      message: "Anda harus mencentang pernyataan ini sebelum melanjutkan",
    }),
  }),
});

// ---------------------------------------------------------------------------
// API response types (aligned with domain.go Response struct)
// ---------------------------------------------------------------------------

export interface WorkflowHistoryEntry {
  action: "SUBMIT" | "REVIEW" | "APPROVE" | "REJECT";
  userId: string;
  username: string;
  role: string;
  signedAt: string;
  signatureHash: string;
  comment: string | null;
}

export interface WorkflowStatus {
  currentState: InstrumenWorkflowState;
  workflowEyes: 4 | 6;
  makerId: string | null;
  reviewerId: string | null;
  approverId: string | null;
  history: WorkflowHistoryEntry[];
}

export interface InstrumenItem {
  id: string;
  kodeInstrumen: string;
  tipeInstrumen: TipeInstrumen;
  subTipe: string;
  nama: string;
  isin: string | null;
  counterpartyId: string;
  manajerInvestasiId: string | null;
  bankKustodianId: string | null;
  mataUang: string;
  portofolioId: string;
  nominal: string; // decimal string, 4dp
  jumlahLot: string | null;
  tanggalPenempatan: string;
  tanggalJatuhTempo: string | null;
  kupon: string | null; // decimal string, 4dp
  frekuensiBunga: string | null;
  autoRenewalFlag: boolean;
  fvociElection: boolean;
  sppiResult: string | null;
  bmCategory: string | null;
  klasifikasiPsak71: string | null;
  klasifikasiLockedAt: string | null;
  eirAwal: string | null;
  tanggalEirComputed: string | null;
  premiumDiskonto: string;
  biayaTransaksi: string;
  eirMethodFlag: boolean;
  dayCountConvention: string;
  amortizationFrequency: string | null;
  status: string;
  workflowStatus: InstrumenWorkflowState;
  workflowInstanceId: string | null;
  rowVersion: number;
  createdAt: string;
  createdBy: string;
  updatedAt: string | null;
  updatedBy: string | null;
  deletedAt: string | null;
}

export interface InstrumenDetail extends InstrumenItem {
  workflow: WorkflowStatus | null;
}

// ---------------------------------------------------------------------------
// Dropdown reference types (for FK autocomplete)
// ---------------------------------------------------------------------------

export interface CounterpartyOption {
  id: string;
  kode: string;
  nama: string;
}

export interface PortofolioOption {
  id: string;
  kode: string;
  nama: string;
  bmCategory: string | null;
}

export interface MataUangOption {
  kodeMataUang: string;
  namaMataUang: string;
  simbol: string;
}
