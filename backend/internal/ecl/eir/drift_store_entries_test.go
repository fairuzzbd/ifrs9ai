// drift_store_entries_test.go — B4 fix: storeDriftEntries error marks report FAILED.
//
// Test:
//
//	TestGenerateReport_StoreDriftEntriesFails_MarksReportFailed
//	  — when the UPDATE sys.drift_report (storeDriftEntries) fails, GenerateReport
//	    must rollback finishTx, transition report to FAILED via a new tx, and return error.
//
// Regression guard: previously the error was silently discarded (nolint:errcheck intentional
// best-effort), leaving the drift report COMPLETED but without detail JSON.
//
// References:
//   - drift_service.go §GenerateReport (B4 fix)
//   - FSD-APP-C §M6-002 (drift report completeness requirement)
//   - DEC-018 (no orphan audit records).
package eir

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// newDriftMockStoreFails sets up the mock db for the storeDriftEntries failure path.
//
// Transaction flow:
//
//	tx1: Begin + Commit  — create IN_PROGRESS (driftRepo.Create is stub, no ExecContext)
//	tx2: Begin
//	       storeDriftEntries executes UPDATE sys.drift_report → FAILS (mock returns error)
//	     Rollback
//	tx3: Begin + Commit  — failTx: driftRepo.Update is the stub (no ExecContext on mock)
//	       + auditWriter.Write is stub (no ExecContext on mock)
//
// The stub driftRepo.Update and stub auditWriter.Write do NOT issue ExecContext calls on
// the mock tx, so tx3 only needs Begin+Commit.
func newDriftMockStoreFails(t *testing.T) (*sql.DB, sqlmock.Sqlmock) { //nolint:unparam
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	// tx1: create IN_PROGRESS (stub driftRepo.Create — no ExecContext)
	mock.ExpectBegin()
	mock.ExpectCommit()
	// tx2: finishTx — storeDriftEntries UPDATE fails
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE sys.drift_report")).
		WillReturnError(fmt.Errorf("injected storeDriftEntries failure"))
	mock.ExpectRollback()
	// tx3: failTx — stub driftRepo.Update (no ExecContext); auditWriter stub (no ExecContext)
	mock.ExpectBegin()
	mock.ExpectCommit()

	t.Cleanup(func() { db.Close() })
	return db, mock
}

// TestGenerateReport_StoreDriftEntriesFails_MarksReportFailed verifies that if
// the UPDATE inside storeDriftEntries returns an error:
//   - finishTx is rolled back
//   - a new tx opens, marks report status=FAILED with error_summary set
//   - GenerateReport returns a non-nil error
func TestGenerateReport_StoreDriftEntriesFails_MarksReportFailed(t *testing.T) {
	instrID := uuid.New()
	eirVal := decimal.NewFromFloat(0.08028915)

	// Instrument with a 1-row schedule → no drift (we just need to reach storeDriftEntries).
	schedRepo := newDriftScheduleRepo()
	baseDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	schedRepo.addRows(instrID, []ScheduleRow{
		{
			ID:              uuid.New(),
			InstrumenID:     instrID,
			PeriodeSeq:      1,
			TanggalPosting:  baseDate,
			OpeningCarrying: decimal.NewFromFloat(1_000_000),
			CashInflow:      decimal.NewFromFloat(1_080_291.5),
		},
	})

	instrRepo := &driftInstrRepo{instruments: []InstrumenForEIR{
		{
			ID:                instrID,
			KodeInstrumen:     "BOND-STOREFAIL",
			KlasifikasiPsak71: "AC",
			EIRMethodFlag:     true,
			EIRAwal:           &eirVal,
			Status:            "ACTIVE",
			TenantID:          "TUGURE",
		},
	}}

	driftRepo := newStubDriftRepo()
	amendRepo := newStubAmendmentRepo()
	db, mock := newDriftMockStoreFails(t)

	svc := NewDriftService(db, instrRepo, schedRepo, amendRepo, driftRepo, NewSolver(), stubAuditW(), slog.Default())
	triggered := uuid.New()
	_, err := svc.GenerateReport(context.Background(), DriftGenerateRequest{
		TriggerSource: DriftTriggerManualAdHoc,
		TriggeredBy:   &triggered,
		TenantID:      "TUGURE",
	})
	if err == nil {
		t.Fatal("expected error when storeDriftEntries fails, got nil")
	}
	// Verify the report in the stub driftRepo is marked FAILED.
	var failedReport *DriftReport
	for _, r := range driftRepo.reports {
		if r.Status == DriftStatusFailed {
			failedReport = r
			break
		}
	}
	if failedReport == nil {
		t.Error("expected drift report to be marked FAILED in driftRepo, but no FAILED report found")
	} else if failedReport.ErrorSummary == nil || *failedReport.ErrorSummary == "" {
		t.Error("expected ErrorSummary to be set on FAILED report")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}
