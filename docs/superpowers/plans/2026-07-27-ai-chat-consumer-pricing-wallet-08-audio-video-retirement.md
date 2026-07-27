# AI 音频与视频生成退役 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 完整移除当前产品中的 AI 音频生成与 AI 视频生成链路，同时保留普通媒体上传、图片生成、历史 Run/计费证据和通用 `media` 用量分类。

**Architecture:** 从 Agent 场景、业务模块、runtime/provider 装配和 OpenAI-compatible adapter 四个入口一起退役，避免留下可调用但无页面、或有配置但无执行器的半模块。数据库使用一份显式确认、失败关闭的单向迁移清理 Agent 场景并删除两张任务表；历史 `ai_runs`、attempt、Charge、Hold 终态和钱包流水不删除。

**Tech Stack:** Go、GORM、MySQL 8 / Atlas HCL、Vue 3 / TypeScript、Admin Contract Bundle。

**执行方式:** 按用户确认的低成本节奏执行：Task 1-4 连续完成雏形，中间不跑测试、不做逐任务审查；Task 5 只做一次集中格式化、定向验证和修复。不得自动运行全仓测试、race、frontend build、Docker、Playwright/E2E 或真实 MySQL migration。

---

## 不可越界的保留项

- 保留普通视频/音频文件上传、富文本编辑器视频、通用 MIME 识别和对象存储能力。
- 保留图片生成、`ai_assets` 通用媒体记录、provider/model 通用配置，以及计费用量分类 `media`。
- 保留全部历史 `ai_runs`、`ai_provider_attempts`、`ai_usage_charges`、`ai_usage_charge_items`、已终结 `wallet_holds` 和 `wallet_transactions`；不新增退款路径。
- 不回改 `database/migrations/202607250101` 至 `202607250105`、`database/legacy-migrations/*`、`database/reconciliation/*` 或历史 specs/plans。它们记录当时真实发生过的迁移与导入证据。
- 不创建数据库备份，不连接 MySQL，不自动执行迁移；只生成 migration、HCL 目标状态和 `atlas.sum`。
- 禁止用全局删除 `audio|video` 的方式实施。本计划只删除 AI 生成能力的明确符号和场景值。

### Task 1: 一次性删除后端业务模块与运行时装配

**Files:**
- Delete: `internal/module/ai/audio/dto.go`
- Delete: `internal/module/ai/audio/repository.go`
- Delete: `internal/module/ai/audio/service.go`
- Delete: `internal/module/ai/audio/service_test.go`
- Delete: `internal/module/ai/video/dto.go`
- Delete: `internal/module/ai/video/model.go`
- Delete: `internal/module/ai/video/repository.go`
- Delete: `internal/module/ai/video/service.go`
- Delete: `internal/module/ai/video/service_test.go`
- Modify: `internal/platform/admin/build.go`
- Modify: `internal/runtime/api.go`
- Modify: `internal/runtime/providers.go`
- Modify: `internal/runtime/providers_test.go`
- Delete: `internal/shared/i18n/locales/zh-CN/aiaudio.yaml`
- Delete: `internal/shared/i18n/locales/zh-CN/aivideo.yaml`
- Delete: `internal/shared/i18n/locales/en-US/aiaudio.yaml`
- Delete: `internal/shared/i18n/locales/en-US/aivideo.yaml`

- [x] **Step 1: 删除两个能力包**

删除列出的 9 个 Go 文件及空目录。不得移动到 `legacy`、不得保留空壳 Service，也不得留下新的 feature flag。

- [x] **Step 2: 清除 composition root**

从 Admin `BuildInputs`/Graph 和 runtime `Providers`/API 装配中删除 `AIVideoFactory`、`AIAudioFactory`、`aiVideoEngineFactory`、`aiAudioEngineFactory` 及其 imports/构造。保留 chat、tool、text、image 的单例装配，不改 Gateway、钱包或密钥派生。

- [x] **Step 3: 删除专属运行时测试和错误文案**

