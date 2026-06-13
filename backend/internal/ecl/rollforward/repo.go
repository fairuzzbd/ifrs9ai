package rollforward

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// repo.go — read-only repository for ecl.calc_result_line, ecl.stage_history,
// ecl.calc_run, mst.instrumen, mst.portofolio.
//
// Rules enforced here:
//   - Read-only: no INSERT/UPDATE/DELETE (DEC-018: ecl.* no hard delete).
//   - No float64: all NUMERIC values parsed via decimal.NewFromString (DEC-016).
//   - Allowlisted columns for sort — no string concat in WHERE/ORDER BY.
//
// M11 reads from M7 result lines as source; does NOT re-compute ECL.

// Repo is the read-only data access layer for roll-forward computation.
type Repo struct {
	db *sql.DB
}

// NewRepo creates a Repo. Panics if db is nil.
func NewRepo(db *sql.DB) *Repo {
	if db == nil {
		panic("rollforward.NewRepo: db must not be nil")
	}
	return &Repo{db: db}
}

// GetResultLinesByCalcRun loads all result lines for a calc run.
// Returns the minimal projection needed by roll-forward: instrumen_id, stage,
// ecl_weighted_idr, ead_idr. POCI rows (ecl_weighted_idr IS NULL) are included
// with nil EclWeightedIdr — roll-forward excludes them from sums per §4 doc.
func (r *Repo) GetResultLinesByCalcRun(ctx context.Context, calcRunID uuid.UUID) ([]ResultLineHeader, error) {
	const q = `
SELECT instrumen_id, stage, ecl_weighted_idr, ead_idr
FROM ecl.calc_result_line
WHERE calc_run_id = $1
  AND deleted_at IS NULL
ORDER BY instrumen_id`

	rows, err := r.db.QueryContext(ctx, q, calcRunID)
	if err != nil {
		return nil, fmt.Errorf("rollforward.GetResultLinesByCalcRun(%s): %w", calcRunID, err)
	}
	defer rows.Close() //nolint:errcheck

	var results []ResultLineHeader
	for rows.Next() {
		var (
			instrumenID    uuid.UUID
			stage          int
			eclWeightedRaw sql.NullString
			eadRaw         string
		)
		if err := rows.Scan(&instrumenID, &stage, &eclWeightedRaw, &eadRaw); err != nil {
			return nil, fmt.Errorf("rollforward.GetResultLinesByCalcRun scan: %w", err)
		}

		ead, err := decimal.NewFromString(eadRaw)
		if err != nil {
			return nil, fmt.Errorf("rollforward.GetResultLinesByCalcRun: parse ead_idr %q: %w", eadRaw, err)
		}

		h := ResultLineHeader{
			InstrumenID: instrumenID,
			Stage:       stage,
			EadIdr:      ead,
		}
		if eclWeightedRaw.Valid {
			d, err := decimal.NewFromString(eclWeightedRaw.String)
			if err != nil {
				return nil, fmt.Errorf("rollforward.GetResultLinesByCalcRun: parse ecl_weighted_idr %q: %w", eclWeightedRaw.String, err)
			}
			h.EclWeightedIdr = &d
		}
		results = append(results, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rollforward.GetResultLinesByCalcRun rows: %w", err)
	}
	return results, nil
}

// GetResultLinesByCalcRunAndPortfolio loads result lines filtered by portfolio.
// Used by GetPortfolioRollForward.
func (r *Repo) GetResultLinesByCalcRunAndPortfolio(ctx context.Context, calcRunID, portofolioID uuid.UUID) ([]ResultLineHeader, error) {
	const q = `
SELECT crl.instrumen_id, crl.stage, crl.ecl_weighted_idr, crl.ead_idr
FROM ecl.calc_result_line crl
JOIN mst.instrumen i ON i.id = crl.instrumen_id
WHERE crl.calc_run_id = $1
  AND i.portofolio_id = $2
  AND crl.deleted_at IS NULL
ORDER BY crl.instrumen_id`

	rows, err := r.db.QueryContext(ctx, q, calcRunID, portofolioID)
	if err != nil {
		return nil, fmt.Errorf("rollforward.GetResultLinesByCalcRunAndPortfolio(%s,%s): %w", calcRunID, portofolioID, err)
	}
	defer rows.Close() //nolint:errcheck

	var results []ResultLineHeader
	for rows.Next() {
		var (
			instrumenID    uuid.UUID
			stage          int
			eclWeightedRaw sql.NullString
			eadRaw         string
		)
		if err := rows.Scan(&instrumenID, &stage, &eclWeightedRaw, &eadRaw); err != nil {
			return nil, fmt.Errorf("rollforward.GetResultLinesByCalcRunAndPortfolio scan: %w", err)
		}
		ead, err := decimal.NewFromString(eadRaw)
		if err != nil {
			return nil, fmt.Errorf("rollforward.GetResultLinesByCalcRunAndPortfolio: parse ead_idr %q: %w", eadRaw, err)
		}
		h := ResultLineHeader{InstrumenID: instrumenID, Stage: stage, EadIdr: ead}
		if eclWeightedRaw.Valid {
			d, err := decimal.NewFromString(eclWeightedRaw.String)
			if err != nil {
				return nil, fmt.Errorf("rollforward.GetResultLinesByCalcRunAndPortfolio: parse ecl_weighted %q: %w", eclWeightedRaw.String, err)
			}
			h.EclWeightedIdr = &d
		}
		results = append(results, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rollforward.GetResultLinesByCalcRunAndPortfolio rows: %w", err)
	}
	return results, nil
}

// GetCalcRunStatus returns the status and periode_id of a calc run.
// Returns ("", "", false, nil) when not found.
func (r *Repo) GetCalcRunStatus(ctx context.Context, calcRunID uuid.UUID) (status string, periodeID string, found bool, err error) {
	const q = `SELECT status, periode_id FROM ecl.calc_run WHERE id = $1 AND deleted_at IS NULL`
	var s, p string
	e := r.db.QueryRowContext(ctx, q, calcRunID).Scan(&s, &p)
	if e == sql.ErrNoRows {
		return "", "", false, nil
	}
	if e != nil {
		return "", "", false, fmt.Errorf("rollforward.GetCalcRunStatus(%s): %w", calcRunID, e)
	}
	return s, p, true, nil
}

// GetSealedCalcRunsByPeriode returns the last N SEALED calc runs, ordered by sealed_at ASC.
// Used by GetCKPNTrend.
func (r *Repo) GetSealedCalcRunsByPeriode(ctx context.Context, limit int) ([]CalcRunSummary, error) {
	const q = `
SELECT id, periode_id, status, sealed_at, tenant_id
FROM ecl.calc_run
WHERE status = 'SEALED'
  AND deleted_at IS NULL
ORDER BY sealed_at ASC
LIMIT $1`

	rows, err := r.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("rollforward.GetSealedCalcRunsByPeriode: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var results []CalcRunSummary
	for rows.Next() {
		var (
			id        uuid.UUID
			periodeID string
			status    string
			sealedAt  sql.NullTime
			tenantID  string
		)
		if err := rows.Scan(&id, &periodeID, &status, &sealedAt, &tenantID); err != nil {
			return nil, fmt.Errorf("rollforward.GetSealedCalcRunsByPeriode scan: %w", err)
		}
		cr := CalcRunSummary{
			ID:        id,
			PeriodeID: periodeID,
			Status:    status,
			TenantID:  tenantID,
		}
		if sealedAt.Valid {
			t := sealedAt.Time
			cr.SealedAt = &t
		}
		results = append(results, cr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rollforward.GetSealedCalcRunsByPeriode rows: %w", err)
	}
	return results, nil
}

// GetECLByStageForCalcRun returns ECL grouped by stage for one calc run.
// Used by trend dashboard ECL per-stage breakdown.
func (r *Repo) GetECLByStageForCalcRun(ctx context.Context, calcRunID uuid.UUID) (EclByStage, error) {
	const q = `
SELECT stage, COALESCE(SUM(ecl_weighted_idr::NUMERIC), 0)
FROM ecl.calc_result_line
WHERE calc_run_id = $1
  AND deleted_at IS NULL
  AND ecl_weighted_idr IS NOT NULL
GROUP BY stage`

	rows, err := r.db.QueryContext(ctx, q, calcRunID)
	if err != nil {
		return EclByStage{}, fmt.Errorf("rollforward.GetECLByStageForCalcRun(%s): %w", calcRunID, err)
	}
	defer rows.Close() //nolint:errcheck

	var result EclByStage
	for rows.Next() {
		var stage int
		var eclStr string
		if err := rows.Scan(&stage, &eclStr); err != nil {
			return EclByStage{}, fmt.Errorf("rollforward.GetECLByStageForCalcRun scan: %w", err)
		}
		d, err := decimal.NewFromString(eclStr)
		if err != nil {
			return EclByStage{}, fmt.Errorf("rollforward.GetECLByStageForCalcRun: parse ecl %q: %w", eclStr, err)
		}
		switch stage {
		case 1:
			result.Stage1 = d
		case 2:
			result.Stage2 = d
		case 3:
			result.Stage3 = d
		}
	}
	if err := rows.Err(); err != nil {
		return EclByStage{}, fmt.Errorf("rollforward.GetECLByStageForCalcRun rows: %w", err)
	}
	return result, nil
}

// GetStageHistoryForCalcRun loads the latest stage_history entry per instrument
// for a given calc run. Used for override flag detection (MANAGEMENT_OVERRIDE).
func (r *Repo) GetStageHistoryForCalcRun(ctx context.Context, calcRunID uuid.UUID) (map[uuid.UUID]StageHistoryRow, error) {
	// DISTINCT ON instrumen_id, ordered by created_at DESC → latest entry per instrument.
	const q = `
SELECT DISTINCT ON (instrumen_id)
       instrumen_id, calc_run_id, trigger_type, created_at
FROM ecl.stage_history
WHERE calc_run_id = $1
ORDER BY instrumen_id, created_at DESC`

	rows, err := r.db.QueryContext(ctx, q, calcRunID)
	if err != nil {
		return nil, fmt.Errorf("rollforward.GetStageHistoryForCalcRun(%s): %w", calcRunID, err)
	}
	defer rows.Close() //nolint:errcheck

	result := make(map[uuid.UUID]StageHistoryRow)
	for rows.Next() {
		var (
			instrumenID  uuid.UUID
			calcRunIDRow uuid.UUID
			triggerType  string
			createdAt    time.Time
		)
		if err := rows.Scan(&instrumenID, &calcRunIDRow, &triggerType, &createdAt); err != nil {
			return nil, fmt.Errorf("rollforward.GetStageHistoryForCalcRun scan: %w", err)
		}
		result[instrumenID] = StageHistoryRow{
			InstrumenID: instrumenID,
			CalcRunID:   calcRunIDRow,
			TriggerType: triggerType,
			CreatedAt:   createdAt,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rollforward.GetStageHistoryForCalcRun rows: %w", err)
	}
	return result, nil
}

// GetInstrumenStatusByIDs loads mst.instrumen status snapshots for a set of instrument IDs.
// Used by lifecycle detector for derecognition reason classification.
func (r *Repo) GetInstrumenStatusByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]InstrumenStatusSnapshot, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]InstrumenStatusSnapshot{}, nil
	}

	// Build parameterized IN clause — allowlisted column set, no user input in SQL.
	args := make([]interface{}, len(ids))
	placeholders := make([]byte, 0, len(ids)*5)
	for i, id := range ids {
		args[i] = id
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '$')
		for _, c := range fmt.Sprintf("%d", i+1) {
			placeholders = append(placeholders, byte(c))
		}
	}

	//nolint:gosec // placeholders built from parameterized index numbers only
	q := fmt.Sprintf(`
SELECT id, kode, status, tanggal_jatuh_tempo
FROM mst.instrumen
WHERE id IN (%s)
  AND deleted_at IS NULL`, string(placeholders))

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("rollforward.GetInstrumenStatusByIDs: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	result := make(map[uuid.UUID]InstrumenStatusSnapshot, len(ids))
	for rows.Next() {
		var (
			id            uuid.UUID
			kode          string
			status        string
			jatuhTempoRaw sql.NullTime
		)
		if err := rows.Scan(&id, &kode, &status, &jatuhTempoRaw); err != nil {
			return nil, fmt.Errorf("rollforward.GetInstrumenStatusByIDs scan: %w", err)
		}
		snap := InstrumenStatusSnapshot{ID: id, Kode: kode, Status: status}
		if jatuhTempoRaw.Valid {
			t := jatuhTempoRaw.Time
			snap.TanggalJatuhTempo = &t
		}
		result[id] = snap
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rollforward.GetInstrumenStatusByIDs rows: %w", err)
	}
	return result, nil
}

// GetPeriodeTanggalMulai returns the tanggal_mulai (DATE) for a periode_id string.
// Used by validatePeriodeOrdering to enforce temporal ordering via real DB dates
// rather than lexicographic string comparison (F1 — FSD-APP-C §5.1).
// Returns time.Time{} zero value and false when not found.
func (r *Repo) GetPeriodeTanggalMulai(ctx context.Context, periodeID string) (time.Time, bool, error) {
	const q = `SELECT tanggal_mulai FROM mst.periode_buku WHERE id = $1 AND deleted_at IS NULL`
	var t time.Time
	err := r.db.QueryRowContext(ctx, q, periodeID).Scan(&t)
	if err == sql.ErrNoRows {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("rollforward.GetPeriodeTanggalMulai(%q): %w", periodeID, err)
	}
	return t, true, nil
}

// GetPortofolioNama returns the nama of a portfolio. Returns "", false, nil if not found.
func (r *Repo) GetPortofolioNama(ctx context.Context, portofolioID uuid.UUID) (string, bool, error) {
	const q = `SELECT nama FROM mst.portofolio WHERE id = $1 AND deleted_at IS NULL`
	var nama string
	err := r.db.QueryRowContext(ctx, q, portofolioID).Scan(&nama)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("rollforward.GetPortofolioNama(%s): %w", portofolioID, err)
	}
	return nama, true, nil
}

// GetPortofolioInstruments returns all instrument IDs for a portfolio.
func (r *Repo) GetPortofolioInstruments(ctx context.Context, portofolioID uuid.UUID) ([]uuid.UUID, error) {
	const q = `
SELECT id FROM mst.instrumen
WHERE portofolio_id = $1
  AND deleted_at IS NULL
ORDER BY id`

	rows, err := r.db.QueryContext(ctx, q, portofolioID)
	if err != nil {
		return nil, fmt.Errorf("rollforward.GetPortofolioInstruments(%s): %w", portofolioID, err)
	}
	defer rows.Close() //nolint:errcheck

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("rollforward.GetPortofolioInstruments scan: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rollforward.GetPortofolioInstruments rows: %w", err)
	}
	return ids, nil
}
