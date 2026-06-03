package document

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinIOConfig adalah konfigurasi koneksi MinIO.
// Nilai dari environment — TIDAK pernah hardcode.
type MinIOConfig struct {
	Endpoint        string // MINIO_ENDPOINT mis. "localhost:9000"
	AccessKeyID     string // MINIO_ACCESS_KEY (dari Vault di prod)
	SecretAccessKey string // MINIO_SECRET_KEY (dari Vault di prod)
	UseSSL          bool   // MINIO_USE_SSL default false (dev), true prod
	// PresignTTLMinutes adalah TTL presigned download URL, default 60 menit.
	PresignTTLMinutes int
}

// DefaultPresignTTLMinutes adalah TTL default presigned URL: 60 menit.
const DefaultPresignTTLMinutes = 60

// MinIOClient adalah wrapper di atas minio.Client dengan konvensi BLIPS.
type MinIOClient struct {
	client *minio.Client
	cfg    MinIOConfig
	logger *slog.Logger
}

// NewMinIOClient membuat MinIOClient baru.
// Mengembalikan error jika koneksi gagal.
func NewMinIOClient(cfg MinIOConfig, logger *slog.Logger) (*MinIOClient, error) {
	if logger == nil {
		logger = slog.Default()
	}

	if cfg.PresignTTLMinutes <= 0 {
		cfg.PresignTTLMinutes = DefaultPresignTTLMinutes
	}

	mc, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio: init client endpoint=%s: %w", cfg.Endpoint, err)
	}

	return &MinIOClient{
		client: mc,
		cfg:    cfg,
		logger: logger,
	}, nil
}

// EnsureBucket memastikan bucket ada; membuat jika belum ada.
// Dipanggil saat startup service.
func (m *MinIOClient) EnsureBucket(ctx context.Context, bucket string) error {
	exists, err := m.client.BucketExists(ctx, bucket)
	if err != nil {
		return fmt.Errorf("minio: check bucket %q: %w", bucket, err)
	}
	if !exists {
		if err := m.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("minio: create bucket %q: %w", bucket, err)
		}
		m.logger.Info("minio: bucket created", "bucket", bucket)
	}
	return nil
}

// UploadOptions adalah opsi untuk PutObject.
type UploadOptions struct {
	Bucket      string
	ObjectKey   string
	Reader      io.Reader
	ObjectSize  int64 // -1 jika tidak diketahui (streaming)
	ContentType string
	// SHA256HashHex dari file yang sudah dihitung oleh caller (untuk log verifikasi).
	// MinIO mendukung checksum via metadata tapi tidak wajib di Phase 1.
	SHA256HashHex string
}

// Upload mengupload file ke MinIO.
// Reader HARUS sudah divalidasi path traversal oleh caller sebelum pemanggilan ini.
func (m *MinIOClient) Upload(ctx context.Context, opts UploadOptions) (minio.UploadInfo, error) {
	if err := ValidateObjectKey(opts.ObjectKey); err != nil {
		return minio.UploadInfo{}, fmt.Errorf("minio: upload: %w", err)
	}

	putOpts := minio.PutObjectOptions{
		ContentType: opts.ContentType,
	}
	if opts.SHA256HashHex != "" {
		putOpts.UserMetadata = map[string]string{
			"X-Blips-Sha256": opts.SHA256HashHex,
		}
	}

	info, err := m.client.PutObject(ctx, opts.Bucket, opts.ObjectKey, opts.Reader, opts.ObjectSize, putOpts)
	if err != nil {
		return minio.UploadInfo{}, fmt.Errorf("minio: put object bucket=%s key=%s: %w", opts.Bucket, opts.ObjectKey, err)
	}

	m.logger.InfoContext(ctx, "minio: object uploaded",
		"bucket", opts.Bucket,
		"key", opts.ObjectKey,
		"size", info.Size,
		"sha256", opts.SHA256HashHex,
	)
	return info, nil
}

// PresignedGetURL menghasilkan presigned URL untuk download.
// TTL dari MinIOConfig.PresignTTLMinutes.
//
// Security note untuk security-engineer:
// - URL ditandatangani dengan HMAC-SHA256 (AWS Signature V4).
// - TTL default 60 menit — cukup untuk satu sesi download.
// - URL tidak perlu auth header (presigned sudah mengandung credentials).
// - Log URL di slog DEBUG (bukan INFO/ERROR) agar tidak bocor ke log aggregation default.
// - Tidak boleh di-cache di client selain TTL yang diberikan.
func (m *MinIOClient) PresignedGetURL(ctx context.Context, bucket, objectKey string) (string, time.Time, error) {
	if err := ValidateObjectKey(objectKey); err != nil {
		return "", time.Time{}, fmt.Errorf("minio: presign: %w", err)
	}

	ttl := time.Duration(m.cfg.PresignTTLMinutes) * time.Minute
	expiresAt := time.Now().Add(ttl)

	u, err := m.client.PresignedGetObject(ctx, bucket, objectKey, ttl, nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("minio: presign url bucket=%s key=%s: %w", bucket, objectKey, err)
	}

	m.logger.DebugContext(ctx, "minio: presigned URL generated",
		"bucket", bucket,
		"key", objectKey,
		"ttl_min", m.cfg.PresignTTLMinutes,
		// SENGAJA tidak log URL penuh di INFO — security: URL mengandung credentials.
	)

	return u.String(), expiresAt, nil
}

// Ping melakukan koneksi ringan ke MinIO untuk verifikasi availability.
// Dipakai oleh /readyz health check — mengembalikan nil jika MinIO dapat di-reach.
// Pakai context dengan deadline pendek (mis. 3 detik) agar tidak hang.
func (m *MinIOClient) Ping(ctx context.Context) error {
	_, err := m.client.ListBuckets(ctx)
	if err != nil {
		return fmt.Errorf("minio: ping failed endpoint=%s: %w", m.cfg.Endpoint, err)
	}
	return nil
}

// CopyToQuarantine memindahkan file terinfeksi ke quarantine bucket.
// Dipanggil oleh virus scan worker ketika file terdeteksi INFECTED.
func (m *MinIOClient) CopyToQuarantine(ctx context.Context, srcBucket, srcKey string) error {
	dstKey := fmt.Sprintf("quarantine/%s/%s", srcBucket, srcKey)
	if err := ValidateObjectKey(dstKey); err != nil {
		return fmt.Errorf("minio: quarantine key invalid: %w", err)
	}

	src := minio.CopySrcOptions{Bucket: srcBucket, Object: srcKey}
	dst := minio.CopyDestOptions{Bucket: QuarantineBucket, Object: dstKey}

	_, err := m.client.CopyObject(ctx, dst, src)
	if err != nil {
		return fmt.Errorf("minio: copy to quarantine src=%s/%s: %w", srcBucket, srcKey, err)
	}

	// Remove dari bucket asli setelah copy berhasil.
	if err := m.client.RemoveObject(ctx, srcBucket, srcKey, minio.RemoveObjectOptions{}); err != nil {
		m.logger.WarnContext(ctx, "minio: quarantine copy ok tapi remove from src gagal",
			"bucket", srcBucket, "key", srcKey, "error", err)
	}

	m.logger.WarnContext(ctx, "minio: file quarantined (INFECTED)",
		"src_bucket", srcBucket,
		"src_key", srcKey,
		"quarantine_key", dstKey,
	)
	return nil
}
