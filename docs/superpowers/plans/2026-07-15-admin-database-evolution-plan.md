# Admin Database Evolution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reconcile the imported MySQL database to a checksummed Admin target without losing recoverability, while proving data invariants, object reachability, query plans, repeatability, and empty/imported-schema convergence.

**Architecture:** A repository-owned `admin-db` command captures deterministic schema fingerprints and runs zero-row invariants. PowerShell owns Windows-safe dump/restore and the pinned Atlas container. Reconciliation is serialized as expand, backfill, and verify; all destructive App/Canvas contract DDL remains in P09.

**Tech Stack:** Go 1.26.5, MySQL 8.4, Atlas OSS 0.38.0 pinned at `sha256:9883fdf5290020022ad0ac91fe20b846d32f93c19f68dfd3cf3b327c3e1b7e1a`, PowerShell 7, Docker.

---

## Target file map

- Create `cmd/admin-db/main.go` and `internal/databaseevolution/*` for fingerprint, invariant, SQL-checksum, and query-manifest logic.
- Create `scripts/database/{atlas,new-recovery-artifact,capture-baseline,reconcile,verify-cos-references,capture-query-evidence,establish-baseline,check-drift}.ps1`.
- Move `database/migrations/*.sql` to `database/legacy-migrations/`; historical SQL is never executable migration input.
- Create `database/reconciliation/001_ledger.sql`, `010_expand_core.sql`, `020_backfill_core.sql`, `021_backfill_ai.sql`, `030_verify_schema.sql` through `034_verify_platform.sql`, `040_query_candidates.json`, and `041_apply_proven_indexes.sql`.
- Create `database/schema/admin.hcl`, `database/migrations/202607150001_baseline.sql`, and `atlas.sum`.
- Create `scripts/verify-database.ps1` and `.github/workflows/verify-database.yml`.

## Hard boundary

This plan adds compatible columns/tables, backfills explicit facts, verifies invariants, and adds evidence-backed indexes. It must not drop `users_quick_entry`, `canvas_*` source tables, App/Canvas permissions, `access_token_hash`, compatibility columns, or source AI tables.

### Task 1: Quarantine historical SQL and pin Atlas

**Files:**
- Move: `database/migrations/*.sql` → `database/legacy-migrations/*.sql`
- Create: `database/migrations/202607150001_baseline.sql`
- Create: `scripts/database/atlas.ps1`
- Create: `internal/architecture/database_layout_test.go`
- Create: `database/README.md`

- [ ] **Step 1: Write the failing guard**

```go
func TestDatabaseLayoutSeparatesLegacyAndAtlasMigrations(t *testing.T) {
	root := backendRoot(t)
	legacy, _ := filepath.Glob(filepath.Join(root, "database", "legacy-migrations", "*.sql"))
	if len(legacy) < 40 {
		t.Fatalf("legacy migrations=%d", len(legacy))
	}
	active, _ := filepath.Glob(filepath.Join(root, "database", "migrations", "*.sql"))
	if len(active) != 1 || filepath.Base(active[0]) != "202607150001_baseline.sql" {
		t.Fatalf("active migrations=%v", active)
	}
	data, err := os.ReadFile(filepath.Join(root, "scripts", "database", "atlas.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "arigaio/atlas:0.38.0@sha256:9883fdf5290020022ad0ac91fe20b846d32f93c19f68dfd3cf3b327c3e1b7e1a") {
		t.Fatal("Atlas image is not digest pinned")
	}
}
```

- [ ] **Step 2: Prove failure**

Run: `go test ./internal/architecture -run TestDatabaseLayoutSeparatesLegacyAndAtlasMigrations -count=1`

Expected: FAIL because the legacy directory and wrapper do not exist.

- [ ] **Step 3: Move SQL and create the inert baseline**

Run:

```powershell
git mv database/migrations database/legacy-migrations
New-Item -ItemType Directory database/migrations | Out-Null
```

The baseline file contains only:

```sql
-- atlas:baseline
-- Imported data is reconciled before this revision; no statement is applied.
```

- [ ] **Step 4: Implement the offline wrapper**

```powershell
[CmdletBinding()]
param(
  [Parameter(Mandatory, Position=0)][ValidateSet("migrate","schema")][string]$Command,
  [Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments
)
$ErrorActionPreference = "Stop"
$root = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$image = "arigaio/atlas:0.38.0@sha256:9883fdf5290020022ad0ac91fe20b846d32f93c19f68dfd3cf3b327c3e1b7e1a"
& docker run --rm --network none --volume "${root}:/workspace:ro" --workdir /workspace $image $Command @Arguments
if ($LASTEXITCODE -ne 0) { throw "Atlas exited with code $LASTEXITCODE" }
```

- [ ] **Step 5: Verify and commit**

