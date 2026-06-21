// Package closeflow implements P5-M4 — Periode Buku Close Workflow.
//
// Responsibilities:
//   - Soft-close request + approve (4-eyes SoD, OPEN → SOFT_CLOSED).
//   - Hard-close request + CFO approve (step-up MFA, SOFT_CLOSED → CLOSED).
//   - CFO reject hard-close (HARD_CLOSE_PENDING → SOFT_CLOSED).
//   - Reopen (SOFT_CLOSED→OPEN, CLOSED→SOFT_CLOSED within grace window).
//   - Closing checklist real-time 4-item evaluation.
//   - Status periode report (list + sort + filter + export).
//   - PeriodeLockMiddleware: cross-cutting 423 enforcement.
//
// Compliance decisions:
//   - DEC-017: 4-eyes SoD; CFO sole approver for hard-close.
//   - DEC-018: Audit trail append-only; sys.closing_checklist_snapshot immutable.
//   - DEC-021: Idempotency-Key mandatory on all mutating endpoints.
//   - DEC-022: Cursor-based pagination.
//   - DEC-026: MFA mandatory for ROLE-CFO.
//   - DEC-027: Step-up MFA for hard-close-approve + reopen CLOSED→SOFT_CLOSED.
//   - DEC-036: Cure evaluation allowed during SOFT_CLOSED via CORRECTION_PERIODE_CLOSED.
//
// Invariants:
//   - sys.closing_checklist_snapshot: append-only (DB triggers enforce).
//   - Audit writes MUST be in-transaction with state mutation.
//   - Advisory audit events (on rejected paths) use context.Background() child tx.
//   - shopspring/decimal for JURNAL_BALANCED threshold check (no float64).
package closeflow

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── Status enum ─────────────────────────────────────────────────────────────

// PeriodeStatus mirrors mst.periode_buku.status_periode CHECK constraint.
// State machine (P5-M4):
//
//	OPEN → (soft-close-request) → OPEN [pending request]
//	OPEN → (soft-close-approve, SoD) → SOFT_CLOSED
//	SOFT_CLOSED → (hard-close-request) → HARD_CLOSE_PENDING
//	SOFT_CLOSED → (reopen) → OPEN
//	HARD_CLOSE_PENDING → (hard-close-approve, CFO step-up MFA) → CLOSED
//	HARD_CLOSE_PENDING → (hard-close-reject, CFO) → SOFT_CLOSED
//	CLOSED → (reopen in grace window, CFO step-up MFA) → SOFT_CLOSED
type PeriodeStatus string

const (
	PeriodeStatusOpen             PeriodeStatus = "OPEN"
	PeriodeStatusSoftClosed       PeriodeStatus = "SOFT_CLOSED"
	PeriodeStatusHardClosePending PeriodeStatus = "HARD_CLOSE_PENDING"
	PeriodeStatusClosed           PeriodeStatus = "CLOSED"
)

// IsValid returns true if the status is a known value.
func (s PeriodeStatus) IsValid() bool {
	switch s {
	case PeriodeStatusOpen, PeriodeStatusSoftClosed,
		PeriodeStatusHardClosePending, PeriodeStatusClosed:
		return true
	}
	return false
}

// AllowsMutation returns true if the status allows normal domain mutations
// (trx, jrnl, instrumen). SOFT_CLOSED and HARD_CLOSE_PENDING restrict mutations
// to allowlist; CLOSED blocks all.
func (s PeriodeStatus) AllowsMutation() bool {
	return s == PeriodeStatusOpen
}

// IsTerminal returns true for CLOSED (after grace window: effectively terminal via API).
func (s PeriodeStatus) IsTerminal() bool {
	return s == PeriodeStatusClosed
}

// ─── Checklist item key ───────────────────────────────────────────────────────

// ChecklistKey identifies one of the 4 closing pre-condition items.
type ChecklistKey string

const (
	ChecklistKeyPendingApprovalZero ChecklistKey = "PENDING_APPROVAL_ZERO"
	ChecklistKeyJurnalBalanced      ChecklistKey = "JURNAL_BALANCED"
	ChecklistKeyGLDelivered         ChecklistKey = "GL_DELIVERED"
	ChecklistKeyReconPass           ChecklistKey = "RECON_PASS"
)

