package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// repo.go — CalcResultLineRepo: persistence layer for ecl.calc_result_line.
//
// Rules enforced here:
//   - NO hard delete (DB trigger trg_ecl_calc_no_delete_result_line enforces; we comply).
//   - NO UPDATE of sealed rows (sealed_at IS NOT NULL); attempts return ECL_CALC_RUN_SEALED.
//   - All IDR stored as NUMERIC(20,4) via .StringFixed(4) → text form, no float64.
//   - PD/LGD/FL stored as NUMERIC(10,8) via .StringFixed(8).
//   - Bobot stored as NUMERIC(7,4).
//
// Migration: db/migrations/000029_ecl_core_tables.up.sql

// CalcResultLineRepo handles ecl.calc_result_line persistence.
type CalcResultLineRepo struct {
	db *sql.DB
}

// NewCalcResultLineRepo creates a new repo. Panics if db is nil.
func NewCalcResultLineRepo(db *sql.DB) *CalcResultLineRepo {
	if db == nil {
		panic("core.NewCalcResultLineRepo: db must not be nil")
	}
	return &CalcResultLineRepo{db: db}
}

// InsertResultLine inserts one ecl.calc_result_line row within the given transaction.
// The row must not already exist (UNIQUE: calc_run_id + instrumen_id).
// No float64. All decimals serialized as text for numeric columns.
// F8 fix: includes formula_version column (migration 000030).
func (r *CalcResultLineRepo) InsertResultLine(ctx context.Context, tx *sql.Tx, row ResultLineRow) error {
	// Default formula version if not set.
	fv := row.FormulaVersion
	if fv == "" {
		fv = FormulaVersionM7
	}

	q := `
INSERT INTO ecl.calc_result_line (
	id, calc_run_id, instrumen_id, evaluation_date, periode_id, stage, routing_path,
	ead_idr,
	pd_used_good, pd_used_normal, pd_used_bad,
	lgd_used,
	fl_multiplier_good, fl_multiplier_normal, fl_multiplier_bad,
	ecl_good_idr, ecl_normal_idr, ecl_bad_idr,
	ecl_fl_good_idr, ecl_fl_normal_idr, ecl_fl_bad_idr,
	ecl_weighted_idr,
	bobot_good, bobot_normal, bobot_bad,
	net_carrying_idr, prior_sealed_ecl_idr,
	flag_poci, parameter_snapshot_id, warnings_json,
	formula_version,
	created_by, updated_by, tenant_id
) VALUES (
	$1,$2,$3,$4,$5,$6,$7,
	$8,
	$9,$10,$11,
	$12,
	$13,$14,$15,
	$16,$17,$18,
	$19,$20,$21,
	$22,
	$23,$24,$25,
	$26,$27,
	$28,$29,$30,
	$31,
	$32,$32,'TUGURE'
)`

	warningsJSON, err := marshalWarnings(row.Warnings)
	if err != nil {
		return fmt.Errorf("core.InsertResultLine: marshal warnings: %w", err)
	}

	_, err = tx.ExecContext(ctx, q,
		row.ID,
		row.CalcRunID,
		row.InstrumenID,
		row.EvaluationDate,
		row.PeriodeID,
		int(row.Stage),
		string(row.RoutingPath),
		row.EADIDR.StringFixed(4),
		decimalPtrStr8(row.PDGood),
		decimalPtrStr8(row.PDNormal),
		decimalPtrStr8(row.PDBad),
		decimalPtrStr8(row.LGDUsed),
		decimalPtrStr8(row.FLGood),
		decimalPtrStr8(row.FLNormal),
		decimalPtrStr8(row.FLBad),
		row.ECLGoodIDR.StringFixed(4),
		row.ECLNormalIDR.StringFixed(4),
		row.ECLBadIDR.StringFixed(4),
		row.ECLFLGoodIDR.StringFixed(4),
		row.ECLFLNormalIDR.StringFixed(4),
		row.ECLFLBadIDR.StringFixed(4),
		decimalPtrStr4(row.ECLWeightedIDR), // NULL for POCI
		row.BobotGood.StringFixed(4),
		row.BobotNormal.StringFixed(4),
		row.BobotBad.StringFixed(4),
		decimalPtrStr4(row.NetCarryingIDR),
		decimalPtrStr4(row.PriorSealedECLIDR),
		row.FlagPOCI,
		row.ParameterSnapshotID,
		warningsJSON,
		fv,
		row.ActorID,
	)
	return err
}

