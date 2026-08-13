# Admin 架构减法 Wave 01 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans or superpowers:subagent-driven-development to execute this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让角色权限编辑器自然地选择页面访问，并把 Realtime/AI 取消信号从 Redis 缓存 DB 0 隔离到 DB 1，不改变权限接口、数据库结构和已有业务语义。

**Architecture:** 页面权限 ID 仍然是现有 PAGE 节点 ID，页面名称前的 checkbox 直接操作它；按钮 checkbox 操作现有 BUTTON 节点，选按钮自动补页面。后端增加独立 Realtime Redis 客户端，API/Worker 的 WebSocket Pub/Sub 和 AI cancel publisher/subscriber 使用它，缓存、验证码、权限缓存和 scheduler lock 继续使用 DB 0。

**Tech Stack:** Vue 3.5 + TypeScript + Element Plus + Vitest；Go 1.26.5 + Redis go-redis/v9 + Gin runtime。

---

## 执行前检查

执行窗口必须先读取：

```text
E:/admin/admin_back_go/AGENTS.md
E:/admin/admin_back_go/docs/superpowers/specs/2026-08-13-admin-architecture-reduction-direction.md
E:/admin/admin_back_go/docs/superpowers/plans/2026-08-13-admin-architecture-reduction-execution-index.md
E:/admin/admin_front_ts/AGENTS.md（存在时）
```

确认两个仓库工作区没有不属于本波的未提交修改：

```powershell
git -C E:/admin/admin_back_go status --short
git -C E:/admin/admin_front_ts status --short
```

如果出现用户或其他窗口的新修改，停止并报告，不要覆盖。不要启动 `admin-dev`、不要运行 Playwright、不要运行全量 `go test ./...` 或 `npm run verify:frontend`。

---

## Task 1：锁定权限矩阵交互的失败测试

**Files:**

- Create: `E:/admin/admin_front_ts/tests/component/permission/RolePermissionMatrix.test.ts`
- Read-only reference: `E:/admin/admin_front_ts/src/views/Main/permission/role/components/RolePermissionMatrix.vue`
- Read-only reference: `E:/admin/admin_front_ts/src/views/Main/permission/role/role-matrix.ts`

- [ ] **Step 1: 写组件行为测试**

创建完整测试文件，使用最小 stub 展开 `el-table-column` 的作用域插槽，不依赖 Element Plus 内部 DOM：

