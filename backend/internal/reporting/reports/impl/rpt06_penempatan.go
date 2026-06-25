package impl

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT06Penempatan{}) }

type RPT06Penempatan struct{}

func (r *RPT06Penempatan) Slug() string             { return "rpt-06" }
func (r *RPT06Penempatan) Permission() string        { return "report.rpt-06.read" }
func (r *RPT06Penempatan) ExportPermission() string  { return "report.rpt-06.export" }
func (r *RPT06Penempatan) RegulatedFlag() bool       { return false }
func (r *RPT06Penempatan) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "tanggal_penempatan", Dir: "desc"}}
}
func (r *RPT06Penempatan) AllowedSort() []string {
	return []string{"tanggal_penempatan", "kode_instrumen", "nominal_idr", "status", "created_at"}
}
func (r *RPT06Penempatan) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "status"}, {Col: "periode_id"}, {Col: "instrumen_id"}, {Col: "tenant_id"}}
}
func (r *RPT06Penempatan) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{
		{Key: "tanggal_penempatan", Header: "Tanggal Penempatan", Format: "date"},
		{Key: "kode_instrumen", Header: "Kode Instrumen", Format: "text"},
		{Key: "nominal_idr", Header: "Nominal (IDR)", Format: "idr"},
		{Key: "status", Header: "Status", Format: "text"},
		{Key: "maker_id", Header: "Maker", Format: "text"},
		{Key: "approver_id", Header: "Approver", Format: "text"},
	}
}
func (r *RPT06Penempatan) Query(ctx context.Context, db *sql.DB, params reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
	where, args, _, err := reports.BuildWhere(params.Filters, r.AllowedSort(), 1)
	if err != nil {
		return nil, 0, err
	}
	cond := "WHERE p.deleted_at IS NULL"
	if where != "" {
		cond += " AND " + where
	}
	order := reports.BuildOrderBy(params.Sort, r.AllowedSort(), r.DefaultSort())
	limit := params.Limit + 1
	var total int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM trx.penempatan p %s", cond), args...).Scan(&total)
	lArgs := append(args, limit) //nolint:gocritic
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT p.tanggal_penempatan, i.kode_instrumen, p.nominal_idr,
		       p.workflow_status AS status, p.maker_id, p.approver_id
		FROM trx.penempatan p
		LEFT JOIN mst.instrumen i ON i.id = p.instrumen_id
		%s %s LIMIT $%d`, cond, order, len(lArgs)), lArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("rpt-06 query: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	seq, err := reports.ScanRowsToMaps(rows)
	return seq, total, err
}
