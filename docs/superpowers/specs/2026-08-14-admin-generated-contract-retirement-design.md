# Admin 生成合同体系彻底退役设计

> 日期：2026-08-14
>
> 状态：用户已批准方案 C；本文是 Wave 07 的专项设计依据，尚未开始物理删除
>
> 适用仓库：`E:\admin\admin_back_go`、`E:\admin\admin_front_ts`
>
> 上位方向：`docs/superpowers/specs/2026-08-13-admin-architecture-reduction-direction.md`

## 1. 需求判断

【需求判断】

这是真问题。当前项目只服务自有前后端，不发布公共 API、SDK、Swagger 或第三方接口文档，却维护了一套比业务代码更难理解的生成合同平台。

【核心问题】

同一个 HTTP 接口被重复描述为：

```text
后端 request/response DTO
-> route HTTPContract
-> OpenAPI
-> permissions/views/manifest bundle
-> 前端合同镜像
-> generated operations/types/views/permissions
-> runtime schema
-> 业务 API
-> 页面
```

任何一处不同步都会产生假失败、哈希漂移或运行时解包错误。真正需要保留的是 API 行为、RBAC、菜单和实时协议，不是生成链本身。

【复杂度检查】

当前已确认的主要生成物规模：

| 范围 | 规模 |
|---|---:|
| 后端 `contracts/admin/v1` | 约 42,674 行，1.10 MB |
| 后端 `internal/admincontract` | 约 6,194 行 |
| 前端 `contracts/backend/admin` | 约 42,686 行，1.10 MB |
| 前端 `src/modules/http/generated` | 约 31,496 行，0.98 MB |

这些代码没有增加本项目当前需要的产品能力，却让一次普通 CRUD 同时修改多个事实副本。复杂度没有对应真实收益，应当删除。

【破坏性分析】

直接删除会破坏仍依赖 generated types、runtime schema、generated views/permissions 和合同脚本的消费者。因此不能现在删目录，也不能保留第二套兼容 facade。正确顺序是：各业务模块先迁移到手写 API 事实，引用清零并人工验收后，由 Wave 07 一次性物理删除整条生成链。

## 2. 最终决策

采用方案 C：彻底退役 Admin OpenAPI 与生成合同体系。

最终不保留：

- OpenAPI JSON；
- Swagger 页面；
- SDK 生成入口；
- permissions/views/manifest 合同 bundle；
- 前后端合同镜像；
- 合同哈希和提交 revision 绑定；
- generated operations/types/views/permissions；
- route `HTTPContract`；
- 合同生成、同步、校验和发布门禁。

这不是把生成改成可选，也不是把文件换个目录归档。Wave 07 完成后，主动代码和日常开发流程中不存在这套体系。

## 3. 新的唯一事实来源

| 事实 | 唯一来源 | 验证方式 |
|---|---|---|
| HTTP 路径、方法、中间件 | 后端模块 `route.go` | Router/Handler 定向测试 |
| 请求字段和基础校验 | 后端模块 `request.go` | Handler 请求测试 |
| `data` 业务结构 | 后端模块 `response.go` | Handler 真实 JSON 测试 |
| 外层 `code/data/msg/error` | 后端公共 response/apperror | 公共响应短测试 |
| 前端请求和响应类型 | `src/api/<business>.ts` | TypeScript、Zod 和 API 单测 |
| 分页协议 | 后端 `internal/shared/pagination` 与前端 `src/utils/pagination.ts` | 两端定向测试 |
| 权限码和路由授权 | 数据库权限事实 + 后端路由中间件 | 低权限 403 矩阵测试 |
| 当前用户菜单和按钮 | `users/me` 运行时响应 | 菜单/按钮行为测试 |
| 页面组件映射 | 前端普通页面 registry 或 `import.meta.glob` | 路由单测和人工导航 |
| WebSocket 事件 | 后端/前端各自明确的事件结构 | 协议定向测试 |

前后端会各自维护一次语言边界类型，这是正常的跨语言成本。不得为了消灭这两份类型重新建设生成器；真实 JSON 和短测试才是两端兼容证据。

## 4. 后端删除边界

Wave 07 最终删除：

```text
contracts/admin/v1/
internal/admincontract/
cmd/admin-contract/
```

同时删除：

- `internal/server/adminroute` 中的 `HTTPContract` 类型和 `Definition.Contract` 字段；
- 各模块 `route.go` 中只服务生成合同的 `Contract: &adminroute.HTTPContract{...}`；
- OpenAPI、permissions、views、realtime schema 和 manifest 的构建、写入、哈希与检查逻辑；
- `generate-admin-contract.ps1`、`check-admin-contract.ps1` 及只服务合同链的同步/验证脚本；
- 只验证生成 bundle、固定哈希、固定 revision、生成文件路径或 `GenericObject` 的测试；
- Release、Dockerfile、Runbook、架构文档和 CI 对合同 bundle 的门禁或引用。

保留：

- 运行时 Gin 路由；
- Auth、Permission 和 OperationLog 中间件；
- 稳定权限码、菜单数据和 `users/me`；
- `OperationID` 中仍有真实日志或诊断消费者的部分；无消费者时再按引用删除；
- 统一错误响应和双语言消息；
- WebSocket 运行时事件结构与定向测试。

路由权限和审计必须继续在 `route.go` 显式可见。删除 `HTTPContract` 不得变成“没有权限元数据就默认放行”。

## 5. 前端删除边界

Wave 07 最终删除：

