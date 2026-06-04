package ratinghistory

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

// ─── Repository interface ─────────────────────────────────────────────────────

// Repository defines the data-access contract for rating_history_counterparty.
type Repository interface {
	// Create inserts a new rating_history row.
	Create(ctx context.Context, tx *sql.Tx, rh *RatingHistory) error

	// GetByID fetches one record by UUID.
	GetByID(ctx context.Context, id uuid.UUID) (*RatingHistory, error)

	// GetByKode fetches one record by business key.
	GetByKode(ctx context.Context, kode string) (*RatingHistory, error)

	// GetActiveByCounterparty fetches the current (tanggal_berakhir IS NULL) rating for a counterparty.
	// Returns (nil, nil) if none exists.
	GetActiveByCounterparty(ctx context.Context, counterpartyID uuid.UUID) (*RatingHistory, error)

	// List returns paginated records matching query.
	List(ctx context.Context, q listquery.Query, cursor string, limit int) ([]*RatingHistory, error)

	// ListByCounterparty returns paginated records for a specific counterparty.
	ListByCounterparty(ctx context.Context, counterpartyID uuid.UUID, cursor string, limit int) ([]*RatingHistory, error)

	// Update applies partial update with optimistic lock.
	Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, f UpdateFields) (*RatingHistory, error)

	// CloseActiveRating sets tanggal_berakhir on the current active rating.
	// tanggalBerakhir is the date to set (typically new rating's tanggal_berlaku minus 1 day).
	CloseActiveRating(ctx context.Context, tx *sql.Tx, counterpartyID uuid.UUID, tanggalBerakhir string, updatedBy uuid.UUID) error

	// SoftDelete sets deleted_at, deleted_by.
	SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*RatingHistory, error)

	// UpdateWorkflowStatus updates workflow_status column.
	UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error

	// SetSICRFlags updates sicr_triggered, default_triggered after approval.
	SetSICRFlags(ctx context.Context, tx *sql.Tx, id uuid.UUID, sicr, defaultFlag bool, updatedBy uuid.UUID) error

	// BeginTx starts a DB transaction.
	BeginTx(ctx context.Context) (*sql.Tx, error)

	// ListAuditHistory returns paginated audit log rows.
	ListAuditHistory(ctx context.Context, entityID uuid.UUID, cursor string, limit int, isAuditRole bool) ([]AuditHistoryItem, bool, error)

	// ExportAll returns records as CSV.
	ExportAll(ctx context.Context, q listquery.Query) (io.Reader, int, error)
}

// UpdateFields captures mutable columns for update.
type UpdateFields struct {
	RatingPefindo          *string
	RatingOutlook          *string
	SumberRating           *string
	TanggalPublikasiRating *string
	ActionType             *ActionType
	NotchChange            *int
	DokumenBuktiID         *uuid.UUID
	UpdatedBy              uuid.UUID
	ExpectedVersion        int64
}

// ─── Sentinel errors ──────────────────────────────────────────────────────────

var (
	ErrNotFound           = fmt.Errorf("rating_history not found")
	ErrConflict           = fmt.Errorf("optimistic lock conflict")
	ErrKodeDuplicate      = fmt.Errorf("rating_history kode duplicate")
	ErrMultipleActive     = fmt.Errorf("counterparty already has an active rating")
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

// querier abstracts *sql.DB and *sql.Tx.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

const baseSelect = `
SELECT
    r.id, r.rating_history_id_kode, r.counterparty_id,
    r.tanggal_berlaku, r.tanggal_berakhir,
    r.rating_pefindo, r.rating_outlook, r.sumber_rating,
    r.tanggal_publikasi_rating, r.action_type,
    r.notch_change, r.sicr_triggered, r.default_triggered,
    r.dokumen_bukti_id,
    r.maker_id, r.approver_id, r.approved_at,
    r.created_at, r.created_by, r.updated_at, r.updated_by,
    r.deleted_at, r.deleted_by, r.row_version, r.tenant_id,
    r.workflow_status
FROM mst.rating_history_counterparty r`

// Create inserts a new row.
func (r *DBRepository) Create(ctx context.Context, tx *sql.Tx, rh *RatingHistory) error {
	query := `
INSERT INTO mst.rating_history_counterparty (
    id, rating_history_id_kode, counterparty_id,
    tanggal_berlaku, tanggal_berakhir,
    rating_pefindo, rating_outlook, sumber_rating,
    tanggal_publikasi_rating, action_type,
    notch_change, sicr_triggered, default_triggered,
    dokumen_bukti_id,
    maker_id, approver_id, approved_at,
    created_at, created_by, updated_at, updated_by,
    row_version, tenant_id, workflow_status
) VALUES (
    $1, $2, $3,
    $4, $5,
    $6, $7, $8,
    $9, $10,
    $11, $12, $13,
    $14,
    $15, $16, $17,
    $18, $19, $18, $19,
    1, $20, $21
)`
	_, err := tx.ExecContext(ctx, query,
		rh.ID, rh.RatingHistoryIDKode, rh.CounterpartyID,
		rh.TanggalBerlaku, rh.TanggalBerakhir,
		rh.RatingPefindo, rh.RatingOutlook, rh.SumberRating,
		rh.TanggalPublikasiRating, string(rh.ActionType),
		rh.NotchChange, rh.SicrTriggered, rh.DefaultTriggered,
		rh.DokumenBuktiID,
		rh.MakerID, rh.ApproverID, rh.ApprovedAt,
		rh.CreatedAt, rh.CreatedBy,
		rh.TenantID, string(rh.WorkflowStatus),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("rating_history_id_kode: %w", ErrKodeDuplicate)
		}
		// Check for active rating constraint violation
		if strings.Contains(err.Error(), "uq_rating_aktif") || strings.Contains(err.Error(), "already active") {
			return ErrMultipleActive
		}
		return fmt.Errorf("repo.Create rating_history: %w", err)
	}
	return nil
}

