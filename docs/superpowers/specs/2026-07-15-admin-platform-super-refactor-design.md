# Admin Platform Super Refactor Design

**Status:** Approved in conversation on 2026-07-15; written for final file review.

**Scope owners:** `admin_back_go` and `admin_front_ts`

**Goal:** Produce one complete, deployable, observable, high-confidence Admin platform whose reusable Capability Modules can support the immediately following SaaS phase without preserving the retired App/Canvas product-platform lines.

## Decision summary

The program uses a foundation-first hybrid strategy:

1. Restore verifiable foundations serially: environment, database evolution, dependency integrity, contract truth, and CI.
2. Deepen Go and TypeScript Modules behind explicit interfaces.
3. After foundation gates pass, execute backend, frontend, and database-performance workstreams in parallel with isolated subagents.
4. Preserve reusable business capability implementation; delete App/Canvas transport, workflow, route, permission, and dead platform code.
5. Do not implement tenants in this program. The next design cycle adds SaaS ownership through `TenantScope` without reopening the Capability Module seams created here.

The separate `E:/admin/canvas_front_next` repository is outside this program and must not be modified or deleted.

## Why this is not a rewrite

A big-bang rewrite would combine unknown schema state, behavior migration, and architecture change into one untestable event. A shallow incremental cleanup would leave the same lifecycle, contract, and data-evolution failures in place. This design instead establishes proof-producing foundations and then performs aggressive internal refactoring behind preserved Admin behavior.

Breaking changes are allowed for internal Go interfaces, TypeScript interfaces, database schema, and retired App/Canvas contracts. Active Admin HTTP behavior may change only through an explicit contract change documented in the design/implementation plan and updated atomically in both repositories. This design explicitly approves the secure browser/desktop refresh-credential cutover, versioned Admin Contract Bundle, and realtime resume envelope described by the backend/frontend sub-specs.

## Verified starting facts

### Repository and build facts

- The repositories are separate Git repositories on `master`; the workspace root is not a Git repository.
- Backend full verification is blocked by an invalid `github.com/hibiken/asynqmon@v0.7.2` content checksum in `go.sum`. The public checksum is `h1:YohWgTIPwtMyZ6khBDcVUz9BdSdQW2Dxn8SoxtbmjSg=`.
- Backend has no active CI workflow.
- Frontend tests, typecheck, quality checks, and production build pass, but ESLint reports 3 errors and 3,619 warnings.
- The active frontend deployment workflow builds without running typecheck, tests, lint, browser smoke, or Rust verification.
- Docker and Rust are not installed in the current workstation environment. The implementation plan must either install approved tooling or run those gates in pinned containers/CI.

### Database facts

- MySQL 8.4.10 contains an approximately 15 MB `admin` database with 52 tables and no migration history table.
- The live schema is a mixed historical version, not the schema required by current code.
- Missing required columns include `ai_runs.platform`, `ai_runs.input_snapshot`, `export_tasks.platform`, `export_tasks.kind`, `export_tasks.object_key`, `ai_image_tasks.platform`, and `user_wallets.total_consume_cents`.
- Missing required tables include `ai_assets`, `ai_text_tasks`, `ai_image_files`, `canvas_video_tasks`, and `payment_callback_events`.
- `users_quick_entry` and legacy AI image tables still exist.
- Historical SQL files contain destructive DDL and cannot safely be replayed in lexical filename order.

## Target architecture

```text
Admin HTTP / Worker Transport Adapters
                  │
                  ▼
          Admin Workflow Modules
                  │
                  ▼
           Capability Modules
                  │
        ┌─────────┼──────────┐
        ▼         ▼          ▼
   MySQL/Redis  Provider    Queue/Realtime
     Adapters   Adapters       Adapters
```

The target optimizes for depth:

- A Capability Module exposes a small interface and hides validation, invariants, transaction ordering, idempotency, and failure semantics.
- An Admin Workflow Module owns Admin-specific orchestration without leaking transport DTOs into a capability interface.
- A Transport Adapter owns Gin, Asynq, WebSocket, or external-provider protocol details.
- A seam is introduced only when two real adapters exist or when a process/testing variation is already required. No speculative Factory/Manager/Impl hierarchy is allowed.

## Admin-only product line

The only active HTTP prefix after this program is `/api/admin/v1`, plus intentionally public health and provider callback endpoints required by Admin behavior.

The implementation removes:

- `/api/app/v1` and `/api/canvas/v1` route registration;
- `transport/app` and `transport/canvas` packages;
- App/Canvas authentication entries, permission seeds, route metadata, and active smoke assertions;
- the `internal/module/canvas` product-platform Module;
- product-platform DTOs and presenters that add no reusable capability behavior;
- dead compatibility names and historical fallback paths.

Reusable AI image, video, audio, asset, prompt, storage, provider, and run implementation may remain only after Canvas terminology is removed from its capability interface and direct tests prove the implementation independently of a retired transport. An implementation with no Admin use, no immediate SaaS reuse, and no meaningful independent test is deleted.

## Program workstreams

### Workstream A — Foundation and database evolution

Defined by `2026-07-15-admin-foundation-database-design.md`.

It establishes environment ownership, schema fingerprinting, live-schema reconciliation, version/checksum tracking, migration locking, drift gates, dependency integrity, and database query baselines. No architecture implementation proceeds against a schema that current code cannot use.

### Workstream B — Go architecture deepening

Defined by `2026-07-15-admin-go-architecture-design.md`.

It deepens Runtime, Session Lifecycle, Route Policy Registry, Conversation Reply Command, queue/scheduler control, Realtime Delivery, Error, and telemetry Modules while reducing the active system to Admin.

### Workstream C — Frontend architecture deepening

Defined in `E:/admin/admin_front_ts/docs/superpowers/specs/2026-07-15-admin-frontend-super-refactor-design.md`.

It deepens AppBootstrap, AuthSession, HTTP contract/error, RealtimeClient, RuntimeRouteRegistry, ResourceQuery, Persistence, and NativeBridge Modules; it also replaces source-string-heavy testing with executable behavior tests.

## Required execution order

### Gate 0 — Preserve a recoverable starting point

- Copy `deploy/docker-first/admin-go.env.example` to the ignored `admin-go.env` and fill local MySQL, Redis, APP_SECRET, and CORS values.
- Produce a schema fingerprint and automated recovery dump before destructive DDL. The user does not need to operate or manage this artifact; the workflow owns it.
- Record current Git commits, database version, table/column/index inventory, row counts, and COS object references involved in migration.

### Gate 1 — Make verification trustworthy

- Correct dependency checksums from a trusted source without disabling `GOSUMDB`.
- Add local and CI verification entrypoints shared by developers and pipelines.
- Publish and checksum the OpenAPI, permission/view, and realtime artifacts as one Admin Contract Bundle; lock the frontend to its exact manifest.
- Make backend tests/build, frontend lint/typecheck/test/build, migration lint, and contract checks blocking.
- Deploy only an immutable artifact produced by the passing verification workflow.

### Gate 2 — Reconcile the database

- Align the imported database to the current Admin target through staged, reviewed reconciliation.
- Verify row counts, wallet ledger balances, RBAC references, payment references, AI migration mappings, and COS object reachability.
- Establish the versioned migration baseline only after target-schema equality is proven.

### Gate 3 — Deepen shared foundations

- Implement Runtime, Error, telemetry, contract, identity, and route-policy Modules.
- Preserve the Admin route snapshot unless a documented Admin contract change is approved.

### Gate 4 — Parallel domain execution

- Backend capability/runtime tasks, frontend Module tasks, and query/index tasks may run in parallel only when their file ownership and schema dependencies do not overlap.
- Database migrations and shared contracts remain serialized integration points.

