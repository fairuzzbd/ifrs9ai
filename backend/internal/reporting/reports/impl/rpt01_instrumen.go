// Package impl contains all 25 Report interface implementations for P5-M14.
package impl

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT01Instrumen{}) }

// RPT01Instrumen — Daftar Instrumen (mst.instrumen).
type RPT01Instrumen struct{}

func (r *RPT01Instrumen) Slug() string             { return "rpt-01" }
func (r *RPT01Instrumen) Permission() string        { return "report.rpt-01.read" }
func (r *RPT01Instrumen) ExportPermission() string  { return "report.rpt-01.export" }
func (r *RPT01Instrumen) RegulatedFlag() bool       { return false }
func (r *RPT01Instrumen) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "created_at", Dir: "desc"}}
}
func (r *RPT01Instrumen) AllowedSort() []string {
	return []string{"kode_instrumen", "nama", "jenis_instrumen", "klasifikasi_psak71", "stage", "ead_idr", "tanggal_jatuh_tempo", "created_at"}
}
func (r *RPT01Instrumen) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{
		{Col: "jenis_instrumen"}, {Col: "klasifikasi_psak71"}, {Col: "stage"},
		{Col: "tanggal_jatuh_tempo"}, {Col: "tenant_id"},
	}
}
func (r *RPT01Instrumen) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{
		{Key: "kode_instrumen", Header: "Kode Instrumen", Format: "text"},
		{Key: "nama", Header: "Nama", Format: "text"},
		{Key: "jenis_instrumen", Header: "Jenis", Format: "text"},
		{Key: "klasifikasi_psak71", Header: "Klasifikasi PSAK 71", Format: "text"},
		{Key: "stage", Header: "Stage ECL", Format: "text"},
		{Key: "ead_idr", Header: "EAD (IDR)", Format: "idr"},
		{Key: "tanggal_jatuh_tempo", Header: "Jatuh Tempo", Format: "date"},
		{Key: "created_at", Header: "Dibuat", Format: "datetime"},
	}
}

func (r *RPT01Instrumen) Query(ctx context.Context, db *sql.DB, params reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
	where, args, _, err := reports.BuildWhere(params.Filters, r.AllowedSort(), 1)
	if err != nil {
		return nil, 0, err
	}
	cond := "WHERE i.deleted_at IS NULL"
	if where != "" {
		cond += " AND " + where
	}
	order := reports.BuildOrderBy(params.Sort, r.AllowedSort(), r.DefaultSort())
	limit := params.Limit + 1

	var total int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM mst.instrumen i %s", cond), args...).Scan(&total)

	lArgs := append(args, limit) //nolint:gocritic
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT kode_instrumen, nama, jenis_instrumen, klasifikasi_psak71,
		       COALESCE(stage::text,'') AS stage, ead_idr, tanggal_jatuh_tempo, i.created_at
		FROM mst.instrumen i %s %s LIMIT $%d`, cond, order, len(lArgs)), lArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("rpt-01 query: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	seq, err := reports.ScanRowsToMaps(rows)
	if err != nil {
		return nil, 0, err
	}
	return seq, total, nil
}
