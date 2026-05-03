# admin_back_go Architecture

本仓库采用 `Gin modular monolith`。

完整架构规则见：

```text
E:\admin_go\docs\architecture\04-go-backend-framework.md
E:\admin_go\docs\architecture\05-development-quality-rules.md
```

## 当前阶段

当前阶段只允许搭建架构骨架。

已允许：

```text
cmd/admin-api
internal/bootstrap
internal/config
internal/server
internal/response
internal/version
internal/module 文档边界
internal/platform 文档边界
internal/platform/database 连接边界
internal/platform/redisclient 连接边界
internal/readiness 运行期探针结果
internal/module/session token/session 认证读取边界
internal/module/authplatform 平台认证策略读取边界
internal/module/captcha go-captcha slide 验证码边界
internal/module/auth password login/refresh/logout HTTP 边界
internal/module/permission RBAC read context 计算边界
internal/module/user Users/init legacy-compatible read adapter
internal/middleware/PermissionCheck 显式 route metadata 边界
internal/middleware/OperationLog 显式 route metadata 边界
```

未允许：

```text
批量迁移所有 RBAC 写路径
短信/邮箱验证码登录迁移
AI 应用接入
队列和定时任务
```

## 固定调用链

```text
route -> handler -> service -> repository -> model
```

没有真实职责的层不要硬建。

## 第一批模块

```text
system
session
auth
captcha
user
permission
role
operationlog
```

`system` 用来证明架构能跑。RBAC 从 `auth/session/permission/role/user` 开始迁移。

## Gin core usage

Gin 是本仓库 HTTP 核心，不再额外包一层自造框架。

当前只采用 Gin 的基础能力：

```text
router := gin.New()
router.Use(...middleware)
router.Group("/api/admin/v1")
module.RegisterRoutes(router)
c.JSON(...)
```

不要把 Gin 藏进复杂 adapter，也不要让业务 service 依赖 `gin.Context`。

## Middleware baseline

全局 middleware 由 `internal/server/router.go` 按顺序装配。

当前顺序：

```text
Recovery
RequestID
AccessLog
CORS
AuthToken
PermissionCheck
OperationLog
module routes
```

middleware 必须一个一个加，并且必须有测试：

```text
AccessLog
CORS
AuthToken
PermissionCheck
OperationLog
```

`PermissionCheck` / `OperationLog` 禁止注解、反射、handler 名字猜测。Go 里用显式 route metadata；没有 metadata 就不假装有权限规则。

## Access log baseline

`AccessLog` 是 HTTP 横切层，不是业务日志。

当前记录字段：

```text
request_id
method
path
status
latency_ms
client_ip
```

规则：

```text
不记录 request body
不记录 response body
不记录完整 query string
不在 handler/service 里手写访问日志
```

后续登录、权限、业务操作的审计日志属于 `OperationLog`，不要塞进 AccessLog。

## CORS baseline

CORS 使用 Gin 生态组件：

```text
github.com/gin-contrib/cors
```

不要手写一堆 `Access-Control-*` header。CORS 是浏览器边界，不是业务权限。

当前默认只放本地 Vite 前端开发源：

```text
http://localhost:5173
http://127.0.0.1:5173
http://localhost:5174
http://127.0.0.1:5174
```

允许的前端公共请求头来自当前 `admin_front_ts`：

```text
Content-Type
Authorization
platform
device-id
X-Trace-Id
X-Request-Id
```

可配置环境变量：

```text
CORS_ALLOW_ORIGINS
CORS_ALLOW_HEADERS
CORS_ALLOW_CREDENTIALS
CORS_MAX_AGE
```

规则：

```text
生产域名必须显式配置 CORS_ALLOW_ORIGINS
不使用 AllowAllOrigins
不把 CORS 当鉴权
遇到浏览器 CORS 报错先确认真实路由和状态码，不要盲改 middleware
```

## AuthToken baseline

`AuthToken` 当前只是认证边界，不承载登录业务。

它只负责：

