package document

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Repository adalah interface untuk akses data doc.document.
type Repository interface {
	// Insert menyimpan metadata dokumen dalam transaksi.
	Insert(ctx context.Context, tx *sql.Tx, doc *Document) error
	// GetByID mengambil dokumen berdasarkan ID.
	GetByID(ctx context.Context, id uuid.UUID) (*Document, error)
	// SoftDelete soft-deletes dokumen.
	SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) error
}

// DBRepository mengimplementasikan Repository dengan PostgreSQL.
type DBRepository struct {
	db *sql.DB
}

// NewDBRepository membuat DBRepository.
func NewDBRepository(db *sql.DB) *DBRepository {
	return &DBRepository{db: db}
}

// Insert menyimpan metadata dokumen ke doc.document dalam transaksi.
// Dipanggil bersama audit.TxWriter.Write dalam transaksi yang sama.
func (r *DBRepository) Insert(ctx context.Context, tx *sql.Tx, doc *Document) error {
	if tx == nil {
		return fmt.Errorf("document repo: Insert membutuhkan transaksi aktif (tx tidak boleh nil)")
	}

	_, err := tx.ExecContext(ctx, `
		INSERT INTO doc.document (
			id, doc_ref_kode, bucket, object_key,
			filename_original, mime_type, file_size_bytes, sha256_hash,
			virus_scan_status, virus_scan_at, virus_scan_engine,
			entity_type, entity_id, document_category, document_description,
			status,
			created_at, created_by, updated_at, updated_by,
			row_version, tenant_id
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8,
			$9, $10, $11,
			$12, $13, $14, $15,
			$16,
			$17, $18, $19, $20,
			1, $21
		)`,
		doc.ID, doc.DocRefKode, doc.Bucket, doc.ObjectKey,
		doc.FilenameOriginal, doc.MimeType, doc.FileSizeBytes, doc.SHA256Hash,
		string(doc.VirusScanStatus), doc.VirusScanAt, doc.VirusScanEngine,
		doc.EntityType, doc.EntityID, string(doc.DocumentCategory), doc.DocumentDescription,
		string(doc.Status),
		doc.CreatedAt, doc.CreatedBy, doc.UpdatedAt, doc.UpdatedBy,
		doc.TenantID,
	)
	if err != nil {
		return fmt.Errorf("document repo: insert: %w", err)
	}
	return nil
}

// GetByID mengambil dokumen aktif berdasarkan ID.
func (r *DBRepository) GetByID(ctx context.Context, id uuid.UUID) (*Document, error) {
	if r.db == nil {
		return nil, fmt.Errorf("document repo: database tidak tersedia")
	}

	var doc Document
	var virusScanAt sql.NullTime
	var virusScanEngine sql.NullString
	var description sql.NullString
	var deletedAt sql.NullTime
	var deletedBy uuid.NullUUID

	err := r.db.QueryRowContext(ctx, `
		SELECT
			id, doc_ref_kode, bucket, object_key,
			filename_original, mime_type, file_size_bytes, sha256_hash,
			virus_scan_status, virus_scan_at, virus_scan_engine,
			entity_type, entity_id, document_category, document_description,
			status,
			created_at, created_by, updated_at, updated_by,
			deleted_at, deleted_by,
			row_version, tenant_id
		FROM doc.document
		WHERE id = $1 AND deleted_at IS NULL
	`, id).Scan(
		&doc.ID, &doc.DocRefKode, &doc.Bucket, &doc.ObjectKey,
		&doc.FilenameOriginal, &doc.MimeType, &doc.FileSizeBytes, &doc.SHA256Hash,
		&doc.VirusScanStatus, &virusScanAt, &virusScanEngine,
		&doc.EntityType, &doc.EntityID, &doc.DocumentCategory, &description,
		&doc.Status,
		&doc.CreatedAt, &doc.CreatedBy, &doc.UpdatedAt, &doc.UpdatedBy,
		&deletedAt, &deletedBy,
		&doc.RowVersion, &doc.TenantID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("document repo: get by id: %w", err)
	}

	if virusScanAt.Valid {
		doc.VirusScanAt = &virusScanAt.Time
	}
	if virusScanEngine.Valid {
		doc.VirusScanEngine = &virusScanEngine.String
	}
	if description.Valid {
		doc.DocumentDescription = &description.String
	}
	if deletedAt.Valid {
		doc.DeletedAt = &deletedAt.Time
	}
	if deletedBy.Valid {
		doc.DeletedBy = &deletedBy.UUID
	}

	return &doc, nil
}

// SoftDelete soft-deletes dokumen.
func (r *DBRepository) SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) error {
	execCtx := func(query string, args ...any) error {
		if tx != nil {
			_, err := tx.ExecContext(ctx, query, args...)
			return err
		}
		_, err := r.db.ExecContext(ctx, query, args...)
		return err
	}

	err := execCtx(`
		UPDATE doc.document
		SET deleted_at = $1, deleted_by = $2, status = 'DELETED', updated_at = $1, updated_by = $2
		WHERE id = $3 AND deleted_at IS NULL
	`, time.Now(), deletedBy, id)
	if err != nil {
		return fmt.Errorf("document repo: soft delete: %w", err)
	}
	return nil
}

// BeginTx membuka transaksi baru.
func (r *DBRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	if r.db == nil {
		return nil, fmt.Errorf("document repo: database tidak tersedia")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("document repo: begin tx: %w", err)
	}
	return tx, nil
}
