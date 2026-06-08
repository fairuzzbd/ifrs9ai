package periodebuku

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

// Repository defines the data-access contract for periode_buku.
// Service layer only depends on this interface (for testability).
type Repository interface {
	// Create inserts a new periode_buku row in the given transaction.
	Create(ctx context.Context, tx *sql.Tx, p *PeriodeBuku) error

	// GetByID fetches one record by surrogate UUID (active and deleted).
	GetByID(ctx context.Context, id uuid.UUID) (*PeriodeBuku, error)

	// GetByKode fetches one record by business key (periode_id_kode).
	// Returns (nil, nil) if not found.
	GetByKode(ctx context.Context, kode string) (*PeriodeBuku, error)

	// List fetches paginated records.
	// Returns rows limited to limit+1 (caller detects hasMore).
	List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*PeriodeBuku, error)

	// Update applies partial update in the given transaction using optimistic lock.
	Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, fields UpdateFields) (*PeriodeBuku, error)

	// SoftDelete sets deleted_at/deleted_by in the given transaction.
	SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*PeriodeBuku, error)

	// UpdateWorkflowStatus updates workflow_status column (called after workflow transitions).
	UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error

	// BulkCreateIfNotExists inserts rows whose periode_id_kode does not yet exist.
	// Returns (created, skipped, error). Each insert is in-tx (caller commits).
	BulkCreateIfNotExists(ctx context.Context, tx *sql.Tx, rows []*PeriodeBuku) (created int, skipped int, err error)

	// CountReferences returns the number of active references to this periode_buku id.
	CountReferences(ctx context.Context, id uuid.UUID) (int64, error)

	// BeginTx starts a database transaction.
	BeginTx(ctx context.Context) (*sql.Tx, error)

	// ListAuditHistory returns paginated audit log rows for a given entity UUID.
	ListAuditHistory(ctx context.Context, entityID uuid.UUID, cursor string, limit int, isAuditRole bool) ([]AuditHistoryItem, bool, error)

	// ExportAll returns all records matching the query as an io.Reader CSV stream.
	ExportAll(ctx context.Context, q listquery.Query) (io.Reader, int, error)
}

// UpdateFields captures the mutable columns for an update operation.
type UpdateFields struct {
	TahunBuku       *int
	Bulan           *int
	Triwulan        *int
	TanggalMulai    *string
	TanggalAkhir    *string
	UpdatedBy       uuid.UUID
	ExpectedVersion int64
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
// Selects all audit columns added in migration 000009.
const baseSelect = `
SELECT
    id, periode_id_kode, tipe_periode, tahun_buku, bulan, triwulan,
    tanggal_mulai, tanggal_akhir, status_periode,
    tanggal_soft_close, tanggal_hard_close,
    user_closer_id, user_approver_close_id, catatan_closing,
    reopened_flag, reopened_reason, reopened_at, reopened_by, reopened_approved_by,
    workflow_status,
    created_at, created_by, updated_at, updated_by,
    deleted_at, deleted_by, row_version, tenant_id
FROM mst.periode_buku`

// Create inserts a new row. Called inside a transaction.
func (r *DBRepository) Create(ctx context.Context, tx *sql.Tx, p *PeriodeBuku) error {
	query := `
INSERT INTO mst.periode_buku (
    id, periode_id_kode, tipe_periode, tahun_buku, bulan, triwulan,
    tanggal_mulai, tanggal_akhir, status_periode,
    workflow_status,
    created_at, created_by, updated_at, updated_by,
    row_version, tenant_id
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9,
    $10,
    $11, $12, $11, $12,
    1, $13
)`
	_, err := tx.ExecContext(ctx, query,
		p.ID, p.PeriodeIDKode, string(p.TipePeriode), p.TahunBuku, p.Bulan, p.Triwulan,
		p.TanggalMulai, p.TanggalAkhir, string(p.StatusPeriode),
		string(p.WorkflowStatus),
		p.CreatedAt, p.CreatedBy,
		p.TenantID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("periode_id_kode: %w", ErrKodeDuplicate)
		}
		return fmt.Errorf("repo.Create periode_buku: %w", err)
	}
	return nil
}

// GetByID fetches by surrogate UUID.
func (r *DBRepository) GetByID(ctx context.Context, id uuid.UUID) (*PeriodeBuku, error) {
	return r.getOne(ctx, r.db, "id = $1", id, true)
}

// GetByKode fetches by business key.
func (r *DBRepository) GetByKode(ctx context.Context, kode string) (*PeriodeBuku, error) {
	return r.getOne(ctx, r.db, "periode_id_kode = $1", kode, false)
}

