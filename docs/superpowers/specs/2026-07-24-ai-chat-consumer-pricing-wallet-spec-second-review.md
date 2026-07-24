# AI 消费者对话、定价与钱包合并 Spec 二次复核

**日期：** 2026-07-24  
**审查对象：** `2026-07-24-ai-chat-consumer-pricing-wallet-design.md`  
**结论：** Request changes；只剩 2 组阻断项，修订后再做一次秒级静态复核

## 已确认关闭

- 普通重试已复用原 Run、Charge、Hold、价格快照和绝对截止时间。
- 最终不可变计费明细已改由 finalizer 同事务写入。
- 用户主动删除已使用 `user_deleted` 与平台损坏区分。
- `request_id` 已明确使用 binary collation。
- Run 状态与 Run Event 已要求原子提交。
- 所有付费入口已统一为 durable task runner 所有执行权。
- 媒体结果已补充不可变 `storage_location_id` 与存储恢复语义。
- 钱包 units 已改为 expand/backfill/validate/contract。
- 新权限已明确“只注册、不自动授权”，并在恢复管理页面前由用户人工授权。

## 仍需修订

### 1. 取消胜出且 usage 不完整时，状态仍自相矛盾

第 747 行先把 Command/Run 一律写成 `canceled`，同时让 Charge 保持 `uncertain`；第 769、781 行却要求这种情况保持 `outcome_unknown/uncertain`。第 745 行的“用量不完整立即 `unbilled` 并释放”也没有排除取消后仍处于对账期限内的场景。

必须明确分支优先级：

| 取消胜出后的证据 | Command / Run | Charge / Hold |
| --- | --- | --- |
| usage 完整 | `canceled` | 按实际用量 `settled`，释放差额 |
| 已证明未执行或无 usage | `canceled` | `released` |
| usage 不完整且未到截止时间 | `outcome_unknown`，保留 `cancel_requested_at` | `uncertain`，继续冻结 |
| 到截止时间仍无可靠 usage | `canceled` | `unbilled` 并释放 |

同步修订第 11.4 节的 finalizer 分支：不能先无条件写 `canceled`，也不能把仍应对账的取消请求提前按 `unbilled` 释放。

### 2. 普通重试的冻结上界和统一 Runner 顺序没有闭合

第 524、675 行允许新 `attempt_no` 发起真实重试，但没有明确该重试在派发前如何覆盖可能收费的失败 attempt。若 `PD-07` 决定失败 attempt 也收费，多次调用的累计实际费用可能超过首次 Hold。

必须固定一种实现规则：

- 首次 Hold 覆盖 `max_attempts` 及允许工具轮次的最坏总上界；或
- 每个新 `attempt_no` 派发前，按“已发生的可收费 usage + 下一次调用保守上界”补充同一个 Hold，补充失败则不得派发。

同时修订第 1087-1092 行统一路径。当前写成“创建 Hold -> 检查 pricing -> dispatched”，顺序不成立且缺少报价/冻结动作。应改为：

```text
claim/续租
  -> 创建或恢复 Run / Charge
  -> 检查 pricing、billing block、storage readiness
  -> 计算本次所需保守上界
  -> reserve 或 top-up 同一 Hold
  -> 持久化 provider-attempt dispatched
  -> 调用供应商
```

## 修订门槛

只需全文静态确认以下三点，不运行测试、Docker 或 Playwright：

1. `canceled + uncertain` 的冲突组合已消失。
2. 未到截止时间的取消不完整 usage 不会提前 `unbilled/released`。
3. 每次新的真实供应商调用都在派发前拥有足够冻结上界。

以上关闭后，技术架构复核可通过；随后只剩 `PD-01` 至 `PD-08` 由用户确认，确认后再分别编写小型 implementation plan。
