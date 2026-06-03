package lgdbasel

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

// Repository defines the data-access contract for lgd_basel.
// Service layer only uses this interface — no SQL in service or handler.
type Repository interface {
	// Create inserts a new lgd_basel row in the given transaction.
	Create(ctx context.Context, tx *sql.Tx, e *LGDBasel) error

	// GetByID fetches one record by surrogate UUID.
	// Returns (nil, nil) if not found (soft-deleted returned if includeDeleted=true).
	GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*LGDBasel, error)

	// List fetches paginated records matching listquery + cursor.
	// Returns rows limited to limit+1 (caller detects hasMore).
	List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*LGDBasel, error)

	// Update applies partial update in the given transaction.
	// Uses optimistic lock: UPDATE … WHERE row_version = expected AND id = id.
	// Returns ErrNotFound if id not found; ErrConflict if row_version mismatch.
	Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, f UpdateFields) (*LGDBasel, error)

	// SoftDelete sets deleted_at/deleted_by in the given transaction.
	SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*LGDBasel, error)

	// UpdateWorkflowStatus updates workflow_status (called after workflow engine transitions).
	UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error

	// CountReferences returns the number of active ECL calc-result lines that reference
	// this lgd_basel row. Used by the delete guard.
	CountReferences(ctx context.Context, id uuid.UUID) (int64, error)

	// CountOverlap counts rows of the same tipe_eksposur whose [dari, sampai) period
	// overlaps the candidate period. Excludes the given excludeID (for update checks).
	// A null sampai is treated as an open-ended period.
	CountOverlap(ctx context.Context, tipe TipeEksposur, dari string, sampai *string, excludeID uuid.UUID) (int64, error)

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
	TipeEksposur         *TipeEksposur
	LGD                  *decimal.Decimal
	Karakteristik        *string
	PeriodeBerlakuDari   *string
	PeriodeBerlakuSampai *string // nil = leave unchanged; pointer to empty string = set to NULL
	Sumber               *string
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
// NOTE: workflow_instance_id is not stored in mst.lgd_basel; it is derived from
// sys.workflow_instance. We omit it from direct DB select and leave it nil in the
// scanned struct (handler/service can look it up from workflow engine if needed).
const baseSelectCols = `
	id, tipe_eksposur, lgd, karakteristik,
	periode_berlaku_dari, periode_berlaku_sampai, sumber,
	dokumen_pendukung_id, maker_id, approver_id,
	workflow_status,
	created_at, created_by, updated_at, updated_by,
	deleted_at, deleted_by, row_version, tenant_id`

const baseSelectFrom = `FROM mst.lgd_basel`

// Create inserts a new row. Called inside a transaction.
func (r *DBRepository) Create(ctx context.Context, tx *sql.Tx, e *LGDBasel) error {
	query := `
INSERT INTO mst.lgd_basel (
	id, tipe_eksposur, lgd, karakteristik,
	periode_berlaku_dari, periode_berlaku_sampai, sumber,
	dokumen_pendukung_id, maker_id, approver_id,
	workflow_status,
	created_at, created_by, updated_at, updated_by,
	row_version, tenant_id
) VALUES (
	$1, $2, $3, $4,
	$5, $6, $7,
	$8, $9, $10,
	$11,
	$12, $13, $12, $13,
	1, $14
)`
	_, err := tx.ExecContext(ctx, query,
		e.ID,
		string(e.TipeEksposur),
		e.LGD.StringFixed(8), // store with full 8dp precision
		e.Karakteristik,
		e.PeriodeBerlakuDari,
		e.PeriodeBerlakuSampai,
		e.Sumber,
		e.DokumenPendukungID,
		e.MakerID,    // legacy column: NOT NULL in DB, set = currentUser (see service comment)
		e.ApproverID, // nullable legacy column
		string(e.WorkflowStatus),
		e.CreatedAt,
		e.CreatedBy,
		e.TenantID,
	)
	if err != nil {
		return fmt.Errorf("repo.Create lgd_basel: %w", err)
	}
	return nil
}

// GetByID fetches by surrogate UUID.
func (r *DBRepository) GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*LGDBasel, error) {
	return r.getOne(ctx, r.db, "id = $1", id, includeDeleted)
}

type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

func (r *DBRepository) getOne(ctx context.Context, q querier, where string, arg interface{}, includeDeleted bool) (*LGDBasel, error) {
	deletedFilter := " AND deleted_at IS NULL"
	if includeDeleted {
		deletedFilter = ""
	}
	query := "SELECT" + baseSelectCols + " " + baseSelectFrom + " WHERE " + where + deletedFilter
	row := q.QueryRowContext(ctx, query, arg)
	e, err := scanLGDBaselRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.getOne lgd_basel: %w", err)
	}
	return e, nil
}

