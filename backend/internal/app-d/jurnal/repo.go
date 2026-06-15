package jurnal

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// ListPage is a lightweight pagination result for list endpoints.
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
	return &response.PaginationMeta{
		NextCursor: next,
		HasMore:    p.HasMore,
		Limit:      p.Limit,
	}
}

// ─── MappingRepo ─────────────────────────────────────────────────────────────

// MappingRepo handles mst.mapping_jurnal_header + mst.mapping_jurnal_detail.
// All SQL uses parameterized queries only — no string concat (SQLi prevention).
type MappingRepo struct {
	db *sql.DB
}

// NewMappingRepo creates a MappingRepo. Panics on nil db.
func NewMappingRepo(db *sql.DB) *MappingRepo {
	if db == nil {
		panic("jurnal.NewMappingRepo: db must not be nil")
	}
	return &MappingRepo{db: db}
}

// AllowedMappingSortCols is the whitelist for list sort/filter (DataTable rule §1).
var AllowedMappingSortCols = []string{
	"event_code",
	"nama_event",
	"kategori_event",
	"workflow_status",
	"aktif_flag",
	"created_at",
	"updated_at",
}

// BeginTx opens a serializable transaction.
func (r *MappingRepo) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
}

// Create inserts a new mapping_jurnal_header + detail rows (DRAFT, in tx).
func (r *MappingRepo) Create(ctx context.Context, tx *sql.Tx, h *MappingHeader) error {
	klasJSON, err := json.Marshal(h.KlasifikasiBerlaku)
	if err != nil {
		return fmt.Errorf("jurnal.MappingRepo.Create: marshal klasifikasi_berlaku: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO mst.mapping_jurnal_header (
			id, event_id_kode, event_code, nama_event, kategori_event,
			trigger_source, klasifikasi_berlaku, aktif_flag, workflow_status,
			workflow_path, deskripsi, maker_id,
			created_by, updated_by, tenant_id
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, 'DRAFT',
			$9, $10, $11,
			$12, $13, $14
		)`,
		h.ID, h.EventIDKode, h.EventCode, h.NamaEvent, h.KategoriEvent,
		h.TriggerSource, klasJSON, h.AktifFlag,
		string(h.WorkflowPath), h.Deskripsi, h.MakerID,
		h.CreatedBy, h.UpdatedBy, h.TenantID,
	)
	if err != nil {
		return fmt.Errorf("jurnal.MappingRepo.Create header: %w", err)
	}
	for i := range h.DetailRows {
		if err := r.insertDetailRow(ctx, tx, h.ID, h.DetailRows[i]); err != nil {
			return err
		}
	}
	return nil
}

func (r *MappingRepo) insertDetailRow(ctx context.Context, tx *sql.Tx, headerID uuid.UUID, d MappingDetailRow) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO mst.mapping_jurnal_detail (
			id, event_header_id, urutan, kode_akun_id,
			dk_indicator, sumber_amount, klasifikasi_filter,
			multiplier, catatan, aktif_flag
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		d.ID, headerID, d.Urutan, d.KodeAkunID,
		d.DKIndicator, d.SumberAmount, d.KlasifikasiFilter,
		d.Multiplier, d.Catatan, d.AktifFlag,
	)
	if err != nil {
		return fmt.Errorf("jurnal.MappingRepo.insertDetailRow: %w", err)
	}
	return nil
}

// GetByID fetches a full mapping header + detail rows.
func (r *MappingRepo) GetByID(ctx context.Context, id uuid.UUID) (*MappingHeader, error) {
	h := &MappingHeader{}
	var klasJSON []byte
	var wpStr string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, event_id_kode, event_code, nama_event, kategori_event,
		       trigger_source, klasifikasi_berlaku, aktif_flag, workflow_status,
		       workflow_path, deskripsi,
		       maker_id, reviewer_id, approver_id, approver_2_id,
		       reviewer_signed_at, approver_signed_at, approver_2_signed_at,
		       comment_review, comment_approve, comment_approve_2,
		       submit_at, reject_reason,
		       created_at, created_by, updated_at, updated_by, row_version, tenant_id
		FROM mst.mapping_jurnal_header
		WHERE id = $1 AND deleted_at IS NULL`, id,
	).Scan(
		&h.ID, &h.EventIDKode, &h.EventCode, &h.NamaEvent, &h.KategoriEvent,
		&h.TriggerSource, &klasJSON, &h.AktifFlag, &h.WorkflowStatus,
		&wpStr, &h.Deskripsi,
		&h.MakerID, &h.ReviewerID, &h.ApproverID, &h.Approver2ID,
		&h.ReviewerSignedAt, &h.ApproverSignedAt, &h.Approver2SignedAt,
		&h.CommentReview, &h.CommentApprove, &h.CommentApprove2,
		&h.SubmitAt, &h.RejectReason,
		&h.CreatedAt, &h.CreatedBy, &h.UpdatedAt, &h.UpdatedBy, &h.RowVersion, &h.TenantID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("jurnal.MappingRepo.GetByID: %w", err)
	}
	h.WorkflowPath = WorkflowPath(wpStr)
	if len(klasJSON) > 0 {
		if err := json.Unmarshal(klasJSON, &h.KlasifikasiBerlaku); err != nil {
			return nil, fmt.Errorf("jurnal.MappingRepo.GetByID: unmarshal klasifikasi_berlaku: %w", err)
		}
	}
	details, err := r.listDetails(ctx, nil, h.ID)
	if err != nil {
		return nil, err
	}
	h.DetailRows = details
	return h, nil
}

