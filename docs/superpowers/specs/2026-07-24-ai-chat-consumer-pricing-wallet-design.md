# AI Gateway、官方数值定价与钱包结算设计

**修订日期：** 2026-07-25
**状态：** 技术架构与核心产品决策已确认；阶段 A、阶段 B 均已拆分 implementation plan，等待按执行索引分波实施。
**唯一规范性 Spec：** 本文是本需求唯一可用于计划和实现的规范。此前版本保留在 Git 历史及两份 `*-spec-review.md` 审查记录中，但不再具有规范性。

## 1. 目标与收敛原则

项目需要让消费者的 AI 使用、钱包余额和实际供应商用量形成闭环，同时不把模块化单体演变成一个自建的 Sub2API。

本期采用一个进程内的 `aigateway` 模块：

```text
消费者会话 / 智能体 / 钱包
        -> aigateway
        -> 官方 API、Sub2API 或其他专业上游
```

`aigateway` 是项目内部边界，不是独立微服务。它统一处理报价、冻结、上游派发、归一化用量、结算和停止语义；它不重复实现上游的模型路由、聚合计费或账户系统。现有供应商继续使用其 `base_url + api_key` 配置，不增加计费专用密钥或环境变量。

此前规范把消费者交互、精确计费、跨实例长期恢复、自动封禁和钱包大迁移一次性捆绑，导致范围失控。本规范按阶段交付：

1. **阶段 A：计费核心。** `aigateway`、官方数值价格、冻结/结算、幂等、聊天停止，以及图片/视频任务取消。
2. **阶段 B：消费者交互。** 消息选择与软删除、编辑、重新生成、未读、Run 点赞、浏览器朗读、Run 输入快照结构化展示和充值页精简；它们复用阶段 A 的 Run、幂等和结算状态，但不阻塞阶段 A 上线。

阶段 A 的计划不得混入阶段 B 的大面积 UI 改造。两个阶段仍以本文为唯一业务依据，各自写独立、短小的 implementation plan。

## 2. 已确认的业务规则

### 2.1 定价

1. 模型基础价来自审核后录入仓库的官方公开价格目录，不在运行时抓取网页或信任第三方聚合价格表。
2. 使用 `official_numeric_parity_v1`：官方 `$5 / 1M input tokens` 在本平台直接表示为 `¥5 / 1M input tokens`。这不是汇率换算，不存在 FX 配置。
3. 供应商和供应商模型不配置倍率。每个智能体只保存一个大于零的 `billing_multiplier`，默认 `1.0`；后端以 PPM 定点整数保存（`1_000_000 = 1.0`），接口使用十进制字符串，不使用浮点数。
4. 最终价格使用精确有理数计算：

```text
ceil_once(sum(quantity_i * price_units_i / unit_scale_i) * billing_multiplier_ppm / 1,000,000)
```

所有类别和 attempt 先精确求和、再乘智能体倍率，整个 Run 最终只向上取整一次。禁止逐 item 取整、禁止浮点数。持久化明细使用确定性最大余数法分配最终整数金额，明细合计必须严格等于整单金额。

5. 普通输入、输出、缓存读取、缓存写入和媒体单位分别记录和计价，归一化后的计费分类必须互斥。若供应商的 aggregate input 已包含 cached token，adapter 只能依据该供应商的文档化字段关系拆成“非缓存输入 + 缓存读/写”；不能证明包含关系或拆分后出现负数/重叠时，usage 标为不可用。缓存命中不得再次按普通输入计费。
6. 价格目录无法确定、模型映射不唯一、上游声明的计量能力不能返回所需分类 usage，或单位不受支持时，不调用上游，也不猜测价格。
7. 价格目录版本、rate rows、智能体倍率、effective `max_output_tokens` 和解析后的模型身份在 Run 接受事务中形成不可变配置快照；以后配置变化不重算或改写该 Run。每轮最终出站请求只能在 Worker 组装后确定，因此其精确请求体和上界报价单独保存到对应 provider attempt，不能事后塞回或覆盖 Run 快照。现有 `engine_type=openai` 只表示 OpenAI-compatible 传输，不能当作官方定价厂商。目录按全局 canonical model ID/受审别名解析并记录 `catalog_vendor`，同时另存 `transport_engine`；别名映射不唯一时 fail closed，不要求管理员再维护厂商或价格字段。
8. 管理员不能编辑模型基础价，只能在智能体上设置倍率和该智能体允许的最大输出。

### 2.2 钱包与结算

1. 用户钱包绝不允许负余额，也不允许部分扣款后继续生成。
2. 钱包使用整数 `money_units`，`1 RMB = 100,000,000 money_units`（`1 分 = 1,000,000 units`）；金额计算、冻结和流水禁止 `float`。
3. `available_units = balance_units - held_units`。任何真实上游调用前，必须先冻结足以覆盖该调用最大可能费用的金额。
   `user_wallets.held_units` 是该钱包全部 active Hold 的实时总和：reserve/top-up 必须与 Run Hold 同事务增加，capture/release 必须与 Run Hold 同事务扣减，任何提交后都满足 `user_wallets.held_units = SUM(active wallet_holds.held_units)`。
