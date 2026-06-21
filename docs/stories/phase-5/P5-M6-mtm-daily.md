# P5-M6 — APP-D MTM Daily Job + Manual Upload: User Stories

**Story Set ID**: P5-M6
**Modul**: APP-D — Mark-to-Market Harian + Manual Upload (Phase 5, setelah P5-M5 FX)
**Status**: DRAFT — menunggu handoff ke `system-analyst` + `ifrs9-compliance-reviewer`
**Author**: business-analyst
**Tanggal**: 2026-06-18
**Linked FSD**: FSD-APP-D-PeriodeBuku-FX-Mapping-v1.0.docx §3 (MTM Harian), §B5.7.2A (FX OCI Reserve)
**Linked BRD**: BRD §6.4 (APP-D MTM), RACI: ROLE-AKUN-CTL (A), ROLE-AKUN (R), ROLE-RISK (C), ROLE-IT-ADMIN (C), ROLE-AUDIT (I)
**Linked Decision Log**:
- `DEC-016` (LOCKED) — `NUMERIC(20,8)` untuk harga dan kurs; `NUMERIC(20,4)` untuk nilai IDR; `shopspring/decimal` di Go — **never float64**
- `DEC-017` (LOCKED) — 4-eyes SoD: uploader (maker) ≠ override-approver; SoD enforced server-side
- `DEC-018` (LOCKED) — audit trail append-only, retensi 10+10 tahun
- `DEC-021` (LOCKED) — Idempotency-Key wajib di setiap mutating endpoint
- `DEC-022` (LOCKED) — cursor-based pagination

**Dependensi**:
- **P5-M1** (`mst.instrumen` approved + klasifikasi PSAK 71 locked) — MTM hanya dijalankan untuk instrumen dengan `status = 'ACTIVE'` dan `klasifikasi_psak71 IS NOT NULL AND klasifikasi_locked = TRUE`
- **P5-M2** (jurnal engine) — MTM delta diposting via P5-M2 `POST /api/v1/jurnal/post-entry` dengan event code yang sesuai klasifikasi
- **P5-M5** (`mst.kurs` APPROVED) — instrumen FCY memerlukan `kurs_tengah` terbaru dari `mst.kurs` untuk konversi EAD_IDR; jika kurs belum tersedia → MTM row berstatus `STALE_PRICE` (kurs stale, bukan harga stale)
- **Holiday Calendar** (`sys.holiday_calendar` dari migration 000039) — cron skip pada hari libur nasional

**Handoff berikutnya**:
- `system-analyst` → OpenAPI fragment: 5 endpoints (`GET /trx/mtm`, `GET /trx/mtm/{id}`, `POST /trx/mtm/upload`, `POST /trx/mtm/{id}/override-approve`, `POST /trx/mtm/{id}/override-reject`); state machine `trx.mtm.status`; error codes baru (lihat §Error Codes Proposed)
- `data-modeler` → migration 000040 (`trx.mtm` tabel utama), migration 000041 (`sys.upload_batch` rows MTM — extend existing pattern)
- `ifrs9-compliance-reviewer` → **BLOCKING gate** untuk: (a) routing MTM_FVOCI / MTM_FVTPL / MTM_FVOCI_ELECTION / MTM_FVTPL_POCI per klasifikasi; (b) FX OCI Reserve entry per §B5.7.2A (FVOCI debt FCY); (c) AC skip logic; (d) POCI treatment
- `security-engineer` → review SoD enforcement on override, audit trail completeness, idempotency pada override endpoint
- `ecl-eir-engineer` → POCI instrument EAD sudah PD-adjusted — konfirmasi bahwa MTM_FVTPL_POCI tidak double-count ECL adjustment

**Compliance path**: P5-M6 adalah **regulated path** — menyentuh klasifikasi PSAK 71, OCI vs P&L routing, FVOCI Election irrevocable. **ifrs9-compliance-reviewer BLOCKING** wajib sebelum implementasi backend S5 (jurnal routing). Security review BLOCKING untuk S3 (override SoD) dan S4 (override approve).

---

## Konteks & Arsitektur P5-M6

### Alur MTM Harian

```
Hari kerja (Senin–Jumat, bukan hari libur nasional)
  │
  18:00 WIB — Asynq cron job "trx:mtm_daily_run"
  │    Check sys.holiday_calendar → skip jika hari libur
  │    For each mst.instrumen WHERE:
  │       status = 'ACTIVE'
  │       AND klasifikasi_psak71 IN ('FVOCI_DEBT','FVTPL','FVOCI_ELECTION','POCI')
  │       AND deleted_at IS NULL
  │    Fetch harga terbaru:
  │       IBPA → Obligasi (bond closing price)
  │       BEI  → Saham (equity closing price)
  │       KSEI/MI → NAB Reksadana (NAV per unit)
  │    Check STALE_PRICE: tanggal_harga vs tanggal_hari_ini
  │       Jika harga_age > STALE_PRICE_THRESHOLD_DAYS (default 3) → flag STALE_PRICE
  │    Compute MTM delta:
  │       delta_idr = (harga_pasar_idr − harga_buku_idr) × jumlah_unit
  │       Jika FCY: harga_pasar_idr = harga_pasar_fcy × kurs_tengah (dari mst.kurs APPROVED)
  │    Check price-deviation: |delta_pct| > PRICE_DEVIATION_THRESHOLD_PCT
  │       Jika TRUE → status = 'PENDING_REVIEW', notif ke ROLE-AKUN-CTL
  │       Jika FALSE → status = 'AUTO_POSTED'
  │    Insert trx.mtm row
  │    Jika AUTO_POSTED → call P5-M2 jurnal engine (event code per routing S5)
  │    Audit: MTM.AUTO_POSTED atau MTM.PENDING_REVIEW per instrumen
  │    Failure: retry 3×, lalu DLQ + alert ke ROLE-IT-ADMIN
  │
  Manual Upload (ROLE-AKUN — untuk harga override atau instrumen yang tidak ada di feed)
  │    POST /trx/mtm/upload (XLSX/CSV per template)
  │    status = 'PENDING_REVIEW' → queue ke ROLE-AKUN-CTL
  │
  Override Approve/Reject (ROLE-AKUN-CTL — SoD vs uploader)
  │    POST /trx/mtm/{id}/override-approve
  │    POST /trx/mtm/{id}/override-reject (comment ≥ 30 char)
  │    Jika approved → call P5-M2 jurnal engine
```

### State Machine `trx.mtm.status`

```
PENDING_REVIEW (deviation > threshold ATAU manual upload ATAU STALE_PRICE override request)
  │
  ├─ [override-approve] ROLE-AKUN-CTL (SoD: approver_id ≠ uploader_id)
  │    → status = 'APPROVED'
  │    → call P5-M2 jurnal engine (event code per klasifikasi)
  │    → audit MTM.OVERRIDE_APPROVED in-transaction
  │
  ├─ [override-reject] ROLE-AKUN-CTL (comment ≥ 30 char wajib)
  │    → status = 'REJECTED'
  │    → audit MTM.OVERRIDE_REJECTED in-transaction
  │    → notifikasi ke uploader (manual upload) atau ROLE-AKUN (cron row)
  │
AUTO_POSTED (delta_pct ≤ threshold, harga fresh — cron path normal)
  │    → jurnal sudah diposting oleh cron worker
  │    → read-only setelah ini (kecuali periode hard-close lock)
  │
APPROVED (override-approve dari PENDING_REVIEW)
  │    → jurnal sudah diposting via P5-M2
  │    → read-only
  │
REJECTED
  │    → jurnal tidak diposting
  │    → soft-delete candidate jika tidak dibutuhkan
  │
STALE_PRICE (harga_age > threshold, belum ada override)
  │    → tidak diposting ke jurnal
  │    → alert ROLE-AKUN-CTL untuk manual override atau konfirmasi harga
  │
[periode hard-close via P5-M4]
  → semua trx.mtm rows dalam periode: locked_flag = TRUE
  → tidak bisa mutasi setelah lock
```

### Sumber Harga per Tipe Instrumen

| Tipe Instrumen | Feed Sumber | Tabel Staging |
|---|---|---|
| Obligasi (bond) | IBPA daily SFTP CSV | `sys.ibpa_feed_staging` (P5-M3) |
| Saham (equity) | BEI closing price file | `sys.bei_feed_staging` (P5-M3) |
| Reksadana (mutual fund) | KSEI/MI NAB harian | `sys.ksei_feed_staging` (P5-M3) |
| Deposito / AC instrumen | **SKIP** — tidak ada MTM untuk AC | — |

### Jurnal Routing per Klasifikasi (Compliance-Critical)

