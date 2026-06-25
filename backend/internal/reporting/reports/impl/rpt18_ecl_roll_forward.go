package impl

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT18ECLRollForward{}) }

// RPT18ECLRollForward — ECL Roll-Forward per periode (ecl.ecl_roll_forward).
// Regulated: audit REPORT.RPT18_VIEWED in-tx per compliance evidence.
// reconcile_diff = ecl_closing - Σecl_calc_result_line.ecl_weighted; flag UNRECONCILED if ≠ 0.
type RPT18ECLRollForward struct{}

func (r *RPT18ECLRollForward) Slug() string             { return "rpt-18" }
func (r *RPT18ECLRollForward) Permission() string        { return "report.rpt-18.read" }
func (r *RPT18ECLRollForward) ExportPermission() string  { return "report.rpt-18.export" }
func (r *RPT18ECLRollForward) RegulatedFlag() bool       { return true } // REPORT.RPT18_VIEWED audit in-tx
func (r *RPT18ECLRollForward) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "periode_id", Dir: "asc"}}
}
func (r *RPT18ECLRollForward) AllowedSort() []string {
	return []string{"periode_id", "ecl_opening", "ecl_closing", "reconcile_diff"}
}
func (r *RPT18ECLRollForward) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "periode_id"}, {Col: "tenant_id"}}
}
func (r *RPT18ECLRollForward) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{
		{Key: "periode_id", Header: "Periode", Format: "text"},
		{Key: "ecl_opening", Header: "ECL Opening (IDR)", Format: "idr"},
		{Key: "transfers_in", Header: "Transfers In (IDR)", Format: "idr"},
		{Key: "transfers_out", Header: "Transfers Out (IDR)", Format: "idr"},
		{Key: "new_originations", Header: "New Originations (IDR)", Format: "idr"},
		{Key: "derecognitions", Header: "Derecognitions (IDR)", Format: "idr"},
		{Key: "remeasurements", Header: "Remeasurements (IDR)", Format: "idr"},
		{Key: "ecl_closing", Header: "ECL Closing (IDR)", Format: "idr"},
		{Key: "reconcile_diff", Header: "Selisih Rekonsiliasi", Format: "idr"},
		{Key: "reconcile_flag", Header: "Rekonsiliasi Flag", Format: "text"},
	}
}
func (r *RPT18ECLRollForward) Query(ctx context.Context, db *sql.DB, params reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
	where, args, _, err := reports.BuildWhere(params.Filters, r.AllowedSort(), 1)
	if err != nil {
		return nil, 0, err
	}
	cond := "WHERE rf.deleted_at IS NULL"
	if where != "" {
		cond += " AND " + where
	}
	order := reports.BuildOrderBy(params.Sort, r.AllowedSort(), r.DefaultSort())
	limit := params.Limit + 1
	var total int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM ecl.ecl_roll_forward rf %s", cond), args...).Scan(&total)
	lArgs := append(args, limit) //nolint:gocritic
	// reconcile_diff = ecl_closing - sum of ecl_weighted from result lines for same periode.
	// CASE WHEN diff = 0 → 'RECONCILED', else 'UNRECONCILED'.
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT rf.periode_id::text,
		       rf.ecl_opening, rf.transfers_in, rf.transfers_out,
		       rf.new_originations, rf.derecognitions, rf.remeasurements,
		       rf.ecl_closing,
		       rf.ecl_closing - COALESCE(
		           (SELECT SUM(ecl_weighted) FROM ecl.ecl_calc_result_line el
		            WHERE el.periode_id = rf.periode_id AND el.deleted_at IS NULL), 0
		       ) AS reconcile_diff,
		       CASE WHEN ABS(rf.ecl_closing - COALESCE(
		           (SELECT SUM(ecl_weighted) FROM ecl.ecl_calc_result_line el2
		            WHERE el2.periode_id = rf.periode_id AND el2.deleted_at IS NULL), 0
		       )) < 0.0001 THEN 'RECONCILED' ELSE 'UNRECONCILED' END AS reconcile_flag
		FROM ecl.ecl_roll_forward rf %s %s LIMIT $%d`, cond, order, len(lArgs)), lArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("rpt-18 query: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	seq, err := reports.ScanRowsToMaps(rows)
	return seq, total, err
}
