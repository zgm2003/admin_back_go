# Database layout

The database tree separates imported history, reconciliation, and future Atlas migrations.

- `legacy-migrations/` preserves historical SQL as audit evidence. Never execute this directory automatically or treat filename order as a valid migration plan.
- `reconciliation/` contains staged, checksummed scripts for reconciling an imported Admin database. These scripts are introduced by the P02 database-evolution plan.
- `schema/` contains the canonical Admin schema after reconciliation has been verified.
- `migrations/` is the checksummed Atlas migration directory for the canonical baseline and future migrations. Its single checksum source is `migrations/atlas.sum`, because Atlas resolves the checksum relative to the migration directory.

Application startup never applies database migrations. Validation and deployment use the digest-pinned Atlas wrapper:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/database/atlas.ps1 migrate validate --dir file://database/migrations
```

The wrapper runs without network access and mounts the repository read-only. Docker is an external verification requirement; environments without Docker must rely on the protected database-verification workflow for this gate.

Real dumps, recovery artifacts, MySQL option files, and `deploy/docker-first/admin-go.env` must remain outside Git.
