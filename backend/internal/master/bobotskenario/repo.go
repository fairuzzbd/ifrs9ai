package bobotskenario

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
)

// Repository defines the data-access contract for bobot_skenario.
// Service layer only uses this interface — no SQL in service or handler.
type Repository interface {
	// Create inserts a new bobot_skenario row in the given transaction.
	Create(ctx context.Context, tx *sql.Tx, e *BobotSkenario) error

	// GetByID fetches one record by surrogate UUID.
	// Returns (nil, nil) if not found (soft-deleted returned if includeDeleted=true).
	GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*BobotSkenario, error)

	// List fetches paginated records matching listquery + cursor.
	// Returns rows limited to limit+1 (caller detects hasMore).
	List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*BobotSkenario, error)

	// Update applies partial update in the given transaction.
	// Uses optimistic lock: UPDATE … WHERE row_version = expected AND id = id.
	// Returns ErrNotFound if id not found; ErrConflict if row_version mismatch.
	Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, f UpdateFields) (*BobotSkenario, error)

	// SoftDelete sets deleted_at/deleted_by in the given transaction.
	SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*BobotSkenario, error)

	// UpdateWorkflowStatus updates workflow_status (called after workflow engine transitions).
	UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error

	// CountReferences returns the number of active ECL calc-result lines referencing this row.
	CountReferences(ctx context.Context, id uuid.UUID) (int64, error)

	// CountOverlap counts rows of the same skenario whose [dari, sampai) period overlaps.
	// Excludes the given excludeID (for update checks).
	CountOverlap(ctx context.Context, skenario Skenario, dari string, sampai *string, excludeID uuid.UUID) (int64, error)

	// CountDuplicate counts rows with exactly the same (skenario, dari, sampai) tuple.
	// Excludes the given excludeID. Used to enforce uniqueness within a period.
	CountDuplicate(ctx context.Context, skenario Skenario, dari string, sampai *string, excludeID uuid.UUID) (int64, error)

	// CountByPeriod counts active rows for the given (dari, sampai) period tuple.
	// Used to detect if a period already has N skenario rows.
	CountByPeriod(ctx context.Context, dari string, sampai *string) (int64, error)

	// SumByPeriod returns the sum of bobot for all active, non-REJECTED rows in a period.
	// Returns decimal.Zero if no rows match.
	SumByPeriod(ctx context.Context, dari string, sampai *string, excludeID uuid.UUID) (decimal.Decimal, error)

	// SumByPeriodTx is identical to SumByPeriod but runs inside the given transaction.
	// Used by WorkflowHook.BeforeCommit to perform the sum check atomically.
	SumByPeriodTx(ctx context.Context, tx *sql.Tx, dari string, sampai *string, excludeID uuid.UUID) (decimal.Decimal, error)

	// UpdateWorkflowStatusTx updates workflow_status inside the given transaction.
	// Used by WorkflowHook.BeforeCommit to keep mst.bobot_skenario.workflow_status
	// in sync with sys.workflow_instance in the same atomic transaction.
	UpdateWorkflowStatusTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error

	// BeginTx starts a database transaction with read-committed isolation.
	BeginTx(ctx context.Context) (*sql.Tx, error)

	// ListAuditHistory returns paginated audit log rows for the given entity UUID.
	ListAuditHistory(ctx context.Context, entityID uuid.UUID, cursor string, limit int, isAuditRole bool) ([]AuditHistoryItem, bool, error)

	// ExportAll returns all records matching the query as a UTF-8 BOM CSV io.Reader.
	ExportAll(ctx context.Context, q listquery.Query) (io.Reader, int, error)
}

