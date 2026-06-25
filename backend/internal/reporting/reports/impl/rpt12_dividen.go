package impl

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT12Dividen{}) }

type RPT12Dividen struct{}

func (r *RPT12Dividen) Slug() string             { return "rpt-12" }
func (r *RPT12Dividen) Permission() string        { return "report.rpt-12.read" }
func (r *RPT12Dividen) ExportPermission() string  { return "report.rpt-12.export" }
func (r *RPT12Dividen) RegulatedFlag() bool       { return false }
func (r *RPT12Dividen) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "tanggal_dividen", Dir: "desc"}}
}
func (r *RPT12Dividen) AllowedSort() []string {
	return []string{"tanggal_dividen", "instrumen_id", "jumlah_dividen_idr", "status", "created_at"}
}
func (r *RPT12Dividen) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "status"}, {Col: "periode_id"}, {Col: "instrumen_id"}, {Col: "tenant_id"}}
}
func (r *RPT12Dividen) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{
		{Key: "tanggal_dividen", Header: "Tanggal Dividen", Format: "date"},
		{Key: "kode_instrumen", Header: "Kode Instrumen", Format: "text"},
		{Key: "jumlah_dividen_idr", Header: "Jumlah Dividen (IDR)", Format: "idr"},
		{Key: "status", Header: "Status", Format: "text"},
	}
}
func (r *RPT12Dividen) Query(ctx context.Context, db *sql.DB, params reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
	where, args, _, err := reports.BuildWhere(params.Filters, r.AllowedSort(), 1)
	if err != nil {
		return nil, 0, err
	}
	cond := "WHERE dv.deleted_at IS NULL"
	if where != "" {
		cond += " AND " + where
	}
	order := reports.BuildOrderBy(params.Sort, r.AllowedSort(), r.DefaultSort())
	limit := params.Limit + 1
	var total int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM trx.dividen dv %s", cond), args...).Scan(&total)
	lArgs := append(args, limit) //nolint:gocritic
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT dv.tanggal_dividen, i.kode_instrumen, dv.jumlah_dividen_idr, COALESCE(dv.status,'') AS status
		FROM trx.dividen dv LEFT JOIN mst.instrumen i ON i.id = dv.instrumen_id
		%s %s LIMIT $%d`, cond, order, len(lArgs)), lArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("rpt-12 query: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	seq, err := reports.ScanRowsToMaps(rows)
	return seq, total, err
}
