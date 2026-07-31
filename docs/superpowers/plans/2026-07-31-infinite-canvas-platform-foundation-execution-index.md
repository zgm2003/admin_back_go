# 无限画布平台基础接入执行索引 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 按已确认规格把 `infinite_canvas` 建成 Go 模块化单体中的可信第二平台，交付平台隔离的 Auth/RBAC、服务端画布项目、私有 COS 素材、后台提示词同步和 `canvas_front_next` 产品外壳。

**Architecture:** 共享 `users` 身份，所有成员资格、角色、权限、principal、session、Cookie、路由策略和 Contract Bundle 按平台隔离。后端 capability 保持 transport-neutral，Admin 与 Infinite Canvas 通过独立编译期 graph 和可信 route group 组装；Canvas 浏览器只消费生成契约、服务端项目、素材和提示词，不保存渠道或第三方 AI 密钥。

**Tech Stack:** Go 1.26.5、Gin、GORM、MySQL 8.4、Atlas、Redis/Asynq、腾讯云 COS、React 19、TypeScript 5、Vite 7、Ant Design 6、Zustand、TanStack Router/Query、Hey API fetch/codegen、Vitest、Playwright、OpenAPI 3.1。

---

## Canonical Inputs

- 长任务调度与提交规范：`E:\admin\LONG_TASK_PARALLEL_EXECUTION.md`。本索引与各子计划冲突时，以该规范和本索引的文件所有权覆盖规则为准。
- 唯一业务规格：`docs/superpowers/specs/2026-07-31-infinite-canvas-platform-foundation-design.md`。
- 后端根目录：`E:\admin\admin_back_go`。
- Admin 前端根目录：`E:\admin\admin_front_ts`。
- Canvas 前端根目录：`E:\admin\canvas_front_next`。
- 交互参考项目：`E:\admin\infinite-canvas\web`，只能作为交互和视觉来源，不能作为 API、持久化或运行时配置来源。
- 已审查 backend capability 基线 `d028e17ffd2a66d08b898f905e44cb93cb262bcf` 已包含 AI native file attachment、双 Contract 更新和上传规则扩展名规范化；实施分支必须以它为祖先，并保留其 `database/schema/admin.hcl`、`database/migrations/atlas.sum`、COS、Admin graph 和 runtime 结果。
- 已审查 Admin frontend 基线 `123657244c7d2569db1bb9798dffea455832033b` 已包含附件交互、Contract 收敛和上传扩展名全选；Plan 02/05 的 Admin 工作必须以它为祖先。
- `canvas_front_next` 基线提交 `25538629587da498dd5e4dcb38d79db54c728100` 已包含 React/Vite 基础框架并正式删除占位文件 `a`；所有后续计划都禁止重新创建或重新跟踪该路径。
- 实施前读取 `AGENTS.md`、`docs/architecture.md`、`internal/module/README.md`、`internal/platform/README.md`；涉及 Admin 前端时额外读取 `E:\admin\admin_front_ts\docs\rule.md`。
- 若实施前任一仓库 HEAD 已前移，主线程先审查对应 baseline..HEAD 的提交和 changed paths；只有纯计划文档或已确认兼容的变更可以保留。任何 schema、migration、Contract、COS、runtime、generated client 或 Canvas foundation 漂移都必须先更新本索引的精确基线和受影响 ownership，不能直接当作共享输入。

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

## Baseline Gate

任何实现执行器启动前，主线程串行完成以下检查：

- [ ] backend HEAD 以 `d028e17ffd2a66d08b898f905e44cb93cb262bcf` 为祖先，Admin HEAD 以 `123657244c7d2569db1bb9798dffea455832033b` 为祖先，Canvas HEAD 精确等于 `25538629587da498dd5e4dcb38d79db54c728100`；任何偏差已按 Canonical Inputs 审查并回写索引。
- [ ] 记录三个仓库的实际 HEAD、各自 baseline..HEAD changed paths 和 `git status --short`；除当前计划文档外没有来源不明的未提交修改。
- [ ] 重新审查 Plan 01 的 canonical HCL/Atlas 输入，以及 Plan 04 与现有 COS object inspector/streamer/reader/writer/signer 的复用边界。
- [ ] `canvas_front_next` 工作树 clean，`a` 不存在且不在 index；基础框架通过 `npm run verify` 和生产依赖 audit。
- [ ] 三个仓库的任务 worktree 都从上述已提交基线创建，禁止把任一工作树的未提交状态当作共享输入。

## Plan Files And Waves

