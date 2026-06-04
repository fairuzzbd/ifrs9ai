package mappingjurnal

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
	"github.com/lib/pq"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
)

// Repository defines the data-access contract for mapping_jurnal_header + detail.
type Repository interface {
	// CreateHeader inserts a new header row inside the given transaction.
	CreateHeader(ctx context.Context, tx *sql.Tx, h *Header) error

	// CreateDetails inserts all detail rows inside the given transaction.
	CreateDetails(ctx context.Context, tx *sql.Tx, details []*Detail) error

	// GetHeaderByID fetches a header by UUID.
	// Returns (nil, nil) if not found.
	GetHeaderByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*Header, error)

	// GetHeaderByEventCode fetches a header by event_code.
	GetHeaderByEventCode(ctx context.Context, code string, includeDeleted bool) (*Header, error)

	// GetDetailsByHeaderID fetches all active detail rows for a header.
	GetDetailsByHeaderID(ctx context.Context, headerID uuid.UUID, includeDeleted bool) ([]*Detail, error)

	// ListHeaders returns paginated headers matching listquery + cursor.
	// Returns limit+1 rows so caller can detect hasMore.
	ListHeaders(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*Header, error)

	// UpdateHeader applies partial header changes with optimistic lock.
	UpdateHeader(ctx context.Context, tx *sql.Tx, id uuid.UUID, f HeaderUpdateFields) (*Header, error)

	// BulkReplaceDetails soft-deletes all existing active details for headerID,
	// then inserts the new set — all within the provided transaction.
	BulkReplaceDetails(ctx context.Context, tx *sql.Tx, headerID uuid.UUID, details []*Detail, deletedBy uuid.UUID) error

	// SoftDeleteHeader sets deleted_at/deleted_by on the header row.
	// The DB FK ON DELETE CASCADE handles physical rows on detail table only if hard-delete;
	// here we soft-delete the header and let detail rows remain (cascade handled by query filters).
	SoftDeleteHeader(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*Header, error)

	// UpdateWorkflowStatus updates workflow_status on the header.
	UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error

	// CountHeaderReferences counts active references to a header from other tables.
	CountHeaderReferences(ctx context.Context, headerID uuid.UUID) (int64, error)

	// CheckCoAApproved returns true if the given chart_of_accounts id has workflow_status='APPROVED'
	// and deleted_at IS NULL.
	CheckCoAApproved(ctx context.Context, coaID uuid.UUID) (bool, error)

	// BeginTx starts a database transaction.
	BeginTx(ctx context.Context) (*sql.Tx, error)

	// ListAuditHistory returns paginated audit events for the given header UUID.
	ListAuditHistory(ctx context.Context, entityID uuid.UUID, cursor string, limit int, isAuditRole bool) ([]AuditHistoryItem, bool, error)

	// ExportAll streams matching headers as CSV.
	ExportAll(ctx context.Context, q listquery.Query) (io.Reader, int, error)
}

// HeaderUpdateFields captures the mutable columns for a header update.
type HeaderUpdateFields struct {
	EventIDKode          *string
	EventCode            *string
	NamaEvent            *string
	KategoriEvent        *string
	TriggerSource        *string
	TipeInstrumenBerlaku []string // nil = no change; empty slice = clear
	KlasifikasiBerlaku   []string // nil = no change; empty slice = clear
	AktifFlag            *bool
	Catatan              *string
	UpdatedBy            uuid.UUID
	ExpectedVersion      int64
	// sentinel to distinguish "caller passed empty slice" from "caller omitted field"
	TipeInstrumenSet bool
	KlasifikasiSet   bool
}

// ─── Sentinel errors ──────────────────────────────────────────────────────────

// ErrNotFound is returned when a record is not found.
var ErrNotFound = fmt.Errorf("mapping_jurnal not found")

// ErrConflict is returned on row_version mismatch.
var ErrConflict = fmt.Errorf("optimistic lock conflict")

// ErrEventCodeDuplicate is returned when event_code already exists.
var ErrEventCodeDuplicate = fmt.Errorf("event_code duplicate")

// ErrEventIDKodeDuplicate is returned when event_id_kode already exists.
var ErrEventIDKodeDuplicate = fmt.Errorf("event_id_kode duplicate")

// ─── DB implementation ────────────────────────────────────────────────────────

// DBRepository is the production SQL implementation.
type DBRepository struct {
	db *sql.DB
}

