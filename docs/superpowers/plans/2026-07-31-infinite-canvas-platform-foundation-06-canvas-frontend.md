# Infinite Canvas ToC 前端 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在已经建立 React/Vite 基础框架的 `canvas_front_next` 仓库中交付可直接使用的 Infinite Canvas ToC 前端：独立认证、云端项目、严格画布编辑、IndexedDB 恢复草稿、私有 COS 素材和只读提示词库，并彻底移除原项目的运行时配置与第三方生成面。

**Architecture:** 浏览器只消费同步到仓库的 Infinite Canvas Contract Bundle 和由其生成的类型化 client。access token 只在内存 token vault 中，refresh Cookie 由浏览器管理；项目服务端 document/revision 是同步真相，IndexedDB 只保存恢复草稿。画布运行态和持久化 document 分离，图片始终以 asset id 持久化并通过统一 resolver 获取短期读取地址。

**Tech Stack:** React 19、TypeScript 5 strict、Vite 7、TanStack Router、TanStack Query、Zustand、Ant Design、Lucide、IndexedDB `idb`、腾讯云 COS Browser SDK、Vitest、Testing Library、MSW、Playwright、OpenAPI 3.1、Hey API fetch client/codegen。

---

## 执行边界

> **并行与提交覆盖规则：** 实施时同时遵守 `E:\admin\LONG_TASK_PARALLEL_EXECUTION.md` 和 execution index。子执行器只修改分配的 feature/test 目录并返回 diff/测试证据，不运行 `git add`、`git commit`、merge 或 rebase；contract sync/generated tree、`src/modules/http/{client,error}.ts`、`src/app/**`、cross-feature canvas/navigation wiring、package/lockfile 和最终提交归主线程。Auth/Project executor 可独占 `token-vault.ts` 和 `shared/layout/**`；Canvas executor 独占 `canvas/**`/`drafts/**` 直到交付；Product executor 不直接修改这两组目录。下文“提交”步骤均为主线程检查点。

- 所有前端路径相对 `E:\admin\canvas_front_next`；Contract 源路径为 `E:\admin\admin_back_go\contracts\infinite-canvas\v1`。
- 已审查基础提交 `25538629587da498dd5e4dcb38d79db54c728100` 已正式删除原占位文件 `a`。任何 Task 都不得恢复、创建或重新跟踪 `a`；每次提交前必须证明 `git ls-files -- a` 无输出且 `Test-Path -LiteralPath .\a` 为 `False`，并使用显式 pathspec 暂存。
- 参考项目 `E:\admin\infinite-canvas\web` 只能逐文件阅读和提取交互思路。禁止复制其 `.env`、runtime config、API service、持久化 store、内置提示词来源或整棵 `src`。
- API 根路径固定 `/api/infinite-canvas/v1`。客户端不发送可选 platform header/query/body，不存在 Admin API fallback。
- 首期 route 精确为 `/login`、`/projects`、`/canvas/:projectId`、`/assets`、`/prompts`；根路由只按会话跳转，不创建 landing page，也没有 `/register`。
- 首期节点精确为 `text/image/config/group`，`config` 在界面显示“提示词配置”。没有生成、执行、渠道、模型、Provider、API Key、Base URL、WebDAV、Agent、插件、文档站、智能体/GitHub/版本发布外链、audio 或 video 入口；提示词中经过校验的 HTTPS cover/reference 不属于被禁入口。
- `contracts/backend/**` 和 `src/modules/http/generated/**` 只能由脚本产生，业务代码不得手写备用 DTO、`any`、宽泛 index signature 或旧字段 alias。
- 自动测试不得访问真实邮件、COS 或公网提示词来源；Contract、MSW、fake IndexedDB 和本地后端 fixture 是自动测试边界。
- 当前基础框架已在上述 clean baseline 中提交，包含稳定 session bootstrap surface、providers、theme、route constants、架构测试和质量门禁，但还没有 Contract、Auth/RBAC/COS/Prompt 业务调用。Task 1 只复核并冻结该事实，不再创建重复 F0 提交。
- `@hey-api/openapi-ts@0.99.0` 与 `@hey-api/client-fetch@0.13.1` 是当前批准的生成/runtime 组合；真正加入 lockfile 前必须使用官方 registry 重新做全依赖 audit，且不得新增当前已记录 ESLint 9 `brace-expansion` 之外的 high/critical 公告。若不满足则停止 Contract Task 并更新计划，禁止静默换回有已知公告的生成器。

## 目标文件结构

