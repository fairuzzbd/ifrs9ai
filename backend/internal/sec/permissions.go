// Package sec defines the canonical permission string registry for BLIPS IFRS9.
// Permission format: {entity}.{action} — never role string comparison.
//
// References: security-baseline.md §"Permission model", api-conventions.md §"Auth & Permission".
package sec

// ─── P5-M14 Report Permissions (25 new permission strings) ───────────────────
//
// Pattern: report.{slug}.read  — list/view the report
//          report.{slug}.export — export the report (CSV/XLSX/PDF)
//
// Wildcard: ROLE-AUDIT has "report.*.read" and "report.*.export" (checked via HasPermission loop).
// RPT-25 (Audit Log Browser): ROLE-AUDIT only (permission "report.rpt-25.read").
// RPT-28 (Regulator Pack):    ROLE-CFO only (permission "report.rpt-28.export").

// Master Data report permissions (RPT-01..05)
const (
	PermReportRPT01Read   = "report.rpt-01.read"
	PermReportRPT01Export = "report.rpt-01.export"
	PermReportRPT02Read   = "report.rpt-02.read"
	PermReportRPT02Export = "report.rpt-02.export"
	PermReportRPT03Read   = "report.rpt-03.read"
	PermReportRPT03Export = "report.rpt-03.export"
	PermReportRPT04Read   = "report.rpt-04.read"
	PermReportRPT04Export = "report.rpt-04.export"
	PermReportRPT05Read   = "report.rpt-05.read"
	PermReportRPT05Export = "report.rpt-05.export"
)

// Transaksi report permissions (RPT-06..12)
const (
	PermReportRPT06Read   = "report.rpt-06.read"
	PermReportRPT06Export = "report.rpt-06.export"
	PermReportRPT07Read   = "report.rpt-07.read"
	PermReportRPT07Export = "report.rpt-07.export"
	PermReportRPT08Read   = "report.rpt-08.read"
	PermReportRPT08Export = "report.rpt-08.export"
	PermReportRPT09Read   = "report.rpt-09.read"
	PermReportRPT09Export = "report.rpt-09.export"
	PermReportRPT10Read   = "report.rpt-10.read"
	PermReportRPT10Export = "report.rpt-10.export"
	PermReportRPT11Read   = "report.rpt-11.read"
	PermReportRPT11Export = "report.rpt-11.export"
	PermReportRPT12Read   = "report.rpt-12.read"
	PermReportRPT12Export = "report.rpt-12.export"
)

// ECL/EIR report permissions (RPT-13..18) — ifrs9-compliance-reviewer BLOCKING
const (
	PermReportRPT13Read   = "report.rpt-13.read"
	PermReportRPT13Export = "report.rpt-13.export"
	PermReportRPT14Read   = "report.rpt-14.read"
	PermReportRPT14Export = "report.rpt-14.export"
	PermReportRPT15Read   = "report.rpt-15.read"
	PermReportRPT15Export = "report.rpt-15.export"
	PermReportRPT16Read   = "report.rpt-16.read"
	PermReportRPT16Export = "report.rpt-16.export"
	PermReportRPT17Read   = "report.rpt-17.read"
	PermReportRPT17Export = "report.rpt-17.export"
	PermReportRPT18Read   = "report.rpt-18.read" // regulated: REPORT.RPT18_VIEWED audit
	PermReportRPT18Export = "report.rpt-18.export"
)

// Jurnal/GL report permissions (RPT-22, RPT-22b)
const (
	PermReportRPT22Read   = "report.rpt-22.read"
	PermReportRPT22Export = "report.rpt-22.export"
	PermReportRPT22bRead  = "report.rpt-22b.read"
	PermReportRPT22bExport = "report.rpt-22b.export"
)

// Compliance & Periode report permissions (RPT-23, RPT-25..28)
const (
	PermReportRPT23Read   = "report.rpt-23.read"
	PermReportRPT23Export = "report.rpt-23.export"
	// RPT-25 ROLE-AUDIT only — permission check relies on audit_log.read wildcard.
	PermReportRPT25Read   = "report.rpt-25.read"
	PermReportRPT25Export = "report.rpt-25.export"
	PermReportRPT26Read   = "report.rpt-26.read"
	PermReportRPT26Export = "report.rpt-26.export"
	// RPT-27 regulated: REPORT.RPT27_SENSITIVITY_RUN audit.
	PermReportRPT27Read   = "report.rpt-27.read"
	PermReportRPT27Export = "report.rpt-27.export"
	// RPT-28 ROLE-CFO only + MFA step-up.
	PermReportRPT28Export = "report.rpt-28.export"
)

// ─── Wildcard permissions ──────────────────────────────────────────────────────

// PermReportReadAll is the ROLE-AUDIT wildcard permission string.
// Service checks: claims.HasPermission("report.*.read")
const PermReportReadAll = "report.*.read"

// PermReportExportAll is the ROLE-AUDIT wildcard export permission.
const PermReportExportAll = "report.*.export"

// PermAuditLogRead is the ROLE-AUDIT primary permission (also grants wildcard bypass for reports).
const PermAuditLogRead = "audit_log.read"

// ─── RBAC assignment table (documentation; enforcement is in Keycloak) ────────
//
// Role               → Permissions granted
// ROLE-AUDIT         → report.*.read + report.*.export + audit_log.read
// ROLE-RISK          → report.rpt-{13..18,23,26,27}.read + export
// ROLE-CFO           → all read + report.rpt-28.export
// ROLE-AKUN          → report.rpt-{01..12,22}.read + export
// ROLE-AKUN-CTL      → report.rpt-{22b,23,26}.read + export
// ROLE-APPR-TR       → report.rpt-{06..12}.read (no export)
