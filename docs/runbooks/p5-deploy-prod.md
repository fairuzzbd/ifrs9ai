# Runbook: Deploy BLIPS ke Production (Kubernetes)

**Versi**: P5-M18  
**Lingkup**: Production on-premise Jakarta DC, Kubernetes cluster  
**Prereq**: `kubectl` access dengan ROLE-IT-ADMIN, release tag sudah ada di Git, UAT sign-off selesai

---

## 1. Pre-deploy checklist

- [ ] Release tag `v{MAJOR}.{MINOR}.{PATCH}` sudah dibuat dan signed di Git
- [ ] UAT deploy berhasil (lihat `p5-deploy-uat.md`)
- [ ] QA sign-off (qa-engineer approval di PR)
- [ ] Compliance review (ifrs9-compliance-reviewer approval jika ECL/EIR berubah)
- [ ] Security review (security-engineer approval)
- [ ] Backup PostgreSQL terbaru tersedia (cek `p5-backup-restore.md §1`)
- [ ] Change window: business hours (09:00-17:00 WIB Senin-Jumat)
- [ ] On-call engineer standby

## 2. Setup namespace dan secrets

```bash
# 2.1 Buat namespace (jika belum ada)
kubectl apply -f deploy/k8s/namespace.yaml

# 2.2 Inject secrets (JANGAN apply secret.example.yaml dengan nilai placeholder)
# Pastikan password sudah tersimpan di vault / password manager ROLE-IT-ADMIN
kubectl create secret generic blips-secret \
  --namespace=blips \
  --from-literal=POSTGRES_PASSWORD="${POSTGRES_PASSWORD}" \
  --from-literal=REDIS_PASSWORD="${REDIS_PASSWORD}" \
  --from-literal=MINIO_ROOT_USER="${MINIO_ROOT_USER}" \
  --from-literal=MINIO_ROOT_PASSWORD="${MINIO_ROOT_PASSWORD}" \
  --from-literal=JWT_PUBLIC_KEY_PEM="${BLIPS_JWT_PUBLIC_KEY_PEM}" \
  --from-literal=KC_BOOTSTRAP_ADMIN_PASSWORD="${KC_BOOTSTRAP_ADMIN_PASSWORD}" \
  --from-literal=GRAFANA_ADMIN_PASSWORD="${GRAFANA_ADMIN_PASSWORD}" \
  --save-config \
  --dry-run=client -o yaml | kubectl apply -f -

# Verifikasi secret ada
kubectl get secret blips-secret -n blips
```

## 3. Apply konfigurasi

```bash
# 3.1 Network policies (default deny)
kubectl apply -f deploy/k8s/network-policy.yaml

# 3.2 ConfigMap
kubectl apply -f deploy/k8s/configmap.yaml

# 3.3 Observability config
kubectl apply -f deploy/k8s/prometheus/configmap.yaml

# Verifikasi
kubectl get configmap -n blips
```

## 4. Deploy stateful services (berurutan)

```bash
# 4.1 PostgreSQL primary
kubectl apply -f deploy/k8s/postgres/statefulset.yaml
kubectl apply -f deploy/k8s/postgres/service.yaml
kubectl rollout status statefulset/postgres -n blips --timeout=300s

# 4.2 PostgreSQL replica (setelah primary ready)
kubectl rollout status statefulset/postgres-replica -n blips --timeout=300s

# 4.3 Redis
kubectl apply -f deploy/k8s/redis/statefulset.yaml
kubectl rollout status statefulset/redis -n blips --timeout=120s

# 4.4 MinIO (4 nodes, parallel launch)
kubectl apply -f deploy/k8s/minio/statefulset.yaml
kubectl rollout status statefulset/minio -n blips --timeout=300s

# 4.5 Keycloak
kubectl apply -f deploy/k8s/keycloak/deployment.yaml
kubectl rollout status deployment/keycloak -n blips --timeout=300s
```

## 5. Jalankan schema migration (Kubernetes Job)

```bash
# Apply migrator Job (one-shot)
# TODO: tambahkan deploy/k8s/migrator-job.yaml di iterasi berikutnya
# Sementara, jalankan via kubectl run:
kubectl run blips-migrator --restart=Never -n blips \
  --image=migrate/migrate:v4.18.1 \
  --env="DATABASE_URL=postgres://blips_admin:${POSTGRES_PASSWORD}@postgres:5432/blips_db?sslmode=disable" \
  -- -path=/migrations -database "${DATABASE_URL}" up

# Monitor job
kubectl logs blips-migrator -n blips -f

# Cleanup
kubectl delete pod blips-migrator -n blips
```

