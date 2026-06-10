package workflow

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// -----------------------------------------------------------------------
// Service integration (in-memory, no DB required)
// -----------------------------------------------------------------------

// buildTestService builds a Service with in-memory components.
// Integration tests that require a real DB should be in service_integration_test.go
// and are tagged `//go:build integration`.
func buildTestService(configs map[string]*Config) (*Service, *InMemoryRepository) {
	loader := NewInMemoryConfigLoader(configs)
	engine := NewEngine(loader)
	repo := NewInMemoryRepository()
	// auditWriter nil — tests don't have a DB
	svc := NewService(engine, repo, nil, nil)
	return svc, repo
}

// ctxWithClaims builds a context with JWT claims for the given user.
func ctxWithClaims(userID uuid.UUID, username string, permissions ...string) context.Context {
	perms := append([]string{}, permissions...)
	claims := &auth.Claims{
		Sub:               userID.String(),
		PreferredUsername: username,
		Roles:             []string{"ROLE-MAKER-TR"},
		Permissions:       perms,
		TenantID:          "TUGURE",
		MFAVerified:       true,
	}
	return auth.ContextWithClaims(context.Background(), claims)
}

func seedWorkflow(repo *InMemoryRepository, cfg *Config, makerUUID uuid.UUID) *Instance {
	inst := &Instance{
		ID:                uuid.New(),
		EntityType:        cfg.EntityType,
		EntityID:          uuid.New(),
		EntitySchema:      "test",
		WorkflowConfigKey: configKey(cfg.EntityType),
		Eyes:              cfg.Eyes,
		CurrentState:      StateDraft,
		MakerID:           makerUUID,
		RowVersion:        1,
		CreatedBy:         makerUUID,
		TenantID:          "TUGURE",
	}
	repo.Seed(inst)
	return inst
}

// -----------------------------------------------------------------------
// 4-Eyes full workflow
// -----------------------------------------------------------------------

func TestService_FourEyes_FullWorkflow(t *testing.T) {
	cfg := cfg4Eyes(false)
	svc, repo := buildTestService(map[string]*Config{cfg.EntityType: cfg})

	mUUID := uuid.New()
	rUUID := uuid.New()
	aUUID := uuid.New()

	inst := seedWorkflow(repo, cfg, mUUID)

	// Submit
	ctxMaker := ctxWithClaims(mUUID, "maker", "penempatan.submit")
	result, err := svc.Submit(ctxMaker, SubmitInput{
		EntityType:  cfg.EntityType,
		EntityID:    inst.EntityID,
		Request:     defaultActionRequest(),
		StepUpFresh: false,
	})
	mustNoError(t, "submit", err)
	if result.CurrentState != StatePendingReview {
		t.Errorf("after submit: got %s, want PENDING_REVIEW", result.CurrentState)
	}

	// Update in-memory state for the next step
	updated, _ := repo.GetByEntityID(context.Background(), cfg.EntityType, inst.EntityID)

	// Review
	ctxReviewer := ctxWithClaims(rUUID, "reviewer", "penempatan.review")
	result, err = svc.Review(ctxReviewer, ReviewInput{
		EntityType:  cfg.EntityType,
		EntityID:    updated.EntityID,
		Request:     defaultActionRequest(),
		StepUpFresh: false,
	})
	mustNoError(t, "review", err)
	if result.CurrentState != StatePendingApproval {
		t.Errorf("after review: got %s, want PENDING_APPROVAL", result.CurrentState)
	}

	updated, _ = repo.GetByEntityID(context.Background(), cfg.EntityType, inst.EntityID)

	// Approve
	ctxApprover := ctxWithClaims(aUUID, "approver", "penempatan.approve")
	result, err = svc.Approve(ctxApprover, ApproveInput{
		EntityType:  cfg.EntityType,
		EntityID:    updated.EntityID,
		Request:     defaultActionRequest(),
		StepUpFresh: false,
	})
	mustNoError(t, "approve", err)
	if result.CurrentState != StateApproved {
		t.Errorf("after approve: got %s, want APPROVED", result.CurrentState)
	}

	// Verify signature records
	sigs, err := repo.ListSignatures(context.Background(), inst.ID)
	mustNoError(t, "list signatures", err)
	if len(sigs) != 3 {
		t.Errorf("expected 3 signatures (submit/review/approve), got %d", len(sigs))
	}
}

// -----------------------------------------------------------------------
// 6-Eyes full workflow
// -----------------------------------------------------------------------

