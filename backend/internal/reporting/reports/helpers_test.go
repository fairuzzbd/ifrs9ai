package reports_test

import (
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
	_ "blips-ifrs9.tugu-re.com/internal/reporting/reports/impl"
)

// ─── BuildWhere ───────────────────────────────────────────────────────────────

func TestBuildWhere_Empty(t *testing.T) {
	clause, args, nextIdx, err := reports.BuildWhere(nil, []string{"id"}, 1)
	require.NoError(t, err)
	assert.Empty(t, clause)
	assert.Empty(t, args)
	assert.Equal(t, 1, nextIdx)
}

func TestBuildWhere_EqFilter(t *testing.T) {
	filters := []reports.FilterSpec{{Col: "stage", Op: "eq", Value: "2"}}
	clause, args, nextIdx, err := reports.BuildWhere(filters, []string{"stage"}, 1)
	require.NoError(t, err)
	assert.Equal(t, "stage = $1", clause)
	assert.Equal(t, []any{"2"}, args)
	assert.Equal(t, 2, nextIdx)
}

func TestBuildWhere_MultipleFilters(t *testing.T) {
	filters := []reports.FilterSpec{
		{Col: "stage", Op: "eq", Value: "2"},
		{Col: "ead", Op: "gte", Value: "1000000"},
	}
	clause, args, nextIdx, err := reports.BuildWhere(filters, []string{"stage", "ead"}, 1)
	require.NoError(t, err)
	assert.Contains(t, clause, "stage = $1")
	assert.Contains(t, clause, "ead >= $2")
	assert.Len(t, args, 2)
	assert.Equal(t, 3, nextIdx)
}

func TestBuildWhere_AllOps(t *testing.T) {
	allowed := []string{"col"}
	tests := []struct {
		op      string
		wantSub string
	}{
		{"ne", "!= $1"},
		{"lte", "<= $1"},
		{"lt", "< $1"},
		{"gt", "> $1"},
		{"like", "ILIKE $1"},
		{"is_null", "IS NULL"},
		{"is_not_null", "IS NOT NULL"},
	}
	for _, tt := range tests {
		t.Run(tt.op, func(t *testing.T) {
			f := []reports.FilterSpec{{Col: "col", Op: tt.op, Value: "v"}}
			clause, _, _, err := reports.BuildWhere(f, allowed, 1)
			require.NoError(t, err)
			assert.Contains(t, clause, tt.wantSub)
		})
	}
}

func TestBuildWhere_UnknownOp(t *testing.T) {
	filters := []reports.FilterSpec{{Col: "stage", Op: "regex", Value: ".*"}}
	_, _, _, err := reports.BuildWhere(filters, []string{"stage"}, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown filter op")
}

func TestBuildWhere_DisallowedCol(t *testing.T) {
	filters := []reports.FilterSpec{{Col: "password", Op: "eq", Value: "secret"}}
	_, _, _, err := reports.BuildWhere(filters, []string{"stage"}, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

// ─── BuildOrderBy ─────────────────────────────────────────────────────────────

func TestBuildOrderBy_Empty_UsesDefault(t *testing.T) {
	defaultSort := []reports.SortSpec{{Col: "created_at", Dir: "desc"}}
	ob := reports.BuildOrderBy(nil, []string{"created_at"}, defaultSort)
	assert.Equal(t, "ORDER BY created_at DESC", ob)
}

func TestBuildOrderBy_ExplicitSort(t *testing.T) {
	sort := []reports.SortSpec{{Col: "stage", Dir: "asc"}, {Col: "id", Dir: "desc"}}
	ob := reports.BuildOrderBy(sort, []string{"stage", "id"}, nil)
	assert.Equal(t, "ORDER BY stage ASC, id DESC", ob)
}

func TestBuildOrderBy_DisallowedColSkipped(t *testing.T) {
	sort := []reports.SortSpec{{Col: "secret_col", Dir: "asc"}, {Col: "id", Dir: "asc"}}
	ob := reports.BuildOrderBy(sort, []string{"id"}, nil)
	assert.Equal(t, "ORDER BY id ASC", ob)
}

func TestBuildOrderBy_AllDisallowed_ReturnsEmpty(t *testing.T) {
	sort := []reports.SortSpec{{Col: "secret_col", Dir: "asc"}}
	ob := reports.BuildOrderBy(sort, []string{"id"}, nil)
	assert.Empty(t, ob)
}

// ─── ScanRowsToMaps ───────────────────────────────────────────────────────────

func TestScanRowsToMaps_Basic(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	rows := sqlmock.NewRows([]string{"id", "name"}).
		AddRow("uuid-1", "Instrumen A").
		AddRow("uuid-2", "Instrumen B")
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	sqlRows, err := db.Query("SELECT id, name FROM mst.instrumen")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlRows.Close() })

	seq, err := reports.ScanRowsToMaps(sqlRows)
	require.NoError(t, err)

	var collected []map[string]any
	for row := range seq {
		collected = append(collected, row)
	}
	assert.Len(t, collected, 2)
	assert.Equal(t, "uuid-1", collected[0]["id"])
	assert.Equal(t, "Instrumen A", collected[0]["name"])
	assert.Equal(t, "uuid-2", collected[1]["id"])
}

// ─── mapQueryError (via service timeout path) ─────────────────────────────────

func TestService_MapQueryError_Timeout(t *testing.T) {
	// Register a stub that returns a "statement timeout" error.
	slug := "rpt-test-timeout"
	stub := &stubReport{
		slug:     slug,
		queryErr: &timeoutError{msg: "ERROR: canceling statement due to statement timeout (SQLSTATE 57014)"},
	}
	reports.Register(stub)
	defer delete(reports.Registry, slug)

	db := newMockDB(t)
	svc := newSvcWithDB(db)
	_, err := svc.List(ctxWithClaims("audit_log.read"), slug, reports.QueryParams{Limit: 10})
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeReportQueryTimeout, de.Code())
}

type timeoutError struct{ msg string }

func (e *timeoutError) Error() string { return e.msg }

// ─── ValidateSortFilter ───────────────────────────────────────────────────────

func TestValidateSortFilter_AllowedSort(t *testing.T) {
	stub := &stubReport{slug: "rpt-vsf"}
	params := reports.QueryParams{
		Sort: []reports.SortSpec{{Col: "id", Dir: "asc"}},
	}
	err := reports.ValidateSortFilter(stub, params)
	require.NoError(t, err)
}

func TestValidateSortFilter_DisallowedSort(t *testing.T) {
	stub := &stubReport{slug: "rpt-vsf2"}
	params := reports.QueryParams{
		Sort: []reports.SortSpec{{Col: "secret", Dir: "asc"}},
	}
	err := reports.ValidateSortFilter(stub, params)
	require.Error(t, err)
}

func TestValidateSortFilter_AllowedFilter(t *testing.T) {
	stub := &stubReport{slug: "rpt-vsf3"}
	params := reports.QueryParams{
		Filters: []reports.FilterSpec{{Col: "id", Op: "eq", Value: "x"}},
	}
	err := reports.ValidateSortFilter(stub, params)
	require.NoError(t, err)
}

func TestValidateSortFilter_DisallowedFilter(t *testing.T) {
	stub := &stubReport{slug: "rpt-vsf4"}
	params := reports.QueryParams{
		Filters: []reports.FilterSpec{{Col: "password", Op: "eq", Value: "x"}},
	}
	err := reports.ValidateSortFilter(stub, params)
	require.Error(t, err)
}