| Klasifikasi PSAK 71 | Mata Uang | Event Code Jurnal | Akun Debit | Akun Kredit | Notes |
|---|---|---|---|---|---|
| `FVOCI_DEBT` | IDR | `MTM_FVOCI` | `Aset Investasi FVOCI` | `OCI — Perubahan Nilai Wajar` | PSAK 71 §5.7.10 |
| `FVOCI_DEBT` | FCY | `MTM_FVOCI` + `MTM_FX_OCI_RESERVE` | idem + `OCI — FX Reserve` | idem + `Selisih Kurs OCI` | §B5.7.2A: MTM + FX OCI Reserve terpisah |
| `FVOCI_ELECTION` | IDR/FCY | `MTM_FVOCI_ELECTION` | `Aset Investasi FVOCI Ekuitas` | `OCI — Perubahan Nilai Wajar Ekuitas` | No recycling ke P&L on disposal |
| `FVTPL` | IDR/FCY | `MTM_FVTPL` | `Aset Investasi FVTPL` | `P&L — Keuntungan/Kerugian Fair Value` | PSAK 71 §5.7.7 |
| `AC` | Any | **SKIP** | — | — | Tidak ada MTM untuk AC |
| `POCI` | IDR/FCY | `MTM_FVTPL_POCI` | `Aset Investasi POCI` | `P&L — POCI Fair Value` | Credit-adjusted; no Stage escalation |

### Schema P5-M6

#### `trx.mtm` (baru — migration 000040)

| Kolom | Tipe | Keterangan |
|---|---|---|
| `id` | UUID PK DEFAULT gen_random_uuid() | |
| `instrumen_id` | UUID NOT NULL FK `mst.instrumen(id)` | |
| `periode_bulanan_id` | UUID NOT NULL FK `mst.periode_buku(id)` | |
| `tanggal_mtm` | DATE NOT NULL | Tanggal penilaian |
| `harga_pasar_fcy` | NUMERIC(20,8) | Harga pasar dalam mata uang asal (NULL jika IDR) |
| `harga_pasar_idr` | NUMERIC(20,4) NOT NULL | Harga pasar sudah dikonversi ke IDR |
| `harga_buku_idr` | NUMERIC(20,4) NOT NULL | Nilai buku saat MTM dihitung |
| `delta_idr` | NUMERIC(20,4) NOT NULL | `harga_pasar_idr − harga_buku_idr` |
| `delta_pct` | NUMERIC(8,4) NOT NULL | `(delta_idr / harga_buku_idr) × 100` |
| `kurs_id` | UUID FK `mst.kurs(id)` | NULL jika IDR instrumen |
| `kurs_tengah` | NUMERIC(20,8) | Snapshot kurs saat MTM (DEC-016) |
| `harga_sumber` | VARCHAR(30) NOT NULL | `'IBPA'`, `'BEI'`, `'KSEI'`, `'MANUAL'` |
| `harga_tanggal` | DATE NOT NULL | Tanggal harga yang dipakai (bisa berbeda dari tanggal_mtm jika stale) |
| `harga_age_days` | SMALLINT NOT NULL | `tanggal_mtm − harga_tanggal` |
| `stale_price_flag` | BOOLEAN NOT NULL DEFAULT FALSE | TRUE jika `harga_age_days > STALE_PRICE_THRESHOLD_DAYS` |
| `deviation_flag` | BOOLEAN NOT NULL DEFAULT FALSE | TRUE jika `ABS(delta_pct) > PRICE_DEVIATION_THRESHOLD_PCT` |
| `status` | VARCHAR(20) NOT NULL | `AUTO_POSTED`, `PENDING_REVIEW`, `APPROVED`, `REJECTED`, `STALE_PRICE` |
| `jurnal_entry_id` | UUID FK `jrnl.jurnal_entry(id)` | NULL hingga jurnal diposting |
| `jurnal_event_code` | VARCHAR(50) | Event code yang digunakan saat posting |
| `uploader_id` | UUID FK `sec.user(id)` | NULL untuk cron auto; user untuk manual upload |
| `override_approver_id` | UUID FK `sec.user(id)` | NULL hingga override-approve/reject |
| `override_comment` | TEXT | Wajib ≥ 30 char jika reject |
| `override_at` | TIMESTAMPTZ | |
| `upload_batch_id` | UUID FK `sys.upload_batch(id)` | NULL untuk cron auto |
| `locked_flag` | BOOLEAN NOT NULL DEFAULT FALSE | TRUE setelah periode hard-close |
| `cron_job_id` | TEXT | Asynq job ID dari run yang generate baris ini |
| — *audit columns* — | TIMESTAMPTZ/UUID | `created_at`, `created_by`, `updated_at`, `updated_by`, `deleted_at`, `deleted_by`, `row_version`, `tenant_id` |
| UNIQUE | `(instrumen_id, tanggal_mtm, harga_sumber)` | Satu MTM per instrumen per tanggal per sumber |

#### Indexes (migration 000040)

```sql
CREATE INDEX idx_mtm_instrumen_tanggal ON trx.mtm(instrumen_id, tanggal_mtm DESC);
CREATE INDEX idx_mtm_status            ON trx.mtm(status, tanggal_mtm DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_mtm_stale_flag        ON trx.mtm(stale_price_flag, tanggal_mtm DESC) WHERE stale_price_flag = TRUE AND deleted_at IS NULL;
CREATE INDEX idx_mtm_deviation_flag    ON trx.mtm(deviation_flag, tanggal_mtm DESC) WHERE deviation_flag = TRUE AND deleted_at IS NULL;
CREATE INDEX idx_mtm_periode           ON trx.mtm(periode_bulanan_id, tanggal_mtm DESC);
-- Partisi bulanan via pg_partman (range created_at, mirror trx.transaction)
```

---

## Story P5-M6-S1 — Daily MTM Cron 18:00 WIB (Asynq Scheduled Job)

**Actor**: System (Asynq worker `trx:mtm_daily_run`) — dipicu otomatis; jika gagal → ROLE-IT-ADMIN + ROLE-AKUN-CTL dinotifikasi
**Trigger**: Asynq cron `"00 11 * * 1-5"` (18:00 WIB = 11:00 UTC, Senin–Jumat). Worker pertama cek `sys.holiday_calendar` untuk hari ini. Jika hari libur → skip + log advisory. Jika hari kerja → jalankan MTM run.
**Goal**: Setiap hari kerja, worker fetch harga terbaru per instrumen dari feed (IBPA/BEI/KSEI), hitung delta nilai wajar, insert `trx.mtm` row. Baris AUTO_POSTED langsung diposting ke jurnal via P5-M2. Baris dengan deviasi > threshold masuk PENDING_REVIEW. Instrumen tanpa harga → STALE_PRICE. AC instrumen di-skip. Cron failure → DLQ + alert.

### Pre-conditions
1. Hari adalah hari kerja (Senin–Jumat, bukan hari libur nasional di `sys.holiday_calendar`)
2. Asynq worker `trx:mtm_daily_run` ter-schedule dan aktif
3. Feed IBPA/BEI/KSEI sudah diproses untuk hari ini (P5-M3 feed jobs berjalan lebih awal — 16:00 WIB)
4. `mst.kurs` APPROVED untuk `tanggal_berlaku = today` sudah tersedia untuk FCY instrumen (dari P5-M5 10:30 WIB)
5. `sys.config` key `PRICE_DEVIATION_THRESHOLD_PCT` (default 5) dan `STALE_PRICE_THRESHOLD_DAYS` (default 3) dikonfigurasi

### Compute Logic

```
For each mst.instrumen WHERE:
    status = 'ACTIVE'
    AND klasifikasi_psak71 IN ('FVOCI_DEBT','FVTPL','FVOCI_ELECTION','POCI')
    AND deleted_at IS NULL:

    1. Cek UNIQUE(instrumen_id, tanggal_mtm, harga_sumber):
       - Sudah ada dan status != 'REJECTED' → SKIP (idempotent)
       - Sudah ada dan status = 'REJECTED' → insert row baru (re-run untuk rejected)

    2. Fetch harga dari feed staging tabel sesuai tipe instrumen
       - Jika tidak ditemukan: harga_age_days = INFINITY → STALE_PRICE

    3. Hitung harga_age_days = tanggal_mtm − harga_tanggal
       - Jika harga_age_days > STALE_PRICE_THRESHOLD_DAYS → stale_price_flag = TRUE

    4. Jika FCY: harga_pasar_idr = harga_pasar_fcy × kurs_tengah (mst.kurs APPROVED tanggal_mtm)
       - Jika kurs tidak tersedia → stale_price_flag = TRUE, status = STALE_PRICE

    5. delta_idr = harga_pasar_idr − harga_buku_idr
       delta_pct  = (delta_idr / harga_buku_idr) × 100

    6. Tentukan status:
       - stale_price_flag = TRUE → status = 'STALE_PRICE'
       - ABS(delta_pct) > PRICE_DEVIATION_THRESHOLD_PCT → deviation_flag = TRUE, status = 'PENDING_REVIEW'
       - Else → status = 'AUTO_POSTED'

    7. INSERT trx.mtm (dalam transaksi per instrumen, bukan satu transaksi raksasa)

    8. Jika status = 'AUTO_POSTED':
       → Call P5-M2 POST /api/v1/jurnal/post-entry (event_code dari routing matrix S5)
       → Update trx.mtm.jurnal_entry_id + jurnal_event_code

    9. Audit MTM.AUTO_POSTED atau MTM.PENDING_REVIEW atau MTM.STALE_PRICE_ALERT in-transaction

Jika worker error pada instrumen tertentu (mis. harga parse error):
    → Log error, lanjut ke instrumen berikutnya (partial run acceptable)
    → Setelah loop selesai: jika error_count > 0 → INSERT DLQ + alert ke ROLE-IT-ADMIN

Jika failure catastrophic (DB down, semua fetch gagal):
    → Asynq retry max 3×, interval 15 menit
    → Setelah 3 retry: DLQ + alert ROLE-IT-ADMIN + ROLE-AKUN-CTL
```

