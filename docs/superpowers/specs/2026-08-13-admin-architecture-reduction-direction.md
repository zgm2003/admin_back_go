# Admin 平台架构减法中心方向书

> 日期：2026-08-13
>
> 状态：数据库基线减法已完成；架构减法设计已完成，等待用户审阅；尚未开始架构迁移
>
> 适用仓库：`E:\admin\admin_back_go`、`E:\admin\admin_front_ts`
>
> 文档地位：本轮架构减法的唯一中心设计。后续规格、计划、代码和验收必须引用并服从本文；若需改变方向，先修改本文并重新确认。

## 1. 为什么必须重构

项目的核心业务有价值，真正的问题是多次重构后出现了重复架构：同一个接口同时存在后端 DTO、路由合同、OpenAPI、前端生成类型、运行时 Schema、Adapter 和页面类型；同一个权限事实又散落在数据库、路由元数据、生成文件和前端运行时注册器中。

结果不是系统能力更强，而是：

- 开发者无法从路由顺着代码读到数据库；
- 普通 CRUD 需要修改过多文件；
- 同一返回结构在多处重复声明并发生漂移；
- 一次性迁移工具、验证脚本和过程文档长期留在主动架构中；
- 前后端日常开发依赖生成器、Registry、Kernel 和 Adapter；
- 业务行为越来越依赖 AI 理解，项目所有者反而难以维护自己的项目。

这是真实维护问题，不是代码风格争论。本次重构的目标不是换一套更高级的架构，而是恢复一条中国开发者和 Go/Vue 社区都能直接理解的业务调用链。

## 2. 总体目标

采用：

```text
模块化单体
+ 经典分层架构
+ 显式依赖注入
+ 事件驱动实时层
```

后端日常调用链固定为：

```text
route
-> middleware
-> handler
-> service
-> repository
-> model
```

前端日常调用链固定为：

```text
views
-> api
-> request
-> 后端
```

基础设施事实边界固定为：

```text
业务事实       -> MySQL
文件内容       -> COS
向量派生索引   -> Qdrant
缓存/会话/队列 -> Redis
实时通知       -> WebSocket + Redis Pub/Sub
业务规则       -> Service
数据库访问     -> Repository
外部 SDK       -> infra 客户端
```

## 3. 不可破坏的兼容边界

这是原仓库逐模块迁移，不新建项目，不建立长期 `v2` 双轨。以下内容必须保留：

- 现有数据库业务数据、表字段语义和必要索引；
- 现有 API 路径、请求字段和响应协议；
- 菜单、权限码、角色关系和用户操作习惯；
- 登录、会话、RBAC、支付、上传、WebSocket、AI 和上下文工程能力；
- 支付验签、幂等、钱包流水和事务边界；
- AI Conversation、Message、Run、Attachment 的持久化事实；
- Worker 任务幂等、重试、终态收口和恢复；
- COS 对象访问授权；
- MySQL 真相与 Redis/Qdrant 派生状态的边界；
- 中英文用户提示和稳定的程序错误码。

删除表、字段、接口、功能或数据前，必须给出引用和业务证据并获得用户批准。每迁移一个模块，先验收新实现，再删除该模块旧实现。

## 4. 后端目标结构

### 4.1 顶层目录

```text
admin_back_go/
├── cmd/
│   ├── admin-api/
│   ├── admin-worker/
│   └── admin-cli/
├── internal/
│   ├── config/
│   ├── middleware/
│   ├── module/
│   ├── infra/
│   ├── realtime/
│   └── shared/
├── database/
├── deploy/
└── scripts/
```

`module` 保存业务，`infra` 保存 MySQL、Redis、COS、AI/支付供应商等外部实现，`shared` 只保存真正跨业务且稳定的公共代码。不得用 `shared`、`common` 或 `utils` 隐藏业务归属。

### 4.2 单个业务模块

```text
internal/module/systemsetting/
├── model.go
├── request.go
├── response.go
├── repository.go
├── service.go
├── handler.go
└── route.go
```

职责固定：