// List fetches paginated records.
func (r *DBRepository) List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*LGDBasel, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("t")

	conditions := []string{}
	if !includeDeleted {
		conditions = append(conditions, "t.deleted_at IS NULL")
	}

	// Cursor: UUID-based ordering on (created_at, id).
	if cursor != "" {
		cd, err := pagination.DecodeCursor(cursor)
		if err == nil && cd.ID != "" {
			argIdx := len(args) + 1
			conditions = append(conditions, fmt.Sprintf("t.id::text > $%d", argIdx))
			args = append(args, cd.ID)
		}
	}

	// Text search.
	if q.Search != "" {
		argIdx := len(args) + 1
		searchCond := make([]string, 0, len(SearchCols))
		for _, col := range SearchCols {
			if col == "karakteristik" {
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
		orderBy = "t.tipe_eksposur ASC, t.periode_berlaku_dari DESC"
	}

	argIdx := len(args) + 1
	query := fmt.Sprintf( //nolint:gosec
		"SELECT t.id, t.tipe_eksposur, t.lgd, t.karakteristik, t.periode_berlaku_dari, t.periode_berlaku_sampai, t.sumber, t.dokumen_pendukung_id, t.maker_id, t.approver_id, t.workflow_status, t.created_at, t.created_by, t.updated_at, t.updated_by, t.deleted_at, t.deleted_by, t.row_version, t.tenant_id FROM mst.lgd_basel t%s ORDER BY %s LIMIT $%d",
		whereClause, orderBy, argIdx,
	)
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.List lgd_basel: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []*LGDBasel
	for rows.Next() {
		e, err := scanLGDBaselRows(rows)
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
func (r *DBRepository) Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, f UpdateFields) (*LGDBasel, error) {
	setClauses := []string{}
	args := []interface{}{}
	idx := 1

	if f.TipeEksposur != nil {
		setClauses = append(setClauses, fmt.Sprintf("tipe_eksposur = $%d", idx))
		args = append(args, string(*f.TipeEksposur))
		idx++
	}
	if f.LGD != nil {
		setClauses = append(setClauses, fmt.Sprintf("lgd = $%d", idx))
		args = append(args, f.LGD.StringFixed(8))
		idx++
	}
	if f.Karakteristik != nil {
		setClauses = append(setClauses, fmt.Sprintf("karakteristik = $%d", idx))
		args = append(args, *f.Karakteristik)
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
	if f.Sumber != nil {
		setClauses = append(setClauses, fmt.Sprintf("sumber = $%d", idx))
		args = append(args, *f.Sumber)
		idx++
	}
	if len(setClauses) == 0 {
		// Nothing to update — return current record.
		return r.GetByID(ctx, id, false)
	}

	// Append audit + optimistic lock columns.
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
		`UPDATE mst.lgd_basel SET %s WHERE id = $%d AND row_version = $%d AND deleted_at IS NULL`,
		strings.Join(setClauses, ", "),
		whereIDIdx, whereVersionIdx,
	)

	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.Update lgd_basel: %w", err)
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
		UPDATE mst.lgd_basel
		SET workflow_status = $1, updated_at = now(), updated_by = $2, row_version = row_version + 1
		WHERE id = $3
	`, string(status), updatedBy, id)
	if err != nil {
		return fmt.Errorf("repo.UpdateWorkflowStatus lgd_basel: %w", err)
	}
	return nil
}

// SoftDelete marks the record as deleted.
func (r *DBRepository) SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*LGDBasel, error) {
	now := time.Now()
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.lgd_basel
		SET deleted_at = $1, deleted_by = $2, updated_at = $1, updated_by = $2
		WHERE id = $3 AND deleted_at IS NULL
	`, now, deletedBy, id)
	if err != nil {
		return nil, fmt.Errorf("repo.SoftDelete lgd_basel: %w", err)
	}
	return r.getOne(ctx, tx, "id = $1", id, true)
}

// CountReferences counts active references that would block soft-delete.
//
// Phase 3 status: ECL engine tables (ecl.calc_header / ecl.calc_detail_skenario
// from 0001) currently store LGD by VALUE COPY (NUMERIC column), not by FK to
// mst.lgd_basel(id). There is no lgd_pool_id FK in any current migration, so
// FK-based reference counting is not possible until APP-C Phase 5 introduces
// the snapshot/FK link (planned migration 0020+).
//
// Pragmatic Phase 3 behavior: probe pg_catalog for any table column named
// 'lgd_pool_id'. If none exists, return 0 (FK-based references not yet
// modeled). When the FK column is introduced in a later migration, this
// function will start counting automatically without code change.
//
// TODO (Phase 5, post APP-C build): replace with explicit join to
// ecl.ecl_calc_result_line or whatever final FK-bearing table is introduced.
// Ref: docs/audit/COMPLIANCE-lgd-basel-*.md BLOCKER 2.
func (r *DBRepository) CountReferences(ctx context.Context, id uuid.UUID) (int64, error) {
	// Probe for any ecl.* table with a lgd_pool_id column.
	var refTable sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT table_schema || '.' || table_name
		FROM information_schema.columns
		WHERE table_schema = 'ecl'
		  AND column_name  = 'lgd_pool_id'
		LIMIT 1
	`).Scan(&refTable)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("repo.CountReferences probe lgd_basel: %w", err)
	}
	if !refTable.Valid {
		// No FK column exists yet — Phase 3 returns 0 (no ECL-engine
		// references can exist by definition until APP-C schema lands).
		return 0, nil
	}

	// Quote identifier safely: refTable comes from information_schema (trusted
	// pg_catalog source, not user input — safe to interpolate into SQL).
	// #nosec G201 -- refTable origin is information_schema, sanitised by Postgres.
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE lgd_pool_id = $1", refTable.String) //nolint:gosec
	var count int64
	if err := r.db.QueryRowContext(ctx, q, id).Scan(&count); err != nil {
		return 0, fmt.Errorf("repo.CountReferences lgd_basel: %w", err)
	}
	return count, nil
}

// CountOverlap counts rows with the same tipe_eksposur whose period overlaps [dari, sampai).
// A null sampai is treated as open-ended (infinity).
// Overlap condition (using half-open intervals):
//
//	existing.dari < candidate.sampai (or candidate.sampai IS NULL)
//	AND (existing.sampai IS NULL OR existing.sampai > candidate.dari)
//
// excludeID is the ID of the row being updated (so it doesn't overlap with itself).
func (r *DBRepository) CountOverlap(ctx context.Context, tipe TipeEksposur, dari string, sampai *string, excludeID uuid.UUID) (int64, error) {
	var count int64
	var err error

	// sampai=nil means open-ended; candidate covers from dari to infinity.
	if sampai == nil || *sampai == "" {
		// Candidate is open-ended: overlaps any existing entry that starts before "now"
		// and is open-ended or extends beyond dari.
		err = r.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM mst.lgd_basel
			WHERE tipe_eksposur = $1
			  AND deleted_at IS NULL
			  AND id <> $2
			  AND (periode_berlaku_sampai IS NULL OR periode_berlaku_sampai > $3)
		`, string(tipe), excludeID, dari).Scan(&count)
	} else {
		// Candidate has a definite end date.
		err = r.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM mst.lgd_basel
			WHERE tipe_eksposur = $1
			  AND deleted_at IS NULL
			  AND id <> $2
			  AND periode_berlaku_dari < $4
			  AND (periode_berlaku_sampai IS NULL OR periode_berlaku_sampai > $3)
		`, string(tipe), excludeID, dari, *sampai).Scan(&count)
	}
	if err != nil {
		return 0, fmt.Errorf("repo.CountOverlap lgd_basel: %w", err)
	}
	return count, nil
}

// BeginTx starts a transaction.
func (r *DBRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("repo.BeginTx lgd_basel: %w", err)
	}
	return tx, nil
}

// ListAuditHistory returns paginated audit events for the given lgd_basel entity UUID.
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
	args = append(args, entityID, "mst.lgd_basel")
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
		return nil, false, fmt.Errorf("repo.ListAuditHistory lgd_basel: %w", err)
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
			if col == "karakteristik" {
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
		orderBy = "t.tipe_eksposur ASC, t.periode_berlaku_dari DESC"
	}

	query := fmt.Sprintf( //nolint:gosec
		"SELECT t.id, t.tipe_eksposur, t.lgd, t.periode_berlaku_dari, t.periode_berlaku_sampai, t.sumber, t.workflow_status FROM mst.lgd_basel t%s ORDER BY %s",
		whereClause, orderBy,
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll lgd_basel: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM for Excel
	w := csv.NewWriter(&buf)

	headers := []string{"ID", "Tipe Eksposur", "LGD", "Berlaku Dari", "Berlaku Sampai", "Sumber", "Workflow Status"}
	if err := w.Write(headers); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll write header: %w", err)
	}

	count := 0
	for rows.Next() {
		var (
			id       uuid.UUID
			tipe     string
			lgdStr   string
			dari     string
			sampai   *string
			sumber   string
			wfStatus string
		)
		if err := rows.Scan(&id, &tipe, &lgdStr, &dari, &sampai, &sumber, &wfStatus); err != nil {
			return nil, 0, fmt.Errorf("repo.ExportAll scan: %w", err)
		}
		sampaiStr := ""
		if sampai != nil {
			sampaiStr = *sampai
		}
		record := []string{id.String(), tipe, lgdStr, dari, sampaiStr, sumber, wfStatus}
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

// scanLGDBaselRow scans one *sql.Row into LGDBasel.
func scanLGDBaselRow(row *sql.Row) (*LGDBasel, error) {
	e := &LGDBasel{}
	var (
		tipeStr        string
		lgdStr         string
		workflowStatus string
		sampai         *string
		createdBy      *uuid.UUID
		updatedAt      *time.Time
		updatedBy      *uuid.UUID
		deletedAt      *time.Time
		deletedBy      *uuid.UUID
		approverID     *uuid.UUID
		dokumenID      *uuid.UUID
		makerID        uuid.UUID
	)
	err := row.Scan(
		&e.ID,
		&tipeStr,
		&lgdStr,
		&e.Karakteristik,
		&e.PeriodeBerlakuDari,
		&sampai,
		&e.Sumber,
		&dokumenID,
		&makerID,
		&approverID,
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
	fillScanned(e, tipeStr, lgdStr, workflowStatus, sampai, createdBy, updatedAt, updatedBy, deletedAt, deletedBy, approverID, dokumenID, makerID)
	return e, nil
}

// scanLGDBaselRows scans one *sql.Rows row into LGDBasel.
func scanLGDBaselRows(rows *sql.Rows) (*LGDBasel, error) {
	e := &LGDBasel{}
	var (
		tipeStr        string
		lgdStr         string
		workflowStatus string
		sampai         *string
		createdBy      *uuid.UUID
		updatedAt      *time.Time
		updatedBy      *uuid.UUID
		deletedAt      *time.Time
		deletedBy      *uuid.UUID
		approverID     *uuid.UUID
		dokumenID      *uuid.UUID
		makerID        uuid.UUID
	)
	err := rows.Scan(
		&e.ID,
		&tipeStr,
		&lgdStr,
		&e.Karakteristik,
		&e.PeriodeBerlakuDari,
		&sampai,
		&e.Sumber,
		&dokumenID,
		&makerID,
		&approverID,
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
	fillScanned(e, tipeStr, lgdStr, workflowStatus, sampai, createdBy, updatedAt, updatedBy, deletedAt, deletedBy, approverID, dokumenID, makerID)
	return e, nil
}

func fillScanned(
	e *LGDBasel,
	tipeStr, lgdStr, workflowStatus string,
	sampai *string,
	createdBy *uuid.UUID,
	updatedAt *time.Time,
	updatedBy *uuid.UUID,
	deletedAt *time.Time,
	deletedBy *uuid.UUID,
	approverID *uuid.UUID,
	dokumenID *uuid.UUID,
	makerID uuid.UUID,
) {
	e.TipeEksposur = TipeEksposur(tipeStr)
	e.WorkflowStatus = WorkflowStatus(workflowStatus)
	e.PeriodeBerlakuSampai = sampai
	e.CreatedBy = createdBy
	e.UpdatedAt = updatedAt
	e.UpdatedBy = updatedBy
	e.DeletedAt = deletedAt
	e.DeletedBy = deletedBy
	e.ApproverID = approverID
	e.DokumenPendukungID = dokumenID
	e.MakerID = makerID

	// Parse LGD decimal — safe from DB because CHECK constraint BETWEEN 0 AND 1.
	d, err := decimal.NewFromString(lgdStr)
	if err != nil {
		// Fallback: store as-is with zero decimal.
		e.LGD = decimal.Zero
	} else {
		e.LGD = d
	}
}

// ─── Sentinel errors ──────────────────────────────────────────────────────────

// ErrNotFound is returned when a record is not found.
var ErrNotFound = fmt.Errorf("lgd_basel not found")

// ErrConflict is returned on row_version mismatch.
var ErrConflict = fmt.Errorf("optimistic lock conflict")
