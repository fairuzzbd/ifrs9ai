// Package eir — tests for Service, ScheduleService, AmendmentService, and domain helpers.
//
// Tests follow M5 Story acceptance criteria:
//   - APP-C-EIR-001: Compute (FVTPL reject, POCI mode, persist, duplicate guard)
//   - APP-C-EIR-002: Generate schedule (build rows, closing delta)
//   - APP-C-EIR-004: Amendment 4-eyes workflow + SoD
//   - APP-C-EIR-005: Bulk recompute
//   - Signature hash functions
//   - Domain error constructors
//
// Uses in-memory stub repos + go-sqlmock to avoid live DB.
// DEC-016: no float64 in any assertion.
package eir

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// obligasiAtDiscount2 is a local alias for service_test to avoid redeclaration
// (solver_test.go owns the canonical obligasiAtDiscount + date + mustDec helpers).
func obligasiAtDiscount2() []CashflowItem {
	return []CashflowItem{
		{Date: date(2026, 1, 1), AmountIDR: mustDec("-1005000000")},
		{Date: date(2026, 7, 1), AmountIDR: mustDec("40000000")},
		{Date: date(2027, 1, 1), AmountIDR: mustDec("40000000")},
		{Date: date(2027, 7, 1), AmountIDR: mustDec("40000000")},
		{Date: date(2028, 1, 1), AmountIDR: mustDec("40000000")},
		{Date: date(2028, 7, 1), AmountIDR: mustDec("40000000")},
		{Date: date(2029, 1, 1), AmountIDR: mustDec("40000000")},
		{Date: date(2029, 7, 1), AmountIDR: mustDec("40000000")},
		{Date: date(2030, 1, 1), AmountIDR: mustDec("40000000")},
		{Date: date(2030, 7, 1), AmountIDR: mustDec("40000000")},
		{Date: date(2031, 1, 1), AmountIDR: mustDec("1040000000")},
	}
}

// ─── Stub repos ───────────────────────────────────────────────────────────────

type stubInstrumenRepo struct {
	instruments map[uuid.UUID]*InstrumenForEIR
	updateCalls int
}

func newStubInstrumenRepo() *stubInstrumenRepo {
	return &stubInstrumenRepo{instruments: make(map[uuid.UUID]*InstrumenForEIR)}
}

func (r *stubInstrumenRepo) put(inst InstrumenForEIR) { r.instruments[inst.ID] = &inst }

func (r *stubInstrumenRepo) GetByID(_ context.Context, id uuid.UUID) (*InstrumenForEIR, error) {
	if inst, ok := r.instruments[id]; ok {
		cp := *inst
		return &cp, nil
	}
	return nil, nil
}

func (r *stubInstrumenRepo) ListActiveForBulk(_ context.Context, _ BulkScope) (<-chan InstrumenForEIR, error) {
	ch := make(chan InstrumenForEIR, len(r.instruments))
	for _, inst := range r.instruments {
		ch <- *inst
	}
	close(ch)
	return ch, nil
}

func (r *stubInstrumenRepo) UpdateEIRAwal(_ context.Context, _ *sql.Tx, id uuid.UUID, eirAwal decimal.Decimal, _ uuid.UUID) error {
	r.updateCalls++
	if inst, ok := r.instruments[id]; ok {
		v := eirAwal
		inst.EIRAwal = &v
	}
	return nil
}

type stubScheduleRepo struct {
	rows      []ScheduleRow
	maxSeq    int
	hasActive bool
}

func (r *stubScheduleRepo) InsertBatch(_ context.Context, _ *sql.Tx, rows []ScheduleRow) error {
	r.rows = append(r.rows, rows...)
	for i := range rows {
		if rows[i].PeriodeSeq > r.maxSeq {
			r.maxSeq = rows[i].PeriodeSeq
		}
	}
	r.hasActive = true
	return nil
}

func (r *stubScheduleRepo) MarkSuperseded(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ int, _ uuid.UUID) error {
	for i := range r.rows {
		if r.rows[i].RecomputedFromSeq == nil {
			seq := r.maxSeq + 1
			r.rows[i].RecomputedFromSeq = &seq
		}
	}
	return nil
}

func (r *stubScheduleRepo) GetActiveByPeriode(_ context.Context, _ uuid.UUID, _ int) ([]ScheduleRow, error) {
	var active []ScheduleRow
	for i := range r.rows {
		if r.rows[i].RecomputedFromSeq == nil {
			active = append(active, r.rows[i])
		}
	}
	return active, nil
}

func (r *stubScheduleRepo) GetMaxPeriodeSeq(_ context.Context, _ uuid.UUID) (int, error) {
	return r.maxSeq, nil
}

func (r *stubScheduleRepo) HasActiveRows(_ context.Context, _ uuid.UUID) (bool, error) {
	return r.hasActive, nil
}

func (r *stubScheduleRepo) List(_ context.Context, _ uuid.UUID, _ listquery.Query, _ bool, _ string, limit int) ([]ScheduleRow, *response.PaginationMeta, error) {
	var active []ScheduleRow
	for i := range r.rows {
		if r.rows[i].RecomputedFromSeq == nil {
			active = append(active, r.rows[i])
		}
	}
	hasMore := len(active) > limit
	if hasMore {
		active = active[:limit]
	}
	return active, &response.PaginationMeta{HasMore: hasMore, Limit: limit}, nil
}

type stubAmendmentRepo struct {
	proposals   map[uuid.UUID]*AmendmentProposal
	activeForID map[uuid.UUID]bool
	createCalls int
	updateCalls int
}

func newStubAmendmentRepo() *stubAmendmentRepo {
	return &stubAmendmentRepo{
		proposals:   make(map[uuid.UUID]*AmendmentProposal),
		activeForID: make(map[uuid.UUID]bool),
	}
}

