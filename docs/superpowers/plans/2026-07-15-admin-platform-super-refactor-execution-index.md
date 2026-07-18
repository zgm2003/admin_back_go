# Admin Platform Super Refactor Execution Index

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Coordinate the independently testable P01-P09 phases, including P03.5 and P08.5, that turn the imported system into one deployable Admin platform and leave a clean seam for the next SaaS design.

**Architecture:** Foundation and database work are serialized. All later backend and frontend plans execute serially in the two existing `master` checkouts; Git worktrees and Web/backend GitHub deployment are retired. The sole allowed GitHub Workflow builds, signs, and uploads a Windows Tauri candidate; it never deploys Web/backend or promotes latest. P09 is the only plan allowed to execute destructive contract DDL or declare App/Canvas retirement complete.

**Tech Stack:** Go 1.26.5, Gin, GORM, MySQL 8.4, Redis, Asynq, Atlas 0.38.0, Vue 3.5, TypeScript 5.9, Vite 8, Vitest 4, Tauri 2, Rust, Windows NSIS, Docker Compose, PowerShell 7.

---

## Approved specifications

- `../specs/2026-07-15-admin-platform-super-refactor-design.md`
- `../specs/2026-07-15-admin-foundation-database-design.md`
- `../specs/2026-07-15-admin-go-architecture-design.md`
- `../specs/2026-07-17-admin-docker-stability-closure-design.md`
- `../specs/2026-07-18-p07-p09-execution-rebaseline-design.md`
- `E:/admin/admin_front_ts/docs/superpowers/specs/2026-07-15-admin-frontend-super-refactor-design.md`
- `../../../CONTEXT.md`

The implementation plans may refine mechanics, but they may not change the approved product boundary, durability semantics, credential model, or completion criteria. A design change returns to the spec-review gate.

## Plan set and dependency order

| ID | Plan | Repository | Depends on | Produces |
| --- | --- | --- | --- | --- |
| P01 | `2026-07-15-admin-foundation-verification-plan.md` | backend | approved specs | trusted Go build, strict config, ignored local env, blocking local verification |
| P02 | `2026-07-15-admin-database-evolution-plan.md` | backend | P01 | fingerprint/backup tooling, expand/backfill/verify SQL, Atlas baseline, query evidence |
| P03 | `2026-07-15-admin-go-runtime-contracts-plan.md` | backend | P01, P02 expand schema | process Runtime, Error Module, route registry foundation, Admin Contract Bundle, telemetry seams |
| P03.5 | `2026-07-17-admin-docker-stability-closure-plan.md` | backend + frontend | P03 | dynamic API discovery, bounded state-late startup, one backend build, image provenance, Docker-only recovery proof |
| P04 | `2026-07-15-admin-go-identity-routing-plan.md` | backend | P03.5 | atomic Session Lifecycle, secure browser/desktop auth transport, RBAC principal versions, complete route policy |
| P05 | `2026-07-15-admin-go-durable-work-realtime-plan.md` | backend | P02, P03.5 | durable AI reply command, TaskRegistry, scheduler reconciliation, realtime cursor/recovery |
| P06 | `E:/admin/admin_front_ts/docs/superpowers/plans/2026-07-15-admin-frontend-kernel-plan.md` | frontend | P03 contract bundle, P04 auth contract | AppKernel, AuthSession, ApiClient, route registry, typed persistence |
| P07 | `E:/admin/admin_front_ts/docs/superpowers/plans/2026-07-15-admin-frontend-realtime-resource-plan.md` | frontend | P05 realtime contract, P06; Tasks 6-10 wait for P08 | RealtimeClient, ResourceQuery, table/page migration, behavior tests, Docker smoke, user acceptance checklist, budgets |
| P08 | `E:/admin/admin_front_ts/docs/superpowers/plans/2026-07-15-admin-tauri-security-plan.md` | frontend | P04 desktop auth contract, P06, P07 Tasks 1-5 | Windows local packaged UI, narrow NativeBridge, safe downloads/credentials/updater, Rust gates |
| P08.5 | `2026-07-18-admin-tauri-windows-release-plan.md` | backend + frontend | P07, P08 | tag-triggered signed Windows candidate, immutable COS upload, Admin import, user-controlled promotion |
| P09 | `2026-07-15-admin-only-release-plan.md` | backend + frontend | P02 verify, P03.5, P04-P08.5 | App/Canvas code/schema contract, Docker release proof, runbooks and rollback |

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
                                         └─→ P08.5 Windows Tauri candidate release

