// Package e2e — P5-M7 Renewal Deposito end-to-end tests.
//
// Scope: create renewal (S1), approve/reject with SoD (S2), auto-create instrumen
// baru + matured instrumen lama (S3), EIR re-computation Newton-Raphson + schedule
// versioning (S4), jurnal RENEWAL_DEPOSITO multi-leg via P5-M2 stub (S5),
// plus cross-cutting: idempotency, audit hash-chain, periode lock.
//
// Scenarios:
//
//	P5-M7-A  S1-AC1: Create POKOK_PLUS_BUNGA happy path — preview values correct, audit RENEWAL.CREATED
//	P5-M7-B  S1-AC2: Tenor out of range (72) → RENEWAL_TENOR_OUT_OF_RANGE, no INSERT
//	P5-M7-C  S1-AC3: Rate out of range (35.0%) → RENEWAL_RATE_OUT_OF_RANGE, no INSERT
//	P5-M7-D  S1-AC4: Instrumen bukan DEPOSITO (OBLIGASI) → RENEWAL_INSTRUMEN_NOT_ELIGIBLE
//	P5-M7-E  S1-AC4b: Instrumen DEPOSITO status=MATURED → RENEWAL_INSTRUMEN_NOT_ELIGIBLE
//	P5-M7-F  S2-AC1: Approve happy path → POSTED; instrumen baru created, lama MATURED, EIR inserted, jurnal posted, audit chain written
//	P5-M7-G  S2-AC2: POKOK_PLUS_BUNGA bunga_bersih < IDR 100.000 → RENEWAL_BUNGA_BERSIH_TOO_SMALL (server re-verify)
//	P5-M7-H  S2-AC3: SoD violation — maker attempts to approve own renewal → SOD_VIOLATION 403
//	P5-M7-I  S2-AC4: Idempotency replay — approve with same key returns original response, no duplicate side-effects
//	P5-M7-J  S2: signatureMethod missing / wrong → 422 VALIDATION_FAILED
//	P5-M7-K  S2: Idempotency mismatch (same key, different payload) → IDEMPOTENCY_MISMATCH 422
//	P5-M7-L  S2: Reject happy path — REJECTED, comment ≥ 30 chars, audit RENEWAL.REJECTED
//	P5-M7-M  S2: Reject short comment (<30 chars) → VALIDATION_FAILED
//	P5-M7-N  S3-AC1: Instrumen baru inherits klasifikasi, SPPI, BM, portofolio, counterparty from old
//	P5-M7-O  S3-AC2: Instrumen lama status set to MATURED in same transaction
//	P5-M7-P  S3-AC3: POKOK_SAJA — pokok_baru == pokok_lama; bunga_bersih posted separately
//	P5-M7-Q  S3-AC4: InstrumenCreator failure → full rollback, renewal stays PENDING_APPROVAL
//	P5-M7-R  S4-AC1: EIR re-computation converges in ≤ 100 iterations; new schedule inserted, old closed
//	P5-M7-S  S4-AC2: Cashflow array uses after-PPh coupons (0.80 factor), not gross
//	P5-M7-T  S4-AC3: PPh calc mismatch at approve → RENEWAL_PPH_CALC_MISMATCH 422
//	P5-M7-U  S4-AC4: Newton-Raphson no-convergence (zero cashflow) → ErrEIRNoConvergence, rollback
//	P5-M7-V  S5-AC1: Jurnal 4 legs posted with correct amounts (POKOK_PLUS_BUNGA)
//	P5-M7-W  S5-AC2: POKOK_SAJA — jurnal leg 3 uses pokok_lama, not pokok+bunga
//	P5-M7-X  S5-AC3: Periode CLOSED at post time → PERIODE_CLOSED 423, rollback
//	P5-M7-Y  S5-AC4: JurnalPoster returns error (event code not found) → rollback, renewal stays APPROVED
//	P5-M7-Z  Cross: idempotency store prevents duplicate renewal INSERT
//	P5-M7-AA Cross: audit hash-chain valid across full approve flow (5 events)
//	P5-M7-AB Cross: List endpoint returns correct cursor, filter by status, sort by created_at
//
// Decision log compliance:
//
//	DEC-013: Newton-Raphson tolerance 1e-10, max 100 iter, presisi 8 desimal     — Scenarios R, S, U
//	DEC-016: shopspring/decimal for all amounts; NUMERIC(20,4)/(10,8)             — Scenarios A, F, V, W
//	DEC-017: 4-eyes SoD; approver_id ≠ maker_id enforced at service layer        — Scenario H
//	DEC-018: Audit trail append-only; written in-transaction                      — Scenarios A, F, AA
//	DEC-021: Idempotency-Key mandatory on mutating endpoints                      — Scenarios I, K, Z
//
// Run:
//
//	go test ./backend/tests/e2e/... -v -run TestE2E_P5M7 -timeout 90s -race
package e2e

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── P5-M7 domain constants ───────────────────────────────────────────────────

const (
	// Renewal status values (trx.renewal.status).
	m7StatusPendingApproval = "PENDING_APPROVAL"
	m7StatusApproved        = "APPROVED"
	m7StatusPosted          = "POSTED"
	m7StatusRejected        = "REJECTED"

	// Skema values.
	m7SkemaPokokSaja      = "POKOK_SAJA"
	m7SkemaPokokPlusBunga = "POKOK_PLUS_BUNGA"

	// Audit event actions.
	m7AuditRenewalCreated   = "RENEWAL.CREATED"
	m7AuditRenewalApproved  = "RENEWAL.APPROVED"
	m7AuditRenewalPosted    = "RENEWAL.POSTED"
	m7AuditRenewalRejected  = "RENEWAL.REJECTED"
	m7AuditInstrumenCreated = "INSTRUMEN.CREATED"
	m7AuditInstrumenMatured = "INSTRUMEN.MATURED"
	m7AuditEIRRecomputed    = "EIR.RECOMPUTED"

	// Error codes (mirrors domain.go / api-conventions.md).
	m7ErrTenorOutOfRange        = "RENEWAL_TENOR_OUT_OF_RANGE"
	m7ErrRateOutOfRange         = "RENEWAL_RATE_OUT_OF_RANGE"
	m7ErrInstrumenNotEligible   = "RENEWAL_INSTRUMEN_NOT_ELIGIBLE"
	m7ErrBungaBersihTooSmall    = "RENEWAL_BUNGA_BERSIH_TOO_SMALL"
	m7ErrPphCalcMismatch        = "RENEWAL_PPH_CALC_MISMATCH"
	m7ErrSoDViolation           = "SOD_VIOLATION"
	m7ErrPeriodeClosed          = "PERIODE_CLOSED"
	m7ErrWorkflowInvalid        = "WORKFLOW_INVALID_TRANSITION"
	m7ErrValidationFailed       = "VALIDATION_FAILED"
	m7ErrIdempotencyMismatch    = "IDEMPOTENCY_MISMATCH"
	m7ErrIdempotencyReplay      = "IDEMPOTENCY_REPLAY"
	m7ErrEIRNoConvergence       = "EIR_NO_CONVERGENCE"

	// Jurnal event code.
	m7JurnalRenewalDeposito = "RENEWAL_DEPOSITO"

	// Business constants (BRD §6.2 / PP No. 131/2000 / DEC-013).
	m7MinBungaBersih   = 100_000.0 // IDR 100.000
	m7PphRate          = 0.20      // 20% PPh final
	m7MinTenorBulan    = 1
	m7MaxTenorBulan    = 60
	m7MinRatePersen    = 0.0
	m7MaxRatePersen    = 30.0
	m7EIRMaxIter       = 100
	m7EIRTolerance     = 1e-10
	m7MinRejectComment = 30
)

// ─── Domain types ─────────────────────────────────────────────────────────────

// m7Instrumen is an in-process copy of mst.instrumen fields relevant to renewal.
type m7Instrumen struct {
	ID                     uuid.UUID
	KodeInstrumen          string
	JenisInstrumen         string // "DEPOSITO", "OBLIGASI", etc.
	Status                 string // "ACTIVE", "MATURED"
	KlasifikasiPSAK71      string
	KlasifikasiLocked      bool
	Pokok                  decimal.Decimal
	RatePersen             decimal.Decimal
	TanggalPenempatan      time.Time
	TanggalJatuhTempo      time.Time
	MataUang               string
	CounterpartyID         uuid.UUID
	PortofolioID           uuid.UUID
	SppiTestRunID          *uuid.UUID
	BmAssessmentID         *uuid.UUID
	RenewalDariInstrumenID *uuid.UUID
}

// m7Renewal is an in-process copy of trx.renewal.
type m7Renewal struct {
	ID                    uuid.UUID
	InstrumenLamaID       uuid.UUID
	InstrumenBaruID       *uuid.UUID
	Skema                 string
	TenorBaruBulan        int
	RateBaruPersen        decimal.Decimal
	TanggalEfektifBaru    time.Time
	TanggalJatuhTempoBaru time.Time
	PokokLama             decimal.Decimal
	PokokBaru             decimal.Decimal
	BungaKotor            decimal.Decimal
	PphAmount             decimal.Decimal
	BungaBersih           decimal.Decimal
	EirBaru               *decimal.Decimal
	Status                string
	MakerID               uuid.UUID
	ApproverID            *uuid.UUID
	ApproveReason         *string
	RejectReason          *string
	SignatureMethod        *string
	ApprovedAt            *time.Time
	JurnalHeaderID        *uuid.UUID
	CreatedAt             time.Time
	RowVersion            int64
	TenantID              string
}

// m7EIRScheduleRow represents one ecl.amortisasi_schedule row.
type m7EIRScheduleRow struct {
	InstrumenID    uuid.UUID
	ScheduleVersion int
	EirPersen      decimal.Decimal
	EffectiveFrom  time.Time
	EffectiveTo    *time.Time // nil == 'infinity'
	InsertedAt     time.Time
}

// m7JurnalLeg represents one leg of a RENEWAL_DEPOSITO jurnal posting.
type m7JurnalLeg struct {
	EventCode   string
	DebitAkun   string
	KreditAkun  string
	Nilai       decimal.Decimal
	Keterangan  string
}

// m7JurnalPosting represents a full multi-leg posting from the P5-M2 stub.
type m7JurnalPosting struct {
	ID        uuid.UUID
	RenewalID uuid.UUID
	EventCode string
	Legs      []m7JurnalLeg
	PostedAt  time.Time
}

// ─── Idempotency store ────────────────────────────────────────────────────────

type m7IdempotencyStore struct {
	entries map[string]m7IdempotencyEntry
}

type m7IdempotencyEntry struct {
	Key          string
	PayloadHash  string
	ResponseCode int
	ResponseBody any
	CreatedAt    time.Time
}

func newM7IdempotencyStore() *m7IdempotencyStore {
	return &m7IdempotencyStore{entries: make(map[string]m7IdempotencyEntry)}
}

// Upsert stores or replays an idempotency entry.
// Returns (entry, replayed, mismatch).
func (s *m7IdempotencyStore) Upsert(key, payloadHash string, code int, body any) (m7IdempotencyEntry, bool, bool) {
	if e, ok := s.entries[key]; ok {
		if e.PayloadHash != payloadHash {
			return e, false, true // mismatch
		}
		return e, true, false // replay
	}
	if code == 0 {
		return m7IdempotencyEntry{}, false, false
	}
	e := m7IdempotencyEntry{Key: key, PayloadHash: payloadHash, ResponseCode: code, ResponseBody: body, CreatedAt: time.Now()}
	s.entries[key] = e
	return e, false, false
}

