# P5-M8 — APP-B Penjualan/Pencairan Instrumen: User Stories

**Story Set ID**: P5-M8
**Modul**: APP-B — Transaction Lifecycle (Penjualan/Pencairan, Phase 5)
**Status**: DRAFT — menunggu handoff ke `system-analyst` + `ifrs9-compliance-reviewer`
**Author**: business-analyst
**Tanggal**: 2026-06-20
**Linked FSD**: FSD-APP-B-TransactionLifecycle-v1.0.docx §6 (Penjualan/Pencairan), §7 (Derecognition + OCI Recycling)
**Linked BRD**: BRD §6.3 (APP-B Penjualan), RACI: ROLE-APPR-TR (A), ROLE-MAKER-TR (R), ROLE-RISK (C), ROLE-AKUN (C), ROLE-AUDIT (I)
**Linked Decision Log**:
- `DEC-016` (LOCKED) — `shopspring/decimal`; `NUMERIC(20,4)` IDR, `NUMERIC(10,8)` EIR; **never float64**
- `DEC-017` (LOCKED) — 4-eyes SoD: `maker_id ≠ reviewer_id ≠ approver_id`; enforced server-side
- `DEC-018` (LOCKED) — audit trail append-only, retensi 10+10 tahun
- `DEC-021` (LOCKED) — Idempotency-Key wajib di setiap mutating endpoint

**Dependensi**:
- **P5-M1** — `mst.instrumen` ACTIVE, klasifikasi PSAK 71 locked (`klasifikasi_locked = TRUE`), SPPI + BM final; `penjualan` hanya untuk instrumen `status = 'ACTIVE'`
- **P5-M2** — jurnal engine; event codes `PENJUALAN_AC`, `PENJUALAN_FVOCI_DEBT`, `PENJUALAN_FVOCI_ELECTION`, `PENJUALAN_FVTPL`, `PENJUALAN_POCI` harus tersedia di mapping master sebelum posting
- **Phase 4 ECL** — staging aktif (`ecl.staging_history`) diperlukan untuk menentukan akrual bunga Stage 3 (net carrying) yang masuk dalam cost_basis penjualan

**Handoff berikutnya**:
- `system-analyst` → OpenAPI: 4 endpoints (`POST /trx/penjualan`, `GET /trx/penjualan/{id}`, `POST /trx/penjualan/{id}/approve`, `POST /trx/penjualan/{id}/reject`); state machine `trx.penjualan.status`; error codes baru (§Error Codes Proposed)
- `data-modeler` → migration baru (`trx.penjualan` tabel; `trx.penjualan_leg` untuk partial tracking); FK ke `mst.instrumen`, `ecl.amortisasi_schedule`, `jrnl.jurnal_entry`
- `ifrs9-compliance-reviewer` → **BLOCKING gate** untuk: (a) OCI recycling FVOCI debt vs no-recycling FVOCI Election (S3); (b) BM Test frequency trigger dan threshold HTC (S4); (c) jurnal multi-leg per klasifikasi (S5); (d) partial vs full derecognition amortized cost
- `security-engineer` → SoD enforcement (S2), audit trail in-transaction (S1–S5), idempotency approve

**Compliance path**: P5-M8 adalah **regulated path** — menyentuh OCI recycling (PSAK 71 §B5.7.1), Business Model reclassification trigger (PSAK 71 §4.4.1), dan multi-klasifikasi derecognition accounting. **ifrs9-compliance-reviewer BLOCKING** wajib sebelum implementasi S3 (OCI), S4 (BM trigger), S5 (jurnal). **security-engineer BLOCKING** untuk S2 (SoD + audit).

---

## Konteks & Arsitektur P5-M8

### Alur Penjualan/Pencairan

```
ROLE-MAKER-TR
  │  Pilih mst.instrumen (status='ACTIVE', klasifikasi_locked=TRUE)
  │  Pilih tipe penjualan: FULL atau PARTIAL
  │  Input: qty_terjual (unit, untuk obligasi/saham) ATAU notional_sold (IDR, untuk deposito/sukuk)
  │         harga_jual (per unit atau total notional)
  │         tanggal_eksekusi
  │  Preview kalkulasi:
  │    cost_basis       = dari ecl.amortisasi_schedule (amortized carrying, per klasifikasi)
  │    realized_g_l     = proceeds − cost_basis
  │    OCI_recycled     = cumulative OCI (untuk FVOCI debt full disposal)
  │    no_recycling_note = "OCI cumulative tetap di ekuitas" (untuk FVOCI Election)
  │    BM_freq_impact   = persentase cumulative penjualan 12-bulan rolling di portofolio HTC
  │  Submit → status = 'PENDING_APPROVAL'
  │
ROLE-APPR-TR (SoD: ≠ maker)
  │  Review preview, validate proceeds vs harga pasar IBPA/BEI (jika obligasi/saham)
  │  Approve → APPROVED → POSTED (side-effects S3, S4, S5 dalam satu transaksi)
  │  Reject  → REJECTED, comment ≥ 30 char
  │
System (on APPROVED)
  │  S3: OCI recycling jurnal (FVOCI debt) ATAU no-recycling note (FVOCI Election)
  │  S4: BM frequency check — jika melewati threshold → flag BM_VIOLATION_RISK + notif ROLE-RISK
  │  S5: Jurnal multi-leg per klasifikasi via P5-M2 + derecognition:
  │        FULL  → mst.instrumen.status = 'DISPOSED'
  │        PARTIAL → mst.instrumen.qty_holding -= qty_terjual (soft partial update)
  │  Audit: seluruh events in-transaction
```

### State Machine `trx.penjualan.status`

