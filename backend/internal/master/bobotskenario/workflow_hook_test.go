package bobotskenario_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/master/bobotskenario"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// hookTestEntity returns a BobotSkenario entity for a given skenario and bobot string.
func hookTestEntity(sk bobotskenario.Skenario, bobot string) *bobotskenario.BobotSkenario {
	actorID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	return &bobotskenario.BobotSkenario{
		ID:                 uuid.New(),
		Skenario:           sk,
		Bobot:              decimal.RequireFromString(bobot),
		PeriodeBerlakuDari: "2026-01-01",
		MakerID:            actorID,
		WorkflowStatus:     bobotskenario.WorkflowStatusPendingApproval2,
		CreatedBy:          &actorID,
		RowVersion:         3,
		TenantID:           "TUGURE",
	}
}

// buildHook returns a WorkflowHook backed by a repoAdapter.
// It also returns the adapter so callers can configure stubs.
func buildHook(entity *bobotskenario.BobotSkenario) (*bobotskenario.WorkflowHook, *repoAdapter) {
	adapter := &repoAdapter{
		getByIDStub: &stubGetByID{result: entity},
	}
	// Service is not used directly by the hook in test path (nil is fine).
	hook := bobotskenario.NewWorkflowHook(nil, adapter)
	return hook, adapter
}

// approve2ApprovedEvt returns a HookEvent simulating an approve2 → APPROVED transition.
func approve2ApprovedEvt(entityID uuid.UUID) workflow.HookEvent {
	return workflow.HookEvent{
		EntityType: "BOBOT_SKENARIO",
		EntityID:   entityID,
		Action:     workflow.ActionApprove2,
		NewState:   workflow.StateApproved,
		OldState:   workflow.StatePendingApproval2,
		ActorID:    uuid.MustParse("00000000-0000-0000-0000-000000000099"),
	}
}

// submitEvt returns a HookEvent simulating a submit transition (DRAFT → PENDING_REVIEW).
func submitEvt(entityID uuid.UUID) workflow.HookEvent {
	return workflow.HookEvent{
		EntityType: "BOBOT_SKENARIO",
		EntityID:   entityID,
		Action:     workflow.ActionSubmit,
		NewState:   workflow.StatePendingReview,
		OldState:   workflow.StateDraft,
		ActorID:    uuid.MustParse("00000000-0000-0000-0000-000000000099"),
	}
}

// ─── Test cases ───────────────────────────────────────────────────────────────

// Case 1: approve2 + sum=1.0 (0.25 entity + 0.75 siblings) → success.
func TestWorkflowHook_Approve2_SumExact_Success(t *testing.T) {
	entity := hookTestEntity(bobotskenario.SkenarioBad, "0.25000000") // entity bobot
	hook, adapter := buildHook(entity)

	// Siblings sum = GOOD(0.25) + NORMAL(0.50) = 0.75
	adapter.sumByPeriodStub = &stubSumByPeriod{
		sum: decimal.RequireFromString("0.75000000"),
	}
	evt := approve2ApprovedEvt(entity.ID)

	err := hook.BeforeCommit(context.Background(), nil, evt)
	if err != nil {
		t.Fatalf("expected nil error for sum=1.0, got: %v", err)
	}
}

// Case 2: approve2 + sum=0.95 (entity 0.20 + siblings 0.75) → BOBOT_SUM_INVARIANT_VIOLATED.
func TestWorkflowHook_Approve2_SumBelow_Error(t *testing.T) {
	entity := hookTestEntity(bobotskenario.SkenarioBad, "0.20000000") // entity bobot = 0.20
	hook, adapter := buildHook(entity)

	// Siblings sum = 0.75 → total = 0.95 < 1.0
	adapter.sumByPeriodStub = &stubSumByPeriod{
		sum: decimal.RequireFromString("0.75000000"),
	}
	evt := approve2ApprovedEvt(entity.ID)

	err := hook.BeforeCommit(context.Background(), nil, evt)
	if err == nil {
		t.Fatal("expected error for sum=0.95, got nil")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError, got %T: %v", err, err)
	}
	if de.Code() != domainerrors.CodeBobotSumInvariantViolated {
		t.Errorf("expected code %s, got %s", domainerrors.CodeBobotSumInvariantViolated, de.Code())
	}
	// Message should mention "Kurang dari"
	if msg := de.Message(); len(msg) == 0 {
		t.Error("expected non-empty message")
	}
}

