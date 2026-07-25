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
- `internal/module/export`：增加可复用的有界 UTF-8 CSV writer；明文码不进入异步 export task 或 COS。
- 前端支付管理：新增兑换码管理页、生成弹窗、复制和有界 CSV 下载。
- 前端我的钱包：新增兑换弹窗，并刷新钱包概览和资金明细。

### Task 1: 数据库、权限和资金不变量

**Files:**
- Create: `database/migrations/202607240101_wallet_redeem_codes.sql`
- Create: `database/migrations/202607240102_wallet_redeem_code_permissions.sql`
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
  id BIGINT AUTO_INCREMENT（signed，与 Go int64 / wallet source_id 一致）
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
  CHECK expires_at IS NULL OR expires_at > created_at

redeem_codes:
  id BIGINT AUTO_INCREMENT（signed）
  batch_id BIGINT -> redeem_code_batches.id RESTRICT
  code CHAR(28) CHARACTER SET ascii COLLATE ascii_bin, UNIQUE
  state VARCHAR(16), CHECK unused|used|voided
  used_by INT UNSIGNED NULL -> users.id RESTRICT
  used_at DATETIME(6) NULL
  created_at/updated_at DATETIME(6)
  CHECK used <=> used_by/used_at 均非空，其余状态两列均为空
```

唯一键名称固定为 `uk_redeem_code_batches_batch_no`、`uk_redeem_code_batches_creator_request` 和 `uk_redeem_codes_code`，repository 只按这些名字分类冲突。普通索引必须覆盖批次创建时间/过期时间、码的 `batch_id/state/id`、`state/id` 和 `used_by/used_at/id`；两张表均不增加 `is_del`，因为没有删除能力。主键不得使用 `UNSIGNED`，否则会与现有有符号 `wallet_transactions.source_id` 和 Go `int64` 形成不可表达区间。

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

1. `202607240101_wallet_redeem_codes.sql` 只负责 schema。先用临时 guard 校验两个目标表均不存在、被引用主键类型与预期一致；preflight 通过后创建两张表、检查约束、索引和 RESTRICT 外键。该 revision 不写 `permissions`、`role_permissions` 或 principal version，也不宣称 MySQL DDL 可由事务回滚。
2. `202607240102_wallet_redeem_code_permissions.sql` 只负责权限 DML。复用 `202607150201_admin_only_rows.sql` 的临时 guard 表模式：开头 `DROP TEMPORARY TABLE IF EXISTS`，再创建带 `CHECK (violations = 0)` 的 `_wallet_redeem_code_permission_guard`；preflight 校验两张兑换码表已存在、`roles.id=1 AND is_del=2`、Admin `payment` 父权限 437、权限 ID/code 657-659 占用情况和 principal version 溢出。`roles` 没有 `status` 条件。
3. 开启权限 DML 事务并锁定、二次校验上述权限事实；657-659 未被占用时创建，已是完全相同的 active 事实时保持不变，已是完全相同的逻辑删除事实时恢复，任何 ID/code 交叉占用或字段不一致都失败。
4. 向 role ID 1 幂等授予权限 437、657、658、659；只恢复同一 `(role_id, permission_id)` 行，不向其他角色授权。
5. 为 `users.role_id=1 AND status=1 AND is_del=2` 补齐 `authz_principal_versions(platform='admin')`，再把这些 principal version 统一加一；版本溢出前置失败。
6. 在提交前通过同一 guard 验证三条权限、四条授权和目标 principal version 均成立；提交后删除临时 guard 表。由于 schema 与权限使用不同 Atlas revision，授权事务失败时只需重试 `202607240102`，不会被已经提交的建表 DDL 卡住。

资金累计口径统一改为：

```sql
direction = 'in'
AND source_type IN ('recharge', 'redeem_code')
```

`030` 校验表/列/索引/check，`031` 校验 batch/creator/user 外键与孤儿事实，并校验每个 batch 的 `quantity` 恰好等于实际 code 行数。`032` 和 `050` 双向验证：used 码恰好对应一条 `is_del=2`、同用户、同金额、同钱包归属、`direction='in'` 且 `balance_before_cents + amount_cents = balance_after_cents` 的 `redeem_code + code_id` 流水；每条 `source_type='redeem_code'` 流水都必须为 active 并反向对应一个 used 码；unused/voided 码不得已有该来源流水。

- [ ] **Step 4: 更新 Atlas checksum**

```powershell
pwsh -NoProfile -File scripts/database/atlas.ps1 migrate hash --dir file://database/migrations
```

Expected: `database/migrations/atlas.sum` 只新增/更新与 `202607240101`、`202607240102` 对应的合法 hash。该命令若需要拉取 Docker 镜像或预计超过两分钟，由用户手动执行。

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

断言覆盖：按 user ID 锁定 active 钱包或在确实不存在时创建、同一唯一身份若对应软删除钱包则返回完整性 sentinel 而不是创建第二个钱包、正数校验、`balance_cents` 和 `total_recharge_cents` 两次加法各自防 `int64` 溢出、唯一来源冲突返回受控 sentinel、任一步失败由外层事务回滚。

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

已有来源不得像通用 `Credit` 那样静默返回成功；查询唯一来源时不能用 `is_del` 过滤掩盖软删除事实，任何 `redeem_code` 来源流水 `is_del != 2` 都返回完整性冲突。在未使用码路径中已有来源同样表示完整性冲突。本人幂等重放由 `FindRedeemCodeCreditInTx` 读取原事实后在 redeemcode 模块校验：流水 active、user/wallet/direction/source/amount 必须匹配，历史 `balance_before + amount = balance_after` 必须成立，但当前钱包余额允许已被后续流水改变，不得要求它仍等于历史 `balance_after`。

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
- Modify: `internal/module/export/writer.go`
- Modify: `internal/module/export/writer_test.go`
- Create: `internal/module/payment/redeemcode/model.go`
- Create: `internal/module/payment/redeemcode/dto.go`
- Create: `internal/module/payment/redeemcode/code.go`
- Create: `internal/module/payment/redeemcode/code_test.go`
- Create: `internal/module/payment/redeemcode/repository.go`
- Create: `internal/module/payment/redeemcode/repository_test.go`
- Create: `internal/module/payment/redeemcode/repository_integration_test.go`
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
    MaxVoidCodes              = 1000
    MaxExportRows             = 10000
    MaxRawCodeBytes           = 128
    MaxAmountCents      int64 = 100_000_000
)

func GenerateCode(random io.Reader) (string, error)
func NormalizeCode(raw string) (string, error)
func ParseAmountCents(raw string) (int64, string, error)
```