```
DRAFT (preview tersimpan, belum submit)
  │
  [submit] ROLE-MAKER-TR
  ↓
PENDING_APPROVAL
  │
  ├─ [approve] ROLE-APPR-TR (SoD: ≠ maker_id)
  │    → APPROVED
  │    → trigger: OCI check (S3), BM frequency check (S4), jurnal + derecognition (S5)
  │    → POSTED (setelah semua side-effect selesai dalam satu transaksi)
  │
  ├─ [reject] ROLE-APPR-TR (comment ≥ 30 char)
  │    → REJECTED → notif maker
  │
  [reopen] ROLE-MAKER-TR (REJECTED → DRAFT untuk amend)
  ↓
POSTED (immutable; instrumen DISPOSED atau partial qty updated)
```

### Preview Kalkulasi

```
# Cost basis per klasifikasi
AC / POCI:
    cost_basis = amortized_carrying_amount dari ecl.amortisasi_schedule (tanggal_eksekusi)
    realized_g_l = proceeds_IDR − cost_basis   [booked di P&L]

FVOCI debt:
    cost_basis = amortized_carrying_amount (sama seperti AC)
    oci_cumulative = Σ fair_value_changes di OCI sejak inisiasi
    realized_g_l = proceeds_IDR − cost_basis
    OCI_recycled = oci_cumulative [jika FULL disposal → REKLAS_OCI_PL]

FVOCI Election (saham):
    cost_basis = cost pada saat inisiasi (tidak diamortisasi)
    realized_g_l = proceeds_IDR − cost_basis   [TIDAK dibooked ke P&L — stays in OCI]
    no_recycling_note = "Gain/loss tetap di OCI per PSAK 71 §B5.7.1"

FVTPL:
    cost_basis = fair_value terakhir (MTM terkini dari trx.mtm)
    realized_g_l = proceeds_IDR − cost_basis   [booked di P&L]

# Partial penjualan
proceeds_IDR   = harga_jual × qty_terjual (obligasi/saham) ATAU notional_sold (deposito)
cost_basis_partial = cost_basis_total × (qty_terjual / qty_holding_total)
```

### Jurnal Multi-Leg per Klasifikasi

| Klasifikasi | Event Code P5-M2 | Keterangan |
|---|---|---|
| AC | `PENJUALAN_AC` | Dr Kas/Bank, Cr Aset AC, Dr/Cr Realized G/L P&L |
| FVOCI debt (full) | `PENJUALAN_FVOCI_DEBT` | + REKLAS_OCI_PL: Dr/Cr OCI reserve → P&L |
| FVOCI debt (partial) | `PENJUALAN_FVOCI_DEBT` | Proportional OCI recycle untuk bagian terjual |
| FVOCI Election | `PENJUALAN_FVOCI_ELECTION` | Dr Kas/Bank, Cr Aset FVOCI; G/L stays in OCI (no P&L) |
| FVTPL | `PENJUALAN_FVTPL` | Dr Kas/Bank, Cr Aset FVTPL, Dr/Cr Realized G/L P&L |
| POCI | `PENJUALAN_POCI` | Sama seperti AC, credit-adjusted EIR basis |

---

## Story P5-M8-S1 — Create Penjualan Request (ROLE-MAKER-TR)

**Actor**: ROLE-MAKER-TR
**Trigger**: ROLE-MAKER-TR membuka `/trx/penjualan/new`, memilih instrumen aktif (AC/FVOCI/FVTPL/POCI/FVOCI_ELECTION), memilih FULL atau PARTIAL, input qty/notional_sold + harga_jual + tanggal_eksekusi, melihat preview kalkulasi, dan submit.
**Goal**: Request masuk `PENDING_APPROVAL`. Preview akurat per klasifikasi — cost_basis dari amortisasi schedule, realized_g_l, OCI_recycled (jika applicable), no_recycling_note (jika FVOCI Election), BM frequency impact. Validasi server-side: instrumen ACTIVE + klasifikasi locked, qty/notional tidak melebihi holding, harga_jual > 0, periode OPEN, klasifikasi bukan DRAFT.

### Pre-conditions
1. User ter-autentikasi dengan permission `transaksi.create`
2. Request mengandung `Idempotency-Key` header (UUID v4)
3. `mst.instrumen` target: `status = 'ACTIVE'`, `klasifikasi_locked = TRUE`
4. `mst.periode_buku.status_periode = 'OPEN'` untuk `tanggal_eksekusi`
5. Tidak ada `trx.penjualan` PENDING untuk instrumen yang sama (full disposal tidak boleh duplikat)

### Endpoint

```
POST /api/v1/trx/penjualan
Authorization: Bearer <jwt>
Idempotency-Key: <uuid-v4>

Body:
{
  "instrumen_id": "<uuid>",
  "tipe_penjualan": "PARTIAL",
  "qty_terjual": 500,
  "harga_jual": 1050000.0000,
  "tanggal_eksekusi": "2026-07-15"
}

→ 201 Created
{
  "data": {
    "penjualan_id": "<uuid>",
    "status": "PENDING_APPROVAL",
    "preview": {
      "klasifikasi_psak71": "FVOCI",
      "proceeds_IDR": 525000000.0000,
      "cost_basis": 498500000.0000,
      "realized_g_l": 26500000.0000,
      "OCI_recycled": 18200000.0000,
      "no_recycling_note": null,
      "bm_freq_impact_pct": 3.2000,
      "bm_freq_warning": null
    }
  }
}
```

### Audit Events

| Action | Trigger |
|---|---|
| `PENJUALAN.CREATED` | Insert `trx.penjualan` — in-transaction. `after_jsonb`: `{instrumen_id, klasifikasi, tipe_penjualan, qty_terjual, harga_jual, proceeds_IDR, cost_basis, realized_g_l, tanggal_eksekusi}` |

### Acceptance Criteria

