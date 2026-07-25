# 官方定价与智能体配置 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 提供版本化官方数值价格目录、分类报价和智能体级 PPM 倍率/最大输出配置。

**Architecture:** 价格目录是编译进后端的只读、版本化数据，运行时不抓网页、不读 Sub2API 聚合余额。模型不保存供应商倍率；智能体保存一个 `billing_multiplier_ppm`，`1_000_000` 表示 `1.0`。Run 创建时复制价格和倍率快照。

**Tech Stack:** Go `math/big` 精确有理数、checked `int64` 落库、GORM、Admin HTTP DTO、官方目录 fixture。

---

### Task 1: Build the immutable pricing catalog

**Files:**
- Create: `internal/module/ai/pricing/catalog.go`
- Create: `internal/module/ai/pricing/quote.go`
- Create: `internal/module/ai/pricing/usage.go`
- Create: `internal/module/ai/pricing/catalog/official_numeric_parity_v1.json`
- Create: `internal/module/ai/pricing/catalog_test.go`
- Create: `internal/module/ai/pricing/quote_test.go`

- [ ] **Step 1: Define catalog types**

Use non-float fields:

```go
type Category string
const (InputTokens Category = "input"; OutputTokens Category = "output"; CacheRead Category = "cache_read"; CacheWrite Category = "cache_write"; MediaUnits Category = "media")
type Rate struct { Category Category; Unit string; TierKey string; PriceUnits int64; UnitScale int64 }
type ModelPrice struct { Version string; CatalogVendor string; ModelID string; Aliases []string; MaxOutputTokens int64; SourceURL string; RetrievedAt string; Rates []Rate }
type QuoteLine struct { Key string; Item billing.UsageItem }
type QuoteLineResult struct { Key string; Rate Rate; AmountUnits int64 }
type QuoteResult struct { AmountUnits int64; Lines []QuoteLineResult }
```

`CatalogVendor` is the official price owner (`openai`, `anthropic`, `google`, and so on), not the repository's OpenAI-compatible transport engine. Canonical model IDs must be globally unique and may not also appear as another row's alias. Resolution trims outer whitespace but remains case-sensitive: try the canonical ID first, otherwise accept an alias only when it identifies exactly one row. Ambiguous alias lookup returns a typed error rather than consulting `base_url`, provider name or `engine_type`. The catalog must also reject non-positive unit scales, negative prices and duplicate `(category,unit,tier_key)` rates at construction. `tier_key` is the empty string for untiered rates and an explicit value such as a cache TTL for tiered rates. `QuoteLine.Key` is caller-provided stable identity; actual settlement uses the canonical attempt/category/tier/unit tuple, while upper-bound quotes use a deterministic request-line key.

- [ ] **Step 2: Encode official numeric parity**

Store `$5/M` as `5 RMB/M` by writing `PriceUnits=500000000, UnitScale=1000000`. Store per-image/per-second prices with the same rational pair and `UnitScale=1` where the official unit is one. Embed the reviewed JSON with `go:embed`; every row records the official source URL and retrieval date. Do not copy Sub2API prices, add an FX rate, add provider multipliers or perform runtime HTTP lookup. An entry without an auditable official source is omitted, so the model fails closed with `ErrPriceUnavailable`.

- [ ] **Step 3: Implement one exact Run-level rounding**

`Quote(price ModelPrice, lines []QuoteLine, multiplierPPM int64) (QuoteResult, error)` matches every line by `(category,unit,tier_key)` and computes the exact rational value `sum(quantity * PriceUnits / UnitScale) * multiplierPPM / 1000000`. Use `math/big.Int`/`big.Rat` (or an equivalently proven integer common-denominator algorithm), never binary floating point and never `ceil` an individual item. Round upward exactly once after summing the complete Run, then reject a final value outside non-negative `int64`. Quote input is already a set of mutually exclusive normalized categories; pricing must never add cache read/write to aggregate input or silently merge duplicate stable line identities.

- [ ] **Step 4: Allocate the rounded total deterministically**

