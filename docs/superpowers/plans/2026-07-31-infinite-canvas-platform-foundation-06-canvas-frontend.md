# Infinite Canvas ToC 前端 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在空的 `canvas_front_next` 仓库中交付可直接使用的 Infinite Canvas ToC 前端：独立认证、云端项目、严格画布编辑、IndexedDB 恢复草稿、私有 COS 素材和只读提示词库，并彻底移除原项目的运行时配置与第三方生成面。

**Architecture:** 浏览器只消费同步到仓库的 Infinite Canvas Contract Bundle 和由其生成的类型化 client。access token 只在内存 token vault 中，refresh Cookie 由浏览器管理；项目服务端 document/revision 是同步真相，IndexedDB 只保存恢复草稿。画布运行态和持久化 document 分离，图片始终以 asset id 持久化并通过统一 resolver 获取短期读取地址。

**Tech Stack:** React 19、TypeScript 5 strict、Vite 7、React Router、TanStack Query、Zustand、Ant Design、Lucide、IndexedDB `idb`、腾讯云 COS Browser SDK、Vitest、Testing Library、MSW、Playwright、OpenAPI 3.1。

---

## 执行边界

- 所有前端路径相对 `E:\admin\canvas_front_next`；Contract 源路径为 `E:\admin\admin_back_go\contracts\infinite-canvas\v1`。
- 仓库初始只有 tracked path `a` 且用户已删除它。任何 Task 都不得恢复、创建、暂存或提交 `a`；每次提交前必须证明 `git status --short -- a` 仍精确为 ` D a`，并使用显式 pathspec 暂存。
- 参考项目 `E:\admin\infinite-canvas\web` 只能逐文件阅读和提取交互思路。禁止复制其 `.env`、runtime config、API service、持久化 store、内置提示词来源或整棵 `src`。
- API 根路径固定 `/api/infinite-canvas/v1`。客户端不发送可选 platform header/query/body，不存在 Admin API fallback。
- 首期 route 精确为 `/login`、`/projects`、`/canvas/:projectId`、`/assets`、`/prompts`；根路由只按会话跳转，不创建 landing page，也没有 `/register`。
- 首期节点精确为 `text/image/config/group`，`config` 在界面显示“提示词配置”。没有生成、执行、渠道、模型、Provider、API Key、Base URL、WebDAV、Agent、插件、文档、外链、audio 或 video 入口。
- `contracts/backend/**` 和 `src/modules/http/generated/**` 只能由脚本产生，业务代码不得手写备用 DTO、`any`、宽泛 index signature 或旧字段 alias。
- 自动测试不得访问真实邮件、COS 或公网提示词来源；Contract、MSW、fake IndexedDB 和本地后端 fixture 是自动测试边界。

## 目标文件结构

