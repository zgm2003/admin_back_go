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
9. `system_mail_logView` is a high-sensitivity permission because one list
   response can contain multiple plaintext codes. An authorized list or detail
   response is released only after a subject-bearing audit record succeeds,
   without recording response data, and every plaintext HTTP response is
   marked non-cacheable.

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
- versioned encryption metadata, previous-key reads, and an explicit rekey
  operation so `APP_SECRET` rotation does not strand retained snapshots;
- the `system_mail_logView` permission definition and route policies;
- route-level required-audit support for the two plaintext reads while
  preserving the existing fail-open default for all other OperationLog routes;
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
- changing the rotation behavior of encrypted credentials outside mail
  verification diagnostics;
- making operation-log persistence fail closed for routes that do not
  explicitly opt into required audit;
- rebuilding application Docker images for local verification.

If consumed or replacement history is requested later, it requires a separate
auth-owned audit design with explicit cross-store consistency and recovery
semantics. It must not be inferred from mail delivery data.

## 5. Chosen Architecture

The mail module adds one semantically immutable child snapshot per
verification-code mail log. Only its cryptographic envelope can later be
rewritten by the explicit rekey operation:

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
          -> versioned secretbox decrypt by key ID
          -> derive temporal status

explicit maintenance
  -> admin-db mail-diagnostic-rekey
      -> mail diagnostic rekey service/repository
          -> previous-key decrypt
          -> current-key encrypt
          -> bounded, resumable database updates
