# Infinite Canvas Auth 与独立 Contract Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 注册可信 `infinite_canvas` runtime adapter，交付邮箱验证码登录即注册、账号密码登录、找回密码、会话恢复、退出、`/me` 和独立 Infinite Canvas Contract Bundle，并证明它与 Admin 全面隔离。

**Architecture:** 全局只保留 recovery/request-id/log/CORS/i18n，Admin 和 Canvas 各自在独立 Gin route group 上装配固定 platform 的 AuthToken、PermissionCheck 和 OperationLog。共享 auth capability 通过平台化 membership repository 工作，但 transport 不接受客户端 platform；验证码 key、Origin allowlist、refresh Cookie、session 和 route registry 都独立。Canvas graph 是编译期最小组合入口，后续计划向其中追加项目、素材和提示词 capability。

**Tech Stack:** Go、Gin、JWT + opaque refresh session、Redis、GORM、OpenAPI 3.1、Contract Bundle、PowerShell generation scripts。

---

## 执行边界

> **并行与提交覆盖规则：** 实施时同时遵守 `E:\admin\LONG_TASK_PARALLEL_EXECUTION.md` 和 execution index。子执行器只修改分配给自己的文件并返回 diff/测试证据，不运行 `git add`、`git commit`、merge 或 rebase；下文所有“提交”步骤均为主线程审查后的集成检查点。首轮只并发：Task 1 auth capability、Task 2 的 `internal/config/**` + `internal/middleware/cors.go`、Task 5 Step 2 contractbundle core。`internal/server/**`、`internal/platform/**`、runtime composition、Canvas contract package/CLI/artifacts 只归主线程；三条 lane 返回后再串行完成 Tasks 3-6 的依赖部分。

- 依赖 Plan 02 已完成：principal 只读 `user_platform_roles`，通用包名为 `internal/server/routepolicy`。
- 本 Plan 结束时 `enum.RegisteredPlatforms()` 才从 `[admin]` 变为 `[admin,infinite_canvas]`。
- Canvas transport 不导入 Admin transport，也不调用 Admin presenter；共享逻辑留在 `internal/module/auth` 和 `internal/module/user`。
- API 路径严格为 `/api/infinite-canvas/v1`；不存在 `/register`、`/api/canvas` 或 `/api/app` alias。
- Admin origin 继续来自 `CORS_ALLOW_ORIGINS`；Canvas origin 来自新变量 `INFINITE_CANVAS_ALLOW_ORIGINS`。全局 CORS 使用两者并集，但各 auth handler 只接受自己的集合。
- access token 只通过 JSON 返回；refresh token 只通过 Canvas HttpOnly Cookie 返回。

## 文件结构

**Create:**

- `internal/platform/infinitecanvas/graph.go`、`graph_test.go`：Canvas 首期 capability graph。
- `internal/platform/infinitecanvas/build.go`、`build_test.go`：独立 auth/principal/user composition。
- `internal/module/auth/transport/infinitecanvas/{route,handler,request,presenter}.go` 及测试。
- `internal/module/user/transport/infinitecanvas/{route,handler,presenter}.go` 及测试。
- `internal/infinitecanvascontract/{bundle,manifest,openapi,permissions}.go` 及测试。
- `cmd/infinite-canvas-contract/{main,main_test}.go`。
- `scripts/generate-infinite-canvas-contract.ps1`、`scripts/check-infinite-canvas-contract.ps1`。
- `contracts/infinite-canvas/v1/{openapi,permissions,manifest}.json`：生成物。
- `internal/server/infinite_canvas_route_registry_integration_test.go`。

**Modify:**

- `internal/config/{config,runtime,snapshot}.go` 及测试：Canvas origins。
- `internal/middleware/cors.go` 及测试：使用去重后的 origin 并集。
- `internal/shared/enum/platform.go` 及测试：激活 compiled platform。
- `internal/module/auth/{model,dto,repository,service,verifycode/key,session_contract,session_lifecycle}.go` 及测试。
- `internal/module/user/{dto,repository,service}.go`：platform-current-user projection。
- `internal/server/router.go`、各 `routes_*.go`、测试/golden。
- `internal/runtime/api.go`、`internal/platform/admin/build.go` 及测试：组装两个 graph 和两套 middleware dependencies。
- `internal/contractbundle/**`：从 Admin bundle 提取确定性文件写入/check/hash 小核心。
- `internal/admincontract/{bundle,manifest}.go`：改用 contractbundle，但 Admin artifacts 不减少。
- `cmd/admin-api/main.go`、`cmd/admin-contract/main.go`。
- `docs/architecture.md`：从 Admin-only adapter 更新为双 trusted adapter。

