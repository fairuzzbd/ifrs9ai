package document

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ServiceConfig adalah konfigurasi opsional untuk document Service.
type ServiceConfig struct {
	// BlockPendingDownload menentukan apakah download dokumen yang masih berstatus
	// VirusScanPending diblokir. Default true (aman). Set false hanya untuk dev/testing.
	// Sesuai Decision-B / MEDIUM-1 security mandate.
	BlockPendingDownload bool
}

// DefaultServiceConfig mengembalikan config aman untuk production.
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{BlockPendingDownload: true}
}

// Service adalah document upload/download service.
// Satu transaksi per upload: MinIO upload + DB insert + audit log.
type Service struct {
	repo        *DBRepository
	minio       *MinIOClient
	auditWriter *audit.Writer
	logger      *slog.Logger
	cfg         ServiceConfig
}

// NewService membuat Service.
// minio boleh nil (testing mode — skip MinIO upload).
func NewService(
	repo *DBRepository,
	minio *MinIOClient,
	auditWriter *audit.Writer,
	logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		repo:        repo,
		minio:       minio,
		auditWriter: auditWriter,
		logger:      logger,
		cfg:         DefaultServiceConfig(),
	}
}

// WithConfig mengembalikan Service dengan ServiceConfig yang diberikan.
// Pakai ini di main.go untuk wiring config dari environment.
func (s *Service) WithConfig(cfg ServiceConfig) *Service {
	s.cfg = cfg
	return s
}

