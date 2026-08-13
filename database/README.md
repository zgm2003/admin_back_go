# Database baseline

MySQL schema has one source: `database/schema.sql`. Required non-secret startup
facts have one source: `database/seed.sql`. Public address reference data lives
in `database/reference/address.sql`. The single database command loads it between
the schema and seed; it is not a migration or user-owned business state. Files
under `database/migrations/` are forward-only changes created after baseline
`202608130001`.

Application startup never changes the database. Use the repository-owned
PowerShell entry from the backend repository:

```powershell
pwsh -NoProfile -File scripts/database.ps1 init
pwsh -NoProfile -File scripts/database.ps1 reset -ConfirmReset admin -CreateAdmin -AdminUsername "Local Admin" -AdminEmail admin@example.com
pwsh -NoProfile -File scripts/database.ps1 migrate
pwsh -NoProfile -File scripts/database.ps1 check
```

`init` requires an empty `admin` schema. `reset` refuses a running Admin API,
Worker, or `admin-dev`, replaces only the local `admin` schema, clears Redis DB
0/2/3, and removes only Qdrant aliases and collections with the configured
`admin_context_` prefix. It never stops application processes automatically.

`-CreateAdmin` reads the password from `ADMIN_INITIAL_PASSWORD` or a hidden
credential prompt. Passwords and password hashes do not belong in SQL, command
arguments, logs, or initialization data.

## Forward migrations

Name each new migration `<12-digit-version>_<lowercase_name>.sql`. Its version
must be greater than `202608130001`. Never edit a migration after it has been
applied: `scripts/database.ps1 migrate` records its SHA-256 in
`schema_migrations` and rejects changed bytes. Keep each migration short and
forward-only; take a backup before destructive production changes.

`scripts/database.ps1 check` is read-only. It verifies baseline file hashes,
live table/foreign-key/CHECK/unique-index counts, seed ownership facts, and the
exact ordered migration ledger.
