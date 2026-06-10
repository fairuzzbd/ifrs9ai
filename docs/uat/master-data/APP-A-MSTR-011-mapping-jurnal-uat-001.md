# UAT Script — APP-A-MSTR-011: Mapping Jurnal (Header + Detail)
**ID UAT**: UAT-APP-A-MSTR-011-001
**Story**: APP-A-MSTR-011 Master Mapping Jurnal (4-eyes, header + detail)
**Modul**: APP-D Mapping Jurnal & GL
**Tanggal**: 2026-06-04
**Versi**: 1.0
**Author**: qa-engineer
**Status**: READY FOR UAT

---

## Lingkup

Modul ini mengelola konfigurasi pemetaan jurnal akuntansi (mst.mapping_jurnal_header + mst.mapping_jurnal_detail). Setiap header mendeskripsikan event akuntansi (contoh: PENEMPATAN_DEPOSITO, MTM_OBLIGASI) beserta daftar pasangan DEBIT/KREDIT akun CoA (mst.chart_of_accounts).

**Aturan bisnis kritis:**
1. event_code harus unik dan mengikuti pola `^[A-Z0-9_]+$`.
2. Minimal 2 baris detail (pasangan DEBIT + KREDIT).
3. Sum multiplier DEBIT = Sum multiplier KREDIT saat Approve (toleransi 0.0001).
4. Setiap kode_akun_id yang direferensikan harus berstatus APPROVED di mst.chart_of_accounts.
5. Workflow 4-eyes: Maker (AKUN) → Reviewer (AKUN-CTL) → Approver (AKUN-CTL, user berbeda).
6. SoD: maker ≠ reviewer ≠ approver.

---

## Pre-conditions

### Infrastruktur
- BLIPS dev/UAT stack berjalan: `docker compose -f deploy/docker/docker-compose.dev.yml up -d`
- Database sudah dimigrasikan: `go run ./cmd/migrator up` (pastikan migration 0017 sudah dijalankan)
- Backend API berjalan di `http://localhost:8080`
- Frontend berjalan di `http://localhost:3001`

### Seed Data Minimal (jalankan SQL berikut sebelum UAT)

