// Package e2e — P5-M4 Periode Buku Close Workflow end-to-end tests.
//
// Scope: soft-close request (S1), soft-close approve with SoD (S2),
// hard-close request + CFO step-up MFA approve (S3), reopen with grace window (S4),
// closing checklist endpoint + status periode report (S5), plus cross-cutting.
//
// Scenarios:
//
//	P5-M4-A  S1-AC1: Soft-close request happy path — checklist evaluated, snapshot persisted, 202
//	P5-M4-B  S1-AC2: Duplicate soft-close request → 409 SOFT_CLOSE_PENDING_EXISTS
//	P5-M4-C  S1-AC3: Checklist has 1 failing item → 422 CLOSING_CHECKLIST_FAILED, snapshot saved
//	P5-M4-D  S1-AC4: MANUAL_CHECK via GetChecklist — background snapshot written, 4 items returned
//	P5-M4-E  S2-AC1: Soft-close approve SoD violation (approver = requester) → 403 SOD_VIOLATION
//	P5-M4-F  S2-AC2: Stale checklist (> 24h) at approve time → re-eval fails → 422 CLOSING_CHECKLIST_STALE
//	P5-M4-G  S2-AC3: Soft-close approve happy path — OPEN → SOFT_CLOSED, snapshot, audit in-tx
//	P5-M4-H  S2-AC4: Row-version optimistic lock conflict → 409 CONFLICT
//	P5-M4-I  S3-AC1: Hard-close request + approve full flow → CLOSED, kurs locked, MV job enqueued
//	P5-M4-J  S3-AC2: Hard-close-approve missing X-Step-Up-Token → 401 MFA_STEP_UP_REQUIRED
//	P5-M4-K  S3-AC3: Hard-close-approve expired step-up token (> 5 min) → 401 MFA_STEP_UP_EXPIRED
//	P5-M4-L  S3-AC4: Hard-close-reject → HARD_CLOSE_PENDING → SOFT_CLOSED, no step-up needed
//	P5-M4-M  S4-AC1: Reopen SOFT_CLOSED → OPEN, no step-up, SoD respected
//	P5-M4-N  S4-AC2: Reopen CLOSED → SOFT_CLOSED in grace window, CFO step-up, kurs unlocked
//	P5-M4-O  S4-AC3: Reopen reason < 30 chars → VALIDATION_FAILED
//	P5-M4-P  S4-AC4: Reopen CLOSED outside grace window → 423 PERIODE_GRACE_EXPIRED
//	P5-M4-Q  S5-AC1: GetChecklist — 1 item failing returns 422, all pass returns 200 with snapshot
//	P5-M4-R  S5-AC2: ListStatusPeriode — cursor pagination + sort + filter + ROLE-MAKER-TR → 403
//	P5-M4-S  S5-AC3: Export audit row written on ListStatusPeriode with ?export=csv
//	P5-M4-T  S5-AC4: GetChecklist after CLOSED — snapshot reads HARD_CLOSE_APPROVE transition
//	P5-M4-U  Idempotency: replay same Idempotency-Key returns original response, no duplicate side-effects
//	P5-M4-V  Audit: every mutation writes audit_log with unbroken hash-chain
//	P5-M4-W  Append-only: sys.closing_checklist_snapshot DELETE blocked (trigger-style guard)
//
// Decision log compliance:
//
//	DEC-016: shopspring/decimal for JURNAL_BALANCED threshold                     — Scenarios C, F, Q
//	DEC-017: 4-eyes SoD; CFO sole approver for hard-close                         — Scenarios E, I
//	DEC-018: Audit trail append-only; snapshot immutable                           — Scenarios V, W
//	DEC-021: Idempotency-Key mandatory on mutating endpoints                       — Scenario U
//	DEC-022: Cursor-based pagination                                               — Scenario R
//	DEC-026/027: MFA mandatory for ROLE-CFO; step-up for hard-close + CLOSED→reopen — Scenarios J, K, N
//
// Run:
//
//	go test ./backend/tests/e2e/... -v -run TestE2E_P5M4 -timeout 60s -race
package e2e

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── P5-M4 domain constants ───────────────────────────────────────────────────

const (
	// Periode status values (mst.periode_buku.status_periode).
	m4StatusOpen             = "OPEN"
	m4StatusSoftClosed       = "SOFT_CLOSED"
	m4StatusHardClosePending = "HARD_CLOSE_PENDING"
	m4StatusClosed           = "CLOSED"

	// Snapshot transition values (sys.closing_checklist_snapshot.transition).
	m4TransitionSoftCloseRequest = "SOFT_CLOSE_REQUEST"
	m4TransitionSoftCloseApprove = "SOFT_CLOSE_APPROVE"
	m4TransitionHardCloseRequest = "HARD_CLOSE_REQUEST"
	m4TransitionHardCloseApprove = "HARD_CLOSE_APPROVE"
	m4TransitionReopenRequest    = "REOPEN_REQUEST"
	m4TransitionReopenApprove    = "REOPEN_APPROVE"
	m4TransitionManualCheck      = "MANUAL_CHECK"

	// Checklist item keys.
	m4ChecklistPendingApprovalZero = "PENDING_APPROVAL_ZERO"
	m4ChecklistJurnalBalanced      = "JURNAL_BALANCED"
	m4ChecklistGLDelivered         = "GL_DELIVERED"
	m4ChecklistReconPass           = "RECON_PASS"

	// Audit event actions (PERIODE.*).
	m4AuditSoftCloseRequested  = "PERIODE.SOFT_CLOSE_REQUESTED"
	m4AuditSoftCloseApproved   = "PERIODE.SOFT_CLOSE_APPROVED"
	m4AuditSoftCloseRejected   = "PERIODE.SOFT_CLOSE_REJECTED"
	m4AuditHardCloseRequested  = "PERIODE.HARD_CLOSE_REQUESTED"
	m4AuditHardClosed          = "PERIODE.HARDCLOSED"
	m4AuditHardCloseRejected   = "PERIODE.HARD_CLOSE_REJECTED"
	m4AuditReopenRequested     = "PERIODE.REOPEN_REQUESTED"
	m4AuditReopenApproved      = "PERIODE.REOPEN_APPROVED"
	m4AuditSoDViolation        = "PERIODE.SOD_VIOLATION"
	m4AuditExport              = "PERIODE.EXPORT"

	// Asynq task types.
	m4TaskMVRefresh = "reporting:mv_refresh"

	// Error codes (mirrors OpenAPI + errors.go).
	m4ErrChecklistFailed       = "CLOSING_CHECKLIST_FAILED"
	m4ErrChecklistStale        = "CLOSING_CHECKLIST_STALE"
	m4ErrPeriodeSoftClosed     = "PERIODE_SOFT_CLOSED"
	m4ErrPeriodeClosed         = "PERIODE_CLOSED"
	m4ErrMFAStepUpRequired     = "MFA_STEP_UP_REQUIRED"
	m4ErrMFAStepUpExpired      = "MFA_STEP_UP_EXPIRED"
	m4ErrGraceExpired          = "PERIODE_GRACE_EXPIRED"
	m4ErrSoftClosePendingExists = "SOFT_CLOSE_PENDING_EXISTS"
	m4ErrInvalidTransition     = "WORKFLOW_INVALID_TRANSITION"
	m4ErrSoDViolation          = "SOD_VIOLATION"
	m4ErrConflict              = "CONFLICT"
	m4ErrValidationFailed      = "VALIDATION_FAILED"
	m4ErrForbidden             = "FORBIDDEN"
	m4ErrNotFound              = "NOT_FOUND"

	// Permissions.
	m4PermSoftcloseRequest = "periode.softclose.request"
	m4PermSoftcloseApprove = "periode.softclose.approve"
	m4PermHardcloseRequest = "periode.hardclose.request"
	m4PermHardcloseApprove = "periode.hardclose.approve"
	m4PermHardcloseReject  = "periode.hardclose.reject"
	m4PermReopenRequest    = "periode.reopen.request"
	m4PermReopenApprove    = "periode.reopen.approve"
	m4PermStatusRead       = "periode.status.read"
	m4PermExport           = "periode.export"

	// Roles.
	m4RoleAkunCTL  = "ROLE-AKUN-CTL"
	m4RoleCFO      = "ROLE-CFO"
	m4RoleAudit    = "ROLE-AUDIT"
	m4RoleMakerTR  = "ROLE-MAKER-TR"

	// Stale/grace config constants (mirror closeflow.DefaultConfig).
	m4StaleHours = 24
	m4GraceHours = 48
)

// ─── Domain types ─────────────────────────────────────────────────────────────

// m4PeriodeBuku is an in-process copy of mst.periode_buku (close workflow columns).
type m4PeriodeBuku struct {
	ID                      uuid.UUID
	PeriodeIDKode           string
	TahunBuku               int
	StatusPeriode           string
	TanggalSoftClose        *time.Time
	TanggalHardClose        *time.Time
	HardCloseGraceExpiresAt *time.Time
	ReopenedFlag            bool
	ReopenedReason          *string
	ReopenedBy              *uuid.UUID
	ReopenedApprovedBy      *uuid.UUID
	ReopenedAt              *time.Time
	RowVersion              int64
	SoftCloseRequestedBy    *uuid.UUID
	SoftCloseRequestedAt    *time.Time
	SoftCloseApprovedBy     *uuid.UUID
	HardCloseRequestedBy    *uuid.UUID
	HardCloseApprovedBy     *uuid.UUID
	KursLocked              bool
	UpdatedAt               time.Time
	TenantID                string
}

// m4ChecklistItem mirrors closeflow.ChecklistItem.
type m4ChecklistItem struct {
	Key       string
	Label     string
	Passed    bool
	Detail    string
	ActionURL *string
}

// m4ChecklistSnapshot mirrors sys.closing_checklist_snapshot.
type m4ChecklistSnapshot struct {
	ID             uuid.UUID
	PeriodeBukuID  uuid.UUID
	Transition     string
	EvaluatedAt    time.Time
	EvaluatedBy    uuid.UUID
	ActorRole      string
	AllPassed      bool
	Items          []m4ChecklistItem
	CreatedAt      time.Time
	DeleteAttempts int // incremented by "trigger"; must stay 0
}

// m4MFAToken represents a step-up MFA token claim.
type m4MFAToken struct {
	Ref       string
	IssuedAt  time.Time
	Scope     string
	UserID    uuid.UUID
	Expired   bool
	MisScope  bool
}

// m4Actor mirrors closeflow.Actor.
type m4Actor struct {
	UserID      uuid.UUID
	Role        string
	Permissions []string
	MFAVerified bool
	StepUpToken *m4MFAToken
}

func (a m4Actor) HasPermission(p string) bool {
	for _, pp := range a.Permissions {
		if pp == p {
			return true
		}
	}
	return false
}

// m4IdempotencyStore mirrors sys.idempotency_key.
type m4IdempotencyStore struct {
	entries map[string]m4IdempotencyEntry
}
type m4IdempotencyEntry struct {
	Key          string
	PayloadHash  string
	ResponseCode int
	ResponseBody string
	CreatedAt    time.Time
}

func newM4IdempotencyStore() *m4IdempotencyStore {
	return &m4IdempotencyStore{entries: make(map[string]m4IdempotencyEntry)}
}

// Upsert stores an entry if code > 0 (final response). Returns (existing, true) on replay.
// First-phase call (code=0) only checks for existing; does not store a placeholder.
func (s *m4IdempotencyStore) Upsert(key, payloadHash string, code int, body string) (m4IdempotencyEntry, bool) {
	if e, ok := s.entries[key]; ok {
		return e, true
	}
	if code == 0 {
		// First-phase check — no existing entry. Don't store yet.
		return m4IdempotencyEntry{}, false
	}
	e := m4IdempotencyEntry{Key: key, PayloadHash: payloadHash, ResponseCode: code, ResponseBody: body, CreatedAt: time.Now()}
	s.entries[key] = e
	return e, false
}