| 文件 | 唯一职责 |
|---|---|
| `model.go` | 数据表映射 |
| `request.go` | 请求结构与基础格式校验 |
| `response.go` | `data` 内的业务响应结构 |
| `repository.go` | MySQL/Redis 数据访问 |
| `service.go` | 业务规则、复杂校验和事务编排 |
| `handler.go` | 解析 HTTP 输入、调用 Service、写统一响应 |
| `route.go` | 路径、中间件和 Handler 绑定 |

不以文件数量作为简单标准。这里保留社区熟悉的文件名，是为了让任何人都能预测代码位置；真正要删除的是重复合同、运行时编译、平台内核和纯转发层。

### 4.3 依赖规则

```text
handler -> service -> repository -> model
```

- Handler 不直接访问 MySQL、Redis、COS 或供应商 SDK。
- Service 不依赖 `gin.Context`。
- Repository 不包含 HTTP 响应和页面语义。
- 业务模块不自行创建数据库、Redis 或第三方客户端。
- 依赖由 `main.go` 创建并显式传入。
- 接口只在确实存在替代实现或测试边界时创建，不为单实现预建抽象。

## 5. 公共响应与错误协议

现有协议保持兼容。

成功：

```json
{
  "code": 0,
  "data": {},
  "msg": "ok"
}
```

失败：

```json
{
  "code": 100,
  "data": {},
  "msg": "余额不足",
  "error": {
    "code": "ai.balance.insufficient",
    "category": "validation",
    "retryable": false,
    "request_id": "req_xxx",
    "trace_id": "trace_xxx"
  }
}
```

含义固定：

- `msg` 面向用户，由中英文目录生成；
- `error.code` 面向程序，是稳定的业务错误标识；
- `category` 供全局请求层分类；
- `retryable` 供 AI、支付、上传等流程判断是否可重试；
- `request_id`、`trace_id` 用于日志定位。

公共响应实现只负责外层协议。业务模块只定义 `data` 内部结构，不把所有 DTO 堆入公共包。不得用默认空值吞掉理论上不应为空的数据。

## 6. HTTP 合同减法

当前复杂链路：

```text
后端 DTO
-> route HTTPContract
-> OpenAPI
-> 前端 generated operation
-> runtime schema
-> API adapter
-> 页面
```

目标链路：

```text
后端 request/response + handler
-> 统一 JSON 响应
-> 前端 api/*.ts
-> views
```

规则：

- 后端 `request.go`、`response.go` 是人可读的接口定义；
- 前端业务类型放在对应 `api/*.ts` 中；
- 删除前端 generated operation 和 runtime schema compiler 的日常依赖；
- OpenAPI 可作为文档或检查工具保留，但不能阻塞普通 CRUD；
- 核心接口用短集成测试验证真实 JSON；
- 不再维护额外的 route HTTPContract 业务响应副本；
- 后端返回结构与前端解包必须在同一变更中完成。

## 7. RBAC 与操作日志

数据库继续采用标准关系：

```text
users
-> roles
-> role_permissions
-> permissions
```

每个管理接口必须显式属于以下一种：

```text
Public
Authenticated
Permission(code)
```

缺少权限规则时不得默认放行。路由直接表达权限和操作日志：

```go
router.POST(
    "/system-settings",
    middleware.Auth(),
    middleware.Permission("system_setting_add"),
    middleware.OperationLog("系统设置", "新增配置"),
    handler.Create,
)
```

删除通过 `AuditDecision`、route registry 和规则编译间接生成权限或审计行为的链路。

前端当前用户接口返回：

```json
{
  "menus": [],
  "button_codes": [
    "system_setting_add",
    "system_setting_edit"
  ]
}
```

前端只负责页面展示和按钮控制，后端中间件始终是真正的授权边界。`permissions.platform` 保留，以支持单后端多前端。

## 8. 单后端多前端

明确保留三个入口：

```text
/api/admin/v1
/api/app/v1
/api/canvas/v1
```

同一业务的多个前端共享 Service、Repository 和 Model，只分离平台 Handler：