// GetByID fetches by UUID.
func (r *DBRepository) GetByID(ctx context.Context, id uuid.UUID) (*RatingHistory, error) {
	return r.getOne(ctx, r.db, "r.id = $1 AND r.deleted_at IS NULL", id)
}

// GetByKode fetches by business key.
func (r *DBRepository) GetByKode(ctx context.Context, kode string) (*RatingHistory, error) {
	return r.getOne(ctx, r.db, "r.rating_history_id_kode = $1 AND r.deleted_at IS NULL", kode)
}

// GetActiveByCounterparty fetches the active (tanggal_berakhir IS NULL) rating.
func (r *DBRepository) GetActiveByCounterparty(ctx context.Context, counterpartyID uuid.UUID) (*RatingHistory, error) {
	return r.getOne(ctx, r.db,
		"r.counterparty_id = $1 AND r.tanggal_berakhir IS NULL AND r.deleted_at IS NULL AND r.workflow_status = 'APPROVED'",
		counterpartyID)
}

func (r *DBRepository) getOne(ctx context.Context, q querier, where string, arg interface{}) (*RatingHistory, error) {
	query := baseSelect + " WHERE " + where
	row := q.QueryRowContext(ctx, query, arg)
	rh, err := scanRatingHistory(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.getOne rating_history: %w", err)
	}
	return rh, nil
}

// List returns paginated records.
func (r *DBRepository) List(ctx context.Context, q listquery.Query, cursor string, limit int) ([]*RatingHistory, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("r")

	conditions := []string{"r.deleted_at IS NULL"}

	if cursor != "" {
		cd, err := pagination.DecodeCursor(cursor)
		if err == nil && cd.ID != "" {
			argIdx := len(args) + 1
			conditions = append(conditions, fmt.Sprintf("r.rating_history_id_kode > $%d", argIdx))
			args = append(args, cd.ID)
		}
	}

	if q.Search != "" {
		argIdx := len(args) + 1
		searchCond := make([]string, 0, len(SearchCols))
		for _, col := range SearchCols {
			searchCond = append(searchCond, fmt.Sprintf("r.%s ILIKE $%d", col, argIdx))
		}
		conditions = append(conditions, "("+strings.Join(searchCond, " OR ")+")")
		args = append(args, "%"+q.Search+"%")
	}

	if where != "" {
		conditions = append(conditions, where)
	}

	whereClause := " WHERE " + strings.Join(conditions, " AND ")
	if orderBy == "" {
		orderBy = "r.tanggal_berlaku DESC"
	}

	argIdx := len(args) + 1
	query := fmt.Sprintf( //nolint:gosec
		"SELECT r.id, r.rating_history_id_kode, r.counterparty_id, r.tanggal_berlaku, r.tanggal_berakhir, r.rating_pefindo, r.rating_outlook, r.sumber_rating, r.tanggal_publikasi_rating, r.action_type, r.notch_change, r.sicr_triggered, r.default_triggered, r.dokumen_bukti_id, r.maker_id, r.approver_id, r.approved_at, r.created_at, r.created_by, r.updated_at, r.updated_by, r.deleted_at, r.deleted_by, r.row_version, r.tenant_id, r.workflow_status FROM mst.rating_history_counterparty r%s ORDER BY %s LIMIT $%d",
		whereClause, orderBy, argIdx,
	)
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.List rating_history: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []*RatingHistory
	for rows.Next() {
		rh, err := scanRatingHistoryRow(rows)
		if err != nil {
			return nil, fmt.Errorf("repo.List scan: %w", err)
		}
		items = append(items, rh)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo.List rows.Err: %w", err)
	}
	return items, nil
}

