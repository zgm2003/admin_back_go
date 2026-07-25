# AI Gateway 与钱包结算阶段 A 执行索引

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement these plans task-by-task. Each step uses checkbox syntax and must be checked only after the stated command succeeds.

**Goal:** 按唯一规范性 Spec 完成阶段 A 的官方数值定价、money units 钱包、Run 级 Hold、Gateway 用量归一化、聊天停止结算和媒体任务结算，并保持模块化单体边界。

**Architecture:** 继续使用进程内 `aigateway`，由 durable Runner 持有付费执行权。钱包只处理整数余额、Hold、capture/release 和流水，pricing 只处理只读官方目录与报价，provider adapter 只处理 `base_url + api_key` 协议，业务模块通过小接口调用 Gateway。不会拆微服务、不会新增密钥、不会复制 Sub2API。

**Tech Stack:** Go、GORM、MySQL/Atlas HCL migrations、Redis task queue、OpenAI-compatible HTTP、Admin Contract Bundle、Vue/TypeScript 前端。

---

## Canonical Inputs

- 业务唯一依据：[2026-07-24-ai-chat-consumer-pricing-wallet-design.md](../specs/2026-07-24-ai-chat-consumer-pricing-wallet-design.md)。
- `2026-07-24-ai-chat-consumer-pricing-wallet-spec-review.md` 与 `*-second-review.md` 只保存历史审查证据，不得作为第二份需求来源。
- 本索引只覆盖阶段 A。消息选择、编辑、重新生成、未读、点赞和浏览器朗读另开阶段 B 计划。
- 后端 agent 开始前读取 `AGENTS.md`、`docs/architecture.md`、`internal/module/README.md` 和 `internal/platform/README.md`；Plan 07 前端步骤还必须读取 `E:\admin\admin_front_ts\docs\rule.md`。

## Plan Files

| Wave | Plan | Ownership | Depends on |
| --- | --- | --- | --- |
| 0 | `2026-07-25-ai-chat-consumer-pricing-wallet-01-schema-shared-contracts.md` | Atlas schema、迁移、跨模块状态/接口契约 | none |
| 1 | `2026-07-25-ai-chat-consumer-pricing-wallet-02-money-wallet-holds.md` | money units、充值/兑换写入收口、钱包行锁、Hold、capture/release、AI 退款残留删除 | 01 |
| 1 | `2026-07-25-ai-chat-consumer-pricing-wallet-03-pricing-agent-config.md` | 官方目录、整单一次取整/明细分摊、智能体倍率和最大输出 | 01 |
| 1 | `2026-07-25-ai-chat-consumer-pricing-wallet-04-aigateway-provider-usage.md` | Gateway、provider adapter、分类 usage、幂等 attempt | 01 |
| 2 | `2026-07-25-ai-chat-consumer-pricing-wallet-05-chat-drain-settlement.md` | 聊天 Runner、取消 drain、重试、unknown、统一 finalizer | 02、03、04 |
| 2 | `2026-07-25-ai-chat-consumer-pricing-wallet-06-media-settlement.md` | 文本/工具/图片/视频/音频的任务级 Gateway 接入 | 02、03、04 |
| 3 | `2026-07-25-ai-chat-consumer-pricing-wallet-07-runtime-contract-frontend.md` | runtime 装配、Admin 权限/契约、前端停止状态与展示 | 05、06 |

## Dependency Waves

### Wave 0: 串行共享事实

只允许一个 agent 修改 `database/schema/admin.hcl`、`database/migrations/atlas.sum` 和新增迁移文件。先确定列名、状态值、非空唯一键和迁移闸门，再让后续计划使用这些名字。

### Wave 1: 三条独立实现线

钱包、pricing/agent、Gateway/provider 可并行，但 Plan 04 必须只依赖 Plan 01 的共享契约和自己定义的小接口；若实施者需要直接引用 Plan 02/03 的具体类型，则把 Plan 04 延后到 02/03 合并后，不得用临时重复类型绕过依赖。它们不得修改对方的包；`internal/infra/ai/types.go` 由 Plan 04 单独拥有。

