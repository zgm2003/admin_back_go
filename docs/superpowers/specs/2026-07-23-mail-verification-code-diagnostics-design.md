# Mail Verification Code Diagnostics Design

**Date:** 2026-07-23

## 1. Purpose

Administrators need to inspect the verification code associated with an email
delivery while troubleshooting login and account-recovery problems. The mail
log list and detail currently expose delivery facts only, so they cannot show
the code or whether its configured lifetime has elapsed.

This design adds an RBAC-protected, encrypted verification-code diagnostic
snapshot to email delivery logs. It preserves the existing authentication and
mail module boundaries and does not turn the mail database into a second
verification-code authorization source.

## 2. Approved User Behavior

1. An administrator with `system_mail_logView` can open the mail sending-log
   tab and see the plaintext verification code in both the list and detail.
2. The same views show the code's time status and absolute expiration time.
3. Time status is one of `sending`, `not_expired`, `expired`, or `send_failed`.
4. The Chinese labels are respectively `发送中`, `未过期`, `已过期`, and
   `发送失败`.
5. The UI deliberately says `未过期`, not `有效`. A code can be temporally
   unexpired while already consumed or replaced in Redis.
6. Test emails and historical mail logs without a diagnostic snapshot return
   and display no verification-code data.
7. A user without `system_mail_logView` does not see the sending-log tab, and
   direct list or detail requests receive HTTP 403.
8. The implementation creates the permission definition but does not grant it
   to any role. The administrator will assign it through existing role
   management.

## 3. Existing Architecture And Invariants

The current modular-monolith boundaries remain authoritative:

- `internal/module/auth` owns code generation, captcha validation, Redis code
  state, atomic consumption, resend replacement, and authentication decisions.
- `internal/module/mail` owns Tencent SES orchestration, `mail_configs`,
  `mail_templates`, and `mail_logs`.
- Auth depends only on the small `VerifyCodeMailSender` capability. It does not
  import the mail module, query mail tables, or know Tencent-specific fields.
- Redis is the real-time source for whether a code can authenticate. MySQL mail
  diagnostics never authorize a login.
- `mail_logs` remains a delivery-fact table. Verification-code diagnostic data
  is not added as nullable columns to that table.
- Route metadata and MySQL RBAC data are the authorization truth. Frontend
  visibility is not an authorization boundary, and there is no super-admin
  bypass.
- Application startup never applies database migrations.

The recently added Redis Lua consumption and delivery-lease behavior remains
in place. This feature must not weaken atomic consumption, replacement safety,
or the rule that an old code cannot authenticate after consumption or resend.

## 4. Scope And Non-goals

In scope:

- email verification-code diagnostic snapshots owned by the mail module;
- encryption at rest and authorized decryption on mail-log reads;
- list and detail API contract additions;
- time-status calculation from delivery facts and one absolute expiration
  deadline;
- the `system_mail_logView` permission definition and route policies;
- frontend list/detail rendering, permission visibility, and Chinese labels;
- Atlas schema evolution, canonical schema, permission seed, generated Admin
  Contract Bundle, architecture documentation, and tests;
- local runtime verification through `admin-dev`.

Out of scope:

- recording `consumed`, `superseded`, or other authentication lifecycle states
  in MySQL;
- changing Redis from the verification authorization source;
- changing the Redis key shape solely for mail-log display;
- adding SMS verification-code display;
- storing email body, complete template data, captcha answers, or credentials;
- adding an independent diagnostic-record CRUD API;
- automatically granting permissions or writing `role_permissions`;
- adding a cleanup schedule or changing existing mail-log retention;
- rebuilding application Docker images for local verification.

If consumed or replacement history is requested later, it requires a separate
auth-owned audit design with explicit cross-store consistency and recovery
semantics. It must not be inferred from mail delivery data.

## 5. Chosen Architecture

The mail module adds one immutable child record per verification-code mail log:

```text
auth.Service
  -> VerifyCodeMailSender
      -> mail.Service
          -> transaction: mail_logs + mail_log_verification_codes
          -> Tencent SES
          -> finalize mail_logs delivery result

mail log GET
  -> mail handler
      -> mail service
          -> mail repository LEFT JOIN child snapshot
          -> secretbox decrypt
          -> derive temporal status
```

