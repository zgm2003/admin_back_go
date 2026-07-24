# AI 消费者对话交互、官方数值定价与钱包结算设计

**日期：** 2026-07-24
**状态：** 待联合书面复核
**取代：** `2026-07-24-ai-official-pricing-wallet-settlement-design.md` 与 `2026-07-24-ai-chat-consumer-interactions-design.md`

## 1. 目的

项目已经具备消费者式 AI 对话、持久化异步回复、AI Runs、充值和钱包入账，但消息级交互仍不完整，AI 调用也尚未形成钱包消费闭环。本设计统一解决两类本来不可分割的问题：

1. 补齐消息选择、删除、编辑、重新生成、点赞、免费朗读、异步未读数、运行输入快照和充值页精简；
2. 为对话、文本、图片、视频、音频和智能体生成接入官方数值定价、智能体倍率、分类用量、资金冻结和精确结算。

编辑和重新生成都会创建新的回复命令与 AI Run；停止、失败、流式完成和未读到达都受结算终态影响。因此交互与计费必须共享一条明确的运行生命周期，不能继续作为互不相关的两份事实。

核心计费公式只有一条：

```text
最终费用
  = Σ（模型官方数值人民币基准价 × 实际分类用量）
  × 智能体消费倍率
```

例如，厂商官方价格为 `$5 / 1M input tokens`，平台基础价直接采用 `¥5 / 1M input tokens`。只保留官方价格数值并将平台结算币种定义为人民币，不进行美元兑人民币换算，也不增加汇率配置。

## 2. 已确认的业务规则

### 2.1 定价与资金

1. 模型基础价格来自厂商官方公开价格。
2. 官方价格数值直接作为人民币价格：`$5 / M Token -> ¥5 / M Token`。
3. 该规则命名为 `official_numeric_parity_v1`，不能描述成“官方人民币汇率换算价”。
4. 管理员不能新增、编辑或覆盖模型基础价格。
5. 管理员只在智能体上配置一个消费倍率。
6. 智能体倍率默认 `1.0`，必须大于零。
7. 供应商和供应商模型不保存消费倍率。
8. 模型成本差异由官方基础价表达；智能体产品售价差异由智能体倍率表达。
9. 智能体更换模型后自动采用新模型的官方基础价，智能体倍率保持不变。
10. 价格目录或倍率更新只影响之后的新运行，历史账单永不重算。
11. 普通输入、输出、缓存写入和缓存读取分别记录、分别计价。
12. 缓存命中的 Token 不能再次按普通输入 Token 计费。
13. 官方区分 5 分钟、1 小时等缓存写入价格时，分别记录对应 TTL 档位。
14. 图片、视频和音频遵循各模型官方计费单位，可以是 Token、张、秒、分钟、字符或次。
15. 价格缺失、规格不匹配或用量口径不完整时，既不能按零元静默放行，也不能猜高价扣用户。
16. 钱包可用余额不能因 AI 结算变成负数。
17. 冻结只是预占可用余额，不是消费；只按最终真实用量扣款并释放差额。
18. 所有付费入口都由客户端提供稳定 `request_id`；同一身份只能重放首次任务或结果，不能再次调用供应商或再次扣款。
19. 正常状态下，已经交付或已经扣款的生成结果必须可由首次请求身份重放。本期不自动清理成功任务、成功结果正文或媒体对象；若平台永久损坏已扣款结果，则完整退款并稳定报错，绝不能借原请求再次生成或扣款。
20. 供应商结果不确定、平台 finalizer 暂时失败或媒体对象处于候选状态时，都必须有持久化截止时间和有界恢复，不能无限冻结用户余额。

### 2.2 消息与运行

1. 消息删除使用 `ai_messages.is_del` 软删除，不物理删除消息、回复命令、AI Run 或财务记录。
2. 用户可默认按问答对选择消息，也可独立取消任一条并批量删除任意可见消息。
3. 问答配对依据持久化回复命令关系，不通过消息相邻位置猜测。
4. 编辑只修改用户文字，原附件和运行参数原样继承。
5. 编辑从目标用户消息开始替换当前可见尾部，并创建新用户消息、新回复命令和新 Run。
6. 重新生成复制目标回复对应的用户文字、附件和运行参数，也创建新用户消息、新回复命令和新 Run。
7. 编辑和重新生成后的新 Run 使用执行时当前智能体模型、当前官方价格与当前智能体倍率；不复用旧 Run 的计费快照。
8. 旧消息、旧 Run、旧账单和审计事实永久保留，但退出当前可见对话链。
9. 本期保持一条线性可见对话链，不提供历史答案分支切换。
10. 仅成功 AI 回复可点赞；点赞归属具体 `ai_run`，删除消息不清除点赞。
11. 朗读只使用浏览器 `speechSynthesis`，不创建 Run、不调用后端 TTS、不计费。
12. 未读数只统计成功落库且当前可见的 AI 回复；流式分片、失败、取消、超时和结果不确定均不计数。
13. 成功回复无论最终账单是 `settled` 还是因平台计量异常而 `unbilled`，都属于可见回复并按正常规则计入未读。

### 2.3 执行与验证预算

1. Codex 只自动运行与当前改动直接相关、预计单条两分钟内完成的针对性检查。
2. 全仓测试、race、全量前端 coverage/build、Docker 和完整 release gate 只列出命令，由用户决定是否手动运行。
3. 未经用户在当前对话明确授权，不启动预计超过两分钟的验证。

## 3. 当前项目事实与架构约束

### 3.1 模块化单体边界

后端继续遵守：

```text
route -> handler -> service -> repository -> model
```

- handler 只处理传输协议，不直接访问数据库、Redis、计费表或钱包表；
- service 负责业务规则和事务编排，不依赖 `gin.Context`；
- repository 负责查询、锁和持久化，不决定产品业务规则；
- 平台差异留在 `transport/admin`、presenter 或 workflow，不污染通用能力模块；
- 当前对外仍是 `/api/admin/v1` Admin Platform 契约，但 AI 对话页面按 ToC 消费者交互设计，不做传统后台表单式体验。

### 3.2 已有 AI 事实

- `ai_agents` 已选择一个供应商和模型并保存支持场景。
- `ai_provider_models` 只有供应商模型 ID、展示名和状态，没有定价身份或商业倍率。
- `ai_conversations` 当前由一个用户拥有，并固定关联一个智能体。
- `ai_messages` 已有 `is_del`、`meta_json` 和 `reply_command_id`。
- `ai_reply_commands` 持久化异步回复，`user_message_id` 保持一对一唯一约束。
- 回复命令状态包含 `pending`、`claimed`、`running`、`succeeded`、`failed`、`canceled`、`outcome_unknown`、`timed_out`。
- `ai_provider_attempts` 已为聊天回复命令保存供应商尝试身份和结果不确定状态，但目前仍以 `command_id` 为所有者，不能直接覆盖文本、图片、视频、音频和工具草稿生成。
- `ai_runs` 已保存用户、智能体、供应商、模型、请求幂等身份、输入快照、运行状态和 Token 汇总。
- 一次对话运行可能因工具调用产生多次真实模型请求。
- 同步文本虽先创建 `ai_text_tasks` 并持久化答案，但入口没有客户端 `request_id`；客户端超时重试会创建第二个任务和 Run。
- 图片和视频已有稳定任务 ID，能够约束取得 ID 之后的 Worker/轮询重试，但创建入口没有客户端 `request_id`，无法约束“任务已创建但响应丢失”后的重提。
- 音频当前用时间戳生成 Run 请求 ID，只在内存返回 `[]byte`，没有可恢复的任务身份或结果引用。
- `agent_generate` 工具草稿生成会同步调用模型，但当前没有 Run、任务结果或客户端幂等身份。
- 图片、视频和音频的旧 Admin/Canvas 交互 transport 已退役；保留的是 transport-neutral 业务能力，本设计不能借计费重新暴露退役路由。
- 当前消息上下文只读取 `is_del=2` 的可见消息。
- `ai.response.completed.v1` 已是持久化 WebSocket 事件；前端已有去重、序列游标、断线重放和权威恢复。
- WebSocket delta 不进入 `ai_run_events`，最终助手正文进入 `ai_messages`。

### 3.3 已有支付与发布事实

- `user_wallets` 与 `wallet_transactions` 当前以人民币“分”的整数保存金额。
- 钱包扣款与入账已有行锁、余额不足拒绝和 `(source_type, source_id)` 幂等约束。
- `SourceAIGenerate` 与 `SourceAIRefund` 已存在，但 AI 运行时尚未使用。
- 旧 `ai_billing_rules`、`ai_billing_records` 已被有意删除，必须继续保持不存在。
- 应用启动过程不能执行数据库迁移。
- 可以新增权限定义，但不能自动授权，也不能直接写 `role_permissions`。
- Admin Contract Bundle 必须从编译后的路由与 schema 重新生成，前端不能手写漂移契约。

## 4. 范围

### 4.1 本期包含

- 版本化、只读、可审计的官方数值价格目录；
- 供应商模型到规范价格模型的确定性解析；
- 智能体消费倍率；
- 普通输入、输出、缓存读写及媒体单位的统一用量模型；
- 支持低于一分钱消费的人民币高精度钱包；
- 冻结、补充冻结、扣款、释放、退款与异常恢复；
- 不可变的 AI 用量账单及计费明细；
- 所有由钱包用户发起的 AI 生成链路；
- 所有可计费入口的稳定业务幂等身份、请求指纹、供应商派发事实和结果重放语义；
- 消息选择与批量软删除、用户消息编辑、AI 回复重新生成；
- Run 点赞、浏览器免费朗读、服务端权威未读数；
- AI Runs 结构化输入快照、价格与费用详情；
- 智能体价格预览、只读价格目录和高精度钱包流水；
- 充值收银台移除“最近充值”区域；
- Atlas schema、版本化迁移、迁移校验、Admin Contract Bundle、架构文档和针对性测试。

### 4.2 本期不包含

- 汇率或汇率同步；
- 管理员自定义模型单价或供应商渠道价格覆盖；
- 全局倍率、供应商倍率或供应商模型倍率；
- 会员、订阅、优惠券、套餐额度、促销赠送、成本利润和税务发票；
- 运行时抓取厂商价格网页或直接信任社区价格源；
- 对连接测试、健康检查或模型列表同步收费；
- 模型生成的推荐追问或固定推荐模板；
- 点踩、分享、收藏和历史答案版本切换；
- 付费后端 TTS 或非正式 Google Translate TTS；
- 通用事件总线、独立聊天服务或消息版本树；
- 物理删除消息、Run、回复命令、账单或钱包事实；
- 第二套 WebSocket 或新增未读专用事件；
- 改写历史 `ai_runs.input_snapshot`；
- 删除充值记录 Tab、充值记录列表或继续支付能力；
- 自动给角色授权或复活旧 AI 计费权限；
- 新增计费专用密钥、修改 `APP_SECRET` / `APP_SECRET_PREVIOUS` 或增加本需求专用部署环境变量；目录、倍率和恢复策略都是版本化非秘密事实；
- 复活已退役的 Admin 图片/资产页面、Canvas API 或任何已删除 transport；未来消费者 transport 只能复用本设计后的中立 service 契约。

## 5. 选定架构

### 5.1 中央价格目录 + 智能体倍率

官方价格事实只保存一份，智能体保存面向用户的消费策略。同一模型可被多个智能体复用并采用不同倍率；智能体切换模型时只替换基础价。

