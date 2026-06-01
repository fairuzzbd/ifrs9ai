---
description: Build / fix external system integration (Pefindo, IBPA, KSEI, BEI, BI JISDOR, GL, SAML, SMTP)
argument-hint: <sistem eksternal + arah inbound/outbound + deskripsi singkat>
allowed-tools: Read, Grep, Glob, Write, Edit, Bash, Task
---

Panggil subagent `integration-engineer`.

**Task:** $ARGUMENTS

Wajib:
1. Baca @FSD-BLIPS-MASTER-v1.1.docx §5 untuk integration spec.
2. Inbound feed: raw payload disimpan ke MinIO **sebelum** parsing (`raw/{system}/{yyyy/mm/dd}/{filename}`). Parsing harus replayable.
3. Schema validation → business validation → DLQ untuk reject row (`sys.dlq_row`).
4. Asynq retry exponential backoff, max 5. Lebih dari itu → alert ROLE-IT-ADMIN.
5. Idempotency-Key = `sha256(system + business_key + period)`.
6. File upload: antivirus scan dulu, quarantine bucket untuk dirty.
7. Tulis runbook di `docs/runbooks/{system}.md`: gejala kegagalan + langkah remediasi.

Test wajib: mock external pakai `httptest`/fakes. Never call real external in CI.
