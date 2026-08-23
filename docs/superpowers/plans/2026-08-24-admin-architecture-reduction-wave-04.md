# Admin 架构减法 Wave 04 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不改变 API、数据库、任务类型、Realtime 协议和用户行为的前提下，删除 Worker 的空静态调度适配器，并把前端 Realtime/COS 工具迁移到目标目录。

**Architecture:** `internal/jobs.NewRegistry` 是唯一 Asynq 任务注册入口，数据库定时任务只由 `internal/module/crontask` 管理。后端 Realtime 保持 `transport -> module -> infra`，MySQL 仍是可恢复事件事实，Redis 只负责广播。前端跨页面运行工具统一放在 `src/utils`，COS 继续浏览器直传，不新增上传完成 API 或元数据表。

**Tech Stack:** Go 1.26.5、Gin、GORM、MySQL、Redis、Asynq、Gorilla WebSocket、Vue 3、TypeScript、Vite、Vitest、ESLint。

---

## 执行前硬约束

- 先完整阅读 `AGENTS.md` 和设计 Spec：
  - `E:/admin/admin_back_go/AGENTS.md`
  - `E:/admin/admin_back_go/docs/superpowers/specs/2026-08-24-admin-architecture-reduction-wave-04-design.md`
- 两个原始仓库都必须是 `master` 且工作区干净；不创建 worktree，不切换分支。
- 只由唯一 `work-ai` 执行本计划；不得创建第二个外部 worker 窗口。
- 不启动 `admin-dev`，不执行 SQL，不新增 migration/seed/baseline。
- 不运行 `go test ./...`、全量 Vue 测试、Playwright、`verify:frontend` 或其他长脚本。
- 每个任务完成后分别提交 backend/frontend，等待主 AI 复核后再进入下一任务。
- 发现需要修改 AI、支付、数据库字段、生成合同或用户行为时立即停止并汇报，不扩大范围。

## 文件责任地图

| 文件/目录 | Wave 04 责任 |
|---|---|
| `internal/jobs/noop.go` | 保留统一 Registry，删除空静态调度适配器 |
| `internal/jobs/noop_test.go` | 删除适配器测试，保留 Registry 测试 |
| `internal/runtime/worker.go` | 删除空静态注册调用，保持 Worker 启动顺序 |
| `internal/runtime/realtime.go` | 复用 Redis Publisher 构造边界，不改变 local/noop/redis 行为 |
| `internal/architecture/wave04_boundaries_test.go` | 守住 Worker、Realtime、前端目录边界 |
| `src/utils/realtime/` | 前端 Realtime 唯一实现目录 |
| `src/utils/upload/` | 前端 COS 上传唯一实现目录 |
| `src/modules/realtime/` | 迁移后删除 |
| `src/lib/upload/` | 迁移后删除 |
| `docs/superpowers/plans/2026-08-13-admin-architecture-reduction-execution-index.md` | Wave 04 完成人工验收后记录恢复点和结果 |

### Task 1: 删除 Worker 空静态调度适配器

**Files:**
- Modify: `E:/admin/admin_back_go/internal/jobs/noop.go`
- Modify: `E:/admin/admin_back_go/internal/jobs/noop_test.go`
- Modify: `E:/admin/admin_back_go/internal/runtime/worker.go`
- Modify: `E:/admin/admin_back_go/internal/runtime/worker_test.go`（仅删除引用空适配器的断言）
- Create: `E:/admin/admin_back_go/internal/architecture/wave04_boundaries_test.go`

- [x] **Step 1: 写边界回归测试**

在 `internal/architecture/wave04_boundaries_test.go` 增加以下测试，先锁定本任务要删除的符号和调用：

```go
package architecture

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestWave04WorkerHasNoStaticScheduleAdapter(t *testing.T) {
    root := repositoryRoot(t)
    jobs, err := os.ReadFile(filepath.Join(root, "internal", "jobs", "noop.go"))
    if err != nil {
        t.Fatal(err)
    }
    worker, err := os.ReadFile(filepath.Join(root, "internal", "runtime", "worker.go"))
    if err != nil {
        t.Fatal(err)
    }
    for _, forbidden := range []string{
        "type ScheduleRegistrar interface",
        "type ScheduledTaskDefinition struct",
        "func RegisterSchedules(",
        "func registerScheduleDefinitions(",
        "func scheduledEnqueueTask(",
    } {
        if strings.Contains(string(jobs), forbidden) {
            t.Fatalf("jobs/noop.go still contains removed adapter %q", forbidden)
        }
    }
    if strings.Contains(string(worker), "jobs.RegisterSchedules(") {
        t.Fatal("worker runtime still invokes the removed static schedule adapter")
    }
}
```

