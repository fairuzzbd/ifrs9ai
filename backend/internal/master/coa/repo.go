package coa

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

// Repository defines the data-access contract for chart_of_accounts.
type Repository interface {
	// Create inserts a new row in the given transaction.
	Create(ctx context.Context, tx *sql.Tx, c *ChartOfAccount) error

	// BulkCreate inserts multiple rows (used by XLSX import worker). Each row is
	// independently committed; the caller manages partial-failure semantics.
	BulkCreate(ctx context.Context, tx *sql.Tx, rows []*ChartOfAccount) error

	// GetByID fetches one record by surrogate UUID.
	GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*ChartOfAccount, error)

	// GetByKode fetches one record by business key kode_akun.
	GetByKode(ctx context.Context, kode string, includeDeleted bool) (*ChartOfAccount, error)

	// List fetches paginated records matching listquery + cursor.
	// Returns rows limited to limit+1 (caller detects hasMore).
	List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*ChartOfAccount, error)

	// Update applies partial update with optimistic lock.
	// Returns ErrNotFound if kode not found; ErrConflict if version mismatch.
	Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, f UpdateFields) (*ChartOfAccount, error)

	// SoftDelete sets deleted_at/deleted_by.
	SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*ChartOfAccount, error)

	// UpdateWorkflowStatus updates workflow_status (called after workflow transitions).
	UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error

	// CountChildrenOf returns the number of non-deleted records that reference
	// the given id as parent_akun_id. Used by delete guard.
	CountChildrenOf(ctx context.Context, id uuid.UUID) (int64, error)

	// BeginTx starts a database transaction.
	BeginTx(ctx context.Context) (*sql.Tx, error)

	// ListAuditHistory returns paginated audit log rows for a given entity UUID.
	ListAuditHistory(ctx context.Context, entityID uuid.UUID, cursor string, limit int, isAuditRole bool) ([]AuditHistoryItem, bool, error)

	// ExportAll returns all matching records as a UTF-8 BOM CSV io.Reader.
	ExportAll(ctx context.Context, q listquery.Query) (io.Reader, int, error)
}

// UpdateFields captures the mutable columns for an update operation.
// Nil pointer = do not update that column.
type UpdateFields struct {
	NamaAkun          *string
	SubTipeAkun       *string
	KategoriInvestasi *string
	MataUangNative    *string
	PosisiNormal      *PosisiNormal
	AktifFlag         *bool
	ParentAkunID      *uuid.UUID // nil = no change; use ClearParent to null it
	ClearParent       bool       // true = set parent_akun_id = NULL
	SumberCoa         *string
	TanggalMulaiAktif *string
	UpdatedBy         uuid.UUID
	ExpectedVersion   int
}

// ─── Sentinel errors ─────────────────────────────────────────────────────────

// ErrNotFound is returned when a record is not found.
var ErrNotFound = fmt.Errorf("chart_of_account not found")

// ErrConflict is returned on version mismatch (optimistic lock).
var ErrConflict = fmt.Errorf("optimistic lock conflict")

// ErrKodeDuplicate is returned when kode_akun already exists.
var ErrKodeDuplicate = fmt.Errorf("kode_akun duplicate")

// ─── DBRepository ─────────────────────────────────────────────────────────────

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
// Note: the DB column is 'version' INT (Phase 5 will rename to row_version).
const baseSelect = `
SELECT
    id, kode_akun, nama_akun, tipe_akun, sub_tipe_akun,
    kategori_investasi, mata_uang_native, posisi_normal,
    aktif_flag, parent_akun_id, sumber_coa, tanggal_mulai_aktif,
    created_by, created_at, updated_by, updated_at,
    deleted_at, deleted_by, version, tenant_id, workflow_status
FROM mst.chart_of_accounts`

// Create inserts a new row.
func (r *DBRepository) Create(ctx context.Context, tx *sql.Tx, c *ChartOfAccount) error {
	query := `
INSERT INTO mst.chart_of_accounts (
    id, kode_akun, nama_akun, tipe_akun, sub_tipe_akun,
    kategori_investasi, mata_uang_native, posisi_normal,
    aktif_flag, parent_akun_id, sumber_coa, tanggal_mulai_aktif,
    created_by, created_at, updated_by, updated_at,
    version, tenant_id, workflow_status
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8,
    $9, $10, $11, $12,
    $13, $14, $13, $14,
    1, $15, $16
)`
	_, err := tx.ExecContext(ctx, query,
		c.ID, c.KodeAkun, c.NamaAkun, string(c.TipeAkun), c.SubTipeAkun,
		c.KategoriInvestasi, c.MataUangNative, string(c.PosisiNormal),
		c.AktifFlag, c.ParentAkunID, c.SumberCoa, c.TanggalMulaiAktif,
		c.CreatedBy, c.CreatedAt,
		c.TenantID, string(c.WorkflowStatus),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("kode_akun: %w", ErrKodeDuplicate)
		}
		return fmt.Errorf("repo.Create coa: %w", err)
	}
	return nil
}