```gherkin
Feature: Create penjualan request oleh ROLE-MAKER-TR

  Background:
    Given user ROLE-MAKER-TR (USR-MAKER-001) ter-autentikasi dengan permission transaksi.create
    And mst.instrumen OBL-0077:
      | status             | ACTIVE              |
      | klasifikasi_psak71 | FVOCI               |
      | klasifikasi_locked | TRUE                |
      | qty_holding        | 1000 unit           |
      | harga_perolehan    | 1000000.0000 / unit |
    And mst.periode_buku PRD-2026-07: status_periode = 'OPEN'

  Scenario: S1-AC1 — Maker berhasil create penjualan PARTIAL FVOCI dengan preview lengkap
    Given qty_terjual = 500, harga_jual = 1050000.0000, tanggal_eksekusi = 2026-07-15
    And cost_basis per ecl.amortisasi_schedule = 498500000.0000 (untuk 500 unit)
    And oci_cumulative dari trx.mtm = 18200000.0000
    When USR-MAKER-001 mengirim POST /api/v1/trx/penjualan
      With Idempotency-Key: IK-PJL-001
    Then HTTP 201
    And preview.proceeds_IDR = 500 × 1050000 = 525000000.0000 (NUMERIC(20,4))
    And preview.cost_basis = 498500000.0000 (dari amortisasi schedule, bukan float)
    And preview.realized_g_l = 525000000.0000 − 498500000.0000 = 26500000.0000
    And preview.OCI_recycled = 18200000.0000 (proportional 500/1000 dari oci_cumulative)
    And preview.no_recycling_note = null (bukan FVOCI Election)
    And preview.bm_freq_impact_pct = persentase qty terjual vs total portofolio HTC rolling 12 bulan
    And trx.penjualan INSERT: status = 'PENDING_APPROVAL', maker_id = USR-MAKER-001
    And aud.audit_log.action = PENJUALAN.CREATED — in-transaction
    And toast: "Penjualan OBL-0077 (500 unit) berhasil dibuat (PJL-{nomor}). Menunggu approval Treasury Approver."

  Scenario: S1-AC2 — Qty terjual melebihi qty_holding — ditolak PENJUALAN_QTY_EXCEEDS_HOLDING
    Given qty_terjual = 1500, qty_holding = 1000
    When USR-MAKER-001 mengirim POST /api/v1/trx/penjualan
      With Idempotency-Key: IK-PJL-002
    Then HTTP 422:
      | error.code              | PENJUALAN_QTY_EXCEEDS_HOLDING                                       |
      | error.details[0].field  | qty_terjual                                                         |
      | error.details[0].rule   | "qty_terjual 1500 melebihi qty_holding saat ini: 1000 unit OBL-0077" |
    And tidak ada INSERT ke trx.penjualan

  Scenario: S1-AC3 — Instrumen tidak ACTIVE atau klasifikasi belum locked — ditolak
    Given mst.instrumen OBL-0099: status = 'MATURED', klasifikasi_locked = FALSE
    When USR-MAKER-001 mengirim POST /api/v1/trx/penjualan
      With body: instrumen_id = OBL-0099
      With Idempotency-Key: IK-PJL-003
    Then HTTP 422:
      | error.code    | PENJUALAN_INSTRUMEN_NOT_ACTIVE                                                          |
      | error.message | "OBL-0099 tidak eligible untuk penjualan: status = MATURED. Hanya instrumen ACTIVE yang bisa dijual." |
    And tidak ada INSERT ke trx.penjualan

  Scenario: S1-AC4 — FVOCI Election: preview menampilkan no_recycling_note, realized_g_l stays in OCI
    Given mst.instrumen SHM-0011: klasifikasi_psak71 = 'FVOCI_ELECTION', status = 'ACTIVE'
    And tipe_penjualan = 'FULL', harga_jual = 12000000.0000, cost_basis = 10000000.0000
    When USR-MAKER-001 mengirim POST /api/v1/trx/penjualan
      With Idempotency-Key: IK-PJL-004
    Then HTTP 201
    And preview.OCI_recycled = null (tidak ada recycling ke P&L)
    And preview.no_recycling_note = "Gain/loss IDR 2.000.000 tetap di OCI per PSAK 71 §B5.7.1. Tidak direkognisi di P&L."
    And preview.realized_g_l = 2000000.0000 (informational only — tidak ke P&L)
    And aud.audit_log.action = PENJUALAN.CREATED — in-transaction
```

---

## Story P5-M8-S2 — Approve Penjualan (ROLE-APPR-TR, 4-Eyes SoD)

**Actor**: ROLE-APPR-TR
**Trigger**: ROLE-APPR-TR membuka antrian `PENDING_APPROVAL` di `/trx/penjualan`, memvalidasi preview, lalu approve atau reject.
**Goal**: 4-eyes SoD: `approver_id ≠ maker_id` enforced server-side. Approve → `APPROVED` → `POSTED` (side-effects S3–S5 dalam satu transaksi). Reject → `REJECTED` + comment ≥ 30 char + notif maker. Server re-verify cost_basis dari amortisasi schedule saat approval (tidak trust client preview). Idempotency-Key wajib.

### Pre-conditions
1. User ter-autentikasi dengan permission `transaksi.approve`
2. `trx.penjualan.status = 'PENDING_APPROVAL'`
3. `approver_id ≠ maker_id` (DEC-017)
4. `mst.periode_buku.status_periode = 'OPEN'` untuk `tanggal_eksekusi`

### Endpoint

```
POST /api/v1/trx/penjualan/{id}/approve
Authorization: Bearer <jwt>
Idempotency-Key: <uuid-v4>
Body: { "comment": "Preview diverifikasi. Harga OBL-0077 sesuai IBPA closing 2026-07-15. Disetujui.", "signature_method": "JWT_STEP_UP" }

→ 200 OK
{
  "data": {
    "penjualan_id": "<uuid>",
    "status": "POSTED",
    "jurnal_entry_id": "<uuid>",
    "instrumen_status_after": "DISPOSED",
    "approved_by": "USR-APPR-001",
    "approved_at": "2026-07-15T14:00:00+07:00"
  }
}

POST /api/v1/trx/penjualan/{id}/reject
Body: { "comment": "Harga jual 1.050.000 melebihi IBPA fair value 1.035.000 lebih dari 2%. Harap klarifikasi atau revisi harga.", "signature_method": "JWT_STEP_UP" }
→ 200 OK { "data": { "status": "REJECTED", "rejected_by": "...", "comment": "..." } }
```