### Task 1: 平台化验证码和 auth membership 事务

**Files:**
- Modify: `internal/module/auth/{model,dto,repository,service}.go`
- Modify: `internal/module/auth/verifycode/key.go`
- Modify: `internal/module/user/service.go`
- Test: `internal/module/auth/{service,code_store,verify_code_policy}_test.go`

- [ ] **Step 1: 写登录即注册和跨平台验证码失败测试**

至少覆盖：

```go
func TestCanvasCodeLoginCreatesIdentityAndOnlyCanvasMembership(t *testing.T) {
    service, repo := newAuthFixture(authPlatformPolicy{
        Platform: enum.PlatformInfiniteCanvas, AllowRegister: true,
        LoginTypes: []string{auth.LoginTypeEmail, auth.LoginTypePassword},
    })
    repo.defaultRoles[enum.PlatformInfiniteCanvas] = 22
    result, appErr := service.Login(ctx, auth.LoginInput{
        Platform: enum.PlatformInfiniteCanvas, LoginType: auth.LoginTypeEmail,
        LoginAccount: "new@example.com", Code: "123456",
    })
    require.Nil(t, appErr)
    require.True(t, result.IsNewUser)
    require.Equal(t, []auth.PlatformMembership{{
        UserID: result.UserID, Platform: enum.PlatformInfiniteCanvas, RoleID: 22,
    }}, repo.memberships)
    require.Zero(t, repo.adminMembershipWrites)
}
```

再测试：未知账号密码不创建；既有用户首次 Canvas 密码登录只补 Canvas binding；`allow_register=2` 时缺 binding 返回 `auth.platform_membership.required`；缺默认 role fail closed；Admin code 不能用于 Canvas login/forget；Canvas code 不能用于 Admin。

- [ ] **Step 2: 把 verify code key 升级为平台隔离的 v2**

```go
const defaultRedisPrefix = "auth:verify_code:v2:"

func CacheKey(platform, accountType, scene, account string) string {
    sum := sha256.Sum256([]byte(strings.TrimSpace(account)))
    return defaultRedisPrefix + strings.TrimSpace(platform) + ":" +
        accountType + ":" + strings.TrimSpace(scene) + ":" + hex.EncodeToString(sum[:])
}
```

`SendCodeInput` 和 `ForgetPasswordInput` 增加 `Platform string`；`Service.SendCode` 对 login/forget scene 调用 `allowedLoginTypes` 并验证该平台允许 email code。`verifyCodeCacheKey`、delivery lease、consume 全部携带 platform。旧无平台 key 不读取、不迁移、不 fallback。

Admin transport 和 `user.Service` 的 bind-email/bind-phone verification 固定传 `enum.PlatformAdmin`；Canvas transport 固定传 `enum.PlatformInfiniteCanvas`。

- [ ] **Step 3: 定义 auth repository 的 platform membership 接口**

```go
type PlatformMembership struct {
    UserID   int64
    Platform string
    RoleID   int64
}

type CreateUserInput struct {
    Username string
    Email    *string
    Phone    *string
}

type Repository interface {
    WithTx(context.Context, func(Repository) error) error
    FindCredentialByEmail(context.Context, string) (*UserCredential, error)
    FindCredentialByPhone(context.Context, string) (*UserCredential, error)
    FindCredentialByID(context.Context, int64) (*UserCredential, error)
    FindDefaultRole(context.Context, string) (*DefaultRole, error)
    FindPlatformMembership(context.Context, int64, string) (*PlatformMembership, error)
    CreateUser(context.Context, CreateUserInput) (int64, error)
    CreateProfile(context.Context, CreateProfileInput) error
    BindPlatformRole(context.Context, PlatformMembership) error
    UpdatePassword(context.Context, int64, string) error
    RecordLoginAttempt(context.Context, LoginAttempt) error
}
```

