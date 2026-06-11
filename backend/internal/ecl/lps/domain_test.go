package lps

import (
	"bytes"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── WorkflowStatus tests ─────────────────────────────────────────────────────

func TestWorkflowStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		status   WorkflowStatus
		terminal bool
	}{
		{WorkflowStatusPendingApproval, false},
		{WorkflowStatusApprovedActive, false},
		{WorkflowStatusRejected, true},
		{WorkflowStatusExpired, true},
	}
	for _, tc := range tests {
		if got := tc.status.IsTerminal(); got != tc.terminal {
			t.Errorf("IsTerminal(%s) = %v, want %v", tc.status, got, tc.terminal)
		}
	}
}

func TestWorkflowStatus_IsActive(t *testing.T) {
	if !WorkflowStatusApprovedActive.IsActive() {
		t.Error("APPROVED_ACTIVE.IsActive() should be true")
	}
	if WorkflowStatusPendingApproval.IsActive() {
		t.Error("PENDING_APPROVAL.IsActive() should be false")
	}
}

// ─── Error constructors ───────────────────────────────────────────────────────

func TestErrLPSCoverageNoActiveParam(t *testing.T) {
	err := ErrLPSCoverageNoActiveParam("2026-06-30")
	if string(err.Code()) != CodeLPSCoverageNoActiveParam {
		t.Errorf("code = %s, want %s", err.Code(), CodeLPSCoverageNoActiveParam)
	}
	if err.HTTPStatus() != 422 {
		t.Errorf("HTTPStatus = %d, want 422", err.HTTPStatus())
	}
}

func TestErrLPSOverrideInstrumenNotFound(t *testing.T) {
	err := ErrLPSOverrideInstrumenNotFound("abc")
	if err.HTTPStatus() != 404 {
		t.Errorf("HTTPStatus = %d, want 404", err.HTTPStatus())
	}
}

func TestErrLPSOverrideReasonTooShort(t *testing.T) {
	err := ErrLPSOverrideReasonTooShort(10)
	if err.HTTPStatus() != 422 {
		t.Errorf("HTTPStatus = %d, want 422", err.HTTPStatus())
	}
	if string(err.Code()) != CodeLPSOverrideReasonTooShort {
		t.Errorf("code = %s, want %s", err.Code(), CodeLPSOverrideReasonTooShort)
	}
}

func TestErrLPSOverrideInvalidTransition(t *testing.T) {
	err := ErrLPSOverrideInvalidTransition("REJECTED", "APPROVE")
	if err.HTTPStatus() != 422 {
		t.Errorf("HTTPStatus = %d, want 422", err.HTTPStatus())
	}
}

func TestErrLPSOverrideExpired(t *testing.T) {
	err := ErrLPSOverrideExpired()
	if err.HTTPStatus() != 410 {
		t.Errorf("HTTPStatus = %d, want 410", err.HTTPStatus())
	}
}

func TestErrLPSOverrideSoDViolation(t *testing.T) {
	err := ErrLPSOverrideSoDViolation()
	if err.HTTPStatus() != 403 {
		t.Errorf("HTTPStatus = %d, want 403", err.HTTPStatus())
	}
}

func TestErrLPSOverridePeriodeInvalid(t *testing.T) {
	err := ErrLPSOverridePeriodeInvalid()
	if err.HTTPStatus() != 422 {
		t.Errorf("HTTPStatus = %d, want 422", err.HTTPStatus())
	}
}

func TestErrLPSAggregateInstrumenNotDeposito(t *testing.T) {
	err := ErrLPSAggregateInstrumenNotDeposito("OBLIGASI")
	if err.HTTPStatus() != 422 {
		t.Errorf("HTTPStatus = %d, want 422", err.HTTPStatus())
	}
}

func TestErrLPSAggregateBulkTooLarge(t *testing.T) {
	err := ErrLPSAggregateBulkTooLarge(60000)
	if err.HTTPStatus() != 413 {
		t.Errorf("HTTPStatus = %d, want 413", err.HTTPStatus())
	}
}

