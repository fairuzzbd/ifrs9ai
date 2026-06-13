package calcrun

// service_whitebox_test.go — white-box tests for unexported helpers in service.go.
// In package calcrun (not calcrun_test) so unexported types are accessible.

import (
	"context"
	"testing"
)

// ─── noopJobUpdater ───────────────────────────────────────────────────────────
// noopJobUpdater is used when no JobProgressUpdater is wired. All methods must
// return nil and not panic.

func TestNoopJobUpdater_UpdateStatus(t *testing.T) {
	n := &noopJobUpdater{}
	ctx := context.Background()
	if err := n.UpdateStatus(ctx, "job-1", "running", 50, "step"); err != nil {
		t.Errorf("UpdateStatus returned non-nil error: %v", err)
	}
}

func TestNoopJobUpdater_MarkCompleted(t *testing.T) {
	n := &noopJobUpdater{}
	ctx := context.Background()
	if err := n.MarkCompleted(ctx, "job-2", map[string]any{"calcRunId": "run-1"}); err != nil {
		t.Errorf("MarkCompleted returned non-nil error: %v", err)
	}
}

func TestNoopJobUpdater_MarkFailed(t *testing.T) {
	n := &noopJobUpdater{}
	ctx := context.Background()
	if err := n.MarkFailed(ctx, "job-3", "INTERNAL", "unexpected error"); err != nil {
		t.Errorf("MarkFailed returned non-nil error: %v", err)
	}
}

// ─── noopJobUpdater satisfies JobProgressUpdater interface ───────────────────

func TestNoopJobUpdater_ImplementsInterface(t *testing.T) {
	var _ JobProgressUpdater = (*noopJobUpdater)(nil)
}
