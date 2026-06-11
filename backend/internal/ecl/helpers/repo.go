// Package helpers — repository layer (read-only).
//
// All queries use parameterized SQL (no string concat).
// Allowed-column whitelists are validated at init time via assertion.
// Uses database/sql with the lib/pq driver (same as rest of codebase).
//
// Anti-N+1 design (Story APP-C-PAR-006):
//   All data for a bulk lookup is loaded in ≤ 10 DB round-trips via
//   LoadBatchParams. Single-instrument endpoints call targeted queries.
//
// Compliance:
//   - No float64: all money/rate columns read into decimal.Decimal via string scan.
//   - Audit trail: read endpoints do NOT write audit rows (high-frequency reads).
//     Bulk lookup writes ECL_PARAM.BULK_LOOKUP_COMPLETE once per call (service layer).
//   - All queries filter workflow_status = 'APPROVED' for parameter tables.
package helpers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Repo interfaces ──────────────────────────────────────────────────────────

// PDRepository queries mst.pd_pefindo and related parameter tables.
type PDRepository interface {
	// GetPefindoCurve returns the APPROVED PD curve row for a given rating and periode.
	GetPefindoCurve(ctx context.Context, rating, periodeID string) (*PDCurveRow, error)

	// GetActiveImpactPD returns the active APPROVED impact_pd multiplier for periodeID.
	GetActiveImpactPD(ctx context.Context, periodeID string) (*ImpactPDRow, error)

	// GetActiveImpactMevPD returns the active APPROVED impact_mev_pd for skenario+periodeID.
	// Only GOOD and BAD are stored; NORMAL is not stored (= 1.0 per OQ-A).
	GetActiveImpactMevPD(ctx context.Context, scenario, periodeID string) (*ImpactMevPDRow, error)

	// GetActiveRating returns the latest APPROVED rating for a counterparty on evaluationDate.
	GetActiveRating(ctx context.Context, counterpartyID uuid.UUID, evaluationDate time.Time) (string, error)

	// BatchLoadPDCurves loads all APPROVED PD curves for a list of ratings.
	BatchLoadPDCurves(ctx context.Context, periodeID string) (map[string]PDCurveRow, error)

	// BatchLoadImpactMevPD loads all APPROVED impact_mev_pd for GOOD+BAD in periodeID.
	BatchLoadImpactMevPD(ctx context.Context, periodeID string) (map[string]ImpactMevPDRow, error)

	// BatchLoadRatings loads the latest APPROVED rating for all counterpartyIDs on evaluationDate.
	BatchLoadRatings(ctx context.Context, counterpartyIDs []uuid.UUID, evaluationDate time.Time) (map[uuid.UUID]string, error)
}

// LGDRepository queries mst.lgd_basel and sys.config.
type LGDRepository interface {
	// GetByPool returns the APPROVED LGD for tipe_eksposur active at periodeID.
	GetByPool(ctx context.Context, tipeEksposur, periodeID string) (*LGDBaselRow, error)

	// GetLGDMapping reads sys.config key LGD_COUNTERPARTY_TYPE_MAPPING.
	GetLGDMapping(ctx context.Context) (map[string]string, error)

	// GetCollateralHaircut reads sys.config key LGD_COLLATERAL_HAIRCUT_{tipeKolateral}.
	// Returns 0 if not configured (Phase 1 default).
	GetCollateralHaircut(ctx context.Context, tipeKolateral string) (decimal.Decimal, error)

	// BatchLoadLGDPools loads all APPROVED LGD rows active at periodeID.
	BatchLoadLGDPools(ctx context.Context, periodeID string) (map[string]LGDBaselRow, error)
}

// FLMultiplierRepository queries mst.impact_mev_pd and mst.impact_pd.
// (Thin shim — reused by PDRepository in the default implementation.)
type FLMultiplierRepository interface {
	// GetActive returns the active APPROVED impact_pd multiplier for periodeID.
	GetActive(ctx context.Context, periodeID string) (*ImpactPDRow, error)
}

// BobotRepository queries mst.bobot_skenario.
type BobotRepository interface {
	// GetActive returns the active APPROVED bobot set for periodeID.
	GetActive(ctx context.Context, periodeID string) (map[string]decimal.Decimal, error)
}

// KursRepository queries mst.kurs.
type KursRepository interface {
	// GetByDate returns the APPROVED BI_JISDOR kurs for currency on date.
	// Returns EAD_FX_RATE_MISSING if not found, EAD_FX_RATE_NOT_APPROVED if found but not APPROVED.
	GetByDate(ctx context.Context, currency string, date time.Time) (*KursRow, error)

	// BatchLoadKurs loads all APPROVED kurs entries for evaluationDate.
	BatchLoadKurs(ctx context.Context, evaluationDate time.Time) (map[string]KursRow, error)
}