// JurnalBalancedThreshold is the maximum allowed ABS(total_debit - total_kredit)
// per jrnl.header row. Uses decimal to avoid float64 (DEC-016).
// C7: Use RequireFromString instead of NewFromFloat to avoid IEEE 754 imprecision.
var JurnalBalancedThreshold = decimal.RequireFromString("0.01")

// ─── ChecklistItem ────────────────────────────────────────────────────────────

// ChecklistItem is the result of evaluating one closing pre-condition.
type ChecklistItem struct {
	Key       ChecklistKey `json:"key"`
	Label     string       `json:"label"`
	Passed    bool         `json:"passed"`
	Detail    string       `json:"detail"`
	ActionURL *string      `json:"actionUrl,omitempty"`
}

// ─── Snapshot ─────────────────────────────────────────────────────────────────

// SnapshotTransition mirrors sys.closing_checklist_snapshot.transition CHECK.
type SnapshotTransition string

const (
	SnapshotTransitionSoftCloseRequest SnapshotTransition = "SOFT_CLOSE_REQUEST"
	SnapshotTransitionSoftCloseApprove SnapshotTransition = "SOFT_CLOSE_APPROVE"
	SnapshotTransitionHardCloseRequest SnapshotTransition = "HARD_CLOSE_REQUEST"
	SnapshotTransitionHardCloseApprove SnapshotTransition = "HARD_CLOSE_APPROVE"
	SnapshotTransitionReopenRequest    SnapshotTransition = "REOPEN_REQUEST"
	SnapshotTransitionReopenApprove    SnapshotTransition = "REOPEN_APPROVE"
	SnapshotTransitionManualCheck      SnapshotTransition = "MANUAL_CHECK"
)

// SnapshotOverallStatus mirrors sys.closing_checklist_snapshot.overall_status CHECK.
type SnapshotOverallStatus string

const (
	SnapshotOverallPassed   SnapshotOverallStatus = "PASSED"
	SnapshotOverallFailed   SnapshotOverallStatus = "FAILED"
	SnapshotOverallRejected SnapshotOverallStatus = "REJECTED"
)

// SnapshotTransitionStatus mirrors sys.closing_checklist_snapshot.transition_status CHECK.
type SnapshotTransitionStatus string

const (
	SnapshotTransitionStatusApproved SnapshotTransitionStatus = "APPROVED"
	SnapshotTransitionStatusRejected SnapshotTransitionStatus = "REJECTED"
)

// ChecklistSnapshot is the in-memory representation of one sys.closing_checklist_snapshot row.
type ChecklistSnapshot struct {
	ID               uuid.UUID                `json:"snapshotId"`
	PeriodeBukuID    uuid.UUID                `json:"periodeBukuId"`
	Transition       SnapshotTransition       `json:"transition"`
	TriggerAction    SnapshotTransition       `json:"triggerAction"`
	EvaluatedAt      time.Time                `json:"evaluatedAt"`
	EvaluatedBy      uuid.UUID                `json:"evaluatedBy"`
	ActorRole        string                   `json:"actorRole"`
	OverallStatus    SnapshotOverallStatus    `json:"overallStatus"`
	AllPassed        bool                     `json:"allPassed"`
	TransitionStatus SnapshotTransitionStatus `json:"transitionStatus"`
	ChecklistItems   []ChecklistItem          `json:"items"`
	OutcomeJSON      map[string]any           `json:"outcomeJsonb,omitempty"`
	CreatedAt        time.Time                `json:"createdAt"`
	CreatedBy        uuid.UUID                `json:"createdBy"`
	TenantID         string                   `json:"tenantId"`
}

// ─── Periode Buku row ─────────────────────────────────────────────────────────

