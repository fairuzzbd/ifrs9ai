package impl

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT23PeriodeCloseAudit{}) }

// RPT23PeriodeCloseAudit — filter aud.audit_log WHERE action='PERIODE.HARD_CLOSE'.
type RPT23PeriodeCloseAudit struct{}

func (r *RPT23PeriodeCloseAudit) Slug() string             { return "rpt-23" }
func (r *RPT23PeriodeCloseAudit) Permission() string        { return "report.rpt-23.read" }
func (r *RPT23PeriodeCloseAudit) ExportPermission() string  { return "report.rpt-23.export" }
func (r *RPT23PeriodeCloseAudit) RegulatedFlag() bool       { return false }
func (r *RPT23PeriodeCloseAudit) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "event_time", Dir: "desc"}}
}
func (r *RPT23PeriodeCloseAudit) AllowedSort() []string {
	return []string{"event_time", "actor_user_id", "actor_role", "action"}
}
func (r *RPT23PeriodeCloseAudit) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "action"}, {Col: "event_time"}, {Col: "tenant_id"}}
}
func (r *RPT23PeriodeCloseAudit) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{
		{Key: "event_time", Header: "Waktu", Format: "datetime"},
		{Key: "actor_user_id", Header: "User ID", Format: "text"},
		{Key: "actor_role", Header: "Role", Format: "text"},
		{Key: "action", Header: "Action", Format: "text"},
		{Key: "mfa_method", Header: "MFA Method", Format: "text"},
		{Key: "ip", Header: "IP", Format: "text"},
		{Key: "trace_id", Header: "Trace ID", Format: "text"},
	}
}
func (r *RPT23PeriodeCloseAudit) Query(ctx context.Context, db *sql.DB, params reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
	where, args, _, err := reports.BuildWhere(params.Filters, r.AllowedSort(), 1)
	if err != nil {
		return nil, 0, err
	}
	// Always filter to PERIODE.HARD_CLOSE action.
	cond := "WHERE al.action = 'PERIODE.HARD_CLOSE'"
	if where != "" {
		cond += " AND " + where
	}
	order := reports.BuildOrderBy(params.Sort, r.AllowedSort(), r.DefaultSort())
	limit := params.Limit + 1
	var total int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM aud.audit_log al %s", cond), args...).Scan(&total)
	lArgs := append(args, limit) //nolint:gocritic
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT al.event_time, al.actor_user_id::text, al.actor_role, al.action,
		       COALESCE(al.after_jsonb->>'mfa_method','') AS mfa_method,
		       COALESCE(al.ip::text,'') AS ip,
		       COALESCE(al.trace_id,'') AS trace_id
		FROM aud.audit_log al %s %s LIMIT $%d`, cond, order, len(lArgs)), lArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("rpt-23 query: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	seq, err := reports.ScanRowsToMaps(rows)
	return seq, total, err
}
