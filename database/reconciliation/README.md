# Imported database reconciliation

Reconciliation runs only against a verified disposable restore. It is serialized in this order:

1. `ledger`
2. `expand`
3. `backfill-core`
4. `backfill-ai`
5. `proven-indexes`

Every SQL file is hashed from its exact bytes before execution. `schema_reconciliation_runs` records the stage, script name, SHA-256, source fingerprint, target fingerprint, executor, timestamps, status, and sanitized details. A successful name/SHA pair is idempotently skipped; a previously recorded name with different bytes is rejected.

The scripts in this directory are non-destructive P02 inputs. App/Canvas contract deletion remains reserved for P09.
