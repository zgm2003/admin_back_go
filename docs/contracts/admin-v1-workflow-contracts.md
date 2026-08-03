# Admin v1 workflow HTTP contracts

Status: active formal contract

Machine-readable source: `contracts/admin/v1/openapi.json`

Scope: the manually curated field-complete P07 workflow slice

Additional P07 Admin operation families are bound directly to compiled runtime
Go models and documented by `admin-v1-runtime-model-contracts.md`. Both sources
are emitted into the same machine-readable `contracts/admin/v1/openapi.json`.

This document records the semantics behind the generated OpenAPI components.
JSON names are exact. A field without `?` is always present. `?` means the
request field may be omitted. `| null` means the response field is present and
may be JSON `null`. No other alias, fallback, or legacy field is part of this
contract.

## Shared transport rules

- Every operation uses `Authorization: Bearer <access-token>`.
- A normal success body is `{ code: 0, data: T, msg: string }`.
- A mutation with no result uses `data: {}`; it is not `null` and is not omitted.
- A failed request uses `ErrorEnvelope`: `{ code, data, msg, error }`. The
  `error` object contains `code`, `category`, `retryable`, and optional
  `request_id`/`trace_id`. Failure is never converted to an empty success.
- Unless explicitly stated otherwise, success is HTTP `200`. Sending an AI
  message is HTTP `202`.
- Invalid query/body/path input is HTTP `400`; missing/expired authentication is
  HTTP `401`; permission or ownership failure is HTTP `403`; an addressed AI
  resource that does not exist is HTTP `404`; dependency/internal failures are
  HTTP `5xx`. All non-success statuses use `ErrorEnvelope`.
- A path parameter named `id` in this document is a positive integer (`>= 1`).
- `Page = { page_size: integer, current_page: integer, total_page: integer,
  total: integer }`. An empty paged result has `list: []`, `total: 0`, and
  `total_page: 0`; it is distinct from an error.
- `IntOption = { label: string, value: integer }` and
  `StringOption = { label: string, value: string }`.
- Display times are strings formatted as `YYYY-MM-DD HH:mm:ss`. A source time
  that is absent is represented exactly as documented for that field (`""` or
  `null`), never guessed by the client.

## User management

### Operations

| Method and path | Query/path/body | Success `data` |
| --- | --- | --- |
| `GET /api/admin/v1/users/page-init` | no parameters | `UserPageInit` |
| `GET /api/admin/v1/users` | required `current_page >= 1`, `page_size 1..50`; optional `keyword <= 100`, `username <= 64`, `email <= 255`, `detail_address <= 255`, comma-separated positive `address_id`, positive `role_id`, `sex in {0,1,2}`, `date`, `date_start`, `date_end` | `UserListResult` |
| `PUT /api/admin/v1/users/{id}` | required JSON body `UserUpdateRequest` | `{}` |
| `DELETE /api/admin/v1/users/{id}` | path `id`; no body | `{}` |
| `PATCH /api/admin/v1/users/{id}/status` | path `id`; required `{ status: 1 | 2 }` | `{}` |
| `PATCH /api/admin/v1/users` | required JSON body `UserBatchProfileRequest` | `{}` |
| `DELETE /api/admin/v1/users` | required `{ ids: positive integer[] }`, at least one item | `{}` |
| `POST /api/admin/v1/users/export` | required `{ ids: positive integer[] }`, at least one item | `{ id: positive integer, message: string }` |

`date_start`/`date_end` take precedence when either is present. Otherwise
`date` is a comma-separated start/end pair. These are explicit alternatives;
the frontend must not invent a third date alias.

### Schemas

```text
UserPageInit = {
  dict: {
    roleArr: IntOption[],
    auth_address_tree: AddressTreeNode[],
    sexArr: IntOption[] where value is 0 | 1 | 2,
    platformArr: StringOption[] where value is "admin" | "app"
  }
}

AddressTreeNode = {
  id: positive integer,
  parent_id: integer >= 0,
  label: string,
  value: positive integer,
  children?: AddressTreeNode[]
}

UserListResult = { list: UserListItem[], page: Page }
UserListItem = {
  id: positive integer, username: string, email: string, phone: string,
  avatar: string | null, sex: 0 | 1 | 2, sex_show: string,
  role_id: positive integer, role_name: string, bio: string,
  address_show: string, address_id: integer >= 0, detail_address: string,
  status: 1 | 2, created_at: string
}

UserUpdateRequest = {
  username: string (required, <= 64),
  role_id: positive integer (required),
  address_id: integer >= 0 (required),
  avatar?: string <= 255,
  sex?: 0 | 1 | 2,
  detail_address?: string <= 255,
  bio?: string <= 1000
}

UserBatchProfileRequest is exactly one of:
  { ids: positive integer[], field: "sex", sex: 0 | 1 | 2 }
  { ids: positive integer[], field: "address_id", address_id: positive integer }
  { ids: positive integer[], field: "detail_address", detail_address?: string <= 255 }
```

