# AI 消费者对话、定价与钱包合并 Spec 细审整改清单

**日期：** 2026-07-24
**审查对象：** `docs/superpowers/specs/2026-07-24-ai-chat-consumer-pricing-wallet-design.md`
**审查结论：** Request changes，当前禁止进入 implementation plan

## 1. 使用说明

本文交给负责修订合并 Spec 的 AI 阅读。目标不是继续扩大需求，而是关闭现有状态机、事务和迁移路径中的矛盾，使合并 Spec 能够安全地作为后续实施依据。

修订时遵守以下硬约束：

1. 只修订合并 Spec，不实施代码，不编写 implementation plan。
2. 不删除两份源 Spec 已确认的消费者交互或定价需求。
3. 不自行决定本文标记为“需要用户确认”的产品规则。
4. 不新增计费专用密钥，不修改 `APP_SECRET`、`APP_SECRET_PREVIOUS` 或 `role_permissions`。
5. 不运行 Docker、Playwright、全仓测试或任何长测试；本轮只是文档修订与静态核对。
6. 修订后必须重新检查全文，不得只删除文档顶部的 P0 提示而保留正文旧规则。

## 2. P0 阻断项

### 2.1 取消后的计费规则仍然自相矛盾

冲突位置：

- 合并 Spec 第 7 行已经写明：聊天取消不是当然免费的终态，能够取得的供应商实际分类用量应当结算。
- 第 727、737、747 行仍规定 running 取消直接释放冻结、不扣款。
- 第 1244 行仍把取消与“不能捕获金额”混在一起。

必须把取消路径统一改成以下语义：

| 场景 | Command / Run | Charge / Hold | 助手结果 |
| --- | --- | --- | --- |
| provider-attempt 派发前取消 | `canceled` | 无账单或 `released` | 不产生回复 |
| 供应商明确确认取消且没有可计费用量 | `canceled` | `released` | 不产生回复 |
| 请求已经派发，取消后取得完整合法用量 | `canceled` | 按实际用量 `settled`，释放差额 | 丢弃内容，不产生回复和未读 |
| 取消后只取得不完整用量 | 保持待对账状态 | `uncertain`，继续冻结至截止时间 | 不产生回复 |
| 截止时间仍无法取得准确用量 | `canceled` | `unbilled` 并释放，平台承担成本 | 不产生回复 |
| 完成事务先于取消请求提交 | `succeeded` | 正常 `settled` 或 `unbilled` | 返回既有成功结果 |

取消请求写入 `cancel_requested_at` 后不能立即把 running 命令终结。驱动应尝试取消供应商请求并继续取得最终 usage 或权威任务状态。取消 finalizer 必须能够在“业务 canceled、账单 settled”这一合法组合下提交终态。

若取消已经赢得竞态，随后到达的完整内容不得发布为助手消息；但其完整合法用量仍按已确认规则结算。无法取得用量时禁止猜价，最终 `unbilled`。

需要同步修订：业务规则、账单状态说明、11.4、11.5、停止接口、并发规则、错误码、测试范围和验收标准。

### 2.2 普通可重试失败缺少完整生命周期

现有实现 `internal/module/ai/replycommand/runner.go:252` 会把可重试错误从 `running` 放回 `pending`，默认最多尝试三次。合并 Spec 只描述了终态失败和 `outcome_unknown` 恢复，没有描述普通可重试失败。

必须补充：

1. 已证明没有形成成功结果的可重试错误进入 `command: pending`，而不是立即终结整个 Run。
2. 同一逻辑请求继续复用原 Run、Charge、价格快照和智能体倍率快照。
3. Run 在重试等待期间保持非终态，并追加幂等的 `retry_scheduled` 或等价 Run Event。
4. Charge 与非零 Hold 保持非终态，不能先 `released` 再试图复活终态账单。
5. 下一次真实供应商调用使用新的 `attempt_no`；只有在重放同一供应商调用且供应商保证幂等时才复用原 provider idempotency key。
6. 只有不可重试错误或耗尽 `max_attempts` 后，失败 finalizer 才统一终结 Command、Run、Charge、Hold 和 durable failed event。
7. `resolution_deadline_at` 是否继续约束普通重试必须写死，不能因重新 claim 或进程重启无限延长。