| Wave | Plan | Ownership | Depends on |
| --- | --- | --- | --- |
| F0 | Plan 06 Task 1 | 复核已提交的 Canvas React/Vite 基础框架、主题、providers、测试基线；不产生新提交 | baseline gate；可早于后端业务实现 |
| 0 | `2026-07-31-infinite-canvas-platform-foundation-01-schema-platform-rbac.md` | canonical schema、guarded migrations、平台/角色/产品数据和初始种子 | none |
| 1 | `2026-07-31-infinite-canvas-platform-foundation-02-rbac-runtime-admin-management.md` | 通用 route policy、platform RBAC runtime、Admin 角色/用户管理及前端 | 01 |
| 2 | `2026-07-31-infinite-canvas-platform-foundation-03-infinite-canvas-auth-contract.md` | trusted route group、Canvas graph、Auth、`/me`、独立 Contract Bundle | 02 |
| 3A | Plan 04 Tasks 1-3 | `canvasproject/**` 文档、项目 repository/service/transport | 03 |
| 3B | Plan 04 Tasks 4-6 capability steps | 通用 COS object primitive、`ai/asset/**` 上传/素材/cleanup capability；不写 shared integration | 03、冻结后的 AI/COS 基线 |
| 3C | Plan 05 Tasks 1-4 + Task 5 Steps 1-4 | `ai/prompt/**`、SSRF fetch、同步事务、durable task 与 transports；不写 shared integration | 03 |
| 3I | Plan 05 Task 5 Step 5 -> Plan 04 Task 7 -> Plan 05 Task 5 Steps 6-7 | 主线程消费 Plan 04 Tasks 3/6 inputs，统一组装并提交 runtime，提交资源隔离门禁，再从 clean HEAD 生成双 Bundle | 3A、3B、3C |
| 4 | Plan 05 Tasks 6-7 + Plan 06 Tasks 2-9 | Admin 提示词管理、提示词安全门禁和 Canvas Auth/项目/编辑器/素材/提示词前端 | 3I 生成的同一 backend SHA Contract |
| 5 | Plan 07 development short gates | 创建但不执行 104 activation migration、短门禁、Contract/lock 一致性和运维文档 | 01-06 |
| R | Plan 07 release acceptance | 全量 Go/race、真实 Go router Playwright、长 smoke 和真实外部服务人工验收 | 用户明确授权长验收 |

### Wave rules

- Wave F0 与 Wave 0 位于不同仓库，可以并行；F0 只复核已提交基础框架和依赖，不修改文件、不手写 Contract 或业务 mock。
- Wave 0 的数据库写入 lane 必须串行完成；其他槽位只能做只读 migration/security 审查。在它合并前，不允许其他计划猜列名、状态或索引。
- Wave 1 和 Wave 2 串行，因为 Plan 03 的 Auth membership 与 route group 依赖 Plan 02 已切换的 principal 真相源。
- Wave 1 的 Tasks 1-4 是强依赖 runtime chain：routepolicy rename -> role/permission contract -> principal truth source -> user bindings，必须由一个写 lane 串行提交。其余槽位可并发做只读 SQL/授权/Contract 审查和不写共享生成物的测试；不得把依赖尚未提交接口的 User Task 4 伪装成独立写 lane。Admin 前端必须等待已提交 runtime 对应的 Admin Bundle；主线程提交前端 generated baseline 后，Admin UI lane 与 Task 6 后端负向门禁 Steps 1-2 必须并发，最后再做 clean 汇合。
- Wave 2 在 Plan 02 clean checkpoint 后立即并发启动 auth capability、config/CORS 和 contractbundle core 三条写 lane；`internal/server/**`、`internal/platform/**`、runtime composition、Canvas contract package/CLI/artifacts 仍由主线程集成。三条 lane 吸收后，主线程再串行完成 graph、Canvas transports、router 和最终 Bundle。
- Wave 3A/3B/3C 使用三个独立 worktree 并行，执行器不得修改任何共享 composition、registry、runtime 或 Contract 文件。主线程在 3I 中接管这些文件，不再通过两个执行器先后 rebase 同一文件。
- Wave 4 只有在 3I 的组合 backend runtime 已提交、双 Bundle 已绑定该提交 SHA 后开始；禁止手写 DTO、临时 generated tree 或 runtime mock。
- Wave 5 串行执行短门禁和发布准备。Wave R 不因“继续执行计划”自动运行，必须取得用户对长验收和资源占用的明确授权。

## Parallel Lane Ownership

Wave 2 默认使用以下四槽边界：