// ListByCounterparty returns paginated records for one counterparty.
func (r *DBRepository) ListByCounterparty(ctx context.Context, counterpartyID uuid.UUID, cursor string, limit int) ([]*RatingHistory, error) {
	conditions := []string{"r.counterparty_id = $1", "r.deleted_at IS NULL"}
	args := []interface{}{counterpartyID}

	if cursor != "" {
		cd, err := pagination.DecodeCursor(cursor)
		if err == nil && cd.ID != "" {
			args = append(args, cd.ID)
			conditions = append(conditions, fmt.Sprintf("r.rating_history_id_kode > $%d", len(args)))
		}
	}

	whereClause := " WHERE " + strings.Join(conditions, " AND ")
	args = append(args, limit+1)
	argIdx := len(args)

	query := fmt.Sprintf( //nolint:gosec
		"SELECT r.id, r.rating_history_id_kode, r.counterparty_id, r.tanggal_berlaku, r.tanggal_berakhir, r.rating_pefindo, r.rating_outlook, r.sumber_rating, r.tanggal_publikasi_rating, r.action_type, r.notch_change, r.sicr_triggered, r.default_triggered, r.dokumen_bukti_id, r.maker_id, r.approver_id, r.approved_at, r.created_at, r.created_by, r.updated_at, r.updated_by, r.deleted_at, r.deleted_by, r.row_version, r.tenant_id, r.workflow_status FROM mst.rating_history_counterparty r%s ORDER BY r.tanggal_berlaku DESC LIMIT $%d",
		whereClause, argIdx,
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.ListByCounterparty: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []*RatingHistory
	for rows.Next() {
		rh, err := scanRatingHistoryRow(rows)
		if err != nil {
			return nil, fmt.Errorf("repo.ListByCounterparty scan: %w", err)
		}
		items = append(items, rh)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo.ListByCounterparty rows.Err: %w", err)
	}
	return items, nil
}

// Update applies partial update with optimistic lock.
func (r *DBRepository) Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, f UpdateFields) (*RatingHistory, error) {
	setClauses := []string{}
	args := []interface{}{}
	idx := 1

	if f.RatingPefindo != nil {
		setClauses = append(setClauses, fmt.Sprintf("rating_pefindo = $%d", idx))
		args = append(args, *f.RatingPefindo)
		idx++
	}
	if f.RatingOutlook != nil {
		setClauses = append(setClauses, fmt.Sprintf("rating_outlook = $%d", idx))
		args = append(args, *f.RatingOutlook)
		idx++
	}
	if f.SumberRating != nil {
		setClauses = append(setClauses, fmt.Sprintf("sumber_rating = $%d", idx))
		args = append(args, *f.SumberRating)
		idx++
	}
	if f.TanggalPublikasiRating != nil {
		setClauses = append(setClauses, fmt.Sprintf("tanggal_publikasi_rating = $%d", idx))
		args = append(args, *f.TanggalPublikasiRating)
		idx++
	}
	if f.ActionType != nil {
		setClauses = append(setClauses, fmt.Sprintf("action_type = $%d", idx))
		args = append(args, string(*f.ActionType))
		idx++
	}
	if f.NotchChange != nil {
		setClauses = append(setClauses, fmt.Sprintf("notch_change = $%d", idx))
		args = append(args, *f.NotchChange)
		idx++
	}
	if f.DokumenBuktiID != nil {
		setClauses = append(setClauses, fmt.Sprintf("dokumen_bukti_id = $%d", idx))
		args = append(args, *f.DokumenBuktiID)
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
		`UPDATE mst.rating_history_counterparty SET %s WHERE id = $%d AND row_version = $%d AND deleted_at IS NULL`,
		strings.Join(setClauses, ", "),
		whereIDIdx, whereVersionIdx,
	)

	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.Update rating_history: %w", err)
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
	return r.getOne(ctx, tx, "r.id = $1", id)
}