```sql
-- Persona test (3 user distinct untuk SoD)
-- Gunakan Keycloak admin console untuk buat user dan assign role.
-- Atau insert langsung via sec.user:

INSERT INTO sec.user (id, username, email, full_name, status, created_at, created_by)
VALUES
    ('aaaaaaaa-0001-0000-0000-000000000001', 'akun.maker',   'akun.maker@tugu-re.com',   'Treasury Maker UAT',     'AKTIF', now(), '00000000-0000-0000-0000-000000000001'),
    ('aaaaaaaa-0002-0000-0000-000000000001', 'akun.ctl.1',   'akun.ctl1@tugu-re.com',    'Finance CTL UAT 1',      'AKTIF', now(), '00000000-0000-0000-0000-000000000001'),
    ('aaaaaaaa-0003-0000-0000-000000000001', 'akun.ctl.2',   'akun.ctl2@tugu-re.com',    'Finance CTL UAT 2',      'AKTIF', now(), '00000000-0000-0000-0000-000000000001')
ON CONFLICT (username) DO NOTHING;

-- Role assignment di Keycloak:
--   akun.maker  → ROLE-AKUN
--   akun.ctl.1  → ROLE-AKUN-CTL
--   akun.ctl.2  → ROLE-AKUN-CTL

-- CoA seed: 2 akun APPROVED yang akan dipakai sebagai referensi di detail
INSERT INTO mst.chart_of_accounts (
    id, kode_akun, nama_akun, tipe_akun, sub_tipe_akun,
    mata_uang_native, posisi_normal, aktif_flag, sumber_coa,
    tanggal_mulai_aktif, created_by, created_at
) VALUES
    ('bbbbbbbb-0001-0000-0000-000000000001', '1101001', 'Kas dan Bank',          'ASET',       'KAS',      'IDR', 'DEBIT',  true, 'SISTEM', '2020-01-01', '00000000-0000-0000-0000-000000000001', now()),
    ('bbbbbbbb-0002-0000-0000-000000000001', '2101001', 'Utang Bunga Deposito',  'LIABILITAS', 'HUTANG',   'IDR', 'KREDIT', true, 'SISTEM', '2020-01-01', '00000000-0000-0000-0000-000000000001', now()),
    ('bbbbbbbb-0003-0000-0000-000000000001', '4101001', 'Pendapatan Bunga',      'PENDAPATAN', 'BUNGA',    'IDR', 'KREDIT', true, 'SISTEM', '2020-01-01', '00000000-0000-0000-0000-000000000001', now())
ON CONFLICT (kode_akun) DO NOTHING;

-- Set workflow_status APPROVED pada CoA (kolom ditambahkan via migration 000008+)
UPDATE mst.chart_of_accounts
    SET workflow_status = 'APPROVED'
WHERE kode_akun IN ('1101001', '2101001', '4101001');

-- CoA DRAFT: digunakan untuk TC-003 (sengaja dibiarkan DRAFT)
INSERT INTO mst.chart_of_accounts (
    id, kode_akun, nama_akun, tipe_akun, sub_tipe_akun,
    mata_uang_native, posisi_normal, aktif_flag, sumber_coa,
    tanggal_mulai_aktif, created_by, created_at
) VALUES
    ('bbbbbbbb-0004-0000-0000-000000000001', '9901001', 'CoA Draft Uji',  'ASET', 'LAINNYA', 'IDR', 'DEBIT', true, 'SISTEM', '2026-01-01', '00000000-0000-0000-0000-000000000001', now())
ON CONFLICT (kode_akun) DO NOTHING;

UPDATE mst.chart_of_accounts SET workflow_status = 'DRAFT' WHERE kode_akun = '9901001';
```

### Pembersihan setelah UAT
```sql
-- Hapus data test
DELETE FROM mst.mapping_jurnal_detail
WHERE event_header_id IN (
    SELECT id FROM mst.mapping_jurnal_header
    WHERE event_code IN (
        'PENEMPATAN_DEPOSITO_UAT',
        'PENEMPATAN_DEPOSITO_IMBAL',
        'PENEMPATAN_DEPOSITO_CDRAFT',
        'PENEMPATAN_DEPOSITO_DEL',
        'PENEMPATAN_DEPOSITO_EDIT'
    )
);
DELETE FROM mst.mapping_jurnal_header
WHERE event_code IN (
    'PENEMPATAN_DEPOSITO_UAT',
    'PENEMPATAN_DEPOSITO_IMBAL',
    'PENEMPATAN_DEPOSITO_CDRAFT',
    'PENEMPATAN_DEPOSITO_DEL',
    'PENEMPATAN_DEPOSITO_EDIT'
);
```

---

## SKENARIO UAT

---

### TC-001: Create Header + 2 Detail Balanced (Happy Path Create)

**ID**: TC-001
**Aktor**: akun.maker (ROLE-AKUN)
**Pre-condition**: event_code `PENEMPATAN_DEPOSITO_UAT` belum ada di database

**Tujuan**: Memverifikasi bahwa ROLE-AKUN dapat membuat mapping jurnal header + 2 detail (DEBIT + KREDIT) dengan multiplier seimbang, dan record masuk ke DB dalam status DRAFT.

**Langkah-langkah**:

1. Login sebagai `akun.maker` di `http://localhost:3001`
2. Navigasi ke **Master Data > Mapping Jurnal** (`/master/mapping-jurnal`)
3. Klik tombol **Buat Mapping Jurnal**
4. Isi form header:
   - **Event ID Kode**: `PENEMPATAN_DEPOSITO_UAT_ID`
   - **Event Code**: `PENEMPATAN_DEPOSITO_UAT`
   - **Nama Event**: `Penempatan Deposito UAT Test`
   - **Kategori Event**: `PENEMPATAN`
   - **Trigger Source**: `SYSTEM`
