import { z } from "zod";

// ---------------------------------------------------------------------------
// 27 Event Codes (DEC-P5-M1-002)
// ---------------------------------------------------------------------------

export const EVENT_CODE_LIST = [
  "PENEMPATAN",
  "AKRUAL_BUNGA",
  "PEMBAYARAN_BUNGA",
  "PEMBAYARAN_POKOK",
  "PENERIMAAN_DIVIDEN",
  "DISTRIBUSI_REKSADANA",
  "JATUH_TEMPO",
  "RENEWAL_DEPOSITO",
  "FX_REALIZED",
  "AMORTISASI_PREMI_DISKONTO",
  "PENJUALAN_PENCAIRAN",
  "PENGHAPUSAN",
  "ECL_PEMBENTUKAN",
  "ECL_REVERSAL",
  "POCI_DELTA_ECL",
  "MTM_FVTPL",
  "MTM_FVOCI",
  "MTM_FVOCI_ELECTION",
  "REKLAS_OCI_PL",
  "REKLASIFIKASI_AC_FVOCI",
  "REKLASIFIKASI_FVOCI_AC",
  "MODIFIKASI_MATERIAL",
  "EIR_CATCH_UP_ADJUSTMENT",
  "STAGE_MIGRATION",
  "FX_UNREALIZED",
  "PERIODE_ADJUSTMENT",
  "CORRECTION_PERIODE_CLOSED",
] as const;

export const eventCodeEnum = z.enum(EVENT_CODE_LIST);
export type EventCode = z.infer<typeof eventCodeEnum>;

// Regulated codes (6-eyes) per DEC-P5-M1-003
export const REGULATED_EVENT_CODES = new Set<string>([
  "ECL_PEMBENTUKAN",
  "ECL_REVERSAL",
  "EIR_CATCH_UP_ADJUSTMENT",
  "STAGE_MIGRATION",
  "POCI_DELTA_ECL",
  "MTM_FVTPL",
  "MTM_FVOCI",
  "MTM_FVOCI_ELECTION",
  "REKLAS_OCI_PL",
  "MODIFIKASI_MATERIAL",
  "REKLASIFIKASI_AC_FVOCI",
  "REKLASIFIKASI_FVOCI_AC",
  "FX_UNREALIZED",
]);

export function isRegulatedCode(code: string): boolean {
  return REGULATED_EVENT_CODES.has(code);
}

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

export const kategoriEventEnum = z.enum([
  "PENEMPATAN",
  "AKRUAL",
  "ECL",
  "MUTASI_MTM",
  "STAGE_MIGRATION",
  "CLOSURE",
  "REKLASIFIKASI",
  "FX",
  "KOREKSI",
]);
export type KategoriEvent = z.infer<typeof kategoriEventEnum>;

export const triggerSourceEnum = z.enum(["USER_INPUT", "SYSTEM_JOB"]);
export type TriggerSource = z.infer<typeof triggerSourceEnum>;

export const klasifikasiPsak71Enum = z.enum([
  "AC",
  "FVOCI",
  "FVTPL",
  "FVOCI_ELECTION",
  "POCI",
]);
export type KlasifikasiPsak71 = z.infer<typeof klasifikasiPsak71Enum>;

export const dkIndicatorEnum = z.enum(["DEBIT", "KREDIT"]);
export type DkIndicator = z.infer<typeof dkIndicatorEnum>;

export const sumberAmountEnum = z.enum([
  "nominal_idr",
  "ecl_amount",
  "mtm_change",
  "accrued_interest",
  "net_carrying_idr",
  "fx_gain_loss",
  "premium_discount_amortization",
]);
export type SumberAmount = z.infer<typeof sumberAmountEnum>;

// ---------------------------------------------------------------------------
// 6-eyes workflow status (DEC-P5-M1-003)
// ---------------------------------------------------------------------------

export const mappingWorkflowStatusEnum = z.enum([
  "DRAFT",
  "PENDING_REVIEW",
  "PENDING_APPROVAL",
  "PENDING_APPROVAL_2",
  "APPROVED_ACTIVE",
  "REJECTED",
  "RETURNED",
  "WITHDRAWN",
]);
export type MappingWorkflowStatus = z.infer<typeof mappingWorkflowStatusEnum>;

// ---------------------------------------------------------------------------
// 15 Error codes (P5-M2)
// ---------------------------------------------------------------------------