The user list always returns `list` and `page`. `avatar` is the only nullable
list field in this slice.

## Notifications

### Operations

| Method and path | Query/path/body | Success `data` |
| --- | --- | --- |
| `GET /api/admin/v1/notifications/page-init` | none | `NotificationPageInit` |
| `GET /api/admin/v1/notifications` | required `current_page >= 1`, `page_size 1..50`; optional positive `before_id`, `keyword <= 100`, `type in {1,2,3,4}`, `level in {1,2}`, `is_read in {1,2}` | `NotificationListResult` |
| `GET /api/admin/v1/notifications/unread-count` | none | `{ count: integer >= 0 }` |
| `PATCH /api/admin/v1/notifications/{id}/read` | path `id`; no body | `{}` |
| `PATCH /api/admin/v1/notifications/read` | optional `{ ids?: positive integer[] }`; missing/empty `ids` means all notifications owned by the current user | `{}` |
| `DELETE /api/admin/v1/notifications/{id}` | path `id`; no body | `{}` |
| `DELETE /api/admin/v1/notifications` | required non-empty `{ ids: positive integer[] }` | `{}` |

### Schemas

```text
NotificationPageInit = {
  dict: {
    notification_type_arr: IntOption[] where value is 1 | 2 | 3 | 4,
    notification_level_arr: IntOption[] where value is 1 | 2,
    notification_read_status_arr: IntOption[] where value is 1 | 2
  }
}

NotificationListResult = {
  list: NotificationItem[], page: Page, next_id: integer >= 0
}
NotificationItem = {
  id: positive integer, title: string, content: string,
  type: 1 | 2 | 3 | 4, type_text: string,
  level: 1 | 2, level_text: string, link: string,
  is_read: 1 | 2, created_at: string
}
```

An empty cursor page has `list: []`, `next_id: 0`, and the normal zero `Page`.

## Export tasks

### Operations

| Method and path | Query/path/body | Success `data` |
| --- | --- | --- |
| `GET /api/admin/v1/export-tasks` | optional `current_page >= 1` (default `1`), `page_size 1..50` (default `20`), positive `before_id`, `status in {1,2,3}`, `kind <= 64`, `title <= 100`, `file_name <= 255` | `ExportTaskListResult` |
| `GET /api/admin/v1/export-tasks/status-count` | optional `kind <= 64`, `title <= 100`, `file_name <= 255` | `ExportTaskStatusCountItem[]` |
| `DELETE /api/admin/v1/export-tasks/{id}` | path `id`; no body | `{}` |
| `DELETE /api/admin/v1/export-tasks` | required non-empty `{ ids: positive integer[] }` | `{}` |

### Schemas

```text
ExportTaskStatusCountItem = { label: string, value: 1 | 2 | 3, num: integer >= 0 }
ExportTaskListResult = { list: ExportTaskItem[], page: Page, next_id: integer >= 0 }
ExportTaskItem = {
  id: positive integer, kind: string, kind_text: string, title: string,
  file_name: string | null, file_url: string | null,
  file_size_text: string, row_count: integer >= 0 | null,
  status: 1 | 2 | 3, status_text: string,
  error_msg: string | null, expire_at: string | null,
  created_at: string
}
```

All four nullable fields remain present as `null`. An empty status count is an
array (the service currently returns the three known statuses with zero counts),
not a keyed object.

## AI conversations and messages

### Operations

