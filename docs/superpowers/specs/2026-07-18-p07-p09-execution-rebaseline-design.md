# P07-P09 执行重基线设计

**日期：** 2026-07-18
**状态：** 用户已于 2026-07-18 确认
**范围：** P07、P08、新增 P08.5、P09 的计划边界和执行规则

## 1. 背景

P01-P06 已经形成 Admin 后端、前端内核、Docker 运行平台和基础发布证据，但后续子计划仍残留已失效的假设：

- P07 把 Playwright 当作固定依赖和阻塞门禁；
- P08 把 Tauri 安全重构和发布自动化混在同一阶段；
- P09 仍引用 Git worktree、Web/后端 GitHub Workflow 和 Playwright；
- 总计划已经改为 Docker-only 和直接 `master`，子计划却没有全部同步；
- 当前 Tauri 版本需要在多个文件手动同步，安装包、签名和版本记录也依赖人工串联。

这些冲突必须在 P07 开始前消除。后续计划不得以旧文档为理由恢复已明确废弃的交付方式。

## 2. 已确认的不可变决策

### 2.1 仓库与分支

- 只使用 `E:/admin/admin_front_ts` 和 `E:/admin/admin_back_go` 的主检出目录。
- 两个仓库都直接在 `master` 串行执行。
- 禁止创建 Git worktree，禁止创建或保留 `.worktrees` 目录。
- 同一时刻不得让多个执行者修改同一仓库。
- 每个任务使用精确路径暂存并形成聚焦提交。

### 2.2 运行和部署

- Web、API、worker、MySQL、Redis 的应用运行时只通过 Docker Compose 启动。
- 禁止在宿主机启动 Vite、Go API、worker、MySQL 或 Redis。
- Web 和后端的正式交付单元是带 Git revision 标签的 Docker 镜像。
- Web 和后端不使用 GitHub Actions 进行部署。
- Windows Tauri 原生编译不是 Web/后端运行时部署；它允许在专用 GitHub Actions Windows runner 上执行。

### 2.3 Playwright

- P07-P09 不安装、不配置、不运行 Playwright，也不把它写入固定门禁。
- 不创建 `playwright.config.*`、`tests/e2e/` 或 Playwright Workflow。
- 只有用户在具体任务中明确要求调用 Playwright 时，才允许在该次任务边界内使用。
- 自动化无法覆盖的真实 UI 交互由用户依据版本化验收清单手工检查；计划不得把手工检查伪装成自动化证据。

### 2.4 接口契约

- `admin_front_ts/docs/rule.md` 继续作为前端接口集成硬规则。
- 后端最新正式文档和生成的 Admin Contract Bundle 是唯一接口契约。
- P08.5 如需新增候选版本接口，必须先更新后端正式文档、类型和契约测试，再修改前端。
- 禁止猜字段、兼容旧字段、运行时 mock 或错误降级为空态。

### 2.5 认证凭证

- 保持 P06 的浏览器凭证边界：access token 仅在内存，refresh credential 仅由后端通过 `HttpOnly` Cookie 管理。
- 前端不读取、不写入 token Cookie，不恢复 `js-cookie` 作为生产依赖。
- Tauri refresh credential 由 P08 存入操作系统安全存储，不进入 DOM、Pinia 持久化或普通文件。

## 3. 当前 Tauri 基线

当前仓库状态是：

- `package.json`、`package-lock.json`、`src-tauri/Cargo.toml`、`src-tauri/Cargo.lock` 和 `src-tauri/tauri.conf.json` 存在重复版本信息；
- 当前版本为 `1.0.7`；
- bundle 目标仅为 Windows NSIS；
- updater endpoint 为 `https://cos.zgm2003.cn/tauri_updater/{{target}}-{{arch}}.json`；
- 已提交 updater 公钥，私钥不在仓库；
- 当前 `frontendDist` 指向远程站点，且开启 `withGlobalTauri`，CSP 仍允许 `unsafe-inline` 和 `unsafe-eval`；
- 后端 `clientversion` 模块已经保存版本记录，并在“设为最新”时向 COS 发布正式 updater JSON；
- 当前后台页面需要人工上传安装包、填写版本/签名并手工设为最新。

P08 负责修正本地资源和原生安全边界；P08.5 负责将构建、签名和候选包上传自动化。两者不得再次合并为一个不可审查的大任务。

