# AI 上下文工程执行索引 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 按已批准规格把旧知识库替换为可审计、可重建、不会破坏现有聊天终态的上下文工程，并在最终切换后只保留九张 Context 业务表。

**Architecture:** MySQL 保存唯一业务真相和不可变 Context Plan，Qdrant 只保存可重建的 Dense/Sparse 派生索引，Redis/Asynq 只承载有界后台任务。聊天在 Provider 报价和 Prepared Request 之前固定唯一终局 Plan；同一 Run 的 Provider 重试只重放已持久化请求字节，所有 Context 故障复用现有 Run、Attempt、钱包和 finalizer 收口。

**Tech Stack:** Go 1.26.5、Gin、GORM、MySQL 8.4、Atlas、Redis/Asynq、Qdrant、Vue 3.5、TypeScript 5.9、Element Plus、Vitest、Admin Contract Bundle。

---

## Canonical Inputs

- 唯一业务规格：`docs/superpowers/specs/2026-08-01-ai-context-engineering-design.md`。
- 后端规格基线：`6fcc5248a22d3cb4dc4f09e99665ff697be7e6c5`。
- Admin 前端核对基线：`09a3cdd0d13208bc31b7d2b5c97b0e376f9c60b4`。
- 后端根目录：`E:/admin/admin_back_go`。
- Admin 前端根目录：`E:/admin/admin_front_ts`。
- 实施前必须重新读取后端 `AGENTS.md`、`docs/architecture.md`、`internal/module/README.md`、`internal/platform/README.md`，以及前端 `docs/rule.md`。
- 如果任一仓库 HEAD 已前移，先审查上述基线到当前 HEAD 的提交和 changed paths；schema、AI runtime、Contract、generated types、chat、Run 或附件链发生漂移时，先修订受影响计划，再开始实现。
- `E:/admin/LONG_TASK_PARALLEL_EXECUTION.md` 当前不存在，不是本次权威输入。

## Non-Negotiable Invariants

```text
business truth:     MySQL only
derived index:      Qdrant only, always rebuildable
run plan:           zero or one immutable terminal plan per run
ready plan hash:    required
failed plan hash:   SQL NULL
provider retry:     persisted request bytes only; never retrieve again
required atoms:     system/current message/current attachments/tool protocol/output reserve
history atoms:      complete turns and complete tool call/result groups
context failure:    existing finalizer path; never leave running
memory call:        at-least-once across pre-commit crash; terminal chain never forks
new context tables: exactly nine
forbidden tables:   citation/retrieval/hit/job/cursor status tables
runtime dual-write: forbidden
```

保留现有用户可见行为：消息持久化、WebSocket 流式与刷新恢复、原生附件、Prepared Request、Run/Attempt 证据、计费冻结/释放、停止后的部分回答和统一 finalizer。旧 `meta_json.runtime_params.max_history` 只保留历史读取兼容，新代码忽略其行为；最终 OpenAPI 和前端不再发布该字段。

## Fixed Database And Permission Contract

迁移文件名固定且只能出现一次：

```text
database/migrations/202608020101_ai_context_expand.sql
database/migrations/202608020102_ai_context_permissions.sql
database/migrations/202608020103_ai_context_contract.sql
```

Expand 只创建以下九表：

```text
ai_context_profiles
ai_context_spaces
ai_context_documents
ai_context_document_versions
ai_context_chunks
ai_context_bindings
ai_context_plans
ai_context_plan_items
ai_conversation_memories
```

权限身份固定为：

```text
923 ai_context_view
924 ai_context_manage
925 ai_context_document_manage
926 ai_context_profile_manage
927 ai_context_evaluate
```

菜单复用 ID `122` 以保留既有菜单授权，最终身份固定为 `path=/ai/context`、`component=ai/context`、`i18n_key=menu.ai_context`。权限迁移把旧知识权限的角色授权确定性映射到新权限后再删除旧按钮权限；不得自动给从未拥有旧知识权限的角色扩大授权。

## Plan Files And Dependency Order