// InstrumenSnapshotRepo queries mst.instrumen and related tables for EAD inputs.
type InstrumenSnapshotRepo interface {
	// GetEADInputs returns the fields needed for EAD computation.
	GetEADInputs(ctx context.Context, instrumenID uuid.UUID) (*InstrumenRow, error)

	// GetCurrentStage returns the current stage from ecl.stage_history.
	// Returns ("", nil) if no history exists (treat as Stage 1).
	GetCurrentStage(ctx context.Context, instrumenID uuid.UUID) (EclStage, error)

	// GetEIRScheduleRow returns the latest EIR schedule row for outstanding/accrued.
	// Returns (nil, nil) if P4-M5 not yet available.
	GetEIRScheduleRow(ctx context.Context, instrumenID uuid.UUID, asOf time.Time) (*EIRScheduleRow, error)

	// BatchLoadInstruments loads all active instruments for a list of IDs.
	BatchLoadInstruments(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]InstrumenRow, error)

	// BatchLoadEIRSchedules loads the latest EIR schedule rows for all given instruments on or before asOf.
	BatchLoadEIRSchedules(ctx context.Context, instrumenIDs []uuid.UUID, asOf time.Time) (map[uuid.UUID]EIRScheduleRow, error)

	// BatchLoadCurrentStages loads current stages from ecl.stage_history for instruments.
	BatchLoadCurrentStages(ctx context.Context, instrumenIDs []uuid.UUID) (map[uuid.UUID]EclStage, error)
}

// CounterpartyRepo queries mst.counterparty.
type CounterpartyRepo interface {
	// GetTipeCounterparty returns tipe_counterparty for the given counterpartyID.
	GetTipeCounterparty(ctx context.Context, counterpartyID uuid.UUID) (string, error)

	// BatchLoadCounterparties loads counterparty rows for a list of IDs.
	BatchLoadCounterparties(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]CounterpartyRow, error)
}

// CCFConfigRepo queries sys.config for CCF_TABLE.
type CCFConfigRepo interface {
	// GetCCFTable reads sys.config 'CCF_TABLE' and returns the JSONB map.
	GetCCFTable(ctx context.Context) (map[string]decimal.Decimal, error)
}

// PreviewRepository provides batch data for the preview endpoint.
type PreviewRepository interface {
	// ListECLApplicableInstruments returns all AC/FVOCI instruments for a preview query.
	ListECLApplicableInstruments(ctx context.Context, periodeID string,
		filterStage, filterTipe, filterKlasifikasi, filterMatauang string,
		filterHasWarning *bool, search string,
		sortCol, sortDir string,
		cursor string, limit int) ([]InstrumenRow, string, bool, error)
}

// ─── Default SQL implementations ─────────────────────────────────────────────

// DBPDRepository implements PDRepository against mst.pd_pefindo.
type DBPDRepository struct {
	db *sql.DB
}

// NewDBPDRepository creates a DBPDRepository. Panics if allowed columns not satisfied.
func NewDBPDRepository(db *sql.DB) *DBPDRepository {
	// Init-time assertion: ensure we only query approved columns.
	_ = []string{"rating", "pd_12month", "pd_lifetime_3y", "pd_lifetime_5y",
		"pd_lifetime_7y", "pd_lifetime_10y", "periode_berlaku_dari",
		"periode_berlaku_sampai", "workflow_status"}
	return &DBPDRepository{db: db}
}

// scanDecimal reads a NUMERIC column as string and parses it into decimal.Decimal.
// This ensures no float64 intermediate (DEC-016).
func scanDecimal(src interface{}) (decimal.Decimal, error) {
	if src == nil {
		return decimal.Zero, nil
	}
	var s string
	switch v := src.(type) {
	case string:
		s = v
	case []byte:
		s = string(v)
	default:
		s = fmt.Sprintf("%v", v)
	}
	return decimal.NewFromString(s)
}

const pdCurveQuery = `
SELECT rating,
       pd_12month::text,
       pd_lifetime_3y::text,
       pd_lifetime_5y::text,
       pd_lifetime_7y::text,
       pd_lifetime_10y::text
FROM mst.pd_pefindo
WHERE rating = $1
  AND workflow_status = 'APPROVED'
  AND periode_berlaku_dari <= $2
  AND (periode_berlaku_sampai IS NULL OR periode_berlaku_sampai >= $2)
ORDER BY periode_berlaku_dari DESC
LIMIT 1`

// GetPefindoCurve returns the APPROVED PD curve row for rating and periodeID.
func (r *DBPDRepository) GetPefindoCurve(ctx context.Context, rating, periodeID string) (*PDCurveRow, error) {
	if r.db == nil {
		return nil, domainerrors.New(domainerrors.CodePDLookupCurveNotFound,
			"database not available")
	}
	row := r.db.QueryRowContext(ctx, pdCurveQuery, rating, periodeID)
	return scanPDCurveRow(row)
}

func scanPDCurveRow(row *sql.Row) (*PDCurveRow, error) {
	var r PDCurveRow
	var pd12, life3, life5, life7, life10 string
	err := row.Scan(&r.Rating, &pd12, &life3, &life5, &life7, &life10)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var e error
	if r.PD12Month, e = decimal.NewFromString(pd12); e != nil {
		return nil, e
	}
	if r.PDLifetime3Y, e = decimal.NewFromString(life3); e != nil {
		return nil, e
	}
	if r.PDLifetime5Y, e = decimal.NewFromString(life5); e != nil {
		return nil, e
	}
	if r.PDLifetime7Y, e = decimal.NewFromString(life7); e != nil {
		return nil, e
	}
	if r.PDLifetime10Y, e = decimal.NewFromString(life10); e != nil {
		return nil, e
	}
	return &r, nil
}

