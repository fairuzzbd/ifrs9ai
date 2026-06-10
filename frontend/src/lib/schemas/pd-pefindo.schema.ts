import { z } from "zod";

// ---------------------------------------------------------------------------
// Rating enum (Pefindo whitelist per domain.go / Pefindo Annual Default Study)
// ---------------------------------------------------------------------------

export const pefindoRatingEnum = z.enum([
  "idAAA",
  "idAA+", "idAA", "idAA-",
  "idA+", "idA", "idA-",
  "idBBB+", "idBBB", "idBBB-",
  "idBB+", "idBB", "idBB-",
  "idB+", "idB", "idB-",
  "idCCC",
  "idCC",
  "idC",
  "idD",
] as const, {
  error: () => ({ message: "Pilih rating Pefindo yang valid" }),
});

export type PefindoRating = z.infer<typeof pefindoRatingEnum>;

export const PEFINDO_RATINGS: PefindoRating[] = [
  "idAAA",
  "idAA+", "idAA", "idAA-",
  "idA+", "idA", "idA-",
  "idBBB+", "idBBB", "idBBB-",
  "idBB+", "idBB", "idBB-",
  "idB+", "idB", "idB-",
  "idCCC",
  "idCC",
  "idC",
  "idD",
];

// ---------------------------------------------------------------------------
// PD value helpers — all PD values stored as string (DEC-016)
// ---------------------------------------------------------------------------

/**
 * Validates a PD string value: must be parseable as a number between 0 and 1
 * inclusive, with up to 8 decimal places.
 */
const pdDecimalString = (fieldLabel: string) =>
  z
    .string()
    .trim()
    .min(1, `${fieldLabel} wajib diisi`)
    .refine((v) => /^\d+(\.\d{1,8})?$/.test(v), {
      message: `${fieldLabel} harus berupa desimal (0.00000000–1.00000000)`,
    })
    .refine((v) => {
      const n = parseFloat(v);
      return !isNaN(n) && n >= 0 && n <= 1;
    }, {
      message: `${fieldLabel} harus antara 0 dan 1`,
    });

const pdDecimalStringOptional = () =>
  z
    .string()
    .trim()
    .refine(
      (v) => v === "" || /^\d+(\.\d{1,8})?$/.test(v),
      { message: "Harus berupa desimal (0.00000000–1.00000000) atau kosong" },
    )
    .refine(
      (v) => {
        if (v === "") return true;
        const n = parseFloat(v);
        return !isNaN(n) && n >= 0 && n <= 1;
      },
      { message: "Nilai PD harus antara 0 dan 1" },
    )
    .optional();

// ---------------------------------------------------------------------------
// Monotonicity check helper (used in superRefine)
// ---------------------------------------------------------------------------

export function checkMonotonicity(vals: {
  pd12Month: string;
  pdLifetime3Y?: string;
  pdLifetime5Y?: string;
  pdLifetime7Y?: string;
  pdLifetime10Y?: string;
}): string[] {
  const errors: string[] = [];
  const parse = (v?: string): number | null => {
    if (!v || v === "") return null;
    const n = parseFloat(v);
    return isNaN(n) ? null : n;
  };

  const v12 = parse(vals.pd12Month);
  const v3 = parse(vals.pdLifetime3Y);
  const v5 = parse(vals.pdLifetime5Y);
  const v7 = parse(vals.pdLifetime7Y);
  const v10 = parse(vals.pdLifetime10Y);

  const pairs: Array<[number | null, number | null, string]> = [
    [v12, v3, "PD 12 Bulan harus ≤ PD Lifetime 3 Tahun"],
    [v3, v5, "PD Lifetime 3 Tahun harus ≤ PD Lifetime 5 Tahun"],
    [v5, v7, "PD Lifetime 5 Tahun harus ≤ PD Lifetime 7 Tahun"],
    [v7, v10, "PD Lifetime 7 Tahun harus ≤ PD Lifetime 10 Tahun"],
  ];

  for (const [left, right, msg] of pairs) {
    if (left !== null && right !== null && left > right) {
      errors.push(msg);
    }
  }
  return errors;
}

// ---------------------------------------------------------------------------
// Create schema
// ---------------------------------------------------------------------------

