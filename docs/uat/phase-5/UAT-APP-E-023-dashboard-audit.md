# UAT-APP-E-023 — Auditor Dashboard /dashboard/audit

**UAT ID**: UAT-APP-E-023
**Modul**: APP-E — Reporting & Dashboard
**Story Set**: P5-M15 / Story P5-M15-05 (partial — Auditor Dashboard portion)
**AC yang dicakup**: M15-05-AC1, M15-05-AC4 (Auditor Dashboard)
**Tanggal UAT**: _(diisi saat pelaksanaan)_
**Penyusun**: qa-engineer
**Gate**: security-engineer BLOCKING — audit data tidak bocor ke non-AUDIT role; read-only enforcement (tidak ada mutasi dari ROLE-AUDIT); absent-from-DOM

---

## Pre-Kondisi

1. Environment UAT berjalan (`docker compose -f deploy/docker-compose.uat.yml up -d`)
2. M14 deployed — `GET /api/v1/reports/rpt-25`, `rpt-26` return 200 untuk ROLE-AUDIT
3. M13 deployed — `sys.job` ter-populate; hash-chain verify job pernah jalan
4. Data seed:
   - `aud.audit_log`: 85.000 entries total; 3 entries `action='SOD_VIOLATION'` bulan ini
   - `sys.job`: record type=`HASH_CHAIN_VERIFY`, status=`completed`, result `{status: "VERIFIED", rowsChecked: 85000, mismatchCount: 0}`
   - Daily volume: ± 2.500-3.000 events per hari selama 30 hari terakhir
5. User test:

| User ID | Role | `dashboard.audit.read` | `report.*.read` | MFA |
|---|---|---|---|---|
| USR-AUDIT-001 | ROLE-AUDIT | Ya | Ya | Tidak |
| USR-AKUN-001 | ROLE-AKUN | TIDAK | TIDAK | Tidak |
| USR-RISK-001 | ROLE-RISK | TIDAK | TIDAK | Tidak |

---

## Data Test Numerik

- Total audit events 30 hari: 85.000
- SoD violations bulan ini: 3 (actor: USR-MAKER-001 tanggal 20 Jun; USR-APPR-TR-02 tanggal 18 Jun; USR-MAKER-003 tanggal 15 Jun)
- Hash-chain last verify: 25 Jun 2026 07:00-07:08 — VERIFIED (85.000 rows, 0 mismatch)
- Mismatch scenario: `mismatchCount=1` saat test negative case

---

## Skenario UAT

### TC-001 — M15-05-AC1: W-AU-01 Audit Log Volume 30 hari AreaChart

**Actor**: USR-AUDIT-001 (ROLE-AUDIT)

**Langkah**:
1. Login sebagai USR-AUDIT-001
2. Navigasi ke `/dashboard/audit`
3. Perhatikan widget W-AU-01 Volume Audit Log 30 Hari (AreaChart)
4. Hover pada data point salah satu hari

**Hasil yang Diharapkan**:
- AreaChart: X=tanggal (DD/MM, 30 hari); Y=event count per hari
- Area fill dengan opacity 0.2
- KPI di atas chart: "Total 30 hari: 85.000 events"
- Tooltip per data point: "Tanggal {date}: {count} events"
- Data dari `GET /api/v1/reports/rpt-25?filter[event_time]=between:{today-30d},{today}&sort=event_time:asc&limit=200`

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-002 — M15-05-AC1: W-AU-02 Hash-Chain Status — VERIFIED (green)

**Actor**: USR-AUDIT-001

**Langkah**:
1. Navigasi ke `/dashboard/audit`
2. Perhatikan KPI card W-AU-02 Hash-Chain Verification Status
3. Klik link "Lihat detail →"

