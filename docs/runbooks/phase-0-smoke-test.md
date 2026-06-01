# Runbook: Phase 0 Smoke Test — BLIPS IFRS9

**Versi:** 1.0  
**Tanggal:** 2026-06-02  
**Penulis:** devops-engineer  
**Target:** IT Admin / Developer yang melakukan verifikasi Quality Gate Phase 0 → 1

---

## 1. Tujuan

Runbook ini memandu verifikasi tiga quality gate runtime yang wajib lulus sebelum Phase 0 dinyatakan selesai dan Phase 2 (Foundation Layer) dapat dimulai, sesuai `START_HERE.md` §7 "Phase 0 → 1":

| Gate | Kriteria |
|------|----------|
| **Gate 1** | PostgreSQL DDL execute clean — 9 schema terbentuk, seed data masuk tanpa error |
| **Gate 2** | Backend `/healthz` return HTTP 200 dengan JSON valid |
| **Gate 3** | Frontend bisa fetch `/healthz` cross-origin (CORS header hadir, UI render) |

---

## 2. Prerequisites

Sebelum menjalankan runbook ini, pastikan semua item berikut tersedia:

- **Docker Engine 24+** dan **Docker Compose v2** terinstall
  ```bash
  docker --version        # Docker version 24.x atau lebih baru
  docker compose version  # Docker Compose version v2.x
  ```
- **`psql` CLI** (opsional — bisa diganti `docker compose exec postgres psql`)
  ```bash
  psql --version   # PostgreSQL 18.x client
  ```
- **`curl`** dan **`jq`** tersedia di PATH
  ```bash
  curl --version
  jq --version
  ```
- **Browser modern** dengan DevTools (Chrome / Firefox)
- **Working directory**: root repo BLIPS IFRS9
  ```bash
  cd /home/tugure/projects/ifrs9ai
  ```
- **Migration files tersedia** (dibuat oleh `data-modeler` agent):
  - `db/migrations/000001_init_schema.up.sql`
  - `db/migrations/000002_seed_data_dev.up.sql`

---

## 3. Gate 1 — PostgreSQL DDL Execute Clean

### Langkah 3.1 — Clean state (hapus volume lama jika ada)

```bash
docker compose -f deploy/docker/docker-compose.dev.yml down -v
```

**Expected output:**
```
[+] Running 4/4
 ✔ Container blips-postgres  Removed
 ✔ Volume blips-pgdata       Removed
 ✔ Volume blips-redisdata    Removed
 ✔ Volume blips-miniodata    Removed
```

Jika tidak ada container/volume yang berjalan, output akan menampilkan "0/0" — itu normal.

### Langkah 3.2 — Jalankan postgres saja

```bash
docker compose -f deploy/docker/docker-compose.dev.yml up -d postgres
```

**Expected output:**
```
[+] Running 1/1
 ✔ Container blips-postgres  Started
```

### Langkah 3.3 — Tunggu hingga healthy

```bash
docker compose -f deploy/docker/docker-compose.dev.yml ps postgres
```

**Expected output (tunggu hingga STATUS berubah):**
```
NAME             IMAGE        STATUS                 PORTS
blips-postgres   postgres:18  Up X seconds (healthy) 0.0.0.0:5432->5432/tcp
```

Jika status masih `starting`, tunggu 30 detik lalu ulangi perintah di atas.

### Langkah 3.4 — Verifikasi 9 schema terbentuk

```bash
docker compose -f deploy/docker/docker-compose.dev.yml exec postgres \
  psql -U blips_admin -d blips_db -c "\dn"
```

**Expected output:**
```
  List of schemas
  Name  |   Owner
--------+-------------
 aud    | blips_admin
 doc    | blips_admin
 ecl    | blips_admin
 jrnl   | blips_admin
 mst    | blips_admin
 public | pg_database_owner
 sec    | blips_admin
 sppi   | blips_admin
 sys    | blips_admin
 trx    | blips_admin
(10 rows)
```