```ts
import { defineComponent, h, inject, provide, type PropType } from 'vue'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import RolePermissionMatrix from '@/views/Main/permission/role/components/RolePermissionMatrix.vue'
import type { RoleMatrixGroup, RoleMatrixRow } from '@/views/Main/permission/role/role-matrix'

const tableRowsKey = Symbol('tableRows')

const ElTableStub = defineComponent({
  props: {
    data: { type: Array as PropType<RoleMatrixRow[]>, required: true },
  },
  setup(props, { slots }) {
    provide(tableRowsKey, props)
    return () => h('div', { class: 'el-table-stub' }, slots.default?.())
  },
})

const ElTableColumnStub = defineComponent({
  setup(_, { slots }) {
    const table = inject<{ data: RoleMatrixRow[] }>(tableRowsKey)
    if (!table) {
      throw new Error('ElTableColumnStub must be rendered inside ElTableStub')
    }
    return () => h('div', { class: 'el-table-column-stub' }, table.data.map((row) => slots.default?.({ row })))
  },
})

const ElCheckboxStub = defineComponent({
  props: {
    modelValue: { type: Boolean, default: false },
    indeterminate: { type: Boolean, default: false },
  },
  emits: ['update:modelValue'],
  setup(props, { emit, slots }) {
    return () => h('label', [
      h('input', {
        type: 'checkbox',
        checked: props.modelValue,
        onChange: (event: Event) => emit('update:modelValue', (event.target as HTMLInputElement).checked),
      }),
      slots.default?.(),
    ])
  },
})

const PassThroughStub = defineComponent({
  setup(_, { slots }) {
    return () => h('div', slots.default?.())
  },
})

const defaultGroups: RoleMatrixGroup[] = [{
  groupId: 1,
  groupLabel: '用户',
  platform: 'admin',
  rows: [{
    pageId: 2,
    pageLabel: '用户管理',
    pagePermissionId: 2,
    platform: 'admin',
    actions: [{ id: 4, code: 'user_edit', label: '编辑' }],
  }],
}]

function mountMatrix(groups: RoleMatrixGroup[] = defaultGroups) {
  return mount(RolePermissionMatrix, {
    props: {
      modelValue: [],
      groups,
      emptyText: '暂无权限',
      pageLabel: '页面',
      actionLabel: '动作',
      groupSelectLabel: '全选本组',
      groupClearLabel: '清空本组',
      selectedCountLabel: '已选',
      pageCountLabel: '页面',
      actionCountLabel: '动作',
      noActionsText: '无按钮操作',
      helperText: '页面名称控制页面访问',
      groupExpandLabel: '展开',
      groupCollapseLabel: '收起',
    },
    global: {
      stubs: {
        ElTable: ElTableStub,
        ElTableColumn: ElTableColumnStub,
        ElCheckbox: ElCheckboxStub,
        ElSpace: PassThroughStub,
        ElButton: PassThroughStub,
        ElEmpty: PassThroughStub,
      },
    },
  })
}

describe('RolePermissionMatrix', () => {
  it('renders page access on the page name, not as a fake action', () => {
    const wrapper = mountMatrix()

    expect(wrapper.findAll('.role-permission-matrix__page-access')).toHaveLength(1)
    expect(wrapper.findAll('.role-permission-matrix__view')).toHaveLength(0)
    expect(wrapper.text()).toContain('用户管理')
    expect(wrapper.text()).toContain('编辑')
  })

  it('keeps a page without actions directly selectable', async () => {
    const wrapper = mountMatrix([{
      groupId: 1,
      groupLabel: '用户',
      platform: 'admin',
      rows: [{
        pageId: 8,
        pageLabel: '登录日志',
        pagePermissionId: 8,
        platform: 'admin',
        actions: [],
      }],
    }])

    expect(wrapper.findAll('.role-permission-matrix__page-access')).toHaveLength(1)
    expect(wrapper.findAll('.role-permission-matrix__action')).toHaveLength(0)
    expect(wrapper.text()).toContain('无按钮操作')

    await wrapper.find('.role-permission-matrix__page-access input').setValue(true)
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([[8]])
  })

  it('selecting an action emits both its page and button ids', async () => {
    const wrapper = mountMatrix()

    await wrapper.find('.role-permission-matrix__action input').setValue(true)
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([[2, 4]])
  })

  it('does not render page access as an action', () => {
    const wrapper = mountMatrix()

    expect(wrapper.find('.role-permission-matrix__actions').text()).toBe('编辑')
  })
})
```

组件测试锁定模板事件与 `v-model` 的连线；现有 `tests/shared/permission/role-matrix.test.ts` 继续锁定纯数据规则，包括取消 PAGE 清除其按钮 ID。不要在组件测试里复制所有纯函数场景。

- [ ] **Step 2: 运行失败测试**

Run from `E:/admin/admin_front_ts`:

```powershell
npm test -- --run tests/component/permission/RolePermissionMatrix.test.ts
```

Expected: FAIL because the current component has no `.role-permission-matrix__page-access` and still renders `.role-permission-matrix__view`.

## Task 2：实现页面名称 checkbox

**Files:**

- Modify: `E:/admin/admin_front_ts/src/views/Main/permission/role/components/RolePermissionMatrix.vue:11-30,237-296`
- Modify: `E:/admin/admin_front_ts/src/views/Main/permission/role/components/RolePermissionMatrix.styles.css:47-104`
- Modify: `E:/admin/admin_front_ts/src/views/Main/permission/role/index.vue:122-146`
- Modify: `E:/admin/admin_front_ts/src/i18n/locales/zh-CN/permission.ts:1-20`
- Modify: `E:/admin/admin_front_ts/src/i18n/locales/en-US/permission.ts:1-20`

