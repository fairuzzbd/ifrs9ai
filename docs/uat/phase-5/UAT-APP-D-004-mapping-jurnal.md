# UAT-APP-D-004 — Mapping Jurnal CRUD + 6-Eyes Workflow + RPT-19/20/21

**Modul**: APP-D  
**Story Set**: P5-M12 (S1–S5)  
**Tanggal UAT**: 2026-06-22  
**Penyusun**: qa-engineer  
**Gate**: security-engineer BLOCKING (SoD, MFA, audit); ifrs9-compliance-reviewer BLOCKING (regulated mapping)

---

## Pre-Kondisi Global

1. Lingkungan UAT berjalan (`docker compose -f deploy/docker-compose.uat.yml up -d`)
2. `mst.mapping_jurnal_header`: 27 event code seeds (EVT-001..027) status DRAFT dari P5-M2
3. `mst.chart_of_accounts`: kode akun 110201, 440101, 220301 ter-populate
4. `mst.periode_buku` PRD-2026-06: `status_periode = 'OPEN'`
5. `sys.config` key `MAPPING_REGULATED_EVENT_CODES`: berisi 13 kode regulated
6. User test sesuai tabel di bawah, semua ter-autentikasi via Keycloak SSO:

| User ID        | Role           | MFA |
|----------------|----------------|-----|
| USR-AKUN-001   | ROLE-AKUN      | Tidak |
| USR-AKUN-CTL-001 | ROLE-AKUN-CTL | Ya |
| USR-AKUN-CTL-002 | ROLE-AKUN-CTL | Ya |
| USR-RISK-001   | ROLE-RISK      | Ya + step-up |
| USR-AUDIT-001  | ROLE-AUDIT     | Tidak |

---

## TC-001 — List DataTable: Sort, Filter, Export

**Actor**: USR-AKUN-001  
**Pre-kondisi**: 27 headers seeded  

**Langkah**:
1. Buka `/mapping-jurnal`
2. Klik header kolom **event_code** untuk sort ascending
3. Aktifkan filter `workflow_status = DRAFT`
4. Klik **Export** → pilih **CSV**

**Hasil yang Diharapkan**:
- Tabel menampilkan hanya rows dengan `workflow_status = DRAFT`, diurutkan ascending berdasarkan `event_code`
- Icon panah ↑ tampil di kolom event_code
- File CSV terunduh, nama file `mapping-jurnal-YYYYMMDD.csv`
- Baris header CSV: Bahasa Indonesia (`Kode Event`, `Nama Event`, `Status`, `Path Workflow`, dst.)
- `aud.audit_log`: action = `MAPPING.EXPORT`, `after_jsonb.format = 'csv'`, in-transaction
- URL deep-link: `/mapping-jurnal?sort=event_code:asc&filter[workflow_status]=DRAFT` (bookmarkable)

---

## TC-002 — Detail View Event + Version History

**Actor**: USR-AKUN-001  
**Pre-kondisi**: PENEMPATAN status APPROVED_ACTIVE  

**Langkah**:
1. Klik event code **PENEMPATAN** di DataTable

**Hasil yang Diharapkan**:
- Halaman detail tampil: `event_code = PENEMPATAN`, `workflow_status = APPROVED_ACTIVE`, `aktif_flag = true`
- Section detail: `akun_debit`, `akun_kredit`, `debit_kredit` (D/K), `jumlah_calc` per baris
- Section **Riwayat Versi**: minimal 1 versi dengan `effective_from`, `effective_to`
- Badge `APPROVED_ACTIVE` tampil dengan contrast ratio WCAG AA ≥ 4.5:1
- Tombol **Edit** tersedia untuk USR-AKUN-001

---

## TC-003 — Tambah Detail Row pada Mapping DRAFT

**Actor**: USR-AKUN-001  
**Pre-kondisi**: ECL_PEMBENTUKAN status DRAFT, belum ada detail row  

**Langkah**:
1. Buka detail event **ECL_PEMBENTUKAN**
2. Klik **Tambah Baris Detail**
3. Isi: `akun_debit = 110201`, `akun_kredit = 440101`, `debit_kredit = D`, `jumlah_calc = ECL_weighted`, `urutan = 1`
4. Klik **Simpan** (sertakan `Idempotency-Key: IK-DETAIL-001`)

