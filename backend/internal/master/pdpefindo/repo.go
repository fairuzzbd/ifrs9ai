package pdpefindo

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

// Repository defines the data-access contract for pd_pefindo.
type Repository interface {
	// Create inserts a new pd_pefindo row in the given transaction.
	Create(ctx context.Context, tx *sql.Tx, p *PDPefindo) error

	// GetByID fetches one record by surrogate UUID.
	// Returns (nil, nil) if not found.
	GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*PDPefindo, error)

	// List fetches paginated records.
	// Returns limit+1 rows so caller can detect hasMore.
	List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*PDPefindo, error)

	// Update applies partial update with optimistic lock.
	// Returns ErrNotFound or ErrConflict as sentinel errors.
	Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, f UpdateFields) (*PDPefindo, error)

	// SoftDelete sets deleted_at/deleted_by.
	SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*PDPefindo, error)

	// UpdateWorkflowStatus updates workflow_status column (called after workflow transitions).
	UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error

	// CountOverlap returns the number of records for the given rating that overlap the
	// given period [from, sampai], excluding the optional excludeID.
	// Used for period-overlap validation.
	CountOverlap(ctx context.Context, rating string, dari string, sampai *string, excludeID *uuid.UUID) (int64, error)

	// CountReferences returns active FK references — used by delete guard.
	// pd_pefindo rows are referenced by ecl.ecl_calc_result_line via rating+period.
	CountReferences(ctx context.Context, id uuid.UUID) (int64, error)

	// BeginTx starts a database transaction.
	BeginTx(ctx context.Context) (*sql.Tx, error)

	// ListAuditHistory returns paginated audit log rows for a given entity UUID.
	ListAuditHistory(ctx context.Context, entityID uuid.UUID, cursor string, limit int, isAuditRole bool) ([]AuditHistoryItem, bool, error)

	// ExportAll returns all non-deleted records matching the query as a CSV reader.
	ExportAll(ctx context.Context, q listquery.Query) (io.Reader, int, error)

	// BulkCreate inserts multiple pd_pefindo rows in a single transaction (for XLSX upload).
	// Each row is created in DRAFT state. Returns count of created rows.
	BulkCreate(ctx context.Context, tx *sql.Tx, rows []*PDPefindo) (int, error)

	// GetJobByID fetches a sys.job row by its string ID.
	GetJobByID(ctx context.Context, jobID string) (*JobRow, error)

	// CreateJob inserts a new sys.job row.
	CreateJob(ctx context.Context, tx *sql.Tx, j *JobRow) error

	// UpdateJobProgress updates sys.job.progress + current_step.
	UpdateJobProgress(ctx context.Context, jobID string, progress int, currentStep string) error

	// CompleteJob marks job as completed with result JSON.
	CompleteJob(ctx context.Context, jobID string, resultJSON []byte) error

	// FailJob marks job as failed with error JSON.
	FailJob(ctx context.Context, jobID string, errJSON []byte) error
}

// JobRow is a minimal representation of a sys.job row for pd_pefindo upload jobs.
type JobRow struct {
	ID          string
	Type        string
	Status      string
	Progress    int
	CurrentStep *string
	PayloadJSON []byte
	ResultJSON  []byte
	ErrorJSON   []byte
	StartedAt   *time.Time
	CompletedAt *time.Time
	CanCancel   bool
	CreatedBy   uuid.UUID
	CreatedAt   time.Time
	TenantID    string
}

// UpdateFields captures mutable columns for an update operation.
// Nil pointer = do not update that column.
type UpdateFields struct {
	PD12Month            *decimal.Decimal
	PDLifetime3Y         *decimal.Decimal
	PDLifetime5Y         *decimal.Decimal
	PDLifetime7Y         *decimal.Decimal
	PDLifetime10Y        *decimal.Decimal
	Sumber               *string
	TanggalPublikasi     *string
	PeriodeBerlakuDari   *string
	PeriodeBerlakuSampai *string
	DokumenPendukungID   *string
	UpdatedBy            uuid.UUID
	ExpectedVersion      int64
}

// ─── Sentinel errors ──────────────────────────────────────────────────────────

