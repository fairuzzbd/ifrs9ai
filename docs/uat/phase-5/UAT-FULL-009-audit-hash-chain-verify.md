# UAT-FULL-009 — Audit Hash Chain Verification Cross-Modul

**Modul**: Cross-Modul (aud schema)
**Story**: Verifikasi hash chain audit log tidak putus setelah serangkaian mutasi di semua schema
**Tanggal dokumen**: 2026-06-25
**Dibuat oleh**: qa-engineer
**Status**: DRAFT

---

## 1. Metadata

| Field | Nilai |
|---|---|
| ID UAT | UAT-FULL-009 |
| Referensi test | `TestP5M18_Audit_Hash_Chain_Verify_Across_Modul` |
| Referensi FSD | FSD-APP-D §5 (Audit Trail) |
| DEC terkait | DEC-018 (audit append-only, hash-chain SHA-256, retention 10+10 tahun) |
| Formula hash | `current_hash = sha256(previous_hash \|\| canonical_json(row))` |

---

## 2. Persona yang Terlibat

| Persona | Role | Aksi |
|---|---|---|
| Internal Auditor | ROLE-AUDIT | Menjalankan hash chain verifier, membaca audit log |
| ROLE-IT-ADMIN | ROLE-IT-ADMIN | Menjalankan `cmd/audit-verify` CLI tool |
| Compliance Officer | — | Sign-off hasil verifikasi |

---

## 3. Pre-kondisi

| # | Kondisi |
|---|---|
| P1 | Terdapat audit log yang aktif untuk periode 30 hari terakhir |
| P2 | Tool `cmd/audit-verify` sudah ter-deploy dan bisa diakses |
| P3 | Tidak ada tampering (modify, delete) pada tabel `aud.audit_log` sejak last verify |
| P4 | `GET /api/v1/audit/verify` endpoint tersedia untuk ROLE-AUDIT |

---

## 4. Data Test

Audit rows dari lifecycle yang sudah terjadi (UAT-FULL-001 s/d UAT-FULL-008) akan digunakan.
Minimal: 50 audit rows dari berbagai entity type dan schema.

---

## 5. Langkah-Langkah

### Fase 1: Audit Log Browsing (Auditor)

**Step 1.1 — Buka audit log browser**

1. Login sebagai `usr-audit-01` (ROLE-AUDIT, read-only).
2. Buka `/audit/log?from=2026-06-01&to=2026-06-30`.

Hasil yang diharapkan:
- [ ] List audit rows dengan kolom: event_time, action, entity_type, entity_id, actor, current_hash.
- [ ] Semua kolom `current_hash` terisi (non-empty).
- [ ] `before_jsonb` dan `after_jsonb` tidak di-truncate (ROLE-AUDIT boleh lihat).
- [ ] Cursor pagination berfungsi.
- [ ] Export CSV tersedia.

**Step 1.2 — Filter per entity type**

1. Filter: `entity_type = PENEMPATAN`.

Hasil yang diharapkan:
- [ ] Hanya rows dengan `entity_type = PENEMPATAN` tampil.
- [ ] Multi-sort berfungsi (sort by event_time DESC + action ASC).

### Fase 2: Hash Chain Verification via API

**Step 2.1 — Jalankan verifikasi via API (Auditor)**

1. `GET /api/v1/audit/verify?from=2026-06-01&to=2026-06-30`.

Hasil yang diharapkan:
- [ ] Response: `{ "valid": true, "rows_verified": N, "broken_at": null }`.
- [ ] `rows_verified ≥ 50` (dari lifecycle sebelumnya).
- [ ] Waktu respons ≤ 5 detik untuk 30-hari window (SLA roadmap).

**Step 2.2 — Simulasikan tampering (negatif test, IT Admin)**

1. Login sebagai DBA (hanya untuk UAT environment).
2. UPDATE satu baris di `aud.audit_log` — ubah `after_jsonb`.
3. Kembali jalankan verifikasi.

Hasil yang diharapkan:
- [ ] Response: `{ "valid": false, "broken_at": { "event_id": "...", "row_index": N } }`.
- [ ] Sistem terdeteksi tampering pada row yang di-modifikasi.
- [ ] Alert otomatis dikirim ke ROLE-IT-ADMIN (via email/notif).

**Step 2.3 — Restore tampering**

1. Rollback UPDATE yang dilakukan di step 2.2.
2. Jalankan verifikasi lagi.

Hasil yang diharapkan:
- [ ] Response: `{ "valid": true }`.

### Fase 3: Hash Chain Verification via CLI (IT Admin)

**Step 3.1 — Jalankan `cmd/audit-verify`**

1. `ssh` ke server.
2. Jalankan: `./cmd/audit-verify --range "2026-06-01:2026-06-30" --output json`.

Hasil yang diharapkan:
- [ ] Output: `{ "valid": true, "rows": N, "duration_sec": ≤5 }`.
- [ ] Exit code 0.
- [ ] Output ke stdout bisa di-pipe ke file untuk lampiran audit.

**Step 3.2 — Verifikasi immutability (hard delete attempt)**

1. Coba `DELETE FROM aud.audit_log WHERE event_id = '...'` dari psql.

Hasil yang diharapkan:
- [ ] Error: `ERROR: hard delete is forbidden on aud.audit_log — use soft-delete`.
- [ ] DB trigger menolak hard delete (DEC-018).

### Fase 4: Cross-Schema Audit Coverage

**Step 4.1 — Verifikasi coverage schema**

1. `GET /api/v1/audit/coverage?from=2026-06-01&to=2026-06-30`.

Hasil yang diharapkan:
- [ ] Semua 9 schema namespace terwakili: mst, trx, ecl, sppi, doc, jrnl, aud (self), sec, sys.
- [ ] Entity types yang diharapkan hadir: PENEMPATAN, MTM, ECL_RUN, JURNAL, PERIODE, EIR_SCHEDULE, RENEWAL, PENJUALAN, POCI_BASELINE.
- [ ] Total rows per schema > 0 untuk semua schema yang punya mutasi di bulan ini.

---

## 6. Contoh Verifikasi Manual Hash

Untuk verifikasi manual satu baris:

```bash
# Ambil baris N-1 (previous_hash) dan baris N (current row)
echo -n "{previous_hash_hex}||{action}||{entity_id}||{after_jsonb}" | sha256sum
# Harus sama dengan current_hash di baris N
```

---

## 7. Audit Checks

| Aksi | Verifikasi |
|---|---|
| Hash chain verify OK | `{ "valid": true }` |
| Tampering terdeteksi | `{ "valid": false, "broken_at": {...} }` |
| Hard delete forbidden | DB trigger error |

---

## 8. Rollback / Cleanup

Tidak ada rollback diperlukan. Audit log bersifat append-only.
Untuk UAT environment: pastikan tampering dari Step 2.2 sudah di-restore sebelum UAT selesai.

---

## 9. Sign-Off

| Peran | Nama | Tanggal | Hasil | Tanda tangan |
|---|---|---|---|---|
| QA Engineer | | | PASS / FAIL | |
| Internal Auditor | | | PASS / FAIL | |
| ROLE-IT-ADMIN | | | KONFIRMASI CLI | |
| Compliance Officer | | | APPROVED / REJECT | |
| CFO | | | VERIFIED | |
