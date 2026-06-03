package auth

import (
	"fmt"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// WorkflowParticipants menyimpan ID user yang sudah berpartisipasi dalam workflow.
// Field kosong ("") berarti step belum ada.
type WorkflowParticipants struct {
	MakerID    string
	ReviewerID string
	ApproverID string  // 4-eyes approver
	Approver2ID string // 6-eyes second approver
}

// SoDStep adalah langkah workflow yang sedang dicoba.
type SoDStep string

const (
	SoDStepReview   SoDStep = "REVIEW"
	SoDStepApprove  SoDStep = "APPROVE"
	SoDStepApprove2 SoDStep = "APPROVE2"
)

// EnforceSoD memeriksa aturan Segregation of Duties untuk workflow transition.
// Persis implementasi contoh di security-baseline.md.
//
// Rules (DEC-017):
//   - Reviewer TIDAK boleh sama dengan Maker
//   - Approver TIDAK boleh sama dengan Maker ATAU Reviewer
//   - Approver2 (6-eyes) TIDAK boleh sama dengan siapapun di step sebelumnya
//
// Returns *DomainError SOD_VIOLATION jika ada pelanggaran, nil jika OK.
func EnforceSoD(participants WorkflowParticipants, currentUserID string, step SoDStep) error {
	switch step {
	case SoDStepReview:
		if participants.MakerID != "" && participants.MakerID == currentUserID {
			return domainerrors.New(
				domainerrors.CodeSoDViolation,
				fmt.Sprintf("Anda tidak bisa menjadi reviewer untuk transaksi yang Anda buat sendiri. "+
					"Maker dan reviewer harus user berbeda (DEC-017)."),
			)
		}

	case SoDStepApprove:
		if participants.MakerID != "" && participants.MakerID == currentUserID {
			return domainerrors.New(
				domainerrors.CodeSoDViolation,
				"Anda tidak bisa menjadi approver untuk transaksi yang Anda buat sendiri (DEC-017).",
			)
		}
		if participants.ReviewerID != "" && participants.ReviewerID == currentUserID {
			return domainerrors.New(
				domainerrors.CodeSoDViolation,
				"Anda tidak bisa menjadi approver untuk transaksi yang Anda review sendiri (DEC-017).",
			)
		}

	case SoDStepApprove2:
		// 6-eyes: approver2 tidak boleh sama dengan siapapun di step sebelumnya.
		if participants.MakerID != "" && participants.MakerID == currentUserID {
			return domainerrors.New(
				domainerrors.CodeSoDApprover1SameAsMaker,
				"Approver2 tidak bisa sama dengan maker (6-eyes, DEC-017).",
			)
		}
		if participants.ReviewerID != "" && participants.ReviewerID == currentUserID {
			return domainerrors.New(
				domainerrors.CodeSoDApprover2SameAsReviewer,
				"Approver2 tidak bisa sama dengan reviewer (6-eyes, DEC-017).",
			)
		}
		if participants.ApproverID != "" && participants.ApproverID == currentUserID {
			return domainerrors.New(
				domainerrors.CodeSoDViolation,
				"Approver2 tidak bisa sama dengan approver pertama (6-eyes, DEC-017).",
			)
		}
	}

	return nil
}

// MustEnforceSoD adalah versi yang wrap error ke fmt.Errorf untuk service layer.
// Service wajib call ini sebelum workflow transition.
func MustEnforceSoD(participants WorkflowParticipants, currentUserID string, step SoDStep) error {
	if err := EnforceSoD(participants, currentUserID, step); err != nil {
		return fmt.Errorf("sod check failed: %w", err)
	}
	return nil
}
