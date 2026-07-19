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

5. Verify retirement and run the Browser-only smokes against the already
   running Docker API:

   ```powershell
   pwsh -NoProfile -File scripts/browser-only/verify-retirement.ps1
   pwsh -NoProfile -File .tmp/p08r-docker-smoke.ps1
   pwsh -NoProfile -File scripts/check-admin-contract.ps1
   pwsh -NoProfile -File scripts/docker-platform.ps1 status
   ```

   The Task 10 one-time smoke harness targets `http://127.0.0.1:8080`, creates
   and removes its temporary password-login identity through the formal Admin
   API, emits no credential, and starts no process. Its reviewed SHA-256 is
   recorded below. Do **not** run the historical `basic-admin-smoke.ps1` or
   `full-admin-smoke.ps1` during this cutover: both still compile and launch a
   host API/Worker with `Start-Process`, which violates the Docker-only runtime
   boundary. P09 owns the permanent Docker release-smoke replacement.

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
backend_commit=9cce01072c5713983f8646c69d30e8bc61c826d2
frontend_commit=39fe04755a4fc76a83ab385a961cb9ccbbb08f92
backend_image_id=sha256:27e4ff63e0c9b74805478faaeef0350f4366a442a34865b72e9ab6642b54164d
frontend_image_id=sha256:cffc07471498bcf60239029c0ea54f411f639aeb27c3250d714ad9fb730558f2
worker_image_id=sha256:27e4ff63e0c9b74805478faaeef0350f4366a442a34865b72e9ab6642b54164d
contract_manifest_sha256=d0a7649f4fe22ac5a095a108e7c8969fa1a626dea50fdf82f1fa19dfc0b8b1fa
recovery_artifact=C:\Users\Administrator\AppData\Local\Temp\admin-p08r-recovery\20260720T010547947-93598a6f17bc\artifact.json
recovery_dump_sha256=a9590af7315c105809ac34ad0f438e59d8f38d4b0dbf87656295343b6d2178ec
source_schema_fingerprint=2196c34285433b56b7ed9b2bd12394ce1e2c06472b52abcbcfbc85901a0ffafd
reconciliation_run_id=13
reconciliation_script_sha256=e66c16c5a6bab94f9bdeba321ef3c7929dab9e94f1a2153da9955d0d97c6a64f
client_versions_count=8
client_versions_sha256=ca574b6ce101d92b05cc3571e7e138aa9bf2bc5096c04357c8d39792ba806661
revoked_admin_sessions=1
post_fix_active_admin_sessions=0
token_redis_keys_after=0
smoke_harness_sha256=e8d8214d3d1e7b8632d9c31014f1a9e9b0e643fd90b90249a79ecc262640c209
automated_gate_result=passed
user_acceptance=passed_at_2026-07-20T05:35:17+08:00
```

The first cutover revoked one pre-cutover Admin session. During final evidence
review, the empty Redis-password path was found to set an empty
`REDISCLI_AUTH`, so `redis-cli` remained on DB0 instead of selecting the
isolated Token DB2. Commit `9cce01072c5713983f8646c69d30e8bc61c826d2`
added a failing regression guard and a no-auth invocation path. The corrected
apply pass observed Token DB2 `1 -> 0`, zero active Admin sessions, and the
retirement verifier then passed with DB0 untouched.
