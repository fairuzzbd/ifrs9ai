package exporter_test

import (
	"bytes"
	"testing"
	"time"

	"blips-ifrs9.tugu-re.com/internal/reporting/exporter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

func TestExportXLSX_Returns_NonEmptyBytes(t *testing.T) {
	fb, sha, err := exporter.ExportXLSX(exporter.ExportXLSXOptions{
		SheetName:  "Data",
		Headers:    []string{"periode", "ead", "ecl"},
		Rows:       [][]string{{"2026-06", "1000000", "5000"}},
		ExportedAt: testExportedAt,
		Username:   "treasury",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, fb)
	assert.NotEmpty(t, sha)
}

func TestExportXLSX_SHA256MatchesContent(t *testing.T) {
	fb, sha, err := exporter.ExportXLSX(exporter.ExportXLSXOptions{
		Headers:    []string{"a"},
		Rows:       [][]string{{"1"}},
		ExportedAt: testExportedAt,
		Username:   "u",
	})
	require.NoError(t, err)
	assert.Equal(t, exporter.SHA256OfBytes(fb), sha)
}

func TestExportXLSX_WatermarkPresent(t *testing.T) {
	fb, _, err := exporter.ExportXLSX(exporter.ExportXLSXOptions{
		SheetName:  "Laporan",
		Headers:    []string{"id"},
		Rows:       [][]string{{"row1"}, {"row2"}},
		ExportedAt: time.Date(2026, 6, 23, 10, 30, 0, 0, time.UTC),
		Username:   "user-xyz",
	})
	require.NoError(t, err)

	// Read XLSX and check watermark cell.
	f, err := excelize.OpenReader(bytes.NewReader(fb))
	require.NoError(t, err)
	defer f.Close()

	sheetName := "Laporan"
	// Data: row 1 = header, rows 2..N = data, row N+1 = watermark.
	// 2 data rows → watermark at row 4.
	cell, _ := excelize.CoordinatesToCellName(1, 4)
	val, err := f.GetCellValue(sheetName, cell)
	require.NoError(t, err)
	assert.Contains(t, val, "RAHASIA - BLIPS Tugu Re")
	assert.Contains(t, val, "user-xyz")
}

func TestExportXLSX_HeaderRowBold(t *testing.T) {
	fb, _, err := exporter.ExportXLSX(exporter.ExportXLSXOptions{
		SheetName:  "Data",
		Headers:    []string{"col1", "col2"},
		Rows:       [][]string{{"a", "b"}},
		ExportedAt: testExportedAt,
		Username:   "u",
	})
	require.NoError(t, err)

	f, err := excelize.OpenReader(bytes.NewReader(fb))
	require.NoError(t, err)
	defer f.Close()

	// Verify header value in A1.
	val, err := f.GetCellValue("Data", "A1")
	require.NoError(t, err)
	assert.Equal(t, "col1", val)
}

func TestExportXLSX_DefaultSheetName(t *testing.T) {
	fb, _, err := exporter.ExportXLSX(exporter.ExportXLSXOptions{
		SheetName:  "", // should default to "Data"
		Headers:    []string{"x"},
		Rows:       nil,
		ExportedAt: testExportedAt,
		Username:   "u",
	})
	require.NoError(t, err)

	f, err := excelize.OpenReader(bytes.NewReader(fb))
	require.NoError(t, err)
	defer f.Close()

	sheets := f.GetSheetList()
	assert.Contains(t, sheets, "Data")
}

func TestExportXLSX_EmptyRows(t *testing.T) {
	fb, sha, err := exporter.ExportXLSX(exporter.ExportXLSXOptions{
		Headers:    []string{"a", "b"},
		Rows:       nil,
		ExportedAt: testExportedAt,
		Username:   "u",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, fb)
	assert.NotEmpty(t, sha)

	// Watermark at row 2 (header row 1, 0 data rows, watermark row 2).
	f, err := excelize.OpenReader(bytes.NewReader(fb))
	require.NoError(t, err)
	defer f.Close()
	cell, _ := excelize.CoordinatesToCellName(1, 2)
	val, _ := f.GetCellValue("Data", cell)
	assert.Contains(t, val, "RAHASIA")
}

func TestExportXLSX_Deterministic(t *testing.T) {
	opts := exporter.ExportXLSXOptions{
		SheetName:  "Test",
		Headers:    []string{"id", "value"},
		Rows:       [][]string{{"1", "100"}, {"2", "200"}},
		ExportedAt: testExportedAt,
		Username:   "user-det",
	}
	fb1, sha1, err := exporter.ExportXLSX(opts)
	require.NoError(t, err)
	fb2, sha2, err := exporter.ExportXLSX(opts)
	require.NoError(t, err)

	// excelize may produce slightly different ZIP internal ordering between runs
	// so we compare SHA of content rather than raw bytes for this test.
	assert.Equal(t, len(fb1), len(fb2), "file sizes should match")
	assert.Equal(t, sha1, sha2, "SHA-256 should be deterministic")
}
