import { z } from "zod";

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

export const tipePeriodeEnum = z.enum(["BULANAN", "TRIWULANAN", "TAHUNAN"], {
  error: () => ({ message: "Pilih tipe periode yang valid" }),
});
export type TipePeriode = z.infer<typeof tipePeriodeEnum>;

export const statusPeriodeEnum = z.enum(["OPEN", "SOFT_CLOSED", "CLOSED"], {
  error: () => ({ message: "Status periode tidak valid" }),
});
export type StatusPeriode = z.infer<typeof statusPeriodeEnum>;

export const periodeWorkflowStatusEnum = z.enum([
  "DRAFT",
  "PENDING_REVIEW",
  "PENDING_APPROVAL",
  "PENDING_APPROVAL_2",
  "APPROVED",
  "RETURNED",
]);
export type PeriodeWorkflowStatusValue = z.infer<typeof periodeWorkflowStatusEnum>;

// ---------------------------------------------------------------------------
// Create schema — conditional fields: bulan, triwulan
// ---------------------------------------------------------------------------

export const periodeBukuCreateSchema = z
  .object({
    periodeIdKode: z
      .string()
      .min(1, "Kode periode wajib diisi")
      .max(20, "Kode periode maksimal 20 karakter")
      .regex(
        /^\d{4}-(M(0[1-9]|1[0-2])|Q[1-4]|Y)$/,
        "Format tidak valid. Contoh valid: 2026-M06, 2026-Q2, 2026-Y",
      ),
    tipePeriode: tipePeriodeEnum,
    tahunBuku: z
      .number({ error: () => ({ message: "Tahun buku harus berupa angka" }) })
      .int("Tahun buku harus bilangan bulat")
      .min(2000, "Tahun buku minimal 2000")
      .max(2099, "Tahun buku maksimal 2099"),
    bulan: z
      .number({ error: () => ({ message: "Bulan harus berupa angka" }) })
      .int()
      .min(1, "Bulan minimal 1")
      .max(12, "Bulan maksimal 12")
      .nullable()
      .optional(),
    triwulan: z
      .number({ error: () => ({ message: "Triwulan harus berupa angka" }) })
      .int()
      .min(1, "Triwulan minimal 1")
      .max(4, "Triwulan maksimal 4")
      .nullable()
      .optional(),
    tanggalMulai: z
      .string()
      .min(1, "Tanggal mulai wajib diisi")
      .date("Format tanggal tidak valid (YYYY-MM-DD)"),
    tanggalAkhir: z
      .string()
      .min(1, "Tanggal akhir wajib diisi")
      .date("Format tanggal tidak valid (YYYY-MM-DD)"),
  })
  .superRefine((data, ctx) => {
    // bulan required if BULANAN
    if (data.tipePeriode === "BULANAN") {
      if (data.bulan === null || data.bulan === undefined) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: "Bulan wajib diisi untuk tipe BULANAN",
          path: ["bulan"],
        });
      }
    }
    // triwulan required if TRIWULANAN
    if (data.tipePeriode === "TRIWULANAN") {
      if (data.triwulan === null || data.triwulan === undefined) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: "Triwulan wajib diisi untuk tipe TRIWULANAN",
          path: ["triwulan"],
        });
      }
    }
    // tanggal_akhir >= tanggal_mulai
    if (data.tanggalMulai && data.tanggalAkhir) {
      if (data.tanggalAkhir < data.tanggalMulai) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: "Tanggal akhir harus sama dengan atau setelah tanggal mulai",
          path: ["tanggalAkhir"],
        });
      }
    }
  });

export type PeriodeBukuCreateInput = z.infer<typeof periodeBukuCreateSchema>;

// ---------------------------------------------------------------------------
// Update schema (PATCH — excludes immutable fields + adds rowVersion)
// ---------------------------------------------------------------------------

