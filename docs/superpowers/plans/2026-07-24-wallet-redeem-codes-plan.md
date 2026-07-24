# 钱包兑换码充值 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task in the current session; do not dispatch subagents. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在支付管理中提供可生成、查询、导出和作废的一次性人民币兑换码，并让登录用户在“我的钱包”中原子兑换为余额和累计充值。

**Architecture:** 在 payment capability 内新增 `redeemcode` 子模块，由它拥有兑换码、批次、状态、限流和兑换编排；`wallet` 继续唯一拥有余额、累计充值和资金流水。兑换事务由 `redeemcode` repository 开启，锁顺序固定为“兑换码行 -> 钱包行”，并通过 wallet 的窄事务参与接口完成同一 MySQL 事务内入账；Redis 只负责跨实例试码限流，不参与资金正确性。

**Tech Stack:** Go 1.25、Gin、GORM、MySQL 8.4、go-redis、Atlas、Admin Contract Bundle、Vue 3、TypeScript、Element Plus、Vitest

---

## 实施边界

- 权威需求：`docs/superpowers/specs/2026-07-24-wallet-redeem-codes-design.md`。
- 本计划只实施兑换码充值；不实施 AI 模型定价、取消计费、欠费或 money units 迁移。
- 本期金额只写 `*_cents`，不双写 units；不读取或修改 `APP_SECRET`、`APP_SECRET_PREVIOUS`，不新增环境变量或密钥。
- 完整码只能出现在受权数据库、请求体、响应体和当前页面内存中；不得进入 URL、日志、指标、trace、操作审计 payload 或浏览器持久化存储。
- Codex 只运行单条预计小于两分钟的针对性检查。全仓测试、完整前端验证、Docker、全量 Playwright 和带真实 MySQL/Redis 的并发检查只列为用户手动命令。

## 文件结构

后端根目录为 `E:/admin/admin_back_go`，前端根目录为 `E:/admin/admin_front_ts`。

- 数据库：新增迁移和兑换码 schema，同步权限 seed、reconciliation 和 architecture contracts。
- `internal/module/payment/wallet`：增加 `redeem_code` 来源及只参加外层事务的钱包入账接口。
- `internal/module/payment/redeemcode`：新增模型、DTO、码值规则、repository、limiter、service 和 Admin transport。
- `internal/platform/admin`、`internal/server`：完成依赖注入、路由、RBAC、审计和 route golden。
- `contracts/admin/v1` 与前端 generated contract：从同一后端提交生成。
- 前端支付管理：新增兑换码管理页、生成弹窗、复制和 CSV 下载。
- 前端我的钱包：新增兑换弹窗，并刷新钱包概览和资金明细。

### Task 1: 数据库、权限和资金不变量

**Files:**
- Create: `database/migrations/202607240101_wallet_redeem_codes.sql`
- Create: `internal/architecture/wallet_redeem_codes_test.go`
- Modify: `database/migrations/atlas.sum`
- Modify: `database/schema/admin.hcl`
- Modify: `database/seeds/admin_permissions.sql`
- Modify: `database/seeds/README.md`
- Modify: `database/reconciliation/020_backfill_core.sql`
- Modify: `database/reconciliation/030_verify_schema.sql`
- Modify: `database/reconciliation/031_verify_relations.sql`
- Modify: `database/reconciliation/032_verify_money.sql`
- Modify: `database/reconciliation/050_contract_preconditions.sql`
- Modify: `internal/architecture/reconciliation_invariants_test.go`
- Modify: `internal/architecture/local_initialization_seed_test.go`
- Modify: `internal/architecture/database_baseline_test.go`

- [ ] **Step 1: 先写数据库契约测试**

新增 `TestWalletRedeemCodeDatabaseContract`，并扩展现有 architecture tests，锁定以下事实：

```text
redeem_code_batches:
  id BIGINT UNSIGNED AUTO_INCREMENT
  batch_no VARCHAR(64) ASCII BINARY, UNIQUE
  request_id VARCHAR(128) ASCII BINARY
  request_fingerprint_version VARCHAR(64) ASCII BINARY
  request_fingerprint CHAR(64) ASCII BINARY
  amount_cents BIGINT, CHECK 1..100000000
  quantity INT UNSIGNED, CHECK 1..1000
  expires_at DATETIME(6) NULL
  note VARCHAR(255) NOT NULL DEFAULT ''
  created_by INT UNSIGNED -> users.id RESTRICT
  created_at/updated_at DATETIME(6)
  UNIQUE(created_by, request_id)

redeem_codes:
  id BIGINT UNSIGNED AUTO_INCREMENT
  batch_id BIGINT UNSIGNED -> redeem_code_batches.id RESTRICT
  code CHAR(28) CHARACTER SET ascii COLLATE ascii_bin, UNIQUE
  state VARCHAR(16), CHECK unused|used|voided
  used_by INT UNSIGNED NULL -> users.id RESTRICT
  used_at DATETIME(6) NULL
  created_at/updated_at DATETIME(6)
  CHECK used <=> used_by/used_at 均非空，其余状态两列均为空
```