**Hasil yang Diharapkan**:
- Badge hijau `<CheckCircle>`: "Hash-chain VERIFIED — last run: 25 Jun 2026 07:08:00"
- Data dari `GET /api/v1/jobs?type=HASH_CHAIN_VERIFY&sort=created_at:desc&limit=1`
- Link "Lihat detail →" → `/jobs/{jobId}` (halaman detail job hash-chain)

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-003 — M15-05-AC1: W-AU-02 Hash-Chain Status — MISMATCH (red alert)

**Actor**: USR-AUDIT-001
**Pre-kondisi**: Manipulasi seed — update `sys.job` hash-chain job terakhir: result `{status: "MISMATCH", mismatchCount: 1, firstMismatchEventId: "evt-999"}`

**Langkah**:
1. Ubah seed — job hash-chain terakhir = MISMATCH
2. Navigasi ke `/dashboard/audit`
3. Perhatikan W-AU-02

**Hasil yang Diharapkan**:
- Badge merah `<AlertTriangle>`: "PERINGATAN: Hash-chain MISMATCH terdeteksi!"
- Detail: "1 mismatch ditemukan" + link ke event: "evt-999"
- Link "Lihat detail →" ke `/jobs/{jobId}`
- Status card menggunakan warna merah (danger)

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-004 — M15-05-AC1: W-AU-03 SoD Violation Alerts — 3 pelanggaran

**Actor**: USR-AUDIT-001

**Langkah**:
1. Navigasi ke `/dashboard/audit`
2. Perhatikan widget W-AU-03 Peringatan Pelanggaran SoD (DataTable)

**Hasil yang Diharapkan**:
- Badge merah di header widget: "3 pelanggaran SoD bulan ini"
- DataTable dengan kolom: event_time, actor_user_id, entity_type, entity_id, detail (dari after_jsonb)
- Baris pertama: "20 Jun 14:30:00 | USR-MAKER-001 | PENEMPATAN | pls-001 | Attempted self-review"
- Data dari `GET /api/v1/reports/rpt-25?filter[action]=SOD_VIOLATION&sort=event_time:desc&limit=20`

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-005 — M15-05-AC1: W-AU-03 SoD Violation Alerts — empty state (tidak ada pelanggaran)

**Actor**: USR-AUDIT-001
**Pre-kondisi**: Tidak ada record `action='SOD_VIOLATION'` dalam 30 hari

**Langkah**:
1. Ubah seed — hapus sementara SOD_VIOLATION events dari audit log
2. Navigasi ke `/dashboard/audit`
3. Perhatikan W-AU-03

**Hasil yang Diharapkan**:
- Empty state: badge hijau ✓ "Tidak ada pelanggaran SoD yang terdeteksi dalam 30 hari terakhir."
- Tidak ada DataTable rows
- Badge merah TIDAK muncul

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-006 — M15-05-AC4: Role gate — ROLE-AKUN tidak bisa akses /dashboard/audit

**Actor**: USR-AKUN-001 (ROLE-AKUN — tanpa `dashboard.audit.read`)

**Langkah**:
1. Login sebagai USR-AKUN-001
2. Ketik langsung: `[URL_UAT]/dashboard/audit`
3. Perhatikan hasilnya
4. Buka Network tab — cek request ke rpt-25

**Hasil yang Diharapkan**:
- Redirect ke `/dashboard/akuntansi` (role default ROLE-AKUN)
- Widget W-AU-01..W-AU-04 TIDAK ADA di DOM (inspect element: tidak ada `data-widget-id="W-AU-*"`)
- Audit log data tidak bocor ke non-AUDIT role — Network tab: tidak ada request ke `rpt-25` dari sesi ini

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-007 — M15-05-AC4: ROLE-AUDIT — tidak ada tombol mutasi di DOM

**Actor**: USR-AUDIT-001

**Langkah**:
1. Login sebagai USR-AUDIT-001
2. Navigasi ke `/dashboard/audit`
3. Inspect DOM — cari button create, submit, approve, reject
4. Coba mengakses endpoint mutasi langsung (mis. POST /api/v1/reports/...)

