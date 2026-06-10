package portofolio

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

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
)

// Repository defines the data-access contract for portofolio.
type Repository interface {
	// Create inserts a new portofolio row in the given transaction.
	Create(ctx context.Context, tx *sql.Tx, p *Portofolio) error

	// GetByKode fetches one record by business key.
	// Returns (nil, nil) if not found.
	GetByKode(ctx context.Context, kode string, includeDeleted bool) (*Portofolio, error)

	// GetByID fetches one record by surrogate UUID.
	GetByID(ctx context.Context, id uuid.UUID) (*Portofolio, error)

	// List fetches paginated records matching listquery + cursor.
	// Returns rows limited to limit+1 (caller detects hasMore).
	List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*Portofolio, error)

	// Update applies partial update in the given transaction.
	// Optimistic lock on version column (aliased as row_version in domain).
	// Returns ErrNotFound if kode not found; ErrConflict on version mismatch.
	Update(ctx context.Context, tx *sql.Tx, kode string, fields UpdateFields) (*Portofolio, error)

	// SoftDelete sets deleted_at/deleted_by in the given transaction.
	SoftDelete(ctx context.Context, tx *sql.Tx, kode string, deletedBy uuid.UUID) (*Portofolio, error)

	// UpdateWorkflowStatus updates workflow_status column.
	UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error

	// CountReferences returns the number of active references to this kode_portofolio.
	CountReferences(ctx context.Context, kode string) (int64, error)

	// BeginTx starts a database transaction with default isolation.
	BeginTx(ctx context.Context) (*sql.Tx, error)

	// ListAuditHistory returns paginated audit log rows for a given entity UUID.
	ListAuditHistory(ctx context.Context, entityID uuid.UUID, cursor string, limit int, isAuditRole bool) ([]AuditHistoryItem, bool, error)

	// ExportAll returns all records matching the query as a CSV reader.
	ExportAll(ctx context.Context, q listquery.Query) (io.Reader, int, error)
}

// UpdateFields captures the mutable columns for an update operation.
type UpdateFields struct {
	Nama                   *string
	TujuanPengelolaan      *string
	BMCategoryDefault      *string
	Benchmark              *string
	KompensasiManagerBasis *string
	PeriodeReviewTerakhir  *string
	AktifFlag              *bool
	UpdatedBy              uuid.UUID
	ExpectedVersion        int64
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

var _ Repository = (*DBRepository)(nil)

// baseSelect is the SELECT fragment used by all read queries.
// Note: the DB column is "version" (from 0001) aliased as row_version.
// deleted_at, tenant_id, workflow_status were added in migration 0018.
const baseSelect = `
SELECT
    id, kode_portofolio, nama, tujuan_pengelolaan, bm_category_default,
    benchmark, kompensasi_manager_basis, periode_review_terakhir, aktif_flag,
    workflow_status,
    created_at, created_by, updated_at, updated_by,
    deleted_at, deleted_by, version AS row_version, tenant_id
FROM mst.portofolio`

// Create inserts a new row. Called inside a transaction.
func (r *DBRepository) Create(ctx context.Context, tx *sql.Tx, p *Portofolio) error {
	query := `
INSERT INTO mst.portofolio (
    id, kode_portofolio, nama, tujuan_pengelolaan, bm_category_default,
    benchmark, kompensasi_manager_basis, periode_review_terakhir, aktif_flag,
    workflow_status,
    created_at, created_by, updated_at, updated_by,
    deleted_at, deleted_by, version, tenant_id,
    is_deleted
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9,
    $10,
    $11, $12, $11, $12,
    NULL, NULL, 1, $13,
    FALSE
)`
	_, err := tx.ExecContext(ctx, query,
		p.ID, p.KodePortofolio, p.Nama, p.TujuanPengelolaan, string(p.BMCategoryDefault),
		p.Benchmark, p.KompensasiManagerBasis, p.PeriodeReviewTerakhir, p.AktifFlag,
		string(p.WorkflowStatus),
		p.CreatedAt, p.CreatedBy,
		p.TenantID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("kode_portofolio: %w", ErrKodeDuplicate)
		}
		return fmt.Errorf("repo.Create portofolio: %w", err)
	}
	return nil
}

