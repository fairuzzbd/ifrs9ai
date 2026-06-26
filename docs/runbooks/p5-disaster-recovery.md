# Runbook: Disaster Recovery — BLIPS IFRS9

**Versi**: P5-M18  
**RTO target**: ≤ 4 jam  
**RPO target**: ≤ 15 menit  
**DR site**: Secondary DC (Tugure, lokasi berbeda dari Jakarta DC)  
**Test schedule**: Kuartalan (Januari, April, Juli, Oktober)

---

## 1. Klasifikasi skenario DR

| Skenario | RTO | RPO | Prosedur |
|---|---|---|---|
| PostgreSQL primary failure (replica OK) | 15 menit | 0 (no data loss) | §2 |
| PostgreSQL total loss (primary + replica) | 2 jam | ≤ 15 menit | §3 |
| MinIO site failure (primary) | 30 menit | 0 (cross-site sync) | §4 |
| Keycloak failure | 30 menit | realm export | §5 |
| K8s cluster total failure | 4 jam | ≤ 15 menit | §6 |
| Application data corruption | case-by-case | point-in-time restore | §7 |

---

## 2. PostgreSQL replica promotion (primary down, replica sehat)

```bash
# 2.1 Verifikasi replica up dan lag WAL-nya kecil
kubectl exec -n blips postgres-replica-0 -- \
  psql -U blips_admin -d blips_db -c "SELECT now() - pg_last_xact_replay_timestamp() AS lag;"
# Expected: lag < 15 menit

# 2.2 Promote replica menjadi primary
kubectl exec -n blips postgres-replica-0 -- pg_ctl promote -D /var/lib/postgresql/data/pgdata

# 2.3 Update service selector agar api/worker connect ke replica yang sudah jadi primary
kubectl patch service postgres -n blips \
  -p '{"spec":{"selector":{"app.kubernetes.io/name":"postgres-replica","role":"replica"}}}'

# 2.4 Verifikasi backend bisa connect
kubectl exec -n blips deployment/blips-api -- \
  curl -s http://localhost:8080/readyz

# 2.5 Update ConfigMap DATABASE_URL ke host postgres-replica
kubectl patch configmap blips-config -n blips \
  --type merge \
  -p '{"data":{"PG_HOST":"postgres-replica"}}'

# 2.6 Restart pods agar env ter-reload
kubectl rollout restart deployment/blips-api deployment/blips-worker -n blips

# 2.7 Notify on-call + mulai provisioning replica baru
```

---

## 3. PostgreSQL restore dari backup (total loss)

```bash
# 3.1 Identify backup terbaru di MinIO
mc ls minio/blips-backups/postgres/ | tail -20
# Format: blips_pg_YYYYMMDD_HHMMSS.dump.gz

# 3.2 Download backup ke restore host
BACKUP_FILE="blips_pg_$(date +%Y%m%d)_*.dump.gz"
mc cp minio/blips-backups/postgres/${BACKUP_FILE} /tmp/pg_restore.dump.gz

# 3.3 Decrypt (jika encrypted dengan AES-256)
# openssl enc -d -aes-256-cbc -in /tmp/pg_restore.dump.gz.enc -out /tmp/pg_restore.dump.gz \
#   -pass env:BACKUP_ENCRYPTION_KEY

# 3.4 Decompress
gunzip /tmp/pg_restore.dump.gz

# 3.5 Restore ke PostgreSQL baru (pastikan DB kosong)
kubectl exec -n blips postgres-0 -- \
  pg_restore -U blips_admin -d blips_db -v /tmp/pg_restore.dump

# 3.6 Verifikasi row count pada tabel kritis
kubectl exec -n blips postgres-0 -- \
  psql -U blips_admin -d blips_db -c "
    SELECT schemaname, tablename, n_live_tup
    FROM pg_stat_user_tables
    WHERE n_live_tup > 0
    ORDER BY n_live_tup DESC
    LIMIT 20;
  "

# 3.7 Apply WAL archives sejak backup untuk RPO ≤ 15 menit
# (Jika WAL archive tersedia di MinIO blips-wal-archive/)
```

---

