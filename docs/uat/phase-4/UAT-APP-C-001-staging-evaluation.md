# UAT Script — APP-C-001: Evaluasi Staging SICR & Cure
**ID UAT**: UAT-APP-C-001-001
**Story**: APP-C-ECL-001..003 — Staging Engine (SICR, Cure, Stage 3 Entry)
**Modul**: APP-C ECL Engine — Phase 4
**Tanggal**: 2026-06-13
**Versi**: 1.0
**Author**: qa-engineer
**Status**: READY FOR UAT

---

## Pre-conditions

### Infrastruktur
- Stack berjalan: `docker compose -f deploy/docker-compose.dev.yml up -d`
- Migrasi terbaru diterapkan: `go run ./cmd/migrator up` (termasuk 000022 staging engine)
- Backend API: `http://localhost:8080`
- Frontend: `http://localhost:3001`
- Periode buku PBUKU-2026-06 sudah ada dan berstatus `OPEN`

### Seed Data

```sql
-- ── Aktor ──────────────────────────────────────────────────────────────────
INSERT INTO sec.user (id, username, email, full_name, status, created_at, created_by)
VALUES
  ('b1000000-0000-0000-0000-000000000001', 'risk.officer.uat1', 'risk1@tugu-re.com', 'Risk Officer UAT', 'AKTIF', now(), '00000000-0000-0000-0000-000000000001'),
  ('b1000000-0000-0000-0000-000000000002', 'alco.member.uat1',  'alco1@tugu-re.com', 'ALCO UAT',         'AKTIF', now(), '00000000-0000-0000-0000-000000000001'),
  ('b1000000-0000-0000-0000-000000000003', 'komite.uat1',       'komite1@tugu-re.com','Komite Investasi UAT', 'AKTIF', now(), '00000000-0000-0000-0000-000000000001')
ON CONFLICT (username) DO NOTHING;

-- ── Counterparty + rating (AC Obligasi BANK-X, rating awal idAA) ──────────
INSERT INTO mst.counterparty (id, nama, tipe, created_by, updated_by) VALUES
  ('c1000000-0000-0000-0000-000000000001', 'PT Bank X (UAT)', 'BANK', 'b1000000-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000001');

INSERT INTO mst.instrumen (id, kode_instrumen, nama_instrumen, tipe_instrumen,
  klasifikasi_psak71, counterparty_id, nominal, mata_uang_kode,
  tanggal_penempatan, tanggal_jatuh_tempo, status, stage, created_by, updated_by)
VALUES (
  'd1000000-0000-0000-0000-000000000001',
  'OBL-UAT-001',
  'Obligasi AC BANK-X (UAT)',
  'OBLIGASI',
  'AC',
  'c1000000-0000-0000-0000-000000000001',
  1000000000.0000,  -- IDR 1B
  'IDR',
  '2024-01-01',
  '2030-12-31',
  'AKTIF',
  1,  -- Stage 1 awal
  'b1000000-0000-0000-0000-000000000001',
  'b1000000-0000-0000-0000-000000000001'
);

-- Rating awal instrumen: idAA (Investment Grade)
INSERT INTO mst.rating_history_counterparty
  (id, counterparty_id, rating, tanggal_efektif, sumber, created_by, updated_by)
VALUES
  ('e1000000-0000-0000-0000-000000000001',
   'c1000000-0000-0000-0000-000000000001',
   'idAA', '2024-01-01', 'PEFINDO',
   'b1000000-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000001');
```

---

## Skenario UAT

### TC-001: SICR — Rating Downgrade ≥ 2 Notch → Stage 1 ke Stage 2

**Aktor**: risk.officer.uat1 (ROLE-RISK)
**Prasyarat**: Instrumen OBL-UAT-001 dalam Stage 1, rating awal `idAA`

**Langkah-langkah**:

1. Login sebagai `risk.officer.uat1`.
2. Navigasi ke **Manajemen Data → Rating Counterparty → PT Bank X (UAT)**.
3. Klik **Tambah Rating Baru**.
4. Isi form:
   - **Rating**: `idBBB-` (turun 4 notch dari `idAA` ke `idBBB-`)
   - **Tanggal Efektif**: `2026-06-13`
   - **Sumber**: `PEFINDO`