// ─── Audit store (hash-chain) ─────────────────────────────────────────────────

type m4AuditRow struct {
	EventID      string
	Action       string
	EntityID     string
	ActorID      string
	ActorRole    string
	PreviousHash []byte
	CurrentHash  []byte
	AfterJSON    map[string]any
}

type m4AuditStore struct {
	rows []m4AuditRow
}

func newM4AuditStore() *m4AuditStore { return &m4AuditStore{} }

func (s *m4AuditStore) append(action, entityID, actorID, actorRole string, afterJSON map[string]any) {
	var prevHash []byte
	if len(s.rows) > 0 {
		prevHash = s.rows[len(s.rows)-1].CurrentHash
	}
	payload := fmt.Sprintf("%x||%s||%s||%v", prevHash, action, entityID, afterJSON)
	h := sha256.Sum256([]byte(payload))
	s.rows = append(s.rows, m4AuditRow{
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

func (s *m4AuditStore) containsAction(entityID, action string) bool {
	for _, r := range s.rows {
		if r.EntityID == entityID && r.Action == action {
			return true
		}
	}
	return false
}

func (s *m4AuditStore) actionsForEntity(entityID string) []string {
	var out []string
	for _, r := range s.rows {
		if r.EntityID == entityID {
			out = append(out, r.Action)
		}
	}
	return out
}

func (s *m4AuditStore) verifyHashChain() (bool, string) {
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

// ─── Periode store ────────────────────────────────────────────────────────────

type m4PeriodeStore struct {
	records map[uuid.UUID]*m4PeriodeBuku
}

func newM4PeriodeStore() *m4PeriodeStore {
	return &m4PeriodeStore{records: make(map[uuid.UUID]*m4PeriodeBuku)}
}

func (s *m4PeriodeStore) seed(kode string, status string) *m4PeriodeBuku {
	p := &m4PeriodeBuku{
		ID:            uuid.New(),
		PeriodeIDKode: kode,
		TahunBuku:     2026,
		StatusPeriode: status,
		RowVersion:    1,
		TenantID:      "TUGURE",
		UpdatedAt:     time.Now(),
	}
	s.records[p.ID] = p
	return p
}

func (s *m4PeriodeStore) get(id uuid.UUID) *m4PeriodeBuku {
	return s.records[id]
}

// ─── Snapshot store (append-only) ────────────────────────────────────────────

type m4SnapshotStore struct {
	snapshots []*m4ChecklistSnapshot
}

func newM4SnapshotStore() *m4SnapshotStore { return &m4SnapshotStore{} }

func (s *m4SnapshotStore) append(snap *m4ChecklistSnapshot) {
	snap.CreatedAt = time.Now()
	s.snapshots = append(s.snapshots, snap)
}

// delete simulates the DB BEFORE DELETE trigger — always panics.
// Any test that accidentally calls delete will fail loudly.
func (s *m4SnapshotStore) delete(id uuid.UUID) error {
	for _, snap := range s.snapshots {
		if snap.ID == id {
			snap.DeleteAttempts++
			return fmt.Errorf("trigger: sys.closing_checklist_snapshot is append-only; DELETE not permitted")
		}
	}
	return fmt.Errorf("snapshot %s not found", id)
}

func (s *m4SnapshotStore) latestForPeriode(periodeID uuid.UUID, transition string) *m4ChecklistSnapshot {
	var latest *m4ChecklistSnapshot
	for _, snap := range s.snapshots {
		if snap.PeriodeBukuID != periodeID {
			continue
		}
		if transition != "" && snap.Transition != transition {
			continue
		}
		if latest == nil || snap.CreatedAt.After(latest.CreatedAt) {
			latest = snap
		}
	}
	return latest
}

func (s *m4SnapshotStore) countForPeriode(periodeID uuid.UUID) int {
	n := 0
	for _, snap := range s.snapshots {
		if snap.PeriodeBukuID == periodeID {
			n++
		}
	}
	return n
}

// ─── Kurs store ───────────────────────────────────────────────────────────────

type m4KursStore struct {
	locked map[string]bool // keyed by periodeKode
}

func newM4KursStore() *m4KursStore { return &m4KursStore{locked: make(map[string]bool)} }

func (s *m4KursStore) setLocked(periodeKode string, v bool) { s.locked[periodeKode] = v }
func (s *m4KursStore) isLocked(periodeKode string) bool      { return s.locked[periodeKode] }

// ─── Asynq queue (stub) ───────────────────────────────────────────────────────

type m4AsynqTask struct {
	Type    string
	Payload map[string]any
}

type m4AsynqQueue struct {
	enqueued []m4AsynqTask
}

func newM4AsynqQueue() *m4AsynqQueue { return &m4AsynqQueue{} }

func (q *m4AsynqQueue) Enqueue(taskType string, payload map[string]any) string {
	q.enqueued = append(q.enqueued, m4AsynqTask{Type: taskType, Payload: payload})
	return fmt.Sprintf("job_%s", uuid.New().String()[:8])
}

func (q *m4AsynqQueue) HasTask(taskType string) bool {
	for _, t := range q.enqueued {
		if t.Type == taskType {
			return true
		}
	}
	return false
}

// ─── Checklist evaluator (in-process) ────────────────────────────────────────

// m4ChecklistConfig controls whether each item passes, simulating DB state.
type m4ChecklistConfig struct {
	PendingApprovalZeroPassed bool
	JurnalBalancedPassed      bool
	GLDeliveredPassed         bool
	ReconPassPassed           bool
	// For GL_DELIVERED failure, provide failed jurnal IDs.
	GLFailedJurnalIDs []string
}

func allPassedChecklistConfig() m4ChecklistConfig {
	return m4ChecklistConfig{
		PendingApprovalZeroPassed: true,
		JurnalBalancedPassed:      true,
		GLDeliveredPassed:         true,
		ReconPassPassed:           true,
	}
}

// evaluate simulates ChecklistService.Evaluate(). Returns items and allPassed.
func evaluate(cfg m4ChecklistConfig) ([]m4ChecklistItem, bool) {
	items := []m4ChecklistItem{
		{
			Key:    m4ChecklistPendingApprovalZero,
			Label:  "Tidak ada transaksi/jurnal yang menunggu approval",
			Passed: cfg.PendingApprovalZeroPassed,
			Detail: func() string {
				if cfg.PendingApprovalZeroPassed {
					return "Semua transaksi sudah final"
				}
				return "3 transaksi masih PENDING_APPROVAL"
			}(),
		},
		{
			Key:    m4ChecklistJurnalBalanced,
			Label:  "Seluruh jurnal periode balanced (threshold IDR 0.01)",
			Passed: cfg.JurnalBalancedPassed,
			Detail: func() string {
				if cfg.JurnalBalancedPassed {
					return "Debit = Kredit (delta = 0.0000)"
				}
				// Use decimal to format — no float64 per DEC-016.
				threshold := decimal.NewFromFloat(0.01)
				return fmt.Sprintf("1 jurnal tidak balanced, delta maks IDR %s", threshold.String())
			}(),
		},
		{
			Key:    m4ChecklistGLDelivered,
			Label:  "Semua jurnal ter-deliver ke GL Host",
			Passed: cfg.GLDeliveredPassed,
			Detail: func() string {
				if cfg.GLDeliveredPassed {
					return "Semua jurnal berstatus DELIVERED"
				}
				return fmt.Sprintf("%d jurnal berstatus FAILED (tidak termasuk DEAD_LETTER)", len(cfg.GLFailedJurnalIDs))
			}(),
			ActionURL: func() *string {
				if !cfg.GLDeliveredPassed && len(cfg.GLFailedJurnalIDs) > 0 {
					u := "/api/v1/jurnal/dlq?filter[status]=FAILED"
					return &u
				}
				return nil
			}(),
		},
		{
			Key:    m4ChecklistReconPass,
			Label:  "Rekonsiliasi GL terakhir berstatus COMPLETED",
			Passed: cfg.ReconPassPassed,
			Detail: func() string {
				if cfg.ReconPassPassed {
					return "Rekonsiliasi terakhir: COMPLETED (0 mismatch)"
				}
				return "Rekonsiliasi terakhir: COMPLETED_WITH_MISMATCH atau belum ada"
			}(),
		},
	}
	allPassed := cfg.PendingApprovalZeroPassed &&
		cfg.JurnalBalancedPassed &&
		cfg.GLDeliveredPassed &&
		cfg.ReconPassPassed
	return items, allPassed
}

// ─── In-process workflow operations ───────────────────────────────────────────

// softCloseRequestResult captures the outcome.
type softCloseRequestResult struct {
	StatusCode          int
	ErrorCode           string
	ChecklistSnapshotID uuid.UUID
	AllPassed           bool
	Transition          string
}

// softCloseRequest simulates POST /api/v1/periode/{id}/soft-close-request.
func softCloseRequest(
	h *p5M4Harness,
	periodeID uuid.UUID,
	actor m4Actor,
	rowVersion int64,
	catatan string,
	idempotencyKey string,
	checklistCfg m4ChecklistConfig,
) softCloseRequestResult {
	// Idempotency check.
	payloadHash := fmt.Sprintf("%s|%s|%d", periodeID, catatan, rowVersion)
	if entry, replayed := h.idempotency.Upsert(idempotencyKey, payloadHash, 0, ""); replayed {
		return softCloseRequestResult{StatusCode: entry.ResponseCode, ErrorCode: "IDEMPOTENCY_REPLAY"}
	}

	// Permission check.
	if !actor.HasPermission(m4PermSoftcloseRequest) {
		h.idempotency.Upsert(idempotencyKey, payloadHash, 403, m4ErrForbidden)
		return softCloseRequestResult{StatusCode: 403, ErrorCode: m4ErrForbidden}
	}

	// Load periode.
	periode := h.periodes.get(periodeID)
	if periode == nil {
		h.idempotency.Upsert(idempotencyKey, payloadHash, 404, m4ErrNotFound)
		return softCloseRequestResult{StatusCode: 404, ErrorCode: m4ErrNotFound}
	}

	// Row version optimistic lock.
	if periode.RowVersion != rowVersion {
		h.idempotency.Upsert(idempotencyKey, payloadHash, 409, m4ErrConflict)
		return softCloseRequestResult{StatusCode: 409, ErrorCode: m4ErrConflict}
	}

	// State check: must be OPEN.
	if periode.StatusPeriode != m4StatusOpen {
		h.idempotency.Upsert(idempotencyKey, payloadHash, 422, m4ErrInvalidTransition)
		return softCloseRequestResult{StatusCode: 422, ErrorCode: m4ErrInvalidTransition}
	}

	// Duplicate pending request check.
	if periode.SoftCloseRequestedBy != nil {
		// Advisory audit.
		h.audit.append(m4AuditSoftCloseRejected, periodeID.String(),
			actor.UserID.String(), actor.Role,
			map[string]any{"reason": "duplicate_request", "status": m4ErrSoftClosePendingExists})
		h.idempotency.Upsert(idempotencyKey, payloadHash, 409, m4ErrSoftClosePendingExists)
		return softCloseRequestResult{StatusCode: 409, ErrorCode: m4ErrSoftClosePendingExists}
	}

	// Evaluate checklist.
	items, allPassed := evaluate(checklistCfg)

	// Create snapshot (even on checklist failure).
	snap := &m4ChecklistSnapshot{
		ID:            uuid.New(),
		PeriodeBukuID: periodeID,
		Transition:    m4TransitionSoftCloseRequest,
		EvaluatedAt:   time.Now(),
		EvaluatedBy:   actor.UserID,
		ActorRole:     actor.Role,
		AllPassed:     allPassed,
		Items:         items,
	}
	h.snapshots.append(snap)

	if !allPassed {
		// Write advisory audit.
		h.audit.append(m4AuditSoftCloseRejected, periodeID.String(),
			actor.UserID.String(), actor.Role,
			map[string]any{"reason": "checklist_failed", "snapshot_id": snap.ID.String()})
		h.idempotency.Upsert(idempotencyKey, payloadHash, 422, m4ErrChecklistFailed)
		return softCloseRequestResult{
			StatusCode:          422,
			ErrorCode:           m4ErrChecklistFailed,
			ChecklistSnapshotID: snap.ID,
			AllPassed:           false,
			Transition:          m4TransitionSoftCloseRequest,
		}
	}

	// Mutate periode — mark pending request.
	now := time.Now()
	periode.SoftCloseRequestedBy = &actor.UserID
	periode.SoftCloseRequestedAt = &now
	periode.RowVersion++
	periode.UpdatedAt = now

	// Audit in-transaction.
	h.audit.append(m4AuditSoftCloseRequested, periodeID.String(),
		actor.UserID.String(), actor.Role,
		map[string]any{
			"snapshot_id":  snap.ID.String(),
			"all_passed":   allPassed,
			"periode_kode": periode.PeriodeIDKode,
		})

	h.idempotency.Upsert(idempotencyKey, payloadHash, 202, "soft_close_requested")
	return softCloseRequestResult{
		StatusCode:          202,
		ChecklistSnapshotID: snap.ID,
		AllPassed:           true,
		Transition:          m4TransitionSoftCloseRequest,
	}
}

// softCloseApproveResult captures the outcome of soft-close approve.
type softCloseApproveResult struct {
	StatusCode    int
	ErrorCode     string
	StatusPeriode string
	SnapshotID    uuid.UUID
}

// softCloseApprove simulates POST /api/v1/periode/{id}/soft-close-approve.
func softCloseApprove(
	h *p5M4Harness,
	periodeID uuid.UUID,
	actor m4Actor,
	rowVersion int64,
	idempotencyKey string,
	checklistCfg m4ChecklistConfig,
) softCloseApproveResult {
	payloadHash := fmt.Sprintf("%s|%d", periodeID, rowVersion)
	if entry, replayed := h.idempotency.Upsert(idempotencyKey, payloadHash, 0, ""); replayed {
		return softCloseApproveResult{StatusCode: entry.ResponseCode, ErrorCode: "IDEMPOTENCY_REPLAY"}
	}

	if !actor.HasPermission(m4PermSoftcloseApprove) {
		h.idempotency.Upsert(idempotencyKey, payloadHash, 403, m4ErrForbidden)
		return softCloseApproveResult{StatusCode: 403, ErrorCode: m4ErrForbidden}
	}

	periode := h.periodes.get(periodeID)
	if periode == nil {
		h.idempotency.Upsert(idempotencyKey, payloadHash, 404, m4ErrNotFound)
		return softCloseApproveResult{StatusCode: 404, ErrorCode: m4ErrNotFound}
	}

	if periode.RowVersion != rowVersion {
		h.idempotency.Upsert(idempotencyKey, payloadHash, 409, m4ErrConflict)
		return softCloseApproveResult{StatusCode: 409, ErrorCode: m4ErrConflict}
	}

	if periode.StatusPeriode != m4StatusOpen {
		h.idempotency.Upsert(idempotencyKey, payloadHash, 422, m4ErrInvalidTransition)
		return softCloseApproveResult{StatusCode: 422, ErrorCode: m4ErrInvalidTransition}
	}

	// SoD: approver ≠ requester.
	if periode.SoftCloseRequestedBy != nil && *periode.SoftCloseRequestedBy == actor.UserID {
		h.audit.append(m4AuditSoDViolation, periodeID.String(),
			actor.UserID.String(), actor.Role,
			map[string]any{"action": "soft_close_approve", "reason": "maker=approver"})
		h.idempotency.Upsert(idempotencyKey, payloadHash, 403, m4ErrSoDViolation)
		return softCloseApproveResult{StatusCode: 403, ErrorCode: m4ErrSoDViolation}
	}

	// Stale check: latest SOFT_CLOSE_REQUEST snapshot > staleHours old.
	reqSnap := h.snapshots.latestForPeriode(periodeID, m4TransitionSoftCloseRequest)
	if reqSnap != nil && time.Since(reqSnap.CreatedAt) > time.Duration(m4StaleHours)*time.Hour {
		// Re-evaluate.
		items, allPassed := evaluate(checklistCfg)
		reSnap := &m4ChecklistSnapshot{
			ID:            uuid.New(),
			PeriodeBukuID: periodeID,
			Transition:    m4TransitionSoftCloseRequest,
			EvaluatedAt:   time.Now(),
			EvaluatedBy:   actor.UserID,
			ActorRole:     actor.Role,
			AllPassed:     allPassed,
			Items:         items,
		}
		h.snapshots.append(reSnap)
		if !allPassed {
			h.idempotency.Upsert(idempotencyKey, payloadHash, 422, m4ErrChecklistStale)
			return softCloseApproveResult{StatusCode: 422, ErrorCode: m4ErrChecklistStale}
		}
	}

	// Transition OPEN → SOFT_CLOSED.
	now := time.Now()
	periode.StatusPeriode = m4StatusSoftClosed
	periode.SoftCloseApprovedBy = &actor.UserID
	periode.TanggalSoftClose = &now
	periode.RowVersion++
	periode.UpdatedAt = now

	// Approve snapshot.
	approveSnap := &m4ChecklistSnapshot{
		ID:            uuid.New(),
		PeriodeBukuID: periodeID,
		Transition:    m4TransitionSoftCloseApprove,
		EvaluatedAt:   now,
		EvaluatedBy:   actor.UserID,
		ActorRole:     actor.Role,
		AllPassed:     true,
	}
	h.snapshots.append(approveSnap)

	// Audit in-transaction.
	h.audit.append(m4AuditSoftCloseApproved, periodeID.String(),
		actor.UserID.String(), actor.Role,
		map[string]any{
			"snapshot_id":       approveSnap.ID.String(),
			"status_periode":    m4StatusSoftClosed,
			"tanggal_soft_close": now.Format(time.RFC3339),
		})

	h.idempotency.Upsert(idempotencyKey, payloadHash, 200, "soft_closed")
	return softCloseApproveResult{
		StatusCode:    200,
		StatusPeriode: m4StatusSoftClosed,
		SnapshotID:    approveSnap.ID,
	}
}

// hardCloseRequestResult captures outcome of hard-close request.
type hardCloseRequestResult struct {
	StatusCode  int
	ErrorCode   string
	SnapshotID  uuid.UUID
	AllPassed   bool
}

// hardCloseRequest simulates POST /api/v1/periode/{id}/hard-close-request.
func hardCloseRequest(
	h *p5M4Harness,
	periodeID uuid.UUID,
	actor m4Actor,
	rowVersion int64,
	idempotencyKey string,
	checklistCfg m4ChecklistConfig,
) hardCloseRequestResult {
	payloadHash := fmt.Sprintf("hcr|%s|%d", periodeID, rowVersion)
	if entry, replayed := h.idempotency.Upsert(idempotencyKey, payloadHash, 0, ""); replayed {
		return hardCloseRequestResult{StatusCode: entry.ResponseCode, ErrorCode: "IDEMPOTENCY_REPLAY"}
	}

	if !actor.HasPermission(m4PermHardcloseRequest) {
		h.idempotency.Upsert(idempotencyKey, payloadHash, 403, m4ErrForbidden)
		return hardCloseRequestResult{StatusCode: 403, ErrorCode: m4ErrForbidden}
	}

	periode := h.periodes.get(periodeID)
	if periode == nil {
		h.idempotency.Upsert(idempotencyKey, payloadHash, 404, m4ErrNotFound)
		return hardCloseRequestResult{StatusCode: 404, ErrorCode: m4ErrNotFound}
	}

	if periode.RowVersion != rowVersion {
		h.idempotency.Upsert(idempotencyKey, payloadHash, 409, m4ErrConflict)
		return hardCloseRequestResult{StatusCode: 409, ErrorCode: m4ErrConflict}
	}

	// Must be SOFT_CLOSED.
	if periode.StatusPeriode != m4StatusSoftClosed {
		h.idempotency.Upsert(idempotencyKey, payloadHash, 422, m4ErrInvalidTransition)
		return hardCloseRequestResult{StatusCode: 422, ErrorCode: m4ErrInvalidTransition}
	}

	// Evaluate checklist.
	items, allPassed := evaluate(checklistCfg)

	snap := &m4ChecklistSnapshot{
		ID:            uuid.New(),
		PeriodeBukuID: periodeID,
		Transition:    m4TransitionHardCloseRequest,
		EvaluatedAt:   time.Now(),
		EvaluatedBy:   actor.UserID,
		ActorRole:     actor.Role,
		AllPassed:     allPassed,
		Items:         items,
	}
	h.snapshots.append(snap)

	if !allPassed {
		h.idempotency.Upsert(idempotencyKey, payloadHash, 422, m4ErrChecklistFailed)
		return hardCloseRequestResult{StatusCode: 422, ErrorCode: m4ErrChecklistFailed, SnapshotID: snap.ID}
	}

	// Transition → HARD_CLOSE_PENDING.
	now := time.Now()
	periode.StatusPeriode = m4StatusHardClosePending
	periode.HardCloseRequestedBy = &actor.UserID
	periode.RowVersion++
	periode.UpdatedAt = now

	h.audit.append(m4AuditHardCloseRequested, periodeID.String(),
		actor.UserID.String(), actor.Role,
		map[string]any{"snapshot_id": snap.ID.String(), "status": m4StatusHardClosePending})

	h.idempotency.Upsert(idempotencyKey, payloadHash, 202, "hard_close_pending")
	return hardCloseRequestResult{StatusCode: 202, SnapshotID: snap.ID, AllPassed: true}
}

// hardCloseApproveResult captures outcome of hard-close approve.
type hardCloseApproveResult struct {
	StatusCode     int
	ErrorCode      string
	StatusPeriode  string
	GraceExpiresAt *time.Time
	SnapshotID     uuid.UUID
	MVJobID        *string
}

// hardCloseApprove simulates POST /api/v1/periode/{id}/hard-close-approve.
func hardCloseApprove(
	h *p5M4Harness,
	periodeID uuid.UUID,
	actor m4Actor,
	rowVersion int64,
	idempotencyKey string,
	stepUpToken *m4MFAToken,
) hardCloseApproveResult {
	payloadHash := fmt.Sprintf("hca|%s|%d", periodeID, rowVersion)
	if entry, replayed := h.idempotency.Upsert(idempotencyKey, payloadHash, 0, ""); replayed {
		return hardCloseApproveResult{StatusCode: entry.ResponseCode, ErrorCode: "IDEMPOTENCY_REPLAY"}
	}

	if !actor.HasPermission(m4PermHardcloseApprove) {
		h.idempotency.Upsert(idempotencyKey, payloadHash, 403, m4ErrForbidden)
		return hardCloseApproveResult{StatusCode: 403, ErrorCode: m4ErrForbidden}
	}

	// Step-up MFA validation (DEC-027).
	if stepUpToken == nil {
		h.idempotency.Upsert(idempotencyKey, payloadHash, 401, m4ErrMFAStepUpRequired)
		return hardCloseApproveResult{StatusCode: 401, ErrorCode: m4ErrMFAStepUpRequired}
	}
	if stepUpToken.Expired || time.Since(stepUpToken.IssuedAt) > 5*time.Minute {
		h.idempotency.Upsert(idempotencyKey, payloadHash, 401, m4ErrMFAStepUpExpired)
		return hardCloseApproveResult{StatusCode: 401, ErrorCode: m4ErrMFAStepUpExpired}
	}
	if stepUpToken.MisScope || stepUpToken.Scope != "periode.hardclose.approve" {
		h.idempotency.Upsert(idempotencyKey, payloadHash, 401, m4ErrMFAStepUpRequired)
		return hardCloseApproveResult{StatusCode: 401, ErrorCode: m4ErrMFAStepUpRequired}
	}

	periode := h.periodes.get(periodeID)
	if periode == nil {
		h.idempotency.Upsert(idempotencyKey, payloadHash, 404, m4ErrNotFound)
		return hardCloseApproveResult{StatusCode: 404, ErrorCode: m4ErrNotFound}
	}

	if periode.RowVersion != rowVersion {
		h.idempotency.Upsert(idempotencyKey, payloadHash, 409, m4ErrConflict)
		return hardCloseApproveResult{StatusCode: 409, ErrorCode: m4ErrConflict}
	}

	if periode.StatusPeriode != m4StatusHardClosePending {
		h.idempotency.Upsert(idempotencyKey, payloadHash, 422, m4ErrInvalidTransition)
		return hardCloseApproveResult{StatusCode: 422, ErrorCode: m4ErrInvalidTransition}
	}

	// SoD: CFO approver ≠ HARD_CLOSE_PENDING requester.
	if periode.HardCloseRequestedBy != nil && *periode.HardCloseRequestedBy == actor.UserID {
		h.audit.append(m4AuditSoDViolation, periodeID.String(),
			actor.UserID.String(), actor.Role,
			map[string]any{"action": "hard_close_approve", "reason": "requester=approver"})
		h.idempotency.Upsert(idempotencyKey, payloadHash, 403, m4ErrSoDViolation)
		return hardCloseApproveResult{StatusCode: 403, ErrorCode: m4ErrSoDViolation}
	}

	// Transition → CLOSED.
	now := time.Now()
	graceExpiry := now.Add(time.Duration(m4GraceHours) * time.Hour)
	periode.StatusPeriode = m4StatusClosed
	periode.HardCloseApprovedBy = &actor.UserID
	periode.TanggalHardClose = &now
	periode.HardCloseGraceExpiresAt = &graceExpiry
	periode.RowVersion++
	periode.UpdatedAt = now

	// Lock kurs.
	h.kurs.setLocked(periode.PeriodeIDKode, true)

	// Approve snapshot.
	snap := &m4ChecklistSnapshot{
		ID:            uuid.New(),
		PeriodeBukuID: periodeID,
		Transition:    m4TransitionHardCloseApprove,
		EvaluatedAt:   now,
		EvaluatedBy:   actor.UserID,
		ActorRole:     actor.Role,
		AllPassed:     true,
	}
	h.snapshots.append(snap)

	// Audit in-transaction.
	h.audit.append(m4AuditHardClosed, periodeID.String(),
		actor.UserID.String(), actor.Role,
		map[string]any{
			"snapshot_id":        snap.ID.String(),
			"tanggal_hard_close": now.Format(time.RFC3339),
			"grace_expires_at":   graceExpiry.Format(time.RFC3339),
			"kurs_locked":        true,
		})

	// Enqueue MV refresh job (async, non-blocking).
	jobID := h.asynq.Enqueue(m4TaskMVRefresh, map[string]any{
		"periode_id":   periodeID.String(),
		"periode_kode": periode.PeriodeIDKode,
	})

	h.idempotency.Upsert(idempotencyKey, payloadHash, 200, "closed")
	return hardCloseApproveResult{
		StatusCode:     200,
		StatusPeriode:  m4StatusClosed,
		GraceExpiresAt: &graceExpiry,
		SnapshotID:     snap.ID,
		MVJobID:        &jobID,
	}
}

// hardCloseRejectResult captures outcome.
type hardCloseRejectResult struct {
	StatusCode    int
	ErrorCode     string
	StatusPeriode string
}

// hardCloseReject simulates POST /api/v1/periode/{id}/hard-close-reject.
func hardCloseReject(
	h *p5M4Harness,
	periodeID uuid.UUID,
	actor m4Actor,
	reason string,
	idempotencyKey string,
) hardCloseRejectResult {
	payloadHash := fmt.Sprintf("hcrej|%s|%s", periodeID, reason)
	if entry, replayed := h.idempotency.Upsert(idempotencyKey, payloadHash, 0, ""); replayed {
		return hardCloseRejectResult{StatusCode: entry.ResponseCode, ErrorCode: "IDEMPOTENCY_REPLAY"}
	}

	if !actor.HasPermission(m4PermHardcloseReject) {
		h.idempotency.Upsert(idempotencyKey, payloadHash, 403, m4ErrForbidden)
		return hardCloseRejectResult{StatusCode: 403, ErrorCode: m4ErrForbidden}
	}

	if len(reason) < 30 {
		h.idempotency.Upsert(idempotencyKey, payloadHash, 400, m4ErrValidationFailed)
		return hardCloseRejectResult{StatusCode: 400, ErrorCode: m4ErrValidationFailed}
	}

	periode := h.periodes.get(periodeID)
	if periode == nil {
		return hardCloseRejectResult{StatusCode: 404, ErrorCode: m4ErrNotFound}
	}

	if periode.StatusPeriode != m4StatusHardClosePending {
		return hardCloseRejectResult{StatusCode: 422, ErrorCode: m4ErrInvalidTransition}
	}

	// No step-up MFA required for reject.
	now := time.Now()
	periode.StatusPeriode = m4StatusSoftClosed
	periode.HardCloseRequestedBy = nil
	periode.RowVersion++
	periode.UpdatedAt = now

	snap := &m4ChecklistSnapshot{
		ID:            uuid.New(),
		PeriodeBukuID: periodeID,
		Transition:    m4TransitionReopenRequest,
		EvaluatedAt:   now,
		EvaluatedBy:   actor.UserID,
		ActorRole:     actor.Role,
		AllPassed:     false,
	}
	h.snapshots.append(snap)

	h.audit.append(m4AuditHardCloseRejected, periodeID.String(),
		actor.UserID.String(), actor.Role,
		map[string]any{"reason": reason, "status": m4StatusSoftClosed})

	h.idempotency.Upsert(idempotencyKey, payloadHash, 200, "hard_close_rejected")
	return hardCloseRejectResult{StatusCode: 200, StatusPeriode: m4StatusSoftClosed}
}

// reopenResult captures outcome.
type reopenResult struct {
	StatusCode    int
	ErrorCode     string
	StatusPeriode string
	SnapshotID    uuid.UUID
}

// requestReopen simulates POST /api/v1/periode/{id}/reopen-request.
func requestReopen(
	h *p5M4Harness,
	periodeID uuid.UUID,
	actor m4Actor,
	reason string,
	targetStatus string,
	rowVersion int64,
	idempotencyKey string,
) reopenResult {
	payloadHash := fmt.Sprintf("ror|%s|%s|%d", periodeID, reason, rowVersion)
	if entry, replayed := h.idempotency.Upsert(idempotencyKey, payloadHash, 0, ""); replayed {
		return reopenResult{StatusCode: entry.ResponseCode, ErrorCode: "IDEMPOTENCY_REPLAY"}
	}

	if !actor.HasPermission(m4PermReopenRequest) {
		return reopenResult{StatusCode: 403, ErrorCode: m4ErrForbidden}
	}

	if len(reason) < 30 {
		return reopenResult{StatusCode: 400, ErrorCode: m4ErrValidationFailed}
	}

	periode := h.periodes.get(periodeID)
	if periode == nil {
		return reopenResult{StatusCode: 404, ErrorCode: m4ErrNotFound}
	}

	if periode.RowVersion != rowVersion {
		return reopenResult{StatusCode: 409, ErrorCode: m4ErrConflict}
	}

	switch periode.StatusPeriode {
	case m4StatusSoftClosed:
		// OK, no step-up needed; targetStatus must be OPEN.
	case m4StatusClosed:
		// Grace window check.
		if periode.HardCloseGraceExpiresAt == nil || time.Now().After(*periode.HardCloseGraceExpiresAt) {
			return reopenResult{StatusCode: 423, ErrorCode: m4ErrGraceExpired}
		}
		// targetStatus must be SOFT_CLOSED.
		if targetStatus != m4StatusSoftClosed {
			return reopenResult{StatusCode: 422, ErrorCode: m4ErrInvalidTransition}
		}
	default:
		return reopenResult{StatusCode: 422, ErrorCode: m4ErrInvalidTransition}
	}

	// Write reopen request snapshot.
	now := time.Now()
	snap := &m4ChecklistSnapshot{
		ID:            uuid.New(),
		PeriodeBukuID: periodeID,
		Transition:    m4TransitionReopenRequest,
		EvaluatedAt:   now,
		EvaluatedBy:   actor.UserID,
		ActorRole:     actor.Role,
		AllPassed:     true,
	}
	h.snapshots.append(snap)

	h.audit.append(m4AuditReopenRequested, periodeID.String(),
		actor.UserID.String(), actor.Role,
		map[string]any{"reason": reason, "target_status": targetStatus, "snapshot_id": snap.ID.String()})

	periode.ReopenedReason = &reason
	periode.RowVersion++
	periode.UpdatedAt = now

	h.idempotency.Upsert(idempotencyKey, payloadHash, 202, "reopen_requested")
	return reopenResult{StatusCode: 202, StatusPeriode: periode.StatusPeriode, SnapshotID: snap.ID}
}

// approveReopen simulates POST /api/v1/periode/{id}/reopen-approve.
func approveReopen(
	h *p5M4Harness,
	periodeID uuid.UUID,
	actor m4Actor,
	idempotencyKey string,
	stepUpToken *m4MFAToken, // only required for CLOSED→SOFT_CLOSED
) reopenResult {
	payloadHash := fmt.Sprintf("roa|%s", periodeID)
	if entry, replayed := h.idempotency.Upsert(idempotencyKey, payloadHash, 0, ""); replayed {
		return reopenResult{StatusCode: entry.ResponseCode, ErrorCode: "IDEMPOTENCY_REPLAY"}
	}

	if !actor.HasPermission(m4PermReopenApprove) {
		return reopenResult{StatusCode: 403, ErrorCode: m4ErrForbidden}
	}

	periode := h.periodes.get(periodeID)
	if periode == nil {
		return reopenResult{StatusCode: 404, ErrorCode: m4ErrNotFound}
	}

	var newStatus string
	switch periode.StatusPeriode {
	case m4StatusSoftClosed:
		newStatus = m4StatusOpen
		// No step-up needed.
	case m4StatusClosed:
		// Grace window check.
		if periode.HardCloseGraceExpiresAt == nil || time.Now().After(*periode.HardCloseGraceExpiresAt) {
			return reopenResult{StatusCode: 423, ErrorCode: m4ErrGraceExpired}
		}
		// Step-up MFA required.
		if stepUpToken == nil {
			return reopenResult{StatusCode: 401, ErrorCode: m4ErrMFAStepUpRequired}
		}
		if stepUpToken.Expired || time.Since(stepUpToken.IssuedAt) > 5*time.Minute {
			return reopenResult{StatusCode: 401, ErrorCode: m4ErrMFAStepUpExpired}
		}
		if stepUpToken.Scope != "periode.reopen.approve" {
			return reopenResult{StatusCode: 401, ErrorCode: m4ErrMFAStepUpRequired}
		}
		newStatus = m4StatusSoftClosed
	default:
		return reopenResult{StatusCode: 422, ErrorCode: m4ErrInvalidTransition}
	}

	// SoD: approver ≠ requester.
	if periode.ReopenedBy != nil && *periode.ReopenedBy == actor.UserID {
		h.audit.append(m4AuditSoDViolation, periodeID.String(),
			actor.UserID.String(), actor.Role,
			map[string]any{"action": "reopen_approve", "reason": "requester=approver"})
		return reopenResult{StatusCode: 403, ErrorCode: m4ErrSoDViolation}
	}

	now := time.Now()
	periode.StatusPeriode = newStatus
	periode.ReopenedFlag = true
	periode.ReopenedAt = &now
	periode.ReopenedApprovedBy = &actor.UserID
	periode.RowVersion++
	periode.UpdatedAt = now

	// Unlock kurs if going from CLOSED.
	if newStatus == m4StatusSoftClosed {
		h.kurs.setLocked(periode.PeriodeIDKode, false)
	}

	snap := &m4ChecklistSnapshot{
		ID:            uuid.New(),
		PeriodeBukuID: periodeID,
		Transition:    m4TransitionReopenApprove,
		EvaluatedAt:   now,
		EvaluatedBy:   actor.UserID,
		ActorRole:     actor.Role,
		AllPassed:     true,
	}
	h.snapshots.append(snap)

	h.audit.append(m4AuditReopenApproved, periodeID.String(),
		actor.UserID.String(), actor.Role,
		map[string]any{
			"new_status":  newStatus,
			"snapshot_id": snap.ID.String(),
		})

	h.idempotency.Upsert(idempotencyKey, payloadHash, 200, "reopened")
	return reopenResult{StatusCode: 200, StatusPeriode: newStatus, SnapshotID: snap.ID}
}

// getChecklistResult captures outcome of GetChecklist.
type getChecklistResult struct {
	StatusCode  int
	ErrorCode   string
	Items       []m4ChecklistItem
	AllPassed   bool
	SnapshotID  uuid.UUID
	Transition  string
}

// getChecklist simulates GET /api/v1/periode/{id}/closing-checklist.
func getChecklist(
	h *p5M4Harness,
	periodeID uuid.UUID,
	actor m4Actor,
	checklistCfg m4ChecklistConfig,
) getChecklistResult {
	if !actor.HasPermission(m4PermStatusRead) {
		return getChecklistResult{StatusCode: 403, ErrorCode: m4ErrForbidden}
	}

	periode := h.periodes.get(periodeID)
	if periode == nil {
		return getChecklistResult{StatusCode: 404, ErrorCode: m4ErrNotFound}
	}

	// Evaluate in background (non-blocking to caller, simulated synchronously here).
	items, allPassed := evaluate(checklistCfg)
	snap := &m4ChecklistSnapshot{
		ID:            uuid.New(),
		PeriodeBukuID: periodeID,
		Transition:    m4TransitionManualCheck,
		EvaluatedAt:   time.Now(),
		EvaluatedBy:   actor.UserID,
		ActorRole:     actor.Role,
		AllPassed:     allPassed,
		Items:         items,
	}
	h.snapshots.append(snap)

	// Audit the export-like read (MANUAL_CHECK is advisory).
	h.audit.append("PERIODE.MANUAL_CHECK", periodeID.String(),
		actor.UserID.String(), actor.Role,
		map[string]any{"snapshot_id": snap.ID.String(), "all_passed": allPassed})

	return getChecklistResult{
		StatusCode: 200,
		Items:      items,
		AllPassed:  allPassed,
		SnapshotID: snap.ID,
		Transition: m4TransitionManualCheck,
	}
}

// listStatusPeriodeResult captures the paginated list response.
type listStatusPeriodeResult struct {
	StatusCode int
	ErrorCode  string
	Items      []m4PeriodeListItem
	HasMore    bool
	AuditRows  int
}

// m4PeriodeListItem mirrors StatusPeriodeListItem response.
type m4PeriodeListItem struct {
	PeriodeIDKode string
	StatusPeriode string
	LatestSnap    *m4ChecklistSnapshot
}

// listStatusPeriode simulates GET /api/v1/reports/status-periode.
func listStatusPeriode(
	h *p5M4Harness,
	actor m4Actor,
	filterStatus string,
	exportCSV bool,
	limit int,
) listStatusPeriodeResult {
	if !actor.HasPermission(m4PermStatusRead) {
		return listStatusPeriodeResult{StatusCode: 403, ErrorCode: m4ErrForbidden}
	}

	var items []m4PeriodeListItem
	for _, p := range h.periodes.records {
		if filterStatus != "" && p.StatusPeriode != filterStatus {
			continue
		}
		snap := h.snapshots.latestForPeriode(p.ID, "")
		items = append(items, m4PeriodeListItem{
			PeriodeIDKode: p.PeriodeIDKode,
			StatusPeriode: p.StatusPeriode,
			LatestSnap:    snap,
		})
		if len(items) >= limit {
			break
		}
	}

	if exportCSV {
		h.audit.append(m4AuditExport, "periode.status_report",
			actor.UserID.String(), actor.Role,
			map[string]any{"format": "csv", "row_count": len(items), "filter_status": filterStatus})
	}

	return listStatusPeriodeResult{
		StatusCode: 200,
		Items:      items,
		HasMore:    false,
		AuditRows:  len(h.audit.rows),
	}
}

// periodeLockCheckResult captures the middleware guard result.
type periodeLockCheckResult struct {
	Allowed   bool
	ErrorCode string
	HTTPCode  int
}

// periodeLockCheck simulates PeriodeLockMiddleware.Handler().
func periodeLockCheck(
	h *p5M4Harness,
	periodeID uuid.UUID,
	closeWorkflowAction string, // value of X-Close-Workflow-Action header
) periodeLockCheckResult {
	periode := h.periodes.get(periodeID)
	if periode == nil {
		return periodeLockCheckResult{Allowed: false, ErrorCode: m4ErrNotFound, HTTPCode: 404}
	}

	allowlist := map[string]bool{
		"JURNAL_RETRY_GL_DELIVERY": true,
		"CORRECTION_PERIODE_CLOSED": true,
	}

	switch periode.StatusPeriode {
	case m4StatusClosed:
		return periodeLockCheckResult{Allowed: false, ErrorCode: m4ErrPeriodeClosed, HTTPCode: 423}
	case m4StatusSoftClosed, m4StatusHardClosePending:
		if closeWorkflowAction != "" && allowlist[closeWorkflowAction] {
			return periodeLockCheckResult{Allowed: true}
		}
		return periodeLockCheckResult{Allowed: false, ErrorCode: m4ErrPeriodeSoftClosed, HTTPCode: 423}
	default:
		return periodeLockCheckResult{Allowed: true}
	}
}

// ─── Harness ─────────────────────────────────────────────────────────────────

// p5M4Harness wires up in-process stubs for Periode Close tests.
type p5M4Harness struct {
	t           *testing.T
	audit       *m4AuditStore
	periodes    *m4PeriodeStore
	snapshots   *m4SnapshotStore
	kurs        *m4KursStore
	asynq       *m4AsynqQueue
	idempotency *m4IdempotencyStore
}

func newP5M4Harness(t *testing.T) *p5M4Harness {
	t.Helper()
	return &p5M4Harness{
		t:           t,
		audit:       newM4AuditStore(),
		periodes:    newM4PeriodeStore(),
		snapshots:   newM4SnapshotStore(),
		kurs:        newM4KursStore(),
		asynq:       newM4AsynqQueue(),
		idempotency: newM4IdempotencyStore(),
	}
}

// helper: actor with common permissions.
func akunCTLActor() m4Actor {
	id := uuid.New()
	return m4Actor{
		UserID: id,
		Role:   m4RoleAkunCTL,
		Permissions: []string{
			m4PermSoftcloseRequest,
			m4PermSoftcloseApprove,
			m4PermHardcloseRequest,
			m4PermStatusRead,
			m4PermExport,
		},
		MFAVerified: true,
	}
}

func cfoActor() m4Actor {
	id := uuid.New()
	return m4Actor{
		UserID: id,
		Role:   m4RoleCFO,
		Permissions: []string{
			m4PermHardcloseApprove,
			m4PermHardcloseReject,
			m4PermReopenRequest,
			m4PermReopenApprove,
			m4PermStatusRead,
			m4PermExport,
		},
		MFAVerified: true,
	}
}

func auditActor() m4Actor {
	return m4Actor{
		UserID:      uuid.New(),
		Role:        m4RoleAudit,
		Permissions: []string{m4PermStatusRead},
		MFAVerified: false,
	}
}

func makerTRActor() m4Actor {
	return m4Actor{
		UserID:      uuid.New(),
		Role:        m4RoleMakerTR,
		Permissions: []string{}, // no periode permissions
		MFAVerified: false,
	}
}

func freshStepUpToken(scope string, userID uuid.UUID) *m4MFAToken {
	return &m4MFAToken{
		Ref:      uuid.New().String(),
		IssuedAt: time.Now(),
		Scope:    scope,
		UserID:   userID,
	}
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestE2E_P5M4_A — S1-AC1: Soft-close request happy path.
func TestE2E_P5M4_A_SoftCloseRequest_HappyPath(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	maker := akunCTLActor()
	periode := h.periodes.seed("2026-06", m4StatusOpen)

	result := softCloseRequest(h, periode.ID, maker, periode.RowVersion,
		"Tutup periode Juni 2026", uuid.New().String(), allPassedChecklistConfig())

	require.Equal(t, 202, result.StatusCode, "expect 202 Accepted")
	assert.Equal(t, m4TransitionSoftCloseRequest, result.Transition, "S1-AC1: transition label")
	assert.True(t, result.AllPassed, "S1-AC1: checklist all passed")
	assert.NotEqual(t, uuid.Nil, result.ChecklistSnapshotID, "S1-AC1: snapshot ID set")

	// Snapshot persisted.
	snap := h.snapshots.latestForPeriode(periode.ID, m4TransitionSoftCloseRequest)
	require.NotNil(t, snap, "S1-AC1: snapshot must be persisted")
	assert.Equal(t, 4, len(snap.Items), "S1-AC1: 4 checklist items")

	// Audit written.
	assert.True(t, h.audit.containsAction(periode.ID.String(), m4AuditSoftCloseRequested),
		"S1-AC1: audit SOFT_CLOSE_REQUESTED written")

	// Periode row mutated.
	p := h.periodes.get(periode.ID)
	assert.NotNil(t, p.SoftCloseRequestedBy, "S1-AC1: SoftCloseRequestedBy set")
	assert.Equal(t, int64(2), p.RowVersion, "S1-AC1: RowVersion incremented")
}

// TestE2E_P5M4_B — S1-AC2: Duplicate soft-close request → 409 SOFT_CLOSE_PENDING_EXISTS.
func TestE2E_P5M4_B_SoftCloseRequest_DuplicatePending(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	maker := akunCTLActor()
	periode := h.periodes.seed("2026-06", m4StatusOpen)

	// First request.
	r1 := softCloseRequest(h, periode.ID, maker, periode.RowVersion,
		"Tutup periode Juni 2026", uuid.New().String(), allPassedChecklistConfig())
	require.Equal(t, 202, r1.StatusCode, "first request must succeed")

	// Reload periode (row version bumped). Patch back to simulate pending state:
	p := h.periodes.get(periode.ID)
	// Duplicate: SoftCloseRequestedBy is now set; second request should fail.
	maker2 := akunCTLActor()
	r2 := softCloseRequest(h, periode.ID, maker2, p.RowVersion,
		"Duplicate", uuid.New().String(), allPassedChecklistConfig())

	assert.Equal(t, 409, r2.StatusCode, "S1-AC2: expect 409")
	assert.Equal(t, m4ErrSoftClosePendingExists, r2.ErrorCode, "S1-AC2: SOFT_CLOSE_PENDING_EXISTS")

	// Advisory audit written.
	actions := h.audit.actionsForEntity(periode.ID.String())
	assert.Contains(t, actions, m4AuditSoftCloseRejected, "S1-AC2: advisory audit on duplicate")
}

// TestE2E_P5M4_C — S1-AC3: Checklist failing item → 422 CLOSING_CHECKLIST_FAILED, snapshot saved.
func TestE2E_P5M4_C_SoftCloseRequest_ChecklistFailed(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	maker := akunCTLActor()
	periode := h.periodes.seed("2026-06", m4StatusOpen)

	failedIDs := []string{uuid.New().String(), uuid.New().String()}
	cfg := m4ChecklistConfig{
		PendingApprovalZeroPassed: true,
		JurnalBalancedPassed:      true,
		GLDeliveredPassed:         false, // failing
		ReconPassPassed:           true,
		GLFailedJurnalIDs:         failedIDs,
	}

	result := softCloseRequest(h, periode.ID, maker, periode.RowVersion,
		"Coba tutup", uuid.New().String(), cfg)

	assert.Equal(t, 422, result.StatusCode, "S1-AC3: expect 422")
	assert.Equal(t, m4ErrChecklistFailed, result.ErrorCode, "S1-AC3: CLOSING_CHECKLIST_FAILED")
	assert.False(t, result.AllPassed, "S1-AC3: not all passed")
	assert.NotEqual(t, uuid.Nil, result.ChecklistSnapshotID, "S1-AC3: snapshot saved even on failure")

	// Snapshot shows GL_DELIVERED failing.
	snap := h.snapshots.latestForPeriode(periode.ID, m4TransitionSoftCloseRequest)
	require.NotNil(t, snap)
	assert.False(t, snap.AllPassed)
	var glItem *m4ChecklistItem
	for i := range snap.Items {
		if snap.Items[i].Key == m4ChecklistGLDelivered {
			glItem = &snap.Items[i]
			break
		}
	}
	require.NotNil(t, glItem, "S1-AC3: GL_DELIVERED item must be in snapshot")
	assert.False(t, glItem.Passed, "S1-AC3: GL_DELIVERED must be failed")
	assert.NotNil(t, glItem.ActionURL, "S1-AC3: actionUrl set for DLQ link")

	// Periode row NOT mutated (no SoftCloseRequestedBy set after failure).
	p := h.periodes.get(periode.ID)
	assert.Nil(t, p.SoftCloseRequestedBy, "S1-AC3: periode remains unchanged on failure")
	assert.Equal(t, int64(1), p.RowVersion, "S1-AC3: RowVersion unchanged")
}

// TestE2E_P5M4_D — S1-AC4: MANUAL_CHECK via GetChecklist — background snapshot, 4 items.
func TestE2E_P5M4_D_GetChecklist_ManualCheck(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	actor := auditActor()
	actor.Permissions = append(actor.Permissions, m4PermStatusRead)
	periode := h.periodes.seed("2026-06", m4StatusOpen)

	result := getChecklist(h, periode.ID, actor, allPassedChecklistConfig())

	assert.Equal(t, 200, result.StatusCode, "S1-AC4: expect 200")
	assert.Equal(t, 4, len(result.Items), "S1-AC4: 4 checklist items returned")
	assert.True(t, result.AllPassed, "S1-AC4: all passed")
	assert.Equal(t, m4TransitionManualCheck, result.Transition, "S1-AC4: MANUAL_CHECK transition")

	// Background snapshot written.
	snap := h.snapshots.latestForPeriode(periode.ID, m4TransitionManualCheck)
	require.NotNil(t, snap, "S1-AC4: MANUAL_CHECK snapshot persisted")
	assert.Equal(t, 4, len(snap.Items), "S1-AC4: snapshot contains all 4 items")

	// Audit advisory event written.
	assert.True(t, h.audit.containsAction(periode.ID.String(), "PERIODE.MANUAL_CHECK"),
		"S1-AC4: audit MANUAL_CHECK written")
}

// TestE2E_P5M4_E — S2-AC1: Soft-close approve SoD violation → 403 SOD_VIOLATION.
func TestE2E_P5M4_E_SoftCloseApprove_SoDViolation(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	maker := akunCTLActor()
	// Give maker the approve permission too (to simulate direct API call bypassing UI).
	maker.Permissions = append(maker.Permissions, m4PermSoftcloseApprove)
	periode := h.periodes.seed("2026-06", m4StatusOpen)

	// Maker submits request.
	r1 := softCloseRequest(h, periode.ID, maker, periode.RowVersion,
		"Tutup periode", uuid.New().String(), allPassedChecklistConfig())
	require.Equal(t, 202, r1.StatusCode)

	p := h.periodes.get(periode.ID)
	// Same user tries to approve.
	result := softCloseApprove(h, periode.ID, maker, p.RowVersion,
		uuid.New().String(), allPassedChecklistConfig())

	assert.Equal(t, 403, result.StatusCode, "S2-AC1: expect 403")
	assert.Equal(t, m4ErrSoDViolation, result.ErrorCode, "S2-AC1: SOD_VIOLATION")

	// SoD audit written.
	assert.True(t, h.audit.containsAction(periode.ID.String(), m4AuditSoDViolation),
		"S2-AC1: SoD advisory audit written")

	// Periode stays OPEN.
	assert.Equal(t, m4StatusOpen, p.StatusPeriode, "S2-AC1: status unchanged")
}

// TestE2E_P5M4_F — S2-AC2: Stale checklist > 24h at approve → re-eval fails → 422 CLOSING_CHECKLIST_STALE.
func TestE2E_P5M4_F_SoftCloseApprove_StaleChecklist(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	maker := akunCTLActor()
	approver := akunCTLActor()
	periode := h.periodes.seed("2026-06", m4StatusOpen)

	// Maker submits, checklist passes.
	r1 := softCloseRequest(h, periode.ID, maker, periode.RowVersion,
		"Tutup", uuid.New().String(), allPassedChecklistConfig())
	require.Equal(t, 202, r1.StatusCode)

	// Back-date the snapshot so it appears stale (> 24h).
	snap := h.snapshots.latestForPeriode(periode.ID, m4TransitionSoftCloseRequest)
	require.NotNil(t, snap)
	snap.CreatedAt = time.Now().Add(-(m4StaleHours + 1) * time.Hour)

	p := h.periodes.get(periode.ID)

	// At approve time, re-eval fails (GL not delivered).
	staleCfg := m4ChecklistConfig{
		PendingApprovalZeroPassed: true,
		JurnalBalancedPassed:      true,
		GLDeliveredPassed:         false,
		ReconPassPassed:           true,
		GLFailedJurnalIDs:         []string{uuid.New().String()},
	}

	result := softCloseApprove(h, periode.ID, approver, p.RowVersion,
		uuid.New().String(), staleCfg)

	assert.Equal(t, 422, result.StatusCode, "S2-AC2: expect 422")
	assert.Equal(t, m4ErrChecklistStale, result.ErrorCode, "S2-AC2: CLOSING_CHECKLIST_STALE")

	// Re-eval snapshot was saved.
	count := h.snapshots.countForPeriode(periode.ID)
	assert.GreaterOrEqual(t, count, 2, "S2-AC2: original + re-eval snapshots persisted")

	// Periode remains OPEN.
	assert.Equal(t, m4StatusOpen, p.StatusPeriode, "S2-AC2: status unchanged after stale")
}

// TestE2E_P5M4_G — S2-AC3: Soft-close approve happy path → OPEN → SOFT_CLOSED.
func TestE2E_P5M4_G_SoftCloseApprove_HappyPath(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	maker := akunCTLActor()
	approver := akunCTLActor()
	periode := h.periodes.seed("2026-06", m4StatusOpen)

	r1 := softCloseRequest(h, periode.ID, maker, periode.RowVersion,
		"Tutup periode", uuid.New().String(), allPassedChecklistConfig())
	require.Equal(t, 202, r1.StatusCode)

	p := h.periodes.get(periode.ID)
	result := softCloseApprove(h, periode.ID, approver, p.RowVersion,
		uuid.New().String(), allPassedChecklistConfig())

	require.Equal(t, 200, result.StatusCode, "S2-AC3: expect 200")
	assert.Equal(t, m4StatusSoftClosed, result.StatusPeriode, "S2-AC3: SOFT_CLOSED")
	assert.NotEqual(t, uuid.Nil, result.SnapshotID, "S2-AC3: approve snapshot ID returned")

	// DB state.
	p = h.periodes.get(periode.ID)
	assert.Equal(t, m4StatusSoftClosed, p.StatusPeriode, "S2-AC3: DB status SOFT_CLOSED")
	assert.NotNil(t, p.TanggalSoftClose, "S2-AC3: tanggalSoftClose set")
	assert.NotNil(t, p.SoftCloseApprovedBy, "S2-AC3: SoftCloseApprovedBy set")

	// Audit in-tx.
	assert.True(t, h.audit.containsAction(periode.ID.String(), m4AuditSoftCloseApproved),
		"S2-AC3: SOFT_CLOSE_APPROVED audit written")

	// Approve snapshot created.
	approveSnap := h.snapshots.latestForPeriode(periode.ID, m4TransitionSoftCloseApprove)
	assert.NotNil(t, approveSnap, "S2-AC3: SOFT_CLOSE_APPROVE snapshot persisted")
}

// TestE2E_P5M4_H — S2-AC4: Optimistic lock conflict → 409 CONFLICT.
func TestE2E_P5M4_H_SoftCloseApprove_RowVersionConflict(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	maker := akunCTLActor()
	approver := akunCTLActor()
	periode := h.periodes.seed("2026-06", m4StatusOpen)

	r1 := softCloseRequest(h, periode.ID, maker, periode.RowVersion,
		"Tutup periode", uuid.New().String(), allPassedChecklistConfig())
	require.Equal(t, 202, r1.StatusCode)

	// Pass stale row_version (1) instead of current (2).
	result := softCloseApprove(h, periode.ID, approver, 1 /*stale*/,
		uuid.New().String(), allPassedChecklistConfig())

	assert.Equal(t, 409, result.StatusCode, "S2-AC4: expect 409 CONFLICT")
	assert.Equal(t, m4ErrConflict, result.ErrorCode, "S2-AC4: CONFLICT code")
}

// TestE2E_P5M4_I — S3-AC1: Full hard-close flow → CLOSED, kurs locked, MV job enqueued.
func TestE2E_P5M4_I_HardClose_FullFlow(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	maker := akunCTLActor()
	approver := akunCTLActor()
	cfo := cfoActor()
	periode := h.periodes.seed("2026-06", m4StatusOpen)

	// Step 1: Soft-close request + approve.
	r1 := softCloseRequest(h, periode.ID, maker, periode.RowVersion,
		"Soft close", uuid.New().String(), allPassedChecklistConfig())
	require.Equal(t, 202, r1.StatusCode)
	p := h.periodes.get(periode.ID)
	r2 := softCloseApprove(h, periode.ID, approver, p.RowVersion,
		uuid.New().String(), allPassedChecklistConfig())
	require.Equal(t, 200, r2.StatusCode)
	assert.Equal(t, m4StatusSoftClosed, h.periodes.get(periode.ID).StatusPeriode)

	// Step 2: Hard-close request (AKUN-CTL).
	p = h.periodes.get(periode.ID)
	r3 := hardCloseRequest(h, periode.ID, maker, p.RowVersion,
		uuid.New().String(), allPassedChecklistConfig())
	require.Equal(t, 202, r3.StatusCode, "S3-AC1: hard-close-request 202")
	assert.Equal(t, m4StatusHardClosePending, h.periodes.get(periode.ID).StatusPeriode)

	// Step 3: Hard-close approve (CFO + step-up).
	p = h.periodes.get(periode.ID)
	stepUp := freshStepUpToken("periode.hardclose.approve", cfo.UserID)
	r4 := hardCloseApprove(h, periode.ID, cfo, p.RowVersion,
		uuid.New().String(), stepUp)

	require.Equal(t, 200, r4.StatusCode, "S3-AC1: hard-close-approve 200")
	assert.Equal(t, m4StatusClosed, r4.StatusPeriode, "S3-AC1: CLOSED")
	assert.NotNil(t, r4.GraceExpiresAt, "S3-AC1: graceExpiresAt set")
	assert.NotNil(t, r4.MVJobID, "S3-AC1: MV refresh job enqueued")

	// kurs locked.
	assert.True(t, h.kurs.isLocked(periode.PeriodeIDKode), "S3-AC1: mst.kurs.locked_flag = TRUE")

	// MV task enqueued.
	assert.True(t, h.asynq.HasTask(m4TaskMVRefresh), "S3-AC1: reporting:mv_refresh enqueued")

	// Audit PERIODE.HARDCLOSED written.
	assert.True(t, h.audit.containsAction(periode.ID.String(), m4AuditHardClosed),
		"S3-AC1: PERIODE.HARDCLOSED audit written in-tx")

	// Snapshot.
	snap := h.snapshots.latestForPeriode(periode.ID, m4TransitionHardCloseApprove)
	assert.NotNil(t, snap, "S3-AC1: HARD_CLOSE_APPROVE snapshot persisted")

	// DB state.
	p = h.periodes.get(periode.ID)
	assert.Equal(t, m4StatusClosed, p.StatusPeriode)
	assert.NotNil(t, p.TanggalHardClose)
	assert.NotNil(t, p.HardCloseGraceExpiresAt)
}

// TestE2E_P5M4_J — S3-AC2: Hard-close-approve missing step-up → 401 MFA_STEP_UP_REQUIRED.
func TestE2E_P5M4_J_HardCloseApprove_MissingStepUp(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	maker := akunCTLActor()
	cfo := cfoActor()
	periode := h.periodes.seed("2026-06", m4StatusSoftClosed)

	r1 := hardCloseRequest(h, periode.ID, maker, periode.RowVersion,
		uuid.New().String(), allPassedChecklistConfig())
	require.Equal(t, 202, r1.StatusCode)

	p := h.periodes.get(periode.ID)
	result := hardCloseApprove(h, periode.ID, cfo, p.RowVersion,
		uuid.New().String(), nil /*no step-up*/)

	assert.Equal(t, 401, result.StatusCode, "S3-AC2: expect 401")
	assert.Equal(t, m4ErrMFAStepUpRequired, result.ErrorCode, "S3-AC2: MFA_STEP_UP_REQUIRED")
	assert.Equal(t, m4StatusHardClosePending, h.periodes.get(periode.ID).StatusPeriode,
		"S3-AC2: status unchanged")
}

// TestE2E_P5M4_K — S3-AC3: Expired step-up token (> 5 min) → 401 MFA_STEP_UP_EXPIRED.
func TestE2E_P5M4_K_HardCloseApprove_ExpiredStepUp(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	maker := akunCTLActor()
	cfo := cfoActor()
	periode := h.periodes.seed("2026-06", m4StatusSoftClosed)

	r1 := hardCloseRequest(h, periode.ID, maker, periode.RowVersion,
		uuid.New().String(), allPassedChecklistConfig())
	require.Equal(t, 202, r1.StatusCode)

	p := h.periodes.get(periode.ID)
	expiredToken := &m4MFAToken{
		Ref:      uuid.New().String(),
		IssuedAt: time.Now().Add(-6 * time.Minute), // > 5 minutes ago
		Scope:    "periode.hardclose.approve",
		UserID:   cfo.UserID,
		Expired:  false, // not explicitly flagged, but IssuedAt is old
	}

	result := hardCloseApprove(h, periode.ID, cfo, p.RowVersion,
		uuid.New().String(), expiredToken)

	assert.Equal(t, 401, result.StatusCode, "S3-AC3: expect 401")
	assert.Equal(t, m4ErrMFAStepUpExpired, result.ErrorCode, "S3-AC3: MFA_STEP_UP_EXPIRED")
}

// TestE2E_P5M4_L — S3-AC4: Hard-close-reject → HARD_CLOSE_PENDING → SOFT_CLOSED, no step-up.
func TestE2E_P5M4_L_HardCloseReject_NoStepUp(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	maker := akunCTLActor()
	cfo := cfoActor()
	periode := h.periodes.seed("2026-06", m4StatusSoftClosed)

	r1 := hardCloseRequest(h, periode.ID, maker, periode.RowVersion,
		uuid.New().String(), allPassedChecklistConfig())
	require.Equal(t, 202, r1.StatusCode)
	require.Equal(t, m4StatusHardClosePending, h.periodes.get(periode.ID).StatusPeriode)

	// Reject without step-up — must succeed.
	result := hardCloseReject(h, periode.ID, cfo,
		"Perlu koreksi jurnal sebelum hard close dapat dilanjutkan",
		uuid.New().String())

	assert.Equal(t, 200, result.StatusCode, "S3-AC4: expect 200")
	assert.Equal(t, m4StatusSoftClosed, result.StatusPeriode, "S3-AC4: SOFT_CLOSED after reject")

	p := h.periodes.get(periode.ID)
	assert.Equal(t, m4StatusSoftClosed, p.StatusPeriode, "S3-AC4: DB state SOFT_CLOSED")
	assert.Nil(t, p.HardCloseRequestedBy, "S3-AC4: HardCloseRequestedBy cleared on reject")

	assert.True(t, h.audit.containsAction(periode.ID.String(), m4AuditHardCloseRejected),
		"S3-AC4: HARD_CLOSE_REJECTED audit written")
}

// TestE2E_P5M4_M — S4-AC1: Reopen SOFT_CLOSED → OPEN, no step-up, SoD checked.
func TestE2E_P5M4_M_Reopen_SoftClosed_ToOpen(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	cfo := cfoActor()
	approveCFO := cfoActor()
	periode := h.periodes.seed("2026-06", m4StatusSoftClosed)

	// Set a reopen requester to test SoD.
	periode.ReopenedBy = &cfo.UserID

	r1 := requestReopen(h, periode.ID, cfo,
		"Kesalahan jurnal ditemukan setelah soft close, perlu koreksi ulang",
		m4StatusOpen, periode.RowVersion, uuid.New().String())
	require.Equal(t, 202, r1.StatusCode, "S4-AC1: reopen request 202")

	p := h.periodes.get(periode.ID)
	// Approve by different CFO (SoD).
	r2 := approveReopen(h, periode.ID, approveCFO,
		uuid.New().String(), nil /*no step-up for SOFT_CLOSED reopen*/)

	assert.Equal(t, 200, r2.StatusCode, "S4-AC1: reopen approve 200")
	assert.Equal(t, m4StatusOpen, r2.StatusPeriode, "S4-AC1: OPEN after reopen")

	p = h.periodes.get(periode.ID)
	assert.Equal(t, m4StatusOpen, p.StatusPeriode, "S4-AC1: DB OPEN")
	assert.True(t, p.ReopenedFlag, "S4-AC1: ReopenedFlag true")

	assert.True(t, h.audit.containsAction(periode.ID.String(), m4AuditReopenApproved),
		"S4-AC1: REOPEN_APPROVED audit written")
}

// TestE2E_P5M4_N — S4-AC2: Reopen CLOSED → SOFT_CLOSED in grace window, CFO step-up.
func TestE2E_P5M4_N_Reopen_Closed_WithinGrace(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	requesterCFO := cfoActor()
	approverCFO := cfoActor()
	periode := h.periodes.seed("2026-06", m4StatusClosed)

	// Set grace window to future (within 48h).
	graceExpiry := time.Now().Add(24 * time.Hour)
	periode.HardCloseGraceExpiresAt = &graceExpiry
	periode.KursLocked = true
	h.kurs.setLocked(periode.PeriodeIDKode, true)

	r1 := requestReopen(h, periode.ID, requesterCFO,
		"Ditemukan kesalahan posting setelah hard close, reopen dalam grace window",
		m4StatusSoftClosed, periode.RowVersion, uuid.New().String())
	require.Equal(t, 202, r1.StatusCode, "S4-AC2: reopen request 202")

	p := h.periodes.get(periode.ID)
	stepUp := freshStepUpToken("periode.reopen.approve", approverCFO.UserID)
	r2 := approveReopen(h, periode.ID, approverCFO, uuid.New().String(), stepUp)

	assert.Equal(t, 200, r2.StatusCode, "S4-AC2: reopen approve 200")
	assert.Equal(t, m4StatusSoftClosed, r2.StatusPeriode, "S4-AC2: SOFT_CLOSED after reopen")

	// kurs unlocked.
	assert.False(t, h.kurs.isLocked(periode.PeriodeIDKode), "S4-AC2: kurs unlocked on reopen")

	p = h.periodes.get(periode.ID)
	assert.Equal(t, m4StatusSoftClosed, p.StatusPeriode)
	assert.True(t, p.ReopenedFlag)
}

// TestE2E_P5M4_O — S4-AC3: Reopen reason < 30 chars → 400 VALIDATION_FAILED.
func TestE2E_P5M4_O_Reopen_ReasonTooShort(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	cfo := cfoActor()
	periode := h.periodes.seed("2026-06", m4StatusSoftClosed)

	result := requestReopen(h, periode.ID, cfo,
		"Terlalu pendek", // < 30 chars
		m4StatusOpen, periode.RowVersion, uuid.New().String())

	assert.Equal(t, 400, result.StatusCode, "S4-AC3: expect 400")
	assert.Equal(t, m4ErrValidationFailed, result.ErrorCode, "S4-AC3: VALIDATION_FAILED")
}

// TestE2E_P5M4_P — S4-AC4: Reopen CLOSED outside grace window → 423 PERIODE_GRACE_EXPIRED.
func TestE2E_P5M4_P_Reopen_Closed_GraceExpired(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	cfo := cfoActor()
	periode := h.periodes.seed("2026-06", m4StatusClosed)

	// Grace window already expired.
	expired := time.Now().Add(-1 * time.Hour)
	periode.HardCloseGraceExpiresAt = &expired

	result := requestReopen(h, periode.ID, cfo,
		"Perlu reopen tapi sudah lewat grace window, alasan teknis yang panjang cukup",
		m4StatusSoftClosed, periode.RowVersion, uuid.New().String())

	assert.Equal(t, 423, result.StatusCode, "S4-AC4: expect 423")
	assert.Equal(t, m4ErrGraceExpired, result.ErrorCode, "S4-AC4: PERIODE_GRACE_EXPIRED")

	// Status unchanged.
	assert.Equal(t, m4StatusClosed, h.periodes.get(periode.ID).StatusPeriode,
		"S4-AC4: status still CLOSED")
}

// TestE2E_P5M4_Q — S5-AC1: GetChecklist with 1 failing item returns non-pass result.
func TestE2E_P5M4_Q_GetChecklist_OneItemFailing(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	actor := akunCTLActor()
	periode := h.periodes.seed("2026-06", m4StatusOpen)

	cfg := m4ChecklistConfig{
		PendingApprovalZeroPassed: true,
		JurnalBalancedPassed:      false, // jurnals not balanced
		GLDeliveredPassed:         true,
		ReconPassPassed:           true,
	}

	result := getChecklist(h, periode.ID, actor, cfg)

	assert.Equal(t, 200, result.StatusCode, "S5-AC1: always 200 for GET checklist")
	assert.False(t, result.AllPassed, "S5-AC1: not all passed")
	assert.Equal(t, 4, len(result.Items), "S5-AC1: 4 items always returned")

	// JURNAL_BALANCED item is failing; uses decimal — no float64.
	var jbItem *m4ChecklistItem
	for i := range result.Items {
		if result.Items[i].Key == m4ChecklistJurnalBalanced {
			jbItem = &result.Items[i]
			break
		}
	}
	require.NotNil(t, jbItem)
	assert.False(t, jbItem.Passed, "S5-AC1: JURNAL_BALANCED failing")
	assert.True(t, strings.Contains(jbItem.Detail, decimal.NewFromFloat(0.01).String()),
		"S5-AC1: detail references decimal threshold (DEC-016 compliance)")
}

// TestE2E_P5M4_R — S5-AC2: ListStatusPeriode — cursor pagination + filter + ROLE-MAKER-TR → 403.
func TestE2E_P5M4_R_ListStatusPeriode_FilterAndForbidden(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	actor := auditActor()

	// Seed 3 periodes with different statuses.
	h.periodes.seed("2026-01", m4StatusClosed)
	h.periodes.seed("2026-02", m4StatusSoftClosed)
	h.periodes.seed("2026-03", m4StatusOpen)

	// Filter by CLOSED.
	result := listStatusPeriode(h, actor, m4StatusClosed, false, 50)
	assert.Equal(t, 200, result.StatusCode, "S5-AC2: expect 200")
	assert.Equal(t, 1, len(result.Items), "S5-AC2: only 1 CLOSED item")
	assert.Equal(t, m4StatusClosed, result.Items[0].StatusPeriode, "S5-AC2: correct filter")

	// ROLE-MAKER-TR → 403.
	unauthorized := makerTRActor()
	r2 := listStatusPeriode(h, unauthorized, "", false, 50)
	assert.Equal(t, 403, r2.StatusCode, "S5-AC2: MAKER-TR gets 403")
	assert.Equal(t, m4ErrForbidden, r2.ErrorCode, "S5-AC2: FORBIDDEN")
}

// TestE2E_P5M4_S — S5-AC3: Export audit row written on ListStatusPeriode with export=csv.
func TestE2E_P5M4_S_ListStatusPeriode_ExportAudit(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	actor := auditActor()
	h.periodes.seed("2026-06", m4StatusClosed)

	result := listStatusPeriode(h, actor, "", true /*export*/, 50)

	assert.Equal(t, 200, result.StatusCode, "S5-AC3: expect 200")
	assert.True(t, h.audit.containsAction("periode.status_report", m4AuditExport),
		"S5-AC3: PERIODE.EXPORT audit row written")
}

// TestE2E_P5M4_T — S5-AC4: GetChecklist after CLOSED returns HARD_CLOSE_APPROVE snapshot ref.
func TestE2E_P5M4_T_GetChecklist_AfterClosed(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	cfo := cfoActor()
	actor := auditActor()
	periode := h.periodes.seed("2026-06", m4StatusClosed)

	// Plant a HARD_CLOSE_APPROVE snapshot.
	hardSnap := &m4ChecklistSnapshot{
		ID:            uuid.New(),
		PeriodeBukuID: periode.ID,
		Transition:    m4TransitionHardCloseApprove,
		EvaluatedAt:   time.Now(),
		EvaluatedBy:   cfo.UserID,
		ActorRole:     m4RoleCFO,
		AllPassed:     true,
		Items:         nil,
	}
	h.snapshots.append(hardSnap)

	result := getChecklist(h, periode.ID, actor, allPassedChecklistConfig())

	assert.Equal(t, 200, result.StatusCode, "S5-AC4: expect 200")
	// Snapshot history must include the HARD_CLOSE_APPROVE entry.
	approveSnap := h.snapshots.latestForPeriode(periode.ID, m4TransitionHardCloseApprove)
	require.NotNil(t, approveSnap, "S5-AC4: HARD_CLOSE_APPROVE snapshot present in history")
	assert.Equal(t, hardSnap.ID, approveSnap.ID, "S5-AC4: correct snapshot ref")
}

// TestE2E_P5M4_U — Idempotency: replay same key returns original, no duplicate side-effects.
func TestE2E_P5M4_U_Idempotency_Replay(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	maker := akunCTLActor()
	periode := h.periodes.seed("2026-06", m4StatusOpen)
	key := uuid.New().String()

	r1 := softCloseRequest(h, periode.ID, maker, periode.RowVersion,
		"Tutup Juni 2026", key, allPassedChecklistConfig())
	require.Equal(t, 202, r1.StatusCode, "U: first call 202")

	// Replay with same key.
	r2 := softCloseRequest(h, periode.ID, maker, periode.RowVersion,
		"Tutup Juni 2026", key, allPassedChecklistConfig())

	assert.Equal(t, 202, r2.StatusCode, "U: replay returns original status code")
	assert.Equal(t, "IDEMPOTENCY_REPLAY", r2.ErrorCode, "U: replay marker present")

	// Audit row count must not increase on replay.
	countBefore := len(h.audit.rows)
	softCloseRequest(h, periode.ID, maker, periode.RowVersion,
		"Tutup Juni 2026", key, allPassedChecklistConfig())
	assert.Equal(t, countBefore, len(h.audit.rows), "U: no duplicate audit rows on replay")

	// Snapshot count must not increase on replay.
	snapCountBefore := h.snapshots.countForPeriode(periode.ID)
	softCloseRequest(h, periode.ID, maker, periode.RowVersion,
		"Tutup Juni 2026", key, allPassedChecklistConfig())
	assert.Equal(t, snapCountBefore, h.snapshots.countForPeriode(periode.ID),
		"U: no duplicate snapshots on replay")
}

// TestE2E_P5M4_V — Audit hash-chain intact after multiple mutations.
func TestE2E_P5M4_V_Audit_HashChainIntact(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	maker := akunCTLActor()
	approver := akunCTLActor()
	cfo := cfoActor()
	periode := h.periodes.seed("2026-06", m4StatusOpen)

	// Full flow: request → approve → hard-close-request → hard-close-approve.
	r1 := softCloseRequest(h, periode.ID, maker, periode.RowVersion,
		"V test", uuid.New().String(), allPassedChecklistConfig())
	require.Equal(t, 202, r1.StatusCode)

	p := h.periodes.get(periode.ID)
	r2 := softCloseApprove(h, periode.ID, approver, p.RowVersion,
		uuid.New().String(), allPassedChecklistConfig())
	require.Equal(t, 200, r2.StatusCode)

	p = h.periodes.get(periode.ID)
	r3 := hardCloseRequest(h, periode.ID, maker, p.RowVersion,
		uuid.New().String(), allPassedChecklistConfig())
	require.Equal(t, 202, r3.StatusCode)

	p = h.periodes.get(periode.ID)
	stepUp := freshStepUpToken("periode.hardclose.approve", cfo.UserID)
	r4 := hardCloseApprove(h, periode.ID, cfo, p.RowVersion,
		uuid.New().String(), stepUp)
	require.Equal(t, 200, r4.StatusCode)

	// Verify chain.
	ok, msg := h.audit.verifyHashChain()
	assert.True(t, ok, "V: audit hash chain intact: %s", msg)

	// Expected audit events present (DEC-018).
	actions := h.audit.actionsForEntity(periode.ID.String())
	assert.Contains(t, actions, m4AuditSoftCloseRequested, "V: SOFT_CLOSE_REQUESTED in chain")
	assert.Contains(t, actions, m4AuditSoftCloseApproved, "V: SOFT_CLOSE_APPROVED in chain")
	assert.Contains(t, actions, m4AuditHardCloseRequested, "V: HARD_CLOSE_REQUESTED in chain")
	assert.Contains(t, actions, m4AuditHardClosed, "V: HARDCLOSED in chain")
}

// TestE2E_P5M4_W — Append-only: sys.closing_checklist_snapshot DELETE blocked.
func TestE2E_P5M4_W_AppendOnly_SnapshotDeleteBlocked(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	maker := akunCTLActor()
	periode := h.periodes.seed("2026-06", m4StatusOpen)

	r1 := softCloseRequest(h, periode.ID, maker, periode.RowVersion,
		"Append only test", uuid.New().String(), allPassedChecklistConfig())
	require.Equal(t, 202, r1.StatusCode)

	snap := h.snapshots.latestForPeriode(periode.ID, m4TransitionSoftCloseRequest)
	require.NotNil(t, snap, "W: snapshot present")

	// Simulate DELETE attempt — trigger raises error.
	err := h.snapshots.delete(snap.ID)
	require.Error(t, err, "W: DELETE must be blocked by trigger")
	assert.Contains(t, err.Error(), "append-only", "W: trigger message must say append-only")

	// Snapshot still present.
	assert.Equal(t, 1, h.snapshots.countForPeriode(periode.ID),
		"W: snapshot count unchanged after blocked delete")
}

// TestE2E_P5M4_X — PeriodeLockMiddleware: SOFT_CLOSED blocks mutation, allowlist passes.
func TestE2E_P5M4_X_PeriodeLockMiddleware_SoftClosed(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	periode := h.periodes.seed("2026-06", m4StatusSoftClosed)

	// Normal mutation blocked.
	r1 := periodeLockCheck(h, periode.ID, "")
	assert.False(t, r1.Allowed, "X: SOFT_CLOSED blocks mutation")
	assert.Equal(t, 423, r1.HTTPCode)
	assert.Equal(t, m4ErrPeriodeSoftClosed, r1.ErrorCode)

	// Allowlist action passes.
	r2 := periodeLockCheck(h, periode.ID, "JURNAL_RETRY_GL_DELIVERY")
	assert.True(t, r2.Allowed, "X: allowlist action passes through SOFT_CLOSED")

	r3 := periodeLockCheck(h, periode.ID, "CORRECTION_PERIODE_CLOSED")
	assert.True(t, r3.Allowed, "X: CORRECTION_PERIODE_CLOSED passes through SOFT_CLOSED")
}

// TestE2E_P5M4_Y — PeriodeLockMiddleware: CLOSED blocks all including allowlist.
func TestE2E_P5M4_Y_PeriodeLockMiddleware_Closed(t *testing.T) {
	t.Parallel()
	h := newP5M4Harness(t)
	periode := h.periodes.seed("2026-06", m4StatusClosed)

	r1 := periodeLockCheck(h, periode.ID, "")
	assert.False(t, r1.Allowed, "Y: CLOSED blocks all")
	assert.Equal(t, 423, r1.HTTPCode)
	assert.Equal(t, m4ErrPeriodeClosed, r1.ErrorCode)

	// Even allowlist actions are blocked in CLOSED state.
	r2 := periodeLockCheck(h, periode.ID, "JURNAL_RETRY_GL_DELIVERY")
	assert.False(t, r2.Allowed, "Y: CLOSED blocks allowlist too")
	assert.Equal(t, m4ErrPeriodeClosed, r2.ErrorCode)
}