// Upload mengupload satu dokumen ke MinIO, verifikasi SHA-256, dan simpan metadata.
//
// Alur:
//  1. Validasi input (mime type, size, entity, category).
//  2. Path traversal guard di object key.
//  3. Virus scan STUB — set VirusScanPending (Phase 1).
//  4. Stream ke MinIO sambil hitung SHA-256.
//  5. Mulai DB transaction.
//  6. Insert doc.document + audit.Write (same tx).
//  7. Commit.
//
// Jika MinIO client nil (testing), skip step 4.
func (s *Service) Upload(ctx context.Context, in UploadInput, reader io.Reader) (*UploadResult, error) {
	if err := validateUploadInput(in); err != nil {
		return nil, err
	}

	now := time.Now()
	docID := uuid.New()
	objectKey := BuildObjectKey(docID, in.Filename, now)
	docRefKode := GenerateDocRefKode(docID, now)

	// Double-check: path traversal guard.
	if err := ValidateObjectKey(objectKey); err != nil {
		return nil, domainerrors.Wrap(domainerrors.CodeValidationFailed,
			"Object key tidak valid setelah sanitasi.", err)
	}

	tenantID := in.TenantID
	if tenantID == "" {
		tenantID = "TUGURE"
	}

	// Virus scan stub — Phase 1.
	// Phase 2: gRPC call ke ClamAV sebelum upload ke MinIO.
	// Dokumentasi: file.VirusScanStatus = PENDING berarti scan belum dilakukan.
	// Worker async akan men-scan dan update ke CLEAN/INFECTED.
	// Jika INFECTED: quarantine ke QuarantineBucket, update status, alert ROLE-IT-ADMIN.
	virusScanStatus := VirusScanPending
	s.logger.DebugContext(ctx, "document: virus scan STUB (Phase 1) — status PENDING",
		"docId", docID,
		"filename", in.Filename,
	)

	// Stream ke MinIO sambil hitung SHA-256.
	var sha256Hash string
	var actualSize int64

	if s.minio != nil {
		hr := NewHashReader(reader)
		uploadOpts := UploadOptions{
			Bucket:      DefaultBucket,
			ObjectKey:   objectKey,
			Reader:      hr,
			ObjectSize:  in.FileSizeBytes,
			ContentType: in.MimeType,
		}
		if _, err := s.minio.Upload(ctx, uploadOpts); err != nil {
			return nil, domainerrors.Wrap(domainerrors.CodeInternal,
				"Upload ke MinIO gagal.", err)
		}
		sha256Hash = hr.SHA256Hex()
		actualSize = hr.BytesRead()
		s.logger.DebugContext(ctx, "document: uploaded to MinIO",
			"key", objectKey,
			"sha256", sha256Hash,
			"size", actualSize,
		)
	} else {
		// Testing mode: hitung hash dari reader tanpa upload.
		var err error
		sha256Hash, actualSize, err = ComputeSHA256Hex(reader)
		if err != nil {
			return nil, domainerrors.Wrap(domainerrors.CodeInternal, "Gagal compute SHA-256.", err)
		}
	}

	// Validasi size yang actual vs yang diklaim.
	if in.FileSizeBytes > 0 && actualSize != in.FileSizeBytes {
		s.logger.WarnContext(ctx, "document: actual size berbeda dari claimed",
			"claimed", in.FileSizeBytes, "actual", actualSize)
		// Non-fatal: simpan actual size.
	}

	doc := &Document{
		ID:                  docID,
		DocRefKode:          docRefKode,
		Bucket:              DefaultBucket,
		ObjectKey:           objectKey,
		FilenameOriginal:    sanitizeFilename(in.Filename),
		MimeType:            in.MimeType,
		FileSizeBytes:       actualSize,
		SHA256Hash:          sha256Hash,
		VirusScanStatus:     virusScanStatus,
		EntityType:          in.EntityType,
		EntityID:            in.EntityID,
		Category:            in.Category,
		DocumentDescription: in.Description,
		Status:              DocumentStatusActive,
		CreatedAt:           now,
		CreatedBy:           in.UploadedByUserID,
		UpdatedAt:           now,
		UpdatedBy:           in.UploadedByUserID,
		RowVersion:          1,
		TenantID:            tenantID,
	}

	// DB transaction: insert + audit.
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, domainerrors.Wrap(domainerrors.CodeInternal, "Gagal membuka transaksi DB.", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				slog.Default().ErrorContext(ctx, "document service: tx rollback failed", "error", rbErr)
			}
		}
	}()

	if err = s.repo.Insert(ctx, tx, doc); err != nil {
		return nil, domainerrors.Wrap(domainerrors.CodeInternal, "Gagal menyimpan metadata dokumen.", err)
	}

	if s.auditWriter != nil {
		txAudit := s.auditWriter.WithTx(tx)
		auditErr := txAudit.Write(ctx, audit.Event{
			Action:      "DOCUMENT.UPLOAD",
			EntityType:  "doc.document",
			EntityID:    docID,
			ActorUserID: in.UploadedByUserID.String(),
			After: map[string]any{
				"id":                docID.String(),
				"doc_ref_kode":      docRefKode,
				"filename":          doc.FilenameOriginal,
				"sha256_hash":       sha256Hash,
				"entity_type":       in.EntityType,
				"entity_id":         in.EntityID.String(),
				"category":          string(in.Category),
				"virus_scan_status": string(virusScanStatus),
				"size_bytes":        actualSize,
			},
		})
		if auditErr != nil {
			s.logger.WarnContext(ctx, "document: audit write gagal (non-fatal)",
				"error", auditErr, "docId", docID)
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, domainerrors.Wrap(domainerrors.CodeInternal, "Gagal commit transaksi.", err)
	}

	s.logger.InfoContext(ctx, "document: upload berhasil",
		"docId", docID,
		"docRefKode", docRefKode,
		"filename", in.Filename,
		"sha256", sha256Hash,
		"entityType", in.EntityType,
		"entityId", in.EntityID,
	)

	return &UploadResult{
		DocumentID:      docID,
		DocRefKode:      docRefKode,
		ObjectKey:       objectKey,
		Bucket:          DefaultBucket,
		SHA256Hash:      sha256Hash,
		VirusScanStatus: virusScanStatus,
	}, nil
}