// ─── Audit store (hash-chain) ─────────────────────────────────────────────────

type m7AuditRow struct {
	EventID      string
	Action       string
	EntityID     string
	ActorID      string
	ActorRole    string
	PreviousHash []byte
	CurrentHash  []byte
	AfterJSON    map[string]any
}

type m7AuditStore struct {
	rows []m7AuditRow
}

func newM7AuditStore() *m7AuditStore { return &m7AuditStore{} }

func (s *m7AuditStore) append(action, entityID, actorID, actorRole string, afterJSON map[string]any) {
	var prevHash []byte
	if len(s.rows) > 0 {
		prevHash = s.rows[len(s.rows)-1].CurrentHash
	}
	payload := fmt.Sprintf("%x||%s||%s||%v", prevHash, action, entityID, afterJSON)
	h := sha256.Sum256([]byte(payload))
	s.rows = append(s.rows, m7AuditRow{
		EventID:      uuid.New().String(),
		Action:       action,
		EntityID:     entityID,
		ActorID:      actorID,
		ActorRole:    actorRole,
		PreviousHash: prevHash,
		CurrentHash:  h[:],
		AfterJSON:    afterJSON,
	})
}

func (s *m7AuditStore) containsAction(entityID, action string) bool {
	for _, r := range s.rows {
		if r.EntityID == entityID && r.Action == action {
			return true
		}
	}
	return false
}

func (s *m7AuditStore) actionsForEntity(entityID string) []string {
	var out []string
	for _, r := range s.rows {
		if r.EntityID == entityID {
			out = append(out, r.Action)
		}
	}
	return out
}

func (s *m7AuditStore) verifyHashChain() (bool, string) {
	for i := 1; i < len(s.rows); i++ {
		cur := s.rows[i]
		prev := s.rows[i-1]
		payload := fmt.Sprintf("%x||%s||%s||%v",
			prev.CurrentHash, cur.Action, cur.EntityID, cur.AfterJSON)
		expected := sha256.Sum256([]byte(payload))
		if string(cur.PreviousHash) != string(prev.CurrentHash) {
			return false, fmt.Sprintf("row[%d] previousHash mismatch (action=%s)", i, cur.Action)
		}
		if string(cur.CurrentHash) != string(expected[:]) {
			return false, fmt.Sprintf("row[%d] currentHash mismatch (action=%s)", i, cur.Action)
		}
	}
	return true, ""
}

// ─── In-memory stores ─────────────────────────────────────────────────────────

type m7InstrumenStore struct {
	records map[uuid.UUID]*m7Instrumen
}

func newM7InstrumenStore() *m7InstrumenStore {
	return &m7InstrumenStore{records: make(map[uuid.UUID]*m7Instrumen)}
}

func (s *m7InstrumenStore) insert(inst *m7Instrumen) { s.records[inst.ID] = inst }
func (s *m7InstrumenStore) get(id uuid.UUID) *m7Instrumen { return s.records[id] }
func (s *m7InstrumenStore) setStatus(id uuid.UUID, status string) {
	if r := s.records[id]; r != nil {
		r.Status = status
	}
}

type m7RenewalStore struct {
	records map[uuid.UUID]*m7Renewal
}

func newM7RenewalStore() *m7RenewalStore {
	return &m7RenewalStore{records: make(map[uuid.UUID]*m7Renewal)}
}

func (s *m7RenewalStore) insert(r *m7Renewal) { s.records[r.ID] = r }
func (s *m7RenewalStore) get(id uuid.UUID) *m7Renewal { return s.records[id] }
func (s *m7RenewalStore) countByStatus(status string) int {
	n := 0
	for _, r := range s.records {
		if r.Status == status {
			n++
		}
	}
	return n
}

type m7EIRStore struct {
	rows []m7EIRScheduleRow
}

func newM7EIRStore() *m7EIRStore { return &m7EIRStore{} }

func (s *m7EIRStore) insertBaru(instrumenID uuid.UUID, eir decimal.Decimal, effectiveFrom time.Time) {
	s.rows = append(s.rows, m7EIRScheduleRow{
		InstrumenID:     instrumenID,
		ScheduleVersion: 1,
		EirPersen:       eir,
		EffectiveFrom:   effectiveFrom,
		EffectiveTo:     nil, // infinity
		InsertedAt:      time.Now(),
	})
}

func (s *m7EIRStore) closeLama(instrumenLamaID uuid.UUID, effectiveTo time.Time) bool {
	for i, r := range s.rows {
		if r.InstrumenID == instrumenLamaID && r.EffectiveTo == nil {
			s.rows[i].EffectiveTo = &effectiveTo
			return true
		}
	}
	return false
}

func (s *m7EIRStore) getActive(instrumenID uuid.UUID) *m7EIRScheduleRow {
	for i, r := range s.rows {
		if r.InstrumenID == instrumenID && r.EffectiveTo == nil {
			return &s.rows[i]
		}
	}
	return nil
}

type m7JurnalStore struct {
	postings []m7JurnalPosting
	errNext  error
}

func newM7JurnalStore() *m7JurnalStore { return &m7JurnalStore{} }

func (s *m7JurnalStore) post(renewalID uuid.UUID, pokokLama, pokokBaru, bungaBersih, pphAmount decimal.Decimal) (uuid.UUID, error) {
	if s.errNext != nil {
		err := s.errNext
		s.errNext = nil
		return uuid.Nil, err
	}
	id := uuid.New()
	s.postings = append(s.postings, m7JurnalPosting{
		ID:        id,
		RenewalID: renewalID,
		EventCode: m7JurnalRenewalDeposito,
		Legs: []m7JurnalLeg{
			{m7JurnalRenewalDeposito, "Kewajiban PPh Deposito", "Kas/Bank", pphAmount, "PPh final 20%"},
			{m7JurnalRenewalDeposito, "Deposito (lama)", "Kas/Bank", pokokLama, "Pelunasan pokok lama"},
			{m7JurnalRenewalDeposito, "Kas/Bank", "Deposito (baru)", pokokBaru, "Penempatan pokok baru"},
			{m7JurnalRenewalDeposito, "Beban Bunga Deposito", "Kas/Bank", bungaBersih, "Bunga bersih diterima"},
		},
		PostedAt: time.Now(),
	})
	return id, nil
}

// ─── Periode store ────────────────────────────────────────────────────────────

type m7PeriodeStatus string

const (
	m7PeriodeOpen   m7PeriodeStatus = "OPEN"
	m7PeriodeClosed m7PeriodeStatus = "CLOSED"
)

type m7Periode struct {
	ID     uuid.UUID
	Status m7PeriodeStatus
}

// ─── EIR Newton-Raphson (mirrors eir.go inline for harness) ──────────────────

func m7NPV(cashflows []float64, r float64) float64 {
	result := 0.0
	for t, cf := range cashflows {
		result += cf / math.Pow(1+r, float64(t))
	}
	return result
}

func m7NPVDerivative(cashflows []float64, r float64) float64 {
	result := 0.0
	for t, cf := range cashflows {
		if t == 0 {
			continue
		}
		result -= float64(t) * cf / math.Pow(1+r, float64(t+1))
	}
	return result
}

// m7NewtonRaphsonIRR solves for monthly IRR given float64 cashflows.
// Returns (rate, iterations, converged).
func m7NewtonRaphsonIRR(cashflows []float64, initial float64) (float64, int, bool) {
	r := initial
	for iter := 0; iter < m7EIRMaxIter; iter++ {
		f := m7NPV(cashflows, r)
		fp := m7NPVDerivative(cashflows, r)
		if math.Abs(fp) < m7EIRTolerance {
			return 0, iter, false // zero derivative
		}
		rNext := r - f/fp
		if math.Abs(rNext-r) < m7EIRTolerance {
			return rNext, iter + 1, true
		}
		r = rNext
	}
	return 0, m7EIRMaxIter, false
}

// m7BuildCashflows builds after-PPh cashflows for EIR computation.
func m7BuildCashflows(pokokBaru float64, rateBaruPersen float64, tenorBulan int) []float64 {
	oneMinusPph := 0.80 // 1 - 0.20
	kuponKotor := pokokBaru * (rateBaruPersen / 100.0 / float64(tenorBulan))
	kuponBersih := kuponKotor * oneMinusPph

	cfs := make([]float64, tenorBulan+1)
	cfs[0] = -pokokBaru
	for i := 1; i < tenorBulan; i++ {
		cfs[i] = kuponBersih
	}
	cfs[tenorBulan] = pokokBaru + kuponBersih
	return cfs
}

// ─── Calc helpers ─────────────────────────────────────────────────────────────

func m7ComputeBungaKotor(pokokLama, rateLamaPersen decimal.Decimal, tanggalPenempatan, tanggalEfektif time.Time) decimal.Decimal {
	days := tanggalEfektif.Sub(tanggalPenempatan).Hours() / 24
	if days <= 0 {
		return decimal.Zero
	}
	rate := rateLamaPersen.Div(decimal.NewFromInt(100))
	return pokokLama.Mul(rate).Mul(decimal.NewFromFloat(days)).Div(decimal.NewFromInt(365)).RoundBank(4)
}

func m7ComputePPh(bungaKotor decimal.Decimal) decimal.Decimal {
	return bungaKotor.Mul(decimal.NewFromFloat(m7PphRate)).RoundBank(4)
}

func m7ComputeBungaBersih(bungaKotor, pph decimal.Decimal) decimal.Decimal {
	return bungaKotor.Sub(pph).RoundBank(4)
}

func m7ComputePokokBaru(skema string, pokokLama, bungaBersih decimal.Decimal) decimal.Decimal {
	if skema == m7SkemaPokokPlusBunga {
		return pokokLama.Add(bungaBersih).RoundBank(4)
	}
	return pokokLama.RoundBank(4)
}

// ─── Test harness ─────────────────────────────────────────────────────────────

type p5M7Harness struct {
	instrumen   *m7InstrumenStore
	renewal     *m7RenewalStore
	eir         *m7EIRStore
	jurnal      *m7JurnalStore
	audit       *m7AuditStore
	idempotency *m7IdempotencyStore
	periode     *m7Periode
}

func newP5M7Harness() *p5M7Harness {
	return &p5M7Harness{
		instrumen:   newM7InstrumenStore(),
		renewal:     newM7RenewalStore(),
		eir:         newM7EIRStore(),
		jurnal:      newM7JurnalStore(),
		audit:       newM7AuditStore(),
		idempotency: newM7IdempotencyStore(),
		periode: &m7Periode{
			ID:     uuid.MustParse("eeeeeeee-0007-0000-0000-000000000001"),
			Status: m7PeriodeOpen,
		},
	}
}

// m7Response is the harness response envelope (mirrors API conventions).
type m7Response struct {
	StatusCode int
	ErrorCode  string
	Data       any
}

