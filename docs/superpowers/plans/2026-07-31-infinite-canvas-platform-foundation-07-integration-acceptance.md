# Infinite Canvas 集成激活与发布验收 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 Plans 01-06 收敛为同一个可发布系统：追加受保护的 runtime activation migration、冻结并发布同一 backend commit 的双 Contract Bundle、完成数据库/后端/双前端门禁和真实浏览器验收，最后在 handler 已部署的前提下启用提示词同步与上传清理 cron。

**Architecture:** 发布分为 build、deploy、activate 三个不可交换的阶段。build 阶段在 disposable infrastructure 上证明 schema、route、Auth/RBAC、项目、COS、提示词和前端契约；deploy 阶段先发布包含 task registry/Worker handler 的冻结 backend runtime commit；activate 阶段才应用只修改两条 `cron_task.status` 的 migration。任何失败都先把 schedule 关回 disabled，不用删除数据或回滚 additive schema。

**Tech Stack:** Go、MySQL 8.4、Atlas、Redis/Asynq、PowerShell、Admin Vue frontend、Infinite Canvas React frontend、Vitest、Playwright、Docker disposable services、Contract Bundle SHA-256。

---

## 执行边界

> **并行与提交覆盖规则：** 实施时同时遵守 `E:\admin\LONG_TASK_PARALLEL_EXECUTION.md` 和 execution index。本 Plan 的 migration、Atlas、Contract、generated tree、release evidence、部署动作和所有提交均由主线程串行执行；执行器只能承担互不写共享状态的定向验证并返回证据。下文所有“提交”步骤均为主线程检查点。

- 此 Plan 依赖 Plans 01-06 全部合并，且三个仓库都从已提交基线开始并保持 clean。`canvas_front_next/a` 已在基础 baseline 中正式删除，任何工作树或 index 都不得重新出现该路径。
- 只有此 Plan 可以创建 `database/migrations/202607310104_infinite_canvas_runtime_activation.sql` 并为它重算 `database/migrations/atlas.sum`；不得修改 101-103、canonical HCL 或已有 migration。
- activation migration 只允许启用 `infinite_canvas_prompt_sync` 与 `infinite_canvas_asset_upload_cleanup` 两行，不能修改 cron、handler、title、其他 schedule 或业务数据。
- 两个 Contract manifest 必须绑定同一个 40 位 backend runtime commit。冻结后如任何 Go、migration、route、permission 或 DTO 改变，废弃该 SHA，重新运行全部 backend gates 并重新生成两个 Bundle。
- 自动 E2E 使用 disposable MySQL/Redis、测试 SMTP/验证码读取器和本地 COS-compatible fixture；真实腾讯云 COS、邮件投递和公网 prompt feed 另做人工 smoke，不能把凭证写入仓库或测试报告。
- Playwright 必须访问同一个真实 Go router 和生成 client，不允许用 MSW/page.route 替换业务 API。只有外部邮件/COS/feed adapter 可由 acceptance fixture 替代。
- 不启用任何 AI generation、audio/video 或计费功能；发现相关 route/UI 即阻断发布。
- “继续执行计划”默认只授权开发短门禁；`go test ./...`、race、真实 Go router Playwright、长 smoke、真实 COS/mail/feed 属于 release acceptance，必须由用户明确授权后运行。

## Release Invariants

```text
compiled platforms: admin, infinite_canvas
Canvas API prefix:  /api/infinite-canvas/v1
Canvas node types:  text, image, config, group
cron registry names:
  infinite_canvas_prompt_sync
  infinite_canvas_asset_upload_cleanup
task types:
  infinite-canvas:prompt-sync-dispatch:v1
  infinite-canvas:prompt-source-sync:v1
  infinite-canvas:asset-upload-cleanup:v1
```

## Verification Tiers

**Development-complete short gate（默认自动执行）：**

- 受影响 Go package 定向 tests、architecture tests、route/registry tests。
- `verify-runtime-contracts.ps1`、Contract drift/hash check。
- 通过固定 `--network none` 的 `atlas.ps1` 执行 checksum-backed `migrate validate`，并通过 architecture test 静态验证 guarded activation；短门禁不执行任何需要数据库 URL 的 `migrate status`。
- 两个前端的 contract check、typecheck、定向 Vitest、lint/format 和 production build。
- `git diff --check`、敏感字段和退役 surface 静态扫描。

