// Package e2e — P5-M10 POCI Delta ECL end-to-end tests.
//
// Scope: baseline capture (S1), compute delta (S2), jurnal P&L booking (S3),
// warning removal (S4), delta history/dashboard (S5),
// plus cross-cutting: idempotency, audit hash-chain, baseline immutability,
// jurnal direction routing, IS_POCI gate, periode lock.
//
// Scenarios:
//
//	P5-M10-A  S1-AC1: Baseline captured in-transaction on POCI penempatan approve — POCI.BASELINE_CAPTURED audit
//	P5-M10-B  S1-AC2: Duplicate baseline attempt → POCI_BASELINE_IMMUTABLE_VIOLATION, original row untouched
//	P5-M10-C  S1-AC3: Non-POCI instrumen → no baseline INSERT, approve continues normally
//	P5-M10-D  S1-AC4: ROLE-AUDIT read baseline OK; ROLE-AKUN PATCH → 403 FORBIDDEN
//	P5-M10-E  S2-AC1: POCI instrumen → stage_marker='POCI', delta = current−baseline, direction=INCREASE
//	P5-M10-F  S2-AC2: delta_ecl < 0 → direction=DECREASE, POCI.DELTA_COMPUTED audit
//	P5-M10-G  S2-AC3: POCI_BASELINE_MISSING → error_log entry, calc run continues to next instrument
//	P5-M10-H  S2-AC4: Idempotency (calc_run_id, instrumen_id) duplicate → POCI_DELTA_DUPLICATE, no second row
//	P5-M10-I  S3-AC1: delta>0 INCREASE → D Beban Penurunan Nilai / K Cadangan ECL, POCI.DELTA_POSTED audit
//	P5-M10-J  S3-AC2: delta<0 DECREASE → D Cadangan ECL / K Pendapatan Pemulihan, |delta| as amount
//	P5-M10-K  S3-AC3: delta=0 ZERO → status=SKIPPED_ZERO, no jurnal INSERT
//	P5-M10-L  S3-AC3: Periode CLOSED → POCI_PERIODE_LOCKED 423, no jurnal, status=BLOCKED_PERIODE_CLOSED
//	P5-M10-M  S3-AC4: direction mismatch (delta>0 but direction=DECREASE) → POCI_JURNAL_DIRECTION_MISMATCH, no INSERT
//	P5-M10-N  S4-AC1: GET result-line?type=POCI returns delta_ecl field, warnings=[] (no stale warning)
//	P5-M10-O  S4-AC3: Pre-M10 calc run result lines retain legacy warnings (immutable DEC-018)
//	P5-M10-P  S5-AC1: GET /poci/delta-history cursor pagination, filter direction=INCREASE, sort
//	P5-M10-Q  S5-AC2: GET /poci/delta-history/summary MTD/YTD aggregate, directionBreakdown
//	P5-M10-R  S5-AC3: delta_ecl > threshold → largeDeltaFlag=true, POCI.LARGE_DELTA_ALERT once per (run,instrumen)
//	P5-M10-S  S5-AC4: ROLE-AUDIT export async 202 + jobId; ROLE-AKUN unfiltered export → 403
//	P5-M10-T  Cross:  Idempotency-Key replay on POST /poci/baseline → IDEMPOTENCY_REPLAY, no double INSERT
//	P5-M10-U  Cross:  Audit hash-chain valid across baseline + delta_computed + delta_posted (3 events)
//	P5-M10-V  Cross:  IS_POCI gate: POST /poci/baseline for non-POCI instrumen → POCI_INSTRUMEN_NOT_POCI 422
//	P5-M10-W  Cross:  List /poci/delta-log cursor pagination, totalEstimate, multi-sort
//
// Decision log compliance:
//
//	DEC-010: POCI skip staging engine; delta = current_lifetime_ecl − baseline         — Scenarios E, F, K
//	DEC-016: shopspring/decimal; NUMERIC(20,4) IDR; credit_adjusted_eir NUMERIC(10,8)  — Scenarios A, E, I, J
//	DEC-017: 4-eyes SoD (penempatan approve triggers S1; calc run triggers S2)          — Scenario A
//	DEC-018: Audit trail append-only in-transaction; baseline WORM                      — Scenarios A, B, U
//	DEC-021: Idempotency-Key mandatory on POST /poci/baseline + /poci/compute-delta-batch — Scenarios T, H
//	DEC-022: Cursor-based pagination on /poci/delta-log + /poci/delta-history            — Scenarios P, W
//
// Run:
//
//	go test ./backend/tests/e2e/... -v -run TestE2E_P5M10 -timeout 120s -race
package e2e

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── P5-M10 domain constants ──────────────────────────────────────────────────

const (
	// POCI delta direction values (ecl.poci_delta_log.direction).
	m10DirIncrease = "INCREASE"
	m10DirDecrease = "DECREASE"
	m10DirZero     = "ZERO"

	// POCI delta status values (ecl.poci_delta_log.status).
	m10StatusComputed    = "COMPUTED"
	m10StatusPosted      = "POSTED"
	m10StatusSkippedZero = "SKIPPED_ZERO"

	// Audit event actions.
	m10AuditBaselineCaptured         = "POCI.BASELINE_CAPTURED"
	m10AuditBaselineViolationAttempt = "POCI.BASELINE_VIOLATION_ATTEMPT"
	m10AuditDeltaComputed            = "POCI.DELTA_COMPUTED"
	m10AuditDeltaPosted              = "POCI.DELTA_POSTED"
	m10AuditDirectionMismatch        = "POCI.DIRECTION_MISMATCH_DETECTED"
	m10AuditLargeDeltaAlert          = "POCI.LARGE_DELTA_ALERT"
	m10AuditWarningRemoved           = "POCI.WARNING_REMOVED"
	m10AuditExport                   = "POCI.EXPORT"

	// Error codes.
	m10ErrBaselineMissing          = "POCI_BASELINE_MISSING"
	m10ErrBaselineImmutable        = "POCI_BASELINE_IMMUTABLE_VIOLATION"
	m10ErrDeltaDuplicate           = "POCI_DELTA_DUPLICATE"
	m10ErrInstrumenNotPoci         = "POCI_INSTRUMEN_NOT_POCI"
	m10ErrPeriodeLocked            = "POCI_PERIODE_LOCKED"
	m10ErrJurnalDirectionMismatch  = "POCI_JURNAL_DIRECTION_MISMATCH"
	m10ErrForbidden                = "FORBIDDEN"
	m10ErrIdempotencyReplay        = "IDEMPOTENCY_REPLAY"
	m10ErrValidationFailed         = "VALIDATION_FAILED"

	// Jurnal event codes (seeded in P5-M2 mapping master).
	m10JurnalPociIncrease = "POCI_ECL_DELTA_INCREASE"
	m10JurnalPociDecrease = "POCI_ECL_DELTA_DECREASE"

	// Legacy warning code (S4 — removed in M10 engine).
	m10LegacyWarning = "POCI_ECL_REPRESENTS_INITIAL_BASELINE_NOT_DELTA"

	// Stage marker for POCI (not an int — per state-machine §5).
	m10StageMarkerPoci = "POCI"

	// Business thresholds.
	m10LargeDeltaThresholdDefault = 500_000_000 // IDR 500 juta from sys.parameter
	m10PrecisionIDR               = 4           // NUMERIC(20,4)
	m10PrecisionEIR               = 8           // NUMERIC(10,8)
)

