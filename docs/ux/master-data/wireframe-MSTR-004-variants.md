# Wireframe MSTR-004-VARIANTS — Varian Desain per Kelompok Modul

**Screen ID**: MSTR-004-VARIANTS  
**Berlaku untuk**: Varian A (ECL Param), Varian B (Counterparty PII), Varian C (Upload/Import)  
**Story**: APP-A-MSTR-001 §Varian A, B, C  
**Author**: uiux-designer  
**Tanggal**: 2026-06-03

---

## Varian A: ECL Parameter Masters (6-Eyes + ALCO)

**Modul**: `lgd_basel`, `bobot_skenario`, `lps_coverage`, `pd_pefindo`, `impact_mev_pd`, `impact_pd`

### A1. Banner Peringatan ECL Parameter

Muncul di bagian atas setiap halaman (list, detail, form) modul ECL param:

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  ⚠  Parameter ECL — Dampak Signifikan terhadap Perhitungan CKPN             │
│                                                                              │
│  Data pada modul ini digunakan langsung dalam kalkulasi ECL (CKPN) sesuai   │
│  PSAK 71. Perubahan memerlukan persetujuan 6-Eyes: RISK → Finance            │
│  Controller → ALCO (dengan step-up MFA).                                     │
│                                                                              │
│  Setelah Calc Run di-seal, parameter yang digunakan tidak dapat diubah       │
│  retroaktif (ECL_PARAM_FROZEN).                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

Warna: amber-50 background, amber-600 border, amber-800 text. Tidak bisa di-dismiss.

### A2. Badge Versi Parameter

Di samping judul halaman detail dan di kolom list:

```
Bobot Skenario ECL     [v.2026-Q2]  ◑ Menunggu Approval 2
                       ↑
                       Badge: bg-blue-100 text-blue-800 rounded
                       "v.YYYY-QN" atau "v.YYYY-MM-DD"
                       Link ke record approval ALCO terkait
```

### A3. Indikator ECL_PARAM_FROZEN

Muncul saat parameter sudah dipakai dalam Calc Run yang di-seal:

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  🔒  Parameter ini sudah digunakan dalam Calc Run yang telah di-seal         │
│      (Periode: Jun 2026 — CalcRun ID: CR-2026-06-001)                       │
│      Parameter ini tidak dapat diubah. Buat versi baru jika diperlukan.     │
│      [Lihat Calc Run →]                                                      │
└──────────────────────────────────────────────────────────────────────────────┘
```

Warna: slate-100 background, slate-400 border. Tombol Edit di-disable dengan tooltip yang sama.

### A4. Panel Workflow 6-Eyes

Stepper 4 step (bukan 3):

```
PROSES PERSETUJUAN (6-eyes — Parameter ECL)
─────────────────────────────────────────────────────

  ✓  Step 1: Pembuat (Maker)
     ROLE-RISK — risk.analyst
     03 Jun 2026, 09:00 WIB

  ✓  Step 2: Pemeriksa (Reviewer)
     ROLE-AKUN-CTL — akun.ctl.1
     03 Jun 2026, 11:00 WIB
     "Review kalibrasi PD vs Pefindo study 2025 OK."

  ●  Step 3: Persetujuan 1 (Approver)              ← AKTIF
     ROLE-ALCO — [menunggu]
     Memerlukan: MFA Step-Up
     ──────────────────────────────
     [APPROVAL ACTION PANEL dengan MFA step-up prompt]

  ○  Step 4: Persetujuan 2 (Approver 2)
     ROLE-ALCO — user berbeda dari Approver 1
     (menunggu step 3 selesai)