This keeps write and read ownership inside `internal/module/mail`. Auth passes
the code, configured TTL, and absolute expiration deadline through its existing
sender boundary, but it does not receive a mail-log ID or update mail data when
Redis state changes.

The narrow sender method keeps its configured `ttl time.Duration` input and
adds an `expiresAt time.Time` input. Primitives are used deliberately so mail
does not import an auth-owned DTO and auth does not import the mail module.

Rejected alternatives:

- Adding code and lifecycle columns directly to `mail_logs` mixes delivery
  facts with authentication state and leaves many nullable fields on non-code
  emails.
- An auth-owned durable lifecycle table would require a cross-module admin read
  projection and Redis/MySQL lifecycle synchronization that the requested
  expiration-only status does not justify.
- Reading Redis from mail-log list/detail loses history at TTL expiry, creates
  an N+1 or batch-cache dependency on a read path, and still cannot recover
  consumed history after key deletion.

## 6. Data Model

The new mail-owned table is `mail_log_verification_codes`:

| Column | Contract |
| --- | --- |
| `id` | `BIGINT UNSIGNED`, primary key, auto increment |
| `mail_log_id` | `BIGINT UNSIGNED NOT NULL`, unique one-to-one reference to `mail_logs.id` |
| `code_enc` | `VARCHAR(255) NOT NULL`, existing `secretbox` ciphertext only |
| `expires_at` | `DATETIME NOT NULL`, absolute authentication deadline |
| `created_at` | `DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP` |

The table has a unique key on `mail_log_id` and a foreign key to `mail_logs`
that restricts physical parent deletion. It has no `status`, `updated_at`, or
independent `is_del` column:

- the snapshot is immutable;
- status is derived and must not become stale persisted data;
- it has no independent HTTP surface;
- visibility and soft deletion always follow the parent `mail_logs.is_del=2`
  query.

`code_enc` uses the `secretbox.Box` already injected into `mail.Service` and
derived from `APP_SECRET`. Plaintext is never stored in `mail_logs`, the child
table, error messages, access logs, operation logs, or runtime logs.

Existing mail logs are not backfilled. A missing child row is valid historical
state and projects all three verification fields as JSON `null`.

## 7. Expiration Deadline And Delivery Flow

Auth computes one deadline for an email verification delivery:

1. Validate account, scene, channel readiness, and captcha as today.
2. Load the configured mail verification TTL.
3. Generate the six-digit code.
4. Compute one absolute `expires_at` immediately before pending Redis state is
   installed.
5. Compute `remainingTTL = time.Until(expires_at)` and reject a non-positive
   result before calling `SetPendingDelivery`, so the Redis key uses that
   deadline rather than independently adding a fresh full TTL.
6. Pass the configured TTL and the same `expires_at` to
   `VerifyCodeMailSender`. The TTL remains the template's `ttl_minutes` input;
   `expires_at` is the diagnostic deadline.
7. Continue the existing send, renewal, commit, and rollback sequence.

The mail sender validates a positive configured TTL and a future expiration
deadline. It then performs this sequence:

1. Resolve and validate the enabled config and scene template.
2. Encrypt the code before starting database work.
3. In one short MySQL transaction, insert the pending `mail_logs` row and its
   one-to-one diagnostic snapshot.
4. Commit the transaction before the external Tencent SES call; no database
   transaction is held across network I/O.
5. Call Tencent SES with the exact code and configured TTL template values.
6. Finalize only the parent mail log as success or failure using the existing
   delivery fields.

If encryption or the creation transaction fails, Tencent SES is not called and
auth rolls back the pending Redis code. If Tencent SES succeeds but finalizing
the mail log fails, the sender returns an error and auth does not activate the
code. If Redis commit fails after a successful mail delivery, the email remains
a successful delivery fact while the code is not usable for authentication.
The UI's `not_expired` wording intentionally makes no usability claim.

