// Package workflow implements the generic config-driven Maker-Reviewer-Approver
// state machine per workflow-state-machine.md §1-5.
//
// Design contract:
//   - Engine reads Config from sys.config (no if-else per entity in engine code).
//   - 4-eyes: DRAFT→PENDING_REVIEW→PENDING_APPROVAL→APPROVED|REJECTED.
//   - 6-eyes: DRAFT→PENDING_REVIEW→PENDING_APPROVAL→PENDING_APPROVAL_2→APPROVED|REJECTED.
//   - SoD: maker≠reviewer≠approver≠approver2 (DEC-017). Enforced by service layer.
//   - Optimistic lock: rowVersion in request body, matched against row_version in DB.
//   - Signature immutability: sys.workflow_signature rows are append-only (DB trigger).
//   - Audit: every transition writes to aud.audit_log in the SAME transaction.
package workflow

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// State adalah workflow state enum — tepat 6 nilai valid.
type State string

const (
	StateDraft            State = "DRAFT"
	StatePendingReview    State = "PENDING_REVIEW"
	StatePendingApproval  State = "PENDING_APPROVAL"
	StatePendingApproval2 State = "PENDING_APPROVAL_2"
	StateApproved         State = "APPROVED"
	StateRejected         State = "REJECTED"
)

// IsTerminal returns true jika state sudah final (tidak bisa transisi).
func (s State) IsTerminal() bool {
	return s == StateApproved || s == StateRejected
}

// IsPending returns true jika state adalah salah satu PENDING_*.
func (s State) IsPending() bool {
	switch s {
	case StatePendingReview, StatePendingApproval, StatePendingApproval2:
		return true
	}
	return false
}

// Action adalah workflow action yang memicu transisi.
type Action string

const (
	ActionSubmit   Action = "SUBMIT"
	ActionReview   Action = "REVIEW"
	ActionApprove  Action = "APPROVE"
	ActionApprove2 Action = "APPROVE2"
	ActionReject   Action = "REJECT"
	ActionRetract  Action = "RETRACT"
)

// SignatureMethod adalah metode signing sesuai OpenAPI WorkflowActionRequest.
type SignatureMethod string

const (
	SignatureMethodJWTStandard SignatureMethod = "JWT_STANDARD"
	SignatureMethodJWTStepUp   SignatureMethod = "JWT_STEP_UP"
)

// Instance adalah runtime representation dari sys.workflow_instance row.
type Instance struct {
	ID                uuid.UUID
	EntityType        string
	EntityID          uuid.UUID
	EntitySchema      string
	WorkflowConfigKey string
	Eyes              int
	CurrentState      State
	MakerID           uuid.UUID
	ReviewerID        *uuid.UUID
	Approver1ID       *uuid.UUID
	Approver2ID       *uuid.UUID // 6-eyes only
	RejectedBy        *uuid.UUID
	SubmittedAt       *time.Time
	ReviewedAt        *time.Time
	Approved1At       *time.Time
	Approved2At       *time.Time
	RejectedAt        *time.Time
	RejectComment     *string
	RejectStep        *string
	CreatedAt         time.Time
	CreatedBy         uuid.UUID
	UpdatedAt         time.Time
	UpdatedBy         uuid.UUID
	RowVersion        int64
	TenantID          string
}

// Participants builds Participants for SoD checks.
func (i *Instance) Participants() Participants {
	p := Participants{
		MakerID: i.MakerID.String(),
	}
	if i.ReviewerID != nil {
		p.ReviewerID = i.ReviewerID.String()
	}
	if i.Approver1ID != nil {
		p.ApproverID = i.Approver1ID.String()
	}
	if i.Approver2ID != nil {
		p.Approver2ID = i.Approver2ID.String()
	}
	return p
}

// Participants mirrors auth.Participants — defined here to avoid
// circular import between workflow ↔ auth. Converter fills it from Instance.
type Participants struct {
	MakerID     string
	ReviewerID  string
	ApproverID  string
	Approver2ID string
}

// SignatureRecord is a single row from sys.workflow_signature.
type SignatureRecord struct {
	ID              uuid.UUID
	WorkflowID      uuid.UUID
	Action          Action
	UserID          uuid.UUID
	Username        string
	RoleAtTime      string
	SignedAt        time.Time
	SignatureHash   string
	SignatureMethod SignatureMethod
	Comment         *string
	TenantID        string
}

// ActionRequest is the parsed + validated request body for all workflow actions.
type ActionRequest struct {
	Comment         *string
	SignatureMethod SignatureMethod
	RowVersion      *int64
}

// RejectRequest adds the mandatory comment constraint.
type RejectRequest struct {
	Comment         string // minLength 10, required
	SignatureMethod SignatureMethod
	RowVersion      *int64
}

// ActionResult is the data portion of a WorkflowActionResponse.
type ActionResult struct {
	EntityID        uuid.UUID
	EntityType      string
	PreviousState   State
	CurrentState    State
	Action          Action
	PerformedBy     string // preferred_username
	PerformedAt     time.Time
	SignatureHash   string
	SignatureMethod SignatureMethod
	NextActions     []string
	WorkflowEyes    int
}

// StatusResponse is the data portion of a WorkflowStatusResponse.
type StatusResponse struct {
	EntityID     uuid.UUID
	EntityType   string
	CurrentState State
	WorkflowEyes int
	MakerID      *uuid.UUID
	ReviewerID   *uuid.UUID
	Approver1ID  *uuid.UUID
	Approver2ID  *uuid.UUID
	History      []SignatureRecord
}

// EntityHook is called by the workflow service after each successful state transition.
// Modules register an implementation per entityType so the generic workflow service can
// call back into the domain layer to sync workflow_status columns without knowing the
// module internals.
//
// Implementations must be idempotent — the same (entityID, newState, action) may be
// replayed in error-recovery scenarios.
type EntityHook interface {
	// OnTransition is called after the workflow instance state has been committed.
	// entityType: upper snake_case entity type (e.g. "MAPPING_JURNAL")
	// entityID: UUID of the domain entity (e.g. mst.mapping_jurnal_header.id)
	// newState: new workflow state string (e.g. "APPROVED")
	// action: action that caused the transition (e.g. "APPROVE")
	OnTransition(ctx context.Context, entityType string, entityID uuid.UUID, newState string, action string) error
}
