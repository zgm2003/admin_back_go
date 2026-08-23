# Admin 架构减法 Wave 05 设计：支付与钱包线性收口

日期：2026-08-24

状态：已获用户确认，待执行计划

## 1. 目标

Wave 05 同时收口后端和前端支付模块。目标不是重新实现支付，也不是增加支付产品能力，而是在保持现有业务事实、API JSON、数据库数据和用户行为不变的前提下，把支付与钱包改成中国开发者熟悉的扁平、线性调用链。

本波要回答一个问题：一个支付请求从入口到数据落库，能否沿着一条清楚的链路读懂，而不需要穿过多个只转发的 Adapter、Facade、Manager、Workflow 或重复 DTO。

## 2. 第一性原则

标准业务链固定为：

```text
route -> middleware -> handler -> service -> repository -> model
```

支付宝回调固定为：

```text
public callback route
  -> callback handler
  -> payment service
  -> payment gateway / repository
  -> shared paid finalizer
  -> wallet service
  -> wallet transaction
```

前端固定为：

```text
view -> page-local composable (only when state is actually shared) -> api -> utils/request
```

扁平化的是调用链，不是把所有代码塞进一个文件。`transport` 继续保留为 HTTP 和支付宝回调边界；`internal/infra/payment` 继续保留为外部支付 SDK、证书和网关边界；钱包继续作为真实的资金业务边界。只有没有独立业务所有权的包装层才允许删除。

## 3. 当前事实

后端现有支付 bounded context 位于 `internal/module/payment`，包含：

- 支付配置和支付宝证书；
- 充值套餐、充值单和支付订单；
- 支付宝异步回调、手动同步和定时补偿；
- 钱包余额、钱包流水和充值入账；
- 已存在的兑换码管理能力。

前端现有支付入口位于 `src/views/Main/payment`，API 位于 `src/api/payment` 和 `src/api/wallet`，包括支付配置、充值、钱包、资金明细和兑换码页面。

这些都是现有能力，不在本波重新设计业务规则。Wave 05 只整理调用关系、目录归属、重复结构和边界测试。

## 4. 后端设计

### 4.1 业务所有权

`internal/module/payment` 负责支付配置、充值单、订单生命周期、支付宝回调编排和支付完成后的钱包入账；`internal/module/payment/wallet` 只保留钱包余额、钱包流水、钱包 Service 和钱包 Repository 等真实资金职责。

允许保留 `payment/wallet` 目录，因为钱包拥有独立的数据一致性和并发锁语义。禁止为了目录看起来扁平，把钱包逻辑复制到支付 Service 或让支付 Handler 直接更新钱包表。

每个写入路径必须归属于一个 Service：

```text
支付配置写入       -> payment config service
创建/支付/关闭订单  -> payment order service
回调/同步/补偿入账  -> payment paid finalizer
钱包余额与流水      -> wallet service
```

Handler 只负责绑定请求、调用 Service、映射响应和错误；Repository 只负责查询、锁定和持久化；Model 只表达数据库事实。任何违反这条链的现有调用都列为迁移对象。

### 4.2 支付状态与幂等

支付宝回调必须先验签，再按订单号查找本地订单。回调、手动同步和定时补偿必须共用同一个 paid finalizer，不允许各自维护一套“支付成功后入账”逻辑。

终结器状态只能单调前进：迟到的旧回调不能把已 credited 的充值倒退；已关闭或失败的充值不能被重新打开。钱包入账必须在 MySQL 事务内完成，并通过 `wallet_transactions(source_type, source_id)` 保证同一充值事实只产生一次资金流水。

本波不修改充值金额语义、不引入新的冻结或预扣流程、不处理退款、提现、微信支付、订阅或履约。AI 余额负数和删除理论冻结属于 Wave 06。

### 4.3 外部供应商边界

支付宝 SDK 只能出现在 `internal/infra/payment/alipay`。`internal/module/payment` 只能依赖小而明确的支付 Gateway 接口和 DTO，不能直接导入 SDK。

证书继续写入本地私有相对路径，不走 COS，不暴露公有 URL。应用私钥不能进入响应、错误消息、操作日志、审计 payload、指标或测试输出。

如果现有 Gateway、证书 Store 或 Config Provider 只是同一实现的转发包装，先补调用方证据，再删除包装；如果它承担 SDK、证书或外部 IO 所有权，则保留。

## 5. 前端设计

前端支付页面统一使用：

```text
src/views/Main/payment/<page>
  -> src/api/payment 或 src/api/wallet
  -> src/utils/request
```

页面状态只保留在页面或页面专属 composable 中。只有被两个以上页面真实共享的状态才进入公共 store 或 utils。纯转发的 feature/module/workflow 层不保留。

本波逐页检查并收口：

- 支付配置：列表、创建、编辑、启停、证书和连接测试；
- 充值：套餐、创建充值单、支付、同步、关闭和充值记录；
- 订单/资金明细：查询、筛选和详情展示；
- 钱包：余额概览和流水；
- 兑换码：管理端生成/查询/作废以及用户自助兑换。

前端不改变后端请求路径、字段名、错误协议或权限码。错误统一交给现有 request/notifier 链路，不在页面里复制一套 HTTP 错误判断。支付金额使用现有字符串展示和提交规则，不用 JavaScript 浮点数进行金额计算。

