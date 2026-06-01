---
description: Build UI screen via uiux-designer + frontend-engineer-nextjs
argument-hint: <screen name, modul, persona aktor>
allowed-tools: Read, Grep, Glob, Write, Edit, Bash, Task
---

Dua-tahap UI:

**Tahap 1:** Panggil `uiux-designer` untuk wireframe + interaction spec.
- Input: $ARGUMENTS + user story link + OpenAPI yaml
- Output: `docs/ux/{module}/wireframe-{slug}.md` + `interaction-{slug}.md`
- Identifikasi BLIPS pattern: MakerReviewerApproverPanel, StagingBadge, ParameterFreeze, CalcRunReviewer, StagingTransitionTimeline, PeriodeBukuCloser, ApprovalWithSignature

**Tahap 2:** Panggil `frontend-engineer-nextjs` untuk implementasi.
- Generate API client + Zod schemas dari OpenAPI yaml (gunakan `openapi-typescript-codegen`)
- Build page/route App Router + Server Component default
- Forms pakai React Hook Form + Zod resolver
- Tables pakai TanStack Table + cursor pagination
- Test: Playwright e2e + Vitest component (happy + 1 validation-fail)

Wajib:
- WCAG 2.1 AA, keyboard reachable, aria-describedby untuk error
- Bahasa Indonesia label, English secondary (next-intl)
- IDR format `Intl.NumberFormat('id-ID', { style:'currency', currency:'IDR' })`
- Asia/Jakarta timezone (date-fns-tz)
