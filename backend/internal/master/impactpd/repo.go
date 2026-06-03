package impactpd

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

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
)

// Repository defines data-access for mst.impact_pd.
type Repository interface {
	// Create inserts a new row inside the provided transaction.
	Create(ctx context.Context, tx *sql.Tx, m *ImpactPD) error

	// GetByID fetches one record by UUID. Returns (nil, nil) if not found.
	GetByID(ctx context.Context, id uuid.UUID) (*ImpactPD, error)

	// List returns paginated rows (limit+1 for hasMore detection).
	List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*ImpactPD, error)

	// Update applies partial changes with optimistic lock.
	// Returns ErrNotFound or ErrConflict sentinels.
	Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, f UpdateFields) (*ImpactPD, error)

	// SoftDelete sets deleted_at/deleted_by inside the provided transaction.
	SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*ImpactPD, error)

	// UpdateWorkflowStatus updates the workflow_status column.
	UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error

	// CountByPeriode returns the number of active rows for the given periode_id.
	// Used for UNIQUE guard.
	CountByPeriode(ctx context.Context, periodeID uuid.UUID) (int64, error)

	// CountByPeriodeExcluding is like CountByPeriode but excludes the row with excludeID.
	CountByPeriodeExcluding(ctx context.Context, periodeID uuid.UUID, excludeID uuid.UUID) (int64, error)

	// BeginTx starts a read-committed database transaction.
	BeginTx(ctx context.Context) (*sql.Tx, error)

	// ListAuditHistory returns paginated audit_log rows for the entity.
	ListAuditHistory(ctx context.Context, entityID uuid.UUID, cursor string, limit int, isAuditRole bool) ([]AuditHistoryItem, bool, error)

	// ExportAll returns all rows matching q as UTF-8 BOM CSV.
	ExportAll(ctx context.Context, q listquery.Query) (io.Reader, int, error)
}

// UpdateFields captures mutable columns for Update.
type UpdateFields struct {
	ImpactMultiplier *decimal.Decimal
	Catatan          *string
	UpdatedBy        uuid.UUID
	ExpectedVersion  int64
}

// ─── Sentinel errors ──────────────────────────────────────────────────────────

var (
	// ErrNotFound is returned when the record does not exist.
	ErrNotFound = fmt.Errorf("impact_pd not found")
	// ErrConflict is returned on row_version mismatch.
	ErrConflict = fmt.Errorf("optimistic lock conflict")
)

// ─── DB implementation ────────────────────────────────────────────────────────

// DBRepository is the production SQL implementation.
type DBRepository struct {
	db *sql.DB
}

// NewDBRepository creates a DBRepository.
func NewDBRepository(db *sql.DB) *DBRepository {
	return &DBRepository{db: db}
}

var _ Repository = (*DBRepository)(nil)

const baseSelect = `
SELECT
    id, periode_id,
    impact_multiplier, catatan,
    workflow_status,
    created_at, created_by, updated_at, updated_by,
    deleted_at, deleted_by, row_version, tenant_id
FROM mst.impact_pd`

// Create inserts a new row.
func (r *DBRepository) Create(ctx context.Context, tx *sql.Tx, m *ImpactPD) error {
	query := `
INSERT INTO mst.impact_pd (
    id, periode_id,
    impact_multiplier, catatan,
    workflow_status,
    created_at, created_by, updated_at, updated_by,
    row_version, tenant_id
) VALUES (
    $1, $2,
    $3, $4,
    $5,
    $6, $7, $6, $7,
    1, $8
)`
	_, err := tx.ExecContext(ctx, query,
		m.ID, m.PeriodeID,
		m.ImpactMultiplier.String(), m.Catatan,
		string(m.WorkflowStatus),
		m.CreatedAt, m.CreatedBy,
		m.TenantID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("unique_violation: %w", ErrConflict)
		}
		return fmt.Errorf("repo.Create impact_pd: %w", err)
	}
	return nil
}

// GetByID fetches one record.
func (r *DBRepository) GetByID(ctx context.Context, id uuid.UUID) (*ImpactPD, error) {
	query := baseSelect + " WHERE id = $1 AND deleted_at IS NULL"
	row := r.db.QueryRowContext(ctx, query, id)
	m, err := scanOne(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.GetByID impact_pd: %w", err)
	}
	return m, nil
}

