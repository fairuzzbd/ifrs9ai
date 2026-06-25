# UAT-APP-B-012 — Transaksi Jatuh Tempo: DataTable Monitoring Read-Only UX §1

**UAT ID**: UAT-APP-B-012
**Modul**: APP-B — Transaction Lifecycle (Frontend Consolidation)
**Story Set**: P5-M16 / Story P5-M16-04
**AC yang dicakup**: M16-04-AC1 (jatuh tempo monitoring DataTable)
**Tanggal UAT**: _(diisi saat pelaksanaan)_
**Penyusun**: qa-engineer
**Gate**: security-engineer BLOCKING — mutation buttons absent for ROLE-AUDIT; cron trigger gated

---

## Pre-Kondisi

1. Environment UAT berjalan
2. P5-M9 deployed — jatuh tempo endpoints aktif
3. P5-M16 deployed — gap fixes aktif (default sort asc, quick-filter buttons, hari_tersisa column, Buat Renewal CTA, cron trigger gating)
4. Data seed:

| ID | Kode | Jenis | Counterparty | Nominal IDR | Tgl Jatuh Tempo | Status |
|---|---|---|---|---|---|---|
| jt-001 | DEP-0042 | DEPOSITO | Bank BCA | Rp 2.000.000.000 | +7 hari dari UAT | UPCOMING |
| jt-002 | DEP-0043 | DEPOSITO | Bank BNI | Rp 1.000.000.000 | +30 hari dari UAT | UPCOMING |
| jt-003 | OBL-0010 | OBLIGASI | PT ABC | Rp 5.000.000.000 | -5 hari dari UAT | PAST_DUE |
| jt-004 | DEP-0055 | DEPOSITO | Bank BRI | Rp 500.000.000 | -55 hari dari UAT | SETTLED |

5. User test:

| User ID | Role | Permission |
|---|---|---|
| USR-MAKER-001 | ROLE-MAKER-TR | transaksi.jatuh-tempo.read, renewal.create |
| USR-AKUN-CTL-001 | ROLE-AKUN-CTL | transaksi.jatuh-tempo.read |
| USR-AUDIT-001 | ROLE-AUDIT | transaksi.jatuh-tempo.read |

---

## Data Test Numerik

- DEP-0042: tanggal_jatuh_tempo = tanggal UAT + 7 hari → hari_tersisa = +7
- DEP-0043: tanggal_jatuh_tempo = tanggal UAT + 30 hari → hari_tersisa = +30
- OBL-0010: tanggal_jatuh_tempo = tanggal UAT - 5 hari → hari_tersisa = -5 (PAST_DUE, tampil merah)
- DEP-0055: tanggal_jatuh_tempo = tanggal UAT - 55 hari → SETTLED

---

## Skenario UAT

### TC-001 — M16-04-AC1: DataTable default sort tanggal_jatuh_tempo:asc (upcoming first)

**Actor**: USR-MAKER-001

**Langkah**:
1. Login sebagai USR-MAKER-001, navigasi ke `/transaksi/jatuh-tempo`
2. Perhatikan urutan baris default
3. Cek kolom yang tampil termasuk "Hari Tersisa"
4. Cek nilai kolom "Hari Tersisa" untuk setiap baris

**Hasil yang Diharapkan**:
- Default sort: ascending (DEP-0042 +7 hari di atas; DEP-0043 +30 hari; OBL-0010 -5 hari; DEP-0055 -55 hari paling bawah) — M16 gap fix: sebelumnya descending
- Kolom "Hari Tersisa" tampil (M16 gap fix — sebelumnya MISSING)
  - DEP-0042: "+7 hari" atau "7 hari" (hijau/normal)
  - OBL-0010: "−5 hari" atau "−5 hari" (merah — past due)
  - SETTLED row: dapat menampilkan "−55 hari" atau "-" tergantung implementasi
