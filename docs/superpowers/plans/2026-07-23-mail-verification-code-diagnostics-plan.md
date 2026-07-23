# Mail Verification Code Diagnostics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans`. Execute one task at a time; every task has RED, GREEN, and commit checkpoints.

**Goal:** Add an RBAC-protected, encrypted mail verification-code diagnostic snapshot that shares Auth's absolute Redis deadline, supports explicit rekeying, and exposes plaintext only through the two audited mail-log reads.

**Architecture:** Auth remains the verification authority. It computes one second-precision absolute deadline and passes it through `VerifyCodeMailSender`; Redis uses `PEXPIREAT`, while Mail stores an immutable one-to-one encrypted child. A versioned AES-GCM box derives current/previous keys from the single `APP_SECRET` root. Both GET routes require `system_mail_logView`, no-store headers, and payload-free required audit before response release.

**Repositories:** Backend `E:\admin\admin_back_go`; frontend `E:\admin\admin_front_ts`.

**Tech Stack:** Go 1.25, Gin, GORM, MySQL 8, Redis Lua, AES-GCM, HKDF-SHA256, Atlas, Vue 3, TypeScript, Vitest, Element Plus, Pinia, generated Admin Contract Bundle, PowerShell.

---

## Non-negotiable Decisions

- `APP_SECRET` is the only current root. Do not add `MAIL_DIAGNOSTIC_SECRET` or any other deploy-time secret.
- `APP_SECRET_PREVIOUS` is empty normally and contains at most one old root during rotation. It supplies previous JWT verification and previous diagnostic decryption only.
- HKDF purposes stay separate: `admin_go:jwt-signing:v1`, `admin_go:token-pepper:v1`, `admin_go:secretbox:v1`, and `admin_go:mail-verification-diagnostic:v1`.
- Remove only the unused derived `admin_go:session-cache:v1` capability. Keep Auth's live Redis `sessionCacheKey()` formatter and session cache.
- Redis remains the authorization source. The MySQL child has no consumed/replaced/superseded state and is never queried by authentication.
- No migration, seed, runtime setup, smoke test, or `admin-dev` path writes `role_permissions`.
- Migrations are explicit Atlas operations; application startup and `admin-dev` never migrate.
- Plaintext exists only in the Auth-to-Mail-to-provider call path and authorized response bodies. It must not enter errors, logs, URLs, headers, command output, browser storage, or operation payloads.

## Change Map

Backend cryptography/time: `internal/infra/secretkey`, `internal/infra/secretbox/versioned.go`, `internal/shared/clock`.

Auth: `internal/module/auth/service.go`, `code_store.go`, and their tests.

Mail: `internal/module/mail/model.go`, `dto.go`, `repository.go`, `service.go`, `errors.go`, `rekey.go`, `rekey_repository.go`, and tests; Tencent adapter under `internal/infra/mail/tencentcloudses`.

Runtime/HTTP: `internal/runtime/providers.go`, `internal/platform/admin/build.go`, `internal/server/adminroute`, `internal/middleware/operation_log.go`, and Mail admin transport.

Database/contracts/docs: the forward Atlas migration, schema/reconciliation files, permission seed, backend contract bundle, runbook, and architecture docs.

Frontend: `src/api/system/mail.ts`, mail views/components, generated contract/locale files, and shared/component tests in `E:\admin\admin_front_ts`.

---

## Task 1: Remove the Unused `session-cache:v1` Capability

**Files:**
- Modify `internal/infra/secretkey/secretkey.go`
- Modify `internal/infra/secretkey/secretkey_test.go`

- [ ] **Step 1: RED source-contract test**

Read `secretkey.go` and reject `sessionCacheKey`, `SessionCacheKey`, and `admin_go:session-cache:v1`. Read `../../module/auth/session_cache.go` and require the live `sessionCacheKey` method still exists. Report only the fixed forbidden token, never key material.

- [ ] **Step 2: Confirm RED**

Run `go test ./internal/infra/secretkey -run TestKeyRingDoesNotDeriveSessionCacheCapability -count=1`; it must fail on the current dead derivation.

- [ ] **Step 3: Implement**

Delete only the `KeyRing` field, constructor derivation/assignment, and `SessionCacheKey()` accessor. Do not touch `internal/module/auth`.

- [ ] **Step 4: GREEN**

```powershell
go test ./internal/infra/secretkey -count=1
rg -n "SessionCacheKey|admin_go:session-cache:v1|sessionCacheKey\s+\[\]byte" internal/infra/secretkey cmd
rg -n "sessionCacheKey" internal/module/auth/session_cache.go internal/module/auth/session_lifecycle.go
```

The first scan is empty; the second still finds the live formatter.

- [ ] **Step 5: Commit**

```powershell
git add internal/infra/secretkey/secretkey.go internal/infra/secretkey/secretkey_test.go
git commit -m "refactor: remove unused session cache derivation"
```

## Task 2: Add Versioned Diagnostic Derivation and Exact-Key AES-GCM

**Files:**
- Modify `internal/infra/secretkey/secretkey.go`, `secretkey_test.go`
- Create `internal/infra/secretbox/versioned.go`, `versioned_test.go`

