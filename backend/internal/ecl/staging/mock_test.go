// Package staging_test — in-process mock implementations for service-layer testing.
//
// These mocks implement the repository interfaces defined in repo.go without any
// real database connection.  They are intentionally simple (in-memory maps) to
// keep test dependencies minimal (no gomock code-gen dependency).
package staging_test

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
	"blips-ifrs9.tugu-re.com/internal/ecl/staging"
)

// ─── Auth context helpers ─────────────────────────────────────────────────────

// ctxWithActor injects a minimal auth.Claims into context (no step-up).
func ctxWithActor(sub, role, tenantID string) context.Context {
	return auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub:      sub,
		Roles:    []string{role},
		TenantID: tenantID,
	})
}

// ctxWithStepUp injects a Claims with a fresh step-up timestamp.
func ctxWithStepUp(sub, role, tenantID string) context.Context {
	now := time.Now().Unix()
	return auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub:              sub,
		Roles:            []string{role},
		TenantID:         tenantID,
		MFAVerified:      true,
		StepupVerifiedAt: &now,
	})
}

// ─── noopAuditWriter ─────────────────────────────────────────────────────────

// noopAuditWriter creates an audit.Writer backed by a nil *sql.DB; writes are
// silently skipped (the writer checks for nil db).
func noopAuditWriter() *audit.Writer {
	return audit.NewWriter(nil)
}

// trackingAuditContainer wraps audit.Writer so tests can verify it was passed.
// Because audit.Writer does not expose an interface, we use the noop (nil-DB)
// variant and just check that the object is the same pointer that was passed in.
type trackingAuditContainer struct {
	Writer *audit.Writer
}

// newTrackingAuditWriter returns a container holding a noop audit.Writer.
// Tests can pass container.Writer to constructors and later confirm no error path
// was triggered (the noop writer silently drops writes on nil DB).
func newTrackingAuditWriter() *trackingAuditContainer {
	return &trackingAuditContainer{Writer: audit.NewWriter(nil)}
}

// ─── noopLogger ──────────────────────────────────────────────────────────────

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(noopWriter{}, nil))
}

type noopWriter struct{}

func (noopWriter) Write(p []byte) (int, error) { return len(p), nil }

// ─── mockDPDRepo ─────────────────────────────────────────────────────────────

type mockDPDRepo struct {
	records   map[string]*staging.DPDRecord // key: instrumenID+periode
	latest    map[uuid.UUID]*staging.DPDRecord
	aboveCnt  int
	upsertErr error
}

func newMockDPDRepo() *mockDPDRepo {
	return &mockDPDRepo{
		records: make(map[string]*staging.DPDRecord),
		latest:  make(map[uuid.UUID]*staging.DPDRecord),
	}
}

func (m *mockDPDRepo) BeginTx(ctx context.Context) (*sql.Tx, error) { return beginNoopTx(ctx) }

func (m *mockDPDRepo) UpsertDPD(_ context.Context, _ *sql.Tx, rec *staging.DPDRecord) (*staging.DPDRecord, error) {
	if m.upsertErr != nil {
		return nil, m.upsertErr
	}
	if rec.ID == uuid.Nil {
		rec.ID = uuid.New()
	}
	key := rec.InstrumenID.String() + rec.Periode.Format("2006-01-02")
	m.records[key] = rec
	m.latest[rec.InstrumenID] = rec
	return rec, nil
}

func (m *mockDPDRepo) GetLatestDPD(_ context.Context, instrumenID uuid.UUID) (*staging.DPDRecord, error) {
	if r, ok := m.latest[instrumenID]; ok {
		return r, nil
	}
	return nil, staging.ErrNotFound
}

func (m *mockDPDRepo) GetDPDForPeriode(_ context.Context, instrumenID uuid.UUID, periode time.Time) (*staging.DPDRecord, error) {
	key := instrumenID.String() + periode.Format("2006-01-02")
	if r, ok := m.records[key]; ok {
		return r, nil
	}
	return nil, staging.ErrNotFound
}

func (m *mockDPDRepo) ListDPD(_ context.Context, _ uuid.UUID, _ listquery.Query, _ string, limit int) ([]*staging.DPDRecord, pagination.Result, error) {
	return nil, pagination.Result{Limit: limit}, nil
}

func (m *mockDPDRepo) CountDPDAboveThreshold(_ context.Context, _ uuid.UUID, _, _ time.Time, _ int) (int, error) {
	return m.aboveCnt, nil
}

// ─── mockHistRepo ─────────────────────────────────────────────────────────────