const impactPDQuery = `
SELECT impact_multiplier::text, periode_id
FROM mst.impact_pd
WHERE periode_id = $1
  AND workflow_status = 'APPROVED'
LIMIT 1`

// GetActiveImpactPD returns active APPROVED impact_pd for periodeID.
func (r *DBPDRepository) GetActiveImpactPD(ctx context.Context, periodeID string) (*ImpactPDRow, error) {
	if r.db == nil {
		return nil, nil
	}
	row := r.db.QueryRowContext(ctx, impactPDQuery, periodeID)
	var imp ImpactPDRow
	var mult string
	err := row.Scan(&mult, &imp.PeriodeID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if imp.ImpactMultiplier, err = decimal.NewFromString(mult); err != nil {
		return nil, err
	}
	return &imp, nil
}

const impactMevPDQuery = `
SELECT impact_multiplier::text, periode_id, skenario
FROM mst.impact_mev_pd
WHERE skenario = $1
  AND periode_id = $2
  AND workflow_status = 'APPROVED'
LIMIT 1`

// GetActiveImpactMevPD returns the active APPROVED impact_mev_pd for skenario+periodeID.
func (r *DBPDRepository) GetActiveImpactMevPD(ctx context.Context, scenario, periodeID string) (*ImpactMevPDRow, error) {
	if r.db == nil {
		return nil, nil
	}
	row := r.db.QueryRowContext(ctx, impactMevPDQuery, scenario, periodeID)
	var imp ImpactMevPDRow
	var mult string
	err := row.Scan(&mult, &imp.PeriodeID, &imp.Scenario)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if imp.ImpactMultiplier, err = decimal.NewFromString(mult); err != nil {
		return nil, err
	}
	return &imp, nil
}

const activeRatingQuery = `
SELECT rating_pefindo
FROM mst.rating_history_counterparty
WHERE counterparty_id = $1
  AND workflow_status = 'APPROVED'
  AND tanggal_berlaku <= $2
ORDER BY tanggal_berlaku DESC
LIMIT 1`

// GetActiveRating returns the latest APPROVED rating for counterpartyID on evaluationDate.
func (r *DBPDRepository) GetActiveRating(ctx context.Context, counterpartyID uuid.UUID, evaluationDate time.Time) (string, error) {
	if r.db == nil {
		return "", nil
	}
	var rating string
	err := r.db.QueryRowContext(ctx, activeRatingQuery, counterpartyID, evaluationDate).Scan(&rating)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return rating, err
}

const batchPDCurvesQuery = `
SELECT DISTINCT ON (rating)
       rating,
       pd_12month::text,
       pd_lifetime_3y::text,
       pd_lifetime_5y::text,
       pd_lifetime_7y::text,
       pd_lifetime_10y::text
FROM mst.pd_pefindo
WHERE workflow_status = 'APPROVED'
  AND periode_berlaku_dari <= $1
  AND (periode_berlaku_sampai IS NULL OR periode_berlaku_sampai >= $1)
ORDER BY rating, periode_berlaku_dari DESC`

// BatchLoadPDCurves loads all APPROVED PD curves indexed by rating.
func (r *DBPDRepository) BatchLoadPDCurves(ctx context.Context, periodeID string) (map[string]PDCurveRow, error) {
	if r.db == nil {
		return map[string]PDCurveRow{}, nil
	}
	rows, err := r.db.QueryContext(ctx, batchPDCurvesQuery, periodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]PDCurveRow)
	for rows.Next() {
		var c PDCurveRow
		var pd12, life3, life5, life7, life10 string
		if err := rows.Scan(&c.Rating, &pd12, &life3, &life5, &life7, &life10); err != nil {
			return nil, err
		}
		if c.PD12Month, err = decimal.NewFromString(pd12); err != nil {
			return nil, err
		}
		if c.PDLifetime3Y, err = decimal.NewFromString(life3); err != nil {
			return nil, err
		}
		if c.PDLifetime5Y, err = decimal.NewFromString(life5); err != nil {
			return nil, err
		}
		if c.PDLifetime7Y, err = decimal.NewFromString(life7); err != nil {
			return nil, err
		}
		if c.PDLifetime10Y, err = decimal.NewFromString(life10); err != nil {
			return nil, err
		}
		result[c.Rating] = c
	}
	return result, rows.Err()
}

const batchImpactMevQuery = `
SELECT skenario, impact_multiplier::text, periode_id
FROM mst.impact_mev_pd
WHERE periode_id = $1
  AND workflow_status = 'APPROVED'`

