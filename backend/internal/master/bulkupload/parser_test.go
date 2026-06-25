package bulkupload

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

// ─── MIME validation ──────────────────────────────────────────────────────────

func TestValidateFileMIME_ValidXLSX(t *testing.T) {
	magic := []byte{'P', 'K', 0x03, 0x04, 0xFF, 0xFF}
	err := ValidateFileMIME(magic)
	require.NoError(t, err)
}

func TestValidateFileMIME_InvalidBytes(t *testing.T) {
	bad := []byte{0x00, 0x01, 0x02, 0x03}
	err := ValidateFileMIME(bad)
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeBulkMimeInvalid)
}

func TestValidateFileMIME_TooShort(t *testing.T) {
	err := ValidateFileMIME([]byte{0x50, 0x4B})
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeBulkMimeInvalid)
}

func TestValidateFileMIME_Empty(t *testing.T) {
	err := ValidateFileMIME([]byte{})
	require.Error(t, err)
}

// ─── File size validation ─────────────────────────────────────────────────────

func TestValidateFileSize_UnderLimit(t *testing.T) {
	err := ValidateFileSize(1024*1024, 50*1024*1024) // 1MB under 50MB limit
	require.NoError(t, err)
}

func TestValidateFileSize_ExactLimit(t *testing.T) {
	limit := int64(50 * 1024 * 1024)
	err := ValidateFileSize(limit, limit) // exactly at limit
	require.NoError(t, err)
}

func TestValidateFileSize_OverLimit(t *testing.T) {
	err := ValidateFileSize(51*1024*1024, 50*1024*1024)
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeBulkFileTooLarge)
}

func TestValidateFileSize_Zero(t *testing.T) {
	err := ValidateFileSize(0, 50*1024*1024)
	require.NoError(t, err)
}

// ─── Parser happy path ────────────────────────────────────────────────────────

