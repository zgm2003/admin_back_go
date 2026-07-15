# Admin Foundation and Database Evolution Design

**Status:** Approved in conversation on 2026-07-15; written for final file review.

**Goal:** Make builds, configuration, schema state, migrations, data invariants, and performance evidence reproducible before application refactoring begins.

## Scope

This design owns:

- local and deployment environment preparation;
- Go dependency integrity;
- database fingerprinting and reconciliation;
- versioned future migrations;
- App/Canvas data retirement required by the Admin-only decision;
- query-plan/index changes;
- CI and deployment gates that prove these results.

It does not redesign domain behavior or introduce tenant data.

## Environment ownership

The supported backend developer entry remains Docker-first. The implementation creates the ignored file:

```text
deploy/docker-first/admin-go.env
```

from `admin-go.env.example`, then fills it with environment-specific values. For the current workstation, MySQL listens on port 3306; operator-supplied credentials are written only to the ignored environment file and never copied into committed documentation, command history, or logs. The implementation must discover and validate the Redis address before declaring the environment ready. It must generate a non-placeholder APP_SECRET of at least 64 random characters and retain it across restarts.

Environment validation is strict:

- malformed integer, duration, boolean, URL, DSN, or enum values return an actionable startup error;
- required settings vary by process capability;
- production rejects loopback dependencies, placeholder origins, unsafe secrets, local realtime topology, and missing payment certificate paths when payment is enabled;
- configuration diagnostics may expose key names and validation reasons but never secret values.

## Dependency integrity

The incorrect `asynqmon` content checksum is replaced only after validating the module version against `sum.golang.org`. Disabling `GOSUMDB`, adding `GONOSUMDB`, using `GOINSECURE`, or accepting an unverified archive is forbidden.

The clean-cache gate is:

```powershell
$verifyRoot = Join-Path $env:TEMP ("admin-go-verify-" + [guid]::NewGuid())
$emptyCache = Join-Path $verifyRoot "modcache"
$binDir = Join-Path $verifyRoot "bin"
New-Item -ItemType Directory -Path $emptyCache, $binDir | Out-Null
$previousModCache = $env:GOMODCACHE
try {
  $env:GOMODCACHE = $emptyCache
  go mod download
  go mod verify
  go test ./...
  go build -o (Join-Path $binDir "admin-api.exe") ./cmd/admin-api
  go build -o (Join-Path $binDir "admin-worker.exe") ./cmd/admin-worker
} finally {
  $env:GOMODCACHE = $previousModCache
}
```

CI runs the equivalent in a clean job.

## Live schema fingerprint

Before DDL, the Database Evolution Module records a machine-readable fingerprint containing:

- MySQL server version and SQL mode;
- tables, columns, types, nullability, defaults, generated expressions, and comments;
- primary, unique, and secondary indexes including prefix lengths and ordering;
- foreign keys, check constraints, triggers, routines, and events;
- approximate and exact row counts for migration-sensitive tables;
- the Git commit and fingerprint SHA-256.

The fingerprint excludes volatile auto-increment values and statistics so identical schemas produce identical hashes.

## Recovery artifact

Before destructive DDL, automation creates a timestamped recovery dump outside both repositories:

```powershell
mysqldump --defaults-extra-file=<runtime-secret-file> `
  --single-transaction --quick --routines --triggers --events `
  --default-character-set=utf8mb4 `
  --result-file=<runtime-backup-path> admin
```

The restricted runtime secret file and dump both live outside the repositories; the secret file is deleted immediately after use. `--result-file` is mandatory on Windows so PowerShell cannot transcode the SQL stream. The workflow verifies that the dump is non-empty, contains the expected schema header and critical table definitions, records its SHA-256, restores it into a disposable database, and compares critical schema/row-count invariants. The user is not required to manage this artifact. A destructive stage cannot begin when recovery-artifact or restore verification fails.

## Current incompatibilities to reconcile

The reconciliation explicitly addresses all verified differences:

| Area | Current live state | Required Admin target |
| --- | --- | --- |
| Export | `export_tasks` lacks `platform`, `kind`, `object_key` | columns, backfill, indexes, worker behavior |
| AI Run | `ai_runs` lacks `platform`, `input_snapshot` | unified run schema and indexes |
| AI Image | old `ai_image_assets` / `ai_image_task_assets`; no `ai_image_files`; task lacks `platform` | generic AI task/file schema or deletion if capability is not retained |
| AI Text/Video | `ai_text_tasks` and video task table absent | generic retained capability schema or explicit capability deletion |
| AI Assets | `ai_assets` absent | generic retained capability schema or explicit capability deletion |
| Wallet | `user_wallets.total_consume_cents` absent | column, ledger reconciliation, constraints |
| Payment callback | `payment_callback_events` absent | callback audit and idempotency schema |
| Verification channels | mail/SMS TTL columns absent | channel-owned TTL columns and backfill |
| Retired product lines | `users_quick_entry` and App/Canvas metadata remain | removed after Admin-only verification |

Historical migrations are audit evidence, not an executable plan. In particular, lexical filename order is invalid for the June AI migrations: unified run fields must exist before image convergence reads them, and source fields are removed only after both complete.

Retained `platform` columns are validated provenance/routing metadata, not a policy switch inside Capability Modules. New active rows in this phase use only `admin`; reconciliation must explicitly account for every legacy `app` or `canvas` value before contract DDL.

## Reconciliation stages

### Stage 1 — Expand

- Add missing tables, columns, unique constraints, and compatibility indexes without dropping readable data.
- Create reconciliation audit metadata containing stage name, SQL SHA-256, start/end timestamps, executor, and result.
- Make each operation conditional on the observed fingerprint rather than catching and ignoring arbitrary SQL errors.

### Stage 2 — Backfill

- Backfill platform and ownership fields from explicit source facts.
- Migrate AI image task/file references and prove source-to-target counts.
- Recompute wallet totals from the immutable ledger and stop on discrepancies.
- Populate export kinds/platforms/object keys without guessing unknown values.
- Backfill channel TTL values from the currently documented default only when the source is absent.

### Stage 3 — Verify

- Compare source and target row counts and deterministic content hashes.
- Run orphan checks for RBAC, payment, wallet, AI message/run/file, notification, and export relationships.
- Verify the single known legacy AI image object through the configured COS read adapter. A valid existing object is referenced; it is not re-uploaded.
- Run focused Admin smoke against the expanded schema.

### Stage 4 — Contract

- Remove old App/Canvas metadata, obsolete tables, compatibility columns, redundant indexes, and dead constraints.
- Execute one destructive group at a time with an invariant query before and after each group.
- Regenerate and compare the final fingerprint.

## Future migration system

Atlas OSS CLI is the migration adapter for future versioned SQL because it provides migration-directory checksums, linting, status, diff inspection, baseline support, and MySQL compatibility. CI and deployment use a pinned Atlas container digest; application startup does not silently apply migrations.

The repository layout becomes:

```text
database/
  schema/                 # canonical target schema
  migrations/             # new checksummed Atlas migration directory
  legacy-migrations/      # preserved historical SQL, never auto-applied
  reconciliation/         # imported-live-db staged scripts and invariant queries
