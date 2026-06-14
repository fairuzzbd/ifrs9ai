import { z } from "zod";
import type { WorkflowStatus, MasterWorkflowState } from "@/lib/schemas/mata-uang.schema";

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

export const kategoriEventEnum = z.enum(
  [
    "PENEMPATAN",
    "MTM",
    "BUNGA_AKRUAL",
    "BUNGA_TERIMA",
    "JATUH_TEMPO",
    "PENJUALAN",
    "REKLASIFIKASI",
    "ECL_IMPAIRMENT",
    "FX_REVALUATION",
    "AMORTISASI_EIR",
    "OTHER",
  ],
  { error: () => ({ message: "Pilih kategori event yang valid" }) },
);

export type KategoriEvent = z.infer<typeof kategoriEventEnum>;

export const triggerSourceEnum = z.enum(
  ["MANUAL", "AUTO_EOD", "AUTO_EOM", "FEED_IBPA", "FEED_PEFINDO", "SYSTEM"],
  { error: () => ({ message: "Pilih trigger source yang valid" }) },
);

export type TriggerSource = z.infer<typeof triggerSourceEnum>;

export const tipeInstrumenEnum = z.enum([
  "DEPOSITO",
  "OBLIGASI",
  "SAHAM",
  "REKSADANA",
  "SBN",
  "REPO",
  "ALL",
]);

export type TipeInstrumen = z.infer<typeof tipeInstrumenEnum>;

export const klasifikasiEnum = z.enum([
  "AC",
  "FVOCI_DEBT",
  "FVOCI_EQUITY",
  "FVTPL",
  "POCI",
  "ALL",
]);

export type Klasifikasi = z.infer<typeof klasifikasiEnum>;

export const dkIndicatorEnum = z.enum(["DEBIT", "KREDIT"], {
  error: () => ({ message: "Pilih DEBIT atau KREDIT" }),
});

export type DkIndicator = z.infer<typeof dkIndicatorEnum>;

export const sumberAmountEnum = z.enum(
  [
    "PRINCIPAL",
    "ACCRUED_INTEREST",
    "FAIR_VALUE_CHANGE",
    "ECL_AMOUNT",
    "EIR_AMORTIZATION",
    "FX_GAIN_LOSS",
    "PREMIUM_DISCOUNT",
    "OTHER",
  ],
  { error: () => ({ message: "Pilih sumber amount yang valid" }) },
);

export type SumberAmount = z.infer<typeof sumberAmountEnum>;

// ---------------------------------------------------------------------------
// Decimal string validation helper
// ---------------------------------------------------------------------------

/**
 * Validates a string as a decimal number (supports negative, up to 8 decimal places).
 * Stored as string end-to-end to avoid float64 precision loss.
 */
const decimalStringSchema = z
  .string()
  .trim()
  .min(1, "Multiplier wajib diisi")
  .regex(
    /^-?\d+(\.\d{1,8})?$/,
    "Format angka tidak valid (maks 8 desimal, contoh: 1.00000000)",
  );

// ---------------------------------------------------------------------------
// Detail schema
// ---------------------------------------------------------------------------

export const mappingJurnalDetailSchema = z.object({
  /** Temporary client-side key for React key prop (not sent to server) */
  _clientKey: z.string().optional(),
  id: z.string().uuid().optional(),
  urutan: z
    .number({ error: () => ({ message: "Urutan harus berupa angka" }) })
    .int("Urutan harus bilangan bulat")
    .min(1, "Urutan minimal 1"),
  kodeAkunId: z
    .string()
    .uuid("Pilih akun CoA yang valid")
    .min(1, "Akun CoA wajib dipilih"),
  kodeAkunDisplay: z.string().optional(), // for UI display only
  dkIndicator: dkIndicatorEnum,
  sumberAmount: sumberAmountEnum,
  klasifikasiFilter: klasifikasiEnum.nullable().optional(),
  tipeInstrumenFilter: z.array(tipeInstrumenEnum).optional(),
  underlyingTypeFilter: z.string().max(100).nullable().optional(),
  multiplier: decimalStringSchema,
  matauangPosting: z
    .string()
    .min(1, "Mata uang wajib diisi")
    .max(10, "Mata uang terlalu panjang"),
});

export type MappingJurnalDetailInput = z.infer<typeof mappingJurnalDetailSchema>;

// ---------------------------------------------------------------------------
// Header schema
// ---------------------------------------------------------------------------

export const mappingJurnalHeaderSchema = z.object({
  eventIdKode: z
    .string()
    .min(1, "Event ID Kode wajib diisi")
    .max(50, "Event ID Kode maksimal 50 karakter")
    .regex(
      /^[A-Z0-9_]+$/,
      "Event ID Kode hanya boleh huruf kapital, angka, dan underscore",
    ),
  eventCode: z
    .string()
    .min(1, "Event Code wajib diisi")
    .max(100, "Event Code maksimal 100 karakter"),
  namaEvent: z
    .string()
    .min(3, "Nama event minimal 3 karakter")
    .max(200, "Nama event maksimal 200 karakter"),
  kategoriEvent: kategoriEventEnum,
  triggerSource: triggerSourceEnum,
  tipeInstrumenBerlaku: z
    .array(tipeInstrumenEnum)
    .min(1, "Pilih minimal 1 tipe instrumen"),
  klasifikasiBerlaku: z
    .array(klasifikasiEnum)
    .min(1, "Pilih minimal 1 klasifikasi"),
  aktifFlag: z.boolean(),
  catatan: z.string().max(1000, "Catatan maksimal 1000 karakter").optional(),
});

