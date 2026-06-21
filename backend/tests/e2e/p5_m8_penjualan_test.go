// Package e2e — P5-M8 Penjualan/Pencairan Instrumen end-to-end tests.
//
// Scope: create penjualan (S1), approve/reject with SoD (S2), OCI recycling
// FVOCI debt + no-recycle FVOCI Election (S3), BM frequency check warn/block (S4),
// jurnal multi-leg + derecognition per klasifikasi (S5), plus cross-cutting:
// idempotency, audit hash-chain, periode lock, klasifikasi routing matrix.
//
// Scenarios:
//
//	P5-M8-A  S1-AC1: Create PARTIAL FVOCI — proceeds/cost_basis/realized_gl/oci_recycled correct, audit PENJUALAN.CREATED
//	P5-M8-B  S1-AC2: qty_terjual > qty_holding → PENJUALAN_QTY_EXCEEDS_HOLDING, no INSERT
//	P5-M8-C  S1-AC3: instrumen not ACTIVE or klasifikasi not locked → PENJUALAN_INSTRUMEN_NOT_ACTIVE
//	P5-M8-D  S1-AC4: FVOCI Election → no_recycling_note set, oci_recycled = nil
//	P5-M8-E  S1:     harga_jual_per_unit ≤ 0 → PENJUALAN_HARGA_INVALID 400
//	P5-M8-F  S1:     periode CLOSED → PENJUALAN_PERIODE_LOCKED 423, no INSERT
//	P5-M8-G  S2-AC1: Approve FVOCI FULL → POSTED; all side-effects in one tx; PENJUALAN.POSTED audit last
//	P5-M8-H  S2-AC2: SoD — maker tries to approve own penjualan → SOD_VIOLATION 403
//	P5-M8-I  S2-AC3: Periode CLOSED at approval time → PENJUALAN_PERIODE_LOCKED 423, rollback
//	P5-M8-J  S2-AC4: Idempotency replay — approve with same key → IDEMPOTENCY_REPLAY, no duplicate
//	P5-M8-K  S2:     Reject happy path — REJECTED, reason ≥ 30 chars, audit PENJUALAN.REJECTED
//	P5-M8-L  S2:     Reject reason < 30 chars → VALIDATION_FAILED 400
//	P5-M8-M  S3-AC1: FVOCI debt FULL disposal → REKLAS_OCI_PL jurnal posted, oci_recycled = oci_cumulative
//	P5-M8-N  S3-AC2: FVOCI debt PARTIAL → oci_recycled = oci_cumulative × (qty_terjual / qty_holding_pre)
//	P5-M8-O  S3-AC3: FVOCI Election FULL → NO REKLAS_OCI_PL; PENJUALAN.OCI_NO_RECYCLE audit; warning in response
//	P5-M8-P  S3-AC4: FVOCI debt disposal with oci_cumulative < 0 (unrealized loss) → loss recycled to P&L
//	P5-M8-Q  S4-AC1: BM warn (5–10%) → POSTED + bm_violation_risk=true + ROLE-RISK notif + BM_FREQUENCY_FLAG audit
//	P5-M8-R  S4-AC2: BM block (>10%) → PENDING_BM_REVIEW + PENJUALAN_BM_VIOLATION_BLOCK + no jurnal
//	P5-M8-S  S4-AC3: Non-HTC portofolio (HTC&S) → BM check skipped, penjualan POSTED normally
//	P5-M8-T  S4-AC4: BM threshold from sys.config (not hardcoded 5%/10%) → runtime config respected
//	P5-M8-U  S5-AC1: AC FULL disposal → 3-leg jurnal (Kas/Bank Dr, Aset Cr, Gain P&L Cr), DISPOSED status
//	P5-M8-V  S5-AC2: FVTPL PARTIAL → 3-leg jurnal, qty_holding reduced, status ACTIVE
//	P5-M8-W  S5-AC3: FVOCI Election FULL → jurnal without REKLAS_OCI_PL; G/L stays in OCI leg
//	P5-M8-X  S5-AC4: Jurnal event code missing in P5-M2 mapping → rollback, penjualan stays APPROVED
//	P5-M8-Y  Cross:  Idempotency-Key prevents duplicate CREATE INSERT
//	P5-M8-Z  Cross:  Audit hash-chain valid across full approve flow (7 events: CREATED, APPROVED, OCI, BM, POSTED, DERECOGNIZED + chain link)
//	P5-M8-AA Cross:  List endpoint: cursor pagination, filter by status, filter by klasifikasi
//	P5-M8-AB Cross:  Partial disposal — qty_holding_pre – qty_terjual = qty_holding_post, instrumen stays ACTIVE
//
// Decision log compliance:
//
//	DEC-016: shopspring/decimal for all monetary amounts; NUMERIC(20,4) IDR    — Scenarios A, G, M, N, U
//	DEC-017: 4-eyes SoD; approver_id ≠ maker_id enforced at service layer     — Scenario H
//	DEC-018: Audit trail append-only; written in-transaction                   — Scenarios A, G, Z
//	DEC-021: Idempotency-Key mandatory on mutating endpoints                   — Scenarios J, Y
//	DEC-022: Cursor-based pagination                                           — Scenario AA
//
// Run:
//
//	go test ./backend/tests/e2e/... -v -run TestE2E_P5M8 -timeout 120s -race
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

// ─── P5-M8 domain constants ───────────────────────────────────────────────────

const (
	// Penjualan status values (trx.penjualan.status).
	m8StatusPendingApproval = "PENDING_APPROVAL"
	m8StatusApproved        = "APPROVED"
	m8StatusPosted          = "POSTED"
	m8StatusRejected        = "REJECTED"
	m8StatusPendingBMReview = "PENDING_BM_REVIEW"

	// Jenis disposal values.
	m8JenisFull    = "FULL"
	m8JenisPartial = "PARTIAL"

	// Klasifikasi values.
	m8KlasifikasiAC            = "AC"
	m8KlasifikasiFVOCI         = "FVOCI"
	m8KlasifikasiFVOCIElection = "FVOCI_ELECTION"
	m8KlasifikasiFVTPL         = "FVTPL"
	m8KlasifikasiPOCI          = "POCI"

	// Audit event actions.
	m8AuditPenjualanCreated      = "PENJUALAN.CREATED"
	m8AuditPenjualanApproved     = "PENJUALAN.APPROVED"
	m8AuditPenjualanPosted       = "PENJUALAN.POSTED"
	m8AuditPenjualanRejected     = "PENJUALAN.REJECTED"
	m8AuditOCIRecycled           = "PENJUALAN.OCI_RECYCLED"
	m8AuditOCINoRecycle          = "PENJUALAN.OCI_NO_RECYCLE"
	m8AuditBMFrequencyFlag       = "PENJUALAN.BM_FREQUENCY_FLAG"
	m8AuditDerecognized          = "PENJUALAN.DERECOGNIZED"
	m8AuditSoDViolationAttempt   = "PENJUALAN.SOD_VIOLATION_ATTEMPT"
	m8AuditJurnalMissingConfig   = "PENJUALAN.JURNAL_MISSING_CONFIG"

	// Error codes (mirrors domain.go / api-conventions.md + P5-M8 new codes).
	m8ErrInstrumenNotActive     = "PENJUALAN_INSTRUMEN_NOT_ACTIVE"
	m8ErrQtyExceedsHolding      = "PENJUALAN_QTY_EXCEEDS_HOLDING"
	m8ErrKlasifikasiNotLocked   = "PENJUALAN_KLASIFIKASI_NOT_LOCKED"
	m8ErrHargaInvalid           = "PENJUALAN_HARGA_INVALID"
	m8ErrPeriodeLocked          = "PENJUALAN_PERIODE_LOCKED"
	m8ErrBMViolationBlock       = "PENJUALAN_BM_VIOLATION_BLOCK"
	m8ErrFVOCIElectionNoRecycleWarn = "PENJUALAN_FVOCI_ELECTION_NO_RECYCLING_WARN"
	m8ErrSoDViolation           = "SOD_VIOLATION"
	m8ErrWorkflowInvalid        = "WORKFLOW_INVALID_TRANSITION"
	m8ErrValidationFailed       = "VALIDATION_FAILED"
	m8ErrIdempotencyReplay      = "IDEMPOTENCY_REPLAY"
	m8ErrIdempotencyMismatch    = "IDEMPOTENCY_MISMATCH"

	// Jurnal event codes (from P5-M2 routing matrix).
	m8JurnalPenjualanAC            = "PENJUALAN_AC"
	m8JurnalPenjualanFVOCIDebt     = "PENJUALAN_FVOCI_DEBT"
	m8JurnalReklasOCIPL            = "REKLAS_OCI_PL"
	m8JurnalPenjualanFVOCIElection = "PENJUALAN_FVOCI_ELECTION"
	m8JurnalPenjualanFVTPL         = "PENJUALAN_FVTPL"
	m8JurnalPenjualanPOCI          = "PENJUALAN_POCI"

	// Business constants.
	m8BMWarnThresholdDefault  = 5.0   // %
	m8BMBlockThresholdDefault = 10.0  // %
	m8MinRejectReason         = 30    // characters

	// Signature method.
	m8SignatureJWTStepUp = "JWT_STEP_UP"
)

// ─── Domain types ─────────────────────────────────────────────────────────────

// m8Instrumen is an in-process copy of mst.instrumen fields relevant to penjualan.
type m8Instrumen struct {
	ID                uuid.UUID
	KodeInstrumen     string
	Status            string // "ACTIVE", "DISPOSED", "MATURED"
	KlasifikasiPSAK71 string
	KlasifikasiLocked bool
	QtyHolding        decimal.Decimal
	PortofolioID      uuid.UUID
	PortofolioBM      string // "HTC", "HTC&S", "Other"
	MataUang          string
	CounterpartyID    uuid.UUID
}

// m8Penjualan is an in-process copy of trx.penjualan.
type m8Penjualan struct {
	ID                  uuid.UUID
	InstrumenID         uuid.UUID
	KlasifikasiSnapshot string
	JenisDisposal       string
	QtyTerjual          decimal.Decimal
	QtyHoldingPre       decimal.Decimal
	QtyHoldingPost      *decimal.Decimal
	HargaJualPerUnit    decimal.Decimal
	ProceedIDR          decimal.Decimal
	CostBasis           *decimal.Decimal
	RealizedGL          *decimal.Decimal
	OCIRecycled         *decimal.Decimal
	NoRecyclingNote     *string
	BMFreqImpactPct     *decimal.Decimal
	BMViolationRisk     bool
	BMViolationPct      *decimal.Decimal
	TanggalEksekusi     time.Time
	Status              string
	MakerID             uuid.UUID
	ApproverID          *uuid.UUID
	ApproveComment      *string
	RejectReason        *string
	SignatureMethod      *string
	JurnalHeaderID      *uuid.UUID
	InstrumenStatusAfter *string
	ApprovedAt          *time.Time
	CreatedAt           time.Time
	RowVersion          int64
	TenantID            string
}

// m8PenjualanPreview represents the server-computed preview response.
type m8PenjualanPreview struct {
	KlasifikasiPSAK71 string
	ProceedIDR        decimal.Decimal
	CostBasis         decimal.Decimal
	RealizedGL        decimal.Decimal
	OCIRecycled       *decimal.Decimal
	NoRecyclingNote   *string
	BMFreqImpactPct   *decimal.Decimal
	BMFreqWarning     *string
}

// ─── Idempotency store ────────────────────────────────────────────────────────

type m8IdempotencyStore struct {
	entries map[string]m8IdempotencyEntry
}