```text
canvas_front_next/
  contracts/backend/infinite-canvas/v1/{openapi,permissions,manifest}.json
  contracts/backend/infinite-canvas.lock.json
  scripts/{sync,generate,check}-infinite-canvas-contract.mjs
  scripts/check-product-surface.mjs
  src/app/{providers,router,protected-route}.tsx
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

### Task 1: 建立空仓库脚手架和确定性 Contract client

**Files:**
- Create: `.editorconfig`、`.gitignore`、`.prettierignore`、`.prettierrc.json`
- Create: `package.json`、`package-lock.json`、`tsconfig*.json`、`vite.config.ts`、`vitest.config.ts`、`playwright.config.ts`
- Create: `index.html`、`src/main.tsx`、`src/styles/{tokens,global}.css`、`src/vite-env.d.ts`
- Create: `scripts/{sync,generate,check}-infinite-canvas-contract.mjs`
- Create: `contracts/backend/infinite-canvas.lock.json` and synced bundle files
- Generate: `src/modules/http/generated/**`
- Test: `tests/architecture/{contract-pipeline,repository-baseline}.test.ts`

- [ ] **Step 1: 锁定仓库基线并写失败门禁**

测试先断言：tracked bundle 缺失时 `contract:check` 失败；manifest commit 非 40 位小写 SHA、artifact hash 不符、OpenAPI 出现非 `/api/infinite-canvas/v1/**` path、permissions 出现非 `infinite_canvas_*` code 时失败；生成目录被手改时失败。`repository-baseline` 脚本额外拒绝 path `a` 进入 index。

Run: `git status --short -- a`

Expected: 唯一输出 ` D a`。

- [ ] **Step 2: 创建 React/Vite strict 工程**

`package.json` 固定 Node/npm engine，与 Admin 前端一致使用 Node 24/npm 11。安装 React、router、TanStack Query、Zustand、Ant Design、Lucide、`idb`、`cos-js-sdk-v5`、`nanoid`；开发依赖包含 TypeScript、Vite、ESLint、Prettier、Vitest、Testing Library、MSW、Playwright、axe-core、`openapi-typescript` 和覆盖率工具。所有版本进入 lockfile，不保留 `bun.lock` 或双包管理器。

正式 scripts 至少为：

```json
{
  "dev": "vite --host 0.0.0.0 --port 3000",
  "contract:sync": "node scripts/sync-infinite-canvas-contract.mjs",
  "contract:generate": "node scripts/generate-infinite-canvas-contract.mjs",
  "contract:check": "node scripts/check-infinite-canvas-contract.mjs",
  "surface:check": "node scripts/check-product-surface.mjs",
  "typecheck": "tsc -b",
  "lint": "eslint . --max-warnings 0",
  "format:check": "prettier --check .",
  "test": "vitest run",
  "build": "tsc -b && vite build",
  "verify": "npm run contract:check && npm run surface:check && npm run format:check && npm run lint && npm run test && npm run build",
  "test:e2e": "playwright test"
}
```

- [ ] **Step 3: 实现 Contract 同步、生成和 drift check**

`contract:sync -- --backend E:/admin/admin_back_go --commit <sha>` 必须读取 backend manifest，验证参数 commit 与 `backend_commit` 相同，逐个验证 manifest SHA-256，再原子替换本地三文件并写 lock。不能从工作树任意 OpenAPI 文件、URL 或 npm package 获取契约。

`contract:generate` 从固定 synced OpenAPI 生成 `schema.d.ts` 和 operation type helpers；HTTP runtime 用生成的 `paths/components/operations` 约束 method/path/input/output。`contract:check` 在临时目录重新生成并逐字比较，禁止未声明 path、手工修补 generated file 或备用 response interface。

- [ ] **Step 4: 创建应用最小入口和稳定主题基线**

应用首屏只能是 session boot 状态或实际 route，不显示营销 hero。定义浅/深主题 token、8px 以内控件圆角、44px 移动触控目标、稳定 toolbar/node 尺寸和响应式 breakpoints；字体大小不按 viewport 连续缩放，letter spacing 为 0。Vite dev proxy 可选，但生产 API origin 必须来自单一构建配置并通过绝对 URL/同源校验。

- [ ] **Step 5: 运行脚手架门禁并提交**

```powershell
npm run contract:check
npm run typecheck
npm test -- tests/architecture/contract-pipeline.test.ts tests/architecture/repository-baseline.test.ts
npm run build
git status --short -- a
```

Expected: 全部退出 0；最后仍只有 ` D a`。

```bash
git add .editorconfig .gitignore .prettierignore .prettierrc.json package.json package-lock.json tsconfig.json tsconfig.app.json tsconfig.node.json vite.config.ts vitest.config.ts playwright.config.ts index.html scripts contracts src/main.tsx src/styles src/vite-env.d.ts tests/architecture
git commit -m "chore(canvas): 初始化独立前端与契约流水线"
```

### Task 2: 实现内存会话、启动恢复和登录流程

**Files:**
- Create: `src/modules/http/{client,error,token-vault}.ts`
- Create: `src/features/auth/{api,store,session-bootstrap,login-page,login-form,password-reset-dialog}.tsx`
- Create: `src/app/{providers,router,protected-route}.tsx`
- Test: `tests/unit/auth/{token-vault,single-flight-refresh}.test.ts`
- Test: `tests/component/auth/login-page.test.tsx`
- Test: `tests/integration/auth/session-lifecycle.test.tsx`

- [ ] **Step 1: 先写会话状态机和 refresh 竞争失败测试**

覆盖 `booting -> anonymous|authenticated`、启动只 refresh 一次、20 个并发 401 只产生一次 refresh、等待请求用新 token 各重放一次、refresh 自身 401 不递归、失败清空状态并跳 `/login`、logout 即使网络失败也清本地状态。证明 localStorage/sessionStorage/IndexedDB 中都没有 access/refresh token。

- [ ] **Step 2: 实现内存 token vault 和类型化 HTTP client**

token vault 只保存在模块闭包，暴露 `get/set/clear/subscribe` 最小接口。所有 API 请求使用 generated client、`credentials: 'include'`、`Authorization: Bearer <memory token>`；统一解析正式 `{code,data,msg}` envelope 为 `ApiError{status,code,message,details}`，不把后端原始响应或 token 写日志。

401 middleware 只处理受保护业务请求：共享一个 `refreshPromise`，成功后重放原请求一次；login、verification、password reset、refresh 和 logout 不进入该循环。403 不 refresh，404/409 保留给业务层，5xx 只提供显式重试。

- [ ] **Step 3: 实现 session bootstrap、`/me` 和 route guard**

应用挂载先 `PUT /auth/session`，成功写内存 token 后读取 `/me`，二者完成前渲染固定尺寸初始化状态。受保护 route 只在 authenticated 时挂载；anonymous 进入根或受保护地址时跳 `/login` 并保留站内 return path，不能接受外部 redirect URL。

- [ ] **Step 4: 实现无注册入口的双方式登录**

登录页先读取 `/auth/login-config`，用 tabs 提供“邮箱验证码”和“账号密码”。验证码发送严格走 fresh captcha -> `/auth/verification-codes(scene=login)`；密码登录提交前也按 login config 完成独立 fresh captcha，再调用 `/auth/sessions`。邮箱 code session 不复用已经被发送请求消费的 captcha。服务端返回 `is_new_user=true` 时仍直接进入 `/projects`，不出现注册确认页。密码找回作为 modal/drawer，走 fresh captcha、`scene=forget` 和 `/auth/password-resets`，完成后回到密码登录。

页面覆盖 config loading/unavailable、captcha renew、发送倒计时、429、错误账号密码、平台注册关闭和邮件渠道不可用状态。表单与按钮不暴露 provider/channel 字样，不提供注册链接。

- [ ] **Step 5: 运行 auth 测试并提交**

```powershell
npm test -- tests/unit/auth tests/component/auth tests/integration/auth
npm run typecheck
git status --short -- a
```

```bash
git add src/modules/http src/features/auth src/app tests/unit/auth tests/component/auth tests/integration/auth
git commit -m "feat(canvas): 接入独立浏览器认证"
```

### Task 3: 实现项目 API、项目库和产品外壳

**Files:**
- Create: `src/shared/layout/{app-shell,primary-nav,mobile-nav,user-menu}.tsx`
- Create: `src/features/projects/{api,queries,project-list-page,project-card,project-actions,create-project-dialog}.tsx`
- Modify: `src/app/router.tsx`
- Test: `tests/unit/projects/project-api.test.ts`
- Test: `tests/component/projects/project-list-page.test.tsx`
- Test: `tests/integration/projects/project-mutations.test.tsx`

- [ ] **Step 1: 写 generated operation 与幂等行为失败测试**

断言 list 不依赖 document 字段；create 生成一次 `request_id` 并在网络结果不明重试时复用；rename/delete 带当前 `expected_revision`；copy 只使用 POST body 的 `source_project_id`，不存在 `/copy` action URL。400/404/409 分别进入确定 UI 状态。

- [ ] **Step 2: 实现项目 query/mutation facade**

所有输入输出类型从 generated operations 派生。TanStack Query key 含固定 platform domain 但不把 platform 发给后端；列表分页按服务端稳定顺序。mutation 成功只做精确 cache 更新/失效，失败不乐观提升 revision。

创建操作在 dialog 打开时生成 opaque request id，直到收到确定成功或确定 4xx 前不更换。复制沿用同一机制；重命名、删除如果 409，刷新 detail/list 后向用户展示已变化状态，不能覆盖服务端。

- [ ] **Step 3: 实现 `/projects` 和共享产品外壳**

桌面为紧凑顶部导航和可扫描项目网格/列表，移动端使用 drawer；导航只包含项目、素材、提示词、主题和用户退出。项目页提供 loading/skeleton、empty、error、分页、创建、打开、重命名、复制、删除确认。项目标题动态换行/截断且 action 不被挤出；不使用 section card 套 card。

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

### Task 4: 提取并收紧无限画布编辑器

**Files:**
- Create: `src/features/canvas/model/{document,node-registry,geometry,selection,serialization}.ts`
- Create: `src/features/canvas/store/{canvas-store,canvas-ui-store}.ts`
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

- [ ] **Step 4: 实现响应式 editor shell**

桌面侧栏不遮住画布工具，移动端将属性面板放入 drawer/bottom sheet；icon tool 使用 Lucide 并有 tooltip/accessible label。固定格式工具条使用稳定 grid/尺寸，节点文本和按钮不能溢出。无 AI route 时不渲染生成按钮、空 loading 或 disabled provider selector。

- [ ] **Step 5: 运行 canvas 测试并提交**

```powershell
npm test -- tests/unit/canvas tests/component/canvas tests/architecture/canvas-node-surface.test.ts
npm run typecheck
git status --short -- a
```

```bash
git add src/features/canvas tests/unit/canvas tests/component/canvas tests/architecture/canvas-node-surface.test.ts
git commit -m "feat(canvas): 提取严格无限画布编辑器"
```

### Task 5: 实现 IndexedDB 恢复草稿、串行自动保存和冲突处理

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

### Task 6: 接入私有 COS 素材和短期 URL resolver

**Files:**
- Create: `src/features/assets/{api,queries,asset-list-page,asset-picker,asset-editor,image-upload}.tsx`
- Create: `src/features/assets/upload/{sha256-worker,sha256-client,cos-uploader}.ts`
- Create: `src/features/assets/resolver/{asset-resolver,use-asset-object-url}.ts`
- Modify: image/config canvas nodes and `src/app/router.tsx`
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

resolver 以 `{asset_id,read_url,expires_at}` 缓存，提前 30 秒失效。画布展示将签名 URL fetch 为 blob URL；失败时按 asset id 重新 GET content/detail 并仅再尝试一次。blob URL 只存在 resolver，serializer 和 draft schema 均拒绝它。

- [ ] **Step 5: 运行素材测试并提交**

```powershell
npm test -- tests/unit/assets tests/component/assets tests/integration/assets
npm run typecheck
npm run build
```

```bash
git add src/features/assets src/features/canvas src/app/router.tsx tests/unit/assets tests/component/assets tests/integration/assets
git commit -m "feat(canvas): 接入私有图片素材库"
```

### Task 7: 接入只读提示词库并封闭产品功能面

**Files:**
- Create: `src/features/prompts/{api,queries,prompt-list-page,prompt-card,prompt-detail,prompt-insert}.tsx`
- Create: `scripts/check-product-surface.mjs`
- Modify: Canvas side panel、router、shared navigation and locale strings
- Test: `tests/unit/prompts/prompt-api.test.ts`
- Test: `tests/component/prompts/{prompt-list-page,prompt-insert}.test.tsx`
- Test: `tests/architecture/product-surface.test.ts`

- [ ] **Step 1: 先写提示词只读和禁用面失败测试**

断言前端只调用两条 GET prompt operations，不存在 prompt/source mutation。提示词内容始终 text binding，不使用 `dangerouslySetInnerHTML`。插入到 text node 使用 prompt 正文，插入到 config node 使用 prompt 字段；不会调用任何 generation route。

架构测试递归扫描 route、navigation、source 和依赖，拒绝产品 route `/config`、`/image`、`/video`、`/audio`、`/agent`、`/docs`，以及 WebDAV、channel、provider、api key/base URL、plugin manager、GitHub/release 外链、原内置 source code/URL 和第三方 AI host。`config` allowlist 只包含 Vite/TypeScript/Playwright 配置文件、generated `/auth/login-config` operation、Canvas node type 和“提示词配置”组件命名。

- [ ] **Step 2: 实现 `/prompts` 浏览与纯文本详情**

页面消费 generated list/detail operations，支持 keyword/category/tags/source 筛选、稳定分页、loading/empty/error。card 可显示已校验 HTTPS cover，但失败时使用中性占位，不代理或持久化远端图。详情 prompt/reference URL 只作为纯文本/经过 `https:` 校验的显式安全链接；不执行 HTML/Markdown script。

- [ ] **Step 3: 实现画布内插入工作流**

从提示词页可“在新画布使用”，先创建项目再插入 config/text node；编辑器 side panel 可把提示词插入当前选择或新节点。config 节点仍只编辑 prompt/素材引用，没有模型、质量、渠道或运行按钮。

- [ ] **Step 4: 删除所有旧产品入口并运行静态门禁**

`check-product-surface.mjs` 使用 TypeScript/JSON parser 和明确 allowlist 检查 dependencies、routes、nav、API base paths、source imports，不对 bundle 做模糊字符串误判。确认 reference 项目的以下文件族均未进入新 repo：`services/api/{image,audio,video,model-plugin}`、`webdav-sync`、`components/agent`、`plugin-*`、`config-prompt-sources`、`prompt-source-presets`、`seedance-video`。

- [ ] **Step 5: 运行提示词与产品面测试并提交**

```powershell
npm test -- tests/unit/prompts tests/component/prompts tests/architecture/product-surface.test.ts
npm run surface:check
npm run typecheck
git status --short -- a
```

```bash
git add src/features/prompts src/features/canvas src/shared src/app/router.tsx scripts/check-product-surface.mjs tests/unit/prompts tests/component/prompts tests/architecture/product-surface.test.ts
git commit -m "feat(canvas): 接入只读提示词工作流"
```

### Task 8: 完成前端质量、可访问性和视觉门禁

**Files:**
- Create: `tests/component/shared/{responsive-layout,accessibility}.test.tsx`
- Create: `e2e/{fixtures,mocked-shell,mocked-canvas}.spec.ts`
- Create: `docs/frontend-architecture.md`
- Modify: styles/components discovered by tests

- [ ] **Step 1: 补齐 reducer/store 边界和覆盖率**

Vitest 覆盖 auth、projects、document/serializer、autosave、draft、upload、resolver、prompts 的 success/empty/error/conflict。给核心纯逻辑设 branch/line/function 90% 门槛，展示组件整体门槛 80%；不为追覆盖率测试第三方 UI 内部实现。

- [ ] **Step 2: 运行 axe 与键盘工作流**

覆盖 login、projects、canvas toolbar/node selection/dialog、assets upload/picker、prompts detail。焦点 trap、返回焦点、Escape、Tab 顺序、图标按钮 accessible name、错误关联、颜色对比和 reduced motion 必须通过。

- [ ] **Step 3: 用 Playwright 做 mocked 视觉基线**

在 Chromium 1440x900、1280x720、390x844、360x800 截图检查 `/login`、`/projects`、`/canvas/1`、`/assets`、`/prompts`。同时读取 bounding boxes 和 canvas root pixels，断言画布非空、工具条/节点/抽屉不重叠、最长中文/英文文本不溢出、移动端主要操作可点击。截图只用于视觉基线；真实后端验收归 Plan 07。

- [ ] **Step 4: 记录前端不变量并运行总门禁**

`docs/frontend-architecture.md` 记录 Contract 真相、内存 token、single-flight refresh、server canonical project、draft key/version、autosave state machine、asset resolver 和禁止产品面。不得写用户教程式 in-app 文案。

```powershell
npm run verify
npm run test:e2e -- e2e/mocked-shell.spec.ts e2e/mocked-canvas.spec.ts
git diff --check
git status --short -- a
```

Expected: 所有命令退出 0；桌面/移动截图无空白、重叠或溢出；最后仍精确保留用户的 ` D a`。

```bash
git add docs/frontend-architecture.md tests/component/shared e2e src package.json package-lock.json
git commit -m "test(canvas): 锁定前端质量与视觉基线"
```

## 完成标准

- `/login`、`/projects`、`/canvas/:projectId`、`/assets`、`/prompts` 全部消费正式 generated client，无 `/register` 或 Admin fallback。
- access token 仅在内存，启动 refresh 和并发 401 single-flight 行为通过确定测试。
- 项目服务端是唯一同步真相；1 秒串行 autosave、IndexedDB 恢复、结果不明读取和 409 两种处理完整。
- document 只含四类节点和 asset id；任何临时/签名 URL、COS key、渠道或 provider 都不持久化。
- JPEG/PNG/WebP 通过 SHA-256、STS、COS 直传和 intent 确认进入素材库；签名 URL 有刷新上限并释放 blob URL。
- 提示词只读浏览/插入完成，用户不能管理来源，也不能触发 AI 生成。
- strict typecheck、format、lint、Vitest、production build、桌面/移动 Playwright 和产品面静态门禁全部通过。
- tracked path `a` 仍保持用户原有删除状态，未进入任何提交。
