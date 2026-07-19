# Admin Browser-only 与 Tauri 退役设计

**状态：** 已确认方向，等待用户审阅本文后进入实施计划编写
**日期：** 2026-07-19
**涉及仓库：** `E:/admin/admin_back_go`、`E:/admin/admin_front_ts`

## 1. 决策

Admin 产品只保留浏览器客户端。Tauri、Windows 桌面客户端、Rust/NSIS 构建、桌面凭证传输、原生更新与候选发布链路全部退役，不引入 Electron、PWA 桌面壳或其他替代客户端。

该决策覆盖并替代既有设计中关于以下内容的约定：

- Admin browser/desktop 双客户端；
- `ClientVariant`、`browser|desktop` 和 `X-Admin-Client-Variant`；
- Tauri `NativeBridge`、Windows Credential Manager、托盘、窗口命令、原生通知、受管下载和 updater；
- P08.5 的 tag → Windows 构建/签名 → COS candidate → 后台导入/晋级；
- P09 发布证明中的 Tauri candidate、updater manifest 和 Tauri Workflow。

历史 P08 实施记录与 Git 提交保留用于审计，不重写历史；其运行时成果由新的 P08R 退役阶段移除。

## 2. 目标

最终系统满足以下不变量：

1. Admin 只有一个浏览器运行时和一套认证传输。
2. Web、API、Worker、MySQL、Redis 继续只通过 Docker 运行和验收。
3. 前端不存在 Tauri/Rust/NSIS、桌面分支、客户端变体分支或误导性的 native 命名。
4. 后端 Admin 登录、刷新、退出只实现浏览器 Cookie/Origin 契约。
5. `客户端版本` 菜单、前后端运行时代码、接口、权限和正式契约在 P08R 退役。
6. `client_versions` 表和历史版本记录在 P08R 只冻结，不物理删除；P09 在恢复证据和用户再次授权后执行物理删除。
7. P08.5 取消；P07 Task 6–10 与 P09 在执行前改写为 Browser-only。

## 3. 命名与边界

### 3.1 保留的 Admin 含义

`admin` 仍是 REST 路径 scope、认证平台 code、RBAC/菜单所属产品和 Docker 服务的业务名称。Browser-only 不等于删除所有 `admin` 命名。

### 3.2 必须删除的双端命名

以下概念从生产代码、正式接口和当前计划中删除：

- `ClientVariant`、`clientVariant`、`ClientBrowser`、`ClientDesktop`；
- `browser|desktop` 判别联合类型；
- `X-Admin-Client-Variant`；
- `DesktopCredentialBridge`、`BrowserCredentialBridge` 这类仅为双端分派存在的接口；
- `NativeBridge`、`TauriManager`、`tauriStore`、`isTauri`；
- `src/adapters/tauri`、`src-tauri`、Tauri 专属测试/脚本/依赖。

浏览器安全机制可以使用准确的领域名称，例如 Cookie credential、Origin policy、external navigation、browser notification 和 browser download；不得再建立一个只有单实现的通用 `ClientVariant` 或 `NativeBridge` 抽象。

## 4. Browser-only 认证契约

### 4.1 唯一凭证传输

- login/refresh/logout 使用浏览器安全契约，不再读取客户端变体 header。
- login/refresh 的 JSON 成功响应只包含正式文档规定的 access credential 字段；不再出现桌面专用 `refresh_token` 或 `refresh_expires_in`。
- refresh credential 只存在于 `HttpOnly + Secure + SameSite=Strict` 的限定路径 Cookie。
- refresh 只读取 Cookie，不读取 JSON refresh token，不增加兼容字段或备用来源。
- login/refresh/logout 继续要求精确允许的 Web Origin；未知、缺失或 Tauri 本地 Origin 必须失败。
- CORS allowed headers、OpenAPI、后端 DTO、前端生成类型和测试同步删除 `X-Admin-Client-Variant`。

### 4.2 会话策略

当前 Admin 实际策略为 `single_session=1`、`max_sessions=1`。Session Lifecycle 按 `user_id + platform(admin)` 管理会话，client variant 从未形成独立会话域。

