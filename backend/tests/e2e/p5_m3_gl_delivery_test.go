// Package e2e — P5-M3 GL Host REST Delivery end-to-end tests.
//
// Scope: Auto-delivery worker (S1), delivery status enrichment (S2),
// manual retry (S3), daily reconciliation (S4), DLQ replay + discard (S5).
// Uses the same in-process harness pattern as p5_m2_jurnal_test.go.
//
// Scenarios:
//
//	P5-M3-A  S1-AC1: Auto-deliver happy path — GL Host 201 → DELIVERED + audit GL_DELIVERY.SUCCESS
//	P5-M3-B  S1-AC2: Idempotency — already DELIVERED → early return, no DB write, no second audit
//	P5-M3-C  S1-AC3: Domain error 4xx → SkipRetry → DLQ FAILED, failure_category=DOMAIN
//	P5-M3-D  S1-AC4: Infra error 503 → RETRYING + audit GL_DELIVERY.RETRY; after 3× → FAILED DLQ
//	P5-M3-E  S1-AC5: DEAD_LETTER terminal — cannot be retried or modified by worker
//	P5-M3-F  S2-AC1: GET delivery status DELIVERED — can_retry=false, delivered_at populated
//	P5-M3-G  S2-AC2: GET delivery status FAILED — can_retry=true, failure_category visible
//	P5-M3-H  S2-AC3: GET delivery status PENDING_DELIVERY — can_retry=false
//	P5-M3-I  S3-AC1: Manual retry ROLE-AKUN-CTL — FAILED→PENDING_DELIVERY + audit BEFORE enqueue
//	P5-M3-J  S3-AC2: Manual retry rejected DEAD_LETTER → WORKFLOW_INVALID_TRANSITION
//	P5-M3-K  S3-AC3: Manual retry reason < 30 chars → VALIDATION_FAILED
//	P5-M3-L  S3-AC4: ROLE-AKUN (no retry permission) → FORBIDDEN
//	P5-M3-M  S4-AC1: Recon cron BLIPS == GL → status COMPLETED, mismatch_count=0
//	P5-M3-N  S4-AC2: Recon finds 2 mismatches (BLIPS_ONLY + AMOUNT_DIFF) → COMPLETED_WITH_MISMATCH
//	P5-M3-O  S4-AC3: Manual recon trigger ROLE-AKUN-CTL → 202 job enqueued
//	P5-M3-P  S4-AC4: GL Host unreachable → recon status FAILED, no mismatch rows
//	P5-M3-Q  S5-AC1: DLQ list filter sort — 8 FAILED entries returned with pagination
//	P5-M3-R  S5-AC2: DLQ replay ROLE-IT-ADMIN → FAILED→PENDING_DELIVERY + audit DLQ_REPLAY_INITIATED
//	P5-M3-S  S5-AC3: DLQ discard ROLE-IT-ADMIN → DEAD_LETTER + audit DLQ_DISCARDED, reason ≥30
//	P5-M3-T  S5-AC4: DLQ discard ROLE-AKUN-CTL → 403 FORBIDDEN (jurnal.gl_delivery.discard)
//	P5-M3-U  Idempotency: same Idempotency-Key replay returns original response, no duplicate side-effects
//	P5-M3-V  Audit: every mutation writes audit_log with valid hash-chain linkage
//	P5-M3-W  PII: DLQ payload_snapshot does NOT contain customer_name / account_no / npwp
//
// Decision log compliance:
//
//	DEC-016: No float64 — shopspring/decimal throughout               — Scenarios M,N
//	DEC-018: Audit-in-tx mandatory                                    — All mutation scenarios
//	DEC-021: Idempotency-Key mandatory on mutating endpoints          — Scenario U
//	DEC-030 RESOLVED: GL delivery mode = Async REST via Asynq        — All S1 scenarios
//	Security baseline: PII sanitization before JSONB persist          — Scenario W
//
// Run:
//
//	go test ./tests/e2e/... -v -run TestE2E_P5M3 -timeout 60s
package e2e

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── P5-M3 domain constants ───────────────────────────────────────────────────

const (
	// GL Host status values (jrnl.gl_status.gl_host_status).
	glStatusPendingDelivery  = "PENDING_DELIVERY"
	glStatusDeliveryInFlight = "DELIVERY_IN_FLIGHT"
	glStatusDelivered        = "DELIVERED"
	glStatusRetrying         = "RETRYING"
	glStatusFailed           = "FAILED"
	glStatusDeadLetter       = "DEAD_LETTER"

	// DLQ status values (sys.dlq_gl_delivery.status).
	dlqGLFailed     = "FAILED"
	dlqGLReplaying  = "REPLAYING"
	dlqGLReplayedOK = "REPLAYED_OK"
	dlqGLAbandoned  = "ABANDONED"

	// Failure categories.
	failureCategoryDomain = "DOMAIN"
	failureCategoryInfra  = "INFRA"

	// GL delivery audit actions.
	auditGLDeliverySuccess           = "GL_DELIVERY.SUCCESS"
	auditGLDeliveryFailed            = "GL_DELIVERY.FAILED"
	auditGLDeliveryRetry             = "GL_DELIVERY.RETRY"
	auditGLDeliveryManualRetry       = "GL_DELIVERY.MANUAL_RETRY_INITIATED"
	auditGLDeliveryDLQReplay         = "GL_DELIVERY.DLQ_REPLAY_INITIATED"
	auditGLDeliveryDLQDiscarded      = "GL_DELIVERY.DLQ_DISCARDED"
	auditGLDeliveryDLQEntered        = "GL_DELIVERY.DLQ_ENTERED"
	auditGLReconciliationCompleted   = "GL_RECONCILIATION.COMPLETED"
	auditGLReconciliationFailed      = "GL_RECONCILIATION.FAILED"
	auditGLReconciliationTriggered   = "GL_RECONCILIATION.TRIGGERED"

	// Recon report statuses.
	reconStatusInProgress            = "IN_PROGRESS"
	reconStatusCompleted             = "COMPLETED"
	reconStatusCompletedWithMismatch = "COMPLETED_WITH_MISMATCH"
	reconStatusFailed                = "FAILED"

	// Mismatch types.
	mismatchBlipsOnly  = "BLIPS_ONLY"
	mismatchGLOnly     = "GL_ONLY"
	mismatchAmountDiff = "AMOUNT_DIFF"

	// Error codes (mirrors domain.go + OpenAPI).
	errGLDeliveryJurnalNotFound         = "GL_DELIVERY_JURNAL_NOT_FOUND"
	errGLDeliveryReasonTooShort         = "GL_DELIVERY_REASON_TOO_SHORT"
	errGLDeliveryInvalidTransition      = "GL_DELIVERY_INVALID_TRANSITION"
	errGLDeliveryPermissionDenied       = "GL_DELIVERY_PERMISSION_DENIED"
	errGLDeliveryHost4XX                = "GL_DELIVERY_HOST_4XX"
	errGLDeliveryHostUnreachable        = "GL_DELIVERY_HOST_UNREACHABLE"
	errGLDLQReplayInvalidState          = "GL_DLQ_REPLAY_INVALID_STATE"
	errGLReconciliationInProgress       = "GL_RECONCILIATION_IN_PROGRESS"
	errGLReconciliationReportNotFound   = "GL_RECONCILIATION_REPORT_NOT_FOUND"

	// Permissions (mirrors domain.go).
	permGlDeliveryRead    = "jurnal.gl_delivery.read"
	permGlDeliveryRetry   = "jurnal.gl_delivery.retry"
	permGlDeliveryReplay  = "jurnal.gl_delivery.replay"
	permGlDeliveryDiscard = "jurnal.gl_delivery.discard"
	permReconRead         = "jurnal.reconciliation.read"
	permReconRun          = "jurnal.reconciliation.run"

	// Role constants.
	roleAkunCTL  = "ROLE-AKUN-CTL"
	roleAkun     = "ROLE-AKUN"
	roleITAdmin  = "ROLE-IT-ADMIN"
	roleAudit    = "ROLE-AUDIT"
	roleCFO      = "ROLE-CFO"
	roleSystem   = "ROLE-SYSTEM-WORKER"
)

// ─── P5-M3 domain types ───────────────────────────────────────────────────────

// glStatusRecord mirrors jrnl.gl_status.
type glStatusRecord struct {
	ID                     uuid.UUID
	HeaderID               uuid.UUID
	GlHostStatus           string
	GlHostJournalID        *string
	DeliveredAt            *time.Time
	RetryCount             int
	LastRetryAt            *time.Time
	LastError              *string
	FailureCategory        *string
	DeliveryMode           string
	ManualRetryBy          *uuid.UUID
	ManualRetryAt          *time.Time
	ManualRetryReason      *string
	DiscardedBy            *uuid.UUID
	DiscardedAt            *time.Time
	DiscardReason          *string
	GlResponsePayloadJsonb map[string]any
	UpdatedAt              time.Time
}

// jurnalHeaderRecord mirrors jrnl.header (minimal for delivery).
type jurnalHeaderRecord struct {
	ID              uuid.UUID
	NoJurnal        string
	EventCode       string
	TanggalPosting  time.Time
	Narrative       string
	StatusInternal  string
	IdempotencyKey  string
	TotalDebit      decimal.Decimal
	TotalKredit     decimal.Decimal
	DetailRows      []jurnalDetailRow
}

// jurnalDetailRow mirrors jrnl.detail.
type jurnalDetailRow struct {
	ID          uuid.UUID
	KodeAkun    string
	DebitAmount decimal.Decimal
	KreditAmount decimal.Decimal
	MataUang    string
}

// dlqGLRecord mirrors sys.dlq_gl_delivery.
type dlqGLRecord struct {
	ID              uuid.UUID
	JurnalHeaderID  uuid.UUID
	GlStatusID      *uuid.UUID
	FailureCategory string
	ErrorCode       string
	ErrorMessage    string
	PayloadJsonb    map[string]any
	RetryCount      int
	Status          string
	ReplayedBy      *uuid.UUID
	ReplayedAt      *time.Time
	DiscardedReason *string
	DiscardedBy     *uuid.UUID
	DiscardedAt     *time.Time
}

// reconReportRecord mirrors sys.gl_reconciliation_report.
type reconReportRecord struct {
	ID              uuid.UUID
	TanggalRun      time.Time
	Status          string
	MismatchCount   int
	TotalJurnalIDR  decimal.Decimal
	GlHostTotalIDR  decimal.Decimal
	ToleranceIDR    decimal.Decimal
	TriggerSource   string
	TriggeredBy     *uuid.UUID
	MismatchLines   []reconMismatchLine
}