在同一测试文件中加入最小的 `repositoryRoot` 辅助函数；当前 `internal/architecture` 没有可复用实现，不要引入全局路径配置：

```go
func repositoryRoot(t *testing.T) string {
    t.Helper()
    dir, err := os.Getwd()
    if err != nil {
        t.Fatal(err)
    }
    for {
        if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
            return dir
        }
        parent := filepath.Dir(dir)
        if parent == dir {
            t.Fatal("go.mod not found")
        }
        dir = parent
    }
}
```

- [x] **Step 2: 运行测试确认当前基线失败**

Run from `E:/admin/admin_back_go`:

```powershell
go test ./internal/architecture -run '^TestWave04WorkerHasNoStaticScheduleAdapter$' -count=1
```

Expected: FAIL，错误指出 `noop.go` 仍包含 `ScheduleRegistrar` 或 Worker 仍调用 `jobs.RegisterSchedules`。

- [x] **Step 3: 删除空适配器并保留 Registry**

从 `internal/jobs/noop.go` 删除以下类型、函数和只服务于它们的错误变量/导入：

```text
ScheduleRegistrar
ScheduledTaskDefinition
RegisterSchedules
registerScheduleDefinitions
scheduledEnqueueTask
ErrScheduleRegistrarRequired
ErrScheduleEnqueuerRequired
ErrScheduleTaskBuilderRequired
```

不要删除 `Dependencies`、`NewRegistry`、`Register`、`NewNoopTask` 或任何模块的任务注册函数。

从 `internal/runtime/worker.go` 删除唯一的：

```go
return jobs.RegisterSchedules(built, queueClient, logger)
```

删除 `noop_test.go` 中 fake schedule registrar、fake enqueuer 以及 `TestRegisterSchedulesDoesNotRegisterStaticNotificationDispatchDue` 等适配器测试；保留 Registry 和 Noop task 测试。`worker_test.go` 只删除因该调用存在而产生的断言，不改 Worker 生命周期测试。

- [x] **Step 4: 运行 Worker 定向测试**

```powershell
go test ./internal/jobs ./internal/infra/taskqueue ./internal/runtime -run 'Test(NewRegistry|Registry|Noop|Worker|Task|RealtimePublisher)' -count=1
go test ./internal/architecture -run '^TestWave04WorkerHasNoStaticScheduleAdapter$' -count=1
```

Expected: 两条命令均 PASS；任务类型和队列策略测试数量不应减少到 0。

- [x] **Step 5: 提交 backend Task 1**

```powershell
git add -- internal/jobs/noop.go internal/jobs/noop_test.go internal/runtime/worker.go internal/runtime/worker_test.go internal/architecture/wave04_boundaries_test.go
git diff --cached --check
git commit -m "refactor(worker): remove empty static schedule adapter"
```

### Task 2: 收紧 Realtime 运行边界并迁移前端目录

**Files:**
- Modify: `E:/admin/admin_back_go/internal/runtime/realtime.go`
- Modify: `E:/admin/admin_back_go/internal/runtime/realtime_test.go`
- Modify: `E:/admin/admin_back_go/internal/runtime/worker_test.go`
- Move: `E:/admin/admin_front_ts/src/modules/realtime/` -> `E:/admin/admin_front_ts/src/utils/realtime/`
- Modify imports: `E:/admin/admin_front_ts/src/app/kernel.ts`, `src/main.ts`, `src/adapters/web/websocket.ts`, `src/api/ai/billing-error.ts`, `src/api/ai/chat-events.ts`, `src/features/ai-chat/workflow.ts`
- Modify realtime test imports under `E:/admin/admin_front_ts/tests/unit/realtime/`, `tests/integration/realtime/`, `tests/integration/features/support.ts`, `tests/shared/http/ai-conversation-websocket-contract.test.ts`
- Modify: `E:/admin/admin_back_go/internal/architecture/wave04_boundaries_test.go`

