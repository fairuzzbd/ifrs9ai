package impl

import (
	"context"
	"database/sql"
	"iter"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

func init() { reports.Register(&RPT28RegulatorPack{}) }

// RPT28RegulatorPack is a placeholder Report entry in the registry.
// The actual composite XLSX generation (4 sheets: Jurnal, ECL Summary, EIR Schedule, GL Delivery)
// is handled by the dedicated Asynq worker "reports:rpt28-regulator-pack" which:
//   - Assembles rows from RPT-13 + RPT-18 + RPT-22 + RPT-22b for the requested periode_id.
//   - Adds SHA-256 hash + text watermark to cover sheet.
//   - Uploads to MinIO exports/{tenantID}/{actorID}/{yyyy/mm/dd}/{jobID}.xlsx.
//   - Writes audit EXPORT.REGULATOR_PACK_GENERATED in-transaction.
//   - Sends SMTP email to actor with signed download URL.
//
// Query() is intentionally a no-op for RPT-28 (always-async path via POST /reports/rpt-28/export).
// The handler bypasses the generic Export() service method and calls ExportRegulatorPack() directly.
//
// MFA step-up required: X-Step-Up-Token header (DEC-027, scope=regulator_pack).
// Permission: report.rpt-28.export (ROLE-CFO only).
type RPT28RegulatorPack struct{}

func (r *RPT28RegulatorPack) Slug() string             { return "rpt-28" }
func (r *RPT28RegulatorPack) Permission() string        { return "report.rpt-28.read" }
func (r *RPT28RegulatorPack) ExportPermission() string  { return "report.rpt-28.export" }
func (r *RPT28RegulatorPack) RegulatedFlag() bool       { return true } // EXPORT.REGULATOR_PACK_GENERATED audit
func (r *RPT28RegulatorPack) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{}
}
func (r *RPT28RegulatorPack) AllowedSort() []string    { return []string{} }
func (r *RPT28RegulatorPack) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "periode_id"}}
}
func (r *RPT28RegulatorPack) Columns() []reports.ColumnSpec { return []reports.ColumnSpec{} }

// Query is a no-op for RPT-28 (always-async). The service bypasses this for RPT-28.
func (r *RPT28RegulatorPack) Query(_ context.Context, _ *sql.DB, _ reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
	// RPT-28 is always dispatched via POST /reports/rpt-28/export → ExportRegulatorPack.
	// This method should not be called in normal flow.
	return func(yield func(map[string]any) bool) {}, 0, nil
}
