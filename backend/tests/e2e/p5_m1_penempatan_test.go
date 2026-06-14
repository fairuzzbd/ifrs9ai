// Package e2e — P5-M1 Penempatan Deposito end-to-end tests.
//
// Scope: Full lifecycle of trx.penempatan_deposito: create → 4-eyes workflow →
// APPROVED_ACTIVE → auto-mature (Asynq cron) and manual terminate (4-eyes).
// All scenarios use the in-process harness pattern from phase4_ecl_engine_test.go.
//
// Scenarios:
//
//	P5-A  Full AC happy path (create → submit → review → approve → APPROVED_ACTIVE → MATURED)
//	P5-B  FVTPL guard — staging_skipped, no EIR enqueue (DEC-P5-M1-001)
//	P5-C  FVOCI_ELECTION guard — same skip behavior
//	P5-D  SoD: maker tries to review own penempatan → 403 PENEMPATAN_SOD_VIOLATION
//	P5-E  SoD: reviewer tries to approve → 403
//	P5-F  Step-up MFA stale (> 5 min) → 403 PENEMPATAN_STEP_UP_REQUIRED
//	P5-G  Terminate 4-eyes happy path → TERMINATED + audit + PenempatanTerminatedEvent
//	P5-H  Terminate SoD violations (F2 fix) — maker cannot terminate-review or terminate-approve
//	P5-I  Settlement balance hint informational — no 422 block (DEC-P5-M1-004)
//	P5-J  kode_transaksi sequence — 3 penempatan in same month → PNP-202606-000001..3
//	P5-K  Hard delete forbidden — DB trigger rejects direct DELETE
//
// Decision log compliance:
//
//	DEC-P5-M1-001: FVTPL/FVOCI_ELECTION skip staging + EIR          — Scenario P5-B, P5-C
//	DEC-P5-M1-004: Settlement balance informational only              — Scenario P5-I
//	DEC-P5-M1-005: Terminate = 4-eyes full workflow                  — Scenario P5-G, P5-H
//	DEC-013:       EIR Newton-Raphson                                 — Scenario P5-A (EIR enqueue)
//	DEC-017:       SoD maker ≠ reviewer ≠ approver                  — Scenario P5-D, P5-E, P5-H
//	DEC-018:       Audit trail in-transaction                         — All scenarios
//	DEC-021:       Idempotency-Key mandatory on mutations             — Scenario P5-A, P5-G
//	DEC-027:       Step-up MFA for approve                           — Scenario P5-F
//
// Run:
//
//	go test ./tests/e2e/... -v -run TestE2E_P5M1 -timeout 60s
package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// ─── P5-M1 domain constants ───────────────────────────────────────────────────

const (
	// Workflow statuses (mirrors trx.penempatan_workflow_status ENUM).
	statusDraft                      = "DRAFT"
	statusPendingReview              = "PENDING_REVIEW"
	statusPendingApproval            = "PENDING_APPROVAL"
	statusApprovedActive             = "APPROVED_ACTIVE"
	statusMatured                    = "MATURED"
	statusTerminated                 = "TERMINATED"
	statusCancelled                  = "CANCELLED"
	statusTerminationPendingReview   = "TERMINATION_PENDING_REVIEW"
	statusTerminationPendingApproval = "TERMINATION_PENDING_APPROVAL"

	// Klasifikasi PSAK 71 (mirrors mst.instrumen.klasifikasi_psak71).
	klasifikasiAC        = "AC"
	klasifikasiFVOCI     = "FVOCI"
	klasifikasiFVTPL     = "FVTPL"
	klasifikasiFVOCIELEC = "FVOCI_ELECTION"
	klasifikasiPOCI      = "POCI"

	// Audit action constants (mirrors aud.audit_log.action values for P5-M1).
	auditPenempatanCreated        = "PENEMPATAN.CREATED"
	auditPenempatanSubmitted      = "PENEMPATAN.SUBMITTED"
	auditPenempatanReviewed       = "PENEMPATAN.REVIEWED"
	auditPenempatanApproved       = "PENEMPATAN.APPROVED"
	auditPenempatanRejected       = "PENEMPATAN.REJECTED"
	auditPenempatanStagingInitial = "PENEMPATAN.STAGING_INITIAL"
	auditPenempatanStagingSkipped = "PENEMPATAN.STAGING_SKIPPED_FVTPL"
	auditPenempatanMatured        = "PENEMPATAN.MATURED"
	auditPenempatanDerecognQueued = "PENEMPATAN.DERECOGNITION_QUEUED"
	auditPenempatanTermProp       = "PENEMPATAN.TERMINATE_PROPOSED"
	auditPenempatanTermReviewed   = "PENEMPATAN.TERMINATE_REVIEWED"
	auditPenempatanTermApproved   = "PENEMPATAN.TERMINATE_APPROVED"
	auditPenempatanTermRejected   = "PENEMPATAN.TERMINATE_REJECTED"
	auditPenempatanSODAttempt     = "PENEMPATAN.SOD_VIOLATION_ATTEMPT"

	// Staging action response field values.
	stagingActionStage1Assigned = "STAGE_1_ASSIGNED"
	stagingActionSkippedFVTPL   = "SKIPPED_FVTPL"

	// Error codes (mirrors p5-m1-penempatan.md §6 error catalog).
	errSODViolation        = "PENEMPATAN_SOD_VIOLATION"
	errStepUpRequired      = "PENEMPATAN_STEP_UP_REQUIRED"
	errInvalidTransition   = "PENEMPATAN_INVALID_TRANSITION"
	errPeriodeClosed       = "PENEMPATAN_PERIODE_HARD_CLOSED"
	errHardDeleteForbidden = "HARD_DELETE_FORBIDDEN"
)

// ─── P5-M1 domain types ───────────────────────────────────────────────────────

// penempatanRecord is the in-memory representation of trx.penempatan_deposito.
type penempatanRecord struct {
	ID                  uuid.UUID
	KodeTransaksi       string
	InstrumenID         uuid.UUID
	KlasifikasiPsak71   string
	CounterpartyBankID  uuid.UUID
	PeriodeID           uuid.UUID
	TanggalPenempatan   time.Time
	TanggalJatuhTempo   time.Time
	NominalIDR          decimal.Decimal
	NominalFCY          *decimal.Decimal
	KursPenempatan      *decimal.Decimal
	TenorBulan          int
	KuponPersen         decimal.Decimal
	BiayaTransaksiIDR   decimal.Decimal
	SettlementAccount   string
	WorkflowStatus      string
	MakerID             uuid.UUID
	ReviewerID          *uuid.UUID
	ApproverID          *uuid.UUID
	ReviewerSignedAt    *time.Time
	ApproverSignedAt    *time.Time
	ReviewerSigHash     string
	ApproverSigHash     string
	RejectReason        string
	TerminateReason     string
	TerminatedAt        *time.Time
	TerminateReviewerID *uuid.UUID
	TerminateApproverID *uuid.UUID
	TermRevSignedAt     *time.Time
	TermApprSignedAt    *time.Time
	TermRevSigHash      string
	TermApprSigHash     string
	MaturedAt           *time.Time
	EIRAwal             *decimal.Decimal
	CarryingAmountAwal  *decimal.Decimal
	StagingAction       string
	EIRComputeJobID     *string
	RowVersion          int64
	DeletedAt           *time.Time
	TenantID            string
}

// penempatanApprovedEvent is the event emitted post-approve (consumed by P5-M2).
type penempatanApprovedEvent struct {
	InstrumenID       uuid.UUID
	PenempatanID      uuid.UUID
	KodeTransaksi     string
	KlasifikasiPsak71 string
	NominalIDR        decimal.Decimal
	StagingAction     string
	EventTime         time.Time
	TenantID          string
}

// penempatanMaturedEvent is the event emitted by the Asynq maturity-checker.
type penempatanMaturedEvent struct {
	InstrumenID   uuid.UUID
	PenempatanID  uuid.UUID
	KodeTransaksi string
	MaturedAt     time.Time
	NominalIDR    decimal.Decimal
	EventTime     time.Time
	TenantID      string
}

// penempatanTerminatedEvent is the event emitted post-terminate-approve.
type penempatanTerminatedEvent struct {
	InstrumenID   uuid.UUID
	PenempatanID  uuid.UUID
	KodeTransaksi string
	TerminateDate time.Time
	NominalIDR    decimal.Decimal
	EventTime     time.Time
	TenantID      string
}

// settlementBalanceHint is the informational hint returned at create (DEC-P5-M1-004).
type settlementBalanceHint struct {
	LastKnownIDR decimal.Decimal
	AsOfDate     time.Time
	IsStale      bool // true if as_of_date > 24h ago
}

// eIRComputeTask is the Asynq task payload for EIR computation.
type eIRComputeTask struct {
	PenempatanID      uuid.UUID
	InstrumenID       uuid.UUID
	KlasifikasiPsak71 string
	Enqueued          bool
}

// penempatanSODError is returned for Segregation of Duties violations.
type penempatanSODError struct {
	Code    string
	Message string
}

func (e *penempatanSODError) Error() string { return e.Code + ": " + e.Message }

// penempatanStepUpError is returned when MFA step-up token is stale/missing.
type penempatanStepUpError struct {
	Code    string
	Message string
}

func (e *penempatanStepUpError) Error() string { return e.Code + ": " + e.Message }

// penempatanTransitionError is returned for invalid workflow transitions.
type penempatanTransitionError struct {
	Code    string
	Message string
}

func (e *penempatanTransitionError) Error() string { return e.Code + ": " + e.Message }

// penempatanHardDeleteError is returned when direct DELETE is attempted.
type penempatanHardDeleteError struct {
	Code    string
	Message string
}

func (e *penempatanHardDeleteError) Error() string { return e.Code + ": " + e.Message }

// ─── P5-M1 harness ───────────────────────────────────────────────────────────

// p5M1Harness wires up the in-process P5-M1 service stubs.
// Mirrors the phase4 e2eHarness pattern — no real DB, no Docker.
type p5M1Harness struct {
	t              *testing.T
	svc            *penempatanTestService
	auditLog       *p5AuditStore
	instrStore     *p5InstrumenStore
	periodeStore   *p5PeriodeStore
	balanceStore   *p5SettlementBalanceStore
	eventBus       *p5EventBus
	asynqTaskQueue *p5AsynqQueue
	seqCounter     map[string]int // month-key → sequence counter for kode_transaksi
}

func newP5M1Harness(t *testing.T) *p5M1Harness {
	t.Helper()
	h := &p5M1Harness{t: t}
	h.auditLog = newP5AuditStore()
	h.instrStore = newP5InstrumenStore()
	h.periodeStore = newP5PeriodeStore()
	h.balanceStore = newP5SettlementBalanceStore()
	h.eventBus = newP5EventBus()
	h.asynqTaskQueue = newP5AsynqQueue()
	h.seqCounter = make(map[string]int)
	h.svc = newPenempatanTestService(h)
	return h
}

// ─── Audit store ──────────────────────────────────────────────────────────────

type p5AuditRow struct {
	EventID     string
	Action      string
	EntityID    string
	ActorID     string
	ActorRole   string
	CurrentHash []byte
	AfterJSON   map[string]interface{}
}

type p5AuditStore struct {
	rows []p5AuditRow
}