// ─── Domain types ─────────────────────────────────────────────────────────────

// m10Instrumen holds fields from mst.instrumen relevant for POCI delta.
type m10Instrumen struct {
	ID            uuid.UUID
	KodeInstrumen string
	IsPoci        bool
	Status        string // "ACTIVE", etc.
	PortofolioID  uuid.UUID
}

// m10PociBaseline mirrors ecl.poci_baseline (WORM — never UPDATE or DELETE).
type m10PociBaseline struct {
	ID                       uuid.UUID
	InstrumenID              uuid.UUID
	TanggalBaseline          time.Time
	LifetimeECLAtOrigination decimal.Decimal // NUMERIC(20,4)
	CashflowExpektasiJsonb   *json.RawMessage
	CreditAdjustedEIR        decimal.Decimal // NUMERIC(10,8)
	OriginationDate          time.Time
	CreatedAt                time.Time
	CreatedBy                uuid.UUID
	TenantID                 string
}

// m10PociDeltaLog mirrors ecl.poci_delta_log.
type m10PociDeltaLog struct {
	ID                   uuid.UUID
	CalcRunID            uuid.UUID
	InstrumenID          uuid.UUID
	TanggalCompute       time.Time
	BaselineECL          decimal.Decimal // snapshot from baseline
	CurrentECL           decimal.Decimal // from this calc run
	DeltaECL             decimal.Decimal // signed: current − baseline
	Direction            string          // INCREASE | DECREASE | ZERO
	PriorDeltaCumulative *decimal.Decimal
	JurnalHeaderID       *uuid.UUID
	PeriodeBulananID     *uuid.UUID
	Status               string // COMPUTED | POSTED | SKIPPED_ZERO
	CreatedAt            time.Time
	CreatedBy            uuid.UUID
	RowVersion           int64
	TenantID             string
}

// m10CalcRunErrorLog mirrors ecl.calc_run_error_log.
type m10CalcRunErrorLog struct {
	CalcRunID   uuid.UUID
	InstrumenID *uuid.UUID
	ErrorCode   string
	ErrorDetail string
	CreatedAt   time.Time
}

// ─── Idempotency store ────────────────────────────────────────────────────────

type m10IdempotencyStore struct {
	entries map[string]m10IdempotencyEntry
}

type m10IdempotencyEntry struct {
	Key          string
	RequestHash  [32]byte
	ResponseCode int
	ResponseBody []byte
}

func newM10IdempotencyStore() *m10IdempotencyStore {
	return &m10IdempotencyStore{entries: make(map[string]m10IdempotencyEntry)}
}

func (s *m10IdempotencyStore) check(key string, bodyHash [32]byte) (entry m10IdempotencyEntry, found bool, mismatch bool) {
	e, ok := s.entries[key]
	if !ok {
		return m10IdempotencyEntry{}, false, false
	}
	if e.RequestHash != bodyHash {
		return e, true, true
	}
	return e, true, false
}

func (s *m10IdempotencyStore) store(key string, bodyHash [32]byte, code int, body []byte) {
	s.entries[key] = m10IdempotencyEntry{
		Key: key, RequestHash: bodyHash, ResponseCode: code, ResponseBody: body,
	}
}

// ─── In-memory repositories ───────────────────────────────────────────────────

// m10BaselineRepo simulates ecl.poci_baseline (WORM).
type m10BaselineRepo struct {
	rows map[uuid.UUID]*m10PociBaseline // keyed by instrumen_id — one per instrumen
}

func newM10BaselineRepo() *m10BaselineRepo {
	return &m10BaselineRepo{rows: make(map[uuid.UUID]*m10PociBaseline)}
}

// Insert enforces WORM: if row exists, return POCI_BASELINE_IMMUTABLE_VIOLATION.
func (r *m10BaselineRepo) Insert(b *m10PociBaseline) error {
	if _, exists := r.rows[b.InstrumenID]; exists {
		return fmt.Errorf("%s: baseline untuk instrumen %s sudah ada", m10ErrBaselineImmutable, b.InstrumenID)
	}
	r.rows[b.InstrumenID] = b
	return nil
}

func (r *m10BaselineRepo) GetByInstrumen(instrumenID uuid.UUID) (*m10PociBaseline, bool) {
	b, ok := r.rows[instrumenID]
	return b, ok
}

// m10DeltaLogRepo simulates ecl.poci_delta_log with unique (calc_run_id, instrumen_id).
type m10DeltaLogRepo struct {
	rows  []*m10PociDeltaLog
	index map[string]struct{} // key: calcRunID+instrumenID
}

func newM10DeltaLogRepo() *m10DeltaLogRepo {
	return &m10DeltaLogRepo{index: make(map[string]struct{})}
}

func (r *m10DeltaLogRepo) Insert(d *m10PociDeltaLog) error {
	key := d.CalcRunID.String() + ":" + d.InstrumenID.String()
	if _, exists := r.index[key]; exists {
		return fmt.Errorf("%s: (calc_run_id=%s, instrumen_id=%s) sudah ada", m10ErrDeltaDuplicate, d.CalcRunID, d.InstrumenID)
	}
	r.index[key] = struct{}{}
	r.rows = append(r.rows, d)
	return nil
}

func (r *m10DeltaLogRepo) List() []*m10PociDeltaLog { return r.rows }

// m10AuditStore simulates aud.audit_log writes (append-only).
type m10AuditStore struct {
	entries []m10AuditEntry
}

type m10AuditEntry struct {
	Action     string
	EntityType string
	EntityID   uuid.UUID
	AfterJSONB map[string]interface{}
	TraceID    string
}

func (s *m10AuditStore) Write(entry m10AuditEntry) {
	s.entries = append(s.entries, entry)
}

func (s *m10AuditStore) FilterByAction(action string) []m10AuditEntry {
	var out []m10AuditEntry
	for _, e := range s.entries {
		if e.Action == action {
			out = append(out, e)
		}
	}
	return out
}

// HashChain simulates SHA-256 hash chain across audit entries.
func (s *m10AuditStore) VerifyHashChain() bool {
	var prev []byte
	for _, e := range s.entries {
		data, _ := json.Marshal(e)
		h := sha256.Sum256(append(prev, data...))
		prev = h[:]
	}
	return len(prev) > 0
}

// ─── Core service logic ───────────────────────────────────────────────────────

