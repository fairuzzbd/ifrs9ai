// Package penempatan implements the Penempatan Deposito transaction lifecycle (P5-M1).
//
// State machine: docs/state-machines/p5-m1-penempatan.md §1.
// API contract: api/openapi/app-b-penempatan-deposito.yaml (15 endpoints).
// Compliance decisions: DEC-P5-M1-001 (FVTPL guard), DEC-P5-M1-004 (settlement hint),
//                       DEC-P5-M1-005 (terminate 4-eyes), DEC-017 (SoD), DEC-018 (audit).
//
// No float64 for money/rates — shopspring/decimal everywhere (DEC-016).
// Audit-in-tx mandatory for all state transitions (DEC-018).
// Idempotency-Key mandatory on all mutating endpoints (DEC-021).
// SoD server-side enforcement: maker≠reviewer≠approver, terminate maker≠reviewer≠approver.
package penempatan

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── Status enum ──────────────────────────────────────────────────────────────

// Status is the lifecycle status of a trx.penempatan_deposito row.
// Mirrors the DB enum trx.penempatan_workflow_status (migration 000033).
type Status string

// Workflow status constants mirroring trx.penempatan_workflow_status DB enum.
const (
	StatusDraft                       Status = "DRAFT"
	StatusPendingReview               Status = "PENDING_REVIEW"
	StatusPendingApproval             Status = "PENDING_APPROVAL"
	StatusApprovedActive              Status = "APPROVED_ACTIVE"
	StatusRejected                    Status = "REJECTED"
	StatusCancelled                   Status = "CANCELLED"
	StatusMatured                     Status = "MATURED"
	StatusTerminationPendingReview    Status = "TERMINATION_PENDING_REVIEW"
	StatusTerminationPendingApproval  Status = "TERMINATION_PENDING_APPROVAL"
	StatusTerminated                  Status = "TERMINATED"
	StatusTerminationRejected         Status = "TERMINATION_REJECTED"
)

// IsTerminal returns true for terminal statuses (no further transitions).
func (s Status) IsTerminal() bool {
	return s == StatusMatured || s == StatusTerminated || s == StatusCancelled
}

// CanSubmit returns true if the status allows /submit transition.
func (s Status) CanSubmit() bool { return s == StatusDraft }

// CanEdit returns true if the status allows PATCH (edit DRAFT only).
func (s Status) CanEdit() bool { return s == StatusDraft }

// CanWithdraw returns true if the status allows DELETE (withdraw DRAFT only).
func (s Status) CanWithdraw() bool { return s == StatusDraft }

// CanReview returns true if the status allows /review transition.
func (s Status) CanReview() bool { return s == StatusPendingReview }

// CanApprove returns true if the status allows /approve transition.
func (s Status) CanApprove() bool { return s == StatusPendingApproval }

// CanReject returns true if the status allows /reject transition (from review or approval steps).
func (s Status) CanReject() bool {
	return s == StatusPendingReview || s == StatusPendingApproval
}

// CanRequestTerminate returns true if the status allows /terminate.
func (s Status) CanRequestTerminate() bool { return s == StatusApprovedActive }

// CanTerminateReview returns true if the status allows /terminate-review.
func (s Status) CanTerminateReview() bool { return s == StatusTerminationPendingReview }

// CanTerminateApprove returns true if the status allows /terminate-approve.
func (s Status) CanTerminateApprove() bool { return s == StatusTerminationPendingApproval }

// CanTerminateReject returns true if the status allows /terminate-reject.
func (s Status) CanTerminateReject() bool {
	return s == StatusTerminationPendingReview || s == StatusTerminationPendingApproval
}

// ─── Permission constants ──────────────────────────────────────────────────────

