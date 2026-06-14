// Package jurnal implements the Jurnal Event Resolver & Posting Engine (P5-M2).
//
// Responsibilities:
//   - Mapping Jurnal Header CRUD with 4-eyes (operational) / 6-eyes (regulated) workflow.
//   - Jurnal Resolver: event payload → balanced debit/kredit JurnalLine array (no DB writes).
//   - Jurnal Posting: automated (Asynq subscribers) + manual (PERIODE_ADJUSTMENT,
//     CORRECTION_PERIODE_CLOSED) with atomic INSERT into jrnl.header + jrnl.detail.
//   - DLQ (Dead Letter Queue): inspect, replay, and discard failed postings.
//
// Compliance decisions anchoring this package:
//   - DEC-P5-M1-002 — 27 master event codes; seeded in migration 000035.
//   - DEC-P5-M1-003 — 6-eyes regulated vs 4-eyes operational (hardcoded whitelist).
//   - DEC-017 — SoD: maker ≠ reviewer ≠ approver ≠ approver_2.
//   - DEC-018 — Audit trail append-only; JURNAL.POST in same tx as INSERT jrnl.header.
//   - DEC-021 — Idempotency-Key mandatory on all mutating endpoints.
//   - DEC-027 — Step-up MFA on approve (regulated) and approve-2.
//
// Invariants:
//   - jrnl.header + jrnl.detail are APPEND-ONLY (DB triggers from migration 000035).
//   - CHECK CONSTRAINT: jrnl.header.total_debit = jrnl.header.total_kredit.
//   - JURNAL.POST audit written in same DB transaction as INSERT jrnl.header.
//   - Constructor panics on nil auditWriter (DEC-018).
//
// No float64 for money/rates — shopspring/decimal throughout (DEC-016).
package jurnal

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── Status enums ─────────────────────────────────────────────────────────────

// MappingHeaderStatus is the workflow status of a mst.mapping_jurnal_header row.
type MappingHeaderStatus string

const (
	MappingStatusDraft            MappingHeaderStatus = "DRAFT"
	MappingStatusPendingReview    MappingHeaderStatus = "PENDING_REVIEW"
	MappingStatusPendingApproval  MappingHeaderStatus = "PENDING_APPROVAL"
	MappingStatusPendingApproval2 MappingHeaderStatus = "PENDING_APPROVAL_2"
	MappingStatusApprovedActive   MappingHeaderStatus = "APPROVED_ACTIVE"
	MappingStatusWithdrawn        MappingHeaderStatus = "WITHDRAWN"
	// Legacy compat (pre-P5-M2 rows treated as APPROVED_ACTIVE by resolver).
	MappingStatusApproved MappingHeaderStatus = "APPROVED"
	MappingStatusRejected MappingHeaderStatus = "REJECTED"
	MappingStatusReturned MappingHeaderStatus = "RETURNED"
)

// IsActiveForResolver returns true if this status allows the mapping to be used by the resolver.
func (s MappingHeaderStatus) IsActiveForResolver() bool {
	return s == MappingStatusApprovedActive || s == MappingStatusApproved
}

// CanSubmit returns true if the status allows /submit transition.
func (s MappingHeaderStatus) CanSubmit() bool { return s == MappingStatusDraft }

// CanReview returns true if the status allows /review transition.
func (s MappingHeaderStatus) CanReview() bool { return s == MappingStatusPendingReview }

// CanApprove returns true if the status allows /approve transition.
func (s MappingHeaderStatus) CanApprove() bool { return s == MappingStatusPendingApproval }

// CanApprove2 returns true if the status allows /approve-2 transition.
func (s MappingHeaderStatus) CanApprove2() bool { return s == MappingStatusPendingApproval2 }

// CanReject returns true if the status allows /reject transition.
func (s MappingHeaderStatus) CanReject() bool {
	return s == MappingStatusPendingReview ||
		s == MappingStatusPendingApproval ||
		s == MappingStatusPendingApproval2
}

// CanWithdraw returns true if the status allows /withdraw transition.
func (s MappingHeaderStatus) CanWithdraw() bool { return s == MappingStatusDraft }