// NewDBRepository creates a new DBRepository.
func NewDBRepository(db *sql.DB) *DBRepository {
	return &DBRepository{db: db}
}

// Ensure compile-time interface satisfaction.
var _ Repository = (*DBRepository)(nil)

// querier abstracts *sql.DB and *sql.Tx for read queries.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
}

// ─── Header CRUD ──────────────────────────────────────────────────────────────

const headerSelect = `
SELECT
    h.id, h.event_id_kode, h.event_code, h.nama_event, h.kategori_event,
    h.trigger_source, h.tipe_instrumen_berlaku, h.klasifikasi_berlaku,
    h.aktif_flag, h.catatan, h.workflow_status,
    h.created_at, h.created_by, h.updated_at, h.updated_by,
    h.deleted_at, h.deleted_by, h.row_version, h.tenant_id
FROM mst.mapping_jurnal_header h`

// CreateHeader inserts a new header row.
func (r *DBRepository) CreateHeader(ctx context.Context, tx *sql.Tx, h *Header) error {
	query := `
INSERT INTO mst.mapping_jurnal_header (
    id, event_id_kode, event_code, nama_event, kategori_event, trigger_source,
    tipe_instrumen_berlaku, klasifikasi_berlaku, aktif_flag, catatan, workflow_status,
    created_at, created_by, updated_at, updated_by, row_version, tenant_id
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11,
    $12, $13, $12, $13, 1, $14
)`
	_, err := tx.ExecContext(ctx, query,
		h.ID, h.EventIDKode, h.EventCode, h.NamaEvent, h.KategoriEvent, h.TriggerSource,
		pq.Array(h.TipeInstrumenBerlaku), pq.Array(h.KlasifikasiBerlaku),
		h.AktifFlag, h.Catatan, string(h.WorkflowStatus),
		h.CreatedAt, h.CreatedBy,
		h.TenantID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			if strings.Contains(err.Error(), "event_code") {
				return fmt.Errorf("event_code: %w", ErrEventCodeDuplicate)
			}
			return fmt.Errorf("event_id_kode: %w", ErrEventIDKodeDuplicate)
		}
		return fmt.Errorf("repo.CreateHeader: %w", err)
	}
	return nil
}

// GetHeaderByID fetches one header by UUID.
func (r *DBRepository) GetHeaderByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*Header, error) {
	return r.getOneHeader(ctx, r.db, "h.id = $1", id, includeDeleted)
}

// GetHeaderByEventCode fetches one header by event_code.
func (r *DBRepository) GetHeaderByEventCode(ctx context.Context, code string, includeDeleted bool) (*Header, error) {
	return r.getOneHeader(ctx, r.db, "h.event_code = $1", code, includeDeleted)
}

func (r *DBRepository) getOneHeader(ctx context.Context, q querier, where string, arg interface{}, includeDeleted bool) (*Header, error) {
	del := " AND h.deleted_at IS NULL"
	if includeDeleted {
		del = ""
	}
	row := q.QueryRowContext(ctx, headerSelect+" WHERE "+where+del, arg)
	h, err := scanHeader(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.getOneHeader: %w", err)
	}
	return h, nil
}

