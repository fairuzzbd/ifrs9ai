package impl

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT05FXRateHistory{}) }

type RPT05FXRateHistory struct{}

func (r *RPT05FXRateHistory) Slug() string             { return "rpt-05" }
func (r *RPT05FXRateHistory) Permission() string        { return "report.rpt-05.read" }
func (r *RPT05FXRateHistory) ExportPermission() string  { return "report.rpt-05.export" }
func (r *RPT05FXRateHistory) RegulatedFlag() bool       { return false }
func (r *RPT05FXRateHistory) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "tanggal", Dir: "desc"}}
}
func (r *RPT05FXRateHistory) AllowedSort() []string {
	return []string{"tanggal", "kode_valuta", "rate", "sumber"}
}
func (r *RPT05FXRateHistory) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "kode_valuta"}, {Col: "tanggal"}, {Col: "sumber"}, {Col: "tenant_id"}}
}
func (r *RPT05FXRateHistory) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{
		{Key: "tanggal", Header: "Tanggal", Format: "date"},
		{Key: "kode_valuta", Header: "Mata Uang", Format: "text"},
		{Key: "rate", Header: "Rate (IDR)", Format: "idr"},
		{Key: "sumber", Header: "Sumber", Format: "text"},
		{Key: "created_at", Header: "Dibuat", Format: "datetime"},
	}
}
func (r *RPT05FXRateHistory) Query(ctx context.Context, db *sql.DB, params reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
	where, args, _, err := reports.BuildWhere(params.Filters, r.AllowedSort(), 1)
	if err != nil {
		return nil, 0, err
	}
	cond := "WHERE f.deleted_at IS NULL"
	if where != "" {
		cond += " AND " + where
	}
	order := reports.BuildOrderBy(params.Sort, r.AllowedSort(), r.DefaultSort())
	limit := params.Limit + 1
	var total int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM sys.fx_rate f %s", cond), args...).Scan(&total)
	lArgs := append(args, limit) //nolint:gocritic
	rows, err := db.QueryContext(ctx, fmt.Sprintf(
		`SELECT tanggal, kode_valuta, rate, COALESCE(sumber,'JISDOR') AS sumber, f.created_at FROM sys.fx_rate f %s %s LIMIT $%d`,
		cond, order, len(lArgs)), lArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("rpt-05 query: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	seq, err := reports.ScanRowsToMaps(rows)
	return seq, total, err
}