```

This keeps write and read ownership inside `internal/module/mail`. Auth passes
the code, configured TTL, and absolute expiration deadline through its existing
sender boundary, but it does not receive a mail-log ID or update mail data when
Redis state changes.

The narrow sender method keeps its configured `ttl time.Duration` input and
adds an `expiresAt time.Time` input. Primitives are used deliberately so mail
does not import an auth-owned DTO and auth does not import the mail module.

Runtime constructs two distinct cryptographic capabilities for Mail Service:
the existing current-key `secretbox.Box` remains responsible for configured
Tencent credentials, while a versioned diagnostic cipher handles only
verification snapshots. `internal/infra/secretkey` owns key derivation,
`internal/infra/secretbox` owns AES-GCM and key selection, and
`internal/module/mail` owns snapshot and rekey use cases. The
`cmd/admin-db` command is only an explicit maintenance entry point; it invokes
the mail-owned rekey service and does not issue ad hoc SQL against mail tables.

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
- A new independent diagnostic root secret would violate the platform's
  single-root key architecture and would still need its own versioning and
  rotation workflow. Versioned keys derived from current/previous
  `APP_SECRET` reuse the established rotation boundary.
- Deleting or nulling the ciphertext at expiration would remove the requested
  historical `expired` diagnostic view and introduce a cleanup dependency into
  this feature. Retention remains tied to the parent mail log instead.

## 6. Data Model

The new mail-owned table is `mail_log_verification_codes`:

| Column | Contract |
| --- | --- |
| `id` | `BIGINT UNSIGNED`, primary key, auto increment |
| `mail_log_id` | `BIGINT UNSIGNED NOT NULL`, unique one-to-one reference to `mail_logs.id` |
| `key_id` | `VARCHAR(64) NOT NULL`, non-secret diagnostic-key fingerprint |
| `code_enc` | `VARCHAR(255) NOT NULL`, diagnostic AES-GCM ciphertext only |
| `expires_at` | `DATETIME NOT NULL`, absolute authentication deadline |
| `created_at` | `DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP` |

The table has a unique key on `mail_log_id`, an index on `(key_id, id)` for
bounded rekey scans, and a foreign key to `mail_logs` that restricts physical
parent deletion. It has no `status`, general-purpose `updated_at`, or
independent `is_del` column:

- the mail-log link, expiration deadline, and decrypted code are semantically
  immutable;
- only the cryptographic representation (`key_id` and `code_enc`) may change,
  atomically and only through the explicit rekey operation;
- status is derived and must not become stale persisted data;
- it has no independent HTTP surface;
- visibility and soft deletion always follow the parent `mail_logs.is_del=2`
  query.

`code_enc` uses AES-GCM through `internal/infra/secretbox`. The diagnostic
cipher is versioned and domain-separated rather than using the current-only
`secretbox.Box` already injected for mail credentials:

- `internal/infra/secretkey` derives the dedicated purpose
  `admin_go:mail-verification-diagnostic:v1` for the current `APP_SECRET` and
  for the single allowed `APP_SECRET_PREVIOUS` value; it does not reuse the
  general `admin_go:secretbox:v1` credential key;
- each derived key has the stable, non-secret ID
  `mail-diagnostic-v1-<base64url(SHA-256(derived-key)[:16])>`, mirroring the
  existing JWT key-ID pattern and never hashing or exposing the root secret
  directly;
- runtime injects a diagnostic cipher whose encrypt operation always returns
  the current `key_id` and ciphertext and whose decrypt operation selects an
  exact current or previous key by `key_id`;
- an empty or unknown `key_id` is an explicit read error. Decryption never
  guesses keys and never treats failure as historical null.

Plaintext is never stored in `mail_logs`, the child table, error messages,
access logs, operation logs, maintenance output, or runtime logs.

### 6.1 Key Rotation And Rekeying

Retained diagnostic rows must survive normal `APP_SECRET` rotation. The
rotation sequence is therefore part of this feature's contract:

1. In the approved maintenance window, stage the new `APP_SECRET` with the old
   value as `APP_SECRET_PREVIOUS`, then drain and stop every API/Worker process
   that still treats the old secret as current. No verification-code writer may
   run during the rekey. This places rekeying before the new deployment's JWT
   dual-verification window rather than extending that window for database
   maintenance.
2. With the staged current/previous pair, run the explicit
   `go run ./cmd/admin-db mail-diagnostic-rekey` maintenance operation before
   starting new-current application processes. It acquires a named database
   lock and first validates that every
   distinct stored key ID is either current or the configured previous ID,
   failing before mutation if not. It then scans the previous ID through the
   `(key_id, id)` index in ascending-ID batches of at most 100 rows, decrypts
   with the identified previous key, re-encrypts with the current key, and
   updates `key_id` and `code_enc` together in one short transaction per batch.
   Each update compares the row's old key ID and ciphertext so a concurrent or
   resumed operation cannot overwrite a value it did not read. The scan covers
   every child row, including rows hidden by a soft-deleted parent.
3. The operation is idempotent and resumable. It emits row IDs and aggregate
   counts only, never ciphertext or plaintext, and exits non-zero on an
   unknown key ID, corrupt ciphertext, or failed update.
4. Only after the command records zero previous/unknown references may the
   new-current API/Worker processes start with the staged pair. New rows then
   use the current diagnostic key, while previous-key support remains available
   for the runbook's bounded JWT transition window.
5. The same recorded zero-reference result remains a precondition for removing
   `APP_SECRET_PREVIOUS` after the JWT/session and credential-rotation steps.
   Removing it while matching rows remain violates the rotation runbook and is
   not a supported degraded mode.

The rekey database lock prevents concurrent maintenance runs; it does not
replace the required writer outage. The command prints the non-secret current
key ID, and the runbook records that ID with the zero-reference verification
result. If rekeying fails, writers remain stopped until the idempotent command
successfully resumes or the operator executes the documented generation-
matched rollback; mixed-key rows are never handed to a current-only runtime.

Lazy re-encryption during list/detail reads is not sufficient because unread
historical rows could retain the previous key indefinitely. Existing mail
credential re-entry remains governed by the current secret-rotation runbook;
this operation changes only `mail_log_verification_codes`.

Rekeying the live database does not rewrite older backups. Database backups
remain paired with their matching deployment secret set under the existing
rotation and rollback policy. Restoring a pre-rekey backup requires starting
with that backup's matching `APP_SECRET` generation, then repeating the
dual-key rollout and diagnostic rekey before returning to the newer current
secret. A pre-rekey backup must never be opened by a new-current-only runtime.

Existing mail logs are not backfilled. A missing child row is valid historical
state and projects all three verification fields as JSON `null`.

## 7. Expiration Deadline And Delivery Flow

Auth and Redis use one absolute deadline for every verification-code delivery.
The store is shared by email and phone paths, so its expiration semantics must
not diverge by channel. Auth owns an injectable clock and reads it once for
deadline creation:

1. Validate account, scene, channel readiness, and captcha as today.
2. Load the configured verification TTL for the selected email or phone
   channel.
3. Generate the six-digit code.
4. Acquire the existing delivery lease for the verification cache key.
5. Compute `expires_at = now.Add(ttl).Truncate(time.Second)` immediately before
   pending Redis state is installed. Truncation deliberately ensures the
   authentication lifetime never exceeds the configured TTL and matches the
   project's second-precision `DATETIME` and HTTP time format.
6. Reject a non-future deadline and pass the absolute `expires_at`, not a
   relative duration, to `SetPendingDelivery`.
7. The pending-state Lua script receives `expires_at.UnixMilli()`, rejects a
   deadline that is not in the future according to Redis `TIME`, and installs
   the hash with `PEXPIREAT`. Transport and command latency therefore cannot
   extend the authentication deadline.
8. On the email path, pass the configured TTL and the same `expires_at` to
   `VerifyCodeMailSender`. The TTL remains the template's `ttl_minutes` input;
   `expires_at` is the diagnostic deadline.
9. Continue the existing send, lease renewal, commit, and rollback sequence.

The private `verifyCodeDeliveryStore.SetPendingDelivery` contract changes its
last argument from `time.Duration` to the absolute `time.Time` deadline for
both email and phone. Email additionally passes that value through the changed
`VerifyCodeMailSender` method for diagnostic persistence. The phone sender
keeps its current TTL-only capability and no SMS diagnostic record or API field
is introduced.

The delivery lease keeps its independent relative TTL and renewal behavior;
only the verification-code hash changes to an absolute expiry. API and Redis
hosts must have synchronized system clocks. A Redis clock that considers the
deadline already elapsed fails the send closed rather than creating a key with
ambiguous validity.

`expires_at` follows the repository's existing MySQL `loc=Local` convention,
and all API instances must use the same process time zone. Redis receives the
timezone-independent Unix millisecond value from that same `time.Time`; the
HTTP DTO uses the existing local, second-precision format. Temporal status is
always computed by the server and is not inferred by parsing browser-local
time.

The mail sender validates the existing scene/email rules, requires the code to
be exactly six ASCII digits, validates a positive configured TTL, and from one
injected clock read requires `now < expires_at <= now.Add(ttl)`. Runtime gives
Auth and Mail the same system-clock implementation; tests can inject
deterministic clocks. The sender then performs this sequence:

1. Resolve and validate the enabled config and scene template.
2. Encrypt the code with the current diagnostic key before starting database
   work, retaining both `key_id` and ciphertext.
3. In one short MySQL transaction, insert the pending `mail_logs` row and its
   one-to-one diagnostic snapshot.
4. Commit the transaction before the external Tencent SES call; no database
   transaction is held across network I/O.
5. Check the deadline again after commit and immediately before provider I/O.
   If it elapsed during local work, finalize the parent as failed, do not call
   Tencent SES, and return a sanitized deadline error so auth rolls back.
6. Call Tencent SES with the exact code and configured TTL template values
   using a child context whose deadline is the earlier of the incoming context
   deadline and `expires_at`.
7. Finalize only the parent mail log as success or failure using the existing
   delivery fields.

The repository exposes a dedicated atomic capability for this path, equivalent
to `CreateVerificationLog(parent, snapshot)`. It is not modeled as a generic
nullable child argument on every send. Only `SendVerifyCode` calls that
capability. `TestSend` continues to create and finalize only a parent
`mail_logs` row, never receives an auth-issued code or diagnostic deadline,
and therefore always projects the three diagnostic fields as null even when a
template sample contains a placeholder code.

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

`key_id` and `code_enc` are repository/service-only fields. They are never
published in `LogDTO`, generated OpenAPI schemas, frontend types, operation
logs, or error metadata.

Mail Service owns an injectable clock. List captures `now` exactly once for the
whole page and detail captures it once for the item, so rows in one response
cannot cross the deadline under different clock reads. Status is calculated
using this precedence:

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

A child snapshot is valid only when all of these invariants hold:

- parent status is exactly pending, success, or failed;
- `key_id` and `code_enc` are non-empty and the key ID is known;
- `expires_at` is non-zero and has second precision;
- decrypted plaintext is exactly six ASCII digits.

Unknown parent status, invalid metadata, invalid ciphertext, a missing
decryptor, or any other impossible joined record is an explicit internal
error. The API fails closed for the whole response instead of returning an
empty code that looks like a legitimate historical null. At the HTTP boundary,
the three verification properties are always either all null or all non-null.

## 9. RBAC Contract

The deterministic permission definition is:

```text
id        = 515
parent_id = 506                    # system_mail page
type      = 3                      # BUTTON
sort      = 9
name      = 查看邮件日志及验证码
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

