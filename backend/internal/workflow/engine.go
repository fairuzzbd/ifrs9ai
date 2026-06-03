package workflow

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// validTransitions defines allowed (fromState, action) pairs and the target state
// for each eyes value. The approve action target is dynamic (see config.ApproveTarget()).
// All other transitions are static.
//
// The engine uses this table to reject invalid transitions with
// WORKFLOW_INVALID_TRANSITION (422) without any domain-specific if-else.
type transitionSpec struct {
	from   State
	action Action
	// toState is the destination. For APPROVE it is overridden by config.ApproveTarget().
	toState State
}

// baseTransitions are the same for both 4-eyes and 6-eyes.
var baseTransitions = []transitionSpec{
	{from: StateDraft, action: ActionSubmit, toState: StatePendingReview},
	{from: StatePendingReview, action: ActionReview, toState: StatePendingApproval},
	{from: StatePendingReview, action: ActionReject, toState: StateRejected},
	// PENDING_APPROVAL → APPROVE target is config-driven (4-eyes=APPROVED, 6-eyes=PENDING_APPROVAL_2)
	{from: StatePendingApproval, action: ActionApprove, toState: StateApproved}, // overridden for 6-eyes
	{from: StatePendingApproval, action: ActionReject, toState: StateRejected},
}

// sixEyesTransitions are additional transitions only for 6-eyes.
var sixEyesTransitions = []transitionSpec{
	{from: StatePendingApproval2, action: ActionApprove2, toState: StateApproved},
	{from: StatePendingApproval2, action: ActionReject, toState: StateRejected},
}

// retractTransition is only available when config.Retractable = true.
var retractTransition = transitionSpec{
	from: StatePendingReview, action: ActionRetract, toState: StateDraft,
}

// Engine is the generic config-driven state machine engine.
// It has NO knowledge of specific entities — all domain rules are encoded in Config.
type Engine struct {
	configs ConfigLoader
}

// NewEngine creates a new Engine.
func NewEngine(configs ConfigLoader) *Engine {
	return &Engine{configs: configs}
}

// TransitionInput contains everything the engine needs to compute a transition.
type TransitionInput struct {
	Instance        *Instance
	Action          Action
	CurrentUserID   string
	CurrentUsername string
	CurrentRole     string
	// StepUpFresh is true when X-Step-Up-Token header is valid and < 5 min old.
	StepUpFresh bool
	Request     ActionRequest
	// For reject: comment is required and validated before calling Transition.
	RejectComment *string
}

// TransitionResult is what the engine returns when a transition succeeds.
type TransitionResult struct {
	PreviousState   State
	NewState        State
	SignatureHash   string
	SignatureMethod SignatureMethod
	NextActions     []string
}

// Transition validates all guards and computes the new state.
// It does NOT write to DB — that is the responsibility of the service/repo layer.
//
// Order of checks (per workflow-state-machine.md §6.3):
//  1. Load + validate Config
//  2. Check entity not terminal (APPROVED/REJECTED)
//  3. Check valid transition for (currentState, action, eyes)
//  4. Check permission guard (caller must inject claims)
//  5. Check SoD guard
//  6. Check step-up MFA if required by config
//  7. Check optimistic lock (rowVersion)
//  8. Compute signature hash
func (e *Engine) Transition(input TransitionInput) (*TransitionResult, error) {
	inst := input.Instance

	// 1. Load config.
	cfg, err := e.configs.Load(inst.EntityType)
	if err != nil {
		return nil, fmt.Errorf("workflow engine: load config for %q: %w", inst.EntityType, err)
	}

	// 2. Terminal state guard.
	if inst.CurrentState.IsTerminal() {
		return nil, domainerrors.New(
			domainerrors.CodeWorkflowInvalidTransition,
			fmt.Sprintf("Tidak bisa melakukan aksi pada state terminal '%s'.", inst.CurrentState),
		)
	}

	// 3. Valid transition check.
	newState, err := computeTargetState(inst.CurrentState, input.Action, cfg)
	if err != nil {
		return nil, err
	}

	// 4. Permission guard (caller already checked via RequirePermission middleware, but
	//    the engine double-checks for service-layer safety).
	// RETRACT uses the same permission as UPDATE ({entity}.update or not in requiredPermissions).
	// Skip the permission guard for RETRACT — it is guarded by the retractable config flag
	// (only reachable if Retractable=true) and the caller must have passed handler permission check.
	permKey := strings.ToLower(string(input.Action))
	if input.Action != ActionRetract {
		requiredPerm := cfg.RequiredPermission(permKey)
		if requiredPerm == "" {
			return nil, domainerrors.New(
				domainerrors.CodeForbidden,
				fmt.Sprintf("Tidak ada permission yang dikonfigurasi untuk aksi '%s' pada entitas '%s'.",
					input.Action, inst.EntityType),
			)
		}
	}
	// Note: actual permission presence in JWT was already verified by auth middleware;
	// this guard is a secondary service-layer check.
	// Caller is responsible for verifying via auth.RequirePermission middleware.

	// 5. SoD guard.
	if err := checkSoD(inst, input.Action, input.CurrentUserID, cfg); err != nil {
		return nil, err
	}

	// 6. Step-up MFA guard.
	if cfg.NeedsStepUp(permKey) && !input.StepUpFresh {
		return nil, domainerrors.New(
			domainerrors.CodeStepUpRequired,
			fmt.Sprintf("Aksi '%s' untuk '%s' memerlukan step-up MFA. Lakukan POST /auth/step-up terlebih dahulu.",
				input.Action, inst.EntityType),
		)
	}

	// 7. Optimistic lock.
	if input.Request.RowVersion != nil && *input.Request.RowVersion != inst.RowVersion {
		return nil, domainerrors.ErrConflict()
	}

	// 8. Compute signature hash.
	// SHA-256(userId || action || entityId || signedAt || comment)
	comment := ""
	if input.RejectComment != nil {
		comment = *input.RejectComment
	} else if input.Request.Comment != nil {
		comment = *input.Request.Comment
	}
	signedAt := time.Now()
	sigHash := computeSignatureHash(
		input.CurrentUserID,
		string(input.Action),
		inst.EntityID.String(),
		signedAt.Format(time.RFC3339Nano),
		comment,
	)

	return &TransitionResult{
		PreviousState:   inst.CurrentState,
		NewState:        newState,
		SignatureHash:   sigHash,
		SignatureMethod: input.Request.SignatureMethod,
		NextActions:     nextActions(newState, cfg),
	}, nil
}

