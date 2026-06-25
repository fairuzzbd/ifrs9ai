package impl

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT14StageMovement{}) }

type RPT14StageMovement struct{}

func (r *RPT14StageMovement) Slug() string             { return "rpt-14" }
func (r *RPT14StageMovement) Permission() string        { return "report.rpt-14.read" }
func (r *RPT14StageMovement) ExportPermission() string  { return "report.rpt-14.export" }
func (r *RPT14StageMovement) RegulatedFlag() bool       { return false }
func (r *RPT14StageMovement) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "tanggal_transisi", Dir: "desc"}}
}
func (r *RPT14StageMovement) AllowedSort() []string {
	return []string{"tanggal_transisi", "instrumen_id", "stage_sebelum", "stage_sesudah", "cure_flag", "created_at"}
}
func (r *RPT14StageMovement) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "instrumen_id"}, {Col: "stage_sebelum"}, {Col: "stage_sesudah"}, {Col: "cure_flag"}, {Col: "periode_id"}, {Col: "tenant_id"}}
}
func (r *RPT14StageMovement) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{
		{Key: "kode_instrumen", Header: "Kode Instrumen", Format: "text"},
		{Key: "stage_sebelum", Header: "Stage Sebelum", Format: "text"},
		{Key: "stage_sesudah", Header: "Stage Sesudah", Format: "text"},
		{Key: "tanggal_transisi", Header: "Tanggal Transisi", Format: "date"},
		{Key: "sicr_trigger", Header: "SICR Trigger", Format: "text"},
		{Key: "rating_sebelum", Header: "Rating Sebelum", Format: "text"},
		{Key: "rating_sesudah", Header: "Rating Sesudah", Format: "text"},
		{Key: "dpd", Header: "DPD", Format: "text"},
		{Key: "cure_flag", Header: "Cure", Format: "text"},
	}
}
func (r *RPT14StageMovement) Query(ctx context.Context, db *sql.DB, params reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
	where, args, _, err := reports.BuildWhere(params.Filters, r.AllowedSort(), 1)
	if err != nil {
		return nil, 0, err
	}
	cond := "WHERE sh.deleted_at IS NULL"
	if where != "" {
		cond += " AND " + where
	}
	order := reports.BuildOrderBy(params.Sort, r.AllowedSort(), r.DefaultSort())
	limit := params.Limit + 1
	var total int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM ecl.staging_history sh %s", cond), args...).Scan(&total)
	lArgs := append(args, limit) //nolint:gocritic
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT i.kode_instrumen,
		       sh.stage_sebelum::text, sh.stage_sesudah::text, sh.tanggal_transisi,
		       COALESCE(st.trigger_type,'') AS sicr_trigger,
		       COALESCE(sh.rating_sebelum,'') AS rating_sebelum,
		       COALESCE(sh.rating_sesudah,'') AS rating_sesudah,
		       COALESCE(sh.dpd::text,'') AS dpd,
		       sh.cure_flag::text
		FROM ecl.staging_history sh
		LEFT JOIN mst.instrumen i ON i.id = sh.instrumen_id
		LEFT JOIN ecl.sicr_trigger_log st ON st.staging_history_id = sh.id
		%s %s LIMIT $%d`, cond, order, len(lArgs)), lArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("rpt-14 query: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	seq, err := reports.ScanRowsToMaps(rows)
	return seq, total, err
}
