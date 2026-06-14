# P4-M4 — Look-through ECL untuk Reksadana: User Stories

**Story Set ID**: P4-M4
**Modul**: APP-C — ECL Engine (Phase 4, Sprint 2)
**Status**: DRAFT — menunggu review `ifrs9-compliance-reviewer` (BLOCKING gate)
**Author**: business-analyst
**Tanggal**: 2026-06-11
**Linked FSD**: FSD-APP-C-ECL-EIR-v1.0.docx §3 (Look-through ECL), §4 (EAD)
**Linked BRD**: BRD §8.3 (ECL Requirements — Reksadana look-through), RACI: ROLE-AKUN (R/input), ROLE-RISK (R/review), ROLE-ALCO (A/approve), ROLE-AUDIT (I)
**Linked Decision Log**:
- DEC-015 — Look-through ECL untuk Reksadana: decompose by underlying asset class
- DEC-010 — ECL formula 3-stage × 3-skenario × dual FL multiplier; Reksadana ECL tidak dihitung di level fund
- DEC-016 — NUMERIC precision: IDR `NUMERIC(20,4)`, PD/LGD `NUMERIC(10,8)`
- DEC-017 — Workflow 6-eyes untuk parameter master yang mempengaruhi ECL (fund composition = sensitif, apply 6-eyes)
- DEC-018 — Audit trail append-only; `ecl.*` no hard delete

**Depends on**: P4-M1 (staging engine), P4-M2 (PD/LGD/EAD helpers), P4-M3 (LPS aggregator — Reksadana bukan LPS scope, tapi engine wiring sama), Phase 3 (instrumen REKSADANA, counterparty MI, NAB feed KSEI/MI)

**Handoff berikutnya**:
- `system-analyst` — Go interface contract `LookThroughService` + endpoint contract (OpenAPI)
- `data-modeler` — migration 000024: `mst.fund_composition` tabel baru + `ecl.lookthrough_underlying` schema-fix (precision + audit cols + skenario columns)
- `ecl-eir-engineer` — implementasi domain logic `backend/internal/ecl/lookthrough/`

---

## Konteks & Dependensi

### Skema yang dikonsumsi

| Tabel | Kolom Kunci | Catatan |
|---|---|---|
| `mst.instrumen` | `tipe_instrumen = 'REKSADANA'`, `id`, `nama`, `counterparty_id`, `nominal_nab_idr NUMERIC(20,4)`, `status = 'AKTIF'`, `is_deleted = FALSE`, `workflow_status = 'APPROVED'` | Existing Phase 3. `nominal_nab_idr` dipakai sebagai proxy NAB jika feed harian belum tersedia. |
| `mst.counterparty` | `id`, `tipe = 'MI'` (Manajer Investasi), `nama` | Existing Phase 3. |
| **`mst.fund_composition`** | `instrumen_id`, `asset_class`, `weight_pct`, `effective_from`, `effective_to`, `workflow_status`, `source_doc_id` | **TABEL BARU** — lihat DDL Gap di bawah. Belum ada di init schema 0001. OQ-C dari plan. |
| `mst.nab_harian` atau `mst.instrumen.nominal_nab_idr` | NAB terbaru dari feed KSEI/MI | Perlu konfirmasi tabel NAB — lihat OQ-M4-3. |
| `mst.pd_pefindo` | `rating`, `stage`, `pd_12m NUMERIC(10,8)`, `pd_lifetime NUMERIC(10,8)` | P4-M2 helpers: `LookupPD(rating, stage, periodeID)` |
| `mst.lgd_basel` | `tipe_eksposur`, `lgd NUMERIC(10,8)` | P4-M2 helpers: `LookupLGD(tipeEksposur, periodeID)` |
| `mst.bobot_skenario` | `bobot_good`, `bobot_normal`, `bobot_bad` | P4-M2: default 0.25/0.50/0.25 (DEC-010), ALCO dapat override |
| `mst.impact_mev_pd` | `skenario`, `impact_multiplier` | P4-M2: dual FL multiplier per skenario |
| `ecl.lookthrough_underlying` | `ecl_calc_header_id`, `underlying_kategori`, `weight`, `ead_underlying_idr`, `pd_normal`, `lgd`, `ecl_weighted_idr` | Existing (init schema 0001) — **precision GAP**: NUMERIC(20,2) untuk IDR (seharusnya NUMERIC(20,4)) dan NUMERIC(8,4) untuk PD/LGD (seharusnya NUMERIC(10,8)). Lihat DDL Gap di bawah. |
| `ecl.calc_header` | `instrumen_id`, `ecl_fl_idr`, `ead_idr`, `calc_run_id` | Existing. Look-through menulis ke `ecl.calc_header.ecl_fl_idr` + detail ke `ecl.lookthrough_underlying`. |
| `doc.upload` | `id`, `file_name`, `mime_type` | Untuk `source_doc_id` fund composition (fund fact sheet). |

### DDL Gap 1 — `mst.fund_composition` belum ada (OQ-C CONFIRMED)

**Severity: HIGH — BLOCKER untuk P4-M4**

Tabel `mst.fund_composition` tidak ada di init schema 0001. Migration 000024 **wajib** menciptakan tabel ini sebelum `LookThroughService` dapat diimplementasi. Data seed juga diperlukan (minimal 1 Reksadana aktif dengan komposisi valid) agar integration test dapat berjalan.

DDL yang direkomendasikan untuk `data-modeler` (scope migration 000024):

```sql
CREATE TABLE mst.fund_composition (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    instrumen_id        UUID        NOT NULL REFERENCES mst.instrumen(id) ON DELETE RESTRICT,
    asset_class         TEXT        NOT NULL
                            CONSTRAINT chk_fc_asset_class
                            CHECK (asset_class IN ('GOVT_BOND','CORP_BOND','CASH','EQUITY','OTHER')),
    weight_pct          NUMERIC(7,4) NOT NULL  -- mis. 60.0000 = 60%, maks 100
                            CONSTRAINT chk_fc_weight_positive CHECK (weight_pct > 0),
    effective_from      DATE        NOT NULL,
    effective_to        DATE,                  -- NULL = masih aktif
    source_doc_id       UUID        REFERENCES doc.upload(id),
    catatan             TEXT,
    -- 6-eyes workflow
    workflow_status     TEXT        NOT NULL DEFAULT 'DRAFT'
                            CONSTRAINT chk_fc_workflow_status
                            CHECK (workflow_status IN (
                                'DRAFT','PENDING_REVIEW','PENDING_APPROVAL',
                                'APPROVED','REJECTED','SUPERSEDED'
                            )),
    maker_id            UUID        NOT NULL REFERENCES sec.user(id),
    reviewer_id         UUID        REFERENCES sec.user(id),
    approver_id         UUID        REFERENCES sec.user(id),
    signed_at_review    TIMESTAMPTZ,
    signature_hash_review BYTEA,
    signed_at_approve   TIMESTAMPTZ,
    signature_hash_approve BYTEA,
    CONSTRAINT chk_fc_sod_rev   CHECK (maker_id <> reviewer_id    OR reviewer_id IS NULL),
    CONSTRAINT chk_fc_sod_appr  CHECK (maker_id <> approver_id    OR approver_id IS NULL),
    CONSTRAINT chk_fc_sod_rev_appr CHECK (reviewer_id <> approver_id OR approver_id IS NULL),
    -- Audit cols wajib (db-conventions.md)
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by  UUID        NOT NULL REFERENCES sec.user(id),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by  UUID        NOT NULL REFERENCES sec.user(id),
    deleted_at  TIMESTAMPTZ,
    deleted_by  UUID        REFERENCES sec.user(id),
    row_version BIGINT      NOT NULL DEFAULT 1,
    tenant_id   TEXT        NOT NULL DEFAULT 'TUGURE'
);
-- Constraint sum weight 100% TIDAK bisa di DB CHECK (multi-row constraint).
-- Enforced di application layer — lihat Story 1 AC Scenario 3.
-- Index: active composition per instrumen per effective date
CREATE UNIQUE INDEX uq_fc_instrumen_effective
    ON mst.fund_composition(instrumen_id, effective_from, asset_class)
    WHERE deleted_at IS NULL AND workflow_status = 'APPROVED';
CREATE INDEX idx_fc_instrumen_status
    ON mst.fund_composition(instrumen_id, workflow_status)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_fc_active_approved
    ON mst.fund_composition(instrumen_id, effective_from, effective_to)
    WHERE workflow_status = 'APPROVED' AND deleted_at IS NULL;
CREATE INDEX idx_fc_tenant_created
    ON mst.fund_composition(tenant_id, created_at DESC);
```

