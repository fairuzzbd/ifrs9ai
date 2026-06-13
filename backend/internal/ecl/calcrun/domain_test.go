package calcrun_test

import (
	"testing"

	"blips-ifrs9.tugu-re.com/internal/ecl/calcrun"
)

// domain_test.go — unit tests for CalcRunStatus enum and helper methods.
// Covers all valid transitions and terminal-state detection.
// Reference: docs/state-machines/p4-m8-calc-run.md §2 transition table.

func TestCalcRunStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		status   calcrun.CalcRunStatus
		terminal bool
	}{
		{calcrun.StatusDraft, false},
		{calcrun.StatusInProgress, false},
		{calcrun.StatusCompleted, false},
		{calcrun.StatusCompletedWithErrors, false},
		{calcrun.StatusSealRequested, false},
		{calcrun.StatusSealed, true},
		{calcrun.StatusCancelled, true},
	}
	for _, tt := range tests {
		got := tt.status.IsTerminal()
		if got != tt.terminal {
			t.Errorf("IsTerminal(%q) = %v; want %v", tt.status, got, tt.terminal)
		}
	}
}

func TestCalcRunStatus_CanStart(t *testing.T) {
	canStart := []calcrun.CalcRunStatus{calcrun.StatusDraft}
	cannotStart := []calcrun.CalcRunStatus{
		calcrun.StatusInProgress,
		calcrun.StatusCompleted,
		calcrun.StatusCompletedWithErrors,
		calcrun.StatusSealRequested,
		calcrun.StatusSealed,
		calcrun.StatusCancelled,
	}
	for _, s := range canStart {
		if !s.CanStart() {
			t.Errorf("CanStart(%q) = false; want true", s)
		}
	}
	for _, s := range cannotStart {
		if s.CanStart() {
			t.Errorf("CanStart(%q) = true; want false", s)
		}
	}
}

func TestCalcRunStatus_CanCancel(t *testing.T) {
	canCancel := []calcrun.CalcRunStatus{
		calcrun.StatusDraft,
		calcrun.StatusInProgress,
	}
	cannotCancel := []calcrun.CalcRunStatus{
		calcrun.StatusCompleted,
		calcrun.StatusCompletedWithErrors,
		calcrun.StatusSealRequested,
		calcrun.StatusSealed,
		calcrun.StatusCancelled,
	}
	for _, s := range canCancel {
		if !s.CanCancel() {
			t.Errorf("CanCancel(%q) = false; want true", s)
		}
	}
	for _, s := range cannotCancel {
		if s.CanCancel() {
			t.Errorf("CanCancel(%q) = true; want false", s)
		}
	}
}

func TestCalcRunStatus_CanRequestSeal(t *testing.T) {
	canRequest := []calcrun.CalcRunStatus{calcrun.StatusCompleted}
	cannotRequest := []calcrun.CalcRunStatus{
		calcrun.StatusDraft,
		calcrun.StatusInProgress,
		calcrun.StatusCompletedWithErrors,
		calcrun.StatusSealRequested,
		calcrun.StatusSealed,
		calcrun.StatusCancelled,
	}
	for _, s := range canRequest {
		if !s.CanRequestSeal() {
			t.Errorf("CanRequestSeal(%q) = false; want true", s)
		}
	}
	for _, s := range cannotRequest {
		if s.CanRequestSeal() {
			t.Errorf("CanRequestSeal(%q) = true; want false", s)
		}
	}
}

func TestCalcRunStatus_CanApproveSeal(t *testing.T) {
	canApprove := []calcrun.CalcRunStatus{calcrun.StatusSealRequested}
	cannotApprove := []calcrun.CalcRunStatus{
		calcrun.StatusDraft,
		calcrun.StatusInProgress,
		calcrun.StatusCompleted,
		calcrun.StatusCompletedWithErrors,
		calcrun.StatusSealed,
		calcrun.StatusCancelled,
	}
	for _, s := range canApprove {
		if !s.CanApproveSeal() {
			t.Errorf("CanApproveSeal(%q) = false; want true", s)
		}
	}
	for _, s := range cannotApprove {
		if s.CanApproveSeal() {
			t.Errorf("CanApproveSeal(%q) = true; want false", s)
		}
	}
}

func TestPermissionConstants(t *testing.T) {
	// Sanity check that permission strings are non-empty and unique.
	perms := []string{
		calcrun.PermCalcRunCreate,
		calcrun.PermCalcRunRead,
		calcrun.PermCalcRunStart,
		calcrun.PermCalcRunCancel,
		calcrun.PermCalcRunSealRequest,
		calcrun.PermCalcRunSealApprove,
		calcrun.PermCalcRunExport,
	}
	seen := make(map[string]bool)
	for _, p := range perms {
		if p == "" {
			t.Error("empty permission string found")
		}
		if seen[p] {
			t.Errorf("duplicate permission string: %q", p)
		}
		seen[p] = true
	}
}

func TestCalcRunStatus_StateTransitionMatrix(t *testing.T) {
	// Validates the full state machine transition matrix from §2.
	// Reference: p4-m8-calc-run.md
	type stateCheck struct {
		status      calcrun.CalcRunStatus
		canStart    bool
		canCancel   bool
		canReqSeal  bool
		canApprSeal bool
		isTerminal  bool
	}
	matrix := []stateCheck{
		{calcrun.StatusDraft, true, true, false, false, false},
		{calcrun.StatusInProgress, false, true, false, false, false},
		{calcrun.StatusCompleted, false, false, true, false, false},
		{calcrun.StatusCompletedWithErrors, false, false, false, false, false},
		{calcrun.StatusSealRequested, false, false, false, true, false},
		{calcrun.StatusSealed, false, false, false, false, true},
		{calcrun.StatusCancelled, false, false, false, false, true},
	}
	for _, m := range matrix {
		if got := m.status.CanStart(); got != m.canStart {
			t.Errorf("[%s] CanStart = %v; want %v", m.status, got, m.canStart)
		}
		if got := m.status.CanCancel(); got != m.canCancel {
			t.Errorf("[%s] CanCancel = %v; want %v", m.status, got, m.canCancel)
		}
		if got := m.status.CanRequestSeal(); got != m.canReqSeal {
			t.Errorf("[%s] CanRequestSeal = %v; want %v", m.status, got, m.canReqSeal)
		}
		if got := m.status.CanApproveSeal(); got != m.canApprSeal {
			t.Errorf("[%s] CanApproveSeal = %v; want %v", m.status, got, m.canApprSeal)
		}
		if got := m.status.IsTerminal(); got != m.isTerminal {
			t.Errorf("[%s] IsTerminal = %v; want %v", m.status, got, m.isTerminal)
		}
	}
}
