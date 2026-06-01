# Locked Decisions — Quick Reference

> **Source of truth**: `BLIPS_Decision_Log_v1.0.docx`. Ringkasan di bawah ini boleh stale — selalu cek dokumen sebelum reopen.

## DEC — Tech Stack (LOCKED)
- **DEC-001** Backend: **Go 1.22+** dengan Gin (HTTP), GORM v2 (CRUD), sqlx (heavy queries), Asynq (jobs), golang-migrate (DDL).
- **DEC-002** Frontend: **Next.js 14+** App Router, TypeScript strict, shadcn/ui, Zustand, TanStack Query, Recharts.
- **DEC-003** Database: **PostgreSQL 18** (primary + read replica + standby DR). Redis 7+ untuk Asynq + cache.
- **DEC-004** Object Storage: **MinIO** on-prem (S3 compatible).
- **DEC-005** GL Integration: **Phase 1 deferred** (file batch). REST real-time = Phase 2.
- **DEC-006** Auth: **Keycloak** sebagai SSO Hub, federasi ke LDAP/AD Tugure via SAML 2.0/OIDC.
- **DEC-007** Job queue: **Asynq** (Go-native, Redis-based). Temporal sebagai Phase 2 jika butuh long-running.
- **DEC-008** Deployment: **On-premise Jakarta DC** (UU PDP data residency), Docker Compose dev/UAT, **Kubernetes** prod, Terraform + Ansible.
- **DEC-009** Observability: **Prometheus + Grafana + Loki** (Tempo opsional Phase 2).

## DEC — Domain (LOCKED)
- **DEC-010** ECL formula: 3-stage × 3-skenario × dual forward-looking, default bobot Good/Normal/Bad = **0.25/0.50/0.25**, ALCO dapat override per periode.
- **DEC-011** SICR trigger: rating turun **≥ 2 notch**, IG → non-IG, DPD ≥ 30 hari (any of).
- **DEC-012** Cure: **3 bulan berturut-turut** memenuhi kriteria.
- **DEC-013** EIR: **Newton-Raphson IRR**, tolerance 1e-10, max 100 iter, presisi 8 desimal.
- **DEC-014** LPS Aggregator: **IDR 2 miliar** per nasabah per bank, applied **sebelum** ECL.
- **DEC-015** Look-through ECL untuk Reksadana: decompose by underlying asset class.
- **DEC-016** Decimal lib: **shopspring/decimal** (Go). Storage `NUMERIC(20,4)` IDR, `NUMERIC(20,8)` FX, `NUMERIC(10,8)` PD/LGD/EIR.
- **DEC-017** Workflow: **4-eyes** rutin, **6-eyes** untuk klasifikasi PSAK 71 + parameter master. SoD `maker ≠ reviewer ≠ approver`.
- **DEC-018** Audit trail: append-only, hash-chain optional, retention **10+10 tahun**.

## DEC — Architecture (LOCKED)
- **DEC-019** Arsitektur: **Modular monolith** (bukan microservices). Service-style separation di backend Go monorepo. Shared DB.
- **DEC-020** API style: **REST** (`/api/v1/`), OpenAPI 3.0, JSON, snake_case DB / camelCase JSON.
- **DEC-021** Mutation endpoints: **Idempotency-Key wajib**.
- **DEC-022** Pagination: **cursor only** (no offset).
- **DEC-023** Multi-tenant: **single tenant Phase 1** (TUGURE), kolom `tenant_id` placeholder untuk Phase 2.

## DEC — Security (LOCKED)
- **DEC-024** Password hashing: **Argon2id** (m=64MB, t=3, p=4).
- **DEC-025** JWT: RSA-2048, access 15 min, refresh 8h, idle 15 min.
- **DEC-026** MFA mandatory: CEO, CFO, KOMITE, ALCO, Treasury Manager, Finance Controller.
- **DEC-027** Step-up MFA: hard-close periode, ECL parameter approve, klasifikasi approve, calc-run seal.
- **DEC-028** Encryption at rest: AES-256 TDE + column-level untuk PII (NPWP, no.rek, KTP).
- **DEC-029** TLS: 1.3 minimum.

## Cara reopen decision
1. Tulis proposal di `docs/decisions/RFC-{slug}.md`.
2. Tag stakeholder per RACI (lihat BRD §3).
3. Diskusi → konsensus → update `BLIPS_Decision_Log_v1.0.docx` dengan `superseded_by` di DEC lama + DEC baru.
4. **Tidak boleh** code change masuk yang melanggar DEC tanpa supersede formal.

## Bagaimana agent meng-enforce ini
- `tech-lead-orchestrator`: cek Decision Log sebelum bikin plan. Refuse silently melanggar.
- `business-analyst`: cek saat story conflicts dengan DEC, escalate.
- `ifrs9-compliance-reviewer`: BLOCK merge yang melanggar DEC-010..018.
- `security-engineer`: BLOCK merge yang melanggar DEC-024..029.
