# Runbook: Incident Response — BLIPS IFRS9

**Versi**: P5-M18  
**Post-mortem template**: `docs/incidents/YYYY-MM-DD-{slug}.md`

---

## 1. Severity matrix

| Severity | Definisi | Response time | Eskalasi |
|---|---|---|---|
| **SEV-1 (Critical)** | Production down total atau compliance breach (audit write failure, hash chain broken) | < 15 menit | L1 → L2 → L3 (CFO/CISO) |
| **SEV-2 (High)** | Fitur utama tidak berfungsi (ECL calc gagal, jurnal posting failure, MTM stale > 24h) | < 30 menit | L1 → L2 |
| **SEV-3 (Medium)** | Degradasi performa (latency tinggi, error rate > 0.1%) | < 2 jam | L1 |
| **SEV-4 (Low)** | Non-blocking issue, workaround tersedia | Next business day | L1 |

---

## 2. First response checklist (semua severity)

```bash
# 2.1 Identifikasi scope dari alert
# Buka Grafana: https://monitoring.blips.tugu-re.com
# Dashboard blips-overview → identifikasi metric yang merah

# 2.2 Cek pod status
kubectl get pods -n blips

# 2.3 Cek recent events
kubectl get events -n blips --sort-by='.lastTimestamp' | tail -20

# 2.4 Catat: waktu mulai, alert name, metric value, pod/service yang terdampak
```

---

## 3. Skenario spesifik

### sod-violation

SoD violation attempt terdeteksi via `BlipsSoDViolationAttempt` alert.

```bash
# 3.1 Query audit log untuk melihat detail attempt
kubectl exec -n blips postgres-0 -- \
  psql -U blips_admin -d blips_db -c "
    SELECT event_time, actor_user_id, action, entity_type, entity_id, ip, trace_id
    FROM aud.audit_log
    WHERE action LIKE '%SOD%'
    ORDER BY event_time DESC
    LIMIT 20;
  "

# 3.2 Identifikasi user
# 3.3 Jika terindikasi deliberate bypass attempt → eskalasi ke security-engineer
# 3.4 Pertimbangkan disable user sementara di Keycloak:
#   kubectl exec keycloak-pod -- /opt/keycloak/bin/kcadm.sh update users/<userId> -r blips -s enabled=false
```

### mfa-bypass

Alert `BlipsMFABypassAttempt` — MFA bypass pada endpoint protected.

```bash
# 3.1 Lihat logs detail
{service="blips-api"} | json | msg=~".*mfa.*bypass.*" or msg=~".*step.up.*failed.*"

# 3.2 Identifikasi IP + user
kubectl exec -n blips postgres-0 -- \
  psql -U blips_admin -d blips_db -c "
    SELECT event_time, actor_user_id, ip, action, trace_id
    FROM aud.audit_log
    WHERE action LIKE '%MFA%'
    AND event_time > now() - interval '1 hour'
    ORDER BY event_time DESC;
  "

# 3.3 Eskalasi immediate ke security-engineer
# 3.4 Block IP jika perlu via Traefik middleware
```

### jwt-spike

Alert `BlipsJWTValidationFailureSpike` — rate JWT failure tinggi.

```bash
# Kemungkinan: token replay, brute force, atau jam server tidak sinkron
# 3.1 Cek waktu server vs Keycloak
kubectl exec -n blips deployment/blips-api -- date
kubectl exec -n blips deployment/keycloak -- date

# 3.2 Cek Keycloak availability
curl -fs https://auth.blips.tugu-re.com/health/ready

# 3.3 Cek apakah JWT signing key berubah (setelah rotation)
# Verifikasi JWKS endpoint
curl https://auth.blips.tugu-re.com/realms/blips/protocol/openid-connect/certs | jq .
```

### db-pool-exhaustion

Alert `BlipsDBConnectionPoolCritical`.