5. Klik **Simpan**.
   - Toast hijau: `"Rating PT Bank X berhasil ditambahkan."`
6. Navigasi ke **ECL Engine → Staging → OBL-UAT-001**.
7. Klik **Evaluasi Staging (Manual)**.
   - Konfirmasi dialog: klik **Evaluasi**.

**Hasil yang Diharapkan**:

- Toast hijau: `"Evaluasi staging OBL-UAT-001 selesai. Stage berubah: Stage 1 → Stage 2."`
- Halaman staging menampilkan:
  - **Stage Saat Ini**: Stage 2
  - **Trigger**: `RATING_DOWNGRADE`
  - **Detail**: `"Rating idAA → idBBB- (4 notch, ≥ 2 notch threshold DEC-011)"`
  - **Tanggal Migrasi**: `2026-06-13`
  - **Status Approval**: `AUTO`

**Pemeriksaan Audit (DB)**:

```sql
-- Harus ada baris stage_history dengan trigger RATING_DOWNGRADE
SELECT stage_sebelum, stage_sesudah, trigger_type, detail_trigger, rating_saat_migrasi
FROM ecl.stage_history
WHERE instrumen_id = 'd1000000-0000-0000-0000-000000000001'
ORDER BY tanggal_migrasi DESC LIMIT 1;
-- Expected: STAGE_1 | STAGE_2 | RATING_DOWNGRADE | "Rating idAA → idBBB- (4 notch...)" | idBBB-

-- Harus ada audit event
SELECT action, event_time FROM aud.audit_log
WHERE entity_type = 'ecl.stage_history'
  AND entity_id = (SELECT id FROM ecl.stage_history WHERE instrumen_id = 'd1000000-0000-0000-0000-000000000001' ORDER BY tanggal_migrasi DESC LIMIT 1)
ORDER BY event_time DESC LIMIT 1;
-- Expected: ECL_STAGING.EVALUATE
```

**Rollback**:
```sql
DELETE FROM ecl.stage_history WHERE instrumen_id = 'd1000000-0000-0000-0000-000000000001';
UPDATE mst.instrumen SET stage = 1 WHERE id = 'd1000000-0000-0000-0000-000000000001';
```

---

### TC-002: SICR — DPD ≥ 30 Hari → Stage 2

**Aktor**: risk.officer.uat1 (ROLE-RISK)
**Prasyarat**: Instrumen OBL-UAT-001 dalam Stage 1, tanpa rating change

**Langkah-langkah**:

1. Login sebagai `risk.officer.uat1`.
2. Navigasi ke **ECL Engine → DPD → Catat DPD Manual**.
3. Isi form:
   - **Instrumen**: `OBL-UAT-001`
   - **Periode**: `2026-06-01`
   - **Nilai DPD**: `35`
   - **Sumber**: `MANUAL`
4. Klik **Simpan**.
   - Toast hijau: `"DPD OBL-UAT-001 berhasil dicatat."`
5. Navigasi ke **ECL Engine → Staging → OBL-UAT-001 → Evaluasi Staging**.
6. Klik **Evaluasi**.

**Hasil yang Diharapkan**:

- Stage berubah dari Stage 1 → Stage 2.
- Trigger ditampilkan: `DPD_GTE_30`, Detail: `"DPD = 35 hari (≥ 30 threshold DEC-011)"`

**Verifikasi Numerik** (DPD 35 ≥ 30 threshold → SICR fired):
```
DPD_value = 35
Threshold = 30 (DEC-011)
SICR = DPD ≥ 30 → TRUE → Stage 1 → Stage 2
```

---

### TC-003: Cure — Stage 2 → Stage 1 Setelah 3 Periode Berturut-Turut

**Aktor**: risk.officer.uat1 (ROLE-RISK)
**Prasyarat**: Instrumen dalam Stage 2 (dari TC-001 atau seed manual), 3 periode buku BULANAN berturut-turut berstatus CLOSED tanpa SICR trigger.

**Seed tambahan**:
```sql
-- Simulasi 3 periode BULANAN CLOSED tanpa SICR trigger baru
-- (Dalam kondisi riil, tidak ada rating downgrade baru selama 3 bulan)
-- Pastikan periode Maret, April, Mei 2026 dalam status CLOSED:
UPDATE mst.periode_buku
SET status = 'CLOSED', closed_at = now()
WHERE periode_id IN ('PBUKU-2026-03', 'PBUKU-2026-04', 'PBUKU-2026-05');
```

