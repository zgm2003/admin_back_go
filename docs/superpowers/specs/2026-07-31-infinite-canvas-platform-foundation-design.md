# 无限画布平台首期基础接入设计

**日期：** 2026-07-31

**状态：** 已确认设计，等待书面复核

**适用仓库：** `admin_back_go`、`admin_front_ts`、`canvas_front_next`

## 1. 背景

`infinite-canvas/web` 是一个以浏览器本地存储、用户自配渠道、本地 Agent、
WebDAV 和多种生成能力为中心的前端项目。目标不是把它原样部署，而是提取其
无限画布交互，接入现有 Go 后端和管理系统，形成面向消费者的商业平台。

新平台的正式身份为：

```text
platform: infinite_canvas
name:     无限画布
API:      /api/infinite-canvas/v1
frontend: canvas_front_next
```

它是现有 Go 模块化单体中的原生第二产品平台，不是独立 BFF，也不是已经退役
的 `canvas` 平台复活。`auth_platforms` 仍负责安全策略，但数据库记录本身不能
激活平台；只有经过审核的 trusted transport、编译期注册、正式契约和权限数据
同时存在时，平台才可运行。

## 2. 已批准决策

1. Admin 与无限画布共用 `users` 身份，不共用平台成员资格、角色、权限、
   session、refresh Cookie 或登录日志范围。
2. 新增平台级角色绑定，角色本身也归属一个平台。
3. 无限画布没有独立注册页或注册接口。登录页提供“邮箱验证码”和“账号密码”
   两种方式；新邮箱验证码登录即完成首次创建。
4. 画布以服务端版本化 JSON 文档为正式数据源，浏览器本地数据只用于恢复草稿。
5. 私有 COS 保存用户图片，浏览器通过限定 object key 的 STS 直传，数据库只
   保存稳定 object key。
6. 提示词及提示词来源由 Admin 管理，Worker 通过现有定时任务系统拉取；
   Canvas 只读消费已启用提示词。
7. 本期不接入文本、图片、视频或音频生成，不接入计费。AI 能力等待并行任务
   完成后按智能体场景接入。

## 3. 目标

- 建立独立、可信、可测试的 `infinite_canvas` product platform adapter。
- 完成登录、刷新、退出、登录即注册、找回密码和平台级 RBAC 闭环。
- 完成画布项目 CRUD、自动保存、revision 冲突和跨设备同步。
- 完成用户素材库、私有 COS 直传和短期签名读取。
- 完成 Admin 提示词与来源管理、定时同步和 Canvas 只读提示词库。
- 从 `infinite-canvas/web` 提取消费者需要的画布体验到 `canvas_front_next`。
- 发布独立的 Infinite Canvas Contract Bundle，并更新 Admin Bundle 中新增的
  管理接口。

## 4. 非目标

本期明确不实现：

- 文本生成、对话生成、图片生成和图片按质量计费；
- Seedance 视频生成和按秒计费；
- 音频节点和音频生成；
- WebDAV、浏览器本地正式同步、本地 Agent、MCP、Codex 插件；
- 用户自配渠道、API Key、Base URL、供应商或底层模型；
- 智能体外链、文档站跳转和旧项目的 GitHub/版本发布入口；
- 画布历史版本、实时多人协作和自动冲突合并；
- 提示词远程图片镜像到 COS；本期保留经过校验的 HTTPS 图片地址；
- 恢复任何 `app` 或 `canvas` 兼容路由、别名、数据或运行时分支。

## 5. 总体架构

### 5.1 平台适配器

新增 `infinite_canvas` 编译期平台注册项和独立平台组合入口。推荐边界为：

```text
internal/platform/infinitecanvas
  graph/build       组装该平台允许使用的 capability
  route policy      固定路由认证、权限和审计策略

internal/module/*/transport/infinitecanvas
  auth              登录、刷新、退出、当前用户
  canvasproject     画布项目
  asset             素材和上传意图
  prompt            只读提示词
```

transport 在调用 service 前固定写入 `infinite_canvas` provenance。任何
`platform` header、query 或 body 字段都不能改变请求平台。

现有 capability 继续保持 transport-neutral。平台表现和策略放在 adapter 或
workflow 中，不在共享模块内增加散落的 `if platform == ...` 分支。

