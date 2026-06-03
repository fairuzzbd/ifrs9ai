package listquery_test

import (
	"net/http"
	"strings"
	"testing"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

func TestParseFromRequest_Sort(t *testing.T) {
	req, _ := http.NewRequest("GET", "/test?sort=created_at:desc,kode:asc", nil)
	q, err := listquery.ParseFromRequest(req, []string{"created_at", "kode", "stage"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.Sort) != 2 {
		t.Fatalf("expected 2 sort specs, got %d", len(q.Sort))
	}
	if q.Sort[0].Col != "created_at" || q.Sort[0].Dir != "desc" {
		t.Errorf("sort[0] = %+v", q.Sort[0])
	}
	if q.Sort[1].Col != "kode" || q.Sort[1].Dir != "asc" {
		t.Errorf("sort[1] = %+v", q.Sort[1])
	}
}

func TestParseFromRequest_SortDisallowedColumn(t *testing.T) {
	req, _ := http.NewRequest("GET", "/test?sort=password:desc", nil)
	_, err := listquery.ParseFromRequest(req, []string{"created_at", "kode"})
	if err == nil {
		t.Fatal("expected error for disallowed sort column, got nil")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError, got %T: %v", err, err)
	}
	if de.Code() != domainerrors.CodeInvalidSortCol {
		t.Errorf("expected INVALID_SORT_COL, got %s", de.Code())
	}
}

func TestParseFromRequest_MaxSortCols(t *testing.T) {
	// More than 3 sort cols — should be truncated to 3.
	req, _ := http.NewRequest("GET", "/test?sort=a:asc,b:desc,c:asc,d:asc", nil)
	q, err := listquery.ParseFromRequest(req, []string{"a", "b", "c", "d"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.Sort) > 3 {
		t.Errorf("expected max 3 sort specs, got %d", len(q.Sort))
	}
}

func TestParseFromRequest_Filter(t *testing.T) {
	req, _ := http.NewRequest("GET", "/test?filter[stage]=2&filter[bank]=BCA", nil)
	q, err := listquery.ParseFromRequest(req, []string{"stage", "bank", "created_at"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.Filters) != 2 {
		t.Fatalf("expected 2 filters, got %d", len(q.Filters))
	}
}

func TestParseFromRequest_FilterDisallowedColumn(t *testing.T) {
	req, _ := http.NewRequest("GET", "/test?filter[secret_key]=value", nil)
	_, err := listquery.ParseFromRequest(req, []string{"stage", "bank"})
	if err == nil {
		t.Fatal("expected error for disallowed filter column, got nil")
	}
}

func TestParseFromRequest_Search(t *testing.T) {
	req, _ := http.NewRequest("GET", "/test?q=deposito", nil)
	q, err := listquery.ParseFromRequest(req, []string{"stage"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Search != "deposito" {
		t.Errorf("search mismatch: %s", q.Search)
	}
}

func TestParseFromRequest_SearchMaxLength(t *testing.T) {
	longSearch := strings.Repeat("a", 300)
	req, _ := http.NewRequest("GET", "/test?q="+longSearch, nil)
	q, _ := listquery.ParseFromRequest(req, []string{"stage"})
	if len(q.Search) > 200 {
		t.Errorf("search should be truncated to 200 chars, got %d", len(q.Search))
	}
}

func TestToSQL_Filter(t *testing.T) {
	q := listquery.Query{}
	q = q.WithAllowed([]string{"stage", "bank", "created_at"})
	q.Filters = []listquery.FilterSpec{
		{Col: "stage", Op: listquery.FilterOpEq, Value: "2"},
	}

	where, args, _ := q.ToSQL("t")
	if !strings.Contains(where, "t.stage") {
		t.Errorf("expected t.stage in WHERE clause, got: %s", where)
	}
	if len(args) != 1 {
		t.Errorf("expected 1 arg, got %d", len(args))
	}
	if args[0] != "2" {
		t.Errorf("expected arg '2', got %v", args[0])
	}
}

func TestToSQL_Sort(t *testing.T) {
	q := listquery.Query{}
	q = q.WithAllowed([]string{"created_at", "kode"})
	q.Sort = []listquery.SortSpec{
		{Col: "created_at", Dir: "desc"},
	}

	_, _, orderBy := q.ToSQL("t")
	if !strings.Contains(orderBy, "t.created_at DESC") {
		t.Errorf("expected t.created_at DESC in ORDER BY, got: %s", orderBy)
	}
}

func TestToSQL_NoSQLInjection(t *testing.T) {
	// Column name injection — should be blocked by whitelist.
	q := listquery.Query{}
	q = q.WithAllowed([]string{"stage"})
	q.Filters = []listquery.FilterSpec{
		{Col: "stage", Op: listquery.FilterOpEq, Value: "'; DROP TABLE mst.instrumen; --"},
	}

	where, args, _ := q.ToSQL("t")
	// Column is safe (whitelisted), value is parameterized.
	if strings.Contains(where, "DROP TABLE") {
		t.Error("SQL injection in WHERE clause — value should be parameterized")
	}
	if len(args) == 1 && args[0] == "'; DROP TABLE mst.instrumen; --" {
		// Correct: value is passed as parameter, not inlined in SQL.
	} else if !strings.Contains(where, "$1") {
		t.Error("value should be parameterized with $1")
	}
}

func TestToSQL_MultipleFilters(t *testing.T) {
	q := listquery.Query{}
	q = q.WithAllowed([]string{"stage", "bank", "status"})
	q.Filters = []listquery.FilterSpec{
		{Col: "stage", Op: listquery.FilterOpGte, Value: "1"},
		{Col: "bank", Op: listquery.FilterOpLike, Value: "%BCA%"},
		{Col: "status", Op: listquery.FilterOpNe, Value: "DELETED"},
	}

	where, args, _ := q.ToSQL("")
	if !strings.Contains(where, "AND") {
		t.Error("multiple filters should be joined with AND")
	}
	if len(args) != 3 {
		t.Errorf("expected 3 args, got %d", len(args))
	}
}

func TestToSQL_InOperator(t *testing.T) {
	q := listquery.Query{}
	q = q.WithAllowed([]string{"bank"})
	q.Filters = []listquery.FilterSpec{
		{Col: "bank", Op: listquery.FilterOpIn, Value: []string{"BCA", "BNI", "MANDIRI"}},
	}

	where, args, _ := q.ToSQL("t")
	if !strings.Contains(where, "t.bank") {
		t.Error("expected bank column in WHERE")
	}
	if len(args) != 3 {
		t.Errorf("expected 3 args for IN, got %d", len(args))
	}
}

func TestToSQL_IsNull(t *testing.T) {
	q := listquery.Query{}
	q = q.WithAllowed([]string{"deleted_at"})
	q.Filters = []listquery.FilterSpec{
		{Col: "deleted_at", Op: listquery.FilterOpIsNull},
	}

	where, args, _ := q.ToSQL("t")
	if !strings.Contains(where, "IS NULL") {
		t.Errorf("expected IS NULL, got: %s", where)
	}
	if len(args) != 0 {
		t.Errorf("expected 0 args for IS NULL, got %d", len(args))
	}
}

func TestAppliedFilter_Empty(t *testing.T) {
	q := listquery.Query{}
	if f := q.AppliedFilter(); f != nil {
		t.Error("expected nil applied filter for empty query")
	}
}

func TestAppliedSort_Empty(t *testing.T) {
	q := listquery.Query{}
	if s := q.AppliedSort(); len(s) != 0 {
		t.Error("expected empty applied sort")
	}
}
