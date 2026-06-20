package renewal

// repo.go — sqlx repository for trx.renewal.
// No business logic. Only SQL + mapping.
// Service owns transactions; repo never opens tx.

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// Repository defines the data access interface for trx.renewal.
type Repository interface {
	// GetByID fetches a renewal by UUID. Returns (nil, nil) if not found.
	GetByID(ctx context.Context, id uuid.UUID) (*Renewal, error)

	// GetInstrumenInfo fetches minimal instrumen data for eligibility checks.
	GetInstrumenInfo(ctx context.Context, id uuid.UUID) (*InstrumenInfo, error)

	// HasActiveRenewal returns true if instrumen_lama_id has a non-REJECTED active renewal.
	HasActiveRenewal(ctx context.Context, instrumenLamaID uuid.UUID) (bool, error)

	// Insert inserts a new renewal row inside the given tx.
	Insert(ctx context.Context, tx *sql.Tx, r *Renewal) error

	// UpdateStatus updates workflow status fields inside the given tx.
	UpdateStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, u StatusUpdate) error

	// List returns paginated renewal rows.
	List(ctx context.Context, q listquery.Query, cursor string, limit int) ([]*Renewal, bool, int, error)

	// GetPeriodeByTanggal looks up mst.periode_buku. Returns nil if not found.
	GetPeriodeByTanggal(ctx context.Context, tanggal time.Time) (*PeriodeBuku, error)

	// BeginTx starts a new DB transaction.
	BeginTx(ctx context.Context) (*sql.Tx, error)
}

// PeriodeBuku holds minimal period data used by renewal service.
type PeriodeBuku struct {
	ID            uuid.UUID `db:"id"`
	StatusPeriode string    `db:"status_periode"` // OPEN | CLOSED
	TanggalMulai  time.Time `db:"tanggal_mulai"`
	TanggalAkhir  time.Time `db:"tanggal_akhir"`
}

// Repo is the concrete database/sql implementation of Repository.
type Repo struct {
	db *sql.DB
}

// NewRepo creates a new Repo.
func NewRepo(db *sql.DB) *Repo {
	return &Repo{db: db}
}

// BeginTx starts a database transaction.
func (r *Repo) BeginTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("repo.BeginTx: %w", err)
	}
	return tx, nil
}

// GetByID fetches one renewal by UUID.
// Scans NUMERIC columns as text strings and parses them via decimal.NewFromString.
func (r *Repo) GetByID(ctx context.Context, id uuid.UUID) (*Renewal, error) {
	const q = `
		SELECT id, instrumen_lama_id, instrumen_baru_id, skema, tenor_baru_bulan,
		       rate_baru_persen::text, tanggal_efektif_baru, tanggal_jatuh_tempo_baru,
		       pokok_lama::text, pokok_baru::text, bunga_kotor::text,
		       pph_amount::text, bunga_bersih::text,
		       COALESCE(eir_baru::text, '') AS eir_baru_str,
		       status, maker_id, approver_id,
		       request_reason, approve_reason, reject_reason, signature_method,
		       approved_at, jurnal_header_id, periode_bulanan_id,
		       created_at, created_by, updated_at, updated_by,
		       deleted_at, deleted_by, row_version, tenant_id
		FROM trx.renewal
		WHERE id = $1 AND deleted_at IS NULL
		LIMIT 1`

	var (
		r2                  Renewal
		rateStr             string
		pokokLamaStr        string
		pokokBaruStr        string
		bungaKotorStr       string
		pphAmountStr        string
		bungaBersihStr      string
		eirBaruStr          string
	)

	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&r2.ID, &r2.InstrumenLamaID, &r2.InstrumenBaruID, &r2.Skema, &r2.TenorBaruBulan,
		&rateStr, &r2.TanggalEfektifBaru, &r2.TanggalJatuhTempoBaru,
		&pokokLamaStr, &pokokBaruStr, &bungaKotorStr,
		&pphAmountStr, &bungaBersihStr, &eirBaruStr,
		&r2.Status, &r2.MakerID, &r2.ApproverID,
		&r2.RequestReason, &r2.ApproveReason, &r2.RejectReason, &r2.SignatureMethod,
		&r2.ApprovedAt, &r2.JurnalHeaderID, &r2.PeriodeBulananID,
		&r2.CreatedAt, &r2.CreatedBy, &r2.UpdatedAt, &r2.UpdatedBy,
		&r2.DeletedAt, &r2.DeletedBy, &r2.RowVersion, &r2.TenantID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.GetByID: %w", err)
	}

	if v, err2 := decimal.NewFromString(rateStr); err2 == nil {
		r2.RateBaruPersen = v
	}
	if v, err2 := decimal.NewFromString(pokokLamaStr); err2 == nil {
		r2.PokokLama = v
	}
	if v, err2 := decimal.NewFromString(pokokBaruStr); err2 == nil {
		r2.PokokBaru = v
	}
	if v, err2 := decimal.NewFromString(bungaKotorStr); err2 == nil {
		r2.BungaKotor = v
	}
	if v, err2 := decimal.NewFromString(pphAmountStr); err2 == nil {
		r2.PphAmount = v
	}
	if v, err2 := decimal.NewFromString(bungaBersihStr); err2 == nil {
		r2.BungaBersih = v
	}
	if eirBaruStr != "" {
		if v, err2 := decimal.NewFromString(eirBaruStr); err2 == nil {
			r2.EirBaru = &v
		}
	}
	return &r2, nil
}