// GetPriorSealedECL returns the ecl_weighted_idr from the most recent SEALED calc_result_line
// for instrumenID. Returns nil if no sealed row exists (first run).
// Query: SELECT ecl_weighted_idr FROM ecl.calc_result_line
//
//	WHERE instrumen_id = $1 AND sealed_at IS NOT NULL
//	ORDER BY evaluation_date DESC LIMIT 1
//
// Per OQ-M7-3 (BLOCKING resolution).
func (r *CalcResultLineRepo) GetPriorSealedECL(ctx context.Context, instrumenID uuid.UUID) (*decimal.Decimal, error) {
	q := `
SELECT ecl_weighted_idr
FROM ecl.calc_result_line
WHERE instrumen_id = $1
  AND sealed_at IS NOT NULL
  AND deleted_at IS NULL
ORDER BY evaluation_date DESC
LIMIT 1`

	var raw sql.NullString
	err := r.db.QueryRowContext(ctx, q, instrumenID).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("core.GetPriorSealedECL(%s): %w", instrumenID, err)
	}
	if !raw.Valid {
		return nil, nil
	}
	d, err := decimal.NewFromString(raw.String)
	if err != nil {
		return nil, fmt.Errorf("core.GetPriorSealedECL: parse decimal %q: %w", raw.String, err)
	}
	return &d, nil
}

// GetResultLine returns one ecl.calc_result_line by (calcRunID, instrumenID).
// Returns nil, nil when not found.
// F8 fix: includes formula_version column (migration 000030).
func (r *CalcResultLineRepo) GetResultLine(ctx context.Context, calcRunID, instrumenID uuid.UUID) (*ResultLineRow, error) {
	q := `
SELECT id, calc_run_id, instrumen_id, evaluation_date, periode_id, stage, routing_path,
       ead_idr, pd_used_good, pd_used_normal, pd_used_bad, lgd_used,
       fl_multiplier_good, fl_multiplier_normal, fl_multiplier_bad,
       ecl_good_idr, ecl_normal_idr, ecl_bad_idr,
       ecl_fl_good_idr, ecl_fl_normal_idr, ecl_fl_bad_idr,
       ecl_weighted_idr, bobot_good, bobot_normal, bobot_bad,
       net_carrying_idr, prior_sealed_ecl_idr, flag_poci, parameter_snapshot_id,
       warnings_json, sealed_at, created_at,
       COALESCE(formula_version, 'M7-v1.0') AS formula_version
FROM ecl.calc_result_line
WHERE calc_run_id = $1 AND instrumen_id = $2 AND deleted_at IS NULL
LIMIT 1`

	row := r.db.QueryRowContext(ctx, q, calcRunID, instrumenID)
	return scanResultLineRow(row)
}