func (r *DBRepository) getOne(ctx context.Context, q dbQuerier, where string, arg interface{}, includeDeleted bool) (*PeriodeBuku, error) {
	deletedFilter := " AND deleted_at IS NULL"
	if includeDeleted {
		deletedFilter = ""
	}
	query := baseSelect + " WHERE " + where + deletedFilter
	row := q.QueryRowContext(ctx, query, arg)
	p, err := scanPeriodeBuku(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.getOne periode_buku: %w", err)
	}
	return p, nil
}

// List fetches paginated records.
func (r *DBRepository) List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*PeriodeBuku, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("t")

	conditions := []string{}
	if !includeDeleted {
		conditions = append(conditions, "t.deleted_at IS NULL")
	}

	// Cursor: UUID-based cursor ordered by (created_at, id).
	if cursor != "" {
		cd, err := pagination.DecodeCursor(cursor)
		if err == nil && cd.ID != "" {
			argIdx := len(args) + 1
			conditions = append(conditions, fmt.Sprintf("t.id > $%d::uuid", argIdx))
			args = append(args, cd.ID)
		}
	}

	// Text search across search columns.
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
		orderBy = "t.tahun_buku ASC, t.tipe_periode ASC, t.bulan ASC NULLS LAST"
	}

	argIdx := len(args) + 1
	query := fmt.Sprintf( //nolint:gosec
		`SELECT t.id, t.periode_id_kode, t.tipe_periode, t.tahun_buku, t.bulan, t.triwulan,
		t.tanggal_mulai, t.tanggal_akhir, t.status_periode,
		t.tanggal_soft_close, t.tanggal_hard_close,
		t.user_closer_id, t.user_approver_close_id, t.catatan_closing,
		t.reopened_flag, t.reopened_reason, t.reopened_at, t.reopened_by, t.reopened_approved_by,
		t.workflow_status,
		t.created_at, t.created_by, t.updated_at, t.updated_by,
		t.deleted_at, t.deleted_by, t.row_version, t.tenant_id
		FROM mst.periode_buku t%s ORDER BY %s LIMIT $%d`,
		whereClause, orderBy, argIdx,
	)
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.List periode_buku: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []*PeriodeBuku
	for rows.Next() {
		p, err := scanPeriodeBukuRow(rows)
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
func (r *DBRepository) Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, f UpdateFields) (*PeriodeBuku, error) {
	setClauses := []string{}
	args := []interface{}{}
	idx := 1

	if f.TahunBuku != nil {
		setClauses = append(setClauses, fmt.Sprintf("tahun_buku = $%d", idx))
		args = append(args, *f.TahunBuku)
		idx++
	}
	if f.Bulan != nil {
		setClauses = append(setClauses, fmt.Sprintf("bulan = $%d", idx))
		args = append(args, *f.Bulan)
		idx++
	}
	if f.Triwulan != nil {
		setClauses = append(setClauses, fmt.Sprintf("triwulan = $%d", idx))
		args = append(args, *f.Triwulan)
		idx++
	}
	if f.TanggalMulai != nil {
		setClauses = append(setClauses, fmt.Sprintf("tanggal_mulai = $%d", idx))
		args = append(args, *f.TanggalMulai)
		idx++
	}
	if f.TanggalAkhir != nil {
		setClauses = append(setClauses, fmt.Sprintf("tanggal_akhir = $%d", idx))
		args = append(args, *f.TanggalAkhir)
		idx++
	}
	if len(setClauses) == 0 {
		// Nothing to update — return current record.
		return r.GetByID(ctx, id)
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
		`UPDATE mst.periode_buku SET %s WHERE id = $%d AND row_version = $%d AND deleted_at IS NULL`,
		strings.Join(setClauses, ", "),
		whereIDIdx, whereVersionIdx,
	)

	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.Update periode_buku: %w", err)
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
		if existing == nil || existing.DeletedAt != nil {
			return nil, ErrNotFound
		}
		return nil, ErrConflict
	}

	// Return refreshed row within same tx.
	return r.getOne(ctx, tx, "id = $1", id, true)
}

// UpdateWorkflowStatus updates only the workflow_status column.
func (r *DBRepository) UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.periode_buku
		SET workflow_status = $1, updated_at = now(), updated_by = $2, row_version = row_version + 1
		WHERE id = $3
	`, string(status), updatedBy, id)
	if err != nil {
		return fmt.Errorf("repo.UpdateWorkflowStatus periode_buku: %w", err)
	}
	return nil
}

// SoftDelete marks the record as deleted.
func (r *DBRepository) SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*PeriodeBuku, error) {
	now := time.Now()
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.periode_buku
		SET deleted_at = $1, deleted_by = $2, updated_at = $1, updated_by = $2
		WHERE id = $3 AND deleted_at IS NULL
	`, now, deletedBy, id)
	if err != nil {
		return nil, fmt.Errorf("repo.SoftDelete periode_buku: %w", err)
	}
	return r.getOne(ctx, tx, "id = $1", id, true)
}