// newTestXLSX creates a minimal XLSX with 5 sheets for testing.
func newTestXLSX(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()

	// Deposito sheet
	f.NewSheet("Deposito")
	f.SetCellValue("Deposito", "A1", "kode")
	f.SetCellValue("Deposito", "B1", "counterparty_id")
	f.SetCellValue("Deposito", "C1", "bank_id")
	f.SetCellValue("Deposito", "D1", "mata_uang")
	f.SetCellValue("Deposito", "E1", "saldo")
	f.SetCellValue("Deposito", "F1", "tanggal_penempatan")
	f.SetCellValue("Deposito", "G1", "jatuh_tempo")
	f.SetCellValue("Deposito", "H1", "bunga")
	// Data row
	f.SetCellValue("Deposito", "A2", "DEP-001")
	f.SetCellValue("Deposito", "B2", "CP-001")
	f.SetCellValue("Deposito", "C2", "BCA")
	f.SetCellValue("Deposito", "D2", "IDR")
	f.SetCellValue("Deposito", "E2", "1000000000")
	f.SetCellValue("Deposito", "F2", "2026-01-01")
	f.SetCellValue("Deposito", "G2", "2027-01-01")
	f.SetCellValue("Deposito", "H2", "0.065")

	// Obligasi sheet
	f.NewSheet("Obligasi")
	f.SetCellValue("Obligasi", "A1", "kode")
	f.SetCellValue("Obligasi", "B1", "issuer_id")
	f.SetCellValue("Obligasi", "C1", "mata_uang")
	f.SetCellValue("Obligasi", "D1", "nilai_nominal")
	f.SetCellValue("Obligasi", "E1", "kupon")
	f.SetCellValue("Obligasi", "F1", "tanggal_penerbitan")
	f.SetCellValue("Obligasi", "G1", "jatuh_tempo")
	f.SetCellValue("Obligasi", "A2", "OBL-001")
	f.SetCellValue("Obligasi", "B2", "ISS-001")
	f.SetCellValue("Obligasi", "C2", "IDR")
	f.SetCellValue("Obligasi", "D2", "5000000000")
	f.SetCellValue("Obligasi", "E2", "0.08")
	f.SetCellValue("Obligasi", "F2", "2025-06-01")
	f.SetCellValue("Obligasi", "G2", "2030-06-01")

	// Saham sheet
	f.NewSheet("Saham")
	f.SetCellValue("Saham", "A1", "kode")
	f.SetCellValue("Saham", "B1", "emiten_id")
	f.SetCellValue("Saham", "C1", "mata_uang")
	f.SetCellValue("Saham", "D1", "jumlah_lembar")
	f.SetCellValue("Saham", "E1", "harga_beli")
	f.SetCellValue("Saham", "A2", "SAH-001")
	f.SetCellValue("Saham", "B2", "EMT-001")
	f.SetCellValue("Saham", "C2", "IDR")
	f.SetCellValue("Saham", "D2", "10000")
	f.SetCellValue("Saham", "E2", "7500")

	// Reksadana sheet
	f.NewSheet("Reksadana")
	f.SetCellValue("Reksadana", "A1", "kode")
	f.SetCellValue("Reksadana", "B1", "manajer_id")
	f.SetCellValue("Reksadana", "C1", "mata_uang")
	f.SetCellValue("Reksadana", "D1", "nilai_investasi")
	f.SetCellValue("Reksadana", "E1", "tanggal_investasi")
	f.SetCellValue("Reksadana", "A2", "RD-001")
	f.SetCellValue("Reksadana", "B2", "MJ-001")
	f.SetCellValue("Reksadana", "C2", "IDR")
	f.SetCellValue("Reksadana", "D2", "2000000000")
	f.SetCellValue("Reksadana", "E2", "2026-03-01")

	// Tabungan_Cash sheet
	f.NewSheet("Tabungan_Cash")
	f.SetCellValue("Tabungan_Cash", "A1", "kode")
	f.SetCellValue("Tabungan_Cash", "B1", "bank_id")
	f.SetCellValue("Tabungan_Cash", "C1", "mata_uang")
	f.SetCellValue("Tabungan_Cash", "D1", "saldo")
	f.SetCellValue("Tabungan_Cash", "E1", "tanggal_penempatan")
	f.SetCellValue("Tabungan_Cash", "A2", "TAB-001")
	f.SetCellValue("Tabungan_Cash", "B2", "BNI")
	f.SetCellValue("Tabungan_Cash", "C2", "IDR")
	f.SetCellValue("Tabungan_Cash", "D2", "500000000")
	f.SetCellValue("Tabungan_Cash", "E2", "2026-01-15")

	// Remove default Sheet1
	f.DeleteSheet("Sheet1")

	buf := new(bytes.Buffer)
	require.NoError(t, f.Write(buf))
	return buf.Bytes()
}

func TestParser_Parse_HappyPath(t *testing.T) {
	data := newTestXLSX(t)
	p := NewParser()
	reader := newBytesReaderAt(data)
	result, err := p.Parse(reader, int64(len(data)))
	require.NoError(t, err)
	assert.Equal(t, 5, result.TotalRows)
	assert.Equal(t, 1, result.SheetBreakdown[SheetDeposito])
	assert.Equal(t, 1, result.SheetBreakdown[SheetObligasi])
	assert.Equal(t, 1, result.SheetBreakdown[SheetSaham])
	assert.Equal(t, 1, result.SheetBreakdown[SheetReksadana])
	assert.Equal(t, 1, result.SheetBreakdown[SheetTabungan])
	assert.Empty(t, result.ParseErrors)
}

func TestParser_Parse_CorrectSheetNames(t *testing.T) {
	data := newTestXLSX(t)
	p := NewParser()
	reader := newBytesReaderAt(data)
	result, err := p.Parse(reader, int64(len(data)))
	require.NoError(t, err)
	for _, row := range result.Rows {
		assert.NotEmpty(t, row.SheetName)
		found := false
		for _, valid := range ValidSheets {
			if row.SheetName == valid {
				found = true
				break
			}
		}
		assert.True(t, found, "unexpected sheet name: %s", row.SheetName)
	}
}