P08R 不擅自改变这一业务策略：同一用户仍只保留一个 Admin 会话。是否允许多浏览器会话属于后续独立产品决策，不能夹带在 Tauri 删除中。

### 4.3 切换与旧客户端失效

前后端 Browser-only 镜像在同一维护切换中发布：

1. 新后端只接受允许的 Web Origin 和 Cookie refresh；旧 Tauri Origin 无法重新 login/refresh。
2. 切换时撤销现有 Admin sessions，使旧桌面 access token 不继续存活到自然过期。
3. 新 Web 重新登录并建立唯一浏览器会话。
4. 记录 Windows Credential Manager 中历史 `cn.zgm2003.admin.refresh/current-session` 的清理步骤和核对方法。

不得通过保留 `desktop` alias、忽略旧 header、双写 Cookie/JSON 或猜测 User-Agent 来实现过渡。

## 5. 前端退役范围

P08R 删除或重构以下边界：

- `src-tauri/`、`rust-toolchain.toml`、Cargo/Tauri/NSIS/updater 配置；
- `@tauri-apps/*`、Tauri CLI 和仅为 candidate 发布存在的依赖；
- Tauri adapter、NativeBridge 类型、TauriManager、窗口控件、托盘/重启/退出分支；
- Windows OS credential、桌面 refresh、原生下载、原生通知和 updater 调用；
- Tauri 专属 store、preferences 字段、环境判定、测试、PowerShell 门禁和人工安装清单；
- `客户端版本` 页面、API wrapper、本地 view loader、菜单文案与权限引用；
- P08.5 Workflow、version/candidate/COS upload 计划产物（若尚未创建则由门禁保证继续不存在）。

仍有 Web 业务价值的能力按领域保留，而不是连带删除：

- 浏览器下载；
- 浏览器通知或站内通知；
- HTTPS 外链校验；
- 全局网络状态提示；
- 普通对话框、主题、语言和用户偏好。

## 6. 后端与正式契约退役范围

P08R 先修改正式后端文档，再修改运行时代码和生成契约，严格遵守“文档即唯一契约”。

必须退役：

- Admin `ClientVariant` 解析、header validation 与 desktop presenter 分支；
- desktop JSON refresh credential 请求/响应形状；
- client-version 管理、候选导入、updater manifest、公开更新查询相关路由与权限；
- `客户端版本` 菜单/view/permission 的正式目录项；
- 仅供 Tauri release/candidate 使用的 COS reader/publisher 和配置路径；
- Admin Contract Bundle 中对应 operation、schema、permission 和 view。

前端只能在新 bundle 发布后同步并删除调用，不得保留旧字段兼容。

## 7. 客户端版本数据处置

### 7.1 P08R：运行时退役、数据冻结

- 通过可重复执行且可核对的数据库 reconciliation 退役客户端版本菜单、view 和权限授权，确保 UI 不再出现入口。
- 删除所有 client-version 运行时读写入口后，将 `client_versions` 视为冻结历史表。
- 不在 P08R `DROP TABLE`，不批量删除历史版本记录，不删除 COS 历史对象。
- 验证不存在活动路由、任务、菜单、角色授权或运行时代码继续引用该表。

### 7.2 P09：物理删除

P09 只有在以下条件全部满足后才能删除 `client_versions` 表和相关约束：

- 最新数据库恢复 artifact 已实际恢复并核对 fingerprint；
- 历史版本记录数量与处置报告已记录；
- COS 历史对象保留/删除策略已明确；
- P08R 和新版 P07 Task 6–10 已通过；
- 用户在破坏性步骤前再次明确批准。

## 8. 网络与离线行为

Browser-only Admin 是在线业务系统，不承诺离线业务模式，也不使用 mock、旧数据或生成数据掩盖网络错误。

- 已加载页面遇到浏览器 offline/online 事件时，全局网络状态 UI 必须在登录页和已登录布局都可见。
- `navigator.onLine` 只表达浏览器网络提示，不得把 API 401、500、契约错误或后端不可用统一伪装为“无网络”。
- API 请求失败继续进入明确 error 状态；网络恢复后由现有业务刷新/重连策略处理。
- 首次打开 Web 且 Docker frontend 不可访问时，由浏览器显示连接失败；系统不伪装成可离线启动的桌面应用。
- P07 Task 6–10 必须保留并测试网络 Hook、全局提示挂载和恢复行为。