### 5.2 路由认证

当前 `router.go` 全局固定使用 Admin 鉴权中间件，需要改成按 trusted route
group 组装：

```text
/api/admin/v1             -> Admin authenticator(PlatformAdmin)
/api/infinite-canvas/v1   -> Infinite Canvas authenticator(PlatformInfiniteCanvas)
```

两组中间件使用相同的认证 capability，但平台常量、Cookie、允许来源和 Contract
Bundle 均独立。未注册平台即使存在启用的 `auth_platforms` 行也没有路由。

### 5.3 Contract Bundle

新增 `contracts/infinite-canvas/v1`，至少包含：

- OpenAPI；
- route policy 与 permission catalog；
- bundle manifest 和 SHA-256；
- `canvas_front_next` 使用的生成 TypeScript 类型和 API client 输入。

Admin 的提示词、来源、角色和平台用户管理接口继续发布到 Admin Contract
Bundle。两个前端都不能手写备用 DTO、猜测字段或保留旧接口兼容层。

## 6. 认证与平台成员资格

### 6.1 登录方式

Infinite Canvas 初始 `auth_platforms` 策略为：

```text
login_types:   ["email", "password"]
allow_register: enabled
bind_platform: enabled
status:         enabled
```

页面只有登录入口，没有注册入口：

- **邮箱验证码：** 已有邮箱直接登录；不存在的邮箱在验证码成功消费后创建共享
  用户、用户资料和 Canvas 平台角色绑定。
- **账号密码：** 账号沿用现有 Admin 语义，即邮箱或手机号。只有既有用户和正确
  密码才能登录，未知账号不能通过密码方式自动创建。
- **首次进入 Canvas：** 既有共享用户在验证码或密码认证成功后，如果尚无
  Canvas 角色绑定且平台允许注册，则绑定 Canvas 默认角色。
- **找回密码：** 沿用现有验证码校验和密码重置 capability，使验证码创建的
  用户之后可以设置密码。

密码登录和验证码发送均执行 `auth_platforms` 对应的图形验证码与频率策略；
Canvas transport 不因复用 Admin capability 而绕过消费者平台的验证码检查。

若平台禁止注册，已有身份但没有 Canvas 绑定的用户也不能自动入驻。无论哪种
方式，都不能从 `users` 的历史角色或 Admin 绑定推导 Canvas 角色。

### 6.2 Session 与 Cookie

- access token 只保存在浏览器内存，subject 必须包含 `infinite_canvas`。
- 生产 refresh Cookie 使用独立名称，例如
  `__Secure-infinite_canvas_refresh`。
- Cookie 为 `HttpOnly`、`Secure`、受限 `Path=/api/infinite-canvas/v1/auth`，
  且固定 `SameSite=Strict`。
- 本地 HTTP 回环开发使用独立的 `infinite_canvas_refresh_dev` Cookie 名，只能
  回退读取 Canvas 自己的生产 Cookie，不能读取 Admin Cookie。
- refresh 只旋转 Canvas session；logout 只撤销 Canvas session。
- 登录、刷新和退出继续执行严格 Origin/CORS 校验。
- 登录日志固定记录 `platform=infinite_canvas`。

### 6.3 平台配置初始化

数据库演进在 transport 已进入同一版本后，以确定值创建或验证
`auth_platforms.code=infinite_canvas`。迁移不覆盖运维人员之后修改的 TTL、
设备绑定和最大 session 数等策略。邮件渠道未就绪时，登录配置必须明确暴露
邮箱验证码不可用，不能伪造发送成功。

## 7. RBAC 设计

### 7.1 角色归属平台

`roles` 新增非空 `platform`：

```text
roles
- id
- platform
- name
- is_default
- is_del
- created_at / updated_at
```

约束：

- `UNIQUE(platform, name)` 保证角色名在同一平台内唯一，并保留现有软删除恢复
  语义；
- `UNIQUE(platform, id)` 为平台匹配的组合外键提供数据库约束；
- 用仅在 `is_default=1 AND is_del=2` 时返回 platform、其他情况返回 NULL 的
  generated column 建唯一索引，保证每个平台只有一个有效默认角色；
