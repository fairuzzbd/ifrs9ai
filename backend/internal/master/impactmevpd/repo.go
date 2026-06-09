package impactmevpd

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
)

// Repository defines the data-access contract for mst.impact_mev_pd.
type Repository interface {
	Create(ctx context.Context, tx *sql.Tx, e *ImpactMevPd) error
	GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*ImpactMevPd, error)
	List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool, tenantID string) ([]*ImpactMevPd, error)
	Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, f UpdateFields, tenantID string) (*ImpactMevPd, error)
	SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID, tenantID string) (*ImpactMevPd, error)
	UpdateWorkflowStatusTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, tenantID string) error
	// CountDuplicate returns count of non-deleted rows with (periode_id, skenario)
	// that have workflow_status NOT IN ('REJECTED','RETURNED'), excluding excludeID.
	CountDuplicate(ctx context.Context, periodeID uuid.UUID, skenario Skenario, excludeID uuid.UUID, tenantID string) (int64, error)
	// CountDuplicateTx is the tx-aware version of CountDuplicate, called inside BeforeCommit.
	CountDuplicateTx(ctx context.Context, tx *sql.Tx, periodeID uuid.UUID, skenario Skenario, excludeID uuid.UUID, tenantID string) (int64, error)
	// GetActive returns APPROVED rows for the given periode_id (GOOD + BAD).
	GetActive(ctx context.Context, periodeID uuid.UUID, tenantID string) ([]*ImpactMevPd, error)
	BeginTx(ctx context.Context) (*sql.Tx, error)
	ListAuditHistory(ctx context.Context, entityID uuid.UUID, cursor string, limit int, isAuditRole bool) ([]AuditHistoryItem, bool, error)
	ExportAll(ctx context.Context, q listquery.Query, tenantID string) (io.Reader, int, error)
}

// UpdateFields captures mutable columns for an update operation.
type UpdateFields struct {
	ImpactMultiplier   *decimal.Decimal
	MevComponentsJSON  *string
	Catatan            *string
	DokumenPendukungID *uuid.UUID
	UpdatedBy          uuid.UUID
	ExpectedVersion    int64
}

// DBRepository is the production SQL implementation.
type DBRepository struct {
	db *sql.DB
}

// NewDBRepository creates a new DBRepository.
func NewDBRepository(db *sql.DB) *DBRepository { return &DBRepository{db: db} }

var _ Repository = (*DBRepository)(nil)

const baseSelect = `
SELECT
    id, periode_id, skenario, impact_multiplier,
    mev_components_json, catatan, dokumen_pendukung_id,
    maker_id, approver_id, approved_at,
    workflow_status, workflow_instance_id,
    created_at, created_by, updated_at, updated_by,
    deleted_at, deleted_by, row_version, tenant_id
FROM mst.impact_mev_pd`

func (r *DBRepository) Create(ctx context.Context, tx *sql.Tx, e *ImpactMevPd) error {
	query := `
INSERT INTO mst.impact_mev_pd (
    id, periode_id, skenario, impact_multiplier,
    mev_components_json, catatan, dokumen_pendukung_id,
    maker_id, workflow_status,
    created_at, created_by, updated_at, updated_by,
    row_version, tenant_id
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7,
    $8, $9,
    $10, $11, $10, $11,
    1, $12
)`
	_, err := tx.ExecContext(ctx, query,
		e.ID, e.PeriodeID, string(e.Skenario), e.ImpactMultiplier,
		e.MevComponentsJSON, e.Catatan, e.DokumenPendukungID,
		e.MakerID, string(e.WorkflowStatus),
		e.CreatedAt, e.CreatedBy,
		e.TenantID,
	)
	if err != nil {
		return fmt.Errorf("repo.Create impact_mev_pd: %w", err)
	}
	return nil
}