测试必须证明：20 个随机字符、固定 `ZHR-XXXX-XXXX-XXXX-XXXX-XXXX`、内存去重、原始 code 最多 128 bytes、只容忍 ASCII 空格/连字符、拒绝控制字符和 Unicode 同形字符；31 字符 alphabet 使用拒绝采样，只接受 `<248` 的随机字节再 `%31`，确定性 reader 用例必须证明 248-255 会被丢弃而不是产生取模偏差；单码拒绝采样和批内去重均设置固定尝试预算，持续拒绝字节/持续重复码的 reader 返回受控错误而不是挂死；金额只接受匹配 `^(0|[1-9][0-9]*)(\.[0-9]{1,2})?$` 的 ASCII 字符串，必须用整数位/小数位组装 cents，禁止 `ParseFloat`，并返回规范 `0.00` 格式。

service 测试覆盖 request ID 必须匹配 `^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`、数量/金额/未来过期时间/备注、最多 1000 个作废 ID、指纹、相同请求重放、不同版本或指纹冲突、状态投影、作废全有或全无、有界导出，以及稳定错误映射。过期时间先 UTC 化并截断到微秒；首次创建使用早于或等于服务端 `created_at` 的过期时间必须拒绝，但原批次过期后使用同一 request ID、版本和指纹重放仍必须返回原批次与原码，且已有批次重放在注入失败 random reader 时仍成功，证明普通重放不依赖随机码生成。另用 fake repository 模拟首次查询未命中、`CreateOrReplayBatch` 因并发 race 回读到已过期原批次，断言 service 不会在第二次 repository 判断前按当前时间提前拒绝。备注 trim、拒绝控制字符并限制 255 个字符；指纹使用字段固定的 canonical JSON 后 SHA-256，不能用分隔符字符串拼接。telemetry 测试使用 `telemetry.NewMemoryRecorder()` 锁定生成数量、状态转换、兑换结果/延迟、竞态冲突、唯一来源冲突和溢出拒绝；属性只能出现受控的 `operation`、`outcome`、`state`、`reason`，不得出现 user ID、code、batch number 或其他高基数值。