- 一个角色只能绑定同平台的 permissions；
- 删除角色时必须检查所有平台角色绑定；
- 修改角色只失效实际使用该角色的 `user + platform` principal。

现有角色全部明确回填为 `admin`。默认角色唯一性使用数据库可证明的约束，
不能只依赖前端选择或无锁的“先清空再设置”。

`role_permissions` 同步新增 `platform`，唯一键改为
`(platform, role_id, permission_id)`，并分别通过 `(platform, role_id)` 和
`(platform, permission_id)` 组合外键引用同平台角色与权限。现有授权全部回填为
`admin`，repository 的查询、写入和软删除都必须携带 platform。`permissions`
增加 `UNIQUE(platform, id)` 作为对应组合外键目标。

### 7.2 用户平台角色

新增：

```text
user_platform_roles
- user_id
- platform
- role_id
- created_at / updated_at
```

约束：

- 主唯一键为 `user_id + platform`；
- `(platform, role_id)` 通过组合外键引用 `roles(platform, id)`；
- principal snapshot、版本查询、角色用户统计、通知角色受众和缓存失效均从该表
  读取，不再从 `users.role_id` 推断；
- 用户全局禁用仍由 `users.status/is_del` 控制；平台成员资格由该表是否存在控制。

数据库演进先允许 `users.role_id` 为空并回填 Admin 映射，再切换全部运行时读写。
旧列在证明无读写者前仅作为迁移期字段，不是授权来源；最终物理删除遵守项目
既有 destructive migration 审批门禁。

### 7.3 Admin 用户与角色管理

Admin 用户管理展示和编辑平台角色绑定，而不是一个全局角色。Canvas-only 用户
可以在用户列表中被管理，但没有 Admin 角色绑定就不能登录 Admin。

角色管理必须显式选择平台，权限树只显示该平台节点。Canvas 默认消费者角色
获得首期工作区、项目、素材和提示词权限，不获得任何 Admin 权限。

初始化创建 `无限画布用户` 作为 Canvas 默认角色，并只授予下列 Canvas 权限。
新增的 Admin 提示词管理权限进入 Admin 权限树，但不自动授予现有普通 Admin
角色；管理员通过现有角色管理显式授权，避免新发布静默扩大后台权限。

建议的 Canvas permission code：

```text
infinite_canvas_workspace
infinite_canvas_project_read
infinite_canvas_project_write
infinite_canvas_asset_read
infinite_canvas_asset_write
infinite_canvas_prompt_read
```

## 8. 画布项目

### 8.1 数据模型

新增：

```text
canvas_projects
- id
- platform
- user_id
- request_id
- request_fingerprint
- title
- schema_version
- document_json
- revision
- is_del
- created_at / updated_at
```

关键约束和索引：

- `request_id` 在 `platform + user_id` 范围唯一；
- 同一幂等键和同一 fingerprint 重放返回原项目；同一键不同 fingerprint 返回
  `409`；
- 所有读写包含 `platform + user_id + is_del`；
- 列表只读取摘要，不加载 `document_json`；
- revision 从 1 开始，每次成功保存加 1。

首期文档只允许以下节点：

```text
text
image
config
group
```

`CanvasDocumentV1` 的 canonical JSON 使用 snake_case，顶层只包含：

```text
nodes
connections
viewport              x / y / k
background_mode       dots | lines | blank
show_image_info       boolean
```

节点公共字段只包含 `id/type/title/position/width/height/group_id/data`。`data` 按
类型严格区分：

```text
text    content / font_size
image   asset_id / natural_width / natural_height / free_resize
config  prompt / referenced_asset_ids
group   空对象
```

connection 只包含 `id/from_node_id/to_node_id`，两端必须引用当前文档节点。
`group_id` 只能引用 group 节点，不能形成嵌套循环。config 中的引用与 image 的
`asset_id` 一并进入素材归属校验和 `canvas_project_assets` 同步。

选择状态、撤销栈、loading/error、生成历史和临时 object URL 都是客户端瞬时
状态，不进入文档。provider、model、channel、quality、API Key、Base URL 和任意
catch-all metadata 在 v1 中均为未知字段。AI 契约批准后通过新的 schema version
和显式迁移增加场景字段，不能把未知 JSON 偷渡进 v1。

