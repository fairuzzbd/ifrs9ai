# P5-M13 State Machines — Reporting MV Foundation

**Story Set**: P5-M13  
**Modul**: APP-E — Reporting & Dashboard  
**Author**: backend-engineer-go (system-analyst handoff)  
**Date**: 2026-06-23  

---

## 1. MV Refresh State Machine

```
IDLE ──► REFRESHING ──► IDLE       (happy path)
                    └──► FAILED    (DB error / lock timeout → DLQ)
```

### Transitions

| From | Event | To | Guard | Effect |
|---|---|---|---|---|
| IDLE | TriggerRefresh (CRON / HARD_CLOSE / MANUAL) | REFRESHING | Advisory lock acquired | INSERT sys.mv_refresh_log status=RUNNING; Asynq job enqueued |
| IDLE | TriggerRefresh (any) | IDLE (rejected) | Advisory lock **not** acquired (another refresh running) | HTTP 423 MV_REFRESH_LOCKED; no INSERT |
| REFRESHING | RefreshComplete | IDLE | — | UPDATE sys.mv_refresh_log status=COMPLETED, completed_at, row_count; audit REPORT.MV_REFRESH |
| REFRESHING | RefreshError | FAILED | — | UPDATE sys.mv_refresh_log status=FAILED, error_detail; Asynq DLQ; audit REPORT.MV_REFRESH_FAILED; alert |

**Advisory Lock**: PostgreSQL `pg_try_advisory_lock(hashtext(mv_name))` — non-blocking. If lock not acquired → MV_REFRESH_LOCKED immediately (no wait). Lock released after REFRESH MATERIALIZED VIEW CONCURRENTLY completes.

**Cron schedule**: `0 18 * * *` UTC = `01:00 WIB` (env `MV_REFRESH_CRON`, default `0 18 * * *`).

**Concurrent refresh prerequisite**: every MV must have a unique index (migration 000050). Without it, REFRESH MATERIALIZED VIEW CONCURRENTLY fails.

**Trigger sources**:
- `CRON`: Asynq scheduler, daily 01:00 WIB, all 8 MVs
- `HARD_CLOSE`: P5-M4 hard-close handler enqueues `reporting:mv-refresh_all` after commit
- `MANUAL`: ROLE-IT-ADMIN POST /api/v1/admin/mv-refresh

---

## 2. Export State Machine

```
REQUESTED ──► INLINE_DELIVERED     (dataset ≤ REPORT_EXPORT_INLINE_THRESHOLD; HTTP 200 stream)
          └──► QUEUED ──► COMPUTING ──► COMPLETED    (async: Asynq + MinIO)
                                    └──► FAILED      (worker error → DLQ)
```

### Transitions

| From | Event | To | Guard | Effect |
|---|---|---|---|---|
| — | RequestExport (format valid, permission OK) | REQUESTED | row_count ≤ MAX_ROWS | INSERT sys.export_log status=REQUESTED |
| REQUESTED | row_count ≤ INLINE_THRESHOLD | INLINE_DELIVERED | — | stream file; compute SHA-256; UPDATE sys.export_log status=COMPLETED; audit EXPORT.GENERATED |
| REQUESTED | row_count > INLINE_THRESHOLD | QUEUED | — | Asynq enqueue `reporting:export-async`; UPDATE sys.export_log status=QUEUED; HTTP 202 |
| QUEUED | Worker picks up | COMPUTING | — | UPDATE sys.export_log status=COMPUTING; sys.job progress=0 |
| COMPUTING | Export complete, upload MinIO | COMPLETED | — | UPDATE sys.export_log + sys.job; signed URL; SMTP notif; audit EXPORT.GENERATED |
| COMPUTING | Worker error | FAILED | — | UPDATE status=FAILED; Asynq DLQ; audit |
| — | row_count > MAX_ROWS | — (rejected) | — | HTTP 422 EXPORT_TOO_LARGE; no INSERT |
| — | format not csv/xlsx/pdf | — (rejected) | — | HTTP 400 EXPORT_FORMAT_UNSUPPORTED |
| — | permission missing, not ROLE-AUDIT | — (rejected) | — | HTTP 403 EXPORT_PERMISSION_DENIED |

### Thresholds (sys.config_param)

| Key | Default | Effect |
|---|---|---|
| `REPORT_EXPORT_INLINE_THRESHOLD` | 10000 | ≤ → inline 200; > → async 202 |
| `REPORT_EXPORT_MAX_ROWS` | 100000 | > → EXPORT_TOO_LARGE 422 |
| `REPORT_EXPORT_MINIO_TTL_HOURS` | 24 | MinIO signed URL TTL |

### MinIO path pattern
```
exports/{tenant_id}/{user_id}/{yyyy}/{mm}/{dd}/{job_id}.{format}
```
MinIO bucket lifecycle: `Expiration: 1 day` (auto-delete). After expiry, signed URL returns 404.

### SHA-256 dual
- **File content hash**: `sha256(file_bytes)` — stored in `sys.export_log.sha256_hash`
- **Audit log row hash**: standard BLIPS hash chain in `aud.audit_log.current_hash` (existing pattern)

### Watermark
- **XLSX**: footer row per sheet — `"RAHASIA - BLIPS Tugu Re — exported {timestamp} by {username}"`; freeze header pane; header bold
- **PDF**: footer every page via gofpdf — same text; diagonal faint watermark "RAHASIA"
- **CSV**: last line comment — `# RAHASIA - BLIPS Tugu Re — exported {timestamp} by {username}`