## 4. 新执行序列

```text
P06 用户人工验收
  -> P07 Tasks 1-5
  -> P08 Tauri 安全架构
  -> P07 Tasks 6-10
  -> P08.5 Windows Tauri 发布自动化
  -> P09 Admin-only 最终收口
```

P06 人工验收发现的问题优先作为 P06 缺陷修复，不得混入 P07。

## 5. P07：前端实时、资源与质量收口

### 5.1 保留范围

- typed `RealtimeClient`、断线恢复和事件顺序；
- latest-wins `ResourceQuery` 和明确的 loading/success/empty/missing/error 状态；
- mutation 与 query 分离；
- 页面迁移到共享 HTTP/realtime/resource 边界；
- 用行为测试替换源码字符串堆叠测试；
- 页面职责拆分、零 lint warning、懒加载和 bundle budget；
- WCAG 2.2 AA 的关键组件和键盘行为。

### 5.2 替换原浏览器任务

原 P07 Task 9 不再创建 Playwright 测试，改为三类证据：

1. Vitest unit/component/integration 测试验证状态机、请求竞态、错误状态、键盘事件和 ARIA；
2. Docker 环境中的 HTTP、WebSocket、健康检查和资源契约 smoke；
3. 版本化人工验收清单，列出登录、动态路由、菜单、表格、弹窗、通知、实时消息和关键响应式页面。

人工验收结果由用户决定是否通过，Agent 不得代替用户勾选。

### 5.3 P07 完成门禁

- Admin Contract Bundle hash 一致；
- ESLint 为零 warning；
- TypeScript、Vitest、生产构建和 bundle budget 通过；
- Docker 镜像构建并带当前 frontend HEAD revision；
- Docker API/WebSocket smoke 通过；
- 没有 Playwright 文件、依赖、命令或 Workflow；
- 用户确认 P07 人工验收清单。

## 6. P08：Tauri 安全架构

### 6.1 平台边界

- 仅支持 Windows x86_64 和 NSIS；
- 不在本阶段增加 macOS、Linux、MSI 或移动平台；
- Rust/Tauri 构建使用固定 Windows runner、固定 Rust 工具链和锁文件。

### 6.2 本地可信 UI

- `frontendDist` 改为构建产生的本地 `dist`；
- 关闭 `withGlobalTauri`；
- 删除 remote capability、`unsafe-eval` 和不必要的 `unsafe-inline`；
- 远程内容不得获得 invoke、shell、opener、filesystem、window 或 updater 能力。

### 6.3 原生边界

- TypeScript 只通过一个窄化 `NativeBridge` 调用原生能力；
- refresh credential 使用 Windows 安全凭证存储并且永不返回 DOM；
- 下载由 Rust 管理 URL、重定向、文件名、保存目录、校验和和临时文件清理；
- window、notification、process 和 updater 只暴露枚举后的命令；
- updater 只接受现有公钥验证通过的签名产物。

### 6.4 P08 完成门禁

- Rust fmt、Clippy、tests、audit 和 locked release build 通过；
- Tauri security source/config guards 通过；
- 本地资源包不依赖 Web 服务器运行；
- 没有远程 Tauri capability；
- 没有 Playwright；
- P08 不上传正式版本，也不切换 COS updater latest 指针。

## 7. P08.5：Windows Tauri 发布自动化

### 7.1 版本来源

- 版本采用 SemVer，不接受 `latest`、日期别名或任意字符串。
- 仓库提供单一版本更新命令，同时更新全部版本文件和 lockfile 中的根包版本。
- 正式发布标签格式固定为 `tauri-vX.Y.Z`。
- Workflow 必须验证标签版本与仓库文件完全一致，并验证该提交属于 `master`；不一致立即失败。

### 7.2 Workflow 边界

- 唯一允许新增的 GitHub Workflow 是 Tauri Windows 验证/候选发布 Workflow；它不得部署 Web 或后端。
- Workflow 只由 `tauri-v*` 标签触发。
- Windows job 运行前端静态门禁、Rust/Tauri 门禁、签名构建和产物校验；不启动 Vite 或任何后端服务。
- GitHub Actions 和第三方 actions 固定到审核过的 commit SHA。
- Tauri 私钥、私钥密码和 COS 凭证只存在于 GitHub protected environment secrets，不写入日志、缓存或 artifacts。

