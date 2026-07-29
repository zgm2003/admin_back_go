# AI 官方模型、Agent 与聊天能力交互 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Do not dispatch subagents. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将后台“模型定价”重做为“官方模型”，从 Agent 和聊天删除最大输出配置，并让参数、图片入口与 Agent 切换完全服从服务端有效能力。

**Architecture:** 前端只消费 Admin Contract 返回的 `official_model` 与 `capabilities`，不猜模型名称。官方模型页面只允许价格同步；Agent 页面配置业务策略；聊天输入区只显示当前 Agent 真正可用的参数和附件入口。

**Tech Stack:** Vue 3、TypeScript、Element Plus、Vitest、Vue Test Utils、Playwright、Admin Contract generated types。

**Spec:** `docs/superpowers/specs/2026-07-28-ai-chat-capability-tools-interaction-design.md` 第 4 节。

---

## 执行边界

- 工作目录为 `E:/admin/admin_front_ts`。
- 必须在计划 02 后端实现完成后执行，并与计划 02 同一发布窗口上线。
- 不保留 `/ai/model-pricing`、旧 API、旧权限或前端 fallback。
- 不开放通用文件按钮，不修改知识库页面与知识绑定流程。
- 不自动提交 Git。

### Task 1：同步正式契约并完成前端全链路改名

**Files:**
- Regenerate: `contracts/backend/admin/v1/*`
- Regenerate: `src/modules/http/generated/admin.ts`
- Regenerate: `src/modules/http/generated/operations.ts`
- Move: `src/api/ai/model-prices.ts` -> `src/api/ai/official-models.ts`
- Move: `src/views/Main/ai/model-pricing` -> `src/views/Main/ai/official-models`
- Modify: `src/i18n/locales/zh-CN/ai.ts`
- Modify: `src/i18n/locales/en-US/ai.ts`
- Modify: `src/i18n/locales/zh-CN/layout.ts`
- Modify: `src/i18n/locales/en-US/layout.ts`
- Regenerate: `src/i18n/locales/generated.ts`
- Regenerate: `src/router/view-registry.ts`
- Regenerate: `src/modules/routing/generated/*`
- Move: `tests/shared/ai/ai-model-pricing-api.test.ts` -> `tests/shared/ai/ai-official-model-api.test.ts`
- Move: `tests/component/ai/ModelPricingPage.test.ts` -> `tests/component/ai/OfficialModelPage.test.ts`
- Modify: `tests/shared/ai/admin-ai-interaction-retirement.test.ts`

- [ ] **Step 1：先把旧契约测试改成最终命名并确认失败**

测试固定使用：

```text
/ai/official-models
ai/official-models
menu.ai_official_models
ai_official_model_list
ai_official_model_price_sync
/api/admin/v1/ai-official-models
```

运行：

```powershell
npm test -- tests/shared/ai/ai-official-model-api.test.ts tests/shared/ai/admin-ai-interaction-retirement.test.ts
```

预期：FAIL，当前代码仍引用模型定价路径和权限。

- [ ] **Step 2：同步真实后端提交发布的契约**

前置条件：后端已由用户批准形成真实提交，并用该 SHA 生成正式 `contracts/admin/v1/*`。执行：

```powershell
npm run contract:sync
npm run contract:generate
```

不得手改 generated TypeScript，不得使用旧 OpenAPI 临时目录覆盖正式 contract。

- [ ] **Step 3：完成 rename 和生成物更新**

API 导出固定为 `AiOfficialModelApi`，页面组合函数固定为 `useOfficialModelPage`，i18n namespace 固定为 `aiOfficialModel`。执行：

```powershell
npm run routes:generate
npm run locale:generate
npm test -- tests/shared/ai/ai-official-model-api.test.ts tests/shared/ai/admin-ai-interaction-retirement.test.ts
```

预期：PASS；活动前端不存在旧路由或旧权限。

### Task 2：实现“官方模型”列表与详情抽屉

**Files:**
- Modify: `src/api/ai/official-models.ts`
- Modify: `src/views/Main/ai/official-models/index.vue`
- Modify: `src/views/Main/ai/official-models/use-official-model-page.ts`
- Move: `src/views/Main/ai/official-models/components/ModelPriceDrawer.vue` -> `src/views/Main/ai/official-models/components/OfficialModelDrawer.vue`
- Move: `src/views/Main/ai/official-models/components/ModelPriceDrawer.css` -> `src/views/Main/ai/official-models/components/official-model-drawer.css`
- Test: `tests/shared/ai/ai-official-model-api.test.ts`
- Test: `tests/component/ai/OfficialModelPage.test.ts`

- [ ] **Step 1：写页面行为失败测试**

增加：

```ts
it('renders identity lifecycle capabilities limits prices and sources')
it('keeps catalog facts readonly and only submits a complete price set')
it('hides price actions without ai_official_model_price_sync')
it('shows deprecated and retired lifecycle rules')
it('uses a full-screen drawer on mobile')
```