// GetInstrumenInfo fetches minimal instrumen data from mst.instrumen.
func (r *Repo) GetInstrumenInfo(ctx context.Context, id uuid.UUID) (*InstrumenInfo, error) {
	const q = `
		SELECT id, kode_instrumen, nama_instrumen,
		       COALESCE(jenis_instrumen, '') AS jenis_instrumen,
		       COALESCE(status, '') AS status,
		       COALESCE(klasifikasi_psak71, '') AS klasifikasi_psak71,
		       klasifikasi_locked,
		       COALESCE(pokok::text, '0') AS pokok_str,
		       COALESCE(rate_persen::text, '0') AS rate_persen_str,
		       tanggal_penempatan, tanggal_jatuh_tempo,
		       COALESCE(mata_uang, 'IDR') AS mata_uang,
		       counterparty_id, portofolio_id,
		       sppi_test_run_id, bm_assessment_id, renewal_dari_instrumen_id
		FROM mst.instrumen
		WHERE id = $1 AND deleted_at IS NULL
		LIMIT 1`

	type row struct {
		ID                     uuid.UUID  `db:"id"`
		KodeInstrumen          string     `db:"kode_instrumen"`
		NamaInstrumen          string     `db:"nama_instrumen"`
		JenisInstrumen         string     `db:"jenis_instrumen"`
		Status                 string     `db:"status"`
		KlasifikasiPSAK71      string     `db:"klasifikasi_psak71"`
		KlasifikasiLocked      bool       `db:"klasifikasi_locked"`
		PokokStr               string     `db:"pokok_str"`
		RatePersenStr          string     `db:"rate_persen_str"`
		TanggalPenempatan      time.Time  `db:"tanggal_penempatan"`
		TanggalJatuhTempo      time.Time  `db:"tanggal_jatuh_tempo"`
		MataUang               string     `db:"mata_uang"`
		CounterpartyID         uuid.UUID  `db:"counterparty_id"`
		PortofolioID           uuid.UUID  `db:"portofolio_id"`
		SppiTestRunID          *uuid.UUID `db:"sppi_test_run_id"`
		BmAssessmentID         *uuid.UUID `db:"bm_assessment_id"`
		RenewalDariInstrumenID *uuid.UUID `db:"renewal_dari_instrumen_id"`
	}

	sqlRow := r.db.QueryRowContext(ctx, q, id)
	var dbRow row
	var pokokStr, ratePersenStr string
	err := sqlRow.Scan(
		&dbRow.ID, &dbRow.KodeInstrumen, &dbRow.NamaInstrumen,
		&dbRow.JenisInstrumen, &dbRow.Status, &dbRow.KlasifikasiPSAK71,
		&dbRow.KlasifikasiLocked, &pokokStr, &ratePersenStr,
		&dbRow.TanggalPenempatan, &dbRow.TanggalJatuhTempo, &dbRow.MataUang,
		&dbRow.CounterpartyID, &dbRow.PortofolioID,
		&dbRow.SppiTestRunID, &dbRow.BmAssessmentID, &dbRow.RenewalDariInstrumenID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.GetInstrumenInfo: %w", err)
	}

	pokok, err := decimal.NewFromString(pokokStr)
	if err != nil {
		return nil, fmt.Errorf("repo.GetInstrumenInfo: parse pokok '%s': %w", pokokStr, err)
	}
	ratePersen, err := decimal.NewFromString(ratePersenStr)
	if err != nil {
		return nil, fmt.Errorf("repo.GetInstrumenInfo: parse rate_persen '%s': %w", ratePersenStr, err)
	}

	return &InstrumenInfo{
		ID:                     dbRow.ID,
		KodeInstrumen:          dbRow.KodeInstrumen,
		NamaInstrumen:          dbRow.NamaInstrumen,
		JenisInstrumen:         dbRow.JenisInstrumen,
		Status:                 dbRow.Status,
		KlasifikasiPSAK71:      dbRow.KlasifikasiPSAK71,
		KlasifikasiLocked:      dbRow.KlasifikasiLocked,
		Pokok:                  pokok,
		RatePersen:             ratePersen,
		TanggalPenempatan:      dbRow.TanggalPenempatan,
		TanggalJatuhTempo:      dbRow.TanggalJatuhTempo,
		MataUang:               dbRow.MataUang,
		CounterpartyID:         dbRow.CounterpartyID,
		PortofolioID:           dbRow.PortofolioID,
		SppiTestRunID:          dbRow.SppiTestRunID,
		BmAssessmentID:         dbRow.BmAssessmentID,
		RenewalDariInstrumenID: dbRow.RenewalDariInstrumenID,
	}, nil
}

