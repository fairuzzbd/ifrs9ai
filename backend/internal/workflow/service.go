package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// Service is the workflow service layer. It owns the transaction boundary:
//   - loads the instance
//   - calls engine.Transition (pure business logic, no DB)
//   - updates instance state + inserts signature + writes audit — all in one tx
//
// All methods take context.Context and propagate trace/tenant/user per convention.
type Service struct {
	engine      *Engine
	repo        Repository
	auditWriter *audit.Writer
	logger      *slog.Logger
}

// NewService constructs a Service.
func NewService(engine *Engine, repo Repository, auditWriter *audit.Writer, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		engine:      engine,
		repo:        repo,
		auditWriter: auditWriter,
		logger:      logger,
	}
}

// SubmitInput is the request payload for Submit.
type SubmitInput struct {
	EntityType string
	EntityID   uuid.UUID
	Request    ActionRequest
	// StepUpFresh is set by handler after checking X-Step-Up-Token.
	StepUpFresh bool
}

// Submit transitions DRAFT → PENDING_REVIEW.
func (s *Service) Submit(ctx context.Context, in SubmitInput) (*ActionResult, error) {
	return s.performTransition(ctx, transitionParams{
		entityType:  in.EntityType,
		entityID:    in.EntityID,
		action:      ActionSubmit,
		request:     in.Request,
		stepUpFresh: in.StepUpFresh,
	})
}

// ReviewInput is the request payload for Review.
type ReviewInput struct {
	EntityType  string
	EntityID    uuid.UUID
	Request     ActionRequest
	StepUpFresh bool
}

// Review transitions PENDING_REVIEW → PENDING_APPROVAL.
func (s *Service) Review(ctx context.Context, in ReviewInput) (*ActionResult, error) {
	return s.performTransition(ctx, transitionParams{
		entityType:  in.EntityType,
		entityID:    in.EntityID,
		action:      ActionReview,
		request:     in.Request,
		stepUpFresh: in.StepUpFresh,
	})
}

// ApproveInput is the request payload for Approve.
type ApproveInput struct {
	EntityType  string
	EntityID    uuid.UUID
	Request     ActionRequest
	StepUpFresh bool
}

// Approve transitions PENDING_APPROVAL → APPROVED (4-eyes) or PENDING_APPROVAL_2 (6-eyes).
func (s *Service) Approve(ctx context.Context, in ApproveInput) (*ActionResult, error) {
	return s.performTransition(ctx, transitionParams{
		entityType:  in.EntityType,
		entityID:    in.EntityID,
		action:      ActionApprove,
		request:     in.Request,
		stepUpFresh: in.StepUpFresh,
	})
}

// Approve2Input is the request payload for Approve2 (6-eyes second approver).
type Approve2Input struct {
	EntityType  string
	EntityID    uuid.UUID
	Request     ActionRequest
	StepUpFresh bool
}

// Approve2 transitions PENDING_APPROVAL_2 → APPROVED (6-eyes only).
func (s *Service) Approve2(ctx context.Context, in Approve2Input) (*ActionResult, error) {
	return s.performTransition(ctx, transitionParams{
		entityType:  in.EntityType,
		entityID:    in.EntityID,
		action:      ActionApprove2,
		request:     in.Request,
		stepUpFresh: in.StepUpFresh,
	})
}

// RejectInput is the request payload for Reject.
type RejectInput struct {
	EntityType    string
	EntityID      uuid.UUID
	RejectRequest RejectRequest
	StepUpFresh   bool
}

// Reject transitions any PENDING_* state → REJECTED. Comment is mandatory.
func (s *Service) Reject(ctx context.Context, in RejectInput) (*ActionResult, error) {
	return s.performTransition(ctx, transitionParams{
		entityType:    in.EntityType,
		entityID:      in.EntityID,
		action:        ActionReject,
		request:       in.RejectRequest.toActionRequest(),
		rejectComment: &in.RejectRequest.Comment,
		stepUpFresh:   in.StepUpFresh,
	})
}

// GetStatus returns workflow status + signature history.
func (s *Service) GetStatus(ctx context.Context, entityType string, entityID uuid.UUID) (*StatusResponse, error) {
	inst, err := s.repo.GetByEntityID(ctx, entityType, entityID)
	if err != nil {
		return nil, fmt.Errorf("workflow service: get status: %w", err)
	}
	if inst == nil {
		return nil, domainerrors.ErrNotFound("workflow instance untuk entitas ini")
	}

	sigs, err := s.repo.ListSignatures(ctx, inst.ID)
	if err != nil {
		return nil, fmt.Errorf("workflow service: list signatures: %w", err)
	}

	return &StatusResponse{
		EntityID:     inst.EntityID,
		EntityType:   inst.EntityType,
		CurrentState: inst.CurrentState,
		WorkflowEyes: inst.Eyes,
		MakerID:      &inst.MakerID,
		ReviewerID:   inst.ReviewerID,
		Approver1ID:  inst.Approver1ID,
		Approver2ID:  inst.Approver2ID,
		History:      sigs,
	}, nil
}

// -----------------------------------------------------------------------
// Internal helpers
// -----------------------------------------------------------------------

type transitionParams struct {
	entityType    string
	entityID      uuid.UUID
	action        Action
	request       ActionRequest
	rejectComment *string
	stepUpFresh   bool
}

