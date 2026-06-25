# P5-M7 — APP-B Renewal Deposito: User Stories

**Story Set ID**: P5-M7
**Modul**: APP-B — Transaction Lifecycle (Renewal Deposito, Phase 5)
**Status**: DRAFT — menunggu handoff ke `system-analyst` + `ifrs9-compliance-reviewer`
**Author**: business-analyst
**Tanggal**: 2026-06-19
**Linked FSD**: FSD-APP-B-TransactionLifecycle-v1.0.docx §4 (Renewal), §5 (EIR Re-estimation on Amendment)
**Linked BRD**: BRD §6.2 (APP-B Renewal), RACI: ROLE-APPR-TR (A), ROLE-MAKER-TR (R), ROLE-AKUN (C), ROLE-RISK (C), ROLE-AUDIT (I)
**Linked Decision Log**:
- `DEC-013` (LOCKED) — EIR Newton-Raphson, tolerance 1e-10, max 100 iter, presisi 8 desimal
- `DEC-016` (LOCKED) — `shopspring/decimal`; `NUMERIC(20,4)` IDR, `NUMERIC(10,8)` PD/LGD/EIR; **never float64**
- `DEC-017` (LOCKED) — 4-eyes SoD: `maker_id ≠ reviewer_id ≠ approver_id`; enforced server-side
- `DEC-018` (LOCKED) — audit trail append-only, retensi 10+10 tahun
- `DEC-021` (LOCKED) — Idempotency-Key wajib di setiap mutating endpoint

**Dependensi**:
- **P5-M1** (`mst.instrumen` ACTIVE, klasifikasi PSAK 71 locked, SPPI+BM final) — renewal hanya untuk instrumen dengan `status = 'ACTIVE'` dan `jenis_instrumen = 'DEPOSITO'`
- **P5-M2** (jurnal engine) — event code `RENEWAL_DEPOSITO` diposting via `POST /api/v1/jurnal/post-entry`; multi-leg entry (pelunasan pokok lama + penempatan pokok baru + akrual bunga bersih)

**Handoff berikutnya**:
- `system-analyst` → OpenAPI fragment: 4 endpoints (`POST /trx/renewal`, `GET /trx/renewal/{id}`, `POST /trx/renewal/{id}/approve`, `POST /trx/renewal/{id}/reject`); state machine `trx.renewal.status`; error codes baru (§Error Codes Proposed)
- `data-modeler` → migration 000042 (`trx.renewal` tabel), migration 000043 (`ecl.amortisasi_schedule` tambah `schedule_version` untuk renewal EIR)
- `ifrs9-compliance-reviewer` → **BLOCKING gate** untuk: (a) EIR re-computation Newton-Raphson pada `effective_from/to` versioning; (b) PPh 20% deposito bank (sesuai PP No. 131/2000); (c) inherit SPPI+BM dari instrumen lama; (d) klasifikasi AC tetap tidak berubah karena renewal bukan reklasifikasi
- `security-engineer` → review SoD enforcement (S2), audit trail in-transaction (S1, S2, S3, S4), idempotency

**Compliance path**: P5-M7 adalah **regulated path** — menyentuh EIR re-estimation (DEC-013) dan PPh deposito (regulasi perpajakan). **ifrs9-compliance-reviewer BLOCKING** wajib sebelum implementasi backend S3 (EIR) dan S5 (jurnal). Security review BLOCKING untuk S2 (SoD + audit).

---

## Konteks & Arsitektur P5-M7

### Alur Renewal Deposito

```
ROLE-MAKER-TR
  │  Pilih mst.instrumen (jenis='DEPOSITO', status='ACTIVE', approaching maturity)
  │  Pilih skema: POKOK_SAJA atau POKOK_PLUS_BUNGA
  │  Input: tenor_baru (bulan), rate_baru (% p.a.), tanggal_efektif_baru
  │  Preview kalkulasi:
  │    pokok_baru = pokok_lama (POKOK_SAJA) ATAU pokok_lama + bunga_bersih (POKOK_PLUS_BUNGA)
  │    bunga_kotor = pokok_lama × rate_lama × (hari_berjalan / 365)
  │    PPh_20pct   = bunga_kotor × 0.20
  │    bunga_bersih = bunga_kotor − PPh_20pct
  │    EIR_baru = Newton-Raphson(cashflow baru)
  │  Submit → status = 'PENDING_APPROVAL'
  │
ROLE-APPR-TR (SoD: ≠ maker)
  │  Review preview kalkulasi
  │  Validate: tenor 1–60 bulan, rate 0–30%, bunga_bersih ≥ IDR 100.000 (jika POKOK_PLUS_BUNGA)
  │  Approve → status = 'APPROVED' → 'POSTED'
  │  Reject → status = 'REJECTED', comment wajib ≥ 30 char
  │
System (on APPROVED)
  │  INSERT mst.instrumen baru (klasifikasi inherit, SPPI+BM copy, status='ACTIVE')
  │  UPDATE mst.instrumen lama: status = 'MATURED'
  │  INSERT ecl.amortisasi_schedule schedule_version = old_version + 1 (EIR baru, effective_from = tanggal_efektif_baru)
  │  UPDATE schedule lama: effective_to = tanggal_efektif_baru
  │  POST /api/v1/jurnal/post-entry (event_code = 'RENEWAL_DEPOSITO', multi-leg)
  │  Audit: RENEWAL.APPROVED + INSTRUMEN.CREATED + INSTRUMEN.MATURED + EIR.RECOMPUTED — in-transaction
```

### State Machine `trx.renewal.status`

