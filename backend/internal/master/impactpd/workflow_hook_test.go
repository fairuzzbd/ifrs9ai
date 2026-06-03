package impactpd_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/master/impactpd"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// TestWorkflowHook_ImplementsEntityHook verifies the interface at compile time.
func TestWorkflowHook_ImplementsEntityHook(t *testing.T) {
	var _ workflow.EntityHook = (*impactpd.WorkflowHook)(nil)
}

// TestWorkflowHook_OnTransition_EntityNotFound verifies missing entity causes error.
func TestWorkflowHook_OnTransition_EntityNotFound(t *testing.T) {
	adapter := &repoAdapter{
		getByID: &stubGetByID{result: nil, err: nil},
	}
	svc := impactpd.NewService(adapter, audit.NewWriter(nil), slog.Default())
	hook := impactpd.NewWorkflowHook(svc)

	claims := &auth.Claims{
		Sub:      "00000000-0000-0000-0000-000000000001",
		TenantID: "TUGURE",
	}
	ctx := auth.ContextWithClaims(context.Background(), claims)

	err := hook.OnTransition(ctx, workflow.HookEvent{
		EntityID:   uuid.New(),
		EntityType: "IMPACT_PD",
		NewState:   "APPROVED",
		Action:     "approve",
	})
	if err == nil {
		t.Error("expected error when entity not found, got nil")
	}
}

// TestWorkflowHook_OnTransition_NoClaims verifies that missing JWT claims cause error.
func TestWorkflowHook_OnTransition_NoClaims(t *testing.T) {
	m := &impactpd.ImpactPD{
		ID:               uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		PeriodeID:        uuid.MustParse("00000000-0000-0000-0000-000000000010"),
		ImpactMultiplier: decimal.NewFromFloat(1.1),
		WorkflowStatus:   impactpd.WorkflowStatusPendingApproval,
		RowVersion:       1,
		TenantID:         "TUGURE",
	}
	adapter := &repoAdapter{
		getByID: &stubGetByID{result: m},
	}
	svc := impactpd.NewService(adapter, audit.NewWriter(nil), slog.Default())
	hook := impactpd.NewWorkflowHook(svc)

	err := hook.OnTransition(context.Background(), workflow.HookEvent{
		EntityID:   m.ID,
		EntityType: "IMPACT_PD",
		NewState:   "APPROVED",
		Action:     "approve",
	})
	if err == nil {
		t.Error("expected error when no claims in context, got nil")
	}
}