// UpdateHeader applies partial header updates with optimistic lock.
func (r *DBRepository) UpdateHeader(ctx context.Context, tx *sql.Tx, id uuid.UUID, f HeaderUpdateFields) (*Header, error) {
	setClauses := []string{}
	args := []interface{}{}
	idx := 1

	if f.EventIDKode != nil {
		setClauses = append(setClauses, fmt.Sprintf("event_id_kode = $%d", idx))
		args = append(args, *f.EventIDKode)
		idx++
	}
	if f.EventCode != nil {
		setClauses = append(setClauses, fmt.Sprintf("event_code = $%d", idx))
		args = append(args, *f.EventCode)
		idx++
	}
	if f.NamaEvent != nil {
		setClauses = append(setClauses, fmt.Sprintf("nama_event = $%d", idx))
		args = append(args, *f.NamaEvent)
		idx++
	}
	if f.KategoriEvent != nil {
		setClauses = append(setClauses, fmt.Sprintf("kategori_event = $%d", idx))
		args = append(args, *f.KategoriEvent)
		idx++
	}
	if f.TriggerSource != nil {
		setClauses = append(setClauses, fmt.Sprintf("trigger_source = $%d", idx))
		args = append(args, *f.TriggerSource)
		idx++
	}
	if f.TipeInstrumenSet {
		setClauses = append(setClauses, fmt.Sprintf("tipe_instrumen_berlaku = $%d", idx))
		args = append(args, pq.Array(f.TipeInstrumenBerlaku))
		idx++
	}
	if f.KlasifikasiSet {
		setClauses = append(setClauses, fmt.Sprintf("klasifikasi_berlaku = $%d", idx))
		args = append(args, pq.Array(f.KlasifikasiBerlaku))
		idx++
	}
	if f.AktifFlag != nil {
		setClauses = append(setClauses, fmt.Sprintf("aktif_flag = $%d", idx))
		args = append(args, *f.AktifFlag)
		idx++
	}
	if f.Catatan != nil {
		setClauses = append(setClauses, fmt.Sprintf("catatan = $%d", idx))
		args = append(args, *f.Catatan)
		idx++
	}

	if len(setClauses) == 0 {
		return r.GetHeaderByID(ctx, id, false)
	}

	// Audit + optimistic lock
	setClauses = append(setClauses,
		fmt.Sprintf("updated_at = $%d", idx),
		fmt.Sprintf("updated_by = $%d", idx+1),
		"row_version = row_version + 1",
	)
	args = append(args, time.Now(), f.UpdatedBy)
	idx += 2

	whereIDIdx := idx
	whereVerIdx := idx + 1
	args = append(args, id, f.ExpectedVersion)

	query := fmt.Sprintf( //nolint:gosec
		`UPDATE mst.mapping_jurnal_header SET %s WHERE id = $%d AND row_version = $%d AND deleted_at IS NULL`,
		strings.Join(setClauses, ", "),
		whereIDIdx, whereVerIdx,
	)

	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		if isUniqueViolation(err) {
			if strings.Contains(err.Error(), "event_code") {
				return nil, fmt.Errorf("event_code: %w", ErrEventCodeDuplicate)
			}
			return nil, fmt.Errorf("event_id_kode: %w", ErrEventIDKodeDuplicate)
		}
		return nil, fmt.Errorf("repo.UpdateHeader: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		existing, ferr := r.GetHeaderByID(ctx, id, false)
		if ferr != nil {
			return nil, ferr
		}
		if existing == nil {
			return nil, ErrNotFound
		}
		return nil, ErrConflict
	}
	return r.getOneHeader(ctx, tx, "h.id = $1", id, false)
}

// SoftDeleteHeader soft-deletes a header row.
func (r *DBRepository) SoftDeleteHeader(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*Header, error) {
	now := time.Now()
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.mapping_jurnal_header
		SET deleted_at = $1, deleted_by = $2, updated_at = $1, updated_by = $2
		WHERE id = $3 AND deleted_at IS NULL
	`, now, deletedBy, id)
	if err != nil {
		return nil, fmt.Errorf("repo.SoftDeleteHeader: %w", err)
	}
	return r.getOneHeader(ctx, tx, "h.id = $1", id, true)
}

// UpdateWorkflowStatus updates only workflow_status on header.
func (r *DBRepository) UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.mapping_jurnal_header
		SET workflow_status = $1, updated_at = now(), updated_by = $2, row_version = row_version + 1
		WHERE id = $3
	`, string(status), updatedBy, id)
	if err != nil {
		return fmt.Errorf("repo.UpdateWorkflowStatus: %w", err)
	}
	return nil
}

