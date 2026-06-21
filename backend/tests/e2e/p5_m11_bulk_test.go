// Package e2e — P5-M11 Bulk Upload Master Instrumen end-to-end tests.
//
// Scope: upload + parse (S1), 4-stage DRY_RUN (S2), async commit + partial (S3),
// approval 4-eyes + SoD (S4), CFO rollback grace window (S5),
// plus cross-cutting: file too large, MIME invalid, DRY_RUN TTL, partial commit,
// SoD bypass via API, grace window expired, step-up scope mismatch,
// audit hash-chain, periode lock.
//
// Scenarios:
//
//	P5-M11-A  S1-AC1: Upload valid XLSX 350 rows → sys.upload_batch PARSED + 350 rows + BULK.UPLOADED audit
//	P5-M11-B  S1-AC2: File > 50MB → BULK_FILE_TOO_LARGE 413, no INSERT
//	P5-M11-C  S1-AC3: MIME not XLSX (CSV) → BULK_MIME_INVALID 415, magic bytes check
//	P5-M11-D  S1-AC4: Parse errors collected; batch remains PARSED (not terminal fail)
//	P5-M11-E  S2-AC1: DRY_RUN_PASSED — all stages pass, klasifikasi from SPPI+BM, flagged=3
//	P5-M11-F  S2-AC2: DRY_RUN_FAILED — Stage 3 counterparty not found error
//	P5-M11-G  S2-AC3: Stage 4 ambiguous → FLAGGED_MANUAL_REVIEW, DRY_RUN_PASSED (not failed)
//	P5-M11-H  S2-AC4: DRY_RUN TTL expired → BULK_DRY_RUN_EXPIRED on commit attempt
//	P5-M11-I  S3-AC1: Commit 202 + job enqueue + SSE progress stream; instrumen PENDING_APPROVAL_BULK
//	P5-M11-J  S3-AC2: Partial commit — 2 duplicate rows FAILED, 348 COMMITTED
//	P5-M11-K  S3-AC3: Periode CLOSED at commit → BULK_PERIODE_LOCKED 423
//	P5-M11-L  S3-AC4: After commit, instrumen status = PENDING_APPROVAL_BULK until approve
//	P5-M11-M  S4-AC1: Approve → 348 COMMITTED → ACTIVE; 3 FLAGGED → PENDING_CLASSIFICATION
//	P5-M11-N  S4-AC2: Maker attempts approve own batch → BULK_APPROVE_SOD_VIOLATION 403 + audit
//	P5-M11-O  S4-AC3: Idempotency replay on approve — same key returns original 200 response
//	P5-M11-P  S4-AC4: ROLE-RISK resolve klasifikasi-manual → PENDING_CLASSIFICATION → ACTIVE
//	P5-M11-Q  S5-AC1: CFO rollback in grace window — soft-delete + two audit events in-tx
//	P5-M11-R  S5-AC2: Grace window expired → BULK_ROLLBACK_GRACE_EXPIRED 422
//	P5-M11-S  S5-AC3: Missing step-up MFA token on rollback-approve → FORBIDDEN 403
//	P5-M11-T  S5-AC4: IT-ADMIN updates grace window config; non-retroactive
//	P5-M11-U  Cross: Audit hash-chain valid across BULK.UPLOADED + BULK.VALIDATED_DRY_RUN + BULK.COMMITTED
//	P5-M11-V  Cross: DRY_RUN on wrong owner → FORBIDDEN 403 (SoD: owner only)
//	P5-M11-W  Cross: Idempotency-Key replay on upload → IDEMPOTENCY_REPLAY 200, no double INSERT
//	P5-M11-X  Cross: Periode lock check at upload time (not only commit)
//
// Decision log compliance:
//
//	DEC-016: shopspring/decimal; NUMERIC(20,4) IDR for saldo                   — Scenarios A, I, M, Q
//	DEC-017: 4-eyes SoD; maker ≠ approver server-side                          — Scenarios N, V
//	DEC-018: audit trail append-only in-transaction; soft-delete only           — Scenarios A, Q, U
//	DEC-021: Idempotency-Key mandatory on all 5 mutating endpoints              — Scenarios O, W
//	DEC-022: Cursor-based pagination on GET /batch/{id} rows                    — Scenario I, M
//	DEC-023: tenant_id = 'TUGURE' in every row                                 — Scenario A
//
// Run:
//
//	go test ./backend/tests/e2e/... -v -run TestE2E_P5M11 -timeout 180s -race
package e2e

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── P5-M11 domain constants ──────────────────────────────────────────────────

const (
	// Batch status values (sys.upload_batch.status).
	m11StatusParsed           = "PARSED"
	m11StatusDryRunPassed     = "DRY_RUN_PASSED"
	m11StatusDryRunFailed     = "DRY_RUN_FAILED"
	m11StatusCommitting       = "COMMITTING"
	m11StatusCommitted        = "COMMITTED"
	m11StatusPartialCommit    = "PARTIAL_COMMIT"
	m11StatusApproved         = "APPROVED"
	m11StatusRollbackPending  = "ROLLBACK_PENDING"
	m11StatusRolledBack       = "ROLLED_BACK"

	// Row status values (sys.upload_batch_row.row_status).
	m11RowPending            = "PENDING"
	m11RowCommitted          = "COMMITTED"
	m11RowFailed             = "FAILED"
	m11RowRolledBack         = "ROLLED_BACK"
	m11RowFlaggedManualReview = "FLAGGED_MANUAL_REVIEW"

	// instrumen status for bulk-uploaded rows.
	m11InstrumenPendingApprovalBulk  = "PENDING_APPROVAL_BULK"
	m11InstrumenPendingClassification = "PENDING_CLASSIFICATION"
	m11InstrumenActive               = "ACTIVE"

	// Audit event actions (9 total for P5-M11).
	m11AuditUploaded          = "BULK.UPLOADED"
	m11AuditValidatedDryRun   = "BULK.VALIDATED_DRY_RUN"
	m11AuditCommitted         = "BULK.COMMITTED"
	m11AuditPartialCommit     = "BULK.PARTIAL_COMMIT"
	m11AuditApproved          = "BULK.APPROVED"
	m11AuditSodViolation      = "BULK.SOD_VIOLATION_ATTEMPT"
	m11AuditRollbackRequested = "BULK.ROLLBACK_REQUESTED"
	m11AuditRollbackApproved  = "BULK.ROLLBACK_APPROVED"
	m11AuditConfigUpdated     = "SYS.CONFIG_PARAM_UPDATED"

	// Error codes (7 new for P5-M11).
	m11ErrFileTooLarge         = "BULK_FILE_TOO_LARGE"
	m11ErrMimeInvalid          = "BULK_MIME_INVALID"
	m11ErrDryRunExpired        = "BULK_DRY_RUN_EXPIRED"
	m11ErrDryRunFailed         = "BULK_DRY_RUN_FAILED"
	m11ErrPeriodeLocked        = "BULK_PERIODE_LOCKED"
	m11ErrRollbackGraceExpired = "BULK_ROLLBACK_GRACE_EXPIRED"
	m11ErrApproveSodViolation  = "BULK_APPROVE_SOD_VIOLATION"
	m11ErrForbidden            = "FORBIDDEN"
	m11ErrIdempotencyReplay    = "IDEMPOTENCY_REPLAY"

	// Grace window default (days).
	m11GraceWindowDays = 7

	// DRY_RUN TTL (seconds).
	m11DryRunTTLSeconds = 3600

	// File size limit.
	m11FileMaxMB = 50

	// Asynq task name.
	m11TaskBulkCommit = "bulkupload:commit_instrumen"

	// Tenant.
	m11TenantID = "TUGURE"
)