// Permission strings used in JWT claims and server-side authorization checks.
const (
	PermTransaksiCreate    = "transaksi.create"
	PermTransaksiRead      = "transaksi.read"
	PermTransaksiUpdate    = "transaksi.update"
	PermTransaksiDelete    = "transaksi.delete"
	PermTransaksiSubmit    = "transaksi.submit"
	PermTransaksiReview    = "transaksi.review"
	PermTransaksiApprove   = "transaksi.approve"
	PermTransaksiReject    = "transaksi.reject"
	PermTransaksiTerminate = "transaksi.terminate"
	PermAuditLogRead       = "audit_log.read"
	PermEIRPreview         = "eir.preview"
)

// ─── Error code constants ──────────────────────────────────────────────────────
// Stable error codes (OpenAPI PenempatanErrorCode enum, app-b-penempatan-deposito.yaml).

// Domain-level error codes returned in API error envelopes.
const (
	ErrCodeInstrumenNotFound             = "PENEMPATAN_INSTRUMEN_NOT_FOUND"
	ErrCodeInstrumenInvalidKlasifikasi   = "PENEMPATAN_INSTRUMEN_INVALID_KLASIFIKASI"
	ErrCodeTanggalPenempatanInvalid      = "PENEMPATAN_TANGGAL_PENEMPATAN_INVALID"
	ErrCodeTenorInvalid                  = "PENEMPATAN_TENOR_INVALID"
	ErrCodeKuponInvalid                  = "PENEMPATAN_KUPON_INVALID"
	ErrCodeInvalidTransition             = "PENEMPATAN_INVALID_TRANSITION"
	ErrCodeSoDViolation                  = "PENEMPATAN_SOD_VIOLATION"
	ErrCodeStepUpRequired                = "PENEMPATAN_STEP_UP_REQUIRED"
	ErrCodeReasonTooShort                = "PENEMPATAN_REASON_TOO_SHORT"
	ErrCodeEditLocked                    = "PENEMPATAN_EDIT_LOCKED"
	ErrCodePeriodeHardClosed             = "PENEMPATAN_PERIODE_HARD_CLOSED"
	ErrCodeTerminateForbiddenNotActive   = "PENEMPATAN_TERMINATE_FORBIDDEN_NOT_ACTIVE"
	ErrCodeCalc2010                      = "ERR_CALC_2010"
	ErrCodeNotFound                      = "PENEMPATAN_NOT_FOUND"
)

// ─── Domain types ─────────────────────────────────────────────────────────────