`repository_integration_test.go` 定义三个 opt-in 用例：`TestConcurrentBatchRequestReplayCreatesOneBatch`、`TestConcurrentRedeemHasOneWalletCredit`、`TestConcurrentRedeemAndVoidHasOneTerminalState`。未设置 `TEST_MYSQL_DSN` 时立即 `t.Skip`；设置时只允许专用测试库，使用每例唯一 fixture 并清理自身数据。它们分别断言并发 request race 只产生一批码、不同用户争同一码只有一条钱包收入流水，以及作废/兑换竞态只留下一个合法终态。这些用例只由交付区的用户手动命令运行。

- [ ] **Step 2: 运行纯逻辑 RED**

```powershell
go test ./internal/module/export -run 'TestCSVWriter' -count=1
go test ./internal/module/payment/redeemcode -run 'Test(Generate|Normalize|Parse|Service|Telemetry)' -count=1
```

Expected: FAIL，报告共享 CSV writer、redeemcode package 或函数尚不存在。

- [ ] **Step 3: 建立模型、DTO 和 repository 边界**

`Service` 只依赖下列领域 repository，不接触 GORM：

```go
type Repository interface {
    FindBatchByRequest(context.Context, int64, string) (*BatchWithCodes, error)
    CreateOrReplayBatch(context.Context, CreateBatchRecord) (*BatchWithCodes, bool, error)
    ListCodes(context.Context, ListQuery, time.Time) ([]CodeView, int64, error)
    LookupCode(context.Context, string, time.Time) (*CodeView, error)
    ExportCodes(context.Context, ListQuery, time.Time, int) ([]CodeView, error)
    VoidCodes(context.Context, []int64, time.Time) (int, error)
    Redeem(context.Context, int64, string) (*RedemptionFact, error)
}
```

service 在规范化并计算指纹后先调用 `FindBatchByRequest(createdBy, requestID)`：存在时比较版本/指纹并直接重放，不能先校验“当前是否已过期”、调用 random reader 或生成 batch number。普通查询不存在时才生成候选码并调用 `CreateOrReplayBatch`，但 service 不能在该调用前以过去的 `expires_at` 直接返回，因为并发创建可能尚未对首次查询可见。`CreateOrReplayBatch` 接收已规范化金额、微秒精度 UTC 过期时间、指纹版本/SHA-256 指纹、`serialno.New("RCB", now)` 批次号和一组内存内唯一候选码，并在事务锁/唯一键 race 层重复保护 request identity：同版本/指纹返回原批次/原码，即使原批次此时已经过期；版本或指纹不同返回冲突；只有最终确认不存在既有 request、确实要首次插入时才校验 `expires_at > created_at`。普通重放和 request race 回读都必须按 code ID 升序返回恰好 `quantity` 个原码，缺行/多行视为完整性错误。repository 必须按约束名区分 request race、batch number 碰撞和 code 碰撞；只有后两类返回不含原值的 collision sentinel，service 最多重建整批三次。request race 不能被误当随机碰撞重发；任一尝试都在单独完整事务内同成同败。

`Service` 通过 option 接收项目现有 `clock.Clock`、`telemetry.Recorder` 和仅供测试替换的 `io.Reader`；生产默认分别为 `clock.SystemClock{}`、`telemetry.Noop()` 和 `crypto/rand.Reader`，nil option 必须回落到这些安全默认值。生成、查询、导出和作废各捕获一次 `Clock.Now().UTC().Truncate(time.Microsecond)` 并向下传递。GORM repository 另外接收同一个 `clock.Clock`，只在兑换事务已取得 code `FOR UPDATE` 锁后捕获一次 decision time，用它判断 unused 是否过期并写 `used_at`；禁止用 service 进入时或锁等待前的旧时间。统一在 `telemetry.go` 发出 `payment.redeem_code.batches`、`payment.redeem_code.codes`、`payment.redeem_code.state_transitions`、`payment.redeem_code.redemptions`、`payment.redeem_code.redemption_latency_seconds` 和 `payment.redeem_code.conflicts`；生成计入 `unused`，成功兑换计入 `used`，作废计入 `voided`，遇到过期码计入 `expired`，所有原因均先映射到固定枚举。