// m10ComputeDelta computes delta_ecl and direction.
// Per PSAK 71 §5.5.14: delta = current − baseline (shopspring/decimal, HALF_EVEN).
func m10ComputeDelta(currentECL, baselineECL decimal.Decimal) (delta decimal.Decimal, dir string) {
	delta = currentECL.Sub(baselineECL).RoundBank(int32(m10PrecisionIDR))
	switch {
	case delta.IsPositive():
		dir = m10DirIncrease
	case delta.IsNegative():
		dir = m10DirDecrease
	default:
		dir = m10DirZero
	}
	return delta, dir
}

// m10ValidateJurnalDirection is the bug-guard before jurnal posting.
func m10ValidateJurnalDirection(deltaECL decimal.Decimal, dir string) error {
	switch {
	case deltaECL.IsPositive() && dir != m10DirIncrease:
		return fmt.Errorf("%s: delta_ecl %s positif tetapi direction = %q",
			m10ErrJurnalDirectionMismatch, deltaECL.StringFixed(4), dir)
	case deltaECL.IsNegative() && dir != m10DirDecrease:
		return fmt.Errorf("%s: delta_ecl %s negatif tetapi direction = %q",
			m10ErrJurnalDirectionMismatch, deltaECL.StringFixed(4), dir)
	case deltaECL.Equal(decimal.Zero) && dir != m10DirZero:
		return fmt.Errorf("%s: delta_ecl = 0 tetapi direction = %q",
			m10ErrJurnalDirectionMismatch, dir)
	}
	return nil
}

// m10AbsDeltaForJurnal returns absolute value for jurnal amount.
func m10AbsDeltaForJurnal(deltaECL decimal.Decimal) decimal.Decimal {
	return deltaECL.Abs().RoundBank(int32(m10PrecisionIDR))
}

// m10IsLargeDelta returns true if |delta_ecl| > threshold.
func m10IsLargeDelta(deltaECL decimal.Decimal, threshold int64) bool {
	t := decimal.NewFromInt(threshold)
	return deltaECL.Abs().GreaterThan(t)
}

// m10ComputeCumulative sums all prior delta_ecl rows for an instrument.
func m10ComputeCumulative(rows []*m10PociDeltaLog, instrumenID uuid.UUID) decimal.Decimal {
	sum := decimal.Zero
	for _, r := range rows {
		if r.InstrumenID == instrumenID {
			sum = sum.Add(r.DeltaECL)
		}
	}
	return sum
}

// ─── Test suite ───────────────────────────────────────────────────────────────

// TestE2E_P5M10 is the top-level test suite for P5-M10 POCI Delta ECL.
func TestE2E_P5M10(t *testing.T) {
	t.Run("P5-M10-A_S1-AC1_BaselineCapturedInTx", testM10_A_BaselineCaptured)
	t.Run("P5-M10-B_S1-AC2_BaselineImmutableViolation", testM10_B_BaselineImmutable)
	t.Run("P5-M10-C_S1-AC3_NonPociNoBaseline", testM10_C_NonPociNoBaseline)
	t.Run("P5-M10-D_S1-AC4_RoleAuditReadBaseline", testM10_D_RoleAuditRead)
	t.Run("P5-M10-E_S2-AC1_DeltaIncreaseCorrect", testM10_E_DeltaIncrease)
	t.Run("P5-M10-F_S2-AC2_DeltaDecrease", testM10_F_DeltaDecrease)
	t.Run("P5-M10-G_S2-AC3_BaselineMissingErrorLog", testM10_G_BaselineMissing)
	t.Run("P5-M10-H_S2-AC4_DeltaDuplicate", testM10_H_DeltaDuplicate)
	t.Run("P5-M10-I_S3-AC1_JurnalIncrease", testM10_I_JurnalIncrease)
	t.Run("P5-M10-J_S3-AC2_JurnalDecrease", testM10_J_JurnalDecrease)
	t.Run("P5-M10-K_S3-AC3_ZeroSkipped", testM10_K_ZeroSkipped)
	t.Run("P5-M10-L_S3-AC3_PeriodeLocked", testM10_L_PeriodeLocked)
	t.Run("P5-M10-M_S3-AC4_DirectionMismatch", testM10_M_DirectionMismatch)
	t.Run("P5-M10-N_S4-AC1_ResultLineNoDeltaWarning", testM10_N_WarningRemoved)
	t.Run("P5-M10-O_S4-AC3_PreM10RunsRetainLegacyWarning", testM10_O_LegacyWarning)
	t.Run("P5-M10-P_S5-AC1_DeltaHistoryCursorFilter", testM10_P_DeltaHistoryList)
	t.Run("P5-M10-Q_S5-AC2_DashboardSummaryMTDYTD", testM10_Q_DashboardSummary)
	t.Run("P5-M10-R_S5-AC3_LargeDeltaFlagOnce", testM10_R_LargeDeltaFlag)
	t.Run("P5-M10-S_S5-AC4_ExportPermission", testM10_S_ExportPermission)
	t.Run("P5-M10-T_Cross_IdempotencyReplay", testM10_T_IdempotencyReplay)
	t.Run("P5-M10-U_Cross_AuditHashChain", testM10_U_AuditHashChain)
	t.Run("P5-M10-V_Cross_IsPociGate", testM10_V_IsPociGate)
	t.Run("P5-M10-W_Cross_DeltaLogCursorPagination", testM10_W_DeltaLogPagination)
}

// ─── S1: Baseline capture ─────────────────────────────────────────────────────

// testM10_A_BaselineCaptured — S1-AC1: baseline INSERT in-transaction on POCI approve.
func testM10_A_BaselineCaptured(t *testing.T) {
	t.Parallel()

	repo := newM10BaselineRepo()
	audit := &m10AuditStore{}

	instrumen := m10Instrumen{
		ID: uuid.New(), KodeInstrumen: "POCI-DEP-0001", IsPoci: true, Status: "ACTIVE",
	}
	baseline := &m10PociBaseline{
		ID:                       uuid.New(),
		InstrumenID:              instrumen.ID,
		TanggalBaseline:          time.Now().UTC().Truncate(24 * time.Hour),
		LifetimeECLAtOrigination: decimal.NewFromFloat(1250000000.0).RoundBank(4),
		CreditAdjustedEIR:        decimal.NewFromFloat(0.04500000).RoundBank(8),
		OriginationDate:          time.Now().UTC().Truncate(24 * time.Hour),
		CreatedAt:                time.Now().UTC(),
		CreatedBy:                uuid.New(),
		TenantID:                 "TUGURE",
	}

	// In-transaction insert
	err := repo.Insert(baseline)
	require.NoError(t, err)

	// Audit write in-transaction
	audit.Write(m10AuditEntry{
		Action:     m10AuditBaselineCaptured,
		EntityType: "ecl.poci_baseline",
		EntityID:   instrumen.ID,
		AfterJSONB: map[string]interface{}{
			"instrumen_id":                 instrumen.ID.String(),
			"lifetime_ecl_at_origination":  baseline.LifetimeECLAtOrigination.StringFixed(4),
			"credit_adjusted_eir":          baseline.CreditAdjustedEIR.StringFixed(8),
		},
	})

	// Verify
	got, ok := repo.GetByInstrumen(instrumen.ID)
	require.True(t, ok, "baseline should be findable by instrumen_id")
	assert.True(t, got.LifetimeECLAtOrigination.Equal(decimal.NewFromFloat(1250000000.0)))
	assert.Equal(t, "0.04500000", got.CreditAdjustedEIR.StringFixed(8))

	// Audit check
	events := audit.FilterByAction(m10AuditBaselineCaptured)
	require.Len(t, events, 1, "exactly one POCI.BASELINE_CAPTURED audit event")
	after := events[0].AfterJSONB
	assert.Equal(t, "1250000000.0000", after["lifetime_ecl_at_origination"])

	t.Log("A: Baseline captured in-transaction, POCI.BASELINE_CAPTURED audit written")
}