- [ ] **Step 1: 删除重复页面访问 prop 和动作列 checkbox**

从 `RolePermissionMatrix.vue` props 删除：

```ts
pageAccessLabel: string
emptyActionsText: string
```

删除动作列中绑定 `row.pagePermissionId` 的 checkbox。动作列只保留真实按钮权限：

```vue
<el-checkbox
  v-for="action in row.actions"
  :key="action.id"
  class="role-permission-matrix__action"
  :model-value="isChecked(action.id)"
  @update:model-value="(value: unknown) => setActionChecked(row, action.id, Boolean(value))"
>
  {{ action.label }}
</el-checkbox>
<span
  v-if="row.actions.length === 0"
  class="role-permission-matrix__no-actions"
>
  {{ noActionsText }}
</span>
```

`noActionsText` 使用用户可理解的“无按钮操作/No button actions”，不再出现“仅控制页面访问”这样的内部解释。

- [ ] **Step 2: 把页面 checkbox 放到页面名称列**

页面列使用现有 `setPageChecked` 和 `isChecked(row.pagePermissionId)`，实现如下结构。页面 checkbox 只表达 PAGE 权限本身，不能把“部分按钮已选”误画成页面半选；半选状态只用于目录汇总 checkbox：

```vue
<el-checkbox
  v-if="row.pagePermissionId"
  class="role-permission-matrix__page-access"
  :model-value="isChecked(row.pagePermissionId)"
  @update:model-value="(value: unknown) => setPageChecked(row, Boolean(value))"
>
  <div class="role-permission-matrix__page">
    <div class="role-permission-matrix__page-copy">
      <div class="role-permission-matrix__page-name">{{ row.pageLabel }}</div>
      <div class="role-permission-matrix__page-meta">
        <span>{{ actionCountLabel }} {{ rowRuntimeState(row).actionSelected }}/{{ row.actions.length }}</span>
      </div>
    </div>
  </div>
</el-checkbox>
<div v-else class="role-permission-matrix__page">
  <div class="role-permission-matrix__page-copy">
    <div class="role-permission-matrix__page-name">{{ row.pageLabel }}</div>
    <div class="role-permission-matrix__page-meta">
      <span>{{ actionCountLabel }} {{ rowRuntimeState(row).actionSelected }}/{{ row.actions.length }}</span>
    </div>
  </div>
</div>
```

根级 BUTTON 行（`pagePermissionId === null`）必须走上面的非 checkbox 分支，以继续支持 App 根级按钮；不要为它伪造页面权限。

- [ ] **Step 3: Update parent props and translations**

In `index.vue`, remove `:page-access-label` and `:empty-actions-text`, add `:no-actions-text="t('role.permissionMatrix.noActions')"`.

Replace translation keys:

```ts
// zh-CN
permissionMatrix: {
  helper: '目录只负责分组；页面名称控制页面访问，动作列只显示按钮权限。勾选动作会自动拥有页面访问。',
  selected: '已选',
  pages: '页面',
  actions: '动作',
  clearGroup: '清空本组',
  clearPlatform: '清空当前平台',
  noActions: '无按钮操作',
}
```

英文值固定为：

```ts
permissionMatrix: {
  helper: 'Directories are groups only. Page names control access, and the actions column contains button permissions only. Selecting an action grants page access automatically.',
  selected: 'Selected',
  pages: 'Pages',
  actions: 'Actions',
  clearGroup: 'Clear Group',
  clearPlatform: 'Clear Current Platform',
  noActions: 'No button actions',
}
```

不要修改 API 标签或权限码。

- [ ] **Step 4: Keep styles minimal**

删除 `.role-permission-matrix__view` 以及重复表达 checkbox 状态的 `.role-permission-matrix__page-status*` 样式，只加入以下样式；不要写 `:deep()`，也不要覆盖 Element Plus checkbox 的框、颜色或交互状态：

```css
.role-permission-matrix__page-access {
  width: 100%;
}

.role-permission-matrix__no-actions {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
```

- [ ] **Step 5: Run focused frontend tests**

