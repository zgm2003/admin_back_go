# Platform Boundary

`internal/platform` 放外部资源适配层。

这里以后才允许放：

```text
database
redis
taskqueue
scheduler
storage
ai client
mail/sms client
```

## 规则

platform 只负责连接、配置和底层 client 封装。

业务含义不准写在 platform 里。

正确：

```text
platform/database 提供 DB handle
permission/repository 使用 DB 查询权限表
permission/service 决定 RBAC 业务规则
```

错误：

```text
platform/database 判断用户有没有按钮权限
platform/redis 拼 RBAC 业务规则
```

当前阶段只接 MySQL/Redis 的连接边界，不写任何业务查询、缓存 key 或 RBAC 规则。

## Config ownership

`internal/config` 只描述资源参数。

`internal/platform` 以后负责根据配置创建真实 client：

```text
config.MySQL -> platform/database
config.Redis -> platform/redis
config.Token -> auth/session service
config.Queue -> platform/taskqueue
config.Scheduler -> platform/scheduler
```

禁止在 config 包里打开连接。

## Database boundary

`internal/platform/database` 是唯一允许创建 MySQL/GORM client 的地方。

它只负责：

```text
打开 GORM MySQL client
拿到底层 *sql.DB
设置连接池
Ping
Close
```

它不负责：

```text
查询业务表
判断 RBAC
拼 SQL 条件
做事务业务决策
```

Repository 以后依赖 database client，不直接读取环境变量。

## Redis boundary

`internal/platform/redisclient` 是唯一允许创建 go-redis client 的地方。

它只负责：

```text
创建 Redis client
Ping
Close
```

它不负责：

```text
拼 token key
拼 RBAC buttonCodes key
决定缓存 TTL
序列化业务结构
```

缓存 key 规则属于对应模块/service，例如 session/auth/permission。

当前有两条 Redis 资源线：

```text
Resources.Redis      # 通用 Redis，使用 REDIS_DB，默认 0
Resources.TokenRedis # token/session Redis，使用 TOKEN_REDIS_DB，默认 2
```

它们共用 `REDIS_ADDR` / `REDIS_PASSWORD`，但 DB 分开。这个不是花活，是为了对齐旧 PHP 的 token 连接，避免把登录态和未来通用缓存/RBAC 缓存搅在一个 DB 里。

## Lifecycle ownership

资源生命周期属于 `internal/bootstrap.Resources`。

```text
bootstrap.NewResources 创建 platform client
bootstrap.App.Shutdown 关闭 platform client
module/repository 只使用注入进来的资源
```

禁止模块自己调用 `database.Open` 或 `redisclient.Open`。

## Queue / scheduler boundary

`internal/platform/taskqueue` 是唯一允许直接使用 Asynq 的地方。

它只负责：

```text
创建 Asynq client/server
映射 Redis DB / queue 权重 / retry / timeout
提供项目自己的 Task / Enqueuer / Mux
```

它不负责：

```text
决定业务 task type
解析业务 payload
直接调用业务 repository
```

`internal/platform/scheduler` 是唯一允许直接使用 gocron/v2 的地方。

规则：

```text
scheduler 注册定时触发
scheduler task 只投递 queue task
真正业务执行发生在 worker handler
unknown task type 必须显式失败，不允许静默吞掉
```