4. 对话的冻结上界等于最终出站请求的保守输入上界加本次请求的 effective `max_output_tokens` 上界。未显式请求时使用智能体上限；显式值必须为正且不超过智能体上限，超过时拒绝而不是静默截短。报价快照和真实 provider 请求必须使用同一个 effective 值。余额不足时返回充值提示，v1 不静默缩短回答。
5. Gateway 必须在完成最终请求组装后报价。输入上界使用供应商/模型可证明安全的上界估算，覆盖消息包装、系统提示词、知识库内容、工具定义及附件元数据；没有安全上界的模型不可用于付费调用。
6. 图片、视频、音频等非 Token 模态根据最终请求中可确定的张数、时长、分辨率、字符数或次数字段冻结最大费用。
7. 一个 Run 只对应一张账单和一个 Hold。含工具轮次的 Run 在每次新调用前，把冻结总额补到“此前仍可计费的完整实际用量费用 + 本次调用最大费用”；余额不足则绝不发起本次调用。首次 reserve 失败时以 Run `failed`、Charge `released`、reason `released_insufficient_balance` 收尾。后续 top-up 失败时，若此前没有可收费 usage 则使用同一 release reason；若此前 succeeded attempt 的 usage 完整、合法、可定价，则只捕获这些已经发生且已由 Hold 覆盖的实际费用，以 Run `failed`、Charge `settled`、reason `settled_complete_usage` 收尾，绝不为未派发的新 attempt 收费。明确失败的 attempt 即使返回完整 usage，也只保存为审计事实，不进入用户实际扣款。
8. 得到完整、合法、可定价的终态 usage 后，先追加到该 Run 的用量明细；业务成功、用户停止，或后续 top-up 失败但已有可收费 succeeded attempt 时，原子地捕获应收费的整单实际金额并释放剩余冻结。整体上游失败或 unknown 时整单释放，因此不需要 AI 退款。实际金额为零时不创建零金额扣款流水。
9. 没有完整 usage 时，释放冻结且不向用户扣款；由平台承担无法确定的上游成本。不能用冻结额、余额差、累计统计或猜测 Token 数补扣。finalizer 不发布结果候选：用户停止的 Run 保持 `canceled + unbilled`，其他表面成功的调用收尾为 `failed + unbilled`，reason 均为 `unbilled_usage_incomplete`。
10. 完整 usage 算出的累计实际金额若意外超过当前冻结上界，不在调用后补冻结、不透支、不部分扣款；释放冻结、标为未计费、丢弃结果候选并记录平台计费异常。用户停止时业务状态保持 `canceled`，其他调用为 `failed`；账单 reason 为 `unbilled_over_hold`。这说明报价或上游契约有缺陷，不是向用户追款的理由。
11. 不提供 AI 退款。已按完整 usage 结算后，用户删除结果、结果永久丢失或部分媒体损坏都不产生 AI 入账流水。
12. 当前存在但未接入业务的 `SourceAIRefund`、对应字典、服务和测试残留属于本期删除项，不得被新设计复活。
13. 钱包和账单 API 将金额以规范十进制人民币字符串返回：非负、无指数/分组符、最多 8 位小数，去除小数末尾零，整数不带小数点，零固定为 `"0"`（例如 `1 unit -> "0.00000001"`、`500000000 units -> "5"`）；不把 `int64 money_units` 直接作为 JSON number。
14. `payment_orders`、`payment_recharges` 和充值套餐可继续以 cents 保存支付渠道面额；`user_wallets` 与 `wallet_transactions` 只能由 `payment/wallet` 以 units 写入。充值结算在同一外层事务中只做一次 `cents * 1,000,000` 检查换算并调用钱包 participant，禁止根 payment 包保留第二套钱包写模型。

### 2.3 上游、幂等与失败

1. 每个付费用户动作由客户端提供稳定 `request_id`。同一用户、同一请求内容重放时只返回原任务或原终态；同 ID 内容不同返回 `409`，绝不再次调用上游或再次扣款。
2. 每次真实上游调用另有稳定的 provider-attempt idempotency key，并作为 `Idempotency-Key` 传给兼容上游。持久化保存该 key、上游 `request_id` 和原始归一化 usage。
3. 上游在派发前拒绝或网络明确未派发时释放冻结；上游业务失败时也不向用户收费，即使它返回了完整或部分用量。失败 attempt 的原始 usage 仍持久化，但后续重试成功时也不纳入用户实际扣款。
4. 已派发后进程崩溃、网络中断或无法确认结果时，任务转为 `unknown`，释放冻结且不自动创建第二次上游调用。用户可以在界面看到失败/未知状态后用新的用户动作重试。
5. 上游 `request_id` 与 attempt key 保留给管理员按需查询；v1 不对 `unknown` 自动重发或自动对账。任何后续人工排查都不能换 key 盲目重发，平台承担无法证明的成本风险。
6. Sub2API 的聚合余额、倍率或 `/v1/usage` 不能作为单请求扣款证据；只有本次调用的终态 usage 才能结算。
7. 同一个 durable Runner 在发现已派发 attempt 的租约到期且没有终态事实时，只执行一次 `unknown + release` 本地收尾；它不访问上游、不重新派发，也不创建新的恢复子系统。
8. 成功、用户停止，或因后续 top-up 失败而终止的 Run，只有在全部可收费 attempt 的终态 usage 都完整、合法、可定价时才能结算：所有 succeeded attempt 必须完整，用户停止时当前已派发 attempt 也必须完整。failed attempt 永远只作审计，不计费，也不影响其他可收费 attempt 的完整性判断。top-up 失败本身不得创建 attempt；它只决定是否结算此前已经 succeeded 的完整用量。

