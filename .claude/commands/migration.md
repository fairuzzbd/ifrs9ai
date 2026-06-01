---
description: Buat DB migration baru via data-modeler (golang-migrate up/down)
argument-hint: <deskripsi perubahan schema, schema namespace, tabel target>
allowed-tools: Read, Grep, Glob, Write, Edit, Bash, Task
---

Panggil subagent `data-modeler` untuk menulis migration.

**Perubahan:** $ARGUMENTS

Wajib:
1. Baca current schema dulu: @BLIPS_init_schema.sql + `db/migrations/*.sql` terakhir.
2. Skill `migration-scaffold` boleh dipanggil untuk template (@.claude/skills/migration-scaffold/SKILL.md).
3. Tulis dry-run dulu (DDL + impact + downtime estimate), tunggu konfirmasi orchestrator/user.
4. Setelah approve, tulis file numbered: `db/migrations/{NNNN}_{slug}.up.sql` dan `.down.sql`.
5. Update ERD delta di `docs/erd/delta-{yyyymmdd}.md`.

Aturan keras yang harus dicek (@.claude/memory/db-conventions.md):
- Schema namespace: `mst|trx|ecl|sppi|doc|jrnl|aud|sec|sys` (9 namespace, ada `rpt` untuk MV)
- Audit fields wajib di semua tabel: `created_at, created_by, updated_at, updated_by, deleted_at, deleted_by, row_version`
- Money: `NUMERIC(20,4)` IDR, `NUMERIC(20,8)` FX, `NUMERIC(10,8)` PD/LGD/EIR
- No hard delete di `aud`, `jrnl`, `ecl`
- Partition by month untuk fact tables besar
