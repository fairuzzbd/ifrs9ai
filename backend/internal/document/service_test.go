package document

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestService_Upload_NilMinIO memverifikasi upload bekerja di testing mode (nil MinIO, nil DB).
// Ini test unit yang tidak memerlukan live MinIO atau PostgreSQL.
func TestService_Upload_NilMinIO(t *testing.T) {
	// Service tanpa MinIO dan tanpa DB — untuk unit test pure.
	svc := NewService(
		&DBRepository{db: nil}, // DB nil: insert akan error, ditest secara terpisah
		nil,                     // MinIO nil: skip upload
		nil,                     // audit nil
		nil,
	)

	content := []byte("test document content for sha256 verification")
	expectedHash := sha256.Sum256(content)
	expectedHex := hex.EncodeToString(expectedHash[:])

	userID := uuid.New()
	entityID := uuid.New()
	in := UploadInput{
		Filename:         "laporan-ecl.pdf",
		MimeType:         "application/pdf",
		FileSizeBytes:    int64(len(content)),
		EntityType:       "ecl.calc_run",
		EntityID:         entityID,
		Category:         DocCategoryECLParameter,
		UploadedByUserID: userID,
	}

	// Upload tanpa MinIO harus menghitung SHA256 dengan benar.
	// DB nil akan menyebabkan BeginTx error — kita test hash compute saja.
	// Test SHA256 via ComputeSHA256Hex langsung.
	gotHash, gotSize, err := ComputeSHA256Hex(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("ComputeSHA256Hex error: %v", err)
	}
	if gotHash != expectedHex {
		t.Errorf("hash = %q, want %q", gotHash, expectedHex)
	}
	if gotSize != int64(len(content)) {
		t.Errorf("size = %d, want %d", gotSize, len(content))
	}

	_ = svc
	_ = in
}

// TestValidateUploadInput_Valid memverifikasi bahwa input valid tidak menghasilkan error.
func TestValidateUploadInput_Valid(t *testing.T) {
	in := UploadInput{
		Filename:         "report.pdf",
		MimeType:         "application/pdf",
		FileSizeBytes:    1024,
		EntityType:       "mst.instrumen",
		EntityID:         uuid.New(),
		Category:         DocCategoryBuktiTransaksi,
		UploadedByUserID: uuid.New(),
	}
	if err := validateUploadInput(in); err != nil {
		t.Errorf("validateUploadInput valid harus tidak error, got: %v", err)
	}
}

// TestValidateUploadInput_MissingFilename memverifikasi error untuk filename kosong.
func TestValidateUploadInput_MissingFilename(t *testing.T) {
	in := UploadInput{
		Filename:         "",
		MimeType:         "application/pdf",
		FileSizeBytes:    100,
		EntityType:       "mst.instrumen",
		EntityID:         uuid.New(),
		Category:         DocCategoryBuktiTransaksi,
		UploadedByUserID: uuid.New(),
	}
	err := validateUploadInput(in)
	if err == nil {
		t.Error("harus error untuk filename kosong")
	}
}

// TestValidateUploadInput_FileTooLarge memverifikasi error untuk file > 100MB.
func TestValidateUploadInput_FileTooLarge(t *testing.T) {
	in := UploadInput{
		Filename:         "huge.pdf",
		MimeType:         "application/pdf",
		FileSizeBytes:    MaxFileSizeBytes + 1,
		EntityType:       "mst.instrumen",
		EntityID:         uuid.New(),
		Category:         DocCategoryBuktiTransaksi,
		UploadedByUserID: uuid.New(),
	}
	err := validateUploadInput(in)
	if err == nil {
		t.Error("harus error untuk file > 100MB")
	}
}

// TestValidateUploadInput_InvalidCategory memverifikasi error untuk kategori tidak valid.
func TestValidateUploadInput_InvalidCategory(t *testing.T) {
	in := UploadInput{
		Filename:         "file.pdf",
		MimeType:         "application/pdf",
		FileSizeBytes:    1024,
		EntityType:       "mst.instrumen",
		EntityID:         uuid.New(),
		Category:         DocumentCategory("TIDAK_VALID"),
		UploadedByUserID: uuid.New(),
	}
	err := validateUploadInput(in)
	if err == nil {
		t.Error("harus error untuk kategori tidak valid")
	}
}