```text
contracts/backend/admin/
src/modules/http/generated/
src/modules/routing/generated/
```

同时删除：

- `contract:sync`、`contract:generate`、`contract:check` 等 package scripts；
- 后端 bundle 镜像、lock、revision 和 SHA-256 校验；
- generated operation/type 到业务 API 的 Adapter；
- generated views/permissions 到 Router、Store、`permission.ts` 的运行时依赖；
- 只验证生成文件、固定操作清单或合同镜像一致性的测试；
- 只为合同同步存在的 PowerShell/TypeScript 脚本。

替代方式：

```text
views
-> src/api/<business>.ts
-> src/utils/request.ts
-> backend
```

- 业务 API 文件显式声明 TypeScript 类型；需要运行时边界校验时，在同一文件使用严格 Zod schema。
- 不为所有响应强制重复 Zod；只在外部输入、不可信持久化数据或高风险协议边界使用。
- 页面加载使用普通静态 registry 或 `import.meta.glob`，不重新生成 views 文件。
- `permission.ts` 接收稳定字符串权限码，按钮显示仍以后端 `users/me` 为事实。
- 不建立 `src/lib/http` 与 `src/utils/request.ts` 两个 Client；迁移完成后只保留后者。

## 6. 兼容边界

必须保持：

- 现有 HTTP 路径和方法；
- 请求字段、响应字段和 `code/data/msg/error` 外层协议；
- 401、403、404 和业务错误的现有用户行为；
- 数据库表字段语义；
- 菜单、角色、权限码和按钮控制；
- Admin/App/Canvas 平台隔离；
- WebSocket 事件名、载荷和刷新恢复行为；
- 上传、支付、AI 等核心业务流程。

允许删除：

- 只服务生成体系的元数据、目录、脚本、测试和文档；
- 无运行时消费者的合同哈希、bundle version 和 backend revision；
- `HTTPContract` 中重复描述 request/response 的字段。

若迁移时发现某个 generated 类型比后端真实 JSON 多字段或少字段，必须追查 Handler 的真实响应并修正两端业务代码。禁止用 `?? []`、`|| {}`、双 shape 解包或 `Record<string, unknown>` 掩盖漂移。

## 7. 退役顺序

退役遵循消费者优先，不按目录优先：

```text
逐模块迁移前端 API 与后端 request/response
-> 用短测试锁定真实 JSON、权限和页面行为
-> 用户人工验收
-> 清除该模块 generated/runtime schema 消费者
-> 全部业务消费者清零
-> 冻结一个可恢复提交
-> Wave 07 物理删除整条合同生成链
-> 删除剩余脚本、测试、CI 和文档引用
```

迁移期间旧生成物可以暂时存在，但禁止：

- 为新接口增加 `HTTPContract` 或 generated operation；
- 为旧体系增加兼容层、第二个 Client 或第二套 registry；
- 一边删除生成物，一边让未迁移页面通过宽松类型继续运行；
- 把物理删除分散进不相关业务 Wave。

## 8. 验证策略

替代生成合同门禁的验证必须短而直接：

- 后端 Handler 测试断言真实状态码和 JSON shape；
- Service/Repository 测试覆盖业务规则与数据库约束；
- 前端 API 测试断言请求参数、响应解包和严格边界校验；
- 低权限用户对管理接口得到 403；
- `users/me` 返回的菜单和按钮能驱动真实页面；
- WebSocket 的 start/delta/completed/failed/canceled 事件保持兼容；
- `rg` 证明生成目录、`HTTPContract`、合同命令和 package scripts 无主动引用；
- 用户人工验收核心 CRUD、登录、菜单、支付、上传和 AI。

不使用另一个大型“合同退役验证框架”替代被删除的合同框架。标准 `go test`、Vitest、ESLint 和少量 `rg` 足够。

## 9. 不做的事情

- 不保留可选 OpenAPI 产物。
- 不增加 Swagger、Redoc、SDK 或第三方接口门户。
- 不使用 protobuf、GraphQL 或另一种 IDL 替换 OpenAPI。
- 不生成前端 API Client。
- 不从数据库菜单反向生成 Vue 页面注册表。
- 不将后端 Go DTO 自动转换为 TypeScript。
- 不保留 deprecated facade 让旧 generated imports 永久可用。
- 不删除运行时 RBAC、动态菜单或统一错误协议。

未来若项目真实开放第三方 API，必须基于当时的外部消费者需求单独设计；不能以“未来可能需要”为理由保留当前整套内部生成链。

## 10. 完成标准

Wave 07 完成时必须同时满足：

- 后端不存在 `contracts/admin/v1`、`internal/admincontract` 和 `cmd/admin-contract`；
- 前端不存在 `contracts/backend/admin`、`src/modules/http/generated` 和 `src/modules/routing/generated`；
- 路由中不存在 `HTTPContract` 和 `Definition.Contract`；
- package scripts、CI、Docker、Release、Runbook 和日常开发命令不再生成或检查合同；
- 普通 CRUD 只修改后端 route/request/response/handler/service/repository 和前端 api/view 的必要文件；
- 权限、菜单、错误、WebSocket 和核心业务行为保持不变；
- 两个仓库都没有为旧体系保留的兼容 facade 或空目录；
- 用户人工验收通过。

## 11. 最终原则

```text
接口行为是真相，生成文件不是。
路由、DTO 和短测试足够表达内部 API。
删除重复事实源，不是把重复事实源改成可选。
不破坏业务，但彻底删除没有消费者价值的平台工程。
```
