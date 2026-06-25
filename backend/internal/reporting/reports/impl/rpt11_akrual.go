package impl

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT11AkrualHarian{}) }

// RPT11AkrualHarian — Akrual Harian (trx.akrual). Requires filter to avoid full scan.
type RPT11AkrualHarian struct{}

func (r *RPT11AkrualHarian) Slug() string             { return "rpt-11" }
func (r *RPT11AkrualHarian) Permission() string        { return "report.rpt-11.read" }
func (r *RPT11AkrualHarian) ExportPermission() string  { return "report.rpt-11.export" }
func (r *RPT11AkrualHarian) RegulatedFlag() bool       { return false }
func (r *RPT11AkrualHarian) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "tanggal_akrual", Dir: "desc"}}
}
func (r *RPT11AkrualHarian) AllowedSort() []string {
	return []string{"tanggal_akrual", "instrumen_id", "jumlah_idr", "jenis_akrual", "periode_id", "created_at"}
}
func (r *RPT11AkrualHarian) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "jenis_akrual"}, {Col: "periode_id"}, {Col: "instrumen_id"}, {Col: "tanggal_akrual"}, {Col: "tenant_id"}}
}
func (r *RPT11AkrualHarian) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{
		{Key: "tanggal_akrual", Header: "Tanggal Akrual", Format: "date"},
		{Key: "kode_instrumen", Header: "Kode Instrumen", Format: "text"},
		{Key: "jenis_akrual", Header: "Jenis Akrual", Format: "text"},
		{Key: "jumlah_idr", Header: "Jumlah (IDR)", Format: "idr"},
	}
}
func (r *RPT11AkrualHarian) Query(ctx context.Context, db *sql.DB, params reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
	where, args, _, err := reports.BuildWhere(params.Filters, r.AllowedSort(), 1)
	if err != nil {
		return nil, 0, err
	}
	cond := "WHERE ak.deleted_at IS NULL"
	if where != "" {
		cond += " AND " + where
	}
	order := reports.BuildOrderBy(params.Sort, r.AllowedSort(), r.DefaultSort())
	limit := params.Limit + 1
	var total int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM trx.akrual ak %s", cond), args...).Scan(&total)
	lArgs := append(args, limit) //nolint:gocritic
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT ak.tanggal_akrual, i.kode_instrumen, ak.jenis_akrual, ak.jumlah_idr
		FROM trx.akrual ak LEFT JOIN mst.instrumen i ON i.id = ak.instrumen_id
		%s %s LIMIT $%d`, cond, order, len(lArgs)), lArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("rpt-11 query: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	seq, err := reports.ScanRowsToMaps(rows)
	return seq, total, err
}