- [ ] **Step 1: RED tests**

Cover deterministic diagnostic IDs, current plus one previous key, purpose separation from `SecretboxKey`, cloned accessors, current-only encryption, exact-ID decryption, empty/unknown IDs, malformed keys, and corrupt ciphertext. Key IDs are `mail-diagnostic-v1-` plus base64url(SHA-256(derived key) first 16 bytes).

- [ ] **Step 2: Confirm RED**

Run `go test ./internal/infra/secretkey ./internal/infra/secretbox -count=1`; it must fail because the diagnostic methods and `VersionedBox` are absent.

- [ ] **Step 3: Implement**

Extend `KeyRing` with cloned current diagnostic key/ID and a cloned current/previous map. Reject duplicate roots and diagnostic ID collisions. Add:

```go
const mailDiagnosticPurpose = "admin_go:mail-verification-diagnostic:v1"

func (k *KeyRing) MailDiagnosticKey() []byte
func (k *KeyRing) MailDiagnosticKeyID() string
func (k *KeyRing) MailDiagnosticDecryptionKeys() map[string][]byte
```

Create:

```go
var (
	ErrMissingCurrentKeyID = errors.New("secretbox: current key ID is required")
	ErrUnknownKeyID        = errors.New("secretbox: unknown key ID")
)

type VersionedBox struct { currentKeyID string; boxes map[string]Box }
func NewVersioned(currentKeyID string, keys map[string][]byte) (VersionedBox, error)
func (b VersionedBox) CurrentKeyID() string
func (b VersionedBox) Encrypt(plain string) (keyID, ciphertext string, err error)
func (b VersionedBox) Decrypt(keyID, ciphertext string) (string, error)
```

`NewVersioned` trims IDs, requires a current entry, rejects duplicate-after-trim IDs and non-32-byte keys, and clones bytes. Encrypt always selects current. Decrypt selects exactly one ID and never probes another key after lookup/authentication failure.

- [ ] **Step 4: GREEN**

Run `go test ./internal/infra/secretkey ./internal/infra/secretbox -count=1`.

- [ ] **Step 5: Commit**

```powershell
git add internal/infra/secretkey internal/infra/secretbox/versioned.go internal/infra/secretbox/versioned_test.go
git commit -m "feat: derive versioned mail diagnostic keys"
```

## Task 3: Share One Absolute Deadline Across Auth, Redis, and Email

**Files:**
- Create `internal/shared/clock/clock.go`, `clock_test.go`
- Modify `internal/module/auth/service.go`, `service_test.go`, `code_store.go`, `code_store_test.go`

- [ ] **Step 1: RED tests**

Create `internal/shared/clock`. Test `Clock`, `Func`, and `SystemClock`. Extend Auth fakes with `pendingExpiresAt` and email `expiresAt`; assert one injected clock read and identical `now.Add(ttl).Truncate(time.Second)` values. Keep phone sender TTL-only. Add Redis integration cases for command latency, a past deadline, and Lua source containing `TIME` plus `PEXPIREAT` but no pending-key `PEXPIRE`.

- [ ] **Step 2: Confirm RED**

```powershell
go test ./internal/shared/clock ./internal/module/auth -run "Test(Func|SystemClock|SendCodeUsesOneSecondPrecisionDeadline|RedisCodeStoreSetPendingAbsoluteDeadline|PhonePathUsesAbsolutePendingDeadline)" -count=1
```

- [ ] **Step 3: Implement**

```go
type Clock interface { Now() time.Time }
type Func func() time.Time
func (f Func) Now() time.Time { if f == nil { return time.Now() }; return f() }
type SystemClock struct{}
func (SystemClock) Now() time.Time { return time.Now() }
```

Add `clock clock.Clock` to Auth, default it to `clock.SystemClock{}`, and expose `WithClock`. Change only the pending-store contract:

```go
SetPendingDelivery(context.Context, verifyCodeDeliveryLease, string, string, time.Time) error
```

Immediately before pending installation, read the clock once, compute `expiresAt := now.Add(ttl).Truncate(time.Second)`, reject non-future values, and pass the absolute time to Redis. Lua checks Redis `TIME`, returns `-1` for elapsed deadlines, and calls `PEXPIREAT` with Unix milliseconds. Map `-1` to fixed `errVerificationDeadlineElapsed`.

Change only email:

```go
SendVerifyCode(ctx context.Context, scene, toEmail, code string, ttl time.Duration, expiresAt time.Time) *apperror.Error
```

`sendVerifyCodeWithDeliveryRenewal` receives `expiresAt` and forwards it only in the email branch. Phone and lease acquire/renew signatures remain unchanged.

- [ ] **Step 4: GREEN**

Run `go test ./internal/shared/clock ./internal/module/auth -count=1`. Redis tests may use the repository's existing explicit skip only when Redis is unavailable.

- [ ] **Step 5: Commit**

```powershell
git add internal/shared/clock internal/module/auth/service.go internal/module/auth/service_test.go internal/module/auth/code_store.go internal/module/auth/code_store_test.go
git commit -m "feat: share absolute verification deadline"
```

## Task 4: Add the Immutable Mail Child and Atomic Repository APIs