5. Tambahkan detail row pertama (DEBIT):
   - **Urutan**: 1
   - **Kode Akun**: `1101001 — Kas dan Bank`
   - **DK Indicator**: `DEBIT`
   - **Sumber Amount**: `POKOK`
   - **Multiplier**: `1.0000`
   - **Mata Uang Posting**: `IDR`
6. Tambahkan detail row kedua (KREDIT):
   - **Urutan**: 2
   - **Kode Akun**: `2101001 — Utang Bunga Deposito`
   - **DK Indicator**: `KREDIT`
   - **Sumber Amount**: `POKOK`
   - **Multiplier**: `1.0000`
   - **Mata Uang Posting**: `IDR`
7. Klik **Simpan**

**Hasil yang Diharapkan**:

- Toast sukses muncul: `"Mapping Jurnal PENEMPATAN_DEPOSITO_UAT berhasil dibuat. Menunggu review."` dengan link "Lihat detail"
- Form direset ke kosong
- Redirect atau navigasi ke halaman detail `/master/mapping-jurnal/{id}`
- Di halaman detail: `workflowStatus = DRAFT`, 2 detail row tampil

**Audit Check**:

```sql
-- Harus ada 1 row audit MAPPING_JURNAL.CREATE
SELECT action, actor_role, after_value
FROM aud.audit_log
WHERE entity_type = 'mst.mapping_jurnal_header'
  AND action = 'MAPPING_JURNAL.CREATE'
ORDER BY event_time DESC LIMIT 1;

-- Harus ada header + 2 detail di DB
SELECT h.event_code, h.workflow_status, COUNT(d.id) AS detail_count
FROM mst.mapping_jurnal_header h
LEFT JOIN mst.mapping_jurnal_detail d ON d.event_header_id = h.id AND d.deleted_at IS NULL
WHERE h.event_code = 'PENEMPATAN_DEPOSITO_UAT'
GROUP BY h.event_code, h.workflow_status;
-- Expected: PENEMPATAN_DEPOSITO_UAT | DRAFT | 2
```

**Rollback**: Jalankan SQL cleanup di atas.

---

### TC-002: Sum Debit ≠ Kredit Blocks Approve

**ID**: TC-002
**Aktor**: akun.maker (ROLE-AKUN), akun.ctl.1 (reviewer), akun.ctl.2 (approver)
**Pre-condition**: TC-001 sudah selesai. Header `PENEMPATAN_DEPOSITO_IMBAL` dibuat dengan multiplier tidak seimbang (DEBIT=1.0, KREDIT=2.0).

**Tujuan**: Memverifikasi bahwa approve gagal dengan error `MAPPING_JURNAL_DEBIT_CREDIT_MISMATCH` jika total multiplier DEBIT tidak sama dengan KREDIT.

**Setup data awal**:
```sql
-- Insert header imbalanced (manual seed, atau via UI)
-- DEBIT = 1.0000, KREDIT = 2.0000 (imbalanced)
```

**Langkah-langkah**:

1. Login sebagai `akun.maker`, buat mapping jurnal `PENEMPATAN_DEPOSITO_IMBAL` dengan:
   - Detail 1: Kode Akun `1101001`, DEBIT, Multiplier `1.0000`
   - Detail 2: Kode Akun `2101001`, KREDIT, Multiplier `2.0000`
   - Klik **Simpan** (berhasil — validasi balance baru terjadi saat Approve)
2. Klik **Submit untuk Review** (Simpan → Submit)
3. Login sebagai `akun.ctl.1` (reviewer)
4. Navigasi ke queue **Mapping Jurnal Pending Review**
5. Buka `PENEMPATAN_DEPOSITO_IMBAL`, klik **Review / Setujui ke Approval**
6. Login sebagai `akun.ctl.2` (approver, user berbeda dari reviewer)
7. Navigasi ke queue **Mapping Jurnal Pending Approval**
8. Buka `PENEMPATAN_DEPOSITO_IMBAL`, klik **Approve**

