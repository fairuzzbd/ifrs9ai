# Wireframe MSTR-003-DETAIL-WORKFLOW — Halaman Detail + Panel Workflow

**Screen ID**: MSTR-003-DETAIL-WORKFLOW  
**Berlaku untuk**: Semua modul `mst.*` — konkret `mata_uang`  
**Story**: APP-A-MSTR-001 §Feature:Workflow Approval 4-Eyes; APP-A-MSTR-002  
**Pattern utama**: MakerReviewerApproverPanel (stepper vertikal), ApprovalWithSignature  
**Author**: uiux-designer  
**Tanggal**: 2026-06-03

---

## Layout Halaman Detail — 2 Kolom (content + workflow)

```
┌────────────────────────────────────────────────────────────────────────────────────────────┐
│  BLIPS IFRS9  [nav]                                               [user badge] [notif 1]   │
├────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                            │
│  ▸ Master Data / Mata Uang / GBP                    [breadcrumb]                          │
│                                                                                            │
│  Mata Uang: Pound Sterling (GBP)             ◑ Menunggu Review     [Edit] [•••]           │
│  ────────────────────────────────────────────────────────────────────────────────────────  │
│                                                                                            │
│  ┌─────────────────────────────────────────────┐  ┌─────────────────────────────────────┐ │
│  │  DETAIL ENTITAS                             │  │  PROSES PERSETUJUAN                 │ │
│  │  ───────────────────────────────────────    │  │  ─────────────────────────────────  │ │
│  │                                             │  │                                     │ │
│  │  Kode Mata Uang   GBP                       │  │  [STEPPER WORKFLOW — lihat bawah]   │ │
│  │  Nama             Pound Sterling            │  │                                     │ │
│  │  Simbol           £                         │  │  ─────────────────────────────────  │ │
│  │  Decimal Places   2                         │  │  AKSI TERSEDIA                      │ │
│  │  Sumber Kurs      BI Kurs Tengah            │  │  [APPROVAL ACTION PANEL]            │ │
│  │  Frekuensi        Harian                    │  │                                     │ │
│  │  Tgl Mulai Aktif  03 Jun 2026               │  │                                     │ │
│  │  Status Aktif     Ya                        │  │                                     │ │
│  │                                             │  │                                     │ │
│  │  ─────────────────────────────────────────  │  │                                     │ │
│  │  METADATA                                   │  │                                     │ │
│  │  Dibuat oleh      akun.maker                │  │                                     │ │
│  │  Dibuat pada      03 Jun 2026, 08:00 WIB    │  │                                     │ │
│  │  Diperbarui oleh  akun.maker                │  │                                     │ │
│  │  Diperbarui pada  03 Jun 2026, 08:30 WIB    │  │                                     │ │
│  │  Versi            1                         │  │                                     │ │
│  │                                             │  │                                     │ │
│  │  ─────────────────────────────────────────  │  │                                     │ │
│  │  [Lihat Riwayat Audit →]                    │  │                                     │ │
│  └─────────────────────────────────────────────┘  └─────────────────────────────────────┘ │
│                                                                                            │
└────────────────────────────────────────────────────────────────────────────────────────────┘
```

Rasio kolom: 60% detail / 40% workflow panel. Di mobile: stack vertikal (detail dulu, workflow di bawah).

---

## Panel Workflow — MakerReviewerApproverPanel

### State: PENDING_REVIEW (tampilan dari sudut pandang AKUN-CTL yang akan mereview)

```
PROSES PERSETUJUAN (4-eyes)
─────────────────────────────────────

  ✓  Step 1: Pembuat (Maker)
     ─────────────────────────────
     akun.maker — ROLE-AKUN
     03 Jun 2026, 08:30 WIB
     "Mata uang GBP siap untuk review."
     [Lihat detail ▸]  (collapsed by default)

  ●  Step 2: Pemeriksa (Reviewer)          ← AKTIF (highlighted)
     ─────────────────────────────
     Menunggu review dari Finance Controller
     [tidak ada penanda user — belum ditugaskan]

     TINDAKAN:
     ─────────────────────────────
     Komentar (opsional)
     ┌─────────────────────────────────────────────────────┐
     │                                                     │
     │  Review OK — kode ISO valid, decimal...             │
     │                                                     │
     └─────────────────────────────────────────────────────┘
     (maksimal 1000 karakter)

     ┌─────────────────────────────────────────────────────┐
     │ ☐ Saya menyatakan bahwa data mata uang ini telah    │
     │   saya periksa dan sesuai dengan standar yang       │
     │   berlaku.                                          │
     └─────────────────────────────────────────────────────┘

     [Tolak ▸]          [Setujui & Lanjutkan →]

     ─────────────────────────────
     Catatan: Anda tidak bisa mereview mata uang yang
     Anda buat sendiri (Segregation of Duties).

  ○  Step 3: Pemberi Persetujuan (Approver)
     ─────────────────────────────
     Menunggu step 2 selesai
```