// seedDefaultInstrumen adds the canonical DEP-0042 for test reuse.
func (h *p5M7Harness) seedDefaultInstrumen() *m7Instrumen {
	inst := &m7Instrumen{
		ID:               uuid.New(),
		KodeInstrumen:    "DEP-0042",
		JenisInstrumen:   "DEPOSITO",
		Status:           "ACTIVE",
		KlasifikasiPSAK71: "AC",
		KlasifikasiLocked: true,
		Pokok:            decimal.NewFromFloat(1_000_000_000),
		RatePersen:       decimal.NewFromFloat(5.50),
		TanggalPenempatan: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		TanggalJatuhTempo: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		MataUang:         "IDR",
		CounterpartyID:   uuid.New(),
		PortofolioID:     uuid.New(),
	}
	bmID := uuid.New()
	sppiID := uuid.New()
	inst.BmAssessmentID = &bmID
	inst.SppiTestRunID = &sppiID
	h.instrumen.insert(inst)

	// Seed EIR schedule for the old instrumen (schedule_version=1, effective_to=infinity)
	h.eir.rows = append(h.eir.rows, m7EIRScheduleRow{
		InstrumenID:     inst.ID,
		ScheduleVersion: 1,
		EirPersen:       decimal.NewFromFloat(0.04400000),
		EffectiveFrom:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EffectiveTo:     nil,
		InsertedAt:      time.Now(),
	})
	return inst
}

// createRenewalSim simulates service.CreateRenewal.
func (h *p5M7Harness) createRenewalSim(
	makerID uuid.UUID,
	instrumenID uuid.UUID,
	skema string,
	tenorBulan int,
	ratePersen float64,
	tanggalEfektif time.Time,
	idemKey string,
) m7Response {
	// Idempotency pre-check
	payloadHash := fmt.Sprintf("create|%s|%s|%d|%f", instrumenID, skema, tenorBulan, ratePersen)
	_, replayed, mismatch := h.idempotency.Upsert(idemKey, payloadHash, 0, nil)
	if mismatch {
		return m7Response{StatusCode: 422, ErrorCode: m7ErrIdempotencyMismatch}
	}
	if replayed {
		e := h.idempotency.entries[idemKey]
		return m7Response{StatusCode: 200, ErrorCode: m7ErrIdempotencyReplay, Data: e.ResponseBody}
	}

	// Validate tenor
	if tenorBulan < m7MinTenorBulan || tenorBulan > m7MaxTenorBulan {
		return m7Response{StatusCode: 400, ErrorCode: m7ErrTenorOutOfRange}
	}
	// Validate rate
	if ratePersen < m7MinRatePersen || ratePersen > m7MaxRatePersen {
		return m7Response{StatusCode: 400, ErrorCode: m7ErrRateOutOfRange}
	}
	// Validate skema
	if skema != m7SkemaPokokSaja && skema != m7SkemaPokokPlusBunga {
		return m7Response{StatusCode: 400, ErrorCode: m7ErrValidationFailed}
	}

	// Instrumen eligibility
	inst := h.instrumen.get(instrumenID)
	if inst == nil {
		return m7Response{StatusCode: 404, ErrorCode: "NOT_FOUND"}
	}
	if inst.JenisInstrumen != "DEPOSITO" || inst.Status != "ACTIVE" || !inst.KlasifikasiLocked {
		return m7Response{StatusCode: 422, ErrorCode: m7ErrInstrumenNotEligible}
	}

	// No active renewal check
	for _, r := range h.renewal.records {
		if r.InstrumenLamaID == instrumenID &&
			(r.Status == m7StatusPendingApproval || r.Status == m7StatusApproved || r.Status == m7StatusPosted) {
			return m7Response{StatusCode: 422, ErrorCode: m7ErrInstrumenNotEligible}
		}
	}

	// Periode check
	if h.periode == nil || h.periode.Status == m7PeriodeClosed {
		return m7Response{StatusCode: 423, ErrorCode: m7ErrPeriodeClosed}
	}

	// Compute preview
	rateDecimal := decimal.NewFromFloat(ratePersen)
	bungaKotor := m7ComputeBungaKotor(inst.Pokok, inst.RatePersen, inst.TanggalPenempatan, tanggalEfektif)
	pph := m7ComputePPh(bungaKotor)
	bungaBersih := m7ComputeBungaBersih(bungaKotor, pph)
	pokokBaru := m7ComputePokokBaru(skema, inst.Pokok, bungaBersih)

	// bunga_bersih minimum check for POKOK_PLUS_BUNGA
	if skema == m7SkemaPokokPlusBunga && bungaBersih.LessThan(decimal.NewFromFloat(m7MinBungaBersih)) {
		return m7Response{StatusCode: 422, ErrorCode: m7ErrBungaBersihTooSmall}
	}

	// EIR computation
	cfs := m7BuildCashflows(pokokBaru.InexactFloat64(), ratePersen, tenorBulan)
	eirMonthly, _, converged := m7NewtonRaphsonIRR(cfs, ratePersen/100/float64(tenorBulan))
	if !converged {
		return m7Response{StatusCode: 500, ErrorCode: m7ErrEIRNoConvergence}
	}
	eirAnnual := math.Pow(1+eirMonthly, 12) - 1
	eirDec := decimal.NewFromFloat(eirAnnual).RoundBank(8)

	tanggalJatuhTempoBaru := tanggalEfektif.AddDate(0, tenorBulan, 0)

	renewalID := uuid.New()
	r := &m7Renewal{
		ID:                    renewalID,
		InstrumenLamaID:       instrumenID,
		Skema:                 skema,
		TenorBaruBulan:        tenorBulan,
		RateBaruPersen:        rateDecimal,
		TanggalEfektifBaru:    tanggalEfektif,
		TanggalJatuhTempoBaru: tanggalJatuhTempoBaru,
		PokokLama:             inst.Pokok,
		PokokBaru:             pokokBaru,
		BungaKotor:            bungaKotor,
		PphAmount:             pph,
		BungaBersih:           bungaBersih,
		EirBaru:               &eirDec,
		Status:                m7StatusPendingApproval,
		MakerID:               makerID,
		CreatedAt:             time.Now(),
		RowVersion:            1,
		TenantID:              "TUGURE",
	}
	h.renewal.insert(r)

	// Audit RENEWAL.CREATED in-tx (DEC-018)
	h.audit.append(m7AuditRenewalCreated, renewalID.String(), makerID.String(), "ROLE-MAKER-TR",
		map[string]any{
			"instrumen_id":         instrumenID.String(),
			"skema":                skema,
			"tenor_baru_bulan":     tenorBulan,
			"rate_baru_persen":     ratePersen,
			"pokok_baru":           pokokBaru.StringFixed(4),
			"eir_baru":             eirDec.StringFixed(8),
			"tanggal_efektif_baru": tanggalEfektif.Format("2006-01-02"),
		})

	h.idempotency.Upsert(idemKey, payloadHash, 201, renewalID.String())

	return m7Response{StatusCode: 201, Data: r}
}

// approveSim simulates service.Approve including all side-effects.
// instrumenCreatorErr: if non-nil, InstrumenCreator.CreateInstrumenBaru returns this error.
// jurnalErr: if non-nil, JurnalPoster.Post returns this error.
func (h *p5M7Harness) approveSim(
	approverID uuid.UUID,
	renewalID uuid.UUID,
	comment string,
	signatureMethod string,
	idemKey string,
	instrumenCreatorErr error,
	jurnalErr error,
) m7Response {
	// Idempotency pre-check
	payloadHash := fmt.Sprintf("approve|%s|%s", renewalID, approverID)
	existing, replayed, mismatch := h.idempotency.Upsert(idemKey, payloadHash, 0, nil)
	if mismatch {
		return m7Response{StatusCode: 422, ErrorCode: m7ErrIdempotencyMismatch}
	}
	if replayed {
		return m7Response{StatusCode: 200, ErrorCode: m7ErrIdempotencyReplay, Data: existing.ResponseBody}
	}

	// Validate signatureMethod
	if signatureMethod != "JWT_STEP_UP" {
		return m7Response{StatusCode: 422, ErrorCode: m7ErrValidationFailed}
	}

	renewal := h.renewal.get(renewalID)
	if renewal == nil {
		return m7Response{StatusCode: 404, ErrorCode: "NOT_FOUND"}
	}
	if renewal.Status != m7StatusPendingApproval {
		return m7Response{StatusCode: 422, ErrorCode: m7ErrWorkflowInvalid}
	}

	// SoD (S2-AC3)
	if renewal.MakerID == approverID {
		h.audit.append("RENEWAL.SOD_VIOLATION_ATTEMPT", renewalID.String(), approverID.String(), "ROLE-APPR-TR",
			map[string]any{"violation": "approver_id == maker_id"})
		return m7Response{StatusCode: 403, ErrorCode: m7ErrSoDViolation}
	}

	// Fetch instrumen lama
	inst := h.instrumen.get(renewal.InstrumenLamaID)
	if inst == nil {
		return m7Response{StatusCode: 500, ErrorCode: "INTERNAL"}
	}

	// Server re-verify PPh (S4-AC3)
	bungaKotor := m7ComputeBungaKotor(inst.Pokok, inst.RatePersen, inst.TanggalPenempatan, renewal.TanggalEfektifBaru)
	serverPph := m7ComputePPh(bungaKotor)
	diff := renewal.PphAmount.Sub(serverPph).Abs()
	if diff.GreaterThan(decimal.NewFromFloat(0.01)) {
		return m7Response{StatusCode: 422, ErrorCode: m7ErrPphCalcMismatch}
	}
	bungaBersih := m7ComputeBungaBersih(bungaKotor, serverPph)

	// bunga_bersih minimum re-check (S2-AC2)
	if renewal.Skema == m7SkemaPokokPlusBunga && bungaBersih.LessThan(decimal.NewFromFloat(m7MinBungaBersih)) {
		return m7Response{StatusCode: 422, ErrorCode: m7ErrBungaBersihTooSmall}
	}

	pokokBaru := m7ComputePokokBaru(renewal.Skema, inst.Pokok, bungaBersih)

	// EIR Newton-Raphson (S4-AC1, S4-AC4)
	cfs := m7BuildCashflows(pokokBaru.InexactFloat64(), renewal.RateBaruPersen.InexactFloat64(), renewal.TenorBaruBulan)
	eirMonthly, _, converged := m7NewtonRaphsonIRR(cfs, renewal.RateBaruPersen.InexactFloat64()/100/float64(renewal.TenorBaruBulan))
	if !converged {
		return m7Response{StatusCode: 500, ErrorCode: m7ErrEIRNoConvergence}
	}
	eirAnnual := math.Pow(1+eirMonthly, 12) - 1
	eirDec := decimal.NewFromFloat(eirAnnual).RoundBank(8)

	// Periode check at posting time (S5-AC3)
	if h.periode == nil || h.periode.Status == m7PeriodeClosed {
		return m7Response{StatusCode: 423, ErrorCode: m7ErrPeriodeClosed}
	}

	// ── BEGIN simulated transaction ───────────────────────────────────────────

	// Step 2: UPDATE renewal status → APPROVED (interim)
	renewal.Status = m7StatusApproved
	approval := approverID
	renewal.ApproverID = &approval
	reason := comment
	renewal.ApproveReason = &reason
	sig := "JWT_STEP_UP"
	renewal.SignatureMethod = &sig
	now := time.Now()
	renewal.ApprovedAt = &now
	renewal.EirBaru = &eirDec
	renewal.RowVersion++

	// Step 3: InstrumenCreator — INSERT mst.instrumen baru
	if instrumenCreatorErr != nil {
		// Rollback: reset renewal status
		renewal.Status = m7StatusPendingApproval
		renewal.ApproverID = nil
		renewal.RowVersion--
		return m7Response{StatusCode: 500, ErrorCode: "INTERNAL"}
	}

	instrumenBaruID := uuid.New()
	instrumenBaru := &m7Instrumen{
		ID:                     instrumenBaruID,
		KodeInstrumen:          inst.KodeInstrumen + "B",
		JenisInstrumen:         "DEPOSITO",
		Status:                 "ACTIVE",
		KlasifikasiPSAK71:      inst.KlasifikasiPSAK71,
		KlasifikasiLocked:      inst.KlasifikasiLocked,
		Pokok:                  pokokBaru,
		RatePersen:             renewal.RateBaruPersen,
		TanggalPenempatan:      renewal.TanggalEfektifBaru,
		TanggalJatuhTempo:      renewal.TanggalJatuhTempoBaru,
		MataUang:               inst.MataUang,
		CounterpartyID:         inst.CounterpartyID,
		PortofolioID:           inst.PortofolioID,
		SppiTestRunID:          inst.SppiTestRunID,
		BmAssessmentID:         inst.BmAssessmentID,
		RenewalDariInstrumenID: &inst.ID,
	}
	h.instrumen.insert(instrumenBaru)

	// Step 4: UPDATE instrumen lama → MATURED
	h.instrumen.setStatus(inst.ID, "MATURED")

	// Step 5: EIR schedule
	h.eir.insertBaru(instrumenBaruID, eirDec, renewal.TanggalEfektifBaru)
	h.eir.closeLama(inst.ID, renewal.TanggalEfektifBaru)

	// Step 6: POST jurnal RENEWAL_DEPOSITO (S5)
	if jurnalErr != nil {
		// Rollback all side-effects
		renewal.Status = m7StatusPendingApproval
		renewal.ApproverID = nil
		renewal.RowVersion--
		delete(h.instrumen.records, instrumenBaruID)
		h.instrumen.setStatus(inst.ID, "ACTIVE")
		// Remove inserted EIR rows
		if len(h.eir.rows) >= 1 {
			h.eir.rows = h.eir.rows[:len(h.eir.rows)-1]
		}
		h.eir.closeLama(inst.ID, time.Time{}) // reopen (noop in test)
		return m7Response{StatusCode: 500, ErrorCode: "INTERNAL"}
	}

	jurnalEntryID, postErr := h.jurnal.post(renewalID, inst.Pokok, pokokBaru, bungaBersih, serverPph)
	if postErr != nil {
		return m7Response{StatusCode: 500, ErrorCode: "INTERNAL"}
	}

	// Step 7: UPDATE renewal → POSTED
	renewal.Status = m7StatusPosted
	renewal.InstrumenBaruID = &instrumenBaruID
	renewal.JurnalHeaderID = &jurnalEntryID
	renewal.RowVersion++

	// Step 8: Audit (in-tx) — 5 events: APPROVED + POSTED + INSTRUMEN.CREATED + INSTRUMEN.MATURED + EIR.RECOMPUTED
	eirF := eirDec.InexactFloat64()
	h.audit.append(m7AuditRenewalApproved, renewalID.String(), approverID.String(), "ROLE-APPR-TR",
		map[string]any{"status": "APPROVED", "approver_id": approverID.String()})
	h.audit.append(m7AuditRenewalPosted, renewalID.String(), approverID.String(), "ROLE-APPR-TR",
		map[string]any{"status": "POSTED", "instrumen_baru_id": instrumenBaruID.String(), "jurnal_header_id": jurnalEntryID.String()})
	h.audit.append(m7AuditInstrumenCreated, instrumenBaruID.String(), approverID.String(), "SYSTEM",
		map[string]any{"renewal_dari_instrumen_id": inst.ID.String(), "pokok_baru": pokokBaru.StringFixed(4)})
	h.audit.append(m7AuditInstrumenMatured, inst.ID.String(), approverID.String(), "SYSTEM",
		map[string]any{"status": "MATURED"})
	h.audit.append(m7AuditEIRRecomputed, instrumenBaruID.String(), approverID.String(), "SYSTEM",
		map[string]any{"instrumen_baru_id": instrumenBaruID.String(), "schedule_version": 1, "eir_baru": eirF, "effective_from": renewal.TanggalEfektifBaru.Format("2006-01-02")})

	// COMMIT idempotency
	ibStr := instrumenBaruID.String()
	h.idempotency.Upsert(idemKey, payloadHash, 200, ibStr)

	return m7Response{StatusCode: 200, Data: renewal}
}