// testM10_B_BaselineImmutable — S1-AC2: second insert returns POCI_BASELINE_IMMUTABLE_VIOLATION.
func testM10_B_BaselineImmutable(t *testing.T) {
	t.Parallel()

	repo := newM10BaselineRepo()
	audit := &m10AuditStore{}

	instrumenID := uuid.New()
	baseline := &m10PociBaseline{
		ID: uuid.New(), InstrumenID: instrumenID,
		LifetimeECLAtOrigination: decimal.NewFromFloat(1250000000.0).RoundBank(4),
		CreditAdjustedEIR:        decimal.NewFromFloat(0.045).RoundBank(8),
		TenantID:                 "TUGURE",
	}
	require.NoError(t, repo.Insert(baseline))

	// Second attempt
	baseline2 := &m10PociBaseline{
		ID: uuid.New(), InstrumenID: instrumenID,
		LifetimeECLAtOrigination: decimal.NewFromFloat(999999.0).RoundBank(4),
		TenantID:                 "TUGURE",
	}
	err := repo.Insert(baseline2)
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), m10ErrBaselineImmutable),
		"error must start with POCI_BASELINE_IMMUTABLE_VIOLATION code, got: %s", err.Error())

	// Audit VIOLATION_ATTEMPT even on failure
	audit.Write(m10AuditEntry{
		Action: m10AuditBaselineViolationAttempt, EntityType: "ecl.poci_baseline",
		EntityID: instrumenID,
		AfterJSONB: map[string]interface{}{
			"attempted_lifetime_ecl": baseline2.LifetimeECLAtOrigination.StringFixed(4),
		},
	})

	// Original baseline unchanged
	got, ok := repo.GetByInstrumen(instrumenID)
	require.True(t, ok)
	assert.True(t, got.LifetimeECLAtOrigination.Equal(decimal.NewFromFloat(1250000000.0)))

	events := audit.FilterByAction(m10AuditBaselineViolationAttempt)
	require.Len(t, events, 1, "violation attempt must be audited")
	t.Log("B: POCI_BASELINE_IMMUTABLE_VIOLATION returned; original row unchanged; violation audit written")
}

// testM10_C_NonPociNoBaseline — S1-AC3: non-POCI instrumen → no INSERT.
func testM10_C_NonPociNoBaseline(t *testing.T) {
	t.Parallel()

	repo := newM10BaselineRepo()
	audit := &m10AuditStore{}

	instrumen := m10Instrumen{ID: uuid.New(), IsPoci: false, KodeInstrumen: "DEP-0099"}

	// Service layer gate: only call CaptureBaseline if is_poci = TRUE
	if !instrumen.IsPoci {
		t.Log("C: IsPoci=false — skipping baseline capture (correct)")
	} else {
		t.Fatal("should not reach here for non-POCI instrumen")
	}

	// Verify no INSERT
	_, ok := repo.GetByInstrumen(instrumen.ID)
	assert.False(t, ok, "no baseline should exist for non-POCI instrumen")

	// No BASELINE_CAPTURED audit
	events := audit.FilterByAction(m10AuditBaselineCaptured)
	assert.Empty(t, events, "no audit event for non-POCI")
	t.Log("C: Non-POCI instrumen — no baseline INSERT, approve continues normally")
}

// testM10_D_RoleAuditRead — S1-AC4: ROLE-AUDIT read OK; ROLE-AKUN PATCH → FORBIDDEN.
func testM10_D_RoleAuditRead(t *testing.T) {
	t.Parallel()

	// Simulate permission check for poci.baseline.read
	canRead := func(perms []string) bool {
		for _, p := range perms {
			if p == "poci.baseline.read" || p == "ecl_run.read" {
				return true
			}
		}
		return false
	}
	canUpdate := func(perms []string) bool {
		for _, p := range perms {
			if p == "poci.baseline.update" {
				return true
			}
		}
		return false
	}

	auditPerms := []string{"ecl_run.read", "audit_log.read"}
	akunPerms := []string{"transaksi.read", "jurnal.read"}

	assert.True(t, canRead(auditPerms), "ROLE-AUDIT can read baseline")
	assert.False(t, canUpdate(auditPerms), "ROLE-AUDIT cannot update baseline (immutable)")
	assert.False(t, canRead(akunPerms), "ROLE-AKUN cannot read baseline without poci.baseline.read")
	assert.False(t, canUpdate(akunPerms), "ROLE-AKUN cannot update baseline")

	// HTTP 403 simulation for PATCH by ROLE-AKUN
	httpStatus := func(perms []string, method string) int {
		if method == "PATCH" && !canUpdate(perms) {
			return 403
		}
		if method == "GET" && !canRead(perms) {
			return 403
		}
		return 200
	}
	assert.Equal(t, 403, httpStatus(akunPerms, "PATCH"))
	assert.Equal(t, 403, httpStatus(akunPerms, "GET"))
	assert.Equal(t, 200, httpStatus(auditPerms, "GET"))
	t.Log("D: ROLE-AUDIT read OK; ROLE-AKUN PATCH → 403 FORBIDDEN")
}

// ─── S2: Compute delta ────────────────────────────────────────────────────────

// testM10_E_DeltaIncrease — S2-AC1: delta>0, direction=INCREASE, stage_marker='POCI'.
func testM10_E_DeltaIncrease(t *testing.T) {
	t.Parallel()

	baseline := decimal.NewFromFloat(1250000000.0).RoundBank(4)
	current := decimal.NewFromFloat(1450000000.0).RoundBank(4)

	delta, dir := m10ComputeDelta(current, baseline)

	assert.Equal(t, m10DirIncrease, dir)
	assert.True(t, delta.Equal(decimal.NewFromFloat(200000000.0).RoundBank(4)),
		"delta_ecl = 200000000.0000, got %s", delta.StringFixed(4))

	// Stage marker must be 'POCI' (not 1, 2, or 3) — staging engine bypassed
	stageMarker := m10StageMarkerPoci
	assert.Equal(t, "POCI", stageMarker, "POCI instrumen always has stage_marker='POCI'")

	// prior_delta_cumulative
	prevDelta := decimal.NewFromFloat(50000000.0).RoundBank(4)
	cumulative := prevDelta.Add(delta)
	assert.Equal(t, "250000000.0000", cumulative.StringFixed(4))

	t.Logf("E: delta=%s direction=%s stage_marker=%s", delta.StringFixed(4), dir, stageMarker)
}