// CanDeactivate returns true if the status allows /deactivate.
func (s MappingHeaderStatus) CanDeactivate() bool {
	return s == MappingStatusApprovedActive || s == MappingStatusApproved
}

// JurnalHeaderStatus is the status_internal of a jrnl.header row.
type JurnalHeaderStatus string //nolint:revive // Jurnal prefix required for cross-package clarity

const (
	JurnalStatusDraftManual    JurnalHeaderStatus = "PENDING_APPROVAL" // pre-submit + post-submit draft state
	JurnalStatusPosted         JurnalHeaderStatus = "POSTED"
	JurnalStatusReversed       JurnalHeaderStatus = "REVERSED"
	JurnalStatusPendingApprove JurnalHeaderStatus = "PENDING_APPROVAL"
)

// DLQStatus is the lifecycle status of a sys.dlq_jurnal_post row.
type DLQStatus string

const (
	DLQStatusFailed     DLQStatus = "FAILED"
	DLQStatusReplaying  DLQStatus = "REPLAYING"
	DLQStatusReplayedOK DLQStatus = "REPLAYED_OK"
	DLQStatusAbandoned  DLQStatus = "ABANDONED"
)

// WorkflowPath discriminates 4-eyes vs 6-eyes mapping workflow.
type WorkflowPath string

const (
	WorkflowPath4Eyes WorkflowPath = "4-eyes"
	WorkflowPath6Eyes WorkflowPath = "6-eyes"
)

// ─── EventCode constants (27 codes per DEC-P5-M1-002) ────────────────────────

const (
	EventCodePenempatan              = "PENEMPATAN"
	EventCodeAkrualBunga             = "AKRUAL_BUNGA"
	EventCodeECLPembentukan          = "ECL_PEMBENTUKAN"
	EventCodeECLReversal             = "ECL_REVERSAL"
	EventCodePOCIDeltaECL            = "POCI_DELTA_ECL"
	EventCodeMTMFVTPL                = "MTM_FVTPL"
	EventCodeMTMFVOCI                = "MTM_FVOCI"
	EventCodeMTMFVOCIElection        = "MTM_FVOCI_ELECTION"
	EventCodeReklasOCIPL             = "REKLAS_OCI_PL"
	EventCodeReklasAcFVOCI           = "REKLASIFIKASI_AC_FVOCI"
	EventCodeReklasFVOCIAc           = "REKLASIFIKASI_FVOCI_AC"
	EventCodeModifikasiMaterial      = "MODIFIKASI_MATERIAL"
	EventCodeEIRCatchup              = "EIR_CATCH_UP_ADJUSTMENT"
	EventCodeStageMigration          = "STAGE_MIGRATION"
	EventCodeJatuhTempo              = "JATUH_TEMPO"
	EventCodePenjualanPencairan      = "PENJUALAN_PENCAIRAN"
	EventCodePembayaranBunga         = "PEMBAYARAN_BUNGA"
	EventCodePembayaranPokok         = "PEMBAYARAN_POKOK"
	EventCodeRenewalDeposito         = "RENEWAL_DEPOSITO"
	EventCodePenerimaanDividen       = "PENERIMAAN_DIVIDEN"
	EventCodeDistribusiReksadana     = "DISTRIBUSI_REKSADANA"
	EventCodeFXRealized              = "FX_REALIZED"
	EventCodeFXUnrealized            = "FX_UNREALIZED"
	EventCodeAmortisasiPremiDiskonto = "AMORTISASI_PREMI_DISKONTO"
	EventCodePenghapusan             = "PENGHAPUSAN"
	EventCodePeriodeAdjustment       = "PERIODE_ADJUSTMENT"
	EventCodeCorrectionPeriodeClosed = "CORRECTION_PERIODE_CLOSED"
)

