# Runbook: Notification Service & Document Upload Service

System: BLIPS IFRS9
Owner escalation: ROLE-IT-ADMIN
Last updated: 2026-06-02

---

## 1. Notification Service

### 1.1 Overview

Async notification via Asynq (DEC-007). Two channels: EMAIL (SMTP) and INAPP.
Queue: `notification` in Redis. Task type: `notification:deliver`.
Max retry: 5 with exponential backoff. After max retry: DLQ alert to ROLE-IT-ADMIN.

### 1.2 Configuration

| Env var | Default | Notes |
|---|---|---|
| `SMTP_HOST` | (empty) | Empty = dry-run mode (dev safe, logs only) |
| `SMTP_PORT` | `587` | STARTTLS default |
| `SMTP_USERNAME` | (empty) | From Vault/KMS in prod |
| `SMTP_PASSWORD` | (empty) | From Vault/KMS in prod — NEVER hardcode |
| `SMTP_FROM` | `BLIPS IFRS9 <noreply@blips.tugu-re.com>` | Sender address |
| `SMTP_USE_TLS` | `false` | `true` = TLS on connect (port 465 typical) |
| `REDIS_URL` | `redis://localhost:6379/0` | Asynq queue backend |

### 1.3 Failure: SMTP not delivering

Symptoms: Email notifications not arriving. Dry-run logs appear instead of real sends.

Diagnosis:
```bash
# Check if SMTP_HOST is set
grep SMTP_HOST /etc/blips/env

# Test SMTP connectivity
nc -zv $SMTP_HOST $SMTP_PORT

# Check Asynq queue depth
redis-cli -u $REDIS_URL LLEN asynq:{notification}:tasks:default
```

Resolution:
1. Verify SMTP credentials from Vault: `vault kv get secret/blips/smtp`
2. Set `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD` correctly
3. Restart API service
4. Failed tasks will auto-retry (Asynq exponential backoff)
5. If max retries exhausted, tasks are in DLQ:
   ```bash
   # List DLQ tasks for notification queue
   redis-cli -u $REDIS_URL LRANGE asynq:{notification}:tasks:dead 0 9
   ```
6. After fixing SMTP, re-enqueue DLQ tasks via Asynq UI or CLI

### 1.4 Failure: Asynq worker not consuming tasks

Symptoms: Notifications queued but not delivered. Queue depth grows.

Diagnosis:
```bash
# Check worker process
ps aux | grep blips-worker

# Check Asynq queue stats
redis-cli -u $REDIS_URL LLEN asynq:{notification}:tasks:default
```

Resolution:
1. Ensure worker binary `blips-worker` is running
2. Worker registers handler: `notification.RegisterHandler(mux, worker)`
3. Restart worker if stuck: `systemctl restart blips-worker`

### 1.5 Template not found

Symptoms: Error log `notification template tidak ditemukan: code=WORKFLOW.SUBMITTED channel=EMAIL lang=id-ID`

Resolution:
1. Check `sys.notification_template` table:
   ```sql
   SELECT template_code, channel, language, aktif_flag
   FROM sys.notification_template
   ORDER BY template_code, channel;
   ```
2. If missing, seed from migration 0002 or insert manually
3. InMemoryTemplateStore is used as fallback when DB unavailable (dev mode)

---

## 2. Document Upload Service

### 2.1 Overview

Upload flow:
1. Multipart HTTP POST /api/v1/documents
2. Virus scan STUB (Phase 1: status = PENDING, Phase 2: ClamAV gRPC)
3. Stream to MinIO with SHA-256 computed inline
4. INSERT doc.document + audit.Write in same DB transaction

### 2.2 Configuration

| Env var | Default | Notes |
|---|---|---|
| `MINIO_ENDPOINT` | `localhost:9000` | MinIO S3-compatible endpoint |
| `MINIO_ACCESS_KEY` | `minioadmin` | From Vault/KMS in prod |
| `MINIO_SECRET_KEY` | `minioadmin` | From Vault/KMS in prod — NEVER hardcode |
| `MINIO_USE_SSL` | `false` | `true` for production TLS |
| `DOCUMENT_PRESIGN_TTL_MINUTES` | `60` | Presigned URL TTL |

### 2.3 Failure: MinIO upload fails

Symptoms: `POST /api/v1/documents` returns 500. Log shows `minio: put object bucket=blips-documents key=...`

Diagnosis:
```bash
# Test MinIO connectivity
curl http://$MINIO_ENDPOINT/minio/health/live

# Check bucket exists
mc alias set blips http://$MINIO_ENDPOINT $MINIO_ACCESS_KEY $MINIO_SECRET_KEY
mc ls blips/blips-documents
```

Resolution:
1. Ensure MinIO is running: `systemctl status minio`
2. Create missing bucket: `mc mb blips/blips-documents`
3. Verify credentials in Vault
4. EnsureBucket is called at startup — check startup logs for bucket creation errors

### 2.4 Failure: SHA-256 hash mismatch in logs

Symptoms: Log warning `document: actual size berbeda dari claimed`

This is non-fatal (actual size is stored). But log should be investigated:
1. Check if client is sending wrong `Content-Length`
2. This does not affect data integrity — SHA-256 is computed from actual bytes streamed