// PeriodeBuku is the mst.periode_buku row (close-workflow columns).
type PeriodeBuku struct {
	ID                 uuid.UUID     `db:"id"`
	PeriodeIDKode      string        `db:"periode_id_kode"`
	TahunBuku          int           `db:"tahun_buku"`
	Bulan              *int          `db:"bulan"`
	TipePeriode        string        `db:"tipe_periode"`
	TanggalMulai       time.Time     `db:"tanggal_mulai"`
	TanggalAkhir       time.Time     `db:"tanggal_akhir"`
	StatusPeriode      PeriodeStatus `db:"status_periode"`
	TanggalSoftClose   *time.Time    `db:"tanggal_soft_close"`
	TanggalHardClose   *time.Time    `db:"tanggal_hard_close"`
	ReopenedFlag       bool          `db:"reopened_flag"`
	ReopenedReason     *string       `db:"reopened_reason"`
	ReopenedAt         *time.Time    `db:"reopened_at"`
	ReopenedBy         *uuid.UUID    `db:"reopened_by"`
	ReopenedApprovedBy *uuid.UUID    `db:"reopened_approved_by"`
	RowVersion         int64         `db:"row_version"`
	// Close-workflow tracking columns (migration 000038).
	SoftCloseRequestedBy    *uuid.UUID `db:"soft_close_requested_by"`
	SoftCloseRequestedAt    *time.Time `db:"soft_close_requested_at"`
	SoftCloseRequestReason  *string    `db:"soft_close_request_reason"`
	SoftCloseApprovedBy     *uuid.UUID `db:"soft_close_approved_by"`
	SoftCloseApprovedAt     *time.Time `db:"soft_close_approved_at"`
	SoftCloseApproveReason  *string    `db:"soft_close_approve_reason"`
	HardCloseRequestedBy    *uuid.UUID `db:"hard_close_requested_by"`
	HardCloseRequestedAt    *time.Time `db:"hard_close_requested_at"`
	HardCloseRequestReason  *string    `db:"hard_close_request_reason"`
	HardCloseApprovedBy     *uuid.UUID `db:"hard_close_approved_by"`
	HardCloseApprovedAt     *time.Time `db:"hard_close_approved_at"`
	HardCloseApproveReason  *string    `db:"hard_close_approve_reason"`
	HardCloseGraceExpiresAt *time.Time `db:"hard_close_grace_expires_at"`
	StepUpTokenRef          *string    `db:"step_up_token_ref"`
	ReopenReason            *string    `db:"reopen_reason"`
	TenantID                string     `db:"tenant_id"`
	CreatedAt               time.Time  `db:"created_at"`
	UpdatedAt               time.Time  `db:"updated_at"`
}

// HasPendingSoftCloseRequest returns true if a soft-close request is pending approval.
func (p *PeriodeBuku) HasPendingSoftCloseRequest() bool {
	return p.SoftCloseRequestedBy != nil && p.StatusPeriode == PeriodeStatusOpen
}

// IsWithinGraceWindow returns true if reopen from CLOSED is still permitted.
func (p *PeriodeBuku) IsWithinGraceWindow() bool {
	if p.StatusPeriode != PeriodeStatusClosed {
		return false
	}
	if p.HardCloseGraceExpiresAt == nil {
		return false
	}
	return time.Now().Before(*p.HardCloseGraceExpiresAt)
}

// ─── Request/Response types ───────────────────────────────────────────────────

// SoftCloseRequestBody is the POST /soft-close-request request body.
type SoftCloseRequestBody struct {
	Catatan    *string `json:"catatan" binding:"omitempty,max=1000"`
	RowVersion int64   `json:"rowVersion" binding:"required,min=1"`
}

// WorkflowApproveBody is the shared approve body for soft-close, hard-close, reopen.
type WorkflowApproveBody struct {
	Comment         string `json:"comment" binding:"omitempty,max=1000"`
	SignatureMethod string `json:"signatureMethod" binding:"omitempty,oneof=JWT_STEP_UP JWT_STANDARD"`
}

// HardCloseRequestBody is the POST /hard-close-request request body.
type HardCloseRequestBody struct {
	Catatan    *string `json:"catatan" binding:"omitempty,max=1000"`
	RowVersion int64   `json:"rowVersion" binding:"required,min=1"`
}

