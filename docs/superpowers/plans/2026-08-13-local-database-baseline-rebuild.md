# Local Database Baseline Rebuild Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the pre-launch Atlas/reconciliation database chain with one reproducible local baseline, one minimal seed, and short forward-only migrations without weakening runtime data invariants.

**Architecture:** `database/schema.sql` becomes the only full schema source and `database/seed.sql` the only non-secret initialization data source. A small PowerShell entry owns `init`, `reset`, `migrate`, and `check`; migrations after the baseline are recorded in one `schema_migrations` table, while MySQL remains authoritative and Redis/Qdrant are cleared on reset.

**Tech Stack:** MySQL 8.4, PowerShell 7, Docker Compose, Go 1.26, Redis 8, Qdrant 1.18

---

## File Map

**Create:**

- `database/schema.sql`: complete current runtime schema without development rows, secrets, `DEFINER`, Atlas, or reconciliation tables.
- `database/seed.sql`: deterministic permissions, roles, role grants, Admin auth policy, required settings, six registered cron tasks, and the official `admin_user_count` AI tool.
- `database/migrations/README.md`: forward-only migration naming and transaction rules from baseline `202608130001` onward.
- `database/baseline.json`: reviewed source commit, source Atlas version, dump SHA-256, schema SHA-256, table count, and seed row counts; no credentials.
- `scripts/database.ps1`: the sole local `init`, `reset`, `migrate`, and `check` entry.
- `scripts/tests/database-baseline.tests.ps1`: fast command and destructive-scope contract tests.

**Modify:**

- `cmd/admin-db/main.go` and `cmd/admin-db/main_test.go`: add explicit `create-admin` using runtime password input and a transaction; remove evolution-only commands after callers are retired.
- `database/README.md`, `README.md`, `docs/architecture.md`, `AGENTS.md`: describe only the new database workflow.
- `.gitattributes`: keep the three active SQL surfaces on stable LF bytes.
- `scripts/release/*.ps1`, `scripts/verify-database.ps1`, `scripts/verify-durable-work.ps1`, and focused script tests: consume `database.ps1 check` and baseline hashes instead of Atlas/reconciliation artifacts.
- `internal/architecture/*schema*_test.go`, `internal/architecture/database_*_test.go`, and direct schema contract tests: assert semantic facts in `schema.sql`; delete tests whose only subject is an old file path or migration string.

**Retire after cutover:**

- `database/legacy-migrations/`, old contents of `database/migrations/`, `database/reconciliation/`, `database/schema/admin.hcl`, `database/seeds/`.
- `scripts/database/atlas*.ps1`, `capture-baseline.ps1`, `check-drift.ps1`, `contract-admin-only.ps1`, `establish-baseline.ps1`, `new-recovery-artifact.ps1`, `reconcile.ps1`, and evolution-only evidence scripts.
- Evolution-only parts of `internal/databaseevolution`; retain only code with a current runtime or baseline-check caller.

## Frozen Seed Whitelist

`seed.sql` contains only these reviewed facts:

| Table | Rows | Rule |
|---|---:|---|
| `permissions` | current active Admin tree | Generated from the reviewed permission seed; no soft-deleted history |
| `roles` | 2 | Stable ordinary-user and super-admin roles |
| `role_permissions` | active grants only | Ordinary role gets its reviewed subset; super-admin gets every active non-directory permission plus required root directories |
| `auth_platforms` | 1 | Enabled `admin` platform policy |
| `system_settings` | 3 | Required CAPTCHA TTL, slide padding, and upload-token TTL only |
| `cron_task` | 6 | Exact names in `crontask.NewDefaultRegistry()` only |
| `ai_tools` | 1 | Official `admin_user_count` tool only |

Official model definitions remain in `internal/module/ai/officialmodel/catalog/official_models_v1.json`; they are not duplicated into SQL. Provider models, providers, agents, agent-tool bindings, upload credentials, users, conversations, runs, context data, documents, logs, payments, queues, and vectors are excluded.

### Task 1: Freeze the Recoverable Source

**Files:**
- Create: `database/baseline.json`

- [ ] Verify `git status --short` contains only the approved plan and record backend/frontend full commits.
- [ ] Create annotated tags `pre-database-baseline-20260813` in both repositories without moving branches.
- [ ] Verify the existing full dump at `C:\Users\20931\AppData\Local\Temp\admin-db-baseline\admin-current-full-20260813-092521.sql` has SHA-256 `c2b73e639892c3c1cd274443758738ce153d7c3eef7b8fdad67545453a018e50` and size `15936795`; stop if either differs.
- [ ] Query the live source and require Atlas version `202608080103`, zero pending migrations, and 79 base tables before extracting anything.
- [ ] Write `database/baseline.json` with source evidence and placeholder-free zeroed target fields, then update target hashes/counts only after Task 4 verification.
- [ ] Commit: `chore(database): freeze pre-baseline recovery point`.

