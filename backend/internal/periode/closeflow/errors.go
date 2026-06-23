package closeflow

// P5-M4 Periode Close Workflow — convenience error constructors.
// Error codes are defined in internal/common/errors/domain.go
// (CodeClosingChecklistFailed, CodePeriodeSoftClosed, etc.).

import (
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ErrChecklistFailed returns 422 CLOSING_CHECKLIST_FAILED with item-level details.
func ErrChecklistFailed(msg string, details ...domainerrors.Detail) *domainerrors.DomainError {
	return domainerrors.New(domainerrors.CodeClosingChecklistFailed, msg, details...)
}

// ErrChecklistStale returns 422 CLOSING_CHECKLIST_STALE.
func ErrChecklistStale() *domainerrors.DomainError {
	return domainerrors.New(domainerrors.CodeClosingChecklistStale,
		"Checklist sudah stale (melebihi batas waktu). Sistem sedang menjalankan evaluasi ulang.")
}

// ErrPeriodeSoftClosed returns 423 PERIODE_SOFT_CLOSED.
func ErrPeriodeSoftClosed(periodeKode string) *domainerrors.DomainError {
	return domainerrors.New(domainerrors.CodePeriodeSoftClosed,
		"Periode "+periodeKode+" sudah soft-closed. Mutasi tidak diizinkan. "+
			"Hubungi Finance Controller untuk koreksi darurat.")
}

// ErrPeriodeClosed returns 423 PERIODE_CLOSED.
func ErrPeriodeClosed(periodeKode string, tanggalHardClose string) *domainerrors.DomainError {
	return domainerrors.New(domainerrors.CodePeriodeClosed,
		"Periode "+periodeKode+" sudah hard-closed pada "+tanggalHardClose+
			". Mutasi tidak diizinkan. Hubungi CFO untuk reopen (hanya dalam grace window 48 jam).")
}

// ErrMFAStepUpRequired returns 401 MFA_STEP_UP_REQUIRED.
func ErrMFAStepUpRequired(action string) *domainerrors.DomainError {
	return domainerrors.New(domainerrors.CodeMFAStepUpRequired,
		action+" wajib step-up MFA (DEC-027). "+
			"Lakukan challenge via POST /auth/step-up lalu sertakan X-Step-Up-Token di request.")
}

// ErrMFAStepUpExpired returns 401 MFA_STEP_UP_EXPIRED.
func ErrMFAStepUpExpired() *domainerrors.DomainError {
	return domainerrors.New(domainerrors.CodeMFAStepUpExpired,
		"Step-up MFA token sudah expired (maksimal 5 menit). Ulangi step-up challenge via POST /auth/step-up.")
}

// ErrGraceExpired returns 423 PERIODE_GRACE_EXPIRED.
func ErrGraceExpired(periodeKode string, graceExpiredAt string) *domainerrors.DomainError {
	return domainerrors.New(domainerrors.CodePeriodeGraceExpired,
		"Grace window untuk reopen periode "+periodeKode+" telah berakhir pada "+graceExpiredAt+
			". Reopen tidak dapat dilakukan secara otomatis. Ajukan RFC ke Direksi sesuai RACI BRD §3.")
}

// ErrSoftClosePendingExists returns 409 SOFT_CLOSE_PENDING_EXISTS.
func ErrSoftClosePendingExists(periodeKode string) *domainerrors.DomainError {
	return domainerrors.New(domainerrors.CodeSoftClosePendingExists,
		"Sudah ada soft-close request yang menunggu approval untuk periode "+periodeKode+
			". Batalkan request tersebut atau tunggu approval.")
}

// ErrInvalidTransition returns 422 WORKFLOW_INVALID_TRANSITION with a clear message.
func ErrInvalidTransition(msg string) *domainerrors.DomainError {
	return domainerrors.New(domainerrors.CodeWorkflowInvalidTransition, msg)
}

// ErrSoDViolation returns 403 SOD_VIOLATION.
func ErrSoDViolation(msg string) *domainerrors.DomainError {
	return domainerrors.New(domainerrors.CodeSoDViolation, msg)
}

// ErrRowVersionConflict returns 409 CONFLICT (optimistic lock row_version mismatch).
func ErrRowVersionConflict(periodeKode string) *domainerrors.DomainError {
	return domainerrors.New(domainerrors.CodeConflict,
		"Data periode "+periodeKode+" sudah diubah oleh pengguna lain (row_version mismatch). "+
			"Refresh dan coba lagi.")
}

// ErrPeriodeNotFound returns 404 NOT_FOUND for a missing periode buku.
func ErrPeriodeNotFound(id string) *domainerrors.DomainError {
	return domainerrors.New(domainerrors.CodeNotFound,
		"Periode buku tidak ditemukan: "+id)
}