func TestService_SixEyes_FullWorkflow(t *testing.T) {
	cfg := cfg6Eyes()
	svc, repo := buildTestService(map[string]*Config{cfg.EntityType: cfg})

	mUUID := uuid.New()
	rUUID := uuid.New()
	a1UUID := uuid.New()
	a2UUID := uuid.New()

	inst := seedWorkflow(repo, cfg, mUUID)

	// Submit
	ctx1 := ctxWithClaims(mUUID, "maker", "klasifikasi.submit")
	r, err := svc.Submit(ctx1, SubmitInput{EntityType: cfg.EntityType, EntityID: inst.EntityID, Request: defaultActionRequest()})
	mustNoError(t, "submit", err)
	if r.CurrentState != StatePendingReview {
		t.Fatalf("expected PENDING_REVIEW")
	}

	// Review
	ctx2 := ctxWithClaims(rUUID, "reviewer", "klasifikasi.review")
	r, err = svc.Review(ctx2, ReviewInput{EntityType: cfg.EntityType, EntityID: inst.EntityID, Request: defaultActionRequest()})
	mustNoError(t, "review", err)
	if r.CurrentState != StatePendingApproval {
		t.Fatalf("expected PENDING_APPROVAL")
	}

	// Approve (6-eyes → PENDING_APPROVAL_2)
	ctx3 := ctxWithClaims(a1UUID, "approver1", "klasifikasi.approve")
	r, err = svc.Approve(ctx3, ApproveInput{EntityType: cfg.EntityType, EntityID: inst.EntityID, Request: defaultActionRequest()})
	mustNoError(t, "approve", err)
	if r.CurrentState != StatePendingApproval2 {
		t.Fatalf("expected PENDING_APPROVAL_2, got %s", r.CurrentState)
	}
	if r.WorkflowEyes != 6 {
		t.Errorf("expected workflowEyes=6, got %d", r.WorkflowEyes)
	}

	// Approve2 (6-eyes → APPROVED)
	ctx4 := ctxWithClaims(a2UUID, "approver2", "klasifikasi.approve")
	// approve2 requires step-up MFA — StepUpFresh=true
	r, err = svc.Approve2(ctx4, Approve2Input{EntityType: cfg.EntityType, EntityID: inst.EntityID, Request: defaultActionRequest(), StepUpFresh: true})
	mustNoError(t, "approve2", err)
	if r.CurrentState != StateApproved {
		t.Errorf("after approve2: got %s, want APPROVED", r.CurrentState)
	}

	sigs, _ := repo.ListSignatures(context.Background(), inst.ID)
	if len(sigs) != 4 {
		t.Errorf("expected 4 signatures, got %d", len(sigs))
	}
}

// -----------------------------------------------------------------------
// SoD — service layer enforcement (integration test via API direct calls)
// -----------------------------------------------------------------------

// TestService_SoD_MakerTriesToReview — QA integration test scenario
// "Maker mencoba jadi Reviewer via API langsung" → SOD_VIOLATION 403.
func TestService_SoD_MakerTriesToReview(t *testing.T) {
	cfg := cfg4Eyes(false)
	svc, repo := buildTestService(map[string]*Config{cfg.EntityType: cfg})

	mUUID := uuid.New()
	inst := seedWorkflow(repo, cfg, mUUID)

	// Maker submits
	ctx1 := ctxWithClaims(mUUID, "maker", "penempatan.submit", "penempatan.review")
	_, err := svc.Submit(ctx1, SubmitInput{EntityType: cfg.EntityType, EntityID: inst.EntityID, Request: defaultActionRequest()})
	mustNoError(t, "submit", err)

	// Same maker tries to review — SOD_VIOLATION
	_, err = svc.Review(ctx1, ReviewInput{EntityType: cfg.EntityType, EntityID: inst.EntityID, Request: defaultActionRequest()})
	assertDomainCode(t, err, domainerrors.CodeSoDViolation)
}

// TestService_SoD_MakerTriesToApprove — second QA scenario.
func TestService_SoD_MakerTriesToApprove(t *testing.T) {
	cfg := cfg4Eyes(false)
	svc, repo := buildTestService(map[string]*Config{cfg.EntityType: cfg})

	mUUID := uuid.New()
	rUUID := uuid.New()
	inst := seedWorkflow(repo, cfg, mUUID)

	ctx1 := ctxWithClaims(mUUID, "maker", "penempatan.submit", "penempatan.approve")
	ctx2 := ctxWithClaims(rUUID, "reviewer", "penempatan.review")

	_, err := svc.Submit(ctx1, SubmitInput{EntityType: cfg.EntityType, EntityID: inst.EntityID, Request: defaultActionRequest()})
	mustNoError(t, "submit", err)
	_, err = svc.Review(ctx2, ReviewInput{EntityType: cfg.EntityType, EntityID: inst.EntityID, Request: defaultActionRequest()})
	mustNoError(t, "review", err)

	// Same maker tries to approve — SOD_VIOLATION
	_, err = svc.Approve(ctx1, ApproveInput{EntityType: cfg.EntityType, EntityID: inst.EntityID, Request: defaultActionRequest()})
	assertDomainCode(t, err, domainerrors.CodeSoDViolation)
}