func (r *stubAmendmentRepo) Create(_ context.Context, _ *sql.Tx, p *AmendmentProposal) error {
	r.createCalls++
	cp := *p
	r.proposals[p.ID] = &cp
	r.activeForID[p.InstrumenID] = true
	return nil
}

func (r *stubAmendmentRepo) Update(_ context.Context, _ *sql.Tx, p *AmendmentProposal) error {
	r.updateCalls++
	cp := *p
	r.proposals[p.ID] = &cp
	if p.Status.IsTerminal() {
		r.activeForID[p.InstrumenID] = false
	}
	return nil
}

func (r *stubAmendmentRepo) GetByID(_ context.Context, id uuid.UUID) (*AmendmentProposal, error) {
	if p, ok := r.proposals[id]; ok {
		cp := *p
		return &cp, nil
	}
	return nil, nil
}

func (r *stubAmendmentRepo) HasActiveProposal(_ context.Context, instrumenID uuid.UUID) (bool, error) {
	return r.activeForID[instrumenID], nil
}

func (r *stubAmendmentRepo) List(_ context.Context, _ listquery.Query, _ string, limit int, _ uuid.UUID, _ bool) ([]AmendmentProposal, *response.PaginationMeta, error) {
	result := make([]AmendmentProposal, 0, len(r.proposals))
	for _, p := range r.proposals {
		result = append(result, *p)
	}
	return result, &response.PaginationMeta{Limit: limit}, nil
}

// Cancel implements AmendmentRepoIface (M6-005 addition).
func (r *stubAmendmentRepo) Cancel(_ context.Context, _ *sql.Tx, proposalID uuid.UUID, cancelReason string, cancelledBy uuid.UUID) error {
	if p, ok := r.proposals[proposalID]; ok {
		p.Status = AmendStatusCancelled
		p.CancelReason = &cancelReason
		p.CancelledBy = &cancelledBy
		r.activeForID[p.InstrumenID] = false
	}
	return nil
}

// ListQueue implements AmendmentRepoIface (M6-004 addition).
func (r *stubAmendmentRepo) ListQueue(_ context.Context, _ listquery.Query, _ string, limit int) ([]QueueRow, *response.PaginationMeta, error) {
	rows := make([]QueueRow, 0, len(r.proposals))
	for _, p := range r.proposals {
		if p.Status.IsTerminal() {
			continue
		}
		rows = append(rows, QueueRow{
			AmendmentID:      p.ID,
			InstrumenID:      p.InstrumenID,
			Status:           p.Status,
			TriggerSource:    p.TriggerSource,
			EIRLama:          p.EIRLama,
			MakerID:          p.MakerID,
			TanggalAmandemen: p.TanggalAmandemen,
			CreatedAt:        p.CreatedAt,
		})
	}
	return rows, &response.PaginationMeta{Limit: limit}, nil
}

type stubAuditWriter struct{ events []AuditEvent }

func (w *stubAuditWriter) Write(_ context.Context, _ *sql.Tx, evt AuditEvent) error {
	w.events = append(w.events, evt)
	return nil
}

// ─── sqlmock helpers ──────────────────────────────────────────────────────────

func newMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	return db, mock
}

// ─── Instrument factory ───────────────────────────────────────────────────────

func testLogger() *slog.Logger { return slog.Default() }

func actInstrumen(id uuid.UUID, klasifikasi string, eirAwal *decimal.Decimal) InstrumenForEIR {
	nominal := mustDec("1000000000")
	kupon := mustDec("0.08")
	jt := date(2031, 1, 1)
	return InstrumenForEIR{
		ID:                        id,
		KodeInstrumen:             "OBL-TEST-001",
		KlasifikasiPsak71:         klasifikasi,
		EIRMethodFlag:             true,
		EIRAwal:                   eirAwal,
		FlagPOCI:                  false,
		Nominal:                   nominal,
		BiayaTransaksiCapitalized: mustDec("5000000"),
		Kupon:                     &kupon,
		TanggalPenempatan:         date(2026, 1, 1),
		TanggalJatuhTempo:         &jt,
		Status:                    "ACTIVE",
		TenantID:                  "TUGURE",
	}
}

// ─── Service tests ─────────────────────────────────────────────────────────

func TestService_Compute_FVTPL_Rejected(t *testing.T) {
	instrRepo := newStubInstrumenRepo()
	id := uuid.New()
	instrRepo.put(actInstrumen(id, "FVTPL", nil))

	svc := &Service{instrRepo: instrRepo, solver: NewSolver(), auditWriter: &stubAuditWriter{}, logger: testLogger()}

	_, err := svc.Compute(context.Background(), ComputeRequest{
		InstrumenID:        id,
		CashflowProjection: obligasiAtDiscount2(),
	}, uuid.New(), "ROLE-RISK")

	assertDomainErr(t, err, CodeEIRInstrumenFVTPLNoEIR)
}

func TestService_Compute_InstrumenNotFound(t *testing.T) {
	svc := &Service{instrRepo: newStubInstrumenRepo(), solver: NewSolver(), auditWriter: &stubAuditWriter{}, logger: testLogger()}

	_, err := svc.Compute(context.Background(), ComputeRequest{
		InstrumenID:        uuid.New(),
		CashflowProjection: obligasiAtDiscount2(),
	}, uuid.New(), "ROLE-RISK")

	assertDomainErr(t, err, CodeEIRInstrumenNotFound)
}

