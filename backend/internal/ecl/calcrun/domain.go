// Package calcrun implements the ECL Calc Run lifecycle and Seal workflow (P4-M8).
//
// Responsibilities:
//   - CRUD for ecl.calc_run (run-level header entity).
//   - Status state machine: DRAFT → IN_PROGRESS → COMPLETED / COMPLETED_WITH_ERRORS →
//     SEAL_REQUESTED → SEALED (terminal) or COMPLETED (after SEAL_REJECTED).
//     DRAFT / IN_PROGRESS → CANCELLED (terminal).
//   - Parameter snapshot: freeze all ALCO-approved params at /start time (full snapshot).
//   - Asynq task: ECL_CALC_RUN — dispatches M7 ComputeBulk, reports progress.
//   - Seal (4-eyes): RISK request → ALCO/CFO approve + step-up MFA (DEC-027).
//   - SoD: seal_requested_by ≠ seal_approved_by (server-side, DEC-017).
//   - Immutability: DB trigger + service guard after SEALED.
//   - Audit in-transaction (DEC-018): every state transition writes to aud.audit_log.
//
// Compliance: FSD-APP-C §5 (calc run), §6 (seal). SoW §4. DEC-010, 016, 017, 018, 021, 026, 027.
// State machine: docs/state-machines/p4-m8-calc-run.md.
// Stories: APP-C-M8-001..006.
// Migration: db/migrations/000031_calc_run_lifecycle_seal.up.sql.
package calcrun

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ─── Status enum ──────────────────────────────────────────────────────────────

// Status is the lifecycle status of an ecl.calc_run row.
// All valid values are listed below; any other value is rejected by service guards
// and the DB CHECK constraint.
type Status string

const (
	// StatusDraft is the initial state after creation. No Asynq job yet.
	StatusDraft Status = "DRAFT"

	// StatusInProgress means an Asynq bulk compute job is running.
	StatusInProgress Status = "IN_PROGRESS"

	// StatusCompleted means bulk compute finished with zero errors.
	// Eligible for seal request.
	StatusCompleted Status = "COMPLETED"

	// StatusCompletedWithErrors means bulk compute finished but some instruments failed.
	// Seal request is BLOCKED until errors are fixed (CALC_RUN_HAS_ERRORS).
	StatusCompletedWithErrors Status = "COMPLETED_WITH_ERRORS"

	// StatusSealRequested means ROLE-RISK submitted a seal request.
	// Awaiting ROLE-ALCO / ROLE-CFO approve.
	StatusSealRequested Status = "SEAL_REQUESTED"

	// StatusSealed is the terminal immutable state.
	// DB trigger blocks all further updates; service guard returns ECL_PARAM_FROZEN.
	StatusSealed Status = "SEALED"

	// StatusCancelled is a terminal state for calc runs that were abandoned.
	// Partial ecl.calc_result_line rows are preserved for audit.
	StatusCancelled Status = "CANCELLED"
)

// IsTerminal returns true for terminal states (SEALED, CANCELLED).
func (s Status) IsTerminal() bool {
	return s == StatusSealed || s == StatusCancelled
}

// CanStart returns true if this status allows a /start transition.
func (s Status) CanStart() bool { return s == StatusDraft }

// CanCancel returns true if this status allows a /cancel transition.
func (s Status) CanCancel() bool { return s == StatusDraft || s == StatusInProgress }

// CanRequestSeal returns true if this status allows a /seal/request transition.
func (s Status) CanRequestSeal() bool { return s == StatusCompleted }

// CanApproveSeal returns true if this status allows a /seal/approve or /seal/reject.
func (s Status) CanApproveSeal() bool { return s == StatusSealRequested }

// ─── Permission constants ──────────────────────────────────────────────────────

const (
	// PermCalcRunCreate allows creating a new calc_run (DRAFT). ROLE-RISK.
	PermCalcRunCreate = "calc_run.create"

	// PermCalcRunRead allows reading calc_run status and results. ROLE-RISK,ROLE-AUDIT,ROLE-AKUN,ROLE-AKUN-CTL,ROLE-CFO.
	PermCalcRunRead = "calc_run.read"

	// PermCalcRunStart allows triggering bulk compute (DRAFT → IN_PROGRESS). ROLE-RISK.
	PermCalcRunStart = "calc_run.start"

	// PermCalcRunCancel allows canceling a calc_run (maker only). ROLE-RISK.
	PermCalcRunCancel = "calc_run.cancel"

	// PermCalcRunSealRequest allows submitting a seal request. ROLE-RISK.
	PermCalcRunSealRequest = "calc_run.seal_request"

	// PermCalcRunSealApprove allows approving or rejecting a seal request (step-up MFA). ROLE-ALCO, ROLE-CFO.
	PermCalcRunSealApprove = "calc_run.seal_approve"

	// PermCalcRunExport allows exporting calc_run results. ROLE-RISK, ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT.
	PermCalcRunExport = "calc_run.export"
)

// ─── Domain types ─────────────────────────────────────────────────────────────