// RejectBody is the POST /hard-close-reject request body.
type RejectBody struct {
	Reason string `json:"reason" binding:"required,min=30,max=1000"`
}

// ReopenRequestBody is the POST /reopen-request request body.
type ReopenRequestBody struct {
	TargetStatus PeriodeStatus `json:"targetStatus" binding:"required,oneof=OPEN SOFT_CLOSED"`
	Reason       string        `json:"reason" binding:"required,min=30,max=2000"`
	RowVersion   int64         `json:"rowVersion" binding:"required,min=1"`
}

// ─── Response types ───────────────────────────────────────────────────────────

// ChecklistEvalResult is the outcome of evaluating the 4-item checklist.
type ChecklistEvalResult struct {
	EvaluatedAt time.Time       `json:"evaluatedAt"`
	AllPassed   bool            `json:"allPassed"`
	Items       []ChecklistItem `json:"items"`
}

// SoftCloseRequestResponse is returned for 202 soft-close-request.
type SoftCloseRequestResponse struct {
	PeriodeID           uuid.UUID           `json:"periodeId"`
	PeriodeKode         string              `json:"periodeKode"`
	Transition          SnapshotTransition  `json:"transition"`
	ChecklistSnapshotID uuid.UUID           `json:"checklistSnapshotId"`
	Checklist           ChecklistEvalResult `json:"checklist"`
	AllPassed           bool                `json:"allPassed"`
	NextStep            string              `json:"nextStep"`
}

// SoftCloseApproveResponse is returned for 200 soft-close-approve.
type SoftCloseApproveResponse struct {
	PeriodeID           uuid.UUID     `json:"periodeId"`
	PeriodeKode         string        `json:"periodeKode"`
	StatusPeriode       PeriodeStatus `json:"statusPeriode"`
	TanggalSoftClose    time.Time     `json:"tanggalSoftClose"`
	ApprovedBy          uuid.UUID     `json:"approvedBy"`
	ChecklistSnapshotID uuid.UUID     `json:"checklistSnapshotId"`
	Message             string        `json:"message"`
}

// HardCloseRequestResponse is returned for 202 hard-close-request.
type HardCloseRequestResponse struct {
	PeriodeID           uuid.UUID           `json:"periodeId"`
	PeriodeKode         string              `json:"periodeKode"`
	Transition          SnapshotTransition  `json:"transition"`
	StatusPeriode       PeriodeStatus       `json:"statusPeriode"`
	ChecklistSnapshotID uuid.UUID           `json:"checklistSnapshotId"`
	Checklist           ChecklistEvalResult `json:"checklist"`
	NextStep            string              `json:"nextStep"`
}

// HardCloseApproveResponse is returned for 200 hard-close-approve.
type HardCloseApproveResponse struct {
	PeriodeID           uuid.UUID     `json:"periodeId"`
	PeriodeKode         string        `json:"periodeKode"`
	StatusPeriode       PeriodeStatus `json:"statusPeriode"`
	TanggalHardClose    time.Time     `json:"tanggalHardClose"`
	GraceExpiresAt      time.Time     `json:"graceExpiresAt"`
	ApprovedBy          uuid.UUID     `json:"approvedBy"`
	ChecklistSnapshotID uuid.UUID     `json:"checklistSnapshotId"`
	MvRefreshJobID      *string       `json:"mvRefreshJobId,omitempty"`
	MvRefreshStatusURL  *string       `json:"mvRefreshStatusUrl,omitempty"`
	Message             string        `json:"message"`
}

// PeriodeStateTransitionResponse is a generic state-transition response
// (used for hard-close-reject, reopen-approve from SOFT_CLOSED).
type PeriodeStateTransitionResponse struct {
	PeriodeID      uuid.UUID     `json:"periodeId"`
	PeriodeKode    string        `json:"periodeKode"`
	PreviousStatus PeriodeStatus `json:"previousStatus"`
	NewStatus      PeriodeStatus `json:"newStatus"`
	Transition     string        `json:"transition"`
	Reason         string        `json:"reason,omitempty"`
	ActorID        uuid.UUID     `json:"actorId"`
	TransitionedAt time.Time     `json:"transitionedAt"`
}

