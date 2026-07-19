# Admin Browser-only cutover

Status: operator runbook; P08R evidence is filled only during Task 10

This cutover retires the historical desktop/Tauri transport, client-version
menu/grants, and all pre-cutover Admin sessions. It does **not** drop or modify
the `client_versions` history table and does not delete historical COS objects.
Physical table deletion belongs only to P09 after restore proof and a fresh
explicit approval.

## Preconditions

1. Both repositories are clean, directly on `master`, and each has exactly one
   primary checkout with no `.worktrees` path.
2. Record the full backend and frontend Git revisions and the generated Admin
   Contract Bundle manifest hash.
3. Run both static verification gates before changing live data.
4. Confirm `auth_platforms.code=admin` still has `single_session=1` and
   `max_sessions=1`; this cutover must not change session product policy.
5. Create and verify the current database recovery artifact. Do not proceed
   without the source fingerprint required by `reconcile.ps1`.

## Maintenance order

1. Enter a maintenance window and stop accepting new Admin login traffic.
2. Build and start the exact backend/frontend revisions only through:

   ```powershell
   pwsh -NoProfile -File scripts/docker-platform.ps1 up
   pwsh -NoProfile -File scripts/docker-platform.ps1 status
   ```

3. Apply the idempotent menu/grant reconciliation:

   ```powershell
   pwsh -NoProfile -File scripts/database/reconcile.ps1 `
     -Stage browser-only-retirement `
     -Database admin `
     -ExpectedSourceFingerprint $env:ADMIN_VERIFIED_FINGERPRINT
   ```

   The SQL soft-deletes role grants before permissions and proves the
   `client_versions` count/hash are unchanged in the same transaction.

4. Preview, then apply, the one-time session cutover:

   ```powershell
   $backendCommit = (git rev-parse HEAD).Trim()
   $frontendCommit = (git -C ../admin_front_ts rev-parse HEAD).Trim()

   pwsh -NoProfile -File scripts/browser-only/revoke-admin-sessions.ps1 `
     -BackendCommit $backendCommit -FrontendCommit $frontendCommit

   pwsh -NoProfile -File scripts/browser-only/revoke-admin-sessions.ps1 `
     -BackendCommit $backendCommit -FrontendCommit $frontendCommit -Apply
   ```

   This revokes only active `platform='admin'` sessions and clears only the
   isolated `TOKEN_REDIS_DB`. It does not change users, login logs,
   `auth_platforms`, `single_session`, or `max_sessions`.

5. Verify retirement and run the Browser-only smokes:

   ```powershell
   pwsh -NoProfile -File scripts/browser-only/verify-retirement.ps1
   pwsh -NoProfile -File scripts/basic-admin-smoke.ps1
   pwsh -NoProfile -File scripts/full-admin-smoke.ps1
   pwsh -NoProfile -File scripts/check-admin-contract.ps1
   pwsh -NoProfile -File scripts/docker-platform.ps1 status
   ```

6. End the maintenance window only after the operator records automated
   evidence and the user completes the separate frontend manual checklist.

## Former Windows desktop cleanup

The application cannot remotely erase already installed clients or Windows
Credential Manager entries. Each former desktop user must remove this historical
entry manually after retirement:

```text
service: cn.zgm2003.admin.refresh
account: current-session
```

Then uninstall the old desktop package. No server-side result should claim that
deployment cleared Windows Credential Manager.

## Rollback

P08R uses no reverse DDL. Before user acceptance, rollback means deploying the
previous Docker images together and restoring the matching verified database
artifact if the soft-deleted menu/grant state must be recovered. Sessions were
intentionally revoked and are never resurrected; users sign in again. Do not
recreate desktop refresh transport or mutate frozen `client_versions`/COS
history as an ad-hoc rollback.

## Task 10 evidence record

```text
backend_commit=
frontend_commit=
backend_image_id=
frontend_image_id=
worker_image_id=
contract_manifest_sha256=
reconciliation_run_id=
reconciliation_script_sha256=
client_versions_count=
client_versions_sha256=
revoked_admin_sessions=
token_redis_keys_after=
automated_gate_result=
user_acceptance=
```
