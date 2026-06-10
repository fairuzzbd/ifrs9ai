# UAT-001 — Master Data Kurs (APP-A-MSTR-014)

**Modul**: APP-A Master Data  
**Fitur**: Manajemen Kurs Valuta Asing (mst.kurs)  
**Versi dokumen**: 1.0  
**Tanggal**: 2026-06-05  
**Penulis**: QA Engineer (qa-engineer)  
**Referensi**: FSD-APP-A-MASTER-DATA-v1.1.docx §9, SoW_v1.4.docx §3.9, BRD §4.7

---

## Ringkasan Scope

Pengujian end-to-end untuk proses pencatatan kurs valuta asing harian melalui
workflow 4-mata (ROLE-AKUN sebagai Maker, ROLE-AKUN-CTL sebagai Reviewer dan
Approver yang berbeda), validasi bisnis (IDR dilarang, relasi beli/tengah/jual,
tanggal masa depan), perlindungan kurs terkunci (locked_flag), dan endpoint
stub JISDOR-sync.

---

## Prasyarat

### Data Seed

Sebelum menjalankan semua skenario, pastikan data berikut tersedia di sistem UAT:

| Entitas | Nilai | Keterangan |
|---|---|---|
| `mst.mata_uang` | `USD` — APPROVED | Digunakan di SC-01 s.d. SC-04 |
| `mst.mata_uang` | `EUR` — APPROVED | Digunakan di SC-02 |
| `mst.mata_uang` | `GBP` — APPROVED | Digunakan di SC-03 |
| `mst.mata_uang` | `CHF` — APPROVED | Digunakan di SC-05 |
| `mst.periode_buku` | `PRD-2026-06` — OPEN, 2026-06-01 s.d. 2026-06-30 | Dari seed migration 000002 |
| `mst.periode_buku` | `PRD-2026-05` — CLOSED/SOFT_CLOSED | Untuk SC-05 (locked) |

### Role Assignment (3 user berbeda)

| Username UAT | Role | Digunakan untuk |
|---|---|---|
| `uat_kurs_maker` | `ROLE-AKUN` | Maker di semua skenario |
| `uat_kurs_reviewer` | `ROLE-AKUN-CTL` | Reviewer di SC-03 |
| `uat_kurs_approver` | `ROLE-AKUN-CTL` | Approver di SC-03 (user berbeda dari reviewer) |

> **SoD penting**: `uat_kurs_maker`, `uat_kurs_reviewer`, dan `uat_kurs_approver`
> harus 3 user yang berbeda. Jangan gunakan akun yang sama untuk 2 peran berbeda.

### Periode Buku State

Periode `PRD-2026-06` harus dalam status `OPEN` sebelum SC-01 s.d. SC-04 dijalankan.  
Periode `PRD-2026-05` harus dalam status `CLOSED` sebelum SC-05 dijalankan.

---

## Skenario

---

### SC-01 — Validasi: Mata Uang IDR Ditolak

**Aktor**: `uat_kurs_maker` (ROLE-AKUN)  
**Pre-kondisi**: Login sebagai Maker.

**Langkah-langkah**:

1. Buka menu **Master Data > Kurs**.
2. Klik tombol **Tambah Kurs Baru**.
3. Isi form:
   - Kode Mata Uang: `IDR`
   - Tanggal Berlaku: `2026-06-05`
   - Kurs Tengah: `1.0000`
   - Sumber Kurs: `MANUAL`
4. Klik **Simpan**.

**Hasil yang diharapkan**:

- Form tidak dapat di-submit.
- Muncul toast error merah (persistent) dengan pesan yang menyebutkan `IDR` tidak valid sebagai kurs.
- HTTP status yang dikirim ke backend: `422 Unprocessable Entity`.
- Response body berisi `"code": "VALIDATION_FAILED"` dengan detail field `kodeMataUang`.
- Tidak ada record baru di `mst.kurs`.
- Tidak ada event di `aud.audit_log` untuk aksi ini.

**Audit check**:

```sql
SELECT COUNT(*) FROM mst.kurs WHERE kode_mata_uang = 'IDR';
-- Harus 0
```