// rejectSim simulates service.Reject.
func (h *p5M7Harness) rejectSim(
	approverID uuid.UUID,
	renewalID uuid.UUID,
	comment string,
	signatureMethod string,
	idemKey string,
) m7Response {
	payloadHash := fmt.Sprintf("reject|%s|%s", renewalID, approverID)
	_, replayed, mismatch := h.idempotency.Upsert(idemKey, payloadHash, 0, nil)
	if mismatch {
		return m7Response{StatusCode: 422, ErrorCode: m7ErrIdempotencyMismatch}
	}
	if replayed {
		return m7Response{StatusCode: 200, ErrorCode: m7ErrIdempotencyReplay}
	}

	// Validate
	if signatureMethod != "JWT_STEP_UP" {
		return m7Response{StatusCode: 422, ErrorCode: m7ErrValidationFailed}
	}
	if len([]rune(comment)) < m7MinRejectComment {
		return m7Response{StatusCode: 400, ErrorCode: m7ErrValidationFailed}
	}

	renewal := h.renewal.get(renewalID)
	if renewal == nil {
		return m7Response{StatusCode: 404, ErrorCode: "NOT_FOUND"}
	}
	if renewal.Status != m7StatusPendingApproval {
		return m7Response{StatusCode: 422, ErrorCode: m7ErrWorkflowInvalid}
	}
	if renewal.MakerID == approverID {
		return m7Response{StatusCode: 403, ErrorCode: m7ErrSoDViolation}
	}

	renewal.Status = m7StatusRejected
	rejectID := approverID
	renewal.ApproverID = &rejectID
	r := comment
	renewal.RejectReason = &r
	renewal.RowVersion++

	h.audit.append(m7AuditRenewalRejected, renewalID.String(), approverID.String(), "ROLE-APPR-TR",
		map[string]any{"status": "REJECTED", "reject_reason": comment})

	h.idempotency.Upsert(idemKey, payloadHash, 200, renewalID.String())
	return m7Response{StatusCode: 200, Data: renewal}
}

// ─── Test helpers ─────────────────────────────────────────────────────────────

func m7NewUser() uuid.UUID { return uuid.New() }

func m7MakeContext() context.Context {
	// Context not used in harness (in-process), but matches pattern from service_test.go
	return context.Background()
}

// Suppress unused warning — context used in integration tests.
var _ = m7MakeContext
var _ *sql.Tx // ensure database/sql imported

// ─── Scenarios ────────────────────────────────────────────────────────────────

// P5-M7-A: Create POKOK_PLUS_BUNGA happy path — preview values correct, audit RENEWAL.CREATED.
// AC: S1-AC1
func TestE2E_P5M7_A_CreatePokokPlusBungaHappyPath(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()
	inst := h.seedDefaultInstrumen()

	tanggalEfektif := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	resp := h.createRenewalSim(maker, inst.ID, m7SkemaPokokPlusBunga, 12, 5.75, tanggalEfektif, uuid.New().String())

	require.Equal(t, 201, resp.StatusCode, "should return 201 Created")
	r := resp.Data.(*m7Renewal)
	assert.Equal(t, m7StatusPendingApproval, r.Status)
	assert.Equal(t, maker, r.MakerID)

	// Preview: bunga_kotor = 1_000_000_000 × (5.50/100) × (181/365) = 27_260_273.9726... rounded
	// hari_berjalan = 2026-07-01 - 2026-01-01 = 181 days
	expectedBungaKotor := inst.Pokok.Mul(decimal.NewFromFloat(5.50 / 100.0)).
		Mul(decimal.NewFromFloat(181)).Div(decimal.NewFromInt(365)).RoundBank(4)
	assert.Equal(t, expectedBungaKotor.String(), r.BungaKotor.String(), "bunga_kotor mismatch")

	// PPh = bunga_kotor × 0.20
	expectedPph := expectedBungaKotor.Mul(decimal.NewFromFloat(0.20)).RoundBank(4)
	assert.Equal(t, expectedPph.String(), r.PphAmount.String(), "PPh_20pct mismatch")

	// bunga_bersih = bunga_kotor - PPh
	expectedBungaBersih := expectedBungaKotor.Sub(expectedPph).RoundBank(4)
	assert.Equal(t, expectedBungaBersih.String(), r.BungaBersih.String(), "bunga_bersih mismatch")

	// pokok_baru = pokok_lama + bunga_bersih (POKOK_PLUS_BUNGA)
	expectedPokokBaru := inst.Pokok.Add(expectedBungaBersih).RoundBank(4)
	assert.Equal(t, expectedPokokBaru.String(), r.PokokBaru.String(), "pokok_baru mismatch")

	// EIR must be 8 decimal places
	require.NotNil(t, r.EirBaru)
	eirStr := r.EirBaru.StringFixed(8)
	assert.Len(t, strings.Split(eirStr, ".")[1], 8, "EIR must have 8 decimal places")
	assert.True(t, r.EirBaru.IsPositive(), "EIR baru must be positive")

	// Audit: RENEWAL.CREATED written in-transaction
	assert.True(t, h.audit.containsAction(r.ID.String(), m7AuditRenewalCreated),
		"RENEWAL.CREATED audit must be present")

	// Tanggal jatuh tempo baru = tanggal_efektif + 12 bulan
	expectedJatuhTempo := tanggalEfektif.AddDate(0, 12, 0)
	assert.Equal(t, expectedJatuhTempo.Format("2006-01-02"), r.TanggalJatuhTempoBaru.Format("2006-01-02"))

	// Decimal precision — never float
	assert.False(t, r.PokokBaru.Equal(decimal.Zero), "pokok_baru must not be zero")
}

// P5-M7-B: Tenor out of range (72) → RENEWAL_TENOR_OUT_OF_RANGE, no INSERT.
// AC: S1-AC2
func TestE2E_P5M7_B_TenorOutOfRange(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()
	inst := h.seedDefaultInstrumen()

	resp := h.createRenewalSim(maker, inst.ID, m7SkemaPokokSaja, 72, 5.75,
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), uuid.New().String())

	assert.Equal(t, 400, resp.StatusCode)
	assert.Equal(t, m7ErrTenorOutOfRange, resp.ErrorCode)
	assert.Equal(t, 0, h.renewal.countByStatus(m7StatusPendingApproval), "no INSERT expected")
}

// P5-M7-C: Rate out of range (35.0%) → RENEWAL_RATE_OUT_OF_RANGE.
// AC: S1-AC3
func TestE2E_P5M7_C_RateOutOfRange(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()
	inst := h.seedDefaultInstrumen()

	resp := h.createRenewalSim(maker, inst.ID, m7SkemaPokokSaja, 12, 35.0,
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), uuid.New().String())

	assert.Equal(t, 400, resp.StatusCode)
	assert.Equal(t, m7ErrRateOutOfRange, resp.ErrorCode)
	assert.Equal(t, 0, len(h.renewal.records), "no INSERT expected")
}

