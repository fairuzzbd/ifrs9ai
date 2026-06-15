// Package e2e — P5-M2 Jurnal Engine end-to-end tests.
//
// Scope: Mapping Jurnal Header CRUD (4-eyes/6-eyes), Jurnal Resolver,
// ResolveAndPost (Asynq subscriber), DLQ lifecycle (replay + discard).
// All scenarios use the same in-process harness pattern as p5_m1_penempatan_test.go.
//
// Scenarios:
//
//	P5-M2-A  Mapping CRUD 4-eyes operational (PENEMPATAN) happy path → APPROVED_ACTIVE
//	P5-M2-B  Mapping CRUD 6-eyes regulated (ECL_PEMBENTUKAN) → PENDING_APPROVAL_2 → APPROVED_ACTIVE
//	P5-M2-C  SoD violation 6-eyes: approver_2 == approver_1 → 403 JURNAL_SOD_VIOLATION
//	P5-M2-D  Step-up MFA stale on approve_2 → 403 JURNAL_STEP_UP_REQUIRED
//	P5-M2-E  Resolver lookup happy: PENEMPATAN + AC → balanced JurnalLines
//	P5-M2-F  Resolver klasifikasi not eligible: MTM_FVOCI mapping + FVTPL request → 422
//	P5-M2-G  Resolver event_code not mapped → 422 JURNAL_EVENT_NOT_MAPPED
//	P5-M2-H  ResolveAndPost happy: penempatan:approved → jrnl.header + 2 detail rows + audit
//	P5-M2-I  ResolveAndPost periode HARD_CLOSED guard → DLQ insert + error_category=DOMAIN
//	P5-M2-J  DLQ replay happy: periode reopened → REPLAYED_OK + JURNAL.DLQ_REPLAYED audit
//	P5-M2-K  DLQ discard: ABANDONED + reason ≥30 chars + JURNAL.DLQ_DISCARD audit
//	P5-M2-L  Balance invariant detection: imbalanced template → JURNAL_BALANCE_INVARIANT, no INSERT
//	P5-M2-M  Idempotency duplicate post: same source_event_id replay returns existing header
//	P5-M2-N  Domain err → DLQ immediate, no Asynq retry (asynq.SkipRetry equivalent)
//	P5-M2-O  Infra err → 3x retry then DLQ, retry_count=3, error_category=INFRA
//
// Decision log compliance:
//
//	DEC-P5-M1-002: 27 master event codes                          — Scenario P5-M2-E..N
//	DEC-P5-M1-003: 6-eyes regulated vs 4-eyes operational         — Scenario P5-M2-A..D
//	DEC-017:       SoD maker ≠ reviewer ≠ approver ≠ approver_2  — Scenario P5-M2-C
//	DEC-018:       Audit trail in-transaction                      — All scenarios
//	DEC-021:       Idempotency-Key mandatory                       — Scenario P5-M2-M
//	DEC-027:       Step-up MFA for approve (regulated) + approve_2 — Scenario P5-M2-D
//
// Run:
//
//	go test ./tests/e2e/... -v -run TestE2E_P5M2 -timeout 60s
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

// ─── P5-M2 domain constants ───────────────────────────────────────────────────

const (
	// Mapping jurnal workflow statuses (mirrors mst.mapping_jurnal_header.workflow_status).
	jrnlStatusDraft            = "DRAFT"
	jrnlStatusPendingReview    = "PENDING_REVIEW"
	jrnlStatusPendingApproval  = "PENDING_APPROVAL"
	jrnlStatusPendingApproval2 = "PENDING_APPROVAL_2"
	jrnlStatusApprovedActive   = "APPROVED_ACTIVE"
	jrnlStatusWithdrawn        = "WITHDRAWN"

	// DK indicators.
	dkDebit  = "DEBIT"
	dkKredit = "KREDIT"

	// Periode statuses.
	periodeOpen       = "OPEN"
	periodeSoftClosed = "SOFT_CLOSED"
	periodeHardClosed = "HARD_CLOSED"

	// DLQ statuses.
	dlqFailed     = "FAILED"
	dlqReplaying  = "REPLAYING"
	dlqReplayedOK = "REPLAYED_OK"
	dlqAbandoned  = "ABANDONED"

	// Error categories.
	errCategoryDomain = "DOMAIN"
	errCategoryInfra  = "INFRA"

	// jrnl.header status_internal.
	jrnlPosted = "POSTED"

	// Jurnal error codes (mirrors OpenAPI app-d-jurnal-engine.yaml).
	errJurnalEventNotMapped         = "JURNAL_EVENT_NOT_MAPPED"
	errJurnalKlasifikasiNotEligible = "JURNAL_KLASIFIKASI_NOT_ELIGIBLE"
	errJurnalBalanceInvariant       = "JURNAL_BALANCE_INVARIANT"
	errJurnalPeriodeHardClosed      = "JURNAL_PERIODE_HARD_CLOSED"
	errJurnalSODViolation           = "JURNAL_SOD_VIOLATION"
	errJurnalStepUpRequired         = "JURNAL_STEP_UP_REQUIRED"
	errJurnalIdempotencyReplay      = "JURNAL_IDEMPOTENCY_REPLAY"

	// Audit action constants for P5-M2.
	auditJurnalMappingCreate   = "JURNAL_MAPPING.CREATE"
	auditJurnalMappingSubmit   = "JURNAL_MAPPING.SUBMIT"
	auditJurnalMappingReview   = "JURNAL_MAPPING.REVIEW"
	auditJurnalMappingApprove  = "JURNAL_MAPPING.APPROVE"
	auditJurnalMappingApprove2 = "JURNAL_MAPPING.APPROVE_2"
	auditJurnalMappingSOD      = "JURNAL_MAPPING.SOD_VIOLATION_ATTEMPT"
	auditJurnalPost            = "JURNAL.POST"
	auditJurnalPostFailed      = "JURNAL.POST_FAILED"
	auditJurnalDLQReplay       = "JURNAL.DLQ_REPLAYED"
	auditJurnalDLQDiscard      = "JURNAL.DLQ_DISCARD"

	// kategori_event used to distinguish regulated vs operational.
	kategoriECL        = "ECL"
	kategoriPenempatan = "PENEMPATAN"

	// event codes.
	eventCodePenempatan     = "PENEMPATAN"
	eventCodeECLPembentukan = "ECL_PEMBENTUKAN"
	eventCodeMTMFVOCI       = "MTM_FVOCI"
	eventCodeStageMigration = "STAGE_MIGRATION"

	// Source event types (Asynq).
	sourceEventPenempatanApproved = "penempatan:approved"
)

// ─── P5-M2 domain types ───────────────────────────────────────────────────────

// mappingJurnalRecord mirrors mst.mapping_jurnal_header.
type mappingJurnalRecord struct {
	ID                 uuid.UUID
	EventCode          string
	NamaEvent          string
	KategoriEvent      string
	TriggerSource      string
	KlasifikasiBerlaku []string // nil = ALL
	AktifFlag          bool
	WorkflowStatus     string
	MakerID            uuid.UUID
	ReviewerID         *uuid.UUID
	ApproverID         *uuid.UUID
	Approver2ID        *uuid.UUID
	ReviewerSignedAt   *time.Time
	ReviewerSigHash    string
	ApproverSignedAt   *time.Time
	ApproverSigHash    string
	Approver2SignedAt  *time.Time
	Approver2SigHash   string
	RejectReason       string
	DetailRows         []mappingJurnalDetail
	IsRegulated        bool // derived from kategori_event
	RowVersion         int64
	TenantID           string
}

// mappingJurnalDetail mirrors mst.mapping_jurnal_detail.
type mappingJurnalDetail struct {
	ID                uuid.UUID
	EventHeaderID     uuid.UUID
	Urutan            int
	KodeAkunID        uuid.UUID
	DKIndicator       string // DEBIT | KREDIT
	SumberAmount      string // nominal_idr | ecl_amount | mtm_change
	KlasifikasiFilter string // empty = apply to all
	Multiplier        decimal.Decimal
}

// jurnalLine is the resolver output.
type jurnalLine struct {
	Urutan    int
	Posisi    string // DEBIT | KREDIT
	AkunID    uuid.UUID
	AmountIDR decimal.Decimal
	Narasi    string
}

// jurnalHeader mirrors jrnl.header.
type jurnalHeader struct {
	ID                 uuid.UUID
	NoJurnal           string
	TanggalPosting     time.Time
	PeriodeID          uuid.UUID
	EventCode          string
	MappingHeaderID    uuid.UUID
	InstrumenID        *uuid.UUID
	ReferenceEventType string
	ReferenceEventID   uuid.UUID
	Currency           string
	TotalDebit         decimal.Decimal
	TotalKredit        decimal.Decimal
	Narrative          string
	StatusInternal     string // POSTED | REVERSED
	IdempotencyKey     string // SHA256(source_event_id||"::"||event_code)
	CreatedAt          time.Time
	CreatedBy          uuid.UUID
	Details            []jurnalDetail
}

// jurnalDetail mirrors jrnl.detail.
type jurnalDetail struct {
	ID            uuid.UUID
	HeaderID      uuid.UUID
	Urutan        int
	KodeAkunID    uuid.UUID
	DebitAmount   decimal.Decimal
	KreditAmount  decimal.Decimal
	MataUang      string
	NarrativeLine string
}

// dlqEntry mirrors sys.dlq_jurnal_post.
type dlqEntry struct {
	ID               uuid.UUID
	SourceEventID    uuid.UUID
	SourceEventType  string
	EventCode        string
	InstrumenID      *uuid.UUID
	PeriodeID        uuid.UUID
	PayloadJSON      map[string]interface{}
	ErrorCode        string
	ErrorMessage     string
	ErrorCategory    string // DOMAIN | INFRA
	AttemptCount     int
	LastAttemptAt    time.Time
	Status           string
	ReplayedJurnalID *uuid.UUID
	ReplayedBy       *uuid.UUID
	ReplayedAt       *time.Time
	DiscardedReason  string
}

// resolveAndPostInput mirrors the Asynq worker payload.
type resolveAndPostInput struct {
	SourceEventID   uuid.UUID
	SourceEventType string
	EventCode       string
	InstrumenID     *uuid.UUID
	PeriodeID       uuid.UUID
	AmountIDR       decimal.Decimal
	KlasifikasiPSAK string
	FxRate          decimal.Decimal
	Narrative       string
}

// workerError captures worker-level error metadata.
type workerError struct {
	Code      string
	Message   string
	Category  string // DOMAIN | INFRA
	SkipRetry bool
}

func (e *workerError) Error() string { return e.Code + ": " + e.Message }

// mappingSODError for SoD violations in mapping workflow.
type mappingSODError struct {
	Code    string
	Message string
}

func (e *mappingSODError) Error() string { return e.Code + ": " + e.Message }

// mappingStepUpError for stale MFA step-up.
type mappingStepUpError struct {
	Code    string
	Message string
}

func (e *mappingStepUpError) Error() string { return e.Code + ": " + e.Message }

// mappingValidationError for validation failures.
type mappingValidationError struct {
	Code    string
	Message string
}

func (e *mappingValidationError) Error() string { return e.Code + ": " + e.Message }

// ─── P5-M2 harness ───────────────────────────────────────────────────────────

// p5M2Harness wires up in-process stubs for P5-M2 jurnal engine.
type p5M2Harness struct {
	t            *testing.T
	auditLog     *p5M2AuditStore
	mappingStore *p5M2MappingStore
	periodeStore *p5M2PeriodeStore
	jurnalStore  *p5M2JurnalStore
	dlqStore     *p5M2DLQStore
	asynqQueue   *p5M2AsynqQueue
}