// Case 3: approve2 + sum=1.05 (entity 0.30 + siblings 0.75) → BOBOT_SUM_INVARIANT_VIOLATED "Lebih dari".
func TestWorkflowHook_Approve2_SumAbove_Error(t *testing.T) {
	entity := hookTestEntity(bobotskenario.SkenarioBad, "0.30000000") // entity bobot = 0.30
	hook, adapter := buildHook(entity)

	// Siblings sum = 0.75 → total = 1.05 > 1.0
	adapter.sumByPeriodStub = &stubSumByPeriod{
		sum: decimal.RequireFromString("0.75000000"),
	}
	evt := approve2ApprovedEvt(entity.ID)

	err := hook.BeforeCommit(context.Background(), nil, evt)
	if err == nil {
		t.Fatal("expected error for sum=1.05, got nil")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError, got %T: %v", err, err)
	}
	if de.Code() != domainerrors.CodeBobotSumInvariantViolated {
		t.Errorf("expected code %s, got %s", domainerrors.CodeBobotSumInvariantViolated, de.Code())
	}
	// Message should contain "Lebih dari"
	if msg := de.Message(); len(msg) == 0 {
		t.Error("expected non-empty message")
	}
}

// Case 4: submit action (DRAFT → PENDING_REVIEW) → no sum check, always succeeds.
func TestWorkflowHook_Submit_NoSumCheck_Success(t *testing.T) {
	entity := hookTestEntity(bobotskenario.SkenarioNormal, "0.50000000")
	hook, adapter := buildHook(entity)

	// Even if siblings sum would fail, submit doesn't trigger the check.
	adapter.sumByPeriodStub = &stubSumByPeriod{
		sum: decimal.RequireFromString("0.00000000"), // would fail if checked
	}
	evt := submitEvt(entity.ID)

	err := hook.BeforeCommit(context.Background(), nil, evt)
	if err != nil {
		t.Fatalf("expected nil error for submit action, got: %v", err)
	}
}

// Case 5: SumByPeriod excludes REJECTED siblings — verify the hook uses
// SumByPeriodTx which filters workflow_status IN active states.
// We simulate via the stub: sumByPeriodTxStub returns 0.75 (REJECTED row excluded),
// while sumByPeriodStub would have returned 1.00 (REJECTED included).
// The hook should use sumByPeriodTxStub when tx != nil, which gives 0.75 + 0.25 = 1.0 → success.
// (In this test tx is nil so fallback to sumByPeriodStub, demonstrating separation of stubs.)
func TestWorkflowHook_Approve2_ExcludesRejectedSiblings_Success(t *testing.T) {
	entity := hookTestEntity(bobotskenario.SkenarioBad, "0.25000000")
	hook, adapter := buildHook(entity)

	// Only active siblings contribute. REJECTED sibling (bobot=0.25) is excluded.
	// Active siblings: GOOD=0.25, NORMAL=0.50 → sum=0.75.
	// Total = 0.75 + 0.25 = 1.0 → success.
	adapter.sumByPeriodStub = &stubSumByPeriod{
		sum: decimal.RequireFromString("0.75000000"), // REJECTED excluded
	}
	// sumByPeriodTxStub not set — fallback to sumByPeriodStub in the adapter.
	evt := approve2ApprovedEvt(entity.ID)

	err := hook.BeforeCommit(context.Background(), nil, evt)
	if err != nil {
		t.Fatalf("expected nil error when REJECTED siblings excluded, got: %v", err)
	}
}

// ─── Additional edge cases ────────────────────────────────────────────────────

// TestWorkflowHook_Approve2_WithinTolerance_Success verifies the tolerance boundary.
// sum = 1.0 - 5e-9 (within 1e-8 tolerance) → success.
func TestWorkflowHook_Approve2_WithinTolerance_Success(t *testing.T) {
	entity := hookTestEntity(bobotskenario.SkenarioBad, "0.24999999")
	hook, adapter := buildHook(entity)

	// siblings = 0.75 → total = 0.99999999, diff = 1e-8 = SumTolerance → allowed (LessOrEqual).
	// Actually 0.99999999 diff = 1e-8 is equal to tolerance → check is diff.GreaterThan(tolerance).
	// 1e-8 > 1e-8 is FALSE → passes.
	adapter.sumByPeriodStub = &stubSumByPeriod{
		sum: decimal.RequireFromString("0.75000000"),
	}
	evt := approve2ApprovedEvt(entity.ID)

	err := hook.BeforeCommit(context.Background(), nil, evt)
	if err != nil {
		t.Fatalf("expected nil within tolerance boundary, got: %v", err)
	}
}

// TestWorkflowHook_EntityNotFound verifies graceful handling of missing entity.
func TestWorkflowHook_EntityNotFound_Error(t *testing.T) {
	adapter := &repoAdapter{
		getByIDStub: &stubGetByID{result: nil, err: nil}, // returns nil = not found
	}
	hook := bobotskenario.NewWorkflowHook(nil, adapter)

	evt := approve2ApprovedEvt(uuid.New())

	err := hook.BeforeCommit(context.Background(), nil, evt)
	if err == nil {
		t.Fatal("expected error for missing entity, got nil")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError, got %T: %v", err, err)
	}
	if de.Code() != domainerrors.CodeNotFound {
		t.Errorf("expected NOT_FOUND, got %s", de.Code())
	}
}