// GetPresignedDownloadURL menghasilkan presigned URL untuk download dokumen.
// Memverifikasi bahwa dokumen ada dan aktif sebelum generate URL.
//
// Security:
// - URL presigned TTL 60 menit (konfigurabel via DOCUMENT_PRESIGN_TTL_MINUTES).
// - Caller harus memiliki permission "document.read".
// - URL tidak di-cache di response (client harus minta ulang setelah TTL).
func (s *Service) GetPresignedDownloadURL(ctx context.Context, docID uuid.UUID) (*DownloadURLResult, error) {
	doc, err := s.repo.GetByID(ctx, docID)
	if err != nil {
		return nil, domainerrors.Wrap(domainerrors.CodeInternal, "Gagal membaca metadata dokumen.", err)
	}
	if doc == nil {
		return nil, domainerrors.ErrNotFound("Dokumen")
	}
	if doc.Status != DocumentStatusActive {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("Dokumen %s tidak aktif (status: %s).", docID, doc.Status))
	}
	if doc.VirusScanStatus == VirusScanInfected {
		// Dokumen yang terinfeksi tidak boleh di-download.
		return nil, domainerrors.New(domainerrors.CodeForbidden,
			"Dokumen tidak dapat diunduh: file terdeteksi terinfeksi virus.")
	}
	// Decision-B / MEDIUM-1: blokir download dokumen yang belum selesai dipindai virus.
	// Dikontrol via DOCUMENT_BLOCK_PENDING_DOWNLOAD (default true).
	if doc.VirusScanStatus == VirusScanPending && s.cfg.BlockPendingDownload {
		return nil, domainerrors.New(domainerrors.CodeForbidden,
			"Dokumen belum selesai dipindai virus. Coba lagi dalam beberapa menit.")
	}

	if s.minio == nil {
		return nil, domainerrors.New(domainerrors.CodeInternal,
			"MinIO client tidak tersedia.")
	}

	presignedURL, expiresAt, err := s.minio.PresignedGetURL(ctx, doc.Bucket, doc.ObjectKey)
	if err != nil {
		return nil, domainerrors.Wrap(domainerrors.CodeInternal, "Gagal generate presigned URL.", err)
	}

	return &DownloadURLResult{
		DocumentID:   docID,
		PresignedURL: presignedURL,
		ExpiresAt:    expiresAt,
	}, nil
}

// validateUploadInput memvalidasi UploadInput.
func validateUploadInput(in UploadInput) error {
	var details []domainerrors.Detail

	if strings.TrimSpace(in.Filename) == "" {
		details = append(details, domainerrors.Detail{
			Field: "filename", Rule: "required", Message: "Nama file wajib diisi",
		})
	}
	if strings.TrimSpace(in.MimeType) == "" {
		details = append(details, domainerrors.Detail{
			Field: "mimeType", Rule: "required", Message: "MIME type wajib diisi",
		})
	}
	if in.FileSizeBytes > MaxFileSizeBytes {
		details = append(details, domainerrors.Detail{
			Field:   "file",
			Rule:    "max_size",
			Message: fmt.Sprintf("Ukuran file melebihi batas 100MB (received %d bytes)", in.FileSizeBytes),
		})
	}
	if strings.TrimSpace(in.EntityType) == "" {
		details = append(details, domainerrors.Detail{
			Field: "entityType", Rule: "required", Message: "Entity type wajib diisi",
		})
	}
	if in.EntityID == uuid.Nil {
		details = append(details, domainerrors.Detail{
			Field: "entityId", Rule: "required", Message: "Entity ID wajib diisi",
		})
	}
	if !isValidCategory(in.Category) {
		details = append(details, domainerrors.Detail{
			Field:   "category",
			Rule:    "enum",
			Message: "Document category tidak valid",
		})
	}
	if in.UploadedByUserID == uuid.Nil {
		details = append(details, domainerrors.Detail{
			Field: "uploadedByUserId", Rule: "required", Message: "User ID uploader wajib diisi",
		})
	}

	if len(details) > 0 {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			"Input upload dokumen tidak valid.", details...)
	}
	return nil
}

// isValidCategory memvalidasi Category.
func isValidCategory(cat Category) bool {
	switch cat {
	case DocCategoryBuktiTransaksi, DocCategorySPPIWorksheet, DocCategoryBMAssessment,
		DocCategoryECLParameter, DocCategoryEIRAmortisasi, DocCategoryRatingReport,
		DocCategoryKontrak, DocCategoryKonfirmasiDeal, DocCategoryRekapLaporan,
		DocCategoryLainLain:
		return true
	}
	return false
}

// sanitizeFilename membersihkan nama file untuk disimpan di DB.
// Hanya basename (tanpa path). Tidak boleh ada "/" atau ".." untuk safety.
func sanitizeFilename(name string) string {
	// Ambil hanya basename.
	for strings.Contains(name, "/") {
		idx := strings.LastIndex(name, "/")
		name = name[idx+1:]
	}
	for strings.Contains(name, "\\") {
		idx := strings.LastIndex(name, "\\")
		name = name[idx+1:]
	}
	// Hapus ".." yang mungkin tersisa.
	name = strings.ReplaceAll(name, "..", "")
	return name
}