### DDL Gap 2 — `ecl.lookthrough_underlying` precision mismatch (DEC-016 violation)

**Severity: HIGH**

Init schema 0001 mendefinisikan:
- `weight NUMERIC(8,4)` — cukup (persentase)
- `ead_underlying_idr NUMERIC(20,2)` — **HARUS `NUMERIC(20,4)`** per DEC-016
- `pd_normal NUMERIC(8,4)` — **HARUS `NUMERIC(10,8)`** per DEC-016
- `lgd NUMERIC(8,4)` — **HARUS `NUMERIC(10,8)`** per DEC-016
- `ecl_weighted_idr NUMERIC(20,2)` — **HARUS `NUMERIC(20,4)`** per DEC-016
- Tidak ada kolom: `fund_composition_id` (FK reference ke versi komposisi), audit cols, skenario breakdown (GOOD/NORMAL/BAD individual values)

Migration 000024 **wajib** melakukan ALTER TABLE untuk precision fix + ADD COLUMN untuk tambahan kolom.

**Flag ke `data-modeler`**: Pastikan migration 000024 meng-cover kedua DDL gap di atas.

---

## Asset Class Enum (mst.fund_composition.asset_class)

| Kode | Deskripsi | PD/LGD Reference |
|---|---|---|
| `GOVT_BOND` | Obligasi Pemerintah (SBN, SPN, ORI, sukuk negara) | PD rendah (government); LGD dari `mst.lgd_basel` tipe 'SOVEREIGN' |
| `CORP_BOND` | Obligasi Korporasi | PD dari `mst.pd_pefindo` sesuai rating issuer underlying; LGD 'UNSECURED_CORP' |
| `CASH` | Kas, deposito, pasar uang | PD dari `mst.pd_pefindo` (bank counterparty); LGD 'DEPOSITO' |
| `EQUITY` | Saham | PD dari sektor; LGD 'EQUITY' |
| `OTHER` | Aset lain tidak terkategori | PD/LGD conservative default; di-flag untuk manual review |

**Catatan**: PD/LGD mapping per asset class ke `mst.pd_pefindo` + `mst.lgd_basel` perlu dikonfirmasi oleh `ifrs9-compliance-reviewer` dan `ecl-eir-engineer`. Lihat OQ-M4-4.

---

## Open Questions (M4-spesifik)

| ID | Pertanyaan | Asumsi Default | Perlu Konfirmasi |
|---|---|---|---|
| OQ-M4-1 | Apakah fund composition diinput bulanan atau triwulanan? Fund fact sheet MI biasanya terbit bulanan tetapi beberapa hanya triwulanan. | Triwulanan (quarterly) adalah minimum; ROLE-AKUN wajib update saat fact sheet baru terbit. Tidak ada auto-expiry per bulan jika tidak di-update. | FSD-APP-C + business user (Treasury/Akuntansi) |
| OQ-M4-2 | NAB (Nilai Aktiva Bersih) harian: apakah tersimpan di tabel terpisah `mst.nab_harian` atau sebagai kolom di `mst.instrumen.nominal_nab_idr`? Phase 3 menyebut "NAB harian feed KSEI/MI" tetapi tabel spesifiknya belum dikonfirmasi. | Gunakan `mst.instrumen.nominal_nab_idr` (kolom yang ada di init schema) sebagai source NAB untuk evaluationDate. Jika ada tabel `mst.nab_harian` yang berbeda, ECL engine harus lookup ke sana. | `system-analyst` + `data-modeler` — cek schema Phase 3 |
| OQ-M4-3 | Reksadana FVTPL (paling umum): apakah look-through ECL tetap dihitung? Per OQ-G di plan, FVTPL skip ECL. Tapi Reksadana bisa juga diklasifikasi AC atau FVOCI (jarang). | Hanya Reksadana dengan `klasifikasi_psak71 IN ('AC','FVOCI')` yang masuk look-through ECL. `FVTPL` di-skip (ECL = 0). Konsisten OQ-G plan. | ifrs9-compliance-reviewer — BLOCKING untuk ini |
| OQ-M4-4 | PD/LGD per asset class: apakah ada mapping eksplisit di FSD-APP-C (asset_class → rating bucket → PD)? `GOVT_BOND` tidak punya rating Pefindo (sovereign). | `GOVT_BOND` → PD = 0 (sovereign, IDR-denominated; BI/Pemerintah RI tidak ada default). `CORP_BOND` → PD dari `mst.pd_pefindo` (rating underlying portfolio average atau conservative worst). `CASH` → PD dari bank counterparty. `EQUITY` → PD dari sektor average. `OTHER` → PD conservative (sektor 'C' Pefindo atau highest default bucket). | ifrs9-compliance-reviewer + ecl-eir-engineer — BLOCKING |
| OQ-M4-5 | Jika Reksadana POCI (rare): apakah look-through tetap berlaku atau di-defer ke M7 POCI handling? | POCI Reksadana di-defer. Story 2 mencantumkan error `LOOKTHROUGH_POCI_DEFERRED` jika instrumen POCI. | Plan §1 Exclusions: POCI excluded Phase 4 |
| OQ-M4-6 | Apakah weight_pct per asset class harus integer atau boleh desimal (mis. 33.33%)? Constraint sum = 100% ± tolerance: berapa toleransi yang diterima? | Toleransi ± 0.01% (±0.0001 dalam satuan desimal) untuk floating-point precision di input UI. Misalnya 33.33 + 33.33 + 33.34 = 100.00 diterima. | business user + ifrs9-compliance-reviewer |
| OQ-M4-7 | Migration 000024 numbering: Plan fase 4 menyebut M4 = migration 000023. Namun 000023 sudah diambil oleh P4-M3 LPS exclusion override. Konfirmasi nomor migrasi M4 = **000024**. | Nomor migration untuk P4-M4 adalah **000024**. Plan perlu di-update (lihat catatan di §9 plan). | `data-modeler` + `tech-lead-orchestrator` |

---

## Story APP-C-LKT-001 — Maintain Fund Composition per Reksadana

**Actor**: ROLE-AKUN (maker) → ROLE-RISK (reviewer) → ROLE-ALCO (approver)
**Trigger**: ROLE-AKUN menerima fund fact sheet baru dari Manajer Investasi (MI) dan perlu menginput atau memperbarui komposisi underlying untuk instrumen Reksadana tertentu.
**Goal**: Menyimpan komposisi underlying aset (breakdown per asset class dengan bobot persentase) untuk instrumen Reksadana di `mst.fund_composition`, melalui workflow 6-eyes karena data ini secara langsung mempengaruhi perhitungan ECL.

