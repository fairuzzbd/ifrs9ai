package impl

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT15SICRTrigger{}) }

type RPT15SICRTrigger struct{}

func (r *RPT15SICRTrigger) Slug() string             { return "rpt-15" }
func (r *RPT15SICRTrigger) Permission() string        { return "report.rpt-15.read" }
func (r *RPT15SICRTrigger) ExportPermission() string  { return "report.rpt-15.export" }
func (r *RPT15SICRTrigger) RegulatedFlag() bool       { return false }
func (r *RPT15SICRTrigger) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "triggered_at", Dir: "desc"}}
}
func (r *RPT15SICRTrigger) AllowedSort() []string {
	return []string{"triggered_at", "instrumen_id", "trigger_type", "created_at"}
}
func (r *RPT15SICRTrigger) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "instrumen_id"}, {Col: "trigger_type"}, {Col: "tenant_id"}}
}
func (r *RPT15SICRTrigger) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{
		{Key: "kode_instrumen", Header: "Kode Instrumen", Format: "text"},
		{Key: "trigger_type", Header: "Tipe Trigger", Format: "text"},
		{Key: "triggered_at", Header: "Triggered At", Format: "datetime"},
		{Key: "detail", Header: "Detail", Format: "text"},
	}
}
func (r *RPT15SICRTrigger) Query(ctx context.Context, db *sql.DB, params reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
	where, args, _, err := reports.BuildWhere(params.Filters, r.AllowedSort(), 1)
	if err != nil {
		return nil, 0, err
	}
	cond := "WHERE s.deleted_at IS NULL"
	if where != "" {
		cond += " AND " + where
	}
	order := reports.BuildOrderBy(params.Sort, r.AllowedSort(), r.DefaultSort())
	limit := params.Limit + 1
	var total int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM ecl.sicr_trigger_log s %s", cond), args...).Scan(&total)
	lArgs := append(args, limit) //nolint:gocritic
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT i.kode_instrumen, s.trigger_type, s.triggered_at, COALESCE(s.detail,'') AS detail
		FROM ecl.sicr_trigger_log s LEFT JOIN mst.instrumen i ON i.id = s.instrumen_id
		%s %s LIMIT $%d`, cond, order, len(lArgs)), lArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("rpt-15 query: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	seq, err := reports.ScanRowsToMaps(rows)
	return seq, total, err
}