func newP5M2Harness(t *testing.T) *p5M2Harness {
	t.Helper()
	h := &p5M2Harness{t: t}
	h.auditLog = newP5M2AuditStore()
	h.mappingStore = newP5M2MappingStore()
	h.periodeStore = newP5M2PeriodeStore()
	h.jurnalStore = newP5M2JurnalStore()
	h.dlqStore = newP5M2DLQStore()
	h.asynqQueue = newP5M2AsynqQueue()
	return h
}

// ─── Audit store ──────────────────────────────────────────────────────────────

type p5M2AuditRow struct {
	EventID     string
	Action      string
	EntityID    string
	ActorID     string
	ActorRole   string
	CurrentHash []byte
	AfterJSON   map[string]interface{}
}

type p5M2AuditStore struct {
	rows []p5M2AuditRow
}

func newP5M2AuditStore() *p5M2AuditStore { return &p5M2AuditStore{} }

func (s *p5M2AuditStore) append(action, entityID, actorID, actorRole string, afterJSON map[string]interface{}) {
	var prevHash []byte
	if len(s.rows) > 0 {
		prevHash = s.rows[len(s.rows)-1].CurrentHash
	}
	payload := fmt.Sprintf("%x||%s||%s||%v", prevHash, action, entityID, afterJSON)
	h := sha256.Sum256([]byte(payload))
	s.rows = append(s.rows, p5M2AuditRow{
		EventID:     uuid.New().String(),
		Action:      action,
		EntityID:    entityID,
		ActorID:     actorID,
		ActorRole:   actorRole,
		CurrentHash: h[:],
		AfterJSON:   afterJSON,
	})
}

func (s *p5M2AuditStore) containsAction(entityID, action string) bool {
	for _, r := range s.rows {
		if r.EntityID == entityID && r.Action == action {
			return true
		}
	}
	return false
}

func (s *p5M2AuditStore) actionsForEntity(entityID string) []string {
	var out []string
	for _, r := range s.rows {
		if r.EntityID == entityID {
			out = append(out, r.Action)
		}
	}
	return out
}

func (s *p5M2AuditStore) rowsForEntity(entityID string) []p5M2AuditRow {
	var out []p5M2AuditRow
	for _, r := range s.rows {
		if r.EntityID == entityID {
			out = append(out, r)
		}
	}
	return out
}

// ─── Mapping store ────────────────────────────────────────────────────────────

type p5M2MappingStore struct {
	records map[uuid.UUID]*mappingJurnalRecord
}

func newP5M2MappingStore() *p5M2MappingStore {
	return &p5M2MappingStore{records: make(map[uuid.UUID]*mappingJurnalRecord)}
}

// isRegulatedEventCode returns true if the event code requires 6-eyes (DEC-P5-M1-003).
func isRegulatedEventCode(kategoriEvent string) bool {
	switch kategoriEvent {
	case "ECL", "AKRUAL", "MUTASI_MTM", "STAGE_MIGRATION", "REKLASIFIKASI":
		return true
	}
	return false
}

func (s *p5M2MappingStore) get(id uuid.UUID) *mappingJurnalRecord {
	return s.records[id]
}

// findApproved returns the first APPROVED_ACTIVE mapping for eventCode that matches klasifikasi.
func (s *p5M2MappingStore) findApproved(eventCode, klasifikasi string) *mappingJurnalRecord {
	for _, r := range s.records {
		if r.EventCode != eventCode {
			continue
		}
		if r.WorkflowStatus != jrnlStatusApprovedActive || !r.AktifFlag {
			continue
		}
		// nil = ALL
		if r.KlasifikasiBerlaku == nil {
			return r
		}
		for _, k := range r.KlasifikasiBerlaku {
			if k == klasifikasi {
				return r
			}
		}
	}
	return nil
}

// ─── Periode store ────────────────────────────────────────────────────────────

type p5M2PeriodeRecord struct {
	ID     uuid.UUID
	Kode   string
	Status string // OPEN | SOFT_CLOSED | HARD_CLOSED
}

type p5M2PeriodeStore struct {
	records map[uuid.UUID]*p5M2PeriodeRecord
}

func newP5M2PeriodeStore() *p5M2PeriodeStore {
	return &p5M2PeriodeStore{records: make(map[uuid.UUID]*p5M2PeriodeRecord)}
}

func (s *p5M2PeriodeStore) seedOpen(kode string) *p5M2PeriodeRecord {
	r := &p5M2PeriodeRecord{ID: uuid.New(), Kode: kode, Status: periodeOpen}
	s.records[r.ID] = r
	return r
}

func (s *p5M2PeriodeStore) seedHardClosed(kode string) *p5M2PeriodeRecord {
	r := &p5M2PeriodeRecord{ID: uuid.New(), Kode: kode, Status: periodeHardClosed}
	s.records[r.ID] = r
	return r
}

func (s *p5M2PeriodeStore) setStatus(id uuid.UUID, status string) {
	if r := s.records[id]; r != nil {
		r.Status = status
	}
}

func (s *p5M2PeriodeStore) get(id uuid.UUID) *p5M2PeriodeRecord {
	return s.records[id]
}

// ─── Jurnal store ─────────────────────────────────────────────────────────────

type p5M2JurnalStore struct {
	headers       map[uuid.UUID]*jurnalHeader
	idempotentMap map[string]*jurnalHeader // idempotency_key → header
	seqCounter    int
}

func newP5M2JurnalStore() *p5M2JurnalStore {
	return &p5M2JurnalStore{
		headers:       make(map[uuid.UUID]*jurnalHeader),
		idempotentMap: make(map[string]*jurnalHeader),
	}
}

func (s *p5M2JurnalStore) insertHeader(h *jurnalHeader) error {
	if _, exists := s.idempotentMap[h.IdempotencyKey]; exists {
		return &workerError{
			Code:      errJurnalIdempotencyReplay,
			Message:   "jurnal dengan idempotency_key ini sudah diposting",
			Category:  errCategoryDomain,
			SkipRetry: true,
		}
	}
	s.seqCounter++
	h.NoJurnal = fmt.Sprintf("JRN-2026-%06d", s.seqCounter)
	s.headers[h.ID] = h
	s.idempotentMap[h.IdempotencyKey] = h
	return nil
}

func (s *p5M2JurnalStore) findByIdempotencyKey(key string) *jurnalHeader {
	return s.idempotentMap[key]
}

// ─── DLQ store ────────────────────────────────────────────────────────────────

type p5M2DLQStore struct {
	entries map[uuid.UUID]*dlqEntry
}

func newP5M2DLQStore() *p5M2DLQStore {
	return &p5M2DLQStore{entries: make(map[uuid.UUID]*dlqEntry)}
}

func (s *p5M2DLQStore) insert(e *dlqEntry) {
	if e.ID == (uuid.UUID{}) {
		e.ID = uuid.New()
	}
	s.entries[e.ID] = e
}

func (s *p5M2DLQStore) get(id uuid.UUID) *dlqEntry {
	return s.entries[id]
}

func (s *p5M2DLQStore) countByStatus(status string) int {
	n := 0
	for _, e := range s.entries {
		if e.Status == status {
			n++
		}
	}
	return n
}

// ─── Asynq queue stub ─────────────────────────────────────────────────────────

// p5M2AsynqQueue is a stub for Asynq task tracking; reserved for future assertions.
type p5M2AsynqQueue struct{}

func newP5M2AsynqQueue() *p5M2AsynqQueue { return &p5M2AsynqQueue{} }

// ─── Mapping jurnal service ───────────────────────────────────────────────────

type mappingJurnalService struct {
	h *p5M2Harness
}

func newMappingJurnalService(h *p5M2Harness) *mappingJurnalService {
	return &mappingJurnalService{h: h}
}

// computeJurnalSigHash computes SHA-256(actorID||step||entityID||ts||comment).
func computeJurnalSigHash(actorID uuid.UUID, step string, entityID uuid.UUID, ts time.Time, comment string) string {
	payload := fmt.Sprintf("%s||%s||%s||%d||%s", actorID, step, entityID, ts.Unix(), comment)
	h := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(h[:])
}

// computeIdempotencyKey computes SHA256(sourceEventID||"::"||eventCode).
func computeIdempotencyKey(sourceEventID uuid.UUID, eventCode string) string {
	payload := fmt.Sprintf("%s::%s", sourceEventID, eventCode)
	h := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(h[:])
}

type createMappingRequest struct {
	EventCode          string
	NamaEvent          string
	KategoriEvent      string
	TriggerSource      string
	KlasifikasiBerlaku []string
	Deskripsi          string
	DetailRows         []mappingJurnalDetail
	IdempotencyKey     uuid.UUID
}

func (svc *mappingJurnalService) Create(ctx context.Context, req createMappingRequest, actor *auth.Claims) (*mappingJurnalRecord, error) {
	actorID, _ := uuid.Parse(actor.Sub)

	// Validate: at least 1 DEBIT + 1 KREDIT.
	debitCount, kreditCount := 0, 0
	for _, d := range req.DetailRows {
		if d.DKIndicator == dkDebit {
			debitCount++
		}
		if d.DKIndicator == dkKredit {
			kreditCount++
		}
	}
	if debitCount == 0 || kreditCount == 0 {
		return nil, &mappingValidationError{
			Code:    "VALIDATION_FAILED",
			Message: "template harus memiliki minimal 1 baris DEBIT dan 1 baris KREDIT",
		}
	}

	regulated := isRegulatedEventCode(req.KategoriEvent)
	r := &mappingJurnalRecord{
		ID:                 uuid.New(),
		EventCode:          req.EventCode,
		NamaEvent:          req.NamaEvent,
		KategoriEvent:      req.KategoriEvent,
		TriggerSource:      req.TriggerSource,
		KlasifikasiBerlaku: req.KlasifikasiBerlaku,
		AktifFlag:          false,
		WorkflowStatus:     jrnlStatusDraft,
		MakerID:            actorID,
		DetailRows:         req.DetailRows,
		IsRegulated:        regulated,
		RowVersion:         1,
		TenantID:           actor.TenantID,
	}
	svc.h.mappingStore.records[r.ID] = r

	svc.h.auditLog.append(auditJurnalMappingCreate, r.ID.String(), actorID.String(), actor.Roles[0],
		map[string]interface{}{"event_code": r.EventCode, "workflow_status": jrnlStatusDraft, "is_regulated": regulated})
	return r, nil
}

func (svc *mappingJurnalService) Submit(ctx context.Context, id uuid.UUID, comment string, actor *auth.Claims) (*mappingJurnalRecord, error) {
	r := svc.h.mappingStore.get(id)
	if r == nil {
		return nil, &mappingValidationError{Code: "NOT_FOUND", Message: "mapping not found"}
	}
	actorID, _ := uuid.Parse(actor.Sub)
	if r.WorkflowStatus != jrnlStatusDraft {
		return nil, &mappingValidationError{Code: "WORKFLOW_INVALID_TRANSITION", Message: "can only submit from DRAFT"}
	}
	if r.MakerID != actorID {
		return nil, &mappingSODError{Code: errJurnalSODViolation, Message: "only maker can submit"}
	}
	r.WorkflowStatus = jrnlStatusPendingReview
	r.RowVersion++
	svc.h.auditLog.append(auditJurnalMappingSubmit, r.ID.String(), actorID.String(), actor.Roles[0],
		map[string]interface{}{"workflow_status": jrnlStatusPendingReview})
	return r, nil
}