**Pre-conditions**:
- Instrumen Reksadana sudah ada di `mst.instrumen` dengan `tipe_instrumen = 'REKSADANA'`, `status = 'AKTIF'`, `workflow_status = 'APPROVED'`
- ROLE-AKUN user login dengan permission `fund_composition.create`
- `mst.fund_composition` tabel sudah ada (migration 000024 selesai)
- Tidak ada komposisi aktif (`workflow_status = 'APPROVED'`) yang overlap untuk instrumen + tanggal yang sama (atau Story 5 amend flow sudah dilalui terlebih dahulu)

**Workflow**: 6-eyes — Maker (ROLE-AKUN) → Reviewer (ROLE-RISK) → Approver (ROLE-ALCO).
SoD: `maker_id ≠ reviewer_id ≠ approver_id`.

**Permissions**:
- `fund_composition.create` — ROLE-AKUN (submit)
- `fund_composition.review` — ROLE-RISK (sign review)
- `fund_composition.approve` — ROLE-ALCO (sign approve)
- `fund_composition.read` — ROLE-AKUN, ROLE-RISK, ROLE-ALCO, ROLE-AUDIT

**Audit events**:
- `FUND_COMPOSITION.SUBMIT` — saat ROLE-AKUN submit
- `FUND_COMPOSITION.REVIEW` — saat ROLE-RISK sign review
- `FUND_COMPOSITION.APPROVE` — saat ROLE-ALCO sign approve
- `FUND_COMPOSITION.REJECT` — saat ROLE-RISK atau ROLE-ALCO reject

---

### AC Gherkin — APP-C-LKT-001

**Scenario 1 (Happy Path — Submit + Review + Approve komposisi baru)**

```gherkin
GIVEN ROLE-AKUN user Siti login (permission fund_composition.create)
  AND instrumen "RKD-PENDAPATAN-TETAP-001" (tipe_instrumen = REKSADANA, status = AKTIF)
  AND tidak ada mst.fund_composition APPROVED untuk instrumen ini pada periode yang overlapping
WHEN Siti POST /api/v1/master/fund-compositions dengan:
  {
    "instrumen_id": "RKD-PENDAPATAN-TETAP-001",
    "effective_from": "2026-04-01",
    "source_doc_id": "DOC-001",
    "lines": [
      { "asset_class": "GOVT_BOND",  "weight_pct": 60.0000 },
      { "asset_class": "CORP_BOND",  "weight_pct": 30.0000 },
      { "asset_class": "CASH",       "weight_pct": 10.0000 }
    ]
  }
THEN response 201:
    { "data": { "composition_id": "FC-001", "workflow_status": "PENDING_REVIEW", "total_weight_pct": 100.0000 } }
  AND toast: "Komposisi underlying RKD-PENDAPATAN-TETAP-001 berhasil disubmit. Menunggu review ROLE-RISK."
  AND 3 rows di mst.fund_composition (satu per line) dengan workflow_status = 'DRAFT' (dalam satu composition group)
  AND audit log: action = 'FUND_COMPOSITION.SUBMIT', entity_id = "FC-001", actor = Siti

GIVEN ROLE-RISK user Budi login (permission fund_composition.review, Budi ≠ Siti)
WHEN Budi POST /api/v1/master/fund-compositions/FC-001/review dengan:
  { "comment": "Sesuai fund fact sheet Maret 2026" }
THEN response 200: { "workflow_status": "PENDING_APPROVAL" }
  AND audit log: action = 'FUND_COMPOSITION.REVIEW', signed_at_review populated, signature_hash_review populated

GIVEN ROLE-ALCO user Diana login (permission fund_composition.approve, Diana ≠ Siti ≠ Budi)
  AND Diana memiliki mfa_verified = true (MFA wajib untuk ROLE-ALCO)
WHEN Diana POST /api/v1/master/fund-compositions/FC-001/approve dengan:
  { "comment": "Disetujui" }
THEN response 200: { "workflow_status": "APPROVED", "approved_at": "2026-06-11T..." }
  AND semua 3 lines mst.fund_composition untuk FC-001 di-update ke workflow_status = 'APPROVED'
  AND signed_at_approve + signature_hash_approve populated di setiap row
  AND audit log: action = 'FUND_COMPOSITION.APPROVE', actor = Diana
  AND ECL calc run berikutnya dengan evaluationDate ≥ 2026-04-01 dapat menggunakan komposisi FC-001
```

**Scenario 2 (Error — sum weight ≠ 100%, di luar toleransi)**

```gherkin
GIVEN ROLE-AKUN user Siti login
WHEN Siti POST /api/v1/master/fund-compositions dengan lines:
  [
    { "asset_class": "GOVT_BOND", "weight_pct": 60.0000 },
    { "asset_class": "CORP_BOND", "weight_pct": 25.0000 }
    -- total = 85%, bukan 100%
  ]
THEN response 422:
    code: "VALIDATION_FAILED"
    details: [{ field: "lines", rule: "weight_sum_100pct",
                message: "Total weight_pct harus 100% ± 0.01%. Saat ini: 85.0000%" }]
  AND tidak ada row yang dibuat di mst.fund_composition
```

**Scenario 3 (Error — asset_class tidak valid)**

```gherkin
GIVEN ROLE-AKUN user Siti submit dengan:
  { "asset_class": "CRYPTO", "weight_pct": 100.0000 }
WHEN request diproses
THEN response 422:
    code: "VALIDATION_FAILED"
    details: [{ field: "lines[0].asset_class", rule: "enum",
                message: "Asset class CRYPTO tidak valid. Nilai yang diterima: GOVT_BOND, CORP_BOND, CASH, EQUITY, OTHER" }]
```

**Scenario 4 (Error — instrumen bukan REKSADANA)**

```gherkin
GIVEN instrumen "DEPOSITO-001" (tipe_instrumen = 'DEPOSITO')
WHEN ROLE-AKUN submit fund composition untuk DEPOSITO-001
THEN response 422:
    code: "VALIDATION_FAILED"
    details: [{ field: "instrumen_id", rule: "tipe_instrumen_reksadana",
                message: "Fund composition hanya berlaku untuk instrumen tipe REKSADANA. Instrumen DEPOSITO-001 bertipe DEPOSITO." }]
```

**Scenario 5 (SoD — reviewer = maker)**

```gherkin
GIVEN FC-002 dalam status PENDING_REVIEW, maker = Siti
  AND Siti mencoba melakukan review pada FC-002 (role ROLE-AKUN yang juga punya akses review — mis. dual role)
WHEN Siti POST /api/v1/master/fund-compositions/FC-002/review
THEN response 403:
    code: "SOD_VIOLATION"
    message: "Maker tidak dapat menjadi reviewer. maker_id = reviewer_id untuk FC-002."
```

**Scenario 6 (Error — composition sudah ada aktif untuk periode overlapping)**

```gherkin
GIVEN FC-001 sudah APPROVED untuk instrumen RKD-001, effective_from = 2026-04-01, effective_to = NULL (aktif)
WHEN ROLE-AKUN submit composition baru untuk RKD-001 dengan effective_from = 2026-05-01
  -- tanpa terlebih dahulu menutup FC-001 via amend flow (Story 5)
THEN response 409:
    code: "CONFLICT"
    message: "Instrumen RKD-001 sudah memiliki fund composition APPROVED yang aktif (FC-001, effective_from 2026-04-01). Gunakan fitur Amend Composition untuk membuat versi baru."
```