**Release-acceptance long gate（需要用户明确授权）：**

- `go test ./...`、指定 package race、完整 database verifier，以及 disposable database 上的 migration status/apply/status smoke。
- 真实 Go router + disposable services 的 Playwright 全流程。
- 真实腾讯云 COS、真实邮件投递、公网 prompt feed 的人工 smoke。
- cron 部署、canary、104 activation 和停用演练。

短门禁全部通过且长门禁被准确列为未执行项时，可以声明“开发完成”，不能声明“发布验收完成”。只有 release long gate 与人工 smoke 都有证据后才能使用后者。

### Task 1: 追加 guarded runtime activation migration

**Files:**
- Create: `database/migrations/202607310104_infinite_canvas_runtime_activation.sql`
- Create: `internal/architecture/infinite_canvas_activation_test.go`
- Modify: `database/migrations/atlas.sum`
- Create: `docs/operations/infinite-canvas.md`

- [ ] **Step 1: 先写 migration 静态失败测试**

测试要求 migration 只触及 `cron_task`，没有 CREATE/ALTER/DROP/DELETE/INSERT、没有宽泛 status update。必须逐字包含两个 name、两个 handler、两个 cron 表达式、`is_del=2`、preflight guard、`status=2 -> 1` 条件更新和 postflight 两行计数。拒绝对其他 cron name 的 update。

- [ ] **Step 2: 实现 fail-before-write preflight**

沿用仓库既有 temporary guard pattern，在任何 persistent update 前证明：

```text
infinite_canvas_prompt_sync:
  cron=0 */6 * * *
  handler=infinite-canvas:prompt-sync-dispatch:v1
  status=2, is_del=2

infinite_canvas_asset_upload_cleanup:
  cron=15 * * * *
  handler=infinite-canvas:asset-upload-cleanup:v1
  status=2, is_del=2
```

任一行缺失、重复、被删除、已提前启用、handler/cron 漂移都必须在 UPDATE 前失败；不得自动修复运维配置。

- [ ] **Step 3: 只做条件激活并验证结果**

事务中按两个精确 name 且 `status=2 AND is_del=2` 更新 `status=1, updated_at=CURRENT_TIMESTAMP`。Rows matched 必须为 2，postflight 再证明两行 active 且所有 handler/cron 未变化。migration 本身不能探测进程；“handler 已部署”由后续 release gate 保证。

- [ ] **Step 4: 更新 Atlas checksum 并运行 schema 门禁**

```powershell
pwsh -NoProfile -File scripts/database/atlas.ps1 migrate hash --dir file://database/migrations
pwsh -NoProfile -File scripts/database/atlas.ps1 migrate validate --dir file://database/migrations
go test ./internal/architecture -run 'TestInfiniteCanvasActivation|TestInfiniteCanvasPlatformSchema|TestReconciliationSchema' -count=1
git diff --check
```

Expected: checksum 只因 104 append 改变；canonical HCL 无 diff。

- [ ] **Step 5: 记录显式停用操作并提交**

运维文档给出 incident action：在事务中按两个 name 将 active `status=1` 条件改回 `status=2`，随后确认 scheduler reload/重启；这不是 Atlas down migration，也不删除已排队任务。记录如何暂停队列、排查失败 job、恢复 schedule。

```bash
git add database/migrations/202607310104_infinite_canvas_runtime_activation.sql database/migrations/atlas.sum internal/architecture/infinite_canvas_activation_test.go docs/operations/infinite-canvas.md
git commit -m "chore(canvas): 增加运行时任务激活迁移"
```

### Task 2: 建立后端全链路门禁和 disposable acceptance fixture

**Files:**
- Create: `internal/acceptance/infinitecanvas/**`
- Create: `scripts/verify-infinite-canvas.ps1`
- Create: `scripts/infinite-canvas-smoke.ps1`
- Create: `scripts/tests/infinite-canvas-smoke.tests.ps1`
- Modify: `scripts/verify-runtime-contracts.ps1`
- Test: backend integration packages