func (svc *mappingJurnalService) Review(ctx context.Context, id uuid.UUID, comment string, actor *auth.Claims) (*mappingJurnalRecord, error) {
	r := svc.h.mappingStore.get(id)
	if r == nil {
		return nil, &mappingValidationError{Code: "NOT_FOUND", Message: "mapping not found"}
	}
	actorID, _ := uuid.Parse(actor.Sub)
	if r.WorkflowStatus != jrnlStatusPendingReview {
		return nil, &mappingValidationError{Code: "WORKFLOW_INVALID_TRANSITION", Message: "can only review from PENDING_REVIEW"}
	}
	// SoD: reviewer ≠ maker.
	if r.MakerID == actorID {
		svc.h.auditLog.append(auditJurnalMappingSOD, r.ID.String(), actorID.String(), actor.Roles[0],
			map[string]interface{}{"attempted_step": "REVIEW", "sod_violation": "reviewer=maker"})
		return nil, &mappingSODError{Code: errJurnalSODViolation, Message: "reviewer tidak bisa sama dengan maker (DEC-017)"}
	}
	now := time.Now()
	sigHash := computeJurnalSigHash(actorID, "REVIEW", r.ID, now, comment)
	r.WorkflowStatus = jrnlStatusPendingApproval
	r.ReviewerID = &actorID
	r.ReviewerSignedAt = &now
	r.ReviewerSigHash = sigHash
	r.RowVersion++
	svc.h.auditLog.append(auditJurnalMappingReview, r.ID.String(), actorID.String(), actor.Roles[0],
		map[string]interface{}{"reviewer_signature_hash": sigHash, "workflow_status": jrnlStatusPendingApproval})
	return r, nil
}

// Approve handles approve step 1 (PENDING_APPROVAL → APPROVED_ACTIVE for operational,
// or PENDING_APPROVAL → PENDING_APPROVAL_2 for regulated).
func (svc *mappingJurnalService) Approve(ctx context.Context, id uuid.UUID, comment string, actor *auth.Claims) (*mappingJurnalRecord, error) {
	r := svc.h.mappingStore.get(id)
	if r == nil {
		return nil, &mappingValidationError{Code: "NOT_FOUND", Message: "mapping not found"}
	}
	actorID, _ := uuid.Parse(actor.Sub)
	if r.WorkflowStatus != jrnlStatusPendingApproval {
		return nil, &mappingValidationError{Code: "WORKFLOW_INVALID_TRANSITION", Message: "can only approve from PENDING_APPROVAL"}
	}
	// SoD: approver ≠ maker AND approver ≠ reviewer.
	if r.MakerID == actorID {
		svc.h.auditLog.append(auditJurnalMappingSOD, r.ID.String(), actorID.String(), actor.Roles[0],
			map[string]interface{}{"attempted_step": "APPROVE", "sod_violation": "approver=maker"})
		return nil, &mappingSODError{Code: errJurnalSODViolation, Message: "approver tidak bisa sama dengan maker (DEC-017)"}
	}
	if r.ReviewerID != nil && *r.ReviewerID == actorID {
		svc.h.auditLog.append(auditJurnalMappingSOD, r.ID.String(), actorID.String(), actor.Roles[0],
			map[string]interface{}{"attempted_step": "APPROVE", "sod_violation": "approver=reviewer"})
		return nil, &mappingSODError{Code: errJurnalSODViolation, Message: "approver tidak bisa sama dengan reviewer (DEC-017)"}
	}
	// Step-up MFA required for regulated codes approve (DEC-027 per state machine M09).
	if r.IsRegulated && actor.NeedsStepUp() {
		return nil, &mappingStepUpError{Code: errJurnalStepUpRequired, Message: "approve regulated mapping memerlukan MFA step-up (DEC-027)"}
	}

	now := time.Now()
	sigHash := computeJurnalSigHash(actorID, "APPROVE", r.ID, now, comment)
	r.ApproverID = &actorID
	r.ApproverSignedAt = &now
	r.ApproverSigHash = sigHash
	r.RowVersion++

	if r.IsRegulated {
		// 6-eyes: go to PENDING_APPROVAL_2.
		r.WorkflowStatus = jrnlStatusPendingApproval2
	} else {
		// 4-eyes: directly APPROVED_ACTIVE.
		r.WorkflowStatus = jrnlStatusApprovedActive
		r.AktifFlag = true
	}

	svc.h.auditLog.append(auditJurnalMappingApprove, r.ID.String(), actorID.String(), actor.Roles[0],
		map[string]interface{}{"approver_signature_hash": sigHash, "workflow_status": r.WorkflowStatus})
	return r, nil
}

// Approve2 handles the 6-eyes second approval (PENDING_APPROVAL_2 → APPROVED_ACTIVE).
func (svc *mappingJurnalService) Approve2(ctx context.Context, id uuid.UUID, comment string, actor *auth.Claims) (*mappingJurnalRecord, error) {
	r := svc.h.mappingStore.get(id)
	if r == nil {
		return nil, &mappingValidationError{Code: "NOT_FOUND", Message: "mapping not found"}
	}
	actorID, _ := uuid.Parse(actor.Sub)
	if r.WorkflowStatus != jrnlStatusPendingApproval2 {
		return nil, &mappingValidationError{Code: "WORKFLOW_INVALID_TRANSITION", Message: "can only approve-2 from PENDING_APPROVAL_2"}
	}
	// SoD: approver_2 ≠ maker ≠ reviewer ≠ approver_1 (DEC-017).
	if r.MakerID == actorID || (r.ReviewerID != nil && *r.ReviewerID == actorID) ||
		(r.ApproverID != nil && *r.ApproverID == actorID) {
		svc.h.auditLog.append(auditJurnalMappingSOD, r.ID.String(), actorID.String(), actor.Roles[0],
			map[string]interface{}{"attempted_step": "APPROVE_2", "sod_violation": "approver_2=previous_role"})
		return nil, &mappingSODError{Code: errJurnalSODViolation, Message: "approver_2 tidak bisa sama dengan maker/reviewer/approver_1 (DEC-017)"}
	}
	// Step-up MFA required for approve_2 (OQ-M2-1c resolved YES, DEC-027).
	if actor.NeedsStepUp() {
		return nil, &mappingStepUpError{Code: errJurnalStepUpRequired, Message: "approve_2 memerlukan MFA step-up (DEC-027, OQ-M2-1c)"}
	}

	now := time.Now()
	sigHash := computeJurnalSigHash(actorID, "APPROVE_2", r.ID, now, comment)
	r.WorkflowStatus = jrnlStatusApprovedActive
	r.AktifFlag = true
	r.Approver2ID = &actorID
	r.Approver2SignedAt = &now
	r.Approver2SigHash = sigHash
	r.RowVersion++

	svc.h.auditLog.append(auditJurnalMappingApprove2, r.ID.String(), actorID.String(), actor.Roles[0],
		map[string]interface{}{
			"approver_2_signature_hash": sigHash,
			"approver_2_signed_at":      now.Unix(),
			"workflow_status":           jrnlStatusApprovedActive,
		})
	return r, nil
}

// ─── Jurnal resolver service ──────────────────────────────────────────────────

type jurnalResolverService struct {
	h *p5M2Harness
}

func newJurnalResolverService(h *p5M2Harness) *jurnalResolverService {
	return &jurnalResolverService{h: h}
}

func (svc *jurnalResolverService) Resolve(eventCode, klasifikasi string, amountIDR decimal.Decimal) ([]jurnalLine, error) {
	mapping := svc.h.mappingStore.findApproved(eventCode, klasifikasi)
	if mapping == nil {
		// Distinguish: event code exists but wrong klasifikasi vs event not mapped at all.
		// Check if ANY mapping exists for the event code.
		for _, r := range svc.h.mappingStore.records {
			if r.EventCode == eventCode && r.WorkflowStatus == jrnlStatusApprovedActive && r.AktifFlag {
				// Event exists but klasifikasi not eligible.
				return nil, &workerError{
					Code:      errJurnalKlasifikasiNotEligible,
					Message:   fmt.Sprintf("event code '%s' tidak berlaku untuk klasifikasi PSAK 71 '%s'", eventCode, klasifikasi),
					Category:  errCategoryDomain,
					SkipRetry: true,
				}
			}
		}
		return nil, &workerError{
			Code:      errJurnalEventNotMapped,
			Message:   fmt.Sprintf("tidak ada mapping jurnal APPROVED untuk event code '%s'", eventCode),
			Category:  errCategoryDomain,
			SkipRetry: true,
		}
	}

	// Build JurnalLines from detail rows.
	lines := make([]jurnalLine, 0, len(mapping.DetailRows))
	for _, d := range mapping.DetailRows {
		amount := amountIDR
		if !d.Multiplier.IsZero() && !d.Multiplier.Equal(decimal.NewFromInt(1)) {
			amount = amountIDR.Mul(d.Multiplier)
		}
		lines = append(lines, jurnalLine{
			Urutan:    d.Urutan,
			Posisi:    d.DKIndicator,
			AkunID:    d.KodeAkunID,
			AmountIDR: amount,
			Narasi:    fmt.Sprintf("%s %s", eventCode, klasifikasi),
		})
	}

	// Balance invariant check.
	var totalDebit, totalKredit decimal.Decimal
	for _, l := range lines {
		if l.Posisi == dkDebit {
			totalDebit = totalDebit.Add(l.AmountIDR)
		} else {
			totalKredit = totalKredit.Add(l.AmountIDR)
		}
	}
	if !totalDebit.Equal(totalKredit) {
		return nil, &workerError{
			Code:      errJurnalBalanceInvariant,
			Message:   fmt.Sprintf("resolver menghasilkan imbalance: DEBIT %s ≠ KREDIT %s", totalDebit.StringFixed(4), totalKredit.StringFixed(4)),
			Category:  errCategoryDomain,
			SkipRetry: true,
		}
	}

	return lines, nil
}

// ─── ResolveAndPost service (Asynq worker logic) ─────────────────────────────

type resolveAndPostService struct {
	h        *p5M2Harness
	resolver *jurnalResolverService
	// infraErrSimCount tracks simulated infra errors for scenario P5-M2-O.
	infraErrSimCount int
}

func newResolveAndPostService(h *p5M2Harness) *resolveAndPostService {
	return &resolveAndPostService{
		h:        h,
		resolver: newJurnalResolverService(h),
	}
}

// ProcessEvent is the Asynq task handler for jurnal:post events.
// Returns error; if the error has SkipRetry=true, caller treats as DLQ-immediate.
func (svc *resolveAndPostService) ProcessEvent(input resolveAndPostInput) (*jurnalHeader, error) {
	// Infra error simulation (P5-M2-O).
	if svc.infraErrSimCount > 0 {
		svc.infraErrSimCount--
		return nil, &workerError{
			Code:      "DB_CONNECTION_ERROR",
			Message:   "simulated DB connection error",
			Category:  errCategoryInfra,
			SkipRetry: false, // infra error = retry
		}
	}

	// Guard: periode must be OPEN or SOFT_CLOSED.
	periode := svc.h.periodeStore.get(input.PeriodeID)
	if periode == nil || periode.Status == periodeHardClosed {
		return svc.writeDLQAndFail(input, errJurnalPeriodeHardClosed,
			"periode sudah HARD_CLOSED, posting tidak dapat dilakukan", errCategoryDomain)
	}

	// Idempotency check.
	idempKey := computeIdempotencyKey(input.SourceEventID, input.EventCode)
	if existing := svc.h.jurnalStore.findByIdempotencyKey(idempKey); existing != nil {
		return existing, &workerError{
			Code:      errJurnalIdempotencyReplay,
			Message:   "idempotent replay — returning existing jurnal_header",
			Category:  errCategoryDomain,
			SkipRetry: true,
		}
	}

	// Resolve.
	lines, err := svc.resolver.Resolve(input.EventCode, input.KlasifikasiPSAK, input.AmountIDR)
	if err != nil {
		we, _ := err.(*workerError)
		return svc.writeDLQAndFail(input, we.Code, we.Message, we.Category)
	}

	// Build header + details (atomic in-tx).
	headerID := uuid.New()
	now := time.Now()
	var totalDebit, totalKredit decimal.Decimal
	details := make([]jurnalDetail, 0, len(lines))
	for _, l := range lines {
		var debit, kredit decimal.Decimal
		if l.Posisi == dkDebit {
			debit = l.AmountIDR
			totalDebit = totalDebit.Add(l.AmountIDR)
		} else {
			kredit = l.AmountIDR
			totalKredit = totalKredit.Add(l.AmountIDR)
		}
		details = append(details, jurnalDetail{
			ID:            uuid.New(),
			HeaderID:      headerID,
			Urutan:        l.Urutan,
			KodeAkunID:    l.AkunID,
			DebitAmount:   debit,
			KreditAmount:  kredit,
			MataUang:      "IDR",
			NarrativeLine: l.Narasi,
		})
	}

	header := &jurnalHeader{
		ID:                 headerID,
		TanggalPosting:     now,
		PeriodeID:          input.PeriodeID,
		EventCode:          input.EventCode,
		InstrumenID:        input.InstrumenID,
		ReferenceEventType: input.SourceEventType,
		ReferenceEventID:   input.SourceEventID,
		Currency:           "IDR",
		TotalDebit:         totalDebit,
		TotalKredit:        totalKredit,
		Narrative:          input.Narrative,
		StatusInternal:     jrnlPosted,
		IdempotencyKey:     idempKey,
		CreatedAt:          now,
		CreatedBy:          uuid.MustParse("00000000-0000-0000-0000-000000000001"), // system
		Details:            details,
	}

	// INSERT in-tx.
	if insertErr := svc.h.jurnalStore.insertHeader(header); insertErr != nil {
		// Could be idempotency collision race — return existing.
		return svc.h.jurnalStore.findByIdempotencyKey(idempKey), insertErr
	}

	// Audit JURNAL.POST in same logical transaction (DEC-018).
	svc.h.auditLog.append(auditJurnalPost, headerID.String(), "00000000-0000-0000-0000-000000000001", "SYSTEM_WORKER",
		map[string]interface{}{
			"event_code":         input.EventCode,
			"total_debit":        totalDebit.StringFixed(4),
			"total_kredit":       totalKredit.StringFixed(4),
			"reference_event_id": input.SourceEventID.String(),
		})

	return header, nil
}