// -----------------------------------------------------------------------
// Reject workflow
// -----------------------------------------------------------------------

func TestService_Reject_FromPendingReview(t *testing.T) {
	cfg := cfg4Eyes(false)
	svc, repo := buildTestService(map[string]*Config{cfg.EntityType: cfg})

	mUUID := uuid.New()
	rUUID := uuid.New()
	inst := seedWorkflow(repo, cfg, mUUID)

	ctx1 := ctxWithClaims(mUUID, "maker", "penempatan.submit")
	ctx2 := ctxWithClaims(rUUID, "reviewer", "penempatan.reject")

	_, err := svc.Submit(ctx1, SubmitInput{EntityType: cfg.EntityType, EntityID: inst.EntityID, Request: defaultActionRequest()})
	mustNoError(t, "submit", err)

	r, err := svc.Reject(ctx2, RejectInput{
		EntityType: cfg.EntityType, EntityID: inst.EntityID,
		RejectRequest: RejectRequest{
			Comment:         "Data tidak lengkap, harap dilengkapi sesuai prosedur",
			SignatureMethod: SignatureMethodJWTStandard,
		},
	})
	mustNoError(t, "reject", err)
	if r.CurrentState != StateRejected {
		t.Errorf("expected REJECTED, got %s", r.CurrentState)
	}
}

func TestService_Reject_CommentTooShort(t *testing.T) {
	cfg := cfg4Eyes(false)
	svc, repo := buildTestService(map[string]*Config{cfg.EntityType: cfg})

	mUUID := uuid.New()
	rUUID := uuid.New()
	inst := seedWorkflow(repo, cfg, mUUID)

	ctx1 := ctxWithClaims(mUUID, "maker", "penempatan.submit")
	ctx2 := ctxWithClaims(rUUID, "reviewer", "penempatan.reject")

	_, _ = svc.Submit(ctx1, SubmitInput{EntityType: cfg.EntityType, EntityID: inst.EntityID, Request: defaultActionRequest()})

	// RejectRequest.Comment is validated at handler layer (minLength 10).
	// Here we test that the service correctly passes through — the handler
	// validates before calling Reject. But let's confirm empty comment still works
	// through service (handler is responsible for validation).
	// Actually let's test that a short comment is passed through:
	r, err := svc.Reject(ctx2, RejectInput{
		EntityType: cfg.EntityType, EntityID: inst.EntityID,
		RejectRequest: RejectRequest{
			Comment:         "short", // 5 chars — handler should reject before this
			SignatureMethod: SignatureMethodJWTStandard,
		},
	})
	// Service itself doesn't enforce minLength 10 — that's handler responsibility.
	// So this should succeed at service layer.
	if err != nil {
		t.Logf("service rejected short comment (acceptable): %v", err)
	} else if r.CurrentState != StateRejected {
		t.Errorf("expected REJECTED, got %s", r.CurrentState)
	}
}

// -----------------------------------------------------------------------
// Optimistic lock via service
// -----------------------------------------------------------------------

func TestService_OptimisticLock(t *testing.T) {
	cfg := cfg4Eyes(false)
	svc, repo := buildTestService(map[string]*Config{cfg.EntityType: cfg})

	mUUID := uuid.New()
	inst := seedWorkflow(repo, cfg, mUUID) // RowVersion = 1

	ctx := ctxWithClaims(mUUID, "maker", "penempatan.submit")

	wrongRV := int64(999)
	req := ActionRequest{
		SignatureMethod: SignatureMethodJWTStandard,
		RowVersion:      &wrongRV,
	}
	_, err := svc.Submit(ctx, SubmitInput{
		EntityType: cfg.EntityType, EntityID: inst.EntityID, Request: req,
	})
	assertDomainCode(t, err, domainerrors.CodeConflict)
}

// -----------------------------------------------------------------------
// Entity not found
// -----------------------------------------------------------------------

