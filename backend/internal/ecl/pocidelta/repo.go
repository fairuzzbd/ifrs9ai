package pocidelta

// repo.go — Repository for ecl.poci_baseline and ecl.poci_delta_log.
// Uses database/sql (same as M9 akrualmaturity pattern). TX boundary lives in service.go.
//
// Conventions (DEC-016/020/022):
//   - tenant_id in all WHERE clauses
//   - cursor-based pagination (no offset); limit+1 trick for hasMore
//   - never float64 — decimal types passed as strings to driver
//   - WORM: InsertBaseline never UPDATEs; no DeleteBaseline method
//   - SelectContext / GetContext not used — standard database/sql scan

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// Repository is the interface for POCI delta persistence.
type Repository interface {
	// Baseline
	InsertBaseline(ctx context.Context, tx *sql.Tx, b *Baseline) error
	GetBaselineByInstrumen(ctx context.Context, instrumenID uuid.UUID, tenantID string) (*Baseline, error)
	ListBaselines(ctx context.Context, q listquery.Query, tenantID string) ([]Baseline, Pagination, error)

	// DeltaLog
	InsertDeltaLog(ctx context.Context, tx *sql.Tx, d *DeltaLog) error
	GetDeltaLogByRunAndInstrumen(ctx context.Context, calcRunID, instrumenID uuid.UUID, tenantID string) (*DeltaLog, error)
	UpdateDeltaLogStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, tanggalCompute time.Time, status DeltaStatus, jurnalHeaderID *uuid.UUID, updatedBy uuid.UUID) error
	ListDeltaLogs(ctx context.Context, q listquery.Query, tenantID string) ([]DeltaLog, Pagination, error)
	GetDeltaHistoryByInstrumen(ctx context.Context, instrumenID uuid.UUID, q listquery.Query, tenantID string) ([]DeltaLog, Pagination, error)
	GetCumulativeDelta(ctx context.Context, instrumenID uuid.UUID, beforeRunDate time.Time, tenantID string) (decimal.Decimal, error)

	// Summary
	GetDeltaSummary(ctx context.Context, portofolioID *uuid.UUID, year, month int, tenantID string) (*DeltaSummary, error)

	// Instrumen info
	GetInstrumenPociInfo(ctx context.Context, instrumenID uuid.UUID, tenantID string) (*InstrumenPociInfo, error)
	ListPociInstrumenByCalcRun(ctx context.Context, calcRunID uuid.UUID, tenantID string) ([]InstrumenPociInfo, error)

	// Period and calc run
	GetPeriodeStatus(ctx context.Context, periodeBulananID uuid.UUID, tenantID string) (string, error)
	GetCalcRunStatus(ctx context.Context, calcRunID uuid.UUID, tenantID string) (string, error)
	GetCurrentECLForPociInstrumen(ctx context.Context, calcRunID, instrumenID uuid.UUID, tenantID string) (decimal.Decimal, error)

	// Large delta threshold from sys.parameter
	GetLargeDeltaThreshold(ctx context.Context, tenantID string) (decimal.Decimal, error)
}

// sqlRepo is the concrete database/sql implementation.
type sqlRepo struct {
	db *sql.DB
}

// NewRepository creates a new sql-backed Repository.
func NewRepository(db *sql.DB) Repository {
	return &sqlRepo{db: db}
}

// defaultLimit is the page size used when the caller does not specify one.
// All list queries use limit+1 trick to detect hasMore without COUNT(*).
const defaultLimit = 50

// repoLimit returns the page limit, falling back to defaultLimit.
// listquery.Query does not carry a Limit field.
func repoLimit(_ listquery.Query) int { return defaultLimit }

// paginateBaseline applies the limit+1 trick for []Baseline.
func paginateBaseline(rows []Baseline, limit int) ([]Baseline, Pagination, error) {
	pag := Pagination{Limit: limit}
	if len(rows) > limit {
		rows = rows[:limit]
		pag.HasMore = true
	}
	return rows, pag, nil
}