索引必须覆盖批次创建时间/过期时间、码的 `batch_id/state/id`、`state/id` 和 `used_by/used_at/id`；两张表均不增加 `is_del`，因为没有删除能力。

权限断言固定为：

```go
expected := map[int64]permissionSeedRow{
    657: {id: 657, name: "兑换码管理", path: "/payment/redeem-codes", icon: "Ticket", parentID: 437, component: "payment/redeem-codes", platform: "admin", typeID: 2, sort: 35, code: "payment_redeem_code_list", i18nKey: "menu.payment_redeem_codes", showMenu: 1, status: 1, isDel: 2},
    658: {id: 658, name: "批量生成兑换码", parentID: 657, platform: "admin", typeID: 3, sort: 1, code: "payment_redeem_code_generate", showMenu: 2, status: 1, isDel: 2},
    659: {id: 659, name: "作废兑换码", parentID: 657, platform: "admin", typeID: 3, sort: 2, code: "payment_redeem_code_void", showMenu: 2, status: 1, isDel: 2},
}
```

Seed 断言从 132/102 调整为 135 行/105 个非空唯一 code，且 seed 仍禁止写 `users`、`roles` 和 `role_permissions`。

- [ ] **Step 2: 运行 RED**

```powershell
go test ./internal/architecture -run 'Test(WalletRedeemCodeDatabaseContract|LocalPermissionSeed|DatabaseBaseline|Reconciliation)' -count=1
```

Expected: FAIL，至少报告新迁移、新表、权限 657-659 或 `redeem_code` 资金来源尚不存在。

- [ ] **Step 3: 写迁移、HCL、seed 和 reconciliation**

迁移按以下顺序执行：

1. 先复用 `202607150201_admin_only_rows.sql` 的临时 guard 表模式：开头 `DROP TEMPORARY TABLE IF EXISTS`，再创建带 `CHECK (violations = 0)` 的 `_wallet_redeem_code_guard`；向它插入 preflight 结果，校验 `roles.id=1 AND is_del=2`、Admin `payment` 父权限 437、权限 ID/code 657-659 占用情况和 principal version 溢出。`roles` 没有 `status` 条件。
2. preflight 通过后才创建两张表、检查约束、索引和 RESTRICT 外键；MySQL DDL 不宣称可由后续 DML 回滚。
3. 开启权限 DML 事务并锁定、二次校验上述权限事实；若 657-659 未被占用则创建，若是同一逻辑删除事实则恢复，否则失败。
4. 向 role ID 1 幂等授予权限 437、657、658、659；只恢复同一 `(role_id, permission_id)` 行，不向其他角色授权。
5. 为 `users.role_id=1 AND status=1 AND is_del=2` 补齐 `authz_principal_versions(platform='admin')`，再把这些 principal version 统一加一；版本溢出前置失败。
6. 在提交前通过同一 guard 验证三条权限、四条授权和目标 principal version 均成立；提交后删除临时 guard 表。

资金累计口径统一改为：

```sql
direction = 'in'
AND source_type IN ('recharge', 'redeem_code')
```

`030` 校验表/列/索引/check，`031` 校验 batch/creator/user 外键与孤儿事实，`032` 和 `050` 额外验证：used 码恰好对应一条同用户、同金额、`direction='in'` 的 `redeem_code + code_id` 流水；unused/voided 码不得已有该来源流水。

- [ ] **Step 4: 更新 Atlas checksum**

```powershell
pwsh -NoProfile -File scripts/database/atlas.ps1 migrate hash --dir file://database/migrations
```

Expected: `database/migrations/atlas.sum` 只新增/更新与 `202607240101` 对应的合法 hash。该命令若需要拉取 Docker 镜像或预计超过两分钟，由用户手动执行。

- [ ] **Step 5: 运行 GREEN 并提交**

```powershell
go test ./internal/architecture -run 'Test(WalletRedeemCodeDatabaseContract|LocalPermissionSeed|DatabaseBaseline|Reconciliation)' -count=1
git diff --check
git add database/migrations database/schema/admin.hcl database/seeds database/reconciliation internal/architecture
git commit -m "feat(database): add wallet redeem code facts"
```