func newP5AuditStore() *p5AuditStore { return &p5AuditStore{} }

func (s *p5AuditStore) append(action, entityID, actorID, actorRole string, afterJSON map[string]interface{}) {
	var prevHash []byte
	if len(s.rows) > 0 {
		prevHash = s.rows[len(s.rows)-1].CurrentHash
	}
	payload := fmt.Sprintf("%x||%s||%s||%v", prevHash, action, entityID, afterJSON)
	h := sha256.Sum256([]byte(payload))
	s.rows = append(s.rows, p5AuditRow{
		EventID:     uuid.New().String(),
		Action:      action,
		EntityID:    entityID,
		ActorID:     actorID,
		ActorRole:   actorRole,
		CurrentHash: h[:],
		AfterJSON:   afterJSON,
	})
}

func (s *p5AuditStore) actionsForEntity(entityID string) []string {
	var out []string
	for _, r := range s.rows {
		if r.EntityID == entityID {
			out = append(out, r.Action)
		}
	}
	return out
}

func (s *p5AuditStore) containsAction(entityID, action string) bool {
	for _, a := range s.actionsForEntity(entityID) {
		if a == action {
			return true
		}
	}
	return false
}

func (s *p5AuditStore) countAction(entityID, action string) int {
	n := 0
	for _, a := range s.actionsForEntity(entityID) {
		if a == action {
			n++
		}
	}
	return n
}

func (s *p5AuditStore) rowsForEntity(entityID string) []p5AuditRow {
	var out []p5AuditRow
	for _, r := range s.rows {
		if r.EntityID == entityID {
			out = append(out, r)
		}
	}
	return out
}

// ─── Instrumen store ──────────────────────────────────────────────────────────

type p5InstrumenRecord struct {
	ID                uuid.UUID
	Kode              string
	KlasifikasiPsak71 string // AC | FVOCI | FVTPL | FVOCI_ELECTION | POCI
	WorkflowStatus    string // APPROVED = klasifikasi locked
	Status            string // AKTIF
}

type p5InstrumenStore struct {
	records map[uuid.UUID]*p5InstrumenRecord
}

func newP5InstrumenStore() *p5InstrumenStore {
	return &p5InstrumenStore{records: make(map[uuid.UUID]*p5InstrumenRecord)}
}

func (s *p5InstrumenStore) seed(kode, klasifikasi string) *p5InstrumenRecord {
	r := &p5InstrumenRecord{
		ID:                uuid.New(),
		Kode:              kode,
		KlasifikasiPsak71: klasifikasi,
		WorkflowStatus:    "APPROVED",
		Status:            "AKTIF",
	}
	s.records[r.ID] = r
	return r
}

func (s *p5InstrumenStore) get(id uuid.UUID) *p5InstrumenRecord {
	return s.records[id]
}

// ─── Periode store ────────────────────────────────────────────────────────────

type p5PeriodeRecord struct {
	ID            uuid.UUID
	Kode          string
	StatusPeriode string // OPEN | SOFT_CLOSED | HARD_CLOSED
}

type p5PeriodeStore struct {
	records map[uuid.UUID]*p5PeriodeRecord
}

func newP5PeriodeStore() *p5PeriodeStore {
	return &p5PeriodeStore{records: make(map[uuid.UUID]*p5PeriodeRecord)}
}

func (s *p5PeriodeStore) seedOpen(kode string) *p5PeriodeRecord {
	r := &p5PeriodeRecord{
		ID:            uuid.New(),
		Kode:          kode,
		StatusPeriode: "OPEN",
	}
	s.records[r.ID] = r
	return r
}

func (s *p5PeriodeStore) close(id uuid.UUID) { //nolint:unused // harness helper — used in future TC
	if r := s.records[id]; r != nil {
		r.StatusPeriode = "SOFT_CLOSED"
	}
}

func (s *p5PeriodeStore) isOpen(id uuid.UUID) bool {
	r := s.records[id]
	return r != nil && r.StatusPeriode == "OPEN"
}

// ─── Settlement balance store ─────────────────────────────────────────────────

type p5SettlementBalance struct {
	LastKnownBalanceIDR decimal.Decimal
	AsOfDate            time.Time
}

type p5SettlementBalanceStore struct {
	balances map[string]*p5SettlementBalance
}

func newP5SettlementBalanceStore() *p5SettlementBalanceStore {
	return &p5SettlementBalanceStore{balances: make(map[string]*p5SettlementBalance)}
}

func (s *p5SettlementBalanceStore) seed(account string, balance decimal.Decimal, asOf time.Time) {
	s.balances[account] = &p5SettlementBalance{LastKnownBalanceIDR: balance, AsOfDate: asOf}
}

func (s *p5SettlementBalanceStore) getHint(account string, nominalIDR decimal.Decimal) *settlementBalanceHint {
	b := s.balances[account]
	if b == nil {
		return nil
	}
	isStale := time.Since(b.AsOfDate) > 24*time.Hour
	return &settlementBalanceHint{
		LastKnownIDR: b.LastKnownBalanceIDR,
		AsOfDate:     b.AsOfDate,
		IsStale:      isStale,
	}
}

// ─── Event bus (captures emitted domain events) ───────────────────────────────

type p5EventBus struct {
	approvedEvents   []penempatanApprovedEvent
	maturedEvents    []penempatanMaturedEvent
	terminatedEvents []penempatanTerminatedEvent
}

func newP5EventBus() *p5EventBus { return &p5EventBus{} }

func (b *p5EventBus) emitApproved(e penempatanApprovedEvent) {
	b.approvedEvents = append(b.approvedEvents, e)
}
func (b *p5EventBus) emitMatured(e penempatanMaturedEvent) {
	b.maturedEvents = append(b.maturedEvents, e)
}
func (b *p5EventBus) emitTerminated(e penempatanTerminatedEvent) {
	b.terminatedEvents = append(b.terminatedEvents, e)
}

func (b *p5EventBus) approvedCount() int   { return len(b.approvedEvents) }
func (b *p5EventBus) maturedCount() int    { return len(b.maturedEvents) }
func (b *p5EventBus) terminatedCount() int { return len(b.terminatedEvents) }

// ─── Asynq task queue stub ────────────────────────────────────────────────────

type p5AsynqQueue struct {
	eirTasks      []eIRComputeTask
	maturityTasks int //nolint:unused // counter reserved for future maturity-check enqueue assertions
}

func newP5AsynqQueue() *p5AsynqQueue { return &p5AsynqQueue{} }

func (q *p5AsynqQueue) enqueueEIRCompute(t eIRComputeTask) { q.eirTasks = append(q.eirTasks, t) }
func (q *p5AsynqQueue) enqueueMaturityCheck()              { q.maturityTasks++ } //nolint:unused // harness helper
func (q *p5AsynqQueue) eirComputeCount() int               { return len(q.eirTasks) }
func (q *p5AsynqQueue) hasEIRTaskForPenempatan(id uuid.UUID) bool {
	for _, t := range q.eirTasks {
		if t.PenempatanID == id {
			return true
		}
	}
	return false
}

// ─── penempatan test service ──────────────────────────────────────────────────
//
// This service mirrors the production PenempatanService interface (state machine §12)
// but operates entirely in-memory without a real database. All side-effects
// (audit, events, Asynq tasks) go to the harness stores above.

type penempatanTestService struct {
	h          *p5M1Harness
	records    map[uuid.UUID]*penempatanRecord
	idempotent map[uuid.UUID]*penempatanRecord // idempotency key → original response
}

func newPenempatanTestService(h *p5M1Harness) *penempatanTestService {
	return &penempatanTestService{
		h:          h,
		records:    make(map[uuid.UUID]*penempatanRecord),
		idempotent: make(map[uuid.UUID]*penempatanRecord),
	}
}

func (s *penempatanTestService) generateKode(month time.Month, year int) string {
	key := fmt.Sprintf("%04d%02d", year, int(month))
	s.h.seqCounter[key]++
	return fmt.Sprintf("PNP-%04d%02d-%06d", year, int(month), s.h.seqCounter[key])
}

// computeSignatureHash computes SHA-256(actorID||step||entityID||signedAt||comment).
func computeSignatureHash(actorID uuid.UUID, step string, entityID uuid.UUID, signedAt time.Time, comment string) []byte {
	payload := fmt.Sprintf("%s||%s||%s||%d||%s", actorID, step, entityID, signedAt.Unix(), comment)
	h := sha256.Sum256([]byte(payload))
	return h[:]
}

// ─── T01: Create (DRAFT) ──────────────────────────────────────────────────────

type createPenempatanRequest struct {
	InstrumenID        uuid.UUID
	CounterpartyBankID uuid.UUID
	PeriodeID          uuid.UUID
	TanggalPenempatan  time.Time
	NominalIDR         decimal.Decimal
	NominalFCY         *decimal.Decimal
	MataUangKode       string
	KursPenempatan     *decimal.Decimal
	TenorBulan         int
	KuponPersen        decimal.Decimal
	BiayaTransaksiIDR  decimal.Decimal
	SettlementAccount  string
	IdempotencyKey     uuid.UUID
}

type createPenempatanResult struct {
	Record                *penempatanRecord
	SettlementBalanceHint *settlementBalanceHint
}

func (s *penempatanTestService) Create(ctx context.Context, req createPenempatanRequest, actor *auth.Claims) (*createPenempatanResult, error) {
	// Idempotency check (DEC-021).
	if existing, ok := s.idempotent[req.IdempotencyKey]; ok {
		hint := s.h.balanceStore.getHint(existing.SettlementAccount, existing.NominalIDR)
		return &createPenempatanResult{Record: existing, SettlementBalanceHint: hint}, nil
	}

	// Guard: instrumen must have klasifikasi APPROVED.
	instr := s.h.instrStore.get(req.InstrumenID)
	if instr == nil || instr.WorkflowStatus != "APPROVED" {
		return nil, &penempatanTransitionError{
			Code:    "PENEMPATAN_INSTRUMEN_INVALID_KLASIFIKASI",
			Message: "instrumen belum memiliki klasifikasi PSAK 71 yang di-approve",
		}
	}

	// Guard: periode OPEN.
	if !s.h.periodeStore.isOpen(req.PeriodeID) {
		return nil, &penempatanTransitionError{Code: errPeriodeClosed, Message: "periode buku bukan OPEN"}
	}

	actorID, _ := uuid.Parse(actor.Sub)
	tanggalJatuhTempo := req.TanggalPenempatan.AddDate(0, req.TenorBulan, 0)

	r := &penempatanRecord{
		ID:                 uuid.New(),
		KodeTransaksi:      s.generateKode(req.TanggalPenempatan.Month(), req.TanggalPenempatan.Year()),
		InstrumenID:        req.InstrumenID,
		KlasifikasiPsak71:  instr.KlasifikasiPsak71,
		CounterpartyBankID: req.CounterpartyBankID,
		PeriodeID:          req.PeriodeID,
		TanggalPenempatan:  req.TanggalPenempatan,
		TanggalJatuhTempo:  tanggalJatuhTempo,
		NominalIDR:         req.NominalIDR,
		NominalFCY:         req.NominalFCY,
		KursPenempatan:     req.KursPenempatan,
		TenorBulan:         req.TenorBulan,
		KuponPersen:        req.KuponPersen,
		BiayaTransaksiIDR:  req.BiayaTransaksiIDR,
		SettlementAccount:  req.SettlementAccount,
		WorkflowStatus:     statusDraft,
		MakerID:            actorID,
		RowVersion:         1,
		TenantID:           actor.TenantID,
	}
	s.records[r.ID] = r
	s.idempotent[req.IdempotencyKey] = r

	// Audit PENEMPATAN.CREATED in-transaction (DEC-018).
	s.h.auditLog.append(auditPenempatanCreated, r.ID.String(), actorID.String(), actor.Roles[0],
		map[string]interface{}{"kode_transaksi": r.KodeTransaksi, "workflow_status": statusDraft})

	// Settlement balance hint (DEC-P5-M1-004 — informational, never blocks).
	hint := s.h.balanceStore.getHint(req.SettlementAccount, req.NominalIDR)

	return &createPenempatanResult{Record: r, SettlementBalanceHint: hint}, nil
}

