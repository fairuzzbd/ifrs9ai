package impl

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT22bGLDelivery{}) }

// RPT22bGLDelivery — GL Delivery Status via rpt.mv_gl_delivery_status (M13 MV).
type RPT22bGLDelivery struct{}

func (r *RPT22bGLDelivery) Slug() string             { return "rpt-22b" }
func (r *RPT22bGLDelivery) Permission() string        { return "report.rpt-22b.read" }
func (r *RPT22bGLDelivery) ExportPermission() string  { return "report.rpt-22b.export" }
func (r *RPT22bGLDelivery) RegulatedFlag() bool       { return false }
func (r *RPT22bGLDelivery) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "last_attempt_at", Dir: "desc"}}
}
func (r *RPT22bGLDelivery) AllowedSort() []string {
	return []string{"last_attempt_at", "gl_host_status", "periode_id", "attempt_count", "created_at"}
}
func (r *RPT22bGLDelivery) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "gl_host_status"}, {Col: "periode_id"}, {Col: "tenant_id"}}
}
func (r *RPT22bGLDelivery) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{
		{Key: "delivery_id", Header: "Delivery ID", Format: "text"},
		{Key: "jurnal_header_id", Header: "Jurnal ID", Format: "text"},
		{Key: "gl_host_status", Header: "Status Delivery", Format: "text"},
		{Key: "attempt_count", Header: "Attempt", Format: "text"},
		{Key: "last_attempt_at", Header: "Last Attempt", Format: "datetime"},
		{Key: "delivered_at", Header: "Delivered At", Format: "datetime"},
	}
}
func (r *RPT22bGLDelivery) Query(ctx context.Context, db *sql.DB, params reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
	where, args, _, err := reports.BuildWhere(params.Filters, r.AllowedSort(), 1)
	if err != nil {
		return nil, 0, err
	}
	cond := ""
	if where != "" {
		cond = "WHERE " + where
	}
	order := reports.BuildOrderBy(params.Sort, r.AllowedSort(), r.DefaultSort())
	limit := params.Limit + 1
	var total int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM rpt.mv_gl_delivery_status %s", cond), args...).Scan(&total)
	lArgs := append(args, limit) //nolint:gocritic
	rows, err := db.QueryContext(ctx, fmt.Sprintf(
		`SELECT delivery_id, jurnal_header_id, gl_host_status, attempt_count, last_attempt_at, delivered_at
		 FROM rpt.mv_gl_delivery_status %s %s LIMIT $%d`, cond, order, len(lArgs)), lArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("rpt-22b query: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	seq, err := reports.ScanRowsToMaps(rows)
	return seq, total, err
}