| Lane | 可写范围 | 禁止写入 / handoff |
| --- | --- | --- |
| 主线程 / integration owner | `internal/server/**`、`internal/platform/**`、`internal/runtime/**`，以及最终 Canvas graph/router/Contract | 等 2A/2B/2C 返回后接线；不在 lane 活跃时修改其独占文件 |
| Auth executor (2A) | Plan 03 Task 1 的 `internal/module/auth/**`、明确列出的 `internal/module/user/service.go` 及定向测试 | 不写 server/platform/runtime；返回 membership/session integration input |
| Config/CORS executor (2B) | Plan 03 Task 2 的 `internal/config/**`、`internal/middleware/cors.go` 及定向测试 | 不写 `internal/server/**`；返回 route-group config/CORS API |
| Contract core executor (2C) | Plan 03 Task 5 Step 2 的 `internal/contractbundle/**`、`internal/admincontract/{bundle,manifest}.go` 及定向测试 | 不创建 Canvas contract/CLI/artifacts；证明 Admin bytes 无 drift |

2A/2B/2C 从同一 Plan 02 runtime/Bundle checkpoint 创建。主线程吸收三路提交后，按 Plan 03 Tasks 3-4 完成 user transport、graph 和 Auth transport，再执行 Task 5 Steps 1/3-6 与 Task 6；任何后续步骤不得回头让执行器改 server/platform shared files。

Wave 3 默认占满四个并发槽位：

| Lane | 可写范围 | 禁止写入 |
| --- | --- | --- |
| 主线程 / integration owner | `internal/platform/**`、`internal/server/**`、`internal/jobs/**`、`internal/runtime/**`、`internal/module/crontask/**`、两个 Contract package/artifact | 不替执行器并行改 capability 内部文件；只在结果返回后集成 |
| Project executor | `internal/module/canvasproject/**` 及其定向测试 | asset、prompt、shared composition、Contract |
| Asset/COS executor | `internal/module/ai/asset/**`、经主线程确认的 `internal/infra/storage/cos/**` 独占文件及测试 | project、prompt、shared composition、Contract |
| Prompt executor | `internal/module/ai/prompt/**` 及其定向测试 | project、asset/COS、shared composition、Contract、Admin frontend |

Wave 4 默认使用以下边界：

| Lane | 可写范围 | 前置条件 |
| --- | --- | --- |
| 主线程 / contract + integration owner | 两个前端的 contract sync/generated tree、Canvas `src/modules/http/{client,error}.ts`、`src/app/**`、cross-feature canvas/navigation integration、package/lockfile | backend manifest SHA 已冻结；先完成 Plan 06 Task 2 才开放 feature lanes |
| Canvas feature executor A | `src/features/auth/**`、`src/features/projects/**`、`src/modules/http/token-vault.ts`、`src/shared/layout/**` 及定向测试 | Canvas generated client 已同步；不修改 `src/app/**` 或共享 HTTP client |
| Canvas feature executor B | `src/features/canvas/**`、`src/features/drafts/**` 及定向测试 | CanvasDocumentV1 generated types 已同步 |
| Product UI executor | Admin prompt/source 页面；该提交完成后再接管 Canvas `src/features/assets/**`、`src/features/prompts/**` 和 feature-local tests/scripts | 对应 generated client 已同步；不修改 Canvas `src/app/**`、`src/shared/**` 或 `src/features/canvas/**` |

Wave 4 先由主线程串行完成 Contract sync/codegen，再并发启动三个 feature lanes。执行器通过 typed props/callbacks 交付跨 feature 接口；`router`、navigation、HTTP refresh middleware，以及 asset/prompt 到 canvas node 的最终 wiring 只由主线程在结果返回后修改。若确需把某个集成文件转交执行器，必须先结束原 owner 对该文件的工作并在进度中记录显式 ownership handoff。

## Shared File Ownership

