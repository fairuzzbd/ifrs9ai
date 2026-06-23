package reports_test

import (
	"context"
	"database/sql"
	"iter"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
	_ "blips-ifrs9.tugu-re.com/internal/reporting/reports/impl"
)

// newMockDB returns a *sql.DB backed by go-sqlmock. It expects one or more
// "SET LOCAL statement_timeout" exec calls (AnyArg). Use for service tests that
// need a non-nil DB but don't actually hit PostgreSQL.
func newMockDB(t *testing.T) *sql.DB {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	// Expect SET LOCAL statement_timeout call from service.List(). AnyTimes so tests
	// that call List once or multiple times all pass.
	mock.ExpectExec("SET LOCAL statement_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
	// Allow additional calls (pagination test calls List once, wildcard test once).
	mock.MatchExpectationsInOrder(false)
	return db
}

// ─── Stub Report ──────────────────────────────────────────────────────────────

type stubReport struct {
	slug          string
	regulatedFlag bool
	rows          []map[string]any
	total         int64
	queryErr      error
}

func (s *stubReport) Slug() string            { return s.slug }
func (s *stubReport) Permission() string       { return "report." + s.slug + ".read" }
func (s *stubReport) ExportPermission() string { return "report." + s.slug + ".export" }
func (s *stubReport) RegulatedFlag() bool      { return s.regulatedFlag }
func (s *stubReport) DefaultSort() []reports.SortSpec {
	return []reports.SortSpec{{Col: "id", Dir: "asc"}}
}
func (s *stubReport) AllowedSort() []string { return []string{"id", "created_at"} }
func (s *stubReport) AllowedFilter() []reports.FilterSpec {
	return []reports.FilterSpec{{Col: "id"}}
}
func (s *stubReport) Columns() []reports.ColumnSpec {
	return []reports.ColumnSpec{{Key: "id", Header: "ID", Format: "text"}}
}
func (s *stubReport) Query(_ context.Context, _ *sql.DB, _ reports.QueryParams) (iter.Seq[map[string]any], int64, error) {
	if s.queryErr != nil {
		return nil, 0, s.queryErr
	}
	idx := 0
	rows := s.rows
	return func(yield func(map[string]any) bool) {
		for idx < len(rows) {
			if !yield(rows[idx]) {
				return
			}
			idx++
		}
	}, s.total, nil
}

// ─── Helper ───────────────────────────────────────────────────────────────────

func newSvc() *reports.ReportService {
	return reports.NewReportService(nil, nil, nil, nil, nil)
}

func newSvcWithDB(db *sql.DB) *reports.ReportService {
	return reports.NewReportService(db, nil, nil, nil, nil)
}

func ctxWithClaims(perms ...string) context.Context {
	c := &auth.Claims{
		Sub:         "user-001",
		TenantID:    "TUGURE",
		Permissions: perms,
	}
	return auth.ContextWithClaims(context.Background(), c)
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestService_List_NotFound(t *testing.T) {
	svc := newSvc()
	_, err := svc.List(ctxWithClaims("report.*.read"), "rpt-99", reports.QueryParams{Limit: 10})
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeReportNotFound, de.Code())
}

func TestService_List_PermissionDenied(t *testing.T) {
	// Register a tmp stub
	slug := "rpt-test-perm"
	reports.Register(&stubReport{slug: slug})
	defer delete(reports.Registry, slug)

	svc := newSvc()
	// No permission
	_, err := svc.List(ctxWithClaims(), slug, reports.QueryParams{Limit: 10})
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeReportPermissionDenied, de.Code())
}

func TestService_List_WildcardAuditBypass(t *testing.T) {
	slug := "rpt-test-wildcard"
	reports.Register(&stubReport{slug: slug, total: 2, rows: []map[string]any{{"id": "1"}, {"id": "2"}}})
	defer delete(reports.Registry, slug)

	db := newMockDB(t)
	svc := newSvcWithDB(db)
	// audit_log.read wildcard bypass
	result, err := svc.List(ctxWithClaims("audit_log.read"), slug, reports.QueryParams{Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 2, len(result.Rows))
}

func TestService_List_ParamsInvalid(t *testing.T) {
	slug := "rpt-test-invalid"
	reports.Register(&stubReport{slug: slug})
	defer delete(reports.Registry, slug)

	svc := newSvc()
	params := reports.QueryParams{
		Limit: 10,
		Sort:  []reports.SortSpec{{Col: "not_allowed_col", Dir: "asc"}},
	}
	_, err := svc.List(ctxWithClaims("report."+slug+".read"), slug, params)
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeReportParamsInvalid, de.Code())
}