Provider failure remains the primary returned error when both delivery and
failure-log finalization fail. Diagnostic persistence must not replace a more
specific Tencent delivery error with a generic logging error.

## 8. Read Model And Status Semantics

The mail repository returns log rows with an optional joined diagnostic
snapshot. The service decrypts the ciphertext only on the two permission-
protected log reads and projects these nullable fields in both list and detail:

```text
verification_code
verification_code_status
verification_code_expires_at
```

Status is calculated with one service clock using this precedence:

1. No child snapshot: status is `null`.
2. Parent delivery status is failed: `send_failed`.
3. `now >= expires_at`: `expired`.
4. Parent delivery status is pending: `sending`.
5. Parent delivery status is success and the deadline is in the future:
   `not_expired`.

The exact deadline is expired, not unexpired. A pending row that outlives its
deadline becomes `expired` rather than remaining `sending` forever.

The page-init response adds a typed
`mail_verification_code_status_arr` dictionary for the four string values. The
frontend consumes that dictionary and does not invent fallback status values
or compare translated labels.

Invalid ciphertext, a missing decryptor, or an impossible joined diagnostic
record is an explicit internal error. The API fails closed instead of returning
an empty code that looks like a legitimate historical null.

## 9. RBAC Contract

The deterministic permission definition is:

```text
id        = 515
parent_id = 506                    # system_mail page
type      = 3                      # BUTTON
sort      = 9
name      = 查看邮件日志
code      = system_mail_logView
platform  = admin
status    = 1
is_del    = 2
```

Both routes use compiled permission metadata:

```text
GET /api/admin/v1/mail/logs       -> system_mail_logView
GET /api/admin/v1/mail/logs/:id   -> system_mail_logView
```

The list and detail share one permission because they expose the same sensitive
resource. Delete routes retain `system_mail_logDel`; neither permission
implicitly grants the other.

The permission migration and empty-database seed create only the definition.
They do not insert, restore, or change any `role_permissions` row. The user will
assign `system_mail_logView` through role management, whose existing principal
mutation flow invalidates RBAC snapshots correctly. Until then, list/detail
return 403 and the frontend hides the sending-log tab.

## 10. HTTP And Frontend Contract

`LogDTO` adds three required-but-nullable properties so every list and detail
item has one deterministic shape:

```json
{
  "verification_code": "654321",
  "verification_code_status": "not_expired",
  "verification_code_expires_at": "2026-07-23 15:04:05"
}
```

For test sends and historical rows, all three values are `null`. There are no
fallback aliases or omitted-field guesses.

The sending-log list adds columns for verification code, time status, and
expiration time. The detail descriptions show the same three values. Codes are
shown in full without masking, as explicitly approved. Null values render as
`-`. The existing delivery status remains a separate column and label.

The sending-log tab renders only when `userStore.can('system_mail_logView')` is
true. Tab initialization must choose an available tab rather than retaining a
hidden active tab. API types continue to be generated from the Admin Contract
Bundle and validated by narrow frontend decoders; `any`, silent fallbacks, and
handwritten duplicate response contracts are not introduced.

The current detail notice that says verification codes are not stored is
updated to describe the authorized diagnostic behavior accurately. It must not
expose implementation secrets or present frontend visibility as the security
boundary.

## 11. Error Handling And Sensitive Data

- Ciphertext creation failure stops before database insertion and provider I/O.
- Snapshot transaction failure stops before provider I/O.
- Provider errors retain the existing sanitized provider error flow.
- Decryption failure returns an explicit mail diagnostic read error and no
  partial plaintext response.
- Permission denial occurs before the handler and returns the existing 403
  envelope.
- GET log routes remain read-only and are not added to OperationLog.
- AccessLog continues to omit request and response bodies.
- No debug print, test failure output, smoke output, or browser console log may
  include a real code.
- The frontend does not persist mail-log responses in local storage, session
  storage, or another client cache.

Encryption at rest protects database files and backups from casual plaintext
inspection. Authorized HTTP responses intentionally contain plaintext because
that is the approved administrator diagnostic feature.

## 12. Database Evolution And Compatibility