// BulkCreateIfNotExists inserts rows that have no existing periode_id_kode.
// Idempotent: skips existing rows (ON CONFLICT DO NOTHING).
func (r *DBRepository) BulkCreateIfNotExists(ctx context.Context, tx *sql.Tx, rows []*PeriodeBuku) (created int, skipped int, err error) {
	for _, p := range rows {
		res, execErr := tx.ExecContext(ctx, `
			INSERT INTO mst.periode_buku (
			    id, periode_id_kode, tipe_periode, tahun_buku, bulan, triwulan,
			    tanggal_mulai, tanggal_akhir, status_periode,
			    workflow_status,
			    created_at, created_by, updated_at, updated_by,
			    row_version, tenant_id
			) VALUES (
			    $1, $2, $3, $4, $5, $6,
			    $7, $8, $9,
			    $10,
			    $11, $12, $11, $12,
			    1, $13
			) ON CONFLICT (periode_id_kode) DO NOTHING`,
			p.ID, p.PeriodeIDKode, string(p.TipePeriode), p.TahunBuku, p.Bulan, p.Triwulan,
			p.TanggalMulai, p.TanggalAkhir, string(p.StatusPeriode),
			string(p.WorkflowStatus),
			p.CreatedAt, p.CreatedBy,
			p.TenantID,
		)
		if execErr != nil {
			return created, skipped, fmt.Errorf("repo.BulkCreateIfNotExists: %w", execErr)
		}
		n, rowErr := res.RowsAffected()
		if rowErr != nil {
			return created, skipped, fmt.Errorf("repo.BulkCreateIfNotExists rows affected: %w", rowErr)
		}
		if n == 0 {
			skipped++
		} else {
			created++
		}
	}
	return created, skipped, nil
}

// CountReferences returns the number of active references to this periode_buku ID
// in other tables (kurs, impact_mev_pd, impact_pd, jrnl.header).
func (r *DBRepository) CountReferences(ctx context.Context, id uuid.UUID) (int64, error) {
	var total int64

	queries := []string{
		`SELECT COUNT(*) FROM mst.kurs WHERE periode_bulanan_id = $1`,
		`SELECT COUNT(*) FROM mst.impact_mev_pd WHERE periode_id = $1`,
		`SELECT COUNT(*) FROM mst.impact_pd WHERE periode_id = $1`,
		`SELECT COUNT(*) FROM jrnl.header WHERE periode_id = $1`,
	}

	for _, q := range queries {
		var c int64
		if err := r.db.QueryRowContext(ctx, q, id).Scan(&c); err != nil {
			return 0, fmt.Errorf("repo.CountReferences: %w", err)
		}
		total += c
	}
	return total, nil
}

// BeginTx starts a transaction.
func (r *DBRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("repo.BeginTx: %w", err)
	}
	return tx, nil
}