`CreateUser` 不写 `role_id`。`FindDefaultRole` 条件为 `platform=? AND is_default=1 AND is_del=2`；binding insert 依赖数据库组合 FK。

- [ ] **Step 4: 实现认证后 membership 决策**

登录流程固定顺序：验证 platform/login type/captcha -> 验证密码或消费 code -> 验证 user active -> 在事务中读取/创建 membership -> Issue session。密码凭据错误时绝不查询默认 role 或写 binding。

```go
func (s *Service) ensurePlatformMembership(ctx context.Context, userID int64, platform string) *apperror.Error {
    membership, err := s.repository.FindPlatformMembership(ctx, userID, platform)
    if err != nil { return membershipQueryError(err) }
    if membership != nil { return nil }
    allowed, appErr := s.registerAllowed(ctx, platform)
    if appErr != nil { return appErr }
    if !allowed { return apperror.ForbiddenKey("auth.platform_membership.required", nil, "当前账号尚未加入该平台") }
    return s.bindDefaultPlatformRole(ctx, userID, platform)
}
```

新用户创建、profile 创建和 Canvas binding 必须在同一 DB transaction。duplicate email race 重新读取 identity 后仍必须执行 membership transaction，不能把另一个请求创建的用户当完成态直接签 session。

- [ ] **Step 5: 在 session refresh/authenticate 时检查 membership 仍存在**

在 `auth` 定义窄接口：

```go
type MembershipChecker interface {
    RequireActiveMembership(context.Context, int64, string) *apperror.Error
}
```

`LifecycleDeps` 增加 checker；`Authenticate` 和 `Rotate` 在 session/platform/user active 校验后调用它。`permission.PrincipalService.RequireActiveMembership` 用当前 snapshot 实现；缺 binding、软删除 role 或全局禁用返回 unauthorized/forbidden，且 Rotate 不签发新 token。

- [ ] **Step 6: 运行 auth 定向测试**

```powershell
go test ./internal/module/auth -run 'Canvas|PlatformMembership|VerifyCode|PasswordLogin' -count=1
go test ./internal/module/user -run 'VerifyCode|Init|Profile' -count=1
```

Expected: PASS；Admin 既有登录测试仍通过，验证码 key 包含平台且没有 legacy fallback。

- [ ] **Step 7: 提交 auth core**

```bash
git add internal/module/auth internal/module/user internal/module/permission
git commit -m "feat(auth): 隔离平台登录与成员资格"
```

### Task 2: 配置独立 Origin 并按 trusted route group 组装 middleware

**Files:**
- Modify: `internal/config/{config,runtime,snapshot}.go`
- Modify: `internal/middleware/cors.go`
- Modify: `internal/server/router.go`
- Modify: `internal/server/routes_*.go`
- Test: `internal/config/*_test.go`
- Test: `internal/server/{router,dependencies}_test.go`

- [ ] **Step 1: 写 Origin 隔离和 route group 失败测试**

测试分别用 `https://admin.example.com` 和 `https://canvas.example.com`：全局 CORS preflight 都允许；Admin login 只接受前者；Canvas session 只接受后者。注册同名 header/body `platform=admin` 请求 Canvas route，fake authenticator 收到的仍必须是 `infinite_canvas`。

- [ ] **Step 2: 添加配置结构和 env**

```go
type InfiniteCanvasConfig struct {
    AllowOrigins []string
}

type Config struct {
    // existing fields
    InfiniteCanvas InfiniteCanvasConfig
}
```

开发默认 origins 精确为 `http://localhost:3000`、`http://127.0.0.1:3000`；env 为 `INFINITE_CANVAS_ALLOW_ORIGINS`。生产校验复用现有绝对 HTTPS/public host 规则。`config.Snapshot` 深拷贝两个 origin slices；全局 CORS 传 `UnionOrigins(cfg.CORS.AllowOrigins,cfg.InfiniteCanvas.AllowOrigins)` 并保持 credentials enabled。

- [ ] **Step 3: 定义 platform route dependencies**