管理 DTO 至少固定：`PageInitResponse`、`ListQuery/ListResponse/CodeItem`、`LookupInput/LookupResponse`、`GenerateBatchInput/GenerateBatchResponse`、`VoidInput/VoidResponse`、`ExportInput`、`ExportResponse{filename,content,row_count}`。`GenerateBatchResponse` 只返回一次 batch 元数据和最多 1000 个最小 `{id,code}` 项，不为每个 code 重复 note/creator/amount；`ExportInput` 只复用 list 的非分页、非码值筛选字段。`CodeItem` 包含 code/batch/amount/state/expiry/used user/time/creator，并为 used 码返回 `wallet_transaction_no` 供前端定位资金明细。

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
4. 本人 used 重放：在过期判断前读取原 `redeem_code + code_id` 流水，验证流水 active、用户、钱包归属、方向、金额、来源和历史余额算术后返回 `Replayed=true`；返回最新钱包概览，不把当前余额与历史 `balance_after` 强行比较。
5. 未知、过期、voided、他人 used 返回同一领域 `ErrUnavailable`；unused 却已有流水、本人重放事实不一致或 wallet participant 报告加法溢出均返回 `ErrIntegrityViolation`，且不得计入用户试码失败。
6. MySQL 1213/1205 只重试相同本地事务，不生成新资金身份。

repository 测试使用可推进的 fake clock 模拟“请求进入时未过期、等待 code 行锁后已过期”，断言不调用 wallet participant 且码保持 unused；另断言本人已经 used 的重放即使当前超过批次过期时间仍按历史事实幂等成功。

list/lookup/export 使用有界 JOIN/read model 一次取回 batch、creator、used user 和可选钱包流水号，禁止逐行查询用户或流水；完整码精确条件始终参数化。分页 list 可以单独执行 count，export 不执行全量 count，只读取排序稳定的 `MaxExportRows + 1` 行。

所有绑定或插入完整码的 GORM 语句使用查询级 `gormlogger.Discard` session。`NormalizeCode` 及相关 validator 只能返回不含原输入的 sentinel；`uk_redeem_codes_code` 等驱动错误在 repository 内映射为不包裹原错误的 sentinel，防止 GORM delegate、apperror cause 或日志带出完整码；其他日志只允许 user ID、code ID、batch ID、transaction ID。

在 `internal/module/export` 增加共享 `CSVWriter`：使用 `encoding/csv`、UTF-8 BOM 和 CRLF，对任意首个非空白字符为 `= + - @` 的单元格前置 `'`。redeemcode 复用该 writer，但不创建 `export_tasks`、不上传 COS。repository 查询固定使用 `LIMIT MaxExportRows + 1`，service 在多出一行时返回 `payment.redeem_code.export_too_large`，绝不无界加载。成功返回 JSON：

```json
{"filename":"redeem-codes-20260724.csv","content":"...","row_count":1000}
```

- [ ] **Step 5: 运行 repository/service GREEN 并提交**

```powershell
go test ./internal/module/export -run 'TestCSVWriter' -count=1
go test ./internal/module/payment/redeemcode -run 'Test(Generate|Normalize|Parse|Service|Repository|Telemetry|Export)' -count=1
git diff --check
git add internal/module/export/writer.go internal/module/export/writer_test.go internal/module/payment/redeemcode
git commit -m "feat(payment): add redeem code core"
```

Expected: 纯逻辑和 sqlmock tests PASS；三个真实 MySQL 并发用例因未设置 `TEST_MYSQL_DSN` 而 SKIP，不在此自动运行。

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
    attemptTimeout    = 10 * time.Second
    cleanupTimeout    = 2 * time.Second
)