- [ ] **Step 1: 写 fixture 真实性和失败关闭测试**

fixture 必须启动正式 platform graph/router、真实 GORM repositories、Redis session/lease/Asynq 和 migration schema；只替换 SMTP delivery、COS remote transport 和公网 feed HTTP。测试证明 fixture 没有额外 HTTP route、绕过 Auth/RBAC header、固定 access token、跳过 document validator 或直接写业务响应。

邮件验证码通过进程外 Redis reader 按 v2 platform key 读取，captcha answer沿用 `basic-admin-smoke.ps1` 的 Redis secret reader；两者只存在 acceptance script，不暴露 production endpoint。每次 run 使用随机数据库、Redis namespace、邮箱、用户和对象 prefix，退出后按已验证 scope 清理。

- [ ] **Step 2: 实现本地 COS/feed adapters**

COS fixture 只接受已签发的单一 key、PUT/HEAD/GET/DELETE 和 SHA metadata，能模拟 size/MIME/hash mismatch、expired credential、404 和断流；浏览器仍走 COS SDK，后端仍走 private gateway。Feed fixture 提供六来源的严格 JSON snapshot、ETag/304、超限、redirect-to-private 和失败响应，用于 Worker 行为测试。

- [ ] **Step 3: 建立后端 smoke matrix**

`infinite-canvas-smoke.ps1` 至少验证：

1. `/health`/`/ready` 后，Canvas login config/captcha/code/session/refresh/logout 正常。
2. 新邮箱验证码创建共享 user 和仅 Canvas binding；未知密码账号不创建。
3. Admin token/Cookie 调 Canvas 为 401，Canvas token/Cookie 调 Admin 为 401；一边 logout 不撤销另一边。
4. 项目 create replay、不同 fingerprint 409、rename/save/delete revision、越权 404。
5. 上传 intent policy、COS PUT、图片确认、并发确认同 asset、签名读取、被引用删除 409。
6. Admin manual prompt/source CRUD 与 queue sync；Canvas 只读启用 prompt。
7. prompt sync 失败保留旧数据；cleanup 对 missing object 幂等。
8. 两条 cron 在 activation migration 前仍 disabled，registry 和 Worker handler 已存在。
9. `-Mode MigrationStatus -ExpectedPending 202607310104` 创建独立 disposable schema，通过 `atlas-runtime-common.ps1` 的临时受限 config 和 fixture network 运行 pinned Atlas：先 apply 到 103，解析 status 并要求 pending 文件精确只有 104；再 apply 104，要求输出包含 `Migration Status: OK`、pending 数为零，并验证除两条目标 cron 的 `status` 外没有数据变化。不得调用固定 `--network none` 的 `scripts/database/atlas.ps1` 执行数据库命令，也不得接受未知 pending migration。

- [ ] **Step 4: 实现默认短门禁和显式 `-Release` 长门禁**

`scripts/verify-infinite-canvas.ps1` 默认按 fail-fast 顺序只运行短门禁，不吞 exit code：

```powershell
go test ./internal/module/auth/... ./internal/module/permission/... ./internal/module/role/... ./internal/module/user/... ./internal/module/canvasproject/... ./internal/module/ai/asset/... ./internal/module/ai/prompt/... ./internal/module/crontask/... ./internal/infra/storage/cos ./internal/platform/admin ./internal/platform/infinitecanvas ./internal/server/... ./internal/jobs/... ./internal/runtime ./internal/admincontract ./internal/infinitecanvascontract
go test ./internal/architecture -run 'InfiniteCanvas|PlatformRBAC' -count=1
pwsh -NoProfile -File scripts/verify-runtime-contracts.ps1
pwsh -NoProfile -File scripts/database/atlas.ps1 migrate validate --dir file://database/migrations
go vet ./internal/module/auth/... ./internal/module/permission/... ./internal/module/role/... ./internal/module/user/... ./internal/module/canvasproject/... ./internal/module/ai/asset/... ./internal/module/ai/prompt/... ./internal/module/crontask/... ./internal/infra/storage/cos ./internal/platform/admin ./internal/platform/infinitecanvas ./internal/server/... ./internal/jobs/... ./internal/runtime ./internal/admincontract ./internal/infinitecanvascontract
git diff --check
```

