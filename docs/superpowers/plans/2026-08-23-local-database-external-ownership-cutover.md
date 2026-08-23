# 本地开发数据库外置所有权切换实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax to track progress.

**Goal:** 在 Wave 03 Permission + AuthPlatform 完成并验收后，彻底删除个人开发仓库中的数据库维护链，把当前本地 MySQL 设为唯一事实源。

**Architecture:** 运行时代码仍通过 `internal/infra/database` 连接 MySQL；仓库不再包含 schema、seed、migration、baseline 或数据库生命周期命令。work-ai 直接对已确认的本地 `admin` 数据库执行最小 SQL，验证数据库事实后提交应用代码。

**Tech Stack:** Go 1.26.5、GORM、MySQL 8.4、Docker Compose、PowerShell 7、Git。

---

## 执行边界

本计划只能在以下条件全部满足后开始：

- Wave 03 Permission + AuthPlatform 已由 work-ai 完成并合并到两个仓库 `master`；
- 用户已经人工验收该批次；
- 当前窗口没有其他 agent 正在修改本计划列出的文档、数据库脚本或 `cmd/admin-db`；
- 不启动或重启 `admin-dev`，不运行 `go test ./...`、全量前端测试、Playwright、`verify:frontend` 或发布长脚本；
- 本计划不执行备份、导出、恢复或远程数据库操作；这是个人本地开发阶段的明确风险接受。

目标检查不是备份：执行任何 SQL 前，必须确认连接目标是当前本机 Docker 的 `admin` 数据库。DSN 主机、端口、数据库名或 Compose 身份不匹配时立即停止，禁止猜测或切换连接。

本计划不删除业务表、业务字段、业务索引或用户数据；只删除仓库维护文件、`cmd/admin-db` 和确认没有运行时读者的 `schema_migrations` 治理表。

## 文件地图

**Create:**

- `E:/admin/admin_back_go/docs/database-ownership.md`：日常开发规则、直接 SQL 变更协议和未来重新引入正式治理的触发条件。

**Modify:**

- `E:/admin/admin_back_go/README.md`：删除仓库 SQL 初始化、迁移和 baseline 命令，改为说明当前数据库由本地 MySQL 持有。
- `E:/admin/admin_back_go/CONTEXT.md`：移除数据库文件哈希和迁移账本作为事实源的描述。
- `E:/admin/admin_back_go/docs/architecture.md`：保留 MySQL 业务真相和 `internal/infra/database`，删除仓库 schema/migration 依赖。
- `E:/admin/admin_back_go/AGENTS.md`：加入“不得新增 database/、migration、seed 或 baseline”的当前阶段约束。
- `E:/admin/admin_back_go/docs/superpowers/specs/2026-08-13-admin-architecture-reduction-direction.md`：同步第 11、18、20、21 节。
- `E:/admin/admin_back_go/docs/superpowers/plans/2026-08-13-admin-architecture-reduction-execution-index.md`：同步数据库基线状态、Wave 入口和删除边界。
- 仍被主动脚本或测试引用的发布/重启脚本：删除数据库 gate，不删除与应用启动本身无关的其他发布能力。
- `E:/admin/admin_back_go/internal/architecture/platform_kernel_test.go` 及其他只读取仓库 SQL 的测试：改为运行时/模块语义测试或删除低价值文件路径断言。

**Delete:**

- `E:/admin/admin_back_go/database/` entire directory；
- `E:/admin/admin_back_go/scripts/database.ps1`；
- `E:/admin/admin_back_go/scripts/verify-database.ps1`；
- `E:/admin/admin_back_go/scripts/tests/database-baseline.tests.ps1`；
- `E:/admin/admin_back_go/cmd/admin-db/` entire directory；
- 只验证迁移源码、baseline 哈希、seed 文本或数据库目录存在性的测试。

历史 superpowers 计划可以保留为历史记录，但必须在总索引中标记为“旧数据库治理方案，已被本设计取代”，不能再作为执行指令。

## Task 1：锁定 Wave 03 完成后的文档基线

**Files:**

- Read-only: 两仓库 `git status --short`、`git rev-parse HEAD`、Wave 03 最终交接记录；
- Read-only: `docs/superpowers/specs/2026-08-23-local-database-external-ownership-design.md`；
- Read-only: `docs/superpowers/plans/2026-08-15-admin-architecture-reduction-wave-03-permission-auth-platform.md`。

- [ ] **Step 1: 确认当前批次已合并且工作区干净**

