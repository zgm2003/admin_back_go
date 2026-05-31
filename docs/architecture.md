# admin_back_go Architecture

本仓库采用 `Gin modular monolith`。

完整架构规则见：

```text
E:\admin_go\docs\architecture\04-go-backend-framework.md
E:\admin_go\docs\architecture\05-development-quality-rules.md
```

## 当前阶段

当前后端已经不是早期 admin core foundation。`admin_back_go` 是 Go active runtime：认证、RBAC、用户、App API、日志、通知、邮件、上传、队列、定时任务、支付、AI、WebSocket realtime 等模块已经按根仓库 `E:/admin_go/docs/status/current-status.md` 分批落地。

当前事实来源顺序：

```text
1. live runtime behavior
2. smoke / focused tests
3. E:/admin_go/docs/status/current-status.md
4. E:/admin_go/docs/contracts/*.md
5. 本文件
6. historical specs/plans/comments
```

当前允许：

```text
cmd/admin-api HTTP runtime
cmd/admin-worker queue consumer / scheduler runtime
internal/bootstrap/config/server/middleware/response/version/readiness 基础设施
internal/infra database/redis/taskqueue/scheduler/storage/realtime/ai/payment 等外部资源边界
internal/module 下按业务边界维护 Go modules
internal/jobs 下维护版本化 Asynq task type 和 cron-to-queue 注册
```

当前禁止：

```text
把历史系统当作 active runtime 依赖
把旧 all POST/action path 带进新的 Go REST contract
把 planned/spec 说成 implemented
在 admin_back_go/docs 下继续新增 superpowers plan/spec；统一放 E:/admin_go/docs/superpowers
为了兼容猜测添加 silent fallback 字段
handler 直连 DB/Redis 或 service 依赖 gin.Context
改运行时却不同步 current-status/contract/smoke/backend architecture
```

## 固定调用链

```text
route -> handler -> service -> repository -> model
```

没有真实职责的层不要硬建。

## 模块家族

`internal/module` 是业务边界，不是技术分层垃圾桶。当前模块家族以根仓库 `E:/admin_go/docs/status/current-status.md` 为准，包含 auth/RBAC/user/log/notification/mail/sms/upload/payment/ai/realtime/queue-monitor 等已落地切片。

当前模块拓扑索引：

```text
core/system: system, systemsetting, systemlog, operationlog, crontask, queuemonitor, realtime
identity/RBAC: auth, auth_platform, profile, user, permission, role
comms/upload: mail, sms, notification, notification/task, uploadconfig, uploadtoken, export, clientversion
commerce: payment, payment/wallet
ai: ai/provider, ai/agent, ai/chat, ai/conversation, ai/message, ai/run, ai/tool, ai/knowledge, ai/image
```

App 用户端 API 是独立 HTTP 命名空间，当前挂在 `/api/app/v1`，但它仍复用同一套 capability service。平台不是 module。新增平台不得默认新增 `xxxauth` / `xxxuser` / `xxxupload` 这类平台命名业务模块。平台差异通过 route prefix、platform 字段、策略表和 presenter 表达；业务能力仍归属 `auth` / `user` / `profile` / `uploadtoken` 等模块。当前 `/api/app/v1/auth/*` 归属 `internal/module/auth/transport/app`，`/api/app/v1/users/me` 与 `/api/app/v1/profile` 由 `internal/module/profile/transport/app` 注册并复用现有 user service，`/api/app/v1/upload-tokens` 归属 `internal/module/uploadtoken/transport/app`。

当前 user/profile split 口径：

```text
internal/module/user/transport/admin     # admin 用户管理 HTTP 表面
internal/module/profile/transport/admin  # current-user profile/account-security/quick-entry HTTP 表面
internal/module/profile/transport/app    # app current-user profile compile route
internal/module/uploadtoken/transport/app # app upload-token HTTP 表面
```

本切片只拆 HTTP ownership，不改 admin URL、DB schema、RBAC permission code，也不强行把 user/profile repository 一刀切开。`user` 是 admin user-management capability；`profile` 是 current-user self-service HTTP capability；底层 service/repository 的进一步归属治理需要单独计划。

根 module 不再注册 HTTP 表面。旧 `app_handler.go`、`platform_handler.go`、`app_route_test.go`、`platform_route.go` 不得新增；HTTP 表面统一放在对应 `transport/{platform}/route.go|handler.go|request.go|presenter.go`，并由 `TestNoModuleRootHTTPSurface` 守住；service/repository/model/jobs 仍留在能力根目录。

平台差异默认收敛在 route / handler / presenter / policy。`authplatform` 只拥有认证/会话策略，例如登录方式、验证码类型、token TTL、会话绑定、单端登录和是否允许注册；它不是 AI、钱包、通知等业务的全局平台配置中心。

新增模块必须回答：

```text
这个模块解决的真实业务问题是什么
API contract 在哪里
是否需要 repository/model，还是 handler/service 已经足够
会不会破坏登录、权限、菜单、前端路由和已有 smoke
```

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
I18n
AuthToken
PermissionCheck
OperationLog
module routes
```

middleware 必须一个一个加，并且必须有测试：

```text
AccessLog
CORS
I18n
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

## Operation log baseline

`OperationLog` 只记录显式 route metadata 命中的路由，route metadata 由 `internal/bootstrap/route_meta.go` 维护。

当前 recorder 输入包含：

```text
user_id
session_id
platform
method
path
module
action
title
request_id
client_ip
status
success
latency_ms
request_payload
response_payload
```

规则：

```text
没有 metadata 就不记录。
OperationLog middleware 会在不破坏 handler 读取的前提下捕获 mutating request JSON body，并包裹 ResponseWriter 捕获 JSON response 摘要。
request_data / response_data 存 JSON 字符串摘要，敏感字段先遮蔽再落库；最大捕获 64KB，避免大响应把日志表打爆。
captcha_answer 需要递归遮蔽，不允许把真实验证码坐标写进审计日志。
delete 只开放 devTools_operationLog_del 对应的显式 REST 路由。
```

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
```

允许的前端公共请求头来自当前 `admin_front_ts`：

```text
Content-Type
Accept-Language
Authorization
platform
device-id
X-Trace-Id
X-Request-Id
```

可配置环境变量：

```text
CORS_ALLOW_ORIGINS
```

规则：

```text
生产域名必须显式配置 CORS_ALLOW_ORIGINS
不使用 AllowAllOrigins
不把 CORS 当鉴权
CORS_ALLOW_HEADERS / CORS_ALLOW_CREDENTIALS / CORS_MAX_AGE 是旧 env，Docker-first 下已由代码默认值接管并忽略
遇到浏览器 CORS 报错先确认真实路由和状态码，不要盲改 middleware
```

## I18n baseline

后端 i18n 使用官方 Gin 生态组件：

```text
github.com/gin-contrib/i18n
```

规则：

```text
middleware 顺序是 CORS -> I18n -> AuthToken，保证缺 Token / 无权限这类外层错误也能翻译。
语言来源只读 Accept-Language；支持 zh-CN / en-US；默认 zh-CN。
response shape 不变：{ code, data, msg }。
response 是唯一的 HTTP msg 本地化边界。
msg 是展示文案，业务判断不能依赖 msg。
apperror.Error 保留 fallback Message；MessageID 只做内部翻译 key，不返回给前端。
Catalog 按 internal/shared/i18n/locales/{lang}/{module}.yaml 分模块维护。
已显式收口的模块使用 module-scoped MessageID；剩余 legacy fallback 文案通过 deterministic legacy.{sha1} catalog bridge 在 response 边界翻译。
缺翻译 key 时继续返回 fallback 中文，不允许因为缺翻译 key panic。
legacy fallback 只是一座迁移桥；source coverage tests 会约束它和 explicit MessageID。
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

浏览器不能给部分特殊入口稳定附加 `Authorization` header，所以 `AuthToken` 允许**路径限定 cookie token**，但这不是全局 cookie 登录：

```text
允许：GET/HEAD /api/admin/v1/queue-monitor-ui/* 从 access_token cookie 取 token
允许：GET/HEAD /api/admin/v1/realtime/ws 从 access_token cookie 取 token
禁止：普通 JSON API 从 cookie token 静默兜底
禁止：POST/PUT/PATCH/DELETE 从 cookie token 静默兜底
禁止：/api/admin/v1/realtime/ws?access_token=... query-string token
```

这条边界很重要：cookie fallback 只服务浏览器 UI/upgrade 限制，不改变 REST API 的认证契约。

这里没有直接套通用 JWT Gin middleware。原因很简单：当前系统不是纯 stateless auth，而是 JWT access token + opaque refresh token + Redis session cache + MySQL user_sessions 真相源 + 平台/设备/IP/单端策略。成熟中间件能用就用，但不能用错地方。

## Session authenticator baseline

`internal/module/auth/session.go` 现在负责项目自管登录态，不把认证真相交给第三方 Gin JWT middleware；旧 session standalone module 已合并进 `auth`。

当前实现：

```text
APP_SECRET 是唯一根密钥；internal/infra/secretkey 用 HKDF-SHA256 派生 jwt-signing、token-pepper、secretbox、session-cache keys。
access_token 是本系统签发的 JWT，只包含 sid/sub/platform/device_id/iat/nbf/exp/iss 最小 claims。
refresh_token 是 opaque random string，数据库只保存 sha256(refresh_token + "|" + derived token pepper)。
Redis session key = "token:session:" + session_id，其中 "token:" 是代码内置命名空间。
Redis single-session pointer key = "token:cur_sess:" + platform + ":" + user_id。
Redis payload = user_id|expires_at|ip|platform|device_id|session_id
Token Redis 使用独立 DB，默认 TOKEN_REDIS_DB = 2。
Redis 未命中 -> MySQL user_sessions.id
MySQL 条件：revoked_at IS NULL、is_del = 2、expires_at > now
命中 MySQL 后回写 Redis，并按代码内置 30m 续期。
按 auth_platforms 执行 current platform、bind_platform、bind_device、bind_ip、single_session 策略。
access/refresh token 有效期只来自 auth_platforms.access_ttl / auth_platforms.refresh_ttl。
最终 AuthIdentity.Platform 来自 session.platform，前端不得解析 JWT 决定权限。
```