var (
	ErrNotFound = fmt.Errorf("pd_pefindo not found")
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

// Ensure DBRepository satisfies Repository at compile time.
var _ Repository = (*DBRepository)(nil)

// baseSelect is the SELECT fragment used by all read queries.
// Columns must match scan order in scanPDPefindo / scanPDPefindoRow.
const baseSelect = `
SELECT
    id, rating,
    pd_12month, pd_lifetime_3y, pd_lifetime_5y, pd_lifetime_7y, pd_lifetime_10y,
    sumber, tanggal_publikasi, periode_berlaku_dari, periode_berlaku_sampai,
    dokumen_pendukung_id,
    uploaded_by, uploaded_at, approved_by, approved_at,
    workflow_status,
    created_at, created_by, updated_at, updated_by,
    deleted_at, deleted_by, row_version, tenant_id
FROM mst.pd_pefindo`

// Create inserts a new row inside a transaction.
func (r *DBRepository) Create(ctx context.Context, tx *sql.Tx, p *PDPefindo) error {
	q := `
INSERT INTO mst.pd_pefindo (
    id, rating,
    pd_12month, pd_lifetime_3y, pd_lifetime_5y, pd_lifetime_7y, pd_lifetime_10y,
    sumber, tanggal_publikasi, periode_berlaku_dari, periode_berlaku_sampai,
    dokumen_pendukung_id,
    uploaded_by, uploaded_at, approved_by, approved_at,
    workflow_status,
    created_at, created_by, updated_at, updated_by,
    row_version, tenant_id
) VALUES (
    $1, $2,
    $3, $4, $5, $6, $7,
    $8, $9, $10, $11,
    $12,
    $13, $14, $15, $16,
    $17,
    $18, $19, $18, $19,
    1, $20
)`
	_, err := tx.ExecContext(ctx, q,
		p.ID, p.Rating,
		p.PD12Month.String(), pdDecimalOrNil(p.PDLifetime3Y), pdDecimalOrNil(p.PDLifetime5Y),
		pdDecimalOrNil(p.PDLifetime7Y), pdDecimalOrNil(p.PDLifetime10Y),
		p.Sumber, p.TanggalPublikasi, p.PeriodeBerlakuDari, p.PeriodeBerlakuSampai,
		p.DokumenPendukungID,
		p.UploadedBy, p.UploadedAt, p.ApprovedBy, p.ApprovedAt,
		string(p.WorkflowStatus),
		p.CreatedAt, p.CreatedBy,
		p.TenantID,
	)
	if err != nil {
		return fmt.Errorf("repo.Create pd_pefindo: %w", err)
	}
	return nil
}

// GetByID fetches one record by UUID.
func (r *DBRepository) GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*PDPefindo, error) {
	deletedFilter := " AND deleted_at IS NULL"
	if includeDeleted {
		deletedFilter = ""
	}
	// #nosec G202 — deletedFilter is a constant whitelist (empty or " AND deleted_at IS NULL"); no user input.
	q := baseSelect + " WHERE id = $1" + deletedFilter
	row := r.db.QueryRowContext(ctx, q, id)
	p, err := scanPDPefindo(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.GetByID pd_pefindo: %w", err)
	}
	return p, nil
}

// List fetches paginated records.
func (r *DBRepository) List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*PDPefindo, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("t")

	conditions := []string{}
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
		searchConds := make([]string, 0, len(SearchCols))
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
		orderBy = "t.rating ASC, t.periode_berlaku_dari DESC"
	}

	argIdx := len(args) + 1
	query := fmt.Sprintf( //nolint:gosec
		`SELECT t.id, t.rating,
    t.pd_12month, t.pd_lifetime_3y, t.pd_lifetime_5y, t.pd_lifetime_7y, t.pd_lifetime_10y,
    t.sumber, t.tanggal_publikasi, t.periode_berlaku_dari, t.periode_berlaku_sampai,
    t.dokumen_pendukung_id,
    t.uploaded_by, t.uploaded_at, t.approved_by, t.approved_at,
    t.workflow_status,
    t.created_at, t.created_by, t.updated_at, t.updated_by,
    t.deleted_at, t.deleted_by, t.row_version, t.tenant_id
FROM mst.pd_pefindo t%s ORDER BY %s LIMIT $%d`,
		whereClause, orderBy, argIdx,
	)
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.List pd_pefindo: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []*PDPefindo
	for rows.Next() {
		p, err := scanPDPefindoRow(rows)
		if err != nil {
			return nil, fmt.Errorf("repo.List pd_pefindo scan: %w", err)
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo.List pd_pefindo rows.Err: %w", err)
	}
	return items, nil
}

