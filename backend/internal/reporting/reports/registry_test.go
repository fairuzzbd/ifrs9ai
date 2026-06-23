package reports_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
	_ "blips-ifrs9.tugu-re.com/internal/reporting/reports/impl" // trigger all init()
)

// expectedSlugs lists all 25 slugs that must be registered in M14.
var expectedSlugs = []string{
	"rpt-01", "rpt-02", "rpt-03", "rpt-04", "rpt-05",
	"rpt-06", "rpt-07", "rpt-08", "rpt-09", "rpt-10",
	"rpt-11", "rpt-12",
	"rpt-13", "rpt-14", "rpt-15", "rpt-16", "rpt-17", "rpt-18",
	"rpt-22", "rpt-22b",
	"rpt-23", "rpt-25", "rpt-26", "rpt-27", "rpt-28",
}

func TestRegistry_Contains25Reports(t *testing.T) {
	assert.Equal(t, len(expectedSlugs), len(reports.Registry),
		"Registry must contain exactly 25 reports for P5-M14")
}

func TestRegistry_AllExpectedSlugsPresent(t *testing.T) {
	for _, slug := range expectedSlugs {
		t.Run(slug, func(t *testing.T) {
			r, ok := reports.Registry[slug]
			require.True(t, ok, "Registry missing slug %q", slug)
			assert.Equal(t, slug, r.Slug())
		})
	}
}

func TestRegistry_AllReports_PermissionNonEmpty(t *testing.T) {
	for slug, r := range reports.Registry {
		t.Run(slug, func(t *testing.T) {
			assert.NotEmpty(t, r.Permission(), "Permission() must not be empty for %q", slug)
			assert.NotEmpty(t, r.ExportPermission(), "ExportPermission() must not be empty for %q", slug)
		})
	}
}

func TestRegistry_AllReports_DefaultSortNonNil(t *testing.T) {
	for slug, r := range reports.Registry {
		t.Run(slug, func(t *testing.T) {
			assert.NotNil(t, r.DefaultSort(), "DefaultSort() must not return nil for %q", slug)
			assert.NotNil(t, r.AllowedSort(), "AllowedSort() must not return nil for %q", slug)
			assert.NotNil(t, r.AllowedFilter(), "AllowedFilter() must not return nil for %q", slug)
			assert.NotNil(t, r.Columns(), "Columns() must not return nil for %q", slug)
		})
	}
}

func TestRegistry_RegulatedReports(t *testing.T) {
	regulated := []string{"rpt-18", "rpt-27", "rpt-28"}
	for _, slug := range regulated {
		t.Run(slug, func(t *testing.T) {
			r, ok := reports.Registry[slug]
			require.True(t, ok)
			assert.True(t, r.RegulatedFlag(), "slug %q must have RegulatedFlag=true", slug)
		})
	}
}

func TestRegistry_NonRegulatedReports(t *testing.T) {
	nonRegulated := []string{"rpt-01", "rpt-06", "rpt-13", "rpt-22"}
	for _, slug := range nonRegulated {
		t.Run(slug, func(t *testing.T) {
			r, ok := reports.Registry[slug]
			require.True(t, ok)
			assert.False(t, r.RegulatedFlag(), "slug %q must have RegulatedFlag=false", slug)
		})
	}
}

func TestRegistry_PermissionPattern(t *testing.T) {
	for slug, r := range reports.Registry {
		t.Run(slug, func(t *testing.T) {
			expected := "report." + slug + ".read"
			assert.Equal(t, expected, r.Permission())
			expectedExport := "report." + slug + ".export"
			assert.Equal(t, expectedExport, r.ExportPermission())
		})
	}
}

func TestRegistry_AllReports_ColumnsHaveKeyAndHeader(t *testing.T) {
	// rpt-28 has empty columns by design (always-async worker path).
	for slug, r := range reports.Registry {
		if slug == "rpt-28" {
			continue
		}
		t.Run(slug, func(t *testing.T) {
			cols := r.Columns()
			require.NotEmpty(t, cols, "slug %q must have at least 1 column", slug)
			for _, c := range cols {
				assert.NotEmpty(t, c.Key, "column Key must not be empty for %q", slug)
				assert.NotEmpty(t, c.Header, "column Header must not be empty for %q", slug)
			}
		})
	}
}

func TestRegistry_NoDuplicateSlugs(t *testing.T) {
	seen := map[string]bool{}
	for slug := range reports.Registry {
		assert.False(t, seen[slug], "duplicate slug %q in registry", slug)
		seen[slug] = true
	}
}
