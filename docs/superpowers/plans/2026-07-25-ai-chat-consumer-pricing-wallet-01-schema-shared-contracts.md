# AI 计费核心 Schema 与共享契约 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立 Run 级 Charge/Hold、分类 usage、money units 和智能体计费配置所需的数据库事实与跨模块类型契约。

**Architecture:** 本计划只锁定持久化事实和接口名字，不实现钱包业务、报价或 provider 调用。迁移采用 expand/backfill/validate/contract 四步，应用启动不自动执行；所有唯一身份字段使用非空规范值，避免 MySQL nullable unique 语义。

**Tech Stack:** MySQL、Atlas HCL/SQL、GORM、Go interfaces、JSON snapshots。

---

### Task 1: 固化状态和列名清单

**Files:**
- Read: `docs/superpowers/specs/2026-07-24-ai-chat-consumer-pricing-wallet-design.md`

- [x] **Step 1: Write the contract checklist**

Record these non-null states before touching HCL: `ai_runs.status` = `running|success|failed|canceled|timeout|outcome_unknown`; `ai_runs.billing_status` = `pending|held|settled|released|unbilled`; `ai_runs.billing_reason` = `pending|held|settled_complete_usage|released_before_dispatch|released_insufficient_balance|released_provider_failed|released_outcome_unknown|unbilled_usage_incomplete|unbilled_over_hold|legacy_unpriced`; `wallet_holds.status` = `active|captured|released`; `ai_provider_attempts.state` = `prepared|dispatched|succeeded|failed|canceled|outcome_unknown`; `ai_usage_charges.status` = `open|settled|released|unbilled`. The canonical Spec has no `uncertain` billing state: incomplete usage and unknown outcomes release the Hold.

- [x] **Step 2: Run the fast schema search**

Run `rg -n 'table "(ai_agents|ai_runs|ai_provider_attempts|user_wallets|wallet_transactions)"|SourceAIRefund' database internal`.

Expected: the existing cents fields, command-owned attempts, and refund constant are listed; no new names are present yet.

### Task 2: Add Atlas schema for the billing facts

**Files:**
- Modify: `database/schema/admin.hcl`
- Test: `database/schema/admin.hcl` reviewed by `atlas schema inspect` in the maintenance environment

- [x] **Step 1: Add agent snapshot inputs**

Add non-negative `ai_agents.billing_multiplier_ppm BIGINT UNSIGNED NOT NULL DEFAULT 1000000` and `ai_agents.max_output_tokens INT UNSIGNED NOT NULL DEFAULT 4096`; reject zero multiplier and zero max output in the service layer.

- [x] **Step 2: Add Run and attempt ownership**

The final HCL contract adds `ai_runs.request_fingerprint BINARY(32) NOT NULL`, `pricing_snapshot_json MEDIUMTEXT NOT NULL`, `billing_status VARCHAR(16) NOT NULL`, and `billing_reason VARCHAR(32) NOT NULL`; add the same non-null fingerprint to `ai_reply_commands`. `pricing_snapshot_json` is the immutable Run-acceptance configuration snapshot, not a mutable per-attempt quote container. Set every paid `request_id` to binary collation and replace chat-only uniqueness with the canonical `(user_id, request_id)` unique identity on Runs, commands and paid task tables. Extend the `ai_runs.status` check with terminal `outcome_unknown`. Add `ai_provider_attempts.run_id BIGINT UNSIGNED NOT NULL`, `prepared_request_json MEDIUMTEXT NOT NULL`, `prepared_request_sha256 BINARY(32) NOT NULL`, `quote_json MEDIUMTEXT NOT NULL`, `usage_json MEDIUMTEXT NOT NULL`, `usage_status VARCHAR(16) NOT NULL DEFAULT 'unavailable'`, and `result_candidate_json MEDIUMTEXT NULL`. Preserve `command_id` only as nullable legacy chat correlation; unique attempt identity is `(run_id, attempt_no)`.

- [x] **Step 3: Add Hold, Charge and immutable item tables**

Create `wallet_holds` with `wallet_id`, `user_id`, `run_id`, `held_units`, `captured_units`, `status`, timestamps and unique `(run_id)`. Create `ai_usage_charges` with `run_id`, `user_id`, `currency='CNY'`, `pricing_version`, `multiplier_ppm`, `held_units`, `actual_units`, `status`, `finalized_at` and unique `(run_id)`. Create `ai_usage_charge_items` with `charge_id`, `attempt_id`, non-null `category`, non-null `tier_key DEFAULT ''`, `quantity`, `unit`, `unit_price_units`, `unit_scale`, `amount_units`, and a unique `(charge_id, attempt_id, category, tier_key, unit)`.

- [x] **Step 4: Add wallet unit columns and indexes**

Add `balance_units`, `total_recharge_units`, `total_consume_units`, `held_units` to `user_wallets`; add `amount_units`, `balance_before_units`, `balance_after_units` to `wallet_transactions`. Keep cents columns during expand/backfill; do not add dual-write behavior.

