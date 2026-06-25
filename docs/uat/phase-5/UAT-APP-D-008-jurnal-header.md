# UAT-APP-D-008 — Jurnal Header: 4-Eyes Workflow + GL Posting

**Modul**: APP-D — Jurnal Engine  
**Story**: STORY-M17-04  
**AC yang diuji**: M17-04-AC1, M17-04-AC2  
**Tanggal dokumen**: 2026-06-25  
**Dibuat oleh**: qa-engineer  
**Status**: DRAFT

---

## 1. Pre-kondisi

| # | Kondisi | Cara memverifikasi |
|---|---|---|
| P1 | Periode buku Juni 2026 dalam status `OPEN` | `SELECT status_close FROM sys.periode WHERE bulan='2026-06-01'` → `OPEN` |
| P2 | Mapping jurnal `DEPOSITO_INT` dalam status `ACTIVE` | `SELECT status_workflow FROM jrnl.mapping_jurnal WHERE kode_jurnal='DEPOSITO_INT'` → `ACTIVE` |
| P3 | Tersedia 3 user aktif untuk SoD | Tabel user P3 di bawah |
| P4 | GL Host stub berjalan (UAT mock endpoint) | `GET /api/v1/gl/health` → 200 |

**Tabel user SoD (P3)**:

| User ID | Peran | Langkah |
|---|---|---|
| `usr-akun-01` | ROLE-AKUN | Maker: buat + submit jurnal |
| `usr-akunctl-01` | ROLE-AKUN-CTL | Approver: approve + trigger post |
| `usr-audit-01` | ROLE-AUDIT | Observer: baca saja |

---

## 2. Data Uji

```
Nomor Jurnal (auto)  : JRN-2026-XXXX (auto-generate)
Tanggal Jurnal       : 2026-06-25
Periode              : Juni 2026
Kode Mapping         : DEPOSITO_INT
Keterangan           : Akrual bunga deposito BCA Rp 10.000.000
Akun Debit           : 113.101.001 — Pendapatan Bunga Deposito
Akun Kredit          : 311.001.001 — Akrual Bunga Deposito
Jumlah               : Rp 10.000.000,0000
Mata Uang            : IDR
```

---

## 3. Skenario Uji

### TC-008-01 — DataTable Jurnal Header: Sort + Filter + Status Chip (M17-04-AC1)

**Aktor**: ROLE-AKUN (`usr-akun-01`)  
**Langkah**:

1. Login sebagai `usr-akun-01`.
2. Buka `/jurnal/header`.
3. Verifikasi tab "Header Jurnal" aktif.
4. Amati daftar jurnal yang ada.
5. Klik header kolom "Tanggal Jurnal" → urutkan descending.
6. Filter "Status" → pilih "DRAFT".
7. Klik salah satu baris untuk membuka detail.

**Hasil yang diharapkan**:
- [ ] Tabel tampil: Nomor Jurnal, Tanggal, Kode Mapping, Keterangan, Status, Jumlah, Dibuat Oleh.
- [ ] Status chip warna sesuai: DRAFT (abu), SUBMITTED (kuning), APPROVED (biru), POSTED_TO_GL (hijau).
- [ ] Sort bekerja, ikon panah muncul di header.
- [ ] Filter status DRAFT berfungsi.
- [ ] Klik baris → halaman detail `/jurnal/header/{id}` terbuka.

---

### TC-008-02 — Buat Jurnal Baru + Submit (DRAFT → SUBMITTED) (M17-04-AC1)

**Aktor**: ROLE-AKUN (`usr-akun-01`)  
**Langkah**:

1. Login sebagai `usr-akun-01`.
2. Buka `/jurnal/header` → klik **Buat Jurnal**.
3. Isi form:
   - Tanggal: `2026-06-25`
   - Kode Mapping: `DEPOSITO_INT`
   - Keterangan: "Akrual bunga deposito BCA Rp 10.000.000"
   - Line item: Debit `113.101.001` Rp 10.000.000, Kredit `311.001.001` Rp 10.000.000
4. Klik **Simpan sebagai Draft**.
5. Toast muncul → klik **Lihat Detail** dari link di toast.
6. Klik **Submit untuk Approval**.

**Hasil yang diharapkan**:
- [ ] Toast hijau: "Jurnal JRN-2026-XXXX berhasil disimpan sebagai Draft." + link detail.
- [ ] Setelah Submit: toast hijau: "Jurnal JRN-2026-XXXX berhasil di-submit. Menunggu approval Finance Controller."
- [ ] Status → `SUBMITTED`.
- [ ] Panel 4-eyes: node Submit aktif.
- [ ] POST `/api/v1/jurnal/header` dan POST `/api/v1/jurnal/header/{id}/submit` membawa `Idempotency-Key`.

**Verifikasi SQL**:
```sql
SELECT id, nomor_jurnal, status_workflow, maker_id
FROM jrnl.jurnal_header
WHERE tanggal_jurnal = '2026-06-25' AND kode_mapping = 'DEPOSITO_INT'
ORDER BY created_at DESC LIMIT 1;
-- Simpan id sebagai :jurnal_id
```

---

### TC-008-03 — Approve + Post ke GL (SUBMITTED → APPROVED → POSTED_TO_GL) (M17-04-AC2)

**Aktor**: ROLE-AKUN-CTL (`usr-akunctl-01`)  
**Pre-kondisi**: Jurnal dalam status `SUBMITTED`.  
**Langkah**:

1. Login sebagai `usr-akunctl-01`.
2. Buka detail jurnal (status `SUBMITTED`).
3. Klik **Approve**.
4. Isi comment: "Jurnal sesuai mapping aktif DEPOSITO_INT."
5. Klik **Konfirmasi Approve**.
6. Amati status berubah dan toast.
7. Setelah `APPROVED`, klik **Post ke GL** (jika ada tombol terpisah) atau approve sudah auto-trigger post.

**Hasil yang diharapkan**:
- [ ] Toast hijau: "Jurnal JRN-2026-XXXX berhasil di-approve."
- [ ] Status → `APPROVED`, lalu (setelah GL sync) → `POSTED_TO_GL`.
- [ ] Panel 4-eyes: node Approve aktif dengan nama `usr-akunctl-01`.
- [ ] Signature hash tersimpan.
- [ ] Setelah POSTED_TO_GL: **tidak ada tombol** workflow apapun (Approve, Submit, Reject, Post) di halaman detail.

**Verifikasi SQL**:
```sql
SELECT status_workflow, approver_id, approver_signed_at, gl_post_at, gl_reference_number
FROM jrnl.jurnal_header
WHERE id = :'jurnal_id';
-- Ekspektasi: POSTED_TO_GL, approver_id = usr-akunctl-01, gl_post_at IS NOT NULL
```

**Audit check**:
```sql
SELECT action, actor_user_id, after_jsonb->>'status_workflow', event_time
FROM aud.audit_log
WHERE entity_type = 'jrnl.jurnal_header' AND entity_id = :'jurnal_id'
ORDER BY event_time;
```

---

### TC-008-04 — SoD: Maker Tidak Bisa Approve (M17-04-AC2)

**Aktor**: ROLE-AKUN (`usr-akun-01`) — maker jurnal  
**Pre-kondisi**: Jurnal dibuat oleh `usr-akun-01` dalam status `SUBMITTED`.  
**Langkah**:

1. Login sebagai `usr-akun-01`.
2. Buka detail jurnal yang dia buat.
3. Verifikasi tombol **Approve** tidak ada di DOM.
4. Kirim POST langsung via DevTools:
   ```
   POST /api/v1/jurnal/header/{jurnal_id}/approve
   Authorization: Bearer <token usr-akun-01>
   Idempotency-Key: <uuid>
   Content-Type: application/json
   { "comment": "bypass" }
   ```

**Hasil yang diharapkan**:
- [ ] Tombol "Approve" tidak ada di DOM untuk usr-akun-01 pada jurnal buatannya.
- [ ] API response: `403 FORBIDDEN`, `{ "error": { "code": "SOD_VIOLATION" } }`.
- [ ] Status jurnal tetap `SUBMITTED`.

---

### TC-008-05 — Jurnal POSTED_TO_GL Tidak Dapat Dimodifikasi (M17-04-AC2)

**Aktor**: ROLE-AKUN (`usr-akun-01`)  
**Pre-kondisi**: Jurnal sudah `POSTED_TO_GL`.  
**Langkah**:

1. Buka detail jurnal yang sudah `POSTED_TO_GL`.
2. Amati halaman detail.
3. Coba kirim PATCH/POST workflow ke jurnal tersebut via DevTools.

**Hasil yang diharapkan**:
- [ ] Halaman detail: read-only. Tidak ada form edit, tombol Submit, Approve.
- [ ] API PATCH: `423 LOCKED` atau `422 WORKFLOW_INVALID_TRANSITION`.
- [ ] Jurnal lines tampil (detail debit/kredit beserta jumlah).

---

### TC-008-06 — Export Daftar Jurnal (M17-04-AC1)

**Aktor**: ROLE-AKUN-CTL (`usr-akunctl-01`)  
**Langkah**:

1. Login sebagai `usr-akunctl-01`.
2. Buka `/jurnal/header`.
3. Set filter: Status = `POSTED_TO_GL`, Tanggal = `2026-06-25`.
4. Klik **Export** → pilih **CSV**.
5. Verifikasi file terunduh.

**Hasil yang diharapkan**:
- [ ] File CSV terunduh dengan nama `jurnal-header-20260625.csv`.
- [ ] Kolom: Nomor Jurnal, Tanggal, Kode Mapping, Keterangan, Status, Jumlah IDR, GL Reference, Dibuat Oleh.
- [ ] Hanya baris sesuai filter yang masuk (bukan semua data).
- [ ] Audit log: `JURNAL_HEADER.EXPORT` terekam.

---

## 4. Verifikasi Audit Trail

```sql
SELECT action, actor_user_id, after_jsonb->>'status_workflow', event_time
FROM aud.audit_log
WHERE entity_type = 'jrnl.jurnal_header'
ORDER BY event_time DESC LIMIT 10;
```

---

## 5. Rollback / Cleanup

```sql
-- CATATAN: POSTED_TO_GL tidak bisa dihapus (no hard delete)
-- Untuk UAT cleanup, set deleted_at jika jurnal masih DRAFT
UPDATE jrnl.jurnal_header
SET deleted_at = now(), deleted_by = 'uat-cleanup'
WHERE created_by = 'usr-akun-01'
  AND tanggal_jurnal = '2026-06-25'
  AND status_workflow = 'DRAFT';
```

---

## 6. Sign-Off

| Peran | Nama | Tanggal | Hasil | Tanda tangan |
|---|---|---|---|---|
| QA Tester | | | PASS / FAIL | |
| ROLE-AKUN (Maker UAT) | | | PASS / FAIL | |
| ROLE-AKUN-CTL (Approver UAT) | | | PASS / FAIL | |
| Product Owner | | | APPROVED / REJECT | |