## 3. `aigateway` 模块边界

### 3.1 所有权

```text
route -> handler -> service -> aigateway -> provider adapter
                                |              |
                                |              +-- base_url + api_key
                                +-- pricing / wallet / run / attempt
```

- `handler` 只处理 HTTP/WebSocket，不访问钱包或供应商。
- 业务服务创建或读取业务任务、消息和 Run，然后调用 Gateway；不自行计算价格或扣款。
- `aigateway` 负责报价、冻结、attempt、供应商调用、usage 归一化和一次终态结算。
- `pricing` 只负责只读目录解析、报价和整数金额计算。
- `wallet` 只拥有余额、冻结、捕获、释放和流水，不知道模型或供应商。
- provider adapter 只处理协议差异，返回规范的 `Usage`、上游请求 ID 和可识别的派发结果。

现有 `route -> handler -> service -> repository -> model` 分层不变。Gateway 通过小接口依赖 `pricing`、`wallet`、Run 和 attempt repository；它不引入跨服务 RPC、第二个数据库、分布式事务或新事件总线。

### 3.2 最小调用协议

一个 Run 的固定顺序是：

```text
接受事务持久化 Run / Charge / 不可变价格配置快照
    -> 对每个真实 attempt：
         组装最终上游请求
         -> 计算“累计实际费用 + 本次最大费用”
         -> 在同一外层事务中按 Run、Charge、wallet、Hold 顺序加锁并确保 Hold 达到该总额
         -> 同一事务末尾创建 prepared attempt，保存精确请求体和本轮上界报价
         -> 标记 attempt 已派发
         -> 调用上游并持久化终态 Usage
    -> 根据整个 Run 的业务终态：
         成功、用户停止，或后续 top-up 失败但已有完整 succeeded usage：原子 capture 应收费实际费用 + release 差额
         上游失败或 unknown：release 全部冻结，不扣用户
```

Run 与 Charge 在用户动作接受事务中创建，Hold 在第一次成功 reserve 时创建。Worker 先用 Run 的不可变价格配置快照组装并报价；reserve/top-up 成功后，才在同一事务末尾持久化 `prepared` attempt 的 exact request body、request hash 与 quote evidence。余额不足、无价格、能力不符或 top-up 失败时不存在新 attempt。已提交的 prepared attempt 是恢复事实：Worker 只能恢复其保存的请求和报价，不能重新组装后覆盖；它在网络调用前才可变为 `dispatched`。首次派发前的报价/能力失败把 Run 原子收尾为 `failed`、Charge 为 `released`、reason 为 `released_before_dispatch`；余额不足使用 `released_insufficient_balance`；用户在派发前停止时 Run 为 `canceled`、reason 为 `released_before_dispatch`。后续 top-up 失败按第 2.2 节只结算此前完整的 succeeded usage。一个含工具轮次的 Run 可以有多个 attempt，但只允许一张 Run 级账单、一条最终 AI 扣款流水和一个可增加上限的 Hold；任何 attempt 都不能提前捕获用户资金。

所有跨模块计费事务的行锁顺序固定为：

```text
Run -> Charge -> wallet -> Hold
```

只涉及钱包和 Hold 的事务使用 `wallet -> Hold`。Gateway 的 reserve/top-up 也必须由外层事务先锁 Run/Charge，再把同一个事务句柄交给 wallet participant 锁 wallet/Hold；禁止 wallet 方法自行开启第二个事务。禁止 Hold 后再锁 wallet；Runner 不得持有 reply command、媒体 task、attempt 或结果候选行锁进入计费事务。业务 participant 在计费锁就绪后通过条件状态更新参与同一提交，状态不匹配时重试 finalizer，不反转锁序。

Gateway 的规范输出至少包含：

- `provider_request_id`；
- `dispatch_state`：未派发、已派发或未知；
- `terminal_state`：成功、用户停止、上游失败或未知；
- 分类 usage、原始来源摘要和可计价标记；
- 用户可见结果或可安全丢弃的结果。

## 4. 聊天停止与媒体取消

### 4.1 聊天停止不是上游取消

当前 OpenAI 兼容客户端已经传递 `stream_options.include_usage=true`，但 Reply Runner 把本地取消信号传给 `StreamChat` 的 HTTP context，导致连接被提前关闭，终态 usage 无法读到。这是 P0 账单缺陷。

新的规则如下：

| 情况 | Gateway 行为 | 钱包结果 | 用户界面 |
| --- | --- | --- | --- |
| 派发前停止 | 不调用上游，释放冻结 | 不收费 | `已停止` |
| 聊天已派发后点停止 | 立即停止向前端发送 delta、丢弃最终答案；后台继续读取同一上游流直到终态 | 有完整 usage 则按实际收费；没有完整 usage 则释放冻结不收费 | 先显示`正在停止`，结算完成后显示`已停止` |
| 上游明确失败 | 关闭调用并释放冻结 | 不收费 | `生成失败` |
| 连接/进程未知 | 不重发，不猜 usage，释放冻结 | 不收费 | `状态未知`或`生成失败` |