// UpdateFields captures the mutable columns for an update operation.
// Nil pointer = do not update that column.
type UpdateFields struct {
	Skenario             *Skenario
	Bobot                *decimal.Decimal
	PeriodeBerlakuDari   *string
	PeriodeBerlakuSampai *string // nil = leave unchanged; pointer to empty string = set to NULL
	Catatan              *string
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

// baseSelectCols is the ordered column list for all SELECT queries.
const baseSelectCols = `
	id, skenario, bobot, catatan,
	periode_berlaku_dari, periode_berlaku_sampai,
	maker_id, approver_id, approved_at,
	workflow_status,
	created_at, created_by, updated_at, updated_by,
	deleted_at, deleted_by, row_version, tenant_id`

const baseSelectFrom = `FROM mst.bobot_skenario`

// Create inserts a new row. Called inside a transaction.
func (r *DBRepository) Create(ctx context.Context, tx *sql.Tx, e *BobotSkenario) error {
	query := `
INSERT INTO mst.bobot_skenario (
	id, skenario, bobot, catatan,
	periode_berlaku_dari, periode_berlaku_sampai,
	maker_id, approver_id, approved_at,
	workflow_status,
	created_at, created_by, updated_at, updated_by,
	row_version, tenant_id
) VALUES (
	$1, $2, $3, $4,
	$5, $6,
	$7, $8, $9,
	$10,
	$11, $12, $11, $12,
	1, $13
)`
	_, err := tx.ExecContext(ctx, query,
		e.ID,
		string(e.Skenario),
		e.Bobot.StringFixed(8), // store with full 8dp precision
		e.Catatan,
		e.PeriodeBerlakuDari,
		e.PeriodeBerlakuSampai,
		e.MakerID,    // legacy NOT NULL column
		e.ApproverID, // nullable legacy column
		e.ApprovedAt, // nullable legacy column
		string(e.WorkflowStatus),
		e.CreatedAt,
		e.CreatedBy,
		e.TenantID,
	)
	if err != nil {
		return fmt.Errorf("repo.Create bobot_skenario: %w", err)
	}
	return nil
}

// GetByID fetches by surrogate UUID.
func (r *DBRepository) GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*BobotSkenario, error) {
	return r.getOne(ctx, r.db, "id = $1", id, includeDeleted)
}

type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

func (r *DBRepository) getOne(ctx context.Context, q querier, where string, arg interface{}, includeDeleted bool) (*BobotSkenario, error) {
	deletedFilter := " AND deleted_at IS NULL"
	if includeDeleted {
		deletedFilter = ""
	}
	query := "SELECT" + baseSelectCols + " " + baseSelectFrom + " WHERE " + where + deletedFilter
	row := q.QueryRowContext(ctx, query, arg)
	e, err := scanBobotSkenarioRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.getOne bobot_skenario: %w", err)
	}
	return e, nil
}

// List fetches paginated records.
func (r *DBRepository) List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*BobotSkenario, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("t")

	conditions := []string{}
	if !includeDeleted {
		conditions = append(conditions, "t.deleted_at IS NULL")
	}

	if cursor != "" {
		cd, err := pagination.DecodeCursor(cursor)
		if err == nil && cd.ID != "" {
			argIdx := len(args) + 1
			conditions = append(conditions, fmt.Sprintf("t.id::text > $%d", argIdx))
			args = append(args, cd.ID)
		}
	}

	if q.Search != "" {
		argIdx := len(args) + 1
		searchCond := make([]string, 0, len(SearchCols))
		for _, col := range SearchCols {
			if col == "catatan" {
				searchCond = append(searchCond, fmt.Sprintf("t.%s ILIKE $%d", col, argIdx))
			} else {
				searchCond = append(searchCond, fmt.Sprintf("t.%s::text ILIKE $%d", col, argIdx))
			}
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
		orderBy = "t.skenario ASC, t.periode_berlaku_dari DESC"
	}

	argIdx := len(args) + 1
	query := fmt.Sprintf( //nolint:gosec
		"SELECT t.id, t.skenario, t.bobot, t.catatan, t.periode_berlaku_dari, t.periode_berlaku_sampai, t.maker_id, t.approver_id, t.approved_at, t.workflow_status, t.created_at, t.created_by, t.updated_at, t.updated_by, t.deleted_at, t.deleted_by, t.row_version, t.tenant_id FROM mst.bobot_skenario t%s ORDER BY %s LIMIT $%d",
		whereClause, orderBy, argIdx,
	)
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.List bobot_skenario: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []*BobotSkenario
	for rows.Next() {
		e, err := scanBobotSkenarioRows(rows)
		if err != nil {
			return nil, fmt.Errorf("repo.List scan: %w", err)
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo.List rows.Err: %w", err)
	}
	return items, nil
}

// Update applies partial field changes with optimistic lock.
func (r *DBRepository) Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, f UpdateFields) (*BobotSkenario, error) {
	setClauses := []string{}
	args := []interface{}{}
	idx := 1

	if f.Skenario != nil {
		setClauses = append(setClauses, fmt.Sprintf("skenario = $%d", idx))
		args = append(args, string(*f.Skenario))
		idx++
	}
	if f.Bobot != nil {
		setClauses = append(setClauses, fmt.Sprintf("bobot = $%d", idx))
		args = append(args, f.Bobot.StringFixed(8))
		idx++
	}
	if f.PeriodeBerlakuDari != nil {
		setClauses = append(setClauses, fmt.Sprintf("periode_berlaku_dari = $%d", idx))
		args = append(args, *f.PeriodeBerlakuDari)
		idx++
	}
	if f.PeriodeBerlakuSampai != nil {
		setClauses = append(setClauses, fmt.Sprintf("periode_berlaku_sampai = $%d", idx))
		args = append(args, *f.PeriodeBerlakuSampai)
		idx++
	}
	if f.Catatan != nil {
		setClauses = append(setClauses, fmt.Sprintf("catatan = $%d", idx))
		args = append(args, *f.Catatan)
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
		`UPDATE mst.bobot_skenario SET %s WHERE id = $%d AND row_version = $%d AND deleted_at IS NULL`,
		strings.Join(setClauses, ", "),
		whereIDIdx, whereVersionIdx,
	)

	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.Update bobot_skenario: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("repo.Update rows affected: %w", err)
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
	return r.getOne(ctx, tx, "id = $1", id, false)
}