---

### SC-02 — Validasi: Kurs Beli Lebih Besar dari Kurs Jual

**Aktor**: `uat_kurs_maker` (ROLE-AKUN)  
**Pre-kondisi**: Login sebagai Maker. Mata uang `USD` sudah APPROVED.

**Langkah-langkah**:

1. Buka menu **Master Data > Kurs > Tambah Kurs Baru**.
2. Isi form:
   - Kode Mata Uang: `USD`
   - Tanggal Berlaku: `2026-06-05`
   - Kurs Beli: `16000.0000`
   - Kurs Tengah: `15500.0000`
   - Kurs Jual: `15000.0000`
   - Sumber Kurs: `MANUAL`
3. Klik **Simpan**.

**Hasil yang diharapkan**:

- Form tidak dapat di-submit.
- Toast error merah: pesan menyebutkan `kursBeli` harus ≤ `kursTengah` dan `kursJual` harus ≥ `kursTengah`.
- HTTP `422 Unprocessable Entity`, `"code": "VALIDATION_FAILED"`.
- Detail error menyebutkan field `kursBeli` atau `kursJual`.
- Nilai numerik referensi:
  - kursBeli=16000 > kursTengah=15500 → invalid (harusnya beli ≤ tengah).
  - kursJual=15000 < kursTengah=15500 → invalid (harusnya jual ≥ tengah).

**Audit check**:

```sql
SELECT COUNT(*) FROM mst.kurs WHERE kode_mata_uang = 'USD' AND tanggal_berlaku = '2026-06-05';
-- Harus 0
```

---

### SC-03 — Happy Path: Workflow 4-Mata Penuh (DRAFT → APPROVED)

**Aktor**:
- **Maker**: `uat_kurs_maker` (ROLE-AKUN)
- **Reviewer**: `uat_kurs_reviewer` (ROLE-AKUN-CTL)
- **Approver**: `uat_kurs_approver` (ROLE-AKUN-CTL, user berbeda dari reviewer)

**Pre-kondisi**:
- Mata uang `GBP` sudah APPROVED di `mst.mata_uang`.
- Periode `PRD-2026-06` status OPEN.
- Tidak ada kurs `GBP` tanggal `2026-06-05`.

**Data uji** (dari SoW §3.9, data harian representatif):

| Field | Nilai |
|---|---|
| Kode Mata Uang | `GBP` |
| Tanggal Berlaku | `2026-06-05` |
| Kurs Beli | `20500.0000` |
| Kurs Tengah | `20540.5000` |
| Kurs Jual | `20580.0000` |
| Sumber Kurs | `MANUAL` |

**Langkah-langkah**:

**Bagian A — Maker membuat kurs (login sebagai `uat_kurs_maker`)**:

1. Buka **Master Data > Kurs > Tambah Kurs Baru**.
2. Isi form sesuai data uji di atas.
3. Verifikasi validasi lokal: kursBeli (20500) ≤ kursTengah (20540.5) ≤ kursJual (20580) → tidak ada error.
4. Klik **Simpan**.
5. Toast sukses: "Kurs GBP_20260605 berhasil dibuat. Menunggu review."

**Hasil Bagian A yang diharapkan**:
- HTTP `201 Created`.
- Record `mst.kurs` terbuat dengan `workflow_status = 'DRAFT'`.
- `sys.workflow_instance` terbuat dengan `current_state = 'DRAFT'`, `maker_id = uat_kurs_maker`.
- Audit event `KURS.CREATE` di `aud.audit_log`.

**Bagian B — Maker submit untuk review (masih login sebagai Maker)**:

6. Di daftar kurs, temukan record GBP tanggal 2026-06-05 (status DRAFT).
7. Buka detail, klik **Ajukan untuk Review**.
8. Isi komentar: `"Kurs GBP harian 5 Juni 2026"`.
9. Klik **Submit**.

**Hasil Bagian B yang diharapkan**:
- HTTP `200 OK`.
- `sys.workflow_instance.current_state = 'PENDING_REVIEW'`.
- Toast sukses: menyebutkan status "Menunggu Review".
- Audit event `KURS.SUBMIT` di `aud.audit_log`.