**Langkah-langkah**:

1. Login sebagai `risk.officer.uat1`.
2. Navigasi ke **ECL Engine → Staging → OBL-UAT-001**.
3. Klik **Evaluasi Cure** (tombol terpisah dari Evaluasi SICR).

**Hasil yang Diharapkan**:

- Toast hijau: `"Evaluasi cure OBL-UAT-001 selesai. Stage berubah: Stage 2 → Stage 1. (Cure 3 periode BULANAN berturut-turut)"`
- Stage ditampilkan: Stage 1
- Trigger: `CURE_3_PERIODE_BULANAN`
- `stage_history` baru di-append (bukan update row lama — DEC-018).

**Pemeriksaan DB**:
```sql
SELECT COUNT(*) FROM ecl.stage_history
WHERE instrumen_id = 'd1000000-0000-0000-0000-000000000001'
  AND trigger_type = 'CURE_3_PERIODE_BULANAN';
-- Expected: 1
```

---

### TC-004: Stage 3 — Rating idD Langsung

**Aktor**: risk.officer.uat1 (ROLE-RISK)
**Prasyarat**: Instrumen dalam Stage 1

**Langkah-langkah**:

1. Tambahkan rating `idD` untuk counterparty PT Bank X dengan tanggal efektif hari ini.
2. Evaluasi staging manual.

**Hasil yang Diharapkan**:

- Stage berubah langsung ke Stage 3.
- Trigger: `RATING_DEFAULT`
- Di timeline staging: jika sebelumnya Stage 1, muncul 2 baris: Stage1→Stage2 (intermediate) lalu Stage2→Stage3 (langsung default) — sesuai `needsDoubleRow = true` untuk direct Stage 1→3.

---

### TC-005: View Staging History DataTable

**Aktor**: risk.officer.uat1 (ROLE-RISK)

**Langkah-langkah**:

1. Navigasi ke **ECL Engine → Staging History → OBL-UAT-001**.
2. Verifikasi DataTable (UX §1):
   - Kolom: Tanggal Migrasi, Stage Sebelum, Stage Sesudah, Trigger, Detail, Status Approval.
   - Sort: klik header kolom Tanggal Migrasi → urutan desc/asc.
   - Filter: filter `Stage Sesudah = STAGE_2` → hanya tampil row SICR.
   - Export CSV: klik **Export ▾ → CSV** → file terdownload dengan nama `staging-history-OBL-UAT-001-YYYYMMDD.csv`.

**Hasil yang Diharapkan**:
- DataTable menampilkan semua history rows (append-only — tidak bisa diedit/dihapus dari UI).
- Filter + sort berfungsi.
- Export CSV mengandung data yang sama dengan yang tampil di layar.

---

## Checklist Audit Pasca UAT

```sql
-- 1. Semua stage_history rows tidak bisa didelete (hard-delete dilarang DEC-018)
SELECT COUNT(*) FROM ecl.stage_history WHERE instrumen_id = 'd1000000-0000-0000-0000-000000000001' AND deleted_at IS NOT NULL;
-- Expected: 0

-- 2. Hash chain audit_log tidak rusak (jalankan verifier)
-- go run ./cmd/audit-verify --entity-type ecl.stage_history --range "2026-06-01:2026-06-30"
-- Expected: "Hash chain OK — 0 violations"
```

---

## Rollback / Cleanup

```sql
BEGIN;
DELETE FROM ecl.stage_history WHERE instrumen_id = 'd1000000-0000-0000-0000-000000000001';
UPDATE mst.instrumen SET stage = 1 WHERE id = 'd1000000-0000-0000-0000-000000000001';
DELETE FROM mst.rating_history_counterparty WHERE counterparty_id = 'c1000000-0000-0000-0000-000000000001' AND rating IN ('idBBB-', 'idD');
ROLLBACK; -- ubah ke COMMIT setelah verifikasi
```

---

## Sign-off UAT

| Nama | Jabatan | Tanda Tangan | Tanggal |
|------|---------|--------------|---------|
| | Risk Officer | | |
| | QA Engineer | | |
| | IFRS9 Compliance Reviewer | | |
