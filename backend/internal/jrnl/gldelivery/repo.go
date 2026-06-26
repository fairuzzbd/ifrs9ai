package gldelivery

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// ListPage is a lightweight pagination result.
type ListPage struct {
	NextCursor string
	HasMore    bool
	Limit      int
}

// PaginationMeta converts a ListPage to the standard response PaginationMeta.
func (p ListPage) PaginationMeta() *response.PaginationMeta {
	var next *string
	if p.NextCursor != "" {
		nc := p.NextCursor
		next = &nc
	}
	return &response.PaginationMeta{NextCursor: next, HasMore: p.HasMore, Limit: p.Limit}
}

// ─── GlStatusUpdateFields ─────────────────────────────────────────────────────

// GlStatusUpdateFields holds fields to update in jrnl.gl_status.
type GlStatusUpdateFields struct {
	GlHostStatus           GlHostStatus
	GlHostJournalID        *string
	DeliveredAt            *time.Time
	RetryCount             *int
	LastRetryAt            *time.Time
	LastError              *string
	FailureCategory        *string
	GlResponsePayloadJsonb json.RawMessage
	ManualRetryBy          *uuid.UUID
	ManualRetryAt          *time.Time
	ManualRetryReason      *string
	DiscardedBy            *uuid.UUID
	DiscardedAt            *time.Time
	DiscardReason          *string
	PayloadSentAt          *time.Time
	DeliveryResponseID     *string
}

// ─── JurnalGLRepo ─────────────────────────────────────────────────────────────

// AllowedGLStatusSortCols is the DataTable whitelist for gl_status queries.
var AllowedGLStatusSortCols = []string{
	"last_retry_at", "retry_count", "created_at", "updated_at",
	"gl_host_status", "failure_category",
}

func init() {
	// init-time assertion: ensure all sort cols are lowercase snake_case.
	for _, c := range AllowedGLStatusSortCols {
		if c != strings.ToLower(c) {
			panic("gldelivery: AllowedGLStatusSortCols must be lowercase snake_case: " + c)
		}
	}
}

// JurnalGLRepo handles jrnl.gl_status queries.
type JurnalGLRepo struct {
	db *sql.DB
}

// NewJurnalGLRepo creates a JurnalGLRepo. Panics on nil db.
func NewJurnalGLRepo(db *sql.DB) *JurnalGLRepo {
	if db == nil {
		panic("gldelivery.NewJurnalGLRepo: db must not be nil")
	}
	return &JurnalGLRepo{db: db}
}

// BeginTx opens a read-committed transaction.
func (r *JurnalGLRepo) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
}

// GetDeliveryStatus returns the current DeliveryStatus for a jurnal_header.
func (r *JurnalGLRepo) GetDeliveryStatus(ctx context.Context, jurnalHeaderID uuid.UUID) (*DeliveryStatus, error) {
	const q = `
		SELECT gs.id, gs.gl_host_status, gs.gl_host_journal_id, gs.delivered_at,
		       gs.retry_count, gs.last_retry_at, gs.last_error, gs.failure_category,
		       gs.delivery_mode, gs.payload_sent_at, gs.gl_response_payload_jsonb,
		       gs.manual_retry_by, gs.manual_retry_at, gs.manual_retry_reason,
		       gs.delivery_response_id
		FROM jrnl.gl_status gs
		WHERE gs.header_id = $1
		LIMIT 1`

	row := r.db.QueryRowContext(ctx, q, jurnalHeaderID)
	ds := &DeliveryStatus{JurnalHeaderID: jurnalHeaderID}

	var (
		glHostJournalID        sql.NullString
		deliveredAt            sql.NullTime
		lastRetryAt            sql.NullTime
		lastError              sql.NullString
		failureCategory        sql.NullString
		payloadSentAt          sql.NullTime
		glResponsePayloadJsonb []byte
		manualRetryBy          uuid.NullUUID
		manualRetryAt          sql.NullTime
		manualRetryReason      sql.NullString
		deliveryResponseID     sql.NullString
		deliveryMode           sql.NullString
	)

	err := row.Scan(
		&ds.GlStatusID, &ds.GlHostStatus, &glHostJournalID, &deliveredAt,
		&ds.RetryCount, &lastRetryAt, &lastError, &failureCategory,
		&deliveryMode, &payloadSentAt, &glResponsePayloadJsonb,
		&manualRetryBy, &manualRetryAt, &manualRetryReason,
		&deliveryResponseID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("gldelivery.JurnalGLRepo.GetDeliveryStatus: %w", err)
	}

	if glHostJournalID.Valid {
		ds.GlHostJournalID = &glHostJournalID.String
	}
	if deliveredAt.Valid {
		ds.DeliveredAt = &deliveredAt.Time
	}
	if lastRetryAt.Valid {
		ds.LastRetryAt = &lastRetryAt.Time
	}
	if lastError.Valid {
		ds.LastError = &lastError.String
	}
	if failureCategory.Valid {
		ds.FailureCategory = &failureCategory.String
	}
	if payloadSentAt.Valid {
		ds.PayloadSentAt = &payloadSentAt.Time
	}
	if deliveryMode.Valid {
		ds.DeliveryMode = deliveryMode.String
	} else {
		ds.DeliveryMode = "API"
	}
	if len(glResponsePayloadJsonb) > 0 {
		raw := json.RawMessage(glResponsePayloadJsonb)
		ds.GlResponsePayloadJsonb = &raw
	}
	if manualRetryBy.Valid && manualRetryAt.Valid && manualRetryReason.Valid {
		ds.ManualRetryHistory = []ManualRetryHistoryItem{{
			RetriedBy: manualRetryBy.UUID,
			RetriedAt: manualRetryAt.Time,
			Reason:    manualRetryReason.String,
		}}
	}

	return ds, nil
}

