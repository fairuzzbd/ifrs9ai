// detection_doctype_test.go — B2 fix: document-type validation in DetectFromDocument.
//
// Tests (4 cases per compliance finding):
//  1. TestDetectFromDocument_RejectsNonAmendmentDocType     — valuation report (RATING_REPORT) → 422
//  2. TestDetectFromDocument_RejectsNonExistentDocument     — unknown UUID → 422 (NOT_FOUND path)
//  3. TestDetectFromDocument_AcceptsAmendmentDeposito        — KONTRAK category → success
//  4. TestDetectFromDocument_AcceptsAmendmentObligasi        — KONTRAK category (obligasi) → success
//  5. TestDetectFromDocument_DocTypeRepoNil_SkipsValidation  — nil docTypeRepo → validation skipped
//
// Additional:
//  6. TestAllowedAmendmentDocCategories_ContainsKontrak — whitelist sanity check
//
// References:
//   - detection_service.go §DetectFromDocument (B2 fix, step 2)
//   - db/migrations/000006_doc_document.up.sql ck_doc_category
//   - FSD-APP-C §M6-001
//
// package eir (white-box: same package as service under test)
package eir

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── stub document-type repo ──────────────────────────────────────────────────

type stubDocTypeRepo struct {
	docs map[uuid.UUID]string // documentID → category
}

func newStubDocTypeRepo() *stubDocTypeRepo {
	return &stubDocTypeRepo{docs: make(map[uuid.UUID]string)}
}

func (r *stubDocTypeRepo) put(id uuid.UUID, category string) {
	r.docs[id] = category
}