```
DRAFT (preview tersimpan, belum submit)
  │
  [submit] ROLE-MAKER-TR
  ↓
PENDING_APPROVAL
  │
  ├─ [approve] ROLE-APPR-TR (SoD: ≠ maker_id)
  │    → status = 'APPROVED'
  │    → trigger: insert instrumen baru, matured instrumen lama, EIR recompute, jurnal post
  │    → status = 'POSTED' (setelah semua side-effect selesai dalam satu transaksi)
  │
  ├─ [reject] ROLE-APPR-TR (comment ≥ 30 char wajib)
  │    → status = 'REJECTED'
  │    → notifikasi ke maker dengan alasan
  │
  [reopen] ROLE-MAKER-TR (dari REJECTED → DRAFT untuk amend dan resubmit)
  ↓
POSTED (immutable setelah ini; kecuali periode hard-close lock)
```

### Kalkulasi Preview

```
# Bunga akrual dari instrumen lama (hari berjalan sampai tanggal_efektif_baru)
bunga_kotor  = pokok_lama × (rate_lama / 100) × (hari_berjalan / 365)
PPh_20pct    = bunga_kotor × Decimal("0.20")
bunga_bersih = bunga_kotor − PPh_20pct

# Pokok instrumen baru
IF skema = 'POKOK_SAJA':
    pokok_baru = pokok_lama
ELSE skema = 'POKOK_PLUS_BUNGA':
    pokok_baru = pokok_lama + bunga_bersih
    constraint: bunga_bersih >= IDR 100.000 (RENEWAL_BUNGA_BERSIH_TOO_SMALL jika tidak terpenuhi)

# EIR baru (Newton-Raphson)
cashflows = [-pokok_baru] + [kupon_per_periode × (1 − 0.20)] + [pokok_baru pada jatuh tempo]
EIR_baru  = newton_raphson(cashflows, tolerance=1e-10, max_iter=100)
```

### Jurnal Multi-Leg `RENEWAL_DEPOSITO`

| Leg | Akun Debit | Akun Kredit | Nilai | Keterangan |
|---|---|---|---|---|
| 1 | `Kewajiban PPh Deposito` | `Kas/Rekening Bank` | PPh_20pct | Setoran PPh final |
| 2 | `Deposito (lama)` | `Kas/Rekening Bank` | pokok_lama | Pelunasan pokok lama |
| 3 | `Kas/Rekening Bank` | `Deposito (baru)` | pokok_baru | Penempatan pokok baru |
| 4 | `Beban Bunga Deposito` | `Kas/Rekening Bank` | bunga_bersih | Akrual bunga bersih diterima |

---

## Story P5-M7-S1 — Create Renewal Request (ROLE-MAKER-TR)

**Actor**: ROLE-MAKER-TR
**Trigger**: ROLE-MAKER-TR membuka `/trx/renewal/new`, memilih instrumen deposito yang mendekati jatuh tempo (`tanggal_jatuh_tempo ≤ today + 30 hari`), mengisi parameter renewal, dan menyimpan preview.
**Goal**: Maker menginput renewal request dengan skema dan parameter valid, preview kalkulasi (pokok_baru, bunga_kotor, PPh_20pct, bunga_bersih, EIR_baru) ditampilkan sebelum submit. Request masuk status `PENDING_APPROVAL`. Validasi server-side: tenor 1–60 bulan, rate 0–30%, instrumen eligible.

### Pre-conditions
1. User ter-autentikasi dengan permission `transaksi.create`
2. Request mengandung `Idempotency-Key` header (UUID v4)
3. `mst.instrumen` target: `jenis_instrumen = 'DEPOSITO'`, `status = 'ACTIVE'`, `klasifikasi_locked = TRUE`
4. `mst.periode_buku.status_periode = 'OPEN'` untuk `tanggal_efektif_baru`
5. Instrumen lama belum memiliki `trx.renewal` aktif (status bukan `REJECTED` atau `MATURED`)

### Endpoint

```
POST /api/v1/trx/renewal
Authorization: Bearer <jwt>
Idempotency-Key: <uuid-v4>

Body:
{
  "instrumen_id": "<uuid-DEP-0042>",
  "skema": "POKOK_PLUS_BUNGA",
  "tenor_baru_bulan": 12,
  "rate_baru_persen": 5.75,
  "tanggal_efektif_baru": "2026-07-01"
}

→ 201 Created
{
  "data": {
    "renewal_id": "<uuid>",
    "status": "PENDING_APPROVAL",
    "preview": {
      "pokok_lama": 1000000000.0000,
      "bunga_kotor": 14246575.3425,
      "PPh_20pct": 2849315.0685,
      "bunga_bersih": 11397260.2740,
      "pokok_baru": 1011397260.2740,
      "EIR_baru": 0.04600000,
      "tanggal_jatuh_tempo_baru": "2027-07-01"
    },
    "next_step": "Menunggu approval ROLE-APPR-TR. SoD: approver tidak boleh sama dengan maker."
  }
}
```

### Audit Events

| Action | Trigger |
|---|---|
| `RENEWAL.CREATED` | Insert `trx.renewal` — in-transaction. `after_jsonb`: `{instrumen_id, skema, tenor_baru_bulan, rate_baru_persen, pokok_baru, EIR_baru, tanggal_efektif_baru}` |

### Acceptance Criteria

