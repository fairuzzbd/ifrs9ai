# Security Baseline — BLIPS

## Auth chain
```
User → Browser → Traefik (TLS 1.3) → Keycloak (SAML/OIDC) ← LDAP/AD Tugure
                                  ↓
                                  JWT (RSA-2048, 15min access + 8h refresh)
                                  ↓
                                  API service (verify signature + scope + tenant)
```

## JWT claims (canonical)
| Claim | Required | Notes |
|---|---|---|
| `sub` | yes | user UUID |
| `preferred_username` | yes | display |
| `roles` | yes | array of `ROLE-*` |
| `permissions` | yes | array of `{entity}.{action}` |
| `tenant_id` | yes | `TUGURE` (Phase 1 single tenant) |
| `mfa_verified` | yes | boolean |
| `mfa_method` | when true | `TOTP`, `WEBAUTHN`, `PUSH` |
| `exp`, `iat`, `nbf` | yes | standard |

## MFA mandatory roles
- ROLE-CFO
- ROLE-CEO
- ROLE-KOMITE
- ROLE-ALCO
- Treasury Manager (sub-role MAKER-TR/APPR-TR senior)
- Finance Controller (ROLE-AKUN-CTL)

Action-level step-up MFA (re-prompt even if `mfa_verified=true` & lebih dari 5 menit lalu):
- Hard-close periode buku
- ECL parameter approve (PD curve, LGD pool, weights, FL multiplier)
- Klasifikasi PSAK 71 approval
- Calc-run seal

## Permission model
- Pattern: `{entity}.{action}`
- Entities: `instrumen`, `counterparty`, `portofolio`, `penempatan`, `transaksi`, `sppi_test`, `bm_assessment`, `klasifikasi`, `ecl_run`, `ecl_parameter`, `eir`, `periode`, `jurnal`, `fx_rate`, `user`, `role`, `audit_log`
- Actions: `create`, `read`, `update`, `delete`, `submit`, `review`, `approve`, `reject`, `seal`, `reopen`, `export`
- Wildcard: `aud.*.read` untuk ROLE-AUDIT (read-only di semua schema)

## SoD enforcement
Service layer wajib check sebelum workflow transition:
```go
if workflow.MakerID == currentUser.ID && step == "REVIEW" {
    return ErrSoDViolation("maker cannot be reviewer")
}
if (workflow.MakerID == currentUser.ID || workflow.ReviewerID == currentUser.ID) && step == "APPROVE" {
    return ErrSoDViolation("maker or reviewer cannot be approver")
}
```

**Bukan** hanya di UI. Test integration wajib cover skenario "Maker mencoba menjadi Approver lewat API langsung".

## Encryption
| Layer | Standard |
|---|---|
| Transit | TLS 1.3 (Traefik termination, mTLS internal services) |
| At rest (full DB) | TDE (Transparent Data Encryption) AES-256 |
| At rest (column-level PII) | AES-256-GCM, key di KMS |
| Password (Keycloak) | Argon2id (m=64MB, t=3, p=4) |
| Signature | RSA-2048 atau Ed25519, private key di HSM/KMS |
| Hash chain (audit) | SHA-256 |

### Column-level encrypted PII fields
- `mst.counterparty.npwp`
- `mst.counterparty.nomor_rekening`
- `mst.counterparty.ktp`
- `sec.user.email_personal`
- `sec.user.phone`

Encryption transparent via PG function `sec.encrypt(col)` / `sec.decrypt(col)`, role-gated.

## Secrets management
- **Never** in repo, `.env` committed, atau Docker image.
- Vault (HashiCorp) atau Keycloak vault per Phase 2.
- Boot-time lookup via `vault kv get` atau `KMS_DECRYPT(ciphertext)`.
- Rotate quarterly. Service restart triggered on rotation (Asynq cron).

## Audit trail — required fields per mutation
Setiap mutasi (INSERT/UPDATE/SOFT-DELETE) wajib menulis row ke `aud.audit_log` **di transaksi yang sama**:
- `event_id, event_time, actor_user_id, actor_role`
- `action` (e.g. `INSTRUMEN.CREATE`)
- `entity_type, entity_id`
- `before_jsonb, after_jsonb`
- `ip, user_agent, trace_id, idempotency_key`
- `previous_hash, current_hash`

Tools verifikasi hash chain: `cmd/audit-verify --range "2026-06-01:2026-06-30"`. Job di-jalankan harian + ad-hoc.

## Threat-model checklist (untuk setiap endpoint baru)
1. **Auth** — JWT valid? Scope/permission check ada? Bukan role string?
2. **Tenancy** — `tenant_id` di WHERE clause repository?
3. **Input** — SSRF (URL params jangan di-fetch unvalidated), SQLi (param binding wajib, never string concat), path traversal (no `../`), XXE (XML parser disabled DOCTYPE), insecure deserialization (no `pickle`, no Java serialization).
4. **Output** — PII redacted di response? Log? Error?
5. **Audit** — mutation tulis ke `aud.audit_log` di same tx?
6. **Idempotency** — mutation cek `Idempotency-Key` against `sys.idempotency_key`?
7. **SoD** — workflow transition cek `maker_id ≠ reviewer_id ≠ approver_id`?
8. **Rate limit** — bucket per user + per endpoint?
9. **Error** — internal errors di-mask, hanya `traceId` + safe message ke client?
10. **Signature** — approval action store `signature_hash` + `signed_at` + `signature_method`?

## BLOCKING red flags (refuse merge)
- Plaintext PII di kolom non-encrypted
- Hardcoded credentials / API keys / secrets
- Role string comparison di code (`if user.role == "CFO"`)
- `aud.audit_log` write di luar business transaction
- Skip Idempotency-Key check di mutating endpoint
- `// TODO: remove in prod` flags untuk SoD bypass
- Return full `before/after` jsonb ke non-AUDIT role
- Logging password/JWT/PII

## Retention
- `aud.audit_log`: **10 + 10 tahun** (sesuai regulasi). Tidak boleh hard-delete. Bisa di-archive ke cold storage (MinIO Glacier-equivalent) setelah 5 tahun.
- Soft-deleted records (`deleted_at IS NOT NULL`): 5 tahun, lalu archive.
- Session token: 8 jam refresh.
- Idempotency keys: 24 jam.