export const jurnalErrorCodeEnum = z.enum([
  "JURNAL_EVENT_NOT_MAPPED",
  "JURNAL_KLASIFIKASI_NOT_ELIGIBLE",
  "JURNAL_BALANCE_INVARIANT",
  "JURNAL_PERIODE_HARD_CLOSED",
  "JURNAL_DUPLICATE_POST",
  "JURNAL_INVALID_TRANSITION",
  "JURNAL_SOD_VIOLATION",
  "JURNAL_STEP_UP_REQUIRED",
  "JURNAL_AMOUNT_INVALID",
  "JURNAL_INSTRUMEN_NOT_FOUND",
  "JURNAL_HEADER_NOT_FOUND",
  "JURNAL_DLQ_NOT_FOUND",
  "JURNAL_DLQ_ALREADY_REPLAYED",
  "JURNAL_DLQ_DISCARD_REASON_TOO_SHORT",
  "JURNAL_DLQ_REPLAY_PERIODE_HARD_CLOSED",
  "JURNAL_MAPPING_WORKFLOW_GATE",
]);
export type JurnalErrorCode = z.infer<typeof jurnalErrorCodeEnum>;

// ---------------------------------------------------------------------------
// DLQ status
// ---------------------------------------------------------------------------

export const dlqStatusEnum = z.enum([
  "FAILED",
  "REPLAYING",
  "REPLAYED_OK",
  "ABANDONED",
]);
export type DlqStatus = z.infer<typeof dlqStatusEnum>;

// ---------------------------------------------------------------------------
// Detail row schema (template rows)
// ---------------------------------------------------------------------------

export const mappingDetailRowSchema = z.object({
  id: z.string().optional(),
  _clientKey: z.string().optional(),
  urutan: z.number().int().min(1),
  dkIndicator: dkIndicatorEnum,
  kodeAkunId: z.string().min(1, "Akun wajib dipilih"),
  kodeAkunDisplay: z.string().optional(),
  namaAkun: z.string().optional(),
  sumberAmount: sumberAmountEnum,
  multiplier: z
    .string()
    .regex(/^\d+(\.\d{1,4})?$/, "Format tidak valid (maks 4 desimal)")
    .default("1.0000"),
  klasifikasiFilter: klasifikasiPsak71Enum.nullable().optional(),
  catatan: z.string().max(500).optional(),
});
export type MappingDetailRow = z.infer<typeof mappingDetailRowSchema>;

// ---------------------------------------------------------------------------
// Mapping Header schemas
// ---------------------------------------------------------------------------

export const mappingHeaderCreateSchema = z
  .object({
    eventCode: z
      .string()
      .min(1, "Kode event wajib diisi")
      .max(40, "Maks 40 karakter")
      .regex(/^[A-Z_]+$/, "Hanya huruf kapital dan underscore"),
    namaEvent: z
      .string()
      .min(1, "Nama event wajib diisi")
      .max(120, "Maks 120 karakter"),
    kategoriEvent: kategoriEventEnum,
    triggerSource: triggerSourceEnum,
    klasifikasiBerlaku: z.array(klasifikasiPsak71Enum).nullable().optional(),
    deskripsi: z.string().max(500).optional(),
    detailRows: z
      .array(mappingDetailRowSchema)
      .min(2, "Minimal 2 baris detail"),
  })
  .superRefine((val, ctx) => {
    const hasDebit = val.detailRows.some((r) => r.dkIndicator === "DEBIT");
    const hasKredit = val.detailRows.some((r) => r.dkIndicator === "KREDIT");
    if (!hasDebit || !hasKredit) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "Template harus memiliki minimal 1 baris DEBIT dan 1 baris KREDIT",
        path: ["detailRows"],
      });
    }
  });

export type MappingHeaderCreateInput = z.infer<typeof mappingHeaderCreateSchema>;

// For edits: same fields but all optional (cannot call .partial() on superRefine schema)
export const mappingHeaderEditSchema = z.object({
  eventCode: z
    .string()
    .min(1, "Kode event wajib diisi")
    .max(40, "Maks 40 karakter")
    .regex(/^[A-Z_]+$/, "Hanya huruf kapital dan underscore")
    .optional(),
  namaEvent: z.string().min(1).max(120).optional(),
  kategoriEvent: kategoriEventEnum.optional(),
  triggerSource: triggerSourceEnum.optional(),
  klasifikasiBerlaku: z.array(klasifikasiPsak71Enum).nullable().optional(),
  deskripsi: z.string().max(500).optional(),
  detailRows: z.array(mappingDetailRowSchema).min(2).optional(),
});
export type MappingHeaderEditInput = z.infer<typeof mappingHeaderEditSchema>;

