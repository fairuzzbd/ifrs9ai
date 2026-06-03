// Package document menyediakan document upload service untuk BLIPS IFRS9.
//
// Alur upload:
//  1. HTTP handler menerima multipart/form-data.
//  2. Antivirus scan via ClamAV (stub Phase 1: set PENDING/SKIPPED).
//  3. Stream upload ke MinIO (raw/{system}/{yyyy/mm/dd}/{filename}).
//  4. Hitung SHA-256 hash saat streaming, verifikasi integritas.
//  5. Simpan metadata ke doc.document dalam transaksi yang sama dengan audit log.
//
// Path traversal guard: object_key di-sanitize sebelum dikirim ke MinIO.
// Presigned download URL TTL: 1 jam (konfigurasi DOCUMENT_PRESIGN_TTL_MINUTES).
//
// ClamAV: stub Phase 1. virus_scan_status = 'PENDING'. Worker async di-enqueue
// untuk scan background. File yang INFECTED akan di-quarantine ke bucket terpisah.
//
// Idempotency: di-handle oleh middleware di level HTTP.
// Audit: write via audit.TxWriter dalam transaksi yang sama dengan INSERT doc.document.
package document

import (
	"time"

	"github.com/google/uuid"
)

// Document adalah model domain doc.document.
type Document struct {
	ID                  uuid.UUID
	DocRefKode          string
	Bucket              string
	ObjectKey           string
	FilenameOriginal    string
	MimeType            string
	FileSizeBytes       int64
	SHA256Hash          string // hex-encoded
	VirusScanStatus     VirusScanStatus
	VirusScanAt         *time.Time
	VirusScanEngine     *string
	EntityType          string
	EntityID            uuid.UUID
	DocumentCategory    DocumentCategory
	DocumentDescription *string
	Status              DocumentStatus
	CreatedAt           time.Time
	CreatedBy           uuid.UUID
	UpdatedAt           time.Time
	UpdatedBy           uuid.UUID
	DeletedAt           *time.Time
	DeletedBy           *uuid.UUID
	RowVersion          int64
	TenantID            string
}

// VirusScanStatus adalah status scan antivirus.
type VirusScanStatus string

const (
	// VirusScanPending scan belum dilakukan (Phase 1 stub default).
	VirusScanPending VirusScanStatus = "PENDING"
	// VirusScanClean file bersih (dari ClamAV).
	VirusScanClean VirusScanStatus = "CLEAN"
	// VirusScanInfected file terinfeksi — masuk quarantine bucket.
	VirusScanInfected VirusScanStatus = "INFECTED"
	// VirusScanError scan error (ClamAV down, dll).
	VirusScanScanError VirusScanStatus = "SCAN_ERROR"
)

// DocumentStatus adalah status lifecycle dokumen.
type DocumentStatus string

const (
	// DocumentStatusActive dokumen aktif.
	DocumentStatusActive DocumentStatus = "ACTIVE"
	// DocumentStatusSuperseded dokumen sudah digantikan versi baru.
	DocumentStatusSuperseded DocumentStatus = "SUPERSEDED"
	// DocumentStatusDeleted dokumen soft-deleted.
	DocumentStatusDeleted DocumentStatus = "DELETED"
)

// DocumentCategory adalah kategori dokumen sesuai constraint di migration 0006.
type DocumentCategory string

const (
	DocCategoryBuktiTransaksi  DocumentCategory = "BUKTI_TRANSAKSI"
	DocCategorySPPIWorksheet   DocumentCategory = "SPPI_WORKSHEET"
	DocCategoryBMAssessment    DocumentCategory = "BM_ASSESSMENT"
	DocCategoryECLParameter    DocumentCategory = "ECL_PARAMETER"
	DocCategoryEIRAmortisasi   DocumentCategory = "EIR_AMORTISASI"
	DocCategoryRatingReport    DocumentCategory = "RATING_REPORT"
	DocCategoryKontrak         DocumentCategory = "KONTRAK"
	DocCategoryKonfirmasiDeal  DocumentCategory = "KONFIRMASI_DEAL"
	DocCategoryRekapLaporan    DocumentCategory = "REKAP_LAPORAN"
	DocCategoryLainLain        DocumentCategory = "LAIN_LAIN"
)

// DefaultBucket adalah bucket MinIO default untuk dokumen BLIPS.
const DefaultBucket = "blips-documents"

// QuarantineBucket adalah bucket untuk file yang terinfeksi virus.
const QuarantineBucket = "blips-quarantine"

// MaxFileSizeBytes adalah batas ukuran file per constraint migration 0006 (100MB).
const MaxFileSizeBytes = 100 * 1024 * 1024

// UploadInput adalah input untuk upload satu dokumen.
type UploadInput struct {
	// Filename adalah nama file asli dari client (tanpa path traversal).
	Filename string
	// MimeType dari Content-Type multipart part.
	MimeType string
	// FileSizeBytes adalah ukuran file dalam bytes (dari Content-Length atau dihitung).
	FileSizeBytes int64
	// EntityType adalah tipe entitas yang dilampiri dokumen ini, mis. "mst.instrumen".
	EntityType string
	// EntityID adalah UUID entitas.
	EntityID uuid.UUID
	// Category adalah kategori dokumen.
	Category DocumentCategory
	// Description adalah deskripsi opsional.
	Description *string
	// UploadedByUserID adalah UUID user yang mengupload.
	UploadedByUserID uuid.UUID
	// TenantID default "TUGURE".
	TenantID string
}

// UploadResult adalah hasil sukses upload.
type UploadResult struct {
	DocumentID  uuid.UUID
	DocRefKode  string
	ObjectKey   string
	Bucket      string
	SHA256Hash  string
	VirusScanStatus VirusScanStatus
}

// DownloadURLResult adalah hasil generate presigned download URL.
type DownloadURLResult struct {
	DocumentID  uuid.UUID
	PresignedURL string
	ExpiresAt   time.Time
}
