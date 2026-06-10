package instrumen

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

// Repository defines the data-access contract for mst.instrumen.
type Repository interface {
	// Create inserts a new instrumen row in the given transaction.
	Create(ctx context.Context, tx *sql.Tx, m *Instrumen) error

	// GetByID fetches one record by surrogate UUID. Returns (nil, nil) if not found.
	GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*Instrumen, error)

	// GetByKode fetches one record by kode_instrumen. Returns (nil, nil) if not found.
	GetByKode(ctx context.Context, kode string, includeDeleted bool) (*Instrumen, error)

	// List fetches paginated records (limit+1 trick for hasMore detection).
	List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*Instrumen, error)

	// Update applies a partial update in the given transaction with optimistic lock.
	Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, f UpdateFields) (*Instrumen, error)

	// SoftDelete sets deleted_at/deleted_by in the given transaction.
	SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*Instrumen, error)

	// UpdateWorkflowStatus updates workflow_status after workflow transitions.
	UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error

	// CountActiveTransactions returns live trx references (delete guard).
	CountActiveTransactions(ctx context.Context, id uuid.UUID) (int64, error)

	// CheckCounterpartyApproved returns true if counterparty exists + APPROVED.
	CheckCounterpartyApproved(ctx context.Context, counterpartyID uuid.UUID) (bool, error)

	// CheckPortofolioApproved returns approved flag + bm_category_default.
	CheckPortofolioApproved(ctx context.Context, portofolioID uuid.UUID) (approved bool, bmCategoryDefault *string, err error)

	// CheckMataUangApproved returns true if mata_uang exists + APPROVED.
	CheckMataUangApproved(ctx context.Context, kode string) (bool, error)

	// BeginTx starts a database transaction.
	BeginTx(ctx context.Context) (*sql.Tx, error)

	// ListAuditHistory returns paginated audit_log for a given entity UUID.
	ListAuditHistory(ctx context.Context, entityID uuid.UUID, cursor string, limit int, isAuditRole bool) ([]AuditHistoryItem, bool, error)

	// ExportAll returns all records matching q as UTF-8 BOM CSV stream.
	ExportAll(ctx context.Context, q listquery.Query) (io.Reader, int, error)
}

// UpdateFields captures mutable columns for an update operation.
// Nil pointer = do not update that column.
type UpdateFields struct {
	SubTipe               *string
	Nama                  *string
	ISIN                  *string
	ManajerInvestasiID    *uuid.UUID
	BankKustodianID       *uuid.UUID
	MataUang              *string
	Kupon                 *decimal.Decimal
	FrekuensiBunga        *string
	AutoRenewalFlag       *bool
	FvociElection         *bool
	BmCategory            *string
	EirAwal               *decimal.Decimal
	DayCountConvention    *string
	AmortizationFrequency *string
	Status                *string
	UpdatedBy             uuid.UUID
	ExpectedVersion       int64
}

// ─── Sentinel errors ──────────────────────────────────────────────────────────

