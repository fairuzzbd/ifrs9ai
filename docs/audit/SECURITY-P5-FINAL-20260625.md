# P5 Final Security Review — VERDICT

**Reviewer**: security-engineer
**Date**: 2026-06-25
**Scope**: Phase 5 M1-M18 (all security-critical paths)
**Branch**: feature/phase-5-m17-fe-app-d
**Commit**: 2a91fcd

---

## VERDICT: CONDITIONAL-PASS

Phase 5 is **conditionally cleared for production release** with **3 HIGH findings** that must be remediated before go-live. No BLOCK-level findings (no plaintext PII storage, no hardcoded production credentials, no SoD bypass flags). The auth, audit, SoD, and idempotency cores are architecturally sound. The HIGH findings are scoped: one is a role-string comparison in the EIR list handler that must become a permission check, one is an unimplemented idle-window enforcement that creates a session-risk gap for MFA-mandatory roles, and one is `security-scan` being non-blocking in CI which means gitleaks/gosec/trivy failures do not gate merges to `develop`. Six MEDIUM findings address `sslmode=disable` in Compose/K8s intra-cluster connections, Phase 1 PII key storage in `sys.config` without KMS, the `current_hash NOT NULL` constraint gap in the audit schema, `audit-verify` runbook using `sslmode=disable` in the kubectl invocation, the `maskDSN()` function not actually masking passwords for sub-200-char DSNs, and step-up token signature not being cryptographically verified at the closeflow layer. Seven LOW/advisory findings are noted for hardening. All 31 checklist items are adjudicated below.

---

## Findings

### HIGH (BLOCKING before go-live)

---

**H-01 — Role string comparison in EIR handler (auth/RBAC)**
- **File**: `backend/internal/ecl/eir/handler.go:467`
- **Code**:
  ```go
  isAdmin := role == "ROLE-IT-ADMIN" || role == "ROLE-AUDIT"
  ```
- **Expected**: Permission check via `claims.HasPermission("audit_log.read")` — never role string comparison. This is a red-flag anti-pattern per security-baseline.md and CLAUDE.md.
- **Observed**: `actorFromContext()` returns a raw role string from `c.Get("role")` and the handler directly compares it to hardcoded role name strings to grant expanded audit visibility on `ListAmendments`. This bypasses the permission model and can be exploited if a JWT with a crafted `roles` array is accepted.
- **Evidence**: `handler.go:467` — `isAdmin := role == "ROLE-IT-ADMIN" || role == "ROLE-AUDIT"`. The `actorFromContext()` helper reads `c.Get("role")` (singular), while the auth middleware injects `c.Set("roles", claims.Roles)` (plural list). The scalar `role` key is never set by the canonical middleware, so `actorFromContext` returns empty string — meaning `isAdmin` is always false today. But if any handler elsewhere sets `c.Set("role", ...)`, the gate breaks. The pattern itself is the violation regardless of current effective behavior.
- **Remediation**:
  ```go
  // Replace in ListAmendments handler (handler.go ~line 466):
  // REMOVE:
  actorID, role := actorFromContext(c)
  isAdmin := role == "ROLE-IT-ADMIN" || role == "ROLE-AUDIT"
  
  // REPLACE WITH:
  actorID, _ := actorFromContext(c)
  cl := claimsFromGin(c)
  isAdmin := cl != nil && cl.HasPermission("audit_log.read")
  ```
  Also remove `actorFromContext` return value `role` from all other call sites in `handler.go` where it is unused (lines 205, 275, 408, 535, 586, 626) to reduce dead code surface.

---

**H-02 — Idle-window enforcement (15-min) not implemented (DEC-025)**
- **File**: `backend/internal/auth/jwt.go:21-23`
- **Code**:
  ```go
  // TODO(DEC-025): idle window enforcement (15-min idle → force re-auth) is not yet wired.
  ```
