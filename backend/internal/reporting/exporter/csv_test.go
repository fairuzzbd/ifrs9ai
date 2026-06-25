package exporter_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"blips-ifrs9.tugu-re.com/internal/reporting/exporter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testExportedAt = time.Date(2026, 6, 23, 10, 30, 0, 0, time.UTC)

func TestExportCSV_BOM(t *testing.T) {
	var buf bytes.Buffer
	_, err := exporter.ExportCSV(&buf, exporter.ExportCSVOptions{
		Headers:    []string{"col1"},
		Rows:       [][]string{{"val1"}},
		ExportedAt: testExportedAt,
		Username:   "user-test",
	})
	require.NoError(t, err)
	b := buf.Bytes()
	// First 3 bytes must be UTF-8 BOM.
	assert.Equal(t, []byte{0xEF, 0xBB, 0xBF}, b[:3], "missing UTF-8 BOM")
}

func TestExportCSV_HeaderPresent(t *testing.T) {
	var buf bytes.Buffer
	_, err := exporter.ExportCSV(&buf, exporter.ExportCSVOptions{
		Headers:    []string{"periode_id", "ead", "ecl"},
		Rows:       [][]string{{"P001", "1000000", "50000"}},
		ExportedAt: testExportedAt,
		Username:   "akun-user",
	})
	require.NoError(t, err)
	text := buf.String()
	assert.Contains(t, text, "periode_id")
	assert.Contains(t, text, "ead")
	assert.Contains(t, text, "ecl")
}

func TestExportCSV_DataRows(t *testing.T) {
	rows := [][]string{
		{"P001", "500000000.0000", "25000.0000"},
		{"P002", "750000000.0000", "37500.0000"},
	}
	var buf bytes.Buffer
	_, err := exporter.ExportCSV(&buf, exporter.ExportCSVOptions{
		Headers:    []string{"periode", "ead", "ecl"},
		Rows:       rows,
		ExportedAt: testExportedAt,
		Username:   "akun-user",
	})
	require.NoError(t, err)
	text := buf.String()
	assert.Contains(t, text, "500000000.0000")
	assert.Contains(t, text, "750000000.0000")
}

func TestExportCSV_WatermarkPresent(t *testing.T) {
	var buf bytes.Buffer
	_, err := exporter.ExportCSV(&buf, exporter.ExportCSVOptions{
		Headers:    []string{"id"},
		Rows:       [][]string{{"123"}},
		ExportedAt: testExportedAt,
		Username:   "treasury-user",
	})
	require.NoError(t, err)
	text := buf.String()
	assert.Contains(t, text, "RAHASIA - BLIPS Tugu Re")
	assert.Contains(t, text, "exported")
	assert.Contains(t, text, "treasury-user")
}

func TestExportCSV_SHA256Deterministic(t *testing.T) {
	opts := exporter.ExportCSVOptions{
		Headers:    []string{"id", "value"},
		Rows:       [][]string{{"1", "100"}, {"2", "200"}},
		ExportedAt: testExportedAt,
		Username:   "user-abc",
	}
	var buf1, buf2 bytes.Buffer
	sha1, err := exporter.ExportCSV(&buf1, opts)
	require.NoError(t, err)
	sha2, err := exporter.ExportCSV(&buf2, opts)
	require.NoError(t, err)

	assert.Equal(t, sha1, sha2, "same opts must produce same SHA-256")
	assert.Equal(t, buf1.Bytes(), buf2.Bytes(), "same opts must produce identical bytes")
}

func TestExportCSV_SHA256MatchesContent(t *testing.T) {
	var buf bytes.Buffer
	sha, err := exporter.ExportCSV(&buf, exporter.ExportCSVOptions{
		Headers:    []string{"a"},
		Rows:       [][]string{{"1"}},
		ExportedAt: testExportedAt,
		Username:   "u",
	})
	require.NoError(t, err)
	assert.Equal(t, exporter.SHA256OfBytes(buf.Bytes()), sha, "SHA-256 must match file content")
}

func TestExportCSV_EmptyRows(t *testing.T) {
	var buf bytes.Buffer
	sha, err := exporter.ExportCSV(&buf, exporter.ExportCSVOptions{
		Headers:    []string{"col1"},
		Rows:       nil,
		ExportedAt: testExportedAt,
		Username:   "u",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, sha)
	text := buf.String()
	assert.Contains(t, text, "col1")
	assert.Contains(t, text, "RAHASIA")
}

func TestExportCSV_NoHeaders(t *testing.T) {
	var buf bytes.Buffer
	_, err := exporter.ExportCSV(&buf, exporter.ExportCSVOptions{
		Headers:    nil,
		Rows:       [][]string{{"a", "b"}},
		ExportedAt: testExportedAt,
		Username:   "u",
	})
	require.NoError(t, err)
	text := buf.String()
	assert.Contains(t, text, "a")
	// No crash when headers are nil.
}

// errWriter is an io.Writer that always returns an error.
type errWriter struct{ err error }

func (e *errWriter) Write(_ []byte) (int, error) { return 0, e.err }

func TestExportCSV_WriterError(t *testing.T) {
	w := &errWriter{err: errors.New("disk full")}
	_, err := exporter.ExportCSV(w, exporter.ExportCSVOptions{
		Headers: []string{"col1"}, Rows: [][]string{{"x"}},
		ExportedAt: testExportedAt, Username: "u",
	})
	// BOM write fails → ExportCSV returns error.
	assert.Error(t, err)
}

func TestExportCSV_WatermarkAfterData(t *testing.T) {
	var buf bytes.Buffer
	_, err := exporter.ExportCSV(&buf, exporter.ExportCSVOptions{
		Headers:    []string{"x"},
		Rows:       [][]string{{"data-row"}},
		ExportedAt: testExportedAt,
		Username:   "u",
	})
	require.NoError(t, err)
	text := buf.String()
	dataPos := strings.Index(text, "data-row")
	markPos := strings.Index(text, "RAHASIA")
	assert.True(t, dataPos < markPos, "watermark must appear after data rows")
}