func TestParser_Parse_MissingMandatoryCol(t *testing.T) {
	f := excelize.NewFile()
	f.NewSheet("Deposito")
	// Missing "saldo" column
	f.SetCellValue("Deposito", "A1", "kode")
	f.SetCellValue("Deposito", "B1", "counterparty_id")
	f.SetCellValue("Deposito", "C1", "bank_id")
	f.SetCellValue("Deposito", "D1", "mata_uang")
	// no saldo, tanggal_penempatan, jatuh_tempo, bunga
	f.SetCellValue("Deposito", "A2", "DEP-MISSING")
	f.DeleteSheet("Sheet1")

	buf := new(bytes.Buffer)
	require.NoError(t, f.Write(buf))

	data := buf.Bytes()
	p := NewParser()
	reader := newBytesReaderAt(data)
	result, err := p.Parse(reader, int64(len(data)))
	require.NoError(t, err)
	// Parse errors collected, not fatal
	assert.NotEmpty(t, result.ParseErrors)
}

func TestParser_Parse_MissingSheet(t *testing.T) {
	f := excelize.NewFile()
	// Only create Deposito sheet, others missing
	f.NewSheet("Deposito")
	f.SetCellValue("Deposito", "A1", "kode")
	f.SetCellValue("Deposito", "B1", "counterparty_id")
	f.SetCellValue("Deposito", "C1", "bank_id")
	f.SetCellValue("Deposito", "D1", "mata_uang")
	f.SetCellValue("Deposito", "E1", "saldo")
	f.SetCellValue("Deposito", "F1", "tanggal_penempatan")
	f.SetCellValue("Deposito", "G1", "jatuh_tempo")
	f.SetCellValue("Deposito", "H1", "bunga")
	f.SetCellValue("Deposito", "A2", "DEP-001")
	f.SetCellValue("Deposito", "B2", "CP-001")
	f.SetCellValue("Deposito", "C2", "BCA")
	f.SetCellValue("Deposito", "D2", "IDR")
	f.SetCellValue("Deposito", "E2", "1000000")
	f.SetCellValue("Deposito", "F2", "2026-01-01")
	f.SetCellValue("Deposito", "G2", "2027-01-01")
	f.SetCellValue("Deposito", "H2", "0.05")
	f.DeleteSheet("Sheet1")

	buf := new(bytes.Buffer)
	require.NoError(t, f.Write(buf))

	data := buf.Bytes()
	p := NewParser()
	reader := newBytesReaderAt(data)
	result, err := p.Parse(reader, int64(len(data)))
	require.NoError(t, err) // missing sheets are non-fatal
	// Deposito sheet parsed, others noted as parse error
	assert.Equal(t, 1, result.SheetBreakdown[SheetDeposito])
	// Should have parse errors for missing sheets
	assert.NotEmpty(t, result.ParseErrors)
}

func TestParser_Parse_EmptySheet(t *testing.T) {
	f := excelize.NewFile()
	f.NewSheet("Deposito")
	// Only header row, no data
	f.SetCellValue("Deposito", "A1", "kode")
	f.SetCellValue("Deposito", "B1", "counterparty_id")
	f.SetCellValue("Deposito", "C1", "bank_id")
	f.SetCellValue("Deposito", "D1", "mata_uang")
	f.SetCellValue("Deposito", "E1", "saldo")
	f.SetCellValue("Deposito", "F1", "tanggal_penempatan")
	f.SetCellValue("Deposito", "G1", "jatuh_tempo")
	f.SetCellValue("Deposito", "H1", "bunga")
	f.DeleteSheet("Sheet1")

	buf := new(bytes.Buffer)
	require.NoError(t, f.Write(buf))

	data := buf.Bytes()
	p := NewParser()
	reader := newBytesReaderAt(data)
	result, err := p.Parse(reader, int64(len(data)))
	require.NoError(t, err)
	assert.Equal(t, 0, result.SheetBreakdown[SheetDeposito])
}

func TestParser_Parse_RowDataMapKeys(t *testing.T) {
	data := newTestXLSX(t)
	p := NewParser()
	reader := newBytesReaderAt(data)
	result, err := p.Parse(reader, int64(len(data)))
	require.NoError(t, err)
	// Find the Deposito row
	var depRow *ParsedRow
	for i := range result.Rows {
		if result.Rows[i].SheetName == SheetDeposito {
			depRow = &result.Rows[i]
			break
		}
	}
	require.NotNil(t, depRow)
	assert.Equal(t, "DEP-001", depRow.Data["kode"])
	assert.Equal(t, "IDR", depRow.Data["mata_uang"])
}

