// Package errors menyediakan domain error BLIPS yang ter-map ke stable error codes
// sesuai api-conventions.md §"Error codes". Semua error yang expose ke API WAJIB
// menggunakan tipe-tipe ini; jangan pakai fmt.Errorf langsung di handler.
package errors

import (
	"fmt"
	"net/http"
)

// Code adalah stable error code string (tidak pernah berubah antar minor version).
// Nilai wajib cocok dengan catalogue di _common.yaml §ErrorCode.
type Code string

const (
	CodeValidationFailed           Code = "VALIDATION_FAILED"
	CodeUnauthorized               Code = "UNAUTHORIZED"
	CodeIdleTimeout                Code = "IDLE_TIMEOUT"
	CodeForbidden                  Code = "FORBIDDEN"
	CodeSoDViolation               Code = "SOD_VIOLATION"
	CodeMFARequired                Code = "MFA_REQUIRED"
	CodeMFAChallengeFailed         Code = "MFA_CHALLENGE_FAILED"
	CodeNotFound                   Code = "NOT_FOUND"
	CodeConflict                   Code = "CONFLICT"
	CodeIdempotencyReplay          Code = "IDEMPOTENCY_REPLAY"
	CodeIdempotencyMismatch        Code = "IDEMPOTENCY_MISMATCH"
	CodeWorkflowInvalidTransition  Code = "WORKFLOW_INVALID_TRANSITION"
	CodePeriodeClosed              Code = "PERIODE_CLOSED"
	CodeECLParamFrozen             Code = "ECL_PARAM_FROZEN"
	CodeRateLimited                Code = "RATE_LIMITED"
	CodeInternal                   Code = "INTERNAL"
	CodeSPPITestIncomplete         Code = "SPPI_TEST_INCOMPLETE"
	CodeBMAssessmentRequired       Code = "BM_ASSESSMENT_REQUIRED"
	CodeSoDApprover1SameAsMaker    Code = "SOD_APPROVER1_SAME_AS_MAKER"
	CodeSoDApprover2SameAsReviewer Code = "SOD_APPROVER2_SAME_AS_REVIEWER"
	CodeInvalidSortCol             Code = "INVALID_SORT_COL"
	CodeStepUpRequired             Code = "STEP_UP_REQUIRED"
	CodeStepUpExpired              Code = "STEP_UP_EXPIRED"
	CodeJobNotCancellable          Code = "JOB_NOT_CANCELLABLE"
	CodeJobNotFound                Code = "JOB_NOT_FOUND"
	// Master-data module codes (shared across all mst.* modules)
	CodeSystemCurrencyProtected Code = "SYSTEM_CURRENCY_PROTECTED"
	CodeEntityInUse             Code = "ENTITY_IN_USE"
	CodeMasterApprovedNoEdit    Code = "MASTER_APPROVED_NO_EDIT"

	// ECL parameter module codes
	// CodeLGDPeriodOverlap is returned when a new lgd_basel entry has a period that
	// overlaps an existing entry of the same tipe_eksposur. HTTP 422: caller must
	// verify with ALCO before proceeding.
	CodeLGDPeriodOverlap Code = "LGD_PERIOD_OVERLAP"
)

// HTTPStatus memetakan Code ke HTTP status code.
func (c Code) HTTPStatus() int {
	switch c {
	case CodeValidationFailed, CodeInvalidSortCol:
		return http.StatusBadRequest
	case CodeUnauthorized, CodeIdleTimeout:
		return http.StatusUnauthorized
	case CodeForbidden, CodeSoDViolation, CodeMFARequired, CodeMFAChallengeFailed,
		CodeSoDApprover1SameAsMaker, CodeSoDApprover2SameAsReviewer,
		CodeStepUpRequired, CodeStepUpExpired:
		return http.StatusForbidden
	case CodeSystemCurrencyProtected, CodeMasterApprovedNoEdit:
		return http.StatusForbidden
	case CodeEntityInUse:
		return http.StatusConflict
	case CodeNotFound, CodeJobNotFound:
		return http.StatusNotFound
	case CodeConflict:
		return http.StatusConflict
	case CodeIdempotencyReplay:
		return http.StatusOK // replay: return original status
	case CodeIdempotencyMismatch, CodeWorkflowInvalidTransition,
		CodeSPPITestIncomplete, CodeBMAssessmentRequired, CodeJobNotCancellable,
		CodeLGDPeriodOverlap:
		return http.StatusUnprocessableEntity
	case CodePeriodeClosed, CodeECLParamFrozen:
		return 423 // Locked
	case CodeRateLimited:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

// Detail adalah field-level error detail (sesuai OpenAPI ErrorDetail).
type Detail struct {
	Field   string `json:"field,omitempty"`
	Rule    string `json:"rule,omitempty"`
	Message string `json:"message,omitempty"`
}

// DomainError adalah error yang aman untuk diekspos ke client melalui API.
// Implements error interface.
type DomainError struct {
	code    Code
	message string
	details []Detail
	cause   error
}

func (e *DomainError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.message, e.cause)
	}
	return e.message
}

