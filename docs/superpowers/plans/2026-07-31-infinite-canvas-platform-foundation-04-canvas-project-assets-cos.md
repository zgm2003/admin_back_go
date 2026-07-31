# Canvas 项目、素材与私有 COS Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付用户隔离的服务端画布项目、严格 `CanvasDocumentV1`、revision 冲突、素材引用事务、JPEG/PNG/WebP 私有 COS 直传确认、短期签名读取和过期上传清理。

**Architecture:** `canvasproject` 拥有项目、canonical document、revision 和引用关系；既有 `ai/asset` 被收紧为 platform-aware 素材 capability。浏览器先创建 upload intent 获取单 key STS，再直传 COS；服务端确认时通过独立 private object gateway 做 HEAD 和最多 20 MiB 的流式校验，最后在事务中创建唯一素材并消费 intent。项目保存与素材删除都在数据库约束和 owner scope 内收敛。

**Tech Stack:** Go、GORM/MySQL transaction、Gin、腾讯云 COS SDK/STS、Redis lease、Asynq、OpenAPI Contract。

---

## 执行边界

> **并行与提交覆盖规则：** 实施时同时遵守 `E:\admin\LONG_TASK_PARALLEL_EXECUTION.md` 和 execution index。Project、Asset/COS 使用不同 worktree 和独占目录；子执行器只返回 diff/测试证据，不运行 `git add`、`git commit`、merge、rebase 或 Contract 生成。`internal/platform/**`、`internal/server/**`、`internal/jobs/**`、`internal/module/crontask/**`、`internal/runtime/**` 和 Contract 全部由主线程在 Wave 3I 一次性集成；下文“提交”步骤均为主线程检查点。

- 依赖 Plan 03 的 Canvas graph、trusted route group 和 Contract Bundle。
- 还依赖 AI native file attachment 工作已经合并并形成干净 backend 基线；实施前必须重新读取现有 `internal/infra/storage/cos/object_inspector.go`、`object_stream.go`、`object_reader.go`、`object_writer.go`、`signer.go` 及测试。
- 所有后端路径相对 `E:\admin\admin_back_go`。
- 本 Plan 不实现 AI 生成、图片编辑处理、视频/音频素材写入、永久公开 URL 或素材物理删除任务。
- 所有 owner query 必须在 SQL 中包含 `platform='infinite_canvas' + user_id + is_del=2`；越权与不存在统一 `404`。
- COS bucket 保持私有。object key 固定为 `infinite-canvas/users/{userId}/assets/{yyyy}/{mm}/{32 lowercase hex}.{jpg|png|webp}`；客户端不能提交 key。
- 单图最大 `20 MiB`；intent TTL 10 分钟；读取签名 TTL 5 分钟；项目 JSON 最大 5 MiB。
- Canvas COS 能力必须复用现有 config provider、signed client、HEAD/GET metadata、context body 和错误映射。允许抽取 transport-neutral primitive，但禁止复制一套 SDK client/config/error 实现；AI key policy 与 Canvas key policy 保持各自 adapter，不能互相放宽 namespace。

## 文件结构

**Create:**

- `internal/module/canvasproject/{model,dto,document,repository,service}.go` 及对应测试。
- `internal/module/canvasproject/transport/infinitecanvas/{route,request,handler,presenter}.go` 及测试。
- `internal/infra/storage/cos/private_object_gateway.go`、`private_object_gateway_test.go`：从现有 inspector/streamer 抽出的 transport-neutral private object core。
- `internal/module/ai/asset/{upload_intent,image_verifier,upload_service,cleanup_job}.go` 及测试。
- `internal/module/ai/asset/transport/infinitecanvas/{route,request,handler,presenter}.go` 及测试。
- `internal/architecture/infinite_canvas_resource_isolation_test.go`。

**Modify:**