// List fetches paginated records.
func (r *DBRepository) List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*ImpactPD, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("t")

	var conditions []string
	if !includeDeleted {
		conditions = append(conditions, "t.deleted_at IS NULL")
	}

	if cursor != "" {
		cd, err := pagination.DecodeCursor(cursor)
		if err == nil && cd.ID != "" {
			argIdx := len(args) + 1
			conditions = append(conditions, fmt.Sprintf("t.id > $%d", argIdx))
			args = append(args, cd.ID)
		}
	}

	if q.Search != "" {
		argIdx := len(args) + 1
		var sc []string
		for _, col := range SearchCols {
			sc = append(sc, fmt.Sprintf("t.%s ILIKE $%d", col, argIdx))
		}
		conditions = append(conditions, "("+strings.Join(sc, " OR ")+")")
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
		orderBy = "t.created_at DESC"
	}

	argIdx := len(args) + 1
	query := fmt.Sprintf( //nolint:gosec
		`SELECT t.id, t.periode_id,
		    t.impact_multiplier, t.catatan,
		    t.workflow_status,
		    t.created_at, t.created_by, t.updated_at, t.updated_by,
		    t.deleted_at, t.deleted_by, t.row_version, t.tenant_id
		 FROM mst.impact_pd t%s ORDER BY %s LIMIT $%d`,
		whereClause, orderBy, argIdx,
	)
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.List impact_pd: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []*ImpactPD
	for rows.Next() {
		m, err := scanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("repo.List scan impact_pd: %w", err)
		}
		items = append(items, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo.List rows.Err impact_pd: %w", err)
	}
	return items, nil
}

// Update applies partial changes with optimistic lock.
func (r *DBRepository) Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, f UpdateFields) (*ImpactPD, error) {
	var setClauses []string
	var args []interface{}
	idx := 1

	if f.ImpactMultiplier != nil {
		setClauses = append(setClauses, fmt.Sprintf("impact_multiplier = $%d", idx))
		args = append(args, f.ImpactMultiplier.String())
		idx++
	}
	if f.Catatan != nil {
		setClauses = append(setClauses, fmt.Sprintf("catatan = $%d", idx))
		args = append(args, *f.Catatan)
		idx++
	}
	if len(setClauses) == 0 {
		return r.GetByID(ctx, id)
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
		`UPDATE mst.impact_pd SET %s WHERE id = $%d AND row_version = $%d AND deleted_at IS NULL`,
		strings.Join(setClauses, ", "),
		whereIDIdx, whereVersionIdx,
	)

	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.Update impact_pd: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("repo.Update rows affected: %w", err)
	}
	if n == 0 {
		existing, err := r.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, ErrNotFound
		}
		return nil, ErrConflict
	}
	return r.GetByID(ctx, id)
}

// SoftDelete marks the record as deleted.
func (r *DBRepository) SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*ImpactPD, error) {
	now := time.Now()
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.impact_pd
		SET deleted_at = $1, deleted_by = $2, updated_at = $1, updated_by = $2
		WHERE id = $3 AND deleted_at IS NULL
	`, now, deletedBy, id)
	if err != nil {
		return nil, fmt.Errorf("repo.SoftDelete impact_pd: %w", err)
	}
	query := baseSelect + " WHERE id = $1"
	row := tx.QueryRowContext(ctx, query, id)
	m, err := scanOne(row)
	if err != nil {
		return nil, fmt.Errorf("repo.SoftDelete refetch: %w", err)
	}
	return m, nil
}

// UpdateWorkflowStatus updates only workflow_status.
func (r *DBRepository) UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.impact_pd
		SET workflow_status = $1, updated_at = now(), updated_by = $2, row_version = row_version + 1
		WHERE id = $3
	`, string(status), updatedBy, id)
	if err != nil {
		return fmt.Errorf("repo.UpdateWorkflowStatus impact_pd: %w", err)
	}
	return nil
}

// CountByPeriode returns the active row count for a given periode_id.
func (r *DBRepository) CountByPeriode(ctx context.Context, periodeID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM mst.impact_pd
		WHERE periode_id = $1 AND deleted_at IS NULL
	`, periodeID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("repo.CountByPeriode: %w", err)
	}
	return count, nil
}

// CountByPeriodeExcluding counts excluding the given ID.
func (r *DBRepository) CountByPeriodeExcluding(ctx context.Context, periodeID uuid.UUID, excludeID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM mst.impact_pd
		WHERE periode_id = $1 AND id != $2 AND deleted_at IS NULL
	`, periodeID, excludeID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("repo.CountByPeriodeExcluding: %w", err)
	}
	return count, nil
}

// BeginTx starts a transaction.
func (r *DBRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("repo.BeginTx impact_pd: %w", err)
	}
	return tx, nil
}