```bash
# 3.1 Identifikasi koneksi stale
kubectl exec -n blips postgres-0 -- \
  psql -U blips_admin -d blips_db -c "
    SELECT pid, now()-pg_stat_activity.query_start AS duration,
           query, state, wait_event_type, wait_event
    FROM pg_stat_activity
    WHERE state != 'idle' AND query_start < now() - interval '5 minutes'
    ORDER BY duration DESC;
  "

# 3.2 Kill koneksi idle-in-transaction > 10 menit
kubectl exec -n blips postgres-0 -- \
  psql -U blips_admin -d blips_db -c "
    SELECT pg_terminate_backend(pid)
    FROM pg_stat_activity
    WHERE state = 'idle in transaction'
    AND query_start < now() - interval '10 minutes';
  "

# 3.3 Short-term: scale down worker untuk kurangi connection pressure
kubectl scale deployment blips-worker --replicas=1 -n blips
```

### jurnal-posting-failure

Alert `BlipsJurnalPostingFailure` — compliance-critical.

```bash
# 3.1 Identifikasi jurnal yang gagal
kubectl exec -n blips postgres-0 -- \
  psql -U blips_admin -d blips_db -c "
    SELECT id, periode_id, status, error_message, created_at
    FROM jrnl.posting_queue
    WHERE status = 'failed'
    ORDER BY created_at DESC
    LIMIT 20;
  "

# 3.2 Cek apakah error karena periode sudah closed
# Jika ya → koordinasikan dengan ROLE-AKUN-CTL untuk reopen periode (soft close only)

# 3.3 Retry posting untuk jurnal yang gagal karena transient error
curl -X POST https://api.blips.tugu-re.com/api/v1/jurnal/retry-failed \
  -H "Authorization: Bearer <IT-ADMIN-TOKEN>"

# 3.4 Dokumentasikan semua entry yang gagal untuk audit trail
# JANGAN hapus record gagal dari jrnl.posting_queue — soft delete saja
```

---

## 4. ECL/EIR drift — halt deploy

Jika perubahan menyebabkan ECL/EIR yang sudah dihitung berubah:

1. **Halt**: stop deploy segera via `kubectl rollout undo`.
2. **Alert**: notifikasi `ifrs9-compliance-reviewer` + `ROLE-ALCO` + CFO.
3. **Assess**: bandingkan ECL output lama vs baru untuk semua instrumen pada periode aktif.
4. **Reconcile**: jika memang berubah secara signifikan → ini adalah MAJOR version bump per git-conventions.md.
5. **ALCO approval**: perubahan ECL/EIR calculation butuh ALCO approval formal.
6. **Back-fill plan**: dokumentasikan perbedaan dan apakah perlu recompute periode sebelumnya.

---

## 5. Audit hash chain break — forensic procedure

Lihat runbook khusus: `p5-audit-hash-verify.md §chain-broken`.

**JANGAN truncate tabel `aud.audit_log` dalam kondisi apapun.**

---

## 6. Regulatory-impact triage

| Impact | Kategori | Tindakan |
|---|---|---|
| ECL/EIR output berubah | Regulatory recompute | Halt deploy, ALCO approval, MAJOR semver |
| Audit hash chain break | Forensic | Isolate window, eskalasi ke Audit + CISO |
| PII data exposure | Data breach | OJK notification dalam 14 hari (UU PDP) |
| Jurnal posting failure | GL integrity | Hard stop, ROLE-AKUN-CTL + CFO sign-off |
| Periode hard-close tanpa approval | Governance | Reopen jika soft-close, eskalasi jika hard |

---

## 7. Post-mortem template

Buat file `docs/incidents/YYYY-MM-DD-{slug}.md` dengan:

```markdown
# Incident: {judul singkat}

**Tanggal**: YYYY-MM-DD  
**Severity**: SEV-{1|2|3|4}  
**Durasi**: {mulai} — {selesai} ({total menit})  
**Impact**: {service/feature terdampak}  
**Regulatory impact**: {ya/tidak, jika ya: apa}

## Timeline
- HH:MM — Alert masuk
- HH:MM — L1 on-call acknowledge
- ...

## Root cause
{analisis teknis}

## Mitigasi yang diambil
{langkah-langkah}

## Action items
| Item | Owner | Due date |
|------|-------|----------|
| | | |

## Lessons learned
{apa yang bisa diperbaiki}
```

---

## 8. Referensi

- Observability: `p5-observability.md`
- DR: `p5-disaster-recovery.md`
- Audit: `p5-audit-hash-verify.md`
- Secrets rotation: `p5-secrets-rotation.md`
