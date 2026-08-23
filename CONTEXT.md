# Admin Platform Domain Context

This repository currently delivers one product platform: **Admin**. Its final
P09 product shape is one Browser-only client and one backend image shared by API
and Worker. The published Admin Contract Bundle contains no desktop, App,
Canvas, client-version, or Prompt-management product transport. The next product
direction prioritizes integrating a concrete additional platform supplied by
the user; SaaS/tenant behavior remains lower priority and outside the current
implementation scope. No speculative future platform framework is built before
that concrete project and contract exist.

## Admin-only immutable release

The release manifest binds the backend/frontend commits, immutable Docker image
IDs and archive hashes, current Bundle digest, and reviewed evidence. The
complete proof is written to
`release/admin-only/out/proof.json`; generated release output is ignored and is
never a source file.

Admin remains the only compile-time registered adapter. Generic platform-aware
authentication, RBAC, session, login-log, notification, provenance, and index
capabilities remain available for a separately approved platform. A database row or client header
never activates a platform. Activation requires an
approved contract, dedicated trusted transport, compile-time registry entry,
isolation tests, and a new immutable Docker release.

## Domain language

- **Admin Platform** — the only deployable product platform after this refactor. It owns the `/api/admin/v1` transport contract and the `admin_front_ts` client.
- **Capability Module** — reusable business behavior such as identity, AI, wallet, payment, storage, notification, export, or realtime. Its interface does not expose transport DTOs or product-platform presentation details.
- **Admin Workflow Module** — Admin-specific orchestration around one or more Capability Modules. It owns Admin policy and may adapt Admin parameters, but it does not duplicate capability implementation.
- **Transport Adapter** — Gin or Worker implementation that maps an external protocol to a workflow or capability interface and maps results back to that protocol.
- **Runtime Module** — process lifecycle, capability assembly, dependency validation, health, startup, and shutdown for `admin-api` and `admin-worker`.
- **Database ownership** — local Docker MySQL is the business source of truth; `internal/infra/database` is only the Go connection layer. See `docs/database-ownership.md`.
- **Session Lifecycle Module** — issue, authenticate, rotate, revoke, and enforce session policy atomically across MySQL and Redis adapters.
- **Mail Diagnostic Evidence** — an Admin-owned, one-to-one encrypted verification-code snapshot attached to a mail log. It is diagnostic evidence, not an authentication authority or a second source of verification-code state.
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

A canonical Product Platform value may remain in persisted records as validated provenance or routing metadata. That value does not permit platform-policy branching inside a Capability Module. New active records use only `admin`; retired product values are not compatibility inputs.

## Current truth hierarchy

1. Approved design specs under `docs/superpowers/specs/`.
2. Executable tests and active route snapshots.
3. Current code.
4. Current database facts read from the confirmed local MySQL instance.
5. README and historical migration comments.

When these disagree, fix the lower-priority source instead of adding compatibility guesses.

## Mail diagnostic boundary

`internal/module/mail` owns mail-log verification diagnostic evidence. Auth remains
the verification authority and owns code issuance, validation, and the Redis
deadline. The diagnostic child stores only encrypted evidence, its key ID, and the
same absolute expiry; it is not copied into `mail_logs` and has no lifecycle flags.
Plaintext is limited to authorized mail-log responses, required audit records are
payload-free, and all browser transport uses TLS. `APP_SECRET` is the sole current
root while `APP_SECRET_PREVIOUS` is the optional single previous root used only
during the documented rotation and rekey procedure.

## Runtime contract gate

- `internal/runtime` is the only API/Worker process composition and lifecycle owner.
- Runtime constructors capture a deep `config.Snapshot`; caller-owned mutable slices never become live process configuration.
- `contracts/admin/v1` is generated from the compiled route registry and is the checked-in Admin Contract Bundle truth for HTTP, permission/view, and realtime consumers.
- `scripts/verify-runtime-contracts.ps1` is a blocking local/CI gate. It checks race-enabled runtime packages, contract drift, architecture rules, and both process builds.
- `internal/bootstrap/route_meta.go` is the single temporary P03 policy-input bridge. P04 moves policy beside transport registration and deletes that file; no additional route-policy map is allowed.