// ListResultLines returns paginated result lines for a calcRunID.
// Cursor-based pagination per DEC-022.
func (r *CalcResultLineRepo) ListResultLines(ctx context.Context, req ListResultsRequest) (*ListResultsResponse, error) {
	allowed := map[string]bool{
		"evaluation_date": true, "stage": true, "ecl_weighted_idr": true,
		"ead_idr": true, "routing_path": true, "created_at": true,
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	// Build WHERE clause
	conditions := []string{"calc_run_id = $1", "deleted_at IS NULL"}
	args := []interface{}{req.CalcRunID}

	if req.InstrumenID != nil {
		args = append(args, *req.InstrumenID)
		conditions = append(conditions, fmt.Sprintf("instrumen_id = $%d", len(args)))
	}
	if req.Stage != nil {
		args = append(args, int(*req.Stage))
		conditions = append(conditions, fmt.Sprintf("stage = $%d", len(args)))
	}
	if req.RoutingPath != nil {
		args = append(args, string(*req.RoutingPath))
		conditions = append(conditions, fmt.Sprintf("routing_path = $%d", len(args)))
	}
	if req.FlagPOCI != nil {
		args = append(args, *req.FlagPOCI)
		conditions = append(conditions, fmt.Sprintf("flag_poci = $%d", len(args)))
	}

	// Cursor decode
	if req.Cursor != "" {
		args = append(args, req.Cursor)
		conditions = append(conditions, fmt.Sprintf("id > $%d", len(args)))
	}

	// ORDER BY
	orderBy := "created_at ASC, id ASC"
	if len(req.Sort) > 0 {
		parts := make([]string, 0, len(req.Sort))
		for _, s := range req.Sort {
			if !allowed[s.Col] {
				continue
			}
			dir := "ASC"
			if strings.EqualFold(s.Dir, "DESC") {
				dir = "DESC"
			}
			parts = append(parts, s.Col+" "+dir)
		}
		if len(parts) > 0 {
			orderBy = strings.Join(parts, ", ")
		}
	}

	where := strings.Join(conditions, " AND ")
	args = append(args, limit+1)
	//nolint:gosec // orderBy and where are built from validated allowlist columns and parameterized values only
	q := fmt.Sprintf(`
SELECT id, calc_run_id, instrumen_id, evaluation_date, periode_id, stage, routing_path,
       ead_idr, ecl_weighted_idr, flag_poci, sealed_at, created_at
FROM ecl.calc_result_line
WHERE %s
ORDER BY %s
LIMIT $%d`, where, orderBy, len(args))

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("core.ListResultLines: %w", err)
	}
	defer rows.Close()

	var items []ResultLine
	for rows.Next() {
		var (
			id, calcRunID, instrumenID uuid.UUID
			evalDate                   time.Time
			periodeID                  string
			stage                      int
			routingPath                string
			eadRaw, eclRaw             sql.NullString
			flagPOCI                   bool
			sealedAt                   sql.NullTime
			createdAt                  time.Time
		)
		if err := rows.Scan(&id, &calcRunID, &instrumenID, &evalDate, &periodeID,
			&stage, &routingPath, &eadRaw, &eclRaw, &flagPOCI, &sealedAt, &createdAt); err != nil {
			return nil, fmt.Errorf("core.ListResultLines scan: %w", err)
		}
		line := ResultLine{
			ID: id, CalcRunID: calcRunID, InstrumenID: instrumenID,
			EvaluationDate: evalDate, PeriodeID: periodeID,
			Stage: Stage(stage), RoutingPath: RoutingPath(routingPath),
			FlagPOCI: flagPOCI, CreatedAt: createdAt,
		}
		if eadRaw.Valid {
			d, err := decimal.NewFromString(eadRaw.String)
			if err != nil {
				return nil, fmt.Errorf("core.ListResultLines: parse ead_idr %q: %w", eadRaw.String, err)
			}
			line.EADIDR = &d
		}
		if eclRaw.Valid {
			d, err := decimal.NewFromString(eclRaw.String)
			if err != nil {
				return nil, fmt.Errorf("core.ListResultLines: parse ecl_weighted_idr %q: %w", eclRaw.String, err)
			}
			line.ECLWeightedIDR = &d
		}
		if sealedAt.Valid {
			t := sealedAt.Time
			line.SealedAt = &t
		}
		items = append(items, line)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("core.ListResultLines rows: %w", err)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	nextCursor := ""
	if hasMore && len(items) > 0 {
		nextCursor = items[len(items)-1].ID.String()
	}

	return &ListResultsResponse{
		Items:       items,
		NextCursor:  nextCursor,
		HasMore:     hasMore,
		AppliedSort: req.Sort,
	}, nil
}

// GetPortfolioAggregate computes stage-level aggregation for a portfolio + calc run.
func (r *CalcResultLineRepo) GetPortfolioAggregate(ctx context.Context, calcRunID, portofolioID uuid.UUID) ([]StageSummaryRow, error) {
	q := `
SELECT
    CASE stage WHEN 1 THEN 'STAGE_1' WHEN 2 THEN 'STAGE_2' WHEN 3 THEN 'STAGE_3' ELSE 'UNKNOWN' END AS stage_label,
    COUNT(*) AS cnt,
    COALESCE(SUM(ead_idr::NUMERIC), 0) AS ead_total,
    COALESCE(SUM(ecl_weighted_idr::NUMERIC), 0) AS ecl_total
FROM ecl.calc_result_line crl
JOIN mst.instrumen i ON i.id = crl.instrumen_id
WHERE crl.calc_run_id = $1
  AND i.portofolio_id = $2
  AND crl.deleted_at IS NULL
GROUP BY stage
ORDER BY stage`

	rows, err := r.db.QueryContext(ctx, q, calcRunID, portofolioID)
	if err != nil {
		return nil, fmt.Errorf("core.GetPortfolioAggregate: %w", err)
	}
	defer rows.Close()

	var result []StageSummaryRow
	var totalECL decimal.Decimal
	var totalEAD decimal.Decimal
	var totalCount int

	for rows.Next() {
		var stageLabel string
		var cnt int
		var eadStr, eclStr string
		if err := rows.Scan(&stageLabel, &cnt, &eadStr, &eclStr); err != nil {
			return nil, fmt.Errorf("core.GetPortfolioAggregate scan: %w", err)
		}
		ead, err := decimal.NewFromString(eadStr)
		if err != nil {
			return nil, fmt.Errorf("core.GetPortfolioAggregate: parse ead_total %q: %w", eadStr, err)
		}
		ecl, err := decimal.NewFromString(eclStr)
		if err != nil {
			return nil, fmt.Errorf("core.GetPortfolioAggregate: parse ecl_total %q: %w", eclStr, err)
		}
		row := StageSummaryRow{
			Stage: stageLabel, Count: cnt,
			EADTotalIDR: ead, ECLWeightedTotalIDR: ecl,
		}
		result = append(result, row)
		totalECL = totalECL.Add(ecl)
		totalEAD = totalEAD.Add(ead)
		totalCount += cnt
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("core.GetPortfolioAggregate rows: %w", err)
	}

	// Append TOTAL row
	result = append(result, StageSummaryRow{
		Stage: "TOTAL", Count: totalCount,
		EADTotalIDR: totalEAD, ECLWeightedTotalIDR: totalECL,
	})
	return result, nil
}

// GetCalcRunECLTotal returns the SUM of ecl_weighted_idr for a calc run.
// Used by roll-forward reconcile.
func (r *CalcResultLineRepo) GetCalcRunECLTotal(ctx context.Context, calcRunID uuid.UUID) (decimal.Decimal, error) {
	q := `SELECT COALESCE(SUM(ecl_weighted_idr::NUMERIC), 0) FROM ecl.calc_result_line
          WHERE calc_run_id = $1 AND deleted_at IS NULL`
	var raw string
	if err := r.db.QueryRowContext(ctx, q, calcRunID).Scan(&raw); err != nil {
		return decimal.Zero, fmt.Errorf("core.GetCalcRunECLTotal: %w", err)
	}
	d, err := decimal.NewFromString(raw)
	if err != nil {
		return decimal.Zero, err
	}
	return d, nil
}

// ExistsResultLine checks if a result line already exists for (calcRunID, instrumenID).
// Used to prevent duplicate inserts during bulk recompute.
func (r *CalcResultLineRepo) ExistsResultLine(ctx context.Context, calcRunID, instrumenID uuid.UUID) (bool, error) {
	q := `SELECT EXISTS(SELECT 1 FROM ecl.calc_result_line
                        WHERE calc_run_id=$1 AND instrumen_id=$2 AND deleted_at IS NULL)`
	var exists bool
	err := r.db.QueryRowContext(ctx, q, calcRunID, instrumenID).Scan(&exists)
	return exists, err
}

// FormulaVersionM7 is the default formula version tag stored in calc_result_line.formula_version.
// Updated on every algorithm change so historical rows identify the version used.
const FormulaVersionM7 = "M7-v1.0"

// ─── ResultLineRow — full DB row for insert ─────────────────────────────────

// ResultLineRow is the full set of fields for one ecl.calc_result_line insert.
// Follows the migration schema from 000029 + 000030 (formula_version column).
type ResultLineRow struct {
	ID                  uuid.UUID
	CalcRunID           uuid.UUID
	InstrumenID         uuid.UUID
	EvaluationDate      time.Time
	PeriodeID           string
	Stage               Stage
	RoutingPath         RoutingPath
	EADIDR              decimal.Decimal
	PDGood              *decimal.Decimal
	PDNormal            *decimal.Decimal
	PDBad               *decimal.Decimal
	LGDUsed             *decimal.Decimal
	FLGood              *decimal.Decimal
	FLNormal            *decimal.Decimal
	FLBad               *decimal.Decimal
	ECLGoodIDR          decimal.Decimal
	ECLNormalIDR        decimal.Decimal
	ECLBadIDR           decimal.Decimal
	ECLFLGoodIDR        decimal.Decimal
	ECLFLNormalIDR      decimal.Decimal
	ECLFLBadIDR         decimal.Decimal
	ECLWeightedIDR      *decimal.Decimal // nil for POCI
	BobotGood           decimal.Decimal
	BobotNormal         decimal.Decimal
	BobotBad            decimal.Decimal
	NetCarryingIDR      *decimal.Decimal
	PriorSealedECLIDR   *decimal.Decimal
	FlagPOCI            bool
	ParameterSnapshotID *uuid.UUID
	Warnings            []string
	ActorID             uuid.UUID
	// FormulaVersion identifies the ECL formula version used (F8 fix, migration 000030).
	// Default: FormulaVersionM7 ("M7-v1.0"). Stored for audit trail + regression detection.
	FormulaVersion string
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func scanResultLineRow(row *sql.Row) (*ResultLineRow, error) {
	var (
		id, calcRunID, instrumenID uuid.UUID
		evalDate                   time.Time
		periodeID                  string
		stage                      int
		routingPath                string
		eadRaw                     string
		pdGoodRaw, pdNormalRaw, pdBadRaw,
		lgdRaw,
		flGoodRaw, flNormalRaw, flBadRaw sql.NullString
		eclGood, eclNormal, eclBad,
		eclFlGood, eclFlNormal, eclFlBad string
		eclWeightedRaw                   sql.NullString
		bobotGood, bobotNormal, bobotBad string
		netCarryingRaw, priorSealedRaw   sql.NullString
		flagPOCI                         bool
		paramSnapshotID                  uuid.NullUUID
		warningsJSON                     sql.NullString
		sealedAt                         sql.NullTime
		createdAt                        time.Time
		formulaVersion                   string // F8 fix: formula_version column (migration 000030)
	)

	err := row.Scan(
		&id, &calcRunID, &instrumenID, &evalDate, &periodeID, &stage, &routingPath,
		&eadRaw, &pdGoodRaw, &pdNormalRaw, &pdBadRaw, &lgdRaw,
		&flGoodRaw, &flNormalRaw, &flBadRaw,
		&eclGood, &eclNormal, &eclBad,
		&eclFlGood, &eclFlNormal, &eclFlBad,
		&eclWeightedRaw, &bobotGood, &bobotNormal, &bobotBad,
		&netCarryingRaw, &priorSealedRaw, &flagPOCI, &paramSnapshotID,
		&warningsJSON, &sealedAt, &createdAt,
		&formulaVersion,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	r := &ResultLineRow{
		ID: id, CalcRunID: calcRunID, InstrumenID: instrumenID,
		EvaluationDate: evalDate, PeriodeID: periodeID,
		Stage: Stage(stage), RoutingPath: RoutingPath(routingPath),
		FlagPOCI: flagPOCI, ActorID: uuid.Nil,
		FormulaVersion: formulaVersion,
	}

	ead, err := decimal.NewFromString(eadRaw)
	if err != nil {
		return nil, fmt.Errorf("core.scanResultLineRow: parse ead_idr %q: %w", eadRaw, err)
	}
	r.EADIDR = ead

	if pdGoodRaw.Valid {
		d, err := decimal.NewFromString(pdGoodRaw.String)
		if err != nil {
			return nil, fmt.Errorf("core.scanResultLineRow: parse pd_used_good %q: %w", pdGoodRaw.String, err)
		}
		r.PDGood = &d
	}
	if pdNormalRaw.Valid {
		d, err := decimal.NewFromString(pdNormalRaw.String)
		if err != nil {
			return nil, fmt.Errorf("core.scanResultLineRow: parse pd_used_normal %q: %w", pdNormalRaw.String, err)
		}
		r.PDNormal = &d
	}
	if pdBadRaw.Valid {
		d, err := decimal.NewFromString(pdBadRaw.String)
		if err != nil {
			return nil, fmt.Errorf("core.scanResultLineRow: parse pd_used_bad %q: %w", pdBadRaw.String, err)
		}
		r.PDBad = &d
	}
	if lgdRaw.Valid {
		d, err := decimal.NewFromString(lgdRaw.String)
		if err != nil {
			return nil, fmt.Errorf("core.scanResultLineRow: parse lgd_used %q: %w", lgdRaw.String, err)
		}
		r.LGDUsed = &d
	}
	if flGoodRaw.Valid {
		d, err := decimal.NewFromString(flGoodRaw.String)
		if err != nil {
			return nil, fmt.Errorf("core.scanResultLineRow: parse fl_multiplier_good %q: %w", flGoodRaw.String, err)
		}
		r.FLGood = &d
	}
	if flNormalRaw.Valid {
		d, err := decimal.NewFromString(flNormalRaw.String)
		if err != nil {
			return nil, fmt.Errorf("core.scanResultLineRow: parse fl_multiplier_normal %q: %w", flNormalRaw.String, err)
		}
		r.FLNormal = &d
	}
	if flBadRaw.Valid {
		d, err := decimal.NewFromString(flBadRaw.String)
		if err != nil {
			return nil, fmt.Errorf("core.scanResultLineRow: parse fl_multiplier_bad %q: %w", flBadRaw.String, err)
		}
		r.FLBad = &d
	}

	g, err := decimal.NewFromString(eclGood)
	if err != nil {
		return nil, fmt.Errorf("core.scanResultLineRow: parse ecl_good_idr %q: %w", eclGood, err)
	}
	r.ECLGoodIDR = g
	n, err := decimal.NewFromString(eclNormal)
	if err != nil {
		return nil, fmt.Errorf("core.scanResultLineRow: parse ecl_normal_idr %q: %w", eclNormal, err)
	}
	r.ECLNormalIDR = n
	b, err := decimal.NewFromString(eclBad)
	if err != nil {
		return nil, fmt.Errorf("core.scanResultLineRow: parse ecl_bad_idr %q: %w", eclBad, err)
	}
	r.ECLBadIDR = b
	fg, err := decimal.NewFromString(eclFlGood)
	if err != nil {
		return nil, fmt.Errorf("core.scanResultLineRow: parse ecl_fl_good_idr %q: %w", eclFlGood, err)
	}
	r.ECLFLGoodIDR = fg
	fn, err := decimal.NewFromString(eclFlNormal)
	if err != nil {
		return nil, fmt.Errorf("core.scanResultLineRow: parse ecl_fl_normal_idr %q: %w", eclFlNormal, err)
	}
	r.ECLFLNormalIDR = fn
	fb, err := decimal.NewFromString(eclFlBad)
	if err != nil {
		return nil, fmt.Errorf("core.scanResultLineRow: parse ecl_fl_bad_idr %q: %w", eclFlBad, err)
	}
	r.ECLFLBadIDR = fb

	bg, err := decimal.NewFromString(bobotGood)
	if err != nil {
		return nil, fmt.Errorf("core.scanResultLineRow: parse bobot_good %q: %w", bobotGood, err)
	}
	r.BobotGood = bg
	bn, err := decimal.NewFromString(bobotNormal)
	if err != nil {
		return nil, fmt.Errorf("core.scanResultLineRow: parse bobot_normal %q: %w", bobotNormal, err)
	}
	r.BobotNormal = bn
	bb, err := decimal.NewFromString(bobotBad)
	if err != nil {
		return nil, fmt.Errorf("core.scanResultLineRow: parse bobot_bad %q: %w", bobotBad, err)
	}
	r.BobotBad = bb

	if eclWeightedRaw.Valid {
		d, err := decimal.NewFromString(eclWeightedRaw.String)
		if err != nil {
			return nil, fmt.Errorf("core.scanResultLineRow: parse ecl_weighted_idr %q: %w", eclWeightedRaw.String, err)
		}
		r.ECLWeightedIDR = &d
	}
	if netCarryingRaw.Valid {
		d, err := decimal.NewFromString(netCarryingRaw.String)
		if err != nil {
			return nil, fmt.Errorf("core.scanResultLineRow: parse net_carrying_idr %q: %w", netCarryingRaw.String, err)
		}
		r.NetCarryingIDR = &d
	}
	if priorSealedRaw.Valid {
		d, err := decimal.NewFromString(priorSealedRaw.String)
		if err != nil {
			return nil, fmt.Errorf("core.scanResultLineRow: parse prior_sealed_ecl_idr %q: %w", priorSealedRaw.String, err)
		}
		r.PriorSealedECLIDR = &d
	}
	if paramSnapshotID.Valid {
		r.ParameterSnapshotID = &paramSnapshotID.UUID
	}
	if warningsJSON.Valid && warningsJSON.String != "" && warningsJSON.String != "null" {
		if err := json.Unmarshal([]byte(warningsJSON.String), &r.Warnings); err != nil {
			return nil, fmt.Errorf("core.scanResultLineRow: parse warnings_json: %w", err)
		}
	}
	return r, nil
}

// decimalPtrStr8 returns *string with 8 decimal places, or nil.
// Used for SQL NULLable NUMERIC(10,8) params.
func decimalPtrStr8(d *decimal.Decimal) interface{} {
	if d == nil {
		return nil
	}
	s := d.StringFixed(8)
	return s
}

// decimalPtrStr4 returns *string with 4 decimal places, or nil.
// Used for SQL NULLable NUMERIC(20,4) params.
func decimalPtrStr4(d *decimal.Decimal) interface{} {
	if d == nil {
		return nil
	}
	s := d.StringFixed(4)
	return s
}

// marshalWarnings serializes []string to JSON bytes for warnings_json column.
func marshalWarnings(warnings []string) (interface{}, error) {
	if len(warnings) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(warnings)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}