~~~powershell
git -C E:/admin/admin_back_go status --short
git -C E:/admin/admin_back_go rev-parse HEAD
git -C E:/admin/admin_front_ts status --short
git -C E:/admin/admin_front_ts rev-parse HEAD
~~~

Expected：两个仓库工作区为空；Wave 03 的提交已在 `master`。若未满足，停止，不删除任何文件。

- [ ] **Step 2: 记录当前数据库只读身份**

读取当前已忽略的 `deploy/docker-first/admin-go.env`，只核对 DSN 的主机、端口和数据库名，不打印密码；通过 Docker inspect 确认 Compose 项目和 MySQL 容器身份。

- [ ] **Step 3: Commit**

本 Task 不修改代码，不单独提交。

## Task 2：移除运行时无关的数据库维护入口

**Files:**

- Delete: `database/`、`scripts/database.ps1`、`scripts/verify-database.ps1`、`scripts/tests/database-baseline.tests.ps1`、`cmd/admin-db/`；
- Modify: `README.md`、`CONTEXT.md`、`docs/architecture.md`、`AGENTS.md`、`docs/database-ownership.md`。

- [ ] **Step 1: 先写主动引用失败守卫**

新增一个短 PowerShell 文本检查，要求主动代码和脚本（排除 `docs/superpowers/plans` 历史文件）不再引用：

~~~text
database/schema.sql
database/seed.sql
database/migrations
database/baseline.json
scripts/database.ps1
cmd/admin-db
~~~

删除旧文件后运行该检查应先失败，证明它确实覆盖了旧引用；不要让检查扫描自身或历史计划。

- [ ] **Step 2: 删除文件并清理文档命令**

使用 `git rm` 删除上述文件；README 只保留 Docker 启动、`admin-dev`、运行时 MySQL 连接说明。禁止添加新的数据库管理脚本、初始化命令或 admin CLI 替代品。

- [ ] **Step 3: 写 `docs/database-ownership.md`**

文档必须明确：

~~~text
MySQL = 当前业务事实
internal/infra/database = Go 连接适配层，不是数据库实例
database/ 目录 = 当前阶段不存在
数据库变更 = work-ai 直接 SQL + 读回验证
私有 SQL 导出 = 仓库外、用户自行管理
~~~

同时写明禁止远程 DSN、禁止把密码写入 SQL/日志、禁止为新模块创建 migration 文件。

- [ ] **Step 4: Run short check and commit**

~~~powershell
rg -n --glob '!docs/superpowers/plans/**' --glob '!docs/superpowers/specs/**' "database/schema\.sql|database/seed\.sql|database/migrations|database/baseline\.json|scripts/database\.ps1|cmd/admin-db" E:/admin/admin_back_go
git diff --check
git add -A -- README.md CONTEXT.md docs/architecture.md AGENTS.md docs/database-ownership.md database scripts/database.ps1 scripts/verify-database.ps1 scripts/tests/database-baseline.tests.ps1 cmd/admin-db
git commit -m "refactor(database): remove personal development migration surface"
~~~

Expected：`rg` 无主动代码命中；提交只包含数据库维护入口和文档，不包含业务模块改动。

## Task 3：清理数据库依赖测试和发布门禁

**Files:**

- Modify/Delete: `internal/architecture/database_baseline_test.go`、`internal/architecture/platform_kernel_test.go`；
- Modify: `scripts/release/check-platform-kernel.ps1`、`scripts/release/check-release-manifest.ps1`、`scripts/release/new-release-manifest.ps1`、`scripts/release/deploy-admin-only.ps1`、`scripts/release/verify-admin-only-release.ps1`；
- Modify/Delete: 只因仓库 SQL 存在而失败的 `scripts/tests/*.tests.ps1`。

- [ ] **Step 1: 分类引用**

用 `rg -l` 列出每个引用，分为运行时、发布门禁、低价值文件路径断言和历史文档。运行时代码不得因为本 Task 获得新的数据库兜底或自动迁移。

- [ ] **Step 2: 删除低价值基线断言**

删除只验证 schema/seed 文件、migration 字符串、baseline 哈希、Atlas/HCL 路径的测试；保留 Repository SQL、事务、唯一键、外键、状态转换和 `/ready` 语义测试。

- [ ] **Step 3: 移除发布数据库字段**

发布脚本不再读取 `database/baseline.json`，不再计算 schema/seed/migration checksum，不再调用 `scripts/database.ps1`。当前项目是本地开发，发布脚本不得因此新增替代 gate。

- [ ] **Step 4: Run focused checks and commit**

~~~powershell
go test ./internal/infra/database ./internal/module/permission ./internal/module/auth_platform -count=1
git diff --check
git add internal scripts/release
git commit -m "chore(database): remove baseline and release gates"
~~~

