package penempatan_test

// domain_test.go — unit tests for Status methods and domain constants (P5-M1).
// Pure in-memory, no DB, 100% coverage on Status state machine.

import (
	"testing"

	"blips-ifrs9.tugu-re.com/internal/app-b/penempatan"
)

// ─── Status.IsTerminal ─────────────────────────────────────────────────────

func TestStatus_IsTerminal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		status   penempatan.Status
		terminal bool
	}{
		{"Matured", penempatan.StatusMatured, true},
		{"Terminated", penempatan.StatusTerminated, true},
		{"Cancelled", penempatan.StatusCancelled, true},
		{"Draft", penempatan.StatusDraft, false},
		{"PendingReview", penempatan.StatusPendingReview, false},
		{"PendingApproval", penempatan.StatusPendingApproval, false},
		{"ApprovedActive", penempatan.StatusApprovedActive, false},
		{"Rejected", penempatan.StatusRejected, false},
		{"TerminationPendingReview", penempatan.StatusTerminationPendingReview, false},
		{"TerminationPendingApproval", penempatan.StatusTerminationPendingApproval, false},
		{"TerminationRejected", penempatan.StatusTerminationRejected, false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.status.IsTerminal(); got != tc.terminal {
				t.Errorf("Status(%q).IsTerminal() = %v, want %v", tc.status, got, tc.terminal)
			}
		})
	}
}

// ─── Status.CanSubmit ──────────────────────────────────────────────────────

func TestStatus_CanSubmit(t *testing.T) {
	t.Parallel()
	allowedFrom := map[penempatan.Status]bool{
		penempatan.StatusDraft: true,
	}
	allStatuses := allStatusValues()
	for _, s := range allStatuses {
		s := s
		t.Run(string(s), func(t *testing.T) {
			t.Parallel()
			want := allowedFrom[s]
			if got := s.CanSubmit(); got != want {
				t.Errorf("Status(%q).CanSubmit() = %v, want %v", s, got, want)
			}
		})
	}
}

// ─── Status.CanEdit ────────────────────────────────────────────────────────

func TestStatus_CanEdit(t *testing.T) {
	t.Parallel()
	allowedFrom := map[penempatan.Status]bool{
		penempatan.StatusDraft: true,
	}
	for _, s := range allStatusValues() {
		s := s
		t.Run(string(s), func(t *testing.T) {
			t.Parallel()
			want := allowedFrom[s]
			if got := s.CanEdit(); got != want {
				t.Errorf("Status(%q).CanEdit() = %v, want %v", s, got, want)
			}
		})
	}
}

// ─── Status.CanWithdraw ────────────────────────────────────────────────────

func TestStatus_CanWithdraw(t *testing.T) {
	t.Parallel()
	allowedFrom := map[penempatan.Status]bool{
		penempatan.StatusDraft: true,
	}
	for _, s := range allStatusValues() {
		s := s
		t.Run(string(s), func(t *testing.T) {
			t.Parallel()
			want := allowedFrom[s]
			if got := s.CanWithdraw(); got != want {
				t.Errorf("Status(%q).CanWithdraw() = %v, want %v", s, got, want)
			}
		})
	}
}

// ─── Status.CanReview ──────────────────────────────────────────────────────

func TestStatus_CanReview(t *testing.T) {
	t.Parallel()
	allowedFrom := map[penempatan.Status]bool{
		penempatan.StatusPendingReview: true,
	}
	for _, s := range allStatusValues() {
		s := s
		t.Run(string(s), func(t *testing.T) {
			t.Parallel()
			want := allowedFrom[s]
			if got := s.CanReview(); got != want {
				t.Errorf("Status(%q).CanReview() = %v, want %v", s, got, want)
			}
		})
	}
}

// ─── Status.CanApprove ─────────────────────────────────────────────────────

func TestStatus_CanApprove(t *testing.T) {
	t.Parallel()
	allowedFrom := map[penempatan.Status]bool{
		penempatan.StatusPendingApproval: true,
	}
	for _, s := range allStatusValues() {
		s := s
		t.Run(string(s), func(t *testing.T) {
			t.Parallel()
			want := allowedFrom[s]
			if got := s.CanApprove(); got != want {
				t.Errorf("Status(%q).CanApprove() = %v, want %v", s, got, want)
			}
		})
	}
}

// ─── Status.CanReject ──────────────────────────────────────────────────────

func TestStatus_CanReject(t *testing.T) {
	t.Parallel()
	allowedFrom := map[penempatan.Status]bool{
		penempatan.StatusPendingReview:   true,
		penempatan.StatusPendingApproval: true,
	}
	for _, s := range allStatusValues() {
		s := s
		t.Run(string(s), func(t *testing.T) {
			t.Parallel()
			want := allowedFrom[s]
			if got := s.CanReject(); got != want {
				t.Errorf("Status(%q).CanReject() = %v, want %v", s, got, want)
			}
		})
	}
}

// ─── Status.CanRequestTerminate ────────────────────────────────────────────

func TestStatus_CanRequestTerminate(t *testing.T) {
	t.Parallel()
	allowedFrom := map[penempatan.Status]bool{
		penempatan.StatusApprovedActive: true,
	}
	for _, s := range allStatusValues() {
		s := s
		t.Run(string(s), func(t *testing.T) {
			t.Parallel()
			want := allowedFrom[s]
			if got := s.CanRequestTerminate(); got != want {
				t.Errorf("Status(%q).CanRequestTerminate() = %v, want %v", s, got, want)
			}
		})
	}
}

// ─── Status.CanTerminateReview ─────────────────────────────────────────────

