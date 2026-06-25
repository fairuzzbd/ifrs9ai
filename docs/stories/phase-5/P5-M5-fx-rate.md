# P5-M5 — APP-D FX Rate Management + BI JISDOR Job: User Stories

**Story Set ID**: P5-M5
**Modul**: APP-D — FX Rate Management (Phase 5, Sprint 1 paralel dengan P5-M2)
**Status**: DRAFT — menunggu handoff ke `system-analyst` + `integration-engineer`
**Author**: business-analyst
**Tanggal**: 2026-06-18
**Linked FSD**: FSD-APP-D-PeriodeBuku-FX-Mapping-v1.0.docx §2 (FX Rate Management)
**Linked BRD**: BRD §6.3 (APP-D FX Rate), RACI: ROLE-AKUN-CTL (A), ROLE-AKUN (R), ROLE-IT-ADMIN (C), ROLE-AUDIT (I)
**Linked Decision Log**:
- `DEC-016` (LOCKED) — `NUMERIC(20,8)` untuk FX rate (`kurs_tengah`, `kurs_beli`, `kurs_jual`); `shopspring/decimal` di Go — **never float64**
- `DEC-017` (LOCKED) — 4-eyes SoD wajib untuk manual upload: ROLE-AKUN (maker) ≠ ROLE-AKUN-CTL (approver)
- `DEC-018` (LOCKED) — audit trail append-only, retensi 10+10 tahun
- `DEC-021` (LOCKED) — Idempotency-Key wajib di setiap mutating endpoint
- `DEC-022` (LOCKED) — cursor-based pagination

**Dependensi**:
- **Phase 3** (`mst.mata_uang`, `mst.kurs`) — existing schema: `mst.kurs` sudah ada `locked_flag BOOLEAN NOT NULL DEFAULT FALSE`, trigger `fn_kurs_no_modify_when_locked`, `workflow_status` (migration 000020), UNIQUE constraint `(kode_mata_uang, tanggal_berlaku)`
- **P5-M4** — hard-close approve men-set `locked_flag = TRUE` untuk semua `mst.kurs` rows dalam periode yang di-close (lihat P5-M4-S3-AC1). P5-M5 menerima trigger ini — locked rows selanjutnya protected oleh DB trigger dan API middleware
- **P5-M6 (MTM)** — mengkonsumsi `mst.kurs` untuk konversi FCY → IDR pada MTM harian
- **Holiday Calendar** — Indonesia public holiday data di `sys.config` atau tabel `sys.holiday_calendar` (baru — migration 000039 scope)

**Handoff berikutnya**:
- `system-analyst` → OpenAPI fragment: 6 endpoints (`GET /master/kurs`, `GET /master/kurs/{id}`, `POST /master/kurs/upload`, `POST /master/kurs/{id}/approve`, `POST /master/kurs/{id}/reject`, `POST /master/kurs/jisdor-sync`, `GET /master/kurs/treatment/{instrumen_id}`); state machine `mst.kurs.workflow_status` (DRAFT → PENDING_APPROVAL → ACTIVE | REJECTED); error codes baru (lihat §Error Codes Proposed)
- `data-modeler` → migration 000039: (a) ADD COLUMN `status` VARCHAR(20) alias bersih untuk FX lifecycle (`ACTIVE | PENDING_APPROVAL | REJECTED | LOCKED`) dengan derived logic dari `workflow_status` + `locked_flag`; (b) CREATE TABLE `sys.holiday_calendar` untuk kalender libur nasional Indonesia; (c) ADD COLUMN `jisdor_fetch_metadata_jsonb` JSONB di `mst.kurs` untuk menyimpan raw JISDOR response + retry count; (d) ADD COLUMN `rate_deviation_pct` NUMERIC(8,4) untuk tracking deviasi dari hari sebelumnya
- `integration-engineer` → implementasi BI JISDOR Asynq cron worker: JISDOR URL scraping/API, holiday calendar check, idempotent insert, DLQ pattern (mirror P5-M3 DLQ), rate deviation alert
- `ifrs9-compliance-reviewer` → cek: FX gain/loss treatment routing (P&L vs OCI per klasifikasi PSAK 71) — path ini regulated; `AC → P&L`, `FVOCI debt → OCI`, `FVOCI Election → OCI no recycling`, `FVTPL → P&L`

**Compliance path**: P5-M5 sendiri bukan regulated path untuk ECL/EIR (advisory gate). Namun `GET /master/kurs/treatment/{instrumen_id}` (S5) menyentuh klasifikasi PSAK 71 routing → **ifrs9-compliance-reviewer BLOCKING** untuk endpoint treatment. Audit trail `KURS.*` standard — `security-engineer` advisory review.

---

## Konteks & Arsitektur P5-M5

### Alur FX Rate Harian

```
Hari kerja (Senin–Jumat, bukan hari libur nasional)
  │
  10:30 WIB — Asynq cron job "fx:jisdor_fetch"
  │    Fetch BI JISDOR rates (USD, EUR, JPY, SGD, AUD, GBP, CNY, dll)
  │    Idempotent: UNIQUE(kode_mata_uang, tanggal_berlaku) — skip jika sudah ada
  │    Insert mst.kurs row per mata uang:
  │       sumber_kurs = 'BI_JISDOR'
  │       workflow_status = 'APPROVED' (auto-approve, tidak perlu 4-eyes)
  │       locked_flag = FALSE
  │    Rate deviation check: |rate_hari_ini − rate_kemarin| / rate_kemarin > 20% → WARNING + alert
  │    Success: aud.audit_log KURS.JISDOR_FETCH per baris
  │    Failure: DLQ entry + alert ke ROLE-AKUN + ROLE-IT-ADMIN
  │
  Jika JISDOR fetch gagal (3× retry 15 menit) atau hari libur → Manual Upload
  │
  Manual Upload (ROLE-AKUN — Maker)
  │    POST /master/kurs/upload (multi-row XLSX/CSV per template)
  │    Validate: format, rate range, tidak ada duplikat tanggal
  │    Insert dengan workflow_status = 'PENDING_APPROVAL'
  │    Audit: KURS.MANUAL_UPLOAD_SUBMITTED
  │
  Approve (ROLE-AKUN-CTL — Approver, SoD: approver ≠ maker)
  │    POST /master/kurs/{id}/approve
  │    workflow_status = 'APPROVED' → rate aktif
  │    Audit: KURS.APPROVED in-transaction
  │
  Periode Hard-Close (trigger dari P5-M4)
  │    mst.kurs.locked_flag = TRUE untuk semua row dalam periode
  │    DB trigger fn_kurs_no_modify_when_locked memblok UPDATE/DELETE
  │    API middleware: 423 FX_RATE_LOCKED untuk mutasi row LOCKED
```

### State Machine `mst.kurs.workflow_status`

```
DRAFT (manual upload baru tersimpan)
  │
  ├─ [submit] ROLE-AKUN
  │    → workflow_status = 'PENDING_APPROVAL'
  │
  ├─ [jisdor_auto_approve] integration-worker (system actor)
  │    → workflow_status = 'APPROVED' langsung (bypass 4-eyes untuk feed otomatis)
  │
PENDING_APPROVAL
  │
  ├─ [approve] ROLE-AKUN-CTL (SoD: approver_id ≠ maker_id)
  │    → workflow_status = 'APPROVED'
  │    → rate mulai dikonsumsi sistem
  │
  ├─ [reject] ROLE-AKUN-CTL (dengan komentar wajib)
  │    → workflow_status = 'REJECTED'
  │    → maker notifikasi untuk re-upload
  │
APPROVED
  │
  ├─ [periode hard-close] trigger dari P5-M4 (system atau CFO action)
  │    → locked_flag = TRUE (tidak merubah workflow_status — sudah APPROVED)
  │    → UPDATE/DELETE diblok via DB trigger + API 423
  │
  ├─ [soft-delete] ROLE-AKUN atau ROLE-IT-ADMIN (hanya jika belum APPROVED)
  │    → deleted_at = now() — tidak boleh hard-delete
  │    → tidak bisa delete row APPROVED (error 403)
  │
REJECTED
  │
  └─ [re-upload] ROLE-AKUN — create row baru, bukan update yang rejected
```

### Mata Uang yang Di-Fetch dari JISDOR

Per seed `mst.mata_uang` (migration 000002): IDR, USD, SGD, EUR, JPY, AUD, CNY. JISDOR secara resmi menerbitkan kurs tengah untuk mata uang utama. Untuk mata uang yang tidak tersedia di JISDOR (mis. CNY via BI Kurs Tengah), `sumber_kurs = 'BI_KURS_TENGAH'` dan masuk via manual upload atau endpoint fallback. Sumber kurs per mata uang dikonfigurasi di `mst.mata_uang.sumber_kurs_default`.

### Schema Referensi P5-M5

#### `mst.kurs` (existing — init_schema + migration 000020)
| Kolom | Tipe | Status |
|---|---|---|
| `id` | UUID PK DEFAULT uuidv7() | existing |
| `fx_rate_id_kode` | VARCHAR(20) NOT NULL | existing |
| `kode_mata_uang` | CHAR(3) NOT NULL FK `mst.mata_uang` | existing |
| `tanggal_berlaku` | DATE NOT NULL | existing |
| `kurs_beli` | NUMERIC(15,4) | existing — **migration 000039 upgrade ke NUMERIC(20,8) per DEC-016** |
| `kurs_jual` | NUMERIC(15,4) | existing — **upgrade ke NUMERIC(20,8)** |
| `kurs_tengah` | NUMERIC(15,4) NOT NULL | existing — **upgrade ke NUMERIC(20,8)** |
| `sumber_kurs` | VARCHAR(30) NOT NULL | existing — CHECK: `'BI_JISDOR','BI_KURS_TENGAH','INTERNAL','MANUAL'` |
| `periode_bulanan_id` | UUID NOT NULL FK `mst.periode_buku(id)` | existing |
| `locked_flag` | BOOLEAN NOT NULL DEFAULT FALSE | existing (init_schema) |
| `maker_id` | UUID FK `sec.user(id)` | existing |
| `approver_id` | UUID FK `sec.user(id)` | existing |
| `dokumen_bukti_id` | UUID | existing |
| `created_at` | TIMESTAMPTZ NOT NULL DEFAULT now() | existing |
| `approved_at` | TIMESTAMPTZ | existing |
| `workflow_status` | VARCHAR(30) NOT NULL DEFAULT 'DRAFT' | existing (migration 000020) |
| `created_by` | UUID FK `sec.user(id)` | existing (migration 000020) |
| `updated_by` | UUID FK `sec.user(id)` | existing (migration 000020) |
| `updated_at` | TIMESTAMPTZ | existing (migration 000020) |
| `deleted_at` | TIMESTAMPTZ | existing (migration 000020) |
| `deleted_by` | UUID FK `sec.user(id)` | existing (migration 000020) |
| `row_version` | BIGINT NOT NULL DEFAULT 1 | existing (migration 000020) |
| `tenant_id` | TEXT NOT NULL DEFAULT 'TUGURE' | existing (migration 000020) |
| UNIQUE | `(kode_mata_uang, tanggal_berlaku)` | existing |

