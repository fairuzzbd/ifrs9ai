// export_test.go provides test helpers for the reports package (external test package).
package reports_test

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
)

// parseQueryParamsExposed mirrors handler.parseQueryParams for white-box testing.
// Kept in sync with the production implementation in handler.go.
func parseQueryParamsExposed(c *gin.Context) reports.QueryParams {
	cursor := c.Query("cursor")
	limit := 50
	if l := c.Query("limit"); l != "" {
		var n int
		if _, err := fmt.Sscanf(l, "%d", &n); err == nil && n > 0 {
			limit = n
		}
	}

	var sortSpecs []reports.SortSpec
	if sortStr := c.Query("sort"); sortStr != "" {
		for _, part := range strings.Split(sortStr, ",") {
			kv := strings.SplitN(strings.TrimSpace(part), ":", 2)
			spec := reports.SortSpec{Col: kv[0], Dir: "asc"}
			if len(kv) == 2 {
				spec.Dir = kv[1]
			}
			sortSpecs = append(sortSpecs, spec)
		}
	}

	var filters []reports.FilterSpec
	for key, vals := range c.Request.URL.Query() {
		if !strings.HasPrefix(key, "filter[") || !strings.HasSuffix(key, "]") {
			continue
		}
		col := key[7 : len(key)-1]
		for _, val := range vals {
			op := "eq"
			value := val
			if idx := strings.Index(val, ":"); idx > 0 {
				prefix := val[:idx]
				switch prefix {
				case "eq", "ne", "gt", "gte", "lt", "lte", "like", "is_null", "is_not_null", "between", "in":
					op = prefix
					value = val[idx+1:]
				}
			}
			filters = append(filters, reports.FilterSpec{Col: col, Op: op, Value: value})
		}
	}

	return reports.QueryParams{
		Cursor:  cursor,
		Limit:   limit,
		Sort:    sortSpecs,
		Filters: filters,
		Search:  c.Query("q"),
	}
}