func (r *DBRepository) GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*ImpactMevPd, error) {
	deletedFilter := " AND deleted_at IS NULL"
	if includeDeleted {
		deletedFilter = ""
	}
	//nolint:gosec
	query := baseSelect + " WHERE id = $1" + deletedFilter
	row := r.db.QueryRowContext(ctx, query, id)
	e, err := scanRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.GetByID impact_mev_pd: %w", err)
	}
	return e, nil
}

func (r *DBRepository) List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool, tenantID string) ([]*ImpactMevPd, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("t")

	var conditions []string
	if !includeDeleted {
		conditions = append(conditions, "t.deleted_at IS NULL")
	}

	// F3: tenant isolation
	argIdx := len(args) + 1
	conditions = append(conditions, fmt.Sprintf("t.tenant_id = $%d", argIdx))
	args = append(args, tenantID)

	if cursor != "" {
		cd, err := pagination.DecodeCursor(cursor)
		if err == nil && cd.ID != "" {
			argIdx = len(args) + 1
			conditions = append(conditions, fmt.Sprintf("t.id > $%d", argIdx))
			args = append(args, cd.ID)
		}
	}

	if q.Search != "" {
		argIdx = len(args) + 1
		var searchParts []string
		for _, col := range SearchCols {
			searchParts = append(searchParts, fmt.Sprintf("t.%s ILIKE $%d", col, argIdx))
		}
		conditions = append(conditions, "("+strings.Join(searchParts, " OR ")+")")
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

	argIdx = len(args) + 1
	query := fmt.Sprintf( //nolint:gosec
		`SELECT t.id, t.periode_id, t.skenario, t.impact_multiplier,
		    t.mev_components_json, t.catatan, t.dokumen_pendukung_id,
		    t.maker_id, t.approver_id, t.approved_at,
		    t.workflow_status, t.workflow_instance_id,
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

	var items []*ImpactMevPd
	for rows.Next() {
		e, err := scanRows(rows)
		if err != nil {
			return nil, fmt.Errorf("repo.List impact_mev_pd scan: %w", err)
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo.List impact_mev_pd rows.Err: %w", err)
	}
	return items, nil
}

func (r *DBRepository) Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, f UpdateFields, tenantID string) (*ImpactMevPd, error) {
	var setClauses []string
	var args []interface{}
	idx := 1

	if f.ImpactMultiplier != nil {
		setClauses = append(setClauses, fmt.Sprintf("impact_multiplier = $%d", idx))
		args = append(args, *f.ImpactMultiplier)
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

	// F3: tenant isolation
	args = append(args, id, f.ExpectedVersion, tenantID)
	query := fmt.Sprintf( //nolint:gosec
		`UPDATE mst.impact_mev_pd SET %s WHERE id = $%d AND row_version = $%d AND deleted_at IS NULL AND tenant_id = $%d`,
		strings.Join(setClauses, ", "), idx, idx+1, idx+2,
	)

	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.Update impact_mev_pd: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("repo.Update impact_mev_pd RowsAffected: %w", err)
	}
	if n == 0 {
		// Could be not-found or version mismatch — check existence.
		existing, checkErr := r.GetByID(ctx, id, false)
		if checkErr != nil {
			return nil, fmt.Errorf("repo.Update impact_mev_pd check: %w", checkErr)
		}
		if existing == nil {
			return nil, ErrNotFound
		}
		return nil, ErrConflict
	}

	return r.GetByID(ctx, id, false)
}

func (r *DBRepository) SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID, tenantID string) (*ImpactMevPd, error) {
	now := time.Now()
	_, err := tx.ExecContext(ctx,
		`UPDATE mst.impact_mev_pd SET deleted_at=$1, deleted_by=$2, updated_at=$1, row_version=row_version+1
		 WHERE id=$3 AND deleted_at IS NULL AND tenant_id=$4`,
		now, deletedBy, id, tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("repo.SoftDelete impact_mev_pd: %w", err)
	}
	return r.GetByID(ctx, id, true)
}

func (r *DBRepository) UpdateWorkflowStatusTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, tenantID string) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE mst.impact_mev_pd SET workflow_status=$1, updated_at=now(), row_version=row_version+1 WHERE id=$2 AND tenant_id=$3`,
		string(status), id, tenantID,
	)
	if err != nil {
		return fmt.Errorf("repo.UpdateWorkflowStatusTx impact_mev_pd: %w", err)
	}
	return nil
}

func (r *DBRepository) CountDuplicate(ctx context.Context, periodeID uuid.UUID, skenario Skenario, excludeID uuid.UUID, tenantID string) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM mst.impact_mev_pd
		 WHERE periode_id=$1 AND skenario=$2
		   AND workflow_status NOT IN ('REJECTED','RETURNED')
		   AND deleted_at IS NULL
		   AND tenant_id=$3
		   AND ($4::uuid IS NULL OR id != $4)`,
		periodeID, string(skenario), tenantID, nullIfNil(excludeID),
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("repo.CountDuplicate impact_mev_pd: %w", err)
	}
	return count, nil
}

// CountDuplicateTx is the tx-aware version used inside BeforeCommit overlap guard (F1).
func (r *DBRepository) CountDuplicateTx(ctx context.Context, tx *sql.Tx, periodeID uuid.UUID, skenario Skenario, excludeID uuid.UUID, tenantID string) (int64, error) {
	var count int64
	err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM mst.impact_mev_pd
		 WHERE periode_id=$1 AND skenario=$2
		   AND workflow_status='APPROVED'
		   AND deleted_at IS NULL
		   AND tenant_id=$3
		   AND ($4::uuid IS NULL OR id != $4)`,
		periodeID, string(skenario), tenantID, nullIfNil(excludeID),
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("repo.CountDuplicateTx impact_mev_pd: %w", err)
	}
	return count, nil
}

