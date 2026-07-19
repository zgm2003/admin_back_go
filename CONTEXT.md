# Admin Platform Domain Context

This repository currently delivers one product platform: **Admin**. Its approved
P08R product shape is one Browser-only client; the currently published bundle is
still the historical browser/desktop variant contract until P08R Task 5 activates
the replacement. `app` and `canvas` are retired product-platform lines during the
2026-07 super refactor. The next product direction prioritizes integrating a
concrete additional platform supplied by the user; SaaS/tenant behavior remains
lower priority and outside the current implementation scope. No generic future
platform framework is built before that concrete project and contract exist.

## Domain language

- **Admin Platform** — the only deployable product platform after this refactor. It owns the `/api/admin/v1` transport contract and the `admin_front_ts` client.
- **Capability Module** — reusable business behavior such as identity, AI, wallet, payment, storage, notification, export, or realtime. Its interface does not expose transport DTOs or product-platform presentation details.
- **Admin Workflow Module** — Admin-specific orchestration around one or more Capability Modules. It owns Admin policy and may adapt Admin parameters, but it does not duplicate capability implementation.
- **Transport Adapter** — Gin or Worker implementation that maps an external protocol to a workflow or capability interface and maps results back to that protocol.
- **Runtime Module** — process lifecycle, capability assembly, dependency validation, health, startup, and shutdown for `admin-api` and `admin-worker`.
- **Database Evolution Module** — schema fingerprinting, reconciliation, migration checksums, deployment locking, drift detection, data verification, and destructive-change gates.
- **Session Lifecycle Module** — issue, authenticate, rotate, revoke, and enforce session policy atomically across MySQL and Redis adapters.
- **Route Policy Registry** — the complete classification of every active route as Public, Authenticated, or Permission-protected, including audit policy.
- **Conversation Reply Command** — a durable, idempotent request to produce an AI reply after the user message and command are committed atomically.
- **Realtime Delivery Module** — event publication, subscription state, delivery semantics, sequence/resume behavior, and client backpressure policy.
- **Admin Contract Bundle** — the versioned OpenAPI document, permission catalog, runtime-view contract, realtime schemas, bundle manifest, and SHA-256 consumed by the Admin frontend.
- **Admin Browser Client** — the approved P08R-only Admin client. Once the new
  bundle is active it uses in-memory access credentials and a scoped HttpOnly
  refresh Cookie; it is not a separate Product Platform.
- **Tenant** — a future SaaS ownership and isolation concept. Tenant is not a synonym for product platform and is not implemented during this refactor.
- **Product Platform** — a product entry with its own transport and workflow
  differences. Admin is the only current Product Platform. A future platform is
  designed from its supplied project and formal contract, then reuses Capability
  Modules without speculative aliases or catch-all compatibility layers.

## Non-negotiable distinctions

```text
Tenant           = data ownership and isolation (future SaaS phase)
Product Platform = product entry and workflow policy
Capability       = reusable business behavior
```

Platform-specific request fields, response fields, and workflow rules stay outside Capability Modules. Capability Modules must not accumulate `if platform == ...` branches or accept catch-all JSON configuration.

A canonical Product Platform value may remain in persisted records as validated provenance or routing metadata. That value does not permit platform-policy branching inside a Capability Module. During this refactor, new active records use only `admin`; legacy `app`/`canvas` values are explicitly migrated, archived, or deleted by the reconciliation design.

## Current truth hierarchy

1. Approved design specs under `docs/superpowers/specs/`.
2. Executable tests and active route snapshots.
3. Current code.
4. Fingerprinted live schema plus versioned migration history.
5. README and historical migration comments.

When these disagree, stop and reconcile the higher-priority truth instead of adding compatibility guesses.

## Runtime contract gate

- `internal/runtime` is the only API/Worker process composition and lifecycle owner.
- Runtime constructors capture a deep `config.Snapshot`; caller-owned mutable slices never become live process configuration.
- `contracts/admin/v1` is generated from the compiled route registry and is the checked-in Admin Contract Bundle truth for HTTP, permission/view, and realtime consumers.
- `scripts/verify-runtime-contracts.ps1` is a blocking local/CI gate. It checks race-enabled runtime packages, contract drift, architecture rules, and both process builds.
- `internal/bootstrap/route_meta.go` is the single temporary P03 policy-input bridge. P04 moves policy beside transport registration and deletes that file; no additional route-policy map is allowed.
