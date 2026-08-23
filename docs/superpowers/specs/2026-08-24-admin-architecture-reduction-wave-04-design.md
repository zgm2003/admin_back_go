# Admin 架构减法 Wave 04 设计：运行与存储收口

**状态：已获用户确认，待执行计划**

## 1. 目标

在不改变现有 API、数据库表、任务类型、WebSocket JSON 协议和用户行为的前提下，收紧 Worker、Realtime 和 COS 直传的边界，删除已经没有业务意义的包装层，让个人开发者可以沿着一条直线理解运行链路。

本波不是重新实现 durable work。当前仓库已经包含 Asynq 任务注册、Worker 运行时、WebSocket 会话、MySQL 可恢复事件和 COS 临时凭证直传；本波只处理真实存在的重复层和结构漂移。

## 2. 当前事实

后端保留社区可读的调用链：

```text
route -> middleware -> handler -> service -> repository -> model
```

运行时边界保持：

```text
cmd/admin-api    -> internal/runtime/api
cmd/admin-worker -> internal/runtime/worker
```

MySQL 是业务和恢复事实源。Redis 的职责仅按配置分别承担缓存、Token、队列和 Realtime Pub/Sub；Realtime Redis 丢失时不能伪造或丢弃 MySQL 已提交的终态。

现有 Realtime 后端分层保留：

```text
internal/module/realtime/transport/admin
    -> internal/module/realtime
    -> internal/infra/realtime
```

现有 COS 上传语义保留：

```text
upload-token -> browser direct upload -> SDK success result
```

API 不接收大文件，也不在本波引入通用上传元数据表或 `complete` API。

## 3. 设计原则

### 3.1 Worker / Asynq

`internal/jobs.NewRegistry` 是唯一的可执行 Asynq 任务注册入口。任务类型、队列名、超时、重试次数、幂等 TTL 和 payload 处理继续由 `internal/infra/taskqueue.Registry` 统一提供。

数据库定时任务只由 `internal/module/crontask` 的数据库调度器和 Reconciler 管理。`internal/jobs` 不再保留一个“没有静态定义但仍被 Worker 调用”的调度适配器。

删除范围：

- `ScheduleRegistrar`；
- `ScheduledTaskDefinition`；
- `RegisterSchedules`；
- `registerScheduleDefinitions`；
- `scheduledEnqueueTask`；
- 仅服务于上述空适配器的测试替身。

保留范围：

- `jobs.Dependencies`；
- `jobs.NewRegistry`；
- 所有现有版本化任务类型；
- 现有 Asynq 队列、Redis DB、重试和超时策略；
- 当前 AI Context 任务注册，直到 Wave 06 按批准设计整体删除。

`internal/runtime/worker.go` 删除对空静态调度适配器的调用，Worker 启动顺序仍是资源、Provider、任务处理器、队列、数据库调度器。

### 3.2 Realtime / WebSocket

后端不修改事件名称、JSON 字段、Durability、sequence、resume、resync、事件保留窗口或权限主题规则。

API 进程拥有 WebSocket 升级、会话和本地连接管理；Worker 只拥有事件发布，不创建连接管理器。MySQL durable event 和 retention watermark 仍是恢复事实，Redis Pub/Sub 只做跨进程实时广播。

前端 Realtime 客户端属于跨页面运行工具，不应继续挂在目标架构中将被淘汰的 `src/modules`。本波将：

- 将 `src/modules/realtime` 迁移为 `src/utils/realtime`；
- 同步 `app/kernel`、`main.ts`、AI Chat、AI Run、WebSocket adapter 和测试导入；
- 删除旧目录，不保留第二套兼容实现；
- 保持公开导出、连接状态、断线重连、游标恢复、事件去重和错误通知不变。

后端 `transport` 目录明确保留，作为 HTTP/WebSocket 边界，不把 Handler 塞回业务根目录。

### 3.3 COS 上传

现有上传流程保持不变：

1. 前端请求 `POST /api/admin/v1/upload-tokens`；
2. 后端校验目录、文件名、文件大小和扩展名；
3. STS 策略只允许写入本次生成的对象 Key；
4. 浏览器直接调用 COS SDK 上传；
5. 只有 SDK 成功回调后，前端才返回 URL、Key 和 ETag。

本波将：

- 将 `src/lib/upload` 迁移为 `src/utils/upload`；
- 同步所有业务导入，删除旧上传目录；
- 为 upload-token、COS signer、前端取消/失败回调补充或收紧定向测试；
- 确认临时密钥不会进入日志或错误消息；
- 确认取消、网络错误和 SDK 回调异常不会被包装成成功结果。

明确不做：

- 不新增上传元数据表；
- 不新增 `init` / `complete` API；
- 不让 API 中转文件；
- 不改变 AI 附件请求结构、COS 对象 Key 规则或现有业务组件调用方式；
- 不迁移 `src/lib/http`、`src/lib/browser` 等其他历史目录。

## 4. 兼容性边界

以下内容必须保持兼容：

- 现有 HTTP 路径、请求和响应 JSON；
- Asynq 任务类型和 Redis DB；
- WebSocket 事件 JSON、连接状态和恢复语义；
- MySQL 表、字段和现有业务数据；
- COS upload-token 响应字段和前端上传结果；
- AI Chat、AI Run、通知、导出和上传组件的用户行为。

本波只允许改变源码导入路径和删除无调用方的内部适配器。若发现删除会影响生产调用方，必须停止并记录，不得用兼容兜底掩盖调用关系。

## 5. 验收标准

### Worker

- 生产代码不存在静态调度空适配器；
- 所有 Asynq 任务仍来自一个 `jobs.NewRegistry`；
- 数据库定时任务仍由 `crontask` Reconciler 管理；
- 任务类型、队列、重试和超时定向测试通过。

### Realtime

- 后端事件合同和生成文件无变更；
- 前端不存在 `src/modules/realtime` 或其导入；
- 握手、订阅、断线重连、游标恢复和去重定向测试通过；
- Redis 广播不可用时，MySQL 恢复语义仍成立。

### COS

- 前端不存在 `src/lib/upload` 或其导入；
- 无效上传参数在签发凭证前被拒绝；
- STS resource 只指向当前对象 Key；
- 上传失败、取消和空回调不会产生成功结果；
- 上传相关定向 Go/Vitest 测试通过。

## 6. 执行约束

- 直接在两个原始 `master` 工作区执行，不创建 worktree、不切换分支；
- 唯一 `work-ai` 只执行批准 Plan，不进入 Wave 05 或 Wave 06；
- 数据库不新增迁移、不运行 SQL；
- 不启动 `admin-dev`；
- 不运行全量测试、Playwright、`verify:frontend` 或其他长脚本；
- 只运行 Plan 中列出的定向短测试；
- 每个子批次提交后停止，等待用户人工黑盒验收；
- Wave 04 完成后更新总索引和交接记录，然后停止。