Harus ada tepat 9 schema custom (aud, doc, ecl, jrnl, mst, sec, sppi, sys, trx) selain `public`.

### Langkah 3.5 — Verifikasi seed data masuk

```bash
docker compose -f deploy/docker/docker-compose.dev.yml exec postgres \
  psql -U blips_admin -d blips_db -c "SELECT count(*) FROM mst.mata_uang"
```

**Expected output:**
```
 count
-------
     8
(1 row)
```

```bash
docker compose -f deploy/docker/docker-compose.dev.yml exec postgres \
  psql -U blips_admin -d blips_db -c "SELECT count(*) FROM sec.role"
```

**Expected output:**
```
 count
-------
    10
(1 row)
```

### Troubleshooting Gate 1

**Jika postgres tidak mencapai `healthy`:**
```bash
docker compose -f deploy/docker/docker-compose.dev.yml logs postgres --tail=50
```
Cari baris `ERROR:` atau `FATAL:`. Error umum:
- `could not open file ".../000001_init_schema.up.sql"` → file migration belum dibuat oleh `data-modeler` agent. Pastikan `db/migrations/000001_init_schema.up.sql` ada di repo.
- `syntax error at or near "\"` → file mengandung `\echo` psql metacommand yang tidak valid di entrypoint. Cek file migration tidak mengandung `\echo`.

**Jika schema tidak lengkap (< 9 schema):**
```bash
docker compose -f deploy/docker/docker-compose.dev.yml logs postgres | grep -i "error\|fatal"
```
Lihat baris error, kemudian perbaiki file migration dan ulangi dari Langkah 3.1 (wajib `down -v` dulu agar entrypoint re-execute).

---

## 4. Gate 2 — Backend `/healthz` Return 200

### Langkah 4.1 — Jalankan backend

```bash
docker compose -f deploy/docker/docker-compose.dev.yml up -d backend
```

**Expected output:**
```
[+] Running 1/1
 ✔ Container blips-backend  Started
```

### Langkah 4.2 — Tunggu backend siap

```bash
sleep 5
docker compose -f deploy/docker/docker-compose.dev.yml ps backend
```

**Expected:** STATUS `Up X seconds`.

### Langkah 4.3 — Cek endpoint `/healthz`

```bash
curl -fsS http://localhost:8080/healthz | jq
```

**Expected output:**
```json
{
  "status": "ok",
  "service": "blips-api",
  "version": "0.1.0",
  "timestamp": "2026-06-02T10:30:00+07:00"
}
```

HTTP status harus 200. Jika `curl` mengembalikan exit code non-zero, periksa log.

### Troubleshooting Gate 2

**Jika `curl: (7) Failed to connect`:**
```bash
docker compose -f deploy/docker/docker-compose.dev.yml logs backend --tail=50
```
Cari baris `ERROR` atau `panic`. Error umum:
- `failed to connect to database` → `DATABASE_URL` salah atau postgres belum healthy. Pastikan Gate 1 sudah pass sebelum menjalankan backend.
- `bind: address already in use` → port 8080 sudah dipakai proses lain. Jalankan `lsof -i :8080` untuk identifikasi proses.

**Jika response bukan JSON valid:**
Backend kemungkinan mengembalikan HTML error page. Cek log backend untuk stack trace.

---

## 5. Gate 3 — Frontend Cross-Origin Fetch

### Langkah 5.1 — Jalankan frontend

```bash
docker compose -f deploy/docker/docker-compose.dev.yml up -d frontend
```

**Expected output:**
```
[+] Running 1/1
 ✔ Container blips-frontend  Started
```

### Langkah 5.2 — Tunggu frontend build selesai

```bash
sleep 15
docker compose -f deploy/docker/docker-compose.dev.yml ps frontend
```

**Expected:** STATUS `Up X seconds`.

### Langkah 5.3 — Verifikasi CORS header via CLI

