# Module Boundary

`internal/module` 是业务模块区，不是随手丢代码的地方。

## 模块文件规则

一个模块最多包含：

```text
route.go
handler.go
service.go
repository.go
model.go
dto.go
errors.go
```

## 每层职责

```text
route.go       注册路由，只绑定 handler
handler.go     解析 HTTP，请求校验，调用 service，返回 response
service.go     业务规则，不依赖 gin.Context
repository.go  数据访问，不写业务决策
model.go       数据库映射，不写业务方法
dto.go         请求/响应结构
errors.go      模块错误
```

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
```

## 当前阶段

当前已建立：

```text
system   # health / ready / ping
session  # token hash + TokenRedis/MySQL session lookup
authplatform # auth_platforms read path for session policy
auth # refresh/logout HTTP boundary
```

RBAC 模块要等现有 PHP 契约文档固定后再迁移。

`authplatform` 当前只给认证链路提供平台策略：

```text
bind_platform
bind_device
bind_ip
single_session
max_sessions
allow_register
```

它不是 RBAC PermissionCheck，也不负责菜单/按钮权限。

`auth` 当前只开放 refresh/logout：

```text
refresh -> session.Refresh
logout  -> session.Logout
```

不要把密码登录、验证码、用户资料、RBAC init 一次性塞进 `auth`。这些要按阶段继续拆。

## 错误规则

```text
service 返回 *apperror.Error
handler 负责把 app error 映射成 HTTP response
middleware 鉴权失败时使用 response.Abort
```

禁止：

```text
service 调 c.JSON
service import gin
repository 返回 HTTP 状态码
model 返回业务错误文本
```