// ReopenRequestResponse is returned for 202 reopen-request.
type ReopenRequestResponse struct {
	PeriodeID           uuid.UUID     `json:"periodeId"`
	PeriodeKode         string        `json:"periodeKode"`
	CurrentStatus       PeriodeStatus `json:"currentStatus"`
	TargetStatus        PeriodeStatus `json:"targetStatus"`
	ChecklistSnapshotID uuid.UUID     `json:"checklistSnapshotId"`
	StepUpMFARequired   bool          `json:"stepUpMfaRequired"`
	NextStep            string        `json:"nextStep"`
}

// ReopenApproveResponse is returned for 200 reopen-approve.
type ReopenApproveResponse struct {
	PeriodeID      uuid.UUID     `json:"periodeId"`
	PeriodeKode    string        `json:"periodeKode"`
	PreviousStatus PeriodeStatus `json:"previousStatus"`
	NewStatus      PeriodeStatus `json:"newStatus"`
	ReopenedAt     time.Time     `json:"reopenedAt"`
	ReopenedBy     uuid.UUID     `json:"reopenedBy"`
	FXRateUnlocked bool          `json:"fxRateUnlocked"`
	Message        string        `json:"message"`
}

// LastSnapshotSummary is the summary of the most recent checklist snapshot.
type LastSnapshotSummary struct {
	SnapshotID  uuid.UUID          `json:"snapshotId"`
	Transition  SnapshotTransition `json:"transition"`
	EvaluatedAt time.Time          `json:"evaluatedAt"`
	AllPassed   bool               `json:"allPassed"`
}

