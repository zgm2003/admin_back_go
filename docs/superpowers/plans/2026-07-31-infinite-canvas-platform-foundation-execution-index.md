# 无限画布平台基础接入执行索引 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 按已确认规格把 `infinite_canvas` 建成 Go 模块化单体中的可信第二平台，交付平台隔离的 Auth/RBAC、服务端画布项目、私有 COS 素材、后台提示词同步和 `canvas_front_next` 产品外壳。

**Architecture:** 共享 `users` 身份，所有成员资格、角色、权限、principal、session、Cookie、路由策略和 Contract Bundle 按平台隔离。后端 capability 保持 transport-neutral，Admin 与 Infinite Canvas 通过独立编译期 graph 和可信 route group 组装；Canvas 浏览器只消费生成契约、服务端项目、素材和提示词，不保存渠道或第三方 AI 密钥。

**Tech Stack:** Go 1.26.5、Gin、GORM、MySQL 8.4、Atlas、Redis/Asynq、腾讯云 COS、React 19、TypeScript 5、Vite 7、Ant Design 6、Zustand、TanStack Query、Vitest、Playwright、OpenAPI 3.1。

---

## Canonical Inputs

- 唯一业务规格：`docs/superpowers/specs/2026-07-31-infinite-canvas-platform-foundation-design.md`。
- 后端根目录：`E:\admin\admin_back_go`。
- Admin 前端根目录：`E:\admin\admin_front_ts`。
- Canvas 前端根目录：`E:\admin\canvas_front_next`。
- 交互参考项目：`E:\admin\infinite-canvas\web`，只能作为交互和视觉来源，不能作为 API、持久化或运行时配置来源。
- `canvas_front_next` 中预先存在的 `D a` 是用户改动；所有计划都禁止恢复、暂存或提交该路径。
- 实施前读取 `AGENTS.md`、`docs/architecture.md`、`internal/module/README.md`、`internal/platform/README.md`；涉及 Admin 前端时额外读取 `E:\admin\admin_front_ts\docs\rule.md`。

## Fixed Product Contract

```text
platform: infinite_canvas
name:     无限画布
API:      /api/infinite-canvas/v1
frontend: canvas_front_next
```

- 不新增 BFF，不恢复 `app` 或旧 `canvas` adapter。
- 本期没有文本、图片、视频、音频生成或计费 route；没有渠道、Provider、API Key、Base URL、Agent 外链、WebDAV、插件或文档入口。
- Canvas 节点集合严格为 `text | image | config | group`，其中 `config` 的产品名为“提示词配置”。
- 邮箱验证码是登录即注册；密码登录只接受既有身份。不存在注册页和 `/register` 接口。
- 数据库中的 `auth_platforms` 行不能激活平台；只有编译期 registry、trusted transport、graph、权限和 Contract Bundle 同时存在才可运行。

## Plan Files And Waves

| Wave | Plan | Ownership | Depends on |
| --- | --- | --- | --- |
| 0 | `2026-07-31-infinite-canvas-platform-foundation-01-schema-platform-rbac.md` | canonical schema、guarded migrations、平台/角色/产品数据和初始种子 | none |
| 1 | `2026-07-31-infinite-canvas-platform-foundation-02-rbac-runtime-admin-management.md` | 通用 route policy、platform RBAC runtime、Admin 角色/用户管理及前端 | 01 |
| 2 | `2026-07-31-infinite-canvas-platform-foundation-03-infinite-canvas-auth-contract.md` | trusted route group、Canvas graph、Auth、`/me`、独立 Contract Bundle | 02 |
| 3 | `2026-07-31-infinite-canvas-platform-foundation-04-canvas-project-assets-cos.md` | 项目文档、revision、素材引用、上传意图、私有 COS、清理任务 | 03 |
| 3 | `2026-07-31-infinite-canvas-platform-foundation-05-prompt-sync-admin-ui.md` | 提示词/来源、SSRF 防护、durable sync、Admin transport/UI、Canvas 只读 transport | 03 |
| 4 | `2026-07-31-infinite-canvas-platform-foundation-06-canvas-frontend.md` | 空仓库脚手架、生成 client、产品外壳、画布、草稿、素材和提示词 UI | 04、05 |
| 5 | `2026-07-31-infinite-canvas-platform-foundation-07-integration-acceptance.md` | 激活迁移、契约发布、全链路门禁、Playwright、运维文档 | 01-06 |