包含证书、私钥、完整兑换码或支付敏感数据的响应不能进入 URL、持久化 store、localStorage、sessionStorage、页面标题或日志。页面刷新失败、支付同步失败和钱包刷新失败必须区分，不能把已经成功入账的事实显示成“支付失败”。

## 6. 允许删除的范围

只有以下条件同时满足才允许删除：生产代码无调用方、测试无合同依赖、前端无导入、路由和数据库事实不需要它。

候选删除范围：

- 只转发到同一 Service 的 payment/wallet Adapter、Facade、Manager 和 Workflow；
- 同一个 HTTP 响应的重复 DTO 或页面专用分页结构；
- 仅把请求再传一次的 API 包装函数；
- 已经没有 active route 的旧支付通知入口和菜单元数据；
- 只保护历史路径、没有实际行为的边界测试和空目录。

禁止删除：

- `transport` 目录；
- 支付 Service、钱包 Service、Repository 和 Model；
- 支付宝验签、证书私有存储、Gateway；
- shared paid finalizer、事务和幂等唯一约束；
- 当前支付 API、数据库表、历史订单和钱包流水。

任何候选层如果同时承担事务、权限、审计、外部 IO 或数据兼容职责，不能按“看起来像包装层”删除，必须保留或先拆出真实职责。

## 7. 兼容性边界

必须保持：

- 现有支付配置、充值、订单、钱包、回调和兑换码 API JSON；
- `/api/payment/callbacks/alipay` 公共回调语义；
- 支付订单、充值单、钱包余额和钱包流水的历史数据可读取；
- 现有权限码、菜单行为和管理员角色边界；
- 现有支付定时任务类型、队列和补偿行为；
- 前端页面的成功、失败、同步和刷新行为。

本波不新增表、字段、索引、migration、seed、database 管理命令或数据库生命周期脚本。若审查发现确实需要数据库变更，必须停止并单独报告，不能把结构变更偷偷塞进重构提交。

本波不生成新的 Admin Contract，不修改 OpenAPI 或 WebSocket 合同。只有确认必须改变公开路由时，才停止并另开合同变更评审。

## 8. 执行顺序

### 批次一：后端事实源

1. 盘点 payment、wallet、callback、job 的真实入口和重复层；
2. 为支付回调、手动同步、定时补偿和钱包入账补齐/整理线性 Service 测试；
3. 删除确认无调用方的后端包装层；
4. 运行支付、钱包、回调、任务和权限边界的定向短测试；
5. 提交 backend，停止等待人工验收。

### 批次二：前端调用链

1. 盘点 payment/wallet API、页面 composable、store 和历史模块导入；
2. 把页面收敛到 `view -> api -> utils/request`；
3. 删除确认无调用方的前端转发层和旧目录；
4. 运行支付页面、钱包页面、全局错误通知和定向 ESLint/Vitest；
5. 提交 frontend，停止等待人工验收。

### 批次三：共同收口

1. 检查后端和前端没有旧支付路径、重复 Service、重复 DTO 和死导入；
2. 检查数据库、API JSON、权限码、队列任务和错误协议没有变化；
3. 更新总索引和 Wave 05 handoff；
4. Wave 05 完成后停止，不进入 Wave 06。

## 9. 测试与人工验收

只执行计划内的短测试，不运行 `admin-dev`、`go test ./...`、全量 Vue/Vitest、全量 typecheck、Playwright、Docker 构建、合同生成或其他长脚本。

后端至少覆盖：

- 金额解析与边界；
- 配置和证书敏感字段不泄露；
- 支付宝验签失败被拒绝；
- 回调、手动同步、定时补偿共享 finalizer；
- 重复回调和并发入账只有一条钱包流水；
- 关闭/失败/credited 状态不会被迟到事件倒退；
- 钱包事务失败时充值状态和流水全部回滚；
- 旧订单、充值单和钱包流水仍可读取。

前端至少覆盖：

- 支付配置 CRUD 和状态切换；
- 充值创建、支付、同步、关闭和记录刷新；
- 钱包概览、流水筛选和兑换码入口；
- 支付失败、同步失败、权限 403 均触发全局 notifier；
- 已成功入账但后续刷新失败时不显示成支付失败；
- 敏感支付字段不进入 URL 和持久化状态。

人工验收由用户完成：使用现有支付宝配置验证配置页、充值页、订单/流水页、钱包页和兑换码页；至少覆盖成功、重复点击、同步、关闭、权限不足和刷新失败场景。没有人工验收通过，不得把 Wave 05 标记为完成。

## 10. 完成门槛

Wave 05 只有同时满足以下条件才算完成：

1. 后端和前端都在 `master`，工作区干净；
2. 支付与钱包的主要写入链路符合 `route -> handler -> service -> repository -> model`；
3. 没有无调用方的支付包装层、重复 DTO 或死导入；
4. 支付宝验签、共享 finalizer、钱包事务和幂等语义保持；
5. API JSON、数据库、权限、队列和用户行为未被破坏；
6. 计划内短测试通过；
7. 用户完成前后端人工黑盒验收；
8. 总索引和 handoff 已记录真实提交、测试和未运行项目；
9. 明确停止在 Wave 05，不进入 Wave 06 AI 减法。