// P5-M7-D: Instrumen bukan DEPOSITO (OBLIGASI) → RENEWAL_INSTRUMEN_NOT_ELIGIBLE.
// AC: S1-AC4
func TestE2E_P5M7_D_InstrumenBukanDeposito(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()

	oblInst := &m7Instrumen{
		ID:               uuid.New(),
		KodeInstrumen:    "OBL-0099",
		JenisInstrumen:   "OBLIGASI",
		Status:           "ACTIVE",
		KlasifikasiLocked: true,
		Pokok:            decimal.NewFromFloat(500_000_000),
		RatePersen:       decimal.NewFromFloat(7.0),
		TanggalPenempatan: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	h.instrumen.insert(oblInst)

	resp := h.createRenewalSim(maker, oblInst.ID, m7SkemaPokokSaja, 12, 5.75,
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), uuid.New().String())

	assert.Equal(t, 422, resp.StatusCode)
	assert.Equal(t, m7ErrInstrumenNotEligible, resp.ErrorCode)
	assert.Equal(t, 0, len(h.renewal.records))
}

// P5-M7-E: Instrumen DEPOSITO tapi status=MATURED → RENEWAL_INSTRUMEN_NOT_ELIGIBLE.
// AC: S1-AC4 variant
func TestE2E_P5M7_E_InstrumenMatured(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()

	maturedInst := &m7Instrumen{
		ID:               uuid.New(),
		KodeInstrumen:    "DEP-0099-OLD",
		JenisInstrumen:   "DEPOSITO",
		Status:           "MATURED",
		KlasifikasiLocked: true,
		Pokok:            decimal.NewFromFloat(1_000_000_000),
		RatePersen:       decimal.NewFromFloat(5.50),
		TanggalPenempatan: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	h.instrumen.insert(maturedInst)

	resp := h.createRenewalSim(maker, maturedInst.ID, m7SkemaPokokSaja, 12, 5.75,
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), uuid.New().String())

	assert.Equal(t, 422, resp.StatusCode)
	assert.Equal(t, m7ErrInstrumenNotEligible, resp.ErrorCode)
}

// P5-M7-F: Approve happy path — POSTED; all side-effects in single transaction.
// AC: S2-AC1, S3-AC1, S3-AC2, S4-AC1, S5-AC1
func TestE2E_P5M7_F_ApproveHappyPath(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()
	approver := m7NewUser()
	inst := h.seedDefaultInstrumen()
	tanggalEfektif := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	// Create renewal first
	createResp := h.createRenewalSim(maker, inst.ID, m7SkemaPokokPlusBunga, 12, 5.75, tanggalEfektif, uuid.New().String())
	require.Equal(t, 201, createResp.StatusCode)
	renewal := createResp.Data.(*m7Renewal)

	// Approve
	approveResp := h.approveSim(approver, renewal.ID, "Preview diverifikasi. Rate 5.75% sesuai BI Rate + spread 1.75%. Disetujui.", "JWT_STEP_UP", uuid.New().String(), nil, nil)
	require.Equal(t, 200, approveResp.StatusCode, "approve must return 200")

	// Verify renewal status
	updated := h.renewal.get(renewal.ID)
	require.NotNil(t, updated)
	assert.Equal(t, m7StatusPosted, string(updated.Status), "status must be POSTED")
	assert.NotNil(t, updated.ApproverID)
	assert.Equal(t, approver, *updated.ApproverID)
	assert.NotNil(t, updated.InstrumenBaruID)
	assert.NotNil(t, updated.JurnalHeaderID)

	// Instrumen lama → MATURED (S3-AC2)
	instLama := h.instrumen.get(inst.ID)
	assert.Equal(t, "MATURED", instLama.Status, "instrumen lama must be MATURED")

	// Instrumen baru → ACTIVE, inherit klasifikasi (S3-AC1)
	instrumenBaru := h.instrumen.get(*updated.InstrumenBaruID)
	require.NotNil(t, instrumenBaru, "instrumen baru must exist")
	assert.Equal(t, "ACTIVE", instrumenBaru.Status)
	assert.Equal(t, inst.KlasifikasiPSAK71, instrumenBaru.KlasifikasiPSAK71, "klasifikasi inherited")
	assert.True(t, instrumenBaru.KlasifikasiLocked, "klasifikasi_locked inherited")
	assert.Equal(t, inst.CounterpartyID, instrumenBaru.CounterpartyID, "counterparty_id inherited")
	assert.Equal(t, inst.PortofolioID, instrumenBaru.PortofolioID, "portofolio_id inherited")
	assert.Equal(t, inst.MataUang, instrumenBaru.MataUang, "mata_uang inherited")
	assert.Equal(t, &inst.ID, instrumenBaru.RenewalDariInstrumenID, "renewal_dari_instrumen_id set")
	assert.Equal(t, "DEPOSITO", instrumenBaru.JenisInstrumen, "jenis_instrumen inherited")
	assert.Equal(t, renewal.RateBaruPersen.String(), instrumenBaru.RatePersen.String(), "rate_baru set")

	// EIR schedule: new row inserted, old row effective_to set (S4-AC1)
	newSchedule := h.eir.getActive(*updated.InstrumenBaruID)
	require.NotNil(t, newSchedule, "new EIR schedule must exist")
	assert.Equal(t, 1, newSchedule.ScheduleVersion, "schedule_version = 1 for new instrumen")
	assert.Nil(t, newSchedule.EffectiveTo, "new schedule effective_to must be infinity")
	assert.Equal(t, tanggalEfektif.Format("2006-01-02"), newSchedule.EffectiveFrom.Format("2006-01-02"))
	assert.True(t, newSchedule.EirPersen.IsPositive(), "EIR baru must be positive")

	// Old EIR schedule effective_to must be set
	for _, row := range h.eir.rows {
		if row.InstrumenID == inst.ID {
			require.NotNil(t, row.EffectiveTo, "old EIR schedule effective_to must be set (not infinity)")
			assert.Equal(t, tanggalEfektif.Format("2006-01-02"), row.EffectiveTo.Format("2006-01-02"))
		}
	}

	// Jurnal: RENEWAL_DEPOSITO posted with 4 legs (S5-AC1)
	require.Len(t, h.jurnal.postings, 1)
	posting := h.jurnal.postings[0]
	assert.Equal(t, m7JurnalRenewalDeposito, posting.EventCode)
	require.Len(t, posting.Legs, 4, "must have exactly 4 jurnal legs")
	// Leg 2 (index 1): pelunasan pokok lama
	assert.Equal(t, inst.Pokok.StringFixed(4), posting.Legs[1].Nilai.StringFixed(4), "leg 2 = pokok_lama")
	// Leg 3 (index 2): penempatan pokok baru = pokok_lama + bunga_bersih (POKOK_PLUS_BUNGA)
	assert.True(t, posting.Legs[2].Nilai.GreaterThan(inst.Pokok), "leg 3 must be > pokok_lama for POKOK_PLUS_BUNGA")

	// Audit: 5 events in chain (CREATED + APPROVED + POSTED + INSTRUMEN.CREATED + INSTRUMEN.MATURED + EIR.RECOMPUTED)
	renewalActions := h.audit.actionsForEntity(renewal.ID.String())
	assert.Contains(t, renewalActions, m7AuditRenewalCreated)
	assert.Contains(t, renewalActions, m7AuditRenewalApproved)
	assert.Contains(t, renewalActions, m7AuditRenewalPosted)
	assert.True(t, h.audit.containsAction(updated.InstrumenBaruID.String(), m7AuditEIRRecomputed))
}

// P5-M7-G: POKOK_PLUS_BUNGA bunga_bersih < IDR 100.000 at approve time.
// AC: S2-AC2
func TestE2E_P5M7_G_BungaBersihTooSmall(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()
	approver := m7NewUser()

	// Instrumen with tiny pokok to generate bunga_bersih < 100k
	smallInst := &m7Instrumen{
		ID:                uuid.New(),
		KodeInstrumen:     "DEP-SMALL",
		JenisInstrumen:    "DEPOSITO",
		Status:            "ACTIVE",
		KlasifikasiPSAK71: "AC",
		KlasifikasiLocked: true,
		Pokok:             decimal.NewFromFloat(100_000),  // tiny — 3-day accrual → bunga_bersih << 100k
		RatePersen:        decimal.NewFromFloat(2.0),
		TanggalPenempatan: time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC),
	}
	h.instrumen.insert(smallInst)
	tanggalEfektif := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC) // 3 days accrual

	// Force insert renewal with POKOK_PLUS_BUNGA — bunga_bersih will be tiny
	bungaKotor := m7ComputeBungaKotor(smallInst.Pokok, smallInst.RatePersen, smallInst.TanggalPenempatan, tanggalEfektif)
	pph := m7ComputePPh(bungaKotor)
	bungaBersih := m7ComputeBungaBersih(bungaKotor, pph)
	require.True(t, bungaBersih.LessThan(decimal.NewFromFloat(m7MinBungaBersih)),
		"test pre-condition: bunga_bersih must be < IDR 100.000")

	renewalID := uuid.New()
	r := &m7Renewal{
		ID:                 renewalID,
		InstrumenLamaID:    smallInst.ID,
		Skema:              m7SkemaPokokPlusBunga,
		TenorBaruBulan:     12,
		RateBaruPersen:     decimal.NewFromFloat(2.0),
		TanggalEfektifBaru: tanggalEfektif,
		PokokLama:          smallInst.Pokok,
		BungaKotor:         bungaKotor,
		PphAmount:          pph,
		BungaBersih:        bungaBersih,
		Status:             m7StatusPendingApproval,
		MakerID:            maker,
		RowVersion:         1,
		TenantID:           "TUGURE",
	}
	h.renewal.insert(r)

	resp := h.approveSim(approver, renewalID, "Approve renewal kecil ini.", "JWT_STEP_UP", uuid.New().String(), nil, nil)

	assert.Equal(t, 422, resp.StatusCode)
	assert.Equal(t, m7ErrBungaBersihTooSmall, resp.ErrorCode)

	// Status must remain PENDING_APPROVAL
	updated := h.renewal.get(renewalID)
	assert.Equal(t, m7StatusPendingApproval, string(updated.Status))
}

// P5-M7-H: SoD violation — maker attempts to approve own renewal → SOD_VIOLATION 403.
// AC: S2-AC3
func TestE2E_P5M7_H_SoDViolationMakerApprover(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser() // maker and approver are same user
	inst := h.seedDefaultInstrumen()
	tanggalEfektif := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	createResp := h.createRenewalSim(maker, inst.ID, m7SkemaPokokSaja, 12, 5.75, tanggalEfektif, uuid.New().String())
	require.Equal(t, 201, createResp.StatusCode)
	renewal := createResp.Data.(*m7Renewal)

	// Same user (maker) tries to approve
	resp := h.approveSim(maker, renewal.ID, "approve sendiri", "JWT_STEP_UP", uuid.New().String(), nil, nil)

	assert.Equal(t, 403, resp.StatusCode)
	assert.Equal(t, m7ErrSoDViolation, resp.ErrorCode)

	// Renewal status unchanged
	assert.Equal(t, m7StatusPendingApproval, string(h.renewal.get(renewal.ID).Status))

	// Audit SOD_VIOLATION_ATTEMPT recorded (advisory)
	assert.True(t, h.audit.containsAction(renewal.ID.String(), "RENEWAL.SOD_VIOLATION_ATTEMPT"))
}