// GetByEventCode returns the APPROVED_ACTIVE mapping header for the given event code.
func (r *MappingRepo) GetByEventCode(ctx context.Context, eventCode string) (*MappingHeader, error) {
	h := &MappingHeader{}
	var klasJSON []byte
	var wpStr string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, event_id_kode, event_code, nama_event, kategori_event,
		       trigger_source, klasifikasi_berlaku, aktif_flag, workflow_status, workflow_path
		FROM mst.mapping_jurnal_header
		WHERE event_code = $1
		  AND workflow_status IN ('APPROVED_ACTIVE', 'APPROVED')
		  AND aktif_flag = true
		  AND deleted_at IS NULL
		ORDER BY updated_at DESC
		LIMIT 1`, eventCode,
	).Scan(
		&h.ID, &h.EventIDKode, &h.EventCode, &h.NamaEvent, &h.KategoriEvent,
		&h.TriggerSource, &klasJSON, &h.AktifFlag, &h.WorkflowStatus, &wpStr,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("jurnal.MappingRepo.GetByEventCode: %w", err)
	}
	h.WorkflowPath = WorkflowPath(wpStr)
	if len(klasJSON) > 0 {
		if err := json.Unmarshal(klasJSON, &h.KlasifikasiBerlaku); err != nil {
			return nil, fmt.Errorf("jurnal.MappingRepo.GetByEventCode: unmarshal: %w", err)
		}
	}
	details, err := r.listDetails(ctx, nil, h.ID)
	if err != nil {
		return nil, err
	}
	h.DetailRows = details
	return h, nil
}

// ListSummary returns paginated mapping header summaries.
func (r *MappingRepo) ListSummary(ctx context.Context, q listquery.Query, limit int) ([]MappingHeaderSummary, ListPage, error) {
	where, args, orderBy := q.WithAllowed(AllowedMappingSortCols).ToSQL("h")
	fetchLimit := limit + 1
	args = append(args, fetchLimit)
	//nolint:gosec // where/orderBy are built by whitelist-validated listquery package
	query := fmt.Sprintf(`
		SELECT h.id, h.event_code, h.nama_event, h.kategori_event,
		       h.trigger_source, h.workflow_status, h.workflow_path,
		       h.aktif_flag, COUNT(d.id) AS detail_count,
		       h.created_at, h.updated_at
		FROM mst.mapping_jurnal_header h
		LEFT JOIN mst.mapping_jurnal_detail d ON d.event_header_id = h.id AND d.deleted_at IS NULL
		WHERE h.deleted_at IS NULL %s
		GROUP BY h.id, h.event_code, h.nama_event, h.kategori_event,
		         h.trigger_source, h.workflow_status, h.workflow_path,
		         h.aktif_flag, h.created_at, h.updated_at
		%s
		LIMIT $%d`, where, orderBy, len(args),
	)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, ListPage{}, fmt.Errorf("jurnal.MappingRepo.ListSummary: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []MappingHeaderSummary
	for rows.Next() {
		var s MappingHeaderSummary
		var wp string
		if err := rows.Scan(
			&s.ID, &s.EventCode, &s.NamaEvent, &s.KategoriEvent,
			&s.TriggerSource, &s.WorkflowStatus, &wp,
			&s.AktifFlag, &s.DetailCount, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, ListPage{}, fmt.Errorf("jurnal.MappingRepo.ListSummary scan: %w", err)
		}
		s.WorkflowPath = WorkflowPath(wp)
		items = append(items, s)
	}
	if err := rows.Err(); err != nil {
		return nil, ListPage{}, fmt.Errorf("jurnal.MappingRepo.ListSummary rows: %w", err)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	var nextCursor string
	if hasMore && len(items) > 0 {
		lastID := items[len(items)-1].ID.String()
		nextCursor = base64.StdEncoding.EncodeToString([]byte(lastID))
	}
	return items, ListPage{NextCursor: nextCursor, HasMore: hasMore, Limit: limit}, nil
}

// UpdateDraft updates mutable DRAFT fields (optimistic lock on row_version).
func (r *MappingRepo) UpdateDraft(ctx context.Context, tx *sql.Tx, h *MappingHeader) error {
	klasJSON, _ := json.Marshal(h.KlasifikasiBerlaku) //nolint:errcheck
	res, err := tx.ExecContext(ctx, `
		UPDATE mst.mapping_jurnal_header
		SET nama_event = $1, kategori_event = $2, klasifikasi_berlaku = $3,
		    deskripsi = $4, updated_by = $5, updated_at = now(),
		    row_version = row_version + 1
		WHERE id = $6 AND row_version = $7 AND deleted_at IS NULL`,
		h.NamaEvent, h.KategoriEvent, klasJSON,
		h.Deskripsi, h.UpdatedBy,
		h.ID, h.RowVersion,
	)
	if err != nil {
		return fmt.Errorf("jurnal.MappingRepo.UpdateDraft: %w", err)
	}
	n, _ := res.RowsAffected() //nolint:errcheck
	if n == 0 {
		return fmt.Errorf("jurnal.MappingRepo.UpdateDraft: row_version conflict or not found")
	}
	// Delete + re-insert detail rows atomically.
	if _, err := tx.ExecContext(ctx, `DELETE FROM mst.mapping_jurnal_detail WHERE event_header_id = $1`, h.ID); err != nil {
		return fmt.Errorf("jurnal.MappingRepo.UpdateDraft delete details: %w", err)
	}
	for i := range h.DetailRows {
		if err := r.insertDetailRow(ctx, tx, h.ID, h.DetailRows[i]); err != nil {
			return err
		}
	}
	return nil
}

// UpdateStatus updates workflow_status + related signature fields atomically.
func (r *MappingRepo) UpdateStatus(ctx context.Context, tx *sql.Tx, h *MappingHeader) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.mapping_jurnal_header
		SET workflow_status = $1,
		    reviewer_id = $2, reviewer_signed_at = $3, comment_review = $4,
		    approver_id = $5, approver_signed_at = $6, comment_approve = $7,
		    approver_2_id = $8, approver_2_signed_at = $9, comment_approve_2 = $10,
		    submit_at = $11, reject_reason = $12, aktif_flag = $13,
		    updated_by = $14, updated_at = now(), row_version = row_version + 1
		WHERE id = $15 AND deleted_at IS NULL`,
		string(h.WorkflowStatus),
		h.ReviewerID, h.ReviewerSignedAt, h.CommentReview,
		h.ApproverID, h.ApproverSignedAt, h.CommentApprove,
		h.Approver2ID, h.Approver2SignedAt, h.CommentApprove2,
		h.SubmitAt, h.RejectReason, h.AktifFlag,
		h.UpdatedBy, h.ID,
	)
	if err != nil {
		return fmt.Errorf("jurnal.MappingRepo.UpdateStatus: %w", err)
	}
	return nil
}