**Kolom tambahan yang dibutuhkan P5-M5** (migration 000039 — data-modeler):
| Kolom | Tipe | Keterangan |
|---|---|---|
| `rate_deviation_pct` | NUMERIC(8,4) | Deviasi persentase dari hari sebelumnya. `NULL` jika tidak ada data kemarin (hari pertama). Positif = naik. |
| `deviation_flag` | BOOLEAN NOT NULL DEFAULT FALSE | `TRUE` jika `rate_deviation_pct > 20%` atau `< -20%`. Trigger human review. |
| `jisdor_fetch_metadata` | JSONB | Raw metadata JISDOR fetch: `{url, fetched_at, http_status, response_hash, retry_count}`. `NULL` untuk manual upload. |
| `reject_reason` | TEXT | Alasan reject oleh ROLE-AKUN-CTL (wajib jika `workflow_status = 'REJECTED'`). |
| `upload_batch_id` | UUID FK `sys.upload_batch(id)` | Link ke batch jika upload via file. `NULL` untuk JISDOR auto-fetch. |

#### `sys.holiday_calendar` (baru — migration 000039)
Menyimpan kalender libur nasional Indonesia untuk menentukan hari kerja (JISDOR tidak terbit pada hari libur).

| Kolom | Tipe | Keterangan |
|---|---|---|
| `id` | UUID PK DEFAULT gen_random_uuid() | |
| `tanggal` | DATE NOT NULL | Tanggal libur |
| `nama_libur` | VARCHAR(100) NOT NULL | Nama hari libur (mis. "Hari Raya Idul Fitri 1447H") |
| `tipe` | VARCHAR(20) NOT NULL | `NASIONAL` / `CUTI_BERSAMA` / `WEEKEND` (Sabtu/Minggu sudah handled by ISODOW check) |
| `tahun` | SMALLINT NOT NULL | Tahun, untuk query cepat |
| `created_at` | TIMESTAMPTZ NOT NULL DEFAULT now() | |
| `created_by` | UUID NOT NULL | |
| `tenant_id` | TEXT NOT NULL DEFAULT 'TUGURE' | |
| UNIQUE | `(tanggal)` | Satu baris per hari libur |

---

## Story P5-M5-S1 — BI JISDOR Daily Cron (Asynq Scheduled 10:30 WIB Hari Kerja)

**Actor**: System (Asynq worker `integration-engineer`) — dipicu otomatis; jika gagal → ROLE-IT-ADMIN + ROLE-AKUN dinotifikasi
**Trigger**: Asynq cron scheduler `"30 3 * * 1-5"` (10:30 WIB = 03:30 UTC, Senin–Jumat). Worker pertama memeriksa apakah hari ini hari libur nasional Indonesia. Jika libur → skip run, log advisory. Jika hari kerja → fetch JISDOR.
**Goal**: Setiap hari kerja, Asynq worker fetch kurs tengah BI JISDOR untuk semua mata uang yang `sumber_kurs_default = 'BI_JISDOR'` di `mst.mata_uang`. Insert ke `mst.kurs` secara idempotent (skip jika sudah ada untuk hari yang sama). Validasi deviasi ±20% dari hari sebelumnya. Jika fetch gagal setelah 3 retry (interval 15 menit) → entry DLQ + alert. Seluruh operasi audit di `aud.audit_log` per mata uang.

### Pre-conditions
1. Hari adalah hari kerja (Senin–Jumat, bukan hari libur nasional di `sys.holiday_calendar`)
2. Asynq worker "integration:jisdor_fetch" ter-schedule dan aktif
3. `mst.mata_uang` sudah ter-seed dengan mata uang yang `sumber_kurs_default = 'BI_JISDOR'`
4. `sys.config` key `JISDOR_BASE_URL` dan `JISDOR_TIMEOUT_SECONDS` dikonfigurasi oleh ROLE-IT-ADMIN

### Fetch & Insert Logic
```
For each kode_mata_uang WHERE mst.mata_uang.sumber_kurs_default = 'BI_JISDOR':
    1. Fetch kurs_tengah dari BI JISDOR API/scrape (tanggal = today)
    2. Check UNIQUE(kode_mata_uang, tanggal_berlaku):
       - Sudah ada + workflow_status = 'APPROVED' → SKIP (idempotent), log KURS.JISDOR_SKIPPED
       - Sudah ada + workflow_status = 'PENDING_APPROVAL' → SKIP (manual upload pending), log KURS.JISDOR_SKIPPED_PENDING_EXISTS
       - Belum ada → INSERT
    3. Compute rate_deviation_pct vs hari kemarin (tanggal_berlaku - 1 hari kerja)
    4. Jika deviation_flag = TRUE → WARNING alert ke ROLE-AKUN + log KURS.DEVIATION_WARNING
    5. Audit KURS.JISDOR_FETCH in-transaction per row

Jika HTTP error atau parse error (setelah retry 1):
    → Asynq retry (max 3×, interval 15 menit)
    Setelah 3 retry gagal:
    → INSERT DLQ entry (sys.dead_letter_queue, mirror P5-M3 pattern)
    → Alert ke ROLE-AKUN: "JISDOR fetch gagal untuk {tanggal}. Upload manual diperlukan."
    → Alert ke ROLE-IT-ADMIN: "JISDOR worker error. DLQ entry dibuat."
    → log KURS.JISDOR_FETCH_FAILED
```

### Idempotency
- Jika endpoint di-trigger ulang (`POST /master/kurs/jisdor-sync`), logika sama: cek `UNIQUE` constraint sebelum insert.
- Worker memakai `Idempotency-Key` internal: `sha256("JISDOR-{kode_mata_uang}-{tanggal}")` untuk SoD antar-proses jika ada duplicate trigger.

### Nomor Kode Otomatis
`fx_rate_id_kode = 'FX-{KODE}-{YYYYMMDD}'` (mis. `FX-USD-20260618`). Dihasilkan oleh worker sebelum INSERT. Jika kode sudah ada (idempotent run) → dipakai kode yang sudah ada.

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `mst.mata_uang` | READ | Daftar mata uang + `sumber_kurs_default` |
| `sys.holiday_calendar` | READ | Cek hari libur nasional |
| `sys.config` | READ | `JISDOR_BASE_URL`, `JISDOR_TIMEOUT_SECONDS`, `JISDOR_RATE_DEVIATION_THRESHOLD_PCT` (default 20) |
| `mst.kurs` | READ + INSERT | UNIQUE check + insert baris baru |
| `mst.periode_buku` | READ | Lookup `periode_bulanan_id` via `tanggal_berlaku` dalam range `[tanggal_mulai, tanggal_akhir]` |
| `sys.dead_letter_queue` | INSERT | Saat 3 retry exhausted |
| `aud.audit_log` | INSERT | Per baris berhasil: `KURS.JISDOR_FETCH`; per skip: `KURS.JISDOR_SKIPPED`; per error: `KURS.JISDOR_FETCH_FAILED` |
| `sys.job` | UPDATE | Progress tracking job (untuk JobProgressPanel fallback) |

### Permissions
| Actor | Aksi | Catatan |
|---|---|---|
| System (Asynq worker) | INSERT `mst.kurs` | Service account, tidak ada RBAC JWT |
| ROLE-IT-ADMIN | `POST /master/kurs/jisdor-sync` | Manual trigger JISDOR fetch jika cron terlewat; membutuhkan `kurs.sync` permission |
| ROLE-AKUN, ROLE-AKUN-CTL, ROLE-AUDIT | `GET /master/kurs` | Read-only list hasil fetch |

### Audit Events
| Action | Trigger |
|---|---|
| `KURS.JISDOR_FETCH` | Setiap baris berhasil di-insert — in-transaction. `after_jsonb`: `{kode_mata_uang, tanggal_berlaku, kurs_tengah, rate_deviation_pct, deviation_flag}` |
| `KURS.JISDOR_SKIPPED` | Row sudah ada (idempotent skip) — advisory log |
| `KURS.JISDOR_SKIPPED_PENDING_EXISTS` | Sudah ada PENDING_APPROVAL row untuk tanggal yang sama — advisory log |
| `KURS.DEVIATION_WARNING` | `deviation_flag = TRUE` — advisory log + alert notification |
| `KURS.JISDOR_FETCH_FAILED` | Setelah 3 retry DLQ — advisory log + alert notification |
| `KURS.JISDOR_HOLIDAY_SKIP` | Hari libur nasional — advisory log (no alert) |

### Acceptance Criteria