```powershell
npm test -- --run tests/component/permission/RolePermissionMatrix.test.ts tests/shared/permission/role-matrix.test.ts
```

Expected: PASS. Existing shared matrix tests must remain unchanged and pass because permission data and toggle functions are not changed.

- [ ] **Step 6: Commit only the permission UI wave**

```powershell
git add src/views/Main/permission/role/components/RolePermissionMatrix.vue src/views/Main/permission/role/components/RolePermissionMatrix.styles.css src/views/Main/permission/role/index.vue src/i18n/locales/zh-CN/permission.ts src/i18n/locales/en-US/permission.ts tests/component/permission/RolePermissionMatrix.test.ts
git diff --cached --check
git commit -m "fix(rbac): simplify role permission matrix"
```

## Task 3：增加 Realtime Redis DB 配置

**Files:**

- Modify: `E:/admin/admin_back_go/internal/config/config.go:183-198,300-380`
- Modify: `E:/admin/admin_back_go/internal/config/runtime.go:35-70`
- Modify: `E:/admin/admin_back_go/internal/config/config_test.go:55-95`
- Modify: `E:/admin/admin_back_go/internal/config/env_test.go:90-110`
- Modify: `E:/admin/admin_back_go/internal/config/runtime_test.go:120-155`
- Modify: `E:/admin/admin_back_go/deploy/docker-first/admin-go.env.example:20-55`
- Modify: `E:/admin/admin_back_go/README.md:230-285,520-540,830-850`

- [ ] **Step 1: 写配置失败测试**

在 `config_test.go` 的默认配置测试中先加入：

```go
if cfg.Realtime.RedisDB != DefaultRealtimeRedisDB {
	t.Fatalf("expected realtime redis db %d, got %d", DefaultRealtimeRedisDB, cfg.Realtime.RedisDB)
}
```

在现有 `TestLoadReadsEnvironmentOverrides` 中设置并断言显式覆盖值，证明配置不是写死的：

```go
t.Setenv("REALTIME_REDIS_DB", "6")

if cfg.Realtime.RedisDB != 6 {
	t.Fatalf("expected realtime redis db override 6, got %d", cfg.Realtime.RedisDB)
}
```

在 `runtime_test.go` 的验证表中加入负数场景：

```go
{
	name: "realtime redis db",
	process: ProcessAPI,
	mutate: func(c *Config) { c.Realtime.RedisDB = -1 },
	want: "REALTIME_REDIS_DB",
},
```

在 `env_test.go` 的无效整数表中加入：

```go
{"REALTIME_REDIS_DB", "-1", "REALTIME_REDIS_DB: must not be negative"},
```

运行：

```powershell
go test ./internal/config -run 'TestLoadUsesSafeDefaults|TestValidate' -count=1
```

预期：FAIL，原因是 `RealtimeConfig` 尚无 `RedisDB` 字段或校验规则。

- [ ] **Step 2: 实现单一配置字段和校验**

在 `config.go` 增加：

```go
const DefaultRealtimeRedisDB = 1

type RealtimeConfig struct {
	Enabled           bool
	Publisher         string
	RedisDB           int
	HeartbeatInterval time.Duration
	SendBuffer        int
	RedisChannel      string
}
```

在 `loadFrom` 中使用 `envInteger(lookup, "REALTIME_REDIS_DB", DefaultRealtimeRedisDB, false)`，填入 `Config.Realtime.RedisDB`。在 `runtime.go` 的 `Validate` 流程中，调用 `validateRealtimeConfig` 前检查该值不得小于零，错误文本必须包含 `REALTIME_REDIS_DB`。不要把 Redis DB 号散落到实时模块或调用方，也不要用空值回退。

- [ ] **Step 3: 同步环境模板和文档**

在 `admin-go.env.example` 的 `REALTIME_ENABLED` 与 `REALTIME_PUBLISHER` 附近增加：

```dotenv
REALTIME_REDIS_DB=1
```

扩展 `config_test.go` 现有 Docker-first 环境合同测试：`admin-go.env.example` 必须显式固定为 1；本地存在的 `admin-go.env` 若声明该键，也必须是 1。旧本地 env 可以暂时缺少该键，由代码默认值兼容：