实现上，前端 delivery context 和上游 drain context 必须分离。用户停止后，delivery sink 变为丢弃 delta 的 no-op，不能把停止信号作为 sink error 传回 provider adapter；同时 Runner 继续续租，直到 usage 与结算终态落库。用户停止只关闭 delivery，不取消由 Worker/Gateway 拥有的上游读取 context。租约丢失或进程关闭才可中断该 Worker；后续任务恢复不得把未知调用自动重发成新 attempt。

### 4.2 图片、视频和音频

图片、视频等任务型上游在其 API 明确提供取消能力时，Gateway 调用该能力并轮询同一任务的最终状态。只有返回完整权威 usage 时才捕获实际金额；否则释放冻结。没有已文档化的上游取消 API 时，平台只停止本地等待与展示，不伪造“已取消上游”。

媒体任务同样使用 `request_id` 和 provider attempt key；终态结果保存后可重放，用户删除不退款、不重新生成。
成功结果不设置 TTL；结果永久损坏时只返回不可用状态，不退款、不自动重新生成。

## 5. 数据与迁移

阶段 A 只引入完成闭环所需的最小事实：

| 事实 | 责任 |
| --- | --- |
| 版本化官方价格目录、模型别名和计费单位 | `pricing` |
| `ai_agents.billing_multiplier_ppm` 与 `max_output_tokens` | 智能体 |
| Run 的请求指纹、价格快照、结算状态与稳定原因 | `ai_runs` |
| 每次成功冻结后的精确 prepared request、上界 quote、idempotency key、上游请求 ID、归一化 usage | `ai_provider_attempts`，改为 Run 级所有者 |
| 冻结金额、状态和关联 Run/attempt | `wallet_holds` |
| 不可变的实际扣款与分类明细 | `ai_usage_charges` / items |
| 钱包余额、冻结和资金流水 | `user_wallets` / `wallet_transactions` |

`ai_runs` 的业务状态和账单状态分开表达。例如聊天停止且终态 usage 完整时，业务状态为 `canceled`，账单状态仍可为 `settled`。不存在“取消即免费”的隐式规则。

### 5.1 `money_units` 迁移

当前钱包以分保存，需要在阶段 A 的维护窗口迁移到 `money_units`。实施计划必须单独列出该迁移，但不把它扩展成一套通用金融系统：

1. 应用启动时不自动迁移；停止全部钱包写入者后执行版本化迁移。
2. 对钱包和流水同时新增并回填 units 字段，换算固定为 `cents * 1,000,000`，先做 `int64` 溢出和非负前置校验。
3. 回填后逐行校验余额、流水前后余额和汇总守恒；校验通过后，新二进制只读写 units。
4. 不做 cents/units 双写，也不允许旧、新写入者并行。确认稳定后再在单独 contract migration 删除 cents 字段。
5. 按现有部署政策不创建备份数据库；因此任何前置校验失败都必须在写入 units 前停止迁移。

## 6. API、界面与权限

### 6.1 阶段 A

- AI 发起接口要求客户端 `request_id`，余额不足返回稳定的充值错误和钱包入口。
- 仍在 HTTP 中等待同一 durable task 结果的同步入口，余额不足固定返回 HTTP `409`、`error.code = ai.billing.insufficient_balance`，并在 `data` 中提供 `wallet_path=/profile/wallet` 与 `recharge_path=/payment/recharge`。聊天、编辑和重新生成在报价前已经按契约返回 HTTP `202`，不能事后伪造 HTTP `409`；Worker 必须以 durable `ai.response.failed.v1` 返回同一 `error_code` 和两个路径，且上游零调用。前端只能按 machine code 分支，不能匹配本地化 `msg`。
- `ai.response.failed.v1` 的 payload 在现有 `conversation_id`、`request_id`、`msg` 基础上增加必填 `error_code` 与可空 `wallet_path`、`recharge_path`；只有余额不足时两个路径非空，其他失败显式为 `null`。HTTP 等待中断只停止等待，不能取消 Worker；相同 `request_id` 重放读取同一任务/终态。
- 聊天停止接口只确认 durable stop intent，成功响应固定为 `status="stopping"`，不能沿用会误报终态的 `status="canceled"`。只有 finalizer 已在同一提交中完成 Run/Charge/Hold 收尾后，才能追加 durable `ai.response.canceled.v1`；前端收到该终态事件后才显示“已停止”。
- 钱包资金明细显示 AI 消费金额、关联 Run 和可读的模型/智能体摘要；兑换码充值继续作为独立入账来源。
- 智能体配置只允许受权管理员修改倍率和最大输出配置；模型官方基础价只读展示。
- Run 详情显示价格快照、分类 usage、冻结/实际扣款、provider request ID（管理员可见）和终态原因。
- 任何新增 Admin 权限只注册定义，不自动写入 `role_permissions`；由管理员手动授予。

阶段 A 的 `AIRunDetail` 在保留现有字段基础上增加以下闭合字段，前端不得解析持久化 JSON 猜账单：