```

MFA Step-Up prompt muncul inline di action panel (bukan modal baru) setelah checkbox dicentang dan tombol diklik.

### A5. Indikator Compliance Gate

Di halaman list dan detail ECL param:

```
Status Compliance Gate: [ Menunggu Review ifrs9-compliance-reviewer ]
```

Badge khusus (bukan workflow_status standar):

| Status | Label | Warna |
|---|---|---|
| Belum review | Menunggu Compliance Review | amber |
| Lulus | Compliance Verified | green |
| Gagal | Compliance Ditolak | red |

Compliance review adalah proses di luar sistem (via PR) — badge ini manual dan di-set oleh sistem melalui API khusus.

### A6. Versioning (bukan amandemen in-place)

Untuk ECL param: perubahan = record baru, bukan update.

Pada halaman list ECL param, kolom tambahan:
- "Berlaku Dari" (effective_from)
- "Berlaku Sampai" (effective_to — null jika current)
- "Status Versi" (badge: Aktif / Tidak Aktif / Menunggu Persetujuan)

Filter default: tampilkan hanya versi terkini (effective_to IS NULL). Toggle "Tampilkan Semua Versi" di filter bar.

---

## Varian B: Counterparty & Rating History (PII Masked)

**Modul**: `mst.counterparty`, `mst.rating_history`

### B1. Masking PII di Tabel List

Kolom `npwp`, `nomor_rekening`, `ktp` ditampilkan termasking secara default:

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ Nama Counterparty │ NPWP           │ No. Rekening    │ Rating  │ Status     │
│───────────────────┼────────────────┼─────────────────┼─────────┼────────────│
│ PT ABC Tbk        │ **.**.***.*.** │ ****-****-5678  │ idA     │ ●APPROVED  │
│ PT XYZ            │ **.**.***.*.** │ ****-****-9012  │ idAA    │ ●APPROVED  │
└──────────────────────────────────────────────────────────────────────────────┘
```

Format masking:
- NPWP (15 digit): `**.**.***.*.******` (tampilkan 2 digit terakhir saja)
- No. Rekening: `****-****-XXXX` (tampilkan 4 digit terakhir)
- KTP (16 digit): `****-****-****-XXXX` (tampilkan 4 terakhir)

### B2. Reveal PII (permission counterparty.read.pii)

Hanya ROLE-AUDIT dengan permission `counterparty.read.pii`:

Tombol [Lihat PII] di header halaman detail (bukan di list):

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  ⚠  Anda akan melihat data PII. Akses ini dicatat dalam audit log.          │
│                                                                              │
│  [Batal]                                    [Tampilkan PII untuk Sesi Ini]  │
└──────────────────────────────────────────────────────────────────────────────┘
```

Setelah konfirmasi:
- Data PII ditampilkan decrypted untuk sesi ini (tidak persist)
- Tombol berubah menjadi "Sembunyikan PII"
- Audit log: `COUNTERPARTY.PII_VIEWED` dengan actor, timestamp, IP

### B3. SICR Trigger Indicator

Pada halaman detail counterparty dan di tabel rating_history:

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  ⚡ SICR TRIGGERED — Rating turun dari idAA → idBBB (turun 4 notch)         │
│     Tanggal trigger: 15 Mei 2026                                             │
│     Instrument terdampak: 3 instrumen                                        │
│     Stage: 1 → 2 (Significant Increase in Credit Risk)                      │
│     [Lihat instrumen terdampak →]                                            │
└──────────────────────────────────────────────────────────────────────────────┘
```

Warna: red-50 background, red-500 border. Hanya muncul jika `sicr_triggered = true`.

### B4. Export PII

Tombol Export untuk counterparty menampilkan dialog tambahan:

```
Export Counterparty
─────────────────────────────────────────────────────────

Format: [CSV ▾]

Kolom PII (NPWP, No. Rekening, KTP):
  ○ Masked (default) — tampilkan dengan format **.**.***.*.***...
  ● Tidak termasuk — kolom PII tidak ada di file export

  (Opsi "Tampilkan penuh" hanya untuk ROLE-AUDIT dengan
   permission counterparty.read.pii — tidak ada di sini)

                            [Batal]    [Export]
```

PII **tidak pernah** di-export dalam bentuk plain text ke non-AUDIT role.

