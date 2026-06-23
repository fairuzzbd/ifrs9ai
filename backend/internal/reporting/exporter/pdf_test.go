package exporter_test

import (
	"testing"
	"time"

	"blips-ifrs9.tugu-re.com/internal/reporting/exporter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExportPDF_Returns_NonEmptyBytes(t *testing.T) {
	fb, sha, err := exporter.ExportPDF(exporter.ExportPDFOptions{
		Title:      "Status Periode",
		Headers:    []string{"periode", "ead"},
		Rows:       [][]string{{"2026-06", "1000000"}},
		ExportedAt: testExportedAt,
		Username:   "u",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, fb)
	assert.NotEmpty(t, sha)
}

func TestExportPDF_SHA256MatchesContent(t *testing.T) {
	fb, sha, err := exporter.ExportPDF(exporter.ExportPDFOptions{
		Headers:    []string{"a"},
		Rows:       [][]string{{"1"}},
		ExportedAt: testExportedAt,
		Username:   "u",
	})
	require.NoError(t, err)
	assert.Equal(t, exporter.SHA256OfBytes(fb), sha)
}

func TestExportPDF_WatermarkPresent(t *testing.T) {
	ts := time.Date(2026, 6, 23, 10, 30, 0, 0, time.UTC)
	fb, _, err := exporter.ExportPDF(exporter.ExportPDFOptions{
		Title:      "Test",
		Headers:    []string{"id"},
		Rows:       [][]string{{"row1"}},
		ExportedAt: ts,
		Username:   "test-actor",
	})
	require.NoError(t, err)
	text := string(fb)
	assert.Contains(t, text, "RAHASIA - BLIPS Tugu Re")
	assert.Contains(t, text, "exported")
	assert.Contains(t, text, "test-actor")
}

func TestExportPDF_WatermarkAppearsTwice(t *testing.T) {
	// Watermark at header + footer (page 1 header and page footer).
	fb, _, err := exporter.ExportPDF(exporter.ExportPDFOptions{
		Headers:    []string{"x"},
		Rows:       [][]string{{"a"}},
		ExportedAt: testExportedAt,
		Username:   "u",
	})
	require.NoError(t, err)
	text := string(fb)
	count := 0
	idx := 0
	needle := "RAHASIA - BLIPS Tugu Re"
	for {
		i := 0
		for j := idx; j < len(text); j++ {
			if text[j] == needle[i] {
				i++
				if i == len(needle) {
					count++
					idx = j + 1
					break
				}
			} else {
				i = 0
			}
		}
		if idx >= len(text) || i < len(needle) {
			break
		}
	}
	// Implementation puts watermark at [WATERMARK] and [PAGE FOOTER] → at least 2 occurrences.
	assert.GreaterOrEqual(t, count, 2, "watermark should appear at header and footer")
}

func TestExportPDF_TitlePresent(t *testing.T) {
	fb, _, err := exporter.ExportPDF(exporter.ExportPDFOptions{
		Title:      "Laporan Status Periode",
		ExportedAt: testExportedAt,
		Username:   "u",
	})
	require.NoError(t, err)
	assert.Contains(t, string(fb), "Laporan Status Periode")
}

func TestExportPDF_HeadersPresent(t *testing.T) {
	fb, _, err := exporter.ExportPDF(exporter.ExportPDFOptions{
		Headers:    []string{"periode_id", "total_ecl", "status"},
		ExportedAt: testExportedAt,
		Username:   "u",
	})
	require.NoError(t, err)
	text := string(fb)
	assert.Contains(t, text, "periode_id")
	assert.Contains(t, text, "total_ecl")
	assert.Contains(t, text, "status")
}

func TestExportPDF_DataRowsPresent(t *testing.T) {
	fb, _, err := exporter.ExportPDF(exporter.ExportPDFOptions{
		Headers:    []string{"a"},
		Rows:       [][]string{{"value-xyz-789"}},
		ExportedAt: testExportedAt,
		Username:   "u",
	})
	require.NoError(t, err)
	assert.Contains(t, string(fb), "value-xyz-789")
}

func TestExportPDF_EmptyRows(t *testing.T) {
	fb, sha, err := exporter.ExportPDF(exporter.ExportPDFOptions{
		Headers:    []string{"a"},
		Rows:       nil,
		ExportedAt: testExportedAt,
		Username:   "u",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, fb)
	assert.NotEmpty(t, sha)
}

func TestExportPDF_EndMarkerPresent(t *testing.T) {
	fb, _, err := exporter.ExportPDF(exporter.ExportPDFOptions{
		ExportedAt: testExportedAt,
		Username:   "u",
	})
	require.NoError(t, err)
	assert.Contains(t, string(fb), "END OF REPORT")
}
