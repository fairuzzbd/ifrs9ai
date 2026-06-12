// detection_service_test.go — unit tests for DetectionService (P4-M6 M6-001, M6-005).
//
// DetectFromDocument and CancelAmendment: amendRepo.Create/Cancel go through stubs
// (in-memory); auditWriter is stubbed. The service still calls db.BeginTx + tx.Commit
// — only Begin+Commit expectations are needed; no ExecContext on the DB path.
//
// UpdateCashflows: does a direct tx.ExecContext (UPDATE ecl.eir_reestimation_log) —
// needs Begin + Exec + Commit expectations.
//
// Error assertions use assertDomainCode (defined below) — compares de.Code() string
// rather than err.Error() message text, which is human-readable Bahasa Indonesia.
//
// References: FSD-APP-C §M6-001, §M6-005; DEC-016, DEC-017.
package eir

import (
	"context"
	"database/sql"
	"log/slog"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func newDetectionSvc(db *sql.DB, instrRepo InstrumenEIRRepoIface, amendRepo AmendmentRepoIface) *DetectionService {
	return NewDetectionService(db, instrRepo, amendRepo, stubAuditW(), slog.Default())
}

func stubAuditW() *stubAuditWriter { return &stubAuditWriter{} }

// makeProposalM6 creates a proposal for M6 tests — note service_test.go's makeProposal has different sig.
func makeProposalM6(id uuid.UUID, status AmendmentStatus, makerID uuid.UUID) *AmendmentProposal {
	instrID := uuid.New()
	cfJSON := `[{"date":"2026-01-01T00:00:00Z","amount_idr":"-1000000.0000"},{"date":"2026-07-01T00:00:00Z","amount_idr":"1050000.0000"}]`
	eirLama := decimal.NewFromFloat(0.08)
	return &AmendmentProposal{
		ID:                  id,
		InstrumenID:         instrID,
		Status:              status,
		TanggalAmandemen:    time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		TanggalReEstimasi:   time.Now(),
		AlasanAmandemen:     "test amendment m6",
		EIRLama:             &eirLama,
		RevisedCashflowJSON: cfJSON,
		MakerID:             &makerID,
		TenantID:            "TUGURE",
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		CreatedBy:           makerID,
		UpdatedBy:           makerID,
	}
}

func aclInstrumentAC() InstrumenForEIR {
	eirVal := decimal.NewFromFloat(0.08028915)
	id := uuid.New()
	return InstrumenForEIR{
		ID:                id,
		KodeInstrumen:     "BOND-001",
		KlasifikasiPsak71: "AC",
		EIRMethodFlag:     true,
		EIRAwal:           &eirVal,
		Nominal:           decimal.NewFromInt(1_000_000_000),
		TanggalPenempatan: time.Now().Add(-365 * 24 * time.Hour),
		Status:            "ACTIVE",
		TenantID:          "TUGURE",
	}
}

// assertDomainCode checks err is a DomainError with the expected code string.
// Uses de.Code() not err.Error() — avoids coupling tests to Bahasa Indonesia messages.
func assertDomainCode(t *testing.T, err error, wantCode string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %s, got nil", wantCode)
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError with code %s, got: %T %v", wantCode, err, err)
	}
	if string(de.Code()) != wantCode {
		t.Errorf("expected code %s, got %s (message: %s)", wantCode, de.Code(), de.Error())
	}
}

// newMockDBForDetection returns a mock that expects Begin+Commit only.
// Used for DetectFromDocument and CancelAmendment where stub repos bypass Exec.
func newMockDBForDetection(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectCommit()
	t.Cleanup(func() { db.Close() })
	return db, mock
}

