# Admin Platform Super Refactor Execution Index

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to execute the active plan task-by-task. Work inline unless the user explicitly requests delegation. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Coordinate the independently testable P01-P09 phases, including the completed P03.5 and P08R, into one current Browser-only Admin product whose formal delivery remains exclusively Docker-based while its authenticated platform kernel remains ready for the next explicitly contracted adapter.

**Architecture:** Foundation/database/runtime phases remain serialized. All active work runs directly in the two existing `master` checkouts with no worktrees, no browser automation, and no GitHub Workflow. Formal builds and acceptance remain full-Docker; the approved Windows development phase alone runs Vite/API/Worker on the host while MySQL/Redis stay in Docker. P08 is retained as completed history but its Tauri result is superseded by P08R; P08.5 is cancelled. P09 is the only phase allowed to execute destructive App/Canvas or `client_versions` contract DDL.

**Tech Stack:** Go 1.26.5, Gin, GORM, MySQL 8.4, Redis, Asynq, Atlas 0.38.0, Vue 3.5, TypeScript 5.9, Node 24.18.0, npm 11.16.0, Vite 8, Vitest 4, Air 1.66.0, Docker Compose, PowerShell 7.

---

## Approved specifications

Current precedence, highest first:

1. `../specs/2026-07-20-extensible-platform-kernel-design.md`
2. `../specs/2026-07-19-admin-browser-only-tauri-retirement-design.md`
3. `../specs/2026-07-20-windows-local-hot-reload-development-design.md`
4. `../specs/2026-07-15-admin-platform-super-refactor-design.md`
5. `../specs/2026-07-15-admin-foundation-database-design.md`
6. `../specs/2026-07-15-admin-go-architecture-design.md`
7. `../specs/2026-07-17-admin-docker-stability-closure-design.md`
8. `E:/admin/admin_front_ts/docs/superpowers/specs/2026-07-15-admin-frontend-super-refactor-design.md`, except sections explicitly superseded by items 1 and 2
9. `../../../CONTEXT.md`

`../specs/2026-07-18-p07-p09-execution-rebaseline-design.md` is an archived, superseded Tauri-era rebaseline and is not executable.

Implementation plans may refine mechanics but may not change Browser-only
scope, Cookie/Origin authentication, current Admin single-session policy,
Docker-only runtime, the frontend document-as-contract rule, the extensible
platform-kernel boundary, or the P09 destructive boundary. Any such change
returns to specification review.

## Plan set and dependency order

| ID | State | Plan | Repository | Depends on | Produces |
| --- | --- | --- | --- | --- | --- |
| P01 | complete | `2026-07-15-admin-foundation-verification-plan.md` | backend | approved specs | trusted Go build, strict config, blocking verification |
| P02 | complete | `2026-07-15-admin-database-evolution-plan.md` | backend | P01 | fingerprint/backup tooling, expand/backfill/verify schema, query evidence |
| P03 | complete | `2026-07-15-admin-go-runtime-contracts-plan.md` | backend | P01, P02 expand | process Runtime, Error Module, route registry, Admin Contract Bundle |
| P03.5 | complete | `2026-07-17-admin-docker-stability-closure-plan.md` | backend + frontend | P03 | dynamic discovery, bounded startup, image provenance, Docker recovery proof |
| P04 | complete | `2026-07-15-admin-go-identity-routing-plan.md` | backend | P03.5 | atomic Session Lifecycle, RBAC principal versions, route policy; its client-variant transport is superseded by P08R |
| P05 | complete | `2026-07-15-admin-go-durable-work-realtime-plan.md` | backend | P02, P03.5 | durable AI work, scheduler reconciliation, realtime recovery |
| P06 | complete, user-reviewed | `E:/admin/admin_front_ts/docs/superpowers/plans/2026-07-15-admin-frontend-kernel-plan.md` | frontend | P03, P04 | AppKernel, AuthSession, ApiClient, route registry, persistence |
| P07 Tasks 1-5 | complete, user-reviewed | `E:/admin/admin_front_ts/docs/superpowers/plans/2026-07-15-admin-frontend-realtime-resource-plan.md` | frontend | P05, P06 | RealtimeClient, ResourceQuery, typed workflows, behavior-test migration |
| P08 | historical, superseded | `E:/admin/admin_front_ts/docs/superpowers/plans/2026-07-15-admin-tauri-security-plan.md` | frontend | historical P04/P06/P07 1-5 | completed Tauri security implementation retained only for audit; runtime result removed by P08R |
| P08R | complete, user-reviewed | `2026-07-19-admin-browser-only-tauri-retirement-plan.md` | backend + frontend | approved Browser-only spec, P07 1-5, historical P08 baseline | one Browser-only auth/runtime, Tauri removal, frozen client-version history, Docker cutover proof |
| P07 Tasks 6-10 | complete, user-reviewed | `E:/admin/admin_front_ts/docs/superpowers/plans/2026-07-15-admin-frontend-realtime-resource-plan.md` | frontend | P08R + user P08R acceptance | page decomposition, zero warnings, budgets, WCAG, Docker/manual acceptance |
| P08.5 | cancelled; do not execute | `2026-07-18-admin-tauri-windows-release-plan.md` | none | cancelled by approved Browser-only spec | no artifact |
| Windows local development | complete, user-reviewed | `2026-07-20-windows-local-hot-reload-development-plan.md` | backend + frontend | Gate F + approved Windows design | one-command host HMR with Docker state and unchanged full-Docker delivery |
| P09 | complete; rollback rehearsal explicitly waived by operator | `2026-07-15-admin-only-release-plan.md` | backend + frontend | Gate F, accepted Windows development loop, approved extensible-platform spec, restore proof, fresh user approval | App/Canvas and `client_versions` contract DDL, preserved platform kernel, immutable Docker release proof |