Expected: 针对性 architecture tests PASS；没有全仓测试。

### Task 2: Wallet 事务参与接口

**Files:**
- Modify: `internal/module/payment/wallet/dto.go`
- Modify: `internal/module/payment/wallet/repository.go`
- Modify: `internal/module/payment/wallet/repository_test.go`
- Modify: `internal/module/payment/wallet/service.go`
- Modify: `internal/module/payment/wallet/service_test.go`

- [ ] **Step 1: 写 wallet 失败测试**

测试固定以下契约：

```go
const SourceRedeemCode = "redeem_code"

type RedeemCodeCreditInput struct {
    UserID      int64
    CodeID      int64
    AmountCents int64
    BatchNo     string
}

type TransactionParticipant interface {
    FindRedeemCodeCreditInTx(context.Context, *gorm.DB, int64, bool) (*Wallet, *Transaction, error)
    CreditRedeemCodeInTx(context.Context, *gorm.DB, RedeemCodeCreditInput, time.Time) (*Wallet, *Transaction, error)
}
```

`FindRedeemCodeCreditInTx` 的第三个参数是 code ID，第四个参数控制是否 `FOR UPDATE`。`CreditRedeemCodeInTx` 必须只使用传入的 `*gorm.DB`，自身不得 `Begin`/`Transaction`。

断言覆盖：锁定或创建钱包、正数校验、`balance_cents` 和 `total_recharge_cents` 两次加法各自防 `int64` 溢出、唯一来源冲突返回受控 sentinel、任一步失败由外层事务回滚。

- [ ] **Step 2: 运行 RED**

```powershell
go test ./internal/module/payment/wallet -run 'Test.*RedeemCode|TestWalletDictExposesOnlyCurrentContractSourceTypes' -count=1
```

Expected: FAIL，报告 `SourceRedeemCode` 或事务参与方法缺失。

- [ ] **Step 3: 实现最小 wallet 能力**

实现顺序固定为：

```text
校验已有外层 tx 和输入
-> 检查 (redeem_code, code_id) 尚无流水
-> 锁定或创建钱包
-> 在加法前检查 MaxInt64
-> 插入 direction=in 的唯一流水，remark 只写 batch_no
-> 同时更新 balance_cents 与 total_recharge_cents
```

已有来源不得像通用 `Credit` 那样静默返回成功；在未使用码路径中它表示完整性冲突。本人幂等重放由 `FindRedeemCodeCreditInTx` 读取原事实后在 redeemcode 模块校验。

`wallet.Credit` 仍只接受 AI 退款，不能因为新增常量而开放任意兑换码加款。只扩展资金来源字典和 `sourceTypeText` 为“兑换码充值”。

- [ ] **Step 4: 运行 GREEN 并提交**

```powershell
go test ./internal/module/payment/wallet -run 'Test.*RedeemCode|TestWalletDictExposesOnlyCurrentContractSourceTypes' -count=1
git diff --check
git add internal/module/payment/wallet
git commit -m "feat(wallet): add redeem code transaction participant"
```

Expected: 针对性 wallet tests PASS。

### Task 3: Redeemcode 生成、查询、作废与兑换核心

**Files:**
- Create: `internal/module/payment/redeemcode/model.go`
- Create: `internal/module/payment/redeemcode/dto.go`
- Create: `internal/module/payment/redeemcode/code.go`
- Create: `internal/module/payment/redeemcode/code_test.go`
- Create: `internal/module/payment/redeemcode/repository.go`
- Create: `internal/module/payment/redeemcode/repository_test.go`
- Create: `internal/module/payment/redeemcode/telemetry.go`
- Create: `internal/module/payment/redeemcode/telemetry_test.go`
- Create: `internal/module/payment/redeemcode/service.go`
- Create: `internal/module/payment/redeemcode/service_test.go`

- [ ] **Step 1: 写码值、金额和 service 失败测试**

核心常量和纯函数契约固定为：

```go
const (
    CodeAlphabet              = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"
    RequestFingerprintVersion = "redeem_batch_request_v1"
    MaxBatchQuantity          = 1000
    MaxAmountCents      int64 = 100_000_000
)

func GenerateCode(random io.Reader) (string, error)
func NormalizeCode(raw string) (string, error)
func ParseAmountCents(raw string) (int64, string, error)
```

测试必须证明：20 个随机字符、固定 `ZHR-XXXX-XXXX-XXXX-XXXX-XXXX`、内存去重、只容忍 ASCII 空格/连字符、拒绝控制字符和 Unicode 同形字符；金额只接受最多两位小数的字符串并返回规范 `0.00` 格式。

