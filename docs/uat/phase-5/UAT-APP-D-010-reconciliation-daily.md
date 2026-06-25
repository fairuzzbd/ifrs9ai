# UAT-APP-D-010 — Rekonsiliasi Harian: KPI Cards, Mismatch List, Export

**Modul**: APP-D — Rekonsiliasi GL  
**Story**: STORY-M17-05  
**AC yang diuji**: M17-05-AC1, M17-05-AC2, M17-05-AC3, M17-05-AC4  
**Tanggal dokumen**: 2026-06-25  
**Dibuat oleh**: qa-engineer  
**Status**: DRAFT

---

## 1. Pre-kondisi

| # | Kondisi | Cara memverifikasi |
|---|---|---|
| P1 | Data rekonsiliasi untuk tanggal `2026-06-25` tersedia di database | `SELECT COUNT(*) FROM jrnl.rekon_daily WHERE tanggal='2026-06-25'` → ≥ 1 |
| P2 | Data mismatch untuk `2026-06-25` tersedia (minimal 3 baris, jenis berbeda) | `SELECT COUNT(*) FROM jrnl.rekon_mismatch WHERE tanggal='2026-06-25'` → ≥ 3 |
| P3 | Tersedia 3 user aktif | Tabel user P3 di bawah |
| P4 | Layar `/reconciliation/daily` dapat diakses | Buka URL, pastikan tidak 404 |

**Seed data rekonsiliasi jika belum ada**:
```sql
-- UAT ONLY
INSERT INTO jrnl.rekon_daily (tanggal, blips_total, gl_total, jumlah_mismatch, status, created_by, tenant_id)
VALUES ('2026-06-25', 1240, 1235, 5, 'AVAILABLE', 'uat-seed', 'TUGURE')
ON CONFLICT (tanggal) DO NOTHING;

INSERT INTO jrnl.rekon_mismatch (tanggal, nomor_jurnal, jenis_mismatch, nilai_blips, nilai_gl, selisih, created_by, tenant_id)
VALUES
  ('2026-06-25', 'JRN-2026-0001', 'MISSING_IN_GL', 10000000, 0, 10000000, 'uat-seed', 'TUGURE'),
  ('2026-06-25', 'JRN-2026-0002', 'AMOUNT_DIFF',   20000000, 19500000, 500000, 'uat-seed', 'TUGURE'),
  ('2026-06-25', 'JRN-2026-0003', 'EXTRA_IN_GL',   0, 5000000, -5000000, 'uat-seed', 'TUGURE')
ON CONFLICT DO NOTHING;
```

**Tabel user (P3)**:

| User ID | Peran | Akses |
|---|---|---|
| `usr-akunctl-01` | ROLE-AKUN-CTL | Full read + Refresh |
| `usr-audit-01` | ROLE-AUDIT | Full read + Export |
| `usr-maker-01` | ROLE-MAKER-TR | Tidak punya akses |

---

## 2. Skenario Uji

### TC-010-01 — KPI Cards Tampil Benar (M17-05-AC1)

**Aktor**: ROLE-AKUN-CTL (`usr-akunctl-01`)  
**Langkah**:

1. Login sebagai `usr-akunctl-01`.
2. Buka `/reconciliation/daily`.
3. Verifikasi halaman ter-load dengan tanggal default = hari ini (`2026-06-25`).
4. Amati 4 kartu KPI.

**Hasil yang diharapkan**:
- [ ] Card "Total Jurnal BLIPS": `1.240`.
- [ ] Card "Total Jurnal GL": `1.235`.
- [ ] Card "Selisih Jumlah": `5` (atau `-5` untuk GL vs BLIPS).
- [ ] Card "Mismatch": `5`, warna latar merah (karena > 0).
- [ ] Tanggal ditampilkan di header: "Rekonsiliasi Harian — 25 Juni 2026".
- [ ] URL: `/reconciliation/daily?tanggal=2026-06-25`.

---