- Status badge tampil: UPCOMING (kuning), PAST_DUE (merah), SETTLED (hijau)

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-002 — M16-04-AC1: Quick-filter shortcuts tersedia dan berfungsi

**Actor**: USR-MAKER-001

**Langkah**:
1. Login sebagai USR-MAKER-001, navigasi ke `/transaksi/jatuh-tempo`
2. Klik tombol "Dalam 7 hari"
3. Verifikasi filter chip dan data
4. Klik tombol "Dalam 30 hari"
5. Klik tombol "Sudah Jatuh Tempo"
6. Bersihkan filter

**Hasil yang Diharapkan**:
- Langkah 2-3: Filter chip muncul; tabel hanya tampilkan DEP-0042 (jt ≤ 7 hari); URL mengandung filter (M16 gap fix — sebelumnya MISSING shortcut)
- Langkah 4: Tabel tampilkan DEP-0042 + DEP-0043 (jt ≤ 30 hari)
- Langkah 5: Tabel hanya tampilkan OBL-0010 (status PAST_DUE)
- Hanya satu quick-filter aktif per waktu (klik satu → yang lain di-clear)
- Langkah 6: "Bersihkan semua filter" → seluruh data kembali tampil

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-003 — M16-04-AC1: Buat Renewal CTA per UPCOMING row

**Actor**: USR-MAKER-001

**Langkah**:
1. Login sebagai USR-MAKER-001, navigasi ke `/transaksi/jatuh-tempo`
2. Cek baris DEP-0042 (UPCOMING) — cek kolom Aksi
3. Klik "Buat Renewal" pada baris DEP-0042
4. Cek baris OBL-0010 (PAST_DUE) — cek kolom Aksi

**Hasil yang Diharapkan**:
- Langkah 2: Link "Buat Renewal" tampil di baris DEP-0042 (M16 gap fix — sebelumnya MISSING)
- Langkah 3: Navigasi ke `/transaksi/renewal/new?instrumen_id={id DEP-0042}` — form renewal sudah terisi instrumen asal
- Langkah 4: Tidak ada "Buat Renewal" di baris OBL-0010 (PAST_DUE → tidak bisa renewal)
- Baris DEP-0055 (SETTLED): tidak ada "Buat Renewal"

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-004 — M16-04-AC1: Read-only — tidak ada tombol create/edit/delete; ROLE-AUDIT export tersedia

**Actor**: USR-MAKER-001, USR-AUDIT-001

**Langkah**:
1. Login sebagai USR-MAKER-001, navigasi ke `/transaksi/jatuh-tempo`
2. Inspect DOM untuk tombol create, delete, edit
3. Verifikasi tombol Trigger Cron (jika ada) hanya visible untuk role yang punya permission
4. Logout → Login sebagai USR-AUDIT-001, navigasi ke `/transaksi/jatuh-tempo`
5. Cek tombol mutasi dan export

**Hasil yang Diharapkan**:
- Langkah 2: Tidak ada tombol "+ Tambah", "Hapus", "Edit" di halaman ini (read-only monitoring)
- Langkah 3: Tombol "Trigger Maturity Cron" (jika ada di halaman): hanya tampil untuk role dengan `transaksi.akrual.create` (M16 gap fix — wrap dalam permission check)
- Langkah 5 (USR-AUDIT-001):
  - DataTable tampil dengan data read-only
  - Tombol Trigger Cron **TIDAK ADA** (bukan hanya disabled — absent)
  - Tombol "Ekspor" **TAMPIL** — AUDIT masih dapat export

**Verifikasi Audit** (untuk export):
```sql
SELECT * FROM aud.audit_log WHERE action = 'JATUH_TEMPO.EXPORT' ORDER BY event_time DESC LIMIT 1;
```

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

## Sign-Off

| Peran | Nama | Tanggal | Tanda Tangan |
|---|---|---|---|
| Tester (QA) | | | |
| Reviewer (Tech Lead) | | | |
| Security Reviewer | | | |
| Approver (Business) | | | |
