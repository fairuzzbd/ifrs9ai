# UAT-FULL-010 — SoD Enforcement + Idempotency Cross-Modul

**Modul**: Cross-Modul (Security + API)
**Story**: Verifikasi SoD tidak bisa di-bypass via API langsung, dan idempotency replay berfungsi di semua endpoint mutasi
**Tanggal dokumen**: 2026-06-25
**Dibuat oleh**: qa-engineer
**Status**: DRAFT

---

## 1. Metadata

| Field | Nilai |
|---|---|
| ID UAT | UAT-FULL-010 |
| Referensi test | `TestP5M18_SoD_Cannot_Be_Bypassed_Via_API` + `TestP5M18_Idempotency_Across_Modul` |
| Referensi FSD | FSD-APP-A §6 (SoD), FSD-APP-D §6 (Idempotency) |
| DEC terkait | DEC-017 (SoD maker≠reviewer≠approver), DEC-021 (Idempotency-Key wajib) |
| Referensi security | `.claude/memory/security-baseline.md §SoD enforcement` |

---

## 2. Persona yang Terlibat

| Persona | Role | Aksi |
|---|---|---|
| Treasury Maker | ROLE-MAKER-TR | Buat transaksi (maker), coba review/approve sendiri |
| QA Tester | — | Kirim request API langsung (tanpa melalui UI) |
| Security Engineer | — | Verifikasi audit SoD attempt |
| Internal Auditor | ROLE-AUDIT | Baca audit log SoD violation attempt |

---

## 3. Pre-kondisi

| # | Kondisi |
|---|---|
| P1 | User `usr-maker-tr-01` memiliki JWT valid dengan roles `["ROLE-MAKER-TR"]` |
| P2 | Tools: `curl` atau Postman dengan capability set JWT token arbitrary |
| P3 | Periode OPEN |
| P4 | `sys.idempotency_key` table tersedia (TTL 24 jam) |

---

## 4. Data Test

| Field | Nilai |
|---|---|
| Shared user | `usr-maker-tr-01` (satu user untuk semua SoD test) |
| Penempatan | PNP-202606-TEST-001 (dibuat oleh usr-maker-tr-01) |
| ECL Run | RUN-2026-06-TEST-001 (dibuat oleh usr-maker-tr-01) |
| Mapping Jurnal | MJ-PENEMPATAN-TEST (dibuat oleh usr-maker-tr-01) |
| Idempotency keys | 5 UUID v4 unique (satu per endpoint) |

---

## 5. Langkah-Langkah

### Bagian A: SoD Enforcement

**Step A.1 — Setup: buat entitas sebagai maker**

1. Login sebagai `usr-maker-tr-01`.
2. Buat penempatan PNP-202606-TEST-001 → submit.
3. Buat ECL run request → submit.
4. Buat mapping jurnal → submit.

Semua entitas di atas harus `maker_id = usr-maker-tr-01`.

**Step A.2 — Coba review penempatan sendiri via API (bypass UI)**

1. Buat JWT baru untuk `usr-maker-tr-01` dengan role `ROLE-APPR-TR` (simulasikan role-switching).
2. Kirim request langsung:
   ```
   POST /api/v1/transaksi/penempatan/PNP-202606-TEST-001/review
   Authorization: Bearer {token_usr_maker_tr_01_as_approver}
   Idempotency-Key: {uuid}
   { "comment": "review sendiri", "signature_method": "JWT_STEP_UP" }
   ```

Hasil yang diharapkan:
- [ ] Response: `403 { "error": { "code": "SOD_VIOLATION", "message": "..." } }`.
- [ ] Status penempatan tidak berubah (tetap `PENDING_REVIEW`).
- [ ] Audit: `PENEMPATAN.SOD_VIOLATION_ATTEMPT` tercatat dengan `actor = usr-maker-tr-01`.
- [ ] **Kritis**: Penolakan terjadi di **service layer** (bukan hanya UI).

**Step A.3 — Coba approve ECL run yang dibuat sendiri via API**

1. Kirim:
   ```
   POST /api/v1/ecl/calc-runs/RUN-2026-06-TEST-001/seal
   Authorization: Bearer {token_usr_maker_tr_01}
   ```

Hasil yang diharapkan:
- [ ] Response: `403 { "error": { "code": "SOD_VIOLATION" } }`.
- [ ] ECL run tidak di-seal.
- [ ] Audit: `ECL.SOD_VIOLATION_ATTEMPT`.

**Step A.4 — Coba approve mapping jurnal yang dibuat sendiri via API**

1. Kirim:
   ```
   POST /api/v1/master/mapping-jurnal/MJ-PENEMPATAN-TEST/approve
   Authorization: Bearer {token_usr_maker_tr_01}
   ```

Hasil yang diharapkan:
- [ ] Response: `403 { "error": { "code": "SOD_VIOLATION" } }`.
- [ ] Audit: `SECURITY.SOD_VIOLATION_ATTEMPT`.

