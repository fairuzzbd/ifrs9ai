package impl

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT26WorkflowPending{}) }

// RPT26WorkflowPending — cross-module union of pending approval items.
type RPT26WorkflowPending struct{}

func (r *RPT26WorkflowPending) Slug() string             { return "rpt-26" }
func (r *RPT26WorkflowPending) Permission() string        { return "report.rpt-26.read" }
func (r *RPT26WorkflowPending) ExportPermission() string  { return "report.rpt-26.export" }
func (r *RPT26WorkflowPending) RegulatedFlag() bool       { return false }
func (r *RPT26WorkflowPending) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "submitted_at", Dir: "desc"}}
}
func (r *RPT26WorkflowPending) AllowedSort() []string {
	return []string{"submitted_at", "entity_type", "workflow_status", "maker_id"}
}
func (r *RPT26WorkflowPending) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "entity_type"}, {Col: "workflow_status"}, {Col: "tenant_id"}}
}
func (r *RPT26WorkflowPending) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{
		{Key: "entity_id", Header: "Entity ID", Format: "text"},
		{Key: "entity_type", Header: "Tipe Entity", Format: "text"},
		{Key: "workflow_status", Header: "Status Workflow", Format: "text"},
		{Key: "submitted_at", Header: "Submitted At", Format: "datetime"},
		{Key: "maker_id", Header: "Maker ID", Format: "text"},
	}
}
func (r *RPT26WorkflowPending) Query(ctx context.Context, db *sql.DB, params reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
	// UNION of pending items from all workflow-bearing tables.
	// Only SUBMITTED/UNDER_REVIEW statuses.
	limit := params.Limit + 1
	union := `
		SELECT id::text AS entity_id, 'penempatan' AS entity_type, workflow_status, created_at AS submitted_at, maker_id::text
		FROM trx.penempatan WHERE workflow_status IN ('SUBMITTED','UNDER_REVIEW') AND deleted_at IS NULL
		UNION ALL
		SELECT id::text, 'penjualan_pencairan', workflow_status, created_at, maker_id::text
		FROM trx.penjualan_pencairan WHERE workflow_status IN ('SUBMITTED','UNDER_REVIEW') AND deleted_at IS NULL
		UNION ALL
		SELECT id::text, 'renewal', workflow_status, created_at, maker_id::text
		FROM trx.renewal WHERE workflow_status IN ('SUBMITTED','UNDER_REVIEW') AND deleted_at IS NULL
		UNION ALL
		SELECT id::text, 'mapping_jurnal', workflow_status, created_at, maker_id::text
		FROM mst.mapping_jurnal_header WHERE workflow_status IN ('SUBMITTED','UNDER_REVIEW') AND deleted_at IS NULL
	`
	var total int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM (%s) t", union)).Scan(&total)

	order := reports.BuildOrderBy(params.Sort, r.AllowedSort(), r.DefaultSort())
	rows, err := db.QueryContext(ctx, fmt.Sprintf(
		`SELECT * FROM (%s) t %s LIMIT $1`, union, order), limit)
	if err != nil {
		return nil, 0, fmt.Errorf("rpt-26 query: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	seq, err := reports.ScanRowsToMaps(rows)
	return seq, total, err
}