func TestStatus_CanTerminateReview(t *testing.T) {
	t.Parallel()
	allowedFrom := map[penempatan.Status]bool{
		penempatan.StatusTerminationPendingReview: true,
	}
	for _, s := range allStatusValues() {
		s := s
		t.Run(string(s), func(t *testing.T) {
			t.Parallel()
			want := allowedFrom[s]
			if got := s.CanTerminateReview(); got != want {
				t.Errorf("Status(%q).CanTerminateReview() = %v, want %v", s, got, want)
			}
		})
	}
}

// ─── Status.CanTerminateApprove ────────────────────────────────────────────

func TestStatus_CanTerminateApprove(t *testing.T) {
	t.Parallel()
	allowedFrom := map[penempatan.Status]bool{
		penempatan.StatusTerminationPendingApproval: true,
	}
	for _, s := range allStatusValues() {
		s := s
		t.Run(string(s), func(t *testing.T) {
			t.Parallel()
			want := allowedFrom[s]
			if got := s.CanTerminateApprove(); got != want {
				t.Errorf("Status(%q).CanTerminateApprove() = %v, want %v", s, got, want)
			}
		})
	}
}

// ─── Status.CanTerminateReject ─────────────────────────────────────────────

func TestStatus_CanTerminateReject(t *testing.T) {
	t.Parallel()
	allowedFrom := map[penempatan.Status]bool{
		penempatan.StatusTerminationPendingReview:   true,
		penempatan.StatusTerminationPendingApproval: true,
	}
	for _, s := range allStatusValues() {
		s := s
		t.Run(string(s), func(t *testing.T) {
			t.Parallel()
			want := allowedFrom[s]
			if got := s.CanTerminateReject(); got != want {
				t.Errorf("Status(%q).CanTerminateReject() = %v, want %v", s, got, want)
			}
		})
	}
}

// ─── Error code constants ──────────────────────────────────────────────────

func TestErrCodeConstants(t *testing.T) {
	t.Parallel()

	codes := []struct {
		name  string
		value string
	}{
		{"InstrumenNotFound", penempatan.ErrCodeInstrumenNotFound},
		{"InstrumenInvalidKlasifikasi", penempatan.ErrCodeInstrumenInvalidKlasifikasi},
		{"TanggalPenempatanInvalid", penempatan.ErrCodeTanggalPenempatanInvalid},
		{"TenorInvalid", penempatan.ErrCodeTenorInvalid},
		{"KuponInvalid", penempatan.ErrCodeKuponInvalid},
		{"InvalidTransition", penempatan.ErrCodeInvalidTransition},
		{"SoDViolation", penempatan.ErrCodeSoDViolation},
		{"StepUpRequired", penempatan.ErrCodeStepUpRequired},
		{"ReasonTooShort", penempatan.ErrCodeReasonTooShort},
		{"EditLocked", penempatan.ErrCodeEditLocked},
		{"PeriodeHardClosed", penempatan.ErrCodePeriodeHardClosed},
		{"TerminateForbiddenNotActive", penempatan.ErrCodeTerminateForbiddenNotActive},
		{"Calc2010", penempatan.ErrCodeCalc2010},
		{"NotFound", penempatan.ErrCodeNotFound},
	}

	for _, c := range codes {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if c.value == "" {
				t.Errorf("ErrCode %s is empty string", c.name)
			}
		})
	}
}

// ─── Permission constants ──────────────────────────────────────────────────

func TestPermConstants(t *testing.T) {
	t.Parallel()

	perms := []struct {
		name  string
		value string
	}{
		{"Create", penempatan.PermTransaksiCreate},
		{"Read", penempatan.PermTransaksiRead},
		{"Update", penempatan.PermTransaksiUpdate},
		{"Delete", penempatan.PermTransaksiDelete},
		{"Submit", penempatan.PermTransaksiSubmit},
		{"Review", penempatan.PermTransaksiReview},
		{"Approve", penempatan.PermTransaksiApprove},
		{"Reject", penempatan.PermTransaksiReject},
		{"Terminate", penempatan.PermTransaksiTerminate},
		{"AuditLogRead", penempatan.PermAuditLogRead},
		{"EIRPreview", penempatan.PermEIRPreview},
	}

	for _, p := range perms {
		p := p
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			if p.value == "" {
				t.Errorf("Perm %s is empty string", p.name)
			}
		})
	}
}

// ─── Asynq task type constants ─────────────────────────────────────────────

func TestTaskTypeConstants(t *testing.T) {
	t.Parallel()

	types := []string{
		penempatan.PenempatanApprovedTaskType,
		penempatan.PenempatanMaturedTaskType,
		penempatan.PenempatanTerminatedTaskType,
		penempatan.EIRComputeTaskType,
		penempatan.MaturityCheckTaskType,
	}

	for _, tt := range types {
		tt := tt
		t.Run(tt, func(t *testing.T) {
			t.Parallel()
			if tt == "" {
				t.Error("task type is empty")
			}
		})
	}
}

// ─── helpers ───────────────────────────────────────────────────────────────

// allStatusValues returns every Status enum value (11 values per domain.go).
func allStatusValues() []penempatan.Status {
	return []penempatan.Status{
		penempatan.StatusDraft,
		penempatan.StatusPendingReview,
		penempatan.StatusPendingApproval,
		penempatan.StatusApprovedActive,
		penempatan.StatusRejected,
		penempatan.StatusCancelled,
		penempatan.StatusMatured,
		penempatan.StatusTerminationPendingReview,
		penempatan.StatusTerminationPendingApproval,
		penempatan.StatusTerminated,
		penempatan.StatusTerminationRejected,
	}
}