只有调用者显式传入 `-Release` 时，脚本才追加：

```powershell
go test ./...
pwsh -NoProfile -File scripts/verify-durable-work.ps1
pwsh -NoProfile -File scripts/verify-database.ps1 -Mode all
pwsh -NoProfile -File scripts/infinite-canvas-smoke.ps1 -Mode MigrationStatus -ExpectedPending 202607310104
go test -race ./internal/module/auth ./internal/module/permission ./internal/module/role ./internal/module/user ./internal/module/canvasproject ./internal/module/ai/asset ./internal/module/ai/prompt ./internal/server ./internal/runtime
```

Pester tests 必须证明默认 invocation 不调用任何 long command 或数据库连接型 Atlas command，`-Release` 才调用 migration status fixture 和其他 long command，且任一失败立即返回非零。Windows 无 C compiler 时复用 `verify-runtime-contracts.ps1` 的 pinned Linux Go image 执行同一 race package list，不能降级为无 race 通过。

- [ ] **Step 5: 运行 smoke script 自测并提交**

```powershell
Invoke-Pester scripts/tests/infinite-canvas-smoke.tests.ps1
pwsh -NoProfile -File scripts/verify-infinite-canvas.ps1
git diff --check
```

Expected: 默认只执行 short gate。此 Task 不自动调用 `-Release`。

```bash
git add internal/acceptance/infinitecanvas scripts/verify-infinite-canvas.ps1 scripts/infinite-canvas-smoke.ps1 scripts/tests/infinite-canvas-smoke.tests.ps1 scripts/verify-runtime-contracts.ps1
git commit -m "test(canvas): 增加平台全链路后端门禁"
```

### Task 3: 冻结 backend runtime commit 并发布双 Contract Bundle

**Files:**
- Modify by generators: `contracts/admin/v1/**`
- Modify by generators: `contracts/infinite-canvas/v1/**`
- Verify: `internal/admincontract/**`、`internal/infinitecanvascontract/**`
- Create: `docs/releases/infinite-canvas-contract-freeze.md`

- [ ] **Step 1: 在 clean checkout 运行冻结前全部门禁**

确保 Task 1/2 已提交，Plans 01-05 的所有 runtime changes 都已提交，且 `git status --porcelain --untracked-files=all` 为空。运行 `scripts/verify-infinite-canvas.ps1` 成功后再次确认工作树仍为空，再记录：

```powershell
pwsh -NoProfile -File scripts/verify-infinite-canvas.ps1
$status = git status --porcelain --untracked-files=all
if ($status) { throw "backend changed during freeze gates" }
$backendCommit = (git rev-parse HEAD).Trim()
```

必须是 40 位小写 SHA。这个 SHA 是最终 runtime commit；后续 bundle artifact commit 只能修改 generated contracts/release evidence，不能修改 Go、migration、schema、seed 或 scripts。

- [ ] **Step 2: 用同一 SHA 串行生成并校验两个 Bundle**

```powershell
pwsh -NoProfile -File scripts/generate-admin-contract.ps1 -BackendCommit $backendCommit
pwsh -NoProfile -File scripts/generate-infinite-canvas-contract.ps1 -BackendCommit $backendCommit
pwsh -NoProfile -File scripts/check-admin-contract.ps1 -BackendCommit $backendCommit
pwsh -NoProfile -File scripts/check-infinite-canvas-contract.ps1 -BackendCommit $backendCommit
```

读取两个 manifest，断言 `backend_commit` 都逐字等于 `$backendCommit`；逐个重算 artifact SHA。Admin bundle 必须保留 views/realtime artifacts并含新增 RBAC/prompt routes；Canvas bundle 只能有三文件和 `/api/infinite-canvas/v1/**` routes。

两个 generator 各自对单个 Bundle 原子写入，但二者不是跨目录事务；任一 generate/check 失败时不得 stage 或提交任何一边，修复后从同一 clean `$backendCommit` 重新生成并校验两边。