```

After reconciliation proves target equality:

1. initialize the Atlas revision table at the target baseline;
2. calculate and commit the migration directory checksum;
3. require `atlas migrate lint`, `atlas migrate validate`, and drift comparison in CI;
4. acquire a MySQL deployment lock before apply;
5. fail deployment when another migration is dirty or in progress.

An empty database and a reconciled imported database must converge to the same fingerprint.

## Query and index method

Indexes are not accepted by intuition alone. Each proposed change must include:

1. the exact repository query and expected result ordering;
2. representative bind values and row-count distribution;
3. `EXPLAIN ANALYZE` before the change;
4. the proposed index and its write/storage cost;
5. `EXPLAIN ANALYZE` after the change;
6. a regression query or benchmark.

Initial candidates requiring this proof are:

- `user_sessions(user_id, platform, is_del, revoked_at, refresh_expires_at, id)`;
- `ai_conversations(user_id, is_del, last_message_at DESC, id DESC)` and the agent-filtered variant;
- `ai_runs(status, started_at, id)` and `ai_runs(platform, created_at DESC, id DESC)`;
- `payment_orders(provider, status, is_del, updated_at, id)`;
- `payment_orders(provider, status, is_del, expired_at, id)`;
- `payment_recharges(is_del, credited_at, id)`;
- `notifications(user_id, is_del, is_read, platform)`;
- `notification_task(status, is_del, send_at, id)`;
- `export_tasks(user_id, platform, is_del, id)`;
- `cron_task_log(task_id, is_del, created_at, id)`.

The shorter `payment_configs(provider,status)` index is removed only after performance-schema evidence proves the longer `(provider,status,sort)` index substitutes for every active query.

## Query implementation changes

- AI conversation cursor becomes the tuple `(last_message_at, id)` matching its sort; `id < before_id` alone is forbidden.
- AI Run knowledge hits load in one `IN` query instead of one query per retrieval.
- Knowledge retrieval must not score only the oldest fixed number of chunks; the retrieval interface must make candidate selection explicit and measurable.
- Chunk/hit writes use bounded transaction batches.
- High-growth lists use keyset pagination; exact counts move to explicit count operations or cached aggregates when UI behavior permits.
- Export expiration cleanup moves out of list reads into a Worker command.
- Physical AI task deletion must delete database dependents transactionally and enqueue object cleanup, or use a verified soft-delete policy.

## Concurrency and idempotency requirements

- Refresh rotation uses row locking or compare-and-swap on the previous refresh hash.
- Notification delivery has a durable unique identity such as `(task_id,user_id)` and a claim lease.
- Export execution has pending/running/succeeded/failed states, a claim lease, and idempotent artifact publication.
- AI provider attempts have a non-null idempotency key for both chat and non-chat runs.
- Payment finalization continues to use ledger idempotency and gains query indexes proven by Worker scan plans.

## Database verification queries

The implementation includes executable invariant files covering:

- presence and exact definition of required columns/tables;
- absence of retired App/Canvas schema after contract stage;
- RBAC, payment, wallet, AI, notification, and export orphans;
- wallet balance versus signed ledger sum;
- AI source-to-target migration counts;
- duplicate idempotency keys;
- unknown platform values;
- active rows with missing ownership or required identifiers.

Every invariant returns zero violating rows. The migration task fails on any non-zero result.

## Performance operations

The local server currently has `slow_query_log=ON`, `long_query_time=10s`, `performance_schema=ON`, and a 128 MiB buffer pool. The implementation records digest baselines, lowers the development/staging slow threshold to one second, and sizes production memory from workload evidence. It does not copy workstation memory settings into production.

## Completion criteria

- Clean dependency verification and full backend tests/build pass.
- Imported and empty databases converge to an identical committed fingerprint.
- No application query references a missing table or column.
- Reconciliation and future migrations have checksums, status, locks, and drift checks.
- All invariants pass before and after the contract stage.
- Each index change has before/after `EXPLAIN ANALYZE` evidence.
- Existing referenced COS objects remain reachable or are reported with exact object keys before contract DDL.
- No secret or database dump is tracked by Git.