// paginateDeltaLog applies the limit+1 trick for []DeltaLog.
func paginateDeltaLog(rows []DeltaLog, limit int) ([]DeltaLog, Pagination, error) {
	pag := Pagination{Limit: limit}
	if len(rows) > limit {
		rows = rows[:limit]
		pag.HasMore = true
	}
	return rows, pag, nil
}

// execCtx runs a statement in tx (if non-nil) or db.
func (r *sqlRepo) execCtx(ctx context.Context, tx *sql.Tx, q string, args ...interface{}) error {
	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, q, args...)
	} else {
		_, err = r.db.ExecContext(ctx, q, args...)
	}
	return err
}

// ─── Baseline ─────────────────────────────────────────────────────────────────

func (r *sqlRepo) InsertBaseline(ctx context.Context, tx *sql.Tx, b *Baseline) error {
	const q = `
		INSERT INTO ecl.poci_baseline (
			id, instrumen_id, tanggal_baseline, lifetime_ecl_at_origination,
			cashflow_expectasi_jsonb, credit_adjusted_eir, origination_date,
			created_at, created_by, tenant_id
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10
		)`
	var cashflowJSON []byte
	if b.CashflowExpektasiJsonb != nil {
		cashflowJSON = *b.CashflowExpektasiJsonb
	}
	if err := r.execCtx(ctx, tx, q,
		b.ID, b.InstrumenID, b.TanggalBaseline,
		b.LifetimeECLAtOrigination.StringFixed(4),
		cashflowJSON,
		b.CreditAdjustedEIR.StringFixed(8),
		b.OriginationDate,
		b.CreatedAt, b.CreatedBy, b.TenantID,
	); err != nil {
		return fmt.Errorf("InsertBaseline: %w", err)
	}
	return nil
}

func (r *sqlRepo) GetBaselineByInstrumen(ctx context.Context, instrumenID uuid.UUID, tenantID string) (*Baseline, error) {
	const q = `
		SELECT id, instrumen_id, tanggal_baseline, lifetime_ecl_at_origination,
		       cashflow_expectasi_jsonb, credit_adjusted_eir, origination_date,
		       created_at, created_by, tenant_id
		FROM ecl.poci_baseline
		WHERE instrumen_id = $1 AND tenant_id = $2
		LIMIT 1`
	var b Baseline
	var eclStr, eirStr string
	var cashflowRaw []byte
	row := r.db.QueryRowContext(ctx, q, instrumenID, tenantID)
	if err := row.Scan(
		&b.ID, &b.InstrumenID, &b.TanggalBaseline, &eclStr,
		&cashflowRaw, &eirStr, &b.OriginationDate,
		&b.CreatedAt, &b.CreatedBy, &b.TenantID,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetBaselineByInstrumen: %w", err)
	}
	var parseErr error
	if b.LifetimeECLAtOrigination, parseErr = decimal.NewFromString(eclStr); parseErr != nil {
		return nil, fmt.Errorf("GetBaselineByInstrumen parse ecl: %w", parseErr)
	}
	if b.CreditAdjustedEIR, parseErr = decimal.NewFromString(eirStr); parseErr != nil {
		return nil, fmt.Errorf("GetBaselineByInstrumen parse eir: %w", parseErr)
	}
	if len(cashflowRaw) > 0 {
		raw := json.RawMessage(cashflowRaw)
		b.CashflowExpektasiJsonb = &raw
	}
	return &b, nil
}