## Dependency graph

```text
P01 Foundation
 ├─→ P02 Database expand/backfill/verify
 └─→ P03 Runtime + contracts
       └─→ P03.5 Docker stability
              ├─→ P04 Identity/RBAC
              ├─→ P05 Durable work/realtime ← P02
              └─→ P06 Frontend kernel ← P04
                    └─→ P07 Tasks 1–5 ← P05
                          └─→ P08 historical Tauri implementation
                                └─→ P08R Browser-only retirement
                                      └─→ P07 Tasks 6–10
                                            └─→ Windows local development
                                                  └─→ P09 current-Admin release with extensible platform kernel

P08.5 Windows candidate release = CANCELLED (no dependency edge)
```

Fixed remaining order:

```text
Windows local-development implementation and acceptance
→ P09 non-destructive prerequisites
→ fresh user approval and live contract DDL
→ P09 release proof and closure
```

P08R, all P07 tasks, and the Windows local-development phase are closed and
user-reviewed. The user opened P09 on 2026-07-20 and later explicitly approved
the live contract migration with `DROP_CLIENT_VERSIONS_FOR_P09`. P09's live
contract and release proof are closed; rollback rehearsal was explicitly
waived for this new-forward-state release.

## Global execution protocol

- [x] **Step 1: Use only the two existing master checkouts**

Use only `E:/admin/admin_back_go` and `E:/admin/admin_front_ts`. Do not create or retain `.worktrees`. Never let two executors edit the same repository concurrently.

- [ ] **Step 2: Record the baseline before each active phase**

```powershell
git -C E:/admin/admin_back_go branch --show-current
git -C E:/admin/admin_back_go rev-parse HEAD
git -C E:/admin/admin_back_go status --short
git -C E:/admin/admin_front_ts branch --show-current
git -C E:/admin/admin_front_ts rev-parse HEAD
git -C E:/admin/admin_front_ts status --short
```

Expected: `master`, one full commit per repository, and no status output.

- [ ] **Step 3: Execute one task inline unless the user requests delegation**

Read the exact active plan task, approved spec, `AGENTS.md`, frontend `docs/rule.md`, and relevant formal contract. Use TDD, stage only declared paths, and create the named focused commit. Do not create a subagent, worktree, or parallel editor unless the user explicitly requests it.

- [ ] **Step 4: Preserve the formal Docker boundary**

Formal Web/API/Worker/MySQL/Redis acceptance and frontend generation/lint/typecheck/test/build run through pinned Docker paths. The approved `admin-dev` exception is Windows development only: MySQL/Redis stay in Docker while Vite/API/Worker run under the repository supervisor with pinned host tools. Playwright is absent unless the user explicitly creates a separate, bounded request for it.

- [ ] **Step 5: Review before accepting each task**