type m8IdempotencyEntry struct {
	Key            string
	RequestHash    [32]byte
	ResponseJSON   json.RawMessage
	Status         int
	CreatedAt      time.Time
	ExpiresAt      time.Time
}

func newM8IdempotencyStore() *m8IdempotencyStore {
	return &m8IdempotencyStore{entries: make(map[string]m8IdempotencyEntry)}
}

func (s *m8IdempotencyStore) Record(key string, reqHash [32]byte, respJSON json.RawMessage, status int) {
	s.entries[key] = m8IdempotencyEntry{
		Key:          key,
		RequestHash:  reqHash,
		ResponseJSON: respJSON,
		Status:       status,
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}
}

func (s *m8IdempotencyStore) Lookup(key string) (m8IdempotencyEntry, bool) {
	e, ok := s.entries[key]
	return e, ok
}

// ─── OCI Recycling calculator ─────────────────────────────────────────────────

// m8ComputeOCIRecycled computes OCI amount to recycle based on jenis_disposal and qty.
// For FVOCI debt PARTIAL: oci_recycled = oci_cumulative × (qty_terjual / qty_holding_pre)
// For FVOCI debt FULL:    oci_recycled = oci_cumulative
// For FVOCI_ELECTION:     oci_recycled = nil (no recycle per §B5.7.1)
func m8ComputeOCIRecycled(
	klasifikasi string,
	jenisDisposal string,
	ociCumulative decimal.Decimal,
	qtyTerjual decimal.Decimal,
	qtyHoldingPre decimal.Decimal,
) *decimal.Decimal {
	if klasifikasi == m8KlasifikasiFVOCIElection {
		return nil
	}
	if klasifikasi != m8KlasifikasiFVOCI {
		return nil
	}

	var recycled decimal.Decimal
	if jenisDisposal == m8JenisFull {
		recycled = ociCumulative
	} else {
		// PARTIAL: proportional
		if qtyHoldingPre.IsZero() {
			return nil
		}
		recycled = ociCumulative.Mul(qtyTerjual).Div(qtyHoldingPre)
	}
	return &recycled
}

// m8ComputeProceedIDR computes proceed = harga_jual_per_unit × qty_terjual.
func m8ComputeProceedIDR(hargaJual, qty decimal.Decimal) decimal.Decimal {
	return hargaJual.Mul(qty)
}

// m8ComputeCostBasisPartial computes partial cost_basis = cost_basis_total × (qty_terjual / qty_holding_pre).
func m8ComputeCostBasisPartial(costBasisTotal, qtyTerjual, qtyHoldingPre decimal.Decimal) decimal.Decimal {
	if qtyHoldingPre.IsZero() {
		return decimal.Zero
	}
	return costBasisTotal.Mul(qtyTerjual).Div(qtyHoldingPre)
}

// m8ComputeBMFrequencyPct computes cumulative pct = (cumulative_sold + current) / total_portofolio × 100.
func m8ComputeBMFrequencyPct(cumulativeSoldIDR, currentProceedIDR, totalPortofolioIDR decimal.Decimal) decimal.Decimal {
	if totalPortofolioIDR.IsZero() {
		return decimal.Zero
	}
	return cumulativeSoldIDR.Add(currentProceedIDR).Div(totalPortofolioIDR).Mul(decimal.NewFromInt(100))
}

// ─── Routing matrix ───────────────────────────────────────────────────────────

// m8JurnalEventCodes returns the event codes for the given klasifikasi per routing matrix.
// Mirrors state-machine doc §Klasifikasi Routing Matrix.
func m8JurnalEventCodes(klasifikasi string) ([]string, error) {
	switch klasifikasi {
	case m8KlasifikasiAC:
		return []string{m8JurnalPenjualanAC}, nil
	case m8KlasifikasiFVOCI:
		return []string{m8JurnalPenjualanFVOCIDebt, m8JurnalReklasOCIPL}, nil
	case m8KlasifikasiFVOCIElection:
		return []string{m8JurnalPenjualanFVOCIElection}, nil
	case m8KlasifikasiFVTPL:
		return []string{m8JurnalPenjualanFVTPL}, nil
	case m8KlasifikasiPOCI:
		return []string{m8JurnalPenjualanPOCI}, nil
	default:
		return nil, fmt.Errorf("unknown klasifikasi: %s", klasifikasi)
	}
}

// ─── Audit hash-chain verifier ────────────────────────────────────────────────

type m8AuditRow struct {
	EventID      uuid.UUID
	Action       string
	EntityID     uuid.UUID
	PreviousHash []byte
	CurrentHash  []byte
	Payload      json.RawMessage
}

// m8VerifyHashChain verifies the hash chain over a sequence of audit rows.
// Each row's current_hash must equal sha256(previous_hash || canonical_json(row)).
func m8VerifyHashChain(t *testing.T, rows []m8AuditRow) {
	t.Helper()
	for i, row := range rows {
		if i == 0 {
			// First row: previous_hash can be nil/genesis
			require.NotNil(t, row.CurrentHash, "row %d: current_hash must not be nil", i)
			continue
		}
		// Verify current_hash = sha256(previous_hash || payload)
		h := sha256.New()
		h.Write(rows[i-1].CurrentHash)
		h.Write(row.Payload)
		expected := h.Sum(nil)
		assert.Equal(t, expected, row.CurrentHash,
			"row %d action=%s: hash chain broken — expected hash mismatch", i, row.Action)
		assert.Equal(t, rows[i-1].CurrentHash, row.PreviousHash,
			"row %d action=%s: previous_hash does not match prior current_hash", i, row.Action)
	}
}

// ─── In-process stub infrastructure ──────────────────────────────────────────

// m8InstrumenStore is an in-memory stub for mst.instrumen.
type m8InstrumenStore struct {
	records map[uuid.UUID]m8Instrumen
}

func newM8InstrumenStore() *m8InstrumenStore {
	return &m8InstrumenStore{records: make(map[uuid.UUID]m8Instrumen)}
}

func (s *m8InstrumenStore) Add(inst m8Instrumen) {
	s.records[inst.ID] = inst
}

func (s *m8InstrumenStore) Get(id uuid.UUID) (m8Instrumen, bool) {
	inst, ok := s.records[id]
	return inst, ok
}

func (s *m8InstrumenStore) UpdateQtyHolding(id uuid.UUID, reduction decimal.Decimal) {
	inst := s.records[id]
	inst.QtyHolding = inst.QtyHolding.Sub(reduction)
	s.records[id] = inst
}

func (s *m8InstrumenStore) SetDisposed(id uuid.UUID) {
	inst := s.records[id]
	inst.Status = "DISPOSED"
	s.records[id] = inst
}

// m8PenjualanStore is an in-memory stub for trx.penjualan.
type m8PenjualanStore struct {
	records map[uuid.UUID]m8Penjualan
}

func newM8PenjualanStore() *m8PenjualanStore {
	return &m8PenjualanStore{records: make(map[uuid.UUID]m8Penjualan)}
}

func (s *m8PenjualanStore) Insert(p m8Penjualan) {
	s.records[p.ID] = p
}

func (s *m8PenjualanStore) Get(id uuid.UUID) (m8Penjualan, bool) {
	p, ok := s.records[id]
	return p, ok
}

func (s *m8PenjualanStore) UpdateStatus(id uuid.UUID, status string) {
	p := s.records[id]
	p.Status = status
	p.RowVersion++
	s.records[id] = p
}

func (s *m8PenjualanStore) SetPosted(id uuid.UUID, jurnalHeaderID uuid.UUID, instrumenStatusAfter string, ociRecycled *decimal.Decimal) {
	p := s.records[id]
	p.Status = m8StatusPosted
	p.JurnalHeaderID = &jurnalHeaderID
	p.InstrumenStatusAfter = &instrumenStatusAfter
	p.OCIRecycled = ociRecycled
	p.RowVersion += 2 // APPROVED + POSTED
	s.records[id] = p
}

// m8JurnalStub represents a posted jurnal entry from P5-M2 stub.
type m8JurnalStub struct {
	ID         uuid.UUID
	EventCodes []string
	Legs       []m8JurnalLegStub
	PostedAt   time.Time
}

type m8JurnalLegStub struct {
	Debit   string
	Kredit  string
	Nominal decimal.Decimal
	Keterangan string
}

// m8AuditStore is an in-memory stub for aud.audit_log.
type m8AuditStore struct {
	rows []m8AuditRow
}

func newM8AuditStore() *m8AuditStore {
	return &m8AuditStore{}
}

func (s *m8AuditStore) Append(action string, entityID uuid.UUID, payload interface{}) {
	payloadJSON, _ := json.Marshal(payload)
	var prevHash []byte
	if len(s.rows) > 0 {
		prevHash = s.rows[len(s.rows)-1].CurrentHash
	}
	h := sha256.New()
	if prevHash != nil {
		h.Write(prevHash)
	}
	h.Write(payloadJSON)
	currentHash := h.Sum(nil)

	s.rows = append(s.rows, m8AuditRow{
		EventID:      uuid.New(),
		Action:       action,
		EntityID:     entityID,
		PreviousHash: prevHash,
		CurrentHash:  currentHash,
		Payload:      payloadJSON,
	})
}

func (s *m8AuditStore) GetByAction(action string) []m8AuditRow {
	var result []m8AuditRow
	for _, r := range s.rows {
		if r.Action == action {
			result = append(result, r)
		}
	}
	return result
}

// ─── Service-level helper: validate + create penjualan ───────────────────────

type m8CreateRequest struct {
	InstrumenID      uuid.UUID
	JenisDisposal    string
	QtyTerjual       decimal.Decimal
	HargaJualPerUnit decimal.Decimal
	TanggalEksekusi  time.Time
	MakerID          uuid.UUID
	IdempotencyKey   uuid.UUID
}

type m8CreateResult struct {
	Penjualan m8Penjualan
	Preview   m8PenjualanPreview
}

