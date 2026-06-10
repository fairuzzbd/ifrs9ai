package impactmevpd

import (
	"context"
	"database/sql"
	"fmt"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// WorkflowHook implements workflow.EntityHook for IMPACT_MEV_PD entities.
// BeforeCommit runs inside the workflow transaction to keep
// mst.impact_mev_pd.workflow_status in sync atomically (pre-commit, tx-aware).
type WorkflowHook struct {
	svc  *Service
	repo Repository
}

// NewWorkflowHook creates a WorkflowHook.
func NewWorkflowHook(svc *Service, repo Repository) *WorkflowHook {
	return &WorkflowHook{svc: svc, repo: repo}
}

// BeforeCommit syncs workflow_status on mst.impact_mev_pd inside the workflow tx.
//
// F1: when transitioning to APPROVED (approve2 final step), enforce the
// single-active invariant: no other APPROVED row for the same (periode_id, skenario)
// may already exist. If a duplicate is detected, BeforeCommit returns an error which
// causes the workflow transaction to rollback — the record stays in
// PENDING_APPROVAL_2 and the caller receives 422 FL_PERIODE_DUPLICATE.
func (h *WorkflowHook) BeforeCommit(ctx context.Context, tx *sql.Tx, evt workflow.HookEvent) error {
	// Load entity once — needed for tenant_id and (for APPROVED) invariant check.
	entity, err := h.repo.GetByID(ctx, evt.EntityID, false)
	if err != nil {
		return fmt.Errorf("impactmevpd.WorkflowHook.BeforeCommit: load entity: %w", err)
	}
	tid := "TUGURE"
	if entity != nil {
		tid = entity.TenantID
	}

	// F1: single-active invariant guard on approve2 → APPROVED transition.
	if evt.NewState == workflow.StateApproved && entity != nil {
		count, cntErr := h.repo.CountDuplicateTx(ctx, tx, entity.PeriodeID, entity.Skenario, evt.EntityID, tid)
		if cntErr != nil {
			return fmt.Errorf("impactmevpd.WorkflowHook.BeforeCommit: count duplicate: %w", cntErr)
		}
		if count > 0 {
			return fmt.Errorf("impactmevpd.WorkflowHook.BeforeCommit: %w",
				domainerrors.New(domainerrors.CodeFLPeriodDuplicate,
					fmt.Sprintf("Sudah terdapat row APPROVED untuk periode %s skenario %s. "+
						"Revoke atau hapus data lama sebelum approve.", entity.PeriodeID, entity.Skenario),
				),
			)
		}
	}

	wfStatus := mapWorkflowState(string(evt.NewState))
	if err := h.repo.UpdateWorkflowStatusTx(ctx, tx, evt.EntityID, wfStatus, tid); err != nil {
		return fmt.Errorf("impactmevpd.WorkflowHook.BeforeCommit: %w", err)
	}
	return nil
}