### Wave 2: 两条业务接入线

聊天和媒体可并行，但不得同时修改 `internal/platform/admin/build.go`、`internal/runtime/worker.go` 或 `contracts/admin/v1/*`。两条线通过 Plan 04 的 Gateway 接口接入。

### Wave 3: 串行装配和契约同步

Plan 07 最后修改 runtime、权限、编译后契约和前端 generated contract；它必须在后端接口稳定后运行同步脚本。前端全量 build、Docker、E2E 由用户手动运行。

## Shared File Ownership

| File/area | Owner | Rule |
| --- | --- | --- |
| `database/schema/admin.hcl` | Plan 01 | 后续计划不得直接改 schema |
| `database/migrations/*`、`atlas.sum` | Plan 01 | 迁移只在维护窗口人工执行，不创建备份库 |
| `internal/shared/money/*` | Plan 01 | 唯一 money-units 常量、checked cents 换算和规范人民币字符串格式；其他计划只复用 |
| `internal/module/payment/*` 钱包写入路径、`payment/wallet/*` | Plan 02 | 订单/充值/套餐可保留渠道 cents；钱包和流水只能由 `payment/wallet` 以 units 写入 |
| `internal/module/ai/pricing/*`、agent config | Plan 03 | 只读官方目录，不接受供应商倍率 |
| `internal/infra/ai/types.go`、chat/base adapter contracts | Plan 04 | 删除金额浮点，统一分类 usage；Plan 06 只补媒体方法 |
| `internal/module/ai/aigateway/*` | Plan 04 | Run/attempt/settlement 公共边界，后续只调用公开接口 |
| `replycommand/attempt.go`、attempt persistence adapter | Plan 04 then 05 | Plan 04 完成 Run 级迁移，Plan 05 只接 Runner/finalizer |
| `internal/module/ai/replycommand/*`、`chat/*` 其余文件 | Plan 05 | 只处理聊天 durable execution 和 delivery/drain |
| `internal/module/realtime/event.go` 的 AI failed payload | Plan 05 | 为已接受 chat 命令发布余额不足 machine code；Plan 07 只生成契约 |
| `internal/module/ai/image/*`、`video/*`、`audio/*`、`tool/*`、`text/*` | Plan 06 | 只做任务型 Gateway 接入和结果存储 |
| `internal/platform/admin/build.go`、`internal/runtime/worker.go` | Plan 02 then Plan 07 | Plan 02 只接入 payment wallet participant；Plan 07 在依赖合并后完成 AI Gateway/worker 装配 |
| Run 详情源、`contracts/admin/v1/*`、`admin_front_ts/src` | Plan 07 | 后端先发布字段，再从编译产物同步；工具生成也必须提供稳定 `request_id` |

## Execution Protocol

1. 每个 agent 只执行一个 Plan；先读取仓库规则、本 Plan、主 Spec 和依赖 Plan 的已合并接口，再开始 TDD 步骤。
2. 同一 Wave 的并行 agent 必须使用 `superpowers:using-git-worktrees` 建立独立 worktree；禁止在同一工作树并发提交，禁止宽泛 `git add` 带入其他 Plan 的改动。
3. 每个任务按“失败测试 -> 针对性实现 -> 同一测试通过 -> 小提交”推进；不得把全仓测试当作前置条件。
4. 所有真实上游调用都必须走统一顺序：`claim/lease（不跨阶段持有业务行锁） -> load immutable Run config -> assemble exact request -> quote -> atomic ReserveAndPrepare（Run/Charge -> wallet/Hold -> 保存 exact request/hash/quote） -> mark dispatched -> provider -> usage -> finalizer`。恢复已有 `prepared` attempt 时必须复用已存请求、报价和 attempt key，不得重新组装、改号或覆盖证据。
5. 跨模块计费锁序固定为 `Run -> Charge -> wallet -> Hold`，仅钱包/Hold 操作为 `wallet -> Hold`；禁止先锁 Hold 再锁 wallet，也禁止 Runner 持 command/task 行锁进入计费事务。
6. 钱包余额永不为负；usage 完整性按整个 Run 的全部可收费 attempt 判断，失败 attempt 永远只审计；缺失、整体上游失败或 unknown 时释放 Hold，不猜 Token、不透支、不退款。后续 top-up 失败不派发新 attempt，但已完整 succeeded 的前序 usage 必须从现有 Hold 结算。
7. 聊天停止只关闭 delivery sink；后台继续读同一流。没有完整 usage 时最终释放，不能把 `canceled` 直接当免费终态。
8. 任何迁移前先停钱包写入者并执行前置校验；应用启动不执行迁移，不创建备份数据库。