- `internal/module/ai/asset/{model,dto,repository,service}.go` 及测试。
- `internal/infra/storage/cos/{object_inspector,object_stream}.go` 及测试：保持 AI adapter 行为并改为复用 private object core。
- `internal/infra/storage/cos/signer.go`、`signer_test.go`：证明 policy 仍只授权单个 key。

**Main-thread integration only:**

- `internal/platform/infinitecanvas/{graph,build}.go` 及测试。
- `internal/server/router.go`、Canvas route golden。
- `internal/jobs/noop.go`、`noop_test.go`：注册 cleanup task definition。
- `internal/module/crontask/registry.go`、`registry_test.go`：注册 disabled seed 对应 entry。
- `internal/runtime/worker.go`、`worker_test.go`：装配 cleanup handler。
- `internal/infinitecanvascontract/**`、`contracts/infinite-canvas/v1/**`。

### Task 1: 定义严格、可生成契约的 CanvasDocumentV1

**Files:**
- Create: `internal/module/canvasproject/document.go`
- Create: `internal/module/canvasproject/document_test.go`
- Create: `internal/module/canvasproject/dto.go`

- [ ] **Step 1: 写合法文档与所有拒绝边界测试**

table-driven tests 至少包含：未知顶层/data 字段、未知 node type、重复 node/connection ID、非法 ID、断开的 connection、非 group 的 group_id、group cycle、NaN/Inf/越界坐标、宽高、viewport scale、超过 2,000 nodes/4,000 connections、标题/文本/prompt/reference 数量超限、重复 asset id、未知 schema version。

- [ ] **Step 2: 定义闭合 DTO，不使用 map 或 catch-all metadata**

```go
const (
    SchemaVersionV1 = "canvas_document_v1"
    MaxDocumentBytes int64 = 5 << 20
    MaxNodes = 2000
    MaxConnections = 4000
)

type CanvasDocumentV1 struct {
    Nodes         []CanvasNodeV1       `json:"nodes"`
    Connections   []CanvasConnectionV1 `json:"connections"`
    Viewport      CanvasViewportV1     `json:"viewport"`
    BackgroundMode string              `json:"background_mode"`
    ShowImageInfo bool                 `json:"show_image_info"`
}

type CanvasNodeV1 struct {
    ID       string           `json:"id"`
    Type     string           `json:"type"`
    Title    string           `json:"title"`
    Position CanvasPositionV1 `json:"position"`
    Width    float64          `json:"width"`
    Height   float64          `json:"height"`
    GroupID  *string          `json:"group_id"`
    Data     CanvasNodeDataV1 `json:"data"`
}

type CanvasNodeDataV1 struct {
    Content            *string  `json:"content,omitempty"`
    FontSize           *float64 `json:"font_size,omitempty"`
    AssetID            *int64   `json:"asset_id,omitempty"`
    NaturalWidth       *int     `json:"natural_width,omitempty"`
    NaturalHeight      *int     `json:"natural_height,omitempty"`
    FreeResize         *bool    `json:"free_resize,omitempty"`
    Prompt             *string  `json:"prompt,omitempty"`
    ReferencedAssetIDs []int64  `json:"referenced_asset_ids,omitempty"`
}
```

补齐 `CanvasPositionV1{X,Y}`、`CanvasViewportV1{X,Y,K}`、`CanvasConnectionV1{ID,FromNodeID,ToNodeID}`。类型值闭合为 `text/image/config/group`；background 闭合为 `dots/lines/blank`。

- [ ] **Step 3: 实现 strict decode 和按 node type 的字段判别**

`DecodeDocumentV1(raw []byte)` 使用 `json.Decoder.DisallowUnknownFields()`、只允许单个 JSON value，并先检查 `len(raw)<=5MiB`。`ValidateDocumentV1` 对每个 type 要求/禁止：

```text
text:   content/font_size only
image:  asset_id/natural_width/natural_height/free_resize only
config: prompt/referenced_asset_ids only
group:  data must be {}
```

字符串长度使用 `utf8.RuneCountInString`；ID regex 为 `^[A-Za-z0-9_-]{1,64}$`；有限数使用 `math.IsNaN/IsInf`；group graph 用三色 DFS 检测 cycle。