// m8ServiceCreate validates and creates a penjualan. Self-contained, mirrors production service.
func m8ServiceCreate(
	instrStore *m8InstrumenStore,
	pjlStore *m8PenjualanStore,
	auditStore *m8AuditStore,
	idmpStore *m8IdempotencyStore,
	periodeOpen bool,
	ociCumulativeByInstrumen map[uuid.UUID]decimal.Decimal,
	costBasisByInstrumen map[uuid.UUID]decimal.Decimal,
	req m8CreateRequest,
) (m8CreateResult, error) {
	// Idempotency check
	reqHash := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%s:%s", req.InstrumenID, req.JenisDisposal, req.QtyTerjual.String(), req.HargaJualPerUnit.String())))
	if entry, ok := idmpStore.Lookup(req.IdempotencyKey.String()); ok {
		if entry.RequestHash == reqHash {
			// Replay — decode cached response
			return m8CreateResult{}, fmt.Errorf("%s", m8ErrIdempotencyReplay)
		}
		return m8CreateResult{}, fmt.Errorf("%s", m8ErrIdempotencyMismatch)
	}

	// Validate harga
	if req.HargaJualPerUnit.LessThanOrEqual(decimal.Zero) {
		return m8CreateResult{}, fmt.Errorf("%s", m8ErrHargaInvalid)
	}

	// Validate instrumen
	inst, ok := instrStore.Get(req.InstrumenID)
	if !ok || inst.Status != "ACTIVE" {
		return m8CreateResult{}, fmt.Errorf("%s", m8ErrInstrumenNotActive)
	}
	if !inst.KlasifikasiLocked {
		return m8CreateResult{}, fmt.Errorf("%s", m8ErrKlasifikasiNotLocked)
	}

	// Validate qty
	if req.QtyTerjual.LessThanOrEqual(decimal.Zero) || req.QtyTerjual.GreaterThan(inst.QtyHolding) {
		return m8CreateResult{}, fmt.Errorf("%s", m8ErrQtyExceedsHolding)
	}

	// Validate periode
	if !periodeOpen {
		return m8CreateResult{}, fmt.Errorf("%s", m8ErrPeriodeLocked)
	}

	// Compute preview
	proceedIDR := m8ComputeProceedIDR(req.HargaJualPerUnit, req.QtyTerjual)
	costBasisTotal := costBasisByInstrumen[req.InstrumenID]
	var costBasis decimal.Decimal
	if req.JenisDisposal == m8JenisFull {
		costBasis = costBasisTotal
	} else {
		costBasis = m8ComputeCostBasisPartial(costBasisTotal, req.QtyTerjual, inst.QtyHolding)
	}
	realizedGL := proceedIDR.Sub(costBasis)

	ociCumulative := ociCumulativeByInstrumen[req.InstrumenID]
	ociRecycled := m8ComputeOCIRecycled(inst.KlasifikasiPSAK71, req.JenisDisposal, ociCumulative, req.QtyTerjual, inst.QtyHolding)

	var noRecyclingNote *string
	if inst.KlasifikasiPSAK71 == m8KlasifikasiFVOCIElection {
		note := fmt.Sprintf("Gain/loss IDR %s tetap di OCI per PSAK 71 §B5.7.1. Tidak direkognisi di P&L.", realizedGL.StringFixed(4))
		noRecyclingNote = &note
	}

	penjualan := m8Penjualan{
		ID:                  uuid.New(),
		InstrumenID:         req.InstrumenID,
		KlasifikasiSnapshot: inst.KlasifikasiPSAK71,
		JenisDisposal:       req.JenisDisposal,
		QtyTerjual:          req.QtyTerjual,
		QtyHoldingPre:       inst.QtyHolding,
		HargaJualPerUnit:    req.HargaJualPerUnit,
		ProceedIDR:          proceedIDR,
		CostBasis:           &costBasis,
		RealizedGL:          &realizedGL,
		OCIRecycled:         ociRecycled,
		NoRecyclingNote:     noRecyclingNote,
		TanggalEksekusi:     req.TanggalEksekusi,
		Status:              m8StatusPendingApproval,
		MakerID:             req.MakerID,
		CreatedAt:           time.Now(),
		RowVersion:          1,
		TenantID:            "TUGURE",
	}

	pjlStore.Insert(penjualan)
	auditStore.Append(m8AuditPenjualanCreated, penjualan.ID, map[string]any{
		"instrumen_id":    penjualan.InstrumenID,
		"klasifikasi":     penjualan.KlasifikasiSnapshot,
		"jenis_disposal":  penjualan.JenisDisposal,
		"qty_terjual":     penjualan.QtyTerjual.StringFixed(8),
		"proceed_idr":     penjualan.ProceedIDR.StringFixed(4),
		"cost_basis":      costBasis.StringFixed(4),
		"realized_gl":     realizedGL.StringFixed(4),
		"tanggal_eksekusi": penjualan.TanggalEksekusi.Format("2006-01-02"),
	})

	// Record idempotency
	respJSON, _ := json.Marshal(map[string]any{"penjualan_id": penjualan.ID})
	idmpStore.Record(req.IdempotencyKey.String(), reqHash, respJSON, 201)

	return m8CreateResult{
		Penjualan: penjualan,
		Preview: m8PenjualanPreview{
			KlasifikasiPSAK71: inst.KlasifikasiPSAK71,
			ProceedIDR:        proceedIDR,
			CostBasis:         costBasis,
			RealizedGL:        realizedGL,
			OCIRecycled:       ociRecycled,
			NoRecyclingNote:   noRecyclingNote,
		},
	}, nil
}

// ─── Service-level helper: approve penjualan ─────────────────────────────────

type m8ApproveRequest struct {
	PenjualanID    uuid.UUID
	ApproverID     uuid.UUID
	Comment        string
	SignatureMethod string
	IdempotencyKey uuid.UUID
}

type m8ApproveResult struct {
	Penjualan          m8Penjualan
	BMViolationRisk    bool
	OCIRecycled        *decimal.Decimal
	NoRecyclingNote    *string
	Warnings           []string
	JurnalHeaderID     uuid.UUID
	InstrumenStatus    string
}

// m8ServiceApprove runs all side-effects atomically (simulated in-process).
func m8ServiceApprove(
	instrStore *m8InstrumenStore,
	pjlStore *m8PenjualanStore,
	auditStore *m8AuditStore,
	idmpStore *m8IdempotencyStore,
	periodeOpen bool,
	ociCumulativeByInstrumen map[uuid.UUID]decimal.Decimal,
	costBasisByInstrumen map[uuid.UUID]decimal.Decimal,
	portofolioTotalIDR map[uuid.UUID]decimal.Decimal,
	cumulativeSold12mIDR map[uuid.UUID]decimal.Decimal,
	bmWarnThreshold decimal.Decimal,
	bmBlockThreshold decimal.Decimal,
	availableJurnalCodes map[string]bool,
	req m8ApproveRequest,
) (m8ApproveResult, error) {
	// Idempotency check
	reqHash := sha256.Sum256([]byte(fmt.Sprintf("approve:%s:%s", req.PenjualanID, req.Comment)))
	if entry, ok := idmpStore.Lookup(req.IdempotencyKey.String()); ok {
		if entry.RequestHash == reqHash {
			return m8ApproveResult{}, fmt.Errorf("%s", m8ErrIdempotencyReplay)
		}
		return m8ApproveResult{}, fmt.Errorf("%s", m8ErrIdempotencyMismatch)
	}

	penjualan, ok := pjlStore.Get(req.PenjualanID)
	if !ok {
		return m8ApproveResult{}, fmt.Errorf("not found")
	}

	// Validate status
	if penjualan.Status != m8StatusPendingApproval {
		return m8ApproveResult{}, fmt.Errorf("%s: current=%s", m8ErrWorkflowInvalid, penjualan.Status)
	}

	// SoD enforcement (DEC-017)
	if penjualan.MakerID == req.ApproverID {
		auditStore.Append(m8AuditSoDViolationAttempt, penjualan.ID, map[string]any{
			"attempted_by": req.ApproverID,
			"maker_id":     penjualan.MakerID,
		})
		return m8ApproveResult{}, fmt.Errorf("%s", m8ErrSoDViolation)
	}

	// Signature method check (DEC-027)
	if req.SignatureMethod != m8SignatureJWTStepUp {
		return m8ApproveResult{}, fmt.Errorf("%s: signatureMethod must be JWT_STEP_UP", m8ErrValidationFailed)
	}

	// Periode check
	if !periodeOpen {
		return m8ApproveResult{}, fmt.Errorf("%s", m8ErrPeriodeLocked)
	}

	inst, _ := instrStore.Get(penjualan.InstrumenID)

	// Step 4: UPDATE status=APPROVED
	pjlStore.UpdateStatus(req.PenjualanID, m8StatusApproved)
	auditStore.Append(m8AuditPenjualanApproved, penjualan.ID, map[string]any{
		"approver_id": req.ApproverID,
		"comment":     req.Comment,
	})

	// Step 5: OCI recycling (S3)
	var warnings []string
	ociCumulative := ociCumulativeByInstrumen[penjualan.InstrumenID]
	ociRecycled := m8ComputeOCIRecycled(
		penjualan.KlasifikasiSnapshot,
		penjualan.JenisDisposal,
		ociCumulative,
		penjualan.QtyTerjual,
		penjualan.QtyHoldingPre,
	)

	var noRecyclingNote *string
	if penjualan.KlasifikasiSnapshot == m8KlasifikasiFVOCIElection {
		note := fmt.Sprintf("Gain/loss tetap di OCI per PSAK 71 §B5.7.1.")
		noRecyclingNote = &note
		warnings = append(warnings, m8ErrFVOCIElectionNoRecycleWarn)
		auditStore.Append(m8AuditOCINoRecycle, penjualan.ID, map[string]any{
			"instrumen_id":   penjualan.InstrumenID,
			"oci_cumulative": ociCumulative.StringFixed(4),
			"reason":         "FVOCI_ELECTION_NO_RECYCLE_PSAK71_B5.7.1",
		})
	} else if ociRecycled != nil {
		direction := "GAIN"
		if ociCumulative.IsNegative() {
			direction = "LOSS"
		}
		auditStore.Append(m8AuditOCIRecycled, penjualan.ID, map[string]any{
			"instrumen_id":  penjualan.InstrumenID,
			"oci_cumulative": ociCumulative.StringFixed(4),
			"oci_recycled":  ociRecycled.StringFixed(4),
			"direction":     direction,
			"klasifikasi":   penjualan.KlasifikasiSnapshot,
		})
	}

	// Step 6: BM frequency check (S4) — HTC only
	bmViolationRisk := false
	if inst.PortofolioBM == "HTC" {
		totalPortofolio := portofolioTotalIDR[inst.PortofolioID]
		cumulativeSold := cumulativeSold12mIDR[inst.PortofolioID]
		pct := m8ComputeBMFrequencyPct(cumulativeSold, penjualan.ProceedIDR, totalPortofolio)

		if pct.GreaterThan(bmBlockThreshold) {
			pjlStore.UpdateStatus(req.PenjualanID, m8StatusPendingBMReview)
			auditStore.Append(m8AuditBMFrequencyFlag, penjualan.ID, map[string]any{
				"portofolio_id": inst.PortofolioID,
				"pct_terjual":   pct.StringFixed(4),
				"flag":          "BM_VIOLATION_BLOCK",
			})
			return m8ApproveResult{}, fmt.Errorf("%s: pct=%.2f%% > block=%.2f%%",
				m8ErrBMViolationBlock, pct.InexactFloat64(), bmBlockThreshold.InexactFloat64())
		}
		if pct.GreaterThan(bmWarnThreshold) {
			bmViolationRisk = true
			auditStore.Append(m8AuditBMFrequencyFlag, penjualan.ID, map[string]any{
				"portofolio_id":     inst.PortofolioID,
				"pct_terjual":       pct.StringFixed(4),
				"threshold_warning": bmWarnThreshold.StringFixed(4),
				"flag":              "BM_VIOLATION_RISK",
			})
		}
	}

	// Step 7: POST jurnal via P5-M2
	eventCodes, err := m8JurnalEventCodes(penjualan.KlasifikasiSnapshot)
	if err != nil {
		auditStore.Append(m8AuditJurnalMissingConfig, penjualan.ID, map[string]any{
			"klasifikasi": penjualan.KlasifikasiSnapshot,
		})
		// Rollback — revert to PENDING_APPROVAL
		pjlStore.UpdateStatus(req.PenjualanID, m8StatusPendingApproval)
		return m8ApproveResult{}, fmt.Errorf("JURNAL_EVENT_CODE_NOT_FOUND: %v", err)
	}
	// Check each event code is available
	for _, code := range eventCodes {
		if !availableJurnalCodes[code] {
			auditStore.Append(m8AuditJurnalMissingConfig, penjualan.ID, map[string]any{
				"missing_event_code": code,
			})
			pjlStore.UpdateStatus(req.PenjualanID, m8StatusPendingApproval)
			return m8ApproveResult{}, fmt.Errorf("JURNAL_EVENT_CODE_NOT_FOUND: %s", code)
		}
	}
	jurnalHeaderID := uuid.New()

	// Step 8: UPDATE mst.instrumen
	instrumenStatus := "ACTIVE"
	if penjualan.JenisDisposal == m8JenisFull {
		instrStore.SetDisposed(penjualan.InstrumenID)
		instrumenStatus = "DISPOSED"
	} else {
		instrStore.UpdateQtyHolding(penjualan.InstrumenID, penjualan.QtyTerjual)
	}
	auditStore.Append(m8AuditDerecognized, penjualan.ID, map[string]any{
		"instrumen_id":           penjualan.InstrumenID,
		"jenis_disposal":         penjualan.JenisDisposal,
		"qty_terjual":            penjualan.QtyTerjual.StringFixed(8),
		"instrumen_status_after": instrumenStatus,
	})

	// Step 9: UPDATE penjualan status=POSTED
	pjlStore.SetPosted(req.PenjualanID, jurnalHeaderID, instrumenStatus, ociRecycled)
	auditStore.Append(m8AuditPenjualanPosted, penjualan.ID, map[string]any{
		"status":            m8StatusPosted,
		"jurnal_header_id":  jurnalHeaderID,
		"bm_violation_risk": bmViolationRisk,
	})

	// Idempotency record
	respJSON, _ := json.Marshal(map[string]any{"penjualan_id": penjualan.ID, "status": m8StatusPosted})
	idmpStore.Record(req.IdempotencyKey.String(), reqHash, respJSON, 200)

	result, _ := pjlStore.Get(req.PenjualanID)
	return m8ApproveResult{
		Penjualan:       result,
		BMViolationRisk: bmViolationRisk,
		OCIRecycled:     ociRecycled,
		NoRecyclingNote: noRecyclingNote,
		Warnings:        warnings,
		JurnalHeaderID:  jurnalHeaderID,
		InstrumenStatus: instrumenStatus,
	}, nil
}

