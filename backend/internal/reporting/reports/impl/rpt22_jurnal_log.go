package impl

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT22JurnalLog{}) }

type RPT22JurnalLog struct{}

func (r *RPT22JurnalLog) Slug() string             { return "rpt-22" }
func (r *RPT22JurnalLog) Permission() string        { return "report.rpt-22.read" }
func (r *RPT22JurnalLog) ExportPermission() string  { return "report.rpt-22.export" }
func (r *RPT22JurnalLog) RegulatedFlag() bool       { return false }
func (r *RPT22JurnalLog) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "posted_at", Dir: "desc"}}
}
func (r *RPT22JurnalLog) AllowedSort() []string {
	return []string{"posted_at", "event_code", "instrumen_id", "nominal_idr", "status_posting"}
}
func (r *RPT22JurnalLog) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "event_code"}, {Col: "periode_id"}, {Col: "instrumen_id"}, {Col: "status_posting"}, {Col: "tenant_id"}}
}
func (r *RPT22JurnalLog) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{
		{Key: "jurnal_id", Header: "Jurnal ID", Format: "text"},
		{Key: "event_code", Header: "Event Code", Format: "text"},
		{Key: "kode_instrumen", Header: "Kode Instrumen", Format: "text"},
		{Key: "debit_account", Header: "Akun Debit", Format: "text"},
		{Key: "kredit_account", Header: "Akun Kredit", Format: "text"},
		{Key: "nominal_idr", Header: "Nominal (IDR)", Format: "idr"},
		{Key: "posted_at", Header: "Posted At", Format: "datetime"},
		{Key: "posted_by", Header: "Posted By", Format: "text"},
		{Key: "status_posting", Header: "Status", Format: "text"},
	}
}
func (r *RPT22JurnalLog) Query(ctx context.Context, db *sql.DB, params reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
	where, args, _, err := reports.BuildWhere(params.Filters, r.AllowedSort(), 1)
	if err != nil {
		return nil, 0, err
	}
	cond := "WHERE jh.deleted_at IS NULL"
	if where != "" {
		cond += " AND " + where
	}
	order := reports.BuildOrderBy(params.Sort, r.AllowedSort(), r.DefaultSort())
	limit := params.Limit + 1
	var total int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM jrnl.jurnal_header jh %s", cond), args...).Scan(&total)
	lArgs := append(args, limit) //nolint:gocritic
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT jh.id AS jurnal_id, jh.event_code,
		       COALESCE(i.kode_instrumen,'') AS kode_instrumen,
		       COALESCE(jd.kode_akun_debit,'') AS debit_account,
		       COALESCE(jd.kode_akun_kredit,'') AS kredit_account,
		       COALESCE(jd.amount_idr,0) AS nominal_idr,
		       jh.created_at AS posted_at, jh.created_by::text AS posted_by,
		       COALESCE(jh.workflow_status,'') AS status_posting
		FROM jrnl.jurnal_header jh
		LEFT JOIN mst.instrumen i ON i.id = jh.instrumen_id
		LEFT JOIN LATERAL (
		    SELECT kode_akun_debit, kode_akun_kredit, amount_idr
		    FROM jrnl.jurnal_detail jd2 WHERE jd2.jurnal_header_id = jh.id AND jd2.deleted_at IS NULL LIMIT 1
		) jd ON TRUE
		%s %s LIMIT $%d`, cond, order, len(lArgs)), lArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("rpt-22 query: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	seq, err := reports.ScanRowsToMaps(rows)
	return seq, total, err
}