func TestService_Compute_AlreadyComputed_GuardFires(t *testing.T) {
	instrRepo := newStubInstrumenRepo()
	id := uuid.New()
	eirVal := mustDec("0.08")
	instrRepo.put(actInstrumen(id, "AC", &eirVal))

	svc := &Service{instrRepo: instrRepo, solver: NewSolver(), auditWriter: &stubAuditWriter{}, logger: testLogger()}

	_, err := svc.Compute(context.Background(), ComputeRequest{
		InstrumenID:        id,
		CashflowProjection: obligasiAtDiscount2(),
		PersistResult:      true,
		ForceRecompute:     false,
	}, uuid.New(), "ROLE-RISK")

	assertDomainErr(t, err, CodeEIRAlreadyComputed)
}

func TestService_Compute_PreviewOnly_Success(t *testing.T) {
	instrRepo := newStubInstrumenRepo()
	id := uuid.New()
	instrRepo.put(actInstrumen(id, "AC", nil))

	svc := &Service{instrRepo: instrRepo, solver: NewSolver(), auditWriter: &stubAuditWriter{}, logger: testLogger()}

	result, err := svc.Compute(context.Background(), ComputeRequest{
		InstrumenID:        id,
		CashflowProjection: obligasiAtDiscount2(),
		PersistResult:      false,
	}, uuid.New(), "ROLE-RISK")

	if err != nil {
		t.Fatalf("preview compute failed: %v", err)
	}
	if result.EIRPerPeriod.IsZero() {
		t.Error("EIR should not be zero")
	}
	if result.Persisted {
		t.Error("Persisted should be false for preview mode")
	}
	if result.EIRPerPeriod.LessThan(mustDec("0.03")) || result.EIRPerPeriod.GreaterThan(mustDec("0.15")) {
		t.Errorf("EIR %s out of expected range [0.03, 0.15]", result.EIRPerPeriod.StringFixed(8))
	}
	t.Logf("Preview EIR: %s", result.EIRPerPeriod.StringFixed(8))
}

func TestService_Compute_ForceRecompute_Preview(t *testing.T) {
	instrRepo := newStubInstrumenRepo()
	id := uuid.New()
	eirVal := mustDec("0.08")
	instrRepo.put(actInstrumen(id, "AC", &eirVal))

	svc := &Service{instrRepo: instrRepo, solver: NewSolver(), auditWriter: &stubAuditWriter{}, logger: testLogger()}

	result, err := svc.Compute(context.Background(), ComputeRequest{
		InstrumenID:        id,
		CashflowProjection: obligasiAtDiscount2(),
		PersistResult:      false,
		ForceRecompute:     true,
	}, uuid.New(), "ROLE-RISK")

	if err != nil {
		t.Fatalf("force recompute preview failed: %v", err)
	}
	if result.EIRPerPeriod.IsZero() {
		t.Error("EIR should not be zero")
	}
}

func TestService_Compute_POCI_Mismatch_Rejected(t *testing.T) {
	instrRepo := newStubInstrumenRepo()
	id := uuid.New()
	inst := actInstrumen(id, "AC", nil)
	inst.FlagPOCI = false
	instrRepo.put(inst)

	svc := &Service{instrRepo: instrRepo, solver: NewSolver(), auditWriter: &stubAuditWriter{}, logger: testLogger()}

	_, err := svc.Compute(context.Background(), ComputeRequest{
		InstrumenID:        id,
		CashflowProjection: obligasiAtDiscount2(),
		POCIMode:           true,
	}, uuid.New(), "ROLE-RISK")

	assertDomainErr(t, err, CodeEIRPOCIRequiresPDAdjustedCF)
}

func TestService_Compute_PersistResult_CallsAudit(t *testing.T) {
	instrRepo := newStubInstrumenRepo()
	id := uuid.New()
	instrRepo.put(actInstrumen(id, "AC", nil))

	db, mock := newMockDB(t)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectCommit()

	auditW := &stubAuditWriter{}
	svc := &Service{
		db:          db,
		instrRepo:   instrRepo,
		solver:      NewSolver(),
		auditWriter: auditW,
		logger:      testLogger(),
	}

	result, err := svc.Compute(context.Background(), ComputeRequest{
		InstrumenID:        id,
		CashflowProjection: obligasiAtDiscount2(),
		PersistResult:      true,
	}, uuid.New(), "ROLE-RISK")

	if err != nil {
		t.Fatalf("persist compute failed: %v", err)
	}
	if !result.Persisted {
		t.Error("Persisted should be true")
	}
	if mock.ExpectationsWereMet() != nil {
		t.Errorf("sqlmock: %v", mock.ExpectationsWereMet())
	}
	hasComputeAudit := false
	for _, evt := range auditW.events {
		if evt.Action == "EIR.COMPUTE" {
			hasComputeAudit = true
		}
	}
	if !hasComputeAudit {
		t.Error("EIR.COMPUTE audit event not found")
	}
}

// ─── ScheduleService tests ────────────────────────────────────────────────────