```text
canvas_front_next/
  contracts/backend/infinite-canvas/v1/{openapi,permissions,manifest}.json
  contracts/backend/infinite-canvas.lock.json
  scripts/{sync,generate,check}-infinite-canvas-contract.mjs
  scripts/check-product-surface.mjs
  src/app/{App,AppProviders,queryClient,routes,router,protected-route}.tsx
  src/modules/http/{client,error,token-vault}.ts
  src/modules/http/generated/**
  src/features/auth/**
  src/features/projects/**
  src/features/canvas/**
  src/features/drafts/**
  src/features/assets/**
  src/features/prompts/**
  src/shared/{layout,theme,ui}/**
  tests/{unit,component,integration,architecture}/**
  e2e/**
```

### Task 1: 复核并冻结已经提交的独立前端基础基线

**Files:**
- Existing foundation: `.editorconfig`、`.gitignore`、`.prettierignore`、`.prettierrc.json`、`package.json`、`package-lock.json`、`tsconfig*.json`、Vite/Vitest/Playwright/ESLint config、`index.html`
- Existing application: `src/main.tsx`、`src/app/**`、`src/shared/**`、`src/styles/**`
- Existing tests: `tests/foundation/**`、`tests/architecture/product-surface.test.ts`
- Existing docs: `README.md`、`docs/frontend-architecture.md`

- [ ] **Step 1: 证明 committed baseline、空工作树和删除路径边界**

记录完整状态并拒绝任何未审查 baseline 漂移；`output/playwright/**` 必须保持 ignored：

```powershell
$expectedBaseline = '25538629587da498dd5e4dcb38d79db54c728100'
$head = (git rev-parse HEAD).Trim()
if ($head -ne $expectedBaseline) { throw "canvas baseline changed; review and update execution index first" }
git status --short
git ls-files -- a
Test-Path -LiteralPath .\a
git status --short --ignored -- output/playwright
```

Expected: `git status --short` 和 `git ls-files -- a` 无输出，`Test-Path` 为 `False`，ignored status 输出 `!! output/playwright/`。不得恢复、创建或重新跟踪 `a`。

- [ ] **Step 2: 重新运行基础依赖、安全和质量门禁**

```powershell
npm ci --dry-run
npm audit --omit=dev --audit-level=high --registry=https://registry.npmjs.org
npm run verify
git diff --check
```

Expected: lock 与 manifest 同步；生产依赖 `0 vulnerabilities`；格式、ESLint、TypeScript、5 个测试文件/7 个测试和 production build 全部退出 0。全量 dev audit 中 ESLint 9 `brace-expansion` 公告作为已记录的开发工具上游阻塞，不允许误报为生产依赖漏洞，也不使用 `--force` 绕过 React Hooks peer contract。

- [ ] **Step 3: 记录 F0 复核证据，不创建新提交**

```powershell
git show --stat --oneline $expectedBaseline
git status --porcelain=v1 --untracked-files=all
git diff --check
```

Expected: baseline 证明基础框架和 `a` 删除已在同一提交中，工作树与 index 都为空；本 Task 不执行 `git add` 或 `git commit`。

### Task 2: 在正式 Bundle 后建立确定性 Contract client

**Files:**
- Create: `scripts/{sync,generate,check}-infinite-canvas-contract.mjs`
- Create: `scripts/check-dependency-audit.mjs`：执行官方 registry audit，要求 production 为零漏洞并只允许精确记录的 ESLint 开发期公告。
- Create: `openapi-ts.config.ts`
- Create: `contracts/backend/infinite-canvas/v1/{openapi,permissions,manifest}.json`
- Create: `contracts/backend/infinite-canvas.lock.json`
- Generate: `src/modules/http/generated/**`
- Create: `src/modules/http/{client,error}.ts`
- Test: `tests/architecture/{contract-pipeline,repository-baseline}.test.ts`

- [ ] **Step 1: 写 Contract 缺失、篡改和跨平台失败门禁**

测试断言：tracked bundle 缺失时 `contract:check` 失败；manifest commit 非 40 位小写 SHA、artifact hash 不符、OpenAPI 出现非 `/api/infinite-canvas/v1/**` path、permissions 出现非 `infinite_canvas_*` code、generated tree 被手改或 path `a` 进入 index 时失败。

Run: `npm test -- tests/architecture/contract-pipeline.test.ts tests/architecture/repository-baseline.test.ts`

Expected: FAIL，原因是正式 bundle/scripts 尚不存在，不得因测试编译错误失败。

- [ ] **Step 2: 从已提交 backend SHA 同步 Bundle**

`contract:sync -- --backend E:/admin/admin_back_go --commit <sha>` 必须读取 backend manifest，验证参数 commit 与 `backend_commit` 相同，逐个验证 manifest SHA-256，再原子替换本地三文件并写 lock。不能从 backend 工作树任意 OpenAPI 文件、URL 或 npm package 获取契约。