```gherkin
Feature: BI JISDOR daily cron fetch — Asynq worker 10:30 WIB hari kerja

  Background:
    Given sys.config JISDOR_BASE_URL = "https://www.bi.go.id/id/statistik/informasi-kurs/jisdor"
    And sys.config JISDOR_RATE_DEVIATION_THRESHOLD_PCT = 20
    And mst.mata_uang: USD, EUR, JPY, SGD, AUD (sumber_kurs_default = 'BI_JISDOR')
    And mst.periode_buku PRD-2026-06: status_periode = 'OPEN', tanggal_mulai = 2026-06-01, tanggal_akhir = 2026-06-30
    And tanggal hari ini = 2026-06-18 (Kamis, bukan hari libur)

  # ─── HAPPY PATH: Fetch sukses — 5 mata uang ter-insert, idempotent ───────────

  Scenario: S1-AC1 — Worker berhasil fetch 5 mata uang JISDOR dan insert idempotent
    Given BI JISDOR API mengembalikan kurs tengah:
      | USD | 16250.0000 |
      | EUR | 17530.0000 |
      | JPY | 104.2500   |
      | SGD | 12100.0000 |
      | AUD | 10320.0000 |
    And belum ada mst.kurs rows untuk tanggal 2026-06-18
    And kurs USD kemarin (2026-06-17) = 16100.0000 (deviasi 0.93% — under threshold)
    When Asynq worker "integration:jisdor_fetch" dieksekusi pada 10:30 WIB 2026-06-18
    Then 5 baris INSERT ke mst.kurs dalam transaksi terpisah per mata uang:
      | kode_mata_uang | tanggal_berlaku | kurs_tengah | sumber_kurs | workflow_status | locked_flag |
      | USD            | 2026-06-18      | 16250.00000000 | BI_JISDOR  | APPROVED        | FALSE       |
      | EUR            | 2026-06-18      | 17530.00000000 | BI_JISDOR  | APPROVED        | FALSE       |
      | JPY            | 2026-06-18      | 104.25000000   | BI_JISDOR  | APPROVED        | FALSE       |
      | SGD            | 2026-06-18      | 12100.00000000 | BI_JISDOR  | APPROVED        | FALSE       |
      | AUD            | 2026-06-18      | 10320.00000000 | BI_JISDOR  | APPROVED        | FALSE       |
    And setiap row: fx_rate_id_kode = 'FX-{KODE}-20260618', periode_bulanan_id = PRD-2026-06
    And kurs_tengah disimpan dengan presisi 8 desimal (NUMERIC(20,8)) — DEC-016
    And rate_deviation_pct = 0.9317 (USD), deviation_flag = FALSE
    And 5 baris aud.audit_log action = KURS.JISDOR_FETCH, actor = system_worker_id
    And jika worker dieksekusi ulang (idempotent test):
      | worker cek UNIQUE(USD, 2026-06-18) → row sudah ada workflow_status='APPROVED' |
      | tidak ada INSERT kedua, tidak ada duplikat                                    |
      | aud.audit_log action = KURS.JISDOR_SKIPPED (advisory)                        |
    And toast ke ROLE-AKUN (via sistem notifikasi): "Kurs JISDOR 2026-06-18 berhasil di-fetch. 5 mata uang diperbarui."

  # ─── ERROR: Deviasi > 20% — WARNING alert, tetap insert ─────────────────────

  Scenario: S1-AC2 — Fetch sukses tapi USD deviation > 20% — WARNING flag aktif, bukan blok
    Given BI JISDOR mengembalikan USD = 19500.0000 untuk 2026-06-18
    And kurs USD kemarin (2026-06-17) = 16100.0000 (deviasi = 21.12% — above threshold 20%)
    When worker "integration:jisdor_fetch" dieksekusi
    Then mst.kurs INSERT USD dengan:
      | kurs_tengah        | 19500.00000000 |
      | rate_deviation_pct | 21.1180        |
      | deviation_flag     | TRUE           |
      | workflow_status    | APPROVED       |
    And aud.audit_log: KURS.JISDOR_FETCH (row) + KURS.DEVIATION_WARNING (advisory)
    And notifikasi WAJIB dikirim ke ROLE-AKUN + ROLE-AKUN-CTL:
      "PERHATIAN: Kurs USD 2026-06-18 mengalami deviasi 21.12% dari hari sebelumnya (IDR 16100 → IDR 19500). Harap verifikasi data JISDOR sebelum digunakan dalam perhitungan."
    And insert tetap dilakukan (tidak diblok) — human review diperlukan, bukan auto-reject
    And row USD tersedia di GET /master/kurs dengan badge "DEVIATION WARNING" di UI

  # ─── SKIP: Hari libur nasional — worker skip tanpa error ─────────────────────

  Scenario: S1-AC3 — Worker skip karena hari libur nasional (tidak ada JISDOR pada hari libur)
    Given sys.holiday_calendar entry: tanggal = 2026-06-17, nama_libur = "Hari Raya Idul Adha 1447H", tipe = 'NASIONAL'
    And tanggal trigger = 2026-06-17
    When Asynq worker "integration:jisdor_fetch" dieksekusi pada 10:30 WIB 2026-06-17
    Then worker mendeteksi hari libur via sys.holiday_calendar
    And tidak ada INSERT ke mst.kurs
    And aud.audit_log action = KURS.JISDOR_HOLIDAY_SKIP, after_jsonb = { "tanggal": "2026-06-17", "nama_libur": "Hari Raya Idul Adha 1447H" }
    And tidak ada alert/notifikasi ke ROLE-AKUN (bukan error, ini expected behavior)
    And worker return sukses (exit code 0, tidak masuk DLQ)

  # ─── ERROR: 3 retry exhausted — DLQ + alert wajib ────────────────────────────

  Scenario: S1-AC4 — JISDOR fetch gagal 3 kali berturut-turut — DLQ + alert ke ROLE-AKUN + ROLE-IT-ADMIN
    Given BI JISDOR endpoint mengembalikan HTTP 503 (service unavailable) untuk semua request
    When worker "integration:jisdor_fetch" dieksekusi dan retry 3× (interval 15 menit masing-masing)
    Then setelah retry ke-3 gagal:
      | sys.dead_letter_queue INSERT:                                            |
      |   job_type = 'jisdor_fetch'                                             |
      |   tanggal  = 2026-06-18                                                 |
      |   error    = "HTTP 503 after 3 retries. Last attempt: 11:00 WIB"        |
      |   retry_count = 3                                                        |
    And aud.audit_log action = KURS.JISDOR_FETCH_FAILED (advisory)
    And notifikasi WAJIB ke ROLE-AKUN:
      "KURS JISDOR 2026-06-18 gagal di-fetch setelah 3 percobaan. Upload kurs manual diperlukan sebelum proses MTM dan akrual hari ini. Hubungi IT jika masalah berlanjut."
    And notifikasi ke ROLE-IT-ADMIN:
      "JISDOR worker error 2026-06-18. DLQ entry dibuat. Periksa konektivitas ke BI JISDOR endpoint."
    And tidak ada baris baru di mst.kurs untuk 2026-06-18
    And DLQ entry dapat diakses via GET /admin/dlq?filter[job_type]=jisdor_fetch (ROLE-IT-ADMIN only)
```

---

## Story P5-M5-S2 — Manual Upload FX Rate (ROLE-AKUN — Maker, 4-Eyes)

**Actor**: ROLE-AKUN (Maker)
**Trigger**: JISDOR scrape gagal atau hari libur dan kurs tetap dibutuhkan (mis. untuk MTM instrumen FCY), atau kurs non-JISDOR (CNY, GBP) yang tidak masuk feed otomatis. ROLE-AKUN membuka halaman `/master/kurs` dan mengklik "Upload Kurs Manual".
**Goal**: ROLE-AKUN dapat mengupload kurs harian via XLSX/CSV (template baku) atau entry manual per mata uang. Sistem memvalidasi: format, rate range (tidak nol, tidak negatif), mata uang harus ada di `mst.mata_uang`, tanggal berlaku dalam periode yang `status_periode = 'OPEN'` (tidak bisa upload kurs untuk periode CLOSED), deviasi > 20% memerlukan catatan wajib. Status `PENDING_APPROVAL` hingga ROLE-AKUN-CTL approve. Tidak bisa upload dua kali untuk mata uang + tanggal yang sama jika sudah ada row `APPROVED` atau `PENDING_APPROVAL`.

### Pre-conditions
1. User ter-autentikasi dengan permission `kurs.create`
2. Request mengandung `Idempotency-Key` header (UUID v4)
3. Tidak ada `mst.kurs` row dengan `workflow_status IN ('APPROVED','PENDING_APPROVAL')` untuk `(kode_mata_uang, tanggal_berlaku)` yang sama
4. `mst.periode_buku.status_periode = 'OPEN'` untuk periode yang mencakup `tanggal_berlaku` — tidak bisa upload ke periode CLOSED atau SOFT_CLOSED

### Upload Template (XLSX/CSV)
| Kolom | Tipe | Keterangan |
|---|---|---|
| `kode_mata_uang` | CHAR(3) REQUIRED | Harus ada di `mst.mata_uang` |
| `tanggal_berlaku` | DATE REQUIRED | Format YYYY-MM-DD |
| `kurs_tengah` | NUMERIC REQUIRED | > 0, presisi 8 desimal |
| `kurs_beli` | NUMERIC OPTIONAL | > 0, jika diisi |
| `kurs_jual` | NUMERIC OPTIONAL | > 0, jika diisi |
| `catatan` | TEXT OPTIONAL | Wajib jika deviasi > 20% |

### Endpoint
```
POST /api/v1/master/kurs/upload
Authorization: Bearer <jwt>
Idempotency-Key: <uuid-v4>
Content-Type: multipart/form-data

Body:
  file: <xlsx atau csv — template>
  catatan_upload: "Upload manual karena JISDOR gangguan 2026-06-18"
  tanggal_berlaku_override: "2026-06-18"   ← opsional, override tanggal di file

→ 202 Accepted
{
  "data": {
    "upload_batch_id": "<uuid>",
    "rows_parsed": 3,
    "rows_valid": 3,
    "rows_invalid": 0,
    "status": "PENDING_APPROVAL",
    "kurs_ids": ["<uuid-USD>", "<uuid-EUR>", "<uuid-JPY>"],
    "next_step": "Menunggu approval ROLE-AKUN-CTL. Hubungi Finance Controller untuk review.",
    "deviation_warnings": []
  }
}

→ 422 Unprocessable Entity (validasi gagal)
{
  "error": {
    "code": "KURS_UPLOAD_VALIDATION_FAILED",
    "message": "2 baris tidak valid dalam file upload.",
    "details": [
      { "row": 2, "field": "kode_mata_uang", "rule": "Mata uang 'XYZ' tidak ditemukan di mst.mata_uang." },
      { "row": 3, "field": "kurs_tengah", "rule": "Nilai kurs_tengah tidak boleh nol atau negatif." }
    ]
  }
}
```

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `mst.mata_uang` | READ | Validasi kode mata uang |
| `mst.periode_buku` | READ | Validasi periode OPEN untuk tanggal berlaku |
| `mst.kurs` | READ + INSERT | UNIQUE check + insert `workflow_status='PENDING_APPROVAL'` |
| `sys.upload_batch` | INSERT | Tracking upload batch (mirror P5-M11 pattern) |
| `aud.audit_log` | INSERT | `KURS.MANUAL_UPLOAD_SUBMITTED` — in-transaction |
| `doc.upload` | INSERT | Simpan file original ke MinIO (ref `dokumen_bukti_id`) |