- [ ] **Step 3: 做 route/permission 差异审计**

生成机器可读矩阵，证明：Canvas route 数量精确包含 7 auth + `/me` + 6 project + 7 asset + 2 prompt；没有 register、Admin/payment/health/ready 或 generation route。Canvas permissions 精确六项。Admin 包含 prompt/source 和平台角色管理 operations，但没有 Canvas browser auth/project/asset operations。

- [ ] **Step 4: 提交生成 artifacts 并禁止 runtime 漂移**

```bash
git add contracts/admin/v1 contracts/infinite-canvas/v1 docs/releases/infinite-canvas-contract-freeze.md
git commit -m "chore(contract): 冻结双平台发布契约"
```

提交后再次 diff `$backendCommit..HEAD`，只允许上述 contract/evidence paths；否则本 Task 失败并重新冻结。

### Task 4: 同步两个前端并运行 production gates

**Files:**
- Generated/modified in `E:\admin\admin_front_ts`: synced Admin bundle/client
- Generated/modified in `E:\admin\canvas_front_next`: synced Canvas bundle/client
- Create: each frontend contract freeze evidence

- [ ] **Step 1: 同步 Admin Bundle 并生成 client**

```powershell
npm -C E:/admin/admin_front_ts run contract:sync -- --backend E:/admin/admin_back_go --commit $backendCommit
npm -C E:/admin/admin_front_ts run contract:generate
npm -C E:/admin/admin_front_ts run contract:check
npm -C E:/admin/admin_front_ts run build:check
```

运行角色、用户平台 binding、提示词、来源页面定向测试，再运行 `npm -C E:/admin/admin_front_ts run verify:frontend`。禁止手工改 generated client 通过编译。

Admin lock 和同步后的 manifest 中 `backend_commit` 必须逐字等于 `$backendCommit`。

- [ ] **Step 2: 同步 Canvas Bundle 并生成 client**

```powershell
npm -C E:/admin/canvas_front_next run contract:sync -- --backend E:/admin/admin_back_go --commit $backendCommit
npm -C E:/admin/canvas_front_next run contract:generate
npm -C E:/admin/canvas_front_next run contract:check
npm -C E:/admin/canvas_front_next run verify
```

Canvas lock 和 manifest commit 必须等于 `$backendCommit`。检查 `git ls-files -- a` 无输出且 `Test-Path -LiteralPath .\a` 为 `False`，显式暂存 generated contract/client/evidence，不重新创建 `a`。

- [ ] **Step 3: 校验两个前端无跨 Bundle 依赖**

Admin source 只能 import Admin generated tree；Canvas source 只能 import Infinite Canvas generated tree。扫描两边 API prefix、operation IDs 和 permission codes，拒绝 Canvas import Admin DTO、Admin 页面调用 Canvas browser API、任何 fallback type 或客户端 platform selector。

- [ ] **Step 4: 提交各自 contract sync**

在各仓库单独提交，只包含本仓库 bundle/client/evidence：

```text
admin_front_ts:   chore(contract): 同步无限画布管理契约
canvas_front_next: chore(contract): 锁定无限画布发布契约
```

### Task 5: 经用户授权后完成真实 Go backend Playwright 发布验收

**Files:**
- Modify: `E:\admin\canvas_front_next\playwright.config.ts`
- Create: `E:\admin\canvas_front_next\e2e\fixtures\{backend,accounts,cos,mail}.ts`
- Create: `E:\admin\canvas_front_next\e2e\{auth,projects,assets,cross-device,conflict,prompts,authorization,responsive}.spec.ts`
- Create: `E:\admin\canvas_front_next\scripts\run-acceptance.mjs`

- [ ] **Step 1: 启动同一冻结 commit 的 acceptance stack**

runner 验证 backend executable/image revision 等于 `$backendCommit`，对 disposable DB 执行 101-103 但暂不执行 104，启动 API、Worker、Redis、queue、SMTP/COS/feed fixture 和 Canvas Vite/preview。等待 `/ready` 与 worker handler probe；端口动态分配并只绑定 loopback。浏览器 base URL 使用允许的 Canvas origin，不复用 Admin origin。

