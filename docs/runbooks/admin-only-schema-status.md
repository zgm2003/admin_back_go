# Admin database baseline status

Status: active database contract reference

## Canonical baseline

- Baseline version: `202608130001`.
- Schema source: `database/schema.sql`.
- Seed source: `database/seed.sql`.
- Evidence: `database/baseline.json`.
- Applied versions: `schema_migrations`.

The release manifest binds the schema hash, seed hash, and every ordered
post-baseline migration hash. A file whose applied bytes change is invalid.

## Verification

```powershell
pwsh -NoProfile -File scripts/database.ps1 check

pwsh -NoProfile -File scripts/release/check-platform-kernel.ps1 `
  -Database admin
```

The baseline check is read-only. It validates live table, foreign-key, CHECK,
and unique-index counts, seed ownership facts, and the migration ledger.

## Changes

New migration files must use `<12-digit-version>_<lowercase_name>.sql`, with a
version greater than `202608130001`. Application startup never migrates. Back
up MySQL and use a maintenance window before a destructive production change.
Use the locked recovery artifact for rollback; never synthesize reverse DDL.

**STOP** when a baseline hash, live invariant, or migration checksum differs.