```text
internal/module/user/
├── model.go
├── repository.go
├── service.go
└── handler/
    ├── admin/
    ├── app/
    └── canvas/
```

```text
Admin Handler  ┐
App Handler    ├-> Service -> Repository -> Model
Canvas Handler ┘
```

平台差异停留在对应 Handler。Token 绑定平台，不能跨平台调用。新增前端时新增显式入口，不建设动态万能平台框架。

## 9. 配置与启动装配

采用单一 `config.Config`，环境变量是唯一配置来源：

```text
.env / admin-go.env
-> config.Load()
-> Config{Server, MySQL, Redis, COS, Mail, AI, Payment...}
-> main.go 显式装配
```

规则：

- 保留 `deploy/docker-first/admin-go.env.example` 作为字段模板；
- 敏感值只存在未提交的 env 或部署密钥中；
- 配置只在启动时读取一次；
- 必需配置缺失或格式错误时启动立即失败，并报告明确字段；
- 业务模块不得直接调用 `os.Getenv`；
- 不保留 YAML 与 env 双重优先级；
- 不使用空字符串或默认对象掩盖错误配置。

API 启动链：

```text
cmd/admin-api/main.go
-> config.Load()
-> MySQL / Redis / Qdrant / COS / Provider client
-> repository / service / handler
-> middleware / routes
-> HTTP server
```

Worker 启动链：

```text
cmd/admin-worker/main.go
-> config.Load()
-> MySQL / Redis / Provider client
-> jobs.Register()
-> worker.Run()
```

保留健康检查、Readiness 和优雅退出，删除只做依赖转发的 `graph`、`platform kernel` 等装配包装。

## 10. Redis 角色与未来集群

MySQL 永远是业务真相。Redis 采用“核心依赖必需、缓存可回源、不做内存兜底”。下表是本轮架构迁移后的目标分配；其中 DB 2 和 DB 3 已存在，DB 1 将在迁移 realtime 时建立。

本地单机 Redis 使用逻辑数据库隔离：

| 逻辑库 | 客户端角色 | 当前业务 |
|---|---|---|
| DB 0 | Cache Redis | 系统设置、权限、地址字典、验证码、限流、Scheduler Lock |
| DB 1 | Realtime Redis | WebSocket Pub/Sub、AI 取消信号 |
| DB 2 | Token Redis | Session、单端登录指针、Browser Grant |
| DB 3 | Queue Redis | Asynq 任务和队列监控 |

规则：

- 普通缓存 Redis 故障时允许回源 MySQL；
- Session、Queue 等核心依赖不可用时返回明确错误，不建立进程内内存兜底；
- 实时广播失败不改变已提交的 MySQL 业务终态；
- 不跨 Redis DB 做事务；
- 每类客户端使用明确 key 前缀；
- Readiness 检查已配置的核心 Redis 角色。

Redis 多 DB 只是本地逻辑隔离，不是集群方案。Redis Cluster 通常只支持 DB 0。未来切换集群时使用四类明确连接：

```text
CACHE_REDIS_ADDR
REALTIME_REDIS_ADDR
TOKEN_REDIS_ADDR
QUEUE_REDIS_ADDR
```

各集群连接均使用 DB 0。业务代码只依赖角色客户端，因此不需要修改业务逻辑。

## 11. cmd 与 scripts 减法

### 11.1 cmd

最终只保留：

```text
cmd/
├── admin-api/
├── admin-worker/
└── admin-cli/
```

`admin-cli` 只收纳确有人工运维价值的命令，例如：

```text
admin-cli create-admin
admin-cli mail-diagnostic-rekey
admin-cli check-payment-certs
```

处理规则：

- `admin-contract` 随旧合同链删除；
- `ai-context-preflight` 属于已完成切换的一次性工具，引用清理后删除；
- `admin-db` 的保留命令迁入 `admin-cli`；
- 一次性迁移命令在任务完成并留有恢复证据后删除；
- 不再为临时需求创建长期 `cmd/xxx`。

### 11.2 scripts

最终公开脚本只保留：