var (
	ErrNotFound      = fmt.Errorf("instrumen not found")
	ErrConflict      = fmt.Errorf("instrumen optimistic lock conflict")
	ErrKodeDuplicate = fmt.Errorf("instrumen kode duplicate")
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

// selectCols is the ordered column list matching scanRow.
const selectCols = `
    t.id, t.kode_instrumen, t.tipe_instrumen, t.sub_tipe, t.nama, t.isin,
    t.counterparty_id, t.manajer_investasi_id, t.bank_kustodian_id,
    t.mata_uang, t.portofolio_id,
    t.nominal, t.jumlah_lot, t.tanggal_penempatan::text, t.tanggal_jatuh_tempo::text,
    t.kupon::text, t.frekuensi_bunga, t.auto_renewal_flag, t.fvoci_election,
    t.sppi_result, t.bm_category, t.klasifikasi_psak71,
    t.klasifikasi_locked_at, t.klasifikasi_locked_by, t.sppi_bm_last_review_date::text,
    t.eir_awal::text, t.tanggal_eir_computed::text,
    t.premium_diskonto_awal::text, t.biaya_transaksi_capitalized::text,
    t.eir_method_flag, t.day_count_convention, t.amortization_frequency,
    t.status, t.workflow_status,
    t.created_at, t.created_by, t.updated_at, t.updated_by,
    t.deleted_at, t.deleted_by, t.row_version, t.tenant_id,
    t.version, t.is_deleted`

// Create inserts a new row inside a transaction.
func (r *DBRepository) Create(ctx context.Context, tx *sql.Tx, m *Instrumen) error {
	const query = `
INSERT INTO mst.instrumen (
    id, kode_instrumen, tipe_instrumen, sub_tipe, nama, isin,
    counterparty_id, manajer_investasi_id, bank_kustodian_id,
    mata_uang, portofolio_id,
    nominal, jumlah_lot, tanggal_penempatan, tanggal_jatuh_tempo,
    kupon, frekuensi_bunga, auto_renewal_flag, fvoci_election,
    bm_category, eir_awal, tanggal_eir_computed,
    premium_diskonto_awal, biaya_transaksi_capitalized,
    eir_method_flag, day_count_convention, amortization_frequency,
    status, workflow_status,
    created_at, created_by, updated_at, updated_by,
    row_version, tenant_id, version, is_deleted
) VALUES (
    $1,  $2,  $3,  $4,  $5,  $6,
    $7,  $8,  $9,
    $10, $11,
    $12, $13, $14, $15,
    $16, $17, $18, $19,
    $20, $21, $22,
    $23, $24,
    $25, $26, $27,
    $28, $29,
    $30, $31, $30, $31,
    1, $32, 1, FALSE
)`
	_, err := tx.ExecContext(ctx, query,
		m.ID, m.KodeInstrumen, m.TipeInstrumen, m.SubTipe, m.Nama, m.ISIN, // $1-$6
		m.CounterpartyID, m.ManajerInvestasiID, m.BankKustodianID, // $7-$9
		m.MataUang, m.PortofolioID, // $10-$11
		m.Nominal.String(), nullDecimal(m.JumlahLot), // $12-$13
		m.TanggalPenempatan, m.TanggalJatuhTempo, // $14-$15
		nullDecimal(m.Kupon), m.FrekuensiBunga, m.AutoRenewalFlag, m.FvociElection, // $16-$19
		m.BmCategory, nullDecimal(m.EirAwal), m.TanggalEirComputed, // $20-$22
		m.PremiumDiskonto.String(), m.BiayaTransaksi.String(), // $23-$24
		m.EirMethodFlag, m.DayCountConvention, m.AmortizationFrequency, // $25-$27
		m.Status, string(m.WorkflowStatus), // $28-$29
		m.CreatedAt, m.CreatedBy, // $30-$31
		m.TenantID, // $32
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("kode_instrumen: %w", ErrKodeDuplicate)
		}
		return fmt.Errorf("repo.Create instrumen: %w", err)
	}
	return nil
}

// GetByID fetches by UUID using the db connection (outside tx).
func (r *DBRepository) GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*Instrumen, error) {
	deletedFilter := " AND t.deleted_at IS NULL AND t.is_deleted = FALSE"
	if includeDeleted {
		deletedFilter = ""
	}
	query := fmt.Sprintf("SELECT %s FROM mst.instrumen t WHERE t.id = $1%s", selectCols, deletedFilter) //nolint:gosec
	row := r.db.QueryRowContext(ctx, query, id)
	return scanRow(row)
}

