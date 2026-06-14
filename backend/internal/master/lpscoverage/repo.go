package lpscoverage

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
)

// Repository defines the data-access contract for lps_coverage.
type Repository interface {
	// Create inserts a new lps_coverage row in the given transaction.
	Create(ctx context.Context, tx *sql.Tx, lc *LPSCoverage) error

	// GetByID fetches one record by UUID.
	// Returns (nil, nil) if not found.
	GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*LPSCoverage, error)

	// List fetches paginated records matching listquery + cursor.
	// Returns rows limited to limit+1 (caller detects hasMore).
	List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*LPSCoverage, error)

	// Update applies partial update in the given transaction.
	// Optimistic lock: UPDATE ... WHERE row_version = expected AND id = id.
	// Returns ErrNotFound if not found; ErrConflict if row_version mismatch.
	Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, fields UpdateFields) (*LPSCoverage, error)

	// SoftDelete sets deleted_at/deleted_by in the given transaction.
	SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*LPSCoverage, error)

	// UpdateWorkflowStatusTx updates workflow_status within an existing transaction.
	// Called by WorkflowHook.BeforeCommit and service.SyncWorkflowStatus.
	UpdateWorkflowStatusTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus) error

	// CountOverlap returns the count of non-deleted APPROVED records whose date range
	// overlaps with [dari, sampai]. Used to enforce the single-active invariant.
	// excludeID is excluded from the count (for update scenarios); pass uuid.Nil to skip exclusion.
	CountOverlap(ctx context.Context, dari string, sampai *string, excludeID uuid.UUID) (int64, error)

	// CountReferences returns the count of active references to lps_coverage records.
	// Defensive probe — ECL tables are not yet implemented; returns 0 for now.
	CountReferences(ctx context.Context, id uuid.UUID) (int64, error)

	// BeginTx starts a database transaction with ReadCommitted isolation.
	BeginTx(ctx context.Context) (*sql.Tx, error)

	// ListAuditHistory returns paginated audit log rows for a given entity UUID.
	ListAuditHistory(ctx context.Context, entityID uuid.UUID, cursor string, limit int, isAuditRole bool) ([]AuditHistoryItem, bool, error)

	// ExportAll returns all non-deleted records matching the query as a UTF-8 BOM CSV stream.
	// Caller must not close the returned reader (bytes.Buffer does not need closing).
	// tx must be the transaction opened by the caller; ExportAll runs its SELECT inside
	// that transaction so the audit write (also in tx) is committed atomically (DEC-018).
	ExportAll(ctx context.Context, tx *sql.Tx, q listquery.Query) (io.Reader, int, error)
}

// UpdateFields captures mutable columns for an update operation.
// Nil pointer = do not update that column.
type UpdateFields struct {
	CoverageAmount       *decimal.Decimal
	PeriodeBerlakuDari   *string
	PeriodeBerlakuSampai *string // empty string = set to NULL
	RegulasiReferensi    *string
	DokumenPendukungID   *uuid.UUID
	ClearSampai          bool // set periode_berlaku_sampai to NULL
	UpdatedBy            uuid.UUID
	ExpectedVersion      int64
}

// ─── DB implementation ────────────────────────────────────────────────────────

// DBRepository is the production SQL implementation.
type DBRepository struct {
	db *sql.DB
}

// NewDBRepository creates a new DBRepository.
func NewDBRepository(db *sql.DB) *DBRepository {
	return &DBRepository{db: db}
}

// Ensure DBRepository satisfies Repository at compile time.
var _ Repository = (*DBRepository)(nil)

// baseSelect is the SELECT fragment used by all read queries.
const baseSelect = `
SELECT
    id, coverage_amount, mata_uang,
    periode_berlaku_dari, periode_berlaku_sampai,
    regulasi_referensi, dokumen_pendukung_id,
    maker_id, approver_id,
    workflow_status,
    created_at, created_by, updated_at, updated_by,
    deleted_at, deleted_by, row_version, tenant_id
FROM mst.lps_coverage`