### Audit Events

| Action | Trigger |
|---|---|
| `PENJUALAN.APPROVED` | status → APPROVED — in-transaction |
| `PENJUALAN.POSTED` | semua side-effect selesai — in-transaction |
| `PENJUALAN.REJECTED` | status → REJECTED — in-transaction |

### Acceptance Criteria

```gherkin
Feature: Approve penjualan oleh ROLE-APPR-TR (4-eyes SoD)

  Background:
    Given trx.penjualan PJL-0077: status = 'PENDING_APPROVAL', maker_id = USR-MAKER-001
    And ROLE-APPR-TR (USR-APPR-001, berbeda dari USR-MAKER-001) ter-autentikasi
    And server re-verify cost_basis = 498500000.0000 (dari ecl.amortisasi_schedule tanggal_eksekusi 2026-07-15)

  Scenario: S2-AC1 — Approver berhasil approve — status POSTED, side-effects in-transaction
    When USR-APPR-001 mengirim POST /api/v1/trx/penjualan/PJL-0077/approve
      With Idempotency-Key: IK-PJL-APR-001
      With body: { "comment": "Preview diverifikasi. Harga OBL-0077 sesuai IBPA closing 2026-07-15. Disetujui.", "signature_method": "JWT_STEP_UP" }
    Then HTTP 200
    And dalam satu transaksi DB:
      | trx.penjualan.status       | POSTED                                   |
      | trx.penjualan.approver_id  | USR-APPR-001                             |
      | mst.instrumen OBL-0077     | status = 'DISPOSED' (jika FULL)          |
      | jrnl.jurnal_entry          | event_code = 'PENJUALAN_FVOCI_DEBT'      |
      | aud.audit_log              | PENJUALAN.POSTED (terakhir di chain)     |
    And toast ke USR-APPR-001: "Penjualan OBL-0077 (PJL-0077) disetujui dan diposting. Instrumen DISPOSED."
    And notifikasi ke USR-MAKER-001: "Penjualan OBL-0077 Anda disetujui. Jurnal terposting."

  Scenario: S2-AC2 — SoD violation: maker mencoba approve penjualan sendiri
    Given trx.penjualan PJL-0077: maker_id = USR-MAKER-001
    When USR-MAKER-001 mengirim POST /api/v1/trx/penjualan/PJL-0077/approve
      With Idempotency-Key: IK-PJL-SOD-001
    Then HTTP 403:
      | error.code    | SOD_VIOLATION                                                          |
      | error.message | "maker tidak dapat menjadi approver untuk penjualan yang sama (DEC-017)." |
    And trx.penjualan.status tetap 'PENDING_APPROVAL'
    And aud.audit_log.action = PENJUALAN.SOD_VIOLATION_ATTEMPT (advisory)

  Scenario: S2-AC3 — Periode buku CLOSED saat approval diproses — rollback + error
    Given mst.periode_buku PRD-2026-07: status_periode = 'CLOSED'
    When USR-APPR-001 mengirim POST /api/v1/trx/penjualan/PJL-0077/approve
      With Idempotency-Key: IK-PJL-APR-002
    Then HTTP 423:
      | error.code    | PENJUALAN_PERIODE_LOCKED                                                         |
      | error.message | "Periode PRD-2026-07 sudah hard-closed. Posting penjualan tidak bisa dilakukan." |
    And trx.penjualan.status tetap 'PENDING_APPROVAL'
    And tidak ada INSERT ke jrnl.jurnal_entry atau mutasi mst.instrumen

  Scenario: S2-AC4 — Idempotency replay: approve dikirim dua kali dengan key sama
    Given USR-APPR-001 sebelumnya berhasil approve PJL-0077 dengan Idempotency-Key: IK-PJL-APR-001
    When USR-APPR-001 mengirim POST ulang dengan Idempotency-Key: IK-PJL-APR-001
    Then HTTP 200 (IDEMPOTENCY_REPLAY)
    And response berisi original response dari request pertama
    And tidak ada INSERT atau UPDATE duplikat ke mst.instrumen atau jrnl.jurnal_entry
```

---

## Story P5-M8-S3 — OCI Recycling (FVOCI Debt vs FVOCI Election)

**Actor**: System (dipicu saat penjualan APPROVED, dalam transaksi yang sama)
**Trigger**: Setelah approve, system menentukan perlakuan OCI berdasarkan `klasifikasi_psak71`.
**Goal**: FVOCI debt disposal → jurnal `REKLAS_OCI_PL` (transfer cumulative OCI ke P&L). FVOCI Election disposal → **tidak ada recycling ke P&L** per PSAK 71 §B5.7.1; cumulative OCI tetap di ekuitas atau dipindah ke retained earnings (non-recycled). Perlakuan partial disposal: proportional OCI recycle untuk FVOCI debt; tetap no-recycle untuk FVOCI Election. Audit `PENJUALAN.OCI_RECYCLED` atau `PENJUALAN.OCI_NO_RECYCLE` in-transaction.

### Acceptance Criteria