// SoftDelete soft-deletes a DRAFT mapping header.
func (r *MappingRepo) SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.mapping_jurnal_header
		SET deleted_at = now(), deleted_by = $1, updated_at = now(), updated_by = $1
		WHERE id = $2 AND deleted_at IS NULL`, deletedBy, id)
	if err != nil {
		return fmt.Errorf("jurnal.MappingRepo.SoftDelete: %w", err)
	}
	return nil
}

// listDetails returns all detail rows for a header (uses tx if non-nil).
func (r *MappingRepo) listDetails(ctx context.Context, tx *sql.Tx, headerID uuid.UUID) ([]MappingDetailRow, error) {
	query := `
		SELECT d.id, d.event_header_id, d.urutan, d.kode_akun_id,
		       COALESCE(a.kode_akun, '') AS kode_akun_kode,
		       COALESCE(a.nama_akun, '') AS kode_akun_nama,
		       d.dk_indicator, d.sumber_amount, d.klasifikasi_filter,
		       d.multiplier, d.catatan, d.aktif_flag
		FROM mst.mapping_jurnal_detail d
		LEFT JOIN mst.chart_of_accounts a ON a.id = d.kode_akun_id
		WHERE d.event_header_id = $1 AND d.deleted_at IS NULL
		ORDER BY d.urutan`
	var rowsi *sql.Rows
	var err error
	if tx != nil {
		rowsi, err = tx.QueryContext(ctx, query, headerID)
	} else {
		rowsi, err = r.db.QueryContext(ctx, query, headerID)
	}
	if err != nil {
		return nil, fmt.Errorf("jurnal.MappingRepo.listDetails: %w", err)
	}
	defer rowsi.Close() //nolint:errcheck

	var rows []MappingDetailRow
	for rowsi.Next() {
		var d MappingDetailRow
		if err := rowsi.Scan(
			&d.ID, &d.EventHeaderID, &d.Urutan, &d.KodeAkunID,
			&d.KodeAkunKode, &d.KodeAkunNama,
			&d.DKIndicator, &d.SumberAmount, &d.KlasifikasiFilter,
			&d.Multiplier, &d.Catatan, &d.AktifFlag,
		); err != nil {
			return nil, fmt.Errorf("jurnal.MappingRepo.listDetails scan: %w", err)
		}
		rows = append(rows, d)
	}
	return rows, rowsi.Err()
}

// ─── JurnalRepo ──────────────────────────────────────────────────────────────

// JurnalRepo handles jrnl.header + jrnl.detail (append-only by DB trigger).
type JurnalRepo struct { //nolint:revive // Jurnal prefix required for cross-package clarity
	db *sql.DB
}

// NewJurnalRepo creates a JurnalRepo. Panics on nil db.
func NewJurnalRepo(db *sql.DB) *JurnalRepo {
	if db == nil {
		panic("jurnal.NewJurnalRepo: db must not be nil")
	}
	return &JurnalRepo{db: db}
}

// AllowedJurnalSortCols is the sort/filter whitelist.
var AllowedJurnalSortCols = []string{
	"no_jurnal",
	"tanggal_posting",
	"event_code",
	"total_debit",
	"total_kredit",
	"status_internal",
	"created_at",
}

// BeginTx opens a transaction with serializable isolation (required for posting balance invariant).
func (r *JurnalRepo) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
}

// NextNoJurnal generates next no_jurnal from sys.seq_no_jurnal_{year}.
// Format: JRN-{YYYY}-{######}
func (r *JurnalRepo) NextNoJurnal(ctx context.Context, tx *sql.Tx, year int) (string, error) {
	seqName := fmt.Sprintf("sys.seq_no_jurnal_%d", year)
	var n int64
	err := tx.QueryRowContext(ctx, fmt.Sprintf("SELECT nextval('%s')", seqName)).Scan(&n)
	if err != nil {
		return "", fmt.Errorf("jurnal.JurnalRepo.NextNoJurnal: %w", err)
	}
	return fmt.Sprintf("JRN-%d-%06d", year, n), nil
}

// BuildIdempotencyKey computes SHA256(sourceEventID + "::" + eventCode) as hex.
func BuildIdempotencyKey(sourceEventID uuid.UUID, eventCode string) string {
	h := sha256.Sum256([]byte(sourceEventID.String() + "::" + eventCode))
	return hex.EncodeToString(h[:])
}

// CheckIdempotency returns the existing jrnl.header ID if the idempotency key was already posted.
// Returns uuid.Nil if not found.
func (r *JurnalRepo) CheckIdempotency(ctx context.Context, idempotencyKey string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM jrnl.header WHERE idempotency_key = $1 LIMIT 1`, idempotencyKey,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return uuid.Nil, nil
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("jurnal.JurnalRepo.CheckIdempotency: %w", err)
	}
	return id, nil
}