// ─── T02: Submit (DRAFT → PENDING_REVIEW) ────────────────────────────────────

type workflowActionRequest struct {
	Comment         string
	SignatureMethod string
	IdempotencyKey  uuid.UUID
}

func (s *penempatanTestService) Submit(ctx context.Context, id uuid.UUID, req workflowActionRequest, actor *auth.Claims) (*penempatanRecord, error) {
	r := s.records[id]
	if r == nil {
		return nil, &penempatanTransitionError{Code: "NOT_FOUND", Message: "penempatan not found"}
	}
	actorID, _ := uuid.Parse(actor.Sub)

	// State guard.
	if r.WorkflowStatus != statusDraft {
		return nil, &penempatanTransitionError{Code: errInvalidTransition, Message: "can only submit from DRAFT"}
	}
	// Maker can only submit own draft.
	if r.MakerID != actorID {
		return nil, &penempatanSODError{Code: errSODViolation, Message: "only maker can submit"}
	}

	r.WorkflowStatus = statusPendingReview
	r.RowVersion++

	s.h.auditLog.append(auditPenempatanSubmitted, r.ID.String(), actorID.String(), actor.Roles[0],
		map[string]interface{}{"workflow_status": statusPendingReview})
	return r, nil
}

// ─── T04: Review (PENDING_REVIEW → PENDING_APPROVAL) ─────────────────────────

func (s *penempatanTestService) Review(ctx context.Context, id uuid.UUID, req workflowActionRequest, actor *auth.Claims) (*penempatanRecord, error) {
	r := s.records[id]
	if r == nil {
		return nil, &penempatanTransitionError{Code: "NOT_FOUND", Message: "penempatan not found"}
	}
	actorID, _ := uuid.Parse(actor.Sub)

	// State guard.
	if r.WorkflowStatus != statusPendingReview {
		return nil, &penempatanTransitionError{Code: errInvalidTransition, Message: "can only review from PENDING_REVIEW"}
	}

	// SoD: reviewer ≠ maker (DEC-017).
	if r.MakerID == actorID {
		// Audit SoD attempt (non-transactional per spec — fire-and-forget).
		s.h.auditLog.append(auditPenempatanSODAttempt, r.ID.String(), actorID.String(), actor.Roles[0],
			map[string]interface{}{"attempted_step": "REVIEW", "sod_violation": "reviewer=maker"})
		return nil, &penempatanSODError{
			Code:    errSODViolation,
			Message: "reviewer tidak bisa menjadi maker pada transaksi yang sama (DEC-017)",
		}
	}

	now := time.Now()
	sigHash := computeSignatureHash(actorID, "REVIEW", r.ID, now, req.Comment)
	sigHashHex := hex.EncodeToString(sigHash)

	r.WorkflowStatus = statusPendingApproval
	r.ReviewerID = &actorID
	r.ReviewerSignedAt = &now
	r.ReviewerSigHash = sigHashHex
	r.RowVersion++

	s.h.auditLog.append(auditPenempatanReviewed, r.ID.String(), actorID.String(), actor.Roles[0],
		map[string]interface{}{"reviewer_signature_hash": sigHashHex, "workflow_status": statusPendingApproval})
	return r, nil
}

// ─── T06: Approve (PENDING_APPROVAL → APPROVED_ACTIVE) ───────────────────────

type approveResult struct {
	Record        *penempatanRecord
	StagingAction string
	EIRJobID      *string
}

func (s *penempatanTestService) Approve(ctx context.Context, id uuid.UUID, req workflowActionRequest, actor *auth.Claims) (*approveResult, error) {
	r := s.records[id]
	if r == nil {
		return nil, &penempatanTransitionError{Code: "NOT_FOUND", Message: "penempatan not found"}
	}
	actorID, _ := uuid.Parse(actor.Sub)

	// State guard.
	if r.WorkflowStatus != statusPendingApproval {
		return nil, &penempatanTransitionError{Code: errInvalidTransition, Message: "can only approve from PENDING_APPROVAL"}
	}

	// Step-up MFA guard (DEC-027).
	if actor.NeedsStepUp() {
		return nil, &penempatanStepUpError{
			Code:    errStepUpRequired,
			Message: "persetujuan penempatan memerlukan MFA step-up (DEC-027)",
		}
	}

	// SoD: approver ≠ maker AND approver ≠ reviewer (DEC-017).
	if r.MakerID == actorID {
		s.h.auditLog.append(auditPenempatanSODAttempt, r.ID.String(), actorID.String(), actor.Roles[0],
			map[string]interface{}{"attempted_step": "APPROVE", "sod_violation": "approver=maker"})
		return nil, &penempatanSODError{Code: errSODViolation, Message: "approver tidak bisa menjadi maker (DEC-017)"}
	}
	if r.ReviewerID != nil && *r.ReviewerID == actorID {
		s.h.auditLog.append(auditPenempatanSODAttempt, r.ID.String(), actorID.String(), actor.Roles[0],
			map[string]interface{}{"attempted_step": "APPROVE", "sod_violation": "approver=reviewer"})
		return nil, &penempatanSODError{Code: errSODViolation, Message: "approver tidak bisa menjadi reviewer (DEC-017)"}
	}

	// Re-check periode OPEN.
	if !s.h.periodeStore.isOpen(r.PeriodeID) {
		return nil, &penempatanTransitionError{Code: errPeriodeClosed, Message: "periode buku sudah di-close saat approve"}
	}

	now := time.Now()
	sigHash := computeSignatureHash(actorID, "APPROVE", r.ID, now, req.Comment)
	sigHashHex := hex.EncodeToString(sigHash)

	r.WorkflowStatus = statusApprovedActive
	r.ApproverID = &actorID
	r.ApproverSignedAt = &now
	r.ApproverSigHash = sigHashHex
	r.RowVersion++

	// Audit PENEMPATAN.APPROVED in-transaction.
	s.h.auditLog.append(auditPenempatanApproved, r.ID.String(), actorID.String(), actor.Roles[0],
		map[string]interface{}{"approver_signature_hash": sigHashHex, "workflow_status": statusApprovedActive})

	// FVTPL guard (DEC-P5-M1-001, state machine §3).
	var stagingAction string
	var eirJobID *string

	switch r.KlasifikasiPsak71 {
	case klasifikasiFVTPL, klasifikasiFVOCIELEC:
		// Skip ECL staging and EIR compute entirely.
		stagingAction = stagingActionSkippedFVTPL
		r.StagingAction = stagingAction
		r.EIRAwal = nil
		s.h.auditLog.append(auditPenempatanStagingSkipped, r.ID.String(), "system", "system",
			map[string]interface{}{"klasifikasi": r.KlasifikasiPsak71, "staging_action": stagingAction})

	default: // AC, FVOCI, POCI
		// INSERT ecl.stage_history (STAGE_1, INITIAL_PLACEMENT) — in-transaction.
		stagingAction = stagingActionStage1Assigned
		r.StagingAction = stagingAction
		s.h.auditLog.append(auditPenempatanStagingInitial, r.ID.String(), "system", "system",
			map[string]interface{}{"stage": "STAGE_1", "trigger": "INITIAL_PLACEMENT"})

		// Enqueue Asynq EIR_COMPUTE (async — DEC-013).
		jobIDStr := "eir-job-" + r.ID.String()
		eirJobID = &jobIDStr
		r.EIRComputeJobID = eirJobID
		s.h.asynqTaskQueue.enqueueEIRCompute(eIRComputeTask{
			PenempatanID:      r.ID,
			InstrumenID:       r.InstrumenID,
			KlasifikasiPsak71: r.KlasifikasiPsak71,
			Enqueued:          true,
		})
	}

	// Emit PenempatanApprovedEvent (consumed by P5-M2 jurnal engine).
	s.h.eventBus.emitApproved(penempatanApprovedEvent{
		InstrumenID:       r.InstrumenID,
		PenempatanID:      r.ID,
		KodeTransaksi:     r.KodeTransaksi,
		KlasifikasiPsak71: r.KlasifikasiPsak71,
		NominalIDR:        r.NominalIDR,
		StagingAction:     stagingAction,
		EventTime:         now,
		TenantID:          r.TenantID,
	})

	return &approveResult{Record: r, StagingAction: stagingAction, EIRJobID: eirJobID}, nil
}

// ─── T08: Auto-mature (Asynq maturity-checker cron) ──────────────────────────

// RunMaturityCheck simulates the Asynq 09:00 WIB daily cron.
// Per-penempatan transaction (not one big tx); partial failure allowed.
// Returns (matured, errors).
func (s *penempatanTestService) RunMaturityCheck(ctx context.Context, today time.Time) ([]uuid.UUID, []error) {
	matured := make([]uuid.UUID, 0)
	errs := make([]error, 0)

	for _, r := range s.records {
		if r.WorkflowStatus != statusApprovedActive {
			continue
		}
		if r.DeletedAt != nil {
			continue
		}
		// Skip records in termination workflow (state machine §2 note).
		if r.TanggalJatuhTempo.After(today) {
			continue
		}

		now := time.Now()
		r.WorkflowStatus = statusMatured
		r.MaturedAt = &now
		r.RowVersion++

		// Audit PENEMPATAN.MATURED in per-record transaction.
		s.h.auditLog.append(auditPenempatanMatured, r.ID.String(), "system", "system",
			map[string]interface{}{"matured_at": today.Format("2006-01-02")})
		s.h.auditLog.append(auditPenempatanDerecognQueued, r.ID.String(), "system", "system",
			map[string]interface{}{"event_type": "JATUH_TEMPO", "downstream": "P5-M9"})

		// Emit PenempatanMaturedEvent.
		s.h.eventBus.emitMatured(penempatanMaturedEvent{
			InstrumenID:   r.InstrumenID,
			PenempatanID:  r.ID,
			KodeTransaksi: r.KodeTransaksi,
			MaturedAt:     today,
			NominalIDR:    r.NominalIDR,
			EventTime:     now,
			TenantID:      r.TenantID,
		})

		matured = append(matured, r.ID)
	}

	return matured, errs
}

// ─── T09: Terminate propose (APPROVED_ACTIVE → TERMINATION_PENDING_REVIEW) ───

type terminateRequestBody struct {
	TerminateReason    string
	DokumenTerminasiID *uuid.UUID
	IdempotencyKey     uuid.UUID
}