// regulatedEventCodes is the server-side hardcoded whitelist per DEC-P5-M1-003.
// These codes require 6-eyes workflow (ROLE-RISK second approver) and
// step-up MFA on approve + approve-2 (DEC-027).
// SECURITY: This must be a server-side constant — not client-configurable.
var regulatedEventCodes = map[string]bool{
	EventCodeECLPembentukan:     true,
	EventCodeECLReversal:        true,
	EventCodeEIRCatchup:         true,
	EventCodeStageMigration:     true,
	EventCodePOCIDeltaECL:       true,
	EventCodeMTMFVTPL:           true,
	EventCodeMTMFVOCI:           true,
	EventCodeMTMFVOCIElection:   true,
	EventCodeReklasOCIPL:        true,
	EventCodeModifikasiMaterial: true,
	EventCodeReklasAcFVOCI:      true,
	EventCodeReklasFVOCIAc:      true,
	EventCodeFXUnrealized:       true,
}

// IsRegulated returns true if the event_code requires 6-eyes regulated workflow.
func IsRegulated(eventCode string) bool { return regulatedEventCodes[eventCode] }

// WorkflowPathFor returns the WorkflowPath for a given event code.
func WorkflowPathFor(eventCode string) WorkflowPath {
	if IsRegulated(eventCode) {
		return WorkflowPath6Eyes
	}
	return WorkflowPath4Eyes
}

// manualAllowedEventCodes restricts manual posting to operational adjustment codes only.
var manualAllowedEventCodes = map[string]bool{
	EventCodePeriodeAdjustment:       true,
	EventCodeCorrectionPeriodeClosed: true,
}

// IsManualAllowed returns true if the event_code is allowed for manual posting.
func IsManualAllowed(eventCode string) bool { return manualAllowedEventCodes[eventCode] }

// ─── Permission constants ──────────────────────────────────────────────────────

const (
	PermMappingCreate   = "jurnal_mapping.create"
	PermMappingRead     = "jurnal_mapping.read"
	PermMappingReview   = "jurnal_mapping.review"
	PermMappingApprove  = "jurnal_mapping.approve"
	PermMappingApprove2 = "jurnal_mapping.approve_2"
	PermMappingExport   = "jurnal_mapping.export"

	PermJurnalPost    = "jurnal.post"
	PermJurnalRead    = "jurnal.read"
	PermJurnalApprove = "jurnal.approve"
	PermJurnalExport  = "jurnal.export"

	PermDLQRead    = "jurnal.dlq.read"
	PermDLQReplay  = "jurnal.dlq.replay"
	PermDLQDiscard = "jurnal.dlq.discard"
)

// ─── Domain types — Mapping Header ────────────────────────────────────────────

// MappingDetailRow is one debit/kredit template row in mst.mapping_jurnal_detail.
type MappingDetailRow struct {
	ID                uuid.UUID `json:"id"`
	EventHeaderID     uuid.UUID `json:"eventHeaderId"`
	Urutan            int       `json:"urutan"`
	KodeAkunID        uuid.UUID `json:"kodeAkunId"`
	KodeAkunKode      string    `json:"kodeAkunKode"`
	KodeAkunNama      string    `json:"kodeAkunNama"`
	DKIndicator       string    `json:"dkIndicator"` // "DEBIT" | "KREDIT"
	SumberAmount      string    `json:"sumberAmount"`
	KlasifikasiFilter *string   `json:"klasifikasiFilter,omitempty"`
	// Multiplier: amount = amountIdr * multiplier; default 1.0 (DEC-016: decimal).
	Multiplier decimal.Decimal `json:"multiplier"`
	Catatan    *string         `json:"catatan,omitempty"`
	AktifFlag  bool            `json:"aktifFlag"`
}