**Files:**
- Modify `internal/module/mail/model.go`, `repository.go`, `repository_test.go`, `service.go`, `service_test.go`

- [ ] **Step 1: RED repository tests**

Using existing GORM `sqlmock` helpers, require one transaction for parent plus child, rollback on either insert error, exact left-join aliases, parent soft-delete filtering, historical nil child, preservation of partial joins, and `FinishLog` updating only `mail_logs`. Prove `CreateLog` used by `TestSend` never inserts a child.

- [ ] **Step 2: Confirm RED**

Run `go test ./internal/module/mail -run "Test(CreateVerificationLog|RepositoryListLogRows|RepositoryLogRowByID|FinishLogDoesNotMutateSnapshot)" -count=1`.

- [ ] **Step 3: Implement**

Add `VerificationCodeSnapshot` with `id`, `mail_log_id`, `key_id`, `code_enc`, `expires_at`, and `created_at`. Add `LogReadRow` embedding `Log` plus nullable joined ID/key/cipher/expiry fields.

Replace `ListLogs`/`LogByID` in `Repository` with `ListLogRows`/`LogRowByID`, and add:

```go
CreateVerificationLog(context.Context, Log, VerificationCodeSnapshot) (uint64, error)
```

Use one `db.Transaction` for both inserts. List/detail share:

```sql
mail_logs.*,
mvc.id AS verification_snapshot_id,
mvc.key_id AS verification_key_id,
mvc.code_enc AS verification_code_enc,
mvc.expires_at AS verification_expires_at
```

Join `mail_log_verification_codes AS mvc` by parent ID and filter only `mail_logs.is_del = 2`. Count from `mail_logs`. Keep finalization, soft delete, and test-send creation parent-only.

- [ ] **Step 4: GREEN**

Run `go test ./internal/module/mail -run "Test(CreateVerificationLog|RepositoryListLogRows|RepositoryLogRowByID|FinishLog|CreateLog|ServiceLogs|ServiceLog)" -count=1`.

- [ ] **Step 5: Commit**

```powershell
git add internal/module/mail/model.go internal/module/mail/repository.go internal/module/mail/repository_test.go internal/module/mail/service.go internal/module/mail/service_test.go
git commit -m "feat: persist encrypted mail code snapshots"
```

## Task 5: Bound Delivery and Sanitize Provider Errors

**Files:**
- Modify `internal/module/mail/service.go`, `service_test.go`, `errors.go`
- Modify `internal/infra/mail/tencentcloudses/client.go`, `client_test.go`

- [ ] **Step 1: RED tests**

Assert encryption and child commit precede Tencent I/O; expiry after commit skips I/O; provider context uses the earlier incoming/deadline value; all finalization combinations preserve the primary provider failure and never mutate the child. Cover invalid ASCII code, TTL/deadline bounds, encryption/transaction failure, and `TestSend` isolation. Inject typed, unknown, malformed, serialization, empty-response, and client-construction errors whose raw text contains sensitive values; assert no raw text, cause, or `Unwrap` crosses the adapter.

- [ ] **Step 2: Confirm RED**

Run `go test ./internal/module/mail ./internal/infra/mail/tencentcloudses -run "Test(SendVerifyCode|TestSendDoesNotCreateVerificationSnapshot|WrapSendError|SanitizeSenderError)" -count=1`.

- [ ] **Step 3: Implement**

```go
type ServiceDependencies struct {
	Repository Repository
	CredentialBox secretbox.Box
	DiagnosticBox secretbox.VersionedBox
	Sender Sender
	Clock clock.Clock
}
func NewServiceWithDependencies(ServiceDependencies) *Service
```

Keep boxes separate; default nil clock to `SystemClock`. `SendVerifyCode` validates `^[0-9]{6}$`, positive TTL, second-precision future expiry, and `expiresAt <= now+ttl`. Encrypt before `CreateVerificationLog`; commit before provider I/O. Recheck immediately before I/O and finalize fixed `verification_deadline_elapsed` metadata when elapsed. Bound Tencent with the earlier context/deadline. `TestSend` stays parent-only.

Add fixed safe errors including `ErrDiagnosticCipherNotConfigured` and `ErrInvalidDiagnosticSnapshot`. Replace Tencent error with:

```go
type SendError struct{ Code string }
func (e SendError) Error() string { return "邮件服务调用失败" }
func (e SendError) ErrorCode() string { return e.Code }
```

Accept a code only from a direct typed Tencent error, matching `^[A-Za-z][A-Za-z0-9._-]{0,127}$` and containing none of SecretID, SecretKey, or TemplateData values. All other failures use `provider_error`. Never use `errors.As`, `Unwrap`, raw `Error()`, or Tencent `GetMessage()` for sanitization. Persist/return only safe code plus fixed message. Add injectable SDK client factory and template encoder seams; tests never call network.

- [ ] **Step 4: GREEN**

Run `go test ./internal/module/mail ./internal/infra/mail/tencentcloudses -count=1`.

- [ ] **Step 5: Commit**

```powershell
git add internal/module/mail internal/infra/mail/tencentcloudses/client.go internal/infra/mail/tencentcloudses/client_test.go
git commit -m "feat: bound and sanitize verification mail delivery"
```