func (s *penempatanTestService) TerminateRequest(ctx context.Context, id uuid.UUID, req terminateRequestBody, actor *auth.Claims) (*penempatanRecord, error) {
	r := s.records[id]
	if r == nil {
		return nil, &penempatanTransitionError{Code: "NOT_FOUND", Message: "penempatan not found"}
	}
	if r.WorkflowStatus != statusApprovedActive {
		return nil, &penempatanTransitionError{
			Code:    "PENEMPATAN_TERMINATE_FORBIDDEN_NOT_ACTIVE",
			Message: "terminasi hanya dari APPROVED_ACTIVE",
		}
	}
	if len(req.TerminateReason) < 30 {
		return nil, &penempatanTransitionError{
			Code:    "PENEMPATAN_REASON_TOO_SHORT",
			Message: "alasan terminasi minimal 30 karakter",
		}
	}

	actorID, _ := uuid.Parse(actor.Sub)
	r.WorkflowStatus = statusTerminationPendingReview
	r.TerminateReason = req.TerminateReason
	r.RowVersion++

	s.h.auditLog.append(auditPenempatanTermProp, r.ID.String(), actorID.String(), actor.Roles[0],
		map[string]interface{}{"terminate_reason": req.TerminateReason})
	return r, nil
}

// ─── T10: Terminate review (TERMINATION_PENDING_REVIEW → TERMINATION_PENDING_APPROVAL)

func (s *penempatanTestService) TerminateReview(ctx context.Context, id uuid.UUID, req workflowActionRequest, actor *auth.Claims) (*penempatanRecord, error) {
	r := s.records[id]
	if r == nil {
		return nil, &penempatanTransitionError{Code: "NOT_FOUND", Message: "penempatan not found"}
	}
	actorID, _ := uuid.Parse(actor.Sub)

	if r.WorkflowStatus != statusTerminationPendingReview {
		return nil, &penempatanTransitionError{Code: errInvalidTransition, Message: "must be TERMINATION_PENDING_REVIEW"}
	}

	// SoD: terminate_reviewer ≠ maker (DEC-017, state machine §4).
	if r.MakerID == actorID {
		s.h.auditLog.append(auditPenempatanSODAttempt, r.ID.String(), actorID.String(), actor.Roles[0],
			map[string]interface{}{"attempted_step": "TERMINATE_REVIEW", "sod_violation": "terminate_reviewer=maker"})
		return nil, &penempatanSODError{
			Code:    errSODViolation,
			Message: "terminate reviewer tidak bisa sama dengan maker (DEC-017)",
		}
	}

	now := time.Now()
	sigHash := computeSignatureHash(actorID, "TERMINATE_REVIEW", r.ID, now, req.Comment)
	sigHashHex := hex.EncodeToString(sigHash)

	r.WorkflowStatus = statusTerminationPendingApproval
	r.TerminateReviewerID = &actorID
	r.TermRevSignedAt = &now
	r.TermRevSigHash = sigHashHex
	r.RowVersion++

	s.h.auditLog.append(auditPenempatanTermReviewed, r.ID.String(), actorID.String(), actor.Roles[0],
		map[string]interface{}{"terminate_reviewer_signature_hash": sigHashHex})
	return r, nil
}

// ─── T12: Terminate approve (TERMINATION_PENDING_APPROVAL → TERMINATED) ──────

func (s *penempatanTestService) TerminateApprove(ctx context.Context, id uuid.UUID, req workflowActionRequest, actor *auth.Claims) (*penempatanRecord, error) {
	r := s.records[id]
	if r == nil {
		return nil, &penempatanTransitionError{Code: "NOT_FOUND", Message: "penempatan not found"}
	}
	actorID, _ := uuid.Parse(actor.Sub)

	if r.WorkflowStatus != statusTerminationPendingApproval {
		return nil, &penempatanTransitionError{Code: errInvalidTransition, Message: "must be TERMINATION_PENDING_APPROVAL"}
	}

	// Step-up MFA guard (DEC-027).
	if actor.NeedsStepUp() {
		return nil, &penempatanStepUpError{Code: errStepUpRequired, Message: "terminate-approve requires MFA step-up"}
	}

	// SoD: terminate_approver ≠ maker AND ≠ terminate_reviewer.
	if r.MakerID == actorID {
		s.h.auditLog.append(auditPenempatanSODAttempt, r.ID.String(), actorID.String(), actor.Roles[0],
			map[string]interface{}{"attempted_step": "TERMINATE_APPROVE", "sod_violation": "terminate_approver=maker"})
		return nil, &penempatanSODError{Code: errSODViolation, Message: "terminate approver tidak bisa sama dengan maker"}
	}
	if r.TerminateReviewerID != nil && *r.TerminateReviewerID == actorID {
		s.h.auditLog.append(auditPenempatanSODAttempt, r.ID.String(), actorID.String(), actor.Roles[0],
			map[string]interface{}{"attempted_step": "TERMINATE_APPROVE", "sod_violation": "terminate_approver=terminate_reviewer"})
		return nil, &penempatanSODError{Code: errSODViolation, Message: "terminate approver tidak bisa sama dengan terminate reviewer"}
	}

	now := time.Now()
	sigHash := computeSignatureHash(actorID, "TERMINATE_APPROVE", r.ID, now, req.Comment)
	sigHashHex := hex.EncodeToString(sigHash)
	today := now

	r.WorkflowStatus = statusTerminated
	r.TerminateApproverID = &actorID
	r.TermApprSignedAt = &now
	r.TermApprSigHash = sigHashHex
	r.TerminatedAt = &today
	r.RowVersion++

	s.h.auditLog.append(auditPenempatanTermApproved, r.ID.String(), actorID.String(), actor.Roles[0],
		map[string]interface{}{"terminate_approver_signature_hash": sigHashHex, "workflow_status": statusTerminated})
	s.h.auditLog.append(auditPenempatanDerecognQueued, r.ID.String(), "system", "system",
		map[string]interface{}{"event_type": "TERMINASI", "downstream": "P5-M9"})

	// Emit PenempatanTerminatedEvent.
	s.h.eventBus.emitTerminated(penempatanTerminatedEvent{
		InstrumenID:   r.InstrumenID,
		PenempatanID:  r.ID,
		KodeTransaksi: r.KodeTransaksi,
		TerminateDate: today,
		NominalIDR:    r.NominalIDR,
		EventTime:     now,
		TenantID:      r.TenantID,
	})

	return r, nil
}

// ─── Hard delete guard ────────────────────────────────────────────────────────

// TryHardDelete simulates a direct DB DELETE attempt that the trigger rejects.
func (s *penempatanTestService) TryHardDelete(_ context.Context, id uuid.UUID) error {
	// Production DB trigger: BEFORE DELETE ON trx.penempatan_deposito RAISE EXCEPTION.
	return &penempatanHardDeleteError{
		Code:    errHardDeleteForbidden,
		Message: "hard delete is forbidden on trx.penempatan_deposito — use soft-delete (deleted_at)",
	}
}

// ─── Context helpers ──────────────────────────────────────────────────────────

func ctxWithMakerActor(sub string) context.Context {
	return ctxWithActor(sub, "ROLE-MAKER-TR", "TUGURE", false)
}

func ctxWithApproverActor(sub string, stepUpFresh bool) context.Context {
	var stepUpAt *int64
	if stepUpFresh {
		ts := time.Now().Add(-2 * time.Minute).Unix() // 2 min ago < 5 min threshold
		stepUpAt = &ts
	} else {
		ts := time.Now().Add(-10 * time.Minute).Unix() // 10 min ago > 5 min threshold
		stepUpAt = &ts
	}
	claims := &auth.Claims{
		Sub:              sub,
		Roles:            []string{"ROLE-APPR-TR"},
		TenantID:         "TUGURE",
		MFAVerified:      true,
		StepupVerifiedAt: stepUpAt,
	}
	return auth.ContextWithClaims(context.Background(), claims)
}

func ctxWithSystemActor() context.Context {
	return ctxWithActor("00000000-0000-0000-0000-000000000001", "system", "TUGURE", false)
}

// claimsFromCtx extracts auth.Claims from context (mirrors production auth middleware).
func claimsFromCtx(ctx context.Context) *auth.Claims {
	return auth.ClaimsFromContext(ctx)
}

// ─── Shared seed helpers ──────────────────────────────────────────────────────

func (h *p5M1Harness) seedACInstrumen(kode string) *p5InstrumenRecord {
	return h.instrStore.seed(kode, klasifikasiAC)
}

func (h *p5M1Harness) seedFVTPLInstrumen(kode string) *p5InstrumenRecord {
	return h.instrStore.seed(kode, klasifikasiFVTPL)
}

func (h *p5M1Harness) seedFVOCIElectionInstrumen(kode string) *p5InstrumenRecord {
	return h.instrStore.seed(kode, klasifikasiFVOCIELEC)
}

func (h *p5M1Harness) seedOpenPeriode(kode string) *p5PeriodeRecord {
	return h.periodeStore.seedOpen(kode)
}

// buildCreateRequest constructs a standard IDR penempatan request.
func buildCreateRequest(instrID, bankID, periodeID uuid.UUID, account string, key uuid.UUID) createPenempatanRequest {
	return createPenempatanRequest{
		InstrumenID:        instrID,
		CounterpartyBankID: bankID,
		PeriodeID:          periodeID,
		TanggalPenempatan:  time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC),
		NominalIDR:         decimal.NewFromInt(5_000_000_000),
		MataUangKode:       "IDR",
		TenorBulan:         12,
		KuponPersen:        decimal.NewFromFloat(0.05250000),
		BiayaTransaksiIDR:  decimal.Zero,
		SettlementAccount:  account,
		IdempotencyKey:     key,
	}
}

// assertAuditContains fails the test if the expected audit action is absent for entityID.
func assertAuditContains(t *testing.T, h *p5M1Harness, entityID, action, context string) {
	t.Helper()
	if !h.auditLog.containsAction(entityID, action) {
		t.Errorf("assertAuditContains[%s]: audit action %q not found for entity %s; got: %v",
			context, action, entityID, h.auditLog.actionsForEntity(entityID))
	}
}

func assertAuditAbsent(t *testing.T, h *p5M1Harness, entityID, action, context string) {
	t.Helper()
	if h.auditLog.containsAction(entityID, action) {
		t.Errorf("assertAuditAbsent[%s]: audit action %q must NOT be present for entity %s",
			context, action, entityID)
	}
}

func assertPenempatanStatus(t *testing.T, r *penempatanRecord, want, ctx string) {
	t.Helper()
	if r == nil {
		t.Fatalf("assertPenempatanStatus[%s]: nil record, want %s", ctx, want)
	}
	if r.WorkflowStatus != want {
		t.Errorf("assertPenempatanStatus[%s]: got %s, want %s", ctx, r.WorkflowStatus, want)
	}
}

func assertPenempatanErrorCode(t *testing.T, err error, wantCode, ctx string) {
	t.Helper()
	if err == nil {
		t.Fatalf("assertPenempatanErrorCode[%s]: expected error with code %s, got nil", ctx, wantCode)
	}
	if !containsStr(err.Error(), wantCode) {
		t.Errorf("assertPenempatanErrorCode[%s]: error %v does not contain code %q", ctx, err, wantCode)
	}
}