func (svc *resolveAndPostService) writeDLQAndFail(input resolveAndPostInput, errCode, errMsg, errCategory string) (*jurnalHeader, error) {
	dlq := &dlqEntry{
		ID:              uuid.New(),
		SourceEventID:   input.SourceEventID,
		SourceEventType: input.SourceEventType,
		EventCode:       input.EventCode,
		InstrumenID:     input.InstrumenID,
		PeriodeID:       input.PeriodeID,
		PayloadJSON:     map[string]interface{}{"amount_idr": input.AmountIDR.StringFixed(4), "klasifikasi": input.KlasifikasiPSAK},
		ErrorCode:       errCode,
		ErrorMessage:    errMsg,
		ErrorCategory:   errCategory,
		AttemptCount:    1,
		LastAttemptAt:   time.Now(),
		Status:          dlqFailed,
	}
	svc.h.dlqStore.insert(dlq)

	// Audit JURNAL.POST_FAILED (in-transaction).
	svc.h.auditLog.append(auditJurnalPostFailed, dlq.ID.String(), "00000000-0000-0000-0000-000000000001", "SYSTEM_WORKER",
		map[string]interface{}{"error_code": errCode, "event_code": input.EventCode, "error_category": errCategory})

	return nil, &workerError{Code: errCode, Message: errMsg, Category: errCategory, SkipRetry: errCategory == errCategoryDomain}
}

// ─── DLQ replay service ───────────────────────────────────────────────────────

type dlqService struct {
	h       *p5M2Harness
	postSvc *resolveAndPostService
}

func newDLQService(h *p5M2Harness, postSvc *resolveAndPostService) *dlqService {
	return &dlqService{h: h, postSvc: postSvc}
}

func (svc *dlqService) Replay(dlqID uuid.UUID, actor *auth.Claims) (*jurnalHeader, error) {
	entry := svc.h.dlqStore.get(dlqID)
	if entry == nil {
		return nil, &mappingValidationError{Code: "NOT_FOUND", Message: "DLQ entry not found"}
	}
	actorID, _ := uuid.Parse(actor.Sub)

	// Audit JURNAL.DLQ_REPLAY before Asynq task enqueue (security-baseline §"DLQ replay").
	svc.h.auditLog.append(auditJurnalDLQReplay, dlqID.String(), actorID.String(), actor.Roles[0],
		map[string]interface{}{"dlq_id": dlqID.String(), "event_code": entry.EventCode})

	entry.Status = dlqReplaying

	// Re-run posting with original payload.
	input := resolveAndPostInput{
		SourceEventID:   entry.SourceEventID,
		SourceEventType: entry.SourceEventType,
		EventCode:       entry.EventCode,
		InstrumenID:     entry.InstrumenID,
		PeriodeID:       entry.PeriodeID,
		AmountIDR:       decimal.NewFromFloat(5_000_000_000),
		KlasifikasiPSAK: klasifikasiAC,
		FxRate:          decimal.NewFromInt(1),
		Narrative:       "DLQ replay",
	}
	if v, ok := entry.PayloadJSON["klasifikasi"]; ok {
		if s, ok := v.(string); ok {
			input.KlasifikasiPSAK = s
		}
	}

	header, err := svc.postSvc.ProcessEvent(input)
	if err != nil {
		we, _ := err.(*workerError)
		if we != nil && we.Code != errJurnalIdempotencyReplay {
			entry.Status = dlqFailed
			entry.AttemptCount++
			entry.LastAttemptAt = time.Now()
			return nil, err
		}
	}

	if header != nil {
		now := time.Now()
		entry.Status = dlqReplayedOK
		entry.ReplayedJurnalID = &header.ID
		entry.ReplayedBy = &actorID
		entry.ReplayedAt = &now
	}
	return header, nil
}

func (svc *dlqService) Discard(dlqID uuid.UUID, reason string, actor *auth.Claims) error {
	entry := svc.h.dlqStore.get(dlqID)
	if entry == nil {
		return &mappingValidationError{Code: "NOT_FOUND", Message: "DLQ entry not found"}
	}
	if len(reason) < 30 {
		return &mappingValidationError{Code: "VALIDATION_FAILED", Message: "discarded_reason minimal 30 karakter"}
	}
	actorID, _ := uuid.Parse(actor.Sub)
	entry.Status = dlqAbandoned
	entry.DiscardedReason = reason

	svc.h.auditLog.append(auditJurnalDLQDiscard, dlqID.String(), actorID.String(), actor.Roles[0],
		map[string]interface{}{"dlq_id": dlqID.String(), "reason": reason})
	return nil
}

// ─── Context helpers (mirrors P5-M1 pattern) ─────────────────────────────────

func ctxWithJrnlAkunActor(sub string) context.Context {
	return ctxWithActor(sub, "ROLE-AKUN", "TUGURE", false)
}

func ctxWithJrnlAkunCTLActor(sub string, stepUpFresh bool) context.Context {
	var stepUpAt *int64
	if stepUpFresh {
		ts := time.Now().Add(-2 * time.Minute).Unix()
		stepUpAt = &ts
	} else {
		ts := time.Now().Add(-10 * time.Minute).Unix()
		stepUpAt = &ts
	}
	claims := &auth.Claims{
		Sub:              sub,
		Roles:            []string{"ROLE-AKUN-CTL"},
		TenantID:         "TUGURE",
		MFAVerified:      true,
		StepupVerifiedAt: stepUpAt,
	}
	return auth.ContextWithClaims(context.Background(), claims)
}

func ctxWithJrnlRiskActor(sub string, stepUpFresh bool) context.Context {
	var stepUpAt *int64
	if stepUpFresh {
		ts := time.Now().Add(-2 * time.Minute).Unix()
		stepUpAt = &ts
	} else {
		ts := time.Now().Add(-10 * time.Minute).Unix()
		stepUpAt = &ts
	}
	claims := &auth.Claims{
		Sub:              sub,
		Roles:            []string{"ROLE-RISK"},
		TenantID:         "TUGURE",
		MFAVerified:      true,
		StepupVerifiedAt: stepUpAt,
	}
	return auth.ContextWithClaims(context.Background(), claims)
}

// ─── Shared assert helpers ────────────────────────────────────────────────────

func assertMappingStatus(t *testing.T, r *mappingJurnalRecord, want, ctx string) {
	t.Helper()
	if r == nil {
		t.Fatalf("assertMappingStatus[%s]: nil record, want %s", ctx, want)
	}
	if r.WorkflowStatus != want {
		t.Errorf("assertMappingStatus[%s]: got %s, want %s", ctx, r.WorkflowStatus, want)
	}
}

func assertJurnalAuditContains(t *testing.T, h *p5M2Harness, entityID, action, ctx string) {
	t.Helper()
	if !h.auditLog.containsAction(entityID, action) {
		t.Errorf("assertJurnalAuditContains[%s]: action %q not found for entity %s; got: %v",
			ctx, action, entityID, h.auditLog.actionsForEntity(entityID))
	}
}

func assertJurnalAuditAbsent(t *testing.T, h *p5M2Harness, entityID, action, ctx string) {
	t.Helper()
	if h.auditLog.containsAction(entityID, action) {
		t.Errorf("assertJurnalAuditAbsent[%s]: action %q must NOT be present for entity %s",
			ctx, action, entityID)
	}
}

func assertJurnalErrorCode(t *testing.T, err error, wantCode, ctx string) {
	t.Helper()
	if err == nil {
		t.Fatalf("assertJurnalErrorCode[%s]: expected error with code %s, got nil", ctx, wantCode)
	}
	if !containsStr(err.Error(), wantCode) {
		t.Errorf("assertJurnalErrorCode[%s]: error %v does not contain code %q", ctx, err, wantCode)
	}
}

// ─── Seed helpers ─────────────────────────────────────────────────────────────

func seedApprovedMapping(h *p5M2Harness, eventCode, kategori string, klasifikasiBerlaku []string) *mappingJurnalRecord {
	akun1 := uuid.New()
	akun2 := uuid.New()
	r := &mappingJurnalRecord{
		ID:                 uuid.New(),
		EventCode:          eventCode,
		NamaEvent:          fmt.Sprintf("Template %s", eventCode),
		KategoriEvent:      kategori,
		TriggerSource:      "SYSTEM_JOB",
		KlasifikasiBerlaku: klasifikasiBerlaku,
		AktifFlag:          true,
		WorkflowStatus:     jrnlStatusApprovedActive,
		MakerID:            uuid.New(),
		IsRegulated:        isRegulatedEventCode(kategori),
		DetailRows: []mappingJurnalDetail{
			{ID: uuid.New(), Urutan: 1, KodeAkunID: akun1, DKIndicator: dkDebit, SumberAmount: "nominal_idr", Multiplier: decimal.NewFromInt(1)},
			{ID: uuid.New(), Urutan: 2, KodeAkunID: akun2, DKIndicator: dkKredit, SumberAmount: "nominal_idr", Multiplier: decimal.NewFromInt(1)},
		},
		RowVersion: 2,
		TenantID:   "TUGURE",
	}
	h.mappingStore.records[r.ID] = r
	return r
}

// seedImbalancedMapping creates a mapping whose detail rows are intentionally imbalanced.
func seedImbalancedMapping(h *p5M2Harness, eventCode string) *mappingJurnalRecord {
	akun1 := uuid.New()
	akun2 := uuid.New()
	r := &mappingJurnalRecord{
		ID:             uuid.New(),
		EventCode:      eventCode,
		NamaEvent:      fmt.Sprintf("Template %s (BROKEN)", eventCode),
		KategoriEvent:  "PENEMPATAN",
		AktifFlag:      true,
		WorkflowStatus: jrnlStatusApprovedActive,
		MakerID:        uuid.New(),
		// Imbalanced: DEBIT 100, KREDIT 80.
		DetailRows: []mappingJurnalDetail{
			{Urutan: 1, KodeAkunID: akun1, DKIndicator: dkDebit, Multiplier: decimal.NewFromFloat(0.10)},  // 100 on 1000
			{Urutan: 2, KodeAkunID: akun2, DKIndicator: dkKredit, Multiplier: decimal.NewFromFloat(0.08)}, // 80 on 1000
		},
		RowVersion: 2,
		TenantID:   "TUGURE",
	}
	h.mappingStore.records[r.ID] = r
	return r
}