### TC-010-02 — Date Picker: Ganti Tanggal → URL State Berubah (M17-05-AC2)

**Aktor**: ROLE-AKUN-CTL (`usr-akunctl-01`)  
**Langkah**:

1. Buka `/reconciliation/daily` (default hari ini).
2. Klik date picker → pilih tanggal `2026-06-20`.
3. Verifikasi URL berubah.
4. Copy URL → buka di tab baru.

**Hasil yang diharapkan**:
- [ ] URL berubah menjadi `/reconciliation/daily?tanggal=2026-06-20` tanpa reload penuh (client-side navigation).
- [ ] Data kartu dan mismatch berubah sesuai tanggal `2026-06-20`.
- [ ] Buka URL di tab baru → halaman load langsung dengan tanggal `2026-06-20` (deep-link preserved).
- [ ] Date picker menampilkan tanggal yang dipilih.

---

### TC-010-03 — Banner "Data Tidak Tersedia" untuk Tanggal Tanpa Data (M17-05-AC2)

**Aktor**: ROLE-AKUN-CTL (`usr-akunctl-01`)  
**Langkah**:

1. Buka `/reconciliation/daily`.
2. Pilih tanggal yang belum ada data rekonsiliasi (mis. `2026-06-30`).

**Hasil yang diharapkan**:
- [ ] Banner informatif tampil: "Data rekonsiliasi untuk 30 Juni 2026 belum tersedia. Jalankan proses rekonsiliasi terlebih dahulu."
- [ ] KPI cards kosong atau tidak tampil.
- [ ] Mismatch table kosong dengan empty state.

---

### TC-010-04 — Mismatch DataTable: Warna per Jenis, Filter, Paging (M17-05-AC3)

**Aktor**: ROLE-AKUN-CTL (`usr-akunctl-01`)  
**Pre-kondisi**: Data mismatch 3 jenis tersedia untuk `2026-06-25`.  
**Langkah**:

1. Buka `/reconciliation/daily?tanggal=2026-06-25`.
2. Scroll ke tabel mismatch.
3. Amati warna baris per jenis.
4. Filter "Jenis Mismatch" → pilih `MISSING_IN_GL`.
5. Filter → pilih `AMOUNT_DIFF`.
6. Klik header "Selisih" → sort descending.

**Hasil yang diharapkan**:
- [ ] Baris `MISSING_IN_GL`: latar merah muda / badge merah.
- [ ] Baris `AMOUNT_DIFF`: latar kuning / badge amber.
- [ ] Baris `EXTRA_IN_GL`: latar oranye / badge oranye.
- [ ] Filter `MISSING_IN_GL`: hanya baris jenis tersebut tampil.
- [ ] Filter `AMOUNT_DIFF`: hanya baris tersebut tampil.
- [ ] Sort "Selisih" descending berfungsi.
- [ ] URL terupdate: `?filter[jenis_mismatch]=MISSING_IN_GL&sort=selisih:desc`.

---

### TC-010-05 — Tidak Ada Tombol Mutasi (M17-05-AC3)

**Aktor**: ROLE-AKUN-CTL (`usr-akunctl-01`)  
**Langkah**:

1. Buka `/reconciliation/daily?tanggal=2026-06-25`.
2. Amati seluruh halaman — header, kartu, tabel mismatch.

**Hasil yang diharapkan**:
- [ ] **Tidak ada** tombol "Jalankan Rekonsiliasi" atau "Post Jurnal" di halaman.
- [ ] Tombol "Refresh" tersedia (hanya refresh tampilan, bukan trigger rekonsiliasi baru).
- [ ] Card "DLQ Pending" jika tampil → link ke `/jurnal/dlq` (buka tab baru atau navigasi).

---

### TC-010-06 — Kartu Mismatch Hijau ketika 0 Mismatch (M17-05-AC3)

**Aktor**: ROLE-AKUN-CTL  
**Pre-kondisi**: Gunakan tanggal dengan 0 mismatch (atau seed data 0 mismatch).