音频和视频节点、插件节点、本地 Agent 会话、渠道、API Key、Base URL 和供应商
模型字段均不属于 schema v1。图片节点保存 `asset_id`，不保存 data URL、blob
URL、COS object key 或签名 URL。

初始保护限额：

- 序列化 JSON 不超过 5 MiB；
- 节点不超过 2,000 个；
- 连线不超过 4,000 条；
- node/connection id 必须匹配 `[A-Za-z0-9_-]{1,64}` 且在文档内唯一；
- 标题不超过 120 个 Unicode code point；
- text content 和 config prompt 分别不超过 100,000 个 Unicode code point，
  单个 config 最多引用 32 个不重复素材；
- 坐标必须为绝对值不超过 10,000,000 的有限数，宽高范围为 `32..8192`，
  viewport scale 范围为 `0.05..5`；
- 未知节点类型、未知顶层字段和非法数值直接返回 `400`。

### 8.2 创建、保存和冲突

- 创建由客户端生成 `request_id`，服务端保存 canonical payload fingerprint。
- 重命名请求也携带 `expected_revision` 并原子递增 revision，因此标题和文档
  不会在不同设备间静默互相覆盖。
- 保存使用 `PUT /projects/:id/document`，提交 `expected_revision`、
  `schema_version` 和完整文档。
- repository 使用带 revision 条件的原子更新；影响行数为零时读取当前 revision
  并返回 `409`，不静默覆盖。
- 客户端不盲目重试结果不明的保存。它先重新读取项目；若云端文档与本地
  canonical document 相同，则接受云端 revision，否则进入冲突处理。
- 冲突时暂停自动保存，只提供“载入云端版本”和“将本地内容另存为新画布”。
- 删除采用软删除，不级联删除用户独立素材。
- 首期只保存最新版本，不建立历史快照表。

同一客户端的重命名和文档保存进入同一个 revision mutation queue，不能由两个
独立请求队列互相制造冲突。

### 8.3 素材引用

新增关系表：

```text
canvas_project_assets
- project_id
- asset_id
- created_at
```

保存画布时，服务端解析全部 `asset_id`，验证素材属于同一
`platform + user_id` 且有效，并在同一数据库事务同步引用关系。任何越权或失效
素材都会使整次保存失败。删除仍被有效画布引用的素材返回 `409`。

## 9. COS 与用户素材库

### 9.1 素材模型

复用并收紧 `internal/module/ai/asset`，使其成为可被平台 workflow 使用的素材
capability。`ai_assets` 增加明确的 `platform` 和稳定对象元数据：

```text
ai_assets
- id
- platform
- user_id
- type                  text | image
- title / category / description / tags_json
- content               文本素材正文；图片为空
- storage_provider      图片固定为 cos
- object_key
- sha256
- mime_type
- size_bytes
- width / height
- status / is_del
- created_at / updated_at
```

浏览器本地 `storageKey`、data URL、blob URL 和临时签名 URL 不进入数据库。
未来视频素材通过正式设计扩展，不在本期提前开放 `video` 写入。

### 9.2 上传流程

新增短期资源：

```text
asset_upload_intents
- id
- platform
- user_id
- object_key
- original_filename
- declared_mime_type
- declared_size_bytes
- declared_sha256
- status                pending | consumed | expired
- expires_at / consumed_at
- created_at / updated_at
```

object key 唯一。确认素材时锁定 intent 行并做条件状态转换，保证多 API 节点下
也只能消费一次；STS 临时密钥本身不写数据库。

```text
1. 浏览器提交文件名、MIME、字节数和 SHA-256，创建 upload intent
2. 服务端校验并生成唯一 object key
3. 服务端签发只允许该 key、方法和短 TTL 的 COS STS
4. 浏览器直接上传到私有 bucket
5. 浏览器使用 intent 创建素材
6. 服务端 HEAD 对象，核对 key、大小、MIME 和哈希元数据
7. 服务端在 20 MiB 上限内流式读取对象，嗅探真实图片类型、解析尺寸并计算
   SHA-256，拒绝元数据与对象内容不一致
8. 核对成功后写入 ai_assets，并消费 intent
```