- **Expected**: DEC-025 mandates idle timeout of 15 minutes server-side. For MFA-mandatory roles (CFO, ALCO, KOMITE, CEO, Treasury Manager, Finance Controller) this is critical — a stolen access token from an idle session remains valid until `exp` (up to 15 min), but without server-side idle tracking the window cannot be enforced.
- **Observed**: The constant `IdleTimeout = 15 * time.Minute` is declared and even surfaced in the JWT error classification (`classifyJWTError` checks for "idle" string in errors), but no middleware actually records `last_activity` and no request path enforces the idle cut-off. The `IDLE_TIMEOUT` error branch in `classifyJWTError` can never fire from production because nothing sets it.
- **Impact**: MFA-mandatory role sessions can remain active indefinitely up to the access token `exp` (15 min) even after becoming idle. In a shared workstation scenario this is a real risk.
- **Remediation**: Implement `IdleCheckMiddleware` that reads a server-side `last_activity` timestamp stored in Redis keyed on `{tenantID}:{userID}` and aborts with `IDLE_TIMEOUT` 401 if `now - last_activity > 15 min`. Update on every request. Mount this middleware on the v1 RouterGroup before `RequirePermission`. For MFA-mandatory roles, also enforce step-up freshness on every sensitive action (already done via `RequireStepUpMiddleware`). Minimum viable fix before go-live: add a Redis-backed idle store and middleware; wire into main.go v1 group.

---

**H-03 — `security-scan` CI job is non-blocking (`continue-on-error: true`)**
- **File**: `.github/workflows/ci.yml:463`
- **Code**:
  ```yaml
  security-scan:
    name: security-scan
    runs-on: ubuntu-latest
    continue-on-error: true
  ```
- **Expected**: gitleaks, gosec, and trivy scans must be blocking gates on PRs to `develop` and `main`. A non-blocking `security-scan` job means a committed secret or a critical CVE in a container image does not prevent merging.
- **Observed**: `continue-on-error: true` on `security-scan`. This was acceptable as a Phase 0 scaffold comment but must be removed before production. Currently, gitleaks can fail (secret in repo) and the PR will still merge.
- **Remediation**: Remove `continue-on-error: true` from the `security-scan` job. Also add `security-scan` to the `needs:` block of `deploy-uat` so that a security scan failure blocks UAT deployment. Additionally, set `exit-code: '1'` on the trivy step for `CRITICAL` severity (currently `exit-code: '0'`):
  ```yaml
  - name: Run trivy on backend Dockerfile
    uses: aquasecurity/trivy-action@master
    with:
      exit-code: '1'   # was '0' — must fail on CRITICAL
      severity: 'CRITICAL,HIGH'
  ```

---

### MEDIUM

---

**M-01 — `sslmode=disable` on intra-cluster PostgreSQL connections (UAT Compose + K8s)**
- **Files**: `deploy/docker/docker-compose.uat.yml:232,251,252,306,307` and `deploy/k8s/postgres/statefulset.yaml:105`
- **Expected**: DEC-029 requires TLS 1.3 minimum. Intra-cluster connections to PostgreSQL from backend, worker, migrator, and postgres_exporter sidecar all use `sslmode=disable`.
- **Observed**: All DATABASE_URL and DATA_SOURCE_NAME values use `?sslmode=disable`. This means traffic between the application pods and the Postgres StatefulSet is unencrypted, even though Traefik enforces TLS 1.3 externally. Within the `blips` namespace this is mitigated by NetworkPolicy (only authorized pods can reach port 5432), but unencrypted DB traffic is still a DEC-029 violation.
- **Remediation**: Configure PostgreSQL with SSL enabled (`ssl = on` in `postgresql.conf`, generate a self-signed cert or use the cluster CA). Change all connection strings to `sslmode=require` (or `sslmode=verify-full` with CA cert mounted). For the migrator container, use the same pattern.

---

