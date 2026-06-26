package mtm

// repo.go — Repository for trx.mtm. Uses database/sql only (sqlx not in go.mod).
// No business logic here; all SQL lives in this file.
//
// The Repository interface is defined first so service.go depends on the interface,
// not the concrete type — enables sqlmock-based testing.

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// Repository defines the data-access contract for trx.mtm.
type Repository interface {
	// Insert inserts a new Mtm row in the given transaction.
	Insert(ctx context.Context, tx *sql.Tx, m *Mtm) error

	// GetByID fetches one Mtm row by UUID. Returns (nil, nil) if not found.
	GetByID(ctx context.Context, id uuid.UUID) (*Mtm, error)

	// UpdateStatus updates status, override fields, and jurnal linkage in tx.
	UpdateStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, update StatusUpdate) error

	// ExistsActive checks if a non-REJECTED row exists for (instrumen_id, tanggal_mtm, harga_sumber).
	ExistsActive(ctx context.Context, instrumenID uuid.UUID, tanggalMtm time.Time, hargaSumber string) (bool, *Mtm, error)

	// List returns paginated Mtm rows matching the query.
	List(ctx context.Context, q listquery.Query, cursor string, limit int) ([]*Mtm, bool, int, error)

	// ListByBatchID returns all Mtm rows for a given upload_batch_id.
	ListByBatchID(ctx context.Context, batchID uuid.UUID) ([]*Mtm, error)

	// ListStaleAlerts returns rows with stale_price_flag=TRUE, sorted by harga_age_days DESC.
	ListStaleAlerts(ctx context.Context, cursor string, limit int) ([]*Mtm, bool, int, error)

	// LockMtmForPeriode sets locked_flag=TRUE for all rows in the periode date range.
	LockMtmForPeriode(ctx context.Context, tx *sql.Tx, periodeID uuid.UUID, tanggalMulai, tanggalAkhir time.Time, actorID uuid.UUID) (int64, error)

	// UnlockMtmForPeriode sets locked_flag=FALSE for all rows in the periode date range.
	UnlockMtmForPeriode(ctx context.Context, tx *sql.Tx, periodeID uuid.UUID, tanggalMulai, tanggalAkhir time.Time, actorID uuid.UUID) (int64, error)

	// BeginTx starts a database transaction with default isolation.
	BeginTx(ctx context.Context) (*sql.Tx, error)

	// GetConfigValue reads one sys.config row by config_key.
	GetConfigValue(ctx context.Context, key string) (string, error)

	// IsHoliday checks if the given date is in sys.holiday_calendar.
	IsHoliday(ctx context.Context, t time.Time) (bool, error)

	// GetActiveNonACInstrumen returns InstrumenInfo for all ACTIVE non-AC instruments.
	GetActiveNonACInstrumen(ctx context.Context) ([]InstrumenInfo, error)

	// GetFeedPrice fetches the latest price from the staging table for an instrument.
	// Returns (nil, nil) if no price found in feed.
	GetFeedPrice(ctx context.Context, instrumenID uuid.UUID, tipeInstrumen string, tanggalMtm time.Time) (*FeedPrice, error)

	// GetApprovedKurs fetches APPROVED kurs for a currency on a given date.
	// Returns (nil, nil) if not found.
	GetApprovedKurs(ctx context.Context, kodeMataUang string, tanggalMtm time.Time) (*KursSnapshot, error)

	// GetHargaBukuIdr fetches current book value for an instrument from trx.penempatan.
	// For Stage 3: Net Carrying (Gross − ECL). Returns (nil, nil) if not found.
	GetHargaBukuIdr(ctx context.Context, instrumenID uuid.UUID) (*decimal.Decimal, error)

	// GetPeriodeByTanggal finds the active periode_buku that contains the given date.
	// Returns (nil, nil) if no periode covers tanggal.
	// Used by LockMtmForPeriode (M1 fix) and ProcessOneInstrument (m1 fix).
	GetPeriodeByTanggal(ctx context.Context, tanggal time.Time) (*PeriodeBukuRef, error)

	// InsertUploadBatch inserts a sys.upload_batch row (batch_type='MTM_UPLOAD').
	InsertUploadBatch(ctx context.Context, tx *sql.Tx, b *UploadBatch) error

	// GetUploadBatch fetches sys.upload_batch row by ID. Returns (nil, nil) if not found.
	GetUploadBatch(ctx context.Context, batchID uuid.UUID) (*UploadBatch, error)
}