// Create inserts a new row inside a transaction.
func (r *DBRepository) Create(ctx context.Context, tx *sql.Tx, lc *LPSCoverage) error {
	query := `
INSERT INTO mst.lps_coverage (
    id, coverage_amount, mata_uang,
    periode_berlaku_dari, periode_berlaku_sampai,
    regulasi_referensi, dokumen_pendukung_id,
    maker_id, approver_id,
    workflow_status,
    created_at, created_by, updated_at, updated_by,
    row_version, tenant_id
) VALUES (
    $1, $2, $3,
    $4, $5,
    $6, $7,
    $8, $9,
    $10,
    $11, $12, $11, $12,
    1, $13
)`
	_, err := tx.ExecContext(ctx, query,
		lc.ID,
		lc.CoverageAmount,
		lc.MataUang,
		lc.PeriodeBerlakuDari,
		lc.PeriodeBerlakuSampai,
		lc.RegulasiReferensi,
		lc.DokumenPendukungID,
		lc.MakerID,
		lc.ApproverID,
		string(lc.WorkflowStatus),
		lc.CreatedAt,
		lc.CreatedBy,
		lc.TenantID,
	)
	if err != nil {
		return fmt.Errorf("repo.Create lps_coverage: %w", err)
	}
	return nil
}

// GetByID fetches one record by surrogate UUID.
func (r *DBRepository) GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*LPSCoverage, error) {
	deletedFilter := " AND deleted_at IS NULL"
	if includeDeleted {
		deletedFilter = ""
	}
	// #nosec G202 -- both operands are package constants, no user input
	query := baseSelect + " WHERE id = $1" + deletedFilter
	row := r.db.QueryRowContext(ctx, query, id)
	lc, err := scanLPSCoverage(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.GetByID lps_coverage: %w", err)
	}
	return lc, nil
}

// List fetches paginated records.
func (r *DBRepository) List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*LPSCoverage, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("t")

	conditions := []string{}
	if !includeDeleted {
		conditions = append(conditions, "t.deleted_at IS NULL")
	}

	// Cursor: UUID-based (sorted by created_at DESC then id)
	if cursor != "" {
		cd, err := pagination.DecodeCursor(cursor)
		if err == nil && cd.ID != "" {
			argIdx := len(args) + 1
			conditions = append(conditions, fmt.Sprintf("t.id > $%d", argIdx))
			args = append(args, cd.ID)
		}
	}

	// Text search
	if q.Search != "" {
		argIdx := len(args) + 1
		searchCond := make([]string, 0, len(SearchCols))
		for _, col := range SearchCols {
			searchCond = append(searchCond, fmt.Sprintf("t.%s ILIKE $%d", col, argIdx))
		}
		conditions = append(conditions, "("+strings.Join(searchCond, " OR ")+")")
		args = append(args, "%"+q.Search+"%")
	}

	if where != "" {
		conditions = append(conditions, where)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	if orderBy == "" {
		orderBy = "t.created_at DESC, t.id ASC"
	}

	argIdx := len(args) + 1
	query := fmt.Sprintf( //nolint:gosec
		`SELECT t.id, t.coverage_amount, t.mata_uang, t.periode_berlaku_dari, t.periode_berlaku_sampai,
		    t.regulasi_referensi, t.dokumen_pendukung_id, t.maker_id, t.approver_id, t.workflow_status,
		    t.created_at, t.created_by, t.updated_at, t.updated_by, t.deleted_at, t.deleted_by,
		    t.row_version, t.tenant_id
		 FROM mst.lps_coverage t%s ORDER BY %s LIMIT $%d`,
		whereClause, orderBy, argIdx,
	)
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.List lps_coverage: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []*LPSCoverage
	for rows.Next() {
		lc, err := scanLPSCoverageRow(rows)
		if err != nil {
			return nil, fmt.Errorf("repo.List lps_coverage scan: %w", err)
		}
		items = append(items, lc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo.List lps_coverage rows.Err: %w", err)
	}
	return items, nil
}

// Update applies partial field changes with optimistic lock.
func (r *DBRepository) Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, f UpdateFields) (*LPSCoverage, error) {
	setClauses := []string{}
	args := []interface{}{}
	idx := 1

	if f.CoverageAmount != nil {
		setClauses = append(setClauses, fmt.Sprintf("coverage_amount = $%d", idx))
		args = append(args, *f.CoverageAmount)
		idx++
	}
	if f.PeriodeBerlakuDari != nil {
		setClauses = append(setClauses, fmt.Sprintf("periode_berlaku_dari = $%d", idx))
		args = append(args, *f.PeriodeBerlakuDari)
		idx++
	}
	if f.ClearSampai {
		setClauses = append(setClauses, "periode_berlaku_sampai = NULL")
	} else if f.PeriodeBerlakuSampai != nil {
		setClauses = append(setClauses, fmt.Sprintf("periode_berlaku_sampai = $%d", idx))
		args = append(args, *f.PeriodeBerlakuSampai)
		idx++
	}
	if f.RegulasiReferensi != nil {
		setClauses = append(setClauses, fmt.Sprintf("regulasi_referensi = $%d", idx))
		args = append(args, *f.RegulasiReferensi)
		idx++
	}
	if f.DokumenPendukungID != nil {
		setClauses = append(setClauses, fmt.Sprintf("dokumen_pendukung_id = $%d", idx))
		args = append(args, *f.DokumenPendukungID)
		idx++
	}

	if len(setClauses) == 0 {
		return r.GetByID(ctx, id, false)
	}

	setClauses = append(setClauses,
		fmt.Sprintf("updated_at = $%d", idx),
		fmt.Sprintf("updated_by = $%d", idx+1),
		"row_version = row_version + 1",
	)
	args = append(args, time.Now(), f.UpdatedBy)
	idx += 2

	whereIDIdx := idx
	whereVersionIdx := idx + 1
	args = append(args, id, f.ExpectedVersion)

	query := fmt.Sprintf( //nolint:gosec
		`UPDATE mst.lps_coverage SET %s WHERE id = $%d AND row_version = $%d AND deleted_at IS NULL`,
		strings.Join(setClauses, ", "),
		whereIDIdx, whereVersionIdx,
	)

	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.Update lps_coverage: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("repo.Update lps_coverage rows affected: %w", err)
	}
	if n == 0 {
		existing, err := r.GetByID(ctx, id, false)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, ErrNotFound
		}
		return nil, ErrConflict
	}

	// Read fresh row inside the same tx using a tx-aware querier.
	return r.getOneTx(ctx, tx, id, false)
}