删除 `TestAIAudioEngineFactorySupportsOpenAI` 及只服务于两个退役模块的 fixtures；删除四个 `aiaudio.yaml`/`aivideo.yaml` catalog。此处只改代码，不运行测试。

### Task 2: 删除 provider/infra 适配并收紧 Agent 场景

**Files:**
- Modify: `internal/infra/ai/types.go`
- Modify: `internal/infra/ai/telemetry.go`
- Modify: `internal/infra/ai/telemetry_test.go`
- Modify: `internal/infra/ai/openaicompat/client.go`
- Modify: `internal/infra/ai/openaicompat/client_test.go`
- Modify: `internal/module/ai/capability/scenes.go`
- Modify: `internal/module/ai/capability/scenes_test.go`
- Modify: `internal/module/ai/agent/service.go`
- Modify: `internal/module/ai/agent/service_test.go`

- [x] **Step 1: 删除明确的 infra 契约**

删除 `VideoInput`、`VideoTask`、`VideoEngine`、`AudioInput`、`AudioResult`、`AudioEngine`，以及 `InstrumentVideoEngine`、`InstrumentAudioEngine` 和对应 wrapper/tests。不要删除共享 HTTP、telemetry、用量归一化或 `media` category。

- [x] **Step 2: 删除 OpenAI-compatible 生成方法**

删除 `CreateVideo`、`GetVideo`、`DownloadVideo`、`GenerateAudio`、`videoTaskFromPayload` 和它们的测试。删除 `friendlyUpstreamErrorHint` 中仅针对 reference-video privacy 的提示及测试；保留通用上游错误提取、脱敏和 chat/image adapter。

- [x] **Step 3: Agent 只接受四种场景**

删除 `SceneVideoGenerate`、`SceneAudioGenerate`。`sceneLabels`、`sceneOptions()`、`isScene`、创建/编辑校验和 init 字典只允许以下值，顺序固定：

```text
chat
agent_generate
text_generate
image_generate
```

更新测试证明未知值以及 `video_generate`/`audio_generate` 均被拒绝，init 不再发布这两个值；不要为旧值增加 silent fallback。

### Task 3: 清理前端场景并保持普通媒体 UI

**Files:**
- Modify: `E:\admin\admin_front_ts\src\api\ai\agents.ts`
- Modify: `E:\admin\admin_front_ts\src\views\Main\ai\agents\use-agent-admin-page.ts`
- Modify: `E:\admin\admin_front_ts\src\i18n\locales\zh-CN\ai.ts`
- Modify: `E:\admin\admin_front_ts\src\i18n\locales\en-US\ai.ts`
- Generate: `E:\admin\admin_front_ts\src\i18n\locales\generated.ts`
- Modify: `E:\admin\admin_front_ts\tests\shared\ai\ai-agent-api.test.ts`

- [x] **Step 1: 收紧前端类型和解析器**

把 `AiAgentScene`、`isAgentScene` 和 Agent 页面 fallback options 同步为后端四种场景。init 返回退役值时按契约错误处理，不显示、不提交，也不兼容映射成 chat。

- [x] **Step 2: 删除退役文案并生成 locale keys**

删除 `aiAgents.scene.videoGenerate`、`aiAgents.scene.audioGenerate` 的中英文源文案；最后通过 `npm run locale:generate` 生成 `generated.ts`，不得手改生成文件。

- [x] **Step 3: 更新唯一相关契约 fixture**

`ai-agent-api.test.ts` 的 `scene_arr` 只保留四种场景，并断言解析结果中不存在退役值。以下目录完全不改：

```text
src/components/UpMedia/**
src/lib/upload/**
src/views/Main/component/upload/**
src/views/Main/component/display/components/Editor.vue
```

`components.upMedia.videoPlaceholder` 必须保留，因为它属于普通上传/编辑能力。

### Task 4: 编写单向数据库退役迁移并更新当前架构事实