---

## Story APP-C-LKT-002 — Compute Look-through ECL per Reksadana

**Actor**: ECL calc engine (Asynq worker — `internal/ecl/lookthrough`), dipanggil oleh P4-M7 ECL core engine
**Trigger**: P4-M7 ECL engine menemukan instrumen dengan `tipe_instrumen = 'REKSADANA'` dalam scope calc run. Alih-alih menjalankan ECL standard helper (P4-M2), engine mendelegasi ke `LookThroughService.Compute()`.
**Goal**: Menghitung ECL Reksadana via decomposisi underlying: untuk setiap asset class dalam komposisi aktif, hitung `ECL_class = NAB_portion × PD_class × LGD_class` per skenario, lalu sum weighted. Return breakdown per asset class + total ECL untuk disimpan di `ecl.calc_header` dan `ecl.lookthrough_underlying`.

**Pre-conditions**:
- Instrumen memiliki `tipe_instrumen = 'REKSADANA'`, `klasifikasi_psak71 IN ('AC','FVOCI')` (skip FVTPL per OQ-M4-3)
- `mst.fund_composition` memiliki minimal satu set APPROVED yang berlaku pada `evaluationDate` (`effective_from ≤ evaluationDate AND (effective_to IS NULL OR effective_to ≥ evaluationDate)`)
- NAB tersedia (dari `mst.instrumen.nominal_nab_idr` atau `mst.nab_harian` — per OQ-M4-2)
- P4-M2 `LookupPD()` dan `LookupLGD()` tersedia dan approved parameter untuk `periodeID`
- `mst.bobot_skenario` dan `mst.impact_mev_pd` tersedia untuk `periodeID`

**Permissions**: `lookthrough.compute` (internal engine — tidak ada HTTP endpoint publik; dipanggil Go-to-Go)

**Audit events**: Tidak menulis `aud.audit_log` langsung. P4-M8 menulis saat calc run disimpan/diseal. `ecl.lookthrough_underlying` rows bersifat immutable setelah calc run sealed.

**Output struct**:
```go
type LookThroughResult struct {
    InstrumenID           uuid.UUID
    FundCompositionID     uuid.UUID          // FK ke mst.fund_composition version yang dipakai
    NAB_IDR               decimal.Decimal
    TotalECL_IDR          decimal.Decimal    // weighted aggregate
    Breakdown             []UnderlyingLine
}

type UnderlyingLine struct {
    AssetClass            string             // GOVT_BOND / CORP_BOND / CASH / EQUITY / OTHER
    WeightPct             decimal.Decimal    // dari fund composition
    NAB_Portion_IDR       decimal.Decimal    // NAB × weight_pct / 100
    PD_Good               decimal.Decimal
    PD_Normal             decimal.Decimal
    PD_Bad                decimal.Decimal
    LGD                   decimal.Decimal
    ECL_Skenario_Good     decimal.Decimal
    ECL_Skenario_Normal   decimal.Decimal
    ECL_Skenario_Bad      decimal.Decimal
    ECL_FL_Good           decimal.Decimal    // setelah FL multiplier
    ECL_FL_Normal         decimal.Decimal
    ECL_FL_Bad            decimal.Decimal
    ECL_Weighted_IDR      decimal.Decimal    // Σ(ECL_FL × bobot)
}
```

---

### AC Gherkin — APP-C-LKT-002

**Scenario 1 (Happy Path — 3 asset class, komputasi lengkap)**

```gherkin
GIVEN instrumen "RKD-001" (tipe = REKSADANA, klasifikasi = FVOCI)
  AND fund composition APPROVED (FC-001) berlaku per evaluationDate 2026-06-30:
    GOVT_BOND 60%, CORP_BOND 30%, CASH 10%
  AND NAB_IDR = 5.000.000.000,0000 (IDR 5 miliar)
  AND periodeID = "JUNI-2026"
  AND PD/LGD per asset class (dari P4-M2 lookup, skenario NORMAL):
    GOVT_BOND: PD_Normal = 0,00000000 (sovereign), LGD = 0,45000000
    CORP_BOND: PD_Normal = 0,02500000 (rating A), LGD = 0,45000000
    CASH:      PD_Normal = 0,01000000 (bank AA), LGD = 0,45000000
  AND FL multiplier (NORMAL) = 1,00000000; (GOOD) = 0,80000000; (BAD) = 1,50000000
  AND bobot: GOOD=0,25, NORMAL=0,50, BAD=0,25
WHEN LookThroughService.Compute(ctx, instrumenID, evaluationDate, periodeID) dipanggil
THEN:
  -- Porsi per asset class
  NAB_GOVT = 5.000.000.000 × 0,60 = 3.000.000.000,0000
  NAB_CORP = 5.000.000.000 × 0,30 = 1.500.000.000,0000
  NAB_CASH = 5.000.000.000 × 0,10 =   500.000.000,0000

  -- ECL per skenario per asset class (NORMAL sebagai contoh)
  ECL_GOVT_Normal = 3.000.000.000 × 0,00000000 × 0,45000000 = 0,0000
  ECL_CORP_Normal = 1.500.000.000 × 0,02500000 × 0,45000000 = 16.875.000,0000
  ECL_CASH_Normal =   500.000.000 × 0,01000000 × 0,45000000 =  2.250.000,0000

  -- ECL per asset class weighted (simplified dengan FL=1 untuk NORMAL)
  ECL_GOVT_Weighted = 0 × 0,50 + ... = (tergantung Good/Bad calc)
  ECL_CORP_Weighted = (ECL_CORP_Good × 0,25 + ECL_CORP_Normal × 0,50 + ECL_CORP_Bad × 0,25)
  ECL_CASH_Weighted = (ECL_CASH_Good × 0,25 + ECL_CASH_Normal × 0,50 + ECL_CASH_Bad × 0,25)

  -- Total
  TotalECL_IDR = Σ(ECL_GOVT_Weighted + ECL_CORP_Weighted + ECL_CASH_Weighted)

  AND result.FundCompositionID = FC-001 (versi yang aktif pada evaluationDate)
  AND result.Breakdown berisi 3 baris (satu per asset class) dengan semua nilai terisi
  AND semua nilai menggunakan shopspring/decimal (no float64) — presisi NUMERIC(10,8) untuk PD/LGD, NUMERIC(20,4) untuk IDR amount
  AND 3 rows di ecl.lookthrough_underlying dibuat (satu per asset class) dengan FK ke ecl.calc_header
```

**Scenario 2 (Error — fund composition missing)**

```gherkin
GIVEN instrumen "RKD-002" (tipe = REKSADANA, klasifikasi = AC)
  AND tidak ada mst.fund_composition dengan workflow_status = 'APPROVED'
      yang berlaku pada evaluationDate = 2026-06-30
WHEN LookThroughService.Compute(ctx, "RKD-002", 2026-06-30, periodeID) dipanggil
THEN error dikembalikan:
    code: "LOOKTHROUGH_FUND_COMPOSITION_MISSING"
    message: "Tidak ditemukan fund composition APPROVED untuk instrumen RKD-002 per tanggal 2026-06-30."
  AND calc run job untuk periode ini GAGAL dengan status 'failed'
  AND sys.job.error_jsonb mencantumkan: { code, instrumen_id, evaluation_date }
  AND ROLE-RISK mendapat notifikasi: "ECL calc run gagal — fund composition RKD-002 belum tersedia. Silakan input via menu Fund Composition."
```