- [ ] **Step 4: 实现 canonical bytes、fingerprint 和素材引用提取**

```go
func CanonicalDocumentV1(document CanvasDocumentV1) ([]byte, error) {
    if err := ValidateDocumentV1(document); err != nil { return nil, err }
    return json.Marshal(document)
}

func DocumentAssetIDs(document CanvasDocumentV1) []int64 {
    // collect image asset_id and config referenced_asset_ids; sort ascending and dedupe
}
```

结构体字段顺序就是 canonical key 顺序；不得对原始 JSON 字符串做替换。默认文档 canonical 值固定为：

```json
{"nodes":[],"connections":[],"viewport":{"x":0,"y":0,"k":1},"background_mode":"dots","show_image_info":true}
```

- [ ] **Step 5: 运行 document tests 并提交**

```powershell
go test ./internal/module/canvasproject -run 'Document|Canonical|AssetIDs' -count=1
```

Expected: PASS；unknown fields 和所有限制得到确定错误。

```bash
git add internal/module/canvasproject/document.go internal/module/canvasproject/document_test.go internal/module/canvasproject/dto.go
git commit -m "feat(canvas): 定义严格画布文档"
```

### Task 2: 实现项目幂等创建、revision mutation 和素材引用事务

**Files:**
- Create: `internal/module/canvasproject/{model,repository,repository_test,service,service_test}.go`

- [ ] **Step 1: 写 repository/service 失败测试**

覆盖：同 request_id+fingerprint 返回同项目且 `Replayed=true`；同 key 不同 fingerprint 返回 `ErrIdempotencyConflict`；列表 SQL 不 select `document_json`；rename/save expected revision 成功加一；冲突返回 current revision；跨用户/平台 404；copy 生成 revision 1；保存含越权/disabled/deleted asset 整体失败且 document/ref 都不变；删除项目不删除素材。

- [ ] **Step 2: 定义 model、command 和 result**

```go
type Project struct {
    ID                 int64
    Platform           string
    UserID             int64
    RequestID          string
    RequestFingerprint []byte
    Title              string
    SchemaVersion      string
    DocumentJSON       []byte
    Revision           uint64
    IsDel              int
    CreatedAt          time.Time
    UpdatedAt          time.Time
}

type Owner struct { Platform string; UserID int64 }
type CreateCommand struct {
    Owner Owner; RequestID string; Fingerprint [32]byte; Title string
    SchemaVersion string; DocumentJSON []byte; AssetIDs []int64
}
type MutationResult struct { Revision uint64; UpdatedAt time.Time }
type Conflict struct { CurrentRevision uint64 }
```

API DTO 使用 `ProjectSummary` 和 `ProjectDetail`；summary 不含 document，detail 的 `Document CanvasDocumentV1` 是 typed JSON，不返回数据库原始字符串。

- [ ] **Step 3: 实现 repository owner-scoped SQL**

Repository 接口固定：

```go
List(context.Context, Owner, ListQuery) ([]ProjectSummaryRow, int64, error)
Detail(context.Context, Owner, int64) (*Project, error)
Create(context.Context, CreateCommand) (Project, bool, error)
Rename(context.Context, Owner, int64, uint64, string) (MutationResult, error)
SaveDocument(context.Context, Owner, int64, uint64, string, []byte, []int64) (MutationResult, error)
SoftDelete(context.Context, Owner, int64, uint64) (MutationResult, error)
```

Create transaction 先按 `(platform,user_id,request_id)` 查所有 is_del 状态；fingerprint 相同返回原 row，不同返回 conflict。新建时写 revision 1，并在同事务写 `canvas_project_assets`。

- [ ] **Step 4: 实现 revision 条件更新和引用同步**