// HasActiveRenewal checks for existing non-REJECTED renewal on instrumen_lama_id.
func (r *Repo) HasActiveRenewal(ctx context.Context, instrumenLamaID uuid.UUID) (bool, error) {
	const q = `
		SELECT COUNT(*)
		FROM trx.renewal
		WHERE instrumen_lama_id = $1
		  AND status IN ('PENDING_APPROVAL', 'APPROVED', 'POSTED')
		  AND deleted_at IS NULL`

	var count int
	if err := r.db.QueryRowContext(ctx, q, instrumenLamaID).Scan(&count); err != nil {
		return false, fmt.Errorf("repo.HasActiveRenewal: %w", err)
	}
	return count > 0, nil
}

// Insert inserts a new renewal row using sqlx NamedExecContext.
func (r *Repo) Insert(ctx context.Context, tx *sql.Tx, row *Renewal) error {
	const q = `
		INSERT INTO trx.renewal (
			id, instrumen_lama_id, instrumen_baru_id, skema, tenor_baru_bulan,
			rate_baru_persen, tanggal_efektif_baru, tanggal_jatuh_tempo_baru,
			pokok_lama, pokok_baru, bunga_kotor, pph_amount, bunga_bersih,
			eir_baru, schedule_baru_jsonb, status, maker_id, approver_id,
			request_reason, approve_reason, reject_reason, signature_method,
			signature_hash_meta, approved_at, jurnal_header_id, periode_bulanan_id,
			created_at, created_by, updated_at, updated_by, row_version, tenant_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8,
			$9, $10, $11, $12, $13,
			$14, $15, $16, $17, $18,
			$19, $20, $21, $22, $23, $24, $25, $26,
			$27, $28, $29, $30, $31, $32
		)`

	_, err := tx.ExecContext(ctx, q,
		row.ID, row.InstrumenLamaID, row.InstrumenBaruID, string(row.Skema), row.TenorBaruBulan,
		row.RateBaruPersen.StringFixed(4), row.TanggalEfektifBaru, row.TanggalJatuhTempoBaru,
		row.PokokLama.StringFixed(4), row.PokokBaru.StringFixed(4),
		row.BungaKotor.StringFixed(4), row.PphAmount.StringFixed(4), row.BungaBersih.StringFixed(4),
		decimalPtrToStr(row.EirBaru, 8), row.ScheduleBaruJSONB, string(row.Status), row.MakerID, row.ApproverID,
		row.RequestReason, row.ApproveReason, row.RejectReason, row.SignatureMethod,
		row.SignatureHashMeta, row.ApprovedAt, row.JurnalHeaderID, row.PeriodeBulananID,
		row.CreatedAt, row.CreatedBy, row.UpdatedAt, row.UpdatedBy, row.RowVersion, row.TenantID,
	)
	if err != nil {
		return fmt.Errorf("repo.Insert: %w", err)
	}
	return nil
}