### Wave rules

- Wave 0 必须串行完成；在它合并前，不允许其他计划猜列名、状态或索引。
- Wave 1 和 Wave 2 串行，因为 Plan 03 的 Auth membership 与 route group 依赖 Plan 02 已切换的 principal 真相源。
- Wave 3 可在独立 worktree 并行；Plan 04 独占 `internal/module/ai/asset/**` 和 COS gateway，Plan 05 独占 `internal/module/ai/prompt/**` 和 Admin 提示词页面。两者修改 `internal/platform/infinitecanvas/build.go`、`internal/jobs/noop.go`、`internal/runtime/worker.go` 时必须先后 rebase，不允许手工复制一份临时 graph/registry。
- Wave 4 在两个后端资源契约发布后开始，禁止手写 DTO 或 runtime mock。
- Wave 5 串行执行，只有这一阶段能启用两条新增 cron row，并发布最终两个 Contract Bundle。

## Shared File Ownership

| File/area | Owner | Rule |
| --- | --- | --- |
| `database/schema/admin.hcl`、`database/migrations/202607310101..103_*` | Plan 01 | 只做 expand/backfill/seed；不删除 `users.role_id`，后续不得另行猜 schema |
| `database/migrations/202607310104_*`、`database/migrations/atlas.sum` final append | Plan 07 | 只在 handler 部署验证后激活两条 cron；不得修改 canonical DDL 或旧 migration |
| `internal/server/routepolicy/**` | Plan 02 | 由 `adminroute` 机械迁移并成为两个平台共享核心 |
| `internal/module/permission/**`、`internal/module/role/**`、Admin user role binding | Plan 02 | `user_platform_roles` 是唯一运行时授权来源 |
| `internal/server/router.go`、`internal/platform/infinitecanvas/{graph,build}.go` | Plan 03 then 04/05 | Plan 03 建骨架；04/05 只追加各自 capability，不复制构造逻辑 |
| `internal/infinitecanvascontract/**`、`cmd/infinite-canvas-contract/**` | Plan 03 | Canvas bundle 只含 OpenAPI、permissions、manifest |
| `internal/module/canvasproject/**`、`internal/infra/storage/cos/private_object_gateway*`、`internal/module/ai/asset/**` | Plan 04 | 项目和素材保存必须在服务端归属边界内完成 |
| `internal/module/ai/prompt/**`、Admin prompt transport、prompt jobs | Plan 05 | 远端 JSON 只由 Worker 拉取，Canvas 永不管理来源 |
| `E:\admin\admin_front_ts\src\views\Main\ai\prompts/**`、`promptSources/**` | Plan 05 | 只消费 Admin generated contract |
| `E:\admin\canvas_front_next/**` | Plan 06 | 不触碰预先删除的 `a`；只消费 Infinite Canvas generated contract |
| `contracts/admin/v1/**`、`contracts/infinite-canvas/v1/**` | 各后端 Plan 开发期生成，Plan 07 最终发布 | manifest 必须绑定同一最终 backend commit |

## Cross-Plan Type Contract

后续计划只能使用以下已经固定的名字，改变名字必须先更新主规格和所有依赖计划：

```go
const enum.PlatformInfiniteCanvas = "infinite_canvas"

type PlatformRoleBinding struct {
    UserID   int64
    Platform string
    RoleID   int64
}

type PrincipalSubject struct {
    UserID   int64
    Platform string
}

const canvasproject.SchemaVersionV1 = "canvas_document_v1"
```

```text
cron registry names:
  infinite_canvas_prompt_sync
  infinite_canvas_asset_upload_cleanup

task types:
  infinite-canvas:prompt-sync-dispatch:v1
  infinite-canvas:prompt-source-sync:v1
  infinite-canvas:asset-upload-cleanup:v1
```

HTTP 错误固定使用 `400/401/403/404/409/413/429/5xx` 的规格语义；项目或素材越权一律 `404`，revision/幂等/素材引用冲突一律 `409`，大小超限一律 `413`。

## Database Cutover Gates