```text
scripts/
├── admin-dev.ps1
├── database.ps1
├── docker.ps1
├── install-shortcuts.ps1
└── internal/
    └── common.ps1
```

规则：

- `admin-dev.ps1` 负责本地前后端开发启动；
- `database.ps1` 负责 `init/reset/migrate/check`；
- `docker.ps1` 只负责 MySQL、Redis、Qdrant 等状态服务；
- 合同生成、历史 cutover、browser-only、长 smoke 和未上线项目的发布平台脚本删除；
- 标准 `go test`、`go vet`、`go build` 不再被多层 PowerShell 套娃；
- 支付证书等人工维护能力进入 `admin-cli`；
- 删除脚本前先清理 README、Runbook、测试和其他脚本引用。

## 12. Worker 与任务队列

保留独立 `admin-worker` 和 Asynq，任务采用显式注册：

```text
cmd/admin-worker/main.go
-> config.Load()
-> 连接 MySQL / Queue Redis
-> jobs.Register()
-> worker.Run()
```

每个业务模块只提供自己的任务处理函数。任务名、重试次数和队列归属在一个明确注册位置可见。不经过通用任务图、动态平台注册或多层运行时包装。

需要数据库与入队强一致时，根据真实故障需求使用 Outbox；不得把 Redis 队列假装成 MySQL 事务。

## 13. WebSocket 与实时通信

保留 WebSocket，收口为一个 `realtime` 包：

```text
WebSocket Route
-> Auth
-> Connection Manager
-> Event Publish / Subscribe
-> Frontend
```

规则：

- MySQL 中的消息、运行记录和最终状态是真相；
- Redis Pub/Sub 只负责多节点广播；
- 单节点允许本地直接分发；
- 断线后通过 HTTP 消息/运行记录接口从 MySQL 恢复；
- AI 流式事件和普通通知共用 realtime 基础设施，但事件类型明确；
- 删除动态事件注册、复杂合同生成和多层 realtime 包装；
- 鉴权失败、非法事件和 Redis 故障必须记录，不能静默吞掉；
- Redis 广播故障不得覆盖或回滚已落库的终态。

## 14. 文件上传与 COS

采用 COS 直传/预签名上传，API 不中转文件内容：

```text
Frontend
-> POST /uploads/init
-> Backend 校验用户、文件类型和大小，生成 object_key 与短期签名
-> Frontend 直传 COS
-> POST /uploads/complete
-> Backend HEAD COS 校验真实对象
-> MySQL 写入文件元数据和完成状态
```

边界：

- COS 保存文件内容；
- MySQL 保存文件元数据、归属、业务关联和状态；
- 上传 Service 统一处理权限和状态；
- 下载、预览、AI 读取前必须重新校验权限并生成短期签名 URL；
- AI 消息只引用 `asset_id`，不存文件二进制；
- 业务代码不得直接散落调用 COS SDK；
- 未完成上传和 COS 中孤立对象由明确清理任务处理；
- 不为上传再建设通用文件平台内核。

## 15. AI 业务域

AI 是核心复杂业务，允许内部按真实职责拆包，但不得再建设“平台内的平台”：

```text
internal/module/ai/
├── provider/       供应商与模型业务配置
├── agent/          智能体配置
├── conversation/   会话与消息
├── run/            运行记录与流式终态
├── asset/          附件关联与授权
├── context/        上下文工程
└── billing/        用量计算与扣费编排
```

统一调用链：

```text
AI Handler
-> AI Service
-> Repository / Provider Client / Storage / Wallet Service
```

规则：

- `ai/provider` 保存供应商、官方模型、模型能力和场景等业务事实；
- OpenAI-compatible 等 SDK/HTTP 客户端放 `infra/ai`；
- `conversation` 持有会话和消息事实；
- `run` 持有一次模型执行、状态和终态收口；
- `asset` 只管理消息与已授权文件的关联；
- `context` 管理空间、文档、版本、检索和派生索引；
- `billing` 计算 AI 用量，但实际余额变更必须调用钱包 Service；
- 上下文工程和 Embedding 可热插拔，关闭或上游不可用时不得破坏普通聊天；
- MySQL 保存上下文文档与版本事实，Qdrant 只保存可重建的向量索引；
- 删除通用 Kernel、Feature Registry、万能 Workflow 和二次 DTO 生成器；
- AI 内部只有出现真实独立生命周期时才继续拆包。