service 测试覆盖 request ID 1-128、数量/金额/过期时间/备注、指纹、相同请求重放、不同指纹冲突、状态投影、作废全有或全无，以及稳定错误映射。telemetry 测试使用 `telemetry.NewMemoryRecorder()` 锁定生成数量、状态转换、兑换结果/延迟、竞态冲突、唯一来源冲突和溢出拒绝；属性只能出现受控的 `operation`、`outcome`、`state`、`reason`，不得出现 user ID、code、batch number 或其他高基数值。

- [ ] **Step 2: 运行纯逻辑 RED**

```powershell
go test ./internal/module/payment/redeemcode -run 'Test(Generate|Normalize|Parse|Service|Telemetry)' -count=1
```

Expected: FAIL，报告 package 或函数尚不存在。

- [ ] **Step 3: 建立模型、DTO 和 repository 边界**

`Service` 只依赖下列领域 repository，不接触 GORM：

```go
type Repository interface {
    CreateOrReplayBatch(context.Context, CreateBatchRecord) (*BatchWithCodes, bool, error)
    ListCodes(context.Context, ListQuery, time.Time) ([]CodeView, int64, error)
    LookupCode(context.Context, string, time.Time) (*CodeView, error)
    ExportCodes(context.Context, ListQuery, time.Time) ([]CodeView, error)
    VoidCodes(context.Context, []int64, time.Time) (int, error)
    Redeem(context.Context, int64, string, time.Time) (*RedemptionFact, error)
}
```

`CreateOrReplayBatch` 接收已规范化金额、UTC 过期时间、SHA-256 指纹、`serialno.New("RCB", now)` 批次号和一组内存内唯一候选码。相同 `(created_by, request_id)` 且同指纹返回原批次/原码；不同指纹返回冲突。码或批次号唯一碰撞返回不含原值的 sentinel，service 最多重建整批三次，任一尝试都在单独完整事务内同成同败。

`Service` 通过 option 接收项目现有 `telemetry.Recorder`，未注入时使用 `telemetry.Noop()`。统一在 `telemetry.go` 发出 `payment.redeem_code.batches`、`payment.redeem_code.codes`、`payment.redeem_code.state_transitions`、`payment.redeem_code.redemptions`、`payment.redeem_code.redemption_latency_seconds` 和 `payment.redeem_code.conflicts`；生成计入 `unused`，成功兑换计入 `used`，作废计入 `voided`，遇到过期码计入 `expired`，所有原因均先映射到固定枚举。

管理 DTO 至少固定：`PageInitResponse`、`ListQuery/ListResponse/CodeItem`、`LookupInput/LookupResponse`、`GenerateBatchInput/GenerateBatchResponse`、`VoidInput/VoidResponse`、`ExportResponse{filename,content}`。`CodeItem` 包含 code/batch/amount/state/expiry/used user/time/creator，并为 used 码返回 `wallet_transaction_no` 供前端定位资金明细。

用户成功 DTO 固定为：

```go
type RedemptionResponse struct {
    Amount      string                  `json:"amount"`
    Transaction wallet.TransactionItem `json:"transaction"`
    Wallet      wallet.SummaryResponse `json:"wallet"`
    Replayed    bool                    `json:"replayed"`
}
```

- [ ] **Step 4: 实现 GORM repository 的事务规则**

实现以下不变量：

1. 生成：批次和全部码在一个事务中插入；request race 回读已提交批次；不返回部分码。
2. 作废：去重并升序锁定所有 ID；不存在、已使用或非法集合整体冲突；unused/expired -> voided，voided 幂等。
3. 兑换：先 `FOR UPDATE` 锁 code，再读不可变 batch，最后调用 wallet participant；锁顺序不得反转。
4. 本人 used 重放：读取原 `redeem_code + code_id` 流水，验证用户、方向、金额、来源和钱包事实后返回 `Replayed=true`。
5. 未知、过期、voided、他人 used 返回同一领域 `ErrUnavailable`；unused 却已有流水返回 `ErrIntegrityViolation`。
6. MySQL 1213/1205 只重试相同本地事务，不生成新资金身份。

所有绑定或插入完整码的 GORM 语句使用查询级 `gormlogger.Discard` session。`uk_redeem_codes_code` 等驱动错误在 repository 内映射为不包裹原错误的 sentinel，防止 GORM delegate、apperror cause 或日志带出完整码；其他日志只允许 user ID、code ID、batch ID、transaction ID。

CSV 使用 `encoding/csv`，对备注和账号等首个非空白字符为 `= + - @` 的单元格前置 `'`，返回 JSON：