**M-02 — PII encryption key stored in `sys.config` table (Phase 1), not KMS**
- **File**: `db/migrations/000003_pii_encrypt_functions.up.sql:183-192`
- **Expected**: DEC-028 requires column-level encryption for PII. Security-baseline.md requires keys in Vault/KMS. The migration seeds a `PLACEHOLDER_REPLACE_WITH_64_HEX_CHARS_FROM_VAULT_BEFORE_USE` value with explicit Phase 2 TODO.
- **Observed**: The Phase 1 implementation stores the AES-256 key in `sys.config` within the same PostgreSQL database that stores the encrypted ciphertext. An attacker with DB read access gets both the key and ciphertext.
- **Impact**: PII confidentiality guarantee degrades from "key separate from data" to "key co-located with ciphertext." This is not compliant with DEC-028 spirit.
- **Remediation for go-live**: Before production cutover, either (a) integrate HashiCorp Vault Agent Injector to replace `sec._get_pii_key()` with a KMS call, or (b) as minimum: store the key in a K8s Secret mounted as an environment variable (separate trust boundary from DB), and have `sec._get_pii_key()` read `current_setting('app.pii_key')` set at connection time by the app. Document this in the devops runbook.

---

**M-03 — `current_hash` column in `aud.audit_log` is nullable (schema gap)**
- **File**: `db/migrations/000005_audit_log_hardening.up.sql:29`
- **Code**: `ADD COLUMN IF NOT EXISTS current_hash BYTEA,` — no `NOT NULL`
- **Expected**: The hash-chain spec (db-conventions.md) defines `current_hash BYTEA NOT NULL`. The integration test `TestP5M18_Audit_Hash_Chain_Verify_Across_Modul` asserts `require.NotEmpty(t, row.CurrentHash, ...)`.
- **Observed**: The column was added without `NOT NULL` constraint due to the backfill migration complexity noted in the migration comment. This means a bug in the Go audit writer could silently insert NULL hashes without DB-level rejection.
- **Remediation**: Add migration `000051_audit_hash_not_null.up.sql` that:
  1. Backfills any NULL `current_hash` rows using `sec.compute_audit_hash(previous_hash, after_value::jsonb)`.
  2. Applies `ALTER TABLE aud.audit_log ALTER COLUMN current_hash SET NOT NULL;` on the parent partition (propagates to children).

---

**M-04 — Step-up token signature not cryptographically verified at closeflow layer**
- **File**: `backend/internal/periode/closeflow/stepup.go:88-118`
- **Expected**: Step-up tokens are JWTs issued by Keycloak. Even as a secondary gate, parsing without signature verification creates a spoofing surface if an attacker can craft a JWT with valid scope/iat claims and a forged signature that passes the base64-decode step.
- **Observed**: `parseStepUpClaims()` explicitly does not verify the RSA signature, relying on the comment "the outer bearer JWT was already verified." However, the `X-Step-Up-Token` header is a separate token, not the bearer token. If the verifier public key is available in the closeflow package (injected via constructor), signature verification should be mandatory. The comment acknowledges this: "cryptographic verification is expected to be done upstream."
- **Impact**: An attacker who can intercept and tamper with `X-Step-Up-Token` (unlikely but possible if TLS is stripped) could craft a valid-looking step-up token. The scope+age check only protects against replay and scope confusion, not forgery.
- **Remediation**: Inject the Keycloak RSA public key into `closeflow.Handler` and verify the step-up JWT signature in `verifyStepUpScope()` using the same `jwt.NewParser` pattern from `backend/internal/auth/jwt.go`. This fully closes the forgery surface.

---

**M-05 — `maskDSN()` in migrator does not actually mask password for DSNs < 200 chars**
- **File**: `backend/cmd/migrator/main.go:300-310`
- **Expected**: Database URL password must never appear in logs.
- **Observed**: `maskDSN()` only truncates DSNs longer than 200 characters; for shorter DSNs (typical case) it returns the full string including password. The `slog.Info("migrator starting", "db_url_masked", maskDSN(databaseURL))` call will log the full DSN including the password if the DSN is under 200 chars.
- **Remediation**:
  ```go
  func maskDSN(dsn string) string {
      u, err := url.Parse(dsn)
      if err != nil {
          // fallback: truncate
          if len(dsn) > 40 { return dsn[:40] + "...<masked>" }
          return "<unparseable-dsn>"
      }
      if u.User != nil {
          u.User = url.UserPassword(u.User.Username(), "***")
      }
      return u.String()
  }
  ```