type mockHistRepo struct {
	rows           []*staging.StageHistoryEntry
	insertErr      error
	insertConflict bool
	sicrDate       *time.Time
	hasSICR        bool
	stage2IDs      []uuid.UUID
}

func newMockHistRepo() *mockHistRepo { return &mockHistRepo{} }

func (m *mockHistRepo) BeginTx(ctx context.Context) (*sql.Tx, error) { return beginNoopTx(ctx) }

func (m *mockHistRepo) Insert(_ context.Context, _ *sql.Tx, entry *staging.StageHistoryEntry) (*staging.StageHistoryEntry, error) {
	if m.insertConflict {
		return nil, staging.ErrConflict
	}
	if m.insertErr != nil {
		return nil, m.insertErr
	}
	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}
	m.rows = append(m.rows, entry)
	return entry, nil
}

func (m *mockHistRepo) GetCurrentStage(_ context.Context, instrumenID uuid.UUID) (*staging.StageHistoryEntry, error) {
	var last *staging.StageHistoryEntry
	for _, r := range m.rows {
		if r.InstrumenID == instrumenID {
			last = r
		}
	}
	return last, nil
}

func (m *mockHistRepo) ListHistory(_ context.Context, _ uuid.UUID, _ listquery.Query, _ string, limit int, _ bool) ([]*staging.StageHistoryEntry, pagination.Result, error) {
	return nil, pagination.Result{Limit: limit}, nil
}

func (m *mockHistRepo) GetLastSICRDate(_ context.Context, _ uuid.UUID) (*time.Time, error) {
	return m.sicrDate, nil
}

func (m *mockHistRepo) HasSICRInPeriode(_ context.Context, _ uuid.UUID, _, _ time.Time) (bool, error) {
	return m.hasSICR, nil
}

func (m *mockHistRepo) ExistsForKey(_ context.Context, _ uuid.UUID, _ time.Time, _ staging.TriggerType) (bool, error) {
	return false, nil
}

func (m *mockHistRepo) ListStage2Instruments(_ context.Context, _ string) ([]uuid.UUID, error) {
	return m.stage2IDs, nil
}

// ─── mockOverrideRepo ─────────────────────────────────────────────────────────

type mockOverrideRepo struct {
	proposals map[uuid.UUID]*staging.OverrideProposal
	active    []*staging.OverrideProposal
	expired   []*staging.OverrideProposal
	createErr error
	getErr    error
}

func newMockOverrideRepo() *mockOverrideRepo {
	return &mockOverrideRepo{proposals: make(map[uuid.UUID]*staging.OverrideProposal)}
}

func (m *mockOverrideRepo) BeginTx(ctx context.Context) (*sql.Tx, error) { return beginNoopTx(ctx) }

func (m *mockOverrideRepo) Create(_ context.Context, _ *sql.Tx, prop *staging.OverrideProposal) (*staging.OverrideProposal, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	if prop.ID == uuid.Nil {
		prop.ID = uuid.New()
	}
	m.proposals[prop.ID] = prop
	return prop, nil
}

func (m *mockOverrideRepo) GetByID(_ context.Context, id uuid.UUID, _ bool) (*staging.OverrideProposal, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if p, ok := m.proposals[id]; ok {
		return p, nil
	}
	return nil, staging.ErrNotFound
}

func (m *mockOverrideRepo) UpdateWorkflowStatus(_ context.Context, _ *sql.Tx, id uuid.UUID, newStatus staging.OverrideWorkflowStatus, _ uuid.UUID, _ time.Time, _ []byte, _ *string) error {
	if p, ok := m.proposals[id]; ok {
		p.WorkflowStatus = newStatus
	}
	return nil
}

func (m *mockOverrideRepo) ActivateWithHistoryRow(_ context.Context, _ *sql.Tx, id, histID uuid.UUID, _ uuid.UUID) error {
	if p, ok := m.proposals[id]; ok {
		p.WorkflowStatus = staging.OverrideStatusActive
		p.StageHistoryRowID = &histID
	}
	return nil
}

func (m *mockOverrideRepo) ListActiveForInstrumen(_ context.Context, _ uuid.UUID) ([]*staging.OverrideProposal, error) {
	return m.active, nil
}

func (m *mockOverrideRepo) ListOverrides(_ context.Context, _ listquery.Query, _ string, limit int) ([]*staging.OverrideProposal, pagination.Result, error) {
	return nil, pagination.Result{Limit: limit}, nil
}

func (m *mockOverrideRepo) ListExpiredActive(_ context.Context, _ time.Time) ([]*staging.OverrideProposal, error) {
	return m.expired, nil
}