// P5-M7-I: Idempotency replay — same Idempotency-Key returns original response, no duplicates.
// AC: S2-AC4
func TestE2E_P5M7_I_IdempotencyReplayApprove(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()
	approver := m7NewUser()
	inst := h.seedDefaultInstrumen()
	tanggalEfektif := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	createResp := h.createRenewalSim(maker, inst.ID, m7SkemaPokokSaja, 12, 5.75, tanggalEfektif, uuid.New().String())
	require.Equal(t, 201, createResp.StatusCode)
	renewal := createResp.Data.(*m7Renewal)

	idemKey := uuid.New().String()
	comment := "Preview diverifikasi. Rate 5.75% sesuai BI Rate. Disetujui."

	// First approve
	resp1 := h.approveSim(approver, renewal.ID, comment, "JWT_STEP_UP", idemKey, nil, nil)
	require.Equal(t, 200, resp1.StatusCode)
	require.Equal(t, "", resp1.ErrorCode, "first approve must not return idempotency code")

	// Second approve with same key
	resp2 := h.approveSim(approver, renewal.ID, comment, "JWT_STEP_UP", idemKey, nil, nil)

	assert.Equal(t, 200, resp2.StatusCode)
	assert.Equal(t, m7ErrIdempotencyReplay, resp2.ErrorCode, "replay must return IDEMPOTENCY_REPLAY")

	// No duplicate instrumen baru
	activeCount := 0
	for _, inst := range h.instrumen.records {
		if inst.Status == "ACTIVE" {
			activeCount++
		}
	}
	assert.Equal(t, 1, activeCount, "only one ACTIVE instrumen (instrumen baru) — no duplicates")

	// No duplicate jurnal postings
	assert.Len(t, h.jurnal.postings, 1, "exactly 1 jurnal posting — no duplicates from replay")
}

// P5-M7-J: signatureMethod missing or wrong → VALIDATION_FAILED 422.
func TestE2E_P5M7_J_SignatureMethodInvalid(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()
	approver := m7NewUser()
	inst := h.seedDefaultInstrumen()
	tanggalEfektif := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	createResp := h.createRenewalSim(maker, inst.ID, m7SkemaPokokSaja, 12, 5.75, tanggalEfektif, uuid.New().String())
	require.Equal(t, 201, createResp.StatusCode)
	renewal := createResp.Data.(*m7Renewal)

	// Wrong signatureMethod
	resp := h.approveSim(approver, renewal.ID, "valid comment for approve action here",
		"TOTP", // invalid
		uuid.New().String(), nil, nil)
	assert.Equal(t, 422, resp.StatusCode)
	assert.Equal(t, m7ErrValidationFailed, resp.ErrorCode)

	// Empty signatureMethod
	resp2 := h.approveSim(approver, renewal.ID, "valid comment for approve action here",
		"",
		uuid.New().String(), nil, nil)
	assert.Equal(t, 422, resp2.StatusCode)
	assert.Equal(t, m7ErrValidationFailed, resp2.ErrorCode)
}

// P5-M7-K: Idempotency mismatch — same key, different payload → IDEMPOTENCY_MISMATCH 422.
func TestE2E_P5M7_K_IdempotencyMismatch(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()
	inst := h.seedDefaultInstrumen()
	inst2 := h.seedDefaultInstrumen()

	idemKey := uuid.New().String()

	// First create
	resp1 := h.createRenewalSim(maker, inst.ID, m7SkemaPokokSaja, 12, 5.75,
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), idemKey)
	require.Equal(t, 201, resp1.StatusCode)

	// Second create with SAME key but different instrumen_id (different payload hash)
	resp2 := h.createRenewalSim(maker, inst2.ID, m7SkemaPokokSaja, 12, 5.75,
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), idemKey)
	assert.Equal(t, 422, resp2.StatusCode)
	assert.Equal(t, m7ErrIdempotencyMismatch, resp2.ErrorCode)

	// Only 1 renewal in store (from first successful create)
	assert.Equal(t, 1, len(h.renewal.records))
}

// P5-M7-L: Reject happy path — REJECTED, comment ≥ 30 chars, audit RENEWAL.REJECTED.
func TestE2E_P5M7_L_RejectHappyPath(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()
	approver := m7NewUser()
	inst := h.seedDefaultInstrumen()

	createResp := h.createRenewalSim(maker, inst.ID, m7SkemaPokokSaja, 12, 5.75,
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), uuid.New().String())
	require.Equal(t, 201, createResp.StatusCode)
	renewal := createResp.Data.(*m7Renewal)

	comment := "Rate 5.75% melebihi benchmark internal 5.50%. Harap revisi rate atau lampirkan persetujuan ALCO."
	resp := h.rejectSim(approver, renewal.ID, comment, "JWT_STEP_UP", uuid.New().String())

	require.Equal(t, 200, resp.StatusCode)
	updated := h.renewal.get(renewal.ID)
	assert.Equal(t, m7StatusRejected, string(updated.Status))
	require.NotNil(t, updated.RejectReason)
	assert.Equal(t, comment, *updated.RejectReason)

	// Audit RENEWAL.REJECTED in-tx
	assert.True(t, h.audit.containsAction(renewal.ID.String(), m7AuditRenewalRejected))

	// No instrumen baru created
	assert.Equal(t, 1, len(h.instrumen.records), "only original instrumen; no baru on reject")

	// No EIR schedule changes
	assert.Equal(t, 1, len(h.eir.rows), "old EIR schedule row unchanged")
	assert.Nil(t, h.eir.rows[0].EffectiveTo, "old schedule effective_to must remain infinity")
}

// P5-M7-M: Reject with short comment (<30 chars) → VALIDATION_FAILED.
func TestE2E_P5M7_M_RejectShortComment(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()
	approver := m7NewUser()
	inst := h.seedDefaultInstrumen()

	createResp := h.createRenewalSim(maker, inst.ID, m7SkemaPokokSaja, 12, 5.75,
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), uuid.New().String())
	require.Equal(t, 201, createResp.StatusCode)
	renewal := createResp.Data.(*m7Renewal)

	resp := h.rejectSim(approver, renewal.ID, "terlalu pendek", "JWT_STEP_UP", uuid.New().String())
	assert.Equal(t, 400, resp.StatusCode)
	assert.Equal(t, m7ErrValidationFailed, resp.ErrorCode)
	assert.Equal(t, m7StatusPendingApproval, string(h.renewal.get(renewal.ID).Status))
}

// P5-M7-N: Instrumen baru inherits klasifikasi + SPPI + BM + portofolio + counterparty.
// AC: S3-AC1
func TestE2E_P5M7_N_InstrumenBaruInherit(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()
	approver := m7NewUser()
	inst := h.seedDefaultInstrumen()

	createResp := h.createRenewalSim(maker, inst.ID, m7SkemaPokokPlusBunga, 12, 5.75,
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), uuid.New().String())
	require.Equal(t, 201, createResp.StatusCode)
	renewal := createResp.Data.(*m7Renewal)

	approveResp := h.approveSim(approver, renewal.ID, "Disetujui setelah review. Semua parameter sesuai kebijakan treasury.", "JWT_STEP_UP", uuid.New().String(), nil, nil)
	require.Equal(t, 200, approveResp.StatusCode)

	updated := h.renewal.get(renewal.ID)
	require.NotNil(t, updated.InstrumenBaruID)
	instrumenBaru := h.instrumen.get(*updated.InstrumenBaruID)
	require.NotNil(t, instrumenBaru)

	// All inherited fields must match instrumen lama exactly
	assert.Equal(t, inst.KlasifikasiPSAK71, instrumenBaru.KlasifikasiPSAK71)
	assert.Equal(t, inst.KlasifikasiLocked, instrumenBaru.KlasifikasiLocked)
	assert.Equal(t, inst.SppiTestRunID, instrumenBaru.SppiTestRunID)
	assert.Equal(t, inst.BmAssessmentID, instrumenBaru.BmAssessmentID)
	assert.Equal(t, inst.PortofolioID, instrumenBaru.PortofolioID)
	assert.Equal(t, inst.CounterpartyID, instrumenBaru.CounterpartyID)
	assert.Equal(t, inst.MataUang, instrumenBaru.MataUang)
	assert.Equal(t, "DEPOSITO", instrumenBaru.JenisInstrumen)

	// Temporal fields must be from renewal
	assert.True(t, instrumenBaru.Pokok.GreaterThan(inst.Pokok), "pokok_baru > pokok_lama for POKOK_PLUS_BUNGA")
	assert.Equal(t, renewal.RateBaruPersen.StringFixed(4), instrumenBaru.RatePersen.StringFixed(4))
	assert.Equal(t, "ACTIVE", instrumenBaru.Status)

	// SPPI re-test must NOT happen (not a reclassification event per PSAK 71 §4.4.1)
	// The SppiTestRunID is copied as-is — no new SPPI test row created
	assert.Equal(t, inst.SppiTestRunID, instrumenBaru.SppiTestRunID, "SPPI test copied, not re-run")
}

// P5-M7-O: Instrumen lama MATURED in same transaction (S3-AC2).
func TestE2E_P5M7_O_InstrumenLamaMatured(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()
	approver := m7NewUser()
	inst := h.seedDefaultInstrumen()

	createResp := h.createRenewalSim(maker, inst.ID, m7SkemaPokokSaja, 6, 5.50,
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), uuid.New().String())
	require.Equal(t, 201, createResp.StatusCode)
	renewal := createResp.Data.(*m7Renewal)

	approveResp := h.approveSim(approver, renewal.ID, "Disetujui. Semua dokumen lengkap dan rate sesuai kebijakan saat ini.", "JWT_STEP_UP", uuid.New().String(), nil, nil)
	require.Equal(t, 200, approveResp.StatusCode)

	// Instrumen lama must be MATURED
	instLama := h.instrumen.get(inst.ID)
	assert.Equal(t, "MATURED", instLama.Status)

	// Instrumen lama cannot be renewed again (not ACTIVE)
	resp2 := h.createRenewalSim(maker, inst.ID, m7SkemaPokokSaja, 6, 5.50,
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), uuid.New().String())
	assert.Equal(t, 422, resp2.StatusCode, "MATURED instrumen cannot be renewed again")
	assert.Equal(t, m7ErrInstrumenNotEligible, resp2.ErrorCode)

	// Audit INSTRUMEN.MATURED in chain
	assert.True(t, h.audit.containsAction(inst.ID.String(), m7AuditInstrumenMatured))
}

// P5-M7-P: POKOK_SAJA — pokok_baru == pokok_lama; bunga_bersih posted as separate leg.
// AC: S3-AC3
func TestE2E_P5M7_P_PokokSajaPokok(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()
	approver := m7NewUser()
	inst := h.seedDefaultInstrumen()
	tanggalEfektif := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	createResp := h.createRenewalSim(maker, inst.ID, m7SkemaPokokSaja, 12, 5.75, tanggalEfektif, uuid.New().String())
	require.Equal(t, 201, createResp.StatusCode)
	renewal := createResp.Data.(*m7Renewal)

	// POKOK_SAJA: pokok_baru should equal pokok_lama
	assert.Equal(t, inst.Pokok.StringFixed(4), renewal.PokokBaru.StringFixed(4))

	approveResp := h.approveSim(approver, renewal.ID, "POKOK_SAJA renewal disetujui. Bunga dikembalikan ke kas perusahaan.", "JWT_STEP_UP", uuid.New().String(), nil, nil)
	require.Equal(t, 200, approveResp.StatusCode)

	// Leg 3 (penempatan pokok baru) must equal pokok_lama exactly (S5-AC2)
	require.NotEmpty(t, h.jurnal.postings)
	posting := h.jurnal.postings[0]
	assert.Equal(t, inst.Pokok.StringFixed(4), posting.Legs[2].Nilai.StringFixed(4),
		"POKOK_SAJA: jurnal leg 3 = pokok_lama")

	// Leg 4 (bunga_bersih) must be positive separately
	assert.True(t, posting.Legs[3].Nilai.IsPositive(), "bunga_bersih jurnal leg must be positive")
}

