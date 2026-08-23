# Wave 03 Remaining Modules Implementation Plan

> **For agentic workers:** Use the assigned backend/frontend worktree only. Do not modify the source master checkouts.

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
- The local database external-ownership cutover occurs only after Wave 03 implementation and black-box acceptance are complete; it is not part of any worker task.

## Worker Boundaries

| Window | Worktree | Backend files | Frontend files |
|---|---|---|---|
| Mail | `wave03-mail-back`, `wave03-mail-front` | backend `internal/module/mail/**` | frontend `src/api/system/mail.ts`, `src/api/system/mailDiagnostics.ts`, and existing mail views only |
| SMS | `wave03-sms-back`, `wave03-sms-front` | backend `internal/module/sms/**` | frontend `src/api/system/sms.ts` and existing SMS views only |
| Logs | `wave03-log-back`, `wave03-log-front` | backend `internal/module/systemlog/**`, `internal/module/operationlog/**` | frontend `src/api/system/log.ts`, `src/api/system/operationLog.ts`, and existing log views only |
| UploadConfig (coordinator) | source master during coordination, then reviewed integration | `internal/module/uploadconfig/**` | `src/api/system/uploadConfig.ts`, `src/api/system/uploadConfig.types.ts`, `src/views/Main/system/uploadConfig/**` |

The SMS branch already existed before this plan. It must not be deleted, reset, renamed, or overwritten; the coordinator must report that conflict before starting the SMS window. The same rule applies to any pre-existing target directory.

## Execution Tasks

### Task 1: Worker implementation

- [ ] Each worker verifies its worktree starts at the given master commit and is clean.
- [ ] Each worker writes focused failing tests for the module's existing read/list/page-init and mutation/error behavior, then makes the minimum implementation change.
- [ ] Each worker runs only its package tests and frontend type/build check, formats touched files, and commits its own branch.
- [ ] Any schema, migration, seed, baseline, or forbidden-file requirement is reported as blocked; no workaround is added.

### Task 2: UploadConfig implementation

- [ ] Coordinator tests and fixes only `internal/module/uploadconfig/**` and the declared frontend UploadConfig files.
- [ ] Verify driver, rule, and setting page-init/list/create/update flows, validation, encrypted-secret projection, permission failures, and not-found behavior against existing contracts.
- [ ] Run focused backend tests and the frontend type/build check; do not touch uploadtoken or COS runtime.

### Task 3: Review and integration

- [ ] Review all six worker diffs for boundary violations, compatibility breaks, silent defaults, and unnecessary abstractions.
- [ ] Run the plan's focused short tests for all four modules and frontend checks.
- [ ] Merge reviewed backend/frontend commits into their respective `master` branches; preserve unrelated user changes and never reset or checkout over them.
- [ ] Update `docs/superpowers/plans/2026-08-13-admin-architecture-reduction-execution-index.md` to mark Wave 03 fully complete only after black-box acceptance evidence is recorded.
- [ ] Provide a user-facing black-box acceptance checklist for Mail, SMS, system logs, operation logs, and UploadConfig.
- [ ] Stop after Wave 03. Do not plan, implement, or start Wave 04.

## Completion Gate

Wave 03 is complete only when every worker diff is reviewed, focused tests and frontend checks pass, both master branches contain the reviewed commits, the execution index says Wave 03 complete, and the user has the black-box checklist. Database ownership cutover remains a later, separately gated activity.