// computeTargetState resolves (currentState, action) → targetState using the
// config's eyes value to determine APPROVE branching. Returns
// WORKFLOW_INVALID_TRANSITION if the transition is not allowed.
func computeTargetState(current State, action Action, cfg *Config) (State, error) {
	// Build the allowed transitions for this config.
	specs := make([]transitionSpec, 0, len(baseTransitions)+len(sixEyesTransitions)+1)
	for _, t := range baseTransitions {
		s := t
		if s.action == ActionApprove {
			s.toState = cfg.ApproveTarget()
		}
		specs = append(specs, s)
	}
	if cfg.Eyes == 6 {
		specs = append(specs, sixEyesTransitions...)
	}
	if cfg.Retractable {
		specs = append(specs, retractTransition)
	}

	for _, spec := range specs {
		if spec.from == current && spec.action == action {
			return spec.toState, nil
		}
	}

	return "", domainerrors.New(
		domainerrors.CodeWorkflowInvalidTransition,
		fmt.Sprintf("Transisi '%s' dari state '%s' tidak valid untuk entitas '%s' (%d-eyes).",
			action, current, cfg.EntityType, cfg.Eyes),
		domainerrors.Detail{
			Field:   "state",
			Rule:    "invalid_transition",
			Message: fmt.Sprintf("Cannot perform %s from %s", action, current),
		},
	)
}