价格目录采用仓库内审核并随版本发布的文件，不直接依赖运行时社区价格源。`sub2api` 的解析、回退和多计费单位设计可作参考，但其 LiteLLM 兼容目录不是厂商官方 API，不能未经审核成为本项目资金事实源。

### 5.2 持久化回复命令 + 线性可见对话链

初次发送、编辑和重新生成都先在一个短数据库事务中写入用户消息与 `ai_reply_command`，提交后由现有 Worker 执行。编辑和重新生成不修改原消息：

```text
旧可见前缀 + 被替换问答 + 后续消息
               |
               +-- 软删除，继续保留审计、Run 和财务事实

旧可见前缀 + 新用户消息 + 新回复命令 + 新 AI 回复
```

这样保留 `ai_reply_commands.user_message_id` 一对一约束，也避免旧 Run 看起来像由新提示词产生。

### 5.3 执行端冻结，供应商调用前生效

对持久化对话命令而言，HTTP 接收事务不冻结钱包。最终序列化的供应商请求、知识上下文、工具轮次上限、模型快照和保守报价只能由 Worker 在执行时确定；把冻结塞进对话 HTTP 接收事务会复制请求组装逻辑并产生过期快照。

平台统一不变量是：真正执行供应商调用的一端在取得稳定业务任务和 Run、组装最终请求后、发送请求前完成权威冻结。对话和异步媒体任务由 Worker 执行；同步文本、音频或工具草稿链路由当前请求内的业务 service 执行。可选预检只能改善提示，不能代替执行端的价格、计量能力和可用余额校验。

所有可计费入口都必须先接收客户端 `request_id`、计算服务端请求指纹并持久化业务任务，再创建 Run、冻结和派发供应商请求。服务端自增 task ID 只能作为 task 创建成功后的内部身份，不能代替客户端请求 ID。相同请求重放返回首次任务或首次结果；相同 ID 携带不同输入返回 HTTP 409。

### 5.4 WebSocket 只负责到达与流式体验

WebSocket 不是未读、钱包或完成状态的真相源：

```text
冻结成功
  -> ai.response.start.v1
  -> 非持久化 delta

最终完成事务
  -> 账单结算或 unbilled 释放
  -> 写入 assistant message
  -> 完成 reply command 与 ai_run
  -> 追加 durable ai.response.completed.v1

客户端收到完成事件
  -> 当前会话：恢复消息后推进已读游标
  -> 非当前会话：刷新会话列表中的服务端 unread_count
```

余额不足或定价不可用时不会先发 start/delta；只发持久化失败结果。重复事件、页面刷新、多标签页和断线重连都以 HTTP 权威查询恢复。

### 5.5 模块所有权

- `pricing`：加载并校验价格目录，解析规范模型，生成保守报价，执行确定性金额计算。
- AI 驱动层：解析供应商原始用量并归一化为受控分类用量。
- `usagecharge`：拥有账单、计费明细、价格快照、状态机和结算参与接口。
- `wallet`：拥有余额、冻结、补充冻结、扣款、释放、退款和钱包流水规则。
- `providerattempt`：以 Run 为所有者持久化每次真实供应商派发、幂等 key、供应商请求 ID、结果哈希和不确定状态；聊天命令和各模态任务通过受租约约束的小接口使用它。
- `replycommand`：拥有回复命令、租约、停止、结果不确定恢复和聊天完成事务编排。
- `replycommand` 同时拥有最终助手结果的短期持久化候选，保证供应商已经返回后可以只重试本地完成事务，而不重复调用供应商。
- `message` / `conversation`：拥有消息与会话读写、软删除、配对投影和已读游标。
- `run`：拥有 AI Run 监控记录、输入快照、点赞和运行详情投影。
- text、tool、image、video、audio 各自拥有自己的业务任务、请求指纹和可重放结果；不建立一个同时接管所有产品状态的通用“生成任务”大表。
- 各模态业务服务通过小接口接入 `pricing`、`providerattempt`、`usagecharge` 和 `wallet`，不复制价格公式或直接修改钱包表。

聊天完成时，由 `replycommand` 的窄工作流 finalizer 使用共享数据库事务，调用各所有者提供的事务参与接口；它不能绕过模块 repository 直接写财务表。其他模态由各自任务 finalizer 使用同一 `usagecharge`/`wallet` 能力，不复制第二套余额逻辑。

### 5.6 计费 profile 的跨实例熔断

价格解析时必须生成两种身份：

- `billing_profile_digest`：规范模型、模态/档位、实际采用的价格项、用量归一化 profile、估算器与金额算法实现版本、输入/输出硬上限的规范 SHA-256，用于账单快照和金额指纹；
- `billing_safety_key`：规范模型、模态/档位、用量/估算 profile key 和显式 `billing_safety_revision` 的规范 SHA-256，用于跨目录版本的运行安全 block。

运行中出现以下任一情况，都说明此前宣称 billing-ready 的 safety profile 已经失效，不能只在当前进程打印日志后继续接单：

- 实际费用超过保守冻结；
- profile 宣称可计量，但供应商返回的必要用量缺失、不合法或不能守恒；
- 实际出现未被 profile 覆盖的计量单位、缓存档位或价格档位；
- 同一持久化用量/价格指纹产生不同金额。

`pricing` 能力维护持久化的 `ai_billing_profile_blocks` 运行安全事实，唯一身份为：

```text
billing_safety_key
```

记录同时保存触发时的 `billing_profile_digest`、目录摘要、规范模型、模态、profile key、首次/最近违规 Run 与账单、受控原因、测算金额、冻结金额和 `blocked_at`。finalizer 在完成该次 `settled`、`released` 或 `unbilled` 的同一数据库事务内幂等写入 block；之后所有实例在冻结前都必须检查 safety key，命中时返回定价未就绪且不得调用供应商。该表不是价格覆盖表，不允许修改单价或倍率，也不提供本期 Admin 解封接口。修正相关价格/计量/估算输入后必须显式递增 `billing_safety_revision` 才能形成新 key；仅修改无关目录项、目录摘要或官方价格数值不能意外绕过旧 block，旧 block 永久保留审计。

## 6. 官方数值价格目录

### 6.1 事实源

价格目录固定为：

```text
contracts/ai-pricing/v1/catalog.json
contracts/ai-pricing/v1/catalog.schema.json
```

目录在构建时嵌入后端。每个版本必须包含：

- 单调递增的目录版本；
- 规范内容 SHA-256；
- 发布时间和人工核验时间；
- `official_numeric_parity_v1` 计价策略；
- 官方来源 URL；
- 模型厂商、规范模型 key 和显式别名；
- 支持模态、计量能力、用量归一化 profile、冻结估算 profile 和显式 `billing_safety_revision`；
- 带生效时间的结构化价格项。

运行时不能修改目录。官方价格变化通过新目录版本和应用发布生效。每笔账单快照目录版本、摘要、官方原始币种/数值、人民币基准数值和来源，后续目录更新不能改变历史费用。

### 6.2 模型身份解析

`engine_type` 表示 API 协议，不代表价格厂商。解析器使用供应商模型 ID 和目录显式别名选择规范模型：

1. 精确匹配规范模型 key；
2. 精确匹配目录声明的别名；
3. 否则返回 `pricing_model_unavailable`。

禁止模糊前缀、日期后缀猜测或回退到相近模型。别名歧义使整个目录校验失败。未知别名只能通过审核后的目录更新解决。

### 6.3 价格项

价格项使用受控字段，不使用任意 JSON 配置：

- `metric_code`：如 `input_text_token`、`output_text_token`、`cache_read_token`、`cache_write_5m_token`、`cache_write_1h_token`、`generated_image`、`video_millisecond`、`audio_millisecond`、`input_character`；
- `tier_key`：明确的图片尺寸/质量或视频分辨率档位；
- `quantity_unit` 与 `price_quantity`：如每百万 Token、每张、每 60000 毫秒、每百万字符；
- 官方币种与官方十进制数值；
- 数值相同的人民币结算价；
- 生效时间与官方来源。

默认只使用官方标准按量价。priority、flex、batch 等档位只有目录明确存在且请求明确选择时才可使用，不能根据供应商账户自行猜测。

官方明确为零的价格可以作为显式价格项，但必须同样带来源、生效时间和核验信息；“目录没有价格”与“官方价格为零”是两种状态，禁止用默认零掩盖缺失配置。

价格快照时间取第一次供应商派发前的服务端时间。每个规范模型、`metric_code` 和 `tier_key` 在该时间必须恰好命中一条 `effective_at <= snapshot_at < expires_at` 的价格项；`expires_at` 可为空表示仍有效。目录校验拒绝重叠区间，运行时遇到空档也必须 `ai.pricing_unavailable`，不能回退到过期价或未来价。

## 7. 智能体消费倍率与运行快照

`ai_agents` 新增整数定点字段 `billing_multiplier_ppm`：

```text
1_000_000 = 1.0 倍
1_500_000 = 1.5 倍
```

接口使用十进制字符串，避免 JavaScript/JSON 浮点改变金额。允许范围为 `0.000001` 至 `1000.000000`，对应 `1` 至 `1_000_000_000` ppm。零、负数、超精度和超范围全部拒绝。

启用智能体前，所选模型必须覆盖该智能体全部启用场景所需的价格和计量能力。智能体页面只读展示解析后的价格。

模型、供应商、目录版本和倍率在账单第一次供应商调用前完成快照。同一 Run 的 Worker 重试继续使用原快照；管理员修改只影响之后的新 Run。

编辑或重新生成虽然复制旧用户输入和运行参数，但创建的是新 Run，因此采用执行时的当前快照。旧 Run 的模型、价格和倍率只用于历史审计。

## 8. 分类用量与缓存公平性

### 8.1 统一用量契约

现有 `PromptTokens`、`CompletionTokens`、`TotalTokens` 只适合运行统计，不能作为资金明细。驱动层必须返回结构化归一用量：

- 用量状态及原始字段是否真实存在；
- 供应商请求身份；
- 一个或多个受控用量项；
- 当前驱动采用的原始字段语义；
- 完整性与守恒校验结果。

每个用量项保存供应商调用序号、`metric_code`、`tier_key`、整数数量和数量单位。`ai_runs` 的 Token 总量继续作为监控汇总，账单明细才是资金事实。

### 8.2 缓存分类必须互斥

输入用量归一化后分为：

```text
普通输入
缓存写入（必要时按 TTL 拆分）
缓存读取
```

输出单独记录。驱动校验数量非负，并按供应商字段语义验证输入总量守恒。

- OpenAI 的 `prompt_tokens_details.cached_tokens` 是 prompt tokens 子集，因此普通输入为 `prompt_tokens - cached_tokens`，缓存部分记为 `cache_read_token`。
- 供应商分别返回普通输入、缓存创建和缓存读取时，直接映射为三个互斥项。
- 5 分钟与 1 小时缓存创建不能先合并再计价。
- 工具对话每次模型调用分别记录缓存读写，整次 Run 最终只汇总一次。

### 8.3 缓存明细缺失时不猜价

驱动必须区分“字段不存在”和“字段存在且为 0”。模型存在独立缓存价格、但响应无法区分普通与缓存输入时，用量标记为不完整。

不完整或不合法用量不能全部按普通输入高价扣款。若供应商已经返回有效内容，账单进入 `unbilled`、释放冻结，用户仍获得完整回复，平台承担异常成本。若没有形成有效结果，则按失败流程结束。两种情况都记录可诊断计费故障，并按第 5.6 节 block 此前错误宣称 billing-ready 的 safety profile。