**Anotasi desain MakerReviewerApproverPanel:**
- Step 1 (selesai): collapsed, tampilkan ringkasan: nama user, waktu, komentar (truncate 100ch), link "Lihat detail ▸"
- Step 2 (aktif): expanded penuh, tampilkan action area
- Step 3 (belum): collapsed, warna abu, tidak ada content
- Setiap step: ikon status (✓ selesai / ● aktif / ○ menunggu) di kiri, tidak hanya warna
- Stepper vertikal, tidak horizontal (agar riwayat panjang bisa di-scroll)

---

### State: PENDING_REVIEW dari sudut pandang Maker (SoD blocking)

```
PROSES PERSETUJUAN (4-eyes)
─────────────────────────────────────

  ✓  Step 1: Pembuat (Maker)
     ─────────────────────────────
     Anda — akun.maker
     03 Jun 2026, 08:30 WIB

  ●  Step 2: Pemeriksa (Reviewer)          ← AKTIF
     ─────────────────────────────
     Menunggu review dari Finance Controller

     ┌─────────────────────────────────────────────────────┐
     │ ⓘ Anda tidak bisa mereview data yang Anda buat      │
     │   sendiri (Segregation of Duties / 4-Eyes Policy).  │
     │   Hubungi Finance Controller untuk melanjutkan.     │
     └─────────────────────────────────────────────────────┘

     (tombol Tolak dan Setujui disabled dengan tooltip SoD)

  ○  Step 3: Pemberi Persetujuan (Approver)
```

---

### State: APPROVED (tampilan semua role)

```
PROSES PERSETUJUAN (4-eyes)
─────────────────────────────────────

  ✓  Step 1: Pembuat (Maker)
     ─────────────────────────────
     akun.maker — ROLE-AKUN
     03 Jun 2026, 08:30 WIB
     [Lihat detail ▸]

  ✓  Step 2: Pemeriksa (Reviewer)
     ─────────────────────────────
     akun.ctl.1 — ROLE-AKUN-CTL
     03 Jun 2026, 09:00 WIB
     "Review OK — kode ISO valid, decimal places sesuai standar."
     Tanda tangan: sha256:abc123... (JWT_STANDARD)
     [Lihat detail ▸]

  ✓  Step 3: Pemberi Persetujuan (Approver)
     ─────────────────────────────
     akun.ctl.2 — ROLE-AKUN-CTL
     03 Jun 2026, 10:00 WIB
     "Disetujui. Mata uang GBP aktif per 2026-06-03."
     Tanda tangan: sha256:def456... (JWT_STANDARD)
     [Lihat detail ▸]

─────────────────────────────────────
  ● Disetujui pada 03 Jun 2026, 10:00 WIB
    Mata uang GBP sekarang aktif dan dapat digunakan.
```

---

### State: RETURNED (tampilan Maker)

```
PROSES PERSETUJUAN (4-eyes)
─────────────────────────────────────

  ✓  Step 1: Pembuat (Maker)
     ─────────────────────────────
     Anda — akun.maker
     03 Jun 2026, 08:30 WIB

  ↩  Step 2: Pemeriksa (Reviewer) — DIKEMBALIKAN
     ─────────────────────────────
     akun.ctl.1 — ROLE-AKUN-CTL
     03 Jun 2026, 09:15 WIB

     ALASAN PENOLAKAN:
     "Kode GBP sudah ada di sistem dengan nama berbeda.
      Harap verifikasi nama resmi sesuai ISO 4217."

  ○  Step 3: Pemberi Persetujuan (Approver)
     ─────────────────────────────
     (menunggu proses review ulang)

─────────────────────────────────────
TINDAKAN ANDA:
  Data ini dikembalikan untuk diperbaiki.

  [Edit & Perbaiki]    [Kirim Ulang untuk Review]

  Kirim Ulang hanya tersedia setelah Anda
  menyimpan perubahan terlebih dahulu.
```

---

## ApprovalWithSignature Panel — Detail Komponen

```
TINDAKAN:
─────────────────────────────────────────────────────────────

Komentar
┌──────────────────────────────────────────────────────────┐
│                                                          │
│  Review OK — kode ISO valid...                           │
│                                                          │
│  (Komentar opsional untuk Setujui, WAJIB untuk Tolak)    │
└──────────────────────────────────────────────────────────┘
Sisa: 967 karakter

┌──────────────────────────────────────────────────────────┐
│ ☐ Saya telah memeriksa data mata uang ini dan            │
│   menyatakan bahwa informasi yang diberikan adalah       │
│   benar dan sesuai dengan standar yang berlaku.          │
└──────────────────────────────────────────────────────────┘

[Tolak ▸]                         [Setujui & Lanjutkan →]
```

**Aturan tombol:**
- "Setujui & Lanjutkan" aktif hanya jika: checkbox attest dicentang + SoD OK
- "Tolak" selalu aktif jika SoD OK, membuka sub-panel reject (lihat bawah)
- Jika SoD violation: kedua tombol disabled + pesan SoD di atas tombol