**Hasil yang Diharapkan**:

- Langkah 1–5: Berhasil, workflow masuk PENDING_APPROVAL
- Langkah 8: Toast error merah persistent:
  `"Approve gagal: Total multiplier DEBIT (1.0000) tidak sama dengan KREDIT (2.0000). Selisih: 1.0000. Sesuaikan multiplier sebelum approval."`
  - Error code: `MAPPING_JURNAL_DEBIT_CREDIT_MISMATCH`
- Workflow state tetap `PENDING_APPROVAL` (tidak berubah ke APPROVED)

**Audit Check**:

```sql
-- Tidak ada audit event MAPPING_JURNAL.APPROVE untuk entity ini
SELECT COUNT(*) FROM aud.audit_log
WHERE action = 'MAPPING_JURNAL.APPROVE'
  AND entity_id = (
    SELECT id FROM mst.mapping_jurnal_header WHERE event_code = 'PENEMPATAN_DEPOSITO_IMBAL'
  );
-- Expected: 0

-- workflow_instance tetap di PENDING_APPROVAL
SELECT wi.current_state
FROM sys.workflow_instance wi
JOIN mst.mapping_jurnal_header h ON wi.entity_id = h.id
WHERE h.event_code = 'PENEMPATAN_DEPOSITO_IMBAL';
-- Expected: PENDING_APPROVAL
```

**Rollback**: Hapus header via admin atau SQL cleanup.

---

### TC-003: Detail Kode Akun DRAFT Blocks Approve

**ID**: TC-003
**Aktor**: akun.maker, akun.ctl.1 (reviewer), akun.ctl.2 (approver)
**Pre-condition**: CoA `9901001` ada di DB dengan `workflow_status = 'DRAFT'` (seed TC-003 di atas).

**Tujuan**: Memverifikasi bahwa approve gagal jika salah satu detail referensi CoA yang belum APPROVED, dengan error `MAPPING_JURNAL_KODE_AKUN_NOT_APPROVED`.

**Langkah-langkah**:

1. Login sebagai `akun.maker`, buat mapping jurnal `PENEMPATAN_DEPOSITO_CDRAFT` dengan:
   - Detail 1: Kode Akun `1101001`, DEBIT, Multiplier `1.0000`
   - Detail 2: Kode Akun `9901001 — CoA Draft Uji`, KREDIT, Multiplier `1.0000`
   - Klik **Simpan**
2. Klik **Submit untuk Review**
3. Login sebagai `akun.ctl.1`, klik **Review** di antrian
4. Login sebagai `akun.ctl.2`, klik **Approve** di antrian

**Hasil yang Diharapkan**:

- Langkah 1–3: Berhasil (CoA DRAFT dapat dipakai di detail sebelum approve)
- Langkah 4: Toast error merah persistent:
  `"Approve gagal: Akun '9901001' belum disetujui. Semua akun yang direferensikan harus berstatus APPROVED sebelum mapping dapat disetujui."`
  - Error code: `MAPPING_JURNAL_KODE_AKUN_NOT_APPROVED`
- Workflow tetap `PENDING_APPROVAL`

**Audit Check**:

```sql
-- Tidak ada MAPPING_JURNAL.APPROVE
SELECT COUNT(*) FROM aud.audit_log
WHERE action = 'MAPPING_JURNAL.APPROVE'
  AND entity_id = (
    SELECT id FROM mst.mapping_jurnal_header WHERE event_code = 'PENEMPATAN_DEPOSITO_CDRAFT'
  );
-- Expected: 0

-- CoA 9901001 masih DRAFT
SELECT workflow_status FROM mst.chart_of_accounts WHERE kode_akun = '9901001';
-- Expected: DRAFT
```

**Rollback**: Hapus data test via SQL cleanup.

---

### TC-004: 4-Eyes Happy Path — Full Workflow DRAFT → APPROVED