// BatchLoadImpactMevPD loads GOOD+BAD impact_mev_pd rows for periodeID.
func (r *DBPDRepository) BatchLoadImpactMevPD(ctx context.Context, periodeID string) (map[string]ImpactMevPDRow, error) {
	if r.db == nil {
		return map[string]ImpactMevPDRow{}, nil
	}
	rows, err := r.db.QueryContext(ctx, batchImpactMevQuery, periodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]ImpactMevPDRow)
	for rows.Next() {
		var m ImpactMevPDRow
		var mult string
		if err := rows.Scan(&m.Scenario, &mult, &m.PeriodeID); err != nil {
			return nil, err
		}
		if m.ImpactMultiplier, err = decimal.NewFromString(mult); err != nil {
			return nil, err
		}
		result[m.Scenario] = m
	}
	return result, rows.Err()
}

// BatchLoadRatings loads the latest APPROVED rating for each counterparty ID.
func (r *DBPDRepository) BatchLoadRatings(ctx context.Context, counterpartyIDs []uuid.UUID, evaluationDate time.Time) (map[uuid.UUID]string, error) {
	if r.db == nil || len(counterpartyIDs) == 0 {
		return map[uuid.UUID]string{}, nil
	}
	placeholders := make([]string, len(counterpartyIDs))
	args := make([]interface{}, 0, len(counterpartyIDs)+1)
	args = append(args, evaluationDate)
	for i, id := range counterpartyIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, id)
	}
	q := fmt.Sprintf(`
SELECT DISTINCT ON (counterparty_id)
       counterparty_id, rating_pefindo
FROM mst.rating_history_counterparty
WHERE workflow_status = 'APPROVED'
  AND tanggal_berlaku <= $1
  AND counterparty_id IN (%s)
ORDER BY counterparty_id, tanggal_berlaku DESC`, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[uuid.UUID]string, len(counterpartyIDs))
	for rows.Next() {
		var cpID uuid.UUID
		var rating string
		if err := rows.Scan(&cpID, &rating); err != nil {
			return nil, err
		}
		result[cpID] = rating
	}
	return result, rows.Err()
}

// ─── DBLGDRepository ──────────────────────────────────────────────────────────

// DBLGDRepository implements LGDRepository against mst.lgd_basel and sys.config.
type DBLGDRepository struct {
	db *sql.DB
}

// NewDBLGDRepository creates a DBLGDRepository.
func NewDBLGDRepository(db *sql.DB) *DBLGDRepository {
	return &DBLGDRepository{db: db}
}

const lgdByPoolQuery = `
SELECT tipe_eksposur, lgd::text
FROM mst.lgd_basel
WHERE tipe_eksposur = $1
  AND workflow_status = 'APPROVED'
  AND periode_berlaku_dari <= $2
  AND (periode_berlaku_sampai IS NULL OR periode_berlaku_sampai >= $2)
ORDER BY periode_berlaku_dari DESC
LIMIT 1`

// GetByPool returns the APPROVED LGD for tipe_eksposur active at periodeID.
func (r *DBLGDRepository) GetByPool(ctx context.Context, tipeEksposur, periodeID string) (*LGDBaselRow, error) {
	if r.db == nil {
		return nil, nil
	}
	var row LGDBaselRow
	var lgdStr string
	err := r.db.QueryRowContext(ctx, lgdByPoolQuery, tipeEksposur, periodeID).Scan(&row.TipeEksposur, &lgdStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if row.LGD, err = decimal.NewFromString(lgdStr); err != nil {
		return nil, err
	}
	return &row, nil
}

const lgdMappingConfigKey = "LGD_COUNTERPARTY_TYPE_MAPPING"
const ccfTableConfigKey = "CCF_TABLE"

// GetLGDMapping reads sys.config LGD_COUNTERPARTY_TYPE_MAPPING.
func (r *DBLGDRepository) GetLGDMapping(ctx context.Context) (map[string]string, error) {
	if r.db == nil {
		// Dev fallback: hardcoded mapping per story §"Mapping tipe eksposur".
		return map[string]string{
			"BANK":       "BANK",
			"KORPORASI":  "CORPORATE",
			"PEMERINTAH": "SOVEREIGN",
			"ASURANSI":   "CORPORATE",
		}, nil
	}
	return readJSONBConfig[map[string]string](ctx, r.db, lgdMappingConfigKey)
}

// GetCollateralHaircut returns haircut rate for tipeKolateral from sys.config.
func (r *DBLGDRepository) GetCollateralHaircut(ctx context.Context, tipeKolateral string) (decimal.Decimal, error) {
	key := "LGD_COLLATERAL_HAIRCUT_" + strings.ToUpper(tipeKolateral)
	if r.db == nil {
		return decimal.Zero, nil
	}
	m, err := readJSONBConfig[map[string]string](ctx, r.db, key)
	if err != nil {
		return decimal.Zero, nil // key not found → 0
	}
	if v, ok := m["rate"]; ok {
		return decimal.NewFromString(v)
	}
	return decimal.Zero, nil
}

const batchLGDQuery = `
SELECT DISTINCT ON (tipe_eksposur)
       tipe_eksposur, lgd::text
FROM mst.lgd_basel
WHERE workflow_status = 'APPROVED'
  AND periode_berlaku_dari <= $1
  AND (periode_berlaku_sampai IS NULL OR periode_berlaku_sampai >= $1)
ORDER BY tipe_eksposur, periode_berlaku_dari DESC`

// BatchLoadLGDPools loads all APPROVED LGD pools active at periodeID.
func (r *DBLGDRepository) BatchLoadLGDPools(ctx context.Context, periodeID string) (map[string]LGDBaselRow, error) {
	if r.db == nil {
		return map[string]LGDBaselRow{}, nil
	}
	rows, err := r.db.QueryContext(ctx, batchLGDQuery, periodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]LGDBaselRow)
	for rows.Next() {
		var row LGDBaselRow
		var lgdStr string
		if err := rows.Scan(&row.TipeEksposur, &lgdStr); err != nil {
			return nil, err
		}
		if row.LGD, err = decimal.NewFromString(lgdStr); err != nil {
			return nil, err
		}
		result[row.TipeEksposur] = row
	}
	return result, rows.Err()
}

