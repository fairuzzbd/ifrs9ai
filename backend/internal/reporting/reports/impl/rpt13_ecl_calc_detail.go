package impl

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT13ECLCalcDetail{}) }

// RPT13ECLCalcDetail — ECL Calc Run Detail (ecl.ecl_calc_result_line).
// Hot index: (calc_run_id, instrumen_id). ifrs9-compliance-reviewer BLOCKING.
type RPT13ECLCalcDetail struct{}

func (r *RPT13ECLCalcDetail) Slug() string             { return "rpt-13" }
func (r *RPT13ECLCalcDetail) Permission() string        { return "report.rpt-13.read" }
func (r *RPT13ECLCalcDetail) ExportPermission() string  { return "report.rpt-13.export" }
func (r *RPT13ECLCalcDetail) RegulatedFlag() bool       { return false }
func (r *RPT13ECLCalcDetail) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "ead_idr", Dir: "desc"}}
}
func (r *RPT13ECLCalcDetail) AllowedSort() []string {
	return []string{"ead_idr", "ecl_weighted", "stage", "instrumen_id", "calc_run_id", "created_at"}
}
func (r *RPT13ECLCalcDetail) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "calc_run_id"}, {Col: "instrumen_id"}, {Col: "stage"}, {Col: "tenant_id"}}
}
func (r *RPT13ECLCalcDetail) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{
		{Key: "kode_instrumen", Header: "Kode Instrumen", Format: "text"},
		{Key: "stage", Header: "Stage", Format: "text"},
		{Key: "ead_idr", Header: "EAD (IDR)", Format: "idr"},
		{Key: "pd_good", Header: "PD Good", Format: "pct"},
		{Key: "pd_normal", Header: "PD Normal", Format: "pct"},
		{Key: "pd_bad", Header: "PD Bad", Format: "pct"},
		{Key: "lgd", Header: "LGD", Format: "pct"},
		{Key: "ecl_good", Header: "ECL Good (IDR)", Format: "idr"},
		{Key: "ecl_normal", Header: "ECL Normal (IDR)", Format: "idr"},
		{Key: "ecl_bad", Header: "ECL Bad (IDR)", Format: "idr"},
		{Key: "ecl_weighted", Header: "ECL Weighted (IDR)", Format: "idr"},
		{Key: "fl_multiplier_good", Header: "FL Mult Good", Format: "text"},
		{Key: "fl_multiplier_normal", Header: "FL Mult Normal", Format: "text"},
		{Key: "fl_multiplier_bad", Header: "FL Mult Bad", Format: "text"},
	}
}
func (r *RPT13ECLCalcDetail) Query(ctx context.Context, db *sql.DB, params reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
	where, args, _, err := reports.BuildWhere(params.Filters, r.AllowedSort(), 1)
	if err != nil {
		return nil, 0, err
	}
	cond := "WHERE e.deleted_at IS NULL"
	if where != "" {
		cond += " AND " + where
	}
	order := reports.BuildOrderBy(params.Sort, r.AllowedSort(), r.DefaultSort())
	limit := params.Limit + 1
	var total int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM ecl.ecl_calc_result_line e %s", cond), args...).Scan(&total)
	lArgs := append(args, limit) //nolint:gocritic
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT i.kode_instrumen, e.stage::text,
		       e.ead_idr, e.pd_good, e.pd_normal, e.pd_bad, e.lgd,
		       e.ecl_good, e.ecl_normal, e.ecl_bad, e.ecl_weighted,
		       e.fl_multiplier_good::text, e.fl_multiplier_normal::text, e.fl_multiplier_bad::text
		FROM ecl.ecl_calc_result_line e
		LEFT JOIN mst.instrumen i ON i.id = e.instrumen_id
		%s %s LIMIT $%d`, cond, order, len(lArgs)), lArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("rpt-13 query: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	seq, err := reports.ScanRowsToMaps(rows)
	return seq, total, err
}