// ─── Supporting types ─────────────────────────────────────────────────────────

// FeedPrice is the minimal result from feed staging tables.
type FeedPrice struct {
	InstrumenID  uuid.UUID
	HargaPasar   decimal.Decimal
	HargaTanggal time.Time
	MataUang     string // ISO 4217
}

// KursSnapshot is the minimal result from mst.kurs for a given date.
type KursSnapshot struct {
	KursID         uuid.UUID
	KodeMataUang   string
	KursTengah     decimal.Decimal
	TanggalBerlaku time.Time
}

// StatusUpdate carries the fields to UPDATE on a trx.mtm row.
type StatusUpdate struct {
	Status             Status
	OverrideApproverID *uuid.UUID
	OverrideComment    *string
	OverrideAt         *time.Time
	JurnalEntryID      *uuid.UUID
	JurnalEntryID2     *uuid.UUID
	JurnalEventCode    *string
	JurnalEventCode2   *string
	UpdatedBy          uuid.UUID
	RowVersion         int64 // expected current row_version for optimistic lock
}

// PeriodeBukuRef holds the minimal periode_buku fields needed by MTM service.
// Obtained via GetPeriodeByTanggal (M1 fix — replaces hardcoded 2000-2100 stub).
type PeriodeBukuRef struct {
	ID            uuid.UUID
	TanggalMulai  time.Time
	TanggalAkhir  time.Time
	StatusPeriode string // OPEN | SOFT_CLOSED | HARD_CLOSED
}

// UploadBatch mirrors sys.upload_batch minimal shape for MTM upload.
type UploadBatch struct {
	ID            uuid.UUID
	BatchType     string // always "MTM_UPLOAD"
	Status        string // "PENDING_REVIEW"
	CatatanUpload string
	UploaderID    uuid.UUID
	TotalRows     int
	ValidRows     int
	InvalidRows   int
	TenantID      string
	CreatedAt     time.Time
	CreatedBy     uuid.UUID
	UpdatedAt     time.Time
	UpdatedBy     uuid.UUID
}

// ─── DBRepository ─────────────────────────────────────────────────────────────

// DBRepository is the concrete database/sql-based Repository implementation.
type DBRepository struct {
	db *sql.DB
}

// NewDBRepository creates a new DBRepository.
func NewDBRepository(db *sql.DB) *DBRepository {
	return &DBRepository{db: db}
}

// BeginTx starts a transaction with default isolation.
func (r *DBRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	if r.db == nil {
		return nil, fmt.Errorf("DBRepository.BeginTx: database not configured (dev mode)")
	}
	return r.db.BeginTx(ctx, nil)
}

// GetConfigValue reads one sys.config row by config_key.
func (r *DBRepository) GetConfigValue(ctx context.Context, key string) (string, error) {
	if r.db == nil {
		return "", nil
	}
	var val string
	err := r.db.QueryRowContext(ctx,
		`SELECT config_value FROM sys.config WHERE config_key = $1`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("GetConfigValue(%q): %w", key, err)
	}
	return val, nil
}