```go
type RouteGroupDependencies struct {
    Platform          string
    Registry          *routepolicy.Registry
    Authenticator     middleware.TokenAuthenticator
    PermissionChecker middleware.PermissionChecker
    OperationRecorder middleware.OperationRecorder
}

type Dependencies struct {
    Core           CoreDependencies
    Admin          platformadmin.Graph
    InfiniteCanvas platforminfinitecanvas.Graph
    AdminHTTP      RouteGroupDependencies
    CanvasHTTP     RouteGroupDependencies
}
```

`CoreDependencies` 只保留 Readiness/Logger/Telemetry/CORS/QueueMonitorUI/RealtimeHandler，不再保存 Admin authenticator 或 registry。

- [ ] **Step 4: 把 middleware 装到两个独立 Gin groups**

```go
adminGroup := router.Group("/")
adminGroup.Use(productMiddleware(deps.AdminHTTP, adminBrowserGrants(deps.Admin)))
registerAdminRoutes(adminGroup, deps)

canvasGroup := router.Group("/")
canvasGroup.Use(productMiddleware(deps.CanvasHTTP, middleware.BrowserGrantAuthConfig{}))
registerInfiniteCanvasRoutes(canvasGroup, deps)
```

所有 transport `Register` 参数从 `*gin.Engine` 放宽为 `gin.IRoutes`，但保留绝对正式 path。每个 registry 只用本平台实际 route 编译：Admin 接受 `/health`、`/ready`、`/api/admin/**`、payment callbacks；Canvas 只接受 `/api/infinite-canvas/**`。未知/重复/未定义 route 继续导致启动失败。

- [ ] **Step 5: 激活 compiled registry**

`registeredPlatforms` 改为 `[PlatformAdmin, PlatformInfiniteCanvas]`；notification audience 和 session Admin 字典因此显示两平台。测试确认 `app/canvas` 仍被拒绝，数据库行无法新增第三个平台。

- [ ] **Step 6: 运行 config/middleware/router 测试**

```powershell
go test ./internal/config ./internal/middleware ./internal/server ./internal/shared/enum -count=1
```

Expected: PASS；两个 fake authenticator 只收到自己编译期 platform。

- [ ] **Step 7: 提交 route group 基础**

```bash
git add internal/config internal/middleware internal/server internal/shared/enum
git commit -m "feat(server): 注册无限画布可信路由组"
```

### Task 3: 构造 Infinite Canvas graph 和 `/me`

**Files:**
- Create: `internal/platform/infinitecanvas/{graph,graph_test,build,build_test}.go`
- Create: `internal/module/user/transport/infinitecanvas/{route,handler,presenter,handler_test}.go`
- Modify: `internal/module/user/{dto,repository,service}.go`
- Modify: `internal/runtime/api.go`
- Modify: `internal/platform/admin/build.go`
- Test: `internal/runtime/api_test.go`

- [ ] **Step 1: 写 graph 必需 capability 测试**

```go
type IdentityGraph struct {
    Auth        auth.SessionService
    Captcha     auth.CaptchaHTTPService
    CurrentUser *user.Service
}
type Graph struct { Identity IdentityGraph }
```

`Validate()` 对三个 nil capability 分别返回 `infinite canvas capability identity.auth/captcha/current_user is required`。Build 不组装 Admin session manager、browser grants、AI、payment、notifications 或 queue monitor。

- [ ] **Step 2: 实现独立 Build**

`BuildInput` 只接收 config/resources/keyring/secretbox/mail/sms/logger/telemetry/queue。构造自己的 auth platform service、session lifecycle、captcha、auth service、principal service 和 current-user service；所有 Redis prefix 复用系统 namespace 但 key 已含 platform/session id。返回：

```go
type BuildResult struct {
    Graph             Graph
    Authenticator     middleware.TokenAuthenticator
    PermissionChecker middleware.PermissionChecker
    OperationRecorder middleware.OperationRecorder
}
```

`runtime/api.go` 先创建共享 resources/providers，再分别 Build Admin 和 Infinite Canvas，并将两套 graph/result 交给 router。不得从 Admin BuildResult 拿 Auth service 塞入 Canvas graph。

- [ ] **Step 3: 添加 platform-current-user projection**

```go
type CurrentPlatformUser struct {
    UserID      int64    `json:"user_id"`
    Username    string   `json:"username"`
    Email       string   `json:"email"`
    Avatar      string   `json:"avatar"`
    RoleID      int64    `json:"role_id"`
    RoleName    string   `json:"role_name"`
    Permissions []string `json:"permissions"`
}
```

