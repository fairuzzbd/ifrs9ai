package impl

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT25AuditLog{}) }

// RPT25AuditLog — full aud.audit_log browser. ROLE-AUDIT only.
// before_jsonb/after_jsonb excluded from columns (returned only to audit_log.read permission in handler).
type RPT25AuditLog struct{}

func (r *RPT25AuditLog) Slug() string             { return "rpt-25" }
func (r *RPT25AuditLog) Permission() string        { return "report.rpt-25.read" }
func (r *RPT25AuditLog) ExportPermission() string  { return "report.rpt-25.export" }
func (r *RPT25AuditLog) RegulatedFlag() bool       { return false }
func (r *RPT25AuditLog) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "event_time", Dir: "desc"}}
}
func (r *RPT25AuditLog) AllowedSort() []string {
	return []string{"event_time", "actor_user_id", "actor_role", "action", "entity_type", "entity_id"}
}
func (r *RPT25AuditLog) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "action"}, {Col: "actor_role"}, {Col: "entity_type"}, {Col: "event_time"}, {Col: "tenant_id"}}
}
func (r *RPT25AuditLog) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{
		{Key: "event_id", Header: "Event ID", Format: "text"},
		{Key: "event_time", Header: "Waktu", Format: "datetime"},
		{Key: "actor_user_id", Header: "User ID", Format: "text"},
		{Key: "actor_role", Header: "Role", Format: "text"},
		{Key: "action", Header: "Action", Format: "text"},
		{Key: "entity_type", Header: "Entity Type", Format: "text"},
		{Key: "entity_id", Header: "Entity ID", Format: "text"},
		{Key: "ip", Header: "IP", Format: "text"},
		{Key: "trace_id", Header: "Trace ID", Format: "text"},
	}
}
func (r *RPT25AuditLog) Query(ctx context.Context, db *sql.DB, params reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
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
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM aud.audit_log al %s", cond), args...).Scan(&total)
	lArgs := append(args, limit) //nolint:gocritic
	// before_jsonb/after_jsonb NOT selected here; handler adds them only for audit_log.read perm.
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT al.event_id::text, al.event_time, al.actor_user_id::text, al.actor_role,
		       al.action, al.entity_type, al.entity_id::text,
		       COALESCE(al.ip::text,'') AS ip,
		       COALESCE(al.trace_id,'') AS trace_id
		FROM aud.audit_log al %s %s LIMIT $%d`, cond, order, len(lArgs)), lArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("rpt-25 query: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	seq, err := reports.ScanRowsToMaps(rows)
	return seq, total, err
}