func TestService_EntityNotFound(t *testing.T) {
	cfg := cfg4Eyes(false)
	svc, _ := buildTestService(map[string]*Config{cfg.EntityType: cfg})

	ctx := ctxWithClaims(uuid.New(), "maker", "penempatan.submit")
	_, err := svc.Submit(ctx, SubmitInput{
		EntityType: cfg.EntityType,
		EntityID:   uuid.New(), // non-existent
		Request:    defaultActionRequest(),
	})
	assertDomainCode(t, err, domainerrors.CodeNotFound)
}

// -----------------------------------------------------------------------
// Signature immutability — signatures never get removed from in-memory store
// -----------------------------------------------------------------------

func TestService_SignatureImmutability(t *testing.T) {
	cfg := cfg4Eyes(false)
	svc, repo := buildTestService(map[string]*Config{cfg.EntityType: cfg})

	mUUID := uuid.New()
	rUUID := uuid.New()
	inst := seedWorkflow(repo, cfg, mUUID)

	ctx1 := ctxWithClaims(mUUID, "maker", "penempatan.submit")
	ctx2 := ctxWithClaims(rUUID, "reviewer", "penempatan.reject")

	_, _ = svc.Submit(ctx1, SubmitInput{EntityType: cfg.EntityType, EntityID: inst.EntityID, Request: defaultActionRequest()})

	sigsBeforeReject, _ := repo.ListSignatures(context.Background(), inst.ID)
	if len(sigsBeforeReject) != 1 {
		t.Fatalf("expected 1 sig after submit, got %d", len(sigsBeforeReject))
	}

	// Reject
	_, _ = svc.Reject(ctx2, RejectInput{
		EntityType: cfg.EntityType, EntityID: inst.EntityID,
		RejectRequest: RejectRequest{Comment: "needs correction please resubmit", SignatureMethod: SignatureMethodJWTStandard},
	})

	sigsAfterReject, _ := repo.ListSignatures(context.Background(), inst.ID)
	if len(sigsAfterReject) != 2 {
		t.Errorf("expected 2 sigs after reject, got %d", len(sigsAfterReject))
	}
	// First signature must still exist unchanged (immutability)
	if sigsAfterReject[0].Action != ActionSubmit {
		t.Errorf("first sig action: got %s, want SUBMIT", sigsAfterReject[0].Action)
	}
}

// -----------------------------------------------------------------------
// GetStatus
// -----------------------------------------------------------------------

func TestService_GetStatus(t *testing.T) {
	cfg := cfg4Eyes(false)
	svc, repo := buildTestService(map[string]*Config{cfg.EntityType: cfg})

	mUUID := uuid.New()
	inst := seedWorkflow(repo, cfg, mUUID)

	ctx := ctxWithClaims(mUUID, "maker", "penempatan.read")
	status, err := svc.GetStatus(ctx, cfg.EntityType, inst.EntityID)
	mustNoError(t, "get status", err)

	if status.CurrentState != StateDraft {
		t.Errorf("expected DRAFT, got %s", status.CurrentState)
	}
	if status.WorkflowEyes != 4 {
		t.Errorf("expected 4-eyes, got %d", status.WorkflowEyes)
	}
}

// -----------------------------------------------------------------------
// No claims in context → UNAUTHORIZED
// -----------------------------------------------------------------------

func TestService_NoClaims_Unauthorized(t *testing.T) {
	cfg := cfg4Eyes(false)
	svc, repo := buildTestService(map[string]*Config{cfg.EntityType: cfg})

	inst := seedWorkflow(repo, cfg, uuid.New())

	// Context without claims
	_, err := svc.Submit(context.Background(), SubmitInput{
		EntityType: cfg.EntityType,
		EntityID:   inst.EntityID,
		Request:    defaultActionRequest(),
	})
	assertDomainCode(t, err, domainerrors.CodeUnauthorized)
}

// -----------------------------------------------------------------------
// EntityHook — BeforeCommit called in-transaction and error propagated
// -----------------------------------------------------------------------

// stubHook is a test EntityHook that records calls and returns a configurable error.
type stubHook struct {
	called    int
	returnErr error
}

func (h *stubHook) BeforeCommit(_ context.Context, _ *sql.Tx, _ HookEvent) error {
	h.called++
	return h.returnErr
}