// getOneTx fetches one row using a *sql.Tx as querier.
func (r *DBRepository) getOneTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, includeDeleted bool) (*LPSCoverage, error) {
	deletedFilter := " AND deleted_at IS NULL"
	if includeDeleted {
		deletedFilter = ""
	}
	// #nosec G202 -- both operands are package constants, no user input
	query := baseSelect + " WHERE id = $1" + deletedFilter
	row := tx.QueryRowContext(ctx, query, id)
	lc, err := scanLPSCoverage(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.getOneTx lps_coverage: %w", err)
	}
	return lc, nil
}

// UpdateWorkflowStatusTx updates workflow_status inside an existing transaction.
// Guards: tenant_id must match (multi-tenant safety) and deleted_at IS NULL (no
// transitions on soft-deleted records). Returns ErrNotFound if the guard rejects.
func (r *DBRepository) UpdateWorkflowStatusTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus) error {
	tenantID := tenantIDFromCtx(ctx)
	res, err := tx.ExecContext(ctx, `
		UPDATE mst.lps_coverage
		SET workflow_status = $1, updated_at = now(), row_version = row_version + 1
		WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL
	`, string(status), id, tenantID)
	if err != nil {
		return fmt.Errorf("repo.UpdateWorkflowStatusTx lps_coverage: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repo.UpdateWorkflowStatusTx rows-affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SoftDelete marks the record as deleted.
func (r *DBRepository) SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*LPSCoverage, error) {
	now := time.Now()
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.lps_coverage
		SET deleted_at = $1, deleted_by = $2, updated_at = $1, updated_by = $2
		WHERE id = $3 AND deleted_at IS NULL
	`, now, deletedBy, id)
	if err != nil {
		return nil, fmt.Errorf("repo.SoftDelete lps_coverage: %w", err)
	}
	return r.getOneTx(ctx, tx, id, true)
}

// CountOverlap counts APPROVED non-deleted records whose date range overlaps [dari, sampai].
//
// Overlap logic (half-open intervals):
//
//	existing row overlaps proposed if:
//	  existing.dari <= proposed.sampai  (OR proposed.sampai IS NULL, meaning open-ended)
//	  AND (existing.sampai IS NULL OR existing.sampai >= proposed.dari)
//
// excludeID omits a specific row (used on updates to exclude self).
func (r *DBRepository) CountOverlap(ctx context.Context, dari string, sampai *string, excludeID uuid.UUID) (int64, error) {
	args := []interface{}{dari, "APPROVED"}
	conds := []string{
		"workflow_status = $2",
		"deleted_at IS NULL",
		// existing.sampai IS NULL (open-ended) OR existing.sampai >= proposed.dari
		"(periode_berlaku_sampai IS NULL OR periode_berlaku_sampai >= $1)",
	}

	argIdx := 3
	if sampai != nil {
		// proposed range has an end → existing.dari must be <= proposed.sampai
		conds = append(conds, fmt.Sprintf("periode_berlaku_dari <= $%d", argIdx))
		args = append(args, *sampai)
		argIdx++
	}
	// If proposed sampai is NULL (open-ended), any existing approved row overlaps.

	if excludeID != uuid.Nil {
		conds = append(conds, fmt.Sprintf("id != $%d", argIdx))
		args = append(args, excludeID)
	}

	query := fmt.Sprintf( //nolint:gosec
		"SELECT COUNT(*) FROM mst.lps_coverage WHERE %s",
		strings.Join(conds, " AND "),
	)

	var count int64
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("repo.CountOverlap lps_coverage: %w", err)
	}
	return count, nil
}

// CountReferences returns the count of active references to lps_coverage.
// ECL tables not yet implemented — returns 0.
func (r *DBRepository) CountReferences(_ context.Context, _ uuid.UUID) (int64, error) {
	// Phase 3 placeholder: once ecl.calc_run links to lps_coverage_id,
	// add a query here to count active references.
	return 0, nil
}

// BeginTx starts a transaction with ReadCommitted isolation.
func (r *DBRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("repo.BeginTx lps_coverage: %w", err)
	}
	return tx, nil
}

// ListAuditHistory returns paginated audit events for the given entity UUID.
func (r *DBRepository) ListAuditHistory(ctx context.Context, entityID uuid.UUID, cursor string, limit int, isAuditRole bool) ([]AuditHistoryItem, bool, error) {
	var cursorTime *time.Time
	if cursor != "" {
		cd, err := pagination.DecodeCursor(cursor)
		if err == nil && cd.ID != "" {
			t, err2 := time.Parse(time.RFC3339, cd.ID)
			if err2 == nil {
				cursorTime = &t
			}
		}
	}

	args := []interface{}{entityID, "mst.lps_coverage"}
	cond := ""
	if cursorTime != nil {
		args = append(args, *cursorTime)
		cond = fmt.Sprintf(" AND timestamp < $%d", len(args))
	}

	query := fmt.Sprintf( //nolint:gosec
		`SELECT
			id, timestamp, actor_user_id, actor_role, action,
			before_value, after_value, ip_address, trace_id
		FROM aud.audit_log
		WHERE entity_id = $1 AND entity_type = $2%s
		ORDER BY timestamp DESC
		LIMIT $%d`,
		cond, len(args)+1,
	)
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, fmt.Errorf("repo.ListAuditHistory lps_coverage: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []AuditHistoryItem
	for rows.Next() {
		var (
			eventID   uuid.UUID
			ts        time.Time
			actorUID  uuid.UUID
			actorRole string
			action    string
			beforeRaw []byte
			afterRaw  []byte
			ip        *string
			traceID   *string
		)
		if err := rows.Scan(&eventID, &ts, &actorUID, &actorRole, &action,
			&beforeRaw, &afterRaw, &ip, &traceID); err != nil {
			return nil, false, fmt.Errorf("repo.ListAuditHistory lps_coverage scan: %w", err)
		}
		item := AuditHistoryItem{
			EventID:     eventID.String(),
			EventTime:   ts.Format(time.RFC3339),
			ActorUserID: actorUID.String(),
			ActorRole:   actorRole,
			Action:      action,
			IP:          ip,
			TraceID:     traceID,
		}
		if isAuditRole {
			if len(beforeRaw) > 0 {
				var bj interface{}
				if jsonErr := json.Unmarshal(beforeRaw, &bj); jsonErr == nil {
					item.BeforeJSONB = bj
				}
			}
			if len(afterRaw) > 0 {
				var aj interface{}
				if jsonErr := json.Unmarshal(afterRaw, &aj); jsonErr == nil {
					item.AfterJSONB = aj
				}
			}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("repo.ListAuditHistory lps_coverage rows.Err: %w", err)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	return items, hasMore, nil
}

// ExportAll streams all non-deleted records as UTF-8 BOM CSV inside tx.
// The SELECT runs inside the caller-supplied transaction so that the audit write
// that follows in the same tx is committed atomically (DEC-018).
func (r *DBRepository) ExportAll(ctx context.Context, tx *sql.Tx, q listquery.Query) (io.Reader, int, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("t")

	conditions := []string{"t.deleted_at IS NULL"}
	if q.Search != "" {
		argIdx := len(args) + 1
		searchCond := make([]string, 0, len(SearchCols))
		for _, col := range SearchCols {
			searchCond = append(searchCond, fmt.Sprintf("t.%s ILIKE $%d", col, argIdx))
		}
		conditions = append(conditions, "("+strings.Join(searchCond, " OR ")+")")
		args = append(args, "%"+q.Search+"%")
	}
	if where != "" {
		conditions = append(conditions, where)
	}

	whereClause := " WHERE " + strings.Join(conditions, " AND ")
	if orderBy == "" {
		orderBy = "t.created_at DESC"
	}

	query := fmt.Sprintf( //nolint:gosec
		`SELECT t.id, t.coverage_amount, t.mata_uang, t.periode_berlaku_dari,
		    t.periode_berlaku_sampai, t.regulasi_referensi, t.workflow_status
		 FROM mst.lps_coverage t%s ORDER BY %s`,
		whereClause, orderBy,
	)

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll lps_coverage: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var buf bytes.Buffer
	// UTF-8 BOM for Excel compatibility
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(&buf)

	headers := []string{"ID", "Coverage Amount (IDR)", "Mata Uang", "Periode Dari", "Periode Sampai", "Regulasi Referensi", "Workflow Status"}
	if err := w.Write(headers); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll lps_coverage write header: %w", err)
	}

	count := 0
	for rows.Next() {
		var (
			id       uuid.UUID
			amount   decimal.Decimal
			mataUang string
			dari     string
			sampai   *string
			regulasi *string
			wfStatus string
		)
		if err := rows.Scan(&id, &amount, &mataUang, &dari, &sampai, &regulasi, &wfStatus); err != nil {
			return nil, 0, fmt.Errorf("repo.ExportAll lps_coverage scan: %w", err)
		}
		sampaiStr := ""
		if sampai != nil {
			sampaiStr = *sampai
		}
		regulasiStr := ""
		if regulasi != nil {
			regulasiStr = *regulasi
		}
		record := []string{
			id.String(),
			amount.StringFixed(4),
			mataUang,
			dari,
			sampaiStr,
			regulasiStr,
			wfStatus,
		}
		if err := w.Write(record); err != nil {
			return nil, 0, fmt.Errorf("repo.ExportAll lps_coverage write record: %w", err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll lps_coverage rows.Err: %w", err)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll lps_coverage flush: %w", err)
	}
	return &buf, count, nil
}

// ─── Scan helpers ─────────────────────────────────────────────────────────────

// scanLPSCoverage scans one *sql.Row into LPSCoverage.
func scanLPSCoverage(row *sql.Row) (*LPSCoverage, error) {
	lc := &LPSCoverage{}
	var (
		amountStr      string
		dari           string
		sampai         *string
		regulasi       *string
		dokumenID      *uuid.UUID
		makerID        uuid.UUID
		approverID     *uuid.UUID
		workflowStatus string
		createdBy      *uuid.UUID
		updatedAt      *time.Time
		updatedBy      *uuid.UUID
		deletedAt      *time.Time
		deletedBy      *uuid.UUID
	)
	err := row.Scan(
		&lc.ID,
		&amountStr,
		&lc.MataUang,
		&dari,
		&sampai,
		&regulasi,
		&dokumenID,
		&makerID,
		&approverID,
		&workflowStatus,
		&lc.CreatedAt,
		&createdBy,
		&updatedAt,
		&updatedBy,
		&deletedAt,
		&deletedBy,
		&lc.RowVersion,
		&lc.TenantID,
	)
	if err != nil {
		return nil, err
	}
	amount, err := decimal.NewFromString(amountStr)
	if err != nil {
		return nil, fmt.Errorf("parse coverage_amount %q: %w", amountStr, err)
	}
	lc.CoverageAmount = amount
	lc.PeriodeBerlakuDari = dari
	lc.PeriodeBerlakuSampai = sampai
	lc.RegulasiReferensi = regulasi
	lc.DokumenPendukungID = dokumenID
	lc.MakerID = makerID
	lc.ApproverID = approverID
	lc.WorkflowStatus = WorkflowStatus(workflowStatus)
	lc.CreatedBy = createdBy
	lc.UpdatedAt = updatedAt
	lc.UpdatedBy = updatedBy
	lc.DeletedAt = deletedAt
	lc.DeletedBy = deletedBy
	return lc, nil
}

// scanLPSCoverageRow scans one *sql.Rows row into LPSCoverage.
func scanLPSCoverageRow(rows *sql.Rows) (*LPSCoverage, error) {
	lc := &LPSCoverage{}
	var (
		amountStr      string
		dari           string
		sampai         *string
		regulasi       *string
		dokumenID      *uuid.UUID
		makerID        uuid.UUID
		approverID     *uuid.UUID
		workflowStatus string
		createdBy      *uuid.UUID
		updatedAt      *time.Time
		updatedBy      *uuid.UUID
		deletedAt      *time.Time
		deletedBy      *uuid.UUID
	)
	err := rows.Scan(
		&lc.ID,
		&amountStr,
		&lc.MataUang,
		&dari,
		&sampai,
		&regulasi,
		&dokumenID,
		&makerID,
		&approverID,
		&workflowStatus,
		&lc.CreatedAt,
		&createdBy,
		&updatedAt,
		&updatedBy,
		&deletedAt,
		&deletedBy,
		&lc.RowVersion,
		&lc.TenantID,
	)
	if err != nil {
		return nil, err
	}
	amount, err := decimal.NewFromString(amountStr)
	if err != nil {
		return nil, fmt.Errorf("parse coverage_amount %q: %w", amountStr, err)
	}
	lc.CoverageAmount = amount
	lc.PeriodeBerlakuDari = dari
	lc.PeriodeBerlakuSampai = sampai
	lc.RegulasiReferensi = regulasi
	lc.DokumenPendukungID = dokumenID
	lc.MakerID = makerID
	lc.ApproverID = approverID
	lc.WorkflowStatus = WorkflowStatus(workflowStatus)
	lc.CreatedBy = createdBy
	lc.UpdatedAt = updatedAt
	lc.UpdatedBy = updatedBy
	lc.DeletedAt = deletedAt
	lc.DeletedBy = deletedBy
	return lc, nil
}

// ─── Sentinel errors ──────────────────────────────────────────────────────────

// ErrNotFound is returned when a record is not found.
var ErrNotFound = fmt.Errorf("lps_coverage not found")

// ErrConflict is returned on row_version mismatch.
var ErrConflict = fmt.Errorf("optimistic lock conflict")

// tenantIDFromCtx extracts the tenant_id from context claims, defaulting to TUGURE.
func tenantIDFromCtx(ctx context.Context) string {
	claims := auth.ClaimsFromContext(ctx)
	if claims != nil && claims.TenantID != "" {
		return claims.TenantID
	}
	return "TUGURE"
}
