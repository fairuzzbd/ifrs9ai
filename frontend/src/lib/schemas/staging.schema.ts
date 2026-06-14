/**
 * Zod schemas for APP-C Staging Engine (P4-M9).
 *
 * Mirrors OpenAPI app-c-staging.yaml schemas.
 * Money: string-based (NUMERIC(20,4) via string) per DEC-016 — no parseFloat.
 */

import { z } from "zod";

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

export const stageEnum = z.enum(["STAGE_1", "STAGE_2", "STAGE_3"]);
export type Stage = z.infer<typeof stageEnum>;

export const triggerTypeEnum = z.enum([
  "RATING_DOWNGRADE",
  "IG_TO_NON_IG",
  "RATING_DEFAULT",
  "DPD_GTE_30",
  "DPD_GTE_90",
  "CURE_3_PERIODE_BULANAN",
  "MANUAL_OVERRIDE",
  "OVERRIDE_EXPIRED",
]);
export type TriggerType = z.infer<typeof triggerTypeEnum>;

export const statusApprovalEnum = z.enum(["AUTO", "APPROVED", "OVERRIDE_EXPIRED"]);
export type StatusApproval = z.infer<typeof statusApprovalEnum>;

export const overrideStatusEnum = z.enum([
  "PENDING_REVIEW",
  "PENDING_APPROVAL",
  "APPROVED_ALCO",
  "ACTIVE",
  "EXPIRED",
  "REJECTED",
]);
export type OverrideStatus = z.infer<typeof overrideStatusEnum>;

// ---------------------------------------------------------------------------
// SICR Evidence
// ---------------------------------------------------------------------------

export const sicrEvidenceSchema = z.object({
  triggerType: z
    .enum([
      "RATING_DOWNGRADE_NOTCH",
      "IG_TO_NON_IG",
      "DPD_GTE_30",
      "DPD_GTE_90",
      "MANUAL_OVERRIDE",
      "CURE_ASSESSMENT",
      "INITIAL",
    ])
    .optional(),
  ratingBaseline: z.string().nullable().optional(),
  ratingCurrent: z.string().nullable().optional(),
  notchDelta: z.number().int().nullable().optional(),
  dpdValue: z.number().int().nullable().optional(),
  dpdPeriode: z.string().nullable().optional(),
  cureConsecutivePeriodes: z.number().int().nullable().optional(),
  overrideProposalId: z.string().uuid().nullable().optional(),
});
export type SicrEvidence = z.infer<typeof sicrEvidenceSchema>;

// ---------------------------------------------------------------------------
// Stage History Row
// ---------------------------------------------------------------------------

export const stageHistoryRowSchema = z.object({
  id: z.string().uuid(),
  stageHistoryIdKode: z.string(),
  instrumenId: z.string().uuid(),
  tanggalMigrasi: z.string(),
  stageSebelum: stageEnum.nullable().optional(),
  stageSesudah: stageEnum,
  triggerType: triggerTypeEnum,
  detailTrigger: z.string().nullable().optional(),
  ratingSaatMigrasi: z.string().nullable().optional(),
  dpd: z.number().int().nullable().optional(),
  statusApproval: statusApprovalEnum,
  userApproverId: z.string().uuid().nullable().optional(),
  userApproverNama: z.string().nullable().optional(),
  dokumenPendukungId: z.string().uuid().nullable().optional(),
  dokumenPendukungFilename: z.string().nullable().optional(),
  sicrEvidence: sicrEvidenceSchema.nullable().optional(),
  createdAt: z.string(),
});
export type StageHistoryRow = z.infer<typeof stageHistoryRowSchema>;

// ---------------------------------------------------------------------------
// Staging Current Response
// ---------------------------------------------------------------------------