### Permissions
| Permission | Role | MFA | Catatan |
|---|---|---|---|
| `kurs.create` | ROLE-AKUN | Tidak | Maker upload — tidak perlu MFA |

### Audit Events
| Action | Trigger |
|---|---|
| `KURS.MANUAL_UPLOAD_SUBMITTED` | Setiap row berhasil di-parse + insert — in-transaction. `after_jsonb`: `{kode_mata_uang, tanggal_berlaku, kurs_tengah, upload_batch_id, deviation_flag}` |

### Acceptance Criteria

```gherkin
Feature: Manual upload FX rate oleh ROLE-AKUN (Maker, PENDING_APPROVAL)

  Background:
    Given user ROLE-AKUN (USR-AKUN-001) ter-autentikasi dengan permission kurs.create
    And mst.periode_buku PRD-2026-06: status_periode = 'OPEN'
    And mst.mata_uang: USD, EUR, JPY tersedia
    And tidak ada mst.kurs row APPROVED atau PENDING_APPROVAL untuk 2026-06-18

  # ─── HAPPY PATH: Upload 3 mata uang berhasil — PENDING_APPROVAL ──────────────

  Scenario: S2-AC1 — ROLE-AKUN berhasil upload kurs 3 mata uang via XLSX
    Given file kurs_manual_20260618.xlsx berisi:
      | kode_mata_uang | tanggal_berlaku | kurs_tengah  | kurs_beli    | kurs_jual    | catatan |
      | USD            | 2026-06-18      | 16250.00     |              |              |         |
      | EUR            | 2026-06-18      | 17530.00     |              |              |         |
      | JPY            | 2026-06-18      | 104.25       |              |              |         |
    And kurs USD kemarin = 16100.00 (deviasi 0.93% — under threshold)
    When USR-AKUN-001 mengirim POST /api/v1/master/kurs/upload
      With Idempotency-Key: IK-KURS-UPL-001
      With file: kurs_manual_20260618.xlsx
      With catatan_upload: "Upload manual karena JISDOR gangguan 2026-06-18"
    Then HTTP 202
    And 3 baris INSERT ke mst.kurs dengan workflow_status = 'PENDING_APPROVAL'
    And setiap row: kurs_tengah disimpan NUMERIC(20,8) — DEC-016
    And dalam satu transaksi:
      | aud.audit_log.action        | KURS.MANUAL_UPLOAD_SUBMITTED (3 entries) |
      | aud.audit_log.actor_user_id | USR-AKUN-001 UUID                        |
      | mst.kurs.maker_id           | USR-AKUN-001 UUID                        |
      | mst.kurs.upload_batch_id    | <uuid batch>                             |
      | doc.upload → MinIO          | file original tersimpan                  |
    And toast ke USR-AKUN-001: "3 kurs berhasil di-upload untuk 2026-06-18. Status: Menunggu approval Finance Controller."
    And notifikasi ke ROLE-AKUN-CTL: "Ada 3 kurs manual 2026-06-18 menunggu approval dari USR-AKUN-001. Review di /master/kurs?filter[workflow_status]=PENDING_APPROVAL"

  # ─── WARNING: Deviasi > 20% — catatan wajib ada, tetap masuk PENDING ─────────

  Scenario: S2-AC2 — Upload USD dengan deviasi > 20% — catatan wajib, upload masuk PENDING_APPROVAL
    Given file berisi USD 2026-06-18 = 19500.00 (deviasi 21.12% dari kemarin 16100.00)
    And kolom catatan USD kosong (tidak ada justifikasi)
    When USR-AKUN-001 mengirim POST /api/v1/master/kurs/upload
      With Idempotency-Key: IK-KURS-UPL-002
    Then HTTP 422:
      | error.code              | KURS_UPLOAD_VALIDATION_FAILED              |
      | error.details[0].row    | 1                                          |
      | error.details[0].field  | catatan                                    |
      | error.details[0].rule   | "Deviasi 21.12% melebihi threshold 20%. Kolom 'catatan' wajib diisi untuk deviasi besar (min 20 karakter)." |
    And tidak ada INSERT ke mst.kurs
    And tidak ada aud.audit_log entry (validasi gagal sebelum DB write)

    Given USR-AKUN-001 memperbaiki file: catatan = "Kenaikan akibat shock global FX 18 Juni 2026. Referensi Reuters: USDX+21."
    When USR-AKUN-001 re-submit dengan Idempotency-Key baru: IK-KURS-UPL-003
    Then HTTP 202
    And mst.kurs INSERT USD dengan deviation_flag = TRUE, catatan_upload = "Kenaikan akibat shock..."
    And aud.audit_log: KURS.MANUAL_UPLOAD_SUBMITTED + KURS.DEVIATION_WARNING
    And notifikasi ke ROLE-AKUN-CTL berisi badge "DEVIATION WARNING 21.12%" di pesan

  # ─── ERROR: Periode CLOSED — upload ditolak ──────────────────────────────────

  Scenario: S2-AC3 — Upload kurs untuk tanggal dalam periode CLOSED ditolak
    Given mst.periode_buku PRD-2026-05: status_periode = 'CLOSED', tanggal_akhir = 2026-05-31
    When USR-AKUN-001 mengirim POST /api/v1/master/kurs/upload
      With file berisi tanggal_berlaku = 2026-05-15 (dalam PRD-2026-05 yang CLOSED)
      With Idempotency-Key: IK-KURS-UPL-004
    Then HTTP 423:
      | error.code    | PERIODE_CLOSED                              |
      | error.message | "Periode PRD-2026-05 sudah hard-closed. Tidak bisa menambah kurs untuk tanggal 2026-05-15. Hubungi CFO untuk reopen jika perlu koreksi." |
    And tidak ada INSERT ke mst.kurs
    And tidak ada aud.audit_log entry

  # ─── ERROR: Duplikat — row APPROVED sudah ada untuk tanggal yang sama ─────────

  Scenario: S2-AC4 — Upload ditolak karena JISDOR sudah berhasil fetch hari ini
    Given mst.kurs USD 2026-06-18: workflow_status = 'APPROVED', sumber_kurs = 'BI_JISDOR' (sudah ada dari cron)
    When USR-AKUN-001 mengirim POST /api/v1/master/kurs/upload
      With file berisi USD 2026-06-18 = 16200.00
      With Idempotency-Key: IK-KURS-UPL-005
    Then HTTP 409:
      | error.code    | KURS_DUPLICATE_DATE                         |
      | error.message | "Kurs USD untuk 2026-06-18 sudah ada (APPROVED via BI_JISDOR). Tidak bisa di-override via manual upload. Jika ada koreksi, hubungi Finance Controller untuk proses adjustment." |
    And tidak ada INSERT ke mst.kurs
```

---

## Story P5-M5-S3 — Manual Upload Approve (ROLE-AKUN-CTL — Approver, 4-Eyes SoD)

**Actor**: ROLE-AKUN-CTL (Approver — user berbeda dari ROLE-AKUN Maker, SoD enforced)
**Trigger**: ROLE-AKUN-CTL menerima notifikasi in-app/email "Ada kurs manual PENDING_APPROVAL dari ROLE-AKUN". ROLE-AKUN-CTL membuka antrian di `/master/kurs?filter[workflow_status]=PENDING_APPROVAL` dan mengklik "Review".
**Goal**: ROLE-AKUN-CTL mereview kurs manual satu per satu (atau bulk approve per batch). Sistem enforce SoD (`approver_id ≠ maker_id`) server-side. Saat approve: `workflow_status → APPROVED`, kurs langsung aktif untuk dikonsumsi MTM + akrual + ECL conversion. Saat reject: wajib komentar, ROLE-AKUN dinotifikasi untuk re-upload. Semua state change audit in-transaction.

### Pre-conditions
1. User ter-autentikasi dengan permission `kurs.approve`
2. `mst.kurs.workflow_status = 'PENDING_APPROVAL'`
3. `approver_id ≠ maker_id` (SoD — enforced server-side, DEC-017)
4. Request mengandung `Idempotency-Key` header
5. `signature_method` wajib di body (`"JWT_STEP_UP"`)

### Endpoint
```
POST /api/v1/master/kurs/{id}/approve
Authorization: Bearer <jwt>
Idempotency-Key: <uuid-v4>

Body:
{
  "comment": "Kurs manual 2026-06-18 telah diverifikasi dengan sumber Bloomberg. Disetujui.",
  "signature_method": "JWT_STEP_UP"
}

→ 200 OK
{
  "data": {
    "kurs_id": "<uuid>",
    "kode_mata_uang": "USD",
    "tanggal_berlaku": "2026-06-18",
    "kurs_tengah": 16250.00000000,
    "workflow_status": "APPROVED",
    "approved_by": "USR-AKUN-CTL-001",
    "approved_at": "2026-06-18T11:30:00+07:00",
    "message": "Kurs USD 2026-06-18 berhasil disetujui dan aktif untuk digunakan."
  }
}

POST /api/v1/master/kurs/{id}/reject
Authorization: Bearer <jwt>
Idempotency-Key: <uuid-v4>

Body:
{
  "reject_reason": "Kurs tengah tidak sesuai dengan data BI. USD seharusnya 16100, bukan 16250. Mohon re-upload dengan sumber yang tepat.",
  "signature_method": "JWT_STEP_UP"
}

→ 200 OK
{
  "data": {
    "kurs_id": "<uuid>",
    "kode_mata_uang": "USD",
    "workflow_status": "REJECTED",
    "rejected_by": "USR-AKUN-CTL-001",
    "reject_reason": "Kurs tengah tidak sesuai..."
  }
}
```

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `mst.kurs` | READ + UPDATE | Set `workflow_status = 'APPROVED'`/`'REJECTED'`, `approver_id`, `approved_at`, `reject_reason` |
| `aud.audit_log` | INSERT | `KURS.APPROVED` atau `KURS.REJECTED` — in-transaction |