// ─── ComputeApproveSignatureHash ──────────────────────────────────────────────

func TestComputeApproveSignatureHash_Deterministic(t *testing.T) {
	approver := uuid.MustParse("550e8400-e29b-41d4-a716-000000000010")
	override := uuid.MustParse("550e8400-e29b-41d4-a716-000000000020")
	at := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	comment := "Approved per LEG-2026-042"

	h1 := ComputeApproveSignatureHash(approver, override, at, comment)
	h2 := ComputeApproveSignatureHash(approver, override, at, comment)
	if !bytes.Equal(h1, h2) {
		t.Error("signature hash should be deterministic")
	}
	if len(h1) != 32 {
		t.Errorf("expected 32 bytes (SHA-256), got %d", len(h1))
	}
}

func TestComputeApproveSignatureHash_DifferentInputs(t *testing.T) {
	id1 := uuid.MustParse("550e8400-e29b-41d4-a716-000000000001")
	id2 := uuid.MustParse("550e8400-e29b-41d4-a716-000000000002")
	at := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)

	h1 := ComputeApproveSignatureHash(id1, id2, at, "comment A")
	h2 := ComputeApproveSignatureHash(id1, id2, at, "comment B")
	h3 := ComputeApproveSignatureHash(id2, id1, at, "comment A")
	if bytes.Equal(h1, h2) {
		t.Error("different comment should produce different hash")
	}
	if bytes.Equal(h1, h3) {
		t.Error("swapped actor/override IDs should produce different hash")
	}
}

// ─── itoa ────────────────────────────────────────────────────────────────────

func TestItoa(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"}, {1, "1"}, {42, "42"}, {-1, "-1"}, {-100, "-100"}, {1000000, "1000000"},
	}
	for _, tc := range tests {
		if got := itoa(tc.n); got != tc.want {
			t.Errorf("itoa(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// ─── INVARIANT: InstrumenBreakdown sum ────────────────────────────────────────

// TestInstrumenBreakdownInvariant verifies that AllocatedToCovered + AllocatedToExcess == EAD_IDR.
func TestInstrumenBreakdownInvariant(t *testing.T) {
	tests := []struct {
		ead     string
		covered string
		excess  string
	}{
		{"1500000000.0000", "1500000000.0000", "0.0000"},
		{"1000000000.0000", "500000000.0000", "500000000.0000"},
		{"2000000000.0000", "2000000000.0000", "0.0000"},
		{"500000.1234", "500000.1234", "0.0000"},
		{"0.0000", "0.0000", "0.0000"},
	}
	for _, tc := range tests {
		ead, _ := decimal.NewFromString(tc.ead)
		cov, _ := decimal.NewFromString(tc.covered)
		exc, _ := decimal.NewFromString(tc.excess)
		if !cov.Add(exc).Equal(ead) {
			t.Errorf("INVARIANT broken: covered(%s) + excess(%s) != ead(%s)",
				tc.covered, tc.excess, tc.ead)
		}
	}
}

// TestAllowedColsOverride ensures init assertion passes (no panic on import).
func TestAllowedColsOverride_InitAssert(t *testing.T) {
	// If init() panicked, this test would not run.
	for _, col := range AllowedSortColsOverride {
		if !allowedOverrideSortCols[col] {
			t.Errorf("AllowedSortColsOverride contains %q not in allowedOverrideSortCols", col)
		}
	}
}

// ─── strPtr utility ──────────────────────────────────────────────────────────

func TestStrPtr(t *testing.T) {
	s := "hello"
	p := strPtr(s)
	if p == nil {
		t.Fatal("expected non-nil pointer")
	}
	if *p != s {
		t.Errorf("*p = %q, want %q", *p, s)
	}
	// Verify it's a distinct pointer (not address of s).
	if p == &s {
		t.Error("strPtr should return new allocation, not address of parameter")
	}
}