// Unwrap memungkinkan errors.Is/As bekerja dengan cause chain.
func (e *DomainError) Unwrap() error { return e.cause }

// Code mengembalikan stable error code.
func (e *DomainError) Code() Code { return e.code }

// Message mengembalikan pesan yang aman untuk client.
func (e *DomainError) Message() string { return e.message }

// Details mengembalikan field-level error details.
func (e *DomainError) Details() []Detail { return e.details }

// HTTPStatus mengembalikan HTTP status code yang sesuai.
func (e *DomainError) HTTPStatus() int { return e.code.HTTPStatus() }

// New membuat DomainError baru.
func New(code Code, message string, details ...Detail) *DomainError {
	return &DomainError{code: code, message: message, details: details}
}

// Wrap membungkus error underlying dengan domain error.
func Wrap(code Code, message string, cause error) *DomainError {
	return &DomainError{code: code, message: message, cause: cause}
}

// IsDomainError mengecek apakah error adalah DomainError.
func IsDomainError(err error) (*DomainError, bool) {
	var de *DomainError
	if ok := asError(err, &de); ok {
		return de, true
	}
	return nil, false
}

// asError adalah helper untuk menghindari import errors stdlib pada level ini.
func asError(err error, target **DomainError) bool {
	if err == nil {
		return false
	}
	if de, ok := err.(*DomainError); ok {
		*target = de
		return true
	}
	// Walk cause chain
	type unwrapper interface{ Unwrap() error }
	if u, ok := err.(unwrapper); ok {
		return asError(u.Unwrap(), target)
	}
	return false
}

// --- Convenience constructors ---

// ErrUnauthorized returns 401 UNAUTHORIZED.
func ErrUnauthorized(msg string) *DomainError {
	return New(CodeUnauthorized, msg)
}

// ErrIdleTimeout returns 401 IDLE_TIMEOUT.
func ErrIdleTimeout() *DomainError {
	return New(CodeIdleTimeout, "Sesi idle lebih dari 15 menit. Silakan login kembali.")
}

// ErrForbidden returns 403 FORBIDDEN.
func ErrForbidden(permission string) *DomainError {
	return New(CodeForbidden,
		fmt.Sprintf("Anda tidak memiliki permission '%s'", permission))
}

// ErrSoDViolation returns 403 SOD_VIOLATION.
func ErrSoDViolation(msg string) *DomainError {
	return New(CodeSoDViolation, msg)
}

// ErrStepUpRequired returns 403 STEP_UP_REQUIRED.
func ErrStepUpRequired(action string) *DomainError {
	return New(CodeStepUpRequired,
		fmt.Sprintf("Action '%s' membutuhkan step-up MFA. Hubungi /auth/step-up.", action))
}

// ErrStepUpExpired returns 403 STEP_UP_EXPIRED.
func ErrStepUpExpired() *DomainError {
	return New(CodeStepUpExpired, "Step-up MFA sudah expired (> 5 menit). Ulangi /auth/step-up.")
}

// ErrNotFound returns 404 NOT_FOUND.
func ErrNotFound(entity string) *DomainError {
	return New(CodeNotFound, fmt.Sprintf("%s tidak ditemukan.", entity))
}

// ErrConflict returns 409 CONFLICT (optimistic lock).
func ErrConflict() *DomainError {
	return New(CodeConflict, "Data sudah diubah oleh pengguna lain. Refresh dan ulangi.")
}

// ErrIdempotencyMismatch returns 422 IDEMPOTENCY_MISMATCH.
func ErrIdempotencyMismatch() *DomainError {
	return New(CodeIdempotencyMismatch, "Idempotency-Key sudah dipakai dengan payload berbeda dari request sebelumnya.")
}

// ErrRateLimited returns 429 RATE_LIMITED.
func ErrRateLimited() *DomainError {
	return New(CodeRateLimited, "Terlalu banyak request. Coba lagi dalam 60 detik.")
}

// ErrInternal returns 500 INTERNAL.
func ErrInternal(cause error) *DomainError {
	return Wrap(CodeInternal, "Terjadi kesalahan internal. Hubungi admin dengan traceId.", cause)
}

// ErrMFARequired returns 403 MFA_REQUIRED.
func ErrMFARequired() *DomainError {
	return New(CodeMFARequired, "Action ini membutuhkan verifikasi MFA.")
}

// ErrWorkflowInvalidTransition returns 422 WORKFLOW_INVALID_TRANSITION.
func ErrWorkflowInvalidTransition(from, to string) *DomainError {
	return New(CodeWorkflowInvalidTransition,
		fmt.Sprintf("Transisi dari '%s' ke '%s' tidak valid.", from, to),
		Detail{Field: "state", Rule: "invalid_transition",
			Message: fmt.Sprintf("Transition %s → %s tidak valid", from, to)})
}
