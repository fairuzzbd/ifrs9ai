package bobotskenario

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// WorkflowHook implements workflow.EntityHook for the BOBOT_SKENARIO entity type.
//
// It is called by workflow.Service.performTransition AFTER state+signature+audit are
// written but BEFORE tx.Commit(), so all DB writes here are atomic with the workflow
// transition.
//
// Responsibilities:
//  1. Sync mst.bobot_skenario.workflow_status to match the new workflow state.
//  2. On approve2 (→ APPROVED): enforce DEC-010 sum=1.0 invariant using a tx-scoped
//     SUM query so there is no TOCTOU window between the check and the commit.
type WorkflowHook struct {
	svc  *Service
	repo Repository
}

// NewWorkflowHook creates a WorkflowHook bound to the given service and repo.
func NewWorkflowHook(svc *Service, repo Repository) *WorkflowHook {
	return &WorkflowHook{svc: svc, repo: repo}
}

// BeforeCommit is called inside the workflow transaction.
// tx may be nil when using InMemoryRepository in unit tests — the method must
// handle that gracefully (skip DB ops, still perform pure logic checks).
func (h *WorkflowHook) BeforeCommit(ctx context.Context, tx *sql.Tx, evt workflow.HookEvent) error {
	newStatus := mapWorkflowState(string(evt.NewState))

	// 1. Load the entity so we have period fields and current bobot.
	e, err := h.repo.GetByID(ctx, evt.EntityID, false)
	if err != nil {
		return fmt.Errorf("bobotskenario hook.BeforeCommit: load entity: %w", err)
	}
	if e == nil {
		return domainerrors.ErrNotFound("BobotSkenario " + evt.EntityID.String())
	}

	// 2. Sync workflow_status on mst.bobot_skenario in the same tx.
	if tx != nil {
		actorID := evt.ActorID
		if err := h.repo.UpdateWorkflowStatusTx(ctx, tx, evt.EntityID, newStatus, actorID); err != nil {
			return fmt.Errorf("bobotskenario hook.BeforeCommit: update workflow_status: %w", err)
		}
	}

	// 3. On final approval (approve2 → APPROVED for 6-eyes), enforce sum=1.0 invariant
	//    using the same tx to prevent TOCTOU race (BLOCKER 1+2 fix).
	if evt.Action == workflow.ActionApprove2 && evt.NewState == workflow.StateApproved {
		if err := h.checkSumInvariantTx(ctx, tx, e); err != nil {
			return err
		}
	}

	return nil
}

// checkSumInvariantTx verifies G+N+B sum = 1.0 for the entity's period, running
// inside the provided transaction (or without tx if nil for in-memory tests).
//
// DEC-010: sum of GOOD + NORMAL + BAD bobot must equal exactly 1.0 within
// SumTolerance (0.00000001).
//
// It fetches the sum of all OTHER sibling rows (excludeID = entity.ID) and adds
// the current entity's bobot, then checks the total.
func (h *WorkflowHook) checkSumInvariantTx(ctx context.Context, tx *sql.Tx, e *BobotSkenario) error {
	var (
		otherSum decimal.Decimal
		err      error
	)

	if tx != nil {
		otherSum, err = h.repo.SumByPeriodTx(ctx, tx, e.PeriodeBerlakuDari, e.PeriodeBerlakuSampai, e.ID)
	} else {
		// In-memory / test mode: fall back to non-tx query.
		otherSum, err = h.repo.SumByPeriod(ctx, e.PeriodeBerlakuDari, e.PeriodeBerlakuSampai, e.ID)
	}
	if err != nil {
		return fmt.Errorf("bobotskenario hook.checkSumInvariantTx: sum siblings: %w", err)
	}

	totalSum := otherSum.Add(e.Bobot)
	diff := totalSum.Sub(SumTarget).Abs()

	if diff.GreaterThan(SumTolerance) {
		direction := "Kurang dari"
		if totalSum.GreaterThan(SumTarget) {
			direction = "Lebih dari"
		}
		return domainerrors.New(
			domainerrors.CodeBobotSumInvariantViolated,
			fmt.Sprintf("Total bobot G+N+B untuk periode %s harus = 1.0 (current sum: %s). "+
				"%s 1.0. DEC-010. Rollback dilakukan — tidak ada perubahan tersimpan.",
				e.PeriodeBerlakuDari, totalSum.StringFixed(8), direction),
			domainerrors.Detail{
				Field:   "bobot",
				Rule:    "sum_invariant",
				Message: fmt.Sprintf("Sum bobot = %s, expected 1.0 (tolerance %s)",
					totalSum.StringFixed(8), SumTolerance.String()),
			},
		)
	}
	return nil
}