// CloseActiveRating sets tanggal_berakhir on the active rating for a counterparty.
func (r *DBRepository) CloseActiveRating(ctx context.Context, tx *sql.Tx, counterpartyID uuid.UUID, tanggalBerakhir string, updatedBy uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.rating_history_counterparty
		SET tanggal_berakhir = $1,
		    updated_at = now(), updated_by = $2,
		    row_version = row_version + 1
		WHERE counterparty_id = $3
		  AND tanggal_berakhir IS NULL
		  AND deleted_at IS NULL
		  AND workflow_status = 'APPROVED'
	`, tanggalBerakhir, updatedBy, counterpartyID)
	if err != nil {
		return fmt.Errorf("repo.CloseActiveRating: %w", err)
	}
	return nil
}

// SoftDelete marks the record as deleted.
func (r *DBRepository) SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*RatingHistory, error) {
	now := time.Now()
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.rating_history_counterparty
		SET deleted_at = $1, deleted_by = $2,
		    updated_at = $1, updated_by = $2,
		    row_version = row_version + 1
		WHERE id = $3 AND deleted_at IS NULL
	`, now, deletedBy, id)
	if err != nil {
		return nil, fmt.Errorf("repo.SoftDelete rating_history: %w", err)
	}
	return r.getOne(ctx, tx, "r.id = $1", id)
}

// UpdateWorkflowStatus updates only the workflow_status column.
func (r *DBRepository) UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.rating_history_counterparty
		SET workflow_status = $1, updated_at = now(), updated_by = $2,
		    row_version = row_version + 1
		WHERE id = $3
	`, string(status), updatedBy, id)
	if err != nil {
		return fmt.Errorf("repo.UpdateWorkflowStatus rating_history: %w", err)
	}
	return nil
}

// SetSICRFlags updates sicr_triggered and default_triggered flags.
func (r *DBRepository) SetSICRFlags(ctx context.Context, tx *sql.Tx, id uuid.UUID, sicr, defaultFlag bool, updatedBy uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.rating_history_counterparty
		SET sicr_triggered = $1, default_triggered = $2,
		    updated_at = now(), updated_by = $3,
		    row_version = row_version + 1
		WHERE id = $4
	`, sicr, defaultFlag, updatedBy, id)
	if err != nil {
		return fmt.Errorf("repo.SetSICRFlags: %w", err)
	}
	return nil
}