// UpdateStatus updates workflow status and optional related fields with optimistic lock.
func (r *Repo) UpdateStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, u StatusUpdate) error {
	const q = `
		UPDATE trx.renewal SET
			status             = $1,
			approver_id        = $2,
			instrumen_baru_id  = $3,
			approve_reason     = $4,
			reject_reason      = $5,
			signature_method   = $6,
			signature_hash_meta = $7,
			approved_at        = $8,
			jurnal_header_id   = $9,
			eir_baru           = $10::NUMERIC,
			updated_by         = $11,
			updated_at         = NOW()
		WHERE id = $12
		  AND row_version = $13
		  AND deleted_at IS NULL`

	res, err := tx.ExecContext(ctx, q,
		string(u.Status),
		u.ApproverID,
		u.InstrumenBaruID,
		u.ApproveReason,
		u.RejectReason,
		u.SignatureMethod,
		u.SignatureHashMeta,
		u.ApprovedAt,
		u.JurnalHeaderID,
		decimalPtrToStr(u.EirBaru, 8),
		u.UpdatedBy,
		id,
		u.RowVersion,
	)
	if err != nil {
		return fmt.Errorf("repo.UpdateStatus: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repo.UpdateStatus: rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("repo.UpdateStatus: optimistic lock conflict for renewal %s (row_version %d)", id, u.RowVersion)
	}
	return nil
}

// List returns paginated renewal rows filtered by tenant_id (DEC-023 placeholder).
//
// Cursor format: base64(created_at_rfc3339nano + "|" + id_uuid).
// Returns (rows, hasMore, totalOnPage, error).
func (r *Repo) List(ctx context.Context, q listquery.Query, cursor string, limit int) ([]*Renewal, bool, int, error) {
	// Decode cursor to obtain the (created_at, id) boundary for keyset pagination.
	var cursorTime time.Time
	var cursorID uuid.UUID
	if cursor != "" {
		ct, cid, err := decodeCursor(cursor)
		if err != nil {
			return nil, false, 0, fmt.Errorf("repo.List: invalid cursor: %w", err)
		}
		cursorTime, cursorID = ct, cid
	}

	// Build query. Cursor clause applied only when cursor is set.
	const baseQ = `
		SELECT id, instrumen_lama_id, instrumen_baru_id, skema, tenor_baru_bulan,
		       rate_baru_persen::text, tanggal_efektif_baru, tanggal_jatuh_tempo_baru,
		       pokok_lama::text, pokok_baru::text, bunga_kotor::text,
		       pph_amount::text, bunga_bersih::text,
		       COALESCE(eir_baru::text, '') AS eir_baru_str,
		       status, maker_id, approver_id,
		       approve_reason, reject_reason, jurnal_header_id, periode_bulanan_id,
		       created_at, created_by, updated_at, updated_by, row_version, tenant_id
		FROM trx.renewal
		WHERE deleted_at IS NULL
		  AND tenant_id = $1`

	const noCursorSuffix = `
		ORDER BY created_at DESC, id DESC
		LIMIT $2`

	const cursorSuffix = `
		  AND (created_at, id) < ($3, $4)
		ORDER BY created_at DESC, id DESC
		LIMIT $2`

	// Resolve tenant from context claims; fall back to 'TUGURE' for Phase 1.
	tenantID := "TUGURE"

	var (
		sqlRows *sql.Rows
		err     error
	)
	if cursor == "" {
		sqlRows, err = r.db.QueryContext(ctx, baseQ+noCursorSuffix, tenantID, limit+1)
	} else {
		sqlRows, err = r.db.QueryContext(ctx, baseQ+cursorSuffix, tenantID, limit+1, cursorTime, cursorID)
	}
	if err != nil {
		return nil, false, 0, fmt.Errorf("repo.List: %w", err)
	}
	defer sqlRows.Close() //nolint:errcheck

	var result []*Renewal
	for sqlRows.Next() {
		var (
			row            Renewal
			rateStr        string
			pokokLamaStr   string
			pokokBaruStr   string
			bungaKotorStr  string
			pphAmountStr   string
			bungaBersihStr string
			eirBaruStr     string
		)
		if err := sqlRows.Scan(
			&row.ID, &row.InstrumenLamaID, &row.InstrumenBaruID, &row.Skema, &row.TenorBaruBulan,
			&rateStr, &row.TanggalEfektifBaru, &row.TanggalJatuhTempoBaru,
			&pokokLamaStr, &pokokBaruStr, &bungaKotorStr,
			&pphAmountStr, &bungaBersihStr, &eirBaruStr,
			&row.Status, &row.MakerID, &row.ApproverID,
			&row.ApproveReason, &row.RejectReason, &row.JurnalHeaderID, &row.PeriodeBulananID,
			&row.CreatedAt, &row.CreatedBy, &row.UpdatedAt, &row.UpdatedBy,
			&row.RowVersion, &row.TenantID,
		); err != nil {
			return nil, false, 0, fmt.Errorf("repo.List: scan: %w", err)
		}
		if v, e := decimal.NewFromString(rateStr); e == nil {
			row.RateBaruPersen = v
		}
		if v, e := decimal.NewFromString(pokokLamaStr); e == nil {
			row.PokokLama = v
		}
		if v, e := decimal.NewFromString(pokokBaruStr); e == nil {
			row.PokokBaru = v
		}
		if v, e := decimal.NewFromString(bungaKotorStr); e == nil {
			row.BungaKotor = v
		}
		if v, e := decimal.NewFromString(pphAmountStr); e == nil {
			row.PphAmount = v
		}
		if v, e := decimal.NewFromString(bungaBersihStr); e == nil {
			row.BungaBersih = v
		}
		if eirBaruStr != "" {
			if v, e := decimal.NewFromString(eirBaruStr); e == nil {
				row.EirBaru = &v
			}
		}
		result = append(result, &row)
	}
	if err := sqlRows.Err(); err != nil {
		return nil, false, 0, fmt.Errorf("repo.List: iterate: %w", err)
	}

	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}
	return result, hasMore, len(result), nil
}