```go
if fileName == "admin-go.env.example" {
	if got := values["REALTIME_REDIS_DB"]; got != "1" {
		t.Fatalf("deploy/docker-first/%s must keep REALTIME_REDIS_DB=1, got %q", fileName, got)
	}
} else if got, exists := values["REALTIME_REDIS_DB"]; exists && got != "1" {
	t.Fatalf("deploy/docker-first/%s must use REALTIME_REDIS_DB=1 when present, got %q", fileName, got)
}
```

`deploy/docker-first/admin-go.env` 是 `.gitignore` 忽略的本地运行配置。执行者应把本机文件同步为 `REALTIME_REDIS_DB=1` 供当前开发环境显式可见，但禁止 `git add` 或提交。不要把该键加入 `scripts/admin-dev.ps1` 的 `requiredEnvironmentKeys`；旧环境未显式配置时必须继续由 `DefaultRealtimeRedisDB` 兼容为 DB 1。

将 README 中 Redis 角色表、环境变量示例、Readiness 说明和数据库 reset 说明统一改为 `DB 0/1/2/3`，明确 `REALTIME_REDIS_DB` 选择 WebSocket Pub/Sub 与 AI cancel 使用的逻辑库；`REALTIME_ENABLED` 只控制 WebSocket/实时事件能力，不控制 DB 1 客户端生命周期。不要修改已有 `REDIS_DB=0`、`TOKEN_REDIS_DB=2` 或 `QUEUE_REDIS_DB=3` 的语义。

- [ ] **Step 4: 运行配置定向测试并提交**

```powershell
go test ./internal/config -count=1
git diff --check
git add internal/config/config.go internal/config/runtime.go internal/config/config_test.go internal/config/env_test.go internal/config/runtime_test.go deploy/docker-first/admin-go.env.example README.md
git commit -m "feat(runtime): configure realtime redis database"
```

预期：配置包测试通过；提交只包含配置、模板和文档，不包含运行时资源或业务模块改动。

## Task 4：建立独立 Realtime Redis 资源并切换调用点

**Files:**

- Modify: `E:/admin/admin_back_go/internal/runtime/resources.go:35-230,300-410`
- Modify: `E:/admin/admin_back_go/internal/runtime/resources_test.go:230-430`
- Modify: `E:/admin/admin_back_go/internal/runtime/realtime.go:150-180`
- Modify: `E:/admin/admin_back_go/internal/runtime/api.go:135-180`
- Modify: `E:/admin/admin_back_go/internal/runtime/worker.go:155-235,380-395`
- Modify: `E:/admin/admin_back_go/internal/platform/admin/build.go:60-75,235-250,365-382`
- Modify: `E:/admin/admin_back_go/internal/platform/admin/build_test.go:330-345`

- [ ] **Step 1: 写资源隔离失败测试**

扩展测试 opener，记录每个 Redis opener 收到的 `config.RedisConfig.DB`。在 `resources_test.go` 增加测试，配置 `Realtime.RedisDB = 1`、`Token.RedisDB = 2`、`Queue.RedisDB = 3` 后断言：

```go
if got := openedDBs["redis"]; got != 0 {
	t.Fatalf("cache redis db=%d", got)
}
if got := openedDBs["realtime_redis"]; got != 1 {
	t.Fatalf("realtime redis db=%d", got)
}
if got := openedDBs["token_redis"]; got != 2 {
	t.Fatalf("token redis db=%d", got)
}
if got := openedDBs["queue_redis"]; got != 3 {
	t.Fatalf("queue redis db=%d", got)
}
```

同时断言关闭事件顺序为 `qdrant -> queue_redis -> token_redis -> realtime_redis -> redis -> database`。在 `platform/admin/build_test.go` 的 `buildTestResources()` 中增加 `RealtimeRedis`，避免测试图遗漏该必需依赖。

资源生命周期测试必须覆盖 `Realtime.Enabled=false`，因为关闭 WebSocket 不能连带关闭 AI cancel：