// CalcRun is the run-level header for a bulk ECL computation.
// Maps to ecl.calc_run (migration 000031).
type CalcRun struct {
	ID             uuid.UUID `json:"id"`
	PeriodeID      string    `json:"periodeId"`
	EvaluationDate time.Time `json:"evaluationDate"`
	Scope          string    `json:"scope"` // "ALL_ACTIVE"
	Status         Status    `json:"status"`

	// Asynq job linkage.
	JobID *string `json:"jobId,omitempty"`

	// Progress counters.
	TotalInstrumen *int `json:"totalInstrumen,omitempty"`
	ProcessedCount int  `json:"processedCount"`
	ErrorCount     int  `json:"errorCount"`

	// Timing.
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`

	// Parameter snapshot frozen at /start.
	ParameterSnapshotJSONB []byte `json:"parameterSnapshot,omitempty"`

	// Seal workflow (4-eyes: RISK request → ALCO/CFO approve).
	SealRequestedBy *uuid.UUID `json:"sealRequestedBy,omitempty"`
	SealRequestedAt *time.Time `json:"sealRequestedAt,omitempty"`
	SealComment     *string    `json:"sealComment,omitempty"`

	SealApprovedBy    *uuid.UUID `json:"sealApprovedBy,omitempty"`
	SealApprovedAt    *time.Time `json:"sealApprovedAt,omitempty"`
	SealedAt          *time.Time `json:"sealedAt,omitempty"`
	SignatureHashSeal []byte     `json:"signatureHashSeal,omitempty"`

	// Seal reject tracking.
	SealRejectedBy *uuid.UUID `json:"sealRejectedBy,omitempty"`
	SealRejectedAt *time.Time `json:"sealRejectedAt,omitempty"`
	RejectReason   *string    `json:"rejectReason,omitempty"`

	// Cancel tracking.
	CancelledBy  *uuid.UUID `json:"cancelledBy,omitempty"`
	CancelledAt  *time.Time `json:"cancelledAt,omitempty"`
	CancelReason *string    `json:"cancelReason,omitempty"`

	// Superseded tracking (audit only; no auto-supersede logic).
	SupersededByRunID *uuid.UUID `json:"supersededByRunId,omitempty"`

	// Audit columns.
	CreatedAt  time.Time `json:"createdAt"`
	CreatedBy  uuid.UUID `json:"createdBy"`
	UpdatedAt  time.Time `json:"updatedAt"`
	UpdatedBy  uuid.UUID `json:"updatedBy"`
	RowVersion int64     `json:"rowVersion"`
	TenantID   string    `json:"tenantId"`
}

// ─── Request types ────────────────────────────────────────────────────────────

// CreateRequest is the payload for POST /ecl/calc-runs.
type CreateRequest struct {
	PeriodeID      string `json:"periodeId" binding:"required,max=50"`
	EvaluationDate string `json:"evaluationDate" binding:"required"` // YYYY-MM-DD
	Scope          string `json:"scope" binding:"required,oneof=ALL_ACTIVE"`
	Comment        string `json:"comment,omitempty"` // max 500 chars
}

// StartRequest is the payload for POST /ecl/calc-runs/{id}/start.
type StartRequest struct {
	// Currently no body fields required for start.
	// ActorID is taken from JWT claims.
}

// CancelRequest is the payload for POST /ecl/calc-runs/{id}/cancel.
type CancelRequest struct {
	// CancelReason must be ≥ 30 characters (DB CHECK + service guard).
	CancelReason string `json:"cancelReason" binding:"required,min=30,max=1000"`
}

// SealRequestBody is the payload for POST /ecl/calc-runs/{id}/seal/request.
type SealRequestBody struct {
	// Comment is required (min 10 chars) to explain the seal rationale.
	Comment string `json:"comment" binding:"required,min=10,max=1000"`
}

// SealApproveBody is the payload for POST /ecl/calc-runs/{id}/seal/approve.
type SealApproveBody struct {
	// Comment is required (min 10 chars) for ALCO/CFO approval confirmation.
	Comment string `json:"comment" binding:"required,min=10,max=1000"`
}

// SealRejectBody is the payload for POST /ecl/calc-runs/{id}/seal/reject.
type SealRejectBody struct {
	// RejectReason is required (min 10 chars).
	RejectReason string `json:"rejectReason" binding:"required,min=10,max=1000"`
}

// ─── Response types ───────────────────────────────────────────────────────────

// StartResponse is returned by POST /ecl/calc-runs/{id}/start.
// Follows UX §3 long-running process pattern.
type StartResponse struct {
	JobID     string `json:"jobId"`
	StatusURL string `json:"statusUrl"`
	StreamURL string `json:"streamUrl"`
}

// Summary is a lightweight list view (DataTable) of a calc run.
type Summary struct {
	ID             uuid.UUID  `json:"id"`
	PeriodeID      string     `json:"periodeId"`
	EvaluationDate time.Time  `json:"evaluationDate"`
	Scope          string     `json:"scope"`
	Status         Status     `json:"status"`
	ProcessedCount int        `json:"processedCount"`
	ErrorCount     int        `json:"errorCount"`
	TotalInstrumen *int       `json:"totalInstrumen,omitempty"`
	StartedAt      *time.Time `json:"startedAt,omitempty"`
	CompletedAt    *time.Time `json:"completedAt,omitempty"`
	SealedAt       *time.Time `json:"sealedAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	CreatedBy      uuid.UUID  `json:"createdBy"`
}

// ─── Interfaces ───────────────────────────────────────────────────────────────

// RepoIface is the minimal repo interface (used for testing / dependency inversion).
type RepoIface interface {
	// IsSealedCalcRun implements the core.CalcRunSealChecker interface.
	// Returns true if the calc_run with given id has status = 'SEALED'.
	IsSealedCalcRun(ctx context.Context, calcRunID uuid.UUID) (bool, error)
}