```json
{"filename":"redeem-codes-20260724.csv","content":"..."}
```

- [ ] **Step 5: 运行 repository/service GREEN 并提交**

```powershell
go test ./internal/module/payment/redeemcode -run 'Test(Generate|Normalize|Parse|Service|Repository|Telemetry)' -count=1
git diff --check
git add internal/module/payment/redeemcode
git commit -m "feat(payment): add redeem code core"
```

Expected: 纯逻辑和 sqlmock tests PASS；真实 MySQL 并发测试不在此自动运行。

### Task 4: Redis 失败限流

**Files:**
- Create: `internal/module/payment/redeemcode/limiter.go`
- Create: `internal/module/payment/redeemcode/limiter_test.go`
- Modify: `internal/module/payment/redeemcode/service.go`
- Modify: `internal/module/payment/redeemcode/service_test.go`

- [ ] **Step 1: 写 limiter/service 失败测试**

接口和常量固定为：

```go
const (
    redeemRedisPrefix = "admin_go:wallet:redeem:v1:"
    failureLimit      = 10
    failureWindow     = 10 * time.Minute
    attemptLockTTL    = 15 * time.Second
)

type AttemptLimiter interface {
    Acquire(context.Context, string, int64) (AttemptLease, error)
    FailureState(context.Context, string, int64) (FailureState, error)
    RecordFailure(context.Context, string, int64) (FailureState, error)
    Release(context.Context, AttemptLease) error
}
```

Redis keys 固定为 `admin_go:wallet:redeem:v1:{<platform>:<user_id>}:attempt` 和 `...:failures`，不得包含 code、batch number 或新配置项。

- [ ] **Step 2: 运行 RED**

```powershell
go test ./internal/module/payment/redeemcode -run 'Test.*(Limiter|RateLimit)' -count=1
```

Expected: FAIL，报告 limiter 尚未实现。

- [ ] **Step 3: 实现 redislock + 固定窗口 Lua**

复用 `internal/infra/redislock` 获取带 owner 的用户尝试锁。失败计数使用独立原子 Lua：首次 `INCR` 后设置 10 分钟 TTL，后续只递增并返回 `PTTL`；读取 count/TTL 也在一个脚本中完成，发现无 TTL 的异常 key 时恢复固定窗口而不是永久封禁。

service 顺序必须是：

```text
Acquire user lock
-> FailureState，count >= 10 返回 429 + Retry-After
-> Normalize + repository.Redeem
-> 仅用户原因失败调用 RecordFailure
-> Release owner-checked lease
```

成功和本人重放不清零、不增加；MySQL/Redis 故障不增加。Redis 任一步失败都 fail closed 为 503。MySQL 已提交后 Release 失败只写受控告警并返回成功；锁占用返回 429，`Retry-After` 至少 1 秒。

- [ ] **Step 4: 运行 GREEN 并提交**

```powershell
go test ./internal/module/payment/redeemcode -run 'Test.*(Limiter|RateLimit|Redeem)' -count=1
git diff --check
git add internal/module/payment/redeemcode
git commit -m "feat(payment): rate limit wallet redemptions"
```

Expected: fake limiter 和 Lua contract tests PASS；需要真实 Redis 的用例因未设置 `TEST_REDIS_ADDR` 而 SKIP。

### Task 5: HTTP、RBAC、审计和依赖注入

**Files:**
- Create: `internal/module/payment/redeemcode/transport/admin/request.go`
- Create: `internal/module/payment/redeemcode/transport/admin/handler.go`
- Create: `internal/module/payment/redeemcode/transport/admin/handler_test.go`
- Create: `internal/module/payment/redeemcode/transport/admin/route.go`
- Modify: `internal/platform/admin/graph.go`
- Modify: `internal/platform/admin/graph_test.go`
- Modify: `internal/platform/admin/build.go`
- Modify: `internal/platform/admin/build_test.go`
- Modify: `internal/server/routes_admin_commerce_rbac.go`
- Modify: `internal/server/dependencies_test.go`
- Modify: `internal/server/testdata/admin_routes_golden.txt`
- Modify: `internal/server/testdata/admin_route_policy_golden.json`

- [ ] **Step 1: 写 handler、route 和 composition 失败测试**

固定七条路由：

```text
GET   /api/admin/v1/payment/redeem-codes/page-init
GET   /api/admin/v1/payment/redeem-codes
POST  /api/admin/v1/payment/redeem-code-lookups
GET   /api/admin/v1/payment/redeem-codes/export
POST  /api/admin/v1/payment/redeem-code-batches
PATCH /api/admin/v1/payment/redeem-codes
POST  /api/admin/v1/wallet/redemptions
```

