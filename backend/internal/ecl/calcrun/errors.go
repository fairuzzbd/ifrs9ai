package calcrun

import "net/http"

// errors.go — 17 domain error codes per state machine doc §6 (p4-m8-calc-run.md).
// All codes map to stable HTTP status codes for the error envelope (api-conventions.md).
// Sentinel errors satisfy the `error` interface and expose Code() for handler mapping.

// ─── Domain error type ────────────────────────────────────────────────────────

// Error is the exported domain error type for the calcrun package.
// All Err* constructors return the error interface; use IsCalcRunError to
// unwrap and access Code()/HTTPStatus() when needed.
type Error struct {
	code    string
	message string
	http    int
}

func (e *Error) Error() string { return e.code + ": " + e.message }

// Code returns the stable error code string (e.g. "CALC_RUN_NOT_FOUND").
func (e *Error) Code() string { return e.code }

// HTTPStatus returns the HTTP status code for this error.
func (e *Error) HTTPStatus() int {
	if e.http == 0 {
		return http.StatusInternalServerError
	}
	return e.http
}

func domainErr(code, message string, httpStatus int) *Error {
	return &Error{code: code, message: message, http: httpStatus}
}

// ─── Error constructors (17 codes per state machine §6) ──────────────────────

// ErrCalcRunNotFound — HTTP 404. calc_run UUID does not exist.
func ErrCalcRunNotFound(id string) error {
	return domainErr("CALC_RUN_NOT_FOUND", "Calc run "+id+" tidak ditemukan.", http.StatusNotFound)
}

// ErrCalcRunInvalidTransition — HTTP 422. Invalid status machine transition.
func ErrCalcRunInvalidTransition(currentStatus, attempted, detail string) error {
	return domainErr("CALC_RUN_INVALID_TRANSITION",
		"Calc run tidak bisa di-"+attempted+": status saat ini "+currentStatus+". "+detail,
		http.StatusUnprocessableEntity)
}

// ErrCalcRunDuplicateInProgress — HTTP 409. Another calc_run is IN_PROGRESS for the same periode.
func ErrCalcRunDuplicateInProgress(periodeID, existingID string) error {
	return domainErr("CALC_RUN_DUPLICATE_IN_PROGRESS",
		"Sudah ada calc run "+existingID+" sedang berjalan untuk periode "+periodeID+
			". Tunggu hingga selesai atau cancel terlebih dahulu.",
		http.StatusConflict)
}

// ErrCalcRunPeriodeAlreadySealed — HTTP 409. A SEALED calc_run exists for the same periode.
func ErrCalcRunPeriodeAlreadySealed(periodeID, sealedID string) error {
	return domainErr("CALC_RUN_PERIODE_ALREADY_SEALED",
		"Periode "+periodeID+" sudah memiliki calc run yang di-seal ("+sealedID+
			"). Override memerlukan persetujuan ALCO — fitur ini belum tersedia (backlog).",
		http.StatusConflict)
}

// ErrCalcRunPeriodeHardClosed — HTTP 422 (PERIODE_CLOSED maps to 423 per common errors).
// We use 422 here per state machine §6; handler maps to 423 via common error codes.
func ErrCalcRunPeriodeHardClosed(periodeID string) error {
	return domainErr("CALC_RUN_PERIODE_HARD_CLOSED",
		"Periode "+periodeID+" sudah hard-closed, tidak bisa membuat atau start calc run baru.",
		http.StatusLocked)
}

// ErrCalcRunParameterSnapshotInvalid — HTTP 422. One or more ECL params not APPROVED.
func ErrCalcRunParameterSnapshotInvalid(detail string) error {
	return domainErr("CALC_RUN_PARAMETER_SNAPSHOT_INVALID",
		"Parameter ECL belum tersedia atau belum APPROVED untuk periode ini: "+detail,
		http.StatusUnprocessableEntity)
}

// ErrCalcRunSealRequiresCompleted — HTTP 422. Seal request needs COMPLETED status.
func ErrCalcRunSealRequiresCompleted(currentStatus string) error {
	return domainErr("CALC_RUN_SEAL_REQUIRES_COMPLETED",
		"Seal request membutuhkan status COMPLETED, saat ini: "+currentStatus+".",
		http.StatusUnprocessableEntity)
}

