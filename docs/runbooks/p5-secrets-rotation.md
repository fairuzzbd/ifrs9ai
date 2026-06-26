# Runbook: Secrets Rotation — BLIPS IFRS9

**Versi**: P5-M18  
**Frekuensi**: Kuartalan (tiap Januari, April, Juli, Oktober)  
**Owner**: ROLE-IT-ADMIN  
**Sign-off**: Tech Lead + Security Engineer

---

## 1. Daftar secrets yang dirotasi

| Secret | Lokasi | Dampak restart | Prioritas |
|---|---|---|---|
| JWT signing key (Keycloak RSA-2048) | Keycloak DB | Semua active session invalid | Tinggi |
| PostgreSQL password (`blips_admin`) | K8s Secret + configmap | Restart API + Worker + Keycloak | Tinggi |
| Redis password | K8s Secret | Restart API + Worker | Sedang |
| MinIO access key + secret | K8s Secret | Restart API + Worker + Loki | Sedang |
| Keycloak admin password (`KC_BOOTSTRAP_ADMIN_PASSWORD`) | K8s Secret | No restart needed | Rendah |
| Grafana admin password | K8s Secret | No restart needed | Rendah |

---

## 2. Rotasi JWT signing key (Keycloak)

```bash
# 2.1 Login ke Keycloak admin console
# https://auth.blips.tugu-re.com
# Realm: blips → Keys → Providers

# 2.2 Generate RSA-2048 key baru (via Keycloak UI atau API)
KEYCLOAK_TOKEN=$(curl -s -X POST \
  https://auth.blips.tugu-re.com/realms/master/protocol/openid-connect/token \
  -d "client_id=admin-cli&username=admin&password=${KC_ADMIN_PASSWORD}&grant_type=password" \
  | jq -r '.access_token')

# Tambah provider RSA baru dengan priority lebih tinggi
curl -s -X POST \
  https://auth.blips.tugu-re.com/admin/realms/blips/components \
  -H "Authorization: Bearer ${KEYCLOAK_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "rsa-generated-v2",
    "providerId": "rsa-generated",
    "providerType": "org.keycloak.keys.KeyProvider",
    "config": {
      "priority": ["200"],
      "keySize": ["2048"],
      "active": ["true"]
    }
  }'

# 2.3 Ambil public key baru
curl -s https://auth.blips.tugu-re.com/realms/blips/protocol/openid-connect/certs | jq .

# 2.4 Update K8s secret dengan public key baru (PEM format)
NEW_PEM=$(bash scripts/keycloak-get-pubkey.sh)
kubectl create secret generic blips-secret -n blips \
  --from-literal=JWT_PUBLIC_KEY_PEM="${NEW_PEM}" \
  --dry-run=client -o yaml | kubectl apply -f -

# 2.5 Restart API pods (rolling, tidak ada downtime)
kubectl rollout restart deployment/blips-api -n blips
kubectl rollout status deployment/blips-api -n blips --timeout=120s

# 2.6 Disable key lama di Keycloak (biarkan tetap ada tapi non-active, untuk validasi token lama)
# Setelah 24 jam (semua token expired), hapus key lama

# 2.7 Verifikasi
curl -fs https://api.blips.tugu-re.com/healthz
```

---

## 3. Rotasi PostgreSQL password

```bash
# 3.1 Generate password baru (min 32 karakter)
NEW_PG_PASSWORD=$(openssl rand -base64 32)

# 3.2 Update password di PostgreSQL
kubectl exec -n blips postgres-0 -- \
  psql -U blips_admin -d blips_db -c \
  "ALTER USER blips_admin PASSWORD '${NEW_PG_PASSWORD}';"

# 3.3 Update K8s secret
kubectl create secret generic blips-secret -n blips \
  --from-literal=POSTGRES_PASSWORD="${NEW_PG_PASSWORD}" \
  --dry-run=client -o yaml | kubectl apply -f -

# 3.4 Rolling restart semua service yang menggunakan DB
kubectl rollout restart deployment/blips-api deployment/blips-worker deployment/keycloak -n blips

# 3.5 Verifikasi koneksi
kubectl exec -n blips deployment/blips-api -- \
  curl -s http://localhost:8080/readyz

# 3.6 Simpan password baru di vault/password manager ROLE-IT-ADMIN
# JANGAN simpan di file teks atau email

# 3.7 Update .env.uat di UAT host jika relevant
```

