# admin_back_go Architecture

本仓库采用 `Gin modular monolith`。

完整架构规则见：

```text
E:\admin_go\docs\architecture\04-go-backend-framework.md
E:\admin_go\docs\architecture\05-development-quality-rules.md
```

## 当前阶段

当前阶段是 admin core foundation：先把认证、RBAC、队列、日志、配置、验证、基础 smoke 固定住，再迁移更多业务模块。

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
internal/module/authplatform 平台认证策略读取和管理边界
internal/module/captcha go-captcha slide 验证码边界
internal/module/auth password login/refresh/logout HTTP 边界
internal/module/auth email/phone code login、本地 dev code、自动注册、登录日志 task 边界
internal/module/permission RBAC read context 计算边界
internal/module/permission 权限定义 RESTful 写路径
internal/module/role 角色授权 RESTful 写路径
internal/module/user Users/init legacy-compatible read adapter
internal/module/operationlog 操作日志 REST + 显式 route metadata 审计边界
internal/middleware/PermissionCheck 显式 route metadata 边界
internal/middleware/OperationLog 显式 route metadata 边界
cmd/admin-worker queue consumer / scheduler 独立进程边界
internal/platform/taskqueue Asynq 封装边界
internal/platform/scheduler gocron/v2 封装边界
internal/jobs 版本化任务注册边界
internal/module/uploadtoken COS-first 上传临时凭证签发边界
internal/platform/storage/cos 腾讯云 COS STS signer 边界
internal/platform/realtime gorilla/websocket 薄封装、Session/Manager/Publisher 边界
internal/module/realtime admin WebSocket upgrade、envelope、heartbeat、subscribe 基础边界
```

未允许：

```text
批量迁移非 RBAC 业务模块
真实短信/邮件发送器接入
AI 应用接入
把业务模块批量异步化
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
uploadconfig
uploadtoken
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

浏览器不能给部分特殊入口稳定附加 `Authorization` header，所以 `AuthToken` 允许**路径限定 cookie token**，但这不是全局 cookie 登录：

```text
允许：GET/HEAD /api/admin/v1/queue-monitor-ui/* 从 access_token cookie 取 token
允许：GET/HEAD /api/admin/v1/realtime/ws 从 access_token cookie 取 token
禁止：普通 JSON API 从 cookie token 静默兜底
禁止：POST/PUT/PATCH/DELETE 从 cookie token 静默兜底
禁止：/api/admin/v1/realtime/ws?access_token=... query-string token
```

这条边界很重要：cookie fallback 只服务浏览器 UI/upgrade 限制，不改变 REST API 的认证契约。

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
access/refresh token 有效期只来自 auth_platforms.access_ttl / auth_platforms.refresh_ttl
最终 AuthIdentity.Platform 来自 session.platform
```

当前已实现：

```text
password login 通过 session.Create 签发 access/refresh token
refresh 重新签发 access token 时继续读取当前 session.platform 对应 auth_platforms.access_ttl
single_session / max_sessions 登录时撤销旧会话
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
Queue
Realtime
Scheduler
Secretbox
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
TOKEN_REDIS_PREFIX
TOKEN_REDIS_DB
TOKEN_SESSION_CACHE_TTL
TOKEN_SINGLE_SESSION_POINTER_TTL
CAPTCHA_TTL
CAPTCHA_REDIS_PREFIX
CAPTCHA_SLIDE_PADDING
QUEUE_ENABLED
QUEUE_REDIS_DB
QUEUE_CONCURRENCY
QUEUE_DEFAULT_QUEUE
QUEUE_CRITICAL_WEIGHT
QUEUE_DEFAULT_WEIGHT
QUEUE_LOW_WEIGHT
QUEUE_SHUTDOWN_TIMEOUT
QUEUE_DEFAULT_MAX_RETRY
QUEUE_DEFAULT_TIMEOUT
REALTIME_ENABLED
REALTIME_PUBLISHER
REALTIME_HEARTBEAT_INTERVAL
REALTIME_SEND_BUFFER
SCHEDULER_ENABLED
SCHEDULER_TIMEZONE
SCHEDULER_LOCK_PREFIX
CORS_ALLOW_ORIGINS
CORS_ALLOW_HEADERS
CORS_ALLOW_CREDENTIALS
CORS_MAX_AGE
VAULT_KEY
```

规则：

```text
config 不连接 DB
config 不连接 Redis
config 不读取业务表
platform 层以后根据 config 创建 client
TOKEN_REDIS_PREFIX / TOKEN_REDIS_DB / TOKEN_SESSION_CACHE_TTL / TOKEN_SINGLE_SESSION_POINTER_TTL 是部署级 Redis/session 基础设施配置，保留 env
TOKEN_ACCESS_TTL / TOKEN_REFRESH_TTL 不再存在；业务 token TTL 只在 auth_platforms 表中配置和管理
```

## Secretbox baseline

上传驱动密钥使用 `internal/platform/secretbox`，只做 AES-GCM 加解密，不知道上传业务。

当前规则：

```text
env = VAULT_KEY
key derivation = sha256(VAULT_KEY)
cipher = AES-256-GCM
nonce/iv = 12 bytes
tag = 16 bytes
storage = base64(iv + tag + ciphertext)
```

这是为了兼容旧 PHP KeyVault 已有密文格式。`VAULT_KEY` 为空时 encrypt/decrypt 必须明确失败；不能假加密，不能明文落库。

## Realtime / WebSocket baseline

Realtime 当前已完成 admin WebSocket 基建和通知任务最小 Redis Pub/Sub fan-out，不做 AI streaming 业务。

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
REALTIME_REDIS_CHANNEL=admin_go:realtime:publish
REALTIME_HEARTBEAT_INTERVAL=25s    # server ping interval，也返回给 connected envelope
REALTIME_SEND_BUFFER=16            # 每连接 bounded send queue，满了关闭慢客户端
```

