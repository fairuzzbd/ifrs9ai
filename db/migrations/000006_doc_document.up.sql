-- migration: 0006 doc_document
-- author: data-modeler
-- requires: 0001, 0004
-- description: doc.document — MinIO-backed document metadata table per Phase 2 deliverable.
--              0001 has doc.upload (simple uploader-centric table) and doc.link + doc.access_log.
--              doc.document is the richer, workflow-aware metadata record that references
--              the MinIO object path, SHA-256 verified content hash, virus scan status,
--              associated entity, and full audit trail.
--              RELATIONSHIP: doc.document.storage_ref_id may optionally link to doc.upload.id
--              for compatibility, or operate independently for non-batch uploads.

BEGIN;

CREATE TABLE doc.document (
    -- Identity
    id                      UUID        PRIMARY KEY DEFAULT uuidv7(),
    doc_ref_kode            VARCHAR(30) NOT NULL,           -- human-readable: DOC-YYYYMMDD-NNNNN

    -- Storage (MinIO)
    bucket                  TEXT        NOT NULL DEFAULT 'blips-documents',
    object_key              TEXT        NOT NULL,           -- e.g. documents/2026/06/02/{uuid}.pdf
    storage_ref_id          UUID        REFERENCES doc.upload(id),  -- optional link to doc.upload

    -- File metadata
    filename_original       TEXT        NOT NULL,
    mime_type               TEXT        NOT NULL,
    file_size_bytes         BIGINT      NOT NULL
                                CONSTRAINT ck_doc_file_size CHECK (file_size_bytes > 0 AND file_size_bytes <= 104857600),  -- max 100MB
    sha256_hash             CHAR(64)    NOT NULL,           -- hex-encoded SHA-256 of file content

    -- Virus scan
    virus_scan_status       TEXT        NOT NULL DEFAULT 'PENDING',
                                                            -- PENDING|CLEAN|INFECTED|SCAN_ERROR
    virus_scan_at           TIMESTAMPTZ,
    virus_scan_engine       TEXT,                           -- e.g. ClamAV_0.103.0 (stub Phase 1)

    -- Entity association
    entity_type             TEXT        NOT NULL,           -- e.g. mst.instrumen, trx.penempatan
    entity_id               UUID        NOT NULL,
    document_category       TEXT        NOT NULL,           -- e.g. BUKTI_TRANSAKSI, SPPI_WORKSHEET, ECL_PARAMETER
    document_description    TEXT,

    -- Status
    status                  TEXT        NOT NULL DEFAULT 'ACTIVE',
                                                            -- ACTIVE|SUPERSEDED|DELETED

    -- Audit fields (db-conventions.md wajib)
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by              UUID        NOT NULL REFERENCES sec.user(id),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by              UUID        NOT NULL REFERENCES sec.user(id),
    deleted_at              TIMESTAMPTZ,
    deleted_by              UUID        REFERENCES sec.user(id),
    row_version             BIGINT      NOT NULL DEFAULT 1,
    tenant_id               TEXT        NOT NULL DEFAULT 'TUGURE',

    CONSTRAINT uq_doc_ref_kode      UNIQUE (doc_ref_kode),
    CONSTRAINT uq_doc_object_key    UNIQUE (bucket, object_key),
    CONSTRAINT ck_doc_virus_status  CHECK (virus_scan_status IN ('PENDING','CLEAN','INFECTED','SCAN_ERROR')),
    CONSTRAINT ck_doc_status        CHECK (status IN ('ACTIVE','SUPERSEDED','DELETED')),
    CONSTRAINT ck_doc_category      CHECK (document_category IN (
        'BUKTI_TRANSAKSI','SPPI_WORKSHEET','BM_ASSESSMENT',
        'ECL_PARAMETER','EIR_AMORTISASI','RATING_REPORT',
        'KONTRAK','KONFIRMASI_DEAL','REKAP_LAPORAN','LAIN_LAIN'
    ))
);

-- Indexes
CREATE INDEX ix_doc_entity         ON doc.document(entity_type, entity_id) WHERE deleted_at IS NULL;
CREATE INDEX ix_doc_created_by     ON doc.document(created_by, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX ix_doc_sha256         ON doc.document(sha256_hash);
CREATE INDEX ix_doc_virus_pending  ON doc.document(virus_scan_status) WHERE virus_scan_status IN ('PENDING','INFECTED');
CREATE INDEX ix_doc_status         ON doc.document(status) WHERE status = 'ACTIVE';
CREATE INDEX ix_doc_tenant_time    ON doc.document(tenant_id, created_at DESC);
CREATE INDEX ix_doc_category       ON doc.document(document_category, entity_type) WHERE deleted_at IS NULL;

-- Auto-update triggers
CREATE TRIGGER tg_doc_document_updated_at
    BEFORE UPDATE ON doc.document
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER tg_doc_document_row_version
    BEFORE UPDATE ON doc.document
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

COMMENT ON TABLE doc.document IS
    'MinIO-backed document metadata. Stores SHA-256 verified file content hash, '
    'ClamAV virus scan status (stub Phase 1), and entity association. '
    'Soft-delete only (deleted_at). object_key is the MinIO object path. '
    'bucket + object_key is unique. SHA-256 verified at upload by integration layer.';

COMMIT;
