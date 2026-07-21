# Admin-only schema status

Status: final P09 contract reference

## Fingerprints

- Pre-contract source fingerprint:
  `2196c34285433b56b7ed9b2bd12394ce1e2c06472b52abcbcfbc85901a0ffafd`.
- Post-contract **target fingerprint**:
  `9a019819051e7252cb09c4c4ea56cd3b285aedb4362d49f6ec95f80299604678`.

The release manifest binds the target fingerprint and `atlas.sum`. A different
source or target is not equivalent even if migrations report success.

## Contract groups and stop points

1. `202607150201` migrates classified Admin-only rows. **STOP** if preconditions,
   row dispositions, ownership, or counts differ.
2. `202607150202` removes retired product schema, including the approved
   `client_versions` table drop. **STOP** without the exact fresh approval or if
   any runtime/table reference remains.
3. `202607150203` installs final constraints and indexes. **STOP** on any
   constraint failure or target fingerprint mismatch.

`users_quick_entry`, `canvas_prompts`, `canvas_assets`, legacy AI billing
tables, and their historical product surfaces were already absent before P09;
the contract proves absence and does not recreate them. The generic
`auth_platforms`, RBAC/session/login-log/notification dimensions, provenance
columns, and indexes remain.

## Verification

```powershell
pwsh -NoProfile -File scripts/database/check-drift.ps1 `
  -Database $env:ADMIN_RELEASE_DB

go run ./cmd/admin-db invariants `
  --schema $env:ADMIN_RELEASE_DB `
  --file database/reconciliation/053_verify_admin_only.sql

pwsh -NoProfile -File scripts/release/check-platform-kernel.ps1 `
  -Database $env:ADMIN_RELEASE_DB
```

All invariant counts must be zero, `client_versions` must be absent only on the
approved post-contract database, and canonical drift must be zero. Never apply
reverse DDL to recover deleted rows; use the locked recovery artifact.