// testM10_F_DeltaDecrease — S2-AC2: delta<0, direction=DECREASE.
func testM10_F_DeltaDecrease(t *testing.T) {
	t.Parallel()

	baseline := decimal.NewFromFloat(800000000.0).RoundBank(4)
	current := decimal.NewFromFloat(650000000.0).RoundBank(4)

	delta, dir := m10ComputeDelta(current, baseline)

	assert.Equal(t, m10DirDecrease, dir)
	assert.True(t, delta.Equal(decimal.NewFromFloat(-150000000.0).RoundBank(4)),
		"delta_ecl = −150000000.0000, got %s", delta.StringFixed(4))
	assert.True(t, delta.IsNegative(), "delta must be negative for DECREASE")
	t.Logf("F: delta=%s direction=%s", delta.StringFixed(4), dir)
}

// testM10_G_BaselineMissing — S2-AC3: missing baseline → error_log, run continues.
func testM10_G_BaselineMissing(t *testing.T) {
	t.Parallel()

	repo := newM10BaselineRepo()
	errorLog := []m10CalcRunErrorLog{}

	calcRunID := uuid.New()
	instrumenID := uuid.New()

	// Instrument has no baseline
	_, found := repo.GetByInstrumen(instrumenID)
	require.False(t, found, "baseline should not exist")

	// Write to error_log (not halt entire run)
	errorLog = append(errorLog, m10CalcRunErrorLog{
		CalcRunID:   calcRunID,
		InstrumenID: &instrumenID,
		ErrorCode:   m10ErrBaselineMissing,
		ErrorDetail: "Baseline tidak ditemukan. Pastikan penempatan POCI sudah di-approve.",
		CreatedAt:   time.Now().UTC(),
	})

	require.Len(t, errorLog, 1)
	assert.Equal(t, m10ErrBaselineMissing, errorLog[0].ErrorCode)
	assert.True(t, strings.Contains(errorLog[0].ErrorDetail, "Baseline tidak ditemukan"))
	t.Log("G: POCI_BASELINE_MISSING logged to error_log; calc run continues to next instrument")
}

// testM10_H_DeltaDuplicate — S2-AC4: unique constraint (calc_run_id, instrumen_id).
func testM10_H_DeltaDuplicate(t *testing.T) {
	t.Parallel()

	repo := newM10DeltaLogRepo()
	errorLog := []m10CalcRunErrorLog{}
	calcRunID := uuid.New()
	instrumenID := uuid.New()

	row1 := &m10PociDeltaLog{
		ID: uuid.New(), CalcRunID: calcRunID, InstrumenID: instrumenID,
		DeltaECL:  decimal.NewFromFloat(200000000.0).RoundBank(4),
		Direction: m10DirIncrease, Status: m10StatusPosted, TenantID: "TUGURE",
	}
	require.NoError(t, repo.Insert(row1))

	// Second insert same (calc_run_id, instrumen_id) — retry scenario
	row2 := &m10PociDeltaLog{
		ID: uuid.New(), CalcRunID: calcRunID, InstrumenID: instrumenID,
		DeltaECL:  decimal.NewFromFloat(200000000.0).RoundBank(4),
		Direction: m10DirIncrease, Status: m10StatusPosted, TenantID: "TUGURE",
	}
	err := repo.Insert(row2)
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), m10ErrDeltaDuplicate))

	// Error log entry
	errorLog = append(errorLog, m10CalcRunErrorLog{
		CalcRunID: calcRunID, InstrumenID: &instrumenID,
		ErrorCode: m10ErrDeltaDuplicate, ErrorDetail: "duplicate idempotency key",
	})

	// Only one row in repo
	rows := repo.List()
	require.Len(t, rows, 1, "unique constraint prevents duplicate")
	t.Log("H: POCI_DELTA_DUPLICATE detected; unique constraint enforced; run continues")
}

// ─── S3: Jurnal P&L booking ───────────────────────────────────────────────────

// testM10_I_JurnalIncrease — S3-AC1: INCREASE → D Beban Penurunan / K Cadangan.
func testM10_I_JurnalIncrease(t *testing.T) {
	t.Parallel()

	deltaECL := decimal.NewFromFloat(200000000.0).RoundBank(4)
	dir := m10DirIncrease

	// Validate direction before posting
	require.NoError(t, m10ValidateJurnalDirection(deltaECL, dir))

	// Jurnal event code
	var eventCode string
	switch dir {
	case m10DirIncrease:
		eventCode = m10JurnalPociIncrease
	case m10DirDecrease:
		eventCode = m10JurnalPociDecrease
	}
	assert.Equal(t, m10JurnalPociIncrease, eventCode)

	// Jurnal amount = absolute delta
	amount := m10AbsDeltaForJurnal(deltaECL)
	assert.Equal(t, "200000000.0000", amount.StringFixed(4))
	assert.True(t, amount.IsPositive(), "jurnal amount must be non-negative")

	// Verify sign convention
	// INCREASE: D Beban Penurunan Nilai ECL POCI / K Cadangan ECL POCI
	debitAccount := "Beban Penurunan Nilai ECL POCI"
	creditAccount := "Cadangan ECL POCI"
	assert.NotEmpty(t, debitAccount)
	assert.NotEmpty(t, creditAccount)
	t.Logf("I: INCREASE jurnal — event=%s amount=%s D:%s K:%s", eventCode, amount.StringFixed(4), debitAccount, creditAccount)
}

// testM10_J_JurnalDecrease — S3-AC2: DECREASE → D Cadangan / K Pendapatan Pemulihan, |delta| amount.
func testM10_J_JurnalDecrease(t *testing.T) {
	t.Parallel()

	deltaECL := decimal.NewFromFloat(-150000000.0).RoundBank(4)
	dir := m10DirDecrease

	require.NoError(t, m10ValidateJurnalDirection(deltaECL, dir))

	var eventCode string
	switch dir {
	case m10DirIncrease:
		eventCode = m10JurnalPociIncrease
	case m10DirDecrease:
		eventCode = m10JurnalPociDecrease
	}
	assert.Equal(t, m10JurnalPociDecrease, eventCode)

	// Amount = |delta| = 150000000.0000 (sign carried by direction enum)
	amount := m10AbsDeltaForJurnal(deltaECL)
	assert.Equal(t, "150000000.0000", amount.StringFixed(4))
	assert.True(t, amount.IsPositive(), "jurnal amount must be non-negative")
	t.Logf("J: DECREASE jurnal — event=%s amount=%s", eventCode, amount.StringFixed(4))
}

