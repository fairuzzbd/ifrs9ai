package impl

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT04COA{}) }

type RPT04COA struct{}

func (r *RPT04COA) Slug() string             { return "rpt-04" }
func (r *RPT04COA) Permission() string        { return "report.rpt-04.read" }
func (r *RPT04COA) ExportPermission() string  { return "report.rpt-04.export" }
func (r *RPT04COA) RegulatedFlag() bool       { return false }
func (r *RPT04COA) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "kode_akun", Dir: "asc"}}
}
func (r *RPT04COA) AllowedSort() []string {
	return []string{"kode_akun", "nama_akun", "tipe_akun", "created_at"}
}
func (r *RPT04COA) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "tipe_akun"}, {Col: "tenant_id"}}
}
func (r *RPT04COA) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{
		{Key: "kode_akun", Header: "Kode Akun", Format: "text"},
		{Key: "nama_akun", Header: "Nama Akun", Format: "text"},
		{Key: "tipe_akun", Header: "Tipe Akun", Format: "text"},
		{Key: "created_at", Header: "Dibuat", Format: "datetime"},
	}
}
func (r *RPT04COA) Query(ctx context.Context, db *sql.DB, params reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
	where, args, _, err := reports.BuildWhere(params.Filters, r.AllowedSort(), 1)
	if err != nil {
		return nil, 0, err
	}
	cond := "WHERE c.deleted_at IS NULL"
	if where != "" {
		cond += " AND " + where
	}
	order := reports.BuildOrderBy(params.Sort, r.AllowedSort(), r.DefaultSort())
	limit := params.Limit + 1
	var total int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM jrnl.coa_mapping c %s", cond), args...).Scan(&total)
	lArgs := append(args, limit) //nolint:gocritic
	rows, err := db.QueryContext(ctx, fmt.Sprintf(
		`SELECT kode_akun, nama_akun, COALESCE(tipe_akun,'') AS tipe_akun, c.created_at FROM jrnl.coa_mapping c %s %s LIMIT $%d`,
		cond, order, len(lArgs)), lArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("rpt-04 query: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	seq, err := reports.ScanRowsToMaps(rows)
	return seq, total, err
}