export const stagingCurrentSchema = z.object({
  instrumenId: z.string().uuid(),
  kodeInstrumen: z.string(),
  namaInstrumen: z.string(),
  klasifikasiPsak71: z.enum(["AC", "FVOCI", "FVTPL", "FVOCI_ELECTION"]),
  currentStage: stageEnum.nullable().optional(),
  lastTransitionDate: z.string().nullable().optional(),
  lastTriggerType: triggerTypeEnum.nullable().optional(),
  lastTriggerDetail: z.string().nullable().optional(),
  lastRatingSaatMigrasi: z.string().nullable().optional(),
  lastDpd: z.number().int().nullable().optional(),
  lastStatusApproval: statusApprovalEnum.nullable().optional(),
  activeOverrideId: z.string().uuid().nullable().optional(),
  activeOverrideExpiresAt: z.string().nullable().optional(),
});
export type StagingCurrent = z.infer<typeof stagingCurrentSchema>;

// ---------------------------------------------------------------------------
// Override Proposal
// ---------------------------------------------------------------------------

export const stagingOverrideProposalSchema = z.object({
  id: z.string().uuid(),
  instrumenId: z.string().uuid(),
  kodeInstrumen: z.string().optional(),
  stageFrom: stageEnum,
  stageTo: stageEnum,
  alasan: z.string(),
  dokumenPendukungId: z.string().uuid().nullable().optional(),
  status: overrideStatusEnum,
  makerId: z.string().uuid(),
  reviewerId: z.string().uuid().nullable().optional(),
  approverId: z.string().uuid().nullable().optional(),
  periodeId: z.string().uuid(),
  periodeAkhir: z.string(),
  createdAt: z.string(),
  updatedAt: z.string().optional(),
});
export type StagingOverrideProposal = z.infer<typeof stagingOverrideProposalSchema>;

// ---------------------------------------------------------------------------
// Override Submit Form
// ---------------------------------------------------------------------------

export const overrideSubmitFormSchema = z.object({
  instrumenId: z.string().uuid({ message: "Pilih instrumen yang valid." }),
  stageTarget: stageEnum,
  alasan: z
    .string()
    .min(20, { message: "Alasan harus minimal 20 karakter." })
    .max(1000, { message: "Alasan maksimal 1000 karakter." }),
  periodeId: z.string().uuid({ message: "Pilih periode buku." }),
  dokumenPendukungId: z.string().uuid().optional(),
});
export type OverrideSubmitForm = z.infer<typeof overrideSubmitFormSchema>;

// ---------------------------------------------------------------------------
// DPD Record
// ---------------------------------------------------------------------------

export const dpdRecordSchema = z.object({
  id: z.string().uuid(),
  instrumenId: z.string().uuid(),
  kodeInstrumen: z.string().optional(),
  namaInstrumen: z.string().optional(),
  periode: z.string(),
  dpdValue: z.number().int().min(0),
  source: z.enum(["MANUAL", "APP_B"]),
  catatan: z.string().nullable().optional(),
  currentStage: stageEnum.nullable().optional(),
  createdBy: z.string().optional(),
  createdAt: z.string().optional(),
  updatedAt: z.string().optional(),
});
export type DpdRecord = z.infer<typeof dpdRecordSchema>;

export const dpdRecordFormSchema = z.object({
  instrumenId: z.string().uuid({ message: "Pilih instrumen yang valid." }),
  periode: z.string().min(1, { message: "Pilih periode." }),
  dpdValue: z
    .number({ message: "Nilai DPD harus berupa angka." })
    .int({ message: "Nilai DPD harus bilangan bulat." })
    .min(0, { message: "Nilai DPD tidak boleh negatif." }),
  catatan: z.string().max(200).optional(),
});
export type DpdRecordForm = z.infer<typeof dpdRecordFormSchema>;

// ---------------------------------------------------------------------------
// Workflow action (approve / review / reject)
// ---------------------------------------------------------------------------

export const workflowRejectFormSchema = z.object({
  comment: z
    .string()
    .min(20, { message: "Alasan penolakan minimal 20 karakter." })
    .max(1000),
  signatureMethod: z.string(),
});
export type WorkflowRejectForm = z.infer<typeof workflowRejectFormSchema>;
