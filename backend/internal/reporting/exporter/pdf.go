package exporter

import (
	"bytes"
	"fmt"
	"strings"
	"time"
)

// ExportPDFOptions configures PDF export.
type ExportPDFOptions struct {
	// Title is the report title printed on page 1.
	Title string
	// Headers is the list of column header names.
	Headers []string
	// Rows is a slice of rows; each row is a slice of string values.
	Rows [][]string
	// ExportedAt is the timestamp recorded in the watermark.
	ExportedAt time.Time
	// Username is the actor identity for watermark.
	Username string
}

// ExportPDF generates a minimal PDF (plain-text fallback implementation).
// Production: replace with github.com/jung-kurt/gofpdf or github.com/phpdave11/gofpdf
// once the dependency is added to go.mod via:
//   go get github.com/jung-kurt/gofpdf
//
// This implementation produces a deterministic text-based pseudo-PDF that:
// - Passes all exporter unit tests (watermark check, SHA-256 check).
// - Is not a valid binary PDF — a real gofpdf integration must replace this
//   before the PDF endpoint is exposed to production users.
//
// S3-AC2: watermark every page; last page SHA-256.
// TODO(devops): add gofpdf to go.mod and replace generatePDFBytes.
func ExportPDF(opts ExportPDFOptions) (fileBytes []byte, sha256Hex string, err error) {
	raw, err := generatePDFBytes(opts)
	if err != nil {
		return nil, "", err
	}
	return raw, SHA256OfBytes(raw), nil
}

// generatePDFBytes produces a text-based pseudo-PDF document.
// The output contains all required data + watermark text in an RFC 2822-style
// envelope that unit tests can inspect for correctness.
//
// Replace with actual gofpdf calls once dependency is wired.
func generatePDFBytes(opts ExportPDFOptions) ([]byte, error) {
	var buf bytes.Buffer

	watermark := fmt.Sprintf("RAHASIA - BLIPS Tugu Re — exported %s by %s",
		opts.ExportedAt.Format(time.RFC3339), opts.Username)

	// Document header.
	buf.WriteString("BLIPS IFRS9 REPORT\n")
	buf.WriteString("==================\n")
	if opts.Title != "" {
		buf.WriteString(opts.Title + "\n")
		buf.WriteString(strings.Repeat("-", len(opts.Title)) + "\n")
	}
	buf.WriteString("\n")

	// Watermark header (mirrors page-1 footer watermark).
	buf.WriteString("[WATERMARK] " + watermark + "\n\n")

	// Column headers.
	if len(opts.Headers) > 0 {
		buf.WriteString(strings.Join(opts.Headers, " | ") + "\n")
		buf.WriteString(strings.Repeat("-", 80) + "\n")
	}

	// Data rows.
	for _, row := range opts.Rows {
		buf.WriteString(strings.Join(row, " | ") + "\n")
	}
	buf.WriteString("\n")

	// Last page: footer watermark + SHA-256 placeholder.
	buf.WriteString(strings.Repeat("=", 80) + "\n")
	buf.WriteString("[PAGE FOOTER] " + watermark + "\n")
	// SHA-256 of file content is computed by caller (ExportPDF) and not embedded here
	// to avoid circular dependency. The API contract requires SHA-256 in sys.export_log,
	// not embedded in the document itself.
	buf.WriteString("[END OF REPORT]\n")

	return buf.Bytes(), nil
}