| 字段 | 类型与语义 |
| --- | --- |
| `billing_status` | `pending|held|settled|released|unbilled` |
| `billing_reason` | `pending|held|settled_complete_usage|released_before_dispatch|released_insufficient_balance|released_provider_failed|released_outcome_unknown|unbilled_usage_incomplete|unbilled_over_hold|legacy_unpriced` |
| `held_amount` / `actual_amount` | 十进制人民币字符串；`held_amount` 来自 Charge，表示本 Run 曾成功冻结的最大金额，终态 capture/release 后不随活动 Hold 归零 |
| `pricing` | 闭合对象：`version`、`catalog_vendor`、`transport_engine`、`model_id`、`resolved_alias`、`billing_multiplier`、`max_output_tokens`、`rates` |
| `usage_items` | 闭合数组：`attempt_no`、`category`、`tier_key`、`quantity`、`unit`、`unit_price`、`unit_scale`、`amount`、`billable` |
| `provider_attempts` | 闭合数组：`attempt_no`、`state`、可空 `provider_request_id`、`usage_status` |

`pricing.rates` 的每项为 `category`、`tier_key`、`unit`、十进制人民币字符串 `price` 和整数 `unit_scale`。`usage_items.amount` 是分摊后的人民币字符串；failed attempt 的审计 usage 可展示但 `billable=false` 且不进入实际金额。provider API key、请求 Authorization、完整私密 header 和密钥提示不得进入该响应。

### 6.2 阶段 B

阶段 B 必须使用阶段 A 的 `(user_id, request_id)`、Run、durable command 和停止/结算终态，不得为编辑、重新生成或点赞各自直连供应商或钱包。

#### 6.2.1 消息选择与软删除

1. 用户可从任意当前可见消息进入选择模式，默认选中该消息及其问答配对消息；用户可取消任一条，也可跨多个问答对选择。
2. 允许删除完整问答对、只删用户消息、只删 AI 回复或批量删除任意组合。
3. 问答配对只依据 `ai_reply_commands.user_message_id` 与 `assistant_message_id`，不得按相邻位置猜测。
4. 删除只更新 `ai_messages.is_del`；消息、回复命令、Run、usage、Charge、Hold 和资金流水不得物理删除或级联修改。
5. 删除后，后续模型上下文只读取仍然可见的消息。删除已结算消息不退款、不改账单。

#### 6.2.2 编辑用户文字

1. 只有当前可见的用户消息可编辑，本期只修改文字。
2. 原消息的附件和运行参数必须从服务端原事实完整继承；前端不得提交附件替换。
3. 提交后从源用户消息开始切断事务开始时的当前可见尾部，创建新用户消息、新 durable reply command 和新 Run，再异步生成回复。
4. 新消息只替换文字；旧消息、旧 Run、旧账单和审计永久保留，但退出当前可见链。
5. 编辑是新的付费用户动作，必须提供新的稳定 `request_id`；同一动作的网络重放复用原 ID。

#### 6.2.3 重新生成

1. 只有已完成的 AI 回复显示重新生成入口，且其配对用户消息仍须处于当前可见链。
2. 服务端复制配对用户消息的文字、附件和运行参数，创建新用户消息、新 reply command 和新 Run；不得复用旧 Run 或旧账单。
3. 当前可见链只保留新问答版本，旧问答退出可见链但永久保留审计。本期不提供历史答案版本切换。
4. 任一配对消息已删除时返回 404，不能通过重新生成恢复用户已删除的问题。
5. 重新生成是新的付费用户动作，使用新的稳定 `request_id`，并完整走阶段 A 报价、Hold、Gateway 和结算路径。

#### 6.2.4 Run 点赞

1. 只有成功并已绑定 AI 消息的聊天 Run 可点赞；再次设置 `liked=false` 即取消。
2. 点赞持久化到 `ai_runs.liked_at`，不写入 `ai_messages.meta_json`，也不增加点赞计数或独立点赞表。
3. 删除聊天消息不清除 Run 点赞。Run 详情显示 `liked` 与 `liked_at`，列表本期不增加点赞列。
4. 点赞是当前登录用户的 self-service：路由只要求 `Authenticated()`，Service 必须校验 Run 所有权；它不要求后台管理权限 `ai_run_list`。

#### 6.2.5 免费浏览器朗读

1. 只有 AI 回复显示朗读入口，使用 `window.speechSynthesis` 与 `SpeechSynthesisUtterance`。
2. 优先选择 Google 中文音色，其次其他 `zh-CN` 系统音色，最后使用浏览器默认音色。
3. 同时只朗读一条，支持开始、暂停/继续和停止；切换会话、离开页面或朗读另一条时停止当前朗读。
4. 浏览器不支持时禁用入口并显示简短提示，不调用后端 TTS，不创建 Run，不产生费用，也不回退到网络 TTS。

#### 6.2.6 服务端权威未读数

1. 只统计成功落库、仍可见且 ID 大于会话已读游标的 AI 回复；流式分片不计数。
2. failed、canceled、timeout 和终态 `outcome_unknown` 不计数。
3. 非当前会话完成回复时，前端刷新服务端会话列表；当前会话完成消息恢复并确认可见后，推进最新可见 AI 消息的已读游标。
4. 删除未读 AI 消息后，该消息不再计数。WebSocket 只负责到达通知，不作为计数真相源。

