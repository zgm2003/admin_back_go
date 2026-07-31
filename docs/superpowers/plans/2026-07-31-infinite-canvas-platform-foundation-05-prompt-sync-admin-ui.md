# 提示词同步、Admin 管理与 Canvas 只读库 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将提示词及六个来源完全迁到服务端，由 Admin 管理、Cron 入队、Worker 安全拉取并原子同步，Canvas 用户只读消费启用内容。

**Architecture:** `ai/prompt` capability 同时拥有 manual prompt、source 配置和同步事务；Admin transport 固定管理 `platform=infinite_canvas`，Canvas transport 固定只读同一平台。远端 fetcher 通过自定义 HTTPS dial/redirect policy 阻断 SSRF，Worker 获取每来源 Redis lease 后严格解析完整快照，成功才 upsert/下架，失败只更新 attempt/error。Cron 与手动按钮统一进入 durable task registry。

**Tech Stack:** Go、GORM、Redis fencing lease、Asynq、net/http、Gin、Admin Contract、Vue 3、Element Plus、Vitest。

---

## 执行边界

- 依赖 Plan 03；可与 Plan 04 在独立 worktree 开发，但合并 `infinitecanvas/build.go`、`jobs/noop.go`、`runtime/worker.go` 时必须 rebase。
- 后端路径相对 `E:\admin\admin_back_go`；Admin 前端路径相对 `E:\admin\admin_front_ts`。
- Canvas 前端不保存来源、schedule、ETag 或远端 cache；不运行 browser interval。
- Admin prompt/source workflow 固定目标平台 `infinite_canvas`，request 不接受任意 platform。
- 远端响应上限 16 MiB、每来源最多 5,000 条、HTTP 总超时 30 秒、最多 3 次 HTTPS redirect。
- source prompt 内容由同步维护；Admin 只能启停，不能编辑内容或普通删除。manual prompt 可完整 CRUD。
- cover/reference URL 保持 HTTPS 远端地址，本期不镜像 COS；所有显示按纯文本，不执行远端 HTML。

## 文件结构

**Create:**

- `internal/module/ai/prompt/source_model.go`、`source_dto.go`、`source_service.go` 及测试。
- `internal/module/ai/prompt/public_url.go`、`public_url_test.go`：HTTPS/public DNS/IP 校验。
- `internal/module/ai/prompt/fetcher.go`、`fetcher_test.go`：SSRF-safe conditional HTTP。
- `internal/module/ai/prompt/feed.go`、`feed_test.go`：严格 feed schema/limits。
- `internal/module/ai/prompt/sync.go`、`sync_test.go`：lease + snapshot transaction。
- `internal/module/ai/prompt/jobs.go`、`jobs_test.go`：dispatch/source durable tasks。
- `internal/module/ai/prompt/transport/admin/{route,request,handler}.go` 及测试。
- `internal/module/ai/prompt/transport/infinitecanvas/{route,request,handler,presenter}.go` 及测试。
- `E:\admin\admin_front_ts\src/api/ai/prompts.ts`、`prompt-sources.ts`。
- `E:\admin\admin_front_ts\src/views/Main/ai/prompts/**`、`promptSources/**`。
- `E:\admin\admin_front_ts\tests/shared/ai/{ai-prompt-api,prompt-source-api}.test.ts`。
- `E:\admin\admin_front_ts\tests/component/ai/{PromptManagement,PromptSourceManagement}.test.ts`。

**Modify:**

- `internal/module/ai/prompt/{model,dto,repository,service}.go` 及测试。
- `internal/platform/admin/{graph,build}.go`、`internal/platform/infinitecanvas/{graph,build}.go`。
- `internal/jobs/noop.go`、`internal/module/crontask/registry.go`、`internal/runtime/worker.go` 及测试。
- `internal/server/routes_admin_ai.go`、`router.go`、route goldens。
- `internal/admincontract/**`、`internal/infinitecanvascontract/**`、两个 generated bundle。
- `internal/admincontract/views.go`：新增两个 Admin local view keys。
- Admin 前端 generated contract、view registry、i18n。

### Task 1: 将 ai_prompts 收敛为 platform/origin-aware capability

**Files:**
- Modify: `internal/module/ai/prompt/{model,dto,repository,service}.go`
- Modify: corresponding tests

- [ ] **Step 1: 写 manual/source 行为失败测试**