// IsHoliday checks sys.holiday_calendar for the given date.
func (r *DBRepository) IsHoliday(ctx context.Context, t time.Time) (bool, error) {
	if r.db == nil {
		return false, nil
	}
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM sys.holiday_calendar WHERE tanggal = $1)`,
		t.Format("2006-01-02")).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("IsHoliday: %w", err)
	}
	return exists, nil
}

// GetActiveNonACInstrumen returns InstrumenInfo for ACTIVE non-AC instruments.
func (r *DBRepository) GetActiveNonACInstrumen(ctx context.Context) ([]InstrumenInfo, error) {
	if r.db == nil {
		return nil, nil
	}
	const q = `
		SELECT id, kode_instrumen, nama_instrumen,
		       klasifikasi_psak71, klasifikasi_locked,
		       mata_uang, tipe_instrumen,
		       COALESCE(poci_flag, FALSE)
		FROM mst.instrumen
		WHERE status = 'ACTIVE'
		  AND klasifikasi_psak71 IN ('FVOCI_DEBT','FVTPL','FVOCI_ELECTION','POCI')
		  AND klasifikasi_locked = TRUE
		  AND deleted_at IS NULL
		  AND tenant_id = 'TUGURE'`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("GetActiveNonACInstrumen: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var result []InstrumenInfo
	for rows.Next() {
		var i InstrumenInfo
		if err := rows.Scan(&i.ID, &i.KodeInstrumen, &i.NamaInstrumen,
			&i.KlasifikasiPSAK71, &i.KlasifikasiLocked,
			&i.MataUang, &i.TipeInstrumen, &i.IsPOCI); err != nil {
			return nil, fmt.Errorf("GetActiveNonACInstrumen: scan: %w", err)
		}
		result = append(result, i)
	}
	return result, rows.Err()
}

// GetFeedPrice fetches the latest price from the appropriate feed staging table.
func (r *DBRepository) GetFeedPrice(ctx context.Context, instrumenID uuid.UUID, tipeInstrumen string, tanggalMtm time.Time) (*FeedPrice, error) {
	if r.db == nil {
		return nil, nil
	}
	var query string
	switch tipeInstrumen {
	case "OBLIGASI":
		query = `SELECT instrumen_id, harga_pasar, tanggal_harga, mata_uang
		         FROM sys.ibpa_feed_staging
		         WHERE instrumen_id = $1 AND tanggal_harga <= $2
		         ORDER BY tanggal_harga DESC LIMIT 1`
	case "SAHAM":
		query = `SELECT instrumen_id, harga_pasar, tanggal_harga, mata_uang
		         FROM sys.bei_feed_staging
		         WHERE instrumen_id = $1 AND tanggal_harga <= $2
		         ORDER BY tanggal_harga DESC LIMIT 1`
	case "REKSADANA":
		query = `SELECT instrumen_id, harga_pasar, tanggal_harga, mata_uang
		         FROM sys.ksei_feed_staging
		         WHERE instrumen_id = $1 AND tanggal_harga <= $2
		         ORDER BY tanggal_harga DESC LIMIT 1`
	default:
		return nil, nil // unknown tipe → STALE_PRICE
	}
	row := r.db.QueryRowContext(ctx, query, instrumenID, tanggalMtm.Format("2006-01-02"))
	fp := &FeedPrice{}
	var hargaPasarStr string
	if err := row.Scan(&fp.InstrumenID, &hargaPasarStr, &fp.HargaTanggal, &fp.MataUang); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetFeedPrice: %w", err)
	}
	if v, err := decimal.NewFromString(hargaPasarStr); err == nil {
		fp.HargaPasar = v
	}
	return fp, nil
}

// GetApprovedKurs fetches an APPROVED kurs row for the given currency and date.
func (r *DBRepository) GetApprovedKurs(ctx context.Context, kodeMataUang string, tanggalMtm time.Time) (*KursSnapshot, error) {
	if r.db == nil {
		return nil, nil
	}
	const q = `
		SELECT id, kode_mata_uang, kurs_tengah, tanggal_berlaku
		FROM mst.kurs
		WHERE kode_mata_uang = $1
		  AND tanggal_berlaku = $2
		  AND workflow_status = 'APPROVED'
		  AND deleted_at IS NULL
		LIMIT 1`
	ks := &KursSnapshot{}
	var kursTengahStr string
	err := r.db.QueryRowContext(ctx, q, kodeMataUang, tanggalMtm.Format("2006-01-02")).
		Scan(&ks.KursID, &ks.KodeMataUang, &kursTengahStr, &ks.TanggalBerlaku)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("GetApprovedKurs: %w", err)
	}
	if v, err := decimal.NewFromString(kursTengahStr); err == nil {
		ks.KursTengah = v
	}
	return ks, nil
}

// GetHargaBukuIdr fetches current book value for an instrument.
// NOTE(OQ-M6-6): ecl-eir-engineer to confirm exact column from trx.penempatan.
// For Stage 3 FVTPL: must be Net Carrying (Gross − ECL).
func (r *DBRepository) GetHargaBukuIdr(ctx context.Context, instrumenID uuid.UUID) (*decimal.Decimal, error) {
	// TODO(OQ-M6-6): confirm exact column from trx.penempatan with ecl-eir-engineer.
	// Placeholder returns nil causing service to flag STALE_PRICE.
	return nil, nil
}

// Insert inserts a new Mtm row in the given transaction.
// Includes mata_uang column added by migration 000042 (B2 fix).
func (r *DBRepository) Insert(ctx context.Context, tx *sql.Tx, m *Mtm) error {
	if r.db == nil {
		return fmt.Errorf("DBRepository.Insert: database not configured")
	}
	const q = `
		INSERT INTO trx.mtm (
			id, instrumen_id, periode_bulanan_id, tanggal_mtm,
			harga_sumber, harga_tanggal, harga_age_days,
			harga_pasar_fcy, harga_pasar_idr, harga_buku_idr, delta_idr, delta_pct,
			kurs_id, kurs_tengah,
			mata_uang,
			klasifikasi_snapshot, treatment_snapshot,
			jurnal_entry_id, jurnal_entry_id_2, jurnal_event_code, jurnal_event_code_2,
			stale_price_flag, deviation_flag, locked_flag, status,
			upload_batch_id, uploader_id, cron_job_id,
			override_approver_id, override_comment, override_at,
			created_at, created_by, updated_at, updated_by, row_version, tenant_id
		) VALUES (
			$1,$2,$3,$4,
			$5,$6,$7,
			$8,$9,$10,$11,$12,
			$13,$14,
			$15,
			$16,$17,
			$18,$19,$20,$21,
			$22,$23,$24,$25,
			$26,$27,$28,
			$29,$30,$31,
			$32,$33,$34,$35,$36,$37
		)`
	mataUang := m.MataUang
	if mataUang == "" {
		mataUang = "IDR"
	}
	_, err := tx.ExecContext(ctx, q,
		m.ID, m.InstrumenID, m.PeriodeBulananID, m.TanggalMtm,
		m.HargaSumber, m.HargaTanggal, m.HargaAgeDays,
		m.HargaPasarFcy, m.HargaPasarIdr, m.HargaBukuIdr, m.DeltaIdr, m.DeltaPct,
		m.KursID, m.KursTengah,
		mataUang,
		m.KlasifikasiSnapshot, m.TreatmentSnapshot,
		m.JurnalEntryID, m.JurnalEntryID2, m.JurnalEventCode, m.JurnalEventCode2,
		m.StalePriceFlag, m.DeviationFlag, m.LockedFlag, string(m.Status),
		m.UploadBatchID, m.UploaderID, m.CronJobID,
		m.OverrideApproverID, m.OverrideComment, m.OverrideAt,
		m.CreatedAt, m.CreatedBy, m.UpdatedAt, m.UpdatedBy, m.RowVersion, m.TenantID,
	)
	if err != nil {
		return fmt.Errorf("DBRepository.Insert: %w", err)
	}
	return nil
}

// GetByID fetches one Mtm row by UUID.
// Includes mata_uang column added by migration 000042 (B2 fix).
func (r *DBRepository) GetByID(ctx context.Context, id uuid.UUID) (*Mtm, error) {
	if r.db == nil {
		return nil, nil
	}
	const q = `
		SELECT id, instrumen_id, periode_bulanan_id, tanggal_mtm,
		       harga_sumber, harga_tanggal, harga_age_days,
		       harga_pasar_fcy, harga_pasar_idr, harga_buku_idr, delta_idr, delta_pct,
		       kurs_id, kurs_tengah,
		       COALESCE(mata_uang, 'IDR'),
		       klasifikasi_snapshot, treatment_snapshot,
		       jurnal_entry_id, jurnal_entry_id_2, jurnal_event_code, jurnal_event_code_2,
		       stale_price_flag, deviation_flag, locked_flag, status,
		       upload_batch_id, uploader_id, cron_job_id,
		       override_approver_id, override_comment, override_at,
		       created_at, created_by, updated_at, updated_by, row_version, tenant_id
		FROM trx.mtm WHERE id = $1 AND deleted_at IS NULL`
	m := &Mtm{}
	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&m.ID, &m.InstrumenID, &m.PeriodeBulananID, &m.TanggalMtm,
		&m.HargaSumber, &m.HargaTanggal, &m.HargaAgeDays,
		&m.HargaPasarFcy, &m.HargaPasarIdr, &m.HargaBukuIdr, &m.DeltaIdr, &m.DeltaPct,
		&m.KursID, &m.KursTengah,
		&m.MataUang,
		&m.KlasifikasiSnapshot, &m.TreatmentSnapshot,
		&m.JurnalEntryID, &m.JurnalEntryID2, &m.JurnalEventCode, &m.JurnalEventCode2,
		&m.StalePriceFlag, &m.DeviationFlag, &m.LockedFlag, &m.Status,
		&m.UploadBatchID, &m.UploaderID, &m.CronJobID,
		&m.OverrideApproverID, &m.OverrideComment, &m.OverrideAt,
		&m.CreatedAt, &m.CreatedBy, &m.UpdatedAt, &m.UpdatedBy, &m.RowVersion, &m.TenantID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("DBRepository.GetByID: %w", err)
	}
	return m, nil
}

// GetPeriodeByTanggal finds the active periode_buku that covers the given date.
// Returns (nil, nil) if no periode row is found for the date.
// M1 fix: used by LockMtmForPeriode to derive real tanggal_mulai/tanggal_akhir.
func (r *DBRepository) GetPeriodeByTanggal(ctx context.Context, tanggal time.Time) (*PeriodeBukuRef, error) {
	if r.db == nil {
		return nil, nil
	}
	const q = `
		SELECT id, tanggal_mulai, tanggal_akhir, status_periode
		FROM mst.periode_buku
		WHERE tanggal_mulai <= $1 AND tanggal_akhir >= $1
		  AND deleted_at IS NULL
		  AND tenant_id = 'TUGURE'
		ORDER BY tanggal_akhir DESC
		LIMIT 1`
	p := &PeriodeBukuRef{}
	err := r.db.QueryRowContext(ctx, q, tanggal.Format("2006-01-02")).Scan(
		&p.ID, &p.TanggalMulai, &p.TanggalAkhir, &p.StatusPeriode,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("DBRepository.GetPeriodeByTanggal: %w", err)
	}
	return p, nil
}

// UpdateStatus updates status and related fields in a transaction.
func (r *DBRepository) UpdateStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, u StatusUpdate) error {
	if r.db == nil {
		return fmt.Errorf("DBRepository.UpdateStatus: database not configured")
	}
	const q = `
		UPDATE trx.mtm SET
			status               = $1,
			override_approver_id = $2,
			override_comment     = $3,
			override_at          = $4,
			jurnal_entry_id      = $5,
			jurnal_entry_id_2    = $6,
			jurnal_event_code    = $7,
			jurnal_event_code_2  = $8,
			updated_by           = $9,
			updated_at           = now(),
			row_version          = row_version + 1
		WHERE id = $10 AND row_version = $11 AND deleted_at IS NULL`
	result, err := tx.ExecContext(ctx, q,
		string(u.Status), u.OverrideApproverID, u.OverrideComment, u.OverrideAt,
		u.JurnalEntryID, u.JurnalEntryID2, u.JurnalEventCode, u.JurnalEventCode2,
		u.UpdatedBy,
		id, u.RowVersion,
	)
	if err != nil {
		return fmt.Errorf("DBRepository.UpdateStatus: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("DBRepository.UpdateStatus: rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("DBRepository.UpdateStatus: row not found or version mismatch (id=%s, row_version=%d)", id, u.RowVersion)
	}
	return nil
}

// ExistsActive checks if a non-REJECTED, non-deleted MTM row already exists.
func (r *DBRepository) ExistsActive(ctx context.Context, instrumenID uuid.UUID, tanggalMtm time.Time, hargaSumber string) (bool, *Mtm, error) {
	if r.db == nil {
		return false, nil, nil
	}
	const q = `
		SELECT id, status FROM trx.mtm
		WHERE instrumen_id = $1 AND tanggal_mtm = $2 AND harga_sumber = $3
		  AND status != 'REJECTED'
		  AND deleted_at IS NULL
		LIMIT 1`
	var id uuid.UUID
	var status string
	err := r.db.QueryRowContext(ctx, q, instrumenID, tanggalMtm.Format("2006-01-02"), hargaSumber).Scan(&id, &status)
	if err == sql.ErrNoRows {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, fmt.Errorf("DBRepository.ExistsActive: %w", err)
	}
	return true, &Mtm{ID: id, Status: Status(status)}, nil
}

// List returns paginated Mtm rows matching the query.
// Full cursor implementation follows kurs/repo.go pattern. Stub for dev mode.
func (r *DBRepository) List(ctx context.Context, q listquery.Query, cursor string, limit int) ([]*Mtm, bool, int, error) {
	if r.db == nil {
		return nil, false, 0, nil
	}
	// TODO(follow-up): implement full cursor-paginated list with listquery.ToSQL.
	return nil, false, 0, nil
}

// ListByBatchID returns all Mtm rows for a batch.
func (r *DBRepository) ListByBatchID(ctx context.Context, batchID uuid.UUID) ([]*Mtm, error) {
	if r.db == nil {
		return nil, nil
	}
	const q = `
		SELECT id, instrumen_id, status, harga_pasar_idr, delta_idr, delta_pct,
		       stale_price_flag, deviation_flag, harga_sumber, tanggal_mtm, created_at
		FROM trx.mtm WHERE upload_batch_id = $1 AND deleted_at IS NULL ORDER BY created_at ASC`
	rows, err := r.db.QueryContext(ctx, q, batchID)
	if err != nil {
		return nil, fmt.Errorf("DBRepository.ListByBatchID: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var result []*Mtm
	for rows.Next() {
		m := &Mtm{}
		if err := rows.Scan(
			&m.ID, &m.InstrumenID, &m.Status, &m.HargaPasarIdr, &m.DeltaIdr, &m.DeltaPct,
			&m.StalePriceFlag, &m.DeviationFlag, &m.HargaSumber, &m.TanggalMtm, &m.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("DBRepository.ListByBatchID: scan: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// ListStaleAlerts returns stale rows ordered by harga_age_days DESC.
func (r *DBRepository) ListStaleAlerts(ctx context.Context, cursor string, limit int) ([]*Mtm, bool, int, error) {
	if r.db == nil {
		return nil, false, 0, nil
	}
	const q = `
		SELECT id, instrumen_id, status, harga_age_days, tanggal_mtm, harga_sumber,
		       stale_price_flag, deviation_flag, created_at
		FROM trx.mtm
		WHERE stale_price_flag = TRUE
		  AND status IN ('STALE_PRICE', 'PENDING_REVIEW')
		  AND deleted_at IS NULL
		ORDER BY harga_age_days DESC, tanggal_mtm ASC
		LIMIT $1`
	rows, err := r.db.QueryContext(ctx, q, limit+1)
	if err != nil {
		return nil, false, 0, fmt.Errorf("DBRepository.ListStaleAlerts: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var result []*Mtm
	for rows.Next() {
		m := &Mtm{}
		if err := rows.Scan(
			&m.ID, &m.InstrumenID, &m.Status, &m.HargaAgeDays, &m.TanggalMtm, &m.HargaSumber,
			&m.StalePriceFlag, &m.DeviationFlag, &m.CreatedAt,
		); err != nil {
			return nil, false, 0, fmt.Errorf("DBRepository.ListStaleAlerts: scan: %w", err)
		}
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		return nil, false, 0, err
	}
	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}
	return result, hasMore, len(result), nil
}

// LockMtmForPeriode sets locked_flag=TRUE for all rows in the periode date range.
// M1 fix: added AND periode_bulanan_id = $4 for defence-in-depth scoping.
func (r *DBRepository) LockMtmForPeriode(ctx context.Context, tx *sql.Tx, periodeID uuid.UUID, tanggalMulai, tanggalAkhir time.Time, actorID uuid.UUID) (int64, error) {
	if r.db == nil {
		return 0, nil
	}
	const q = `
		UPDATE trx.mtm
		SET locked_flag = TRUE, updated_at = now(), updated_by = $1, row_version = row_version + 1
		WHERE tanggal_mtm BETWEEN $2 AND $3
		  AND periode_bulanan_id = $4
		  AND tenant_id = 'TUGURE'
		  AND deleted_at IS NULL`
	result, err := tx.ExecContext(ctx, q, actorID,
		tanggalMulai.Format("2006-01-02"), tanggalAkhir.Format("2006-01-02"), periodeID)
	if err != nil {
		return 0, fmt.Errorf("DBRepository.LockMtmForPeriode: %w", err)
	}
	count, _ := result.RowsAffected()
	return count, nil
}

// UnlockMtmForPeriode sets locked_flag=FALSE for all rows in the periode date range.
// M1 fix: added AND periode_bulanan_id = $4 for defence-in-depth scoping.
func (r *DBRepository) UnlockMtmForPeriode(ctx context.Context, tx *sql.Tx, periodeID uuid.UUID, tanggalMulai, tanggalAkhir time.Time, actorID uuid.UUID) (int64, error) {
	if r.db == nil {
		return 0, nil
	}
	const q = `
		UPDATE trx.mtm
		SET locked_flag = FALSE, updated_at = now(), updated_by = $1, row_version = row_version + 1
		WHERE tanggal_mtm BETWEEN $2 AND $3
		  AND periode_bulanan_id = $4
		  AND tenant_id = 'TUGURE'
		  AND deleted_at IS NULL`
	result, err := tx.ExecContext(ctx, q, actorID,
		tanggalMulai.Format("2006-01-02"), tanggalAkhir.Format("2006-01-02"), periodeID)
	if err != nil {
		return 0, fmt.Errorf("DBRepository.UnlockMtmForPeriode: %w", err)
	}
	count, _ := result.RowsAffected()
	return count, nil
}

// InsertUploadBatch inserts a sys.upload_batch row.
func (r *DBRepository) InsertUploadBatch(ctx context.Context, tx *sql.Tx, b *UploadBatch) error {
	if r.db == nil {
		return nil
	}
	const q = `
		INSERT INTO sys.upload_batch (id, batch_type, status, catatan, uploader_id,
		            total_rows, valid_rows, invalid_rows, tenant_id,
		            created_at, created_by, updated_at, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`
	_, err := tx.ExecContext(ctx, q,
		b.ID, b.BatchType, b.Status, b.CatatanUpload, b.UploaderID,
		b.TotalRows, b.ValidRows, b.InvalidRows, b.TenantID,
		b.CreatedAt, b.CreatedBy, b.UpdatedAt, b.UpdatedBy,
	)
	if err != nil {
		return fmt.Errorf("DBRepository.InsertUploadBatch: %w", err)
	}
	return nil
}

// GetUploadBatch fetches a sys.upload_batch row.
func (r *DBRepository) GetUploadBatch(ctx context.Context, batchID uuid.UUID) (*UploadBatch, error) {
	if r.db == nil {
		return nil, nil
	}
	const q = `
		SELECT id, batch_type, status, catatan, uploader_id,
		       total_rows, valid_rows, invalid_rows, tenant_id,
		       created_at, created_by, updated_at, updated_by
		FROM sys.upload_batch WHERE id = $1`
	b := &UploadBatch{}
	err := r.db.QueryRowContext(ctx, q, batchID).Scan(
		&b.ID, &b.BatchType, &b.Status, &b.CatatanUpload, &b.UploaderID,
		&b.TotalRows, &b.ValidRows, &b.InvalidRows, &b.TenantID,
		&b.CreatedAt, &b.CreatedBy, &b.UpdatedAt, &b.UpdatedBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("DBRepository.GetUploadBatch: %w", err)
	}
	return b, nil
}

// Ensure DBRepository satisfies Repository at compile time.
var _ Repository = (*DBRepository)(nil)