---

## Varian C: Upload/Import Masters (Long-Process, UX §3)

**Modul**: `pd_pefindo` (XLSX Pefindo), `chart_of_accounts` (Excel CoA), `kurs` (BI JISDOR job)

### C1. Halaman Upload pd_pefindo

```
┌────────────────────────────────────────────────────────────────────────────────┐
│  BLIPS IFRS9  [nav]                                          [user] [notif]    │
├────────────────────────────────────────────────────────────────────────────────┤
│                                                                                │
│  ▸ Master Data / PD Pefindo / Import Data                                     │
│                                                                                │
│  Import Data PD Pefindo                                                        │
│  ────────────────────────────────────────────────────────                      │
│                                                                                │
│  ┌──────────────────────────────────────────────────────────────────────────┐ │
│  │  ⚠  Parameter ECL — Perubahan ini memerlukan persetujuan 6-Eyes ALCO.   │ │
│  └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                                │
│  LANGKAH 1: UPLOAD FILE                                                        │
│  ─────────────────────────────────────────────────────────                     │
│                                                                                │
│  ┌──────────────────────────────────────────────────────────────────────────┐ │
│  │                                                                          │ │
│  │         [Drag & Drop file XLSX Pefindo di sini]                          │ │
│  │                   atau                                                   │ │
│  │                [Pilih File]                                              │ │
│  │                                                                          │ │
│  │  Format: XLSX (Pefindo Annual Default Study format)                      │ │
│  │  Ukuran maksimal: 50 MB                                                  │ │
│  │  Kolom wajib: Sektor, Rating, PD_12M, PD_Lifetime, Tahun                │ │
│  │                                                                          │ │
│  └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                                │
│  File terpilih: pefindo-default-study-2025.xlsx (2.3 MB)  [×]                │
│                                                                                │
│  [Kembali]                              [Unggah & Validasi →]                 │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### C2. Progress Upload (JobProgressPanel)

Setelah klik "Unggah & Validasi", muncul JobProgressPanel:

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  Mengimpor Data PD Pefindo                                                   │
│                                                                              │
│  ████████████░░░░░░░░░░░░░░░░░░  35%                                        │
│                                                                              │
│  Membaca sheet "Rating_Matrix"... (baris 340 dari 980)                       │
│                                                                              │
│  Mulai: 10:30:00  ·  Estimasi selesai: 10:32:00  (2 menit lagi)            │
│                                                                              │
│                         [ Batalkan ]     [ Background ]                     │
└──────────────────────────────────────────────────────────────────────────────┘
```

### C3. Preview Diff (sebelum commit)

Setelah parsing selesai, sebelum data di-commit ke DB:

```
LANGKAH 2: PREVIEW PERUBAHAN
─────────────────────────────────────────────────────────────────────────────
File: pefindo-default-study-2025.xlsx
SHA-256: a1b2c3d4...
Total baris: 980  |  Baru: 45  |  Diperbarui: 312  |  Tidak berubah: 623

┌──────────────────────────────────────────────────────────────────────────────┐
│ Sektor        │ Rating │ PD 12M (Lama)  │ PD 12M (Baru)  │ Delta           │
│───────────────┼────────┼────────────────┼────────────────┼─────────────────│
│ Perbankan     │ idAAA  │ 0.00010000     │ 0.00008500     │ ▼ -15.0%        │
│ Perbankan     │ idAA   │ 0.00045000     │ 0.00042000     │ ▼ -6.7%         │
│ Manufaktur    │ idBBB  │ 0.00350000     │ 0.00380000     │ ▲ +8.6%  ← ⚠   │
│ Manufaktur    │ idBB   │ 0.01200000     │ 0.01350000     │ ▲ +12.5% ← ⚠   │
│ ...           │        │                │                │                  │
└──────────────────────────────────────────────────────────────────────────────┘

⚠ 2 perubahan signifikan (delta > 10%) — perlu perhatian khusus saat review.

Baris baru (45):
  [ Toggle: Lihat semua baris baru ▸ ]

Filter diff: [Semua ▾] [Baru saja ▾] [Diperbarui ▾] [Signifikan ▾]

                            [Batalkan]    [Lanjut: Simpan & Mulai Workflow →]
```

