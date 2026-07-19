# Admin v1 runtime-model HTTP contracts

Status: active formal contract

Machine-readable source: `contracts/admin/v1/openapi.json`

The active bundle is the Browser-only contract identified by
`admin-browser-auth-contract.md`. Its source revision and manifest SHA-256 are
recorded there; historical browser/desktop variants are not part of the active
field contract.

This document defines how field-complete Admin operations outside the original
workflow slice are published. Runtime route definitions bind their exact query,
request, and response Go models through `adminroute.HTTPContract`. The Admin
Contract Bundle is generated from those compiled runtime definitions; frontend
types and legacy frontend wrappers are never contract inputs.

## Covered operation families

The runtime-model catalog covers these Admin v1 families:

- authentication helpers, profile/security, sessions, and login logs;
- authentication platforms, permissions, and roles;
- cron tasks, system logs/settings, and operation logs;
- mail, SMS, notification tasks, upload drivers/rules/settings/tokens;
- payment configuration/certificates/recharges and wallet/ledger queries;
- AI agents, knowledge bases/documents, prompts, providers, and tools.

The manually curated user, notification, export, AI conversation/message, and
AI run contracts remain defined by `admin-v1-workflow-contracts.md`. Browser
grant responses remain the exact schemas defined by the Admin Contract Bundle.

## Exact model rules

- Query names come only from runtime `form` tags. `binding:"required"` makes a
  query parameter required; binding length/range/enum rules are emitted into
  OpenAPI.
- JSON request names come only from runtime `json` tags. Required fields come
  from `binding:"required"`; omitted optional fields are not synthesized.
- Response names come only from response DTO `json` tags. A pointer without
  `omitempty` is a required nullable field. `omitempty` is an optional field.
- Response structs are closed (`additionalProperties: false`). Maps retain
  their documented value schema. `json.RawMessage`/`any` is open only where the
  backend DTO explicitly declares arbitrary JSON.
- Slice responses are arrays. Mutation success with no domain result is exactly
  `{}`. Create results that return an ID are exactly `{ "id": integer }`.
- A runtime path parameter named `id` is a positive integer. Other path
  parameters retain their declared string semantics.
- Every normal success remains `{ code: 0, data: T, msg: string }`; failures use
  the shared classified `ErrorEnvelope` and are never converted to empty data.

## Explicit exceptional shapes

- The bundle has no client-version route, view, or permission. The
  `client_versions` table remains frozen until P09's separately approved
  destructive gate.
- `POST /api/admin/v1/payment/certificates` consumes
  `multipart/form-data` with `config_code`, `cert_type`, and binary `file`.
- The Browser-only credential contract has one closed login/refresh response
  containing only `access_token` and `expires_in`; its refresh credential exists
  only in the scoped HttpOnly Cookie.
- AI tool JSON-schema fields are explicitly arbitrary JSON because the backend
  request/response DTO uses `json.RawMessage`; clients may not substitute other
  business fields for them.

Any route in a covered family that falls back to `SuccessEnvelope` or a generic
request body is a contract regression and is rejected by backend contract tests.