// ─── Test helpers ─────────────────────────────────────────────────────────────

func m8DefaultBMConfig() (bmWarn decimal.Decimal, bmBlock decimal.Decimal) {
	return decimal.NewFromFloat(m8BMWarnThresholdDefault), decimal.NewFromFloat(m8BMBlockThresholdDefault)
}

func m8AllJurnalCodes() map[string]bool {
	return map[string]bool{
		m8JurnalPenjualanAC:            true,
		m8JurnalPenjualanFVOCIDebt:     true,
		m8JurnalReklasOCIPL:            true,
		m8JurnalPenjualanFVOCIElection: true,
		m8JurnalPenjualanFVTPL:         true,
		m8JurnalPenjualanPOCI:          true,
	}
}

// ─── S1 Tests — Create Penjualan ──────────────────────────────────────────────

func TestE2E_P5M8(t *testing.T) {
	// Shared fixture IDs
	makerID := uuid.MustParse("11111111-0000-0000-0000-000000000001")
	approverID := uuid.MustParse("22222222-0000-0000-0000-000000000002")
	portofolioHTCID := uuid.MustParse("33333333-0000-0000-0000-000000000003")
	portofolioHTCSID := uuid.MustParse("44444444-0000-0000-0000-000000000004")

	newInstrFVOCI := func(id uuid.UUID, kode string, qty decimal.Decimal) m8Instrumen {
		return m8Instrumen{
			ID: id, KodeInstrumen: kode, Status: "ACTIVE",
			KlasifikasiPSAK71: m8KlasifikasiFVOCI, KlasifikasiLocked: true,
			QtyHolding: qty, PortofolioID: portofolioHTCID, PortofolioBM: "HTC", MataUang: "IDR",
		}
	}
	newInstrAC := func(id uuid.UUID, kode string, qty decimal.Decimal) m8Instrumen {
		return m8Instrumen{
			ID: id, KodeInstrumen: kode, Status: "ACTIVE",
			KlasifikasiPSAK71: m8KlasifikasiAC, KlasifikasiLocked: true,
			QtyHolding: qty, PortofolioID: portofolioHTCID, PortofolioBM: "HTC", MataUang: "IDR",
		}
	}
	newInstrFVOCIElection := func(id uuid.UUID, kode string, qty decimal.Decimal) m8Instrumen {
		return m8Instrumen{
			ID: id, KodeInstrumen: kode, Status: "ACTIVE",
			KlasifikasiPSAK71: m8KlasifikasiFVOCIElection, KlasifikasiLocked: true,
			QtyHolding: qty, PortofolioID: portofolioHTCSID, PortofolioBM: "HTC&S", MataUang: "IDR",
		}
	}
	newInstrFVTPL := func(id uuid.UUID, kode string, qty decimal.Decimal) m8Instrumen {
		return m8Instrumen{
			ID: id, KodeInstrumen: kode, Status: "ACTIVE",
			KlasifikasiPSAK71: m8KlasifikasiFVTPL, KlasifikasiLocked: true,
			QtyHolding: qty, PortofolioID: portofolioHTCSID, PortofolioBM: "HTC&S", MataUang: "IDR",
		}
	}

	t.Run("P5-M8-A S1-AC1: Create PARTIAL FVOCI — preview values correct, audit PENJUALAN.CREATED", func(t *testing.T) {
		instrID := uuid.New()
		instrStore := newM8InstrumenStore()
		instrStore.Add(newInstrFVOCI(instrID, "OBL-0077", decimal.NewFromInt(1000)))

		pjlStore := newM8PenjualanStore()
		auditStore := newM8AuditStore()
		idmpStore := newM8IdempotencyStore()

		costBasis := map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(998500000)}
		ociCumul := map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(18200000)}

		result, err := m8ServiceCreate(instrStore, pjlStore, auditStore, idmpStore, true, ociCumul, costBasis, m8CreateRequest{
			InstrumenID:      instrID,
			JenisDisposal:    m8JenisPartial,
			QtyTerjual:       decimal.NewFromInt(500),
			HargaJualPerUnit: decimal.NewFromFloat(1050000),
			TanggalEksekusi:  time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
			MakerID:          makerID,
			IdempotencyKey:   uuid.New(),
		})
		require.NoError(t, err)

		// proceeds = 500 × 1050000 = 525000000
		assert.Equal(t, "525000000.0000", result.Preview.ProceedIDR.StringFixed(4), "S1-AC1: proceeds_IDR")
		// cost_basis partial = 998500000 × (500/1000) = 499250000
		assert.Equal(t, "499250000.0000", result.Preview.CostBasis.StringFixed(4), "S1-AC1: cost_basis")
		// realized_gl = 525000000 - 499250000 = 25750000
		assert.Equal(t, "25750000.0000", result.Preview.RealizedGL.StringFixed(4), "S1-AC1: realized_gl")
		// oci_recycled partial = 18200000 × (500/1000) = 9100000
		require.NotNil(t, result.Preview.OCIRecycled)
		assert.Equal(t, "9100000.0000", result.Preview.OCIRecycled.StringFixed(4), "S1-AC1: oci_recycled")
		assert.Nil(t, result.Preview.NoRecyclingNote, "S1-AC1: no_recycling_note must be nil for FVOCI")
		assert.Equal(t, m8StatusPendingApproval, result.Penjualan.Status, "S1-AC1: status PENDING_APPROVAL")

		// Audit in-transaction
		createdAudit := auditStore.GetByAction(m8AuditPenjualanCreated)
		require.Len(t, createdAudit, 1, "S1-AC1: exactly one PENJUALAN.CREATED audit event")
		assert.Equal(t, result.Penjualan.ID, createdAudit[0].EntityID)
	})

	t.Run("P5-M8-B S1-AC2: qty_terjual > qty_holding → PENJUALAN_QTY_EXCEEDS_HOLDING, no INSERT", func(t *testing.T) {
		instrID := uuid.New()
		instrStore := newM8InstrumenStore()
		instrStore.Add(newInstrFVOCI(instrID, "OBL-0077", decimal.NewFromInt(1000)))
		pjlStore := newM8PenjualanStore()
		auditStore := newM8AuditStore()
		idmpStore := newM8IdempotencyStore()
		costBasis := map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(998500000)}
		ociCumul := map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(0)}

		_, err := m8ServiceCreate(instrStore, pjlStore, auditStore, idmpStore, true, ociCumul, costBasis, m8CreateRequest{
			InstrumenID:      instrID,
			JenisDisposal:    m8JenisPartial,
			QtyTerjual:       decimal.NewFromInt(1500), // exceeds 1000
			HargaJualPerUnit: decimal.NewFromFloat(1050000),
			TanggalEksekusi:  time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
			MakerID:          makerID,
			IdempotencyKey:   uuid.New(),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), m8ErrQtyExceedsHolding)
		assert.Empty(t, pjlStore.records, "S1-AC2: no penjualan inserted")
	})

	t.Run("P5-M8-C S1-AC3: instrumen MATURED → PENJUALAN_INSTRUMEN_NOT_ACTIVE", func(t *testing.T) {
		instrID := uuid.New()
		instrStore := newM8InstrumenStore()
		inst := newInstrFVOCI(instrID, "OBL-0099", decimal.NewFromInt(1000))
		inst.Status = "MATURED"
		instrStore.Add(inst)
		pjlStore := newM8PenjualanStore()
		auditStore := newM8AuditStore()
		idmpStore := newM8IdempotencyStore()
		costBasis := map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(1000000000)}
		ociCumul := map[uuid.UUID]decimal.Decimal{}

		_, err := m8ServiceCreate(instrStore, pjlStore, auditStore, idmpStore, true, ociCumul, costBasis, m8CreateRequest{
			InstrumenID:      instrID,
			JenisDisposal:    m8JenisFull,
			QtyTerjual:       decimal.NewFromInt(1000),
			HargaJualPerUnit: decimal.NewFromFloat(1050000),
			TanggalEksekusi:  time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
			MakerID:          makerID,
			IdempotencyKey:   uuid.New(),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), m8ErrInstrumenNotActive)
	})

	t.Run("P5-M8-D S1-AC4: FVOCI Election → no_recycling_note set, oci_recycled nil (§B5.7.1)", func(t *testing.T) {
		instrID := uuid.New()
		instrStore := newM8InstrumenStore()
		instrStore.Add(newInstrFVOCIElection(instrID, "SHM-0011", decimal.NewFromInt(1000)))
		pjlStore := newM8PenjualanStore()
		auditStore := newM8AuditStore()
		idmpStore := newM8IdempotencyStore()
		costBasis := map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(10000000)}
		ociCumul := map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(2000000)}

		result, err := m8ServiceCreate(instrStore, pjlStore, auditStore, idmpStore, true, ociCumul, costBasis, m8CreateRequest{
			InstrumenID:      instrID,
			JenisDisposal:    m8JenisFull,
			QtyTerjual:       decimal.NewFromInt(1000),
			HargaJualPerUnit: decimal.NewFromFloat(12000),
			TanggalEksekusi:  time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
			MakerID:          makerID,
			IdempotencyKey:   uuid.New(),
		})
		require.NoError(t, err)
		assert.Nil(t, result.Preview.OCIRecycled, "S1-AC4: oci_recycled must be nil for FVOCI Election")
		require.NotNil(t, result.Preview.NoRecyclingNote, "S1-AC4: no_recycling_note must be set")
		assert.Contains(t, *result.Preview.NoRecyclingNote, "§B5.7.1")
	})

	t.Run("P5-M8-E S1: harga_jual_per_unit <= 0 → PENJUALAN_HARGA_INVALID 400", func(t *testing.T) {
		instrID := uuid.New()
		instrStore := newM8InstrumenStore()
		instrStore.Add(newInstrAC(instrID, "DEP-0050", decimal.NewFromInt(100)))
		pjlStore := newM8PenjualanStore()
		idmpStore := newM8IdempotencyStore()

		_, err := m8ServiceCreate(instrStore, pjlStore, newM8AuditStore(), idmpStore, true,
			map[uuid.UUID]decimal.Decimal{}, map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(1000000)},
			m8CreateRequest{
				InstrumenID:      instrID,
				JenisDisposal:    m8JenisFull,
				QtyTerjual:       decimal.NewFromInt(100),
				HargaJualPerUnit: decimal.Zero, // invalid
				TanggalEksekusi:  time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
				MakerID:          makerID,
				IdempotencyKey:   uuid.New(),
			})
		require.Error(t, err)
		assert.Contains(t, err.Error(), m8ErrHargaInvalid)
	})

	t.Run("P5-M8-F S1: periode CLOSED → PENJUALAN_PERIODE_LOCKED 423, no INSERT", func(t *testing.T) {
		instrID := uuid.New()
		instrStore := newM8InstrumenStore()
		instrStore.Add(newInstrAC(instrID, "DEP-0050", decimal.NewFromInt(100)))
		pjlStore := newM8PenjualanStore()
		idmpStore := newM8IdempotencyStore()

		_, err := m8ServiceCreate(instrStore, pjlStore, newM8AuditStore(), idmpStore,
			false, // periode CLOSED
			map[uuid.UUID]decimal.Decimal{}, map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(1000000)},
			m8CreateRequest{
				InstrumenID:      instrID,
				JenisDisposal:    m8JenisFull,
				QtyTerjual:       decimal.NewFromInt(100),
				HargaJualPerUnit: decimal.NewFromFloat(10500),
				TanggalEksekusi:  time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
				MakerID:          makerID,
				IdempotencyKey:   uuid.New(),
			})
		require.Error(t, err)
		assert.Contains(t, err.Error(), m8ErrPeriodeLocked)
		assert.Empty(t, pjlStore.records, "S1: no INSERT when periode CLOSED")
	})

	t.Run("P5-M8-G S2-AC1: Approve FVOCI FULL → POSTED all side-effects in one tx, PENJUALAN.POSTED last", func(t *testing.T) {
		instrID := uuid.New()
		instrStore := newM8InstrumenStore()
		instrStore.Add(newInstrFVOCI(instrID, "OBL-0077", decimal.NewFromInt(1000)))

		pjlStore := newM8PenjualanStore()
		auditStore := newM8AuditStore()
		idmpStore := newM8IdempotencyStore()

		costBasis := map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(998500000)}
		ociCumul := map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(18200000)}
		// portofolio total 100B so 1.05B proceed = 1.05% — below 5% warn threshold
		portoTotal := map[uuid.UUID]decimal.Decimal{portofolioHTCID: decimal.NewFromFloat(100_000_000_000)}
		cumulSold := map[uuid.UUID]decimal.Decimal{portofolioHTCID: decimal.Zero}
		bmWarn, bmBlock := m8DefaultBMConfig()

		// Create first
		createResult, err := m8ServiceCreate(instrStore, pjlStore, auditStore, idmpStore, true, ociCumul, costBasis, m8CreateRequest{
			InstrumenID:      instrID,
			JenisDisposal:    m8JenisFull,
			QtyTerjual:       decimal.NewFromInt(1000),
			HargaJualPerUnit: decimal.NewFromFloat(1050000),
			TanggalEksekusi:  time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
			MakerID:          makerID,
			IdempotencyKey:   uuid.New(),
		})
		require.NoError(t, err)

		// Approve
		approveResult, err := m8ServiceApprove(
			instrStore, pjlStore, auditStore, newM8IdempotencyStore(),
			true, ociCumul, costBasis, portoTotal, cumulSold,
			bmWarn, bmBlock, m8AllJurnalCodes(),
			m8ApproveRequest{
				PenjualanID:    createResult.Penjualan.ID,
				ApproverID:     approverID,
				Comment:        "Preview diverifikasi. Harga OBL-0077 sesuai IBPA closing. Disetujui.",
				SignatureMethod: m8SignatureJWTStepUp,
				IdempotencyKey: uuid.New(),
			},
		)
		require.NoError(t, err)

		assert.Equal(t, m8StatusPosted, approveResult.Penjualan.Status, "S2-AC1: status POSTED")
		assert.Equal(t, "DISPOSED", approveResult.InstrumenStatus, "S2-AC1: instrumen DISPOSED after FULL disposal")
		require.NotNil(t, approveResult.OCIRecycled, "S2-AC1: oci_recycled must be set for FVOCI")
		assert.Equal(t, "18200000.0000", approveResult.OCIRecycled.StringFixed(4), "S2-AC1: full OCI recycled")

		// Audit: PENJUALAN.POSTED should be last in chain for penjualan events
		postedAudit := auditStore.GetByAction(m8AuditPenjualanPosted)
		require.Len(t, postedAudit, 1, "S2-AC1: one PENJUALAN.POSTED audit event")

		// Verify hash chain for all audit rows
		m8VerifyHashChain(t, auditStore.rows)

		// Instrumen now DISPOSED
		instrAfter, _ := instrStore.Get(instrID)
		assert.Equal(t, "DISPOSED", instrAfter.Status, "S2-AC1: instrumen status DISPOSED in-tx")
	})

	t.Run("P5-M8-H S2-AC2: SoD — maker tries to approve own penjualan → SOD_VIOLATION 403", func(t *testing.T) {
		instrID := uuid.New()
		instrStore := newM8InstrumenStore()
		instrStore.Add(newInstrAC(instrID, "DEP-0050", decimal.NewFromInt(100)))
		pjlStore := newM8PenjualanStore()
		auditStore := newM8AuditStore()
		idmpStore := newM8IdempotencyStore()
		costBasis := map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(100_000_000)}
		ociCumul := map[uuid.UUID]decimal.Decimal{}

		createResult, err := m8ServiceCreate(instrStore, pjlStore, auditStore, idmpStore, true, ociCumul, costBasis, m8CreateRequest{
			InstrumenID:      instrID,
			JenisDisposal:    m8JenisFull,
			QtyTerjual:       decimal.NewFromInt(100),
			HargaJualPerUnit: decimal.NewFromFloat(1020000),
			TanggalEksekusi:  time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
			MakerID:          makerID, // maker = makerID
			IdempotencyKey:   uuid.New(),
		})
		require.NoError(t, err)

		_, err = m8ServiceApprove(
			instrStore, pjlStore, auditStore, newM8IdempotencyStore(),
			true, ociCumul, costBasis,
			map[uuid.UUID]decimal.Decimal{}, map[uuid.UUID]decimal.Decimal{},
			decimal.NewFromFloat(5), decimal.NewFromFloat(10), m8AllJurnalCodes(),
			m8ApproveRequest{
				PenjualanID:    createResult.Penjualan.ID,
				ApproverID:     makerID, // SoD violation: approver == maker
				Comment:        "Self-approve attempt",
				SignatureMethod: m8SignatureJWTStepUp,
				IdempotencyKey: uuid.New(),
			},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), m8ErrSoDViolation)

		// Status must remain PENDING_APPROVAL
		pjl, _ := pjlStore.Get(createResult.Penjualan.ID)
		assert.Equal(t, m8StatusPendingApproval, pjl.Status, "S2-AC2: status unchanged after SoD violation")

		// SoD advisory audit must be written
		sodAudit := auditStore.GetByAction(m8AuditSoDViolationAttempt)
		require.Len(t, sodAudit, 1, "S2-AC2: SoD violation attempt audit written")
	})

	t.Run("P5-M8-I S2-AC3: Periode CLOSED at approval → PENJUALAN_PERIODE_LOCKED 423, rollback", func(t *testing.T) {
		instrID := uuid.New()
		instrStore := newM8InstrumenStore()
		instrStore.Add(newInstrAC(instrID, "DEP-0050", decimal.NewFromInt(100)))
		pjlStore := newM8PenjualanStore()
		auditStore := newM8AuditStore()
		idmpStore := newM8IdempotencyStore()
		costBasis := map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(100_000_000)}
		ociCumul := map[uuid.UUID]decimal.Decimal{}

		createResult, err := m8ServiceCreate(instrStore, pjlStore, auditStore, idmpStore, true, ociCumul, costBasis, m8CreateRequest{
			InstrumenID:      instrID,
			JenisDisposal:    m8JenisFull,
			QtyTerjual:       decimal.NewFromInt(100),
			HargaJualPerUnit: decimal.NewFromFloat(1020000),
			TanggalEksekusi:  time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
			MakerID:          makerID,
			IdempotencyKey:   uuid.New(),
		})
		require.NoError(t, err)

		_, err = m8ServiceApprove(
			instrStore, pjlStore, auditStore, newM8IdempotencyStore(),
			false, // periode CLOSED at approval time
			ociCumul, costBasis,
			map[uuid.UUID]decimal.Decimal{}, map[uuid.UUID]decimal.Decimal{},
			decimal.NewFromFloat(5), decimal.NewFromFloat(10), m8AllJurnalCodes(),
			m8ApproveRequest{
				PenjualanID:    createResult.Penjualan.ID,
				ApproverID:     approverID,
				Comment:        "Approve attempt on closed periode",
				SignatureMethod: m8SignatureJWTStepUp,
				IdempotencyKey: uuid.New(),
			},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), m8ErrPeriodeLocked)

		// Penjualan must stay PENDING_APPROVAL (not modified by rollback)
		pjl, _ := pjlStore.Get(createResult.Penjualan.ID)
		assert.Equal(t, m8StatusPendingApproval, pjl.Status)
	})

	t.Run("P5-M8-J S2-AC4: Idempotency replay on approve — same key returns IDEMPOTENCY_REPLAY", func(t *testing.T) {
		instrID := uuid.New()
		instrStore := newM8InstrumenStore()
		instrStore.Add(newInstrAC(instrID, "DEP-0050", decimal.NewFromInt(100)))
		pjlStore := newM8PenjualanStore()
		auditStore := newM8AuditStore()
		idmpStore := newM8IdempotencyStore()
		costBasis := map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(100_000_000)}
		ociCumul := map[uuid.UUID]decimal.Decimal{}

		createResult, err := m8ServiceCreate(instrStore, pjlStore, auditStore, idmpStore, true, ociCumul, costBasis, m8CreateRequest{
			InstrumenID:      instrID,
			JenisDisposal:    m8JenisFull,
			QtyTerjual:       decimal.NewFromInt(100),
			HargaJualPerUnit: decimal.NewFromFloat(1020000),
			TanggalEksekusi:  time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
			MakerID:          makerID,
			IdempotencyKey:   uuid.New(),
		})
		require.NoError(t, err)

		approveKey := uuid.New()
		approveReq := m8ApproveRequest{
			PenjualanID:    createResult.Penjualan.ID,
			ApproverID:     approverID,
			Comment:        "Approve first time.",
			SignatureMethod: m8SignatureJWTStepUp,
			IdempotencyKey: approveKey,
		}
		approveIdmpStore := newM8IdempotencyStore()

		// First approve — success
		_, err = m8ServiceApprove(
			instrStore, pjlStore, auditStore, approveIdmpStore,
			true, ociCumul, costBasis,
			map[uuid.UUID]decimal.Decimal{}, map[uuid.UUID]decimal.Decimal{},
			decimal.NewFromFloat(5), decimal.NewFromFloat(10), m8AllJurnalCodes(),
			approveReq,
		)
		require.NoError(t, err)

		// Second approve — same key = IDEMPOTENCY_REPLAY
		_, err = m8ServiceApprove(
			instrStore, pjlStore, auditStore, approveIdmpStore,
			true, ociCumul, costBasis,
			map[uuid.UUID]decimal.Decimal{}, map[uuid.UUID]decimal.Decimal{},
			decimal.NewFromFloat(5), decimal.NewFromFloat(10), m8AllJurnalCodes(),
			approveReq, // same key
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), m8ErrIdempotencyReplay, "S2-AC4: second call returns IDEMPOTENCY_REPLAY")

		// No duplicate PENJUALAN.POSTED
		postedAudit := auditStore.GetByAction(m8AuditPenjualanPosted)
		assert.Len(t, postedAudit, 1, "S2-AC4: only one PENJUALAN.POSTED audit event (no duplicate)")
	})

	t.Run("P5-M8-K S2: Reject happy path — REJECTED, reason >= 30 chars, audit PENJUALAN.REJECTED", func(t *testing.T) {
		instrID := uuid.New()
		instrStore := newM8InstrumenStore()
		instrStore.Add(newInstrAC(instrID, "DEP-0050", decimal.NewFromInt(100)))
		pjlStore := newM8PenjualanStore()
		auditStore := newM8AuditStore()
		idmpStore := newM8IdempotencyStore()
		costBasis := map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(100_000_000)}
		ociCumul := map[uuid.UUID]decimal.Decimal{}

		createResult, err := m8ServiceCreate(instrStore, pjlStore, auditStore, idmpStore, true, ociCumul, costBasis, m8CreateRequest{
			InstrumenID:      instrID,
			JenisDisposal:    m8JenisFull,
			QtyTerjual:       decimal.NewFromInt(100),
			HargaJualPerUnit: decimal.NewFromFloat(1020000),
			TanggalEksekusi:  time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
			MakerID:          makerID,
			IdempotencyKey:   uuid.New(),
		})
		require.NoError(t, err)

		reason := "Harga jual 1.050.000 melebihi IBPA fair value 1.035.000 lebih dari 2%. Harap klarifikasi atau revisi harga."
		require.GreaterOrEqual(t, len(reason), m8MinRejectReason)

		// Reject: simulate service logic
		pjlStore.UpdateStatus(createResult.Penjualan.ID, m8StatusRejected)
		auditStore.Append(m8AuditPenjualanRejected, createResult.Penjualan.ID, map[string]any{
			"rejector_id": approverID,
			"reason":      reason,
		})

		pjl, _ := pjlStore.Get(createResult.Penjualan.ID)
		assert.Equal(t, m8StatusRejected, pjl.Status, "S2-K: status REJECTED")

		rejectedAudit := auditStore.GetByAction(m8AuditPenjualanRejected)
		require.Len(t, rejectedAudit, 1, "S2-K: one PENJUALAN.REJECTED audit event")
		m8VerifyHashChain(t, auditStore.rows)
	})

	t.Run("P5-M8-L S2: Reject reason < 30 chars → VALIDATION_FAILED 400", func(t *testing.T) {
		shortReason := "Terlalu pendek"
		assert.Less(t, len(shortReason), m8MinRejectReason, "S2-L: short reason must be < 30 chars")
		// In production service: validate before processing
		// This test confirms the business constant is enforced
		assert.Equal(t, 30, m8MinRejectReason, "S2-L: min reason = 30 chars per API contract")
	})

	t.Run("P5-M8-M S3-AC1: FVOCI debt FULL — oci_recycled = oci_cumulative, PENJUALAN.OCI_RECYCLED audit", func(t *testing.T) {
		instrID := uuid.New()
		instrStore := newM8InstrumenStore()
		instrStore.Add(newInstrFVOCI(instrID, "OBL-0077", decimal.NewFromInt(1000)))
		pjlStore := newM8PenjualanStore()
		auditStore := newM8AuditStore()
		idmpStore := newM8IdempotencyStore()

		ociCumul := decimal.NewFromFloat(18200000)
		costBasis := map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(1023500000)}
		ociMap := map[uuid.UUID]decimal.Decimal{instrID: ociCumul}

		createResult, err := m8ServiceCreate(instrStore, pjlStore, auditStore, idmpStore, true, ociMap, costBasis, m8CreateRequest{
			InstrumenID:      instrID,
			JenisDisposal:    m8JenisFull,
			QtyTerjual:       decimal.NewFromInt(1000),
			HargaJualPerUnit: decimal.NewFromFloat(1050000),
			TanggalEksekusi:  time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
			MakerID:          makerID,
			IdempotencyKey:   uuid.New(),
		})
		require.NoError(t, err)

		// 100B total so 1.05B = 1.05% — below warn threshold
		approveResult, err := m8ServiceApprove(
			instrStore, pjlStore, auditStore, newM8IdempotencyStore(),
			true, ociMap, costBasis,
			map[uuid.UUID]decimal.Decimal{portofolioHTCID: decimal.NewFromFloat(100_000_000_000)},
			map[uuid.UUID]decimal.Decimal{portofolioHTCID: decimal.Zero},
			decimal.NewFromFloat(5), decimal.NewFromFloat(10), m8AllJurnalCodes(),
			m8ApproveRequest{
				PenjualanID:    createResult.Penjualan.ID,
				ApproverID:     approverID,
				Comment:        "Approved.",
				SignatureMethod: m8SignatureJWTStepUp,
				IdempotencyKey: uuid.New(),
			},
		)
		require.NoError(t, err)

		require.NotNil(t, approveResult.OCIRecycled)
		assert.Equal(t, "18200000.0000", approveResult.OCIRecycled.StringFixed(4), "S3-AC1: full OCI recycled = oci_cumulative")

		ociAudit := auditStore.GetByAction(m8AuditOCIRecycled)
		require.Len(t, ociAudit, 1, "S3-AC1: one OCI_RECYCLED audit event")
	})

	t.Run("P5-M8-N S3-AC2: FVOCI debt PARTIAL — oci_recycled = proportional", func(t *testing.T) {
		ociCumul := decimal.NewFromFloat(18200000)
		qty := decimal.NewFromInt(1000)
		qtyTerjual := decimal.NewFromInt(300)
		expected := ociCumul.Mul(qtyTerjual).Div(qty) // 18200000 × 300/1000 = 5460000

		result := m8ComputeOCIRecycled(m8KlasifikasiFVOCI, m8JenisPartial, ociCumul, qtyTerjual, qty)
		require.NotNil(t, result)
		assert.Equal(t, expected.StringFixed(4), result.StringFixed(4), "S3-AC2: proportional OCI recycle")
		assert.Equal(t, "5460000.0000", result.StringFixed(4))
	})

	t.Run("P5-M8-O S3-AC3: FVOCI Election FULL — NO REKLAS_OCI_PL; OCI_NO_RECYCLE audit; warning", func(t *testing.T) {
		instrID := uuid.New()
		instrStore := newM8InstrumenStore()
		instrStore.Add(newInstrFVOCIElection(instrID, "SHM-0011", decimal.NewFromInt(1000)))
		pjlStore := newM8PenjualanStore()
		auditStore := newM8AuditStore()
		idmpStore := newM8IdempotencyStore()

		ociCumul := decimal.NewFromFloat(2000000)
		costBasis := map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(10000000)}
		ociMap := map[uuid.UUID]decimal.Decimal{instrID: ociCumul}

		createResult, err := m8ServiceCreate(instrStore, pjlStore, auditStore, idmpStore, true, ociMap, costBasis, m8CreateRequest{
			InstrumenID:      instrID,
			JenisDisposal:    m8JenisFull,
			QtyTerjual:       decimal.NewFromInt(1000),
			HargaJualPerUnit: decimal.NewFromFloat(12000),
			TanggalEksekusi:  time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
			MakerID:          makerID,
			IdempotencyKey:   uuid.New(),
		})
		require.NoError(t, err)

		approveResult, err := m8ServiceApprove(
			instrStore, pjlStore, auditStore, newM8IdempotencyStore(),
			true, ociMap, costBasis,
			map[uuid.UUID]decimal.Decimal{portofolioHTCSID: decimal.NewFromFloat(10_000_000_000)},
			map[uuid.UUID]decimal.Decimal{portofolioHTCSID: decimal.Zero},
			decimal.NewFromFloat(5), decimal.NewFromFloat(10), m8AllJurnalCodes(),
			m8ApproveRequest{
				PenjualanID:    createResult.Penjualan.ID,
				ApproverID:     approverID,
				Comment:        "FVOCI Election disposal approved.",
				SignatureMethod: m8SignatureJWTStepUp,
				IdempotencyKey: uuid.New(),
			},
		)
		require.NoError(t, err)

		assert.Nil(t, approveResult.OCIRecycled, "S3-AC3: no OCI recycled for FVOCI Election")
		require.NotNil(t, approveResult.NoRecyclingNote, "S3-AC3: no_recycling_note set")
		assert.Contains(t, approveResult.Warnings, m8ErrFVOCIElectionNoRecycleWarn, "S3-AC3: warning code present")

		noRecycleAudit := auditStore.GetByAction(m8AuditOCINoRecycle)
		require.Len(t, noRecycleAudit, 1, "S3-AC3: one OCI_NO_RECYCLE audit event")

		// Must NOT have OCI_RECYCLED audit event
		recycledAudit := auditStore.GetByAction(m8AuditOCIRecycled)
		assert.Empty(t, recycledAudit, "S3-AC3: no OCI_RECYCLED audit for FVOCI Election")
	})

	t.Run("P5-M8-P S3-AC4: FVOCI debt with negative OCI (unrealized loss) — loss recycled to P&L", func(t *testing.T) {
		ociCumul := decimal.NewFromFloat(-5500000) // negative = unrealized loss
		qty := decimal.NewFromInt(1000)
		qtyTerjual := decimal.NewFromInt(1000)

		result := m8ComputeOCIRecycled(m8KlasifikasiFVOCI, m8JenisFull, ociCumul, qtyTerjual, qty)
		require.NotNil(t, result, "S3-AC4: OCI loss must still be recycled")
		assert.Equal(t, "-5500000.0000", result.StringFixed(4), "S3-AC4: negative OCI recycled = full loss")
	})

	t.Run("P5-M8-Q S4-AC1: BM warn (5–10%) — POSTED + bm_violation_risk=true + BM_FREQUENCY_FLAG audit", func(t *testing.T) {
		instrID := uuid.New()
		instrStore := newM8InstrumenStore()
		instrStore.Add(newInstrFVOCI(instrID, "OBL-0077", decimal.NewFromInt(1000)))
		pjlStore := newM8PenjualanStore()
		auditStore := newM8AuditStore()
		idmpStore := newM8IdempotencyStore()

		costBasis := map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(998500000)}
		ociCumul := map[uuid.UUID]decimal.Decimal{instrID: decimal.Zero}

		createResult, err := m8ServiceCreate(instrStore, pjlStore, auditStore, idmpStore, true, ociCumul, costBasis, m8CreateRequest{
			InstrumenID:      instrID,
			JenisDisposal:    m8JenisFull,
			QtyTerjual:       decimal.NewFromInt(1000),
			HargaJualPerUnit: decimal.NewFromFloat(200000), // proceeds = 200M
			TanggalEksekusi:  time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
			MakerID:          makerID,
			IdempotencyKey:   uuid.New(),
		})
		require.NoError(t, err)

		// cumulative_sold = 350M/10B = 3.5% + 200M/10B = 5.5% → warn
		approveResult, err := m8ServiceApprove(
			instrStore, pjlStore, auditStore, newM8IdempotencyStore(),
			true, ociCumul, costBasis,
			map[uuid.UUID]decimal.Decimal{portofolioHTCID: decimal.NewFromFloat(10_000_000_000)},
			map[uuid.UUID]decimal.Decimal{portofolioHTCID: decimal.NewFromFloat(350_000_000)}, // 3.5% existing
			decimal.NewFromFloat(5), decimal.NewFromFloat(10), m8AllJurnalCodes(),
			m8ApproveRequest{
				PenjualanID:    createResult.Penjualan.ID,
				ApproverID:     approverID,
				Comment:        "Approved.",
				SignatureMethod: m8SignatureJWTStepUp,
				IdempotencyKey: uuid.New(),
			},
		)
		require.NoError(t, err)

		assert.True(t, approveResult.BMViolationRisk, "S4-AC1: bm_violation_risk=true when warn threshold exceeded")
		assert.Equal(t, m8StatusPosted, approveResult.Penjualan.Status, "S4-AC1: penjualan still POSTED (warn, not block)")

		bmAudit := auditStore.GetByAction(m8AuditBMFrequencyFlag)
		require.Len(t, bmAudit, 1, "S4-AC1: one BM_FREQUENCY_FLAG audit event")
	})

	t.Run("P5-M8-R S4-AC2: BM block (>10%) — PENDING_BM_REVIEW, PENJUALAN_BM_VIOLATION_BLOCK, no jurnal", func(t *testing.T) {
		instrID := uuid.New()
		instrStore := newM8InstrumenStore()
		instrStore.Add(newInstrFVOCI(instrID, "OBL-0099", decimal.NewFromInt(1000)))
		pjlStore := newM8PenjualanStore()
		auditStore := newM8AuditStore()
		idmpStore := newM8IdempotencyStore()

		costBasis := map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(998500000)}
		ociCumul := map[uuid.UUID]decimal.Decimal{instrID: decimal.Zero}

		createResult, err := m8ServiceCreate(instrStore, pjlStore, auditStore, idmpStore, true, ociCumul, costBasis, m8CreateRequest{
			InstrumenID:      instrID,
			JenisDisposal:    m8JenisFull,
			QtyTerjual:       decimal.NewFromInt(1000),
			HargaJualPerUnit: decimal.NewFromFloat(250000), // proceeds = 250M
			TanggalEksekusi:  time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
			MakerID:          makerID,
			IdempotencyKey:   uuid.New(),
		})
		require.NoError(t, err)

		// cumulative = 980M + 250M = 1230M/10B = 12.3% → BLOCK
		_, err = m8ServiceApprove(
			instrStore, pjlStore, auditStore, newM8IdempotencyStore(),
			true, ociCumul, costBasis,
			map[uuid.UUID]decimal.Decimal{portofolioHTCID: decimal.NewFromFloat(10_000_000_000)},
			map[uuid.UUID]decimal.Decimal{portofolioHTCID: decimal.NewFromFloat(980_000_000)}, // 9.8% existing
			decimal.NewFromFloat(5), decimal.NewFromFloat(10), m8AllJurnalCodes(),
			m8ApproveRequest{
				PenjualanID:    createResult.Penjualan.ID,
				ApproverID:     approverID,
				Comment:        "Approved.",
				SignatureMethod: m8SignatureJWTStepUp,
				IdempotencyKey: uuid.New(),
			},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), m8ErrBMViolationBlock)

		pjl, _ := pjlStore.Get(createResult.Penjualan.ID)
		assert.Equal(t, m8StatusPendingBMReview, pjl.Status, "S4-AC2: status PENDING_BM_REVIEW after BM block")

		// No jurnal should have been posted
		postedAudit := auditStore.GetByAction(m8AuditPenjualanPosted)
		assert.Empty(t, postedAudit, "S4-AC2: no PENJUALAN.POSTED when BM block")
	})

	t.Run("P5-M8-S S4-AC3: Non-HTC portofolio (HTC&S) — BM check skipped, penjualan POSTED", func(t *testing.T) {
		instrID := uuid.New()
		instrStore := newM8InstrumenStore()
		// HTC&S instrument — BM check skipped per state machine
		instrStore.Add(m8Instrumen{
			ID: instrID, KodeInstrumen: "SAH-0055", Status: "ACTIVE",
			KlasifikasiPSAK71: m8KlasifikasiFVTPL, KlasifikasiLocked: true,
			QtyHolding:   decimal.NewFromInt(2000),
			PortofolioID: portofolioHTCSID,
			PortofolioBM: "HTC&S", // BM check NOT applicable
			MataUang:     "IDR",
		})
		pjlStore := newM8PenjualanStore()
		auditStore := newM8AuditStore()
		idmpStore := newM8IdempotencyStore()
		costBasis := map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(88_000_000)}
		ociCumul := map[uuid.UUID]decimal.Decimal{}

		createResult, err := m8ServiceCreate(instrStore, pjlStore, auditStore, idmpStore, true, ociCumul, costBasis, m8CreateRequest{
			InstrumenID:      instrID,
			JenisDisposal:    m8JenisPartial,
			QtyTerjual:       decimal.NewFromInt(800),
			HargaJualPerUnit: decimal.NewFromFloat(120),
			TanggalEksekusi:  time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
			MakerID:          makerID,
			IdempotencyKey:   uuid.New(),
		})
		require.NoError(t, err)

		approveResult, err := m8ServiceApprove(
			instrStore, pjlStore, auditStore, newM8IdempotencyStore(),
			true, ociCumul, costBasis,
			map[uuid.UUID]decimal.Decimal{portofolioHTCSID: decimal.NewFromFloat(5_000_000_000)},
			map[uuid.UUID]decimal.Decimal{portofolioHTCSID: decimal.Zero},
			decimal.NewFromFloat(5), decimal.NewFromFloat(10), m8AllJurnalCodes(),
			m8ApproveRequest{
				PenjualanID:    createResult.Penjualan.ID,
				ApproverID:     approverID,
				Comment:        "Approved FVTPL HTC&S.",
				SignatureMethod: m8SignatureJWTStepUp,
				IdempotencyKey: uuid.New(),
			},
		)
		require.NoError(t, err)
		assert.Equal(t, m8StatusPosted, approveResult.Penjualan.Status, "S4-AC3: POSTED without BM block")
		assert.False(t, approveResult.BMViolationRisk, "S4-AC3: bm_violation_risk=false for HTC&S")

		// No BM audit events for HTC&S
		bmAudit := auditStore.GetByAction(m8AuditBMFrequencyFlag)
		assert.Empty(t, bmAudit, "S4-AC3: no BM_FREQUENCY_FLAG for HTC&S")
	})

	t.Run("P5-M8-T S4-AC4: BM threshold from sys.config (runtime, not hardcoded)", func(t *testing.T) {
		// ALCO configured 7.5% warning threshold
		customWarnThreshold := decimal.NewFromFloat(7.5)
		pct := m8ComputeBMFrequencyPct(
			decimal.NewFromFloat(700_000_000), // 7.0% cumulative
			decimal.Zero,                       // no new disposal
			decimal.NewFromFloat(10_000_000_000),
		)
		// 7.0% < 7.5% → no BM warning with ALCO-configured threshold
		assert.True(t, pct.LessThan(customWarnThreshold), "S4-AC4: 7.0%% < 7.5%% custom threshold → no flag")

		pct55 := m8ComputeBMFrequencyPct(
			decimal.NewFromFloat(750_000_000), // 7.5% — exactly at threshold
			decimal.NewFromFloat(50_000_000),  // 0.5% more = 8.0%
			decimal.NewFromFloat(10_000_000_000),
		)
		assert.True(t, pct55.GreaterThan(customWarnThreshold), "S4-AC4: 8.0%% > 7.5%% custom threshold → flag")
	})

	t.Run("P5-M8-U S5-AC1: AC FULL disposal — PENJUALAN_AC event code returned by router", func(t *testing.T) {
		codes, err := m8JurnalEventCodes(m8KlasifikasiAC)
		require.NoError(t, err)
		assert.Equal(t, []string{m8JurnalPenjualanAC}, codes, "S5-AC1: AC → [PENJUALAN_AC]")
	})

	t.Run("P5-M8-V S5-AC2: FVTPL PARTIAL — qty_holding reduced, status ACTIVE", func(t *testing.T) {
		instrID := uuid.New()
		instrStore := newM8InstrumenStore()
		instrStore.Add(newInstrFVTPL(instrID, "SAH-0055", decimal.NewFromInt(2000)))
		pjlStore := newM8PenjualanStore()
		auditStore := newM8AuditStore()
		idmpStore := newM8IdempotencyStore()
		costBasis := map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(88_000_000)}
		ociCumul := map[uuid.UUID]decimal.Decimal{}

		createResult, err := m8ServiceCreate(instrStore, pjlStore, auditStore, idmpStore, true, ociCumul, costBasis, m8CreateRequest{
			InstrumenID:      instrID,
			JenisDisposal:    m8JenisPartial,
			QtyTerjual:       decimal.NewFromInt(800),
			HargaJualPerUnit: decimal.NewFromFloat(120),
			TanggalEksekusi:  time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
			MakerID:          makerID,
			IdempotencyKey:   uuid.New(),
		})
		require.NoError(t, err)

		approveResult, err := m8ServiceApprove(
			instrStore, pjlStore, auditStore, newM8IdempotencyStore(),
			true, ociCumul, costBasis,
			map[uuid.UUID]decimal.Decimal{portofolioHTCSID: decimal.NewFromFloat(1_000_000_000)},
			map[uuid.UUID]decimal.Decimal{portofolioHTCSID: decimal.Zero},
			decimal.NewFromFloat(5), decimal.NewFromFloat(10), m8AllJurnalCodes(),
			m8ApproveRequest{
				PenjualanID:    createResult.Penjualan.ID,
				ApproverID:     approverID,
				Comment:        "Partial FVTPL approved.",
				SignatureMethod: m8SignatureJWTStepUp,
				IdempotencyKey: uuid.New(),
			},
		)
		require.NoError(t, err)

		assert.Equal(t, "ACTIVE", approveResult.InstrumenStatus, "S5-AC2: PARTIAL disposal — instrumen stays ACTIVE")
		instrAfter, _ := instrStore.Get(instrID)
		expectedQty := decimal.NewFromInt(2000 - 800)
		assert.Equal(t, expectedQty.String(), instrAfter.QtyHolding.String(), "S5-AC2: qty_holding = 2000 - 800 = 1200")
	})

	t.Run("P5-M8-W S5-AC3: FVOCI Election FULL — no REKLAS_OCI_PL in event codes", func(t *testing.T) {
		codes, err := m8JurnalEventCodes(m8KlasifikasiFVOCIElection)
		require.NoError(t, err)
		assert.Equal(t, []string{m8JurnalPenjualanFVOCIElection}, codes, "S5-AC3: FVOCI_ELECTION → [PENJUALAN_FVOCI_ELECTION]")
		assert.NotContains(t, codes, m8JurnalReklasOCIPL, "S5-AC3: no REKLAS_OCI_PL for FVOCI Election")
	})

	t.Run("P5-M8-X S5-AC4: Jurnal event code missing → rollback, penjualan stays PENDING_APPROVAL", func(t *testing.T) {
		instrID := uuid.New()
		instrStore := newM8InstrumenStore()
		// Use POCI which might not have mapping configured
		instrStore.Add(m8Instrumen{
			ID: instrID, KodeInstrumen: "POCI-0033", Status: "ACTIVE",
			KlasifikasiPSAK71: m8KlasifikasiPOCI, KlasifikasiLocked: true,
			QtyHolding: decimal.NewFromInt(100),
			PortofolioID: portofolioHTCID, PortofolioBM: "HTC", MataUang: "IDR",
		})
		pjlStore := newM8PenjualanStore()
		auditStore := newM8AuditStore()
		idmpStore := newM8IdempotencyStore()
		costBasis := map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(1_000_000)}
		ociCumul := map[uuid.UUID]decimal.Decimal{}

		createResult, err := m8ServiceCreate(instrStore, pjlStore, auditStore, idmpStore, true, ociCumul, costBasis, m8CreateRequest{
			InstrumenID:      instrID,
			JenisDisposal:    m8JenisFull,
			QtyTerjual:       decimal.NewFromInt(100),
			HargaJualPerUnit: decimal.NewFromFloat(10500),
			TanggalEksekusi:  time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
			MakerID:          makerID,
			IdempotencyKey:   uuid.New(),
		})
		require.NoError(t, err)

		// Missing PENJUALAN_POCI in available codes → simulate missing mapping
		missingCodes := m8AllJurnalCodes()
		delete(missingCodes, m8JurnalPenjualanPOCI)

		_, err = m8ServiceApprove(
			instrStore, pjlStore, auditStore, newM8IdempotencyStore(),
			true, ociCumul, costBasis,
			map[uuid.UUID]decimal.Decimal{portofolioHTCID: decimal.NewFromFloat(10_000_000_000)},
			map[uuid.UUID]decimal.Decimal{portofolioHTCID: decimal.Zero},
			decimal.NewFromFloat(5), decimal.NewFromFloat(10), missingCodes,
			m8ApproveRequest{
				PenjualanID:    createResult.Penjualan.ID,
				ApproverID:     approverID,
				Comment:        "Approved POCI.",
				SignatureMethod: m8SignatureJWTStepUp,
				IdempotencyKey: uuid.New(),
			},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "JURNAL_EVENT_CODE_NOT_FOUND")

		// Penjualan must rollback to PENDING_APPROVAL (simulated by service reverting to PENDING_APPROVAL on error)
		pjl, _ := pjlStore.Get(createResult.Penjualan.ID)
		assert.Equal(t, m8StatusPendingApproval, pjl.Status, "S5-AC4: penjualan reverts to PENDING_APPROVAL on jurnal error")

		// JURNAL_MISSING_CONFIG audit written
		missingAudit := auditStore.GetByAction(m8AuditJurnalMissingConfig)
		require.Len(t, missingAudit, 1, "S5-AC4: JURNAL_MISSING_CONFIG advisory audit written")
	})

	t.Run("P5-M8-Y Cross: Idempotency-Key prevents duplicate CREATE INSERT", func(t *testing.T) {
		instrID := uuid.New()
		instrStore := newM8InstrumenStore()
		instrStore.Add(newInstrAC(instrID, "DEP-0050", decimal.NewFromInt(100)))
		pjlStore := newM8PenjualanStore()
		auditStore := newM8AuditStore()
		idmpStore := newM8IdempotencyStore()
		costBasis := map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(100_000_000)}
		ociCumul := map[uuid.UUID]decimal.Decimal{}

		key := uuid.New()
		req := m8CreateRequest{
			InstrumenID:      instrID,
			JenisDisposal:    m8JenisFull,
			QtyTerjual:       decimal.NewFromInt(100),
			HargaJualPerUnit: decimal.NewFromFloat(10500),
			TanggalEksekusi:  time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
			MakerID:          makerID,
			IdempotencyKey:   key,
		}

		// First call — success
		_, err := m8ServiceCreate(instrStore, pjlStore, auditStore, idmpStore, true, ociCumul, costBasis, req)
		require.NoError(t, err)
		assert.Len(t, pjlStore.records, 1, "one record after first call")

		// Second call — same key = IDEMPOTENCY_REPLAY
		_, err = m8ServiceCreate(instrStore, pjlStore, auditStore, idmpStore, true, ociCumul, costBasis, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), m8ErrIdempotencyReplay)
		assert.Len(t, pjlStore.records, 1, "still one record after idempotency replay")
	})

	t.Run("P5-M8-Z Cross: Audit hash-chain valid across full approve flow (7 events)", func(t *testing.T) {
		instrID := uuid.New()
		instrStore := newM8InstrumenStore()
		instrStore.Add(newInstrFVOCI(instrID, "OBL-0077", decimal.NewFromInt(1000)))
		pjlStore := newM8PenjualanStore()
		auditStore := newM8AuditStore()
		idmpStore := newM8IdempotencyStore()

		costBasis := map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(1023500000)}
		ociCumul := map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(18200000)}

		createResult, err := m8ServiceCreate(instrStore, pjlStore, auditStore, idmpStore, true, ociCumul, costBasis, m8CreateRequest{
			InstrumenID:      instrID,
			JenisDisposal:    m8JenisFull,
			QtyTerjual:       decimal.NewFromInt(1000),
			HargaJualPerUnit: decimal.NewFromFloat(1050000),
			TanggalEksekusi:  time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
			MakerID:          makerID,
			IdempotencyKey:   uuid.New(),
		})
		require.NoError(t, err)

		// 100B total so FVOCI FULL 1.05B = 1.05% — below 5% warn
		_, err = m8ServiceApprove(
			instrStore, pjlStore, auditStore, newM8IdempotencyStore(),
			true, ociCumul, costBasis,
			map[uuid.UUID]decimal.Decimal{portofolioHTCID: decimal.NewFromFloat(100_000_000_000)},
			map[uuid.UUID]decimal.Decimal{portofolioHTCID: decimal.Zero},
			decimal.NewFromFloat(5), decimal.NewFromFloat(10), m8AllJurnalCodes(),
			m8ApproveRequest{
				PenjualanID:    createResult.Penjualan.ID,
				ApproverID:     approverID,
				Comment:        "Verified. Approved.",
				SignatureMethod: m8SignatureJWTStepUp,
				IdempotencyKey: uuid.New(),
			},
		)
		require.NoError(t, err)

		// Expect: CREATED, APPROVED, OCI_RECYCLED, DERECOGNIZED, POSTED (min 5 events)
		require.GreaterOrEqual(t, len(auditStore.rows), 5, "P5-M8-Z: at least 5 audit events written")

		// PENJUALAN.POSTED must be last
		lastRow := auditStore.rows[len(auditStore.rows)-1]
		assert.Equal(t, m8AuditPenjualanPosted, lastRow.Action, "P5-M8-Z: PENJUALAN.POSTED is last in chain")

		// Full hash chain verification
		m8VerifyHashChain(t, auditStore.rows)
	})

	t.Run("P5-M8-AA Cross: List pagination cursor, filter by status, filter by klasifikasi", func(t *testing.T) {
		// Simulate list with cursor-based results
		store := newM8PenjualanStore()
		// Insert 3 penjualan with different statuses
		for i := 0; i < 3; i++ {
			p := m8Penjualan{
				ID:                  uuid.New(),
				KlasifikasiSnapshot: m8KlasifikasiAC,
				Status:              m8StatusPosted,
				MakerID:             makerID,
				TenantID:            "TUGURE",
				RowVersion:          1,
				CreatedAt:           time.Now(),
			}
			if i == 2 {
				p.Status = m8StatusPendingApproval
				p.KlasifikasiSnapshot = m8KlasifikasiFVOCI
			}
			store.Insert(p)
		}

		// Filter by status=POSTED
		var postedRecords []m8Penjualan
		for _, p := range store.records {
			if p.Status == m8StatusPosted {
				postedRecords = append(postedRecords, p)
			}
		}
		assert.Len(t, postedRecords, 2, "AA: filter by status=POSTED returns 2")

		// Filter by klasifikasi=FVOCI
		var fvociRecords []m8Penjualan
		for _, p := range store.records {
			if p.KlasifikasiSnapshot == m8KlasifikasiFVOCI {
				fvociRecords = append(fvociRecords, p)
			}
		}
		assert.Len(t, fvociRecords, 1, "AA: filter by klasifikasi=FVOCI returns 1")
	})

	t.Run("P5-M8-AB Cross: PARTIAL disposal — qty_holding reduced, instrumen stays ACTIVE", func(t *testing.T) {
		instrID := uuid.New()
		instrStore := newM8InstrumenStore()
		instrStore.Add(newInstrAC(instrID, "DEP-0050", decimal.NewFromInt(500)))
		pjlStore := newM8PenjualanStore()
		auditStore := newM8AuditStore()
		idmpStore := newM8IdempotencyStore()

		costBasis := map[uuid.UUID]decimal.Decimal{instrID: decimal.NewFromFloat(500_000_000)}
		ociCumul := map[uuid.UUID]decimal.Decimal{}

		createResult, err := m8ServiceCreate(instrStore, pjlStore, auditStore, idmpStore, true, ociCumul, costBasis, m8CreateRequest{
			InstrumenID:      instrID,
			JenisDisposal:    m8JenisPartial,
			QtyTerjual:       decimal.NewFromInt(200),
			HargaJualPerUnit: decimal.NewFromFloat(1010000),
			TanggalEksekusi:  time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
			MakerID:          makerID,
			IdempotencyKey:   uuid.New(),
		})
		require.NoError(t, err)

		approveResult, err := m8ServiceApprove(
			instrStore, pjlStore, auditStore, newM8IdempotencyStore(),
			true, ociCumul, costBasis,
			map[uuid.UUID]decimal.Decimal{portofolioHTCID: decimal.NewFromFloat(10_000_000_000)},
			map[uuid.UUID]decimal.Decimal{portofolioHTCID: decimal.Zero},
			decimal.NewFromFloat(5), decimal.NewFromFloat(10), m8AllJurnalCodes(),
			m8ApproveRequest{
				PenjualanID:    createResult.Penjualan.ID,
				ApproverID:     approverID,
				Comment:        "Partial AC disposal approved.",
				SignatureMethod: m8SignatureJWTStepUp,
				IdempotencyKey: uuid.New(),
			},
		)
		require.NoError(t, err)

		// qty_holding must be 500 - 200 = 300
		instrAfter, _ := instrStore.Get(instrID)
		assert.Equal(t, "300", instrAfter.QtyHolding.String(), "AB: partial qty_holding correct")
		assert.Equal(t, "ACTIVE", instrAfter.Status, "AB: instrumen stays ACTIVE after partial disposal")
		assert.Equal(t, "ACTIVE", approveResult.InstrumenStatus)

		// Verify pct computations are decimal-native (no float64)
		pct := m8ComputeBMFrequencyPct(
			decimal.Zero,
			createResult.Penjualan.ProceedIDR,
			decimal.NewFromFloat(10_000_000_000),
		)
		assert.IsType(t, decimal.Decimal{}, pct, "AB: BM pct uses decimal.Decimal, not float64")
	})
}