"⚠" hanya tampil di baris dengan delta > 10% (threshold configurable). Warna delta: hijau untuk turun (PD lebih baik), merah untuk naik (PD lebih buruk).

### C4. Setelah Commit — Mulai Workflow

Setelah "Simpan & Mulai Workflow":

```
Import berhasil!
─────────────────────────────────────
Data PD Pefindo telah disimpan dengan status DRAFT.
Import ID: IMP-2026-0042

Langkah selanjutnya:
1. Submit data untuk review Finance Controller
2. Approval ALCO (2 approver, step-up MFA)

[Lihat Draft yang Dibuat →]    [Submit untuk Review →]
```

Toast hijau: "Import PD Pefindo berhasil — 980 baris diproses, 45 baru, 312 diperbarui. Status: DRAFT. [Lihat →]"

### C5. BI JISDOR Scheduled Job

Untuk `kurs` yang auto-fetch dari BI JISDOR:

Di halaman Master Kurs, section status job:

```
STATUS FEED BI JISDOR
─────────────────────────────────────────────────
Job terakhir: Hari ini, 10:32 WIB  ●  Berhasil
254 kurs diperbarui (mata uang aktif)
[Lihat log →]

Job berikutnya: besok, 10:30 WIB (terjadwal)

[Override Manual]  (ROLE-AKUN saja, jika feed gagal)
```

Jika job gagal:
```
STATUS FEED BI JISDOR
─────────────────────────────────────────────────
Job terakhir: Hari ini, 10:32 WIB  ✗  GAGAL
Error: Koneksi ke server BI timeout (30s)
[Lihat detail error →]

Kurs hari ini belum tersedia. Gunakan Override Manual
atau tunggu retry otomatis pada 11:00 WIB.

[Retry Sekarang]    [Override Manual]
```

---

## Kombinasi Varian (pd_pefindo = A + C)

Modul `pd_pefindo` memiliki kedua varian:
- Banner ECL param (A1)
- Badge versi (A2)
- Panel 6-Eyes (A4)
- Compliance gate indicator (A5)
- Versioning bukan amandemen (A6)
- Upload XLSX flow (C1-C4)

Pada halaman list pd_pefindo, ada **2 CTA button**:
```
[+ Tambah Manual]    [Import dari File XLSX]
```

Import flow membawa user ke halaman terpisah `/ecl-param/pd-pefindo/import`.

---

## Komponen yang Dipakai (Varian)

| Komponen | Sumber | Catatan |
|---|---|---|
| `EclParamBanner` | `components/blips/EclParamBanner.tsx` | **BARU** |
| `EclParamFrozenBanner` | `components/blips/EclParamFrozenBanner.tsx` | **BARU** |
| `ComplianceGateBadge` | `components/blips/ComplianceGateBadge.tsx` | **BARU** |
| `SixEyesWorkflowPanel` | Extend dari `MakerReviewerApproverPanel` | **BARU** (4 steps) |
| `PiiMaskedField` | `components/blips/PiiMaskedField.tsx` | **BARU** |
| `PiiRevealDialog` | `components/blips/PiiRevealDialog.tsx` | **BARU** |
| `SicrTriggerBanner` | `components/blips/SicrTriggerBanner.tsx` | **BARU** |
| `FileUploadDropzone` | `components/blips/FileUploadDropzone.tsx` | **BARU** |
| `ImportDiffTable` | `components/blips/ImportDiffTable.tsx` | **BARU** |
| `JobProgressPanel` | `components/blips/JobProgressPanel.tsx` | Phase 2, sudah ada |
| `FeedStatusPanel` | `components/blips/FeedStatusPanel.tsx` | **BARU** |
