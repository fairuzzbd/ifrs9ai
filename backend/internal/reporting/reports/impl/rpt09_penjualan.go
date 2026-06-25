package impl

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT09Penjualan{}) }

type RPT09Penjualan struct{}

func (r *RPT09Penjualan) Slug() string             { return "rpt-09" }
func (r *RPT09Penjualan) Permission() string        { return "report.rpt-09.read" }
func (r *RPT09Penjualan) ExportPermission() string  { return "report.rpt-09.export" }
func (r *RPT09Penjualan) RegulatedFlag() bool       { return false }
func (r *RPT09Penjualan) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "tanggal_jual", Dir: "desc"}}
}
func (r *RPT09Penjualan) AllowedSort() []string {
	return []string{"tanggal_jual", "instrumen_id", "proceeds_idr", "realized_gainloss_idr", "workflow_status", "created_at"}
}
func (r *RPT09Penjualan) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "workflow_status"}, {Col: "periode_id"}, {Col: "instrumen_id"}, {Col: "tenant_id"}}
}
func (r *RPT09Penjualan) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{
		{Key: "tanggal_jual", Header: "Tanggal Jual", Format: "date"},
		{Key: "kode_instrumen", Header: "Kode Instrumen", Format: "text"},
		{Key: "proceeds_idr", Header: "Proceeds (IDR)", Format: "idr"},
		{Key: "realized_gainloss_idr", Header: "Realized Gain/Loss (IDR)", Format: "idr"},
		{Key: "workflow_status", Header: "Status", Format: "text"},
	}
}
func (r *RPT09Penjualan) Query(ctx context.Context, db *sql.DB, params reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
	where, args, _, err := reports.BuildWhere(params.Filters, r.AllowedSort(), 1)
	if err != nil {
		return nil, 0, err
	}
	cond := "WHERE pp.deleted_at IS NULL"
	if where != "" {
		cond += " AND " + where
	}
	order := reports.BuildOrderBy(params.Sort, r.AllowedSort(), r.DefaultSort())
	limit := params.Limit + 1
	var total int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM trx.penjualan_pencairan pp %s", cond), args...).Scan(&total)
	lArgs := append(args, limit) //nolint:gocritic
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT pp.tanggal_jual, i.kode_instrumen, pp.proceeds_idr, pp.realized_gainloss_idr, pp.workflow_status
		FROM trx.penjualan_pencairan pp LEFT JOIN mst.instrumen i ON i.id = pp.instrumen_id
		%s %s LIMIT $%d`, cond, order, len(lArgs)), lArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("rpt-09 query: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	seq, err := reports.ScanRowsToMaps(rows)
	return seq, total, err
}