- [ ] **Step 2: 验收认证和双平台隔离**

桌面/移动都完成新邮箱 captcha + code 登录即入驻、已有账号密码登录、启动 refresh、logout 和找回密码。通过第二 browser context 持有 Admin session，验证 token/Cookie 交叉均 401，Canvas logout 后 Admin session 仍有效。检查浏览器 storage 不含 token/credentials。

- [ ] **Step 3: 验收项目、跨设备和两种冲突处理**

创建、重命名、复制、删除项目；编辑四类节点并等到 saved。第二 context 登录同 user 读取相同 canonical document。两个 context 从同 revision 修改：一边保存后另一边收到 conflict，分别运行“载入云端版本”和“将本地内容另存为新画布”，证明原 cloud、新 local copy 都保留预期内容。

再模拟离线、route leave 和 PUT response lost，确认 IndexedDB draft 恢复、GET 比对和 revision 单调，没有静默覆盖。

- [ ] **Step 4: 验收素材和提示词**

上传 JPEG/PNG/WebP fixture，验证 intent -> COS PUT -> asset confirm -> picker -> image/config reference -> autosave。刷新签名、跨设备加载图片、被引用删除 409、解除引用后删除成功。尝试超限/伪图片必须失败且无 asset row。

浏览/筛选/查看 prompt，以纯文本插入 text/config 节点并保存。UI 中不能出现来源编辑、同步、渠道、provider、WebDAV、Agent、插件、文档站、智能体/GitHub/版本发布外链、audio/video 或生成按钮；经过校验的提示词 HTTPS cover/reference 可以显示。

- [ ] **Step 5: 验收 owner/RBAC 404 和视觉布局**

第二用户直接访问第一用户 project/asset detail/content/delete 均为 404；移除 Canvas binding 或 read/write permission 后现有 session fail closed。Admin Canvas-only 用户无 Admin role 时不能登录 Admin。

在 1440x900、1280x720、390x844、360x800 采集 screenshot 和 bounding-box assertions。检查 canvas root 非空、图片像素非占位、toolbar/nav/dialog/drawer/node 不重叠、长 title/prompt/error 不溢出、移动端触控目标可点击。

- [ ] **Step 6: 在 release authorization 下运行无 API mock 的 Playwright suite**

```powershell
npm -C E:/admin/canvas_front_next run test:e2e
```

Expected: 所有 spec 通过；runner 报告中业务 API interception count 为 0，backend revision 精确匹配，退出后 disposable state 被验证并清理。

没有用户对 release long gate 的明确授权时，本 Task 保持未执行并进入最终手工验收清单，不阻断“开发完成”，但阻断“发布验收完成”和 Task 6 cron activation。

### Task 6: 部署 handler 后激活 cron 并验证异步运行

**Files:**
- External prerequisite: approved immutable release manifest covering backend、Admin frontend、Canvas frontend and migration hold/apply phases
- Modify: `docs/operations/infinite-canvas.md`
- Create after release authorization: `docs/releases/infinite-canvas-release-evidence.md`

现有 `release/admin-only/**`、`scripts/release/*admin-only*` 和两制品 manifest 只覆盖 backend + Admin frontend，不能用于本 Task，也不能为一次发布临时放宽。若用户未授权 release long gate，或平台发布系统尚未提供三制品 manifest、revision/contract lock 校验和独立 migration hold/apply 能力，本 Task 保持 `manual_pending`，不得创建空白/占位 `infinite-canvas-release-evidence.md`；这不阻断“开发完成”，但阻断 cron activation 和“发布验收完成”。

- [ ] **Step 1: 发布冻结 backend runtime commit**