```text
跳过 public path：/health /ready /api/admin/v1/ping /api/admin/v1/auth/login-config /api/admin/v1/auth/captcha /api/admin/v1/auth/login /api/admin/v1/auth/refresh
解析 Authorization: Bearer <token>
把 token/platform/device-id/client-ip 交给注入的 authenticator
把 authenticator 返回的 session identity 挂到 Gin context
认证失败时返回统一 response
```

它不负责：

```text
生成 token
hash token
查 Redis/DB session
判断平台策略
判断单端登录
判断 RBAC 按钮权限
校验验证码
```

旧系统 `CheckToken` 的业务事实要保留：

```text
前端通过 Authorization: Bearer <token> 传 access token
platform/device-id 作为请求输入传入认证服务
最终可信 platform 来自 session identity，不盲信 header
```

这里没有直接套通用 JWT Gin middleware。原因很简单：当前系统不是纯 JWT stateless auth，而是 token hash + Redis session + DB fallback + 平台/设备/IP/单端策略。成熟中间件能用就用，但不能用错地方。

## Session authenticator baseline

`internal/module/session` 现在负责把现有 PHP 登录态读出来。

当前实现：

```text
hash = sha256(access_token + "|" + TOKEN_PEPPER)
Redis token key = TOKEN_REDIS_PREFIX + hash
Redis payload = user_id|expires_at|ip|platform|device_id|session_id
Token Redis 使用独立 DB，默认 TOKEN_REDIS_DB = 2，对齐旧 PHP token 连接
Redis 未命中 -> MySQL user_sessions.access_token_hash
MySQL 条件：revoked_at IS NULL、is_del = 2、expires_at > now
命中 MySQL 后回写 Redis，并按 TOKEN_SESSION_CACHE_TTL 续期
按 auth_platforms 执行 current platform、bind_platform、bind_device、bind_ip、single_session 策略
最终 AuthIdentity.Platform 来自 session.platform
```

当前已实现：

```text
password login 通过 session.Create 签发 access/refresh token
single_session / max_sessions 登录时撤销旧会话
登录前必须通过 go-captcha slide 验证
```

这些仍然不塞回 middleware。

## API contract baseline

新 Go 接口必须是 RESTful：

```text
GET    /api/admin/v1/resources
POST   /api/admin/v1/resources
PUT    /api/admin/v1/resources/:id
PATCH  /api/admin/v1/resources/:id/status
DELETE /api/admin/v1/resources/:id
```

禁止新接口继续 `/api/admin/Xxx/add|edit|del|status` 全 POST。旧 PHP 接口只能是 legacy adapter，不能定义新世界。

## No fallback-field baseline

禁止写静默兜底字段：

```text
不同时接受 user_id/userId/id
不同时接受 id/ids/permission_id/permissionIds
不对缺失关键字段静默补空字符串继续写库
不让前端用 any/Record<string, any> 吞掉契约漂移
```

允许的默认值必须是业务规则本身，例如根节点 `parent_id=0`。兼容必须显式命名为 legacy adapter，并且不能污染 module service。

## App error baseline

服务层返回 `internal/apperror.Error`，不要返回 Gin 响应。

```text
service -> apperror.Error
handler -> response.Error / response.OK
middleware -> response.Abort
```

错误码沿用旧系统核心语义：

```text
0   success
100 parameter/business error
401 unauthorized
403 forbidden
404 not found
500 server error
```

这不是最终业务错误码大全，只是 RBAC/登录/中间件迁移前的最小骨架。

## Typed config baseline

`internal/config` 只负责读取环境变量并产出类型化配置，不创建外部连接。

当前配置域：

```text
App
HTTP
MySQL
Redis
Token
Captcha
```

当前环境变量：

```text
APP_NAME
APP_ENV
HTTP_ADDR
HTTP_READ_HEADER_TIMEOUT
MYSQL_DSN
MYSQL_MAX_OPEN_CONNS
MYSQL_MAX_IDLE_CONNS
MYSQL_CONN_MAX_LIFETIME
REDIS_ADDR
REDIS_PASSWORD
REDIS_DB
TOKEN_PEPPER
TOKEN_ACCESS_TTL
TOKEN_REFRESH_TTL
TOKEN_REDIS_PREFIX
TOKEN_REDIS_DB
TOKEN_SESSION_CACHE_TTL
TOKEN_SINGLE_SESSION_POINTER_TTL
CAPTCHA_TTL
CAPTCHA_REDIS_PREFIX
CAPTCHA_SLIDE_PADDING
CORS_ALLOW_ORIGINS
CORS_ALLOW_HEADERS
CORS_ALLOW_CREDENTIALS
CORS_MAX_AGE
```