- [ ] **Step 3: 加入经审计的生成器并生成 fetch client**

只在 `check-dependency-audit.mjs` 证明没有新增 high/critical 后精确安装 `@hey-api/openapi-ts@0.99.0` 和 `@hey-api/client-fetch@0.13.1`，移除不再使用的 `openapi-fetch`。配置只生成 TypeScript types、SDK operation functions 和 fetch client；输出固定到 `src/modules/http/generated`。禁止生成 React Query hooks，Query ownership仍在 feature repository 层。

本 Task 的 HTTP runtime 只在 generated client 上统一配置 API base、`credentials: 'include'`、request ID、严格错误 envelope，以及后续注入 access-token/refresh 协调器的 typed hooks；此时不实现 token storage 或 401 refresh。内存 token vault 与 single-flight refresh 归 Task 3，业务 feature 始终不直接调用裸 `fetch`。

- [ ] **Step 4: 实现逐字 drift check**

`contract:check` 在临时目录重新同步、生成并逐字比较 bundle、lock 和 generated tree；拒绝未声明 path、手工修补 generated file、备用 response interface 或不同 backend SHA。

- [ ] **Step 5: 运行 Contract 门禁并由主线程提交**

```powershell
npm run contract:check
npm run typecheck
npm test -- tests/architecture/contract-pipeline.test.ts tests/architecture/repository-baseline.test.ts
npm run build
npm audit --omit=dev --audit-level=high --registry=https://registry.npmjs.org
node scripts/check-dependency-audit.mjs
git ls-files -- a
Test-Path -LiteralPath .\a
```

Expected: 全部退出 0；生产依赖 `0 vulnerabilities`，完整 audit 没有超出精确 ESLint allowlist 的 high/critical；Canvas lock/manifest 精确绑定主线程提供的 backend runtime SHA；`a` 仍不存在且未被跟踪。

```bash
git add package.json package-lock.json openapi-ts.config.ts scripts contracts/backend src/modules/http tests/architecture
git commit -m "chore(canvas): 接入确定性契约流水线"
```

### Task 3: 实现内存会话、启动恢复和登录流程

**Files:**
- Create in Auth/Project lane: `src/modules/http/token-vault.ts`
- Create: `src/features/auth/{api,store}.ts`
- Create: `src/features/auth/{session-bootstrap,login-page,login-form,password-reset-dialog}.tsx`
- Main-thread integration only: modify `src/modules/http/client.ts`
- Main-thread integration only: create `src/app/{router,protected-route}.tsx`
- Main-thread integration only: modify `src/app/{App,AppProviders}.tsx`
- Test: `tests/unit/auth/{token-vault,single-flight-refresh}.test.ts`
- Test: `tests/component/auth/login-page.test.tsx`
- Test: `tests/integration/auth/session-lifecycle.test.tsx`

- [ ] **Step 1: 先写会话状态机和 refresh 竞争失败测试**

覆盖 `booting -> anonymous|authenticated`、启动只 refresh 一次、20 个并发 401 只产生一次 refresh、等待请求用新 token 各重放一次、refresh 自身 401 不递归、失败清空状态并跳 `/login`、logout 即使网络失败也清本地状态。证明 localStorage/sessionStorage/IndexedDB 中都没有 access/refresh token。

- [ ] **Step 2: 执行器实现内存 token vault，主线程接入类型化 HTTP client**

执行器交付的 token vault 只保存在模块闭包，暴露 `get/set/clear/subscribe` 最小接口。主线程将它接入 Task 2 的共享 client：所有 API 请求使用 generated client、`credentials: 'include'`、`Authorization: Bearer <memory token>`；统一解析正式 `{code,data,msg}` envelope 为 `ApiError{status,code,message,details}`，不把后端原始响应或 token 写日志。

401 middleware 只处理受保护业务请求：共享一个 `refreshPromise`，成功后重放原请求一次；login、verification、password reset、refresh 和 logout 不进入该循环。403 不 refresh，404/409 保留给业务层，5xx 只提供显式重试。

- [ ] **Step 3: 实现 session bootstrap、`/me` 和 route guard**

Auth executor 实现 session bootstrap/store 和可测试的 guard decision；主线程在 `src/app/**` 完成 router/protected-route wiring。应用挂载先 `PUT /auth/session`，成功写内存 token 后读取 `/me`，二者完成前渲染固定尺寸初始化状态。受保护 route 只在 authenticated 时挂载；anonymous 进入根或受保护地址时跳 `/login` 并保留站内 return path，不能接受外部 redirect URL。

- [ ] **Step 4: 实现无注册入口的双方式登录**

