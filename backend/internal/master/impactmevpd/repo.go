package impactmevpd

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

// Repository defines data-access for mst.impact_mev_pd.
type Repository interface {
	// Create inserts a new row inside the provided transaction.
	Create(ctx context.Context, tx *sql.Tx, m *ImpactMevPD) error

	// GetByID fetches one record by UUID. Returns (nil, nil) if not found.
	GetByID(ctx context.Context, id uuid.UUID) (*ImpactMevPD, error)

	// List returns paginated rows; returns limit+1 rows so caller can detect hasMore.
	List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*ImpactMevPD, error)

	// Update applies partial changes with optimistic lock.
	// Returns ErrNotFound or ErrConflict sentinels.
	Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, f UpdateFields) (*ImpactMevPD, error)

	// SoftDelete sets deleted_at/deleted_by inside the provided transaction.
	SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*ImpactMevPD, error)

	// UpdateWorkflowStatus updates the workflow_status column.
	UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error

	// CountByPeriodSkenario returns the number of active (non-deleted) rows for
	// the given (periode_id, skenario) pair. Used for duplicate guard.
	CountByPeriodSkenario(ctx context.Context, periodeID uuid.UUID, skenario Skenario) (int64, error)

	// CountByPeriodSkenarioExcluding is like CountByPeriodSkenario but excludes
	// the row with the given ID (used during updates).
	CountByPeriodSkenarioExcluding(ctx context.Context, periodeID uuid.UUID, skenario Skenario, excludeID uuid.UUID) (int64, error)

	// BeginTx starts a read-committed database transaction.
	BeginTx(ctx context.Context) (*sql.Tx, error)

	// ListAuditHistory returns paginated audit_log rows for the entity.
	ListAuditHistory(ctx context.Context, entityID uuid.UUID, cursor string, limit int, isAuditRole bool) ([]AuditHistoryItem, bool, error)

	// ExportAll returns all rows matching q as UTF-8 BOM CSV.
	ExportAll(ctx context.Context, q listquery.Query) (io.Reader, int, error)
}

// UpdateFields captures mutable columns for Update.
type UpdateFields struct {
	ImpactMultiplier  *decimal.Decimal
	MevComponentsJSON *string
	Catatan           *string
	UpdatedBy         uuid.UUID
	ExpectedVersion   int64
}

// ─── Sentinel errors ──────────────────────────────────────────────────────────

var (
	// ErrNotFound is returned when the record does not exist.
	ErrNotFound = fmt.Errorf("impact_mev_pd not found")
	// ErrConflict is returned on row_version mismatch.
	ErrConflict = fmt.Errorf("optimistic lock conflict")
)

// ─── DB implementation ────────────────────────────────────────────────────────

// DBRepository is the production SQL implementation.
type DBRepository struct {
	db *sql.DB
}

// NewDBRepository creates a new DBRepository.
func NewDBRepository(db *sql.DB) *DBRepository {
	return &DBRepository{db: db}
}

var _ Repository = (*DBRepository)(nil)

const baseSelect = `
SELECT
    id, periode_id, skenario,
    impact_multiplier, mev_components_json, catatan,
    workflow_status,
    created_at, created_by, updated_at, updated_by,
    deleted_at, deleted_by, row_version, tenant_id
FROM mst.impact_mev_pd`

// Create inserts a new row.
func (r *DBRepository) Create(ctx context.Context, tx *sql.Tx, m *ImpactMevPD) error {
	query := `
INSERT INTO mst.impact_mev_pd (
    id, periode_id, skenario,
    impact_multiplier, mev_components_json, catatan,
    workflow_status,
    created_at, created_by, updated_at, updated_by,
    row_version, tenant_id
) VALUES (
    $1, $2, $3,
    $4, $5, $6,
    $7,
    $8, $9, $8, $9,
    1, $10
)`
	var mevJSON interface{}
	if m.MevComponentsJSON != nil {
		mevJSON = *m.MevComponentsJSON
	}
	_, err := tx.ExecContext(ctx, query,
		m.ID, m.PeriodeID, string(m.Skenario),
		m.ImpactMultiplier.String(), mevJSON, m.Catatan,
		string(m.WorkflowStatus),
		m.CreatedAt, m.CreatedBy,
		m.TenantID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("unique_violation: %w", ErrConflict)
		}
		return fmt.Errorf("repo.Create impact_mev_pd: %w", err)
	}
	return nil
}