运行：

```powershell
npm test -- tests/shared/ai/ai-official-model-api.test.ts tests/component/ai/OfficialModelPage.test.ts
```

预期：FAIL，现有页面以价格为主体且缺能力与生命周期展示。

- [ ] **Step 2：实现列表和抽屉**

列表列固定为官方模型、生命周期、输入/输出、核心能力、上下文/最大输出、当前价格、核验、操作；筛选固定为 vendor、family、lifecycle、input modality、model ID。

抽屉顺序固定为模型身份、容量与限制、输入与输出、模型能力、价格、事实来源。身份、alias、能力、上下文和最大输出全部用文本/状态图标呈现，不渲染表单控件。只有价格区域提供“同步价格”和“恢复官方价格”；409 时提示数据已更新并重新拉取详情。

- [ ] **Step 3：验证通过**

运行 Step 1 命令。预期：PASS，桌面抽屉宽度稳定，移动端全屏，无卡片嵌套卡片。

### Task 3：重组 Agent 编辑页并删除最大输出配置

**Files:**
- Modify: `src/api/ai/agents.ts`
- Modify: `src/views/Main/ai/agents/index.vue`
- Modify: `src/views/Main/ai/agents/use-agent-admin-page.ts`
- Move: `src/views/Main/ai/agents/components/AgentModelPricingPanel.vue` -> `src/views/Main/ai/agents/components/AgentOfficialModelSummary.vue`
- Modify: `src/views/Main/ai/agents/components/AgentToolDialog/index.vue`
- Modify: `tests/shared/ai/ai-agent-api.test.ts`
- Modify: `tests/shared/ai/ai-agent-billing-config.test.ts`
- Create: `tests/component/ai/AgentOfficialModelForm.test.ts`

- [ ] **Step 1：写 Agent 契约与交互失败测试**

```ts
it('never serializes max_output_tokens in agent mutations')
it('lists only mapped active provider models for a new selection')
it('shows an existing deprecated binding as readonly warning')
it('blocks a retired binding until the model is changed')
it('renders official limits as readonly facts')
it('lists only executable low-risk tools when tools are supported')
```

运行：

```powershell
npm test -- tests/shared/ai/ai-agent-api.test.ts tests/shared/ai/ai-agent-billing-config.test.ts tests/component/ai/AgentOfficialModelForm.test.ts
```

预期：FAIL，当前 mutation 和表单仍有 `max_output_tokens`。

- [ ] **Step 2：实现最终 Agent 信息结构**

表单分为基本信息、模型、可用能力、计费四个无外层装饰卡的区块。模型选择后紧邻显示官方 model ID、生命周期、目录版本、上下文、最大输出、输入/输出模态、tools、streaming；这些均只读。

Agent mutation 只提交 provider/model、基本信息、scenes、system prompt、status 和 billing multiplier。工具按钮在有效能力不支持 tools 时禁用并显示明确 tooltip；工具弹窗只消费后端返回的可执行工具。

- [ ] **Step 3：验证通过**

运行 Step 1 命令。预期：PASS，代码中只读官方摘要可出现 `max_output_tokens`，Agent mutation 和表单 state 不再出现该字段。

### Task 4：重做“本次回答设置”并拒绝隐式覆盖

**Files:**
- Modify: `src/views/Main/ai/chat/components/MessageInput/runtime-params.ts`
- Modify: `src/views/Main/ai/chat/components/MessageInput/RuntimeParamsPanel.vue`
- Modify: `src/views/Main/ai/chat/components/MessageInput/runtime-params-panel.css`
- Modify: `src/views/Main/ai/chat/components/MessageInput/index.vue`
- Modify: `src/api/ai/messages.ts`
- Modify: `tests/shared/ai/page-presenters.test.ts`
- Modify: `tests/shared/ai/ai-run-input-snapshot.test.ts`
- Create: `tests/component/ai/RuntimeParamsPanel.test.ts`

- [ ] **Step 1：写参数面板失败测试**

```ts
it('never constructs max_tokens')
it('submits no runtime params until an override switch is enabled')
it('hides temperature when the effective capability rejects it')
it('submits only enabled temperature and transitional max_history')
it('restores model defaults on conversation or agent change')
```

运行：

```powershell
npm test -- tests/shared/ai/page-presenters.test.ts tests/shared/ai/ai-run-input-snapshot.test.ts tests/component/ai/RuntimeParamsPanel.test.ts
```

预期：FAIL，当前 helper 仍构造 `max_tokens`，滑块本身也代表启用覆盖。

- [ ] **Step 2：实现显式开关模型**

`createRuntimeParams` 固定签名：