路由策略固定为：

| 路由 | Access | Audit |
| --- | --- | --- |
| page-init/list | `payment_redeem_code_list` | `NoAudit("read-only")` |
| lookup | `payment_redeem_code_list` | `NoAudit("read-only exact lookup")` |
| export | `payment_redeem_code_list` | required `payment_redeem_code/export`，跳过响应 payload |
| generate | `payment_redeem_code_generate` | required `payment_redeem_code/generate`，跳过响应 payload |
| void | `payment_redeem_code_void` | required `payment_redeem_code/void` |
| wallet redemption | `Authenticated()` | required `wallet/redeem`，跳过请求 payload |

- [ ] **Step 2: 运行 RED**

```powershell
go test ./internal/module/payment/redeemcode/transport/admin ./internal/platform/admin ./internal/server -run 'Test.*(Redeem|Redemption|Route|Graph|Build)' -count=1
```

Expected: FAIL，报告 transport、graph capability 或 route golden 缺失。

- [ ] **Step 3: 实现协议映射和安全响应**

handler 从 `middleware.AuthIdentity` 取得 `UserID` 和 `Platform`，不接受请求里的 user ID。管理端生成使用当前管理员 ID；完整码精确查询只绑定 JSON body。

稳定错误严格使用 spec 的六个 code。429 的 `Retry-After` 由 limiter 返回的秒数生成，不读取用户输入。包含完整码的 list、lookup、export、generate 响应统一设置：

```http
Cache-Control: no-store, private
Pragma: no-cache
```

request/response、apperror cause 和 audit 均不得包含完整码；用户不可用原因统一映射为 `wallet.redeem.unavailable`。

- [ ] **Step 4: 注入 capability 并更新 route golden**

`CommerceGraph` 新增 `RedeemCodes redeemcodeadmin.HTTPService`。`build.go` 只构造一个 wallet repository，同时把它作为 wallet service repository 和 redeemcode transaction participant；limiter 使用 `resources.Redis.Redis` 与 `redislock.New(resources.Redis.Redis)`，redeemcode service 复用 `BuildInput.Telemetry`，不新增 config。

```powershell
$env:UPDATE_ADMIN_ROUTE_POLICY_GOLDEN='1'
$env:UPDATE_ADMIN_ROUTE_SNAPSHOT='1'
go test ./internal/server -run 'Test(RoutePolicyGoldenIsAdminOnlyAndCurrent|AdminRouteSnapshot)' -count=1
Remove-Item Env:UPDATE_ADMIN_ROUTE_POLICY_GOLDEN,Env:UPDATE_ADMIN_ROUTE_SNAPSHOT
```

Expected: 两份 golden 只新增上述七条路由及对应 access/audit contract。

- [ ] **Step 5: 运行 GREEN 并提交后端源码**

```powershell
go test ./internal/module/payment/redeemcode/transport/admin ./internal/platform/admin ./internal/server -run 'Test.*(Redeem|Redemption|Route|Graph|Build)' -count=1
git diff --check
git add internal/module/payment/redeemcode/transport internal/platform/admin internal/server
git commit -m "feat(payment): expose redeem code APIs"
```

Expected: 针对性 transport/composition/route tests PASS。此提交 SHA 是 Task 6 的 contract source commit。

### Task 6: Admin Contract Bundle 与前端 API

**Backend Files:**
- Modify: `contracts/admin/v1/manifest.json`
- Modify: `contracts/admin/v1/openapi.json`
- Modify: `contracts/admin/v1/permissions.json`
- Modify: `contracts/admin/v1/views.json`

**Frontend Files:**
- Modify: `contracts/backend/admin/lock.json`
- Modify: `contracts/backend/admin/v1/manifest.json`
- Modify: `contracts/backend/admin/v1/openapi.json`
- Modify: `contracts/backend/admin/v1/permissions.json`
- Modify: `contracts/backend/admin/v1/views.json`
- Modify: `src/modules/http/generated/admin.ts`
- Modify: `src/modules/http/generated/operations.ts`
- Create: `src/api/payment/redeem-codes.ts`
- Modify: `src/api/wallet/index.ts`
- Create: `tests/shared/payment/redeem-code-api.test.ts`
- Create: `tests/shared/wallet/wallet-redemption-api.test.ts`

- [ ] **Step 1: 从已提交后端源码生成 bundle**

在后端仓库执行：

```powershell
$backendCommit = (git rev-parse HEAD).Trim()
pwsh -NoProfile -File scripts/generate-admin-contract.ps1 -BackendCommit $backendCommit
```