export const pdPefindoCreateSchema = z
  .object({
    rating: pefindoRatingEnum,
    pd12Month: pdDecimalString("PD 12 Bulan"),
    pdLifetime3Y: pdDecimalStringOptional(),
    pdLifetime5Y: pdDecimalStringOptional(),
    pdLifetime7Y: pdDecimalStringOptional(),
    pdLifetime10Y: pdDecimalStringOptional(),
    sumber: z
      .string()
      .min(1, "Sumber wajib diisi")
      .max(200, "Sumber maksimal 200 karakter")
      .default("PEFINDO_ANNUAL_DEFAULT_STUDY"),
    tanggalPublikasi: z
      .string()
      .date("Format tanggal tidak valid (YYYY-MM-DD)")
      .optional()
      .or(z.literal("")),
    periodeBerlakuDari: z
      .string()
      .date("Format tanggal tidak valid (YYYY-MM-DD)")
      .min(1, "Periode berlaku dari wajib diisi"),
    periodeBerlakuSampai: z
      .string()
      .date("Format tanggal tidak valid (YYYY-MM-DD)")
      .optional()
      .or(z.literal("")),
  })
  .superRefine((data, ctx) => {
    const monErrors = checkMonotonicity(data);
    if (monErrors.length > 0) {
      for (const msg of monErrors) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: msg,
          path: ["pd12Month"],
        });
      }
    }
    // Date ordering
    if (
      data.periodeBerlakuDari &&
      data.periodeBerlakuSampai &&
      data.periodeBerlakuSampai !== "" &&
      data.periodeBerlakuSampai < data.periodeBerlakuDari
    ) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "Periode berlaku sampai tidak boleh sebelum periode berlaku dari",
        path: ["periodeBerlakuSampai"],
      });
    }
    // Special case idD: all PD must be 1.0
    if (data.rating === "idD") {
      const shouldBeOne = [
        { val: data.pd12Month, field: "pd12Month", label: "PD 12 Bulan" },
        { val: data.pdLifetime3Y, field: "pdLifetime3Y", label: "PD Lifetime 3Y" },
        { val: data.pdLifetime5Y, field: "pdLifetime5Y", label: "PD Lifetime 5Y" },
        { val: data.pdLifetime7Y, field: "pdLifetime7Y", label: "PD Lifetime 7Y" },
        { val: data.pdLifetime10Y, field: "pdLifetime10Y", label: "PD Lifetime 10Y" },
      ];
      for (const f of shouldBeOne) {
        if (f.val && f.val !== "" && parseFloat(f.val) !== 1) {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            message: `Rating idD: ${f.label} harus 1.0 (certain default)`,
            path: [f.field],
          });
        }
      }
    }
  });

export type PDPefindoCreateInput = z.infer<typeof pdPefindoCreateSchema>;

// ---------------------------------------------------------------------------
// Update schema
// ---------------------------------------------------------------------------

export const pdPefindoUpdateSchema = z
  .object({
    pd12Month: pdDecimalString("PD 12 Bulan").optional(),
    pdLifetime3Y: pdDecimalStringOptional(),
    pdLifetime5Y: pdDecimalStringOptional(),
    pdLifetime7Y: pdDecimalStringOptional(),
    pdLifetime10Y: pdDecimalStringOptional(),
    sumber: z.string().min(1).max(200).optional(),
    tanggalPublikasi: z
      .string()
      .date("Format tanggal tidak valid")
      .optional()
      .or(z.literal("")),
    periodeBerlakuDari: z
      .string()
      .date("Format tanggal tidak valid")
      .optional(),
    periodeBerlakuSampai: z
      .string()
      .date("Format tanggal tidak valid")
      .optional()
      .or(z.literal("")),
    rowVersion: z.number().int().positive("rowVersion diperlukan untuk update"),
  })
  .superRefine((data, ctx) => {
    if (data.pd12Month) {
      const monErrors = checkMonotonicity({
        pd12Month: data.pd12Month,
        pdLifetime3Y: data.pdLifetime3Y,
        pdLifetime5Y: data.pdLifetime5Y,
        pdLifetime7Y: data.pdLifetime7Y,
        pdLifetime10Y: data.pdLifetime10Y,
      });
      for (const msg of monErrors) {
        ctx.addIssue({ code: z.ZodIssueCode.custom, message: msg, path: ["pd12Month"] });
      }
    }
    if (
      data.periodeBerlakuDari &&
      data.periodeBerlakuSampai &&
      data.periodeBerlakuSampai !== "" &&
      data.periodeBerlakuSampai < data.periodeBerlakuDari
    ) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "Periode berlaku sampai tidak boleh sebelum periode berlaku dari",
        path: ["periodeBerlakuSampai"],
      });
    }
  });

