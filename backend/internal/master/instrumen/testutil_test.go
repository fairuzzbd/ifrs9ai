package instrumen_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/master/instrumen"
)

// ─── Fixed test UUIDs ─────────────────────────────────────────────────────────

var (
	testInstrumenID    = uuid.MustParse("10000000-0000-0000-0000-000000000001")
	testActorID        = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	testCounterpartyID = uuid.MustParse("20000000-0000-0000-0000-000000000001")
	testPortofolioID   = uuid.MustParse("30000000-0000-0000-0000-000000000001")
)

// ─── Factory helpers ──────────────────────────────────────────────────────────

func testInstrumen() *instrumen.Instrumen {
	nominal, _ := decimal.NewFromString("1000000000.00")
	now := time.Now()
	return &instrumen.Instrumen{
		ID:                 testInstrumenID,
		KodeInstrumen:      "INST-001",
		TipeInstrumen:      "DEPOSITO",
		SubTipe:            "Deposito Berjangka",
		Nama:               "Deposito BCA 3 Bulan",
		CounterpartyID:     testCounterpartyID,
		MataUang:           "IDR",
		PortofolioID:       testPortofolioID,
		Nominal:            nominal,
		TanggalPenempatan:  "2026-01-01",
		AutoRenewalFlag:    false,
		FvociElection:      false,
		PremiumDiskonto:    decimal.Zero,
		BiayaTransaksi:     decimal.Zero,
		EirMethodFlag:      true,
		DayCountConvention: "ACT/365",
		Status:             "AKTIF",
		WorkflowStatus:     instrumen.WorkflowStatusDraft,
		CreatedAt:          now,
		CreatedBy:          testActorID,
		RowVersion:         1,
		TenantID:           "TUGURE",
		Version:            1,
	}
}

func testApprovedInstrumen() *instrumen.Instrumen {
	m := testInstrumen()
	m.WorkflowStatus = instrumen.WorkflowStatusApproved
	return m
}

func testLockedInstrumen() *instrumen.Instrumen {
	m := testInstrumen()
	now := time.Now()
	m.KlasifikasiLockedAt = &now
	m.KlasifikasiLockedBy = &testActorID
	klasifikasi := "AC"
	m.KlasifikasiPsak71 = &klasifikasi
	return m
}

// ─── Repository stub ──────────────────────────────────────────────────────────

// stubRepo is a flexible Repository stub for unit tests.
type stubRepo struct {
	// Per-method overrides
	createErr                   error
	getByIDResult               *instrumen.Instrumen
	getByIDErr                  error
	getByKodeResult             *instrumen.Instrumen
	getByKodeErr                error
	listResult                  []*instrumen.Instrumen
	listErr                     error
	updateResult                *instrumen.Instrumen
	updateErr                   error
	softDeleteResult            *instrumen.Instrumen
	softDeleteErr               error
	updateWorkflowStatusErr     error
	countActiveTxVal            int64
	countActiveTxErr            error
	checkCPApprovedResult       bool
	checkCPApprovedErr          error
	checkPortoApprovedResult    bool
	checkPortoBmDefault         *string
	checkPortoApprovedErr       error
	checkMataUangApprovedResult bool
	checkMataUangApprovedErr    error
	beginTxErr                  error
	auditItems                  []instrumen.AuditHistoryItem
	auditHasMore                bool
	auditErr                    error
	exportReader                io.Reader
	exportCount                 int
	exportErr                   error
}

var _ instrumen.Repository = (*stubRepo)(nil)

func (s *stubRepo) Create(_ context.Context, _ *sql.Tx, _ *instrumen.Instrumen) error {
	return s.createErr
}

func (s *stubRepo) GetByID(_ context.Context, _ uuid.UUID, _ bool) (*instrumen.Instrumen, error) {
	return s.getByIDResult, s.getByIDErr
}

func (s *stubRepo) GetByKode(_ context.Context, _ string, _ bool) (*instrumen.Instrumen, error) {
	return s.getByKodeResult, s.getByKodeErr
}

func (s *stubRepo) List(_ context.Context, _ listquery.Query, _ string, _ int, _ bool) ([]*instrumen.Instrumen, error) {
	return s.listResult, s.listErr
}

func (s *stubRepo) Update(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ instrumen.UpdateFields) (*instrumen.Instrumen, error) {
	return s.updateResult, s.updateErr
}

func (s *stubRepo) SoftDelete(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID) (*instrumen.Instrumen, error) {
	return s.softDeleteResult, s.softDeleteErr
}

func (s *stubRepo) UpdateWorkflowStatus(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ instrumen.WorkflowStatus, _ uuid.UUID) error {
	return s.updateWorkflowStatusErr
}

func (s *stubRepo) CountActiveTransactions(_ context.Context, _ uuid.UUID) (int64, error) {
	return s.countActiveTxVal, s.countActiveTxErr
}

func (s *stubRepo) CheckCounterpartyApproved(_ context.Context, _ uuid.UUID) (bool, error) {
	return s.checkCPApprovedResult, s.checkCPApprovedErr
}

func (s *stubRepo) CheckPortofolioApproved(_ context.Context, _ uuid.UUID) (bool, *string, error) {
	return s.checkPortoApprovedResult, s.checkPortoBmDefault, s.checkPortoApprovedErr
}

func (s *stubRepo) CheckMataUangApproved(_ context.Context, _ string) (bool, error) {
	return s.checkMataUangApprovedResult, s.checkMataUangApprovedErr
}

func (s *stubRepo) BeginTx(_ context.Context) (*sql.Tx, error) {
	if s.beginTxErr != nil {
		return nil, s.beginTxErr
	}
	return nil, fmt.Errorf("test: no database — tx not available")
}

func (s *stubRepo) ListAuditHistory(_ context.Context, _ uuid.UUID, _ string, _ int, _ bool) ([]instrumen.AuditHistoryItem, bool, error) {
	return s.auditItems, s.auditHasMore, s.auditErr
}

func (s *stubRepo) ExportAll(_ context.Context, _ listquery.Query) (io.Reader, int, error) {
	return s.exportReader, s.exportCount, s.exportErr
}

// approvedStub returns a stubRepo pre-configured with all FK checks returning approved=true.
func approvedStub() *stubRepo {
	return &stubRepo{
		checkCPApprovedResult:       true,
		checkPortoApprovedResult:    true,
		checkMataUangApprovedResult: true,
	}
}