### Job Progress (rule §3 UX)

Worker menerbitkan progress via `sys.job` + Redis pub/sub (SSE):
- Phase 1: "Membaca daftar instrumen aktif non-AC ({N} instrumen)..."
- Phase 2: "Menghitung MTM instrumen {i} dari {N}: {kode_instrumen}"
- Phase 3: "Posting jurnal AUTO_POSTED ({X} transaksi)..."
- Complete: "MTM run selesai: {auto_posted} auto-posted, {pending_review} pending review, {stale_price} stale price, {skipped_ac} skipped (AC)."

### Data References

| Tabel | Akses | Catatan |
|---|---|---|
| `mst.instrumen` | READ | Daftar instrumen ACTIVE non-AC |
| `mst.kurs` | READ | Kurs tengah APPROVED untuk tanggal_mtm (FCY instrumen) |
| `sys.ibpa_feed_staging` | READ | Harga obligasi dari P5-M3 |
| `sys.bei_feed_staging` | READ | Harga saham dari P5-M3 |
| `sys.ksei_feed_staging` | READ | NAB Reksadana dari P5-M3 |
| `sys.holiday_calendar` | READ | Cek hari libur |
| `sys.config` | READ | Threshold parameters |
| `trx.mtm` | INSERT | Row per instrumen |
| `jrnl.jurnal_entry` | INSERT (via P5-M2) | Hanya untuk AUTO_POSTED |
| `sys.dead_letter_queue` | INSERT | Jika 3 retry exhausted |
| `sys.job` | READ + UPDATE | Progress tracking |
| `aud.audit_log` | INSERT | Per baris: in-transaction |

### Permissions

| Actor | Aksi | Catatan |
|---|---|---|
| System (Asynq worker) | INSERT `trx.mtm`, call P5-M2 | Service account |
| ROLE-IT-ADMIN | `POST /api/v1/mtm/trigger-run` | Manual trigger jika cron terlewat |
| ROLE-AKUN-CTL | `GET /trx/mtm?filter[status]=PENDING_REVIEW` | Review antrian |

### Audit Events

| Action | Trigger |
|---|---|
| `MTM.AUTO_POSTED` | Row status=AUTO_POSTED + jurnal diposting — in-transaction. `after_jsonb`: `{instrumen_id, tanggal_mtm, delta_idr, delta_pct, jurnal_entry_id, event_code}` |
| `MTM.PENDING_REVIEW` | Row status=PENDING_REVIEW karena deviation — in-transaction. `after_jsonb`: `{instrumen_id, delta_pct, threshold_pct}` |
| `MTM.STALE_PRICE_ALERT` | Row status=STALE_PRICE — in-transaction. `after_jsonb`: `{instrumen_id, harga_tanggal, harga_age_days, threshold_days}` |
| `MTM.HOLIDAY_SKIP` | Hari libur — advisory log |
| `MTM.CRON_FAILED` | DLQ setelah 3 retry — advisory + alert |
| `MTM.INSTRUMEN_AC_SKIP` | Instrumen AC di-skip — advisory log (batch per run, bukan per instrumen) |

### Acceptance Criteria

```gherkin
Feature: MTM daily cron 18:00 WIB — Asynq worker hari kerja

  Background:
    Given sys.config: PRICE_DEVIATION_THRESHOLD_PCT = 5, STALE_PRICE_THRESHOLD_DAYS = 3
    And tanggal hari ini = 2026-06-18 (Kamis, bukan hari libur)
    And mst.periode_buku PRD-2026-06: status_periode = 'OPEN'
    And mst.instrumen aktif:
      | OBL-0042 | FVOCI_DEBT | USD | klasifikasi_locked = TRUE |
      | SHM-0015 | FVOCI_ELECTION | IDR | klasifikasi_locked = TRUE |
      | DEP-0001 | AC | IDR | — |
    And mst.kurs USD 2026-06-18: kurs_tengah = 16250.00000000, workflow_status = 'APPROVED'
    And sys.ibpa_feed_staging: OBL-0042 harga_pasar_fcy = 98.50, harga_tanggal = 2026-06-18

  # ─── HAPPY PATH: AC skip, FVOCI_DEBT auto-post, FVOCI_ELECTION auto-post ─────

  Scenario: S1-AC1 — Cron berjalan normal: AC di-skip, 2 instrumen non-AC di-proses auto-post
    Given sys.bei_feed_staging: SHM-0015 harga_pasar_idr = 1850.00, harga_tanggal = 2026-06-18
    And harga_buku OBL-0042 = 985000.0000 IDR (99.00 × 10.000 unit × kurs kemarin 16200)
    And harga_buku SHM-0015 = 180000.0000 IDR
    When Asynq worker "trx:mtm_daily_run" dieksekusi pada 18:00 WIB 2026-06-18
    Then DEP-0001 (AC): tidak ada INSERT ke trx.mtm
    And aud.audit_log: MTM.INSTRUMEN_AC_SKIP batch entry (bukan per instrumen)
    And OBL-0042:
      | harga_pasar_idr = 98.50 × 10.000 unit × 16250 = 16.006.250.0000 IDR |
      | delta_idr       = 16.006.250 − 985.000 (harga_buku) = tergantung jumlah unit |
      | delta_pct       = (delta_idr / harga_buku) × 100 — cek < 5%                 |
      | stale_price_flag = FALSE (harga_age_days = 0)                                |
      | status           = 'AUTO_POSTED'                                              |
      | jurnal_event_code = 'MTM_FVOCI' + 'MTM_FX_OCI_RESERVE' (FCY §B5.7.2A)      |
      | jurnal_entry_id  = non-null (diposting via P5-M2)                            |
      | aud.audit_log.action = MTM.AUTO_POSTED — in-transaction                      |
    And SHM-0015: status = 'AUTO_POSTED', jurnal_event_code = 'MTM_FVOCI_ELECTION'
    And notifikasi ke ROLE-AKUN-CTL: "MTM run 2026-06-18 selesai: 2 auto-posted, 0 pending review, 0 stale price, 1 AC skipped."
    And jika worker dijalankan ulang (idempotent test):
      | cek UNIQUE(OBL-0042, 2026-06-18, 'IBPA') → row sudah ada status=AUTO_POSTED |
      | tidak ada INSERT kedua                                                        |

  # ─── WARNING: Deviasi > 5% → PENDING_REVIEW, tidak auto-post jurnal ─────────

  Scenario: S1-AC2 — Instrumen dengan delta_pct > threshold masuk PENDING_REVIEW, jurnal tidak diposting
    Given sys.ibpa_feed_staging: OBL-0042 harga_pasar_fcy = 90.00 (vs buku 99.00 — delta > 5%)
    When worker "trx:mtm_daily_run" dieksekusi
    Then OBL-0042: status = 'PENDING_REVIEW', deviation_flag = TRUE, jurnal_entry_id = NULL
    And aud.audit_log.action = MTM.PENDING_REVIEW — in-transaction
    And notifikasi WAJIB ke ROLE-AKUN-CTL:
      "PERHATIAN: MTM OBL-0042 tanggal 2026-06-18 memiliki deviasi harga {X}% melebihi threshold 5%. Review dan override diperlukan sebelum jurnal diposting. Buka /trx/mtm?filter[status]=PENDING_REVIEW"
    And jurnal TIDAK diposting ke P5-M2

  # ─── STALE_PRICE: Harga > 3 hari — STALE_PRICE flag, tidak auto-post ────────

  Scenario: S1-AC3 — Harga IBPA OBL-0042 terakhir 5 hari lalu — STALE_PRICE alert
    Given sys.ibpa_feed_staging: OBL-0042 harga_tanggal = 2026-06-13 (5 hari lalu, threshold = 3)
    When worker "trx:mtm_daily_run" dieksekusi
    Then OBL-0042: status = 'STALE_PRICE', stale_price_flag = TRUE, harga_age_days = 5, jurnal_entry_id = NULL
    And aud.audit_log.action = MTM.STALE_PRICE_ALERT — in-transaction
    And notifikasi WAJIB ke ROLE-AKUN-CTL:
      "PERHATIAN: Harga OBL-0042 belum diperbarui selama 5 hari (terakhir: 2026-06-13). MTM tidak dapat diposting otomatis. Lakukan manual upload harga atau konfirmasi harga ke IBPA."
    And jurnal TIDAK diposting

  # ─── ERROR: 3 retry exhausted — DLQ + alert ROLE-IT-ADMIN ───────────────────

  Scenario: S1-AC4 — Feed staging tidak tersedia (DB koneksi gagal) — DLQ + alert
    Given koneksi ke DB staging tabel gagal (timeout) untuk semua instrumen
    When worker retry 3× dengan interval 15 menit
    Then setelah retry ke-3 gagal:
      | sys.dead_letter_queue INSERT: job_type = 'mtm_daily_run', tanggal = 2026-06-18, error = "DB timeout after 3 retries" |
      | aud.audit_log.action = MTM.CRON_FAILED (advisory)                                                                     |
    And notifikasi WAJIB ke ROLE-IT-ADMIN: "MTM daily run 2026-06-18 gagal setelah 3 retry. DLQ entry dibuat. Periksa koneksi DB dan feed staging."
    And notifikasi ke ROLE-AKUN-CTL: "MTM harian 2026-06-18 belum berjalan karena error teknis. Hubungi IT untuk penanganan."
    And tidak ada INSERT ke trx.mtm
```