```ts
export function createRuntimeParams(input: {
  temperature?: { enabled: boolean; value: number }
  maxHistory?: { enabled: boolean; value: number }
}): AIRuntimeParams
```

关闭开关不提交对应字段；temperature 只在 capability 支持时显示；`max_history` 文案为“携带历史消息”并显示过渡标识。面板关闭保留本会话草稿，切换会话或 Agent 恢复默认。Run 历史 presenter 可以只读展示旧快照 `max_tokens`，但新请求类型、编辑控件和发送 helper 必须清零。

- [ ] **Step 3：验证通过**

运行 Step 1 命令。预期：PASS，新发送路径不存在最大输出设置。

### Task 5：按有效能力控制图片并处理 Agent 切换冲突

**Files:**
- Modify: `src/api/ai/agents.ts`
- Modify: `src/views/Main/ai/chat/composables/useAgents.ts`
- Modify: `src/views/Main/ai/chat/use-chat-page.ts`
- Modify: `src/views/Main/ai/chat/components/MessageInput/use-image-attachments.ts`
- Modify: `src/views/Main/ai/chat/components/MessageInput/PendingAttachments.vue`
- Modify: `src/views/Main/ai/chat/components/MessageInput/index.vue`
- Modify: `tests/component/ai/MessageInteractions.test.ts`
- Create: `tests/shared/ai/ai-chat-capabilities.test.ts`

- [ ] **Step 1：写图片门控和切换失败测试**

```ts
it('hides image actions for a text-only agent')
it('uses server mime count and byte limits for image selection')
it('never renders a generic document input')
it('asks before switching when attachments or temperature become invalid')
it('keeps the current agent when conflict cleanup is canceled')
it('clears only invalid state after confirmed switch')
```

运行：

```powershell
npm test -- tests/shared/ai/ai-chat-capabilities.test.ts tests/component/ai/MessageInteractions.test.ts
```

预期：FAIL，当前 `supportsImage` 永远为 true，限制固定为 5，切换不检查冲突。

- [ ] **Step 2：实现服务端能力驱动**

`useImageAttachments(capability)` 从 `attachments.image` 读取 enabled、MIME、max files、max bytes；文件 input 的 `accept` 同步 MIME。上传完成后请求附件包含 `type=image`、object key、name、客户端 MIME 和 size，后端仍以 HEAD 为准。

Agent 切换前比较目标能力与当前待发送状态。若图片或 temperature 失效，弹出一次确认；取消则不切换，确认后只删除失效附件/覆盖并切换。不存在冲突时直接切换。不得增加通用文件入口。

- [ ] **Step 3：验证通过**

运行 Step 1 命令。预期：PASS，图片入口、粘贴和拖放使用同一能力判断。

### Task 6：完成生成检查、响应式和真实浏览器验收

**Files:**
- Modify: `src/views/Main/ai/official-models/**/*.css`
- Modify: `src/views/Main/ai/agents/styles.css`
- Modify: `src/views/Main/ai/chat/styles.css`
- Modify: `src/views/Main/ai/chat/components/MessageInput/*.css`

- [ ] **Step 1：运行前端全门禁**

```powershell
npm run contract:check
npm run routes:check
npm run locale:check
npm run typecheck
npm test -- tests/shared/ai tests/component/ai
npm run build
```

预期：全部 PASS；无手写 generated 类型漂移。

- [ ] **Step 2：执行旧名与可变最大输出清零**

```powershell
rg -n 'model-pricing|ai-model-prices|ai_model_pricing|ai_model_price_overrides|ai_model_price_override_rates' src tests contracts/backend/admin/v1
rg -n 'max_tokens|max_output_tokens' src/views/Main/ai/chat src/views/Main/ai/agents src/api/ai/messages.ts src/api/ai/agents.ts
```

预期：第一条无结果；第二条只允许 Run 历史只读 presenter 或官方模型只读摘要命中，不允许 mutation、聊天设置或发送 helper 命中。

- [ ] **Step 3：真实浏览器验收**

从 `E:/admin/admin_back_go` 启动现有 Docker 平台：

```powershell
pwsh -NoProfile -File scripts/docker-platform.ps1 up
```

使用 Playwright 分别检查 `1440x900`、`1024x768`、`390x844`：官方模型列表/抽屉、Agent 新建/编辑、文本模型聊天、图片模型聊天、参数开关、Agent 冲突切换、键盘焦点与 axe 扫描。截图确认没有文字溢出、控件重叠、横向滚动或嵌套卡片。

若本地 Docker、登录账号或后端契约未就绪，记录为“浏览器验收未执行”并保留 Step 1 自动门禁结果，不能写成 PASS。

- [ ] **Step 4：阶段收尾**

```powershell
git diff --check
git status --short
```

预期：仅包含计划 03 文件和契约同步产物；没有知识库页面改动，没有 Git commit。计划 02 与 03 此时才具备共同发布条件。