// BulkCreate inserts multiple rows in one transaction.
func (r *DBRepository) BulkCreate(ctx context.Context, tx *sql.Tx, rows []*ChartOfAccount) error {
	for _, c := range rows {
		if err := r.Create(ctx, tx, c); err != nil {
			return err
		}
	}
	return nil
}

// GetByID fetches by surrogate UUID.
func (r *DBRepository) GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*ChartOfAccount, error) {
	return r.getOne(ctx, r.db, "id = $1", id, includeDeleted)
}

// GetByKode fetches by business PK.
func (r *DBRepository) GetByKode(ctx context.Context, kode string, includeDeleted bool) (*ChartOfAccount, error) {
	return r.getOne(ctx, r.db, "kode_akun = $1", kode, includeDeleted)
}

func (r *DBRepository) getOne(ctx context.Context, q querier, where string, arg interface{}, includeDeleted bool) (*ChartOfAccount, error) {
	deletedFilter := " AND deleted_at IS NULL"
	if includeDeleted {
		deletedFilter = ""
	}
	query := baseSelect + " WHERE " + where + deletedFilter
	row := q.QueryRowContext(ctx, query, arg)
	c, err := scanRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.getOne coa: %w", err)
	}
	return c, nil
}

// List fetches paginated records.
func (r *DBRepository) List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*ChartOfAccount, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("t")

	conditions := []string{}
	if !includeDeleted {
		conditions = append(conditions, "t.deleted_at IS NULL")
	}

	if cursor != "" {
		cd, err := pagination.DecodeCursor(cursor)
		if err == nil && cd.ID != "" {
			argIdx := len(args) + 1
			conditions = append(conditions, fmt.Sprintf("t.kode_akun > $%d", argIdx))
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
		orderBy = "t.kode_akun ASC"
	}

	argIdx := len(args) + 1
	query := fmt.Sprintf( //nolint:gosec
		`SELECT t.id, t.kode_akun, t.nama_akun, t.tipe_akun, t.sub_tipe_akun,
		    t.kategori_investasi, t.mata_uang_native, t.posisi_normal,
		    t.aktif_flag, t.parent_akun_id, t.sumber_coa, t.tanggal_mulai_aktif,
		    t.created_by, t.created_at, t.updated_by, t.updated_at,
		    t.deleted_at, t.deleted_by, t.version, t.tenant_id, t.workflow_status
		FROM mst.chart_of_accounts t%s ORDER BY %s LIMIT $%d`,
		whereClause, orderBy, argIdx,
	)
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.List coa: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []*ChartOfAccount
	for rows.Next() {
		c, err := scanRows(rows)
		if err != nil {
			return nil, fmt.Errorf("repo.List coa scan: %w", err)
		}
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo.List coa rows.Err: %w", err)
	}
	return items, nil
}