// ListAuditHistory returns paginated audit log rows.
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
	args = append(args, entityID, "mst.impact_pd")
	cond := ""
	if cursorTime != nil {
		args = append(args, *cursorTime)
		cond = fmt.Sprintf(" AND timestamp < $%d", len(args))
	}

	query := fmt.Sprintf( //nolint:gosec
		`SELECT id, timestamp, actor_user_id, actor_role, action,
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
		return nil, false, fmt.Errorf("repo.ListAuditHistory impact_pd: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []AuditHistoryItem
	for rows.Next() {
		var (
			evID      uuid.UUID
			ts        time.Time
			actorUID  uuid.UUID
			actorRole string
			action    string
			beforeRaw []byte
			afterRaw  []byte
			ip        *string
			traceID   *string
		)
		if err := rows.Scan(&evID, &ts, &actorUID, &actorRole, &action,
			&beforeRaw, &afterRaw, &ip, &traceID); err != nil {
			return nil, false, fmt.Errorf("repo.ListAuditHistory scan: %w", err)
		}
		item := AuditHistoryItem{
			EventID:     evID.String(),
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
				if err := json.Unmarshal(beforeRaw, &bj); err == nil {
					item.BeforeJSONB = bj
				}
			}
			if len(afterRaw) > 0 {
				var aj interface{}
				if err := json.Unmarshal(afterRaw, &aj); err == nil {
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

// ExportAll streams all rows as UTF-8 BOM CSV.
func (r *DBRepository) ExportAll(ctx context.Context, q listquery.Query) (io.Reader, int, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("t")
	conditions := []string{"t.deleted_at IS NULL"}
	if where != "" {
		conditions = append(conditions, where)
	}
	if q.Search != "" {
		argIdx := len(args) + 1
		var sc []string
		for _, col := range SearchCols {
			sc = append(sc, fmt.Sprintf("t.%s ILIKE $%d", col, argIdx))
		}
		conditions = append(conditions, "("+strings.Join(sc, " OR ")+")")
		args = append(args, "%"+q.Search+"%")
	}

	whereClause := " WHERE " + strings.Join(conditions, " AND ")
	if orderBy == "" {
		orderBy = "t.created_at DESC"
	}

	query := fmt.Sprintf( //nolint:gosec
		`SELECT t.id, t.periode_id, t.impact_multiplier,
		    t.catatan, t.workflow_status, t.created_at
		 FROM mst.impact_pd t%s ORDER BY %s`,
		whereClause, orderBy,
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll impact_pd: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	w := csv.NewWriter(&buf)

	headers := []string{"ID", "Periode ID", "Impact Multiplier", "Catatan", "Workflow Status", "Dibuat"}
	if err := w.Write(headers); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll write header: %w", err)
	}

	count := 0
	for rows.Next() {
		var (
			id         uuid.UUID
			periodeID  uuid.UUID
			multiplier string
			catatan    *string
			wfStatus   string
			createdAt  time.Time
		)
		if err := rows.Scan(&id, &periodeID, &multiplier, &catatan, &wfStatus, &createdAt); err != nil {
			return nil, 0, fmt.Errorf("repo.ExportAll scan: %w", err)
		}
		catStr := ""
		if catatan != nil {
			catStr = *catatan
		}
		record := []string{
			id.String(), periodeID.String(), multiplier,
			catStr, wfStatus, createdAt.Format(time.RFC3339),
		}
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

func scanOne(row *sql.Row) (*ImpactPD, error) {
	m := &ImpactPD{}
	var (
		multiplierStr string
		wfStatus      string
		createdBy     *uuid.UUID
		updatedAt     *time.Time
		updatedBy     *uuid.UUID
		deletedAt     *time.Time
		deletedBy     *uuid.UUID
	)
	err := row.Scan(
		&m.ID, &m.PeriodeID,
		&multiplierStr, &m.Catatan,
		&wfStatus,
		&m.CreatedAt, &createdBy, &updatedAt, &updatedBy,
		&deletedAt, &deletedBy, &m.RowVersion, &m.TenantID,
	)
	if err != nil {
		return nil, err
	}
	m.WorkflowStatus = WorkflowStatus(wfStatus)
	m.CreatedBy = createdBy
	m.UpdatedAt = updatedAt
	m.UpdatedBy = updatedBy
	m.DeletedAt = deletedAt
	m.DeletedBy = deletedBy

	d, err := decimal.NewFromString(multiplierStr)
	if err != nil {
		return nil, fmt.Errorf("scan impact_multiplier impact_pd: %w", err)
	}
	m.ImpactMultiplier = d
	return m, nil
}

func scanRow(rows *sql.Rows) (*ImpactPD, error) {
	m := &ImpactPD{}
	var (
		multiplierStr string
		wfStatus      string
		createdBy     *uuid.UUID
		updatedAt     *time.Time
		updatedBy     *uuid.UUID
		deletedAt     *time.Time
		deletedBy     *uuid.UUID
	)
	err := rows.Scan(
		&m.ID, &m.PeriodeID,
		&multiplierStr, &m.Catatan,
		&wfStatus,
		&m.CreatedAt, &createdBy, &updatedAt, &updatedBy,
		&deletedAt, &deletedBy, &m.RowVersion, &m.TenantID,
	)
	if err != nil {
		return nil, err
	}
	m.WorkflowStatus = WorkflowStatus(wfStatus)
	m.CreatedBy = createdBy
	m.UpdatedAt = updatedAt
	m.UpdatedBy = updatedBy
	m.DeletedAt = deletedAt
	m.DeletedBy = deletedBy

	d, err := decimal.NewFromString(multiplierStr)
	if err != nil {
		return nil, fmt.Errorf("scan impact_multiplier row impact_pd: %w", err)
	}
	m.ImpactMultiplier = d
	return m, nil
}

// isUniqueViolation checks for PostgreSQL unique constraint violation (23505).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "23505") ||
		strings.Contains(err.Error(), "duplicate key") ||
		strings.Contains(err.Error(), "unique constraint")
}
