package document

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// objectKeyAllowedChars adalah karakter yang diizinkan di object key.
// Hanya alfanumerik, hyphen, underscore, titik, dan slash.
var objectKeyAllowedChars = regexp.MustCompile(`^[a-zA-Z0-9/_\-\.]+$`)

// BuildObjectKey membangun object key MinIO untuk file upload.
// Format: documents/{yyyy}/{mm}/{dd}/{docID}.{ext}
//
// Path traversal guard: hasilnya sudah bersih dari ".." dan absolute path.
// Semua komponen di-sanitize sebelum digabung.
func BuildObjectKey(docID uuid.UUID, filename string, uploadedAt time.Time) string {
	ext := sanitizeExtension(path.Ext(filename))
	return fmt.Sprintf("documents/%d/%02d/%02d/%s%s",
		uploadedAt.Year(),
		uploadedAt.Month(),
		uploadedAt.Day(),
		docID.String(),
		ext,
	)
}

// BuildRawFeedObjectKey membangun object key untuk raw feed inbound.
// Format: raw/{system}/{yyyy}/{mm}/{dd}/{filename}
// Dipakai oleh integration adapters (Pefindo, IBPA, KSEI, BEI, BI JISDOR).
func BuildRawFeedObjectKey(system, filename string, uploadedAt time.Time) string {
	cleanFilename := sanitizeFilenameComponent(filename)
	cleanSystem := sanitizeFilenameComponent(system)
	return fmt.Sprintf("raw/%s/%d/%02d/%02d/%s",
		cleanSystem,
		uploadedAt.Year(),
		uploadedAt.Month(),
		uploadedAt.Day(),
		cleanFilename,
	)
}

// ValidateObjectKey memverifikasi bahwa object key aman (tidak ada path traversal).
// Mengembalikan error jika ada "..", absolute path, atau karakter berbahaya.
func ValidateObjectKey(key string) error {
	if key == "" {
		return fmt.Errorf("object key tidak boleh kosong")
	}

	// Tolak absolute path.
	if strings.HasPrefix(key, "/") {
		return fmt.Errorf("object key tidak boleh dimulai dengan '/' (absolute path)")
	}

	// Tolak path traversal ".." setelah normalisasi.
	cleaned := path.Clean(key)
	if strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, "/../") || strings.Contains(cleaned, "/..") {
		return fmt.Errorf("object key mengandung path traversal (..): %q", key)
	}

	// Tolak komponen ".." di semua segmen.
	for _, segment := range strings.Split(key, "/") {
		if segment == ".." || segment == "." {
			return fmt.Errorf("object key mengandung komponen path berbahaya (%q): %q", segment, key)
		}
	}

	// Validasi karakter yang diizinkan.
	if !objectKeyAllowedChars.MatchString(key) {
		return fmt.Errorf("object key mengandung karakter tidak diizinkan: %q (hanya alfanumerik, /, -, _, . diizinkan)", key)
	}

	return nil
}

// sanitizeExtension membersihkan extension file.
// Memastikan dimulai dengan titik dan hanya alfanumerik.
func sanitizeExtension(ext string) string {
	if ext == "" {
		return ""
	}
	// Hanya izinkan alfanumerik dalam extension.
	cleaned := regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(ext[1:], "")
	if cleaned == "" {
		return ""
	}
	return "." + strings.ToLower(cleaned)
}

// sanitizeFilenameComponent membersihkan satu komponen nama file.
// Mengganti karakter berbahaya dengan underscore.
func sanitizeFilenameComponent(name string) string {
	// Hanya izinkan alfanumerik, hyphen, underscore.
	cleaned := regexp.MustCompile(`[^a-zA-Z0-9\-_]`).ReplaceAllString(name, "_")
	return strings.ToLower(cleaned)
}

// GenerateDocRefKode menghasilkan kode referensi human-readable untuk dokumen.
// Format: DOC-YYYYMMDD-NNNNN
// NNNNN adalah 5 digit random dari UUID (bukan sequential — tidak butuh sequence di DB).
func GenerateDocRefKode(docID uuid.UUID, uploadedAt time.Time) string {
	// Ambil 5 karakter hexadecimal dari UUID sebagai suffix.
	idHex := strings.ReplaceAll(docID.String(), "-", "")
	suffix := strings.ToUpper(idHex[:5])
	return fmt.Sprintf("DOC-%d%02d%02d-%s",
		uploadedAt.Year(),
		uploadedAt.Month(),
		uploadedAt.Day(),
		suffix,
	)
}