## Task 6: Build the Strict Nullable Projector and Four Statuses

**Files:**
- Modify `internal/module/mail/dto.go`, `service.go`, `service_test.go`, `errors.go`

- [ ] **Step 1: RED tests**

Table-test precedence: failed first; exact deadline expired; old pending expired; future pending sending; future success not-expired. Test historical all-null, current/previous all-present, partial joins, unknown parent status, zero/subsecond expiry, unknown key, corrupt ciphertext, non-six-digit plaintext, and missing cipher. Any invalid row fails the whole response without key ID, ciphertext, or code in the error.

- [ ] **Step 2: Confirm RED**

Run `go test ./internal/module/mail -run "Test(VerificationCodeStatusPrecedence|LogDTOFromReadRow)" -count=1`.

- [ ] **Step 3: Implement**

Add required-but-nullable DTO fields:

```go
VerificationCode *string `json:"verification_code"`
VerificationCodeStatus *string `json:"verification_code_status"`
VerificationCodeExpiresAt *string `json:"verification_code_expires_at"`
```

Add constants `sending`, `not_expired`, `expired`, `send_failed` and dictionary labels `发送中`, `未过期`, `已过期`, `发送失败`. Implement `logDTOFromReadRow(row, now)`. Historical absence requires every joined pointer including snapshot ID nil; partial tuples are invalid. Require known non-empty key/cipher, second precision, exact-ID decrypt, and six ASCII digits. Assign pointers only after validation. Capture `now` once per list page/detail item. Keep public reads parent-only until Task 9.

- [ ] **Step 4: GREEN**

Run `go test ./internal/module/mail -count=1`.

- [ ] **Step 5: Commit**

```powershell
git add internal/module/mail/dto.go internal/module/mail/service.go internal/module/mail/service_test.go internal/module/mail/errors.go
git commit -m "feat: add strict mail diagnostic projector"
```

## Task 7: Add Bounded, Resumable Mail-Owned Rekeying

**Files:**
- Create `internal/module/mail/rekey.go`, `rekey_repository.go`, `rekey_test.go`
- Modify `internal/module/mail/errors.go`
- Modify `cmd/admin-db/main.go`, `main_test.go`

- [ ] **Step 1: RED tests**

Cover current-only no-op, previous conversion in ascending batches up to 100, unknown-ID preflight with zero mutation, rows behind soft-deleted parents, corrupt ciphertext, optimistic rollback, resume after observer/output failure, idempotent rerun, lock contention, and zero previous/unknown references. CLI tests cover no extra args, required env, one optional previous root, safe failures/output, and no secret/plaintext/ciphertext output.

- [ ] **Step 2: Confirm RED**

Run `go test ./internal/module/mail ./cmd/admin-db -run "Test(DiagnosticRekey|MailDiagnosticRekeyCommand)" -count=1`.

- [ ] **Step 3: Implement**

Add fixed sentinels for repository-not-configured/failure, lock, unknown key, corrupt cipher, compare failure, and output failure. Define:

```go
const DefaultDiagnosticRekeyBatchSize = 100
type DiagnosticCipherRow struct { ID uint64; KeyID, CodeEnc string }
type DiagnosticCipherRewrite struct { ID uint64; OldKeyID, OldCodeEnc, NewKeyID, NewCodeEnc string }
type DiagnosticRekeyObserverFunc func(uint64) error

type DiagnosticRekeyRepository interface {
	WithDiagnosticRekeyLock(context.Context, string, func(DiagnosticRekeyRepository) error) error
	DistinctDiagnosticKeyIDs(context.Context) ([]string, error)
	ListDiagnosticCipherRows(context.Context, string, uint64, int) ([]DiagnosticCipherRow, error)
	RewriteDiagnosticCipherBatch(context.Context, []DiagnosticCipherRewrite) error
	CountDiagnosticKeyID(context.Context, string) (int64, error)
	CountUnknownDiagnosticKeyIDs(context.Context, []string) (int64, error)
}
```

`Run` validates current ID/cipher, preflights every distinct ID, and treats empty previous as current-only. For each previous batch: exact-ID decrypt, six-digit validation, current encrypt, one transactional optimistic update, then advance/notify only after commit. Observer failure stops after committed work and is resumable. Never join/filter parents.

GORM repository uses `database.Client.Gorm`. Acquire/release `admin_go:mail-diagnostic-rekey:v1` on one pinned connection and verify both `GET_LOCK`/`RELEASE_LOCK` results. Use parameterized scan:

```sql
SELECT id,key_id,code_enc FROM mail_log_verification_codes
WHERE key_id=? AND id>? ORDER BY key_id ASC,id ASC LIMIT ?
```

Add `mail-diagnostic-rekey` to `cmd/admin-db`: no args; read `MYSQL_DSN`, `APP_SECRET`, optional `APP_SECRET_PREVIOUS` through injected `getenv`; validate DSN/runtime roots; invoke injected runner. Production builds KeyRing, one-connection DB client, VersionedBox, GORM repository, and Mail service. Output only `current_key_id`, `previous_key_id`, `scanned`, `rekeyed`, `previous_references`, `unknown_references`, and committed `rekeyed_row_id` lines. Fixed safe errors only.