// newMockDBNoTx returns a mock with no expectations (used for error-before-BeginTx paths).
func newMockDBNoTx(t *testing.T) *sql.DB {
	t.Helper()
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// ─── DetectFromDocument ────────────────────────────────────────────────────────

func TestDetectFromDocument_Success(t *testing.T) {
	inst := aclInstrumentAC()
	instrRepo := newStubInstrumenRepo()
	instrRepo.put(InstrumenForEIR{
		ID:                inst.ID,
		KodeInstrumen:     inst.KodeInstrumen,
		KlasifikasiPsak71: "AC",
		EIRMethodFlag:     true,
		EIRAwal:           inst.EIRAwal,
		Nominal:           inst.Nominal,
		TanggalPenempatan: inst.TanggalPenempatan,
		Status:            "ACTIVE",
		TenantID:          "TUGURE",
	})
	amendRepo := newStubAmendmentRepo()
	db, mock := newMockDBForDetection(t)

	svc := newDetectionSvc(db, instrRepo, amendRepo)
	docID := uuid.New()

	proposal, err := svc.DetectFromDocument(context.Background(), DetectAmendmentRequest{
		InstrumenID:    inst.ID,
		DocumentID:     docID,
		AlasanDetected: "Kontrak diamandemen pada 2026-06-01",
		ActorID:        uuid.New(),
		TenantID:       "TUGURE",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if proposal == nil {
		t.Fatal("expected proposal, got nil")
	}
	if proposal.Status != AmendStatusDraft {
		t.Errorf("expected DRAFT, got %s", proposal.Status)
	}
	if proposal.TriggerSource != AmendTriggerDocumentUpload {
		t.Errorf("expected DOCUMENT_UPLOAD, got %s", proposal.TriggerSource)
	}
	if proposal.DocumentID == nil || *proposal.DocumentID != docID {
		t.Error("documentId not set correctly")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

func TestDetectFromDocument_InstrumenNotFound(t *testing.T) {
	instrRepo := newStubInstrumenRepo()
	amendRepo := newStubAmendmentRepo()
	// Error returned before BeginTx — no tx expectations.
	db := newMockDBNoTx(t)

	svc := newDetectionSvc(db, instrRepo, amendRepo)
	_, err := svc.DetectFromDocument(context.Background(), DetectAmendmentRequest{
		InstrumenID: uuid.New(),
		DocumentID:  uuid.New(),
		ActorID:     uuid.New(),
		TenantID:    "TUGURE",
	})
	assertDomainCode(t, err, CodeEIRInstrumenNotFound)
}

func TestDetectFromDocument_FVTPLIneligible(t *testing.T) {
	instrRepo := newStubInstrumenRepo()
	fvtplID := uuid.New()
	instrRepo.put(InstrumenForEIR{
		ID:                fvtplID,
		KodeInstrumen:     "SAHAM-001",
		KlasifikasiPsak71: "FVTPL",
		EIRMethodFlag:     false,
		Status:            "ACTIVE",
		TenantID:          "TUGURE",
	})
	amendRepo := newStubAmendmentRepo()
	db := newMockDBNoTx(t)

	svc := newDetectionSvc(db, instrRepo, amendRepo)
	_, err := svc.DetectFromDocument(context.Background(), DetectAmendmentRequest{
		InstrumenID: fvtplID,
		DocumentID:  uuid.New(),
		ActorID:     uuid.New(),
		TenantID:    "TUGURE",
	})
	assertDomainCode(t, err, CodeEIRAmendmentDetectionNoMatch)
}

func TestDetectFromDocument_ActiveProposalExists(t *testing.T) {
	inst := aclInstrumentAC()
	instrRepo := newStubInstrumenRepo()
	instrRepo.put(InstrumenForEIR{
		ID: inst.ID, KodeInstrumen: "BOND-001",
		KlasifikasiPsak71: "AC", EIRMethodFlag: true,
		EIRAwal: inst.EIRAwal, Status: "ACTIVE", TenantID: "TUGURE",
	})

	amendRepo := newStubAmendmentRepo()
	// Seed an existing active proposal.
	amendRepo.activeForID[inst.ID] = true

	db := newMockDBNoTx(t)

	svc := newDetectionSvc(db, instrRepo, amendRepo)
	_, err := svc.DetectFromDocument(context.Background(), DetectAmendmentRequest{
		InstrumenID: inst.ID,
		DocumentID:  uuid.New(),
		ActorID:     uuid.New(),
		TenantID:    "TUGURE",
	})
	assertDomainCode(t, err, CodeEIRAmendmentDetectionNoMatch)
}

// ─── CancelAmendment ──────────────────────────────────────────────────────────

func TestCancelAmendment_DraftSuccess(t *testing.T) {
	makerID := uuid.New()
	proposal := makeProposalM6(uuid.New(), AmendStatusDraft, makerID)
	amendRepo := newStubAmendmentRepo()
	amendRepo.proposals[proposal.ID] = proposal

	db, mock := newMockDBForDetection(t)
	instrRepo := newStubInstrumenRepo()
	svc := newDetectionSvc(db, instrRepo, amendRepo)

	result, err := svc.CancelAmendment(context.Background(), CancelAmendmentRequest{
		AmendmentID:  proposal.ID,
		CancelReason: "Pembatalan karena perubahan regulasi terbaru",
		ActorID:      makerID,
		TenantID:     "TUGURE",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != AmendStatusCancelled {
		t.Errorf("expected CANCELLED, got %s", result.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

func TestCancelAmendment_PendingReviewNoReviewer_Success(t *testing.T) {
	makerID := uuid.New()
	proposal := makeProposalM6(uuid.New(), AmendStatusPendingReview, makerID)
	proposal.ReviewerID = nil // reviewer not yet signed

	amendRepo := newStubAmendmentRepo()
	amendRepo.proposals[proposal.ID] = proposal

	db, mock := newMockDBForDetection(t)
	instrRepo := newStubInstrumenRepo()
	svc := newDetectionSvc(db, instrRepo, amendRepo)

	result, err := svc.CancelAmendment(context.Background(), CancelAmendmentRequest{
		AmendmentID:  proposal.ID,
		CancelReason: "Amandemen dibatalkan karena data baru diterima",
		ActorID:      makerID,
		TenantID:     "TUGURE",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != AmendStatusCancelled {
		t.Errorf("expected CANCELLED, got %s", result.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

func TestCancelAmendment_PendingReviewWithReviewer_Forbidden(t *testing.T) {
	makerID := uuid.New()
	reviewerID := uuid.New()
	proposal := makeProposalM6(uuid.New(), AmendStatusPendingReview, makerID)
	proposal.ReviewerID = &reviewerID // reviewer already signed

	amendRepo := newStubAmendmentRepo()
	amendRepo.proposals[proposal.ID] = proposal

	// Error returned before BeginTx — no tx expectations.
	db := newMockDBNoTx(t)
	instrRepo := newStubInstrumenRepo()
	svc := newDetectionSvc(db, instrRepo, amendRepo)

	_, err := svc.CancelAmendment(context.Background(), CancelAmendmentRequest{
		AmendmentID:  proposal.ID,
		CancelReason: "Pembatalan karena perubahan regulasi terbaru",
		ActorID:      makerID,
		TenantID:     "TUGURE",
	})
	assertDomainCode(t, err, CodeEIRAmendmentCancelForbidden)
}

func TestCancelAmendment_PendingApproval_InvalidTransition(t *testing.T) {
	makerID := uuid.New()
	proposal := makeProposalM6(uuid.New(), AmendStatusPendingApproval, makerID)
	amendRepo := newStubAmendmentRepo()
	amendRepo.proposals[proposal.ID] = proposal

	db := newMockDBNoTx(t)
	instrRepo := newStubInstrumenRepo()
	svc := newDetectionSvc(db, instrRepo, amendRepo)

	_, err := svc.CancelAmendment(context.Background(), CancelAmendmentRequest{
		AmendmentID:  proposal.ID,
		CancelReason: "Pembatalan karena perubahan regulasi terbaru",
		ActorID:      makerID,
		TenantID:     "TUGURE",
	})
	assertDomainCode(t, err, CodeEIRAmendInvalidTransition)
}

func TestCancelAmendment_NotMaker_Forbidden(t *testing.T) {
	makerID := uuid.New()
	otherID := uuid.New()
	proposal := makeProposalM6(uuid.New(), AmendStatusDraft, makerID)
	amendRepo := newStubAmendmentRepo()
	amendRepo.proposals[proposal.ID] = proposal

	db := newMockDBNoTx(t)
	instrRepo := newStubInstrumenRepo()
	svc := newDetectionSvc(db, instrRepo, amendRepo)

	_, err := svc.CancelAmendment(context.Background(), CancelAmendmentRequest{
		AmendmentID:  proposal.ID,
		CancelReason: "Pembatalan karena perubahan regulasi terbaru",
		ActorID:      otherID, // not the maker
		TenantID:     "TUGURE",
	})
	assertDomainCode(t, err, CodeEIRAmendmentCancelForbidden)
}

func TestCancelAmendment_ReasonTooShort(t *testing.T) {
	makerID := uuid.New()
	proposal := makeProposalM6(uuid.New(), AmendStatusDraft, makerID)
	amendRepo := newStubAmendmentRepo()
	amendRepo.proposals[proposal.ID] = proposal

	// reason length checked first — no DB call at all.
	db := newMockDBNoTx(t)
	instrRepo := newStubInstrumenRepo()
	svc := newDetectionSvc(db, instrRepo, amendRepo)

	_, err := svc.CancelAmendment(context.Background(), CancelAmendmentRequest{
		AmendmentID:  proposal.ID,
		CancelReason: "short", // < 20 chars
		ActorID:      makerID,
		TenantID:     "TUGURE",
	})
	assertDomainCode(t, err, CodeEIRAmendmentCancelReasonShort)
}

func TestCancelAmendment_AlreadyCancelled_InvalidTransition(t *testing.T) {
	makerID := uuid.New()
	proposal := makeProposalM6(uuid.New(), AmendStatusCancelled, makerID)
	amendRepo := newStubAmendmentRepo()
	amendRepo.proposals[proposal.ID] = proposal

	db := newMockDBNoTx(t)
	instrRepo := newStubInstrumenRepo()
	svc := newDetectionSvc(db, instrRepo, amendRepo)

	_, err := svc.CancelAmendment(context.Background(), CancelAmendmentRequest{
		AmendmentID:  proposal.ID,
		CancelReason: "Pembatalan karena perubahan regulasi terbaru",
		ActorID:      makerID,
		TenantID:     "TUGURE",
	})
	assertDomainCode(t, err, CodeEIRAmendInvalidTransition)
}

func TestCancelAmendment_ApprovedTerminal_InvalidTransition(t *testing.T) {
	makerID := uuid.New()
	proposal := makeProposalM6(uuid.New(), AmendStatusApproved, makerID)
	amendRepo := newStubAmendmentRepo()
	amendRepo.proposals[proposal.ID] = proposal

	db := newMockDBNoTx(t)
	instrRepo := newStubInstrumenRepo()
	svc := newDetectionSvc(db, instrRepo, amendRepo)

	_, err := svc.CancelAmendment(context.Background(), CancelAmendmentRequest{
		AmendmentID:  proposal.ID,
		CancelReason: "Pembatalan karena perubahan regulasi terbaru",
		ActorID:      makerID,
		TenantID:     "TUGURE",
	})
	assertDomainCode(t, err, CodeEIRAmendInvalidTransition)
}

func TestCancelAmendment_NotFound(t *testing.T) {
	// amendRepo is empty → proposal == nil → ErrEIRAmendNotFound.
	amendRepo := newStubAmendmentRepo()
	db := newMockDBNoTx(t)
	svc := newDetectionSvc(db, newStubInstrumenRepo(), amendRepo)

	_, err := svc.CancelAmendment(context.Background(), CancelAmendmentRequest{
		AmendmentID:  uuid.New(),
		CancelReason: "Pembatalan karena tidak diperlukan lagi",
		ActorID:      uuid.New(),
		TenantID:     "TUGURE",
	})
	assertDomainCode(t, err, CodeEIRAmendNotFound)
}

func TestCancelAmendment_NilMakerID_Forbidden(t *testing.T) {
	// proposal.MakerID = nil → SoD branch `proposal.MakerID == nil`.
	proposal := makeProposalM6(uuid.New(), AmendStatusDraft, uuid.New())
	proposal.MakerID = nil // force nil
	amendRepo := newStubAmendmentRepo()
	amendRepo.proposals[proposal.ID] = proposal

	db := newMockDBNoTx(t)
	svc := newDetectionSvc(db, newStubInstrumenRepo(), amendRepo)

	_, err := svc.CancelAmendment(context.Background(), CancelAmendmentRequest{
		AmendmentID:  proposal.ID,
		CancelReason: "Pembatalan diperlukan karena kondisi pasar",
		ActorID:      uuid.New(),
		TenantID:     "TUGURE",
	})
	assertDomainCode(t, err, CodeEIRAmendmentCancelForbidden)
}

// ─── UpdateCashflows ───────────────────────────────────────────────────────────

func TestUpdateCashflows_DraftSuccess(t *testing.T) {
	makerID := uuid.New()
	proposal := makeProposalM6(uuid.New(), AmendStatusDraft, makerID)
	amendRepo := newStubAmendmentRepo()
	amendRepo.proposals[proposal.ID] = proposal

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// UpdateCashflows does a direct tx.ExecContext for the SQL UPDATE.
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE ecl.eir_reestimation_log")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	instrRepo := newStubInstrumenRepo()
	svc := newDetectionSvc(db, instrRepo, amendRepo)

	cfs := obligasiAtDiscount2()
	result, err := svc.UpdateCashflows(context.Background(), UpdateCashflowsRequest{
		AmendmentID:      proposal.ID,
		RevisedCashflows: cfs,
		ActorID:          makerID,
		TenantID:         "TUGURE",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RevisedCashflowJSON == "" {
		t.Error("expected cashflowJSON to be populated")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

func TestUpdateCashflows_NotDraft_Rejected(t *testing.T) {
	makerID := uuid.New()
	proposal := makeProposalM6(uuid.New(), AmendStatusPendingReview, makerID)
	amendRepo := newStubAmendmentRepo()
	amendRepo.proposals[proposal.ID] = proposal

	// Error returned before BeginTx.
	db := newMockDBNoTx(t)
	instrRepo := newStubInstrumenRepo()
	svc := newDetectionSvc(db, instrRepo, amendRepo)

	_, err := svc.UpdateCashflows(context.Background(), UpdateCashflowsRequest{
		AmendmentID:      proposal.ID,
		RevisedCashflows: obligasiAtDiscount2(),
		ActorID:          makerID,
		TenantID:         "TUGURE",
	})
	assertDomainCode(t, err, CodeEIRAmendInvalidTransition)
}

func TestUpdateCashflows_SoDViolation(t *testing.T) {
	// MakerID = X, but actor = Y → SoD error.
	makerID := uuid.New()
	otherActor := uuid.New()
	proposal := makeProposalM6(uuid.New(), AmendStatusDraft, makerID)
	amendRepo := newStubAmendmentRepo()
	amendRepo.proposals[proposal.ID] = proposal

	db := newMockDBNoTx(t)
	svc := newDetectionSvc(db, newStubInstrumenRepo(), amendRepo)

	_, err := svc.UpdateCashflows(context.Background(), UpdateCashflowsRequest{
		AmendmentID:      proposal.ID,
		RevisedCashflows: obligasiAtDiscount2(),
		ActorID:          otherActor,
		TenantID:         "TUGURE",
	})
	assertDomainCode(t, err, CodeEIRAmendmentCancelForbidden)
}

func TestUpdateCashflows_EmptyCashflows(t *testing.T) {
	makerID := uuid.New()
	proposal := makeProposalM6(uuid.New(), AmendStatusDraft, makerID)
	amendRepo := newStubAmendmentRepo()
	amendRepo.proposals[proposal.ID] = proposal

	db := newMockDBNoTx(t)
	svc := newDetectionSvc(db, newStubInstrumenRepo(), amendRepo)

	_, err := svc.UpdateCashflows(context.Background(), UpdateCashflowsRequest{
		AmendmentID:      proposal.ID,
		RevisedCashflows: []CashflowItem{},
		ActorID:          makerID,
		TenantID:         "TUGURE",
	})
	assertDomainCode(t, err, CodeEIRCashflowInvalid)
}

func TestUpdateCashflows_WrongSign(t *testing.T) {
	// CF[0] must be negative; positive → ErrEIRCashflowSignMismatch.
	makerID := uuid.New()
	proposal := makeProposalM6(uuid.New(), AmendStatusDraft, makerID)
	amendRepo := newStubAmendmentRepo()
	amendRepo.proposals[proposal.ID] = proposal

	db := newMockDBNoTx(t)
	svc := newDetectionSvc(db, newStubInstrumenRepo(), amendRepo)

	badCFs := []CashflowItem{
		{Date: time.Now(), AmountIDR: decimal.NewFromFloat(1000000)}, // positive outflow — wrong
		{Date: time.Now().AddDate(1, 0, 0), AmountIDR: decimal.NewFromFloat(1080000)},
	}
	_, err := svc.UpdateCashflows(context.Background(), UpdateCashflowsRequest{
		AmendmentID:      proposal.ID,
		RevisedCashflows: badCFs,
		ActorID:          makerID,
		TenantID:         "TUGURE",
	})
	assertDomainCode(t, err, CodeEIRCashflowSignMismatch)
}

func TestUpdateCashflows_NotFound(t *testing.T) {
	amendRepo := newStubAmendmentRepo() // empty — proposal not found
	db := newMockDBNoTx(t)
	svc := newDetectionSvc(db, newStubInstrumenRepo(), amendRepo)

	_, err := svc.UpdateCashflows(context.Background(), UpdateCashflowsRequest{
		AmendmentID:      uuid.New(),
		RevisedCashflows: obligasiAtDiscount2(),
		ActorID:          uuid.New(),
		TenantID:         "TUGURE",
	})
	assertDomainCode(t, err, CodeEIRAmendNotFound)
}

// ─── isEIRApplicableForDetection coverage ────────────────────────────────────

func TestIsEIRApplicableForDetection_Nil(t *testing.T) {
	if isEIRApplicableForDetection(nil) {
		t.Error("expected false for nil instrument")
	}
}

func TestIsEIRApplicableForDetection_Deleted(t *testing.T) {
	now := time.Now()
	inst := &InstrumenForEIR{DeletedAt: &now}
	if isEIRApplicableForDetection(inst) {
		t.Error("expected false for deleted instrument")
	}
}

func TestIsEIRApplicableForDetection_EIRFlagFalse(t *testing.T) {
	inst := &InstrumenForEIR{EIRMethodFlag: false, KlasifikasiPsak71: "AC"}
	if isEIRApplicableForDetection(inst) {
		t.Error("expected false when EIRMethodFlag=false")
	}
}

func TestIsEIRApplicableForDetection_FVTPL(t *testing.T) {
	inst := &InstrumenForEIR{EIRMethodFlag: true, KlasifikasiPsak71: "FVTPL"}
	if isEIRApplicableForDetection(inst) {
		t.Error("expected false for FVTPL classification")
	}
}

func TestIsEIRApplicableForDetection_FVOCI(t *testing.T) {
	inst := &InstrumenForEIR{EIRMethodFlag: true, KlasifikasiPsak71: "FVOCI"}
	if !isEIRApplicableForDetection(inst) {
		t.Error("expected true for FVOCI+EIRMethodFlag")
	}
}

// ─── IsTerminal extended to CANCELLED ─────────────────────────────────────────

func TestAmendmentStatus_IsTerminal_IncludesCancelled(t *testing.T) {
	for _, s := range []AmendmentStatus{AmendStatusApproved, AmendStatusRejected, AmendStatusCancelled} {
		if !s.IsTerminal() {
			t.Errorf("expected %s.IsTerminal() = true", s)
		}
	}
	for _, s := range []AmendmentStatus{AmendStatusDraft, AmendStatusPendingReview, AmendStatusPendingApproval} {
		if s.IsTerminal() {
			t.Errorf("expected %s.IsTerminal() = false", s)
		}
	}
}
