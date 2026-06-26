# Runbook: Observability — BLIPS IFRS9

**Versi**: P5-M18  
**Stack**: Prometheus + Grafana 11 + Loki 3 + Alertmanager

---

## 1. Akses dashboard

| Dashboard | URL | Siapa |
|---|---|---|
| Grafana | https://monitoring.blips.tugu-re.com | ROLE-IT-ADMIN, ROLE-RISK |
| Prometheus | Internal only — `kubectl port-forward svc/prometheus 9090:9090 -n blips` | ROLE-IT-ADMIN |
| Alertmanager | Internal only — `kubectl port-forward svc/alertmanager 9093:9093 -n blips` | ROLE-IT-ADMIN |

**Login Grafana**: SSO via Keycloak (OAuth2). Admin password di secret `blips-secret`.

---

## 2. Daftar dashboard dan cara membaca

### 2.1 `blips-overview`

Dashboard utama, refresh 30 detik. Buka ini pertama kali saat ada alert.

Panels kunci:
- **API RPS**: request rate. Normal: 10-200 req/s. Spike > 1000 → check DDoS atau bulk import.
- **Error Rate**: target < 0.1%. Alert saat > 1%. Lihat panel "Request Rate by Status Code" untuk breakdown 4xx vs 5xx.
- **P95 Latency**: target ≤ 200ms. SLA breach saat > 500ms sustained.
- **Asynq Queue Depth**: normal < 500. Alert saat > 10k (lihat `blips-asynq-jobs`).
- **PostgreSQL Connections**: alert saat > 80% pool (default max_connections=100).

### 2.2 `blips-api-latency`

Breakdown latency per route. Berguna saat P95 naik untuk isolasi route bermasalah.

Panel "P95 per Route": cari route dengan latency tertinggi → cek backend code + index DB.

### 2.3 `blips-asynq-jobs`

Monitor Asynq worker queue. Panels:
- **Queue Depth per Queue**: normal < 100. ECL queue bisa spike > 1000 saat calc run.
- **DLQ Growth**: setiap entry di DLQ = job yang gagal 3x. Investigasi segera.
- **ECL Calc Run Duration**: normal 10-30 menit. Alert saat > 60 menit.

### 2.4 `blips-ecl-engine`

Khusus ECL. Gunakan saat periode close atau ada issue ECL calc.
- **Instruments by Stage**: monitor pergerakan stage 1→2 atau 2→3.
- **ECL Engine Logs**: filter `level=error` untuk lihat exception.

### 2.5 `blips-audit-trail`

Dashboard compliance. Audit write failures HARUS selalu nol.
- Alert `BlipsAuditLogWriteFailure` = critical. Eskalasi ke security-engineer dalam 15 menit.
- Alert `BlipsAuditHashChainBroken` = forensic. Lihat §p5-audit-hash-verify.

### 2.6 `blips-mtm-pipeline`

Monitor feed harian (IBPA bond price, FX JISDOR, NAB Reksadana).
- **Last IBPA Import**: normal < 26 jam (feed harian). Alert jika > 24h.
- **MTM Import Failures**: setiap failure di-investigate (lihat worker logs di Loki).

### 2.7 `blips-periode-close`

Aktif saat mendekati akhir bulan.
- **Pending Journal Postings**: harus turun ke 0 sebelum hard close.
- **MV Refresh Lag**: harus < 2 jam setelah hard close.

---

## 3. Log exploration via Loki

Akses dari Grafana → Explore → datasource Loki.

### Query umum:

```logql
# Semua error dari API
{service="blips-api"} | json | level="error"

# Audit events untuk user tertentu
{service="blips-api"} | json | action=~".*" | userId="<uuid>"

# ECL calc run logs
{service="blips-worker"} |= "ecl" | json

# Request lambat > 1s
{service="blips-api"} | json | latency > 1000

# Logs dengan traceId tertentu (dari error toast di UI)
{service=~"blips-.*"} | json | traceId="<traceId>"
```

---

## 4. Alert escalation matrix

