# UAT-APP-D-006 — Master Kurs: Input Manual, JISDOR Sync, Bulk Upload

**Modul**: APP-D — Periode Buku & FX  
**Story**: STORY-M17-02  
**AC yang diuji**: M17-02-AC1, M17-02-AC2, M17-02-AC3, M17-02-AC4  
**Tanggal dokumen**: 2026-06-25  
**Dibuat oleh**: qa-engineer  
**Status**: DRAFT

---

## 1. Pre-kondisi

| # | Kondisi | Cara memverifikasi |
|---|---|---|
| P1 | Tabel `sys.fx_rate` dapat diakses dan service berjalan | `SELECT COUNT(*) FROM sys.fx_rate` |
| P2 | Tersedia 3 user aktif: AKUN (`usr-akun-01`), AUDIT (`usr-audit-01`), MAKER-TR (`usr-maker-01`) | Keycloak → Users |
| P3 | BI JISDOR endpoint dikonfigurasi di environment (atau stub tersedia di UAT) | `GET /api/v1/fx/jisdor-status` → 200 |
| P4 | File CSV kurs bulk upload tersedia: `uat_kurs_valid.csv`, `uat_kurs_invalid.csv` | Lampiran UAT kit |
| P5 | Tidak ada kurs USD untuk tanggal `2026-06-25` (agar tidak konflik saat insert) | `SELECT * FROM sys.fx_rate WHERE kode_mata_uang='USD' AND tanggal_kurs='2026-06-25'` → 0 rows |

### Contoh data valid (`uat_kurs_valid.csv`):
```
tanggal_kurs,kode_mata_uang,nilai_kurs,sumber
2026-06-25,USD,16350.0000,MANUAL
2026-06-25,EUR,17820.5000,MANUAL
2026-06-25,SGD,12150.2500,MANUAL
```

### Contoh data invalid (`uat_kurs_invalid.csv`):
```
tanggal_kurs,kode_mata_uang,nilai_kurs,sumber
2026-06-25,INVALID_CCY,16350.0000,MANUAL
2026-06-26,USD,abc,MANUAL
```

---

## 2. Skenario Uji

### TC-006-01 — DataTable List: Sort, Filter, Chip JISDOR/MANUAL (M17-02-AC1)

**Aktor**: ROLE-AKUN (`usr-akun-01`)  
**Langkah**:

1. Login sebagai `usr-akun-01`.
2. Buka `/master/kurs`.
3. Verifikasi daftar kurs tampil dalam tabel.
4. Klik header kolom "Nilai Kurs" → urutkan ascending, lalu descending.
5. Klik filter "Sumber" → pilih "JISDOR" → verifikasi hanya baris JISDOR tampil.
6. Hapus filter → pilih "MANUAL".
7. Gunakan kolom pencarian (search global) ketik "USD".
8. Klik tombol "Export" → pilih "CSV".

**Hasil yang diharapkan**:
- [ ] Tabel tampil dengan kolom: Tanggal, Mata Uang, Nilai Kurs (IDR), Sumber, Diperbarui.
- [ ] Kolom "Sumber" menampilkan chip berwarna: JISDOR (biru), MANUAL (abu).
- [ ] Sort ascending/descending berfungsi, ikon panah tampil di header.
- [ ] Filter Sumber bekerja — baris dari sumber lain hilang.
- [ ] Pencarian "USD" hanya tampilkan baris USD.
- [ ] Export CSV berhasil — file terunduh, kolom sesuai.
- [ ] Audit log: `FX_RATE.EXPORT` terekam setelah export.

---

### TC-006-02 — Input Manual: Validasi Form + Toast Sukses (M17-02-AC2)

**Aktor**: ROLE-AKUN (`usr-akun-01`)  
**Langkah**:

1. Login sebagai `usr-akun-01`.
2. Buka `/master/kurs`.
3. Klik tombol **Input Manual**.
4. Form input muncul (drawer/dialog/halaman baru).
5. Biarkan semua field kosong → klik **Simpan**.
6. Isi: Tanggal = `2026-06-25`, Mata Uang = `USD`, Nilai Kurs = `16350`, Sumber = `MANUAL`.
7. Klik **Simpan**.