## 16. 支付与钱包

支付作为独立业务域，不塞入 AI：

```text
internal/module/payment/
├── order/       订单和支付状态
├── wallet/      余额、扣款、退款和流水
├── provider/    支付供应商业务配置
└── handler/     创建订单、回调、查询
```

```text
Payment Handler
-> Payment Service
-> MySQL Repository
-> infra/payment Provider Client
```

规则：

- 创建订单、扣款和退款由 MySQL 事务控制；
- 支付宝回调先验签，再按订单号幂等更新；
- 重复回调不能重复入账；
- AI 扣费只调用 `wallet.Service`；
- 支付 SDK 和证书解析放 `infra/payment`；
- 支付成功、余额变化、AI 扣费和退款保留可追溯流水；
- 不建设万能支付策略引擎，只为真实供应商写适配器。

## 17. 前端目标结构

```text
src/
├── api/
├── assets/
├── components/
├── hooks/
├── layout/
├── locales/
├── router/
├── store/
├── styles/
├── types/
├── utils/
├── views/
├── App.vue
├── main.ts
└── permission.ts
```

规则：

- 普通页面只走 `views -> api -> request -> 后端`；
- `api/<business>.ts` 同时保存该业务请求函数和 TypeScript 类型；
- Store 只保存跨页面状态，不包装所有接口；
- Hook 只抽取真实复用的有状态逻辑；
- Component 只做展示和交互，不隐藏接口调用链；
- `app/adapters/features/lib/modules/shared` 不再成为普通 CRUD 必经层；
- 删除 AppKernel、RuntimeRouteRegistry 和 generated routes/contracts 的日常依赖；
- 使用 Element Plus 和现有 App 组件的默认能力，避免大量 `:deep()` 覆盖；
- 不用兜底值掩盖后端字段缺失，接口不符必须明确报错。

### 17.1 permission.ts

`permission.ts` 只负责：

- 检查登录状态；
- 未登录跳转登录页；
- 首次加载当前用户、菜单和按钮权限；
- 注册动态路由；
- 页面访问控制。

数据流：

```text
permission.ts
-> 获取当前用户
-> 根据 menus 注册路由
-> store 保存 button_codes
-> userStore.can(code)
```

真正 RBAC 始终由后端中间件负责。

### 17.2 全局请求错误

根请求层统一处理公共响应：

- `code == 0` 返回 `data`；
- `401` 清理登录态并跳转登录；
- `403/404` 按固定交互处理；
- 其他业务错误使用后端 `msg` 全局通知；
- 网络错误、超时和非法响应显示明确错误；
- API 方法不得重复吞错或重复弹窗；
- 响应结构不匹配时报告真实结构错误，不能归一化成空数组或空对象。

## 18. 数据库初始化基线

项目尚未上线且处于纯本地开发，允许把旧迁移链收口为初始化基线，并从今天开始重新做增量加法。

当前数据库减法已经完成。仓库中的目标目录为：

```text
database/
├── schema.sql
├── seed.sql
├── reference/
│   └── address.sql
├── migrations/
├── baseline.json
└── README.md
```

职责：

- `schema.sql` 是当前完整结构的唯一初始化事实；
- `seed.sql` 只保存系统运行必需的最小数据；
- `reference/address.sql` 保存用户资料依赖的公开地址字典，不属于业务历史；
- `migrations/` 只保存新基线之后的短 forward migration；
- `baseline.json` 保存可恢复来源、文件哈希和结构计数；
- 应用启动不自动迁移；
- `scripts/database.ps1` 是唯一人工数据库入口。

当前已验证的最小 Seed 保留：

- 权限目录、必要角色和角色权限；
- Admin 认证平台；
- 四条真实默认系统设置，包括默认头像；
- 四条邮件验证码模板；
- 六条定时任务；
- AI 工具目录；
- 新迁移基线账本。

