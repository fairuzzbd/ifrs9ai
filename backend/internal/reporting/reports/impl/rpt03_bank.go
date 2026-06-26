package impl

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT03Bank{}) }

type RPT03Bank struct{}

func (r *RPT03Bank) Slug() string             { return "rpt-03" }
func (r *RPT03Bank) Permission() string        { return "report.rpt-03.read" }
func (r *RPT03Bank) ExportPermission() string  { return "report.rpt-03.export" }
func (r *RPT03Bank) RegulatedFlag() bool       { return false }
func (r *RPT03Bank) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "nama_bank", Dir: "asc"}}
}
func (r *RPT03Bank) AllowedSort() []string {
	return []string{"kode_bank", "nama_bank", "negara", "created_at"}
}
func (r *RPT03Bank) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "negara"}, {Col: "tenant_id"}}
}
func (r *RPT03Bank) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{
		{Key: "kode_bank", Header: "Kode Bank", Format: "text"},
		{Key: "nama_bank", Header: "Nama Bank", Format: "text"},
		{Key: "negara", Header: "Negara", Format: "text"},
		{Key: "created_at", Header: "Dibuat", Format: "datetime"},
	}
}
func (r *RPT03Bank) Query(ctx context.Context, db *sql.DB, params reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
	where, args, _, err := reports.BuildWhere(params.Filters, r.AllowedSort(), 1)
	if err != nil {
		return nil, 0, err
	}
	cond := "WHERE b.deleted_at IS NULL"
	if where != "" {
		cond += " AND " + where
	}
	order := reports.BuildOrderBy(params.Sort, r.AllowedSort(), r.DefaultSort())
	limit := params.Limit + 1
	var total int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM mst.bank b %s", cond), args...).Scan(&total)
	lArgs := append(args, limit) //nolint:gocritic
	rows, err := db.QueryContext(ctx, fmt.Sprintf(
		`SELECT kode_bank, nama_bank, COALESCE(negara,'') AS negara, b.created_at FROM mst.bank b %s %s LIMIT $%d`,
		cond, order, len(lArgs)), lArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("rpt-03 query: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	seq, err := reports.ScanRowsToMaps(rows)
	return seq, total, err
}