---

## Story P5-M6-S2 — Manual Upload MTM (ROLE-AKUN — Maker)

**Actor**: ROLE-AKUN (Maker)
**Trigger**: Harga instrumen tidak tersedia di feed otomatis (mis. obligasi non-IBPA, saham OTC), atau ROLE-AKUN ingin mengoreksi harga yang stale dengan sumber manual. ROLE-AKUN membuka `/trx/mtm` dan klik "Upload MTM Manual".
**Goal**: ROLE-AKUN upload XLSX/CSV berisi harga pasar per instrumen. Sistem memvalidasi: instrumen harus ACTIVE non-AC, tanggal dalam periode OPEN, format harga valid (> 0, presisi 8 desimal), mata uang konsisten dengan `mst.instrumen`. Semua baris masuk `status = 'PENDING_REVIEW'`. ROLE-AKUN-CTL dinotifikasi untuk review.

### Pre-conditions
1. User ter-autentikasi dengan permission `mtm.create`
2. Request mengandung `Idempotency-Key` header (UUID v4)
3. `mst.periode_buku.status_periode = 'OPEN'` untuk periode yang mencakup `tanggal_mtm`
4. Instrumen dalam file harus: `status = 'ACTIVE'`, `klasifikasi_psak71 NOT NULL`, bukan AC

### Upload Template (XLSX/CSV)

| Kolom | Tipe | Keterangan |
|---|---|---|
| `kode_instrumen` | VARCHAR(20) REQUIRED | Harus ada di `mst.instrumen` dan ACTIVE |
| `tanggal_mtm` | DATE REQUIRED | Format YYYY-MM-DD, dalam periode OPEN |
| `harga_pasar` | NUMERIC REQUIRED | > 0, presisi 8 desimal. Dalam mata uang instrumen (IDR atau FCY) |
| `harga_sumber` | VARCHAR(30) OPTIONAL | Default `'MANUAL'`; bisa `'IBPA_MANUAL'`, `'BEI_MANUAL'` |
| `catatan` | TEXT OPTIONAL | Wajib jika override existing AUTO_POSTED atau PENDING_REVIEW row |

### Endpoint

```
POST /api/v1/trx/mtm/upload
Authorization: Bearer <jwt>
Idempotency-Key: <uuid-v4>
Content-Type: multipart/form-data

Body:
  file: <xlsx atau csv>
  catatan_upload: "Upload harga manual OBL-0088 dari Bloomberg 2026-06-18"
  tanggal_mtm_override: "2026-06-18"  ← opsional

→ 202 Accepted
{
  "data": {
    "upload_batch_id": "<uuid>",
    "rows_parsed": 2,
    "rows_valid": 2,
    "rows_invalid": 0,
    "status": "PENDING_REVIEW",
    "mtm_ids": ["<uuid-OBL-0088>", "<uuid-SHM-0099>"],
    "next_step": "Menunggu approval ROLE-AKUN-CTL. Hubungi Finance Controller untuk review.",
    "stale_price_warnings": [],
    "deviation_warnings": ["OBL-0088: delta_pct = 7.3% > threshold 5%"]
  }
}
```

### Data References

| Tabel | Akses | Catatan |
|---|---|---|
| `mst.instrumen` | READ | Validasi kode, status, klasifikasi, mata_uang |
| `mst.kurs` | READ | Konversi FCY → IDR jika mata_uang instrumen FCY |
| `mst.periode_buku` | READ | Validasi status_periode = 'OPEN' |
| `trx.mtm` | INSERT | `status = 'PENDING_REVIEW'`, `harga_sumber = 'MANUAL'` |
| `sys.upload_batch` | INSERT | Tracking batch, mirror P5-M5 pattern |
| `aud.audit_log` | INSERT | `MTM.UPLOADED` — in-transaction |
| `doc.upload` | INSERT | File original ke MinIO |

### Audit Events

| Action | Trigger |
|---|---|
| `MTM.UPLOADED` | Setiap row valid di-insert — in-transaction. `after_jsonb`: `{instrumen_id, tanggal_mtm, harga_pasar, harga_sumber, upload_batch_id, deviation_flag}` |

### Acceptance Criteria

```gherkin
Feature: Manual upload MTM oleh ROLE-AKUN (Maker, PENDING_REVIEW)

  Background:
    Given user ROLE-AKUN (USR-AKUN-001) ter-autentikasi dengan permission mtm.create
    And mst.periode_buku PRD-2026-06: status_periode = 'OPEN'
    And mst.instrumen OBL-0088: status = 'ACTIVE', klasifikasi_psak71 = 'FVOCI_DEBT', mata_uang = 'USD'
    And mst.kurs USD 2026-06-18: kurs_tengah = 16250.00000000, workflow_status = 'APPROVED'
    And belum ada trx.mtm untuk OBL-0088 tanggal 2026-06-18

  # ─── HAPPY PATH: Upload 1 instrumen, masuk PENDING_REVIEW ─────────────────

  Scenario: S2-AC1 — ROLE-AKUN berhasil upload harga manual OBL-0088 — PENDING_REVIEW
    Given file mtm_manual_20260618.xlsx berisi:
      | kode_instrumen | tanggal_mtm | harga_pasar | harga_sumber |
      | OBL-0088       | 2026-06-18  | 97.50       | IBPA_MANUAL  |
    When USR-AKUN-001 mengirim POST /api/v1/trx/mtm/upload
      With Idempotency-Key: IK-MTM-UPL-001
    Then HTTP 202
    And trx.mtm INSERT OBL-0088:
      | harga_pasar_fcy    | 97.50000000 (NUMERIC(20,8))           |
      | harga_pasar_idr    | 97.50 × kurs × unit (NUMERIC(20,4)) |
      | status             | PENDING_REVIEW                        |
      | harga_sumber       | IBPA_MANUAL                           |
      | uploader_id        | USR-AKUN-001 UUID                     |
      | upload_batch_id    | <uuid batch>                          |
    And dalam satu transaksi:
      | aud.audit_log.action = MTM.UPLOADED                          |
      | aud.audit_log.actor_user_id = USR-AKUN-001 UUID              |
      | doc.upload → MinIO (file original tersimpan)                 |
    And toast ke USR-AKUN-001: "1 MTM berhasil di-upload untuk 2026-06-18. Status: Menunggu approval Finance Controller."
    And notifikasi ke ROLE-AKUN-CTL: "1 MTM manual 2026-06-18 menunggu approval dari USR-AKUN-001. Review di /trx/mtm?filter[status]=PENDING_REVIEW"

  # ─── ERROR: Instrumen AC diupload — ditolak MTM_INSTRUMEN_AC_SKIP ────────

  Scenario: S2-AC2 — Upload harga untuk instrumen AC — ditolak
    Given file berisi kode_instrumen = DEP-0001 (AC instrumen)
    When USR-AKUN-001 mengirim POST /api/v1/trx/mtm/upload
      With Idempotency-Key: IK-MTM-UPL-002
    Then HTTP 422:
      | error.code              | MTM_INSTRUMEN_AC_SKIP                                |
      | error.details[0].field  | kode_instrumen                                       |
      | error.details[0].rule   | "DEP-0001 berklasifikasi AC — tidak ada MTM untuk AC per PSAK 71. Hapus baris ini dari file upload." |
    And tidak ada INSERT ke trx.mtm

  # ─── ERROR: Periode CLOSED — upload ditolak MTM_PERIODE_LOCKED ──────────

  Scenario: S2-AC3 — Upload MTM untuk tanggal dalam periode CLOSED
    Given mst.periode_buku PRD-2026-05: status_periode = 'CLOSED'
    When USR-AKUN-001 mengirim POST /api/v1/trx/mtm/upload
      With file berisi tanggal_mtm = 2026-05-15 (dalam PRD-2026-05 CLOSED)
      With Idempotency-Key: IK-MTM-UPL-003
    Then HTTP 423:
      | error.code    | MTM_PERIODE_LOCKED                                              |
      | error.message | "Periode PRD-2026-05 sudah hard-closed. Tidak bisa menambah MTM untuk 2026-05-15." |
    And tidak ada INSERT ke trx.mtm

  # ─── ERROR: Idempotency replay — return original response ────────────────

  Scenario: S2-AC4 — Upload ulang dengan Idempotency-Key yang sama dan payload identik — replay
    Given USR-AKUN-001 sebelumnya berhasil upload dengan Idempotency-Key: IK-MTM-UPL-001
    When USR-AKUN-001 mengirim POST /api/v1/trx/mtm/upload ulang
      With Idempotency-Key: IK-MTM-UPL-001 (sama)
      With file yang identik
    Then HTTP 200 (IDEMPOTENCY_REPLAY)
    And response berisi original response dari request pertama
    And tidak ada INSERT baru ke trx.mtm
```

