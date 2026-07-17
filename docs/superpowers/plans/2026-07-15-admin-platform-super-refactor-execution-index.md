# Admin Platform Super Refactor Execution Index

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Coordinate the ten independently testable implementation plans that turn the imported system into one deployable Admin platform and leave a clean seam for the next SaaS design.

**Architecture:** Foundation and database work are serialized. Once the target schema, contract bundle, and runtime seams are stable, backend identity, backend durable work, frontend kernel, frontend realtime/resource, and desktop security can proceed in isolated lanes. The final plan is the only plan allowed to execute destructive contract DDL or declare App/Canvas retirement complete.

**Tech Stack:** Go 1.26.5, Gin, GORM, MySQL 8.4, Redis, Asynq, Atlas 0.38.0, Vue 3.5, TypeScript 5.9, Vite 8, Vitest 4, Playwright, Tauri 2, Rust.

---

## Approved specifications

- `../specs/2026-07-15-admin-platform-super-refactor-design.md`
- `../specs/2026-07-15-admin-foundation-database-design.md`
- `../specs/2026-07-15-admin-go-architecture-design.md`
- `../specs/2026-07-17-admin-docker-stability-closure-design.md`
- `E:/admin/admin_front_ts/docs/superpowers/specs/2026-07-15-admin-frontend-super-refactor-design.md`
- `../../../CONTEXT.md`

The implementation plans may refine mechanics, but they may not change the approved product boundary, durability semantics, credential model, or completion criteria. A design change returns to the spec-review gate.

## Plan set and dependency order

| ID | Plan | Repository | Depends on | Produces |
| --- | --- | --- | --- | --- |
| P01 | `2026-07-15-admin-foundation-verification-plan.md` | backend | approved specs | trusted Go build, strict config, ignored local env, blocking backend CI |
| P02 | `2026-07-15-admin-database-evolution-plan.md` | backend | P01 | fingerprint/backup tooling, expand/backfill/verify SQL, Atlas baseline, query evidence |
| P03 | `2026-07-15-admin-go-runtime-contracts-plan.md` | backend | P01, P02 expand schema | process Runtime, Error Module, route registry foundation, Admin Contract Bundle, telemetry seams |
| P03.5 | `2026-07-17-admin-docker-stability-closure-plan.md` | backend + frontend | P03 | dynamic API discovery, bounded state-late startup, one backend build, image provenance, Docker-only recovery proof |
| P04 | `2026-07-15-admin-go-identity-routing-plan.md` | backend | P03.5 | atomic Session Lifecycle, secure browser/desktop auth transport, RBAC principal versions, complete route policy |
| P05 | `2026-07-15-admin-go-durable-work-realtime-plan.md` | backend | P02, P03.5 | durable AI reply command, TaskRegistry, scheduler reconciliation, realtime cursor/recovery |
| P06 | `E:/admin/admin_front_ts/docs/superpowers/plans/2026-07-15-admin-frontend-kernel-plan.md` | frontend | P03 contract bundle, P04 auth contract | AppKernel, AuthSession, ApiClient, route registry, typed persistence |
| P07 | `E:/admin/admin_front_ts/docs/superpowers/plans/2026-07-15-admin-frontend-realtime-resource-plan.md` | frontend | P05 realtime contract, P06; Tasks 6–10 wait for P08 | RealtimeClient, ResourceQuery, table/page migration, executable behavior tests, budgets |
| P08 | `E:/admin/admin_front_ts/docs/superpowers/plans/2026-07-15-admin-tauri-security-plan.md` | frontend | P04 desktop auth contract, P06, P07 Tasks 1–5 | local packaged UI, narrow NativeBridge, safe downloads/credentials/updater, Rust gates |
| P09 | `2026-07-15-admin-only-release-plan.md` | both | P02 verify, P03.5, P04–P08 | App/Canvas code/schema contract, synchronized release proof, runbooks and rollback |

## Dependency graph

```text
P01 Foundation
 ├─→ P02 Database expand/backfill/verify
 └─→ P03 Runtime + contracts
       └─→ P03.5 Docker stability
              ├─→ P04 Identity/RBAC
              ├─→ P05 Durable work/realtime ← P02
              └─→ P06 Frontend kernel ← P04 auth contract
                    └─→ P07 Tasks 1–5 realtime/resource ← P05
                           └─→ P08 Tauri security ← P04
                                  └─→ P07 Tasks 6–10 quality/budgets

P02 verified + P03.5 + P04 + P05 + P06 + P07 + P08
                           └─→ P09 Admin-only contract/release
```

P04 and P05 may run concurrently after P03.5 only in separate worktrees and only if they do not edit `internal/runtime`, `internal/server/adminroute`, `contracts/admin`, or the same database migration. In the frontend lane, execute P07 Tasks 1–5, then all of P08, then P07 Tasks 6–10. This barrier prevents the page decomposition/lazy-import tasks from racing the NativeBridge, DownloadManager, Tauri adapter, package, and workflow changes.

## Global execution protocol

- [ ] **Step 1: Create isolated worktrees at execution time**