登录页先读取 `/auth/login-config`，用 tabs 提供“邮箱验证码”和“账号密码”。验证码发送严格走 fresh captcha -> `/auth/verification-codes(scene=login)`；密码登录提交前也按 login config 完成独立 fresh captcha，再调用 `/auth/sessions`。邮箱 code session 不复用已经被发送请求消费的 captcha。服务端返回 `is_new_user=true` 时仍直接进入 `/projects`，不出现注册确认页。密码找回作为 modal/drawer，走 fresh captcha、`scene=forget` 和 `/auth/password-resets`，完成后回到密码登录。

页面覆盖 config loading/unavailable、captcha renew、发送倒计时、429、错误账号密码、平台注册关闭和邮件渠道不可用状态。表单与按钮不暴露 provider/channel 字样，不提供注册链接。

- [ ] **Step 5: 运行 auth 测试并提交**

```powershell
npm test -- tests/unit/auth tests/component/auth tests/integration/auth
npm run typecheck
git ls-files -- a
Test-Path -LiteralPath .\a
```

```bash
git add src/modules/http/token-vault.ts src/modules/http/client.ts src/features/auth src/app/router.tsx src/app/protected-route.tsx src/app/App.tsx src/app/AppProviders.tsx tests/unit/auth tests/component/auth tests/integration/auth
git commit -m "feat(canvas): 接入独立浏览器认证"
```

### Task 4: 实现项目 API、项目库和产品外壳

**Files:**
- Create: `src/shared/layout/{app-shell,primary-nav,mobile-nav,user-menu}.tsx`
- Create: `src/features/projects/{api,queries}.ts`
- Create: `src/features/projects/{project-list-page,project-card,project-actions,create-project-dialog}.tsx`
- Main-thread integration only: modify `src/app/router.tsx`
- Test: `tests/unit/projects/project-api.test.ts`
- Test: `tests/component/projects/project-list-page.test.tsx`
- Test: `tests/integration/projects/project-mutations.test.tsx`

- [ ] **Step 1: 写 generated operation 与幂等行为失败测试**

断言 list 不依赖 document 字段；create 生成一次 `request_id` 并在网络结果不明重试时复用；rename/delete 带当前 `expected_revision`；copy 只使用 POST body 的 `source_project_id`，不存在 `/copy` action URL。400/404/409 分别进入确定 UI 状态。

- [ ] **Step 2: 实现项目 query/mutation facade**

所有输入输出类型从 generated operations 派生。TanStack Query key 含固定 platform domain 但不把 platform 发给后端；列表分页按服务端稳定顺序。mutation 成功只做精确 cache 更新/失效，失败不乐观提升 revision。

创建操作在 dialog 打开时生成 opaque request id，直到收到确定成功或确定 4xx 前不更换。复制沿用同一机制；重命名、删除如果 409，刷新 detail/list 后向用户展示已变化状态，不能覆盖服务端。

- [ ] **Step 3: 实现 `/projects` 和共享产品外壳**

Auth/Project executor 实现 layout 与项目页面并返回 route descriptor；主线程只负责把它接入 `src/app/router.tsx`。桌面为紧凑顶部导航和可扫描项目网格/列表，移动端使用 drawer；导航只包含项目、素材、提示词、主题和用户退出。项目页提供 loading/skeleton、empty、error、分页、创建、打开、重命名、复制、删除确认。项目标题动态换行/截断且 action 不被挤出；不使用 section card 套 card。

- [ ] **Step 4: 运行项目测试并提交**

```powershell
npm test -- tests/unit/projects tests/component/projects tests/integration/projects
npm run typecheck
npm run build
```

```bash
git add src/shared/layout src/features/projects src/app/router.tsx tests/unit/projects tests/component/projects tests/integration/projects
git commit -m "feat(canvas): 接入云端项目库"
```

### Task 5: 提取并收紧无限画布编辑器

**Files:**
- Create: `src/features/canvas/model/{document,node-registry,geometry,selection,serialization}.ts`
- Create: `src/features/canvas/store/{canvas-store,canvas-ui-store}.ts`
- Create: `src/features/canvas/ports.ts`
- Create: `src/features/canvas/components/{canvas-editor,canvas-node,connections,toolbar,top-bar,mini-map,zoom-controls,context-menu,node-create-menu}.tsx`
- Create: `src/features/canvas/components/nodes/{text-node,image-node,prompt-config-node,group-node}.tsx`
- Create: `src/features/canvas/canvas-page.tsx`
- Test: `tests/unit/canvas/{document,geometry,node-registry,serialization}.test.ts`
- Test: `tests/component/canvas/{editor,node-create-menu,prompt-config-node}.test.tsx`
- Test: `tests/architecture/canvas-node-surface.test.ts`