// Update applies partial field changes with optimistic lock on 'version'.
func (r *DBRepository) Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, f UpdateFields) (*ChartOfAccount, error) {
	setClauses := []string{}
	args := []interface{}{}
	idx := 1

	if f.NamaAkun != nil {
		setClauses = append(setClauses, fmt.Sprintf("nama_akun = $%d", idx))
		args = append(args, *f.NamaAkun)
		idx++
	}
	if f.SubTipeAkun != nil {
		setClauses = append(setClauses, fmt.Sprintf("sub_tipe_akun = $%d", idx))
		args = append(args, *f.SubTipeAkun)
		idx++
	}
	if f.KategoriInvestasi != nil {
		setClauses = append(setClauses, fmt.Sprintf("kategori_investasi = $%d", idx))
		args = append(args, *f.KategoriInvestasi)
		idx++
	}
	if f.MataUangNative != nil {
		setClauses = append(setClauses, fmt.Sprintf("mata_uang_native = $%d", idx))
		args = append(args, *f.MataUangNative)
		idx++
	}
	if f.PosisiNormal != nil {
		setClauses = append(setClauses, fmt.Sprintf("posisi_normal = $%d", idx))
		args = append(args, string(*f.PosisiNormal))
		idx++
	}
	if f.AktifFlag != nil {
		setClauses = append(setClauses, fmt.Sprintf("aktif_flag = $%d", idx))
		args = append(args, *f.AktifFlag)
		idx++
	}
	if f.ClearParent {
		setClauses = append(setClauses, "parent_akun_id = NULL")
	} else if f.ParentAkunID != nil {
		setClauses = append(setClauses, fmt.Sprintf("parent_akun_id = $%d", idx))
		args = append(args, *f.ParentAkunID)
		idx++
	}
	if f.SumberCoa != nil {
		setClauses = append(setClauses, fmt.Sprintf("sumber_coa = $%d", idx))
		args = append(args, *f.SumberCoa)
		idx++
	}
	if f.TanggalMulaiAktif != nil {
		setClauses = append(setClauses, fmt.Sprintf("tanggal_mulai_aktif = $%d", idx))
		args = append(args, *f.TanggalMulaiAktif)
		idx++
	}
	if len(setClauses) == 0 {
		return r.GetByID(ctx, id, false)
	}

	setClauses = append(setClauses,
		fmt.Sprintf("updated_at = $%d", idx),
		fmt.Sprintf("updated_by = $%d", idx+1),
		"version = version + 1",
	)
	args = append(args, time.Now(), f.UpdatedBy)
	idx += 2

	whereIDIdx := idx
	whereVersionIdx := idx + 1
	args = append(args, id, f.ExpectedVersion)

	query := fmt.Sprintf( //nolint:gosec
		`UPDATE mst.chart_of_accounts SET %s WHERE id = $%d AND version = $%d AND deleted_at IS NULL`,
		strings.Join(setClauses, ", "),
		whereIDIdx, whereVersionIdx,
	)

	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.Update coa: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("repo.Update coa rows affected: %w", err)
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
		UPDATE mst.chart_of_accounts
		SET workflow_status = $1, updated_at = now(), updated_by = $2, version = version + 1
		WHERE id = $3
	`, string(status), updatedBy, id)
	if err != nil {
		return fmt.Errorf("repo.UpdateWorkflowStatus coa: %w", err)
	}
	return nil
}

// SoftDelete marks the record as deleted.
func (r *DBRepository) SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*ChartOfAccount, error) {
	now := time.Now()
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.chart_of_accounts
		SET deleted_at = $1, deleted_by = $2, updated_at = $1, updated_by = $2
		WHERE id = $3 AND deleted_at IS NULL
	`, now, deletedBy, id)
	if err != nil {
		return nil, fmt.Errorf("repo.SoftDelete coa: %w", err)
	}
	return r.getOne(ctx, tx, "id = $1", id, true)
}

