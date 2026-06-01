---
description: Bikin user story baru via business-analyst (Actor / Trigger / AC Gherkin)
argument-hint: <judul story, modul, persona aktor>
allowed-tools: Read, Grep, Glob, Write, Edit, Task
---

Panggil subagent `business-analyst` untuk menulis user story.

**Topik:** $ARGUMENTS

Wajibkan output mengandung:
- Actor (gunakan RBAC role dari @.claude/memory/personas.md)
- Trigger + Pre-conditions + Steps + Post-conditions
- Acceptance Criteria dalam format Gherkin (Given/When/Then), minimum 3 skenario (happy + 2 edge)
- Linked FSD section + linked RACI dari BRD
- Workflow rule: 4-eyes (rutin) atau 6-eyes (klasifikasi PSAK 71 / parameter master)

Tulis ke `docs/stories/{modul}-{slug}.md`. Jika menyentuh SPPI/BM/ECL/EIR/klasifikasi/reklasifikasi → tag `review-required: ifrs9-compliance-reviewer`.

Reference: @.claude/memory/glossary.md
