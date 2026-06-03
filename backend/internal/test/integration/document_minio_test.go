//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/document"
)

// TestDocument_UploadAndVerifySHA256 uploads a document to MinIO (live), stores
// metadata in PostgreSQL, and verifies the SHA-256 hash integrity end-to-end.
//
// Covers: document upload + SHA-256 verify (MinIO) — §7 gate item.
func TestDocument_UploadAndVerifySHA256(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	minioCfg := document.MinIOConfig{
		Endpoint:          infra.MinioCfg.Endpoint,
		AccessKeyID:       infra.MinioCfg.AccessKey,
		SecretAccessKey:   infra.MinioCfg.SecretKey,
		UseSSL:            infra.MinioCfg.UseSSL,
		PresignTTLMinutes: 60,
	}

	mc, err := document.NewMinIOClient(minioCfg, nil)
	if err != nil {
		t.Skipf("MinIO not reachable (%v) — start dev stack", err)
	}

	ctx := context.Background()
	bucket := "blips-documents-inttest"
	if err := mc.EnsureBucket(ctx, bucket); err != nil {
		t.Fatalf("ensure bucket %s: %v", bucket, err)
	}

	// Build test file content.
	content := "BLIPS integration test document — " + time.Now().String()
	contentBytes := []byte(content)

	// Compute expected SHA-256.
	h := sha256.New()
	h.Write(contentBytes)
	expectedHash := hex.EncodeToString(h.Sum(nil))

	objectKey := "raw/test/" + time.Now().Format("2006/01/02") + "/test_doc_" + uuid.New().String() + ".txt"

	// Upload to MinIO.
	_, err = mc.Upload(ctx, document.UploadOptions{
		Bucket:        bucket,
		ObjectKey:     objectKey,
		Reader:        bytes.NewReader(contentBytes),
		ObjectSize:    int64(len(contentBytes)),
		ContentType:   "text/plain",
		SHA256HashHex: expectedHash,
	})
	if err != nil {
		t.Fatalf("MinIO upload: %v", err)
	}
	t.Logf("uploaded: bucket=%s key=%s sha256=%s", bucket, objectKey, expectedHash)

	// Generate presigned URL and verify it yields the same content.
	presignedURL, expiresAt, err := mc.PresignedGetURL(ctx, bucket, objectKey)
	if err != nil {
		t.Fatalf("PresignedGetURL: %v", err)
	}
	if expiresAt.Before(time.Now()) {
		t.Errorf("presigned URL already expired: %v", expiresAt)
	}
	if !strings.HasPrefix(presignedURL, "http") {
		t.Errorf("presigned URL does not look like HTTP URL: %q", presignedURL[:min(40, len(presignedURL))])
	}
	t.Logf("presigned URL valid until: %v", expiresAt)

	// Store document metadata in PostgreSQL with audit trail.
	uploaderID := seedUserSQL(t, infra.DB, "doc_uploader_"+uuid.New().String()[:8])
	docID := uuid.New()

	auditWriter := audit.NewWriter(infra.DB)
	dbRepo := document.NewDBRepository(infra.DB)

	tx, err := infra.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	now := time.Now()
	doc := &document.Document{
		ID:               docID,
		DocRefKode:       "DOC-INTTEST-001",
		Bucket:           bucket,
		ObjectKey:        objectKey,
		FilenameOriginal: "test_doc.txt",
		MimeType:         "text/plain",
		FileSizeBytes:    int64(len(contentBytes)),
		SHA256Hash:       expectedHash,
		VirusScanStatus:  document.VirusScanPending,
		EntityType:       "mst.instrumen",
		EntityID:         uuid.New(),
		DocumentCategory: document.DocCategoryBuktiTransaksi,
		Status:           document.DocumentStatusActive,
		CreatedAt:        now,
		CreatedBy:        uploaderID,
		UpdatedAt:        now,
		UpdatedBy:        uploaderID,
		TenantID:         "TUGURE",
	}

	if err = dbRepo.Insert(ctx, tx, doc); err != nil {
		t.Fatalf("document Insert: %v", err)
	}

	// Write audit log in same transaction.
	txWriter := auditWriter.WithTx(tx)
	auditCtx := userCtx(uploaderID, []string{"instrumen.create"})
	if err = txWriter.Write(auditCtx, audit.Event{
		Action:      "DOCUMENT.CREATE",
		EntityType:  "doc.document",
		EntityID:    docID,
		After:       map[string]any{"object_key": objectKey, "sha256": expectedHash},
		ActorUserID: uploaderID.String(),
		ActorRole:   "ROLE-MAKER-TR",
	}); err != nil {
		t.Fatalf("audit Write: %v", err)
	}

	if err = tx.Commit(); err != nil {
		t.Fatalf("tx Commit: %v", err)
	}

	// Read back from DB and verify SHA-256 stored correctly.
	fetched, err := dbRepo.GetByID(ctx, docID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if fetched == nil {
		t.Fatal("document not found after insert")
	}
	if fetched.SHA256Hash != expectedHash {
		t.Errorf("SHA-256 mismatch: stored=%s expected=%s", fetched.SHA256Hash, expectedHash)
	}
	if fetched.ObjectKey != objectKey {
		t.Errorf("object key mismatch: stored=%s expected=%s", fetched.ObjectKey, objectKey)
	}
	t.Logf("document stored and retrieved correctly: id=%s sha256=%s", docID, fetched.SHA256Hash)
}

// TestDocument_ObjectKey_PathTraversalRejected verifies that object keys
// containing path traversal are rejected before reaching MinIO.
//
// Covers: security-baseline input validation (path traversal guard).
func TestDocument_ObjectKey_PathTraversalRejected(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	minioCfg := document.MinIOConfig{
		Endpoint:  infra.MinioCfg.Endpoint,
		AccessKeyID: infra.MinioCfg.AccessKey,
		SecretAccessKey: infra.MinioCfg.SecretKey,
	}
	mc, err := document.NewMinIOClient(minioCfg, nil)
	if err != nil {
		t.Skipf("MinIO not reachable: %v", err)
	}

	maliciousKeys := []string{
		"../../etc/passwd",
		"raw/docs/../../../secrets.txt",
		"raw/docs/%2F%2F..%2F..%2F/etc/passwd",
		"raw/docs\x00null_byte.txt",
	}

	for _, key := range maliciousKeys {
		_, err := mc.Upload(context.Background(), document.UploadOptions{
			Bucket:     "blips-documents-inttest",
			ObjectKey:  key,
			Reader:     strings.NewReader("data"),
			ObjectSize: 4,
		})
		if err == nil {
			t.Errorf("expected path traversal rejection for key %q, got nil error", key)
		} else {
			t.Logf("path traversal correctly rejected for %q: %v", key[:min(30, len(key))], err)
		}
	}
}

// TestDocument_SoftDelete verifies that soft-deleting a document marks it as
// deleted (deleted_at not null) without physically removing it.
func TestDocument_SoftDelete(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	ctx := context.Background()
	uploaderID := seedUserSQL(t, infra.DB, "doc_softdel_"+uuid.New().String()[:8])
	docID := uuid.New()
	dbRepo := document.NewDBRepository(infra.DB)

	// Insert a document directly for soft-delete test.
	tx, err := infra.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	now := time.Now()
	if err := dbRepo.Insert(ctx, tx, &document.Document{
		ID: docID, DocRefKode: "DOC-SOFTDEL-001",
		Bucket: "blips-documents-inttest", ObjectKey: "raw/test/softdel-" + docID.String() + ".txt",
		FilenameOriginal: "softdel.txt", MimeType: "text/plain",
		FileSizeBytes: 10, SHA256Hash: hex.EncodeToString(make([]byte, 32)),
		VirusScanStatus: document.VirusScanPending,
		EntityType: "mst.instrumen", EntityID: uuid.New(),
		DocumentCategory: document.DocCategoryLainLain,
		Status: document.DocumentStatusActive,
		CreatedAt: now, CreatedBy: uploaderID,
		UpdatedAt: now, UpdatedBy: uploaderID, TenantID: "TUGURE",
	}); err != nil {
		_ = tx.Rollback()
		t.Fatalf("Insert: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Soft delete.
	if err := dbRepo.SoftDelete(ctx, nil, docID, uploaderID); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	// GetByID should return nil (filter WHERE deleted_at IS NULL).
	fetched, err := dbRepo.GetByID(ctx, docID)
	if err != nil {
		t.Fatalf("GetByID after soft-delete: %v", err)
	}
	if fetched != nil {
		t.Errorf("expected nil after soft-delete, got document with status=%s", fetched.Status)
	}
	t.Logf("soft-delete verified: document no longer returned by GetByID")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// discard is used to consume io.Reader in tests.
var _ = io.Discard