---

## Story P5-M6-S3 — Price-Deviation WARNING + STALE_PRICE Alert

**Actor**: System (Asynq worker — menghasilkan flag) + ROLE-AKUN-CTL (menerima notifikasi dan memutuskan tindak lanjut)
**Trigger**: (a) Cron MTM mendeteksi `ABS(delta_pct) > PRICE_DEVIATION_THRESHOLD_PCT` → `deviation_flag = TRUE`, status = PENDING_REVIEW. (b) Cron MTM mendeteksi `harga_age_days > STALE_PRICE_THRESHOLD_DAYS` → `stale_price_flag = TRUE`, status = STALE_PRICE. (c) ROLE-AKUN-CTL dapat mengkonfigurasi threshold via `sys.config` (ROLE-IT-ADMIN write, ROLE-AKUN-CTL read).
**Goal**: ROLE-AKUN-CTL menerima notifikasi yang actionable, dapat melihat antrian PENDING_REVIEW dan STALE_PRICE di `/trx/mtm`, memutuskan: (a) override-approve dengan justifikasi, atau (b) override-reject dan minta ROLE-AKUN re-upload harga yang benar.

### Alert Content

| Kondisi | Notifikasi ke | Isi Notifikasi | Urgency |
|---|---|---|---|
| `deviation_flag = TRUE` | ROLE-AKUN-CTL | "MTM {instrumen}: delta {X}% > threshold {Y}%. Review required." | WARNING |
| `stale_price_flag = TRUE` | ROLE-AKUN-CTL | "Harga {instrumen} sudah {N} hari. MTM belum diposting. Tindakan: upload harga baru atau override-approve." | WARNING |
| `stale_price_flag = TRUE` > 5 hari | ROLE-AKUN-CTL + ROLE-RISK | Idem + eskalasi ke ROLE-RISK | HIGH |
| Kurs FCY tidak tersedia | ROLE-AKUN + ROLE-AKUN-CTL | "Kurs {FCY} untuk {tanggal} belum tersedia. MTM {instrumen} ditunda." | WARNING |

### Dashboard Filter

`GET /api/v1/trx/mtm?filter[status]=PENDING_REVIEW,STALE_PRICE&sort=tanggal_mtm:desc`

Kolom wajib di DataTable: `kode_instrumen`, `tanggal_mtm`, `klasifikasi_psak71`, `delta_idr`, `delta_pct`, `harga_age_days`, `status`, badge `DEVIATION` / `STALE` / keduanya, Actions (Override Approve / Override Reject).

### Acceptance Criteria

```gherkin
Feature: Price-deviation WARNING dan STALE_PRICE alert untuk ROLE-AKUN-CTL

  Background:
    Given sys.config: PRICE_DEVIATION_THRESHOLD_PCT = 5, STALE_PRICE_THRESHOLD_DAYS = 3
    And ROLE-AKUN-CTL (USR-AKUN-CTL-001) ter-autentikasi

  # ─── HAPPY PATH: List PENDING_REVIEW + STALE_PRICE di DataTable ──────────

  Scenario: S3-AC1 — ROLE-AKUN-CTL melihat antrian MTM PENDING_REVIEW dan STALE_PRICE
    Given trx.mtm berisi:
      | OBL-0042 | 2026-06-18 | PENDING_REVIEW | deviation_flag = TRUE | delta_pct = 7.3%   |
      | SHM-0099 | 2026-06-17 | STALE_PRICE    | harga_age_days = 4    | stale_price_flag = TRUE |
    When USR-AKUN-CTL-001 membuka GET /api/v1/trx/mtm?filter[status]=PENDING_REVIEW,STALE_PRICE
    Then HTTP 200 dengan 2 baris
    And setiap baris berisi: badge 'DEVIATION WARNING' (OBL-0042) dan 'STALE PRICE' (SHM-0099)
    And kolom action: tombol "Override Approve" dan "Override Reject" untuk setiap baris
    And total_estimate di pagination.totalEstimate = 2
    And response memiliki appliedFilter: { status: ["PENDING_REVIEW", "STALE_PRICE"] }

  # ─── STALE_PRICE > 5 hari — eskalasi ke ROLE-RISK ───────────────────────

  Scenario: S3-AC2 — Harga SHM-0099 sudah 6 hari — notifikasi dikirim ke ROLE-RISK juga
    Given trx.mtm SHM-0099 2026-06-18: status = 'STALE_PRICE', harga_age_days = 6
    When worker MTM atau job monitoring harian berjalan
    Then notifikasi ke ROLE-AKUN-CTL: "SHM-0099: harga tidak update 6 hari. MTM belum diposting."
    And notifikasi ke ROLE-RISK: "ESKALASI: Harga SHM-0099 sudah 6 hari tidak diperbarui. Risk review diperlukan untuk instrumen FVOCI_ELECTION ini."
    And trx.mtm.status tetap 'STALE_PRICE' (tidak berubah otomatis)

  # ─── Kurs FCY tidak tersedia — STALE_PRICE + notif ke ROLE-AKUN ─────────

  Scenario: S3-AC3 — MTM instrumen FCY gagal karena kurs USD hari ini belum di-upload
    Given mst.kurs USD 2026-06-18: tidak ada row (JISDOR gagal, manual belum upload)
    And mst.instrumen OBL-0042: mata_uang = 'USD'
    When worker "trx:mtm_daily_run" memproses OBL-0042
    Then OBL-0042: status = 'STALE_PRICE', stale_price_flag = TRUE
    And aud.audit_log.action = MTM.STALE_PRICE_ALERT, after_jsonb berisi "reason": "kurs_not_available"
    And notifikasi ke ROLE-AKUN: "MTM OBL-0042 tidak dapat diproses: kurs USD 2026-06-18 belum tersedia. Upload kurs manual sebelum MTM dapat diposting."
    And notifikasi ke ROLE-AKUN-CTL: "MTM OBL-0042 ditangguhkan karena kurs USD tidak tersedia. Pantau di /trx/mtm?filter[status]=STALE_PRICE"

  # ─── Config threshold dapat diubah ROLE-IT-ADMIN ─────────────────────────

  Scenario: S3-AC4 — ROLE-IT-ADMIN mengubah PRICE_DEVIATION_THRESHOLD_PCT dari 5 ke 3
    Given USR-IT-ADMIN-001 ter-autentikasi dengan permission sys_config.update
    When USR-IT-ADMIN-001 mengirim PATCH /api/v1/sys/config/PRICE_DEVIATION_THRESHOLD_PCT
      With body: { "value": "3", "reason": "Kebijakan baru: threshold lebih ketat per ALCO memo Juni 2026" }
      With Idempotency-Key: IK-CFG-001
    Then HTTP 200
    And sys.config PRICE_DEVIATION_THRESHOLD_PCT = 3
    And aud.audit_log.action = SYS_CONFIG.UPDATED — in-transaction
    And MTM run berikutnya menggunakan threshold baru (3%)
    And notifikasi ke ROLE-AKUN-CTL: "Threshold deviasi MTM diubah dari 5% menjadi 3% oleh IT Admin."
```

---

## Story P5-M6-S4 — Override per Row: Approve / Reject (ROLE-AKUN-CTL, SoD)

**Actor**: ROLE-AKUN-CTL (Override Approver) — berbeda dari ROLE-AKUN uploader (SoD)
**Trigger**: ROLE-AKUN-CTL melihat antrian PENDING_REVIEW atau STALE_PRICE di `/trx/mtm`, memutuskan approve (posting jurnal) atau reject (tidak posting, notif ke uploader).
**Goal**: ROLE-AKUN-CTL meng-override per row dengan komentar wajib (≥ 30 char). SoD server-side: `override_approver_id ≠ uploader_id`. Saat approve → call P5-M2 jurnal engine. Saat reject → notif ke uploader. Idempotency-Key wajib. Audit in-transaction.

### Endpoints

```
POST /api/v1/trx/mtm/{id}/override-approve
Authorization: Bearer <jwt>
Idempotency-Key: <uuid-v4>

Body:
{
  "comment": "Harga terverifikasi via Bloomberg terminal. Delta 7.3% wajar karena rilis FOMC. Disetujui.",
  "signature_method": "JWT_STEP_UP"
}

→ 200 OK
{
  "data": {
    "mtm_id": "<uuid>",
    "instrumen_kode": "OBL-0042",
    "status": "APPROVED",
    "jurnal_entry_id": "<uuid>",
    "jurnal_event_code": "MTM_FVOCI",
    "approved_by": "USR-AKUN-CTL-001",
    "approved_at": "2026-06-18T14:30:00+07:00"
  }
}

POST /api/v1/trx/mtm/{id}/override-reject
Body:
{
  "comment": "Harga 90.00 tidak sesuai IBPA. Harap re-upload harga yang benar (min referensi sumber).",
  "signature_method": "JWT_STEP_UP"
}

→ 200 OK { "data": { "status": "REJECTED", "rejected_by": "...", "comment": "..." } }
```

### Acceptance Criteria