```powershell
go test ./internal/architecture -run TestDatabaseLayoutSeparatesLegacyAndAtlasMigrations -count=1
pwsh -NoProfile -File scripts/database/atlas.ps1 migrate validate --dir file://database/migrations
git add -- database/legacy-migrations database/migrations database/README.md scripts/database/atlas.ps1 internal/architecture/database_layout_test.go
git commit -m "chore(database): quarantine legacy migrations and pin atlas"
```

Expected: test and Atlas validation pass.

### Task 2: Capture deterministic schema fingerprints

**Files:**
- Create: `cmd/admin-db/main.go`
- Create: `internal/databaseevolution/fingerprint.go`
- Create: `internal/databaseevolution/fingerprint_test.go`

- [ ] **Step 1: Write canonicalization tests**

```go
func TestCanonicalJSONSortsAndExcludesVolatileValues(t *testing.T) {
	in := Fingerprint{ServerVersion: "8.4.10", SQLMode: "NO_ENGINE_SUBSTITUTION", Schema: "admin", Tables: []Table{
		{Name: "users", AutoIncrement: 901, Columns: []Column{{Ordinal: 2, Name: "email"}, {Ordinal: 1, Name: "id"}}},
		{Name: "roles", AutoIncrement: 20, Columns: []Column{{Ordinal: 1, Name: "id"}}},
	}}
	first, err := CanonicalJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := CanonicalJSON(in)
	if !bytes.Equal(first, second) || strings.Contains(string(first), "901") {
		t.Fatalf("non-canonical output: %s", first)
	}
	if strings.Index(string(first), "roles") > strings.Index(string(first), "users") {
		t.Fatalf("tables not sorted: %s", first)
	}
}
```

- [ ] **Step 2: Prove failure**

Run: `go test ./internal/databaseevolution -run TestCanonicalJSON -count=1`

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement stable capture**

Define:

```go
type Fingerprint struct {
	ServerVersion string       `json:"server_version"`
	SQLMode       string       `json:"sql_mode"`
	Schema        string       `json:"schema"`
	Tables        []Table      `json:"tables"`
	ForeignKeys   []ForeignKey `json:"foreign_keys"`
	Checks        []Check      `json:"checks"`
	Triggers      []Trigger    `json:"triggers"`
	Routines      []Routine    `json:"routines"`
	Events        []Event      `json:"events"`
}
type Table struct {
	Name string `json:"name"`
	Engine string `json:"engine"`
	Collation string `json:"collation"`
	Comment string `json:"comment"`
	AutoIncrement uint64 `json:"-"`
	Columns []Column `json:"columns"`
	Indexes []Index `json:"indexes"`
}
type Column struct {
	Ordinal int `json:"ordinal"`
	Name string `json:"name"`
	ColumnType string `json:"column_type"`
	Nullable bool `json:"nullable"`
	Default *string `json:"default"`
	Extra string `json:"extra"`
	Generation string `json:"generation"`
	Comment string `json:"comment"`
}
```

`Capture` queries `@@version`, `@@sql_mode`, and stable fields from `information_schema.tables`, `columns`, `statistics`, `key_column_usage`, `referential_constraints`, `check_constraints`, `triggers`, `routines`, and `events` using bound schema parameters. Sort every collection by its natural stable key. Exclude auto-increment counters and optimizer statistics from the hash.

`admin-db fingerprint --schema admin --out $OutPath --commit $Commit` reads `MYSQL_DSN` only from the environment, rejects a DSN for another schema, writes through temporary-file rename, and prints only the output path and SHA-256.

- [ ] **Step 4: Verify the live schema is read-only and deterministic**

```powershell
$env:MYSQL_DSN = $env:ADMIN_LOCAL_MYSQL_DSN
go run ./cmd/admin-db fingerprint --schema admin --out $env:TEMP\admin-a.json --commit (git rev-parse HEAD)
go run ./cmd/admin-db fingerprint --schema admin --out $env:TEMP\admin-b.json --commit (git rev-parse HEAD)
$a = (Get-Content -Raw $env:TEMP\admin-a.json | ConvertFrom-Json).schema_sha256
$b = (Get-Content -Raw $env:TEMP\admin-b.json | ConvertFrom-Json).schema_sha256
if ($a -ne $b) { throw "fingerprint is not deterministic" }
```

- [ ] **Step 5: Commit**

```powershell
git add -- cmd/admin-db internal/databaseevolution
git commit -m "feat(database): add deterministic schema fingerprinting"
```

### Task 3: Automate a verified recovery artifact

**Files:**
- Create: `scripts/database/new-recovery-artifact.ps1`
- Create: `scripts/tests/database-recovery.tests.ps1`
- Create: `database/evidence/.gitignore`
- Modify: `database/README.md`