**Bagian C — Reviewer mereview (login sebagai `uat_kurs_reviewer`)**:

10. Buka **Antrean Review > Kurs**.
11. Temukan GBP 2026-06-05, klik **Detail**.
12. Verifikasi nilai kurs (Beli 20500, Tengah 20540.5, Jual 20580).
13. Klik **Setujui Review**.
14. Isi komentar: `"Kurs sesuai sumber manual, nilai wajar"`.
15. Klik **Review**.

**Hasil Bagian C yang diharapkan**:
- HTTP `200 OK`.
- `sys.workflow_instance.current_state = 'PENDING_APPROVAL'`.
- `sys.workflow_signature` memiliki baris dengan `signer_id = uat_kurs_reviewer`.
- Toast sukses: menyebutkan "Menunggu Approval".

**Bagian D — Approver menyetujui (login sebagai `uat_kurs_approver`)**:

> Pastikan login dengan akun yang **berbeda** dari `uat_kurs_reviewer`.

16. Buka **Antrean Approval > Kurs**.
17. Temukan GBP 2026-06-05, klik **Detail**.
18. Klik **Approve**.
19. Isi komentar: `"Approve kurs GBP harian"`.
20. Klik **Approve**.

**Hasil Bagian D yang diharapkan**:
- HTTP `200 OK`.
- `sys.workflow_instance.current_state = 'APPROVED'`.
- `mst.kurs.workflow_status = 'APPROVED'` (sync via hook).
- Toast sukses: "Kurs GBP_20260605 berhasil disetujui."
- `sys.workflow_signature`: minimal 2 baris (submit + review + approve).

**Audit check lengkap**:

```sql
-- 1. Kurs ada dan APPROVED
SELECT workflow_status, locked_flag, kurs_beli, kurs_tengah, kurs_jual
FROM mst.kurs
WHERE kode_mata_uang = 'GBP' AND tanggal_berlaku = '2026-06-05';
-- Hasil: workflow_status='APPROVED', locked_flag=false
-- kurs_beli=20500.0000, kurs_tengah=20540.5000, kurs_jual=20580.0000

-- 2. Workflow instance final
SELECT current_state, maker_id IS NOT NULL, reviewer_id IS NOT NULL
FROM sys.workflow_instance
WHERE entity_type = 'KURS'
  AND entity_id = (SELECT id FROM mst.kurs WHERE kode_mata_uang = 'GBP' AND tanggal_berlaku = '2026-06-05');
-- Hasil: APPROVED, maker ada, reviewer ada

-- 3. Audit events ada
SELECT action FROM aud.audit_log
WHERE entity_id = (SELECT id FROM mst.kurs WHERE kode_mata_uang = 'GBP' AND tanggal_berlaku = '2026-06-05')
ORDER BY event_time;
-- Harus mengandung: KURS.CREATE, KURS.SUBMIT, KURS.REVIEW, KURS.APPROVE

-- 4. Signature count
SELECT COUNT(*) FROM sys.workflow_signature ws
JOIN sys.workflow_instance wi ON ws.instance_id = wi.id
WHERE wi.entity_type = 'KURS'
  AND wi.entity_id = (SELECT id FROM mst.kurs WHERE kode_mata_uang = 'GBP' AND tanggal_berlaku = '2026-06-05');
-- Harus >= 2
```

---

### SC-04 — Validasi: Tanggal Berlaku Terlalu Jauh di Masa Depan

**Aktor**: `uat_kurs_maker` (ROLE-AKUN)

**Langkah-langkah**:

1. Buka **Master Data > Kurs > Tambah Kurs Baru**.
2. Isi form:
   - Kode Mata Uang: `USD`
   - Tanggal Berlaku: tanggal **lusa** dari hari ini (hari ini + 2 hari), misal `2026-06-07` jika hari ini `2026-06-05`.
   - Kurs Tengah: `15432.0000`
   - Sumber Kurs: `MANUAL`
3. Klik **Simpan**.