规则：

```text
config 不连接 DB
config 不连接 Redis
config 不读取业务表
platform 层以后根据 config 创建 client
```

## Database platform baseline

数据库连接属于 `internal/platform/database`，业务查询属于各模块 repository。

```text
config.MySQL -> platform/database.Open -> *gorm.DB / *sql.DB
repository -> uses database client
service -> calls repository
handler -> calls service
```

当前只建立连接边界和连接池设置，不迁移任何表。

GORM 只作为 MySQL 访问工具，不允许把 GORM model 方法写成业务层。

## Redis platform baseline

Redis 连接属于 `internal/platform/redisclient`，缓存语义属于模块 service。

```text
config.Redis -> platform/redisclient.Open -> *redis.Client
session service -> token/session cache keys, using TokenRedis DB
authplatform service -> auth_platforms policy read path
captcha service -> go-captcha slide answer cache
permission module -> RBAC button grant cache contract
```

当前只建立 Redis client 边界。默认 Redis 连接给通用缓存预留；TokenRedis 使用同一 Redis 地址和密码，但 DB 来自 `TOKEN_REDIS_DB`，默认 2，对齐旧 PHP token 连接。	

## Bootstrap resources baseline

`internal/bootstrap` 负责把 typed config 装配成运行期资源。

```text
config.Load
  -> bootstrap.NewResources
      -> platform/database.Open when MYSQL_DSN is not empty
      -> platform/redisclient.Open when REDIS_ADDR is not empty
      -> platform/redisclient.Open token Redis when REDIS_ADDR is not empty
  -> bootstrap.App owns resources
  -> App.Shutdown closes resources
```

当前规则：

```text
MYSQL_DSN 为空时 DB resource 为 nil，HTTP skeleton 仍可启动
REDIS_ADDR 为空时 Redis resource 为 nil
REDIS_ADDR 为空时 TokenRedis resource 也为 nil
MYSQL_DSN 可由旧 PHP 环境变量 DB_HOST/DB_PORT/DB_DATABASE/DB_USERNAME/DB_PASSWORD 组合得到
REDIS_ADDR 可由旧 PHP 环境变量 REDIS_HOST/REDIS_PORT 组合得到
Token Redis 使用独立 DB，默认 TOKEN_REDIS_DB = 2，对齐旧 PHP token 连接
单端登录指针 TTL 默认 TOKEN_SINGLE_SESSION_POINTER_TTL = 720h，对齐旧 PHP 30 天指针
构造资源不 Ping 外部服务
Ping 放到后续 health/readiness 或运维检查里
```

模块以后通过依赖注入拿资源，不允许自己创建 DB/Redis client。

## Auth login/refresh/logout baseline

`internal/module/auth` 只做认证相关 HTTP 边界。

当前路由：

```text
GET  /api/admin/v1/auth/login-config
GET  /api/admin/v1/auth/captcha
POST /api/admin/v1/auth/login
POST /api/admin/v1/auth/refresh
POST /api/admin/v1/auth/logout
POST /api/Users/getLoginConfig # legacy-compatible adapter
POST /api/Users/login          # legacy-compatible adapter
POST /api/Users/refresh   # legacy-compatible adapter
POST /api/Users/logout    # legacy-compatible adapter
```

规则：