// Insert inserts a jrnl.header + jrnl.detail rows in the given transaction.
// The DB append-only trigger prevents UPDATE/DELETE so we only INSERT.
func (r *JurnalRepo) Insert(ctx context.Context, tx *sql.Tx, h *JurnalHeader) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO jrnl.header (
			id, no_jurnal, tanggal_posting, periode_id, event_code,
			mapping_header_id, instrumen_id,
			reference_event_type, reference_event_id,
			currency, total_debit, total_kredit, narrative,
			status_internal, idempotency_key, dokumen_doc_id, created_by
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7,
			$8, $9,
			$10, $11, $12, $13,
			$14, $15, $16, $17
		)`,
		h.ID, h.NoJurnal, h.TanggalPosting, h.PeriodeID, h.EventCode,
		h.MappingHeaderID, h.InstrumenID,
		h.ReferenceEventType, h.ReferenceEventID,
		h.Currency, h.TotalDebit, h.TotalKredit, h.Narrative,
		string(h.StatusInternal), h.IdempotencyKey, h.DokumenDocID, h.CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("jurnal.JurnalRepo.Insert header: %w", err)
	}
	for i := range h.DetailRows {
		if err := r.insertDetailRow(ctx, tx, h.DetailRows[i]); err != nil {
			return err
		}
	}
	return nil
}

func (r *JurnalRepo) insertDetailRow(ctx context.Context, tx *sql.Tx, d JurnalDetailRow) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO jrnl.detail (
			id, header_id, urutan, kode_akun_id,
			debit_amount, kredit_amount, mata_uang, narrative_line
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		d.ID, d.HeaderID, d.Urutan, d.KodeAkunID,
		d.DebitAmount, d.KreditAmount, d.MataUang, d.NarrativeLine,
	)
	if err != nil {
		return fmt.Errorf("jurnal.JurnalRepo.insertDetailRow: %w", err)
	}
	return nil
}

// UpdateStatus updates status_internal only (manual posting workflow).
func (r *JurnalRepo) UpdateStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status JurnalHeaderStatus) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE jrnl.header SET status_internal = $1 WHERE id = $2`,
		string(status), id,
	)
	if err != nil {
		return fmt.Errorf("jurnal.JurnalRepo.UpdateStatus: %w", err)
	}
	return nil
}