当前已实现：

```text
password login 通过 session.Create 签发 JWT access token + opaque refresh token
refresh 通过 refresh_token_hash 查 user_sessions，并重新签发 JWT access token
single_session / max_sessions 登录时撤销旧会话并删除 token:session:<session_id> 缓存
登录前必须通过 go-captcha slide 验证
```

这些仍然不塞回 middleware。

## API contract baseline

新 Go 接口必须是 RESTful：

```text
/api/{scope}/v1/resources
scope 当前为 admin，未来预留 app

GET    /api/admin/v1/resources
POST   /api/admin/v1/resources
PUT    /api/admin/v1/resources/:id
PATCH  /api/admin/v1/resources/:id/status
DELETE /api/admin/v1/resources/:id
```

禁止新接口继续 `/api/admin/Xxx/add|edit|del|status` 全 POST。旧接口只能作为历史事实参考，不能定义新世界。

## No fallback-field baseline

禁止写静默兜底字段：

```text
不同时接受 user_id/userId/id
不同时接受 id/ids/permission_id/permissionIds
不对缺失关键字段静默补空字符串继续写库
不让前端用 any/Record<string, any> 吞掉契约漂移
```

允许的默认值必须是业务规则本身，例如根节点 `parent_id=0`。兼容必须显式写清来源、退出条件和验证边界，并且不能污染 module service。

## App error baseline

服务层返回 `internal/shared/apperror.Error`，不要返回 Gin 响应。

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
Queue
Realtime
Scheduler
AI
```

当前环境变量：

```text
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
APP_SECRET
TOKEN_REDIS_DB
QUEUE_ENABLED
QUEUE_REDIS_DB
QUEUE_CONCURRENCY
REALTIME_ENABLED
REALTIME_PUBLISHER
SCHEDULER_ENABLED
CORS_ALLOW_ORIGINS
```

CAPTCHA 业务策略不放 env：`auth.captcha.ttl_minutes` 和
`auth.captcha.slide_padding` 由 `system_settings` 管理，Redis key 前缀
`captcha:slide:` 为代码内置命名空间。

队列运行策略不放 env：Docker-first 只保留 `QUEUE_ENABLED`、
`QUEUE_REDIS_DB`、`QUEUE_CONCURRENCY`。队列 lane 名称
`critical` / `default` / `low`、lane 权重 `6/3/1`、默认重试
`3`、默认 task timeout `30s`、worker shutdown timeout `10s` 都是
`internal/infra/taskqueue` 代码内置默认值。

Scheduler 基础策略不放 env：Docker-first 只保留 `SCHEDULER_ENABLED`。
Scheduler timezone (`Asia/Shanghai`)、Redis lock prefix (`admin_go:scheduler:`)
和 lock TTL (`30s`) 是代码内置默认值，不进 `system_settings`；业务任务启用和
cron 表达式仍由 `cron_task` 表管理。

规则：

```text
config 不连接 DB
config 不连接 Redis
config 不读取业务表
infra 层以后根据 config 创建 client
APP_SECRET 是部署级唯一根密钥，TOKEN_REDIS_DB 是部署级 TokenRedis 隔离项；token Redis prefix `token:`、session cache TTL `30m`、single-session pointer TTL `720h` 是代码内置默认，不进 env，也不进 system_settings。
TOKEN_ACCESS_TTL / TOKEN_REFRESH_TTL 不再存在；业务 token TTL 只在 auth_platforms 表中配置和管理
AIConfig 只表达运行时超时边界：stream max duration、stream idle timeout、run stale timeout；不存 provider 业务参数
```

## Secretbox baseline

上传驱动密钥使用 `internal/infra/secretbox`，只做 AES-GCM 加解密，不知道上传业务。

当前规则：

```text
root env = APP_SECRET
key derivation = HKDF-SHA256 purpose admin_go:secretbox:v1
secretbox input = 32-byte derived key
cipher = AES-256-GCM
nonce/iv = 12 bytes
tag = 16 bytes
storage = base64(iv + tag + ciphertext)
```

`secretbox` 只接收 32-byte derived key；它不读 env，也不知道 APP_SECRET。APP_SECRET 缺失、默认值或长度不足时 API/worker 启动失败；不能假加密，不能明文落库。

## Realtime / WebSocket baseline

Realtime 当前已完成 admin WebSocket 基建、通知任务最小 Redis Pub/Sub fan-out、以及 AI chat runtime 的 `ai.response.*.v1` first-version envelope。它不做 SSE，也不把 AI 事件伪装成旧 unversioned AI run event。

当前路由：

```text
GET /api/admin/v1/realtime/ws
```

认证规则：

```text
优先 Authorization: Bearer <access_token>
浏览器 Vue runtime 使用 GET /api/admin/v1/realtime/ws + access_token cookie 完成 upgrade
cookie token 只对该 WebSocket path 生效；普通 JSON API 不继承这个能力
从 cookie 取 token 时 platform 固定为 admin，用于 session policy 校验
ticket auth 只作为跨域、网关隔离、多端部署后的 planned 方案
```

当前配置：

```text
REALTIME_ENABLED=true              # false 时明确拒绝 WebSocket upgrade，返回 503
REALTIME_PUBLISHER=local|noop|redis
```

Docker-first realtime env 只保留启用开关和 publisher 拓扑。

代码内置：Redis Pub/Sub channel `admin_go:realtime:publish`、heartbeat interval `25s`、send buffer `16`。

装配边界：

```text
bootstrap.newRealtimeStack
  -> infra/realtime.Manager
  -> infra/realtime.Publisher
      local = LocalPublisher -> Manager.Send
      noop  = NoopPublisher  -> 显式丢弃 publication
      redis = RedisPublisher + RedisSubscriber -> LocalPublisher -> Manager.Send
  -> module/realtime.Handler
```

规则：

```text
REALTIME_ENABLED=false 是功能关闭，不是静默兜底；upgrade 直接 503。
REALTIME_PUBLISHER=noop 只允许用于未接业务推送或测试场景；WebSocket connect/ping/pong 仍可运行。
REALTIME_PUBLISHER=redis 用 Redis Pub/Sub 做跨进程 best-effort fan-out；DB notifications 仍是真相源。
WebSocket Origin 不走普通 CORS 预检，gorilla/websocket 需要单独 CheckOrigin；当前复用 CORS_ALLOW_ORIGINS 白名单并允许非浏览器空 Origin / 同 host upgrade。
admin-api 当前可以承载第一期 WebSocket I/O goroutine，但不能在 handler 里跑 CPU-heavy AI 或报表任务。
App.Shutdown 会关闭本机 realtime Manager 下的连接，避免进程停机时遗留连接状态。
Vue runtime 已从旧 ws://127.0.0.1:7272 和 /api/admin/WebSocket/bind 切到 Go baseline：/api/admin/v1/realtime/ws + versioned type/request_id/data envelope。
```

## Database infra baseline

数据库连接属于 `internal/infra/database`，业务查询属于各模块 repository。

```text
config.MySQL -> infra/database.Open -> *gorm.DB / *sql.DB
repository -> uses database client
service -> calls repository
handler -> calls service
```

当前只建立连接边界和连接池设置，不迁移任何表。

GORM 只作为 MySQL 访问工具，不允许把 GORM model 方法写成业务层。

## Redis infra baseline

Redis 连接属于 `internal/infra/redisclient`，缓存语义属于模块 service。

```text
config.Redis -> infra/redisclient.Open -> *redis.Client
session service -> token/session cache keys, using TokenRedis DB
authplatform service -> auth_platforms policy read path
captcha service -> go-captcha slide answer cache
permission module -> RBAC route access grant cache contract
```

当前只建立 Redis client 边界。默认 Redis 连接给通用缓存预留；TokenRedis 使用同一 Redis 地址和密码，但 DB 来自 `TOKEN_REDIS_DB`，默认 2，对齐旧 token 连接。

### Address dict cache

`address` 表仍是行政区划真相源。`user` module 只缓存派生结构：

```text
key: admin_go:dict:address:v1
ttl: none
payload: AddressDictSnapshot { tree, path_by_id, row_count, source_max_updated }
```

读取策略：

```text
Redis hit -> return cached tree/path_by_id
Redis miss -> query MySQL address -> rebuild snapshot -> SET key without expiration
Redis corrupt payload -> DEL key best-effort -> query MySQL
Redis connection error -> query MySQL
```

失效策略：

```powershell
redis-cli DEL admin_go:dict:address:v1
```

如果未来新增 Go address CRUD/import，写入成功后必须删除该 key。

## Bootstrap resources baseline

`internal/bootstrap` 负责把 typed config 装配成运行期资源。

```text
config.Load
  -> bootstrap.NewResources
      -> infra/database.Open when MYSQL_DSN is not empty
      -> infra/redisclient.Open when REDIS_ADDR is not empty
      -> infra/redisclient.Open token Redis when REDIS_ADDR is not empty
  -> bootstrap.App owns resources
  -> App.Shutdown closes resources
