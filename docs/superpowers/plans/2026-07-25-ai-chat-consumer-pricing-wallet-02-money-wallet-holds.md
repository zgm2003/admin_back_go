# Money Units 与钱包 Hold Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将钱包从 cents 切到 `money_units`，提供不可透支的 Run 级 reserve/top-up/capture/release，并删除 AI 退款入口。

**Architecture:** 钱包行是唯一余额事实；所有 Hold 操作在同一用户钱包行锁下完成，`available = balance_units - held_units`。Hold 只预占，不提前扣款；最终 finalizer 才 capture 实际金额并释放差额。支付订单、充值单和套餐继续以 cents 表示渠道面额，但只有 `payment/wallet` 能以 units 写 `user_wallets` 与 `wallet_transactions`；兑换码与支付充值都复用其事务 participant 和唯一来源幂等。

**Tech Stack:** Go、GORM、MySQL row locks、整数溢出检查、JSON 金额字符串。

---

### Task 1: Replace wallet models and money conversion helpers

**Files:**
- Modify: `internal/module/payment/wallet/model.go`
- Modify: `internal/module/payment/wallet/dto.go`
- Modify: `internal/module/payment/wallet/service.go`
- Test: `internal/module/payment/wallet/service_test.go`

- [ ] **Step 1: Add checked units conversion**

Use Plan 01's `internal/shared/money.CentsToUnits` and `FormatRMBUnits`; do not add a second payment-local multiplier or formatter. Payment callers must propagate negative/overflow/format errors and never use `float64`.

- [ ] **Step 2: Rename persistence fields**

Map wallet and transaction models to `balance_units`, `total_recharge_units`, `total_consume_units`, `held_units`, `amount_units`, `balance_before_units`, and `balance_after_units`. Keep no cents fallback in runtime code.

- [ ] **Step 3: Change API DTOs to strings**

Return `balance`, `available_balance`, `held_amount`, `amount`, `balance_before`, `balance_after`, `total_recharge`, and `total_consume` as decimal RMB strings. Do not serialize an `int64 money_units` JSON number and do not keep runtime cents aliases after the maintenance-window cutover.

- [ ] **Step 4: Test arithmetic**

Cover wallet DTO use of zero -> `"0"`, one unit -> `"0.00000001"`, one cent -> `"0.01"`, one RMB -> `"1"`, trailing-zero trimming, maximum safe cents, negative input and overflow from the shared helper. Run `go test ./internal/module/payment/wallet -run 'Test.*Money|Test.*Summary'`.

### Task 2: Implement Hold lifecycle under row locks

**Files:**
- Modify: `internal/module/payment/wallet/repository.go`
- Modify: `internal/module/payment/wallet/service.go`
- Create: `internal/module/payment/wallet/hold.go`
- Test: `internal/module/payment/wallet/repository_test.go`
- Test: `internal/module/payment/wallet/service_test.go`

- [ ] **Step 1: Define operations**

Add these interfaces and inputs:

```go
ReserveHoldInTx(ctx context.Context, tx *gorm.DB, in ReserveHoldInput) (*Hold, error)
TopUpHoldInTx(ctx context.Context, tx *gorm.DB, in TopUpHoldInput) (*Hold, error)
CaptureHoldInTx(ctx context.Context, tx *gorm.DB, in CaptureHoldInput) (*Wallet, *Transaction, error)
ReleaseHoldInTx(ctx context.Context, tx *gorm.DB, in ReleaseHoldInput) (*Hold, error)
CreditRechargeInTx(ctx context.Context, tx *gorm.DB, in CreditRechargeInput) (*Wallet, *Transaction, error)
```

Hold inputs include `user_id`, `run_id` and `amount_units`; for reserve/top-up this is the required target Hold total, never an additive delta. `run_id` is the single Hold owner and the final AI transaction source identity. `CaptureHoldInput` additionally requires a non-blank, at-most-255-character `source_summary` supplied by Gateway from persisted Run facts; wallet stores it as the transaction `remark` and does not import or query an AI module. `CreditRechargeInput` includes `user_id`, `recharge_id` and already-converted `amount_units`, and always writes `SourceRecharge`. Every operation is an outer-transaction participant and must reject a nil or root/non-transactional GORM handle, using the repository's existing `gorm.TxCommitter` guard pattern, so reserve/finalization/recharge can never split wallet and owner-state commits.