```gherkin
Feature: Override approve/reject MTM per row oleh ROLE-AKUN-CTL (SoD)

  Background:
    Given trx.mtm OBL-0042 2026-06-18: status = 'PENDING_REVIEW', uploader_id = USR-AKUN-001
    And ROLE-AKUN-CTL (USR-AKUN-CTL-001) ter-autentikasi dengan permission mtm.override

  # ─── HAPPY PATH: Override approve → jurnal diposting ────────────────────

  Scenario: S4-AC1 — ROLE-AKUN-CTL override-approve MTM OBL-0042 — jurnal diposting via P5-M2
    When USR-AKUN-CTL-001 mengirim POST /api/v1/trx/mtm/<uuid-OBL-0042>/override-approve
      With Idempotency-Key: IK-MTM-OVR-001
      With body: { "comment": "Harga terverifikasi via Bloomberg. Delta wajar karena FOMC. Disetujui.", "signature_method": "JWT_STEP_UP" }
    Then HTTP 200
    And dalam satu transaksi DB:
      | trx.mtm.status                | APPROVED                   |
      | trx.mtm.override_approver_id  | USR-AKUN-CTL-001 UUID      |
      | trx.mtm.override_comment      | "Harga terverifikasi..."   |
      | trx.mtm.override_at           | timestamp now              |
      | trx.mtm.jurnal_entry_id       | <non-null, dari P5-M2>     |
      | trx.mtm.jurnal_event_code     | 'MTM_FVOCI'                |
      | aud.audit_log.action          | MTM.OVERRIDE_APPROVED      |
    And toast ke USR-AKUN-CTL-001: "Override MTM OBL-0042 2026-06-18 disetujui. Jurnal MTM_FVOCI berhasil diposting."
    And notifikasi ke USR-AKUN-001: "MTM OBL-0042 2026-06-18 Anda disetujui oleh Finance Controller. Jurnal telah diposting."

  # ─── HAPPY PATH: Override reject dengan komentar ─────────────────────────

  Scenario: S4-AC2 — ROLE-AKUN-CTL override-reject — jurnal tidak diposting, notif ke uploader
    When USR-AKUN-CTL-001 mengirim POST /api/v1/trx/mtm/<uuid-OBL-0042>/override-reject
      With Idempotency-Key: IK-MTM-OVR-002
      With body: { "comment": "Harga 90.00 tidak sesuai IBPA. Re-upload dengan referensi Bloomberg atau IBPA.", "signature_method": "JWT_STEP_UP" }
    Then HTTP 200
    And dalam satu transaksi DB:
      | trx.mtm.status           | REJECTED               |
      | trx.mtm.override_comment | "Harga 90.00 tidak..." |
      | aud.audit_log.action     | MTM.OVERRIDE_REJECTED  |
    And jurnal TIDAK diposting ke P5-M2
    And notifikasi ke USR-AKUN-001: "MTM OBL-0042 2026-06-18 DITOLAK. Alasan: 'Harga 90.00 tidak sesuai IBPA...' Harap re-upload harga yang benar."

  # ─── ERROR: SoD — Uploader mencoba override-approve sendiri ─────────────

  Scenario: S4-AC3 — USR-AKUN-001 (uploader) mencoba override-approve MTM yang dia upload — SoD violation
    Given trx.mtm OBL-0042: uploader_id = USR-AKUN-001
    And USR-AKUN-001 memiliki permission mtm.override (dual role)
    When USR-AKUN-001 mengirim POST /api/v1/trx/mtm/<uuid-OBL-0042>/override-approve
      With Idempotency-Key: IK-MTM-OVR-SOD
    Then HTTP 403:
      | error.code    | MTM_OVERRIDE_SOD_VIOLATION                                      |
      | error.message | "Anda tidak dapat meng-approve MTM yang Anda upload sendiri. SoD: override_approver_id ≠ uploader_id (DEC-017)." |
    And tidak ada perubahan state
    And aud.audit_log.action = MTM.OVERRIDE_REJECTED_SOD (advisory)

  # ─── ERROR: Override-reject tanpa komentar ≥ 30 char ─────────────────────

  Scenario: S4-AC4 — Override-reject dengan komentar terlalu pendek
    When USR-AKUN-CTL-001 mengirim POST /api/v1/trx/mtm/<uuid>/override-reject
      With body: { "comment": "Salah harga.", "signature_method": "JWT_STEP_UP" }
      With Idempotency-Key: IK-MTM-OVR-003
    Then HTTP 400:
      | error.code              | VALIDATION_FAILED                                              |
      | error.details[0].field  | comment                                                        |
      | error.details[0].rule   | "comment wajib minimal 30 karakter untuk override-reject MTM." |
    And tidak ada perubahan state
```

---

## Story P5-M6-S5 — Jurnal Routing per Klasifikasi PSAK 71 (Compliance-Critical)

**Actor**: System (MTM worker atau override-approve handler) + `ifrs9-compliance-reviewer` (BLOCKING gate)
**Trigger**: Setiap kali `trx.mtm` berpindah ke status `AUTO_POSTED` atau `APPROVED`, system memanggil P5-M2 jurnal engine dengan `event_code` yang ditentukan oleh routing matrix di bawah.
**Goal**: Routing jurnal MTM yang benar per klasifikasi PSAK 71 — salah routing = misstatement P&L atau OCI (audit finding regulatory). Fungsi `resolveJurnalEventCode(instrumen) string` wajib di-review ifrs9-compliance-reviewer sebelum implementasi.

### Routing Resolution Logic

```go
// internal/trx/mtm/routing.go
func resolveJurnalEventCode(inst Instrumen, kursAvailable bool) ([]string, error) {
    if inst.KlasifikasiPSAK71 == "AC" {
        return nil, ErrMTMInstrumenACSkip  // AC: tidak ada MTM jurnal
    }
    var codes []string
    switch inst.KlasifikasiPSAK71 {
    case "FVOCI_DEBT":
        codes = append(codes, "MTM_FVOCI")
        if inst.MataUang != "IDR" && kursAvailable {
            codes = append(codes, "MTM_FX_OCI_RESERVE")  // §B5.7.2A — FX terpisah ke OCI
        }
    case "FVOCI_ELECTION":
        codes = append(codes, "MTM_FVOCI_ELECTION")
        // Tidak ada FX recycling ke P&L on disposal — irrevocable (PSAK 71 §5.7.5)
    case "FVTPL":
        codes = append(codes, "MTM_FVTPL")
    case "POCI":
        codes = append(codes, "MTM_FVTPL_POCI")
        // POCI: credit-adjusted EIR; MTM ke P&L (Stage tidak berlaku, tidak ada Stage escalation)
    default:
        return nil, ErrUnknownKlasifikasi
    }
    return codes, nil
}
```

### Compliance Checklist (untuk ifrs9-compliance-reviewer)

- [ ] `FVOCI_DEBT IDR` → `MTM_FVOCI` saja (OCI). Tidak ada FX entry karena IDR.
- [ ] `FVOCI_DEBT FCY` → `MTM_FVOCI` + `MTM_FX_OCI_RESERVE` (dua entry terpisah per §B5.7.2A): MTM delta ke OCI, FX gain/loss ke OCI Reserve (bukan P&L).
- [ ] `FVOCI_ELECTION` → `MTM_FVOCI_ELECTION` (OCI, tidak pernah ke P&L — irrevocable per §5.7.5). Disposal gain/loss tetap di OCI.
- [ ] `FVTPL` → `MTM_FVTPL` (P&L — fair value change langsung ke laporan laba rugi §5.7.7).
- [ ] `AC` → **SKIP** — tidak ada MTM. Amortised cost method via EIR; perubahan nilai tidak diakui.
- [ ] `POCI` → `MTM_FVTPL_POCI` (P&L). POCI tidak memiliki Stage; ECL movement separate dari MTM.
- [ ] Semua event codes (`MTM_FVOCI`, `MTM_FX_OCI_RESERVE`, `MTM_FVOCI_ELECTION`, `MTM_FVTPL`, `MTM_FVTPL_POCI`) harus tersedia di mapping jurnal master P5-M2 sebelum MTM cron berjalan.

### Acceptance Criteria