// MappingHeader is the full header row of mst.mapping_jurnal_header.
type MappingHeader struct {
	ID                 uuid.UUID           `json:"id"`
	EventIDKode        string              `json:"eventIdKode"`
	EventCode          string              `json:"eventCode"`
	NamaEvent          string              `json:"namaEvent"`
	KategoriEvent      string              `json:"kategoriEvent"`
	TriggerSource      string              `json:"triggerSource"`
	KlasifikasiBerlaku []string            `json:"klasifikasiBerlaku"` // nil = ALL
	AktifFlag          bool                `json:"aktifFlag"`
	WorkflowStatus     MappingHeaderStatus `json:"workflowStatus"`
	WorkflowPath       WorkflowPath        `json:"workflowPath"`
	Deskripsi          *string             `json:"deskripsi,omitempty"`

	// 4-eyes workflow participants
	MakerID    *uuid.UUID `json:"makerId,omitempty"`
	ReviewerID *uuid.UUID `json:"reviewerId,omitempty"`
	ApproverID *uuid.UUID `json:"approverId,omitempty"`

	// Reviewer signature
	ReviewerSignedAt      *time.Time `json:"reviewerSignedAt,omitempty"`
	ReviewerSignatureHash []byte     `json:"-"`
	CommentReview         *string    `json:"commentReview,omitempty"`

	// Approver-1 signature
	ApproverSignedAt      *time.Time `json:"approverSignedAt,omitempty"`
	ApproverSignatureHash []byte     `json:"-"`
	CommentApprove        *string    `json:"commentApprove,omitempty"`

	// 6-eyes: approver-2 (ROLE-RISK, regulated only)
	Approver2ID            *uuid.UUID `json:"approver2Id,omitempty"`
	Approver2SignedAt      *time.Time `json:"approver2SignedAt,omitempty"`
	Approver2SignatureHash []byte     `json:"-"`
	CommentApprove2        *string    `json:"commentApprove2,omitempty"`

	// Workflow timestamps
	SubmitAt     *time.Time `json:"submitAt,omitempty"`
	RejectReason *string    `json:"rejectReason,omitempty"`

	// Detail rows (populated on GET detail)
	DetailRows []MappingDetailRow `json:"detailRows,omitempty"`

	// Audit columns
	CreatedAt  time.Time  `json:"createdAt"`
	CreatedBy  uuid.UUID  `json:"createdBy"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	UpdatedBy  uuid.UUID  `json:"updatedBy"`
	DeletedAt  *time.Time `json:"deletedAt,omitempty"`
	RowVersion int64      `json:"rowVersion"`
	TenantID   string     `json:"tenantId"`
}

// MappingHeaderSummary is the lightweight list view.
type MappingHeaderSummary struct {
	ID             uuid.UUID           `json:"id"`
	EventCode      string              `json:"eventCode"`
	NamaEvent      string              `json:"namaEvent"`
	KategoriEvent  string              `json:"kategoriEvent"`
	TriggerSource  string              `json:"triggerSource"`
	WorkflowStatus MappingHeaderStatus `json:"workflowStatus"`
	WorkflowPath   WorkflowPath        `json:"workflowPath"`
	AktifFlag      bool                `json:"aktifFlag"`
	DetailCount    int                 `json:"detailCount"`
	CreatedAt      time.Time           `json:"createdAt"`
	UpdatedAt      time.Time           `json:"updatedAt"`
}

// ─── Request types — Mapping Header ───────────────────────────────────────────

// MappingDetailRowInput is one row in create/edit request.
type MappingDetailRowInput struct {
	Urutan            int              `json:"urutan" binding:"required,min=1"`
	KodeAkunID        uuid.UUID        `json:"kodeAkunId" binding:"required"`
	DKIndicator       string           `json:"dkIndicator" binding:"required,oneof=DEBIT KREDIT"`
	SumberAmount      string           `json:"sumberAmount" binding:"required"`
	KlasifikasiFilter *string          `json:"klasifikasiFilter,omitempty"`
	Multiplier        *decimal.Decimal `json:"multiplier"`
	Catatan           *string          `json:"catatan,omitempty"`
}

// MappingHeaderCreateRequest is the body for POST /jurnal/mapping-headers.
type MappingHeaderCreateRequest struct {
	EventCode          string                  `json:"eventCode" binding:"required,max=40"`
	NamaEvent          string                  `json:"namaEvent" binding:"required,max=120"`
	KategoriEvent      string                  `json:"kategoriEvent" binding:"required"`
	TriggerSource      string                  `json:"triggerSource" binding:"required,oneof=USER_INPUT SYSTEM_JOB"`
	KlasifikasiBerlaku []string                `json:"klasifikasiBerlaku"`
	Deskripsi          *string                 `json:"deskripsi,omitempty"`
	DetailRows         []MappingDetailRowInput `json:"detailRows" binding:"required,min=2"`
}

// MappingHeaderEditRequest is the body for PATCH /jurnal/mapping-headers/{id}.
type MappingHeaderEditRequest struct {
	NamaEvent          *string                 `json:"namaEvent,omitempty"`
	KategoriEvent      *string                 `json:"kategoriEvent,omitempty"`
	KlasifikasiBerlaku []string                `json:"klasifikasiBerlaku"`
	Deskripsi          *string                 `json:"deskripsi,omitempty"`
	DetailRows         []MappingDetailRowInput `json:"detailRows,omitempty"`
	RowVersion         int64                   `json:"rowVersion" binding:"required"`
}

// WorkflowTransitionRequest is the body for submit/withdraw (no comment required).
type WorkflowTransitionRequest struct {
	Comment         string `json:"comment"`
	SignatureMethod string `json:"signatureMethod"`
}

// WorkflowSigningRequest is the body for review/approve/approve-2/reject.
type WorkflowSigningRequest struct {
	Comment         string `json:"comment" binding:"required,min=1"`
	SignatureMethod string `json:"signatureMethod" binding:"required"`
}

// WorkflowRejectRequest requires minimum 30-char reason.
type WorkflowRejectRequest struct {
	RejectReason    string `json:"rejectReason" binding:"required,min=30"`
	SignatureMethod string `json:"signatureMethod" binding:"required"`
}

// WorkflowTransitionResponse wraps the updated header.
type WorkflowTransitionResponse struct {
	ID             uuid.UUID           `json:"id"`
	WorkflowStatus MappingHeaderStatus `json:"workflowStatus"`
	AktifFlag      bool                `json:"aktifFlag"`
}

// ─── Domain types — Resolver ──────────────────────────────────────────────────

// ResolverRequest is the input to the resolver (event payload).
// AmountIDR must be > 0 (DEC-016: no float64).
type ResolverRequest struct {
	EventCode         string          `json:"eventCode" binding:"required"`
	KlasifikasiPSAK71 string          `json:"klasifikasiPsak71" binding:"required"`
	InstrumenID       *uuid.UUID      `json:"instrumenId,omitempty"`
	PeriodeID         uuid.UUID       `json:"periodeId" binding:"required"`
	AmountIDR         decimal.Decimal `json:"amountIdr"`
	Currency          string          `json:"currency"`
	FxRate            decimal.Decimal `json:"fxRate"`
	SourceEventID     uuid.UUID       `json:"sourceEventId" binding:"required"`
	SourceEventType   string          `json:"sourceEventType" binding:"required"`
	MetadataJSON      json.RawMessage `json:"metadataJson,omitempty"`
	Narasi            string          `json:"narasi,omitempty"`
}

// JurnalLine is one resolved debit or kredit line.
// AmountIDR uses decimal.Decimal (DEC-016: no float64).
type JurnalLine struct { //nolint:revive // Jurnal prefix required for cross-package clarity
	Urutan              int             `json:"urutan"`
	Posisi              string          `json:"posisi"` // "DEBIT" | "KREDIT"
	AkunID              uuid.UUID       `json:"akunId"`
	AkunKode            string          `json:"akunKode"`
	AkunNama            string          `json:"akunNama"`
	AmountIDR           decimal.Decimal `json:"amountIdr"` // NUMERIC(20,4)
	Narasi              string          `json:"narasi"`
	KlasifikasiEligible string          `json:"klasifikasiEligible"`
}

// ResolverResponse is returned by POST /jurnal/resolve.
type ResolverResponse struct {
	Lines          []JurnalLine    `json:"lines"`
	TotalDebitIDR  decimal.Decimal `json:"totalDebitIdr"`
	TotalKreditIDR decimal.Decimal `json:"totalKreditIdr"`
	IsBalanced     bool            `json:"isBalanced"`
	HeaderUsed     *HeaderUsedRef  `json:"headerUsed,omitempty"`
}

// HeaderUsedRef is a lightweight reference to the mapping header used in resolution.
type HeaderUsedRef struct {
	ID            uuid.UUID `json:"id"`
	EventCode     string    `json:"eventCode"`
	KategoriEvent string    `json:"kategoriEvent"`
}

// ─── Domain types — Jurnal Header + Detail ────────────────────────────────────

// JurnalHeader maps to jrnl.header (append-only).
type JurnalHeader struct { //nolint:revive // Jurnal prefix required for cross-package clarity
	ID                 uuid.UUID          `json:"id"`
	NoJurnal           string             `json:"noJurnal"`
	TanggalPosting     time.Time          `json:"tanggalPosting"`
	PeriodeID          uuid.UUID          `json:"periodeId"`
	EventCode          string             `json:"eventCode"`
	MappingHeaderID    *uuid.UUID         `json:"mappingHeaderId,omitempty"`
	InstrumenID        *uuid.UUID         `json:"instrumenId,omitempty"`
	ReferenceEventType string             `json:"referenceEventType"`
	ReferenceEventID   *uuid.UUID         `json:"referenceEventId,omitempty"`
	Currency           string             `json:"currency"`
	TotalDebit         decimal.Decimal    `json:"totalDebit"`
	TotalKredit        decimal.Decimal    `json:"totalKredit"`
	Narrative          string             `json:"narrative"`
	StatusInternal     JurnalHeaderStatus `json:"statusInternal"`
	IdempotencyKey     string             `json:"idempotencyKey"`

	// Manual posting extras
	DokumenDocID *uuid.UUID `json:"dokumenDocId,omitempty"`
	CreatedBy    uuid.UUID  `json:"createdBy"`
	CreatedAt    time.Time  `json:"createdAt"`

	// Detail rows (populated on GET detail)
	DetailRows []JurnalDetailRow `json:"detailRows,omitempty"`
}

// JurnalDetailRow maps to jrnl.detail (append-only).
type JurnalDetailRow struct { //nolint:revive // Jurnal prefix required for cross-package clarity
	ID            uuid.UUID       `json:"id"`
	HeaderID      uuid.UUID       `json:"headerId"`
	Urutan        int             `json:"urutan"`
	KodeAkunID    uuid.UUID       `json:"kodeAkunId"`
	KodeAkunKode  string          `json:"kodeAkunKode,omitempty"`
	KodeAkunNama  string          `json:"kodeAkunNama,omitempty"`
	DebitAmount   decimal.Decimal `json:"debitAmount"`  // 0 if KREDIT
	KreditAmount  decimal.Decimal `json:"kreditAmount"` // 0 if DEBIT
	MataUang      string          `json:"mataUang"`
	NarrativeLine string          `json:"narrativeLine"`
	CreatedAt     time.Time       `json:"createdAt"`
}

// JurnalHeaderSummary is the lightweight list item.
type JurnalHeaderSummary struct { //nolint:revive // Jurnal prefix required for cross-package clarity
	ID                 uuid.UUID          `json:"id"`
	NoJurnal           string             `json:"noJurnal"`
	TanggalPosting     time.Time          `json:"tanggalPosting"`
	EventCode          string             `json:"eventCode"`
	TotalDebit         decimal.Decimal    `json:"totalDebit"`
	TotalKredit        decimal.Decimal    `json:"totalKredit"`
	StatusInternal     JurnalHeaderStatus `json:"statusInternal"`
	ReferenceEventType string             `json:"referenceEventType"`
	CreatedAt          time.Time          `json:"createdAt"`
}

// JurnalWorkflowTransitionResponse is the response for manual jurnal workflow endpoints.
type JurnalWorkflowTransitionResponse struct { //nolint:revive // Jurnal prefix required for cross-package clarity
	ID             uuid.UUID          `json:"id"`
	NoJurnal       string             `json:"noJurnal"`
	StatusInternal JurnalHeaderStatus `json:"statusInternal"`
}

// ─── Request types — Manual Posting ───────────────────────────────────────────

// ManualPostRequest is the body for POST /jurnal/post.
type ManualPostRequest struct {
	EventCode    string          `json:"eventCode" binding:"required"`
	PeriodeID    uuid.UUID       `json:"periodeId" binding:"required"`
	InstrumenID  *uuid.UUID      `json:"instrumenId,omitempty"`
	AmountIDR    decimal.Decimal `json:"amountIdr"`
	Narasi       string          `json:"narasi" binding:"required,max=500"`
	DokumenDocID *uuid.UUID      `json:"dokumenDocId,omitempty"`
}

// ─── Domain types — DLQ ───────────────────────────────────────────────────────

// DLQEntry maps to sys.dlq_jurnal_post.
type DLQEntry struct {
	ID                  uuid.UUID  `json:"id"`
	SourceEventID       uuid.UUID  `json:"sourceEventId"`
	SourceEventType     string     `json:"sourceEventType"`
	EventCode           string     `json:"eventCode"`
	InstrumenID         *uuid.UUID `json:"instrumenId,omitempty"`
	PeriodeID           *uuid.UUID `json:"periodeId,omitempty"`
	PayloadJSONB        []byte     `json:"payloadJsonb,omitempty"`
	ErrorCode           string     `json:"errorCode"`
	ErrorMessage        string     `json:"errorMessage"`
	ErrorCategory       string     `json:"errorCategory"` // "DOMAIN" | "INFRA"
	RetryCount          int        `json:"retryCount"`
	LastRetryAt         *time.Time `json:"lastRetryAt,omitempty"`
	Status              DLQStatus  `json:"status"`
	ReplayedBy          *uuid.UUID `json:"replayedBy,omitempty"`
	ReplayedAt          *time.Time `json:"replayedAt,omitempty"`
	FinalJurnalHeaderID *uuid.UUID `json:"finalJurnalHeaderId,omitempty"`
	DiscardedReason     *string    `json:"discardedReason,omitempty"`
	DiscardedBy         *uuid.UUID `json:"discardedBy,omitempty"`
	DiscardedAt         *time.Time `json:"discardedAt,omitempty"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
	RowVersion          int64      `json:"rowVersion"`
}