First review spec/plan/contract compliance. Second review code quality, tests, failure behavior, concurrency, secret handling, and architecture. Run the declared verification and inspect the committed diff before checking the task.

## Shared-state ownership

Only one active task may own each row:

| Shared state | Exclusive owner |
| --- | --- |
| live `admin` database, recovery artifact, Atlas revision table | current P02/P08R/P09 database task |
| `database/migrations`, `atlas.sum`, destructive DDL | P09 database owner only |
| P08R menu reconciliation and `client_versions` freeze proof | P08R Task 4 |
| physical `client_versions`/constraint deletion | P09 after fresh approval |
| backend `contracts/admin/v1` and manifest | P08R Task 5, then P09 final-contract task |
| frontend contract snapshot/lock/generated types | P08R Task 6, then P09 consumer task |
| backend Admin auth transport and CORS policy | P08R Task 2 |
| frontend auth adapter/environment/common headers | P08R Task 6 |
| frontend package.json/lockfile and Tauri deletion | P08R Task 7, then P07 Task 10 |
| browser navigation/download helpers and retired client-version UI | P08R Task 8 |
| Docker delivery files and live Compose projects | active cutover/release task only |
| P07 page decomposition, locale split, budgets, accessibility | P07 Tasks 6-10 after P08R acceptance |
| `.tmp/dev/admin-dev.lock.json`, private Air and dependency stamp | Windows local-development supervisor only |

Read-only database checks may run only after the current database owner records the fingerprint. No task writes COS candidate/updater objects; P08R preserves historical objects and P09 decides their disposition.

## Commit discipline

Each task ends with one focused commit. Generated contracts, runtime code, data reconciliation, frontend consumer sync, dependency deletion, and execution evidence remain separate commits. Use literal paths:

```powershell
git add -- path/declared/by/the/task
git diff --cached --check
git diff --cached
git commit -m "the task's exact message"
```

Never use `git add -A`, `git reset --hard`, `git checkout --`, an unreviewed migration replay, or a destructive cleanup command.

## Program gates

- [x] **Gate A:** P01 passes from a clean module cache and both backend binaries build.
- [x] **Gate B:** P02 restores a verified recovery artifact and imported/empty schemas converge.
- [x] **Gate C:** P03 publishes a deterministic Admin Contract Bundle and both processes pass lifecycle tests.
- [x] **Gate C.5:** P03.5 proves dynamic discovery, bounded state-late startup, image revisions, zero-exit SIGTERM, and volume-preserving recovery.
- [x] **Gate D:** P04 proves one-winner refresh, session/RBAC invalidation, and route-policy completeness. Its browser/desktop presentation is historical and does not satisfy P08R.
- [x] **Gate E:** P05 proves durable work recovery, scheduler lease safety, and realtime resume/deduplication.
- [x] **Gate F:** P08R plus P07 Tasks 6-10 prove one Browser-only contract, no Tauri/client-variant/client-version runtime, Docker-only delivery, zero-warning frontend quality, budgets, WCAG behavior, smoke, and user acceptance.
- [x] **Gate G:** P09 removed the approved retired product/schema surfaces—including frozen `client_versions` after fresh approval—preserved and proved the extensible authenticated platform kernel, and produced the immutable Docker release proof. The prior 12-gate artifact proof and fresh post-fix contract/live/artifact checks passed; rollback rehearsal was explicitly waived by the operator and is not represented as passed evidence.

P08 and P08.5 are not active Gate F deliverables. No later gate waives an earlier one. P09's destructive boundary was crossed only after recovery restore, frozen-table evidence, COS disposition, contract lock, Docker image revision, and explicit user approval were present. The rollback runbooks remain available, but this release records the user's explicit waiver instead of a rehearsal result.

## Completed evidence summary

### Gate A-C.5 (2026-07-17)

- P01 clean-cache Go verification, race, vet, staticcheck, govulncheck, and both binaries passed.
- P02 empty/imported schemas converged; the retained recovery artifact was restored and hash/count verified.
- P03 runtime/contract gates passed and produced a deterministic manifest.
- P03.5 Docker stability tests passed API-address replacement, state-late recovery without restart loops, revision-label checks, SIGTERM exit `0`, and five-container restoration without deleting volumes.

### Gate D (2026-07-17)

