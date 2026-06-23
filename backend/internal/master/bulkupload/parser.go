package bulkupload

// parser.go — XLSX 5-sheet parser using excelize.
//
// Validates:
//   - File size ≤ BULK_FILE_MAX_MB (S1-AC2 — server-side before parse)
//   - MIME: magic bytes PK\x03\x04 (S1-AC3 — not trusting Content-Type)
//   - 5 sheets: Deposito, Obligasi, Saham, Reksadana, Tabungan_Cash
//   - Per-sheet column mapping + parse error collection
//
// Parse errors are COLLECTED, not halted (S1-AC4):
//   rows with parse errors → row_status=FAILED, still inserted in sys.upload_batch_row.
//
// References: P5-M11-S1, DEC-016 (numeric — collected as string for decimal conversion by validator).

import (
	"fmt"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"
)

const (
	// XLSXMagicByte0 is the first byte of a PK zip (XLSX) magic signature.
	XLSXMagicByte0 = 'P'
	// XLSXMagicByte1 is the second byte.
	XLSXMagicByte1 = 'K'
	// XLSXMagicByte2 is the third byte.
	XLSXMagicByte2 = 0x03
	// XLSXMagicByte3 is the fourth byte.
	XLSXMagicByte3 = 0x04
)

// ValidateFileMIME checks the first 4 bytes of the file for XLSX zip magic.
// Returns CodeBulkMimeInvalid error if not PK\x03\x04.
// S1-AC3: server-side magic byte check — do not rely on Content-Type header.
func ValidateFileMIME(header []byte) error {
	if len(header) < 4 {
		return fmt.Errorf("%s: file terlalu kecil untuk divalidasi MIME (< 4 bytes)", CodeBulkMimeInvalid)
	}
	if header[0] != XLSXMagicByte0 || header[1] != XLSXMagicByte1 ||
		header[2] != XLSXMagicByte2 || header[3] != XLSXMagicByte3 {
		return fmt.Errorf("%s: Tipe file tidak valid. Hanya XLSX (application/vnd.openxmlformats-officedocument.spreadsheetml.sheet) yang diterima.", CodeBulkMimeInvalid)
	}
	return nil
}

// ValidateFileSize checks that file size does not exceed maxBytes.
// S1-AC2: server-side size check — do NOT rely on Content-Length header.
func ValidateFileSize(sizeBytes int64, maxBytes int64) error {
	if sizeBytes > maxBytes {
		maxMB := maxBytes / (1024 * 1024)
		sizeMB := sizeBytes / (1024 * 1024)
		return fmt.Errorf("%s: Ukuran file %dMB melebihi batas %dMB. Upload dibatalkan.", CodeBulkFileTooLarge, sizeMB, maxMB)
	}
	return nil
}

// Parser parses an XLSX 5-sheet file into ParsedRows.
type Parser struct{}

// NewParser creates a new XLSX parser.
func NewParser() *Parser { return &Parser{} }

// Parse reads from r and returns a ParseResult with all rows and errors.
// Parse errors are collected per-row (not fatal). Returns error only for fatal file-level issues.
func (p *Parser) Parse(r io.ReaderAt, size int64) (*ParseResult, error) {
	f, err := excelize.OpenReader(io.NewSectionReader(r, 0, size))
	if err != nil {
		return nil, fmt.Errorf("ParseXLSX: excelize.OpenReader: %w", err)
	}
	defer func() { _ = f.Close() }()

	result := &ParseResult{
		SheetBreakdown: make(map[SheetName]int),
	}

	for _, sheet := range ValidSheets {
		rows, parseErrs, err := parseSheet(f, sheet)
		if err != nil {
			// Sheet not found — not fatal, collect as parse warning
			result.ParseErrors = append(result.ParseErrors, RowError{
				Sheet: sheet,
				Row:   0,
				Stage: 0,
				Error: fmt.Sprintf("Sheet '%s' tidak ditemukan dalam file XLSX", sheet),
			})
			continue
		}
		result.Rows = append(result.Rows, rows...)
		result.ParseErrors = append(result.ParseErrors, parseErrs...)
		result.SheetBreakdown[sheet] = len(rows)
	}

	result.TotalRows = len(result.Rows)
	return result, nil
}

// parseSheet parses a single sheet from an excelize.File.
// Returns (rows, parseErrors, error). error is non-nil only if sheet is missing entirely.
func parseSheet(f *excelize.File, sheet SheetName) ([]ParsedRow, []RowError, error) {
	sheetName := string(sheet)
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, nil, fmt.Errorf("sheet '%s' tidak ditemukan: %w", sheetName, err)
	}
	if len(rows) == 0 {
		return nil, nil, nil
	}

	// Row 1 = headers
	headers := rows[0]
	headerIdx := make(map[string]int, len(headers))
	for i, h := range headers {
		headerIdx[strings.TrimSpace(strings.ToLower(h))] = i
	}

	mandatoryCols := MandatoryColumns[sheet]
	var parsed []ParsedRow
	var parseErrs []RowError

	for rowIdx, row := range rows[1:] { // skip header row
		actualRowNum := rowIdx + 2 // 1-indexed, row 1 is header
		rowData := make(map[string]interface{}, len(headers))
		var rowErrors []RowError

		// Build row data map
		for i, val := range row {
			if i < len(headers) {
				key := strings.TrimSpace(strings.ToLower(headers[i]))
				rowData[key] = strings.TrimSpace(val)
			}
		}

		// Check mandatory columns
		for _, col := range mandatoryCols {
			colKey := strings.ToLower(col)
			val, ok := rowData[colKey]
			if !ok {
				_, colExists := headerIdx[colKey]
				if !colExists {
					rowErrors = append(rowErrors, RowError{
						Sheet: sheet,
						Row:   actualRowNum,
						Stage: 1,
						Col:   col,
						Error: fmt.Sprintf("Kolom wajib '%s' tidak ditemukan di header sheet %s", col, sheet),
					})
				} else {
					rowErrors = append(rowErrors, RowError{
						Sheet: sheet,
						Row:   actualRowNum,
						Stage: 1,
						Col:   col,
						Error: fmt.Sprintf("Kolom wajib '%s' kosong di baris %d", col, actualRowNum),
					})
				}
			} else if v, ok := val.(string); ok && v == "" {
				rowErrors = append(rowErrors, RowError{
					Sheet: sheet,
					Row:   actualRowNum,
					Stage: 1,
					Col:   col,
					Error: fmt.Sprintf("Kolom wajib '%s' kosong di baris %d", col, actualRowNum),
				})
			}
		}

		pr := ParsedRow{
			SheetName: sheet,
			RowNumber: actualRowNum,
			Data:      rowData,
		}
		if len(rowErrors) > 0 {
			pr.ParseErrors = rowErrors
			parseErrs = append(parseErrs, rowErrors...)
		}
		parsed = append(parsed, pr)
	}

	return parsed, parseErrs, nil
}