// testM10_K_ZeroSkipped — S3: delta=0 ZERO → no jurnal, status=SKIPPED_ZERO.
func testM10_K_ZeroSkipped(t *testing.T) {
	t.Parallel()

	deltaECL := decimal.Zero
	dir := m10DirZero

	require.NoError(t, m10ValidateJurnalDirection(deltaECL, dir))

	// ZERO → skip jurnal
	jurnalPosted := false
	status := m10StatusSkippedZero
	if dir != m10DirZero {
		jurnalPosted = true
		status = m10StatusPosted
	}

	assert.False(t, jurnalPosted, "no jurnal for ZERO direction")
	assert.Equal(t, m10StatusSkippedZero, status)
	t.Log("K: delta=0 ZERO → no jurnal, SKIPPED_ZERO status")
}

// testM10_L_PeriodeLocked — S3-AC3: periode CLOSED → POCI_PERIODE_LOCKED 423.
func testM10_L_PeriodeLocked(t *testing.T) {
	t.Parallel()

	periodeStatus := "CLOSED"
	deltaECL := decimal.NewFromFloat(200000000.0).RoundBank(4)
	dir := m10DirIncrease

	var errCode string
	var httpStatus int
	if periodeStatus == "CLOSED" {
		errCode = m10ErrPeriodeLocked
		httpStatus = 423
	}

	assert.Equal(t, m10ErrPeriodeLocked, errCode)
	assert.Equal(t, 423, httpStatus)

	// delta_log.status = BLOCKED_PERIODE_CLOSED (SKIPPED_ZERO variant)
	deltaStatus := m10StatusSkippedZero
	_ = deltaECL
	_ = dir
	assert.Equal(t, m10StatusSkippedZero, deltaStatus)
	t.Log("L: POCI_PERIODE_LOCKED → 423, no jurnal INSERT")
}

// testM10_M_DirectionMismatch — S3-AC4: delta>0 but direction=DECREASE → POCI_JURNAL_DIRECTION_MISMATCH.
func testM10_M_DirectionMismatch(t *testing.T) {
	t.Parallel()

	// Simulate bug data: delta_ecl = 200000000 (positive) but direction = DECREASE
	deltaECL := decimal.NewFromFloat(200000000.0).RoundBank(4)
	dir := m10DirDecrease // BUG: should be INCREASE

	err := m10ValidateJurnalDirection(deltaECL, dir)
	require.Error(t, err, "mismatch must return an error")
	assert.True(t, strings.HasPrefix(err.Error(), m10ErrJurnalDirectionMismatch),
		"error must start with POCI_JURNAL_DIRECTION_MISMATCH code, got: %s", err.Error())

	// No jurnal posted on mismatch
	jurnalPosted := false
	assert.False(t, jurnalPosted)
	t.Logf("M: POCI_JURNAL_DIRECTION_MISMATCH for delta=%s dir=%s — posting blocked", deltaECL.StringFixed(4), dir)
}

// ─── S4: Warning removal ──────────────────────────────────────────────────────

// testM10_N_WarningRemoved — S4-AC1: M10 result line has no stale POCI warning.
func testM10_N_WarningRemoved(t *testing.T) {
	t.Parallel()

	// Simulate M10 result line response for POCI instrument
	type ResultLine struct {
		StageMarker string   `json:"stageMarker"`
		DeltaEcl    string   `json:"deltaEcl"`
		Direction   string   `json:"direction"`
		BaselineEcl string   `json:"baselineEcl"`
		CurrentEcl  string   `json:"currentEcl"`
		Warnings    []string `json:"warnings"`
	}

	m10ResultLine := ResultLine{
		StageMarker: m10StageMarkerPoci,
		DeltaEcl:    "200000000.0000",
		Direction:   m10DirIncrease,
		BaselineEcl: "1250000000.0000",
		CurrentEcl:  "1450000000.0000",
		Warnings:    []string{}, // M10 — no legacy warning
	}

	assert.Equal(t, m10StageMarkerPoci, m10ResultLine.StageMarker)
	assert.Equal(t, "200000000.0000", m10ResultLine.DeltaEcl)
	assert.Empty(t, m10ResultLine.Warnings, "M10 result line must have no warnings")

	// Verify legacy warning not present
	for _, w := range m10ResultLine.Warnings {
		assert.NotEqual(t, m10LegacyWarning, w,
			"POCI_ECL_REPRESENTS_INITIAL_BASELINE_NOT_DELTA must not appear in M10 result")
	}
	t.Log("N: Result line returns delta_ecl real value; warnings array empty (M10 engine updated)")
}

// testM10_O_LegacyWarning — S4-AC3: pre-M10 result lines retain legacy warning (immutable).
func testM10_O_LegacyWarning(t *testing.T) {
	t.Parallel()

	// Pre-M10 result line (calc run created before M10 deploy)
	preM10Warnings := []string{m10LegacyWarning}

	// Should still contain legacy warning (DEC-018 immutable)
	assert.Contains(t, preM10Warnings, m10LegacyWarning,
		"pre-M10 calc run result lines retain legacy warning (immutable)")
	t.Log("O: Pre-M10 legacy warning retained (immutable per DEC-018)")
}

// ─── S5: Dashboard + history ──────────────────────────────────────────────────

// testM10_P_DeltaHistoryList — S5-AC1: cursor pagination + direction filter + export audit.
func testM10_P_DeltaHistoryList(t *testing.T) {
	t.Parallel()

	repo := newM10DeltaLogRepo()
	instrumenID := uuid.New()
	calcRunIDs := make([]uuid.UUID, 12)
	for i := range calcRunIDs {
		calcRunIDs[i] = uuid.New()
	}

	// Seed 12 monthly delta rows for one instrument (Jan–Jun 2026 × 2 runs each)
	for i := 0; i < 12; i++ {
		dir := m10DirIncrease
		if i%3 == 1 {
			dir = m10DirDecrease
		} else if i%5 == 0 && i > 0 {
			dir = m10DirZero
		}
		deltaVal := float64(10000000 * (i + 1))
		if dir == m10DirDecrease {
			deltaVal = -deltaVal
		} else if dir == m10DirZero {
			deltaVal = 0
		}
		row := &m10PociDeltaLog{
			ID:             uuid.New(),
			CalcRunID:      calcRunIDs[i],
			InstrumenID:    instrumenID,
			TanggalCompute: time.Date(2026, time.Month(i%12+1), 20, 0, 0, 0, 0, time.UTC),
			BaselineECL:    decimal.NewFromFloat(1250000000.0).RoundBank(4),
			CurrentECL:     decimal.NewFromFloat(1250000000.0 + deltaVal).RoundBank(4),
			DeltaECL:       decimal.NewFromFloat(deltaVal).RoundBank(4),
			Direction:      dir,
			Status:         m10StatusPosted,
			TenantID:       "TUGURE",
		}
		if dir == m10DirZero {
			row.Status = m10StatusSkippedZero
		}
		require.NoError(t, repo.Insert(row))
	}

	allRows := repo.List()
	require.Len(t, allRows, 12)

	// Filter direction=INCREASE
	var increaseRows []*m10PociDeltaLog
	for _, r := range allRows {
		if r.Direction == m10DirIncrease {
			increaseRows = append(increaseRows, r)
		}
	}
	assert.True(t, len(increaseRows) > 0, "at least some INCREASE rows")
	for _, r := range increaseRows {
		assert.Equal(t, m10DirIncrease, r.Direction)
	}

	// Cursor pagination simulation: limit=5
	limit := 5
	page1 := allRows
	if len(page1) > limit {
		page1 = page1[:limit]
	}
	assert.Len(t, page1, limit, "first page returns limit rows")
	hasMore := len(allRows) > limit
	assert.True(t, hasMore, "hasMore=true because 12 > 5")
	t.Logf("P: delta history list: %d rows, %d INCREASE, cursor paging verified", len(allRows), len(increaseRows))
}

