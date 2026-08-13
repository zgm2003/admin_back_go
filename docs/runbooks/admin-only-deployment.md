# Admin-only immutable deployment

Status: production operator runbook

This runbook deploys one synchronized Browser-only Admin frontend/API/Worker
release from the generated manifest. It does not build from an older commit,
publish a desktop unit, use a deployment Workflow, or modify historical COS
objects.

## Roles

- **Release operator** verifies Git commits, image/archive digests, Compose
  resources, health, readiness, and Admin smoke evidence.
- **Database operator** owns the recovery artifact, source/target fingerprints,
  the maintenance lock, and all three Atlas contract groups.
- **Security owner** supplies the approval and secrets through the approved
  environment or ignored files. Values are never pasted into logs or evidence.

No one role may silently waive a failed gate. The Database operator controls
database execution; the Release operator controls image promotion.

## Required artifacts

Acquire these as one release package and verify them before the Maintenance
window begins:

```text
release/admin-only/out/release-manifest.json
release/admin-only/out/platform-kernel-proof.json
release/admin-only/out/images/metadata.json
release/admin-only/out/images/backend-<commit>.tar
release/admin-only/out/images/frontend-<commit>.tar
locked recovery artifact and restore-rehearsal evidence
approved backend environment file
```

The manifest commits must equal the two clean primary checkouts. The archive
hashes, loaded image IDs, OCI revision labels, Bundle digest, Atlas checksum,
database target fingerprint, and all evidence hashes must match exactly.

## Preflight

1. Confirm the approved Maintenance window, incident channel, rollback owner,
   RTO, and RPO.
2. `admin-status` must show that the development supervisor is stopped on the
   deployment host. The deploy script intentionally rejects a live
   `admin-dev` lock.
3. Confirm one primary checkout per repository, no secondary worktree, clean
   status, and no `.github` directory or deployment Workflow.
4. Verify the recovery artifact was restored successfully and its dump hash
   matches the pre-contract input lock.
5. Verify retained COS keys and the historical-object disposition evidence.
6. Run the non-mutating package check:

   ```powershell
   pwsh -NoProfile -File scripts/release/check-release-manifest.ps1 `
     -Manifest release/admin-only/out/release-manifest.json
   ```

**STOP** if any path, digest, revision label, proof, checkout, or
acceptance artifact differs. Never regenerate evidence to hide a mismatch.

## Deploy

Set secret-bearing variables from the approved environment or ignored file;
do not echo them. Previewing or omitting `-Apply` must fail closed.

```powershell
pwsh -NoProfile -File scripts/release/deploy-admin-only.ps1 `
  -Manifest release/admin-only/out/release-manifest.json `
  -Database $env:ADMIN_RELEASE_DB `
  -BackendEnvFile $env:ADMIN_BACKEND_ENV_FILE `
  -RuntimeVolume admin-runtime `
  -ExportVolume admin-exports `
  -MaintenanceWindow `
  -Apply
```

The script validates and loads immutable archives, runs
`scripts/database.ps1 migrate` followed by the read-only
`scripts/database.ps1 check`, starts staging
with `--no-build`, probes
health/readiness and Admin smoke, then promotes the exact images. It archives
manifest/proof/metadata under the release ID before atomically changing
deployment state.

## Verify and close

Record only IDs, hashes, counts, and timings:

```powershell
pwsh -NoProfile -File scripts/release/verify-admin-only-release.ps1 `
  -Manifest release/admin-only/out/release-manifest.json `
  -Database $env:ADMIN_RELEASE_DB
```

Confirm frontend `/healthz`, API `/health`, API `/ready`, authenticated Admin
HTTP, WebSocket resume, queue/Worker recovery, scheduler ownership, and provider
error metrics. Close the window only when `release/admin-only/out/proof.json`
passes and the operator records the release ID.

**STOP** and invoke the rollback runbook if staging, promotion, health,
readiness, smoke, durable-work recovery, or post-contract drift fails.