// GetByKode fetches by kode_instrumen.
func (r *DBRepository) GetByKode(ctx context.Context, kode string, includeDeleted bool) (*Instrumen, error) {
	deletedFilter := " AND t.deleted_at IS NULL AND t.is_deleted = FALSE"
	if includeDeleted {
		deletedFilter = ""
	}
	query := fmt.Sprintf("SELECT %s FROM mst.instrumen t WHERE t.kode_instrumen = $1%s", selectCols, deletedFilter) //nolint:gosec
	row := r.db.QueryRowContext(ctx, query, kode)
	return scanRow(row)
}

// List fetches paginated records with cursor/filter/sort support.
func (r *DBRepository) List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*Instrumen, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("t")

	conditions := []string{}
	if !includeDeleted {
		conditions = append(conditions, "t.deleted_at IS NULL", "t.is_deleted = FALSE")
	}

	if cursor != "" {
		cd, err := pagination.DecodeCursor(cursor)
		if err == nil && cd.ID != "" {
			argIdx := len(args) + 1
			conditions = append(conditions, fmt.Sprintf("t.id::text > $%d", argIdx))
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
		orderBy = "t.created_at DESC"
	}

	argIdx := len(args) + 1
	query := fmt.Sprintf( //nolint:gosec
		"SELECT %s FROM mst.instrumen t%s ORDER BY %s LIMIT $%d",
		selectCols, whereClause, orderBy, argIdx,
	)
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.List instrumen: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []*Instrumen
	for rows.Next() {
		m, err := scanRows(rows)
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
func (r *DBRepository) Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, f UpdateFields) (*Instrumen, error) {
	setClauses := []string{}
	args := []interface{}{}
	idx := 1

	if f.SubTipe != nil {
		setClauses = append(setClauses, fmt.Sprintf("sub_tipe = $%d", idx))
		args = append(args, *f.SubTipe)
		idx++
	}
	if f.Nama != nil {
		setClauses = append(setClauses, fmt.Sprintf("nama = $%d", idx))
		args = append(args, *f.Nama)
		idx++
	}
	if f.ISIN != nil {
		setClauses = append(setClauses, fmt.Sprintf("isin = $%d", idx))
		args = append(args, *f.ISIN)
		idx++
	}
	if f.ManajerInvestasiID != nil {
		setClauses = append(setClauses, fmt.Sprintf("manajer_investasi_id = $%d", idx))
		args = append(args, *f.ManajerInvestasiID)
		idx++
	}
	if f.BankKustodianID != nil {
		setClauses = append(setClauses, fmt.Sprintf("bank_kustodian_id = $%d", idx))
		args = append(args, *f.BankKustodianID)
		idx++
	}
	if f.MataUang != nil {
		setClauses = append(setClauses, fmt.Sprintf("mata_uang = $%d", idx))
		args = append(args, *f.MataUang)
		idx++
	}
	if f.Kupon != nil {
		setClauses = append(setClauses, fmt.Sprintf("kupon = $%d", idx))
		args = append(args, f.Kupon.String())
		idx++
	}
	if f.FrekuensiBunga != nil {
		setClauses = append(setClauses, fmt.Sprintf("frekuensi_bunga = $%d", idx))
		args = append(args, *f.FrekuensiBunga)
		idx++
	}
	if f.AutoRenewalFlag != nil {
		setClauses = append(setClauses, fmt.Sprintf("auto_renewal_flag = $%d", idx))
		args = append(args, *f.AutoRenewalFlag)
		idx++
	}
	if f.FvociElection != nil {
		setClauses = append(setClauses, fmt.Sprintf("fvoci_election = $%d", idx))
		args = append(args, *f.FvociElection)
		idx++
	}
	if f.BmCategory != nil {
		setClauses = append(setClauses, fmt.Sprintf("bm_category = $%d", idx))
		args = append(args, *f.BmCategory)
		idx++
	}
	if f.EirAwal != nil {
		setClauses = append(setClauses, fmt.Sprintf("eir_awal = $%d", idx))
		args = append(args, f.EirAwal.String())
		idx++
	}
	if f.DayCountConvention != nil {
		setClauses = append(setClauses, fmt.Sprintf("day_count_convention = $%d", idx))
		args = append(args, *f.DayCountConvention)
		idx++
	}
	if f.AmortizationFrequency != nil {
		setClauses = append(setClauses, fmt.Sprintf("amortization_frequency = $%d", idx))
		args = append(args, *f.AmortizationFrequency)
		idx++
	}
	if f.Status != nil {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", idx))
		args = append(args, *f.Status)
		idx++
	}

	if len(setClauses) == 0 {
		return r.GetByID(ctx, id, false)
	}

	setClauses = append(setClauses,
		fmt.Sprintf("updated_at = $%d", idx),
		fmt.Sprintf("updated_by = $%d", idx+1),
		"row_version = row_version + 1",
		"version = version + 1",
	)
	args = append(args, time.Now(), f.UpdatedBy)
	idx += 2

	whereIDIdx := idx
	whereVersionIdx := idx + 1
	args = append(args, id, f.ExpectedVersion)

	query := fmt.Sprintf( //nolint:gosec
		`UPDATE mst.instrumen SET %s WHERE id = $%d AND row_version = $%d AND deleted_at IS NULL`,
		strings.Join(setClauses, ", "),
		whereIDIdx, whereVersionIdx,
	)

	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.Update instrumen: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("repo.Update rows affected: %w", err)
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
	return r.getByIDInTx(ctx, tx, id)
}

// SoftDelete marks the record as deleted.
func (r *DBRepository) SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*Instrumen, error) {
	now := time.Now()
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.instrumen
		SET deleted_at = $1, deleted_by = $2, updated_at = $1, updated_by = $2, is_deleted = TRUE
		WHERE id = $3 AND deleted_at IS NULL
	`, now, deletedBy, id)
	if err != nil {
		return nil, fmt.Errorf("repo.SoftDelete instrumen: %w", err)
	}
	return r.getByIDInTx(ctx, tx, id)
}

// UpdateWorkflowStatus updates only workflow_status.
func (r *DBRepository) UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.instrumen
		SET workflow_status = $1, updated_at = now(), updated_by = $2, row_version = row_version + 1
		WHERE id = $3
	`, string(status), updatedBy, id)
	if err != nil {
		return fmt.Errorf("repo.UpdateWorkflowStatus instrumen: %w", err)
	}
	return nil
}

