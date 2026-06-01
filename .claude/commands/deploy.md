---
description: Deploy / update infra via devops-engineer (UAT / prod / DR)
argument-hint: <environment (uat/prod/dr), service, jenis perubahan>
allowed-tools: Read, Grep, Glob, Write, Edit, Bash, Task
---

Panggil subagent `devops-engineer`.

**Task:** $ARGUMENTS

Wajib:
1. Confirm scope: UAT only vs prod-affecting. Prod → butuh release tag + ROLE-IT-ADMIN approver.
2. IaC change as PR ke `deploy/`:
   - Terraform `plan` dulu sebelum `apply`
   - Ansible `--check` dulu
   - Helm: `helm diff` sebelum upgrade
3. Update Grafana dashboard / alert jika observability surface berubah.
4. Update runbook di `docs/runbooks/` jika perilaku operasional berubah.
5. **Destructive ops** (drop schema, delete bucket, downsize PG) → REQUIRE orchestrator + `security-engineer` sign-off.

CI/CD stages reference: lint → unit → integration → security-scan → build (cosign-signed) → deploy-uat → e2e → deploy-prod (manual gate).

Refuse: manual `kubectl edit` in prod, mutable Docker tags (`:latest`), secrets dalam YAML committed.