The list and detail share one high-sensitivity permission because they expose
the same plaintext-code resource. Its role-management label explicitly names
verification codes so an administrator does not mistake it for ordinary
delivery-metadata access. Delete routes retain `system_mail_logDel`; neither
permission implicitly grants the other.

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
true. Tab initialization and any later RBAC snapshot change must choose an
available tab rather than retaining a hidden active tab. Mounting or
preloading the log panel is forbidden while permission is absent. Permission
loss aborts in-flight log requests, closes detail UI, clears list/detail state,
unmounts the panel, and then selects an available tab; a late response cannot
repopulate hidden state. Route deactivation or component unmount performs the
same abort-and-clear operation so `KeepAlive` cannot retain plaintext; a later
reactivation with permission performs a fresh request. API types continue to
be generated from the Admin Contract Bundle and validated by narrow frontend
decoders; `any`, silent fallbacks, and handwritten duplicate response
contracts are not introduced. The decoder enforces the all-null or
all-non-null tuple, the closed four-value status enum, the six-digit code shape,
and the existing second-precision time format.

The current detail notice that says verification codes are not stored is
updated to describe the authorized diagnostic behavior accurately. It must not
expose implementation secrets or present frontend visibility as the security
boundary.

## 11. Error Handling And Sensitive Data