// ---------------------------------------------------------------------------
// Workflow transition request
// ---------------------------------------------------------------------------

export const workflowTransitionSchema = z.object({
  comment: z.string().optional(),
  signatureMethod: z.enum(["JWT_STEP_UP", "JWT_STANDARD"]).default("JWT_STANDARD"),
});
export type WorkflowTransitionInput = z.infer<typeof workflowTransitionSchema>;

export const rejectMappingSchema = z.object({
  rejectReason: z
    .string()
    .min(30, "Alasan penolakan minimal 30 karakter")
    .max(1000),
  signatureMethod: z.enum(["JWT_STEP_UP", "JWT_STANDARD"]).default("JWT_STANDARD"),
});
export type RejectMappingInput = z.infer<typeof rejectMappingSchema>;

// ---------------------------------------------------------------------------
// Resolver request / response
// ---------------------------------------------------------------------------

export const resolverRequestSchema = z.object({
  eventCode: z.string().min(1, "Kode event wajib diisi"),
  klasifikasiPsak71: klasifikasiPsak71Enum,
  instrumenId: z.string().uuid().nullable().optional(),
  periodeId: z.string().min(1, "Periode wajib diisi"),
  amountIdr: z
    .string()
    .regex(/^\d+(\.\d{1,4})?$/, "Format nominal tidak valid")
    .refine((v) => parseFloat(v) > 0, "Nominal harus lebih dari 0"),
  currency: z.string().default("IDR"),
  fxRate: z.string().default("1.00000000"),
  sourceEventId: z.string().uuid().optional(),
  sourceEventType: z.string().optional(),
  metadataJson: z.record(z.string(), z.unknown()).optional(),
});
export type ResolverRequest = z.infer<typeof resolverRequestSchema>;

export interface JurnalLine {
  urutan: number;
  posisi: "DEBIT" | "KREDIT";
  akunId: string;
  akunKode: string;
  akunNama: string;
  amountIdr: string;
  narasi?: string;
  klasifikasiEligible?: string;
}

export interface ResolverResponse {
  lines: JurnalLine[];
  totalDebitIdr: string;
  totalKreditIdr: string;
  isBalanced: boolean;
  headerUsed: {
    id: string;
    eventCode: string;
    kategoriEvent: string;
    namaEvent?: string;
  };
}

// ---------------------------------------------------------------------------
// Manual posting
// ---------------------------------------------------------------------------

export const manualPostSchema = z.object({
  eventCode: z.enum(["PERIODE_ADJUSTMENT", "CORRECTION_PERIODE_CLOSED"]),
  periodeId: z.string().min(1, "Periode wajib dipilih"),
  instrumenId: z.string().uuid().nullable().optional(),
  amountIdr: z
    .string()
    .regex(/^\d+(\.\d{1,4})?$/, "Format nominal tidak valid")
    .refine((v) => parseFloat(v) > 0, "Nominal harus lebih dari 0"),
  narasi: z.string().min(1, "Narasi wajib diisi").max(500),
  dokumenDocId: z.string().uuid().nullable().optional(),
});
export type ManualPostInput = z.infer<typeof manualPostSchema>;

// ---------------------------------------------------------------------------
// Journal status
// ---------------------------------------------------------------------------

export const jurnalStatusEnum = z.enum([
  "POSTED",
  "REVERSED",
  "PENDING_APPROVAL",
]);
export type JurnalStatus = z.infer<typeof jurnalStatusEnum>;

// ---------------------------------------------------------------------------
// API Response types (matching OpenAPI schemas)
// ---------------------------------------------------------------------------

export interface WorkflowActor {
  id: string;
  nama: string;
  role: string;
  signedAt?: string | null;
  comment?: string | null;
  signatureHash?: string | null;
}