```bash
curl -fsS -H "Origin: http://localhost:3000" -I http://localhost:8080/healthz \
  | grep -i "access-control"
```

**Expected output:**
```
Access-Control-Allow-Origin: http://localhost:3000
```

Jika header tidak muncul, CORS middleware backend belum aktif (lihat Troubleshooting di bawah).

### Langkah 5.4 — Verifikasi via browser

1. Buka `http://localhost:3000` di Chrome atau Firefox.
2. Buka **DevTools** (F12) → tab **Network**.
3. Refresh halaman.
4. Cari request ke `http://localhost:8080/healthz` di daftar Network.
5. Verifikasi:
   - **Status**: `200`
   - Tab **Headers** → Response Headers → `Access-Control-Allow-Origin: http://localhost:3000`
6. Verifikasi UI: halaman menampilkan kartu "BLIPS Health Status" dengan `status: ok`, `service: blips-api`, `version: 0.1.0`.

### Troubleshooting Gate 3

**Jika browser menampilkan CORS error di console:**
```
Access to fetch at 'http://localhost:8080/healthz' from origin 'http://localhost:3000'
has been blocked by CORS policy
```
Cek environment variable backend:
```bash
docker compose -f deploy/docker/docker-compose.dev.yml exec backend env | grep CORS
```
Harus menghasilkan: `CORS_ALLOWED_ORIGINS=http://localhost:3000`

Jika variabel tidak ada atau salah, cek file `deploy/docker/docker-compose.dev.yml` bagian `environment` service `backend`.

**Jika frontend tidak bisa reach backend:**
```bash
docker compose -f deploy/docker/docker-compose.dev.yml logs frontend --tail=30
```
Pastikan `NEXT_PUBLIC_API_URL` ter-set ke `http://localhost:8080` (atau nilai yang sesuai saat build).

---

## 6. Tear-Down

Setelah verifikasi selesai, bersihkan semua container dan volume:

```bash
docker compose -f deploy/docker/docker-compose.dev.yml down -v
```

**Expected output:**
```
[+] Running 6/6
 ✔ Container blips-frontend  Removed
 ✔ Container blips-backend   Removed
 ✔ Container blips-minio     Removed
 ✔ Container blips-redis     Removed
 ✔ Container blips-postgres  Removed
 ✔ Volume blips-pgdata       Removed
 ...
```

Flag `-v` menghapus named volumes (pgdata, redisdata, miniodata) sehingga run berikutnya mulai dari state bersih.

---

## 7. Sign-Off Checklist

Centang setiap item setelah diverifikasi. Submit checklist ini ke tech-lead-orchestrator sebagai bukti Phase 0 → 1 gate clear.

- [ ] PostgreSQL DDL execute clean (tidak ada baris ERROR/FATAL di log postgres, 9 schema terbentuk)
- [ ] Backend `/healthz` return HTTP 200 dengan JSON valid (`status`, `service`, `version`, `timestamp` hadir)
- [ ] Frontend bisa fetch `/healthz` cross-origin (header `Access-Control-Allow-Origin: http://localhost:3000` hadir + UI render kartu health)
- [ ] CI pipeline green (cek GitLab pipeline untuk branch aktif — semua stage lint, test-unit, security-scan pass)
- [ ] First commit di Git (konfirmasi `git log --oneline -5` menampilkan commit baseline Phase 0)

---

## 8. Output Expectation

Setelah semua 5 item sign-off checklist di-tick, sampaikan ke **tech-lead-orchestrator**:

> "Phase 0 smoke test selesai. Semua 5 quality gate Phase 0 → 1 pass per runbook `docs/runbooks/phase-0-smoke-test.md`. Siap handoff ke Phase 2 Foundation Layer."

Phase 2 akan dimulai dengan: implementasi `sec` (auth/RBAC), `aud` (audit trail), workflow engine Maker-Reviewer-Approver, dan binary `cmd/migrator` menggunakan `golang-migrate/migrate/v4`.