- [ ] **Step 4: GREEN**

Run focused tests, then `go test -race ./internal/module/mail -run TestDiagnosticRekey -count=1`.

- [ ] **Step 5: Commit**

```powershell
git add internal/module/mail/rekey.go internal/module/mail/rekey_repository.go internal/module/mail/rekey_test.go internal/module/mail/errors.go cmd/admin-db/main.go cmd/admin-db/main_test.go
git commit -m "feat: add mail diagnostic rekey command"
```

## Task 8: Wire the Single-Root Capability Through Runtime

**Files:**
- Modify `internal/runtime/providers.go`, `providers_test.go`, `api_test.go`, `worker_test.go`
- Modify `internal/platform/admin/build.go`, `build_test.go`
- Modify `internal/module/mail/service.go`, `service_test.go`, `readiness_test.go`

- [ ] **Step 1: RED composition tests**

Assert current-only/current-previous construction, diagnostic/credential purpose separation, one shared clock passed to Auth/Mail, and absence of `MAIL_DIAGNOSTIC_SECRET`.

- [ ] **Step 2: Confirm RED**

Run `go test ./internal/runtime ./internal/platform/admin -run "Test(BuildProvidersBuildsDedicatedMailDiagnosticBox|BuildWiresMailDiagnosticDependencies|BuildUsesSingleVerificationClock)" -count=1`.

- [ ] **Step 3: Implement**

Add `MailDiagnosticBox secretbox.VersionedBox` beside `Secretbox` in runtime/platform provider sets. Build it only from KeyRing diagnostic methods. In Admin Build instantiate one `clock.SystemClock{}` and inject it plus both boxes into Mail; pass the same clock to Auth. Thread the provider through API hooks; Worker may carry it but never writes snapshots/rekeys.

Migrate every Mail constructor call, including readiness and TestSend fixtures, to `NewServiceWithDependencies` with an explicit test diagnostic box. Remove positional `mail.NewService` and scan for remaining calls or credential-box substitution.

- [ ] **Step 4: GREEN**

Run `go test ./internal/runtime ./internal/platform/admin ./internal/module/mail ./internal/module/auth -count=1`.

- [ ] **Step 5: Commit**

```powershell
git add internal/runtime/providers.go internal/runtime/providers_test.go internal/runtime/api_test.go internal/runtime/worker_test.go internal/platform/admin/build.go internal/platform/admin/build_test.go internal/module/mail/service.go internal/module/mail/service_test.go internal/module/mail/readiness_test.go
git commit -m "feat: wire mail diagnostic runtime dependencies"
```

## Task 9: Require Payload-Free Audit Before Diagnostic Response Release

**Files:**
- Modify `internal/server/adminroute/definition.go`, `compile.go`, `registry.go`, and tests
- Modify `internal/middleware/operation_log.go`, `operation_log_test.go`
- Modify Mail admin `route.go`, `handler.go`, tests, and Mail `service.go`/tests
- Create `internal/module/mail/transport/admin/route_test.go`

- [ ] **Step 1: RED tests**

Lock both GET routes to `system_mail_logView`, module `mail`, actions `list_logs`/`view_log`, exact Chinese titles, both skip-payload flags, and `Required=true`. Test required success, recorder failure, nil recorder, exact 1 MiB success/overflow, required error responses, staged headers/status/flush, optional-interface safety, and ordinary fail-open behavior. Test no-store headers on all handler paths. Principals with only `system_mail` or `system_mail_logDel` must receive 403 before handler.

- [ ] **Step 2: Confirm RED**

Run `go test ./internal/server/adminroute ./internal/middleware ./internal/module/mail ./internal/module/mail/transport/admin -run "Test(MailLogReadsRequirePermissionAndAudit|OperationLogRequired|LogsDiagnosticProjection|LogDiagnosticProjection|HandlerLogsNoStore|HandlerLogNoStore)" -count=1`.

- [ ] **Step 3: Implement**

Add `Required bool` to audit metadata and `OperationRule`; reject required-without-enabled and preserve it through compiler/legacy adapters. Required rules use a private `1 << 20` byte writer that clones headers and stages status/body/flush without forwarding. It rejects `Hijack` and exposes no output-capable underlying interfaces. Build a payload-free `OperationInput`, record, restore underlying writer, then release. Recorder failure/overflow discards staged body and writes fixed no-store internal error. Overflow first records a failed subject-bearing audit with status 500. Ordinary rules retain fail-open behavior.

Set `Cache-Control: no-store, private` and `Pragma: no-cache` at entry of both handlers. In the same commit switch public list/detail to Task 6's projector, capture one clock time per response, and return no partial list on invalid data. Keep access/operation logs payload-free.

- [ ] **Step 4: GREEN**

Run all four packages and `go test -race ./internal/middleware -run TestOperationLogRequired -count=1`.

- [ ] **Step 5: Commit**

```powershell
git add internal/server/adminroute internal/middleware/operation_log.go internal/middleware/operation_log_test.go internal/module/mail/service.go internal/module/mail/service_test.go internal/module/mail/transport/admin
git commit -m "feat: require audit for mail diagnostic reads"
```

