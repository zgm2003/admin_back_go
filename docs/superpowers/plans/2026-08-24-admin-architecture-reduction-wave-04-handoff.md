# Wave 04 Handoff：运行与存储收口

日期：2026-08-24

状态：已完成，用户人工黑盒验收通过。

## 提交

Backend `E:/admin/admin_back_go`：

```text
8ae73f7 refactor(worker): remove empty static schedule adapter
b39fec4 refactor(realtime): share publisher boundary
```

Frontend `E:/admin/admin_front_ts`：

```text
ea23d84 refactor(realtime): move client to utils
6e20b66 refactor(upload): move direct COS client to utils
```

## 实际验证

Backend：

```text
PASS go test ./internal/jobs ./internal/infra/taskqueue ./internal/runtime -run 'Test(NewRegistry|Registry|Noop|Worker|Task|RealtimePublisher)' -count=1
PASS go test ./internal/architecture -run '^TestWave04WorkerHasNoStaticScheduleAdapter$' -count=1
PASS go test ./internal/infra/realtime ./internal/module/realtime ./internal/runtime -run 'Test(Envelope|Subscription|Resume|Realtime|Redis|Worker)' -count=1
PASS go test ./internal/architecture -run '^TestWave04WorkerDoesNotOwnRealtimeSessions$' -count=1
PASS go test ./internal/module/uploadtoken ./internal/infra/storage/cos -run 'Test(CreateRejectsNonCOSDriver|CreateDoesNotExposeDriverSecrets|SignerRejectsInvalidInput|SignerCallsSTSWithScopedPolicy)' -count=1
PASS git diff --check
```

Frontend：

```text
PASS npx vitest run --no-file-parallelism tests/unit/realtime tests/integration/realtime tests/integration/app/kernel.test.ts tests/integration/features/ai-chat.test.ts tests/integration/features/ai-runs.test.ts tests/integration/features/notifications.test.ts
PASS Realtime：10 files / 88 tests
PASS npx vitest run --no-file-parallelism tests/unit/upload tests/shared/system/upload-client-url.test.ts tests/shared/system/upload-rule-selection.test.ts
PASS Upload：3 files / 7 tests
PASS npx eslint src/utils/realtime src/app/kernel.ts src/main.ts src/adapters/web/websocket.ts src/api/ai/billing-error.ts src/api/ai/chat-events.ts src/features/ai-chat/workflow.ts src/features/ai-runs/workflow.ts
PASS npx eslint src/utils/upload src/components/UpFile src/components/UpMedia src/views/Main/component/upload src/views/Main/component/display/components/Editor.vue src/views/Main/ai/chat/components/MessageInput/use-attachments.ts src/api/ai/context.ts
PASS git diff --check
PASS rg legacy Realtime/Upload paths：无匹配
```

ESLint 无错误；`src/features/ai-chat/workflow.ts` 保留既有 max-lines warning。

## 用户人工验收

用户确认以下行为正常：

```text
PASS 登录后 Realtime 连接、断线重连、AI/通知终态刷新
PASS 文件、图片、AI 附件和导出继续直传 COS
PASS 上传取消或失败时页面不显示成功
PASS Worker 队列任务和数据库定时任务行为正常
```

## 未运行

本波未启动、停止或重启 `admin-dev`，未执行 SQL 或数据库迁移，未运行：

```text
go test ./...
全量 Vue/Vitest 测试
全量 typecheck
Playwright
verify:frontend
发布及其他长脚本
```

## 计划外问题

无影响本波的计划外问题。并行 Vitest 隔离架构、既有 max-lines warning 均未扩大处理。

## 范围确认

本波没有修改数据库表、字段、业务数据、Admin Contract、合同生成物、AI 业务或支付业务；没有新增 migration、seed、baseline、上传 complete API、上传元数据表或 API 文件中转。

## 下一入口

Wave 05 支付与钱包仍按独立设计和人工验收门执行。本窗口在 Wave 04 停止，不自动进入 Wave 05 或 Wave 06。