**Reject sub-panel** (muncul inline saat klik "Tolak", bukan modal — anti-pattern modal stacking):
```
TOLAK & KEMBALIKAN KE MAKER
─────────────────────────────

Alasan Penolakan *
┌──────────────────────────────────────────────────────────┐
│                                                          │
│  Kode GBP sudah ada di sistem...                         │
│                                                          │
└──────────────────────────────────────────────────────────┘
Minimal 10 karakter. Wajib diisi.     Sisa: 920 karakter

┌──────────────────────────────────────────────────────────┐
│ ☐ Saya menyatakan bahwa penolakan ini berdasarkan        │
│   pertimbangan yang valid.                               │
└──────────────────────────────────────────────────────────┘

[Batal]                           [Tolak & Kembalikan ↩]
```

"Tolak & Kembalikan" aktif hanya jika: komentar min 10 char + checkbox dicentang.

---

## MFA Step-Up Prompt (untuk modul yang butuh)

Tidak berlaku untuk `mata_uang` (stepUpRequired = false). Berlaku untuk: AKUN-CTL approve periode_buku, ALCO approve ECL param.

Saat user klik "Setujui" dan server return MFA challenge:

```
┌──────────────────────────────────────────────────────────────┐
│  Verifikasi Tambahan Diperlukan                              │
│  ──────────────────────────────────────────────────────────  │
│                                                              │
│  Aksi ini memerlukan konfirmasi identitas ulang.             │
│                                                              │
│  Kode OTP (dari aplikasi autentikator)                       │
│  ┌──────────────────────────────────────┐                    │
│  │  _ _ _ _ _ _                         │                    │
│  └──────────────────────────────────────┘                    │
│                                                              │
│  [Batal]                         [Verifikasi →]             │
│                                                              │
│  Setelah terverifikasi, persetujuan Anda akan diproses.      │
└──────────────────────────────────────────────────────────────┘
```

Ini adalah inline prompt di dalam panel workflow (bukan modal baru). Muncul menggantikan komentar + attest + tombol sementara MFA verification berlangsung.

---

## Tombol Header Halaman Detail

```
Mata Uang: Pound Sterling (GBP)    ◑ Menunggu Review    [Edit] [•••]
```

**[Edit]**: muncul jika user punya permission update + status DRAFT/RETURNED  
**[•••] DropdownMenu**:
- Kirim untuk Review — jika DRAFT + maker
- Kirim Ulang — jika RETURNED + maker
- Hapus — jika DRAFT + permission delete, dengan confirm dialog
- Nonaktifkan / Aktifkan — toggle aktif_flag via workflow singkat
- Lihat Riwayat Audit

---

## Riwayat Audit Trail Tab

Di bawah detail entitas, link "Lihat Riwayat Audit →" menuju `/master/mata-uang/GBP/history`.

Format tabel riwayat:
```
┌──────────────────────────────────────────────────────────────────────────────┐
│ Waktu             │ Aksi           │ Dilakukan Oleh │ Komentar              │
│───────────────────┼────────────────┼────────────────┼───────────────────────│
│ 03 Jun, 10:00 WIB │ APPROVE        │ akun.ctl.2     │ Disetujui. Mata...    │
│ 03 Jun, 09:00 WIB │ REVIEW         │ akun.ctl.1     │ Review OK — kode...   │
│ 03 Jun, 08:30 WIB │ SUBMIT         │ akun.maker     │ -                     │
│ 03 Jun, 08:00 WIB │ CREATE         │ akun.maker     │ -                     │
└──────────────────────────────────────────────────────────────────────────────┘
```

before_jsonb / after_jsonb: tampil sebagai collapsible section "Perubahan Data" hanya untuk ROLE-AUDIT.

---

## Komponen yang Dipakai

| Komponen | Sumber | Catatan |
|---|---|---|
| `MakerReviewerApproverPanel` | `components/blips/MakerReviewerApproverPanel.tsx` | **BARU — perlu dibuat** |
| `ApprovalWithSignature` | `components/blips/ApprovalWithSignature.tsx` | **BARU — perlu dibuat** |
| `WorkflowStatusBadge` | `components/blips/WorkflowStatusBadge.tsx` | **BARU** (dari wireframe list) |
| `ReturnedBanner` | `components/blips/ReturnedBanner.tsx` | **BARU — perlu dibuat** |
| `SodBlockBanner` | `components/blips/SodBlockBanner.tsx` | **BARU — perlu dibuat** |
| `MfaStepUpPrompt` | `components/blips/MfaStepUpPrompt.tsx` | **BARU — perlu dibuat** |
| `AuditHistoryTable` | `components/blips/AuditHistoryTable.tsx` | **BARU — perlu dibuat** |
| `notify` | `lib/notify.ts` | Phase 2, sudah ada |