仍需用户确认一个业务问题：供应商明确失败但返回了可计费用量，之后平台重试成功时，失败尝试的用量是否向用户收费。修订 AI 不得自行把“每次真实调用都计量”解释成“每次失败尝试都向用户扣款”。

### 2.3 不可变计费明细的写入时机错误

当前合并 Spec 同时规定：

- `ai_usage_charge_items` 是追加写、不可变事实，并保存用户实际扣款金额；
- provider-attempt、计费明细和结果候选先在一个短事务中写入；
- 后续 finalizer 锁钱包后，才可能因为补差余额不足把账单改成 `unbilled`，用户实际扣款变为零。

这三个规则不能同时成立。finalizer 之前无法确定最终用户实际扣款金额。

必须采用以下路径之一，推荐方案 A：

**方案 A，推荐：** provider-attempt 或候选阶段只持久化经过校验的归一化原始用量和用量指纹；finalizer 确定 `settled/released/unbilled` 后，在同一终态事务中一次性插入最终不可变的 `ai_usage_charge_items`。

**方案 B：** 计费明细只保存数量、价格和理论计算金额，不保存用户实际捕获分配；实际扣款只保存在 Charge 和独立的最终分配事实中。

不得先写非零“用户实际扣款金额”，再因 `unbilled` 回写为零。

同时写死数据库唯一语义：`tier_key` 等参与唯一身份的字段必须使用非空规范值，例如空字符串，而不能依赖 MySQL 对 nullable unique column 的行为阻止重复。

### 2.4 用户主动删除与平台结果损坏没有区分

合并 Spec 承诺成功结果长期重放、平台永久损坏已扣款结果时完整退款，但当前图片能力已经存在用户软删除：

`internal/module/ai/image/repository.go:155`

如果不区分删除原因，会产生两类错误：

1. 用户下载结果后主动删除，再被完整性检查误判为平台损坏并退款；
2. 同一 `request_id` 重放时查不到软删除任务，错误创建新任务或再次调用供应商。

必须增加明确的用户删除终态或等价事实，例如 `result_state=user_deleted`：

- 用户主动删除不退款，不修改旧账单和钱包流水；
- 相同 `request_id` 重放返回同一已删除任务和稳定状态，不重新生成；
- 用户要再次生成必须使用新的 `request_id`；
- 平台完整性损坏才允许 `finalized -> unavailable` 和幂等完整退款；
- 对象是否继续保留、何时删除必须由明确产品规则决定，不能让孤儿清理器自行判断。

聊天消息软删除继续遵循已有规则：不退款、不删除 Run 或财务事实。

### 2.5 媒体结果存储路径尚不能落地

合并 Spec 把 `storage_provider + storage_key` 定义为对象身份，但现有 COS 访问实际还依赖 bucket、region、endpoint 或稳定配置身份。上传配置更换后，相同 provider/key 不能定位原付费结果。

必须补充：

1. 结果任务快照不可变的 `storage_location_id` 或等价 bucket/region/endpoint 身份，不保存秘密明文。
2. 图片、视频、音频在供应商派发前检查结果存储 readiness；存储不可用时不得先调用供应商。
3. 私有结果的授权读取契约，明确使用后端代理读取还是短时签名 URL。
4. 大视频使用流式或 multipart 上传，禁止复用当前只接受 `[]byte` 的 `ObjectWriter` 把完整视频放进内存。
5. 为确定性 key 恢复提供受控的 Stat/HEAD 能力，为 discarded/orphan 清理提供幂等 Delete 能力。
6. `staged`、`finalized`、`user_deleted` 和孤儿对象必须使用互不混淆的前缀与清理规则。
7. 继续复用现有加密 COS 配置和根密钥体系，不新增计费专用密钥或环境变量。

