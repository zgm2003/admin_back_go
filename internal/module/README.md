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

模块根目录不注册 HTTP 表面。`route.go`、`handler.go`、`app_handler.go`、`platform_handler.go`、`platform_route.go` 这类 HTTP 文件必须落在 `transport/{platform}`。

## 每层职责

```text
transport/{platform}/route.go       注册路由，只绑定 handler
transport/{platform}/handler.go     解析 HTTP，请求校验，调用 service，返回 response
transport/{platform}/request.go     只放 HTTP 入参结构和 binding 规则
transport/{platform}/presenter.go   只做该平台响应投影，不写业务状态机
service.go                          业务规则、状态转换、事务编排
repository.go                       数据访问、锁、分页、条件查询
model.go                            数据库映射
dto.go                              能力内数据结构
errors.go                           模块错误 key/code
jobs.go                             queue task type、payload、handler
```

## 当前能力家族

当前代码事实以目录和 `docs/status/current-status.md` 为准：

```text
system / systemsetting / systemlog
auth / auth_platform / profile / user / permission / role
mail / sms / notification / notification/task
uploadconfig / uploadtoken / export / clientversion
operationlog / crontask / queuemonitor / realtime
payment / payment/wallet
ai/provider / ai/agent / ai/chat / ai/conversation / ai/message / ai/run / ai/tool / ai/knowledge / ai/image
```

平台不是 module。当前只有 admin/app 部分入口，不代表能力是长期 `admin-only`。新增 app/openapi/merchant/miniapp 入口时，默认在同一个 capability 下增加 `transport/{platform}`，不要复制 `appauth`、`adminuser`、`merchantupload` 这类平台命名业务模块。

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
