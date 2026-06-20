# P5-M3 — GL Host Delivery UI Design Specification

**Story Set**: P5-M3
**Modul**: APP-D — GL Interface (Phase 5, Module 3)
**Desainer**: uiux-designer
**Tanggal**: 2026-06-17
**Status**: READY FOR HANDOFF
**Linked Stories**: `docs/stories/phase-5/P5-M3-gl-delivery.md`
**Linked API**: `api/openapi/app-d-gl-delivery.yaml`
**Linked State Machine**: `docs/state-machines/p5-m3-gl-delivery.md`
**Decisions applied**:
- DEC-005 (GL Integration Phase 2 REST real-time, aktif P5-M3)
- DEC-007 (Asynq job queue — delivery dan recon workers)
- DEC-018 (Audit trail append-only, 10+10 tahun retensi)
- DEC-021 (Idempotency-Key wajib di setiap mutating endpoint)
- DEC-022 (Cursor-only pagination)
- DEC-027 (MFA step-up TIDAK diperlukan untuk GL delivery routine)
- DEC-030 RESOLVED (GL delivery mode = Async REST via Asynq)
- OQ-M3-5a (DLQ GL delivery = tabel `sys.dlq_gl_delivery`, bukan view)

**Dependensi**: P5-M2 (`/jrnl/journal-entries/[id]` screen) — P5-M3 meng-embed panel ke dalam halaman detail P5-M2.

---

## 1. Screen Inventory

### 1.1 Sitemap P5-M3

```
Jurnal (side nav group — extension dari P5-M2)
├── Journal Entries
│   └── /jrnl/journal-entries/[id]        — P5-M2 detail screen +
│                                           <GlDeliveryStatusPanel> embedded (S2)
│                                           + Retry dialog (S3)
├── GL Delivery DLQ                        [badge merah jika ada FAILED]
│   ├── /jrnl/gl-delivery-dlq             — DLQ list console (S5)
│   └── /jrnl/gl-delivery-dlq/[id]        — DLQ detail + replay/discard (S5)
└── Rekonsiliasi GL
    ├── /jrnl/rekonsiliasi                 — Rekonsiliasi dashboard hari ini + trigger (S4)
    └── /jrnl/rekonsiliasi/riwayat         — Riwayat laporan + mismatch drill-down (S4)
```

### 1.2 Navigasi Side Nav (tambahan ke P5-M2)

```
Jurnal
  ▾ ...P5-M2 items (tidak berubah)...
  ─
  GL Delivery DLQ  → /jrnl/gl-delivery-dlq    (ROLE-IT-ADMIN, ROLE-AKUN-CTL)
                      [badge merah angka jika ada FAILED entries]
  ─
  Rekonsiliasi GL  → /jrnl/rekonsiliasi        (ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT)
```

**GL DLQ Badge**: sama polanya dengan DLQ badge P5-M2 — badge merah di top nav bar dan di side nav item. Menampilkan total entri `sys.dlq_gl_delivery WHERE status = 'FAILED'`. Polling setiap 60 detik (SSE subscribe ke channel `gl-dlq-count` jika available).

### 1.3 AC Mapping

| Screen | Route | Persona Utama | Story Ref | AC Yang Tercakup |
|---|---|---|---|---|
| GlDeliveryStatusPanel (embedded) | `/jrnl/journal-entries/[id]` | ROLE-AKUN, ROLE-AKUN-CTL | S2 | S2-AC1, S2-AC2, S2-AC3 |
| RetryDialog | modal dalam `/jrnl/journal-entries/[id]` | ROLE-AKUN-CTL, ROLE-IT-ADMIN | S3 | S3-AC1, S3-AC2, S3-AC3, S3-AC4 |
| Reconciliation Dashboard | `/jrnl/rekonsiliasi` | ROLE-AKUN-CTL, ROLE-CFO | S4 | S4-AC1, S4-AC2, S4-AC3, S4-AC4 |
| Reconciliation History | `/jrnl/rekonsiliasi/riwayat` | ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT | S4 | S4-AC1, S4-AC3 |
| GL Delivery DLQ List | `/jrnl/gl-delivery-dlq` | ROLE-IT-ADMIN, ROLE-AKUN-CTL | S5 | S5-AC1, S5-AC2, S5-AC3, S5-AC4 |
| GL Delivery DLQ Detail | `/jrnl/gl-delivery-dlq/[id]` | ROLE-IT-ADMIN, ROLE-AKUN-CTL | S5 | S5-AC2, S5-AC3, S5-AC4 |

---

## 2. Status Badge Design — `gl_host_status`

Semua badge menggunakan warna + ikon + teks label. Warna bukan satu-satunya sinyal (WCAG 2.1 AA).

| `gl_host_status` | Warna | Ikon | Label ID | Label EN (export) | Keterangan |
|---|---|---|---|---|---|
| `PENDING_DELIVERY` | Abu-abu (slate-400) | clock | Menunggu Pengiriman | Pending Delivery | Antri di Asynq, belum di-pick up worker |
| `DELIVERY_IN_FLIGHT` | Biru (blue-500) | loader-2 (spin) | Sedang Dikirim | Delivering | Worker sedang POST ke GL Host |
| `RETRYING` | Amber (amber-500) | refresh-cw | Sedang Retry | Retrying | Infra error, menunggu backoff |
| `FAILED` | Merah (red-600) | x-circle | Gagal — Delivery | Failed | Domain atau infra error terminal |
| `DELIVERED` | Hijau (green-600) | check-circle-2 | Terkirim ke GL | Delivered | GL Host konfirmasi |
| `DEAD_LETTER` | Merah gelap (red-900) | skull | Dihentikan — DLQ | Dead Letter | Discarded, tidak bisa di-retry |

**DLQ `status` badges** (untuk tabel DLQ list):

| `status` | Warna | Ikon | Label ID |
|---|---|---|---|
| `FAILED` | Merah (red-600) | x-circle | Gagal |
| `REPLAYING` | Biru (blue-500) | refresh-cw (spin) | Sedang Replay |
| `REPLAYED_OK` | Hijau (green-600) | check-circle-2 | Replay Berhasil |
| `ABANDONED` | Slate (slate-600) | archive-x | Dihentikan |

**`failure_category` badges** (secondary, tampil di samping status):

| `failure_category` | Warna | Label ID |
|---|---|---|
| `DOMAIN` | Merah solid outline | Domain Error |
| `INFRA` | Amber solid outline | Infra Error |

**Mismatch type badges** (untuk reconciliation mismatch table):

| `mismatch_type` | Warna | Ikon | Label ID |
|---|---|---|---|
| `BLIPS_ONLY` | Amber (amber-600) | arrow-right-from-line | Hanya di BLIPS |
| `GL_ONLY` | Biru (blue-600) | arrow-left-from-line | Hanya di GL Host |
| `AMOUNT_DIFF` | Merah (red-500) | triangle-alert | Selisih Jumlah |

---

## 3. Wireframes — 6 Screens

### SCREEN-P5-M3-01: GL Delivery Status Panel (embedded dalam Journal Entry Detail)

**Route**: `/jrnl/journal-entries/[id]` — section baru di bawah baris D/K

**AC**: S2-AC1 (DELIVERED), S2-AC2 (FAILED + can_retry), S2-AC3 (PENDING_DELIVERY)

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ ...header + baris D/K dari P5-M2 SCREEN-P5-M2-06...                                     │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                          │
│  STATUS PENGIRIMAN KE GL HOST                                                            │
│  ─────────────────────────────────────────────────────────────────────────────────────── │
│                                                                                          │
│  STATE A — DELIVERED (hijau)                                                             │
│  ┌─────────────────────────────────────────────────────────────────────────────────┐     │
│  │ [✓ check-circle-2] Terkirim ke GL                                               │     │
│  │                                                                                 │     │
│  │  GL Journal ID  : GLHOST-JRN-20260615-00001                                    │     │
│  │  Waktu Terkirim : 15 Jun 2026, 10:32:44                                        │     │
│  │  Mode Pengiriman: API (REST)                                                   │     │
│  │  Jumlah Retry   : 0                                                            │     │
│  └─────────────────────────────────────────────────────────────────────────────────┘     │
│                                                                                          │
│  STATE B — FAILED (merah), can_retry = true                                             │
│  ┌─────────────────────────────────────────────────────────────────────────────────┐     │
│  │ [✗ x-circle] Gagal — Delivery  [Domain Error]                                  │     │
│  │                                                                                 │     │
│  │  Penyebab      : GL_HOST_REJECTED — INVALID_ACCOUNT_CODE                       │     │
│  │  Detail        : Account 1110-DEP not found in GL chart                        │     │
│  │  Kategori      : [Domain Error]                                                 │     │
│  │  Jumlah Retry  : 0                                                             │     │
│  │                                                                                 │     │
│  │  ⚠ Jurnal ini gagal dikirim ke GL Host. Perbaiki penyebab kegagalan            │     │
│  │    sebelum melakukan retry.                                                     │     │
│  │                                                                                 │     │
│  │                               [Retry Pengiriman ↺]  (ROLE-AKUN-CTL + IT-ADMIN)│     │
│  └─────────────────────────────────────────────────────────────────────────────────┘     │
│                                                                                          │
│  STATE C — PENDING_DELIVERY (abu-abu)                                                   │
│  ┌─────────────────────────────────────────────────────────────────────────────────┐     │
│  │ [○ clock] Menunggu Pengiriman                                                   │     │
│  │                                                                                 │     │
│  │  Jurnal ini sudah diposting dan sedang antri untuk dikirim ke GL Host.         │     │
│  │  Proses pengiriman otomatis berjalan dalam beberapa momen.                     │     │
│  │  (Tidak ada tombol retry — pengiriman otomatis berjalan)                       │     │
│  └─────────────────────────────────────────────────────────────────────────────────┘     │
│                                                                                          │
│  STATE D — RETRYING (amber)                                                              │
│  ┌─────────────────────────────────────────────────────────────────────────────────┐     │
│  │ [↺ refresh-cw] Sedang Retry  [Infra Error]                                     │     │
│  │                                                                                 │     │
│  │  Error Terakhir  : GL_HOST_UNREACHABLE — 503 Service Unavailable               │     │
│  │  Percobaan ke-   : 2 dari 3 (maks. otomatis)                                   │     │
│  │  Retry Terakhir  : 15 Jun 2026, 10:15:00                                       │     │
│  │  Retry Berikutnya: sekitar 15 Jun 2026, 10:20:00 (5 menit)                    │     │
│  │  (Tidak ada tombol — retry otomatis sedang berjalan)                           │     │
│  └─────────────────────────────────────────────────────────────────────────────────┘     │
│                                                                                          │
│  STATE E — DEAD_LETTER (merah gelap)                                                    │
│  ┌─────────────────────────────────────────────────────────────────────────────────┐     │
│  │ [☠ skull] Dihentikan — DLQ                                                     │     │
│  │                                                                                 │     │
│  │  Entry ini sudah dihentikan secara permanen (DEAD_LETTER).                     │     │
│  │  Pengiriman otomatis tidak bisa dilakukan lagi.                                │     │
│  │  Alasan: [nilai discard_reason dari DB]                                        │     │
│  │  Dihentikan oleh: [nama user] pada [timestamp]                                 │     │
│  │                                                                                 │     │
│  │  ℹ Jika jurnal ini masih perlu dikirim ke GL Host,                             │     │
│  │    buat jurnal koreksi baru via Posting Manual (CORRECTION_PERIODE_CLOSED).   │     │
│  └─────────────────────────────────────────────────────────────────────────────────┘     │
│                                                                                          │
│  RIWAYAT DELIVERY (collapsible — default collapsed)                                     │
│  ┌─────────────────────────────────────────────────────────────────────────────────┐     │
│  │ [▾ Lihat Riwayat Delivery (3 entri)]                                            │     │
│  │                                                                                 │     │
│  │  Saat di-expand (query aud.audit_log action IN GL_DELIVERY.*):                 │     │
│  │                                                                                 │     │
│  │  15 Jun 2026, 10:32  GL_DELIVERY.SUCCESS     SYSTEM_WORKER                    │     │
│  │  15 Jun 2026, 10:15  GL_DELIVERY.RETRY (1)   SYSTEM_WORKER — 503 infra        │     │
│  │  15 Jun 2026, 10:10  GL_DELIVERY.MANUAL_RETRY_INITIATED  Eko Susanto          │     │
│  │                                                                                 │     │
│  │  (ROLE-AUDIT: melihat semua aksi; ROLE-AKUN: melihat tanpa raw payload)        │     │
│  └─────────────────────────────────────────────────────────────────────────────────┘     │
│                                                                                          │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