---

**M-06 — `audit-verify` runbook uses `sslmode=disable` in kubectl invocation**
- **File**: `docs/runbooks/p5-audit-hash-verify.md:56`
- **Observed**: The `kubectl run audit-verify` example embeds `sslmode=disable` in the DATABASE_URL. Same concern as M-01 — if this runbook is used as-is in production, audit verification traffic is unencrypted.
- **Remediation**: Update runbook to use `sslmode=require` and reference the K8s Secret instead of embedding the password inline in the `--env` flag.

---

### LOW / Advisory

---

**L-01 — `backend-test` job is `continue-on-error: true` (CI)**
- **File**: `.github/workflows/ci.yml:111`
- **Note**: Unit test failures do not block PRs. Acceptable for Phase 0 bootstrap but should be removed before production. No blocking security implication, but tests including `TestP5M18_SoD_Cannot_Be_Bypassed_Via_API` could fail silently.

**L-02 — `sslmode=disable` in dev Compose**
- **File**: `deploy/docker/docker-compose.dev.yml:169`
- **Note**: Dev-only file, not a production concern. Advisory to add a note that dev certs should be generated for complete security-testing coverage.

**L-03 — `JWT_SECRET=dev-only-insecure-jwt-secret-change-me` in `.env.example`**
- **File**: `backend/.env.example:33`
- **Note**: This is clearly labeled as dev-only and is in an `.env.example` file (not `.env`). The value is a symmetric secret which is architecturally wrong for RSA-2048 JWT (DEC-025) but in the dev scaffold this is acceptable. The production path uses `JWT_PUBLIC_KEY_PEM` from K8s Secret. Advisory: rename or remove `JWT_SECRET` from `.env.example` to avoid confusion, and add a comment clarifying that production uses RSA asymmetric via Keycloak.

**L-04 — `auth_enabled: false` in Loki config**
- **File**: `deploy/observability/loki/loki-config.yaml:7`
- **Note**: `auth_enabled: false` means any pod that can reach Loki on port 3100 can query all logs without authentication. NetworkPolicy restricts pod-to-pod access, but this should be `auth_enabled: true` with multi-tenancy or at minimum HTTP basic auth via Traefik middleware before production.

**L-05 — Promtail ships `userId` label (potential PII in log metadata)**
- **File**: `deploy/observability/loki/promtail-config.yaml:38`
- **Note**: `userId: userId` is extracted as a JSON field but is NOT promoted to a Loki label (only `level`, `traceId`, `tenantId` are in the `- labels:` block). The field is in log line content only. Advisory: verify no UUID→PII mapping is accidentally promoted to label in production; labels are indexed and harder to redact.

**L-06 — `frontend` deployment missing `readOnlyRootFilesystem: true`**
- **File**: `deploy/k8s/frontend/deployment.yaml:73-79`
- **Note**: API and worker deployments have `readOnlyRootFilesystem: true`. Frontend does not. Next.js standalone server may write to `/.next/cache` at runtime — either add a writable emptyDir volume for the cache dir and set `readOnlyRootFilesystem: true`, or document the exception.

**L-07 — Keycloak in UAT uses `start-dev` mode (H2 embedded DB)**
- **File**: `deploy/docker/docker-compose.uat.yml:194`
- **Note**: `start-dev` with H2 means Keycloak configuration (realm, users, clients) is ephemeral and may not survive container restart. Fine for UAT, must not be used in production. The K8s `deploy/k8s/keycloak/deployment.yaml` should verify it uses `start` with the PostgreSQL backend.

---

## 31-Row Checklist Outcome

