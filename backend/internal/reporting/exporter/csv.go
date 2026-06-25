package exporter

import (
	"encoding/csv"
	"fmt"
	"io"
	"time"
)

// WatermarkCSV is the CSV comment watermark footer format.
// Appended as last line of the CSV file.
const watermarkCSVFmt = "# RAHASIA - BLIPS Tugu Re — exported %s by %s"

// ExportCSVOptions configures CSV export.
type ExportCSVOptions struct {
	// Headers is the list of column header names.
	Headers []string
	// Rows is a slice of rows; each row is a slice of string values.
	Rows [][]string
	// ExportedAt is the timestamp recorded in the watermark.
	ExportedAt time.Time
	// Username is the actor identity for watermark.
	Username string
}

// ExportCSV writes UTF-8 BOM + CSV rows + watermark comment to w.
// Returns SHA-256 hex of the written bytes via SHA256Writer.
// S3-AC1: watermark every CSV; UTF-8 BOM for Excel compatibility.
func ExportCSV(w io.Writer, opts ExportCSVOptions) (sha256Hex string, err error) {
	sw := NewSHA256Writer(w)

	// UTF-8 BOM (for Excel ID compatibility per ux-patterns.md §1.4)
	if _, err = sw.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return "", fmt.Errorf("ExportCSV: BOM: %w", err)
	}

	cw := csv.NewWriter(sw)

	// Write header row.
	if len(opts.Headers) > 0 {
		if err = cw.Write(opts.Headers); err != nil {
			return "", fmt.Errorf("ExportCSV: header: %w", err)
		}
	}

	// Write data rows.
	for _, row := range opts.Rows {
		if err = cw.Write(row); err != nil {
			return "", fmt.Errorf("ExportCSV: row: %w", err)
		}
	}
	cw.Flush()
	if err = cw.Error(); err != nil {
		return "", fmt.Errorf("ExportCSV: flush: %w", err)
	}

	// Watermark footer.
	watermark := fmt.Sprintf(watermarkCSVFmt,
		opts.ExportedAt.Format(time.RFC3339), opts.Username)
	if _, err = io.WriteString(sw, "\n"+watermark+"\n"); err != nil {
		return "", fmt.Errorf("ExportCSV: watermark: %w", err)
	}

	return sw.Sum(), nil
}