- [x] **Step 5: Add durable identities to every paid task**

Add `request_id` and `request_fingerprint` to `ai_text_tasks`, `ai_image_tasks` and `ai_video_tasks`, and add `run_id` only where it does not already exist, with a binary unique `(user_id, request_id)`. The existing `ai_video_tasks.run_id` must be validated/backfilled and lose its sentinel-zero semantics rather than being added a second time. Add `kind` (`text|tool_draft`) and `last_error_code` to `ai_text_tasks`; add `last_error_code` to every other paid task. Create `ai_audio_tasks` with request identity, Run/provider/model snapshots, normalized request JSON, status, immutable `storage_provider`/`storage_key`, content type, error code/message and timestamps. Existing image files keep their immutable storage tuple; add the same output tuple to video tasks. Every `run_id` gets a real foreign key/index after legacy mapping; sentinel zero is not a valid final identity.

- [x] **Step 6: Extend Run Event types**

Extend the event check/index to allow `retry_scheduled`, `usage_recorded`, `outcome_unknown`, `settled`, `released`, `unbilled` while preserving `(run_id, seq)` uniqueness. Keep state and event writes in the same transaction API planned for Plan 05.

### Task 3: Write expand/backfill/validate/contract migrations

**Files:**
- Create: `database/migrations/202607250101_ai_billing_expand.sql`
- Create: `database/migrations/202607250102_ai_billing_backfill.sql`
- Create: `database/migrations/202607250103_ai_billing_contract.sql`
- Create: `database/migrations/202607250104_ai_billing_permissions.sql`
- Modify: `database/migrations/atlas.sum`
- Modify: `database/seeds/admin_permissions.sql`

- [x] **Step 1: Write the expand migration**

Create new tables and initially nullable columns only. Before any write, abort if there are active legacy paid workloads (`ai_reply_commands` in `pending|claimed|running`, or text/image/video tasks in a non-terminal state), negative cents, duplicate wallet users, duplicate `(source_type, source_id)`, duplicate canonical `(user_id, request_id)` identities, or orphan AI foreign keys. The migration must not create a backup database. Do not add `NOT NULL request_fingerprint`, `NOT NULL run_id`, unit-column constraints or new unique indexes to populated tables until backfill validation succeeds.

- [x] **Step 2: Write the backfill validation**

Convert every wallet/transaction cents value with checked arithmetic `units = cents * 1000000`; reject `cents < 0` and `cents > 9223372036854`. Within the maintenance window, lock every wallet writer, backfill both wallet and transaction unit columns, and verify each transaction satisfies `before + in - out = after` plus per-wallet totals. Historical terminal Runs are explicitly non-billable: set `billing_status='unbilled'`, `billing_reason='legacy_unpriced'` and `pricing_snapshot_json` to a validated marker such as `{"version":"legacy_unpriced_v1","billable":false}`; never manufacture historical charges. Set legacy attempt `usage_status='unavailable'`, a canonical unavailable `usage_json`, and explicit validated legacy markers for `prepared_request_json`, its hash and `quote_json`; historical attempts remain non-replayable/non-billable because their exact outbound request and categorized quote cannot be reconstructed. Persist one `legacy_cutover_v1` timestamp and marker version, write each historical Run a stable `legacy_non_replayable_v1:<table>:<id>` identity whose fingerprint is only a hash of that marker, and copy that identity to commands/tasks. Never treat that marker as a canonical replay tuple. Map each paid task and each legacy attempt to exactly one Run through stored user/conversation/task/request identities, and abort on every zero or multiple match.

- [x] **Step 3: Write the contract migration**

After the user verifies the new binary reads only units and all rows pass validation, make the new identity/unit columns non-null, add the canonical binary unique indexes and foreign keys, and drop cents columns in one separately invoked maintenance step. Before dropping, verify `units = cents * 1000000` row by row; non-zero legacy cents are valid and must not be treated as an error. Contract must validate the persisted cutover metadata, require every legacy marker to have `created_at < legacy_cutover_at` and the expected marker hash, and fail if any marker appears on a newly created Run or if any task/attempt still has a null/zero Run owner.

- [x] **Step 4: Register the Run permission without role grants**

Add permission ID `920`, code `ai_run_list`, parent ID `50`, type `3`, active/not-deleted to the seed and `202607250104_ai_billing_permissions.sql`. Use the existing temporary guard-table pattern and reject ID/code collisions. The migration must not read or write `role_permissions`; the user grants this code manually after deployment.

- [x] **Step 5: Update Atlas checksum**

Run `pwsh -NoProfile -File scripts/database/atlas.ps1 migrate hash --dir file://database/migrations` and inspect the diff. If Atlas image startup exceeds two minutes, stop and leave this command for the user. Do not run the migration against Docker/MySQL in this plan.

