package matauang

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

// Repository defines the data-access contract for mata_uang.
// REUSE PATTERN: implement this interface per module; service layer only uses this interface.
type Repository interface {
	// Create inserts a new mata_uang row in the given transaction.
	Create(ctx context.Context, tx *sql.Tx, m *MataUang) error

	// GetByKode fetches one record by business PK.
	// Returns (nil, nil) if not found (soft-deleted record returned if includeDeleted=true).
	GetByKode(ctx context.Context, kode string, includeDeleted bool) (*MataUang, error)

	// GetByID fetches one record by surrogate UUID.
	GetByID(ctx context.Context, id uuid.UUID) (*MataUang, error)

	// List fetches paginated records matching listquery + cursor.
	// Returns rows limited to limit+1 (caller detects hasMore).
	List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*MataUang, error)

	// Update applies partial update in the given transaction.
	// Uses optimistic lock: UPDATE ... WHERE row_version = expected AND kode_mata_uang = kode.
	// Returns ErrNotFound if kode not found; ErrConflict if row_version mismatch.
	Update(ctx context.Context, tx *sql.Tx, kode string, fields UpdateFields) (*MataUang, error)

	// SoftDelete sets deleted_at/deleted_by in the given transaction.
	SoftDelete(ctx context.Context, tx *sql.Tx, kode string, deletedBy uuid.UUID) (*MataUang, error)

	// UpdateWorkflowStatus updates workflow_status column (called after workflow transitions).
	UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error

	// CountReferences returns the number of active references to this kode_mata_uang
	// in other tables (instrumen, kurs). Used by delete guard.
	CountReferences(ctx context.Context, kode string) (int64, error)

	// BeginTx starts a database transaction with default isolation.
	BeginTx(ctx context.Context) (*sql.Tx, error)

	// ListAuditHistory returns paginated audit log rows for a given entity UUID.
	ListAuditHistory(ctx context.Context, entityID uuid.UUID, cursor string, limit int, isAuditRole bool) ([]AuditHistoryItem, bool, error)

	// ExportAll returns all records matching the query as an io.Reader CSV stream.
	// Caller must close the reader.
	ExportAll(ctx context.Context, q listquery.Query) (io.Reader, int, error)
}

// UpdateFields captures the mutable columns for an update operation.
// Nil pointer = do not update that column.
type UpdateFields struct {
	NamaMataUang      *string
	Simbol            *string
	DecimalPlaces     *int16
	SumberKursDefault *string
	FrekuensiUpdate   *string
	AktifFlag         *bool
	TanggalMulaiAktif *string
	UpdatedBy         uuid.UUID
	ExpectedVersion   int64
}

// ─── DB implementation ────────────────────────────────────────────────────────

// DBRepository is the production SQL implementation.
// It uses database/sql directly (no GORM) to stay aligned with sqlx-style heavy queries.
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
    kode_mata_uang, id, nama_mata_uang, simbol, decimal_places,
    sumber_kurs_default, frekuensi_update, aktif_flag, tanggal_mulai_aktif,
    is_system_currency, workflow_status,
    created_at, created_by, updated_at, updated_by,
    deleted_at, deleted_by, row_version, tenant_id
FROM mst.mata_uang`

// Create inserts a new row. Called inside a transaction.
func (r *DBRepository) Create(ctx context.Context, tx *sql.Tx, m *MataUang) error {
	query := `