- [ ] **Step 1: 先写闭合 document 和节点注册表失败测试**

从 generated `CanvasDocumentV1` schema 派生 domain aliases。测试 serializer 精确输出 `nodes/connections/viewport/background_mode/show_image_info`，只允许 `text/image/config/group`；图片只持久化 `asset_id` 和尺寸，config 只持久化 prompt/asset id 列表。拒绝 `blob:`、`data:`、signed URL、object key、provider、model、audio/video 和任意 plugin node 数据。

- [ ] **Step 2: 逐文件提取纯交互逻辑**

只可参考原项目的 canvas geometry、node sizing、connections、selection、pan/zoom、mini-map 和 keyboard interaction。每复制一个算法先写 characterization test，再改为当前 closed types；禁止导入参考项目代码、localforage store、generation helper、plugin registry、agent event bus 或 runtime config。

- [ ] **Step 3: 实现四类 feature-complete 节点**

`text` 支持纯文本编辑与字号；`image` 通过 asset picker 选择 asset id、保持 natural size/free resize；`config` 在 UI 统一显示“提示词配置”，编辑 prompt 并选择最多 32 个素材引用，但没有执行按钮；`group` 支持分组、移动和解除。连线、选择、多选、删除、复制粘贴、撤销重做、缩放和移动保持稳定尺寸与键盘可达。

`ports.ts` 定义 Canvas 所需的最小 typed callbacks（asset pick/resolve、prompt browse/insert），Canvas package 不 import `features/assets` 或 `features/prompts`。isolated component tests 注入 fakes；真实实现只在主线程的 app composition 中绑定。这是 Wave 4 并行文件所有权边界，不得绕过。

- [ ] **Step 4: 实现响应式 editor shell**

桌面侧栏不遮住画布工具，移动端将属性面板放入 drawer/bottom sheet；icon tool 使用 Lucide 并有 tooltip/accessible label。固定格式工具条使用稳定 grid/尺寸，节点文本和按钮不能溢出。无 AI route 时不渲染生成按钮、空 loading 或 disabled provider selector。

- [ ] **Step 5: 运行 canvas 测试并提交**

```powershell
npm test -- tests/unit/canvas tests/component/canvas tests/architecture/canvas-node-surface.test.ts
npm run typecheck
git ls-files -- a
Test-Path -LiteralPath .\a
```

```bash
git add src/features/canvas tests/unit/canvas tests/component/canvas tests/architecture/canvas-node-surface.test.ts
git commit -m "feat(canvas): 提取严格无限画布编辑器"
```

### Task 6: 实现 IndexedDB 恢复草稿、串行自动保存和冲突处理

**Files:**
- Create: `src/features/drafts/{database,draft-schema,draft-repository}.ts`
- Create: `src/features/canvas/sync/autosave-machine.ts`、`use-project-sync.ts`、`save-as-new.ts`、`conflict-panel.tsx`
- Modify: `src/features/canvas/canvas-page.tsx`
- Test: `tests/unit/drafts/draft-repository.test.ts`
- Test: `tests/unit/canvas/autosave-machine.test.ts`
- Test: `tests/integration/canvas/{offline-draft,unknown-result,revision-conflict}.test.tsx`

- [ ] **Step 1: 写自动保存状态机失败测试**

fake timers 覆盖：编辑后 1 秒 debounce；任意时刻最多一条 PUT；飞行期间 30 次编辑只追加一份最新 snapshot；成功 revision 单调提升；离线保存 draft；网络恢复续传；PUT 结果不明先 GET 比对 canonical document；409 停止队列。保存状态严格为 `idle/saving/saved/offline/conflict/error`。

- [ ] **Step 2: 建立 versioned IndexedDB 草稿库**

key 精确为 `infinite_canvas:{user_id}:{project_id}`，value 为 `{schema_version,user_id,project_id,title,base_revision,document,updated_at}`。读取时验证版本、owner、project 和 closed document；非法/旧版本隔离删除，不传到服务端。logout 关闭 DB handle，但同浏览器恢复所需草稿不因普通 logout 自动丢失；切换 user 永远不读取另一 user key。

- [ ] **Step 3: 实现单通道 autosave**

页面先 GET project，再对比 draft：相同 base revision 且 document 不同才提供恢复；云端更新更晚时不静默套用旧 draft。PUT body 固定为 `{expected_revision,schema_version:'canvas_document_v1',document}`。成功写回 revision 后再删除已确认 snapshot 对应 draft；飞行后存在更新则以新 revision 立刻保存最新 snapshot。