// Update applies partial field changes with optimistic lock.
func (r *DBRepository) Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, f UpdateFields) (*PDPefindo, error) {
	setClauses := []string{}
	args := []interface{}{}
	idx := 1

	if f.PD12Month != nil {
		setClauses = append(setClauses, fmt.Sprintf("pd_12month = $%d", idx))
		args = append(args, f.PD12Month.String())
		idx++
	}
	if f.PDLifetime3Y != nil {
		setClauses = append(setClauses, fmt.Sprintf("pd_lifetime_3y = $%d", idx))
		args = append(args, f.PDLifetime3Y.String())
		idx++
	}
	if f.PDLifetime5Y != nil {
		setClauses = append(setClauses, fmt.Sprintf("pd_lifetime_5y = $%d", idx))
		args = append(args, f.PDLifetime5Y.String())
		idx++
	}
	if f.PDLifetime7Y != nil {
		setClauses = append(setClauses, fmt.Sprintf("pd_lifetime_7y = $%d", idx))
		args = append(args, f.PDLifetime7Y.String())
		idx++
	}
	if f.PDLifetime10Y != nil {
		setClauses = append(setClauses, fmt.Sprintf("pd_lifetime_10y = $%d", idx))
		args = append(args, f.PDLifetime10Y.String())
		idx++
	}
	if f.Sumber != nil {
		setClauses = append(setClauses, fmt.Sprintf("sumber = $%d", idx))
		args = append(args, *f.Sumber)
		idx++
	}
	if f.TanggalPublikasi != nil {
		setClauses = append(setClauses, fmt.Sprintf("tanggal_publikasi = $%d", idx))
		args = append(args, *f.TanggalPublikasi)
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
	if f.DokumenPendukungID != nil {
		setClauses = append(setClauses, fmt.Sprintf("dokumen_pendukung_id = $%d", idx))
		if *f.DokumenPendukungID == "" {
			args = append(args, nil)
		} else if docID, perr := uuid.Parse(*f.DokumenPendukungID); perr == nil {
			args = append(args, docID)
		} else {
			args = append(args, nil)
		}
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
		`UPDATE mst.pd_pefindo SET %s WHERE id = $%d AND row_version = $%d AND deleted_at IS NULL`,
		strings.Join(setClauses, ", "),
		whereIDIdx, whereVersionIdx,
	)

	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.Update pd_pefindo: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("repo.Update pd_pefindo rows affected: %w", err)
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

	// Return refreshed row inside same tx.
	q := baseSelect + " WHERE id = $1 AND deleted_at IS NULL"
	row := tx.QueryRowContext(ctx, q, id)
	return scanPDPefindo(row)
}

// SoftDelete marks the record as deleted.
func (r *DBRepository) SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*PDPefindo, error) {
	now := time.Now()
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.pd_pefindo
		SET deleted_at = $1, deleted_by = $2, updated_at = $1, updated_by = $2
		WHERE id = $3 AND deleted_at IS NULL
	`, now, deletedBy, id)
	if err != nil {
		return nil, fmt.Errorf("repo.SoftDelete pd_pefindo: %w", err)
	}
	q := baseSelect + " WHERE id = $1"
	row := tx.QueryRowContext(ctx, q, id)
	return scanPDPefindo(row)
}

// UpdateWorkflowStatus updates workflow_status column.
func (r *DBRepository) UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.pd_pefindo
		SET workflow_status = $1, updated_at = now(), updated_by = $2, row_version = row_version + 1
		WHERE id = $3
	`, string(status), updatedBy, id)
	if err != nil {
		return fmt.Errorf("repo.UpdateWorkflowStatus pd_pefindo: %w", err)
	}
	return nil
}