// GetByKode fetches by business key.
func (r *DBRepository) GetByKode(ctx context.Context, kode string, includeDeleted bool) (*Portofolio, error) {
	return r.getOne(ctx, r.db, "kode_portofolio = $1", kode, includeDeleted)
}

// GetByID fetches by surrogate UUID.
func (r *DBRepository) GetByID(ctx context.Context, id uuid.UUID) (*Portofolio, error) {
	return r.getOne(ctx, r.db, "id = $1", id, true)
}

func (r *DBRepository) getOne(ctx context.Context, q querier, where string, arg interface{}, includeDeleted bool) (*Portofolio, error) {
	deletedFilter := " AND deleted_at IS NULL"
	if includeDeleted {
		deletedFilter = ""
	}
	query := baseSelect + " WHERE " + where + deletedFilter
	row := q.QueryRowContext(ctx, query, arg)
	p, err := scanPortofolio(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.getOne portofolio: %w", err)
	}
	return p, nil
}

// List fetches paginated records.
func (r *DBRepository) List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*Portofolio, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("t")

	conditions := []string{}
	if !includeDeleted {
		conditions = append(conditions, "t.deleted_at IS NULL")
	}

	if cursor != "" {
		cd, err := pagination.DecodeCursor(cursor)
		if err == nil && cd.ID != "" {
			argIdx := len(args) + 1
			conditions = append(conditions, fmt.Sprintf("t.kode_portofolio > $%d", argIdx))
			args = append(args, cd.ID)
		}
	}

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
		orderBy = "t.kode_portofolio ASC"
	}

	argIdx := len(args) + 1
	query := fmt.Sprintf( //nolint:gosec
		"SELECT t.id, t.kode_portofolio, t.nama, t.tujuan_pengelolaan, t.bm_category_default, "+
			"t.benchmark, t.kompensasi_manager_basis, t.periode_review_terakhir, t.aktif_flag, "+
			"t.workflow_status, "+
			"t.created_at, t.created_by, t.updated_at, t.updated_by, "+
			"t.deleted_at, t.deleted_by, t.version AS row_version, t.tenant_id "+
			"FROM mst.portofolio t%s ORDER BY %s LIMIT $%d",
		whereClause, orderBy, argIdx,
	)
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.List portofolio: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []*Portofolio
	for rows.Next() {
		p, err := scanPortofolioRow(rows)
		if err != nil {
			return nil, fmt.Errorf("repo.List scan: %w", err)
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo.List rows.Err: %w", err)
	}
	return items, nil
}