func (s *Service) performTransition(ctx context.Context, p transitionParams) (*ActionResult, error) {
	// Extract actor info from context JWT claims.
	claims := auth.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, domainerrors.New(domainerrors.CodeUnauthorized, "Claims tidak ada di context.")
	}

	currentUserID := claims.Sub
	username := claims.PreferredUsername
	roleAtTime := ""
	if len(claims.Roles) > 0 {
		roleAtTime = claims.Roles[0]
	}
	tenantID := claims.TenantID
	if tenantID == "" {
		tenantID = "TUGURE"
	}

	userUUID, err := uuid.Parse(currentUserID)
	if err != nil {
		return nil, fmt.Errorf("workflow service: parse user UUID: %w", err)
	}

	// Load the workflow instance.
	inst, err := s.repo.GetByEntityID(ctx, p.entityType, p.entityID)
	if err != nil {
		return nil, fmt.Errorf("workflow service: get instance: %w", err)
	}
	if inst == nil {
		return nil, domainerrors.ErrNotFound(fmt.Sprintf("workflow instance untuk %s/%s", p.entityType, p.entityID))
	}

	// Run engine transition (pure logic, no DB).
	engineInput := TransitionInput{
		Instance:        inst,
		Action:          p.action,
		CurrentUserID:   currentUserID,
		CurrentUsername: username,
		CurrentRole:     roleAtTime,
		StepUpFresh:     p.stepUpFresh,
		Request:         p.request,
		RejectComment:   p.rejectComment,
	}

	result, err := s.engine.Transition(engineInput)
	if err != nil {
		return nil, err // already a DomainError
	}

	// Build StateUpdate.
	now := time.Now()
	update := StateUpdate{
		WorkflowID: inst.ID,
		NewState:   result.NewState,
		UpdatedBy:  userUUID,
		RowVersion: inst.RowVersion,
	}
	s.applyActorToUpdate(&update, p.action, userUUID, p.rejectComment, inst.CurrentState, now)

	// Compute signature record.
	comment := p.rejectComment
	if comment == nil {
		comment = p.request.Comment
	}
	sigRecord := &SignatureRecord{
		ID:              uuid.New(),
		WorkflowID:      inst.ID,
		Action:          p.action,
		UserID:          userUUID,
		Username:        username,
		RoleAtTime:      roleAtTime,
		SignedAt:        now,
		SignatureHash:   result.SignatureHash,
		SignatureMethod: result.SignatureMethod,
		Comment:         comment,
		TenantID:        tenantID,
	}

	// Execute everything in one transaction.
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("workflow service: begin tx: %w", err)
	}

	// For InMemoryRepository, tx is nil — that's fine; UpdateState/InsertSignature accept nil tx.
	defer func() {
		if tx != nil && err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				slog.Default().ErrorContext(ctx, "workflow service: tx rollback failed", "error", rbErr)
			}
		}
	}()

	if err = s.repo.UpdateState(ctx, tx, update); err != nil {
		return nil, fmt.Errorf("workflow service: update state: %w", err)
	}

	if err = s.repo.InsertSignature(ctx, tx, sigRecord); err != nil {
		return nil, fmt.Errorf("workflow service: insert signature: %w", err)
	}

	// Write audit log in the same transaction.
	// If auditWriter is nil (tests without DB), skip.
	if s.auditWriter != nil && tx != nil {
		txWriter := s.auditWriter.WithTx(tx)
		entityID := inst.EntityID
		auditAction := fmt.Sprintf("%s.%s", strings.ToUpper(p.entityType), strings.ToUpper(string(p.action)))
		auditErr := txWriter.Write(ctx, audit.Event{
			Action:     auditAction,
			EntityType: fmt.Sprintf("sys.workflow_instance[%s]", p.entityType),
			EntityID:   entityID,
			Before:     map[string]any{"state": string(inst.CurrentState), "row_version": inst.RowVersion},
			After:      map[string]any{"state": string(result.NewState), "row_version": inst.RowVersion + 1},
		})
		if auditErr != nil {
			s.logger.WarnContext(ctx, "workflow audit write failed",
				"error", auditErr,
				"entityType", p.entityType,
				"entityID", p.entityID,
				"action", p.action,
			)
			// Non-fatal per audit.writer.go comment, but we log it.
		}
	}

	if tx != nil {
		if err = tx.Commit(); err != nil {
			return nil, fmt.Errorf("workflow service: commit: %w", err)
		}
	}

	return &ActionResult{
		EntityID:        inst.EntityID,
		EntityType:      inst.EntityType,
		PreviousState:   result.PreviousState,
		CurrentState:    result.NewState,
		Action:          p.action,
		PerformedBy:     username,
		PerformedAt:     now,
		SignatureHash:   result.SignatureHash,
		SignatureMethod: result.SignatureMethod,
		NextActions:     result.NextActions,
		WorkflowEyes:    inst.Eyes,
	}, nil
}

// applyActorToUpdate sets the actor + timestamp fields on StateUpdate based on action.
func (s *Service) applyActorToUpdate(u *StateUpdate, action Action, userID uuid.UUID, rejectComment *string, fromState State, now time.Time) {
	switch action {
	case ActionSubmit:
		u.SubmittedAt = &now
	case ActionReview:
		u.ReviewerID = &userID
		u.ReviewedAt = &now
	case ActionApprove:
		u.Approver1ID = &userID
		u.Approved1At = &now
	case ActionApprove2:
		u.Approver2ID = &userID
		u.Approved2At = &now
	case ActionReject:
		u.RejectedBy = &userID
		u.RejectedAt = &now
		u.RejectComment = rejectComment
		step := string(fromState)
		u.RejectStep = &step
	case ActionRetract:
		// No actor field changes; submitted_at is already set and immutable.
	}
}

// toActionRequest converts a RejectRequest to an ActionRequest for engine use.
func (r *RejectRequest) toActionRequest() ActionRequest {
	return ActionRequest{
		Comment:         &r.Comment,
		SignatureMethod: r.SignatureMethod,
		RowVersion:      r.RowVersion,
	}
}