### Task 2: Generate and Review the Canonical Schema

**Files:**
- Create: `database/schema.sql`
- Test: `internal/architecture/database_baseline_test.go`

- [ ] Add a focused failing test that requires `database/schema.sql`, exactly one `schema_migrations` table, InnoDB/utf8mb4 tables, and rejects `atlas_schema_revisions`, `schema_reconciliation_runs`, `DEFINER=`, `AUTO_INCREMENT=<n>`, stored credentials, and data `INSERT` statements.
- [ ] Run `go test ./internal/architecture -run 'TestDatabaseBaseline' -count=1`; expect failure because `schema.sql` is absent.
- [ ] Extract structure from the live latest MySQL database with `mysqldump --no-data --skip-comments --skip-dump-date --skip-add-locks --skip-disable-keys --set-gtid-purged=OFF`.
- [ ] Remove only `atlas_schema_revisions`, `schema_reconciliation_runs`, and `ai_billing_migration_metadata` after confirming no production Go caller reads them; keep every business table, foreign key, unique key, check, decimal type, and runtime index.
- [ ] Normalize volatile auto-increment counters and environment clauses; append `schema_migrations(version varchar(32) primary key, checksum_sha256 char(64), applied_at datetime)` plus baseline row `202608130001` only in `seed.sql`.
- [ ] Review the schema for 76 expected business tables plus `schema_migrations`, no `DEFINER`, no secrets, no data, and no lost foreign keys/checks/indexes.
- [ ] Run the focused test and require PASS.
- [ ] Commit: `feat(database): add canonical schema baseline`.

### Task 3: Build the Minimal Deterministic Seed

**Files:**
- Create: `database/seed.sql`
- Modify: `internal/architecture/database_baseline_test.go`

- [ ] Add failing assertions for the exact whitelist above and for absence of every excluded table name in `INSERT INTO` statements.
- [ ] Generate active permission rows from `database/seeds/admin_permissions.sql`, remove its historical guards, and preserve stable IDs and parent relations.
- [ ] Write explicit stable rows for two roles, active role grants, one Admin auth policy, three required settings, the six registry-backed cron tasks, and `admin_user_count`; use UTC-neutral defaults instead of copied development timestamps.
- [ ] Insert baseline migration `202608130001` with the SHA-256 of `database/schema.sql`; do not put a user or password hash in SQL.
- [ ] Wrap the seed in one transaction and make duplicate application fail explicitly instead of silently updating rows.
- [ ] Run `go test ./internal/architecture -run 'TestDatabaseBaseline' -count=1`; require PASS.
- [ ] Commit: `feat(database): add minimal initialization seed`.

### Task 4: Prove Schema and Seed in a Disposable Database

**Files:**
- Modify: `database/baseline.json`

- [ ] Create a random schema named `admin_baseline_<12 lowercase hex>` inside `admin-state-mysql-1`; validate the resolved name before any create/drop.
- [ ] Apply `database/schema.sql` and `database/seed.sql` with `--default-character-set=utf8mb4`; require both commands to exit zero.
- [ ] Run structural checks: table count, foreign keys, check constraints, unique indexes, decimal money columns, exact migration ledger row, exact seed counts, and zero rows in volatile business tables.
- [ ] Attempt a second seed application and require a deterministic non-zero duplicate-initialization error.
- [ ] Point a temporary `MYSQL_DSN` at the disposable schema and run only focused repository/readiness tests that do not start `admin-dev` or call external providers.
- [ ] Update `database/baseline.json` with final schema SHA-256, verified table count, constraint counts, and seed counts.
- [ ] Drop only the exact generated disposable schema in `finally` after re-validating its prefix and 12-hex suffix.
- [ ] Commit: `test(database): verify empty baseline rebuild`.

### Task 5: Add One Small Database Command Surface

**Files:**
- Create: `scripts/database.ps1`
- Create: `scripts/tests/database-baseline.tests.ps1`
- Modify: `cmd/admin-db/main.go`
- Modify: `cmd/admin-db/main_test.go`

- [ ] Write failing PowerShell contract tests for `init`, `reset`, `migrate`, and `check`, including exact repository-root resolution, container/schema allowlists, app-container refusal, and reset confirmation.
- [ ] Write failing Go tests for `admin-db create-admin --username <name> --role-id 2`; password must come from a hidden terminal prompt or `ADMIN_INITIAL_PASSWORD`, never an argument, log, or SQL file.
- [ ] Implement `create-admin` with strict username/password validation, bcrypt default cost, active-role check, duplicate-user rejection, and one transaction creating the user plus required owned rows. Print only the created user ID and username.
- [ ] Implement `database.ps1 init`: require an empty `admin` schema, apply schema/seed, then optionally call `create-admin` when `-CreateAdmin` is supplied.
- [ ] Implement `database.ps1 migrate`: sort `database/migrations/*.sql`, verify SHA-256 against the immutable ledger, apply unseen files one at a time, and fail on changed previously applied bytes. No automatic migration occurs in API or Worker startup.
- [ ] Implement `database.ps1 check`: validate MySQL connectivity, schema hash/semantic invariants, migration checksums, and seed ownership facts without modifying state.
- [ ] Implement `database.ps1 reset`: require exact local Docker state containers, explicit `-ConfirmReset admin`, stop neither `admin-dev` nor app processes automatically, refuse while API/Worker containers are running, replace the `admin` schema, flush only the dedicated Redis database, and delete only Qdrant collections/aliases whose configured prefix belongs to this Admin project.
- [ ] Run `pwsh -NoProfile -File scripts/tests/database-baseline.tests.ps1` and `go test ./cmd/admin-db -count=1`; require PASS.
- [ ] Commit: `feat(database): add local database lifecycle command`.

