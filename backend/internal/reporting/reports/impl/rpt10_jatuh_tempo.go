package impl

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT10JatuhTempo{}) }

type RPT10JatuhTempo struct{}

func (r *RPT10JatuhTempo) Slug() string             { return "rpt-10" }
func (r *RPT10JatuhTempo) Permission() string        { return "report.rpt-10.read" }
func (r *RPT10JatuhTempo) ExportPermission() string  { return "report.rpt-10.export" }
func (r *RPT10JatuhTempo) RegulatedFlag() bool       { return false }
func (r *RPT10JatuhTempo) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "tanggal_jatuh_tempo", Dir: "desc"}}
}
func (r *RPT10JatuhTempo) AllowedSort() []string {
	return []string{"tanggal_jatuh_tempo", "instrumen_id", "nominal_idr", "status", "created_at"}
}
func (r *RPT10JatuhTempo) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "status"}, {Col: "periode_id"}, {Col: "instrumen_id"}, {Col: "tenant_id"}}
}
func (r *RPT10JatuhTempo) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{
		{Key: "tanggal_jatuh_tempo", Header: "Tanggal Jatuh Tempo", Format: "date"},
		{Key: "kode_instrumen", Header: "Kode Instrumen", Format: "text"},
		{Key: "nominal_idr", Header: "Nominal (IDR)", Format: "idr"},
		{Key: "status", Header: "Status", Format: "text"},
	}
}
func (r *RPT10JatuhTempo) Query(ctx context.Context, db *sql.DB, params reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
	where, args, _, err := reports.BuildWhere(params.Filters, r.AllowedSort(), 1)
	if err != nil {
		return nil, 0, err
	}
	cond := "WHERE jt.deleted_at IS NULL"
	if where != "" {
		cond += " AND " + where
	}
	order := reports.BuildOrderBy(params.Sort, r.AllowedSort(), r.DefaultSort())
	limit := params.Limit + 1
	var total int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM trx.jatuh_tempo jt %s", cond), args...).Scan(&total)
	lArgs := append(args, limit) //nolint:gocritic
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT jt.tanggal_jatuh_tempo, i.kode_instrumen, jt.nominal_idr, COALESCE(jt.status,'') AS status
		FROM trx.jatuh_tempo jt LEFT JOIN mst.instrumen i ON i.id = jt.instrumen_id
		%s %s LIMIT $%d`, cond, order, len(lArgs)), lArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("rpt-10 query: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	seq, err := reports.ScanRowsToMaps(rows)
	return seq, total, err
}
