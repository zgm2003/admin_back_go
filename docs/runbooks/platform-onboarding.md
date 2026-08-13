# Future platform onboarding

Status: mandatory order for a separately contracted product platform

Admin is the only active platform. A database row or client header never activates a platform.

Use this order without skipping a step:

1. **Approve the platform contract** with explicit product workflows, identity,
   isolation, lifecycle, and ownership decisions.
2. **Implement a dedicated trusted transport** that derives provenance from its
   registered server-side adapter rather than a caller-selected header.
3. **Add the compile-time registry entry** so the runtime can recognize exactly
   the reviewed adapter.
4. **Publish a matching Admin Contract Bundle** revision that formally exposes
   the approved operations/schemas to its consuming client contract.
5. **Configure auth_platforms** only after the runtime transport exists.
6. **Assign independent permissions and roles**; do not inherit Admin grants by
   name or infer replacements for missing legacy permissions.
7. **Configure notification audiences** and platform-aware delivery policy.
8. **Run cross-platform isolation tests** for authentication, RBAC, sessions,
   login logs, notifications, provenance, realtime, queue work, and storage.
9. **Deploy immutable Docker images** bound to clean commits, a generated
   contract digest, baseline schema/seed hashes, ordered migration checksums,
   evidence hashes, and rollback proof.

Capability modules remain transport-neutral. Do not add platform conditionals
to reusable AI, wallet, payment, storage, export, notification, or realtime
logic. Platform-specific policy belongs in the trusted transport/workflow and
its explicit contract.
