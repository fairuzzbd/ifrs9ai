---
description: Trigger security review via security-engineer (auth, RBAC, PII, audit, encryption)
argument-hint: <endpoint / file / module yang di-review>
allowed-tools: Read, Grep, Glob, Task
---

Panggil subagent `security-engineer`.

**Scope:** $ARGUMENTS

Reviewer akan jalankan checklist berikut (@.claude/memory/security-baseline.md):
1. Auth: permission check `{entity}.{action}`? Bukan role string comparison?
2. Tenancy: `tenant_id` scoping enforced?
3. Input: SSRF / SQLi / path traversal / XXE / deserialization?
4. Output: PII redaction di response + log?
5. Audit: mutation tulis ke `aud.audit_log` di same tx (atomik)?
6. Idempotency-Key dicek?
7. SoD: `maker_id ≠ reviewer_id ≠ approver_id` di service layer?
8. Rate limit + abuse signal?
9. Error message tidak leak internal state?
10. Signature/attestation untuk approval action?

Output: `PASS | FIX-REQUIRED | BLOCK` + remediation spesifik.

**BLOCKING** veto: plaintext PII, hardcoded credentials, audit-log write di luar business tx, role string in code.