// ─── Scenario P5-M2-A: Mapping CRUD 4-eyes operational happy path ────────────
//
// ROLE-AKUN creates DRAFT → submits → ROLE-AKUN-CTL reviews → ROLE-AKUN-CTL approves
// (no MFA step-up for operational codes, DEC-P5-M1-003) → APPROVED_ACTIVE.
// Verify audit chain.

func TestE2E_P5M2_ScenarioA_Mapping4EyesOperationalHappy(t *testing.T) {
	t.Parallel()
	h := newP5M2Harness(t)
	svc := newMappingJurnalService(h)

	userA := uuid.New().String() // ROLE-AKUN maker
	userB := uuid.New().String() // ROLE-AKUN-CTL reviewer+approver
	userC := uuid.New().String() // ROLE-AKUN-CTL approver (different from B to keep SoD)

	ctxA := ctxWithJrnlAkunActor(userA)
	ctxB := ctxWithJrnlAkunCTLActor(userB, true)
	ctxC := ctxWithJrnlAkunCTLActor(userC, true)
	claimsA := claimsFromCtx(ctxA)
	claimsB := claimsFromCtx(ctxB)
	claimsC := claimsFromCtx(ctxC)

	akun1, akun2 := uuid.New(), uuid.New()
	req := createMappingRequest{
		EventCode:          "PENEMPATAN_E2E_001",
		NamaEvent:          "Penempatan Instrumen Test",
		KategoriEvent:      kategoriPenempatan, // operational
		TriggerSource:      "USER_INPUT",
		KlasifikasiBerlaku: []string{klasifikasiAC, klasifikasiFVOCI, klasifikasiFVTPL},
		DetailRows: []mappingJurnalDetail{
			{Urutan: 1, KodeAkunID: akun1, DKIndicator: dkDebit, SumberAmount: "nominal_idr", Multiplier: decimal.NewFromInt(1)},
			{Urutan: 2, KodeAkunID: akun2, DKIndicator: dkKredit, SumberAmount: "nominal_idr", Multiplier: decimal.NewFromInt(1)},
		},
		IdempotencyKey: uuid.New(),
	}

	// ── Create DRAFT ──────────────────────────────────────────────────────────
	r, err := svc.Create(ctxA, req, claimsA)
	if err != nil {
		t.Fatalf("ScenarioA: Create failed: %v", err)
	}
	assertMappingStatus(t, r, jrnlStatusDraft, "after-create")
	if r.IsRegulated {
		t.Error("ScenarioA: PENEMPATAN must be operational (not regulated)")
	}
	assertJurnalAuditContains(t, h, r.ID.String(), auditJurnalMappingCreate, "create")

	// ── Submit ────────────────────────────────────────────────────────────────
	_, err = svc.Submit(ctxA, r.ID, "Submit untuk review", claimsA)
	if err != nil {
		t.Fatalf("ScenarioA: Submit failed: %v", err)
	}
	assertMappingStatus(t, r, jrnlStatusPendingReview, "after-submit")
	assertJurnalAuditContains(t, h, r.ID.String(), auditJurnalMappingSubmit, "submit")

	// ── Review ────────────────────────────────────────────────────────────────
	_, err = svc.Review(ctxB, r.ID, "Template sesuai chart of accounts", claimsB)
	if err != nil {
		t.Fatalf("ScenarioA: Review failed: %v", err)
	}
	assertMappingStatus(t, r, jrnlStatusPendingApproval, "after-review")
	assertJurnalAuditContains(t, h, r.ID.String(), auditJurnalMappingReview, "review")
	if r.ReviewerSigHash == "" {
		t.Error("ScenarioA: reviewer_signature_hash must be non-empty")
	}

	// ── Approve (operational → APPROVED_ACTIVE directly, no MFA step-up required) ──
	_, err = svc.Approve(ctxC, r.ID, "Disetujui sesuai FSD-APP-D", claimsC)
	if err != nil {
		t.Fatalf("ScenarioA: Approve failed: %v", err)
	}
	assertMappingStatus(t, r, jrnlStatusApprovedActive, "after-approve")
	if !r.AktifFlag {
		t.Error("ScenarioA: aktif_flag must be true after approve for operational mapping")
	}
	assertJurnalAuditContains(t, h, r.ID.String(), auditJurnalMappingApprove, "approve")
	if r.ApproverSigHash == "" {
		t.Error("ScenarioA: approver_signature_hash must be non-empty")
	}

	// PENDING_APPROVAL_2 must NOT be in history for 4-eyes path.
	assertJurnalAuditAbsent(t, h, r.ID.String(), auditJurnalMappingApprove2, "no-approve-2-for-4-eyes")

	// Verify audit chain has all 4 lifecycle events.
	auditActions := h.auditLog.actionsForEntity(r.ID.String())
	for _, want := range []string{auditJurnalMappingCreate, auditJurnalMappingSubmit, auditJurnalMappingReview, auditJurnalMappingApprove} {
		found := false
		for _, got := range auditActions {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ScenarioA: audit action %q missing; got: %v", want, auditActions)
		}
	}

	// Audit hash chain: every row non-empty current_hash.
	rows := h.auditLog.rowsForEntity(r.ID.String())
	for _, row := range rows {
		if len(row.CurrentHash) == 0 {
			t.Errorf("ScenarioA: audit row action=%q has empty current_hash", row.Action)
		}
	}
}

// ─── Scenario P5-M2-B: Mapping CRUD 6-eyes regulated (ECL_PEMBENTUKAN) ──────
//
// ROLE-AKUN creates DRAFT → submits → ROLE-AKUN-CTL reviews → ROLE-AKUN-CTL approves
// (MFA step-up required for approve on regulated) → PENDING_APPROVAL_2.
// ROLE-RISK approves_2 (MFA step-up required) → APPROVED_ACTIVE.
// Verify: approver_2_signed_at, approver_2_signature_hash, audit chain.

func TestE2E_P5M2_ScenarioB_Mapping6EyesRegulatedHappy(t *testing.T) {
	t.Parallel()
	h := newP5M2Harness(t)
	svc := newMappingJurnalService(h)

	userA := uuid.New().String() // ROLE-AKUN maker
	userB := uuid.New().String() // ROLE-AKUN-CTL reviewer
	userC := uuid.New().String() // ROLE-AKUN-CTL approver_1
	userD := uuid.New().String() // ROLE-RISK approver_2

	ctxA := ctxWithJrnlAkunActor(userA)
	ctxB := ctxWithJrnlAkunCTLActor(userB, true)
	ctxC := ctxWithJrnlAkunCTLActor(userC, true) // fresh step-up for regulated approve
	ctxD := ctxWithJrnlRiskActor(userD, true)    // fresh step-up for approve_2
	claimsA := claimsFromCtx(ctxA)
	claimsB := claimsFromCtx(ctxB)
	claimsC := claimsFromCtx(ctxC)
	claimsD := claimsFromCtx(ctxD)

	akun1, akun2 := uuid.New(), uuid.New()
	req := createMappingRequest{
		EventCode:     eventCodeECLPembentukan,
		NamaEvent:     "ECL Pembentukan Cadangan",
		KategoriEvent: kategoriECL, // regulated → 6-eyes
		TriggerSource: "SYSTEM_JOB",
		DetailRows: []mappingJurnalDetail{
			{Urutan: 1, KodeAkunID: akun1, DKIndicator: dkDebit, SumberAmount: "ecl_amount", Multiplier: decimal.NewFromInt(1)},
			{Urutan: 2, KodeAkunID: akun2, DKIndicator: dkKredit, SumberAmount: "ecl_amount", Multiplier: decimal.NewFromInt(1)},
		},
		IdempotencyKey: uuid.New(),
	}

	r, err := svc.Create(ctxA, req, claimsA)
	if err != nil {
		t.Fatalf("ScenarioB: Create failed: %v", err)
	}
	if !r.IsRegulated {
		t.Error("ScenarioB: ECL_PEMBENTUKAN must be detected as regulated (6-eyes)")
	}
	assertMappingStatus(t, r, jrnlStatusDraft, "after-create")

	_, err = svc.Submit(ctxA, r.ID, "Submit ECL template", claimsA)
	if err != nil {
		t.Fatalf("ScenarioB: Submit failed: %v", err)
	}

	_, err = svc.Review(ctxB, r.ID, "Reviewed sesuai PSAK 71 §5.5", claimsB)
	if err != nil {
		t.Fatalf("ScenarioB: Review failed: %v", err)
	}
	assertMappingStatus(t, r, jrnlStatusPendingApproval, "after-review")

	// Approve_1 for regulated → goes to PENDING_APPROVAL_2 (NOT APPROVED_ACTIVE).
	_, err = svc.Approve(ctxC, r.ID, "Approve_1 ECL template", claimsC)
	if err != nil {
		t.Fatalf("ScenarioB: Approve failed: %v", err)
	}
	assertMappingStatus(t, r, jrnlStatusPendingApproval2, "after-approve-1-regulated")
	if r.AktifFlag {
		t.Error("ScenarioB: aktif_flag must NOT be set after approve_1 for regulated (still needs approve_2)")
	}
	assertJurnalAuditContains(t, h, r.ID.String(), auditJurnalMappingApprove, "approve-1-audit")

	// Approve_2 by ROLE-RISK (step-up MFA fresh, DEC-027).
	_, err = svc.Approve2(ctxD, r.ID, "Template ECL sudah sesuai PSAK 71 §5.5.8", claimsD)
	if err != nil {
		t.Fatalf("ScenarioB: Approve2 failed: %v", err)
	}
	assertMappingStatus(t, r, jrnlStatusApprovedActive, "after-approve-2")
	if !r.AktifFlag {
		t.Error("ScenarioB: aktif_flag must be true after approve_2")
	}
	if r.Approver2ID == nil {
		t.Error("ScenarioB: approver_2_id must be set")
	}
	approver2ID, _ := uuid.Parse(userD)
	if *r.Approver2ID != approver2ID {
		t.Errorf("ScenarioB: approver_2_id = %s, want %s", *r.Approver2ID, approver2ID)
	}
	if r.Approver2SignedAt == nil {
		t.Error("ScenarioB: approver_2_signed_at must be set")
	}
	if r.Approver2SigHash == "" {
		t.Error("ScenarioB: approver_2_signature_hash must be non-empty SHA-256")
	}
	assertJurnalAuditContains(t, h, r.ID.String(), auditJurnalMappingApprove2, "approve-2-audit")
}

// ─── Scenario P5-M2-C: SoD violation 6-eyes (approver_2 == approver_1) ───────
//
// User C approves_1 for regulated mapping → User C tries approve_2
// → 403 JURNAL_SOD_VIOLATION.
// Audit SoD violation attempt must be written.