// UpdateGLStatus updates delivery-related columns on jrnl.gl_status within an existing tx.
// jrnl.gl_status terminal state guard is enforced at DB level (trigger from migration 000037).
func (r *JurnalGLRepo) UpdateGLStatus(ctx context.Context, tx *sql.Tx, jurnalHeaderID uuid.UUID, f GlStatusUpdateFields) error {
	setClauses := []string{"gl_host_status = $1", "updated_by = $2"}
	args := []any{string(f.GlHostStatus), uuid.Nil} // updated_by = system worker UUID
	pos := 3

	if f.GlHostJournalID != nil {
		setClauses = append(setClauses, fmt.Sprintf("gl_host_journal_id = $%d", pos))
		args = append(args, *f.GlHostJournalID)
		pos++
	}
	if f.DeliveredAt != nil {
		setClauses = append(setClauses, fmt.Sprintf("delivered_at = $%d", pos))
		args = append(args, *f.DeliveredAt)
		pos++
	}
	if f.RetryCount != nil {
		setClauses = append(setClauses, fmt.Sprintf("retry_count = $%d", pos))
		args = append(args, *f.RetryCount)
		pos++
	}
	if f.LastRetryAt != nil {
		setClauses = append(setClauses, fmt.Sprintf("last_retry_at = $%d", pos))
		args = append(args, *f.LastRetryAt)
		pos++
	}
	if f.LastError != nil {
		setClauses = append(setClauses, fmt.Sprintf("last_error = $%d", pos))
		args = append(args, *f.LastError)
		pos++
	}
	if f.FailureCategory != nil {
		setClauses = append(setClauses, fmt.Sprintf("failure_category = $%d", pos))
		args = append(args, *f.FailureCategory)
		pos++
	}
	if len(f.GlResponsePayloadJsonb) > 0 {
		setClauses = append(setClauses, fmt.Sprintf("gl_response_payload_jsonb = $%d", pos))
		args = append(args, []byte(f.GlResponsePayloadJsonb))
		pos++
	}
	if f.ManualRetryBy != nil {
		setClauses = append(setClauses, fmt.Sprintf("manual_retry_by = $%d", pos))
		args = append(args, *f.ManualRetryBy)
		pos++
	}
	if f.ManualRetryAt != nil {
		setClauses = append(setClauses, fmt.Sprintf("manual_retry_at = $%d", pos))
		args = append(args, *f.ManualRetryAt)
		pos++
	}
	if f.ManualRetryReason != nil {
		setClauses = append(setClauses, fmt.Sprintf("manual_retry_reason = $%d", pos))
		args = append(args, *f.ManualRetryReason)
		pos++
	}
	if f.DiscardedBy != nil {
		setClauses = append(setClauses, fmt.Sprintf("discarded_by = $%d", pos))
		args = append(args, *f.DiscardedBy)
		pos++
	}
	if f.DiscardedAt != nil {
		setClauses = append(setClauses, fmt.Sprintf("discarded_at = $%d", pos))
		args = append(args, *f.DiscardedAt)
		pos++
	}
	if f.DiscardReason != nil {
		setClauses = append(setClauses, fmt.Sprintf("discard_reason = $%d", pos))
		args = append(args, *f.DiscardReason)
		pos++
	}
	if f.PayloadSentAt != nil {
		setClauses = append(setClauses, fmt.Sprintf("payload_sent_at = $%d", pos))
		args = append(args, *f.PayloadSentAt)
		pos++
	}
	if f.DeliveryResponseID != nil {
		setClauses = append(setClauses, fmt.Sprintf("delivery_response_id = $%d", pos))
		args = append(args, *f.DeliveryResponseID)
		pos++
	}

	args = append(args, jurnalHeaderID)
	query := fmt.Sprintf( //nolint:gosec
		"UPDATE jrnl.gl_status SET %s WHERE header_id = $%d",
		strings.Join(setClauses, ", "), pos,
	)

	_, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("gldelivery.JurnalGLRepo.UpdateGLStatus: %w", err)
	}
	return nil
}