INSERT INTO mst.mata_uang (
    kode_mata_uang, id, nama_mata_uang, simbol, decimal_places,
    sumber_kurs_default, frekuensi_update, aktif_flag, tanggal_mulai_aktif,
    is_system_currency, workflow_status,
    created_at, created_by, updated_at, updated_by,
    row_version, tenant_id
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9,
    $10, $11,
    $12, $13, $12, $13,
    1, $14
)`
	_, err := tx.ExecContext(ctx, query,
		m.KodeMataUang, m.ID, m.NamaMataUang, m.Simbol, m.DecimalPlaces,
		m.SumberKursDefault, m.FrekuensiUpdate, m.AktifFlag, m.TanggalMulaiAktif,
		m.IsSystemCurrency, string(m.WorkflowStatus),
		m.CreatedAt, m.CreatedBy,
		m.TenantID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("kode_mata_uang: %w", ErrKodeDuplicate)
		}
		return fmt.Errorf("repo.Create mata_uang: %w", err)
	}
	return nil
}

// GetByKode fetches by business PK.
func (r *DBRepository) GetByKode(ctx context.Context, kode string, includeDeleted bool) (*MataUang, error) {
	return r.getOne(ctx, r.db, "kode_mata_uang = $1", kode, includeDeleted)
}

// GetByID fetches by surrogate UUID.
func (r *DBRepository) GetByID(ctx context.Context, id uuid.UUID) (*MataUang, error) {
	return r.getOne(ctx, r.db, "id = $1", id, true)
}

func (r *DBRepository) getOne(ctx context.Context, querier querier, where string, arg interface{}, includeDeleted bool) (*MataUang, error) {
	deletedFilter := " AND deleted_at IS NULL"
	if includeDeleted {
		deletedFilter = ""
	}
	query := baseSelect + " WHERE " + where + deletedFilter
	row := querier.QueryRowContext(ctx, query, arg)
	m, err := scanMataUang(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.getOne mata_uang: %w", err)
	}
	return m, nil
}

// List fetches paginated records.
func (r *DBRepository) List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*MataUang, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("t")

	conditions := []string{}
	if !includeDeleted {
		conditions = append(conditions, "t.deleted_at IS NULL")
	}

	// Cursor: simple ID-based cursor (kode_mata_uang alphabetical)
	if cursor != "" {
		cd, err := pagination.DecodeCursor(cursor)
		if err == nil && cd.ID != "" {
			argIdx := len(args) + 1
			conditions = append(conditions, fmt.Sprintf("t.kode_mata_uang > $%d", argIdx))
			args = append(args, cd.ID)
		}
	}

	// Text search across search columns
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
		orderBy = "t.kode_mata_uang ASC"
	}

	// limit+1 trick for hasMore detection.

	argIdx := len(args) + 1
	query := fmt.Sprintf( //nolint:gosec
		"SELECT t.kode_mata_uang, t.id, t.nama_mata_uang, t.simbol, t.decimal_places, t.sumber_kurs_default, t.frekuensi_update, t.aktif_flag, t.tanggal_mulai_aktif, t.is_system_currency, t.workflow_status, t.created_at, t.created_by, t.updated_at, t.updated_by, t.deleted_at, t.deleted_by, t.row_version, t.tenant_id FROM mst.mata_uang t%s ORDER BY %s LIMIT $%d",
		whereClause, orderBy, argIdx,
	)
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.List mata_uang: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []*MataUang
	for rows.Next() {
		m, err := scanMataUangRow(rows)
		if err != nil {
			return nil, fmt.Errorf("repo.List scan: %w", err)
		}
		items = append(items, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo.List rows.Err: %w", err)
	}
	return items, nil
}

// Update applies partial field changes with optimistic lock.
func (r *DBRepository) Update(ctx context.Context, tx *sql.Tx, kode string, f UpdateFields) (*MataUang, error) {
	setClauses := []string{}
	args := []interface{}{}
	idx := 1

	if f.NamaMataUang != nil {
		setClauses = append(setClauses, fmt.Sprintf("nama_mata_uang = $%d", idx))
		args = append(args, *f.NamaMataUang)
		idx++
	}
	if f.Simbol != nil {
		setClauses = append(setClauses, fmt.Sprintf("simbol = $%d", idx))
		args = append(args, *f.Simbol)
		idx++
	}
	if f.DecimalPlaces != nil {
		setClauses = append(setClauses, fmt.Sprintf("decimal_places = $%d", idx))
		args = append(args, *f.DecimalPlaces)
		idx++
	}
	if f.SumberKursDefault != nil {
		setClauses = append(setClauses, fmt.Sprintf("sumber_kurs_default = $%d", idx))
		args = append(args, *f.SumberKursDefault)
		idx++
	}
	if f.FrekuensiUpdate != nil {
		setClauses = append(setClauses, fmt.Sprintf("frekuensi_update = $%d", idx))
		args = append(args, *f.FrekuensiUpdate)
		idx++
	}
	if f.AktifFlag != nil {
		setClauses = append(setClauses, fmt.Sprintf("aktif_flag = $%d", idx))
		args = append(args, *f.AktifFlag)
		idx++
	}
	if f.TanggalMulaiAktif != nil {
		setClauses = append(setClauses, fmt.Sprintf("tanggal_mulai_aktif = $%d", idx))
		args = append(args, *f.TanggalMulaiAktif)
		idx++
	}
	if len(setClauses) == 0 {
		// Nothing to update — return current record
		return r.GetByKode(ctx, kode, false)
	}

	// Append audit + optimistic lock columns
	setClauses = append(setClauses,
		fmt.Sprintf("updated_at = $%d", idx),
		fmt.Sprintf("updated_by = $%d", idx+1),
		"row_version = row_version + 1",
	)
	args = append(args, time.Now(), f.UpdatedBy)
	idx += 2

	// WHERE clause: kode + optimistic lock
	whereKodeIdx := idx
	whereVersionIdx := idx + 1
	args = append(args, kode, f.ExpectedVersion)

	query := fmt.Sprintf( //nolint:gosec
		`UPDATE mst.mata_uang SET %s WHERE kode_mata_uang = $%d AND row_version = $%d AND deleted_at IS NULL`,
		strings.Join(setClauses, ", "),
		whereKodeIdx, whereVersionIdx,
	)

	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.Update mata_uang: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("repo.Update rows affected: %w", err)
	}
	if n == 0 {
		// Could be not found OR version mismatch. Check which one.
		existing, err := r.GetByKode(ctx, kode, false)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, ErrNotFound
		}
		return nil, ErrConflict
	}

	// Return refreshed row within same tx
	return r.getOne(ctx, tx, "kode_mata_uang = $1", kode, false)
}

// UpdateWorkflowStatus updates only the workflow_status column.
func (r *DBRepository) UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.mata_uang
		SET workflow_status = $1, updated_at = now(), updated_by = $2, row_version = row_version + 1
		WHERE id = $3
	`, string(status), updatedBy, id)
	if err != nil {
		return fmt.Errorf("repo.UpdateWorkflowStatus mata_uang: %w", err)
	}
	return nil
}