For persisted charge items, calculate each item's exact post-multiplier rational share, take each floor, and distribute `final_actual_units - sum(floors)` one unit at a time by largest remainder. Break equal remainders by stable `(attempt_id, category, tier_key, unit)` order. Assert every item is non-negative and item `amount_units` sum exactly equals the once-rounded Run total; do not hide a rounding delta in an extra synthetic item.

- [ ] **Step 5: Test price resolution and allocation**

Cover exact canonical IDs, one-to-one aliases, ambiguous aliases across catalog vendors, proof that `transport_engine=openai` does not force `catalog_vendor=openai`, missing cache tier, unsupported media unit, duplicate line identities, zero/negative multiplier, intermediate values larger than `int64` but with a valid final result, final overflow, the `$5/M -> ¥5/M` example, multiple fractional items that would overcharge under per-item `ceil`, and deterministic largest-remainder ties. Run `go test ./internal/module/ai/pricing`.

### Task 2: Add agent billing configuration

**Files:**
- Modify: `internal/module/ai/agent/model.go`
- Modify: `internal/module/ai/agent/dto.go`
- Modify: `internal/module/ai/agent/service.go`
- Modify: `internal/module/ai/agent/transport/admin/request.go`
- Modify: `internal/module/ai/agent/repository.go`
- Test: `internal/module/ai/agent/service_test.go`

- [ ] **Step 1: Add validated inputs**

Add `BillingMultiplier string` and `MaxOutputTokens int` to create/update input and DTOs. Parse the multiplier from a decimal string with at most six fractional digits; reject `<= 0` and any value whose PPM representation exceeds `math.MaxInt64`. Require positive output tokens and, when the model is cataloged, reject values above that catalog entry’s official `MaxOutputTokens`.

- [ ] **Step 2: Persist PPM and output cap**

Map the validated multiplier to `billing_multiplier_ppm` and `max_output_tokens`. Editing provider/model must not reset either field. Existing agents default to PPM `1000000` and the chosen `4096` max output during backfill.

- [ ] **Step 3: Add read-only price preview**

Expose the catalog version, resolved `catalog_vendor`, canonical model ID/base rates and the agent multiplier in the existing agent detail/page-init response. Format each non-negative `PriceUnits` through Plan 01's `sharedmoney.FormatRMBUnits`; `pricing/quote.go` remains responsible only for exact quote/allocation math and must not implement another API decimal formatter. `engine_type` remains transport information and is never shown as the official price owner. Do not add an admin mutation for base price and do not add supplier/model multiplier fields.

- [ ] **Step 4: Test validation and snapshot source**

Test `"1"`, `"1.25"`, `"0.000001"`, too many decimals, zero, negative, max output bounds, and preserving values on provider/model update. Run `go test ./internal/module/ai/agent -run 'Test.*Billing|Test.*Create|Test.*Update'`.

### Task 3: Define fail-closed pricing validation

**Files:**
- Modify: `internal/module/ai/pricing/catalog.go`
- Modify: `internal/module/ai/agent/service.go`
- Modify: `internal/module/ai/pricing/catalog_test.go`

- [ ] **Step 1: Reject unsafe price inputs**

Return a typed error for missing model identity, ambiguous catalog mapping, unsupported usage category or unsafe token upper bound. The Gateway must interpret it as zero provider calls and a user-facing configuration error.

- [ ] **Step 2: Keep recovery out of this phase**

Do not introduce cross-instance automatic billing blocks, block caches or new unblock permissions in this plan. A single request integrity failure is recorded by the finalizer and stops that Run; broader safety controls are outside the canonical Phase A scope.

### Task 4: Verify and commit

- [ ] **Step 1: Run focused checks**

Run `gofmt -w internal/module/ai/pricing internal/module/ai/agent`, `go test ./internal/module/ai/pricing ./internal/module/ai/agent`, and `git diff --check`.

- [ ] **Step 2: Commit**

```powershell
git add internal/module/ai/pricing internal/module/ai/agent
git commit -m "feat(ai): add official pricing and agent multiplier"
```