**Files:**
- Create: `database/migrations/202607270101_ai_audio_video_retirement.sql`
- Modify: `database/migrations/atlas.sum`
- Modify: `database/schema/admin.hcl`
- Modify: `internal/architecture/ai_billing_wave0_contract_test.go`
- Modify: `internal/architecture/ai_capability_boundary_test.go`
- Modify: `internal/architecture/ai_runtime_aggregation_test.go`
- Modify: `internal/architecture/admin_only_test.go`
- Modify: `docs/architecture.md`

- [x] **Step 1: 先写失败关闭的 migration guard**

迁移开头要求维护人员在同一连接显式执行：

```sql
SET @ai_audio_video_retirement_verified = 1;
```

随后使用临时 guard table + `CHECK (violations = 0)`，在任何持久化更新/DDL 前一次性验证：

1. 确认变量严格等于 `1`。
2. `ai_agents.scenes_json` 中涉及退役值的行必须是 JSON array，元素只能来自六个旧合法场景；异常形状或未知值直接阻断。
3. 任务状态必须落在旧模块的已知枚举内，并且只能剩终态：video 只允许 `completed|failed|cancelled`，audio 只允许 `success|failed|canceled|outcome_unknown`；`pending`、`running` 和任何未知值全部阻断。
4. 两表关联的 Run 不得缺失，也不得处于 `running`、`billing_status IN ('pending','held')`；关联 Charge 不得为 `open`，Hold 不得为 `active`，attempt 不得为 `prepared`/`dispatched`。
5. `information_schema.KEY_COLUMN_USAGE` 中不存在其他表指向这两张表的 FK；`VIEW_TABLE_USAGE`、`TRIGGERS`、`EVENTS`、`ROUTINES` 中不存在依赖或引用。目标表自身到 `ai_runs` 的 FK 不算外部引用；元数据定义不可见时也必须失败，不得把权限不足解释为“没有引用”。

本迁移的人工执行顺序固定为：先部署已删除 writer/route/factory 的新 API 与 Worker、停止全部旧进程并确认没有活动任务，再在维护连接中设置确认变量并执行 migration。migration 执行后不支持回滚到仍查询两张 task table 的旧二进制，只能向前修复。本计划自身不执行这些部署或数据库动作。

- [x] **Step 2: 原子迁移 Agent 场景**

使用 MySQL 8 `JSON_TABLE` 按原顺序重建数组，规则固定：

- 混合 Agent：只删除 `video_generate`、`audio_generate`，保留 chat/tool/text/image。
- 仅包含一个或两个退役场景的 Agent：同一条 UPDATE 设置 `scenes_json = JSON_ARRAY()`、`status = 2`、`is_del = 1`；禁止自动改成 chat。
- `NULL` 或原本为空数组但不含退役值的行不由本迁移改写。
- 更新后验证所有 `is_del = 2` 的 Agent 均不再包含退役值；本次命中的每一行都必须得到非 NULL 的合法 JSON array，不得写 `NULL` 或空字符串。原本不含退役值的历史 `NULL`/空数组不纳入该断言。

- [x] **Step 3: 删除任务表但保留账务事实**

所有 guard 和 Agent 更新成功后，以一个 DDL statement 执行：

```sql
DROP TABLE `ai_video_tasks`, `ai_audio_tasks`;
```

迁移不得出现针对下列历史表的 `DELETE`、`DROP` 或级联清理：

```text
ai_runs
ai_provider_attempts
ai_usage_charges
ai_usage_charge_items
wallet_holds
wallet_transactions
ai_assets
```

从 `database/schema/admin.hcl` 删除两个 task table block；保留 `ai_usage_charge_items.category` 的 `media` 取值。只运行一次 `pwsh -NoProfile -File scripts/database/atlas.ps1 migrate hash --dir file://database/migrations` 更新 `atlas.sum`；若 Atlas 镜像未就绪或两分钟内不能完成，停止并把该命令留给用户，绝不连接数据库或执行 `migrate apply`。在 `atlas.sum` 尚未成功覆盖新 migration 时不得提交或声称 Plan 08 完成。

