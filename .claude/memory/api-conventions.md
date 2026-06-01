# API Conventions — BLIPS REST

## Path
`METHOD /api/v1/{resource}` — kebab-case di path, snake_case di query, camelCase di JSON body.

Contoh:
- `GET  /api/v1/master/instrumen?bank_id=...&cursor=...`
- `POST /api/v1/transaksi/penempatan`
- `POST /api/v1/ecl/calc-run`
- `PATCH /api/v1/sppi/test-run/{id}/answer`

## Versioning
- URL prefix `/api/v1/` — major versioning.
- Breaking change → `/api/v2/`, **never** mutate v1 contract.

## Headers wajib (mutating endpoints)
| Header | Required | Purpose |
|---|---|---|
| `Authorization: Bearer <jwt>` | Always | Auth |
| `Idempotency-Key: <uuid v4>` | Mutation (POST/PATCH/PUT/DELETE) | Dedup |
| `X-Trace-Id` | Optional (gateway injects) | Trace propagation |
| `Content-Type: application/json` | Body present | — |

## Pagination — cursor only (no offset)
Query params:
- `cursor` — opaque base64 cursor (server-managed)
- `limit` — int, default 50, max 200

Response:
```json
{
  "data": [...],
  "pagination": {
    "nextCursor": "eyJpZCI6...==",
    "hasMore": true
  }
}
```

## Success envelope
```json
{
  "data": { ... },                  // object atau array
  "meta": { "traceId": "..." }
}
```

## Error envelope
```json
{
  "error": {
    "code": "SPPI_TEST_INCOMPLETE",
    "message": "SPPI test belum lengkap, 3 jawaban kosong",
    "details": [
      { "field": "answer.Q7", "rule": "required" }
    ],
    "traceId": "..."
  }
}
```

### Error codes (stable strings)
| Code | HTTP | When |
|---|---|---|
| `VALIDATION_FAILED` | 400 | Field validation gagal |
| `UNAUTHORIZED` | 401 | JWT invalid/missing |
| `FORBIDDEN` | 403 | Permission `{entity}.{action}` tidak terpenuhi |
| `SOD_VIOLATION` | 403 | maker=reviewer=approver |
| `NOT_FOUND` | 404 | Entity tidak ditemukan |
| `CONFLICT` | 409 | Optimistic lock (row_version mismatch) |
| `IDEMPOTENCY_REPLAY` | 200 | Same key, return original response |
| `IDEMPOTENCY_MISMATCH` | 422 | Same key, different payload |
| `SPPI_TEST_INCOMPLETE` | 422 | Belum semua Q1–Q10 dijawab |
| `BM_ASSESSMENT_REQUIRED` | 422 | BM belum di-assess untuk portofolio |
| `ECL_PARAM_FROZEN` | 423 | Calc run sealed, tidak bisa diubah |
| `WORKFLOW_INVALID_TRANSITION` | 422 | State machine reject transition |
| `PERIODE_CLOSED` | 423 | Periode hard-closed, tidak bisa post |
| `RATE_LIMITED` | 429 | Too many requests |
| `INTERNAL` | 500 | Unexpected, log + alert |

## Auth & Permission
- JWT signed RSA-2048, claims:
  ```json
  {
    "sub": "user-uuid",
    "preferred_username": "...",
    "roles": ["ROLE-MAKER-TR"],
    "permissions": ["instrumen.create", "instrumen.read"],
    "tenant_id": "TUGURE",
    "mfa_verified": true,
    "exp": 1748...,
    "iat": 1748...
  }
  ```
- Permission check di service layer pakai `{entity}.{action}`, **bukan** role string comparison.
- Step-up MFA untuk action sensitif (hard-close, ECL parameter approve, klasifikasi).

## Date & Number formatting
- Date: ISO 8601 dengan timezone, `2026-06-02T10:30:00+07:00` (atau `Z`).
- Date-only: `2026-06-02` (string).
- Money: number, jangan string. Presisi sesuai field (IDR 4 decimal, FX 8 decimal).
- Boolean: `true`/`false`, jangan 0/1 atau "Y"/"N".

## Idempotency
- Server table `sys.idempotency_key`:
  ```sql
  (key UUID PK, request_hash BYTEA, response_json JSONB, status SMALLINT,
   created_at TIMESTAMPTZ, expires_at TIMESTAMPTZ)
  ```
- TTL: 24 jam.
- Same key + same payload → return original response (200/201/4xx — apapun yang asli).
- Same key + different payload → `IDEMPOTENCY_MISMATCH` 422.

## Soft delete via API
- `DELETE /api/v1/{resource}/{id}` → set `deleted_at`, `deleted_by` di tabel target (kecuali `aud`, `jrnl`, `ecl` — refuse 403).
- Filter `deleted_at IS NULL` default. Query param `?include_deleted=true` hanya untuk ROLE-AUDIT.

## Workflow endpoints
Mutation pada entity workflow-bearing **tidak** PATCH biasa. Pakai:
- `POST /api/v1/{resource}/{id}/submit`     — maker submits
- `POST /api/v1/{resource}/{id}/review`     — reviewer signs
- `POST /api/v1/{resource}/{id}/approve`    — approver signs
- `POST /api/v1/{resource}/{id}/reject`     — reviewer/approver rejects with comment

Setiap signing endpoint butuh: `{ comment: string, signature_method: "JWT_STEP_UP" }`.

## Rate limiting
- Default 100 req/min per user.
- Sensitif (hard-close, parameter approve): 10 req/min.
- Bypass untuk ROLE-AUDIT read-only: 500 req/min.