## Task 10: Evolve Schema, Permission Truth, and Rotation Docs

**Files:**
- Create `database/migrations/202607230101_mail_verification_code_diagnostics.sql`
- Modify Atlas sum, `database/schema/admin.hcl`, reconciliation 030/031
- Modify permission seed/README, architecture tests/docs, rotation runbook, `deploy/docker-first/admin-go.env.example`
- Verify-only `deploy/docker-first/admin-go.env`; never edit/stage this untracked secret-bearing file

- [ ] **Step 1: RED tests**

Expect 132 seed rows, 102 codes, and exact permission 515 (parent 506, BUTTON, sort 9, `system_mail_logView`), with no role grants. Assert six child columns, no status/updated_at/is_del, unique parent, `(key_id,id)`, restrict FK, migration/seed parity, and no `role_permissions` write. Require docs to name the diagnostic purpose and zero-reference gate.

- [ ] **Step 2: Confirm RED**

Run architecture tests and Atlas validation; architecture tests fail before the migration/docs exist.

- [ ] **Step 3: Implement**

Create exactly:

```sql
CREATE TABLE `mail_log_verification_codes` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `mail_log_id` BIGINT UNSIGNED NOT NULL,
  `key_id` VARCHAR(64) NOT NULL,
  `code_enc` VARCHAR(255) NOT NULL,
  `expires_at` DATETIME NOT NULL,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_mail_log_verification_codes_mail_log` (`mail_log_id`),
  KEY `idx_mail_log_verification_codes_key_id_id` (`key_id`, `id`),
  CONSTRAINT `fk_mail_log_verification_codes_mail_log`
    FOREIGN KEY (`mail_log_id`) REFERENCES `mail_logs` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO `permissions`
