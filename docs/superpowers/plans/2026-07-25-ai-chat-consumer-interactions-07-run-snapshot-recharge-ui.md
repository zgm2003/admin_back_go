# Run 快照与充值页 UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 结构化展示 Run 输入快照，并从充值收银台彻底移除最近充值区域而不影响充值记录。

**Architecture:** Run 快照解析是纯前端显示适配，只消费契约中的 `input_snapshot` 原文并保留安全回退；不改历史数据。充值页直接消费已删除 `recent` 的生成类型，移除专用状态/组件/样式，记录 Tab 继续调用独立列表 API。

**Tech Stack:** Vue 3、TypeScript strict parsing、Element Plus image preview、Vitest/Vue Test Utils。

---

### Task 1: Build a safe snapshot view model

**Files:**
- Create: `..\admin_front_ts\src\views\Main\ai\runs\components\RunList\input-snapshot.ts`
- Create: `..\admin_front_ts\tests\shared\ai\ai-run-input-snapshot.test.ts`
- Modify: `..\admin_front_ts\src\lib\browser\download.ts`
- Modify: `..\admin_front_ts\tests\unit\browser\download.test.ts`

- [ ] **Step 1: Write parser tests first**

Cover plain text, outer JSON with content/attachments/runtime params, outer JSON with stringified `meta_json`, malformed outer JSON, malformed nested JSON, missing optional fields, non-HTTP attachment URL and mixed attachment types. Required unknown shapes must return raw fallback, not an invented partial business object.

- [ ] **Step 2: Parse strictly without `any` or field aliases**

Return a discriminated view model: `{kind:'structured', content, attachments, runtimeParams}` or `{kind:'raw', text}`. Only use documented `content`, `attachments`, `runtime_params` and `meta_json`; parse nested `meta_json` once when it is a string. Export a focused `resolveTrustedFileURL(input: string): string` from `src/lib/browser/download.ts` and make both downloads and snapshot previews use its existing rule: same-origin URLs or no-port allowlisted HTTPS hosts, with credentials and other schemes rejected. Invalid preview URLs remain escaped display text; keep the original snapshot text for whole-object fallback. Extend the existing download unit test so the shared parser accepts same-origin/allowlisted HTTPS and rejects credentialed, non-allowlisted and non-HTTP(S) inputs.

### Task 2: Render structured Run input

**Files:**
- Modify: `..\admin_front_ts\src\views\Main\ai\runs\components\RunList\RunDetailDialog.vue`
- Modify: `..\admin_front_ts\src\views\Main\ai\runs\components\RunList\detail-dialog.ts`
- Modify: `..\admin_front_ts\tests\integration\features\ai-runs.test.ts`
- Create: `..\admin_front_ts\tests\component\ai\RunInputSnapshot.test.ts`

- [ ] **Step 1: Add a compact operational layout**

Replace the raw interpolation with unframed labeled sections for user text, attachments and runtime parameters. Show image thumbnails with the existing preview component; show name/type/size/URL for all documented attachments. Keep typography compact for an admin detail dialog, avoid nested cards and preserve stable dimensions for long names/URLs.

- [ ] **Step 2: Keep history readable and safe**

Render raw fallback as normal escaped text, never `v-html`. Invalid/untrusted URLs are text only. Empty optional sections are omitted without feature-explanation copy; a parser failure must not break the rest of Run detail, billing or usage sections.

- [ ] **Step 3: Test structured, raw and hostile input**

Assert text escaping, no HTML execution, no preview for rejected schemes, correct nested meta display, image preview for trusted URLs and unchanged Run detail loading/error behavior.

### Task 3: Remove the recharge recent region

**Files:**
- Modify: `..\admin_front_ts\src\api\payment\recharges.ts`
- Modify: `..\admin_front_ts\src\views\Main\payment\recharge\index.vue`
- Modify: `..\admin_front_ts\src\views\Main\payment\recharge\composables\usePaymentRechargePage.ts`
- Delete: `..\admin_front_ts\src\views\Main\payment\recharge\components\RechargeRecentRecords.vue`
- Modify: `..\admin_front_ts\src\i18n\locales\zh-CN\payment.ts`
- Modify: `..\admin_front_ts\src\i18n\locales\en-US\payment.ts`
- Modify through locale generator: `..\admin_front_ts\src\i18n\locales\generated.ts`
- Modify: `..\admin_front_ts\tests\component\payment\PaymentRechargePage.test.ts`
- Create: `..\admin_front_ts\tests\shared\payment\payment-recharge-api.test.ts`

- [ ] **Step 1: Make the generated type failure visible**

Update tests to use the Phase B PageInit without `recent`; existing code should fail type/test assertions until all stale reads are removed. Keep records fixtures on the independent list response.

- [ ] **Step 2: Remove state, component and layout area**

Delete `recent`, its PageInit assignment, component import/render, grid area, responsive CSS and now-unused locale strings. Rebalance the cashier as a single focused band using existing tokens; do not replace the area with a placeholder/card or load records on cashier initialization.

- [ ] **Step 3: Prove records and continue-pay remain**

Test cashier PageInit, switching to records, paginated list loading, pay/continue-pay and return-to-records behavior. The API adapter must follow generated fields exactly with no `recent: []` fallback.

### Task 4: Focused handoff

- [ ] **Step 1: Run targeted frontend tests**

From the frontend root run `npm test -- tests/unit/browser/download.test.ts tests/shared/ai/ai-run-input-snapshot.test.ts tests/integration/features/ai-runs.test.ts tests/component/ai/RunInputSnapshot.test.ts tests/component/payment/PaymentRechargePage.test.ts tests/shared/payment/payment-recharge-api.test.ts`. Run typecheck only if under two minutes; do not run Playwright, full tests or build automatically.

- [ ] **Step 2: Search stale UI references**

Run `rg -n 'RechargeRecentRecords|paymentRecharge\.recent|\.recent\b' src/views/Main/payment/recharge src/api/payment/recharges.ts src/i18n/locales`; expected no recharge-recent runtime matches.

- [ ] **Step 3: Commit**

```powershell
git add src/api/payment/recharges.ts src/lib/browser/download.ts src/views/Main/ai/runs src/views/Main/payment/recharge src/i18n/locales tests/unit/browser/download.test.ts tests/shared/ai tests/integration/features/ai-runs.test.ts tests/component/ai tests/component/payment/PaymentRechargePage.test.ts tests/shared/payment/payment-recharge-api.test.ts
git commit -m "feat(ai): present run input and simplify recharge"
```
