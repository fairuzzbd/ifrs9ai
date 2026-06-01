---
description: Generate OpenAPI fragment + state machine via system-analyst dari user story yang sudah ada
argument-hint: <path user story atau ringkasan, endpoint yang dibutuhkan>
allowed-tools: Read, Grep, Glob, Write, Edit, Task
---

Panggil subagent `system-analyst`.

**Input:** $ARGUMENTS

Output yang diharapkan, dalam urutan:
1. OpenAPI 3.0 fragment di `api/openapi/{modul}.yaml` (atau update existing).
2. State machine dalam Mermaid markdown jika workflow-bearing (Maker-Reviewer-Approver).
3. Validation rules table: field → rule → error code → message-id.
4. Hand-off note: agent berikutnya yang dipanggil.

Wajib ikut konvensi @.claude/memory/api-conventions.md:
- `METHOD /api/v1/{resource}`, snake_case DB / camelCase JSON
- `Idempotency-Key` header pada semua mutation
- Error envelope `{ code, message, details[], traceId }`
- Cursor pagination
- JWT scopes mapping ke permission `{entity}.{action}`

Jika butuh schema change → STOP dan panggil `data-modeler` dulu.