(`id`,`name`,`path`,`icon`,`parent_id`,`component`,`platform`,`type`,`sort`,`code`,`i18n_key`,`show_menu`,`status`,`is_del`)
VALUES (515,'查看邮件日志及验证码','','',506,NULL,'admin',3,9,'system_mail_logView','',2,1,2);
```

Mirror HCL and append exact table/column/index/FK/orphan invariants to reconciliation files. Add seed row 515 after 514; never touch `role_permissions`. Hash only through Atlas.

Update runbook: drain/stop old-current writers; stage new current plus previous; run rekey before new-current API/Worker; require zero previous/unknown references; then start processes; retain evidence before previous removal; pair backups with their secret generation. Update docs for ownership/plaintext/audit/TLS and the env example's live derivation list. Do not modify env values.

- [ ] **Step 4: GREEN**

```powershell
pwsh -NoProfile -File scripts/database/atlas.ps1 migrate hash --dir file://database/migrations
pwsh -NoProfile -File scripts/database/atlas.ps1 migrate validate --dir file://database/migrations
go test ./internal/architecture -run "Test(LocalPermissionSeed|DatabaseLayout|DatabaseBaseline|MailVerificationDiagnosticDocumentation)" -count=1
rg -n -i "(insert into|update|delete from|replace into|truncate table).*role_permissions" database/migrations/202607230101_mail_verification_code_diagnostics.sql database/seeds/admin_permissions.sql
```

The first three pass; the scan is empty.

- [ ] **Step 5: Commit**

```powershell
git add database/migrations/202607230101_mail_verification_code_diagnostics.sql database/migrations/atlas.sum database/schema/admin.hcl database/reconciliation/030_verify_schema.sql database/reconciliation/031_verify_relations.sql database/seeds internal/architecture docs/runbooks/session-secret-rotation.md CONTEXT.md docs/architecture.md internal/infra/README.md internal/module/README.md docs/contracts/admin-v1-runtime-model-contracts.md deploy/docker-first/admin-go.env.example
git commit -m "feat: define mail diagnostic schema and permission"
```

## Task 11: Publish Backend Route Policy and API Contract

**Files:**
- Modify `internal/admincontract/manifest.go` and tests
- Modify `internal/server/testdata/admin_route_policy_golden.json`
- Regenerate `contracts/admin/v1/manifest.json`, `openapi.json`, `permissions.json`; verify views/realtime unchanged

- [ ] **Step 1: RED contract tests**

Require catalog count 102 and `system_mail_logView`; both mail GET operations carry permission, required audit, skip flags, titles/actions. Require all three diagnostic properties in schema `required` while nullable; close status to four strings; reject key/cipher/template data from artifacts.

- [ ] **Step 2: Confirm RED**

Run `go test ./internal/admincontract ./internal/server -run "Test(PermissionsCatalogAndOperationPoliciesAreComplete|OpenAPI|Bundle|RoutePolicyGolden)" -count=1`.

- [ ] **Step 3: Regenerate**

Bump only bundle/permission versions. Regenerate route policy, then bundle using current committed backend revision:

```powershell
$backendCommit = (git rev-parse HEAD).Trim()
$env:UPDATE_ADMIN_ROUTE_POLICY_GOLDEN = '1'
go test ./internal/server -run TestRoutePolicyGoldenIsAdminOnlyAndCurrent -count=1
Remove-Item Env:UPDATE_ADMIN_ROUTE_POLICY_GOLDEN
pwsh -NoProfile -File scripts/generate-admin-contract.ps1 -BackendCommit $backendCommit
pwsh -NoProfile -File scripts/check-admin-contract.ps1 -BackendCommit $backendCommit
```

Do not rewrite route list or unchanged views/realtime bytes.

- [ ] **Step 4: GREEN**

Run `go test ./internal/admincontract ./internal/server ./internal/architecture -count=1` and artifact check with manifest `backend_commit`.

- [ ] **Step 5: Commit**

```powershell
git add internal/admincontract internal/server/testdata/admin_route_policy_golden.json contracts/admin/v1
git commit -m "feat: publish mail diagnostic admin contract"
```

## Task 12: Sync Frontend Types and Strictly Decode the Tuple

**Repository:** `E:\admin\admin_front_ts`

**Files:** generated backend contract/http/routing files; modify `src/api/system/mail.ts`, `mailDict.ts`; create `tests/shared/system/mail-diagnostics-contract.test.ts`.

- [ ] **Step 1: RED tests**

Mock `executeAdminOperation` and exercise public list/detail through one decoder. Cover all-null/all-present, each status, six ASCII digits, valid ordinary/leap/boundary timestamps, partial nulls, Unicode digits, unknown status, timezone suffix, and invalid dates. Prove `AbortSignal` forwarding and generated `PermissionCode` without casts. Create missing test directory first.

- [ ] **Step 2: Confirm RED**

Run `npm test -- tests/shared/system/mail-diagnostics-contract.test.ts`.

- [ ] **Step 3: Implement**

```powershell
$backendCommit = (Get-Content E:/admin/admin_back_go/contracts/admin/v1/manifest.json -Raw | ConvertFrom-Json).backend_commit
npm run contract:sync -- --backend E:/admin/admin_back_go --commit $backendCommit
npm run contract:generate
```

Use generated fields in `MailLogItem`. One narrow decoder enforces all-null/all-non-null, closed status, six ASCII digits, and valid `YYYY-MM-DD HH:mm:ss`; no `any`, unsafe cast, alias, or fallback. Forward `ExecuteOptions` including signal. Require exactly one dictionary option per status.

- [ ] **Step 4: GREEN**

Run the focused test, `npm run contract:check`, and `npm run typecheck`.

- [ ] **Step 5: Commit**

```powershell
git add contracts/backend/admin src/modules/http/generated src/modules/routing/generated src/api/system/mail.ts src/views/Main/system/mail/mailDict.ts tests/shared/system/mail-diagnostics-contract.test.ts
git commit -m "feat: decode mail diagnostic contract"
```

## Task 13: Gate, Render, Abort, and Clear Plaintext UI State

**Repository:** `E:\admin\admin_front_ts`

**Files:** mail `index.vue`, `components/MailLogPanel.vue`, `mailDict.ts`, locale source/generated; create `tests/component/system/mail-diagnostics-lifecycle.test.ts`.

- [ ] **Step 1: RED lifecycle tests**

Create test directory. Mount real page with Pinia, deterministic tabs/table/dialog stubs, deferred requests, and unrelated panel stubs. Cover denied tab/no requests, authorized lazy activation, one fresh list, permission-loss abort/clear/unmount/fallback, late response rejection, detail abort, deactivation/unmount, fresh reactivation, duplicate suppression, identical list/detail values, null dashes, dictionary labels, and no local/session storage writes.

- [ ] **Step 2: Confirm RED**

Run `npm test -- tests/component/system/mail-diagnostics-lifecycle.test.ts`.

- [ ] **Step 3: Implement**

In `index.vue` compute `userStore.can('system_mail_logView')`, render/lazy-mount log only when allowed, synchronously clear and select config on loss, and handle activated/deactivated/unmount. Add stable test IDs.

In panel set table `immediate:false` and expose:

```ts
interface MailLogPanelExpose {
  activate(): Promise<void>
  clearDiagnostics(): void
  refreshLogs(): Promise<void>
}
```

Clear increments generation, aborts detail, calls table `reset()`, resets pagination, closes detail, clears dictionary/list/selection, and drops plaintext. Activation owns one promise and commits only when generation/active/permission remain current. Detail aborts prior request, forwards signal, and rejects stale completion; typed canceled API errors are cancellation, other errors propagate. Render exact unmasked code, separate delivery status, diagnostic code/status/expiry, and dictionary-only labels.

Update Chinese/English notice: encrypted diagnostic snapshot is retained; only authorized administrators see plaintext; body/full TemplateData are not stored. Regenerate locale keys.

- [ ] **Step 4: GREEN**

```powershell
npm test -- tests/shared/system/mail-diagnostics-contract.test.ts tests/component/system/mail-diagnostics-lifecycle.test.ts
npm run locale:generate
npm run locale:check
npm run lint -- --max-warnings 0
npm run typecheck
```

- [ ] **Step 5: Commit**

```powershell
git add src/views/Main/system/mail src/i18n/locales tests/component/system/mail-diagnostics-lifecycle.test.ts
git commit -m "feat: secure mail diagnostic ui lifecycle"
```

## Task 14: Full Gates, Disposable Rekey, and Explicit Local Migration

**Files:**
- Modify `scripts/full-admin-smoke.ps1`, `scripts/tests/admin-contract.tests.ps1`, `session-secret-rotation.tests.ps1`
- Create `scripts/tests/full-admin-smoke-mail-diagnostics.tests.ps1`
- Verify backend/database/runtime scripts and frontend package scripts

- [ ] **Step 1: RED source test**

Extract `Assert-MailLogs` only (so SMS `verify_code` rejection is unaffected). Require validation of three approved fields, no Mail `verify_code` rejection, no body serialization/printing, default 403 mode, explicit `ExpectMailDiagnosticAccess` mode, no-store checks, and no role-grant mutation. Rotation/contract tests require rekey, zero-reference, generated contract, and route policy checks.

- [ ] **Step 2: Confirm RED**

Run `pwsh -NoProfile -File scripts/tests/full-admin-smoke-mail-diagnostics.tests.ps1`.

- [ ] **Step 3: Implement verification harness**

Add `[switch]$ExpectMailDiagnosticAccess`. Default mode calls list/detail with allow-failure helper, asserts 403, discards bodies. Explicit mode uses the existing operator-authorized smoke account, validates non-empty list/detail tuples and headers in memory, then clears response objects. It never creates accounts/permissions/grants and prints counts/status only.

In a verifier-created disposable schema, test current, previous, unknown, and corrupt rows. Stage current/previous roots, assert conversion, zero references, idempotent rerun, and non-zero unknown/corrupt exits; always use verified cleanup. Never put old-key rows in persistent state.

Before changed processes, explicitly apply Atlas `202607230101` under `cmd/admin-db lock-run` name `admin:atlas:migrate`. Use `admin-dev-common.ps1` and `atlas-runtime-common.ps1` to parse Docker-first env; snapshot/restore `MYSQL_DSN`, `APP_SECRET`, optional `APP_SECRET_PREVIOUS` in nested `finally` blocks. Run Atlas status, reconciliation 030/031, fingerprint, and current-key rekey no-op. DSN/roots stay in process env or restricted temporary config and never appear in args/output. Startup/admin-dev never migrate/rekey.

Start `pwsh -NoProfile -File scripts/admin-dev.ps1 -NoBrowser` without image rebuild. Before grant verify hidden tab and 403. After normal role assignment run authorized mode, deliver one real email, and verify identical tuple plus payload-free operation records. Never output/screenshot/store the code. Stop admin-dev; leave state containers running.

- [ ] **Step 4: GREEN full gates**

```powershell
Set-Location E:\admin\admin_back_go
go test ./... -count=1
go vet ./...
pwsh -NoProfile -File scripts/verify-backend.ps1
pwsh -NoProfile -File scripts/verify-database.ps1 -Mode all
pwsh -NoProfile -File scripts/tests/full-admin-smoke-mail-diagnostics.tests.ps1
pwsh -NoProfile -File scripts/tests/admin-contract.tests.ps1
pwsh -NoProfile -File scripts/tests/session-secret-rotation.tests.ps1