**Hasil yang diharapkan**:

- HTTP `422 Unprocessable Entity`.
- Toast error: menyebutkan `tanggalBerlaku` tidak boleh lebih dari besok.
- `"code": "VALIDATION_FAILED"`, detail field `tanggalBerlaku`, rule `max`.
- Tidak ada record baru di `mst.kurs`.

**Catatan**: Tanggal maksimum yang diizinkan adalah `today + 1 day` (untuk
kurs yang disiapkan H-1 sebelum berlaku).

---

### SC-05 — Kurs Terkunci Tidak Bisa Diedit (Periode CLOSED)

**Aktor**: `uat_kurs_maker` (ROLE-AKUN)  
**Pre-kondisi**:
- Ada record kurs `CHF` tanggal `2026-05-15` dengan `locked_flag = true`.
- Periode `PRD-2026-05` sudah CLOSED (status `CLOSED`).

**Seed pre-kondisi** (jalankan sekali sebelum skenario):

```sql
-- Pastikan CHF ada sebagai APPROVED
INSERT INTO mst.mata_uang (kode_mata_uang, ..., workflow_status)
VALUES ('CHF', ..., 'APPROVED') ON CONFLICT DO NOTHING;

-- Ambil periode_id untuk Mei 2026
SELECT id FROM mst.periode_buku WHERE periode_id_kode = 'PRD-2026-05';

-- Insert kurs CHF terkunci
INSERT INTO mst.kurs (
    fx_rate_id_kode, kode_mata_uang, tanggal_berlaku,
    kurs_tengah, sumber_kurs, periode_bulanan_id,
    locked_flag, maker_id, workflow_status,
    created_at, created_by, updated_at, updated_by, row_version, tenant_id
) VALUES (
    'CHF_20260515', 'CHF', '2026-05-15',
    16800.0000, 'BI_JISDOR', '<periode_id_PRD-2026-05>',
    true, '<system_user_id>', 'APPROVED',
    now(), '<system_user_id>', now(), '<system_user_id>', 1, 'TUGURE'
) ON CONFLICT (kode_mata_uang, tanggal_berlaku) DO UPDATE SET locked_flag = true;
```

**Langkah-langkah**:

1. Buka **Master Data > Kurs**.
2. Cari kurs `CHF` tanggal `2026-05-15`.
3. Klik **Detail**.
4. Perhatikan indikator "Terkunci" (ikon gembok / label LOCKED) di UI.
5. Klik **Edit** (atau coba ubah nilai kurs tengah).
6. Ubah Kurs Tengah menjadi `17000.0000`.
7. Klik **Simpan**.

**Hasil yang diharapkan**:

- HTTP `423 Locked`.
- Toast error merah (persistent): "Kurs ini tidak bisa diubah karena periode buku sudah CLOSED."
- `"code"` pada response: `"PERIODE_CLOSED"` atau `"KURS_LOCKED"`.
- Nilai di DB tidak berubah (`kurs_tengah` tetap `16800.0000`).
- Tombol Edit/Delete mungkin di-disable di UI jika sudah ada indikator locked.

**Audit check**:

```sql
SELECT kurs_tengah, locked_flag
FROM mst.kurs
WHERE kode_mata_uang = 'CHF' AND tanggal_berlaku = '2026-05-15';
-- Harus: kurs_tengah=16800.0000, locked_flag=true (tidak berubah)
```

**Rollback**:

```sql
-- Jika perlu cleanup setelah test:
UPDATE mst.kurs SET locked_flag = false
WHERE kode_mata_uang = 'CHF' AND tanggal_berlaku = '2026-05-15';
DELETE FROM mst.kurs WHERE kode_mata_uang = 'CHF' AND tanggal_berlaku = '2026-05-15';
```

---

### SC-06 — JISDOR Sync Stub (Phase 3)

**Aktor**: `uat_kurs_maker` (ROLE-AKUN dengan permission `kurs.jisdor_sync`)

**Langkah-langkah**:

1. Buka **Master Data > Kurs > Sinkronisasi JISDOR** (atau gunakan Postman/API client).
2. POST ke `/api/v1/master/kurs/jisdor-sync` dengan body:
   ```json
   {
     "tanggalBerlaku": "2026-06-05"
   }
   ```
3. Kirim request.

**Hasil yang diharapkan**:

- HTTP `202 Accepted` (bukan error).
- Response body berisi:
  ```json
  {
    "data": {
      "jobId": "not-implemented",
      "statusUrl": "",
      "message": "JISDOR otomatis belum tersedia (Phase 4). ... Gunakan input manual melalui POST /api/v1/master/kurs."
    }
  }
  ```
- `jobId` = `"not-implemented"` (stub Phase 3).
- Pesan mengarahkan ke input manual.
- Tidak ada kurs baru yang dibuat.
- Tidak ada job Asynq yang di-enqueue.

**Catatan**: JISDOR sync otomatis akan diimplementasikan di Phase 4.
Skenario ini memverifikasi bahwa endpoint tersedia dan memberikan respons
yang informatif (bukan 500 atau timeout).

---

## Rollback / Cleanup

Setelah semua skenario selesai, jalankan cleanup berikut di DB UAT:

```sql
-- Hapus workflow signatures dan instances untuk kurs test
DELETE FROM sys.workflow_signature ws
USING sys.workflow_instance wi
WHERE ws.instance_id = wi.id
  AND wi.entity_type = 'KURS'
  AND wi.entity_id IN (
      SELECT id FROM mst.kurs
      WHERE kode_mata_uang IN ('GBP', 'CHF', 'EUR', 'USD')
        AND tanggal_berlaku BETWEEN '2026-05-01' AND '2026-06-30'
  );

DELETE FROM sys.workflow_instance
WHERE entity_type = 'KURS'
  AND entity_id IN (
      SELECT id FROM mst.kurs
      WHERE kode_mata_uang IN ('GBP', 'CHF', 'EUR', 'USD')
        AND tanggal_berlaku BETWEEN '2026-05-01' AND '2026-06-30'
  );

-- Hapus audit log untuk kurs test (jika diizinkan di UAT env — bukan prod)
-- JANGAN jalankan di production
DELETE FROM aud.audit_log
WHERE entity_type = 'mst.kurs'
  AND entity_id IN (
      SELECT id FROM mst.kurs
      WHERE kode_mata_uang IN ('GBP', 'CHF', 'EUR', 'USD')
        AND tanggal_berlaku BETWEEN '2026-05-01' AND '2026-06-30'
  );

-- Unlock CHF sebelum delete (karena trigger melarang DELETE pada locked)
UPDATE mst.kurs SET locked_flag = false
WHERE kode_mata_uang = 'CHF' AND tanggal_berlaku = '2026-05-15';

-- Hapus kurs test
DELETE FROM mst.kurs
WHERE kode_mata_uang IN ('GBP', 'CHF', 'EUR', 'USD')
  AND tanggal_berlaku BETWEEN '2026-05-01' AND '2026-06-30'
  AND tenant_id = 'TUGURE';
```

---

## Matriks Traceability

| Skenario | AC / Business Rule | Layer Test | Regression Tag |
|---|---|---|---|
| SC-01 | IDR tidak boleh dijadikan kurs | Integration §T1 | `KURS_INVALID_CURRENCY` |
| SC-02 | beli ≤ tengah ≤ jual | Integration §T2 | `KURS_INVALID_RATES` |
| SC-03 | 4-eyes DRAFT→APPROVED, audit, SoD | Integration §T5 | SoD §6, audit §7 |
| SC-04 | tanggalBerlaku ≤ today+1 | Integration §T4 | `TANGGAL_FUTURE` |
| SC-05 | locked_flag=true → 423 | Integration §T6 | Periode close §5 |
| SC-06 | JISDOR stub Phase 3 → 202 | Integration §T7 | `JISDOR_STUB` |

---

*Dokumen ini dihasilkan oleh qa-engineer sesuai instruksi `/uat kurs`. Untuk perubahan
pada domain rules atau formula, koordinasikan dengan `ifrs9-compliance-reviewer`.*
