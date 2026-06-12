// detection_idempotency_test.go — B3 fix: (document_id, instrumen_id) idempotency.
//
// Tests:
//  1. TestDetectFromDocument_DuplicateDocumentInstrumen_ReturnsExisting
//     — when amendRepo.Create signals a unique violation (23505), service
//     fetches the existing proposal and returns it (200 idempotent semantics).
//  2. TestGetByDocumentAndInstrumen_StubReturnsExisting
//     — unit test for the stub's GetByDocumentAndInstrumen helper.
//
// Note: PG error 23505 is exercised via a custom stubAmendmentRepoWithDup that
// returns a pq.Error with Code="23505" on Create. The real constraint is enforced
// by migration 000028 at DB level; this unit test covers the service-layer handling.
//
// References:
//   - detection_service.go §DetectFromDocument (B3 fix, Create error handling)
//   - db/migrations/000028_amendment_detection_idempotency.up.sql
//   - DEC-021 (idempotency mandatory).
package eir

import (
	"context"
	"database/sql"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// stubAmendmentRepoWithDup wraps stubAmendmentRepo and returns a pq.Error 23505
// on the first Create call, then stores the proposal for subsequent GetByDocumentAndInstrumen.
type stubAmendmentRepoWithDup struct {
	*stubAmendmentRepo
	existingProposal *AmendmentProposal
}

func (r *stubAmendmentRepoWithDup) Create(_ context.Context, _ *sql.Tx, _ *AmendmentProposal) error {
	// Simulate PG unique_violation for (document_id, instrumen_id).
	return &pq.Error{Code: "23505", Message: "duplicate key value violates unique constraint"}
}

func (r *stubAmendmentRepoWithDup) GetByDocumentAndInstrumen(_ context.Context, docID uuid.UUID, instrID uuid.UUID) (*AmendmentProposal, error) {
	if r.existingProposal != nil &&
		r.existingProposal.DocumentID != nil &&
		*r.existingProposal.DocumentID == docID &&
		r.existingProposal.InstrumenID == instrID {
		cp := *r.existingProposal
		return &cp, nil
	}
	return nil, nil
}

// TestDetectFromDocument_DuplicateDocumentInstrumen_ReturnsExisting exercises the
// 23505-idempotent path: when the DB unique index fires, DetectFromDocument should
// return the existing proposal rather than a 409 error.
func TestDetectFromDocument_DuplicateDocumentInstrumen_ReturnsExisting(t *testing.T) {
	inst := aclInstrumentAC()
	instrRepo := newStubInstrumenRepo()
	instrRepo.put(InstrumenForEIR{
		ID:                inst.ID,
		KodeInstrumen:     "BOND-DUP",
		KlasifikasiPsak71: "AC",
		EIRMethodFlag:     true,
		EIRAwal:           inst.EIRAwal,
		Status:            "ACTIVE",
		TenantID:          "TUGURE",
	})

	docID := uuid.New()
	makerID := uuid.New()
	existingProposal := makeProposalM6(uuid.New(), AmendStatusDraft, makerID)
	existingProposal.InstrumenID = inst.ID
	existingProposal.DocumentID = &docID
	existingProposal.TriggerSource = AmendTriggerDocumentUpload

	dupRepo := &stubAmendmentRepoWithDup{
		stubAmendmentRepo: newStubAmendmentRepo(),
		existingProposal:  existingProposal,
	}

	// After 23505, the service calls tx.Rollback() then reads the existing proposal
	// without a new tx. So mock needs Begin + Rollback only.
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectRollback()

	svc := newDetectionSvc(db, instrRepo, dupRepo)

	result, svcErr := svc.DetectFromDocument(context.Background(), DetectAmendmentRequest{
		InstrumenID:    inst.ID,
		DocumentID:     docID,
		AlasanDetected: "Amandemen kontrak duplikat idempotent",
		ActorID:        makerID,
		TenantID:       "TUGURE",
	})
	if svcErr != nil {
		t.Fatalf("expected idempotent success, got error: %v", svcErr)
	}
	if result == nil {
		t.Fatal("expected existing proposal returned, got nil")
	}
	if result.ID != existingProposal.ID {
		t.Errorf("expected existing proposal ID %s, got %s", existingProposal.ID, result.ID)
	}
	if result.Status != AmendStatusDraft {
		t.Errorf("expected DRAFT, got %s", result.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestGetByDocumentAndInstrumen_StubReturnsExisting verifies the stub helper used
// in tests correctly returns the seeded proposal for matching (docID, instrID).
func TestGetByDocumentAndInstrumen_StubReturnsExisting(t *testing.T) {
	docID := uuid.New()
	makerID := uuid.New()
	instrID := uuid.New()

	proposal := makeProposalM6(uuid.New(), AmendStatusDraft, makerID)
	proposal.InstrumenID = instrID
	proposal.DocumentID = &docID

	repo := newStubAmendmentRepo()
	repo.proposals[proposal.ID] = proposal

	result, err := repo.GetByDocumentAndInstrumen(context.Background(), docID, instrID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected proposal, got nil")
	}
	if result.ID != proposal.ID {
		t.Errorf("expected %s, got %s", proposal.ID, result.ID)
	}

	// Non-matching documentID should return nil.
	result2, err2 := repo.GetByDocumentAndInstrumen(context.Background(), uuid.New(), instrID)
	if err2 != nil {
		t.Fatalf("unexpected error: %v", err2)
	}
	if result2 != nil {
		t.Error("expected nil for non-matching documentID, got proposal")
	}
}
