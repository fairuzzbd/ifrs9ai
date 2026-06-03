// Package listquery menyediakan parser dan SQL builder untuk list endpoint:
// sort, filter, search (q) — sesuai ux-patterns.md §1.
//
// WAJIB: kolom yang di-sort/filter HARUS di whitelist allowedCols.
// SQL selalu pakai parameterized query — TIDAK ada string concat.
package listquery

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// SortSpec adalah satu kolom sort.
type SortSpec struct {
	Col string
	Dir string // "asc" atau "desc"
}

// FilterOp adalah operator filter yang didukung.
type FilterOp string

const (
	FilterOpEq       FilterOp = "eq"
	FilterOpNe       FilterOp = "ne"
	FilterOpGt       FilterOp = "gt"
	FilterOpGte      FilterOp = "gte"
	FilterOpLt       FilterOp = "lt"
	FilterOpLte      FilterOp = "lte"
	FilterOpLike     FilterOp = "like"
	FilterOpIsNull   FilterOp = "is_null"
	FilterOpNotNull  FilterOp = "is_not_null"
	FilterOpIn       FilterOp = "in"
)

// FilterSpec adalah satu filter kondisi.
type FilterSpec struct {
	Col   string
	Op    FilterOp
	Value any // string, []string, atau nil untuk is_null/is_not_null
}

// Query adalah parameter list yang sudah ter-parse dan ter-validasi.
type Query struct {
	Sort    []SortSpec
	Search  string
	Filters []FilterSpec
	// AllowedCols adalah whitelist kolom yang boleh di-sort/filter.
	// Di-set via WithAllowed().
	allowedCols map[string]struct{}
}

// WithAllowed mengembalikan Query copy dengan whitelist kolom.
func (q Query) WithAllowed(cols []string) Query {
	q.allowedCols = make(map[string]struct{}, len(cols))
	for _, c := range cols {
		q.allowedCols[c] = struct{}{}
	}
	return q
}

// sortDirRe mem-validasi nilai dir.
var sortDirRe = regexp.MustCompile(`^(asc|desc)$`)

// colNameRe mem-validasi nama kolom (hanya a-z, 0-9, _).
var colNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// ParseFromRequest mem-parse query params dari http.Request.
// Mengembalikan error DomainError jika ada kolom invalid.
func ParseFromRequest(r *http.Request, allowedCols []string) (Query, error) {
	q := Query{}
	q.allowedCols = make(map[string]struct{}, len(allowedCols))
	for _, c := range allowedCols {
		q.allowedCols[c] = struct{}{}
	}

	params := r.URL.Query()

	// Parse sort: ?sort=col1:asc,col2:desc (max 3)
	if sortStr := params.Get("sort"); sortStr != "" {
		parts := strings.SplitN(sortStr, ",", 4)
		if len(parts) > 3 {
			parts = parts[:3]
		}
		for _, part := range parts {
			segments := strings.SplitN(part, ":", 2)
			col := strings.TrimSpace(segments[0])
			dir := "asc"
			if len(segments) == 2 {
				dir = strings.ToLower(strings.TrimSpace(segments[1]))
			}
			if !colNameRe.MatchString(col) || !sortDirRe.MatchString(dir) {
				continue // skip malformed entries
			}
			if _, ok := q.allowedCols[col]; !ok {
				return Query{}, domainerrors.New(domainerrors.CodeInvalidSortCol,
					fmt.Sprintf("Kolom sort '%s' tidak diizinkan.", col))
			}
			q.Sort = append(q.Sort, SortSpec{Col: col, Dir: dir})
		}
	}

	// Parse search: ?q=...
	q.Search = strings.TrimSpace(params.Get("q"))
	if len(q.Search) > 200 {
		q.Search = q.Search[:200]
	}

	// Parse filter: ?filter[col]=op:val atau ?filter[col]=val
	for key, vals := range params {
		if !strings.HasPrefix(key, "filter[") || !strings.HasSuffix(key, "]") {
			continue
		}
		col := key[len("filter[") : len(key)-1]
		if !colNameRe.MatchString(col) {
			continue
		}
		if _, ok := q.allowedCols[col]; !ok {
			return Query{}, domainerrors.New(domainerrors.CodeInvalidSortCol,
				fmt.Sprintf("Kolom filter '%s' tidak diizinkan.", col))
		}

		for _, rawVal := range vals {
			spec := parseFilterValue(col, rawVal)
			q.Filters = append(q.Filters, spec)
		}
	}

	return q, nil
}