### 7.3 COS 候选版本

Workflow 按以下顺序发布：

1. 构建 NSIS `.exe` 安装器以及签名后的 `.nsis.zip` updater bundle；
2. 分别计算 SHA-256、文件大小和 immutable object key；
3. 上传到版本化候选目录；
4. 下载回读并核对大小和 SHA-256；
5. 最后上传候选 manifest。

候选 manifest 至少包含 schema version、SemVer、platform、Git commit、tag、安装器 object key/URL/SHA-256/file size、updater object key/URL/signature/SHA-256/file size 和创建时间。后端版本记录使用 updater bundle，不得把原始 `.exe` 冒充 updater artifact。失败时不得写正式 updater JSON。

### 7.4 后台导入与人工晋级

- 后端使用已有 COS 配置读取候选 manifest，不让 Workflow 持有 Admin token 或数据库凭证；
- 后端验证 schema、平台、SemVer、对象归属、URL origin、对象存在性、file size 和 SHA-256；
- 导入候选版本时创建普通版本记录，`is_latest=NO`、`force_update=NO`；
- 重复导入相同 platform/version 必须返回明确冲突，不得静默覆盖；
- 后台页面展示候选 commit、tag、hash 和文件大小；
- 只有用户点击现有“设为最新”动作后，后端才写数据库状态并发布正式 `tauri_updater/windows-x86_64.json`；
- 强制更新仍由用户单独确认，不从 tag 或 commit message 推断。

### 7.5 发布失败和回退

- 候选构建或上传失败不会影响当前 latest；
- 正式 manifest 始终最后切换；
- 已发布的版本化安装包不可覆盖；
- Windows 客户端不依赖降级更新。已推送错误版本后的恢复策略是发布更高 SemVer 的修复版本，而不是让客户端自动降级；
- 旧 artifacts 和版本记录按明确保留策略清理，不由 Workflow 猜测删除。

## 8. P09：Admin-only 最终收口

### 8.1 计划清理

P09 必须删除以下旧假设：

- Git worktree 和 `.worktrees`；
- Web/后端 GitHub deployment Workflow；
- Playwright/browser gate；
- 未版本化 Web 目录切换脚本；
- 将 Tauri 发布与 Web/后端 Docker 发布混成同一 artifact 的行为。

### 8.2 发布单元

- Web：revision-labelled Docker image；
- API/worker：revision-labelled Docker image；
- Tauri：P08.5 产生并经用户晋级的 signed Windows candidate；
- 数据库：有恢复证据和 stop point 的 Atlas contract groups。

### 8.3 破坏性阶段

P09 在执行任何删除数据、删除列/表或不可逆 contract DDL 前必须同时满足：

- P01-P08.5 已提交并通过各自门禁；
- 两仓主检出目录状态干净；
- 最新恢复 artifact 已实际恢复并核对 fingerprint；
- 所有 legacy row 都有明确 disposition；
- COS 保留对象可访问；
- Docker 镜像 revision 与锁定 commit 一致；
- 用户在破坏性步骤前再次明确批准。

缺少任一项时必须停止，不能用兼容层或默认值绕过。

## 9. 后续文档变更范围

本设计经用户审阅后，下一阶段只重写计划文档，不修改运行时代码：

- 更新总执行索引和依赖图；
- 重写 P07 的 Task 9、Task 10 和 completion gate；
- 重写 P08 的平台、运行、验证和 release 边界；
- 新建 P08.5 详细实施计划；
- 重写 P09 的 prerequisites、Workflow、验证、回滚和完成门禁；
- 把“Playwright 只能由用户明确点名调用”写入所有相关计划；
- 把“GitHub Actions 仅允许 Tauri Windows candidate release”写入总索引。

计划文档完成并再次经用户确认后，才允许开启 P07 Goal。

## 10. 非目标

- macOS、Linux 或移动端 Tauri；
- tag 后自动设为 latest 或自动开启 force update；
- GitHub Actions 部署 Web、API、worker 或数据库；
- 恢复 JS-readable token Cookie 或 `js-cookie` 生产依赖；
- 永久引入 Playwright；
- 在 P08/P08.5 中执行 P09 的 App/Canvas 删除或破坏性 DDL。