Save transaction 顺序：读取 active owner project -> 验证 expected revision -> 按 asset id 升序查询并锁定 `ai_assets` active owner rows -> 数量精确相等 -> `UPDATE canvas_projects ... WHERE id=? AND platform=? AND user_id=? AND is_del=2 AND revision=?` -> 删除旧 refs -> 批量插入新 refs。RowsAffected=0 时读取 current revision 并返回 conflict。

Rename 和 SoftDelete 使用相同条件更新；title/document 共享一个 revision，不存在独立计数器。

- [ ] **Step 5: 实现 service validation/fingerprint**

Create request：`request_id` 符合 ID regex，title 1..120 code points；copy 时读取 source owner project，采用其 canonical document 和 refs。fingerprint 由以下结构 JSON 后 SHA-256 得到：

```go
type createFingerprintV1 struct {
    Version         string `json:"version"`
    Title           string `json:"title"`
    SourceProjectID *int64 `json:"source_project_id"`
}
```

普通创建使用默认 document；copy 的 source ID 是 fingerprint 的一部分。保存先 strict decode、schema version 精确匹配 v1，再调用 repository。

- [ ] **Step 6: 运行项目 tests 并提交**

```powershell
go test ./internal/module/canvasproject -run 'Project|Create|Rename|Save|Conflict|Copy|Delete' -count=1
```

Expected: PASS，事务失败不会部分更新引用。

```bash
git add internal/module/canvasproject
git commit -m "feat(canvas): 持久化版本化画布项目"
```

### Task 3: 交付项目 REST transport 和冲突契约

**Files:**
- Create: `internal/module/canvasproject/transport/infinitecanvas/**`
- Main-thread integration only: `internal/platform/infinitecanvas/{graph,build}.go`、`internal/server/router.go`
- Test in executor lane: project transport tests

- [ ] **Step 1: 写六条 route 的 handler 失败测试**

请求中的 `platform/user_id` 字段即使出现也必须因 strict JSON 被拒绝，handler owner 只来自 middleware identity + platform constant。测试 400/404/409/413、分页默认、list 不含 document、conflict body 含 `current_revision`。

- [ ] **Step 2: 定义 request contract**

```go
type createRequest struct {
    RequestID       string `json:"request_id" binding:"required,max=64"`
    Title           string `json:"title" binding:"required,max=120"`
    SourceProjectID *int64 `json:"source_project_id,omitempty"`
}
type renameRequest struct {
    Title            string `json:"title" binding:"required,max=120"`
    ExpectedRevision uint64 `json:"expected_revision" binding:"required,min=1"`
}
type saveDocumentRequest struct {
    ExpectedRevision uint64           `json:"expected_revision" binding:"required,min=1"`
    SchemaVersion    string           `json:"schema_version" binding:"required"`
    Document         json.RawMessage  `json:"document" binding:"required"`
}
type deleteRequest struct {
    ExpectedRevision uint64 `json:"expected_revision" binding:"required,min=1"`
}
```

DELETE body 在正式契约中允许且必须只有 `expected_revision`；未知字段通过 transport strict decoder 返回 400。

- [ ] **Step 3: 注册项目 routes 与 permission**

```text
GET    /projects                         infinite_canvas_project_read
POST   /projects                         infinite_canvas_project_write
GET    /projects/:id                     infinite_canvas_project_read
PATCH  /projects/:id                     infinite_canvas_project_write
PUT    /projects/:id/document            infinite_canvas_project_write
DELETE /projects/:id                     infinite_canvas_project_write
```

规格列出的项目 HTTP 共六条；复制通过 POST body 实现。所有 mutation 写 operation audit，但 `SkipRequestPayload=true` 用于 document save，日志只记录 project id/revision/size outcome，不保存完整 document。

- [ ] **Step 4: 执行器运行 transport 测试，主线程登记 integration input**

执行器只完成 transport 与 handler tests，并向主线程返回以下固定 integration input：`Graph.Workspace.Projects *canvasproject.Service`、GORM repository constructor、六条 route definitions。主线程在 Wave 3I 更新 Graph/Build/Router 和 route golden。

