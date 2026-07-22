# Local Initialization Data Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Track the current 125-row Admin permission tree as an empty-table seed and reset all local payment data except `payment_configs`.

**Architecture:** Keep the permission seed as explicit SQL under `database/seeds/`, outside Atlas migrations and application startup. Validate the seed structurally in Go and by loading it into a disposable MySQL schema cloned from the canonical `permissions` table. Reset payment data directly in the current local MySQL state using an allowlisted child-first transaction, then reset counters and prove the retained payment configuration hash is unchanged.

**Tech Stack:** MySQL 8.4, SQL, Go tests, PowerShell, Docker state container

---

## File Structure

- Create `database/seeds/admin_permissions.sql`: canonical 125-row Admin-only permission seed with stable IDs and an empty-table guard.
- Create `database/seeds/README.md`: exact seed purpose, precondition, and import command; explicitly excludes users, roles, and grants.
- Create `internal/architecture/local_initialization_seed_test.go`: parses and validates seed tuple structure and forbidden table writes.
- Modify `docs/superpowers/plans/2026-07-22-local-initialization-data-plan.md`: mark completed steps with evidence.

### Task 1: Guard The Permission Seed Contract

**Files:**
- Create: `internal/architecture/local_initialization_seed_test.go`
- Test: `internal/architecture/local_initialization_seed_test.go`

- [ ] **Step 1: Write the failing architecture test**

Add a test that reads `database/seeds/admin_permissions.sql`, parses each one-line values tuple while respecting SQL quoted strings, and asserts:

```go
if len(rows) != 125 { t.Fatalf("permission seed row count=%d want 125", len(rows)) }
if row.Platform != "admin" || row.Status != 1 || row.IsDel != 2 {
    t.Fatalf("permission %d has invalid lifecycle fields", row.ID)
}
if _, ok := ids[row.ParentID]; row.ParentID != 0 && !ok {
    t.Fatalf("permission %d references missing parent %d", row.ID, row.ParentID)
}
```

It must also reject writes to `users`, `roles`, and `role_permissions`, reject `app`, `canvas`, and `clientVersion`, and require an explicit empty-table guard before the permission insert.

- [ ] **Step 2: Run the test and verify RED**

Run:

```powershell
go test ./internal/architecture -run TestLocalPermissionSeed -count=1
```

Expected: FAIL because `database/seeds/admin_permissions.sql` does not exist.

### Task 2: Materialize The Current Permission Tree

**Files:**
- Create: `database/seeds/admin_permissions.sql`
- Create: `database/seeds/README.md`
- Test: `internal/architecture/local_initialization_seed_test.go`

- [ ] **Step 1: Generate the deterministic row projection**

Query the current local `permissions` table in `id` order for these columns only:

```sql
id, name, path, icon, parent_id, component, platform, type, sort,
code, i18n_key, show_menu, status, is_del
```

Filter with `platform = 'admin' AND is_del = 2`. Do not export timestamps or any row from another platform.

- [ ] **Step 2: Add the empty-table guard and one atomic insert**

The seed starts a transaction, creates a temporary non-null guard table, fails the guard insert when `permissions` is not empty, inserts all 125 tuples with explicit IDs, and commits. It contains no statement targeting account or authorization tables.

- [ ] **Step 3: Document the seed boundary**

Document that the seed is for an empty local `permissions` table, is never run at API startup, and does not initialize `users`, `roles`, or `role_permissions`.

- [ ] **Step 4: Run the architecture test and verify GREEN**

Run:

```powershell
go test ./internal/architecture -run TestLocalPermissionSeed -count=1
```

Expected: PASS with exactly 125 valid rows.

- [ ] **Step 5: Verify against a disposable MySQL schema**

Create a random temporary schema, clone the table with
`CREATE TABLE admin_seed_verify.permissions LIKE admin.permissions`, apply the
seed, and compare the complete 14-column ordered projection against the current
active Admin rows. Verify a second seed application fails the empty-table
guard. Drop the temporary schema in `finally`.

### Task 3: Reset Current Local Payment Data

**Files:**
- Modify: current local MySQL data only

- [ ] **Step 1: Capture the retained configuration proof**

Capture the row count and SHA-256 of a deterministic data-only dump of `payment_configs` without printing its credential fields.

- [ ] **Step 2: Clear the allowlisted tables child-first**

In one transaction, execute:

```sql
DELETE FROM payment_callback_events;
DELETE FROM payment_recharges;
DELETE FROM wallet_transactions;
DELETE FROM payment_orders;
DELETE FROM payment_recharge_packages;
DELETE FROM user_wallets;
```

Commit, then set each cleared table's `AUTO_INCREMENT` to `1`. Do not issue any write against `payment_configs`.

- [ ] **Step 3: Verify the reset**

Assert every cleared table has zero rows. Capture the payment configuration count and SHA-256 again and require exact equality with Step 1.

### Task 4: Full Verification And Delivery

**Files:**
- Modify: `docs/superpowers/plans/2026-07-22-local-initialization-data-plan.md`

- [ ] **Step 1: Run focused and full tests**

Run:

```powershell
go test ./internal/architecture -count=1
go test ./cmd/admin-db ./internal/databaseevolution -count=1
go test ./... -count=1
```

Expected: all tests pass.

- [ ] **Step 2: Review repository state**

Run `git diff --check` and confirm only the seed, seed documentation, architecture test, design, and plan changed.

- [ ] **Step 3: Commit and push master**

Commit the implementation with:

```powershell
git add database/seeds/admin_permissions.sql database/seeds/README.md internal/architecture/local_initialization_seed_test.go docs/superpowers/plans/2026-07-22-local-initialization-data-plan.md
git commit -m "feat(database): add local permission seed"
git push origin master
```
