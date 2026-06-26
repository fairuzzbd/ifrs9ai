# Runbook: Backup & Restore — BLIPS IFRS9

**Versi**: P5-M18  
**Regulasi**: Retensi audit log 10+10 tahun per DEC-018. Retensi backup operasional 30 hari hot + 1 tahun cold + arsip tape 10 tahun.

---

## 1. PostgreSQL backup

### 1.1 Nightly base backup (otomatis via Asynq cron)

Job `blips.pg.nightly-backup` dijalankan setiap hari pukul 01:00 WIB:

```bash
# Manual trigger backup (jika cron gagal)
kubectl exec -n blips postgres-0 -- \
  pg_dump -U blips_admin blips_db -F c -Z 9 \
  -f /tmp/blips_pg_$(date +%Y%m%d_%H%M%S).dump

# Upload ke MinIO
kubectl exec -n blips postgres-0 -- \
  sh -c 'mc cp /tmp/blips_pg_*.dump minio/blips-backups/postgres/'

# Verifikasi
mc ls minio/blips-backups/postgres/ | tail -5
```

### 1.2 WAL archive (continuous, RPO ≤ 15 menit)

PostgreSQL dikonfigurasi dengan `archive_command` ke MinIO bucket `blips-wal-archive`:

```bash
# Cek WAL archive lag
kubectl exec -n blips postgres-0 -- \
  psql -U blips_admin -c "SELECT pg_walfile_name(pg_current_wal_lsn());"

# List WAL archive terbaru
mc ls minio/blips-wal-archive/ | tail -10

# Verifikasi replica WAL lag
kubectl exec -n blips postgres-replica-0 -- \
  psql -U blips_admin -c "SELECT now() - pg_last_xact_replay_timestamp() AS lag;"
```

### 1.3 Restore dari backup

```bash
# 1.3.1 Download backup
BACKUP_DATE="20260625"
mc cp minio/blips-backups/postgres/blips_pg_${BACKUP_DATE}_*.dump /tmp/pg_restore.dump

# 1.3.2 Restore ke database kosong
kubectl exec -n blips postgres-0 -- \
  pg_restore -U blips_admin -d blips_db -v -F c /tmp/pg_restore.dump

# 1.3.3 Verifikasi row count tabel kritis
kubectl exec -n blips postgres-0 -- \
  psql -U blips_admin -d blips_db -c "
    SELECT 'mst.instrumen' as tbl, count(*) FROM mst.instrumen
    UNION ALL SELECT 'trx.transaction', count(*) FROM trx.transaction
    UNION ALL SELECT 'ecl.ecl_calc_result_line', count(*) FROM ecl.ecl_calc_result_line
    UNION ALL SELECT 'aud.audit_log', count(*) FROM aud.audit_log;
  "
```

### 1.4 Weekly tape backup

Setiap Minggu 02:00 WIB, job `blips.pg.weekly-tape` mengkompres + mengenkripsi backup ke cold storage (MinIO lifecycle tier atau tape drive fisik):

```bash
# Manual offload ke cold storage
mc cp minio/blips-backups/postgres/blips_pg_$(date +%Y%m%d)_*.dump \
  minio/blips-cold-archive/postgres/$(date +%Y/)/

# Set lifecycle rule agar file di cold archive tidak expired
mc ilm import minio/blips-cold-archive <<EOF
{"Rules":[{"Status":"Enabled","Expiration":{"Days":3650}}]}
EOF
```

---

## 2. MinIO backup (cross-site replication)

### 2.1 Setup replication ke DR site

```bash
# Cek status replication
mc admin replicate info minio blips-docs
mc admin replicate info minio blips-exports
mc admin replicate info minio blips-loki

# Lag check (objek belum ter-replicate)
mc admin replicate backlog minio blips-docs
```

### 2.2 Restore dari DR replica

```bash
# Arahkan aplikasi ke DR MinIO
# (lihat p5-disaster-recovery.md §4)

# Sync bucket ke arah sebaliknya jika primary kembali
mc mirror minio-dr/blips-docs minio-primary/blips-docs
```

---

## 3. Audit log archival (DEC-018: 10+10 tahun)

### 3.1 Arsip partisi lama ke cold storage

Jalankan manual atau via cron tahunan. Arsipkan partisi `aud.audit_log` yang berusia > 5 tahun:

```bash
# Identifikasi partisi tua
kubectl exec -n blips postgres-0 -- \
  psql -U blips_admin -d blips_db -c "
    SELECT tablename, pg_size_pretty(pg_total_relation_size('aud.'||tablename)) as size
    FROM pg_tables
    WHERE schemaname='aud' AND tablename LIKE 'audit_log_%'
    ORDER BY tablename;
  "

# Export partisi ke file terenkripsi
kubectl exec -n blips postgres-0 -- \
  pg_dump -U blips_admin blips_db \
  -t 'aud.audit_log_y2020*' -F c -Z 9 \
  -f /tmp/audit_log_2020.dump

# Encrypt
# openssl enc -aes-256-cbc -in /tmp/audit_log_2020.dump \
#   -out /tmp/audit_log_2020.dump.enc \
#   -pass env:BACKUP_ENCRYPTION_KEY

# Upload ke cold archive
mc cp /tmp/audit_log_2020.dump.enc minio/blips-cold-archive/audit/2020/

# JANGAN hapus dari PostgreSQL — soft archive saja
# Data tetap queryable selama retention aktif (10 tahun)
# Hapus dari hot storage hanya setelah 10 tahun + konfirmasi regulasi
```

### 3.2 Verifikasi retention

```bash
# Cek umur entry tertua di audit_log
kubectl exec -n blips postgres-0 -- \
  psql -U blips_admin -d blips_db -c "
    SELECT min(event_time) as oldest_entry,
           max(event_time) as newest_entry,
           count(*) as total_rows
    FROM aud.audit_log;
  "
```

---

## 4. Keycloak realm backup

```bash
# Export realm (jalankan setiap minggu via cron)
KEYCLOAK_POD=$(kubectl get pod -n blips -l app.kubernetes.io/name=keycloak -o jsonpath='{.items[0].metadata.name}')

kubectl exec -n blips ${KEYCLOAK_POD} -- \
  /opt/keycloak/bin/kc.sh export \
  --realm blips \
  --file /tmp/realm-blips-$(date +%Y%m%d).json \
  --users different_files

# Copy ke MinIO
kubectl cp blips/${KEYCLOAK_POD}:/tmp/realm-blips-$(date +%Y%m%d).json /tmp/
mc cp /tmp/realm-blips-$(date +%Y%m%d).json minio/blips-backups/keycloak/
```

---

## 5. Redis backup

Redis data adalah ephemeral (Asynq queues, cache). Backup tidak kritis karena:
- Job data juga di-persist di `sys.job` PostgreSQL.
- Cache akan rebuild otomatis.

```bash
# Manual snapshot (jika ada data penting di Redis)
kubectl exec -n blips redis-0 -- redis-cli -a ${REDIS_PASSWORD} BGSAVE
kubectl exec -n blips redis-0 -- cat /data/dump.rdb | gzip > /tmp/redis_$(date +%Y%m%d).rdb.gz
mc cp /tmp/redis_$(date +%Y%m%d).rdb.gz minio/blips-backups/redis/
```

---

## 6. Disk full — emergency actions

```bash
# Identifikasi penggunaan disk PostgreSQL
kubectl exec -n blips postgres-0 -- \
  psql -U blips_admin -d blips_db -c "
    SELECT pg_size_pretty(pg_database_size('blips_db')) as db_size;
    SELECT pg_size_pretty(pg_total_relation_size('aud.audit_log')) as audit_size;
  "

# Hapus WAL archive lama di MinIO (> 30 hari, bukan untuk aud.audit_log)
mc rm --recursive --force \
  --older-than 30d \
  minio/blips-wal-archive/

# Vacuum + analyze (jangan jalankan saat peak hours)
kubectl exec -n blips postgres-0 -- \
  psql -U blips_admin -d blips_db -c "VACUUM ANALYZE;"
```

---

## 7. Loki archive

```bash
# Loki retention otomatis 30 hari (dikonfigurasi di loki-config.yaml)
# Untuk audit log Loki jangka panjang, export ke MinIO cold archive:
# Loki menggunakan MinIO sebagai backend — data otomatis masuk ke blips-loki bucket
# Cold archive: lifecycle policy di MinIO memindahkan ke tier dingin setelah 30 hari

# Manual export log audit specific (untuk regulasi / audit eksternal)
# Gunakan LogQL via Grafana Explore atau API:
curl -s "http://localhost:3100/loki/api/v1/query_range" \
  --data-urlencode 'query={service="blips-api"} | json | action=~".*"' \
  --data-urlencode "start=2026-01-01T00:00:00Z" \
  --data-urlencode "end=2026-01-31T23:59:59Z" \
  --data-urlencode "limit=50000" \
  > /tmp/audit_logs_jan2026.json
```

---

## 8. Referensi

- DR procedure: `p5-disaster-recovery.md`
- Audit hash chain: `p5-audit-hash-verify.md`
- Incidents: `docs/incidents/`