P02 verified + P03.5 + P04 + P05 + P06 + P07 + P08 + P08.5
                           └─→ P09 Admin-only contract/release
```

P04 and P05 were completed serially on backend `master`. All remaining plans continue serially: accept P06 manually, execute P07 Tasks 1–5, execute all of P08, finish P07 Tasks 6–10, then execute cross-repository P08.5 before P09. This barrier prevents page decomposition/lazy-import work from racing NativeBridge, DownloadManager, Tauri packaging, candidate import, or COS publication changes.

## Global execution protocol

- [x] **Step 1: Use only the existing master checkouts**

Use only `E:/admin/admin_back_go` and `E:/admin/admin_front_ts`, both on `master`. Do not create or retain `.worktrees` directories. Never let two implementation agents edit the same repository concurrently.

- [ ] **Step 2: Record the baseline**

Run in each checkout:

```powershell
git rev-parse HEAD
git status --short
```

Expected: one commit ID per repository and no status output.

- [ ] **Step 3: Execute one plan task inline unless the user requests delegation**

Give the active executor the exact plan task, approved spec links, repository path, allowed files, and verification command. Use TDD, stage only declared files, and create the task's named commit. Do not create a subagent or parallel edit unless the user explicitly requests delegation.

- [ ] **Step 4: Run two reviews before accepting the task**

Perform two explicit review passes. The first checks only spec/plan compliance. The second checks code quality, test quality, concurrency, failure behavior, and secret handling. Resolve all findings and rerun task verification. A separate reviewer is used only when the user explicitly requests delegation.

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
| Docker verification and Compose delivery files | current Docker delivery task only |
| `.github/workflows/release-tauri.yml`, Tauri version files, signing inputs | P08.5 frontend release task only |
| COS `tauri_candidates/` and live `tauri_updater/` keys | P08.5 candidate owner; live pointer changes only after user promotion |

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
- [x] **Gate D:** P04 proves one-winner refresh, route-policy completeness, and secure browser/desktop auth transport.
- [x] **Gate E:** P05 proves AI reply survival across process termination, scheduler lease safety, and realtime recovery.
- [ ] **Gate F:** P06-P08 pass frontend unit/component/integration, Docker runtime, manual acceptance, and Rust/Tauri gates; P08.5 produces a verified signed Windows candidate without automatic promotion.
- [ ] **Gate G:** P09 removes retired platform code/schema and passes the complete cross-repository release proof.

No later gate waives an earlier one. P09 must stop before destructive DDL if any invariant, COS reachability check, recovery restore, or rollback rehearsal fails.

## Gate A-C.5 evidence (2026-07-17)

- **Gate A:** `scripts/verify-go-clean.ps1` exited `0` from an empty module cache; module verification, all tests, Linux race tests, vet, staticcheck, govulncheck, and both binary builds passed. Govulncheck found `0` called vulnerabilities.
- **Gate B:** `scripts/verify-database.ps1 -Mode all` exited `0`; empty/imported SHA-256 both equal `76d7d64d8151e8122369fcd07ce18ae194d779037816b8496ed78e62c655ccbf`, reconciliation applied/skipped `7/7`, and invariants/smoke passed. The retained `61,618,047`-byte recovery artifact is verified with SHA-256 `78390456ed511f9507233e41df170223b365dd1b056e804f6e55052259e04a85` and equal source/restore counts.
- **Gate C:** the runtime gate passed ordinary and Linux race suites, the Admin Contract Bundle reported no drift, manifest SHA-256 is `25f1bab4c875541311628263e23766e358ca3f65c81b14c804fe3cb5bf34e4d7`, and both binaries built.
- **Gate C.5:** `scripts/tests/docker-stability.tests.ps1` passed API-address replacement without frontend recreation, state-late recovery with API/worker restart counts `0/0`, Docker SIGTERM exit `0`, and final five-container restoration. Backend/frontend image revision labels were inspected against their owning Git revisions; no volume-delete command was used.

## Gate D evidence (2026-07-17)

- P04 executed directly on backend `master` by explicit operator instruction; no P04 worktree was created or used.
- Session issue/refresh contention tests proved max/single-session invariants and exactly one CAS refresh winner. Docker `TestMultiNode -race -count=5` proved revoke, user-disable, role-change, and refresh-reuse denial within two seconds across shared MySQL/Redis nodes.
- Browser/desktop credential tests proved HttpOnly/Secure/SameSite-Strict browser refresh transport, desktop-only refresh response data, exact-Origin validation, one-use realtime tickets, and scoped queue-monitor grants with no access-token cookie/query fallback.
- Versioned principal tests proved zero-SQL cache hits, mutation version bumps, fail-closed Redis behavior, and stale-snapshot denial.
- The compiled route-policy golden and Admin Contract Bundle passed with every active route classified, every mutation carrying an audit decision, and every permission code present in the catalog.
- `scripts/tests/session-secret-rotation.tests.ps1`, `scripts/verify-identity-routing.ps1`, and `scripts/verify-backend.ps1` exited `0`; `go vet`, pinned `staticcheck`, `govulncheck` (zero called vulnerabilities), and both process builds passed.

## Gate E evidence (2026-07-18)

- P05 executed directly on backend `master` by explicit operator instruction; no P05 worktree was created or used.
- Typed TaskRegistry ownership, MySQL reply commands/provider attempts, renewable fenced leases, scheduler reconciliation, and notification/export claims are committed through `8a73dc8`; durable typed realtime recovery is committed as `8458ecf`.
- The Docker kill/restart scenario passed API-after-commit termination, Worker-after-claim termination and lease recovery, cross-node cancellation, Redis-backed multi-node fan-out, disconnected cursor replay, and duplicate-result assertions.
- `scripts/verify-backend.ps1` exited `0` with all repository/race/architecture/contract tests, vet, pinned staticcheck, govulncheck (`0` called vulnerabilities), Atlas validation, and API/Worker builds passing.
- `scripts/verify-database.ps1 -Mode all` exited `0`; empty/imported schemas converged at `50e7642abe6f615167ab0fc64e3bd4aa765c0dc8695d2d4a2fc515365bc713cb`, all 8 reconciliations were idempotent, and database invariants/Admin smoke passed.
- The Admin realtime bundle has no drift and records backend commit `8458ecfc671f558af65a6f89c590891253179cdc`; live Docker MySQL has the seven-day retention schema, watermark, and enabled cleanup cron.

## Gate F P06 evidence (2026-07-18)

- P06 executed directly on frontend `master` by explicit operator instruction. Frontend and backend have one registered primary checkout each; `.worktrees` and `.github` are absent from both repositories.
- Frontend AppKernel/AuthSession/ApiClient/routes/persistence integration and the route-install/menu-persistence login regression fix are committed through `ca8e500`; Docker-only frontend delivery is implemented by `84868a6`. Backend Compose-only deployment plus the Admin password-login contract fix are committed through `fed96e8`.
- The frontend verifier passed contract generation, route generation, lint baseline with 0 errors/81 warnings, typecheck/production build, and 460 tests across 134 files. P07 still owns removal of the warning baseline.
- The revision-labelled frontend/backend images matched `ca8e500`/`fed96e8`. The Compose lifecycle rebuilt both application images and restored five healthy containers; `/healthz`, `/health`, and `/ready` all passed.
- Gate F remains unchecked until the user accepts P06, P07 Docker/manual gates pass, P08 Windows Rust/Tauri gates pass, and P08.5 proves tag-to-COS candidate import with no automatic latest/force-update mutation.