| File/area | Owner | Rule |
| --- | --- | --- |
| `database/schema/admin.hcl`、`database/migrations/202607310101..103_*` | Plan 01 | 只做 expand/backfill/seed；不删除 `users.role_id`，后续不得另行猜 schema |
| `database/migrations/202607310104_*`、`database/migrations/atlas.sum` final append | Plan 07 | 只在 handler 部署验证后激活两条 cron；不得修改 canonical DDL 或旧 migration |
| `internal/server/routepolicy/**` | Plan 02 | 由 `adminroute` 机械迁移并成为两个平台共享核心 |
| `internal/module/permission/**`、`internal/module/role/**`、Admin user role binding | Plan 02 | `user_platform_roles` 是唯一运行时授权来源 |
| `internal/server/router.go`、`internal/platform/**/{graph,build}.go` | 主线程 | Plan 03 建骨架；Wave 3I 一次性接入 project/asset/prompt，子执行器不得修改 |
| `internal/jobs/**`、`internal/module/crontask/**`、`internal/runtime/**` | 主线程 | durable task 定义可由 capability executor 创建；registry/worker composition 只在 3I 修改 |
| `internal/infinitecanvascontract/**`、`cmd/infinite-canvas-contract/**` | 主线程 | Plan 03 建脚手架；之后所有生成/check 都在已提交 runtime SHA 上串行执行 |
| `internal/module/canvasproject/**`、`internal/module/ai/asset/**` | Plan 04 executors | 项目和素材保存必须在服务端归属边界内完成 |
| `internal/module/ai/prompt/**`（含 capability-local task constructors/handlers） | Prompt executor then 主线程审查 | 远端 JSON 只由 Worker 拉取；`internal/jobs/**`、crontask registry 和 Worker composition 仍只归主线程 |
| `E:\admin\admin_front_ts\src\views\Main\ai\prompts/**`、`promptSources/**` | Plan 05 | 只消费 Admin generated contract |
| `E:\admin\canvas_front_next/**` | Plan 06 | 不触碰预先删除的 `a`；只消费 Infinite Canvas generated contract |
| `internal/infra/storage/cos/**` shared primitives | Asset/COS executor then 主线程审查 | 必须复用冻结基线中的 inspector/streamer/client 逻辑；禁止新增第二套签名 client/config/error mapping |
| `contracts/admin/v1/**`、`contracts/infinite-canvas/v1/**` | 主线程 | 每次都先提交组合 backend runtime，再由同一 runtime SHA 串行生成；执行器不得生成或修改 |

## Lane Handoff Protocol

每个并发 lane 使用主线程从同一 clean baseline 创建的命名 branch + worktree。执行器只编辑允许路径、运行定向测试并返回：worktree 路径、`git status --short`、完整 changed/untracked file list、测试命令与 exit code；不得 stage 或 commit。

执行器返回后，该 lane 立即进入 frozen 状态，不再接受后续写入。主线程按以下顺序吸收结果：

1. 在 lane worktree 重新读取 `git status --porcelain=v1 --untracked-files=all` 和完整 diff，拒绝越界路径、来源不明文件及生成物。
2. 主线程在该 lane worktree 重新运行当前 ownership slice 的定向门禁；失败时在原 lane 修复并重跑，不能把失败 diff 带入 integration branch。
3. 只有主线程可以按计划中的显式 pathspec stage，并在 lane branch 创建一个或多个 cohesive checkpoint commits；提交前再次检查 staged name list 和 `git diff --cached --check`。
4. 主线程回到 integration worktree，按依赖顺序串行 `git cherry-pick` 已审查 commits。禁止用 directory copy、手工重写或宽泛 `git add -A` 传递 lane 结果。
5. 每次 cherry-pick 后运行受影响 package/feature 的短门禁；3A/3B/3C 全部吸收后再开始 3I，Wave 4 三个 feature lanes 全部吸收后再做 app wiring。
6. 只有 lane branch 已提交、worktree clean 且 integration commit/测试证据已记录后才移除 worktree。Contract/artifact、Atlas、lockfile 和 shared integration 始终直接在 integration worktree 由主线程完成，不经过 lane handoff。

cherry-pick 只负责传递已经由主线程创建的 lane commits，不改变 Contract 规则：所有 Bundle 的 `backend_commit` 最终只绑定 integration branch 上已经提交且 clean 的 SHA。

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

- [ ] 当两个以上无共享写入的 lane 同时 ready 时，主线程必须立即并发分派并继续处理 integration owner 工作，不得把 Wave 3A/3B/3C 或 Wave 4 feature lanes 人为串行化；只有依赖未满足、并发槽不足或用户明确要求串行时例外，并在进度中说明。
- [ ] 每个 Plan 开始前创建隔离 worktree，并记录基线 `git status --short`；不得使用宽泛 `git add -A`。
- [ ] 每个 Task 严格执行“失败测试 -> 确认失败原因 -> 最小实现 -> 同一测试通过 -> 返回差异和证据”。
- [ ] 子执行器默认不运行 `git add`、`git commit`、merge 或 rebase。各计划中的“提交”步骤和命令只表示主线程集成检查点；主线程审查当前差异并重新运行门禁后才可执行。
- [ ] 后端 transport 固定写入平台常量；禁止读取 `platform` header/query/body 决定 provenance。
- [ ] 所有 repository 的资源查询必须显式包含 `platform + user_id + is_del`；禁止先按 ID 查出再在 service 比较归属。
- [ ] 两个前端都先同步 Contract Bundle 再生成类型；禁止 `any`、备用 DTO、字段别名和运行时 mock。
- [ ] 真实 COS、邮件、付费上游和公网 prompt feed 只做用户手工验收；自动测试使用 fake、httptest、MinIO/COS fixture 或本地数据。
- [ ] 每个主线程提交只包含一个经审查的 task/integration slice；执行器 diff 与主线程 wiring 只有在 ownership 已收口且定向门禁重跑后才能进入同一检查点。`canvas_front_next/a` 在 committed baseline 中已经删除，后续工作树和 index 都不得重新出现该路径。