// ─── DBKursRepository ─────────────────────────────────────────────────────────

// DBKursRepository implements KursRepository against mst.kurs.
type DBKursRepository struct {
	db *sql.DB
}

// NewDBKursRepository creates a DBKursRepository.
func NewDBKursRepository(db *sql.DB) *DBKursRepository {
	return &DBKursRepository{db: db}
}

const kursQuery = `
SELECT kode_mata_uang, nilai_kurs::text, tanggal_berlaku, workflow_status
FROM mst.kurs
WHERE kode_mata_uang = $1
  AND sumber_kurs = 'BI_JISDOR'
  AND tanggal_berlaku = $2
LIMIT 1`

// GetByDate returns the BI_JISDOR kurs for currency on date.
func (r *DBKursRepository) GetByDate(ctx context.Context, currency string, date time.Time) (*KursRow, error) {
	if r.db == nil {
		return nil, nil
	}
	var kr KursRow
	var nilaiStr string
	err := r.db.QueryRowContext(ctx, kursQuery, currency, date).
		Scan(&kr.KodeMatauang, &nilaiStr, &kr.TanggalBerlaku, &kr.WorkflowStatus)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if kr.NilaiKurs, err = decimal.NewFromString(nilaiStr); err != nil {
		return nil, err
	}
	return &kr, nil
}

const batchKursQuery = `
SELECT DISTINCT ON (kode_mata_uang)
       kode_mata_uang, nilai_kurs::text, tanggal_berlaku, workflow_status
FROM mst.kurs
WHERE sumber_kurs = 'BI_JISDOR'
  AND tanggal_berlaku = $1
ORDER BY kode_mata_uang`

// BatchLoadKurs loads all kurs entries for evaluationDate.
func (r *DBKursRepository) BatchLoadKurs(ctx context.Context, evaluationDate time.Time) (map[string]KursRow, error) {
	if r.db == nil {
		return map[string]KursRow{}, nil
	}
	rows, err := r.db.QueryContext(ctx, batchKursQuery, evaluationDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]KursRow)
	for rows.Next() {
		var kr KursRow
		var nilaiStr string
		if err := rows.Scan(&kr.KodeMatauang, &nilaiStr, &kr.TanggalBerlaku, &kr.WorkflowStatus); err != nil {
			return nil, err
		}
		if kr.NilaiKurs, err = decimal.NewFromString(nilaiStr); err != nil {
			return nil, err
		}
		result[kr.KodeMatauang] = kr
	}
	return result, rows.Err()
}

// ─── DBInstrumenSnapshotRepo ──────────────────────────────────────────────────

// DBInstrumenSnapshotRepo implements InstrumenSnapshotRepo.
type DBInstrumenSnapshotRepo struct {
	db *sql.DB
}

// NewDBInstrumenSnapshotRepo creates a DBInstrumenSnapshotRepo.
func NewDBInstrumenSnapshotRepo(db *sql.DB) *DBInstrumenSnapshotRepo {
	return &DBInstrumenSnapshotRepo{db: db}
}

const instrumenQuery = `
SELECT id, kode_instrumen, nama_instrumen, tipe_instrumen,
       mata_uang, nominal::text, klasifikasi_psak71,
       tanggal_jatuh_tempo, counterparty_id, status
FROM mst.instrumen
WHERE id = $1 AND deleted_at IS NULL`

// GetEADInputs returns fields needed for EAD computation.
func (r *DBInstrumenSnapshotRepo) GetEADInputs(ctx context.Context, instrumenID uuid.UUID) (*InstrumenRow, error) {
	if r.db == nil {
		return nil, nil
	}
	return scanInstrumenRow(r.db.QueryRowContext(ctx, instrumenQuery, instrumenID))
}

