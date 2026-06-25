package impl

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT16POCIDelta{}) }

// RPT16POCIDelta — POCI Delta History via rpt.mv_poci_delta_summary (M13 MV).
type RPT16POCIDelta struct{}

func (r *RPT16POCIDelta) Slug() string             { return "rpt-16" }
func (r *RPT16POCIDelta) Permission() string        { return "report.rpt-16.read" }
func (r *RPT16POCIDelta) ExportPermission() string  { return "report.rpt-16.export" }
func (r *RPT16POCIDelta) RegulatedFlag() bool       { return false }
func (r *RPT16POCIDelta) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "created_at", Dir: "desc"}}
}
func (r *RPT16POCIDelta) AllowedSort() []string {
	return []string{"created_at", "instrumen_id", "delta_ecl_idr", "direction", "periode_id"}
}
func (r *RPT16POCIDelta) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "instrumen_id"}, {Col: "direction"}, {Col: "periode_id"}, {Col: "tenant_id"}}
}
func (r *RPT16POCIDelta) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{
		{Key: "instrumen_id", Header: "Instrumen ID", Format: "text"},
		{Key: "delta_ecl_idr", Header: "Delta ECL (IDR)", Format: "idr"},
		{Key: "direction", Header: "Arah", Format: "text"},
		{Key: "periode_id", Header: "Periode", Format: "text"},
		{Key: "created_at", Header: "Dibuat", Format: "datetime"},
	}
}
func (r *RPT16POCIDelta) Query(ctx context.Context, db *sql.DB, params reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
	where, args, _, err := reports.BuildWhere(params.Filters, r.AllowedSort(), 1)
	if err != nil {
		return nil, 0, err
	}
	cond := ""
	if where != "" {
		cond = "WHERE " + where
	}
	order := reports.BuildOrderBy(params.Sort, r.AllowedSort(), r.DefaultSort())
	limit := params.Limit + 1
	var total int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM rpt.mv_poci_delta_summary %s", cond), args...).Scan(&total)
	lArgs := append(args, limit) //nolint:gocritic
	rows, err := db.QueryContext(ctx, fmt.Sprintf(
		`SELECT instrumen_id, delta_ecl_idr, direction, periode_id, created_at FROM rpt.mv_poci_delta_summary %s %s LIMIT $%d`,
		cond, order, len(lArgs)), lArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("rpt-16 query: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	seq, err := reports.ScanRowsToMaps(rows)
	return seq, total, err
}