export interface MappingHeaderSummary {
  id: string;
  eventCode: string;
  eventIdKode: string;
  namaEvent: string;
  kategoriEvent: KategoriEvent;
  triggerSource: TriggerSource;
  klasifikasiBerlaku: KlasifikasiPsak71[] | null;
  aktifFlag: boolean;
  workflowStatus: MappingWorkflowStatus;
  workflowPath: "4-eyes" | "6-eyes";
  detailCount: number;
  createdAt: string;
  updatedAt: string;
  makerId?: string;
  makerNama?: string;
  reviewerId?: string;
  reviewerNama?: string;
  approverId?: string;
  approverNama?: string;
  approver2Id?: string;
  approver2Nama?: string;
  rowVersion: number;
}

export interface MappingDetailRowItem {
  id: string;
  urutan: number;
  dkIndicator: DkIndicator;
  kodeAkunId: string;
  kodeAkunDisplay: string;
  namaAkun: string;
  sumberAmount: SumberAmount;
  multiplier: string;
  klasifikasiFilter: KlasifikasiPsak71 | null;
  catatan?: string | null;
}

export interface MappingHeaderDetail extends MappingHeaderSummary {
  deskripsi?: string | null;
  detailRows: MappingDetailRowItem[];
  reviewerSignedAt?: string | null;
  reviewerSignatureHash?: string | null;
  approverSignedAt?: string | null;
  approverSignatureHash?: string | null;
  approver2SignedAt?: string | null;
  approver2SignatureHash?: string | null;
  rejectReason?: string | null;
}

export interface WorkflowTransitionResponse {
  data: {
    id: string;
    workflowStatus: MappingWorkflowStatus;
    updatedAt: string;
  };
  meta: { traceId: string };
}

export interface JurnalHeaderSummary {
  id: string;
  noJurnal: string;
  tanggalPosting: string;
  periodeId: string;
  periodeLabel: string;
  eventCode: string;
  instrumenId?: string | null;
  instrumenNama?: string | null;
  totalDebit: string;
  totalKredit: string;
  statusInternal: JurnalStatus;
  createdAt: string;
  createdBy: string;
}

export interface JurnalDetailLine {
  id: string;
  urutan: number;
  kodeAkunId: string;
  kodeAkun: string;
  namaAkun: string;
  debitAmount: string;
  kreditAmount: string;
  narrativeLine?: string;
}

export interface JurnalHeaderDetail extends JurnalHeaderSummary {
  mappingHeaderId: string;
  mappingEventCode?: string;
  currency: string;
  narrative: string;
  referenceEventType?: string;
  referenceEventId?: string;
  idempotencyKey?: string;
  detailLines: JurnalDetailLine[];
}

export interface DlqEntrySummary {
  id: string;
  sourceEventId: string;
  sourceEventType: string;
  eventCode: string;
  instrumenId?: string | null;
  periodeId?: string | null;
  errorCode: string;
  errorMessage: string;
  attemptCount: number;
  lastAttemptAt: string;
  status: DlqStatus;
  replayedJurnalId?: string | null;
  createdAt: string;
}

export interface DlqEntryDetail extends DlqEntrySummary {
  payloadJsonb: Record<string, unknown>;
  replayedBy?: string | null;
  replayedAt?: string | null;
  abandonedReason?: string | null;
}

// ---------------------------------------------------------------------------
// Klasifikasi compatibility matrix (§5 state machine doc)
// ---------------------------------------------------------------------------