Implementation adds a forward-only Atlas migration and updates:

- `database/schema/admin.hcl`;
- `database/migrations/atlas.sum`;
- schema and relation verification gates;
- `database/seeds/admin_permissions.sql` and its deterministic row-count tests;
- the tracked Admin permission contract.

The migration creates the child table and permission definition but never
touches `role_permissions`. The deterministic permission seed grows from 125 to
126 rows. Existing databases receive no fabricated diagnostic rows, and empty
databases receive the same permission definition through the canonical seed.

The database migration is applied explicitly before running the changed API.
Neither `admin-dev` nor application startup applies it automatically.

The generated Admin Contract Bundle, frontend generated types, route-policy
goldens, runtime-model contract documentation, and backend architecture
documentation are updated together. This design supersedes only the earlier
statements that no verification code is durably retained anywhere for mail
diagnostics. It does not permit new plaintext persistence or new Admin API
exposure outside the two authorized mail-log reads.

## 13. Test Strategy

Backend red-green coverage includes:

- one deadline being shared by Redis pending TTL and the mail sender;
- successful creation of parent and encrypted child rows in one transaction;
- no provider call when encryption or the creation transaction fails;
- plaintext absence from persisted columns, errors, and non-authorized internal
  projections; plaintext appears only in the two authorized HTTP DTOs;
- provider success/failure finalization with an immutable child snapshot;
- list and detail joining and decrypting the same code;
- `null` projections for test sends and historical logs;
- all four status values and the exact `now == expires_at` boundary;
- expired precedence for an old pending row and failed precedence for a failed
  delivery;
- explicit failure for corrupt ciphertext;
- both GET routes requiring `system_mail_logView` and denying a principal that
  has only `system_mail` or `system_mail_logDel`;
- permission seed identity, parent, count, and the absence of persistent
  `role_permissions` writes;
- Atlas validation, canonical-schema drift checks, contract generation, route
  policy goldens, and architecture gates;
- existing Redis integration tests for matching consumption, mismatches,
  pending state, resend replacement, and concurrent ownership.

Frontend red-green coverage includes:

- strict decoding of nullable code fields and the closed status enum;
- list columns and detail fields using the same values;
- Chinese status dictionary labels without raw translation keys;
- `-` rendering for historical and test logs;
- sending-log tab visibility for granted and ungranted users;
- a valid fallback active tab when sending logs are hidden;
- no request to either log endpoint when the tab is unavailable;
- generated operation and schema type checks after bundle synchronization.

Verification sequence:

1. Run focused backend and frontend tests during implementation.
2. Run the repositories' full host-side quality, unit, contract, locale, and
   architecture gates supported by the fixed toolchains.
3. Apply the non-destructive Atlas migration explicitly to the local Docker
   state database.
4. Start API, worker, and Vite through `admin-dev`; do not rebuild application
   Docker images.
5. Before manual role assignment, verify the tab is hidden and direct list and
   detail requests return 403.
6. Automated route/service/component tests provide the authorized-path proof
   without mutating local role grants. After the user assigns the permission,
   verify one real email code appears identically in list and detail and shows
   the expected expiration deadline.
7. Stop `admin-dev` cleanly while leaving MySQL and Redis state containers
   running.

## 14. Acceptance Criteria

The feature is complete when:

- new verification email deliveries create an encrypted, one-to-one diagnostic
  snapshot before Tencent SES is called;
- Redis and the diagnostic snapshot use one auth-owned expiration deadline;
- neither consumption nor resend writes authentication lifecycle state to mail
  tables;
- authorized list and detail responses show identical plaintext code, time
  status, and expiration values;
- failed, pending, expired, historical, and test-mail projections follow the
  exact contract above;
- both read routes are protected by `system_mail_logView` and the frontend tab
  uses the same permission;
- no user-managed role grant is created or changed by migrations, seeds, local
  setup scripts, or runtime verification;
- schema, permission, API, route policy, generated frontend types, i18n, and
  architecture documentation remain synchronized;
- all focused and full verification gates pass;
- runtime behavior is exercised through `admin-dev` without an application
  Docker rebuild.
