# Forward migrations

This directory contains only database changes created after baseline
`202608130001`.

Name each migration `<12-digit-version>_<lowercase_name>.sql`. Versions must be
strictly greater than the baseline and must increase in execution order. Keep
each file short, forward-only, and reviewable. Do not edit a migration after
`scripts/database.ps1 migrate` has recorded its SHA-256 in `schema_migrations`.

Application startup never runs migrations. Use:

```powershell
pwsh -NoProfile -File scripts/database.ps1 migrate
pwsh -NoProfile -File scripts/database.ps1 check
```
