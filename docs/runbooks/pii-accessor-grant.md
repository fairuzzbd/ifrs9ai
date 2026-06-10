# PostgreSQL Grant — `blips_pii_accessor` for sec.decrypt

## Purpose

Counterparty PII columns (`npwp_encrypted`, `nomor_rekening_encrypted`, `ktp_encrypted`) are stored
encrypted via `sec.encrypt()` / `sec.decrypt()` (migration `000003_pii_encrypt_functions.up.sql`,
DEC-028). The `sec.decrypt(TEXT)` function uses a role-gate — only callers that are a member of
`blips_pii_accessor` may execute it. Without this grant the endpoint
`GET /api/v1/master/counterparty/:id/pii` returns HTTP 500 with
`INSUFFICIENT_PRIVILEGE: sec.decrypt() requires blips_pii_accessor role`.

This runbook MUST be executed once per environment (dev / UAT / prod / DR), **after** migration
`000015_counterparty_rating_schema_fix.up.sql` has been applied (which re-declares the PII
column comments and documents the grant requirement), and **before** the `blips_app_user` DB role
is wired to JWT claims in Keycloak.

References:
- Security baseline: `.claude/memory/security-baseline.md` §Column-level encrypted PII fields
- DB conventions: `.claude/memory/db-conventions.md` §Audit log
- Migration that introduces the columns: `db/migrations/000003_pii_encrypt_functions.up.sql`
- Migration that documents the grant: `db/migrations/000015_counterparty_rating_schema_fix.up.sql`

---

## When to Run

| Trigger | Details |
|---|---|
| First deploy to any environment | Run once after `go run ./cmd/migrator up` completes |
| DR failover to new cluster | Re-run on the standby after promotion |
| Role recreation (e.g. after DB restore) | Re-run if `blips_pii_accessor` role was dropped |
| New app DB user added | Run Step 4 for the new user |

---

## Prerequisites

- PostgreSQL superuser access (role `postgres` or equivalent) for the target environment
- `psql` client installed, or access via `kubectl exec` / `docker exec` to the DB container
- Migration `000015_counterparty_rating_schema_fix.up.sql` already applied:
  ```sql
  SELECT version FROM schema_migrations WHERE version = '15'; -- must return a row
  ```

---

## Steps

### Step 1 — Connect as superuser

```bash
psql -h <DB_HOST> -p 5432 -U postgres -d blips
```

For Kubernetes environments:
```bash
kubectl exec -it <postgres-pod> -n blips -- psql -U postgres -d blips
```

### Step 2 — Verify `blips_pii_accessor` role exists

```sql
\du blips_pii_accessor
```

If the role is missing (should not happen after migration 0003 — the `DO $$...$$` block creates it
idempotently), create it:

```sql
CREATE ROLE blips_pii_accessor NOLOGIN;
```

### Step 3 — Verify `sec.decrypt` function signature

Confirm the actual signature before issuing the grant:

```sql
\df sec.decrypt
```

Expected output: `sec | decrypt | text | p_ciphertext text | ...`

The function signature is `sec.decrypt(TEXT)`. The function is defined in
`db/migrations/000003_pii_encrypt_functions.up.sql` and the grant below uses this exact signature.

### Step 4 — Grant EXECUTE on `sec.decrypt` and `sec.encrypt`

```sql
-- Grant decrypt (role-gated function — only members of blips_pii_accessor may call it)
GRANT EXECUTE ON FUNCTION sec.decrypt(TEXT) TO blips_pii_accessor;

-- Grant encrypt (application write path — blips_app_user encrypts on INSERT/UPDATE)
GRANT EXECUTE ON FUNCTION sec.encrypt(TEXT) TO blips_app_user;
```

> Note: `sec.decrypt` is itself a `SECURITY DEFINER` function that internally checks
> `pg_has_role(session_user, 'blips_pii_accessor', 'member')`. The `GRANT EXECUTE` above
> grants execution rights; membership in `blips_pii_accessor` is what the function checks
> at runtime. Both are required.