// GetByID fetches one record by UUID.
func (r *DBRepository) GetByID(ctx context.Context, id uuid.UUID) (*ImpactMevPD, error) {
	query := baseSelect + " WHERE id = $1 AND deleted_at IS NULL"
	row := r.db.QueryRowContext(ctx, query, id)
	m, err := scanOne(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.GetByID impact_mev_pd: %w", err)
	}
	return m, nil
}

// List fetches paginated records.
func (r *DBRepository) List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*ImpactMevPD, error) {
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
		var searchConds []string
		for _, col := range SearchCols {
			searchConds = append(searchConds, fmt.Sprintf("t.%s ILIKE $%d", col, argIdx))
		}
		conditions = append(conditions, "("+strings.Join(searchConds, " OR ")+")")
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
		`SELECT t.id, t.periode_id, t.skenario,
		    t.impact_multiplier, t.mev_components_json, t.catatan,
		    t.workflow_status,
		    t.created_at, t.created_by, t.updated_at, t.updated_by,
		    t.deleted_at, t.deleted_by, t.row_version, t.tenant_id
		 FROM mst.impact_mev_pd t%s ORDER BY %s LIMIT $%d`,
		whereClause, orderBy, argIdx,
	)
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.List impact_mev_pd: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []*ImpactMevPD
	for rows.Next() {
		m, err := scanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("repo.List scan impact_mev_pd: %w", err)
		}
		items = append(items, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo.List rows.Err impact_mev_pd: %w", err)
	}
	return items, nil
}

// Update applies partial changes with optimistic lock.
func (r *DBRepository) Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, f UpdateFields) (*ImpactMevPD, error) {
	var setClauses []string
	var args []interface{}
	idx := 1

	if f.ImpactMultiplier != nil {
		setClauses = append(setClauses, fmt.Sprintf("impact_multiplier = $%d", idx))
		args = append(args, f.ImpactMultiplier.String())
		idx++
	}
	if f.MevComponentsJSON != nil {
		setClauses = append(setClauses, fmt.Sprintf("mev_components_json = $%d", idx))
		args = append(args, *f.MevComponentsJSON)
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
		`UPDATE mst.impact_mev_pd SET %s WHERE id = $%d AND row_version = $%d AND deleted_at IS NULL`,
		strings.Join(setClauses, ", "),
		whereIDIdx, whereVersionIdx,
	)

	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.Update impact_mev_pd: %w", err)
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

// SoftDelete sets deleted_at/deleted_by.
func (r *DBRepository) SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*ImpactMevPD, error) {
	now := time.Now()
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.impact_mev_pd
		SET deleted_at = $1, deleted_by = $2, updated_at = $1, updated_by = $2
		WHERE id = $3 AND deleted_at IS NULL
	`, now, deletedBy, id)
	if err != nil {
		return nil, fmt.Errorf("repo.SoftDelete impact_mev_pd: %w", err)
	}
	// Re-fetch including deleted
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
		UPDATE mst.impact_mev_pd
		SET workflow_status = $1, updated_at = now(), updated_by = $2, row_version = row_version + 1
		WHERE id = $3
	`, string(status), updatedBy, id)
	if err != nil {
		return fmt.Errorf("repo.UpdateWorkflowStatus impact_mev_pd: %w", err)
	}
	return nil
}

// CountByPeriodSkenario returns the count for duplicate guard.
func (r *DBRepository) CountByPeriodSkenario(ctx context.Context, periodeID uuid.UUID, skenario Skenario) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM mst.impact_mev_pd
		WHERE periode_id = $1 AND skenario = $2 AND deleted_at IS NULL
	`, periodeID, string(skenario)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("repo.CountByPeriodSkenario: %w", err)
	}
	return count, nil
}

// CountByPeriodSkenarioExcluding counts excluding the given ID (used for update check).
func (r *DBRepository) CountByPeriodSkenarioExcluding(ctx context.Context, periodeID uuid.UUID, skenario Skenario, excludeID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM mst.impact_mev_pd
		WHERE periode_id = $1 AND skenario = $2 AND id != $3 AND deleted_at IS NULL
	`, periodeID, string(skenario), excludeID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("repo.CountByPeriodSkenarioExcluding: %w", err)
	}
	return count, nil
}