### Migration cutover gate

`202607250101` 至 `202607250104` 只能在 Plan 05、06、07 合并并部署后，于
维护窗口按 expand -> backfill -> contract -> permissions 顺序执行。contract
之前必须证明所有 paid Run writer 已经走 Gateway acceptance/finalizer：旧的
`RunRecorder.Start`、旧 task `Start` 和 command-owned attempt writer 不得再创建
新 Run；新写入必须包含真实 request fingerprint、pricing snapshot、billing
status/reason、Run owner 和 attempt evidence。`ai_billing_migration_metadata.phase`
是半执行恢复闸门；任何 `started` 阶段都必须按
`docs/database/ai-billing-migration-recovery.md` 人工核对，不能盲目重跑或强行标记
完成。

## Acceptance Gates

- 余额不足、未定价、无安全输入上界：上游零调用；首次余额不足固定收尾为 `failed + released + released_insufficient_balance`，同步 task-wait 入口返回 HTTP `409`，已接受 chat 命令发布同 machine code 的 durable failed event。
- 并发 reserve/top-up/capture/release 后，`balance_units - held_units >= 0`，且钱包 `held_units` 严格等于其全部 active Hold 金额之和。
- 同一 `request_id` 只能返回原 Run/结果；不同请求内容复用 ID 返回 `409`。
- 一个 Run 只有一个 Charge、一个 Hold 和最多一条最终 AI 扣款流水。
- AI 扣款流水固定以 `source_id=run_id` 关联 Run，并保存不含私密请求信息的模型/智能体摘要；管理流水和个人资金明细均显示该 Run 身份与摘要。
- 完整 usage 只结算一次；usage 不完整或异常超出 Hold 时释放、标记 unbilled 并丢弃结果候选，不补扣也不免费发布完整结果。
- 报价先精确汇总全部 rational item、乘 PPM 后整单只向上取整一次；明细最大余数分摊后严格等于总额；aggregate input 与 cache subset 必须先拆成互斥分类。
- 余额不足返回 `ai.billing.insufficient_balance` 和 `/profile/wallet`、`/payment/recharge`，且没有新 attempt/provider call；continuation top-up 失败只结算此前完整 succeeded usage。
- 停止 HTTP 响应只返回 `status="stopping"`；此后不再发送 delta，但 drain 能读完 usage。完整 usage 可收费且正文不发布，finalizer 提交后才发布 durable canceled 终态。
- 图片/视频只有文档化取消 API 和完整 usage 时结算；音频无权威 usage 时 fail closed。
- 已结算结果没有 AI 退款路径，`SourceAIRefund` 及残留被删除。
- 新权限仅为 `ai_run_list`：只注册定义，由管理员手动挂载，不写 `role_permissions`。
- Plan 03/05 必须共同证明 Agent runtime projection 读取并持久化
  `billing_multiplier_ppm`、`max_output_tokens`；Plan 05/06 必须共同证明 chat/media
  Run acceptance 写入真实 fingerprint/pricing/billing 四组必填事实。

## Verification Budget

每个 Plan 只自动运行其列出的包级 `go test`、`go test -run`、`go vet` 或 `git diff --check`，单条命令预计不超过两分钟。以下命令不由 agent 自动启动，交付时仅提供给用户手动运行：

```powershell
go test ./...
go test -race ./...
cd ..\admin_front_ts; npm test; npm run build
docker compose ...
```

完成全部 Wave 后，用户再决定是否执行全仓、race、前端全量、Docker 和真实上游验收。