// parseFilterValue mem-parse satu nilai filter menjadi FilterSpec.
func parseFilterValue(col, rawVal string) FilterSpec {
	// Multi-value: "in:val1,val2"
	if strings.HasPrefix(rawVal, "in:") {
		vals := strings.Split(rawVal[3:], ",")
		return FilterSpec{Col: col, Op: FilterOpIn, Value: vals}
	}

	// Operator prefix: "gte:1000"
	operators := []string{"gte:", "lte:", "gt:", "lt:", "ne:", "like:", "eq:", "is_null", "is_not_null"}
	for _, opPrefix := range operators {
		if rawVal == "is_null" {
			return FilterSpec{Col: col, Op: FilterOpIsNull}
		}
		if rawVal == "is_not_null" {
			return FilterSpec{Col: col, Op: FilterOpNotNull}
		}
		if strings.HasPrefix(rawVal, opPrefix) {
			op := FilterOp(strings.TrimSuffix(opPrefix, ":"))
			val := rawVal[len(opPrefix):]
			return FilterSpec{Col: col, Op: op, Value: val}
		}
	}

	// Default: eq
	return FilterSpec{Col: col, Op: FilterOpEq, Value: rawVal}
}

// ToSQL membangun WHERE clause + ORDER BY dari Query.
// tableAlias adalah alias tabel (mis. "t") untuk prefix kolom.
// Returns: whereClause (tanpa "WHERE"), args (positional $1,$2,...), orderBy.
//
// SECURITY: Kolom WAJIB sudah divalidasi di whitelist. Value selalu pakai parameter binding.
func (q Query) ToSQL(tableAlias string) (whereClause string, args []any, orderBy string) {
	prefix := ""
	if tableAlias != "" {
		prefix = tableAlias + "."
	}

	var conditions []string
	argIdx := 1

	for _, f := range q.Filters {
		col := prefix + f.Col // safe karena col sudah divalidasi via colNameRe + whitelist

		switch f.Op {
		case FilterOpEq:
			conditions = append(conditions, fmt.Sprintf("%s = $%d", col, argIdx))
			args = append(args, f.Value)
			argIdx++
		case FilterOpNe:
			conditions = append(conditions, fmt.Sprintf("%s != $%d", col, argIdx))
			args = append(args, f.Value)
			argIdx++
		case FilterOpGt:
			conditions = append(conditions, fmt.Sprintf("%s > $%d", col, argIdx))
			args = append(args, f.Value)
			argIdx++
		case FilterOpGte:
			conditions = append(conditions, fmt.Sprintf("%s >= $%d", col, argIdx))
			args = append(args, f.Value)
			argIdx++
		case FilterOpLt:
			conditions = append(conditions, fmt.Sprintf("%s < $%d", col, argIdx))
			args = append(args, f.Value)
			argIdx++
		case FilterOpLte:
			conditions = append(conditions, fmt.Sprintf("%s <= $%d", col, argIdx))
			args = append(args, f.Value)
			argIdx++
		case FilterOpLike:
			conditions = append(conditions, fmt.Sprintf("%s ILIKE $%d", col, argIdx))
			args = append(args, f.Value)
			argIdx++
		case FilterOpIsNull:
			conditions = append(conditions, fmt.Sprintf("%s IS NULL", col))
		case FilterOpNotNull:
			conditions = append(conditions, fmt.Sprintf("%s IS NOT NULL", col))
		case FilterOpIn:
			if vals, ok := f.Value.([]string); ok && len(vals) > 0 {
				placeholders := make([]string, len(vals))
				for i, v := range vals {
					placeholders[i] = fmt.Sprintf("$%d", argIdx)
					args = append(args, v)
					argIdx++
				}
				conditions = append(conditions, fmt.Sprintf("%s = ANY(ARRAY[%s])",
					col, strings.Join(placeholders, ",")))
			}
		}
	}

	whereClause = strings.Join(conditions, " AND ")

	// ORDER BY — kolom sudah divalidasi di Parse.
	var orderParts []string
	for _, s := range q.Sort {
		dir := "ASC"
		if s.Dir == "desc" {
			dir = "DESC"
		}
		orderParts = append(orderParts, fmt.Sprintf("%s%s %s", prefix, s.Col, dir))
	}
	orderBy = strings.Join(orderParts, ", ")

	return whereClause, args, orderBy
}

// AppliedFilter mengembalikan map representasi filter aktif (untuk echo back di response).
func (q Query) AppliedFilter() map[string]any {
	if len(q.Filters) == 0 {
		return nil
	}
	m := make(map[string]any, len(q.Filters))
	for _, f := range q.Filters {
		m[f.Col] = f.Value
	}
	return m
}

// AppliedSort mengembalikan slice SortApplied untuk echo back di response.
func (q Query) AppliedSort() []map[string]string {
	out := make([]map[string]string, len(q.Sort))
	for i, s := range q.Sort {
		out[i] = map[string]string{"col": s.Col, "dir": s.Dir}
	}
	return out
}