当前相关能力只有 `Put(ctx, PutInput)`，参考 `internal/infra/storage/cos/object_writer.go:33`，不能把 HEAD、Delete、流式上传和授权下载当成已经存在。

## 3. P1 重要修订

### 3.1 工具多轮调用的余额策略存在免费消耗漏洞

当前规则允许每一轮前补充冻结，但又规定后续补充冻结失败时释放整个 Run，不捕获此前费用。第一轮真实模型调用已经产生供应商成本。

必须选择并写死一种策略：

1. 首次冻结覆盖产品允许的全部工具轮次、最大工具输出和最大模型输出；或
2. 后续余额不足时停止新调用，但结算此前已经发生且能够准确取得的实际用量。

推荐优先采用可证明的整次最大报价；无法形成安全整次上界时，采用第二种实际用量结算，不应默认把已发生费用全部释放。

### 3.2 `request_id` 大小写规则与数据库排序规则冲突

合并 Spec 要求 `request_id` 区分大小写，但当前 schema 默认排序规则为 `utf8mb4_0900_ai_ci`：

`database/schema/admin.hcl:5619`

现有 `ai_reply_commands.request_id` 也是普通 `VARCHAR(128)`：

`database/schema/admin.hcl:1539`

所有参与幂等唯一约束的 `request_id` 必须显式使用 binary collation，或改成受控 ASCII 格式并按二进制存储。应用比较规则与数据库唯一规则必须完全一致。`legacy:` 保留前缀是否忽略大小写也要写死。

### 3.3 统一 finalizer 必须同时写 Run Event

合并 Spec 只写“将 Run 设为成功/失败”，但现有 Run recorder 会在同一事务中更新 Run 并追加顺序 `ai_run_events`：

`internal/module/ai/run/recorder_repository.go:121`

Run 模块提供给 finalizer 的事务参与接口必须原子完成：

- 状态条件更新；
- `finished_at` 或非终态时间字段；
- Token/用量监控汇总；
- assistant message 绑定；
- 幂等追加 success、failed、canceled、timeout、outcome_unknown 或 retry 事件。

不能为了统一财务事务而丢失 AI Runs 时间线。

### 3.4 钱包单位迁移应避免破坏性中间状态

当前迁移顺序先删除钱包 cents 字段，再转换钱包流水。MySQL DDL 存在隐式提交，中途失败会形成钱包和流水使用不同单位的半迁移数据库。

维护窗口内应采用：

1. 停止全部钱包写入者并备份；
2. 为钱包和流水同时增加 units 字段；
3. 完成全部溢出前检、回填、逐行与汇总守恒校验；
4. 验证新二进制只读写 units；
5. 最后统一删除 cents 字段并恢复流量。

维护窗口内没有并发写入，因此临时同时存在两组列不等于双写或两套业务事实。失败时可以在删除旧列前修复或重跑，而不是只能整库恢复。

### 3.5 同步 HTTP 生成缺少持久化执行所有权

合并 Spec 允许文本、工具草稿和音频继续在当前 HTTP 请求内调用供应商。当前实现把请求 context 直接传给供应商，客户端断开即可取消 context，留下任务、Run、Hold 或 provider-attempt 待恢复。

必须选定：

1. 推荐：所有付费生成都交给 durable task runner，HTTP 可以等待同一任务的短时结果，从而保留同步体验；或
2. 请求内执行者必须先取得任务 lease，并使用独立、有界、可被后台接管的任务 context，不能把 HTTP 连接生命周期当作执行所有权。

无论选择哪种，同一任务只能有一个合法执行 owner，HTTP 断开不能导致第二次供应商调用。

### 3.6 权限发布路径需要说明人工授权时点

合并 Spec 要新增 `ai_run_list`、`ai_pricing_list` 权限定义且禁止自动写 `role_permissions`。这会使没有人工授权的角色在发布后无法访问相应页面。

发布章节应写明：权限定义随迁移发布，但角色授权由用户在恢复相关管理页面前手动完成。不得自动授权，也不能假设现有角色天然拥有新 code。

## 4. 需要用户明确确认的新增产品政策