- P04 ran directly on backend `master` with no worktree.
- Refresh CAS, session ceilings, multi-node revoke/user/role invalidation, route-policy completeness, and contract generation passed.
- Its client-variant transport evidence is retained as history but is explicitly replaced by P08R.

### Gate E (2026-07-18)

- P05 durable task ownership, provider attempts, fenced leases, scheduler reconciliation, typed realtime recovery, Docker kill/restart, and full backend/database gates passed.

### Gate F complete evidence (2026-07-18 through 2026-07-20)

- P06 AppKernel/AuthSession/ApiClient/routes/persistence and login-route regression fixes were implemented and manually reviewed.
- P07 Tasks 1-5 completed typed realtime/resources/mutations/workflow migration and behavior-test conversion; exact evidence remains in the P07 plan.
- P08 completed the now-retired Tauri security implementation. Its task checkboxes and commits remain audit history only.
- P08R completed one Browser-only Cookie/Origin transport, removed Tauri and
  client-version runtime surfaces, froze `client_versions`, passed backend,
  database, Docker API/realtime, contract, frontend type/test/build gates, and
  received explicit user functional acceptance on 2026-07-20.
- P07 Task 6 decomposed oversized pages and closed ESLint at 0 errors / 0 warnings across commits `c863919`, `d644418`, `d853a93`, `10cca02`, and `51bd37d`.
- P07 Task 7 commit `a75989b` enforced lazy boundaries, generated locale parity, and exact compressed budgets.
- P07 Task 8 commit `870cc79` added WCAG 2.2 AA critical-flow behavior and zero serious/critical axe findings.
- P07 Task 9 commit `74e98fd` passed five-container health/revision/auth/realtime smoke and recorded the user-owned manual checklist.
- P07 Task 10 commit `0858edd` made Browser-only, contract, routes, locale, zero-warning lint, coverage, typecheck, build, bundle, architecture, and dependency audit blocking in Docker.
- The user explicitly accepted the P07 functional stage on 2026-07-20; Gate F is closed.

### Windows local-development phase (completed and user-reviewed 2026-07-20)

- Approved design and implementation plan are `2026-07-20-windows-local-hot-reload-development-design.md` and `2026-07-20-windows-local-hot-reload-development-plan.md`.
- Node/npm formal and host baselines are now exact `24.18.0` / `11.16.0`; Go remains exact `1.26.5 windows/amd64`, and Air is private/pinned at `1.66.0`.
- Static fixtures passed; real Vite HMR, API/worker Air rebuild, host-port release, lock release, state-container preservation, and full five-container restoration were verified without browser automation.
- Implementation commits include frontend `f83fee8` and backend `fc6a00a`, `653ecef`, `8fbb381`, `823d809`, `9b59ead`, plus focused acceptance fixes through `9a4aaf8`.
- The user accepted the local-development loop and explicitly opened P09 on
  2026-07-20. This acceptance does not authorize P09 destructive DDL.

### Gate G verification evidence (2026-07-22)

- The approved disposable P09 contract removed the frozen `client_versions`
  surface and converged both restore schemas plus empty/imported verification
  paths to fingerprint
  `9a019819051e7252cb09c4c4ea56cd3b285aedb4362d49f6ec95f80299604678`.
- Release `admin-v2026.07.21.1` passed all 12 immutable release gates against
  the prior backend commit and frontend `95937a8958ef41e12b15a5547a859c07a43d05c8`;
  the proof contains no failed gate and completed in `2,995,071 ms`. It remains
  the quality baseline for the current release.
- Release `admin-v2026.07.22.1` was regenerated from the final clean `master`
  checkouts after the live contract fix. Its current backend/frontend image
  IDs, archive digests, target fingerprint, and platform-kernel proof are
  recorded in `release/admin-only/out/release-manifest.json` and passed
  `check-release-manifest.ps1`.
- The platform-kernel proof, Admin Contract Bundle, Browser-only retirement,
  backend/database/runtime/frontend quality, sensitive-material scan, and
  image/archive integrity all passed. The live groups `201/202/203`, checks
  `050`-`053`, drift, COS disposition, and all six platform invariants passed;
  Docker stability used an isolated no-host-port project while `admin-dev`
  remained available.
- No frontend source change was required: the existing P09 contract consumer
  passed the fresh Docker quality gate. The user explicitly waived application
  and database rollback rehearsal, so the closure records no rollback result.