---

## 4. Rotasi Redis password

```bash
# 4.1 Generate password baru
NEW_REDIS_PASSWORD=$(openssl rand -base64 24)

# 4.2 Update Redis dengan command CONFIG SET (tanpa restart)
kubectl exec -n blips redis-0 -- \
  redis-cli -a "${CURRENT_REDIS_PASSWORD}" CONFIG SET requirepass "${NEW_REDIS_PASSWORD}"

# 4.3 Update K8s secret
kubectl create secret generic blips-secret -n blips \
  --from-literal=REDIS_PASSWORD="${NEW_REDIS_PASSWORD}" \
  --dry-run=client -o yaml | kubectl apply -f -

# 4.4 Rolling restart aplikasi
kubectl rollout restart deployment/blips-api deployment/blips-worker -n blips
kubectl rollout status deployment/blips-api -n blips --timeout=120s
```

---

## 5. Rotasi MinIO access key

```bash
# 5.1 Login ke MinIO console atau CLI
mc alias set minio-prod https://minio.blips.tugu-re.com \
  "${CURRENT_MINIO_USER}" "${CURRENT_MINIO_PASSWORD}"

# 5.2 Buat service account baru dengan policy yang sama
NEW_MINIO_USER="blips_app_$(date +%Y%m)"
NEW_MINIO_PASSWORD=$(openssl rand -base64 32)

mc admin user add minio-prod ${NEW_MINIO_USER} ${NEW_MINIO_PASSWORD}
mc admin policy attach minio-prod blips-readwrite --user ${NEW_MINIO_USER}

# 5.3 Update K8s secret dengan credential baru
kubectl create secret generic blips-secret -n blips \
  --from-literal=MINIO_ROOT_USER="${NEW_MINIO_USER}" \
  --from-literal=MINIO_ROOT_PASSWORD="${NEW_MINIO_PASSWORD}" \
  --dry-run=client -o yaml | kubectl apply -f -

# 5.4 Restart aplikasi
kubectl rollout restart deployment/blips-api deployment/blips-worker -n blips
kubectl rollout status deployment/blips-api -n blips --timeout=120s

# 5.5 Verifikasi upload berhasil
kubectl exec -n blips deployment/blips-api -- \
  curl -s http://localhost:8080/healthz

# 5.6 Hapus user lama
mc admin user remove minio-prod ${CURRENT_MINIO_USER}
```

---

## 6. Rollback procedure

Jika layanan gagal setelah rotasi:

```bash
# 6.1 Restore secret ke nilai lama (simpan selalu sebelum rotasi!)
kubectl create secret generic blips-secret -n blips \
  --from-literal=<KEY>="${OLD_VALUE}" \
  --dry-run=client -o yaml | kubectl apply -f -

# 6.2 Restart pods
kubectl rollout restart deployment/blips-api deployment/blips-worker -n blips

# 6.3 Investigasi mengapa rotasi gagal (lihat pod logs)
kubectl logs deployment/blips-api -n blips --since=5m
```

---

## 7. Checklist rotasi kuartalan

Lakukan berurutan, verifikasi setelah setiap langkah:

- [ ] Backup current secrets ke vault sebelum mulai
- [ ] Rotasi PostgreSQL password (§3)
- [ ] Rotasi Redis password (§4)
- [ ] Rotasi MinIO access key (§5)
- [ ] Rotasi JWT signing key Keycloak (§2) — di-lakukan terakhir karena paling impactful
- [ ] Verifikasi semua health endpoints
- [ ] Update .env.uat di UAT host
- [ ] Update dokumentasi tanggal rotasi terakhir di file ini
- [ ] Sign-off: ROLE-IT-ADMIN + security-engineer

**Tanggal rotasi terakhir**: [diisi setelah eksekusi]

---

## 8. Referensi

- Security baseline: `.claude/memory/security-baseline.md §Secrets management`
- DEC-028: encryption at rest
- DEC-025: JWT RSA-2048