**Hasil yang diharapkan**:
- [ ] Saat field kosong: field bermasalah highlight merah + pesan inline per field.
- [ ] Toast merah: "3 field bermasalah — lihat form di bawah." (atau sesuai jumlah field kosong).
- [ ] Setelah isi valid dan Simpan: toast hijau spesifik: "Kurs USD 2026-06-25 berhasil disimpan. Nilai: Rp 16.350,0000."
- [ ] Baris baru muncul di tabel dengan chip "MANUAL".
- [ ] Request POST `/api/v1/fx/rates` membawa header `Idempotency-Key` (UUID v4).

**Verifikasi SQL**:
```sql
SELECT kode_mata_uang, nilai_kurs, sumber, created_by
FROM sys.fx_rate
WHERE tanggal_kurs = '2026-06-25' AND kode_mata_uang = 'USD';
-- Ekspektasi: 1 row, sumber = 'MANUAL', created_by = usr-akun-01
```

---

### TC-006-03 — Input Manual: Konflik Kurs Sudah Ada (M17-02-AC2)

**Aktor**: ROLE-AKUN (`usr-akun-01`)  
**Pre-kondisi**: Kurs USD `2026-06-25` sudah ada (dari TC-006-02).  
**Langkah**:

1. Klik **Input Manual**.
2. Isi: Tanggal = `2026-06-25`, Mata Uang = `USD`, Nilai = `16400`.
3. Klik **Simpan**.

**Hasil yang diharapkan**:
- [ ] Toast merah (persistent): "Kurs USD untuk 2026-06-25 sudah ada. Gunakan fitur Update untuk mengubah." dengan error code `CONFLICT`.
- [ ] Nilai di database **tidak berubah** (tetap 16350).
- [ ] Tidak ada baris duplikat di tabel.

---

### TC-006-04 — JISDOR Sync: Dialog Konfirmasi + JobProgressPanel (M17-02-AC3)

**Aktor**: ROLE-AKUN (`usr-akun-01`)  
**Langkah**:

1. Login sebagai `usr-akun-01`.
2. Buka `/master/kurs`.
3. Klik tombol **Sinkron JISDOR**.
4. Dialog konfirmasi muncul: "Sinkronisasi kurs dari BI JISDOR untuk tanggal [hari ini]?"
5. Klik **Konfirmasi**.
6. Amati respons dan progress.

**Hasil yang diharapkan**:
- [ ] Dialog konfirmasi tampil sebelum aksi.
- [ ] Setelah konfirmasi: response `202 Accepted`, `JobProgressPanel` muncul.
- [ ] Progress panel menampilkan: bar kemajuan, step saat ini ("Mengambil data JISDOR..."), ETA.
- [ ] Tombol "Batalkan" **tidak ada** pada panel (operasi JISDOR tidak dapat dibatalkan).
- [ ] Setelah selesai: toast hijau "Sinkronisasi JISDOR selesai. X mata uang diperbarui."
- [ ] Baris sumber "JISDOR" muncul/diperbarui di tabel dengan chip biru.
- [ ] Audit log: `FX_RATE.JISDOR_SYNC` terekam.

**Verifikasi SQL**:
```sql
SELECT kode_mata_uang, nilai_kurs, sumber, created_at
FROM sys.fx_rate
WHERE tanggal_kurs = CURRENT_DATE AND sumber = 'JISDOR'
ORDER BY created_at DESC;
```

---

### TC-006-05 — Bulk Upload: Format Invalid Ditolak (M17-02-AC4)

**Aktor**: ROLE-AKUN (`usr-akun-01`)  
**Langkah**:

1. Login sebagai `usr-akun-01`.
2. Buka `/master/kurs`.
3. Klik tombol **Upload**.
4. Pilih file `uat_kurs_invalid.csv`.
5. Klik **Upload**.

