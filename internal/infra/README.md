# Infra Boundary

`internal/infra` 放外部资源适配层。

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

infra 只负责连接、配置和底层 client 封装。

业务含义不准写在 infra 里。

正确：

```text
infra/database 提供 DB handle
permission/repository 使用 DB 查询权限表
permission/service 决定 RBAC 业务规则
```

错误：

```text
infra/database 判断用户有没有按钮权限
infra/redis 拼 RBAC 业务规则
```

当前阶段只接 MySQL/Redis 的连接边界，不写任何业务查询、缓存 key 或 RBAC 规则。

## Config ownership

`internal/config` 只描述资源参数。

`internal/infra` 以后负责根据配置创建真实 client：

```text
config.MySQL -> infra/database
config.Redis -> infra/redis
config.Token -> auth/session service
config.Queue -> infra/taskqueue
config.Scheduler -> infra/scheduler
```

禁止在 config 包里打开连接。

## Database boundary

`internal/infra/database` 是唯一允许创建 MySQL/GORM client 的地方。

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

`internal/infra/redisclient` 是唯一允许创建 go-redis client 的地方。

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

当前 API 有三条 Redis 资源线，Worker 只打开实际需要的通用与队列资源：

```text
Resources.Redis      # 通用 Redis，使用 REDIS_DB，默认 0
Resources.TokenRedis # token/session Redis，使用 TOKEN_REDIS_DB，默认 2
Resources.QueueRedis # queue Redis，使用 QUEUE_REDIS_DB，默认 3
```

它们共用 `REDIS_ADDR` / `REDIS_PASSWORD`，但 DB 分开。这个不是花活，是为了对齐旧 PHP 的 token 连接，避免把登录态和未来通用缓存/RBAC 缓存搅在一个 DB 里。

## Root-secret derivations

`APP_SECRET` is the sole current root. `APP_SECRET_PREVIOUS` is optional and
contains at most one prior root during the documented rotation; it is not a
separate deployment secret. `internal/infra/secretkey` derives the live JWT
signing, refresh-token pepper, secretbox, and mail diagnostic purposes. The mail
diagnostic box encrypts evidence for `internal/module/mail`; it never owns mail
business policy, exposes plaintext to infrastructure logs, or replaces Auth's
verification state.

## Lifecycle ownership

资源生命周期属于 `internal/runtime.Resources`。

```text
runtime.OpenResources 按进程能力创建、Ping 并原子发布 infra client
runtime.Resources.Close 按创建顺序反向关闭 infra client
module/repository 只使用注入进来的资源
```

禁止模块自己调用 `database.Open` 或 `redisclient.Open`。

## Queue / scheduler boundary

`internal/infra/taskqueue` 是唯一允许直接使用 Asynq 的地方。

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

`internal/infra/scheduler` 是唯一允许直接使用 gocron/v2 的地方。

规则：

```text
scheduler 注册定时触发
scheduler task 只投递 queue task
真正业务执行发生在 worker handler
unknown task type 必须显式失败，不允许静默吞掉
```