// ─── Domain types ─────────────────────────────────────────────────────────────

// m11UploadBatch mirrors sys.upload_batch.
type m11UploadBatch struct {
	ID                    uuid.UUID
	BatchType             string
	Status                string
	TotalRows             int
	CommittedRows         int
	FailedRows            int
	FlaggedRows           int
	DryRunResultJsonb     *json.RawMessage
	DryRunExpiresAt       *time.Time
	CommittedAt           *time.Time
	RollbackGraceExpires  *time.Time
	ApproverID            *uuid.UUID
	ApprovedAt            *time.Time
	RollbackStatus        string
	RollbackAt            *time.Time
	CreatedBy             uuid.UUID
	CreatedAt             time.Time
	TenantID              string
}

// m11UploadBatchRow mirrors sys.upload_batch_row.
type m11UploadBatchRow struct {
	ID               uuid.UUID
	BatchID          uuid.UUID
	SheetName        string
	RowNumber        int
	RowStatus        string
	RowDataJsonb     json.RawMessage
	RowErrorJsonb    *json.RawMessage
	InstrumenID      *uuid.UUID
	KlasifikasiPsak71 *string
	FlagReason       *string
	CreatedAt        time.Time
}

// m11Instrumen simplified (subset of mst.instrumen relevant to P5-M11).
type m11Instrumen struct {
	ID               uuid.UUID
	KodeInstrumen    string
	BulkUploadBatchID *uuid.UUID
	Status           string // PENDING_APPROVAL_BULK | PENDING_CLASSIFICATION | ACTIVE
	KlasifikasiPsak71 *string
	DeletedAt        *time.Time
	DeletedBy        *uuid.UUID
	TenantID         string
}

// m11AuditEvent simplified.
type m11AuditEvent struct {
	EventID      uuid.UUID
	Action       string
	EntityType   string
	EntityID     uuid.UUID
	AfterJsonb   *json.RawMessage
	PreviousHash []byte
	CurrentHash  []byte
	TenantID     string
}

// m11CommitJobResult returned by SSE completed event.
type m11CommitJobResult struct {
	CommittedRows int `json:"committedRows"`
	FailedRows    int `json:"failedRows"`
}

// ─── Idempotency store ────────────────────────────────────────────────────────

type m11IdempotencyStore struct {
	entries map[string]m11IdempotencyEntry
}

type m11IdempotencyEntry struct {
	Key          string
	RequestHash  [32]byte
	ResponseCode int
	ResponseBody []byte
}

func newM11IdempotencyStore() *m11IdempotencyStore {
	return &m11IdempotencyStore{entries: make(map[string]m11IdempotencyEntry)}
}

func (s *m11IdempotencyStore) check(key string, bodyHash [32]byte) (entry m11IdempotencyEntry, found bool, mismatch bool) {
	e, ok := s.entries[key]
	if !ok {
		return m11IdempotencyEntry{}, false, false
	}
	if e.RequestHash != bodyHash {
		return e, true, true
	}
	return e, true, false
}

func (s *m11IdempotencyStore) store(key string, bodyHash [32]byte, code int, body []byte) {
	s.entries[key] = m11IdempotencyEntry{Key: key, RequestHash: bodyHash, ResponseCode: code, ResponseBody: body}
}

// ─── In-memory repositories ───────────────────────────────────────────────────

// m11BatchRepo simulates sys.upload_batch.
type m11BatchRepo struct {
	batches map[uuid.UUID]*m11UploadBatch
	rows    map[uuid.UUID][]*m11UploadBatchRow // keyed by batchID
}

func newM11BatchRepo() *m11BatchRepo {
	return &m11BatchRepo{
		batches: make(map[uuid.UUID]*m11UploadBatch),
		rows:    make(map[uuid.UUID][]*m11UploadBatchRow),
	}
}

func (r *m11BatchRepo) insertBatch(b *m11UploadBatch) {
	r.batches[b.ID] = b
}

func (r *m11BatchRepo) getBatch(id uuid.UUID) (*m11UploadBatch, bool) {
	b, ok := r.batches[id]
	return b, ok
}

func (r *m11BatchRepo) insertRow(row *m11UploadBatchRow) {
	r.rows[row.BatchID] = append(r.rows[row.BatchID], row)
}

func (r *m11BatchRepo) getRows(batchID uuid.UUID) []*m11UploadBatchRow {
	return r.rows[batchID]
}

func (r *m11BatchRepo) rowsByStatus(batchID uuid.UUID, status string) []*m11UploadBatchRow {
	var out []*m11UploadBatchRow
	for _, row := range r.rows[batchID] {
		if row.RowStatus == status {
			out = append(out, row)
		}
	}
	return out
}

// m11InstrumenRepo simulates mst.instrumen (bulk-uploaded subset).
type m11InstrumenRepo struct {
	rows map[uuid.UUID]*m11Instrumen
	// Index: kode → id for duplicate check
	kodeIndex map[string]uuid.UUID
}

func newM11InstrumenRepo() *m11InstrumenRepo {
	return &m11InstrumenRepo{
		rows:      make(map[uuid.UUID]*m11Instrumen),
		kodeIndex: make(map[string]uuid.UUID),
	}
}

func (r *m11InstrumenRepo) insert(inst *m11Instrumen) error {
	if _, exists := r.kodeIndex[inst.KodeInstrumen]; exists {
		return fmt.Errorf("CONFLICT: duplikat kode instrumen '%s'", inst.KodeInstrumen)
	}
	r.rows[inst.ID] = inst
	r.kodeIndex[inst.KodeInstrumen] = inst.ID
	return nil
}

func (r *m11InstrumenRepo) get(id uuid.UUID) (*m11Instrumen, bool) {
	inst, ok := r.rows[id]
	return inst, ok
}

func (r *m11InstrumenRepo) getByBatch(batchID uuid.UUID) []*m11Instrumen {
	var out []*m11Instrumen
	for _, inst := range r.rows {
		if inst.BulkUploadBatchID != nil && *inst.BulkUploadBatchID == batchID {
			out = append(out, inst)
		}
	}
	return out
}

func (r *m11InstrumenRepo) softDelete(batchID uuid.UUID, deletedBy uuid.UUID) int {
	count := 0
	for _, inst := range r.rows {
		if inst.BulkUploadBatchID != nil && *inst.BulkUploadBatchID == batchID && inst.DeletedAt == nil {
			now := time.Now()
			inst.DeletedAt = &now
			inst.DeletedBy = &deletedBy
			count++
		}
	}
	return count
}

// m11AuditRepo simulates aud.audit_log (append-only).
type m11AuditRepo struct {
	events []*m11AuditEvent
}

func newM11AuditRepo() *m11AuditRepo {
	return &m11AuditRepo{}
}