type AttemptLimiter interface {
    Acquire(context.Context, string, int64) (AttemptLease, error)
    FailureState(context.Context, string, int64) (FailureState, error)
    RecordFailure(context.Context, string, int64) (FailureState, error)
    Release(context.Context, AttemptLease) error
}
```

Redis keys 固定为 `admin_go:wallet:redeem:v1:{<platform>:<user_id>}:attempt` 和 `...:failures`，不得包含 code、batch number 或新配置项。测试必须证明 attempt key 最迟 15 秒过期、failures key 最迟 10 分钟过期，并且不会产生 `:fencing-token` 等第三个永久 per-user key。

真实 Redis integration case 仅在 `TEST_REDIS_ADDR` 存在时运行，固定使用 DB 14、每例生成唯一正 user ID，只删除本例计算出的 attempt/failures 两个精确 key，禁止 `FLUSHDB`、通配扫描或清理其他测试数据；默认短检查必须立即 SKIP。

- [ ] **Step 2: 运行 RED**

```powershell
go test ./internal/module/payment/redeemcode -run 'Test.*(Limiter|RateLimit)' -count=1
```

Expected: FAIL，报告 limiter 尚未实现。

- [ ] **Step 3: 实现 owned attempt lock + 固定窗口 Lua**

attempt lock 使用 `SET key <crypto-random-128-bit-owner> NX PX 15000`；占用时读取 `PTTL`，按 `max(1, ceil(pttl_ms/1000))` 生成整数秒 Retry-After，释放使用 compare-owner-and-delete Lua。这里不得复用 `internal/infra/redislock`：它会为每个 lock key 保留持久 fencing counter，而该防试码锁不参与资金正确性、不需要 fencing token。失败计数使用独立原子 Lua：首次 `INCR` 后设置 10 分钟 TTL，后续只递增并返回 `PTTL`；读取 count/TTL 也在一个脚本中完成，发现无 TTL 的异常 key 时恢复固定窗口而不是永久封禁。

service 顺序必须是：

```text
Acquire user lock
-> 立即创建 10 秒 attempt context
-> 在 attempt context 内执行 FailureState；count >= 10 返回 429 + Retry-After
-> 在同一个 attempt context 内 Normalize + repository.Redeem
-> 仅用户原因失败使用 WithoutCancel + 独立 2 秒 cleanup context 调用 RecordFailure
-> 使用另一个独立 cleanup context Release owner-checked lease
```

第 10 次实际失败先原子写成 count=10，仍返回本次 `wallet.redeem.unavailable`；下一次请求起在访问 MySQL 前 429。达到阈值后所有请求都先 429，包括本人的已用码重放；本人重放“不计失败”不代表可绕过限流继续查询数据库。可解析 DTO 的空 code/格式错误在 service 锁内计数，畸形 JSON 由 transport 拒绝且不计数。成功和获准执行的本人重放不清零、不增加；MySQL/Redis 故障不增加。

`RecordFailure` 和 `Release` 必须从 `context.WithoutCancel(requestCtx)` 派生 cleanup timeout，测试模拟 repository 已判定不可用后客户端取消，仍然记录失败并释放锁。Redis 任一步失败都 fail closed 为 503；唯一结果优先级例外是 repository 已返回新兑换成功或已验证的本人重放成功，此后的 Release 失败只写受控告警并保持成功事实。锁占用返回 429，`Retry-After` 至少 1 秒。

- [ ] **Step 4: 运行 GREEN 并提交**

```powershell
go test ./internal/module/payment/redeemcode -run 'Test.*(Limiter|RateLimit|Redeem)' -count=1
git diff --check
git add internal/module/payment/redeemcode
git commit -m "feat(payment): rate limit wallet redemptions"
```

Expected: fake limiter、取消后计数和 Lua contract tests PASS；需要真实 Redis 的用例因未设置 `TEST_REDIS_ADDR` 而 SKIP。

### Task 5: HTTP、RBAC、审计和依赖注入

**Files:**
- Create: `internal/module/payment/redeemcode/transport/admin/request.go`
- Create: `internal/module/payment/redeemcode/transport/admin/handler.go`
- Create: `internal/module/payment/redeemcode/transport/admin/handler_test.go`
- Create: `internal/module/payment/redeemcode/transport/admin/route.go`
- Modify: `internal/shared/i18n/locales/zh-CN/payment.yaml`
- Modify: `internal/shared/i18n/locales/en-US/payment.yaml`
- Modify: `internal/shared/i18n/locales/zh-CN/wallet.yaml`
- Modify: `internal/shared/i18n/locales/en-US/wallet.yaml`
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
POST  /api/admin/v1/payment/redeem-code-exports
POST  /api/admin/v1/payment/redeem-code-batches
PATCH /api/admin/v1/payment/redeem-codes
POST  /api/admin/v1/wallet/redemptions
```

路由策略固定为：

| 路由 | Access | Audit |
| --- | --- | --- |
| page-init/list | `payment_redeem_code_list` | `NoAudit("read-only")` |
| lookup | `payment_redeem_code_list` | `NoAudit("read-only exact lookup")` |
| export | `payment_redeem_code_list` | best-effort `payment_redeem_code/export`，跳过请求/响应 payload |
| generate | `payment_redeem_code_generate` | required `payment_redeem_code/generate`，跳过请求/响应 payload |
| void | `payment_redeem_code_void` | required `payment_redeem_code/void`，跳过请求 payload |
| wallet redemption | `Authenticated()` | required `wallet/redeem`，跳过请求/响应 payload |