release manifest 必须分别固定 backend artifact HEAD、运行时 `$backendCommit`、Admin frontend HEAD/lock、Canvas frontend HEAD/lock，并在 canary phase 明确 `apply_migrations=false`（或等价的 target 103 hold）。获批 release controller 使用其受管数据库连接运行 pinned Atlas status，必须证明 current version 是 103、pending 文件精确只有 `202607310104_infinite_canvas_runtime_activation.sql`；不得使用固定 `--network none` 的 `scripts/database/atlas.ps1`，也不得在日志或 evidence 中落 database URL。然后部署 API 和 Worker `$backendCommit`，验证 API route registry、cron registry 和 Asynq handler registry 都含固定 name/type。至少执行一次直接 enqueue 的 prompt dispatch/source sync 和 asset cleanup canary，确认 Worker 消费成功；此时数据库两条 cron 必须仍为 status 2。任何未知 pending migration 或部署器自动 apply 104 都是 release blocker，不能事后把 schedule 关回去伪造通过。

- [ ] **Step 2: 发布两个前端并验证 manifest**

部署 Admin/Canvas production build，读取构建 revision 和 bundled contract lock，必须都指向 `$backendCommit`。Canvas Origin、refresh Cookie path、私有 COS CORS 和无 `/register`/旧入口得到 smoke 验证。

- [ ] **Step 3: 执行 104 activation migration**

只有前两步全部通过才由同一获批 release controller 运行 Atlas migrate apply 到 104。apply 后立即再次运行 status，输出必须包含 `Migration Status: OK` 且 pending 数为零；读取 `cron_task` 证明两行 status 1、handler/cron 不变，scheduler reload 后观察下次运行时间。禁止直接在 Admin UI 手工提前开启来规避 migration，status 非 clean 时不得继续 schedule 验收。

- [ ] **Step 4: 验证两条 schedule 的真实行为**

手动触发/等待各一次 schedule：cron 只创建 queue job，prompt dispatch 再为 active source 创建 source jobs，cleanup 扫描 bounded batch。检查 queue type、retry/unique policy、cron log、业务 metrics 和结构化日志；日志中不得出现 STS secret、signed URL query、完整 prompt feed 或 canvas JSON。

- [ ] **Step 5: 演练停用而非 destructive rollback**

在 disposable 环境执行文档中的 `status=1 -> 2` 条件停用并 reload scheduler，证明不再新建 schedule job、已排队 job 可控消费/暂停、业务 HTTP 不受影响。随后按 guarded 流程恢复 status 1。生产只有发生 incident 时执行停用，不为验收反复改数据。

- [ ] **Step 6: 写入并提交脱敏 release evidence**

只有 Steps 1-5 获得授权并全部通过后才创建 `docs/releases/infinite-canvas-release-evidence.md`。记录批准单/三制品 immutable manifest 标识、三个 artifact revision、四处 `$backendCommit` lock、103 hold、activation 前唯一 pending 104、apply 后 `Migration Status: OK`、canary/schedule/停用演练时间与结果；只记录命令摘要和脱敏状态，不记录 database URL、secret、Cookie、signed query、邮箱或 prompt 正文。

```bash
git add docs/operations/infinite-canvas.md docs/releases/infinite-canvas-release-evidence.md
git commit -m "docs(canvas): 记录发布激活证据"
```

Task 保持 `manual_pending` 时跳过本 Step，文件必须不存在；Task 7 只在 coverage matrix 中记录未执行原因，不得提交占位 evidence。

### Task 7: 最终静态泄露、架构文档和覆盖矩阵

Task 7 的开发证据部分可在 Task 4 short gates 后执行；若 Task 5/6 尚未获得 release authorization，则对应行记录为 `manual_pending`。授权并完成 release acceptance 后，主线程更新同一份矩阵为最终状态。

**Files:**
- Modify: `docs/architecture.md`
- Modify: `docs/operations/infinite-canvas.md`
- Create: `docs/releases/infinite-canvas-foundation-acceptance.md`
- Verify: all three repositories

- [ ] **Step 1: 扫描旧入口和敏感字段**

后端、Admin、Canvas 分别执行精确 allowlist scanner，阻断 retired `/api/app`、`/api/canvas` runtime route，Canvas generation/audio/video routes，客户端 provider/channel/API key/Base URL，WebDAV/Agent/plugin/doc links，prompt source preset URL。扫描 tracked source 和 production bundle，不扫描 node_modules/vendor fixture 产生误报。

