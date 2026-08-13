# Module Boundary

`internal/module` 是业务能力区，不是随手丢代码的地方。当前长期结构是：

```text
internal/module/{capability}/
  service*.go       # 业务规则；不依赖 gin.Context
  repository*.go    # 数据访问；不做业务决策
  model*.go         # 数据库映射；不写查询方法
  dto*.go           # 能力内 DTO；跨平台响应差异优先放 presenter/request
  errors*.go        # 模块错误；返回 *apperror.Error
  jobs*.go          # 版本化 queue task 构造和 handler
  transport/{platform}/
    route*.go       # 注册该 platform HTTP 表面
    handler*.go     # 解析请求、校验、调用 service、返回 response
    request*.go     # HTTP request binding
    presenter*.go   # platform-specific response projection
```

单一 Admin HTTP 表面的已迁移模块采用：

```text
internal/module/{capability}/
  model.go
  request.go
  response.go
  repository.go
  service.go
  handler.go
  route.go
```

当同一能力真实存在 Admin/App/Canvas 两个以上 HTTP 表面时，才把平台差异拆到：

```text
internal/module/{capability}/transport/{platform}/
  request.go
  handler.go
  route.go
```

Service、Repository 和 Model 始终共享。不得为了未来可能出现的第二平台预建 transport 目录；也不得在第二平台出现后复制业务 Service。`systemsetting` 是 Wave 02 第一条新样板，其他模块在各自 Wave 前仍以现状为准，不能一次性机械搬目录。

## 每层职责

```text
route.go                             注册路径、中间件和 Handler 绑定
handler.go                           解析 HTTP，请求校验，调用 service，返回 response
request.go                           请求结构和基础格式校验
response.go                          data 内的业务响应结构
service.go                          业务规则、状态转换、事务编排
repository.go                       数据访问、锁、分页、条件查询
model.go                            数据库映射
jobs.go                             queue task type、payload、handler
```

## 当前能力家族

当前代码事实以目录和根仓库 `E:/admin_go/docs/status/current-status.md` 为准：

```text
system / systemsetting / systemlog
auth / auth_platform / profile / user / permission / role
mail / sms / notification / notification/task
uploadconfig / uploadtoken / export
operationlog / crontask / queuemonitor / realtime
payment / payment/wallet
ai/provider / ai/agent / ai/chat / ai/conversation / ai/message / ai/run / ai/tool / ai/knowledge / ai/contextengine / ai/image
```

平台不是 module。当前只有 admin/app 部分入口，不代表能力是长期 `admin-only`。新增 app/openapi/merchant/miniapp 入口时，默认在同一个 capability 下增加 `transport/{platform}`，不要复制 `appauth`、`adminuser`、`merchantupload` 这类平台命名业务模块。

## Context engineering ownership

`ai/contextengine` owns closed Context facts and deterministic policy only:
immutable Plan persistence, typed blocks, token budgeting, packing, hashing,
and Provider Attempt Plan evidence. Provider-specific JSON stays in
`internal/infra/ai`; the future `internal/infra/contextindex` adapter owns
Qdrant; runtime composition stays in `internal/platform/admin` and
`internal/runtime`.

Context retrieval is the only active chat context path. The legacy Knowledge
module and routes are retired; do not add a compatibility adapter around them.

## 禁止

```text
handler 直接查数据库
handler 直接访问 Redis
service 引入 gin.Context
repository 决定业务分支
model 写查询方法
无意义 interface
ServiceImpl
Manager/Factory 滥用
为了兼容猜测添加 silent fallback 字段
把旧 PHP/action API 当成 Go 模块规则
```

## 错误规则

```text
service 返回 *apperror.Error
handler 负责把 app error 映射成 HTTP response
middleware 鉴权失败时使用 response.Abort
可见错误必须走 i18n catalog key
```

禁止：

```text
service 调 c.JSON
service import gin
repository 返回 HTTP 状态码
model 返回业务错误文本
```

## Mail diagnostic evidence

`internal/module/mail` owns `mail_log_verification_codes` as a one-to-one child
of `mail_logs`. It persists encrypted verification-code diagnostic evidence with
the key ID and absolute expiry; Auth remains the only code issuance and
verification authority. Plaintext is projected only for an authorized mail-log
read, and its required audit record is payload-free. Repositories preserve the
restrict parent relation and do not add status, soft-delete, or update lifecycle
fields to this immutable child.