| Method and path | Query/path/body | Success `data` |
| --- | --- | --- |
| `GET /api/admin/v1/ai-conversations` | optional positive `agent_id`, `before_time`, positive `before_id`, `limit 1..100` (default `20`); `before_time` and `before_id` must be supplied together | `AIConversationListResult` |
| `POST /api/admin/v1/ai-conversations` | required `{ agent_id: positive integer, title?: string <= 100 }` | `{ id: positive integer }` |
| `GET /api/admin/v1/ai-conversations/{id}` | path `id` | `AIConversationDetail` |
| `PUT /api/admin/v1/ai-conversations/{id}` | path `id`; required `{ title: non-empty string <= 100 }` | `{}` |
| `DELETE /api/admin/v1/ai-conversations/{id}` | path `id`; no body | `{}` |
| `GET /api/admin/v1/ai-conversations/{id}/messages` | path `id`; optional positive `before_id`, `limit 1..100` (default `20`) | `AIMessageListResult` |
| `POST /api/admin/v1/ai-conversations/{id}/messages` | path `id`; required `AIMessageSendRequest`; HTTP `202` | `AIMessageSendResult` |
| `POST /api/admin/v1/ai-conversations/{id}/messages/cancel` | path `id`; required `{ request_id: non-empty string <= 128 }` | `{ conversation_id: positive integer, request_id: string, status: "stopping" }` |
| `POST /api/admin/v1/ai-conversations/{id}/messages/{message_id}/revisions` | positive path `id` and `message_id`; required `AIMessageRevisionRequest`; HTTP `202` | `AIMessageSendResult` |
| `POST /api/admin/v1/ai-conversations/{id}/messages/{message_id}/regenerations` | positive path `id` and `message_id`; required `AIMessageRegenerationRequest`; HTTP `202` | `AIMessageSendResult` |
| `DELETE /api/admin/v1/ai-conversations/{id}/messages` | positive path `id`; required `AIMessageDeleteRequest` | `AIMessageDeleteResult` |
| `PUT /api/admin/v1/ai-conversations/{id}/read-cursor` | positive path `id`; required `{ message_id: positive integer }` | `AIConversationReadCursorResult` |

`before_time` uses `YYYY-MM-DD HH:mm:ss`. Supplying only one conversation cursor
field is a validation error; the client must preserve both values returned by
the previous response.

### Schemas

```text
AIConversationItem = {
  id: positive integer, agent_id: positive integer, agent_name: string,
  title: string, unread_count: integer >= 0,
  last_message_at: string, updated_at: string
}
AIConversationListResult = {
  list: AIConversationItem[], next_time: string,
  next_id: integer >= 0, has_more: boolean
}
AIConversationDetail = AIConversationItem + { created_at: string }

AIAttachmentRequest = {
  type: "image", url: non-empty string, name?: string, size?: integer >= 0
}
AIRuntimeParams = {
  temperature?: number 0..2
}
AIMessageSendRequest = {
  request_id: non-empty string <= 128,
  content?: string <= 20000,
  attachments?: AIAttachmentRequest[] (maximum 5),
  runtime_params?: AIRuntimeParams
}
Exactly one usable input is required: trimmed `content` must be non-empty or
`attachments` must contain at least one item.
Legacy integer `runtime_params.max_history` input remains parseable but is
ignored and is not published in OpenAPI or response metadata.

AIMessageItem = {
  id: positive integer, role: 1 | 2 | 3, content_type: string,
  content: string, meta_json?: AIMessageMeta,
  paired_message_id: positive integer | null,
  run_id: positive integer | null, liked: boolean,
  created_at: string, updated_at: string
}
AIMessageMeta = {
  attachments?: { type: "image", url: string, name: string, size: integer >= 0 }[],
  runtime_params?: AIRuntimeParams
}
AIMessageListResult = {
  list: AIMessageItem[], next_id: integer >= 0, has_more: boolean
}
AIMessageSendResult = {
  conversation_id: positive integer, user_message_id: positive integer,
  command_id: positive integer, request_id: string,
  state: "pending" | "claimed" | "running" | "succeeded" | "failed" |
         "canceled" | "outcome_unknown" | "timed_out"
}
AIMessageRevisionRequest = {
  content: non-empty string <= 20000,
  request_id: non-empty string <= 128
}
AIMessageRegenerationRequest = { request_id: non-empty string <= 128 }
AIMessageDeleteRequest = { ids: unique non-empty positive integer[] }
AIMessageDeleteResult = { deleted_ids: unique positive integer[] in ascending order }
AIConversationReadCursorResult = {
  conversation_id: positive integer,
  last_read_message_id: positive integer,
  unread_count: integer >= 0
}
```