| Alert | Severity | SLA | Escalate ke | Waktu |
|---|---|---|---|---|
| `BlipsAuditLogWriteFailure` | critical/compliance | Immediately | security-engineer + CFO | < 15 menit |
| `BlipsAuditHashChainBroken` | critical/forensic | Immediately | security-engineer + IT-Admin | < 15 menit |
| `BlipsAPIErrorRateHigh` | critical | 5 menit | IT-Admin on-call | < 30 menit |
| `BlipsDBConnectionPoolCritical` | critical | 2 menit | IT-Admin on-call | < 15 menit |
| `BlipsPostgresDiskCritical` | critical | 5 menit | IT-Admin on-call | < 15 menit |
| `BlipsAsynqQueueBacklog` | critical | 5 menit | IT-Admin + ROLE-RISK | < 30 menit |
| `BlipsAPILatencyP99Critical` | critical | 5 menit | IT-Admin on-call | < 30 menit |
| `BlipsJurnalPostingFailure` | critical/compliance | Immediately | IT-Admin + ROLE-AKUN-CTL | < 15 menit |
| `BlipsAPILatencyP95High` | warning | 30 menit | IT-Admin | business hours |
| `BlipsDBConnectionPoolHigh` | warning | 30 menit | IT-Admin | business hours |
| `BlipsPostgresDiskHigh` | warning | 4 jam | IT-Admin | next business day |
| `BlipsMTMPricingStale` | warning | 2 jam | ROLE-AKUN | business hours |

---

## 5. On-call rotation

Lihat `docs/team/oncall-schedule.md` (TODO: buat oleh ROLE-IT-ADMIN).

Kontak eskalasi darurat:
- **L1**: IT-Admin on-call — via PagerDuty
- **L2**: Tech Lead + Security Engineer — via Slack #blips-oncall + telepon
- **L3**: CFO + CISO — hanya untuk insiden compliance/forensic

---

## 6. Topik-topik khusus

### api-latency
Investigasi latency tinggi:
1. Buka `blips-api-latency` → identifikasi route.
2. Cek Loki query: `{service="blips-api"} | json | route="/api/v1/ecl/..."  | latency > 500`.
3. Cek `pg_stat_activity` untuk long-running query: `kubectl exec postgres-0 -n blips -- psql -U blips_admin -c "SELECT pid,now()-query_start as dur,query FROM pg_stat_activity WHERE state='active' ORDER BY dur DESC LIMIT 10;"`.
4. Cek Prometheus: `pg_stat_user_tables` untuk missing index.

### db-pool
Investigasi connection pool exhaustion:
1. Lihat `blips-overview` panel PostgreSQL Connections.
2. `pg_stat_activity`: `SELECT client_addr, state, count(*) FROM pg_stat_activity GROUP BY 1,2 ORDER BY 3 DESC;`
3. Identifikasi koneksi idle-in-transaction yang tidak di-close.
4. Short-term fix: increase `max_connections` via ConfigMap + restart pods.
5. Long-term: tuning pool size di application atau tambah pgbouncer.

### asynq-backlog
1. Cek `blips-asynq-jobs` → queue yang backlog.
2. Cek worker logs: `{service="blips-worker"} | json | level="error"`.
3. Scale worker replicas sementara: `kubectl scale deployment blips-worker --replicas=4 -n blips`.
4. Cek DLQ untuk failed jobs, retry jika safe.

### ecl-calc-slow
1. Cek `blips-ecl-engine` panel.
2. Cek DB query plan pada ECL computation query.
3. Cek apakah calc run bisa di-cancel via API: `POST /api/v1/jobs/{jobId}/cancel`.

### mtm-stale
1. Cek IBPA SFTP server accessibility.
2. Manual trigger re-import: `POST /api/v1/integration/ibpa/import`.
3. Jika IBPA down > 24h, gunakan pricing terakhir yang valid + notifikasi ke ROLE-RISK.

### jurnal-backlog
1. Lihat `blips-periode-close` panel.
2. Cek worker logs untuk error posting.
3. Manual trigger re-post: `POST /api/v1/jurnal/retry-failed`.

### mv-refresh
1. Cek Prometheus `blips_mv_last_refresh_timestamp`.
2. Manual trigger: `POST /api/v1/admin/mv-refresh`.
3. Jika gagal berulang, cek disk space PostgreSQL.

### error-rate
Lihat `blips-overview` → "Request Rate by Status Code".
- Spike 4xx: kemungkinan validasi client error atau auth issue.
- Spike 5xx: server error — cek Loki `level=error`.

### reporting-latency
- Jika endpoint `/api/v1/reports/` lambat, pastikan async export digunakan untuk dataset > 10k row.
- Cek `rpt.mv_*` sudah di-refresh setelah hard close.