### Permissions & SoD
| Permission | Role | MFA | SoD Rule |
|---|---|---|---|
| `kurs.approve` | ROLE-AKUN-CTL | Tidak | `approver_id ≠ maker_id` (DEC-017, server-side) |
| `kurs.reject` | ROLE-AKUN-CTL | Tidak | `approver_id ≠ maker_id` |

### Audit Events
| Action | Trigger |
|---|---|
| `KURS.APPROVED` | `workflow_status` berhasil di-set `APPROVED` — in-transaction. `after_jsonb`: `{kode_mata_uang, tanggal_berlaku, kurs_tengah, approver_id, comment}` |
| `KURS.REJECTED` | `workflow_status` berhasil di-set `REJECTED` — in-transaction. `after_jsonb`: `{reject_reason, approver_id}` |

### Acceptance Criteria

```gherkin
Feature: Manual upload FX rate approve/reject oleh ROLE-AKUN-CTL (4-eyes, SoD)

  Background:
    Given mst.kurs USD 2026-06-18: workflow_status = 'PENDING_APPROVAL', maker_id = USR-AKUN-001
    And user ROLE-AKUN-CTL (USR-AKUN-CTL-001) ter-autentikasi dengan permission kurs.approve — bukan USR-AKUN-001

  # ─── HAPPY PATH: Approve sukses — kurs aktif (APPROVED) ──────────────────────

  Scenario: S3-AC1 — ROLE-AKUN-CTL berhasil approve kurs manual USD 2026-06-18
    When USR-AKUN-CTL-001 mengirim POST /api/v1/master/kurs/<uuid-USD>/approve
      With Idempotency-Key: IK-KURS-APR-001
      With body: { "comment": "Diverifikasi dengan Bloomberg 11:25 WIB.", "signature_method": "JWT_STEP_UP" }
    Then HTTP 200
    And dalam satu transaksi DB:
      | mst.kurs.workflow_status  | APPROVED                               |
      | mst.kurs.approver_id      | USR-AKUN-CTL-001 UUID                  |
      | mst.kurs.approved_at      | timestamp now                          |
      | mst.kurs.row_version      | incremented                            |
      | aud.audit_log.action      | KURS.APPROVED                          |
      | aud.audit_log.after_jsonb.kurs_tengah | 16250.00000000               |
    And toast ke USR-AKUN-CTL-001: "Kurs USD 2026-06-18 (IDR 16.250,00000000) berhasil disetujui. Kurs aktif dan siap digunakan."
    And notifikasi ke USR-AKUN-001 (maker): "Kurs manual USD 2026-06-18 Anda telah disetujui oleh Finance Controller. Kurs aktif."
    And GET /master/kurs/USD/2026-06-18 sekarang mengembalikan workflow_status = 'APPROVED'

  # ─── HAPPY PATH: Reject dengan komentar wajib ────────────────────────────────

  Scenario: S3-AC2 — ROLE-AKUN-CTL reject kurs karena nilai tidak sesuai
    When USR-AKUN-CTL-001 mengirim POST /api/v1/master/kurs/<uuid-USD>/reject
      With Idempotency-Key: IK-KURS-REJ-001
      With body: { "reject_reason": "Nilai USD 16250 tidak sesuai data BI JISDOR. Seharusnya 16100. Mohon re-upload.", "signature_method": "JWT_STEP_UP" }
    Then HTTP 200
    And dalam satu transaksi DB:
      | mst.kurs.workflow_status | REJECTED                                       |
      | mst.kurs.reject_reason   | "Nilai USD 16250 tidak sesuai data BI JISDOR..." |
      | aud.audit_log.action     | KURS.REJECTED                                  |
    And notifikasi ke USR-AKUN-001 (maker): "Kurs manual USD 2026-06-18 DITOLAK oleh Finance Controller. Alasan: 'Nilai USD 16250 tidak sesuai...' Harap re-upload kurs yang benar."

  # ─── ERROR: SoD — Maker mencoba approve kurs sendiri ─────────────────────────

  Scenario: S3-AC3 — USR-AKUN-001 (maker) mencoba approve kurs yang dia upload sendiri — SoD violation
    Given mst.kurs USD 2026-06-18: maker_id = USR-AKUN-001
    And USR-AKUN-001 memiliki permission kurs.approve (asumsi dual role)
    When USR-AKUN-001 mengirim POST /api/v1/master/kurs/<uuid-USD>/approve
      With Idempotency-Key: IK-KURS-APR-SOD
    Then HTTP 403:
      | error.code    | SOD_VIOLATION                                  |
      | error.message | "Anda tidak dapat meng-approve kurs yang Anda upload sendiri. Segregation of Duties wajib: approver_id ≠ maker_id (DEC-017)." |
    And tidak ada perubahan state
    And aud.audit_log action = KURS.APPROVE_REJECTED_SOD (advisory, actor = USR-AKUN-001)

  # ─── ERROR: Reject tanpa komentar (reject_reason kosong) ─────────────────────

  Scenario: S3-AC4 — Reject ditolak karena reject_reason tidak diisi
    When USR-AKUN-CTL-001 mengirim POST /api/v1/master/kurs/<uuid-USD>/reject
      With body: { "reject_reason": "", "signature_method": "JWT_STEP_UP" }
      With Idempotency-Key: IK-KURS-REJ-002
    Then HTTP 400:
      | error.code              | VALIDATION_FAILED                              |
      | error.details[0].field  | reject_reason                                  |
      | error.details[0].rule   | "reject_reason wajib diisi minimal 20 karakter saat menolak kurs manual." |
    And tidak ada perubahan state
```

---

## Story P5-M5-S4 — Locked Flag Enforcement (Hard-Close Trigger dari P5-M4)

**Actor**: System (triggered by P5-M4 hard-close approve action) — tidak ada user langsung di story ini
**Trigger**: Setelah ROLE-CFO berhasil approve hard-close (`POST /api/v1/periode-buku/{id}/hard-close-approve`) dan `mst.periode_buku.status_periode` berubah ke `'CLOSED'`, sistem (dalam transaksi yang sama di P5-M4-S3) men-set `mst.kurs.locked_flag = TRUE` untuk semua rows dengan `periode_bulanan_id = {id}`. Setelah ini, setiap attempt untuk INSERT/UPDATE/DELETE baris kurs yang locked diblok oleh:
1. DB trigger `tg_kurs_locked_check` (sudah ada di init_schema) — raises EXCEPTION level DB
2. API middleware `FxRateLockMiddleware` — cek `locked_flag` sebelum handler mutating, return 423

**Goal**: Memastikan integritas data historis kurs setelah periode hard-closed. Kurs yang sudah LOCKED tidak bisa diubah atau dihapus. Attempt untuk insert kurs baru dengan `tanggal_berlaku` dalam range periode CLOSED juga harus ditolak. Jika periode di-reopen (P5-M4-S4), `locked_flag = FALSE` di-reset.

### Mekanisme Lock (dari P5-M4 ke P5-M5)

```
P5-M4 hard-close-approve (ROLE-CFO, step-up MFA) — dalam SATU TRANSAKSI:
  1. UPDATE mst.periode_buku SET status_periode = 'CLOSED', ...
  2. UPDATE mst.kurs
        SET locked_flag = TRUE, updated_by = {cfo_user_id}, updated_at = now()
        WHERE periode_bulanan_id = {periode_id}
          AND locked_flag = FALSE                     ← idempotent
  3. INSERT aud.audit_log action = 'KURS.LOCKED_BY_PERIODE_CLOSE'
         after_jsonb = { periode_kode, count_locked, locked_by, locked_at }
  4. INSERT sys.closing_checklist_snapshot (P5-M4 flow)
  5. COMMIT

DB trigger fn_kurs_no_modify_when_locked (existing — BEFORE UPDATE OR DELETE):
  IF OLD.locked_flag = TRUE → RAISE EXCEPTION 'Kurs is locked because periode is CLOSED.'

API layer FxRateLockMiddleware:
  Sebelum handler mutating (POST/PATCH/PUT/DELETE untuk mst.kurs):
  IF kurs.locked_flag = TRUE → HTTP 423 FX_RATE_LOCKED
```

### Reopen Recovery

Saat periode di-reopen (P5-M4-S4):
- SOFT_CLOSED → OPEN: kurs `locked_flag = FALSE` di-reset untuk semua rows dalam periode yang di-reopen
- CLOSED → SOFT_CLOSED (dalam grace window): kurs `locked_flag = FALSE` di-reset

```
P5-M4 reopen-approve (ROLE-CFO) — dalam SATU TRANSAKSI:
  UPDATE mst.kurs SET locked_flag = FALSE, updated_by = ..., updated_at = now()
  WHERE periode_bulanan_id = {periode_id} AND locked_flag = TRUE
  INSERT aud.audit_log action = 'KURS.UNLOCKED_BY_PERIODE_REOPEN'
```

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `mst.kurs` | UPDATE | Bulk SET `locked_flag = TRUE` — in-transaction dengan P5-M4 hard-close |
| `aud.audit_log` | INSERT | `KURS.LOCKED_BY_PERIODE_CLOSE` — in-transaction |
| `mst.periode_buku` | READ | Lookup `periode_bulanan_id` untuk range check |

### Permissions
| Actor | Aksi | Catatan |
|---|---|---|
| System (P5-M4 transaction) | UPDATE `mst.kurs.locked_flag` | Dalam transaksi hard-close CFO |
| ROLE-CFO | Via P5-M4 hard-close-approve | Tidak langsung ke endpoint `mst.kurs` |
| Anyone (ROLE-AKUN etc.) | INSERT/UPDATE/DELETE kurs dengan `locked_flag = TRUE` | Diblok 423 FX_RATE_LOCKED |

### Audit Events
| Action | Trigger |
|---|---|
| `KURS.LOCKED_BY_PERIODE_CLOSE` | Bulk lock berhasil — in-transaction dengan `PERIODE.HARDCLOSED`. `after_jsonb`: `{periode_kode, count_locked, period_range_start, period_range_end}` |
| `KURS.UNLOCKED_BY_PERIODE_REOPEN` | Bulk unlock saat reopen — in-transaction dengan `PERIODE.REOPENED`. `after_jsonb`: `{periode_kode, count_unlocked}` |