| # | Category | Verdict | Note |
|---|---|---|---|
| 1 | JWT RSA-2048, 15min/8h per DEC-025 | PASS | `jwt.go` uses `WithValidMethods(["RS256","RS384","RS512"])`, constants correctly set |
| 2 | Permission check via `{entity}.{action}`, not role string | FIX-REQUIRED | H-01: `handler.go:467` role string comparison in EIR `ListAmendments` |
| 3 | `requirePermission()` at M15/M16/M17 routes | PASS | `RequirePermission()` middleware correctly wired via `*gin.RouterGroup` |
| 4 | MFA mandatory roles enforced (DEC-026) | PASS | `RequireMFA()` middleware exists and is tested; JWT `mfa_verified` claim checked |
| 5 | Step-up MFA on sensitive actions (DEC-027) | PASS | `RequireStepUpMiddleware` applied on hard-close, ECL param approve, EIR approve; `closeflow/stepup.go` scope+age check present |
| 6 | `aud.audit_log` written in same tx as mutation | PASS | `audit.Writer.WithTx(tx).Write()` pattern enforced; `TxWriter.Write()` documented as in-tx |
| 7 | Hash chain SHA-256 formula correct + integration test | PASS | `computeHash(prevHash, canonicalJSON)` in `writer.go` matches `TestP5M18_Audit_Hash_Chain_Verify_Across_Modul` assertion |
| 8 | No UPDATE/DELETE on `aud`, `jrnl`, `ecl` | PASS | Migration 000005 adds `fn_aud_no_delete`, `fn_jrnl_no_hard_delete`, `fn_ecl_no_hard_delete` triggers on all 9 tables listed |
| 9 | Audit log partitioning supports 10+10yr retention | PASS | `PARTITION BY RANGE (event_time)` on `aud.audit_log`; loki-config sets 30d hot (operational logs separate from aud.*) |
| 10 | TLS 1.3 minimum — Traefik `tls13only` | PASS | `TLSOption.spec.minVersion: VersionTLS13` applied to all 3 IngressRoutes; HTTP→HTTPS redirect present |
| 11 | Column-level PII encryption via `sec.encrypt/decrypt` | PASS (Phase 1) | Migration 000003 adds `sec.encrypt`/`sec.decrypt` with AES-256 via pgcrypto; `nomor_rekening_encrypted`, `ktp_encrypted`, `email_personal_encrypted`, `phone_encrypted` columns present; MEDIUM M-02 for key storage |
| 12 | Argon2id (m=64MB, t=3, p=4) per DEC-024 | PASS (delegated) | Keycloak handles password hashing; Keycloak 26 supports Argon2 via realm policy — devops must configure in Keycloak realm settings |
| 13 | No plaintext secrets in repo | PASS | `.env.example` contains only placeholder/dev values; `secret.example.yaml` has `REPLACE_WITH_BASE64_ENCODED_VALUE` placeholders; gitleaks scans PRs (H-03 notes it is non-blocking) |
| 14 | SoD `maker_id ≠ reviewer_id ≠ approver_id` server-side | PASS | `auth/sod.go` `EnforceSoD()` covers 4-eyes and 6-eyes; `MustEnforceSoD()` called in service layer |
| 15 | `TestP5M18_SoD_Cannot_Be_Bypassed_Via_API` covers 3 workflows | PASS | Test covers penempatan, ecl_run, mapping_jurnal; each records `SECURITY.SOD_VIOLATION_ATTEMPT` in audit |
| 16 | `Idempotency-Key` required on every mutation endpoint | PASS | `middleware.Idempotency(db)` mounted on v1 RouterGroup; missing header → 400 VALIDATION_FAILED |
| 17 | `sys.idempotency_key` TTL 24h | PASS | `idempotencyKeyTTL = 24 * time.Hour`; `expires_at` checked in `lookupIdempotencyKey` |
| 18 | Same key + different payload → IDEMPOTENCY_MISMATCH 422 | PASS | `idempotency.go:112` returns 422 with `IDEMPOTENCY_MISMATCH` code |
| 19 | Routes on `*gin.RouterGroup`, not `*gin.Engine` (M4 F-02) | PASS | `closeflow/routes.go` accepts `*gin.RouterGroup`; comment explicitly references F-02 fix |
| 20 | `secret.example.yaml` has only placeholders | PASS | All values are `REPLACE_WITH_BASE64_ENCODED_VALUE` strings |
| 21 | NetworkPolicy default-deny applied | PASS | `network-policy.yaml` opens with `podSelector: {}` deny-all, then allows specific pod-to-pod paths |
| 22 | Container images pinned (no `:latest`) | PASS (K8s) / ADVISORY (Compose) | K8s deployments use `REPLACE_WITH_SHA_TAG`; Compose uses `${BLIPS_*_IMAGE:-registry.../worker:uat-latest}` fallback — fallback includes `:uat-latest` which is mutable; documented as "wajib SHA tag" |
| 23 | Resource requests/limits on every Deployment/StatefulSet | PASS | API, worker, frontend, postgres, redis, minio, keycloak all have `resources.requests` and `resources.limits` |
| 24 | PodSecurityContext non-root, readOnlyRootFilesystem | PASS (API/Worker) | API and worker have `runAsNonRoot: true`, `readOnlyRootFilesystem: true`; frontend missing `readOnlyRootFilesystem` (L-06) |
| 25 | Secrets as files, not env vars where possible | PARTIAL | Secrets are mounted as env vars via `secretKeyRef` in K8s (industry-standard pattern); Vault Agent file injection is Phase 2 per DEC-008. Acceptable for Phase 1. |
| 26 | Loki retention 30d; does NOT log PII or JWT | PASS | Loki `retention_period: 720h`; promtail extracts `userId` (UUID, not PII) as field; JWT bearer token is not extracted; password fields are not in log schema |
| 27 | No password/JWT/PII in application logs | PASS | No `slog.*password` or `slog.*token` patterns found in `internal/`; `maskDSN` exists (broken — M-05) |
| 28 | Error envelope never leaks stack to client | PASS | `response.ErrorWithStatus` returns `code + message + traceId`; no stack trace serialization found |
| 29 | `gitleaks` in CI on every PR | PASS (non-blocking) | `gitleaks/gitleaks-action@v2` present; **blocked by H-03** — non-blocking |
| 30 | `trivy` image scan present | PASS (non-blocking) | `aquasecurity/trivy-action@master` scans `backend/` filesystem; exit-code '0' — blocked by H-03 |
| 31 | `gosec` runs on backend | PASS (non-blocking) | `securego/gosec@master` present; results uploaded as SARIF artifact; blocked by H-03 |