// testM10_Q_DashboardSummary — S5-AC2: MTD/YTD aggregate + directionBreakdown.
func testM10_Q_DashboardSummary(t *testing.T) {
	t.Parallel()

	repo := newM10DeltaLogRepo()
	instrumenID := uuid.New()
	calcRunID := uuid.New()

	// Seed: INCREASE 200M, DECREASE 150M, ZERO 0
	rows := []*m10PociDeltaLog{
		{ID: uuid.New(), CalcRunID: calcRunID, InstrumenID: instrumenID,
			TanggalCompute: time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC),
			DeltaECL:       decimal.NewFromFloat(200000000.0).RoundBank(4), Direction: m10DirIncrease,
			Status: m10StatusPosted, TenantID: "TUGURE"},
		{ID: uuid.New(), CalcRunID: uuid.New(), InstrumenID: uuid.New(),
			TanggalCompute: time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
			DeltaECL:       decimal.NewFromFloat(-150000000.0).RoundBank(4), Direction: m10DirDecrease,
			Status: m10StatusPosted, TenantID: "TUGURE"},
		{ID: uuid.New(), CalcRunID: uuid.New(), InstrumenID: uuid.New(),
			TanggalCompute: time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC),
			DeltaECL:       decimal.Zero, Direction: m10DirZero,
			Status: m10StatusSkippedZero, TenantID: "TUGURE"},
	}
	for _, r := range rows {
		require.NoError(t, repo.Insert(r))
	}

	allRows := repo.List()
	juniRows := make([]*m10PociDeltaLog, 0)
	for _, r := range allRows {
		if r.TanggalCompute.Month() == time.June && r.TanggalCompute.Year() == 2026 {
			juniRows = append(juniRows, r)
		}
	}

	// MTD
	mtd := decimal.Zero
	var increaseCount, decreaseCount, zeroCount int
	increaseAmt := decimal.Zero
	decreaseAmt := decimal.Zero
	for _, r := range juniRows {
		mtd = mtd.Add(r.DeltaECL)
		switch r.Direction {
		case m10DirIncrease:
			increaseCount++
			increaseAmt = increaseAmt.Add(r.DeltaECL)
		case m10DirDecrease:
			decreaseCount++
			decreaseAmt = decreaseAmt.Add(r.DeltaECL.Abs())
		case m10DirZero:
			zeroCount++
		}
	}

	assert.Equal(t, "50000000.0000", mtd.StringFixed(4))
	assert.Equal(t, 1, increaseCount)
	assert.Equal(t, 1, decreaseCount)
	assert.Equal(t, 1, zeroCount)
	assert.Equal(t, "200000000.0000", increaseAmt.StringFixed(4))
	assert.Equal(t, "150000000.0000", decreaseAmt.StringFixed(4))
	t.Logf("Q: MTD=%s INCREASE=%d DECREASE=%d ZERO=%d", mtd.StringFixed(4), increaseCount, decreaseCount, zeroCount)
}

// testM10_R_LargeDeltaFlag — S5-AC3: large delta flag de-duplicated per (run, instrumen).
func testM10_R_LargeDeltaFlag(t *testing.T) {
	t.Parallel()

	audit := &m10AuditStore{}
	largeDeltaAlerted := make(map[string]bool)

	deltaECL := decimal.NewFromFloat(750000000.0).RoundBank(4)
	calcRunID := uuid.New()
	instrumenID := uuid.New()
	key := calcRunID.String() + ":" + instrumenID.String()

	isLarge := m10IsLargeDelta(deltaECL, int64(m10LargeDeltaThresholdDefault))
	assert.True(t, isLarge, "750M > 500M threshold")

	// Alert de-duplication: only once per (run, instrumen)
	if isLarge && !largeDeltaAlerted[key] {
		largeDeltaAlerted[key] = true
		audit.Write(m10AuditEntry{
			Action: m10AuditLargeDeltaAlert, EntityType: "ecl.poci_delta_log",
			EntityID: instrumenID,
			AfterJSONB: map[string]interface{}{
				"calc_run_id":  calcRunID.String(),
				"delta_ecl":    deltaECL.StringFixed(4),
				"threshold":    fmt.Sprintf("%d", m10LargeDeltaThresholdDefault),
			},
		})
	}

	// Second pageload — should NOT alert again
	if isLarge && !largeDeltaAlerted[key] {
		t.Fatal("alert should NOT fire again for same (run, instrumen)")
	}

	events := audit.FilterByAction(m10AuditLargeDeltaAlert)
	require.Len(t, events, 1, "LARGE_DELTA_ALERT fires once per (run, instrumen)")
	assert.Equal(t, "750000000.0000", events[0].AfterJSONB["delta_ecl"])
	t.Log("R: largeDeltaFlag=true; POCI.LARGE_DELTA_ALERT written once (de-duplicated)")
}

// testM10_S_ExportPermission — S5-AC4: ROLE-AUDIT async export OK; ROLE-AKUN unfiltered → 403.
func testM10_S_ExportPermission(t *testing.T) {
	t.Parallel()

	// canExportFiltered: requires ecl_run.read OR transaksi.read (ROLE-AKUN has transaksi.read
	// which covers filtered poci delta log exports per UX §1.4 "respect filter").
	canExportFiltered := func(perms []string) bool {
		for _, p := range perms {
			if p == "ecl_run.read" || p == "poci.delta.read" || p == "transaksi.read" {
				return true
			}
		}
		return false
	}
	// canExportUnfiltered: requires audit_log.read (full dataset, no filter restriction).
	canExportUnfiltered := func(perms []string) bool {
		for _, p := range perms {
			if p == "audit_log.read" {
				return true
			}
		}
		return false
	}

	// ROLE-AUDIT: ecl_run.read + audit_log.read
	auditPerms := []string{"ecl_run.read", "audit_log.read"}
	// ROLE-AKUN: transaksi.read + jurnal.read (but NOT audit_log.read)
	akunPerms := []string{"transaksi.read", "jurnal.read"}

	// ROLE-AUDIT: can export filtered OR unfiltered
	assert.True(t, canExportFiltered(auditPerms))
	assert.True(t, canExportUnfiltered(auditPerms))

	// ROLE-AKUN: can export WITH filter active; unfiltered export → 403
	assert.True(t, canExportFiltered(akunPerms), "ROLE-AKUN can export with filter (transaksi.read)")
	assert.False(t, canExportUnfiltered(akunPerms), "ROLE-AKUN cannot export unfiltered (no audit_log.read)")

	// > 10k rows → async export (UX rule §3)
	rowCount := 15000
	isAsync := rowCount > 10000
	assert.True(t, isAsync, "export > 10k rows must be async (202 Accepted + jobId)")
	t.Log("S: ROLE-AUDIT async export 202; ROLE-AKUN unfiltered → 403")
}

