package kurs

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

// Repository defines the data-access contract for mst.kurs.
type Repository interface {
	// Create inserts a new kurs row in the given transaction.
	Create(ctx context.Context, tx *sql.Tx, k *Kurs) error

	// GetByID fetches one record by surrogate UUID.
	// Returns (nil, nil) if not found.
	GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*Kurs, error)

	// GetByKodeAndDate fetches one record by (kode_mata_uang, tanggal_berlaku).
	GetByKodeAndDate(ctx context.Context, kode string, tanggal time.Time) (*Kurs, error)

	// FindActivePeriode returns the periode_buku UUID for the given date, or uuid.Nil if not found.
	FindActivePeriode(ctx context.Context, tanggal time.Time) (uuid.UUID, error)

	// FindMataUangApproved checks whether the kode_mata_uang exists and is APPROVED.
	// Returns (false, nil) if not found; (true, nil) if APPROVED; (false, nil) if not APPROVED.
	FindMataUangApproved(ctx context.Context, kode string) (bool, error)

	// List fetches paginated records matching listquery + cursor.
	List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*Kurs, error)

	// Update applies partial update in the given transaction.
	// Returns ErrNotFound if id not found; ErrConflict if row_version mismatch.
	Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, fields UpdateFields) (*Kurs, error)

	// SoftDelete sets deleted_at/deleted_by in the given transaction.
	SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*Kurs, error)

	// UpdateWorkflowStatus updates workflow_status column after workflow transitions.
	UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error

	// BeginTx starts a database transaction with default isolation.
	BeginTx(ctx context.Context) (*sql.Tx, error)

	// ListAuditHistory returns paginated audit log rows for a given entity UUID.
	ListAuditHistory(ctx context.Context, entityID uuid.UUID, cursor string, limit int, isAuditRole bool) ([]AuditHistoryItem, bool, error)

	// ExportAll returns all records matching the query as a CSV io.Reader.
	ExportAll(ctx context.Context, q listquery.Query) (io.Reader, int, error)
}

// UpdateFields captures the mutable columns for an update operation.
type UpdateFields struct {
	KursBeli        *decimal.Decimal
	KursJual        *decimal.Decimal
	KursTengah      *decimal.Decimal
	SumberKurs      *SumberKurs
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

// baseSelectCols lists columns in scan order for kurs queries.
const baseSelectCols = `
    k.id, k.fx_rate_id_kode, k.kode_mata_uang, k.tanggal_berlaku,
    k.kurs_beli, k.kurs_jual, k.kurs_tengah, k.sumber_kurs,
    k.periode_bulanan_id, k.locked_flag,
    k.maker_id, k.approver_id, k.approved_at,
    k.workflow_status,
    k.created_at, k.created_by, k.updated_at, k.updated_by,
    k.deleted_at, k.deleted_by, k.row_version, k.tenant_id`

// Create inserts a new row. Called inside a transaction.
func (r *DBRepository) Create(ctx context.Context, tx *sql.Tx, k *Kurs) error {
	query := `
INSERT INTO mst.kurs (
    id, fx_rate_id_kode, kode_mata_uang, tanggal_berlaku,
    kurs_beli, kurs_jual, kurs_tengah, sumber_kurs,
    periode_bulanan_id, locked_flag,
    maker_id, approver_id, approved_at,
    workflow_status,
    created_at, created_by, updated_at, updated_by,
    row_version, tenant_id
) VALUES (
    $1,  $2,  $3,  $4,
    $5,  $6,  $7,  $8,
    $9,  $10,
    $11, $12, $13,
    $14,
    $15, $16, $15, $16,
    1, $17
)`
	var kursBeli, kursJual interface{}
	if k.KursBeli != nil {
		kursBeli = k.KursBeli.StringFixed(4)
	}
	if k.KursJual != nil {
		kursJual = k.KursJual.StringFixed(4)
	}

	_, err := tx.ExecContext(ctx, query,
		k.ID, k.FxRateIDKode, k.KodeMataUang, k.TanggalBerlaku,
		kursBeli, kursJual, k.KursTengah.StringFixed(4), string(k.SumberKurs),
		k.PeriodeBulananID, k.LockedFlag,
		k.MakerID, k.ApproverID, k.ApprovedAt,
		string(k.WorkflowStatus),
		k.CreatedAt, k.CreatedBy,
		k.TenantID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicateDate
		}
		if isLockedTriggerError(err) {
			return ErrLocked
		}
		return fmt.Errorf("repo.Create kurs: %w", err)
	}
	return nil
}