网络异常或 abort 造成结果不明时，重新 GET：若 cloud canonical document 与 snapshot 相同，接受 cloud revision；否则保留 draft 并进入 error/offline，不盲目用旧 revision PUT。

- [ ] **Step 4: 实现两种明确的 409 处理**

冲突 panel 显示本地/云端更新时间和云端 revision，只提供：

1. “载入云端版本”：GET 最新 detail，替换 editor、revision 和 draft base 后恢复 autosave。
2. “将本地内容另存为新画布”：保留冲突瞬间 local snapshot，用稳定 request id POST 空项目，再对新项目 revision 1 PUT local document；任一步结果不明都用同 request id/GET 收敛，成功后导航新项目。

原项目在用户选择前绝不继续 autosave，也不实现自动 merge。

- [ ] **Step 5: 实现离开页面 flush 与测试**

每次 editor mutation 都先排队持久化 IndexedDB，再进入 1 秒服务端 debounce。站内 route 离开先等待 draft write，随后等待当前服务端保存并尝试一次 pending save；`beforeunload` 不启动不可靠的异步写入，只在已有 draft write 尚未完成时提示。失败时保留此前已确认写入的 draft 并允许离开，不使用 `sendBeacon` 绕过 auth。

```powershell
npm test -- tests/unit/drafts tests/unit/canvas/autosave-machine.test.ts tests/integration/canvas
npm run typecheck
```

```bash
git add src/features/drafts src/features/canvas/sync src/features/canvas/canvas-page.tsx tests/unit/drafts tests/unit/canvas/autosave-machine.test.ts tests/integration/canvas
git commit -m "feat(canvas): 增加恢复草稿与版本化自动保存"
```

### Task 7: 接入私有 COS 素材和短期 URL resolver

**Files:**
- Create: `src/features/assets/{api,queries}.ts`
- Create: `src/features/assets/{asset-list-page,asset-picker,asset-editor,image-upload}.tsx`
- Create: `src/features/assets/upload/{sha256-worker,sha256-client,cos-uploader}.ts`
- Create: `src/features/assets/resolver/{asset-resolver,use-asset-object-url}.ts`
- Main-thread integration only: create `src/app/canvas-feature-wiring.tsx` and modify `src/app/router.tsx`
- Test: `tests/unit/assets/{sha256,cos-uploader,asset-resolver}.test.ts`
- Test: `tests/component/assets/{asset-list-page,image-upload,asset-picker}.test.tsx`
- Test: `tests/integration/assets/upload-confirm.test.tsx`

- [ ] **Step 1: 写上传凭证和 URL 生命周期失败测试**

覆盖 JPEG/PNG/WebP、20 MiB 前端上限、lowercase SHA-256、intent body、COS key 只能来自响应、STS policy 不被客户端扩展、metadata `x-cos-meta-sha256`、上传后消费同一个 intent。证明临时 secret/session token/object key 不进入 store、query cache、IndexedDB、console 或 error telemetry。

resolver 测试覆盖 expires_at 前预刷新、并发同 asset 只请求一次、读取失败最多刷新签名一次、owner 404 不重试、组件 unmount/replacement 撤销 blob URL。

- [ ] **Step 2: 用 Web Worker 计算文件 SHA-256 并创建 intent**

选择文件立即校验 declared MIME/extension/size；worker 用 Web Crypto 对 ArrayBuffer 计算 64 位 lowercase hex，主线程显示确定进度状态。调用 `POST /asset-upload-intents` 后只在函数局部保留返回的 key、bucket、region、endpoint 和 STS credentials。

- [ ] **Step 3: 用 COS SDK 直传并由后端确认**

COS SDK 只允许 `putObject` 到响应精确 bucket/region/key，设置文件 Content-Type 与 SHA metadata，不提供通配 prefix 或永久密钥。上传完成调用 `POST /assets` 的 image variant `{type:'image',upload_intent_id,...safe metadata}`；后端返回 asset 前不把本地预览视为正式素材。取消/失败保留 pending intent 由服务端清理，客户端不尝试列举或删除 bucket。

- [ ] **Step 4: 实现素材页、picker 和 resolver**

`/assets` 提供图片上传、文本素材创建、筛选、详情编辑、删除和被引用 409 状态。图片 card 不展示 object key/COS 配置；文本使用纯文本预览。picker 支持搜索、选择和空/错状态，并只把 asset id 写入节点。

resolver 以 `{asset_id,read_url,expires_at}` 缓存，提前 30 秒失效。画布展示将签名 URL fetch 为 blob URL；失败时按 asset id 重新 GET content/detail 并仅再尝试一次。blob URL 只存在 resolver，serializer 和 draft schema 均拒绝它。Product executor 以满足 Canvas ports 的 typed picker/resolver exports 交付 feature，不修改 Canvas 源码；主线程在 `canvas-feature-wiring.tsx` 绑定实现并接入 router。