### Step 5 — Grant `blips_pii_accessor` membership to the application role

```sql
GRANT blips_pii_accessor TO blips_app_user;
```

Replace `blips_app_user` with the actual application DB user name for the target environment
(check `deploy/docker/docker-compose.*.yml` for the `POSTGRES_USER` or equivalent env var).

### Step 6 — Verify end-to-end decryption

```sql
-- Switch to the application role to simulate the service layer
SET ROLE blips_app_user;

-- Attempt a single decrypt call (safe — returns NULL for NULL input)
SELECT sec.decrypt(npwp_encrypted)
FROM mst.counterparty
WHERE npwp_encrypted IS NOT NULL
LIMIT 1;
```

Expected: returns a TEXT value (the plaintext NPWP) or `NULL` if no encrypted rows yet.
Any `INSUFFICIENT_PRIVILEGE` error means Step 5 was not applied or the role name is wrong.

Reset role after verification:
```sql
RESET ROLE;
```

### Step 7 — Record the grant in `sys.config_change`

Insert an audit row noting this runbook was executed:

```sql
INSERT INTO sys.config (
    config_key, config_value, config_type, sensitive, description, category
) VALUES (
    'RUNBOOK_PII_ACCESSOR_GRANT_APPLIED',
    current_timestamp::TEXT || ' | env=' || current_database() || ' | operator=' || current_user,
    'STRING',
    FALSE,
    'Records when pii-accessor-grant.md runbook was last applied. See docs/runbooks/pii-accessor-grant.md.',
    'SECURITY'
)
ON CONFLICT (config_key) DO UPDATE
    SET config_value = EXCLUDED.config_value,
        updated_at   = now();
```

---

## Rollback

If the grant must be revoked (e.g., security incident, role misconfiguration):

```sql
REVOKE blips_pii_accessor FROM blips_app_user;
REVOKE EXECUTE ON FUNCTION sec.decrypt(TEXT) FROM blips_pii_accessor;
```

After revocation, `GET /api/v1/master/counterparty/:id/pii` will return HTTP 500 until the grant
is re-applied. All other counterparty endpoints (list, get by ID, create, update) are unaffected —
they do not call `sec.decrypt`.

---

## CI/CD Wiring

[NEEDS-DEVOPS] The following integration work is pending:

1. **Ansible post-migration playbook** — add this grant as a task in
   `deploy/ansible/playbooks/db-init.yml` (file does not yet exist; create it):

   ```yaml
   - name: Grant blips_pii_accessor execute on sec.decrypt
     community.postgresql.postgresql_privs:
       db: blips
       role: blips_pii_accessor
       objs: decrypt(TEXT)
       schema: sec
       type: function
       privs: EXECUTE
       login_user: postgres

   - name: Grant blips_pii_accessor membership to app user
     community.postgresql.postgresql_membership:
       db: blips
       group: blips_pii_accessor
       target_roles: "{{ blips_app_user }}"
       login_user: postgres
   ```

2. **Post-deploy checklist** — add a checkbox to
   `deploy/runbooks/post-deploy-checklist.md` (file does not yet exist; create it):

   ```markdown
   - [ ] PII accessor grant applied (pii-accessor-grant.md) — verify with Step 6
   ```

3. **GitHub Actions** — add a step in `.github/workflows/ci.yml` for integration test
   environments to run this grant automatically after `go run ./cmd/migrator up`.

---

## Troubleshooting

| Symptom | Likely Cause | Fix |
|---|---|---|
| `INSUFFICIENT_PRIVILEGE: sec.decrypt()` | `blips_app_user` not member of `blips_pii_accessor` | Re-run Step 5 |
| `function sec.decrypt(text) does not exist` | Migration 0003 not applied | Run `go run ./cmd/migrator up` |
| `PII_ENCRYPTION_KEY not configured` | Placeholder key not replaced | Insert real key in `sys.config` per DEC-028 |
| `SET ROLE blips_app_user` fails | Role does not exist | Create app role per DB provisioning runbook |