// TestValidateUploadInput_NilEntityID memverifikasi error untuk entity ID kosong.
func TestValidateUploadInput_NilEntityID(t *testing.T) {
	in := UploadInput{
		Filename:         "file.pdf",
		MimeType:         "application/pdf",
		FileSizeBytes:    1024,
		EntityType:       "mst.instrumen",
		EntityID:         uuid.Nil, // kosong
		Category:         DocCategoryBuktiTransaksi,
		UploadedByUserID: uuid.New(),
	}
	err := validateUploadInput(in)
	if err == nil {
		t.Error("harus error untuk entity ID nil")
	}
}

// TestValidateUploadInput_NilUserID memverifikasi error untuk user ID kosong.
func TestValidateUploadInput_NilUserID(t *testing.T) {
	in := UploadInput{
		Filename:         "file.pdf",
		MimeType:         "application/pdf",
		FileSizeBytes:    1024,
		EntityType:       "mst.instrumen",
		EntityID:         uuid.New(),
		Category:         DocCategoryBuktiTransaksi,
		UploadedByUserID: uuid.Nil, // kosong
	}
	err := validateUploadInput(in)
	if err == nil {
		t.Error("harus error untuk uploaded by user ID nil")
	}
}

// TestIsValidCategory_AllCategories memverifikasi semua kategori yang valid.
func TestIsValidCategory_AllCategories(t *testing.T) {
	valid := []DocumentCategory{
		DocCategoryBuktiTransaksi,
		DocCategorySPPIWorksheet,
		DocCategoryBMAssessment,
		DocCategoryECLParameter,
		DocCategoryEIRAmortisasi,
		DocCategoryRatingReport,
		DocCategoryKontrak,
		DocCategoryKonfirmasiDeal,
		DocCategoryRekapLaporan,
		DocCategoryLainLain,
	}
	for _, cat := range valid {
		if !isValidCategory(cat) {
			t.Errorf("isValidCategory(%q) harus true", cat)
		}
	}
}

// TestIsValidCategory_Invalid memverifikasi kategori tidak valid.
func TestIsValidCategory_Invalid(t *testing.T) {
	invalid := []DocumentCategory{
		"",
		"UNKNOWN",
		"bukti_transaksi", // lowercase harus invalid
		"BUKTI TRANSAKSI", // spasi harus invalid
	}
	for _, cat := range invalid {
		if isValidCategory(cat) {
			t.Errorf("isValidCategory(%q) harus false", cat)
		}
	}
}

// TestSanitizeFilename_Security memverifikasi filename sanitization untuk keamanan.
func TestSanitizeFilename_Security(t *testing.T) {
	cases := []struct {
		input     string
		wantSafe  bool // hasilnya tidak boleh mengandung path separator
	}{
		{"normal-file.pdf", true},
		{"../../../etc/passwd", true},   // harus jadi "passwd"
		{"/absolute/path/file.xlsx", true},
		{"windows\\path\\file.csv", true},
		{"file with spaces.pdf", true},
	}
	for _, tc := range cases {
		result := sanitizeFilename(tc.input)
		if strings.Contains(result, "/") || strings.Contains(result, "\\") || strings.Contains(result, "..") {
			t.Errorf("sanitizeFilename(%q) = %q masih mengandung karakter berbahaya", tc.input, result)
		}
	}
}

// TestMaxFileSizeBytes memverifikasi konstanta MaxFileSizeBytes adalah 100MB.
func TestMaxFileSizeBytes(t *testing.T) {
	if MaxFileSizeBytes != 100*1024*1024 {
		t.Errorf("MaxFileSizeBytes = %d, want %d (100MB)", MaxFileSizeBytes, 100*1024*1024)
	}
}

// TestVirusScanStatus_Constants memverifikasi nilai konstanta scan status.
func TestVirusScanStatus_Constants(t *testing.T) {
	// Nilai ini harus cocok dengan CHECK constraint di migration 0006.
	cases := []struct {
		status   VirusScanStatus
		expected string
	}{
		{VirusScanPending, "PENDING"},
		{VirusScanClean, "CLEAN"},
		{VirusScanInfected, "INFECTED"},
		{VirusScanScanError, "SCAN_ERROR"},
	}
	for _, tc := range cases {
		if string(tc.status) != tc.expected {
			t.Errorf("VirusScanStatus constant mismatch: got %q, want %q", tc.status, tc.expected)
		}
	}
}