官方模型目录及其类型、能力和基础价格的权威来源是 `internal/module/ai/officialmodel/catalog/official_models_v1.json`，不在 SQL 中再复制一份。Provider、Provider Model、智能体和上下文配置属于用户配置，也不进入初始化 Seed。

Seed 不保留：

- 用户和管理员密码；
- Session、聊天、AI Run、上传记录；
- 支付订单和运行流水；
- 操作日志和队列历史；
- 私有 Provider 密钥、支付证书和 COS 密钥；
- Qdrant 向量和本地临时数据。

管理员由 `admin-cli create-admin` 创建，密码不得硬编码在 SQL 或命令参数中。

重建顺序：

```text
停止 API/Worker
-> 备份 MySQL
-> schema.sql + seed.sql 重建 MySQL
-> 清理本项目 Redis DB 0/1/2/3
-> 清理本项目 Qdrant 派生集合
-> 创建本地管理员
-> 启动 API/Worker
-> 验证 /ready 和关键页面
```

### 18.1 已完成的数据库基线证据

以下是已经完成的事实，不属于后续架构计划，不得重复执行：

```text
Baseline version: 202608130001
Recovery tag: pre-database-baseline-20260813
schema.sql SHA256: b4884de66b62700e47fe2481769012ef4f28ddeffaf7fb6e39e19bbe5fea4033
seed.sql SHA256: 772a58baba0acfdf9593f2545021b94e9c025484795a42c06c7d22690eea99c7
address.sql SHA256: af82e6ebe0120afebf10bdf13a7cf6ebc092eaa6388a2f47a398ed46e0f72bc2
```

完整恢复快照仍存在：

```text
C:\Users\20931\AppData\Local\Temp\admin-db-baseline\admin-current-full-20260813-092521.sql
Size: 15936795 bytes
SHA256: c2b73e639892c3c1cd274443758738ce153d7c3eef7b8fdad67545453a018e50
```

仓库当前已经在基线之后产生 forward migration，例如 `202608130002_restore_mail_templates.sql`。后续只继续新增短迁移，不能重新生成基线或修改已经执行的迁移文件。

完整 dump 只是恢复材料，不能原样作为初始化 SQL。架构迁移只能消费当前 `schema.sql + reference/address.sql + seed.sql + migrations`，不得回退到 Atlas/Reconciliation 链。

## 19. 测试策略

采用“小而快的定向测试 + 核心模块集成测试 + 人工终验”。

普通 CRUD：

```text
Service     -> 业务规则
Repository  -> 关键 SQL 和约束
Handler     -> 请求解析和真实 JSON
Frontend API-> 请求参数和响应解包
```

核心模块增加 MySQL/Redis 集成测试：

- 登录、Session 和 RBAC；
- 支付、钱包、回调幂等；
- AI 对话、附件、运行终态和扣费；
- WebSocket 断线恢复和多节点广播；
- Worker 幂等、重试和恢复；
- COS 权限和对象完成确认；
- 上下文工程开关、索引降级和 Qdrant 重建。

删除：

- 几千行 PowerShell Smoke；
- 测试脚本自己的大型测试；
- generated contract 与 runtime schema 的重复测试；
- 只检查固定源码字符串和文件路径的架构测试；
- 为 Getter、DTO 或框架行为编写的低价值测试；
- 默认运行十几分钟的套娃验证入口。

日常只运行受影响模块的短命令。全量长测试、Playwright 和人工全流程均不作为每个迁移步骤的默认门禁。最终由用户人工验收完整业务。

## 20. 迁移顺序

### 阶段 0：建立恢复点

- 保存前后端当前提交和工作区差异；
- 保留 MySQL 完整备份及哈希；
- 记录 Redis/Qdrant 清理范围；
- 禁止新增 Kernel、Registry、generated contract 和一次性长期脚本。

### 阶段 1：基础骨架