```gherkin
Feature: Create renewal deposito oleh ROLE-MAKER-TR

  Background:
    Given user ROLE-MAKER-TR (USR-MAKER-001) ter-autentikasi dengan permission transaksi.create
    And mst.instrumen DEP-0042:
      | jenis_instrumen    | DEPOSITO            |
      | status             | ACTIVE              |
      | klasifikasi_psak71 | AC                  |
      | klasifikasi_locked | TRUE                |
      | pokok_lama         | 1.000.000.000 IDR   |
      | rate_lama          | 5.50% p.a.          |
      | tanggal_jatuh_tempo | 2026-07-01         |
    And mst.periode_buku PRD-2026-07: status_periode = 'OPEN'

  Scenario: S1-AC1 — Maker berhasil create renewal POKOK_PLUS_BUNGA dengan preview lengkap
    Given tanggal_efektif_baru = 2026-07-01, tenor_baru_bulan = 12, rate_baru_persen = 5.75
    And hari_berjalan dari penempatan lama ke 2026-07-01 = 181 hari
    When USR-MAKER-001 mengirim POST /api/v1/trx/renewal
      With Idempotency-Key: IK-RNW-001
    Then HTTP 201
    And preview.bunga_kotor = 1.000.000.000 × (5.50/100) × (181/365) — presisi NUMERIC(20,4)
    And preview.PPh_20pct = bunga_kotor × 0.20 (Decimal, bukan float)
    And preview.bunga_bersih = bunga_kotor − PPh_20pct
    And preview.pokok_baru = pokok_lama + bunga_bersih (POKOK_PLUS_BUNGA)
    And preview.EIR_baru = hasil Newton-Raphson(cashflows_baru, tolerance=1e-10, max_iter=100) — 8 desimal
    And trx.renewal INSERT: status = 'PENDING_APPROVAL', maker_id = USR-MAKER-001
    And aud.audit_log.action = RENEWAL.CREATED — in-transaction
    And toast ke USR-MAKER-001: "Renewal DEP-0042 berhasil dibuat (RNW-{nomor}). Menunggu approval Treasury Approver."

  Scenario: S1-AC2 — Tenor di luar range 1–60 bulan — ditolak RENEWAL_TENOR_OUT_OF_RANGE
    Given tenor_baru_bulan = 72
    When USR-MAKER-001 mengirim POST /api/v1/trx/renewal
      With Idempotency-Key: IK-RNW-002
    Then HTTP 400:
      | error.code              | RENEWAL_TENOR_OUT_OF_RANGE                         |
      | error.details[0].field  | tenor_baru_bulan                                   |
      | error.details[0].rule   | "Tenor harus antara 1 dan 60 bulan. Nilai: 72."    |
    And tidak ada INSERT ke trx.renewal

  Scenario: S1-AC3 — Rate di luar range 0–30% — ditolak RENEWAL_RATE_OUT_OF_RANGE
    Given rate_baru_persen = 35.0
    When USR-MAKER-001 mengirim POST /api/v1/trx/renewal
      With Idempotency-Key: IK-RNW-003
    Then HTTP 400:
      | error.code              | RENEWAL_RATE_OUT_OF_RANGE                            |
      | error.details[0].field  | rate_baru_persen                                     |
      | error.details[0].rule   | "Rate harus antara 0% dan 30%. Nilai: 35.00%."       |
    And tidak ada INSERT ke trx.renewal

  Scenario: S1-AC4 — Instrumen bukan deposito atau tidak ACTIVE — ditolak RENEWAL_INSTRUMEN_NOT_ELIGIBLE
    Given mst.instrumen OBL-0099: jenis_instrumen = 'OBLIGASI', status = 'ACTIVE'
    When USR-MAKER-001 mengirim POST /api/v1/trx/renewal
      With body: instrumen_id = OBL-0099
      With Idempotency-Key: IK-RNW-004
    Then HTTP 422:
      | error.code    | RENEWAL_INSTRUMEN_NOT_ELIGIBLE                                                              |
      | error.message | "OBL-0099 bukan instrumen deposito atau tidak berstatus ACTIVE. Renewal hanya untuk deposito ACTIVE." |
    And tidak ada INSERT ke trx.renewal
```

---

## Story P5-M7-S2 — Approve Renewal (ROLE-APPR-TR, 4-Eyes SoD)

**Actor**: ROLE-APPR-TR
**Trigger**: ROLE-APPR-TR melihat antrian `PENDING_APPROVAL` di `/trx/renewal`, membuka detail renewal DEP-0042, memvalidasi preview, lalu approve atau reject.
**Goal**: 4-eyes SoD: `approver_id ≠ maker_id` enforced server-side. Saat approve → status `APPROVED` → `POSTED` (side-effects S3, S4, S5 dalam satu transaksi). Saat reject → status `REJECTED`, comment wajib ≥ 30 char, notif ke maker. Idempotency-Key wajib. PPh_20pct di-revalidasi server-side sebelum approve (bukan trust preview client).

### Pre-conditions
1. User ter-autentikasi dengan permission `transaksi.approve`
2. `trx.renewal.status = 'PENDING_APPROVAL'`
3. `maker_id ≠ approver_id` (SoD DEC-017)
4. `mst.periode_buku.status_periode = 'OPEN'`

### Endpoints

```
POST /api/v1/trx/renewal/{id}/approve
Authorization: Bearer <jwt>
Idempotency-Key: <uuid-v4>
Body: { "comment": "Preview diverifikasi. Rate 5.75% sesuai BI Rate + spread 1.75%. Disetujui.", "signature_method": "JWT_STEP_UP" }

→ 200 OK
{
  "data": {
    "renewal_id": "<uuid>",
    "status": "POSTED",
    "instrumen_baru_id": "<uuid-DEP-0042B>",
    "jurnal_entry_id": "<uuid>",
    "approved_by": "USR-APPR-001",
    "approved_at": "2026-06-19T09:15:00+07:00"
  }
}

POST /api/v1/trx/renewal/{id}/reject
Body: { "comment": "Rate 5.75% melebihi benchmark internal 5.50%. Harap revisi rate atau lampirkan persetujuan ALCO.", "signature_method": "JWT_STEP_UP" }
→ 200 OK { "data": { "status": "REJECTED", "rejected_by": "...", "comment": "..." } }
```

