package impl

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT17EIRAmortisasi{}) }

// RPT17EIRAmortisasi — EIR Amortisasi Schedule (ecl.amortisasi_schedule). Immutable per DEC-013.
type RPT17EIRAmortisasi struct{}

func (r *RPT17EIRAmortisasi) Slug() string             { return "rpt-17" }
func (r *RPT17EIRAmortisasi) Permission() string        { return "report.rpt-17.read" }
func (r *RPT17EIRAmortisasi) ExportPermission() string  { return "report.rpt-17.export" }
func (r *RPT17EIRAmortisasi) RegulatedFlag() bool       { return false }
func (r *RPT17EIRAmortisasi) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "periode", Dir: "asc"}}
}
func (r *RPT17EIRAmortisasi) AllowedSort() []string {
	return []string{"periode", "instrumen_id", "schedule_version", "saldo_awal", "saldo_akhir"}
}
func (r *RPT17EIRAmortisasi) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "instrumen_id"}, {Col: "schedule_version"}, {Col: "tenant_id"}}
}
func (r *RPT17EIRAmortisasi) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{
		{Key: "periode", Header: "Periode", Format: "date"},
		{Key: "saldo_awal", Header: "Saldo Awal (IDR)", Format: "idr"},
		{Key: "bunga_eir", Header: "Bunga EIR (IDR)", Format: "idr"},
		{Key: "pokok", Header: "Pokok (IDR)", Format: "idr"},
		{Key: "saldo_akhir", Header: "Saldo Akhir (IDR)", Format: "idr"},
		{Key: "effective_from", Header: "Efektif Dari", Format: "date"},
		{Key: "effective_to", Header: "Efektif Sampai", Format: "date"},
	}
}
func (r *RPT17EIRAmortisasi) Query(ctx context.Context, db *sql.DB, params reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
	where, args, _, err := reports.BuildWhere(params.Filters, r.AllowedSort(), 1)
	if err != nil {
		return nil, 0, err
	}
	cond := "WHERE a.deleted_at IS NULL"
	if where != "" {
		cond += " AND " + where
	}
	order := reports.BuildOrderBy(params.Sort, r.AllowedSort(), r.DefaultSort())
	limit := params.Limit + 1
	var total int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM ecl.amortisasi_schedule a %s", cond), args...).Scan(&total)
	lArgs := append(args, limit) //nolint:gocritic
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT a.periode, a.saldo_awal, a.bunga_eir, a.pokok, a.saldo_akhir,
		       a.effective_from, a.effective_to
		FROM ecl.amortisasi_schedule a %s %s LIMIT $%d`, cond, order, len(lArgs)), lArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("rpt-17 query: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	seq, err := reports.ScanRowsToMaps(rows)
	return seq, total, err
}