// Update applies partial field changes with optimistic lock.
// DB column "version" is used for the optimistic lock check.
func (r *DBRepository) Update(ctx context.Context, tx *sql.Tx, kode string, f UpdateFields) (*Portofolio, error) {
	setClauses := []string{}
	args := []interface{}{}
	idx := 1

	if f.Nama != nil {
		setClauses = append(setClauses, fmt.Sprintf("nama = $%d", idx))
		args = append(args, *f.Nama)
		idx++
	}
	if f.TujuanPengelolaan != nil {
		setClauses = append(setClauses, fmt.Sprintf("tujuan_pengelolaan = $%d", idx))
		args = append(args, *f.TujuanPengelolaan)
		idx++
	}
	if f.BMCategoryDefault != nil {
		setClauses = append(setClauses, fmt.Sprintf("bm_category_default = $%d", idx))
		args = append(args, *f.BMCategoryDefault)
		idx++
	}
	if f.Benchmark != nil {
		setClauses = append(setClauses, fmt.Sprintf("benchmark = $%d", idx))
		args = append(args, *f.Benchmark)
		idx++
	}
	if f.KompensasiManagerBasis != nil {
		setClauses = append(setClauses, fmt.Sprintf("kompensasi_manager_basis = $%d", idx))
		args = append(args, *f.KompensasiManagerBasis)
		idx++
	}
	if f.PeriodeReviewTerakhir != nil {
		setClauses = append(setClauses, fmt.Sprintf("periode_review_terakhir = $%d", idx))
		args = append(args, *f.PeriodeReviewTerakhir)
		idx++
	}
	if f.AktifFlag != nil {
		setClauses = append(setClauses, fmt.Sprintf("aktif_flag = $%d", idx))
		args = append(args, *f.AktifFlag)
		idx++
	}
	if len(setClauses) == 0 {
		return r.GetByKode(ctx, kode, false)
	}

	setClauses = append(setClauses,
		fmt.Sprintf("updated_at = $%d", idx),
		fmt.Sprintf("updated_by = $%d", idx+1),
		"version = version + 1",
	)
	args = append(args, time.Now(), f.UpdatedBy)
	idx += 2

	whereKodeIdx := idx
	whereVersionIdx := idx + 1
	args = append(args, kode, f.ExpectedVersion)

	query := fmt.Sprintf( //nolint:gosec
		`UPDATE mst.portofolio SET %s WHERE kode_portofolio = $%d AND version = $%d AND deleted_at IS NULL`,
		strings.Join(setClauses, ", "),
		whereKodeIdx, whereVersionIdx,
	)

	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.Update portofolio: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("repo.Update rows affected: %w", err)
	}
	if n == 0 {
		existing, err := r.GetByKode(ctx, kode, false)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, ErrNotFound
		}
		return nil, ErrConflict
	}

	return r.getOne(ctx, tx, "kode_portofolio = $1", kode, false)
}

// UpdateWorkflowStatus updates only the workflow_status column.
func (r *DBRepository) UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.portofolio
		SET workflow_status = $1, updated_at = now(), updated_by = $2, version = version + 1
		WHERE id = $3
	`, string(status), updatedBy, id)
	if err != nil {
		return fmt.Errorf("repo.UpdateWorkflowStatus portofolio: %w", err)
	}
	return nil
}

// SoftDelete marks the record as deleted.
func (r *DBRepository) SoftDelete(ctx context.Context, tx *sql.Tx, kode string, deletedBy uuid.UUID) (*Portofolio, error) {
	now := time.Now()
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.portofolio
		SET deleted_at = $1, deleted_by = $2, updated_at = $1, updated_by = $2, is_deleted = TRUE
		WHERE kode_portofolio = $3 AND deleted_at IS NULL
	`, now, deletedBy, kode)
	if err != nil {
		return nil, fmt.Errorf("repo.SoftDelete portofolio: %w", err)
	}
	return r.getOne(ctx, tx, "kode_portofolio = $1", kode, true)
}