// ListAuditHistory returns paginated audit events for the given periode_buku entity UUID.
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
	args = append(args, entityID, "mst.periode_buku")
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
		orderBy = "t.tahun_buku ASC, t.tipe_periode ASC, t.bulan ASC NULLS LAST"
	}

	query := fmt.Sprintf( //nolint:gosec
		`SELECT t.periode_id_kode, t.tipe_periode, t.tahun_buku, t.bulan, t.triwulan,
		t.tanggal_mulai, t.tanggal_akhir, t.status_periode, t.workflow_status
		FROM mst.periode_buku t%s ORDER BY %s`,
		whereClause, orderBy,
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var buf bytes.Buffer
	// UTF-8 BOM for Excel compatibility (ux-patterns.md §1.4)
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(&buf)

	// Header row (Indonesian labels per ux-patterns.md §1.4)
	headers := []string{
		"Kode Periode", "Tipe Periode", "Tahun Buku", "Bulan", "Triwulan",
		"Tanggal Mulai", "Tanggal Akhir", "Status Periode", "Workflow Status",
	}
	if err := w.Write(headers); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll write header: %w", err)
	}

	count := 0
	for rows.Next() {
		var (
			kode          string
			tipe          string
			tahun         int
			bulan         *int
			triwulan      *int
			mulai         string
			akhir         string
			statusPeriode string
			wfStatus      string
		)
		if err := rows.Scan(&kode, &tipe, &tahun, &bulan, &triwulan,
			&mulai, &akhir, &statusPeriode, &wfStatus); err != nil {
			return nil, 0, fmt.Errorf("repo.ExportAll scan: %w", err)
		}

		bulanStr := ""
		if bulan != nil {
			bulanStr = fmt.Sprintf("%d", *bulan)
		}
		triwulanStr := ""
		if triwulan != nil {
			triwulanStr = fmt.Sprintf("%d", *triwulan)
		}

		record := []string{
			kode, tipe, fmt.Sprintf("%d", tahun), bulanStr, triwulanStr,
			mulai, akhir, statusPeriode, wfStatus,
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

// dbQuerier abstracts *sql.DB and *sql.Tx for read queries.
type dbQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// scanPeriodeBuku scans one *sql.Row into PeriodeBuku.
func scanPeriodeBuku(row *sql.Row) (*PeriodeBuku, error) {
	p := &PeriodeBuku{}
	var (
		tipe      string
		status    string
		mulai     string
		akhir     string
		wfStatus  string
		createdBy *uuid.UUID
		updatedAt *time.Time
		updatedBy *uuid.UUID
		deletedAt *time.Time
		deletedBy *uuid.UUID
	)
	err := row.Scan(
		&p.ID, &p.PeriodeIDKode, &tipe, &p.TahunBuku, &p.Bulan, &p.Triwulan,
		&mulai, &akhir, &status,
		&p.TanggalSoftClose, &p.TanggalHardClose,
		&p.UserCloserID, &p.UserApproverCloseID, &p.CatatanClosing,
		&p.ReopenedFlag, &p.ReopenedReason, &p.ReopenedAt, &p.ReopenedBy, &p.ReopenedApprovedBy,
		&wfStatus,
		&p.CreatedAt, &createdBy, &updatedAt, &updatedBy,
		&deletedAt, &deletedBy, &p.RowVersion, &p.TenantID,
	)
	if err != nil {
		return nil, err
	}
	p.TipePeriode = TipePeriode(tipe)
	p.StatusPeriode = StatusPeriode(status)
	p.TanggalMulai = mulai
	p.TanggalAkhir = akhir
	p.WorkflowStatus = WorkflowStatus(wfStatus)
	p.CreatedBy = createdBy
	p.UpdatedAt = updatedAt
	p.UpdatedBy = updatedBy
	p.DeletedAt = deletedAt
	p.DeletedBy = deletedBy
	return p, nil
}

// scanPeriodeBukuRow scans one *sql.Rows row into PeriodeBuku.
func scanPeriodeBukuRow(rows *sql.Rows) (*PeriodeBuku, error) {
	p := &PeriodeBuku{}
	var (
		tipe      string
		status    string
		mulai     string
		akhir     string
		wfStatus  string
		createdBy *uuid.UUID
		updatedAt *time.Time
		updatedBy *uuid.UUID
		deletedAt *time.Time
		deletedBy *uuid.UUID
	)
	err := rows.Scan(
		&p.ID, &p.PeriodeIDKode, &tipe, &p.TahunBuku, &p.Bulan, &p.Triwulan,
		&mulai, &akhir, &status,
		&p.TanggalSoftClose, &p.TanggalHardClose,
		&p.UserCloserID, &p.UserApproverCloseID, &p.CatatanClosing,
		&p.ReopenedFlag, &p.ReopenedReason, &p.ReopenedAt, &p.ReopenedBy, &p.ReopenedApprovedBy,
		&wfStatus,
		&p.CreatedAt, &createdBy, &updatedAt, &updatedBy,
		&deletedAt, &deletedBy, &p.RowVersion, &p.TenantID,
	)
	if err != nil {
		return nil, err
	}
	p.TipePeriode = TipePeriode(tipe)
	p.StatusPeriode = StatusPeriode(status)
	p.TanggalMulai = mulai
	p.TanggalAkhir = akhir
	p.WorkflowStatus = WorkflowStatus(wfStatus)
	p.CreatedBy = createdBy
	p.UpdatedAt = updatedAt
	p.UpdatedBy = updatedBy
	p.DeletedAt = deletedAt
	p.DeletedBy = deletedBy
	return p, nil
}

// ─── Sentinel errors ──────────────────────────────────────────────────────────

// ErrNotFound is returned when a record is not found.
var ErrNotFound = fmt.Errorf("periode_buku not found")

// ErrConflict is returned on row_version mismatch.
var ErrConflict = fmt.Errorf("optimistic lock conflict")

// ErrKodeDuplicate is returned when periode_id_kode already exists.
var ErrKodeDuplicate = fmt.Errorf("periode_id_kode duplicate")

// isUniqueViolation checks for PostgreSQL unique constraint violation (error code 23505).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "23505") ||
		strings.Contains(err.Error(), "duplicate key") ||
		strings.Contains(err.Error(), "unique constraint")
}
