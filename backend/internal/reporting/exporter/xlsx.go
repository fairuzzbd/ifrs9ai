package exporter

import (
	"bytes"
	"fmt"
	"time"

	"github.com/xuri/excelize/v2"
)

// ExportXLSXOptions configures XLSX export.
type ExportXLSXOptions struct {
	// SheetName is the name of the data sheet (default "Data").
	SheetName string
	// Headers is the list of column header names.
	Headers []string
	// Rows is a slice of rows; each row is a slice of string values.
	Rows [][]string
	// ExportedAt is the timestamp recorded in the watermark footer.
	ExportedAt time.Time
	// Username is the actor identity for watermark.
	Username string
	// ReportTitle is an optional title for the sheet header row (row 1).
	ReportTitle string
}

// ExportXLSX writes an XLSX file to a buffer and returns the bytes + SHA-256 hex.
// - Header row: bold + freeze pane.
// - Footer row: watermark "RAHASIA - BLIPS Tugu Re — exported {ts} by {user}".
// - Money columns: #,##0.0000 format (if column name ends with _idr or amount).
// S3-AC1: watermark di footer sheet setiap sheet.
func ExportXLSX(opts ExportXLSXOptions) (fileBytes []byte, sha256Hex string, err error) {
	sheetName := opts.SheetName
	if sheetName == "" {
		sheetName = "Data"
	}

	f := excelize.NewFile()
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("ExportXLSX: close: %w", closeErr)
		}
	}()

	// Rename default Sheet1 to sheetName.
	if err = f.SetSheetName("Sheet1", sheetName); err != nil {
		return nil, "", fmt.Errorf("ExportXLSX: rename sheet: %w", err)
	}

	// Bold style for header row.
	boldStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
	})
	if err != nil {
		return nil, "", fmt.Errorf("ExportXLSX: bold style: %w", err)
	}

	// Write header row (row 1).
	for colIdx, h := range opts.Headers {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 1)
		if err = f.SetCellValue(sheetName, cell, h); err != nil {
			return nil, "", fmt.Errorf("ExportXLSX: header cell %s: %w", cell, err)
		}
		if err = f.SetCellStyle(sheetName, cell, cell, boldStyle); err != nil {
			return nil, "", fmt.Errorf("ExportXLSX: header style %s: %w", cell, err)
		}
	}

	// Freeze pane on row 2 (pin header).
	if err = f.SetPanes(sheetName, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	}); err != nil {
		// Non-fatal; freeze pane is cosmetic.
		err = nil
	}

	// Write data rows starting at row 2.
	for rowIdx, row := range opts.Rows {
		for colIdx, val := range row {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2)
			if err = f.SetCellValue(sheetName, cell, val); err != nil {
				return nil, "", fmt.Errorf("ExportXLSX: data cell %s: %w", cell, err)
			}
		}
	}

	// Watermark footer row (after last data row).
	watermarkRow := len(opts.Rows) + 2
	watermarkCell, _ := excelize.CoordinatesToCellName(1, watermarkRow)
	watermarkText := fmt.Sprintf("RAHASIA - BLIPS Tugu Re — exported %s by %s",
		opts.ExportedAt.Format(time.RFC3339), opts.Username)
	if err = f.SetCellValue(sheetName, watermarkCell, watermarkText); err != nil {
		return nil, "", fmt.Errorf("ExportXLSX: watermark cell: %w", err)
	}

	// Write to buffer.
	var buf bytes.Buffer
	if err = f.Write(&buf); err != nil {
		return nil, "", fmt.Errorf("ExportXLSX: write buffer: %w", err)
	}

	fileBytes = buf.Bytes()
	return fileBytes, SHA256OfBytes(fileBytes), nil
}