Expected: manifest 的 `backend_commit` 等于 `$backendCommit`，OpenAPI 出现七个 operation，permissions/views 出现 657-659 和 `payment/redeem-codes`。

- [ ] **Step 2: 提交后端 contract bundle**

```powershell
git add contracts/admin/v1
git commit -m "chore(contract): publish wallet redemption APIs"
```

- [ ] **Step 3: 同步前端并生成客户端**

在前端仓库执行，`$backendCommit` 仍使用 Step 1 的源码 SHA：

```powershell
npm run contract:sync -- --backend E:/admin/admin_back_go --commit $backendCommit
npm run contract:generate
```

API adapter 只从 `AdminOperationInput` 和 generated `components` 派生类型；不得手写重复响应 DTO。`redeem-codes.ts` 封装六个管理 operation，`WalletApi.redeem` 封装 `post_api_admin_v1_wallet_redemptions`，完整码只进入 body。

- [ ] **Step 4: 运行前端 API 针对性检查并提交**

```powershell
npm test -- tests/shared/payment/redeem-code-api.test.ts tests/shared/wallet/wallet-redemption-api.test.ts
npm run contract:check
git diff --check
git add contracts/backend/admin src/modules/http/generated src/api/payment/redeem-codes.ts src/api/wallet/index.ts tests/shared/payment/redeem-code-api.test.ts tests/shared/wallet/wallet-redemption-api.test.ts
git commit -m "feat(api): add wallet redeem code clients"
```

Expected: API tests PASS，并断言没有 code path/query 或手写 operation URL。

### Task 7: 后台兑换码管理页面

**Frontend Files:**
- Create: `src/views/Main/payment/redeem-codes/index.vue`
- Create: `src/views/Main/payment/redeem-codes/components/RedeemCodeGenerateDialog.vue`
- Create: `src/views/Main/payment/redeem-codes/composables/useRedeemCodePage.ts`
- Modify: `src/lib/browser/download.ts`
- Modify: `tests/unit/browser/download.test.ts`
- Modify: `src/i18n/locales/zh-CN/payment.ts`
- Modify: `src/i18n/locales/en-US/payment.ts`
- Modify: `src/i18n/locales/generated.ts`
- Modify: `src/router/view-registry.ts`
- Create: `tests/component/payment/RedeemCodePage.test.ts`

- [ ] **Step 1: 写页面和下载 helper 失败测试**

测试断言：

- list/page-init、精确 lookup、generate、void、export 调用正确 operation；精确码不进入 query/route。
- 只有相应权限显示生成/作废按钮；used 只读，unused/expired 可作废。
- 首次提交生成 `crypto.randomUUID()`；网络失败且表单未变时重试复用 request ID，修改表单后生成新 ID。
- 生成结果只保存在组件内存，关闭/卸载清空；代码中不存在 localStorage/sessionStorage/Pinia 持久化。
- `downloadTextFile(content, filename, mime)` 创建 Blob、点击临时 anchor，并在成功或异常时 `URL.revokeObjectURL`。

- [ ] **Step 2: 运行 RED**

```powershell
npm test -- tests/component/payment/RedeemCodePage.test.ts tests/unit/browser/download.test.ts
```

Expected: FAIL，报告页面或 `downloadTextFile` 不存在。

- [ ] **Step 3: 实现管理交互**

页面沿用当前紧凑 Search + AppTable 模式：筛选批次号、状态、兑换用户、备注和时间；完整码精确查询使用独立输入和 POST。表格展示完整码、面额、派生状态、过期/兑换/创建事实，支持复制；used 行通过 `wallet_transaction_no` 跳转 `/payment/ledger?keyword=<transaction_no>`，URL 中不出现兑换码。

生成弹窗字段固定为金额字符串、数量、可选过期时间和备注。成功后展示本批完整码、复制和按 batch filter 调用后端 export；作废使用现有确认组件并显示选中数量。CSV 只把 API 返回的 `content` 交给内存下载 helper，不使用现有无 bearer token 的 `downloadFile()`。

- [ ] **Step 4: 生成 locale 和 view registry**

```powershell
npm run locale:generate
npm run routes:generate
```

Expected: `menu.payment_redeem_codes` 和页面文案进入 generated locale type，`payment/redeem-codes` 映射到新 view。

- [ ] **Step 5: 运行 GREEN 并提交**