func (r *sqlRepo) ListBaselines(ctx context.Context, q listquery.Query, tenantID string) ([]Baseline, Pagination, error) {
	allowed := []string{"tanggal_baseline", "lifetime_ecl_at_origination", "instrumen_id", "created_at"}
	where, args, orderBy := q.WithAllowed(allowed).ToSQL("b")
	tenantCond := fmt.Sprintf("b.tenant_id = $%d", len(args)+1)
	args = append(args, tenantID)
	fullWhere := tenantCond
	if where != "" {
		fullWhere = tenantCond + " AND " + where
	}
	if orderBy == "" {
		orderBy = "b.created_at DESC"
	}
	limit := repoLimit(q)
	query := fmt.Sprintf(`
		SELECT id, instrumen_id, tanggal_baseline, lifetime_ecl_at_origination,
		       cashflow_expectasi_jsonb, credit_adjusted_eir, origination_date,
		       created_at, created_by, tenant_id
		FROM ecl.poci_baseline b
		WHERE %s
		ORDER BY %s
		LIMIT %d`, fullWhere, orderBy, limit+1)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, Pagination{}, fmt.Errorf("ListBaselines: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var baselines []Baseline
	for rows.Next() {
		var b Baseline
		var eclStr, eirStr string
		var cashflowRaw []byte
		if err := rows.Scan(
			&b.ID, &b.InstrumenID, &b.TanggalBaseline, &eclStr,
			&cashflowRaw, &eirStr, &b.OriginationDate,
			&b.CreatedAt, &b.CreatedBy, &b.TenantID,
		); err != nil {
			return nil, Pagination{}, fmt.Errorf("ListBaselines scan: %w", err)
		}
		if b.LifetimeECLAtOrigination, err = decimal.NewFromString(eclStr); err != nil {
			return nil, Pagination{}, fmt.Errorf("ListBaselines parse ecl: %w", err)
		}
		if b.CreditAdjustedEIR, err = decimal.NewFromString(eirStr); err != nil {
			return nil, Pagination{}, fmt.Errorf("ListBaselines parse eir: %w", err)
		}
		if len(cashflowRaw) > 0 {
			raw := json.RawMessage(cashflowRaw)
			b.CashflowExpektasiJsonb = &raw
		}
		baselines = append(baselines, b)
	}
	if err := rows.Err(); err != nil {
		return nil, Pagination{}, fmt.Errorf("ListBaselines rows: %w", err)
	}
	return paginateBaseline(baselines, limit)
}

// ─── DeltaLog ─────────────────────────────────────────────────────────────────

func (r *sqlRepo) InsertDeltaLog(ctx context.Context, tx *sql.Tx, d *DeltaLog) error {
	const q = `
		INSERT INTO ecl.poci_delta_log (
			id, calc_run_id, instrumen_id, tanggal_compute,
			baseline_ecl, current_ecl, delta_ecl, direction,
			prior_delta_cumulative, jurnal_header_id, periode_bulanan_id,
			status, created_at, created_by, updated_at, updated_by,
			row_version, tenant_id
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18
		)`
	var priorCumStr interface{}
	if d.PriorDeltaCumulative != nil {
		priorCumStr = d.PriorDeltaCumulative.StringFixed(4)
	}
	if err := r.execCtx(ctx, tx, q,
		d.ID, d.CalcRunID, d.InstrumenID, d.TanggalCompute,
		d.BaselineECL.StringFixed(4), d.CurrentECL.StringFixed(4),
		d.DeltaECL.StringFixed(4), string(d.Direction),
		priorCumStr, d.JurnalHeaderID, d.PeriodeBulananID,
		string(d.Status),
		d.CreatedAt, d.CreatedBy, d.UpdatedAt, d.UpdatedBy,
		d.RowVersion, d.TenantID,
	); err != nil {
		return fmt.Errorf("InsertDeltaLog: %w", err)
	}
	return nil
}

func (r *sqlRepo) GetDeltaLogByRunAndInstrumen(ctx context.Context, calcRunID, instrumenID uuid.UUID, tenantID string) (*DeltaLog, error) {
	const q = `
		SELECT id, calc_run_id, instrumen_id, tanggal_compute,
		       baseline_ecl, current_ecl, delta_ecl, direction,
		       prior_delta_cumulative, jurnal_header_id, periode_bulanan_id,
		       status, created_at, created_by, updated_at, updated_by,
		       deleted_at, deleted_by, row_version, tenant_id
		FROM ecl.poci_delta_log
		WHERE calc_run_id = $1 AND instrumen_id = $2 AND tenant_id = $3
		  AND deleted_at IS NULL
		LIMIT 1`
	d, err := r.scanOneDeltaLog(r.db.QueryRowContext(ctx, q, calcRunID, instrumenID, tenantID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("GetDeltaLogByRunAndInstrumen: %w", err)
	}
	return d, nil
}

func (r *sqlRepo) UpdateDeltaLogStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, tanggalCompute time.Time, status DeltaStatus, jurnalHeaderID *uuid.UUID, updatedBy uuid.UUID) error {
	const q = `
		UPDATE ecl.poci_delta_log
		SET status = $1, jurnal_header_id = $2, updated_by = $3, updated_at = now()
		WHERE id = $4 AND tanggal_compute = $5`
	return r.execCtx(ctx, tx, q, string(status), jurnalHeaderID, updatedBy, id, tanggalCompute)
}

func (r *sqlRepo) ListDeltaLogs(ctx context.Context, q listquery.Query, tenantID string) ([]DeltaLog, Pagination, error) {
	allowed := []string{"tanggal_compute", "delta_ecl", "instrumen_id", "direction", "status", "created_at"}
	where, args, orderBy := q.WithAllowed(allowed).ToSQL("d")
	tenantCond := fmt.Sprintf("d.tenant_id = $%d AND d.deleted_at IS NULL", len(args)+1)
	args = append(args, tenantID)
	fullWhere := tenantCond
	if where != "" {
		fullWhere = tenantCond + " AND " + where
	}
	if orderBy == "" {
		orderBy = "d.tanggal_compute DESC"
	}
	limit := repoLimit(q)
	query := fmt.Sprintf(`
		SELECT id, calc_run_id, instrumen_id, tanggal_compute,
		       baseline_ecl, current_ecl, delta_ecl, direction,
		       prior_delta_cumulative, jurnal_header_id, periode_bulanan_id,
		       status, created_at, created_by, updated_at, updated_by,
		       deleted_at, deleted_by, row_version, tenant_id
		FROM ecl.poci_delta_log d
		WHERE %s
		ORDER BY %s
		LIMIT %d`, fullWhere, orderBy, limit+1)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, Pagination{}, fmt.Errorf("ListDeltaLogs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var logs []DeltaLog
	for rows.Next() {
		d, scanErr := r.scanOneDeltaLogRow(rows)
		if scanErr != nil {
			return nil, Pagination{}, fmt.Errorf("ListDeltaLogs scan: %w", scanErr)
		}
		logs = append(logs, *d)
	}
	if err := rows.Err(); err != nil {
		return nil, Pagination{}, fmt.Errorf("ListDeltaLogs rows: %w", err)
	}
	return paginateDeltaLog(logs, limit)
}

func (r *sqlRepo) GetDeltaHistoryByInstrumen(ctx context.Context, instrumenID uuid.UUID, q listquery.Query, tenantID string) ([]DeltaLog, Pagination, error) {
	allowed := []string{"tanggal_compute", "delta_ecl", "direction", "status", "created_at"}
	where, args, orderBy := q.WithAllowed(allowed).ToSQL("d")
	baseWhere := fmt.Sprintf("d.instrumen_id = $%d AND d.tenant_id = $%d AND d.deleted_at IS NULL",
		len(args)+1, len(args)+2)
	args = append(args, instrumenID, tenantID)
	fullWhere := baseWhere
	if where != "" {
		fullWhere = baseWhere + " AND " + where
	}
	if orderBy == "" {
		orderBy = "d.tanggal_compute DESC"
	}
	limit := repoLimit(q)
	query := fmt.Sprintf(`
		SELECT id, calc_run_id, instrumen_id, tanggal_compute,
		       baseline_ecl, current_ecl, delta_ecl, direction,
		       prior_delta_cumulative, jurnal_header_id, periode_bulanan_id,
		       status, created_at, created_by, updated_at, updated_by,
		       deleted_at, deleted_by, row_version, tenant_id
		FROM ecl.poci_delta_log d
		WHERE %s
		ORDER BY %s
		LIMIT %d`, fullWhere, orderBy, limit+1)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, Pagination{}, fmt.Errorf("GetDeltaHistoryByInstrumen: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var logs []DeltaLog
	for rows.Next() {
		d, scanErr := r.scanOneDeltaLogRow(rows)
		if scanErr != nil {
			return nil, Pagination{}, fmt.Errorf("GetDeltaHistoryByInstrumen scan: %w", scanErr)
		}
		logs = append(logs, *d)
	}
	if err := rows.Err(); err != nil {
		return nil, Pagination{}, fmt.Errorf("GetDeltaHistoryByInstrumen rows: %w", err)
	}
	return paginateDeltaLog(logs, limit)
}

func (r *sqlRepo) GetCumulativeDelta(ctx context.Context, instrumenID uuid.UUID, beforeRunDate time.Time, tenantID string) (decimal.Decimal, error) {
	const q = `
		SELECT COALESCE(SUM(delta_ecl), 0)
		FROM ecl.poci_delta_log
		WHERE instrumen_id = $1 AND tenant_id = $2
		  AND tanggal_compute < $3 AND deleted_at IS NULL`
	var sum string
	if err := r.db.QueryRowContext(ctx, q, instrumenID, tenantID, beforeRunDate).Scan(&sum); err != nil {
		return decimal.Zero, fmt.Errorf("GetCumulativeDelta: %w", err)
	}
	d, err := decimal.NewFromString(sum)
	if err != nil {
		return decimal.Zero, fmt.Errorf("GetCumulativeDelta parse: %w", err)
	}
	return d, nil
}

func (r *sqlRepo) GetDeltaSummary(_ context.Context, _ *uuid.UUID, year, month int, _ string) (*DeltaSummary, error) {
	// Placeholder — real implementation aggregates ecl.poci_delta_log by MTD/YTD.
	// Stubbed for compilation; full implementation in integration sprint.
	return &DeltaSummary{
		Year:  year,
		Month: month,
	}, nil
}

func (r *sqlRepo) GetInstrumenPociInfo(ctx context.Context, instrumenID uuid.UUID, tenantID string) (*InstrumenPociInfo, error) {
	const q = `
		SELECT id, kode_instrumen, is_poci, status, portofolio_id
		FROM mst.instrumen
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`
	var info InstrumenPociInfo
	if err := r.db.QueryRowContext(ctx, q, instrumenID, tenantID).Scan(
		&info.ID, &info.KodeInstrumen, &info.IsPoci, &info.Status, &info.PortofolioID,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetInstrumenPociInfo: %w", err)
	}
	return &info, nil
}

func (r *sqlRepo) ListPociInstrumenByCalcRun(ctx context.Context, calcRunID uuid.UUID, tenantID string) ([]InstrumenPociInfo, error) {
	const q = `
		SELECT DISTINCT i.id, i.kode_instrumen, i.is_poci, i.status, i.portofolio_id
		FROM ecl.ecl_calc_result_line rl
		JOIN mst.instrumen i ON i.id = rl.instrumen_id
		WHERE rl.calc_run_id = $1 AND i.is_poci = TRUE
		  AND i.tenant_id = $2 AND i.deleted_at IS NULL`
	rows, err := r.db.QueryContext(ctx, q, calcRunID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("ListPociInstrumenByCalcRun: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var infos []InstrumenPociInfo
	for rows.Next() {
		var info InstrumenPociInfo
		if err := rows.Scan(&info.ID, &info.KodeInstrumen, &info.IsPoci, &info.Status, &info.PortofolioID); err != nil {
			return nil, fmt.Errorf("ListPociInstrumenByCalcRun scan: %w", err)
		}
		infos = append(infos, info)
	}
	return infos, rows.Err()
}

func (r *sqlRepo) GetPeriodeStatus(ctx context.Context, periodeBulananID uuid.UUID, tenantID string) (string, error) {
	const q = `
		SELECT status_periode FROM mst.periode_buku
		WHERE id = $1 AND tenant_id = $2 LIMIT 1`
	var status string
	if err := r.db.QueryRowContext(ctx, q, periodeBulananID, tenantID).Scan(&status); err != nil {
		return "", fmt.Errorf("GetPeriodeStatus: %w", err)
	}
	return status, nil
}

func (r *sqlRepo) GetCalcRunStatus(ctx context.Context, calcRunID uuid.UUID, tenantID string) (string, error) {
	const q = `
		SELECT status FROM ecl.ecl_calc_run
		WHERE id = $1 AND tenant_id = $2 LIMIT 1`
	var status string
	if err := r.db.QueryRowContext(ctx, q, calcRunID, tenantID).Scan(&status); err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("calc_run %s: %w", calcRunID, sql.ErrNoRows)
		}
		return "", fmt.Errorf("GetCalcRunStatus: %w", err)
	}
	return status, nil
}

func (r *sqlRepo) GetCurrentECLForPociInstrumen(ctx context.Context, calcRunID, instrumenID uuid.UUID, tenantID string) (decimal.Decimal, error) {
	const q = `
		SELECT ecl_weighted
		FROM ecl.ecl_calc_result_line
		WHERE calc_run_id = $1 AND instrumen_id = $2 AND tenant_id = $3
		LIMIT 1`
	var val string
	if err := r.db.QueryRowContext(ctx, q, calcRunID, instrumenID, tenantID).Scan(&val); err != nil {
		if err == sql.ErrNoRows {
			return decimal.Zero, fmt.Errorf("ecl_calc_result_line: instrumen %s not found in run %s", instrumenID, calcRunID)
		}
		return decimal.Zero, fmt.Errorf("GetCurrentECLForPociInstrumen: %w", err)
	}
	d, err := decimal.NewFromString(val)
	if err != nil {
		return decimal.Zero, fmt.Errorf("GetCurrentECLForPociInstrumen parse: %w", err)
	}
	return d, nil
}

func (r *sqlRepo) GetLargeDeltaThreshold(ctx context.Context, tenantID string) (decimal.Decimal, error) {
	const q = `
		SELECT value FROM sys.parameter
		WHERE key = 'POCI_LARGE_DELTA_THRESHOLD' AND tenant_id = $1 LIMIT 1`
	var val string
	if err := r.db.QueryRowContext(ctx, q, tenantID).Scan(&val); err != nil {
		if err == sql.ErrNoRows {
			return decimal.NewFromFloat(500000000), nil // default per S5-AC3
		}
		return decimal.Zero, fmt.Errorf("GetLargeDeltaThreshold: %w", err)
	}
	return decimal.NewFromString(val)
}

// ─── scan helpers ─────────────────────────────────────────────────────────────

// rowScanner is implemented by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

// scanOneDeltaLog scans a single DeltaLog from a *sql.Row.
func (r *sqlRepo) scanOneDeltaLog(row *sql.Row) (*DeltaLog, error) {
	return scanDeltaLog(row)
}

// scanOneDeltaLogRow scans a single DeltaLog from *sql.Rows (in loop).
func (r *sqlRepo) scanOneDeltaLogRow(rows *sql.Rows) (*DeltaLog, error) {
	return scanDeltaLog(rows)
}

func scanDeltaLog(s rowScanner) (*DeltaLog, error) {
	var d DeltaLog
	var baselineStr, currentStr, deltaStr string
	var priorStr *string
	var dirStr, statusStr string
	if err := s.Scan(
		&d.ID, &d.CalcRunID, &d.InstrumenID, &d.TanggalCompute,
		&baselineStr, &currentStr, &deltaStr, &dirStr,
		&priorStr, &d.JurnalHeaderID, &d.PeriodeBulananID,
		&statusStr,
		&d.CreatedAt, &d.CreatedBy, &d.UpdatedAt, &d.UpdatedBy,
		&d.DeletedAt, &d.DeletedBy, &d.RowVersion, &d.TenantID,
	); err != nil {
		return nil, err
	}
	var err error
	if d.BaselineECL, err = decimal.NewFromString(baselineStr); err != nil {
		return nil, fmt.Errorf("scan baseline_ecl: %w", err)
	}
	if d.CurrentECL, err = decimal.NewFromString(currentStr); err != nil {
		return nil, fmt.Errorf("scan current_ecl: %w", err)
	}
	if d.DeltaECL, err = decimal.NewFromString(deltaStr); err != nil {
		return nil, fmt.Errorf("scan delta_ecl: %w", err)
	}
	if priorStr != nil {
		v, pErr := decimal.NewFromString(*priorStr)
		if pErr != nil {
			return nil, fmt.Errorf("scan prior_delta_cumulative: %w", pErr)
		}
		d.PriorDeltaCumulative = &v
	}
	d.Direction = Direction(dirStr)
	d.Status = DeltaStatus(statusStr)
	return &d, nil
}