// checkSoD enforces Segregation of Duties per DEC-017.
// Rules are always enforced regardless of sodRules config flags — config is
// read only for approver2NotAnyPrevious.
func checkSoD(inst *Instance, action Action, currentUserID string, cfg *Config) error {
	switch action {
	case ActionReview:
		// reviewer ≠ maker (always enforced)
		if inst.MakerID.String() == currentUserID {
			return domainerrors.New(
				domainerrors.CodeSoDViolation,
				"Anda tidak bisa mereview entitas yang Anda buat sendiri (DEC-017).",
			)
		}

	case ActionApprove:
		// approver ≠ maker AND approver ≠ reviewer (always enforced)
		if inst.MakerID.String() == currentUserID {
			return domainerrors.New(
				domainerrors.CodeSoDViolation,
				"Anda tidak bisa meng-approve entitas yang Anda buat sendiri (DEC-017).",
			)
		}
		if inst.ReviewerID != nil && inst.ReviewerID.String() == currentUserID {
			return domainerrors.New(
				domainerrors.CodeSoDViolation,
				"Anda tidak bisa meng-approve entitas yang Anda review sendiri (DEC-017).",
			)
		}

	case ActionApprove2:
		// approver2 ≠ maker (always enforced)
		if inst.MakerID.String() == currentUserID {
			return domainerrors.New(
				domainerrors.CodeSoDApprover1SameAsMaker,
				"Approver2 tidak bisa sama dengan maker (6-eyes, DEC-017).",
			)
		}
		// approver2 ≠ reviewer (always enforced)
		if inst.ReviewerID != nil && inst.ReviewerID.String() == currentUserID {
			return domainerrors.New(
				domainerrors.CodeSoDApprover2SameAsReviewer,
				"Approver2 tidak bisa sama dengan reviewer (6-eyes, DEC-017).",
			)
		}
		// approver2 ≠ approver1 — enforced when config flag is set (Approver2NotAnyPrevious)
		if cfg.SoDRules.Approver2NotAnyPrevious && inst.Approver1ID != nil && inst.Approver1ID.String() == currentUserID {
			return domainerrors.New(
				domainerrors.CodeSoDViolation,
				"Approver2 tidak bisa sama dengan approver pertama (6-eyes, DEC-017).",
			)
		}

	case ActionReject:
		// Reject SoD depends on current state.
		switch inst.CurrentState {
		case StatePendingReview:
			if inst.MakerID.String() == currentUserID {
				return domainerrors.New(
					domainerrors.CodeSoDViolation,
					"Maker tidak bisa mereject entitas yang dibuat sendiri dari state PENDING_REVIEW.",
				)
			}
		case StatePendingApproval:
			if inst.MakerID.String() == currentUserID {
				return domainerrors.New(
					domainerrors.CodeSoDViolation,
					"Maker tidak bisa mereject entitas sendiri dari state PENDING_APPROVAL.",
				)
			}
			if inst.ReviewerID != nil && inst.ReviewerID.String() == currentUserID {
				return domainerrors.New(
					domainerrors.CodeSoDViolation,
					"Reviewer tidak bisa mereject entitas yang sudah melewati review dari PENDING_APPROVAL.",
				)
			}
		case StatePendingApproval2:
			if cfg.SoDRules.Approver2NotAnyPrevious {
				if inst.MakerID.String() == currentUserID ||
					(inst.ReviewerID != nil && inst.ReviewerID.String() == currentUserID) ||
					(inst.Approver1ID != nil && inst.Approver1ID.String() == currentUserID) {
					return domainerrors.New(
						domainerrors.CodeSoDViolation,
						"Reject dari PENDING_APPROVAL_2 hanya bisa dilakukan oleh user yang berbeda dari maker, reviewer, dan approver1.",
					)
				}
			}
		}
	}
	return nil
}

// nextActions returns the list of valid action names from the given state,
// empty if terminal.
func nextActions(state State, cfg *Config) []string {
	if state.IsTerminal() {
		return []string{}
	}
	specs := make([]transitionSpec, 0, len(baseTransitions)+len(sixEyesTransitions)+1)
	for _, t := range baseTransitions {
		s := t
		if s.action == ActionApprove {
			s.toState = cfg.ApproveTarget()
		}
		specs = append(specs, s)
	}
	if cfg.Eyes == 6 {
		specs = append(specs, sixEyesTransitions...)
	}
	if cfg.Retractable {
		specs = append(specs, retractTransition)
	}

	var actions []string
	for _, spec := range specs {
		if spec.from == state {
			actions = append(actions, strings.ToLower(string(spec.action)))
		}
	}
	return actions
}

// computeSignatureHash computes SHA-256(userId||action||entityId||signedAt||comment).
// This is the canonical formula per workflow-state-machine.md §7.
func computeSignatureHash(userID, action, entityID, signedAt, comment string) string {
	h := sha256.New()
	h.Write([]byte(userID))
	h.Write([]byte("|"))
	h.Write([]byte(action))
	h.Write([]byte("|"))
	h.Write([]byte(entityID))
	h.Write([]byte("|"))
	h.Write([]byte(signedAt))
	h.Write([]byte("|"))
	h.Write([]byte(comment))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// NewSignatureRecord builds a SignatureRecord to be persisted (append-only).
func NewSignatureRecord(
	workflowID uuid.UUID,
	action Action,
	userID uuid.UUID,
	username, roleAtTime, signedAt, sigHash, tenantID string,
	method SignatureMethod,
	comment *string,
) *SignatureRecord {
	t, err := time.Parse(time.RFC3339Nano, signedAt)
	if err != nil || t.IsZero() {
		t = time.Now()
	}
	return &SignatureRecord{
		ID:              uuid.New(),
		WorkflowID:      workflowID,
		Action:          action,
		UserID:          userID,
		Username:        username,
		RoleAtTime:      roleAtTime,
		SignedAt:        t,
		SignatureHash:   sigHash,
		SignatureMethod: method,
		Comment:         comment,
		TenantID:        tenantID,
	}
}