object key 固定隔离为：

```text
infinite-canvas/users/{userId}/assets/{yyyy}/{mm}/{opaqueId}.{ext}
```

客户端不能提交或替换服务端选定的 key。upload intent 以
`platform + user_id + intent_id` 隔离并短期有效。Worker 定期处理过期 pending
intent：删除对应未引用 COS 对象后标记 expired；对象不存在也以幂等成功收敛。
该清理器注册为 `infinite_canvas_asset_upload_cleanup`，初始每小时第 15 分钟执行，
之后可由现有定时任务管理页调整。
首期图片允许 JPEG、PNG、WebP，单文件最大 20 MiB；服务端还校验可解析的图片
尺寸，拒绝伪造 MIME 和非图片内容。

### 9.3 私有读取和删除

- bucket 保持私有，不保存永久公开 URL；
- 素材列表和详情返回短期签名 URL 及明确过期时间；
- 前端在过期前或收到鉴权失败后按 asset id 刷新一次；
- 签名只能读取该素材 object key；
- 软删除素材前检查 `canvas_project_assets`；被引用时返回 `409`；
- 删除数据库记录不立即物理删除对象，由后续受审清理任务处理孤儿对象。

## 10. 提示词与来源同步

### 10.1 提示词来源

新增：

```text
ai_prompt_sources
- id
- platform
- code
- name
- feed_url
- homepage_url
- status
- last_attempt_at
- last_success_at
- last_error_summary
- etag / last_modified
- created_at / updated_at / is_del
```

六个原前端内置 GitHub JSON 来源作为初始数据库数据导入，不再编译进 Canvas
前端。它们只是普通可管理来源，Admin 可以修改、启停和删除。

```text
banana-prompt-quicker
davidwu-gpt-image2-prompts
awesome-gpt-image
awesome-gpt4o-image-prompts
youmind-gpt-image-2
youmind-nano-banana-pro
```

初始 feed URL 和 homepage 必须逐字迁移自原项目的
`prompt-source-presets.ts`，迁移测试验证六个 code 和 URL，运行时不保留前端
fallback。设计复核时六个 feed 共 1,201 条记录，均具有非空且来源内唯一的 id、
title 和 prompt，符合严格解析的身份前提。

删除来源时，在同一事务软删除来源及其全部来源提示词；手工提示词不受影响。
正在执行同步的来源先等待或拒绝删除，不能让同步提交重新激活已删除来源。

`feed_url` 只允许 HTTPS。拉取器限制 DNS/IP、重定向、超时、响应大小和条目数，
拒绝 loopback、private、link-local 和其他非公网目标，并在每次重定向后重新
校验目标。提示词条目中的 cover/reference 地址也只接受可解析的 HTTPS URL。

### 10.2 提示词归属

扩展 `ai_prompts`：

```text
- platform
- origin_type           manual | source
- source_id             手工提示词为空
- external_id           来源内稳定 ID
- slug / category / title
- description / prompt / preview
- cover_url / reference_urls_json / tags_json
- status / is_del
- created_at / updated_at
```

约束：

- 手工 slug 在平台内唯一；
- 来源提示词以 `source_id + external_id` 唯一；
- 来源同步维护内容字段，Admin 维护启停状态；
- 来源提示词不能通过普通删除制造“下次同步又出现”的歧义，只允许禁用；
- 手工提示词不受来源同步影响。

Canvas 只返回启用、未删除且来源仍有效的提示词。内容以纯文本呈现，不执行
来源中的 HTML。

### 10.3 同步语义

在现有 `cron_task` registry 注册固定任务：

```text
infinite_canvas_prompt_sync
```

定时任务只负责为所有启用来源创建 durable queue work；Admin 手动同步则创建
同一种 prompt sync job，并可限定 `source_id`。初始 schedule 为每 6 小时一次，
之后由现有定时任务管理页调整；不建立第二套浏览器定时器。

每个来源的执行规则：

1. 获取分布式 lease，禁止同一来源并发同步；
2. 使用 ETag/Last-Modified 条件请求；
3. 严格解析根数组和每个条目；缺少 ID、标题或 prompt，重复 ID、非法类型或
   超限内容均使本次来源同步失败；