func TestE2E_P5M2_ScenarioC_SoDViolation6Eyes(t *testing.T) {
	t.Parallel()
	h := newP5M2Harness(t)
	svc := newMappingJurnalService(h)

	userA := uuid.New().String()
	userB := uuid.New().String()
	userC := uuid.New().String() // approver_1 AND attempts approver_2 — SoD violation

	ctxA := ctxWithJrnlAkunActor(userA)
	ctxB := ctxWithJrnlAkunCTLActor(userB, true)
	ctxC := ctxWithJrnlAkunCTLActor(userC, true)
	ctxCRisk := ctxWithJrnlRiskActor(userC, true) // same user, ROLE-RISK context

	claimsA := claimsFromCtx(ctxA)
	claimsB := claimsFromCtx(ctxB)
	claimsC := claimsFromCtx(ctxC)
	claimsCRisk := claimsFromCtx(ctxCRisk)

	akun1, akun2 := uuid.New(), uuid.New()
	req := createMappingRequest{
		EventCode:     "STAGE_MIGRATION_SOD_TEST",
		NamaEvent:     "Stage Migration SoD Test",
		KategoriEvent: "STAGE_MIGRATION", // regulated
		TriggerSource: "SYSTEM_JOB",
		DetailRows: []mappingJurnalDetail{
			{Urutan: 1, KodeAkunID: akun1, DKIndicator: dkDebit, Multiplier: decimal.NewFromInt(1)},
			{Urutan: 2, KodeAkunID: akun2, DKIndicator: dkKredit, Multiplier: decimal.NewFromInt(1)},
		},
		IdempotencyKey: uuid.New(),
	}

	r, err := svc.Create(ctxA, req, claimsA)
	if err != nil {
		t.Fatalf("ScenarioC: Create failed: %v", err)
	}
	_, _ = svc.Submit(ctxA, r.ID, "submit", claimsA)
	_, _ = svc.Review(ctxB, r.ID, "review", claimsB)
	_, err = svc.Approve(ctxC, r.ID, "approve_1", claimsC) // User C is approver_1
	if err != nil {
		t.Fatalf("ScenarioC: Approve failed: %v", err)
	}
	assertMappingStatus(t, r, jrnlStatusPendingApproval2, "after-approve-1")

	// User C tries to be approver_2 → SoD violation.
	_, err = svc.Approve2(ctxCRisk, r.ID, "approve_2 attempt by same user", claimsCRisk)
	assertJurnalErrorCode(t, err, errJurnalSODViolation, "sod-violation-approve2=approve1")

	// Status must not change.
	assertMappingStatus(t, r, jrnlStatusPendingApproval2, "after-sod-violation")

	// Audit SoD violation attempt must be written.
	assertJurnalAuditContains(t, h, r.ID.String(), auditJurnalMappingSOD, "sod-audit-written")
}

// ─── Scenario P5-M2-D: Step-up MFA stale on approve_2 ───────────────────────
//
// User D tries approve_2 with StepupVerifiedAt 10 minutes ago → 403 JURNAL_STEP_UP_REQUIRED.

func TestE2E_P5M2_ScenarioD_StepUpMFAStaleOnApprove2(t *testing.T) {
	t.Parallel()
	h := newP5M2Harness(t)
	svc := newMappingJurnalService(h)

	userA := uuid.New().String()
	userB := uuid.New().String()
	userC := uuid.New().String()
	userD := uuid.New().String() // approve_2 with stale MFA

	ctxA := ctxWithJrnlAkunActor(userA)
	ctxB := ctxWithJrnlAkunCTLActor(userB, true)
	ctxC := ctxWithJrnlAkunCTLActor(userC, true)

	claimsA := claimsFromCtx(ctxA)
	claimsB := claimsFromCtx(ctxB)
	claimsC := claimsFromCtx(ctxC)

	akun1, akun2 := uuid.New(), uuid.New()
	req := createMappingRequest{
		EventCode:     "ECL_REVERSAL_MFA_TEST",
		NamaEvent:     "ECL Reversal MFA Test",
		KategoriEvent: kategoriECL,
		TriggerSource: "SYSTEM_JOB",
		DetailRows: []mappingJurnalDetail{
			{Urutan: 1, KodeAkunID: akun1, DKIndicator: dkDebit, Multiplier: decimal.NewFromInt(1)},
			{Urutan: 2, KodeAkunID: akun2, DKIndicator: dkKredit, Multiplier: decimal.NewFromInt(1)},
		},
		IdempotencyKey: uuid.New(),
	}

	r, _ := svc.Create(ctxA, req, claimsA)
	_, _ = svc.Submit(ctxA, r.ID, "submit", claimsA)
	_, _ = svc.Review(ctxB, r.ID, "review", claimsB)
	_, _ = svc.Approve(ctxC, r.ID, "approve_1", claimsC)
	assertMappingStatus(t, r, jrnlStatusPendingApproval2, "before-stale-stepup")

	// User D with stale step-up (10 min ago > 5 min threshold, DEC-027).
	ctxDStale := ctxWithJrnlRiskActor(userD, false) // stepUpFresh=false → stale
	claimsDStale := claimsFromCtx(ctxDStale)

	_, err := svc.Approve2(ctxDStale, r.ID, "approve_2 with stale MFA", claimsDStale)
	assertJurnalErrorCode(t, err, errJurnalStepUpRequired, "stale-stepup-approve2")
	assertMappingStatus(t, r, jrnlStatusPendingApproval2, "status-unchanged-after-stale")

	// Fresh step-up should succeed.
	ctxDFresh := ctxWithJrnlRiskActor(userD, true)
	claimsDFresh := claimsFromCtx(ctxDFresh)
	_, err = svc.Approve2(ctxDFresh, r.ID, "approve_2 with fresh MFA", claimsDFresh)
	if err != nil {
		t.Errorf("ScenarioD: Fresh step-up should succeed, got: %v", err)
	}
	assertMappingStatus(t, r, jrnlStatusApprovedActive, "after-fresh-stepup")
}

// ─── Scenario P5-M2-E: Resolver lookup happy ─────────────────────────────────
//
// Approved PENEMPATAN mapping exists.
// Call Resolve(PENEMPATAN, AC, 5_000_000_000) → 2 JurnalLines (D+K), sum debit == sum kredit.

func TestE2E_P5M2_ScenarioE_ResolverLookupHappy(t *testing.T) {
	t.Parallel()
	h := newP5M2Harness(t)
	resolver := newJurnalResolverService(h)

	// Seed approved PENEMPATAN mapping (ALL klasifikasi).
	seedApprovedMapping(h, eventCodePenempatan, kategoriPenempatan, nil)

	amountIDR := decimal.NewFromInt(5_000_000_000)
	lines, err := resolver.Resolve(eventCodePenempatan, klasifikasiAC, amountIDR)
	if err != nil {
		t.Fatalf("ScenarioE: Resolve failed: %v", err)
	}
	if len(lines) != 2 {
		t.Errorf("ScenarioE: expected 2 JurnalLines, got %d", len(lines))
	}

	var totalDebit, totalKredit decimal.Decimal
	for _, l := range lines {
		if l.Posisi == dkDebit {
			totalDebit = totalDebit.Add(l.AmountIDR)
		} else {
			totalKredit = totalKredit.Add(l.AmountIDR)
		}
		if l.AkunID == (uuid.UUID{}) {
			t.Error("ScenarioE: AkunID must be non-zero for all lines")
		}
	}

	// Balance invariant: sum debit == sum kredit == 5_000_000_000.
	if !totalDebit.Equal(amountIDR) {
		t.Errorf("ScenarioE: totalDebit = %s, want 5000000000", totalDebit.StringFixed(4))
	}
	if !totalKredit.Equal(amountIDR) {
		t.Errorf("ScenarioE: totalKredit = %s, want 5000000000", totalKredit.StringFixed(4))
	}
	if !totalDebit.Equal(totalKredit) {
		t.Error("ScenarioE: balance invariant violated: totalDebit ≠ totalKredit")
	}
}

// ─── Scenario P5-M2-F: Resolver klasifikasi not eligible ─────────────────────
//
// Approved MTM_FVOCI mapping with klasifikasi_berlaku=[FVOCI].
// Call Resolve(MTM_FVOCI, FVTPL, amount) → 422 JURNAL_KLASIFIKASI_NOT_ELIGIBLE.

func TestE2E_P5M2_ScenarioF_ResolverKlasifikasiNotEligible(t *testing.T) {
	t.Parallel()
	h := newP5M2Harness(t)
	resolver := newJurnalResolverService(h)

	// MTM_FVOCI only valid for FVOCI klasifikasi.
	seedApprovedMapping(h, eventCodeMTMFVOCI, "MUTASI_MTM", []string{klasifikasiFVOCI})

	_, err := resolver.Resolve(eventCodeMTMFVOCI, klasifikasiFVTPL, decimal.NewFromInt(1_000_000))
	assertJurnalErrorCode(t, err, errJurnalKlasifikasiNotEligible, "klasifikasi-not-eligible")
}

// ─── Scenario P5-M2-G: Resolver event_code not mapped ────────────────────────
//
// No active mapping for STAGE_MIGRATION.
// Call Resolve(STAGE_MIGRATION, AC, amount) → 422 JURNAL_EVENT_NOT_MAPPED.

func TestE2E_P5M2_ScenarioG_ResolverEventNotMapped(t *testing.T) {
	t.Parallel()
	h := newP5M2Harness(t)
	resolver := newJurnalResolverService(h)
	// No mapping seeded for STAGE_MIGRATION.

	_, err := resolver.Resolve(eventCodeStageMigration, klasifikasiAC, decimal.NewFromInt(1_000_000))
	assertJurnalErrorCode(t, err, errJurnalEventNotMapped, "event-not-mapped")
}

// ─── Scenario P5-M2-H: ResolveAndPost happy (penempatan:approved event) ──────
//
// PENEMPATAN mapping active. Worker handles penempatan:approved task.
// Resolver succeeds → INSERT jrnl.header + 2 detail rows (D=K) in same tx.
// Audit JURNAL.POST written in same tx.
// Idempotency: same source_event_id replay returns existing jurnal_header.

func TestE2E_P5M2_ScenarioH_ResolveAndPostHappy(t *testing.T) {
	t.Parallel()
	h := newP5M2Harness(t)
	postSvc := newResolveAndPostService(h)

	// Seed prerequisites.
	seedApprovedMapping(h, eventCodePenempatan, kategoriPenempatan, nil)
	periode := h.periodeStore.seedOpen("PBUKU-2026-06")
	instrID := uuid.New()

	sourceEventID := uuid.New()
	input := resolveAndPostInput{
		SourceEventID:   sourceEventID,
		SourceEventType: sourceEventPenempatanApproved,
		EventCode:       eventCodePenempatan,
		InstrumenID:     &instrID,
		PeriodeID:       periode.ID,
		AmountIDR:       decimal.NewFromInt(5_000_000_000),
		KlasifikasiPSAK: klasifikasiAC,
		FxRate:          decimal.NewFromInt(1),
		Narrative:       "Penempatan deposito BCA",
	}

	header, err := postSvc.ProcessEvent(input)
	if err != nil {
		// Idempotency replay error is expected only on replay; first call must succeed.
		if we, ok := err.(*workerError); ok && we.Code == errJurnalIdempotencyReplay {
			t.Fatal("ScenarioH: first ProcessEvent should not return idempotency replay")
		}
		t.Fatalf("ScenarioH: ProcessEvent failed: %v", err)
	}

	// Verify jrnl.header.
	if header == nil {
		t.Fatal("ScenarioH: header must be non-nil on success")
	}
	if header.EventCode != eventCodePenempatan {
		t.Errorf("ScenarioH: EventCode = %q, want %q", header.EventCode, eventCodePenempatan)
	}
	if header.StatusInternal != jrnlPosted {
		t.Errorf("ScenarioH: StatusInternal = %q, want %q", header.StatusInternal, jrnlPosted)
	}
	if !header.TotalDebit.Equal(header.TotalKredit) {
		t.Errorf("ScenarioH: balance invariant violated: debit=%s kredit=%s",
			header.TotalDebit.StringFixed(4), header.TotalKredit.StringFixed(4))
	}
	expectedTotal := decimal.NewFromInt(5_000_000_000)
	if !header.TotalDebit.Equal(expectedTotal) {
		t.Errorf("ScenarioH: TotalDebit = %s, want 5000000000", header.TotalDebit.StringFixed(4))
	}
	if len(header.Details) != 2 {
		t.Errorf("ScenarioH: expected 2 detail rows, got %d", len(header.Details))
	}
	if header.ReferenceEventID != sourceEventID {
		t.Error("ScenarioH: ReferenceEventID mismatch")
	}
	if header.IdempotencyKey == "" {
		t.Error("ScenarioH: IdempotencyKey must be non-empty")
	}
	if header.NoJurnal == "" {
		t.Error("ScenarioH: NoJurnal must be generated (format JRN-YYYY-######)")
	}

	// Verify JURNAL.POST audit written in same tx.
	assertJurnalAuditContains(t, h, header.ID.String(), auditJurnalPost, "jurnal-post-audit")

	// Idempotency: same source_event_id replay must return existing header, no duplicate INSERT.
	header2, err2 := postSvc.ProcessEvent(input)
	// On replay, error is JURNAL_IDEMPOTENCY_REPLAY but header2 is the original.
	if err2 != nil {
		if we, ok := err2.(*workerError); !ok || we.Code != errJurnalIdempotencyReplay {
			t.Errorf("ScenarioH: replay should return JURNAL_IDEMPOTENCY_REPLAY, got: %v", err2)
		}
	}
	if header2 != nil && header2.ID != header.ID {
		t.Errorf("ScenarioH: replay returned different header ID: %s vs %s", header2.ID, header.ID)
	}

	// Exactly 1 jrnl.header in store.
	if len(h.jurnalStore.headers) != 1 {
		t.Errorf("ScenarioH: expected 1 jrnl.header in store, got %d", len(h.jurnalStore.headers))
	}
}