- [ ] **Step 2: Reserve/top-up atomically**

The Gateway-owned outer transaction first locks `Run -> Charge`, then calls `ReserveHoldInTx`/`TopUpHoldInTx` on that same handle. The wallet participant locks `user_wallets` and then `wallet_holds` with `FOR UPDATE`, verifies `balance_units >= held_units`, computes `available_units`, and calculates `delta = max(0, target_units-existing_hold_units)`. Reject when that delta exceeds available balance; otherwise add the same delta to both `user_wallets.held_units` and the Run Hold before commit. Equal/lower target replay returns the existing Hold without changing either row. Every committed operation must preserve `user_wallets.held_units = SUM(held_units WHERE wallet_id=? AND status='active')`. The participant never begins or commits its own transaction. Standalone wallet/Hold maintenance uses `wallet -> Hold`; no code may lock a Hold and then acquire its wallet.

- [ ] **Step 3: Capture/release atomically**

Lock wallet and Hold in that order and snapshot the active Hold total. Capture requires `actual_units <= hold.held_units`; in one commit subtract actual from wallet balance, add actual to total consume, subtract the full active Hold total from `user_wallets.held_units`, write one `SourceAIGenerate` transaction with `source_id=run_id` and `remark=source_summary` when actual is non-zero, set `hold.captured_units=actual_units`, `hold.held_units=0` and status captured. This releases `old_hold_units-actual_units` without a ledger entry. Release leaves balance unchanged, subtracts the full active Hold total from `user_wallets.held_units`, then sets `hold.held_units=0` and status released without an AI refund transaction. Both operations reject aggregate underflow and preserve the active-Hold sum invariant. When invoked by the Gateway finalizer, the caller has already locked `Run -> Charge`, so the complete cross-module order is `Run -> Charge -> wallet -> Hold`. Idempotent replay of a terminal Hold returns the original terminal fact and never creates a zero-value or duplicate transaction; it verifies the existing AI transaction belongs to the same user/Run and does not rewrite its summary.

- [ ] **Step 4: Test concurrency invariants**

Use the package’s existing MySQL GORM + `go-sqlmock` harness to test rejection of nil/root transaction handles, duplicate reserve, concurrent reserve, insufficient top-up, blank/oversized capture summary, capture transaction `source_id=run_id`, capture twice without summary rewrite, release twice, capture over hold, aggregate-held underflow, wallet-before-Hold acquisition, `balance_units - held_units >= 0`, and `user_wallets.held_units = SUM(active Hold units)` after reserve/top-up/capture/release. Run `go test ./internal/module/payment/wallet -run 'Test.*Hold|Test.*Debit|Test.*Credit|Test.*Summary'`.

### Task 3: Migrate existing wallet and redeem-code paths

**Files:**
- Modify: `internal/module/payment/repository.go`
- Modify: `internal/module/payment/service.go`
- Modify: `internal/module/payment/finalizer.go`
- Modify: `internal/module/payment/recharge_repository.go`
- Modify: `internal/module/payment/recharge_service.go`
- Modify: `internal/module/payment/recharge_dto.go`
- Delete: `internal/module/payment/wallet_model.go`
- Delete: `internal/module/payment/wallet_repository.go`
- Modify: `internal/platform/admin/build.go`
- Modify: `internal/platform/admin/build_test.go`
- Modify: `internal/runtime/worker.go`
- Modify: `internal/runtime/worker_test.go`
- Modify: `internal/module/payment/wallet/repository.go`
- Modify: `internal/module/payment/wallet/service.go`
- Modify: `internal/module/payment/redeemcode/service.go`
- Modify: `internal/module/payment/wallet/dto.go`
- Test: `internal/module/payment/finalizer_test.go`
- Test: `internal/module/payment/recharge_repository_test.go`
- Test: `internal/module/payment/recharge_service_test.go`
- Modify: `internal/module/payment/callback_behavior_test.go`
- Modify: `internal/module/payment/callback_service_test.go`
- Modify: `internal/module/payment/config_service_test.go`
- Modify: `internal/module/payment/jobs_test.go`
- Modify: `internal/module/payment/order_service_test.go`
- Modify: `internal/module/payment/test_fakes_recharge_methods_test.go`
- Delete: `internal/module/payment/wallet_repository_test.go`
- Test: `internal/module/payment/wallet/repository_test.go`
- Test: `internal/module/payment/redeemcode/service_test.go`