### Download audit
`EXPORT.DOWNLOADED` in-transaction written by `GET /reports/export/{export_id}/download` handler.

---

## 3. Scheduled Email State Machine

```
ACTIVE ──► INACTIVE    (soft-delete: deleted_at set)
       └──► OPT_OUT_PER_RECIPIENT   (sys.scheduled_email_optout; config unchanged)
```

### Scheduled email send sub-state (per-send execution)

```
PENDING ──► SENDING ──► SENT      (happy path; audit SCHEDULED_EMAIL.SENT)
                    └──► FAILED   (SMTP error → retry 1..3 → DLQ; audit warning)
```

### Transitions

| From | Event | To | Guard | Effect |
|---|---|---|---|---|
| — | POST /reports/scheduled-emails | ACTIVE | ROLE-AKUN-CTL; cron syntax valid; recipients ≥ 1 | INSERT sys.scheduled_email active=true; audit SCHEDULED_EMAIL.CREATED |
| ACTIVE | DELETE /reports/scheduled-emails/{id} | INACTIVE | ROLE-AKUN-CTL | soft-delete; audit SCHEDULED_EMAIL.DELETED |
| ACTIVE | Cron fire | PENDING | active=true; deleted_at IS NULL | Asynq enqueue `reporting:scheduled-email-send` |
| PENDING | SMTP success | SENT | opt-out filter applied | UPDATE last_sent_at, last_status=SENT; audit SCHEDULED_EMAIL.SENT; recipient_count = total − opt-outs |
| PENDING/SENDING | SMTP error attempt 1..2 | retry | retry_count < MAX | Asynq backoff (30s, 60s) |
| PENDING/SENDING | SMTP error attempt 3 | FAILED | retry_count ≥ MAX | Asynq DLQ; UPDATE last_status=FAILED; audit; alert |
| ACTIVE | POST /opt-out (signed token) | OPT_OUT_PER_RECIPIENT | HMAC token valid | INSERT sys.scheduled_email_optout; future sends skip email |

### SMTP retry config (sys.config_param)

| Key | Default |
|---|---|
| `REPORT_SMTP_RETRY_MAX` | 3 |
| Backoff | 30s / 60s (Asynq MaxRetry) |

### Opt-out signed token
Format: `HMAC-SHA256(secret, scheduled_email_id + ":" + email + ":" + expires_unix)`  
TTL: 30 days (embedded in token). Verify before INSERT opt-out.

---

## 4. Async Export Threshold Decision (UX §3 compliance)

```
row_count > REPORT_EXPORT_MAX_ROWS  → 422 EXPORT_TOO_LARGE (before any job)
row_count > REPORT_EXPORT_INLINE_THRESHOLD → 202 + Asynq job + SSE stream
row_count ≤ REPORT_EXPORT_INLINE_THRESHOLD → 200 inline stream
```

SSE stream endpoint `/api/v1/jobs/{jobId}/stream` (EventSource; fallback polling 2s).  
Progress: `0..100` reported from Asynq worker via Redis pub/sub → SSE handler subscribes.

---

## 5. Read-Replica Routing

```
Query to rpt.mv_* → ChooseDB(primary, replica, ReadIntentReporting)
                     │
                     ├─ MV_DSN set → replica *sql.DB (log "MV query routed to read-replica DSN")
                     └─ MV_DSN not set → primary *sql.DB (log WARN "MV_DSN not set — falling back to primary DSN. Set MV_DSN for read-replica routing.")
```

No error returned on fallback — graceful degradation.

---

## 6. Audit Events Summary

| Event | Trigger | In-transaction |
|---|---|---|
| `REPORT.MV_REFRESH` | Every CONCURRENT refresh completed (S1, S2) | Yes |
| `REPORT.MV_REFRESH_FAILED` | Worker refresh failed (S2-AC4) | Yes |
| `EXPORT.GENERATED` | Export engine complete (S3-AC1/2, S4-AC1) | Yes |
| `EXPORT.DOWNLOADED` | User GET download endpoint (S4-AC4) | Yes |
| `SCHEDULED_EMAIL.CREATED` | POST /scheduled-emails success (S5-AC1) | Yes |
| `SCHEDULED_EMAIL.DELETED` | DELETE /scheduled-emails/{id} (S5) | Yes |
| `SCHEDULED_EMAIL.SENT` | SMTP send success (S5-AC2) | Yes |

---

## 7. Hand-off

- `backend-engineer-go` → implement `backend/internal/reporting/` package per deliverables
- `devops-engineer` → MinIO bucket `exports` + lifecycle `Expiration: 1 day`; env `MV_DSN`, `SMTP_HOST/PORT/FROM/PASSWORD`; Grafana alert for `MV_REFRESH_FAILED` + `SCHEDULED_EMAIL_SMTP_FAILED`
- `security-engineer` BLOCKING → audit 6 events in-tx; SHA-256 dual; MinIO signed URL TTL; SMTP creds env (not hardcoded); EXPORT_PERMISSION_DENIED enforce per-report; ROLE-AUDIT bypass documented
- `ifrs9-compliance-reviewer` ADVISORY → M13 = infra only; ECL/EIR formula not touched; formal gate in M14
- `qa-engineer` → UAT scripts per S1..S5 AC; test async threshold boundary (9999 / 10000 / 10001 / 100000 / 100001 rows)
