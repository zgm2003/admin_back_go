# Wave 03 Remaining Modules Implementation Plan

> **For the execution agent:** Work directly in the two source `master` checkouts. This personal-development plan uses one coordinator/executor window; do not create worker worktrees or branches.

**Goal:** Complete Mail, SMS, system/operation logs, and UploadConfig as the remaining Wave 03 slices while preserving the current API, database, permission, and user-facing contracts.

**Architecture:** Keep the existing modular-monolith path `route -> handler -> service -> repository -> model` and the frontend path `views -> api -> request -> backend`. Changes are limited to each module's declared files; no generated contract layer or new shared abstraction is introduced.

**Tech Stack:** Go, Gin, GORM, Vue 3, TypeScript, existing project test/build tooling.

---

## Global Rules

- This plan ends Wave 03. It explicitly does not enter Wave 04.
- Do not modify Payment, AI, Worker, Realtime/WebSocket, COS runtime, uploadtoken, `component/upload`, or shared files unless a task below names the exact file.
- Do not create migration, seed, baseline, backup, export, restore, or database-maintenance files.
- Do not execute SQL or remote database operations. If an existing table cannot support the required behavior, stop and report the exact schema dependency to the coordinator.
- Preserve existing routes, request/response fields, permission codes, menu behavior, and database column semantics. No silent fallback or guessed DTO field is acceptable.
- The local database external-ownership cutover is a prerequisite for this plan. It must be completed and short-verified after Permission + AuthPlatform acceptance and before any remaining Wave 03 worker starts; it is not part of any worker task.

## Worker Boundaries

| Module | Backend scope | Frontend scope |
|---|---|---|---|
| Mail | backend `internal/module/mail/**` | frontend `src/api/system/mail.ts`, `src/api/system/mailDiagnostics.ts`, and existing mail views only |
| SMS | backend `internal/module/sms/**` | frontend `src/api/system/sms.ts` and existing SMS views only |
| Logs | backend `internal/module/systemlog/**`, `internal/module/operationlog/**` | frontend `src/api/system/log.ts`, `src/api/system/operationLog.ts`, and existing log views only |
| UploadConfig | backend `internal/module/uploadconfig/**` | frontend `src/api/system/uploadConfig.ts`, `src/api/system/uploadConfig.types.ts`, `src/views/Main/system/uploadConfig/**` |

The executor may process independent module files concurrently, but must keep each module's diff reviewable and commit the backend and frontend changes to their respective `master` branches only after all focused checks pass. No separate worker window is part of this plan.

## Execution Tasks

### Task 1: Worker implementation

- [ ] The executor verifies both source `master` checkouts are clean before each module batch.
- [ ] For each module, write focused failing tests for existing read/list/page-init and mutation/error behavior, then make the minimum implementation change.
- [ ] Run only the module package tests and frontend checks, format touched files, and commit the reviewed batch to `master`.
- [ ] Any schema, migration, seed, baseline, or forbidden-file requirement is reported as blocked; no workaround is added.

### Task 2: UploadConfig implementation

- [ ] Coordinator tests and fixes only `internal/module/uploadconfig/**` and the declared frontend UploadConfig files.
- [ ] Verify driver, rule, and setting page-init/list/create/update flows, validation, encrypted-secret projection, permission failures, and not-found behavior against existing contracts.
- [ ] Run focused backend tests and the frontend type/build check; do not touch uploadtoken or COS runtime.

### Task 3: Review and integration

- [ ] Review each module diff for boundary violations, compatibility breaks, silent defaults, and unnecessary abstractions.
- [ ] Run the plan's focused short tests for all four modules and frontend checks.
- [ ] Keep reviewed backend/frontend commits on their respective `master` branches; preserve unrelated user changes and never reset or checkout over them.
- [ ] Update `docs/superpowers/plans/2026-08-13-admin-architecture-reduction-execution-index.md` to mark Wave 03 fully complete only after black-box acceptance evidence is recorded.
- [ ] Provide a user-facing black-box acceptance checklist for Mail, SMS, system logs, operation logs, and UploadConfig.
- [ ] Stop after Wave 03. Do not plan, implement, or start Wave 04.

## Completion Gate

Wave 03 is complete only when the database external-ownership cutover was completed before this plan, every worker diff is reviewed, focused tests and frontend checks pass, both master branches contain the reviewed commits, the execution index says Wave 03 complete, and the user has the black-box checklist.