### Audit Events

| Action | Trigger |
|---|---|
| `RENEWAL.APPROVED` | Saat status → APPROVED — in-transaction |
| `RENEWAL.POSTED` | Saat semua side-effect selesai — in-transaction |
| `RENEWAL.REJECTED` | Saat status → REJECTED — in-transaction |

### Acceptance Criteria

```gherkin
Feature: Approve renewal deposito oleh ROLE-APPR-TR (4-eyes SoD)

  Background:
    Given trx.renewal RNW-0042: status = 'PENDING_APPROVAL', maker_id = USR-MAKER-001
    And ROLE-APPR-TR (USR-APPR-001, berbeda dari USR-MAKER-001) ter-autentikasi
    And preview server-side re-verify: bunga_bersih = 11.397.260,2740 IDR ≥ IDR 100.000

  Scenario: S2-AC1 — ROLE-APPR-TR berhasil approve renewal — status POSTED, semua side-effect in-transaction
    When USR-APPR-001 mengirim POST /api/v1/trx/renewal/RNW-0042/approve
      With Idempotency-Key: IK-RNW-APR-001
      With body: { "comment": "Preview diverifikasi. Rate 5.75% sesuai BI Rate + spread 1.75%. Disetujui.", "signature_method": "JWT_STEP_UP" }
    Then HTTP 200
    And dalam satu transaksi DB:
      | trx.renewal.status          | POSTED                            |
      | trx.renewal.approver_id     | USR-APPR-001                      |
      | mst.instrumen DEP-0042      | status = 'MATURED'                |
      | mst.instrumen DEP-0042B     | INSERT baru, status = 'ACTIVE'    |
      | ecl.amortisasi_schedule     | schedule_version + 1, EIR_baru    |
      | jrnl.jurnal_entry           | event_code = 'RENEWAL_DEPOSITO'   |
      | aud.audit_log.action        | RENEWAL.POSTED (terakhir di chain)|
    And toast ke USR-APPR-001: "Renewal DEP-0042 disetujui dan diposting. Instrumen baru DEP-0042B dibuat."
    And notifikasi ke USR-MAKER-001: "Renewal DEP-0042 Anda disetujui oleh Treasury Approver. Instrumen baru aktif."

  Scenario: S2-AC2 — Skema POKOK_PLUS_BUNGA dengan bunga_bersih < IDR 100.000 — ditolak server-side
    Given trx.renewal RNW-0043: skema = 'POKOK_PLUS_BUNGA', bunga_bersih_preview = 85.000 IDR
    When USR-APPR-001 mengirim POST /api/v1/trx/renewal/RNW-0043/approve
      With Idempotency-Key: IK-RNW-APR-002
    Then HTTP 422:
      | error.code    | RENEWAL_BUNGA_BERSIH_TOO_SMALL                                                         |
      | error.message | "bunga_bersih IDR 85.000 lebih kecil dari minimum IDR 100.000 untuk skema POKOK_PLUS_BUNGA." |
    And trx.renewal.status tetap 'PENDING_APPROVAL'

  Scenario: S2-AC3 — SoD violation: maker mencoba approve request sendiri
    Given trx.renewal RNW-0042: maker_id = USR-MAKER-001
    And USR-MAKER-001 memiliki permission transaksi.approve (dual role)
    When USR-MAKER-001 mengirim POST /api/v1/trx/renewal/RNW-0042/approve
      With Idempotency-Key: IK-RNW-SOD-001
    Then HTTP 403:
      | error.code    | SOD_VIOLATION                                                                 |
      | error.message | "maker tidak dapat menjadi approver untuk renewal yang sama (DEC-017)."       |
    And trx.renewal.status tetap 'PENDING_APPROVAL'
    And aud.audit_log.action = RENEWAL.SOD_VIOLATION_ATTEMPT (advisory)

  Scenario: S2-AC4 — Idempotency replay: approve dikirim dua kali dengan key sama
    Given USR-APPR-001 sebelumnya berhasil approve RNW-0042 dengan Idempotency-Key: IK-RNW-APR-001
    When USR-APPR-001 mengirim POST /api/v1/trx/renewal/RNW-0042/approve ulang
      With Idempotency-Key: IK-RNW-APR-001 (sama)
    Then HTTP 200 (IDEMPOTENCY_REPLAY)
    And response berisi original response dari request pertama
    And tidak ada INSERT atau UPDATE duplikat
```

---

## Story P5-M7-S3 — Auto-Create Instrumen Baru + Matured Instrumen Lama

**Actor**: System (dipicu saat `trx.renewal.status → APPROVED`)
**Trigger**: Setelah ROLE-APPR-TR approve, system secara otomatis: (a) insert `mst.instrumen` baru inherit klasifikasi + copy SPPI+BM dari instrumen lama, (b) update instrumen lama `status = 'MATURED'`. Semua dalam satu transaksi DB bersama jurnal dan EIR.
**Goal**: Instrumen baru harus inherit seluruh atribut non-temporal dari instrumen lama (klasifikasi PSAK 71, SPPI result, BM assessment, portofolio), hanya parameter temporal yang diganti (pokok_baru, rate_baru, tenor_baru, tanggal_efektif_baru). Tidak ada SPPI test ulang — renewal bukan reklasifikasi.

### Aturan Inherit