**Scenario 3 (Error — NAB missing)**

```gherkin
GIVEN instrumen "RKD-003" (tipe = REKSADANA, klasifikasi = AC)
  AND fund composition APPROVED tersedia untuk evaluationDate
  AND mst.instrumen.nominal_nab_idr IS NULL (belum pernah di-update, atau feed KSEI/MI gagal)
WHEN LookThroughService.Compute(ctx, "RKD-003", 2026-06-30, periodeID) dipanggil
THEN error dikembalikan:
    code: "LOOKTHROUGH_NAB_MISSING"
    message: "NAB untuk instrumen RKD-003 tidak tersedia per 2026-06-30. Pastikan feed NAB harian KSEI/MI telah diupload."
  AND calc run GAGAL
```

**Scenario 4 (Error — weight sum tidak valid saat runtime — defensive check)**

```gherkin
GIVEN fund composition FC-001 ter-approve dengan lines:
  GOVT_BOND 60%, CORP_BOND 25% (total = 85% — anomali, seharusnya tidak terjadi jika Story 1 AC bekerja)
  [Ini skenario defensive: misal data legacy atau bypass via DB direct write]
WHEN LookThroughService.Compute meload FC-001 dan memvalidasi sum weight
THEN error dikembalikan:
    code: "LOOKTHROUGH_WEIGHT_INVALID"
    message: "Fund composition FC-001 memiliki total weight 85.0000% (expected 100% ± 0.01%). Data integrity issue — hubungi IT Admin."
  AND calc run GAGAL
  AND log ERROR level di backend: "FUND_COMPOSITION_INTEGRITY_VIOLATION fc_id=FC-001 total=85.0000"
```

**Scenario 5 (Edge — Reksadana FVTPL: skip, ECL = 0)**

```gherkin
GIVEN instrumen "RKD-004" (tipe = REKSADANA, klasifikasi_psak71 = 'FVTPL')
WHEN P4-M7 ECL engine menemukan RKD-004 dalam scope calc run
THEN LookThroughService TIDAK dipanggil
  AND ecl.calc_header untuk RKD-004: ecl_fl_idr = 0,0000, catatan = "FVTPL_SKIP_ECL"
  AND tidak ada rows di ecl.lookthrough_underlying untuk RKD-004
```

**Scenario 6 (Edge — Reksadana POCI: defer)**

```gherkin
GIVEN instrumen "RKD-005" (tipe = REKSADANA, poci_flag = TRUE)
WHEN LookThroughService.Compute dipanggil untuk RKD-005
THEN error dikembalikan:
    code: "LOOKTHROUGH_POCI_DEFERRED"
    message: "Instrumen RKD-005 adalah POCI. Look-through ECL untuk POCI Reksadana di-defer ke Phase 5. Instrument di-skip dari calc run ini."
  AND ecl.calc_header untuk RKD-005: ecl_fl_idr = NULL, catatan = "POCI_DEFERRED"
  AND calc run TIDAK gagal — RKD-005 di-skip dengan flag, proses instrumen lain lanjut
```

---

## Story APP-C-LKT-003 — Preview Look-through Breakdown per Reksadana

**Actor**: ROLE-RISK
**Trigger**: ROLE-RISK membuka halaman detail instrumen Reksadana di UI untuk melihat komposisi underlying dan estimasi ECL sebelum atau sesudah calc run. Atau mengakses panel preview dari halaman ECL Run Detail.
**Goal**: Menampilkan breakdown komposisi underlying per Reksadana beserta estimasi ECL per asset class, tanpa harus menjalankan full calc run. Memberikan visibility kepada ROLE-RISK untuk verifikasi sebelum mengapprove ECL parameter atau setelah review hasil calc run.

**Pre-conditions**:
- ROLE-RISK user login dengan permission `lookthrough.preview`
- Instrumen target adalah REKSADANA dengan komposisi APPROVED minimal satu
- P4-M2 PD/LGD helpers tersedia (digunakan untuk preview kalkulasi on-the-fly)

**Permissions**: `lookthrough.preview` — ROLE-RISK, ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT (read-only)

**Audit events**: `LOOKTHROUGH.PREVIEW` ditulis ke `aud.audit_log` setiap panggilan. `LOOKTHROUGH.EXPORT` untuk export.

---

### AC Gherkin — APP-C-LKT-003

**Scenario 1 (Happy Path — Lihat breakdown komposisi + estimasi ECL)**

```gherkin
GIVEN ROLE-RISK user Budi login (permission lookthrough.preview)
  AND instrumen "RKD-001" memiliki fund composition APPROVED (FC-001)
  AND evaluationDate = 2026-06-30
WHEN Budi GET /api/v1/ecl/lookthrough/preview/{instrumen_id}?evaluation_date=2026-06-30
THEN response 200:
  {
    "data": {
      "instrumen_id": "RKD-001",
      "instrumen_nama": "Reksa Dana Pendapatan Tetap XYZ",
      "nab_idr": 5000000000.0000,
      "fund_composition_id": "FC-001",
      "fund_composition_effective_from": "2026-04-01",
      "breakdown": [
        {
          "asset_class": "GOVT_BOND",
          "weight_pct": 60.0000,
          "nab_portion_idr": 3000000000.0000,
          "pd_normal": 0.00000000,
          "lgd": 0.45000000,
          "ecl_weighted_idr": 0.0000
        },
        {
          "asset_class": "CORP_BOND",
          "weight_pct": 30.0000,
          "nab_portion_idr": 1500000000.0000,
          "pd_normal": 0.02500000,
          "lgd": 0.45000000,
          "ecl_weighted_idr": 15937500.0000   -- approximate (tergantung FL + bobot actual)
        },
        {
          "asset_class": "CASH",
          "weight_pct": 10.0000,
          "nab_portion_idr": 500000000.0000,
          "pd_normal": 0.01000000,
          "lgd": 0.45000000,
          "ecl_weighted_idr": 2025000.0000    -- approximate
        }
      ],
      "total_ecl_estimate_idr": 17962500.0000,
      "is_preview": true,
      "note": "Estimasi menggunakan parameter ECL aktif. Bukan hasil calc run resmi."
    }
  }
  AND DataTable breakdown mendukung: sort per kolom, filter per asset_class, export CSV/XLSX
  AND audit log: action = 'LOOKTHROUGH.PREVIEW', entity_id = RKD-001, actor = Budi

THEN UI menampilkan:
  - Card ringkasan: NAB IDR, Total ECL Estimasi, Jumlah Asset Class, Versi Komposisi
  - Pie chart komposisi (weight_pct per asset_class)
  - DataTable breakdown per asset class (sort+filter+export sesuai UX §1)
  - Badge "Estimasi — Bukan hasil calc run resmi"
```

**Scenario 2 (Drill-down — lihat ECL per skenario per asset class)**

```gherkin
GIVEN Budi sudah di halaman preview RKD-001
WHEN Budi klik expand row "CORP_BOND"
THEN tampilkan sub-baris:
    - ECL Skenario GOOD:   nab_portion × PD_Good × LGD × FL_Good
    - ECL Skenario NORMAL: nab_portion × PD_Normal × LGD × FL_Normal
    - ECL Skenario BAD:    nab_portion × PD_Bad × LGD × FL_Bad
    - ECL Weighted:        Σ(ECL_FL × bobot)
  AND angka konsisten dengan Scenario 1 `ecl_weighted_idr` untuk CORP_BOND
```

**Scenario 3 (Error — permission denied)**