// CountChildrenOf counts non-deleted child records for the given parent.
func (r *DBRepository) CountChildrenOf(ctx context.Context, id uuid.UUID) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM mst.chart_of_accounts
		WHERE parent_akun_id = $1 AND deleted_at IS NULL
	`, id).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("repo.CountChildrenOf coa: %w", err)
	}
	return count, nil
}

// BeginTx starts a transaction.
func (r *DBRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("repo.BeginTx coa: %w", err)
	}
	return tx, nil
}

// ListAuditHistory returns paginated audit events.
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
	args = append(args, entityID, "mst.chart_of_accounts")
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
		return nil, false, fmt.Errorf("repo.ListAuditHistory coa: %w", err)
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
			return nil, false, fmt.Errorf("repo.ListAuditHistory coa scan: %w", err)
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
		return nil, false, fmt.Errorf("repo.ListAuditHistory coa rows.Err: %w", err)
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
		orderBy = "t.kode_akun ASC"
	}

	query := fmt.Sprintf( //nolint:gosec
		`SELECT t.kode_akun, t.nama_akun, t.tipe_akun, t.sub_tipe_akun,
		    t.kategori_investasi, t.mata_uang_native, t.posisi_normal,
		    t.aktif_flag, t.sumber_coa, t.tanggal_mulai_aktif, t.workflow_status
		FROM mst.chart_of_accounts t%s ORDER BY %s`,
		whereClause, orderBy,
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll coa: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	w := csv.NewWriter(&buf)

	headers := []string{
		"Kode Akun", "Nama Akun", "Tipe Akun", "Sub Tipe Akun",
		"Kategori Investasi", "Mata Uang Native", "Posisi Normal",
		"Status", "Sumber CoA", "Tgl Mulai Aktif", "Workflow Status",
	}
	if err := w.Write(headers); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll coa write header: %w", err)
	}

	count := 0
	for rows.Next() {
		var (
			kode     string
			nama     string
			tipe     string
			subTipe  string
			kategori *string
			mataUang string
			posisi   string
			aktif    bool
			sumber   string
			tanggal  string
			wfStatus string
		)
		if err := rows.Scan(&kode, &nama, &tipe, &subTipe, &kategori, &mataUang,
			&posisi, &aktif, &sumber, &tanggal, &wfStatus); err != nil {
			return nil, 0, fmt.Errorf("repo.ExportAll coa scan: %w", err)
		}
		aktifStr := "Aktif"
		if !aktif {
			aktifStr = "Tidak Aktif"
		}
		kat := ""
		if kategori != nil {
			kat = *kategori
		}
		record := []string{kode, nama, tipe, subTipe, kat, mataUang, posisi, aktifStr, sumber, tanggal, wfStatus}
		if err := w.Write(record); err != nil {
			return nil, 0, fmt.Errorf("repo.ExportAll coa write record: %w", err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll coa rows.Err: %w", err)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll coa flush: %w", err)
	}
	return &buf, count, nil
}

// ─── Scan helpers ─────────────────────────────────────────────────────────────

type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

func scanRow(row *sql.Row) (*ChartOfAccount, error) {
	c := &ChartOfAccount{}
	var (
		tipeStr    string
		posisiStr  string
		wfStr      string
		tanggalStr string
		updatedBy  *uuid.UUID
		updatedAt  *time.Time
		deletedAt  *time.Time
		deletedBy  *uuid.UUID
	)
	err := row.Scan(
		&c.ID, &c.KodeAkun, &c.NamaAkun, &tipeStr, &c.SubTipeAkun,
		&c.KategoriInvestasi, &c.MataUangNative, &posisiStr,
		&c.AktifFlag, &c.ParentAkunID, &c.SumberCoa, &tanggalStr,
		&c.CreatedBy, &c.CreatedAt, &updatedBy, &updatedAt,
		&deletedAt, &deletedBy, &c.Version, &c.TenantID, &wfStr,
	)
	if err != nil {
		return nil, err
	}
	c.TipeAkun = TipeAkun(tipeStr)
	c.PosisiNormal = PosisiNormal(posisiStr)
	c.WorkflowStatus = WorkflowStatus(wfStr)
	c.TanggalMulaiAktif = tanggalStr
	c.UpdatedBy = updatedBy
	c.UpdatedAt = updatedAt
	c.DeletedAt = deletedAt
	c.DeletedBy = deletedBy
	return c, nil
}

func scanRows(rows *sql.Rows) (*ChartOfAccount, error) {
	c := &ChartOfAccount{}
	var (
		tipeStr    string
		posisiStr  string
		wfStr      string
		tanggalStr string
		updatedBy  *uuid.UUID
		updatedAt  *time.Time
		deletedAt  *time.Time
		deletedBy  *uuid.UUID
	)
	err := rows.Scan(
		&c.ID, &c.KodeAkun, &c.NamaAkun, &tipeStr, &c.SubTipeAkun,
		&c.KategoriInvestasi, &c.MataUangNative, &posisiStr,
		&c.AktifFlag, &c.ParentAkunID, &c.SumberCoa, &tanggalStr,
		&c.CreatedBy, &c.CreatedAt, &updatedBy, &updatedAt,
		&deletedAt, &deletedBy, &c.Version, &c.TenantID, &wfStr,
	)
	if err != nil {
		return nil, err
	}
	c.TipeAkun = TipeAkun(tipeStr)
	c.PosisiNormal = PosisiNormal(posisiStr)
	c.WorkflowStatus = WorkflowStatus(wfStr)
	c.TanggalMulaiAktif = tanggalStr
	c.UpdatedBy = updatedBy
	c.UpdatedAt = updatedAt
	c.DeletedAt = deletedAt
	c.DeletedBy = deletedBy
	return c, nil
}

// isUniqueViolation checks for PostgreSQL unique constraint violation (error code 23505).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "23505") ||
		strings.Contains(s, "duplicate key") ||
		strings.Contains(s, "unique constraint")
}