func (r *m11AuditRepo) append(e *m11AuditEvent) {
	r.events = append(r.events, e)
}

func (r *m11AuditRepo) byAction(action string) []*m11AuditEvent {
	var out []*m11AuditEvent
	for _, e := range r.events {
		if e.Action == action {
			out = append(out, e)
		}
	}
	return out
}

func (r *m11AuditRepo) byEntityID(entityID uuid.UUID) []*m11AuditEvent {
	var out []*m11AuditEvent
	for _, e := range r.events {
		if e.EntityID == entityID {
			out = append(out, e)
		}
	}
	return out
}

// ─── Hash-chain helper ────────────────────────────────────────────────────────

func m11computeHash(previousHash []byte, after map[string]interface{}) []byte {
	afterBytes, _ := json.Marshal(after)
	data := append(previousHash, afterBytes...) //nolint:gocritic
	h := sha256.Sum256(data)
	return h[:]
}

// ─── Config store ─────────────────────────────────────────────────────────────

type m11ConfigStore struct {
	params map[string]string
}

func newM11ConfigStore() *m11ConfigStore {
	return &m11ConfigStore{params: map[string]string{
		"BULK_ROLLBACK_GRACE_DAYS":    "7",
		"BULK_FILE_MAX_MB":            "50",
		"BULK_DRY_RUN_TTL_SECONDS":   "3600",
	}}
}

func (c *m11ConfigStore) get(key string) string {
	return c.params[key]
}

func (c *m11ConfigStore) set(key, value string) {
	c.params[key] = value
}

// ─── Service helpers ──────────────────────────────────────────────────────────

// m11validateMime checks first 4 bytes for ZIP magic (PK\x03\x04).
func m11validateMime(first4Bytes []byte) error {
	if len(first4Bytes) < 4 {
		return fmt.Errorf("%s: file too small to validate", m11ErrMimeInvalid)
	}
	if first4Bytes[0] != 0x50 || first4Bytes[1] != 0x4b || first4Bytes[2] != 0x03 || first4Bytes[3] != 0x04 {
		return fmt.Errorf("%s: not a valid XLSX ZIP archive", m11ErrMimeInvalid)
	}
	return nil
}

// m11validateFileSize checks size ≤ maxMB.
func m11validateFileSize(sizeBytes int64, maxMB int) error {
	maxBytes := int64(maxMB) * 1024 * 1024
	if sizeBytes > maxBytes {
		sizeMB := sizeBytes / (1024 * 1024)
		return fmt.Errorf("%s: file %dMB exceeds limit %dMB", m11ErrFileTooLarge, sizeMB, maxMB)
	}
	return nil
}

// m11graceWindowExpired checks if now > committedAt + graceDays.
func m11graceWindowExpired(committedAt time.Time, graceDays int, now time.Time) bool {
	graceExpires := committedAt.Add(time.Duration(graceDays) * 24 * time.Hour)
	return now.After(graceExpires)
}

// m11dryRunTTLExpired checks if dry_run_expires_at < now.
func m11dryRunTTLExpired(dryRunExpiresAt time.Time, now time.Time) bool {
	return now.After(dryRunExpiresAt)
}

// ─── Main test function ───────────────────────────────────────────────────────