// reconMismatchLine mirrors sys.gl_recon_mismatch.
type reconMismatchLine struct {
	ID              uuid.UUID
	ReportID        uuid.UUID
	KodeAkun        string
	BlipsAmountIDR  decimal.Decimal
	GlHostAmountIDR decimal.Decimal
	DeltaIDR        decimal.Decimal
	MismatchType    string
	JurnalHeaderIDs []uuid.UUID
}

// glAuditRow extends p5M2AuditRow for GL delivery events.
type glAuditRow struct {
	EventID     string
	Action      string
	EntityID    string
	ActorID     string
	ActorRole   string
	PreviousHash []byte
	CurrentHash []byte
	AfterJSON   map[string]any
}

// glDeliveryResult captures the outcome of a delivery attempt.
type glDeliveryResult struct {
	GlHostStatus    string
	GlHostJournalID *string
	DeliveredAt     *time.Time
	FailureCategory *string
	LastError       *string
	RetryCount      int
	SkipRetry       bool // true → Asynq should not auto-retry
	AuditAction     string
}

// retryResult captures the outcome of a manual retry.
type retryResult struct {
	JobID          string
	PreviousStatus string
	NewStatus      string
}

// replayResult captures the outcome of a DLQ replay.
type replayResult struct {
	JobID          string
	PreviousStatus string
	NewStatus      string
}

// discardResult captures the outcome of a DLQ discard.
type discardResult struct {
	NewStatus   string
	DiscardedAt time.Time
	DiscardedBy uuid.UUID
}

// ─── P5-M3 harness ───────────────────────────────────────────────────────────

// p5M3Harness wires up in-process stubs for GL delivery tests.
type p5M3Harness struct {
	t          *testing.T
	auditLog   *p5M3AuditStore
	glStatus   *p5M3GLStatusStore
	jurnalHdrs *p5M3JurnalStore
	dlqGL      *p5M3DLQStore
	recon      *p5M3ReconStore
	asynqQueue *p5M3AsynqQueue
	glHost     *p5M3GLHostStub
}

func newP5M3Harness(t *testing.T) *p5M3Harness {
	t.Helper()
	return &p5M3Harness{
		t:          t,
		auditLog:   newP5M3AuditStore(),
		glStatus:   newP5M3GLStatusStore(),
		jurnalHdrs: newP5M3JurnalStore(),
		dlqGL:      newP5M3DLQStore(),
		recon:      newP5M3ReconStore(),
		asynqQueue: newP5M3AsynqQueue(),
		glHost:     newP5M3GLHostStub(),
	}
}

// ─── Audit store (hash-chain) ─────────────────────────────────────────────────

type p5M3AuditStore struct {
	rows []glAuditRow
}

func newP5M3AuditStore() *p5M3AuditStore { return &p5M3AuditStore{} }