- Ciphertext creation failure stops before database insertion and provider I/O.
- Snapshot transaction failure stops before provider I/O.
- Tencent SDK errors are converted at the provider boundary to a safe code and
  local fixed message. A code is retained only from the typed Tencent error,
  when it matches `[A-Za-z][A-Za-z0-9._-]{0,127}` and contains none of the
  sensitive template values supplied to the provider; otherwise it becomes
  `provider_error`. Persisted and returned messages come from local code-based
  mappings or the fixed generic `邮件服务调用失败`, never Tencent
  `GetMessage()`. An unknown sender error receives the same generic
  code/message. Raw SDK messages, causes, and `err.Error()` values never cross
  the provider adapter into persistence, HTTP errors, or runtime logs.
- Decryption failure returns an explicit mail diagnostic read error and no
  partial plaintext response.
- Permission denial occurs before the handler and returns the existing 403
  envelope.
- Both GET log routes are added to OperationLog under module `mail`: list uses
  action `list_logs` and title `查看邮件日志及验证码`; detail uses action
  `view_log` and title `查看单条邮件日志及验证码`. Both set
  `SkipRequestPayload=true`, `SkipResponsePayload=true`, and a new route-level
  `Required=true`. The record keeps principal, session, route, request ID,
  client IP, status, and latency but never a query body or response DTO.
  Request ID correlates the subject-bearing operation record with the exact
  path in AccessLog when resource-level review is needed.
- Required audit is fail closed only for these explicitly marked routes. The
  middleware holds the completed JSON response in a transient buffer capped at
  1 MiB without decoding or persisting it, writes the audit record, and
  releases the response only after recording succeeds. Audit failure or buffer
  overflow discards the buffered body and returns a no-store internal-error
  envelope containing no diagnostic data. Existing non-required operation
  routes retain their current fail-open behavior. The failure path emits a
  structured warning and counter with user ID, session ID, route, and request
  ID, but no request or response payload.
- AccessLog continues to omit request and response bodies.
- Both handlers set `Cache-Control: no-store, private` and
  `Pragma: no-cache` at handler entry, before query binding or service work, so
  every handler-produced success or error response carries the headers.
  Frontend storage restrictions supplement these HTTP controls; they are not
  the cache security boundary.
- Plaintext codes appear only in authenticated JSON response bodies, never in
  URLs, query strings, redirect targets, or headers. Any non-loopback
  deployment must terminate TLS at its ingress or reverse-proxy boundary;
  plain HTTP is limited to the loopback `admin-dev` workflow.
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
- the tracked Admin permission contract;
- `docs/runbooks/session-secret-rotation.md`, including the mandatory
  diagnostic rekey and old-key reference check before previous-key removal.

The migration creates the child table and permission definition but never
touches `role_permissions`. The deterministic permission seed grows from 131 to
132 rows and its non-empty unique permission-code count grows from 101 to 102.
Existing databases receive no fabricated diagnostic rows, and empty databases
receive the same permission definition through the canonical seed.

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
- command latency not extending the Redis deadline, a Redis-past deadline being
  rejected, and the pending hash using `PEXPIREAT` rather than `PEXPIRE`;
- email and phone deliveries both using the absolute pending-store contract,
  while the phone sender signature and absence of SMS diagnostics remain
  unchanged;
- successful creation of parent and encrypted child rows in one transaction;
- no provider call when encryption or the creation transaction fails;
- no provider call when the deadline expires before I/O and provider context
  cancellation at the diagnostic deadline when a call is still in flight;
- plaintext absence from persisted columns, errors, and non-authorized internal
  projections; outside the transient Auth-to-Mail-to-provider delivery path,
  plaintext is projected only by the two authorized HTTP reads;
- provider success/failure finalization leaving the child snapshot's code and
  deadline unchanged;