- [ ] **Step 1: Move recharge credit to the units-only wallet owner**

Keep `payment_orders`, `payment_recharges` and `payment_recharge_packages` cents fields because they are payment-channel denominations. Remove the root `payment` package's direct wallet/ledger write model and `CreditRecharge` implementation. Inject the wallet transaction participant into `payment.GormRepository`; its repository-owned outer transaction locks/finalizes order and recharge facts, performs checked `units = cents * 1000000` exactly once, calls `payment/wallet.CreditRechargeInTx`, and then commits. `FinalizeOrderPaid` orchestrates through that repository operation without importing GORM. The wallet participant owns wallet creation/lock, overflow checks, `SourceRecharge`, `(source_type, source_id)` replay and balance/total updates; no cents value may cross that participant boundary.

- [ ] **Step 2: Keep recharge reads and HTTP DTOs contract-correct**

Inject a read-only units wallet dependency into the payment service for PageInit/status responses. Recharge/package list amounts remain channel-denomination decimal RMB strings; wallet balance and totals come from units and use `sharedmoney.FormatRMBUnits`. Remove wallet cents JSON fields and do not duplicate a second wallet persistence model in the root payment package.

Construct one `payment/wallet` repository per process and pass it to the payment repository in both `internal/platform/admin/build.go` and `internal/runtime/worker.go`; callback, sync and worker reconciliation must therefore use the same units-only participant. Add composition tests that fail if either runtime constructs payment without it.

- [ ] **Step 3: Preserve redeem-code identity**

Use `SourceRedeemCode` and `(source_type, source_id)` exactly as today. The redeem-code owner calls the shared checked `CentsToUnits` exactly once before invoking the units-only wallet participant; no cents value crosses that participant method. Return the same transaction on replay, reject a code if the user/source ownership fact does not match, and do not create a refund source.

- [ ] **Step 4: Remove the AI refund symbol**

Delete `SourceAIRefund`, its dictionary option and all service/test references. Keep `SourceAIGenerate` for the final capture only; failed/unknown AI calls release the Hold with no compensating transaction.

- [ ] **Step 5: Test recharge, replay and no-refund behavior**

Cover callback/sync racing to finalize the same recharge, one cents-to-units conversion, one `SourceRecharge` ledger row, outer-transaction rollback, source-owner mismatch and wallet overflow. Run `rg -n 'SourceAIRefund|ai_refund' internal database contracts`; expected no runtime or contract result. Run `go test ./internal/module/payment ./internal/module/payment/redeemcode ./internal/module/payment/wallet -run 'Test.*Recharge|Test.*Finalize|Test.*Redeem|Test.*Refund|Test.*Replay'`.

### Task 4: Verify migration readiness without executing migration

- [ ] **Step 1: Check unit-only runtime references**

Run `rg -n 'BalanceCents|AmountCents|balance_cents|amount_cents' internal/module/payment`. Wallet and wallet-transaction runtime models must have no match. Remaining matches are allowed only on `payment_orders`, `payment_recharges`, `payment_recharge_packages` and redeem-code denomination inputs; every credit path must show exactly one checked conversion before entering the units-only wallet participant.

- [ ] **Step 2: Fast formatting and commit**

Run `gofmt -w internal/module/payment internal/platform/admin/build.go internal/platform/admin/build_test.go internal/runtime/worker.go internal/runtime/worker_test.go`, the focused payment/wallet/redeem tests plus `go test ./internal/platform/admin ./internal/runtime -run 'Test.*Payment|Test.*Wallet|Test.*Build|Test.*Worker'`, and `git diff --check`.

```powershell
git add internal/module/payment internal/platform/admin/build.go internal/platform/admin/build_test.go internal/runtime/worker.go internal/runtime/worker_test.go
git commit -m "feat(wallet): add money units and run holds"
```