## 6. Deploy application (update image tag)

```bash
# 6.1 Set SHA tag dari release
export RELEASE_TAG=<sha8>

# 6.2 Update image di deployment YAML (atau gunakan kubectl set image)
kubectl set image deployment/blips-api \
  api=registry.tugu-re.local/blips/backend:${RELEASE_TAG} \
  -n blips

kubectl set image deployment/blips-worker \
  worker=registry.tugu-re.local/blips/worker:${RELEASE_TAG} \
  -n blips

kubectl set image deployment/blips-frontend \
  frontend=registry.tugu-re.local/blips/frontend:${RELEASE_TAG} \
  -n blips

# 6.3 Atau apply full manifest
kubectl apply -f deploy/k8s/api/
kubectl apply -f deploy/k8s/worker/
kubectl apply -f deploy/k8s/frontend/

# 6.4 Monitor rolling update
kubectl rollout status deployment/blips-api -n blips --timeout=300s
kubectl rollout status deployment/blips-worker -n blips --timeout=300s
kubectl rollout status deployment/blips-frontend -n blips --timeout=300s
```

## 7. Deploy ingress

```bash
# RBAC Traefik
kubectl apply -f deploy/k8s/traefik/deployment.yaml
kubectl rollout status deployment/traefik -n blips --timeout=120s

# IngressRoute
kubectl apply -f deploy/k8s/traefik/ingressroute.yaml
```

## 8. Blue-green traffic shift via Traefik weight

Untuk zero-downtime deployment, gunakan dua service dan shift weight:

```bash
# Contoh: shift 10% → 50% → 100% ke versi baru
# Edit ingressroute.yaml: tambah services array dengan weight
# weight: 10  (versi baru) + weight: 90 (versi lama)
kubectl apply -f deploy/k8s/traefik/ingressroute.yaml

# Monitor error rate di Grafana selama 5 menit
# Jika OK, shift ke 50/50, lalu 100/0
```

## 9. Deploy observability

```bash
kubectl apply -f deploy/k8s/prometheus/
kubectl apply -f deploy/k8s/grafana/
kubectl apply -f deploy/k8s/loki/

kubectl rollout status deployment/prometheus -n blips --timeout=120s
kubectl rollout status deployment/grafana -n blips --timeout=120s
kubectl rollout status statefulset/loki -n blips --timeout=120s
```

## 10. Smoke test prod

```bash
# Tunggu 2 menit setelah semua pods running
sleep 120

# Health endpoints via Traefik
curl -fs https://api.blips.tugu-re.com/healthz && echo "OK" || echo "FAIL"
curl -fs https://api.blips.tugu-re.com/readyz && echo "OK" || echo "FAIL"
curl -fs https://blips.tugu-re.com && echo "OK: frontend" || echo "FAIL: frontend"
curl -fs https://auth.blips.tugu-re.com/health/ready && echo "OK: keycloak" || echo "FAIL: keycloak"
```

## 11. Rollback procedure

```bash
# Opsi 1: Rollback deployment ke revision sebelumnya
kubectl rollout undo deployment/blips-api -n blips
kubectl rollout undo deployment/blips-worker -n blips
kubectl rollout undo deployment/blips-frontend -n blips

# Opsi 2: Set image ke tag sebelumnya
kubectl set image deployment/blips-api \
  api=registry.tugu-re.local/blips/backend:${PREV_TAG} \
  -n blips

# Rollback schema migration (hati-hati — cek impact ke data)
# Gunakan hanya jika down.sql telah ditest di UAT
# kubectl run blips-migrator-down ... -- down 1
```

## 12. Post-deploy

- [ ] Semua pods Running dan Ready di `kubectl get pods -n blips`
- [ ] Grafana `blips-overview` menampilkan metrics (RPS, latency, error rate)
- [ ] Loki: logs masuk dengan label `service=blips-api,env=production`
- [ ] Alert rules aktif di Prometheus (`/rules` endpoint)
- [ ] Notifikasi ke stakeholder via email: "BLIPS v{VERSION} deployed to production"
- [ ] Update incident log jika ada issue saat deploy

## 13. Referensi

- Secrets: `p5-secrets-rotation.md`
- DR: `p5-disaster-recovery.md`
- Incident: `p5-incident-response.md`
- Branch protection: `github-branch-protection.md`