// UpdateWorkflowStatus updates only the workflow_status column.
func (r *DBRepository) UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.bobot_skenario
		SET workflow_status = $1, updated_at = now(), updated_by = $2, row_version = row_version + 1
		WHERE id = $3
	`, string(status), updatedBy, id)
	if err != nil {
		return fmt.Errorf("repo.UpdateWorkflowStatus bobot_skenario: %w", err)
	}
	return nil
}

// SoftDelete marks the record as deleted.
func (r *DBRepository) SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*BobotSkenario, error) {
	now := time.Now()
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.bobot_skenario
		SET deleted_at = $1, deleted_by = $2, updated_at = $1, updated_by = $2
		WHERE id = $3 AND deleted_at IS NULL
	`, now, deletedBy, id)
	if err != nil {
		return nil, fmt.Errorf("repo.SoftDelete bobot_skenario: %w", err)
	}
	return r.getOne(ctx, tx, "id = $1", id, true)
}

// CountReferences counts active references that would block soft-delete.
//
// Phase 3 status: ECL engine tables currently do not reference mst.bobot_skenario by FK.
// Weights are applied by value at calc-run time. This function probes for any ecl.* table
// with a bobot_skenario_id column. If none exists, returns 0.
//
// TODO (Phase 5): replace with explicit join when FK-bearing table is introduced.
func (r *DBRepository) CountReferences(ctx context.Context, id uuid.UUID) (int64, error) {
	var refTable sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT table_schema || '.' || table_name
		FROM information_schema.columns
		WHERE table_schema = 'ecl'
		  AND column_name  = 'bobot_skenario_id'
		LIMIT 1
	`).Scan(&refTable)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("repo.CountReferences probe bobot_skenario: %w", err)
	}
	if !refTable.Valid {
		return 0, nil
	}

	// #nosec G201 -- refTable from information_schema (trusted catalog source).
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE bobot_skenario_id = $1", refTable.String) //nolint:gosec
	var count int64
	if err := r.db.QueryRowContext(ctx, q, id).Scan(&count); err != nil {
		return 0, fmt.Errorf("repo.CountReferences bobot_skenario: %w", err)
	}
	return count, nil
}

// CountOverlap counts rows with the same skenario whose period overlaps [dari, sampai).
// A null sampai is treated as open-ended (infinity).
// Overlap condition (half-open intervals):
//
//	existing.dari < candidate.sampai (or candidate.sampai IS NULL)
//	AND (existing.sampai IS NULL OR existing.sampai > candidate.dari)
func (r *DBRepository) CountOverlap(ctx context.Context, skenario Skenario, dari string, sampai *string, excludeID uuid.UUID) (int64, error) {
	var count int64
	var err error

	if sampai == nil || *sampai == "" {
		err = r.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM mst.bobot_skenario
			WHERE skenario = $1
			  AND deleted_at IS NULL
			  AND id <> $2
			  AND (periode_berlaku_sampai IS NULL OR periode_berlaku_sampai > $3)
		`, string(skenario), excludeID, dari).Scan(&count)
	} else {
		err = r.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM mst.bobot_skenario
			WHERE skenario = $1
			  AND deleted_at IS NULL
			  AND id <> $2
			  AND periode_berlaku_dari < $4
			  AND (periode_berlaku_sampai IS NULL OR periode_berlaku_sampai > $3)
		`, string(skenario), excludeID, dari, *sampai).Scan(&count)
	}
	if err != nil {
		return 0, fmt.Errorf("repo.CountOverlap bobot_skenario: %w", err)
	}
	return count, nil
}

// CountDuplicate counts rows with the exact same (skenario, dari, sampai) tuple.
// Used to enforce the single-skenario-per-period rule.
func (r *DBRepository) CountDuplicate(ctx context.Context, skenario Skenario, dari string, sampai *string, excludeID uuid.UUID) (int64, error) {
	var count int64
	var err error

	if sampai == nil || *sampai == "" {
		err = r.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM mst.bobot_skenario
			WHERE skenario = $1
			  AND deleted_at IS NULL
			  AND id <> $2
			  AND periode_berlaku_dari = $3
			  AND periode_berlaku_sampai IS NULL
		`, string(skenario), excludeID, dari).Scan(&count)
	} else {
		err = r.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM mst.bobot_skenario
			WHERE skenario = $1
			  AND deleted_at IS NULL
			  AND id <> $2
			  AND periode_berlaku_dari = $3
			  AND periode_berlaku_sampai = $4
		`, string(skenario), excludeID, dari, *sampai).Scan(&count)
	}
	if err != nil {
		return 0, fmt.Errorf("repo.CountDuplicate bobot_skenario: %w", err)
	}
	return count, nil
}

// CountByPeriod counts active (non-deleted) rows for the given (dari, sampai) period.
// Used to check if all 3 skenarios already exist for a period (seed-default idempotency).
func (r *DBRepository) CountByPeriod(ctx context.Context, dari string, sampai *string) (int64, error) {
	var count int64
	var err error

	if sampai == nil || *sampai == "" {
		err = r.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM mst.bobot_skenario
			WHERE deleted_at IS NULL
			  AND periode_berlaku_dari = $1
			  AND periode_berlaku_sampai IS NULL
		`, dari).Scan(&count)
	} else {
		err = r.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM mst.bobot_skenario
			WHERE deleted_at IS NULL
			  AND periode_berlaku_dari = $1
			  AND periode_berlaku_sampai = $2
		`, dari, *sampai).Scan(&count)
	}
	if err != nil {
		return 0, fmt.Errorf("repo.CountByPeriod bobot_skenario: %w", err)
	}
	return count, nil
}

// activeWorkflowStatuses is the set of workflow_status values included in the sum invariant.
// REJECTED rows are excluded — they have been definitively rejected and must not count
// toward the G+N+B = 1.0 check (MINOR fix per compliance review).
const activeWorkflowStatuses = `
	AND workflow_status IN ('DRAFT','PENDING_REVIEW','PENDING_APPROVAL','PENDING_APPROVAL_2','APPROVED')`

// SumByPeriod returns the sum of bobot for all active, non-REJECTED rows in a period,
// excluding the given excludeID (for update checks).
func (r *DBRepository) SumByPeriod(ctx context.Context, dari string, sampai *string, excludeID uuid.UUID) (decimal.Decimal, error) {
	return r.sumByPeriodQuerier(ctx, r.db, dari, sampai, excludeID)
}

// SumByPeriodTx runs SumByPeriod inside the given transaction (used by WorkflowHook).
func (r *DBRepository) SumByPeriodTx(ctx context.Context, tx *sql.Tx, dari string, sampai *string, excludeID uuid.UUID) (decimal.Decimal, error) {
	return r.sumByPeriodQuerier(ctx, tx, dari, sampai, excludeID)
}

// rowQuerier is satisfied by both *sql.DB and *sql.Tx.
type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// sumByPeriodQuerier is the shared implementation for SumByPeriod / SumByPeriodTx.
func (r *DBRepository) sumByPeriodQuerier(ctx context.Context, q rowQuerier, dari string, sampai *string, excludeID uuid.UUID) (decimal.Decimal, error) {
	var sumStr sql.NullString
	var err error

	if sampai == nil || *sampai == "" {
		err = q.QueryRowContext(ctx, `
			SELECT COALESCE(SUM(bobot)::TEXT, '0') FROM mst.bobot_skenario
			WHERE deleted_at IS NULL
			  AND id <> $1
			  AND periode_berlaku_dari = $2
			  AND periode_berlaku_sampai IS NULL`+
			activeWorkflowStatuses,
			excludeID, dari).Scan(&sumStr)
	} else {
		err = q.QueryRowContext(ctx, `
			SELECT COALESCE(SUM(bobot)::TEXT, '0') FROM mst.bobot_skenario
			WHERE deleted_at IS NULL
			  AND id <> $1
			  AND periode_berlaku_dari = $2
			  AND periode_berlaku_sampai = $3`+
			activeWorkflowStatuses,
			excludeID, dari, *sampai).Scan(&sumStr)
	}
	if err != nil {
		return decimal.Zero, fmt.Errorf("repo.SumByPeriod bobot_skenario: %w", err)
	}
	if !sumStr.Valid || sumStr.String == "" {
		return decimal.Zero, nil
	}
	d, parseErr := decimal.NewFromString(sumStr.String)
	if parseErr != nil {
		return decimal.Zero, fmt.Errorf("repo.SumByPeriod parse sum: %w", parseErr)
	}
	return d, nil
}

// UpdateWorkflowStatusTx updates only the workflow_status column inside an existing transaction.
// Used by WorkflowHook.BeforeCommit to keep mst.bobot_skenario in sync atomically.
func (r *DBRepository) UpdateWorkflowStatusTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.bobot_skenario
		SET workflow_status = $1, updated_at = now(), updated_by = $2, row_version = row_version + 1
		WHERE id = $3
	`, string(status), updatedBy, id)
	if err != nil {
		return fmt.Errorf("repo.UpdateWorkflowStatusTx bobot_skenario: %w", err)
	}
	return nil
}