Use `superpowers:using-git-worktrees` before the first implementation task. Create one backend worktree and one frontend worktree from the commits containing this plan suite. Never let two implementation agents edit the same worktree concurrently.

- [ ] **Step 2: Record the baseline**

Run in each worktree:

```powershell
git rev-parse HEAD
git status --short
```

Expected: one commit ID per repository and no status output.

- [ ] **Step 3: Execute one plan task with a fresh implementation subagent**

Give the subagent the exact plan task, approved spec links, repository path, allowed files, and verification command. The subagent must use TDD, stage only declared files, and create the task's named commit.

- [ ] **Step 4: Run two reviews before accepting the task**

The first fresh reviewer checks only spec/plan compliance. The second fresh reviewer checks code quality, test quality, concurrency, failure behavior, and secret handling. The root agent resolves all findings and reruns the task verification.

- [ ] **Step 5: Update plan checkboxes only after verification**

A task is complete only when its declared command exits 0 and the root agent has inspected `git diff HEAD^`. Do not mark work complete because a subagent says it is complete.

## Shared-state ownership

Only one active agent may own each row:

| Shared state | Exclusive owner |
| --- | --- |
| live `admin` database, recovery dump, Atlas revision table | P02/P09 database task currently in progress |
| `database/migrations` and `database/migrations/atlas.sum` | database migration owner |
| `contracts/admin` and bundle manifest | P03 contract owner |
| frontend `contracts/backend` lock/generated types | P06 contract consumer owner |
| `internal/runtime` composition roots | P03, then P03.5, then integration owner |
| backend route registry | P03/P04 integration owner |
| `src/app` and `src/modules/auth` | P06 |
| `package.json` / lockfile | one frontend dependency task at a time |
| GitHub workflow files | current plan's CI task only |

Database tasks are never parallelized against the same schema. Read-only `SELECT`/`SHOW`/`EXPLAIN` checks may run concurrently only after the database owner records the current fingerprint.

## Commit discipline

Each plan task ends with one focused commit. Mechanical formatting, generated output, schema DDL, backfill data logic, index evidence, and destructive contract changes are separate commits. Use exact-path staging:

```powershell
git add -- internal/runtime/api.go internal/runtime/api_test.go
git diff --cached --check
git diff --cached
git commit -m "feat(runtime): add signal-aware api lifecycle"
```

The example paths/message illustrate the rule; each task uses its own literal staging list and commit message from the corresponding plan.

Never use `git add -A`, `git reset --hard`, `git checkout --`, or an unreviewed migration replay.

## Program gates

- [x] **Gate A:** P01 passes from a clean module cache and both backend binaries build.
- [x] **Gate B:** P02 restores a verified recovery artifact, and imported/empty schemas converge to the same fingerprint.
- [x] **Gate C:** P03 publishes a deterministic Admin Contract Bundle and both processes pass lifecycle tests.
- [x] **Gate C.5:** P03.5 proves dynamic API discovery, bounded state-late startup with zero restart loops, correct image revisions, and zero-exit Docker SIGTERM; final restoration preserves all state volumes.
- [ ] **Gate D:** P04 proves one-winner refresh, route-policy completeness, and secure browser/desktop auth transport.
- [ ] **Gate E:** P05 proves AI reply survival across process termination, scheduler lease safety, and realtime recovery.
- [ ] **Gate F:** P06–P08 pass frontend unit/component/integration/browser/Rust gates and produce immutable artifacts.
- [ ] **Gate G:** P09 removes retired platform code/schema and passes the complete cross-repository release proof.

No later gate waives an earlier one. P09 must stop before destructive DDL if any invariant, COS reachability check, recovery restore, or rollback rehearsal fails.

## Gate A-C.5 evidence (2026-07-17)

- **Gate A:** `scripts/verify-go-clean.ps1` exited `0` from an empty module cache; module verification, all tests, Linux race tests, vet, staticcheck, govulncheck, and both binary builds passed. Govulncheck found `0` called vulnerabilities.
- **Gate B:** `scripts/verify-database.ps1 -Mode all` exited `0`; empty/imported SHA-256 both equal `76d7d64d8151e8122369fcd07ce18ae194d779037816b8496ed78e62c655ccbf`, reconciliation applied/skipped `7/7`, and invariants/smoke passed. The retained `61,618,047`-byte recovery artifact is verified with SHA-256 `78390456ed511f9507233e41df170223b365dd1b056e804f6e55052259e04a85` and equal source/restore counts.
- **Gate C:** the runtime gate passed ordinary and Linux race suites, the Admin Contract Bundle reported no drift, manifest SHA-256 is `25f1bab4c875541311628263e23766e358ca3f65c81b14c804fe3cb5bf34e4d7`, and both binaries built.
- **Gate C.5:** `scripts/tests/docker-stability.tests.ps1` passed API-address replacement without frontend recreation, state-late recovery with API/worker restart counts `0/0`, Docker SIGTERM exit `0`, and final five-container restoration. Backend/frontend image revision labels were inspected against their owning Git revisions; no volume-delete command was used.