func (r *DBRepository) GetActive(ctx context.Context, periodeID uuid.UUID, tenantID string) ([]*ImpactMevPd, error) {
	query := baseSelect + ` WHERE periode_id=$1 AND workflow_status='APPROVED' AND deleted_at IS NULL AND tenant_id=$2`
	rows, err := r.db.QueryContext(ctx, query, periodeID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("repo.GetActive impact_mev_pd: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []*ImpactMevPd
	for rows.Next() {
		e, err := scanRows(rows)
		if err != nil {
			return nil, fmt.Errorf("repo.GetActive impact_mev_pd scan: %w", err)
		}
		items = append(items, e)
	}
	return items, rows.Err()
}

func (r *DBRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
}

func (r *DBRepository) ListAuditHistory(ctx context.Context, entityID uuid.UUID, cursor string, limit int, isAuditRole bool) ([]AuditHistoryItem, bool, error) {
	var args []interface{}
	conditions := []string{"entity_type='mst.impact_mev_pd'", "entity_id=$1"}
	args = append(args, entityID)
	_ = isAuditRole // No PII filtering on this table; param kept for interface uniformity.

	if cursor != "" {
		cd, err := pagination.DecodeCursor(cursor)
		if err == nil && cd.ID != "" {
			args = append(args, cd.ID)
			conditions = append(conditions, fmt.Sprintf("event_id > $%d", len(args)))
		}
	}

	whereClause := "WHERE " + strings.Join(conditions, " AND ")
	args = append(args, limit+1)
	query := fmt.Sprintf( //nolint:gosec
		`SELECT event_id, event_time, actor_user_id, actor_role, action,
		    before_jsonb, after_jsonb, ip, trace_id
		 FROM aud.audit_log %s ORDER BY event_time DESC LIMIT $%d`,
		whereClause, len(args),
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, fmt.Errorf("repo.ListAuditHistory impact_mev_pd: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []AuditHistoryItem
	for rows.Next() {
		var item AuditHistoryItem
		var before, after []byte
		if err := rows.Scan(
			&item.EventID, &item.EventTime, &item.ActorUserID, &item.ActorRole, &item.Action,
			&before, &after, &item.IP, &item.TraceID,
		); err != nil {
			return nil, false, fmt.Errorf("repo.ListAuditHistory impact_mev_pd scan: %w", err)
		}
		item.BeforeJSONB = nullBytes(before)
		item.AfterJSONB = nullBytes(after)
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

func (r *DBRepository) ExportAll(ctx context.Context, q listquery.Query, tenantID string) (io.Reader, int, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("t")
	// F3: tenant isolation
	argIdx := len(args) + 1
	conditions := []string{"t.deleted_at IS NULL", fmt.Sprintf("t.tenant_id = $%d", argIdx)}
	args = append(args, tenantID)
	if where != "" {
		conditions = append(conditions, where)
	}
	if orderBy == "" {
		orderBy = "t.created_at DESC"
	}
	whereClause := "WHERE " + strings.Join(conditions, " AND ")

	query := fmt.Sprintf( //nolint:gosec
		`SELECT t.id, t.periode_id, t.skenario, t.impact_multiplier, t.catatan, t.workflow_status, t.created_at
		 FROM mst.impact_mev_pd t %s ORDER BY %s`,
		whereClause, orderBy,
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll impact_mev_pd: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var buf bytes.Buffer
	buf.Write([]byte("\xEF\xBB\xBF")) // UTF-8 BOM
	w := csv.NewWriter(&buf)
	if err := w.Write([]string{"ID", "Periode ID", "Skenario", "Impact Multiplier", "Catatan", "Status Workflow", "Dibuat"}); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll impact_mev_pd header write: %w", err)
	}

	count := 0
	for rows.Next() {
		var (
			id, periodeID, skenario string
			multiplier              decimal.Decimal
			catatan                 *string
			status, createdAt       string
		)
		if err := rows.Scan(&id, &periodeID, &skenario, &multiplier, &catatan, &status, &createdAt); err != nil {
			return nil, 0, fmt.Errorf("repo.ExportAll impact_mev_pd scan: %w", err)
		}
		catatanVal := ""
		if catatan != nil {
			catatanVal = *catatan
		}
		if err := w.Write([]string{id, periodeID, skenario, multiplier.StringFixed(8), catatanVal, status, createdAt}); err != nil {
			return nil, 0, fmt.Errorf("repo.ExportAll impact_mev_pd row write: %w", err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll impact_mev_pd rows.Err: %w", err)
	}
	w.Flush()
	return &buf, count, nil
}

// ─── Scan helpers ─────────────────────────────────────────────────────────────

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanRow(s scanner) (*ImpactMevPd, error) {
	var e ImpactMevPd
	var skenario, wfStatus string
	var mevJSON *string
	err := s.Scan(
		&e.ID, &e.PeriodeID, &skenario, &e.ImpactMultiplier,
		&mevJSON, &e.Catatan, &e.DokumenPendukungID,
		&e.MakerID, &e.ApproverID, &e.ApprovedAt,
		&wfStatus, &e.WorkflowInstanceID,
		&e.CreatedAt, &e.CreatedBy, &e.UpdatedAt, &e.UpdatedBy,
		&e.DeletedAt, &e.DeletedBy, &e.RowVersion, &e.TenantID,
	)
	if err != nil {
		return nil, err
	}
	e.Skenario = Skenario(skenario)
	e.WorkflowStatus = WorkflowStatus(wfStatus)
	e.MevComponentsJSON = mevJSON
	return &e, nil
}

func scanRows(rows *sql.Rows) (*ImpactMevPd, error) { return scanRow(rows) }

// nullIfNil converts a zero UUID to nil for parameterized queries.
func nullIfNil(id uuid.UUID) interface{} {
	if id == uuid.Nil {
		return nil
	}
	return id
}

// nullBytes converts empty byte slices to nil interface{}.
func nullBytes(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	return b
}