- [ ] **Step 1: Write a fake-client test**

Fake `mysqldump` and `mysql` programs write a minimal dump and restore counts. Assert the dump is non-empty, SHA is 64 lowercase hex characters, the temporary MySQL option file is removed, the password is absent from output, and the disposable name matches `^admin_restore_[0-9a-f]{12}$`.

- [ ] **Step 2: Prove failure**

Run: `pwsh -NoProfile -File scripts/tests/database-recovery.tests.ps1`

Expected: FAIL because the recovery script is absent.

- [ ] **Step 3: Implement the recovery workflow**

The script reads `ADMIN_DB_HOST`, `ADMIN_DB_PORT`, `ADMIN_DB_USER`, and `ADMIN_DB_PASSWORD` from environment; writes a current-user-only temporary `.cnf`; invokes:

```powershell
mysqldump --defaults-extra-file=$secretFile --single-transaction --quick --routines --triggers --events --default-character-set=utf8mb4 --result-file=$dumpPath admin
```

It requires the `users` and `wallet_transactions` table definitions, records SHA-256, restores into `admin_restore_<12hex>`, and compares exact counts for `users`, `wallet_transactions`, `user_sessions`, `export_tasks`, `ai_runs`, and `notifications`. `finally` drops only the generated restore database and removes the secret file. It writes an ignored `artifact.json` and emits no credential or DSN.

- [ ] **Step 4: Verify and commit**

```powershell
pwsh -NoProfile -File scripts/tests/database-recovery.tests.ps1
pwsh -NoProfile -File scripts/database/new-recovery-artifact.ps1 -Database admin -BackupRoot (Join-Path $env:TEMP "admin-db-recovery")
git status --short
git add -- database/evidence/.gitignore database/README.md scripts/database/new-recovery-artifact.ps1 scripts/tests/database-recovery.tests.ps1
git commit -m "feat(database): verify recoverable pre-migration backups"
```

Expected: fake and live restore checks pass; no dump, option file, or artifact is tracked.

### Task 4: Record baseline facts and a checksummed ledger

**Files:**
- Create: `scripts/database/capture-baseline.ps1`
- Create: `scripts/database/reconcile.ps1`
- Create: `database/reconciliation/001_ledger.sql`
- Create: `database/reconciliation/README.md`
- Create: `internal/databaseevolution/sqlfiles.go`
- Create: `internal/databaseevolution/sqlfiles_test.go`

- [ ] **Step 1: Test ordered, checksummed SQL discovery**

```go
func TestLoadStageFilesRejectsChecksumDrift(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "020_b.sql"), []byte("SELECT 2;\n"), 0600)
	os.WriteFile(filepath.Join(dir, "010_a.sql"), []byte("SELECT 1;\n"), 0600)
	files, err := LoadStageFiles(dir, map[string]string{
		"010_a.sql": SHA256([]byte("SELECT 1;\n")),
		"020_b.sql": SHA256([]byte("SELECT 2;\n")),
	})
	if err != nil || files[0].Name != "010_a.sql" {
		t.Fatalf("files=%v err=%v", files, err)
	}
	if _, err := LoadStageFiles(dir, map[string]string{"010_a.sql": strings.Repeat("0", 64)}); err == nil {
		t.Fatal("expected checksum mismatch")
	}
}
```

- [ ] **Step 2: Create the ledger**