| Wave | Plan | 交付事实 | Depends on |
| --- | --- | --- | --- |
| 0 | `2026-08-02-ai-context-engineering-01-core-contract.md` | 九表 Expand、闭合领域类型、typed `ChatInput`、预算/Packer/Hash、Model Kind、Agent Profile、Plan/Attempt 证据 | none |
| 1 | `2026-08-02-ai-context-engineering-02-ingestion-qdrant.md` | 文档解析/分块、唯一 Sparse 编码器、Embedding、Qdrant、摄取 Worker、重建、Space-source readiness | 01 |
| 2 | `2026-08-02-ai-context-engineering-03-retrieval-chat.md` | canonical ConversationTurn、QueryBatch Dense/Sparse/RRF/Rerank、MySQL 复核、BuildPlan、聊天、Citation、Run 投影 | 01, 02 |
| 3 | `2026-08-02-ai-context-engineering-04-memory-attachments.md` | Conversation Turn 分页/私有索引、历史附件失权闭环、自动历史、Rolling Memory、忽略有效 `max_history` | 03 |
| 4 | `2026-08-02-ai-context-engineering-05-admin-cutover.md` | Tasks 1-10：Admin API/UI、Agent 绑定、评测、Contract、旧代码/页面/权限/六表删除 | 01-04 |

每个 Wave 的主分支 checkpoint 必须通过该计划的定向测试后提交。Wave 0-3 只允许 Expand 和内部切换，不执行破坏性 SQL；Wave 4 的 Contract migration 也只创建并验证文件，本次自动执行不得连接或修改用户的 live database。

## Shared File Ownership

| Area | Owner | Rule |
| --- | --- | --- |
| `database/schema/admin.hcl`, `database/migrations/atlas.sum` | 主线程串行 | Plan 01 Expand；Plan 05 permissions/Contract；任何时候只有一个写入者 |
| `internal/infra/ai/**`, `internal/infra/ai/openaicompat/**` | Plan 01 then Plan 02/03 adapters | typed request 先落地；Embedding/Rerank 只能扩展已固定接口 |
| `internal/module/ai/contextengine/**` | Plan 01-04 按任务依赖串行 | 领域类型和 Repository 先于 ingestion/retrieval/memory；不得拆出第二模块复制状态机 |
| `internal/platform/admin/**`, `internal/runtime/**`, `internal/jobs/**`, `internal/server/**` | 主线程 integration owner | capability executor 不修改共享 composition；返回接口后由主线程一次接线 |
| `contracts/admin/v1/**` | Plan 05 主线程 | 组合后端先提交并 clean，再用该 SHA 生成，不手写 artifact |
| `E:/admin/admin_front_ts/contracts/backend/admin/**` 与 generated types | Plan 05 主线程 | 只从已提交后端 Bundle 同步和生成；Tasks 6-9 保持一个未提交原子工作树 |
| `E:/admin/admin_front_ts/src/views/Main/ai/context/**` | Plan 05 UI lane | 只消费 generated DTO；不定义备用响应类型 |
| Chat、Run、Agent 前端现有文件 | Plan 05 主线程 | Contract 同步后连续完成 Tasks 6-9，第一次全量 typecheck 通过后才提交；禁止生成旧 Knowledge alias |

## Cross-Plan Type Contract

后续计划统一使用以下名字；改名必须同时更新规格、五份计划、后端 Contract 测试和前端 generated consumer：

```go
type ContextRuntime interface {
	BuildPlan(context.Context, BuildPlanInput) (ContextPlan, *apperror.Error)
	GuardDispatch(context.Context, DispatchGuardInput) *apperror.Error
}

type RetrievalOutcome string

const (
	RetrievalSkipped RetrievalOutcome = "skipped"
	RetrievalNoHit   RetrievalOutcome = "no_hit"
	RetrievalHit     RetrievalOutcome = "hit"
	RetrievalFailed  RetrievalOutcome = "failed"
)

type PlanState string

const (
	PlanReady  PlanState = "ready"
	PlanFailed PlanState = "failed"
)
```

```go
type Message struct {
	Role  MessageRole
	Parts []ContentPart
}

type ContentPart struct {
	Kind       ContentPartKind
	Text       string
	Attachment *AttachmentRef
}
```

```text
policy version:       context_policy_v1
plan hash schema:     context_plan_v1
metrics schema:       context_plan_metrics_v1
metadata schemas:     context_block_metadata_v1, retrieval_branches_v1
locator schema:       context_locator_v1
conversation hash:    conversation_turn_v1
sparse encoder:       unicode_lexical_v1
```

任务类型固定为：

```text
ai:context-document-index:v1
ai:context-index-cleanup:v1
ai:context-conversation-index:v1
ai:context-memory-build:v1
ai:context-profile-rebuild:v1
```

## Baseline Gate