```powershell
go test ./internal/module/canvasproject/... -run 'Project|InfiniteCanvas' -count=1
```

Expected: PASS；执行器 diff 不包含 `internal/platform` 或 `internal/server`。

- [ ] **Step 5: 提交项目 HTTP surface**

```bash
# main-thread checkpoint after integration review
git add internal/module/canvasproject/transport
git commit -m "feat(canvas): 交付画布项目接口"
```

### Task 4: 建立私有 COS object gateway

**Files:**
- Create: `internal/infra/storage/cos/private_object_core.go`
- Create: `internal/infra/storage/cos/private_object_gateway.go`
- Create: `internal/infra/storage/cos/private_object_gateway_test.go`
- Modify: `internal/infra/storage/cos/{object_inspector,object_stream}.go`
- Modify: `internal/infra/storage/cos/{object_inspector,object_stream}_test.go`
- Modify: `internal/infra/storage/cos/signer_test.go`

- [ ] **Step 1: 写通用 core、AI adapter 回归和 Canvas policy 失败测试**

先保留现有 inspector/streamer/reader/writer/signer tests，再增加 httptest/fake client 覆盖：通用 core 禁用/缺 config fail closed；key traversal/backslash/错误 prefix 由注入的 policy 拒绝；HEAD 返回 size/type/etag/`x-cos-meta-sha256`；Open 暴露 streaming body 不调用 `io.ReadAll`；context cancel 关闭 body；Delete 404 幂等；signed URL 只含目标 key 且 5 分钟过期。回归测试证明 AI chat prefixes/ETag/If-Match 行为完全不变，Canvas policy 只接受 `infinite-canvas/users/...`。

- [ ] **Step 2: 定义 transport-neutral gateway 和显式 key validator**

```go
type PrivateObjectHead struct {
    Key            string
    Size           int64
    MIMEType       string
    ETag           string
    SHA256Metadata string
}
type PrivateObjectStream struct {
    Head PrivateObjectHead
    Body io.ReadCloser
}
type PrivateObjectGateway interface {
    Head(context.Context, string) (PrivateObjectHead, error)
    Open(context.Context, string) (*PrivateObjectStream, error)
    Delete(context.Context, string) error
    SignGetURL(context.Context, string, time.Duration) (string, time.Time, error)
}
type ObjectKeyValidator func(string) (string, error)
```

constructor 接收既有 `ObjectConfigProvider`、config 和 `ObjectKeyValidator`；每次调用读取 active COS config，不缓存密钥。gateway 文件不得 import `internal/infra/ai`，也不得硬编码 AI/Canvas prefix。AI adapter 继续调用既有 `TrustedAIChatObjectKey`；Canvas adapter 使用只接受当前 user namespace 和规范 path 的 validator。

- [ ] **Step 3: 抽取并复用现有 COS SDK 流式操作**

把 `object_inspector.go`、`object_stream.go` 中重复的 active config、signed client、HEAD/GET metadata 和错误映射抽入 transport-neutral private core；既有 AI types 通过薄 adapter 做 DTO 投影。core 必须复用现有 `object_writer.go`/`object_reader.go` 已提供的 `bucketURL`、`signedHTTPClient`、`HTTPStatus`，不得再实现一套 endpoint、签名 transport 或 COS error parser；reader/writer 的现有公开行为保持不变。Open 返回 SDK response body 给调用者并由调用者 close；gateway 不整体读入。SignGetURL 使用同一 SDK client presign GET，返回 `now+ttl`；日志与 error 不包含 secret、session token 或完整签名 query。

- [ ] **Step 4: 证明 STS policy 仍是单 key 写权限**

扩充 signer test 断言 action 精确 `[cos:PutObject,cos:PostObject]`、resource 精确一个 `qcs::cos.../<object-key>`，没有 wildcard、Get/Delete/ListBucket。

- [ ] **Step 5: 运行 COS tests 和源码门禁**

