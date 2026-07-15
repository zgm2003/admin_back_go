# Admin Go Architecture Deepening Design

**Status:** Approved in conversation on 2026-07-15; written for final file review.

**Goal:** Turn `admin_back_go` into one Admin-only modular monolith with deep Capability Modules, durable multi-process behavior, strict route policy, atomic identity, and explicit extension seams for the next SaaS phase.

## Constraints

- Remain a modular monolith. No microservice extraction occurs in this program.
- Keep Go idiomatic: no `ServiceImpl`, no meaningless interfaces, no generic Manager/Factory hierarchy, and no transport framework in domain logic.
- Admin is the only active Product Platform.
- Internal interfaces and package layout may break aggressively.
- Active Admin contract changes require synchronized backend contract, frontend adapter, tests, and migration updates.
- Tenant ownership is not implemented yet.

## Target source shape

```text
cmd/
  admin-api/              # signal-aware process entry
  admin-worker/           # signal-aware process entry
internal/
  runtime/                # process lifecycle and capability graph
  platform/
    admin/                 # Admin workflow assembly and route registration
  module/                  # reusable Capability Modules
  infra/                   # MySQL, Redis, provider, queue, storage adapters
  shared/                  # narrow cross-capability primitives only
```

This is a responsibility map, not an instruction to move every file mechanically. Moves occur only when they increase locality or remove retired platform code.

## Runtime Module

The current 579-line `bootstrap.New` function and approximately 50-field router dependency struct become a deep Runtime Module.

The Runtime Module interface is conceptual and small:

```text
Start(context) error
Shutdown(context) error
Health(context) Report
```

Concrete API and Worker runtime implementations own:

- normalized, strictly validated configuration;
- secret key derivation;
- database/Redis/queue/realtime resource creation;
- capability and workflow assembly;
- startup ordering and readiness;
- signal-driven graceful shutdown in both process entries;
- reverse-order cleanup with joined errors;
- process-specific required capability validation.

Resource initialization is all-or-nothing for required dependencies. An API process cannot continue assembling nil repositories after a database open failure. Optional capabilities declare explicit disabled behavior in health output.

The API and Worker share builders for AI engines, payment adapters, secret handling, and common Capability Modules. They do not duplicate subtly different runtime graphs.

## Capability and Admin Workflow separation

A Capability Module owns stable business invariants and persistence operations. It accepts canonical commands/queries and returns canonical results or the Error Module interface.

An Admin Workflow Module owns:

- Admin-specific orchestration;
- Admin policy composition;
- mapping between multiple capabilities for a user-visible operation;
- Admin-specific defaults that are not capability invariants.

A Gin Transport Adapter owns:

- route path and method;
- request parsing and validation;
- authenticated actor extraction;
- mapping to canonical command/query;
- HTTP response presentation.

Capability Modules never import Gin and never switch on `platform`. Admin Workflow Modules do not expose GORM models or provider SDK types.

## Published Admin contracts

The backend produces one versioned Admin Contract Bundle containing:

- an OpenAPI 3.1 document for active Admin HTTP operations and stable error envelopes;
- the permission-code catalog and route/view metadata contract;
- JSON schemas for realtime envelopes and event payloads;
- a manifest containing schema versions, the backend Git commit, and SHA-256 for every artifact.

Generation is deterministic and checked into the backend repository. Frontend integration records the exact bundle manifest/digest it consumes and regenerates TypeScript transport types from that bundle. CI fails on stale generated artifacts, an undocumented breaking change, a permission/view mismatch, or a realtime schema mismatch. App/Canvas operations never appear in the active bundle.

## Admin-only retirement

The active router registers only Admin transports plus required public system/payment callback endpoints. The implementation deletes App/Canvas route aggregators and transports in a dependency-safe order.

AI code is classified before deletion:

