// export_test.go — package rollforward (not _test package)
// Exports internal functions and types for white-box testing.
// This file is compiled ONLY during `go test`.
package rollforward

import (
	"database/sql"
	"log/slog"
	"time"

	"github.com/google/uuid"

	_ "github.com/lib/pq" // postgres driver for sql.Open (no actual connection needed in tests)

	"blips-ifrs9.tugu-re.com/internal/audit"
)

// ExportDetectTransfers exposes the internal detectTransfers for testing.
func ExportDetectTransfers(
	prior []ResultLineHeader,
	current []ResultLineHeader,
	history map[uuid.UUID]StageHistoryRow,
) (Transfers, []DataQualityWarning) {
	return detectTransfers(prior, current, history)
}

// ExportDetectLifecycle exposes the internal detectLifecycle for testing.
func ExportDetectLifecycle(
	prior []ResultLineHeader,
	current []ResultLineHeader,
	statuses map[uuid.UUID]InstrumenStatusSnapshot,
	currentCalcRunID uuid.UUID,
	assessmentDate time.Time,
) (Originations, Derecognitions, []DataQualityWarning) {
	return detectLifecycle(prior, current, statuses, currentCalcRunID, assessmentDate)
}

// ExportSetDifference exposes the internal setDifference for testing.
func ExportSetDifference(prior, current []ResultLineHeader) []uuid.UUID {
	return setDifference(prior, current)
}

// ExportErrDomain exposes the internal errDomain for testing.
func ExportErrDomain(code, message string) *domainError {
	return errDomain(code, message)
}

// DomainErrorExported is a type alias that exposes *domainError to the _test package
// so that type assertions work across package boundaries.
type DomainErrorExported = domainError

// ExportXLSXGuardCheck exposes the MISMATCH guard logic from ExportXLSX for testing
// without needing a full Service with a DB connection.
func ExportXLSXGuardCheck(report *RollForwardReport, forceMismatch bool) error {
	if report.ReconcileStatus == ReconcileStatusMismatch && !forceMismatch {
		return errDomain(CodeRollForwardExportMismatchForbidden,
			"Roll-forward tidak reconcile (delta = Rp "+report.ReconcileDeltaIdr.StringFixed(4)+"). Export disclosure formal diblokir.")
	}
	return nil
}

// ExportRollForwardHTTPStatus exposes the internal rollForwardHTTPStatus for testing.
func ExportRollForwardHTTPStatus(code string) int {
	return rollForwardHTTPStatus(code)
}

// NewServiceForTest creates a Service with a fake (unconnected) DB for handler routing tests.
// The DB is opened with a DSN that will never succeed a Ping — callers must not invoke
// any DB-bound service methods. Only use this in tests that exercise routing/validation logic.
func NewServiceForTest() *Service {
	// sql.Open does NOT connect to the database; it defers connection until first use.
	// We use the pgx stdlib driver with an unreachable DSN.
	db, _ := sql.Open("pgx", "host=localhost port=5432 dbname=blips_test user=blips sslmode=disable")
	repo := &Repo{db: db}
	auditWriter := audit.NewWriter(db)
	return &Service{
		repo:        repo,
		db:          db,
		auditWriter: auditWriter,
		logger:      slog.Default(),
	}
}