模型官方规则本身没有缓存价格时，完整输入总量可直接按普通输入计费。

## 9. 人民币精度与钱包冻结

### 9.1 唯一余额事实

钱包和账单统一使用整数单位：

```text
1 元 = 100_000_000 money units
1 分 = 1_000_000 money units
```

现有钱包“分”字段迁移为 money units 字段，不保留两套可写余额事实。`user_wallets` 保存：

- `balance_units`：总余额；
- `held_units`：冻结金额；
- `total_recharge_units`：累计充值；
- `total_consume_units`：累计毛消费。

可用余额恒等于 `balance_units - held_units`。充值订单和套餐仍可使用“分”，充值成功时精确乘以 `1_000_000` 后入钱包。

`wallet_transactions` 的金额和变动前后余额也迁移为 money units。金额接口返回十进制人民币字符串，禁止把 int64 money units 作为 JSON number。充值金额显示两位小数；AI 消费与余额最多显示 8 位并去除无意义尾零。

`total_consume_units` 只累计实际 AI 扣款等出账，退款不回写或减少历史毛消费；退款作为独立入账流水展示，净消费由出账减退款计算，不能让同一历史扣款事实随退款被改写。

### 9.2 确定性金额计算

目录价格和倍率均从字符串解析。价格、余额、冻结和账单禁止使用 `float32` 或 `float64`。

中间乘法和求和使用受检查的整数/有理数实现（例如 `math/big`），只在最终 `ceil`/`floor` 后校验能否写入非负 `int64 money units`；任何解析、负数或溢出都使目录/请求 fail closed，禁止依赖 Go 整数溢出行为。

```text
精确基础费用 = Σ(quantity_i * unit_price_units_i / price_quantity_i)
精确最终费用 = 精确基础费用 * multiplier_ppm / 1_000_000
实际扣款单位 = floor(精确最终费用)
```

整笔账单只向下取整一次，偏差归用户。各明细使用确定性的最大余数分配，保证明细之和严格等于总扣款。

保守报价与冻结金额采用 `ceil` 转换为 money units，最终真实扣款采用上述 `floor`。在估算器不变量成立时，最终扣款必定小于或等于冻结。完整用量计算出的最终扣款若小于 1 money unit，则账单仍正常进入 `settled`，释放全部冻结且不创建金额为零的钱包流水；幂等终态由账单自身保证。

示例：普通输入 `¥5/M`、缓存读取 `¥0.5/M`、输出 `¥15/M`、倍率 `1.5`，实际用量为普通输入 900、缓存读取 100、输出 50 Token：

```text
基础费用 = (900 * 5 + 100 * 0.5 + 50 * 15) / 1_000_000
         = ¥0.0053

最终费用 = ¥0.0053 * 1.5
         = ¥0.00795
```

100 个缓存 Token 只按缓存价计算，不重复进入普通输入。

### 9.3 冻结状态机

`wallet_holds` 支持：

```text
reserve  -> 增加 held_units
top up   -> 将当前冻结补充到新的总额
capture  -> 减少 held_units 与 balance_units，并写一条扣款流水
release  -> 只减少 held_units
```

每个冻结使用唯一 `(source_type, source_id)`，AI 的 `source_id` 是用量账单 ID。最终扣款使用 `SourceAIGenerate`；结算后冲正使用 `SourceAIRefund`。释放未扣款冻结不是退款，不产生钱包收入流水。本期退款是以 charge ID 幂等执行的一次完整冲正，不支持多次或部分退款；退款不修改已结算账单和原扣款流水。

冻结、补充、捕获和释放都锁定钱包行并保持幂等。钱包规则只存在于 `wallet` 模块，`usagecharge` 和 AI 服务不得复制余额算法。

Token 请求临时冻结按普通输入、缓存写入、缓存读取中的最高适用价格估算；最终只按真实互斥分类扣款并释放差额。每个请求必须有最大输出 Token，未提供时使用产品上限与目录模型上限中的较小值；工具调用轮次也必须有产品硬上限，否则无法形成可证明的最大报价。

目录为模型绑定受控冻结估算 profile。估算器接收最终序列化供应商请求并返回保守输入上界，禁止使用通用字符数随意猜测。缺少经过测试的估算 profile 或输出上限时，模型不能进入 billing-ready。

工具对话每次追加模型调用前补充同一冻结；余额不足则在下一次供应商调用前停止，绝不先调用再透支。

冻结的 `expires_at` 是下一次恢复扫描时间，不是自动归还时间；权威硬上限由账单的 `resolution_deadline_at` 表达。恢复 Worker 只有在证明失败、取消、形成可恢复结果候选或到达硬上限后，才能通过统一 finalizer 改变资金状态，不能仅靠一个过期扫描器直接修改钱包。

首版运行策略固定如下，计时起点为首次 provider-attempt 进入 `dispatched`：

| 模态 | `uncertain_resolution_ttl` | `absolute_resolution_ttl` |
| --- | ---: | ---: |
| 对话、文本、工具草稿、音频 | 15 分钟 | 1 小时 |
| 图片 | 30 分钟 | 6 小时 |
| 视频 | 6 小时 | 24 小时 |

`uncertain_resolution_ttl` 到达时，如果没有可靠的“供应商任务仍在执行”证据，则立即终结恢复；只有供应商权威查询明确报告任务仍活跃时，才把 `next_reconcile_at` 推进到不晚于“当前时间 + 本模态 uncertain TTL”，且绝不能越过 `resolution_deadline_at = first_dispatched_at + effective_absolute_resolution_ttl`。目录版本可以为特定模型声明更短期限，但不能超过上表绝对上限，运行中也不能因重新部署而重算首次截止时间。

到达绝对上限后：已有完整且验证通过的结果候选按 `unbilled` 免费完成并交付；没有可靠结果候选则将任务/命令和 Run 设为失败、账单设为 `unbilled`、释放冻结并 best-effort 取消供应商任务。此后到达的迟到结果只记录摘要并丢弃，不能重新扣款或把失败请求改回成功。这样即使供应商或本地 finalizer 长时间异常，也不会无限占用用户可用余额。

## 10. 持久化模型

### 10.1 `ai_conversations`

新增：

| 字段 | 约束 | 含义 |
| --- | --- | --- |
| `last_read_message_id` | `BIGINT UNSIGNED NOT NULL DEFAULT 0` | 当前会话所有者最后确认已读的可见 AI 消息 ID |

不增加外键，避免与 `ai_messages.conversation_id` 形成循环删除关系。Service 验证游标消息属于当前用户、当前会话、角色为 assistant 且仍可见，并使用 `GREATEST(existing, incoming)` 单调推进。

### 10.2 `ai_messages`

不新增版本表或消息状态字段，继续使用：

- `is_del=2`：当前可见链；
- `is_del=1`：从用户会话隐藏，但继续供回复命令、Run、账单和审计引用。

新增组合索引：

```text
(conversation_id, is_del, role, id)
```

该索引服务可见上下文、可见 AI 消息计数和已读游标之后的未读查询。

### 10.3 `ai_reply_commands`

继续保持 `user_message_id` 一对一唯一，并增加足够的请求意图事实：

| 字段 | 含义 |
| --- | --- |
| `request_kind` | `send`、`revision` 或 `regeneration` |
| `source_message_id` | 编辑或重新生成时被操作的原始消息 ID；普通发送为空 |
| `request_fingerprint` | 规范化请求意图 SHA-256 |
| `request_fingerprint_version` | 固定算法版本，首版为 `reply_request_v1` |

同一 `(conversation_id, request_id)` 且版本和指纹一致时返回首次结果；相同 ID 但 kind、源消息、文字、附件或运行参数不同则返回冲突，不能静默复用错误命令。`reply_request_v1` 对请求 kind、会话、源消息、规范化文字和服务端规范化后的 `meta_json` 做长度分隔编码后再计算 SHA-256，不拼接含歧义的自由文本。

聊天 `request_id` 同样使用第 10.9 节的规范化、长度、控制字符和 `legacy:` 保留前缀规则；幂等重放查询必须早于活动命令与源消息当前可见性校验，保证第一次编辑/重新生成已经切断旧尾部后仍可正确重放。

### 10.4 `ai_runs`

新增：

| 字段 | 约束 | 含义 |
| --- | --- | --- |
| `liked_at` | `DATETIME(6) NULL` | 非空表示当前 Run 已被所属用户点赞 |

取消点赞将其置空。本期不记录每次切换历史，也不预留点踩枚举。

`ai_runs` 继续承担运行监控而不承担财务状态，但状态集增加 `outcome_unknown`，生命周期为：

```text
running -> success | failed | canceled | timeout | outcome_unknown
outcome_unknown -> success | failed
outcome_unknown -> running（仅当对账证明上次请求未执行且允许重试）
```

进入 `outcome_unknown` 时追加同名 Run 事件但不写 `finished_at`。恢复重试时仍复用同一 Run，并追加新的 `start` 事件；只有最终状态才写 `finished_at`。通用 stale-run 清理必须排除有关联活动命令、`outcome_unknown` 命令或 `uncertain` 账单的 Run。

### 10.5 `ai_usage_charges`

每个可计费 `ai_run` 最多一张账单，快照：

- run、request、user、agent、provider、model 身份；
- 规范价格模型、目录版本、目录摘要、`billing_profile_digest`、`billing_safety_key` 和数值人民币策略；
- 智能体倍率、模态和请求指纹；
- 报价上限、完整用量计算金额、用户实际扣款和金额精度；
- 状态、诊断码和关键时间；
- `uncertain_at`、`next_reconcile_at` 与首次派发时固定的 `resolution_deadline_at`。

`run_id` 唯一、可空并使用 `ON DELETE SET NULL`。财务记录不能随会话或 Run 删除，账单身份快照必须足够独立审计。

状态固定为：

```text
pending -> reserved -> settled
                   -> released
                   -> uncertain -> reserved | settled | released | unbilled
pending -> rejected
reserved -> unbilled
```

`settled`、`released`、`rejected`、`unbilled` 是终态。`unbilled` 只表达“本次没有捕获用户资金”，不单独表达业务成功或失败：有完整有效结果时，业务任务/命令和 Run 成功、结果 `finalized` 并免费交付；没有可靠结果时，业务任务/命令和 Run 失败且不交付结果。查询与审计必须联合业务状态、结果状态和账单状态判断，不能把所有 `unbilled` 都显示成成功或失败。退款通过新钱包入账表达，不修改已结算账单金额。

`uncertain -> reserved` 只允许在供应商对账明确证明上次请求没有执行、回复命令被重新排队时发生；继续复用原价格快照和冻结。已确认供应商成功或仍无法判定时禁止重发。

报价为零时账单仍进入 `reserved`，但不创建零金额 `wallet_holds`；最终按正常规则进入零金额 `settled`，也不写零金额钱包流水。非零报价必须恰好对应一个以账单 ID 为源身份的 hold。

### 10.6 `ai_usage_charge_items`

计费明细为追加写事实，保存：

- 账单 ID、供应商调用序号；
- 可选 provider-attempt 与 provider-request 身份；
- `metric_code`、`tier_key`、数量、数量单位；
- 定价数量单位和人民币单价；
- 官方原始币种/数值与人民币数值；
- 分配后的基础计算金额、倍率后计算金额和用户实际扣款金额；
- 用量指纹和价格指纹。

唯一的调用/指标/档位身份阻止 Worker 重试重复写入。即使账单 `released` 或 `unbilled`，真实数量与计算金额仍保留，但用户实际扣款金额为零，用于明确区分平台测得成本与用户钱包扣款。