**Data yang ditampilkan:**

| Field | Sumber | Visible untuk |
|---|---|---|
| `gl_host_status` | `jrnl.gl_status.gl_host_status` | Semua dengan `jurnal.read` |
| `gl_host_journal_id` | `jrnl.gl_status.gl_host_journal_id` | Semua |
| `delivered_at` | `jrnl.gl_status.delivered_at` | Semua |
| `retry_count` | `jrnl.gl_status.retry_count` | Semua |
| `last_error` | `jrnl.gl_status.last_error` | Semua (summary text) |
| `failure_category` | `jrnl.gl_status.failure_category` | Semua |
| `gl_response_payload_jsonb` | `jrnl.gl_status.gl_response_payload_jsonb` | **ROLE-IT-ADMIN saja** |
| `can_retry` | dari API `delivery_status.can_retry` | Tombol visible jika `true` |
| Delivery history | `aud.audit_log` (GL_DELIVERY.*) | Semua; raw payload = IT-ADMIN only |

**Komponen yang dipakai:**

- `<GlDeliveryStatusPanel>` (komponen baru — §3.1)
- `<GlStatusBadge>` (komponen baru — §3.1)
- `<GlDeliveryHistoryTimeline>` (komponen baru — §3.1)
- **Reuse**: `<JSONBTreeView>` dari P5-M2 untuk payload tampilan ROLE-IT-ADMIN

**Interaksi:**

- Panel di-render setelah baris D/K, sebelum tab Riwayat Audit P5-M2
- Data di-load via subquery pada `GET /api/v1/jurnal/header/{id}` (delivery_status sub-object)
- Auto-refresh setiap 10 detik jika status = `PENDING_DELIVERY` atau `DELIVERY_IN_FLIGHT` atau `RETRYING` (SSE preferred, polling fallback) — berhenti saat terminal state (DELIVERED / DEAD_LETTER)
- Tombol "Retry Pengiriman" membuka `<RetryDialog>` (SCREEN-P5-M3-02)

**Empty state**: tidak ada `jrnl.gl_status` row — panel menampilkan "Status pengiriman belum tersedia. Hubungi IT Admin." (kasus edge jika posting P5-M2 tidak membuat gl_status row)

**Loading state**: skeleton 3 baris tinggi dengan shimmer, lebar 60%

---

### SCREEN-P5-M3-02: Manual Retry Dialog

**Route**: modal dalam `/jrnl/journal-entries/[id]` (tidak route tersendiri — modal inline)

**AC**: S3-AC1 (retry berhasil), S3-AC2 (DEAD_LETTER ditolak), S3-AC3 (reason terlalu pendek), S3-AC4 (ROLE-AKUN permission denied)

```
┌──────────────────────────────────────────────────────────────────────┐
│  Retry Pengiriman ke GL Host                                        [×]│
├──────────────────────────────────────────────────────────────────────┤
│                                                                       │
│  Jurnal         : JRN-2026-000077                                     │
│  Kode Event     : PENEMPATAN                                          │
│  Error Terakhir : GL_HOST_REJECTED — INVALID_ACCOUNT_CODE            │
│  Kategori       : [Domain Error]                                      │
│                                                                       │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │ ⚠ Pastikan kondisi penyebab kegagalan sudah diperbaiki:          │  │
│  │   Domain error: akun GL Host sudah diperbaiki?                  │  │
│  │   Infra error: GL Host sudah kembali online?                    │  │
│  └─────────────────────────────────────────────────────────────────┘  │
│                                                                       │
│  Alasan Retry *                                                       │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │                                                                 │  │
│  │                                                                 │  │
│  └─────────────────────────────────────────────────────────────────┘  │
│  Minimal 30 karakter. Sisa: [counter real-time]                       │
│  Contoh: "Kode akun 1110-DEP sudah diperbaiki di GL Host             │
│           Chart of Accounts pada 2026-06-15. Retry delivery."        │
│                                                                       │
│  Total Percobaan Sebelumnya: 1 dari 5 (maks.)                        │
│                                                                       │
│  ℹ Sistem akan menjadwalkan ulang pengiriman. Pantau hasilnya di     │
│    panel status jurnal ini.                                           │
│                                                                       │
│         [Batal]                      [Jadwalkan Retry ↺]             │
│                                      (disabled sampai ≥ 30 char)     │
└──────────────────────────────────────────────────────────────────────┘
```

**Setelah submit berhasil (202 Accepted):**

```
┌──────────────────────────────────────────────────────────────────────┐
│  Retry Pengiriman ke GL Host                                        [×]│
├──────────────────────────────────────────────────────────────────────┤
│                                                                       │
│  [✓ check-circle-2]  Retry berhasil dijadwalkan                      │
│                                                                       │
│  JRN-2026-000077 sedang antri untuk dikirim ulang ke GL Host.        │
│  Status akan diperbarui otomatis di panel jurnal ini.                │
│                                                                       │
│                                           [Tutup]                    │
└──────────────────────────────────────────────────────────────────────┘
```

**Constraint panel (di bawah tombol, visible saat MAX_ATTEMPTS tercapai):**

```
┌─────────────────────────────────────────────────────────────────────┐
│ ⛔ Batas maksimum percobaan tercapai (5/5).                          │
│    Retry tidak bisa dilakukan lagi.                                 │
│    Jika jurnal ini masih perlu dikirim, hubungi ROLE-IT-ADMIN       │
│    untuk mendiscard entry DLQ dan membuat jurnal koreksi.           │
└─────────────────────────────────────────────────────────────────────┘
```

**Interaksi:**

1. Tombol "Retry Pengiriman" di-klik di GlDeliveryStatusPanel — membuka dialog (tidak stack modal)
2. Textarea reason: tombol "Jadwalkan Retry" disabled sampai input ≥ 30 karakter (counter real-time)
3. Submit: disable tombol + spinner inline (block double-submit)
4. Success (202): tampilkan state sukses di dalam dialog, panel GlDeliveryStatusPanel di parent di-refresh (optimistic update status ke PENDING_DELIVERY)
5. Toast hijau: "Retry delivery JRN-2026-000077 berhasil dijadwalkan. Pantau status di panel jurnal."
6. Error 422 `GL_DELIVERY_INVALID_TRANSITION` (DEAD_LETTER): dialog tidak bisa dibuka (tombol disabled di panel dengan tooltip "Status DEAD_LETTER — tidak bisa di-retry")
7. Error 422 `GL_DELIVERY_MAX_ATTEMPTS_EXCEEDED`: tampilkan constraint panel merah di dalam dialog, tombol submit disabled
8. Error 400 `GL_DELIVERY_REASON_TOO_SHORT`: inline error di bawah textarea (aria-describedby)
9. Error 403 `GL_DELIVERY_PERMISSION_DENIED`: tombol "Retry Pengiriman" tidak tampil (hidden — tidak display:none saja, perlu tidak ada di DOM)

**Catatan SoD**: GL delivery TIDAK memiliki SoD (tidak ada maker/reviewer/approver chain). Siapa pun dengan permission `jurnal.gl_delivery.retry` bisa retry.

---

### SCREEN-P5-M3-03: Reconciliation Dashboard

**Route**: `/jrnl/rekonsiliasi`