- provider-agnostic generation, run recording, storage, tool, knowledge, and asset behavior is capability implementation;
- Canvas request DTOs, route names, presenters, settings, and table names are retired platform implementation;
- capability tables and Go names using `canvas_` are renamed to generic `ai_` names when the capability remains;
- a capability with no Admin use and no approved immediate SaaS reuse is deleted with its schema after data verification.

Architecture guards fail when active code registers `/api/app/` or `/api/canvas/`, imports retired transports, or seeds retired platform permissions.

## Route Policy Registry

Every route is registered with exactly one access policy:

```text
Public
Authenticated
Permission(code)
```

Each mutation additionally declares one audit decision:

```text
Audit(module, action, title)
NoAudit(reason)
```

Policy lives next to route registration. A central compiled registry may be produced for middleware lookup and tests, but hand-maintained duplicate maps are removed. Duplicate routes, missing policies, unknown permission codes, and unclassified mutations fail startup or tests.

Authorization decisions use a versioned principal snapshot keyed by user, role, Admin platform, and permission version. A cache hit performs no SQL. Role, permission, or user status changes invalidate the affected version atomically. Cache failures fail closed for Permission routes.

## Session Lifecycle Module

The current large session file is separated by behavior, not by arbitrary line count. The external interface is:

```text
Issue
Authenticate
Rotate
Revoke
List/RevokeForAdmin
```

Implementation requirements:

- MySQL is the session truth; Redis is a cache/revocation acceleration adapter.
- Issue enforces single/max-session policy in one transaction using appropriate locks.
- Rotate uses the previous refresh hash as a compare-and-swap condition or locks the session row; concurrent refresh has one winner.
- Authenticate explicitly verifies every persisted token property that is part of the contract. Unused access-token hashes are either enforced or removed through migration.
- Revoke publishes cache invalidation and is effective across API nodes within the documented SLA.
- Secret rotation is an explicit runbook operation, never a silent config edit.

Credential transport is an Admin client-variant concern around the same Session Lifecycle:

- browser login/refresh uses an `HttpOnly`, `Secure`, `SameSite=Strict` refresh cookie scoped to the Admin auth path; responses expose only the short-lived access credential to JavaScript, and state-changing cookie operations validate the Origin;
- packaged desktop login yields a rotating refresh credential that the local verified client immediately seals through OS-protected native storage; subsequent refresh exchange uses the sealed native adapter and never persists the credential in DOM storage;
- both variants use the same database session, rotation CAS, revocation, device, expiry, and audit invariants;
- the legacy JS-readable refresh-cookie contract is removed atomically with the frontend AuthSession change and may require one sign-in after deployment.

## Durable Conversation Reply Command

Sending a user message and creating the durable reply command occur in one database transaction. The HTTP request returns after commit, not after starting a local goroutine.

The command state machine is:

```text
pending → claimed → running → succeeded
                     ├──────→ failed
                     └──────→ canceled
claimed/running --lease expiry--> retryable pending or terminal timeout
running --ambiguous provider outcome--> outcome_unknown
outcome_unknown --reconcile--> succeeded, failed, or retryable pending
```

Requirements:

- one idempotency identity covers conversation, user message, and request ID;
- Worker claims use compare-and-swap, lease expiry, owner identity, and a fencing token; a stale owner cannot publish after losing its lease;
- cancel updates durable state and signals the active Worker through Redis when available;
- API termination after commit does not lose work;
- Worker and synchronous test adapters use the same tool, knowledge, provider, run-recorder, and error behavior;
- each provider attempt is persisted before dispatch and reuses a provider idempotency key when supported;
- an ambiguous timeout/disconnect is marked `outcome_unknown` and reconciled before retry; the Worker never blindly creates a second attempt when provider-side exactly-once behavior cannot be proven;
- local assistant-message publication is idempotent, so duplicate delivery cannot create a second effective local result or an orphan assistant message.

The existing Asynq adapter may deliver wake-up commands, but database state is the durability truth. The process-local dispatcher is removed.

## Queue and Scheduler control

Task types are owned by a TaskRegistry with explicit payload schema, retry class, queue, timeout, uniqueness, and handler.