// MVRefreshInfo is the MV refresh job status (for CLOSED period checklist response).
type MVRefreshInfo struct {
	JobID       string     `json:"jobId"`
	Status      string     `json:"status"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

// ClosingChecklistResponse is returned by GET /closing-checklist.
type ClosingChecklistResponse struct {
	PeriodeID      uuid.UUID            `json:"periodeId"`
	PeriodeKode    string               `json:"periodeKode"`
	StatusPeriode  PeriodeStatus        `json:"statusPeriode"`
	EvaluatedAt    time.Time            `json:"evaluatedAt"`
	AllPassed      bool                 `json:"allPassed"`
	IsRealTimeEval bool                 `json:"isRealTimeEval"`
	Items          []ChecklistItem      `json:"items"`
	LastSnapshot   *LastSnapshotSummary `json:"lastSnapshot,omitempty"`
	MVRefresh      *MVRefreshInfo       `json:"mvRefresh,omitempty"`
}

// StatusPeriodeListItem is one row in GET /reports/status-periode.
type StatusPeriodeListItem struct {
	PeriodeID             uuid.UUID            `json:"periodeId"`
	PeriodeKode           string               `json:"periodeKode"`
	TipePeriode           string               `json:"tipePeriode"`
	TahunBuku             int                  `json:"tahunBuku"`
	Bulan                 *int                 `json:"bulan,omitempty"`
	TanggalMulai          string               `json:"tanggalMulai"`
	TanggalAkhir          string               `json:"tanggalAkhir"`
	StatusPeriode         PeriodeStatus        `json:"statusPeriode"`
	TanggalSoftClose      *time.Time           `json:"tanggalSoftClose,omitempty"`
	TanggalHardClose      *time.Time           `json:"tanggalHardClose,omitempty"`
	SoftCloseBy           *uuid.UUID           `json:"softCloseBy,omitempty"`
	HardCloseBy           *uuid.UUID           `json:"hardCloseBy,omitempty"`
	ReopenedFlag          bool                 `json:"reopenedFlag"`
	ChecklistLastSnapshot *LastSnapshotSummary `json:"checklistLastSnapshot,omitempty"`
	MVRefreshStatus       *string              `json:"mvRefreshStatus,omitempty"`
	MVRefreshAt           *time.Time           `json:"mvRefreshAt,omitempty"`
}

// ─── Permission constants ─────────────────────────────────────────────────────

const (
	PermPeriodeSoftcloseRequest = "periode.softclose.request"
	PermPeriodeSoftcloseApprove = "periode.softclose.approve"
	PermPeriodeHardcloseRequest = "periode.hardclose.request"
	PermPeriodeHardcloseApprove = "periode.hardclose.approve"
	PermPeriodeReopenRequest    = "periode.reopen.request"
	PermPeriodeReopenApprove    = "periode.reopen.approve"
	PermPeriodeRead             = "periode.read"
	PermPeriodeExport           = "periode.export"
)

// ─── Config ───────────────────────────────────────────────────────────────────

// Config holds runtime close-workflow settings (from sys.config, seeded by migration 000038).
type Config struct {
	SoftCloseChecklistStaleHours int    // SOFT_CLOSE_CHECKLIST_STALE_HOURS default 24
	HardCloseGraceWindowHours    int    // HARD_CLOSE_GRACE_WINDOW_HOURS default 48
	SoftClosedMutationAllowlist  string // PERIODE_SOFT_CLOSED_MUTATION_ALLOWLIST
}

// DefaultConfig returns the migration 000038 seeded defaults.
func DefaultConfig() Config {
	return Config{
		SoftCloseChecklistStaleHours: 24,
		HardCloseGraceWindowHours:    48,
		SoftClosedMutationAllowlist:  "JURNAL_RETRY_GL_DELIVERY,CORRECTION_PERIODE_CLOSED",
	}
}

// ─── Pure state-machine transition function ───────────────────────────────────

// CanTransition validates whether a period state transition is allowed.
// Returns (allowed bool, error with domain code).
// Parameters:
//   - from: current status_periode
//   - action: transition action label (e.g. "soft-close-request")
//   - hasStepUp: true if X-Step-Up-Token is valid and fresh
//   - withinGrace: true if now() < hard_close_grace_expires_at (for reopen CLOSED)
//
// This function is pure (no DB/context side effects) — safe for unit testing.
func CanTransition(from PeriodeStatus, action string, hasStepUp bool, withinGrace bool) (bool, error) {
	switch action {
	case "soft-close-request":
		if from != PeriodeStatusOpen {
			return false, fmt.Errorf("periode status must be OPEN for soft-close-request, got %s", from)
		}
		return true, nil

	case "soft-close-approve":
		if from != PeriodeStatusOpen {
			return false, fmt.Errorf("periode status must be OPEN for soft-close-approve, got %s", from)
		}
		return true, nil

	case "hard-close-request":
		if from != PeriodeStatusSoftClosed {
			return false, fmt.Errorf("periode status must be SOFT_CLOSED for hard-close-request, got %s", from)
		}
		return true, nil

	case "hard-close-approve":
		if from != PeriodeStatusHardClosePending {
			return false, fmt.Errorf("periode status must be HARD_CLOSE_PENDING for hard-close-approve, got %s", from)
		}
		if !hasStepUp {
			return false, fmt.Errorf("step-up MFA required for hard-close-approve")
		}
		return true, nil

	case "hard-close-reject":
		if from != PeriodeStatusHardClosePending {
			return false, fmt.Errorf("periode status must be HARD_CLOSE_PENDING for hard-close-reject, got %s", from)
		}
		return true, nil

	case "reopen-soft-closed-to-open":
		if from != PeriodeStatusSoftClosed {
			return false, fmt.Errorf("periode status must be SOFT_CLOSED for reopen to OPEN, got %s", from)
		}
		return true, nil

	case "reopen-closed-to-soft-closed":
		if from != PeriodeStatusClosed {
			return false, fmt.Errorf("periode status must be CLOSED for reopen to SOFT_CLOSED, got %s", from)
		}
		if !withinGrace {
			return false, fmt.Errorf("grace window has expired for CLOSED → SOFT_CLOSED reopen")
		}
		if !hasStepUp {
			return false, fmt.Errorf("step-up MFA required for CLOSED → SOFT_CLOSED reopen")
		}
		return true, nil

	default:
		return false, fmt.Errorf("unknown action: %s", action)
	}
}