func TestScheduleService_Generate_Success(t *testing.T) {
	instrRepo := newStubInstrumenRepo()
	id := uuid.New()
	eirVal := mustDec("0.08028915")
	instrRepo.put(actInstrumen(id, "AC", &eirVal))

	schedRepo := &stubScheduleRepo{}
	auditW := &stubAuditWriter{}

	db, mock := newMockDB(t)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectCommit()

	svc := &ScheduleService{
		db:          db,
		instrRepo:   instrRepo,
		schedRepo:   schedRepo,
		solver:      NewSolver(),
		auditWriter: auditW,
		logger:      testLogger(),
	}

	result, err := svc.Generate(context.Background(), GenerateScheduleRequest{
		InstrumenID:        id,
		CashflowProjection: obligasiAtDiscount2(),
	}, uuid.New(), "ROLE-RISK")

	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if result.TotalRows == 0 {
		t.Error("expected schedule rows")
	}
	if len(schedRepo.rows) == 0 {
		t.Error("no rows in repo")
	}
	hasAudit := false
	for _, evt := range auditW.events {
		if evt.Action == "EIR.SCHEDULE_GENERATED" {
			hasAudit = true
		}
	}
	if !hasAudit {
		t.Error("EIR.SCHEDULE_GENERATED audit event missing")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
	t.Logf("Generated %d rows, delta: %s", result.TotalRows, result.ClosingRoundingDelta.StringFixed(4))
}

func TestScheduleService_Generate_DuplicateGuard(t *testing.T) {
	instrRepo := newStubInstrumenRepo()
	id := uuid.New()
	eirVal := mustDec("0.08")
	instrRepo.put(actInstrumen(id, "AC", &eirVal))

	svc := &ScheduleService{
		instrRepo:   instrRepo,
		schedRepo:   &stubScheduleRepo{hasActive: true},
		solver:      NewSolver(),
		auditWriter: &stubAuditWriter{},
		logger:      testLogger(),
	}

	_, err := svc.Generate(context.Background(), GenerateScheduleRequest{
		InstrumenID:        id,
		CashflowProjection: obligasiAtDiscount2(),
	}, uuid.New(), "ROLE-RISK")

	assertDomainErr(t, err, CodeEIRDuplicateScheduleVersion)
}

func TestScheduleService_Generate_EIRNotComputed_Error(t *testing.T) {
	instrRepo := newStubInstrumenRepo()
	id := uuid.New()
	instrRepo.put(actInstrumen(id, "AC", nil))

	svc := &ScheduleService{
		instrRepo:   instrRepo,
		schedRepo:   &stubScheduleRepo{},
		solver:      NewSolver(),
		auditWriter: &stubAuditWriter{},
		logger:      testLogger(),
	}

	_, err := svc.Generate(context.Background(), GenerateScheduleRequest{
		InstrumenID:        id,
		CashflowProjection: obligasiAtDiscount2(),
	}, uuid.New(), "ROLE-RISK")

	assertDomainErr(t, err, CodeEIRNotYetComputed)
}

func TestScheduleService_Generate_ForceRegenerate(t *testing.T) {
	instrRepo := newStubInstrumenRepo()
	id := uuid.New()
	eirVal := mustDec("0.08028915")
	instrRepo.put(actInstrumen(id, "AC", &eirVal))

	db, mock := newMockDB(t)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectCommit()

	svc := &ScheduleService{
		db:          db,
		instrRepo:   instrRepo,
		schedRepo:   &stubScheduleRepo{hasActive: true},
		solver:      NewSolver(),
		auditWriter: &stubAuditWriter{},
		logger:      testLogger(),
	}

	result, err := svc.Generate(context.Background(), GenerateScheduleRequest{
		InstrumenID:        id,
		CashflowProjection: obligasiAtDiscount2(),
		ForceRegenerate:    true,
	}, uuid.New(), "ROLE-RISK")

	if err != nil {
		t.Fatalf("force regenerate failed: %v", err)
	}
	if result.TotalRows == 0 {
		t.Error("expected rows")
	}
}

func TestScheduleService_Generate_Rows_OpeningEqualsAbsCF0(t *testing.T) {
	instrRepo := newStubInstrumenRepo()
	id := uuid.New()
	eirVal := mustDec("0.08028915")
	instrRepo.put(actInstrumen(id, "AC", &eirVal))

	db, mock := newMockDB(t)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectCommit()

	svc := &ScheduleService{
		db:          db,
		instrRepo:   instrRepo,
		schedRepo:   &stubScheduleRepo{},
		solver:      NewSolver(),
		auditWriter: &stubAuditWriter{},
		logger:      testLogger(),
	}

	cfs := obligasiAtDiscount2()
	result, err := svc.Generate(context.Background(), GenerateScheduleRequest{
		InstrumenID:        id,
		CashflowProjection: cfs,
	}, uuid.New(), "ROLE-RISK")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	expected := cfs[0].AmountIDR.Abs().RoundBank(4)
	if !result.OpeningCarryingFirst.Equal(expected) {
		t.Errorf("OpeningCarryingFirst: want %s, got %s", expected.StringFixed(4), result.OpeningCarryingFirst.StringFixed(4))
	}

	for i := 0; i < len(result.Rows)-1; i++ {
		if !result.Rows[i].ClosingCarrying.Equal(result.Rows[i+1].OpeningCarrying) {
			t.Errorf("row[%d] closing %s != row[%d] opening %s",
				i, result.Rows[i].ClosingCarrying.StringFixed(4),
				i+1, result.Rows[i+1].OpeningCarrying.StringFixed(4))
		}
	}
}

// ─── Domain error constructor tests ───────────────────────────────────────────

func TestErrorConstructors_HTTPStatus(t *testing.T) {
	cases := []struct {
		name    string
		err     *domainerrors.DomainError
		expHTTP int
	}{
		{"NonConvergent", ErrEIRNonConvergent(mustDec("0.001")), 422},
		{"Divergent", ErrEIRDivergent("test"), 422},
		{"CashflowInvalid", ErrEIRCashflowInvalid("test"), 422},
		{"CashflowSignMismatch", ErrEIRCashflowSignMismatch(), 422},
		{"FVTPLNoEIR", ErrEIRInstrumenFVTPLNoEIR("FVTPL"), 422},
		{"ScheduleNotFound", ErrEIRScheduleNotFound("id"), 404},
		{"DuplicateSchedule", ErrEIRDuplicateScheduleVersion("OBL-001"), 409},
		{"POCIRequired", ErrEIRPOCIRequiresPDAdjustedCF(), 422},
		{"BulkInvalidScope", ErrEIRBulkRecomputeInvalidScope("BAD"), 400},
		{"InstrumenNotFound", ErrEIRInstrumenNotFound("id"), 404},
		{"AlreadyComputed", ErrEIRAlreadyComputed(), 409},
		{"NotYetComputed", ErrEIRNotYetComputed(), 422},
		{"AmendNotFound", ErrEIRAmendNotFound("id"), 404},
		{"AmendActiveExists", ErrEIRAmendActiveExists("id"), 409},
		{"AmendInvalidTransition", ErrEIRAmendInvalidTransition("DRAFT", "APPROVED"), 422},
		{"MFAStepUpRequired", ErrEIRMFAStepUpRequired(), 403},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.HTTPStatus() != tt.expHTTP {
				t.Errorf("expected HTTP %d, got %d", tt.expHTTP, tt.err.HTTPStatus())
			}
			if tt.err.Code() == "" {
				t.Error("code must not be empty")
			}
			if tt.err.Message() == "" {
				t.Error("message must not be empty")
			}
		})
	}
}