// Penempatan maps to trx.penempatan_deposito (full row).
// Money fields use decimal.Decimal (DEC-016, no float64).
type Penempatan struct {
	ID               uuid.UUID `json:"id"`
	KodeTransaksi    string    `json:"kodeTransaksi"`

	// References
	InstrumenID       uuid.UUID `json:"instrumenId"`
	CounterpartyBankID uuid.UUID `json:"counterpartyBankId"`
	PeriodeID         uuid.UUID `json:"periodeId"`
	MataUangID        uuid.UUID `json:"mataUangId"`

	// Transaction fields
	TanggalPenempatan  time.Time       `json:"tanggalPenempatan"`
	TanggalJatuhTempo  time.Time       `json:"tanggalJatuhTempo"`
	NominalIDR         decimal.Decimal `json:"nominalIdr"`         // NUMERIC(20,4)
	NominalFCY         *decimal.Decimal `json:"nominalFcy"`         // NULL for IDR
	KursPenempatan     *decimal.Decimal `json:"kursPenempatan"`     // NULL for IDR; NUMERIC(20,8)
	TenorBulan         int16           `json:"tenorBulan"`
	KuponPersen        decimal.Decimal `json:"kuponPersen"`        // NUMERIC(10,8)
	BiayaTransaksiIDR  decimal.Decimal `json:"biayaTransaksiIdr"`  // NUMERIC(20,4)
	NomorReferensiBank *string         `json:"nomorReferensiBank"`
	SettlementAccount  *string         `json:"settlementAccount"`
	Catatan            *string         `json:"catatan"`

	// EIR (populated async post-approve — NULL until computed, NULL for FVTPL/FVOCI_ELECTION)
	EIRAwal            *decimal.Decimal `json:"eirAwal"`            // NUMERIC(10,8)
	CarryingAmountAwal *decimal.Decimal `json:"carryingAmountAwal"` // NUMERIC(20,4)

	// Document references
	KontrakDocID      *uuid.UUID `json:"kontrakDocId"`
	DokumenTerminasiID *uuid.UUID `json:"dokumenTerminasiId"`

	// Workflow
	WorkflowStatus Status `json:"workflowStatus"`

	// Create workflow participants
	MakerID    uuid.UUID  `json:"makerId"`
	ReviewerID *uuid.UUID `json:"reviewerId"`
	ApproverID *uuid.UUID `json:"approverId"`

	// Create workflow signatures
	ReviewerSignedAt       *time.Time `json:"reviewerSignedAt"`
	ApproverSignedAt       *time.Time `json:"approverSignedAt"`
	ReviewerSignatureHash  []byte     `json:"-"`
	ApproverSignatureHash  []byte     `json:"-"`

	// Reject
	RejectReason    *string `json:"rejectReason"`
	CommentReview   *string `json:"commentReview"`
	CommentApprove  *string `json:"commentApprove"`

	// Terminate workflow participants (DEC-P5-M1-005)
	TerminateMakerID    *uuid.UUID `json:"terminateMakerId"`
	TerminateReviewerID *uuid.UUID `json:"terminateReviewerId"`
	TerminateApproverID *uuid.UUID `json:"terminateApproverId"`

	// Terminate workflow signatures
	TerminateReviewerSignedAt      *time.Time `json:"terminateReviewerSignedAt"`
	TerminateApproverSignedAt      *time.Time `json:"terminateApproverSignedAt"`
	TerminateReviewerSignatureHash []byte     `json:"-"`
	TerminateApproverSignatureHash []byte     `json:"-"`

	// Terminate reasons and comments
	TerminateRequestReason  *string `json:"terminateRequestReason"`
	TerminateReviewComment  *string `json:"terminateReviewComment"`
	TerminateApproveComment *string `json:"terminateApproveComment"`
	TerminateRejectReason   *string `json:"terminateRejectReason"`

	// Lifecycle timestamps
	TerminatedAt *time.Time      `json:"terminatedAt"`
	MaturedAt    *time.Time      `json:"maturedAt"`
	RealizedGainLossIDR *decimal.Decimal `json:"realizedGainLossIdr"` // set by P5-M9

	// Audit columns
	CreatedAt  time.Time `json:"createdAt"`
	CreatedBy  uuid.UUID `json:"createdBy"`
	UpdatedAt  time.Time `json:"updatedAt"`
	UpdatedBy  uuid.UUID `json:"updatedBy"`
	DeletedAt  *time.Time `json:"deletedAt,omitempty"`
	DeletedBy  *uuid.UUID `json:"deletedBy,omitempty"`
	RowVersion int64     `json:"rowVersion"`
	TenantID   string    `json:"tenantId"`

	// Settlement balance hint (informational, DEC-P5-M1-004) — populated at service layer
	SettlementBalanceHint *SettlementBalanceHint `json:"settlementBalanceHint,omitempty"`

	// Post-approve response fields
	EIRComputeJobID *string `json:"eirComputeJobId,omitempty"`
	StagingAction   string  `json:"stagingAction,omitempty"` // STAGE_1_ASSIGNED | SKIPPED_FVTPL

	// Joined fields for list view
	NamaCounterparty   string `json:"namaCounterparty,omitempty"`
	KlasifikasiPSAK71  string `json:"klasifikasiPsak71,omitempty"`
	TipeInstrumen      string `json:"tipeInstrumen,omitempty"`
	NamaInstrumen      string `json:"namaInstrumen,omitempty"`
	LabelPeriode       string `json:"labelPeriode,omitempty"`
}