**AC**: S4-AC1 (COMPLETED), S4-AC2 (COMPLETED_WITH_MISMATCH), S4-AC3 (read laporan), S4-AC4 (GL Host down — FAILED)

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ PAGE HEADER                                                                              │
│  Rekonsiliasi GL Host                                                                    │
│  Perbandingan BLIPS vs GL Host per tanggal                                               │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                          │
│  SECTION — Pilih Tanggal + Trigger                                                       │
│  ┌───────────────────────────────────────────────────────────────────────────────────┐   │
│  │ Tanggal Rekonsiliasi *                                                             │   │
│  │ [14 Jun 2026 (Hari Kerja)      ▾]    [Lihat Laporan]                             │   │
│  │                                                                                   │   │
│  │ Rekonsiliasi terakhir: 14 Jun 2026, 08:05 (auto cron)                            │   │
│  │                                                                                   │   │
│  │ (ROLE-AKUN-CTL saja:)                                                             │   │
│  │ [Jalankan Rekonsiliasi Manual ▶]  (POST /jurnal/reconciliation/run)              │   │
│  │  Akan me-overwrite laporan yang sudah ada untuk tanggal ini.                     │   │
│  └───────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                          │
│  SECTION — Summary Card (hasil laporan tanggal terpilih)                                 │
│  ─────────────────────────────────────────────────────────────────────────────────────── │
│                                                                                          │
│  STATE A — COMPLETED (hijau) — tidak ada mismatch                                       │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐    │
│  │ [✓] SESUAI — Rekonsiliasi 14 Jun 2026                                            │    │
│  │                                                                                  │    │
│  │ Akun Diperiksa  : 28 akun          Mismatch      : 0 akun                       │    │
│  │ Total BLIPS     : Rp 12.500.000.000 Total GL Host : Rp 12.500.000.000           │    │
│  │ Selisih         : Rp 0             Toleransi     : Rp 1,00                      │    │
│  │ Dihasilkan      : 15 Jun 2026, 08:05:12  (auto cron)                            │    │
│  └──────────────────────────────────────────────────────────────────────────────────┘    │
│                                                                                          │
│  STATE B — COMPLETED_WITH_MISMATCH (amber) — ada mismatch                              │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐    │
│  │ [⚠] MISMATCH — Rekonsiliasi 14 Jun 2026                    [Export CSV / XLSX ▾]│    │
│  │                                                                                  │    │
│  │ Akun Diperiksa  : 28 akun          Mismatch      : 2 akun                       │    │
│  │ Total BLIPS     : Rp 12.500.000.000 Total GL Host : Rp 12.499.985.000           │    │
│  │ Selisih         : Rp 15.000         Toleransi     : Rp 1,00                     │    │
│  │ Dihasilkan      : 15 Jun 2026, 08:05:12  (auto cron)                            │    │
│  │                                                                                  │    │
│  │  ⚠ Tindak lanjut diperlukan untuk 2 akun berikut:                               │    │
│  ├──────────────────────────────────────────────────────────────────────────────────┤    │
│  │ DETAIL MISMATCH                                                                  │    │
│  │                                                                                  │    │
│  │ Kode Akun ↕  Nama Akun ↕       Tipe            BLIPS (IDR) ↕  GL Host (IDR) ↕  │    │
│  │ ─────────────────────────────────────────────────────────────────────────────── │    │
│  │ 3010-OCI-AST  Aset OCI    [Hanya di BLIPS]  1.000.000,00   0,00                │    │
│  │               [2 jurnal terkait →]                                              │    │
│  │ 1210-OBLIGASI Obligasi    [Selisih Jumlah]  5.014.000,00   5.000.000,00        │    │
│  │               [3 jurnal terkait →]                                              │    │
│  │ ─────────────────────────────────────────────────────────────────────────────── │    │
│  │ TOTAL MISMATCH: 2 akun | Selisih total: Rp 15.000,00                           │    │
│  └──────────────────────────────────────────────────────────────────────────────────┘    │
│                                                                                          │
│  STATE C — FAILED (merah) — GL Host tidak bisa di-reach                                 │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐    │
│  │ [✗] GAGAL — Rekonsiliasi 14 Jun 2026                                            │    │
│  │                                                                                  │    │
│  │ Rekonsiliasi gagal karena GL Host tidak dapat dijangkau saat cron berjalan.    │    │
│  │ Detail: GL_RECONCILIATION_HOST_FETCH_FAILED — 503 Service Unavailable          │    │
│  │ Waktu: 15 Jun 2026, 08:05:00                                                    │    │
│  │                                                                                  │    │
│  │ Tidak ada data mismatch untuk tanggal ini (rekonsiliasi belum selesai).         │    │
│  │                                                                                  │    │
│  │ ℹ Jalankan rekonsiliasi manual setelah GL Host kembali online.                  │    │
│  │                        [Jalankan Rekonsiliasi Manual ▶] (ROLE-AKUN-CTL saja)   │    │
│  └──────────────────────────────────────────────────────────────────────────────────┘    │
│                                                                                          │
│  STATE D — Belum ada laporan untuk tanggal ini                                          │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐    │
│  │ [○] Belum Ada Laporan                                                            │    │
│  │                                                                                  │    │
│  │ Belum ada rekonsiliasi untuk 14 Jun 2026.                                       │    │
│  │ Cron otomatis berjalan setiap hari pukul 08:00 WIB.                            │    │
│  │                        [Jalankan Rekonsiliasi Manual ▶] (ROLE-AKUN-CTL saja)   │    │
│  └──────────────────────────────────────────────────────────────────────────────────┘    │
│                                                                                          │
│  SECTION — Jurnal Terkait per Akun Mismatch (drill-down)                               │
│  (tampil saat baris mismatch di-klik — expand inline, bukan modal baru)                │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐    │
│  │ ▾ Jurnal terkait akun 3010-OCI-AST pada 14 Jun 2026 (2 jurnal)                 │    │
│  │                                                                                  │    │
│  │ JRN-2026-000201  ECL_PEMBENTUKAN  14 Jun 2026  Rp 800.000,00  [→ Lihat jurnal] │    │
│  │ JRN-2026-000205  STAGE_MIGRATION  14 Jun 2026  Rp 200.000,00  [→ Lihat jurnal] │    │
│  └──────────────────────────────────────────────────────────────────────────────────┘    │
│                                                                                          │
│  LINK: [→ Lihat Riwayat Semua Rekonsiliasi]  (/jrnl/rekonsiliasi/riwayat)              │
│                                                                                          │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

**Data yang ditampilkan:**

| Field | Sumber | Catatan |
|---|---|---|
| Summary header | `sys.gl_reconciliation_report` | 1 row per tanggal |
| Mismatch rows | `sys.gl_recon_mismatch JOIN mst.chart_of_accounts` | MAX 200 rows inline |
| Jurnal terkait per akun | `jrnl.header WHERE id = ANY(jurnal_header_ids)` | Drill-down |
| Progress job | `sys.job` via SSE/polling | Saat rekonsiliasi berjalan |

**Long-running: Rekonsiliasi Manual (pattern §3 UX)**

Saat tombol "Jalankan Rekonsiliasi Manual" diklik:

```
┌──────────────────────────────────────────────────────────────────────┐
│  Jalankan Rekonsiliasi Manual?                                      [×]│
├──────────────────────────────────────────────────────────────────────┤
│  Tanggal       : 14 Jun 2026                                          │
│                                                                       │
│  ⚠ Jika sudah ada laporan untuk tanggal ini, laporan lama akan       │
│    digantikan dengan hasil rekonsiliasi baru.                        │
│                                                                       │
│           [Batal]                 [Jalankan ▶]                       │
└──────────────────────────────────────────────────────────────────────┘
```

Setelah confirm → POST → 202 → `<JobProgressPanel>`:

```
┌──────────────────────────────────────────────────────────────────────┐
│ ↺  Rekonsiliasi GL — 14 Jun 2026                                     │
│                                                                       │
│  ████████████░░░░░░░░░░  47%                                         │
│                                                                       │
│  Membandingkan data... (14 dari 28 akun selesai diperiksa)          │
│                                                                       │
│  Mulai: 10:30:00 · ETA: 10:32:00 (2 menit lagi)                     │
│                                                                       │
│                                        [ Background ]               │
└──────────────────────────────────────────────────────────────────────┘
```

Setelah selesai: toast sukses (atau alert jika mismatch) + panel summary di-refresh.

**Filter tanggal:**

- DatePicker: hanya hari kerja (disable hari libur dari `sys.calendar_holiday`)
- Default: hari kerja terakhir (D-1 jika cron sudah berjalan, atau D jika sebelum jam 08:00 WIB)
- Hari libur tooltip: "Hari libur — rekonsiliasi tidak berjalan pada hari ini"

**Kolom mismatch table:**

| Kolom | Sort | Filter | Catatan |
|---|---|---|---|
| `kode_akun` | Ya | Text | |
| `nama_akun` | Ya | Text | |
| `mismatch_type` | Ya | Select: BLIPS_ONLY/GL_ONLY/AMOUNT_DIFF | Badge + ikon |
| `blips_amount_idr` | Ya | gte/lte | Format IDR |
| `gl_host_amount_idr` | Ya | gte/lte | Format IDR |
| `delta_idr` | Ya | gte/lte | Selalu tampil absolute value + sign |
| Jurnal terkait | Tidak | — | Link expand drill-down |

**Export laporan mismatch** (tombol di STATE B header):
- CSV / XLSX — 3 sheet: summary, mismatch detail, jurnal terkait per akun
- Header row bahasa Indonesia, amount format `#,##0.0000`
- Audit: `GL_RECONCILIATION.EXPORT` (entitas bulk)

**Empty/Loading/Error states:**

| State | Tampilan |
|---|---|
| Loading laporan | Skeleton card 4 baris + tabel skeleton |
| Tidak ada laporan (belum di-run) | STATE D card di atas |
| FAILED rekon | STATE C card |
| Error API (network) | Card merah + "Gagal memuat laporan. [Coba lagi]" |

---

### SCREEN-P5-M3-04: Reconciliation History

**Route**: `/jrnl/rekonsiliasi/riwayat`

**AC**: S4-AC1, S4-AC3

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ PAGE HEADER                                                                              │
│  Riwayat Rekonsiliasi GL Host                        [Export ▾ CSV / XLSX]              │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│ FILTER BAR                                                                               │
│ [Tgl Awal ─ Tgl Akhir]  [Status ▾]  [Hanya Mismatch ☐]             [Clear semua]       │
│ Filter chips: [Status: COMPLETED_WITH_MISMATCH ×]                                       │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│ ACTION BAR: [Export ▾ CSV / XLSX]  [Refresh ↺]  "Diperbarui: 17 Jun 2026, 08:05"       │
├──────────────────┬──────────────┬──────────────────────────────────┬────────────────────┤
│ Tgl Rekon ↕      │ Status       │ Ringkasan                         │ Mismatch (IDR) ↕  │
├──────────────────┼──────────────┼──────────────────────────────────┼────────────────────┤
│ 17 Jun 2026      │[hijau]SESUAI │ 28 akun · Rp 15.200.000.000     │ 0                  │
│ 16 Jun 2026      │[amber]MISMAT │ 28 akun · 2 mismatch            │ Rp 15.000,00 [→]  │
│ 15 Jun 2026      │[hijau]SESUAI │ 27 akun · Rp 12.500.000.000     │ 0                  │
│ 14 Jun 2026      │[merah]GAGAL  │ GL Host tidak tersedia            │ —                  │
│ 13 Jun 2026      │[hijau]SESUAI │ 26 akun · Rp 11.800.000.000     │ 0                  │
├──────────────────┴──────────────┴──────────────────────────────────┴────────────────────┤
│ Footer: [← Prev]  Hal. 1 dari ~3  [Next →]  Baris: [50 ▾]  Total estimasi: 126         │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

**Kolom DataTable:**

| ID Kolom | Header | Sort | Filter |
|---|---|---|---|
| `tanggal_rekonsiliasi` | Tgl Rekon | Ya | Date range |
| `status` | Status | Ya | Select: COMPLETED/COMPLETED_WITH_MISMATCH/FAILED |
| `total_akun_checked` | Akun Diperiksa | Ya | gte/lte |
| `blips_total_idr` | Total BLIPS | Ya | gte/lte |
| `total_mismatch_count` | Mismatch | Ya | gte/lte, filter toggle "Hanya Mismatch" |
| `total_mismatch_amount_idr` | Selisih (IDR) | Ya | gte/lte |
| `generated_at` | Dihasilkan | Ya | Date range |
| `triggered_by` | Dipicu Oleh | Tidak | Select: CRON/MANUAL |