// BeginTx starts a transaction.
func (r *DBRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("repo.BeginTx rating_history: %w", err)
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
	args = append(args, entityID, "mst.rating_history_counterparty")
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
		return nil, false, fmt.Errorf("repo.ListAuditHistory rating_history: %w", err)
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

// ExportAll returns all records as UTF-8 BOM CSV.
func (r *DBRepository) ExportAll(ctx context.Context, q listquery.Query) (io.Reader, int, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("r")

	conditions := []string{"r.deleted_at IS NULL"}
	if q.Search != "" {
		argIdx := len(args) + 1
		searchCond := make([]string, 0, len(SearchCols))
		for _, col := range SearchCols {
			searchCond = append(searchCond, fmt.Sprintf("r.%s ILIKE $%d", col, argIdx))
		}
		conditions = append(conditions, "("+strings.Join(searchCond, " OR ")+")")
		args = append(args, "%"+q.Search+"%")
	}
	if where != "" {
		conditions = append(conditions, where)
	}

	whereClause := " WHERE " + strings.Join(conditions, " AND ")
	if orderBy == "" {
		orderBy = "r.tanggal_berlaku DESC"
	}

	query := fmt.Sprintf( //nolint:gosec
		"SELECT r.rating_history_id_kode, r.counterparty_id, r.tanggal_berlaku, r.tanggal_berakhir, r.rating_pefindo, r.action_type, r.notch_change, r.sicr_triggered, r.default_triggered, r.workflow_status FROM mst.rating_history_counterparty r%s ORDER BY %s",
		whereClause, orderBy,
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll rating_history: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	w := csv.NewWriter(&buf)

	headers := []string{"Kode", "Counterparty ID", "Tgl Berlaku", "Tgl Berakhir", "Rating", "Action", "Notch Change", "SICR", "Default", "Workflow Status"}
	if err := w.Write(headers); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll write header: %w", err)
	}

	count := 0
	for rows.Next() {
		var (
			kode            string
			cpID            uuid.UUID
			tanggalBerlaku  string
			tanggalBerakhir *string
			ratingPefindo   string
			actionType      string
			notchChange     int
			sicrTriggered   bool
			defaultTriggered bool
			wfStatus        string
		)
		if err := rows.Scan(&kode, &cpID, &tanggalBerlaku, &tanggalBerakhir,
			&ratingPefindo, &actionType, &notchChange, &sicrTriggered, &defaultTriggered, &wfStatus); err != nil {
			return nil, 0, fmt.Errorf("repo.ExportAll scan: %w", err)
		}
		berakhir := ""
		if tanggalBerakhir != nil {
			berakhir = *tanggalBerakhir
		}
		record := []string{
			kode, cpID.String(), tanggalBerlaku, berakhir,
			ratingPefindo, actionType, fmt.Sprintf("%d", notchChange),
			boolStr(sicrTriggered), boolStr(defaultTriggered), wfStatus,
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

func scanRatingHistory(row *sql.Row) (*RatingHistory, error) {
	rh := &RatingHistory{}
	var (
		actionTypeStr  string
		wfStatusStr    string
		tanggalBerakhir *string
		ratingOutlook  *string
		dokumenBuktiID *uuid.UUID
		approverID     *uuid.UUID
		approvedAt     *time.Time
		createdBy      *uuid.UUID
		updatedAt      *time.Time
		updatedBy      *uuid.UUID
		deletedAt      *time.Time
		deletedBy      *uuid.UUID
	)
	err := row.Scan(
		&rh.ID, &rh.RatingHistoryIDKode, &rh.CounterpartyID,
		&rh.TanggalBerlaku, &tanggalBerakhir,
		&rh.RatingPefindo, &ratingOutlook, &rh.SumberRating,
		&rh.TanggalPublikasiRating, &actionTypeStr,
		&rh.NotchChange, &rh.SicrTriggered, &rh.DefaultTriggered,
		&dokumenBuktiID,
		&rh.MakerID, &approverID, &approvedAt,
		&rh.CreatedAt, &createdBy, &updatedAt, &updatedBy,
		&deletedAt, &deletedBy, &rh.RowVersion, &rh.TenantID,
		&wfStatusStr,
	)
	if err != nil {
		return nil, err
	}
	rh.ActionType = ActionType(actionTypeStr)
	rh.WorkflowStatus = WorkflowStatus(wfStatusStr)
	rh.TanggalBerakhir = tanggalBerakhir
	rh.RatingOutlook = ratingOutlook
	rh.DokumenBuktiID = dokumenBuktiID
	rh.ApproverID = approverID
	rh.ApprovedAt = approvedAt
	rh.CreatedBy = createdBy
	rh.UpdatedAt = updatedAt
	rh.UpdatedBy = updatedBy
	rh.DeletedAt = deletedAt
	rh.DeletedBy = deletedBy
	return rh, nil
}

func scanRatingHistoryRow(rows *sql.Rows) (*RatingHistory, error) {
	rh := &RatingHistory{}
	var (
		actionTypeStr   string
		wfStatusStr     string
		tanggalBerakhir *string
		ratingOutlook   *string
		dokumenBuktiID  *uuid.UUID
		approverID      *uuid.UUID
		approvedAt      *time.Time
		createdBy       *uuid.UUID
		updatedAt       *time.Time
		updatedBy       *uuid.UUID
		deletedAt       *time.Time
		deletedBy       *uuid.UUID
	)
	err := rows.Scan(
		&rh.ID, &rh.RatingHistoryIDKode, &rh.CounterpartyID,
		&rh.TanggalBerlaku, &tanggalBerakhir,
		&rh.RatingPefindo, &ratingOutlook, &rh.SumberRating,
		&rh.TanggalPublikasiRating, &actionTypeStr,
		&rh.NotchChange, &rh.SicrTriggered, &rh.DefaultTriggered,
		&dokumenBuktiID,
		&rh.MakerID, &approverID, &approvedAt,
		&rh.CreatedAt, &createdBy, &updatedAt, &updatedBy,
		&deletedAt, &deletedBy, &rh.RowVersion, &rh.TenantID,
		&wfStatusStr,
	)
	if err != nil {
		return nil, err
	}
	rh.ActionType = ActionType(actionTypeStr)
	rh.WorkflowStatus = WorkflowStatus(wfStatusStr)
	rh.TanggalBerakhir = tanggalBerakhir
	rh.RatingOutlook = ratingOutlook
	rh.DokumenBuktiID = dokumenBuktiID
	rh.ApproverID = approverID
	rh.ApprovedAt = approvedAt
	rh.CreatedBy = createdBy
	rh.UpdatedAt = updatedAt
	rh.UpdatedBy = updatedBy
	rh.DeletedAt = deletedAt
	rh.DeletedBy = deletedBy
	return rh, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "23505") ||
		strings.Contains(s, "duplicate key") ||
		strings.Contains(s, "unique constraint")
}

func boolStr(b bool) string {
	if b {
		return "Ya"
	}
	return "Tidak"
}
