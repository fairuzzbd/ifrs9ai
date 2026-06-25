// Package reports implements P5-M14 APP-E 25 report endpoints.
// Single Report interface + 25 implementations wired through generic service.
// References: P5-M14-S1..S5, UX §1, DEC-018 (audit in-tx), DEC-022 (cursor).
package reports

import (
	"context"
	"database/sql"
	"fmt"
	"iter"
	"strings"
)

// ─── Types ────────────────────────────────────────────────────────────────────

// SortSpec holds one sort column direction pair.
type SortSpec struct {
	Col string
	Dir string // "asc" or "desc"
}

// FilterSpec holds one filter predicate.
type FilterSpec struct {
	Col   string
	Op    string // "eq", "gte", "lte", "between", "like", "is_null", "in"
	Value string
}

// ColumnSpec describes one export column.
type ColumnSpec struct {
	Key    string // DB column name
	Header string // Human-readable (Bahasa Indonesia)
	Format string // "idr", "datetime", "date", "pct", "text"
}

// QueryParams bundles all list/export query parameters.
type QueryParams struct {
	Cursor          string
	Limit           int
	Sort            []SortSpec
	Filters         []FilterSpec
	Search          string
	// Report-specific extra params (e.g. calc_run_id, w_good, etc.)
	Extra           map[string]string
}

// Pagination is the cursor-based pagination result.
type Pagination struct {
	NextCursor    *string
	HasMore       bool
	TotalEstimate int64
	Limit         int
}

// ─── Report interface ─────────────────────────────────────────────────────────

// Report is implemented by each of the 25 report structs.
type Report interface {
	// Slug returns the URL-safe report identifier (e.g. "rpt-01").
	Slug() string

	// Permission returns the required permission string for list/read.
	// e.g. "report.rpt-01.read"
	Permission() string

	// ExportPermission returns the required permission for export.
	// e.g. "report.rpt-01.export"
	ExportPermission() string

	// RegulatedFlag returns true if an audit REPORT.{SLUG}_VIEWED must be written
	// in-transaction on every list call (compliance evidence).
	RegulatedFlag() bool

	// DefaultSort returns the default sort for this report.
	DefaultSort() []SortSpec

	// AllowedSort returns the whitelist of sortable column names.
	AllowedSort() []string

	// AllowedFilter returns the whitelist of filterable columns + their types.
	AllowedFilter() []FilterSpec

	// Columns returns the column specifications for CSV/XLSX export headers.
	Columns() []ColumnSpec

	// Query executes the report query and returns an iterator of rows.
	// Each row is a map[string]any keyed by column Key from Columns().
	// totalEstimate is the approximate count (used for async/inline decision).
	Query(ctx context.Context, db *sql.DB, params QueryParams) (iter.Seq[map[string]any], int64, error)
}

// ─── Registry ─────────────────────────────────────────────────────────────────

// Registry maps slug → Report implementation.
// All 25 reports are registered by init() in their respective files.
var Registry = map[string]Report{}

// Register adds a Report to the Registry. Panics on duplicate slug.
func Register(r Report) {
	if _, exists := Registry[r.Slug()]; exists {
		panic(fmt.Sprintf("reports: duplicate slug %q", r.Slug()))
	}
	Registry[r.Slug()] = r
}

// ─── Shared SQL helpers ────────────────────────────────────────────────────────

// BuildWhere converts FilterSpec list to a parameterized WHERE clause fragment.
// Returns clause (without "WHERE"), args, and the next param index.
// #nosec G202 — col names validated against allowedCols before calling this.
func BuildWhere(filters []FilterSpec, allowed []string, startIdx int) (string, []any, int, error) {
	allowedSet := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		allowedSet[a] = true
	}

	var parts []string
	var args []any
	idx := startIdx

	for _, f := range filters {
		if !allowedSet[f.Col] {
			return "", nil, idx, fmt.Errorf("filter column %q not allowed", f.Col)
		}
		switch f.Op {
		case "eq", "":
			parts = append(parts, fmt.Sprintf("%s = $%d", f.Col, idx))
			args = append(args, f.Value)
			idx++
		case "ne":
			parts = append(parts, fmt.Sprintf("%s != $%d", f.Col, idx))
			args = append(args, f.Value)
			idx++
		case "gte":
			parts = append(parts, fmt.Sprintf("%s >= $%d", f.Col, idx))
			args = append(args, f.Value)
			idx++
		case "lte":
			parts = append(parts, fmt.Sprintf("%s <= $%d", f.Col, idx))
			args = append(args, f.Value)
			idx++
		case "gt":
			parts = append(parts, fmt.Sprintf("%s > $%d", f.Col, idx))
			args = append(args, f.Value)
			idx++
		case "lt":
			parts = append(parts, fmt.Sprintf("%s < $%d", f.Col, idx))
			args = append(args, f.Value)
			idx++
		case "like":
			parts = append(parts, fmt.Sprintf("%s ILIKE $%d", f.Col, idx))
			args = append(args, f.Value)
			idx++
		case "is_null":
			parts = append(parts, f.Col+" IS NULL")
		case "is_not_null":
			parts = append(parts, f.Col+" IS NOT NULL")
		default:
			return "", nil, idx, fmt.Errorf("unknown filter op %q", f.Op)
		}
	}

	return strings.Join(parts, " AND "), args, idx, nil
}

// BuildOrderBy converts SortSpec list to an ORDER BY clause.
// #nosec G202 — col names validated against allowedSort before calling this.
func BuildOrderBy(sort []SortSpec, allowed []string, defaultSort []SortSpec) string {
	allowedSet := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		allowedSet[a] = true
	}

	effective := sort
	if len(effective) == 0 {
		effective = defaultSort
	}

	var parts []string
	for _, s := range effective {
		if !allowedSet[s.Col] {
			continue
		}
		dir := "ASC"
		if strings.ToLower(s.Dir) == "desc" {
			dir = "DESC"
		}
		parts = append(parts, s.Col+" "+dir)
	}
	if len(parts) == 0 {
		return ""
	}
	return "ORDER BY " + strings.Join(parts, ", ")
}

// ScanRowsToMaps iterates sql.Rows and yields map[string]any rows.
// The caller must call rows.Close(). This function does not close rows.
func ScanRowsToMaps(rows *sql.Rows) (iter.Seq[map[string]any], error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("ScanRowsToMaps: columns: %w", err)
	}

	seq := func(yield func(map[string]any) bool) {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		for rows.Next() {
			if err = rows.Scan(ptrs...); err != nil {
				return
			}
			row := make(map[string]any, len(cols))
			for i, c := range cols {
				row[c] = vals[i]
			}
			if !yield(row) {
				return
			}
		}
	}
	return seq, nil
}

// ValidateSortFilter checks that all requested sort/filter columns are in the report's whitelist.
func ValidateSortFilter(r Report, params QueryParams) error {
	allowed := r.AllowedSort()
	allowedSet := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		allowedSet[a] = true
	}
	for _, s := range params.Sort {
		if !allowedSet[s.Col] {
			return fmt.Errorf("sort column %q tidak diizinkan untuk laporan %s", s.Col, r.Slug())
		}
	}

	allowedF := r.AllowedFilter()
	allowedFSet := make(map[string]bool, len(allowedF))
	for _, a := range allowedF {
		allowedFSet[a.Col] = true
	}
	for _, f := range params.Filters {
		if !allowedFSet[f.Col] {
			return fmt.Errorf("filter column %q tidak diizinkan untuk laporan %s", f.Col, r.Slug())
		}
	}
	return nil
}