#### 6.2.7 Run 输入快照

Run 详情不再直接展示嵌套转义 JSON。前端结构化展示用户文字、附件名称/类型/大小/URL、图片缩略图和运行参数。历史纯文本直接展示；解析失败安全回退原文，不得使用 `v-html`，也不得改写历史 `ai_runs.input_snapshot`。

#### 6.2.8 充值页精简

删除收银台 PageInit 与 UI 的整个 `recent` 区域、专用查询、组件和样式；“充值记录”Tab、分页列表、继续支付和对应 API 保持不变。

## 7. 阶段 B 架构

### 7.1 保持线性可见对话链

阶段 B 沿用当前模块化单体和 `route -> handler -> service -> repository -> model`：

```text
旧可见前缀 + 被替换消息及尾部
               -> 软删除并保留审计

旧可见前缀 + 新用户消息 + 新 reply command + 新 AI 回复
```

`ai_reply_commands.user_message_id` 的一对一约束保留。编辑与重新生成都复制出一条新用户消息，因此新命令不会抢占旧命令的用户消息身份。旧 ID 继续供旧 Run、账单和审计引用，不创建消息版本树。

### 7.2 WebSocket 只负责到达通知

```text
回复完成事务
  -> assistant message 落库
  -> Run 绑定 assistant_message_id
  -> 追加 durable ai.response.completed.v1

前端收到事件
  -> 当前会话：恢复消息后推进 read cursor
  -> 非当前会话：刷新会话列表中的 unread_count

断线或事件过期
  -> 现有 realtime recovery
  -> 重新读取会话列表与消息
```

不新增第二套 WebSocket、未读事件或通用事件总线。重复事件、页面刷新、多标签页和 `realtime.resync_required` 都以服务端查询结果恢复。

### 7.3 现有事实复用

- `ai_messages.is_del` 表达当前可见链；
- reply command 的 `user_message_id`/`assistant_message_id` 表达问答配对；
- `ai_runs` 已关联会话与消息，并由阶段 A 承载请求指纹和结算事实；
- 上下文查询继续只读取 `is_del=2` 的消息；
- 点赞归 Run，朗读只存在于浏览器运行态。

## 8. 阶段 B 数据模型

### 8.1 `ai_conversations`

新增 `last_read_message_id BIGINT UNSIGNED NOT NULL DEFAULT 0`，不增加外键，避免与消息形成循环关系。Service 必须验证目标是当前用户、当前会话内成功落库的 AI 消息，并以 `GREATEST(existing, incoming)` 单调推进，禁止回退。

### 8.2 `ai_messages`

不新增状态字段，继续使用 `is_del=2` 表示可见、`is_del=1` 表示隐藏但保留审计。新增组合索引：

```text
(conversation_id, is_del, role, id)
```

该索引服务可见消息读取和读游标之后的未读计数。

### 8.3 `ai_runs`

新增 `liked_at DATETIME(6) NULL`。非空表示当前 Run 被其所有者点赞；取消点赞置空。本期不保存每次切换历史，不预留点踩枚举。

### 8.4 不新增表

不创建消息版本表、点赞表或未读明细表。阶段 B 使用现有软删除、一条会话读游标和 Run 点赞时间即可表达需求。

## 9. 阶段 B 服务端契约

所有接口使用当前登录身份，不接受 `user_id`。除后台 Run 管理读取外，本节消费者接口均为 `Authenticated()` + Service 所有权校验。

### 9.1 消息和会话响应

每条消息增加：

| 字段 | 语义 |
| --- | --- |
| `paired_message_id` | 当前可见问答对的另一条消息 ID，无配对为 `null` |
| `run_id` | AI 回复对应的 Run ID；用户消息为 `null` |
| `liked` | AI 回复对应 Run 是否点赞；用户消息固定为 `false` |

配对和 Run 状态必须批量查询/映射，禁止逐消息 N+1。会话列表增加服务端计算的 `unread_count`：

```text
conversation_id = 当前会话
AND role = assistant
AND is_del = 2
AND id > last_read_message_id
```

### 9.2 编辑消息

```text
POST /api/admin/v1/ai-conversations/:id/messages/:message_id/revisions
```

```json
{"content":"修改后的文字","request_id":"客户端稳定请求 ID"}
```

服务端在一个事务内锁定并验证会话所有权，按 canonical `(user_id, request_id)` 查询幂等事实并比较完整请求指纹，拒绝活动命令，验证源消息为当前可见用户消息，记录当前可见尾部上界，软删除源消息到该上界，复制原 `meta_json` 创建新用户消息，只替换文字，再创建新 command/Run 并更新 `last_message_at`。事务提交后才唤醒现有 Runner。

相同 ID、相同指纹返回第一次的 HTTP `202` 身份，不重复切尾或派发；相同 ID、不同指纹返回 `409`。指纹包含用户、operation、conversation、源消息、规范化文字、继承附件/参数和智能体选择。

成功响应复用阶段 A 的闭合 `AIMessageSendResult`：`conversation_id`、`user_message_id`、`command_id`、`request_id`、`state`；不增加第二套相似 accepted DTO。

### 9.3 重新生成

```text
POST /api/admin/v1/ai-conversations/:id/messages/:message_id/regenerations
```

