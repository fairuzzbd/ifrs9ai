# BLIPS IFRS 9

**BLIPS** (Business Logic & Impairment Processing System) adalah aplikasi PSAK 71 / IFRS 9
untuk PT Tugu Reasuransi Indonesia (Tugure): klasifikasi instrumen (SPPI/BM), Effective Interest
Rate (EIR) & amortisasi, dan Expected Credit Loss (ECL) 3-stage, lengkap dengan audit trail,
workflow maker-reviewer-approver, dan jurnal internal.

> Deployment **on-premise** di Jakarta DC (UU PDP data residency — DEC-002/DEC-008).
> Integrasi GL host **ditangguhkan ke Phase 2**; Phase 1 memakai export file batch CSV/XLSX (DEC-005).

## Tech stack (LOCKED — DEC-001..004)

| Layer | Teknologi |
|---|---|
| Backend | Go 1.22+, Gin, sqlx, Asynq (Redis jobs) |
| Frontend | Next.js 14 (App Router, TypeScript strict), Tailwind, shadcn/ui |
| Database | PostgreSQL 18 |
| Cache / Queue | Redis 7 |
| Object storage | MinIO (S3-compatible) |

Uang selalu memakai `shopspring/decimal` dan kolom `NUMERIC` — **tidak pernah** float.

## Struktur repo

```
ifrs9ai/
├── START_HERE.md              # Panduan implementasi (baca dulu)
├── BLIPS_init_schema.sql      # DDL baseline (9 schema, ~50 tabel, seed data)
├── docs/                      # Spesifikasi (Decision Log, SoW, FSD, ERD)
├── backend/                   # Go backend (Gin/sqlx)
├── frontend/                  # Next.js 14
├── deploy/docker/             # docker-compose dev environment
└── .gitlab-ci.yml             # Pipeline CI/CD (skeleton Phase 0)
```

## Quickstart (Phase 0 dev environment)

Prasyarat: Docker Engine 24+ dan Docker Compose v2. Tidak perlu install Go/Node/PostgreSQL
secara lokal untuk menjalankan stack — semuanya berjalan dalam container.

```bash
# 1. Clone
git clone <repo-url> ifrs9ai
cd ifrs9ai

# 2. Jalankan seluruh stack (postgres + redis + minio + backend + frontend)
#    PostgreSQL otomatis mengeksekusi BLIPS_init_schema.sql saat pertama kali start.
docker compose -f deploy/docker/docker-compose.dev.yml up -d

# 3. Pantau sampai semua service healthy
docker compose -f deploy/docker/docker-compose.dev.yml ps
```

### Verifikasi

| Layanan | URL / perintah | Harapan |
|---|---|---|
| Backend health | `curl http://localhost:8080/healthz` | `{"status":"ok","timestamp":"..."}` |
| Frontend | buka http://localhost:3000 | halaman menampilkan status backend |
| MinIO console | buka http://localhost:9001 | login console MinIO |
| PostgreSQL | `psql postgres://blips_admin@localhost:5432/blips_db` | koneksi sukses |

Menghentikan & membersihkan:

```bash
docker compose -f deploy/docker/docker-compose.dev.yml down        # stop
docker compose -f deploy/docker/docker-compose.dev.yml down -v     # stop + hapus volume data
```

## Konfigurasi URL API frontend per-environment (build-arg)

Frontend Next.js memakai `NEXT_PUBLIC_API_URL` untuk menentukan base URL backend.
Variabel berprefix `NEXT_PUBLIC_` **di-inline ke bundle saat `next build`** — nilainya
ter-bake permanen ke image, bukan dibaca saat container start. Karena itu URL harus
di-set **saat build**, bukan hanya saat runtime, agar satu Dockerfile bisa menghasilkan
image untuk dev / UAT / produksi dengan URL berbeda.

`frontend/Dockerfile` menerima ini lewat `ARG NEXT_PUBLIC_API_URL` (default
`http://localhost:8080`) yang di-promote ke `ENV` di stage builder sebelum `pnpm build`.

**Build manual per-environment:**

```bash
# UAT
docker build \
  --build-arg NEXT_PUBLIC_API_URL=https://api.uat.tugure.example \
  -t blips-web:uat frontend/

# Produksi
docker build \
  --build-arg NEXT_PUBLIC_API_URL=https://api.blips.tugure.example \
  -t blips-web:prod frontend/
```

**Lewat docker-compose** — service `frontend` meneruskan nilai via `build.args`, dengan
default dev tetap `http://localhost:8080`. Override dengan env shell sebelum build:

```bash
# Dev (default, tanpa override)
docker compose -f deploy/docker/docker-compose.dev.yml up -d --build

# Override untuk environment lain
NEXT_PUBLIC_API_URL=https://api.uat.tugure.example \
  docker compose -f deploy/docker/docker-compose.dev.yml build frontend
```

Verifikasi nilai benar ter-bake (URL muncul di bundle hasil build):

```bash
# Cek bundle lokal setelah `NEXT_PUBLIC_API_URL=... pnpm build` di frontend/
grep -r "api.uat.tugure.example" frontend/.next/static | head
```

### CORS backend harus selaras per-environment

Backend (`CORS_ALLOWED_ORIGINS`, comma-separated) harus mengizinkan **origin frontend**
di tiap environment. Pasangkan keduanya saat deploy:

| Environment | Frontend `NEXT_PUBLIC_API_URL` (→ backend) | Backend `CORS_ALLOWED_ORIGINS` (origin frontend) |
|---|---|---|
| Dev | `http://localhost:8080` | `http://localhost:3000` |
| UAT | `https://api.uat.tugure.example` | `https://app.uat.tugure.example` |
| Produksi | `https://api.blips.tugure.example` | `https://app.blips.tugure.example` |

Set `CORS_ALLOWED_ORIGINS` lewat env backend (lihat `backend/.env.example`). Jika tidak
selaras, browser memblokir request lintas-origin meski URL API sudah benar.

## Kredensial default (DEV — NON-PRODUKSI)

> Kredensial di bawah ini **hanya untuk dev lokal**. JANGAN dipakai di UAT/produksi.
> Produksi memakai secrets manager dan password rotated (lihat `.claude/memory/security-baseline.md`).

| Komponen | User | Password |
|---|---|---|
| PostgreSQL | `blips_admin` | `change_me_in_production` |
| MinIO | `minioadmin` | `minioadmin` |

## Dokumentasi

- [`START_HERE.md`](START_HERE.md) — entry point implementasi, urutan phase, prompt templates.
- [`docs/`](docs/) — Decision Log, SoW, FSD per modul (APP-A..E), ERD.
- Decision Log meringkas keputusan terkunci (DEC-001..029); jangan di-override tanpa CR formal.

## Lisensi

Proprietary — PT Tugu Reasuransi Indonesia. Internal use only.