4. 在事务中 upsert 全部条目；
5. 只有完整成功后才软删除本次快照中消失的来源条目；重新出现时恢复；
6. 提交后更新 last success；失败只更新 attempt/error，保留上次成功数据；
7. 日志和错误摘要不得保存远端凭据、完整响应体或用户隐私数据。

## 11. HTTP 契约

### 11.1 Infinite Canvas 公共认证接口

```text
GET    /api/infinite-canvas/v1/auth/login-config
GET    /api/infinite-canvas/v1/auth/captcha
POST   /api/infinite-canvas/v1/auth/verification-codes
POST   /api/infinite-canvas/v1/auth/sessions
PUT    /api/infinite-canvas/v1/auth/session
DELETE /api/infinite-canvas/v1/auth/session
POST   /api/infinite-canvas/v1/auth/password-resets
```

不存在 `/register`。创建 session 的请求根据 login type 接收验证码或密码；
refresh 和 logout 使用独立 HttpOnly Cookie 及严格空 body 规则。

### 11.2 Infinite Canvas 受保护接口

```text
GET    /api/infinite-canvas/v1/me

GET    /api/infinite-canvas/v1/projects
POST   /api/infinite-canvas/v1/projects
GET    /api/infinite-canvas/v1/projects/:id
PATCH  /api/infinite-canvas/v1/projects/:id
PUT    /api/infinite-canvas/v1/projects/:id/document
DELETE /api/infinite-canvas/v1/projects/:id

GET    /api/infinite-canvas/v1/assets
POST   /api/infinite-canvas/v1/assets
GET    /api/infinite-canvas/v1/assets/:id
PATCH  /api/infinite-canvas/v1/assets/:id
DELETE /api/infinite-canvas/v1/assets/:id
GET    /api/infinite-canvas/v1/assets/:id/content
POST   /api/infinite-canvas/v1/asset-upload-intents

GET    /api/infinite-canvas/v1/prompts
GET    /api/infinite-canvas/v1/prompts/:id
```

项目复制使用 `POST /projects` 的 `source_project_id` 与新 `request_id`，不新增
动作式复制 URL。图片素材创建请求消费 upload intent；文本素材直接提交正文。

### 11.3 Admin 管理接口

Admin transport 新增正式资源接口：

```text
GET    /api/admin/v1/ai/prompts
POST   /api/admin/v1/ai/prompts
GET    /api/admin/v1/ai/prompts/:id
PUT    /api/admin/v1/ai/prompts/:id
DELETE /api/admin/v1/ai/prompts/:id
PATCH  /api/admin/v1/ai/prompts/:id/status

GET    /api/admin/v1/ai/prompt-sources
POST   /api/admin/v1/ai/prompt-sources
GET    /api/admin/v1/ai/prompt-sources/:id
PUT    /api/admin/v1/ai/prompt-sources/:id
DELETE /api/admin/v1/ai/prompt-sources/:id
PATCH  /api/admin/v1/ai/prompt-sources/:id/status
POST   /api/admin/v1/ai/prompt-sync-jobs
```

列表使用 GET query，更新使用 PUT/PATCH，删除使用 DELETE。每条路由都有唯一
permission、审计策略和生成契约。同步 job 创建返回 job identity，不阻塞 HTTP
等待远端来源完成。该 Admin workflow 固定管理 `infinite_canvas` 提示词，不接受
客户端提交任意目标 platform。

## 12. `canvas_front_next` 设计

### 12.1 路由和产品外壳

首期路由：

```text
/login
/projects
/canvas/:projectId
/assets
/prompts
```

根路由按登录状态跳转到 `/projects` 或 `/login`，不建设营销 landing page。
没有 `/register`。登录页根据 login config 展示邮箱验证码和账号密码；找回密码
作为登录流程的一部分。

从原项目保留画布交互、项目卡片、素材选择、提示词浏览、主题和必要响应式
布局。首期把原“生成配置”节点呈现为“提示词配置”，只编辑 prompt 和素材引用，
不显示模型或执行生成。删除或不复制：