### Gate 5 — Remove retired platform code

- Delete App/Canvas transports, routes, metadata, permissions, and dead code after Admin contract and capability tests cover retained behavior.
- Execute destructive schema contract changes only after expand/backfill verification passes.

### Gate 6 — Release proof

- Run full unit, integration, browser, database, image, and smoke gates.
- Capture performance and query-plan deltas against the pre-change baseline.
- Produce a runbook for deployment, rollback, secret handling, schema status, and observability.

## Error and reliability model

Every failure crosses one stable Error Module interface containing:

- stable business code;
- HTTP/task classification;
- retryable or permanent category;
- safe localized message;
- internal cause;
- request, task, run, and trace identifiers when applicable.

Transport Adapters map this interface to HTTP or queue semantics. A 5xx cause is logged exactly once. Secrets, tokens, provider credentials, certificates, private prompts, and full sensitive payloads are never emitted to logs or metrics.

Durable work uses database-backed state, idempotency keys, claim/lease semantics, and explicit terminal states. Process-local goroutines or maps are not a durability mechanism.

## Verification architecture

Backend gates:

- clean module-cache download with public checksum verification;
- `go test ./...` and focused race tests for session, command, payment, and queue state;
- `go vet`, static analysis, vulnerability scan, both binary builds, route-policy completeness, migration lint, drift check, and container build;
- MySQL/Redis integration tests for transaction, CAS, idempotency, and lease behavior.

Frontend gates:

- ESLint with zero warnings, TypeScript build, Vitest, Vue behavior tests, production build, and enforced performance budgets;
- Playwright smoke for login, refresh, logout, RBAC routing, CRUD latest-wins, notification, and AI realtime completion/cancel;
- Rust formatting, tests, Clippy, dependency audit, and Tauri build in a pinned environment.

Database gates:

- empty-database creation to the exact target schema;
- imported-database reconciliation to the same fingerprint;
- repeat execution without drift;
- data invariants and orphan checks;
- `EXPLAIN ANALYZE` before/after evidence for every new or removed performance index.

## Quantitative completion criteria

- One active product platform: Admin.
- Zero active App/Canvas routes and permission seeds in the backend.
- Backend full test/build gate passes from a clean dependency cache.
- Frontend lint has 0 errors and 0 warnings; typecheck, tests, browser smoke, and build pass.
- Live, empty, and CI database fingerprints match.
- Every active route is classified Public, Authenticated, or Permission and every mutation has an explicit audit decision.
- The frontend contract lock resolves to the exact backend Admin Contract Bundle and contains no App/Canvas operation.
- API and Worker stop gracefully under SIGTERM and release all owned resources.
- AI reply survives API process termination and supports cross-node cancellation.
- Concurrent refresh has a single winner and session limits hold under contention.
- Critical list and worker queries meet recorded query-plan and latency budgets.
- No secret or sensitive payload appears in response, log, metric, test artifact, or repository history.

## Subagent execution contract

Implementation uses `superpowers:subagent-driven-development` with a fresh implementation subagent per plan task. Each task receives exact file ownership, tests, commands, and expected output. Completion requires two reviews before integration:

1. spec-compliance review;
2. code-quality and test-quality review.

The root agent independently inspects the diff and reruns the declared verification. Subagents do not concurrently edit the same file, migration directory, generated contract, or shared database state.

## SaaS handoff

The next design begins only after all completion criteria above pass. That design introduces Tenant ownership, isolation, provisioning, plans, quotas, metering, tenant-aware authorization, tenant-scoped uniqueness/indexes, and product-platform workflows.

This program must leave the following extension path without implementing it:

```text
Future SaaS Transport Adapter
          │
Future Platform Workflow
          │
Existing Capability Module
          │
Tenant-aware adapter introduced by the SaaS spec
```

No `tenant_id` column, tenant switcher, organization table, or speculative tenant interface is added in this refactor.