### 10.7 `wallet_holds`

冻结保存 wallet、user、source、冻结金额、已扣金额、状态、请求指纹、过期时间和终态时间。状态只允许 `active`、`captured`、`released`，不使用软删除。

### 10.8 `ai_reply_completion_candidates`

供应商返回最终助手内容后、对外发布前，先保存一条一对一的短期完成候选：

- command ID 与 run ID，均唯一；
- 最终助手正文及其 SHA-256；
- 全部调用的聚合用量指纹与用量完整性；
- `staged`、`finalized` 或 `discarded` 状态及关键时间。

最终一次 provider-attempt 结果、已归一化计费明细和完成候选在同一短事务中持久化。该事务必须再次验证 command 仍为当前租约 owner/fencing token 持有的 `running` 状态；候选写入成功后，任何 Worker 重试都只能重试本地 finalizer，不能再次调用供应商。

候选正文只用于崩溃恢复，访问边界与 `ai_messages` 相同且不得进入日志。finalizer 成功插入正式助手消息或确认丢弃时，在同一事务中清空候选正文，只保留哈希、状态和时间，避免永久保存第二份回复正文。

本需求不创建消息版本表、点赞表或未读明细表；完成候选是短期运行恢复事实，不是用户可切换的答案版本。

### 10.9 各计费任务的客户端幂等字段

`ai_text_tasks`、`ai_image_tasks`、`ai_video_tasks` 以及新增的音频和工具草稿任务都保存：

| 字段 | 含义 |
| --- | --- |
| `request_id` | 客户端生成的 1 至 128 字符业务请求 ID |
| `request_id_source` | `client` 或仅迁移使用的 `legacy` |
| `request_fingerprint` | 规范化用户意图的 SHA-256 |
| `request_fingerprint_version` | 各任务类型固定的指纹算法版本 |
| `run_id` | 一对一绑定的稳定 AI Run |

每张任务表使用 `(platform, user_id, request_id)` 唯一约束。相同 ID 和相同指纹返回首次任务及其当前状态；首次任务成功时重放首次结果，失败或结果不确定时返回同一状态，不能创建第二次供应商调用。相同 ID 与不同指纹返回 HTTP 409。用户主动发起一次新的生成必须使用新的 `request_id`。

`request_id` 先执行一次首尾空白去除，之后必须包含 1 至 128 个 Unicode 字符且不得含控制字符、换行或保留前缀 `legacy:`；服务端持久化该规范值，后续比较区分大小写。缺失/空白与格式非法是两个稳定错误。唯一键冲突后必须重新读取既有任务并比较指纹，不能把所有数据库唯一键错误都当成幂等成功。

指纹覆盖任务类型、智能体 ID、服务端规范化后的全部生成参数以及输入内容。图片/视频参考文件使用服务端计算的内容 SHA-256 和稳定存储身份，不能信任客户端文件名、URL 或客户端上报哈希。客户端若仍携带兼容性 `model_id`，该值属于客户端意图并进入指纹；对新任务才校验它与智能体当前模型一致，实际执行仍以服务端智能体配置为准。幂等查询和指纹比较必须早于这类会随管理员配置变化的 readiness 校验，因此旧请求在智能体改模型后仍能重放首次结果。服务端当前模型、目录版本和倍率属于首次执行快照，不属于客户端请求指纹。

首版指纹版本固定为 `text_request_v1`、`tool_generation_request_v1`、`image_request_v1`、`video_request_v1` 和 `audio_request_v1`。规范化对象使用 UTF-8、对象键排序、数组保序、十进制字符串规范化和长度分隔编码后计算 SHA-256；禁止直接拼接自由文本或依赖 Go map/普通 JSON 序列化顺序。

业务任务必须在供应商调用前提交。Run 的 `request_id` 保留客户端 ID 用于关联查询，`idempotency_key` 则由已提交任务身份派生：

```text
reply-command:<id>
text-task:<id>
tool-generation-task:<id>
image-task:<id>
video-task:<id>
audio-task:<id>
```

旧任务没有客户端请求 ID，迁移时只能生成明确带 `legacy:` 前缀且 `request_id_source=legacy` 的内部唯一值用于历史约束；自助幂等查询只接受 `client`，不能把历史值伪装成客户端可重放 ID，也不能因此重新执行旧任务。

### 10.10 可重放结果与模态任务

除聊天 reply command 继续使用 `succeeded` 外，非聊天模态任务统一使用 `pending`、`running`、`success`、`failed`、`outcome_unknown` 业务状态，并另存 `result_state`：

```text
none -> staged -> finalized
               -> discarded
finalized -> unavailable（仅确认结果完整性永久损坏）
legacy（仅 `request_id_source=legacy` 的迁移终态）
```

新的客户端任务进入 `success` 的事务必须同时把结果设为 `finalized`；之后只有确认结果完整性永久损坏时才允许转为 `unavailable`。`legacy` 只隔离无法在 SQL 迁移中证明新结果契约的历史任务，不得进入客户端幂等重放、计费或新 Worker。`staged` 只表示平台已经持久化可恢复候选，不能通过用户查询、直链或签名 URL 对外发布。候选至少保存内容 SHA-256、规范结果 manifest SHA-256、可计费用量指纹、创建时间和 finalizer 错误摘要。正文类候选保存在所属任务表；对象类候选还保存 `storage_provider`、私有 `storage_key`、MIME、字节数和模态元数据。

- `ai_text_tasks` 继续保存最终答案，并补充幂等字段、Run 绑定、业务状态和 `result_state`。
- 新增 `ai_tool_generation_tasks`，保存规范化需求、code hint、有效的最终草稿/澄清结果 JSON 和同一组幂等、Run、状态字段。模型返回合法的 `ok=false` 澄清结果属于成功结果并正常结算；协议无效或无法解析的输出不交付用户，任务按失败结束、释放冻结并记录供应商实际用量与协议诊断，不把原始模型文本当作草稿返回。
- `ai_image_tasks` 增加幂等字段、显式 Run 绑定、结果状态、期望/实际输出数量和 manifest 摘要；现有 `ai_image_files` 继续作为输入、遮罩和输出结果事实。输出行增加 `publish_state`、内容 SHA-256 和验证元数据，多张输出全部进入候选 manifest 后才允许 finalizer；官方允许部分成功时，manifest 明确实际成功集合并只按该集合结算。
- `ai_video_tasks` 增加幂等字段、结果状态与候选对象元数据。供应商成功后先把最终视频复制到平台控制的私有对象存储；只保存易过期的 provider task ID 不足以承诺已付费结果可重放。
- 新增 `ai_audio_tasks`，保存规范化请求、业务/结果状态、Run、候选对象的 storage key、MIME、字节数、内容 SHA-256 和时长等可验证元数据。音频正文进入平台控制的私有对象存储，不把大二进制写入 MySQL。

对象结果采用以下两阶段发布协议：

1. 使用由 task ID 和输出序号派生的确定性私有 key 上传，写入内容摘要元数据；相同 key 的恢复上传必须先 HEAD 并核对摘要，绝不能静默覆盖不同内容；
2. 上传和完整性校验成功后，在持有任务 lease/fencing token 的短事务中写入 `staged` 候选、provider-attempt 结果和归一化用量；
3. finalizer 锁定任务、候选、账单和钱包，原子提交业务 `success`、结果 `finalized`、Run 与财务终态；对象不需要在事务后移动，下载授权只允许读取数据库已 `finalized` 且属于当前用户的私有 key；
4. 上传成功但数据库提交前崩溃时，Worker 用确定性 key 和摘要恢复候选；没有任何任务/候选引用的对象只能由带宽限和宽限期的孤儿清理器删除；
5. 失败或取消的候选进入 `discarded`，对象删除使用幂等清理任务。删除失败只产生清理告警，不能把业务失败改成成功，也不能再次调用供应商。

`storage_provider + storage_key` 是对象身份事实；历史 `storage_url` 如继续保留，只能作为可重新生成的展示缓存，不能用于结算、重放或完整性判断。成功结果使用独立私有前缀，该前缀必须排除在临时上传和孤儿对象生命周期规则之外。

同步文本、工具草稿和音频仍可保持同步 HTTP 体验，但响应必须来自已经 finalizer 成功的持久化结果。若成功响应在网络途中丢失，客户端用同一 `request_id` 重试时直接重放结果，不能重新调用供应商或再次扣款。任务显示 `success` 但正文、manifest 或对象引用经确认永久缺失/摘要不符时，结果完整性 finalizer 锁定结果、账单和钱包，在同一事务内把结果从 `finalized` 置为 `unavailable`，并在原账单确有非零 `settled` 扣款时通过 `SourceAIRefund + charge_id` 身份幂等执行一次完整退款；账单和原扣款仍保持不可变审计。随后返回 `error.code=ai.result_unavailable`，绝不能借同一 ID 重新生成。对象存储暂时不可用时只返回可重试 dependency 错误，不提前判定永久损坏或退款。

本期不实现成功结果 TTL，也不注册清理成功任务、结果正文、`finalized` 图片、视频或音频对象的定时任务。输入附件继续遵守既有存储生命周期，但不得清理仍被付费结果重放所需的对象。未来删除或分层归档成功结果必须另写产品保留策略、用户提示、账单争议规则和数据迁移设计，不能作为普通后台清理顺手加入。

### 10.11 Run 级供应商尝试

`ai_provider_attempts` 的表所有权从 `replycommand` 移到独立 `providerattempt` 能力，并从聊天命令附属事实提升为所有计费模态共享的 Run 级事实：

- `run_id` 为必填所有者，`command_id` 只在聊天时作为可空关联；
- `(run_id, attempt_no)` 唯一，每个 Run 内的供应商调用序号从 1 单调递增；工具调用后的模型轮次使用下一序号；
- 每个调用在发送前持久化 `prepared` 并取得由 `run_id + attempt_no` 派生的稳定 provider idempotency key；“即将发送”不能与外部网络 I/O 真正原子，因此发送前先持久化 `dispatched_at` 和 `dispatched`，再使用同一个 key 调用供应商；
- 驱动支持时必须把该 key 传给供应商；异步媒体同时保存 provider task ID；
- 成功保存供应商请求 ID、响应哈希和结构化用量，失败、取消与 `outcome_unknown` 保存明确状态；
- `prepared` 且没有 `dispatched_at` 可以由当前合法执行 owner 使用原 key 派发；任何已经带 `dispatched_at` 的尝试都视为“可能已到达供应商”；
- 恢复只能复用同一调用身份。只有供应商文档明确保证该 key 的幂等重放，或权威查询证明上次没有执行时才可再次发送；否则保持不确定并最终按本设计释放，禁止换新 key 盲目重发。

聊天现有 command lease/fencing token 继续保护尝试写入；其他任务必须提供等价的 claim/lease 或单次 compare-and-swap 状态保护，不能仅凭 Run ID 绕过执行所有权。

### 10.12 `ai_billing_profile_blocks`

该表由 `pricing` 能力拥有，只保存系统自动触发的运行安全 block：

- 目录摘要、规范模型、模态和估算 profile key；
- `billing_profile_digest`；
- `billing_safety_key` 与触发时的 safety revision；
- 首次/最近触发的 Run 与账单 ID；
- 冻结金额、完整测算金额、原因和触发次数；
- `blocked_at`、`last_seen_at`，不使用软删除。

唯一键为 `billing_safety_key`。表中存在该 key 即表示 block 生效，不设置可被误清空的布尔开关；只有修正后显式递增 `billing_safety_revision` 才能恢复 billing-ready。读取失败必须 fail closed，不能在数据库异常时绕过 block 检查。