repository query join `users + user_profiles + user_platform_roles + roles`，必须带 `user_id + platform + active flags`；service 从 principal snapshot 取 route/button codes，排序去重后返回。Canvas presenter 不返回 Admin menu/router/buttonCodes 结构。

- [ ] **Step 4: 注册 `/me`**

```go
routes.Handle(routepolicy.Definition{
    Method: http.MethodGet,
    Path: "/api/infinite-canvas/v1/me",
    OperationID: "get_api_infinite_canvas_v1_me",
    Access: routepolicy.Permission("infinite_canvas_workspace"),
    Audit: routepolicy.NoAudit("read-only"),
    Tags: []string{"Identity"},
    Contract: &routepolicy.HTTPContract{Response: user.CurrentPlatformUser{}},
}, handler.Me)
```

handler 只从 middleware identity 获取 user ID，调用 service 时固定 `enum.PlatformInfiniteCanvas`。

- [ ] **Step 5: 运行 graph/me/runtime 测试**

```powershell
go test ./internal/platform/infinitecanvas ./internal/module/user/transport/infinitecanvas ./internal/runtime -run 'InfiniteCanvas|CurrentPlatformUser|APIRouter' -count=1
```

Expected: PASS；Canvas graph 不含 Admin-only capability。

- [ ] **Step 6: 提交 graph 和 me**

```bash
git add internal/platform/infinitecanvas internal/module/user internal/runtime internal/server
git commit -m "feat(canvas): 组装平台身份图"
```

### Task 4: 实现 Canvas REST Auth transport 和 Cookie 隔离

**Files:**
- Create: `internal/module/auth/transport/infinitecanvas/{route,handler,request,presenter,handler_test}.go`
- Modify: `internal/server/router.go`
- Test: `internal/server/router_test.go`

- [ ] **Step 1: 写路由、Origin、空 body 和 Cookie 失败测试**

测试精确覆盖七条 route、无 `/register`；Canvas Cookie 名/path/Secure/SameSite；loopback HTTP dev 名；读取 dev Cookie 时最多回退 Canvas production cookie，永不读取 `admin_refresh_dev` 或 `__Secure-admin_refresh`。

- [ ] **Step 2: 定义 REST request/response**

```go
type SessionRequest struct {
    LoginAccount  string                `json:"login_account" binding:"required,max=120"`
    LoginType     string                `json:"login_type" binding:"required,auth_platform_login_type"`
    Password      string                `json:"password" binding:"omitempty,max=128"`
    Code          string                `json:"code" binding:"omitempty,len=6,numeric"`
    CaptchaID     string                `json:"captcha_id" binding:"omitempty,max=80"`
    CaptchaAnswer *captchaAnswerRequest `json:"captcha_answer"`
}
type VerificationCodeRequest struct {
    Account       string                `json:"account" binding:"required,email,max=120"`
    Scene         string                `json:"scene" binding:"required,oneof=login forget"`
    CaptchaID     string                `json:"captcha_id" binding:"required,max=80"`
    CaptchaAnswer *captchaAnswerRequest `json:"captcha_answer" binding:"required"`
}
type PasswordResetRequest struct {
    Account         string `json:"account" binding:"required,email,max=120"`
    Code            string `json:"code" binding:"required,len=6,numeric"`
    NewPassword     string `json:"new_password" binding:"required,min=6,max=128"`
    ConfirmPassword string `json:"confirm_password" binding:"required,min=6,max=128"`
}
type SessionResponse struct {
    AccessToken string `json:"access_token"`
    ExpiresIn   int64  `json:"expires_in"`
    IsNewUser   bool   `json:"is_new_user"`
}
type RefreshResponse struct {
    AccessToken string `json:"access_token"`
    ExpiresIn   int64  `json:"expires_in"`
}
```

- [ ] **Step 3: 注册七条正式 route**

```text
GET    /api/infinite-canvas/v1/auth/login-config       public
GET    /api/infinite-canvas/v1/auth/captcha            public
POST   /api/infinite-canvas/v1/auth/verification-codes public
POST   /api/infinite-canvas/v1/auth/sessions           public
PUT    /api/infinite-canvas/v1/auth/session            public refresh cookie
DELETE /api/infinite-canvas/v1/auth/session            authenticated bearer
POST   /api/infinite-canvas/v1/auth/password-resets    public
```