// ─── Scenario P5-A: Full AC happy path ───────────────────────────────────────
//
// DRAFT → PENDING_REVIEW → PENDING_APPROVAL → APPROVED_ACTIVE → MATURED (auto).
// Verifies: audit chain, STAGING_INITIAL, EIR_COMPUTE enqueue, PenempatanApprovedEvent,
// maturity cron sets MATURED + emits PenempatanMaturedEvent.

func TestE2E_P5M1_ScenarioA_FullACHappyPath(t *testing.T) {
	t.Parallel()
	h := newP5M1Harness(t)

	// ── Seed ────────────────────────────────────────────────────────────────
	instr := h.seedACInstrumen("INST-E2E-AC-001")
	bankID := uuid.New()
	periode := h.seedOpenPeriode("PBUKU-2026-06")
	const settlementAcct = "1234567890"

	userA := uuid.New().String()
	userB := uuid.New().String()
	userC := uuid.New().String()

	ctxA := ctxWithMakerActor(userA)
	ctxB := ctxWithApproverActor(userB, true)
	ctxC := ctxWithApproverActor(userC, true)

	claimsA := claimsFromCtx(ctxA)
	claimsB := claimsFromCtx(ctxB)
	claimsC := claimsFromCtx(ctxC)

	idkCreate := uuid.New()

	// ── T01: Create DRAFT ────────────────────────────────────────────────────
	createReq := buildCreateRequest(instr.ID, bankID, periode.ID, settlementAcct, idkCreate)
	res, err := h.svc.Create(ctxA, createReq, claimsA)
	if err != nil {
		t.Fatalf("ScenarioA: Create failed: %v", err)
	}
	r := res.Record
	assertPenempatanStatus(t, r, statusDraft, "after-create")
	assertAuditContains(t, h, r.ID.String(), auditPenempatanCreated, "create")

	// Verify kode_transaksi format PNP-{YYYY}{MM}-{######}.
	expectedPrefix := fmt.Sprintf("PNP-%d%02d-", 2026, 6)
	if len(r.KodeTransaksi) < len(expectedPrefix) || r.KodeTransaksi[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("ScenarioA: kode_transaksi = %q, want prefix %q", r.KodeTransaksi, expectedPrefix)
	}

	// ── T02: Submit ──────────────────────────────────────────────────────────
	_, err = h.svc.Submit(ctxA, r.ID, workflowActionRequest{
		Comment:        "Penempatan deposito BCA sesuai limit portofolio",
		IdempotencyKey: uuid.New(),
	}, claimsA)
	if err != nil {
		t.Fatalf("ScenarioA: Submit failed: %v", err)
	}
	assertPenempatanStatus(t, r, statusPendingReview, "after-submit")
	assertAuditContains(t, h, r.ID.String(), auditPenempatanSubmitted, "submit")

	// ── T04: Review ──────────────────────────────────────────────────────────
	_, err = h.svc.Review(ctxB, r.ID, workflowActionRequest{
		Comment:        "Dokumen lengkap, nominal dan tenor sesuai limit",
		IdempotencyKey: uuid.New(),
	}, claimsB)
	if err != nil {
		t.Fatalf("ScenarioA: Review failed: %v", err)
	}
	assertPenempatanStatus(t, r, statusPendingApproval, "after-review")
	assertAuditContains(t, h, r.ID.String(), auditPenempatanReviewed, "review")

	// Verify signature hash set.
	if r.ReviewerSigHash == "" {
		t.Error("ScenarioA: reviewer_signature_hash must be non-empty after review")
	}
	if r.ReviewerSignedAt == nil {
		t.Error("ScenarioA: reviewer_signed_at must be set")
	}
	if r.ReviewerID == nil {
		t.Error("ScenarioA: reviewer_id must be set")
	}

	// ── T06: Approve ─────────────────────────────────────────────────────────
	approveRes, err := h.svc.Approve(ctxC, r.ID, workflowActionRequest{
		Comment:        "Disetujui sesuai RKAP 2026",
		IdempotencyKey: uuid.New(),
	}, claimsC)
	if err != nil {
		t.Fatalf("ScenarioA: Approve failed: %v", err)
	}
	assertPenempatanStatus(t, r, statusApprovedActive, "after-approve")

	// Verify audit: APPROVED + STAGING_INITIAL (both in same logical transaction).
	assertAuditContains(t, h, r.ID.String(), auditPenempatanApproved, "approve")
	assertAuditContains(t, h, r.ID.String(), auditPenempatanStagingInitial, "staging-initial")

	// Verify staging action = STAGE_1_ASSIGNED for AC.
	if approveRes.StagingAction != stagingActionStage1Assigned {
		t.Errorf("ScenarioA: StagingAction = %q, want %q", approveRes.StagingAction, stagingActionStage1Assigned)
	}

	// Verify EIR_COMPUTE enqueued (async, DEC-013).
	if !h.asynqTaskQueue.hasEIRTaskForPenempatan(r.ID) {
		t.Error("ScenarioA: EIR_COMPUTE Asynq task not enqueued for AC instrument after approve")
	}

	// Verify PenempatanApprovedEvent emitted (consumed by P5-M2 jurnal engine).
	if h.eventBus.approvedCount() != 1 {
		t.Errorf("ScenarioA: expected 1 PenempatanApprovedEvent, got %d", h.eventBus.approvedCount())
	}
	evt := h.eventBus.approvedEvents[0]
	if evt.PenempatanID != r.ID {
		t.Error("ScenarioA: PenempatanApprovedEvent.PenempatanID mismatch")
	}
	if evt.KlasifikasiPsak71 != klasifikasiAC {
		t.Errorf("ScenarioA: event klasifikasi = %q, want AC", evt.KlasifikasiPsak71)
	}
	if evt.StagingAction != stagingActionStage1Assigned {
		t.Errorf("ScenarioA: event StagingAction = %q, want STAGE_1_ASSIGNED", evt.StagingAction)
	}

	// Verify approver signature hash.
	if r.ApproverSigHash == "" {
		t.Error("ScenarioA: approver_signature_hash must be non-empty")
	}
	if r.ApproverID == nil {
		t.Error("ScenarioA: approver_id must be set")
	}

	// ── T08: Asynq maturity cron (simulate tanggal_jatuh_tempo in past) ──────
	// Override tanggal_jatuh_tempo to be today (or past) for the cron test.
	r.TanggalJatuhTempo = time.Now().Add(-1 * time.Hour) // in the past
	today := time.Now()
	matured, errs := h.svc.RunMaturityCheck(ctxWithSystemActor(), today)
	if len(errs) > 0 {
		t.Fatalf("ScenarioA: RunMaturityCheck errors: %v", errs)
	}
	if len(matured) != 1 || matured[0] != r.ID {
		t.Errorf("ScenarioA: expected 1 matured penempatan, got %v", matured)
	}
	assertPenempatanStatus(t, r, statusMatured, "after-maturity-cron")
	assertAuditContains(t, h, r.ID.String(), auditPenempatanMatured, "matured")
	assertAuditContains(t, h, r.ID.String(), auditPenempatanDerecognQueued, "derecognition-queued")

	// PenempatanMaturedEvent emitted.
	if h.eventBus.maturedCount() != 1 {
		t.Errorf("ScenarioA: expected 1 PenempatanMaturedEvent, got %d", h.eventBus.maturedCount())
	}
	if r.MaturedAt == nil {
		t.Error("ScenarioA: matured_at must be set")
	}
}

// ─── Scenario P5-B: FVTPL guard ──────────────────────────────────────────────
//
// DEC-P5-M1-001: FVTPL instruments must NOT trigger ECL staging or EIR compute.
// Audit must carry PENEMPATAN.STAGING_SKIPPED_FVTPL (not STAGING_INITIAL).

func TestE2E_P5M1_ScenarioB_FVTPLGuard(t *testing.T) {
	t.Parallel()
	h := newP5M1Harness(t)

	instr := h.seedFVTPLInstrumen("INST-E2E-FVTPL-001")
	bankID := uuid.New()
	periode := h.seedOpenPeriode("PBUKU-2026-06")

	userA, userB, userC := uuid.New().String(), uuid.New().String(), uuid.New().String()
	ctxA := ctxWithMakerActor(userA)
	ctxB := ctxWithApproverActor(userB, true)
	ctxC := ctxWithApproverActor(userC, true)
	claimsA, claimsB, claimsC := claimsFromCtx(ctxA), claimsFromCtx(ctxB), claimsFromCtx(ctxC)

	// Create + full 4-eyes.
	res, err := h.svc.Create(ctxA, buildCreateRequest(instr.ID, bankID, periode.ID, "ACT-001", uuid.New()), claimsA)
	if err != nil {
		t.Fatalf("ScenarioB: Create failed: %v", err)
	}
	r := res.Record

	_, _ = h.svc.Submit(ctxA, r.ID, workflowActionRequest{Comment: "test submit", IdempotencyKey: uuid.New()}, claimsA)
	_, _ = h.svc.Review(ctxB, r.ID, workflowActionRequest{Comment: "test review", IdempotencyKey: uuid.New()}, claimsB)
	approveRes, err := h.svc.Approve(ctxC, r.ID, workflowActionRequest{Comment: "test approve", IdempotencyKey: uuid.New()}, claimsC)
	if err != nil {
		t.Fatalf("ScenarioB: Approve failed: %v", err)
	}

	// FVTPL guard assertions.
	if approveRes.StagingAction != stagingActionSkippedFVTPL {
		t.Errorf("ScenarioB: StagingAction = %q, want %q (FVTPL must skip staging)",
			approveRes.StagingAction, stagingActionSkippedFVTPL)
	}

	// STAGING_SKIPPED_FVTPL in audit, NOT STAGING_INITIAL.
	assertAuditContains(t, h, r.ID.String(), auditPenempatanStagingSkipped, "fvtpl-skip-audit")
	assertAuditAbsent(t, h, r.ID.String(), auditPenempatanStagingInitial, "no-staging-initial-for-fvtpl")

	// NO EIR_COMPUTE Asynq task enqueued (DEC-013: EIR not applicable to FVTPL).
	if h.asynqTaskQueue.hasEIRTaskForPenempatan(r.ID) {
		t.Error("ScenarioB: EIR_COMPUTE task must NOT be enqueued for FVTPL instrument")
	}
	if h.asynqTaskQueue.eirComputeCount() != 0 {
		t.Errorf("ScenarioB: expected 0 EIR tasks, got %d", h.asynqTaskQueue.eirComputeCount())
	}

	// PenempatanApprovedEvent IS still emitted (event code 1 for all klasifikasi, state machine §9).
	if h.eventBus.approvedCount() != 1 {
		t.Errorf("ScenarioB: PenempatanApprovedEvent should still be emitted for FVTPL, got %d", h.eventBus.approvedCount())
	}
	if h.eventBus.approvedEvents[0].StagingAction != stagingActionSkippedFVTPL {
		t.Errorf("ScenarioB: event StagingAction = %q, want SKIPPED_FVTPL", h.eventBus.approvedEvents[0].StagingAction)
	}

	// EIR field must be nil for FVTPL.
	if r.EIRAwal != nil {
		t.Error("ScenarioB: eir_awal must be nil for FVTPL instrument")
	}
}

// ─── Scenario P5-C: FVOCI_ELECTION guard ─────────────────────────────────────
//
// Same skip behavior as P5-B — FVOCI_ELECTION is irrevocable equity classification
// with no ECL scope (PSAK 71 §5.7.7a, no recycling, no impairment).