1. 停止 API、Worker 和所有会写 `users.role_id`、`roles`、`role_permissions`、`ai_assets`、`ai_prompts` 的旧进程。
2. 对运行中数据库执行 Plan 01 migration preflight；任何孤儿角色、跨平台 permission、重复默认角色或非法历史平台值都必须在 DDL 前失败。
3. 执行 `202607310101` 后，Admin runtime 必须仍可由回填的 `user_platform_roles(platform='admin')` 登录和授权。
4. `users.role_id` 本期仅改为 nullable migration field；任何运行时代码、查询、导出、统计和测试都不得读取或写入它。
5. `202607310103` 只以 disabled 状态创建两条 cron row，避免旧 Worker 看见未知 registry entry。
6. Plan 07 先部署包含两个 task handler 的 API/Worker，再执行 `202607310104_infinite_canvas_runtime_activation.sql` 启用 schedule。
7. 物理删除 `users.role_id` 必须另开 destructive migration 设计和用户批准；不属于本执行索引。

## Execution Protocol

- [ ] 每个 Plan 开始前创建隔离 worktree，并记录基线 `git status --short`；不得使用宽泛 `git add -A`。
- [ ] 每个 Task 严格执行“失败测试 -> 确认失败原因 -> 最小实现 -> 同一测试通过 -> 小提交”。
- [ ] 后端 transport 固定写入平台常量；禁止读取 `platform` header/query/body 决定 provenance。
- [ ] 所有 repository 的资源查询必须显式包含 `platform + user_id + is_del`；禁止先按 ID 查出再在 service 比较归属。
- [ ] 两个前端都先同步 Contract Bundle 再生成类型；禁止 `any`、备用 DTO、字段别名和运行时 mock。
- [ ] 真实 COS、邮件、付费上游和公网 prompt feed 只做用户手工验收；自动测试使用 fake、httptest、MinIO/COS fixture 或本地数据。
- [ ] 每个提交只包含本 Plan 文件；`canvas_front_next/a` 始终保持用户原有 deleted 状态且不进入 index。

## Release Acceptance

- [ ] registry 精确包含 `admin` 和 `infinite_canvas`，退役的 `app/canvas` 仍被拒绝。
- [ ] 同一用户可持有不同 Admin/Canvas 角色；token、refresh Cookie、session、logout、登录日志和 principal 不串平台。
- [ ] 新邮箱验证码登录创建共享用户和 Canvas binding；未知账号密码登录不创建；没有独立注册入口。
- [ ] 项目以服务端 canonical document 为准，自动保存串行，revision 冲突不静默覆盖，IndexedDB 只保存恢复草稿。
- [ ] JPEG/PNG/WebP 通过固定 key 的 STS 上传，确认时 HEAD、限流读取、真实 MIME、尺寸和 SHA-256 全部一致。
- [ ] 提示词来源由 Admin 管理，Cron 只入队，Worker 严格解析；失败保留上次成功数据且无法 SSRF 内网。
- [ ] Canvas UI 中不存在渠道、WebDAV、Agent、插件、文档、外链、audio/video 或第三方生成调用。
- [ ] Admin Bundle 和 Infinite Canvas Bundle 无 drift；两个前端 strict typecheck、production build 和定向测试通过。
- [ ] 桌面与移动 Playwright 覆盖登录、项目、素材、提示词、跨设备读取和 revision 冲突，且无控件重叠或文本溢出。

## Final Verification Budget

各 Plan 自动运行自己列出的包级测试。只有 Plan 07 运行以下最终门禁：

```powershell
go test ./...
go test -race ./internal/module/auth ./internal/module/permission ./internal/module/role ./internal/module/user ./internal/module/canvasproject ./internal/module/ai/asset ./internal/module/ai/prompt ./internal/server ./internal/runtime
pwsh -NoProfile -File scripts/database/atlas.ps1 migrate validate --dir file://database/migrations
pwsh -NoProfile -File scripts/verify-runtime-contracts.ps1
npm -C E:/admin/admin_front_ts run build:check
npm -C E:/admin/canvas_front_next run verify
npm -C E:/admin/canvas_front_next run test:e2e
```

Expected: 所有命令退出 0；Contract 目录无 drift，三个仓库只保留用户预先存在且未被计划触碰的改动。