| Atribut | Instrumen Lama | Instrumen Baru |
|---|---|---|
| `klasifikasi_psak71` | AC | AC (inherit, tidak berubah) |
| `klasifikasi_locked` | TRUE | TRUE (inherit) |
| `sppi_result` | PASS | PASS (copy FK) |
| `bm_assessment_id` | `<uuid>` | `<uuid>` (copy FK) |
| `portofolio_id` | `<uuid>` | `<uuid>` (copy) |
| `counterparty_id` | `<uuid>` | `<uuid>` (copy) |
| `mata_uang` | IDR | IDR (copy) |
| `pokok` | 1.000.000.000 | `pokok_baru` dari preview |
| `rate_persen` | 5.50 | `rate_baru_persen` dari renewal |
| `tanggal_penempatan` | lama | `tanggal_efektif_baru` |
| `tanggal_jatuh_tempo` | lama | `tanggal_efektif_baru + tenor_baru` |
| `status` | ACTIVE | ACTIVE |
| `renewal_dari_instrumen_id` | NULL | `<uuid-instrumen-lama>` (traceability) |

### Acceptance Criteria

```gherkin
Feature: Auto-create instrumen baru dan matured instrumen lama saat renewal APPROVED

  Background:
    Given trx.renewal RNW-0042: status = 'APPROVED', instrumen_id = DEP-0042
    And mst.instrumen DEP-0042: klasifikasi_psak71 = 'AC', sppi_result = 'PASS', bm_assessment_id = BM-007, portofolio_id = PRT-01
    And semua dalam satu transaksi DB bersama S4 (EIR) dan S5 (jurnal)

  Scenario: S3-AC1 — Instrumen baru inherit klasifikasi + SPPI + BM dari instrumen lama
    When system menjalankan renewal approved handler untuk RNW-0042
    Then mst.instrumen INSERT DEP-0042B:
      | klasifikasi_psak71          | AC (inherit dari DEP-0042)         |
      | klasifikasi_locked          | TRUE                               |
      | sppi_result                 | PASS (copy dari DEP-0042)          |
      | bm_assessment_id            | BM-007 (copy FK dari DEP-0042)     |
      | portofolio_id               | PRT-01 (copy)                      |
      | pokok                       | pokok_baru dari preview            |
      | rate_persen                 | 5.75 (rate_baru dari renewal)      |
      | tanggal_penempatan          | 2026-07-01 (tanggal_efektif_baru)  |
      | tanggal_jatuh_tempo         | 2027-07-01 (+ tenor 12 bulan)      |
      | status                      | ACTIVE                             |
      | renewal_dari_instrumen_id   | DEP-0042 UUID                      |
    And aud.audit_log.action = INSTRUMEN.CREATED — in-transaction, actor = system (service account)

  Scenario: S3-AC2 — Instrumen lama dimatured dalam transaksi yang sama
    When sistem menjalankan renewal approved handler untuk RNW-0042
    Then mst.instrumen DEP-0042:
      | status     | MATURED                         |
      | updated_at | timestamp now                   |
      | updated_by | service account UUID            |
    And aud.audit_log.action = INSTRUMEN.MATURED — in-transaction
    And DEP-0042 tidak bisa di-submit renewal lagi (status != ACTIVE)

  Scenario: S3-AC3 — Skema POKOK_SAJA: pokok_baru = pokok_lama, bunga_bersih dikreditkan terpisah
    Given trx.renewal RNW-0044: skema = 'POKOK_SAJA', pokok_lama = 500.000.000 IDR
    When system menjalankan renewal approved handler untuk RNW-0044
    Then mst.instrumen DEP-0044B:
      | pokok | 500.000.000,0000 (sama dengan pokok_lama) |
    And jurnal leg 4 (bunga_bersih) tetap diposting terpisah sebagai penerimaan bunga

  Scenario: S3-AC4 — Jika INSERT instrumen baru gagal — seluruh transaksi rollback
    Given DB constraint violation saat INSERT DEP-0042B (mis. duplikat kode_instrumen)
    When system menjalankan renewal approved handler untuk RNW-0042
    Then seluruh transaksi rollback:
      | mst.instrumen DEP-0042    | status tetap ACTIVE (bukan MATURED)  |
      | trx.renewal RNW-0042      | status tetap APPROVED (bukan POSTED) |
      | ecl.amortisasi_schedule   | tidak ada INSERT schedule baru       |
      | jrnl.jurnal_entry         | tidak ada INSERT jurnal              |
    And RENEWAL.ROLLBACK dicatat di aud.audit_log (advisory)
    And notifikasi ke ROLE-IT-ADMIN: "Renewal RNW-0042 gagal diproses setelah approval. Transaksi di-rollback. Periksa DLQ."
```

---

## Story P5-M7-S4 — EIR Re-Computation (Newton-Raphson, Schedule Version)

**Actor**: System (dipicu saat `trx.renewal.status → APPROVED`, dalam transaksi yang sama)
**Trigger**: Setelah instrumen baru dibuat (S3), system menghitung EIR baru menggunakan Newton-Raphson dengan cashflow dari instrumen baru, lalu insert `ecl.amortisasi_schedule` dengan `schedule_version = old_version + 1`, `effective_from = tanggal_efektif_baru`, `effective_to = 'infinity'`. Schedule lama di-update: `effective_to = tanggal_efektif_baru`. Schedule lama TIDAK pernah di-UPDATE nilai EIR-nya (immutability).
**Goal**: EIR baru tersimpan dengan versioning yang audit-grade. Newton-Raphson wajib konvergen; jika gagal konvergen → error explicit `RENEWAL_EIR_NO_CONVERGENCE` (tidak return garbage).