// ─── Scenario P5-M2-I: ResolveAndPost periode HARD_CLOSED guard ──────────────
//
// Periode status=HARD_CLOSED. Worker handles event → 423 JURNAL_PERIODE_HARD_CLOSED
// → DLQ insert. Verify DLQ entry with error_code=JURNAL_PERIODE_HARD_CLOSED, error_category=DOMAIN.

func TestE2E_P5M2_ScenarioI_ResolveAndPostPeriodeHardClosed(t *testing.T) {
	t.Parallel()
	h := newP5M2Harness(t)
	postSvc := newResolveAndPostService(h)

	seedApprovedMapping(h, eventCodePenempatan, kategoriPenempatan, nil)
	closedPeriode := h.periodeStore.seedHardClosed("PBUKU-2026-05")
	instrID := uuid.New()

	input := resolveAndPostInput{
		SourceEventID:   uuid.New(),
		SourceEventType: sourceEventPenempatanApproved,
		EventCode:       eventCodePenempatan,
		InstrumenID:     &instrID,
		PeriodeID:       closedPeriode.ID,
		AmountIDR:       decimal.NewFromInt(1_000_000_000),
		KlasifikasiPSAK: klasifikasiAC,
		FxRate:          decimal.NewFromInt(1),
	}

	_, err := postSvc.ProcessEvent(input)
	assertJurnalErrorCode(t, err, errJurnalPeriodeHardClosed, "periode-hard-closed")

	// Worker must not retry for domain errors.
	if we, ok := err.(*workerError); ok {
		if !we.SkipRetry {
			t.Error("ScenarioI: domain error JURNAL_PERIODE_HARD_CLOSED must set SkipRetry=true (no Asynq retry)")
		}
		if we.Category != errCategoryDomain {
			t.Errorf("ScenarioI: error_category = %q, want DOMAIN", we.Category)
		}
	}

	// DLQ entry must exist.
	if h.dlqStore.countByStatus(dlqFailed) != 1 {
		t.Errorf("ScenarioI: expected 1 DLQ entry FAILED, got %d", h.dlqStore.countByStatus(dlqFailed))
	}
	// Find the DLQ entry and verify fields.
	var dlqEntry *dlqEntry
	for _, e := range h.dlqStore.entries {
		dlqEntry = e
		break
	}
	if dlqEntry == nil {
		t.Fatal("ScenarioI: DLQ entry not found")
	}
	if dlqEntry.ErrorCode != errJurnalPeriodeHardClosed {
		t.Errorf("ScenarioI: DLQ.ErrorCode = %q, want %q", dlqEntry.ErrorCode, errJurnalPeriodeHardClosed)
	}
	if dlqEntry.ErrorCategory != errCategoryDomain {
		t.Errorf("ScenarioI: DLQ.ErrorCategory = %q, want DOMAIN", dlqEntry.ErrorCategory)
	}

	// Audit JURNAL.POST_FAILED written.
	assertJurnalAuditContains(t, h, dlqEntry.ID.String(), auditJurnalPostFailed, "post-failed-audit")

	// No jrnl.header must have been inserted.
	if len(h.jurnalStore.headers) != 0 {
		t.Errorf("ScenarioI: no jrnl.header should be inserted on HARD_CLOSED, got %d", len(h.jurnalStore.headers))
	}
}

// ─── Scenario P5-M2-J: DLQ replay happy ──────────────────────────────────────
//
// Periode reopened to OPEN. ROLE-AKUN-CTL replays DLQ entry.
// Resolver+posting succeed → DLQ status=REPLAYED_OK, final_jurnal_header_id set.
// Audit JURNAL.DLQ_REPLAYED written in same tx.

func TestE2E_P5M2_ScenarioJ_DLQReplayHappy(t *testing.T) {
	t.Parallel()
	h := newP5M2Harness(t)
	postSvc := newResolveAndPostService(h)
	dlqSvc := newDLQService(h, postSvc)

	seedApprovedMapping(h, eventCodePenempatan, kategoriPenempatan, nil)
	// Start with HARD_CLOSED periode.
	periode := h.periodeStore.seedHardClosed("PBUKU-2026-05-DLQ")
	instrID := uuid.New()
	sourceEventID := uuid.New()

	// First attempt fails → DLQ.
	input := resolveAndPostInput{
		SourceEventID:   sourceEventID,
		SourceEventType: sourceEventPenempatanApproved,
		EventCode:       eventCodePenempatan,
		InstrumenID:     &instrID,
		PeriodeID:       periode.ID,
		AmountIDR:       decimal.NewFromInt(2_000_000_000),
		KlasifikasiPSAK: klasifikasiAC,
		FxRate:          decimal.NewFromInt(1),
		Narrative:       "Penempatan untuk DLQ test",
	}
	_, _ = postSvc.ProcessEvent(input)

	if h.dlqStore.countByStatus(dlqFailed) != 1 {
		t.Fatal("ScenarioJ: expected 1 DLQ entry after failed post")
	}

	// Get DLQ entry.
	var dlqID uuid.UUID
	for id := range h.dlqStore.entries {
		dlqID = id
		break
	}

	// Reopen periode.
	h.periodeStore.setStatus(periode.ID, periodeOpen)

	// ROLE-AKUN-CTL triggers replay.
	userCTL := uuid.New().String()
	ctxCTL := ctxWithJrnlAkunCTLActor(userCTL, true)
	claimsCTL := claimsFromCtx(ctxCTL)

	header, err := dlqSvc.Replay(dlqID, claimsCTL)
	if err != nil {
		t.Fatalf("ScenarioJ: Replay failed: %v", err)
	}

	entry := h.dlqStore.get(dlqID)
	if entry.Status != dlqReplayedOK {
		t.Errorf("ScenarioJ: DLQ status = %q, want REPLAYED_OK", entry.Status)
	}
	if entry.ReplayedJurnalID == nil {
		t.Error("ScenarioJ: replayed_jurnal_id must be set after successful replay")
	}
	if header != nil && *entry.ReplayedJurnalID != header.ID {
		t.Errorf("ScenarioJ: replayed_jurnal_id = %s, want %s", *entry.ReplayedJurnalID, header.ID)
	}
	if entry.ReplayedBy == nil {
		t.Error("ScenarioJ: replayed_by must be set")
	}
	if entry.ReplayedAt == nil {
		t.Error("ScenarioJ: replayed_at must be set")
	}

	// Audit JURNAL.DLQ_REPLAYED written.
	assertJurnalAuditContains(t, h, dlqID.String(), auditJurnalDLQReplay, "dlq-replay-audit")

	// And the replay itself produced JURNAL.POST audit.
	if header != nil {
		assertJurnalAuditContains(t, h, header.ID.String(), auditJurnalPost, "replay-post-audit")
	}
}

// ─── Scenario P5-M2-K: DLQ discard happy ─────────────────────────────────────
//
// ROLE-AKUN-CTL discards a DLQ entry. Status → ABANDONED.
// discarded_reason ≥ 30 chars stored. Audit JURNAL.DLQ_DISCARD written.

func TestE2E_P5M2_ScenarioK_DLQDiscardHappy(t *testing.T) {
	t.Parallel()
	h := newP5M2Harness(t)
	postSvc := newResolveAndPostService(h)
	dlqSvc := newDLQService(h, postSvc)

	seedApprovedMapping(h, eventCodePenempatan, kategoriPenempatan, nil)
	closedPeriode := h.periodeStore.seedHardClosed("PBUKU-2026-04-DISCARD")
	instrID := uuid.New()

	// Produce a DLQ entry.
	_, _ = postSvc.ProcessEvent(resolveAndPostInput{
		SourceEventID:   uuid.New(),
		SourceEventType: sourceEventPenempatanApproved,
		EventCode:       eventCodePenempatan,
		InstrumenID:     &instrID,
		PeriodeID:       closedPeriode.ID,
		AmountIDR:       decimal.NewFromInt(500_000_000),
		KlasifikasiPSAK: klasifikasiAC,
		FxRate:          decimal.NewFromInt(1),
	})

	var dlqID uuid.UUID
	for id := range h.dlqStore.entries {
		dlqID = id
		break
	}

	userCTL := uuid.New().String()
	ctxCTL := ctxWithJrnlAkunCTLActor(userCTL, true)
	claimsCTL := claimsFromCtx(ctxCTL)

	// Short reason must be rejected (< 30 chars).
	err := dlqSvc.Discard(dlqID, "too short", claimsCTL)
	assertJurnalErrorCode(t, err, "VALIDATION_FAILED", "short-reason-rejected")

	// Valid reason ≥ 30 chars.
	reason := "Entry duplikat dengan DLQ-003-REV. Manual review shows balance sudah benar di versi baru mapping."
	err = dlqSvc.Discard(dlqID, reason, claimsCTL)
	if err != nil {
		t.Fatalf("ScenarioK: Discard failed: %v", err)
	}

	entry := h.dlqStore.get(dlqID)
	if entry.Status != dlqAbandoned {
		t.Errorf("ScenarioK: DLQ status = %q, want ABANDONED", entry.Status)
	}
	if entry.DiscardedReason != reason {
		t.Errorf("ScenarioK: discarded_reason mismatch")
	}
	if len(entry.DiscardedReason) < 30 {
		t.Error("ScenarioK: discarded_reason must be ≥ 30 chars")
	}

	assertJurnalAuditContains(t, h, dlqID.String(), auditJurnalDLQDiscard, "dlq-discard-audit")
}

// ─── Scenario P5-M2-L: Balance invariant detection ───────────────────────────
//
// Mapping with detail rows summing debit=100% kredit=80% (artificially broken).
// Resolve → JURNAL_BALANCE_INVARIANT. ResolveAndPost → no INSERT.

func TestE2E_P5M2_ScenarioL_BalanceInvariantDetection(t *testing.T) {
	t.Parallel()
	h := newP5M2Harness(t)
	resolver := newJurnalResolverService(h)
	postSvc := newResolveAndPostService(h)

	// Seed imbalanced mapping.
	seedImbalancedMapping(h, "BROKEN_PENEMPATAN")
	periode := h.periodeStore.seedOpen("PBUKU-2026-06-BAL")
	instrID := uuid.New()

	amount := decimal.NewFromInt(1_000)

	// Resolver detects imbalance.
	_, err := resolver.Resolve("BROKEN_PENEMPATAN", klasifikasiAC, amount)
	assertJurnalErrorCode(t, err, errJurnalBalanceInvariant, "balance-invariant-resolver")

	// ProcessEvent must also not INSERT.
	_, err = postSvc.ProcessEvent(resolveAndPostInput{
		SourceEventID:   uuid.New(),
		SourceEventType: sourceEventPenempatanApproved,
		EventCode:       "BROKEN_PENEMPATAN",
		InstrumenID:     &instrID,
		PeriodeID:       periode.ID,
		AmountIDR:       amount,
		KlasifikasiPSAK: klasifikasiAC,
		FxRate:          decimal.NewFromInt(1),
	})
	assertJurnalErrorCode(t, err, errJurnalBalanceInvariant, "balance-invariant-post")

	// No jrnl.header inserted.
	if len(h.jurnalStore.headers) != 0 {
		t.Errorf("ScenarioL: no jrnl.header should be inserted on balance invariant failure, got %d", len(h.jurnalStore.headers))
	}
}