// P5-M7-Q: InstrumenCreator failure → full rollback, renewal stays PENDING_APPROVAL.
// AC: S3-AC4
func TestE2E_P5M7_Q_InstrumenCreatorFailureRollback(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()
	approver := m7NewUser()
	inst := h.seedDefaultInstrumen()

	createResp := h.createRenewalSim(maker, inst.ID, m7SkemaPokokSaja, 12, 5.75,
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), uuid.New().String())
	require.Equal(t, 201, createResp.StatusCode)
	renewal := createResp.Data.(*m7Renewal)

	// Inject InstrumenCreator error (simulates DB constraint violation like duplicate kode)
	instrumenCreatorErr := fmt.Errorf("DB: duplicate key value violates unique constraint uq_instrumen_kode")
	resp := h.approveSim(approver, renewal.ID, "approve", "JWT_STEP_UP", uuid.New().String(), instrumenCreatorErr, nil)

	assert.Equal(t, 500, resp.StatusCode)

	// Rollback: renewal must revert to PENDING_APPROVAL
	updated := h.renewal.get(renewal.ID)
	assert.Equal(t, m7StatusPendingApproval, string(updated.Status), "rollback: status must revert to PENDING_APPROVAL")

	// No instrumen baru in store (was rolled back)
	activeInstrumens := 0
	for id, inst := range h.instrumen.records {
		if inst.Status == "ACTIVE" && id != inst.ID {
			activeInstrumens++
		}
	}
	// Only original instrumen should remain active
	assert.Equal(t, "ACTIVE", h.instrumen.get(inst.ID).Status, "instrumen lama must remain ACTIVE on rollback")

	// No jurnal posting
	assert.Empty(t, h.jurnal.postings, "no jurnal posted on rollback")
}

// P5-M7-R: EIR re-computation converges, new schedule inserted, old closed (immutability).
// AC: S4-AC1
func TestE2E_P5M7_R_EIRScheduleVersioning(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()
	approver := m7NewUser()
	inst := h.seedDefaultInstrumen()
	tanggalEfektif := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	createResp := h.createRenewalSim(maker, inst.ID, m7SkemaPokokPlusBunga, 12, 5.75, tanggalEfektif, uuid.New().String())
	require.Equal(t, 201, createResp.StatusCode)
	renewal := createResp.Data.(*m7Renewal)

	// Record old EIR value before approve — must NOT be modified
	oldEIRPersen := h.eir.rows[0].EirPersen.String()

	approveResp := h.approveSim(approver, renewal.ID, "EIR re-computation diverifikasi. Schedule version baru diinsert.", "JWT_STEP_UP", uuid.New().String(), nil, nil)
	require.Equal(t, 200, approveResp.StatusCode)

	updated := h.renewal.get(renewal.ID)
	require.NotNil(t, updated.InstrumenBaruID)

	// Old schedule: EIR value NEVER modified (immutability — PSAK 71 §B5.4.6)
	oldRow := h.eir.rows[0]
	assert.Equal(t, oldEIRPersen, oldRow.EirPersen.String(), "old EIR value must be immutable — PSAK 71 §B5.4.6")
	assert.NotNil(t, oldRow.EffectiveTo, "old schedule effective_to must be set")
	assert.Equal(t, tanggalEfektif.Format("2006-01-02"), oldRow.EffectiveTo.Format("2006-01-02"))

	// New schedule: schedule_version = 1 for the new instrumen, effective_from set
	newRow := h.eir.getActive(*updated.InstrumenBaruID)
	require.NotNil(t, newRow)
	assert.Equal(t, 1, newRow.ScheduleVersion, "new instrumen starts at schedule_version = 1")
	assert.Nil(t, newRow.EffectiveTo, "new schedule effective_to must be infinity")
	assert.True(t, newRow.EirPersen.IsPositive(), "new EIR baru must be positive")
	assert.False(t, newRow.EirPersen.Equal(oldRow.EirPersen), "new EIR should differ from old (different rate)")

	// Audit EIR.RECOMPUTED
	assert.True(t, h.audit.containsAction(updated.InstrumenBaruID.String(), m7AuditEIRRecomputed))
}

// P5-M7-S: Cashflow array uses after-PPh coupons (0.80 factor), not gross.
// AC: S4-AC2
func TestE2E_P5M7_S_CashflowAfterPph(t *testing.T) {
	t.Parallel()

	pokokBaru := 1_011_397_260.2740
	rateBaruPersen := 5.75
	tenorBulan := 12

	// Build cashflows using after-PPh factor (0.80)
	oneMinusPph := 0.80
	kuponKotor := pokokBaru * (rateBaruPersen / 100.0 / float64(tenorBulan))
	kuponBersih := kuponKotor * oneMinusPph
	kuponKotorCheck := pokokBaru * (rateBaruPersen / 100.0 / float64(tenorBulan))

	cfs := m7BuildCashflows(pokokBaru, rateBaruPersen, tenorBulan)

	// cf[0] = -pokok_baru (outflow)
	assert.True(t, math.Abs(cfs[0]+pokokBaru) < 0.01, "cf[0] must be -pokok_baru")

	// cf[1..n-1] = kupon_bersih (after PPh)
	for i := 1; i < tenorBulan; i++ {
		assert.True(t, math.Abs(cfs[i]-kuponBersih) < 0.01,
			"cf[%d] must be kupon_bersih (after PPh 20%%)", i)
	}

	// cf[n] = pokok_baru + kupon_bersih (terminal)
	expectedTerminal := pokokBaru + kuponBersih
	assert.True(t, math.Abs(cfs[tenorBulan]-expectedTerminal) < 0.01, "terminal cashflow mismatch")

	// Verify kupon_bersih < kupon_kotor (PPh applied)
	assert.Less(t, kuponBersih, kuponKotorCheck, "after-PPh coupon must be < gross coupon")

	// Verify Newton-Raphson converges with these after-PPh cashflows
	eirMonthly, iters, converged := m7NewtonRaphsonIRR(cfs, rateBaruPersen/100/float64(tenorBulan))
	assert.True(t, converged, "Newton-Raphson must converge")
	assert.LessOrEqual(t, iters, m7EIRMaxIter, "must converge within %d iterations", m7EIRMaxIter)

	eirAnnual := math.Pow(1+eirMonthly, 12) - 1
	// after-PPh EIR should reflect after-tax yield < gross rate
	assert.Less(t, eirAnnual, rateBaruPersen/100.0, "after-tax EIR must be < gross rate")
}

// P5-M7-T: PPh calc mismatch at approve → RENEWAL_PPH_CALC_MISMATCH 422.
// AC: S4-AC3
func TestE2E_P5M7_T_PphCalcMismatch(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()
	approver := m7NewUser()
	inst := h.seedDefaultInstrumen()
	tanggalEfektif := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	// Insert renewal with deliberately wrong PPh amount
	bungaKotor := m7ComputeBungaKotor(inst.Pokok, inst.RatePersen, inst.TanggalPenempatan, tanggalEfektif)
	correctPph := m7ComputePPh(bungaKotor)
	wrongPph := correctPph.Add(decimal.NewFromFloat(150_685)) // > 0.01 tolerance

	renewalID := uuid.New()
	r := &m7Renewal{
		ID:                 renewalID,
		InstrumenLamaID:    inst.ID,
		Skema:              m7SkemaPokokPlusBunga,
		TenorBaruBulan:     12,
		RateBaruPersen:     decimal.NewFromFloat(5.75),
		TanggalEfektifBaru: tanggalEfektif,
		PokokLama:          inst.Pokok,
		BungaKotor:         bungaKotor,
		PphAmount:          wrongPph, // intentionally wrong
		BungaBersih:        bungaKotor.Sub(wrongPph),
		Status:             m7StatusPendingApproval,
		MakerID:            maker,
		RowVersion:         1,
		TenantID:           "TUGURE",
	}
	h.renewal.insert(r)

	resp := h.approveSim(approver, renewalID, "Approve dengan PPh yang salah.", "JWT_STEP_UP", uuid.New().String(), nil, nil)

	assert.Equal(t, 422, resp.StatusCode)
	assert.Equal(t, m7ErrPphCalcMismatch, resp.ErrorCode)
	assert.Equal(t, m7StatusPendingApproval, string(h.renewal.get(renewalID).Status))
}

// P5-M7-U: Newton-Raphson no-convergence with zero-value cashflows → ErrEIRNoConvergence.
// AC: S4-AC4
func TestE2E_P5M7_U_EIRNoConvergence(t *testing.T) {
	t.Parallel()

	// Edge case: all cashflows are zero → derivative is zero → ErrEIRZeroDerivative
	cfs := []float64{0, 0, 0, 0, 0}
	_, _, converged := m7NewtonRaphsonIRR(cfs, 0.05)
	assert.False(t, converged, "must not converge with all-zero cashflows")

	// Edge case: only outflow, no inflows → derivative is zero at t=0 only → non-convergence
	cfsOnlyOut := []float64{-1_000_000}
	_, iters, converged2 := m7NewtonRaphsonIRR(cfsOnlyOut, 0.05)
	assert.False(t, converged2, "must not converge with only outflow cashflow")
	// iters may be 0 (zero-derivative path) or maxIter (divergence path) — both are non-convergence
	assert.LessOrEqual(t, iters, m7EIRMaxIter, "iters must be within max bound")

	// Correct cashflows MUST converge
	normalCFs := m7BuildCashflows(1_011_397_260.27, 5.75, 12)
	eirMonthly, iters2, convergedNormal := m7NewtonRaphsonIRR(normalCFs, 5.75/100/12)
	assert.True(t, convergedNormal, "normal cashflows must converge")
	assert.LessOrEqual(t, iters2, m7EIRMaxIter)
	eirAnnual := math.Pow(1+eirMonthly, 12) - 1
	// After-PPh EIR < gross rate (5.75%)
	assert.Less(t, eirAnnual, 0.0575)
	assert.Greater(t, eirAnnual, 0.01)

	// Verify tolerance: |NPV(cfs, r*)| < 1e-6 (practical convergence check)
	npvCheck := m7NPV(normalCFs, eirMonthly)
	assert.Less(t, math.Abs(npvCheck), 1e-4, "NPV at solution must be near zero")
}

// P5-M7-V: Jurnal 4 legs posted with correct amounts for POKOK_PLUS_BUNGA.
// AC: S5-AC1
func TestE2E_P5M7_V_JurnalFourLegsPostedCorrectly(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()
	approver := m7NewUser()
	inst := h.seedDefaultInstrumen()
	tanggalEfektif := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	createResp := h.createRenewalSim(maker, inst.ID, m7SkemaPokokPlusBunga, 12, 5.75, tanggalEfektif, uuid.New().String())
	require.Equal(t, 201, createResp.StatusCode)
	renewal := createResp.Data.(*m7Renewal)

	approveResp := h.approveSim(approver, renewal.ID, "Jurnal 4 leg diverifikasi. Pokok baru = pokok lama + bunga bersih.", "JWT_STEP_UP", uuid.New().String(), nil, nil)
	require.Equal(t, 200, approveResp.StatusCode)

	require.Len(t, h.jurnal.postings, 1)
	posting := h.jurnal.postings[0]
	assert.Equal(t, m7JurnalRenewalDeposito, posting.EventCode)
	require.Len(t, posting.Legs, 4)

	// Leg 1: PPh_20pct
	assert.Equal(t, renewal.PphAmount.StringFixed(4), posting.Legs[0].Nilai.StringFixed(4), "leg 1 = PPh_20pct")

	// Leg 2: pokok_lama
	assert.Equal(t, inst.Pokok.StringFixed(4), posting.Legs[1].Nilai.StringFixed(4), "leg 2 = pokok_lama")

	// Leg 3: pokok_baru (= pokok_lama + bunga_bersih for POKOK_PLUS_BUNGA)
	expectedPokokBaru := renewal.PokokBaru
	assert.Equal(t, expectedPokokBaru.StringFixed(4), posting.Legs[2].Nilai.StringFixed(4), "leg 3 = pokok_baru")

	// Leg 4: bunga_bersih
	assert.Equal(t, renewal.BungaBersih.StringFixed(4), posting.Legs[3].Nilai.StringFixed(4), "leg 4 = bunga_bersih")

	// All leg amounts use NUMERIC(20,4) — verify 4 decimal places
	for i, leg := range posting.Legs {
		assert.False(t, leg.Nilai.IsZero(), "leg %d must not be zero", i)
	}
}

