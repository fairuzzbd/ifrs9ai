package impl

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT02Counterparty{}) }

type RPT02Counterparty struct{}

func (r *RPT02Counterparty) Slug() string             { return "rpt-02" }
func (r *RPT02Counterparty) Permission() string        { return "report.rpt-02.read" }
func (r *RPT02Counterparty) ExportPermission() string  { return "report.rpt-02.export" }
func (r *RPT02Counterparty) RegulatedFlag() bool       { return false }
func (r *RPT02Counterparty) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "created_at", Dir: "desc"}}
}
func (r *RPT02Counterparty) AllowedSort() []string {
	return []string{"nama_counterparty", "kode_counterparty", "tipe_counterparty", "rating_terakhir", "created_at"}
}
func (r *RPT02Counterparty) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "tipe_counterparty"}, {Col: "rating_terakhir"}, {Col: "tenant_id"}}
}
func (r *RPT02Counterparty) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{
		{Key: "kode_counterparty", Header: "Kode", Format: "text"},
		{Key: "nama_counterparty", Header: "Nama Counterparty", Format: "text"},
		{Key: "tipe_counterparty", Header: "Tipe", Format: "text"},
		{Key: "rating_terakhir", Header: "Rating Terakhir", Format: "text"},
		{Key: "created_at", Header: "Dibuat", Format: "datetime"},
	}
}
func (r *RPT02Counterparty) Query(ctx context.Context, db *sql.DB, params reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
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
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM mst.counterparty c %s", cond), args...).Scan(&total)

	lArgs := append(args, limit) //nolint:gocritic
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT kode_counterparty, nama_counterparty, tipe_counterparty,
		       COALESCE(rating_terakhir,'') AS rating_terakhir, c.created_at
		FROM mst.counterparty c %s %s LIMIT $%d`, cond, order, len(lArgs)), lArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("rpt-02 query: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	seq, err := reports.ScanRowsToMaps(rows)
	return seq, total, err
}