**Hasil yang Diharapkan**:
- Tidak ada `<button>` dengan teks: "Buat", "Submit", "Approve", "Setujui", "Tolak", "Hapus"
- ROLE-AUDIT hanya bisa read — halaman menampilkan label "Read-Only" di header
- POST/PATCH/DELETE request ke apapun dari ROLE-AUDIT → 403 FORBIDDEN

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-008 — M15-05-AC4: Aksesibilitas DataTable W-AU-03

**Actor**: USR-AUDIT-001
**Tools**: DevTools Accessibility Inspector; keyboard

**Langkah**:
1. Navigasi ke `/dashboard/audit`
2. Inspeksi widget W-AU-03 via Accessibility Inspector
3. Tab navigate ke dalam DataTable

**Hasil yang Diharapkan**:
- Widget container: `aria-label="[Nama Widget] — BLIPS Auditor Dashboard"`
- DataTable: `aria-label="Riwayat Pelanggaran SoD"` atau serupa
- Column headers: `scope="col"`
- Tombol Refresh: `aria-label="Perbarui semua data Auditor Dashboard"` (atau serupa)
- Link "Lihat detail →" hash-chain: `aria-label="Lihat detail verifikasi hash-chain {jobId}"`
- Keyboard Tab: fokus ke filter/controls → rows → link aksi → pagination

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-009 — M15-05-AC1: W-AU-04 Top Action Types chart

**Actor**: USR-AUDIT-001

**Langkah**:
1. Navigasi ke `/dashboard/audit`
2. Perhatikan widget W-AU-04 Top Action Types (BarChart horizontal)

**Hasil yang Diharapkan**:
- BarChart horizontal: top 10 action type by count descending
- Kolom: action label (Bahasa Indonesia), count
- Contoh: "INSTRUMEN.CREATE: 1.234", "JURNAL.POST: 987", dll.
- Link "Lihat RPT-25 →" → laporan audit log M14

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

## Audit Checks

- Dashboard view read-only: tidak ada `aud.audit_log` row per view (DEC-018)
- `/dashboard/audit` view → Loki log (monitoring level — bukan `aud.audit_log`)
- Export dari widget W-AU-01: `aud.audit_log` `action='EXPORT.GENERATED'`

---

## Rollback / Cleanup

- Kembalikan seed hash-chain job ke VERIFIED setelah TC-003
- Kembalikan SOD_VIOLATION events setelah TC-005

---

## Ringkasan TC

| TC | AC | Actor | Status |
|---|---|---|---|
| TC-001 | M15-05-AC1 | USR-AUDIT-001 | ☐ Pass ☐ Fail |
| TC-002 | M15-05-AC1 | USR-AUDIT-001 | ☐ Pass ☐ Fail |
| TC-003 | M15-05-AC1 | USR-AUDIT-001 | ☐ Pass ☐ Fail |
| TC-004 | M15-05-AC1 | USR-AUDIT-001 | ☐ Pass ☐ Fail |
| TC-005 | M15-05-AC1 | USR-AUDIT-001 | ☐ Pass ☐ Fail |
| TC-006 | M15-05-AC4 | USR-AKUN-001 | ☐ Pass ☐ Fail |
| TC-007 | M15-05-AC4 | USR-AUDIT-001 | ☐ Pass ☐ Fail |
| TC-008 | M15-05-AC4 | USR-AUDIT-001 | ☐ Pass ☐ Fail |
| TC-009 | M15-05-AC1 | USR-AUDIT-001 | ☐ Pass ☐ Fail |

**Total: 9 TC covering AC M15-05-AC1 dan M15-05-AC4 (Auditor Dashboard)**

---

## Sign-Off

| Peran | Nama | Tanggal | Tanda Tangan |
|---|---|---|---|
| Tester (QA) | | | |
| Reviewer (Security — BLOCKING) | | | |
| Approver (IT-Admin/PM) | | | |