- [ ] 在两个仓库分别记录 `git rev-parse HEAD` 与 `git status --short --branch`；来源不明的重叠修改先审查，不能覆盖或回退。
- [ ] 运行 `go version`，确认输出为 Go `1.26.5`；运行 `node --version` 与 `npm --version`，确认满足前端 `package.json` engines。
- [ ] 运行 `go test ./internal/module/ai/chat ./internal/module/ai/replycommand ./internal/runtime -count=1`，记录现有聊天、回复租约、Attempt 与 finalizer 基线。
- [ ] 运行 `npm test -- --run tests/component/ai/ChatAttachments.test.ts tests/component/ai/RuntimeParamsPanel.test.ts tests/component/ai/RunInputSnapshot.test.ts`，记录附件、运行参数和 Run UI 基线。
- [ ] 确认没有启动、停止或重启 `admin-dev`；所有自动测试使用 package test、fixture、fake upstream 或临时 Qdrant 容器。
- [ ] 确认本次不使用 Playwright；浏览器行为由用户在最终切换后人工验收。

## 24-Section Design Coverage Matrix

| Spec section | Implemented by | Acceptance evidence |
| --- | --- | --- |
| 1 最终结论 | 01 Tasks 1-7；05 Tasks 1-10 | 九表、typed request、最终旧模块删除检查 |
| 2 当前问题与根因 | 03 Task 4；04 Task 2；05 Tasks 3, 4, 9 | 不再 `strings.Contains`、不污染 user text、不再执行 `max_history` |
| 3 目标与非目标 | 02 Tasks 3-7；03 Tasks 1-7；04 Tasks 1-6 | 六格式摄取、混合检索、自动历史；OCR/GraphRAG 不进入依赖图 |
| 4 开源组件采用决策 | 02 Tasks 1-3 | 固定 Go modules；真实 Qdrant 能力测试后锁 RepoDigest |
| 5 核心不变量 | 01 Tasks 2, 4, 6；03 Tasks 3-5 | MySQL authority、唯一终局 Plan、原子组、严格失败测试 |
| 6 架构边界 | 01 Task 2；02 Tasks 2-6；05 Task 2 | module/infra/runtime/transport imports 审计 |
| 7 Context Plan 数据结构 | 01 Tasks 2, 4 | 闭合类型、canonical hash golden、无 NaN/Infinity |
| 8 最小数据库模型 | 01 Tasks 1, 2, 5, 6；05 Task 4 | HCL/migration 结构测试精确等于九表和必要三处扩展 |
| 9 Qdrant 设计 | 02 Tasks 1, 2, 7；03 Tasks 1, 2；04 Tasks 3, 4 | generation、UUIDv8 point、payload、filter、Conversation cleanup/rebuild 测试 |
| 10 文档摄取 | 02 Tasks 3-6；04 Task 4 | parser golden、lease fencing、幂等 chunk/point、Conversation source 可见性与激活顺序测试 |
| 11 检索管线 | 02 Task 4；03 Tasks 1, 2 | 唯一 Sparse encoder、query 去重、RRF/Rerank、MySQL batch authority 测试 |
| 12 Token Budget 与 Packing | 01 Task 4；03 Task 3 | 属性测试证明预算不等式和原子组不拆分 |
| 13 会话记忆与历史附件 | 03 Task 1；04 Tasks 1-6 | 唯一 Turn hash/text、稳定分页、私有索引、附件失权/补建、Memory 链与失效测试 |
| 14 Chat/Gateway/Run 集成 | 01 Tasks 3, 6；03 Tasks 3-5 | Plan-before-quote、Attempt evidence、retry no-retrieval、dispatch guard |
| 15 错误码与重试 | 02 Tasks 5-7；03 Tasks 3-7；04 Tasks 3-5 | permanent/transient Asynq 分类和 Run 终态矩阵 |
| 16 Admin API 与权限 | 02 Task 5；05 Tasks 1-3 | route policy、闭合 DTO、923-927、Citation/Run Contract tests |
| 17 前端信息架构 | 05 Tasks 6-9 | `/ai/context` 工作台、Agent binding、Citation drawer、Run Plan 与原子生成物切换 |
| 18 可观测性与安全 | 02 Task 7；03 Tasks 2, 7；04 Tasks 3, 4 | source-aware readiness 逐来源扩展、低基数指标、日志/payload 泄漏扫描 |
| 19 Docker 与配置 | 02 Tasks 1, 2 | loopback port、network、healthcheck、四个 Qdrant env keys |
| 20 迁移与旧模块删除 | 01 Task 1；05 Tasks 1, 3, 4 | Expand/permissions/Contract guards，旧六表非空时 `SIGNAL` |
| 21 实施切片 | 本索引 Waves 0-4 | 五个 clean checkpoints 和显式依赖顺序 |
| 22 测试与评估 | 各计划 verification task；03 Task 7；05 Tasks 2, 10 | unit/property/integration/offline evaluation/frontend gates |
| 23 验收清单 | 05 Task 10 | 逐项机器检查加用户浏览器人工验收清单 |
| 24 规范关系 | 03 Tasks 4-7；04 Tasks 2, 4；05 Tasks 3, 4, 9 | 原生附件、停止交付、旧知识假设的回归测试和文档更新 |