```

当前规则：

```text
MYSQL_DSN 为空时 DB resource 为 nil，HTTP skeleton 仍可启动
REDIS_ADDR 为空时 Redis resource 为 nil
REDIS_ADDR 为空时 TokenRedis resource 也为 nil
MYSQL_DSN 可由legacy 环境变量 DB_HOST/DB_PORT/DB_DATABASE/DB_USERNAME/DB_PASSWORD 组合得到
REDIS_ADDR 可由legacy 环境变量 REDIS_HOST/REDIS_PORT 组合得到
Token Redis 使用独立 DB，默认 TOKEN_REDIS_DB = 2，对齐旧 token 连接
单端登录指针 TTL 代码内置为 720h，对齐旧 30 天指针；真正单端登录策略仍由 auth_platforms.single_session 管理。
access/refresh token TTL 不在 bootstrap/config 里生成；登录和 refresh 必须经 auth_platforms 平台策略读取
构造资源不 Ping 外部服务
Ping 放到后续 health/readiness 或运维检查里
```

模块以后通过依赖注入拿资源，不允许自己创建 DB/Redis client。


## System log baseline

系统日志第一期是 Go 运行日志文件的只读浏览，不和操作日志混用。

边界：

```text
cmd/admin-api -> infra/logging -> slog JSON stdout + optional lumberjack file
cmd/admin-worker -> infra/logging -> slog JSON stdout + optional lumberjack file
module/systemlog -> infra/logstore -> runtime/logs/*.log
```

规则：

```text
operationlog = 后台用户操作审计，DB 是事实源
systemlog    = 系统运行日志文件，只读，文件系统是事实源
logstore     = 唯一允许碰 OS 日志文件的边界
```

采用组件：

```text
log/slog                       # Go 官方结构化日志
lumberjack.v2                  # 文件滚动
Gin Recovery + project AccessLog # HTTP runtime log，不重复挂 gin.Logger
```

文件策略：

```text
日志目录来自 LOG_DIR；Docker-first 默认 /app/runtime/logs。
admin-api 默认写 runtime/logs/admin-api.log。
admin-worker 默认写 runtime/logs/admin-worker.log。
文件轮转策略是代码默认值：64MB、7 backups、14 days、compress=true。
日志读取白名单是代码默认值：.log。
读取行数上限是代码默认值：2000。
如果后续拆 admin-realtime，也必须给独立进程文件名，不能和 admin-api 混写。
```

进程身份不来自 env。`cmd/admin-api` 固定使用 `logging.ForProcess("admin-api")`，`cmd/admin-worker` 固定使用 `logging.ForProcess("admin-worker")`；Docker-first Compose service name 也分别是 `admin-api` 和 `admin-worker`。共享 `admin-go.env` 不再提供 `APP&#95;NAME`，避免同一份 env 同时服务 API/worker 时产生错误语义。

这些日志策略不进 system_settings。原因是日志初始化早于 DB；DB 不通、migration 出错、启动失败时仍要能写 stdout 和文件日志。

路由：

```text
GET /api/admin/v1/system-logs/page-init
GET /api/admin/v1/system-logs/files
GET /api/admin/v1/system-logs/files/:name/lines
```

安全限制：

```text
只读，不做 delete/clear/download
只允许配置扩展名，默认 .log
只扫描根目录和一级子目录
禁止绝对路径、..、反斜杠路径、空字节
读取行数受代码默认上限 2000 限制
```

`router.UseRawPath = true` 且 `UnescapePathValues = false`，用于让 `worker%2Fadmin-worker.log` 这种一级子目录文件名在 Gin 参数里保持 escaped slash 语义，不让路由把它误拆成多段路径。


## System settings boundary

系统设置菜单页已经迁到 Go REST：

```text
GET    /api/admin/v1/system-settings/page-init
GET    /api/admin/v1/system-settings
POST   /api/admin/v1/system-settings
PUT    /api/admin/v1/system-settings/:id
PATCH  /api/admin/v1/system-settings/:id/status
DELETE /api/admin/v1/system-settings/:id
DELETE /api/admin/v1/system-settings
```

边界规则：

```text
system_settings 是少量 typed key/value 配置的管理入口，不是所有模块的垃圾抽屉
systemsetting module 只拥有后台 CRUD；已经迁出的跨模块读取不再通过业务模块自己解释 system_settings
shared/setting 拥有仍属于 system_settings 的 typed keys：auth.captcha.ttl_minutes、upload.token.ttl_minutes；验证码发送 TTL 已迁出到 mail_configs.verify_code_ttl_minutes 和 sms_configs.verify_code_ttl_minutes。email code 使用 Tencent SES + mail_configs TTL；phone code 当前固定 123456 写入 Redis，不发送 Tencent SMS，但 Redis TTL 仍来自 sms_configs.verify_code_ttl_minutes。
value_type 只来自 internal/shared/enum -> internal/shared/dict，handler 用 validator 拒绝非法 type
service 做值类型校验：数字、布尔、JSON object/array
key 只允许 create，edit 不允许改 key，避免缓存和业务读取歧义
写入、状态、删除必须清理 Redis cache；key 规则继承 legacy：sys_setting_raw_ + setting key 中的 "." 替换为 "_"
```

历史系统的 `devtools_queue_monitor_queues` 不再属于 Go system-settings 契约。Go 队列监控已经使用 `QUEUE_*` env、Asynq Redis lane 和官方 asynqmon UI；收口时只清理这条旧配置项，不删除队列监控功能。



## Mail Tencent SES boundary

系统管理的邮件管理页已经迁到 Go REST：

```text
GET    /api/admin/v1/mail/page-init
GET    /api/admin/v1/mail/config
PUT    /api/admin/v1/mail/config
DELETE /api/admin/v1/mail/config
POST   /api/admin/v1/mail/test
GET    /api/admin/v1/mail/templates
POST   /api/admin/v1/mail/templates
PUT    /api/admin/v1/mail/templates/:id
PATCH  /api/admin/v1/mail/templates/:id/status
DELETE /api/admin/v1/mail/templates/:id
GET    /api/admin/v1/mail/logs
GET    /api/admin/v1/mail/logs/:id
DELETE /api/admin/v1/mail/logs/:id
DELETE /api/admin/v1/mail/logs
```

边界规则：

```text
internal/module/mail 拥有 mail_configs / mail_templates / mail_logs 业务事实、软删除、日志和验证码邮件编排
internal/infra/mail/tencentcloudses 是唯一允许 import Tencent Cloud SDK 的包
只支持 Tencent Cloud SES API；不做 SMTP、自建邮件服务器、多供应商抽象
SendEmail region 只暴露 ap-guangzhou / ap-hongkong；默认 ap-guangzhou，不让后台用户手写任意 region
SecretId / SecretKey 是后台业务配置，使用 APP_SECRET 派生 secretbox 加密入库，不进入 .env
HTTP config 响应只返回 secret_id_hint / secret_key_hint，不返回明文或密文
mail_logs 只记录场景、收件人、主题、腾讯 RequestId/MessageId、错误码、耗时和状态；详情可附带模板摘要帮助定位 TemplateID/变量名，但不保存正文、验证码明文、完整模板数据
三张表都有 is_del；所有 read path 过滤 is_del=2；删除都是 soft delete
```

`auth/send-code` 集成规则：

```text
auth.Service 只依赖 VerifyCodeMailSender 小接口，不 import module/mail 或 Tencent SDK
email：生成随机验证码，先写 Redis，再发 Tencent SES，TTL 来自 mail_configs.verify_code_ttl_minutes；发信失败 best-effort 删除 Redis key 并返回错误
phone：固定验证码 123456，写 Redis 后返回成功，不接短信，不受 env 控制；Redis TTL 仍来自 sms_configs.verify_code_ttl_minutes
Tencent SMS 模块当前只拥有配置、模板、日志和 test-send；不声明登录短信发送已接入
如果生产不开放手机号登录，由 `auth_platforms.login_types` 关闭 `phone`
```

## System cron task boundary

系统管理的定时任务页已经迁到 Go REST：

```text
GET/POST/PUT/PATCH/DELETE /api/admin/v1/cron-tasks
GET /api/admin/v1/cron-tasks/:id/logs
```

运行时边界：

```text
cron_task DB 只负责配置、状态、cron 表达式和页面展示
Go crontask registry 才是可执行任务真相源
admin-worker 启动时读取启用的 cron_task，并只注册 registry 中存在且 cron 合法的任务
scheduler callback 只写 cron_task_log 并 enqueue Asynq task
业务执行必须在 Asynq handler 内完成
```

当前已注册任务：

```text
notification_task_scheduler -> notification:dispatch-due:v1
ai_run_timeout -> ai:run-timeout:v1
payment_sync_pending_order -> payment:sync-pending-order:v1
payment_close_expired_order -> payment:close-expired-order:v1
```

`cron_task.handler` 不允许按字符串动态执行 handler。已接入 Go registry 的任务必须保存/返回版本化 Asynq task type，例如：`notification_task_scheduler -> notification:dispatch-due:v1`、`ai_run_timeout -> ai:run-timeout:v1`、`payment_sync_pending_order -> payment:sync-pending-order:v1`、`payment_close_expired_order -> payment:close-expired-order:v1`。公共列表不再展示 registry status / legacy handler 迁移态；已废弃的 `clean_expired_contact_request` 通过 cleanup migration 从 active rows 软删除。当前 payment order Alipay pay v1 slice 的 scheduler 只负责充值完成补偿，不包含退款、对账、微信支付或业务履约。

修改 cron_task 后当前不做 worker 热重载；需要重启 admin-worker。scheduler Redis lock 已在 `internal/infra/scheduler` 通过 `internal/infra/redislock` 接入；worker 热重载、outbox 和更高级的多 worker 编排仍是 planned，不在 admin-api 里跑 cron。

## Queue / worker baseline

后台任务第一期不是微服务，而是单体内多进程：

```text
cmd/admin-api     # HTTP API，不消费队列，不跑 cron
cmd/admin-worker  # 队列消费 + scheduler
```

组件选择：

```text
queue     = github.com/hibiken/asynq
monitor   = github.com/hibiken/asynqmon
scheduler = github.com/go-co-op/gocron/v2
```

当前目录：

```text
internal/infra/taskqueue  # 项目自己的 Task / Enqueuer / Mux / Server 封装
internal/infra/scheduler  # 项目自己的 Scheduler 封装
internal/jobs                # 任务 type 和 handler 注册
internal/module/queuemonitor # asynq inspector read model + official asynqmon UI mount
```

队列监控不从零手写完整 dashboard。Gin 只负责 HTTP 路由；真正的 Asynq 队列监控采用 Asynq 官方生态 `github.com/hibiken/asynqmon`，通过 `gin` 挂载到认证后的后台命名空间：

```text
GET/ANY /api/admin/v1/queue-monitor-ui/*
```

当前策略：

```text
asynqmon 以 ReadOnly=true 运行，POST/DELETE 等破坏性操作由 asynqmon 自身拒绝
AuthToken middleware 仍然保护 /api/admin/v1/queue-monitor-ui/*
由于 iframe/new window 不能主动附加 Authorization header，AuthToken 只对 /api/admin/v1/queue-monitor-ui 的 GET/HEAD 文档请求允许读取现有 access_token cookie；普通 JSON API 不启用 cookie token fallback，POST/DELETE 也不启用
cookie token 认证只在该 UI 路径显式使用后台平台 admin 补齐 session policy 入参；不要把这个扩展成全局默认平台
前端 iframe 必须使用 Go API origin 的绝对 URL，不能写成相对路径；否则浏览器会请求前端 SPA 自己的 /api/admin/v1/queue-monitor-ui 并落到前端 404
asynqmon@v0.7.2 内置静态 UI handler 在 Windows 下会把 URL path 经 filepath.Abs 转成盘符路径，导致首页返回 400 unexpected path prefix；因此本项目仅复制官方 ui/build 静态文件并用薄 handler 渲染，/api 子路径仍交给官方 asynqmon handler
保留 GET /api/admin/v1/queue-monitor 与 GET /api/admin/v1/queue-monitor/failed 作为轻量 JSON 摘要接口，服务 dashboard card/smoke，不复制 asynqmon 的完整任务管理能力
configured lane 即使还没有 Asynq Redis key，也必须以 0 计数显示；只把 Asynq queue-not-found 归一化为空队列，Redis 连接/鉴权等真实错误必须继续失败可见
前端队列监控页只是官方 asynqmon 的薄 iframe/新窗口包装，不维护第二套任务列表 UI
```

注意：`asynqmon@v0.7.2` 是 Asynq 官方生态当前可用监控组件，README 的兼容表只写到 Asynq `0.23.x -> 0.7.x`，而本项目用 `asynq v0.26.0`。已通过本地编译/单元测试验证当前 API 可用；后续升级 Asynq 时必须优先复测 `internal/module/queuemonitor`。

jobs 要分层，但不按 `fast/slow` 目录分。快慢是队列 lane 和 worker 配置，不是业务代码所有权。

当前 lane：

```text
critical # 高优先级短任务：登录日志、权限缓存刷新、通知触发
default  # 普通异步业务
low      # 慢任务/批量任务：报表、导入导出、AI 后处理
```

代码所有权。当前最小骨架只有 `internal/jobs/noop.go`；任务增多后再拆，不提前造空目录：

```text
internal/jobs/registry.go        # 全局注册入口，保持薄，任务多了再拆
internal/jobs/types.go           # 跨模块任务类型命名规则，任务多了再拆
internal/jobs/system/*.go        # 系统级探针、维护任务，任务多了再拆
internal/module/<name>/jobs.go   # 业务模块自己的 task 构造和 handler
```

禁止：

```text
internal/jobs/fast
internal/jobs/slow
按速度给业务代码分包
让慢任务和登录/RBAC/操作日志抢同一个 worker lane
```

当前已注册任务：

```text
system:no-op:v1
auth:login-log:v1
notification:dispatch-due:v1
notification:send-task:v1
```

规则：

```text
任务 type 必须版本化
scheduler 只能投递 queue task，不直接跑业务
worker handler 必须幂等，Asynq 是 at-least-once 语义
业务模块用 module/<name>/jobs.go 暴露 task 构造和 handler，不复用 HTTP handler
需要 DB + queue 强一致时再加 outbox，不用 Redis queue 假装事务
```

当前 Phase 8 基建硬化：

```text
taskqueue.Mux 保存显式 handler registry；未知 task type 返回 ErrHandlerNotRegistered: <type>
jobs.Register 是唯一 worker handler 注册入口
cron-to-queue 注册入口迁到 internal/module/crontask.SchedulerService.RegisterEnabled，数据源是 cron_task 表 + Go registry
internal/infra/scheduler 会在配置 locker 时用 Redis lock 包裹任务，避免多 worker 重复触发同一 cron callback
当前第一条真实 Go cron registry 是 notification_task_scheduler：scheduler callback 写 cron_task_log 并 enqueue notification:dispatch-due:v1，不在 callback 里扫描业务表或写通知
```

worker 配置含义：

```text
QUEUE_ENABLED            # 是否启用队列 client/server/monitor
QUEUE_REDIS_DB           # 队列独立 Redis DB，避免和 session/captcha key 混住
QUEUE_CONCURRENCY        # 单个 admin-worker 进程并发执行 handler 数
SCHEDULER_ENABLED        # 是否注册 DB-backed cron tasks
```

Queue lane 名称、lane 权重、默认重试、默认 timeout 和 worker shutdown timeout 是代码内置策略：`critical/default/low`、`6/3/1`、`3`、`30s`、`10s`。Scheduler timezone、Redis lock prefix 和 lock TTL 也是代码内置策略：`Asia/Shanghai`、`admin_go:scheduler:`、`30s`。它们不是 Docker-first env，也不是 `system_settings`。

本地启动命令：

```powershell
# 后端本地开发统一 Docker-first；Compose 同时启动 admin-api 和 admin-worker
cd E:/admin_go/.docker/admin-go-backend
docker compose up -d --build
```

## Go worker concurrency baseline

Go 的并发单位是 goroutine，不是固定请求进程模型。

```text
goroutine          # 轻量协程，由 Go runtime 调度
OS thread          # runtime 按需使用系统线程
GOMAXPROCS         # 同时执行 Go 代码的 CPU 核心数上限，默认按机器 CPU
QUEUE_CONCURRENCY  # Asynq worker 同时处理多少个 task handler
```

规则：

```text
I/O 密集任务可以适当提高 QUEUE_CONCURRENCY
CPU 密集任务不能无限开 goroutine，要进 low queue 或独立 worker
慢任务必须 timeout + context cancellation + 幂等
扩容优先多开 cmd/admin-worker 进程或拆 low/AI worker，不改业务代码
```

## Auth login/refresh/logout baseline

`internal/module/auth` 现在拥有认证相关 HTTP 边界以及 auth-adjacent 业务实现：captcha、token/session primitives、user session management、login log read/write。旧 captcha/session/usersession/userloginlog standalone modules 已合并进 `auth`；对外 API contract URL 不变。

当前路由：

```text
GET  /api/admin/v1/auth/login-config
GET  /api/admin/v1/auth/captcha
POST /api/admin/v1/auth/send-code
POST /api/admin/v1/auth/login
POST /api/admin/v1/auth/refresh
POST /api/admin/v1/auth/logout
```

规则：

```text
login-config 是公开接口，按 `auth_platforms.login_types` 返回当前平台配置的登录方式，并按 enum 稳定顺序 `email -> phone -> password` 输出；password 必须排最后，验证码登录才是主路径，密码登录是备用路径
captcha 是公开接口，使用 go-captcha/v2 slide 生成 master/tile 图片，Redis 短 TTL 保存答案
send-code 是公开接口，只接受 account + scene；scene 必须来自 enum，验证码 key = VERIFY_CODE_REDIS_PREFIX + account_type + scene + md5(account)
login 是公开接口；password login 必须带 captcha_id + captcha_answer，go-captcha fail-closed 且一次性消费
password login 只支持邮箱/手机号账号 + bcrypt $2y$ 密码校验
email/phone code login 使用 Redis 短 TTL 验证码；email 随机码经 `VerifyCodeMailSender` 调 `internal/module/mail.SendVerifyCode` 真实发送腾讯云 SES 邮件，TTL 来自 mail_configs.verify_code_ttl_minutes；phone 固定验证码 123456，不接短信、不受 env 控制，但 Redis TTL 仍来自 sms_configs.verify_code_ttl_minutes
验证码登录支持自动注册：先校验 code 不消费，再检查 auth_platforms.allow_register；允许注册后消费 code，并在同一事务创建 users + user_profiles + 默认角色
登录成功通过 session.Create 生成 JWT access_token + opaque refresh_token，并按 auth_platforms 执行单端/最大会话策略
登录成功/密码错误/验证码错误写 users_login_log；有 queue producer 时投递 `auth:login-log:v1` 到 critical lane，由 `cmd/admin-worker` 消费；producer 未配置或投递失败时同步写库兜底，写日志失败不影响主登录结果
refresh 是公开接口，只接收 refresh_token，不走 AuthToken
logout 是认证接口，先走 AuthToken，再撤销 JWT sid 对应 session
refresh 通过 user_sessions.refresh_token_hash 查会话
refresh 重新签发 JWT access_token，rotate access_token_hash / refresh_token_hash / expires_at / last_seen_at / ip / ua
refresh_expires_at 当前保持旧 token 语义：不延长总 refresh 生命周期
refresh 后删除 token:session:<session_id> Redis 缓存
logout 后 revoke session，并清 token:session:<session_id> Redis 缓存；单端登录指针匹配当前 session 时才清
```

`auth` handler 不查 DB/Redis；它只解析 JSON / Authorization header，调用 `auth` service。
`auth` captcha handler 不操作 Redis；它只调用 `auth.CaptchaService`。

## User Legacy Closure Slice

用户域旧 live 入口已经按窄切片收口到 Go，不把旧 action path 带进新契约。

当前 Go-owned 路由：

```text
PUT   /api/admin/v1/users/me/quick-entries
GET   /api/admin/v1/users/login-logs/page-init
GET   /api/admin/v1/users/login-logs
GET   /api/admin/v1/user-sessions/page-init
GET   /api/admin/v1/user-sessions
GET   /api/admin/v1/user-sessions/stats
PATCH /api/admin/v1/user-sessions/:id/revoke
PATCH /api/admin/v1/user-sessions/revoke
```

边界：

```text
profile 拥有当前登录用户快捷入口保存：校验 permission 是 admin PAGE 且启用，最多 6 个，事务内软删旧 rows 再插入新 rows，返回 quick_entry；i18n key 继续保持 userquickentry.*，避免响应文案漂移。
auth.LoginLogService 只拥有 users_login_log 读路径：LEFT JOIN users，账号/IP 前缀过滤，date_start/date_end 展开全日边界，用户不存在时 user_name=""。
auth.SessionAdminService 拥有 user_sessions 读和 revoke 写路径：列表不返回 access_token_hash/refresh_token_hash；状态由 revoked_at + refresh_expires_at 计算；revoke 禁止踢当前 AuthIdentity.SessionID。
auth.SessionRevocationService 是 token Redis 清理边界：删除 "token:session:"+session_id；只有 "token:cur_sess:<platform>:<user_id>" 当前值等于被撤销 session id 时才删 pointer。
revoke 路由挂 user_userManager_kick 权限，并写 OperationLog user_session/revoke 或 user_session/revoke_batch。
```

本次补齐：

```text
POST /api/admin/v1/auth/forgot-password 已由 Go auth module 接管：校验 forget scene 验证码，写 users.password 的 bcrypt $2y$ hash，并消费验证码。
前端登录页 forgetPassword 走 Go request；src/lib/http 不再保留 legacyRequest / VITE_SOME_KEY 运行依赖。
```

仍非目标：

```text
EditPassword 前端死定义删除，不为了死接口新增 Go 实现。
full smoke 不随机踢 live session；只验证当前 session anti-kick，非当前 revoke/Redis 清理由 Go 单测覆盖。
```

## Auth platform management baseline

`internal/module/auth_platform` 是认证平台策略的唯一写入口。它控制登录方式、验证码类型、token TTL、会话绑定策略和是否允许自动注册，不是普通配置页。

当前 REST 路由：

```text
GET    /api/admin/v1/auth-platforms/page-init
GET    /api/admin/v1/auth-platforms
POST   /api/admin/v1/auth-platforms
PUT    /api/admin/v1/auth-platforms/:id
PATCH  /api/admin/v1/auth-platforms/:id/status
DELETE /api/admin/v1/auth-platforms/:id
DELETE /api/admin/v1/auth-platforms
```

规则：

```text
init dict 从 internal/shared/dict 派生，dict 再从 internal/shared/enum 派生；前端页面不手写登录方式或验证码 label/value
login_types 只允许 email / phone / password，写入前按 enum 稳定顺序 email -> phone -> password 去重归一化
captcha_type 只允许 slide；Go 后端只实现 go-captcha slide，不返回也不接受未实现类型
create/update 在 handler 边界用 validate tag 拒绝非法 enum；service 再做同一业务规则校验，防止绕过 HTTP handler
list 返回 captcha_type 和归一化 login_types；不返回兼容兜底字段
status flow 只允许修改 status，不顺手重写 captcha_type/login_types/token 策略
admin 核心平台不允许删除，不允许禁用
前端使用 Go `request` 访问 `/api/admin/v1/auth-platforms`；不走 legacyRequest，不加 fallback label
登录滑块弹层只复用官方 `go-captcha-vue` Slide 组件和样式，项目包装层只负责 loading、事件透传、外层 spacing，不自造滑块 UI
```

## Health and readiness baseline

`/health` 和 `/ready` 分开。

```text
/health 只证明进程活着，不访问 DB/Redis
/ready 证明配置的运行期依赖是否可达，并检查 realtime 配置值是否可接受；它不证明 WebSocket upgrade 或跨进程 fan-out
```

当前 readiness 规则：

```text
MYSQL_DSN 为空：database check = disabled，不算失败
REDIS_ADDR 为空：redis check = disabled，不算失败
REDIS_ADDR 为空：token_redis check = disabled，不算失败
QUEUE_ENABLED=false：queue_redis check = disabled，不算失败
QUEUE_ENABLED=true 但 REDIS_ADDR 为空：queue_redis check = down，这是配置错误
配置了 DB/Redis/TokenRedis：/ready 才 Ping 对应资源
配置了 QueueRedis：/ready Ping QUEUE_REDIS_DB 对应 Redis DB
REALTIME_ENABLED=false：realtime check = disabled
REALTIME_ENABLED=true 且 REALTIME_PUBLISHER=local/noop/redis/空：realtime check = up
REALTIME_ENABLED=true 但 REALTIME_PUBLISHER 是其他未实现值：realtime check = down，不能假装已支持
Ping 失败：整体 status = not_ready，响应带 checks 明细
```

这条边界很重要：别把 liveness endpoint 写成外部资源压力测试。外部依赖检查只属于 readiness。

## Users/init RBAC read baseline

当前 RBAC 只读切片由 Go REST 接口提供：

```text
GET /api/admin/v1/users/me
GET /api/admin/v1/users/init
```

边界：

```text
AuthToken -> user handler -> user service -> permission service -> repositories
```

规则：

```text
handler 只读取 AuthIdentity，不读 DB/Redis
service 不依赖 gin.Context
permission service 计算公开 permissions/router/buttonCodes 和内部 RouteAccessCodes
route access grant cache key 保持 auth_perm_uid_{userId}_{platform}_rbac_route_access_grants
Redis route access grant cache 写入是 best-effort，不影响 init 返回
PermissionCheck 先验证 user/role，再按 route access grant cache 命中优先，未命中才计算 RBAC context
角色授权变更通过同一个 route access grant cache contract 清理绑定用户缓存
cache 是性能边界，不是权限真相源；miss 或 cache error 必须回源计算，不能放行
```

### RBAC truth table

当前 Go RBAC 的真相源只有 MySQL 的 `users.role_id`、`roles`、`permissions`、`role_permissions`，Redis 只做 route access grant 缓存。没有隐藏的 super admin 绕过逻辑；如果一个账号要拥有全部权限，就必须通过角色授权把对应 PAGE / BUTTON 授给它。

| 场景 | Go 后端行为 | 前端行为 |
| --- | --- | --- |
| DIR 授权 | role 写入时不保存 DIR；service 只在 PAGE/BUTTON 向上解析祖先时带出 DIR | 只渲染后端返回的树，不自己补父级 |
| PAGE 授权 | `permissions` tree + `router` 都包含该 PAGE；PAGE code 可进入内部 `RouteAccessCodes`；`buttonCodes` 不增加 | 动态路由来自 `router`，按钮显隐不能读 PAGE code |
| BUTTON 授权 | service 自动包含父 PAGE 和祖先 DIR；内部 `RouteAccessCodes` 包含 BUTTON code；`buttonCodes` 只包含 BUTTON code | 按钮显隐只读 `userStore.can(code)`，也就是 `buttonCodes` |
| `show_menu = 2` | 只保留在 menu item 上；不删除 `router`，不影响 PAGE 访问真相 | 可以隐藏菜单，但不能据此推断没有页面权限 |
| role 权限变更 | `SyncPermissions` 做 grant/remove diff；变更后清理绑定用户的 `auth_perm_uid_{userId}_{platform}_rbac_route_access_grants` | 下次 `users/init` 以 Go 返回结果为准 |
| PermissionCheck cache hit | 先验证 user 和 role 存在，再用 Redis route access grant codes 判断 PAGE/BUTTON route metadata code | 前端不参与 API 放行 |
| PermissionCheck cache miss/error | 回源计算 RBAC context；计算失败拒绝 | 前端不兜底 |
| user/role 不存在 | fail-closed：401 或 403 | 重新登录或显示无权限 |
| route metadata 未配置 | 不做权限检查；这是显式未保护，不是猜测放行 | 不反向定义后端权限 |

## Users Management Go REST

用户管理页已经从 旧 `UsersList/*` 迁到 Go 的 REST 资源，不把 legacy 字段带进新契约：

```text
GET    /api/admin/v1/users/page-init     # 页面字典：roles/address tree/sex/platform
GET    /api/admin/v1/users               # 列表和筛选
PUT    /api/admin/v1/users/:id           # 编辑用户安全字段
PATCH  /api/admin/v1/users/:id/status    # 修改状态
PATCH  /api/admin/v1/users               # 批量修改 profile 字段
DELETE /api/admin/v1/users/:id           # 单个软删除
DELETE /api/admin/v1/users               # 批量软删除
POST   /api/admin/v1/users/export        # 创建导出任务并投递 worker
```

关键规则：

```text
users/init 仍只做当前登录用户 bootstrap；用户管理页字典使用 users/page-init。
新契约只接受 address_id，不接受旧 address 别名。
用户列表查询由 handler 做入参绑定，service 做业务归一化，repository 只负责 SQL。
列表搜索默认使用 prefix LIKE，避免把 Go 重构写成全表模糊扫描。
编辑 role_id 成功后清理该用户 admin/app 的 auth_perm_uid_{userId}_{platform}_rbac_route_access_grants 缓存。
删除只做 users + user_profiles 软删除，不物理删数据。
用户导出是用户模块动作：创建 `export_tasks` pending 记录后投递 `export:run:v1` 到 low queue；权限固定用 `user_userManager_export`，不能复用编辑权限。
导出 worker 只在 payload 存 `task_id/kind/user_id/platform/ids`，重新读取用户数据后生成 xlsx；不把渲染后的 rows 塞进 Redis。
导出文件使用当前启用 COS 配置上传到 `exports/YYYYMMDD/`；本阶段不做 OSS runtime、不做万能导出平台。
```

## Notification Current-User Read/List Slice

通知中心已分成两条线：当前用户通知 read/list/read/delete，以及后台通知任务发布/调度。

当前用户通知接口：

```text
GET    /api/admin/v1/notifications/page-init
GET    /api/admin/v1/notifications
GET    /api/admin/v1/notifications/unread-count
PATCH  /api/admin/v1/notifications/:id/read
PATCH  /api/admin/v1/notifications/read
DELETE /api/admin/v1/notifications/:id
DELETE /api/admin/v1/notifications
```

关键边界：

```text
handler 只从 AuthToken middleware 读取当前 user_id/platform，不查 DB。
service 做 enum 和 identity 业务归一化，不依赖 gin.Context。
repository 所有查询/更新固定带 user_id、platform IN(current,'all')、is_del=2。
PATCH /read 空 ids 表示标记当前用户可见全部未读通知；DELETE 空 ids 必须拒绝。
当前用户通知 read/delete 不挂 RBAC button permission，也不写 OperationLog。
```

后台通知任务接口：

```text
GET    /api/admin/v1/notification-tasks/page-init
GET    /api/admin/v1/notification-tasks/status-count
GET    /api/admin/v1/notification-tasks
POST   /api/admin/v1/notification-tasks
PATCH  /api/admin/v1/notification-tasks/:id/cancel
DELETE /api/admin/v1/notification-tasks/:id
```

发布/调度边界：

```text
notification/task service 归属 notification capability，拥有 target_type/platform/send_at 业务校验。
POST 无 send_at：写 notification_task pending 后立即 enqueue notification:send-task:v1。
POST 有 send_at：只写 pending，等待 admin-worker scheduler。
admin-worker 通过 cron_task 表里的 notification_task_scheduler 配置注册 gocron，触发后 enqueue notification:dispatch-due:v1。
dispatch-due handler claim `send_at IS NULL` 的立即 pending 任务和到期定时 pending 任务并 enqueue send-task；这给“DB 已写入但 enqueue/旧 worker 失败”的立即任务提供补偿路径。
send-task handler 解析目标用户、批量写 notifications、更新 sent_count/status；handler 必须幂等，允许 Asynq at-least-once 重投。
当前 DB 写入 + Redis enqueue 不是强事务；需要强一致时再加 outbox，不用 Redis queue 假装事务。
notification.created.v1 通过 worker RedisPublisher -> admin-api RedisSubscriber -> 本机 WebSocket Manager 做 best-effort 推送；DB notifications 写入仍是真相。
```

RBAC 数据迁移：

```text
database/migrations/20260505_add_notification_task_button_permissions.sql
为通知管理页面补齐 system_notificationTask_add / cancel / del 三个 BUTTON 权限。
迁移只给已经拥有 /system/notificationTask PAGE 权限的角色补按钮授权，不创建隐藏超级管理员绕过。
执行后如果用户已有旧 RBAC route access grant cache，需要等待 TTL 或删除 auth_perm_uid_{userId}_admin_rbac_route_access_grants 后重新计算。
```

## Profile + Avatar Upload Slice

个人资料是第一个真实上传业务闭环。它仍归 `internal/module/user`，因为表事实是 `users` + `user_profiles`，没有必要为了“看起来规范”新开空模块。

```text
GET /api/admin/v1/profile            # 当前 token 用户资料
GET /api/admin/v1/users/:id/profile  # 用户管理跳转只读查看
PUT /api/admin/v1/profile            # 当前 token 用户编辑自己的资料
PUT /api/admin/v1/profile/security/password # 当前 token 用户修改/设置登录密码
PUT /api/admin/v1/profile/security/email    # 当前 token 用户绑定或换绑邮箱
PUT /api/admin/v1/profile/security/phone    # 当前 token 用户绑定或换绑手机号
```

关键规则：

```text
GET 不创建缺失的 user_profiles；只按默认值返回，避免读接口偷偷写库。
PUT 只允许改 username/avatar/sex/birthday/address_id/detail_address/bio。
PUT 不接受 address 旧别名，只接受 address_id。
PUT 不允许改 email/phone/role_id/password/has_password/is_self。
用户编辑自己资料不挂 user_userManager_edit；只需要登录态，并记录 OperationLog(profile.update_profile)。
头像上传不做服务端转存；前端 UpMedia 继续通过 POST /api/admin/v1/upload-tokens 获取 COS 临时凭证，folder=avatars。
手机号、邮箱、密码安全流程已迁到 Go REST，仍归 user module；只需要登录态，不挂 user_userManager_edit。
账号安全验证码复用 auth/send-code 的 Redis key 规则，service 通过最小 VerifyCodeStore 接口消费，不让 handler 或 repository 碰 Redis。
OperationLog route metadata 固定为 profile_security.update_password / update_email / update_phone，敏感字段必须被 sanitizer 遮蔽。
```

## Basic admin smoke gate

当前“基本 admin 能跑”的最小门禁不是全业务迁移完成，而是这条链路稳定：

```text
/ready
GET  /api/admin/v1/auth/login-config
GET  /api/admin/v1/auth/captcha
POST /api/admin/v1/auth/send-code
POST /api/admin/v1/auth/login
GET  /api/admin/v1/users/me
GET  /api/admin/v1/users/init
GET  /api/admin/v1/users/page-init
GET  /api/admin/v1/users
POST /api/admin/v1/permissions          # DIR/PAGE/BUTTON smoke subtree
PUT  /api/admin/v1/roles/:id            # grant + restore current test role permissions
DELETE /api/admin/v1/permissions        # batch cleanup smoke subtree
POST /api/admin/v1/auth/logout
```

可重复脚本：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\basic-admin-smoke.ps1 `
  -Account <test-account> `
  -Password <test-password>
```

完整 smoke：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\full-admin-smoke.ps1 `
  -Account <test-account> `
  -Password <test-password>
```

脚本规则：

```text
不在仓库硬编码测试账号或密码
自己编译并启动临时 admin-api/admin-worker smoke binary
使用 go-captcha 真实 challenge，不绕过验证码
只用 Redis 读取本次 challenge 的服务端答案做自动化 smoke
登录后必须访问 users/me 和 users/init，证明 session/RBAC bootstrap 能跑
必须访问 users/page-init 和 users list，证明用户管理页已经走 Go REST 基础链路
必须等待 users_login_log 近 5 分钟内出现本账号登录记录，证明 auth queue/worker 或同步兜底路径能跑
创建临时 DIR/PAGE/BUTTON，临时授给测试账号角色，重新 users/init 验证 router 和 buttonCodes，再恢复角色授权并批量删除临时权限
最后调用 logout 清理本次 smoke session
成功后清理临时 binary/helper/log
失败时保留 .tmp/basic-admin-smoke-*.log 供排查
```

full smoke 规则：

```text
先跑 basic smoke 作为基础链路
只读探测 queue monitor JSON 摘要、failed pagination shape 和 asynqmon UI HEAD；不做 retry/delete/clear
只读探测 system-logs init/files shape；文件列表非空时读取第一份日志 tail lines，不做 delete/clear/download
只读探测 upload-drivers/upload-rules/upload-settings init/list shape
upload config 写探针依赖 API/worker 启动时已验证的 APP_SECRET，只创建 disabled 临时 driver/rule/setting，再按 setting -> rule -> driver 反向清理；永远不启用临时 setting，也不修改现有 enabled setting
upload token 探针不再读取 COS STS Docker env 开关；没有 enabled upload setting 时输出 upload_token_probe=skipped_upload_setting_missing，enabled setting 不可用于 COS runtime 时输出 skipped_upload_setting_not_usable
存在 enabled COS setting 时 POST /api/admin/v1/upload-tokens 只校验 provider/key/credentials shape，不上传真实文件
再单独验证 operation log init/list/delete
用临时权限触发 `新增权限` 审计日志
删除 operation log 行后必须确认它不再出现在列表里
最终只输出 JSON summary
失败时保留 .tmp/full-admin-smoke-*.log 供排查
```

这不是替代单元测试。它只证明当前环境里的 MySQL、Redis、captcha、session、RBAC read/write path 这一条基本 admin 启动链路没有断。

仍未实现：

```text
完整业务模块迁移
通用服务端上传平台；当前上传运行时只支持腾讯云 COS
```


## Enum / Dict / Validation Baseline

Go 后端从认证平台开始建立统一基础件：

- `internal/shared` now owns `apperror`、`response`、`i18n`、`enum`、`validate`、`dict`、`setting`；旧 root shared-like packages 已删除。
- `internal/shared/enum` 只放跨模块稳定常量，例如 `CommonYes/CommonNo`、登录方式、平台标识、验证码类型、验证码场景、通知类型/级别。
- `internal/shared/dict` 是跨模块字典边界，统一 option payload 和 `common_status`、`common_yes_no`、`platform`、`system_setting_value_type` registry providers；labels / values 不变。
- `internal/shared/setting` 仍是 migrated typed settings key 的跨模块边界；`systemsetting` 仍只是 `system_settings` 的 admin CRUD surface。
- `internal/shared/validate` 注册 Gin binding / go-playground validator 自定义 tag，例如 `common_status`、`common_yes_no`、`platform_scope`、`platform_code`、`permission_type`、`auth_platform_login_type`、`captcha_type`、`verify_code_scene`、`user_sex`、`notification_type`、`notification_level`、`payment_provider`、`payment_method`（仅历史兼容标签，当前 payment-config request 不使用）；handler 只能用这些 enum-backed tag，不允许散落硬编码 `oneof=...`。
- Plan 11 只迁移 shared package ownership，不声明 module aggregation、DB schema、URL、response shape、enum value、validation semantics 或 i18n catalog behavior 变化。
- 模块 HTTP 入参结构放在 `internal/module/<capability>/transport/{platform}/request.go`，handler 只 bind request 并转换到 service input；`dto.go` 不承载 Gin binding tag。
- HTTP 入参先在 handler 边界拒绝明显非法数据；service 再做业务规则校验。handler 校验是边界，不是业务真相源。
- `auth_platforms.captcha_type` 是认证平台策略字段，当前只允许 `slide`，因为后端只实现了 go-captcha slide；不假装支持未实现类型。
- 新 REST 接口继续走 `/api/admin/v1/...`，旧 all POST 只作为业务事实参考。

上传配置当前新增：

```text
internal/shared/enum/upload.go      # COS-only 上传驱动、上传扩展名、上传 folder 白名单
internal/shared/dict.Upload*Options # upload driver/image ext/file ext dict
internal/shared/validate/upload.go  # upload_driver/upload_image_ext/upload_file_ext/upload_folder
```

`internal/module/uploadconfig` 只管理配置事实源：

```text
GET/POST/PUT/DELETE /api/admin/v1/upload-drivers
GET/POST/PUT/DELETE /api/admin/v1/upload-rules
GET/POST/PUT/PATCH/DELETE /api/admin/v1/upload-settings
```

规则：

```text
driver secret 永远不返回明文或密文，只返回 hint
driver/rule 被 setting 引用时不能删除
setting 启用互斥必须在 repository transaction 内完成，不能靠前端猜或两条普通 update 碰运气
uploadconfig 不做 /api/getUploadToken，不安装任何云 SDK，不做真实上传
upload runtime 只接受腾讯云 COS；OSS SDK 不进入默认 go.mod/package.json，历史/非 COS 配置必须显式报错并重新配置 COS
```

支付域当前收敛为 `internal/module/payment` bounded context，active product scope 是支付宝配置、充值收银台和支付订单/支出流水；`payment_orders` 是底层支付订单 runtime，不再作为用户手工创建入口：

```text
internal/module/payment                 # 支付宝配置 CRUD、证书上传、本地配置测试、充值 cashier、订单 pay/sync/close、钱包入账
internal/infra/payment               # 证书私有存储、路径解析、支付网关接口
internal/infra/payment/alipay        # 唯一允许接入支付宝 SDK 的 infra adapter
payment_configs                         # 支付配置 active runtime 表，sort 用于充值自动选优先配置
payment_orders                          # 底层支付订单 runtime 表
payment_recharge_packages               # 充值套餐 seed/read 表
payment_recharges                       # 用户充值单业务表
payment_callback_events                 # 支付宝异步回调审计表
user_wallets                            # 用户钱包余额表
wallet_transactions                     # 钱包流水与入账幂等表
```

`internal/module/payment` active contract：

```text
GET    /api/admin/v1/payment/configs/page-init
GET    /api/admin/v1/payment/configs
POST   /api/admin/v1/payment/configs
PUT    /api/admin/v1/payment/configs/:id
PATCH  /api/admin/v1/payment/configs/:id/status
DELETE /api/admin/v1/payment/configs/:id
POST   /api/admin/v1/payment/certificates
POST   /api/admin/v1/payment/configs/:id/test

GET    /api/admin/v1/payment/recharges/page-init
GET    /api/admin/v1/payment/recharges
GET    /api/admin/v1/payment/recharges/:id
POST   /api/admin/v1/payment/recharges
POST   /api/admin/v1/payment/recharges/:id/pay
POST   /api/admin/v1/payment/recharges/:id/sync
PATCH  /api/admin/v1/payment/recharges/:id/close

GET    /api/admin/v1/payment/orders/page-init
GET    /api/admin/v1/payment/orders
GET    /api/admin/v1/payment/orders/:id
POST   /api/admin/v1/payment/orders              # backend/internal low-level capability; raw create UX not exposed by product page
POST   /api/admin/v1/payment/orders/:id/pay
POST   /api/admin/v1/payment/orders/:id/sync
PATCH  /api/admin/v1/payment/orders/:id/close

POST   /api/payment/callbacks/alipay     # public Alipay async callback; no admin auth/RBAC/OperationLog
```

规则：

```text
payment V1 范围是 Alipay config、recharge cashier、底层 Alipay order pay 和充值完成闭环；refund、withdraw、reconcile、WeChat、subscription、wallet consumption 或业务履约都不在本 slice。
配置事实源是 payment_configs；充值业务事实源是 payment_recharges；订单事实源是 payment_orders；钱包事实源是 user_wallets + wallet_transactions。
payment_callback_events 只做支付宝回调审计，不作为业务真相源。
payment_configs.sort 参与充值自动选配置：status=1、provider=alipay、enabled_methods_json 包含当前 pay_method 后按 sort ASC, id ASC 取第一条。
return_url 不属于 payment_configs，也不是用户可编辑字段；充值页按当前 `/payment/recharge` 路由生成，后端追加 `tab=records&recharge_no=...`。
RCG/PAY/WLT 单号由后端生成并只当不透明唯一字符串使用；新单号保留时间和纳秒段，但序列后缀使用紧凑大写 base36，不再追加 20 位补零十进制序列；历史单号不迁移。
paid/credited 状态只能由已验签支付宝 callback、手动支付宝 query/sync 或 payment_sync_pending_order 补偿路径写入，后台不能手工改 paid。
callback、manual sync、cron compensation 必须共用 paid finalizer；钱包入账必须在 DB transaction 内完成，并通过 wallet_transactions(source_type, source_id) 保证同一充值单只入账一次。
finalizer 状态推进必须单调：旧 callback/sync/cron 快照不能把 `credited` 倒退回 `paid`，已经 `closed` 或 `failed` 的充值单不能被迟到 finalizer 重新打开或入账。
证书上传只写本地私有相对路径：runtime/payment/certs/alipay/<config_code>/<sha256>.crt，不走 COS，不暴露 public URL，不提供下载。
支付宝 SDK 只允许出现在 internal/infra/payment/alipay；module/payment 只能依赖明确的小接口/DTO，不能直接 import 第三方 SDK。
应用私钥只允许写入、加密保存、本地测试时解密；响应、operation log、smoke 输出和前端类型都不能泄露 app_private_key 或 private_key_enc。
菜单路径展示 /payment/config、/payment/recharge 和 /payment/orders；/payment/orders 是支付订单/支出流水入口，但不能再暴露 raw create UX；旧 channel/event/pay/wallet 菜单必须从 users/init router 消失。
provider 是当前字段合同的一部分，但只允许 alipay；merchant_id、sign_type、extra_config 不属于当前字段合同。
```

支付 cron 当前只注册支付宝充值完成补偿：

```text
payment_sync_pending_order  -> payment:sync-pending-order:v1
payment_close_expired_order -> payment:close-expired-order:v1
```

规则：

```text
正式异步回调入口固定为 POST /api/payment/callbacks/alipay；旧 /api/payment/notify/alipay、/api/pay/notify/alipay 不再作为 active route。
payment_sync_pending_order 扫描支付中支付宝订单并复用 shared finalizer 补偿入账。
payment_close_expired_order 扫描过期未支付订单并关闭本地/支付宝订单。
支付宝返回 `ACQ.TRADE_NOT_EXIST` 且本地订单已过期时，按未支付过期处理：关闭本地 payment_order 和关联 payment_recharge，避免充值页 reopen auto-sync 反复打扰用户。
退款、对账、履约重试、钱包消费必须另写 spec/plan，再决定 cron、索引和事件表。
不要注册旧支付域任务或 WeChat 相关任务。
```

`internal/module/uploadtoken` 管理运行时 token 签发：

```text
POST /api/admin/v1/upload-tokens
```

规则：

```text
只要求 AuthToken；这是登录用户头像/聊天/富文本/文件字段会复用的 current-user capability，不要求 system_uploadToken_create 或其他 RBAC button 权限
不注册 OperationLog route metadata；响应包含临时 STS credentials，真正保存上传对象引用的业务模块负责自己的操作日志
只读取当前 enabled upload_setting，并 join driver/rule；不改 upload_* 表结构
只接受 driver=cos；历史/非 COS 行必须显式报“当前上传驱动未启用 COS runtime”
folder/file_name/file_size/file_kind 在 handler/service 双层校验，folder 来自 enum.UploadFolders
object key 由服务端生成：{folder}/{yyyy}/{mm}/{dd}/{unix_ms}-{randomhex}-{safe_file_name}
rule.max_size_mb/image_exts/file_exts 是上传限制真相；前端校验只做体验优化
secret_id_enc/secret_key_enc 只在 service 内用 secretbox 解密并传给 signer，响应和 operation log 不返回明文
upload token TTL 来自 system_settings.upload.token.ttl_minutes，缺失/禁用/非法时默认 15 分钟
Tencent STS endpoint/region 是 infra/storage/cos 代码内置默认值；上传配置 Region 仍然是 bucket region，用于 COS policy resource
```

上传业务归属规则：

```text
uploadtoken 只签发临时凭证，不定义业务。
禁止新建无业务归属的 upload scene；folder 只能服务已经存在或正在迁移的业务实体。
业务模块先定义自己的表字段、状态、权限、操作日志和 REST contract，再接 upload token/client。
业务模块负责保存 object key/url 等引用；uploadtoken 不落业务引用、不创建“无主文件”事实源。
后续 AI agent avatar、chat attachment、rich text image 等都必须作为对应业务模块的一部分迁移，不能为了“上传页面”单独偷跑。
```

AI Core provider / agent config boundary（2026-05-09）：

```text
admin_go + internal/infra/ai 是当前 AI 架构边界。
已落地“供应商配置”和“智能体配置 MVP”，第一版 provider driver 只有 openai。
Vue 不直连 AI provider，provider key 不进浏览器；module 不直接 import OpenAI SDK/client。
供应商配置不引入流程编排概念，不嵌入第三方控制台。
智能体配置负责选择 provider 下的启用模型，并保存场景、系统提示词和头像等本地运行元数据。
```

Active AI modules:

```text
internal/infra/ai            # OpenAI-compatible chat/test interface; no Dify/RAGFlow active adapter
internal/infra/ai/provider   # provider discovery/test boundary; first driver is OpenAI / GET /models
internal/module/ai/provider      # ai_providers provider config + ai_provider_models model catalog
internal/module/ai/agent         # ai_agents local agent config MVP
internal/module/ai/tool          # ai_tools / ai_agent_tools / ai_tool_calls runtime
internal/module/ai/image         # ai_image_tasks / ai_image_assets generation runtime
internal/module/ai/knowledge     # local RAG: bases/documents/chunks/agent bindings/retrieval audit
internal/module/ai/conversation   # current-user conversations; canonical agent_id -> ai_agents
internal/module/ai/message        # conversation messages, feedback, branch cleanup
internal/module/ai/run            # ai_runs / ai_run_events token-only run monitor
internal/module/ai/chat           # chat runtime through infra/ai.Engine, ai.response.*.v1 publish
```

Retired AI active runtime:

```text
legacy AI model/tool/prompt/knowledge-base REST resources
legacy AI knowledge-map metadata/routes
legacy AI tool-map metadata/routes
legacy model/prompt Vue menu entries and legacy app naming
```

这些旧精确 route 字符串只能留在 backup/rollback SQL、历史 spec/plan 或 negative router tests。不要把 `aimodel` / `aiprompt`、`aiknowledgemap`、`aiapp` 或旧工具映射模块重新挂回 server/bootstrap。

Schema truth:

```text
docs/db/ai-live-schema-mcp-2026-05-10.md # MCP snapshot: the only current AI table-count/column-count handoff truth
20260509_ai_conversation_message_mvp.sql  # ai_conversations / ai_messages WebSocket conversation MVP
20260509_ai_agent_mvp_prune.sql           # prunes ai_agents down to provider/model/scenes/system_prompt/avatar
20260509_ai_agent_drop_type_code.sql      # drops fake agent code/type concepts
20260510_ai_run_monitor_mvp.sql           # ai_runs / ai_run_events token-only monitor
20260510_ai_messages_meta_json.sql        # message attachments/runtime params metadata
20260510_ai_tool_runtime_mvp.sql          # ai_tools, ai_agent_tools, ai_tool_calls, admin_user_count seed
20260510_ai_tool_drop_executor.sql        # removes duplicate ai_tools.executor; tool code is the dispatch key
20260510_ai_tool_generate_permission.sql  # AI tool generate draft button permission
20260510_ai_knowledge_rag.sql             # local RAG tables and retrieval audit
20260510_ai_prune_stale_permissions.sql   # soft-deletes stale unused AI permission codes
```

Provider config truth:

```text
Current runtime accepts only openai. Dify/RAGFlow/Eino/Direct are not active source adapters.
```

```text
base_url empty string means https://api.openai.com/v1
model discovery uses GET {base_url}/models
ai_provider_models is the source for enabled selectable provider models
API key is write-only and encrypted into api_key_enc; DTOs expose only api_key_masked
health/model-sync status values are unknown / ok / failed
```

Agent config truth:

```text
route/menu/API name is agent: /ai/agents and /api/admin/v1/ai-agents
table is ai_agents; old app naming must not be the active contract
create/update require provider_id + model_id where model_id belongs to enabled ai_provider_models under that provider
list search supports name/provider/status plus scene=chat or scene=agent_generate; there is no agent code or agent type in the MVP
MVP scenes currently allow chat and agent_generate; stored as scenes_json and exposed as scenes/scene_names
system_prompt and avatar are optional local agent metadata
ai_agents intentionally has no agent code, agent type, per-agent external app id/key, response mode, runtime config JSON, model snapshot JSON, created_by, or updated_by in the MVP slice
tool usage is stored in ai_agent_tools; knowledge usage is stored in ai_agent_knowledge_bases; do not add duplicate JSON binding fields to ai_agents
```

Knowledge RAG truth:

```text
active tables are ai_knowledge_bases, ai_knowledge_documents, ai_knowledge_chunks, ai_agent_knowledge_bases, ai_knowledge_retrievals, ai_knowledge_retrieval_hits
/ai/knowledge manages bases/documents/chunks/retrieval tests; /ai/agents owns which knowledge bases an agent can read
retrieval is deterministic local text scoring in Go; no vector DB, no hosted file_search, no Dify/RAGFlow dataset sync in this slice
runtime writes retrieval and hit audit rows before provider call; hit rows snapshot chunk content for historical run monitor display
selected context is injected into the current provider input only; it does not mutate ai_agents.system_prompt or persisted user message content
```

Runtime boundary:

```text
POST /api/admin/v1/ai-conversations/:id/messages must fail explicitly when no enabled provider/agent exists; production must not fake success.
Provider streams/events stay server-side; browser receives admin_go WebSocket envelopes: ai.response.start/delta/completed/failed.v1.
OpenAI-compatible StreamChat does not use a 30s HTTP total timeout while reading response body; live max duration and upstream silence timeout are code-owned AI runtime guardrails, not Docker-first env knobs.
ai_run_timeout is stale cleanup only: admin-worker marks running rows older than the code-owned AI run stale timeout default, not fresh online replies.
ai_runs records one reply attempt with status, token totals, duration, and message links.
ai_run_events records lifecycle events only: start/completed/failed/canceled/timeout.
ai_tool_calls records tool execution audit and is shown on run detail; tool calls are not stuffed into ai_run_events.
ai_knowledge_retrievals and ai_knowledge_retrieval_hits record knowledge retrieval audit and are shown on run detail; knowledge retrievals are not stuffed into ai_run_events.
WebSocket delta is not persisted to ai_run_events; final assistant content stays in ai_messages.
There is no daily aggregate table, billing amount, provider task id, execution-step timeline, usage dump, or snapshot JSON in the run-monitor MVP.
admin-worker fan-out still depends on REALTIME_PUBLISHER=redis for cross-process realtime.
```
`internal/infra/storage/cos` 是唯一 COS STS 供应商边界：

```text
采用 github.com/tencentyun/qcloud-cos-sts-sdk/go 签发 STS 临时凭证
module 只依赖 CredentialSigner 小接口，不知道 SDK 类型
STS policy 只授权当前生成 key 的 PutObject/PostObject，不给 bucket 全量写权限
所有网络调用必须接收 context，并由 signer 加 timeout
测试用 fake requester / httptest server，不打真实腾讯网络
```

开源取舍：

```text
qcloud-cos-sts-sdk/go 是本阶段合适的轻量依赖，因为这里只签临时凭证，不做服务端 object 操作
uploadtoken 仍不依赖 cos-go-sdk-v5；clientversion 的 Tauri manifest 服务端发布是独立小边界，允许使用 cos-go-sdk-v5 PutObject 写很小的 update JSON，不把它扩散成通用服务端上传能力
阿里云 OSS Go/JS SDK 不进入默认依赖；当前上传配置和运行时只支持腾讯云 COS，历史/非 COS 配置必须显式报错并重新配置 COS
```

前端共享上传客户端：

```text
src/api/system/uploadToken.ts 定义 Go REST typed API
src/lib/upload/uploadClient.ts 只保留 cos-js-sdk-v5 动态加载
不再使用 legacyRequest、/api/getUploadToken、ali-oss、any/as any/Record<string, any>
上传客户端只支持腾讯云 COS；历史/非 COS 配置必须显式错误，不能静默 fallback 到 COS
```

客户端版本管理当前新增：

```text
internal/shared/enum/client_version.go          # windows-x86_64 / darwin-x86_64 平台枚举
internal/shared/dict.ClientVersionPlatformOptions
internal/shared/validate/client_version.go      # client_platform validator
internal/module/clientversion            # 客户端版本 REST + manifest 发布
internal/infra/storage/cos.ObjectWriter # 仅服务端 PutObject 小边界
```

`internal/module/clientversion` 管理系统管理 / 版本管理：

```text
GET    /api/admin/v1/client-versions/page-init
GET    /api/admin/v1/client-versions
GET    /api/admin/v1/client-versions/update-json
GET    /api/admin/v1/client-versions/current-check
POST   /api/admin/v1/client-versions
PUT    /api/admin/v1/client-versions/:id
PATCH  /api/admin/v1/client-versions/:id/latest
PATCH  /api/admin/v1/client-versions/:id/force-update
DELETE /api/admin/v1/client-versions/:id
```

规则：

```text
业务名是 client version / 客户端版本；DB 表统一为 `client_versions`，mutation 权限统一为 `system_clientVersion_*`。项目未上线，不保留历史 Tauri 表名/按钮 code 特殊情况；旧 Tauri 名称只允许作为迁移 source condition 或 legacy source reference出现。前端 route folder、页面 i18n key、菜单 PAGE path/component/i18n_key 使用 `clientVersion`；旧菜单数据通过 `database/migrations/20260507_client_version_permission_route_cleanup.sql` 迁移。
read/page-init/update-json 只要求 AuthToken，不注册 OperationLog；current-check 是 public path，只返回 force_update boolean。
mutation route 使用 `system_clientVersion_*` button codes，并显式注册 OperationLog module=client_version。
create 默认 is_latest=2、force_update=2、is_del=2；delete 只软删除且拒绝删除当前最新版本。
set latest 在 repository transaction 内清同平台旧 latest、设新 latest，并发布 Tauri static updater manifest。
update/force-update 如果影响最新版本，会重新发布 manifest；发布失败必须返回显式错误，不允许 DB 成功但 update.json 静默旧值。
```

manifest 发布边界：

```text
Service -> ManifestPublisher -> ManifestCOSPublisher -> storage/cos.ObjectWriter -> github.com/tencentyun/cos-go-sdk-v5
```

规则：

```text
只支持 COS server-side PutObject 写 `tauri_updater/{platform}.json`，Content-Type 固定 application/json; charset=utf-8。
COS credential 仍来自 enabled upload_setting + upload_driver，secretbox 只在 publisher 内解密，响应和日志不输出 secret。
OSS 不做静默 fallback；启用配置不是 COS 时明确报错。
这个 cos-go-sdk-v5 依赖不能被滥用成“后端通用上传服务”；真实文件仍按业务模块先定义事实，再走 upload token / 客户端直传。
```