// decodeCursor decodes a base64-encoded keyset cursor into (created_at, id).
// Cursor format: base64(created_at_RFC3339Nano + "|" + uuid_string).
func decodeCursor(cursor string) (time.Time, uuid.UUID, error) {
	b, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("base64 decode: %w", err)
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, uuid.Nil, fmt.Errorf("cursor format invalid (expected 'timestamp|uuid')")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("cursor time parse: %w", err)
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("cursor uuid parse: %w", err)
	}
	return t, id, nil
}

// encodeCursor encodes a (created_at, id) pair into a base64 keyset cursor.
func encodeCursor(createdAt time.Time, id uuid.UUID) string {
	raw := createdAt.UTC().Format(time.RFC3339Nano) + "|" + id.String()
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

// GetPeriodeByTanggal looks up mst.periode_buku for a given date.
func (r *Repo) GetPeriodeByTanggal(ctx context.Context, tanggal time.Time) (*PeriodeBuku, error) {
	const q = `
		SELECT id, status_periode, tanggal_mulai, tanggal_akhir
		FROM mst.periode_buku
		WHERE $1 BETWEEN tanggal_mulai AND tanggal_akhir
		  AND deleted_at IS NULL
		LIMIT 1`

	var row PeriodeBuku
	err := r.db.QueryRowContext(ctx, q, tanggal).Scan(
		&row.ID, &row.StatusPeriode, &row.TanggalMulai, &row.TanggalAkhir,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.GetPeriodeByTanggal: %w", err)
	}
	return &row, nil
}

// decimalPtrToStr returns nil if d is nil, else StringFixed(scale).
func decimalPtrToStr(d *decimal.Decimal, scale int32) interface{} {
	if d == nil {
		return nil
	}
	return d.StringFixed(scale)
}
