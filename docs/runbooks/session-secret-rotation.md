# Session secret rotation

`APP_SECRET` is a deployment-wide root secret. Rotate it only as an explicit Docker deployment operation. All API and Worker nodes in one deployment must use the same current/previous pair.

## Preconditions

1. Deploy this key-ID-capable release with the existing `APP_SECRET` and no `APP_SECRET_PREVIOUS`.
2. Wait one maximum access-token TTL so pre-release JWTs without an explicit `kid` expire.
3. Back up the database and inventory the encrypted AI, upload, mail, SMS, payment, and storage configuration values held in the deployment secret store.
4. Run:

   ```powershell
   pwsh -NoProfile -File scripts/tests/session-secret-rotation.tests.ps1
   ```

The rehearsal runs its old, dual-key, and new-only session nodes in Docker against the shared MySQL/Redis services. It proves issue, authenticate, rotate, revoke, cross-node propagation, current-key signing, previous-key verification, and final old-key rejection. Generated secrets and logs live only below a verified temporary directory and are deleted after the run.

## Rotation procedure

1. Generate a new 64-or-more-character random secret without printing it or placing it in shell history.
2. Update the owner-only Docker env on every API/Worker node:

   ```dotenv
   APP_SECRET=<new-current-secret>
   APP_SECRET_PREVIOUS=<old-secret>
   ```

3. Recreate API and Worker containers through the approved Docker deployment. Do not start host processes. Never use `docker compose down -v`.
4. Verify every node is healthy and that:
   - an access credential issued before the dual-key deployment still authenticates;
   - newly issued JWT headers contain the new current `kid`;
   - a newly issued Browser session can rotate its Cookie-held refresh credential;
   - the old refresh credential cannot rotate after the root-secret cutover.
5. Sign in again under the new current key. Revoke every session created before the cutover through the Admin session-management surface. The refresh-token pepper intentionally has no previous-key fallback.
6. `APP_SECRET` also derives the secretbox key. Re-enter every encrypted business credential from the deployment secret store so it is encrypted under the new current key; validate each affected provider before continuing. This release does not silently migrate or log plaintext credentials.
7. Keep the dual JWT verification window no longer than the declared maximum access-token TTL. Confirm no required old-key access session remains.
8. Remove `APP_SECRET_PREVIOUS` from every node, recreate API/Worker containers with Docker, and prove:
   - current-key credentials still authenticate;
   - old-key access credentials fail;
   - old refresh credentials fail;
   - encrypted business-provider probes succeed under the current secret.

## Rollback

During the dual window, restore the old value as `APP_SECRET` and remove `APP_SECRET_PREVIOUS`, then recreate all API/Worker containers together. After old sessions or encrypted configuration have been deliberately replaced, rollback requires restoring the matching database backup and deployment secret set; do not mix root-secret generations.

## Invariants

- `APP_SECRET_PREVIOUS` is optional and accepts exactly one key.
- Current and previous values must differ and each must satisfy the runtime secret policy.
- JWT issue always uses the current key and an explicit `kid`; parse never guesses a key.
- MySQL remains session truth, Redis is shared by all nodes, and revocation denial must propagate within two seconds.
- Secrets must not appear in Git, command output, logs, metrics, test artifacts, or process arguments.