export 必须是 `Enabled=true, Required=false, SkipRequestPayload=true, SkipResponsePayload=true`：当前 required-audit writer 即使跳过响应 payload 仍会暂存完整 HTTP body，超过 1 MiB 就把响应改写为 500；有界 CSV 不能走这条 staging 路径。operation-log middleware 在 handler 校验前读取原始请求体，攻击者可以给任意 JSON 附加未知 `code` 字段，因此 generate、void 和 wallet redemption 也必须 `SkipRequestPayload=true`，不能依赖 handler 拒绝后再脱敏。generate/void/wallet redemption 继续 required，wallet redemption 和 generate 还必须跳过响应 payload；generate 响应把 batch 元数据只返回一次、codes 使用最小数组结构，测试锁定 1000 码响应不超过当前 required-audit 1 MiB 上限。

handler 测试还要锁定请求体边界：code-only lookup/redemption 在 JSON 绑定前使用 1 KiB 上限，generate/export/void 使用 64 KiB 上限。超过上限或畸形 JSON 返回 400，不能调用 service/limiter，也不计试码失败；`{"code":"..."}` 可正常解析但 code 超过 128 bytes 时必须进入 service，在 attempt lock 内计为用户失败。测试不得把超长 code 打进失败消息或日志断言输出。

- [ ] **Step 2: 运行 RED**

```powershell
go test ./internal/module/payment/redeemcode/transport/admin ./internal/platform/admin ./internal/server -run 'Test.*(Redeem|Redemption|Route|Graph|Build)' -count=1
```

Expected: FAIL，报告 transport、graph capability 或 route golden 缺失。

- [ ] **Step 3: 实现协议映射和安全响应**

handler 从 `middleware.AuthIdentity` 取得 `UserID` 和 `Platform`，不接受请求里的 user ID。管理端生成使用当前管理员 ID；完整码精确查询只绑定 JSON body，普通 list 对 query key 使用显式白名单并拒绝重复值，任何 `code`、码片段或未知 key 都返回受控 400。所有 JSON handler 先用 `http.MaxBytesReader` 有界读取，再使用局部 `json.Decoder` 严格解码唯一的顶层 object：拒绝未知字段、尾随 JSON/垃圾、顶层 `null`/标量/数组以及显式 `code:null`。code-only lookup/redemption 上限 1 KiB，generate/export/void 上限 64 KiB；上限或 JSON shape 错误使用不回显 body 的受控 400。wallet redemption 的 transport 错误映射为 `wallet.redeem.unavailable` 但不进入 limiter，管理端则映射为 `payment.redeem_code.request_invalid`；可解析 object 中缺失 code、空字符串和格式错误仍留给 service，在 attempt lock 内计数。

`POST /wallet/redemptions` 的稳定错误严格使用 spec 的六个 code，并在 `wallet.yaml` 提供中英文 MessageID；不能用只设置 MessageID 的 `BadRequestKey`/`InternalKey` 代替稳定机器码。service 使用 `apperror.New`/`apperror.Wrap`，令 `Error.Code` 与 `MessageID` 都等于对应的 `wallet.redeem.*` 值，并锁定 category/HTTP/retry：`code_required` 与 `unavailable` 为 validation/400/permanent，`rate_limited` 为 rate_limit/429/retryable，两个 `*_unavailable` 为 dependency/503/retryable，`integrity_violation` 为 internal/500/permanent。handler 测试必须解码响应并逐项断言 `error.code/category/retryable`，不能只断言 HTTP 或 `msg`。管理端生成、查询、导出和作废同样使用 `payment.redeem_code.*` 稳定管理错误，不能误压成用户兑换错误。429 的 `Retry-After` 由 limiter 返回的秒数生成，不读取用户输入。包含完整码的 list、lookup、export、generate 响应统一设置：

```http
Cache-Control: no-store, private
Pragma: no-cache
```

request/response 日志、apperror cause 和 audit 均不得包含完整码；用户不可用原因统一映射为 `wallet.redeem.unavailable`。wallet 加法溢出和既有兑换/流水事实不一致映射为 `wallet.redeem.integrity_violation`，数据库连接/超时等临时故障映射为 `wallet.redeem.dependency_unavailable`；两类平台失败都不增加试码计数。handler 只负责 JSON shape；可解析 DTO 中的空 code 和格式错误必须交给 service，保证它们在 attempt lock 内按 spec 计数。

