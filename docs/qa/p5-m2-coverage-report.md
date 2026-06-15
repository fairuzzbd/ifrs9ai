# P5-M2 Jurnal Engine — QA Coverage Report

**Generated**: 2026-06-15
**Author**: qa-engineer
**Branch**: feature/phase-5-m2-qa-suite
**Story Set**: P5-M2 (S1–S6), 25 Acceptance Criteria

---

## Per-AC Coverage Manifest

| # | Story | Acceptance Criteria | E2E Scenario | UAT TC | Perf Bench | Notes |
|---|---|---|---|---|---|---|
| 1 | S1 | 4-eyes operational PENEMPATAN: DRAFT → APPROVED_ACTIVE | P5-M2-A | TC-001 | — | Full audit chain verified |
| 2 | S1 | aktif_flag=true setelah approve operational | P5-M2-A | TC-001 | — | |
| 3 | S1 | 6-eyes regulated ECL_PEMBENTUKAN: approve_1 → PENDING_APPROVAL_2 | P5-M2-B | TC-002 | — | |
| 4 | S1 | 6-eyes regulated: approve_2 → APPROVED_ACTIVE + sig hash + signed_at | P5-M2-B | TC-002 | — | SHA-256 hash verified |
| 5 | S1 | SoD violation: approver_2 == approver_1 → 403 JURNAL_SOD_VIOLATION | P5-M2-C | TC-003 | — | Audit SoD attempt written |
| 6 | S1 | SoD violation audit written even on rejection | P5-M2-C | TC-003 | — | |
| 7 | S1 | Step-up MFA stale on approve_2 → 403 JURNAL_STEP_UP_REQUIRED | P5-M2-D | TC-004 | — | DEC-027 + OQ-M2-1c |
| 8 | S1 | Fresh step-up on approve_2 succeeds | P5-M2-D | TC-004 | — | |
| 9 | S1 | Balance validation: 0 KREDIT rows → VALIDATION_FAILED | P5-M2-L (partial) | TC-005-A (negative) | — | |
| 10 | S2 | Resolver PENEMPATAN+AC → balanced JurnalLines, sum D=K | P5-M2-E | TC-005-A | BenchmarkResolverPreview | 5Bn IDR verified |
| 11 | S2 | Resolver MTM_FVOCI + FVTPL → JURNAL_KLASIFIKASI_NOT_ELIGIBLE | P5-M2-F | TC-005-B | — | |
| 12 | S2 | Resolver unknown event_code → JURNAL_EVENT_NOT_MAPPED | P5-M2-G | TC-005-C | — | |
| 13 | S2 | Resolver imbalanced template → JURNAL_BALANCE_INVARIANT, no lines | P5-M2-L | — | — | Multiplier 0.10/0.08 |
| 14 | S3 | penempatan:approved → jrnl.header + 2 detail rows, D=K | P5-M2-H | TC-006 | BenchmarkResolveAndPost | |
| 15 | S3 | JURNAL.POST audit in-transaction with INSERT | P5-M2-H | TC-006 | — | DEC-018 |
| 16 | S3 | jrnl.header.idempotency_key = SHA256(source_event_id||"::"||event_code) | P5-M2-H | TC-006 | BenchmarkIdempotencyKeyComputation | |
| 17 | S3 | Idempotency replay → existing header, no duplicate INSERT | P5-M2-M | TC-006 step 4 | — | DEC-021 |
| 18 | S3 | Periode HARD_CLOSED → DLQ, error_code=JURNAL_PERIODE_HARD_CLOSED | P5-M2-I | TC-007 | — | |
| 19 | S3 | Periode HARD_CLOSED error_category=DOMAIN, SkipRetry=true | P5-M2-I | TC-007 | — | OQ-M2-3a |
| 20 | S3 | Domain err (event_not_mapped) → DLQ immediate, SkipRetry=true | P5-M2-N | — | BenchmarkAsynqSubscriberProcessing | |
| 21 | S3 | Infra err → 3x retry then DLQ, retry_count=3, error_category=INFRA | P5-M2-O | — | — | OQ-M2-3a |
| 22 | S6 | DLQ replay sukses → REPLAYED_OK + replayed_jurnal_id + audit | P5-M2-J | TC-008 | — | |
| 23 | S6 | DLQ replay audit written BEFORE Asynq enqueue | P5-M2-J | TC-008 | — | security-baseline §DLQ |
| 24 | S6 | DLQ discard → ABANDONED + reason ≥30 chars + audit | P5-M2-K | TC-009 | — | |
| 25 | S6 | DLQ discard reason < 30 chars → VALIDATION_FAILED | P5-M2-K | TC-009 (Path A) | — | |

---

## Scenario Count

| Layer | Count |
|---|---|
| E2E scenarios (P5-M2-A through P5-M2-O) | 15 |
| UAT test cases (TC-001 through TC-009) | 9 |
| Perf benchmarks | 6 |
| AC coverage | 25/25 (100%) |

## Regression Suite Priority Coverage

| Priority | Requirement | Covered By |
|---|---|---|
| 1 | Klasifikasi PSAK71 SPPI×BM matrix | P5-M2-E, P5-M2-F (klasifikasi routing) |
| 5 | Periode buku soft→hard close cannot be reversed | P5-M2-I, TC-007 |
| 6 | SoD enforcement at API level | P5-M2-C, P5-M2-D, TC-003, TC-004 |
| 7 | Audit trail tamper-evidence hash chain | TestE2E_P5M2_AuditHashChainIntegrity |
| 8 | Idempotency — replay returns original, no duplicate | P5-M2-M, P5-M2-H |