以下内容超出了两份源 Spec 原先明确确认的范围。若用户未在其他对话逐项批准，不得继续写在“已确认业务规则”中：

1. 所有成功文本和媒体结果本期无限期保存，不设置成功结果 TTL。
2. 平台永久损坏任何已扣款结果时执行一次完整退款，不支持部分退款。
3. 图片部分成功后，未来只损坏其中一个输出时是否仍完整退款。
4. 各模态 1 小时、6 小时、24 小时的绝对恢复截止时间。
5. 单次计量异常是否立即永久 block 整个 billing safety profile。
6. block 不提供 Admin 解封，只能通过修订并递增 safety revision 恢复。
7. 供应商明确失败但返回可计费用量时，失败尝试是否由用户承担。
8. 用户主动删除付费媒体后，对象是立即清理、保留审计期还是长期保留。

在用户确认前，应把这些放入“待确认决策”，不能伪装成已经批准的需求。

## 5. 修订后必须形成的聊天统一操作路径

修订后的 Spec 至少要明确以下顺序：

```text
HTTP 接收事务
  -> 幂等查询和指纹校验
  -> 写用户消息/历史改写
  -> 写 reply command
  -> 提交并唤醒 Worker

Worker 执行
  -> claim command
  -> 创建或恢复 Run / Charge
  -> 组装最终供应商请求
  -> 检查 pricing、billing block、storage readiness
  -> 计算报价并 reserve/top-up
  -> 持久化 provider-attempt dispatched
  -> 调用供应商

供应商返回
  -> 持久化 attempt 结果
  -> 持久化归一化原始用量
  -> 持久化可恢复结果候选
  -> 禁止再次调用供应商

统一 finalizer
  -> 锁定 command / candidate / charge / wallet
  -> 决定 settled / released / unbilled
  -> 写最终不可变 charge items
  -> capture 或 release
  -> 写 Command 与 Run 终态及 Run Event
  -> 成功时写 assistant message
  -> 写 durable completed / failed / canceled event
  -> 清理候选正文
  -> 提交后 best-effort 实时推送
```

普通重试、用户取消、`outcome_unknown`、finalizer 重试和绝对截止恢复都必须回到这条路径，不能由 chat service、Run recorder、wallet service 和 reconciler 各自 best-effort 写终态。

## 6. 后续实施必须拆分，不写单个巨型 Plan

合并 Spec 可以作为总设计，但批准后应拆成多个有依赖顺序的实施计划：

1. 钱包 money units 迁移、价格目录和基础金额算法；
2. Charge、Hold、provider-attempt 和公共 finalizer 参与接口；
3. 聊天计费生命周期、取消/重试/恢复与消息交互；
4. 文本与工具草稿任务幂等和结算；
5. 图片、视频、音频的私有结果存储、幂等和结算；
6. Admin 价格/Run/钱包界面、权限与发布收尾。

每份计划只列当前阶段的短针对性检查。不得把所有模态、钱包迁移、前后端和完整验证塞进一份数千行计划。

## 7. 修订完成门槛

负责修订的 AI 在交回文档前必须逐项确认：

- 顶部 P0 提示与正文取消规则完全一致；
- 普通可重试错误、取消和 outcome-unknown 都有闭合状态转换；
- 每个状态转换都明确 Command、Run、Charge、Hold、Result、Event 的结果；
- 不可变计费明细只在最终金额确定后形成；
- 用户删除与平台损坏不会混淆或错误退款；
- 媒体存储身份在配置切换后仍能定位旧结果；
- request ID 应用比较与数据库唯一约束一致；
- Run 状态和 Run Event 原子一致；
- 钱包迁移不会留下 cents/units 混合中间状态；
- 同步 HTTP 断开不会导致重复调用；
- 所有新增产品政策已经得到用户确认，或明确留在待确认区；
- 长测试仍由用户手动选择执行。

只有以上问题全部关闭，合并 Spec 才能把状态改为“待用户最终审核”。用户审核通过之后，才能开始分别编写实施计划。