```powershell
go test ./internal/infra/storage/cos -run 'PrivateObject|ObjectInspector|ObjectStream|Signer' -count=1
rg -n 'io\.ReadAll' internal/infra/storage/cos/private_object_gateway.go
```

Expected: tests PASS；rg 无输出；现有 AI object tests 无行为变化。

- [ ] **Step 6: 提交 gateway**

```bash
git add internal/infra/storage/cos
git commit -m "feat(cos): 增加私有素材对象网关"
```

### Task 5: 实现 upload intent、流式图片确认和 platform-aware assets

**Files:**
- Create: `internal/module/ai/asset/{upload_intent,image_verifier,upload_service}.go`
- Modify: `internal/module/ai/asset/{model,dto,repository,service}.go`
- Test: `internal/module/ai/asset/*_test.go`

- [ ] **Step 1: 写上传、并发确认、读取和删除失败测试**

覆盖：只接受 jpg/jpeg/png/webp 且 MIME/extension 一致；客户端 key 被忽略/拒绝；object key 含正确 user/year/month/random ext；STS 输入只有 key；HEAD size/MIME/hash metadata 不同失败；伪图片和解码失败；实际 hash/尺寸；20 MiB+1 拒绝；同 intent 两个并发确认只有一个 asset row；被 project 引用删除 409；签名 URL 只给 owner；text asset 不需要 COS。

- [ ] **Step 2: 收紧 Asset model/DTO**

```go
type Asset struct {
    ID int64; Platform string; UserID uint64
    Slug, Type, Category, Title, Description, Content, TagsJSON string
    StorageProvider string; ObjectKey *string; SHA256 []byte
    MIMEType string; SizeBytes uint64; Width uint; Height uint
    Status, IsDel int; CreatedAt, UpdatedAt time.Time
}
type Owner struct { Platform string; UserID uint64 }
```

Canvas create 类型仅 `text|image`。Item 不返回 object key、历史 URL 或 cover URL；图片返回 `read_url/read_url_expires_at`，文本 detail 返回 content。现有 video 常量只供非 Canvas 历史数据解析，Canvas service 明确拒绝。

- [ ] **Step 3: 定义 create intent 输入和响应**

```go
type CreateUploadIntentInput struct {
    Owner Owner
    OriginalFilename string
    DeclaredMIMEType string
    DeclaredSizeBytes uint64
    DeclaredSHA256 string
}
type UploadCredentials struct {
    TmpSecretID string `json:"tmp_secret_id"`
    TmpSecretKey string `json:"tmp_secret_key"`
    SessionToken string `json:"session_token"`
    StartTime int64 `json:"start_time"`
    ExpiredTime int64 `json:"expired_time"`
}
type UploadIntentResponse struct {
    ID int64 `json:"id"`; ObjectKey string `json:"object_key"`
    Bucket string `json:"bucket"`; Region string `json:"region"`; Endpoint string `json:"endpoint"`
    ExpiresAt string `json:"expires_at"`; Credentials UploadCredentials `json:"credentials"`
}
```

SHA 输入必须是 64 lowercase hex 并 decode 为 32 bytes。pending intent 上限固定为每 user 20 条；达到返回 429。opaque ID 使用 `crypto/rand` 16 bytes hex，不用时间戳或文件名。

- [ ] **Step 4: 实现一次流式图片校验**

`VerifyImage(stream,maxBytes)` 使用 `io.LimitReader(max+1)` + `bufio.Reader.Peek(512)` + `http.DetectContentType` + `sha256.New` + counting `io.TeeReader` + `image.DecodeConfig`；blank-import `image/jpeg`、`image/png`、`golang.org/x/image/webp`。Decode 后 `io.Copy(io.Discard, tee)` 读完并计算完整 hash，计数大于 20 MiB 返回 size error。

返回：

```go
type VerifiedImage struct {
    MIMEType string
    Extension string
    SizeBytes uint64
    SHA256 [32]byte
    Width int
    Height int
}
```

- [ ] **Step 5: 实现 lease + verify + transaction consume**