// ─── Signature hash tests ─────────────────────────────────────────────────────

func TestSignatureHash_Deterministic(t *testing.T) {
	proposalID := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")
	reviewerID := uuid.MustParse("fedcba98-7654-3210-fedc-ba9876543210")
	comment := "reviewed and approved"

	h1 := ComputeReviewerSignatureHash(proposalID, reviewerID, comment)
	h2 := ComputeReviewerSignatureHash(proposalID, reviewerID, comment)

	if h1 != h2 {
		t.Errorf("signature hash not deterministic: %s != %s", h1, h2)
	}
	if len(h1) != 64 {
		t.Errorf("expected 64-char hex hash, got %d chars", len(h1))
	}
}

func TestApproverSignatureHash_DifferentEIR(t *testing.T) {
	proposalID := uuid.New()
	approverID := uuid.New()

	h1 := ComputeApproverSignatureHash(proposalID, approverID, "ok", mustDec("0.08028915"))
	h2 := ComputeApproverSignatureHash(proposalID, approverID, "ok", mustDec("0.09000000"))

	if h1 == h2 {
		t.Error("approver hash should differ for different EIR values")
	}
}

// ─── AmendmentStatus tests ────────────────────────────────────────────────────

func TestAmendmentStatus_IsTerminal(t *testing.T) {
	for _, s := range []AmendmentStatus{AmendStatusApproved, AmendStatusRejected} {
		if !s.IsTerminal() {
			t.Errorf("%s should be terminal", s)
		}
	}
	for _, s := range []AmendmentStatus{AmendStatusDraft, AmendStatusPendingReview, AmendStatusPendingApproval} {
		if s.IsTerminal() {
			t.Errorf("%s should not be terminal", s)
		}
	}
}

// ─── CashflowItem JSON round-trip ─────────────────────────────────────────────

func TestMarshalUnmarshalCashflows_RoundTrip(t *testing.T) {
	original := []CashflowItem{
		{Date: date(2026, 1, 1), AmountIDR: mustDec("-1005000000.0000")},
		{Date: date(2026, 7, 1), AmountIDR: mustDec("40000000.0000")},
		{Date: date(2031, 1, 1), AmountIDR: mustDec("1040000000.0000")},
	}

	jsonStr, err := marshalCashflows(original)
	if err != nil {
		t.Fatalf("marshalCashflows: %v", err)
	}

	restored, err := unmarshalCashflows(jsonStr)
	if err != nil {
		t.Fatalf("unmarshalCashflows: %v", err)
	}
	if len(restored) != len(original) {
		t.Fatalf("length: want %d, got %d", len(original), len(restored))
	}
	for i := range original {
		if !original[i].AmountIDR.Equal(restored[i].AmountIDR) {
			t.Errorf("[%d] amount: %s != %s", i, original[i].AmountIDR, restored[i].AmountIDR)
		}
		if !original[i].Date.Equal(restored[i].Date) {
			t.Errorf("[%d] date: %v != %v", i, original[i].Date, restored[i].Date)
		}
	}
}

// ─── buildScheduleRows tests ──────────────────────────────────────────────────

func TestBuildScheduleRows_OpeningCarrying(t *testing.T) {
	id := uuid.New()
	eirVal := mustDec("0.04")
	inst := actInstrumen(id, "AC", &eirVal)
	cfs := obligasiAtDiscount2()

	rows, _ := buildScheduleRows(id, eirVal, cfs, &inst, uuid.New())
	if len(rows) == 0 {
		t.Fatal("no rows built")
	}

	expected := cfs[0].AmountIDR.Abs().RoundBank(4)
	if !rows[0].OpeningCarrying.Equal(expected) {
		t.Errorf("row[0] OpeningCarrying %s != %s", rows[0].OpeningCarrying.StringFixed(4), expected.StringFixed(4))
	}
}

func TestBuildScheduleRows_AllRowsCorrect(t *testing.T) {
	id := uuid.New()
	eirVal := mustDec("0.04")
	inst := actInstrumen(id, "AC", &eirVal)
	rows, _ := buildScheduleRows(id, eirVal, obligasiAtDiscount2(), &inst, uuid.New())

	for i, row := range rows {
		if row.InstrumenID != id {
			t.Errorf("row[%d] InstrumenID mismatch", i)
		}
		if row.PeriodeSeq != i+1 {
			t.Errorf("row[%d] PeriodeSeq want %d, got %d", i, i+1, row.PeriodeSeq)
		}
		if row.TenantID != "TUGURE" {
			t.Errorf("row[%d] TenantID mismatch", i)
		}
	}
}