装配边界：

```text
bootstrap.newRealtimeStack
  -> platform/realtime.Manager
  -> platform/realtime.Publisher
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
access/refresh token TTL 不在 bootstrap/config 里生成；登录和 refresh 必须经 auth_platforms 平台策略读取
构造资源不 Ping 外部服务
Ping 放到后续 health/readiness 或运维检查里
```

模块以后通过依赖注入拿资源，不允许自己创建 DB/Redis client。


## System log baseline

系统日志第一期是 Go 运行日志文件的只读浏览，不和操作日志混用。

边界：

```text
cmd/admin-api -> platform/logging -> slog JSON stdout + optional lumberjack file
cmd/admin-worker -> platform/logging -> slog JSON stdout + optional lumberjack file
module/systemlog -> platform/logstore -> runtime/logs/*.log
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
LOG_FILE_MAX_SIZE_MB / LOG_FILE_MAX_BACKUPS / LOG_FILE_MAX_AGE_DAYS / LOG_FILE_COMPRESS 控制轮转，不允许单个日志无限增长。
admin-api 默认写 runtime/logs/admin-api.log。
admin-worker 默认写 runtime/logs/admin-worker.log。
LOG_FILE_NAME 保留为兼容基线；进程入口会用 LOG_API_FILE_NAME / LOG_WORKER_FILE_NAME 覆盖实际文件名。
如果后续拆 admin-realtime，也必须给独立进程文件名，不能和 admin-api 混写。
```

路由：

```text
GET /api/admin/v1/system-logs/init
GET /api/admin/v1/system-logs/files
GET /api/admin/v1/system-logs/files/:name/lines
```

安全限制：

```text
只读，不做 delete/clear/download
只允许配置扩展名，默认 .log
只扫描根目录和一级子目录
禁止绝对路径、..、反斜杠路径、空字节
读取行数受 LOG_MAX_TAIL_LINES 限制
```

`router.UseRawPath = true` 且 `UnescapePathValues = false`，用于让 `worker%2Fadmin-worker.log` 这种一级子目录文件名在 Gin 参数里保持 escaped slash 语义，不让路由把它误拆成多段路径。


## System settings boundary

系统设置菜单页已经迁到 Go REST：

```text
GET    /api/admin/v1/system-settings/init
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
value_type 只来自 internal/enum -> internal/dict，handler 用 validator 拒绝非法 type
service 做值类型校验：数字、布尔、JSON object/array
key 只允许 create，edit 不允许改 key，避免缓存和业务读取歧义
写入、状态、删除必须清理 Redis cache；key 规则继承 legacy：sys_setting_raw_ + setting key 中的 "." 替换为 "_"
```