// DLQEntrySummary is the lightweight list item.
type DLQEntrySummary struct {
	ID              uuid.UUID  `json:"id"`
	SourceEventType string     `json:"sourceEventType"`
	EventCode       string     `json:"eventCode"`
	ErrorCode       string     `json:"errorCode"`
	ErrorCategory   string     `json:"errorCategory"`
	RetryCount      int        `json:"retryCount"`
	Status          DLQStatus  `json:"status"`
	LastRetryAt     *time.Time `json:"lastRetryAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
}

// DLQReplayRequest is the body for POST /jurnal/dlq/{id}/replay.
type DLQReplayRequest struct {
	Reason *string `json:"reason,omitempty"`
}

// DLQDiscardRequest is the body for POST /jurnal/dlq/{id}/discard.
type DLQDiscardRequest struct {
	DiscardReason string `json:"discardReason" binding:"required,min=30"`
}

// DLQReplayResponse is returned on successful replay enqueue.
type DLQReplayResponse struct {
	DLQId     uuid.UUID `json:"dlqId"`
	JobID     string    `json:"jobId"`
	StatusURL string    `json:"statusUrl"`
}

// ─── Asynq event payloads (re-declared for P5-M2 subscriber use) ──────────────
// These mirror the types defined in internal/app-b/penempatan/domain.go.
// P5-M2 worker imports these from penempatan package to avoid duplication.

// DLQPostPayload is the full resolver input serialized into sys.dlq_jurnal_post.payload_jsonb.
// This is what gets replayed by DLQService.Replay().
type DLQPostPayload struct {
	EventCode         string          `json:"eventCode"`
	KlasifikasiPSAK71 string          `json:"klasifikasiPsak71"`
	InstrumenID       *string         `json:"instrumenId,omitempty"` // UUID string
	PeriodeID         string          `json:"periodeId"`
	AmountIDR         decimal.Decimal `json:"amountIdr"`
	Currency          string          `json:"currency"`
	FxRate            decimal.Decimal `json:"fxRate"`
	SourceEventID     string          `json:"sourceEventId"`
	SourceEventType   string          `json:"sourceEventType"`
	MetadataJSON      json.RawMessage `json:"metadataJson,omitempty"`
	Narasi            string          `json:"narasi,omitempty"`
}
