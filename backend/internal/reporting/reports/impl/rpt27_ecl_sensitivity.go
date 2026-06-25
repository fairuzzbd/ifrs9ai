package impl

import (
	"context"
	"database/sql"
	"fmt"
	"iter"
	"math"
	"strconv"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT27ECLSensitivity{}) }

// RPT27ECLSensitivity — ECL what-if sensitivity: override scenario weights at query time.
// Regulated: audit REPORT.RPT27_SENSITIVITY_RUN written in-tx by service (RegulatedFlag=true).
// Formula: ecl_weighted_sensitivity = ecl_fl_good*w_good + ecl_fl_normal*w_normal + ecl_fl_bad*w_bad.
// w_good+w_normal+w_bad must = 1.0 ± 1e-6 (REPORT_PARAMS_INVALID if not).
type RPT27ECLSensitivity struct{}

func (r *RPT27ECLSensitivity) Slug() string             { return "rpt-27" }
func (r *RPT27ECLSensitivity) Permission() string        { return "report.rpt-27.read" }
func (r *RPT27ECLSensitivity) ExportPermission() string  { return "report.rpt-27.export" }
func (r *RPT27ECLSensitivity) RegulatedFlag() bool       { return true } // REPORT.RPT27_SENSITIVITY_RUN audit
func (r *RPT27ECLSensitivity) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "ead_idr", Dir: "desc"}}
}
func (r *RPT27ECLSensitivity) AllowedSort() []string {
	return []string{"ead_idr", "ecl_weighted_default", "ecl_weighted_sensitivity", "delta_ecl", "instrumen_id"}
}
func (r *RPT27ECLSensitivity) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "calc_run_id"}, {Col: "instrumen_id"}, {Col: "stage"}, {Col: "tenant_id"}}
}
func (r *RPT27ECLSensitivity) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{
		{Key: "kode_instrumen", Header: "Kode Instrumen", Format: "text"},
		{Key: "ead_idr", Header: "EAD (IDR)", Format: "idr"},
		{Key: "ecl_weighted_default", Header: "ECL Default (IDR)", Format: "idr"},
		{Key: "ecl_weighted_sensitivity", Header: "ECL Sensitivity (IDR)", Format: "idr"},
		{Key: "delta_ecl", Header: "Delta ECL (IDR)", Format: "idr"},
		{Key: "delta_pct", Header: "Delta (%)", Format: "pct"},
	}
}

func (r *RPT27ECLSensitivity) Query(ctx context.Context, db *sql.DB, params reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
	// Parse weights from Extra params.
	wGood, wNormal, wBad, err := parseWeights(params.Extra)
	if err != nil {
		return nil, 0, err
	}

	where, args, _, ferr := reports.BuildWhere(params.Filters, r.AllowedSort(), 1)
	if ferr != nil {
		return nil, 0, ferr
	}
	cond := "WHERE e.deleted_at IS NULL"
	if where != "" {
		cond += " AND " + where
	}
	order := reports.BuildOrderBy(params.Sort, r.AllowedSort(), r.DefaultSort())
	limit := params.Limit + 1
	var total int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM ecl.ecl_calc_result_line e %s", cond), args...).Scan(&total)
	// Append weight params.
	n := len(args) + 1
	args = append(args, wGood, wNormal, wBad, limit) //nolint:gocritic
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT i.kode_instrumen,
		       e.ead_idr,
		       e.ecl_weighted AS ecl_weighted_default,
		       e.ecl_fl_good * $%d + e.ecl_fl_normal * $%d + e.ecl_fl_bad * $%d AS ecl_weighted_sensitivity,
		       (e.ecl_fl_good * $%d + e.ecl_fl_normal * $%d + e.ecl_fl_bad * $%d) - e.ecl_weighted AS delta_ecl,
		       CASE WHEN e.ecl_weighted = 0 THEN 0
		            ELSE ((e.ecl_fl_good * $%d + e.ecl_fl_normal * $%d + e.ecl_fl_bad * $%d) - e.ecl_weighted)
		                 / e.ecl_weighted * 100
		       END AS delta_pct
		FROM ecl.ecl_calc_result_line e
		LEFT JOIN mst.instrumen i ON i.id = e.instrumen_id
		%s %s LIMIT $%d`,
		n, n+1, n+2,
		n, n+1, n+2,
		n, n+1, n+2,
		cond, order, n+3,
	), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("rpt-27 query: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	seq, ferr := reports.ScanRowsToMaps(rows)
	return seq, total, ferr
}

// parseWeights extracts w_good, w_normal, w_bad from Extra params and validates sum = 1.0 ± 1e-6.
func parseWeights(extra map[string]string) (float64, float64, float64, error) {
	if extra == nil {
		return 0.25, 0.50, 0.25, nil // defaults
	}
	parse := func(key string, def float64) (float64, error) {
		v, ok := extra[key]
		if !ok || v == "" {
			return def, nil
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, fmt.Errorf("parameter %s bukan angka valid: %w", key, err)
		}
		return f, nil
	}
	wGood, err := parse("w_good", 0.25)
	if err != nil {
		return 0, 0, 0, domainerrors.New(domainerrors.CodeReportParamsInvalid, err.Error())
	}
	wNormal, err := parse("w_normal", 0.50)
	if err != nil {
		return 0, 0, 0, domainerrors.New(domainerrors.CodeReportParamsInvalid, err.Error())
	}
	wBad, err := parse("w_bad", 0.25)
	if err != nil {
		return 0, 0, 0, domainerrors.New(domainerrors.CodeReportParamsInvalid, err.Error())
	}

	sum := wGood + wNormal + wBad
	if math.Abs(sum-1.0) > 1e-6 {
		return 0, 0, 0, domainerrors.New(domainerrors.CodeReportParamsInvalid,
			fmt.Sprintf("Bobot skenario harus berjumlah 1.0 (diterima: %.8f toleransi 1e-6)", sum))
	}
	return wGood, wNormal, wBad, nil
}