## 11. 对话请求、运行与结算生命周期

### 11.1 接收发送、编辑或重新生成

三种入口必须复用同一 reply-command 创建协调器，并在一个短事务中：

1. 锁定会话并验证当前用户所有权；
2. 先按 `(conversation_id, request_id)` 查询幂等重放并校验请求指纹；
3. 拒绝会话中已有 `pending`、`claimed`、`running` 或 `outcome_unknown` 命令；
4. 普通发送创建用户消息；编辑或重新生成按第 12 节规则切换可见链并创建新用户消息；
5. 创建一个新回复命令，更新 `last_message_at`；
6. 提交后 best-effort 唤醒现有 Worker，持久化轮询仍是恢复路径。

同一会话任何时刻最多一个活动回复命令。正常发送、编辑和重新生成都受该规则约束，不能只在前端禁用按钮。

接口返回 HTTP 202、新用户消息、回复命令和请求 ID。此时尚未调用供应商，也未产生消费。

### 11.2 Worker 调用供应商之前

1. claim 回复命令并取得租约与 fencing token；
2. 创建或恢复由命令 ID 派生幂等身份的 `ai_run`；
3. 读取最终可见上下文，加载智能体、供应商和模型；
4. 解析规范价格模型和所需价格项；
5. 创建或恢复账单，快照目录、模型和智能体倍率；
6. 对最终序列化请求计算保守最大费用；
7. 报价大于零时在钱包冻结该金额；报价为零时不创建 hold，但账单仍进入 `reserved`；
8. 创建或恢复本次 provider-attempt；在发送任何网络字节前，把 attempt 的 `dispatched_at` 与账单首次 `resolution_deadline_at` 在同一事务中持久化；
9. 冻结和派发事实都成功后才允许发送供应商请求，并 best-effort 发布 `ai.response.start.v1`。

价格、计量能力、估算器或余额校验失败时，不调用供应商。Run 与命令进入明确失败终态，账单为 `rejected` 或已冻结部分被 `released`，并通过现有 durable failed 事件把稳定错误类别送达前端。

由于冻结是 Worker 权威动作，HTTP 202 后仍可能异步得到“可用余额不足”。前端保留用户消息，停止加载态，显示简短余额不足提示和现有充值入口；不得把它伪装成供应商错误。

### 11.3 流式输出期间

- start/delta 只表示运行进度，不表示完成或已经扣款；
- delta 不持久化、不增加未读数；
- start/delta 的实时投递失败不改变命令、Run 或账单状态，也不能中断已经获准的供应商调用；最终 durable 事件和 HTTP 查询负责权威恢复；
- 前端必须把 delta 视为临时内容；收到 failed、canceled、timed_out 或权威恢复结果后清除临时内容，不能把未完成 delta 留成一条本地成功回复；
- 工具调用后准备下一次模型请求前，先归一化已完成调用的用量并补充冻结；
- 补充冻结失败时不发送下一次供应商请求；本次 Run 按失败规则释放冻结，不向用户捕获半成品估算费用；
- 页面切换后继续按 `conversation_id + request_id` 保存流式会话状态，断线时以权威查询恢复。

### 11.4 成功结果的原子完成

供应商返回有效最终内容后：

1. 驱动校验并归一化每次调用用量；
2. 在短事务中完成最终 provider-attempt、按调用序号幂等写入计费明细并写入完成候选；
3. 用量完整时计算一次精确总费用；用量不完整但内容有效时选择 `unbilled`，并准备第 5.6 节定义的 safety block 事实；
4. 聊天 finalizer 锁定命令、候选、账单和钱包，在同一 MySQL 事务中完成以下事实：
   - 用量完整且扣款大于零：capture 精确金额、释放差额、写一条钱包流水、账单设为 `settled`；
   - 用量完整且扣款为零：释放全部冻结、不写零金额流水、账单设为 `settled`；
   - 用量不完整：释放全部冻结、账单设为 `unbilled`，并以 `billing_safety_key` 幂等写入 `ai_billing_profile_blocks`；
   - 插入可见 assistant message；
   - 绑定 reply command 与 `ai_run.assistant_message_id`；
   - 将回复命令设为 `succeeded`，Run 设为成功；
   - 追加 durable `ai.response.completed.v1`。
   - 将完成候选设为 `finalized` 并清空候选正文。
5. 事务提交后才 best-effort 实时推送完成事件。

禁止继续采用“先发布成功消息，再 best-effort 记录 Run 或扣钱包”的顺序，也禁止“钱包已扣、消息和 Run 仍报告失败”。事务参与接口由各模块 owner 提供，finalizer 不直接跨模块写表。

finalizer 暂时失败时保留冻结，并从持久化候选重试相同完成事实，不重复调用供应商。数据库死锁、超时或短暂不可用属于可重试 finalizer 故障，必须回滚整个事务，不能被解释成“补充冻结失败”而提前改变资金事实。若恢复时已经越过持久化的 `resolution_deadline_at`，有效候选按 `unbilled` 免费完成；没有候选则按不确定终态释放。若供应商已经响应、但完成候选尚未成功落库时进程崩溃，则进入不确定恢复；最终只能根据供应商查询或其他持久化证据完成，否则免费释放，不能盲目重发或拿估算金额结算。

失败、取消和超时也使用单一终态 finalizer：在同一事务内释放冻结、终结账单、更新 command 与 Run、丢弃并清空已有完成候选、追加对应 durable failed/canceled 事件。禁止由 chat service、Run recorder 和 wallet 各自 best-effort 写终态。

### 11.5 停止、失败和结果不确定

| 场景 | Command | Run | 账单与冻结 | 用户可见结果 |
| --- | --- | --- | --- | --- |
| `pending` 时停止 | `canceled` | 尚未创建 | 无账单 | 无 AI 回复、无未读 |
| `claimed`、调用前停止 | `canceled` | 未创建或 `canceled` | 无账单或 `released` | 无 AI 回复、无未读 |
| 调用前定价/余额失败 | `failed` | `failed` | `rejected` 或 `released` | 稳定错误；余额不足提供充值入口 |
| 供应商确认失败 | `failed` | `failed` | `released`，不扣款 | 无完整 AI 回复、无未读 |
| `running` 时用户停止 | `canceled` | `canceled` | `released`，不扣款 | 丢弃临时 delta，不落助手消息 |
| 超时且确认无成功结果 | `timed_out` | `timeout` | `released`，不扣款 | 无完整 AI 回复、无未读 |
| 请求已发出但结果未知 | `outcome_unknown` | `outcome_unknown` | `uncertain`，继续冻结 | 显示“正在确认结果”，不计未读 |
| 对账证明未执行并允许重试 | `pending` | `running`（重新 claim 时） | `reserved`，复用原冻结 | 恢复生成态，不新建 Run 或账单 |
| 对账恢复内容和完整用量 | `succeeded` | `success` | `settled` | 原子发布完整回复 |
| 对账恢复内容但用量不完整 | `succeeded` | `success` | `unbilled` 并释放 | 免费发布完整回复 |
| 到期仍无可靠结果 | `failed` | `failed` | `unbilled` 并释放 | 无完整回复，不扣款 |

`outcome_unknown` 不能通过“停止生成”伪装成已取消，因为服务端无法证明供应商没有成功。该状态下隐藏或禁用停止操作，阻止发送、编辑、重新生成和删除，等待对账进入明确终态。前端按钮只是体验层，Service 和事务查询必须再次校验。

停止与完成并发时由命令租约、fencing token、行锁和一次终态转换决定唯一赢家：停止请求先写入 `cancel_requested_at` 时，完成 finalizer 必须拒绝发布，取消 finalizer 释放资金并丢弃候选；完成事务先提交时返回既有成功结果，迟到停止不能撤销已结算事实。

现有停止接口继续使用：

```text
POST /api/admin/v1/ai-conversations/:conversation_id/messages/cancel
```

响应必须返回真实 `state` 与 `cancel_requested`，不得固定返回 `canceled`：

- `pending` 可在请求事务内直接进入 `canceled`；
- `claimed` 或 `running` 只表示已接受停止请求，前端保持“正在停止”直到 durable 终态事件或权威恢复；
- 已进入终态时幂等返回真实终态，若为 `succeeded` 说明完成事务赢得竞态；
- `outcome_unknown` 返回 HTTP 409 和稳定错误，不承诺能够取消供应商已经可能完成的请求。

如果实际费用意外超过保守冻结，finalizer 在同一钱包锁内计算差额：可用余额足够时补充同一 hold 后正常捕获；锁内确定余额不足时，有效完整结果免费交付，账单记录完整测算金额但进入 `unbilled`，释放已有冻结且用户实际扣款为零。无论差额是否补充成功，超额本身都证明估算 profile 失效，必须按第 5.6 节持久化 block 并阻止同一 `billing_safety_key` 继续接收新调用。禁止部分捕获、禁止透支，也不能把数据库异常或“调用后再负数扣款”当作正常补差路径。

## 12. 消息选择、编辑与重新生成

### 12.1 消息配对与选择

消息列表为每条消息增加：

| 字段 | 含义 |
| --- | --- |
| `paired_message_id` | 当前可见问答对中的另一条消息 ID；无配对时为 `null` |
| `run_id` | AI 回复对应的 Run ID；用户消息为 `null` |
| `liked` | AI 回复对应 Run 是否已点赞；用户消息固定为 `false` |

配对依据 `ai_reply_commands.user_message_id`、`assistant_message_id` 和助手消息的 `reply_command_id` 批量 JOIN/映射，且只有两端当前都可见时才返回配对 ID。禁止按相邻消息猜测，也禁止逐消息 N+1 查询。

进入选择模式时默认选择触发消息及其 `paired_message_id`；用户可独立取消任一条，因此允许删除完整问答对、只删问题、只删回答或跨多轮批量选择。

### 12.2 编辑用户消息

```text
POST /api/admin/v1/ai-conversations/:conversation_id/messages/:message_id/revisions
```

请求：

```json
{
  "content": "修改后的文字",
  "request_id": "客户端幂等请求 ID"
}
```

服务端在第 11.1 节公共事务内：

1. 验证源消息是当前可见用户消息；
2. 记录事务开始时的当前可见尾部 ID 上界；
3. 将源消息至该上界之间的消息统一软删除；
4. 创建新用户消息，复制原 `meta_json`，只替换文字；
5. 创建 `request_kind=revision` 的新回复命令。

附件与运行参数由服务端复制，不接受前端替换。空白文字返回 400。重复请求且指纹一致返回第一次创建的身份，不重复切断尾部。

### 12.3 重新生成

```text
POST /api/admin/v1/ai-conversations/:conversation_id/messages/:message_id/regenerations
```

请求只包含 `request_id`。目标必须是已完成且当前可见的 AI 回复，其配对用户消息也必须可见。服务端通过回复命令关系复制用户文字和完整 `meta_json`，执行同样的尾部替换，并创建 `request_kind=regeneration` 的新命令。

任一配对消息已删除时返回 404，不能借重新生成恢复用户已经删除的问题。虽然新用户消息 ID 改变，界面仍表现为原问题获得新答案；旧 ID、旧 Run 和旧账单继续保持审计一致。

### 12.4 批量删除消息

```text
DELETE /api/admin/v1/ai-conversations/:conversation_id/messages
```

请求体为去重后的正整数 `ids`。后端只软删除明确提交的消息 ID，不自动扩大到问答对；默认配对选择只是前端交互规则。