// GetByID fetches by surrogate UUID.
func (r *DBRepository) GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*Kurs, error) {
	deletedFilter := " AND k.deleted_at IS NULL"
	if includeDeleted {
		deletedFilter = ""
	}
	query := fmt.Sprintf("SELECT%s FROM mst.kurs k WHERE k.id = $1%s", baseSelectCols, deletedFilter) //nolint:gosec
	row := r.db.QueryRowContext(ctx, query, id)
	k, err := scanKurs(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.GetByID kurs: %w", err)
	}
	return k, nil
}

// GetByKodeAndDate fetches by (kode_mata_uang, tanggal_berlaku).
func (r *DBRepository) GetByKodeAndDate(ctx context.Context, kode string, tanggal time.Time) (*Kurs, error) {
	query := fmt.Sprintf("SELECT%s FROM mst.kurs k WHERE k.kode_mata_uang = $1 AND k.tanggal_berlaku = $2 AND k.deleted_at IS NULL", baseSelectCols)
	row := r.db.QueryRowContext(ctx, query, kode, tanggal)
	k, err := scanKurs(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.GetByKodeAndDate kurs: %w", err)
	}
	return k, nil
}

// FindActivePeriode returns the periode_buku UUID whose range contains the given date.
// Schema (0001): mst.periode_buku.tanggal_mulai + tanggal_akhir.
func (r *DBRepository) FindActivePeriode(ctx context.Context, tanggal time.Time) (uuid.UUID, error) {
	var periodeID uuid.UUID
	err := r.db.QueryRowContext(ctx, `
		SELECT id FROM mst.periode_buku
		WHERE tanggal_mulai <= $1
		  AND tanggal_akhir >= $1
		  AND deleted_at IS NULL
		ORDER BY tanggal_mulai DESC
		LIMIT 1
	`, tanggal).Scan(&periodeID)
	if err == sql.ErrNoRows {
		return uuid.Nil, nil
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("repo.FindActivePeriode kurs: %w", err)
	}
	return periodeID, nil
}

// FindMataUangApproved checks whether the kode_mata_uang exists and is APPROVED.
func (r *DBRepository) FindMataUangApproved(ctx context.Context, kode string) (bool, error) {
	var wfStatus string
	err := r.db.QueryRowContext(ctx, `
		SELECT workflow_status FROM mst.mata_uang
		WHERE kode_mata_uang = $1 AND deleted_at IS NULL
	`, kode).Scan(&wfStatus)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("repo.FindMataUangApproved kurs: %w", err)
	}
	return wfStatus == "APPROVED", nil
}

