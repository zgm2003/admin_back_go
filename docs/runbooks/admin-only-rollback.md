# Admin-only immutable rollback

Status: production operator runbook

Rollback uses the previous archived manifest, proof, image metadata, and image
archives. It never rebuilds an old commit, guesses deleted rows, or invents
reverse DDL.

## Choose the mode

### Application rollback

Use Application rollback when the database remains compatible and only the
frontend/API/Worker images must return to the previous synchronized release.
The previous images are loaded by ID, started with `--no-build`, and checked by
health, readiness, and Admin smoke.

### Full database rollback

Use Full database rollback only when the contract database itself must return
to the pre-contract point. It requires the locked **recovery artifact**, matching
**recovery rehearsal** evidence, an approved
maintenance window, and the Database operator. The verified dump is restored;
there is no reverse DDL and `client_versions` is not reconstructed from guessed
metadata.

## Preconditions

1. Record incident start time, release ID, impact, RTO, RPO, and rollback owner.
2. Keep the current and previous release packages. Do not remove image archives
   or state volumes.
3. Verify `deployment-state.json` manifest hash and all previous archive hashes.
4. Confirm previous image revision labels equal previous manifest commits.
5. For Full database rollback, verify the artifact and rehearsal hashes before
   stopping the current release.

**STOP** if deployment state points outside `release/admin-only/out`, if any
digest differs, or if no previous package exists.

## Application rollback command

```powershell
pwsh -NoProfile -File scripts/release/rollback-admin-only.ps1 `
  -BackendEnvFile $env:ADMIN_BACKEND_ENV_FILE `
  -RuntimeVolume admin-runtime `
  -ExportVolume admin-exports `
  -Apply
```

## Full database rollback command

```powershell
pwsh -NoProfile -File scripts/release/rollback-admin-only.ps1 `
  -FullDatabaseRollback `
  -MaintenanceWindow `
  -RecoveryArtifact $env:ADMIN_RECOVERY_ARTIFACT `
  -RecoveryRehearsalEvidence $env:ADMIN_RECOVERY_REHEARSAL `
  -Database $env:ADMIN_RELEASE_DB `
  -BackendEnvFile $env:ADMIN_BACKEND_ENV_FILE `
  -RuntimeVolume admin-runtime `
  -ExportVolume admin-exports `
  -Apply
```

## Verification and escalation

After either mode, require frontend health, API health/readiness, authenticated
HTTP, WebSocket reconnect/resume, queue recovery, scheduler single ownership,
and provider/storage probes. For Full database rollback, also compare restored
table counts with the locked recovery artifact.

Record actual RTO and recovered data point for RPO. Escalate if either exceeds
the approved objective, if durable work cannot resume/cancel, or if smoke fails.

**STOP** after the first failed verification. Preserve containers, logs, proof,
and packages; redact credentials before attaching evidence to the incident.