export const KLASIFIKASI_COMPATIBILITY: Record<string, KlasifikasiPsak71[]> = {
  PENEMPATAN: ["AC", "FVOCI", "FVTPL", "FVOCI_ELECTION", "POCI"],
  AKRUAL_BUNGA: ["AC", "FVOCI", "POCI"],
  PEMBAYARAN_BUNGA: ["AC", "FVOCI", "POCI"],
  PEMBAYARAN_POKOK: ["AC", "FVOCI", "POCI"],
  ECL_PEMBENTUKAN: ["AC", "FVOCI", "POCI"],
  ECL_REVERSAL: ["AC", "FVOCI", "POCI"],
  POCI_DELTA_ECL: ["POCI"],
  MTM_FVTPL: ["FVTPL"],
  MTM_FVOCI: ["FVOCI"],
  MTM_FVOCI_ELECTION: ["FVOCI_ELECTION"],
  REKLAS_OCI_PL: ["FVOCI"],
  REKLASIFIKASI_AC_FVOCI: ["AC", "FVOCI"],
  REKLASIFIKASI_FVOCI_AC: ["FVOCI", "AC"],
  MODIFIKASI_MATERIAL: ["AC", "FVOCI", "POCI"],
  EIR_CATCH_UP_ADJUSTMENT: ["AC", "FVOCI", "POCI"],
  STAGE_MIGRATION: ["AC", "FVOCI", "POCI"],
  JATUH_TEMPO: ["AC", "FVOCI", "FVTPL", "FVOCI_ELECTION", "POCI"],
  PENJUALAN_PENCAIRAN: ["AC", "FVOCI", "FVTPL", "FVOCI_ELECTION", "POCI"],
  PENERIMAAN_DIVIDEN: ["FVTPL", "FVOCI_ELECTION"],
  DISTRIBUSI_REKSADANA: ["FVTPL"],
  RENEWAL_DEPOSITO: ["AC", "FVOCI"],
  FX_REALIZED: ["AC", "FVOCI", "FVTPL", "FVOCI_ELECTION", "POCI"],
  FX_UNREALIZED: ["AC", "FVOCI", "FVTPL", "FVOCI_ELECTION", "POCI"],
  AMORTISASI_PREMI_DISKONTO: ["AC", "FVOCI", "POCI"],
  PENGHAPUSAN: ["AC", "FVOCI", "POCI"],
  PERIODE_ADJUSTMENT: ["AC", "FVOCI", "FVTPL", "FVOCI_ELECTION", "POCI"],
  CORRECTION_PERIODE_CLOSED: ["AC", "FVOCI", "FVTPL", "FVOCI_ELECTION", "POCI"],
};

// ---------------------------------------------------------------------------
// Event code metadata for picker
// ---------------------------------------------------------------------------

export interface EventCodeMeta {
  eventCode: string;
  namaEvent: string;
  kategoriEvent: KategoriEvent;
  triggerSource: TriggerSource;
  workflowPath: "4-eyes" | "6-eyes";
  klasifikasiAllowed: KlasifikasiPsak71[];
  isRegulated: boolean;
}