- [x] **Step 4: 改写当前架构断言和文档**

当前架构测试应断言两个 module 根目录、runtime factories、当前 HCL 表和当前场景均不存在；`ai_billing_wave0_contract_test.go` 仍可读取 `202607250101/103` 证明历史 audio migration 事实，但不得再要求当前 HCL 存在 `ai_audio_tasks`。`admin_only_test.go` 将 transport-only 负向断言加强为两个 module 根目录均不存在。

保持以下历史测试/证据不变：

```text
internal/architecture/ai_run_records_schema_test.go
internal/architecture/reconciliation_schema_test.go
internal/architecture/reconciliation_ai_test.go
database/legacy-migrations/**
database/reconciliation/**
```

`docs/architecture.md` 改为当前只支持 chat/tool/text/image；说明音视频 AI 生成已退役、历史 Run/账务仍保留。删除当前 `/videos`、`/audio/speech`、Canvas audio/video runtime 和 reference-video 专属描述，但保留普通媒体上传、`ai_assets` 和通用 media 事实。`internal/module/README.md` 当前已只列保留模块，除非实现 diff 产生事实冲突，否则不改；`internal/platform/README.md` 同理。

### Task 5: 一次集中格式化、验证、修复与提交

- [x] **Step 1: 集中生成和格式化**

完成 Task 1-4 后再统一运行：

```powershell
Set-Location E:\admin\admin_back_go
gofmt -w `
  internal/infra/ai/types.go `
  internal/infra/ai/telemetry.go `
  internal/infra/ai/telemetry_test.go `
  internal/infra/ai/openaicompat/client.go `
  internal/infra/ai/openaicompat/client_test.go `
  internal/module/ai/agent/service.go `
  internal/module/ai/agent/service_test.go `
  internal/module/ai/capability/scenes.go `
  internal/module/ai/capability/scenes_test.go `
  internal/platform/admin/build.go `
  internal/runtime/api.go `
  internal/runtime/providers.go `
  internal/runtime/providers_test.go `
  internal/architecture/admin_only_test.go `
  internal/architecture/ai_billing_wave0_contract_test.go `
  internal/architecture/ai_capability_boundary_test.go `
  internal/architecture/ai_runtime_aggregation_test.go

Set-Location E:\admin\admin_front_ts
npm run locale:generate
```

- [x] **Step 2: 只跑最小定向验证并集中修一次**

```powershell
Set-Location E:\admin\admin_back_go
go test ./internal/module/ai/agent ./internal/module/ai/capability ./internal/infra/ai/... ./internal/runtime ./internal/platform/admin
go test ./internal/architecture -run 'TestAI|TestAdminOnly|TestDatabaseLayout|TestReconciliation'