- 文档、GitHub、版本发布和外部智能体链接；
- `/config`、用户渠道、模型脚本、API Key 和 Base URL；
- WebDAV 和 prompt source 浏览器配置；
- 本地 Agent panel、连接页、历史、日志、插件管理；
- audio/video 节点、设置面板、工作台和入口；
- 直接调用第三方 AI API 的 service。

本期没有可用 AI route 时，不显示伪按钮或永久 loading 占位。后续 AI workflow
只能消费服务端智能体场景：对话归属 `text_generation`，图片归属
`image_generation`，视频在 Seedance API 与计费设计批准后再加入。用户永远不
配置供应商和渠道。

### 12.2 会话恢复

- 应用启动调用 Canvas refresh endpoint 恢复内存 access token；
- 恢复期间显示稳定的应用初始化状态，不闪现受保护页面；
- refresh 失败清理内存状态并回到 `/login`；
- API client 遇到一个 `401` 时只允许一次 single-flight refresh，其余请求等待；
- refresh 失败不循环重试。

### 12.3 自动保存

- 打开编辑器先加载服务端项目并校验 schema version；
- 编辑状态即时更新，约 1 秒无新操作后触发保存；
- 保存单通道串行，飞行期间的新变更合并为下一份最新文档；
- 页面显示稳定的 `saving/saved/offline/conflict/error` 状态，不用弹窗打断正常
  自动保存；
- IndexedDB 以 `platform + user_id + project_id` 保存恢复草稿、标题和 base
  revision，不参与跨设备正式同步；
- 网络恢复后从最新已确认 revision 继续；
- `409` 时停止自动保存并展示云端 revision，用户选择载入云端或本地另存；
- 路由离开时先尝试完成当前保存，失败时保留 IndexedDB 草稿。

### 12.4 素材 URL

前端 state 和画布文档只持有 asset id。统一 asset resolver 缓存短期签名 URL
及过期时间；过期前刷新，读取失败最多重新签名一次。组件卸载时释放 blob URL，
但绝不把 blob URL 写回项目文档。

## 13. 错误与可观测性

统一 HTTP 语义：

```text
400  客户端参数或画布 schema 非法
401  缺少、过期或平台不匹配的登录态
403  已认证但无 RBAC 权限或平台不允许入驻
404  资源不存在，或不属于当前 platform + user
409  revision、幂等键或被引用素材冲突
413  客户端提交的画布文档或上传文件超限
429  验证码、登录、上传意图或手工同步频率超限
5xx  内部依赖错误，响应不泄露实现和凭据
```

所有链路传播 `request_id`，结构化日志包含经过验证的 `platform`、`user_id`、
resource id 和 outcome。日志禁止记录验证码、密码、Bearer token、refresh token、
STS secret、完整签名 URL、完整画布 JSON 和远端提示词响应体。

远端提示词响应超限发生在异步 Worker 中，job 标记为失败并保留最后成功数据；
创建同步 job 的 HTTP 请求已经成功入队时不能事后改写成 `413`。

至少观测：

- 登录成功/失败和平台注册结果；
- permission deny 与 principal cache version；
- 画布保存耗时、文档大小和 revision conflict 数；
- upload intent、HEAD 确认失败和签名失败；
- 每个提示词来源的同步时长、条目数、成功/失败和最后成功时间。

## 14. 安全不变量

1. 客户端不能选择 platform provenance。
2. 同一 user 的 Admin token、角色和 Cookie 不能授权 Canvas，反向亦然。
3. 无平台角色绑定不能创建该平台 session。
4. 任意项目、素材和引用读写都包含 `platform + user_id`。
5. 客户端不能自行指定 COS object key 或为其他用户 key 获取 STS/签名。
6. 画布中的 asset id 必须在保存事务内验证归属。
7. prompt feed 不能访问内网或通过重定向绕过目标校验。
8. 远端 prompt 内容始终按数据和纯文本处理，不能执行 HTML/脚本。
9. 未知 schema version 不能被旧客户端保存回服务端。
10. AI 能力未正式接入前，浏览器中不存在渠道密钥和第三方生成调用。

## 15. 测试与验收

### 15.1 后端

