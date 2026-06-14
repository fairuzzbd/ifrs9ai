# Interaction FLOW-001 — Master Data CRUD Flow (Pola Generik)

**Flow ID**: FLOW-001  
**Berlaku untuk**: Semua 16 modul `mst.*` — konkret `mata_uang`  
**Story**: APP-A-MSTR-001, APP-A-MSTR-002  
**Author**: uiux-designer  
**Tanggal**: 2026-06-03

---

## Happy Path — Create → Submit → Review → Approve

```
[ROLE-AKUN]                     [SISTEM]                    [ROLE-AKUN-CTL]
    │                               │                               │
    │  Akses /master/mata-uang      │                               │
    ├──────────────────────────────▶│                               │
    │                               │ GET /api/v1/master/mata-uang  │
    │                               │ → 200 list data               │
    │                               │                               │
    │  Klik "+ Tambah Mata Uang"    │                               │
    ├──────────────────────────────▶│                               │
    │  Buka /master/mata-uang/new   │                               │
    │                               │                               │
    │  Isi form: GBP, Pound...      │                               │
    │  Klik "Simpan"                │                               │
    ├──────────────────────────────▶│                               │
    │                               │ POST /api/v1/master/mata-uang │
    │                               │ Idempotency-Key: {uuid}       │
    │                               │ → 201 {data: {..DRAFT..}}     │
    │◀──────────────────────────────┤                               │
    │  Toast sukses + redirect      │                               │
    │  ke /master/mata-uang/GBP     │                               │
    │                               │                               │
    │  Klik "Kirim untuk Review"    │                               │
    ├──────────────────────────────▶│                               │
    │                               │ POST .../submit               │
    │                               │ → 200 {currentState: PENDING_REVIEW}
    │                               │ → Notif in-app ke AKUN-CTL   │
    │◀──────────────────────────────┤                               │
    │  Toast: "GBP berhasil         │                               │
    │  dikirim untuk review"        │         [notif badge muncul]  │
    │                               │                               │
    │                               │  [AKUN-CTL buka notifikasi]  │
    │                               │◀──────────────────────────────┤
    │                               │ GET /master/mata-uang/GBP     │
    │                               │ → 200 {..PENDING_REVIEW..}    │
    │                               │──────────────────────────────▶│
    │                               │  Tampil detail + panel review │
    │                               │                               │
    │                               │  Isi komentar "Review OK..."  │
    │                               │  Centang attest               │
    │                               │  Klik "Setujui & Lanjutkan"  │
    │                               │◀──────────────────────────────┤
    │                               │ POST .../review               │
    │                               │ SoD check: reviewer ≠ maker ✓ │
    │                               │ MFA check: mfa_verified=true ✓│
    │                               │ → 200 {currentState: PENDING_APPROVAL}
    │                               │──────────────────────────────▶│
    │                               │  Toast: "GBP berhasil di-review"│
    │                               │                               │
    │                               │  [AKUN-CTL ke-2 buka notif]  │
    │                               │◀──────────────────────────────┤
    │                               │ GET /master/mata-uang/GBP     │
    │                               │ → 200 {..PENDING_APPROVAL..}  │
    │                               │──────────────────────────────▶│
    │                               │  Tampil panel approve final   │
    │                               │                               │
    │                               │  Isi komentar "Disetujui..."  │
    │                               │  Centang attest               │
    │                               │  Klik "Setujui & Lanjutkan"  │
    │                               │◀──────────────────────────────┤
    │                               │ POST .../approve              │
    │                               │ SoD: approver ≠ maker ✓       │
    │                               │       approver ≠ reviewer ✓   │
    │                               │ → 200 {currentState: APPROVED}│
    │                               │ → Notif ke maker              │
    │                               │──────────────────────────────▶│
    │   [notif: GBP disetujui]      │  Toast: "GBP berhasil         │
    │◀──────────────────────────────┤   disetujui."                 │
```

---

## Failure Mode: SoD Violation

```
[AKUN-CTL yang juga Maker GBP]        [SISTEM]
    │                                      │
    │  Buka /master/mata-uang/GBP         │
    │  (yang dia buat sendiri)             │
    ├─────────────────────────────────────▶│
    │                                      │ GET → 200 (status PENDING_REVIEW)
    │◀─────────────────────────────────────┤
    │  Panel workflow tampil               │
    │  Step 2 (Review) = AKTIF            │
    │                                      │
    │  NAMUN: SodBlockBanner muncul:       │
    │  "Anda adalah pembuat mata uang ini. │
    │   Tidak bisa menjadi reviewer."      │
    │                                      │
    │  Tombol "Setujui" = disabled         │
    │  Tombol "Tolak" = disabled           │
    │  Tooltip: "SoD — lihat kebijakan"   │
    │                                      │
    │  (user tidak bisa lakukan apapun     │
    │   kecuali membaca data)              │
```