## 9. 发布与部署

P08.5 整体取消，不创建 Tauri GitHub Workflow、tag grammar、Windows runner、签名密钥、NSIS candidate、COS candidate 或后台 candidate import。

最终发布单元只有：

- frontend：revision-labelled Docker image；
- API/Worker：revision-labelled Docker image；
- database：具备恢复证据和 stop point 的 Atlas contract groups。

Web/API/Worker 不使用 GitHub deployment Workflow。构建、启动、健康检查、回滚和验收继续走现有 Docker-first 工具。

## 10. 计划重排

书面规格确认后，先只修改计划文档，不修改运行时代码：

1. 新建 **P08R Browser-only/Tauri 退役实施计划**；
2. 将 P08 标记为已实施但被 P08R supersede，保留审计记录；
3. 将 P08.5 标记为取消且禁止执行；
4. 重写 P07 Task 6–10，删除 Tauri adapter/client-version 假设并保留 Web 质量、无障碍、bundle、Docker 和人工验收；
5. 重写 P09 prerequisites、release manifest、rollback/proof 和数据库 contract，删除所有 Tauri candidate/Workflow 依赖；
6. 更新总执行索引、依赖图、Gate F/G 和共享状态所有权。

新的执行顺序固定为：

```text
Browser-only 规格审阅
→ 计划重写与审阅
→ P08R（先后端正式契约，再前端同步与退役，最后联合 Docker 切换）
→ 新版 P07 Task 6–10
→ 新版 P09
```

## 11. 验证与验收

### 11.1 静态边界

- 生产前端没有 Tauri/Rust/NSIS、desktop variant、NativeBridge 和 `@tauri-apps/*`。
- 后端正式契约没有 client variant header、desktop refresh 字段或 client-version operation。
- Admin view/permission bundle 与数据库菜单均无客户端版本入口。
- 仓库没有 Tauri Workflow，且没有 Web/backend deployment Workflow。

### 11.2 行为边界

- 浏览器 password/code login、cookie refresh、logout 和 session revoke 通过。
- 缺失/错误 Origin 被拒绝，JSON refresh token 被拒绝。
- 同一用户的 single-session 行为保持不变。
- 旧 Tauri Origin 无法 login/refresh；切换前 session 被撤销。
- 登录页与 Layout 的网络提示、恢复行为和 API error 区分通过。
- 浏览器下载、通知、外链和普通 UI 不因删除 NativeBridge 而回归。
- 客户端版本菜单、路由和 API 均不可达，冻结表无运行时引用。

### 11.3 门禁

- 后端正式文档、runtime tests、Admin Contract Bundle、Go 全门禁通过；
- 前端 contract sync、类型检查、全量测试、零错误 lint 目标、production build 和 Docker smoke 通过；
- 两仓只使用现有 `master` checkout，状态干净且无 `.worktrees`；
- 用户完成功能性检查后，才允许进入新版 P07 Task 6。

## 12. 回滚

- 不重写或删除 P08 Git 历史，回滚可以定位到 P08R 前的提交。
- Browser-only 切换前保留前后端上一版 Docker image 和数据库恢复 artifact。
- P08R 不删除 `client_versions` 表和 COS 历史对象，因此运行时代码回滚不依赖反向 DDL。
- 若联合切换失败，恢复上一版 frontend/API/Worker 镜像并按已验证的会话恢复流程重新登录；不发布半套 Browser-only 契约。
- P09 物理删除后的回滚只允许使用已验证恢复 artifact，不编造 reverse DDL。

## 13. 非目标

- 改变 Admin `single_session` / `max_sessions` 产品策略；
- 引入新的桌面容器、PWA、Electron 或移动客户端；
- 提供离线业务数据或运行时 mock；
- 在 P08R 提前执行 App/Canvas 的 P09 破坏性删除；
- 在没有正式后端文档和 bundle 的情况下让前端猜测新认证字段；
- 删除历史 Git 记录来伪装 Tauri 从未存在。