- 平台 registry 从仅 Admin 变为精确的 Admin + Infinite Canvas，退役值仍被拒绝；
- 两个 trusted transport 都忽略客户端 platform header/body；
- 邮箱新用户登录即创建，密码未知账号不创建；
- 已有用户首次 Canvas 登录只增加 Canvas 角色，不继承 Admin 角色；
- 无 Canvas 绑定、禁用用户、禁用平台和无默认角色均失败关闭；
- Admin/Canvas access token、refresh Cookie、session、登录日志和 logout 隔离；
- principal repository 对同一 user 的两个平台加载不同 role/permission/version；
- 角色不能跨平台绑定 permission，默认角色按平台唯一；
- 项目创建幂等、不同 fingerprint 冲突、revision 原子更新和越权 404；
- 非法文档、未知节点、超限文档和跨用户 asset id 被拒绝；
- STS policy 仅允许预定 key，HEAD 不一致不创建素材；
- upload intent 并发确认只能成功一次，过期未消费对象可被幂等清理；
- 被画布引用的素材不能删除；
- prompt 同步严格解析、幂等 upsert、缺失条目下架、失败保留旧数据；
- SSRF、重定向、超时和响应大小防线有确定测试；
- Admin Bundle 和 Infinite Canvas Bundle 均通过 drift、route policy 和 schema gate；
- canonical schema、migration checksum、fingerprint 和恢复验证通过。

### 15.2 前端

- TypeScript strict typecheck、production build、format check；
- auth store 的启动恢复和 single-flight refresh；
- 邮箱验证码、账号密码和找回密码交互；
- 项目 API state、自动保存队列、断网草稿和结果不明的重新读取；
- `409` 的载入云端与另存为新项目；
- asset resolver 的签名过期刷新和失败上限；
- prompt 列表、筛选、详情和插入画布；
- 不存在渠道、WebDAV、Agent、插件、文档、音频和视频入口。

### 15.3 Playwright 验收

在桌面和移动 viewport 完成：

1. 邮箱验证码登录新用户并自动入驻；
2. 已有账号密码登录；
3. 创建、重命名、复制和删除项目；
4. 上传图片、创建素材、插入画布并自动保存；
5. 新浏览器上下文登录后读取同一云端画布；
6. 两个上下文制造 revision 冲突并分别验证载入云端和另存；
7. 浏览提示词并插入文本/配置节点；
8. 尝试读取另一用户项目和素材得到 404；
9. 检查无控制重叠、文本溢出、空白画布或不可点击主流程。

## 16. 实施顺序

1. 数据库 additive migration：平台注册数据、角色平台字段、用户平台角色、画布、
   引用、素材和提示词来源。
2. 平台 registry、trusted route groups、Contract Bundle 骨架和隔离架构测试。
3. 认证 repository/service 的平台成员资格改造，以及 Canvas Auth transport。
4. principal/RBAC、Admin 用户角色管理和跨平台缓存失效。
5. 画布项目、素材、COS upload intent、孤儿上传清理与签名读取。
6. Admin 提示词/来源 transport、Worker sync job 和定时任务注册。
7. Admin 前端管理页面与正式 Admin client 生成物。
8. 提取并改造 `canvas_front_next`，接入 Canvas 生成 client。
9. 集成、Playwright、数据库演进、Contract drift 和完整构建验收。
10. 在无运行时读写者的证明成立后，按 destructive gate 清理 `users.role_id` 等
    迁移期字段。

每一步都必须保持 Admin 可登录、Admin 权限不扩大，并以同一个真实后端验证，
不使用只在前端成立的 mock 作为完成证据。

## 17. 完成标准

- `canvas_front_next` 使用独立 Canvas API、Auth Cookie 和正式契约运行；
- 邮箱验证码可登录即注册，既有账号可密码登录，无独立注册入口；
- 同一共享用户可以拥有完全不同的 Admin/Canvas 角色，且无权限串用；
- 画布、素材和提示词均来自服务端，跨设备可读取，冲突不丢数据；
- 用户图片存于私有 COS，数据库和画布不含临时 URL；
- 提示词由 Admin 和 Worker 维护，Canvas 用户不能配置来源；
- 所有明确排除的旧入口和本地集成都不出现在产品中；
- 两个 Contract Bundle、数据库 schema、后端运行时和两个前端生成物一致；
- 后端、前端、架构、集成和浏览器验收全部通过。