```gherkin
GIVEN user ROLE-MAKER-TR login (tidak memiliki lookthrough.preview)
WHEN GET /api/v1/ecl/lookthrough/preview/{instrumen_id}
THEN response 403:
    code: "FORBIDDEN"
    message: "Permission lookthrough.preview tidak terpenuhi. Role ROLE-MAKER-TR tidak memiliki akses."
```

**Scenario 4 (Export — breakdown untuk reporting)**

```gherkin
GIVEN ROLE-RISK user Budi di halaman preview RKD-001
  AND klik Export → CSV
WHEN GET /api/v1/ecl/lookthrough/preview/{instrumen_id}/export?format=csv&evaluation_date=2026-06-30
THEN response: Content-Disposition: attachment; filename="lookthrough-RKD-001-20260630.csv"
  AND CSV berisi semua kolom breakdown + header row Bahasa Indonesia
  AND audit log: action = 'LOOKTHROUGH.EXPORT', entity_id = RKD-001, after_jsonb = {format: "csv", evaluation_date}
```

---

## Story APP-C-LKT-004 — Bulk Look-through untuk Calc Run

**Actor**: ECL calc engine (Asynq worker — `internal/ecl/lookthrough`), dipanggil oleh P4-M7 ECL core engine
**Trigger**: ECL calc run job dimulai untuk suatu periode. Sebelum memasuki loop kalkulasi per instrumen, M7 engine memanggil `LookThroughService.BulkCompute()` untuk semua instrumen REKSADANA aktif dalam scope sekaligus.
**Goal**: Mengembalikan `map[instrumenID]LookThroughResult` untuk semua Reksadana aktif dalam satu panggilan, menghindari N+1 query dan memenuh SLA performa. Konsisten dengan pola BulkAggregate di P4-M3.

**Pre-conditions**:
- Sama dengan Story 2, untuk semua instrumen `tipe_instrumen = 'REKSADANA'` aktif dalam `periodeID`
- Semua fund composition APPROVED sudah tersedia (jika ada instrumen tanpa composition → error per Story 2 Scenario 2, calc run gagal untuk instrumen tersebut)
- NAB tersedia untuk semua Reksadana aktif (via feed harian atau `nominal_nab_idr`)

**Permissions**: `lookthrough.compute` (internal — tidak ada HTTP endpoint publik)

**Performance SLA**: ≤ 2 detik (P95) untuk 500 instrumen Reksadana aktif

**Audit events**: Tidak menulis audit langsung. P4-M8 menulis saat calc run disimpan.

---

### AC Gherkin — APP-C-LKT-004

**Scenario 1 (Happy Path — Bulk compute 500 Reksadana)**

```gherkin
GIVEN periodeID = "JUNI-2026", evaluationDate = 2026-06-30
  AND 500 instrumen tipe REKSADANA aktif dalam scope calc run
  AND semua 500 instrumen memiliki fund composition APPROVED berlaku per evaluationDate
  AND NAB tersedia untuk semua 500 instrumen
  AND parameter PD/LGD/bobot/FL approved untuk periodeID
WHEN LookThroughService.BulkCompute(ctx, periodeID, evaluationDate) dipanggil
THEN dikembalikan map[uuid.UUID]LookThroughResult untuk 500 instrumen
  AND durasi eksekusi ≤ 2 detik (P95, diukur via metrik Prometheus counter `ecl_lookthrough_bulk_duration_seconds`)
  AND TIDAK ada N+1 query: implementasi menggunakan batch JOIN antara mst.instrumen,
      mst.fund_composition, mst.pd_pefindo, mst.lgd_basel, mst.bobot_skenario, mst.impact_mev_pd
  AND setiap LookThroughResult berisi TotalECL_IDR + Breakdown (array asset class)
  AND presisi semua nilai menggunakan shopspring/decimal (no float64)
```

**Scenario 2 (Consistency — BulkCompute vs single Compute: hasil identik)**

```gherkin
GIVEN instrumen RKD-001 dengan FC-001 dan NAB 5 miliar (seperti Story 2 Scenario 1)
WHEN:
  A: BulkCompute dipanggil dengan RKD-001 + instrumen lain
  B: Compute dipanggil individual untuk RKD-001
THEN TotalECL_IDR dari A untuk RKD-001 = TotalECL_IDR dari B (deterministic, no numeric drift)
  AND Breakdown per asset class identik (bit-for-bit sama dengan shopspring/decimal)
```

**Scenario 3 (Partial failure — satu instrumen missing composition)**

```gherkin
GIVEN 500 Reksadana aktif
  AND 1 instrumen (RKD-MISSING) tidak memiliki fund composition APPROVED per evaluationDate
WHEN BulkCompute dipanggil
THEN error dikembalikan:
    code: "LOOKTHROUGH_FUND_COMPOSITION_MISSING"
    message: "1 instrumen tidak memiliki fund composition: RKD-MISSING"
  AND BulkCompute GAGAL (fail fast — tidak partial-complete per policy calc run)
  AND sys.job.status = 'failed', error_jsonb = { missing_compositions: ["RKD-MISSING"] }
  AND ROLE-RISK mendapat notifikasi dengan daftar instrumen yang perlu di-fix
```

**Scenario 4 (Edge — zero Reksadana dalam scope)**

```gherkin
GIVEN periodeID valid tetapi tidak ada instrumen REKSADANA AKTIF dalam scope
    (semua instrumen DEPOSITO / OBLIGASI / SAHAM)
WHEN BulkCompute(ctx, periodeID, evaluationDate) dipanggil
THEN dikembalikan empty map (bukan error)
  AND calc run melanjutkan proses untuk tipe instrumen lain
  AND log info: "LookThrough BulkCompute: 0 REKSADANA instruments in scope for periode [periodeID]"
```

**Scenario 5 (Performance — metrik observability)**

```gherkin
GIVEN BulkCompute selesai untuk 500 instrumen dalam 1,8 detik
WHEN Prometheus scrape dilakukan
THEN metric tersedia:
    ecl_lookthrough_bulk_duration_seconds{percentile="p95"} <= 2.0
    ecl_lookthrough_bulk_instrument_count 500
    ecl_lookthrough_bulk_errors_total 0
  AND Grafana alert TIDAK terpicu (threshold > 2 detik P95)
```

---

## Story APP-C-LKT-005 — Amend Fund Composition (Re-versioning)

**Actor**: ROLE-AKUN (maker amend) → ROLE-RISK (reviewer) → ROLE-ALCO (approver)
**Trigger**: MI menerbitkan fund fact sheet baru (bulanan / triwulanan), atau terdapat perubahan portofolio underlying yang material. ROLE-AKUN perlu mengupdate komposisi dari versi sebelumnya ke versi baru tanpa menghapus data historis.
**Goal**: Membuat versi baru fund composition untuk instrumen Reksadana dengan versioning immutable: versi lama di-tutup (`effective_to` di-set ke `new_effective_from - 1 hari`), versi baru di-insert dengan `effective_from = tanggal_efektif_baru`. Pola ini konsisten dengan `ecl.eir_amortization_schedule` di P4-M5/M6 (schedule versioning).

**Pre-conditions**:
- Instrumen Reksadana memiliki minimal satu fund composition aktif (`workflow_status = 'APPROVED'`, `effective_to IS NULL`)
- ROLE-AKUN memiliki permission `fund_composition.create` (amend pakai endpoint yang sama + flag `is_amendment = true`)
- Komposisi baru lulus validasi sum = 100% ± 0.01%
- Audit trail: versi lama TIDAK di-UPDATE, hanya di-tutup dengan `effective_to`