// TestService_EntityHook_CalledOnTransition verifies that a registered hook is invoked
// during performTransition.
func TestService_EntityHook_CalledOnTransition(t *testing.T) {
	cfg := cfg4Eyes(false)
	svc, repo := buildTestService(map[string]*Config{cfg.EntityType: cfg})

	hook := &stubHook{}
	svc.RegisterEntityHook(cfg.EntityType, hook)

	mUUID := uuid.New()
	inst := seedWorkflow(repo, cfg, mUUID)

	ctx := ctxWithClaims(mUUID, "maker", "penempatan.submit")
	_, err := svc.Submit(ctx, SubmitInput{
		EntityType: cfg.EntityType,
		EntityID:   inst.EntityID,
		Request:    defaultActionRequest(),
	})
	mustNoError(t, "submit", err)

	if hook.called != 1 {
		t.Errorf("expected hook to be called once, got %d", hook.called)
	}
}

// TestService_EntityHook_ErrorRollsBack verifies that a hook error aborts the transition
// and the error is propagated to the caller.
func TestService_EntityHook_ErrorRollsBack(t *testing.T) {
	cfg := cfg4Eyes(false)
	svc, repo := buildTestService(map[string]*Config{cfg.EntityType: cfg})

	hookErr := domainerrors.New(domainerrors.CodeBobotSumInvariantViolated, "sum invariant check failed")
	hook := &stubHook{returnErr: hookErr}
	svc.RegisterEntityHook(cfg.EntityType, hook)

	mUUID := uuid.New()
	inst := seedWorkflow(repo, cfg, mUUID)

	ctx := ctxWithClaims(mUUID, "maker", "penempatan.submit")
	_, err := svc.Submit(ctx, SubmitInput{
		EntityType: cfg.EntityType,
		EntityID:   inst.EntityID,
		Request:    defaultActionRequest(),
	})

	if err == nil {
		t.Fatal("expected error from hook, got nil")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError, got %T: %v", err, err)
	}
	if de.Code() != domainerrors.CodeBobotSumInvariantViolated {
		t.Errorf("expected BOBOT_SUM_INVARIANT_VIOLATED, got %s", de.Code())
	}
	if hook.called != 1 {
		t.Errorf("expected hook called once, got %d", hook.called)
	}
}

// TestService_EntityHook_MultipleHooks verifies multiple hooks are called in registration order.
func TestService_EntityHook_MultipleHooks(t *testing.T) {
	cfg := cfg4Eyes(false)
	svc, repo := buildTestService(map[string]*Config{cfg.EntityType: cfg})

	var callOrder []int
	makeOrderHook := func(id int) EntityHook {
		return &orderTrackingHook{id: id, order: &callOrder}
	}

	svc.RegisterEntityHook(cfg.EntityType, makeOrderHook(1))
	svc.RegisterEntityHook(cfg.EntityType, makeOrderHook(2))
	svc.RegisterEntityHook(cfg.EntityType, makeOrderHook(3))

	mUUID := uuid.New()
	inst := seedWorkflow(repo, cfg, mUUID)

	ctx := ctxWithClaims(mUUID, "maker", "penempatan.submit")
	_, err := svc.Submit(ctx, SubmitInput{
		EntityType: cfg.EntityType,
		EntityID:   inst.EntityID,
		Request:    defaultActionRequest(),
	})
	mustNoError(t, "submit", err)

	if len(callOrder) != 3 {
		t.Fatalf("expected 3 hooks called, got %d", len(callOrder))
	}
	for i, v := range callOrder {
		if v != i+1 {
			t.Errorf("hook call order[%d] = %d, want %d", i, v, i+1)
		}
	}
}

// TestService_EntityHook_UnregisteredEntityType verifies hooks for other entity types
// are not called for a different entity.
func TestService_EntityHook_UnregisteredEntityType(t *testing.T) {
	cfg := cfg4Eyes(false)
	svc, repo := buildTestService(map[string]*Config{cfg.EntityType: cfg})

	hook := &stubHook{}
	svc.RegisterEntityHook("SOME_OTHER_ENTITY", hook)

	mUUID := uuid.New()
	inst := seedWorkflow(repo, cfg, mUUID)

	ctx := ctxWithClaims(mUUID, "maker", "penempatan.submit")
	_, err := svc.Submit(ctx, SubmitInput{
		EntityType: cfg.EntityType,
		EntityID:   inst.EntityID,
		Request:    defaultActionRequest(),
	})
	mustNoError(t, "submit", err)

	if hook.called != 0 {
		t.Errorf("hook for different entity type should not be called, got %d calls", hook.called)
	}
}

// orderTrackingHook records call order.
type orderTrackingHook struct {
	id    int
	order *[]int
}

func (h *orderTrackingHook) BeforeCommit(_ context.Context, _ *sql.Tx, _ HookEvent) error {
	*h.order = append(*h.order, h.id)
	return nil
}