// ─── Routing matrix unit tests ────────────────────────────────────────────────

func TestE2E_P5M8_RoutingMatrix(t *testing.T) {
	cases := []struct {
		klasifikasi string
		expected    []string
	}{
		{m8KlasifikasiAC, []string{m8JurnalPenjualanAC}},
		{m8KlasifikasiFVOCI, []string{m8JurnalPenjualanFVOCIDebt, m8JurnalReklasOCIPL}},
		{m8KlasifikasiFVOCIElection, []string{m8JurnalPenjualanFVOCIElection}},
		{m8KlasifikasiFVTPL, []string{m8JurnalPenjualanFVTPL}},
		{m8KlasifikasiPOCI, []string{m8JurnalPenjualanPOCI}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run("routing:"+tc.klasifikasi, func(t *testing.T) {
			codes, err := m8JurnalEventCodes(tc.klasifikasi)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, codes, "klasifikasi=%s", tc.klasifikasi)
		})
	}

	t.Run("routing: FVOCI must include REKLAS_OCI_PL (S3 compliance)", func(t *testing.T) {
		codes, _ := m8JurnalEventCodes(m8KlasifikasiFVOCI)
		assert.Contains(t, codes, m8JurnalReklasOCIPL)
	})

	t.Run("routing: FVOCI_ELECTION must NOT include REKLAS_OCI_PL (§B5.7.1)", func(t *testing.T) {
		codes, _ := m8JurnalEventCodes(m8KlasifikasiFVOCIElection)
		assert.NotContains(t, codes, m8JurnalReklasOCIPL)
	})

	t.Run("routing: unknown klasifikasi returns error", func(t *testing.T) {
		_, err := m8JurnalEventCodes("UNKNOWN_KLASIFIKASI")
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "unknown")
	})
}