**Hasil yang Diharapkan**:
- HTTP 201: `data.header_id = ECL-HEADER-ID`, `data.akun_debit = 110201`
- `mst.mapping_jurnal_detail` INSERT: akun divalidasi ke COA
- `aud.audit_log`: action = `MAPPING.DETAIL_CREATED`, `after_jsonb.header_id`, `actor: USR-AKUN-001`, in-transaction
- Toast hijau: "Detail mapping ECL_PEMBENTUKAN (baris 1) berhasil ditambahkan. Status: DRAFT."

---

## TC-004 — Buat Versi Baru dari Mapping APPROVED_ACTIVE

**Actor**: USR-AKUN-001  
**Pre-kondisi**: PENEMPATAN status APPROVED_ACTIVE  

**Langkah**:
1. Buka detail event **PENEMPATAN** → klik **Edit**
2. Ubah kode akun debit menjadi `110101`
3. Isi reason: "Perubahan kode akun kas sesuai COA baru per Juli 2026"
4. Klik **Simpan Versi Baru** (sertakan `Idempotency-Key: IK-VERSION-001`)

**Hasil yang Diharapkan**:
- HTTP 201: `data.parent_id = PENEMPATAN-V1-ID`, `data.workflow_status = DRAFT`
- Versi lama (`effective_to` diisi timestamp): aktif_flag tetap TRUE
- Versi lama TIDAK di-UPDATE status (`APPROVED_ACTIVE` immutable — hanya `effective_to` yang boleh berubah)
- `aud.audit_log`: action = `MAPPING.VERSION_CREATED`, `after_jsonb.parent_id`, in-transaction
- Toast hijau: "Versi baru mapping PENEMPATAN berhasil dibuat (DRAFT). Versi lama aktif sampai versi baru disetujui."

---

## TC-005 — 6-Eyes Full Flow: DRAFT → PENDING_REVIEW → PENDING_APPROVAL_2 → APPROVED_ACTIVE

**Actor**: USR-AKUN-001 (maker), USR-AKUN-CTL-001 (reviewer), USR-RISK-001 (approver-2)  
**Pre-kondisi**: ECL_PEMBENTUKAN DRAFT, detail rows ter-isi, `workflow_path = 6-eyes`  

**Langkah**:
1. USR-AKUN-001: `POST /api/v1/mapping-jurnal/ECL_PEMBENTUKAN/version/{id}/submit` — comment ≥ 1 karakter
2. USR-AKUN-CTL-001: `POST /api/v1/mapping-jurnal/ECL_PEMBENTUKAN/version/{id}/review` — comment ≥ 30 karakter, `signatureMethod: JWT_STEP_UP`
3. USR-RISK-001: `POST /api/v1/mapping-jurnal/ECL_PEMBENTUKAN/version/{id}/approve-2` — sertakan `X-Step-Up-Token`, comment ≥ 10 karakter

**Hasil yang Diharapkan**:
- Setelah step 1: `workflow_status = PENDING_REVIEW`, audit `MAPPING.SUBMITTED`
- Setelah step 2: `workflow_status = PENDING_APPROVAL_2`, `reviewer_id = USR-AKUN-CTL-001`, `reviewer_signature_hash` ter-isi
- Setelah step 3: `workflow_status = APPROVED_ACTIVE`, `aktif_flag = TRUE`, `approver_2_id = USR-RISK-001`, `approver_2_signature_hash` ter-isi
- `aud.audit_log` berisi `MAPPING.APPROVED_ACTIVE` dengan `after_jsonb.mfa_method = 'TOTP'`
- Toast: "Mapping ECL_PEMBENTUKAN disetujui dan aktif. Resolver dapat menggunakan template ini."

---

## TC-006 — 4-Eyes Flow (Non-Regulated): PENDING_APPROVAL → APPROVED_ACTIVE

**Actor**: USR-AKUN-CTL-002 (approver)  
**Pre-kondisi**: PENEMPATAN DRAFT v2 sudah di-review oleh USR-AKUN-CTL-001, status PENDING_APPROVAL  

**Langkah**:
1. USR-AKUN-CTL-002: `POST /approve` dengan comment "Verified — akun penempatan sesuai buku besar", `signatureMethod: JWT_STEP_UP`

**Hasil yang Diharapkan**:
- HTTP 200: `workflow_status = APPROVED_ACTIVE`, `aktif_flag = TRUE`
- `approver_id = USR-AKUN-CTL-002` (≠ reviewer USR-AKUN-CTL-001 ≠ maker USR-AKUN-001 — SoD OK)
- Versi lama PENEMPATAN: `effective_to = now()` (atomic flip dalam satu transaksi DB)
- `aud.audit_log`: action = `MAPPING.APPROVED_ACTIVE`, in-transaction