- [x] **Step 1: 写前端目录边界测试**

在 `wave04_boundaries_test.go` 增加后端侧的静态检查，确认源码不创建连接管理器：

```go
func TestWave04WorkerDoesNotOwnRealtimeSessions(t *testing.T) {
    root := repositoryRoot(t)
    worker, err := os.ReadFile(filepath.Join(root, "internal", "runtime", "worker.go"))
    if err != nil {
        t.Fatal(err)
    }
    if strings.Contains(string(worker), "infrarealtime.NewManager(") {
        t.Fatal("Worker must publish realtime events but must not own WebSocket sessions")
    }
}
```

前端目录检查由计划末尾的 `rg` 验收命令完成，不增加运行时别名。

- [x] **Step 2: 抽出唯一 Redis Publisher 构造点**

在 `internal/runtime/realtime.go` 增加一个同文件私有函数，统一处理 nil Redis 资源，不改变任何 publisher 类型：

```go
func newRedisRealtimePublisher(
    client *redisclient.Client,
    channel string,
    validator infrarealtime.EnvelopeValidator,
) infrarealtime.Publisher {
    if client == nil {
        return infrarealtime.NewRedisPublisher(nil, channel, validator)
    }
    return infrarealtime.NewRedisPublisher(client.Redis, channel, validator)
}
```

将 `realtimePublisherFor` 和 `realtimePublisherForWorker` 中重复的 `NewRedisPublisher(...)` 调用改为使用该函数。保留 API 缺少 Redis 时的日志和 subscriber 行为；保留 Worker 缺少 Realtime Redis 时的显式 noop/redis 行为。不得改变 `RealtimeRedis` DB、channel、enabled 判定或 local/noop publisher 语义。

- [x] **Step 3: 运行 backend Realtime 定向测试**

```powershell
go test ./internal/infra/realtime ./internal/module/realtime ./internal/runtime -run 'Test(Envelope|Subscription|Resume|Realtime|Redis|Worker)' -count=1
go test ./internal/architecture -run '^TestWave04WorkerDoesNotOwnRealtimeSessions$' -count=1
```

Expected: PASS；不得运行生成合同脚本，因为事件 JSON 未变化。

- [x] **Step 4: 迁移前端 Realtime 目录**

在 `E:/admin/admin_front_ts` 执行：

```powershell
git mv src/modules/realtime src/utils/realtime
```

把所有 `@/modules/realtime/...` 改为 `@/utils/realtime/...`，把 `main.ts` 的相对导入改为 `./utils/realtime/client`。只改路径，不改导出名、协议字段或状态机逻辑。

- [x] **Step 5: 运行前端 Realtime 定向测试和 ESLint**

```powershell
npx vitest run --no-file-parallelism tests/unit/realtime tests/integration/realtime tests/integration/app/kernel.test.ts tests/integration/features/ai-chat.test.ts tests/integration/features/ai-runs.test.ts tests/integration/features/notifications.test.ts
npx eslint src/utils/realtime src/app/kernel.ts src/main.ts src/adapters/web/websocket.ts src/api/ai/billing-error.ts src/api/ai/chat-events.ts src/features/ai-chat/workflow.ts src/features/ai-runs/workflow.ts
rg -n "@/modules/realtime|src/modules/realtime|from './modules/realtime'" src tests
```

Expected: Vitest 和 ESLint PASS；最后一条 `rg` 无输出。若发现测试工具的绝对路径仍指向旧目录，只修正导入，不添加兼容目录。

- [x] **Step 6: 提交 backend/frontend Task 2**

Backend:

```powershell
git add -- internal/runtime/realtime.go internal/runtime/realtime_test.go internal/runtime/worker_test.go internal/architecture/wave04_boundaries_test.go
git diff --cached --check
git commit -m "refactor(realtime): share publisher boundary"
```

Frontend:

```powershell
git add -- src/utils/realtime src/app/kernel.ts src/main.ts src/adapters/web/websocket.ts src/api/ai/billing-error.ts src/api/ai/chat-events.ts src/features/ai-chat/workflow.ts tests/unit/realtime tests/integration/realtime tests/integration/features/support.ts tests/shared/http/ai-conversation-websocket-contract.test.ts
git diff --cached --check
git commit -m "refactor(realtime): move client to utils"
```

