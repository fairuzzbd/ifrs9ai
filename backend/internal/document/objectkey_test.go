package document

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestBuildObjectKey_Format memverifikasi format object key yang dihasilkan.
func TestBuildObjectKey_Format(t *testing.T) {
	docID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ts := time.Date(2026, 6, 2, 10, 30, 0, 0, time.UTC)

	key := BuildObjectKey(docID, "laporan-keuangan.pdf", ts)

	// Harus diawali dengan "documents/"
	if !strings.HasPrefix(key, "documents/") {
		t.Errorf("key harus diawali 'documents/', got: %q", key)
	}

	// Harus mengandung tanggal
	if !strings.Contains(key, "2026/06/02") {
		t.Errorf("key harus mengandung tanggal 2026/06/02, got: %q", key)
	}

	// Harus mengandung UUID
	if !strings.Contains(key, docID.String()) {
		t.Errorf("key harus mengandung UUID, got: %q", key)
	}

	// Harus diakhiri dengan .pdf
	if !strings.HasSuffix(key, ".pdf") {
		t.Errorf("key harus diakhiri dengan .pdf, got: %q", key)
	}

	// Tidak boleh ada path traversal
	if strings.Contains(key, "..") {
		t.Errorf("key tidak boleh mengandung '..', got: %q", key)
	}
}

// TestBuildObjectKey_NonAlphanumericInExtension memverifikasi ekstensi dengan karakter
// non-alfanumerik (selain titik) di-strip dari dalam ekstensi.
// Catatan: ekstensi alfanumerik seperti .php, .pdf, .csv tetap valid — BLIPS tidak
// memblokir berdasarkan ekstensi (MIME type validation di service layer).
// Yang di-strip adalah karakter non-alfanumerik DI DALAM ekstensi, mis. ".ph p" → ".php".
func TestBuildObjectKey_NonAlphanumericInExtension(t *testing.T) {
	docID := uuid.New()
	ts := time.Now()

	// Ekstensi dengan karakter non-alfanumerik di dalamnya harus di-strip.
	key := BuildObjectKey(docID, "file.ph p", ts)
	// Spasi harus di-strip dari ekstensi — hasilnya ".php" atau tanpa spasi.
	if strings.Contains(key, " ") {
		t.Errorf("object key tidak boleh mengandung spasi, got: %q", key)
	}

	// Double-check: hasil tetap valid sebagai object key.
	if err := ValidateObjectKey(key); err != nil {
		t.Errorf("object key hasil BuildObjectKey harus valid, got error: %v (key=%q)", err, key)
	}
}

// TestBuildRawFeedObjectKey_Format memverifikasi format untuk raw feed.
func TestBuildRawFeedObjectKey_Format(t *testing.T) {
	ts := time.Date(2026, 6, 2, 10, 30, 0, 0, time.UTC)
	key := BuildRawFeedObjectKey("pefindo", "rating-q2-2026.xlsx", ts)

	if !strings.HasPrefix(key, "raw/pefindo/") {
		t.Errorf("key harus diawali 'raw/pefindo/', got: %q", key)
	}
	if !strings.Contains(key, "2026/06/02") {
		t.Errorf("key harus mengandung tanggal, got: %q", key)
	}
}

// TestValidateObjectKey_Valid memverifikasi key yang valid lolos validasi.
func TestValidateObjectKey_Valid(t *testing.T) {
	validKeys := []string{
		"documents/2026/06/02/abc123.pdf",
		"raw/pefindo/2026/06/02/rating.xlsx",
		"exports/tenant/user/2026/06/02/job123.csv",
		"a/b/c",
		"file.txt",
		"file-name_v2.0.pdf",
	}
	for _, key := range validKeys {
		if err := ValidateObjectKey(key); err != nil {
			t.Errorf("ValidateObjectKey(%q) error = %v, want nil", key, err)
		}
	}
}

// TestValidateObjectKey_PathTraversal memverifikasi bahwa ".." di-reject.
// Ini adalah test keamanan kritis — path traversal harus di-reject selalu.
func TestValidateObjectKey_PathTraversal(t *testing.T) {
	malicious := []string{
		"../etc/passwd",
		"documents/../../etc/passwd",
		"../",
		"..",
		"documents/../../../secret",
		"a/b/../../../etc/shadow",
		"./../../config",
	}
	for _, key := range malicious {
		err := ValidateObjectKey(key)
		if err == nil {
			t.Errorf("ValidateObjectKey(%q) harus mengembalikan error untuk path traversal", key)
		}
	}
}

// TestValidateObjectKey_AbsolutePath memverifikasi bahwa absolute path di-reject.
func TestValidateObjectKey_AbsolutePath(t *testing.T) {
	keys := []string{
		"/etc/passwd",
		"/absolute/path",
		"/",
	}
	for _, key := range keys {
		err := ValidateObjectKey(key)
		if err == nil {
			t.Errorf("ValidateObjectKey(%q) harus mengembalikan error untuk absolute path", key)
		}
	}
}

// TestValidateObjectKey_ForbiddenChars memverifikasi karakter berbahaya di-reject.
func TestValidateObjectKey_ForbiddenChars(t *testing.T) {
	keys := []string{
		"path with spaces/file.pdf",
		"path\x00null/file",
		"path;rm -rf /",
		"file$(whoami).pdf",
	}
	for _, key := range keys {
		err := ValidateObjectKey(key)
		if err == nil {
			t.Errorf("ValidateObjectKey(%q) harus mengembalikan error untuk karakter berbahaya", key)
		}
	}
}

// TestValidateObjectKey_Empty memverifikasi bahwa key kosong di-reject.
func TestValidateObjectKey_Empty(t *testing.T) {
	if err := ValidateObjectKey(""); err == nil {
		t.Error("ValidateObjectKey kosong harus mengembalikan error")
	}
}

// TestGenerateDocRefKode_Format memverifikasi format kode referensi.
func TestGenerateDocRefKode_Format(t *testing.T) {
	docID := uuid.New()
	ts := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)

	kode := GenerateDocRefKode(docID, ts)

	if !strings.HasPrefix(kode, "DOC-20260602-") {
		t.Errorf("kode harus diawali 'DOC-20260602-', got: %q", kode)
	}
	if len(kode) != len("DOC-20260602-XXXXX") {
		t.Errorf("panjang kode tidak sesuai: %q (len=%d)", kode, len(kode))
	}
}

// TestSanitizeFilename_StripPath memverifikasi bahwa path di-strip dari filename.
func TestSanitizeFilename_StripPath(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"simple.pdf", "simple.pdf"},
		{"/etc/passwd", "passwd"},
		{"../../etc/shadow", "shadow"},
		{"dir/subdir/file.xlsx", "file.xlsx"},
		{"windows\\path\\file.csv", "file.csv"},
	}

	for _, tc := range cases {
		got := sanitizeFilename(tc.input)
		if got != tc.want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