- [ ] **Step 5: 运行素材测试并提交**

```powershell
npm test -- tests/unit/assets tests/component/assets tests/integration/assets
npm run typecheck
npm run build
```

```bash
git add src/features/assets src/app/canvas-feature-wiring.tsx src/app/router.tsx tests/unit/assets tests/component/assets tests/integration/assets
git commit -m "feat(canvas): 接入私有图片素材库"
```

### Task 8: 接入只读提示词库并封闭产品功能面

**Files:**
- Create: `src/features/prompts/{api,queries}.ts`
- Create: `src/features/prompts/{prompt-list-page,prompt-card,prompt-detail,prompt-insert}.tsx`
- Create: `scripts/check-product-surface.mjs`
- Main-thread integration only: modify `src/app/{canvas-feature-wiring,router}.tsx` and `src/shared/layout/{primary-nav,mobile-nav}.tsx`
- Test: `tests/unit/prompts/prompt-api.test.ts`
- Test: `tests/component/prompts/{prompt-list-page,prompt-insert}.test.tsx`
- Test: `tests/architecture/product-surface.test.ts`

- [ ] **Step 1: 先写提示词只读和禁用面失败测试**

断言前端只调用两条 GET prompt operations，不存在 prompt/source mutation。提示词内容始终 text binding，不使用 `dangerouslySetInnerHTML`。插入到 text node 使用 prompt 正文，插入到 config node 使用 prompt 字段；不会调用任何 generation route。

架构测试递归扫描 route、navigation、source 和依赖，拒绝产品 route `/config`、`/image`、`/video`、`/audio`、`/agent`、`/docs`，以及 WebDAV、channel、provider、api key/base URL、plugin manager、GitHub/release 外链、原内置 source code/URL 和第三方 AI host。`config` allowlist 只包含 Vite/TypeScript/Playwright 配置文件、generated `/auth/login-config` operation、Canvas node type 和“提示词配置”组件命名。

- [ ] **Step 2: 实现 `/prompts` 浏览与纯文本详情**

页面消费 generated list/detail operations，支持 keyword/category/tags/source 筛选、稳定分页、loading/empty/error。card 可显示已校验 HTTPS cover，但失败时使用中性占位，不代理或持久化远端图。详情 prompt/reference URL 只作为纯文本/经过 `https:` 校验的显式安全链接；不执行 HTML/Markdown script。

- [ ] **Step 3: 实现画布内插入工作流**

Product executor 通过满足 Canvas ports 的 typed insert exports 交付提示词组件，不直接 import 或修改 canvas/shared/app。主线程在 app composition 中完成 wiring：从提示词页可“在新画布使用”，先创建项目再插入 config/text node；编辑器 side panel 可把提示词插入当前选择或新节点。config 节点仍只编辑 prompt/素材引用，没有模型、质量、渠道或运行按钮。

- [ ] **Step 4: 删除所有旧产品入口并运行静态门禁**

`check-product-surface.mjs` 使用 TypeScript/JSON parser 和明确 allowlist 检查 dependencies、routes、nav、API base paths、source imports，不对 bundle 做模糊字符串误判。确认 reference 项目的以下文件族均未进入新 repo：`services/api/{image,audio,video,model-plugin}`、`webdav-sync`、`components/agent`、`plugin-*`、`config-prompt-sources`、`prompt-source-presets`、`seedance-video`。

- [ ] **Step 5: 运行提示词与产品面测试并提交**

```powershell
npm test -- tests/unit/prompts tests/component/prompts tests/architecture/product-surface.test.ts
npm run surface:check
npm run typecheck
git ls-files -- a
Test-Path -LiteralPath .\a
```

```bash
git add src/features/prompts src/app/canvas-feature-wiring.tsx src/app/router.tsx src/shared/layout/primary-nav.tsx src/shared/layout/mobile-nav.tsx scripts/check-product-surface.mjs tests/unit/prompts tests/component/prompts tests/architecture/product-surface.test.ts
git commit -m "feat(canvas): 接入只读提示词工作流"
```

### Task 9: 完成前端质量、可访问性和视觉门禁

本 Task 只在 Auth/Project、Canvas/Drafts、Product UI 三个 feature lane 都已 frozen、由主线程复测/提交并串行吸收到 integration branch，且 `src/app/**` wiring 完成后执行。以下跨 feature 样式和组件收口全部由主线程串行修改；不得把本 Task 作为与三个 lane 同时写入的第四条 executor lane。