---

## TC-007 — SoD Violation: Reviewer Mencoba Menjadi Approver

**Actor**: USR-AKUN-CTL-001 (sudah menjadi reviewer)  
**Pre-kondisi**: PENEMPATAN DRAFT v2 status PENDING_APPROVAL, reviewer_id = USR-AKUN-CTL-001  

**Langkah**:
1. USR-AKUN-CTL-001: `POST /approve` dengan `Idempotency-Key: IK-APPR-SOD-001`

**Hasil yang Diharapkan**:
- HTTP 403: `error.code = MAPPING_SOD_VIOLATION`
- `error.message`: "SoD: reviewer tidak dapat menjadi approver untuk mapping yang sama (DEC-017)."
- `workflow_status` tidak berubah
- `aud.audit_log`: action = `MAPPING.SOD_VIOLATION_ATTEMPT`, in-transaction
- Tidak ada perubahan pada `mst.mapping_jurnal_header`

---

## TC-008 — SoD Violation: Maker Mencoba Menjadi Approver (via API langsung)

**Actor**: USR-AKUN-001 (maker)  
**Pre-kondisi**: PENEMPATAN DRAFT v2 status PENDING_APPROVAL, maker_id = USR-AKUN-001  

**Langkah**:
1. USR-AKUN-001 kirim `POST /approve` langsung via API (bypass UI)

**Hasil yang Diharapkan**:
- HTTP 403: `error.code = MAPPING_SOD_VIOLATION`
- Server-side enforcement (bukan hanya UI) terbukti memblok
- `aud.audit_log`: action = `MAPPING.SOD_VIOLATION_ATTEMPT`

---

## TC-009 — Approve-2 Tanpa Step-Up MFA → FORBIDDEN 403

**Actor**: USR-RISK-001  
**Pre-kondisi**: ECL_PEMBENTUKAN status PENDING_APPROVAL_2  

**Langkah**:
1. USR-RISK-001: `POST /approve-2` TANPA header `X-Step-Up-Token`

**Hasil yang Diharapkan**:
- HTTP 403: `error.code = FORBIDDEN`
- `error.message`: "Approve-2 mapping regulated memerlukan step-up MFA (DEC-027). Re-autentikasi MFA."
- `workflow_status` tidak berubah

---

## TC-010 — Approve-2 dengan Step-Up Token Expired → FORBIDDEN 403

**Actor**: USR-RISK-001  

**Langkah**:
1. Gunakan `X-Step-Up-Token` yang di-issue > 5 menit yang lalu

**Hasil yang Diharapkan**:
- HTTP 403: `error.code = FORBIDDEN`, pesan menyebut token expired

---

## TC-011 — Approve saat Periode HARD_CLOSED → MAPPING_PERIODE_LOCKED 423

**Actor**: USR-AKUN-CTL-002  
**Pre-kondisi**: Set `mst.periode_buku.status_periode = 'HARD_CLOSED'`  

**Langkah**:
1. USR-AKUN-CTL-002: `POST /approve` untuk mapping PENDING_APPROVAL

**Hasil yang Diharapkan**:
- HTTP 423: `error.code = MAPPING_PERIODE_LOCKED`
- `error.message`: "Periode buku HARD_CLOSED. Perubahan mapping tidak dapat diaktifkan di periode ini."
- Submit (DRAFT → PENDING_REVIEW) tetap diperbolehkan saat HARD_CLOSED

---

## TC-012 — Reject + Kembali ke DRAFT

**Actor**: USR-AKUN-CTL-001 (reviewer)  
**Pre-kondisi**: ECL_PEMBENTUKAN status PENDING_REVIEW  

**Langkah**:
1. USR-AKUN-CTL-001: `POST /reject` dengan reason ≥ 30 karakter
   - `reason: "Kode akun kredit salah, harusnya 220301 bukan 440101. Mohon diperbaiki."`

**Hasil yang Diharapkan**:
- HTTP 200: `workflow_status = DRAFT`
- `reject_reason` tersimpan di header
- `aud.audit_log`: action = `MAPPING.REJECTED`, in-transaction
- Toast ke maker: "Mapping ECL_PEMBENTUKAN dikembalikan ke DRAFT. Alasan: Kode akun kredit salah..."

---

## TC-013 — Export XLSX Mapping APPROVED_ACTIVE

**Actor**: USR-AKUN-001  

**Langkah**:
1. `GET /api/v1/mapping-jurnal/export?format=xlsx&filter[workflow_status]=APPROVED_ACTIVE`