Revision and regeneration use the canonical authenticated `(user_id,
request_id)` replay key. Conversation ID, source message ID, operation and the
normalized inherited input are request-fingerprint facts, not alternate
idempotency scopes. Replaying the same key and fingerprint returns the original
`AIMessageSendResult`; reusing the key with a different fingerprint is a
conflict. `paired_message_id` and `run_id` are always present and explicitly
`null` when no persisted relationship exists; clients must not infer either
relationship from message order.

An empty conversation/message cursor result uses an empty list, zero/empty
cursor values, and `has_more: false`. It is not a missing/error response.

## AI run monitoring

### Operations

| Method and path | Query/path/body | Success `data` |
| --- | --- | --- |
| `GET /api/admin/v1/ai-runs/page-init` | none | `AIRunPageInit` |
| `GET /api/admin/v1/ai-runs` | optional `current_page >= 1` (default `1`), `page_size 1..50` (default `20`), `platform in {admin,app,canvas}`, `status in {running,success,failed,canceled,timeout}`, positive `user_id`, `request_id <= 128`, positive `agent_id`, positive `provider_id`, `date_start <= 20`, `date_end <= 20` | `AIRunListResult` |
| `GET /api/admin/v1/ai-runs/{id}` | path `id` | `AIRunDetail` |
| `PUT /api/admin/v1/ai-runs/{id}/user-feedback` | positive path `id`; required `{ liked: boolean }`; authenticated self-service, no `ai_run_list` permission | `{ id: positive integer, liked: boolean, liked_at: string \| null }` |
| `GET /api/admin/v1/ai-runs/stats` | optional `date_start <= 20`, `date_end <= 20`, `platform`, positive `agent_id`, positive `provider_id`, positive `user_id` | `AIRunStatsResult` |
| `GET /api/admin/v1/ai-runs/stats/by-date` | the stats filters plus optional `current_page >= 1` (default `1`) and `page_size 1..50` (default `20`) | `{ list: AIRunStatsByDateItem[], page: Page }` |
| `GET /api/admin/v1/ai-runs/stats/by-agent` | same as by-date | `{ list: AIRunStatsByAgentItem[], page: Page }` |
| `GET /api/admin/v1/ai-runs/stats/by-user` | same as by-date | `{ list: AIRunStatsByUserItem[], page: Page }` |

Date boundaries are optional independent inclusive filters over run creation
time. Their current transport constraint is a string of at most 20 characters;
the frontend must pass the backend-documented display-time form and must not
invent aliases.

### Top-level schemas

```text
AIRunPageInit = {
  dict: {
    status_arr: StringOption[] where value is a run status,
    platform_arr: StringOption[] where value is "admin" | "app" | "canvas",
    providerArr: IntOption[], agentArr: IntOption[]
  }
}

AIRunListResult = { list: AIRunListItem[], page: Page }
AIRunListItem = {
  id: positive integer, request_id: string, user_id: positive integer,
  agent_id: positive integer, agent_name: string,
  provider_id: positive integer, provider_name: string,
  platform: "admin" | "app" | "canvas", input_snapshot: string,
  conversation_id: positive integer | null, conversation_title: string,
  status: RunStatus, status_name: string,
  model_id: string, model_display_name: string,
  prompt_tokens: integer >= 0, completion_tokens: integer >= 0,
  total_tokens: integer >= 0, duration_ms: integer >= 0 | null,
  duration_text: string, error_message: string, created_at: string
}

RunStatus = "running" | "success" | "failed" | "canceled" | "timeout" |
  "outcome_unknown"
AIRunStatsResult = {
  date_range: { start: string | null, end: string | null },
  summary: {
    total_runs: integer >= 0, success_rate: number 0..100,
    fail_runs: integer >= 0, total_tokens: integer >= 0,
    total_prompt_tokens: integer >= 0,
    total_completion_tokens: integer >= 0,
    avg_duration_ms: integer >= 0
  }
}

AIRunStatsMetric = {
  total_runs: integer >= 0, total_tokens: integer >= 0,
  total_prompt_tokens: integer >= 0,
  total_completion_tokens: integer >= 0,
  avg_duration_ms: integer >= 0
}
AIRunStatsByDateItem = { date: string } + AIRunStatsMetric
AIRunStatsByAgentItem = { agent_id: positive integer, agent_name: string } + AIRunStatsMetric
AIRunStatsByUserItem = { username: string } + AIRunStatsMetric
```

### Detail schema