// BeginTx starts a transaction.
func (r *DBRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("repo.BeginTx bobot_skenario: %w", err)
	}
	return tx, nil
}

// ListAuditHistory returns paginated audit events for the given bobot_skenario entity UUID.
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

	var args []interface{}
	args = append(args, entityID, "mst.bobot_skenario")
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
		return nil, false, fmt.Errorf("repo.ListAuditHistory bobot_skenario: %w", err)
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
			return nil, false, fmt.Errorf("repo.ListAuditHistory scan: %w", err)
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
		return nil, false, fmt.Errorf("repo.ListAuditHistory rows.Err: %w", err)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	return items, hasMore, nil
}

// ExportAll streams all records matching q as UTF-8 BOM CSV.
func (r *DBRepository) ExportAll(ctx context.Context, q listquery.Query) (io.Reader, int, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("t")

	conditions := []string{"t.deleted_at IS NULL"}
	if where != "" {
		conditions = append(conditions, where)
	}
	if q.Search != "" {
		argIdx := len(args) + 1
		searchCond := make([]string, 0, len(SearchCols))
		for _, col := range SearchCols {
			if col == "catatan" {
				searchCond = append(searchCond, fmt.Sprintf("t.%s ILIKE $%d", col, argIdx))
			} else {
				searchCond = append(searchCond, fmt.Sprintf("t.%s::text ILIKE $%d", col, argIdx))
			}
		}
		conditions = append(conditions, "("+strings.Join(searchCond, " OR ")+")")
		args = append(args, "%"+q.Search+"%")
	}

	whereClause := " WHERE " + strings.Join(conditions, " AND ")
	if orderBy == "" {
		orderBy = "t.skenario ASC, t.periode_berlaku_dari DESC"
	}

	query := fmt.Sprintf( //nolint:gosec
		"SELECT t.id, t.skenario, t.bobot, t.periode_berlaku_dari, t.periode_berlaku_sampai, t.workflow_status FROM mst.bobot_skenario t%s ORDER BY %s",
		whereClause, orderBy,
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll bobot_skenario: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM for Excel
	w := csv.NewWriter(&buf)

	headers := []string{"ID", "Skenario", "Bobot", "Berlaku Dari", "Berlaku Sampai", "Workflow Status"}
	if err := w.Write(headers); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll write header: %w", err)
	}

	count := 0
	for rows.Next() {
		var (
			id       uuid.UUID
			skenario string
			bobotStr string
			dari     string
			sampai   *string
			wfStatus string
		)
		if err := rows.Scan(&id, &skenario, &bobotStr, &dari, &sampai, &wfStatus); err != nil {
			return nil, 0, fmt.Errorf("repo.ExportAll scan: %w", err)
		}
		sampaiStr := ""
		if sampai != nil {
			sampaiStr = *sampai
		}
		record := []string{id.String(), skenario, bobotStr, dari, sampaiStr, wfStatus}
		if err := w.Write(record); err != nil {
			return nil, 0, fmt.Errorf("repo.ExportAll write record: %w", err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll rows.Err: %w", err)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll flush: %w", err)
	}
	return &buf, count, nil
}

// ─── Scan helpers ─────────────────────────────────────────────────────────────

func scanBobotSkenarioRow(row *sql.Row) (*BobotSkenario, error) {
	e := &BobotSkenario{}
	var (
		skenarioStr    string
		bobotStr       string
		workflowStatus string
		sampai         *string
		createdBy      *uuid.UUID
		updatedAt      *time.Time
		updatedBy      *uuid.UUID
		deletedAt      *time.Time
		deletedBy      *uuid.UUID
		approverID     *uuid.UUID
		approvedAt     *time.Time
		makerID        uuid.UUID
	)
	err := row.Scan(
		&e.ID,
		&skenarioStr,
		&bobotStr,
		&e.Catatan,
		&e.PeriodeBerlakuDari,
		&sampai,
		&makerID,
		&approverID,
		&approvedAt,
		&workflowStatus,
		&e.CreatedAt,
		&createdBy,
		&updatedAt,
		&updatedBy,
		&deletedAt,
		&deletedBy,
		&e.RowVersion,
		&e.TenantID,
	)
	if err != nil {
		return nil, err
	}
	fillScanned(e, skenarioStr, bobotStr, workflowStatus, sampai, createdBy, updatedAt, updatedBy, deletedAt, deletedBy, approverID, approvedAt, makerID)
	return e, nil
}

func scanBobotSkenarioRows(rows *sql.Rows) (*BobotSkenario, error) {
	e := &BobotSkenario{}
	var (
		skenarioStr    string
		bobotStr       string
		workflowStatus string
		sampai         *string
		createdBy      *uuid.UUID
		updatedAt      *time.Time
		updatedBy      *uuid.UUID
		deletedAt      *time.Time
		deletedBy      *uuid.UUID
		approverID     *uuid.UUID
		approvedAt     *time.Time
		makerID        uuid.UUID
	)
	err := rows.Scan(
		&e.ID,
		&skenarioStr,
		&bobotStr,
		&e.Catatan,
		&e.PeriodeBerlakuDari,
		&sampai,
		&makerID,
		&approverID,
		&approvedAt,
		&workflowStatus,
		&e.CreatedAt,
		&createdBy,
		&updatedAt,
		&updatedBy,
		&deletedAt,
		&deletedBy,
		&e.RowVersion,
		&e.TenantID,
	)
	if err != nil {
		return nil, err
	}
	fillScanned(e, skenarioStr, bobotStr, workflowStatus, sampai, createdBy, updatedAt, updatedBy, deletedAt, deletedBy, approverID, approvedAt, makerID)
	return e, nil
}

func fillScanned(
	e *BobotSkenario,
	skenarioStr, bobotStr, workflowStatus string,
	sampai *string,
	createdBy *uuid.UUID,
	updatedAt *time.Time,
	updatedBy *uuid.UUID,
	deletedAt *time.Time,
	deletedBy *uuid.UUID,
	approverID *uuid.UUID,
	approvedAt *time.Time,
	makerID uuid.UUID,
) {
	e.Skenario = Skenario(skenarioStr)
	e.WorkflowStatus = WorkflowStatus(workflowStatus)
	e.PeriodeBerlakuSampai = sampai
	e.CreatedBy = createdBy
	e.UpdatedAt = updatedAt
	e.UpdatedBy = updatedBy
	e.DeletedAt = deletedAt
	e.DeletedBy = deletedBy
	e.ApproverID = approverID
	e.ApprovedAt = approvedAt
	e.MakerID = makerID

	// Parse bobot decimal — safe from DB because CHECK constraint BETWEEN 0 AND 1.
	d, err := decimal.NewFromString(bobotStr)
	if err != nil {
		e.Bobot = decimal.Zero
	} else {
		e.Bobot = d
	}
}

// ─── Sentinel errors ──────────────────────────────────────────────────────────

// ErrNotFound is returned when a record is not found.
var ErrNotFound = fmt.Errorf("bobot_skenario not found")

// ErrConflict is returned on row_version mismatch.
var ErrConflict = fmt.Errorf("optimistic lock conflict")