// ListPendingDelivery returns jrnl.gl_status rows for worker pickup.
func (r *JurnalGLRepo) ListPendingDelivery(ctx context.Context, limit int) ([]uuid.UUID, error) {
	const q = `
		SELECT header_id FROM jrnl.gl_status
		WHERE gl_host_status IN ('PENDING_DELIVERY', 'RETRYING')
		ORDER BY updated_at ASC
		LIMIT $1`
	rows, err := r.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("gldelivery.JurnalGLRepo.ListPendingDelivery: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("gldelivery.JurnalGLRepo.ListPendingDelivery scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetJurnalHeaderForDelivery returns the jurnal header + detail needed to build the GL payload.
// Returns nil if not found.
func (r *JurnalGLRepo) GetJurnalHeaderForDelivery(ctx context.Context, headerID uuid.UUID) (*JurnalHeaderDelivery, error) {
	const hq = `
		SELECT h.id, h.no_jurnal, h.tanggal_posting, h.event_code, h.narrative,
		       h.total_debit, h.total_kredit, h.idempotency_key, h.status_internal,
		       gs.id, gs.gl_host_status, gs.retry_count
		FROM jrnl.header h
		JOIN jrnl.gl_status gs ON gs.header_id = h.id
		WHERE h.id = $1`

	row := r.db.QueryRowContext(ctx, hq, headerID)
	jh := &JurnalHeaderDelivery{}
	var (
		totalDebit     string
		totalKredit    string
		statusInternal string
		glStatusID     uuid.UUID
		glHostStatus   string
		retryCount     int
	)
	err := row.Scan(
		&jh.ID, &jh.NoJurnal, &jh.TanggalPosting, &jh.EventCode,
		&jh.Narrative, &totalDebit, &totalKredit, &jh.IdempotencyKey,
		&statusInternal, &glStatusID, &glHostStatus, &retryCount,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("gldelivery.JurnalGLRepo.GetJurnalHeaderForDelivery: %w", err)
	}
	jh.TotalDebit, _ = decimal.NewFromString(totalDebit)   //nolint:errcheck
	jh.TotalKredit, _ = decimal.NewFromString(totalKredit) //nolint:errcheck
	jh.StatusInternal = statusInternal
	jh.GlStatusID = glStatusID
	jh.GlHostStatus = GlHostStatus(glHostStatus)
	jh.RetryCount = retryCount

	// Load detail rows.
	const dq = `
		SELECT d.id, d.urutan, d.debit_amount, d.kredit_amount, d.mata_uang, d.narrative_line,
		       c.kode_akun, c.nama_akun
		FROM jrnl.detail d
		JOIN mst.chart_of_accounts c ON c.id = d.kode_akun_id
		WHERE d.header_id = $1
		ORDER BY d.urutan`
	dRows, err := r.db.QueryContext(ctx, dq, headerID)
	if err != nil {
		return nil, fmt.Errorf("gldelivery.JurnalGLRepo.GetJurnalHeaderForDelivery detail: %w", err)
	}
	defer dRows.Close() //nolint:errcheck
	for dRows.Next() {
		var dl JurnalDetailDelivery
		var debit, kredit string
		if err := dRows.Scan(&dl.ID, &dl.Urutan, &debit, &kredit, &dl.MataUang, &dl.NarrativeLine, &dl.KodeAkun, &dl.NamaAkun); err != nil {
			return nil, fmt.Errorf("gldelivery.JurnalGLRepo.GetJurnalHeaderForDelivery detail scan: %w", err)
		}
		dl.DebitAmount, _ = decimal.NewFromString(debit)   //nolint:errcheck
		dl.KreditAmount, _ = decimal.NewFromString(kredit) //nolint:errcheck
		jh.DetailRows = append(jh.DetailRows, dl)
	}
	return jh, dRows.Err()
}

// JurnalHeaderDelivery holds the header + details needed to build the GL Host payload.
type JurnalHeaderDelivery struct {
	ID             uuid.UUID
	NoJurnal       string
	TanggalPosting time.Time
	EventCode      string
	Narrative      string
	TotalDebit     decimal.Decimal
	TotalKredit    decimal.Decimal
	IdempotencyKey string
	StatusInternal string
	GlStatusID     uuid.UUID
	GlHostStatus   GlHostStatus
	RetryCount     int
	DetailRows     []JurnalDetailDelivery
}

// JurnalDetailDelivery is one debit/kredit detail row.
type JurnalDetailDelivery struct {
	ID            uuid.UUID
	Urutan        int
	DebitAmount   decimal.Decimal
	KreditAmount  decimal.Decimal
	MataUang      string
	NarrativeLine string
	KodeAkun      string
	NamaAkun      string
}

// GetForRecon returns a map of kode_akun → net IDR amount for a given date (BLIPS side).
//
//nolint:revive
func (r *JurnalGLRepo) GetForRecon(ctx context.Context, date time.Time, tenantID string) (map[string]reconAkunData, error) {
	dateStr := date.Format("2006-01-02")
	const q = `
		SELECT c.id, c.kode_akun, c.nama_akun,
		       SUM(d.debit_amount - d.kredit_amount) AS net_idr,
		       ARRAY_AGG(DISTINCT h.id) AS header_ids
		FROM jrnl.detail d
		JOIN jrnl.header h ON d.header_id = h.id
		JOIN mst.chart_of_accounts c ON c.id = d.kode_akun_id
		WHERE DATE(h.tanggal_posting) = $1
		  AND h.status_internal = 'POSTED'
		  AND h.tenant_id = $2
		GROUP BY c.id, c.kode_akun, c.nama_akun`

	rows, err := r.db.QueryContext(ctx, q, dateStr, tenantID)
	if err != nil {
		return nil, fmt.Errorf("gldelivery.JurnalGLRepo.GetForRecon: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	result := make(map[string]reconAkunData)
	for rows.Next() {
		var (
			akunID    uuid.UUID
			kodeAkun  string
			namaAkun  string
			netIDRStr string
			headerIDs []byte
		)
		if err := rows.Scan(&akunID, &kodeAkun, &namaAkun, &netIDRStr, &headerIDs); err != nil {
			return nil, fmt.Errorf("gldelivery.JurnalGLRepo.GetForRecon scan: %w", err)
		}
		netIDR, _ := decimal.NewFromString(netIDRStr) //nolint:errcheck

		// Parse PostgreSQL ARRAY_AGG UUID array.
		var headerUUIDs []uuid.UUID
		// PostgreSQL returns array as {uuid1,uuid2,...}
		raw := strings.Trim(string(headerIDs), "{}")
		if raw != "" && raw != "NULL" {
			for _, part := range strings.Split(raw, ",") {
				part = strings.TrimSpace(part)
				if id, err := uuid.Parse(part); err == nil {
					headerUUIDs = append(headerUUIDs, id)
				}
			}
		}

		result[kodeAkun] = reconAkunData{
			AkunID:    akunID,
			KodeAkun:  kodeAkun,
			NamaAkun:  namaAkun,
			NetIDR:    netIDR,
			HeaderIDs: headerUUIDs,
		}
	}
	return result, rows.Err()
}

type reconAkunData struct {
	AkunID    uuid.UUID
	KodeAkun  string
	NamaAkun  string
	NetIDR    decimal.Decimal
	HeaderIDs []uuid.UUID
}

// ─── DLQRepo ─────────────────────────────────────────────────────────────────

// AllowedDLQSortCols is the DataTable whitelist.
var AllowedDLQSortCols = []string{
	"last_retry_at", "retry_count", "created_at", "updated_at", "status",
}

// DLQRepo handles sys.dlq_gl_delivery.
type DLQRepo struct {
	db *sql.DB
}

// NewDLQRepo creates a DLQRepo. Panics on nil db.
func NewDLQRepo(db *sql.DB) *DLQRepo {
	if db == nil {
		panic("gldelivery.NewDLQRepo: db must not be nil")
	}
	return &DLQRepo{db: db}
}

// BeginTx opens a read-committed transaction.
func (r *DLQRepo) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
}

// Insert inserts a new DLQ entry (within an open tx).
func (r *DLQRepo) Insert(ctx context.Context, tx *sql.Tx, e DLQEntry) error {
	payload := []byte("{}")
	if len(e.PayloadJsonb) > 0 {
		payload = e.PayloadJsonb
	}

	var glStatusID *uuid.UUID
	if e.GlStatusID != nil {
		glStatusID = e.GlStatusID
	}

	_, err := tx.ExecContext(
		ctx, `
		INSERT INTO sys.dlq_gl_delivery
		  (id, jurnal_header_id, gl_status_id, payload_jsonb,
		   error_code, error_message, error_category,
		   retry_count, status,
		   created_at, created_by, updated_at, updated_by, tenant_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now(), $10, now(), $10, $11)
		ON CONFLICT (jurnal_header_id, tenant_id)
		  WHERE status IN ('FAILED', 'REPLAYING')
		  DO NOTHING`,
		e.ID, e.JurnalHeaderID, glStatusID, payload,
		e.ErrorCode, e.ErrorMessage, e.ErrorCategory,
		e.RetryCount, string(DLQStatusFailed),
		e.CreatedAt, // created_by = system actor UUID
		"TUGURE",
	)
	if err != nil {
		return fmt.Errorf("gldelivery.DLQRepo.Insert: %w", err)
	}
	return nil
}

// GetByID returns a full DLQ entry.
func (r *DLQRepo) GetByID(ctx context.Context, id uuid.UUID) (*DLQEntry, error) {
	const q = `
		SELECT d.id, d.jurnal_header_id, d.gl_status_id, d.payload_jsonb,
		       d.error_code, d.error_message, d.error_category,
		       d.retry_count, d.last_retry_at, d.status,
		       d.replayed_by, d.replayed_at, d.final_delivery_response_id,
		       d.discarded_reason, d.discarded_by, d.discarded_at,
		       d.created_at, d.updated_at, d.row_version, d.tenant_id,
		       h.no_jurnal, h.event_code, h.tanggal_posting
		FROM sys.dlq_gl_delivery d
		JOIN jrnl.header h ON h.id = d.jurnal_header_id
		WHERE d.id = $1`

	row := r.db.QueryRowContext(ctx, q, id)
	return scanDLQEntry(row)
}

// GetByJurnalHeaderID returns the active (FAILED/REPLAYING) DLQ entry for a header.
func (r *DLQRepo) GetByJurnalHeaderID(ctx context.Context, headerID uuid.UUID) (*DLQEntry, error) {
	const q = `
		SELECT d.id, d.jurnal_header_id, d.gl_status_id, d.payload_jsonb,
		       d.error_code, d.error_message, d.error_category,
		       d.retry_count, d.last_retry_at, d.status,
		       d.replayed_by, d.replayed_at, d.final_delivery_response_id,
		       d.discarded_reason, d.discarded_by, d.discarded_at,
		       d.created_at, d.updated_at, d.row_version, d.tenant_id,
		       h.no_jurnal, h.event_code, h.tanggal_posting
		FROM sys.dlq_gl_delivery d
		JOIN jrnl.header h ON h.id = d.jurnal_header_id
		WHERE d.jurnal_header_id = $1
		  AND d.status IN ('FAILED', 'REPLAYING')
		ORDER BY d.created_at DESC
		LIMIT 1`

	row := r.db.QueryRowContext(ctx, q, headerID)
	return scanDLQEntry(row)
}

// UpdateStatusTx updates DLQ status fields within an open tx (audit-in-tx pattern).
func (r *DLQRepo) UpdateStatusTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, status DLQStatus, fields map[string]any) error {
	setClauses := []string{"status = $1", "updated_at = now()", "updated_by = $2"}
	args := []any{string(status), uuid.Nil}
	pos := 3

	for k, v := range fields {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", k, pos))
		args = append(args, v)
		pos++
	}
	args = append(args, id)
	query := fmt.Sprintf("UPDATE sys.dlq_gl_delivery SET %s WHERE id = $%d", //nolint:gosec
		strings.Join(setClauses, ", "), pos)

	_, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("gldelivery.DLQRepo.UpdateStatusTx: %w", err)
	}
	return nil
}

// List returns paginated DLQ entries for the DataTable.
func (r *DLQRepo) List(ctx context.Context, _ listquery.Query, limit int, statusFilter string) ([]DLQEntrySummary, ListPage, error) {
	where := "d.status IN ('FAILED', 'REPLAYING')"

	if statusFilter == "DEAD_LETTER" {
		where = "gs.gl_host_status = 'DEAD_LETTER'"
	} else if statusFilter == "ALL" {
		where = "d.status IN ('FAILED', 'REPLAYING', 'REPLAYED_OK', 'ABANDONED') OR gs.gl_host_status = 'DEAD_LETTER'"
	}

	fetchLimit := limit + 1

	//nolint:gosec
	query := fmt.Sprintf(`
		SELECT d.id, d.jurnal_header_id, d.error_code, d.error_message,
		       d.error_category, d.retry_count, d.last_retry_at, d.status,
		       d.created_at, h.no_jurnal, h.event_code, h.tanggal_posting,
		       gs.gl_host_status
		FROM sys.dlq_gl_delivery d
		JOIN jrnl.header h ON h.id = d.jurnal_header_id
		LEFT JOIN jrnl.gl_status gs ON gs.header_id = d.jurnal_header_id
		WHERE %s
		ORDER BY d.created_at DESC
		LIMIT $1`, where)

	args := []any{fetchLimit}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, ListPage{}, fmt.Errorf("gldelivery.DLQRepo.List: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []DLQEntrySummary
	for rows.Next() {
		var item DLQEntrySummary
		var (
			lastRetryAt    sql.NullTime
			tanggalPosting sql.NullTime
			errorMessage   sql.NullString
			glHostStatus   sql.NullString
			dlqStatus      string
		)
		if err := rows.Scan(
			&item.DLQEntryID, &item.JurnalHeaderID, &item.ErrorCode, &errorMessage,
			&item.FailureCategory, &item.RetryCount, &lastRetryAt, &dlqStatus,
			&item.CreatedAt, &item.NoJurnal, &item.EventCode, &tanggalPosting,
			&glHostStatus,
		); err != nil {
			return nil, ListPage{}, fmt.Errorf("gldelivery.DLQRepo.List scan: %w", err)
		}
		item.Status = DLQStatus(dlqStatus)
		if errorMessage.Valid {
			item.ErrorMessage = &errorMessage.String
		}
		if lastRetryAt.Valid {
			item.LastRetryAt = &lastRetryAt.Time
		}
		if tanggalPosting.Valid {
			item.TanggalPosting = &tanggalPosting.Time
		}
		if glHostStatus.Valid {
			item.GlHostStatus = GlHostStatus(glHostStatus.String)
		} else {
			item.GlHostStatus = GlHostStatusFailed
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, ListPage{}, fmt.Errorf("gldelivery.DLQRepo.List rows: %w", err)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	var nextCursor string
	if hasMore && len(items) > 0 {
		nextCursor = items[len(items)-1].CreatedAt.Format(time.RFC3339Nano)
	}
	return items, ListPage{NextCursor: nextCursor, HasMore: hasMore, Limit: limit}, nil
}

func scanDLQEntry(row *sql.Row) (*DLQEntry, error) {
	var e DLQEntry
	var (
		glStatusID              uuid.NullUUID
		payload                 []byte
		lastRetryAt             sql.NullTime
		replayedBy              uuid.NullUUID
		replayedAt              sql.NullTime
		finalDeliveryResponseID sql.NullString
		discardedReason         sql.NullString
		discardedBy             uuid.NullUUID
		discardedAt             sql.NullTime
		noJurnal                sql.NullString
		eventCode               sql.NullString
		tanggalPosting          sql.NullTime
	)
	err := row.Scan(
		&e.ID, &e.JurnalHeaderID, &glStatusID, &payload,
		&e.ErrorCode, &e.ErrorMessage, &e.ErrorCategory,
		&e.RetryCount, &lastRetryAt, &e.Status,
		&replayedBy, &replayedAt, &finalDeliveryResponseID,
		&discardedReason, &discardedBy, &discardedAt,
		&e.CreatedAt, &e.UpdatedAt, &e.RowVersion, &e.TenantID,
		&noJurnal, &eventCode, &tanggalPosting,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("gldelivery.scanDLQEntry: %w", err)
	}
	if glStatusID.Valid {
		e.GlStatusID = &glStatusID.UUID
	}
	e.PayloadJsonb = json.RawMessage(payload)
	if lastRetryAt.Valid {
		e.LastRetryAt = &lastRetryAt.Time
	}
	if replayedBy.Valid {
		e.ReplayedBy = &replayedBy.UUID
	}
	if replayedAt.Valid {
		e.ReplayedAt = &replayedAt.Time
	}
	if finalDeliveryResponseID.Valid {
		e.FinalDeliveryResponseID = &finalDeliveryResponseID.String
	}
	if discardedReason.Valid {
		e.DiscardedReason = &discardedReason.String
	}
	if discardedBy.Valid {
		e.DiscardedBy = &discardedBy.UUID
	}
	if discardedAt.Valid {
		e.DiscardedAt = &discardedAt.Time
	}
	if noJurnal.Valid {
		e.NoJurnal = noJurnal.String
	}
	if eventCode.Valid {
		e.EventCode = eventCode.String
	}
	if tanggalPosting.Valid {
		tp := tanggalPosting.Time
		e.TanggalPosting = &tp
	}
	return &e, nil
}

// ─── ReconReportRepo ─────────────────────────────────────────────────────────

// AllowedReconSortCols is the DataTable whitelist.
var AllowedReconSortCols = []string{
	"tanggal_run", "generated_at", "mismatch_count", "status", "started_at",
}

// ReconReportRepo handles sys.gl_reconciliation_report.
type ReconReportRepo struct {
	db *sql.DB
}

// NewReconReportRepo creates a ReconReportRepo. Panics on nil db.
func NewReconReportRepo(db *sql.DB) *ReconReportRepo {
	if db == nil {
		panic("gldelivery.NewReconReportRepo: db must not be nil")
	}
	return &ReconReportRepo{db: db}
}

// BeginTx opens a read-committed transaction.
func (r *ReconReportRepo) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
}

// Insert inserts a new reconciliation report (in tx).
func (r *ReconReportRepo) Insert(ctx context.Context, tx *sql.Tx, report *ReconciliationReport, actorID uuid.UUID) error {
	summaryJSON, _ := json.Marshal(map[string]any{}) //nolint:errcheck
	if report.SummaryJsonb != nil {
		summaryJSON, _ = json.Marshal(report.SummaryJsonb) //nolint:errcheck
	}

	_, err := tx.ExecContext(
		ctx, `
		INSERT INTO sys.gl_reconciliation_report
		  (id, tanggal_run, trigger_source, triggered_by, asynq_job_id, status,
		   started_at, total_jurnal_idr, mismatch_count, tolerance_idr,
		   summary_jsonb, created_at, created_by, updated_at, updated_by, tenant_id)
		VALUES ($1, $2, $3, $4, $5, $6, now(), $7, $8, $9, $10, now(), $11, now(), $11, $12)`,
		report.ID, report.TanggalRun.Format("2006-01-02"), report.TriggerSource,
		report.TriggeredBy, report.AsynqJobID, string(report.Status),
		report.TotalJurnalIDR.StringFixed(4), report.MismatchCount,
		report.ToleranceIDR.StringFixed(4), summaryJSON, actorID, "TUGURE",
	)
	if err != nil {
		return fmt.Errorf("gldelivery.ReconReportRepo.Insert: %w", err)
	}
	return nil
}

// Update updates the report status + completion fields (in tx).
func (r *ReconReportRepo) Update(ctx context.Context, tx *sql.Tx, report *ReconciliationReport, actorID uuid.UUID) error {
	var glHostTotal *string
	if report.GlHostTotalIDR != nil {
		s := report.GlHostTotalIDR.StringFixed(4)
		glHostTotal = &s
	}

	summaryJSON := json.RawMessage(`{}`)
	if report.SummaryJsonb != nil {
		summaryJSON = *report.SummaryJsonb
	}
	glSnapshotJSON := json.RawMessage(`{}`)
	if report.GlHostSnapshotJsonb != nil {
		glSnapshotJSON = *report.GlHostSnapshotJsonb
	}

	_, err := tx.ExecContext(
		ctx, `
		UPDATE sys.gl_reconciliation_report
		SET status = $1, completed_at = now(), total_jurnal_idr = $2,
		    gl_host_total_idr = $3, mismatch_count = $4,
		    error_summary = $5, summary_jsonb = $6,
		    gl_host_snapshot_jsonb = $7,
		    updated_at = now(), updated_by = $8
		WHERE id = $9`,
		string(report.Status), report.TotalJurnalIDR.StringFixed(4),
		glHostTotal, report.MismatchCount,
		report.ErrorSummary, summaryJSON, glSnapshotJSON,
		actorID, report.ID,
	)
	if err != nil {
		return fmt.Errorf("gldelivery.ReconReportRepo.Update: %w", err)
	}
	return nil
}

// GetByDate returns the most recent completed report for a date.
func (r *ReconReportRepo) GetByDate(ctx context.Context, date time.Time, tenantID string) (*ReconciliationReport, error) {
	const q = `
		SELECT id, tanggal_run, trigger_source, triggered_by, asynq_job_id,
		       status, started_at, completed_at, total_jurnal_idr, gl_host_total_idr,
		       mismatch_count, tolerance_idr, error_summary, summary_jsonb
		FROM sys.gl_reconciliation_report
		WHERE DATE(tanggal_run) = $1 AND tenant_id = $2
		  AND status IN ('COMPLETED', 'FAILED', 'IN_PROGRESS')
		ORDER BY started_at DESC
		LIMIT 1`

	row := r.db.QueryRowContext(ctx, q, date.Format("2006-01-02"), tenantID)
	return scanReconReport(row)
}

// IsInProgress returns true if there is an IN_PROGRESS report for the date.
func (r *ReconReportRepo) IsInProgress(ctx context.Context, date time.Time, tenantID string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(
		ctx, `
		SELECT COUNT(*) FROM sys.gl_reconciliation_report
		WHERE DATE(tanggal_run) = $1 AND tenant_id = $2 AND status = 'IN_PROGRESS'`,
		date.Format("2006-01-02"), tenantID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("gldelivery.ReconReportRepo.IsInProgress: %w", err)
	}
	return count > 0, nil
}

// List returns paginated reconciliation report summaries.
func (r *ReconReportRepo) List(ctx context.Context, _ listquery.Query, limit int, statusFilter string) ([]ReconSummaryItem, ListPage, error) {
	where := "deleted_at IS NULL"
	var args []any
	pos := 1

	if statusFilter != "" {
		where += fmt.Sprintf(" AND status = $%d", pos)
		args = append(args, statusFilter)
		pos++
	}

	fetchLimit := limit + 1
	args = append(args, fetchLimit)

	//nolint:gosec
	query := fmt.Sprintf(`
		SELECT id, tanggal_run, status, mismatch_count, completed_at, asynq_job_id
		FROM sys.gl_reconciliation_report
		WHERE %s
		ORDER BY tanggal_run DESC
		LIMIT $%d`, where, pos)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, ListPage{}, fmt.Errorf("gldelivery.ReconReportRepo.List: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []ReconSummaryItem
	for rows.Next() {
		var item ReconSummaryItem
		var (
			completedAt sql.NullTime
			jobID       sql.NullString
			tanggalRun  time.Time
		)
		if err := rows.Scan(&item.ReportID, &tanggalRun, &item.Status,
			&item.TotalMismatchCount, &completedAt, &jobID); err != nil {
			return nil, ListPage{}, fmt.Errorf("gldelivery.ReconReportRepo.List scan: %w", err)
		}
		item.TanggalRekonsiliasi = tanggalRun
		if completedAt.Valid {
			t := completedAt.Time
			item.GeneratedAt = &t
		}
		if jobID.Valid {
			item.JobID = &jobID.String
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, ListPage{}, fmt.Errorf("gldelivery.ReconReportRepo.List rows: %w", err)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	var nextCursor string
	if hasMore && len(items) > 0 {
		nextCursor = items[len(items)-1].TanggalRekonsiliasi.Format("2006-01-02")
	}
	return items, ListPage{NextCursor: nextCursor, HasMore: hasMore, Limit: limit}, nil
}

func scanReconReport(row *sql.Row) (*ReconciliationReport, error) {
	var rep ReconciliationReport
	var (
		triggeredBy  uuid.NullUUID
		jobID        sql.NullString
		completedAt  sql.NullTime
		glHostTotal  sql.NullString
		errorSummary sql.NullString
		summaryJsonb []byte
		totalJurnal  string
		toleranceIDR string
		tanggalRun   time.Time
	)
	err := row.Scan(
		&rep.ID, &tanggalRun, &rep.TriggerSource, &triggeredBy, &jobID,
		&rep.Status, &rep.StartedAt, &completedAt, &totalJurnal, &glHostTotal,
		&rep.MismatchCount, &toleranceIDR, &errorSummary, &summaryJsonb,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("gldelivery.scanReconReport: %w", err)
	}
	rep.TanggalRun = tanggalRun
	rep.TotalJurnalIDR, _ = decimal.NewFromString(totalJurnal) //nolint:errcheck
	rep.ToleranceIDR, _ = decimal.NewFromString(toleranceIDR)  //nolint:errcheck
	if triggeredBy.Valid {
		rep.TriggeredBy = &triggeredBy.UUID
	}
	if jobID.Valid {
		rep.AsynqJobID = &jobID.String
	}
	if completedAt.Valid {
		t := completedAt.Time
		rep.CompletedAt = &t
	}
	if glHostTotal.Valid {
		v, _ := decimal.NewFromString(glHostTotal.String) //nolint:errcheck
		rep.GlHostTotalIDR = &v
	}
	if errorSummary.Valid {
		rep.ErrorSummary = &errorSummary.String
	}
	if len(summaryJsonb) > 0 {
		raw := json.RawMessage(summaryJsonb)
		rep.SummaryJsonb = &raw
	}
	return &rep, nil
}

// ─── ReconMismatchRepo ────────────────────────────────────────────────────────

// ReconMismatchRepo handles sys.gl_recon_mismatch.
type ReconMismatchRepo struct {
	db *sql.DB
}

// NewReconMismatchRepo creates a ReconMismatchRepo. Panics on nil db.
func NewReconMismatchRepo(db *sql.DB) *ReconMismatchRepo {
	if db == nil {
		panic("gldelivery.NewReconMismatchRepo: db must not be nil")
	}
	return &ReconMismatchRepo{db: db}
}

// InsertBulk inserts multiple mismatch rows in tx.
func (r *ReconMismatchRepo) InsertBulk(ctx context.Context, tx *sql.Tx, mismatches []ReconMismatch, actorID uuid.UUID) error {
	for i := range mismatches {
		m := &mismatches[i]
		headerIDsJSON, _ := json.Marshal(m.JurnalHeaderIDs) //nolint:errcheck
		_, err := tx.ExecContext(
			ctx, `
			INSERT INTO sys.gl_recon_mismatch
			  (id, report_id, akun_id, blips_amount_idr, gl_host_amount_idr, delta_idr,
			   mismatch_type, jurnal_header_ids, note,
			   created_at, created_by, updated_at, updated_by, tenant_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now(), $10, now(), $10, $11)`,
			m.ID, m.ReportID, m.AkunID,
			m.BlipsAmountIDR.StringFixed(4), m.GlHostAmountIDR.StringFixed(4), m.DeltaIDR.StringFixed(4),
			string(m.MismatchType), headerIDsJSON, m.Note,
			actorID, "TUGURE",
		)
		if err != nil {
			return fmt.Errorf("gldelivery.ReconMismatchRepo.InsertBulk: %w", err)
		}
	}
	return nil
}

// SoftDeleteByReportID soft-deletes existing mismatch rows for a report (UPSERT pattern per OQ-M3-4b).
func (r *ReconMismatchRepo) SoftDeleteByReportID(ctx context.Context, tx *sql.Tx, reportID uuid.UUID, actorID uuid.UUID) error {
	_, err := tx.ExecContext(
		ctx,
		"UPDATE sys.gl_recon_mismatch SET deleted_at = now(), deleted_by = $1 WHERE report_id = $2 AND deleted_at IS NULL",
		actorID, reportID,
	)
	if err != nil {
		return fmt.Errorf("gldelivery.ReconMismatchRepo.SoftDeleteByReportID: %w", err)
	}
	return nil
}

// GetByReportID returns mismatch rows for a report.
func (r *ReconMismatchRepo) GetByReportID(ctx context.Context, reportID uuid.UUID) ([]ReconMismatch, error) {
	const q = `
		SELECT m.id, m.report_id, m.akun_id, m.blips_amount_idr, m.gl_host_amount_idr, m.delta_idr,
		       m.mismatch_type, m.jurnal_header_ids, m.note,
		       c.kode_akun, c.nama_akun
		FROM sys.gl_recon_mismatch m
		JOIN mst.chart_of_accounts c ON c.id = m.akun_id
		WHERE m.report_id = $1 AND m.deleted_at IS NULL
		ORDER BY ABS(m.delta_idr) DESC`

	rows, err := r.db.QueryContext(ctx, q, reportID)
	if err != nil {
		return nil, fmt.Errorf("gldelivery.ReconMismatchRepo.GetByReportID: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var mismatches []ReconMismatch
	for rows.Next() {
		var m ReconMismatch
		var (
			blips, glHost, delta string
			headerIDsJSON        []byte
			note                 sql.NullString
			namaAkun             sql.NullString
		)
		if err := rows.Scan(
			&m.ID, &m.ReportID, &m.AkunID, &blips, &glHost, &delta,
			&m.MismatchType, &headerIDsJSON, &note, &m.KodeAkun, &namaAkun,
		); err != nil {
			return nil, fmt.Errorf("gldelivery.ReconMismatchRepo.GetByReportID scan: %w", err)
		}
		m.BlipsAmountIDR, _ = decimal.NewFromString(blips)   //nolint:errcheck
		m.GlHostAmountIDR, _ = decimal.NewFromString(glHost) //nolint:errcheck
		m.DeltaIDR, _ = decimal.NewFromString(delta)         //nolint:errcheck
		if note.Valid {
			m.Note = &note.String
		}
		if namaAkun.Valid {
			n := namaAkun.String
			m.NamaAkun = &n
		}
		if len(headerIDsJSON) > 0 {
			_ = json.Unmarshal(headerIDsJSON, &m.JurnalHeaderIDs) //nolint:errcheck
		}
		mismatches = append(mismatches, m)
	}
	return mismatches, rows.Err()
}
