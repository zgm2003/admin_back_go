# Admin Platform Domain Context

This repository currently delivers one product platform: **Admin**. `app` and `canvas` are retired product-platform lines during the 2026-07 super refactor. The next product phase is SaaS, but tenant behavior is deliberately outside the current implementation scope.

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
- **Admin Client Variant** — the browser or packaged-desktop adapter used to access the same Admin Product Platform. A client variant may change credential/native transport, but it does not create a Product Platform or duplicate Capability behavior.
- **Tenant** — a future SaaS ownership and isolation concept. Tenant is not a synonym for product platform and is not implemented during this refactor.
- **Product Platform** — a product entry with its own transport and workflow differences. Admin is the only current Product Platform. Future SaaS platforms reuse Capability Modules through their own workflow and transport adapters.

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