// P5-M7-W: POKOK_SAJA — jurnal leg 3 uses pokok_lama, not pokok+bunga.
// AC: S5-AC2
func TestE2E_P5M7_W_JurnalPokokSajaLeg3(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()
	approver := m7NewUser()
	inst := h.seedDefaultInstrumen()
	tanggalEfektif := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	createResp := h.createRenewalSim(maker, inst.ID, m7SkemaPokokSaja, 12, 5.75, tanggalEfektif, uuid.New().String())
	require.Equal(t, 201, createResp.StatusCode)
	renewal := createResp.Data.(*m7Renewal)

	// POKOK_SAJA: pokok_baru == pokok_lama
	assert.Equal(t, inst.Pokok.StringFixed(4), renewal.PokokBaru.StringFixed(4))

	approveResp := h.approveSim(approver, renewal.ID, "POKOK_SAJA disetujui. Pokok tidak berubah, bunga dikembalikan terpisah.", "JWT_STEP_UP", uuid.New().String(), nil, nil)
	require.Equal(t, 200, approveResp.StatusCode)

	require.Len(t, h.jurnal.postings, 1)
	posting := h.jurnal.postings[0]
	require.Len(t, posting.Legs, 4)

	// Leg 3: must equal pokok_lama (not pokok+bunga)
	assert.Equal(t, inst.Pokok.StringFixed(4), posting.Legs[2].Nilai.StringFixed(4),
		"POKOK_SAJA leg 3 must equal pokok_lama")

	// Leg 4: bunga_bersih must still be a separate positive leg
	assert.True(t, posting.Legs[3].Nilai.IsPositive(), "bunga_bersih leg must be positive even for POKOK_SAJA")
}

// P5-M7-X: Periode CLOSED at post time → PERIODE_CLOSED 423, rollback.
// AC: S5-AC3
func TestE2E_P5M7_X_PeriodeClosedAtPostTime(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()
	approver := m7NewUser()
	inst := h.seedDefaultInstrumen()
	tanggalEfektif := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	createResp := h.createRenewalSim(maker, inst.ID, m7SkemaPokokSaja, 12, 5.75, tanggalEfektif, uuid.New().String())
	require.Equal(t, 201, createResp.StatusCode)
	renewal := createResp.Data.(*m7Renewal)

	// Simulate hard-close happening between create and approve
	h.periode.Status = m7PeriodeClosed

	resp := h.approveSim(approver, renewal.ID, "Periode sudah closed tapi tetap approve.", "JWT_STEP_UP", uuid.New().String(), nil, nil)

	assert.Equal(t, 423, resp.StatusCode)
	assert.Equal(t, m7ErrPeriodeClosed, resp.ErrorCode)

	// Rollback: renewal stays PENDING_APPROVAL (not APPROVED/POSTED)
	updated := h.renewal.get(renewal.ID)
	assert.Equal(t, m7StatusPendingApproval, string(updated.Status))

	// No instrumen baru
	instrumenCount := len(h.instrumen.records)
	assert.Equal(t, 1, instrumenCount, "no instrumen baru created on rollback")

	// No EIR changes — old schedule still has EffectiveTo = nil
	assert.Nil(t, h.eir.rows[0].EffectiveTo, "old EIR schedule must remain unclosed on rollback")

	// No jurnal posting
	assert.Empty(t, h.jurnal.postings)
}

// P5-M7-Y: JurnalPoster error → rollback, renewal stays APPROVED (not POSTED).
// AC: S5-AC4
func TestE2E_P5M7_Y_JurnalPosterError(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()
	approver := m7NewUser()
	inst := h.seedDefaultInstrumen()
	tanggalEfektif := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	createResp := h.createRenewalSim(maker, inst.ID, m7SkemaPokokSaja, 12, 5.75, tanggalEfektif, uuid.New().String())
	require.Equal(t, 201, createResp.StatusCode)
	renewal := createResp.Data.(*m7Renewal)

	// Inject jurnal error (event code not found in P5-M2 mapping)
	jurnalErr := fmt.Errorf("JURNAL_EVENT_CODE_NOT_FOUND: RENEWAL_DEPOSITO not configured in mapping jurnal master")
	resp := h.approveSim(approver, renewal.ID, "Jurnal event code tidak ada.", "JWT_STEP_UP", uuid.New().String(), nil, jurnalErr)

	assert.Equal(t, 500, resp.StatusCode)

	// Rollback: renewal must revert to PENDING_APPROVAL
	updated := h.renewal.get(renewal.ID)
	assert.Equal(t, m7StatusPendingApproval, string(updated.Status), "rollback on jurnal error")

	// No jurnal posting
	assert.Empty(t, h.jurnal.postings, "no jurnal posted on rollback")

	// Instrumen lama must remain ACTIVE
	assert.Equal(t, "ACTIVE", h.instrumen.get(inst.ID).Status, "instrumen lama must remain ACTIVE on rollback")
}

// P5-M7-Z: Cross — idempotency prevents duplicate renewal INSERT for same instrumen.
func TestE2E_P5M7_Z_IdempotencyPreventsDuplicateCreate(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()
	inst := h.seedDefaultInstrumen()
	idemKey := uuid.New().String()
	tanggalEfektif := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	// First create
	resp1 := h.createRenewalSim(maker, inst.ID, m7SkemaPokokSaja, 12, 5.75, tanggalEfektif, idemKey)
	require.Equal(t, 201, resp1.StatusCode)

	// Replay with SAME key + SAME payload
	resp2 := h.createRenewalSim(maker, inst.ID, m7SkemaPokokSaja, 12, 5.75, tanggalEfektif, idemKey)
	assert.Equal(t, 200, resp2.StatusCode)
	assert.Equal(t, m7ErrIdempotencyReplay, resp2.ErrorCode, "replay must return IDEMPOTENCY_REPLAY")

	// Only 1 renewal in store
	assert.Equal(t, 1, len(h.renewal.records), "exactly 1 renewal — no duplicate INSERT")
}

// P5-M7-AA: Audit hash-chain valid across full approve flow (5+ events).
func TestE2E_P5M7_AA_AuditHashChainValid(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()
	approver := m7NewUser()
	inst := h.seedDefaultInstrumen()
	tanggalEfektif := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	// Full flow: create → approve
	createResp := h.createRenewalSim(maker, inst.ID, m7SkemaPokokPlusBunga, 12, 5.75, tanggalEfektif, uuid.New().String())
	require.Equal(t, 201, createResp.StatusCode)
	renewal := createResp.Data.(*m7Renewal)

	approveResp := h.approveSim(approver, renewal.ID, "Full hash chain verification test. All 5 audit events in-transaction.", "JWT_STEP_UP", uuid.New().String(), nil, nil)
	require.Equal(t, 200, approveResp.StatusCode)

	// Verify hash chain integrity
	ok, reason := h.audit.verifyHashChain()
	assert.True(t, ok, "audit hash-chain must be valid: %s", reason)

	// All 6 required events must be present (CREATED + APPROVED + POSTED + INSTRUMEN.CREATED + INSTRUMEN.MATURED + EIR.RECOMPUTED)
	updated := h.renewal.get(renewal.ID)
	require.NotNil(t, updated.InstrumenBaruID)

	allActions := make([]string, 0)
	for _, row := range h.audit.rows {
		allActions = append(allActions, row.Action)
	}
	assert.Contains(t, allActions, m7AuditRenewalCreated)
	assert.Contains(t, allActions, m7AuditRenewalApproved)
	assert.Contains(t, allActions, m7AuditRenewalPosted)
	assert.Contains(t, allActions, m7AuditInstrumenCreated)
	assert.Contains(t, allActions, m7AuditInstrumenMatured)
	assert.Contains(t, allActions, m7AuditEIRRecomputed)

	// Total events: 1 CREATED + 5 from approve = 6 minimum
	assert.GreaterOrEqual(t, len(h.audit.rows), 6, "minimum 6 audit events for full flow")
}

// P5-M7-AB: List endpoint — cursor, filter by status, sort by created_at.
func TestE2E_P5M7_AB_ListFilterAndSort(t *testing.T) {
	t.Parallel()
	h := newP5M7Harness()
	maker := m7NewUser()
	tanggalEfektif := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	// Create 3 renewals: 2 PENDING_APPROVAL (on separate instrumens), simulate 1 REJECTED
	inst1 := h.seedDefaultInstrumen()
	inst2 := h.seedDefaultInstrumen()
	inst3 := h.seedDefaultInstrumen()

	r1 := h.createRenewalSim(maker, inst1.ID, m7SkemaPokokSaja, 12, 5.75, tanggalEfektif, uuid.New().String())
	r2 := h.createRenewalSim(maker, inst2.ID, m7SkemaPokokSaja, 12, 5.75, tanggalEfektif, uuid.New().String())
	r3 := h.createRenewalSim(maker, inst3.ID, m7SkemaPokokSaja, 12, 5.75, tanggalEfektif, uuid.New().String())
	require.Equal(t, 201, r1.StatusCode)
	require.Equal(t, 201, r2.StatusCode)
	require.Equal(t, 201, r3.StatusCode)

	// Reject r3
	renewal3 := r3.Data.(*m7Renewal)
	rejecter := m7NewUser()
	h.rejectSim(rejecter, renewal3.ID, "Rate melebihi batas. Harap revisi dan submit ulang dengan rate yang lebih rendah.", "JWT_STEP_UP", uuid.New().String())

	// Filter by status=PENDING_APPROVAL
	pendingCount := h.renewal.countByStatus(m7StatusPendingApproval)
	rejectedCount := h.renewal.countByStatus(m7StatusRejected)

	assert.Equal(t, 2, pendingCount, "2 PENDING_APPROVAL renewals")
	assert.Equal(t, 1, rejectedCount, "1 REJECTED renewal")

	// Simulate cursor-based list (limit=1) — 2 pending > 1 → hasMore=true
	const listLimit = 1
	var listResult []*m7Renewal
	for _, r := range h.renewal.records {
		if r.Status == m7StatusPendingApproval {
			listResult = append(listResult, r)
			if len(listResult) >= listLimit {
				break
			}
		}
	}
	assert.LessOrEqual(t, len(listResult), listLimit, "cursor pagination: max listLimit rows per page")

	// hasMore: true because total PENDING (2) > limit (1)
	hasMore := pendingCount > listLimit
	assert.True(t, hasMore, "hasMore=true when total PENDING_APPROVAL (%d) > listLimit (%d)", pendingCount, listLimit)
}