后端中英文 catalog 必须同时收录代码实际使用的全部 keyed message：用户接口六个 `wallet.redeem.*` code 写入 `wallet.yaml`；管理端至少固定 `payment.redeem_code.request_invalid`、`payment.redeem_code.request_conflict`、`payment.redeem_code.void_conflict`、`payment.redeem_code.export_too_large`、`payment.redeem_code.dependency_unavailable`、`payment.redeem_code.integrity_violation` 和 `payment.redeem_code.service_missing`，写入 `payment.yaml`。同一错误在 service、handler 测试和 catalog 中必须使用完全相同的 `Error.Code` 与 `MessageID`。

- [ ] **Step 4: 注入 capability 并更新 route golden**

`CommerceGraph` 新增 `RedeemCodes redeemcodeadmin.HTTPService`。`build.go` 只构造一个 wallet repository，同时把它作为 wallet service repository 和 redeemcode transaction participant；redeemcode GORM repository 与 service 都复用本函数已有的 `sharedClock`，保证锁后过期判断可测试且时间来源一致；limiter 直接使用 `resources.Redis.Redis` 实现有 TTL 的 owned attempt lock 和失败脚本，不构造 `redislock` fencing store；service 复用 `BuildInput.Telemetry`，生产随机源使用 `crypto/rand.Reader` 默认值，不新增 config。

```powershell
$env:UPDATE_ADMIN_ROUTE_POLICY_GOLDEN='1'
$env:UPDATE_ADMIN_ROUTE_SNAPSHOT='1'
try {
    go test ./internal/server -run 'Test(RoutePolicyGoldenIsAdminOnlyAndCurrent|AdminRouteSnapshot)' -count=1
} finally {
    Remove-Item Env:UPDATE_ADMIN_ROUTE_POLICY_GOLDEN,Env:UPDATE_ADMIN_ROUTE_SNAPSHOT -ErrorAction SilentlyContinue
}
```

Expected: 两份 golden 只新增上述七条路由及对应 access/audit contract。

- [ ] **Step 5: 运行 GREEN 并提交后端源码**

```powershell
go test ./internal/module/payment/redeemcode/transport/admin ./internal/platform/admin ./internal/server ./internal/shared/i18n -run 'Test.*(Redeem|Redemption|Route|Graph|Build|Catalog)' -count=1
git diff --check
git add internal/module/payment/redeemcode/transport internal/shared/i18n/locales internal/platform/admin internal/server
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
- Modify: `src/modules/routing/generated/permissions.ts`
- Modify: `src/modules/routing/generated/views.ts`
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

API adapter 只从 `AdminOperationInput` 和 generated `components` 派生类型；不得手写重复响应 DTO。`redeem-codes.ts` 封装六个管理 operation，其中 export 使用 `post_api_admin_v1_payment_redeem_code_exports` 并把非码值筛选放在 body；`WalletApi.redeem` 封装 `post_api_admin_v1_wallet_redemptions`，完整码只进入 lookup/redeem body。

- [ ] **Step 4: 运行前端 API 针对性检查并提交**

```powershell
npm test -- tests/shared/payment/redeem-code-api.test.ts tests/shared/wallet/wallet-redemption-api.test.ts
npm run contract:check
git diff --check
git add contracts/backend/admin src/modules/http/generated src/modules/routing/generated/permissions.ts src/modules/routing/generated/views.ts src/api/payment/redeem-codes.ts src/api/wallet/index.ts tests/shared/payment/redeem-code-api.test.ts tests/shared/wallet/wallet-redemption-api.test.ts
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

- list/page-init、精确 lookup、generate、void、export 调用正确 operation；精确码不进入 query/route，export 通过 POST body 传递非码值筛选。
- 只有相应权限显示生成/作废按钮；used 只读，unused/expired 可作废。
- 首次提交生成 `crypto.randomUUID()`；只要表单规范值未变且尚未收到成功响应，网络错误、超时、429、5xx 或其他失败后的人工重试都复用同一 request ID，防止“数据库已提交但 required audit/响应失败”后重复发行。未确认请求的 ID/客户端指纹由页面级 composable 在页面生命周期内保留，关闭再打开弹窗不会自动轮换；完整码结果仍在关闭时清空。修改表单、成功后开始新批次，或管理员明确确认放弃未确认请求时才生成新 ID；唯一自动例外是后端明确返回 `payment.redeem_code.request_conflict`，此时确认当前指纹未创建后轮换 ID 并要求管理员再次确认提交。request ID/指纹和完整码均不得写入 localStorage、sessionStorage 或 Pinia persistence。
- 生成结果只保存在组件内存，关闭/卸载清空；代码中不存在 localStorage/sessionStorage/Pinia 持久化。
- `downloadTextFile(content, filename, mime)` 创建 Blob、点击临时 anchor，并在成功或异常时 `URL.revokeObjectURL`。

