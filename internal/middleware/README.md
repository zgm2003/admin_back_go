# Middleware Boundary

Gin middleware 放在 `internal/middleware`。

## 当前规则

```text
server/router.go 决定全局 middleware 顺序
middleware 包只处理横切关注点
module 不直接安装全局 middleware
service 不依赖 gin.Context
```

## 当前 middleware

```text
RequestID
AccessLog
CORS
AuthToken
PermissionCheck
OperationLog
```

`RequestID` 负责：

```text
读取或生成 X-Request-Id
写回响应头
写入 gin.Context
```

`AccessLog` 负责：

```text
请求结束后记录一条结构化日志
记录 request_id/method/path/status/latency_ms/client_ip
不记录 body
不记录完整 query string
不做业务审计
```

`CORS` 负责：

```text
处理浏览器跨域和 OPTIONS preflight
默认只允许本地 Vite 开发源
允许当前前端公共头：Authorization/platform/device-id/X-Trace-Id/X-Request-Id
暴露 X-Request-Id 给前端排障
```

禁止：

```text
手写 Access-Control-* header
用 CORS 代替 AuthToken/PermissionCheck
为了省事开启 AllowAllOrigins
```

`AuthToken` 负责：

```text
跳过明确 public path
解析 Authorization: Bearer <token>
把 token/platform/device-id/client-ip 传给认证服务
挂载认证服务返回的 AuthIdentity
```

`AuthToken` 不负责：

```text
签发 token
执行平台/设备/IP/单端登录策略
判断按钮权限
```

查询 Redis/DB session 属于 `internal/module/session`，现在通过注入的 authenticator 接入。平台策略和 RBAC 属于后续 auth/permission service。中间件只做 HTTP 边界。

当前平台安全策略已经接入 session authenticator：

```text
auth_platforms -> authplatform service -> session authenticator
```

`AuthToken` middleware 仍然不查表、不拼 Redis key、不判断 RBAC。它只把 header 输入传给 authenticator。

公开 refresh 路由必须跳过 `AuthToken`：

```text
POST /api/admin/v1/auth/refresh
POST /api/Users/refresh
```

logout 不跳过。logout 先认证当前 access token，再由 auth handler 调 session service 撤销。

`PermissionCheck` 负责：

```text
按显式 RouteKey(method + path) 查权限码
从 AuthToken 挂载的 AuthIdentity 读取 user/session/platform
把检查交给注入的 PermissionChecker
无规则的路由直接放行
```

`PermissionCheck` 不负责：

```text
扫描注解
反射 handler 名称
自己查 users/roles/permissions 表
自己拼 RBAC 缓存 key
```

`OperationLog` 负责：

```text
按显式 RouteKey(method + path) 查操作日志规则
在 handler 执行后收集 status/success/request_id/client_ip/latency
把记录交给注入的 OperationRecorder
记录失败不改变业务响应
```

`OperationLog` 当前只是 HTTP 边界骨架。后续写库必须放到 operationlog module/repository，不要塞进 middleware。

## 当前全局顺序

```text
Recovery
RequestID
AccessLog
CORS
AuthToken
PermissionCheck
OperationLog
Handler
```

每个 middleware 必须有测试和真实使用场景。