**ID**: TC-004
**Aktor**: akun.maker (Maker), akun.ctl.1 (Reviewer), akun.ctl.2 (Approver)
**Pre-condition**: Semua CoA (`1101001`, `2101001`) berstatus APPROVED. Ketiga user sudah login dan memiliki role yang benar.

**Tujuan**: Memverifikasi alur 4-eyes lengkap: Maker submit → Reviewer review → Approver approve → status APPROVED. Setiap transisi menghasilkan audit event dan tanda tangan.

**Data numerik contoh** (dari SoW §7.3):

| Field | Nilai |
|---|---|
| Event Code | `PENEMPATAN_DEPOSITO_UAT` |
| Multiplier DEBIT | 1.0000 (akun `1101001`) |
| Multiplier KREDIT | 1.0000 (akun `2101001`) |
| Sum DEBIT = Sum KREDIT | 1.0000 = 1.0000 (balanced) |

**Langkah-langkah**:

1. **[Maker]** Login sebagai `akun.maker`
2. Buat mapping jurnal `PENEMPATAN_DEPOSITO_UAT` (jika belum ada, lihat TC-001)
3. Klik **Submit untuk Review**, tambahkan komentar: `"Ajukan review — mapping deposito baru"`
4. Verifikasi: workflowStatus = `PENDING_REVIEW`, toast sukses tampil

5. **[Reviewer]** Login sebagai `akun.ctl.1`
6. Navigasi ke **Mapping Jurnal > Pending Review**
7. Buka `PENEMPATAN_DEPOSITO_UAT`
8. Verifikasi detail: 2 baris, sum DEBIT 1.0000 = sum KREDIT 1.0000
9. Klik **Review (Setujui ke Approval)**, komentar: `"Review OK — multiplier seimbang, CoA valid"`
10. Verifikasi: workflowStatus = `PENDING_APPROVAL`

11. **[Approver]** Login sebagai `akun.ctl.2` (bukan akun.ctl.1)
12. Navigasi ke **Mapping Jurnal > Pending Approval**
13. Buka `PENEMPATAN_DEPOSITO_UAT`
14. Klik **Approve**, komentar: `"Disetujui — mapping valid untuk produksi"`
15. Verifikasi: workflowStatus = `APPROVED`, toast sukses tampil

**Hasil yang Diharapkan**:

- Langkah 4: workflowStatus = `PENDING_REVIEW`
- Langkah 10: workflowStatus = `PENDING_APPROVAL`
- Langkah 15: workflowStatus = `APPROVED`
- Mapping tidak dapat diedit setelah APPROVED (tombol Edit di-disable atau error `MASTER_APPROVED_NO_EDIT`)

**Audit Check**:

```sql
-- Minimal 3 audit event: CREATE, SUBMIT, APPROVE
SELECT action, actor_role, event_time
FROM aud.audit_log
WHERE entity_id = (
    SELECT id FROM mst.mapping_jurnal_header WHERE event_code = 'PENEMPATAN_DEPOSITO_UAT'
)
ORDER BY event_time ASC;
-- Expected rows: MAPPING_JURNAL.CREATE, MAPPING_JURNAL.SUBMIT, MAPPING_JURNAL.APPROVE

-- workflow_instance di APPROVED
SELECT wi.current_state, wi.maker_id, wi.reviewer_id, wi.approver_id
FROM sys.workflow_instance wi
JOIN mst.mapping_jurnal_header h ON wi.entity_id = h.id
WHERE h.event_code = 'PENEMPATAN_DEPOSITO_UAT';
-- Expected: current_state=APPROVED, maker_id ≠ reviewer_id ≠ approver_id

-- Tanda tangan >= 2 (submit + review + approve)
SELECT COUNT(*) AS sig_count
FROM sys.workflow_signature ws
JOIN sys.workflow_instance wi ON ws.workflow_instance_id = wi.id
JOIN mst.mapping_jurnal_header h ON wi.entity_id = h.id
WHERE h.event_code = 'PENEMPATAN_DEPOSITO_UAT';
-- Expected: >= 2

-- workflow_status tersinkron di header
SELECT workflow_status
FROM mst.mapping_jurnal_header
WHERE event_code = 'PENEMPATAN_DEPOSITO_UAT';
-- Expected: APPROVED
```