// List fetches paginated records.
func (r *DBRepository) List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*Kurs, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("k")

	conditions := []string{}
	if !includeDeleted {
		conditions = append(conditions, "k.deleted_at IS NULL")
	}

	if cursor != "" {
		cd, err := pagination.DecodeCursor(cursor)
		if err == nil && cd.ID != "" {
			argIdx := len(args) + 1
			// Cursor on (tanggal_berlaku DESC, id ASC) — use id for stable pagination
			conditions = append(conditions, fmt.Sprintf("k.id > $%d", argIdx))
			args = append(args, cd.ID)
		}
	}

	if q.Search != "" {
		argIdx := len(args) + 1
		searchCond := make([]string, 0, len(SearchCols))
		for _, col := range SearchCols {
			searchCond = append(searchCond, fmt.Sprintf("k.%s ILIKE $%d", col, argIdx))
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
		orderBy = "k.tanggal_berlaku DESC, k.kode_mata_uang ASC"
	}

	argIdx := len(args) + 1
	query := fmt.Sprintf( //nolint:gosec
		"SELECT%s FROM mst.kurs k%s ORDER BY %s LIMIT $%d",
		baseSelectCols, whereClause, orderBy, argIdx,
	)
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.List kurs: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []*Kurs
	for rows.Next() {
		k, err := scanKursRow(rows)
		if err != nil {
			return nil, fmt.Errorf("repo.List kurs scan: %w", err)
		}
		items = append(items, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo.List kurs rows.Err: %w", err)
	}
	return items, nil
}

// Update applies partial field changes with optimistic lock.
func (r *DBRepository) Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, f UpdateFields) (*Kurs, error) {
	setClauses := []string{}
	args := []interface{}{}
	idx := 1

	if f.KursBeli != nil {
		setClauses = append(setClauses, fmt.Sprintf("kurs_beli = $%d", idx))
		args = append(args, f.KursBeli.StringFixed(4))
		idx++
	}
	if f.KursJual != nil {
		setClauses = append(setClauses, fmt.Sprintf("kurs_jual = $%d", idx))
		args = append(args, f.KursJual.StringFixed(4))
		idx++
	}
	if f.KursTengah != nil {
		setClauses = append(setClauses, fmt.Sprintf("kurs_tengah = $%d", idx))
		args = append(args, f.KursTengah.StringFixed(4))
		idx++
	}
	if f.SumberKurs != nil {
		setClauses = append(setClauses, fmt.Sprintf("sumber_kurs = $%d", idx))
		args = append(args, string(*f.SumberKurs))
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
		`UPDATE mst.kurs SET %s WHERE id = $%d AND row_version = $%d AND deleted_at IS NULL`,
		strings.Join(setClauses, ", "),
		whereIDIdx, whereVersionIdx,
	)

	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		if isLockedTriggerError(err) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("repo.Update kurs: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("repo.Update kurs rows affected: %w", err)
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

	return r.getOneInTx(ctx, tx, id)
}

// UpdateWorkflowStatus updates only the workflow_status column.
func (r *DBRepository) UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.kurs
		SET workflow_status = $1, updated_at = now(), updated_by = $2, row_version = row_version + 1
		WHERE id = $3
	`, string(status), updatedBy, id)
	if err != nil {
		return fmt.Errorf("repo.UpdateWorkflowStatus kurs: %w", err)
	}
	return nil
}

// SoftDelete marks the record as deleted.
func (r *DBRepository) SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*Kurs, error) {
	now := time.Now()
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.kurs
		SET deleted_at = $1, deleted_by = $2, updated_at = $1, updated_by = $2
		WHERE id = $3 AND deleted_at IS NULL
	`, now, deletedBy, id)
	if err != nil {
		if isLockedTriggerError(err) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("repo.SoftDelete kurs: %w", err)
	}
	return r.getOneInTx(ctx, tx, id)
}

// BeginTx starts a transaction.
func (r *DBRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("repo.BeginTx kurs: %w", err)
	}
	return tx, nil
}

// ListAuditHistory returns paginated audit events for the given entity UUID.
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
	args = append(args, entityID, "mst.kurs")
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
		return nil, false, fmt.Errorf("repo.ListAuditHistory kurs: %w", err)
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
			return nil, false, fmt.Errorf("repo.ListAuditHistory kurs scan: %w", err)
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
		return nil, false, fmt.Errorf("repo.ListAuditHistory kurs rows.Err: %w", err)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	return items, hasMore, nil
}