```gherkin
Feature: OCI recycling pada penjualan FVOCI debt dan no-recycling FVOCI Election

  Background:
    Given trx.penjualan APPROVED dalam satu transaksi DB aktif

  Scenario: S3-AC1 — FVOCI debt FULL disposal: cumulative OCI di-recycle ke P&L via REKLAS_OCI_PL
    Given mst.instrumen OBL-0077: klasifikasi_psak71 = 'FVOCI', tipe_penjualan = 'FULL'
    And oci_cumulative = +18200000.0000 (gain di OCI)
    When system menjalankan OCI recycling handler
    Then P5-M2 memposting jurnal tambahan event_code = 'REKLAS_OCI_PL':
      | Dr OCI Reserve (Ekuitas)  | 18200000.0000 |
      | Cr Realized Gain (P&L)    | 18200000.0000 |
    And aud.audit_log.action = PENJUALAN.OCI_RECYCLED — in-transaction
      With after_jsonb: { instrumen_id, oci_cumulative: 18200000.0000, direction: "GAIN", klasifikasi: "FVOCI" }
    And OCI reserve untuk OBL-0077 di-reset ke 0 setelah recycling

  Scenario: S3-AC2 — FVOCI debt PARTIAL disposal: OCI di-recycle secara proportional
    Given mst.instrumen OBL-0077: oci_cumulative = 18200000.0000, qty_holding = 1000
    And tipe_penjualan = 'PARTIAL', qty_terjual = 300
    When system menjalankan OCI recycling handler
    Then OCI_recycled = 18200000.0000 × (300 / 1000) = 5460000.0000 (NUMERIC(20,4))
    And jurnal REKLAS_OCI_PL: Dr OCI Reserve 5460000.0000 / Cr Realized Gain P&L 5460000.0000
    And sisa OCI reserve = 18200000.0000 − 5460000.0000 = 12740000.0000 (tetap di OCI untuk sisa 700 unit)
    And aud.audit_log.action = PENJUALAN.OCI_RECYCLED — in-transaction dengan qty proportional

  Scenario: S3-AC3 — FVOCI Election disposal: NO recycling ke P&L, gain/loss stays in OCI (PSAK 71 §B5.7.1)
    Given mst.instrumen SHM-0011: klasifikasi_psak71 = 'FVOCI_ELECTION', tipe_penjualan = 'FULL'
    And oci_cumulative = +2000000.0000 (gain di OCI)
    When system menjalankan OCI recycling handler
    Then tidak ada jurnal REKLAS_OCI_PL diposting
    And OCI reserve untuk SHM-0011 bisa ditransfer ke retained earnings (non-recycled) atau tetap di OCI per policy Tugure
    And aud.audit_log.action = PENJUALAN.OCI_NO_RECYCLE — in-transaction
      With after_jsonb: { instrumen_id, oci_cumulative: 2000000.0000, reason: "FVOCI_ELECTION_NO_RECYCLE_PSAK71_B5.7.1" }
    And response penjualan mengandung warning: PENJUALAN_FVOCI_ELECTION_NO_RECYCLING_WARN

  Scenario: S3-AC4 — FVOCI debt disposal dengan OCI cumulative negatif (unrealized loss): loss di-recycle ke P&L
    Given mst.instrumen OBL-0088: klasifikasi_psak71 = 'FVOCI', tipe_penjualan = 'FULL'
    And oci_cumulative = −5500000.0000 (loss di OCI)
    When system menjalankan OCI recycling handler
    Then jurnal REKLAS_OCI_PL:
      | Dr Realized Loss (P&L)   | 5500000.0000 |
      | Cr OCI Reserve (Ekuitas) | 5500000.0000 |
    And aud.audit_log.action = PENJUALAN.OCI_RECYCLED — in-transaction
      With after_jsonb: { oci_cumulative: -5500000.0000, direction: "LOSS" }
    And OCI reserve reset ke 0
```

---

## Story P5-M8-S4 — BM Test Frequency Trigger (HTC Portfolio)

**Actor**: System (dipicu in-transaction saat penjualan APPROVED) + ROLE-RISK (notifikasi)
**Trigger**: Setelah approve, system menghitung cumulative quantity/notional yang terjual dari portofolio HTC dalam rolling 12-bulan terakhir. Jika > 5% dari total nilai portofolio HTC → flag `BM_VIOLATION_RISK` dan notif ROLE-RISK. Jika melewati hard threshold (configurable, default 10%) → `PENJUALAN_BM_VIOLATION_BLOCK`.
**Goal**: Comply dengan PSAK 71 §4.1.2(b) — frekuensi penjualan dari portofolio HTC yang signifikan dapat membatalkan Business Model assessment. System propose reklasifikasi BM HTC→HTC&S ke ROLE-RISK. Audit `PENJUALAN.BM_FREQUENCY_FLAG` in-transaction.

### Acceptance Criteria

