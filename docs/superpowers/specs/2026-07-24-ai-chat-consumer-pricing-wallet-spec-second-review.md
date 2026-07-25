# AI 消费者对话、定价与钱包合并 Spec 二次复核

**原始复核日期：** 2026-07-24
**闭环日期：** 2026-07-25
**审查对象：** `2026-07-24-ai-chat-consumer-pricing-wallet-design.md`
**结论：** 技术架构复核通过；进入 implementation plan 前仍须由用户确认 `PD-01` 至 `PD-08`

## 已确认关闭

- 普通重试复用原 Run、Charge、Hold、价格快照和绝对截止时间。
- 最终不可变计费明细由 finalizer 同事务写入。
- 用户主动删除使用 `user_deleted` 与平台损坏分离。
- `request_id` 明确使用 binary collation。
- Run 状态与 Run Event 原子提交。
- 所有付费入口统一由 durable task runner 持有执行权。
- 媒体结果保存不可变 `storage_location_id` 与存储恢复语义。
- 钱包 units 使用 expand/backfill/validate/contract 迁移。
- 新权限只注册、不自动授权，并在恢复管理页面前由用户人工授权。
- 取消胜出且 usage 不完整时的业务/财务状态冲突已经关闭。
- 每次普通重试和工具轮次的派发前冻结上界已经闭合。

## 阻断项关闭记录

### 1. 取消胜出后的状态采用证据优先级

正文第 11.4、11.5 节现在固定以下唯一组合：

| 取消胜出后的证据 | Command / Run | Charge / Hold |
| --- | --- | --- |
| usage 完整 | `canceled` | 按实际用量 `settled`，释放差额 |
| 已证明未执行或无可计费用量 | `canceled` | `released` |
| usage 不完整且未到截止时间 | `outcome_unknown`，保留 `cancel_requested_at` | `uncertain`，继续冻结 |
| 到截止时间仍无完整可靠 usage | `canceled` | `unbilled` 并释放 |

取消请求在行锁竞争中胜出后，候选内容立即 `discarded` 并清空，后续永不发布；这不允许业务状态提前伪装成终态。截止前的 usage 对账只追加 outcome-unknown Run Event，不追加 canceled event，也不写最终计费明细。普通“有效结果但 usage 不完整则免费发布”的规则只适用于取消未胜出的请求。

### 2. 普通重试使用同一 Hold 的累计上界补充

正文第 9.3、10.4、10.11、11.3、17、22 和 23 节统一采用：

```text
本次派发所需冻结总额
  = 之前所有仍可能向用户收费的完整实际用量金额
  + 本次待派发调用的保守费用上界
```

普通重试不在首次请求时冻结全部 `max_attempts`。每个新的 `attempt_no` 和未被整次上界覆盖的工具轮次，都在派发前 reserve/top-up 同一个 Charge 对应的 Hold。`PD-07` 未确认时，失败 attempt 的完整用量按“仍可能收费”参与冻结上界，但不代表已经决定扣款。先前 attempt 用量不完整时不得普通重试，除非权威证据证明未执行或没有可计费用量。top-up 失败时不得写新的 `dispatched` 或调用供应商。

统一 Runner 顺序已经固定为：

```text
claim/续租
  -> 创建或恢复 Run / Charge
  -> 检查 pricing、billing block、storage readiness
  -> 构造最终请求并计算保守费用上界
  -> reserve 或 top-up 同一个 Hold
  -> 创建或恢复 provider-attempt prepared
  -> 持久化 provider-attempt dispatched
  -> 调用供应商
```

## 静态复核结果

1. `canceled + uncertain` 的冲突状态组合已经从规范分支中消失。
2. 未到 `resolution_deadline_at` 的取消不完整 usage 明确保持 `outcome_unknown/uncertain`，不会提前 `unbilled/released`。
3. 每次新的真实供应商调用都必须先完成报价和充足的 reserve/top-up，之后才能进入 `dispatched`。
4. `git diff --check` 通过。
5. 按用户约束未运行测试、Docker 或 Playwright。