**Hasil yang diharapkan**:
- [ ] Validasi client-side: sebelum upload ke server, tampilkan error "Format nilai kurs tidak valid (baris 3: 'abc')".
- [ ] Atau validasi server-side: toast merah spesifik dengan detail baris yang salah.
- [ ] File **tidak disimpan** ke database.
- [ ] Tombol Upload kembali aktif (bisa coba upload lagi).

---

### TC-006-06 — Bulk Upload: File Valid → Job Progress (M17-02-AC4)

**Aktor**: ROLE-AKUN (`usr-akun-01`)  
**Pre-kondisi**: Hapus kurs EUR dan SGD untuk `2026-06-25` jika ada.  
**Langkah**:

1. Login sebagai `usr-akun-01`.
2. Klik **Upload** → pilih `uat_kurs_valid.csv`.
3. Klik **Upload**.
4. Amati progress dan hasil.

**Hasil yang diharapkan**:
- [ ] Response `202 Accepted`, `JobProgressPanel` muncul.
- [ ] Progress menampilkan: "Memproses baris 1 dari 3...", kemajuan bar.
- [ ] Setelah selesai: toast hijau "Upload selesai. 3 kurs berhasil diimpor."
- [ ] Baris USD, EUR, SGD untuk `2026-06-25` muncul di tabel.
- [ ] Audit log: `FX_RATE.BULK_IMPORT` terekam dengan `row_count: 3`.

---

### TC-006-07 — Gating: ROLE-AUDIT Tidak Ada Tombol Mutasi (M17-02-AC3)

**Aktor**: ROLE-AUDIT (`usr-audit-01`)  
**Langkah**:

1. Login sebagai `usr-audit-01`.
2. Buka `/master/kurs`.

**Hasil yang diharapkan**:
- [ ] Tabel kurs tampil (read access OK).
- [ ] Tombol "Input Manual", "Sinkron JISDOR", "Upload" **tidak ada di DOM** (bukan hanya disabled).
- [ ] Tombol "Export" tersedia (read + export OK untuk AUDIT).
- [ ] Browser DevTools → tidak ada elemen tersembunyi dengan teks "Input Manual", "Sinkron JISDOR".

---

### TC-006-08 — Gating: ROLE-MAKER-TR Tidak Dapat Akses /master/kurs (M17-02-AC3)

**Aktor**: ROLE-MAKER-TR (`usr-maker-01`)  
**Langkah**:

1. Login sebagai `usr-maker-01`.
2. Navigasi ke `/master/kurs`.

**Hasil yang diharapkan**:
- [ ] Halaman redirect ke `/404` atau `/unauthorized`.
- [ ] Tidak ada data kurs yang terekspos.

---

## 3. Verifikasi Audit Trail

```sql
SELECT action, actor_user_id, after_jsonb, event_time
FROM aud.audit_log
WHERE entity_type = 'sys.fx_rate'
ORDER BY event_time DESC
LIMIT 20;
```

**Aksi yang diharapkan**: `FX_RATE.CREATE`, `FX_RATE.JISDOR_SYNC`, `FX_RATE.BULK_IMPORT`, `FX_RATE.EXPORT`

---

## 4. Rollback / Cleanup

```sql
-- Hapus data uji kurs
DELETE FROM sys.fx_rate
WHERE tanggal_kurs = '2026-06-25'
  AND created_by IN ('usr-akun-01')
  AND sumber = 'MANUAL';
-- Audit entries cleanup (UAT ONLY)
DELETE FROM aud.audit_log
WHERE entity_type = 'sys.fx_rate' AND trace_id LIKE 'uat-%';
```

---

## 5. Sign-Off

| Peran | Nama | Tanggal | Hasil | Tanda tangan |
|---|---|---|---|---|
| QA Tester | | | PASS / FAIL | |
| ROLE-AKUN (UAT Actor) | | | PASS / FAIL | |
| ROLE-AUDIT (UAT Actor) | | | PASS / FAIL | |
| Product Owner | | | APPROVED / REJECT | |