覆盖：所有 query 带 platform；manual slug 只在平台内唯一；source prompt 普通 update/delete 被拒绝；status 可切换；Canvas public list 只返回 active manual 或 active source 下的 active prompt；来源 disabled/deleted 时 prompt 不可见；tags/category/keyword 分页稳定。

- [ ] **Step 2: 定义闭合 model 和 DTO**

```go
const (
    OriginManual = "manual"
    OriginSource = "source"
)

type Prompt struct {
    ID int64; Platform, OriginType string; SourceID *int64; ExternalID string
    Slug, Category, Title, Description, Prompt, Preview, CoverURL string
    ReferenceURLsJSON, TagsJSON, SourceURL string
    Status, IsDel int
    CreatedAt, UpdatedAt time.Time
}

type ListQuery struct {
    Platform string
    CurrentPage, PageSize int
    Keyword, Category, OriginType string
    SourceID int64
    Tags []string
    Status, IsDel int
    PublicOnly bool
}

type ManualInput struct {
    Platform, Slug, Category, Title, Description, Prompt, Preview, CoverURL string
    ReferenceURLs, Tags []string
    Status int
}
```

response `Item` 返回结构化 `reference_urls []string` 和 `tags []string`，不把 JSON string 泄露给前端；包含 `origin_type/source_id/source_name/external_id` 供 Admin 展示，Canvas presenter 不返回内部 ETag/error。

- [ ] **Step 3: 改写 repository SQL**

所有 CRUD 接受 platform。Public list 使用：

```sql
FROM ai_prompts AS p
LEFT JOIN ai_prompt_sources AS s ON s.id=p.source_id AND s.platform=p.platform
WHERE p.platform=? AND p.status=1 AND p.is_del=2
  AND (p.origin_type='manual' OR (s.status=1 AND s.is_del=2))
```

`UpdateManual/SoftDeleteManual` 条件包含 `origin_type='manual'`；`ChangeStatus` 可作用两种 origin。重复键映射为 `ai.prompt.slug.conflict`，不返回 SQL error。

- [ ] **Step 4: 实现字段限制和纯文本语义**

manual/service 与 feed 共用限制：slug 1..191 ASCII-safe；category/title <=191 code points；description/preview <=2,000；prompt <=100,000；tags 最多 32、每项 64；reference URLs 最多 16。HTML 字符串作为普通文本保存，不在 service 清洗成 HTML。

- [ ] **Step 5: 运行 prompt core tests 并提交**

```powershell
go test ./internal/module/ai/prompt -run 'Manual|Public|Origin|Platform' -count=1
```

Expected: PASS。

```bash
git add internal/module/ai/prompt
git commit -m "feat(prompt): 收敛平台提示词模型"
```

### Task 2: 实现来源 CRUD、删除互斥和 URL 校验

**Files:**
- Create: `internal/module/ai/prompt/{source_model,source_dto,source_service,public_url}.go`
- Modify: `internal/module/ai/prompt/repository.go`
- Test: source/public URL tests

- [ ] **Step 1: 写来源验证和并发删除失败测试**

覆盖：https GitHub URL 通过；http、userinfo、fragment、非 443 port、localhost、loopback、private、link-local、multicast、unspecified、IPv4-mapped IPv6 失败；redirect 后重新校验；同 platform code 唯一；删除时同步 lease 已占用返回 409；删除成功同事务 soft delete source + source prompts，manual 不变。

- [ ] **Step 2: 定义 source DTO/repository contract**

```go
type Source struct {
    ID int64; Platform, Code, Name, FeedURL, HomepageURL string
    Status int; LastAttemptAt, LastSuccessAt *time.Time
    LastErrorSummary, ETag, LastModified string
    IsDel int; CreatedAt, UpdatedAt time.Time
}
type SourceInput struct {
    Code, Name, FeedURL, HomepageURL string
    Status int
}
type SourceListQuery struct {
    Platform string; CurrentPage, PageSize int
    Keyword string; Status int
}
```

repository 增加 `WithTx`、source list/detail/create/update/status/delete、`SoftDeletePromptsBySource`、`EnabledSources`。所有 method 强制 platform。

- [ ] **Step 3: 实现 public target validator**

`ValidatePublicHTTPSURL(ctx,resolver,raw)` 要求：scheme https、无 userinfo/query secret 检查、host 非空、port 空或 443；解析 host 的所有 IP，只要任一 IP 是 private/loopback/link-local/multicast/unspecified 就拒绝。DNS 为空或解析失败 fail closed。返回 normalized URL，不静默补 scheme。