请求只包含 `request_id`。服务端通过目标 AI 消息的 reply command 找到当前可见配对用户消息，复制文字与完整 `meta_json`，执行同一线性替换事务并创建新 command/Run。canonical 幂等规则与编辑一致，HTTP `202` 响应同样使用 `AIMessageSendResult`。

### 9.4 批量软删除

```text
DELETE /api/admin/v1/ai-conversations/:id/messages
```

请求体只包含正整数 `ids`。事务验证所有权，拒绝空集合、重复 ID、跨会话 ID、不可见/不存在消息和活动 command，只软删除明确提交的 ID，不自动扩展配对，并按剩余可见消息重算 `last_message_at`。成功 `data` 精确为 `{"deleted_ids":[...]}`，保持服务端规范化后的升序唯一 ID。该事务不得读取或修改钱包/账单。

### 9.5 推进已读游标

```text
PUT /api/admin/v1/ai-conversations/:id/read-cursor
```

请求包含 `message_id`。目标必须是该会话中已成功落库且当前可见的 AI 消息；更新单调、重复请求幂等。成功 `data` 精确包含 `conversation_id`、实际持久化的 `last_read_message_id` 和重新查询得到的 `unread_count`。

### 9.6 Run 点赞

```text
PUT /api/admin/v1/ai-runs/:id/user-feedback
{"liked":true}
```

路由使用 `Authenticated()`，明确不挂 `ai_run_list`。Service 验证 Run 属于当前用户、属于聊天会话、状态为 `success` 且已绑定 AI 消息。`PUT` 幂等设置 `liked_at`，不做计数累加；成功 `data` 精确为 `id`、`liked`、可空 `liked_at`。后台 Run 详情继续由 `ai_run_list` 保护并展示 `liked/liked_at`。

### 9.7 充值 PageInit

`GET /api/admin/v1/payment/recharges/page-init` 删除 `recent` 字段和 `ListRecentRecharges` 查询。充值记录列表接口及其分页、继续支付契约不变。

## 10. 并发与状态

以下 command 状态视为活动状态：

```text
pending | claimed | running
```

`cancel_requested_at` 非空但尚未终态的 command 仍按其当前 `pending|claimed|running` 状态视为活动；`outcome_unknown` 已是终态，不属于活动集合。

存在活动 command 时，Service 与前端都禁止编辑、重新生成、单条删除和批量删除。用户必须先停止并等待 command 进入 `succeeded|failed|canceled|timed_out|outcome_unknown` 终态。前端禁用只是体验层，事务内必须再次检查以拒绝竞态。

编辑/重新生成事务一旦提交，可见历史替换即生效。后续报价失败、余额不足、provider 失败或取消时，新用户消息保留，旧尾部不恢复；用户可用新的用户动作再次编辑。任何历史操作、删除或点赞都不触发退款或账单重算。

## 11. 阶段 B 前端交互

### 11.1 消息操作与选择模式

- AI 回复保留复制，新增朗读、点赞、重新生成；用户消息保留复制，新增编辑和删除。
- 桌面端在 hover 或键盘 focus 时显示操作；触摸设备必须有可点击入口，不能只依赖 hover。
- 使用现有图标库、tooltip、`aria-label` 和固定按钮尺寸。
- 选择模式为每条消息显示复选框，默认选中触发消息与 `paired_message_id`，用户可任意调整。
- 底部固定操作条显示选择数量和删除按钮；空选择禁用，提交前使用现有确认组件。

### 11.2 编辑与重新生成

- 编辑在原用户消息位置进入紧凑文字编辑态，不使用后台式大弹窗；附件只读展示。
- 提交后显示新用户消息和异步回复占位；重新生成也进入同一 durable session 状态。
- 每次新动作创建新 `request_id`；同一未确认响应的传输重试复用原 ID。
- 页面切换不丢失未完成回复，继续由会话 session store 和 durable events 恢复。

### 11.3 未读与朗读

- 会话项右侧显示固定尺寸的数量徽标，`0` 不渲染，标题不因数字出现而跳动或溢出。
- 非当前会话完成事件只触发权威列表刷新；进入会话并完成消息恢复后再推进读游标。
- 朗读状态只能有一个 owner；组件卸载、会话切换和新朗读开始时统一清理。

### 11.4 输入快照与充值页

输入快照解析器依次尝试外层 JSON、字符串类型 `meta_json` 的严格二次 JSON，再映射 `content`、`attachments`、`runtime_params`；任一步失败都回退原始文本。附件预览必须复用 `src/lib/browser/download.ts` 的同源或 HTTPS 白名单 URL 校验（拒绝凭据、非白名单外域和非法协议），不能另写宽松的 `startsWith('http')` 判断；未通过校验的 URL 只显示转义文本。

充值页删除最近充值组件、状态、样式和读取；收银台主体与充值记录 Tab 保持现有 ToC 交互。

## 12. 错误、安全与审计