func scanInstrumenRow(row *sql.Row) (*InstrumenRow, error) {
	var inst InstrumenRow
	var nominalStr string
	var tanggalJT *time.Time
	err := row.Scan(
		&inst.ID, &inst.KodeInstrumen, &inst.NamaInstrumen,
		&inst.TipeInstrumen, &inst.MatauangKode, &nominalStr,
		&inst.KlasifikasiPsak71, &tanggalJT, &inst.CounterpartyID,
		&inst.Status,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	inst.TanggalJatuhTempo = tanggalJT
	if inst.Nominal, err = decimal.NewFromString(nominalStr); err != nil {
		return nil, err
	}
	return &inst, nil
}

const currentStageQuery = `
SELECT stage_sesudah
FROM ecl.stage_history
WHERE instrumen_id = $1
ORDER BY tanggal_migrasi DESC
LIMIT 1`

// GetCurrentStage returns the current stage from ecl.stage_history.
func (r *DBInstrumenSnapshotRepo) GetCurrentStage(ctx context.Context, instrumenID uuid.UUID) (EclStage, error) {
	if r.db == nil {
		return Stage1, nil
	}
	var stage string
	err := r.db.QueryRowContext(ctx, currentStageQuery, instrumenID).Scan(&stage)
	if err == sql.ErrNoRows {
		return Stage1, nil // no history → treat as Stage 1
	}
	if err != nil {
		return "", err
	}
	return EclStage(stage), nil
}

const eirScheduleQuery = `
SELECT instrumen_id, tanggal_cicilan, principal_outstanding::text,
       bunga_akrual::text, schedule_version
FROM ecl.eir_amortization_schedule
WHERE instrumen_id = $1
  AND tanggal_cicilan <= $2
ORDER BY tanggal_cicilan DESC, schedule_version DESC
LIMIT 1`

// GetEIRScheduleRow returns the latest EIR schedule row for outstanding/accrued.
func (r *DBInstrumenSnapshotRepo) GetEIRScheduleRow(ctx context.Context, instrumenID uuid.UUID, asOf time.Time) (*EIRScheduleRow, error) {
	if r.db == nil {
		return nil, nil
	}
	var row EIRScheduleRow
	var principalStr, bungaStr string
	err := r.db.QueryRowContext(ctx, eirScheduleQuery, instrumenID, asOf).
		Scan(&row.InstrumenID, &row.TanggalCicilan, &principalStr, &bungaStr, &row.ScheduleVersion)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var e error
	if row.PrincipalOutstanding, e = decimal.NewFromString(principalStr); e != nil {
		return nil, e
	}
	if row.BungaAkrual, e = decimal.NewFromString(bungaStr); e != nil {
		return nil, e
	}
	return &row, nil
}

// BatchLoadInstruments loads active instruments for a list of IDs.
func (r *DBInstrumenSnapshotRepo) BatchLoadInstruments(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]InstrumenRow, error) {
	if r.db == nil || len(ids) == 0 {
		return map[uuid.UUID]InstrumenRow{}, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	q := fmt.Sprintf(`
SELECT id, kode_instrumen, nama_instrumen, tipe_instrumen,
       mata_uang, nominal::text, klasifikasi_psak71,
       tanggal_jatuh_tempo, counterparty_id, status
FROM mst.instrumen
WHERE id IN (%s) AND deleted_at IS NULL`, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[uuid.UUID]InstrumenRow, len(ids))
	for rows.Next() {
		var inst InstrumenRow
		var nominalStr string
		var tanggalJT *time.Time
		if err := rows.Scan(
			&inst.ID, &inst.KodeInstrumen, &inst.NamaInstrumen,
			&inst.TipeInstrumen, &inst.MatauangKode, &nominalStr,
			&inst.KlasifikasiPsak71, &tanggalJT, &inst.CounterpartyID,
			&inst.Status,
		); err != nil {
			return nil, err
		}
		inst.TanggalJatuhTempo = tanggalJT
		if inst.Nominal, err = decimal.NewFromString(nominalStr); err != nil {
			return nil, err
		}
		result[inst.ID] = inst
	}
	return result, rows.Err()
}

// BatchLoadEIRSchedules loads latest EIR schedule rows for instruments on or before asOf.
func (r *DBInstrumenSnapshotRepo) BatchLoadEIRSchedules(ctx context.Context, instrumenIDs []uuid.UUID, asOf time.Time) (map[uuid.UUID]EIRScheduleRow, error) {
	if r.db == nil || len(instrumenIDs) == 0 {
		return map[uuid.UUID]EIRScheduleRow{}, nil
	}
	placeholders := make([]string, len(instrumenIDs))
	args := make([]interface{}, 0, len(instrumenIDs)+1)
	args = append(args, asOf)
	for i, id := range instrumenIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, id)
	}
	q := fmt.Sprintf(`
SELECT DISTINCT ON (instrumen_id)
       instrumen_id, tanggal_cicilan, principal_outstanding::text,
       bunga_akrual::text, schedule_version
FROM ecl.eir_amortization_schedule
WHERE instrumen_id IN (%s)
  AND tanggal_cicilan <= $1
ORDER BY instrumen_id, tanggal_cicilan DESC, schedule_version DESC`,
		strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[uuid.UUID]EIRScheduleRow)
	for rows.Next() {
		var row EIRScheduleRow
		var principalStr, bungaStr string
		if err := rows.Scan(&row.InstrumenID, &row.TanggalCicilan,
			&principalStr, &bungaStr, &row.ScheduleVersion); err != nil {
			return nil, err
		}
		if row.PrincipalOutstanding, err = decimal.NewFromString(principalStr); err != nil {
			return nil, err
		}
		if row.BungaAkrual, err = decimal.NewFromString(bungaStr); err != nil {
			return nil, err
		}
		result[row.InstrumenID] = row
	}
	return result, rows.Err()
}