### Compliance Note (ifrs9-compliance-reviewer — BLOCKING)
- PPh 20% deposito bank diaplikasikan ke setiap kupon cashflow sebelum dimasukkan ke solver (cashflow bersih setelah pajak).
- Re-estimation EIR atas renewal = amendment kontrak per PSAK 71 paragraf 5.4.3 → insert schedule baru, bukan modifikasi schedule lama.

### Acceptance Criteria

```gherkin
Feature: EIR re-computation Newton-Raphson saat renewal approved — schedule version immutability

  Background:
    Given trx.renewal RNW-0042: APPROVED, pokok_baru = 1.011.397.260,2740 IDR, rate_baru = 5.75%, tenor = 12 bulan
    And ecl.amortisasi_schedule DEP-0042: schedule_version = 1, EIR_lama = 0.04400000, effective_to = 'infinity'

  Scenario: S4-AC1 — EIR baru dihitung Newton-Raphson dan schedule version baru diinsert
    When system menghitung EIR untuk instrumen baru DEP-0042B
    Then Newton-Raphson konvergen dalam ≤ 100 iterasi dengan tolerance 1e-10
    And ecl.amortisasi_schedule INSERT DEP-0042B:
      | instrumen_id     | DEP-0042B UUID (instrumen baru)     |
      | schedule_version | 1 (mulai dari 1 untuk instrumen baru)|
      | EIR_persen       | hasil Newton-Raphson, 8 desimal      |
      | effective_from   | 2026-07-01 (tanggal_efektif_baru)   |
      | effective_to     | 'infinity'                          |
    And ecl.amortisasi_schedule DEP-0042 schedule_version = 1:
      | effective_to | 2026-07-01 (di-update, bukan nilai EIR) |
    And aud.audit_log.action = EIR.RECOMPUTED — in-transaction, after_jsonb: {instrumen_baru_id, schedule_version, EIR_baru, effective_from}

  Scenario: S4-AC2 — Cashflow solver menggunakan bunga bersih after-PPh, bukan bunga kotor
    Given kupon_per_periode_kotor = pokok_baru × (rate_baru / 100) / 12
    When system menyusun cashflow untuk Newton-Raphson
    Then cashflow kupon = kupon_per_periode_kotor × (1 − 0.20) [after PPh 20%]
    And cashflow[0] = −pokok_baru (outflow awal)
    And cashflow[12] = +pokok_baru + kupon_bersih_bulan_12 (terminal cashflow)
    And EIR yang tersimpan mencerminkan yield-to-maturity after-tax, bukan gross

  Scenario: S4-AC3 — PPh_20pct tidak sesuai kalkulasi server — ditolak RENEWAL_PPH_CALC_MISMATCH
    Given client mengirim PPh_20pct = 3.000.000 (tidak sama dengan server-compute: 2.849.315,07)
    When ROLE-APPR-TR approve dan server re-verify kalkulasi
    Then HTTP 422:
      | error.code    | RENEWAL_PPH_CALC_MISMATCH                                                                  |
      | error.message | "PPh 20% tidak sesuai: client=3.000.000, server=2.849.315.0685. Gunakan nilai server-computed." |
    And trx.renewal.status tetap 'PENDING_APPROVAL'

  Scenario: S4-AC4 — Newton-Raphson tidak konvergen dalam 100 iterasi — error explicit, rollback
    Given cashflow instrumen baru memiliki struktur tidak konvergen (edge case, mis. pokok = 0)
    When system menjalankan Newton-Raphson solver
    Then solver mengembalikan error EIR_NO_CONVERGENCE setelah 100 iterasi (bukan return garbage)
    And seluruh transaksi rollback (mirror S3-AC4)
    And aud.audit_log.action = EIR.COMPUTE_FAILED — advisory
    And notifikasi ke ROLE-IT-ADMIN: "EIR solver gagal konvergen untuk renewal RNW-0042. Periksa input cashflow."
```

---

## Story P5-M7-S5 — Jurnal Posting RENEWAL_DEPOSITO (Multi-Leg, P5-M2 Engine)

**Actor**: System (dipicu saat `trx.renewal.status → APPROVED`, dalam transaksi yang sama)
**Trigger**: Setelah S3 (instrumen baru) dan S4 (EIR) selesai, system memanggil P5-M2 jurnal engine dengan event code `RENEWAL_DEPOSITO`. Multi-leg entry: (1) PPh, (2) pelunasan pokok lama, (3) penempatan pokok baru, (4) bunga bersih. Periode buku harus OPEN; jika CLOSED → error `PERIODE_CLOSED`, rollback seluruh transaksi.
**Goal**: Jurnal multi-leg benar, pokok_baru di-reflect sebagai liability baru, bunga_bersih sebagai revenue, PPh sebagai kewajiban pajak. Skema POKOK_SAJA vs POKOK_PLUS_BUNGA hanya mempengaruhi nilai pokok_baru leg 3 — struktur jurnal tetap sama.

### Acceptance Criteria