事务必须：

- 锁定并验证会话所有权；
- 拒绝空 ID、重复 ID、跨会话 ID 和不存在的可见消息；
- 拒绝任何活动回复命令；
- 一次更新全部目标消息；
- 根据剩余可见消息重算 `last_message_at`。

删除不级联修改回复命令、Run、点赞、账单、冻结历史或钱包流水。后续供应商上下文只使用仍可见消息。

### 12.5 删除整个会话

现有会话删除同样属于历史改写。只要存在活动回复命令就返回 HTTP 409，用户必须先停止并等待明确终态；`outcome_unknown` 期间只能等待对账。允许删除时继续软删除会话和可见消息，但不级联回复命令、Run、完成候选或任何财务事实。

## 13. 未读、点赞、朗读与输入快照

### 13.1 服务端权威未读数

推进游标：

```text
PUT /api/admin/v1/ai-conversations/:conversation_id/read-cursor
```

请求包含 `message_id`，必须是该会话当前可见且成功落库的 AI 消息。更新只允许单调前进，重复请求幂等。

会话列表新增 `unread_count`：

```text
conversation_id 相同
AND role = assistant
AND is_del = 2
AND id > last_read_message_id
```

- 当前会话恢复完消息并确认实际可见后，推进到最新可见 AI 消息；
- 非当前会话收到 completed 事件时刷新会话列表；
- start/delta、failed、canceled、timed_out、outcome_unknown 不增加未读；
- 删除未读 AI 消息后，查询结果自然减少；
- `realtime.resync_required` 继续走现有权威恢复，不从本地累计值重建。

会话列表项右侧显示使用现有未读语义色 token 的紧凑红色数量徽标，`0` 不渲染；徽标预留稳定宽度，长数字不能挤压或遮挡会话标题。当前正在查看的会话不显示瞬时红点，消息恢复完成后再推进游标。

不持久化 `unread_count += 1`，避免事件重放、删除和多标签页导致派生值漂移。

会话列表一页的未读数通过一次分组查询或批量 JOIN 投影，禁止为每个会话单独执行 COUNT。

### 13.2 Run 点赞

```text
PUT /api/admin/v1/ai-runs/:id/user-feedback
```

```json
{ "liked": true }
```

服务端验证 Run 属于当前用户、属于聊天会话、运行成功且已绑定 AI 消息。`PUT` 幂等设置 `liked_at`，不做计数累加。AI Runs 详情返回 `liked` 与 `liked_at`；Run 列表本期不增加列。

### 13.3 免费朗读

- 仅 AI 回复显示朗读入口；
- 使用 `window.speechSynthesis` 与 `SpeechSynthesisUtterance`；
- 优先选择可用的 Google 中文音色，其次其他 `zh-CN` 系统音色，最后使用浏览器默认音色；
- 同时只朗读一条，支持开始、暂停/继续和停止；
- 切换会话、离开页面或朗读另一条时停止当前朗读；
- 浏览器不支持时禁用入口并简短提示，不回退到付费服务；
- 文本不发送到新后端或非正式第三方 TTS 接口。

### 13.4 AI Runs 输入快照

运行详情不能直接展示嵌套转义 JSON。前端解析器按顺序：

1. 尝试解析外层 `input_snapshot`；
2. 存在字符串类型 `meta_json` 时再严格解析一次；
3. 将 `content`、`attachments`、`runtime_params` 映射为结构化视图；
4. 历史纯文本直接显示；任何解析失败安全回退原文。

结构化视图展示用户文字、附件名称/类型/大小/URL、图片缩略图和运行参数。禁止 `v-html`；附件 URL 只走现有受信任 HTTP/HTTPS 预览路径，文件名和参数值由 Vue 文本转义。

## 14. 前端消费者交互

### 14.1 消息操作栏

- AI 回复：保留复制，增加朗读、点赞、重新生成和进入选择/删除；
- 用户消息：保留复制，增加编辑和进入选择/删除；
- 桌面端在消息悬停或键盘聚焦时显示操作栏；
- 触摸设备提供始终可点击的入口，不能只依赖 hover；
- 使用现有图标库、tooltip、`aria-label` 和稳定按钮尺寸。

### 14.2 选择模式

- 当前会话头部进入明确的选择状态并提供取消；
- 每条消息显示独立复选框；
- 初始选择为触发消息及其配对消息；
- 底部固定显示删除按钮和已选数量，空选择时禁用；
- 提交前使用现有确认组件；成功后退出选择模式并刷新权威消息与会话数据。

### 14.3 编辑、重新生成和活动态

- 编辑在原用户消息位置进入紧凑文本编辑态，不打开后台式大弹窗；
- 编辑器只编辑文字，附件只读展示；
- 提交后显示新用户消息与异步回复占位；
- 重新生成使用新 request ID，并复用现有流式会话状态；
- `pending`、`claimed`、`running` 时提供现有停止能力并禁止继续发送或改写历史；
- `outcome_unknown` 时显示确认中状态，不显示会造成错误承诺的停止成功入口；
- 余额不足时结束占位加载，保留用户输入并给出充值入口；
- 页面切换不丢失未完成回复，继续按 `conversation_id + request_id` 管理会话缓存。

### 14.4 充值收银台精简

删除收银台整个“最近充值”区域、专用组件/样式以及仅为该区域发起的数据读取。“充值记录”Tab、记录列表、继续支付和相关列表 API 保持不变。

收银台 PageInit 不再返回或查询 `recent`，前提是契约搜索确认没有其他消费者。钱包余额按高精度人民币字符串展示，充值套餐金额仍按两位小数展示。

## 15. Admin API 与只读价格界面

### 15.1 智能体配置

智能体新增/编辑契约增加：

- 十进制字符串消费倍率；
- 只读规范价格模型；
- 只读 billing-ready 状态；
- 当前模型和场景所需的只读基础价格项。

选择供应商模型后刷新价格预览。billing-ready 同时要求目录能力完整且当前 `billing_safety_key` 没有 active block；缺少价格、估算、计量能力或命中 block 时，表单不能启用该智能体，已启用智能体的后续运行也必须在供应商调用前 fail closed。block 只读展示原因和触发时间，不提供绕过或解封控件。

### 15.2 只读价格目录

新增 `/ai/pricing` 页面，展示模型、模态、单位/档位、官方来源数值、人民币基准值、来源 URL、目录版本、核验时间和只读 billing safety 状态。

页面与读取接口使用 `ai_pricing_list` 权限。不存在价格新增、编辑或删除接口。智能体表单价格预览随已有 page-init/detail 返回；拥有 `ai_agent_add` 或 `ai_agent_edit` 的管理员不需要额外价格目录权限才能编辑智能体。

### 15.3 Run 与钱包详情

AI Run 详情统一增加：

- 结构化输入快照与点赞状态；
- 账单状态和总费用；
- 目录、模型和倍率快照；
- 普通输入、缓存写入档位、缓存读取、输出和媒体用量；
- 单价、分项金额、冻结和异常恢复状态；
- 结果可用性、完整退款流水和 billing safety block 诊断。

钱包摘要区分总余额与可用余额。AI 流水最多显示 8 位小数并链接到账单/Run 详情。缓存 Token 及折扣价必须可见，便于确认没有按普通输入重复扣费。

现有 `/ai/runs` 是平台运行监控面，当前接口可按任意用户查看统计，不得因聊天自助点赞而被误改成“所有登录用户都只能看自己的 Run”。本期契约拆分为：

- 现有管理端 Run 列表、统计和详情继续保留平台监控语义。现有 `/ai/runs` 菜单权限行 `code` 为空，因此本期把它显式更新为 `ai_run_list`，相应路由改用同一权限；这只是补齐权限定义，不向任何角色新增关系；
- 聊天消息投影和 `PUT /ai-runs/:id/user-feedback` 走自助所有权查询，只能读取/修改当前用户自己的聊天 Run；
- 钱包 `/wallet/summary`、`/wallet/transactions` 继续是当前用户自助接口；平台钱包和总流水继续使用既有 `payment_wallet_list`、`payment_ledger_list`；
- 自助响应不能暴露其他用户、内部供应商凭据或平台级统计，管理端响应也不能绕过其既有权限边界。

新增 `ai_pricing_list` 和 `ai_run_list` 权限定义并进入 route metadata 与 Admin Contract Bundle，但不自动赋予角色，也不写 `role_permissions`。

## 16. 各模态计费规则

### 16.1 对话、文本与智能体生成

- 每次真实模型调用都计量，包括工具调用后的模型轮次；
- 本地工具执行和本地知识检索本身不收费，独立调用计费模型时才产生新用量；
- 普通输入、缓存写入、缓存读取、输出及官方区分的图像/音频 Token 分别记录；
- `ai_runs` 保存统计汇总，账单明细保存资金分类。

聊天使用 reply command ID 作为稳定业务身份。同步文本与 `agent_generate` 工具草稿接口必须接收客户端 `request_id`，先创建或恢复各自任务，再以 task ID 建立 Run、供应商尝试和账单。

同步文本与工具草稿在返回成功响应前，通过各自 finalizer 原子完成任务结果、Run、账单和钱包事实；若 finalizer 失败则保留冻结并返回带相同任务/请求身份的可恢复错误，不能先返回正文或草稿再 best-effort 扣款。同一 `request_id` 的后续查询或重试只重放已保存结果。

### 16.2 图片

- 按张模型根据请求数量、尺寸和质量冻结，按实际成功的可计费输出数量结算；
- Token 计价图片模型记录图片输入/输出 Token，不能伪装成按张价格；
- 部分成功按实际成功数量和真实用量结算；
- 创建接口携带客户端 `request_id`，相同 ID 重放既有 task；task ID 是 Worker、Run、账单和供应商尝试的内部稳定身份；
- Worker 重试复用同一 task、Run、账单和 provider-attempt，不能因任务已入队但 HTTP 响应丢失而创建第二个任务；
- 图片输出文件与任务成功、Run 和财务终态由模态 finalizer 协调提交；对象已经写入但数据库事务失败时，恢复复用已持久化的候选对象或清理孤儿对象，不能再次调用供应商。

### 16.3 视频

- 创建供应商任务前，根据时长、分辨率、音频选项和官方计费项冻结；
- 供应商任务处理中保持冻结；
- 创建接口携带客户端 `request_id`，相同 ID 返回既有平台 task；供应商 task ID 绑定同一个 provider-attempt；
- 成功后先把供应商内容复制到平台控制的用户结果存储，再由 finalizer 协调提交持久化结果与财务终态；失败、取消或超时释放；
- 状态轮询和内容下载不得产生新费用，重复轮询只能恢复同一供应商 task；
- 过期任务使用统一账单恢复，不新增媒体专用余额逻辑。

### 16.4 音频

- 音频驱动结果补充结构化计量元数据；
- 按字符计费使用规范化请求字符数；
- 按时长计费使用供应商时长或确定性媒体时长解析；
- 按 Token 计费使用分类输入/输出 Token；
- 缺少官方计费所需数据时 `unbilled`，不能用响应字节数猜时长或 Token。

音频接口必须接收客户端 `request_id` 并先创建 `ai_audio_tasks`。当前直接返回原始音频响应的同步链路必须在有界内存或临时文件中完整接收并验证供应商结果，把音频持久化到用户结果存储，再完成 Run 与财务 finalizer，最后才向客户端写成功响应。若网络交付失败，同一 `request_id` 从持久化对象重放，不再调用供应商。