// BeginTx starts a transaction.
func (r *DBRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("repo.BeginTx impact_mev_pd: %w", err)
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
	args = append(args, entityID, "mst.impact_mev_pd")
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
		return nil, false, fmt.Errorf("repo.ListAuditHistory impact_mev_pd: %w", err)
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
		`SELECT t.id, t.periode_id, t.skenario, t.impact_multiplier,
		    t.mev_components_json, t.catatan, t.workflow_status, t.created_at
		 FROM mst.impact_mev_pd t%s ORDER BY %s`,
		whereClause, orderBy,
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll impact_mev_pd: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	w := csv.NewWriter(&buf)

	headers := []string{"ID", "Periode ID", "Skenario", "Impact Multiplier", "MEV Components", "Catatan", "Workflow Status", "Dibuat"}
	if err := w.Write(headers); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll write header: %w", err)
	}

	count := 0
	for rows.Next() {
		var (
			id           uuid.UUID
			periodeID    uuid.UUID
			skenario     string
			multiplier   string
			mevJSON      *string
			catatan      *string
			wfStatus     string
			createdAt    time.Time
		)
		if err := rows.Scan(&id, &periodeID, &skenario, &multiplier, &mevJSON, &catatan, &wfStatus, &createdAt); err != nil {
			return nil, 0, fmt.Errorf("repo.ExportAll scan: %w", err)
		}
		mevStr := ""
		if mevJSON != nil {
			mevStr = *mevJSON
		}
		catStr := ""
		if catatan != nil {
			catStr = *catatan
		}
		record := []string{
			id.String(), periodeID.String(), skenario, multiplier,
			mevStr, catStr, wfStatus, createdAt.Format(time.RFC3339),
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

func scanOne(row *sql.Row) (*ImpactMevPD, error) {
	m := &ImpactMevPD{}
	var (
		skenarioStr  string
		multiplierStr string
		mevJSON      *string
		wfStatus     string
		createdBy    *uuid.UUID
		updatedAt    *time.Time
		updatedBy    *uuid.UUID
		deletedAt    *time.Time
		deletedBy    *uuid.UUID
	)
	err := row.Scan(
		&m.ID, &m.PeriodeID, &skenarioStr,
		&multiplierStr, &mevJSON, &m.Catatan,
		&wfStatus,
		&m.CreatedAt, &createdBy, &updatedAt, &updatedBy,
		&deletedAt, &deletedBy, &m.RowVersion, &m.TenantID,
	)
	if err != nil {
		return nil, err
	}
	m.Skenario = Skenario(skenarioStr)
	m.WorkflowStatus = WorkflowStatus(wfStatus)
	m.MevComponentsJSON = mevJSON
	m.CreatedBy = createdBy
	m.UpdatedAt = updatedAt
	m.UpdatedBy = updatedBy
	m.DeletedAt = deletedAt
	m.DeletedBy = deletedBy

	d, err := decimal.NewFromString(multiplierStr)
	if err != nil {
		return nil, fmt.Errorf("scan impact_multiplier: %w", err)
	}
	m.ImpactMultiplier = d
	return m, nil
}

func scanRow(rows *sql.Rows) (*ImpactMevPD, error) {
	m := &ImpactMevPD{}
	var (
		skenarioStr   string
		multiplierStr string
		mevJSON       *string
		wfStatus      string
		createdBy     *uuid.UUID
		updatedAt     *time.Time
		updatedBy     *uuid.UUID
		deletedAt     *time.Time
		deletedBy     *uuid.UUID
	)
	err := rows.Scan(
		&m.ID, &m.PeriodeID, &skenarioStr,
		&multiplierStr, &mevJSON, &m.Catatan,
		&wfStatus,
		&m.CreatedAt, &createdBy, &updatedAt, &updatedBy,
		&deletedAt, &deletedBy, &m.RowVersion, &m.TenantID,
	)
	if err != nil {
		return nil, err
	}
	m.Skenario = Skenario(skenarioStr)
	m.WorkflowStatus = WorkflowStatus(wfStatus)
	m.MevComponentsJSON = mevJSON
	m.CreatedBy = createdBy
	m.UpdatedAt = updatedAt
	m.UpdatedBy = updatedBy
	m.DeletedAt = deletedAt
	m.DeletedBy = deletedBy

	d, err := decimal.NewFromString(multiplierStr)
	if err != nil {
		return nil, fmt.Errorf("scan impact_multiplier row: %w", err)
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
