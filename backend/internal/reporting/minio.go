package reporting

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinIOConfig holds MinIO client configuration.
type MinIOConfig struct {
	Endpoint        string // e.g. "minio:9000"
	AccessKeyID     string // from env MINIO_ACCESS_KEY
	SecretAccessKey string // from env MINIO_SECRET_KEY (never log)
	UseSSL          bool
	BucketExports   string // default "exports"
}

// MinIOClient wraps minio-go for BLIPS async export uploads.
type MinIOClient struct {
	client *minio.Client
	cfg    MinIOConfig
	logger *slog.Logger
}

// NewMinIOClient creates a MinIOClient.
func NewMinIOClient(cfg MinIOConfig, logger *slog.Logger) (*MinIOClient, error) {
	if logger == nil {
		logger = slog.Default()
	}
	c, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("NewMinIOClient: %w", err)
	}
	if cfg.BucketExports == "" {
		cfg.BucketExports = "exports"
	}
	return &MinIOClient{client: c, cfg: cfg, logger: logger}, nil
}

// UploadExport uploads fileBytes to MinIO at objectName.
// ObjectName format: {tenant}/{userID}/{yyyy}/{mm}/{dd}/{jobID}.{format}
// Returns the object name on success.
func (m *MinIOClient) UploadExport(ctx context.Context, objectName string, fileBytes []byte, contentType string) error {
	reader := bytes.NewReader(fileBytes)
	_, err := m.client.PutObject(ctx, m.cfg.BucketExports, objectName, reader,
		int64(len(fileBytes)), minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("minio.UploadExport: put %s: %w", objectName, err)
	}
	m.logger.InfoContext(ctx, "minio: object uploaded",
		"bucket", m.cfg.BucketExports, "object", objectName, "bytes", len(fileBytes))
	return nil
}

// PresignedGetURL returns a presigned GET URL for objectName with the given TTL.
// S4-AC1: signed URL TTL = REPORT_EXPORT_MINIO_TTL_HOURS (default 24h).
func (m *MinIOClient) PresignedGetURL(ctx context.Context, objectName string, ttl time.Duration) (string, error) {
	reqParams := make(url.Values)
	presignedURL, err := m.client.PresignedGetObject(ctx, m.cfg.BucketExports, objectName, ttl, reqParams)
	if err != nil {
		return "", fmt.Errorf("minio.PresignedGetURL: %w", err)
	}
	return presignedURL.String(), nil
}

// ExportObjectName builds the MinIO object path for an export.
// Pattern: {tenantID}/{userID}/{yyyy}/{mm}/{dd}/{jobID}.{format}
func ExportObjectName(tenantID, userID, jobID, format string, exportedAt time.Time) string {
	return fmt.Sprintf("%s/%s/%d/%02d/%02d/%s.%s",
		tenantID, userID,
		exportedAt.Year(), exportedAt.Month(), exportedAt.Day(),
		jobID, format,
	)
}

// ContentTypeFor returns the HTTP content type for an export format.
func ContentTypeFor(format ExportFormat) string {
	switch format {
	case FormatXLSX:
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case FormatCSV:
		return "text/csv; charset=UTF-8"
	case FormatPDF:
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}