```sql
CREATE TABLE IF NOT EXISTS `schema_reconciliation_runs` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `stage` VARCHAR(32) NOT NULL,
  `script_name` VARCHAR(191) NOT NULL,
  `script_sha256` CHAR(64) NOT NULL,
  `source_fingerprint_sha256` CHAR(64) NOT NULL,
  `target_fingerprint_sha256` CHAR(64) NULL,
  `executor` VARCHAR(191) NOT NULL,
  `status` VARCHAR(16) NOT NULL,
  `details_json` JSON NULL,
  `started_at` DATETIME(6) NOT NULL,
  `finished_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_schema_reconciliation_script_sha` (`script_name`, `script_sha256`),
  CONSTRAINT `chk_schema_reconciliation_status` CHECK (`status` IN ('running','succeeded','failed'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

`reconcile.ps1` inserts `running` before each file, updates the same row afterward, rejects same-name/different-SHA history, and skips same-name/same-SHA success. It accepts an expected source fingerprint and aborts on mismatch.

- [ ] **Step 3: Capture the imported starting point**

`capture-baseline.ps1` performs only `SELECT`/`SHOW` and records commit, version, SQL mode, fingerprint, table/column/index inventory, distinct platform values, object references, recovery SHA, and exact migration-sensitive counts.

Run:

```powershell
pwsh -NoProfile -File scripts/database/capture-baseline.ps1 -Database admin -RecoveryArtifact $env:ADMIN_RECOVERY_ARTIFACT
```

Expected: MySQL `8.4.10` with `NO_ENGINE_SUBSTITUTION`; `cron_task_log=50689`, `notifications=3078`, `export_tasks=116`, `ai_runs=16`, `ai_image_tasks=5`, `user_sessions=38`, and `users_quick_entry=107` with 3 active. A mismatch requires a fresh recovery artifact and fingerprint.

- [ ] **Step 4: Commit**

```powershell
git add -- scripts/database/capture-baseline.ps1 scripts/database/reconcile.ps1 database/reconciliation/001_ledger.sql database/reconciliation/README.md internal/databaseevolution/sqlfiles.go internal/databaseevolution/sqlfiles_test.go
git commit -m "feat(database): record reconciliation inputs and audit ledger"
```

### Task 5: Add the non-destructive expand schema

**Files:**
- Create: `database/reconciliation/010_expand_core.sql`
- Create: `database/reconciliation/030_verify_schema.sql`
- Create: `internal/architecture/reconciliation_schema_test.go`

- [ ] **Step 1: Guard approved identifiers and reject destructive SQL**

The test requires `export_tasks.platform/kind/object_key`, `ai_runs.platform/input_snapshot/idempotency_key`, `ai_image_tasks.platform`, `ai_image_files`, `ai_text_tasks`, `ai_video_tasks`, `ai_assets`, `payment_callback_events`, `user_wallets.total_consume_cents`, both `verify_code_ttl_minutes` fields, `authz_principal_versions`, `ai_reply_commands`, `ai_provider_attempts`, `realtime_events`, notification source identity, and notification/export claim fields. It rejects `DROP TABLE`, `DROP COLUMN`, `DELETE FROM`, and `tenant`.

- [ ] **Step 2: Prove failure**

Run: `go test ./internal/architecture -run TestReconciliationExpand -count=1`

Expected: FAIL because the expand file is missing.

- [ ] **Step 3: Create durable command and delivery tables**

```sql
CREATE TABLE IF NOT EXISTS `authz_principal_versions` (
  `user_id` BIGINT NOT NULL, `platform` VARCHAR(32) NOT NULL,
  `version` BIGINT UNSIGNED NOT NULL DEFAULT 1,
  `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`user_id`,`platform`),
  CONSTRAINT `chk_authz_principal_platform` CHECK (`platform`='admin')
);
CREATE TABLE IF NOT EXISTS `ai_reply_commands` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, `request_id` VARCHAR(64) NOT NULL,
  `idempotency_key` VARCHAR(128) NOT NULL, `platform` VARCHAR(32) NOT NULL,
  `user_id` BIGINT NOT NULL, `conversation_id` BIGINT NOT NULL, `user_message_id` BIGINT NOT NULL,
  `assistant_message_id` BIGINT NULL, `state` VARCHAR(32) NOT NULL DEFAULT 'pending',
  `attempt_count` INT UNSIGNED NOT NULL DEFAULT 0, `max_attempts` INT UNSIGNED NOT NULL DEFAULT 3,
  `lease_owner` VARCHAR(128) NULL, `lease_token` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `lease_expires_at` DATETIME(6) NULL, `next_attempt_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `cancel_requested_at` DATETIME(6) NULL, `outcome_unknown_at` DATETIME(6) NULL,
  `last_error_code` VARCHAR(64) NOT NULL DEFAULT '', `last_error_message` VARCHAR(512) NOT NULL DEFAULT '',
  `started_at` DATETIME(6) NULL, `finished_at` DATETIME(6) NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`), UNIQUE KEY `uk_ai_reply_request` (`conversation_id`,`request_id`),
  UNIQUE KEY `uk_ai_reply_message` (`user_message_id`), UNIQUE KEY `uk_ai_reply_idempotency` (`idempotency_key`),
  KEY `idx_ai_reply_claim` (`state`,`next_attempt_at`,`lease_expires_at`,`id`),
  CONSTRAINT `chk_ai_reply_state` CHECK (`state` IN ('pending','claimed','running','succeeded','failed','canceled','outcome_unknown','timed_out'))
);
CREATE TABLE IF NOT EXISTS `ai_provider_attempts` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, `command_id` BIGINT UNSIGNED NOT NULL,
  `attempt_no` INT UNSIGNED NOT NULL, `idempotency_key` VARCHAR(128) NOT NULL,
  `state` VARCHAR(24) NOT NULL, `provider_request_id` VARCHAR(191) NOT NULL DEFAULT '',
  `response_sha256` CHAR(64) NOT NULL DEFAULT '', `error_code` VARCHAR(64) NOT NULL DEFAULT '',
  `dispatched_at` DATETIME(6) NULL, `finished_at` DATETIME(6) NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`), UNIQUE KEY `uk_ai_attempt_command_no` (`command_id`,`attempt_no`),
  UNIQUE KEY `uk_ai_attempt_key` (`idempotency_key`)
);
CREATE TABLE IF NOT EXISTS `realtime_events` (
  `sequence` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, `event_id` CHAR(26) NOT NULL,
  `event_type` VARCHAR(96) NOT NULL, `request_id` VARCHAR(64) NULL,
  `target_type` VARCHAR(16) NOT NULL, `target_id` VARCHAR(64) NOT NULL,
  `durability` VARCHAR(16) NOT NULL, `payload_json` JSON NOT NULL,
  `occurred_at` DATETIME(6) NOT NULL, `expires_at` DATETIME(6) NULL,
  PRIMARY KEY (`sequence`), UNIQUE KEY `uk_realtime_event_id` (`event_id`),
  KEY `idx_realtime_resume` (`target_type`,`target_id`,`sequence`)
);
```

- [ ] **Step 4: Add missing active schema**

Conditionally add the approved columns plus `claim_owner VARCHAR(128) NULL`, `claim_token BIGINT UNSIGNED NOT NULL DEFAULT 0`, and `claim_expires_at DATETIME(6) NULL` to notification/export work. Add `notifications.source_task_id BIGINT NULL` and unique `(source_task_id,user_id)`. Create missing tables from the exact active Go model fields. The generic video table is `ai_video_tasks`; do not create a new `canvas_video_tasks`.

`030_verify_schema.sql` returns `invariant, violations` and checks exact types, nullability, defaults, constraints, and index order.

- [ ] **Step 5: Verify on a disposable restore**

```powershell
pwsh -NoProfile -File scripts/database/reconcile.ps1 -Database $env:ADMIN_RESTORE_DB -Stage expand -ExpectedSourceFingerprint $env:ADMIN_BASELINE_FINGERPRINT
go run ./cmd/admin-db invariants --schema $env:ADMIN_RESTORE_DB --file database/reconciliation/030_verify_schema.sql
```

Expected: zero schema violations; repeat execution applies no SQL.

- [ ] **Step 6: Commit**

```powershell
git add -- database/reconciliation/010_expand_core.sql database/reconciliation/030_verify_schema.sql internal/architecture/reconciliation_schema_test.go
git commit -m "feat(database): add non-destructive admin expand schema"
```

### Task 6: Backfill core Admin facts without guessing

**Files:**
- Create: `database/reconciliation/020_backfill_core.sql`
- Create: `database/reconciliation/031_verify_relations.sql`
- Create: `database/reconciliation/032_verify_money.sql`
- Create: `internal/databaseevolution/invariants.go`
- Create: `internal/databaseevolution/invariants_test.go`

- [ ] **Step 1: Test zero-row invariant execution**

```go
func TestRunInvariantFileFailsOnViolation(t *testing.T) {
	db := openInvariantFixture(t)
	result, err := RunInvariantFile(context.Background(), db, "testdata/violations.sql")
	if err == nil || result.Name != "wallet_balance_matches_ledger" || result.Violations != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
```

The command prints invariant names and counts, never the violating rows.

- [ ] **Step 2: Implement deterministic core backfills**

`020_backfill_core.sql` performs only these evidence-backed updates:

- existing exports become `platform='admin'` and `kind='user_list'` because the only pre-expand producer is the Admin user export provider;
- an export `object_key` is decoded only when `file_url` host exactly equals the enabled COS `bucket_domain`; any other non-empty URL is reported and left NULL;
- `authz_principal_versions` receives `(active user id,'admin',1)`;
- active mail/SMS configs use the retired enabled integer setting in `1..60`, otherwise documented value `5`;
- `total_consume_cents` is the active ledger sum for `direction='out'`;
- notification task identity is linked only when title/content/type/level/link/platform/timestamp identifies exactly one task.

Before wallet update, a temporary table calculates balance, recharge, and consume totals. `SIGNAL SQLSTATE '45000'` aborts if stored balance or recharge differs. Never rewrite unexplained money.

- [ ] **Step 3: Add relationship and money invariants**

`031_verify_relations.sql` covers RBAC, payment, wallet, AI message/run/file, notification, and export orphans. `032_verify_money.sql` contains:

```sql
SELECT 'wallet_balance_matches_ledger' AS invariant, COUNT(*) AS violations
FROM (
  SELECT w.id
  FROM user_wallets w
  LEFT JOIN (
    SELECT wallet_id,
      SUM(CASE WHEN direction='in' THEN amount_cents ELSE -amount_cents END) balance,
      SUM(CASE WHEN direction='in' AND source_type='recharge' THEN amount_cents ELSE 0 END) recharge,
      SUM(CASE WHEN direction='out' THEN amount_cents ELSE 0 END) consume
    FROM wallet_transactions WHERE is_del=2 GROUP BY wallet_id
  ) x ON x.wallet_id=w.id
  WHERE w.is_del=2 AND (
    w.balance_cents<>COALESCE(x.balance,0) OR
    w.total_recharge_cents<>COALESCE(x.recharge,0) OR
    w.total_consume_cents<>COALESCE(x.consume,0)
  )
) bad;
```

Also assert unique wallet source identity and unique non-empty payment callback identity.

- [ ] **Step 4: Run and commit**

```powershell
pwsh -NoProfile -File scripts/database/reconcile.ps1 -Database $env:ADMIN_RESTORE_DB -Stage backfill-core -ExpectedSourceFingerprint $env:ADMIN_EXPANDED_FINGERPRINT
go run ./cmd/admin-db invariants --schema $env:ADMIN_RESTORE_DB --file database/reconciliation/031_verify_relations.sql
go run ./cmd/admin-db invariants --schema $env:ADMIN_RESTORE_DB --file database/reconciliation/032_verify_money.sql
git add -- database/reconciliation/020_backfill_core.sql database/reconciliation/031_verify_relations.sql database/reconciliation/032_verify_money.sql internal/databaseevolution/invariants.go internal/databaseevolution/invariants_test.go
git commit -m "feat(database): backfill admin ownership and ledger facts"
```

Expected: both invariant files report zero. Ambiguous object/notification mappings are reported and block P09.

### Task 7: Reconcile AI rows and verify COS references

**Files:**
- Create: `database/reconciliation/021_backfill_ai.sql`
- Create: `database/reconciliation/033_verify_ai.sql`
- Create: `scripts/database/verify-cos-references.ps1`
- Create: `internal/databaseevolution/cos_reference_test.go`

- [ ] **Step 1: Encode source/target invariants**

`033_verify_ai.sql` requires each run to have exactly one evidence source, chat rows to be Admin, image provenance to follow its source table, each source task/file to map once, related files to stay in one task, non-NULL idempotency keys to be unique, and source/target count plus deterministic hash equality.

- [ ] **Step 2: Backfill explicit AI provenance**

```sql
UPDATE ai_runs r
JOIN ai_messages m ON m.id=r.user_message_id
SET r.platform='admin',
    r.input_snapshot=m.content,
    r.idempotency_key=CONCAT('legacy:ai-run:',r.id)
WHERE r.platform IS NULL;
```

Non-chat rows use a temporary source-table/ID map. Any run covered by zero or multiple branches raises `SQLSTATE 45000` before commit. Build `ai_image_files` from legacy relation/asset tables while preserving object key, URL, MIME, dimensions, size, role, order, and related-file identity. Create generic text/video/asset rows only from an explicit owner; no user or platform is invented.

- [ ] **Step 3: Verify retained objects through the COS read adapter**

`verify-cos-references.ps1` selects distinct retained storage keys into an ignored manifest, loads the enabled COS configuration, and uses `storage/cos.ObjectReader` for a HEAD/ranged read. It records `reachable`, `not_found`, or a sanitized dependency class. The test uses `httptest.Server` to prove 200/206 success, exact-key 404 reporting, and credential/query redaction.

- [ ] **Step 4: Run and commit**

```powershell
pwsh -NoProfile -File scripts/database/reconcile.ps1 -Database $env:ADMIN_RESTORE_DB -Stage backfill-ai -ExpectedSourceFingerprint $env:ADMIN_CORE_BACKFILL_FINGERPRINT
go run ./cmd/admin-db invariants --schema $env:ADMIN_RESTORE_DB --file database/reconciliation/033_verify_ai.sql
pwsh -NoProfile -File scripts/database/verify-cos-references.ps1 -Database $env:ADMIN_RESTORE_DB -OutputPath $env:TEMP\admin-cos-evidence.json
git add -- database/reconciliation/021_backfill_ai.sql database/reconciliation/033_verify_ai.sql scripts/database/verify-cos-references.ps1 internal/databaseevolution/cos_reference_test.go
git commit -m "feat(database): reconcile ai records and object references"
```

Expected: AI violations = 0 and every retained object, including the known legacy image, is reachable.

### Task 8: Prove Admin platform and repeatability invariants

**Files:**
- Create: `database/reconciliation/034_verify_platform.sql`
- Create: `scripts/database/verify-expanded-schema.ps1`
- Modify: `database/reconciliation/README.md`

- [ ] **Step 1: Encode platform and identity checks**

`034_verify_platform.sql` rejects unknown platform values, duplicate notification/export/AI/payment/wallet identities, and active rows with missing ownership. It reports legacy App/Canvas groups with their P09 disposition. It lists the three active `users_quick_entry` rows as retirement source evidence and does not alter them.

- [ ] **Step 2: Implement the aggregate verifier**

The script runs `030` through `034`, captures before/after fingerprints, runs focused Admin smoke against the expanded schema, and emits:

```json
{"schema_violations":0,"relationship_violations":0,"money_violations":0,"ai_violations":0,"platform_violations":0,"admin_smoke":"passed"}
```

- [ ] **Step 3: Prove repeated execution**

```powershell
pwsh -NoProfile -File scripts/database/verify-expanded-schema.ps1 -Database $env:ADMIN_RESTORE_DB
pwsh -NoProfile -File scripts/database/reconcile.ps1 -Database $env:ADMIN_RESTORE_DB -Stage all-nondestructive -ExpectedSourceFingerprint $env:ADMIN_BASELINE_FINGERPRINT
pwsh -NoProfile -File scripts/database/verify-expanded-schema.ps1 -Database $env:ADMIN_RESTORE_DB
```

Expected: both summaries are zero/passed; second reconciliation applies no SQL and preserves the fingerprint.

- [ ] **Step 4: Commit**

```powershell
git add -- database/reconciliation/034_verify_platform.sql database/reconciliation/README.md scripts/database/verify-expanded-schema.ps1
git commit -m "test(database): enforce expanded admin data invariants"
```

### Task 9: Accept indexes only with before/after evidence

**Files:**
- Create: `database/reconciliation/040_query_candidates.json`
- Create: `database/reconciliation/041_apply_proven_indexes.sql`
- Create: `scripts/database/capture-query-evidence.ps1`
- Create: `internal/databaseevolution/query_manifest.go`
- Create: `internal/databaseevolution/query_manifest_test.go`
- Modify: the repository files named by accepted candidates

- [ ] **Step 1: Define and validate an executable manifest**

```go
type QueryCandidate struct {
	Name string `json:"name"`
	RepositoryFile string `json:"repository_file"`
	SQL string `json:"sql"`
	Bindings map[string]any `json:"bindings"`
	ExpectedOrder []string `json:"expected_order"`
	RowDistributionSQL string `json:"row_distribution_sql"`
	ProposedIndex string `json:"proposed_index"`
	MaxRowsExamined uint64 `json:"max_rows_examined"`
	MaxP95MS uint64 `json:"max_p95_ms"`
}
```

Validation rejects `SELECT *`, missing ID tie-breakers, empty representative binds, and non-`CREATE INDEX` DDL.

`admin-db query-manifest files --manifest database/reconciliation/040_query_candidates.json` validates the same manifest and prints one normalized repository-relative file per accepted candidate. It rejects absolute paths, `..`, duplicates, non-Go files, and paths outside `internal/module`.

- [ ] **Step 2: Populate every approved candidate**

Include session user/platform/active/expiry, conversation user/active/time/id and agent variant, AI run status/start and platform/created, payment order status/update and status/expiry, recharge credited, notification unread, notification task due/claim, export user/platform/active/id, and cron log task/active/created/id queries.

Change conversation pagination to:

```sql
AND (last_message_at < :before_time
 OR (last_message_at = :before_time AND id < :before_id))
ORDER BY last_message_at DESC, id DESC
LIMIT :limit
```

- [ ] **Step 3: Capture and enforce evidence**

For each candidate, record distribution, `EXPLAIN ANALYZE FORMAT=TREE` before, index DDL/storage cost, after plan, five warm runs with p50/p95, and performance-schema digest delta. Drop the temporary index when it misses either budget or fails to reduce rows examined. Copy only accepted DDL to `041_apply_proven_indexes.sql`.

- [ ] **Step 4: Fix coupled query behavior under tests**

- load AI knowledge hits with one `IN` query and assert query count;
- batch chunk/hit writes at at most 500 rows per transaction;
- move export expiration from list reads to `export:cleanup-expired:v1`;
- transactionally delete AI DB dependents and enqueue unique object cleanup;
- apply keyset pagination to high-growth worker/list scans.

Each repository test first demonstrates the old count/order failure.

- [ ] **Step 5: Run and commit**

```powershell
pwsh -NoProfile -File scripts/database/capture-query-evidence.ps1 -Database $env:ADMIN_RESTORE_DB -Manifest database/reconciliation/040_query_candidates.json -OutputRoot $env:TEMP\admin-query-evidence
pwsh -NoProfile -File scripts/database/reconcile.ps1 -Database $env:ADMIN_RESTORE_DB -Stage proven-indexes -ExpectedSourceFingerprint $env:ADMIN_VERIFIED_FINGERPRINT
go test ./internal/module/auth ./internal/module/ai/conversation ./internal/module/ai/knowledge ./internal/module/ai/run ./internal/module/payment/... ./internal/module/notification/... ./internal/module/export ./internal/module/crontask -count=1
git add -- database/reconciliation/040_query_candidates.json database/reconciliation/041_apply_proven_indexes.sql scripts/database/capture-query-evidence.ps1 internal/databaseevolution/query_manifest.go internal/databaseevolution/query_manifest_test.go
$repositoryFiles = @(go run ./cmd/admin-db query-manifest files --manifest database/reconciliation/040_query_candidates.json)
if ($LASTEXITCODE -ne 0 -or $repositoryFiles.Count -eq 0) { throw "query manifest produced no repository files" }
foreach ($repositoryFile in $repositoryFiles) {
  git add -- $repositoryFile
  if ($LASTEXITCODE -ne 0) { throw "failed to stage query-manifest file" }
}
git diff --cached --check
git commit -m "perf(database): apply evidence-backed admin query indexes"
```

Expected: every accepted index has passing before/after evidence; the generated staging list is non-empty and contains only manifest-declared repository files.

### Task 10: Establish the canonical schema and Atlas baseline

**Files:**
- Create: `database/schema/admin.hcl`
- Create: `atlas.sum`
- Create: `scripts/database/establish-baseline.ps1`
- Create: `scripts/database/check-drift.ps1`
- Modify: `database/README.md`

- [ ] **Step 1: Write imported/empty convergence automation**

Create disposable schemas whose names match `^admin_(empty|imported)_[0-9a-f]{12}$`. Build empty from canonical HCL, restore/reconcile imported, fingerprint both, require equal SHA, and drop only names matching that regex.

- [ ] **Step 2: Inspect and normalize the verified target**

`establish-baseline.ps1` runs pinned `atlas schema inspect` using a read-only environment file, rejects auto-increment counters/definers/destructive contract differences, validates HCL, and atomically replaces `database/schema/admin.hcl` only when the live SHA equals `-ExpectedFingerprint`.

- [ ] **Step 3: Initialize history under a deployment lock**

Acquire `GET_LOCK('admin:atlas:migrate',30)`, reject dirty revisions, baseline at `202607150001`, calculate `atlas.sum`, validate, and release in `finally`. Application startup never calls Atlas.

- [ ] **Step 4: Prove convergence and commit**

```powershell
pwsh -NoProfile -File scripts/database/establish-baseline.ps1 -Database $env:ADMIN_RESTORE_DB -ExpectedFingerprint $env:ADMIN_VERIFIED_FINGERPRINT
pwsh -NoProfile -File scripts/database/check-drift.ps1 -Database $env:ADMIN_RESTORE_DB
pwsh -NoProfile -File scripts/database/check-drift.ps1 -Database $env:ADMIN_EMPTY_DB
git add -- atlas.sum database/schema/admin.hcl database/migrations/202607150001_baseline.sql database/README.md scripts/database/establish-baseline.ps1 scripts/database/check-drift.ps1
git commit -m "feat(database): establish checksummed admin schema baseline"
```

Expected: imported and empty fingerprints are identical; repeat execution has no diff.

### Task 11: Make database verification blocking

**Files:**
- Create: `scripts/verify-database.ps1`
- Create: `.github/workflows/verify-database.yml`
- Modify: `internal/architecture/database_layout_test.go`

- [ ] **Step 1: Guard immutable workflow inputs**

Require `mysql:8.4.10`, the exact Atlas digest, `scripts/verify-database.ps1`, empty/imported jobs, and `actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02`. Reject any action `@v` reference.

- [ ] **Step 2: Implement the shared verifier**

Run Atlas validation/hash, canonical empty-schema apply, sanitized synthetic imported fixture restore, non-destructive reconciliation, `030`–`034` invariants, repeated reconciliation, fingerprint equality, query-manifest validation, and a tracked dump/`.cnf`/`admin-go.env` scan. The fixture contains no live person, payment, prompt, credential, or object data.

- [ ] **Step 3: Run and commit**

```powershell
pwsh -NoProfile -File scripts/verify-database.ps1
git status --short
git add -- .github/workflows/verify-database.yml scripts/verify-database.ps1 internal/architecture/database_layout_test.go
git commit -m "ci: block database drift and invariant violations"
```

Expected: local verifier exits 0. If Docker is unavailable locally, the protected workflow must pass before acceptance.

## Plan completion gate

```powershell
pwsh -NoProfile -File scripts/verify-database.ps1
pwsh -NoProfile -File scripts/database/new-recovery-artifact.ps1 -Database admin -BackupRoot $env:ADMIN_BACKUP_ROOT
pwsh -NoProfile -File scripts/database/check-drift.ps1 -Database admin
git status --short
```

Expected: recovery restore passes; imported, empty, and repeated schemas share one fingerprint; invariants are zero; accepted indexes have evidence; no dump/secret is tracked; status is clean. Retired contract data still exists for P09.