func TestService_Export_NotFound(t *testing.T) {
	svc := newSvc()
	_, err := svc.Export(ctxWithClaims("report.*.export"), "rpt-99", reports.QueryParams{Limit: 10}, "csv")
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeReportNotFound, de.Code())
}

func TestService_Export_InvalidFormat(t *testing.T) {
	slug := "rpt-test-fmt"
	reports.Register(&stubReport{slug: slug, total: 5})
	defer delete(reports.Registry, slug)

	svc := newSvc()
	_, err := svc.Export(ctxWithClaims("report."+slug+".export"), slug, reports.QueryParams{Limit: 10}, "docx")
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeExportFormatUnsupported, de.Code())
}

func TestService_Export_PermissionDenied(t *testing.T) {
	slug := "rpt-test-eperm"
	reports.Register(&stubReport{slug: slug})
	defer delete(reports.Registry, slug)

	svc := newSvc()
	_, err := svc.Export(ctxWithClaims(), slug, reports.QueryParams{Limit: 10}, "csv")
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeReportPermissionDenied, de.Code())
}

func TestService_Export_TooLarge(t *testing.T) {
	slug := "rpt-test-large"
	// total exceeds maxRows (100_000)
	reports.Register(&stubReport{slug: slug, total: 200_000})
	defer delete(reports.Registry, slug)

	svc := newSvc()
	_, err := svc.Export(ctxWithClaims("report."+slug+".export"), slug, reports.QueryParams{Limit: 10}, "csv")
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeExportTooLarge, de.Code())
}

func TestService_ExportRegulatorPack_PermissionDenied(t *testing.T) {
	svc := newSvc()
	claims := &auth.Claims{
		Sub:         "user-001",
		Permissions: []string{"report.rpt-01.read"},
	}
	ctx := auth.ContextWithClaims(context.Background(), claims)
	now := int64(9999999999) // future → step-up fresh
	claims.StepupVerifiedAt = &now

	_, err := svc.ExportRegulatorPack(ctx, reports.RegulatorPackRequest{PeriodeID: "PRD-001", Format: "xlsx"}, claims)
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeReportPermissionDenied, de.Code())
}

func TestService_ExportRegulatorPack_StepUpRequired(t *testing.T) {
	svc := newSvc()
	claims := &auth.Claims{
		Sub:         "user-cfo",
		Permissions: []string{"report.rpt-28.export"},
		// StepupVerifiedAt nil → NeedsStepUp() = true
	}
	ctx := auth.ContextWithClaims(context.Background(), claims)

	_, err := svc.ExportRegulatorPack(ctx, reports.RegulatorPackRequest{PeriodeID: "PRD-001", Format: "xlsx"}, claims)
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeStepUpRequired, de.Code())
}

func TestService_ExportRegulatorPack_InvalidFormat(t *testing.T) {
	svc := newSvc()
	now := int64(9999999999)
	claims := &auth.Claims{
		Sub:              "user-cfo",
		Permissions:      []string{"report.rpt-28.export"},
		StepupVerifiedAt: &now,
	}
	ctx := auth.ContextWithClaims(context.Background(), claims)

	_, err := svc.ExportRegulatorPack(ctx, reports.RegulatorPackRequest{PeriodeID: "PRD-001", Format: "pdf"}, claims)
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeReportParamsInvalid, de.Code())
}

func TestService_ExportRegulatorPack_MissingPeriode(t *testing.T) {
	svc := newSvc()
	now := int64(9999999999)
	claims := &auth.Claims{
		Sub:              "user-cfo",
		Permissions:      []string{"report.rpt-28.export"},
		StepupVerifiedAt: &now,
	}
	ctx := auth.ContextWithClaims(context.Background(), claims)

	_, err := svc.ExportRegulatorPack(ctx, reports.RegulatorPackRequest{Format: "xlsx"}, claims)
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeReportParamsInvalid, de.Code())
}

func TestService_List_Pagination(t *testing.T) {
	slug := "rpt-test-page"
	var rows []map[string]any
	for i := 0; i < 60; i++ {
		rows = append(rows, map[string]any{"id": i})
	}
	reports.Register(&stubReport{slug: slug, total: 60, rows: rows})
	defer delete(reports.Registry, slug)

	db := newMockDB(t)
	svc := newSvcWithDB(db)
	result, err := svc.List(ctxWithClaims("audit_log.read"), slug, reports.QueryParams{Limit: 50})
	require.NoError(t, err)
	// 50 rows returned, nextCursor set because 60 > 50
	assert.Equal(t, 50, len(result.Rows))
	assert.True(t, result.Pagination.HasMore)
	assert.NotNil(t, result.Pagination.NextCursor)
}