## 4. MinIO failover ke DR site

```bash
# 4.1 Cek status replication ke DR site
mc admin replicate info minio/blips-docs
mc admin replicate info minio/blips-exports

# 4.2 Jika primary MinIO down, arahkan ke DR replica
# Update ConfigMap endpoint MinIO
kubectl patch configmap blips-config -n blips \
  --type merge \
  -p '{"data":{"MINIO_ENDPOINT":"minio.dr.tugu-re.local:9000"}}'

# 4.3 Restart pods
kubectl rollout restart deployment/blips-api deployment/blips-worker -n blips

# 4.4 Verifikasi upload/download berhasil
kubectl exec -n blips deployment/blips-api -- \
  curl -s http://localhost:8080/healthz
```

---

## 5. Keycloak realm recovery

```bash
# 5.1 Import realm dari export (realm-blips-prod.json)
# Export seharusnya ada di MinIO blips-backups/keycloak/
mc cp minio/blips-backups/keycloak/realm-blips-prod.json /tmp/

# 5.2 Deploy Keycloak baru dan import realm
kubectl apply -f deploy/k8s/keycloak/deployment.yaml

# 5.3 Tunggu Keycloak up
kubectl rollout status deployment/keycloak -n blips --timeout=300s

# 5.4 Import realm via Keycloak CLI
kubectl exec -n blips deployment/keycloak -- \
  /opt/keycloak/bin/kc.sh import \
  --file /tmp/realm-blips-prod.json \
  --override true

# 5.5 Verifikasi realm aktif
curl -fs https://auth.blips.tugu-re.com/realms/blips/.well-known/openid-configuration | jq .issuer
```

---

## 6. K8s cluster full recovery

```bash
# 6.1 Provision K8s cluster baru (Ansible baseline)
# cd deploy/ansible && ansible-playbook -i inventories/dr playbooks/k8s-setup.yml

# 6.2 Apply semua manifests berurutan (lihat deploy/k8s/README.md §Deploy order)
kubectl apply -f deploy/k8s/namespace.yaml
# ... (ikuti urutan di README.md)

# 6.3 Restore data (PostgreSQL §3, MinIO §4, Keycloak §5)

# 6.4 Verify full stack
curl -fs https://api.blips.tugu-re.com/healthz
curl -fs https://blips.tugu-re.com
```

---

## 7. Data corruption — point-in-time restore

```bash
# 7.1 Identifikasi waktu terjadinya corruption
# Cek audit log untuk WINDOW corruption:
kubectl exec -n blips postgres-0 -- \
  psql -U blips_admin -d blips_db -c "
    SELECT event_time, actor_user_id, action, entity_type, entity_id
    FROM aud.audit_log
    WHERE event_time BETWEEN '<suspected_start>' AND '<suspected_end>'
    ORDER BY event_time;
  "

# 7.2 Point-in-time recovery via WAL
# (restore ke waktu sebelum corruption menggunakan pg_restore + WAL replay)
# Jangan hapus data yang rusak — buat schema baru untuk investigasi

# 7.3 Konsultasikan dengan security-engineer sebelum eksekusi restore
# Flag sebagai [NEEDS-HUMAN] jika menyangkut aud.jrnl.ecl schema
```

---

## 8. Quarterly DR drill checklist

Lakukan setiap kuartal. Dokumentasikan hasil di `docs/incidents/dr-drill-YYYY-QN.md`.

- [ ] Test §2: promote replica — verifikasi lag WAL < 15 menit
- [ ] Test §3: restore dari backup — verifikasi row count
- [ ] Test §4: switch endpoint MinIO ke DR
- [ ] Test §5: import Keycloak realm
- [ ] Ukur actual RTO vs target (≤ 4 jam)
- [ ] Ukur actual RPO vs target (≤ 15 menit)
- [ ] Update runbook jika ada perbaikan prosedur
- [ ] Sign-off oleh ROLE-IT-ADMIN + lapor ke CFO

---

## 9. Referensi

- Backup detail: `p5-backup-restore.md`
- Incident escalation: `p5-incident-response.md`
- Audit chain setelah recovery: `p5-audit-hash-verify.md`
