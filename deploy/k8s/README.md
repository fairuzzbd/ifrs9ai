# BLIPS IFRS9 — Kubernetes Production Manifests

## Deploy order

Apply manifests in this exact sequence to respect dependencies:

```bash
# 1. Namespace
kubectl apply -f deploy/k8s/namespace.yaml

# 2. RBAC + ServiceAccounts (traefik, prometheus)
kubectl apply -f deploy/k8s/traefik/deployment.yaml    # includes SA + RBAC
kubectl apply -f deploy/k8s/prometheus/deployment.yaml  # includes SA + RBAC

# 3. ConfigMaps
kubectl apply -f deploy/k8s/configmap.yaml
kubectl apply -f deploy/k8s/prometheus/configmap.yaml
kubectl apply -f deploy/k8s/grafana/deployment.yaml    # includes dashboard ConfigMap ref

# 4. Secrets (kubectl create secret — NOT kubectl apply secret.example.yaml)
# Lihat: docs/runbooks/p5-deploy-prod.md §Secrets

# 5. Network policies (default-deny first)
kubectl apply -f deploy/k8s/network-policy.yaml

# 6. Stateful services
kubectl apply -f deploy/k8s/postgres/statefulset.yaml
kubectl apply -f deploy/k8s/postgres/service.yaml
kubectl apply -f deploy/k8s/redis/statefulset.yaml
kubectl apply -f deploy/k8s/minio/statefulset.yaml

# 7. Auth
kubectl apply -f deploy/k8s/keycloak/deployment.yaml

# 8. Run migrations (Job)
# kubectl apply -f deploy/k8s/migrator-job.yaml  # TODO: add migrator Job manifest

# 9. Application
kubectl apply -f deploy/k8s/api/
kubectl apply -f deploy/k8s/worker/
kubectl apply -f deploy/k8s/frontend/

# 10. Ingress
kubectl apply -f deploy/k8s/traefik/ingressroute.yaml

# 11. Observability
kubectl apply -f deploy/k8s/prometheus/
kubectl apply -f deploy/k8s/grafana/
kubectl apply -f deploy/k8s/loki/
```

## Secrets injection

Lihat `deploy/k8s/secret.example.yaml` untuk daftar secret keys. Jangan apply
file example tersebut — gunakan `kubectl create secret` atau HashiCorp Vault
Agent Injector (Phase 2, DEC-008).

## Storage

StorageClass `local-path` diasumsikan tersedia (Rancher local-path-provisioner
atau ekuivalen). Untuk Ceph/Rook: ganti `storageClassName: local-path` ke
`storageClassName: rook-ceph-block` di semua volumeClaimTemplates.

Ukuran storage:
| Service        | PVC size |
|----------------|----------|
| PostgreSQL     | 500 Gi (primary + replica) |
| Redis          | 50 Gi    |
| MinIO (x4)     | 2 Ti each = 8 Ti total |
| Prometheus     | 200 Gi   |
| Grafana        | 20 Gi    |
| Loki           | 500 Gi   |

## Helm chart

TODO Phase 6: konversi manifests ini ke Helm chart di `deploy/helm/blips/`.
Referensi DEC-008.

## Image tags

Semua image tag di file ini adalah placeholder `REPLACE_WITH_SHA_TAG`. CI pipeline
mengisi tag SHA8 immutable saat deploy. Lihat `.github/workflows/ci.yml` job
`deploy-uat` sebagai referensi pola yang sama untuk prod.

Anti-pattern: jangan pakai `:latest` — lihat DEC-008 locked decision.