不运行全量 Go 测试或发布 rehearsal；若定向测试失败，只修复本 Task 引入的引用错误。

## Task 4：清理本地数据库治理表

**Files:**

- No source file changes unless a runtime reader is discovered；
- Direct local MySQL SQL executed by work-ai。

- [ ] **Step 1: Prove no runtime reader**

~~~powershell
rg -n --glob '*.go' --glob '*.ps1' --glob '*.ts' "schema_migrations|atlas_schema_revisions|schema_reconciliation_runs" E:/admin/admin_back_go E:/admin/admin_front_ts
~~~

Only historical documentation may remain. Any active reader blocks the `DROP TABLE` statement and must be removed in a separate small code commit.

- [ ] **Step 2: Inspect exact local target**

Confirm the current MySQL container belongs to the local `admin-state` Compose project and the selected schema is exactly `admin`. Do not print credentials and do not continue for a remote host.

- [ ] **Step 3: Execute minimal SQL**

~~~sql
SELECT TABLE_NAME
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'schema_migrations';

DROP TABLE IF EXISTS `schema_migrations`;

SELECT COUNT(*) AS remaining
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'schema_migrations';
~~~

Expected: the final count is `0`. Do not drop any business table or truncate any business data.

- [ ] **Step 4: Commit only if code changed**

No source commit is created for a successful direct SQL cleanup. Report the exact SQL result and local target identity in the handoff.

## Task 5：同步总索引和历史入口

**Files:**

- Modify: `docs/superpowers/specs/2026-08-13-admin-architecture-reduction-direction.md`；
- Modify: `docs/superpowers/plans/2026-08-13-admin-architecture-reduction-execution-index.md`；
- Modify: historical database plans only to add a superseded notice, without restoring executable commands。

- [ ] **Step 1: Rewrite database ownership section**

The active direction must say that `database/` is absent in personal-development mode, MySQL is authoritative, and `internal/infra/database` is only the Go connection layer.

- [ ] **Step 2: Rewrite command and script tree**

The active target contains `cmd/admin-api` and `cmd/admin-worker`; `cmd/admin-db` and a replacement `admin-cli` are not required in this phase. The active script list excludes database lifecycle scripts.

- [ ] **Step 3: Mark old baseline as superseded**

The index must show the old baseline tag, seed hash, migration version and recovery dump as historical facts, not current execution inputs. It must point to this design and plan.

- [ ] **Step 4: Commit**

~~~powershell
git diff --check
git add docs/superpowers/specs/2026-08-13-admin-architecture-reduction-direction.md docs/superpowers/plans/2026-08-13-admin-architecture-reduction-execution-index.md docs/superpowers/specs/2026-08-23-local-database-external-ownership-design.md docs/superpowers/plans/2026-08-23-local-database-external-ownership-cutover.md
git commit -m "docs(database): define external ownership cutover"
~~~

## Task 6：最终短验证与人工验收

**Files:**

- No planned source changes.

- [ ] **Step 1: Verify active references**

~~~powershell
rg -n --glob '!docs/superpowers/plans/**' --glob '!docs/superpowers/specs/**' "database/|database\\|schema_migrations|admin-db|scripts/database" E:/admin/admin_back_go
~~~

Expected：只剩 `internal/infra/database`、MySQL runtime references、历史说明和本设计明确允许的字符串；不存在仓库 SQL 文件路径或迁移执行入口。

- [ ] **Step 2: Run focused short tests**

~~~powershell
go test ./internal/infra/database ./internal/module/permission ./internal/module/auth_platform -count=1
git -C E:/admin/admin_back_go diff --check
git -C E:/admin/admin_back_go status --short
~~~

不要运行全量测试、Playwright、`admin-dev` 或长发布脚本。

- [ ] **Step 3: User manual acceptance**

用户自行启动 `admin-dev` 后验收：登录、用户/角色/权限、系统设置、AuthPlatform 页面、AI 工具和官方模型页面。数据库连接失败必须由 `/ready` 明确报告，不能静默回退内存或旧 schema。

## 完成标准

- `database/`、`cmd/admin-db`、数据库生命周期脚本和迁移门禁已从主动仓库删除；
- MySQL 业务数据、表结构和运行时连接未被破坏；
- `schema_migrations` 没有运行时读者并已从当前本地数据库清理；
- 总方向、执行索引和本计划不存在“未来继续新增 migration”的矛盾描述；
- 后端和前端工作区干净，相关提交已合并到 `master`；
- 用户人工验收通过后，才进入下一个业务 Wave。


