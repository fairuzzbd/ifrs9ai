package impactmevpd_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/master/impactmevpd"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// TestWorkflowHook_ImplementsEntityHook verifies the interface is satisfied at compile time.
func TestWorkflowHook_ImplementsEntityHook(t *testing.T) {
	var _ workflow.EntityHook = (*impactmevpd.WorkflowHook)(nil)
}

// TestWorkflowHook_OnTransition_EntityNotFound verifies that a missing entity
// causes OnTransition to return an error (service.SyncWorkflowStatus returns 404).
func TestWorkflowHook_OnTransition_EntityNotFound(t *testing.T) {
	adapter := &repoAdapter{
		getByID: &stubGetByID{result: nil, err: nil},
	}
	svc := impactmevpd.NewService(adapter, audit.NewWriter(nil), slog.Default())
	hook := impactmevpd.NewWorkflowHook(svc)

	// Need claims in context for requireActor to pass.
	claims := &auth.Claims{
		Sub:      "00000000-0000-0000-0000-000000000001",
		TenantID: "TUGURE",
	}
	ctx := auth.ContextWithClaims(context.Background(), claims)

	entityID := uuid.New()
	err := hook.OnTransition(ctx, workflow.HookEvent{
		EntityID:   entityID,
		EntityType: "IMPACT_MEV_PD",
		NewState:   "APPROVED",
		Action:     "approve",
	})

	if err == nil {
		t.Error("expected error when entity not found, got nil")
	}
}

// TestWorkflowHook_OnTransition_NoClaims verifies that missing JWT claims cause error.
func TestWorkflowHook_OnTransition_NoClaims(t *testing.T) {
	m := &impactmevpd.ImpactMevPD{
		ID:               uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		PeriodeID:        uuid.MustParse("00000000-0000-0000-0000-000000000010"),
		Skenario:         impactmevpd.SkenarioBad,
		ImpactMultiplier: decimal.NewFromFloat(1.25),
		WorkflowStatus:   impactmevpd.WorkflowStatusPendingApproval,
		RowVersion:       1,
		TenantID:         "TUGURE",
	}
	adapter := &repoAdapter{
		getByID: &stubGetByID{result: m},
	}
	svc := impactmevpd.NewService(adapter, audit.NewWriter(nil), slog.Default())
	hook := impactmevpd.NewWorkflowHook(svc)

	// Context without claims — should return unauthorized error.
	err := hook.OnTransition(context.Background(), workflow.HookEvent{
		EntityID:   m.ID,
		EntityType: "IMPACT_MEV_PD",
		NewState:   "APPROVED",
		Action:     "approve",
	})

	if err == nil {
		t.Error("expected error when no claims in context, got nil")
	}
}