确认和 cleanup 共用 `redislock` key `infinite-canvas:asset-upload-intent:{id}`，lease TTL 2 分钟。确认顺序：acquire lease -> load owner intent -> check pending/not expired -> HEAD -> compare key/size/MIME/SHA metadata -> Open/VerifyImage -> compare actual -> DB transaction lock intent -> recheck -> insert asset with unique `(platform,object_key)` -> update intent consumed/consumed_at -> release lease。

若 intent 已 consumed，按 object key 查原 asset 并幂等返回同 ID；不得创建第二行。网络调用不持有 MySQL row lock。

- [ ] **Step 6: 实现素材 CRUD、签名和引用冲突**

Repository 所有方法接受 Owner。图片 list/detail/content 调 `SignGetURL(5m)`；文本不签名。更新只允许 title/category/description/tags/status，不允许改 type/object/hash/dimensions。Delete transaction 锁 asset，查询 `canvas_project_assets`；有引用返回 `ErrReferenced`，否则只 soft delete，不调用 COS Delete。

- [ ] **Step 7: 运行 asset tests 并提交**

```powershell
go test ./internal/module/ai/asset -run 'UploadIntent|VerifyImage|Asset|Referenced|SignedURL' -count=1
```

Expected: PASS；race fixture 只有一个 asset ID。

```bash
git add internal/module/ai/asset
git commit -m "feat(canvas): 接入私有图片素材"
```

### Task 6: 交付素材 routes 与 cleanup durable task contract

**Files:**
- Create: `internal/module/ai/asset/transport/infinitecanvas/**`
- Create: `internal/module/ai/asset/cleanup_job.go`
- Main-thread integration only: `internal/platform/infinitecanvas/{graph,build}.go`、`internal/jobs/noop.go`、`internal/module/crontask/registry.go`、`internal/runtime/worker.go`、`internal/server/router.go`
- Main-thread generation only: `internal/infinitecanvascontract/**`、`contracts/infinite-canvas/v1/**`
- Test in executor lane: asset transport/cleanup package tests

- [ ] **Step 1: 写素材 HTTP 和 task registration contract 失败测试**

在 asset package 内验证七条素材 routes、读写 permission、strict body、owner provenance，以及交付给主线程的 cleanup task type/policy/cron name descriptor。Cleanup 对 object not found 幂等，delete 失败留 pending 供重试，成功后条件更新 expired。真实 `crontask.NewDefaultRegistry()` 映射测试由主线程在 Wave 3I 添加并运行，执行器不创建或修改 shared registry test。

- [ ] **Step 2: 注册正式素材 routes**

```text
GET    /assets                         infinite_canvas_asset_read
POST   /assets                         infinite_canvas_asset_write
GET    /assets/:id                     infinite_canvas_asset_read
PATCH  /assets/:id                     infinite_canvas_asset_write
DELETE /assets/:id                     infinite_canvas_asset_write
GET    /assets/:id/content             infinite_canvas_asset_read
POST   /asset-upload-intents           infinite_canvas_asset_write
```

规格实际为七条素材相关 routes。POST `/assets` 是 discriminated request：`type=text` 必须带 content 且无 intent；`type=image` 必须带 `upload_intent_id` 且无 content/object_key。客户端提交 object_key/read_url/storage_provider 由 strict decoder 拒绝。

- [ ] **Step 3: 定义 cleanup task 和供主线程注册的 contract**

```go
const TypeAssetUploadCleanupV1 = "infinite-canvas:asset-upload-cleanup:v1"
type AssetUploadCleanupPayload struct { Limit int `json:"limit,omitempty"` }
```

registry policy：queue low、timeout 10 分钟、max retry 5、unique TTL 30 分钟。handler 每批最多 100 个 `pending AND expires_at<=now` intent；每项 acquire 同一 lease、重新读取、Delete（404 视成功）、条件 mark expired。循环最多 10 批，避免单任务无界。