```gherkin
Feature: BM Test frequency trigger saat penjualan dari portofolio HTC

  Background:
    Given mst.instrumen OBL-0077: portofolio_id = PRT-HTC-01 (Business Model = HTC)
    And total_nilai_portofolio PRT-HTC-01 = 10000000000.0000 IDR (10 miliar)
    And sys.parameter 'BM_FREQ_WARNING_THRESHOLD_PCT' = 5.0
    And sys.parameter 'BM_FREQ_BLOCK_THRESHOLD_PCT' = 10.0
    And trx.penjualan APPROVED dalam transaksi aktif

  Scenario: S4-AC1 — Cumulative penjualan rolling 12-bulan > 5% HTC portfolio — flag + notif ROLE-RISK
    Given cumulative_sold_12m sebelum PJL-0077 = 350000000.0000 IDR (3.5%)
    And PJL-0077.proceeds_IDR = 200000000.0000 (post-approval)
    When system menghitung BM frequency check
    Then cumulative_sold_12m_baru = 550000000.0000 (5.5% dari 10 miliar) — melewati warning 5%
    And sys.bm_frequency_log INSERT: { penjualan_id: PJL-0077, portofolio_id: PRT-HTC-01, pct_terjual: 5.5, flag: 'BM_VIOLATION_RISK', tanggal_check: now() }
    And aud.audit_log.action = PENJUALAN.BM_FREQUENCY_FLAG — in-transaction
      With after_jsonb: { portofolio_id, pct_terjual: 5.5, threshold_warning: 5.0, flag: "BM_VIOLATION_RISK" }
    And notifikasi push ke semua user ROLE-RISK: "Peringatan BM HTC: Penjualan kumulatif 12-bulan PRT-HTC-01 mencapai 5.5% (threshold: 5%). Review Business Model assessment diperlukan (PSAK 71 §4.1.2b). Penjualan PJL-0077 tetap diproses."
    And system propose reklasifikasi BM HTC→HTC&S di antrian ROLE-RISK (read-only proposal, bukan auto-reklasifikasi)

  Scenario: S4-AC2 — Cumulative penjualan rolling 12-bulan > 10% HTC portfolio — block penjualan
    Given cumulative_sold_12m sebelum PJL-0099 = 980000000.0000 IDR (9.8%)
    And PJL-0099.proceeds_IDR = 250000000.0000
    When system menghitung BM frequency check
    Then cumulative_sold_12m_baru = 1230000000.0000 (12.3%) — melewati hard block 10%
    And HTTP 422 (embedded dalam approve response):
      | error.code    | PENJUALAN_BM_VIOLATION_BLOCK                                                              |
      | error.message | "Penjualan PJL-0099 menyebabkan cumulative disposal 12-bulan PRT-HTC-01 = 12.3% (hard limit: 10%). Approval ROLE-RISK diperlukan sebelum penjualan ini bisa diposting." |
    And trx.penjualan.status = 'PENDING_BM_REVIEW' (state baru, bukan POSTED)
    And notifikasi ke ROLE-RISK: "Penjualan PJL-0099 membutuhkan review BM — cumulative disposal melebihi hard threshold 10%. Approve atau reject di /risk/bm-review."
    And aud.audit_log.action = PENJUALAN.BM_FREQUENCY_FLAG — in-transaction dengan flag: "BM_VIOLATION_BLOCK"

  Scenario: S4-AC3 — Portofolio bukan HTC (HTC&S atau Other) — BM frequency check dilewati
    Given mst.instrumen SAH-0055: portofolio_id = PRT-HTCS-02 (Business Model = HTC&S)
    When system menghitung BM frequency check
    Then BM frequency check dilewati (tidak relevan untuk HTC&S / Other BM)
    And tidak ada BM_FREQUENCY_FLAG di audit log
    And penjualan lanjut ke S5 (jurnal + derecognition)

  Scenario: S4-AC4 — BM frequency threshold override oleh ALCO tersimpan di sys.parameter
    Given ROLE-ALCO sebelumnya mengupdate sys.parameter 'BM_FREQ_WARNING_THRESHOLD_PCT' = 7.5 (via APP-C parameter management)
    And cumulative_sold_12m = 7.0% dari portofolio HTC
    When system menghitung BM frequency check untuk penjualan baru
    Then threshold warning yang dipakai = 7.5% (dari sys.parameter, bukan hardcoded 5%)
    And 7.0% < 7.5% → tidak ada flag BM_VIOLATION_RISK
    And aud.audit_log mencatat parameter_version yang dipakai saat check
```

---

## Story P5-M8-S5 — Jurnal Multi-Leg + Derecognition

**Actor**: System (dipicu saat penjualan APPROVED, dalam transaksi yang sama)
**Trigger**: Setelah S3 (OCI) dan S4 (BM check) selesai, system memanggil P5-M2 jurnal engine dengan event code sesuai klasifikasi, lalu melakukan derecognition: FULL → `mst.instrumen.status = 'DISPOSED'`; PARTIAL → `mst.instrumen.qty_holding -= qty_terjual`. Semua dalam satu transaksi. Periode buku harus OPEN; jika CLOSED → `PENJUALAN_PERIODE_LOCKED`, rollback seluruh transaksi.
**Goal**: Jurnal multi-leg benar per klasifikasi. Derecognition akurat: partial tidak boleh set status ke DISPOSED. Audit `PENJUALAN.DERECOGNIZED` in-transaction.

### Acceptance Criteria