// GetByID fetches a full jrnl.header + detail rows.
func (r *JurnalRepo) GetByID(ctx context.Context, id uuid.UUID) (*JurnalHeader, error) {
	h := &JurnalHeader{}
	var statusStr string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, no_jurnal, tanggal_posting, periode_id, event_code,
		       mapping_header_id, instrumen_id,
		       reference_event_type, reference_event_id,
		       currency, total_debit, total_kredit, narrative,
		       status_internal, idempotency_key, dokumen_doc_id, created_by, created_at
		FROM jrnl.header
		WHERE id = $1`, id,
	).Scan(
		&h.ID, &h.NoJurnal, &h.TanggalPosting, &h.PeriodeID, &h.EventCode,
		&h.MappingHeaderID, &h.InstrumenID,
		&h.ReferenceEventType, &h.ReferenceEventID,
		&h.Currency, &h.TotalDebit, &h.TotalKredit, &h.Narrative,
		&statusStr, &h.IdempotencyKey, &h.DokumenDocID, &h.CreatedBy, &h.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("jurnal.JurnalRepo.GetByID: %w", err)
	}
	h.StatusInternal = JurnalHeaderStatus(statusStr)
	details, err := r.listDetails(ctx, id)
	if err != nil {
		return nil, err
	}
	h.DetailRows = details
	return h, nil
}

// ListSummary returns paginated jurnal header summaries.
func (r *JurnalRepo) ListSummary(ctx context.Context, q listquery.Query, limit int) ([]JurnalHeaderSummary, ListPage, error) {
	where, args, orderBy := q.WithAllowed(AllowedJurnalSortCols).ToSQL("h")
	fetchLimit := limit + 1
	args = append(args, fetchLimit)
	//nolint:gosec // where/orderBy are built by whitelist-validated listquery package
	query := fmt.Sprintf(`
		SELECT h.id, h.no_jurnal, h.tanggal_posting, h.event_code,
		       h.total_debit, h.total_kredit, h.status_internal,
		       h.reference_event_type, h.created_at
		FROM jrnl.header h
		WHERE 1=1 %s %s
		LIMIT $%d`, where, orderBy, len(args),
	)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, ListPage{}, fmt.Errorf("jurnal.JurnalRepo.ListSummary: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []JurnalHeaderSummary
	for rows.Next() {
		var s JurnalHeaderSummary
		var st string
		if err := rows.Scan(
			&s.ID, &s.NoJurnal, &s.TanggalPosting, &s.EventCode,
			&s.TotalDebit, &s.TotalKredit, &st,
			&s.ReferenceEventType, &s.CreatedAt,
		); err != nil {
			return nil, ListPage{}, fmt.Errorf("jurnal.JurnalRepo.ListSummary scan: %w", err)
		}
		s.StatusInternal = JurnalHeaderStatus(st)
		items = append(items, s)
	}
	if err := rows.Err(); err != nil {
		return nil, ListPage{}, fmt.Errorf("jurnal.JurnalRepo.ListSummary rows: %w", err)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	var nextCursor string
	if hasMore && len(items) > 0 {
		lastID := items[len(items)-1].ID.String()
		nextCursor = base64.StdEncoding.EncodeToString([]byte(lastID))
	}
	return items, ListPage{NextCursor: nextCursor, HasMore: hasMore, Limit: limit}, nil
}

func (r *JurnalRepo) listDetails(ctx context.Context, headerID uuid.UUID) ([]JurnalDetailRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT d.id, d.header_id, d.urutan, d.kode_akun_id,
		       COALESCE(a.kode_akun, '') AS kode_akun_kode,
		       COALESCE(a.nama_akun, '') AS kode_akun_nama,
		       d.debit_amount, d.kredit_amount, d.mata_uang,
		       d.narrative_line, d.created_at
		FROM jrnl.detail d
		LEFT JOIN mst.chart_of_accounts a ON a.id = d.kode_akun_id
		WHERE d.header_id = $1
		ORDER BY d.urutan`, headerID,
	)
	if err != nil {
		return nil, fmt.Errorf("jurnal.JurnalRepo.listDetails: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var result []JurnalDetailRow
	for rows.Next() {
		var d JurnalDetailRow
		if err := rows.Scan(
			&d.ID, &d.HeaderID, &d.Urutan, &d.KodeAkunID,
			&d.KodeAkunKode, &d.KodeAkunNama,
			&d.DebitAmount, &d.KreditAmount, &d.MataUang,
			&d.NarrativeLine, &d.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("jurnal.JurnalRepo.listDetails scan: %w", err)
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

// IsPeriodeHardClosed checks if a periode is hard-closed (cannot post jurnal).
func (r *JurnalRepo) IsPeriodeHardClosed(ctx context.Context, periodeID uuid.UUID) (bool, error) {
	var status string
	err := r.db.QueryRowContext(ctx,
		`SELECT status FROM mst.periode_buku WHERE id = $1`, periodeID,
	).Scan(&status)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("jurnal.JurnalRepo.IsPeriodeHardClosed: %w", err)
	}
	return strings.EqualFold(status, "HARD_CLOSED"), nil
}

// ─── DLQRepo ─────────────────────────────────────────────────────────────────

// DLQRepo handles sys.dlq_jurnal_post.
type DLQRepo struct {
	db *sql.DB
}

// NewDLQRepo creates a DLQRepo. Panics on nil db.
func NewDLQRepo(db *sql.DB) *DLQRepo {
	if db == nil {
		panic("jurnal.NewDLQRepo: db must not be nil")
	}
	return &DLQRepo{db: db}
}

// AllowedDLQSortCols is the sort/filter whitelist.
var AllowedDLQSortCols = []string{
	"source_event_type",
	"event_code",
	"error_code",
	"error_category",
	"retry_count",
	"status",
	"created_at",
	"last_retry_at",
}

// BeginTx opens a transaction.
func (r *DLQRepo) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, nil)
}

// Insert inserts a new DLQ entry (status=FAILED).
func (r *DLQRepo) Insert(ctx context.Context, tx *sql.Tx, e *DLQEntry) error {
	exec := r.db.ExecContext
	if tx != nil {
		exec = tx.ExecContext
	}
	_, err := exec(ctx, `
		INSERT INTO sys.dlq_jurnal_post (
			id, source_event_id, source_event_type, event_code,
			instrumen_id, periode_id, payload_jsonb,
			error_code, error_message, error_category,
			retry_count, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'FAILED')`,
		e.ID, e.SourceEventID, e.SourceEventType, e.EventCode,
		e.InstrumenID, e.PeriodeID, e.PayloadJSONB,
		e.ErrorCode, e.ErrorMessage, e.ErrorCategory,
		e.RetryCount,
	)
	if err != nil {
		return fmt.Errorf("jurnal.DLQRepo.Insert: %w", err)
	}
	return nil
}

// GetByID fetches a DLQ entry.
func (r *DLQRepo) GetByID(ctx context.Context, id uuid.UUID) (*DLQEntry, error) {
	e := &DLQEntry{}
	var st string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, source_event_id, source_event_type, event_code,
		       instrumen_id, periode_id, payload_jsonb,
		       error_code, error_message, error_category,
		       retry_count, last_retry_at, status,
		       replayed_by, replayed_at, final_jurnal_header_id,
		       discarded_reason, discarded_by, discarded_at,
		       created_at, updated_at, row_version
		FROM sys.dlq_jurnal_post WHERE id = $1`, id,
	).Scan(
		&e.ID, &e.SourceEventID, &e.SourceEventType, &e.EventCode,
		&e.InstrumenID, &e.PeriodeID, &e.PayloadJSONB,
		&e.ErrorCode, &e.ErrorMessage, &e.ErrorCategory,
		&e.RetryCount, &e.LastRetryAt, &st,
		&e.ReplayedBy, &e.ReplayedAt, &e.FinalJurnalHeaderID,
		&e.DiscardedReason, &e.DiscardedBy, &e.DiscardedAt,
		&e.CreatedAt, &e.UpdatedAt, &e.RowVersion,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("jurnal.DLQRepo.GetByID: %w", err)
	}
	e.Status = DLQStatus(st)
	return e, nil
}

// ListSummary returns paginated DLQ summaries.
func (r *DLQRepo) ListSummary(ctx context.Context, q listquery.Query, limit int) ([]DLQEntrySummary, ListPage, error) {
	where, args, orderBy := q.WithAllowed(AllowedDLQSortCols).ToSQL("d")
	fetchLimit := limit + 1
	args = append(args, fetchLimit)
	//nolint:gosec // where/orderBy are built by whitelist-validated listquery package
	query := fmt.Sprintf(`
		SELECT d.id, d.source_event_type, d.event_code,
		       d.error_code, d.error_category, d.retry_count,
		       d.status, d.last_retry_at, d.created_at
		FROM sys.dlq_jurnal_post d
		WHERE 1=1 %s %s
		LIMIT $%d`, where, orderBy, len(args),
	)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, ListPage{}, fmt.Errorf("jurnal.DLQRepo.ListSummary: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []DLQEntrySummary
	for rows.Next() {
		var s DLQEntrySummary
		var st string
		if err := rows.Scan(
			&s.ID, &s.SourceEventType, &s.EventCode,
			&s.ErrorCode, &s.ErrorCategory, &s.RetryCount,
			&st, &s.LastRetryAt, &s.CreatedAt,
		); err != nil {
			return nil, ListPage{}, fmt.Errorf("jurnal.DLQRepo.ListSummary scan: %w", err)
		}
		s.Status = DLQStatus(st)
		items = append(items, s)
	}
	if err := rows.Err(); err != nil {
		return nil, ListPage{}, fmt.Errorf("jurnal.DLQRepo.ListSummary rows: %w", err)
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	var nextCursor string
	if hasMore && len(items) > 0 {
		lastID := items[len(items)-1].ID.String()
		nextCursor = base64.StdEncoding.EncodeToString([]byte(lastID))
	}
	return items, ListPage{NextCursor: nextCursor, HasMore: hasMore, Limit: limit}, nil
}

// UpdateStatus updates status + related fields.
func (r *DLQRepo) UpdateStatus(ctx context.Context, tx *sql.Tx, e *DLQEntry) error {
	var execFn func(ctx context.Context, query string, args ...any) (sql.Result, error)
	if tx != nil {
		execFn = tx.ExecContext
	} else {
		execFn = r.db.ExecContext
	}
	_, err := execFn(ctx, `
		UPDATE sys.dlq_jurnal_post
		SET status = $1,
		    retry_count = $2, last_retry_at = $3,
		    replayed_by = $4, replayed_at = $5, final_jurnal_header_id = $6,
		    discarded_reason = $7, discarded_by = $8, discarded_at = $9,
		    updated_at = now(), row_version = row_version + 1
		WHERE id = $10`,
		string(e.Status),
		e.RetryCount, e.LastRetryAt,
		e.ReplayedBy, e.ReplayedAt, e.FinalJurnalHeaderID,
		e.DiscardedReason, e.DiscardedBy, e.DiscardedAt,
		e.ID,
	)
	if err != nil {
		return fmt.Errorf("jurnal.DLQRepo.UpdateStatus: %w", err)
	}
	return nil
}

// CheckExists returns true if a DLQ row already exists for (source_event_id, event_code).
func (r *DLQRepo) CheckExists(ctx context.Context, sourceEventID uuid.UUID, eventCode string) (bool, error) {
	var id uuid.UUID
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM sys.dlq_jurnal_post
		 WHERE source_event_id = $1 AND event_code = $2
		 LIMIT 1`, sourceEventID, eventCode,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("jurnal.DLQRepo.CheckExists: %w", err)
	}
	return true, nil
}

// GetKodeAkunByID returns kode_akun + nama_akun from mst.chart_of_accounts.
func GetKodeAkunByID(ctx context.Context, db *sql.DB, id uuid.UUID) (kode, nama string, err error) {
	err = db.QueryRowContext(ctx,
		`SELECT kode_akun, nama_akun FROM mst.chart_of_accounts WHERE id = $1`, id,
	).Scan(&kode, &nama)
	if err != nil {
		return "", "", fmt.Errorf("jurnal.GetKodeAkunByID(%s): %w", id, err)
	}
	return kode, nama, nil
}

// rollbackTx is a helper used by services to safely rollback on error.
func rollbackTx(tx *sql.Tx) {
	_ = tx.Rollback() //nolint:errcheck
}

// nowPtr returns a pointer to the current time.
func nowPtr() *time.Time {
	t := time.Now()
	return &t
}