Set-Location E:\admin\admin_front_ts
npm test -- tests/shared/ai/ai-agent-api.test.ts tests/unit/contracts/admin-contract.test.ts
npm run locale:check
npm run typecheck
```

若失败，汇总全部失败后一次修复，再只重跑失败命令；禁止为每个文件开启独立审查循环。

- [x] **Step 3: 做正反两向静态边界审计**

负向搜索只查当前运行时、当前 schema/docs 和前端源代码，不查历史 migrations/reconciliation/specs/plans：

```powershell
Set-Location E:\admin\admin_back_go
rg -n 'audio_generate|video_generate|SceneAudioGenerate|SceneVideoGenerate|AIAudioFactory|AIVideoFactory|aiAudioEngineFactory|aiVideoEngineFactory|AudioEngine|VideoEngine|GenerateAudio|CreateVideo|GetVideo|DownloadVideo|InstrumentAudioEngine|InstrumentVideoEngine|ai_audio_tasks|ai_video_tasks' internal/module internal/infra internal/runtime internal/platform database/schema docs/architecture.md E:\admin\admin_front_ts\src
```

Expected: no matches. 再确认保留项仍存在：

```powershell
rg -n 'media' internal/module/ai internal/infra/ai database/schema/admin.hcl
rg -n 'videoPlaceholder|UpMedia' E:\admin\admin_front_ts\src
```

Expected: 两条均有与保留能力相符的 matches。不得把普通 audio/video 文本当成清理失败。

- [x] **Step 4: 发布并同步 Contract Bundle**

先提交后端源代码，使 manifest 绑定一个确定 commit，再生成契约：

```powershell
Set-Location E:\admin\admin_back_go
git add -- `
  database/migrations/202607270101_ai_audio_video_retirement.sql `
  database/migrations/atlas.sum `
  database/schema/admin.hcl `
  docs/architecture.md `
  internal/architecture/admin_only_test.go `
  internal/architecture/ai_billing_wave0_contract_test.go `
  internal/architecture/ai_capability_boundary_test.go `
  internal/architecture/ai_runtime_aggregation_test.go `
  internal/infra/ai/types.go `
  internal/infra/ai/telemetry.go `
  internal/infra/ai/telemetry_test.go `
  internal/infra/ai/openaicompat/client.go `
  internal/infra/ai/openaicompat/client_test.go `
  internal/module/ai/agent/service.go `
  internal/module/ai/agent/service_test.go `
  internal/module/ai/audio `
  internal/module/ai/capability/scenes.go `
  internal/module/ai/capability/scenes_test.go `
  internal/module/ai/video `
  internal/platform/admin/build.go `
  internal/runtime/api.go `
  internal/runtime/providers.go `
  internal/runtime/providers_test.go `
  internal/shared/i18n/locales/en-US/aiaudio.yaml `
  internal/shared/i18n/locales/en-US/aivideo.yaml `
  internal/shared/i18n/locales/zh-CN/aiaudio.yaml `
  internal/shared/i18n/locales/zh-CN/aivideo.yaml
git commit -m "refactor(ai): retire audio and video generation"
$backendCommit = (git rev-parse HEAD).Trim()
pwsh -NoProfile -File scripts/generate-admin-contract.ps1 -BackendCommit $backendCommit
pwsh -NoProfile -File scripts/check-admin-contract.ps1 -BackendCommit $backendCommit
git add -- contracts/admin/v1
git commit -m "chore(contract): sync ai capability retirement"

Set-Location E:\admin\admin_front_ts
$backendCommit = (Get-Content E:\admin\admin_back_go\contracts\admin\v1\manifest.json -Raw | ConvertFrom-Json).backend_commit
npm run contract:sync -- --backend E:\admin\admin_back_go --commit $backendCommit
npm run contract:generate
npm run contract:check
git add -- contracts/backend/admin src/api/ai/agents.ts src/views/Main/ai/agents/use-agent-admin-page.ts src/i18n/locales tests/shared/ai/ai-agent-api.test.ts
git commit -m "refactor(ai): remove audio and video agent scenes"
```

即使 OpenAPI schema 无结构变化，也必须让后端 manifest 和前端 vendored contract 指向同一个已提交 backend commit；禁止手改 generated contract。

- [x] **Step 5: 最终轻量检查并交付人工命令**

两个仓库各执行一次 `git diff --check` 和 `git status --short`。不自动运行以下项目，只在交付中列给用户按需手动执行：

```powershell
go test ./...
go test -race ./...
npm test
npm run build
# Docker / Playwright / E2E / 真实 MySQL migration
```

## 完成标准

- Admin init 和前端 Agent 配置只提供 chat/tool/text/image 四种场景。
- Go runtime 无音频/视频 AI module、factory、infra interface 或 OpenAI-compatible 生成方法。
- 当前 HCL 无 `ai_audio_tasks`/`ai_video_tasks`；迁移有显式人工确认和全部失败关闭 guard，但未被自动执行。
- 历史 Run、attempt、Charge、usage、Hold 终态、钱包流水及 reconciliation 证据均保留。
- 普通媒体上传、编辑器视频、图片生成、`ai_assets` 和 `media` category 未受影响。
- 只完成一次集中定向验证，没有自动启动大测试链。