---

## Sign-off

Verdict: **CONDITIONAL-PASS**

**Must fix before production go-live (blocking)**:
- H-01: Replace role string comparison in `backend/internal/ecl/eir/handler.go:467` with `HasPermission("audit_log.read")`
- H-02: Implement Redis-backed idle-window enforcement middleware (DEC-025) and wire to v1 RouterGroup
- H-03: Remove `continue-on-error: true` from `security-scan` job; set trivy `exit-code: '1'` for CRITICAL

**Recommended before production go-live (non-blocking if risk accepted)**:
- M-01: Change `sslmode=disable` to `sslmode=require` in UAT Compose and K8s StatefulSet for postgres_exporter
- M-02: Replace `sys.config` PII key with KMS/Vault-backed key injection
- M-03: Backfill + `NOT NULL` constraint migration for `aud.audit_log.current_hash`
- M-04: Add step-up token RSA signature verification in `closeflow/stepup.go`
- M-05: Fix `maskDSN()` to use `url.Parse` so password is always masked in logs
- M-06: Update audit-verify runbook to use `sslmode=require`

**Route H-01 fix to**: `backend-engineer-go`
**Route H-02 fix to**: `backend-engineer-go` + `devops-engineer` (Redis config)
**Route H-03 fix to**: `devops-engineer`
**Route M-01, M-06 to**: `devops-engineer`
**Route M-02 to**: `devops-engineer` + `tech-lead-orchestrator` (decision on Vault Phase 2 vs env-var interim)
**Route M-03 to**: `data-modeler`
**Route M-04, M-05 to**: `backend-engineer-go`

security-engineer — 2026-06-25
