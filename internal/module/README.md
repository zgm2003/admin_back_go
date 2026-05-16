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
captcha # go-captcha slide generation/verification boundary
auth # password login/refresh/logout HTTP boundary
user # current user/init RBAC read context
permission # RBAC menus/routes/buttons and permission management
role # role list/mutation and grant cache invalidation
mail # Tencent SES config/template/log and verify-code email sending
sms # Tencent Cloud SMS config/template/log and isolated test sending
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

`captcha` 当前只负责登录区滑块验证：

```text
GET /api/admin/v1/auth/captcha -> captcha.Generate
Redis key = CAPTCHA_REDIS_PREFIX + captcha_id
TTL = CAPTCHA_TTL
Verify 使用 Redis GETDEL，一次性消费
```

`auth` 当前开放 password login / refresh / logout：

```text
login-config -> auth.LoginConfig
login         -> captcha.Verify -> password check -> session.Create
refresh       -> session.Refresh
logout        -> session.Logout
```

不要把短信/邮箱验证码登录、自动注册、用户资料、RBAC 写路径一次性塞进 `auth`。这些要按阶段继续拆。

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