asset package 导出 task constructor 和 handler 所需窄接口。`crontask.NewDefaultRegistry()` 最终必须增加 name `infinite_canvas_asset_upload_cleanup`、BuildTask payload limit=100；cron row 仍保持 disabled。执行器不修改 registry，主线程在 Wave 3I 按此固定 contract 注册。

- [ ] **Step 4: 向主线程交付 graph/API/Worker integration input**

执行器返回 `Workspace.Assets *asset.Service`、service constructor、七条 route definitions、cleanup handler constructor 和 task policy。主线程在 Wave 3I 使用 `uploadtoken.NewObjectConfigProvider`、private gateway、credential signer、Redis lease 完成 API/Worker composition，并注入 `jobs.Dependencies.AssetUploadCleanup`；Worker 不依赖 HTTP graph 实例。

- [ ] **Step 5: 运行 capability 定向门禁，Contract 延后到组合 runtime**

```powershell
go test ./internal/module/ai/asset/... ./internal/infra/storage/cos -run 'Asset|Upload|Cleanup|PrivateObject|InfiniteCanvas' -count=1
git diff --check
```

Expected: 全部退出 0；执行器 diff 不含 platform/server/jobs/crontask/runtime/Contract。主线程在 Plan 05 Task 5 的组合 runtime 提交后一次性生成包含项目、素材和提示词的 Canvas Bundle。

- [ ] **Step 6: 主线程审查并提交 HTTP/task capability slice**

```bash
# main-thread checkpoint after review; shared integration is committed separately in Wave 3I
git add internal/module/ai/asset/transport internal/module/ai/asset/cleanup_job.go
git commit -m "feat(canvas): 交付项目素材与上传清理"
```

### Task 7: 执行资源隔离和泄露静态门禁

本 Task 只由主线程在 Wave 3I 的组合 runtime 提交后执行；architecture test 是共享门禁文件，不分配给 Project 或 Asset/COS executor。

**Files:**
- Create: `internal/architecture/infinite_canvas_resource_isolation_test.go`
- Verify: all project/asset/COS files

- [ ] **Step 1: 写 architecture negative tests**

扫描 Canvas project/asset transport 和 service，禁止 `platform` 从 request DTO 进入 Owner；禁止 response JSON tag `object_key`（upload intent response 除外）、`secret_key` 长期密钥、`storage_provider`、`cover_url`、`url`；禁止保存 `blob:`、`data:` 和签名 query。

- [ ] **Step 2: 运行完整资源测试**

```powershell
go test ./internal/module/canvasproject/... ./internal/module/ai/asset/... ./internal/infra/storage/cos ./internal/platform/infinitecanvas ./internal/server ./internal/jobs ./internal/module/crontask ./internal/runtime ./internal/infinitecanvascontract -count=1
go test ./internal/architecture -run 'TestInfiniteCanvasResourceIsolation|TestInfiniteCanvasPlatformSchema' -count=1
rg -n 'audio|video|provider|api_key|base_url' internal/module/canvasproject internal/module/ai/asset/transport/infinitecanvas
git diff --check
```

Expected: tests PASS；rg 只允许 negative test/明确拒绝常量命中，不能出现 route/DTO/业务写入。

- [ ] **Step 3: 提交隔离门禁**

```bash
git add internal/architecture/infinite_canvas_resource_isolation_test.go
git commit -m "test(canvas): 锁定项目素材隔离边界"
```

## 完成标准

- 项目幂等、revision、copy、rename/save/delete 和 owner 404 语义完整；列表不加载 document。
- CanvasDocumentV1 只接受四类节点和固定字段，5 MiB/数量/数值/引用限制全部执行。
- 保存项目在同一事务验证素材归属并同步引用；被引用素材不能删除。
- STS 只写服务端生成 key；确认经过 HEAD + 限流流式读取 + MIME + 尺寸 + 完整 SHA-256。
- 数据库和项目文档不保存签名 URL、blob URL、data URL 或客户端 storage key。
- cleanup handler 已注册但 cron 仍 disabled，等待 Plan 07 激活。