- list and detail joining and decrypting the same code;
- `null` projections for test sends and historical logs;
- all four status values and the exact `now == expires_at` boundary;
- expired precedence for an old pending row and failed precedence for a failed
  delivery;
- explicit failure for corrupt ciphertext;
- deterministic diagnostic key IDs, current/previous derivation, and proof
  that the diagnostic HKDF purpose differs from the general credential
  secretbox key;
- current-key writes, previous-key reads, unknown-key failure, idempotent and
  resumable diagnostic rekeying, and a zero-reference proof before the runbook
  permits previous-key removal;
- malicious sender errors containing the code or template JSON being replaced
  by fixed safe provider metadata in persistence, returned errors, and logs;
- both GET routes requiring `system_mail_logView` and denying a principal that
  has only `system_mail` or `system_mail_logDel`;
- both GET routes producing subject-bearing OperationLog records with no
  captured payload and returning `no-store` headers without writing plaintext
  to either log stream;
- required-audit recorder failure discarding the buffered list/detail body and
  returning a no-store error with no code, the 1 MiB overflow boundary, and
  ordinary audited routes remaining fail open;
- permission seed identity, parent, count, and the absence of persistent
  `role_permissions` writes;
- Atlas validation, canonical-schema drift checks, contract generation, route
  policy goldens, and architecture gates;
- existing Redis integration tests for matching consumption, mismatches,
  pending state, resend replacement, and concurrent ownership.

Frontend red-green coverage includes:

- strict decoding of nullable code fields and the closed status enum;
- rejection of partial-null diagnostic tuples, malformed codes, malformed
  expiration values, and unknown statuses;
- list columns and detail fields using the same values;
- Chinese status dictionary labels without raw translation keys;
- `-` rendering for historical and test logs;
- sending-log tab visibility for granted and ungranted users;
- a valid fallback active tab when sending logs are hidden;
- no request to either log endpoint when the tab is unavailable;
- permission loss aborting in-flight reads, clearing plaintext state, closing
  detail UI, and preventing a late response from repopulating the panel;
- route deactivation/unmount clearing plaintext state and reactivation issuing
  a fresh authorized request rather than restoring `KeepAlive` data;
- generated operation and schema type checks after bundle synchronization.

Verification sequence:

1. Run focused backend and frontend tests during implementation.
2. Run the repositories' full host-side quality, unit, contract, locale, and
   architecture gates supported by the fixed toolchains.
3. Exercise current-, previous-, unknown-, and corrupt-key rekey cases only in
   a disposable integration schema; do not fabricate old-key rows in the
   user's persistent local state database.
4. Apply the non-destructive Atlas migration explicitly to the local Docker
   state database, then run the rekey command there as a current-key/empty-table
   no-op validation.
5. Start API, worker, and Vite through `admin-dev`; do not rebuild application
   Docker images.
6. Before manual role assignment, verify the tab is hidden and direct list and
   detail requests return 403.
7. Automated route/service/component tests provide the authorized-path proof
   without mutating local role grants. After the user assigns the permission,
   verify one real email code appears identically in list and detail and shows
   the expected expiration deadline.
8. Confirm authorized reads emit payload-free operation records and `no-store`
   response headers, then stop `admin-dev` cleanly while leaving MySQL and
   Redis state containers running.

## 14. Acceptance Criteria

The feature is complete when:

- new verification email deliveries create an encrypted, one-to-one diagnostic
  snapshot before Tencent SES is called;
- Redis and the diagnostic snapshot use one auth-owned expiration deadline;
- Redis command latency cannot extend that deadline and database/API precision
  matches it exactly;
- current and previous diagnostic keys are selected by key ID, the explicit
  rekey operation is idempotent, and a recorded zero-reference result is
  required before previous-key removal;
- neither consumption nor resend writes authentication lifecycle state to mail
  tables;
- authorized list and detail responses show identical plaintext code, time
  status, and expiration values;
- failed, pending, expired, historical, and test-mail projections follow the
  exact contract above;
- both read routes are protected by `system_mail_logView` and the frontend tab
  uses the same permission;
- every authorized plaintext response is released only after a successful
  payload-free audit record, and every response produced by the diagnostic
  handlers is marked `no-store`;
- provider, persistence, HTTP, access-log, operation-log, runtime-log, test,
  and maintenance error paths never contain plaintext codes;
- no user-managed role grant is created or changed by migrations, seeds, local
  setup scripts, or runtime verification;
- schema, permission, API, route policy, generated frontend types, i18n, and
  architecture documentation remain synchronized;
- all focused and full verification gates pass;
- runtime behavior is exercised through `admin-dev` without an application
  Docker rebuild.
