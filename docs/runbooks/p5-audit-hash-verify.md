# Runbook: Audit Hash Chain Verification — BLIPS IFRS9

**Versi**: P5-M18  
**Compliance**: DEC-018 (audit trail retention 10+10 tahun, hash chain immutability)  
**Frekuensi**: Harian (cron via Asynq) + manual saat diminta atau ada alert

---

## 1. Cara kerja hash chain

Setiap row di `aud.audit_log` memiliki:
- `previous_hash`: SHA-256 dari row sebelumnya (atau zero-hash untuk entry pertama)
- `current_hash`: SHA-256 dari `(previous_hash || canonical_json(row_data))`

Verifikasi dilakukan dengan mengulang komputasi hash dari awal range dan memvalidasi setiap `current_hash`.

```
entry[1]: current_hash = SHA256(zero_hash || json(entry[1]))
entry[2]: current_hash = SHA256(entry[1].current_hash || json(entry[2]))
entry[N]: current_hash = SHA256(entry[N-1].current_hash || json(entry[N]))
```

Jika hash tidak match → ada modifikasi atau penghapusan data di range tersebut.

---

## 2. Verifikasi manual

### 2.1 Via binary `cmd/audit-verify`

```bash
# Verifikasi range tanggal (format: YYYY-MM-DD)
go run ./cmd/audit-verify --range "2026-01-01:2026-06-30"

# Verifikasi seluruh tabel (WARNING: bisa lama untuk dataset besar)
go run ./cmd/audit-verify --range "2024-01-01:2026-12-31"

# Verifikasi partition tertentu
go run ./cmd/audit-verify --partition "audit_log_y2026m06"

# Output jika OK:
# [OK] audit_log range 2026-01-01:2026-06-30 — 1,234,567 rows verified, hash chain intact.

# Output jika ada masalah:
# [FAIL] Hash mismatch at event_id=<uuid>, event_time=2026-06-15T14:23:11+07:00
# Expected: a1b2c3... Got: d4e5f6...
# Rows affected: 12 (event_id range: <uuid> to <uuid>)
```

### 2.2 Via Kubernetes (production)

```bash
# Run sebagai Job di K8s
kubectl run audit-verify --restart=Never -n blips \
  --image=registry.tugu-re.local/blips/tools:latest \
  --env="DATABASE_URL=postgres://blips_admin:${POSTGRES_PASSWORD}@postgres:5432/blips_db?sslmode=disable" \
  -- audit-verify --range "$(date -d '30 days ago' +%Y-%m-%d):$(date +%Y-%m-%d)"

kubectl logs audit-verify -n blips -f
kubectl delete pod audit-verify -n blips
```

### 2.3 Cron harian (Asynq job `blips.audit.hash-verify`)

Job dijadwalkan setiap hari 03:00 WIB untuk memverifikasi 7 hari terakhir.  
Hasil verifikasi di-publish ke Prometheus metric `blips_audit_hash_chain_verification_failures_total`.

---

## 3. Jika hash chain gagal — prosedur forensik

### chain-broken

**PERINGATAN**: Jangan truncate, jangan hapus, jangan amend data di `aud.audit_log`. Tindakan ini bersifat forensik — data harus dipreservasi.

```bash
# 3.1 Catat window kegagalan
# Identifikasi range event_id yang bermasalah dari output audit-verify

# 3.2 Isolasi: buat snapshot tabel untuk investigasi
kubectl exec -n blips postgres-0 -- \
  psql -U blips_admin -d blips_db -c "
    CREATE TABLE aud.audit_log_forensic_$(date +%Y%m%d)
    AS SELECT * FROM aud.audit_log
    WHERE event_id BETWEEN '<start_uuid>'::uuid AND '<end_uuid>'::uuid;
  "

# 3.3 Export snapshot untuk investigasi offline
kubectl exec -n blips postgres-0 -- \
  pg_dump -U blips_admin blips_db \
  -t 'aud.audit_log_forensic_*' \
  -F c -f /tmp/audit_forensic_$(date +%Y%m%d).dump

mc cp postgres-0:/tmp/audit_forensic_$(date +%Y%m%d).dump \
  minio/blips-forensic/$(date +%Y%m%d)/

# 3.4 Eskalasi
# Notifikasi: security-engineer, ROLE-AUDIT, CFO (dalam 15 menit per SLA)
# Buat incident doc: docs/incidents/$(date +%Y-%m-%d)-audit-hash-breach.md

# 3.5 Lanjutkan operasi di window lain (hash chain di luar window tetap valid)
# Jangan stop seluruh sistem karena satu window hash failure

# 3.6 Investigasi penyebab:
# a) Bug di aplikasi (hash computation error) → fix + recompute dari snapshot
# b) Direct DB modification tanpa aplikasi → forensic + regulasi report
# c) Disk corruption → verifikasi dengan pg_dump restore ke DB terpisah

# 3.7 Jika penyebab adalah bug aplikasi:
# - Deploy fix
# - Recompute hash untuk window yang terdampak (HANYA jika data tidak dimodifikasi)
# - Dokumentasikan di audit_log dengan action AUDIT.HASH.RECOMPUTE + alasan

# 3.8 Jika penyebab adalah unauthorized modification:
# [NEEDS-HUMAN] → Eskalasi ke CISO + OJK notification jika menyangkut data nasabah
```

---

## 4. Verifikasi periodik (audit eksternal)

Saat audit eksternal (OJK, akuntan publik) meminta verifikasi:

```bash
# Generate verifikasi report untuk range spesifik
go run ./cmd/audit-verify \
  --range "2026-01-01:2026-12-31" \
  --output /tmp/audit_verify_2026.json \
  --format json

# JSON output format:
# {
#   "verified_range": "2026-01-01:2026-12-31",
#   "total_rows": 4567890,
#   "status": "OK",
#   "verification_time": "2026-06-25T10:00:00+07:00",
#   "hash_algorithm": "SHA-256",
#   "first_event_id": "...",
#   "last_event_id": "...",
#   "failures": []
# }

# Encrypt sebelum share ke auditor eksternal
openssl enc -aes-256-cbc -in /tmp/audit_verify_2026.json \
  -out /tmp/audit_verify_2026.json.enc \
  -pass env:AUDIT_EXPORT_KEY
```

---

## 5. Grafana monitoring

Buka dashboard `blips-audit-trail`:
- Panel **Hash Chain Failures**: harus selalu 0.
- Panel **Audit Write Failures**: harus selalu 0.
- Alert `BlipsAuditHashChainBroken`: critical, immediate page.

---

## 6. Referensi

- Incident response: `p5-incident-response.md`
- Backup untuk forensic: `p5-backup-restore.md §3`
- Security baseline: `.claude/memory/security-baseline.md §Audit trail`
- DEC-018: retention + hash chain requirement
