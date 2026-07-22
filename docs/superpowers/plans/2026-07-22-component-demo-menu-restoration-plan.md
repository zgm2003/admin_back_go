# Component Demo Menu Restoration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore the six-row Admin component demonstration menu tree, grant its five pages to the current local super-admin role, and include the tree in empty-table initialization data.

**Architecture:** Keep the existing database-driven dynamic routing model. Extend the tracked permission seed and its architecture contract, apply the same six rows directly to the current local MySQL state, and add only role-2 page grants; no frontend source or startup behavior changes.

**Tech Stack:** MySQL 8.4, SQL, Go architecture tests, Vue 3 dynamic routes, PowerShell, admin-dev

---

## File Structure

- Modify `internal/architecture/local_initialization_seed_test.go`: validate the 131-row seed and exact restored component tree.
- Modify `database/seeds/admin_permissions.sql`: add the six stable component menu rows in ID order.
- Modify `docs/superpowers/plans/2026-07-22-component-demo-menu-restoration-plan.md`: record execution evidence.
- Modify current local MySQL data only: insert six permissions and five role-2 grants.

### Task 1: Lock The Restored Seed Contract

**Files:**
- Modify: `internal/architecture/local_initialization_seed_test.go`
- Test: `internal/architecture/local_initialization_seed_test.go`

- [x] **Step 1: Write the failing architecture assertions**

Extend `permissionSeedRow` to parse `name`, `path`, `component`, and `i18n_key`.
Change the row count to 131 and assert these exact rows:

```go
expected := map[int64]permissionSeedRow{
    4:  {id: 4, name: "组件演示", parentID: 0, platform: "admin", typeID: 1, i18nKey: "menu.component", showMenu: 1, status: 1, isDel: 2},
    40: {id: 40, name: "上传", path: "/component/upload", parentID: 4, component: "component/upload", platform: "admin", typeID: 2, i18nKey: "menu.component_upload", showMenu: 1, status: 1, isDel: 2},
    41: {id: 41, name: "表单", path: "/component/form", parentID: 4, component: "component/form", platform: "admin", typeID: 2, i18nKey: "menu.component_form", showMenu: 1, status: 1, isDel: 2},
    42: {id: 42, name: "展示", path: "/component/display", parentID: 4, component: "component/display", platform: "admin", typeID: 2, i18nKey: "menu.component_display", showMenu: 1, status: 1, isDel: 2},
    43: {id: 43, name: "特效", path: "/component/effect", parentID: 4, component: "component/effect", platform: "admin", typeID: 2, i18nKey: "menu.component_effect", showMenu: 1, status: 1, isDel: 2},
    80: {id: 80, name: "下载管理器", path: "/component/download", parentID: 4, component: "component/download", platform: "admin", typeID: 2, i18nKey: "menu.component_download", showMenu: 1, status: 1, isDel: 2},
}
```

- [x] **Step 2: Verify RED**

Run:

```powershell
go test ./internal/architecture -run TestLocalPermissionSeed -count=1
```

Expected: FAIL with `permission seed row count=125 want 131`.

Observed: FAIL with `permission seed row count=125 want 131`.

### Task 2: Restore Tracked And Local Data

**Files:**
- Modify: `database/seeds/admin_permissions.sql`
- Test: `internal/architecture/local_initialization_seed_test.go`

- [x] **Step 1: Add the six seed tuples in strict ID order**

Use root ID 4 and page IDs 40, 41, 42, 43, and 80. Use `Menu` for the root
icon, empty path/component values for the directory, null permission codes,
visible active lifecycle values, and the route and locale values from Task 1.

- [x] **Step 2: Verify GREEN**

Run:

```powershell
go test ./internal/architecture -run TestLocalPermissionSeed -count=1
```

Expected: PASS with 131 rows and 101 non-empty unique permission codes.

- [x] **Step 3: Apply the local database transaction**

Insert the six permission rows, then insert active `role_permissions` rows for
role ID 2 and page IDs 40, 41, 42, 43, and 80. Do not grant directory ID 4 and
do not modify role ID 1.

- [x] **Step 4: Verify local data**

Assert the six permission rows match the seed projection, exactly five active
role-2 grants target the restored pages, and no role-1 grant targets them.

Observed: six matching permission rows, five active role-2 page grants, and
zero role-1 grants. The retained local user remains role ID 2.

### Task 3: Runtime And Delivery Verification

**Files:**
- Modify: `docs/superpowers/plans/2026-07-22-component-demo-menu-restoration-plan.md`

- [x] **Step 1: Verify the disposable seed**

Clone `admin.permissions` into a random disposable schema, apply the seed, and
assert 131 rows with zero 14-column mismatches against the current active Admin
rows. Assert a second application fails in `_admin_permission_seed_guard` and
drop the schema in `finally`.

Observed: first application produced 131 matching rows with zero projection
mismatches; the empty-table guard rejected the second application.

- [x] **Step 2: Verify runtime routing**

Using the running `admin-dev` environment, authenticate as the retained local
user and assert the menu response contains `/component/upload`,
`/component/form`, `/component/display`, `/component/effect`, and
`/component/download`. Open at least one restored route and confirm the page
renders without a missing-route or bootstrap error.

Observed: `/users/me` returned all five restored routes. The running frontend
rendered the `组件演示` sidebar entry and the `UpMedia 图片上传` page with zero
bootstrap or missing-route console errors.

- [x] **Step 3: Run full checks**

Run:

```powershell
go test ./internal/architecture -count=1
go test ./... -count=1
git diff --check
```

Expected: all commands pass.

Observed: architecture tests and the complete Go test suite passed;
`git diff --check` completed with no whitespace errors.

- [x] **Step 4: Commit and push master**

```powershell
git add database/seeds/admin_permissions.sql internal/architecture/local_initialization_seed_test.go docs/superpowers/plans/2026-07-22-component-demo-menu-restoration-plan.md
git commit -m "feat(database): restore component demo menu"
git push origin master
```
