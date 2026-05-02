# admin_back_go Architecture

本仓库采用 `Gin modular monolith`。

完整架构规则见：

```text
E:\admin_go\docs\architecture\04-go-backend-framework.md
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
internal/module/auth refresh/logout HTTP 边界
internal/module/permission RBAC read context 计算边界
internal/module/user Users/init legacy-compatible read adapter
```

未允许：

```text
RBAC 写路径和 PermissionCheck 中间件
登录迁移
前端适配
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
router.Group("/api/v1")
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
module routes
```

后续 middleware 必须一个一个加，并且必须有测试：

```text
AccessLog
CORS
AuthToken
PermissionCheck
OperationLog
```

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

`AuthToken` 当前只是认证边界，不迁移登录业务。

它只负责：

```text
跳过 public path：/health /ready /api/v1/ping
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

当前没有实现：

```text
登录签发 token
RBAC PermissionCheck
```

这些下一步按 service 边界继续接，不塞回 middleware。

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
permission service -> RBAC permission cache keys
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

## Auth refresh/logout baseline

`internal/module/auth` 只做认证相关 HTTP 边界。

当前路由：

```text
POST /api/v1/auth/refresh
POST /api/v1/auth/logout
POST /api/Users/refresh   # legacy-compatible adapter
POST /api/Users/logout    # legacy-compatible adapter
```

规则：

```text
refresh 是公开接口，只接收 refresh_token，不走 AuthToken
logout 是认证接口，先走 AuthToken，再撤销当前 access token 对应 session
refresh 通过 user_sessions.refresh_token_hash 查会话
refresh rotate access_token_hash / refresh_token_hash / expires_at / last_seen_at / ip / ua
refresh_expires_at 当前保持旧 PHP 语义：不延长总 refresh 生命周期
refresh 后删除旧 access token Redis 缓存
logout 后 revoke session，并清 token Redis 缓存；单端登录指针匹配当前 session 时才清
```

`auth` handler 不查 DB/Redis；它只解析 JSON / Authorization header，调用 `session` service。

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
```

仍未实现：

```text
login
captcha / go-captcha
PermissionCheck
RBAC 写路径
前端改造
```