**Rollback**: Tidak perlu; data APPROVED dapat dipakai sebagai fixture untuk TC berikutnya.

---

### TC-005: Soft-Delete Header — Detail Cascade

**ID**: TC-005
**Aktor**: akun.maker (ROLE-AKUN)
**Pre-condition**: Header `PENEMPATAN_DEPOSITO_DEL` dalam status DRAFT (buat via TC-001 pattern dengan event_code berbeda). Jumlah detail = 2.

**Tujuan**: Memverifikasi bahwa soft-delete header menyebabkan header.deleted_at ter-set, serta detail rows tetap tercatat di DB (soft-cascade via filter, bukan hard-delete), sehingga audit trail tidak hilang.

**Langkah-langkah**:

1. Login sebagai `akun.maker`
2. Buat mapping jurnal `PENEMPATAN_DEPOSITO_DEL` (DRAFT, 2 detail balanced)
3. Dari halaman detail, klik **Hapus** (tombol Delete)
4. Konfirmasi dialog: `"Apakah Anda yakin ingin menghapus mapping jurnal ini? Tindakan ini tidak dapat dibatalkan."` → klik **Ya, Hapus**

**Hasil yang Diharapkan**:

- Toast sukses: `"Mapping Jurnal PENEMPATAN_DEPOSITO_DEL berhasil dihapus."`
- Header tidak lagi muncul di daftar (`GET /api/v1/master/mapping-jurnal` tanpa `include_deleted=true`)
- Jika dicari dengan `include_deleted=true` (ROLE-AUDIT), header tampil dengan `deletedAt` terisi

**Audit Check**:

```sql
-- header.deleted_at harus ter-set
SELECT event_code, deleted_at, deleted_by
FROM mst.mapping_jurnal_header
WHERE event_code = 'PENEMPATAN_DEPOSITO_DEL';
-- Expected: deleted_at NOT NULL

-- Detail rows masih ada di DB (soft-cascade)
SELECT COUNT(*) FROM mst.mapping_jurnal_detail
WHERE event_header_id = (
    SELECT id FROM mst.mapping_jurnal_header WHERE event_code = 'PENEMPATAN_DEPOSITO_DEL'
);
-- Expected: >= 0 (boleh 2 jika soft-cascade, boleh 0 jika hard FK cascade — keduanya acceptable)

-- Audit event DELETE
SELECT action, actor_role FROM aud.audit_log
WHERE entity_id = (
    SELECT id FROM mst.mapping_jurnal_header WHERE event_code = 'PENEMPATAN_DEPOSITO_DEL'
)
  AND action = 'MAPPING_JURNAL.DELETE';
-- Expected: 1 row
```

**Rollback**: Data sudah soft-deleted, tidak perlu rollback tambahan.

---

### TC-006: Edit + Replace Details Transaksional

**ID**: TC-006
**Aktor**: akun.maker (ROLE-AKUN)
**Pre-condition**: Header `PENEMPATAN_DEPOSITO_EDIT` dalam status DRAFT dengan 2 detail (DEBIT `1101001` + KREDIT `2101001`, multiplier 1.0 masing-masing).

**Tujuan**: Memverifikasi bahwa PATCH dengan field `details` baru mengganti seluruh detail rows secara atomis (bulk replace), dan row_version naik setelah update berhasil.

**Data sebelum edit**:

| Urutan | Kode Akun | DK | Multiplier |
|---|---|---|---|
| 1 | 1101001 | DEBIT | 1.0000 |
| 2 | 2101001 | KREDIT | 1.0000 |

**Langkah-langkah**:

1. Login sebagai `akun.maker`
2. Buat mapping jurnal `PENEMPATAN_DEPOSITO_EDIT` dengan data di atas (row_version awal = 1)
3. Buka halaman detail `/master/mapping-jurnal/{id}`
4. Klik **Edit**
5. Ubah **Nama Event** menjadi `Penempatan Deposito EDIT v2`
6. Di section detail, hapus baris 2 (KREDIT 2101001) dan tambahkan 2 baris baru:
   - Baris 2: Kode Akun `4101001 — Pendapatan Bunga`, KREDIT, Multiplier `0.7500`
   - Baris 3: Kode Akun `2101001 — Utang Bunga Deposito`, KREDIT, Multiplier `0.2500`
   - (total KREDIT = 0.7500 + 0.2500 = 1.0000 = DEBIT; balanced)
7. Klik **Simpan** (row_version dikirim = 1 di request body)

**Data setelah edit yang diharapkan**:

| Urutan | Kode Akun | DK | Multiplier |
|---|---|---|---|
| 1 | 1101001 | DEBIT | 1.0000 |
| 2 | 4101001 | KREDIT | 0.7500 |
| 3 | 2101001 | KREDIT | 0.2500 |

**Hasil yang Diharapkan**:

- Toast sukses: `"Mapping Jurnal PENEMPATAN_DEPOSITO_EDIT berhasil diperbarui."`
- Halaman detail menampilkan 3 baris detail sesuai data setelah edit
- `row_version` = 2 (diinkrement dari 1 ke 2)

**Audit Check**:

```sql
-- Audit event UPDATE
SELECT action, actor_role FROM aud.audit_log
WHERE entity_id = (
    SELECT id FROM mst.mapping_jurnal_header WHERE event_code = 'PENEMPATAN_DEPOSITO_EDIT'
)
  AND action = 'MAPPING_JURNAL.UPDATE'
ORDER BY event_time DESC LIMIT 1;
-- Expected: 1 row

-- Detail lama (2101001 KREDIT) soft-deleted
SELECT COUNT(*) FROM mst.mapping_jurnal_detail
WHERE event_header_id = (
    SELECT id FROM mst.mapping_jurnal_header WHERE event_code = 'PENEMPATAN_DEPOSITO_EDIT'
)
  AND deleted_at IS NULL;
-- Expected: 3 (baris aktif baru)

SELECT COUNT(*) FROM mst.mapping_jurnal_detail
WHERE event_header_id = (
    SELECT id FROM mst.mapping_jurnal_header WHERE event_code = 'PENEMPATAN_DEPOSITO_EDIT'
)
  AND deleted_at IS NOT NULL;
-- Expected: >= 1 (baris lama soft-deleted)

-- row_version naik
SELECT row_version FROM mst.mapping_jurnal_header WHERE event_code = 'PENEMPATAN_DEPOSITO_EDIT';
-- Expected: 2
```

**Rollback**: Hapus via DELETE endpoint atau SQL cleanup.

---

## Checklist Ringkas

| TC | Skenario | Aktor | Status |
|---|---|---|---|
| TC-001 | Create header + 2 detail balanced | Maker | Belum dijalankan |
| TC-002 | Sum debit ≠ kredit blocks approve | Maker + Reviewer + Approver | Belum dijalankan |
| TC-003 | Detail kode_akun DRAFT blocks approve | Maker + Reviewer + Approver | Belum dijalankan |
| TC-004 | 4-eyes happy path DRAFT → APPROVED | Maker + Reviewer + Approver | Belum dijalankan |
| TC-005 | Delete header → detail cascade | Maker | Belum dijalankan |
| TC-006 | Edit + replace details transaksional | Maker | Belum dijalankan |

---

## Referensi

- FSD-APP-A-MSTR-011 §4 (Mapping Jurnal CRUD)
- ERD-BLIPS-IFRS9-v1.2 §4.11–4.12 (mst.mapping_jurnal_header, mst.mapping_jurnal_detail)
- SoW_v1.4 §7.3 (Mapping Jurnal workflow rules)
- `@.claude/memory/security-baseline.md` (SoD enforcement)
- Migration 000017 (mapping_jurnal_schema_fix — audit columns + workflow_status)