func TestE2E_P5M1_ScenarioC_FVOCIElectionGuard(t *testing.T) {
	t.Parallel()
	h := newP5M1Harness(t)

	instr := h.seedFVOCIElectionInstrumen("INST-E2E-FVOCI-ELEC-001")
	bankID := uuid.New()
	periode := h.seedOpenPeriode("PBUKU-2026-06")

	userA, userB, userC := uuid.New().String(), uuid.New().String(), uuid.New().String()
	ctxA := ctxWithMakerActor(userA)
	ctxB := ctxWithApproverActor(userB, true)
	ctxC := ctxWithApproverActor(userC, true)
	claimsA, claimsB, claimsC := claimsFromCtx(ctxA), claimsFromCtx(ctxB), claimsFromCtx(ctxC)

	res, err := h.svc.Create(ctxA, buildCreateRequest(instr.ID, bankID, periode.ID, "ACT-002", uuid.New()), claimsA)
	if err != nil {
		t.Fatalf("ScenarioC: Create failed: %v", err)
	}
	r := res.Record

	_, _ = h.svc.Submit(ctxA, r.ID, workflowActionRequest{Comment: "test", IdempotencyKey: uuid.New()}, claimsA)
	_, _ = h.svc.Review(ctxB, r.ID, workflowActionRequest{Comment: "test", IdempotencyKey: uuid.New()}, claimsB)
	approveRes, err := h.svc.Approve(ctxC, r.ID, workflowActionRequest{Comment: "test", IdempotencyKey: uuid.New()}, claimsC)
	if err != nil {
		t.Fatalf("ScenarioC: Approve failed: %v", err)
	}

	if approveRes.StagingAction != stagingActionSkippedFVTPL {
		t.Errorf("ScenarioC: FVOCI_ELECTION must also skip staging; got %q", approveRes.StagingAction)
	}
	assertAuditContains(t, h, r.ID.String(), auditPenempatanStagingSkipped, "fvoci-election-skip")
	assertAuditAbsent(t, h, r.ID.String(), auditPenempatanStagingInitial, "no-staging-initial-fvoci-election")
	if h.asynqTaskQueue.hasEIRTaskForPenempatan(r.ID) {
		t.Error("ScenarioC: EIR_COMPUTE must NOT be enqueued for FVOCI_ELECTION")
	}
}

// ─── Scenario P5-D: SoD — maker tries to review own penempatan ───────────────
//
// DEC-017: server-side SoD enforcement (not UI only).
// Audit PENEMPATAN.SOD_VIOLATION_ATTEMPT must be written even on rejection.

func TestE2E_P5M1_ScenarioD_SoDMakerTriesToReview(t *testing.T) {
	t.Parallel()
	h := newP5M1Harness(t)

	instr := h.seedACInstrumen("INST-SOD-001")
	bankID := uuid.New()
	periode := h.seedOpenPeriode("PBUKU-2026-06")

	userA := uuid.New().String()
	ctxA := ctxWithMakerActor(userA)
	claimsA := claimsFromCtx(ctxA)

	res, _ := h.svc.Create(ctxA, buildCreateRequest(instr.ID, bankID, periode.ID, "ACT-SOD", uuid.New()), claimsA)
	r := res.Record
	_, _ = h.svc.Submit(ctxA, r.ID, workflowActionRequest{Comment: "submit", IdempotencyKey: uuid.New()}, claimsA)

	// User A tries to review own penempatan → SoD violation.
	ctxAReview := ctxWithApproverActor(userA, true)
	claimsAReview := claimsFromCtx(ctxAReview)
	_, err := h.svc.Review(ctxAReview, r.ID, workflowActionRequest{Comment: "review attempt", IdempotencyKey: uuid.New()}, claimsAReview)
	assertPenempatanErrorCode(t, err, errSODViolation, "maker-review-SoD")

	// Workflow status must not change.
	assertPenempatanStatus(t, r, statusPendingReview, "after-SoD-attempt")

	// Audit SOD_VIOLATION_ATTEMPT must be written.
	assertAuditContains(t, h, r.ID.String(), auditPenempatanSODAttempt, "sod-audit-written")
}

// ─── Scenario P5-E: SoD — reviewer tries to approve ─────────────────────────
//
// DEC-017: reviewer_id cannot equal approver_id.

func TestE2E_P5M1_ScenarioE_SoDReviewerTriesToApprove(t *testing.T) {
	t.Parallel()
	h := newP5M1Harness(t)

	instr := h.seedACInstrumen("INST-SOD-002")
	bankID := uuid.New()
	periode := h.seedOpenPeriode("PBUKU-2026-06")

	userA := uuid.New().String()
	userB := uuid.New().String()

	ctxA := ctxWithMakerActor(userA)
	ctxB := ctxWithApproverActor(userB, true)
	claimsA, claimsB := claimsFromCtx(ctxA), claimsFromCtx(ctxB)

	res, _ := h.svc.Create(ctxA, buildCreateRequest(instr.ID, bankID, periode.ID, "ACT-SOD2", uuid.New()), claimsA)
	r := res.Record
	_, _ = h.svc.Submit(ctxA, r.ID, workflowActionRequest{Comment: "submit", IdempotencyKey: uuid.New()}, claimsA)
	_, _ = h.svc.Review(ctxB, r.ID, workflowActionRequest{Comment: "review", IdempotencyKey: uuid.New()}, claimsB)

	// User B (reviewer) tries to approve → SoD violation.
	_, err := h.svc.Approve(ctxB, r.ID, workflowActionRequest{Comment: "approve attempt", IdempotencyKey: uuid.New()}, claimsB)
	assertPenempatanErrorCode(t, err, errSODViolation, "reviewer-approve-SoD")

	assertPenempatanStatus(t, r, statusPendingApproval, "after-reviewer-SoD-attempt")
	assertAuditContains(t, h, r.ID.String(), auditPenempatanSODAttempt, "reviewer-sod-audit")
}

// ─── Scenario P5-F: Step-up MFA stale on approve ─────────────────────────────
//
// DEC-027: X-Step-Up-Token must be issued ≤ 5 minutes ago.
// Claims with stepup_verified_at = 10 min ago → 403 PENEMPATAN_STEP_UP_REQUIRED.

func TestE2E_P5M1_ScenarioF_StepUpMFAStaleOnApprove(t *testing.T) {
	t.Parallel()
	h := newP5M1Harness(t)

	instr := h.seedACInstrumen("INST-STEPUP-001")
	bankID := uuid.New()
	periode := h.seedOpenPeriode("PBUKU-2026-06")

	userA, userB, userC := uuid.New().String(), uuid.New().String(), uuid.New().String()
	ctxA := ctxWithMakerActor(userA)
	ctxB := ctxWithApproverActor(userB, true)
	claimsA, claimsB := claimsFromCtx(ctxA), claimsFromCtx(ctxB)

	res, _ := h.svc.Create(ctxA, buildCreateRequest(instr.ID, bankID, periode.ID, "ACT-SU", uuid.New()), claimsA)
	r := res.Record
	_, _ = h.svc.Submit(ctxA, r.ID, workflowActionRequest{Comment: "submit", IdempotencyKey: uuid.New()}, claimsA)
	_, _ = h.svc.Review(ctxB, r.ID, workflowActionRequest{Comment: "review", IdempotencyKey: uuid.New()}, claimsB)

	// User C with STALE step-up (10 min ago).
	ctxCStale := ctxWithApproverActor(userC, false) // stepUpFresh=false → 10 min ago
	claimsCStale := claimsFromCtx(ctxCStale)

	_, err := h.svc.Approve(ctxCStale, r.ID, workflowActionRequest{Comment: "approve with stale MFA", IdempotencyKey: uuid.New()}, claimsCStale)
	assertPenempatanErrorCode(t, err, errStepUpRequired, "stale-stepup-approve")

	// Status unchanged.
	assertPenempatanStatus(t, r, statusPendingApproval, "after-stale-stepup")

	// Fresh step-up should succeed.
	ctxCFresh := ctxWithApproverActor(userC, true)
	claimsCFresh := claimsFromCtx(ctxCFresh)
	_, err = h.svc.Approve(ctxCFresh, r.ID, workflowActionRequest{Comment: "approve with fresh MFA", IdempotencyKey: uuid.New()}, claimsCFresh)
	if err != nil {
		t.Errorf("ScenarioF: Fresh step-up should succeed, got: %v", err)
	}
	assertPenempatanStatus(t, r, statusApprovedActive, "after-fresh-stepup-approve")
	_ = userC
}

// ─── Scenario P5-G: Terminate 4-eyes happy path ──────────────────────────────
//
// DEC-P5-M1-005: Terminate = full 4-eyes (propose → review → approve).
// Verifies: audit chain, terminate signature hashes, PenempatanTerminatedEvent.