func (r *stubDocTypeRepo) GetDocType(_ context.Context, id uuid.UUID) (string, error) {
	cat, ok := r.docs[id]
	if !ok {
		return "", nil // not found → empty string
	}
	return cat, nil
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestDetectFromDocument_RejectsNonAmendmentDocType asserts that passing a document
// with category "RATING_REPORT" (not in AllowedAmendmentDocCategories) returns
// EIR_AMENDMENT_DETECTION_NO_MATCH (422).
func TestDetectFromDocument_RejectsNonAmendmentDocType(t *testing.T) {
	inst := aclInstrumentAC()
	instrRepo := newStubInstrumenRepo()
	instrRepo.put(InstrumenForEIR{
		ID:                inst.ID,
		KodeInstrumen:     "BOND-001",
		KlasifikasiPsak71: "AC",
		EIRMethodFlag:     true,
		EIRAwal:           inst.EIRAwal,
		Status:            "ACTIVE",
		TenantID:          "TUGURE",
	})
	amendRepo := newStubAmendmentRepo()
	docRepo := newStubDocTypeRepo()
	docID := uuid.New()
	docRepo.put(docID, "RATING_REPORT") // wrong category

	db := newMockDBNoTx(t)
	svc := newDetectionSvc(db, instrRepo, amendRepo)
	svc.WithDocTypeRepo(docRepo)

	_, err := svc.DetectFromDocument(context.Background(), DetectAmendmentRequest{
		InstrumenID:    inst.ID,
		DocumentID:     docID,
		AlasanDetected: "Kontrak diamandemen",
		ActorID:        uuid.New(),
		TenantID:       "TUGURE",
	})
	assertDomainCode(t, err, CodeEIRAmendmentDetectionNoMatch)
}

// TestDetectFromDocument_RejectsNonExistentDocument asserts that a documentID that
// does not exist in doc.document (GetDocType returns "") returns
// EIR_AMENDMENT_DETECTION_NO_MATCH (NOT_FOUND semantic, 422).
func TestDetectFromDocument_RejectsNonExistentDocument(t *testing.T) {
	inst := aclInstrumentAC()
	instrRepo := newStubInstrumenRepo()
	instrRepo.put(InstrumenForEIR{
		ID:                inst.ID,
		KodeInstrumen:     "BOND-001",
		KlasifikasiPsak71: "AC",
		EIRMethodFlag:     true,
		EIRAwal:           inst.EIRAwal,
		Status:            "ACTIVE",
		TenantID:          "TUGURE",
	})
	amendRepo := newStubAmendmentRepo()
	docRepo := newStubDocTypeRepo()
	// docID not in docRepo → GetDocType returns ("", nil)

	db := newMockDBNoTx(t)
	svc := newDetectionSvc(db, instrRepo, amendRepo)
	svc.WithDocTypeRepo(docRepo)

	_, err := svc.DetectFromDocument(context.Background(), DetectAmendmentRequest{
		InstrumenID:    inst.ID,
		DocumentID:     uuid.New(), // not seeded
		AlasanDetected: "Kontrak diamandemen",
		ActorID:        uuid.New(),
		TenantID:       "TUGURE",
	})
	assertDomainCode(t, err, CodeEIRAmendmentDetectionNoMatch)
}

// TestDetectFromDocument_AcceptsAmendmentDeposito asserts that a document with
// category "KONTRAK" (deposito amendment) is accepted and creates a DRAFT proposal.
func TestDetectFromDocument_AcceptsAmendmentDeposito(t *testing.T) {
	inst := aclInstrumentAC()
	instrRepo := newStubInstrumenRepo()
	instrRepo.put(InstrumenForEIR{
		ID:                inst.ID,
		KodeInstrumen:     "DEP-001",
		KlasifikasiPsak71: "AC",
		EIRMethodFlag:     true,
		EIRAwal:           inst.EIRAwal,
		Status:            "ACTIVE",
		TenantID:          "TUGURE",
	})
	amendRepo := newStubAmendmentRepo()
	docRepo := newStubDocTypeRepo()
	docID := uuid.New()
	docRepo.put(docID, "KONTRAK") // amendment contract

	db, mock := newMockDBForDetection(t)
	svc := newDetectionSvc(db, instrRepo, amendRepo)
	svc.WithDocTypeRepo(docRepo)

	proposal, err := svc.DetectFromDocument(context.Background(), DetectAmendmentRequest{
		InstrumenID:    inst.ID,
		DocumentID:     docID,
		AlasanDetected: "Amandemen deposito tenor diperpanjang",
		ActorID:        uuid.New(),
		TenantID:       "TUGURE",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if proposal == nil || proposal.Status != AmendStatusDraft {
		t.Errorf("expected DRAFT proposal, got %v", proposal)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestDetectFromDocument_AcceptsAmendmentObligasi asserts that a KONTRAK document
// for an obligasi (bond, FVOCI) is also accepted.
func TestDetectFromDocument_AcceptsAmendmentObligasi(t *testing.T) {
	eirVal, _ := decimal.NewFromString("0.07500000")
	instrID := uuid.New()
	instrRepo := newStubInstrumenRepo()
	instrRepo.put(InstrumenForEIR{
		ID:                instrID,
		KodeInstrumen:     "OBL-001",
		KlasifikasiPsak71: "FVOCI",
		EIRMethodFlag:     true,
		EIRAwal:           &eirVal,
		Status:            "ACTIVE",
		TenantID:          "TUGURE",
	})
	amendRepo := newStubAmendmentRepo()
	docRepo := newStubDocTypeRepo()
	docID := uuid.New()
	docRepo.put(docID, "KONTRAK")

	db, mock := newMockDBForDetection(t)
	svc := newDetectionSvc(db, instrRepo, amendRepo)
	svc.WithDocTypeRepo(docRepo)

	proposal, err := svc.DetectFromDocument(context.Background(), DetectAmendmentRequest{
		InstrumenID:    instrID,
		DocumentID:     docID,
		AlasanDetected: "Amandemen obligasi suku bunga berubah",
		ActorID:        uuid.New(),
		TenantID:       "TUGURE",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if proposal == nil || proposal.Status != AmendStatusDraft {
		t.Errorf("expected DRAFT proposal, got %v", proposal)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestDetectFromDocument_DocTypeRepoNil_SkipsValidation asserts that when docTypeRepo
// is nil (unit tests / dev mode without doc DB) the validation is silently skipped
// and the proposal is created as before.
func TestDetectFromDocument_DocTypeRepoNil_SkipsValidation(t *testing.T) {
	inst := aclInstrumentAC()
	instrRepo := newStubInstrumenRepo()
	instrRepo.put(InstrumenForEIR{
		ID:                inst.ID,
		KodeInstrumen:     "BOND-001",
		KlasifikasiPsak71: "AC",
		EIRMethodFlag:     true,
		EIRAwal:           inst.EIRAwal,
		Status:            "ACTIVE",
		TenantID:          "TUGURE",
	})
	amendRepo := newStubAmendmentRepo()

	db, mock := newMockDBForDetection(t)
	// svc created without WithDocTypeRepo → docTypeRepo = nil → skip validation.
	svc := newDetectionSvc(db, instrRepo, amendRepo)

	_, err := svc.DetectFromDocument(context.Background(), DetectAmendmentRequest{
		InstrumenID:    inst.ID,
		DocumentID:     uuid.New(),
		AlasanDetected: "Amandemen kontrak deposito",
		ActorID:        uuid.New(),
		TenantID:       "TUGURE",
	})
	if err != nil {
		t.Fatalf("expected success with nil docTypeRepo, got error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestAllowedAmendmentDocCategories_ContainsKontrak ensures the whitelist is set up
// correctly. If someone accidentally removes KONTRAK, all detections will fail.
func TestAllowedAmendmentDocCategories_ContainsKontrak(t *testing.T) {
	if _, ok := AllowedAmendmentDocCategories["KONTRAK"]; !ok {
		t.Error("AllowedAmendmentDocCategories must contain 'KONTRAK'")
	}
}