### Task 3: 收紧 COS 直传边界并迁移前端目录

**Files:**
- Modify: `E:/admin/admin_front_ts/src/lib/upload/uploadClient.ts`
- Move: `E:/admin/admin_front_ts/src/lib/upload/` -> `E:/admin/admin_front_ts/src/utils/upload/`
- Modify imports in `src/components/UpFile/src/index.vue`, `src/components/UpMedia/src/index.vue`, `src/views/Main/component/upload/components/UpMediaList.vue`, `src/views/Main/component/display/components/Editor.vue`, `src/views/Main/ai/chat/components/MessageInput/use-attachments.ts`, `src/api/ai/context.ts`
- Modify: `E:/admin/admin_front_ts/tests/unit/upload/uploadClient.test.ts`
- Modify imports in `E:/admin/admin_front_ts/tests/shared/system/upload-client-url.test.ts`, `tests/component/ai/ChatAttachmentCancellation.test.ts`, `tests/component/ai/ChatAttachments.test.ts`

- [x] **Step 1: 写前端上传失败测试**

在 `tests/unit/upload/uploadClient.test.ts` 增加两个用例，复用现有 `cosMocks` 和 `config()`：

```ts
it('rejects when COS reports an upload error', async () => {
  const error = new Error('cos network failed')
  cosMocks.putObject.mockImplementation((_params: unknown, callback: (error: Error | null, data?: { ETag?: string }) => void) => {
    callback(error)
  })
  await expect(uploadFileToCloud(new File(['pdf'], 'report.pdf'), config())).rejects.toBe(error)
})

it('does not resolve a successful upload when COS omits callback data', async () => {
  cosMocks.putObject.mockImplementation((_params: unknown, callback: (error: Error | null, data?: { ETag?: string }) => void) => {
    callback(null, undefined)
  })
  await expect(uploadFileToCloud(new File(['pdf'], 'report.pdf'), config())).rejects.toThrow('COS upload returned an empty result')
})
```

测试必须复用文件中现有的 fake COS 注入方式，不访问真实 COS。

- [x] **Step 2: 运行测试确认缺口**

```powershell
npx vitest run --no-file-parallelism tests/unit/upload/uploadClient.test.ts
```

Expected: 新增的空 callback 用例在当前实现下 FAIL；错误 callback 用例应保持 PASS 或明确暴露当前差异。

- [x] **Step 3: 实现严格成功回调**

在 `src/utils/upload/uploadClient.ts` 的 COS SDK 回调中保留错误优先和单次 settle，并加入明确的数据检查：

```ts
if (error) {
  finish(() => reject(error))
  return
}
if (!data) {
  finish(() => reject(new Error('COS upload returned an empty result')))
  return
}
const url = buildPublicFileURL(config.bucket_domain, bucket, region, key)
finish(() => resolve({ url, key, etag: data.ETag }))
```

不得把缺失 ETag 伪装成失败；只有整个 SDK result 缺失才拒绝。保留 AbortController、`cancelTask` 和单次 settle 逻辑。

- [x] **Step 4: 迁移前端上传目录和导入**

在 `E:/admin/admin_front_ts` 执行：

```powershell
git mv src/lib/upload src/utils/upload
```

将所有 `@/lib/upload` 改为 `@/utils/upload`，不迁移 `src/lib/http`、`src/lib/browser` 或其他历史目录。

- [x] **Step 5: 复用现有后端 COS 边界测试**

不要修改后端生产代码或重复创建测试。以下现有测试已经锁定本波的后端边界：

```text
internal/module/uploadtoken/service_test.go:
  TestCreateRejectsNonCOSDriver
  TestCreateDoesNotExposeDriverSecrets
internal/infra/storage/cos/signer_test.go:
  TestSignerRejectsInvalidInput
  TestSignerCallsSTSWithScopedPolicy
```

Step 6 的 Go 定向命令必须执行这些测试；若它们失败，只修复本波引入的回归，不扩大到 COS 业务重构。

- [x] **Step 6: 运行 COS 定向测试和结构检查**

Backend:

```powershell
go test ./internal/module/uploadtoken ./internal/infra/storage/cos -run 'Test(CreateRejectsNonCOSDriver|CreateDoesNotExposeDriverSecrets|SignerRejectsInvalidInput|SignerCallsSTSWithScopedPolicy)' -count=1
```

Frontend:

```powershell
npx vitest run --no-file-parallelism tests/unit/upload tests/shared/system/upload-client-url.test.ts tests/shared/system/upload-rule-selection.test.ts
npx eslint src/utils/upload src/components/UpFile src/components/UpMedia src/views/Main/component/upload src/views/Main/component/display/components/Editor.vue src/views/Main/ai/chat/components/MessageInput/use-attachments.ts src/api/ai/context.ts
rg -n "@/lib/upload|src/lib/upload|from './lib/upload'" src tests
```

Expected: Go/Vitest/ESLint PASS；最后一条 `rg` 无输出。

- [x] **Step 7: 提交 frontend Task 3**

Frontend:

```powershell
git add -- src/utils/upload src/components/UpFile/src/index.vue src/components/UpMedia/src/index.vue src/views/Main/component/upload/components/UpMediaList.vue src/views/Main/component/display/components/Editor.vue src/views/Main/ai/chat/components/MessageInput/use-attachments.ts src/api/ai/context.ts tests/unit/upload tests/shared/system/upload-client-url.test.ts tests/component/ai/ChatAttachmentCancellation.test.ts tests/component/ai/ChatAttachments.test.ts
git diff --cached --check
git commit -m "refactor(upload): move direct COS client to utils"
```

### Task 4: 更新总索引与 Wave 04 交接记录

**Files:**
- Modify: `E:/admin/admin_back_go/docs/superpowers/plans/2026-08-13-admin-architecture-reduction-execution-index.md`
- Create: `E:/admin/admin_back_go/docs/superpowers/plans/2026-08-24-admin-architecture-reduction-wave-04-handoff.md`

- [x] **Step 1: 先完成人工黑盒验收**

用户人工检查：

- 页面登录后 Realtime 连接、断线重连和 AI/通知终态刷新正常；
- 文件、图片、AI 附件和导出上传仍能直传 COS；
- 上传取消、失败时页面不会显示成功；
- Worker 启动后队列和数据库定时任务仍可工作。

没有人工验收通过，不得把 Wave 04 标记为完成。

- [x] **Step 2: 写交接记录**

交接记录必须包含：backend/frontend 最终提交、每条定向命令及 PASS/FAIL、未运行的长脚本、用户黑盒结果、计划外问题和 Wave 05 入口。明确写出：没有 SQL、没有数据库迁移、没有合同生成、没有 AI/支付改动。

- [x] **Step 3: 更新总索引**

把 Wave 04 从“计划”改为“已完成”或“等待人工验收”，并挂载：

```text
docs/superpowers/specs/2026-08-24-admin-architecture-reduction-wave-04-design.md
docs/superpowers/plans/2026-08-24-admin-architecture-reduction-wave-04.md
docs/superpowers/plans/2026-08-24-admin-architecture-reduction-wave-04-handoff.md
```

不要改写已完成 Wave 01-03 的历史事实。

- [x] **Step 4: 最终短检查并提交文档**

Backend:

```powershell
git diff --check
git status --short
git add -- docs/superpowers/plans/2026-08-13-admin-architecture-reduction-execution-index.md docs/superpowers/plans/2026-08-24-admin-architecture-reduction-wave-04-handoff.md
git commit -m "docs(plan): record wave04 handoff"
```

Frontend:

```powershell
git diff --check
git status --short
```

Expected: 两个仓库工作区干净；不得运行全量质量门禁。

## Wave 04 完成门槛

只有以下条件全部满足，才能向用户报告 Wave 04 完成：

1. Task 1-3 的 backend/frontend 提交存在且工作区干净；
2. Worker、Realtime、COS 定向测试全部 PASS；
3. `rg` 确认旧 `src/modules/realtime`、`src/lib/upload` 和空静态调度适配器无残留；
4. 事件合同、数据库表、任务类型和 API JSON 未改变；
5. 用户完成黑盒验收；
6. 总索引和交接记录已提交；
7. 明确停止在 Wave 04，不进入 Wave 05 支付或 Wave 06 AI。
