# AI 官方模型、工具与交互改造 Implementation Plan Index

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Do not dispatch subagents. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 按可验证的顺序恢复聊天工具调用，建立官方模型唯一信源，统一 Agent/聊天能力交互，并补齐延迟持久化证据。

**Architecture:** 四个计划按依赖顺序执行。工具恢复可先独立上线；官方模型后端改造与前端能力交互属于同一个不可拆分发布窗口；延迟观测最后接入稳定后的请求、计费和工具链路。

**Tech Stack:** Go、Gin、GORM、MySQL 8、Atlas、OpenAI-compatible Adapter、Vue 3、TypeScript、Element Plus、Vitest、Playwright。

**Spec:** `docs/superpowers/specs/2026-07-28-ai-chat-capability-tools-interaction-design.md`

---

## 执行规则

- [ ] 使用 `superpowers:executing-plans` 顺序执行，不并行修改共享 AI 运行链路。
- [ ] 每个行为修改先增加定向失败测试，确认预期 FAIL 后再写实现。
- [ ] 不自动创建 Git commit；每个阶段结束只报告变更、测试结果和建议提交边界。
- [ ] 不修改 `internal/module/ai/knowledge/**`、知识库表、知识绑定接口或当前 RAG 拼接算法。现有 `KnowledgeRuntime` 保留原状，它不是本期知识库修复。
- [ ] 不保留旧 API、旧 RBAC、旧前端路由、旧表名、双写或兼容层。历史 Run 的只读快照解析可以保留，不能接受新的旧格式请求。
- [ ] 工具仅执行 `DefaultExecutors` 中已注册的服务端 executor，不解释或执行模型生成的任意代码。
- [ ] 第二、三计划必须在同一发布窗口完成。只完成后端改名、数据库结构或契约时不得部署。
- [ ] 正式 Admin Contract Bundle 只有在用户批准并产生后端提交后才发布；开发阶段可以生成临时目录验证 OpenAPI，但不得把错误的 `backend_commit` 写入正式 bundle。

## 执行顺序

```text
01 工具调用运行时修复
  -> 02 官方模型唯一信源、全链路改名、最终数据库结构
  -> 03 官方模型 / Agent / 聊天能力交互
  -> 04 Run 延迟持久化观测与有证据的本地优化
```

### 计划 01：工具调用运行时修复

文件：`docs/superpowers/plans/2026-07-28-ai-official-model-tools-interaction-01-tool-runtime-repair.md`

独立完成条件：

- Worker 和 Admin API 使用同一默认 executor 构造边界；
- Worker 缺少 `ToolRuntime` 时构造失败；
- 只有启用、已绑定、低风险、executor 已注册且 Schema 有效的工具进入 Provider 请求；
- 参数和结果均通过 JSON Schema 校验；
- `admin_user_count` 完成工具调用、二次模型请求、真实 usage 结算和审计。

### 计划 02：官方模型唯一信源与后端最终结构

文件：`docs/superpowers/plans/2026-07-28-ai-official-model-tools-interaction-02-official-model-single-source.md`

完成后先不要部署。该计划会一次性完成后端领域更名、官方目录、供应商映射、Agent 最大输出删除、系统安全输出上界、附件权威校验、数据库最终结构、RBAC 和 Admin API。

### 计划 03：Agent 与聊天能力交互

文件：`docs/superpowers/plans/2026-07-28-ai-official-model-tools-interaction-03-agent-chat-capability-ui.md`

本计划必须紧接计划 02，在同一发布窗口同步正式契约并完成前端。完成后才允许部署计划 02 和 03。

### 计划 04：延迟观测与诊断

文件：`docs/superpowers/plans/2026-07-28-ai-official-model-tools-interaction-04-run-latency-observability.md`

先持久化受理、排队、准备、TTFT、上游完成和结算时间，再根据同渠道同模型样本判断是否优化本地。现有 Asynq wake 是主路径、1 秒轮询是 fallback，不预设“唤醒不存在”。

## 发布检查点

### 检查点 A：计划 01 完成

- [ ] 后端工具定向测试通过。
- [ ] Worker 装配测试证明 `ToolRuntime` 非空。
- [ ] 真实或 disposable MySQL 集成测试留下 `ai_tool_calls.status=success`。
- [ ] 可独立发布；不依赖官方模型改名。

### 检查点 B：计划 02 后端完成、计划 03 尚未完成

- [ ] 仅允许继续开发和生成临时契约。
- [ ] 禁止部署，因为旧前端路由、请求类型和新后端契约不兼容。

### 检查点 C：计划 02 与 03 同时完成

- [ ] 用户批准后端提交边界并提供真实 40 位 commit SHA。
- [ ] 后端生成正式 `contracts/admin/v1/*`。
- [ ] 前端同步 `contracts/backend/admin/v1/*` 并重新生成 TypeScript 类型。
- [ ] 后端契约、前端契约、路由、locale、typecheck、Vitest、build 和浏览器验收全部通过。
- [ ] 最终 schema 从空库初始化后只包含官方模型新命名。

### 检查点 D：计划 04 完成

- [ ] Run 详情可以还原受理、排队、准备、TTFT、Provider 总耗时和结算。
- [ ] wake、poll、recovery 来源可以区分。
- [ ] 同 Provider + model 的 P50/P95/P99 有样本数和固定窗口。
- [ ] 未证明 wake 失败时不改掉现有可靠唤醒；未证明本地瓶颈时不做无证据优化。

## 设计完成标准映射

| Spec 完成标准 | 实施计划 |
| --- | --- |
| 1. 唯一 `officialmodel.Resolver` | 02 / Task 1-2、6 |
| 2. Agent 不保存最大输出 | 02 / Task 3、6；03 / Task 3 |
| 3. 新聊天不接受 `max_tokens` | 02 / Task 5；03 / Task 4 |
| 4. 请求、冻结、报价共用安全上界 | 02 / Task 6 |
| 5. 余额不足无 Attempt，真实 usage 释放差额 | 02 / Task 6 |
| 6. 未映射、歧义、缺价、retired 禁止调用 | 02 / Task 4、6 |
| 7. 官方模型完整展示身份、生命周期、能力、限制、价格和来源 | 02 / Task 7；03 / Task 2 |
| 8. 除价格同步外基础事实只读 | 02 / Task 7；03 / Task 2 |
| 9. 模型定价旧领域名清零 | 02 / Task 3、7；03 / Task 1-2 |
| 10. temperature 能力门控 | 02 / Task 5；03 / Task 4 |
| 11. 图片/文件入口按有效能力门控且后端权威校验 | 02 / Task 5；03 / Task 5 |
| 12. 原生文件与平台文本解析明确分离 | 02 / Task 5；03 / Task 5 |
| 13. Agent 已实现工具进入 Worker 最终请求 | 01 / Task 1-2、4 |
| 14. admin_user_count 完成两轮调用、结算和审计 | 01 / Task 3-4 |
| 15. Worker 缺少工具运行时构造失败 | 01 / Task 1 |
| 16. Run 详情展示完整耗时分解 | 04 / Task 1-5 |
| 17. hi 慢请求可由持久化证据归因 | 04 / Task 2-5 |
| 18. max_history 过渡标记 | 02 / Task 5；03 / Task 4 |
| 19. 不固化当前知识库补丁 | 四份计划的执行边界 |

## 每阶段通用收尾

在对应仓库执行：

```powershell
git diff --check
git status --short
```

预期：无空白错误；只出现本计划列出的文件和用户原有未提交文件。不要清理、覆盖或回退用户已有改动。