// SettlementBalanceHint is the informational balance hint (DEC-P5-M1-004).
// Never blocks create — informational display only in Phase 5.
type SettlementBalanceHint struct {
	LastKnownIDR decimal.Decimal `json:"lastKnownIdr"` // NUMERIC(20,4)
	AsOfDate     time.Time       `json:"asOfDate"`
	IsStale      bool            `json:"isStale"`   // true if updated_at > 24h
	IsSufficient *bool           `json:"isSufficient"` // always nil in Phase 5
}

// ListItem is the lightweight list view for DataTable.
type ListItem struct {
	ID                uuid.UUID       `json:"id"`
	KodeTransaksi     string          `json:"kodeTransaksi"`
	WorkflowStatus    Status          `json:"workflowStatus"`
	NominalIDR        decimal.Decimal `json:"nominalIdr"`
	TanggalPenempatan time.Time       `json:"tanggalPenempatan"`
	TanggalJatuhTempo time.Time       `json:"tanggalJatuhTempo"`
	KuponPersen       decimal.Decimal `json:"kuponPersen"`
	TenorBulan        int16           `json:"tenorBulan"`
	NamaCounterparty  string          `json:"namaCounterparty"`
	KlasifikasiPSAK71 string          `json:"klasifikasiPsak71"`
	TipeInstrumen     string          `json:"tipeInstrumen"`
	NamaInstrumen     string          `json:"namaInstrumen"`
	MakerID           uuid.UUID       `json:"makerId"`
	CreatedAt         time.Time       `json:"createdAt"`
	DeletedAt         *time.Time      `json:"deletedAt,omitempty"`
}

// ─── Request types ────────────────────────────────────────────────────────────

// CreateRequest is the payload for POST /trx/penempatan-deposito.
// Money fields use decimal.Decimal to avoid float64 (DEC-016).
type CreateRequest struct {
	InstrumenID        uuid.UUID       `json:"instrumenId" binding:"required"`
	CounterpartyBankID uuid.UUID       `json:"counterpartyBankId" binding:"required"`
	PeriodeID          uuid.UUID       `json:"periodeId" binding:"required"`
	TanggalPenempatan  string          `json:"tanggalPenempatan" binding:"required"` // YYYY-MM-DD
	NominalIDR         *decimal.Decimal `json:"nominalIdr"`
	NominalFCY         *decimal.Decimal `json:"nominalFcy"`
	MataUangID         uuid.UUID       `json:"mataUangId" binding:"required"`
	TenorBulan         int16           `json:"tenorBulan" binding:"required,min=1"`
	KuponPersen        decimal.Decimal `json:"kuponPersen" binding:"required"`
	BiayaTransaksiIDR  decimal.Decimal `json:"biayaTransaksiIdr"`
	NomorReferensiBank *string         `json:"nomorReferensiBankIn"`
	SettlementAccount  *string         `json:"settlementAccount"`
	Catatan            *string         `json:"catatan"`
	KontrakDocID       *uuid.UUID      `json:"kontrakDocId"`
}

// UpdateRequest is the payload for PATCH /trx/penempatan-deposito/{id}.
// rowVersion is required for optimistic locking (DEC-016 CONFLICT).
type UpdateRequest struct {
	RowVersion        int64            `json:"rowVersion" binding:"required"`
	TanggalPenempatan *string          `json:"tanggalPenempatan"` // YYYY-MM-DD
	NominalIDR        *decimal.Decimal `json:"nominalIdr"`
	NominalFCY        *decimal.Decimal `json:"nominalFcy"`
	KuponPersen       *decimal.Decimal `json:"kuponPersen"`
	TenorBulan        *int16           `json:"tenorBulan"`
	BiayaTransaksiIDR *decimal.Decimal `json:"biayaTransaksiIdr"`
	NomorReferensiBank *string         `json:"nomorReferensiBank"`
	SettlementAccount  *string         `json:"settlementAccount"`
	Catatan            *string         `json:"catatan"`
	KontrakDocID       *uuid.UUID      `json:"kontrakDocId"`
}

