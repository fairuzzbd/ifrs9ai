package impl

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT07MTMDaily{}) }

// RPT07MTMDaily — MTM Daily (trx.mtm_adjustment). Hot index: (tanggal_mtm, instrumen_id, tenant_id).
type RPT07MTMDaily struct{}

func (r *RPT07MTMDaily) Slug() string             { return "rpt-07" }
func (r *RPT07MTMDaily) Permission() string        { return "report.rpt-07.read" }
func (r *RPT07MTMDaily) ExportPermission() string  { return "report.rpt-07.export" }
func (r *RPT07MTMDaily) RegulatedFlag() bool       { return false }
func (r *RPT07MTMDaily) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "tanggal_mtm", Dir: "desc"}}
}
func (r *RPT07MTMDaily) AllowedSort() []string {
	return []string{"tanggal_mtm", "instrumen_id", "harga_pasar_idr", "unrealized_gainloss_idr", "created_at"}
}
func (r *RPT07MTMDaily) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "tanggal_mtm"}, {Col: "instrumen_id"}, {Col: "tenant_id"}}
}
func (r *RPT07MTMDaily) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{
		{Key: "tanggal_mtm", Header: "Tanggal MTM", Format: "date"},
		{Key: "kode_instrumen", Header: "Kode Instrumen", Format: "text"},
		{Key: "harga_pasar_idr", Header: "Harga Pasar (IDR)", Format: "idr"},
		{Key: "unrealized_gainloss_idr", Header: "Unrealized Gain/Loss (IDR)", Format: "idr"},
		{Key: "sumber_harga", Header: "Sumber Harga", Format: "text"},
	}
}
func (r *RPT07MTMDaily) Query(ctx context.Context, db *sql.DB, params reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
	where, args, _, err := reports.BuildWhere(params.Filters, r.AllowedSort(), 1)
	if err != nil {
		return nil, 0, err
	}
	cond := "WHERE m.deleted_at IS NULL"
	if where != "" {
		cond += " AND " + where
	}
	order := reports.BuildOrderBy(params.Sort, r.AllowedSort(), r.DefaultSort())
	limit := params.Limit + 1
	var total int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM trx.mtm_adjustment m %s", cond), args...).Scan(&total)
	lArgs := append(args, limit) //nolint:gocritic
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT m.tanggal_mtm, i.kode_instrumen, m.harga_pasar_idr,
		       m.unrealized_gainloss_idr, COALESCE(m.sumber_harga,'') AS sumber_harga
		FROM trx.mtm_adjustment m
		LEFT JOIN mst.instrumen i ON i.id = m.instrumen_id
		%s %s LIMIT $%d`, cond, order, len(lArgs)), lArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("rpt-07 query: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	seq, err := reports.ScanRowsToMaps(rows)
	return seq, total, err
}