若模型按请求字符计费，数量仍取规范化请求字符数；缓冲音频字节只用于确保结果完整，绝不能成为计费单位。响应超过受控大小上限时按失败释放，不允许无限缓冲。对象存储写入成功但 finalizer 失败时保留可恢复候选引用并重试本地 finalizer；确认失败后清理孤儿对象。

浏览器消息朗读不属于 AI 音频生成，不进入本节计费链路。

## 17. 幂等、事务与恢复

所有可计费入口的稳定身份链：

```text
客户端 request_id + 服务端请求指纹
  -> 业务任务（聊天为 ai_reply_command；其他模态为各自 task）
      -> ai_run.id（idempotency: <task-type>:<task-id>）
          -> ai_provider_attempts(run_id, attempt_no)
          -> ai_usage_charges.run_id（唯一）
              -> wallet_holds(ai_usage, charge_id)（仅非零报价；唯一）
              -> wallet_transactions(ai_generate, charge_id)（仅非零实际扣款；唯一）
              -> wallet_transactions(ai_refund, charge_id)（仅永久结果损坏且存在非零扣款；唯一）
```

聊天业务任务按 `(conversation_id, request_id)` 唯一；其他任务按 `(platform, user_id, request_id)` 唯一。计费明细额外使用 Run 内调用序号与用量指纹。同一身份、同一指纹重试返回既有任务/结果；同一身份出现不同请求、用量或价格指纹时报冲突，禁止覆盖。

幂等重放的 HTTP 规则固定为：

- 对话、图片和视频的新建或非终态重放返回 HTTP 202 和同一业务任务身份；
- 同步文本、工具草稿和音频首次请求可以等待 finalizer，成功或成功重放返回 HTTP 200；并发重放发现任务仍为 `pending`、`running` 或 `outcome_unknown` 时返回 HTTP 202 和任务身份/当前状态，不等待第二次供应商调用；
- 首次任务已经终态失败时，重放同一稳定错误和任务身份；首次结果不可用时返回 `ai.result_unavailable`，不能创建替代结果；
- 同一 ID 不同指纹始终返回 HTTP 409；不同 request ID 与活动会话命令冲突也返回 HTTP 409，但两者使用不同稳定错误码。

这里的 HTTP 202 是创建、同一请求幂等重放或状态查询的正常“仍在处理”响应，不携带 `ai.provider_outcome_unknown` 错误。只有取消、编辑、重新生成、删除或提交不同新请求等要求当前运行具有确定终态的操作，在命中 `outcome_unknown` 时才返回 HTTP 409 与 `ai.provider_outcome_unknown`；只读查询和同一请求重放仍返回当前任务身份与 `outcome_unknown` 状态。同步入口已经持久化 `staged` 候选、但在本次等待窗口内无法提交 finalizer 时，返回 HTTP 503、`ai.finalization_pending` 和同一任务身份；后续同 ID 重放只能继续本地 finalizer 或返回既有状态，不能重新调用供应商。

所有任务创建/生成响应都返回规范化后的 `request_id`、业务任务 ID 和当前状态；成功结果还返回原业务结果。公共响应本期不新增 `idempotent_replay` 字段，服务端仅记录受控的幂等命中计数；是否命中重放不能改变任务身份、HTTP 状态规则或结果内容。

恢复器先扫描聊天 `staged` 完成候选和各模态已经持久化的结果候选，只重试本地 finalizer；随后扫描超过恢复期限的非终态任务、provider-attempt、账单和冻结。所有扫描使用租约及有界批次幂等恢复。恢复只能依据持久化真实内容/用量结算，或依据已证明失败释放；不能重复调用已知已派发但结果不确定的供应商请求。

成功结果的顺序固定为“结果候选可恢复 -> finalizer 提交业务成功和财务终态 -> 对外返回/发布”。失败路径固定为“证明无可交付成功结果 -> 释放冻结并终结任务/Run/账单”。任何模态都禁止任务先成功而钱包仍未决，也禁止钱包已扣但结果尚无可重放引用。

编辑和重新生成一经服务端接受，就是已提交的可见历史改写。后续调用失败或取消时，新用户消息保留，旧尾部继续软删除，不跨外部调用回滚历史；用户可再次编辑新用户消息提交新请求。

## 18. 错误、安全与可观测性

### 18.1 稳定错误类别

以下值是响应 `error.code` 的稳定机器契约，不是本地化 `msg` 或 message ID：

| `error.code` | HTTP | category | retryable | 含义 |
| --- | ---: | --- | --- | --- |
| `ai.request_id_required` | 400 | `validation` | false | `request_id` 缺失或去空白后为空 |
| `ai.request_id_invalid` | 400 | `validation` | false | 长度或字符格式非法 |
| `ai.request_fingerprint_conflict` | 409 | `conflict` | false | 同一幂等身份携带了不同请求 |
| `ai.generation_in_progress` | 409 | `conflict` | true | 不同新请求与会话活动命令冲突 |
| `ai.pricing_unavailable` | 503 | `dependency` | false | 模型、价格档位、估算或计量能力未就绪 |
| `ai.billing_profile_blocked` | 503 | `dependency` | false | billing safety profile 已被运行安全 block |
| `ai.insufficient_balance` | 409 | `conflict` | false | 执行前可用余额不足 |
| `ai.provider_outcome_unknown` | 409 | `conflict` | true | 已派发请求正在等待权威对账 |
| `ai.finalization_pending` | 503 | `dependency` | true | 结果已形成但本地 finalizer 尚未提交 |
| `ai.result_unavailable` | 500 | `internal` | false | 已成功任务的持久化结果缺失或摘要不符 |

价格模型不存在/别名冲突、所需档位不存在、智能体倍率非法、驱动计量缺失、供应商用量或缓存分类不完整、冻结/结算/用量指纹冲突和恢复失败必须进一步写入账单/Run 诊断码；对外可归入上表稳定类别，但不能只返回模糊中文字符串。

活动命令冲突和请求指纹冲突返回 HTTP 409。消息不存在、已删除、角色错误或不属于当前用户统一返回 404，不能泄露他人资源存在性。空编辑文字返回 400。对象存储暂时不可用、但尚未证明结果丢失时返回普通可重试 dependency 错误；只有确认正文、manifest、对象或摘要事实损坏时才使用不可重试的 `ai.result_unavailable`。

### 18.2 安全与审计

- 所有消息变更、读游标、点赞、账单和钱包读取使用当前登录身份并校验所有权；
- 不接受前端传入 `user_id`、配对消息 ID、Run 归属、模型价格、倍率快照或附件替换；
- 输入快照和消息正文不使用 `v-html`；
- 日志只记录 command、run、charge、hold、provider、model、目录版本、provider request ID、资源 ID、请求 ID 和动作；
- 日志与操作审计不记录 API key、完整提示词、附件 URL 或模型回复正文；
- 软删除不得级联修改 Run、回复命令、运行事件或财务记录。

消息发送/停止、编辑、重新生成、删除、已读游标和当前用户点赞继续使用现有 `Authenticated()` 自助访问策略并在 service 做资源所有权校验，不为 ToC 消息按钮新增 RBAC 权限。只读价格管理页使用 `ai_pricing_list`，平台 Run 监控使用 `ai_run_list`；三者不能混为一套授权规则。

### 18.3 可观测性

指标至少覆盖：

- 回复命令、Run 和账单状态；
- 冻结金额、结算延迟和过期冻结恢复；
- 未计费用量与计量故障；
- 缓存读取 Token 与缓存节省金额；
- 价格解析失败和余额不足；
- request ID 缺失/非法、幂等重放量和请求指纹冲突量；
- provider-attempt outcome-unknown 数量、持续时间、最老年龄、距绝对截止时间和恢复结果；
- `staged` 结果候选数量/年龄、finalizer 重试、孤儿对象和 `ai.result_unavailable`；
- 估算超额次数、补差金额和 active billing-profile block；
- 消息完成到未读查询可见的延迟。

指标标签只允许任务类型、模态、目录版本、规范模型、状态和受控原因；禁止把 user ID、request ID、Run ID、对象 key 或供应商原始错误文本放入指标标签。日志可以记录受控资源 ID 用于关联，但继续遵守第 18.2 节的正文与秘密脱敏规则。

## 19. 数据迁移与发布顺序

迁移只能由版本化脚本执行，不能放进应用启动：

钱包从 cents 切换到 money units 是不兼容的单写者迁移，必须安排维护窗口：先停止旧版 API、AI Worker、支付回调消费者和所有钱包写入者，确认没有在途写事务后制作可恢复备份；再执行迁移与全量校验、部署只读写 money units 的新二进制，最后恢复入口。旧版与新版不得同时写钱包，不做 cents/units 双写。若迁移或校验失败，回滚方式是恢复维护窗口前备份并重新部署旧二进制，不能在开放流量时逆向换算。

1. 增加智能体倍率、会话已读游标、回复命令意图字段、Run 点赞/不确定状态与事件、消息查询索引和聊天完成候选；
2. 将 `ai_provider_attempts` 扩展为 Run 级所有者。现有聊天尝试只允许通过 `ai_runs.idempotency_key = CONCAT('reply-command:', command_id)` 回填，并交叉校验 command/run 的 platform、user、conversation、user message 与 request ID；零条、多条或任一身份不一致都使迁移失败，禁止按时间邻近猜 Run。校验后再把 `run_id` 设为必填，`command_id` 改为可空关联；
3. 为 text/image/video task 增加客户端幂等字段、Run 绑定、业务/结果状态和结果候选字段，新增 tool/audio task；对象结果字段按第 10.10 节回填，不能把 provider 临时 URL 伪装成平台持久化结果；
4. 增加账单、计费明细、钱包冻结、`ai_billing_profile_blocks`、恢复截止时间、约束和索引；
5. 在写入任何 money units 前，对钱包、累计金额、流水金额及变动前后余额的每个旧 cents 值执行非负与 `value <= floor(MaxInt64 / 1_000_000)` 前置校验；任一失败则整次迁移在修改数据前终止；
6. 将每个旧钱包金额从“分”精确乘以 `1_000_000`，逐钱包和汇总校验后删除旧 cents 字段；
7. 钱包流水金额与前后余额按相同规则转换并校验守恒；
8. 校验并嵌入价格目录；
9. 现有智能体倍率回填 `1.0`，未知模型不猜目录映射；
10. 未定价或不可计量模型标记 billing-unready，在供应商调用前失败；
11. 发布统一 reply finalizer、各模态计费接入与恢复 Worker；
12. 发布消息交互、未读、Run 详情和充值页前端；
13. 新增但不授权 `ai_pricing_list`、`ai_run_list`，从编译路由重新生成 Admin Contract Bundle 与前端客户端，Run 状态筛选和事件契约同步加入 `outcome_unknown`。

迁移验收必须证明：

