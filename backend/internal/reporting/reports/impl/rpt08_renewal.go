package impl

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT08Renewal{}) }

type RPT08Renewal struct{}

func (r *RPT08Renewal) Slug() string             { return "rpt-08" }
func (r *RPT08Renewal) Permission() string        { return "report.rpt-08.read" }
func (r *RPT08Renewal) ExportPermission() string  { return "report.rpt-08.export" }
func (r *RPT08Renewal) RegulatedFlag() bool       { return false }
func (r *RPT08Renewal) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "tanggal_renewal", Dir: "desc"}}
}
func (r *RPT08Renewal) AllowedSort() []string {
	return []string{"tanggal_renewal", "instrumen_id", "nominal_pokok_idr", "kupon_baru_persen", "workflow_status", "created_at"}
}
func (r *RPT08Renewal) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "workflow_status"}, {Col: "periode_id"}, {Col: "instrumen_id"}, {Col: "tenant_id"}}
}
func (r *RPT08Renewal) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{
		{Key: "tanggal_renewal", Header: "Tanggal Renewal", Format: "date"},
		{Key: "kode_instrumen", Header: "Kode Instrumen", Format: "text"},
		{Key: "nominal_pokok_idr", Header: "Nominal Pokok (IDR)", Format: "idr"},
		{Key: "kupon_baru_persen", Header: "Kupon Baru (%)", Format: "pct"},
		{Key: "workflow_status", Header: "Status", Format: "text"},
	}
}
func (r *RPT08Renewal) Query(ctx context.Context, db *sql.DB, params reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
	where, args, _, err := reports.BuildWhere(params.Filters, r.AllowedSort(), 1)
	if err != nil {
		return nil, 0, err
	}
	cond := "WHERE rn.deleted_at IS NULL"
	if where != "" {
		cond += " AND " + where
	}
	order := reports.BuildOrderBy(params.Sort, r.AllowedSort(), r.DefaultSort())
	limit := params.Limit + 1
	var total int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM trx.renewal rn %s", cond), args...).Scan(&total)
	lArgs := append(args, limit) //nolint:gocritic
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT rn.tanggal_renewal, i.kode_instrumen, rn.nominal_pokok_idr, rn.kupon_baru_persen, rn.workflow_status
		FROM trx.renewal rn LEFT JOIN mst.instrumen i ON i.id = rn.instrumen_id
		%s %s LIMIT $%d`, cond, order, len(lArgs)), lArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("rpt-08 query: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	seq, err := reports.ScanRowsToMaps(rows)
	return seq, total, err
}