// SoftDelete marks the record as deleted.
func (r *DBRepository) SoftDelete(ctx context.Context, tx *sql.Tx, kode string, deletedBy uuid.UUID) (*MataUang, error) {
	now := time.Now()
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.mata_uang
		SET deleted_at = $1, deleted_by = $2, updated_at = $1, updated_by = $2
		WHERE kode_mata_uang = $3 AND deleted_at IS NULL
	`, now, deletedBy, kode)
	if err != nil {
		return nil, fmt.Errorf("repo.SoftDelete mata_uang: %w", err)
	}
	return r.getOne(ctx, tx, "kode_mata_uang = $1", kode, true)
}

// CountReferences counts active FK references to this kode from other tables.
func (r *DBRepository) CountReferences(ctx context.Context, kode string) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM mst.instrumen
		WHERE mata_uang = $1 AND deleted_at IS NULL
	`, kode).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("repo.CountReferences mata_uang instrumen: %w", err)
	}
	// Also count from mst.kurs (FX rate references)
	var kursCount int64
	err = r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM mst.kurs WHERE kode_mata_uang = $1
	`, kode).Scan(&kursCount)
	if err != nil {
		return 0, fmt.Errorf("repo.CountReferences mata_uang kurs: %w", err)
	}
	return count + kursCount, nil
}

// BeginTx starts a transaction.
func (r *DBRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("repo.BeginTx: %w", err)
	}
	return tx, nil
}

// ListAuditHistory returns paginated audit events for the given mata_uang entity UUID.
func (r *DBRepository) ListAuditHistory(ctx context.Context, entityID uuid.UUID, cursor string, limit int, isAuditRole bool) ([]AuditHistoryItem, bool, error) {
	// Determine offset from cursor (simple timestamp-based cursor)
	var cursorTime *time.Time
	if cursor != "" {
		cd, err := pagination.DecodeCursor(cursor)
		if err == nil && cd.ID != "" {
			// cursor encodes the last event timestamp as ID field
			t, err2 := time.Parse(time.RFC3339, cd.ID)
			if err2 == nil {
				cursorTime = &t
			}
		}
	}

	var args []interface{}
	args = append(args, entityID, "mst.mata_uang")
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
		orderBy = "t.kode_mata_uang ASC"
	}

	query := fmt.Sprintf( //nolint:gosec
		"SELECT t.kode_mata_uang, t.nama_mata_uang, t.simbol, t.decimal_places, t.sumber_kurs_default, t.frekuensi_update, t.aktif_flag, t.tanggal_mulai_aktif, t.workflow_status FROM mst.mata_uang t%s ORDER BY %s",
		whereClause, orderBy,
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var buf bytes.Buffer
	// Write UTF-8 BOM for Excel compatibility
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(&buf)

	// Header row (Indonesian labels per ux-patterns.md §1.4)
	headers := []string{"Kode", "Nama", "Simbol", "Decimal Places", "Sumber Kurs", "Frekuensi", "Status", "Tgl Mulai Aktif", "Workflow Status"}
	if err := w.Write(headers); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll write header: %w", err)
	}

	count := 0
	for rows.Next() {
		var (
			kode          string
			nama          string
			simbol        string
			decimalPlaces int16
			sumber        string
			frekuensi     string
			aktif         bool
			tanggal       string
			wfStatus      string
		)
		if err := rows.Scan(&kode, &nama, &simbol, &decimalPlaces, &sumber, &frekuensi, &aktif, &tanggal, &wfStatus); err != nil {
			return nil, 0, fmt.Errorf("repo.ExportAll scan: %w", err)
		}

		aktifStr := "Aktif"
		if !aktif {
			aktifStr = "Tidak Aktif"
		}

		record := []string{
			kode, nama, simbol,
			fmt.Sprintf("%d", decimalPlaces),
			sumber, frekuensi, aktifStr, tanggal, wfStatus,
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

// querier abstracts *sql.DB and *sql.Tx for read queries.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// scanMataUang scans one *sql.Row into MataUang.
func scanMataUang(row *sql.Row) (*MataUang, error) {
	m := &MataUang{}
	var (
		tanggalStr     string
		workflowStatus string
		createdBy      *uuid.UUID
		updatedAt      *time.Time
		updatedBy      *uuid.UUID
		deletedAt      *time.Time
		deletedBy      *uuid.UUID
		wfInstanceID   *uuid.UUID
	)
	err := row.Scan(
		&m.KodeMataUang, &m.ID, &m.NamaMataUang, &m.Simbol, &m.DecimalPlaces,
		&m.SumberKursDefault, &m.FrekuensiUpdate, &m.AktifFlag, &tanggalStr,
		&m.IsSystemCurrency, &workflowStatus,
		&m.CreatedAt, &createdBy, &updatedAt, &updatedBy,
		&deletedAt, &deletedBy, &m.RowVersion, &m.TenantID,
	)
	if err != nil {
		return nil, err
	}
	m.TanggalMulaiAktif = tanggalStr
	m.WorkflowStatus = WorkflowStatus(workflowStatus)
	m.CreatedBy = createdBy
	m.UpdatedAt = updatedAt
	m.UpdatedBy = updatedBy
	m.DeletedAt = deletedAt
	m.DeletedBy = deletedBy
	m.WorkflowInstanceID = wfInstanceID
	return m, nil
}

// scanMataUangRow scans one *sql.Rows row into MataUang.
func scanMataUangRow(rows *sql.Rows) (*MataUang, error) {
	m := &MataUang{}
	var (
		tanggalStr     string
		workflowStatus string
		createdBy      *uuid.UUID
		updatedAt      *time.Time
		updatedBy      *uuid.UUID
		deletedAt      *time.Time
		deletedBy      *uuid.UUID
	)
	err := rows.Scan(
		&m.KodeMataUang, &m.ID, &m.NamaMataUang, &m.Simbol, &m.DecimalPlaces,
		&m.SumberKursDefault, &m.FrekuensiUpdate, &m.AktifFlag, &tanggalStr,
		&m.IsSystemCurrency, &workflowStatus,
		&m.CreatedAt, &createdBy, &updatedAt, &updatedBy,
		&deletedAt, &deletedBy, &m.RowVersion, &m.TenantID,
	)
	if err != nil {
		return nil, err
	}
	m.TanggalMulaiAktif = tanggalStr
	m.WorkflowStatus = WorkflowStatus(workflowStatus)
	m.CreatedBy = createdBy
	m.UpdatedAt = updatedAt
	m.UpdatedBy = updatedBy
	m.DeletedAt = deletedAt
	m.DeletedBy = deletedBy
	return m, nil
}

// ─── Module-level sentinel errors (used by repo + service) ────────────────────

// ErrNotFound is returned when a record is not found.
var ErrNotFound = fmt.Errorf("mata_uang not found")

// ErrConflict is returned on row_version mismatch.
var ErrConflict = fmt.Errorf("optimistic lock conflict")

// ErrKodeDuplicate is returned when kode_mata_uang already exists.
var ErrKodeDuplicate = fmt.Errorf("kode_mata_uang duplicate")

// isUniqueViolation checks for PostgreSQL unique constraint violation (error code 23505).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "23505") ||
		strings.Contains(err.Error(), "duplicate key") ||
		strings.Contains(err.Error(), "unique constraint")
}
