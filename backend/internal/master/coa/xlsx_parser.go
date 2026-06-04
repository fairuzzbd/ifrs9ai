package coa

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// xlsxBytesToRows parses an XLSX file (ZIP+XML) and returns data rows.
// Skips the header row (row 1). Empty rows (all blank cells) are skipped.
//
// Expected column order (A–H):
//   A: kode_akun
//   B: nama_akun
//   C: tipe_akun
//   D: sub_tipe_akun
//   E: kategori_investasi (optional)
//   F: mata_uang_native (optional)
//   G: posisi_normal
//   H: parent_akun_kode (optional)
func xlsxBytesToRows(data []byte) ([]XLSXRow, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("xlsxBytesToRows: open zip: %w", err)
	}

	// 1. Load shared strings table (xl/sharedStrings.xml).
	sharedStrings, err := loadSharedStrings(zr)
	if err != nil {
		return nil, fmt.Errorf("xlsxBytesToRows: shared strings: %w", err)
	}

	// 2. Parse the first worksheet (xl/worksheets/sheet1.xml).
	rawRows, err := loadSheet1(zr)
	if err != nil {
		return nil, fmt.Errorf("xlsxBytesToRows: load sheet1: %w", err)
	}

	// 3. Convert raw XML rows to XLSXRow, skipping header.
	var result []XLSXRow
	for i, raw := range rawRows {
		if i == 0 {
			continue // skip header
		}
		// Resolve cells to string values.
		cells := make([]string, 8)
		for _, c := range raw.C {
			col := colIndex(c.R)
			if col < 0 || col >= 8 {
				continue
			}
			cells[col] = resolveValue(c, sharedStrings)
		}

		// Skip completely empty rows.
		allEmpty := true
		for _, v := range cells {
			if strings.TrimSpace(v) != "" {
				allEmpty = false
				break
			}
		}
		if allEmpty {
			continue
		}

		result = append(result, XLSXRow{
			RowNum:            i + 1, // 1-indexed (row 1 = header)
			KodeAkun:          strings.TrimSpace(cells[0]),
			NamaAkun:          strings.TrimSpace(cells[1]),
			TipeAkun:          strings.ToUpper(strings.TrimSpace(cells[2])),
			SubTipeAkun:       strings.TrimSpace(cells[3]),
			KategoriInvestasi: strings.TrimSpace(cells[4]),
			MataUangNative:    strings.ToUpper(strings.TrimSpace(cells[5])),
			PosisiNormal:      strings.ToUpper(strings.TrimSpace(cells[6])),
			ParentAkunKode:    strings.TrimSpace(cells[7]),
		})
	}
	return result, nil
}

// ─── XML structs for XLSX parsing ────────────────────────────────────────────

type xlsxSst struct {
	XMLName xml.Name  `xml:"sst"`
	SI      []xlsxSi  `xml:"si"`
}

type xlsxSi struct {
	T  string     `xml:"t"`
	R  []xlsxRun  `xml:"r"`
}

func (si xlsxSi) Value() string {
	if si.T != "" {
		return si.T
	}
	var sb strings.Builder
	for _, r := range si.R {
		sb.WriteString(r.T)
	}
	return sb.String()
}

type xlsxRun struct {
	T string `xml:"t"`
}

type xlsxWorksheet struct {
	XMLName   xml.Name      `xml:"worksheet"`
	SheetData xlsxSheetData `xml:"sheetData"`
}

type xlsxSheetData struct {
	Row []xlsxRow `xml:"row"`
}

type xlsxRow struct {
	R int        `xml:"r,attr"`
	C []xlsxCell `xml:"c"`
}

type xlsxCell struct {
	R string `xml:"r,attr"` // cell reference e.g. "A1"
	T string `xml:"t,attr"` // type: "s" = shared string, "n" = number, "" = inline
	V string `xml:"v"`      // value
}

// loadSharedStrings reads xl/sharedStrings.xml from the ZIP.
func loadSharedStrings(zr *zip.Reader) ([]string, error) {
	for _, f := range zr.File {
		if f.Name == "xl/sharedStrings.xml" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close() //nolint:errcheck
			b, err := io.ReadAll(rc)
			if err != nil {
				return nil, err
			}
			var sst xlsxSst
			if err := xml.Unmarshal(b, &sst); err != nil {
				return nil, fmt.Errorf("parse sharedStrings: %w", err)
			}
			result := make([]string, len(sst.SI))
			for i, si := range sst.SI {
				result[i] = si.Value()
			}
			return result, nil
		}
	}
	// No shared strings — file may contain only inline strings.
	return nil, nil
}

// loadSheet1 reads xl/worksheets/sheet1.xml from the ZIP.
func loadSheet1(zr *zip.Reader) ([]xlsxRow, error) {
	for _, f := range zr.File {
		if f.Name == "xl/worksheets/sheet1.xml" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close() //nolint:errcheck
			b, err := io.ReadAll(rc)
			if err != nil {
				return nil, err
			}
			var ws xlsxWorksheet
			if err := xml.Unmarshal(b, &ws); err != nil {
				return nil, fmt.Errorf("parse sheet1: %w", err)
			}
			return ws.SheetData.Row, nil
		}
	}
	return nil, fmt.Errorf("sheet1.xml not found in XLSX")
}

// resolveValue converts a cell value to a string, dereferencing shared strings.
func resolveValue(c xlsxCell, sharedStrings []string) string {
	if c.T == "s" {
		// Shared string index.
		idx, err := strconv.Atoi(c.V)
		if err != nil || idx < 0 || idx >= len(sharedStrings) {
			return ""
		}
		return sharedStrings[idx]
	}
	return c.V
}

// colIndex converts a cell reference (e.g. "A1", "B3") to a 0-based column index.
// Returns -1 if the reference cannot be parsed.
func colIndex(ref string) int {
	if ref == "" {
		return -1
	}
	// Extract the letter prefix.
	letters := strings.Builder{}
	for _, r := range ref {
		if r >= 'A' && r <= 'Z' {
			letters.WriteRune(r)
		} else {
			break
		}
	}
	s := letters.String()
	if s == "" {
		return -1
	}
	// Convert base-26 (A=1, Z=26, AA=27, ...) to 0-based index.
	n := 0
	for _, r := range s {
		n = n*26 + int(r-'A') + 1
	}
	return n - 1
}
