package errors_test

import (
	"fmt"
	"net/http"
	"testing"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

func TestNew(t *testing.T) {
	err := domainerrors.New(domainerrors.CodeNotFound, "instrumen tidak ditemukan")
	if err.Code() != domainerrors.CodeNotFound {
		t.Errorf("expected %s got %s", domainerrors.CodeNotFound, err.Code())
	}
	if err.Message() != "instrumen tidak ditemukan" {
		t.Errorf("unexpected message: %s", err.Message())
	}
	if err.HTTPStatus() != http.StatusNotFound {
		t.Errorf("expected 404 got %d", err.HTTPStatus())
	}
}

func TestWrap(t *testing.T) {
	cause := fmt.Errorf("db error")
	err := domainerrors.Wrap(domainerrors.CodeInternal, "kesalahan internal", cause)
	if err.Unwrap() != cause {
		t.Error("Unwrap should return cause")
	}
	if err.HTTPStatus() != http.StatusInternalServerError {
		t.Errorf("expected 500 got %d", err.HTTPStatus())
	}
}

func TestIsDomainError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantOK   bool
		wantCode domainerrors.Code
	}{
		{
			name:     "direct domain error",
			err:      domainerrors.New(domainerrors.CodeForbidden, "forbidden"),
			wantOK:   true,
			wantCode: domainerrors.CodeForbidden,
		},
		{
			name:     "wrapped domain error",
			err:      fmt.Errorf("outer: %w", domainerrors.New(domainerrors.CodeConflict, "conflict")),
			wantOK:   true,
			wantCode: domainerrors.CodeConflict,
		},
		{
			name:   "plain error",
			err:    fmt.Errorf("plain error"),
			wantOK: false,
		},
		{
			name:   "nil",
			err:    nil,
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			de, ok := domainerrors.IsDomainError(tc.err)
			if ok != tc.wantOK {
				t.Errorf("IsDomainError() ok=%v want %v", ok, tc.wantOK)
			}
			if ok && de.Code() != tc.wantCode {
				t.Errorf("Code()=%s want %s", de.Code(), tc.wantCode)
			}
		})
	}
}

func TestHTTPStatusMapping(t *testing.T) {
	tests := []struct {
		code     domainerrors.Code
		wantHTTP int
	}{
		{domainerrors.CodeValidationFailed, 400},
		{domainerrors.CodeUnauthorized, 401},
		{domainerrors.CodeIdleTimeout, 401},
		{domainerrors.CodeForbidden, 403},
		{domainerrors.CodeSoDViolation, 403},
		{domainerrors.CodeMFARequired, 403},
		{domainerrors.CodeMFAChallengeFailed, 403},
		{domainerrors.CodeStepUpRequired, 403},
		{domainerrors.CodeStepUpExpired, 403},
		{domainerrors.CodeNotFound, 404},
		{domainerrors.CodeConflict, 409},
		{domainerrors.CodeIdempotencyReplay, 200},
		{domainerrors.CodeIdempotencyMismatch, 422},
		{domainerrors.CodeWorkflowInvalidTransition, 422},
		{domainerrors.CodePeriodeClosed, 423},
		{domainerrors.CodeECLParamFrozen, 423},
		{domainerrors.CodeRateLimited, 429},
		{domainerrors.CodeInternal, 500},
	}
	for _, tc := range tests {
		if got := tc.code.HTTPStatus(); got != tc.wantHTTP {
			t.Errorf("Code %s: HTTPStatus()=%d want %d", tc.code, got, tc.wantHTTP)
		}
	}
}

func TestConvenienceConstructors(t *testing.T) {
	t.Run("ErrUnauthorized", func(t *testing.T) {
		err := domainerrors.ErrUnauthorized("token invalid")
		if err.Code() != domainerrors.CodeUnauthorized {
			t.Error("wrong code")
		}
	})

	t.Run("ErrForbidden", func(t *testing.T) {
		err := domainerrors.ErrForbidden("instrumen.create")
		if err.Code() != domainerrors.CodeForbidden {
			t.Error("wrong code")
		}
		if err.Message() == "" {
			t.Error("message should not be empty")
		}
	})

	t.Run("ErrSoDViolation", func(t *testing.T) {
		err := domainerrors.ErrSoDViolation("maker cannot be reviewer")
		if err.Code() != domainerrors.CodeSoDViolation {
			t.Error("wrong code")
		}
		if err.HTTPStatus() != http.StatusForbidden {
			t.Errorf("expected 403 got %d", err.HTTPStatus())
		}
	})

	t.Run("ErrWorkflowInvalidTransition", func(t *testing.T) {
		err := domainerrors.ErrWorkflowInvalidTransition("DRAFT", "APPROVED")
		if err.Code() != domainerrors.CodeWorkflowInvalidTransition {
			t.Error("wrong code")
		}
		if len(err.Details()) == 0 {
			t.Error("should have details")
		}
	})

	t.Run("ErrStepUpRequired", func(t *testing.T) {
		err := domainerrors.ErrStepUpRequired("PERIODE_HARD_CLOSE")
		if err.Code() != domainerrors.CodeStepUpRequired {
			t.Error("wrong code")
		}
	})

	t.Run("ErrStepUpExpired", func(t *testing.T) {
		err := domainerrors.ErrStepUpExpired()
		if err.Code() != domainerrors.CodeStepUpExpired {
			t.Error("wrong code")
		}
	})
}