```text
login-config 是公开接口，只暴露当前 Go 已迁移的 password 登录和 captcha 元信息
captcha 是公开接口，使用 go-captcha/v2 slide 生成 master/tile 图片，Redis 短 TTL 保存答案
login 是公开接口，但必须带 captcha_id + captcha_answer，验证码 fail-closed 且一次性消费
password login 只支持邮箱/手机号账号 + PHP bcrypt $2y$ 密码校验
email/phone code 登录暂未迁移，不能在 Go 里假装支持
登录成功通过 session.Create 生成 token，并按 auth_platforms 执行单端/最大会话策略
登录成功/密码错误写 users_login_log，写日志失败不影响主登录结果
refresh 是公开接口，只接收 refresh_token，不走 AuthToken
logout 是认证接口，先走 AuthToken，再撤销当前 access token 对应 session
refresh 通过 user_sessions.refresh_token_hash 查会话
refresh rotate access_token_hash / refresh_token_hash / expires_at / last_seen_at / ip / ua
refresh_expires_at 当前保持旧 PHP 语义：不延长总 refresh 生命周期
refresh 后删除旧 access token Redis 缓存
logout 后 revoke session，并清 token Redis 缓存；单端登录指针匹配当前 session 时才清
```

`auth` handler 不查 DB/Redis；它只解析 JSON / Authorization header，调用 `auth` service。
`captcha` handler 不操作 Redis；它只调用 `captcha` service。

## Health and readiness baseline

`/health` 和 `/ready` 分开。

```text
/health 只证明进程活着，不访问 DB/Redis
/ready 证明运行期依赖是否可用
```

当前 readiness 规则：

```text
MYSQL_DSN 为空：database check = disabled，不算失败
REDIS_ADDR 为空：redis check = disabled，不算失败
REDIS_ADDR 为空：token_redis check = disabled，不算失败
配置了 DB/Redis/TokenRedis：/ready 才 Ping 对应资源
Ping 失败：整体 status = not_ready，响应带 checks 明细
```

这条边界很重要：别把 liveness endpoint 写成外部资源压力测试。外部依赖检查只属于 readiness。

## Users/init RBAC read baseline

当前新增的 RBAC 只读切片是 legacy-compatible adapter：

```text
POST /api/Users/init
```

边界：

```text
AuthToken -> user handler -> user service -> permission service -> repositories
```

规则：

```text
handler 只读取 AuthIdentity，不读 DB/Redis
service 不依赖 gin.Context
permission service 只计算 permissions/router/buttonCodes
button cache key 保持 auth_perm_uid_{userId}_{platform}_rbac_page_grants
Redis button cache 写入是 best-effort，不影响 init 返回
PermissionCheck 先验证 user/role，再按 button cache 命中优先，未命中才计算 RBAC context
角色授权变更通过同一个 button grant cache contract 清理绑定用户缓存
cache 是性能边界，不是权限真相源；miss 或 cache error 必须回源计算，不能放行
```

## Basic admin smoke gate

当前“基本 admin 能跑”的最小门禁不是全业务迁移完成，而是这条链路稳定：

```text
/ready
GET  /api/admin/v1/auth/login-config
GET  /api/admin/v1/auth/captcha
POST /api/admin/v1/auth/login
GET  /api/admin/v1/users/me
GET  /api/admin/v1/users/init
POST /api/admin/v1/permissions
DELETE /api/admin/v1/permissions/:id
POST /api/admin/v1/auth/logout
```

可重复脚本：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\basic-admin-smoke.ps1 `
  -Account <test-account> `
  -Password <test-password>
```

脚本规则：

```text
不在仓库硬编码测试账号或密码
自己编译并启动临时 admin-api smoke binary
使用 go-captcha 真实 challenge，不绕过验证码
只用 Redis 读取本次 challenge 的服务端答案做自动化 smoke
登录后必须访问 users/me 和 users/init，证明 session/RBAC bootstrap 能跑
创建并删除一个临时根目录权限，证明“新增菜单”写路径和权限拦截能跑
最后调用 logout 清理本次 smoke session
成功后清理临时 binary/helper/log
失败时保留 .tmp/basic-admin-smoke-*.log 供排查
```

这不是替代单元测试。它只证明当前环境里的 MySQL、Redis、captcha、session、RBAC read/write path 这一条基本 admin 启动链路没有断。

仍未实现：

```text
email/phone code login
自动注册
登录日志异步队列化
完整业务模块迁移
```