**Workflow**: 6-eyes — identik dengan Story 1 (Maker → Reviewer → Approver). SoD identik.

**Permissions**: Sama dengan Story 1.

**Audit events**:
- `FUND_COMPOSITION.AMEND_SUBMIT` — submit versi baru
- `FUND_COMPOSITION.AMEND_REVIEW` — ROLE-RISK sign review
- `FUND_COMPOSITION.AMEND_APPROVE` — ROLE-ALCO sign approve (versi lama secara atomik di-tutup, versi baru aktif)

---

### AC Gherkin — APP-C-LKT-005

**Scenario 1 (Happy Path — Amend komposisi: versi lama ditutup, versi baru aktif)**

```gherkin
GIVEN instrumen "RKD-001" memiliki fund composition aktif:
    FC-001: GOVT_BOND 60%, CORP_BOND 30%, CASH 10%
    effective_from = 2026-04-01, effective_to = NULL, workflow_status = 'APPROVED'
WHEN ROLE-AKUN Siti POST /api/v1/master/fund-compositions dengan:
  {
    "instrumen_id": "RKD-001",
    "effective_from": "2026-07-01",
    "is_amendment": true,
    "supersedes_composition_id": "FC-001",
    "source_doc_id": "DOC-002",
    "lines": [
      { "asset_class": "GOVT_BOND",  "weight_pct": 55.0000 },
      { "asset_class": "CORP_BOND",  "weight_pct": 35.0000 },
      { "asset_class": "CASH",       "weight_pct": 10.0000 }
    ]
  }
THEN response 201:
    { "data": { "composition_id": "FC-002", "workflow_status": "PENDING_REVIEW", "supersedes": "FC-001" } }
  AND toast: "Amend komposisi RKD-001 disubmit (versi baru FC-002). Menunggu review ROLE-RISK."

GIVEN ROLE-RISK Budi review dan ROLE-ALCO Diana approve FC-002 (melalui workflow 6-eyes normal)
WHEN FC-002 di-approve (workflow_status → 'APPROVED')
THEN secara ATOMIK dalam satu transaksi DB:
    1. mst.fund_composition rows untuk FC-001: effective_to di-set ke '2026-06-30' (effective_from baru - 1 hari)
       DAN workflow_status FC-001 di-update ke 'SUPERSEDED'
    2. mst.fund_composition rows untuk FC-002: effective_from = '2026-07-01', effective_to = NULL, workflow_status = 'APPROVED'
  AND FC-001 rows TIDAK di-DELETE (audit-grade immutability)
  AND FC-001 tetap bisa di-query dengan GET /api/v1/master/fund-compositions/FC-001 (history view)
  AND audit log: action = 'FUND_COMPOSITION.AMEND_APPROVE', entity_id = FC-002,
      after_jsonb = { supersedes: FC-001, effective_from: 2026-07-01 }

  AND ECL calc run untuk periode-periode:
    - evaluationDate < 2026-07-01: masih menggunakan FC-001 (GOVT_BOND 60%, dll)
    - evaluationDate ≥ 2026-07-01: menggunakan FC-002 (GOVT_BOND 55%, dll)
```

**Scenario 2 (Versioning integrity — tidak ada gap antara versi)**

```gherkin
GIVEN FC-001 ditutup dengan effective_to = 2026-06-30
  AND FC-002 berlaku mulai effective_from = 2026-07-01
WHEN LookThroughService.Compute dipanggil untuk evaluationDate = 2026-06-30
THEN FC-001 digunakan (effective_from=2026-04-01 ≤ 2026-06-30 ≤ effective_to=2026-06-30)
WHEN LookThroughService.Compute dipanggil untuk evaluationDate = 2026-07-01
THEN FC-002 digunakan (effective_from=2026-07-01 ≤ 2026-07-01, effective_to=NULL)
WHEN LookThroughService.Compute dipanggil untuk evaluationDate = 2026-06-15
THEN FC-001 digunakan
```

**Scenario 3 (Error — is_amendment tanpa supersedes_composition_id)**

```gherkin
GIVEN ROLE-AKUN submit dengan is_amendment = true tapi tidak menyertakan supersedes_composition_id
WHEN request diproses
THEN response 422:
    code: "VALIDATION_FAILED"
    details: [{ field: "supersedes_composition_id", rule: "required_when_amendment",
                message: "Saat is_amendment = true, supersedes_composition_id wajib diisi." }]
```

**Scenario 4 (Error — effective_from amend tidak setelah existing effective_from)**

```gherkin
GIVEN FC-001 effective_from = 2026-04-01
WHEN ROLE-AKUN submit amend dengan effective_from = 2026-03-01 (sebelum FC-001)
THEN response 422:
    code: "VALIDATION_FAILED"
    details: [{ field: "effective_from", rule: "must_be_after_superseded_effective_from",
                message: "Tanggal effective_from versi baru (2026-03-01) harus setelah tanggal effective_from versi yang digantikan FC-001 (2026-04-01)." }]
```

**Scenario 5 (Edge — amend saat FC-001 masih PENDING_APPROVAL)**

```gherkin
GIVEN FC-001 dalam status PENDING_REVIEW (belum APPROVED)
WHEN ROLE-AKUN mencoba submit amend dengan supersedes_composition_id = FC-001
THEN response 409:
    code: "CONFLICT"
    message: "Tidak dapat membuat amend untuk FC-001 yang masih dalam proses persetujuan (PENDING_REVIEW). Tunggu hingga FC-001 APPROVED atau REJECTED terlebih dahulu."
```

**Scenario 6 (Audit — history composition dapat di-query)**

```gherkin
GIVEN RKD-001 memiliki history: FC-001 (SUPERSEDED) → FC-002 (APPROVED)
WHEN ROLE-AUDIT GET /api/v1/master/fund-compositions?instrumen_id=RKD-001&include_superseded=true
THEN response 200 berisi 2 composition groups (FC-001 dan FC-002)
  AND FC-001 ditampilkan dengan workflow_status = 'SUPERSEDED', effective_to = '2026-06-30'
  AND FC-002 ditampilkan dengan workflow_status = 'APPROVED', effective_to = NULL
  AND DataTable mendukung sort per effective_from, filter per workflow_status (UX §1)
  AND ROLE-AUDIT dapat export history untuk audit eksternal
```

---

## Ringkasan Data References

| Story | Tabel Read | Tabel Write | Permission |
|---|---|---|---|
| LKT-001 | `mst.instrumen`, `doc.upload` | `mst.fund_composition`, `aud.audit_log` | `fund_composition.create/review/approve` |
| LKT-002 | `mst.fund_composition`, `mst.instrumen`, `mst.pd_pefindo`, `mst.lgd_basel`, `mst.bobot_skenario`, `mst.impact_mev_pd` | `ecl.calc_header`, `ecl.lookthrough_underlying` | `lookthrough.compute` (internal) |
| LKT-003 | `mst.fund_composition`, `mst.instrumen`, `mst.pd_pefindo`, `mst.lgd_basel`, `mst.bobot_skenario`, `mst.impact_mev_pd` | `aud.audit_log` | `lookthrough.preview` |
| LKT-004 | Sama dengan LKT-002 (bulk) | `ecl.calc_header`, `ecl.lookthrough_underlying` | `lookthrough.compute` (internal) |
| LKT-005 | `mst.fund_composition`, `mst.instrumen`, `doc.upload` | `mst.fund_composition` (new version + supersede old), `aud.audit_log` | `fund_composition.create/review/approve` |