- 每个钱包与流水旧 cents 和新 units 精确相等；
- 迁移钱包 `held_units = 0`；
- 余额、冻结、累计金额和流水金额非负；
- `balance_units - held_units >= 0`；
- 钱包流水变动前后仍守恒；
- 历史回复命令统一回填 `request_kind=send` 和 `request_fingerprint_version=reply_request_v1`，并从其持久化用户消息内容与规范化 `meta_json` 计算稳定指纹；缺失关联消息时迁移失败，不猜数据；
- 每个现有 `ai_provider_attempts.command_id` 都通过 `reply-command:<command_id>` 精确解析到唯一且身份一致的聊天 Run，回填后不存在空 `run_id`、重复 `(run_id, attempt_no)` 或孤儿尝试；
- 旧 text/image/video task 的客户端幂等字段使用唯一 `legacy:` 身份、`request_id_source=legacy` 并标为不可由客户端重放；不得为历史音频或工具调用伪造不存在的任务结果；
- 历史成功文本只有答案正文和新契约要求的摘要事实完整时才能回填 `result_state=finalized`；历史成功图片/视频只有平台控制的持久化对象、输出 manifest 和必要摘要事实都可验证时才能回填 `finalized`；所有无法证明新结果契约的其余历史任务统一回填 `result_state=legacy`，包括只有 provider task ID、临时 URL 或不完整输出行的记录；
- 迁移完成后不存在 `request_id_source=legacy` 却使用 `result_state=none/staged` 的终态任务，也不存在新客户端 `success` 与非 `finalized/unavailable` 结果状态组合；`legacy` 任务不被新 Worker、计费或客户端幂等重放读取；
- 每个旧会话的 `last_read_message_id` 回填为迁移时最新的当前可见 AI 消息 ID，无可见 AI 消息时才为 0，避免上线瞬间把全部历史回复误报为未读；迁移后只由用户阅读动作单调推进；
- 旧 AI 计费表继续不存在；
- `ai_billing_profile_blocks` 初始为空，且不存在绕过 block 的备用计费路径；
- `ai_pricing_list`、`ai_run_list` 权限定义存在，但没有自动新增任何角色权限关系；
- 没有新增清理 `success/finalized` 结果的定时任务或对象存储生命周期规则；
- 维护窗口内旧钱包写入者全部停止，迁移前溢出校验先于任何数据修改，恢复入口后只有 money-units 新版本能够写入。

## 20. 被否决的方案

1. **全局倍率：** 无法让同模型的不同智能体采用不同商业策略。
2. **供应商或供应商模型倍率：** 把产品定价绑定到基础设施，同模型无法按智能体区分。
3. **运行时社区价格源：** 不能作为未经审核的资金事实。
4. **HTTP 接收事务直接冻结：** 最终供应商请求尚未形成，会复制 Worker 组装逻辑并产生过期报价。
5. **供应商返回后直接扣款：** 无法阻止负余额和并发透支。
6. **先发布助手消息、之后 best-effort 扣款：** 会产生免费成功消息或钱包/Run 状态撕裂。
7. **前端本地累计未读：** 刷新、事件重放和多标签页都会漂移。
8. **持久化 `unread_count += 1`：** 删除未读消息后需要额外对账，派生值容易失真。
9. **原地修改旧用户消息：** 破坏旧 Run 的输入审计。
10. **取消 `user_message_id` 一对一约束：** 扩大命令、配对和幂等语义，没有必要。
11. **独立消息版本树或点赞表：** 当前线性可见链和单用户 Run 足以表达需求。
12. **把点赞塞进 `meta_json`：** 点赞评价具体 Run，不属于消息输入元数据。
13. **后端 TTS：** 会增加模型费用和默认音频智能体配置，本期明确使用浏览器朗读。
14. **把 `outcome_unknown` 当作可取消成功：** 无法证明供应商未执行，会破坏对账与资金事实。
15. **用 task 自增 ID 代替客户端幂等 ID：** 只能保护 task 创建后的内部重试，无法阻止 HTTP 响应丢失后的重复付费请求。
16. **只保存音频/视频 provider 引用：** 引用可能过期，不能保证用户重试时拿回已经付费的结果。
17. **所有模态共用一张通用任务表：** 各模态结果、状态和保留语义不同，会形成臃肿的跨域状态机；共享 Run、provider-attempt、计费与钱包能力即可。

## 21. 验证执行边界

本需求改动面较大，但不能因此默认运行耗时巨大的全仓测试。

### 21.1 Codex targeted checks

实施时只自动执行预计单条两分钟内完成的检查，例如：

- 当前 Go package 单元测试；
- 单个 reply-command、wallet 或 usagecharge repository 集成测试；
- 单个前端组件/composable 测试；
- 价格目录 schema 校验；
- `git diff --check`、格式和类型级快速检查。

若预计短命令实际超过两分钟，应停止等待并报告命令与状态，交给用户决定是否继续。

### 21.2 User-run full verification

以下只写入 plan 和最终交付说明，不由 Codex 自动执行：

- `go test ./...`；
- 全仓 `-race`；
- 前端全量 coverage、build、bundle/audit；
- Docker 构建或容器集成；
- 完整 runtime contract、reconciliation 与 release gate；
- Playwright 全套或完整 smoke；
- 任何预计超过两分钟的批量验证脚本。

这些脚本是用户可选的最终信心检查，不作为 Codex 完成单个实施任务的自动前置条件。

## 22. 针对性测试范围

短测试重点覆盖：

- 目录 schema、摘要、别名唯一性、生效价格和官方来源；
- 小于一分钱、最大值、倍率边界、溢出和整单向下取整；
- 官方零价与整单向下取整为零时不创建零金额 hold/流水，但账单仍幂等 `settled`；
- 缓存字段缺失与显式 0、OpenAI inclusive cached tokens、缓存读写拆分和工具轮次汇总；
- 未知模型/档位、计量能力或余额不足时不调用供应商；
- 冻结、补充、捕获、释放、退款和并发幂等；
- 估算上限意外不足时仅在余额足够时补充；余额不足则完整结果 `unbilled` 免费交付，不部分扣款、不透支；估算不足、计量不完整、未知单位/档位和金额指纹漂移都原子写入 profile block，后续实例在供应商调用前 fail closed；
- 账单、回复命令和停止/完成竞态状态机；
- provider 结果候选落库后 finalizer 重试不重复调用供应商，成功后候选正文被清空；
- outcome-unknown 对账期间不允许历史改写且不伪装取消；
- 首版各模态恢复期限、绝对截止时间不可被重启延长、到期资金释放以及迟到结果丢弃；
- 成功聊天原子提交消息、Run、账单、钱包和 durable completed 事件；
- 用量不完整时免费发布有效结果且不重复扣缓存 Token；
- 发送、编辑、重新生成的 request ID 指纹重放与冲突；
- 同步文本、工具草稿、图片、视频和音频创建的 request ID 重放与指纹冲突，HTTP 响应丢失后不创建第二个 task/Run/账单；
- `request_id` 缺失/格式非法、幂等冲突、活动命令冲突、finalizer 待恢复和结果不可用返回各自稳定 `error.code`；
- 所有模态的 provider-attempt 以 Run 和调用序号唯一，`dispatched` 或 `outcome_unknown` 不换 key 盲目重发；
- 不确定冻结在各模态绝对恢复上限内解决，超限自动转 `unbilled` 并释放，不无限占用余额；
- 文本/工具结果、图片文件、平台视频和平台音频在扣款后均可用同一请求身份重放；对象上传、候选提交和 finalizer 任一崩溃点只恢复同一对象/本地提交，不重复生成；
- 已扣款结果确认永久缺失或摘要不符时只转一次 `unavailable`、完整退款一次并稳定返回 `ai.result_unavailable`；临时对象存储故障不误退款；
- `success/finalized` 结果和仍待 finalizer 的 `staged` 候选不被后台 TTL 清理，`discarded`/孤儿对象清理不误删活动或正式结果；
- 任意消息软删除、配对投影、可见上下文与财务记录不变；
- 已读游标单调性、删除未读消息和 WebSocket 重放恢复；
- Run 点赞所有权和删除消息后保留；
- 浏览器朗读状态清理；
- 输入快照纯文本、附件 JSON、嵌套 `meta_json` 和异常回退；
- cents 到 units 的非负/溢出前置校验、维护窗口单写者切换和精确迁移，以及 provider-attempt 通过 `reply-command:<id>` 精确回填 Run；
- 历史任务只能在完整证明新结果契约时回填 `finalized`，其余使用不可重放的 `legacy`，且不存在迁移后的 `success + none/staged`；
- 智能体价格预览、缓存明细、高精度金额和权限显示；
- `ai_run_list`/`ai_pricing_list` 管理权限与当前用户自助所有权接口分离，且迁移不自动授权；
- 收银台移除最近充值但充值记录保持正常。

## 23. 验收标准

1. 官方 `$5 / M Token` 结算为 `¥5 / M Token`，不存在 FX 配置或隐藏换算。
2. 管理员只能修改智能体倍率，不能修改模型基础价。
3. 同模型的两个智能体可采用不同倍率；切换模型后倍率不变。
4. 普通输入、缓存写入、缓存读取、输出和媒体用量形成独立不可变明细。
5. 缓存 Token 使用官方缓存价且不重复进入普通输入；缓存明细缺失不按高价扣用户。
6. 小于一分钱的消费能精确改变钱包，不设置一分钱最低消费。
7. 可用余额不足时不调用供应商，并给用户明确充值入口。
8. 同一客户端 `request_id` 重试后仍只有一个业务任务、一个 Run 身份和一张账单；非零 `settled` 费用最多一次有效冻结捕获和一条扣款流水，零金额不写零 hold/流水，失败或 `unbilled` 不写扣款；同 ID 不同请求返回冲突。
9. 失败、取消、超时、超过不确定恢复上限或无法恢复准确用量时不捕获估算金额，也不无限冻结用户余额。
10. 发送、编辑和重新生成在 Worker 调用前完成价格快照与冻结，成功结果统一原子完成。
11. 编辑只改变文字；附件和运行参数不变；新 Run 使用当前模型与倍率。
12. 编辑和重新生成产生新消息、命令、Run 和账单，旧审计与财务事实不变。
13. 用户可默认按问答对选择，也可只删除任意一条；删除不影响旧 Run、点赞或账单。
14. 同一会话最多一个活动命令；活动期间前后端同时阻止继续发送、消息改写和整会话删除。
15. `outcome_unknown` 等待权威对账，不伪装为已停止，不产生未读或提前扣款。
16. 点赞可切换并在对应 Run 详情持久展示。
17. 朗读完全在浏览器执行，不调用后端且不计费。
18. 非当前会话完整回复产生准确未读数，进入会话后清零，断线重放不重复。
19. AI Runs 同时展示结构化输入、点赞、计费分类、缓存折扣和高精度费用。
20. 图片、视频和音频按模型真实官方单位计费，不回退到场景固定价。
21. 同步文本、工具草稿、图片、视频和音频的成功结果都可由首次请求身份重放，不因响应丢失重复调用供应商或扣款。
22. 所有真实供应商调用都有 Run 级 attempt 身份；不确定结果不得换 key 盲目重发。
23. `staged` 对象不能被用户读取，finalizer 只发布已验证的私有对象；成功结果本期没有自动 TTL，永久结果损坏返回稳定错误、完整退款且不重新生成。
24. 每个冻结都有不可被重启延长的绝对恢复上限；超过上限后不扣估算金额、不无限占用余额，迟到结果也不反转终态。
25. 任一估算上限突破、计量不完整、未知单位/档位或金额指纹漂移都会形成跨实例 billing-profile block，修正并显式升级 safety revision 前同一 `billing_safety_key` 不再调用供应商。
26. 后续价格、模型或倍率变化不能修改历史账单快照或同一请求的首次执行结果。
27. 收银台不再出现最近充值，充值记录与继续支付不受影响。
28. 旧 AI 计费表继续不存在，且没有新增密钥/环境变量、自动角色授权或已退役 Admin/Canvas AI transport。
29. 实施只自动执行短而针对性的测试，长脚本明确交由用户手动运行。
30. 钱包单位切换在停止全部旧写入者的维护窗口内完成，任何 cents 值溢出风险都在修改数据前失败，发布后不存在 cents/units 混写或双写。