**Hasil yang Diharapkan**:
- HTTP 200, `Content-Disposition: attachment; filename="mapping-jurnal-YYYYMMDD.xlsx"`
- Sheet "Header": event_code, nama_event, workflow_path, aktif_flag
- Sheet "Detail": event_code FK, akun_debit, akun_kredit, debit_kredit, formula
- Header baris: Bahasa Indonesia
- Hanya ACTIVE rows yang di-export (filter dihormati)
- `aud.audit_log`: action = `MAPPING.EXPORT`, in-transaction

---

## TC-014 — Bulk Import XLSX: 5 Baris (2 Valid, 2 Unbalanced, 1 Invalid Akun)

**Actor**: USR-AKUN-001  
**Pre-kondisi**: COA 110201, 440101, 220301 tersedia; akun 999999 tidak ada  

**Langkah**:
1. Buat file `mapping_bulk_test.xlsx` dengan 5 baris sesuai format
2. `POST /api/v1/mapping-jurnal/bulk-import` dengan file + `Idempotency-Key: IK-IMPORT-001`

**Data test**:
| Row | event_code | akun_debit | akun_kredit | debit_kredit |
|-----|-----------|-----------|------------|-------------|
| 1 | ECL_PEMBENTUKAN | 110201 | 440101 | D |
| 2 | ECL_PEMBENTUKAN | 440101 | 110201 | K |
| 3 | MTM_FVOCI | 110201 | 440101 | D |
| 4 | MTM_FVOCI | 110201 | 440101 | D |
| 5 | PENEMPATAN | 999999 | 440101 | D |

**Hasil yang Diharapkan**:
- HTTP 202: `data.batch_type = MAPPING_BULK`, `data.total_rows = 5`
- `data.valid_rows = 2` (rows 1, 2)
- `data.invalid_rows = 3` (rows 3, 4 unbalanced; row 5 invalid akun)
- `data.errors[row=5].error_code = MAPPING_AKUN_INVALID`, mengandung "999999"
- Rows 1, 2: DRAFT version di-INSERT ke `mst.mapping_jurnal_header`
- Rows 3, 4: `sys.upload_batch_row.row_status = FAILED`, tidak di-INSERT
- `aud.audit_log`: action = `MAPPING.BULK_IMPORTED`, in-transaction
- Toast: "Import mapping berhasil diparsing. 2 baris valid (DRAFT dibuat), 3 baris gagal."

---

## TC-015 — Idempotency Replay pada Bulk Import

**Actor**: USR-AKUN-001  

**Langkah**:
1. Kirim bulk import pertama dengan `Idempotency-Key: IK-IMPORT-002`
2. Kirim ulang request identik dengan `Idempotency-Key: IK-IMPORT-002`

**Hasil yang Diharapkan**:
- Request kedua: HTTP 200 dengan response body identik request pertama (`IDEMPOTENCY_REPLAY`)
- Tidak ada INSERT baru ke `mst.mapping_jurnal_header` atau `sys.upload_batch`
- Tidak ada audit event duplikat

---

## TC-016 — RPT-19: Coverage Summary Dashboard

**Actor**: USR-AKUN-001  
**Pre-kondisi**: 3 event APPROVED_ACTIVE, 24 event DRAFT/PENDING  

**Langkah**:
1. `GET /api/v1/reports/rpt-19-mapping-coverage`

**Hasil yang Diharapkan**:
- HTTP 200:
  - `data.total_events ≥ 27`
  - `data.active_events ≥ 3`
  - `data.missing_events ≥ 24`
  - `data.gap_events[].gap_coverage`: MISSING untuk event tanpa APPROVED_ACTIVE
- Badge merah (MISSING), kuning (INCOMPLETE), hijau (OK) tampil dengan WCAG AA contrast
- Link tiap event → `/mapping-jurnal?filter[event_code]=...`

---

## TC-017 — RPT-19: APPROVED_ACTIVE dengan Akun Null = INCOMPLETE

**Pre-kondisi**: Satu APPROVED_ACTIVE header memiliki 1 detail row dengan `akun_debit = NULL`  

**Langkah**:
1. `GET /api/v1/reports/rpt-19-mapping-coverage`

**Hasil yang Diharapkan**:
- Event tersebut tampil di `gap_events[]` dengan `gap_coverage = INCOMPLETE`
- Alasan: "1 detail row dengan akun_debit null di version APPROVED_ACTIVE"
- Badge kuning (INCOMPLETE)

---