export const periodeBukuUpdateSchema = z
  .object({
    tahunBuku: z
      .number({ error: () => ({ message: "Tahun buku harus berupa angka" }) })
      .int()
      .min(2000, "Tahun buku minimal 2000")
      .max(2099, "Tahun buku maksimal 2099")
      .optional(),
    bulan: z
      .number()
      .int()
      .min(1, "Bulan minimal 1")
      .max(12, "Bulan maksimal 12")
      .nullable()
      .optional(),
    triwulan: z
      .number()
      .int()
      .min(1, "Triwulan minimal 1")
      .max(4, "Triwulan maksimal 4")
      .nullable()
      .optional(),
    tanggalMulai: z
      .string()
      .date("Format tanggal tidak valid (YYYY-MM-DD)")
      .optional(),
    tanggalAkhir: z
      .string()
      .date("Format tanggal tidak valid (YYYY-MM-DD)")
      .optional(),
    rowVersion: z
      .number()
      .int()
      .positive("rowVersion diperlukan untuk update"),
    // included for context — not sent to backend but used for cross-field validation
    tipePeriode: tipePeriodeEnum.optional(),
  })
  .superRefine((data, ctx) => {
    if (data.tanggalMulai && data.tanggalAkhir) {
      if (data.tanggalAkhir < data.tanggalMulai) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: "Tanggal akhir harus sama dengan atau setelah tanggal mulai",
          path: ["tanggalAkhir"],
        });
      }
    }
    if (data.tipePeriode === "BULANAN") {
      if (data.bulan === null || data.bulan === undefined) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: "Bulan wajib diisi untuk tipe BULANAN",
          path: ["bulan"],
        });
      }
    }
    if (data.tipePeriode === "TRIWULANAN") {
      if (data.triwulan === null || data.triwulan === undefined) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: "Triwulan wajib diisi untuk tipe TRIWULANAN",
          path: ["triwulan"],
        });
      }
    }
  });

export type PeriodeBukuUpdateInput = z.infer<typeof periodeBukuUpdateSchema>;

// ---------------------------------------------------------------------------
// Generate schema
// ---------------------------------------------------------------------------

export const periodeBukuGenerateSchema = z.object({
  tahunBuku: z
    .number({ error: () => ({ message: "Tahun buku harus berupa angka" }) })
    .int("Tahun buku harus bilangan bulat")
    .min(2000, "Tahun buku minimal 2000")
    .max(2099, "Tahun buku maksimal 2099"),
  tipe: z
    .array(tipePeriodeEnum)
    .min(1, "Pilih minimal satu tipe periode")
    .default(["BULANAN", "TRIWULANAN", "TAHUNAN"]),
});

export type PeriodeBukuGenerateInput = z.infer<typeof periodeBukuGenerateSchema>;

// ---------------------------------------------------------------------------
// Filter schema (URL state)
// ---------------------------------------------------------------------------

export const periodeBukuFilterSchema = z.object({
  q: z.string().optional(),
  sort: z.string().optional(),
  "filter[tipe_periode]": z.string().optional(),
  "filter[status_periode]": z.string().optional(),
  "filter[tahun_buku]": z.string().optional(),
  "filter[workflow_status]": z.string().optional(),
  cursor: z.string().optional(),
  limit: z.number().int().min(1).max(200).default(50),
});

export type PeriodeBukuFilterInput = z.infer<typeof periodeBukuFilterSchema>;

// ---------------------------------------------------------------------------
// Workflow schemas (reuse pattern from mata-uang)
// ---------------------------------------------------------------------------

export const workflowApproveSchema = z.object({
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

export type WorkflowApproveInput = z.infer<typeof workflowApproveSchema>;

export const workflowRejectSchema = z.object({
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

export type WorkflowRejectInput = z.infer<typeof workflowRejectSchema>;

export const workflowSubmitSchema = z.object({
  comment: z.string().max(500).optional(),
  rowVersion: z.number().int().positive("rowVersion diperlukan"),
});

export type WorkflowSubmitInput = z.infer<typeof workflowSubmitSchema>;

// ---------------------------------------------------------------------------
// API response types
// ---------------------------------------------------------------------------

export interface PeriodeWorkflowHistoryEntry {
  action: "SUBMIT" | "REVIEW" | "APPROVE" | "REJECT";
  userId: string;
  username: string;
  role: string;
  signedAt: string;
  signatureHash: string;
  comment: string | null;
}

export interface PeriodeWorkflowData {
  currentState: PeriodeWorkflowStatusValue;
  workflowEyes: 4 | 6;
  makerId: string | null;
  reviewerId: string | null;
  approverId: string | null;
  history: PeriodeWorkflowHistoryEntry[];
}

export interface PeriodeBukuItem {
  id: string;
  periodeIdKode: string;
  tipePeriode: TipePeriode;
  tahunBuku: number;
  bulan: number | null;
  triwulan: number | null;
  tanggalMulai: string;
  tanggalAkhir: string;
  statusPeriode: StatusPeriode;
  /** The workflow_status value, e.g. "DRAFT" | "PENDING_REVIEW" | ... */
  workflowStatus: PeriodeWorkflowStatusValue;
  rowVersion: number;
  createdAt: string;
  createdBy: string | null;
  updatedAt: string | null;
  updatedBy: string | null;
  deletedAt: string | null;
  deletedBy: string | null;
}

export interface PeriodeBukuDetail extends PeriodeBukuItem {
  // workflow detail populated by GET /:id (includes history)
  workflow: PeriodeWorkflowData | null;
}

export interface GenerateResult {
  generated: number;
  skipped: number;
  rows: PeriodeBukuItem[];
}