- [ ] **Step 2: 运行 RED**

```powershell
npm test -- tests/component/payment/RedeemCodePage.test.ts tests/unit/browser/download.test.ts
```

Expected: FAIL，报告页面或 `downloadTextFile` 不存在。

- [ ] **Step 3: 实现管理交互**

页面沿用当前紧凑 Search + AppTable 模式：筛选批次号、状态、兑换用户、备注和时间；完整码精确查询使用独立输入和 POST。表格展示完整码、面额、派生状态、过期/兑换/创建事实，支持复制；used 行通过 `wallet_transaction_no` 跳转 `/payment/ledger?keyword=<transaction_no>`，URL 中不出现兑换码。

生成弹窗字段固定为金额字符串、数量、可选过期时间和备注。成功后展示本批完整码、复制和按 `batch_no` filter 调用后端 export；列表导出传递当前非分页筛选，超过 10000 行时提示缩小范围，不做客户端全表拼接。作废使用现有确认组件并显示选中数量。CSV 只把 API 返回的 `content` 交给 `downloadTextFile(..., 'text/csv;charset=utf-8')`，不使用现有无 bearer token 的 `downloadFile()`。

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

测试固定：充值按钮旁有 Ticket 图标的“兑换码”次按钮；弹窗只含单个 code；提交时禁用重复点击；成功关闭弹窗、显示到账金额并同时刷新 summary 和 transactions；兑换已成功但后续任一刷新失败时只能提示“到账成功、部分信息刷新失败”，不能把资金结果显示成兑换失败或自动重提；400/429/500/503 使用不同但不泄露码状态的文案，任一非成功响应都保留输入并允许用户用同一码人工重试，因为 500/网络错误可能发生在资金已提交但 required audit/响应失败之后；资金来源选项包含 `redeem_code`。

- [ ] **Step 2: 运行 RED**

```powershell
npm test -- tests/component/payment/WalletRedeemCodeDialog.test.ts tests/shared/wallet/wallet-redeem-code.test.ts
```

Expected: FAIL，报告兑换弹窗或资金来源缺失。

- [ ] **Step 3: 实现钱包入口**

弹窗使用 `WalletApi.redeem({ code })`，不把 code 写入路由、标题、store 或日志。兑换响应成功后先以响应内的 `wallet` 更新概览、清空输入、关闭弹窗并显示到账金额，资金成功事实不得依赖后续查询。随后复用页面现有的 `summary` 与 `getList()` 做 best-effort 并行刷新：

```ts
summary.value = result.wallet
const [summaryRefresh, transactionRefresh] = await Promise.allSettled([
  WalletApi.summary(),
  getList(),
])
if (summaryRefresh.status === 'fulfilled') summary.value = summaryRefresh.value
if (summaryRefresh.status === 'rejected' || transactionRefresh.status === 'rejected') {
  ElMessage.warning(t('payment.wallet.redeemRefreshPartial'))
}
```

刷新异常不得进入兑换提交的失败分支、不得重新打开弹窗或用“兑换失败”覆盖已经显示的成功消息。

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

各 Task 已在最接近改动的位置运行一次对应的短检查，交付阶段不得为了“最终确认”把这些 targeted suites 再完整重复一遍。只汇总每个 Task 最近一次命令、退出码和对应提交；若某文件在其最近一次检查后又被修改，才重跑覆盖该文件的最小一条命令。

交付收尾自动命令仅为两个仓库各一次静态 whitespace 检查：

```powershell
# E:/admin/admin_back_go
git diff --check

# E:/admin/admin_front_ts
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

发布顺序固定为：提交并同步同一 contract -> 进入受控 Admin 发布窗口，避免迁移授予菜单后旧前端短暂暴露未知 view -> 执行迁移 -> 部署后端并通过现有 principal `Reconcile` 启动门槛 -> 切走旧后端 -> 发布前端 -> 恢复 Admin 流量 -> 用测试码做一次生成、兑换、资金明细和未触发限流的本人重放 smoke。回滚应用时保留新表、码状态、余额和流水，不逆向扣款。