- [ ] **Step 4: 用同一 source lease 保护 delete/sync**

定义窄接口适配 `redislock.LeaseStore`：

```go
type SourceLease interface {
    Acquire(context.Context, int64, time.Duration) (func(context.Context) error, error)
}
```

key 固定 `infinite-canvas:prompt-source:{id}`。Delete acquire 30 秒 lease，失败映射 `409 ai.prompt_source.sync_in_progress`；获得后 transaction lock source、soft delete source 和其 source prompts，最后释放。

- [ ] **Step 5: 运行来源 tests 并提交**

```powershell
go test ./internal/module/ai/prompt -run 'Source|PublicHTTPS|DeleteLease' -count=1
```

Expected: PASS。

```bash
git add internal/module/ai/prompt
git commit -m "feat(prompt): 管理安全提示词来源"
```

### Task 3: 建立 SSRF-safe conditional fetcher 和严格 feed parser

**Files:**
- Create: `internal/module/ai/prompt/{fetcher,fetcher_test,feed,feed_test}.go`

- [ ] **Step 1: 写 fetch 安全边界失败测试**

fake resolver/dialer/httptest 覆盖：DNS 返回 public IP 才 dial；混合 public+private 全拒绝；每次 redirect 重新解析；redirect 到 HTTP/private 拒绝；超过三跳、30 秒、16 MiB 失败；304 返回 NotModified；ETag/Last-Modified 请求头准确；环境 HTTP_PROXY 不被使用；error 不含完整 response body。

- [ ] **Step 2: 实现固定 DNS 结果的 DialContext**

Fetcher 不使用 `http.DefaultTransport`：

```go
transport := &http.Transport{
    Proxy: nil,
    DialContext: validatedDialer.DialContext,
    TLSHandshakeTimeout: 10 * time.Second,
    ResponseHeaderTimeout: 15 * time.Second,
    DisableCompression: false,
}
client := &http.Client{
    Transport: transport,
    Timeout: 30 * time.Second,
    CheckRedirect: validateRedirect(3),
}
```

Dialer 解析原始 hostname，验证全部 IP，再只拨已验证 IP + original port；TLS 仍使用原始 request hostname 做 SNI。每次新 host/redirect 都重新验证，禁止 DNS rebinding 回到默认 resolver。

- [ ] **Step 3: 定义 fetch 结果**

```go
type FetchInput struct { URL, ETag, LastModified string }
type FetchResult struct {
    Body []byte
    ETag, LastModified string
    NotModified bool
}
```

读取使用 `io.LimitReader(response.Body, 16<<20+1)`；只有这个 bounded fetcher 可持有 body bytes。status 非 200/304 返回包含 status、不含 body 的 typed error。

- [ ] **Step 4: 写 strict feed schema 和限制**

```go
type FeedItem struct {
    ID string `json:"id"`
    Title string `json:"title"`
    Prompt string `json:"prompt"`
    Description string `json:"description"`
    CoverURL string `json:"coverUrl"`
    ReferenceImageURLs []string `json:"referenceImageUrls"`
    Tags []string `json:"tags"`
    Preview string `json:"preview"`
    CreatedAt string `json:"createdAt"`
    UpdatedAt string `json:"updatedAt"`
    Author string `json:"author,omitempty"`
    SourceURL string `json:"sourceUrl,omitempty"`
}
```

decoder `DisallowUnknownFields`，根必须数组，最多 5,000。每条必须有非空且来源内唯一 ID/title/prompt；任何非法条目使整次来源失败，不跳过、不生成 fallback ID。cover/reference/source URL 解析为相对 feed URL 后必须是 public HTTPS；未知 `imageMode/imageModel/imageSize/imageCount` 不属于服务端提示词契约并应被拒绝。

- [ ] **Step 5: 运行 fetch/parser tests 并提交**

```powershell
go test ./internal/module/ai/prompt -run 'Fetcher|SSRF|Feed|Redirect|ResponseLimit' -count=1
```

Expected: PASS；全部 fail-closed case 有确定错误。

```bash
git add internal/module/ai/prompt/fetcher.go internal/module/ai/prompt/fetcher_test.go internal/module/ai/prompt/feed.go internal/module/ai/prompt/feed_test.go
git commit -m "feat(prompt): 安全拉取并严格解析来源"
```