额外检查日志/响应/generated schema 不暴露 `secret_key`、长期 COS credential、asset object key（upload intent response 是唯一允许边界）、完整 signed query、password/code/access/refresh token、完整 canvas document audit body。

- [ ] **Step 2: 更新双 adapter 架构和运维文档**

记录 shared users + platform memberships、compiled registry、trusted route groups、双 session/Cookie/Origin、两个 Bundle、project revision/draft、private asset、prompt worker、cron activation/停用和未来 AI workflow 扩展点。明确 `users.role_id` 仍是 nullable migration field 且 runtime 禁用，物理删除另行审批。

- [ ] **Step 3: 建立规格到证据覆盖矩阵**

`infinite-canvas-foundation-acceptance.md` 对设计规格 1-17 节逐项列出实现 commit、自动短门禁、Playwright spec、人工 COS/mail/feed smoke 和结果。状态只允许 `passed | manual_pending | failed`：`failed` 阻断开发完成；`manual_pending` 不阻断开发完成但阻断发布验收完成。任何服务端 route/授权缺口都必须是 `failed`，不允许用“前端隐藏了”降级成 `manual_pending`。

- [ ] **Step 4: 运行开发完成总门禁；按授权追加 release gate**

```powershell
pwsh -NoProfile -File scripts/verify-infinite-canvas.ps1
npm -C E:/admin/admin_front_ts run verify:frontend
npm -C E:/admin/canvas_front_next run verify
pwsh -NoProfile -File scripts/check-admin-contract.ps1 -BackendCommit $backendCommit
pwsh -NoProfile -File scripts/check-infinite-canvas-contract.ps1 -BackendCommit $backendCommit
git diff --check
```

只有用户已授权 release long gate 时再运行：

```powershell
pwsh -NoProfile -File scripts/verify-infinite-canvas.ps1 -Release
npm -C E:/admin/canvas_front_next run test:e2e
```

在三个仓库分别运行 status，三者都必须 clean；`canvas_front_next/a` 必须仍不存在且未被跟踪。读取两个 backend manifest 和两个 frontend lock，四处 `backend_commit` 字段必须逐字等于 `$backendCommit`；三个仓库各自 HEAD 另行记录，不要求跨仓库 SHA 相同。

- [ ] **Step 5: 提交最终文档证据**

只提交无 secret、无完整 signed URL、无用户邮箱/验证码的汇总证据：

```bash
git add docs/architecture.md docs/operations/infinite-canvas.md docs/releases/infinite-canvas-foundation-acceptance.md
git commit -m "docs(canvas): 记录平台基础接入验收"
```

## 开发完成标准

- Admin 与 Infinite Canvas Bundle 绑定同一 runtime commit，两个前端 lock 与之完全一致，所有 drift/check/build 门禁通过。
- 定向 Go、Atlas checksum-backed validate、activation 静态测试、route registry、durable task、前端短门禁和静态泄露测试通过。
- Admin/Canvas 身份、角色、permission、token、Cookie、session、logout、Origin 和登录日志无串用。
- Canvas 产品面没有渠道、WebDAV、Agent、插件、文档站、智能体/GitHub/版本发布外链、audio/video、第三方生成或计费入口；提示词 HTTPS cover/reference 符合 allowlist。
- 未执行的 release long gate 和真实外部服务 smoke 已以 `manual_pending` 准确交付；三个仓库都 clean，且 `canvas_front_next/a` 没有重新出现。

## 发布验收完成标准

- disposable database、全量 Go、race、完整 Atlas/database verifier、migration status/apply/status、route registry、durable task 和静态泄露测试通过。
- Playwright 在真实 Go router 上覆盖验证码登录即注册、密码登录、项目、上传、跨设备、冲突两分支、提示词和越权 404，且不 mock 业务 API。
- 真实 COS、邮件和公网 prompt feed smoke 完成，验收证据已脱敏。
- 104 migration 只在冻结 backend handler 已部署并 canary 成功后执行，两条 schedule 可明确停用且没有 destructive rollback。
- 覆盖矩阵中没有 `manual_pending` 或 `failed`。