// WorkflowActionRequest is the body for workflow transition endpoints (submit/review/approve).
type WorkflowActionRequest struct {
	Comment         string `json:"comment" binding:"required,min=1"`
	SignatureMethod  string `json:"signatureMethod" binding:"required"`
}

// RejectActionRequest is the body for reject endpoints (comment ≥ 30 chars enforced).
type RejectActionRequest struct {
	Comment        string `json:"comment" binding:"required,min=30"`
	SignatureMethod string `json:"signatureMethod"`
}

// TerminateRequestBody is the body for POST /terminate.
type TerminateRequestBody struct {
	TerminateReason    string     `json:"terminateReason" binding:"required,min=30"`
	DokumenTerminasiID *uuid.UUID `json:"dokumenTerminasiId"`
	SignatureMethod     string    `json:"signatureMethod"`
}

// ─── Asynq task types ─────────────────────────────────────────────────────────

// PenempatanApprovedTaskType is the Asynq task type for downstream consumers (P5-M2 jurnal).
const PenempatanApprovedTaskType = "penempatan:approved"

// PenempatanMaturedTaskType is the Asynq task type emitted by the maturity cron.
const PenempatanMaturedTaskType = "penempatan:matured"

// PenempatanTerminatedTaskType is the Asynq task type for downstream consumers (P5-M9).
const PenempatanTerminatedTaskType = "penempatan:terminated"

// EIRComputeTaskType is the Asynq task type for EIR initial compute (P4-M5 subscriber).
const EIRComputeTaskType = "eir:compute_initial"

// MaturityCheckTaskType is the Asynq cron task type for daily maturity scan.
const MaturityCheckTaskType = "penempatan:maturity_check"

// ApprovedEvent is the downstream event emitted on approve (consumed by P5-M2).
type ApprovedEvent struct {
	InstrumenID        uuid.UUID        `json:"instrumenId"`
	PenempatanID       uuid.UUID        `json:"penempatanId"`
	KodeTransaksi      string           `json:"kodeTransaksi"`
	KlasifikasiPSAK71  string           `json:"klasifikasiPsak71"` // AC|FVOCI|FVTPL|FVOCI_ELECTION|POCI
	TanggalPenempatan  time.Time        `json:"tanggalPenempatan"`
	TanggalJatuhTempo  time.Time        `json:"tanggalJatuhTempo"`
	NominalIDR         decimal.Decimal  `json:"nominalIdr"`         // NUMERIC(20,4)
	NominalFCY         *decimal.Decimal `json:"nominalFcy"`
	MataUangKode       string           `json:"mataUangKode"`
	KursPenempatan     *decimal.Decimal `json:"kursPenempatan"`
	KuponPersen        decimal.Decimal  `json:"kuponPersen"`        // NUMERIC(10,8)
	TenorBulan         int16            `json:"tenorBulan"`
	BiayaTransaksiIDR  decimal.Decimal  `json:"biayaTransaksiIdr"` // NUMERIC(20,4)
	PeriodeID          uuid.UUID        `json:"periodeId"`
	StagingAction      string           `json:"stagingAction"` // STAGE_1_ASSIGNED | SKIPPED_FVTPL
	EventTime          time.Time        `json:"eventTime"`
	TenantID           string           `json:"tenantId"`
}

// EIRComputePayload is the Asynq task payload for EIR initial compute (consumed by P4-M5).
type EIRComputePayload struct {
	PenempatanID      uuid.UUID       `json:"penempatanId"`
	InstrumenID       uuid.UUID       `json:"instrumenId"`
	KlasifikasiPSAK71 string         `json:"klasifikasiPsak71"`
	NominalIDR        decimal.Decimal `json:"nominalIdr"`
	KuponPersen       decimal.Decimal `json:"kuponPersen"`
	TenorBulan        int16          `json:"tenorBulan"`
	BiayaTransaksiIDR decimal.Decimal `json:"biayaTransaksiIdr"`
	TanggalPenempatan time.Time      `json:"tanggalPenempatan"`
	TanggalJatuhTempo time.Time      `json:"tanggalJatuhTempo"`
	PeriodeID         uuid.UUID      `json:"periodeId"`
	TenantID          string         `json:"tenantId"`
}

