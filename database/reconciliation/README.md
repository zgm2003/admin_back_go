# Imported database reconciliation

Reconciliation runs only against a verified disposable restore. It is serialized in this order:

1. `ledger`
2. `expand`
3. `backfill-core`
4. `backfill-ai`
5. `proven-indexes`

Every SQL file is hashed from its exact bytes before execution. `schema_reconciliation_runs` records the stage, script name, SHA-256, source fingerprint, target fingerprint, executor, timestamps, status, and sanitized details. A successful name/SHA pair is idempotently skipped; a previously recorded name with different bytes is rejected.

The scripts in this directory are non-destructive P02 inputs. App/Canvas contract deletion remains reserved for P09.

`030_verify_schema.sql` through `034_verify_platform.sql` are blocking zero-row/count invariants for the active target. `scripts/database/verify-expanded-schema.ps1` also emits a separate `legacy_evidence` summary for data that is deliberately preserved rather than treated as active target state:

- App/Canvas/all-platform rows awaiting P09 disposition; the historical quick-entry table is already absent from the current restore and active runtime;
- grants whose permission record is already absent, which cannot grant runtime access and are retained for audit;
- image files and runs paired by the task ID encoded in `ai_runs.request_id` after an older migration removed their soft-deleted parent task;
- global AI assets without an explicit user owner;
- export URLs that cannot be decoded against the enabled COS domain;
- COS reachability results whose exact keys live only in the restricted temporary evidence manifest.

Non-zero legacy evidence is never rewritten or deleted by P02. P09 must consume the report and make each destructive disposition explicit.