export const EVENT_CODE_METADATA: EventCodeMeta[] = [
  { eventCode: "PENEMPATAN", namaEvent: "Penempatan Instrumen Keuangan", kategoriEvent: "PENEMPATAN", triggerSource: "SYSTEM_JOB", workflowPath: "4-eyes", klasifikasiAllowed: ["AC","FVOCI","FVTPL","FVOCI_ELECTION","POCI"], isRegulated: false },
  { eventCode: "JATUH_TEMPO", namaEvent: "Pelunasan / Jatuh Tempo", kategoriEvent: "CLOSURE", triggerSource: "SYSTEM_JOB", workflowPath: "4-eyes", klasifikasiAllowed: ["AC","FVOCI","FVTPL","FVOCI_ELECTION","POCI"], isRegulated: false },
  { eventCode: "PENJUALAN_PENCAIRAN", namaEvent: "Penjualan / Pencairan", kategoriEvent: "CLOSURE", triggerSource: "SYSTEM_JOB", workflowPath: "4-eyes", klasifikasiAllowed: ["AC","FVOCI","FVTPL","FVOCI_ELECTION","POCI"], isRegulated: false },
  { eventCode: "RENEWAL_DEPOSITO", namaEvent: "Renewal Deposito", kategoriEvent: "PENEMPATAN", triggerSource: "SYSTEM_JOB", workflowPath: "4-eyes", klasifikasiAllowed: ["AC","FVOCI"], isRegulated: false },
  { eventCode: "AKRUAL_BUNGA", namaEvent: "Akrual Bunga / EIR", kategoriEvent: "AKRUAL", triggerSource: "SYSTEM_JOB", workflowPath: "4-eyes", klasifikasiAllowed: ["AC","FVOCI","POCI"], isRegulated: false },
  { eventCode: "PEMBAYARAN_BUNGA", namaEvent: "Pembayaran Bunga / Kupon", kategoriEvent: "AKRUAL", triggerSource: "SYSTEM_JOB", workflowPath: "4-eyes", klasifikasiAllowed: ["AC","FVOCI","POCI"], isRegulated: false },
  { eventCode: "PEMBAYARAN_POKOK", namaEvent: "Pembayaran Pokok", kategoriEvent: "AKRUAL", triggerSource: "SYSTEM_JOB", workflowPath: "4-eyes", klasifikasiAllowed: ["AC","FVOCI","POCI"], isRegulated: false },
  { eventCode: "PENERIMAAN_DIVIDEN", namaEvent: "Penerimaan Dividen", kategoriEvent: "AKRUAL", triggerSource: "SYSTEM_JOB", workflowPath: "4-eyes", klasifikasiAllowed: ["FVTPL","FVOCI_ELECTION"], isRegulated: false },
  { eventCode: "DISTRIBUSI_REKSADANA", namaEvent: "Distribusi / NAB Reksadana", kategoriEvent: "AKRUAL", triggerSource: "SYSTEM_JOB", workflowPath: "4-eyes", klasifikasiAllowed: ["FVTPL"], isRegulated: false },
  { eventCode: "AMORTISASI_PREMI_DISKONTO", namaEvent: "Amortisasi Premi / Diskonto EIR", kategoriEvent: "AKRUAL", triggerSource: "SYSTEM_JOB", workflowPath: "4-eyes", klasifikasiAllowed: ["AC","FVOCI","POCI"], isRegulated: false },
  { eventCode: "FX_REALIZED", namaEvent: "Keuntungan / Kerugian FX Realized", kategoriEvent: "FX", triggerSource: "SYSTEM_JOB", workflowPath: "4-eyes", klasifikasiAllowed: ["AC","FVOCI","FVTPL","FVOCI_ELECTION","POCI"], isRegulated: false },
  { eventCode: "PENGHAPUSAN", namaEvent: "Penghapusan / Write-off", kategoriEvent: "CLOSURE", triggerSource: "USER_INPUT", workflowPath: "4-eyes", klasifikasiAllowed: ["AC","FVOCI","POCI"], isRegulated: false },
  { eventCode: "PERIODE_ADJUSTMENT", namaEvent: "Penyesuaian Periode", kategoriEvent: "KOREKSI", triggerSource: "USER_INPUT", workflowPath: "4-eyes", klasifikasiAllowed: ["AC","FVOCI","FVTPL","FVOCI_ELECTION","POCI"], isRegulated: false },
  { eventCode: "CORRECTION_PERIODE_CLOSED", namaEvent: "Koreksi Periode Ditutup", kategoriEvent: "KOREKSI", triggerSource: "USER_INPUT", workflowPath: "4-eyes", klasifikasiAllowed: ["AC","FVOCI","FVTPL","FVOCI_ELECTION","POCI"], isRegulated: false },
  { eventCode: "ECL_PEMBENTUKAN", namaEvent: "Pembentukan Cadangan ECL", kategoriEvent: "ECL", triggerSource: "SYSTEM_JOB", workflowPath: "6-eyes", klasifikasiAllowed: ["AC","FVOCI","POCI"], isRegulated: true },
  { eventCode: "ECL_REVERSAL", namaEvent: "Reversal Cadangan ECL", kategoriEvent: "ECL", triggerSource: "SYSTEM_JOB", workflowPath: "6-eyes", klasifikasiAllowed: ["AC","FVOCI","POCI"], isRegulated: true },
  { eventCode: "POCI_DELTA_ECL", namaEvent: "Delta ECL Instrumen POCI", kategoriEvent: "ECL", triggerSource: "SYSTEM_JOB", workflowPath: "6-eyes", klasifikasiAllowed: ["POCI"], isRegulated: true },
  { eventCode: "EIR_CATCH_UP_ADJUSTMENT", namaEvent: "Penyesuaian EIR Amandemen", kategoriEvent: "AKRUAL", triggerSource: "SYSTEM_JOB", workflowPath: "6-eyes", klasifikasiAllowed: ["AC","FVOCI","POCI"], isRegulated: true },
  { eventCode: "STAGE_MIGRATION", namaEvent: "Migrasi Stage ECL", kategoriEvent: "STAGE_MIGRATION", triggerSource: "SYSTEM_JOB", workflowPath: "6-eyes", klasifikasiAllowed: ["AC","FVOCI","POCI"], isRegulated: true },
  { eventCode: "MTM_FVTPL", namaEvent: "Mark-to-Market FVTPL", kategoriEvent: "MUTASI_MTM", triggerSource: "SYSTEM_JOB", workflowPath: "6-eyes", klasifikasiAllowed: ["FVTPL"], isRegulated: true },
  { eventCode: "MTM_FVOCI", namaEvent: "Mark-to-Market FVOCI Debt", kategoriEvent: "MUTASI_MTM", triggerSource: "SYSTEM_JOB", workflowPath: "6-eyes", klasifikasiAllowed: ["FVOCI"], isRegulated: true },
  { eventCode: "MTM_FVOCI_ELECTION", namaEvent: "Mark-to-Market FVOCI Saham", kategoriEvent: "MUTASI_MTM", triggerSource: "SYSTEM_JOB", workflowPath: "6-eyes", klasifikasiAllowed: ["FVOCI_ELECTION"], isRegulated: true },
  { eventCode: "REKLAS_OCI_PL", namaEvent: "Recycling OCI ke P&L", kategoriEvent: "REKLASIFIKASI", triggerSource: "SYSTEM_JOB", workflowPath: "6-eyes", klasifikasiAllowed: ["FVOCI"], isRegulated: true },
  { eventCode: "REKLASIFIKASI_AC_FVOCI", namaEvent: "Reklasifikasi AC ke FVOCI", kategoriEvent: "REKLASIFIKASI", triggerSource: "USER_INPUT", workflowPath: "6-eyes", klasifikasiAllowed: ["AC","FVOCI"], isRegulated: true },
  { eventCode: "REKLASIFIKASI_FVOCI_AC", namaEvent: "Reklasifikasi FVOCI ke AC", kategoriEvent: "REKLASIFIKASI", triggerSource: "USER_INPUT", workflowPath: "6-eyes", klasifikasiAllowed: ["FVOCI","AC"], isRegulated: true },
  { eventCode: "MODIFIKASI_MATERIAL", namaEvent: "Modifikasi Kontrak Material", kategoriEvent: "AKRUAL", triggerSource: "SYSTEM_JOB", workflowPath: "6-eyes", klasifikasiAllowed: ["AC","FVOCI","POCI"], isRegulated: true },
  { eventCode: "FX_UNREALIZED", namaEvent: "Keuntungan / Kerugian FX Unrealized", kategoriEvent: "FX", triggerSource: "SYSTEM_JOB", workflowPath: "6-eyes", klasifikasiAllowed: ["AC","FVOCI","FVTPL","FVOCI_ELECTION","POCI"], isRegulated: true },
];