```gherkin
Feature: Jurnal posting RENEWAL_DEPOSITO multi-leg via P5-M2 engine

  Background:
    Given trx.renewal RNW-0042: APPROVED, skema = 'POKOK_PLUS_BUNGA'
    And kalkulasi server: bunga_kotor = 14.246.575,3425, PPh_20pct = 2.849.315,0685,
        bunga_bersih = 11.397.260,2740, pokok_baru = 1.011.397.260,2740
    And P5-M2 jurnal engine berjalan dengan event code RENEWAL_DEPOSITO tersedia di mapping master

  Scenario: S5-AC1 — Jurnal 4 leg diposting dengan nilai yang benar (POKOK_PLUS_BUNGA)
    When system memanggil P5-M2 POST /api/v1/jurnal/post-entry
      With event_code = 'RENEWAL_DEPOSITO'
      With instrumen_lama_id = DEP-0042, instrumen_baru_id = DEP-0042B, renewal_id = RNW-0042
    Then P5-M2 menghasilkan 4 leg jurnal:
      | Leg 1 | Dr Kewajiban PPh Deposito  Cr Kas/Bank  | 2.849.315,0685  | PPh final 20%         |
      | Leg 2 | Dr Deposito (lama DEP-0042) Cr Kas/Bank | 1.000.000.000   | Pelunasan pokok lama  |
      | Leg 3 | Dr Kas/Bank Cr Deposito (baru DEP-0042B)| 1.011.397.260,2740 | Penempatan pokok baru |
      | Leg 4 | Dr Beban Bunga Deposito Cr Kas/Bank     | 11.397.260,2740 | Bunga bersih diterima |
    And semua leg menggunakan NUMERIC(20,4) — tidak ada float
    And trx.renewal.jurnal_entry_id = <uuid dari P5-M2>
    And aud.audit_log.action = RENEWAL.POSTED — in-transaction

  Scenario: S5-AC2 — Skema POKOK_SAJA: leg 3 menggunakan pokok_lama, bukan pokok + bunga
    Given trx.renewal RNW-0045: skema = 'POKOK_SAJA', pokok_lama = 500.000.000
    And bunga_bersih = 6.780.821,9178
    When system memanggil P5-M2 untuk RNW-0045
    Then jurnal leg 3: Cr Deposito DEP-0045B = 500.000.000 (bukan 506.780.821,9178)
    And jurnal leg 4: Cr Kas/Bank bunga_bersih = 6.780.821,9178 (tetap dikreditkan ke maker)

  Scenario: S5-AC3 — Periode buku CLOSED saat jurnal diposting — rollback seluruh transaksi renewal
    Given mst.periode_buku PRD-2026-07: status_periode = 'CLOSED' (hard-close terjadi antara approve dan posting)
    When system memanggil P5-M2 untuk RNW-0042
    Then P5-M2 mengembalikan error PERIODE_CLOSED
    And seluruh transaksi rollback:
      | trx.renewal      | status kembali ke 'APPROVED' (bukan 'POSTED') |
      | mst.instrumen    | DEP-0042B tidak ada, DEP-0042 tetap ACTIVE    |
      | ecl.amortisasi_schedule | schedule baru tidak ada                |
    And notifikasi ke ROLE-APPR-TR + ROLE-MAKER-001: "Renewal RNW-0042 gagal diposting: Periode PRD-2026-07 sudah hard-closed. Hubungi Finance Controller."

  Scenario: S5-AC4 — Event code RENEWAL_DEPOSITO belum di-seed di mapping jurnal master P5-M2
    Given mapping jurnal master P5-M2 tidak memiliki entry event_code = 'RENEWAL_DEPOSITO'
    When system memanggil P5-M2 untuk RNW-0042
    Then P5-M2 mengembalikan error JURNAL_EVENT_CODE_NOT_FOUND
    And seluruh transaksi rollback (mirror S5-AC3)
    And aud.audit_log.action = RENEWAL.JURNAL_MISSING_CONFIG — advisory
    And notifikasi ke ROLE-IT-ADMIN: "Mapping jurnal RENEWAL_DEPOSITO belum dikonfigurasi di P5-M2. Setup diperlukan sebelum renewal bisa diposting."
```

---

## Ringkasan P5-M7 Story Set

| Story | Judul | Actor Utama | AC Count | Gate |
|---|---|---|---|---|
| P5-M7-S1 | Create renewal request + preview kalkulasi | ROLE-MAKER-TR | 4 | advisory |
| P5-M7-S2 | Approve renewal (4-eyes SoD) | ROLE-APPR-TR | 4 | **security-engineer BLOCKING** (SoD + audit) |
| P5-M7-S3 | Auto-create instrumen baru + matured instrumen lama | System | 4 | advisory + ifrs9 (inherit klasifikasi) |
| P5-M7-S4 | EIR re-computation Newton-Raphson + schedule versioning | System | 4 | **ifrs9-compliance-reviewer BLOCKING** (EIR + PPh) |
| P5-M7-S5 | Jurnal posting RENEWAL_DEPOSITO multi-leg via P5-M2 | System | 4 | **ifrs9-compliance-reviewer BLOCKING** (akun mapping) |
| **Total** | | | **20** | |

---

## Error Codes Proposed (Baru — untuk system-analyst)

| Code | HTTP | Trigger | Catatan |
|---|---|---|---|
| `RENEWAL_INSTRUMEN_NOT_ELIGIBLE` | 422 | Instrumen bukan deposito ACTIVE, atau sudah punya renewal PENDING/POSTED | Lebih spesifik dari `VALIDATION_FAILED` |
| `RENEWAL_SKEMA_INVALID` | 400 | Nilai `skema` bukan `POKOK_SAJA` atau `POKOK_PLUS_BUNGA` | Enum validation |
| `RENEWAL_TENOR_OUT_OF_RANGE` | 400 | `tenor_baru_bulan` < 1 atau > 60 | Detail: nilai aktual + range |
| `RENEWAL_RATE_OUT_OF_RANGE` | 400 | `rate_baru_persen` < 0 atau > 30 | Detail: nilai aktual + range |
| `RENEWAL_BUNGA_BERSIH_TOO_SMALL` | 422 | skema `POKOK_PLUS_BUNGA` dan `bunga_bersih < IDR 100.000` | Server-side re-verify saat approve |
| `RENEWAL_PPH_CALC_MISMATCH` | 422 | Client-submitted PPh ≠ server-computed PPh (toleransi 0) | Prevent client-side manipulation |