Errors are classified Permanent or Retryable through the Error Module. Invalid payloads and invariant violations do not consume retries. Transient network/provider failures use bounded retry with observable exhaustion.

A SchedulerReconciler continuously reconciles enabled database schedules with the running scheduler. Create/update/status/delete changes become effective within five seconds without process restart. Unknown enabled tasks make scheduler health unhealthy instead of producing a warning and silently disappearing.

Multi-Worker scheduling uses a renewable lease with owner identity and fencing token. A fixed TTL without renewal is not sufficient for work exceeding the lock period.

## Realtime Delivery Module

Realtime distinguishes:

- ephemeral events: typing/progress/delta where occasional loss may be acceptable;
- durable terminal events: notification creation, AI completed/failed/canceled, and other state transitions recoverable from persisted truth.

The Module interface owns event ID, event type, monotonic sequence/cursor, occurrence time, request ID, target, durability class, and payload schema. Topic subscription is real session state and controls delivery. Redis Pub/Sub remains an adapter for ephemeral fan-out, not the source of durable truth.

On reconnect, the client can recover terminal state using a cursor or domain query. Slow-client disconnects, send-buffer overflow, invalid subscriptions, Redis failure, and dropped ephemeral events produce metrics.

## Error Module

The Error Module replaces coarse ad hoc application errors with a stable interface containing:

- machine code;
- category: validation, authentication, authorization, not-found, conflict, rate-limit, dependency, timeout, internal, canceled;
- HTTP status where relevant;
- retryability for background work;
- safe localized message key/data;
- internal cause;
- operation metadata safe for telemetry.

Repositories and provider adapters preserve causes. Transport Adapters map errors once. Unknown errors become internal errors and are logged exactly once with request/task/run context.

## Telemetry

Structured logs, metrics, and tracing cover:

- HTTP duration/status/error code;
- SQL duration and slow-query digest;
- Redis duration/error;
- queue latency, execution, retry, lease expiry, and dead tasks;
- provider first-byte/total latency, token usage, status, and sanitized error class;
- active WebSockets, reconnects, dropped events, and send-buffer pressure;
- scheduler reconciliation and lease ownership.

Telemetry adapters must be replaceable without Capability Module edits. The first implementation may use Prometheus/OpenTelemetry-compatible adapters, but the interface and redaction rules are mandatory regardless of backend.

## Configuration

Configuration parsing returns errors rather than silently falling back on invalid values. Defaults are allowed only when documented as product defaults. Secrets and resource endpoints required by an enabled capability are validated before resource creation.

Config is immutable after startup except for explicitly database-owned runtime policy. Docker environment files contain resource/process settings; business configuration stays in owned tables.

## Testing strategy

- Runtime tests inject real alternative adapters only where variation exists and verify lifecycle order/failure cleanup.
- Route tests prove complete classification, no retired routes, no duplicate metadata, and unchanged approved Admin snapshot.
- Session integration tests use MySQL/Redis and concurrent clients.
- Conversation command tests kill/restart workers and exercise duplicate delivery/cancel/lease expiry.
- Scheduler tests run multiple reconcilers against the same Redis/MySQL state.
- Realtime contract tests cover order, duplicate, loss, resume, subscription, and slow clients across multiple API instances.
- Error tests prove response safety and cause preservation.

## Completion criteria

- `admin-api` and `admin-worker` share consistent capability assembly and both shut down cleanly under signals.
- No required dependency failure produces a partially assembled running process.
- No active App/Canvas route, transport, permission, or product-platform module remains.
- Every active route has access and audit classification.
- Concurrent refresh and session policy tests pass under race.
- AI reply is durable across API/Worker termination and cross-node cancellation.
- Scheduler changes reconcile without restart and multi-Worker duplicates are prevented.
- Realtime delivery semantics are documented and tested.
- Error causes are observable internally and never leak externally.
- Existing Admin behavior and synchronized frontend contracts pass full smoke.