Client-side: tombol disabled dengan tooltip SoD. Server-side jika user bypass UI dan panggil API langsung → 403 `SOD_VIOLATION`.

---

## Failure Mode: Optimistic Lock Conflict (409)

```
[User A]                      [SISTEM]                     [User B]
    │                              │                             │
    │  Buka edit GBP               │   Buka edit GBP             │
    │  (row_version=1)             │   (row_version=1)           │
    ├─────────────────────────────▶│◀────────────────────────────┤
    │                              │ Kirim form kedua user       │
    │                              │                             │
    │  Klik "Simpan"               │                             │
    ├─────────────────────────────▶│                             │
    │                              │ PUT (row_version=1) → 200   │
    │                              │ row_version DB jadi 2       │
    │◀─────────────────────────────┤                             │
    │  Toast sukses                │                             │
    │                              │          Klik "Simpan"      │
    │                              │◀────────────────────────────┤
    │                              │ PUT (row_version=1) → 409   │
    │                              │ CONFLICT                    │
    │                              │──────────────────────────────▶
    │                              │     Toast merah persistent: │
    │                              │     "GBP telah diubah oleh  │
    │                              │      user lain. Muat ulang."│
    │                              │     Form di-lock            │
    │                              │     [Muat Ulang] di toast   │
```

---

## Failure Mode: Reject → Returned → Resubmit

```
[ROLE-AKUN-CTL]               [SISTEM]               [ROLE-AKUN Maker]
    │                              │                          │
    │  Klik "Tolak" di panel       │                          │
    ├─────────────────────────────▶│                          │
    │  Reject sub-panel muncul     │                          │
    │  inline (bukan modal baru)   │                          │
    │                              │                          │
    │  Isi alasan (min 10 char)    │                          │
    │  Centang attest              │                          │
    │  Klik "Tolak & Kembalikan"   │                          │
    ├─────────────────────────────▶│                          │
    │                              │ POST .../reject          │
    │                              │ comment validasi min 10  │
    │                              │ → 200 {RETURNED}         │
    │                              │ → Notif ke maker          │
    │◀─────────────────────────────┤   [notif badge muncul]   │
    │  Toast: "GBP dikembalikan    │◀─────────────────────────┤
    │  ke akun.maker."             │                          │
    │                              │ Maker buka GBP           │
    │                              │──────────────────────────▶
    │                              │  ReturnedBanner tampil    │
    │                              │  dengan komentar penolakan│
    │                              │                          │
    │                              │  Klik [Edit & Perbaiki]   │
    │                              │◀─────────────────────────┤
    │                              │  Form edit aktif          │
    │                              │  (field yang boleh diubah)│
    │                              │                          │
    │                              │  Klik "Simpan"            │
    │                              │◀─────────────────────────┤
    │                              │ PUT .../GBP              │
    │                              │ → 200 (masih RETURNED)   │
    │                              │                          │
    │                              │  Klik "Kirim Ulang"       │
    │                              │◀─────────────────────────┤
    │                              │ POST .../submit          │
    │                              │ audit: MATA_UANG.RESUBMIT│
    │                              │ → 200 {PENDING_REVIEW}   │
    │                              │ → Notif ke AKUN-CTL      │
    │  [notif badge muncul]        │──────────────────────────▶
    │◀─────────────────────────────┤  Toast: "GBP berhasil    │
    │                              │  dikirim ulang."          │
```

---

## Failure Mode: Double Submit Block

```
[User]                              [SISTEM]
    │                                    │
    │  Klik "Simpan" (pertama kali)      │
    ├───────────────────────────────────▶│
    │  Button disabled + spinner inline  │
    │  (tidak bisa diklik lagi)          │
    │                                    │ Processing...
    │  User mencoba klik lagi            │
    │  ─ tombol tidak bisa diklik ─      │
    │                                    │
    │                                    │ → 201 response
    │◀───────────────────────────────────┤
    │  Button aktif kembali              │
    │  Toast sukses muncul               │
    │  Redirect ke detail                │
```

Mekanisme: `submitting` state React (`useState(false)`), tombol disabled + `cursor-not-allowed` + spinner anak dari tombol (bukan overlay). Idempotency-Key baru di-generate setiap kali form dimount (bukan setiap klik).