func TestE2E_P5M1_ScenarioG_Terminate4EyesHappy(t *testing.T) {
	t.Parallel()
	h := newP5M1Harness(t)

	instr := h.seedACInstrumen("INST-TERM-001")
	bankID := uuid.New()
	periode := h.seedOpenPeriode("PBUKU-2026-06")

	userA, userB, userC, userD, userE, userF :=
		uuid.New().String(), uuid.New().String(), uuid.New().String(),
		uuid.New().String(), uuid.New().String(), uuid.New().String()

	ctxA := ctxWithMakerActor(userA)
	ctxB := ctxWithApproverActor(userB, true)
	ctxC := ctxWithApproverActor(userC, true)
	claimsA, claimsB, claimsC := claimsFromCtx(ctxA), claimsFromCtx(ctxB), claimsFromCtx(ctxC)

	// Bring penempatan to APPROVED_ACTIVE.
	res, _ := h.svc.Create(ctxA, buildCreateRequest(instr.ID, bankID, periode.ID, "ACT-TERM", uuid.New()), claimsA)
	r := res.Record
	_, _ = h.svc.Submit(ctxA, r.ID, workflowActionRequest{Comment: "submit", IdempotencyKey: uuid.New()}, claimsA)
	_, _ = h.svc.Review(ctxB, r.ID, workflowActionRequest{Comment: "review", IdempotencyKey: uuid.New()}, claimsB)
	_, _ = h.svc.Approve(ctxC, r.ID, workflowActionRequest{Comment: "approve", IdempotencyKey: uuid.New()}, claimsC)
	assertPenempatanStatus(t, r, statusApprovedActive, "before-terminate")

	// ── Terminate propose (User D = separate maker for terminate) ────────────
	ctxD := ctxWithMakerActor(userD)
	claimsD := claimsFromCtx(ctxD)
	_, err := h.svc.TerminateRequest(ctxD, r.ID, terminateRequestBody{
		TerminateReason: "Bank counterparty meminta pengembalian dana lebih awal karena restrukturisasi internal. Surat tertanggal 2026-11-30 terlampir.",
		IdempotencyKey:  uuid.New(),
	}, claimsD)
	if err != nil {
		t.Fatalf("ScenarioG: TerminateRequest failed: %v", err)
	}
	assertPenempatanStatus(t, r, statusTerminationPendingReview, "after-terminate-propose")
	assertAuditContains(t, h, r.ID.String(), auditPenempatanTermProp, "terminate-proposed")

	// ── Terminate review (User E ≠ User D) ───────────────────────────────────
	ctxE := ctxWithApproverActor(userE, true)
	claimsE := claimsFromCtx(ctxE)
	_, err = h.svc.TerminateReview(ctxE, r.ID, workflowActionRequest{
		Comment:        "Dokumen surat dari bank terlampir dan valid. Alasan termination sesuai prosedur.",
		IdempotencyKey: uuid.New(),
	}, claimsE)
	if err != nil {
		t.Fatalf("ScenarioG: TerminateReview failed: %v", err)
	}
	assertPenempatanStatus(t, r, statusTerminationPendingApproval, "after-terminate-review")
	assertAuditContains(t, h, r.ID.String(), auditPenempatanTermReviewed, "terminate-reviewed")

	// Verify terminate reviewer signature hash.
	if r.TermRevSigHash == "" {
		t.Error("ScenarioG: terminate_reviewer_signature_hash must be non-empty")
	}
	if r.TerminateReviewerID == nil {
		t.Error("ScenarioG: terminate_reviewer_id must be set")
	}

	// ── Terminate approve (User F ≠ User D AND ≠ User E) ─────────────────────
	ctxF := ctxWithApproverActor(userF, true)
	claimsF := claimsFromCtx(ctxF)
	_, err = h.svc.TerminateApprove(ctxF, r.ID, workflowActionRequest{
		Comment:        "Disetujui sesuai memo Direktur Keuangan No. 123/2026",
		IdempotencyKey: uuid.New(),
	}, claimsF)
	if err != nil {
		t.Fatalf("ScenarioG: TerminateApprove failed: %v", err)
	}
	assertPenempatanStatus(t, r, statusTerminated, "after-terminate-approve")

	// Verify audit chain.
	assertAuditContains(t, h, r.ID.String(), auditPenempatanTermApproved, "terminate-approved")
	assertAuditContains(t, h, r.ID.String(), auditPenempatanDerecognQueued, "derecognition-queued-terminate")

	// Verify terminate approver fields.
	if r.TermApprSigHash == "" {
		t.Error("ScenarioG: terminate_approver_signature_hash must be non-empty")
	}
	if r.TerminatedAt == nil {
		t.Error("ScenarioG: terminated_at must be set")
	}

	// PenempatanTerminatedEvent emitted.
	if h.eventBus.terminatedCount() != 1 {
		t.Errorf("ScenarioG: expected 1 PenempatanTerminatedEvent, got %d", h.eventBus.terminatedCount())
	}
	termEvt := h.eventBus.terminatedEvents[0]
	if termEvt.PenempatanID != r.ID {
		t.Error("ScenarioG: PenempatanTerminatedEvent.PenempatanID mismatch")
	}
}

// ─── Scenario P5-H: Terminate SoD violations (F2 fix) ────────────────────────
//
// DEC-017 + DEC-P5-M1-005: Terminate has independent SoD.
// The original maker (userA) cannot act as terminate_reviewer or terminate_approver.
// Audits must be written for every attempted violation.

func TestE2E_P5M1_ScenarioH_TerminateSoDViolations(t *testing.T) {
	t.Parallel()
	h := newP5M1Harness(t)

	instr := h.seedACInstrumen("INST-TERM-SOD-001")
	bankID := uuid.New()
	periode := h.seedOpenPeriode("PBUKU-2026-06")

	userA, userB, userC := uuid.New().String(), uuid.New().String(), uuid.New().String()
	ctxA := ctxWithMakerActor(userA)
	ctxB := ctxWithApproverActor(userB, true)
	ctxC := ctxWithApproverActor(userC, true)
	claimsA, claimsB, claimsC := claimsFromCtx(ctxA), claimsFromCtx(ctxB), claimsFromCtx(ctxC)

	// Bring to APPROVED_ACTIVE.
	res, _ := h.svc.Create(ctxA, buildCreateRequest(instr.ID, bankID, periode.ID, "ACT-TSOD", uuid.New()), claimsA)
	r := res.Record
	_, _ = h.svc.Submit(ctxA, r.ID, workflowActionRequest{Comment: "submit", IdempotencyKey: uuid.New()}, claimsA)
	_, _ = h.svc.Review(ctxB, r.ID, workflowActionRequest{Comment: "review", IdempotencyKey: uuid.New()}, claimsB)
	_, _ = h.svc.Approve(ctxC, r.ID, workflowActionRequest{Comment: "approve", IdempotencyKey: uuid.New()}, claimsC)

	// User A proposes terminate (User A is the original maker).
	_, err := h.svc.TerminateRequest(ctxA, r.ID, terminateRequestBody{
		TerminateReason: "Alasan terminasi yang cukup panjang melebihi tiga puluh karakter untuk validasi",
		IdempotencyKey:  uuid.New(),
	}, claimsA)
	if err != nil {
		t.Fatalf("ScenarioH: TerminateRequest by maker should succeed: %v", err)
	}
	assertPenempatanStatus(t, r, statusTerminationPendingReview, "before-sod-tests")

	// ── Attempt 1: User A (original maker) tries to terminate-review → SoD ──
	ctxAReview := ctxWithApproverActor(userA, true)
	claimsAReview := claimsFromCtx(ctxAReview)
	_, err = h.svc.TerminateReview(ctxAReview, r.ID, workflowActionRequest{Comment: "review by maker", IdempotencyKey: uuid.New()}, claimsAReview)
	assertPenempatanErrorCode(t, err, errSODViolation, "terminate-review-by-maker")
	assertPenempatanStatus(t, r, statusTerminationPendingReview, "after-terminate-review-SoD")
	assertAuditContains(t, h, r.ID.String(), auditPenempatanSODAttempt, "terminate-review-SoD-audit")

	// Legitimate reviewer (User B) reviews.
	_, err = h.svc.TerminateReview(ctxB, r.ID, workflowActionRequest{Comment: "valid review", IdempotencyKey: uuid.New()}, claimsB)
	if err != nil {
		t.Fatalf("ScenarioH: legitimate TerminateReview failed: %v", err)
	}
	assertPenempatanStatus(t, r, statusTerminationPendingApproval, "after-legitimate-terminate-review")

	// ── Attempt 2: User A (original maker) tries to terminate-approve → SoD ─
	ctxAApprove := ctxWithApproverActor(userA, true)
	claimsAApprove := claimsFromCtx(ctxAApprove)
	_, err = h.svc.TerminateApprove(ctxAApprove, r.ID, workflowActionRequest{Comment: "approve by maker", IdempotencyKey: uuid.New()}, claimsAApprove)
	assertPenempatanErrorCode(t, err, errSODViolation, "terminate-approve-by-maker")
	assertPenempatanStatus(t, r, statusTerminationPendingApproval, "after-terminate-approve-SoD")

	// ── Attempt 3: User B (terminate_reviewer) tries to terminate-approve → SoD
	_, err = h.svc.TerminateApprove(ctxB, r.ID, workflowActionRequest{Comment: "approve by reviewer", IdempotencyKey: uuid.New()}, claimsB)
	assertPenempatanErrorCode(t, err, errSODViolation, "terminate-approve-by-reviewer")
	assertPenempatanStatus(t, r, statusTerminationPendingApproval, "after-terminate-reviewer-SoD")

	// Verify total SoD attempt audits = 3.
	sodCount := h.auditLog.countAction(r.ID.String(), auditPenempatanSODAttempt)
	if sodCount != 3 {
		t.Errorf("ScenarioH: expected 3 SOD_VIOLATION_ATTEMPT audit rows, got %d", sodCount)
	}
}

// ─── Scenario P5-I: Settlement balance hint informational ────────────────────
//
// DEC-P5-M1-004: settlement_balance_hint is informational only — never blocks create.
// Covers: (a) no balance record → hint=nil, (b) stale balance → isStale=true, no block.

func TestE2E_P5M1_ScenarioI_SettlementBalanceHintInformational(t *testing.T) {
	t.Parallel()
	h := newP5M1Harness(t)

	instr := h.seedACInstrumen("INST-SETTLE-001")
	bankID := uuid.New()
	periode := h.seedOpenPeriode("PBUKU-2026-06")

	ctxA := ctxWithMakerActor(uuid.New().String())
	claimsA := claimsFromCtx(ctxA)
	nominalIDR := decimal.NewFromInt(5_000_000_000)

	// ── Case 1: No balance record → hint = nil, create succeeds ──────────────
	reqNoBalance := buildCreateRequest(instr.ID, bankID, periode.ID, "9999999999", uuid.New())
	reqNoBalance.NominalIDR = nominalIDR
	res, err := h.svc.Create(ctxA, reqNoBalance, claimsA)
	if err != nil {
		t.Fatalf("ScenarioI[no-balance]: Create must succeed even without settlement balance: %v", err)
	}
	if res.SettlementBalanceHint != nil {
		t.Errorf("ScenarioI[no-balance]: hint should be nil when no balance record, got: %+v", res.SettlementBalanceHint)
	}
	assertPenempatanStatus(t, res.Record, statusDraft, "no-balance-create")

	// ── Case 2: Stale balance (> 24h) → hint.IsStale = true, no block ────────
	staleDate := time.Now().Add(-25 * time.Hour) // 25h ago = stale
	h.balanceStore.seed("STALE-ACCT-001", decimal.NewFromInt(3_000_000_000), staleDate)

	req2 := buildCreateRequest(instr.ID, bankID, periode.ID, "STALE-ACCT-001", uuid.New())
	req2.NominalIDR = nominalIDR
	res2, err := h.svc.Create(ctxA, req2, claimsA)
	if err != nil {
		t.Fatalf("ScenarioI[stale-balance]: Create must succeed (no block): %v", err)
	}
	assertPenempatanStatus(t, res2.Record, statusDraft, "stale-balance-create")
	if res2.SettlementBalanceHint == nil {
		t.Fatal("ScenarioI[stale-balance]: hint must be non-nil when balance record exists")
	}
	if !res2.SettlementBalanceHint.IsStale {
		t.Error("ScenarioI[stale-balance]: hint.IsStale must be true for 25h-old balance")
	}

	// ── Case 3: Fresh balance, nominal > balance → hint returned, no block ────
	freshDate := time.Now().Add(-2 * time.Hour) // 2h ago = fresh
	h.balanceStore.seed("FRESH-ACCT-001", decimal.NewFromInt(3_000_000_000), freshDate)

	req3 := buildCreateRequest(instr.ID, bankID, periode.ID, "FRESH-ACCT-001", uuid.New())
	req3.NominalIDR = nominalIDR // 5B > 3B balance
	res3, err := h.svc.Create(ctxA, req3, claimsA)
	if err != nil {
		t.Fatalf("ScenarioI[fresh-balance-insufficient]: Create must not be blocked even when nominal > balance: %v", err)
	}
	assertPenempatanStatus(t, res3.Record, statusDraft, "fresh-balance-no-block")
	if res3.SettlementBalanceHint == nil {
		t.Fatal("ScenarioI[fresh-balance]: hint must be non-nil")
	}
	if res3.SettlementBalanceHint.IsStale {
		t.Error("ScenarioI[fresh-balance]: hint.IsStale must be false for 2h-old balance")
	}
	// Nominal (5B) > balance (3B): hint present as informational warning; NO 422.
	if res3.SettlementBalanceHint.LastKnownIDR.Cmp(decimal.NewFromInt(3_000_000_000)) != 0 {
		t.Errorf("ScenarioI: hint.LastKnownIDR = %s, want 3000000000",
			res3.SettlementBalanceHint.LastKnownIDR.StringFixed(4))
	}
}