func TestBuildScheduleRows_ClosingLinked(t *testing.T) {
	id := uuid.New()
	eirVal := mustDec("0.04")
	inst := actInstrumen(id, "AC", &eirVal)
	rows, _ := buildScheduleRows(id, eirVal, obligasiAtDiscount2(), &inst, uuid.New())

	for i := 0; i < len(rows)-1; i++ {
		if !rows[i].ClosingCarrying.Equal(rows[i+1].OpeningCarrying) {
			t.Errorf("row[%d] closing %s != row[%d] opening %s",
				i, rows[i].ClosingCarrying.StringFixed(4),
				i+1, rows[i+1].OpeningCarrying.StringFixed(4))
		}
	}
}

// ─── AmendmentService tests ───────────────────────────────────────────────────

func mustMarshalCF(t *testing.T, cfs []CashflowItem) string {
	t.Helper()
	s, err := marshalCashflows(cfs)
	if err != nil {
		t.Fatalf("marshalCashflows: %v", err)
	}
	return s
}

func makeProposal(instrID, makerID uuid.UUID, status AmendmentStatus, cfJSON string) AmendmentProposal {
	eirLama := mustDec("0.08")
	return AmendmentProposal{
		ID:                  uuid.New(),
		InstrumenID:         instrID,
		Status:              status,
		TanggalAmandemen:    date(2026, 6, 1),
		TanggalReEstimasi:   time.Now(),
		AlasanAmandemen:     "test amendment",
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

func TestAmendmentService_Propose_Success(t *testing.T) {
	instrID := uuid.New()
	makerID := uuid.New()
	eirVal := mustDec("0.08")

	instrRepo := newStubInstrumenRepo()
	instrRepo.put(actInstrumen(instrID, "AC", &eirVal))

	amendRepo := newStubAmendmentRepo()

	db, mock := newMockDB(t)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectCommit()

	svc := &AmendmentService{
		db:          db,
		instrRepo:   instrRepo,
		schedRepo:   &stubScheduleRepo{},
		amendRepo:   amendRepo,
		solver:      NewSolver(),
		auditWriter: &stubAuditWriter{},
		logger:      testLogger(),
	}

	proposal, err := svc.Propose(context.Background(), ProposeRequest{
		InstrumenID:               instrID,
		TanggalAmandemen:          date(2026, 6, 1),
		RevisedCashflowProjection: obligasiAtDiscount2(),
		AlasanAmandemen:           "test amendment reason",
	}, makerID, "ROLE-AKUN")

	if err != nil {
		t.Fatalf("Propose failed: %v", err)
	}
	if proposal.Status != AmendStatusPendingReview {
		t.Errorf("expected PENDING_REVIEW, got %s", proposal.Status)
	}
	if proposal.MakerID == nil || *proposal.MakerID != makerID {
		t.Error("MakerID not set correctly")
	}
	if amendRepo.createCalls != 1 {
		t.Errorf("expected 1 create call, got %d", amendRepo.createCalls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

func TestAmendmentService_Propose_NoEIRAwal_Error(t *testing.T) {
	instrID := uuid.New()
	instrRepo := newStubInstrumenRepo()
	instrRepo.put(actInstrumen(instrID, "AC", nil)) // no eir_awal

	svc := &AmendmentService{
		instrRepo:   instrRepo,
		schedRepo:   &stubScheduleRepo{},
		amendRepo:   newStubAmendmentRepo(),
		solver:      NewSolver(),
		auditWriter: &stubAuditWriter{},
		logger:      testLogger(),
	}

	_, err := svc.Propose(context.Background(), ProposeRequest{
		InstrumenID:               instrID,
		TanggalAmandemen:          date(2026, 6, 1),
		RevisedCashflowProjection: obligasiAtDiscount2(),
		AlasanAmandemen:           "test",
	}, uuid.New(), "ROLE-AKUN")

	assertDomainErr(t, err, CodeEIRNotYetComputed)
}

func TestAmendmentService_Propose_ActiveExists(t *testing.T) {
	instrID := uuid.New()
	eirVal := mustDec("0.08")
	instrRepo := newStubInstrumenRepo()
	instrRepo.put(actInstrumen(instrID, "AC", &eirVal))

	amendRepo := newStubAmendmentRepo()
	amendRepo.activeForID[instrID] = true

	svc := &AmendmentService{
		instrRepo:   instrRepo,
		schedRepo:   &stubScheduleRepo{},
		amendRepo:   amendRepo,
		solver:      NewSolver(),
		auditWriter: &stubAuditWriter{},
		logger:      testLogger(),
	}

	_, err := svc.Propose(context.Background(), ProposeRequest{
		InstrumenID:               instrID,
		TanggalAmandemen:          date(2026, 6, 1),
		RevisedCashflowProjection: obligasiAtDiscount2(),
		AlasanAmandemen:           "test",
	}, uuid.New(), "ROLE-AKUN")

	assertDomainErr(t, err, CodeEIRAmendActiveExists)
}

func TestAmendmentService_Review_SoD_MakerCannotBeReviewer(t *testing.T) {
	instrID := uuid.New()
	makerID := uuid.New()

	instrRepo := newStubInstrumenRepo()
	eirVal := mustDec("0.08")
	instrRepo.put(actInstrumen(instrID, "AC", &eirVal))

	amendRepo := newStubAmendmentRepo()
	p := makeProposal(instrID, makerID, AmendStatusPendingReview, mustMarshalCF(t, obligasiAtDiscount2()))
	amendRepo.proposals[p.ID] = &p
	amendRepo.activeForID[instrID] = true

	svc := &AmendmentService{
		instrRepo:   instrRepo,
		schedRepo:   &stubScheduleRepo{},
		amendRepo:   amendRepo,
		solver:      NewSolver(),
		auditWriter: &stubAuditWriter{},
		logger:      testLogger(),
	}

	_, err := svc.Review(context.Background(), ReviewRequest{
		AmendmentID: p.ID,
		Comment:     "self review",
	}, makerID, "ROLE-RISK") // same as maker

	if err == nil {
		t.Fatal("expected SoD violation")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError, got %v", err)
	}
	if string(de.Code()) != string(domainerrors.CodeSoDViolation) {
		t.Errorf("expected SoD violation, got %s", de.Code())
	}
}

func TestAmendmentService_Review_Success(t *testing.T) {
	instrID := uuid.New()
	makerID := uuid.New()
	reviewerID := uuid.New()

	instrRepo := newStubInstrumenRepo()
	eirVal := mustDec("0.08")
	instrRepo.put(actInstrumen(instrID, "AC", &eirVal))

	amendRepo := newStubAmendmentRepo()
	p := makeProposal(instrID, makerID, AmendStatusPendingReview, mustMarshalCF(t, obligasiAtDiscount2()))
	amendRepo.proposals[p.ID] = &p
	amendRepo.activeForID[instrID] = true

	db, mock := newMockDB(t)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectCommit()

	svc := &AmendmentService{
		db:          db,
		instrRepo:   instrRepo,
		schedRepo:   &stubScheduleRepo{},
		amendRepo:   amendRepo,
		solver:      NewSolver(),
		auditWriter: &stubAuditWriter{},
		logger:      testLogger(),
	}

	reviewed, err := svc.Review(context.Background(), ReviewRequest{
		AmendmentID: p.ID,
		Comment:     "looks correct",
	}, reviewerID, "ROLE-RISK")

	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}
	if reviewed.Status != AmendStatusPendingApproval {
		t.Errorf("expected PENDING_APPROVAL, got %s", reviewed.Status)
	}
	if reviewed.ReviewerID == nil || *reviewed.ReviewerID != reviewerID {
		t.Error("ReviewerID not set")
	}
	if reviewed.ReviewerSignatureHash == nil || *reviewed.ReviewerSignatureHash == "" {
		t.Error("ReviewerSignatureHash must be set")
	}
}

func TestAmendmentService_Approve_MFARequired(t *testing.T) {
	instrID := uuid.New()
	makerID := uuid.New()

	instrRepo := newStubInstrumenRepo()
	eirVal := mustDec("0.08")
	instrRepo.put(actInstrumen(instrID, "AC", &eirVal))

	amendRepo := newStubAmendmentRepo()
	p := makeProposal(instrID, makerID, AmendStatusPendingApproval, mustMarshalCF(t, obligasiAtDiscount2()))
	amendRepo.proposals[p.ID] = &p

	svc := &AmendmentService{
		instrRepo:   instrRepo,
		schedRepo:   &stubScheduleRepo{},
		amendRepo:   amendRepo,
		solver:      NewSolver(),
		auditWriter: &stubAuditWriter{},
		logger:      testLogger(),
	}

	_, err := svc.Approve(context.Background(), ApproveRequest{
		AmendmentID: p.ID,
		Comment:     "approved",
		StepUpToken: "", // empty → MFA error
	}, uuid.New(), "ROLE-ALCO")

	assertDomainErr(t, err, CodeEIRMFAStepUpRequired)
}

func TestAmendmentService_Approve_SoD_MakerCannotApprove(t *testing.T) {
	instrID := uuid.New()
	makerID := uuid.New()

	instrRepo := newStubInstrumenRepo()
	eirVal := mustDec("0.08")
	instrRepo.put(actInstrumen(instrID, "AC", &eirVal))

	amendRepo := newStubAmendmentRepo()
	p := makeProposal(instrID, makerID, AmendStatusPendingApproval, mustMarshalCF(t, obligasiAtDiscount2()))
	amendRepo.proposals[p.ID] = &p

	svc := &AmendmentService{
		instrRepo:   instrRepo,
		schedRepo:   &stubScheduleRepo{},
		amendRepo:   amendRepo,
		solver:      NewSolver(),
		auditWriter: &stubAuditWriter{},
		logger:      testLogger(),
	}

	_, err := svc.Approve(context.Background(), ApproveRequest{
		AmendmentID: p.ID,
		Comment:     "self-approved",
		StepUpToken: "valid-token",
	}, makerID, "ROLE-ALCO") // same as maker

	if err == nil {
		t.Fatal("expected SoD violation")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError, got %v", err)
	}
	if string(de.Code()) != string(domainerrors.CodeSoDViolation) {
		t.Errorf("expected SoD, got %s", de.Code())
	}
}

func TestAmendmentService_Approve_Success(t *testing.T) {
	instrID := uuid.New()
	makerID := uuid.New()
	reviewerID := uuid.New()
	approverID := uuid.New()
	eirVal := mustDec("0.08")

	instrRepo := newStubInstrumenRepo()
	instrRepo.put(actInstrumen(instrID, "AC", &eirVal))

	amendRepo := newStubAmendmentRepo()
	base := makeProposal(instrID, makerID, AmendStatusPendingApproval, mustMarshalCF(t, obligasiAtDiscount2()))
	base.ReviewerID = &reviewerID
	amendRepo.proposals[base.ID] = &base
	amendRepo.activeForID[instrID] = true

	schedRepo := &stubScheduleRepo{maxSeq: 10}
	db, mock := newMockDB(t)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectCommit()

	svc := &AmendmentService{
		db:          db,
		instrRepo:   instrRepo,
		schedRepo:   schedRepo,
		amendRepo:   amendRepo,
		solver:      NewSolver(),
		auditWriter: &stubAuditWriter{},
		logger:      testLogger(),
	}

	approved, err := svc.Approve(context.Background(), ApproveRequest{
		AmendmentID: base.ID,
		Comment:     "alco approved",
		StepUpToken: "mfa-step-up-token",
	}, approverID, "ROLE-ALCO")

	if err != nil {
		t.Fatalf("Approve failed: %v", err)
	}
	if approved.Status != AmendStatusApproved {
		t.Errorf("expected APPROVED, got %s", approved.Status)
	}
	if approved.EIRBaru == nil {
		t.Error("EIRBaru must be set")
	}
	if approved.ApproverID == nil || *approved.ApproverID != approverID {
		t.Error("ApproverID not set")
	}
	if approved.ApproverSignatureHash == nil || *approved.ApproverSignatureHash == "" {
		t.Error("ApproverSignatureHash must be set")
	}
	if len(schedRepo.rows) == 0 {
		t.Error("new schedule rows should be inserted")
	}
	if schedRepo.rows[0].PeriodeSeq <= 10 {
		t.Errorf("new rows should start after seq 10, got %d", schedRepo.rows[0].PeriodeSeq)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

func TestAmendmentService_Reject_Success(t *testing.T) {
	instrID := uuid.New()
	makerID := uuid.New()
	rejectorID := uuid.New()

	instrRepo := newStubInstrumenRepo()
	eirVal := mustDec("0.08")
	instrRepo.put(actInstrumen(instrID, "AC", &eirVal))

	amendRepo := newStubAmendmentRepo()
	p := makeProposal(instrID, makerID, AmendStatusPendingReview, mustMarshalCF(t, obligasiAtDiscount2()))
	amendRepo.proposals[p.ID] = &p
	amendRepo.activeForID[instrID] = true

	db, mock := newMockDB(t)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectCommit()

	svc := &AmendmentService{
		db:          db,
		instrRepo:   instrRepo,
		schedRepo:   &stubScheduleRepo{},
		amendRepo:   amendRepo,
		solver:      NewSolver(),
		auditWriter: &stubAuditWriter{},
		logger:      testLogger(),
	}

	rejected, err := svc.Reject(context.Background(), WorkflowAction{
		AmendmentID: p.ID,
		Comment:     "insufficient documentation",
	}, rejectorID, "ROLE-RISK")

	if err != nil {
		t.Fatalf("Reject failed: %v", err)
	}
	if rejected.Status != AmendStatusRejected {
		t.Errorf("expected REJECTED, got %s", rejected.Status)
	}
}

func TestAmendmentService_Reject_AlreadyTerminal(t *testing.T) {
	instrID := uuid.New()
	makerID := uuid.New()

	instrRepo := newStubInstrumenRepo()
	eirVal := mustDec("0.08")
	instrRepo.put(actInstrumen(instrID, "AC", &eirVal))

	amendRepo := newStubAmendmentRepo()
	p := makeProposal(instrID, makerID, AmendStatusApproved, mustMarshalCF(t, obligasiAtDiscount2()))
	amendRepo.proposals[p.ID] = &p

	svc := &AmendmentService{
		instrRepo:   instrRepo,
		schedRepo:   &stubScheduleRepo{},
		amendRepo:   amendRepo,
		solver:      NewSolver(),
		auditWriter: &stubAuditWriter{},
		logger:      testLogger(),
	}

	_, err := svc.Reject(context.Background(), WorkflowAction{
		AmendmentID: p.ID,
		Comment:     "trying to reject approved",
	}, uuid.New(), "ROLE-RISK")

	assertDomainErr(t, err, CodeEIRAmendInvalidTransition)
}

// ─── BulkService tests ────────────────────────────────────────────────────────

func TestBulkService_Recompute_AllMissingEIR(t *testing.T) {
	instrRepo := newStubInstrumenRepo()
	for i := 0; i < 3; i++ {
		instrRepo.put(actInstrumen(uuid.New(), "AC", nil))
	}

	svc := NewBulkService(nil, instrRepo, &stubScheduleRepo{}, nil, nil, testLogger())
	result, err := svc.Recompute(context.Background(), BulkScopeAllActive, "job-missing-eir", uuid.New())
	if err != nil {
		t.Fatalf("Recompute: %v", err)
	}
	if result.TotalInstruments != 3 {
		t.Errorf("total: want 3, got %d", result.TotalInstruments)
	}
	if result.MissingCount != 3 {
		t.Errorf("missing: want 3, got %d", result.MissingCount)
	}
	if result.DriftCount != 0 {
		t.Errorf("drift: want 0, got %d", result.DriftCount)
	}
}

func TestBulkService_Recompute_Empty(t *testing.T) {
	svc := NewBulkService(nil, newStubInstrumenRepo(), &stubScheduleRepo{}, nil, nil, testLogger())
	result, err := svc.Recompute(context.Background(), BulkScopeAllActive, "job-empty", uuid.New())
	if err != nil {
		t.Fatalf("Recompute: %v", err)
	}
	if result.TotalInstruments != 0 {
		t.Errorf("total: want 0, got %d", result.TotalInstruments)
	}
}

// ─── isEIRApplicable tests ────────────────────────────────────────────────────

func TestIsEIRApplicable(t *testing.T) {
	cases := []struct {
		klas string
		flag bool
		want bool
	}{
		{"AC", true, true},
		{"FVOCI", true, true},
		{"FVTPL", true, false},
		{"AC", false, false},
		{"FVOCI_ELECTION", true, false},
	}
	for _, c := range cases {
		got := isEIRApplicable(c.klas, c.flag)
		if got != c.want {
			t.Errorf("isEIRApplicable(%q, %v) = %v, want %v", c.klas, c.flag, got, c.want)
		}
	}
}

// ─── Assert helpers ───────────────────────────────────────────────────────────

func assertDomainErr(t *testing.T, err error, expectedCode string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %s, got nil", expectedCode)
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError, got %T: %v", err, err)
	}
	if string(de.Code()) != expectedCode {
		t.Errorf("expected code %s, got %s", expectedCode, de.Code())
	}
}