```go
cfg.Realtime.Enabled = false

if resources.RealtimeRedis == nil {
	t.Fatal("realtime redis must remain available for AI cancel")
}
if got := resources.Health(t.Context()).Checks["realtime_redis"].Status; got != StatusUp {
	t.Fatalf("realtime_redis status=%q", got)
}
if got := resources.Health(t.Context()).Checks["realtime"].Status; got != StatusDisabled {
	t.Fatalf("realtime status=%q", got)
}
```

API 和 Worker 两种 process 都必须覆盖这个场景。测试还要让已打开客户端的后续 Ping 返回错误，断言 `realtime_redis=down`；不得把它报告成 `disabled`。

- [ ] **Step 2: 在 Resources 中打开独立客户端**

新增 `Openers.RealtimeRedis`、`Resources.RealtimeRedis` 和 `resourceCapabilities.realtimeRedis`。`capabilitiesFor` 在确认 process 是 API 或 Worker 后，始终设置 `realtimeRedis: true`；它不得依赖 `cfg.Realtime.Enabled` 或 publisher 类型，因为 AI cancel 是 API/Worker 的核心运行能力。复制 `cfg.Redis`，将副本的 `DB` 设置为 `cfg.Realtime.RedisDB`，通过 `RealtimeRedis` opener 打开并注册名称 `realtime_redis`。打开顺序必须是 cache Redis、Realtime Redis、Token Redis、Queue Redis；Cleanup 逆序关闭，从而保证 Realtime Redis 在共享 cache Redis 之前关闭。不得因为 WebSocket 关闭或 publisher 是 local 就复用 DB 0。

在 `Health` 与 nil 资源报告中加入 `realtime_redis` 键。已打开的 API/Worker 资源必须始终通过 Redis Ping 报告 `up/down`；只有 nil `Resources` 延续现有模式报告 `disabled`。`realtime` 仍只表示 WebSocket/实时事件能力开关，不能用它代替 DB 1 的健康状态。默认 opener 复用 `defaultRedisOpener`，不新增连接实现、不做进程内内存兜底。

- [ ] **Step 3: 切换 API、Worker 和 AI cancel 发布/订阅**

按以下调用点替换参数，业务行为保持不变：

```go
newRealtimeStackWithRedis(cfg.Realtime, cfg.CORS.AllowOrigins, resources.DB, resources.RealtimeRedis, logger, recorder)
replycommand.NewRedisCancelSubscriber(resources.RealtimeRedis)
replycommand.NewRedisCancelPublisher(resources.RealtimeRedis)
```

`realtimePublisherForWorker` 的 Redis 分支也必须读取 `resources.RealtimeRedis`。Scheduler lock、system setting cache、redeem-code limiter、captcha、验证码、权限缓存和地址字典继续读取 `resources.Redis`；Queue client/inspector 继续使用 `cfg.Queue.RedisDB` 的队列配置。`platformadmin.BuildResources` 增加 `RealtimeRedis` 字段，并将两处 AI cancel publisher 改为该字段。

禁止增加“`RealtimeRedis` 为空就退回 `Redis`”的兼容兜底。资源装配缺失是启动图错误，必须由构造测试暴露；否则 AI cancel 会悄悄重新污染 DB 0。

- [ ] **Step 4: 运行运行时短测试并提交**

```powershell
go test ./internal/runtime -run 'Test(OpenResources|Realtime|Worker)' -count=1
go test ./internal/platform/admin -run 'TestBuild' -count=1
git diff --check
git add internal/runtime internal/platform/admin
git commit -m "fix(runtime): isolate realtime redis role"
```

预期：资源 DB 分配、关闭顺序、Readiness 名称和构造依赖测试通过；不得运行 `go test ./...`。

## Task 5：让 database reset 清理 DB 0/1/2/3

**Files:**

- Modify: `E:/admin/admin_back_go/scripts/database.ps1:35-55,260-275`
- Modify: `E:/admin/admin_back_go/scripts/tests/database-baseline.tests.ps1:35-55`
- Modify: `E:/admin/admin_back_go/database/README.md:15-30`
- Modify: `E:/admin/admin_back_go/README.md:435-450`

