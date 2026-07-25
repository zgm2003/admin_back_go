# 充值 PageInit 精简 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 从充值收银台 PageInit 删除无人消费的最近充值查询与字段，同时完整保留充值记录能力。

**Architecture:** PageInit 只返回钱包摘要、套餐、支付方式和字典；历史记录继续由现有分页列表 API 独立加载。此计划不改支付状态机、钱包入账、继续支付或 money-units 规则。

**Tech Stack:** Go payment service/repository、compiled Admin contract tests。

---

### Task 1: Remove only the `recent` PageInit slice

**Files:**
- Modify: `internal/module/payment/recharge_dto.go`
- Modify: `internal/module/payment/recharge_service.go`
- Modify: `internal/module/payment/recharge_repository.go`
- Modify: `internal/module/payment/repository.go`
- Modify: `internal/module/payment/recharge_service_test.go`
- Modify: `internal/module/payment/recharge_repository_test.go`
- Modify: `internal/module/payment/test_fakes_recharge_methods_test.go`

- [ ] **Step 1: Write the failing service contract test**

Assert `RechargePageInitResponse` has no `recent` JSON property and PageInit never invokes a recent/list-recharge repository method. Separately assert `ListRecharges` still returns its paginated records and filters.

- [ ] **Step 2: Delete the dedicated query path**

Remove `Recent`, `defaultRechargeRecentLimit`, `ListRecentRecharges` from the payment repository interface/implementation/fakes, and the PageInit call. Do not remove `RechargeListItem`, `ListRecharges`, detail, pay, sync, close, package or wallet summary behavior.

- [ ] **Step 3: Prove record behavior is unchanged**

Keep tests for pagination, status/date/keyword filters, `pay_url`, continue-payment and the Phase A decimal-money fields. Do not replace the removed field with an empty array or compatibility alias.

### Task 2: Focused verification

- [ ] **Step 1: Run the payment slice**

Run `gofmt -w internal/module/payment/recharge_dto.go internal/module/payment/recharge_service.go internal/module/payment/recharge_repository.go internal/module/payment/repository.go`, then `go test ./internal/module/payment -run 'Test.*RechargePageInit|Test.*ListRecharges|Test.*RechargeList' -count=1` and `git diff --check`.

- [ ] **Step 2: Confirm the boundary by search**

Run `rg -n 'ListRecentRecharges|defaultRechargeRecentLimit|json:"recent"' internal/module/payment`; expected no matches. `rg -n 'ListRecharges' internal/module/payment` must still show the records API path.

- [ ] **Step 3: Commit**

```powershell
git add internal/module/payment/recharge_dto.go internal/module/payment/recharge_service.go internal/module/payment/recharge_repository.go internal/module/payment/repository.go internal/module/payment/*recharge*test.go
git commit -m "refactor(payment): remove recharge page recent query"
```