- 收口 `config.Config`；
- 简化 `main.go` 依赖装配；
- 固定公共响应和错误协议；
- 建立四类 Redis 角色客户端；
- 收口 `cmd` 和 `scripts` 的新入口；
- 建立后端模块和前端 API 的标准模板。

### 阶段 2：系统设置样板

后端迁移为：

```text
route -> middleware -> handler -> service -> repository -> model
```

前端迁移为：

```text
view -> api -> request -> 后端
```

系统设置作为首个样板，因为它同时覆盖 CRUD、RBAC、操作日志、Redis 缓存、双语言、前端表格和表单。必须保留默认头像等全部真实 Seed 数据。

### 阶段 3：后台基础业务

按样板迁移：用户、角色、权限、邮件、短信、日志和上传配置。每次只迁移一个业务模块。

### 阶段 4：运行能力

迁移 Worker、Asynq、WebSocket、Realtime 和 COS 上传，保持已有持久化和恢复语义。

### 阶段 5：支付

迁移订单、钱包、支付宝和流水，先锁定金额、事务、验签和幂等测试。

### 阶段 6：AI

最后迁移供应商、智能体、会话、附件、运行记录、上下文工程和扣费。AI 每个子域单独验收，不进行一次性整体重写。

### 阶段 7：删除旧架构

只有所有消费者迁移并通过验收后，才删除：

- generated contracts/operations/views/permissions；
- runtime schema compiler；
- AppKernel、RuntimeRouteRegistry、Workflow 和 Adapter 套娃；
- route HTTPContract 业务响应副本；
- 旧 `cmd`、历史脚本、空目录和失效文档；
- 只服务于旧架构的测试。

## 21. 单模块迁移流程

每个模块严格执行：

```text
1. 记录现有 API、数据、权限和页面行为
2. 写受影响范围和兼容清单
3. 按中心方向书实现新结构
4. 运行该模块短测试
5. 用户人工验收
6. 删除该模块旧实现
7. 更新迁移记录后进入下一个模块
```

如果新实现需要兼容旧数据，只在迁移边界做一次显式转换，不在系统各处长期保留双写、双读和默认兜底。

## 22. 完成标准

架构减法完成后必须满足：

- 开发者能从 Route 顺着代码直接读到 Model；
- 前端页面能顺着 API 文件直接看到请求和响应类型；
- 普通 CRUD 不再依赖生成合同、Kernel、Registry 或 Adapter；
- API 只有一份人可读请求/响应定义；
- RBAC 和操作日志在路由处显式可见；
- `cmd` 只剩 API、Worker、CLI；
- `scripts` 只剩四个公开入口和少量内部函数；
- Redis 的缓存、实时、Token、队列角色清晰，并可迁移到 Cluster；
- COS 直传不占用 API 文件带宽；
- WebSocket 断线后能从 MySQL 恢复；
- 支付和 AI 扣费具有明确事务与流水边界；
- 上下文工程关闭或不可用时不影响普通 AI 对话；
- 空数据库可以通过 `schema.sql + seed.sql` 建立完整系统基础数据；
- 所有旧架构删除都有引用盘点、恢复点和用户验收证据。

## 23. 后续项目可直接复用的架构要求

```text
请使用社区习惯的模块化单体架构，不做微服务，不做过度抽象。

后端调用链固定为：
路由 -> 中间件 -> Handler -> Service -> Repository -> Model。

依赖由 main.go 统一装配，业务模块不自行创建数据库、Redis 或第三方客户端。

MySQL 是业务事实源；Redis 只做缓存、会话、队列、锁和实时广播；
WebSocket 断线后通过 HTTP 从 MySQL 恢复；文件采用 COS 预签名直传；
前端调用链固定为：页面 -> api -> request -> 后端。

每个模块按业务名组织，让开发者能从路由一路读到数据库。
不创建通用 Kernel、动态 Registry、重复合同或一次性长期脚本。
```

## 24. 最终原则

```text
好品味优于技巧。
数据结构优于代码。
显式调用优于动态注册。
唯一事实源优于重复合同。
修复来源优于默认兜底。
兼容性优于理论优雅。
能被项目所有者理解的架构，才是可维护的架构。
```