```gherkin
Feature: Jurnal multi-leg + derecognition per klasifikasi via P5-M2

  Background:
    Given trx.penjualan APPROVED, dalam satu transaksi DB aktif
    And P5-M2 jurnal engine berjalan dengan semua event codes PENJUALAN_* tersedia di mapping master

  Scenario: S5-AC1 — FULL disposal AC: jurnal 3 leg, status DISPOSED, audit DERECOGNIZED
    Given mst.instrumen DEP-0050: klasifikasi_psak71 = 'AC', tipe_penjualan = 'FULL'
    And proceeds_IDR = 1020000000.0000, cost_basis = 1000000000.0000, realized_g_l = 20000000.0000
    When system memanggil P5-M2 dengan event_code = 'PENJUALAN_AC'
    Then P5-M2 memposting jurnal:
      | Leg 1 | Dr Kas/Bank           | 1020000000.0000 | Proceeds dari penjualan  |
      | Leg 2 | Cr Aset AC (DEP-0050) | 1000000000.0000 | Derecognition carrying   |
      | Leg 3 | Cr Realized Gain P&L  | 20000000.0000   | Net gain penjualan       |
    And mst.instrumen DEP-0050: status = 'DISPOSED', updated_at = now()
    And aud.audit_log.action = PENJUALAN.DERECOGNIZED — in-transaction
      With after_jsonb: { instrumen_id, tipe: "FULL", klasifikasi: "AC", proceeds_IDR: 1020000000.0000, cost_basis: 1000000000.0000 }

  Scenario: S5-AC2 — PARTIAL disposal FVTPL: qty_holding dikurangi, status tetap ACTIVE
    Given mst.instrumen SAH-0055: klasifikasi_psak71 = 'FVTPL', qty_holding = 2000, tipe_penjualan = 'PARTIAL'
    And qty_terjual = 800, proceeds_IDR = 96000000.0000, cost_basis_partial = 88000000.0000
    When system memanggil P5-M2 dengan event_code = 'PENJUALAN_FVTPL'
    Then P5-M2 memposting jurnal:
      | Leg 1 | Dr Kas/Bank            | 96000000.0000  | Proceeds partial           |
      | Leg 2 | Cr Aset FVTPL (SAH-0055) | 88000000.0000 | Cost basis partial (MTM basis) |
      | Leg 3 | Cr Realized Gain P&L   | 8000000.0000   | Net gain partial           |
    And mst.instrumen SAH-0055: qty_holding = 2000 − 800 = 1200, status tetap 'ACTIVE'
    And aud.audit_log.action = PENJUALAN.DERECOGNIZED — in-transaction
      With after_jsonb: { tipe: "PARTIAL", qty_terjual: 800, qty_holding_after: 1200 }

  Scenario: S5-AC3 — FVOCI Election FULL disposal: jurnal tanpa REKLAS_OCI_PL, G/L stays in OCI
    Given mst.instrumen SHM-0011: klasifikasi_psak71 = 'FVOCI_ELECTION', tipe_penjualan = 'FULL'
    And proceeds_IDR = 12000000.0000, cost_basis = 10000000.0000
    When system memanggil P5-M2 dengan event_code = 'PENJUALAN_FVOCI_ELECTION'
    Then P5-M2 memposting jurnal:
      | Leg 1 | Dr Kas/Bank                  | 12000000.0000 | Proceeds                  |
      | Leg 2 | Cr Aset FVOCI Election (SHM-0011) | 10000000.0000 | Carrying amount          |
      | Leg 3 | Cr OCI Reserve (Equity)      | 2000000.0000  | G/L stays in OCI — no P&L |
    And tidak ada jurnal REKLAS_OCI_PL (konfirmasi S3-AC3)
    And mst.instrumen SHM-0011: status = 'DISPOSED'
    And aud.audit_log.action = PENJUALAN.DERECOGNIZED — in-transaction

  Scenario: S5-AC4 — Event code PENJUALAN_POCI belum tersedia di P5-M2 mapping — rollback + notif
    Given mst.instrumen POCI-0033: klasifikasi_psak71 = 'POCI', tipe_penjualan = 'FULL'
    And mapping jurnal master P5-M2 tidak memiliki event_code = 'PENJUALAN_POCI'
    When system memanggil P5-M2 untuk POCI-0033
    Then P5-M2 mengembalikan error JURNAL_EVENT_CODE_NOT_FOUND
    And seluruh transaksi rollback:
      | trx.penjualan      | status kembali ke 'APPROVED' (bukan 'POSTED') |
      | mst.instrumen      | status tidak berubah, tetap 'ACTIVE'          |
    And aud.audit_log.action = PENJUALAN.JURNAL_MISSING_CONFIG — advisory
    And notifikasi ke ROLE-IT-ADMIN: "Mapping jurnal PENJUALAN_POCI belum dikonfigurasi di P5-M2. Setup diperlukan."
```

---

## Ringkasan P5-M8 Story Set

| Story | Judul | Actor Utama | AC Count | Gate |
|---|---|---|---|---|
| P5-M8-S1 | Create penjualan request + preview multi-klasifikasi | ROLE-MAKER-TR | 4 | advisory |
| P5-M8-S2 | Approve penjualan (4-eyes SoD) | ROLE-APPR-TR | 4 | **security-engineer BLOCKING** (SoD + audit) |
| P5-M8-S3 | OCI recycling FVOCI debt vs no-recycling FVOCI Election | System | 4 | **ifrs9-compliance-reviewer BLOCKING** (§B5.7.1) |
| P5-M8-S4 | BM Test frequency trigger HTC portfolio | System + ROLE-RISK | 4 | **ifrs9-compliance-reviewer BLOCKING** (§4.1.2b) |
| P5-M8-S5 | Jurnal multi-leg + derecognition per klasifikasi | System | 4 | **ifrs9-compliance-reviewer BLOCKING** (akun mapping) |
| **Total** | | | **20** | |

---

## Error Codes Proposed (Baru — untuk system-analyst)

| Code | HTTP | Trigger | Catatan |
|---|---|---|---|
| `PENJUALAN_INSTRUMEN_NOT_ACTIVE` | 422 | Instrumen status != 'ACTIVE' atau `klasifikasi_locked = FALSE` | Lebih spesifik dari VALIDATION_FAILED |
| `PENJUALAN_QTY_EXCEEDS_HOLDING` | 422 | `qty_terjual > qty_holding` atau `notional_sold > carrying_amount` | Detail: nilai aktual vs holding |
| `PENJUALAN_KLASIFIKASI_NOT_LOCKED` | 422 | `klasifikasi_locked = FALSE` pada saat submit | Berlaku untuk semua klasifikasi |
| `PENJUALAN_HARGA_INVALID` | 400 | `harga_jual <= 0` atau format NUMERIC tidak valid | Detail: nilai aktual |
| `PENJUALAN_PERIODE_LOCKED` | 423 | `mst.periode_buku.status_periode = 'CLOSED'` untuk tanggal_eksekusi | HTTP 423 sesuai api-conventions |
| `PENJUALAN_BM_VIOLATION_BLOCK` | 422 | Cumulative disposal 12-bulan melewati hard threshold di sys.parameter | State penjualan → PENDING_BM_REVIEW, bukan rollback |
| `PENJUALAN_FVOCI_ELECTION_NO_RECYCLING_WARN` | 200 (warning in body) | Penjualan FVOCI Election berhasil — warning informatif OCI tidak di-recycle ke P&L | Bukan error, embedded di response data |

Catatan: `SOD_VIOLATION` (HTTP 403), `IDEMPOTENCY_REPLAY` (HTTP 200), `NOT_FOUND` (HTTP 404), `JURNAL_EVENT_CODE_NOT_FOUND` sudah ada di api-conventions.md atau P5-M2 — tidak ditambahkan ulang.

---

## Persona Summary Table