Set-Location E:\admin\admin_front_ts
npm run verify:frontend
```

Record only exit status, Atlas status, non-secret fingerprint, and rekey counts.

- [ ] **Step 5: Commit**

```powershell
Set-Location E:\admin\admin_back_go
git add scripts/full-admin-smoke.ps1 scripts/tests/full-admin-smoke-mail-diagnostics.tests.ps1 scripts/tests/admin-contract.tests.ps1 scripts/tests/session-secret-rotation.tests.ps1
git commit -m "test: verify mail diagnostic security boundaries"
```

---

## Final Self-Review Before Execution

- [ ] Coverage: deadline/delivery Tasks 3/5; data/key/rekey Tasks 1/2/4/7/8; read/status Tasks 6/9; RBAC/audit Tasks 9/11; frontend Tasks 12/13; schema/docs Task 10; acceptance/runtime Task 14.
- [ ] No forbidden placeholders (`TBD`, `TODO`, “implement later”, or unspecified edge-case instructions).
- [ ] Signatures match: `VersionedBox`, `Clock`, absolute `SetPendingDelivery`, six-argument email sender, `LogReadRow`, `CreateVerificationLog`, rekey repository/observer, `Required`, and generated tuple.
- [ ] No diagnostic root, credential-box reuse, live session formatter removal, auth lifecycle persistence, `mail_logs` diagnostic columns, historical backfill, or `role_permissions` write.
- [ ] Previous removal requires zero previous/unknown references; migration is explicit; rekey requires writer outage and is bounded/idempotent/resumable.
- [ ] Run `git diff --check` and `git status --short`. Do not run nonexistent business implementation tests while only this plan exists.

## Execution Handoff

Plan saved to `docs/superpowers/plans/2026-07-23-mail-verification-code-diagnostics-plan.md`. Choose:

1. **Subagent-Driven (recommended):** fresh worker per task, diff and focused verification review before the next task, serialized generated contracts/migrations.
2. **Inline Execution:** use `superpowers:executing-plans` and keep every RED/GREEN/commit checkpoint in this session.

Do not start implementation until an execution mode is selected and its skill is loaded.