旧 PHP 的 `devtools_queue_monitor_queues` 不再属于 Go system-settings 契约。Go 队列监控已经使用 `QUEUE_*` env、Asynq Redis lane 和官方 asynqmon UI；迁移时只清理这条旧配置项，不删除队列监控功能。

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
internal/platform/taskqueue  # 项目自己的 Task / Enqueuer / Mux / Server 封装
internal/platform/scheduler  # 项目自己的 Scheduler 封装
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
jobs.RegisterSchedules 是唯一 cron-to-queue 注册入口
ScheduledTaskDefinition 只能 build queue task；scheduler callback 只调用 Enqueuer.Enqueue
当前第一条真实业务 schedule 是 notification-task-dispatch-due：每 1 分钟只投递 notification:dispatch-due:v1，不在 scheduler callback 里扫描 DB 或写通知
```

worker 配置含义：

```text
QUEUE_REDIS_DB           # 队列独立 Redis DB，避免和 session/captcha key 混住
QUEUE_CONCURRENCY        # 单个 admin-worker 进程并发执行 handler 数
QUEUE_CRITICAL_WEIGHT    # critical lane 权重
QUEUE_DEFAULT_WEIGHT     # default lane 权重
QUEUE_LOW_WEIGHT         # low lane 权重
QUEUE_DEFAULT_MAX_RETRY  # task 默认重试次数；handler 必须幂等
QUEUE_DEFAULT_TIMEOUT    # task 默认超时；慢任务必须尊重 context cancellation
QUEUE_SHUTDOWN_TIMEOUT   # worker 停机等待 in-flight task 的时间
SCHEDULER_TIMEZONE       # gocron 注册时区
SCHEDULER_LOCK_PREFIX    # 预留分布式 scheduler lock 前缀；真正多 worker cron 再启用锁策略
```

本地启动命令：

```powershell
# HTTP API，只处理 REST 请求，不消费队列，不跑 cron
go run ./cmd/admin-api

# 后台进程，消费 Asynq queue，并在启用时启动 scheduler
go run ./cmd/admin-worker
```

## Go worker concurrency baseline

Go 的并发单位是 goroutine，不是 PHP-FPM 那种固定请求进程模型。

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

`internal/module/auth` 只做认证相关 HTTP 边界。

当前路由：

```text
GET  /api/admin/v1/auth/login-config
GET  /api/admin/v1/auth/captcha
POST /api/admin/v1/auth/send-code
POST /api/admin/v1/auth/login
POST /api/admin/v1/auth/refresh
POST /api/admin/v1/auth/logout
POST /api/Users/getLoginConfig # legacy-compatible adapter
POST /api/Users/sendCode       # legacy-compatible adapter
POST /api/Users/login          # legacy-compatible adapter
POST /api/Users/refresh   # legacy-compatible adapter
POST /api/Users/logout    # legacy-compatible adapter
```

规则：

```text
login-config 是公开接口，按 `auth_platforms.login_types` 返回当前平台配置的登录方式，并按 enum 稳定顺序 `email -> phone -> password` 输出；password 必须排最后，验证码登录才是主路径，密码登录是备用路径
captcha 是公开接口，使用 go-captcha/v2 slide 生成 master/tile 图片，Redis 短 TTL 保存答案
send-code 是公开接口，只接受 account + scene；scene 必须来自 enum，验证码 key = VERIFY_CODE_REDIS_PREFIX + account_type + scene + md5(account)
login 是公开接口；password login 必须带 captcha_id + captcha_answer，go-captcha fail-closed 且一次性消费
password login 只支持邮箱/手机号账号 + PHP bcrypt $2y$ 密码校验
email/phone code login 使用 Redis 短 TTL 验证码；本地开发 `VERIFY_CODE_DEV_MODE=true` 使用测试码，生产环境必须接入真实短信/邮件发送器后再关闭 dev mode
验证码登录支持自动注册：先校验 code 不消费，再检查 auth_platforms.allow_register；允许注册后消费 code，并在同一事务创建 users + user_profiles + 默认角色
登录成功通过 session.Create 生成 token，并按 auth_platforms 执行单端/最大会话策略
登录成功/密码错误/验证码错误写 users_login_log；有 queue producer 时投递 `auth:login-log:v1` 到 critical lane，由 `cmd/admin-worker` 消费；producer 未配置或投递失败时同步写库兜底，写日志失败不影响主登录结果
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

## Auth platform management baseline

`internal/module/authplatform` 是认证平台策略的唯一写入口。它控制登录方式、验证码类型、token TTL、会话绑定策略和是否允许自动注册，不是普通配置页。

当前 REST 路由：

```text
GET    /api/admin/v1/auth-platforms/init
GET    /api/admin/v1/auth-platforms
POST   /api/admin/v1/auth-platforms
PUT    /api/admin/v1/auth-platforms/:id
PATCH  /api/admin/v1/auth-platforms/:id/status
DELETE /api/admin/v1/auth-platforms/:id
DELETE /api/admin/v1/auth-platforms
```