```text
AIRunDetail contains every AIRunListItem field except it additionally has:
  username: string,
  billing_status: "pending" | "held" | "settled" | "released" | "unbilled",
  billing_reason: "pending" | "held" | "settled_complete_usage" |
    "released_before_dispatch" | "released_insufficient_balance" |
    "released_provider_failed" | "released_outcome_unknown" |
    "unbilled_usage_incomplete" | "unbilled_over_hold" | "legacy_unpriced",
  held_amount: RMBAmount, actual_amount: RMBAmount,
  pricing: AIRunPricing | null,
  usage_items: AIRunUsageItem[], provider_attempts: AIRunProviderAttempt[],
  user_message: AIRunMessageSummary | null,
  assistant_message: AIRunMessageSummary | null,
  liked: boolean, liked_at: string | null,
  events: AIRunEvent[],
  context_plan: AIContextPlan | null,
  tool_calls: AIRunToolCall[],
  started_at: string, finished_at: string, updated_at: string

RMBAmount is the canonical non-negative decimal string emitted from integer
RMB units (for example `0`, `2.5`, or `0.00000001`; no exponent, sign,
leading zero, or trailing fractional zero).
AIRunPricing = {
  version: non-empty string, catalog_vendor: non-empty string,
  transport_engine: non-empty string, model_id: non-empty string,
  resolved_alias: string, billing_multiplier: positive decimal string,
  max_output_tokens: positive integer, rates: AIRunPricingRate[] (non-empty)
}
AIRunPricingRate = {
  category: "input" | "output" | "cache_read" | "cache_write" | "media",
  tier_key: string, unit: non-empty string, price: RMBAmount,
  unit_scale: positive integer
}
AIRunUsageItem = {
  attempt_no: positive integer,
  category: "input" | "output" | "cache_read" | "cache_write" | "media",
  tier_key: string, quantity: integer >= 0, unit: non-empty string,
  unit_price: RMBAmount, unit_scale: positive integer,
  amount: RMBAmount, billable: boolean
}
AIRunProviderAttempt = {
  attempt_no: positive integer,
  state: "prepared" | "dispatched" | "succeeded" | "failed" | "canceled" |
    "outcome_unknown",
  provider_request_id: non-empty string | null,
  usage_status: "complete" | "unavailable"
}

AIRunMessageSummary = {
  id: positive integer, role: 1 | 2 | 3, content_type: string,
  content: string, meta_json: JSONValue, created_at: string
}
AIRunEvent = {
  id: positive integer, seq: integer >= 0,
  event_type: "start" | "completed" | "failed" | "canceled" | "timeout" |
    "retry_scheduled" | "usage_recorded" | "outcome_unknown" | "settled" |
    "released" | "unbilled",
  event_type_name: string, message: string,
  elapsed_ms: integer >= 0 | null, elapsed_text: string, created_at: string
}
AIRunToolCall = {
  id: positive integer, tool_id: positive integer,
  tool_code: string, tool_name: string, call_id: string | null,
  status: "running" | "success" | "failed" | "timeout",
  arguments_json: JSONValue, result_json: JSONValue,
  error_message: string, duration_ms: integer >= 0 | null,
  started_at: string, finished_at: string
}
```

`JSONValue` is deliberately an explicitly open JSON value because the persisted
message/tool payload is defined as `json.RawMessage`. Invalid or absent stored
JSON is normalized by the backend to `{}`. This is not permission for a client
to read a different business field as a fallback.

All detail collections are always arrays and use `[]` when no records exist.
The two message fields, `pricing`, `liked_at`, and the documented
duration/conversation/call fields are the only nullable detail values; empty
time strings remain strings.
Legacy unpriced runs publish `billing_reason="legacy_unpriced"`, zero amounts,
`pricing=null`, and empty billing evidence arrays. Paid pricing comes only from
the immutable Run snapshot. Failed-attempt usage is audit-only with
`billable=false`; settled billable item amounts are validated against
`actual_amount` before the response is returned.

## AI realtime failure payload

`ai.response.failed.v1` is a closed payload with all six fields required:

```text
{
  conversation_id: positive integer,
  request_id: non-empty string <= 128,
  msg: non-empty string <= 1024,
  error_code: non-empty string <= 128,
  wallet_path: string | null,
  recharge_path: string | null
}
```

For `error_code="ai.billing.insufficient_balance"`, the paths are exactly
`/profile/wallet` and `/payment/recharge`. For every other error code, both
fields are explicitly `null`.
