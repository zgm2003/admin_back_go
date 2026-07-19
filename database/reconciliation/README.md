# Imported database reconciliation

Reconciliation runs only against a verified disposable restore. It is serialized in this order:

1. `ledger`
2. `expand`
3. `backfill-core`
4. `backfill-ai`
5. `proven-indexes`
6. `ai-image-soft-delete`
7. `export-cleanup-schedule`
8. `realtime-retention`
9. `cron-task-utf8-metadata`
10. `browser-only-retirement`

Every SQL file is hashed from its exact bytes before execution. `schema_reconciliation_runs` records the stage, script name, SHA-256, source fingerprint, target fingerprint, executor, timestamps, status, and sanitized details. A successful name/SHA pair is idempotently skipped; a previously recorded name with different bytes is rejected.

The scripts in this directory are non-destructive P02 inputs. App/Canvas contract deletion remains reserved for P09.

`030_verify_schema.sql` through `038_verify_browser_only_retirement.sql` are blocking zero-row/count invariants for the active target. `046_retire_client_version_surface.sql` soft-deletes only the exact Admin client-version page/button permissions and their role grants, while proving the `client_versions` row count and deterministic row hash are unchanged in the same transaction. The historical table and COS objects stay frozen for P09; this stage contains no destructive DDL. AI image deletion uses the approved retained-object policy: task and file rows are soft-deleted in one transaction, while COS objects remain available for an explicit P09 retention/cleanup decision. Export expiration is owned by the hourly `export:cleanup-expired:v1` Worker command; list and count reads do not mutate tasks. P05 realtime retention is owned by the daily `realtime:cleanup-expired:v1` Worker command and atomically advances per-target watermarks. Cron task metadata is byte-for-byte verified after every reconciliation so client-encoding drift cannot pass the database gate. `scripts/database/verify-expanded-schema.ps1` also emits a separate `legacy_evidence` summary for data that is deliberately preserved rather than treated as active target state:

- App/Canvas/all-platform rows awaiting P09 disposition; the historical quick-entry table is already absent from the current restore and active runtime;
- grants whose permission record is already absent, which cannot grant runtime access and are retained for audit;
- image files and runs paired by the task ID encoded in `ai_runs.request_id` after an older migration removed their soft-deleted parent task;
- global AI assets without an explicit user owner;
- export URLs that cannot be decoded against the enabled COS domain;
- COS reachability results whose exact keys live only in the restricted temporary evidence manifest.

Non-zero legacy evidence is never rewritten or deleted by P02. P09 must consume the report and make each destructive disposition explicit.