export type PDPefindoUpdateInput = z.infer<typeof pdPefindoUpdateSchema>;

// ---------------------------------------------------------------------------
// Upload XLSX schema (form fields)
// ---------------------------------------------------------------------------

export const uploadXlsxSchema = z.object({
  tanggalPublikasi: z
    .string()
    .date("Format tanggal tidak valid (YYYY-MM-DD)")
    .min(1, "Tanggal publikasi wajib diisi"),
  periodeBerlakuDari: z
    .string()
    .date("Format tanggal tidak valid (YYYY-MM-DD)")
    .min(1, "Periode berlaku dari wajib diisi"),
  periodeBerlakuSampai: z
    .string()
    .date("Format tanggal tidak valid (YYYY-MM-DD)")
    .optional()
    .or(z.literal("")),
});

export type UploadXlsxInput = z.infer<typeof uploadXlsxSchema>;

// ---------------------------------------------------------------------------
// Workflow schemas (reusable)
// ---------------------------------------------------------------------------

export const workflowApproveSchema = z.object({
  comment: z.string().max(1000).optional(),
  signatureMethod: z
    .enum(["JWT_STANDARD", "JWT_STEP_UP"])
    .default("JWT_STANDARD"),
  rowVersion: z.number().int().positive("rowVersion diperlukan"),
  attestChecked: z.literal(true, {
    error: () => ({ message: "Anda harus mencentang pernyataan ini sebelum melanjutkan" }),
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
    error: () => ({ message: "Anda harus mencentang pernyataan ini sebelum melanjutkan" }),
  }),
});

export type WorkflowRejectInput = z.infer<typeof workflowRejectSchema>;

// ---------------------------------------------------------------------------
// API response types
// ---------------------------------------------------------------------------

export type PDWorkflowStatus =
  | "DRAFT"
  | "PENDING_REVIEW"
  | "PENDING_APPROVAL"
  | "PENDING_APPROVAL_2"
  | "APPROVED"
  | "RETURNED";

export interface WorkflowHistoryEntry {
  action: "SUBMIT" | "REVIEW" | "APPROVE" | "APPROVE2" | "REJECT";
  userId: string;
  username: string;
  role: string;
  signedAt: string;
  signatureHash: string;
  comment: string | null;
}

export interface WorkflowStatusData {
  currentState: PDWorkflowStatus;
  workflowEyes: 4 | 6;
  makerId: string | null;
  reviewerId: string | null;
  approverId: string | null;
  approver2Id: string | null;
  history: WorkflowHistoryEntry[];
}

export interface PDPefindoItem {
  id: string;
  rating: string;
  pd12Month: string;
  pdLifetime3Y: string | null;
  pdLifetime5Y: string | null;
  pdLifetime7Y: string | null;
  pdLifetime10Y: string | null;
  sumber: string;
  tanggalPublikasi: string | null;
  periodeBerlakuDari: string;
  periodeBerlakuSampai: string | null;
  dokumenPendukungId: string | null;
  workflowStatus: PDWorkflowStatus;
  workflowInstanceId: string | null;
  rowVersion: number;
  createdAt: string;
  createdBy: string | null;
  updatedAt: string | null;
  updatedBy: string | null;
  deletedAt: string | null;
}

export interface PDPefindoDetail extends PDPefindoItem {
  workflow: WorkflowStatusData | null;
}

export interface UploadJobResponse {
  jobId: string;
  statusUrl: string;
  streamUrl: string;
}

export type JobStatus =
  | "queued"
  | "running"
  | "completed"
  | "failed"
  | "cancelled";

export interface UploadJobResult {
  rowsCreated: number;
  rowsSkipped: number;
  errors: Array<{ row: number; rating: string; message: string }>;
}

export interface JobStatusResponse {
  jobId: string;
  type: string;
  status: JobStatus;
  progress: number;
  currentStep: string | null;
  startedAt: string | null;
  estimatedCompletionAt: string | null;
  result: UploadJobResult | null;
  error: { code: string; message: string } | null;
  canCancel: boolean;
}