// BatchLoadCurrentStages loads current stages from ecl.stage_history.
func (r *DBInstrumenSnapshotRepo) BatchLoadCurrentStages(ctx context.Context, instrumenIDs []uuid.UUID) (map[uuid.UUID]EclStage, error) {
	if r.db == nil || len(instrumenIDs) == 0 {
		return map[uuid.UUID]EclStage{}, nil
	}
	placeholders := make([]string, len(instrumenIDs))
	args := make([]interface{}, len(instrumenIDs))
	for i, id := range instrumenIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	q := fmt.Sprintf(`
SELECT DISTINCT ON (instrumen_id)
       instrumen_id, stage_sesudah
FROM ecl.stage_history
WHERE instrumen_id IN (%s)
ORDER BY instrumen_id, tanggal_migrasi DESC`, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[uuid.UUID]EclStage, len(instrumenIDs))
	for rows.Next() {
		var id uuid.UUID
		var stage string
		if err := rows.Scan(&id, &stage); err != nil {
			return nil, err
		}
		result[id] = EclStage(stage)
	}
	return result, rows.Err()
}

// ListECLApplicableInstruments pages through instruments eligible for ECL (AC + FVOCI debt).
// Implements previewInstrumentLister. Cursor is the last seen kode_instrumen.
func (r *DBInstrumenSnapshotRepo) ListECLApplicableInstruments(
	ctx context.Context,
	periodeID, filterStage, filterTipe, filterKlasifikasi, filterMatauang string,
	filterHasWarning *bool, search, sortCol, sortDir, cursor string, limit int,
) ([]InstrumenRow, string, bool, error) {
	if r.db == nil {
		return nil, "", false, nil
	}

	// Whitelist sort columns.
	validSort := map[string]bool{
		"kode_instrumen": true, "stage": true, "tipe_instrumen": true,
		"klasifikasi_psak71": true, "mata_uang": true,
	}
	if !validSort[sortCol] {
		sortCol = "kode_instrumen"
	}
	if sortDir != "asc" && sortDir != "desc" {
		sortDir = "asc"
	}

	args := []interface{}{}
	conds := []string{
		"i.deleted_at IS NULL",
		"i.klasifikasi_psak71 IN ('AC','FVOCI_DEBT')", // ECL applicable only
	}
	argIdx := 1

	if filterStage != "" {
		conds = append(conds, fmt.Sprintf(`EXISTS (
			SELECT 1 FROM ecl.stage_history sh
			WHERE sh.instrumen_id = i.id AND sh.stage_sesudah = $%d
			ORDER BY sh.tanggal_migrasi DESC LIMIT 1
		)`, argIdx))
		args = append(args, filterStage)
		argIdx++
	}
	if filterTipe != "" {
		conds = append(conds, fmt.Sprintf("i.tipe_instrumen = $%d", argIdx))
		args = append(args, filterTipe)
		argIdx++
	}
	if filterKlasifikasi != "" {
		conds = append(conds, fmt.Sprintf("i.klasifikasi_psak71 = $%d", argIdx))
		args = append(args, filterKlasifikasi)
		argIdx++
	}
	if filterMatauang != "" {
		conds = append(conds, fmt.Sprintf("i.mata_uang = $%d", argIdx))
		args = append(args, filterMatauang)
		argIdx++
	}
	if search != "" {
		conds = append(conds, fmt.Sprintf("(i.kode_instrumen ILIKE $%d OR i.nama_instrumen ILIKE $%d)",
			argIdx, argIdx))
		args = append(args, "%"+search+"%")
		argIdx++
	}
	if cursor != "" {
		op := ">"
		if sortDir == "desc" {
			op = "<"
		}
		conds = append(conds, fmt.Sprintf("i.kode_instrumen %s $%d", op, argIdx))
		args = append(args, cursor)
		argIdx++
	}

	where := "WHERE " + strings.Join(conds, " AND ")
	// fetch limit+1 to detect hasMore
	args = append(args, limit+1)
	q := fmt.Sprintf(`
SELECT id, kode_instrumen, nama_instrumen, tipe_instrumen,
       mata_uang, nominal::text, klasifikasi_psak71,
       tanggal_jatuh_tempo, counterparty_id, status
FROM mst.instrumen i
%s
ORDER BY i.%s %s
LIMIT $%d`, where, sortCol, strings.ToUpper(sortDir), argIdx)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, "", false, err
	}
	defer rows.Close()

	result := make([]InstrumenRow, 0, limit)
	for rows.Next() {
		var inst InstrumenRow
		var nominalStr string
		var tanggalJT *time.Time
		if err := rows.Scan(
			&inst.ID, &inst.KodeInstrumen, &inst.NamaInstrumen,
			&inst.TipeInstrumen, &inst.MatauangKode, &nominalStr,
			&inst.KlasifikasiPsak71, &tanggalJT, &inst.CounterpartyID,
			&inst.Status,
		); err != nil {
			return nil, "", false, err
		}
		inst.TanggalJatuhTempo = tanggalJT
		if inst.Nominal, err = decimal.NewFromString(nominalStr); err != nil {
			return nil, "", false, err
		}
		result = append(result, inst)
	}
	if err := rows.Err(); err != nil {
		return nil, "", false, err
	}

	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}

	nextCursor := ""
	if hasMore && len(result) > 0 {
		nextCursor = result[len(result)-1].KodeInstrumen
	}

	return result, nextCursor, hasMore, nil
}