Catatan: `SOD_VIOLATION` (HTTP 403), `PERIODE_CLOSED` (HTTP 423), `VALIDATION_FAILED` (HTTP 400), `IDEMPOTENCY_REPLAY` (HTTP 200), `NOT_FOUND` (HTTP 404) sudah ada di `api-conventions.md` — tidak ditambahkan ulang.

---

## Persona Summary Table

| Actor | Permission | Aksi di P5-M7 | MFA Level |
|---|---|---|---|
| ROLE-MAKER-TR | `transaksi.create`, `transaksi.read` | Create renewal request, view preview, view status | Tidak wajib |
| ROLE-APPR-TR | `transaksi.approve`, `transaksi.read` | Approve/reject renewal (SoD ≠ maker), view antrian | Wajib jika Treasury Manager (DEC-026) |
| ROLE-RISK | `transaksi.read`, `instrumen.read` | View renewal list + EIR baru; tidak ada aksi mutasi | Tidak wajib |
| ROLE-AKUN | `transaksi.read`, `jurnal.read` | View jurnal RENEWAL_DEPOSITO yang terposting | Tidak wajib |
| ROLE-AUDIT | `transaksi.read`, `audit_log.read` | Read-only seluruh renewal data + audit trail | Tidak wajib |
| System (handler) | Service account | Insert instrumen baru, matured lama, EIR schedule, jurnal post | N/A |

---

## Dependensi Lintas Modul

| Dependensi | Arah | Keterangan |
|---|---|---|
| `mst.instrumen` ACTIVE + klasifikasi locked | P5-M1 → P5-M7 | Renewal hanya untuk deposito ACTIVE dengan SPPI + BM final |
| Jurnal engine + event code `RENEWAL_DEPOSITO` | P5-M2 → P5-M7 | 4-leg entry harus tersedia di mapping jurnal master P5-M2 sebelum renewal bisa diposting |
| `ecl.amortisasi_schedule` pattern (schedule_version) | P5-M1/APP-C → P5-M7 | Versioning `effective_from/effective_to` mengikuti pola EIR amendment dari FSD-APP-C |
| `mst.periode_buku.status_periode = 'OPEN'` | P5-M4 → P5-M7 | Renewal tidak bisa dipost ke periode CLOSED |
| Migration 000042 (`trx.renewal`) | P5-M7 → data-modeler | Tabel baru; audit columns; FK ke mst.instrumen + trx.jurnal_entry |
| Migration 000043 (`ecl.amortisasi_schedule` index) | P5-M7 → data-modeler | Index `(instrumen_id, schedule_version DESC)` + partial `WHERE effective_to = 'infinity'` |

---

## Compliance & Security Handoff Checklist

### Untuk ifrs9-compliance-reviewer (BLOCKING gate — S4 + S5)
- [ ] EIR cashflow solver menggunakan bunga after-PPh 20% (PSAK 71 §5.4.1 + PP No. 131/2000) — bukan gross yield
- [ ] Renewal = amendment kontrak per PSAK 71 §5.4.3 → EIR re-estimation wajib, bukan hanya update rate
- [ ] `ecl.amortisasi_schedule` insert baru (schedule_version + 1), `effective_to` lama di-update — tidak ada UPDATE nilai EIR lama (immutability PSAK 71 §B5.4.6)
- [ ] Instrumen baru inherit klasifikasi AC dari instrumen lama — renewal bukan triggering event untuk reklasifikasi (PSAK 71 §4.4.1)
- [ ] Skema `POKOK_PLUS_BUNGA`: pokok baru = pokok lama + bunga_bersih (after PPh) — bunga kotor tidak masuk ke pokok baru
- [ ] Jurnal leg 1 (PPh): akun `Kewajiban PPh Deposito` — konfirmasi akun GL sesuai mapping P5-M2 + chart of accounts Tugure
- [ ] PPh 20% sesuai PP No. 131/2000 tentang pajak penghasilan atas bunga deposito bank — rate fixed, tidak berubah berdasarkan tier
- [ ] Konfirmasi minimum bunga_bersih IDR 100.000 adalah business rule Tugure (bukan PSAK 71) — dokumentasi di BRD §6.2

### Untuk security-engineer (BLOCKING — S2 SoD)
- [ ] SoD enforcement `maker_id ≠ approver_id` di service layer — tidak hanya DB constraint
- [ ] `RENEWAL.APPROVED`, `RENEWAL.POSTED`, `RENEWAL.REJECTED` ditulis in-transaction
- [ ] `INSTRUMEN.CREATED` (instrumen baru) dan `INSTRUMEN.MATURED` (instrumen lama) in-transaction bersama RENEWAL.POSTED
- [ ] `EIR.RECOMPUTED` in-transaction — bukan async
- [ ] Idempotency-Key cek di approve endpoint — mencegah double-approve yang create instrumen duplikat
- [ ] Export `GET /trx/renewal/export` — audit `RENEWAL.EXPORT` in-transaction; ROLE-AUDIT read-only enforcement
- [ ] Rate limit approve endpoint: 10 req/menit (sensitif per api-conventions.md)

---

_Story set ini siap dihandoff ke `system-analyst` untuk OpenAPI contract + state machine `trx.renewal.status`, ke `ifrs9-compliance-reviewer` untuk review S4 (EIR + PPh BLOCKING) dan S5 (jurnal multi-leg BLOCKING), dan ke `security-engineer` untuk review S2 (SoD + audit BLOCKING). `data-modeler` memulai migration 000042 + 000043 paralel setelah compliance gate S4 cleared._