### Task 4: 实现来源快照同步和 durable jobs

**Files:**
- Create: `internal/module/ai/prompt/{sync,sync_test,jobs,jobs_test}.go`
- Modify: `internal/jobs/noop.go`
- Modify: `internal/module/crontask/registry.go`
- Modify: `internal/runtime/worker.go`
- Test: jobs/crontask/runtime tests

- [ ] **Step 1: 写完整同步语义失败测试**

覆盖：lease 不可得不执行 HTTP；200 全量 upsert；再次同步幂等；消失 external_id 仅在成功事务后 soft delete；重现恢复；304 更新 attempt/success 但不改 rows；parse/HTTP/DB 失败保留上次成功 prompts/last_success/etag；status 不被 sync 覆盖；source 删除后不能提交；错误摘要 <=512 且无 URL query/body/credential。

- [ ] **Step 2: 定义 SyncService 和 repository transaction**

```go
type SyncService struct {
    repository Repository
    fetcher SourceFetcher
    leases SourceLease
    clock clock.Clock
    logger *slog.Logger
    telemetry telemetry.Recorder
}
type SyncResult struct {
    SourceID int64
    Count, Created, Updated, Removed int
    NotModified bool
}
func (s *SyncService) SyncSource(context.Context, int64) (SyncResult, *apperror.Error)
```

同步 lease TTL 2 分钟，job timeout 90 秒。fetch/parse 后 transaction 再 lock active source；upsert 只更新 content 字段、`is_del=2`，保留已存在 prompt.status；新行 status=1。快照 missing soft delete 只在所有 upsert 成功后执行。

- [ ] **Step 3: 定义两个 task type 和 payload**

```go
const (
    TypePromptSyncDispatchV1 = "infinite-canvas:prompt-sync-dispatch:v1"
    TypePromptSourceSyncV1 = "infinite-canvas:prompt-source-sync:v1"
)
type PromptSyncDispatchPayload struct{}
type PromptSourceSyncPayload struct { SourceID int64 `json:"source_id"` }
```

dispatch handler 列出 enabled source ids 并逐一 enqueue source task；单个 enqueue 失败让 dispatch retry。source task queue low、timeout 2 分钟、max retry 5、unique TTL 5 分钟；dispatch unique TTL 30 分钟。

- [ ] **Step 4: 注册 cron registry**

`crontask.NewDefaultRegistry()` 增加：

```go
RegistryEntry{
    Name: "infinite_canvas_prompt_sync",
    TaskType: prompt.TypePromptSyncDispatchV1,
    Description: "同步无限画布提示词来源",
    BuildTask: func() (taskqueue.Task, error) { return prompt.NewSyncDispatchTask() },
}
```

Cron 只创建 dispatch task，不直接请求网络。Plan 01 row 仍 disabled。

- [ ] **Step 5: 装配 Worker**

`jobs.Dependencies` 增加 `PromptSyncDispatcher` 和 `PromptSourceSyncer` 窄接口。Worker 使用 DB repo、validated fetcher、`redislock.New(resources.Redis.Redis)`、clock/logger/telemetry 构造 SyncService；API graph 不拥有 fetcher，不执行远端同步。

- [ ] **Step 6: 运行 sync/jobs/runtime tests 并提交**

```powershell
go test ./internal/module/ai/prompt ./internal/jobs ./internal/module/crontask ./internal/runtime -run 'Prompt|Sync|Cron' -count=1
```

Expected: PASS；cron test 证明只 enqueue。

```bash
git add internal/module/ai/prompt internal/jobs internal/module/crontask internal/runtime
git commit -m "feat(prompt): 接入持久化来源同步任务"
```

### Task 5: 发布 Admin 管理 routes、Canvas 只读 routes 和两个 Bundle

**Files:**
- Create: prompt Admin/Canvas transport files
- Modify: platform graphs/builds、server route files/goldens
- Modify: Admin/Canvas contract packages and generated bundles

- [ ] **Step 1: 写 Admin/Canvas handler 失败测试**

Admin tests 证明 request platform 被 strict decoder 拒绝，所有 service input 固定 Canvas platform；source prompt update/delete 被映射为 409；manual sync 只返回 queue identity，不等待网络。Canvas tests 证明只返回 public rows、详情越权/disabled 404、response 文本未作为 HTML。