- 活动生成冲突返回 HTTP `409` 和稳定 machine code，提示用户先等待停止完成；
- 源消息已删除、角色错误、配对不可见或不属于当前会话统一返回 404，不泄露其他用户资源存在性；
- 空编辑文字和非法 ID 返回 400；canonical request fingerprint 冲突返回 409；
- 点赞失败恢复按钮原状态，不覆盖消息；朗读不可用只影响朗读入口；快照解析失败只回退原文；
- 所有消息变更、读游标和点赞使用当前登录身份并在 Service 校验所有权；
- 不接受前端传入用户 ID、配对 ID、Run 归属、附件替换或历史运行参数覆盖；
- 操作日志只记录资源 ID、`request_id` 和动作，不记录完整提示词、附件 URL 或回复正文；
- 软删除不得级联修改 Run、command、event、usage、Charge、Hold 或资金流水；
- 浏览器朗读文本不发送到新服务，不使用非正式 Google TTS 网络接口。

## 13. 明确不做

- 不拆分独立 AI Gateway 微服务，不复制 Sub2API；
- 不新增计费专用密钥，不修改 `APP_SECRET` 或 `APP_SECRET_PREVIOUS`；
- 不允许负余额、透支、估算补扣或 AI 退款；
- 不做汇率、供应商倍率、供应商模型倍率、会员套餐或优惠计费；
- 不以聚合余额/统计接口作为逐请求结算依据；
- 不建立长期自动对账、跨实例计费封禁、无限冻结或复杂结果恢复平台；
- 不在 usage 缺失时收费，也不在 unknown 后自动盲目重发；
- 不实现推荐追问、固定推荐模板、点踩、分享、收藏或历史答案版本切换；
- 不实现后端付费 TTS、Google Translate 非正式 TTS、消息物理删除或第二套实时通道；
- 不删除充值记录 Tab/API，不改写历史 `ai_runs.input_snapshot`；
- 阶段 A 不混入阶段 B UI，阶段 B 不重做阶段 A 钱包与 Gateway。

## 14. 验收与验证边界

### 14.1 阶段 A

1. 余额不足、未定价或无安全报价上界时，上游零调用；同步等待入口返回稳定 HTTP 充值契约，已接受的 durable chat 动作返回同 machine code 的 durable 失败事件。
2. 任一实际调用均在 `dispatched` 前完成冻结；锁序一致且任何并发下 `balance_units - held_units >= 0`。
3. 所有 usage 精确求和并乘 PPM 后整单只向上取整一次；明细分摊合计严格等于总额。
4. Run 的全部可收费 attempt usage 完整时只结算一次；failed attempt 只审计；缺失、整体上游失败或 unknown 不扣用户。后续 top-up 失败不得派发新 attempt，但必须结算此前完整的 succeeded usage。
5. 实际金额超过 Hold 时不透支、不部分扣款，释放并留下平台异常事实。
6. 缓存 Token 与普通输入在归一化后互斥且不重复扣费；无法证明 aggregate/subset 关系时不猜价。
7. 相同 canonical `(user_id, request_id)` 只返回原任务/Run；不同指纹复用返回 409。
8. 聊天停止后不再发送 delta，但 drain 读完同一流；完整 usage 收费且正文不发布。
9. 图片/视频只按文档化取消能力和权威终态 usage 结算；音频能力缺失时派发前拒绝，派发后 usage 缺失则释放。
10. `money_units` 迁移保持守恒；支付充值只在外层事务转换一次，钱包/流水无 cents 写入者。
11. 已结算结果没有退款路径，`SourceAIRefund` 残留被删除。

### 14.2 阶段 B

1. 编辑只改变文字，附件和参数继承；编辑/重新生成创建新消息、command 和 Run，旧审计不变。
2. 默认问答配对选择准确，也可只删除任一消息；删除不修改账单或点赞。
3. 活动 command 期间历史操作在前后端都被阻止；终态 `outcome_unknown` 不继续阻塞。
4. Run 点赞可幂等切换，所有权受检验，self-service 不依赖 `ai_run_list`。
5. 朗读完全在浏览器执行，不调用后端、不计费并能正确清理状态。
6. 非当前会话完成后未读数准确，进入后清零，删除与断线恢复不漂移。
7. Run 输入快照结构化展示，历史异常数据安全回退。
8. 收银台不再查询/展示最近充值，充值记录和继续支付不受影响。

### 14.3 执行预算

Codex/实施 Agent 只自动运行当前改动直接相关、预计两分钟内完成的格式、静态检查和小型定向测试。不自动运行 Docker、全仓后端测试、race 全套、前端全量测试、完整 build、Playwright 全套或端到端 smoke。长验证只列命令和预期，由用户决定是否手动执行：

```powershell
go test ./...
go test -race ./...
Set-Location ..\admin_front_ts
npm test
npm run build
# docker compose / Playwright / 真实上游验收按对应计划手工执行
```

## 15. 执行入口

- 阶段 A：`../plans/2026-07-25-ai-chat-consumer-pricing-wallet-phase-a-execution-index.md`
- 阶段 B：`../plans/2026-07-25-ai-chat-consumer-interactions-phase-b-execution-index.md`

先按阶段 A Wave 0-3 完成 Schema、钱包/定价/Gateway、聊天/媒体和契约装配。阶段 A 接口稳定后，再按阶段 B 的 Schema、并行后端能力、契约发布和并行前端波次实施。两个索引都只引用本文作为业务规范；历史 review 和 Git 中旧交互 Spec 只能用于追溯，不能覆盖本文。