```sql
-- UAT seed: tanggal zero mismatch
INSERT INTO jrnl.rekon_daily (tanggal, blips_total, gl_total, jumlah_mismatch, status, created_by, tenant_id)
VALUES ('2026-06-24', 1100, 1100, 0, 'AVAILABLE', 'uat-seed', 'TUGURE')
ON CONFLICT (tanggal) DO NOTHING;
```

**Langkah**:

1. Pilih tanggal `2026-06-24` di date picker.

**Hasil yang diharapkan**:
- [ ] Card "Mismatch": menampilkan `0`, warna latar **hijau** (bukan merah).
- [ ] Tabel mismatch menampilkan empty state: "Tidak ada mismatch untuk tanggal ini."

---

### TC-010-07 — Export Mismatch: CSV dan XLSX (M17-05-AC4)

**Aktor**: ROLE-AKUN-CTL (`usr-akunctl-01`)  
**Pre-kondisi**: Data mismatch tersedia untuk `2026-06-25`.  
**Langkah**:

1. Buka `/reconciliation/daily?tanggal=2026-06-25`.
2. Set filter "Jenis Mismatch" = `AMOUNT_DIFF`.
3. Klik **Export** → pilih **CSV**.
4. Ulangi → pilih **XLSX**.

**Hasil yang diharapkan**:
- [ ] File CSV terunduh: `rekon-mismatch-20260625.csv`.
- [ ] File XLSX terunduh: `rekon-mismatch-20260625.xlsx`.
- [ ] Hanya baris jenis `AMOUNT_DIFF` yang masuk (sesuai filter aktif).
- [ ] Kolom CSV/XLSX: Tanggal, Nomor Jurnal, Jenis Mismatch, Nilai BLIPS, Nilai GL, Selisih.
- [ ] Audit log: `REKON.MISMATCH_EXPORT` terekam.

---

### TC-010-08 — Gating: ROLE-MAKER-TR Tidak Dapat Akses (M17-05-AC4)

**Aktor**: ROLE-MAKER-TR (`usr-maker-01`)  
**Langkah**:

1. Login sebagai `usr-maker-01`.
2. Navigasi ke `/reconciliation/daily`.

**Hasil yang diharapkan**:
- [ ] Halaman redirect ke `/404` atau `/unauthorized`.
- [ ] Data rekonsiliasi tidak terekspos.

---

### TC-010-09 — ROLE-AUDIT: Akses Baca + Export (M17-05-AC4)

**Aktor**: ROLE-AUDIT (`usr-audit-01`)  
**Langkah**:

1. Login sebagai `usr-audit-01`.
2. Buka `/reconciliation/daily?tanggal=2026-06-25`.

**Hasil yang diharapkan**:
- [ ] Halaman terbuka, semua data tampil.
- [ ] Tidak ada tombol mutasi.
- [ ] Tombol Export tersedia.
- [ ] Dapat mengubah tanggal via date picker.

---

## 3. Verifikasi Audit Trail

```sql
SELECT action, actor_user_id, after_jsonb, event_time
FROM aud.audit_log
WHERE action LIKE 'REKON.%'
ORDER BY event_time DESC LIMIT 20;
```

---

## 4. Rollback / Cleanup

```sql
-- Hapus data seed UAT
DELETE FROM jrnl.rekon_mismatch WHERE created_by = 'uat-seed' AND tanggal IN ('2026-06-24','2026-06-25');
DELETE FROM jrnl.rekon_daily WHERE created_by = 'uat-seed' AND tanggal IN ('2026-06-24','2026-06-25');
```

---

## 5. Sign-Off

| Peran | Nama | Tanggal | Hasil | Tanda tangan |
|---|---|---|---|---|
| QA Tester | | | PASS / FAIL | |
| ROLE-AKUN-CTL (UAT Actor) | | | PASS / FAIL | |
| ROLE-AUDIT (UAT Observer) | | | PASS / FAIL | |
| Product Owner | | | APPROVED / REJECT | |