所有 mutation `NoAudit` 原因明确为 authentication/session domain audit；operation ID 使用 method + full path 的现有生成规范。

- [ ] **Step 4: 实现独立 refresh Cookie**

```go
const (
    BrowserRefreshCookieName = "__Secure-infinite_canvas_refresh"
    browserRefreshCookieNameHTTP = "infinite_canvas_refresh_dev"
    refreshCookiePath = "/api/infinite-canvas/v1/auth"
)
```

生产 Cookie `HttpOnly=true, Secure=true, SameSite=Strict`；本地 HTTP loopback 只改名字和 Secure=false，path 不变。Refresh/Logout 要求可信 Canvas Origin 和严格空 body。Logout 只按 bearer session 撤销，不批量撤销同 user 的 Admin session。

- [ ] **Step 5: 运行 transport/router 测试**

```powershell
go test ./internal/module/auth/transport/infinitecanvas ./internal/server -run 'InfiniteCanvas|CanvasAuth|CrossPlatform' -count=1
```

Expected: PASS；Admin token/Cookie/header 无法通过 Canvas，反向同样失败。

- [ ] **Step 6: 提交 Canvas auth transport**

```bash
git add internal/module/auth/transport/infinitecanvas internal/server
git commit -m "feat(canvas): 接入独立浏览器认证"
```

### Task 5: 发布独立 Infinite Canvas Contract Bundle

**Files:**
- Create: `internal/contractbundle/{files,files_test}.go`
- Modify: `internal/admincontract/{bundle,manifest}.go`
- Create: `internal/infinitecanvascontract/**`
- Create: `cmd/infinite-canvas-contract/**`
- Create: `scripts/{generate,check}-infinite-canvas-contract.ps1`
- Create: `contracts/infinite-canvas/v1/**`

- [ ] **Step 1: 先写 bundle 文件集和路由过滤失败测试**

Canvas bundle 文件集必须精确为：

```text
openapi.json
permissions.json
manifest.json
```

OpenAPI 只能包含 `/api/infinite-canvas/v1/**`，不得含 Admin/payment/health/ready；permissions 只能含六个 `infinite_canvas_*` code。manifest 必须为每个 artifact 保存 SHA-256 和 schema version，并绑定显式 40 位 backend commit。

- [ ] **Step 2: 提取通用原子写入/check 核心**

把 Admin `WriteAtomic/Check/normalizedOutputPath/writeBundleFile/sha256Hex` 移到 `internal/contractbundle`，公开最小类型：

```go
type Files map[string][]byte
func WriteAtomic(output string, files Files) error
func Check(output string, files Files) error
func SHA256(data []byte) string
```

Admin bundle 改用该包，生成字节必须保持不变；先运行 `go test ./internal/admincontract -count=1` 证明无 drift。

- [ ] **Step 3: 实现 Canvas OpenAPI/permissions/manifest**

`infinitecanvascontract.Build(BuildOptions{BackendCommit})` 通过 `bootstrap.InfiniteCanvasRouteRegistry()` 构造真实 Router 收集 compiled definitions。manifest 常量固定：

```go
const (
    BundleVersion = "infinite-canvas-2026-07-31.1"
    OpenAPIVersion = "3.1.0"
    PermissionSchemaVersion = "infinite-canvas-permissions-2026-07-31.1"
)
```

OpenAPI response envelope 与 Admin 共用正式 `{code,data,msg}` schema 规则，但 DTO 只来自 Canvas routes。任何 route 缺 operation ID 或 HTTPContract 都使生成失败。

- [ ] **Step 4: 实现 CLI 和 PowerShell scripts**

CLI 语法：

```text
infinite-canvas-contract <generate|check> -out contracts/infinite-canvas/v1 -commit <40-char-sha>
```

scripts 与 Admin 一样：默认生成要求 clean committed checkout；check 默认读取 manifest backend_commit；任何 dirty/default commit 猜测都失败。

- [ ] **Step 5: 主线程先提交 Contract runtime，再从 clean HEAD 生成 bundle**