**Status badges riwayat:**

| `status` | Warna | Label ID |
|---|---|---|
| `COMPLETED` | Hijau | Sesuai |
| `COMPLETED_WITH_MISMATCH` | Amber | Ada Mismatch |
| `FAILED` | Merah | Gagal |
| `RUNNING` | Biru (spinner) | Berjalan |

**Row action**: klik baris → navigasi ke `/jrnl/rekonsiliasi?date=YYYY-MM-DD` (dashboard dengan tanggal terpilih). Tombol "→" di kolom mismatch membuka `/jrnl/rekonsiliasi?date=YYYY-MM-DD#mismatch-detail`.

**Export**: CSV/XLSX riwayat rekonsiliasi. Dataset > 10k baris → async export via MinIO (pattern §1.4).

---

### SCREEN-P5-M3-05: GL Delivery DLQ List

**Route**: `/jrnl/gl-delivery-dlq`

**AC**: S5-AC1 (lihat DLQ list), S5-AC2 (replay), S5-AC3 (discard), S5-AC4 (ROLE-AKUN-CTL tidak bisa discard)

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ PAGE HEADER                                                                              │
│  GL Delivery — Dead Letter Queue                                                         │
│  [⚠] 8 entri FAILED membutuhkan perhatian          [Refresh ↺]                         │
│  (Hanya ROLE-IT-ADMIN dan ROLE-AKUN-CTL)                                                │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│ FILTER BAR                                                                               │
│ [🔍 Cari no_jurnal / kode event...]  [Status ▾]  [Kategori ▾]  [Kode Error ▾]  [Tgl─Tgl│
│ Default filter: Status = FAILED (default open, tidak include REPLAYED_OK atau ABANDONED)│
│ Filter chips: [Status: FAILED ×]  [Kategori: DOMAIN ×]             [Clear semua]        │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│ ACTION BAR: [Export ▾ CSV / XLSX]  [Refresh ↺]  "Diperbarui: 17 Jun 2026, 09:12"       │
├──────────────┬───────────────┬────────────────┬──────────────┬────────────┬─────────────┤
│ No. Jurnal ↕ │ Kode Event ↕  │ Kategori Error │ Kode Error   │ Retry ↕    │ Status      │
├──────────────┼───────────────┼────────────────┼──────────────┼────────────┼─────────────┤
│JRN-2026-00077│ PENEMPATAN    │[merah]Domain   │GL_HOST_REJ.. │ 0          │[merah]GAGAL │
│ 14 Jun 2026  │               │                │              │            │ [Lihat ▸]   │
│              │               │                │              │            │ [Replay]    │
│              │               │                │              │            │ [Discard ⚠] │
├──────────────┼───────────────┼────────────────┼──────────────┼────────────┼─────────────┤
│JRN-2026-00088│ AKRUAL_BUNGA  │[amber]Infra    │GL_HOST_UNREA.│ 3          │[merah]GAGAL │
│ 14 Jun 2026  │               │                │              │            │ [Lihat ▸]   │
│              │               │                │              │            │ [Replay]    │
│              │               │                │              │            │ [Discard ⚠] │
├──────────────┼───────────────┼────────────────┼──────────────┼────────────┼─────────────┤
│JRN-2026-00099│ ECL_PEMBENTUK │[amber]Infra    │GL_HOST_UNREA.│ 3          │[biru] REPLAY│
│ 15 Jun 2026  │               │                │              │            │ ING ↺        │
├──────────────┴───────────────┴────────────────┴──────────────┴────────────┴─────────────┤
│ Footer: [← Prev]  Hal. 1 dari 1  [Next →]  Baris: [50 ▾]  Total estimasi: 8            │
│                                                                                          │
│  [Tampilkan REPLAYED_OK ☐]  [Tampilkan ABANDONED ☐]  (toggle — default off)            │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

**Kolom DataTable:**

| ID Kolom | Header | Sort | Filter |
|---|---|---|---|
| `no_jurnal` | No. Jurnal | Ya | Text search |
| `tanggal_posting` | Tgl Posting | Ya | Date range |
| `event_code` | Kode Event | Ya | Select multi |
| `failure_category` | Kategori Error | Ya | Select: DOMAIN/INFRA |
| `error_code` | Kode Error | Ya | Select multi (dari enum error catalog) |
| `attempt_count` | Retry | Ya | gte/lte |
| `status` | Status | Ya | Select: FAILED/REPLAYING/REPLAYED_OK/ABANDONED |
| `last_retry_at` | Retry Terakhir | Ya | Date range |
| `manual_retry_by_name` | Di-retry oleh | Tidak | Text |

**Row actions (per baris):**

| Aksi | Permission | Kondisi | Keterangan |
|---|---|---|---|
| "Lihat ▸" | `jurnal.gl_delivery.read` | Semua status | Navigasi ke detail |
| "Replay" | `jurnal.gl_delivery.replay` | status = FAILED AND total attempts < max | Membuka ReplayDialog |
| "Discard ⚠" | `jurnal.gl_delivery.discard` | status = FAILED | **ROLE-IT-ADMIN SAJA** — hidden untuk AKUN-CTL |

**Visibility rules per role:**

- ROLE-AKUN-CTL: tampil tombol "Replay", tidak tampil "Discard ⚠"
- ROLE-IT-ADMIN: tampil "Replay" + "Discard ⚠"
- ROLE-AUDIT: hanya "Lihat ▸" (read-only), DLQ page accessible via filter `?filter[gl_host_status]=DEAD_LETTER`

**DLQ List "Tampilkan ABANDONED"**: toggle untuk melihat entri yang sudah di-discard. Default off (auditor harus eksplisit pilih untuk melihat DEAD_LETTER records).

**Perbedaan dengan P5-M2 DLQ** — ditampilkan sebagai info banner di halaman:

```
┌──────────────────────────────────────────────────────────────────────────────────────┐
│ ℹ  Ini adalah DLQ untuk pengiriman jurnal ke GL Host (REST delivery failure).        │
│    Untuk kegagalan pembuatan jurnal (posting failure), lihat:                        │
│    → Dead Letter Queue Jurnal (/jrnl/dlq)                                            │
└──────────────────────────────────────────────────────────────────────────────────────┘
```

**Export**: CSV/XLSX list DLQ. Audit: `GL_DELIVERY.DLQ_EXPORT`. PII-sanitized (tidak ada nomor rekening, NPWP di exported data).

**Empty state**: jika tidak ada FAILED entries — tampilkan "Tidak ada entri DLQ GL Delivery" dengan ikon checkmark hijau besar (tanda sistem sehat) + sub-pesan "Semua jurnal berhasil dikirim ke GL Host."

---

### SCREEN-P5-M3-06: GL Delivery DLQ Detail

**Route**: `/jrnl/gl-delivery-dlq/[id]`

**AC**: S5-AC1 (inspect), S5-AC2 (replay), S5-AC3 (discard), S5-AC4 (403 untuk CTL pada discard)

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Jurnal > GL Delivery DLQ > JRN-2026-000077                                  │
│ STICKY HEADER                                                                            │
│  JRN-2026-000077  ·  PENEMPATAN  ·  [merah] Gagal  ·  [Domain Error]  ·  Retry: 0     │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│ LAYOUT: 2 kolom (60% kiri / 40% kanan)                                                  │
│                                                                                          │
│  KIRI — Konten                                KANAN — GlDlqActionPanel                  │
│  ──────────────────────────────────           ──────────────────────────────────         │
│                                                                                          │
│  CARD — Info Jurnal                           CARD STATUS                                │
│  ┌────────────────────────────────────┐       ┌─────────────────────────────────────┐   │
│  │ No. Jurnal    : JRN-2026-000077   │       │ [merah] Gagal — Delivery             │   │
│  │ Kode Event    : PENEMPATAN        │       │ [Domain Error]                       │   │
│  │ Tgl Posting   : 14 Jun 2026       │       │                                     │   │
│  │ Instrumen     : DEP-BCA-001  [→] │       │ Percobaan    : 0 dari 5 (maks.)     │   │
│  │ Periode       : Juni 2026         │       │ Entry DLQ    : 14 Jun 2026, 10:30   │   │
│  │ Status Internal: POSTED           │       │                                     │   │
│  │ GL Journal ID : —                 │       │ Solusi yang disarankan:             │   │
│  │ [→ Lihat jurnal detail]           │       │ Domain Error (4xx) — perbaiki       │   │
│  └────────────────────────────────────┘       │ payload di GL Host terlebih dulu:  │   │
│                                               │ · Cek Chart of Accounts GL Host    │   │
│  CARD — Error Detail                          │ · Konfirmasi kode akun berlaku     │   │
│  ┌────────────────────────────────────┐       │                                    │   │
│  │ Kode Error  : GL_HOST_REJECTED     │       │ [→ Lihat jurnal di entries]        │   │
│  │ Kategori    : [Domain Error]       │       │                                    │   │
│  │ Pesan:                             │       │ ─────────────────────────────────  │   │
│  │ "INVALID_ACCOUNT_CODE — Account   │       │                                    │   │
│  │  1110-DEP not found in GL chart"  │       │ [Replay ↺]  (AKUN-CTL + IT-ADMIN) │   │
│  │                                    │       │ [Discard ⚠] (IT-ADMIN ONLY)       │   │
│  │ Terjadi: 14 Jun 2026, 10:29:45    │       │                                    │   │
│  └────────────────────────────────────┘       └─────────────────────────────────────┘   │
│                                                                                          │
│  CARD — Payload Snapshot (sanitized)                                                     │
│  ┌────────────────────────────────────┐                                                  │
│  │ Payload yang dikirim ke GL Host:   │                                                  │
│  │ (tanpa PII — customer_name,        │                                                  │
│  │  account_no, NPWP sudah di-mask)   │                                                  │
│  │                                    │                                                  │
│  │ JSONBTreeView (collapsible)        │                                                  │
│  │ ▾ payload                          │                                                  │
│  │   idempotencyKey: "BLIPS-..."      │                                                  │
│  │   journalDate: "2026-06-14"        │                                                  │
│  │   eventCode: "PENEMPATAN"          │                                                  │
│  │   ▾ lines (2)                      │                                                  │
│  │     0: {account:"1110-DEP",...}    │                                                  │
│  │     1: {account:"1001-KAS",...}    │                                                  │
│  │   ▾ metadata                       │                                                  │
│  │     instrumenId: "<uuid>"          │                                                  │
│  │     customerName: "[REDACTED]"     │                                                  │
│  │     accountNo:   "[REDACTED]"      │                                                  │
│  └────────────────────────────────────┘                                                  │
│                                                                                          │
│  CARD — GL Host Raw Response (ROLE-IT-ADMIN only)                                       │
│  ┌────────────────────────────────────┐                                                  │
│  │ [Hanya tampil untuk ROLE-IT-ADMIN] │                                                  │
│  │                                    │                                                  │
│  │ JSONBTreeView — gl_response_payload│                                                  │
│  │ ▾ error                            │                                                  │
│  │   code: "INVALID_ACCOUNT_CODE"    │                                                  │
│  │   message: "Account 1110-DEP..."  │                                                  │
│  │   timestamp: "2026-06-14T..."     │                                                  │
│  └────────────────────────────────────┘                                                  │
│                                                                                          │
│  CARD — Riwayat Percobaan                                                                │
│  ┌────────────────────────────────────┐                                                  │
│  │ 14 Jun 2026, 10:29  — Percobaan 1  │                                                  │
│  │   GL_HOST_REJECTED: INVALID_ACCOUNT│                                                  │
│  │   (actor: SYSTEM_WORKER)           │                                                  │
│  └────────────────────────────────────┘                                                  │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

**Interaksi Replay (dari DLQ Detail):**

1. Klik "Replay ↺" — membuka `<GlDlqReplayDialog>` (bukan DLQ List modal — ini berada di halaman detail)
2. Dialog mengandung: summary jurnal + error sebelumnya + textarea reason (≥ 30 char) + warning "pastikan penyebab sudah diperbaiki"
3. Submit → POST `/jurnal/gl-delivery-dlq/{id}/replay` → 202
4. Optimistic update: status badge di halaman → "Sedang Replay" + spinner
5. DLQ entry status di right panel berubah REPLAYING, tombol Replay disabled
6. SSE/poll sys.job → saat selesai: badge berubah REPLAYED_OK (hijau) + toast "Replay JRN-2026-000088 berhasil. GL Host journal ID: GLHOST-JRN-20260615-00088."
7. Jika gagal lagi: badge kembali FAILED, attempt_count++, toast error persisten

**Interaksi Discard (dari DLQ Detail):**

1. Klik "Discard ⚠" — membuka `<GlDlqDiscardDialog>` (tidak stack modal)
2. Dialog memuat: warning irreversible + textarea reason + counter real-time
3. Tombol "Hentikan Permanen" merah, disabled sampai ≥ 30 karakter reason
4. Submit → POST `/jurnal/gl-delivery-dlq/{id}/discard` → 200
5. Optimistic update: status → ABANDONED (merah gelap)
6. Toast: "JRN-2026-000077 berhasil dihentikan. Status: DEAD_LETTER. Record tersimpan di audit trail."
7. ROLE-AKUN-CTL mencoba klik "Discard ⚠" → tombol tidak ada di DOM (hidden pada render, bukan hanya disabled)

**GlDlqReplayDialog:**

```
┌──────────────────────────────────────────────────────────────────────┐
│  Replay GL Delivery — JRN-2026-000088                              [×]│
├──────────────────────────────────────────────────────────────────────┤
│  Jurnal   : JRN-2026-000088 · AKRUAL_BUNGA · 14 Jun 2026            │
│  Error Lama: GL_HOST_UNREACHABLE — 3x infra retry                    │
│                                                                       │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │ ⚠ Pastikan GL Host sudah kembali online sebelum replay.          │  │
│  │   Sistem akan mengirim ulang payload yang sama ke GL Host.       │  │
│  │   Jika masih gagal, entry akan kembali ke status FAILED.         │  │
│  └─────────────────────────────────────────────────────────────────┘  │
│                                                                       │
│  Alasan Replay *                                                      │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │                                                                 │  │
│  └─────────────────────────────────────────────────────────────────┘  │
│  Minimal 30 karakter. Sisa: [counter]                                 │
│                                                                       │
│  Total Percobaan: 3 dari 5 (maks.)                                    │
│                                                                       │
│         [Batal]                          [Replay ↺]                  │
│                                          (disabled < 30 char)        │
└──────────────────────────────────────────────────────────────────────┘
```

**GlDlqDiscardDialog:**

```
┌──────────────────────────────────────────────────────────────────────┐
│  Hentikan Permanen — JRN-2026-000077                               [×]│
├──────────────────────────────────────────────────────────────────────┤
│  ⛔ Tindakan ini tidak bisa dibatalkan.                               │
│                                                                       │
│  Jurnal JRN-2026-000077 akan berubah status menjadi DEAD_LETTER.     │
│  Sistem tidak akan pernah mencoba mengirim jurnal ini ke GL Host     │
│  secara otomatis lagi.                                               │
│                                                                       │
│  Record akan tetap tersedia di audit trail untuk keperluan           │
│  pemeriksaan. Jika jurnal ini masih perlu dikirim ke GL Host,        │
│  buat jurnal koreksi baru via Posting Manual.                        │
│                                                                       │
│  Alasan Penghentian *  (wajib, min 30 karakter)                      │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │                                                                 │  │
│  │                                                                 │  │
│  └─────────────────────────────────────────────────────────────────┘  │
│  [sisa: 30 karakter lagi]                                             │
│                                                                       │
│  [Batal]                          [Hentikan Permanen ⛔]              │
│                                    (tombol merah — disabled          │
│                                     sampai ≥ 30 karakter)            │
└──────────────────────────────────────────────────────────────────────┘
```

---

## 4. Component Specifications

### 4.1 Komponen Baru (perlu dibuat)

#### `<GlDeliveryStatusPanel>` — `/components/blips/gl-delivery/GlDeliveryStatusPanel.tsx`

**Props:**
```tsx
interface GlDeliveryStatusPanelProps {
  jurналHeaderId: string;
  deliveryStatus: GlDeliveryStatus | null;
  canRetry: boolean;
  currentUserPermissions: string[];
  onRetryClick: () => void;
}

interface GlDeliveryStatus {
  glStatusId: string;
  glHostStatus: GlHostStatus;
  glHostJournalId: string | null;
  deliveredAt: string | null;
  retryCount: number;
  lastRetryAt: string | null;
  lastError: string | null;
  failureCategory: 'DOMAIN' | 'INFRA' | null;
  deliveryMode: 'API' | 'BATCH_FILE';
  canRetry: boolean;
  // ROLE-IT-ADMIN only (null untuk role lain):
  glResponsePayloadJsonb: Record<string, unknown> | null;
}

type GlHostStatus =
  | 'PENDING_DELIVERY'
  | 'DELIVERY_IN_FLIGHT'
  | 'RETRYING'
  | 'FAILED'
  | 'DELIVERED'
  | 'DEAD_LETTER';
```

**Perilaku:**
- Auto-refresh via SSE subscribe ke `gl-status:{headerID}` channel
- Fallback: polling GET `/api/v1/jurnal/header/{id}/gl-delivery-status` setiap 10 detik
- Berhenti poll saat status terminal (DELIVERED atau DEAD_LETTER)
- Render state yang sesuai (A–E per wireframe)
- Riwayat delivery: collapsible `<GlDeliveryHistoryTimeline>` — lazy-loaded saat di-expand
- `gl_response_payload_jsonb` hanya di-render jika bukan null (hanya ROLE-IT-ADMIN yang mendapatkan dari API)

#### `<GlStatusBadge>` — `/components/blips/gl-delivery/GlStatusBadge.tsx`

**Props:**
```tsx
interface GlStatusBadgeProps {
  status: GlHostStatus;
  size?: 'sm' | 'md';
  showIcon?: boolean;
  showLabel?: boolean;
}
```

**Perilaku:**
- Warna + ikon + teks label per tabel di §2 (tidak hanya warna)
- `DELIVERY_IN_FLIGHT` dan `RETRYING`: ikon berputar (animate-spin)
- aria-label: "Status pengiriman GL: {label}"

#### `<GlFailureCategoryBadge>` — `/components/blips/gl-delivery/GlFailureCategoryBadge.tsx`

**Props:**
```tsx
interface GlFailureCategoryBadgeProps {
  category: 'DOMAIN' | 'INFRA';
}
```

**Visual:** Secondary badge outline — merah untuk DOMAIN, amber untuk INFRA. Tooltip menjelaskan perbedaan.

#### `<GlDeliveryHistoryTimeline>` — `/components/blips/gl-delivery/GlDeliveryHistoryTimeline.tsx`

**Props:**
```tsx
interface GlDeliveryHistoryTimelineProps {
  jurналHeaderId: string;
  currentUserRole: string;
}
```

**Perilaku:**
- Query `aud.audit_log` via `GET /api/v1/jurnal/header/{id}/gl-delivery-history`
- Tampilkan timeline: timestamp + action + actor (SYSTEM_WORKER atau nama user) + detail singkat
- ROLE-IT-ADMIN: masing-masing entry bisa di-expand untuk melihat `after_jsonb` raw
- Role lain: hanya timestamp + action label + summary

#### `<RetryGlDeliveryDialog>` — `/components/blips/gl-delivery/RetryGlDeliveryDialog.tsx`

**Props:**
```tsx
interface RetryGlDeliveryDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  jurналHeaderId: string;
  jurналNumber: string;
  lastError: string | null;
  failureCategory: 'DOMAIN' | 'INFRA' | null;
  currentAttemptCount: number;
  maxAttempts: number;
  onSuccess: () => void;
}
```

**Perilaku:**
- Controlled dialog (tidak stack modal — hanya satu dialog muncul)
- Textarea reason: real-time counter, min 30 char, tombol submit disabled di bawah threshold
- Submit: disable button + spinner inline + `Idempotency-Key: uuidv4()` di header
- Success: tampilkan state sukses dalam dialog, panggil `onSuccess()` untuk trigger parent refresh
- Failure: inline error di dalam dialog, tidak tutup otomatis

#### `<GlDlqActionPanel>` — `/components/blips/gl-delivery/GlDlqActionPanel.tsx`

**Props:**
```tsx
interface GlDlqActionPanelProps {
  dlqEntry: GlDlqEntry;
  canReplay: boolean;        // jurnal.gl_delivery.replay
  canDiscard: boolean;       // jurnal.gl_delivery.discard (ROLE-IT-ADMIN only)
  onReplaySuccess: () => void;
  onDiscardSuccess: () => void;
}

interface GlDlqEntry {
  id: string;
  glStatusId: string;
  headerNumber: string;
  eventCode: string;
  status: 'FAILED' | 'REPLAYING' | 'REPLAYED_OK' | 'ABANDONED';
  failureCategory: 'DOMAIN' | 'INFRA';
  errorCode: string;
  errorMessage: string;
  attemptCount: number;
  maxAttempts: number;
  payloadSnapshotJsonb: Record<string, unknown>; // sanitized
  // ROLE-IT-ADMIN only:
  glResponsePayloadJsonb: Record<string, unknown> | null;
}
```

**Perilaku:**
- Replay button: disabled jika status != FAILED ATAU attemptCount >= maxAttempts
- Discard button: hidden jika `canDiscard = false` (tidak hanya disabled — ROLE-AKUN-CTL tidak melihat tombol ini)
- Setelah replay → status REPLAYING → tombol Replay disabled + spinner
- Setelah REPLAYED_OK: tombol keduanya disabled, badge hijau

#### `<GlDlqReplayDialog>` — `/components/blips/gl-delivery/GlDlqReplayDialog.tsx`

Mirip `<RetryGlDeliveryDialog>`, tapi untuk DLQ context. Menerima `dlqId` (bukan `headerID`). Menggunakan endpoint POST `/jurnal/gl-delivery-dlq/{id}/replay`.

#### `<GlDlqDiscardDialog>` — `/components/blips/gl-delivery/GlDlqDiscardDialog.tsx`

Dialog destructive. Tombol "Hentikan Permanen" merah. Textarea reason min 30 char, counter real-time. Endpoint POST `/jurnal/gl-delivery-dlq/{id}/discard`.

Menggunakan `<DestructiveActionDialog>` base dari BLIPS component library (sama polanya dengan P5-M2 DLQ discard).

#### `<ReconSummaryCard>` — `/components/blips/gl-delivery/ReconSummaryCard.tsx`

**Props:**
```tsx
interface ReconSummaryCardProps {
  report: GlReconciliationReport | null;
  isLoading: boolean;
  onManualRunClick?: () => void;  // undefined = sembunyikan tombol
}

interface GlReconciliationReport {
  id: string;
  tanggalRekonsiliasi: string;
  status: 'PENDING' | 'RUNNING' | 'COMPLETED' | 'COMPLETED_WITH_MISMATCH' | 'FAILED';
  totalAkunChecked: number;
  totalMismatchCount: number;
  totalMismatchAmountIdr: string;
  blipsTotalIdr: string;
  glHostTotalIdr: string;
  deltaIdr: string;
  toleranceIdr: string;
  generatedAt: string;
  jobId: string | null;
  mismatches: GlReconMismatch[];
}

interface GlReconMismatch {
  id: string;
  kodeAkun: string;
  namaAkun: string;
  blipsAmountIdr: string;
  glHostAmountIdr: string;
  deltaIdr: string;
  mismatchType: 'BLIPS_ONLY' | 'GL_ONLY' | 'AMOUNT_DIFF';
  jurnalHeaderIds: string[];
}
```

**Perilaku:**
- Render state A/B/C/D per wireframe SCREEN-P5-M3-03
- STATE B: tampilkan mismatch table inline (tidak modal, tidak tab terpisah)
- Mismatch row: klik → expand jurnal terkait (lazy-load via `jurnalHeaderIds` lookup)
- Export button: hanya di STATE B (ada mismatch)
- Auto-refresh jika status = RUNNING (poll sys.job setiap 3 detik)

#### `<ReconMismatchTypeBadge>` — `/components/blips/gl-delivery/ReconMismatchTypeBadge.tsx`

```tsx
interface ReconMismatchTypeBadgeProps {
  type: 'BLIPS_ONLY' | 'GL_ONLY' | 'AMOUNT_DIFF';
}
```

Warna + ikon + teks label per tabel §2.

### 4.2 Komponen Reuse dari P5-M2

| Komponen | Asal | Penggunaan di P5-M3 |
|---|---|---|
| `<DataTable>` | `components/blips/DataTable.tsx` | DLQ list, Rekonsiliasi history |
| `<JobProgressPanel>` | `components/blips/JobProgressPanel.tsx` | Manual rekonsiliasi run (§3 UX) |
| `<DestructiveActionDialog>` | `components/blips/DestructiveActionDialog.tsx` | Discard DLQ (base dialog) |
| `<JSONBTreeView>` | `components/blips/JSONBTreeView.tsx` | Payload snapshot + GL Host response |
| `<DLQActionPanel>` | `components/blips/jurnal/DLQActionPanel.tsx` | Basis `<GlDlqActionPanel>` |
| `<MFAStepUpModal>` | `components/blips/MFAStepUpModal.tsx` | TIDAK dipakai (DEC-027: retry tidak perlu MFA) |
| `<SodBlockBanner>` | `components/blips/SodBlockBanner.tsx` | TIDAK dipakai (GL delivery tidak ada SoD chain) |

---

## 5. Interaction Patterns

### 5.1 Auto-Delivery Status Monitoring (S2)

Status pada GlDeliveryStatusPanel perlu real-time visibility karena operator mungkin membuka halaman jurnal detail segera setelah posting P5-M2.

**SSE-first, polling fallback:**
```
1. Komponen mount → cek gl_host_status dari initial GET /jurnal/header/{id}
2. Jika status PENDING_DELIVERY | DELIVERY_IN_FLIGHT | RETRYING:
   Subscribe EventSource ke /api/v1/jobs/{jobId}/stream (jika jobId tersedia)
   atau polling GET /jurnal/header/{id}/gl-delivery-status setiap 10 detik
3. EventSource error → switch ke polling 10 detik (tidak agresif)
4. Status DELIVERED atau DEAD_LETTER → hentikan semua subscription/polling
5. Tab tidak aktif (visibilitychange = hidden) → pause polling, resume saat aktif
```

Polling 10 detik (bukan 2 detik per §3 UX minimum) karena GL delivery adalah operasi yang bisa membutuhkan 30 detik (timeout GL Host), sehingga polling agresif tidak memberikan value ekstra.

### 5.2 Retry Flow — Tampilan dari Dua Entry Points

Retry bisa dipanggil dari:
- SCREEN-P5-M3-01 (GlDeliveryStatusPanel di jurnal detail) → `<RetryGlDeliveryDialog>` (POST `/retry-gl-delivery`)
- SCREEN-P5-M3-05 (DLQ List row action) → `<GlDlqReplayDialog>` (POST `/gl-delivery-dlq/{id}/replay`)
- SCREEN-P5-M3-06 (DLQ Detail panel) → `<GlDlqReplayDialog>` (POST `/gl-delivery-dlq/{id}/replay`)

Kedua path memanggil `GLDeliveryService.Retry()` di backend (OQ-M3-5b). Di frontend, komponen dialog berbeda karena konteks berbeda, tetapi form logic identik.

Setelah submit retry:
1. Parent halaman (jurnal detail atau DLQ detail) mengoptimistic-update status ke PENDING_DELIVERY
2. Toast: "Retry delivery [no_jurnal] berhasil dijadwalkan. Pantau status di panel jurnal."
3. Asynq worker mengeksekusi delivery → hasil masuk via SSE/polling

### 5.3 Reconciliation Manual Trigger

Pattern §3 UX (long-running process):

```
1. ROLE-AKUN-CTL klik "Jalankan Rekonsiliasi Manual"
2. Confirm dialog → klik "Jalankan"
3. POST /jurnal/reconciliation/run → 202 { jobId, statusUrl }
4. <JobProgressPanel> muncul (tidak blocking halaman — bisa di-collapse ke background)
5. SSE subscribe ke /api/v1/jobs/{jobId}/stream
6. Progress: 0% → "Membaca jurnal.detail..." → 30% → "Fetching GL Host..." → 60% → "Membandingkan..." → 80% → "Menyimpan..." → 100%
7. Selesai: JobProgressPanel berubah ke "selesai", ReconSummaryCard di-refresh, toast sesuai status
```

Toast saat rekonsiliasi selesai:
- COMPLETED: "Rekonsiliasi GL 14 Jun 2026 selesai — tidak ada mismatch. 28 akun diperiksa."
- COMPLETED_WITH_MISMATCH: toast warning amber: "Rekonsiliasi GL 14 Jun 2026 selesai — 2 mismatch ditemukan. Tindak lanjut diperlukan." + link "Lihat detail"
- FAILED: toast error persisten: "Rekonsiliasi GL 14 Jun 2026 GAGAL — GL Host tidak tersedia. [Retry Manual]"

### 5.4 PII Display Rules

Sesuai security baseline dan state machine doc §8:

| Data | Tampil untuk | Tidak tampil untuk |
|---|---|---|
| `payload_snapshot_jsonb` (sanitized) | ROLE-AKUN-CTL, ROLE-IT-ADMIN, ROLE-AUDIT | ROLE-AKUN, ROLE-RISK |
| `gl_response_payload_jsonb` (raw GL Host) | **ROLE-IT-ADMIN saja** | Semua role lain |
| `customer_name`, `account_no`, `npwp` | Tidak tampil — sudah `[REDACTED]` di payload | Semua |
| `GL_HOST_API_KEY` | Tidak pernah tampil di UI | Semua |

Field `[REDACTED]` ditampilkan apa adanya di `<JSONBTreeView>` — tidak ada unmask button.

### 5.5 Toast Copy (Bahasa Indonesia, spesifik)

| Trigger | Toast | Tipe |
|---|---|---|
| Retry submit sukses | "Retry delivery JRN-2026-000077 berhasil dijadwalkan. Pantau status di panel jurnal ini." | success (4s) |
| Delivery berhasil (SSE event) | "JRN-2026-000077 berhasil dikirim ke GL Host. GL Journal ID: GLHOST-JRN-20260615-00001." | success (4s) |
| Delivery gagal lagi setelah retry (SSE event) | "Retry JRN-2026-000077 gagal: GL_HOST_REJECTED — INVALID_ACCOUNT_CODE. Periksa Chart of Accounts GL Host." | error (persisten) |
| DLQ Replay sukses | "Replay JRN-2026-000088 berhasil. GL Host Journal ID: GLHOST-JRN-20260615-00088." | success (4s) |
| DLQ Replay gagal lagi | "Replay JRN-2026-000088 gagal lagi: GL_HOST_UNREACHABLE — GL Host belum tersedia. Coba lagi nanti." | error (persisten) |
| DLQ Discard sukses | "JRN-2026-000077 berhasil dihentikan. Status: DEAD_LETTER. Record tersimpan di audit trail." | warning (8s) |
| Rekon selesai — clean | "Rekonsiliasi GL 14 Jun 2026 selesai. Tidak ada mismatch. 28 akun diperiksa." | success (4s) |
| Rekon selesai — mismatch | "Rekonsiliasi GL 14 Jun 2026: 2 mismatch ditemukan (total selisih Rp 15.000). Tindak lanjut diperlukan." | warning (8s) — link "Lihat detail mismatch" |
| Rekon gagal | "Rekonsiliasi GL 14 Jun 2026 GAGAL — GL Host tidak tersedia. Jalankan ulang setelah GL Host online." | error (persisten) |
| Permission denied discard | "Anda tidak memiliki izin untuk menghentikan entry DLQ. Hanya ROLE-IT-ADMIN yang dapat melakukan tindakan ini." | error (persisten) |
| Max attempts exceeded | "Batas maksimum percobaan delivery tercapai (5/5) untuk JRN-2026-000077. Hubungi ROLE-IT-ADMIN untuk tindak lanjut." | error (persisten) |
| Export DLQ dimulai | "Export DLQ GL Delivery dimulai. Anda akan mendapat notifikasi saat file siap diunduh." | info (4s) |
| Export DLQ selesai | "Export selesai. [Unduh file] (tersedia 24 jam)" | success (4s) |
| GL_RECONCILIATION_IN_PROGRESS | "Rekonsiliasi untuk tanggal ini sedang berjalan. Tunggu proses selesai sebelum menjalankan ulang." | warning (8s) |

### 5.6 Empty / Loading / Error States

| Layar | Empty State | Loading State | Error State |
|---|---|---|---|
| GlDeliveryStatusPanel | "Status pengiriman belum tersedia." (abu) | Skeleton 3 baris shimmer | Card merah "Gagal memuat status. [Coba lagi]" |
| Rekonsiliasi Dashboard | STATE D card (belum ada laporan) | Skeleton card + tabel | Card merah + retry button |
| Mismatch table | "(Tidak ada mismatch)" — teks kosong di dalam STATE B jika mismatches[] = 0 (edge case) | Skeleton rows 3 | Inline error |
| Riwayat Rekonsiliasi | Ilustrasi + "Belum ada riwayat rekonsiliasi." | Skeleton 5 rows | Pesan + [Coba lagi] |
| DLQ List | Ilustrasi checkmark hijau + "Tidak ada entri DLQ GL Delivery. Semua jurnal berhasil dikirim ke GL Host." | Skeleton 5 rows | Pesan + [Coba lagi] |
| DLQ Detail | — | Skeleton card (2 blok) | 404 card "Entry tidak ditemukan" + back button |

---

## 6. Accessibility

### 6.1 WCAG 2.1 AA

- Semua badge GL status menggunakan warna + ikon + teks (tidak hanya warna)
- `DEAD_LETTER` skull icon: teks fallback `aria-label="Status: Dihentikan — Dead Letter"`
- Contrast ratio ≥ 4.5:1 untuk semua kombinasi warna/latar yang digunakan
- Spinner animasi (DELIVERY_IN_FLIGHT, RETRYING): pakai `prefers-reduced-motion` — disable animasi, tampilkan ikon statis

### 6.2 Keyboard Navigation

- Dialog RetryGlDelivery/GlDlqReplay/GlDlqDiscard: focus trap aktif, Escape menutup dialog, focus kembali ke trigger button
- Textarea reason: keyboard-accessible, Tab ke tombol submit, counter terupdate saat mengetik
- DLQ DataTable: row action buttons reachable via Tab dalam satu baris, Enter/Space untuk trigger
- Collapsible riwayat delivery: keyboard expandable via Enter/Space, aria-expanded attribute
- JSONBTreeView: keyboard navigasi antar node via panah atas/bawah, Enter untuk expand/collapse

### 6.3 ARIA

- `<GlStatusBadge>`: `role="status"` jika auto-refresh, `aria-live="polite"` untuk perubahan status
- `GlDeliveryStatusPanel`: `aria-label="Status pengiriman ke GL Host"`
- Form errors (reason textarea): `aria-describedby` pointing ke error message + counter
- Dialog: `role="dialog"`, `aria-labelledby` pointing ke judul, `aria-modal="true"`
- DLQ table: `<th scope="col">`, sort buttons dengan `aria-sort`
- Disabled tombol dengan alasan: `aria-disabled="true"` + `title` atau tooltip

### 6.4 Screen Reader Copy

- GlStatusBadge DELIVERED: "Status pengiriman GL: Terkirim ke GL. GL Journal ID: GLHOST-JRN-20260615-00001."
- GlStatusBadge FAILED: "Status pengiriman GL: Gagal — Domain Error. Tombol retry tersedia."
- GlStatusBadge DEAD_LETTER: "Status pengiriman GL: Dihentikan secara permanen. Pengiriman tidak bisa dilakukan lagi."
- MismatchTypeBadge: "Tipe mismatch: Hanya di BLIPS — akun ini ada di BLIPS tapi tidak ditemukan di GL Host."

---

## 7. Bahasa Indonesia Copy Reference

| Konsep | Label ID | Label EN (export/report) |
|---|---|---|
| GL Host | GL Host | GL Host |
| Status Pengiriman | Status Pengiriman GL | GL Delivery Status |
| PENDING_DELIVERY | Menunggu Pengiriman | Pending Delivery |
| DELIVERY_IN_FLIGHT | Sedang Dikirim | Delivering |
| RETRYING | Sedang Retry | Retrying |
| FAILED | Gagal — Delivery | Failed |
| DELIVERED | Terkirim ke GL | Delivered |
| DEAD_LETTER | Dihentikan — DLQ | Dead Letter |
| failure_category DOMAIN | Domain Error | Domain Error |
| failure_category INFRA | Infra Error | Infrastructure Error |
| DLQ status FAILED | Gagal | Failed |
| DLQ status REPLAYING | Sedang Replay | Replaying |
| DLQ status REPLAYED_OK | Replay Berhasil | Replayed Successfully |
| DLQ status ABANDONED | Dihentikan | Abandoned |
| Rekonsiliasi harian | Rekonsiliasi GL Host | GL Host Reconciliation |
| COMPLETED | Sesuai | Reconciled |
| COMPLETED_WITH_MISMATCH | Ada Mismatch | Mismatch Found |
| FAILED (rekon) | Gagal | Failed |
| mismatch_type BLIPS_ONLY | Hanya di BLIPS | BLIPS Only |
| mismatch_type GL_ONLY | Hanya di GL Host | GL Host Only |
| mismatch_type AMOUNT_DIFF | Selisih Jumlah | Amount Difference |
| gl_host_journal_id | GL Journal ID | GL Journal ID |
| retry_count | Jumlah Retry | Retry Count |
| failure_category | Kategori Error | Error Category |
| error_code | Kode Error | Error Code |
| discard_reason | Alasan Penghentian | Discard Reason |
| manual_retry_reason | Alasan Retry | Retry Reason |
| tanggal_rekonsiliasi | Tgl Rekonsiliasi | Reconciliation Date |
| total_mismatch_amount_idr | Total Selisih (IDR) | Total Mismatch (IDR) |
| tolerance_idr | Toleransi (IDR) | Tolerance (IDR) |
| delta_idr | Selisih (IDR) | Delta (IDR) |
| Replay (DLQ) | Replay | Replay |
| Discard (DLQ) | Hentikan Permanen | Discard Permanently |
| dapat di-retry | Bisa Diulang | Retryable |
| tidak bisa di-retry | Tidak Bisa Diulang | Non-retryable |
| Manual trigger rekon | Jalankan Rekonsiliasi Manual | Run Manual Reconciliation |
| kode_akun | Kode Akun | Account Code |
| nama_akun | Nama Akun | Account Name |

---

## 8. Hand-off untuk Frontend Engineer Next.js

### 8.1 File Structure

```
frontend/src/app/jrnl/
├── journal-entries/
│   └── [id]/
│       └── page.tsx              — P5-M2 detail, tambahkan <GlDeliveryStatusPanel>
│                                    setelah <JurnalLinesTable>, sebelum tabs
├── gl-delivery-dlq/
│   ├── page.tsx                  — SCREEN-P5-M3-05 DLQ list
│   └── [id]/
│       └── page.tsx              — SCREEN-P5-M3-06 DLQ detail
└── rekonsiliasi/
    ├── page.tsx                  — SCREEN-P5-M3-03 Rekonsiliasi Dashboard
    └── riwayat/
        └── page.tsx              — SCREEN-P5-M3-04 Riwayat Rekonsiliasi

frontend/src/components/blips/gl-delivery/
├── GlDeliveryStatusPanel.tsx
├── GlStatusBadge.tsx
├── GlFailureCategoryBadge.tsx
├── GlDeliveryHistoryTimeline.tsx
├── RetryGlDeliveryDialog.tsx
├── GlDlqActionPanel.tsx
├── GlDlqReplayDialog.tsx
├── GlDlqDiscardDialog.tsx
├── ReconSummaryCard.tsx
├── ReconMismatchTypeBadge.tsx
└── ReconMismatchTable.tsx        — sub-komponen dari ReconSummaryCard

frontend/src/lib/
├── gl-delivery.api.ts            — API client (TanStack Query hooks)
├── gl-delivery.store.ts          — Zustand store
└── gl-delivery.schema.ts         — Zod schemas
```

### 8.2 shadcn/ui Components yang Digunakan

| shadcn component | Digunakan untuk |
|---|---|
| `Card` | GlDeliveryStatusPanel states, ReconSummaryCard, DLQ error detail |
| `Dialog` | RetryGlDeliveryDialog, GlDlqReplayDialog, GlDlqDiscardDialog, ReconRunConfirmDialog |
| `Badge` | GlStatusBadge, GlFailureCategoryBadge, ReconMismatchTypeBadge, DLQ status |
| `Collapsible` | GlDeliveryHistoryTimeline, jurnal terkait drill-down di mismatch |
| `ScrollArea` | JSONBTreeView dalam payload preview |
| `Skeleton` | Loading states (panel, cards, tables) |
| `Textarea` | Reason input (retry, discard, replay) |
| `Alert` | Diff DLQ info banner, warning irreversible action |
| `Separator` | Section dividers |
| `DatePicker` | Tanggal rekonsiliasi selector (disable hari libur) |
| `Tabs` | Tidak dipakai di P5-M3 (status selalu visible, tidak di tab) |
| `Progress` | JobProgressPanel internal (dari §3 UX pattern) |
| `Popover` | Tooltip penjelasan badge, disabled button tooltips |

### 8.3 Zod Schemas (`gl-delivery.schema.ts`)

```ts
// Semua nominal uang: string (bukan number) — sesuai DEC-016 presisi

const GlRetryReasonSchema = z.object({
  reason: z
    .string()
    .min(30, "Alasan retry wajib minimal 30 karakter")
    .max(1000, "Alasan terlalu panjang (maks 1000 karakter)")
    .refine(
      (val) => !/(api[_-]?key|secret|password|token)/i.test(val),
      "Alasan tidak boleh mengandung data sensitif (credential/secret)"
    ),
});

const GlDlqReplaySchema = GlRetryReasonSchema; // Alias — schema identik

const GlDlqDiscardSchema = z.object({
  reason: z
    .string()
    .min(30, "Alasan penghentian wajib minimal 30 karakter")
    .max(1000),
});

const ReconRunSchema = z.object({
  date: z
    .string()
    .regex(/^\d{4}-\d{2}-\d{2}$/, "Format tanggal: YYYY-MM-DD")
    .refine(
      (val) => new Date(val) <= new Date(),
      "Tanggal tidak boleh di masa depan"
    ),
});

// Response shapes (untuk TanStack Query typing)
const GlDeliveryStatusSchema = z.object({
  glStatusId: z.string().uuid(),
  glHostStatus: z.enum([
    'PENDING_DELIVERY', 'DELIVERY_IN_FLIGHT', 'RETRYING',
    'FAILED', 'DELIVERED', 'DEAD_LETTER'
  ]),
  glHostJournalId: z.string().nullable(),
  deliveredAt: z.string().datetime().nullable(),
  retryCount: z.number().int().min(0),
  lastRetryAt: z.string().datetime().nullable(),
  lastError: z.string().nullable(),
  failureCategory: z.enum(['DOMAIN', 'INFRA']).nullable(),
  deliveryMode: z.enum(['API', 'BATCH_FILE']),
  canRetry: z.boolean(),
  glResponsePayloadJsonb: z.record(z.unknown()).nullable(), // null for non-IT-ADMIN
});

const GlDlqEntrySchema = z.object({
  id: z.string().uuid(),
  glStatusId: z.string().uuid(),
  noJurnal: z.string(),
  tanggalPosting: z.string(),
  eventCode: z.string(),
  failureCategory: z.enum(['DOMAIN', 'INFRA']),
  errorCode: z.string(),
  errorMessage: z.string(),
  attemptCount: z.number().int(),
  status: z.enum(['FAILED', 'REPLAYING', 'REPLAYED_OK', 'ABANDONED']),
  payloadSnapshotJsonb: z.record(z.unknown()),
  glResponsePayloadJsonb: z.record(z.unknown()).nullable(),
  createdAt: z.string().datetime(),
  lastRetryAt: z.string().datetime().nullable(),
});

const GlReconMismatchSchema = z.object({
  id: z.string().uuid(),
  kodeAkun: z.string(),
  namaAkun: z.string(),
  blipsAmountIdr: z.string(),
  glHostAmountIdr: z.string(),
  deltaIdr: z.string(),
  mismatchType: z.enum(['BLIPS_ONLY', 'GL_ONLY', 'AMOUNT_DIFF']),
  jurnalHeaderIds: z.array(z.string().uuid()),
});

const GlReconciliationReportSchema = z.object({
  id: z.string().uuid(),
  tanggalRekonsiliasi: z.string(),
  status: z.enum(['PENDING', 'RUNNING', 'COMPLETED', 'COMPLETED_WITH_MISMATCH', 'FAILED']),
  totalAkunChecked: z.number().int(),
  totalMismatchCount: z.number().int(),
  totalMismatchAmountIdr: z.string(),
  blipsTotalIdr: z.string(),
  glHostTotalIdr: z.string(),
  deltaIdr: z.string(),
  toleranceIdr: z.string(),
  generatedAt: z.string().datetime().nullable(),
  jobId: z.string().nullable(),
  mismatches: z.array(GlReconMismatchSchema),
});
```

### 8.4 API Client Hooks (`gl-delivery.api.ts`)

Key TanStack Query hooks yang diperlukan:

```ts
// GL Delivery Status (S2)
useGlDeliveryStatus(headerID: string)
  // GET /api/v1/jurnal/header/{id}/gl-delivery-status
  // staleTime: 0 (always fresh — auto-refresh handled by component)

useGlDeliveryHistory(headerID: string)
  // GET /api/v1/jurnal/header/{id}/gl-delivery-history (audit log query)
  // lazy — hanya load saat collapsible di-expand

// Manual Retry (S3)
useRetryGlDelivery()
  // POST /api/v1/jurnal/header/{id}/retry-gl-delivery
  // mutation — wajib Idempotency-Key: uuidv4() per submit

// Reconciliation (S4)
useReconReport(date: string)
  // GET /api/v1/jurnal/reconciliation/daily?date=YYYY-MM-DD
  // enabled: date != null

useReconHistory(params: ReconHistoryParams)
  // GET /api/v1/jurnal/reconciliation/history (cursor paginated, sort, filter)

useRunManualRecon()
  // POST /api/v1/jurnal/reconciliation/run
  // mutation → 202 { jobId, statusUrl }

// GL Delivery DLQ (S5)
useGlDlqList(params: GlDlqListParams)
  // GET /api/v1/jurnal/gl-delivery-dlq (cursor paginated, sort, filter)

useGlDlqDetail(id: string)
  // GET /api/v1/jurnal/gl-delivery-dlq/{id}

useGlDlqReplay()
  // POST /api/v1/jurnal/gl-delivery-dlq/{id}/replay
  // mutation → 202 { jobId }

useGlDlqDiscard()
  // POST /api/v1/jurnal/gl-delivery-dlq/{id}/discard
  // mutation → 200 { newStatus: 'DEAD_LETTER' }

// Job tracking (dari §3 UX pattern — gunakan existing useJobStatus)
useJobStatus(jobId: string | null)
  // GET /api/v1/jobs/{jobId}
  // enabled: jobId != null
```

**TanStack Query key factory:**

```ts
export const glDeliveryKeys = {
  all: ['gl-delivery'] as const,
  status: (headerID: string) => [...glDeliveryKeys.all, 'status', headerID] as const,
  history: (headerID: string) => [...glDeliveryKeys.all, 'history', headerID] as const,
  dlqList: (params: GlDlqListParams) => [...glDeliveryKeys.all, 'dlq', params] as const,
  dlqDetail: (id: string) => [...glDeliveryKeys.all, 'dlq', id] as const,
  recon: (date: string) => [...glDeliveryKeys.all, 'recon', date] as const,
  reconHistory: (params: ReconHistoryParams) => [...glDeliveryKeys.all, 'recon-history', params] as const,
};
```

### 8.5 Zustand Store (`gl-delivery.store.ts`)

```ts
interface GlDeliveryStore {
  // DLQ badge count (global top nav badge)
  glDlqFailedCount: number;
  setGlDlqFailedCount: (count: number) => void;

  // Active job untuk rekonsiliasi progress (null saat idle)
  activeReconJobId: string | null;
  setActiveReconJobId: (jobId: string | null) => void;

  // Filter state DLQ List (URL-synced via nuqs — store hanya untuk non-URL state)
  // Filter di URL search params (nuqs pattern) bukan Zustand
}
```

### 8.6 Route Map

| Route | Component | Guard (permission) |
|---|---|---|
| `/jrnl/journal-entries/[id]` | P5-M2 detail + `<GlDeliveryStatusPanel>` embedded | `jurnal.read` |
| `/jrnl/gl-delivery-dlq` | DLQ List page | `jurnal.gl_delivery.read` (AKUN-CTL + IT-ADMIN + AUDIT) |
| `/jrnl/gl-delivery-dlq/[id]` | DLQ Detail page | `jurnal.gl_delivery.read` |
| `/jrnl/rekonsiliasi` | Rekonsiliasi Dashboard | `jurnal.reconciliation.read` (AKUN-CTL + CFO + AUDIT) |
| `/jrnl/rekonsiliasi/riwayat` | Riwayat Rekonsiliasi | `jurnal.reconciliation.read` |

Jika permission tidak terpenuhi → redirect ke `/unauthorized` (401/403 guard di Next.js middleware).

### 8.7 Validation Rules (Checklist untuk Engineer)

Frontend validasi (Zod + React Hook Form):
- [ ] `reason` (retry, replay, discard): min 30 karakter, counter visible real-time
- [ ] `reason` discard: tombol "Hentikan Permanen" disabled sampai ≥ 30 karakter (bukan hanya Zod validation on submit)
- [ ] `date` (rekon): hanya hari kerja yang bisa dipilih (disable hari libur di DatePicker via `sys.calendar_holiday` fetch)
- [ ] `date` (rekon): tidak boleh future date
- [ ] Retry button hidden jika `can_retry = false` (tidak hanya disabled — backend juga enforce)
- [ ] Discard button hidden jika `!canDiscard` — tidak di-render di DOM (bukan hanya disabled atau opacity:0)
- [ ] Replay button disabled jika `status != FAILED` ATAU `attemptCount >= maxAttempts`
- [ ] `Idempotency-Key`: uuidv4() baru di-generate per submit attempt (bukan cached dari sebelumnya)

Server-side validasi tetap menjadi sole guarantee — client validation untuk UX saja.

### 8.8 Permission Checks (Client-side)

| Element | Permission | Jika tidak ada |
|---|---|---|
| Tombol "Retry Pengiriman" di StatusPanel | `jurnal.gl_delivery.retry` | Hidden |
| Tombol "Replay" di DLQ list/detail | `jurnal.gl_delivery.replay` | Hidden |
| Tombol "Discard ⚠" di DLQ list/detail | `jurnal.gl_delivery.discard` | **Tidak di-render** |
| Tombol "Jalankan Rekonsiliasi Manual" | `jurnal.reconciliation.run` | Hidden |
| DLQ page access | `jurnal.gl_delivery.read` | 403 redirect |
| Rekonsiliasi page access | `jurnal.reconciliation.read` | 403 redirect |
| GL Host raw response card | user.roles includes 'ROLE-IT-ADMIN' | Section tidak di-render |
| Export DLQ | `jurnal.gl_delivery.read` | Disabled + tooltip |
| Export Rekonsiliasi | `jurnal.reconciliation.read` | Disabled + tooltip |

---

## 9. Anti-pattern Notes

Anti-pattern yang dihindari di design ini:

- **Modals stacking**: Retry dialog dari DLQ Detail tidak membuka modal tambahan. Dialog konfirmasi adalah single-level. DLQ Detail adalah halaman penuh (bukan modal).
- **Auto-saving**: Tidak ada auto-save di form retry/replay/discard — operator harus eksplisit submit.
- **Hiding workflow state**: Status GL delivery selalu visible di sticky header jurnal detail, tidak disembunyikan di tab.
- **Toast only confirmation**: Discard (DEAD_LETTER) memiliki `<GlDlqDiscardDialog>` dengan explicit confirm sebelum action, bukan hanya toast setelah.
- **Color sole signal**: Semua badge menggunakan warna + ikon + teks. Termasuk DEAD_LETTER (skull icon), RETRYING (refresh icon berputar).
- **Polling terlalu agresif**: Polling 10 detik (bukan 2 detik) untuk delivery status — karena GL Host timeout bisa sampai 30 detik.
- **Tab menyembunyikan workflow**: Rekonsiliasi dashboard tidak menggunakan tab untuk menyembunyikan mismatch detail — mismatch ditampilkan inline di bawah summary card.
- **Discard tombol disabled vs hidden**: Tombol "Discard ⚠" untuk ROLE-AKUN-CTL tidak di-render sama sekali di DOM — bukan disabled (prevent potential DOM inspection bypass).

---

_Dokumen ini siap dihandoff ke `frontend-engineer-nextjs`. Backend contracts ada di `api/openapi/app-d-gl-delivery.yaml` dan `docs/state-machines/p5-m3-gl-delivery.md`. Migration schema di `db/migrations/000037_*`. Konfirmasi DEC-031 (GL Host vendor) masih PENDING — adapter interface sudah di-design untuk swap-safe._