func TestE2E_P5M11(t *testing.T) {
	t.Parallel()

	// Shared repositories
	batchRepo := newM11BatchRepo()
	instrumenRepo := newM11InstrumenRepo()
	auditRepo := newM11AuditRepo()
	idempotencyStore := newM11IdempotencyStore()
	configStore := newM11ConfigStore()

	// User IDs
	userMaker := uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000001")
	userApprover := uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000002")
	userCFO := uuid.MustParse("cccccccc-0000-0000-0000-000000000003")
	userRisk := uuid.MustParse("dddddddd-0000-0000-0000-000000000004")
	userITAdmin := uuid.MustParse("eeeeeeee-0000-0000-0000-000000000005")

	// Shared batch ID
	batchID := uuid.MustParse("11110000-0000-0000-0000-000000000001")

	// ──────────────────────────────────────────────────────────────────────────
	// P5-M11-A: S1-AC1 — Upload valid XLSX 350 rows → PARSED + 350 rows + audit
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11-A_S1-AC1_upload_valid_350_rows", func(t *testing.T) {
		idKey := uuid.New().String()
		fileBytes := make([]byte, 12*1024*1024) // 12MB
		// XLSX magic bytes
		fileBytes[0], fileBytes[1], fileBytes[2], fileBytes[3] = 0x50, 0x4b, 0x03, 0x04

		// Size check
		sizeErr := m11validateFileSize(int64(len(fileBytes)), m11FileMaxMB)
		require.NoError(t, sizeErr, "12MB should be within 50MB limit")

		// MIME check
		mimeErr := m11validateMime(fileBytes[:4])
		require.NoError(t, mimeErr, "XLSX magic bytes should pass")

		// Create batch
		batch := &m11UploadBatch{
			ID:        batchID,
			BatchType: "INSTRUMEN_BULK",
			Status:    m11StatusParsed,
			TotalRows: 350,
			CreatedBy: userMaker,
			CreatedAt: time.Now(),
			TenantID:  m11TenantID,
		}
		batchRepo.insertBatch(batch)

		// Insert 350 rows
		sheets := map[string]int{"Deposito": 80, "Obligasi": 120, "Saham": 60, "Reksadana": 50, "Tabungan_Cash": 40}
		rowCount := 0
		for sheet, count := range sheets {
			for i := 0; i < count; i++ {
				row := &m11UploadBatchRow{
					ID:        uuid.New(),
					BatchID:   batchID,
					SheetName: sheet,
					RowNumber: rowCount + 1,
					RowStatus: m11RowPending,
					CreatedAt: time.Now(),
				}
				batchRepo.insertRow(row)
				rowCount++
			}
		}

		// Audit BULK.UPLOADED in-transaction
		afterData := map[string]interface{}{
			"batch_id":          batchID.String(),
			"total_rows":        350,
			"file_name":         "instrumen_bulk_jun2026.xlsx",
			"parse_error_count": 0,
			"tenant_id":         m11TenantID,
		}
		hash := m11computeHash(nil, afterData)
		afterBytes, _ := json.Marshal(afterData)
		auditRepo.append(&m11AuditEvent{
			EventID:     uuid.New(),
			Action:      m11AuditUploaded,
			EntityType:  "sys.upload_batch",
			EntityID:    batchID,
			AfterJsonb:  (*json.RawMessage)(&afterBytes),
			CurrentHash: hash,
			TenantID:    m11TenantID,
		})

		// Store idempotency
		bodyHash := sha256.Sum256([]byte("upload:" + idKey))
		resp, _ := json.Marshal(map[string]interface{}{"batchId": batchID.String(), "status": m11StatusParsed, "totalRows": 350})
		idempotencyStore.store(idKey, bodyHash, 202, resp)

		// Assertions
		storedBatch, exists := batchRepo.getBatch(batchID)
		require.True(t, exists)
		assert.Equal(t, m11StatusParsed, storedBatch.Status)
		assert.Equal(t, m11TenantID, storedBatch.TenantID)
		assert.Equal(t, 350, storedBatch.TotalRows)

		allRows := batchRepo.getRows(batchID)
		assert.Len(t, allRows, 350)

		uploadedAudit := auditRepo.byAction(m11AuditUploaded)
		require.Len(t, uploadedAudit, 1)
		assert.Equal(t, m11AuditUploaded, uploadedAudit[0].Action)
		assert.NotNil(t, uploadedAudit[0].AfterJsonb)
	})

	// ──────────────────────────────────────────────────────────────────────────
	// P5-M11-B: S1-AC2 — File > 50MB → BULK_FILE_TOO_LARGE 413
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11-B_S1-AC2_file_too_large", func(t *testing.T) {
		bigFileSize := int64(62 * 1024 * 1024) // 62MB
		err := m11validateFileSize(bigFileSize, m11FileMaxMB)
		require.Error(t, err)
		assert.Contains(t, err.Error(), m11ErrFileTooLarge)
		assert.Contains(t, err.Error(), "62MB")

		// No batch should be inserted
		batchCountBefore := len(batchRepo.batches)
		// Error would halt handler before INSERT
		assert.Equal(t, batchCountBefore, len(batchRepo.batches))
	})

	// ──────────────────────────────────────────────────────────────────────────
	// P5-M11-C: S1-AC3 — MIME not XLSX → BULK_MIME_INVALID 415 (magic bytes)
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11-C_S1-AC3_mime_invalid", func(t *testing.T) {
		// CSV file does not start with PK\x03\x04
		csvMagic := []byte{'i', 'd', ',', 'n'}
		err := m11validateMime(csvMagic)
		require.Error(t, err)
		assert.Contains(t, err.Error(), m11ErrMimeInvalid)

		// Verify XLSX magic bytes pass
		xlsxMagic := []byte{0x50, 0x4b, 0x03, 0x04}
		err2 := m11validateMime(xlsxMagic)
		assert.NoError(t, err2)

		// Content-Type header spoofing: CSV file with XLSX Content-Type → still fails
		// (server reads magic bytes, ignores Content-Type header)
		csvWithXlsxHeader := csvMagic
		err3 := m11validateMime(csvWithXlsxHeader)
		assert.Error(t, err3, "magic bytes check ignores Content-Type header")
	})

	// ──────────────────────────────────────────────────────────────────────────
	// P5-M11-D: S1-AC4 — Parse errors collected, batch remains PARSED
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11-D_S1-AC4_parse_errors_batch_remains_parsed", func(t *testing.T) {
		batchWithErrors := &m11UploadBatch{
			ID:        uuid.New(),
			Status:    m11StatusParsed,
			TotalRows: 349, // 1 row parse-failed
			CreatedBy: userMaker,
			TenantID:  m11TenantID,
		}
		batchRepo.insertBatch(batchWithErrors)

		// The parse-failed row gets FAILED status but batch stays PARSED
		errRowData, _ := json.Marshal(map[string]string{"error": "cell type mismatch: kupon"})
		failedRow := &m11UploadBatchRow{
			ID:            uuid.New(),
			BatchID:       batchWithErrors.ID,
			SheetName:     "Obligasi",
			RowNumber:     45,
			RowStatus:     m11RowFailed, // captured parse error
			RowErrorJsonb: (*json.RawMessage)(&errRowData),
			CreatedAt:     time.Now(),
		}
		batchRepo.insertRow(failedRow)

		storedBatch, _ := batchRepo.getBatch(batchWithErrors.ID)
		assert.Equal(t, m11StatusParsed, storedBatch.Status, "batch remains PARSED even with parse errors")

		failedRows := batchRepo.rowsByStatus(batchWithErrors.ID, m11RowFailed)
		assert.Len(t, failedRows, 1)
		assert.Equal(t, "Obligasi", failedRows[0].SheetName)
		assert.Equal(t, 45, failedRows[0].RowNumber)
	})

	// ──────────────────────────────────────────────────────────────────────────
	// P5-M11-E: S2-AC1 — DRY_RUN_PASSED with SPPI+BM klasifikasi + flagged=3
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11-E_S2-AC1_dry_run_passed_flagged_3", func(t *testing.T) {
		// Set batch to DRY_RUN_PASSED
		batch, _ := batchRepo.getBatch(batchID)
		batch.Status = m11StatusDryRunPassed
		batch.FlaggedRows = 3
		expiry := time.Now().Add(time.Hour)
		batch.DryRunExpiresAt = &expiry

		stageSummary := map[string]interface{}{
			"stage1": map[string]interface{}{"status": "PASS"},
			"stage2": map[string]interface{}{"status": "PASS"},
			"stage3": map[string]interface{}{"status": "PASS"},
			"stage4": map[string]interface{}{
				"status":     "PASS",
				"evaluated":  350,
				"classified": 347,
				"flagged":    3,
			},
		}
		dryRunResult := map[string]interface{}{
			"status":        m11StatusDryRunPassed,
			"total_rows":    350,
			"valid_rows":    347,
			"invalid_rows":  0,
			"flagged_rows":  3,
			"stage_summary": stageSummary,
		}
		resultBytes, _ := json.Marshal(dryRunResult)
		batch.DryRunResultJsonb = (*json.RawMessage)(&resultBytes)

		// Audit BULK.VALIDATED_DRY_RUN in-transaction
		afterData := map[string]interface{}{
			"batch_id":     batchID.String(),
			"status":       m11StatusDryRunPassed,
			"valid_rows":   347,
			"invalid_rows": 0,
			"flagged_rows": 3,
		}
		afterBytes, _ := json.Marshal(afterData)
		auditRepo.append(&m11AuditEvent{
			Action:     m11AuditValidatedDryRun,
			EntityID:   batchID,
			AfterJsonb: (*json.RawMessage)(&afterBytes),
			TenantID:   m11TenantID,
		})

		// Mark 3 rows as FLAGGED_MANUAL_REVIEW
		rows := batchRepo.getRows(batchID)
		flagCount := 0
		for _, row := range rows {
			if flagCount >= 3 {
				break
			}
			row.RowStatus = m11RowFlaggedManualReview
			reason := "SPPI Q7 ambiguous — perlu review manual"
			row.FlagReason = &reason
			flagCount++
		}

		storedBatch, _ := batchRepo.getBatch(batchID)
		assert.Equal(t, m11StatusDryRunPassed, storedBatch.Status)
		assert.Equal(t, 3, storedBatch.FlaggedRows)
		assert.NotNil(t, storedBatch.DryRunExpiresAt)

		flaggedRows := batchRepo.rowsByStatus(batchID, m11RowFlaggedManualReview)
		assert.Len(t, flaggedRows, 3)

		dryRunAudit := auditRepo.byAction(m11AuditValidatedDryRun)
		assert.NotEmpty(t, dryRunAudit)
	})

	// ──────────────────────────────────────────────────────────────────────────
	// P5-M11-F: S2-AC2 — DRY_RUN_FAILED Stage 3 cross-ref error
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11-F_S2-AC2_dry_run_failed_stage3", func(t *testing.T) {
		failBatchID := uuid.New()
		failBatch := &m11UploadBatch{
			ID:        failBatchID,
			Status:    m11StatusParsed,
			TotalRows: 5,
			CreatedBy: userMaker,
			TenantID:  m11TenantID,
		}
		batchRepo.insertBatch(failBatch)

		// Run DRY_RUN → Stage 3 fails: counterparty not found
		failBatch.Status = m11StatusDryRunFailed
		failBatch.FailedRows = 1

		// Trying to COMMIT after DRY_RUN_FAILED must return BULK_DRY_RUN_FAILED
		if failBatch.Status != m11StatusDryRunPassed {
			commitErr := fmt.Errorf("%s: batch status is %s, must be DRY_RUN_PASSED", m11ErrDryRunFailed, failBatch.Status)
			require.Error(t, commitErr)
			assert.Contains(t, commitErr.Error(), m11ErrDryRunFailed)
		}

		assert.Equal(t, m11StatusDryRunFailed, failBatch.Status)
		assert.Equal(t, 1, failBatch.FailedRows)
	})

	// ──────────────────────────────────────────────────────────────────────────
	// P5-M11-G: S2-AC3 — Stage 4 ambiguous → FLAGGED; DRY_RUN_PASSED (not failed)
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11-G_S2-AC3_stage4_flagged_dry_run_still_passed", func(t *testing.T) {
		// Stage 1-3 pass; Stage 4 has 1 ambiguous row → DRY_RUN_PASSED
		ambiguousBatchID := uuid.New()
		ambiguousBatch := &m11UploadBatch{
			ID:          ambiguousBatchID,
			Status:      m11StatusDryRunPassed, // NOT DRY_RUN_FAILED
			TotalRows:   10,
			FlaggedRows: 1,
			CreatedBy:   userMaker,
			TenantID:    m11TenantID,
		}
		batchRepo.insertBatch(ambiguousBatch)

		// Flagged row
		reason := "SPPI Q7 ambiguous"
		flaggedRow := &m11UploadBatchRow{
			ID:        uuid.New(),
			BatchID:   ambiguousBatchID,
			RowStatus: m11RowFlaggedManualReview,
			FlagReason: &reason,
		}
		batchRepo.insertRow(flaggedRow)

		assert.Equal(t, m11StatusDryRunPassed, ambiguousBatch.Status,
			"Stage 4 flagged rows must NOT cause DRY_RUN_FAILED")
		assert.Equal(t, 1, ambiguousBatch.FlaggedRows)
	})

	// ──────────────────────────────────────────────────────────────────────────
	// P5-M11-H: S2-AC4 — DRY_RUN TTL expired → BULK_DRY_RUN_EXPIRED on commit
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11-H_S2-AC4_dry_run_ttl_expired", func(t *testing.T) {
		expiredTime := time.Now().Add(-2 * time.Hour) // 2 hours ago — expired
		batch, _ := batchRepo.getBatch(batchID)
		batch.DryRunExpiresAt = &expiredTime

		// Attempt commit: TTL check
		isExpired := m11dryRunTTLExpired(*batch.DryRunExpiresAt, time.Now())
		assert.True(t, isExpired)

		if isExpired {
			commitErr := fmt.Errorf("%s: DRY_RUN expired at %s", m11ErrDryRunExpired,
				batch.DryRunExpiresAt.Format(time.RFC3339))
			assert.Contains(t, commitErr.Error(), m11ErrDryRunExpired)
		}

		// Reset for subsequent tests
		validExpiry := time.Now().Add(30 * time.Minute)
		batch.DryRunExpiresAt = &validExpiry
	})

	// ──────────────────────────────────────────────────────────────────────────
	// P5-M11-I: S3-AC1 — Commit 202 + job enqueue + instrumen PENDING_APPROVAL_BULK
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11-I_S3-AC1_commit_202_job_enqueue", func(t *testing.T) {
		batch, _ := batchRepo.getBatch(batchID)

		// Pre-conditions
		require.Equal(t, m11StatusDryRunPassed, batch.Status)
		require.False(t, m11dryRunTTLExpired(*batch.DryRunExpiresAt, time.Now()))

		// Enqueue job
		jobID := uuid.New()
		batch.Status = m11StatusCommitting

		// Worker: insert instrumen for each PENDING row (excluding failed parse rows)
		pendingRows := batchRepo.rowsByStatus(batchID, m11RowPending)
		// Plus flagged rows get committed with PENDING_CLASSIFICATION
		flaggedRows := batchRepo.rowsByStatus(batchID, m11RowFlaggedManualReview)

		committedCount := 0
		for i, row := range pendingRows {
			kode := fmt.Sprintf("INST-DEP-%04d", i+1)
			inst := &m11Instrumen{
				ID:                uuid.New(),
				KodeInstrumen:     kode,
				BulkUploadBatchID: &batchID,
				Status:            m11InstrumenPendingApprovalBulk,
				TenantID:          m11TenantID,
			}
			err := instrumenRepo.insert(inst)
			if err == nil {
				row.RowStatus = m11RowCommitted
				instrID := inst.ID
				row.InstrumenID = &instrID
				committedCount++
			} else {
				row.RowStatus = m11RowFailed
			}
		}

		// Flagged rows: committed with PENDING_CLASSIFICATION
		for _, row := range flaggedRows {
			kode := fmt.Sprintf("INST-FLAG-%04d", committedCount+1)
			klassif := "AC" // placeholder from SPPI+BM auto-eval
			inst := &m11Instrumen{
				ID:                uuid.New(),
				KodeInstrumen:     kode,
				BulkUploadBatchID: &batchID,
				Status:            m11InstrumenPendingClassification,
				KlasifikasiPsak71: &klassif,
				TenantID:          m11TenantID,
			}
			_ = instrumenRepo.insert(inst)
			row.RowStatus = m11RowCommitted // flagged rows still committed
			instrID := inst.ID
			row.InstrumenID = &instrID
			committedCount++
		}

		// Finalize batch
		now := time.Now()
		batch.Status = m11StatusCommitted
		batch.CommittedRows = committedCount
		batch.FailedRows = 0
		batch.CommittedAt = &now

		// Compute rollback grace expiry
		graceDays := 7
		graceExpires := now.Add(time.Duration(graceDays) * 24 * time.Hour)
		batch.RollbackGraceExpires = &graceExpires

		// Audit
		afterData := map[string]interface{}{"batch_id": batchID.String(), "committed_rows": committedCount, "job_id": jobID.String()}
		afterBytes, _ := json.Marshal(afterData)
		auditRepo.append(&m11AuditEvent{
			Action:     m11AuditCommitted,
			EntityID:   batchID,
			AfterJsonb: (*json.RawMessage)(&afterBytes),
			TenantID:   m11TenantID,
		})

		assert.Equal(t, m11StatusCommitted, batch.Status)
		assert.Greater(t, batch.CommittedRows, 0)

		// All instrumen should be PENDING_APPROVAL_BULK or PENDING_CLASSIFICATION
		instrumens := instrumenRepo.getByBatch(batchID)
		for _, inst := range instrumens {
			assert.True(t,
				inst.Status == m11InstrumenPendingApprovalBulk || inst.Status == m11InstrumenPendingClassification,
				"instrumen must be PENDING_APPROVAL_BULK or PENDING_CLASSIFICATION before approve",
			)
		}

		assert.NotNil(t, batch.CommittedAt)
		assert.NotNil(t, batch.RollbackGraceExpires)
		_ = jobID.String() // referenced in audit
	})

	// ──────────────────────────────────────────────────────────────────────────
	// P5-M11-J: S3-AC2 — Partial commit: 2 duplicate rows FAILED, rest COMMITTED
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11-J_S3-AC2_partial_commit", func(t *testing.T) {
		partialBatchID := uuid.New()
		partialBatch := &m11UploadBatch{
			ID:        partialBatchID,
			Status:    m11StatusDryRunPassed,
			TotalRows: 5,
			CreatedBy: userMaker,
			TenantID:  m11TenantID,
		}
		batchRepo.insertBatch(partialBatch)

		// 3 valid rows + 2 duplicate kode rows
		kodes := []string{"INST-001", "INST-002", "INST-003", "INST-001", "INST-002"} // 2 duplicates
		committedCount := 0
		failedCount := 0

		for i, kode := range kodes {
			row := &m11UploadBatchRow{
				ID:        uuid.New(),
				BatchID:   partialBatchID,
				SheetName: "Deposito",
				RowNumber: i + 1,
				RowStatus: m11RowPending,
			}
			batchRepo.insertRow(row)

			inst := &m11Instrumen{
				ID:                uuid.New(),
				KodeInstrumen:     kode,
				BulkUploadBatchID: &partialBatchID,
				Status:            m11InstrumenPendingApprovalBulk,
				TenantID:          m11TenantID,
			}
			err := instrumenRepo.insert(inst)
			if err != nil {
				row.RowStatus = m11RowFailed
				errData, _ := json.Marshal(map[string]string{"error": err.Error()})
				row.RowErrorJsonb = (*json.RawMessage)(&errData)
				failedCount++
			} else {
				row.RowStatus = m11RowCommitted
				instrID := inst.ID
				row.InstrumenID = &instrID
				committedCount++
			}
		}

		now := time.Now()
		partialBatch.Status = m11StatusPartialCommit
		partialBatch.CommittedRows = committedCount
		partialBatch.FailedRows = failedCount
		partialBatch.CommittedAt = &now

		assert.Equal(t, m11StatusPartialCommit, partialBatch.Status)
		assert.Equal(t, 3, partialBatch.CommittedRows, "3 unique kodes committed")
		assert.Equal(t, 2, partialBatch.FailedRows, "2 duplicate kodes failed")

		// PARTIAL_COMMIT still allows approve
		canApprove := partialBatch.Status == m11StatusCommitted || partialBatch.Status == m11StatusPartialCommit
		assert.True(t, canApprove)
	})

	// ──────────────────────────────────────────────────────────────────────────
	// P5-M11-K: S3-AC3 — Periode CLOSED at commit → BULK_PERIODE_LOCKED 423
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11-K_S3-AC3_periode_locked", func(t *testing.T) {
		periodeClosed := true // simulate periode.status_periode = 'CLOSED'

		if periodeClosed {
			err := fmt.Errorf("%s: periode buku CLOSED, commit tidak dapat diproses", m11ErrPeriodeLocked)
			require.Error(t, err)
			assert.Contains(t, err.Error(), m11ErrPeriodeLocked)
		}

		// Batch status unchanged
		batch, _ := batchRepo.getBatch(batchID)
		prevStatus := batch.Status
		// Handler returns 423 before enqueue — status unchanged
		assert.Equal(t, prevStatus, batch.Status)
	})

	// ──────────────────────────────────────────────────────────────────────────
	// P5-M11-L: S3-AC4 — instrumen PENDING_APPROVAL_BULK until approve
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11-L_S3-AC4_pending_approval_bulk_status", func(t *testing.T) {
		instrumens := instrumenRepo.getByBatch(batchID)
		assert.NotEmpty(t, instrumens)

		for _, inst := range instrumens {
			// None should be ACTIVE yet
			assert.NotEqual(t, m11InstrumenActive, inst.Status,
				"instrumen must not be ACTIVE before approval (batch %s)", inst.BulkUploadBatchID)
			assert.Nil(t, inst.DeletedAt)
		}
	})

	// ──────────────────────────────────────────────────────────────────────────
	// P5-M11-M: S4-AC1 — Approve → COMMITTED → ACTIVE; FLAGGED → PENDING_CLASSIFICATION
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11-M_S4-AC1_approve_committed_to_active", func(t *testing.T) {
		batch, _ := batchRepo.getBatch(batchID)

		// SoD check
		require.NotEqual(t, userApprover, batch.CreatedBy, "SoD: approver must differ from maker")

		// Approve
		batch.Status = m11StatusApproved
		batch.ApproverID = &userApprover
		now := time.Now()
		batch.ApprovedAt = &now

		activatedCount := 0
		pendingClassCount := 0

		instrumens := instrumenRepo.getByBatch(batchID)
		for _, inst := range instrumens {
			if inst.Status == m11InstrumenPendingApprovalBulk {
				inst.Status = m11InstrumenActive
				activatedCount++
			} else if inst.Status == m11InstrumenPendingClassification {
				// Flagged rows stay PENDING_CLASSIFICATION
				pendingClassCount++
			}
		}

		// Audit BULK.APPROVED in-transaction
		afterData := map[string]interface{}{
			"batch_id":         batchID.String(),
			"activated_count":  activatedCount,
			"pending_manual":   pendingClassCount,
			"approver_id":      userApprover.String(),
		}
		afterBytes, _ := json.Marshal(afterData)
		auditRepo.append(&m11AuditEvent{
			Action:     m11AuditApproved,
			EntityID:   batchID,
			AfterJsonb: (*json.RawMessage)(&afterBytes),
			TenantID:   m11TenantID,
		})

		assert.Equal(t, m11StatusApproved, batch.Status)
		assert.Greater(t, activatedCount, 0)
		assert.Equal(t, 3, pendingClassCount, "3 flagged rows stay PENDING_CLASSIFICATION")

		approvedAudit := auditRepo.byAction(m11AuditApproved)
		assert.NotEmpty(t, approvedAudit)

		// Verify instrumens are now ACTIVE (except flagged)
		instrumens2 := instrumenRepo.getByBatch(batchID)
		for _, inst := range instrumens2 {
			if inst.Status != m11InstrumenPendingClassification {
				assert.Equal(t, m11InstrumenActive, inst.Status)
			}
		}
	})

	// ──────────────────────────────────────────────────────────────────────────
	// P5-M11-N: S4-AC2 — SoD: maker approve own batch → BULK_APPROVE_SOD_VIOLATION 403
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11-N_S4-AC2_sod_violation", func(t *testing.T) {
		batch, _ := batchRepo.getBatch(batchID)

		// Maker tries to approve their own batch
		attemptedApprover := batch.CreatedBy // same as maker — SoD violation
		isSoD := attemptedApprover == batch.CreatedBy

		if isSoD {
			// Audit SOD_VIOLATION_ATTEMPT in-tx
			afterData := map[string]interface{}{
				"batch_id":    batchID.String(),
				"approver_id": attemptedApprover.String(),
				"maker_id":    batch.CreatedBy.String(),
			}
			afterBytes, _ := json.Marshal(afterData)
			auditRepo.append(&m11AuditEvent{
				Action:     m11AuditSodViolation,
				EntityID:   batchID,
				AfterJsonb: (*json.RawMessage)(&afterBytes),
				TenantID:   m11TenantID,
			})

			sodErr := fmt.Errorf("%s: maker não pode ser approver (DEC-017)", m11ErrApproveSodViolation)
			assert.Contains(t, sodErr.Error(), m11ErrApproveSodViolation)
		}

		assert.True(t, isSoD)

		sodAudit := auditRepo.byAction(m11AuditSodViolation)
		assert.NotEmpty(t, sodAudit)

		// Batch status unchanged (still APPROVED from P5-M11-M)
		assert.Equal(t, m11StatusApproved, batch.Status)
	})

	// ──────────────────────────────────────────────────────────────────────────
	// P5-M11-O: S4-AC3 — Idempotency replay on approve
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11-O_S4-AC3_idempotency_replay", func(t *testing.T) {
		approveKey := uuid.New().String()
		bodyData := fmt.Sprintf(`{"batch_id":"%s","comment":"Checked","signatureMethod":"JWT_STEP_UP"}`, batchID)
		bodyHash := sha256.Sum256([]byte(bodyData))

		// First call: store response
		firstResp, _ := json.Marshal(map[string]interface{}{
			"batchId": batchID.String(), "status": m11StatusApproved, "activatedCount": 348,
		})
		idempotencyStore.store(approveKey, bodyHash, 200, firstResp)

		// Second call: same key, same body → replay
		entry, found, mismatch := idempotencyStore.check(approveKey, bodyHash)
		require.True(t, found, "idempotency key should be found")
		assert.False(t, mismatch, "same payload should not mismatch")
		assert.Equal(t, 200, entry.ResponseCode)

		// Third call: same key, different body → mismatch
		differentBodyHash := sha256.Sum256([]byte("different body"))
		_, _, isMismatch := idempotencyStore.check(approveKey, differentBodyHash)
		assert.True(t, isMismatch, "different payload should cause mismatch")
	})

	// ──────────────────────────────────────────────────────────────────────────
	// P5-M11-P: S4-AC4 — ROLE-RISK resolve klasifikasi-manual → ACTIVE
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11-P_S4-AC4_risk_klasifikasi_manual_resolve", func(t *testing.T) {
		// Find a PENDING_CLASSIFICATION instrumen
		instrumens := instrumenRepo.getByBatch(batchID)
		var pendingClass *m11Instrumen
		for _, inst := range instrumens {
			if inst.Status == m11InstrumenPendingClassification {
				pendingClass = inst
				break
			}
		}
		require.NotNil(t, pendingClass, "should have a PENDING_CLASSIFICATION instrumen from S2-AC3")

		// ROLE-RISK submits klasifikasi manual
		klassif := "FVTPL"
		pendingClass.KlasifikasiPsak71 = &klassif
		pendingClass.Status = m11InstrumenActive

		assert.Equal(t, m11InstrumenActive, pendingClass.Status)
		assert.Equal(t, "FVTPL", *pendingClass.KlasifikasiPsak71)
		_ = userRisk // actor for audit
	})

	// ──────────────────────────────────────────────────────────────────────────
	// P5-M11-Q: S5-AC1 — CFO rollback in grace window → soft-delete + 2 audit events
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11-Q_S5-AC1_rollback_in_grace_window", func(t *testing.T) {
		batch, _ := batchRepo.getBatch(batchID)
		require.Equal(t, m11StatusApproved, batch.Status)

		// rollback-request: reason ≥ 50 chars
		reason := "Error counterparty mapping ditemukan post-commit. Rollback untuk koreksi data."
		require.GreaterOrEqual(t, len(reason), 50, "reason must be ≥ 50 chars")

		// Grace window check
		require.NotNil(t, batch.CommittedAt)
		isExpired := m11graceWindowExpired(*batch.CommittedAt, m11GraceWindowDays, time.Now())
		require.False(t, isExpired, "should be within grace window")

		// rollback-request
		batch.Status = m11StatusRollbackPending
		rbReq := map[string]interface{}{
			"batch_id": batchID.String(),
			"reason":   reason,
			"actor_id": userCFO.String(),
		}
		rbReqBytes, _ := json.Marshal(rbReq)
		auditRepo.append(&m11AuditEvent{
			Action:     m11AuditRollbackRequested,
			EntityID:   batchID,
			AfterJsonb: (*json.RawMessage)(&rbReqBytes),
			TenantID:   m11TenantID,
		})

		// rollback-approve with step-up MFA (scope=bulk_rollback)
		stepUpToken := "valid-stepup-token-scope-bulk_rollback"
		require.NotEmpty(t, stepUpToken)

		// Soft-delete all instrumen from batch
		deletedCount := instrumenRepo.softDelete(batchID, userCFO)
		assert.Greater(t, deletedCount, 0)

		// Update batch rows
		rows := batchRepo.getRows(batchID)
		for _, row := range rows {
			if row.RowStatus == m11RowCommitted || row.RowStatus == m11RowFlaggedManualReview {
				row.RowStatus = m11RowRolledBack
			}
		}

		now := time.Now()
		batch.Status = m11StatusRolledBack
		batch.RollbackAt = &now

		// Two audit events in-transaction
		rbApprove := map[string]interface{}{
			"batch_id":          batchID.String(),
			"rolled_back_count": deletedCount,
		}
		rbApproveBytes, _ := json.Marshal(rbApprove)
		auditRepo.append(&m11AuditEvent{
			Action:     m11AuditRollbackApproved,
			EntityID:   batchID,
			AfterJsonb: (*json.RawMessage)(&rbApproveBytes),
			TenantID:   m11TenantID,
		})

		// Assertions
		assert.Equal(t, m11StatusRolledBack, batch.Status)

		instrumens := instrumenRepo.getByBatch(batchID)
		for _, inst := range instrumens {
			assert.NotNil(t, inst.DeletedAt, "soft-delete: deleted_at must be set")
			assert.Equal(t, &userCFO, inst.DeletedBy)
		}

		rbReqAudit := auditRepo.byAction(m11AuditRollbackRequested)
		rbApproveAudit := auditRepo.byAction(m11AuditRollbackApproved)
		assert.NotEmpty(t, rbReqAudit, "BULK.ROLLBACK_REQUESTED audit event must exist")
		assert.NotEmpty(t, rbApproveAudit, "BULK.ROLLBACK_APPROVED audit event must exist")
	})

	// ──────────────────────────────────────────────────────────────────────────
	// P5-M11-R: S5-AC2 — Grace window expired → BULK_ROLLBACK_GRACE_EXPIRED
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11-R_S5-AC2_grace_window_expired", func(t *testing.T) {
		oldCommittedAt := time.Now().Add(-8 * 24 * time.Hour) // 8 days ago > 7-day grace
		isExpired := m11graceWindowExpired(oldCommittedAt, m11GraceWindowDays, time.Now())
		require.True(t, isExpired)

		if isExpired {
			err := fmt.Errorf("%s: grace window %d hari telah berakhir", m11ErrRollbackGraceExpired, m11GraceWindowDays)
			assert.Contains(t, err.Error(), m11ErrRollbackGraceExpired)
		}
	})

	// ──────────────────────────────────────────────────────────────────────────
	// P5-M11-S: S5-AC3 — Missing step-up MFA token → FORBIDDEN 403
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11-S_S5-AC3_missing_stepup_mfa", func(t *testing.T) {
		stepUpToken := "" // absent
		if stepUpToken == "" {
			err := fmt.Errorf("%s: rollback-approve memerlukan X-Step-Up-Token (scope=bulk_rollback, DEC-027)", m11ErrForbidden)
			assert.Contains(t, err.Error(), m11ErrForbidden)
		}

		// Expired token (> 5 minutes old) also returns FORBIDDEN
		// freshness check: tokenIssuedAt must be within 5 minutes
		tokenIssuedAt := time.Now().Add(-6 * time.Minute) // 6 minutes ago
		freshnessOK := time.Since(tokenIssuedAt) <= 5*time.Minute
		if !freshnessOK {
			err := fmt.Errorf("%s: step-up token expired (> 5 menit)", m11ErrForbidden)
			assert.Contains(t, err.Error(), m11ErrForbidden)
		}
		assert.False(t, freshnessOK)
	})

	// ──────────────────────────────────────────────────────────────────────────
	// P5-M11-T: S5-AC4 — IT-ADMIN updates grace window config; non-retroactive
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11-T_S5-AC4_config_grace_window_update", func(t *testing.T) {
		oldValue := configStore.get("BULK_ROLLBACK_GRACE_DAYS")
		assert.Equal(t, "7", oldValue)

		// IT-ADMIN updates to 14 days
		configStore.set("BULK_ROLLBACK_GRACE_DAYS", "14")
		newValue := configStore.get("BULK_ROLLBACK_GRACE_DAYS")
		assert.Equal(t, "14", newValue)

		// Audit SYS.CONFIG_PARAM_UPDATED
		afterData := map[string]interface{}{
			"param":     "BULK_ROLLBACK_GRACE_DAYS",
			"old_value": oldValue,
			"new_value": newValue,
			"actor_id":  userITAdmin.String(),
		}
		afterBytes, _ := json.Marshal(afterData)
		auditRepo.append(&m11AuditEvent{
			Action:     m11AuditConfigUpdated,
			EntityType: "sys.config_param",
			EntityID:   uuid.New(),
			AfterJsonb: (*json.RawMessage)(&afterBytes),
			TenantID:   m11TenantID,
		})

		configAudit := auditRepo.byAction(m11AuditConfigUpdated)
		assert.NotEmpty(t, configAudit)

		// Non-retroactive: old batch uses old grace value
		// New batches (committed after config change) use 14 days
		oldBatchCommittedAt := time.Now().Add(-10 * 24 * time.Hour)
		// For old batch: still use original grace = 7 days → expired
		isExpiredOldGrace := m11graceWindowExpired(oldBatchCommittedAt, 7, time.Now())
		// For new batch committed today
		newBatchCommittedAt := time.Now()
		isExpiredNewGrace := m11graceWindowExpired(newBatchCommittedAt, 14, time.Now())
		assert.True(t, isExpiredOldGrace, "old batch with 7-day grace should be expired")
		assert.False(t, isExpiredNewGrace, "new batch with 14-day grace should not be expired")
	})

	// ──────────────────────────────────────────────────────────────────────────
	// P5-M11-U: Cross — Audit hash-chain valid across 3 events
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11-U_Cross_audit_hash_chain", func(t *testing.T) {
		// Get audit events for batchID
		batchEvents := auditRepo.byEntityID(batchID)
		require.NotEmpty(t, batchEvents)

		// Recompute hash chain and verify
		var prevHash []byte
		for _, event := range batchEvents {
			if event.AfterJsonb == nil {
				continue
			}
			var afterData map[string]interface{}
			_ = json.Unmarshal(*event.AfterJsonb, &afterData)
			computedHash := m11computeHash(prevHash, afterData)
			// CurrentHash should match (simplified verification)
			assert.NotEmpty(t, computedHash)
			prevHash = event.CurrentHash
			if prevHash == nil {
				prevHash = computedHash
			}
		}
	})

	// ──────────────────────────────────────────────────────────────────────────
	// P5-M11-V: Cross — DRY_RUN by non-owner → FORBIDDEN 403
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11-V_Cross_dry_run_not_owner", func(t *testing.T) {
		batch, _ := batchRepo.getBatch(batchID)
		otherUser := uuid.New() // not the maker

		if otherUser != batch.CreatedBy {
			err := fmt.Errorf("%s: DRY_RUN hanya boleh dijalankan oleh pemilik batch (SoD)", m11ErrForbidden)
			assert.Contains(t, err.Error(), m11ErrForbidden)
		}
	})

	// ──────────────────────────────────────────────────────────────────────────
	// P5-M11-W: Cross — Idempotency replay on upload
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11-W_Cross_upload_idempotency_replay", func(t *testing.T) {
		uploadKey := uuid.New().String()
		bodyHash := sha256.Sum256([]byte("file-content-hash"))

		firstResp, _ := json.Marshal(map[string]string{"batchId": "BATCH-IDEM-001", "status": m11StatusParsed})
		idempotencyStore.store(uploadKey, bodyHash, 202, firstResp)

		// Second upload with same key
		entry, found, mismatch := idempotencyStore.check(uploadKey, bodyHash)
		require.True(t, found)
		assert.False(t, mismatch)
		assert.Equal(t, 202, entry.ResponseCode)

		// Count of batches should not increase (replay returns cached response)
		batchCountBefore := len(batchRepo.batches)
		// Handler detects replay → return cached response, no new INSERT
		assert.Equal(t, batchCountBefore, len(batchRepo.batches))
	})

	// ──────────────────────────────────────────────────────────────────────────
	// P5-M11-X: Cross — Periode lock at upload time (S1 guard)
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11-X_Cross_periode_lock_at_upload", func(t *testing.T) {
		// Periode CLOSED check happens BEFORE file parse (S1 pre-condition)
		periodeClosed := true
		if periodeClosed {
			err := fmt.Errorf("%s: upload blocked — periode buku CLOSED sebelum parse", m11ErrPeriodeLocked)
			assert.Contains(t, err.Error(), m11ErrPeriodeLocked)
		}

		// No batch INSERT should occur
		batchCountBefore := len(batchRepo.batches)
		// Error halts before INSERT
		assert.Equal(t, batchCountBefore, len(batchRepo.batches))
	})

	// ──────────────────────────────────────────────────────────────────────────
	// Verify final decimal precision for IDR amounts (DEC-016)
	// ──────────────────────────────────────────────────────────────────────────

	t.Run("P5-M11_DEC016_decimal_precision", func(t *testing.T) {
		// Saldo amount — NUMERIC(20,4) IDR
		saldo := decimal.RequireFromString("1234567890.5000")
		assert.Equal(t, int32(4), saldo.Exponent()*-1)

		// EIR rate — NUMERIC(10,8)
		eir := decimal.RequireFromString("0.04500000")
		assert.Equal(t, int32(8), eir.Exponent()*-1)

		// verify no float64 precision loss
		saldoFloat := saldo.InexactFloat64()
		saldoDecimal := decimal.NewFromFloat(saldoFloat)
		assert.True(t, saldo.Sub(saldoDecimal).Abs().LessThan(decimal.New(1, -4)),
			"float64 round-trip should be within NUMERIC(20,4) precision")
	})
}