## Cutover Gates

最终 Contract 前必须在维护窗口对目标环境执行只读 preflight；任一断言失败就中止，不修改数据库：

```sql
SELECT COUNT(*) FROM ai_reply_commands
WHERE status IN ('claimed', 'running', 'outcome_unknown');

SELECT COUNT(*) FROM ai_provider_attempts
WHERE state IN ('prepared', 'dispatched', 'outcome_unknown');

SELECT 'ai_knowledge_bases' AS table_name, COUNT(*) AS row_count FROM ai_knowledge_bases
UNION ALL SELECT 'ai_knowledge_documents', COUNT(*) FROM ai_knowledge_documents
UNION ALL SELECT 'ai_knowledge_chunks', COUNT(*) FROM ai_knowledge_chunks
UNION ALL SELECT 'ai_agent_knowledge_bases', COUNT(*) FROM ai_agent_knowledge_bases
UNION ALL SELECT 'ai_knowledge_retrievals', COUNT(*) FROM ai_knowledge_retrievals
UNION ALL SELECT 'ai_knowledge_retrieval_hits', COUNT(*) FROM ai_knowledge_retrieval_hits;
```

以上八个计数必须全部为 `0`。另由 Plan 05 的 Go preflight 枚举全部 enabled Chat Agent，逐一验证 Provider、Model、API Protocol、`context_window_tokens`、最大输出和注册 Token Counter；任何缺失报告具体 ID 并中止，禁止填默认窗口。

## Execution Protocol

- [ ] 每个 Task 严格执行“写失败测试 -> 确认因目标能力缺失而失败 -> 最小实现 -> 同一测试通过 -> 定向回归 -> 显式 pathspec 提交”。
- [ ] 不使用 `git add -A` 或 `git add .`；共享 schema、runtime composition、Contract 和 generated outputs 只由主线程串行提交。
- [ ] Qdrant Server 候选固定为 `qdrant/qdrant:v1.18.3`，但它在测试前只是候选，不能直接进入 Compose；`github.com/qdrant/go-client v1.18.3` 的同版本号也不构成 Server 能力证据。Plan 02 必须真实拉取该候选并通过 Sparse IDF、QueryBatch、RRF、Filter 测试，之后才允许把实测镜像的 `RepoDigests[0]` 写成 `qdrant/qdrant:v1.18.3@sha256:digest`。
- [ ] Plan 05 必须先完成 Task 4 的 guarded Contract DDL、旧模块精确删除、全套回归和 clean backend commit；Task 5 才能从该已提交组合 runtime SHA 生成 Bundle。前端随后从同一 SHA 同步。禁止从 Task 3 checkpoint 或脏工作树生成，也禁止手写 OpenAPI、permissions、views 或 generated TypeScript。
- [ ] 前端 Contract 同步会删除旧 Knowledge operation；Plan 05 Tasks 6-9 必须连续完成且只产生一个最终绿色提交。中间不得提交、运行全量 typecheck 后声称通过，或添加手写/generated compatibility alias。
- [ ] 不修改 `database/legacy-migrations/20260510_ai_knowledge_rag.sql`；它只保留历史审计。
- [ ] 不对 live database 执行三份迁移；实际应用迁移、恢复验证和浏览器验收由用户明确安排。
- [ ] 不启动、停止或重启 `admin-dev`，不运行 Playwright。

## Completion Evidence

每个 Wave 返回以下证据后才能进入下一 Wave：

```text
git status --short
git diff --check
exact changed path list
RED command and expected missing-behavior failure
GREEN command and PASS output
targeted regression command and PASS output
commit SHA
```

最终 Wave 额外要求后端 `scripts/verify-backend.ps1`、前端 `npm run verify:frontend`、三份 migration checksum、Admin Contract clean check、旧符号零匹配，以及用户对 `/ai/context`、Chat Citation 和 Run Detail 的人工浏览器验收。