func (m *mockOverrideRepo) MarkExpired(_ context.Context, _ *sql.Tx, id uuid.UUID, _ uuid.UUID) error {
	if p, ok := m.proposals[id]; ok {
		p.WorkflowStatus = staging.OverrideStatusExpired
	}
	return nil
}

// ─── mockInstrumenReader ─────────────────────────────────────────────────────

type mockInstrumenReader struct {
	snap          *staging.InstrumenSnapshot
	getErr        error
	originRating  string
	currentRating string
	ratingErr     error
	origDate      time.Time
	origDateErr   error
}

func defaultMockInstrumen() *mockInstrumenReader {
	return &mockInstrumenReader{
		snap: &staging.InstrumenSnapshot{
			ID:                uuid.New(),
			KlasifikasiPSAK71: "AC",
			Status:            "AKTIF",
			TanggalPenempatan: time.Now().AddDate(-1, 0, 0),
			TenantID:          "TUGURE",
		},
		origDate: time.Now().AddDate(-1, 0, 0),
	}
}

func (m *mockInstrumenReader) GetByID(_ context.Context, id uuid.UUID) (*staging.InstrumenSnapshot, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.snap != nil {
		snap := *m.snap
		snap.ID = id
		return &snap, nil
	}
	return nil, staging.ErrNotFound
}

func (m *mockInstrumenReader) GetRatingAtDate(_ context.Context, _ uuid.UUID, asOf time.Time) (string, error) {
	if m.ratingErr != nil {
		return "", m.ratingErr
	}
	if !m.origDate.IsZero() && (asOf.Before(m.origDate.Add(48 * time.Hour))) {
		return m.originRating, nil
	}
	return m.currentRating, nil
}

func (m *mockInstrumenReader) GetOriginationDate(_ context.Context, _ uuid.UUID) (time.Time, error) {
	if m.origDateErr != nil {
		return time.Time{}, m.origDateErr
	}
	if !m.origDate.IsZero() {
		return m.origDate, nil
	}
	return time.Now().AddDate(-1, 0, 0), nil
}

// ─── mockPeriodeReader ────────────────────────────────────────────────────────

type mockPeriodeReader struct {
	periods         []time.Time
	err             error
	tanggalAkhir    time.Time
	tanggalAkhirErr error
}

func (m *mockPeriodeReader) ListClosedBulananSince(_ context.Context, _ time.Time, _ string) ([]time.Time, error) {
	return m.periods, m.err
}

// GetTanggalAkhirByID returns tanggalAkhir for the mock periode reader.
// If tanggalAkhirErr is set, returns that error.
// If tanggalAkhir is zero, falls back to 1 year from now (permissive default for tests
// that do not care about this field).
func (m *mockPeriodeReader) GetTanggalAkhirByID(_ context.Context, _ uuid.UUID) (time.Time, error) {
	if m.tanggalAkhirErr != nil {
		return time.Time{}, m.tanggalAkhirErr
	}
	if !m.tanggalAkhir.IsZero() {
		return m.tanggalAkhir, nil
	}
	return time.Now().AddDate(1, 0, 0), nil
}

// ─── mockExpiredErrRepo ───────────────────────────────────────────────────────

// mockExpiredErrRepo is an override repo variant where MarkExpired always fails.
// Used to cover the error branch in HandleOverrideExpiryCheck.
type mockExpiredErrRepo struct {
	*mockOverrideRepo
	markExpiredErr error
}

func newMockExpiredErrRepo() *mockExpiredErrRepo {
	return &mockExpiredErrRepo{
		mockOverrideRepo: newMockOverrideRepo(),
		markExpiredErr:   fmt.Errorf("mark expired DB error"),
	}
}

func (m *mockExpiredErrRepo) MarkExpired(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID) error {
	return m.markExpiredErr
}

// ─── noopEnqueuer ─────────────────────────────────────────────────────────────

// noopEnqueuer implements staging.TaskEnqueuer for tests that need a non-nil enqueuer
// (to exercise the Asynq dispatch path) without a real Redis connection.
type noopEnqueuer struct {
	enqueuedCount int
}

func (e *noopEnqueuer) EnqueueContext(_ interface{}, _ *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	e.enqueuedCount++
	return &asynq.TaskInfo{}, nil
}

// ─── newTestService ───────────────────────────────────────────────────────────

func newTestService(
	dpdRepo staging.DPDRepository,
	histRepo staging.StageHistoryRepository,
	overrideRepo staging.OverrideProposalRepository,
	instrumen staging.InstrumenReader,
	periode staging.PeriodeBukuReader,
) *staging.Service {
	return staging.NewService(
		dpdRepo,
		histRepo,
		overrideRepo,
		instrumen,
		periode,
		noopAuditWriter(),
		noopLogger(),
	)
}