- [ ] **Step 1: 锁定 reset 范围合同**

把脚本的 Redis DB 列表测试从：

```powershell
Assert-Match $source "@\(0, 2, 3\)" 'reset must clear only the three project Redis databases'
```

改为：

```powershell
Assert-Match $source "@\(0, 1, 2, 3\)" 'reset must clear only the four project Redis databases'
```

同时保持 `Assert-NotMatch $source '(?i)FLUSHALL'`，以确保 reset 仍然逐库执行 `redis-cli -n <db> FLUSHDB`，不影响同一 Redis 实例上的其他逻辑库。

- [ ] **Step 2: 实现四库清理并同步文字**

将 `scripts/database.ps1` 的 `$script:RedisDatabases` 改为 `@(0, 1, 2, 3)`。将 `database/README.md`、根 README 及相关测试中的 `DB 0/2/3` 改为 `DB 0/1/2/3`，并明确 DB 1 是 Realtime/AI cancel。不要在本波执行 reset、删除容器、执行 `FLUSHDB` 或修改 SQL。

- [ ] **Step 3: 运行非破坏性脚本合同测试并提交**

```powershell
pwsh -NoProfile -File scripts/tests/database-baseline.tests.ps1
git diff --check
git add scripts/database.ps1 scripts/tests/database-baseline.tests.ps1 database/README.md README.md
git commit -m "fix(database): clear realtime redis database on reset"
```

预期：输出 `database baseline command contracts passed`；该测试只验证源码和错误确认分支，不会执行有效 reset。

## Task 6：Wave 01 收口与人工验收交接

**Files:**

- Read-only: `E:/admin/admin_back_go/internal/config`
- Read-only: `E:/admin/admin_back_go/internal/runtime`
- Read-only: `E:/admin/admin_back_go/internal/platform/admin`
- Read-only: `E:/admin/admin_front_ts/src/views/Main/permission/role`

- [ ] **Step 1: 运行 Wave 01 定向测试**

Backend, from `E:/admin/admin_back_go`:

```powershell
go test ./internal/config -count=1
go test ./internal/runtime -run 'Test(OpenResources|Realtime|Worker)' -count=1
go test ./internal/platform/admin -run 'TestBuild' -count=1
```

Frontend, from `E:/admin/admin_front_ts`:

```powershell
npm test -- --run tests/component/permission/RolePermissionMatrix.test.ts tests/shared/permission/role-matrix.test.ts
```

不要运行全量 Go/Vue 测试、Playwright、`npm run verify:frontend` 或长时间 smoke 脚本。

- [ ] **Step 2: 检查差异边界**

```powershell
git -C E:/admin/admin_back_go diff --check
git -C E:/admin/admin_back_go status --short
git -C E:/admin/admin_front_ts diff --check
git -C E:/admin/admin_front_ts status --short
```

确认本波后端提交只涉及配置、runtime 资源、Realtime/AI cancel 注入、reset 文档和测试；前端提交只涉及权限矩阵、翻译、样式和组件测试。若出现其他文件或既有未提交修改，停止，不要清理或覆盖。

- [ ] **Step 3: 输出人工验收清单并停止**

把以下结果写入交接记录，等待用户验收后再开始 Wave 02：

```text
1. 角色权限页：页面名 checkbox 控制访问；有按钮的页面动作列只显示真实按钮；无按钮页面可直接选择；取消页面会清除动作。
2. Redis：DB 0 保留缓存，DB 1 存在 Realtime/AI cancel，DB 2 是 Token，DB 3 是 Queue；Readiness 显示 realtime_redis。
3. 兼容性：权限接口、权限码、数据库表和响应外层未改变。
4. 测试：列出每条短测试命令和实际 PASS/FAIL，不把未运行的全量测试写成通过。
5. 下一入口：用户验收通过后，重新只读盘点并为 Wave 02 单独写系统设置 CRUD Plan。
```

Wave 01 完成后必须停下，不主动启动或重启 `admin-dev`，不跨波次删除旧架构。