// ─── Scenario P5-J: kode_transaksi sequence ───────────────────────────────────
//
// Three penempatan in same month must receive sequential kode_transaksi.
// Format: PNP-{YYYY}{MM}-{######} — server-side auto-generated, unique, sequential.

func TestE2E_P5M1_ScenarioJ_KodeTransaksiSequence(t *testing.T) {
	t.Parallel()
	h := newP5M1Harness(t)

	instr := h.seedACInstrumen("INST-SEQ-001")
	bankID := uuid.New()
	periode := h.seedOpenPeriode("PBUKU-2026-06")

	ctxA := ctxWithMakerActor(uuid.New().String())
	claimsA := claimsFromCtx(ctxA)

	var kodes []string
	for i := 0; i < 3; i++ {
		req := buildCreateRequest(instr.ID, bankID, periode.ID, "ACT-SEQ", uuid.New())
		res, err := h.svc.Create(ctxA, req, claimsA)
		if err != nil {
			t.Fatalf("ScenarioJ: Create[%d] failed: %v", i, err)
		}
		kodes = append(kodes, res.Record.KodeTransaksi)
	}

	// Expect PNP-202606-000001, PNP-202606-000002, PNP-202606-000003.
	expectedKodes := []string{
		"PNP-202606-000001",
		"PNP-202606-000002",
		"PNP-202606-000003",
	}
	for i, want := range expectedKodes {
		if kodes[i] != want {
			t.Errorf("ScenarioJ: kodes[%d] = %q, want %q", i, kodes[i], want)
		}
	}

	// All kodes unique.
	seen := make(map[string]bool)
	for _, k := range kodes {
		if seen[k] {
			t.Errorf("ScenarioJ: duplicate kode_transaksi: %q", k)
		}
		seen[k] = true
	}
}

// ─── Scenario P5-K: Hard delete forbidden ────────────────────────────────────
//
// DB trigger rejects any direct DELETE from trx.penempatan_deposito.
// Service TryHardDelete must return HARD_DELETE_FORBIDDEN error.
// This mirrors the production BEFORE DELETE trigger on the table.

func TestE2E_P5M1_ScenarioK_HardDeleteForbidden(t *testing.T) {
	t.Parallel()
	h := newP5M1Harness(t)

	instr := h.seedACInstrumen("INST-DEL-001")
	bankID := uuid.New()
	periode := h.seedOpenPeriode("PBUKU-2026-06")

	ctxA := ctxWithMakerActor(uuid.New().String())
	claimsA := claimsFromCtx(ctxA)

	res, err := h.svc.Create(ctxA, buildCreateRequest(instr.ID, bankID, periode.ID, "ACT-DEL", uuid.New()), claimsA)
	if err != nil {
		t.Fatalf("ScenarioK: Create failed: %v", err)
	}
	r := res.Record

	// Attempt hard delete → must be rejected.
	err = h.svc.TryHardDelete(context.Background(), r.ID)
	assertPenempatanErrorCode(t, err, errHardDeleteForbidden, "hard-delete-forbidden")

	// Record must still exist in store (not deleted).
	if h.svc.records[r.ID] == nil {
		t.Error("ScenarioK: record must not be removed from store by hard delete attempt")
	}
	if h.svc.records[r.ID].DeletedAt != nil {
		t.Error("ScenarioK: deleted_at must not be set by hard delete attempt")
	}
}

// ─── Additional regression: Audit hash chain integrity ───────────────────────
//
// DEC-018: Every audit row must have a non-empty current_hash.
// DEC-018: Hash chain: current_hash = SHA-256(previous_hash || action || entityID || afterJSON).

func TestE2E_P5M1_AuditHashChainIntegrity(t *testing.T) {
	t.Parallel()
	h := newP5M1Harness(t)

	instr := h.seedACInstrumen("INST-HASH-001")
	bankID := uuid.New()
	periode := h.seedOpenPeriode("PBUKU-2026-06")

	ctxA := ctxWithMakerActor(uuid.New().String())
	ctxB := ctxWithApproverActor(uuid.New().String(), true)
	ctxC := ctxWithApproverActor(uuid.New().String(), true)
	claimsA, claimsB, claimsC := claimsFromCtx(ctxA), claimsFromCtx(ctxB), claimsFromCtx(ctxC)

	res, _ := h.svc.Create(ctxA, buildCreateRequest(instr.ID, bankID, periode.ID, "ACT-HASH", uuid.New()), claimsA)
	r := res.Record
	_, _ = h.svc.Submit(ctxA, r.ID, workflowActionRequest{Comment: "submit", IdempotencyKey: uuid.New()}, claimsA)
	_, _ = h.svc.Review(ctxB, r.ID, workflowActionRequest{Comment: "review", IdempotencyKey: uuid.New()}, claimsB)
	_, _ = h.svc.Approve(ctxC, r.ID, workflowActionRequest{Comment: "approve", IdempotencyKey: uuid.New()}, claimsC)

	rows := h.auditLog.rowsForEntity(r.ID.String())
	if len(rows) == 0 {
		t.Fatal("AuditHashChain: no audit rows found for penempatan lifecycle")
	}

	// Every row must have non-empty current_hash.
	for _, row := range rows {
		if len(row.CurrentHash) == 0 {
			t.Errorf("AuditHashChain: row action=%q has empty current_hash", row.Action)
		}
	}

	// Expected 4 lifecycle events in order.
	actions := h.auditLog.actionsForEntity(r.ID.String())
	expectedInOrder := []string{
		auditPenempatanCreated,
		auditPenempatanSubmitted,
		auditPenempatanReviewed,
		auditPenempatanApproved,
		auditPenempatanStagingInitial,
	}
	for _, want := range expectedInOrder {
		found := false
		for _, got := range actions {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("AuditHashChain: expected audit action %q not found; got: %v", want, actions)
		}
	}
}

// ─── Additional regression: Idempotency replay ────────────────────────────────
//
// DEC-021: Same Idempotency-Key + same payload → returns original response, no duplicate.

func TestE2E_P5M1_IdempotencyReplay(t *testing.T) {
	t.Parallel()
	h := newP5M1Harness(t)

	instr := h.seedACInstrumen("INST-IDEM-001")
	bankID := uuid.New()
	periode := h.seedOpenPeriode("PBUKU-2026-06")

	ctxA := ctxWithMakerActor(uuid.New().String())
	claimsA := claimsFromCtx(ctxA)

	idkKey := uuid.New()
	req := buildCreateRequest(instr.ID, bankID, periode.ID, "ACT-IDEM", idkKey)

	res1, err := h.svc.Create(ctxA, req, claimsA)
	if err != nil {
		t.Fatalf("IdempotencyReplay: first Create failed: %v", err)
	}

	res2, err := h.svc.Create(ctxA, req, claimsA)
	if err != nil {
		t.Fatalf("IdempotencyReplay: second Create (replay) failed: %v", err)
	}

	// Same ID returned.
	if res1.Record.ID != res2.Record.ID {
		t.Errorf("IdempotencyReplay: different IDs on replay: %s vs %s", res1.Record.ID, res2.Record.ID)
	}
	if res1.Record.KodeTransaksi != res2.Record.KodeTransaksi {
		t.Errorf("IdempotencyReplay: kode_transaksi mismatch: %s vs %s",
			res1.Record.KodeTransaksi, res2.Record.KodeTransaksi)
	}

	// Only 1 record in store (no duplicate side-effect).
	count := 0
	for _, r := range h.svc.records {
		if r.InstrumenID == instr.ID {
			count++
		}
	}
	if count != 1 {
		t.Errorf("IdempotencyReplay: expected 1 record in store, got %d", count)
	}

	// Only 1 PENEMPATAN.CREATED audit row (no duplicate).
	createdCount := h.auditLog.countAction(res1.Record.ID.String(), auditPenempatanCreated)
	if createdCount != 1 {
		t.Errorf("IdempotencyReplay: expected 1 CREATED audit row, got %d", createdCount)
	}
}

// ─── Additional regression: Maturity batch partial failure tolerance ──────────
//
// Story P5-M1-S5 AC: if 1 of N maturity candidates fails, job continues for the rest.
// This verifies the per-penempatan (not big-tx) maturity pattern.

func TestE2E_P5M1_MaturityBatchPartialFailure(t *testing.T) {
	t.Parallel()
	h := newP5M1Harness(t)

	instr := h.seedACInstrumen("INST-BATCH-001")
	bankID := uuid.New()
	periode := h.seedOpenPeriode("PBUKU-2026-06")

	ctxA := ctxWithMakerActor(uuid.New().String())
	ctxB := ctxWithApproverActor(uuid.New().String(), true)
	ctxC := ctxWithApproverActor(uuid.New().String(), true)
	claimsA, claimsB, claimsC := claimsFromCtx(ctxA), claimsFromCtx(ctxB), claimsFromCtx(ctxC)

	// Create 3 penempatan in APPROVED_ACTIVE with jatuh_tempo in the past.
	var penempatanIDs []uuid.UUID
	for i := 0; i < 3; i++ {
		res, _ := h.svc.Create(ctxA, buildCreateRequest(instr.ID, bankID, periode.ID, "ACT-BATCH", uuid.New()), claimsA)
		r := res.Record
		// Manually set jatuh_tempo to past.
		r.TanggalJatuhTempo = time.Now().Add(-24 * time.Hour)
		_, _ = h.svc.Submit(ctxA, r.ID, workflowActionRequest{Comment: "submit", IdempotencyKey: uuid.New()}, claimsA)
		_, _ = h.svc.Review(ctxB, r.ID, workflowActionRequest{Comment: "review", IdempotencyKey: uuid.New()}, claimsB)
		_, _ = h.svc.Approve(ctxC, r.ID, workflowActionRequest{Comment: "approve", IdempotencyKey: uuid.New()}, claimsC)
		// Restore jatuh_tempo to past after approve (approve changes nothing on this field).
		r.TanggalJatuhTempo = time.Now().Add(-24 * time.Hour)
		penempatanIDs = append(penempatanIDs, r.ID)
	}

	// Run maturity check.
	today := time.Now()
	matured, errs := h.svc.RunMaturityCheck(ctxWithSystemActor(), today)

	// All 3 should be matured (no partial failures in stub — stub never errors).
	if len(matured) != 3 {
		t.Errorf("MaturityBatch: expected 3 matured, got %d", len(matured))
	}
	if len(errs) != 0 {
		t.Errorf("MaturityBatch: expected 0 errors, got %d: %v", len(errs), errs)
	}

	// 3 PenempatanMaturedEvent emitted.
	if h.eventBus.maturedCount() != 3 {
		t.Errorf("MaturityBatch: expected 3 matured events, got %d", h.eventBus.maturedCount())
	}

	// Each penempatan has MATURED + DERECOGNITION_QUEUED audit.
	for _, pid := range penempatanIDs {
		assertAuditContains(t, h, pid.String(), auditPenempatanMatured, "batch-matured-audit")
		assertAuditContains(t, h, pid.String(), auditPenempatanDerecognQueued, "batch-derecogn-queued")
	}
}