// CountOverlap counts pd_pefindo rows for the same rating whose period overlaps [dari, sampai].
// "Overlap" definition: NOT (sampai < dari_new OR dari > sampai_new).
// When sampai_new is nil (open-ended), it overlaps any row where dari >= dari_new.
func (r *DBRepository) CountOverlap(ctx context.Context, rating string, dari string, sampai *string, excludeID *uuid.UUID) (int64, error) {
	args := []interface{}{rating, dari}
	idx := 3

	// Build overlap condition.
	// Existing row overlaps new [dari, sampai] if:
	//   NOT (existing.sampai < dari_new  OR existing.dari > sampai_new)
	// When sampai_new is nil (open-ended): everything >= dari_new overlaps.
	var overlapCond string
	if sampai == nil {
		// New record is open-ended: overlaps any row whose dari >= dari_new
		// OR whose sampai IS NULL (both open-ended is always overlap)
		overlapCond = `(periode_berlaku_sampai IS NULL OR periode_berlaku_sampai >= $2)`
	} else {
		args = append(args, *sampai)
		overlapCond = fmt.Sprintf(`(
            (periode_berlaku_sampai IS NULL OR periode_berlaku_sampai >= $2) AND
            periode_berlaku_dari <= $%d
        )`, idx)
		idx++
	}

	excludeCond := ""
	if excludeID != nil {
		args = append(args, *excludeID)
		excludeCond = fmt.Sprintf(" AND id != $%d", idx)
	}

	query := fmt.Sprintf( //nolint:gosec
		`SELECT COUNT(*) FROM mst.pd_pefindo
        WHERE rating = $1 AND deleted_at IS NULL AND %s%s`,
		overlapCond, excludeCond,
	)

	var count int64
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("repo.CountOverlap pd_pefindo: %w", err)
	}
	return count, nil
}

// CountReferences counts active FK references to pd_pefindo.id.
// Currently pd_pefindo rows are not directly FK-referenced in DB (joins by rating+period
// at query time), so we return 0 for now. Reserved for future FK enforcement.
func (r *DBRepository) CountReferences(ctx context.Context, id uuid.UUID) (int64, error) {
	// Reserved: when ecl_calc_result_line gets a pd_pefindo_id FK column, add check here.
	return 0, nil
}

// BeginTx starts a transaction.
func (r *DBRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("repo.BeginTx pd_pefindo: %w", err)
	}
	return tx, nil
}

// ListAuditHistory returns paginated audit events for the given pd_pefindo entity UUID.
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

	args := []interface{}{entityID, "mst.pd_pefindo"}
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
		return nil, false, fmt.Errorf("repo.ListAuditHistory pd_pefindo: %w", err)
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