// CountReferences counts active FK references to this kode_portofolio.
func (r *DBRepository) CountReferences(ctx context.Context, kode string) (int64, error) {
	// mst.instrumen references mst.portofolio via portofolio_id (FK by UUID).
	// The kode lookup joins via the ID.
	var count int64
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM mst.instrumen i
		JOIN mst.portofolio p ON p.id = i.portofolio_id
		WHERE p.kode_portofolio = $1 AND i.deleted_at IS NULL
	`, kode).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("repo.CountReferences portofolio: %w", err)
	}
	return count, nil
}

// BeginTx starts a transaction.
func (r *DBRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("repo.BeginTx: %w", err)
	}
	return tx, nil
}

// ListAuditHistory returns paginated audit events for the given portofolio entity UUID.
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
	args = append(args, entityID, "mst.portofolio")
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
		return nil, false, fmt.Errorf("repo.ListAuditHistory: %w", err)
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
			searchCond = append(searchCond, fmt.Sprintf("t.%s ILIKE $%d", col, argIdx))
		}
		conditions = append(conditions, "("+strings.Join(searchCond, " OR ")+")")
		args = append(args, "%"+q.Search+"%")
	}

	whereClause := " WHERE " + strings.Join(conditions, " AND ")
	if orderBy == "" {
		orderBy = "t.kode_portofolio ASC"
	}

	query := fmt.Sprintf( //nolint:gosec
		"SELECT t.kode_portofolio, t.nama, t.bm_category_default, t.aktif_flag, t.workflow_status "+
			"FROM mst.portofolio t%s ORDER BY %s",
		whereClause, orderBy,
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	w := csv.NewWriter(&buf)

	headers := []string{"Kode Portofolio", "Nama", "BM Category Default", "Status Aktif", "Workflow Status"}
	if err := w.Write(headers); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll write header: %w", err)
	}

	count := 0
	for rows.Next() {
		var (
			kode     string
			nama     string
			bmCat    string
			aktif    bool
			wfStatus string
		)
		if err := rows.Scan(&kode, &nama, &bmCat, &aktif, &wfStatus); err != nil {
			return nil, 0, fmt.Errorf("repo.ExportAll scan: %w", err)
		}

		aktifStr := "Aktif"
		if !aktif {
			aktifStr = "Tidak Aktif"
		}

		record := []string{kode, nama, bmCat, aktifStr, wfStatus}
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

type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// scanPortofolio scans one *sql.Row into Portofolio.
func scanPortofolio(row *sql.Row) (*Portofolio, error) {
	p := &Portofolio{}
	var (
		bmCat          string
		workflowStatus string
		createdBy      *uuid.UUID
		updatedAt      *time.Time
		updatedBy      *uuid.UUID
		deletedAt      *time.Time
		deletedBy      *uuid.UUID
	)
	err := row.Scan(
		&p.ID, &p.KodePortofolio, &p.Nama, &p.TujuanPengelolaan, &bmCat,
		&p.Benchmark, &p.KompensasiManagerBasis, &p.PeriodeReviewTerakhir, &p.AktifFlag,
		&workflowStatus,
		&p.CreatedAt, &createdBy, &updatedAt, &updatedBy,
		&deletedAt, &deletedBy, &p.RowVersion, &p.TenantID,
	)
	if err != nil {
		return nil, err
	}
	p.BMCategoryDefault = BMCategory(bmCat)
	p.WorkflowStatus = WorkflowStatus(workflowStatus)
	p.CreatedBy = createdBy
	p.UpdatedAt = updatedAt
	p.UpdatedBy = updatedBy
	p.DeletedAt = deletedAt
	p.DeletedBy = deletedBy
	return p, nil
}

// scanPortofolioRow scans one *sql.Rows row into Portofolio.
func scanPortofolioRow(rows *sql.Rows) (*Portofolio, error) {
	p := &Portofolio{}
	var (
		bmCat          string
		workflowStatus string
		createdBy      *uuid.UUID
		updatedAt      *time.Time
		updatedBy      *uuid.UUID
		deletedAt      *time.Time
		deletedBy      *uuid.UUID
	)
	err := rows.Scan(
		&p.ID, &p.KodePortofolio, &p.Nama, &p.TujuanPengelolaan, &bmCat,
		&p.Benchmark, &p.KompensasiManagerBasis, &p.PeriodeReviewTerakhir, &p.AktifFlag,
		&workflowStatus,
		&p.CreatedAt, &createdBy, &updatedAt, &updatedBy,
		&deletedAt, &deletedBy, &p.RowVersion, &p.TenantID,
	)
	if err != nil {
		return nil, err
	}
	p.BMCategoryDefault = BMCategory(bmCat)
	p.WorkflowStatus = WorkflowStatus(workflowStatus)
	p.CreatedBy = createdBy
	p.UpdatedAt = updatedAt
	p.UpdatedBy = updatedBy
	p.DeletedAt = deletedAt
	p.DeletedBy = deletedBy
	return p, nil
}

// ─── Sentinel errors ──────────────────────────────────────────────────────────

var ErrNotFound = fmt.Errorf("portofolio not found")
var ErrConflict = fmt.Errorf("optimistic lock conflict")
var ErrKodeDuplicate = fmt.Errorf("kode_portofolio duplicate")

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "23505") ||
		strings.Contains(err.Error(), "duplicate key") ||
		strings.Contains(err.Error(), "unique constraint")
}