**Files:**
- Create: `tests/component/shared/{responsive-layout,accessibility}.test.tsx`
- Modify: `docs/frontend-architecture.md`
- Modify: `vitest.config.ts`
- Modify: `src/styles/{global,tokens}.css`
- Modify: `src/shared/layout/{app-shell,primary-nav,mobile-nav}.tsx`
- Modify: `src/features/auth/login-page.tsx`
- Modify: `src/features/projects/{project-list-page,create-project-dialog}.tsx`
- Modify: `src/features/canvas/components/{canvas-editor,toolbar,top-bar}.tsx`
- Modify: `src/features/assets/{asset-list-page,asset-picker,image-upload}.tsx`
- Modify: `src/features/prompts/{prompt-list-page,prompt-detail}.tsx`

- [ ] **Step 1: 补齐 reducer/store 边界和覆盖率**

Vitest 覆盖 auth、projects、document/serializer、autosave、draft、upload、resolver、prompts 的 success/empty/error/conflict。`vitest.config.ts` 对 `src/features/**`（排除 generated、type-only index 和 app composition）设置全局 branch/line/function 80%，并对 model/store/sync/upload/resolver 纯逻辑启用 per-file 90% 门槛；不为追覆盖率测试第三方 UI 内部实现。运行 `npm run test:coverage` 必须因任一阈值不足返回非零。

- [ ] **Step 2: 运行 axe 与键盘工作流**

覆盖 login、projects、canvas toolbar/node selection/dialog、assets upload/picker、prompts detail。焦点 trap、返回焦点、Escape、Tab 顺序、图标按钮 accessible name、错误关联、颜色对比和 reduced motion 必须通过。

- [ ] **Step 3: 用组件与静态门禁固定响应式布局不变量**

组件测试覆盖 desktop/mobile navigation mode、toolbar stable dimensions、dialog/drawer responsive constraints、长中文/英文的换行/截断 contract、44px 触控目标和 reduced motion class。`surface:check` 同时扫描禁止的 viewport 字体缩放、负 letter spacing、嵌套 page cards 和未受约束的 fixed board/tool dimensions。真实像素、bounding box 和非空 Canvas 验证不在本 Task 重复执行，统一归 Plan 07 经授权的真实后端 Playwright。

- [ ] **Step 4: 记录前端不变量并运行总门禁**

`docs/frontend-architecture.md` 记录 Contract 真相、内存 token、single-flight refresh、server canonical project、draft key/version、autosave state machine、asset resolver 和禁止产品面。不得写用户教程式 in-app 文案。

```powershell
npm run test:coverage
npm run verify
git diff --check
git ls-files -- a
Test-Path -LiteralPath .\a
```

Expected: 所有短门禁退出 0；`a` 仍不存在且未被跟踪。本 Task 不自动运行 Playwright。

```bash
git add docs/frontend-architecture.md vitest.config.ts tests/component/shared src/styles/global.css src/styles/tokens.css src/shared/layout/app-shell.tsx src/shared/layout/primary-nav.tsx src/shared/layout/mobile-nav.tsx src/features/auth/login-page.tsx src/features/projects/project-list-page.tsx src/features/projects/create-project-dialog.tsx src/features/canvas/components/canvas-editor.tsx src/features/canvas/components/toolbar.tsx src/features/canvas/components/top-bar.tsx src/features/assets/asset-list-page.tsx src/features/assets/asset-picker.tsx src/features/assets/image-upload.tsx src/features/prompts/prompt-list-page.tsx src/features/prompts/prompt-detail.tsx
git commit -m "test(canvas): 锁定前端质量与响应式边界"
```

## 完成标准

- `/login`、`/projects`、`/canvas/:projectId`、`/assets`、`/prompts` 全部消费正式 generated client，无 `/register` 或 Admin fallback。
- access token 仅在内存，启动 refresh 和并发 401 single-flight 行为通过确定测试。
- 项目服务端是唯一同步真相；1 秒串行 autosave、IndexedDB 恢复、结果不明读取和 409 两种处理完整。
- document 只含四类节点和 asset id；任何临时/签名 URL、COS key、渠道或 provider 都不持久化。
- JPEG/PNG/WebP 通过 SHA-256、STS、COS 直传和 intent 确认进入素材库；签名 URL 有刷新上限并释放 blob URL。
- 提示词只读浏览/插入完成，用户不能管理来源，也不能触发 AI 生成。
- strict typecheck、format、lint、Vitest、production build 和产品面静态门禁全部通过；桌面/移动真实 Playwright 作为 Plan 07 release acceptance 单独报告。
- 原占位路径 `a` 保持 committed deletion，未被任何后续提交重新创建或跟踪。
