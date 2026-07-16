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

## Verified recovery artifact

Before reconciliation, set `ADMIN_DB_HOST`, `ADMIN_DB_PORT`, `ADMIN_DB_USER`, and `ADMIN_DB_PASSWORD` only in the current process, then run:

```powershell
pwsh -NoProfile -File scripts/database/new-recovery-artifact.ps1 `
  -Database admin `
  -BackupRoot (Join-Path $env:TEMP 'admin-db-recovery')
```

The backup root must be a local filesystem path outside both repositories; UNC paths are rejected, including local administrative shares. Local extended drive paths such as `\\?\E:\...` are normalized before the physical-path check. The source database is used only for critical row counts and `mysqldump`; it never receives recovery `CREATE DATABASE` or `DROP DATABASE` statements. The dump uses a temporary MySQL option file atomically created with a current-user-only ACL, and is checked for critical definitions and a lowercase SHA-256.

Restore verification runs in a fixed `mysql:8.4.10` temporary container with `--network none`. It publishes no ports and mounts no host credentials. The dump is copied into the isolated container, restored into a random `admin_restore_<12hex>` database, and checked against the source's exact migration-sensitive row counts. The random database is dropped, and the container plus its anonymous MySQL data volume are removed with `docker rm --force --volumes`; the temporary option file is also deleted.

Only after all verification and cleanup succeed does the script atomically write `artifact.json`. It prints exactly the artifact path and lowercase dump SHA-256. `database/evidence/` is ignored defense-in-depth and must never contain a tracked dump or credential file.

Every external database or Docker command has a bounded timeout. `CommandTimeoutSeconds` defaults to 1800 seconds for dump and restore work. `ReadinessTimeoutSeconds` defaults to 180 seconds for one in-container loop that waits for both the final PID 1 `mysqld` process and a successful socket ping, so the image's temporary initialization server is never mistaken for the restore server. On timeout the script attempts to terminate the process tree and reports any termination failure; operators may lower these values for controlled tests or raise them within the validated ranges for unusually large recoveries.