```powershell
npm test -- tests/component/payment/RedeemCodePage.test.ts tests/unit/browser/download.test.ts tests/shared/router/view-registry.test.ts
npm run locale:check
npm run routes:check
git diff --check
git add src/views/Main/payment/redeem-codes src/lib/browser/download.ts src/i18n src/router/view-registry.ts tests/component/payment/RedeemCodePage.test.ts tests/unit/browser/download.test.ts
git commit -m "feat(payment): add redeem code management page"
```

Expected: 页面、下载、locale 和 route 针对性检查 PASS。

### Task 8: “我的钱包”兑换入口与短验收

**Frontend Files:**
- Create: `src/views/Main/personal/wallet/components/RedeemCodeDialog.vue`
- Modify: `src/views/Main/personal/wallet/index.vue`
- Modify: `src/i18n/locales/zh-CN/payment.ts`
- Modify: `src/i18n/locales/en-US/payment.ts`
- Modify: `src/i18n/locales/generated.ts`
- Create: `tests/component/payment/WalletRedeemCodeDialog.test.ts`
- Create: `tests/shared/wallet/wallet-redeem-code.test.ts`

- [ ] **Step 1: 写钱包交互失败测试**

测试固定：充值按钮旁有 Ticket 图标的“兑换码”次按钮；弹窗只含单个 code；提交时禁用重复点击；成功关闭弹窗、显示到账金额并同时刷新 summary 和 transactions；400/429/503 使用不同但不泄露状态的文案；资金来源选项包含 `redeem_code`。

- [ ] **Step 2: 运行 RED**

```powershell
npm test -- tests/component/payment/WalletRedeemCodeDialog.test.ts tests/shared/wallet/wallet-redeem-code.test.ts
```

Expected: FAIL，报告兑换弹窗或资金来源缺失。

- [ ] **Step 3: 实现钱包入口**

弹窗使用 `WalletApi.redeem({ code })`，不把 code 写入路由、标题、store 或日志。成功后清空输入，并复用页面现有的 `summary` 与 `getList()` 并行刷新：

```ts
const [nextSummary] = await Promise.all([WalletApi.summary(), getList()])
summary.value = nextSummary
```

资金明细沿用既有页面，不新增兑换历史 Tab；`redeem_code` 显示“兑换码充值”。`src/views/Main/profile/wallet/index.vue` 继续保持纯包装，不修改。

- [ ] **Step 4: 生成 locale，运行短检查并提交**

```powershell
npm run locale:generate
npm test -- tests/component/payment/WalletRedeemCodeDialog.test.ts tests/shared/wallet/wallet-redeem-code.test.ts
npm run locale:check
git diff --check
git add src/views/Main/personal/wallet src/i18n tests/component/payment/WalletRedeemCodeDialog.test.ts tests/shared/wallet/wallet-redeem-code.test.ts
git commit -m "feat(wallet): add redeem code entry"
```

Expected: 钱包交互和 locale 针对性检查 PASS。

## 交付检查

Codex 自动检查只限以下短命令，每条设置约两分钟超时：

```powershell
# backend
go test ./internal/module/payment/wallet -run 'Test.*RedeemCode' -count=1
go test ./internal/module/payment/redeemcode/... -count=1
go test ./internal/server -run 'Test(RoutePolicyGoldenIsAdminOnlyAndCurrent|AdminRouteSnapshot)' -count=1
git diff --check

# frontend
npm test -- tests/shared/payment/redeem-code-api.test.ts tests/shared/wallet/wallet-redemption-api.test.ts tests/component/payment/RedeemCodePage.test.ts tests/component/payment/WalletRedeemCodeDialog.test.ts tests/unit/browser/download.test.ts
npm run contract:check
npm run routes:check
npm run locale:check
git diff --check
```

以下命令只交给用户按需手动运行，不是 Codex 完成声明的前置条件：

```powershell
# backend full / database / Docker
go test ./... -count=1
go test -race ./... -count=1
pwsh -NoProfile -File scripts/verify-database.ps1
pwsh -NoProfile -File scripts/verify-go-clean.ps1

# frontend full
npm run verify:frontend

# opt-in live integration
$env:TEST_MYSQL_DSN='<test-only-dsn>'; go test ./internal/module/payment/redeemcode -run 'Test.*Concurrent' -count=1
$env:TEST_REDIS_ADDR='<test-only-redis>'; go test ./internal/module/payment/redeemcode -run 'Test.*Redis' -count=1
```

发布顺序固定为：提交并同步同一 contract -> 受控执行迁移 -> 部署后端并通过现有 principal `Reconcile` 启动门槛 -> 切走旧后端 -> 发布前端 -> 用测试码做一次生成、兑换、资金明细和本人重放 smoke。回滚应用时保留新表、码状态、余额和流水，不逆向扣款。