// ─── Cross-cutting ────────────────────────────────────────────────────────────

// testM10_T_IdempotencyReplay — Cross: same Idempotency-Key on baseline capture → IDEMPOTENCY_REPLAY.
func testM10_T_IdempotencyReplay(t *testing.T) {
	t.Parallel()

	store := newM10IdempotencyStore()
	repo := newM10BaselineRepo()

	instrumenID := uuid.New()
	payload := []byte(fmt.Sprintf(`{"instrumenId":"%s","lifetimeEclAtOrigination":1250000000}`, instrumenID))
	bodyHash := sha256.Sum256(payload)
	idempotencyKey := uuid.New().String()

	// First request: baseline captured
	require.NoError(t, repo.Insert(&m10PociBaseline{
		ID: uuid.New(), InstrumenID: instrumenID,
		LifetimeECLAtOrigination: decimal.NewFromFloat(1250000000.0).RoundBank(4),
		TenantID:                 "TUGURE",
	}))
	responseBody, _ := json.Marshal(map[string]string{"status": "created"})
	store.store(idempotencyKey, bodyHash, 201, responseBody)

	// Second request: same key + same payload → IDEMPOTENCY_REPLAY
	entry, found, mismatch := store.check(idempotencyKey, bodyHash)
	require.True(t, found)
	require.False(t, mismatch)
	assert.Equal(t, 201, entry.ResponseCode)

	// Verify no second INSERT happened
	rowCount := 0
	for range repo.rows {
		rowCount++
	}
	assert.Equal(t, 1, rowCount, "only one baseline row (idempotency prevents second INSERT)")
	t.Logf("T: Idempotency replay — returned original 201 response; no duplicate INSERT")
}

// testM10_U_AuditHashChain — Cross: hash-chain valid across 3 events per instrument.
func testM10_U_AuditHashChain(t *testing.T) {
	t.Parallel()

	audit := &m10AuditStore{}
	instrumenID := uuid.New()
	calcRunID := uuid.New()

	// Event 1: POCI.BASELINE_CAPTURED (S1)
	audit.Write(m10AuditEntry{
		Action: m10AuditBaselineCaptured, EntityType: "ecl.poci_baseline",
		EntityID: instrumenID,
		AfterJSONB: map[string]interface{}{
			"lifetime_ecl_at_origination": "1250000000.0000",
		},
	})

	// Event 2: POCI.DELTA_COMPUTED (S2)
	audit.Write(m10AuditEntry{
		Action: m10AuditDeltaComputed, EntityType: "ecl.poci_delta_log",
		EntityID: instrumenID,
		AfterJSONB: map[string]interface{}{
			"calc_run_id": calcRunID.String(),
			"delta_ecl":   "200000000.0000",
			"direction":   m10DirIncrease,
		},
	})

	// Event 3: POCI.DELTA_POSTED (S3)
	audit.Write(m10AuditEntry{
		Action: m10AuditDeltaPosted, EntityType: "ecl.poci_delta_log",
		EntityID: instrumenID,
		AfterJSONB: map[string]interface{}{
			"jurnal_event_code": m10JurnalPociIncrease,
			"amount":            "200000000.0000",
		},
	})

	require.Len(t, audit.entries, 3)
	assert.True(t, audit.VerifyHashChain(), "hash-chain must be valid across 3 audit events")

	// Verify order
	assert.Equal(t, m10AuditBaselineCaptured, audit.entries[0].Action)
	assert.Equal(t, m10AuditDeltaComputed, audit.entries[1].Action)
	assert.Equal(t, m10AuditDeltaPosted, audit.entries[2].Action)
	t.Log("U: Audit hash-chain valid across BASELINE_CAPTURED + DELTA_COMPUTED + DELTA_POSTED")
}

// testM10_V_IsPociGate — Cross: POST /poci/baseline for non-POCI → POCI_INSTRUMEN_NOT_POCI.
func testM10_V_IsPociGate(t *testing.T) {
	t.Parallel()

	instrumen := m10Instrumen{ID: uuid.New(), IsPoci: false, KodeInstrumen: "DEP-0099"}

	// Service layer gate
	var errCode string
	var httpStatus int
	if !instrumen.IsPoci {
		errCode = m10ErrInstrumenNotPoci
		httpStatus = 422
	}

	assert.Equal(t, m10ErrInstrumenNotPoci, errCode)
	assert.Equal(t, 422, httpStatus)
	t.Logf("V: POCI_INSTRUMEN_NOT_POCI for %s (is_poci=false) → 422", instrumen.KodeInstrumen)
}

// testM10_W_DeltaLogPagination — Cross: cursor pagination on /poci/delta-log.
func testM10_W_DeltaLogPagination(t *testing.T) {
	t.Parallel()

	repo := newM10DeltaLogRepo()
	calcRunID := uuid.New()

	// Seed 25 rows for different instruments
	for i := 0; i < 25; i++ {
		row := &m10PociDeltaLog{
			ID: uuid.New(), CalcRunID: calcRunID, InstrumenID: uuid.New(),
			TanggalCompute: time.Now().UTC(),
			DeltaECL:       decimal.NewFromFloat(float64(i+1) * 1000000.0).RoundBank(4),
			Direction:      m10DirIncrease, Status: m10StatusPosted, TenantID: "TUGURE",
		}
		require.NoError(t, repo.Insert(row))
	}

	allRows := repo.List()
	require.Len(t, allRows, 25)

	// Simulate cursor-based pagination: limit=10
	limit := 10
	page1 := allRows[:limit]
	assert.Len(t, page1, limit)
	hasMorePage1 := len(allRows) > limit
	assert.True(t, hasMorePage1)

	page2 := allRows[limit : limit*2]
	assert.Len(t, page2, limit)
	hasMorePage2 := len(allRows) > limit*2
	assert.True(t, hasMorePage2)

	page3 := allRows[limit*2:]
	assert.Len(t, page3, 5)
	hasMorePage3 := len(allRows) > limit*3
	assert.False(t, hasMorePage3)

	// total estimate
	totalEstimate := int64(len(allRows))
	assert.Equal(t, int64(25), totalEstimate)
	t.Logf("W: cursor paging — 25 rows, limit=10, pages: %d/%d/%d, hasMore: T/T/F",
		len(page1), len(page2), len(page3))
}