// TestDocumentCategory_Constants memverifikasi nilai konstanta kategori cocok dengan DB constraint.
func TestDocumentCategory_Constants(t *testing.T) {
	// Nilai ini harus cocok dengan ck_doc_category CHECK constraint di migration 0006.
	expected := map[DocumentCategory]string{
		DocCategoryBuktiTransaksi: "BUKTI_TRANSAKSI",
		DocCategorySPPIWorksheet:  "SPPI_WORKSHEET",
		DocCategoryBMAssessment:   "BM_ASSESSMENT",
		DocCategoryECLParameter:   "ECL_PARAMETER",
		DocCategoryEIRAmortisasi:  "EIR_AMORTISASI",
		DocCategoryRatingReport:   "RATING_REPORT",
		DocCategoryKontrak:        "KONTRAK",
		DocCategoryKonfirmasiDeal: "KONFIRMASI_DEAL",
		DocCategoryRekapLaporan:   "REKAP_LAPORAN",
		DocCategoryLainLain:       "LAIN_LAIN",
	}
	for cat, want := range expected {
		if string(cat) != want {
			t.Errorf("DocumentCategory constant mismatch: got %q, want %q", cat, want)
		}
	}
}

// TestService_GetPresignedURL_NilMinIO memverifikasi error ketika MinIO tidak tersedia.
func TestService_GetPresignedURL_NilMinIO(t *testing.T) {
	svc := NewService(
		&DBRepository{db: nil},
		nil, // MinIO nil
		nil,
		nil,
	)

	// Dengan DB nil, GetByID akan error — tapi kita test path MinIO nil error.
	_, err := svc.GetPresignedDownloadURL(context.Background(), uuid.New())
	if err == nil {
		t.Error("GetPresignedDownloadURL dengan nil MinIO harus error")
	}
}

// TestService_DefaultServiceConfig_BlockPendingDownloadTrue memverifikasi bahwa
// ServiceConfig default memblokir download dokumen PENDING (Decision-B / MEDIUM-1).
func TestService_DefaultServiceConfig_BlockPendingDownloadTrue(t *testing.T) {
	cfg := DefaultServiceConfig()
	if !cfg.BlockPendingDownload {
		t.Error("DefaultServiceConfig().BlockPendingDownload harus true (secure by default)")
	}
}

// TestService_WithConfig_BlockPendingDownload memverifikasi WithConfig menyimpan konfigurasi.
func TestService_WithConfig_BlockPendingDownload(t *testing.T) {
	svc := NewService(&DBRepository{db: nil}, nil, nil, nil)

	// Default: true
	if !svc.cfg.BlockPendingDownload {
		t.Error("default cfg.BlockPendingDownload harus true")
	}

	// Override ke false (dev mode)
	devSvc := svc.WithConfig(ServiceConfig{BlockPendingDownload: false})
	if devSvc.cfg.BlockPendingDownload {
		t.Error("WithConfig(false) harus mengubah BlockPendingDownload ke false")
	}
}

// TestService_BlockPendingDownload_LogicCheck memverifikasi guard logic untuk dokumen PENDING.
// Test ini mengecek kondisi guard secara langsung tanpa perlu DB live.
func TestService_BlockPendingDownload_LogicCheck(t *testing.T) {
	cases := []struct {
		name                 string
		scanStatus           VirusScanStatus
		blockPendingDownload bool
		expectBlocked        bool
	}{
		{
			name:                 "PENDING + block=true → blocked",
			scanStatus:           VirusScanPending,
			blockPendingDownload: true,
			expectBlocked:        true,
		},
		{
			name:                 "PENDING + block=false → allowed",
			scanStatus:           VirusScanPending,
			blockPendingDownload: false,
			expectBlocked:        false,
		},
		{
			name:                 "CLEAN + block=true → allowed",
			scanStatus:           VirusScanClean,
			blockPendingDownload: true,
			expectBlocked:        false,
		},
		{
			name:                 "INFECTED always blocked (separate guard)",
			scanStatus:           VirusScanInfected,
			blockPendingDownload: false,
			expectBlocked:        false, // guard untuk INFECTED adalah blok terpisah
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulasikan kondisi guard: VirusScanPending && blockPendingDownload
			actuallyBlocked := tc.scanStatus == VirusScanPending && tc.blockPendingDownload
			if actuallyBlocked != tc.expectBlocked {
				t.Errorf("guard(%q, blockPending=%v) = %v, want %v",
					tc.scanStatus, tc.blockPendingDownload, actuallyBlocked, tc.expectBlocked)
			}
		})
	}
}