规则：

```text
init dict 从 internal/dict 派生，dict 再从 internal/enum 派生；前端页面不手写登录方式或验证码 label/value
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
/ready 证明运行期依赖是否可用
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
REALTIME_ENABLED=true 且 REALTIME_PUBLISHER=local/noop/空：realtime check = up
REALTIME_ENABLED=true 但 REALTIME_PUBLISHER 是未实现值，例如 redis：realtime check = down，不能假装 Redis fan-out 已实现
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

### RBAC truth table

当前 Go RBAC 的真相源只有 MySQL 的 `users.role_id`、`roles`、`permissions`、`role_permissions`，Redis 只做 button grant 缓存。没有隐藏的 super admin 绕过逻辑；如果一个账号要拥有全部权限，就必须通过角色授权把对应 PAGE / BUTTON 授给它。

| 场景 | Go 后端行为 | 前端行为 |
| --- | --- | --- |
| DIR 授权 | role 写入时不保存 DIR；service 只在 PAGE/BUTTON 向上解析祖先时带出 DIR | 只渲染后端返回的树，不自己补父级 |
| PAGE 授权 | `permissions` tree + `router` 都包含该 PAGE；`buttonCodes` 不增加 | 动态路由来自 `router` |
| BUTTON 授权 | service 自动包含父 PAGE 和祖先 DIR；`buttonCodes` 包含 BUTTON code | 按钮显隐只读 `userStore.can(code)`，也就是 `buttonCodes` |
| `show_menu = 2` | 只保留在 menu item 上；不删除 `router`，不影响 PAGE 访问真相 | 可以隐藏菜单，但不能据此推断没有页面权限 |
| role 权限变更 | `SyncPermissions` 做 grant/remove diff；变更后清理绑定用户的 `auth_perm_uid_{userId}_{platform}_rbac_page_grants` | 下次 `users/init` 以 Go 返回结果为准 |
| PermissionCheck cache hit | 先验证 user 和 role 存在，再用缓存判断 button code | 前端不参与 API 放行 |
| PermissionCheck cache miss/error | 回源计算 RBAC context；计算失败拒绝 | 前端不兜底 |
| user/role 不存在 | fail-closed：401 或 403 | 重新登录或显示无权限 |
| route metadata 未配置 | 不做权限检查；这是显式未保护，不是猜测放行 | 不反向定义后端权限 |

## Users Management Go REST

用户管理页已经从 PHP `UsersList/*` 迁到 Go 的 REST 资源，不把 legacy 字段带进新契约：

```text
GET    /api/admin/v1/users/page-init     # 页面字典：roles/address tree/sex/platform
GET    /api/admin/v1/users               # 列表和筛选
PUT    /api/admin/v1/users/:id           # 编辑用户安全字段
PATCH  /api/admin/v1/users/:id/status    # 修改状态
PATCH  /api/admin/v1/users               # 批量修改 profile 字段
DELETE /api/admin/v1/users/:id           # 单个软删除
DELETE /api/admin/v1/users               # 批量软删除
```

关键规则：

```text
users/init 仍只做当前登录用户 bootstrap；用户管理页字典使用 users/page-init。
新契约只接受 address_id，不接受旧 PHP 的 address 别名。
用户列表查询由 handler 做入参绑定，service 做业务归一化，repository 只负责 SQL。
列表搜索默认使用 prefix LIKE，避免把 Go 重构写成全表模糊扫描。
编辑 role_id 成功后清理该用户 admin/app 的 auth_perm_uid_{userId}_{platform}_rbac_page_grants 缓存。
删除只做 users + user_profiles 软删除，不物理删数据。
export 暂时保留显式 legacy adapter，等待 Go export-task 队列模块迁移；这不是 silent fallback。
```

## Notification Current-User Read/List Slice

通知中心已分成两条线：当前用户通知 read/list/read/delete，以及后台通知任务发布/调度。

当前用户通知接口：

```text
GET    /api/admin/v1/notifications/init
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
GET    /api/admin/v1/notification-tasks/init
GET    /api/admin/v1/notification-tasks/status-count
GET    /api/admin/v1/notification-tasks
POST   /api/admin/v1/notification-tasks
PATCH  /api/admin/v1/notification-tasks/:id/cancel
DELETE /api/admin/v1/notification-tasks/:id
```

发布/调度边界：

```text
notificationtask service 拥有 target_type/platform/send_at 业务校验。
POST 无 send_at：写 notification_task pending 后立即 enqueue notification:send-task:v1。
POST 有 send_at：只写 pending，等待 admin-worker scheduler。
admin-worker 的 notification-task-dispatch-due schedule 每 1 分钟 enqueue notification:dispatch-due:v1。
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
执行后如果用户已有旧 RBAC button cache，需要等待 TTL 或删除 auth_perm_uid_{userId}_admin_rbac_page_grants 后重新计算。
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
VAULT_KEY 为空时跳过 upload config 写探针，并输出 upload_write_probe=skipped_no_vault_key
VAULT_KEY 存在时只创建 disabled 临时 driver/rule/setting，再按 setting -> rule -> driver 反向清理；永远不启用临时 setting，也不修改现有 enabled setting
COS_STS_ENABLED=false 时跳过 upload token 探针，并输出 upload_token_probe=skipped_cos_sts_disabled
COS_STS_ENABLED=true 时 POST /api/admin/v1/upload-tokens 只校验 provider/key/credentials shape，不上传真实文件
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
服务端上传；OSS runtime 只作为可选扩展
```


## Enum / Dict / Validation Baseline

Go 后端从认证平台开始建立统一基础件：

- `internal/enum` 只放跨模块稳定常量，例如 `CommonYes/CommonNo`、登录方式、平台标识、验证码类型、验证码场景、通知类型/级别。
- `internal/dict` 负责把 enum 转成前端 `dict` 选项，不允许业务页面自己手写一份枚举标签。
- `internal/validate` 注册 Gin binding / go-playground validator 自定义 tag，例如 `common_status`、`common_yes_no`、`platform_scope`、`platform_code`、`permission_type`、`auth_platform_login_type`、`captcha_type`、`verify_code_scene`、`user_sex`、`notification_type`、`notification_level`、`pay_channel`、`pay_method`；handler 只能用这些 enum-backed tag，不允许散落硬编码 `oneof=...`。
- 模块 HTTP 入参结构放在 `internal/module/<name>/request.go`，handler 只 bind request 并转换到 service input；`dto.go` 不承载 Gin binding tag。
- HTTP 入参先在 handler 边界拒绝明显非法数据；service 再做业务规则校验。handler 校验是边界，不是业务真相源。
- `auth_platforms.captcha_type` 是认证平台策略字段，当前只允许 `slide`，因为后端只实现了 go-captcha slide；不假装支持未实现类型。
- 新 REST 接口继续走 `/api/admin/v1/...`，旧 PHP 全 POST 只作为业务事实参考。

上传配置当前新增：

```text
internal/enum/upload.go      # cos/oss、上传扩展名、上传 folder 白名单
internal/dict.Upload*Options # upload driver/image ext/file ext dict
internal/validate/upload.go  # upload_driver/upload_image_ext/upload_file_ext/upload_folder
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
upload runtime 默认只接受 COS 依赖；OSS SDK 不进入默认 go.mod/package.json，缺少可选实现时必须显式报错
```

支付渠道当前新增：

```text
internal/enum/pay.go          # 微信/支付宝渠道、支付方式、渠道支持方式、支付流水状态规则
internal/dict.Pay*Options     # pay channel/method dict
internal/validate/pay.go      # pay_channel/pay_method/pay_txn_status
internal/module/paychannel    # 支付渠道配置管理 REST
internal/module/paytransaction # 支付流水只读 REST
internal/module/payorder       # 后台统一订单管理 REST
```

`internal/module/paychannel` 只管理 `pay_channel` 配置事实源：

```text
GET    /api/admin/v1/pay-channels/page-init
GET    /api/admin/v1/pay-channels
POST   /api/admin/v1/pay-channels
PUT    /api/admin/v1/pay-channels/:id
PATCH  /api/admin/v1/pay-channels/:id/status
DELETE /api/admin/v1/pay-channels/:id
```

规则：

```text
本切片不接支付 SDK、不发起充值、不处理回调、不改钱包余额。
app_private_key 只在 service 内通过 secretbox 加密成 app_private_key_enc，并保存 app_private_key_hint；响应和 operation log 不允许泄露明文或密文。
supported_methods 存在 pay_channel.extra_config.supported_methods；service 负责按 channel 校验微信/支付宝支持方式。
同一 channel+mch_id+app_id 不允许重复。
被 orders.channel_id 或 pay_transactions.channel_id 引用的渠道不能删除，只能禁用，避免破坏历史订单/流水可审计性。
新 REST DELETE 只删除单个 path id，不接受 legacy id 数组批量删除。
```

`internal/module/paytransaction` 只读 `pay_transactions` 流水事实：

```text
GET /api/admin/v1/pay-transactions/page-init
GET /api/admin/v1/pay-transactions
GET /api/admin/v1/pay-transactions/:id
```

规则：

```text
本切片只做只读查询，不接支付 SDK、不发起支付、不重试回调、不改钱包余额、不执行对账。
repository 可以 left join orders/users/pay_channel 生成展示事实，但不能 select pay_channel.app_private_key 或 app_private_key_enc。
handler 用 pay_txn_status/pay_channel validator 拦非法查询值；service 只做分页默认值、trim、时间/JSON 展示归一化。
channel_resp/raw_notify 空或非法 JSON 统一归一成 `{}`，不是字符串兜底，也不是前端猜格式。
只读路由只注册 permission metadata `pay_transaction_list`，不注册 operation log metadata。
```

`internal/module/payorder` 管理后台统一订单页：

```text
GET   /api/admin/v1/pay-orders/page-init
GET   /api/admin/v1/pay-orders/status-count
GET   /api/admin/v1/pay-orders
GET   /api/admin/v1/pay-orders/:id
PATCH /api/admin/v1/pay-orders/:id/remark
PATCH /api/admin/v1/pay-orders/:id/close
```

规则：

```text
本切片只做后台订单查询、状态统计、详情、备注和 Go 本地关闭订单。
read route permission code 使用 DB 里真实存在的 `pay_recharge_list`，不是凭页面名发明 `pay_order_list`。
close/remark 使用 `pay_order_edit`，并显式注册 operation log metadata：module=pay_order，action=close/remark。
close 只允许 pay_status=PENDING/PAYING；repository transaction 内更新 orders 并关闭最后一条 active pay_transactions。
close 不调用第三方支付 SDK，不查单，不关第三方平台订单，不改钱包余额；第三方支付 runtime 后续独立迁移。
status-count 永远按 pay_status enum 顺序返回完整项，前端不能自己猜 label/count shape。
```

`internal/module/uploadtoken` 管理运行时 token 签发：

```text
POST /api/admin/v1/upload-tokens
```

规则：

```text
只读取当前 enabled upload_setting，并 join driver/rule；不改 upload_* 表结构
只接受 driver=cos；driver=oss 必须显式报“当前上传驱动未启用 COS runtime”
folder/file_name/file_size/file_kind 在 handler/service 双层校验，folder 来自 enum.UploadFolders
object key 由服务端生成：{folder}/{yyyy}/{mm}/{dd}/{unix_ms}-{randomhex}-{safe_file_name}
rule.max_size_mb/image_exts/file_exts 是上传限制真相；前端校验只做体验优化
secret_id_enc/secret_key_enc 只在 service 内用 secretbox 解密并传给 signer，响应和 operation log 不返回明文
COS_STS_ENABLED=false 时 signer 返回 ErrDisabled，接口明确报 COS 临时凭证未启用
```

上传业务归属规则：

```text
uploadtoken 只签发临时凭证，不定义业务。
禁止新建无业务归属的 upload scene；folder 只能服务已经存在或正在迁移的业务实体。
业务模块先定义自己的表字段、状态、权限、操作日志和 REST contract，再接 upload token/client。
业务模块负责保存 object key/url 等引用；uploadtoken 不落业务引用、不创建“无主文件”事实源。
后续 AI agent avatar、chat attachment、rich text image 等都必须作为对应业务模块的一部分迁移，不能为了“上传页面”单独偷跑。
```

`internal/platform/storage/cos` 是唯一 COS STS 供应商边界：

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
cos-go-sdk-v5 暂不引入，避免把服务端上传/下载 client 伪装成 runtime token 的必要依赖
阿里云 OSS Go/JS SDK 不进入默认依赖；OSS 只保留配置事实源和未来 optional extension 入口
```

前端共享上传客户端：

```text
src/api/system/uploadToken.ts 定义 Go REST typed API
src/lib/upload/uploadClient.ts 只保留 cos-js-sdk-v5 动态加载
不再使用 legacyRequest、/api/getUploadToken、ali-oss、any/as any/Record<string, any>
OSS runtime 不是默认能力；请求到 OSS 必须显式错误，不能静默 fallback 到 COS
```