- [ ] **Step 2: 注册 Admin 资源 routes**

```text
GET    /api/admin/v1/ai/prompts                       ai_prompt_list
POST   /api/admin/v1/ai/prompts                       ai_prompt_create
GET    /api/admin/v1/ai/prompts/:id                   ai_prompt_detail
PUT    /api/admin/v1/ai/prompts/:id                   ai_prompt_update
DELETE /api/admin/v1/ai/prompts/:id                   ai_prompt_delete
PATCH  /api/admin/v1/ai/prompts/:id/status            ai_prompt_status
GET    /api/admin/v1/ai/prompt-sources                ai_prompt_source_list
POST   /api/admin/v1/ai/prompt-sources                ai_prompt_source_create
GET    /api/admin/v1/ai/prompt-sources/:id            ai_prompt_source_detail
PUT    /api/admin/v1/ai/prompt-sources/:id            ai_prompt_source_update
DELETE /api/admin/v1/ai/prompt-sources/:id            ai_prompt_source_delete
PATCH  /api/admin/v1/ai/prompt-sources/:id/status     ai_prompt_source_status
POST   /api/admin/v1/ai/prompt-sync-jobs              ai_prompt_sync
```

所有 mutation 有 operation audit；prompt body 可记录安全摘要但跳过完整 prompt，source URL audit 去除 query。Sync request 为 `{source_id?: positive int}`，response：

```go
type SyncJobResponse struct {
    ID string `json:"id"`
    Type string `json:"type"`
    Queue string `json:"queue"`
}
```

- [ ] **Step 3: 注册 Canvas 只读 routes**

```text
GET /api/infinite-canvas/v1/prompts       infinite_canvas_prompt_read
GET /api/infinite-canvas/v1/prompts/:id   infinite_canvas_prompt_read
```

query 精确为 `current_page/page_size/keyword/category/tag[]`；response 提供结构化 categories/tags/page。无来源配置、刷新、同步或 status mutation route。

- [ ] **Step 4: 更新两个 graph 和 route goldens**

Admin graph `AI.Prompts` 和 `AI.PromptSources` 可以指向同一个 prompt service；Canvas graph `Workspace.Prompts` 只暴露只读接口。API graph 构造 manual/source service 与 queue enqueuer，绝不构造 remote fetcher。

- [ ] **Step 5: 发布开发期 Bundles**

```powershell
$commit = (git rev-parse HEAD).Trim()
pwsh -NoProfile -File scripts/generate-admin-contract.ps1 -BackendCommit $commit
pwsh -NoProfile -File scripts/generate-infinite-canvas-contract.ps1 -BackendCommit $commit
go test ./internal/admincontract ./internal/infinitecanvascontract -count=1
```

Expected: Admin OpenAPI 有 13 条管理 routes；Canvas OpenAPI 只新增两条只读 routes；Canvas permissions 仍只有既定六项。

- [ ] **Step 6: 提交 transports/contracts**

```bash
git add internal/module/ai/prompt/transport internal/platform/admin internal/platform/infinitecanvas internal/server internal/admincontract internal/infinitecanvascontract contracts/admin/v1 contracts/infinite-canvas/v1
git commit -m "feat(prompt): 发布管理与画布提示词接口"
```

### Task 6: 实现 Admin 提示词与来源管理页面

**Files:**
- Create: Admin frontend API/views/tests listed above
- Modify: generated contract、routing views、i18n

- [ ] **Step 1: 同步 contract 并写 API 失败测试**

```powershell
$manifest = Get-Content -Raw E:/admin/admin_back_go/contracts/admin/v1/manifest.json | ConvertFrom-Json
npm run contract:sync -- --backend E:/admin/admin_back_go --commit $manifest.backend_commit
npm run contract:generate
```

API tests 断言 GET query、REST methods/paths、manual mutation body、source mutation、status patch 和 sync job body 精确来自 generated operations；不接受 `platform`、`tags_json` 或旧 POST action URL。

Run: `npm test -- tests/shared/ai/ai-prompt-api.test.ts tests/shared/ai/prompt-source-api.test.ts`

Expected: FAIL，API adapters 尚不存在。

- [ ] **Step 2: 实现 generated-type-derived API adapters**

`src/api/ai/prompts.ts` 和 `prompt-sources.ts` 只从 `AdminOperationInput/Output` 派生类型。tags/reference URLs 直接使用 contract arrays；不得 JSON.parse 手写字段。Sync 返回 `id/type/queue` 并交给现有 queue monitor，不轮询公网来源。