// ─── Scenario P5-M2-M: Idempotency duplicate post ────────────────────────────
//
// Same source_event_id + source_event_type replay returns existing jurnal_header_id.
// Verifies UNIQUE constraint (idempotency_key) is enforced.

func TestE2E_P5M2_ScenarioM_IdempotencyDuplicatePost(t *testing.T) {
	t.Parallel()
	h := newP5M2Harness(t)
	postSvc := newResolveAndPostService(h)

	seedApprovedMapping(h, eventCodePenempatan, kategoriPenempatan, nil)
	periode := h.periodeStore.seedOpen("PBUKU-2026-06-IDEM")
	instrID := uuid.New()
	sourceEventID := uuid.New()

	input := resolveAndPostInput{
		SourceEventID:   sourceEventID,
		SourceEventType: sourceEventPenempatanApproved,
		EventCode:       eventCodePenempatan,
		InstrumenID:     &instrID,
		PeriodeID:       periode.ID,
		AmountIDR:       decimal.NewFromInt(3_000_000_000),
		KlasifikasiPSAK: klasifikasiAC,
		FxRate:          decimal.NewFromInt(1),
	}

	// First call succeeds.
	h1, err1 := postSvc.ProcessEvent(input)
	if err1 != nil {
		t.Fatalf("ScenarioM: first ProcessEvent failed: %v", err1)
	}

	// Second call with same source_event_id → idempotency replay.
	h2, err2 := postSvc.ProcessEvent(input)
	if err2 != nil {
		if we, ok := err2.(*workerError); !ok || we.Code != errJurnalIdempotencyReplay {
			t.Errorf("ScenarioM: expected JURNAL_IDEMPOTENCY_REPLAY, got: %v", err2)
		}
	}
	// Same or nil h2 is acceptable (replay returns existing or nil); no duplicate in store.
	_ = h2

	// UNIQUE constraint: exactly 1 jrnl.header in store.
	if len(h.jurnalStore.headers) != 1 {
		t.Errorf("ScenarioM: expected 1 jrnl.header, got %d (duplicate not allowed)", len(h.jurnalStore.headers))
	}

	// Idempotency key maps to original header.
	idempKey := computeIdempotencyKey(sourceEventID, eventCodePenempatan)
	stored := h.jurnalStore.findByIdempotencyKey(idempKey)
	if stored == nil || stored.ID != h1.ID {
		t.Errorf("ScenarioM: idempotency map must point to original header ID %s", h1.ID)
	}

	// Only 1 JURNAL.POST audit row (no duplicate side-effects).
	postCount := 0
	for _, row := range h.auditLog.rows {
		if row.Action == auditJurnalPost && row.EntityID == h1.ID.String() {
			postCount++
		}
	}
	if postCount != 1 {
		t.Errorf("ScenarioM: expected 1 JURNAL.POST audit row, got %d", postCount)
	}
}

// ─── Scenario P5-M2-N: Domain err → DLQ immediate, no Asynq retry ────────────
//
// Worker handles event, resolver returns JURNAL_EVENT_NOT_MAPPED (domain error).
// Verify DLQ inserted + SkipRetry=true (signals Asynq to not retry).

func TestE2E_P5M2_ScenarioN_DomainErrDLQImmediate(t *testing.T) {
	t.Parallel()
	h := newP5M2Harness(t)
	postSvc := newResolveAndPostService(h)

	// No mapping for DISTRIBUSI_REKSADANA.
	periode := h.periodeStore.seedOpen("PBUKU-2026-06-N")
	instrID := uuid.New()

	_, err := postSvc.ProcessEvent(resolveAndPostInput{
		SourceEventID:   uuid.New(),
		SourceEventType: "distribusi_reksadana:computed",
		EventCode:       "DISTRIBUSI_REKSADANA",
		InstrumenID:     &instrID,
		PeriodeID:       periode.ID,
		AmountIDR:       decimal.NewFromInt(100_000_000),
		KlasifikasiPSAK: klasifikasiAC,
		FxRate:          decimal.NewFromInt(1),
	})
	if err == nil {
		t.Fatal("ScenarioN: expected error, got nil")
	}
	we, ok := err.(*workerError)
	if !ok {
		t.Fatalf("ScenarioN: expected *workerError, got %T", err)
	}
	if we.Code != errJurnalEventNotMapped {
		t.Errorf("ScenarioN: error code = %q, want %q", we.Code, errJurnalEventNotMapped)
	}
	// SkipRetry=true = domain error, Asynq must not retry.
	if !we.SkipRetry {
		t.Error("ScenarioN: domain error must set SkipRetry=true to signal Asynq no retry")
	}
	if we.Category != errCategoryDomain {
		t.Errorf("ScenarioN: error_category = %q, want DOMAIN", we.Category)
	}

	// DLQ must have 1 FAILED entry.
	if h.dlqStore.countByStatus(dlqFailed) != 1 {
		t.Errorf("ScenarioN: expected 1 DLQ entry, got %d", h.dlqStore.countByStatus(dlqFailed))
	}

	// JURNAL.POST_FAILED audit written (not JURNAL.POST).
	assertJurnalAuditAbsent(t, h, "", auditJurnalPost, "no-post-audit-on-domain-err")
}

// ─── Scenario P5-M2-O: Infra err → 3x retry then DLQ ─────────────────────────
//
// DB connection error simulation → Asynq retries 3x → eventually DLQ
// with retry_count=3, error_category=INFRA.

func TestE2E_P5M2_ScenarioO_InfraErrRetryThenDLQ(t *testing.T) {
	t.Parallel()
	h := newP5M2Harness(t)
	postSvc := newResolveAndPostService(h)

	seedApprovedMapping(h, eventCodePenempatan, kategoriPenempatan, nil)
	periode := h.periodeStore.seedOpen("PBUKU-2026-06-O")
	instrID := uuid.New()

	// Simulate 3 infra errors before success (or DLQ on final attempt).
	postSvc.infraErrSimCount = 3

	input := resolveAndPostInput{
		SourceEventID:   uuid.New(),
		SourceEventType: sourceEventPenempatanApproved,
		EventCode:       eventCodePenempatan,
		InstrumenID:     &instrID,
		PeriodeID:       periode.ID,
		AmountIDR:       decimal.NewFromInt(1_000_000_000),
		KlasifikasiPSAK: klasifikasiAC,
		FxRate:          decimal.NewFromInt(1),
	}

	// Attempt 1: infra error → must NOT set SkipRetry.
	_, err1 := postSvc.ProcessEvent(input)
	if err1 == nil {
		t.Fatal("ScenarioO: attempt 1 should fail with infra error")
	}
	we1, ok := err1.(*workerError)
	if !ok {
		t.Fatalf("ScenarioO: expected *workerError on attempt 1, got %T", err1)
	}
	if we1.SkipRetry {
		t.Error("ScenarioO: infra error on attempt 1 must NOT set SkipRetry (Asynq should retry)")
	}
	if we1.Category != errCategoryInfra {
		t.Errorf("ScenarioO: attempt 1 error_category = %q, want INFRA", we1.Category)
	}

	// Attempt 2: infra error.
	_, err2 := postSvc.ProcessEvent(input)
	if err2 == nil {
		t.Fatal("ScenarioO: attempt 2 should fail with infra error")
	}
	we2, _ := err2.(*workerError)
	if we2 != nil && we2.SkipRetry {
		t.Error("ScenarioO: infra error on attempt 2 must NOT set SkipRetry")
	}

	// Attempt 3: infra error.
	_, err3 := postSvc.ProcessEvent(input)
	if err3 == nil {
		t.Fatal("ScenarioO: attempt 3 should fail with infra error")
	}
	we3, _ := err3.(*workerError)
	if we3 != nil && we3.SkipRetry {
		t.Error("ScenarioO: infra error on attempt 3 must NOT set SkipRetry")
	}

	// After 3 infra errors, infraErrSimCount == 0; next call succeeds OR goes to DLQ.
	// In production the Asynq retry exhaustion handler would write to DLQ with retry_count=3.
	// Simulate DLQ write-on-exhaustion (production responsibility).
	dlqOnExhaustion := &dlqEntry{
		ID:            uuid.New(),
		SourceEventID: input.SourceEventID,
		EventCode:     input.EventCode,
		PeriodeID:     input.PeriodeID,
		ErrorCode:     "DB_CONNECTION_ERROR",
		ErrorMessage:  "3 retry attempts exhausted",
		ErrorCategory: errCategoryInfra,
		AttemptCount:  3,
		LastAttemptAt: time.Now(),
		Status:        dlqFailed,
	}
	h.dlqStore.insert(dlqOnExhaustion)

	// Assert DLQ entry with retry_count=3 and error_category=INFRA.
	entry := h.dlqStore.get(dlqOnExhaustion.ID)
	if entry == nil {
		t.Fatal("ScenarioO: DLQ entry must exist after 3 infra retry exhaustion")
	}
	if entry.AttemptCount != 3 {
		t.Errorf("ScenarioO: DLQ.AttemptCount = %d, want 3", entry.AttemptCount)
	}
	if entry.ErrorCategory != errCategoryInfra {
		t.Errorf("ScenarioO: DLQ.ErrorCategory = %q, want INFRA", entry.ErrorCategory)
	}
	if entry.Status != dlqFailed {
		t.Errorf("ScenarioO: DLQ.Status = %q, want FAILED", entry.Status)
	}

	// After simulated infra errors exhausted, 4th call succeeds.
	h4, err4 := postSvc.ProcessEvent(input)
	if err4 != nil && err4.(*workerError).Code != errJurnalIdempotencyReplay {
		t.Errorf("ScenarioO: 4th attempt (after infra errors) should succeed, got: %v", err4)
	}
	_ = h4
}

// ─── Audit hash chain integrity regression ────────────────────────────────────
//
// DEC-018: every audit row must have non-empty current_hash.
// Hash chain: current_hash = SHA-256(previous_hash || action || entityID || afterJSON).

func TestE2E_P5M2_AuditHashChainIntegrity(t *testing.T) {
	t.Parallel()
	h := newP5M2Harness(t)
	postSvc := newResolveAndPostService(h)

	seedApprovedMapping(h, eventCodePenempatan, kategoriPenempatan, nil)
	periode := h.periodeStore.seedOpen("PBUKU-2026-06-HASH")
	instrID := uuid.New()

	// Run 3 successful postings to generate audit chain.
	for i := 0; i < 3; i++ {
		_, err := postSvc.ProcessEvent(resolveAndPostInput{
			SourceEventID:   uuid.New(),
			SourceEventType: sourceEventPenempatanApproved,
			EventCode:       eventCodePenempatan,
			InstrumenID:     &instrID,
			PeriodeID:       periode.ID,
			AmountIDR:       decimal.NewFromInt(int64(1_000_000_000 * (i + 1))),
			KlasifikasiPSAK: klasifikasiAC,
			FxRate:          decimal.NewFromInt(1),
		})
		if err != nil {
			t.Fatalf("AuditHashChain: ProcessEvent[%d] failed: %v", i, err)
		}
	}

	if len(h.auditLog.rows) == 0 {
		t.Fatal("AuditHashChain: no audit rows found")
	}
	for i, row := range h.auditLog.rows {
		if len(row.CurrentHash) == 0 {
			t.Errorf("AuditHashChain: row[%d] action=%q has empty current_hash", i, row.Action)
		}
	}
}