| Actor | Permission | Aksi di P5-M8 | MFA Level |
|---|---|---|---|
| ROLE-MAKER-TR | `transaksi.create`, `transaksi.read` | Create penjualan request, view preview, view status | Tidak wajib |
| ROLE-APPR-TR | `transaksi.approve`, `transaksi.read` | Approve/reject penjualan (SoD ≠ maker), view antrian | Wajib jika Treasury Manager (DEC-026) |
| ROLE-RISK | `transaksi.read`, `instrumen.read`, `bm_assessment.read` | Terima notif BM frequency flag; review proposal reklasifikasi BM; tidak ada aksi mutasi penjualan | Tidak wajib |
| ROLE-AKUN | `transaksi.read`, `jurnal.read` | View jurnal PENJUALAN_* yang terposting | Tidak wajib |
| ROLE-AUDIT | `transaksi.read`, `audit_log.read` | Read-only seluruh penjualan data + audit trail | Tidak wajib |
| System (handler) | Service account | OCI recycling, BM check, jurnal posting, derecognition | N/A |

---

## Dependensi Lintas Modul

| Dependensi | Arah | Keterangan |
|---|---|---|
| `mst.instrumen` ACTIVE + klasifikasi locked | P5-M1 → P5-M8 | Penjualan hanya untuk instrumen ACTIVE dengan SPPI + BM final |
| Jurnal engine + 5 event codes PENJUALAN_* | P5-M2 → P5-M8 | Semua event codes harus di-seed di mapping master sebelum penjualan bisa diposting |
| `ecl.amortisasi_schedule` (cost_basis) | APP-C / P5-M1 → P5-M8 | Amortized carrying amount diambil dari schedule aktif per tanggal_eksekusi |
| `trx.mtm` (OCI cumulative + FVTPL cost basis) | P5-M6 → P5-M8 | OCI cumulative dan fair value MTM terkini diperlukan untuk preview + jurnal |
| Phase 4 ECL staging history | Phase 4 → P5-M8 | Stage 3 net carrying (gross − ECL) mempengaruhi cost_basis penjualan AC Stage 3 |
| `mst.periode_buku.status_periode = 'OPEN'` | P5-M4 → P5-M8 | Penjualan tidak bisa diposting ke periode CLOSED |
| `sys.parameter` BM threshold | APP-C / ALCO → P5-M8 | BM_FREQ_WARNING_THRESHOLD_PCT dan BM_FREQ_BLOCK_THRESHOLD_PCT; default 5% / 10% |
| Migration baru (`trx.penjualan`, `sys.bm_frequency_log`) | P5-M8 → data-modeler | Tabel baru; FK ke mst.instrumen, jrnl.jurnal_entry; partial tracking via qty_holding_before/after |

---

## Compliance & Security Handoff Checklist

### Untuk ifrs9-compliance-reviewer (BLOCKING gate — S3, S4, S5)
- [ ] **S3**: OCI recycling FVOCI debt (full + partial proportional) → P&L sesuai PSAK 71 §5.7.10-5.7.11
- [ ] **S3**: FVOCI Election no-recycling per PSAK 71 §B5.7.1 — cumulative OCI stays in equity (atau transfer ke retained earnings non-recycled per Tugure policy — konfirmasi akun GL dengan ROLE-AKUN)
- [ ] **S3**: OCI cumulative loss juga di-recycle ke P&L saat FVOCI debt disposal (bukan hanya gain)
- [ ] **S4**: BM Test frequency threshold 5% / 10% — konfirmasi angka ini sesuai BRD §6.3 atau judgement Tugure (PSAK 71 §B4.1.3 tidak memberi angka spesifik — ini entity-specific threshold)
- [ ] **S4**: state `PENDING_BM_REVIEW` untuk hard block — perlu tambahan workflow ROLE-RISK approval sebelum POSTED
- [ ] **S5**: cost_basis Stage 3 AC menggunakan Net Carrying Amount (Gross − ECL) per PSAK 71 §5.4.1(b)
- [ ] **S5**: POCI cost_basis dari credit-adjusted EIR amortization (bukan gross EIR)
- [ ] **S5**: Jurnal leg untuk partial disposal FVTPL: cost_basis_partial = fair_value × (qty_terjual / qty_holding) — bukan amortized cost
- [ ] Konfirmasi 5 event codes PENJUALAN_* di mapping P5-M2 ter-cover sebelum M8 mulai diimplementasi

### Untuk security-engineer (BLOCKING — S2 SoD + audit)
- [ ] SoD enforcement `maker_id ≠ approver_id` di service layer — tidak hanya DB constraint
- [ ] `PENJUALAN.CREATED`, `PENJUALAN.APPROVED`, `PENJUALAN.POSTED`, `PENJUALAN.REJECTED` ditulis in-transaction
- [ ] `PENJUALAN.OCI_RECYCLED` / `PENJUALAN.OCI_NO_RECYCLE` in-transaction bersama PENJUALAN.POSTED
- [ ] `PENJUALAN.BM_FREQUENCY_FLAG` in-transaction
- [ ] `PENJUALAN.DERECOGNIZED` in-transaction — termasuk partial qty update
- [ ] Idempotency-Key cek di approve endpoint — mencegah double-approve yang dispose instrumen duplikat
- [ ] Rate limit approve endpoint: 10 req/menit (sensitif per api-conventions.md)
- [ ] Export `GET /trx/penjualan/export` — audit `PENJUALAN.EXPORT` in-transaction; ROLE-AUDIT read-only

---

_Story set ini siap dihandoff ke `system-analyst` untuk OpenAPI contract + state machine `trx.penjualan.status` (termasuk state `PENDING_BM_REVIEW` baru), ke `ifrs9-compliance-reviewer` untuk review S3 (OCI recycling BLOCKING), S4 (BM Test BLOCKING), dan S5 (jurnal multi-leg BLOCKING), dan ke `security-engineer` untuk review S2 (SoD + audit BLOCKING). `data-modeler` memulai migration paralel setelah compliance gate S3 cleared._