**Step A.5 — Verifikasi audit trail (Internal Auditor)**

1. Login sebagai `usr-audit-01`.
2. `GET /api/v1/audit/log?action=SOD_VIOLATION_ATTEMPT&from=2026-06-25`.

Hasil yang diharapkan:
- [ ] 3 baris `SOD_VIOLATION_ATTEMPT` hadir (penempatan + ecl_run + mapping_jurnal).
- [ ] Setiap baris: `actor_user_id = usr-maker-tr-01`, `ip` terisi, `trace_id` ada.
- [ ] Hash chain tetap valid.

---

### Bagian B: Idempotency Cross-Modul

**Step B.1 — Penempatan create (first call)**

```
POST /api/v1/transaksi/penempatan
Idempotency-Key: {key1}
{ ... payload ... }
→ 201 { "data": { "id": "PNP-IDEM-001", ... } }
```

**Step B.2 — Penempatan create (replay dengan key yang sama)**

```
POST /api/v1/transaksi/penempatan
Idempotency-Key: {key1}   ← same key
{ ... same payload ... }
→ 200 { "data": { "id": "PNP-IDEM-001", ... } }   ← same response
```

Hasil yang diharapkan:
- [ ] HTTP status berbeda: 201 (first), 200 (replay) — atau keduanya 201 tergantung implementasi.
- [ ] Response body identik (same ID, same kode_transaksi).
- [ ] Tidak ada duplikat record di DB: `SELECT COUNT(*) FROM trx.penempatan_deposito WHERE idempotency_key = '{key1}'` → 1.
- [ ] Audit: `IDEMPOTENCY.REPLAY` tercatat pada replay call.

**Step B.3 — MTM batch upload (idempotency)**

Ulangi B.1-B.2 untuk `POST /api/v1/mtm/upload/batch` dengan `Idempotency-Key: {key2}`.

Hasil yang diharapkan:
- [ ] Replay tidak membuat batch upload baru.
- [ ] `SELECT COUNT(*) FROM sys.upload_batch WHERE idempotency_key = '{key2}'` → 1.

**Step B.4 — ECL calc run (idempotency)**

Ulangi untuk `POST /api/v1/ecl/calc-runs` dengan `Idempotency-Key: {key3}`.

Hasil yang diharapkan:
- [ ] Replay mengembalikan jobId yang sama.
- [ ] Tidak ada calc run baru dibuat.

**Step B.5 — Jurnal post (idempotency)**

Ulangi untuk `POST /api/v1/jurnal/header` dengan `Idempotency-Key: {key4}`.

Hasil yang diharapkan:
- [ ] Replay mengembalikan jurnal header yang sama.
- [ ] GL tidak menerima jurnal duplikat.

**Step B.6 — Periode hard-close (idempotency)**

Ulangi untuk `POST /api/v1/periode-buku/PBUKU-2026-05/hard-close-approve` dengan `Idempotency-Key: {key5}`.

Hasil yang diharapkan:
- [ ] Replay aman: mengembalikan response original, tidak menutup periode dua kali.
- [ ] `hard_closed_at` tidak berubah pada replay.

**Step B.7 — Idempotency mismatch (negatif test)**

```
POST /api/v1/transaksi/penempatan
Idempotency-Key: {key1}   ← same key as B.1
{ ... DIFFERENT payload ... }
→ 422 { "error": { "code": "IDEMPOTENCY_MISMATCH" } }
```

Hasil yang diharapkan:
- [ ] `422 IDEMPOTENCY_MISMATCH`.
- [ ] Tidak ada penempatan baru dibuat.
- [ ] Audit: `IDEMPOTENCY.MISMATCH_ATTEMPT`.

---

## 6. Audit Checks

| Aksi | Audit Action |
|---|---|
| SoD: review sendiri | `PENEMPATAN.SOD_VIOLATION_ATTEMPT` |
| SoD: seal sendiri | `ECL.SOD_VIOLATION_ATTEMPT` |
| SoD: approve mapping sendiri | `SECURITY.SOD_VIOLATION_ATTEMPT` |
| Idempotency replay | `IDEMPOTENCY.REPLAY` (5×) |
| Idempotency mismatch | `IDEMPOTENCY.MISMATCH_ATTEMPT` |

---

## 7. Rollback / Cleanup

1. Hapus test data: soft-delete penempatan, ECL run, mapping jurnal.
2. Clear `sys.idempotency_key` test entries setelah 24 jam (auto-expire).

---

## 8. Sign-Off

| Peran | Nama | Tanggal | Hasil | Tanda tangan |
|---|---|---|---|---|
| QA Engineer | | | PASS / FAIL | |
| Security Engineer | | | APPROVED / REJECT | |
| Compliance Officer | | | APPROVED / REJECT | |
| Internal Auditor | | | VERIFIED | |
| CFO | | | VERIFIED | |