### 2.5 Virus scan: PENDING files

Phase 1: all files have `virus_scan_status = PENDING`. Phase 2 will implement ClamAV async scan.

To check pending scans:
```sql
SELECT id, doc_ref_kode, filename_original, created_at
FROM doc.document
WHERE virus_scan_status = 'PENDING'
ORDER BY created_at DESC
LIMIT 20;
```

Phase 2 remediation: deploy ClamAV container + configure gRPC endpoint, implement scan worker.

### 2.6 Virus scan: INFECTED file quarantine

When ClamAV (Phase 2) detects INFECTED file:
1. File is copied to `blips-quarantine` bucket: `quarantine/{src_bucket}/{src_key}`
2. Original deleted from `blips-documents`
3. `doc.document.virus_scan_status` updated to `INFECTED`
4. ROLE-IT-ADMIN alerted via notification service

To list quarantined files:
```bash
mc ls --recursive blips/blips-quarantine
```

To review quarantined file metadata:
```sql
SELECT id, doc_ref_kode, filename_original, virus_scan_at, entity_type, entity_id
FROM doc.document
WHERE virus_scan_status = 'INFECTED'
ORDER BY virus_scan_at DESC;
```

### 2.7 Presigned URL expired

Symptoms: Client gets 403 from MinIO when downloading

Resolution: Client must re-request GET /api/v1/documents/{id} to get a fresh presigned URL.
TTL is 60 minutes by default (configurable via `DOCUMENT_PRESIGN_TTL_MINUTES`).

Security note: presigned URL contains HMAC-signed credentials. Do not log URL at INFO level.

### 2.8 Path traversal attempt blocked

Symptoms: `400 VALIDATION_FAILED` with message about object key

Log entry: `minio: upload: object key tidak boleh dimulai dengan '/'` or `mengandung path traversal`

This is expected security behavior. Review access logs for repeated attempts — may indicate probe.
Alert ROLE-IT-ADMIN if systematic pattern detected.

### 2.9 Document soft delete vs hard delete

`doc.document` supports SOFT DELETE only (`deleted_at`, `deleted_by`, `status='DELETED'`).
Hard delete is forbidden (no endpoint, no DB cascade).

To soft-delete a document (ROLE-IT-ADMIN only):
```bash
curl -X DELETE /api/v1/documents/{id} \
  -H "Authorization: Bearer $TOKEN" \
  -H "Idempotency-Key: $(uuidgen)"
```

Note: object in MinIO is NOT deleted on soft-delete (retained for audit). Implement
periodic MinIO cleanup job only after 10-year audit retention period.

---

## 3. Integration Test Candidates (for qa-engineer)

The following tests require live infrastructure and are excluded from unit CI:

| Test scope | Requires | Run condition |
|---|---|---|
| `document.Service.Upload` end-to-end | MinIO + PostgreSQL | `INTEGRATION_TEST=true` |
| `document.Handler.Upload` multipart | MinIO + PostgreSQL + real JWT | `INTEGRATION_TEST=true` |
| `notification.Service.Deliver` real email | Live SMTP server | `INTEGRATION_TEST=true` |
| `notification.Worker.ProcessTask` Asynq | Redis | `INTEGRATION_TEST=true` |
| `document.MinIOClient.EnsureBucket` | Live MinIO | `INTEGRATION_TEST=true` |
| `document.DBRepository.Insert` | PostgreSQL blips_db | `INTEGRATION_TEST=true` |

Run integration tests:
```bash
INTEGRATION_TEST=true \
DATABASE_URL="postgres://blips_admin:..." \
REDIS_URL="redis://localhost:6379/0" \
MINIO_ENDPOINT="localhost:9000" \
go test ./internal/document/... ./internal/notification/... -run TestIntegration -v
```

---

## 4. Security Notes (for security-engineer)

1. **Presigned URL TTL**: Default 60 minutes. Adjust via `DOCUMENT_PRESIGN_TTL_MINUTES`.
   URL contains HMAC-SHA256 credentials — NEVER log at INFO/ERROR level (only DEBUG).
   
2. **Path traversal mitigation**: `ValidateObjectKey()` rejects `..`, absolute paths, and
   non-alphanumeric characters (except `/`, `-`, `_`, `.`). All object keys go through
   `BuildObjectKey()` which uses UUID-based naming — no user-controlled path components.

3. **SMTP credentials**: Must come from Vault/KMS. `SMTP_PASSWORD` in env is only for
   dev-mode override. Production: set `SMTP_PASSWORD=` empty and use Vault lookup at boot.

4. **MinIO credentials**: Same as SMTP — Vault/KMS only in production.

5. **Virus scan gap (Phase 1)**: Files with `virus_scan_status=PENDING` are served via
   presigned URL without scan confirmation. Phase 2 MUST implement scan-before-serve gate:
   block download if status is PENDING (configurable via feature flag).

6. **INFECTED files**: Handler checks `VirusScanInfected` before generating presigned URL
   and returns 403. This is enforced in `Service.GetPresignedDownloadURL()`.

7. **Audit trail**: Every upload writes to `aud.audit_log` in the same DB transaction
   as the `doc.document` INSERT. If audit write fails (non-fatal), log warning but do
   not block upload.