// ErrCalcRunSealNotRequested — HTTP 422. Seal approve/reject needs SEAL_REQUESTED status.
func ErrCalcRunSealNotRequested(currentStatus string) error {
	return domainErr("CALC_RUN_SEAL_NOT_REQUESTED",
		"Seal approve/reject membutuhkan status SEAL_REQUESTED, saat ini: "+currentStatus+".",
		http.StatusUnprocessableEntity)
}

// ErrCalcRunSealSoDViolation — HTTP 403. Approver is the same person as the requester.
func ErrCalcRunSealSoDViolation(actorID string) error {
	return domainErr("CALC_RUN_SEAL_SOD_VIOLATION",
		"Maker ("+actorID+") tidak boleh menjadi approver untuk seal yang sama (4-eyes SoD).",
		http.StatusForbidden)
}

// ErrCalcRunSealStepUpRequired — HTTP 403. Step-up MFA token missing or stale (DEC-027).
func ErrCalcRunSealStepUpRequired() error {
	return domainErr("CALC_RUN_SEAL_STEP_UP_REQUIRED",
		"Seal approve membutuhkan step-up MFA yang masih valid (≤5 menit). Lakukan re-autentikasi MFA.",
		http.StatusForbidden)
}

// ErrCalcRunHasErrors — HTTP 422. Seal request blocked because error_count > 0.
func ErrCalcRunHasErrors(errorCount int) error {
	return domainErr("CALC_RUN_HAS_ERRORS",
		"Calc run memiliki instrumen dengan error. Perbaiki data dan re-run sebelum seal.",
		http.StatusUnprocessableEntity)
}

// ErrCalcRunCancelReasonTooShort — HTTP 422. cancel_reason < 30 chars.
func ErrCalcRunCancelReasonTooShort() error {
	return domainErr("CALC_RUN_CANCEL_REASON_TOO_SHORT",
		"cancel_reason harus minimal 30 karakter.",
		http.StatusUnprocessableEntity)
}

// ErrCalcRunCancelAfterCompleted — HTTP 422. Cannot cancel COMPLETED or SEALED run.
func ErrCalcRunCancelAfterCompleted(currentStatus string) error {
	return domainErr("CALC_RUN_CANCEL_AFTER_COMPLETED",
		"Calc run dengan status "+currentStatus+" tidak bisa dibatalkan. Hanya DRAFT dan IN_PROGRESS.",
		http.StatusUnprocessableEntity)
}

// ErrECLParamNotFound — HTTP 422. ECL parameter not found or not APPROVED.
func ErrECLParamNotFound(param string) error {
	return domainErr("ECL_PARAM_NOT_FOUND",
		"Parameter ECL ("+param+") tidak ditemukan atau belum disetujui ALCO.",
		http.StatusUnprocessableEntity)
}

// ErrFXRateNotFound — HTTP 422. BI JISDOR rate not available for evaluation_date.
func ErrFXRateNotFound(date string) error {
	return domainErr("FX_RATE_NOT_FOUND",
		"Kurs BI JISDOR untuk tanggal "+date+" tidak tersedia. Upload kurs terlebih dahulu.",
		http.StatusUnprocessableEntity)
}

// ErrCalcRunForbiddenNotMaker — HTTP 403. Only the creator (maker) can cancel a calc_run.
// Uses specific code CALC_RUN_FORBIDDEN_NOT_MAKER (not generic FORBIDDEN) per api-conventions.md.
func ErrCalcRunForbiddenNotMaker(creatorID string) error {
	return domainErr("CALC_RUN_FORBIDDEN_NOT_MAKER",
		"Hanya creator calc run ("+creatorID+") yang dapat membatalkannya.",
		http.StatusForbidden)
}

// ErrCalcRunSealed — HTTP 423. Sealed calc_run is immutable; no further mutations.
func ErrCalcRunSealed() error {
	return domainErr("ECL_PARAM_FROZEN",
		"Calc run sudah di-seal dan bersifat immutable. Tidak ada modifikasi yang diizinkan.",
		http.StatusLocked)
}

// ─── Helper: is calcrun Error ─────────────────────────────────────────────────

// IsCalcRunError checks if err is a *Error (calcrun domain error).
// Returns the *Error and true on match, nil and false otherwise.
func IsCalcRunError(err error) (*Error, bool) {
	if ce, ok := err.(*Error); ok {
		return ce, true
	}
	return nil, false
}