// ExportAll streams all records matching q as UTF-8 BOM CSV.
func (r *DBRepository) ExportAll(ctx context.Context, q listquery.Query) (io.Reader, int, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("k")

	conditions := []string{"k.deleted_at IS NULL"}
	if q.Search != "" {
		argIdx := len(args) + 1
		searchCond := make([]string, 0, len(SearchCols))
		for _, col := range SearchCols {
			searchCond = append(searchCond, fmt.Sprintf("k.%s ILIKE $%d", col, argIdx))
		}
		conditions = append(conditions, "("+strings.Join(searchCond, " OR ")+")")
		args = append(args, "%"+q.Search+"%")
	}
	if where != "" {
		conditions = append(conditions, where)
	}

	whereClause := " WHERE " + strings.Join(conditions, " AND ")
	if orderBy == "" {
		orderBy = "k.tanggal_berlaku DESC, k.kode_mata_uang ASC"
	}

	query := fmt.Sprintf( //nolint:gosec
		`SELECT k.fx_rate_id_kode, k.kode_mata_uang, k.tanggal_berlaku,
		        k.kurs_beli, k.kurs_jual, k.kurs_tengah, k.sumber_kurs,
		        k.workflow_status, k.locked_flag
		 FROM mst.kurs k%s ORDER BY %s`,
		whereClause, orderBy,
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll kurs: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM for Excel
	w := csv.NewWriter(&buf)

	headers := []string{"ID Kode", "Kode Mata Uang", "Tanggal Berlaku",
		"Kurs Beli", "Kurs Jual", "Kurs Tengah", "Sumber Kurs", "Workflow Status", "Locked"}
	if err := w.Write(headers); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll kurs write header: %w", err)
	}

	count := 0
	for rows.Next() {
		var (
			fxRateIDKode   string
			kodeMataUang   string
			tanggalBerlaku time.Time
			kursBeli       *string
			kursJual       *string
			kursTengah     string
			sumberKurs     string
			wfStatus       string
			lockedFlag     bool
		)
		if err := rows.Scan(&fxRateIDKode, &kodeMataUang, &tanggalBerlaku,
			&kursBeli, &kursJual, &kursTengah, &sumberKurs, &wfStatus, &lockedFlag); err != nil {
			return nil, 0, fmt.Errorf("repo.ExportAll kurs scan: %w", err)
		}

		lockedStr := "Tidak"
		if lockedFlag {
			lockedStr = "Ya"
		}
		beliStr := ""
		if kursBeli != nil {
			beliStr = *kursBeli
		}
		jualStr := ""
		if kursJual != nil {
			jualStr = *kursJual
		}

		record := []string{
			fxRateIDKode, kodeMataUang, tanggalBerlaku.Format("2006-01-02"),
			beliStr, jualStr, kursTengah, sumberKurs, wfStatus, lockedStr,
		}
		if err := w.Write(record); err != nil {
			return nil, 0, fmt.Errorf("repo.ExportAll kurs write record: %w", err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll kurs rows.Err: %w", err)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll kurs flush: %w", err)
	}
	return &buf, count, nil
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

// getOneInTx fetches by id within an existing transaction.
func (r *DBRepository) getOneInTx(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*Kurs, error) {
	query := fmt.Sprintf("SELECT%s FROM mst.kurs k WHERE k.id = $1", baseSelectCols)
	row := tx.QueryRowContext(ctx, query, id)
	k, err := scanKurs(row)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repo.getOneInTx kurs: %w", err)
	}
	return k, nil
}

// scanKurs scans one *sql.Row into Kurs.
func scanKurs(row *sql.Row) (*Kurs, error) {
	k := &Kurs{}
	var (
		kursBeli   *string
		kursJual   *string
		kursTengah string
		sumberKurs string
		wfStatus   string
		createdBy  *uuid.UUID
		updatedAt  *time.Time
		updatedBy  *uuid.UUID
		deletedAt  *time.Time
		deletedBy  *uuid.UUID
		makerID    *uuid.UUID
		approverID *uuid.UUID
		approvedAt *time.Time
	)
	err := row.Scan(
		&k.ID, &k.FxRateIDKode, &k.KodeMataUang, &k.TanggalBerlaku,
		&kursBeli, &kursJual, &kursTengah, &sumberKurs,
		&k.PeriodeBulananID, &k.LockedFlag,
		&makerID, &approverID, &approvedAt,
		&wfStatus,
		&k.CreatedAt, &createdBy, &updatedAt, &updatedBy,
		&deletedAt, &deletedBy, &k.RowVersion, &k.TenantID,
	)
	if err != nil {
		return nil, err
	}
	applyKursScanned(k, kursBeli, kursJual, kursTengah, sumberKurs, wfStatus,
		createdBy, updatedAt, updatedBy, deletedAt, deletedBy,
		makerID, approverID, approvedAt)
	return k, nil
}

// scanKursRow scans one *sql.Rows row into Kurs.
func scanKursRow(rows *sql.Rows) (*Kurs, error) {
	k := &Kurs{}
	var (
		kursBeli   *string
		kursJual   *string
		kursTengah string
		sumberKurs string
		wfStatus   string
		createdBy  *uuid.UUID
		updatedAt  *time.Time
		updatedBy  *uuid.UUID
		deletedAt  *time.Time
		deletedBy  *uuid.UUID
		makerID    *uuid.UUID
		approverID *uuid.UUID
		approvedAt *time.Time
	)
	err := rows.Scan(
		&k.ID, &k.FxRateIDKode, &k.KodeMataUang, &k.TanggalBerlaku,
		&kursBeli, &kursJual, &kursTengah, &sumberKurs,
		&k.PeriodeBulananID, &k.LockedFlag,
		&makerID, &approverID, &approvedAt,
		&wfStatus,
		&k.CreatedAt, &createdBy, &updatedAt, &updatedBy,
		&deletedAt, &deletedBy, &k.RowVersion, &k.TenantID,
	)
	if err != nil {
		return nil, err
	}
	applyKursScanned(k, kursBeli, kursJual, kursTengah, sumberKurs, wfStatus,
		createdBy, updatedAt, updatedBy, deletedAt, deletedBy,
		makerID, approverID, approvedAt)
	return k, nil
}

// applyKursScanned converts scanned raw values into typed Kurs fields.
func applyKursScanned(
	k *Kurs,
	kursBeli, kursJual *string,
	kursTengah, sumberKurs, wfStatus string,
	createdBy *uuid.UUID,
	updatedAt *time.Time,
	updatedBy *uuid.UUID,
	deletedAt *time.Time,
	deletedBy *uuid.UUID,
	makerID, approverID *uuid.UUID,
	approvedAt *time.Time,
) {
	if kursBeli != nil {
		d, err := decimal.NewFromString(*kursBeli)
		if err == nil {
			k.KursBeli = &d
		}
	}
	if kursJual != nil {
		d, err := decimal.NewFromString(*kursJual)
		if err == nil {
			k.KursJual = &d
		}
	}
	d, err := decimal.NewFromString(kursTengah)
	if err == nil {
		k.KursTengah = d
	}
	k.SumberKurs = SumberKurs(sumberKurs)
	k.WorkflowStatus = WorkflowStatus(wfStatus)
	k.CreatedBy = createdBy
	k.UpdatedAt = updatedAt
	k.UpdatedBy = updatedBy
	k.DeletedAt = deletedAt
	k.DeletedBy = deletedBy
	k.MakerID = makerID
	k.ApproverID = approverID
	k.ApprovedAt = approvedAt
}

// ─── Sentinel errors ──────────────────────────────────────────────────────────

// ErrNotFound is returned when a record is not found.
var ErrNotFound = fmt.Errorf("kurs not found")

// ErrConflict is returned on row_version mismatch.
var ErrConflict = fmt.Errorf("optimistic lock conflict")

// ErrDuplicateDate is returned when (kode_mata_uang, tanggal_berlaku) already exists.
var ErrDuplicateDate = fmt.Errorf("kurs duplicate date")

// ErrLocked is returned when locked_flag = true.
var ErrLocked = fmt.Errorf("kurs is locked (periode CLOSED)")

// isUniqueViolation checks for PostgreSQL unique constraint violation.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "23505") ||
		strings.Contains(err.Error(), "duplicate key") ||
		strings.Contains(err.Error(), "unique constraint")
}

// isLockedTriggerError checks for the kurs lock trigger RAISE EXCEPTION.
func isLockedTriggerError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "locked because periode is CLOSED")
}