## TC-018 — RPT-20: Validation Report — Akun Null + Unbalanced Terdeteksi

**Actor**: USR-RISK-001  

**Langkah**:
1. `GET /api/v1/reports/rpt-20-mapping-validation`

**Hasil yang Diharapkan**:
- HTTP 200:
  - `data.issues[]` berisi event dengan `MAPPING_AKUN_INVALID` dan/atau `MAPPING_UNBALANCED`
  - Tiap issue: link → `/mapping-jurnal/{header_id}` per-event detail view
- ROLE-RISK dapat pakai RPT-20 sebagai pre-checklist sebelum approve-2
- `aud.audit_log`: action = `MAPPING.RPT20_EXPORTED` saat export di-klik, in-transaction

---

## TC-019 — RPT-21: Audit History Filter per Event Code

**Actor**: USR-AUDIT-001  

**Langkah**:
1. `GET /api/v1/reports/rpt-21-mapping-history?filter[event_code]=ECL_PEMBENTUKAN&sort=event_time:desc`

**Hasil yang Diharapkan**:
- HTTP 200, cursor-paged sesuai §1
- `data[].action`: urutan desc `MAPPING.APPROVED_ACTIVE` → `MAPPING.REVIEWED` → `MAPPING.SUBMITTED`
- `data[].actor_role`: ROLE-RISK, ROLE-AKUN-CTL, ROLE-AKUN
- `data[].before_jsonb`: null untuk MAPPING.SUBMITTED (CREATE); non-null untuk selanjutnya
- Filter berjalan: hanya MAPPING.* actions untuk ECL_PEMBENTUKAN
- USR-AUDIT-001: tidak ada tombol mutasi di UI (read-only confirmed)

---

## TC-020 — RPT-21 Export > 10k Rows → Async Job

**Actor**: USR-AUDIT-001  
**Pre-kondisi**: `aud.audit_log` memiliki > 10.000 MAPPING.* entries  

**Langkah**:
1. `GET /api/v1/reports/rpt-21-mapping-history/export?format=csv`

**Hasil yang Diharapkan**:
- HTTP 202: `data.jobId`, `data.statusUrl`
- Asynq worker stream CSV ke MinIO bucket `exports/`
- Notifikasi download link muncul setelah job selesai (TTL 24 jam)
- Toast: "RPT-21 export selesai. [N] baris siap diunduh (TTL 24 jam)."
- `aud.audit_log`: action = `MAPPING.RPT21_EXPORTED` saat job complete, in-transaction

---

## Audit Checks (Semua TC)

Untuk setiap skenario di atas, verifikasi:

| Cek | Keterangan |
|-----|-----------|
| `aud.audit_log` row ada | Setiap mutasi harus menghasilkan audit row |
| In-transaction | Audit ditulis dalam transaksi DB yang sama dengan mutasi bisnis (tidak boleh di luar) |
| `after_jsonb` non-null | Untuk semua CREATE/UPDATE/APPROVE actions |
| Hash-chain valid | `current_hash = SHA-256(previous_hash || canonical_json(row))` |
| `tenant_id = 'TUGURE'` | Setiap row `mst.mapping_jurnal_header`, `mst.mapping_jurnal_detail`, `aud.audit_log` |
| SoD di-enforce server-side | Test SoD bypass via API langsung (TC-008 merupakan contoh) |

---

## Rollback / Cleanup

```sql
-- Reset ECL_PEMBENTUKAN ke DRAFT untuk re-run
UPDATE mst.mapping_jurnal_header
SET workflow_status = 'DRAFT', aktif_flag = FALSE,
    maker_id = NULL, reviewer_id = NULL, approver_2_id = NULL,
    reviewer_signed_at = NULL, approver_2_signed_at = NULL
WHERE event_code = 'ECL_PEMBENTUKAN' AND tenant_id = 'TUGURE';

-- Hapus batch import test
DELETE FROM sys.upload_batch WHERE batch_type = 'MAPPING_BULK' AND tenant_id = 'TUGURE';

-- Reset periode buku ke OPEN
UPDATE mst.periode_buku SET status_periode = 'OPEN' WHERE id = 'PRD-2026-06' AND tenant_id = 'TUGURE';

-- Hapus versi baru PENEMPATAN (parent_id non-null)
DELETE FROM mst.mapping_jurnal_header WHERE event_code = 'PENEMPATAN' AND parent_id IS NOT NULL AND tenant_id = 'TUGURE';
```

> Audit log TIDAK dihapus (immutable, DEC-018). Baris audit dari UAT tetap ada untuk trail.