### Acceptance Criteria

```gherkin
Feature: FX Rate locked_flag enforcement — triggered by P5-M4 hard-close + guard setelah lock

  Background:
    Given mst.periode_buku PRD-2026-05: tanggal_mulai = 2026-05-01, tanggal_akhir = 2026-05-31
    And 5 baris mst.kurs dalam PRD-2026-05 (USD, EUR, JPY, SGD, AUD): workflow_status = 'APPROVED', locked_flag = FALSE
    And mst.periode_buku PRD-2026-05.status_periode = 'HARD_CLOSE_PENDING'
    And ROLE-CFO (USR-CFO-001) dengan step-up MFA valid

  # ─── HAPPY PATH: Hard-close sets locked_flag = TRUE semua kurs periode ─────────

  Scenario: S4-AC1 — Hard-close PRD-2026-05 men-lock semua 5 baris kurs dalam satu transaksi
    When USR-CFO-001 mengirim POST /api/v1/periode-buku/PRD-2026-05/hard-close-approve
      With X-Step-Up-Token: <valid-stepup-token>
      With Idempotency-Key: IK-HC-APR-M4-001
    Then HTTP 200 (dari P5-M4 handler)
    And dalam SATU TRANSAKSI DB:
      | mst.periode_buku.status_periode          | CLOSED                   |
      | mst.kurs (5 rows PRD-2026-05).locked_flag | TRUE (semua)            |
      | mst.kurs.updated_by                      | USR-CFO-001 UUID (semua) |
      | aud.audit_log action = PERIODE.HARDCLOSED | ada                     |
      | aud.audit_log action = KURS.LOCKED_BY_PERIODE_CLOSE | ada             |
      | aud.audit_log KURS.LOCKED_BY_PERIODE_CLOSE.after_jsonb.count_locked | 5 |
    And DB trigger tg_kurs_locked_check aktif untuk semua 5 rows
    And attempt UPDATE mst.kurs (USD PRD-2026-05) dari service layer → DB RAISE EXCEPTION → 423

  # ─── ERROR: Mutasi kurs LOCKED via API diblok 423 FX_RATE_LOCKED ─────────────

  Scenario: S4-AC2 — Attempt upload kurs untuk tanggal dalam periode CLOSED ditolak
    Given mst.periode_buku PRD-2026-05: status_periode = 'CLOSED'
    And mst.kurs USD 2026-05-15: locked_flag = TRUE
    When ROLE-AKUN mengirim POST /api/v1/master/kurs/upload
      With file berisi tanggal_berlaku = 2026-05-15 (dalam PRD-2026-05 CLOSED)
      With Idempotency-Key: IK-KURS-UPL-LOCKED
    Then HTTP 423:
      | error.code    | FX_RATE_LOCKED                              |
      | error.message | "Kurs untuk periode PRD-2026-05 sudah dikunci (hard-closed pada [tanggal]). Tidak bisa menambah atau mengubah kurs. Hubungi CFO untuk reopen dalam grace window." |
    And tidak ada INSERT ke mst.kurs
    Dan jika reopen PRD-2026-05 berhasil (CLOSED → SOFT_CLOSED via CFO dalam grace window):
    Then mst.kurs (5 rows PRD-2026-05).locked_flag = FALSE
    And aud.audit_log action = KURS.UNLOCKED_BY_PERIODE_REOPEN

  # ─── ERROR: DB trigger guard — bypass API, langsung ke DB ────────────────────

  Scenario: S4-AC3 — DB-level protection: UPDATE langsung ke mst.kurs LOCKED diblok trigger
    Given mst.kurs USD 2026-05-15: locked_flag = TRUE
    When query SQL: "UPDATE mst.kurs SET kurs_tengah = 17000 WHERE id = '<uuid-USD-2026-05-15>'"
    Then PostgreSQL RAISE EXCEPTION: "Kurs is locked because periode is CLOSED. Use prior-period adjustment."
    And UPDATE tidak committed
    And tidak ada perubahan di mst.kurs
    (Catatan: test ini adalah integration test di layer DB — membuktikan DB trigger tidak bypass-able walau API dilewati)

  # ─── ERROR: Kurs baru untuk tanggal di periode CLOSED — periode check ─────────

  Scenario: S4-AC4 — Attempt INSERT kurs baru untuk tanggal dalam periode CLOSED ditolak di service layer
    Given mst.periode_buku PRD-2026-05: status_periode = 'CLOSED'
    And tanggal_berlaku target = 2026-05-20 (dalam range PRD-2026-05)
    When ROLE-AKUN atau JISDOR worker mencoba INSERT mst.kurs untuk 2026-05-20
    Then service layer check: mst.periode_buku untuk tanggal 2026-05-20 = PRD-2026-05 → status_periode = 'CLOSED'
    And HTTP 423 (API) atau RAISE EXCEPTION (worker)
    And aud.audit_log action = KURS.INSERT_REJECTED_PERIODE_CLOSED (advisory)
    And tidak ada INSERT ke mst.kurs
```

---

## Story P5-M5-S5 — FX Gain/Loss Treatment Routing per Klasifikasi PSAK 71

**Actor**: System-internal (dipanggil oleh P5-M6 MTM worker dan P5-M8 Penjualan service) + ROLE-RISK (review); ROLE-AUDIT (read)
**Trigger**: Setiap kali sistem perlu menentukan ke mana FX gain/loss suatu instrumen harus diposting — saat MTM harian (P5-M6), saat penjualan/disposal (P5-M8), saat akrual harian (P5-M9). Internal service memanggil `GET /api/v1/master/kurs/treatment/{instrumen_id}` untuk mendapatkan routing decision.
**Goal**: Endpoint `GET /master/kurs/treatment/{instrumen_id}` mengembalikan routing decision FX gain/loss berdasarkan `klasifikasi_psak71` instrumen (setelah klasifikasi LOCKED):
- `AC` → `P&L_FOREIGN_EXCHANGE` (ke Laporan Laba Rugi, event code `FX_REALIZED` atau `FX_UNREALIZED`)
- `FVTPL` → `P&L_FOREIGN_EXCHANGE` (ke P&L, event code `FX_UNREALIZED`)
- `FVOCI_DEBT` → `OCI_FOREIGN_EXCHANGE_RESERVE` (ke OCI, event code `FX_OCI_RESERVE`)
- `FVOCI_ELECTION` → `OCI_FOREIGN_EXCHANGE_RESERVE_NO_RECYCLING` (ke OCI, tanpa recycling ke P&L saat disposal)
- IDR (mata_uang = 'IDR') → `NO_FX_TREATMENT` (tidak ada FX gain/loss)

Routing decision ini adalah **compliance-critical** — kesalahan routing akan menyebabkan misstatement P&L atau OCI. **ifrs9-compliance-reviewer BLOCKING** untuk perubahan logic routing ini.

### Routing Matrix (per PSAK 71 / IFRS 9)

| Klasifikasi PSAK 71 | Mata Uang | FX Routing Decision | Account Type | OCI Recycling |
|---|---|---|---|---|
| `AC` | FCY | `P&L_FOREIGN_EXCHANGE` | P&L (interest + FX separate) | N/A |
| `FVTPL` | FCY | `P&L_FOREIGN_EXCHANGE` | P&L | N/A |
| `FVOCI_DEBT` | FCY | `OCI_FOREIGN_EXCHANGE_RESERVE` | OCI | Ya (recycled ke P&L saat derecognition) |
| `FVOCI_ELECTION` | FCY | `OCI_FOREIGN_EXCHANGE_RESERVE_NO_RECYCLING` | OCI | **Tidak** (irrevocable election — PSAK 71 §5.7.5) |
| Any | IDR | `NO_FX_TREATMENT` | — | N/A |

### Endpoint
```
GET /api/v1/master/kurs/treatment/{instrumen_id}
Authorization: Bearer <jwt>

→ 200 OK
{
  "data": {
    "instrumen_id": "<uuid>",
    "kode_instrumen": "OBL-2026-00042",
    "klasifikasi_psak71": "FVOCI_DEBT",
    "mata_uang": "USD",
    "fx_treatment": {
      "routing": "OCI_FOREIGN_EXCHANGE_RESERVE",
      "account_type": "OCI",
      "oci_recycling": true,
      "jurnal_event_code": "FX_OCI_RESERVE",
      "psak71_reference": "PSAK 71 §5.7.10 — FVOCI debt instrument: exchange differences in OCI",
      "notes": "FX gain/loss di-post ke OCI. Saat derecognition (penjualan/jatuh tempo), OCI balance di-recycle ke P&L (REKLAS_OCI_PL event)."
    },
    "klasifikasi_locked": true,
    "klasifikasi_locked_at": "2026-01-20T10:00:00+07:00"
  }
}

→ 422 Unprocessable Entity (klasifikasi belum locked)
{
  "error": {
    "code": "KLASIFIKASI_NOT_LOCKED",
    "message": "Instrumen OBL-2026-00043 belum memiliki klasifikasi PSAK 71 yang locked. FX treatment tidak dapat ditentukan. Selesaikan SPPI Test + BM Assessment + Klasifikasi Approval terlebih dahulu.",
    "details": [{ "field": "klasifikasi_locked", "rule": "klasifikasi_psak71 harus locked sebelum FX treatment dapat diquery" }]
  }
}
```

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `mst.instrumen` | READ | `klasifikasi_psak71`, `mata_uang`, `klasifikasi_locked_at` |
| `mst.chart_of_accounts` | READ | Lookup akun target untuk event code (di-return sebagai `target_account_id` jika tersedia) |
| `aud.audit_log` | INSERT | `KURS.FX_TREATMENT_QUERIED` — advisory log (untuk monitoring frekuensi query, bukan setiap call) |

### Permissions
| Permission | Role | Catatan |
|---|---|---|
| `kurs.read` | ROLE-AKUN, ROLE-AKUN-CTL, ROLE-RISK, ROLE-AUDIT | Read-only; sistem internal memanggil langsung via service call |
| `instrumen.read` | (semua role yang dapat lihat instrumen) | Diperlukan untuk resolve klasifikasi |