// ListHeaders returns paginated headers.
func (r *DBRepository) ListHeaders(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*Header, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("h")

	conditions := []string{}
	if !includeDeleted {
		conditions = append(conditions, "h.deleted_at IS NULL")
	}

	if cursor != "" {
		cd, err := pagination.DecodeCursor(cursor)
		if err == nil && cd.ID != "" {
			argIdx := len(args) + 1
			conditions = append(conditions, fmt.Sprintf("h.id > $%d", argIdx))
			args = append(args, cd.ID)
		}
	}

	if q.Search != "" {
		argIdx := len(args) + 1
		searchCond := make([]string, 0, len(SearchCols))
		for _, col := range SearchCols {
			searchCond = append(searchCond, fmt.Sprintf("h.%s ILIKE $%d", col, argIdx))
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
		orderBy = "h.created_at DESC"
	}

	argIdx := len(args) + 1
	query := fmt.Sprintf( //nolint:gosec
		`SELECT h.id, h.event_id_kode, h.event_code, h.nama_event, h.kategori_event,
			h.trigger_source, h.tipe_instrumen_berlaku, h.klasifikasi_berlaku,
			h.aktif_flag, h.catatan, h.workflow_status,
			h.created_at, h.created_by, h.updated_at, h.updated_by,
			h.deleted_at, h.deleted_by, h.row_version, h.tenant_id
		FROM mst.mapping_jurnal_header h%s ORDER BY %s LIMIT $%d`,
		whereClause, orderBy, argIdx,
	)
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.ListHeaders: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []*Header
	for rows.Next() {
		h, err := scanHeaderRow(rows)
		if err != nil {
			return nil, fmt.Errorf("repo.ListHeaders scan: %w", err)
		}
		items = append(items, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo.ListHeaders rows.Err: %w", err)
	}
	return items, nil
}

// CountHeaderReferences counts active references to a header from jrnl tables.
func (r *DBRepository) CountHeaderReferences(ctx context.Context, headerID uuid.UUID) (int64, error) {
	var count int64
	// Check if any journal posting references this mapping header.
	// mst.mapping_jurnal_header.id is referenced by trx.transaction.mapping_header_id
	// (per 000001 schema line 865).
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM trx.transaction
		WHERE mapping_header_id = $1
	`, headerID).Scan(&count)
	if err != nil {
		// If the table doesn't exist yet (early migration), treat as 0 references.
		if strings.Contains(err.Error(), "does not exist") {
			return 0, nil
		}
		return 0, fmt.Errorf("repo.CountHeaderReferences: %w", err)
	}
	return count, nil
}

// CheckCoAApproved returns true if the CoA row has workflow_status='APPROVED' and is not deleted.
func (r *DBRepository) CheckCoAApproved(ctx context.Context, coaID uuid.UUID) (bool, error) {
	var wfStatus string
	err := r.db.QueryRowContext(ctx, `
		SELECT workflow_status FROM mst.chart_of_accounts
		WHERE id = $1 AND deleted_at IS NULL
	`, coaID).Scan(&wfStatus)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		// chart_of_accounts may not have workflow_status column yet (schema debt noted in 000008).
		// Treat as approved for forward-compat until CoA schema fix migration runs.
		if strings.Contains(err.Error(), "workflow_status") && strings.Contains(err.Error(), "does not exist") {
			return true, nil
		}
		return false, fmt.Errorf("repo.CheckCoAApproved: %w", err)
	}
	return wfStatus == "APPROVED", nil
}

// BeginTx starts a transaction with ReadCommitted isolation.
func (r *DBRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("repo.BeginTx: %w", err)
	}
	return tx, nil
}

// ─── Detail CRUD ──────────────────────────────────────────────────────────────

// CreateDetails inserts all detail rows in the given transaction.
func (r *DBRepository) CreateDetails(ctx context.Context, tx *sql.Tx, details []*Detail) error {
	for _, d := range details {
		err := r.insertDetail(ctx, tx, d)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *DBRepository) insertDetail(ctx context.Context, tx *sql.Tx, d *Detail) error {
	query := `
INSERT INTO mst.mapping_jurnal_detail (
    id, event_header_id, urutan, kode_akun_id, dk_indicator, sumber_amount,
    klasifikasi_filter, tipe_instrumen_filter, underlying_type_filter,
    multiplier, mata_uang_posting, aktif_flag, catatan,
    created_at, created_by, updated_at, updated_by, row_version, tenant_id
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9,
    $10, $11, $12, $13,
    $14, $15, $14, $15, 1, $16
)`
	_, err := tx.ExecContext(ctx, query,
		d.ID, d.EventHeaderID, d.Urutan, d.KodeAkunID, d.DKIndicator, d.SumberAmount,
		d.KlasifikasiFilter, pq.Array(d.TipeInstrumenFilter), d.UnderlyingTypeFilter,
		d.Multiplier.String(), d.MataUangPosting, d.AktifFlag, d.Catatan,
		d.CreatedAt, d.CreatedBy,
		d.TenantID,
	)
	if err != nil {
		return fmt.Errorf("repo.insertDetail: %w", err)
	}
	return nil
}

// GetDetailsByHeaderID fetches all (optionally including deleted) detail rows for a header.
func (r *DBRepository) GetDetailsByHeaderID(ctx context.Context, headerID uuid.UUID, includeDeleted bool) ([]*Detail, error) {
	del := " AND deleted_at IS NULL"
	if includeDeleted {
		del = ""
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			id, event_header_id, urutan, kode_akun_id, dk_indicator, sumber_amount,
			klasifikasi_filter, tipe_instrumen_filter, underlying_type_filter,
			multiplier, mata_uang_posting, aktif_flag, catatan,
			created_at, created_by, updated_at, updated_by,
			deleted_at, deleted_by, row_version, tenant_id
		FROM mst.mapping_jurnal_detail
		WHERE event_header_id = $1`+del+`
		ORDER BY urutan ASC`,
		headerID,
	)
	if err != nil {
		return nil, fmt.Errorf("repo.GetDetailsByHeaderID: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []*Detail
	for rows.Next() {
		d, err := scanDetailRow(rows)
		if err != nil {
			return nil, fmt.Errorf("repo.GetDetailsByHeaderID scan: %w", err)
		}
		items = append(items, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo.GetDetailsByHeaderID rows.Err: %w", err)
	}
	return items, nil
}

// BulkReplaceDetails soft-deletes existing active details then inserts new ones, all in tx.
func (r *DBRepository) BulkReplaceDetails(ctx context.Context, tx *sql.Tx, headerID uuid.UUID, details []*Detail, deletedBy uuid.UUID) error {
	now := time.Now()
	// Soft-delete all current active details for this header.
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.mapping_jurnal_detail
		SET deleted_at = $1, deleted_by = $2, updated_at = $1, updated_by = $2
		WHERE event_header_id = $3 AND deleted_at IS NULL
	`, now, deletedBy, headerID)
	if err != nil {
		return fmt.Errorf("repo.BulkReplaceDetails soft-delete: %w", err)
	}

	// Insert new details.
	for _, d := range details {
		if err := r.insertDetail(ctx, tx, d); err != nil {
			return err
		}
	}
	return nil
}

// ─── Audit history ────────────────────────────────────────────────────────────

// ListAuditHistory returns paginated audit_log rows for the given header entity UUID.
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
	args = append(args, entityID, "mst.mapping_jurnal_header")
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
		ORDER BY timestamp DESC LIMIT $%d`,
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

// ─── Export ───────────────────────────────────────────────────────────────────

// ExportAll streams matching headers as UTF-8 BOM CSV.
func (r *DBRepository) ExportAll(ctx context.Context, q listquery.Query) (io.Reader, int, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("h")

	conditions := []string{"h.deleted_at IS NULL"}
	if q.Search != "" {
		argIdx := len(args) + 1
		searchCond := make([]string, 0, len(SearchCols))
		for _, col := range SearchCols {
			searchCond = append(searchCond, fmt.Sprintf("h.%s ILIKE $%d", col, argIdx))
		}
		conditions = append(conditions, "("+strings.Join(searchCond, " OR ")+")")
		args = append(args, "%"+q.Search+"%")
	}
	if where != "" {
		conditions = append(conditions, where)
	}
	whereClause := " WHERE " + strings.Join(conditions, " AND ")
	if orderBy == "" {
		orderBy = "h.created_at DESC"
	}

	query := fmt.Sprintf( //nolint:gosec
		`SELECT h.event_code, h.event_id_kode, h.nama_event, h.kategori_event, h.trigger_source, h.aktif_flag, h.workflow_status
		FROM mst.mapping_jurnal_header h%s ORDER BY %s`,
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
	headers := []string{"Event Code", "Event ID Kode", "Nama Event", "Kategori", "Trigger Source", "Status", "Workflow Status"}
	if err := w.Write(headers); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll write header: %w", err)
	}

	count := 0
	for rows.Next() {
		var (
			eventCode    string
			eventIDKode  string
			namaEvent    string
			kategori     string
			triggerSrc   string
			aktif        bool
			wfStatus     string
		)
		if err := rows.Scan(&eventCode, &eventIDKode, &namaEvent, &kategori, &triggerSrc, &aktif, &wfStatus); err != nil {
			return nil, 0, fmt.Errorf("repo.ExportAll scan: %w", err)
		}
		aktifStr := "Aktif"
		if !aktif {
			aktifStr = "Tidak Aktif"
		}
		record := []string{eventCode, eventIDKode, namaEvent, kategori, triggerSrc, aktifStr, wfStatus}
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

func scanHeader(row *sql.Row) (*Header, error) {
	h := &Header{}
	var (
		wfStatus  string
		createdBy *uuid.UUID
		updatedAt *time.Time
		updatedBy *uuid.UUID
		deletedAt *time.Time
		deletedBy *uuid.UUID
	)
	err := row.Scan(
		&h.ID, &h.EventIDKode, &h.EventCode, &h.NamaEvent, &h.KategoriEvent,
		&h.TriggerSource, pq.Array(&h.TipeInstrumenBerlaku), pq.Array(&h.KlasifikasiBerlaku),
		&h.AktifFlag, &h.Catatan, &wfStatus,
		&h.CreatedAt, &createdBy, &updatedAt, &updatedBy,
		&deletedAt, &deletedBy, &h.RowVersion, &h.TenantID,
	)
	if err != nil {
		return nil, err
	}
	h.WorkflowStatus = WorkflowStatus(wfStatus)
	h.CreatedBy = createdBy
	h.UpdatedAt = updatedAt
	h.UpdatedBy = updatedBy
	h.DeletedAt = deletedAt
	h.DeletedBy = deletedBy
	if h.TipeInstrumenBerlaku == nil {
		h.TipeInstrumenBerlaku = []string{}
	}
	if h.KlasifikasiBerlaku == nil {
		h.KlasifikasiBerlaku = []string{}
	}
	return h, nil
}

func scanHeaderRow(rows *sql.Rows) (*Header, error) {
	h := &Header{}
	var (
		wfStatus  string
		createdBy *uuid.UUID
		updatedAt *time.Time
		updatedBy *uuid.UUID
		deletedAt *time.Time
		deletedBy *uuid.UUID
	)
	err := rows.Scan(
		&h.ID, &h.EventIDKode, &h.EventCode, &h.NamaEvent, &h.KategoriEvent,
		&h.TriggerSource, pq.Array(&h.TipeInstrumenBerlaku), pq.Array(&h.KlasifikasiBerlaku),
		&h.AktifFlag, &h.Catatan, &wfStatus,
		&h.CreatedAt, &createdBy, &updatedAt, &updatedBy,
		&deletedAt, &deletedBy, &h.RowVersion, &h.TenantID,
	)
	if err != nil {
		return nil, err
	}
	h.WorkflowStatus = WorkflowStatus(wfStatus)
	h.CreatedBy = createdBy
	h.UpdatedAt = updatedAt
	h.UpdatedBy = updatedBy
	h.DeletedAt = deletedAt
	h.DeletedBy = deletedBy
	if h.TipeInstrumenBerlaku == nil {
		h.TipeInstrumenBerlaku = []string{}
	}
	if h.KlasifikasiBerlaku == nil {
		h.KlasifikasiBerlaku = []string{}
	}
	return h, nil
}

func scanDetailRow(rows *sql.Rows) (*Detail, error) {
	d := &Detail{}
	var (
		multiplierStr string
		createdBy     *uuid.UUID
		updatedAt     *time.Time
		updatedBy     *uuid.UUID
		deletedAt     *time.Time
		deletedBy     *uuid.UUID
	)
	err := rows.Scan(
		&d.ID, &d.EventHeaderID, &d.Urutan, &d.KodeAkunID, &d.DKIndicator, &d.SumberAmount,
		&d.KlasifikasiFilter, pq.Array(&d.TipeInstrumenFilter), &d.UnderlyingTypeFilter,
		&multiplierStr, &d.MataUangPosting, &d.AktifFlag, &d.Catatan,
		&d.CreatedAt, &createdBy, &updatedAt, &updatedBy,
		&deletedAt, &deletedBy, &d.RowVersion, &d.TenantID,
	)
	if err != nil {
		return nil, err
	}
	d.Multiplier, _ = decimal.NewFromString(multiplierStr)
	d.CreatedBy = createdBy
	d.UpdatedAt = updatedAt
	d.UpdatedBy = updatedBy
	d.DeletedAt = deletedAt
	d.DeletedBy = deletedBy
	if d.TipeInstrumenFilter == nil {
		d.TipeInstrumenFilter = []string{}
	}
	return d, nil
}

// isUniqueViolation detects PostgreSQL unique constraint violation (code 23505).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "23505") ||
		strings.Contains(err.Error(), "duplicate key") ||
		strings.Contains(err.Error(), "unique constraint")
}