### Task 6: Cut Consumers Over Before Deleting History

**Files:**
- Modify: `.gitattributes`
- Modify: `database/README.md`
- Modify: `README.md`
- Modify: `docs/architecture.md`
- Modify: `AGENTS.md`
- Modify: affected `scripts/release/*.ps1`, `scripts/verify-*.ps1`, and focused tests
- Modify/Delete: affected `internal/architecture/*_test.go`

- [ ] Replace active Atlas/reconciliation commands and architecture statements with `scripts/database.ps1 init|reset|migrate|check`.
- [ ] Replace release manifest fields `atlas_sum_sha256` and reconciliation evidence with `baseline_schema_sha256`, `baseline_seed_sha256`, and ordered migration checksums.
- [ ] Rewrite semantic schema tests to inspect `database/schema.sql`; delete tests that only demand old paths, migration wording, reconciliation counts, or HCL formatting.
- [ ] Keep direct behavior tests for payment integrity, AI terminal persistence, RBAC relations, context authority, task recovery, and unique/check constraints.
- [ ] Run narrow searches and require no active reference outside historical docs to `admin.hcl`, `atlas.sum`, `atlas_schema_revisions`, `schema_reconciliation_runs`, `database/reconciliation`, or `database/legacy-migrations`.
- [ ] Run only focused packages/scripts touched by this task; do not run `go test ./...`, Playwright, full frontend build/typecheck, or the old long database/release gates.
- [ ] Commit: `refactor(database): switch governance to baseline workflow`.

### Task 7: Retire the Old Governance Chain

**Files:**
- Delete: old database paths and evolution-only scripts listed in File Map
- Delete/Reduce: evolution-only `internal/databaseevolution` code and tests with no remaining caller
- Create: `database/migrations/README.md`

- [ ] Verify the recovery tags and external dump hash again before deletion.
- [ ] Delete the old Atlas, HCL, legacy migration, reconciliation, recovery-evidence, and unused script surfaces only after Task 6 has no active caller.
- [ ] Recreate an empty `database/migrations/` containing only `README.md`; the first post-baseline migration version must be greater than `202608130001`.
- [ ] Run `rg` for retired symbols and classify every remaining match as either an intentional historical document or a defect; fix all defects.
- [ ] Run the focused database baseline, `cmd/admin-db`, directly affected architecture, and PowerShell tests; require PASS.
- [ ] Commit: `refactor(database): retire atlas reconciliation chain`.

### Task 8: Rebuild the Authorized Local State

**Files:**
- No source changes unless a focused verification exposes a defect.

- [ ] Require the user-owned `admin-dev` process to be stopped; do not start or stop it from this plan.
- [ ] Run `scripts/database.ps1 reset -ConfirmReset admin -CreateAdmin` and enter a new local administrator password outside logs.
- [ ] Verify MySQL contains only baseline/seed facts plus the new administrator and its owned rows.
- [ ] Verify dedicated Redis has no pre-reset session, queue, CAPTCHA, realtime, or cache keys.
- [ ] Verify project Qdrant collections/aliases are absent; they will be rebuilt from future MySQL document facts.
- [ ] Ask the user to start `admin-dev`, then check `/health`, `/ready`, login, `users/me`, permissions, official models, AI tools, and empty AI/context/run screens.
- [ ] Do not call paid AI providers or payment gateways during this database acceptance.
- [ ] Commit only defect fixes, each scoped to the discovered invariant.

## Completion Check

- [ ] A fresh local database is reproducible from `schema.sql + seed.sql` without understanding project history.
- [ ] `schema.sql`, `seed.sql`, and new forward migrations each have one owner and one command surface.
- [ ] No secret, provider configuration, user password, local chat, document, payment, upload, log, queue, or Qdrant state is part of initialization.
- [ ] Old migration/reconciliation mechanisms have no active code, script, test, release, or documentation caller.
- [ ] Existing API/database semantics are unchanged; only the pre-launch initialization and evolution workflow is replaced.
- [ ] Recovery remains possible from the Git tags and verified external full dump.