### Task 4: Add shared Go persistence models and tests

**Files:**
- Create: `internal/module/ai/billing/model.go`
- Create: `internal/module/ai/billing/contracts.go`
- Create: `internal/module/ai/requestidentity/fingerprint.go`
- Create: `internal/module/ai/requestidentity/fingerprint_test.go`
- Create: `internal/shared/money/units.go`
- Create: `internal/shared/money/units_test.go`
- Modify: `docs/architecture.md`
- Modify: `internal/module/ai/run/model.go`
- Modify: `internal/module/ai/chat/dto.go`
- Modify: `internal/module/payment/wallet/model.go`
- Modify: `internal/shared/enum/ai.go`
- Modify: `internal/shared/enum/ai_runtime_test.go`
- Create: `internal/module/ai/billing/contracts_test.go`

- [x] **Step 1: Define shared types**

Use string enums and `int64` units only:

```go
type BillingStatus string
type BillingReason string
type HoldStatus string
type UsageStatus string
type UsageCategory string
type UsageItem struct { Category UsageCategory; Unit string; TierKey string; Quantity int64 }
```

Reject negative values and do not expose a `float64 Cost` field.

In `internal/shared/money`, define `UnitsPerRMB int64 = 100000000`, `UnitsPerCent int64 = 1000000`, checked `CentsToUnits(int64) (int64, error)` and `FormatRMBUnits(int64) (string, error)`. The formatter rejects negatives and emits the canonical API form: `"0"` or a plain decimal with at most 8 fractional digits and no trailing fractional zeros. This package contains only integer representation/conversion; it has no wallet, pricing, GORM or HTTP dependency. Add the same ownership rule to `docs/architecture.md`: payment owns balance mutation, pricing owns quote math, and both plus Run DTOs may depend only on this shared value primitive.

Extend the shared Run enum/label validator with terminal `outcome_unknown`; do not reuse reply-command `timed_out` for Run status (`ai_runs` continues to use `timeout`).

- [x] **Step 2: Define repository seams**

Expose small interfaces for Run/Charge acceptance, reserve-and-prepare attempt recording, dispatched/outcome evidence and finalizer participation. Run acceptance writes the immutable pricing configuration snapshot with the canonical request identity. Reserve-and-prepare accepts a transaction handle/callback so Hold target, Charge held amount and the new attempt's exact request/quote evidence commit atomically after the wallet locks. Status, event, item and wallet finalization must likewise share one transaction. Do not make handlers depend on GORM.

- [x] **Step 3: Define one request fingerprint algorithm**

Build fingerprints from typed normalized structs with `encoding/json` and SHA-256. The struct includes user scope, operation/modality, agent/model selection, normalized text, attachment identities and generation options; it excludes timestamps, leases and provider responses. All paid entry points use the canonical `(user_id, request_id)` lookup, compare all 32 bytes, replay equal fingerprints and return the shared conflict error for unequal fingerprints.

- [x] **Step 4: Test invariant helpers**

Test zero/negative usage rejection, tier normalization, Run/command status distinction, status transition rejection, request fingerprint stability under identical typed input, different fingerprints for changed content/options, checked cents overflow, and exact units formatting (`0`, one unit, one cent, one RMB and trailing-zero trimming). Run `go test ./internal/module/ai/billing ./internal/module/ai/requestidentity ./internal/module/ai/run ./internal/shared/enum ./internal/shared/money`.

### Task 5: Verify only this shared layer

- [x] **Step 1: Run fast checks**

Run `gofmt -w internal/module/ai/billing internal/module/ai/requestidentity internal/module/ai/run internal/module/payment/wallet internal/shared/money internal/shared/enum/ai.go internal/shared/enum/ai_runtime_test.go` and `git diff --check`.

- [x] **Step 2: Commit**

```powershell
git add database/schema/admin.hcl database/migrations database/seeds/admin_permissions.sql internal/module/ai/billing internal/module/ai/requestidentity internal/module/ai/run internal/module/payment/wallet internal/shared/money internal/shared/enum/ai.go internal/shared/enum/ai_runtime_test.go docs/architecture.md docs/superpowers/plans/2026-07-25-ai-chat-consumer-pricing-wallet-01-schema-shared-contracts.md
git commit -m "feat(ai): add billing core schema contracts"
```

### Wave 0 review closure

- [x] **A: Canonical wallet schema** — final Atlas HCL contains units only for wallet balance/ledger facts; payment order/recharge/package cents remain separate.
- [x] **B: Durable cutover boundary** — backfill persists `legacy_cutover_v1`; contract validates marker version, hash, and `created_at < legacy_cutover_at`.
- [x] **C: Legacy identity semantics** — historical rows receive stable non-replayable markers and are rejected by replay comparison; no synthetic canonical tuple is created.
- [x] **D: Provider outcome evidence** — outcome contracts carry provider request ID, typed response SHA-256, and dispatch state without credentials.