---

## Failure Mode: MFA Tidak Terverifikasi (AKUN-CTL)

```
[AKUN-CTL dengan mfa_verified=false]     [SISTEM]
    │                                         │
    │  Buka detail GBP (PENDING_APPROVAL)     │
    ├────────────────────────────────────────▶│
    │                                         │ → 200 detail
    │◀────────────────────────────────────────┤
    │                                         │
    │  Panel approve tampil                   │
    │  (normal, belum tahu MFA status)        │
    │                                         │
    │  Centang attest                         │
    │  Klik "Setujui & Lanjutkan"             │
    ├────────────────────────────────────────▶│
    │                                         │ POST .../approve
    │                                         │ Check: mfa_verified=false
    │                                         │ → 403 MFA_REQUIRED
    │◀────────────────────────────────────────┤
    │  Toast merah: "Multi-Factor             │
    │  Authentication wajib untuk Finance     │
    │  Controller. Silakan login ulang        │
    │  dengan MFA. [Login ulang →]"          │
```

Client-side hint: Sebelum menampilkan panel approve, frontend check JWT claim `mfa_verified`. Jika false + user AKUN-CTL: tampilkan banner informasi "Anda perlu login dengan MFA untuk melakukan approval."

---

## Failure Mode: Export Async (> 10k row)

```
[User]                              [SISTEM]                    [Worker]
    │                                    │                           │
    │  Klik "Export" → "XLSX"            │                           │
    ├───────────────────────────────────▶│                           │
    │                                    │ GET .../export            │
    │                                    │ Row count > 10k           │
    │                                    │ Submit Asynq job          │
    │                                    │ → 202 {jobId: "job_01..."}│
    │◀───────────────────────────────────┤                           │
    │  JobProgressPanel muncul           │                           │
    │  "Menyiapkan export XLSX..."       │   Job mulai berjalan      │
    │  [Batalkan] [Background]           │──────────────────────────▶│
    │                                    │                           │
    │  Klik "Background"                 │                           │
    │  Panel tutup                       │                           │
    │  User lanjut kerja di halaman lain │                           │
    │                                    │         Job selesai ✓    │
    │                                    │◀──────────────────────────┤
    │  Notif badge (+1) di top bar       │ Upload ke MinIO           │
    │  [Badge berkedip]                  │ Signed URL generated      │
    │                                    │                           │
    │  Klik badge → lihat job history    │                           │
    │                                    │                           │
    │  Toast sukses:                     │                           │
    │  "Export XLSX selesai.             │                           │
    │   [Unduh file (berlaku 24 jam) →]"│                           │
```

---

## Empty State — Halaman List Kosong (Pertama Kali)

```
[User dengan permission mata_uang.read]     [SISTEM]
    │                                            │
    │  Akses /master/mata-uang                   │
    │  (belum ada data, baru deploy)             │
    ├───────────────────────────────────────────▶│
    │                                            │ → 200 data=[] pagination.totalEstimate=0
    │◀───────────────────────────────────────────┤
    │  Tabel menampilkan empty state:            │
    │                                            │
    │  [ilustrasi dokumen kosong]                │
    │  "Belum ada mata uang yang terdaftar."     │
    │  "Klik '+ Tambah Mata Uang' untuk mulai." │
    │  [+ Tambah Mata Uang]  ← CTA langsung     │
```

---

## Loading States

**List page loading:**
- Action bar dan filter bar tampil normal
- Tabel area: 7 skeleton rows (shimmer animation)
- Skeleton row: kolom sesuai jumlah kolom yang dikonfigurasi, lebar proporsional

**Detail page loading:**
- Header (judul + badge): skeleton
- Kolom kiri: 6-8 skeleton label-value pairs
- Kolom kanan (workflow panel): skeleton stepper (3 step abu-abu)

**Workflow action submit loading:**
- Tombol "Setujui & Lanjutkan" / "Tolak & Kembalikan": disabled + spinner inline
- Workflow panel tidak di-update sampai response diterima (optimistic update TIDAK dipakai — financial data, server state is truth)

---

## Scroll Behavior

**Setelah validation error:**
- Scroll ke field pertama yang error (smooth scroll)
- Field pertama yang error mendapat focus

**Setelah toast:**
- Toast muncul di top-right, tidak menggusur konten
- Konten tidak auto-scroll

**Setelah submit sukses + redirect:**
- Halaman baru dimuat dari atas (normal navigation)