先运行 generator/contract 定向测试，再显式提交非生成物：

```powershell
go test ./internal/contractbundle ./internal/infinitecanvascontract ./cmd/infinite-canvas-contract ./internal/admincontract -count=1
git diff --check
```

```bash
git add internal/contractbundle internal/infinitecanvascontract internal/admincontract cmd/infinite-canvas-contract scripts/generate-infinite-canvas-contract.ps1 scripts/check-infinite-canvas-contract.ps1
git commit -m "feat(contract): 建立无限画布契约生成器"
```

确认 backend clean 后才读取 SHA 和生成 artifacts：

```powershell
$status = git status --porcelain --untracked-files=all
if ($status) { throw "backend must be clean before contract generation" }
$commit = (git rev-parse HEAD).Trim()
pwsh -NoProfile -File scripts/generate-infinite-canvas-contract.ps1 -BackendCommit $commit
go test ./internal/infinitecanvascontract ./cmd/infinite-canvas-contract ./internal/admincontract -count=1
pwsh -NoProfile -File scripts/check-infinite-canvas-contract.ps1 -BackendCommit $commit
```

Expected: 全部退出 0；Canvas bundle 只有三文件且 SHA 正确，Admin bundle artifacts 不减少。

- [ ] **Step 6: 单独提交 Contract artifacts**

```bash
git add contracts/infinite-canvas/v1
git commit -m "feat(contract): 发布无限画布认证契约"
```

### Task 6: 完成双平台隔离集成测试和架构文档

**Files:**
- Create: `internal/server/infinite_canvas_route_registry_integration_test.go`
- Modify: `internal/server/testdata/admin_route_policy_golden.json`
- Modify: `internal/server/testdata/admin_routes_golden.txt`
- Modify: `docs/architecture.md`
- Test: `internal/server/router_test.go`

- [ ] **Step 1: 加入完整隔离矩阵测试**

矩阵至少包含：Admin access token -> Canvas `/me` 401；Canvas token -> Admin `/users/me` 401；两平台各自 refresh 成功；把 Canvas refresh Cookie 发给 Admin refresh 得 401；Canvas logout 后 Admin session 仍可用；同 user 两次登录日志分别记录准确 platform；客户端 header/body platform 被忽略。

- [ ] **Step 2: 更新 Admin golden 且新增 Canvas route golden**

Admin golden 只因 `routepolicy` schema 名和 RBAC DTO 合法变化；Canvas 新建 `internal/server/testdata/infinite_canvas_routes_golden.txt`，当前精确包含七条 auth route 和 `/me`。不把未来项目/素材/prompt route 预写进 golden。

- [ ] **Step 3: 更新架构文档**

记录双 adapter、route group middleware 顺序、manageable vs registered platform、验证码 v2 key、Origin/Cookie 隔离和双 Contract Bundle。删除“Admin 是唯一 registered runtime adapter”的过时陈述，但保留退役 app/canvas 不可恢复规则。

- [ ] **Step 4: 运行本 Plan 最终门禁**

```powershell
go test ./internal/module/auth ./internal/module/user ./internal/module/permission ./internal/platform/admin ./internal/platform/infinitecanvas ./internal/server ./internal/admincontract ./internal/infinitecanvascontract ./internal/runtime -count=1
go test ./internal/architecture -run 'TestPlatformRBACRuntime|TestRoutePolicyCoreIsPlatformNeutral' -count=1
git diff --check
```

Expected: 全部退出 0。

- [ ] **Step 5: 提交隔离证明**

```bash
git add internal/server docs/architecture.md
git commit -m "test(canvas): 证明双平台认证隔离"
```

## 完成标准

- Canvas 提供邮箱验证码、账号密码、刷新、退出和找回密码，没有注册 route/page contract。
- 新邮箱登录只创建共享 user/profile + Canvas binding；既有用户首次进入只补 Canvas binding；未知密码账号不创建。
- verification code、Origin、token、refresh Cookie、session、logout、登录日志和 principal 均按平台隔离。
- Router 由两个可信 group 组装，客户端不能选择 platform。
- Infinite Canvas Bundle 只有 OpenAPI、permissions、manifest，并可确定性重建；Admin Bundle 继续完整通过检查。