// CountActiveTransactions counts live trx references.
func (r *DBRepository) CountActiveTransactions(ctx context.Context, id uuid.UUID) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM trx.transaction
		WHERE instrumen_id = $1 AND deleted_at IS NULL
	`, id).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("repo.CountActiveTransactions instrumen: %w", err)
	}
	return count, nil
}

// CheckCounterpartyApproved checks existence and APPROVED state of a counterparty.
func (r *DBRepository) CheckCounterpartyApproved(ctx context.Context, counterpartyID uuid.UUID) (bool, error) {
	var status string
	err := r.db.QueryRowContext(ctx, `
		SELECT workflow_status FROM mst.counterparty
		WHERE id = $1 AND deleted_at IS NULL
	`, counterpartyID).Scan(&status)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("repo.CheckCounterpartyApproved: %w", err)
	}
	return status == "APPROVED", nil
}

// CheckPortofolioApproved checks existence, APPROVED state, and returns bm_category_default.
func (r *DBRepository) CheckPortofolioApproved(ctx context.Context, portofolioID uuid.UUID) (bool, *string, error) {
	var status string
	var bmDefault *string
	err := r.db.QueryRowContext(ctx, `
		SELECT workflow_status, bm_category_default FROM mst.portofolio
		WHERE id = $1 AND deleted_at IS NULL
	`, portofolioID).Scan(&status, &bmDefault)
	if err == sql.ErrNoRows {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, fmt.Errorf("repo.CheckPortofolioApproved: %w", err)
	}
	return status == "APPROVED", bmDefault, nil
}

// CheckMataUangApproved checks existence and APPROVED state of a mata_uang.
func (r *DBRepository) CheckMataUangApproved(ctx context.Context, kode string) (bool, error) {
	var status string
	err := r.db.QueryRowContext(ctx, `
		SELECT workflow_status FROM mst.mata_uang
		WHERE kode_mata_uang = $1 AND deleted_at IS NULL
	`, kode).Scan(&status)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("repo.CheckMataUangApproved: %w", err)
	}
	return status == "APPROVED", nil
}

// BeginTx starts a ReadCommitted transaction.
func (r *DBRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("repo.BeginTx instrumen: %w", err)
	}
	return tx, nil
}

// ListAuditHistory returns paginated audit log rows.
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
	args = append(args, entityID, "mst.instrumen")
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
		return nil, false, fmt.Errorf("repo.ListAuditHistory instrumen: %w", err)
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

// ExportAll streams records as UTF-8 BOM CSV.
func (r *DBRepository) ExportAll(ctx context.Context, q listquery.Query) (io.Reader, int, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("t")

	conditions := []string{"t.deleted_at IS NULL", "t.is_deleted = FALSE"}
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
	whereClause := " WHERE " + strings.Join(conditions, " AND ")
	if orderBy == "" {
		orderBy = "t.created_at DESC"
	}

	query := fmt.Sprintf( //nolint:gosec
		`SELECT t.kode_instrumen, t.tipe_instrumen, t.nama, t.mata_uang,
		    t.nominal::text, t.tanggal_penempatan::text, t.tanggal_jatuh_tempo::text,
		    t.klasifikasi_psak71, t.sppi_result, t.bm_category,
		    t.status, t.workflow_status
		FROM mst.instrumen t%s ORDER BY %s`,
		whereClause, orderBy,
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll instrumen: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(&buf)

	headers := []string{
		"Kode Instrumen", "Tipe", "Nama", "Mata Uang",
		"Nominal", "Tgl Penempatan", "Tgl Jatuh Tempo",
		"Klasifikasi PSAK71", "SPPI Result", "BM Category",
		"Status", "Workflow Status",
	}
	if err := w.Write(headers); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll write header: %w", err)
	}

	count := 0
	for rows.Next() {
		var (
			kode, tipe, nama, mataUang string
			nominal, tglPenempatan     string
			tglJatuhTempo              *string
			klasifikasi, sppiResult    *string
			bmCategory                 *string
			status, wfStatus           string
		)
		if err := rows.Scan(&kode, &tipe, &nama, &mataUang,
			&nominal, &tglPenempatan, &tglJatuhTempo,
			&klasifikasi, &sppiResult, &bmCategory,
			&status, &wfStatus); err != nil {
			return nil, 0, fmt.Errorf("repo.ExportAll scan: %w", err)
		}
		record := []string{
			kode, tipe, nama, mataUang,
			nominal, tglPenempatan, strOrEmpty(tglJatuhTempo),
			strOrEmpty(klasifikasi), strOrEmpty(sppiResult), strOrEmpty(bmCategory),
			status, wfStatus,
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

// scanRow scans a *sql.Row into *Instrumen with decimal parsing.
func scanRow(row *sql.Row) (*Instrumen, error) {
	m, err := scan(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.scanRow instrumen: %w", err)
	}
	return m, nil
}

// scanRows scans one *sql.Rows row into *Instrumen.
func scanRows(rows *sql.Rows) (*Instrumen, error) {
	m, err := scan(rows.Scan)
	if err != nil {
		return nil, fmt.Errorf("repo.scanRows instrumen: %w", err)
	}
	return m, nil
}

// scan is the shared scan logic, accepting a Scan function to work with both Row and Rows.
func scan(scanFn func(dest ...interface{}) error) (*Instrumen, error) {
	m := &Instrumen{}
	var (
		nominalStr   string
		jumlahLotStr *string
		kuponStr     *string
		eirAwalStr   *string
		premiumStr   string
		biayaStr     string
		wfStatus     string
		createdBy    uuid.UUID
		updatedAt    *time.Time
		updatedBy    *uuid.UUID
		deletedAt    *time.Time
		deletedBy    *uuid.UUID
	)

	err := scanFn(
		&m.ID, &m.KodeInstrumen, &m.TipeInstrumen, &m.SubTipe, &m.Nama, &m.ISIN,
		&m.CounterpartyID, &m.ManajerInvestasiID, &m.BankKustodianID,
		&m.MataUang, &m.PortofolioID,
		&nominalStr, &jumlahLotStr, &m.TanggalPenempatan, &m.TanggalJatuhTempo,
		&kuponStr, &m.FrekuensiBunga, &m.AutoRenewalFlag, &m.FvociElection,
		&m.SppiResult, &m.BmCategory, &m.KlasifikasiPsak71,
		&m.KlasifikasiLockedAt, &m.KlasifikasiLockedBy, &m.SppiBmLastReviewDate,
		&eirAwalStr, &m.TanggalEirComputed,
		&premiumStr, &biayaStr,
		&m.EirMethodFlag, &m.DayCountConvention, &m.AmortizationFrequency,
		&m.Status, &wfStatus,
		&m.CreatedAt, &createdBy, &updatedAt, &updatedBy,
		&deletedAt, &deletedBy, &m.RowVersion, &m.TenantID,
		&m.Version, &m.IsDeleted,
	)
	if err != nil {
		return nil, err
	}

	m.WorkflowStatus = WorkflowStatus(wfStatus)
	m.CreatedBy = createdBy
	m.UpdatedAt = updatedAt
	m.UpdatedBy = updatedBy
	m.DeletedAt = deletedAt
	m.DeletedBy = deletedBy

	// Parse decimal strings (NUMERIC columns cast to text in SELECT).
	if d, err2 := decimal.NewFromString(nominalStr); err2 == nil {
		m.Nominal = d
	}
	if jumlahLotStr != nil {
		if d, err2 := decimal.NewFromString(*jumlahLotStr); err2 == nil {
			m.JumlahLot = &d
		}
	}
	if kuponStr != nil {
		if d, err2 := decimal.NewFromString(*kuponStr); err2 == nil {
			m.Kupon = &d
		}
	}
	if eirAwalStr != nil {
		if d, err2 := decimal.NewFromString(*eirAwalStr); err2 == nil {
			m.EirAwal = &d
		}
	}
	if d, err2 := decimal.NewFromString(premiumStr); err2 == nil {
		m.PremiumDiskonto = d
	}
	if d, err2 := decimal.NewFromString(biayaStr); err2 == nil {
		m.BiayaTransaksi = d
	}
	return m, nil
}

// getByIDInTx fetches a single record within a transaction.
func (r *DBRepository) getByIDInTx(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*Instrumen, error) {
	query := fmt.Sprintf("SELECT %s FROM mst.instrumen t WHERE t.id = $1", selectCols) //nolint:gosec
	row := tx.QueryRowContext(ctx, query, id)
	m, err := scan(row.Scan)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repo.getByIDInTx instrumen: %w", err)
	}
	return m, nil
}

// ─── Utility ─────────────────────────────────────────────────────────────────

// nullDecimal converts a *decimal.Decimal to a *string for nullable DB columns.
func nullDecimal(d *decimal.Decimal) *string {
	if d == nil {
		return nil
	}
	s := d.String()
	return &s
}

// strOrEmpty dereferences a *string, returning "" for nil.
func strOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// isUniqueViolation checks for PostgreSQL unique constraint violation (23505).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "23505") ||
		strings.Contains(err.Error(), "duplicate key") ||
		strings.Contains(err.Error(), "unique constraint")
}