// MaturedEvent is emitted by the Asynq maturity-checker cron (consumed by P5-M9).
type MaturedEvent struct {
	InstrumenID       uuid.UUID        `json:"instrumenId"`
	PenempatanID      uuid.UUID        `json:"penempatanId"`
	KodeTransaksi     string           `json:"kodeTransaksi"`
	KlasifikasiPSAK71 string          `json:"klasifikasiPsak71"`
	TanggalJatuhTempo time.Time        `json:"tanggalJatuhTempo"`
	MaturedAt         time.Time        `json:"maturedAt"`
	NominalIDR        decimal.Decimal  `json:"nominalIdr"`
	EIRAwal           *decimal.Decimal `json:"eirAwal"` // null for FVTPL
	PeriodeID         uuid.UUID        `json:"periodeId"`
	EventTime         time.Time        `json:"eventTime"`
	TenantID          string           `json:"tenantId"`
}

// TerminatedEvent is emitted on terminate-approve (consumed by P5-M9).
type TerminatedEvent struct {
	InstrumenID            uuid.UUID        `json:"instrumenId"`
	PenempatanID           uuid.UUID        `json:"penempatanId"`
	KodeTransaksi          string           `json:"kodeTransaksi"`
	KlasifikasiPSAK71      string          `json:"klasifikasiPsak71"`
	TerminateDate          time.Time        `json:"terminateDate"`
	TerminateReason        string           `json:"terminateReason"`
	NominalIDR             decimal.Decimal  `json:"nominalIdr"`
	EIRAwal                *decimal.Decimal `json:"eirAwal"`
	CurrentStage           *int             `json:"currentStage"` // 1|2|3; null for FVTPL
	RealizedGainLossIDR    *decimal.Decimal `json:"realizedGainLossIdr"` // computed by P5-M9
	PeriodeID              uuid.UUID        `json:"periodeId"`
	EventTime              time.Time        `json:"eventTime"`
	TenantID               string           `json:"tenantId"`
}

// ─── EIR Preview types ────────────────────────────────────────────────────────

// EIRPreviewResult is returned by GET /eir-preview.
type EIRPreviewResult struct {
	EIRAwal            *decimal.Decimal        `json:"eirAwal"`            // null for FVTPL
	CarryingAmountAwal *decimal.Decimal        `json:"carryingAmountAwal"` // null for FVTPL
	PeriodePreview     int                     `json:"periodePreview"`
	Info               *string                 `json:"info,omitempty"` // informational for FVTPL
	AmortizationSchedule []AmortizationRow     `json:"amortizationSchedule"`
}

// AmortizationRow is one row of the EIR amortization preview.
type AmortizationRow struct {
	Periode          int             `json:"periode"`
	TanggalAngsuran  time.Time       `json:"tanggalAngsuran"`
	AngsuranBunga    decimal.Decimal `json:"angsuranBunga"`    // NUMERIC(20,4)
	AngsuranPokok    decimal.Decimal `json:"angsuranPokok"`    // NUMERIC(20,4)
	CarryingAmount   decimal.Decimal `json:"carryingAmount"`   // NUMERIC(20,4)
}

// ─── Audit timeline types ─────────────────────────────────────────────────────

// AuditTimelineEvent is one row from aud.audit_log for a penempatan.
type AuditTimelineEvent struct {
	EventID     uuid.UUID  `json:"eventId"`
	EventTime   time.Time  `json:"eventTime"`
	ActorUserID uuid.UUID  `json:"actorUserId"`
	ActorRole   string     `json:"actorRole"`
	Action      string     `json:"action"`
	BeforeJSON  *string    `json:"before,omitempty"`  // nil for non-AUDIT roles
	AfterJSON   *string    `json:"after,omitempty"`   // nil for non-AUDIT roles
	TraceID     string     `json:"traceId"`
}