func TestParser_Parse_InvalidXLSX(t *testing.T) {
	// Not a real XLSX file
	data := []byte("this is not xlsx content, clearly not valid")
	p := NewParser()
	reader := newBytesReaderAt(data)
	_, err := p.Parse(reader, int64(len(data)))
	// Should return an error from excelize
	require.Error(t, err)
}

func TestParser_Parse_MandatoryColEmpty(t *testing.T) {
	f := excelize.NewFile()
	f.NewSheet("Deposito")
	f.SetCellValue("Deposito", "A1", "kode")
	f.SetCellValue("Deposito", "B1", "counterparty_id")
	f.SetCellValue("Deposito", "C1", "bank_id")
	f.SetCellValue("Deposito", "D1", "mata_uang")
	f.SetCellValue("Deposito", "E1", "saldo")
	f.SetCellValue("Deposito", "F1", "tanggal_penempatan")
	f.SetCellValue("Deposito", "G1", "jatuh_tempo")
	f.SetCellValue("Deposito", "H1", "bunga")
	// Row with empty mandatory "kode"
	f.SetCellValue("Deposito", "A2", "") // empty kode
	f.SetCellValue("Deposito", "B2", "CP-001")
	f.SetCellValue("Deposito", "C2", "BCA")
	f.SetCellValue("Deposito", "D2", "IDR")
	f.SetCellValue("Deposito", "E2", "1000000")
	f.SetCellValue("Deposito", "F2", "2026-01-01")
	f.SetCellValue("Deposito", "G2", "2027-01-01")
	f.SetCellValue("Deposito", "H2", "0.05")
	f.DeleteSheet("Sheet1")

	buf := new(bytes.Buffer)
	require.NoError(t, f.Write(buf))

	data := buf.Bytes()
	p := NewParser()
	reader := newBytesReaderAt(data)
	result, err := p.Parse(reader, int64(len(data)))
	require.NoError(t, err)
	assert.NotEmpty(t, result.ParseErrors)
	// The row should still be included (parse errors collected, not halted)
	assert.Equal(t, 1, len(result.Rows))
}

// ─── bytesReaderAt ────────────────────────────────────────────────────────────

func TestBytesReaderAt_HappyPath(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5}
	r := newBytesReaderAt(data)
	buf := make([]byte, 3)
	n, err := r.ReadAt(buf, 0)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	assert.Equal(t, []byte{1, 2, 3}, buf)
}

func TestBytesReaderAt_Offset(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5}
	r := newBytesReaderAt(data)
	buf := make([]byte, 2)
	n, err := r.ReadAt(buf, 3)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, []byte{4, 5}, buf)
}

func TestBytesReaderAt_BeyondLen(t *testing.T) {
	data := []byte{1, 2}
	r := newBytesReaderAt(data)
	buf := make([]byte, 1)
	_, err := r.ReadAt(buf, 10)
	require.Error(t, err)
}

// TestParser_Parse_RowMoreCellsThanHeader covers the branch in parseSheet
// where a data row has more cells than the header row (i >= len(headers)).
func TestParser_Parse_RowMoreCellsThanHeader(t *testing.T) {
	f := excelize.NewFile()
	f.NewSheet("Deposito")
	// Header: 3 cols
	f.SetCellValue("Deposito", "A1", "kode")
	f.SetCellValue("Deposito", "B1", "counterparty_id")
	f.SetCellValue("Deposito", "C1", "bank_id")
	// Data row: 8 cols (more than header)
	f.SetCellValue("Deposito", "A2", "DEP-001")
	f.SetCellValue("Deposito", "B2", "CP-001")
	f.SetCellValue("Deposito", "C2", "BCA")
	f.SetCellValue("Deposito", "D2", "extra1") // beyond header length
	f.SetCellValue("Deposito", "E2", "extra2")
	f.DeleteSheet("Sheet1")

	buf := new(bytes.Buffer)
	require.NoError(t, f.Write(buf))

	data := buf.Bytes()
	p := NewParser()
	reader := newBytesReaderAt(data)
	result, err := p.Parse(reader, int64(len(data)))
	require.NoError(t, err)
	// Should parse 1 row (extra cells beyond header are ignored)
	assert.GreaterOrEqual(t, len(result.Rows), 1)
}