```gherkin
Feature: Jurnal routing MTM per klasifikasi PSAK 71 — compliance-critical

  Background:
    Given P5-M2 jurnal engine berjalan dan event codes MTM sudah di-seed di mapping jurnal master
    And mst.instrumen tersedia dengan berbagai klasifikasi

  # ─── FVOCI_DEBT FCY → dua jurnal: MTM_FVOCI + MTM_FX_OCI_RESERVE ────────

  Scenario: S5-AC1 — OBL-0042 FVOCI_DEBT USD — routing ke MTM_FVOCI + MTM_FX_OCI_RESERVE (§B5.7.2A)
    Given mst.instrumen OBL-0042: klasifikasi_psak71 = 'FVOCI_DEBT', mata_uang = 'USD'
    And trx.mtm OBL-0042: status = 'AUTO_POSTED', delta_idr = 12.500.000, kurs_tersedia = TRUE
    When system resolves jurnal event code untuk OBL-0042
    Then resolveJurnalEventCode mengembalikan ["MTM_FVOCI", "MTM_FX_OCI_RESERVE"]
    And P5-M2 menerima dua POST /api/v1/jurnal/post-entry:
      | event_code       | MTM_FVOCI — delta ke OCI (Perubahan Nilai Wajar)    |
      | event_code       | MTM_FX_OCI_RESERVE — FX component ke OCI FX Reserve |
    And kedua jurnal_entry_id tersimpan di trx.mtm.jurnal_entry_id (array atau dua kolom)
    And aud.audit_log.action = MTM.AUTO_POSTED, after_jsonb berisi "event_codes": ["MTM_FVOCI","MTM_FX_OCI_RESERVE"]

  # ─── FVOCI_ELECTION → MTM_FVOCI_ELECTION, no P&L routing ────────────────

  Scenario: S5-AC2 — SHM-0015 FVOCI_ELECTION — routing ke MTM_FVOCI_ELECTION, tidak ada P&L entry
    Given mst.instrumen SHM-0015: klasifikasi_psak71 = 'FVOCI_ELECTION', mata_uang = 'IDR'
    And trx.mtm SHM-0015: status = 'AUTO_POSTED'
    When system resolves jurnal event code untuk SHM-0015
    Then resolveJurnalEventCode mengembalikan ["MTM_FVOCI_ELECTION"]
    And P5-M2 menerima SATU POST /api/v1/jurnal/post-entry dengan event_code = 'MTM_FVOCI_ELECTION'
    And tidak ada entry ke akun P&L (Laba Rugi) — hanya OCI Ekuitas
    And jika instrumen ini di-dispose di masa depan: tidak ada recycle ke P&L (irrevocable §5.7.5)

  # ─── AC instrumen → SKIP, ErrMTMInstrumenACSkip ─────────────────────────

  Scenario: S5-AC3 — DEP-0001 AC instrumen — resolveJurnalEventCode mengembalikan error AC skip
    Given mst.instrumen DEP-0001: klasifikasi_psak71 = 'AC'
    When system resolves jurnal event code untuk DEP-0001
    Then resolveJurnalEventCode mengembalikan error MTM_INSTRUMEN_AC_SKIP
    And tidak ada call ke P5-M2 jurnal engine
    And tidak ada trx.mtm row untuk DEP-0001
    And log advisory: "DEP-0001 klasifikasi AC — MTM di-skip per PSAK 71 amortised cost method."

  # ─── POCI → MTM_FVTPL_POCI, no stage escalation ─────────────────────────

  Scenario: S5-AC4 — OBL-POCI-001 POCI — routing ke MTM_FVTPL_POCI, tidak ada Stage logic
    Given mst.instrumen OBL-POCI-001: klasifikasi_psak71 = 'POCI', mata_uang = 'USD'
    And trx.mtm OBL-POCI-001: status = 'AUTO_POSTED'
    When system resolves jurnal event code untuk OBL-POCI-001
    Then resolveJurnalEventCode mengembalikan ["MTM_FVTPL_POCI"]
    And P5-M2 menerima POST dengan event_code = 'MTM_FVTPL_POCI'
    And aud.audit_log.after_jsonb berisi "poci": true
    And ECL calc untuk OBL-POCI-001 tetap berjalan independent dari MTM (tidak double-count)
    And tidak ada Stage escalation logic dipicu oleh MTM row (Stage logic hanya di APP-C)
```

---

## Ringkasan P5-M6 Story Set

| Story | Judul | Actor Utama | AC Count | Gate |
|---|---|---|---|---|
| P5-M6-S1 | Daily MTM cron 18:00 WIB — Asynq, feed IBPA/BEI/KSEI, AUTO_POSTED vs PENDING_REVIEW vs STALE_PRICE | System (Asynq worker) | 4 | advisory (integration-engineer + backend-engineer implementasi) |
| P5-M6-S2 | Manual upload MTM (Maker, PENDING_REVIEW) | ROLE-AKUN | 4 | advisory |
| P5-M6-S3 | Price-deviation WARNING + STALE_PRICE alert | System + ROLE-AKUN-CTL | 4 | advisory + security (audit trail) |
| P5-M6-S4 | Override per row: approve/reject (SoD) | ROLE-AKUN-CTL | 4 | **security-engineer BLOCKING** (SoD enforcement + audit) |
| P5-M6-S5 | Jurnal routing per klasifikasi PSAK 71 | System internal | 4 | **ifrs9-compliance-reviewer BLOCKING** |
| **Total** | | | **20** | |

---

## Error Codes Proposed (Baru — untuk system-analyst)

Kode baru yang dibutuhkan P5-M6 dan belum ada di `api-conventions.md`:

| Code | HTTP | Trigger | Catatan |
|---|---|---|---|
| `MTM_PRICE_STALE` | 422 | `trx.mtm.stale_price_flag = TRUE`; jurnal tidak dapat diposting otomatis | Berbeda dari `MTM_PERIODE_LOCKED` — spesifik untuk ketidaktersediaan harga terkini |
| `MTM_PRICE_DEVIATION_REJECTED` | 422 | ROLE-AKUN-CTL melakukan override-reject pada baris dengan `deviation_flag = TRUE` | Muncul di notifikasi uploader; bukan HTTP error dari reject endpoint (yang return 200) |
| `MTM_BATCH_NOT_FOUND` | 404 | `upload_batch_id` tidak ditemukan di `sys.upload_batch` saat query status batch | Berbeda dari `NOT_FOUND` generik — spesifik konteks MTM batch |
| `MTM_OVERRIDE_SOD_VIOLATION` | 403 | `override_approver_id = uploader_id` — SoD violation saat override-approve/reject | Lebih spesifik dari `SOD_VIOLATION` generik; `details` berisi `uploader_id` untuk tracing |
| `MTM_INSTRUMEN_AC_SKIP` | 422 | Instrumen berklasifikasi AC dimasukkan ke dalam manual upload MTM | Menjelaskan mengapa AC tidak bisa di-MTM per PSAK 71 |
| `MTM_PERIODE_LOCKED` | 423 | Mutasi `trx.mtm` untuk periode yang sudah hard-closed (`locked_flag = TRUE`) | Mirror `FX_RATE_LOCKED` dari P5-M5; spesifik untuk tabel `trx.mtm` |

Catatan: `PERIODE_CLOSED` (HTTP 423), `SOD_VIOLATION` (HTTP 403), `VALIDATION_FAILED` (HTTP 400), `IDEMPOTENCY_REPLAY` (HTTP 200), `NOT_FOUND` (HTTP 404) sudah ada di `api-conventions.md` — tidak perlu ditambahkan ulang.

---

## Persona Summary Table

| Actor | Permission | Aksi di P5-M6 | MFA Level |
|---|---|---|---|
| ROLE-AKUN | `mtm.create`, `mtm.read` | Upload harga MTM manual (Maker); view list MTM | Tidak wajib |
| ROLE-AKUN-CTL | `mtm.override`, `mtm.read`, `mtm.export` | Override approve/reject (SoD: ≠ uploader); terima alert; view antrian PENDING_REVIEW + STALE_PRICE | MFA wajib (DEC-026) |
| ROLE-RISK | `mtm.read`, `instrumen.read` | View MTM list dan routing decision; menerima eskalasi STALE > 5 hari | Tidak wajib |
| ROLE-IT-ADMIN | `mtm.trigger`, `mtm.read` | Trigger manual MTM run jika cron gagal; view DLQ entries; update sys.config threshold | MFA wajib (DEC-026) |
| ROLE-CFO | Via P5-M4 (`periode.hardclose.approve`) | Secara indirect men-lock semua `trx.mtm` rows dalam periode saat hard-close | MFA + step-up (DEC-026 + DEC-027) |
| ROLE-AUDIT | `mtm.read`, `mtm.export` | Read-only seluruh MTM data; export untuk auditor eksternal | Tidak wajib |
| System (Asynq worker) | Service account (no JWT) | MTM daily cron fetch + compute + insert + auto-post via P5-M2 | N/A |

---

## Dependensi Lintas Modul

| Dependensi | Arah | Keterangan |
|---|---|---|
| `mst.instrumen` approved + `klasifikasi_psak71` locked | P5-M1 → P5-M6 | MTM hanya untuk instrumen ACTIVE dengan klasifikasi final |
| Jurnal engine + event code mapping seed | P5-M2 → P5-M6 | `MTM_FVOCI`, `MTM_FX_OCI_RESERVE`, `MTM_FVOCI_ELECTION`, `MTM_FVTPL`, `MTM_FVTPL_POCI` harus ada di mapping jurnal master sebelum cron berjalan |
| Feed IBPA/BEI/KSEI sudah di-parse ke staging | P5-M3 → P5-M6 | MTM worker membaca dari `sys.ibpa_feed_staging`, `sys.bei_feed_staging`, `sys.ksei_feed_staging` |
| `mst.kurs` APPROVED tersedia | P5-M5 → P5-M6 | FCY instrumen memerlukan kurs hari ini untuk konversi EAD_IDR; jika tidak ada → STALE_PRICE |
| `GET /master/kurs/treatment/{instrumen_id}` | P5-M5 → P5-M6 | MTM worker memanggil endpoint P5-M5 S5 untuk FX routing decision setiap FCY instrumen |
| `sys.holiday_calendar` | P5-M5 (migration 000039) → P5-M6 | Cron skip pada hari libur nasional |
| `locked_flag = TRUE` pada `trx.mtm` setelah hard-close | P5-M4 → P5-M6 | Hard-close lock berlaku juga untuk tabel `trx.mtm` |
| Migration 000040 (`trx.mtm`) | P5-M6 → data-modeler | Tabel baru; partisi bulanan via pg_partman |
| Migration 000041 (`sys.upload_batch` MTM rows) | P5-M6 → data-modeler | Extend `sys.upload_batch.batch_type` CHECK constraint agar mencakup `'MTM'` |
| DLQ pattern | P5-M3 → P5-M6 | MTM cron failure mengikuti pattern `sys.dead_letter_queue` dari P5-M3 |