## Ringkasan Permissions Baru

| Permission | Granted To | Catatan |
|---|---|---|
| `fund_composition.create` | ROLE-AKUN | Submit + amend composition |
| `fund_composition.review` | ROLE-RISK | Sign review (6-eyes, step 2) |
| `fund_composition.approve` | ROLE-ALCO | Sign approve (6-eyes, step 3) — MFA wajib (ALCO) |
| `fund_composition.read` | ROLE-AKUN, ROLE-RISK, ROLE-ALCO, ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT | Read-only |
| `lookthrough.compute` | internal engine only | Tidak di-expose ke HTTP |
| `lookthrough.preview` | ROLE-RISK, ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT | Read-only preview UI |

## Ringkasan Audit Events

| Event | Kapan | Actor |
|---|---|---|
| `FUND_COMPOSITION.SUBMIT` | Submit komposisi baru | ROLE-AKUN |
| `FUND_COMPOSITION.REVIEW` | ROLE-RISK sign review | ROLE-RISK |
| `FUND_COMPOSITION.APPROVE` | ROLE-ALCO sign approve | ROLE-ALCO |
| `FUND_COMPOSITION.REJECT` | Reject oleh RISK atau ALCO | ROLE-RISK / ROLE-ALCO |
| `FUND_COMPOSITION.AMEND_SUBMIT` | Submit versi amend | ROLE-AKUN |
| `FUND_COMPOSITION.AMEND_REVIEW` | Review versi amend | ROLE-RISK |
| `FUND_COMPOSITION.AMEND_APPROVE` | Approve amend + close versi lama | ROLE-ALCO |
| `LOOKTHROUGH.PREVIEW` | Setiap panggilan GET preview | ROLE-RISK (atau bereizin) |
| `LOOKTHROUGH.EXPORT` | Export breakdown CSV/XLSX | ROLE-RISK (atau bereizin) |

---

## Handoff Notes untuk System Analyst

### Go interface yang diperlukan untuk `ecl-eir-engineer`

```go
// internal/ecl/lookthrough/service.go
type LookThroughService interface {
    // Single instrument compute — dipanggil oleh M7 ECL engine untuk satu instrumen
    Compute(ctx context.Context, instrumenID uuid.UUID, evaluationDate time.Time, periodeID uuid.UUID) (LookThroughResult, error)

    // Bulk compute — dipanggil sekali per calc run untuk semua Reksadana aktif
    // SLA: ≤ 2 detik P95 untuk 500 instruments
    BulkCompute(ctx context.Context, periodeID uuid.UUID, evaluationDate time.Time) (map[uuid.UUID]LookThroughResult, error)

    // Preview on-the-fly (tanpa menulis ke ecl.*) — untuk UI Story 3
    Preview(ctx context.Context, instrumenID uuid.UUID, evaluationDate time.Time, periodeID uuid.UUID) (LookThroughResult, error)
}

type FundCompositionService interface {
    // Submit composition group (satu atau lebih lines, sum = 100%)
    Submit(ctx context.Context, req SubmitCompositionRequest) (CompositionGroup, error)

    // Workflow transitions
    Review(ctx context.Context, compositionID uuid.UUID, reviewerID uuid.UUID, comment string) error
    Approve(ctx context.Context, compositionID uuid.UUID, approverID uuid.UUID, comment string) error
    Reject(ctx context.Context, compositionID uuid.UUID, actorID uuid.UUID, reason string) error

    // Get active composition per instrumen per date
    GetActive(ctx context.Context, instrumenID uuid.UUID, asOfDate time.Time) ([]FundCompositionLine, error)

    // History (for ROLE-AUDIT)
    ListHistory(ctx context.Context, instrumenID uuid.UUID, q listquery.Query) ([]CompositionGroup, listquery.Pagination, error)
}
```

### Error codes baru yang perlu ditambah ke `api-conventions.md`

| Code | HTTP | When |
|---|---|---|
| `LOOKTHROUGH_FUND_COMPOSITION_MISSING` | 422 | Tidak ada composition APPROVED untuk instrumen + tanggal evaluasi |
| `LOOKTHROUGH_NAB_MISSING` | 422 | NAB tidak tersedia untuk instrumen Reksadana |
| `LOOKTHROUGH_WEIGHT_INVALID` | 422 | Sum weight fund composition ≠ 100% ± 0.01% |
| `LOOKTHROUGH_POCI_DEFERRED` | 202 | Instrumen POCI Reksadana di-skip (Phase 5) |
| `FUND_COMPOSITION_OVERLAP` | 409 | Overlap dengan composition aktif yang sudah ada |

### Migration 000024 — scope untuk data-modeler

1. **CREATE TABLE `mst.fund_composition`** — DDL di DDL Gap 1 di atas
2. **ALTER TABLE `ecl.lookthrough_underlying`**:
   - `ead_underlying_idr NUMERIC(20,2)` → `NUMERIC(20,4)` (precision fix DEC-016)
   - `pd_normal NUMERIC(8,4)` → `NUMERIC(10,8)` (precision fix DEC-016)
   - `lgd NUMERIC(8,4)` → `NUMERIC(10,8)` (precision fix DEC-016)
   - `ecl_weighted_idr NUMERIC(20,2)` → `NUMERIC(20,4)` (precision fix DEC-016)
   - ADD COLUMN `fund_composition_id UUID REFERENCES mst.fund_composition(id)` — FK ke versi komposisi yang dipakai
   - ADD COLUMN `ecl_skenario_good_idr NUMERIC(20,4)` — ECL per skenario sebelum FL
   - ADD COLUMN `ecl_skenario_normal_idr NUMERIC(20,4)`
   - ADD COLUMN `ecl_skenario_bad_idr NUMERIC(20,4)`
   - ADD COLUMN `ecl_fl_good_idr NUMERIC(20,4)` — ECL per skenario setelah FL multiplier
   - ADD COLUMN `ecl_fl_normal_idr NUMERIC(20,4)`
   - ADD COLUMN `ecl_fl_bad_idr NUMERIC(20,4)`
   - ADD COLUMN `pd_good NUMERIC(10,8)`, `pd_bad NUMERIC(10,8)` — PD per skenario
   - ADD audit cols: `created_at`, `created_by`, `updated_at`, `updated_by`, `deleted_at`, `deleted_by`, `row_version`, `tenant_id`
3. **Seed data dev** — minimal 1 Reksadana aktif dengan fund composition APPROVED (untuk integration test)
4. **No hard delete trigger** untuk `ecl.lookthrough_underlying` (ecl schema rule — DEC-018)

### Compliance gate criteria (untuk ifrs9-compliance-reviewer)

Reviewer HARUS verifikasi sebelum merge:
1. DEC-015: Weighted ECL = `Σ(NAB × %class × PD_class × LGD_class × FL_skenario × bobot_skenario)` per story 2 AC 1
2. DEC-010: 3-skenario × dual FL tetap berlaku untuk setiap underlying — tidak ada simplifikasi skenario
3. DEC-016: Semua NUMERIC IDR = `(20,4)`, semua PD/LGD = `(10,8)`, no float64
4. DEC-015 + OQ-M4-3: FVTPL Reksadana = ECL 0 (skip, bukan error)
5. DEC-018: `ecl.lookthrough_underlying` append-only, tidak ada UPDATE/DELETE rows setelah calc run sealed
6. Konfirmasi OQ-M4-4: mapping asset class → PD/LGD sesuai FSD-APP-C (BLOCKING jika tidak ada di FSD)
