# Runbook: Deploy BLIPS ke UAT

**Versi**: P5-M18  
**Lingkup**: UAT on-premise (Docker Compose), Jakarta DC  
**Prereq**: SSH access ke UAT host, `.env.uat` sudah tersimpan di `/opt/blips/secrets/.env.uat` (chmod 600)

---

## 1. Persiapan sebelum deploy

```bash
# 1.1 Pastikan .env.uat ada dan izin benar
ls -la /opt/blips/secrets/.env.uat
# Expected: -rw------- 1 deploy deploy ... .env.uat

# 1.2 Cek free disk (min 20GB required untuk images + volumes)
df -h /var/lib/docker

# 1.3 Verifikasi koneksi registry internal
docker login registry.tugu-re.local
```

## 2. Pull latest code

```bash
cd /opt/blips/ifrs9ai
git fetch origin develop
git status
# Pastikan clean working tree
git pull origin develop
```

## 3. Pull images (tag dari CI pipeline)

```bash
# Set image tag — ambil dari output CI job atau GitHub Actions summary
export BLIPS_SHA=<sha8 dari CI>

# Update .env.uat dengan SHA tag baru
sed -i "s|BLIPS_BACKEND_IMAGE=.*|BLIPS_BACKEND_IMAGE=registry.tugu-re.local/blips/backend:uat-${BLIPS_SHA}|" /opt/blips/secrets/.env.uat
sed -i "s|BLIPS_WORKER_IMAGE=.*|BLIPS_WORKER_IMAGE=registry.tugu-re.local/blips/worker:uat-${BLIPS_SHA}|" /opt/blips/secrets/.env.uat
sed -i "s|BLIPS_FRONTEND_IMAGE=.*|BLIPS_FRONTEND_IMAGE=registry.tugu-re.local/blips/frontend:uat-${BLIPS_SHA}|" /opt/blips/secrets/.env.uat

# Pull images
docker compose -f /opt/blips/ifrs9ai/deploy/docker/docker-compose.uat.yml \
  --env-file /opt/blips/secrets/.env.uat \
  pull
```

## 4. Jalankan schema migration

```bash
# Jalankan migrator sebelum services up
docker compose -f /opt/blips/ifrs9ai/deploy/docker/docker-compose.uat.yml \
  --env-file /opt/blips/secrets/.env.uat \
  run --rm migrator up

# Verifikasi migration berhasil
echo "Exit code: $?"
# Expected: 0
```

## 5. Deploy services

```bash
docker compose -f /opt/blips/ifrs9ai/deploy/docker/docker-compose.uat.yml \
  --env-file /opt/blips/secrets/.env.uat \
  up -d --remove-orphans

# Tunggu semua services healthy (max 3 menit)
sleep 30
docker compose -f /opt/blips/ifrs9ai/deploy/docker/docker-compose.uat.yml \
  --env-file /opt/blips/secrets/.env.uat \
  ps
```

## 6. Smoke test

```bash
UAT_HOST="${UAT_DOMAIN:-uat.blips.tugu-re.local}"

# 6.1 Backend healthz
curl -fs http://localhost:8081/healthz && echo "OK: /healthz" || echo "FAIL: /healthz"

# 6.2 Backend readyz
curl -fs http://localhost:8081/readyz && echo "OK: /readyz" || echo "FAIL: /readyz"

# 6.3 Frontend
curl -fs http://localhost:3101 && echo "OK: frontend" || echo "FAIL: frontend"

# 6.4 Keycloak health
curl -fs http://localhost:8181/health/ready && echo "OK: keycloak" || echo "FAIL: keycloak"

# 6.5 MinIO health
curl -fs http://localhost:9100/minio/health/live && echo "OK: minio" || echo "FAIL: minio"
```

## 7. Checklist post-deploy

- [ ] Semua services berstatus `healthy` di `docker compose ps`
- [ ] `/healthz` dan `/readyz` backend return 200
- [ ] Login via Keycloak berhasil dengan user test UAT
- [ ] `aud.audit_log` dapat di-query (test via ROLE-AUDIT login)
- [ ] Jalankan smoke test APP-A: GET /api/v1/master/instrumen → 200 dengan pagination
- [ ] Cek Loki: log sudah masuk dengan label `service=blips-api,env=uat`
- [ ] Cek Grafana dashboard `blips-overview`: metrics muncul
- [ ] Backup .env.uat ke secure vault offline

## 8. Rollback

```bash
# Set tag ke versi sebelumnya
export PREV_SHA=<sha8 sebelumnya>
sed -i "s|uat-${BLIPS_SHA}|uat-${PREV_SHA}|g" /opt/blips/secrets/.env.uat

# Re-deploy dengan image lama
docker compose -f /opt/blips/ifrs9ai/deploy/docker/docker-compose.uat.yml \
  --env-file /opt/blips/secrets/.env.uat \
  up -d --remove-orphans

# Rollback migration jika schema berubah
docker compose -f /opt/blips/ifrs9ai/deploy/docker/docker-compose.uat.yml \
  --env-file /opt/blips/secrets/.env.uat \
  run --rm migrator down 1
```

## 9. Referensi

- Lihat `docs/runbooks/phase-0-smoke-test.md` untuk smoke test detail APP-A/B/C/D/E.
- Masalah secrets: `docs/runbooks/p5-secrets-rotation.md`
- Incident: `docs/runbooks/p5-incident-response.md`