### Compliance Note
Endpoint ini menyentuh klasifikasi PSAK 71 dan routing jurnal — **ifrs9-compliance-reviewer BLOCKING** untuk setiap perubahan pada routing matrix. Routing harus konsisten dengan:
- FSD-APP-D §2.4 (FX treatment per klasifikasi)
- PSAK 71 §5.7.10 (FVOCI debt FX in OCI)
- PSAK 71 §5.7.5 (FVOCI Election no recycling)
- SoW_v1.4.docx §10.3 (FX gain/loss treatment)

### Audit Events
| Action | Trigger |
|---|---|
| `KURS.FX_TREATMENT_QUERIED` | Advisory log, sampling-based (tidak setiap call untuk performa) — batch log per 100 calls atau per unique instrumen per hari |

### Acceptance Criteria

```gherkin
Feature: FX gain/loss treatment routing per klasifikasi PSAK 71

  Background:
    Given mst.instrumen tersedia dengan berbagai klasifikasi dan mata uang

  # ─── HAPPY PATH: FVOCI_DEBT instrument FCY → OCI routing ─────────────────────

  Scenario: S5-AC1 — Obligasi USD FVOCI_DEBT mengembalikan routing OCI_FOREIGN_EXCHANGE_RESERVE
    Given mst.instrumen OBL-2026-00042:
      | klasifikasi_psak71  | FVOCI_DEBT |
      | mata_uang           | USD        |
      | klasifikasi_locked  | TRUE       |
      | klasifikasi_locked_at | 2026-01-20T10:00:00+07:00 |
    When GET /api/v1/master/kurs/treatment/<uuid-OBL-0042>
    Then HTTP 200
    And response.data.fx_treatment.routing = "OCI_FOREIGN_EXCHANGE_RESERVE"
    And response.data.fx_treatment.oci_recycling = true
    And response.data.fx_treatment.jurnal_event_code = "FX_OCI_RESERVE"
    And response.data.fx_treatment.psak71_reference = "PSAK 71 §5.7.10 — FVOCI debt instrument: exchange differences in OCI"
    And response.data.klasifikasi_locked = true

  # ─── HAPPY PATH: FVOCI_ELECTION saham USD → OCI no-recycling ────────────────

  Scenario: S5-AC2 — Saham USD FVOCI_ELECTION mengembalikan OCI_NO_RECYCLING — irrevocable
    Given mst.instrumen SHM-2026-00015:
      | klasifikasi_psak71  | FVOCI_ELECTION |
      | mata_uang           | USD            |
      | klasifikasi_locked  | TRUE           |
    When GET /api/v1/master/kurs/treatment/<uuid-SHM-0015>
    Then HTTP 200
    And response.data.fx_treatment.routing = "OCI_FOREIGN_EXCHANGE_RESERVE_NO_RECYCLING"
    And response.data.fx_treatment.oci_recycling = false
    And response.data.fx_treatment.notes contains "irrevocable election"
    And response.data.fx_treatment.jurnal_event_code = "FX_OCI_RESERVE_NO_RECYCLE"

  # ─── HAPPY PATH: Deposito IDR → NO_FX_TREATMENT (tidak ada konversi FX) ─────

  Scenario: S5-AC3 — Deposito IDR AC mengembalikan NO_FX_TREATMENT
    Given mst.instrumen DEP-2026-00001:
      | klasifikasi_psak71  | AC  |
      | mata_uang           | IDR |
      | klasifikasi_locked  | TRUE |
    When GET /api/v1/master/kurs/treatment/<uuid-DEP-0001>
    Then HTTP 200
    And response.data.fx_treatment.routing = "NO_FX_TREATMENT"
    And response.data.fx_treatment.notes = "Instrumen berdenominasi IDR — tidak ada FX gain/loss."
    And response.data.mata_uang = "IDR"

  # ─── ERROR: Klasifikasi belum locked — treatment tidak dapat ditentukan ────────

  Scenario: S5-AC4 — Instrumen dengan klasifikasi belum locked mengembalikan KLASIFIKASI_NOT_LOCKED
    Given mst.instrumen OBL-2026-00099:
      | klasifikasi_locked  | FALSE |
      | klasifikasi_psak71  | NULL  |
      | mata_uang           | EUR   |
    When GET /api/v1/master/kurs/treatment/<uuid-OBL-0099>
    Then HTTP 422:
      | error.code    | KLASIFIKASI_NOT_LOCKED                      |
      | error.message | "Instrumen OBL-2026-00099 belum memiliki klasifikasi PSAK 71 yang locked. FX treatment tidak dapat ditentukan. Selesaikan SPPI Test + BM Assessment + Klasifikasi Approval terlebih dahulu." |
    And tidak ada routing decision di response
    Dan P5-M6 MTM worker yang memanggil endpoint ini → log WARNING + skip instrumen dari MTM run hingga klasifikasi locked
```

---

## Ringkasan P5-M5 Story Set

| Story | Judul | Actor Utama | AC Count | Gate |
|---|---|---|---|---|
| P5-M5-S1 | BI JISDOR daily cron — Asynq 10:30 WIB hari kerja | System (Asynq worker) | 4 | advisory (integration-engineer implementasi) |
| P5-M5-S2 | Manual upload FX rate (Maker, PENDING_APPROVAL) | ROLE-AKUN | 4 | advisory |
| P5-M5-S3 | Manual upload approve/reject (4-eyes, SoD) | ROLE-AKUN-CTL | 4 | advisory + security (SoD) |
| P5-M5-S4 | Locked flag enforcement (hard-close trigger dari P5-M4) | System (P5-M4 transaction) | 4 | security advisory (lock mechanism audit) |
| P5-M5-S5 | FX gain/loss treatment routing per klasifikasi PSAK 71 | System internal + ROLE-RISK | 4 | **ifrs9-compliance-reviewer BLOCKING** |
| **Total** | | | **20** | |

---

## Error Codes Proposed (Baru — untuk system-analyst)

Kode baru yang dibutuhkan P5-M5 dan belum ada di `api-conventions.md`:

| Code | HTTP | Trigger | Catatan |
|---|---|---|---|
| `FX_RATE_LOCKED` | 423 | Mutasi (INSERT/UPDATE/DELETE) pada `mst.kurs` dengan `locked_flag = TRUE` | Berbeda dari `PERIODE_CLOSED` — spesifik untuk FX rate lock; middleware `FxRateLockMiddleware` |
| `KURS_DUPLICATE_DATE` | 409 | INSERT kurs untuk `(kode_mata_uang, tanggal_berlaku)` yang sudah ada dengan status `APPROVED` atau `PENDING_APPROVAL` | Bukan idempotency error — ini business conflict |
| `KURS_UPLOAD_VALIDATION_FAILED` | 422 | File upload gagal validasi (format, range, mata uang tidak dikenal, catatan wajib untuk deviasi besar) | `details[]` berisi row number + field + rule |
| `KLASIFIKASI_NOT_LOCKED` | 422 | Query FX treatment untuk instrumen yang klasifikasinya belum di-lock | FX treatment tidak dapat ditentukan tanpa klasifikasi PSAK 71 final |
| `KURS_PERIODE_MISMATCH` | 422 | `tanggal_berlaku` tidak dalam range periode manapun yang `status_periode = 'OPEN'` | Tanggal tidak valid untuk periode yang tersedia |

Catatan: `PERIODE_CLOSED` (HTTP 423), `SOD_VIOLATION` (HTTP 403), dan `WORKFLOW_INVALID_TRANSITION` (HTTP 422) sudah ada di `api-conventions.md` — tidak perlu ditambahkan ulang.

---

## Persona Summary Table

| Actor | Permission | Aksi di P5-M5 | MFA Level |
|---|---|---|---|
| ROLE-AKUN | `kurs.create`, `kurs.read` | Upload kurs manual (Maker), view list kurs, view treatment routing | Tidak wajib |
| ROLE-AKUN-CTL | `kurs.approve`, `kurs.reject`, `kurs.read`, `kurs.export` | Approve/reject kurs manual (SoD: ≠ maker); view list + export | MFA wajib (DEC-026) |
| ROLE-IT-ADMIN | `kurs.sync`, `kurs.read` | Trigger manual JISDOR sync; view DLQ entries | MFA wajib (DEC-026) |
| ROLE-CFO | Via P5-M4 (`periode.hardclose.approve`) | Secara indirect men-lock semua kurs dalam periode saat hard-close | MFA + step-up (DEC-026 + DEC-027) |
| ROLE-RISK | `kurs.read`, `instrumen.read` | View FX treatment routing; advisory review sebelum MTM run | Tidak wajib |
| ROLE-AUDIT | `kurs.read`, `kurs.export` | Read-only seluruh kurs data termasuk deleted_at IS NOT NULL (`?include_deleted=true`); export audit | Tidak wajib |
| System (Asynq worker) | Service account (no JWT) | BI JISDOR auto-fetch + auto-approve; locked_flag bulk SET (via P5-M4 CFO transaction) | N/A |

---

## Dependensi Lintas Modul

| Dependensi | Arah | Keterangan |
|---|---|---|
| `mst.mata_uang` seeded | Phase 3 → P5-M5 | Worker dan manual upload validasi mata uang vs `mst.mata_uang` |
| `sys.holiday_calendar` | P5-M5 → P5-M5 | Tabel baru (migration 000039) untuk kalender libur nasional Indonesia |
| `mst.kurs.locked_flag = TRUE` trigger | P5-M4 → P5-M5 | Hard-close CFO men-set locked_flag via UPDATE dalam transaksi yang sama dengan PERIODE.HARDCLOSED |
| `mst.kurs.locked_flag = FALSE` on reopen | P5-M4 → P5-M5 | Reopen CFO me-reset locked_flag via UPDATE dalam transaksi PERIODE.REOPENED |
| FX treatment routing | P5-M5 → P5-M6 | MTM worker mengkonsumsi `GET /master/kurs/treatment/{instrumen_id}` untuk routing OCI vs P&L |
| FX treatment routing | P5-M5 → P5-M8 | Penjualan/disposal service mengkonsumsi treatment routing untuk FX recycling decision |
| FX treatment routing | P5-M5 → P5-M9 | Akrual harian (EIR method) mengkonsumsi kurs tengah untuk konversi EAD_FCY → EAD_IDR |
| DLQ pattern | P5-M3 → P5-M5 | JISDOR DLQ mengikuti pattern DLQ `sys.dead_letter_queue` yang sudah diimplementasi di P5-M3 |
| Migration 000039 | P5-M5 → data-modeler | Migration baru: NUMERIC precision upgrade kurs, `sys.holiday_calendar`, kolom tambahan `mst.kurs` |
| Frontend screens | P5-M5 → P5-M17 | P5-M17 mengimplementasikan `/master/kurs` DataTable + upload form + treatment viewer |