export const EVENT_CODE_GROUPS: { label: string; codes: EventCodeMeta[] }[] = [
  { label: "Penempatan", codes: EVENT_CODE_METADATA.filter((e) => ["PENEMPATAN","JATUH_TEMPO","PENJUALAN_PENCAIRAN","RENEWAL_DEPOSITO"].includes(e.eventCode)) },
  { label: "Akrual & Bunga", codes: EVENT_CODE_METADATA.filter((e) => ["AKRUAL_BUNGA","PEMBAYARAN_BUNGA","PEMBAYARAN_POKOK","PENERIMAAN_DIVIDEN","DISTRIBUSI_REKSADANA","AMORTISASI_PREMI_DISKONTO","EIR_CATCH_UP_ADJUSTMENT","MODIFIKASI_MATERIAL"].includes(e.eventCode)) },
  { label: "ECL", codes: EVENT_CODE_METADATA.filter((e) => ["ECL_PEMBENTUKAN","ECL_REVERSAL","POCI_DELTA_ECL","STAGE_MIGRATION"].includes(e.eventCode)) },
  { label: "Mutasi MTM", codes: EVENT_CODE_METADATA.filter((e) => ["MTM_FVTPL","MTM_FVOCI","MTM_FVOCI_ELECTION","REKLAS_OCI_PL"].includes(e.eventCode)) },
  { label: "Reklasifikasi", codes: EVENT_CODE_METADATA.filter((e) => ["REKLASIFIKASI_AC_FVOCI","REKLASIFIKASI_FVOCI_AC"].includes(e.eventCode)) },
  { label: "FX", codes: EVENT_CODE_METADATA.filter((e) => ["FX_REALIZED","FX_UNREALIZED"].includes(e.eventCode)) },
  { label: "Penutupan", codes: EVENT_CODE_METADATA.filter((e) => ["PENGHAPUSAN"].includes(e.eventCode)) },
  { label: "Koreksi", codes: EVENT_CODE_METADATA.filter((e) => ["PERIODE_ADJUSTMENT","CORRECTION_PERIODE_CLOSED"].includes(e.eventCode)) },
];