// ExportAll streams all non-deleted records as UTF-8 BOM CSV.
func (r *DBRepository) ExportAll(ctx context.Context, q listquery.Query) (io.Reader, int, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("t")

	conditions := []string{"t.deleted_at IS NULL"}
	if q.Search != "" {
		argIdx := len(args) + 1
		searchConds := make([]string, 0, len(SearchCols))
		for _, col := range SearchCols {
			searchConds = append(searchConds, fmt.Sprintf("t.%s ILIKE $%d", col, argIdx))
		}
		conditions = append(conditions, "("+strings.Join(searchConds, " OR ")+")")
		args = append(args, "%"+q.Search+"%")
	}
	if where != "" {
		conditions = append(conditions, where)
	}

	whereClause := " WHERE " + strings.Join(conditions, " AND ")
	if orderBy == "" {
		orderBy = "t.rating ASC, t.periode_berlaku_dari DESC"
	}

	query := fmt.Sprintf( //nolint:gosec
		`SELECT t.id, t.rating,
            t.pd_12month, t.pd_lifetime_3y, t.pd_lifetime_5y, t.pd_lifetime_7y, t.pd_lifetime_10y,
            t.sumber, t.tanggal_publikasi, t.periode_berlaku_dari, t.periode_berlaku_sampai,
            t.workflow_status
        FROM mst.pd_pefindo t%s ORDER BY %s`,
		whereClause, orderBy,
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll pd_pefindo: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM for Excel
	w := csv.NewWriter(&buf)

	headers := []string{
		"ID", "Rating",
		"PD 12 Bulan", "PD Seumur Hidup 3Y", "PD Seumur Hidup 5Y", "PD Seumur Hidup 7Y", "PD Seumur Hidup 10Y",
		"Sumber", "Tanggal Publikasi", "Periode Berlaku Dari", "Periode Berlaku Sampai",
		"Workflow Status",
	}
	if err := w.Write(headers); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll write header: %w", err)
	}

	count := 0
	for rows.Next() {
		var (
			id         string
			rating     string
			pd12       string
			pd3y       *string
			pd5y       *string
			pd7y       *string
			pd10y      *string
			sumber     string
			tanggalPub *string
			periodeD   string
			periodeS   *string
			wfStatus   string
		)
		if err := rows.Scan(&id, &rating, &pd12, &pd3y, &pd5y, &pd7y, &pd10y,
			&sumber, &tanggalPub, &periodeD, &periodeS, &wfStatus); err != nil {
			return nil, 0, fmt.Errorf("repo.ExportAll scan: %w", err)
		}

		record := []string{
			id, rating,
			pd12, nvl(pd3y), nvl(pd5y), nvl(pd7y), nvl(pd10y),
			sumber, nvl(tanggalPub), periodeD, nvl(periodeS),
			wfStatus,
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

// BulkCreate inserts multiple pd_pefindo rows in a single transaction.
func (r *DBRepository) BulkCreate(ctx context.Context, tx *sql.Tx, rows []*PDPefindo) (int, error) {
	count := 0
	for _, p := range rows {
		if err := r.Create(ctx, tx, p); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// GetJobByID fetches a sys.job row.
func (r *DBRepository) GetJobByID(ctx context.Context, jobID string) (*JobRow, error) {
	var j JobRow
	var payload, result, errJSON []byte
	var currentStep *string
	var startedAt, completedAt *time.Time

	err := r.db.QueryRowContext(ctx, `
		SELECT id, type, status, progress, current_step, payload_jsonb, result_jsonb, error_jsonb,
		       started_at, completed_at, can_cancel, created_by, created_at, tenant_id
		FROM sys.job WHERE id = $1
	`, jobID).Scan(
		&j.ID, &j.Type, &j.Status, &j.Progress, &currentStep,
		&payload, &result, &errJSON,
		&startedAt, &completedAt, &j.CanCancel,
		&j.CreatedBy, &j.CreatedAt, &j.TenantID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.GetJobByID: %w", err)
	}
	j.CurrentStep = currentStep
	j.PayloadJSON = payload
	j.ResultJSON = result
	j.ErrorJSON = errJSON
	j.StartedAt = startedAt
	j.CompletedAt = completedAt
	return &j, nil
}

// CreateJob inserts a new sys.job row.
func (r *DBRepository) CreateJob(ctx context.Context, tx *sql.Tx, j *JobRow) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO sys.job (
		    id, type, status, progress, current_step, payload_jsonb,
		    can_cancel, created_by, updated_by, created_at, tenant_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8,$9,$10)
	`,
		j.ID, j.Type, j.Status, j.Progress, j.CurrentStep, j.PayloadJSON,
		j.CanCancel, j.CreatedBy, j.CreatedAt, j.TenantID,
	)
	if err != nil {
		return fmt.Errorf("repo.CreateJob: %w", err)
	}
	return nil
}

// UpdateJobProgress updates progress + current_step in sys.job.
func (r *DBRepository) UpdateJobProgress(ctx context.Context, jobID string, progress int, currentStep string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE sys.job SET progress = $1, current_step = $2, status = 'running', started_at = COALESCE(started_at, now()), updated_at = now()
		WHERE id = $3
	`, progress, currentStep, jobID)
	if err != nil {
		return fmt.Errorf("repo.UpdateJobProgress: %w", err)
	}
	return nil
}

// CompleteJob marks job as completed.
func (r *DBRepository) CompleteJob(ctx context.Context, jobID string, resultJSON []byte) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE sys.job SET status = 'completed', progress = 100, result_jsonb = $1, completed_at = now(), updated_at = now()
		WHERE id = $2
	`, resultJSON, jobID)
	if err != nil {
		return fmt.Errorf("repo.CompleteJob: %w", err)
	}
	return nil
}

// FailJob marks job as failed.
func (r *DBRepository) FailJob(ctx context.Context, jobID string, errJSON []byte) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE sys.job SET status = 'failed', error_jsonb = $1, completed_at = now(), updated_at = now()
		WHERE id = $2
	`, errJSON, jobID)
	if err != nil {
		return fmt.Errorf("repo.FailJob: %w", err)
	}
	return nil
}

// ─── Scan helpers ─────────────────────────────────────────────────────────────

func scanPDPefindo(row *sql.Row) (*PDPefindo, error) {
	return scanOnePD(func(dest ...interface{}) error {
		return row.Scan(dest...)
	})
}

func scanPDPefindoRow(rows *sql.Rows) (*PDPefindo, error) {
	return scanOnePD(func(dest ...interface{}) error {
		return rows.Scan(dest...)
	})
}

func scanOnePD(scanFn func(...interface{}) error) (*PDPefindo, error) {
	p := &PDPefindo{}
	var (
		pd12       string
		pd3y       *string
		pd5y       *string
		pd7y       *string
		pd10y      *string
		tanggalPub *string
		periodeD   string
		periodeS   *string
		docID      *uuid.UUID
		uploadedBy uuid.UUID
		uploadedAt time.Time
		approvedBy *uuid.UUID
		approvedAt *time.Time
		wfStatus   string
		createdBy  *uuid.UUID
		updatedAt  *time.Time
		updatedBy  *uuid.UUID
		deletedAt  *time.Time
		deletedBy  *uuid.UUID
	)

	err := scanFn(
		&p.ID, &p.Rating,
		&pd12, &pd3y, &pd5y, &pd7y, &pd10y,
		&p.Sumber, &tanggalPub, &periodeD, &periodeS,
		&docID,
		&uploadedBy, &uploadedAt, &approvedBy, &approvedAt,
		&wfStatus,
		&p.CreatedAt, &createdBy, &updatedAt, &updatedBy,
		&deletedAt, &deletedBy, &p.RowVersion, &p.TenantID,
	)
	if err != nil {
		return nil, err
	}

	p.PD12Month = mustDecimal(pd12)
	p.PDLifetime3Y = maybeDecimal(pd3y)
	p.PDLifetime5Y = maybeDecimal(pd5y)
	p.PDLifetime7Y = maybeDecimal(pd7y)
	p.PDLifetime10Y = maybeDecimal(pd10y)
	p.TanggalPublikasi = tanggalPub
	p.PeriodeBerlakuDari = periodeD
	p.PeriodeBerlakuSampai = periodeS
	p.DokumenPendukungID = docID
	p.UploadedBy = uploadedBy
	p.UploadedAt = uploadedAt
	p.ApprovedBy = approvedBy
	p.ApprovedAt = approvedAt
	p.WorkflowStatus = WorkflowStatus(wfStatus)
	p.CreatedBy = createdBy
	p.UpdatedAt = updatedAt
	p.UpdatedBy = updatedBy
	p.DeletedAt = deletedAt
	p.DeletedBy = deletedBy
	return p, nil
}

// ─── Decimal helpers ──────────────────────────────────────────────────────────

func mustDecimal(s string) decimal.Decimal {
	return decimal.RequireFromString(s)
}

func maybeDecimal(s *string) *decimal.Decimal {
	if s == nil || *s == "" {
		return nil
	}
	d, err := decimal.NewFromString(*s)
	if err != nil {
		return nil
	}
	return &d
}

func pdDecimalOrNil(d *decimal.Decimal) interface{} {
	if d == nil {
		return nil
	}
	return d.String()
}

func nvl(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