export type MappingJurnalHeaderInput = z.infer<typeof mappingJurnalHeaderSchema>;

// ---------------------------------------------------------------------------
// Combined form schema (header + details) with invariant validation
// ---------------------------------------------------------------------------

/**
 * Parse a decimal string to a number for arithmetic.
 * Returns 0 on invalid input (validation already catches it).
 */
function parseDecimal(s: string): number {
  const n = parseFloat(s);
  return isNaN(n) ? 0 : n;
}

export const mappingJurnalFormSchema = z
  .object({
    header: mappingJurnalHeaderSchema,
    details: z
      .array(mappingJurnalDetailSchema)
      .min(2, "Minimal 2 detail diperlukan (1 Debit + 1 Kredit)"),
  })
  .superRefine((val, ctx) => {
    const activeDetails = val.details;

    // Check at least one DEBIT and one KREDIT exist
    const hasDebit = activeDetails.some((d) => d.dkIndicator === "DEBIT");
    const hasKredit = activeDetails.some((d) => d.dkIndicator === "KREDIT");

    if (!hasDebit) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "Minimal 1 baris DEBIT diperlukan",
        path: ["details"],
      });
    }

    if (!hasKredit) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "Minimal 1 baris KREDIT diperlukan",
        path: ["details"],
      });
    }

    // Debit = Kredit balance check (multiplier sum)
    const debitSum = activeDetails
      .filter((d) => d.dkIndicator === "DEBIT")
      .reduce((acc, d) => acc + parseDecimal(d.multiplier), 0);

    const kreditSum = activeDetails
      .filter((d) => d.dkIndicator === "KREDIT")
      .reduce((acc, d) => acc + parseDecimal(d.multiplier), 0);

    // Use epsilon comparison (8 decimal precision)
    if (Math.abs(debitSum - kreditSum) > 1e-8) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: `Total Debit (${debitSum.toFixed(8)}) harus sama dengan total Kredit (${kreditSum.toFixed(8)})`,
        path: ["details"],
      });
    }

    // Check for duplicate urutan
    const urutans = activeDetails.map((d) => d.urutan);
    const dupUrutan = urutans.filter((u, i) => urutans.indexOf(u) !== i);
    if (dupUrutan.length > 0) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: `Urutan duplikat ditemukan: ${[...new Set(dupUrutan)].join(", ")}`,
        path: ["details"],
      });
    }
  });

export type MappingJurnalFormInput = z.infer<typeof mappingJurnalFormSchema>;

// ---------------------------------------------------------------------------
// Update schema (adds rowVersion)
// ---------------------------------------------------------------------------

export const mappingJurnalUpdateSchema = mappingJurnalFormSchema.extend({
  rowVersion: z
    .number()
    .int()
    .positive("rowVersion diperlukan untuk update"),
});

export type MappingJurnalUpdateInput = z.infer<typeof mappingJurnalUpdateSchema>;

// ---------------------------------------------------------------------------
// API response types
// ---------------------------------------------------------------------------

export interface MappingJurnalDetailItem {
  id: string;
  headerId: string;
  urutan: number;
  kodeAkunId: string;
  kodeAkunDisplay: string | null;
  namaAkun: string | null;
  dkIndicator: DkIndicator;
  sumberAmount: SumberAmount;
  klasifikasiFilter: Klasifikasi | null;
  tipeInstrumenFilter: TipeInstrumen[];
  underlyingTypeFilter: string | null;
  multiplier: string; // decimal string
  matauangPosting: string;
  createdAt: string;
  createdBy: string;
  updatedAt: string;
  updatedBy: string;
}

export interface MappingJurnalItem {
  id: string;
  eventIdKode: string;
  eventCode: string;
  namaEvent: string;
  kategoriEvent: KategoriEvent;
  triggerSource: TriggerSource;
  tipeInstrumenBerlaku: TipeInstrumen[];
  klasifikasiBerlaku: Klasifikasi[];
  aktifFlag: boolean;
  catatan: string | null;
  workflowStatus: MasterWorkflowState;
  workflowInstanceId: string | null;
  rowVersion: number;
  createdAt: string;
  createdBy: string;
  updatedAt: string;
  updatedBy: string;
  deletedAt: string | null;
}

export interface MappingJurnalDetail extends MappingJurnalItem {
  details: MappingJurnalDetailItem[];
  workflow: WorkflowStatus | null;
}

// Re-export workflow types for convenience
export type { WorkflowStatus, MasterWorkflowState };