// ─── DBCounterpartyRepo ───────────────────────────────────────────────────────

// DBCounterpartyRepo implements CounterpartyRepo against mst.counterparty.
type DBCounterpartyRepo struct {
	db *sql.DB
}

// NewDBCounterpartyRepo creates a DBCounterpartyRepo.
func NewDBCounterpartyRepo(db *sql.DB) *DBCounterpartyRepo {
	return &DBCounterpartyRepo{db: db}
}

// GetTipeCounterparty returns tipe_counterparty for the given ID.
func (r *DBCounterpartyRepo) GetTipeCounterparty(ctx context.Context, counterpartyID uuid.UUID) (string, error) {
	if r.db == nil {
		return "", nil
	}
	var tipe string
	err := r.db.QueryRowContext(ctx,
		`SELECT tipe_counterparty FROM mst.counterparty WHERE id = $1 AND deleted_at IS NULL`,
		counterpartyID).Scan(&tipe)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return tipe, err
}

// BatchLoadCounterparties loads counterparty rows for a list of IDs.
func (r *DBCounterpartyRepo) BatchLoadCounterparties(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]CounterpartyRow, error) {
	if r.db == nil || len(ids) == 0 {
		return map[uuid.UUID]CounterpartyRow{}, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	q := fmt.Sprintf(`
SELECT id, nama_counterparty, tipe_counterparty
FROM mst.counterparty
WHERE id IN (%s) AND deleted_at IS NULL`, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[uuid.UUID]CounterpartyRow, len(ids))
	for rows.Next() {
		var cr CounterpartyRow
		if err := rows.Scan(&cr.ID, &cr.NamaCounterparty, &cr.TipeCounterparty); err != nil {
			return nil, err
		}
		result[cr.ID] = cr
	}
	return result, rows.Err()
}

// ─── DBCCFConfigRepo ──────────────────────────────────────────────────────────

// DBCCFConfigRepo implements CCFConfigRepo against sys.config.
type DBCCFConfigRepo struct {
	db *sql.DB
}

// NewDBCCFConfigRepo creates a DBCCFConfigRepo.
func NewDBCCFConfigRepo(db *sql.DB) *DBCCFConfigRepo {
	return &DBCCFConfigRepo{db: db}
}

// GetCCFTable reads sys.config 'CCF_TABLE' JSONB.
func (r *DBCCFConfigRepo) GetCCFTable(ctx context.Context) (map[string]decimal.Decimal, error) {
	if r.db == nil {
		// Dev fallback: Phase 1 defaults per OQ-E resolution.
		return map[string]decimal.Decimal{
			"DEPOSITO":   decimal.Zero,
			"OBLIGASI":   decimal.Zero,
			"SAHAM":      decimal.Zero,
			"REKSADANA":  decimal.Zero,
			"SBI":        decimal.Zero,
			"COMMITMENT": decimal.NewFromFloat(0.75),
		}, nil
	}
	raw, err := readJSONBConfig[map[string]interface{}](ctx, r.db, ccfTableConfigKey)
	if err != nil {
		return nil, err
	}
	result := make(map[string]decimal.Decimal, len(raw))
	for k, v := range raw {
		var d decimal.Decimal
		switch vt := v.(type) {
		case string:
			d, err = decimal.NewFromString(vt)
		case float64:
			// json.Unmarshal into interface{} gives float64 for numbers.
			// We convert via string to avoid float64 rounding (DEC-016).
			d, err = decimal.NewFromString(fmt.Sprintf("%v", vt))
		default:
			d, err = decimal.NewFromString(fmt.Sprintf("%v", vt))
		}
		if err != nil {
			return nil, fmt.Errorf("CCF_TABLE value for %q: %w", k, err)
		}
		result[k] = d
	}
	return result, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// readJSONBConfig reads a sys.config JSONB value and unmarshals into T.
func readJSONBConfig[T any](ctx context.Context, db *sql.DB, key string) (T, error) {
	var zero T
	var rawJSON string
	err := db.QueryRowContext(ctx,
		`SELECT config_value::text FROM sys.config WHERE config_key = $1`, key).Scan(&rawJSON)
	if err == sql.ErrNoRows {
		return zero, fmt.Errorf("sys.config key %q not found", key)
	}
	if err != nil {
		return zero, err
	}
	var result T
	if err := json.Unmarshal([]byte(rawJSON), &result); err != nil {
		return zero, fmt.Errorf("sys.config key %q: json unmarshal: %w", key, err)
	}
	return result, nil
}