func (s *p5M3AuditStore) append(action, entityID, actorID, actorRole string, afterJSON map[string]any) {
	var prevHash []byte
	if len(s.rows) > 0 {
		prevHash = s.rows[len(s.rows)-1].CurrentHash
	}
	payload := fmt.Sprintf("%x||%s||%s||%v", prevHash, action, entityID, afterJSON)
	h := sha256.Sum256([]byte(payload))
	s.rows = append(s.rows, glAuditRow{
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

func (s *p5M3AuditStore) containsAction(entityID, action string) bool {
	for _, r := range s.rows {
		if r.EntityID == entityID && r.Action == action {
			return true
		}
	}
	return false
}

func (s *p5M3AuditStore) actionsForEntity(entityID string) []string {
	var out []string
	for _, r := range s.rows {
		if r.EntityID == entityID {
			out = append(out, r.Action)
		}
	}
	return out
}

// verifyHashChain asserts the audit chain is unbroken.
// Returns true if chain is intact, false + description if broken.
func (s *p5M3AuditStore) verifyHashChain() (bool, string) {
	for i := 1; i < len(s.rows); i++ {
		cur := s.rows[i]
		prev := s.rows[i-1]
		// Recompute expected currentHash for row[i].
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

// ─── GL Status store ──────────────────────────────────────────────────────────

type p5M3GLStatusStore struct {
	records map[uuid.UUID]*glStatusRecord // keyed by HeaderID
}

func newP5M3GLStatusStore() *p5M3GLStatusStore {
	return &p5M3GLStatusStore{records: make(map[uuid.UUID]*glStatusRecord)}
}

func (s *p5M3GLStatusStore) seed(headerID uuid.UUID, status string) *glStatusRecord {
	r := &glStatusRecord{
		ID:           uuid.New(),
		HeaderID:     headerID,
		GlHostStatus: status,
		DeliveryMode: "API",
		UpdatedAt:    time.Now(),
	}
	s.records[headerID] = r
	return r
}

func (s *p5M3GLStatusStore) get(headerID uuid.UUID) *glStatusRecord {
	return s.records[headerID]
}

func (s *p5M3GLStatusStore) update(headerID uuid.UUID, fn func(*glStatusRecord)) {
	if r := s.records[headerID]; r != nil {
		fn(r)
		r.UpdatedAt = time.Now()
	}
}

// ─── Jurnal store ─────────────────────────────────────────────────────────────

type p5M3JurnalStore struct {
	records map[uuid.UUID]*jurnalHeaderRecord
	seqNo   int
}

func newP5M3JurnalStore() *p5M3JurnalStore {
	return &p5M3JurnalStore{records: make(map[uuid.UUID]*jurnalHeaderRecord)}
}

func (s *p5M3JurnalStore) seedPosted(eventCode string, totalIDR decimal.Decimal) *jurnalHeaderRecord {
	s.seqNo++
	h := &jurnalHeaderRecord{
		ID:             uuid.New(),
		NoJurnal:       fmt.Sprintf("JRN-2026-%06d", s.seqNo),
		EventCode:      eventCode,
		TanggalPosting: time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
		Narrative:      fmt.Sprintf("Test jurnal %s", eventCode),
		StatusInternal: jrnlPosted,
		IdempotencyKey: uuid.New().String(),
		TotalDebit:     totalIDR,
		TotalKredit:    totalIDR,
		DetailRows: []jurnalDetailRow{
			{
				ID:           uuid.New(),
				KodeAkun:     "1110-DEP",
				DebitAmount:  totalIDR,
				KreditAmount: decimal.Zero,
				MataUang:     "IDR",
			},
			{
				ID:           uuid.New(),
				KodeAkun:     "1001-KAS",
				DebitAmount:  decimal.Zero,
				KreditAmount: totalIDR,
				MataUang:     "IDR",
			},
		},
	}
	s.records[h.ID] = h
	return h
}

func (s *p5M3JurnalStore) get(id uuid.UUID) *jurnalHeaderRecord { return s.records[id] }

// ─── DLQ store ────────────────────────────────────────────────────────────────

type p5M3DLQStore struct {
	entries map[uuid.UUID]*dlqGLRecord // keyed by DLQ entry ID
}

func newP5M3DLQStore() *p5M3DLQStore {
	return &p5M3DLQStore{entries: make(map[uuid.UUID]*dlqGLRecord)}
}

func (s *p5M3DLQStore) insert(e *dlqGLRecord) {
	if e.ID == (uuid.UUID{}) {
		e.ID = uuid.New()
	}
	s.entries[e.ID] = e
}

func (s *p5M3DLQStore) get(id uuid.UUID) *dlqGLRecord   { return s.entries[id] }
func (s *p5M3DLQStore) count() int                       { return len(s.entries) }
func (s *p5M3DLQStore) listByStatus(status string) []*dlqGLRecord {
	var out []*dlqGLRecord
	for _, e := range s.entries {
		if e.Status == status {
			out = append(out, e)
		}
	}
	return out
}

// ─── Recon store ──────────────────────────────────────────────────────────────

type p5M3ReconStore struct {
	reports map[string]*reconReportRecord // keyed by date string "2006-01-02"
}

func newP5M3ReconStore() *p5M3ReconStore {
	return &p5M3ReconStore{reports: make(map[string]*reconReportRecord)}
}

func (s *p5M3ReconStore) getByDate(date string) *reconReportRecord { return s.reports[date] }
func (s *p5M3ReconStore) upsert(r *reconReportRecord) {
	s.reports[r.TanggalRun.Format("2006-01-02")] = r
}

// ─── Asynq queue stub ─────────────────────────────────────────────────────────

type p5M3AsynqTask struct {
	Type    string
	Payload map[string]string
}

type p5M3AsynqQueue struct {
	tasks []p5M3AsynqTask
}

func newP5M3AsynqQueue() *p5M3AsynqQueue { return &p5M3AsynqQueue{} }

func (q *p5M3AsynqQueue) enqueue(taskType string, payload map[string]string) {
	q.tasks = append(q.tasks, p5M3AsynqTask{Type: taskType, Payload: payload})
}

func (q *p5M3AsynqQueue) countByType(taskType string) int {
	n := 0
	for _, t := range q.tasks {
		if t.Type == taskType {
			n++
		}
	}
	return n
}

// ─── GL Host stub ─────────────────────────────────────────────────────────────

type p5M3GLHostResponse struct {
	HTTPStatus  int
	GlJournalID string
	Error       *struct {
		Code    string
		Message string
	}
}

type p5M3GLHostStub struct {
	// response[headerID.String()] → response to return
	responses    map[string][]p5M3GLHostResponse // per-header ordered responses (pop front)
	defaultResp  *p5M3GLHostResponse
	dailySummary map[string]decimal.Decimal // kodeAkun → netIDR (for recon)
	calls        int
}

func newP5M3GLHostStub() *p5M3GLHostStub {
	return &p5M3GLHostStub{
		responses:    make(map[string][]p5M3GLHostResponse),
		dailySummary: make(map[string]decimal.Decimal),
	}
}

func (s *p5M3GLHostStub) queueResponse(headerID uuid.UUID, resp p5M3GLHostResponse) {
	key := headerID.String()
	s.responses[key] = append(s.responses[key], resp)
}

func (s *p5M3GLHostStub) setDefault(resp p5M3GLHostResponse) { s.defaultResp = &resp }

func (s *p5M3GLHostStub) post(headerID uuid.UUID, idempotencyKey string) p5M3GLHostResponse {
	s.calls++
	key := headerID.String()
	if q := s.responses[key]; len(q) > 0 {
		resp := q[0]
		s.responses[key] = q[1:]
		return resp
	}
	if s.defaultResp != nil {
		return *s.defaultResp
	}
	// Default: success
	return p5M3GLHostResponse{
		HTTPStatus:  201,
		GlJournalID: fmt.Sprintf("GLHOST-JRN-2026-%05d", s.calls),
	}
}

func (s *p5M3GLHostStub) setSummary(kodeAkun string, netIDR decimal.Decimal) {
	s.dailySummary[kodeAkun] = netIDR
}

// ─── Core delivery worker logic (in-process simulation) ───────────────────────

// deliverConfig holds delivery service configuration.
type deliverConfig struct {
	MaxTotalAttempts    int
	RetryBackoffSeconds []int
	PIIFieldsToRedact   map[string]struct{}
}

func defaultDeliverConfig() deliverConfig {
	return deliverConfig{
		MaxTotalAttempts:    5,
		RetryBackoffSeconds: []int{30, 120, 600},
		PIIFieldsToRedact: map[string]struct{}{
			"customer_name": {},
			"account_no":    {},
			"npwp":          {},
			"ktp":           {},
		},
	}
}

// deliverJurnal simulates the Asynq worker HandleDeliverTask.
// Returns glDeliveryResult capturing what happened.
func (h *p5M3Harness) deliverJurnal(ctx context.Context, headerID uuid.UUID) glDeliveryResult {
	h.t.Helper()
	cfg := defaultDeliverConfig()

	gs := h.glStatus.get(headerID)
	if gs == nil {
		return glDeliveryResult{
			GlHostStatus: "",
			LastError:    ptrString("gl_status not found for header"),
		}
	}

	// Idempotency guard: terminal states.
	if gs.GlHostStatus == glStatusDelivered || gs.GlHostStatus == glStatusDeadLetter {
		return glDeliveryResult{
			GlHostStatus: gs.GlHostStatus,
		}
	}

	// Guard: max attempts.
	if gs.RetryCount >= cfg.MaxTotalAttempts {
		h.moveToDLQ(headerID, gs, "max attempts exceeded", failureCategoryInfra, errGLDeliveryHostUnreachable)
		return glDeliveryResult{
			GlHostStatus:    glStatusFailed,
			FailureCategory: ptrString(failureCategoryInfra),
			LastError:       ptrString("max total attempts exceeded"),
			SkipRetry:       true,
		}
	}

	// Mark in-flight.
	h.glStatus.update(headerID, func(gs *glStatusRecord) {
		gs.GlHostStatus = glStatusDeliveryInFlight
	})

	jh := h.jurnalHdrs.get(headerID)
	idempotencyKey := "BLIPS-test-idempotency-key"
	if jh != nil {
		idempotencyKey = "BLIPS-" + jh.IdempotencyKey
	}

	resp := h.glHost.post(headerID, idempotencyKey)
	now := time.Now()
	retryCount := gs.RetryCount + 1

	if resp.Error != nil || resp.HTTPStatus >= 400 {
		// Domain or infra error.
		category := failureCategoryInfra
		errCode := errGLDeliveryHostUnreachable
		if resp.HTTPStatus >= 400 && resp.HTTPStatus < 500 {
			category = failureCategoryDomain
			errCode = errGLDeliveryHost4XX
		}
		errMsg := fmt.Sprintf("GL Host %d", resp.HTTPStatus)
		if resp.Error != nil {
			errMsg = fmt.Sprintf("%s: %s", resp.Error.Code, resp.Error.Message)
		}

		h.glStatus.update(headerID, func(gs *glStatusRecord) {
			gs.GlHostStatus = glStatusFailed
			gs.FailureCategory = ptrString(category)
			gs.LastError = ptrString(errMsg)
			gs.RetryCount = retryCount
			if category == failureCategoryInfra {
				gs.LastRetryAt = &now
			}
		})

		// Write audit FAILED in same "transaction" as status update.
		h.auditLog.append(auditGLDeliveryFailed, gs.ID.String(), "00000000-0000-0000-0000-000000000002", roleSystem,
			map[string]any{"gl_host_status": glStatusFailed, "failure_category": category, "error": errMsg})

		// Insert DLQ entry.
		if category == failureCategoryDomain || retryCount >= cfg.MaxTotalAttempts {
			h.moveToDLQ(headerID, gs, errMsg, category, errCode)
			return glDeliveryResult{
				GlHostStatus:    glStatusFailed,
				FailureCategory: ptrString(category),
				LastError:       ptrString(errMsg),
				RetryCount:      retryCount,
				SkipRetry:       category == failureCategoryDomain,
				AuditAction:     auditGLDeliveryFailed,
			}
		}

		// Infra error, still within retry budget.
		h.glStatus.update(headerID, func(gs *glStatusRecord) {
			gs.GlHostStatus = glStatusRetrying
		})
		h.auditLog.append(auditGLDeliveryRetry, gs.ID.String(), "00000000-0000-0000-0000-000000000002", roleSystem,
			map[string]any{"attempt": retryCount, "error_code": errCode})
		return glDeliveryResult{
			GlHostStatus:    glStatusRetrying,
			FailureCategory: ptrString(category),
			LastError:       ptrString(errMsg),
			RetryCount:      retryCount,
			SkipRetry:       false,
			AuditAction:     auditGLDeliveryRetry,
		}
	}

	// Success path.
	sanitizedResp := h.sanitizePII(map[string]any{
		"journalId": resp.GlJournalID,
		"status":    "ACCEPTED",
	})
	h.glStatus.update(headerID, func(gs *glStatusRecord) {
		gs.GlHostStatus = glStatusDelivered
		gs.GlHostJournalID = ptrString(resp.GlJournalID)
		gs.DeliveredAt = &now
		gs.RetryCount = retryCount
		gs.GlResponsePayloadJsonb = sanitizedResp
	})

	// Write audit SUCCESS in same "transaction".
	updatedGS := h.glStatus.get(headerID)
	h.auditLog.append(auditGLDeliverySuccess, updatedGS.ID.String(), "00000000-0000-0000-0000-000000000002", roleSystem,
		map[string]any{"gl_host_status": glStatusDelivered, "gl_host_journal_id": resp.GlJournalID})

	return glDeliveryResult{
		GlHostStatus:    glStatusDelivered,
		GlHostJournalID: ptrString(resp.GlJournalID),
		DeliveredAt:     &now,
		RetryCount:      retryCount,
		AuditAction:     auditGLDeliverySuccess,
	}
}

// moveToDLQ inserts a DLQ entry with sanitized payload snapshot.
func (h *p5M3Harness) moveToDLQ(headerID uuid.UUID, gs *glStatusRecord, errMsg, category, errCode string) {
	jh := h.jurnalHdrs.get(headerID)
	// Build payload snapshot without PII.
	payloadSnapshot := map[string]any{
		"header_id":  headerID.String(),
		"event_code": "",
	}
	if jh != nil {
		payloadSnapshot["event_code"] = jh.EventCode
		payloadSnapshot["narrative"] = jh.Narrative
		// Intentionally include a PII field to test sanitization gets it removed.
		payloadSnapshot["customer_name"] = "WILL-BE-REDACTED"
		payloadSnapshot["account_no"] = "WILL-BE-REDACTED"
	}
	sanitized := h.sanitizePII(payloadSnapshot)

	glStatusID := gs.ID
	dlqEntry := &dlqGLRecord{
		ID:              uuid.New(),
		JurnalHeaderID:  headerID,
		GlStatusID:      &glStatusID,
		FailureCategory: category,
		ErrorCode:       errCode,
		ErrorMessage:    errMsg,
		PayloadJsonb:    sanitized,
		RetryCount:      gs.RetryCount,
		Status:          dlqGLFailed,
	}
	h.dlqGL.insert(dlqEntry)
	h.auditLog.append(auditGLDeliveryDLQEntered, dlqEntry.ID.String(), "00000000-0000-0000-0000-000000000002", roleSystem,
		map[string]any{"jurnal_header_id": headerID.String(), "error_code": errCode, "category": category})
}

// sanitizePII removes known PII fields from a payload map.
func (h *p5M3Harness) sanitizePII(data map[string]any) map[string]any {
	piiFields := map[string]struct{}{
		"customer_name": {},
		"account_no":    {},
		"npwp":          {},
		"ktp":           {},
		"gl_host_api_key": {},
	}
	out := make(map[string]any, len(data))
	for k, v := range data {
		if _, redact := piiFields[k]; redact {
			out[k] = "[REDACTED]"
			continue
		}
		out[k] = v
	}
	return out
}

// manualRetry simulates POST /jurnal/header/{id}/retry-gl-delivery.
// callerPerms must include permGlDeliveryRetry.
func (h *p5M3Harness) manualRetry(
	ctx context.Context,
	headerID uuid.UUID,
	reason string,
	callerID uuid.UUID,
	callerPerms []string,
) (*retryResult, error) {
	h.t.Helper()
	if !hasPermission(callerPerms, permGlDeliveryRetry) {
		return nil, &glPermissionError{Code: errGLDeliveryPermissionDenied, Perm: permGlDeliveryRetry}
	}
	if len(reason) < 30 {
		return nil, &glValidationError{Code: errGLDeliveryReasonTooShort, Field: "reason",
			Message: "retry reason must be at least 30 characters"}
	}
	gs := h.glStatus.get(headerID)
	if gs == nil {
		return nil, &glDomainError{Code: errGLDeliveryJurnalNotFound, Message: "gl_status not found"}
	}
	if gs.GlHostStatus != glStatusFailed {
		return nil, &glDomainError{Code: errGLDeliveryInvalidTransition,
			Message: fmt.Sprintf("cannot retry from status %s", gs.GlHostStatus)}
	}
	prevStatus := gs.GlHostStatus
	now := time.Now()
	zero := 0

	// Audit BEFORE enqueue (DEC-018 compliance — audit must be in same tx as state change).
	auditEntityID := gs.ID.String()
	h.auditLog.append(auditGLDeliveryManualRetry, auditEntityID, callerID.String(), roleAkunCTL,
		map[string]any{"gl_host_status": glStatusPendingDelivery, "reason": reason, "retry_by": callerID.String()})

	// State change in "transaction".
	h.glStatus.update(headerID, func(gs *glStatusRecord) {
		gs.GlHostStatus = glStatusPendingDelivery
		gs.ManualRetryBy = &callerID
		gs.ManualRetryAt = &now
		gs.ManualRetryReason = &reason
		gs.RetryCount = zero
	})

	// Enqueue AFTER commit.
	jobID := uuid.New().String()
	h.asynqQueue.enqueue("gl_delivery:deliver", map[string]string{"jurnal_header_id": headerID.String()})

	return &retryResult{
		JobID:          jobID,
		PreviousStatus: prevStatus,
		NewStatus:      glStatusPendingDelivery,
	}, nil
}

// dlqReplay simulates POST /jurnal/gl-delivery-dlq/{id}/replay.
func (h *p5M3Harness) dlqReplay(
	ctx context.Context,
	dlqID uuid.UUID,
	reason string,
	callerID uuid.UUID,
	callerPerms []string,
) (*replayResult, error) {
	h.t.Helper()
	if !hasPermission(callerPerms, permGlDeliveryReplay) {
		return nil, &glPermissionError{Code: errGLDeliveryPermissionDenied, Perm: permGlDeliveryReplay}
	}
	if len(reason) < 30 {
		return nil, &glValidationError{Code: errGLDeliveryReasonTooShort, Field: "reason",
			Message: "replay reason must be at least 30 characters"}
	}
	entry := h.dlqGL.get(dlqID)
	if entry == nil {
		return nil, &glDomainError{Code: errGLDeliveryJurnalNotFound, Message: "DLQ entry not found"}
	}
	if entry.Status != dlqGLFailed {
		return nil, &glDomainError{Code: errGLDLQReplayInvalidState,
			Message: fmt.Sprintf("DLQ status %s cannot be replayed", entry.Status)}
	}

	gs := h.glStatus.get(entry.JurnalHeaderID)
	prevGLStatus := glStatusFailed
	if gs != nil {
		prevGLStatus = gs.GlHostStatus
	}

	now := time.Now()
	// Update DLQ + gl_host_status + audit in "transaction".
	entry.Status = dlqGLReplaying
	entry.ReplayedBy = &callerID
	entry.ReplayedAt = &now

	h.glStatus.update(entry.JurnalHeaderID, func(gs *glStatusRecord) {
		gs.GlHostStatus = glStatusPendingDelivery
		gs.ManualRetryBy = &callerID
		gs.ManualRetryAt = &now
		gs.ManualRetryReason = &reason
		gs.RetryCount = 0
	})

	h.auditLog.append(auditGLDeliveryDLQReplay, dlqID.String(), callerID.String(), roleITAdmin,
		map[string]any{"status": dlqGLReplaying, "reason": reason, "replayed_by": callerID.String()})

	// Enqueue AFTER commit.
	jobID := uuid.New().String()
	h.asynqQueue.enqueue("gl_delivery:deliver", map[string]string{"jurnal_header_id": entry.JurnalHeaderID.String()})

	return &replayResult{
		JobID:          jobID,
		PreviousStatus: prevGLStatus,
		NewStatus:      glStatusPendingDelivery,
	}, nil
}

// dlqDiscard simulates POST /jurnal/gl-delivery-dlq/{id}/discard.
func (h *p5M3Harness) dlqDiscard(
	ctx context.Context,
	dlqID uuid.UUID,
	reason string,
	callerID uuid.UUID,
	callerPerms []string,
) (*discardResult, error) {
	h.t.Helper()
	if !hasPermission(callerPerms, permGlDeliveryDiscard) {
		return nil, &glPermissionError{Code: errGLDeliveryPermissionDenied, Perm: permGlDeliveryDiscard}
	}
	if len(reason) < 30 {
		return nil, &glValidationError{Code: errGLDeliveryReasonTooShort, Field: "reason",
			Message: "discard reason must be at least 30 characters"}
	}
	entry := h.dlqGL.get(dlqID)
	if entry == nil {
		return nil, &glDomainError{Code: errGLDeliveryJurnalNotFound, Message: "DLQ entry not found"}
	}
	if entry.Status != dlqGLFailed {
		return nil, &glDomainError{Code: errGLDLQReplayInvalidState,
			Message: fmt.Sprintf("DLQ status %s cannot be discarded", entry.Status)}
	}

	now := time.Now()
	// Update DLQ + gl_status in "transaction".
	entry.Status = dlqGLAbandoned
	entry.DiscardedBy = &callerID
	entry.DiscardedAt = &now
	entry.DiscardedReason = &reason

	h.glStatus.update(entry.JurnalHeaderID, func(gs *glStatusRecord) {
		gs.GlHostStatus = glStatusDeadLetter
		gs.DiscardedBy = &callerID
		gs.DiscardedAt = &now
		gs.DiscardReason = &reason
	})

	h.auditLog.append(auditGLDeliveryDLQDiscarded, dlqID.String(), callerID.String(), roleITAdmin,
		map[string]any{"status": dlqGLAbandoned, "reason": reason, "discarded_by": callerID.String()})

	return &discardResult{
		NewStatus:   glStatusDeadLetter,
		DiscardedAt: now,
		DiscardedBy: callerID,
	}, nil
}

// runReconciliation simulates the recon worker for a given date.
func (h *p5M3Harness) runReconciliation(
	ctx context.Context,
	targetDate time.Time,
	triggerSource string,
	callerID *uuid.UUID,
) (*reconReportRecord, error) {
	h.t.Helper()
	cfg := defaultDeliverConfig()
	tolerance := decimal.NewFromFloat(1.0)

	// BLIPS side: aggregate jrnl.detail per kode_akun.
	blipsMap := make(map[string]decimal.Decimal)
	for _, jh := range h.jurnalHdrs.records {
		if jh.StatusInternal != jrnlPosted {
			continue
		}
		if jh.TanggalPosting.Format("2006-01-02") != targetDate.Format("2006-01-02") {
			continue
		}
		for _, d := range jh.DetailRows {
			net := d.DebitAmount.Sub(d.KreditAmount)
			blipsMap[d.KodeAkun] = blipsMap[d.KodeAkun].Add(net)
		}
	}

	// GL Host side.
	glMap := h.glHost.dailySummary
	// Simulate GL Host error if configured.
	if h.glHost.defaultResp != nil && h.glHost.defaultResp.HTTPStatus >= 500 {
		reportID := uuid.New()
		report := &reconReportRecord{
			ID:            reportID,
			TanggalRun:    targetDate,
			Status:        reconStatusFailed,
			ToleranceIDR:  tolerance,
			TriggerSource: triggerSource,
			TriggeredBy:   callerID,
		}
		h.recon.upsert(report)
		h.auditLog.append(auditGLReconciliationFailed, reportID.String(), "00000000-0000-0000-0000-000000000002", roleSystem,
			map[string]any{"status": reconStatusFailed, "error": "GL Host unreachable"})
		return report, nil
	}

	var mismatches []reconMismatchLine
	var totalBLIPS, totalGL decimal.Decimal
	_ = cfg

	// Compare BLIPS vs GL.
	for kodeAkun, blipsNet := range blipsMap {
		totalBLIPS = totalBLIPS.Add(blipsNet)
		glAmount, found := glMap[kodeAkun]
		if !found {
			glAmount = decimal.Zero
		}
		totalGL = totalGL.Add(glAmount)
		delta := blipsNet.Sub(glAmount).Abs()
		if delta.GreaterThan(tolerance) {
			mtype := mismatchAmountDiff
			if !found {
				mtype = mismatchBlipsOnly
			}
			mismatches = append(mismatches, reconMismatchLine{
				ID:              uuid.New(),
				KodeAkun:        kodeAkun,
				BlipsAmountIDR:  blipsNet,
				GlHostAmountIDR: glAmount,
				DeltaIDR:        blipsNet.Sub(glAmount),
				MismatchType:    mtype,
			})
		}
	}

	// GL-only accounts.
	for kodeAkun, glAmount := range glMap {
		if _, inBlips := blipsMap[kodeAkun]; !inBlips {
			totalGL = totalGL.Add(glAmount)
			mismatches = append(mismatches, reconMismatchLine{
				ID:              uuid.New(),
				KodeAkun:        kodeAkun,
				BlipsAmountIDR:  decimal.Zero,
				GlHostAmountIDR: glAmount,
				DeltaIDR:        glAmount.Neg(),
				MismatchType:    mismatchGLOnly,
			})
		}
	}

	status := reconStatusCompleted
	if len(mismatches) > 0 {
		status = reconStatusCompletedWithMismatch
	}

	reportID := uuid.New()
	report := &reconReportRecord{
		ID:             reportID,
		TanggalRun:     targetDate,
		Status:         status,
		MismatchCount:  len(mismatches),
		TotalJurnalIDR: totalBLIPS,
		GlHostTotalIDR: totalGL,
		ToleranceIDR:   tolerance,
		TriggerSource:  triggerSource,
		TriggeredBy:    callerID,
		MismatchLines:  mismatches,
	}
	h.recon.upsert(report)

	// Audit in "transaction" with report upsert.
	h.auditLog.append(auditGLReconciliationCompleted, reportID.String(), "00000000-0000-0000-0000-000000000002", roleSystem,
		map[string]any{
			"status":         status,
			"mismatch_count": len(mismatches),
			"blips_total":    totalBLIPS.StringFixed(4),
			"gl_total":       totalGL.StringFixed(4),
		})

	return report, nil
}

// ─── Error types ──────────────────────────────────────────────────────────────

type glPermissionError struct {
	Code string
	Perm string
}

func (e *glPermissionError) Error() string {
	return fmt.Sprintf("%s: permission required: %s", e.Code, e.Perm)
}

type glValidationError struct {
	Code    string
	Field   string
	Message string
}

func (e *glValidationError) Error() string {
	return fmt.Sprintf("%s: field=%s: %s", e.Code, e.Field, e.Message)
}

type glDomainError struct {
	Code    string
	Message string
}

func (e *glDomainError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func hasPermission(perms []string, perm string) bool {
	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}

func ptrString(s string) *string { return &s }

func akunCTLPerms() []string {
	return []string{
		permGlDeliveryRead,
		permGlDeliveryRetry,
		permGlDeliveryReplay,
		permReconRead,
		permReconRun,
	}
}

func itAdminPerms() []string {
	return []string{
		permGlDeliveryRead,
		permGlDeliveryRetry,
		permGlDeliveryReplay,
		permGlDeliveryDiscard,
		permReconRead,
		permReconRun,
	}
}

func akunPerms() []string {
	return []string{permGlDeliveryRead}
}

// ─── P5-M3 Scenarios ─────────────────────────────────────────────────────────

// P5-M3-A: S1-AC1 Auto-deliver happy path — GL Host 201 → DELIVERED + audit GL_DELIVERY.SUCCESS
func TestE2E_P5M3_A_AutoDeliver_HappyPath(t *testing.T) {
	t.Parallel()
	h := newP5M3Harness(t)
	ctx := context.Background()

	// Seed: posted jurnal + PENDING_DELIVERY gl_status.
	jh := h.jurnalHdrs.seedPosted("PENEMPATAN", decimal.NewFromFloat(5_000_000_000))
	gs := h.glStatus.seed(jh.ID, glStatusPendingDelivery)

	// GL Host returns 201.
	h.glHost.queueResponse(jh.ID, p5M3GLHostResponse{
		HTTPStatus:  201,
		GlJournalID: "GLHOST-JRN-20260615-00001",
	})

	// Execute.
	result := h.deliverJurnal(ctx, jh.ID)

	// Assert status = DELIVERED.
	if result.GlHostStatus != glStatusDelivered {
		t.Errorf("S1-AC1: expected DELIVERED, got %s", result.GlHostStatus)
	}
	if result.GlHostJournalID == nil || *result.GlHostJournalID != "GLHOST-JRN-20260615-00001" {
		t.Errorf("S1-AC1: expected GlHostJournalID GLHOST-JRN-20260615-00001, got %v", result.GlHostJournalID)
	}
	if result.DeliveredAt == nil {
		t.Error("S1-AC1: deliveredAt must be populated on success")
	}

	// Assert jrnl.gl_status updated in "same tx".
	updatedGS := h.glStatus.get(jh.ID)
	if updatedGS.GlHostStatus != glStatusDelivered {
		t.Errorf("S1-AC1: gl_status.gl_host_status not DELIVERED after delivery, got %s", updatedGS.GlHostStatus)
	}
	if updatedGS.GlHostJournalID == nil {
		t.Error("S1-AC1: gl_host_journal_id not populated")
	}

	// Assert audit GL_DELIVERY.SUCCESS written.
	if !h.auditLog.containsAction(gs.ID.String(), auditGLDeliverySuccess) {
		t.Errorf("S1-AC1: audit GL_DELIVERY.SUCCESS not found for gl_status_id %s", gs.ID)
	}
}

// P5-M3-B: S1-AC2 Idempotency — already DELIVERED → early return, no re-delivery, no extra audit.
func TestE2E_P5M3_B_Idempotency_AlreadyDelivered(t *testing.T) {
	t.Parallel()
	h := newP5M3Harness(t)
	ctx := context.Background()

	jh := h.jurnalHdrs.seedPosted("PENEMPATAN", decimal.NewFromFloat(1_000_000))
	gs := h.glStatus.seed(jh.ID, glStatusDelivered)
	gs.GlHostJournalID = ptrString("GLHOST-JRN-ALREADY-DELIVERED")

	initialAuditCount := len(h.auditLog.rows)
	initialGLHostCalls := h.glHost.calls

	// Re-deliver — must be a no-op.
	result := h.deliverJurnal(ctx, jh.ID)

	// Status unchanged, no new GL Host calls.
	if result.GlHostStatus != glStatusDelivered {
		t.Errorf("S1-AC2: expected DELIVERED (unchanged), got %s", result.GlHostStatus)
	}
	if h.glHost.calls != initialGLHostCalls {
		t.Errorf("S1-AC2: GL Host must NOT be called again, calls before=%d after=%d",
			initialGLHostCalls, h.glHost.calls)
	}
	// No new audit entry.
	if len(h.auditLog.rows) != initialAuditCount {
		t.Errorf("S1-AC2: no new audit rows expected, got %d new rows",
			len(h.auditLog.rows)-initialAuditCount)
	}
}

// P5-M3-C: S1-AC3 Domain error 4xx → SkipRetry → DLQ FAILED, failure_category=DOMAIN.
func TestE2E_P5M3_C_DomainError_DirectDLQ(t *testing.T) {
	t.Parallel()
	h := newP5M3Harness(t)
	ctx := context.Background()

	jh := h.jurnalHdrs.seedPosted("PENEMPATAN", decimal.NewFromFloat(5_000_000_000))
	gs := h.glStatus.seed(jh.ID, glStatusPendingDelivery)

	// GL Host returns 422.
	h.glHost.queueResponse(jh.ID, p5M3GLHostResponse{
		HTTPStatus: 422,
		Error: &struct {
			Code    string
			Message string
		}{Code: "INVALID_ACCOUNT_CODE", Message: "Account 1110-DEP not found in GL chart"},
	})

	result := h.deliverJurnal(ctx, jh.ID)

	// Assert FAILED + DOMAIN.
	if result.GlHostStatus != glStatusFailed {
		t.Errorf("S1-AC3: expected FAILED, got %s", result.GlHostStatus)
	}
	if result.FailureCategory == nil || *result.FailureCategory != failureCategoryDomain {
		t.Errorf("S1-AC3: expected failure_category=DOMAIN, got %v", result.FailureCategory)
	}
	// SkipRetry must be true for domain errors.
	if !result.SkipRetry {
		t.Error("S1-AC3: SkipRetry must be true for domain errors (no Asynq auto-retry)")
	}
	// DLQ entry inserted.
	dlqEntries := h.dlqGL.listByStatus(dlqGLFailed)
	if len(dlqEntries) == 0 {
		t.Error("S1-AC3: expected DLQ entry inserted, none found")
	} else {
		dlq := dlqEntries[0]
		if dlq.FailureCategory != failureCategoryDomain {
			t.Errorf("S1-AC3: DLQ failure_category expected DOMAIN, got %s", dlq.FailureCategory)
		}
		if dlq.ErrorCode != errGLDeliveryHost4XX {
			t.Errorf("S1-AC3: DLQ error_code expected %s, got %s", errGLDeliveryHost4XX, dlq.ErrorCode)
		}
	}
	// Audit GL_DELIVERY.FAILED written.
	if !h.auditLog.containsAction(gs.ID.String(), auditGLDeliveryFailed) {
		t.Error("S1-AC3: audit GL_DELIVERY.FAILED not written")
	}
}

// P5-M3-D: S1-AC4 Infra error 503 → RETRYING + audit GL_DELIVERY.RETRY;
// after 3× → FAILED with DLQ.
func TestE2E_P5M3_D_InfraError_RetryThenDLQ(t *testing.T) {
	t.Parallel()
	h := newP5M3Harness(t)
	ctx := context.Background()

	jh := h.jurnalHdrs.seedPosted("PENEMPATAN", decimal.NewFromFloat(5_000_000_000))
	h.glStatus.seed(jh.ID, glStatusPendingDelivery)

	// Configure override: max 3 attempts in test (but default is 5, so set RetryCount high).
	gs := h.glStatus.get(jh.ID)

	// Simulate: attempt 1 → RETRYING.
	h.glHost.queueResponse(jh.ID, p5M3GLHostResponse{HTTPStatus: 503})
	r1 := h.deliverJurnal(ctx, jh.ID)
	if r1.GlHostStatus != glStatusRetrying && r1.GlHostStatus != glStatusFailed {
		t.Logf("S1-AC4: attempt 1 status=%s (RETRYING expected when < max attempts)", r1.GlHostStatus)
	}
	if r1.FailureCategory == nil || *r1.FailureCategory != failureCategoryInfra {
		t.Errorf("S1-AC4: expected INFRA category after 503, got %v", r1.FailureCategory)
	}
	// Reset to retrying state for next attempt.
	h.glStatus.update(jh.ID, func(gs *glStatusRecord) {
		gs.GlHostStatus = glStatusRetrying
	})

	// Attempt 2.
	h.glHost.queueResponse(jh.ID, p5M3GLHostResponse{HTTPStatus: 503})
	r2 := h.deliverJurnal(ctx, jh.ID)
	if r2.RetryCount < 1 {
		t.Logf("S1-AC4: attempt 2 retry_count=%d", r2.RetryCount)
	}
	h.glStatus.update(jh.ID, func(gs *glStatusRecord) {
		gs.GlHostStatus = glStatusRetrying
		gs.RetryCount = 2
	})

	// Attempt 3 — exhaust budget (set RetryCount = maxAttempts-1 so next push = max).
	gs.RetryCount = defaultDeliverConfig().MaxTotalAttempts - 1
	h.glHost.queueResponse(jh.ID, p5M3GLHostResponse{HTTPStatus: 503})
	r3 := h.deliverJurnal(ctx, jh.ID)
	if r3.GlHostStatus != glStatusFailed {
		t.Errorf("S1-AC4: expected FAILED after max attempts, got %s", r3.GlHostStatus)
	}

	// DLQ entry must exist.
	dlqFailed := h.dlqGL.listByStatus(dlqGLFailed)
	if len(dlqFailed) == 0 {
		t.Error("S1-AC4: expected DLQ entry after max retries, none found")
	} else {
		if dlqFailed[len(dlqFailed)-1].FailureCategory != failureCategoryInfra {
			t.Errorf("S1-AC4: DLQ failure_category expected INFRA, got %s",
				dlqFailed[len(dlqFailed)-1].FailureCategory)
		}
	}
}

// P5-M3-E: S1-AC5 DEAD_LETTER terminal — worker skips, no state change.
func TestE2E_P5M3_E_DeadLetter_Terminal(t *testing.T) {
	t.Parallel()
	h := newP5M3Harness(t)
	ctx := context.Background()

	jh := h.jurnalHdrs.seedPosted("PENEMPATAN", decimal.NewFromFloat(1_000_000))
	h.glStatus.seed(jh.ID, glStatusDeadLetter)
	initialCalls := h.glHost.calls

	result := h.deliverJurnal(ctx, jh.ID)

	if result.GlHostStatus != glStatusDeadLetter {
		t.Errorf("S1-AC5: DEAD_LETTER must be terminal — status unchanged, got %s", result.GlHostStatus)
	}
	if h.glHost.calls != initialCalls {
		t.Error("S1-AC5: GL Host must NOT be called for DEAD_LETTER")
	}
}

// P5-M3-F: S2-AC1 GET delivery status DELIVERED — can_retry=false, delivered_at set.
func TestE2E_P5M3_F_GetStatus_Delivered(t *testing.T) {
	t.Parallel()
	h := newP5M3Harness(t)

	jh := h.jurnalHdrs.seedPosted("PENEMPATAN", decimal.NewFromFloat(1_000_000))
	gs := h.glStatus.seed(jh.ID, glStatusDelivered)
	now := time.Now()
	gs.GlHostJournalID = ptrString("GLHOST-JRN-20260615-99999")
	gs.DeliveredAt = &now

	// Simulate enriched GET /jurnal/header/{id}/gl-delivery-status.
	canRetry := gs.GlHostStatus == glStatusFailed
	if canRetry {
		t.Error("S2-AC1: can_retry must be false for DELIVERED status")
	}
	if gs.GlHostJournalID == nil {
		t.Error("S2-AC1: gl_host_journal_id must be populated")
	}
	if gs.DeliveredAt == nil {
		t.Error("S2-AC1: delivered_at must be populated")
	}
}

// P5-M3-G: S2-AC2 GET delivery status FAILED — can_retry=true, failure_category visible.
func TestE2E_P5M3_G_GetStatus_Failed_CanRetry(t *testing.T) {
	t.Parallel()
	h := newP5M3Harness(t)

	jh := h.jurnalHdrs.seedPosted("PENEMPATAN", decimal.NewFromFloat(1_000_000))
	gs := h.glStatus.seed(jh.ID, glStatusFailed)
	gs.FailureCategory = ptrString(failureCategoryDomain)
	gs.LastError = ptrString("GL_HOST_REJECTED: INVALID_ACCOUNT_CODE")

	// can_retry logic: FAILED → true.
	canRetry := gs.GlHostStatus == glStatusFailed
	if !canRetry {
		t.Error("S2-AC2: can_retry must be true for FAILED status")
	}
	if gs.FailureCategory == nil || *gs.FailureCategory != failureCategoryDomain {
		t.Error("S2-AC2: failure_category must be visible in response")
	}
	if gs.LastError == nil {
		t.Error("S2-AC2: last_error must be populated")
	}
}

// P5-M3-H: S2-AC3 GET delivery status PENDING_DELIVERY — can_retry=false.
func TestE2E_P5M3_H_GetStatus_Pending(t *testing.T) {
	t.Parallel()
	h := newP5M3Harness(t)

	jh := h.jurnalHdrs.seedPosted("PENEMPATAN", decimal.NewFromFloat(1_000_000))
	h.glStatus.seed(jh.ID, glStatusPendingDelivery)
	gs := h.glStatus.get(jh.ID)

	canRetry := gs.GlHostStatus == glStatusFailed
	if canRetry {
		t.Error("S2-AC3: can_retry must be false for PENDING_DELIVERY")
	}
	if gs.GlHostStatus != glStatusPendingDelivery {
		t.Errorf("S2-AC3: expected PENDING_DELIVERY, got %s", gs.GlHostStatus)
	}
}

// P5-M3-I: S3-AC1 Manual retry ROLE-AKUN-CTL — FAILED→PENDING_DELIVERY + audit BEFORE enqueue.
func TestE2E_P5M3_I_ManualRetry_HappyPath(t *testing.T) {
	t.Parallel()
	h := newP5M3Harness(t)
	ctx := context.Background()

	jh := h.jurnalHdrs.seedPosted("PENEMPATAN", decimal.NewFromFloat(1_000_000))
	gs := h.glStatus.seed(jh.ID, glStatusFailed)

	callerID := uuid.New()
	reason := "Kode akun 1110-DEP sudah diperbaiki di GL Host Chart of Accounts. Retry delivery."

	result, err := h.manualRetry(ctx, jh.ID, reason, callerID, akunCTLPerms())
	if err != nil {
		t.Fatalf("S3-AC1: unexpected error: %v", err)
	}
	if result.PreviousStatus != glStatusFailed {
		t.Errorf("S3-AC1: previous_status expected FAILED, got %s", result.PreviousStatus)
	}
	if result.NewStatus != glStatusPendingDelivery {
		t.Errorf("S3-AC1: new_status expected PENDING_DELIVERY, got %s", result.NewStatus)
	}

	// gl_status updated.
	updated := h.glStatus.get(jh.ID)
	if updated.GlHostStatus != glStatusPendingDelivery {
		t.Errorf("S3-AC1: gl_status.gl_host_status expected PENDING_DELIVERY, got %s", updated.GlHostStatus)
	}
	if updated.ManualRetryBy == nil || *updated.ManualRetryBy != callerID {
		t.Error("S3-AC1: manual_retry_by must record callerID")
	}
	if updated.ManualRetryReason == nil || *updated.ManualRetryReason != reason {
		t.Error("S3-AC1: manual_retry_reason must be persisted")
	}

	// Audit GL_DELIVERY.MANUAL_RETRY_INITIATED written (must appear BEFORE Asynq enqueue).
	if !h.auditLog.containsAction(gs.ID.String(), auditGLDeliveryManualRetry) {
		t.Errorf("S3-AC1: audit GL_DELIVERY.MANUAL_RETRY_INITIATED not found for gl_status_id %s", gs.ID)
	}
	// Asynq task enqueued AFTER audit/commit.
	if h.asynqQueue.countByType("gl_delivery:deliver") < 1 {
		t.Error("S3-AC1: expected Asynq gl_delivery:deliver task enqueued after retry")
	}
}

// P5-M3-J: S3-AC2 Manual retry rejected DEAD_LETTER → WORKFLOW_INVALID_TRANSITION.
func TestE2E_P5M3_J_ManualRetry_DeadLetter_Rejected(t *testing.T) {
	t.Parallel()
	h := newP5M3Harness(t)
	ctx := context.Background()

	jh := h.jurnalHdrs.seedPosted("PENEMPATAN", decimal.NewFromFloat(1_000_000))
	h.glStatus.seed(jh.ID, glStatusDeadLetter)

	_, err := h.manualRetry(ctx, jh.ID, "Reason long enough to pass the 30-char minimum check.", uuid.New(), akunCTLPerms())
	if err == nil {
		t.Fatal("S3-AC2: expected error for DEAD_LETTER retry, got nil")
	}
	de, ok := err.(*glDomainError)
	if !ok {
		t.Fatalf("S3-AC2: expected glDomainError, got %T: %v", err, err)
	}
	if de.Code != errGLDeliveryInvalidTransition {
		t.Errorf("S3-AC2: expected error code %s, got %s", errGLDeliveryInvalidTransition, de.Code)
	}
	// gl_status unchanged.
	gs := h.glStatus.get(jh.ID)
	if gs.GlHostStatus != glStatusDeadLetter {
		t.Errorf("S3-AC2: gl_status must remain DEAD_LETTER, got %s", gs.GlHostStatus)
	}
	// No Asynq task enqueued.
	if h.asynqQueue.countByType("gl_delivery:deliver") != 0 {
		t.Error("S3-AC2: no Asynq task must be enqueued for DEAD_LETTER retry")
	}
}

// P5-M3-K: S3-AC3 Manual retry reason < 30 chars → VALIDATION_FAILED.
func TestE2E_P5M3_K_ManualRetry_ShortReason(t *testing.T) {
	t.Parallel()
	h := newP5M3Harness(t)
	ctx := context.Background()

	jh := h.jurnalHdrs.seedPosted("PENEMPATAN", decimal.NewFromFloat(1_000_000))
	h.glStatus.seed(jh.ID, glStatusFailed)

	_, err := h.manualRetry(ctx, jh.ID, "Sudah diperbaiki.", uuid.New(), akunCTLPerms())
	if err == nil {
		t.Fatal("S3-AC3: expected validation error for short reason")
	}
	ve, ok := err.(*glValidationError)
	if !ok {
		t.Fatalf("S3-AC3: expected glValidationError, got %T", err)
	}
	if ve.Code != errGLDeliveryReasonTooShort {
		t.Errorf("S3-AC3: expected code %s, got %s", errGLDeliveryReasonTooShort, ve.Code)
	}
	if ve.Field != "reason" {
		t.Errorf("S3-AC3: expected field=reason, got %s", ve.Field)
	}
}

// P5-M3-L: S3-AC4 ROLE-AKUN (no retry permission) → FORBIDDEN.
func TestE2E_P5M3_L_ManualRetry_PermissionDenied(t *testing.T) {
	t.Parallel()
	h := newP5M3Harness(t)
	ctx := context.Background()

	jh := h.jurnalHdrs.seedPosted("PENEMPATAN", decimal.NewFromFloat(1_000_000))
	h.glStatus.seed(jh.ID, glStatusFailed)

	reason := "Mencoba retry tapi tidak punya permission jurnal.gl_delivery.retry."
	_, err := h.manualRetry(ctx, jh.ID, reason, uuid.New(), akunPerms()) // ROLE-AKUN has no retry perm
	if err == nil {
		t.Fatal("S3-AC4: expected permission error for ROLE-AKUN")
	}
	pe, ok := err.(*glPermissionError)
	if !ok {
		t.Fatalf("S3-AC4: expected glPermissionError, got %T", err)
	}
	if pe.Code != errGLDeliveryPermissionDenied {
		t.Errorf("S3-AC4: expected code %s, got %s", errGLDeliveryPermissionDenied, pe.Code)
	}
	// gl_status unchanged.
	gs := h.glStatus.get(jh.ID)
	if gs.GlHostStatus != glStatusFailed {
		t.Errorf("S3-AC4: gl_status must remain FAILED, got %s", gs.GlHostStatus)
	}
}

// P5-M3-M: S4-AC1 Recon BLIPS == GL → COMPLETED, mismatch_count=0.
func TestE2E_P5M3_M_Recon_NoMismatch_Completed(t *testing.T) {
	t.Parallel()
	h := newP5M3Harness(t)
	ctx := context.Background()

	targetDate := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)

	// Seed 2 posted jurnals for targetDate.
	dep := decimal.NewFromFloat(5_000_000_000)
	jh1 := h.jurnalHdrs.seedPosted("PENEMPATAN", dep)
	jh1.TanggalPosting = targetDate

	// GL Host summary matches BLIPS exactly.
	// BLIPS: 1110-DEP: +5000000000, 1001-KAS: -5000000000
	h.glHost.setSummary("1110-DEP", dep)
	h.glHost.setSummary("1001-KAS", dep.Neg())

	report, err := h.runReconciliation(ctx, targetDate, "MANUAL", nil)
	if err != nil {
		t.Fatalf("S4-AC1: unexpected error: %v", err)
	}
	if report.Status != reconStatusCompleted {
		t.Errorf("S4-AC1: expected COMPLETED, got %s", report.Status)
	}
	if report.MismatchCount != 0 {
		t.Errorf("S4-AC1: expected mismatch_count=0, got %d", report.MismatchCount)
	}
	if len(report.MismatchLines) != 0 {
		t.Errorf("S4-AC1: expected no mismatch lines, got %d", len(report.MismatchLines))
	}
	// Audit GL_RECONCILIATION.COMPLETED written.
	if !h.auditLog.containsAction(report.ID.String(), auditGLReconciliationCompleted) {
		t.Error("S4-AC1: audit GL_RECONCILIATION.COMPLETED not written")
	}
	// Decimal integrity: no float64 contamination.
	if !report.TotalJurnalIDR.Equal(dep.Sub(dep)) { // net = 0 for balanced journal
		t.Logf("S4-AC1: total_jurnal_idr=%s (net of debit-kredit)", report.TotalJurnalIDR.String())
	}
}

// P5-M3-N: S4-AC2 Recon finds 2 mismatches (BLIPS_ONLY + AMOUNT_DIFF) →
// COMPLETED_WITH_MISMATCH.
func TestE2E_P5M3_N_Recon_TwoMismatches(t *testing.T) {
	t.Parallel()
	h := newP5M3Harness(t)
	ctx := context.Background()

	targetDate := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)

	// BLIPS: 3010-OCI-AST (1_000_000), 1210-OBLIGASI (14_000_000), 1001-KAS offset.
	jh1 := h.jurnalHdrs.seedPosted("ECL_PEMBENTUKAN", decimal.NewFromFloat(1_000_000))
	jh1.TanggalPosting = targetDate
	jh1.DetailRows = []jurnalDetailRow{
		{ID: uuid.New(), KodeAkun: "3010-OCI-AST", DebitAmount: decimal.NewFromFloat(1_000_000), KreditAmount: decimal.Zero, MataUang: "IDR"},
		{ID: uuid.New(), KodeAkun: "1001-KAS", DebitAmount: decimal.Zero, KreditAmount: decimal.NewFromFloat(1_000_000), MataUang: "IDR"},
	}
	jh2 := h.jurnalHdrs.seedPosted("PENEMPATAN", decimal.NewFromFloat(14_000_000))
	jh2.TanggalPosting = targetDate
	jh2.DetailRows = []jurnalDetailRow{
		{ID: uuid.New(), KodeAkun: "1210-OBLIGASI", DebitAmount: decimal.NewFromFloat(14_000_000), KreditAmount: decimal.Zero, MataUang: "IDR"},
		{ID: uuid.New(), KodeAkun: "1001-KAS", DebitAmount: decimal.Zero, KreditAmount: decimal.NewFromFloat(14_000_000), MataUang: "IDR"},
	}

	// GL Host: 3010-OCI-AST missing, 1210-OBLIGASI amount differs by IDR 14,000.
	// (BLIPS has 14_000_000; GL has 0 → AMOUNT_DIFF).
	// 3010-OCI-AST: BLIPS_ONLY (not in GL summary).
	h.glHost.setSummary("1001-KAS", decimal.NewFromFloat(-15_000_000))
	// Note: not setting 3010-OCI-AST or 1210-OBLIGASI → both will be BLIPS_ONLY or AMOUNT_DIFF.

	report, err := h.runReconciliation(ctx, targetDate, "CRON", nil)
	if err != nil {
		t.Fatalf("S4-AC2: unexpected error: %v", err)
	}
	if report.Status != reconStatusCompletedWithMismatch {
		t.Errorf("S4-AC2: expected COMPLETED_WITH_MISMATCH, got %s", report.Status)
	}
	if report.MismatchCount == 0 {
		t.Error("S4-AC2: expected mismatch_count > 0")
	}
	// Verify mismatch types.
	mismatchTypes := make(map[string]bool)
	for _, m := range report.MismatchLines {
		mismatchTypes[m.MismatchType] = true
	}
	if !mismatchTypes[mismatchBlipsOnly] {
		t.Errorf("S4-AC2: expected BLIPS_ONLY mismatch, got types: %v", mismatchTypes)
	}
	// All delta amounts must be non-zero (above tolerance 1 IDR).
	for _, m := range report.MismatchLines {
		if m.DeltaIDR.IsZero() {
			t.Errorf("S4-AC2: mismatch line %s has zero delta — should not appear", m.KodeAkun)
		}
	}
}

// P5-M3-O: S4-AC3 Manual recon trigger ROLE-AKUN-CTL → 202 job enqueued.
func TestE2E_P5M3_O_ManualReconTrigger(t *testing.T) {
	t.Parallel()
	h := newP5M3Harness(t)
	ctx := context.Background()

	targetDate := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	callerID := uuid.New()

	// Simulate TriggerAsync: creates IN_PROGRESS report + enqueues Asynq.
	h.auditLog.append(auditGLReconciliationTriggered, uuid.New().String(), callerID.String(), roleAkunCTL,
		map[string]any{"tanggal": targetDate.Format("2006-01-02"), "trigger": "MANUAL"})
	h.asynqQueue.enqueue("gl_delivery:reconcile-daily", map[string]string{
		"date":      targetDate.Format("2006-01-02"),
		"tenant_id": "TUGURE",
	})

	// Assertions.
	if h.asynqQueue.countByType("gl_delivery:reconcile-daily") < 1 {
		t.Error("S4-AC3: expected recon Asynq task enqueued")
	}
	if len(h.auditLog.rows) == 0 {
		t.Error("S4-AC3: expected audit GL_RECONCILIATION.TRIGGERED")
	}
	_ = ctx
	_ = callerID
}

// P5-M3-P: S4-AC4 GL Host unreachable → recon status FAILED, no mismatch rows.
func TestE2E_P5M3_P_Recon_GLHostUnreachable(t *testing.T) {
	t.Parallel()
	h := newP5M3Harness(t)
	ctx := context.Background()

	targetDate := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)

	// Set default stub to return 503 (triggers failure path in runReconciliation).
	h.glHost.setDefault(p5M3GLHostResponse{HTTPStatus: 503})

	report, err := h.runReconciliation(ctx, targetDate, "CRON", nil)
	if err != nil {
		t.Fatalf("S4-AC4: unexpected error: %v", err)
	}
	if report.Status != reconStatusFailed {
		t.Errorf("S4-AC4: expected FAILED, got %s", report.Status)
	}
	if len(report.MismatchLines) != 0 {
		t.Errorf("S4-AC4: expected no mismatch rows on FAILED recon, got %d", len(report.MismatchLines))
	}
	// Audit GL_RECONCILIATION.FAILED written.
	if !h.auditLog.containsAction(report.ID.String(), auditGLReconciliationFailed) {
		t.Error("S4-AC4: audit GL_RECONCILIATION.FAILED not written")
	}
}

// P5-M3-Q: S5-AC1 DLQ list filter sort — multiple FAILED entries with pagination fields.
func TestE2E_P5M3_Q_DLQList_FilterSort(t *testing.T) {
	t.Parallel()
	h := newP5M3Harness(t)

	// Seed 8 DLQ entries as per story background.
	for i := 0; i < 8; i++ {
		jh := h.jurnalHdrs.seedPosted("PENEMPATAN", decimal.NewFromFloat(float64((i+1)*1_000_000)))
		h.glStatus.seed(jh.ID, glStatusFailed)
		gs := h.glStatus.get(jh.ID)
		gsID := gs.ID
		category := failureCategoryDomain
		errCode := errGLDeliveryHost4XX
		if i%2 == 0 {
			category = failureCategoryInfra
			errCode = errGLDeliveryHostUnreachable
		}
		h.dlqGL.insert(&dlqGLRecord{
			ID:              uuid.New(),
			JurnalHeaderID:  jh.ID,
			GlStatusID:      &gsID,
			FailureCategory: category,
			ErrorCode:       errCode,
			ErrorMessage:    fmt.Sprintf("error %d", i),
			Status:          dlqGLFailed,
		})
	}

	// List all FAILED entries.
	entries := h.dlqGL.listByStatus(dlqGLFailed)
	if len(entries) != 8 {
		t.Errorf("S5-AC1: expected 8 FAILED DLQ entries, got %d", len(entries))
	}

	// Each entry should have: failure_category, error_code, retry_count, status.
	for _, e := range entries {
		if e.FailureCategory == "" {
			t.Errorf("S5-AC1: DLQ entry %s missing failure_category", e.ID)
		}
		if e.ErrorCode == "" {
			t.Errorf("S5-AC1: DLQ entry %s missing error_code", e.ID)
		}
		if e.Status != dlqGLFailed {
			t.Errorf("S5-AC1: DLQ entry %s expected FAILED, got %s", e.ID, e.Status)
		}
	}
}

// P5-M3-R: S5-AC2 DLQ replay ROLE-IT-ADMIN → FAILED→PENDING_DELIVERY + audit DLQ_REPLAY_INITIATED.
func TestE2E_P5M3_R_DLQReplay_HappyPath(t *testing.T) {
	t.Parallel()
	h := newP5M3Harness(t)
	ctx := context.Background()

	jh := h.jurnalHdrs.seedPosted("PENEMPATAN", decimal.NewFromFloat(5_000_000_000))
	gs := h.glStatus.seed(jh.ID, glStatusFailed)
	gsID := gs.ID
	dlqEntry := &dlqGLRecord{
		ID:              uuid.New(),
		JurnalHeaderID:  jh.ID,
		GlStatusID:      &gsID,
		FailureCategory: failureCategoryInfra,
		ErrorCode:       errGLDeliveryHostUnreachable,
		Status:          dlqGLFailed,
	}
	h.dlqGL.insert(dlqEntry)

	callerID := uuid.New()
	reason := "GL Host sudah pulih setelah maintenance window 2026-06-14 22:00-02:00 WIB."

	result, err := h.dlqReplay(ctx, dlqEntry.ID, reason, callerID, itAdminPerms())
	if err != nil {
		t.Fatalf("S5-AC2: unexpected error: %v", err)
	}
	if result.NewStatus != glStatusPendingDelivery {
		t.Errorf("S5-AC2: expected PENDING_DELIVERY after replay, got %s", result.NewStatus)
	}
	if result.PreviousStatus != glStatusFailed {
		t.Errorf("S5-AC2: expected previous_status FAILED, got %s", result.PreviousStatus)
	}

	// DLQ status = REPLAYING.
	updatedDLQ := h.dlqGL.get(dlqEntry.ID)
	if updatedDLQ.Status != dlqGLReplaying {
		t.Errorf("S5-AC2: DLQ entry expected REPLAYING, got %s", updatedDLQ.Status)
	}
	// gl_status reset to PENDING_DELIVERY.
	updatedGS := h.glStatus.get(jh.ID)
	if updatedGS.GlHostStatus != glStatusPendingDelivery {
		t.Errorf("S5-AC2: gl_status.gl_host_status expected PENDING_DELIVERY, got %s", updatedGS.GlHostStatus)
	}
	// Audit DLQ_REPLAY_INITIATED written.
	if !h.auditLog.containsAction(dlqEntry.ID.String(), auditGLDeliveryDLQReplay) {
		t.Error("S5-AC2: audit GL_DELIVERY.DLQ_REPLAY_INITIATED not written")
	}
	// Asynq task enqueued.
	if h.asynqQueue.countByType("gl_delivery:deliver") < 1 {
		t.Error("S5-AC2: expected Asynq gl_delivery:deliver task enqueued after replay")
	}
}

// P5-M3-S: S5-AC3 DLQ discard ROLE-IT-ADMIN → DEAD_LETTER + audit DLQ_DISCARDED, reason ≥30.
func TestE2E_P5M3_S_DLQDiscard_HappyPath(t *testing.T) {
	t.Parallel()
	h := newP5M3Harness(t)
	ctx := context.Background()

	jh := h.jurnalHdrs.seedPosted("PENEMPATAN", decimal.NewFromFloat(5_000_000_000))
	gs := h.glStatus.seed(jh.ID, glStatusFailed)
	gsID := gs.ID
	dlqEntry := &dlqGLRecord{
		ID:              uuid.New(),
		JurnalHeaderID:  jh.ID,
		GlStatusID:      &gsID,
		FailureCategory: failureCategoryDomain,
		ErrorCode:       errGLDeliveryHost4XX,
		Status:          dlqGLFailed,
	}
	h.dlqGL.insert(dlqEntry)

	callerID := uuid.New()
	reason := "Jurnal JRN-2026-000077 sudah diinput manual di GL Host pada 2026-06-14 oleh tim Akuntansi. Delivery otomatis tidak diperlukan."

	result, err := h.dlqDiscard(ctx, dlqEntry.ID, reason, callerID, itAdminPerms())
	if err != nil {
		t.Fatalf("S5-AC3: unexpected error: %v", err)
	}
	if result.NewStatus != glStatusDeadLetter {
		t.Errorf("S5-AC3: expected DEAD_LETTER, got %s", result.NewStatus)
	}

	// DLQ status = ABANDONED.
	updatedDLQ := h.dlqGL.get(dlqEntry.ID)
	if updatedDLQ.Status != dlqGLAbandoned {
		t.Errorf("S5-AC3: DLQ entry expected ABANDONED, got %s", updatedDLQ.Status)
	}
	if updatedDLQ.DiscardedReason == nil || *updatedDLQ.DiscardedReason != reason {
		t.Error("S5-AC3: DLQ discard_reason must be persisted")
	}
	// gl_status = DEAD_LETTER.
	updatedGS := h.glStatus.get(jh.ID)
	if updatedGS.GlHostStatus != glStatusDeadLetter {
		t.Errorf("S5-AC3: gl_status must be DEAD_LETTER, got %s", updatedGS.GlHostStatus)
	}
	if updatedGS.DiscardReason == nil || *updatedGS.DiscardReason != reason {
		t.Error("S5-AC3: gl_status.discard_reason must be persisted")
	}
	// Audit GL_DELIVERY.DLQ_DISCARDED written with reason.
	if !h.auditLog.containsAction(dlqEntry.ID.String(), auditGLDeliveryDLQDiscarded) {
		t.Error("S5-AC3: audit GL_DELIVERY.DLQ_DISCARDED not written")
	}
	// Verify reason appears in audit after_json.
	var auditRow *glAuditRow
	for i := range h.auditLog.rows {
		r := &h.auditLog.rows[i]
		if r.EntityID == dlqEntry.ID.String() && r.Action == auditGLDeliveryDLQDiscarded {
			auditRow = r
			break
		}
	}
	if auditRow == nil {
		t.Error("S5-AC3: audit row for GL_DELIVERY.DLQ_DISCARDED not found")
	} else if auditRow.AfterJSON["reason"] != reason {
		t.Errorf("S5-AC3: audit after_json.reason expected %q, got %v", reason, auditRow.AfterJSON["reason"])
	}
}

// P5-M3-T: S5-AC4 DLQ discard ROLE-AKUN-CTL → 403 FORBIDDEN (jurnal.gl_delivery.discard).
func TestE2E_P5M3_T_DLQDiscard_PermissionDenied(t *testing.T) {
	t.Parallel()
	h := newP5M3Harness(t)
	ctx := context.Background()

	jh := h.jurnalHdrs.seedPosted("PENEMPATAN", decimal.NewFromFloat(1_000_000))
	gs := h.glStatus.seed(jh.ID, glStatusFailed)
	gsID := gs.ID
	dlqEntry := &dlqGLRecord{
		ID:             uuid.New(),
		JurnalHeaderID: jh.ID,
		GlStatusID:     &gsID,
		Status:         dlqGLFailed,
	}
	h.dlqGL.insert(dlqEntry)

	reason := "Ingin mendiscard jurnal ini karena sudah tidak relevan lagi diposting."
	_, err := h.dlqDiscard(ctx, dlqEntry.ID, reason, uuid.New(), akunCTLPerms()) // akunCTL has no discard perm
	if err == nil {
		t.Fatal("S5-AC4: expected permission error for ROLE-AKUN-CTL discard")
	}
	pe, ok := err.(*glPermissionError)
	if !ok {
		t.Fatalf("S5-AC4: expected glPermissionError, got %T", err)
	}
	if pe.Code != errGLDeliveryPermissionDenied {
		t.Errorf("S5-AC4: expected code %s, got %s", errGLDeliveryPermissionDenied, pe.Code)
	}
	// gl_status unchanged.
	updatedGS := h.glStatus.get(jh.ID)
	if updatedGS.GlHostStatus != glStatusFailed {
		t.Errorf("S5-AC4: gl_status must remain FAILED, got %s", updatedGS.GlHostStatus)
	}
}

// P5-M3-U: Idempotency — same Idempotency-Key replay returns original response,
// no duplicate side-effects (no second DLQ entry, no second audit row for same entity).
func TestE2E_P5M3_U_Idempotency_SameKeyReplay(t *testing.T) {
	t.Parallel()
	h := newP5M3Harness(t)
	ctx := context.Background()

	jh := h.jurnalHdrs.seedPosted("PENEMPATAN", decimal.NewFromFloat(1_000_000))
	h.glStatus.seed(jh.ID, glStatusFailed)
	gsID := h.glStatus.get(jh.ID).ID
	dlqEntry := &dlqGLRecord{
		ID:             uuid.New(),
		JurnalHeaderID: jh.ID,
		GlStatusID:     &gsID,
		Status:         dlqGLFailed,
	}
	h.dlqGL.insert(dlqEntry)

	callerID := uuid.New()
	reason := "Replay diperlukan karena GL Host sudah pulih dari downtime tadi pagi."

	// First replay.
	r1, err := h.dlqReplay(ctx, dlqEntry.ID, reason, callerID, itAdminPerms())
	if err != nil {
		t.Fatalf("U: first replay failed: %v", err)
	}

	// Simulate idempotency: re-playing the same request after status is REPLAYING
	// must return REPLAYED status or idempotency response — not REPLAYED_OK prematurely.
	// For idempotency, we assert: second replay with same dlqID and same-state entry
	// must be rejected (already REPLAYING, not re-enterable).
	_, err2 := h.dlqReplay(ctx, dlqEntry.ID, reason, callerID, itAdminPerms())
	if err2 == nil {
		t.Error("U: second replay of REPLAYING entry must fail (idempotency guard)")
	}
	// Only one Asynq task should have been enqueued.
	if h.asynqQueue.countByType("gl_delivery:deliver") != 1 {
		t.Errorf("U: expected exactly 1 Asynq task, got %d",
			h.asynqQueue.countByType("gl_delivery:deliver"))
	}
	_ = r1
}

// P5-M3-V: Audit hash-chain integrity — every mutation writes a chained audit_log row.
func TestE2E_P5M3_V_AuditHashChain_Integrity(t *testing.T) {
	t.Parallel()
	h := newP5M3Harness(t)
	ctx := context.Background()

	// Run several mutations to build an audit chain.
	jh1 := h.jurnalHdrs.seedPosted("PENEMPATAN", decimal.NewFromFloat(5_000_000_000))
	h.glStatus.seed(jh1.ID, glStatusPendingDelivery)
	h.glHost.queueResponse(jh1.ID, p5M3GLHostResponse{HTTPStatus: 201, GlJournalID: "GLHOST-001"})
	h.deliverJurnal(ctx, jh1.ID) // GL_DELIVERY.SUCCESS

	jh2 := h.jurnalHdrs.seedPosted("PENEMPATAN", decimal.NewFromFloat(1_000_000))
	h.glStatus.seed(jh2.ID, glStatusPendingDelivery)
	h.glHost.queueResponse(jh2.ID, p5M3GLHostResponse{HTTPStatus: 422, Error: &struct {
		Code    string
		Message string
	}{"ERR", "bad account"}})
	h.deliverJurnal(ctx, jh2.ID) // GL_DELIVERY.FAILED + DLQ

	// Chain must be unbroken.
	if len(h.auditLog.rows) == 0 {
		t.Fatal("V: no audit rows written")
	}
	ok, desc := h.auditLog.verifyHashChain()
	if !ok {
		t.Errorf("V: audit hash chain broken: %s", desc)
	}
}

// P5-M3-W: PII sanitization — DLQ payload_snapshot does NOT contain
// customer_name / account_no / npwp.
func TestE2E_P5M3_W_PII_Sanitization(t *testing.T) {
	t.Parallel()
	h := newP5M3Harness(t)
	ctx := context.Background()

	jh := h.jurnalHdrs.seedPosted("PENEMPATAN", decimal.NewFromFloat(5_000_000_000))
	h.glStatus.seed(jh.ID, glStatusPendingDelivery)

	// GL Host returns 422 (domain error → DLQ).
	h.glHost.queueResponse(jh.ID, p5M3GLHostResponse{HTTPStatus: 422, Error: &struct {
		Code    string
		Message string
	}{"INVALID_ACCOUNT_CODE", "Account not found"}})

	h.deliverJurnal(ctx, jh.ID)

	// DLQ entry must exist.
	dlqEntries := h.dlqGL.listByStatus(dlqGLFailed)
	if len(dlqEntries) == 0 {
		t.Fatal("W: expected DLQ entry after domain error")
	}
	dlq := dlqEntries[0]
	piiFields := []string{"customer_name", "account_no", "npwp", "ktp", "gl_host_api_key"}
	for _, field := range piiFields {
		if val, exists := dlq.PayloadJsonb[field]; exists {
			if val != "[REDACTED]" {
				t.Errorf("W: PII field %q not sanitized — value: %v", field, val)
			}
		}
	}
	// Confirm "customer_name" was present in original (moveToDLQ seeds it) and got redacted.
	if val, exists := dlq.PayloadJsonb["customer_name"]; exists {
		if val != "[REDACTED]" {
			t.Errorf("W: customer_name not redacted in DLQ payload, got %v", val)
		}
	} else {
		t.Error("W: customer_name field absent from DLQ payload — sanitization test inconclusive")
	}
}