- [ ] **Step 3: 写页面组件失败测试**

Prompt 页面覆盖 loading/success/empty/error、manual/source badge、source prompt 编辑/删除按钮禁用但 status 可切、长 prompt drawer 纯文本。Source 页面覆盖 create/edit/status/delete、last attempt/success/error、单来源/全部同步按钮、同步返回 job notification。

- [ ] **Step 4: 实现两个工作型页面**

- `/ai/prompts`：紧凑表格 + search/filter + manual editor drawer；列为 title/category/origin/source/status/updated/actions。
- `/ai/prompt-sources`：表格列为 name/code/feed/homepage/status/count/last success/error/actions；URL 使用安全外链 icon，不把整 URL 塞进窄列。
- sync 使用刷新 icon button + tooltip；create/update/delete 使用项目现有 command patterns。
- 页面 sections 不套卡片，不使用超大标题；移动端 action 收进 menu，文本不溢出。

source prompt 内容使用 `<pre>{{ prompt }}</pre>` 或 text binding，禁止 `v-html`。错误状态与 empty 状态分开。

- [ ] **Step 5: 注册 views/i18n 并运行门禁**

后端 `views.go` 和 permissions 指向 `ai/prompts/index`、`ai/promptSources/index`；前端运行 `routes:generate`。补齐中英文文案后：

```powershell
npm run routes:generate
npm run locale:generate
npm test -- tests/shared/ai/ai-prompt-api.test.ts tests/shared/ai/prompt-source-api.test.ts tests/component/ai/PromptManagement.test.ts tests/component/ai/PromptSourceManagement.test.ts
npm run contract:check
npm run routes:check
npm run locale:check
npm run typecheck
npm run build
```

Expected: 全部退出 0。

- [ ] **Step 6: 提交 Admin 前端**

```bash
git add contracts/backend/admin src/modules/http/generated src/api/ai/prompts.ts src/api/ai/prompt-sources.ts src/views/Main/ai/prompts src/views/Main/ai/promptSources src/modules/routing/generated src/i18n tests/shared/ai/ai-prompt-api.test.ts tests/shared/ai/prompt-source-api.test.ts tests/component/ai/PromptManagement.test.ts tests/component/ai/PromptSourceManagement.test.ts
git commit -m "feat(prompt): 增加提示词来源管理页面"
```

### Task 7: 执行提示词安全与完整性门禁

**Files:**
- Verify: all prompt/backend/frontend files

- [ ] **Step 1: 运行后端完整定向测试**

```powershell
go test ./internal/module/ai/prompt/... ./internal/jobs ./internal/module/crontask ./internal/platform/admin ./internal/platform/infinitecanvas ./internal/server ./internal/runtime ./internal/admincontract ./internal/infinitecanvascontract -count=1
git diff --check
```

Expected: PASS。

- [ ] **Step 2: 运行静态安全扫描**

```powershell
rg -n 'http\.DefaultClient|http\.Get\(|ProxyFromEnvironment|InsecureSkipVerify|v-html|dangerouslySetInnerHTML' internal/module/ai/prompt E:/admin/admin_front_ts/src/views/Main/ai/prompts E:/admin/admin_front_ts/src/views/Main/ai/promptSources
rg -n 'setInterval|localforage|prompt-source-presets|raw\.githubusercontent' E:/admin/admin_front_ts/src
```

Expected: 两条命令无输出；唯一远端 URL 真相在数据库 seed/source rows，Worker fetcher 使用受限 transport。

- [ ] **Step 3: 确认 cron 仍未提前启用**

读取目标数据库 migration/seed 测试和 `cron_task` fixture，确认两个新增 row status=2；runtime registry/handler 已就绪。Plan 07 才执行 activation migration。

## 完成标准

- Admin 可管理 manual prompts、来源和 status，可创建同一 durable sync job；Canvas 只能读启用内容。
- Worker 对每来源持 lease，使用 ETag/Last-Modified，严格完整解析并原子 upsert/下架。
- SSRF、redirect、DNS rebinding、timeout、response/entry/field limits 全部有测试。
- 同步失败保留最后成功数据与 last_success，只更新安全 error summary。
- Admin 新权限未自动授权；两个 cron handler 已注册但 row 仍 disabled。