## Release Acceptance

- [ ] registry 精确包含 `admin` 和 `infinite_canvas`，退役的 `app/canvas` 仍被拒绝。
- [ ] 同一用户可持有不同 Admin/Canvas 角色；token、refresh Cookie、session、logout、登录日志和 principal 不串平台。
- [ ] 新邮箱验证码登录创建共享用户和 Canvas binding；未知账号密码登录不创建；没有独立注册入口。
- [ ] 项目以服务端 canonical document 为准，自动保存串行，revision 冲突不静默覆盖，IndexedDB 只保存恢复草稿。
- [ ] JPEG/PNG/WebP 通过固定 key 的 STS 上传，确认时 HEAD、限流读取、真实 MIME、尺寸和 SHA-256 全部一致。
- [ ] 提示词来源由 Admin 管理，Cron 只入队，Worker 严格解析；失败保留上次成功数据且无法 SSRF 内网。
- [ ] Canvas UI 中不存在渠道、WebDAV、Agent、插件、文档站、智能体/GitHub/版本发布外链、audio/video 或第三方生成调用；经过校验的提示词 HTTPS cover/reference 仍按规格允许。
- [ ] Admin Bundle 和 Infinite Canvas Bundle 无 drift；两个前端 strict typecheck、production build 和定向测试通过。
- [ ] 桌面与移动 Playwright 覆盖登录、项目、素材、提示词、跨设备读取和 revision 冲突，且无控件重叠或文本溢出。

## Verification Budget

各 Plan 和执行器只运行自己列出的定向测试。主线程在每个合并点重新运行当前工作树的短门禁，默认自动执行：

`scripts/database/atlas.ps1` 固定使用 `--network none`，因此短门禁只用它执行不连接数据库的 checksum-backed `migrate validate`。`migrate status` 必须连接目标数据库，且 104 在 activation 前按设计保持 pending；它只属于 Wave R：disposable fixture 通过受限 runtime config 验证唯一 pending migration 是 104，获批 release controller 在生产 activation 前后分别验证 pending 集合和最终 clean status。

```powershell
go test ./internal/module/auth/... ./internal/module/permission/... ./internal/module/role/... ./internal/module/user/... ./internal/module/canvasproject/... ./internal/module/ai/asset/... ./internal/module/ai/prompt/... ./internal/module/crontask/... ./internal/infra/storage/cos ./internal/platform/admin ./internal/platform/infinitecanvas ./internal/server/... ./internal/jobs/... ./internal/runtime ./internal/admincontract ./internal/infinitecanvascontract
go test ./internal/architecture -run 'InfiniteCanvas|PlatformRBAC' -count=1
pwsh -NoProfile -File scripts/database/atlas.ps1 migrate validate --dir file://database/migrations
pwsh -NoProfile -File scripts/verify-runtime-contracts.ps1
npm -C E:/admin/admin_front_ts run build:check
npm -C E:/admin/canvas_front_next run verify
git diff --check
```

以下属于 Wave R 长验收，只有用户明确授权后由主线程串行调度；不得与写同一数据库、生成目录或浏览器产物的其他执行器并发：

```powershell
go test ./...
go test -race ./internal/module/auth ./internal/module/permission ./internal/module/role ./internal/module/user ./internal/module/canvasproject ./internal/module/ai/asset ./internal/module/ai/prompt ./internal/server ./internal/runtime
pwsh -NoProfile -File scripts/infinite-canvas-smoke.ps1 -Mode MigrationStatus -ExpectedPending 202607310104
npm -C E:/admin/canvas_front_next run test:e2e
```

短门禁通过可声明“开发完成”；Wave R、真实 COS、邮件和公网 prompt feed 未执行时必须在交付报告中列为人工发布验收项，不能声明“发布验收完成”。无论执行哪一级，Contract 目录都不得 drift，三个仓库必须 clean，且 `canvas_front_next/a` 不得重新出现。