---

## Open Questions — P5-M5

| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-M5-1 | BI JISDOR: apakah tersedia via public API (REST) atau harus web-scraping halaman HTML? | **Web-scraping atau RSS** — BI JISDOR tidak memiliki public REST API yang terdokumentasi per tanggal ini. `integration-engineer` perlu konfirmasi URL pattern + response format. Fallback: `BI_KURS_TENGAH` manual jika scraping diblok. |
| OQ-M5-2 | Apakah JISDOR menerbitkan kurs untuk CNY (Yuan)? CNY seed di `mst.mata_uang` dengan `sumber_kurs_default = 'BI_KURS_TENGAH'`. | **Tidak via JISDOR** — CNY masuk via manual upload atau `BI_KURS_TENGAH` fallback. `integration-engineer` konfirmasi mata uang yang tersedia di JISDOR. |
| OQ-M5-3 | `sys.holiday_calendar` — apakah perlu seeder untuk tahun 2026 di migration 000039, atau upload via UI oleh ROLE-IT-ADMIN? | **Seed tahun berjalan di migration** (2026 + 2027 sebagai bootstrap). Selanjutnya ROLE-IT-ADMIN upload setiap awal tahun. Tanggal libur 2026: lihat Keppres pemerintah Indonesia. |
| OQ-M5-4 | Rate deviation threshold 20% — apakah konfigurabel per mata uang atau global? | **Global via `sys.config` `JISDOR_RATE_DEVIATION_THRESHOLD_PCT`** default 20. Override per mata uang bisa Phase 6 jika dibutuhkan (mis. JPY historically lebih volatile). |
| OQ-M5-5 | Apakah manual JISDOR trigger `POST /master/kurs/jisdor-sync` memerlukan approval ROLE-AKUN-CTL? | **Tidak** — JISDOR source sudah otoritatif (BI). Manual trigger via ROLE-IT-ADMIN langsung → `APPROVED` (sama seperti cron). Berbeda dengan manual upload yang butuh 4-eyes karena sumber tidak otoritatif. |
| OQ-M5-6 | FX treatment untuk instrumen `FVOCI_DEBT` mata uang IDR — apakah `OCI_FOREIGN_EXCHANGE_RESERVE` atau `NO_FX_TREATMENT`? | **`NO_FX_TREATMENT`** — tidak ada konversi FX jika mata_uang = IDR. OCI treatment hanya berlaku untuk FCY instrumen. Konfirmasi: ifrs9-compliance-reviewer. |

---

## Schema Change Summary (untuk data-modeler — migration 000039)

### A. ALTER TABLE `mst.kurs`
```sql
-- Upgrade presisi FX rate per DEC-016 (NUMERIC(20,8))
ALTER TABLE mst.kurs
    ALTER COLUMN kurs_beli   TYPE NUMERIC(20,8),
    ALTER COLUMN kurs_jual   TYPE NUMERIC(20,8),
    ALTER COLUMN kurs_tengah TYPE NUMERIC(20,8);

-- Kolom tambahan P5-M5
ALTER TABLE mst.kurs
    ADD COLUMN rate_deviation_pct NUMERIC(8,4),
    ADD COLUMN deviation_flag     BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN jisdor_fetch_metadata JSONB,
    ADD COLUMN reject_reason      TEXT,
    ADD COLUMN upload_batch_id    UUID REFERENCES sys.upload_batch(id);

-- CHECK: reject_reason wajib jika REJECTED
ALTER TABLE mst.kurs
    ADD CONSTRAINT chk_kurs_reject_reason
        CHECK (workflow_status != 'REJECTED' OR (reject_reason IS NOT NULL AND length(reject_reason) >= 20));
```

### B. CREATE TABLE `sys.holiday_calendar`
```sql
CREATE TABLE sys.holiday_calendar (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tanggal     DATE NOT NULL,
    nama_libur  VARCHAR(100) NOT NULL,
    tipe        VARCHAR(20) NOT NULL CHECK (tipe IN ('NASIONAL','CUTI_BERSAMA')),
    tahun       SMALLINT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by  UUID NOT NULL,
    tenant_id   TEXT NOT NULL DEFAULT 'TUGURE',
    CONSTRAINT uq_holiday_tanggal UNIQUE (tanggal)
);
CREATE INDEX idx_holiday_tahun ON sys.holiday_calendar(tahun);
```

### C. Indexes Tambahan
```sql
-- Untuk FX treatment query (lookup by instrumen → mst.instrumen join)
-- Tidak ada index tambahan di mst.kurs — existing indexes cukup

-- Rate deviation monitoring
CREATE INDEX idx_kurs_deviation ON mst.kurs(deviation_flag, tanggal_berlaku DESC)
    WHERE deviation_flag = TRUE AND deleted_at IS NULL;
```

**Migration number**: 000039 (setelah 000038 periode-close P5-M4)
**Catatan**: `mst.kurs.locked_flag` sudah ada di init_schema + trigger `tg_kurs_locked_check` sudah ada — tidak perlu dibuat ulang.

---

## Compliance & Security Handoff Checklist

### Untuk ifrs9-compliance-reviewer (BLOCKING gate — khusus S5)
- [ ] FX treatment routing matrix (S5) — verifikasi bahwa `FVOCI_DEBT → OCI` sesuai PSAK 71 §5.7.10 dan `FVOCI_ELECTION → OCI no-recycling` sesuai §5.7.5
- [ ] `AC` dan `FVTPL` FCY instrumen → FX ke P&L: konfirmasi bahwa ini distinct dari MTM P&L (event code berbeda: `FX_UNREALIZED` vs `MTM_FVTPL`)
- [ ] `NO_FX_TREATMENT` untuk instrumen IDR — konfirmasi tidak ada edge case di mana IDR instrumen bisa punya FX exposure (mis. embedded derivative)
- [ ] Deviasi 20% threshold — apakah ada guidance PSAK 71 / OJK untuk maximum allowable rate swing yang tidak memerlukan second approval? Atau ini purely operational?
- [ ] FX routing decision harus konsisten dengan event codes di P5-M2 mapping jurnal seed — cross-check: `FX_OCI_RESERVE`, `FX_OCI_RESERVE_NO_RECYCLE`, `FX_REALIZED`, `FX_UNREALIZED` harus ada di mapping jurnal master (OQ-P5-B)

### Untuk security-engineer (advisory, tidak BLOCKING kecuali audit trail gap)
- [ ] `KURS.APPROVED` dan `KURS.REJECTED` ditulis in-transaction — verify implementasi backend tidak async
- [ ] `FxRateLockMiddleware` cek `locked_flag` dari DB (bukan cache) — hindari stale data bypass
- [ ] SoD enforcement `approver_id ≠ maker_id` di service layer, bukan hanya UI — test skenario "ROLE-AKUN dengan dual permission approve sendiri via direct API → 403 SOD_VIOLATION"
- [ ] `jisdor_fetch_metadata` JSONB tidak berisi credentials atau internal URL yang sensitif — cukup `{url_pattern, http_status, response_hash, retry_count}`
- [ ] ROLE-IT-ADMIN endpoint `POST /master/kurs/jisdor-sync` — rate limit 10 req/jam (bukan unlimited; bisa abuse)
- [ ] Export kurs `GET /master/kurs/export` — audit `KURS.EXPORT` in-transaction

### Untuk integration-engineer
- [ ] BI JISDOR URL discovery — tentukan: REST API atau web scraping. Implementasi `internal/integration/jisdor/` dengan interface `JISDORFetcher`
- [ ] Asynq cron registration: `"30 3 * * 1-5"` (10:30 WIB = 03:30 UTC) — verify timezone di docker/k8s TZ=Asia/Jakarta
- [ ] Holiday check: load `sys.holiday_calendar` setiap hari (cache TTL 24 jam di Redis), bukan query per-fetch
- [ ] DLQ insert pattern: mirror `sys.dead_letter_queue` dari P5-M3. Payload: `{job_type: "jisdor_fetch", tanggal, mata_uang_list, error_message, retry_count, exhausted_at}`
- [ ] Rate deviation compute: query `MAX(kurs_tengah) WHERE tanggal_berlaku = GREATEST(today−1, last_working_day_before(today)) AND kode_mata_uang = X` — handle weekend/holiday gaps (last working day, bukan naively yesterday)

### Untuk data-modeler
- [ ] Migration 000039: NUMERIC precision upgrade `kurs_beli/jual/tengah` ke `NUMERIC(20,8)` — verify tidak ada existing data yang truncated (BLIPS_init_schema seed `16000.0000` → safe untuk upgrade)
- [ ] Migration 000039: `sys.holiday_calendar` dengan seed hari libur nasional 2026 + 2027
- [ ] Migration 000039: `mst.kurs.deviation_flag` index partial `WHERE deviation_flag = TRUE` untuk monitoring dashboard
- [ ] Konfirmasi: `mst.kurs.locked_flag` trigger `tg_kurs_locked_check` (existing di init_schema) sudah handle BEFORE INSERT juga? Atau hanya BEFORE UPDATE OR DELETE? Jika hanya UPDATE/DELETE → perlu tambahkan BEFORE INSERT check untuk tanggal dalam periode CLOSED (via lookup `mst.periode_buku.status_periode`)

---

_Story set ini siap dihandoff ke `system-analyst` untuk OpenAPI contract + state machine `mst.kurs.workflow_status`, dan ke `integration-engineer` untuk implementasi JISDOR Asynq worker. `ifrs9-compliance-reviewer` harus mereview S5 (FX treatment routing) sebelum implementasi. Data-modeler memulai migration 000039 paralel dengan backend implementasi._