---

## Open Questions — P5-M6

| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-M6-1 | Untuk instrumen POCI: apakah ECL movement dari APP-C diperlukan sebelum MTM diposting, atau keduanya independent? | **Independent** — ECL APP-C berjalan terpisah; MTM_FVTPL_POCI hanya fair value movement. `ecl-eir-engineer` konfirmasi tidak ada double-count. |
| OQ-M6-2 | MTM untuk instrumen FVOCI_DEBT FCY: apakah `MTM_FX_OCI_RESERVE` diposting sebagai satu jurnal (gabung delta MTM + FX) atau dua jurnal terpisah per §B5.7.2A? | **Dua jurnal terpisah** — `MTM_FVOCI` untuk delta harga pasar (ex-FX) dan `MTM_FX_OCI_RESERVE` untuk FX component. `ifrs9-compliance-reviewer` konfirmasi dekomposisi. |
| OQ-M6-3 | Jika MTM cron berjalan parsial (misalnya 80% instrumen berhasil, 20% gagal) — apakah run dianggap sukses? DLQ dibuat atau tidak? | **Parsial sukses diterima** — instrumen yang berhasil diproses tetap di-auto-post. Instrumen yang gagal di-log per-instrumen. Jika error_count > 0 → DLQ + alert (bukan DLQ per instrumen). `backend-engineer-go` implementasi. |
| OQ-M6-4 | Override-approve untuk baris STALE_PRICE — apakah ROLE-AKUN-CTL dapat approve dengan harga lama (stale)? | **Tidak** — override-approve STALE_PRICE harus didahului upload harga baru via S2, sehingga row menjadi PENDING_REVIEW dengan harga fresh. Override-approve STALE_PRICE langsung tidak diizinkan. Perlu konfirmasi `ifrs9-compliance-reviewer`. |
| OQ-M6-5 | Apakah threshold `STALE_PRICE_THRESHOLD_DAYS` dan `PRICE_DEVIATION_THRESHOLD_PCT` per tipe instrumen (mis. Reksadana NAB lebih toleran stale) atau global? | **Global per `sys.config`** — sama seperti P5-M5 pattern. Override per tipe instrumen bisa Phase 6 jika ALCO minta. |
| OQ-M6-6 | Jumlah unit instrumen (`jumlah_unit`) disimpan di mana — `mst.instrumen` atau `trx.penempatan`? MTM perlu ini untuk hitung `delta_idr`. | **`trx.penempatan`** — MTM worker join ke `trx.penempatan` untuk mengambil `saldo_unit` terkini. Data-modeler konfirmasi kolom yang tepat. |

---

## Schema Change Summary (untuk data-modeler)

### Migration 000040: CREATE TABLE `trx.mtm`

Tabel MTM dengan partisi bulanan (range `created_at`), audit columns lengkap, indexes seperti didefinisikan di §Schema P5-M6. Lihat tabel kolom di atas.

```sql
-- Constraints wajib
ALTER TABLE trx.mtm
    ADD CONSTRAINT chk_mtm_override_comment
        CHECK (status != 'REJECTED' OR (override_comment IS NOT NULL AND length(override_comment) >= 30));

ALTER TABLE trx.mtm
    ADD CONSTRAINT chk_mtm_sod
        CHECK (override_approver_id IS NULL OR override_approver_id != uploader_id);

ALTER TABLE trx.mtm
    ADD CONSTRAINT chk_mtm_status
        CHECK (status IN ('AUTO_POSTED','PENDING_REVIEW','APPROVED','REJECTED','STALE_PRICE'));
```

### Migration 000041: `sys.upload_batch` MTM batch_type

```sql
-- Extend CHECK constraint untuk batch_type agar mencakup 'MTM'
ALTER TABLE sys.upload_batch
    DROP CONSTRAINT IF EXISTS chk_upload_batch_type;
ALTER TABLE sys.upload_batch
    ADD CONSTRAINT chk_upload_batch_type
        CHECK (batch_type IN ('KURS','PEFINDO','IBPA','KSEI','BEI','MTM'));
```

---

## Compliance & Security Handoff Checklist

### Untuk ifrs9-compliance-reviewer (BLOCKING gate — S5)
- [ ] Routing matrix S5 (FVOCI_DEBT IDR → MTM_FVOCI only; FVOCI_DEBT FCY → MTM_FVOCI + MTM_FX_OCI_RESERVE; FVOCI_ELECTION → MTM_FVOCI_ELECTION; FVTPL → MTM_FVTPL; POCI → MTM_FVTPL_POCI; AC → SKIP) sesuai PSAK 71 §5.7.5, §5.7.7, §5.7.10, §B5.7.2A
- [ ] `MTM_FX_OCI_RESERVE` untuk FVOCI_DEBT FCY: konfirmasi dekomposisi dua jurnal terpisah per §B5.7.2A (bukan satu jurnal gabung)
- [ ] POCI: MTM ke P&L (`MTM_FVTPL_POCI`) — konfirmasi tidak ada Stage escalation logic yang ter-trigger dari MTM row
- [ ] AC skip: konfirmasi bahwa tidak ada edge case di mana instrumen AC bisa punya fair value movement yang harus diakui (mis. impairment Stage 3 via ECL saja, tidak via MTM)
- [ ] FVOCI_ELECTION disposal: pastikan routing MTM tidak membuka channel P&L recycling yang melanggar irrevocable election (§5.7.5)
- [ ] Semua 5 event codes harus ada di P5-M2 mapping jurnal master seed sebelum cron berjalan — cross-check dengan P5-M2 story

### Untuk security-engineer (BLOCKING — S4 SoD)
- [ ] SoD enforcement `override_approver_id ≠ uploader_id` di service layer (tidak hanya DB constraint) — test direct API bypass
- [ ] `MTM.OVERRIDE_APPROVED` dan `MTM.OVERRIDE_REJECTED` ditulis in-transaction dengan perubahan status
- [ ] DB constraint `chk_mtm_sod` (`override_approver_id ≠ uploader_id`) sebagai safety net kedua
- [ ] `MTM.UPLOADED`, `MTM.AUTO_POSTED`, `MTM.STALE_PRICE_ALERT` semua in-transaction — bukan async
- [ ] Idempotency-Key cek di override-approve dan override-reject endpoints — mencegah double-approve
- [ ] Export `GET /trx/mtm/export` — audit `MTM.EXPORT` in-transaction; ROLE-AUDIT read-only enforcement
- [ ] `ROLE-IT-ADMIN` manual trigger endpoint rate-limit: 10 req/jam
- [ ] `trx.mtm.locked_flag = TRUE` setelah periode hard-close — API middleware sama dengan `FxRateLockMiddleware` P5-M5

### Untuk ecl-eir-engineer
- [ ] Konfirmasi bahwa POCI MTM (`MTM_FVTPL_POCI`) tidak double-count ECL — ECL movement APP-C dan MTM fair value APP-D adalah dua akrual terpisah
- [ ] Untuk Stage 3 instrumen FVTPL: bunga dihitung dari Net Carrying (Gross − ECL). Pastikan MTM worker menggunakan `harga_buku_idr` yang sudah merefleksikan ECL (Net Carrying), bukan Gross Carrying

### Untuk backend-engineer-go + integration-engineer
- [ ] Cron schedule `"00 11 * * 1-5"` (18:00 WIB = 11:00 UTC) — verify timezone Docker/K8s `TZ=Asia/Jakarta`
- [ ] Partial run logic: loop per instrumen dengan transaksi terpisah, bukan satu transaksi raksasa — satu gagal tidak rollback semua
- [ ] Holiday check: cache `sys.holiday_calendar` di Redis TTL 24 jam (sama dengan P5-M5 pattern)
- [ ] DLQ insert pattern: mirror P5-M3 `sys.dead_letter_queue`; payload `{job_type: "mtm_daily_run", tanggal, instrumen_errors: [{instrumen_id, error}], error_count}`
- [ ] Progress reporting via `sys.job` + Redis pub/sub untuk `<JobProgressPanel>` (rule §3 UX)

---

_Story set ini siap dihandoff ke `system-analyst` untuk OpenAPI contract + state machine `trx.mtm.status`, ke `ifrs9-compliance-reviewer` untuk review S5 (routing matrix BLOCKING), dan ke `security-engineer` untuk review S4 (SoD + audit trail BLOCKING). `data-modeler` memulai migration 000040 + 000041 paralel. Backend implementasi menunggu compliance gate S5 selesai sebelum coding routing logic._
