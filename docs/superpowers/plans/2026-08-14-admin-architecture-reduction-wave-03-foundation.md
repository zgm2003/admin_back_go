# Admin 架构减法 Wave 03 基础段 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立唯一的前后端分页协议、把前端请求入口迁到 `src/utils/request.ts`，并收紧 `src/enums` 的职责边界，同时只迁移已经人工验收的 `systemsetting`，不触碰 Wave 03 的其他业务模块。

**Architecture:** 后端新增 `internal/shared/pagination`，只保存 `Page` 和 `Result[T]` 两个稳定响应数据结构；`systemsetting.ListResponse` 保留业务类型名，但底层使用公共分页结果。前端由 `src/utils/request.ts` 持有唯一 request facade，`src/lib/http` 在迁移期只做同一实例的兼容导出；`src/utils/pagination.ts` 持有严格分页 schema，系统设置率先消费。`src/enums` 只保留跨模块稳定值域，通知颜色映射回到通知页面。

**Tech Stack:** Go 1.26.5、Gin、encoding/json、Vue 3.5、TypeScript 5.9、Zod 4、Vitest 4、现有 `ApiClient`/Admin contract 生成链。

---

## 0. 执行边界

本计划只处理：

1. `internal/shared/pagination` 的 `Page` 与 `Result[T]`；
2. 后端 `systemsetting` 使用公共分页；
3. 前端唯一 request facade 从 `src/lib/http/index.ts` 移到 `src/utils/request.ts`；
4. `src/utils/pagination.ts` 与前端 `systemsetting` 分页迁移；
5. 把 `NotificationTypeColorMap` 从公共枚举移回唯一使用它的通知页面；
6. 因 Go 类型包路径变化而必须同步的 Admin contract 生成物。

本计划明确不处理：

- 不迁移用户、角色、权限、邮件、短信、日志、上传、支付或 AI；
- 不删除 `src/lib`、`src/modules`、generated contract 或 runtime HTTP 模块；
- 不修改任何 API 路径、method、字段、状态码、错误协议或数据库；
- 不建立公共查询 DTO、默认分页参数、自动类型转换或空值兜底；
- 不启动、停止或重启 `admin-dev`；
- 不运行 `go test ./...`、Playwright、`npm run verify:frontend` 或发布长脚本；
- 不覆盖其他窗口或用户的未提交修改；若任一目标文件已有未知修改，停止并汇报。

当前恢复点：

```text
Backend: 1e00ac2 (Wave 02 code at 56b76c0)
Frontend: 3c27ec5
```

## 1. 文件职责

```text
E:/admin/admin_back_go/internal/shared/pagination/response.go
  唯一后端分页响应结构；没有查询参数、默认值或业务规则。

E:/admin/admin_back_go/internal/module/systemsetting/response.go
  保留 SystemSetting 业务列表项和 ListResponse 名称；不再重复 Page 字段。

E:/admin/admin_front_ts/src/utils/request.ts
  唯一 request facade 实现和 ApiClient 安装点。

E:/admin/admin_front_ts/src/lib/http/index.ts
E:/admin/admin_front_ts/src/lib/http/client.ts
  迁移期兼容导出；没有第二份状态或实现，Wave 07 引用清零后删除。

E:/admin/admin_front_ts/src/utils/pagination.ts
  唯一严格分页 schema 与 TypeScript 类型；没有业务字段或默认分页。

E:/admin/admin_front_ts/src/enums/index.ts
  只保存跨模块稳定值域；不保存颜色、标签和页面展示映射。
```

### Task 1：建立后端公共分页数据结构

**Files:**

- Create: `E:/admin/admin_back_go/internal/shared/pagination/response.go`
- Create: `E:/admin/admin_back_go/internal/shared/pagination/response_test.go`

- [ ] **Step 1: 写失败 JSON 合同测试**

创建 `response_test.go`：

```go
package pagination

import (
	"encoding/json"
	"testing"
)

func TestResultJSONKeepsEmptyListAndCompletePage(t *testing.T) {
	payload, err := json.Marshal(Result[int]{
		List: []int{},
		Page: Page{
			PageSize:    20,
			CurrentPage: 1,
			TotalPage:   0,
			Total:       0,
		},
	})
	if err != nil {
		t.Fatalf("marshal pagination result: %v", err)
	}

	const want = `{"list":[],"page":{"page_size":20,"current_page":1,"total_page":0,"total":0}}`
	if string(payload) != want {
		t.Fatalf("pagination JSON = %s, want %s", payload, want)
	}
}
```

- [ ] **Step 2: 运行测试确认类型尚不存在**

```powershell
go test ./internal/shared/pagination -count=1
```

Expected: FAIL，编译错误明确指出 `Result` 和 `Page` 未定义。

- [ ] **Step 3: 写最小公共结构**

创建 `response.go`：

```go
package pagination

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type Result[T any] struct {
	List []T  `json:"list"`
	Page Page `json:"page"`
}
```

不要增加 `omitempty`、构造器、页码默认值、总页数算法或查询 DTO。空列表是否为 `[]` 由业务 Service 创建非 nil slice 保证，公共结构不猜业务状态。

- [ ] **Step 4: 复跑并提交**

```powershell
go test ./internal/shared/pagination -count=1
git diff --check
git add internal/shared/pagination/response.go internal/shared/pagination/response_test.go
git commit -m "feat(shared): add pagination response contract"
```

Expected: 测试 PASS；提交只包含新公共包。

### Task 2：只迁移后端 systemsetting 使用公共分页

**Files:**

- Modify: `E:/admin/admin_back_go/internal/module/systemsetting/response.go:3-23`
- Modify: `E:/admin/admin_back_go/internal/module/systemsetting/service.go:3-59`
- Modify: `E:/admin/admin_back_go/internal/module/systemsetting/service_test.go:3-115`
- Modify: `E:/admin/admin_back_go/internal/module/systemsetting/handler_test.go:3-140`
- Modify: `E:/admin/admin_back_go/internal/server/router_test.go:975-1000`

- [ ] **Step 1: 增加失败的数据结构测试**

在 `service_test.go` imports 增加：

```go
"reflect"

"admin_back_go/internal/shared/pagination"
```

在 `TestListTrimsKeyAndReturnsLabels` 前增加：

```go
func TestListResponseUsesSharedPaginationPage(t *testing.T) {
	field, ok := reflect.TypeOf(ListResponse{}).FieldByName("Page")
	if !ok {
		t.Fatal("ListResponse.Page is missing")
	}
	if field.Type != reflect.TypeOf(pagination.Page{}) {
		t.Fatalf("ListResponse.Page type = %v, want pagination.Page", field.Type)
	}
}
```

- [ ] **Step 2: 运行测试确认本地 Page 重复仍存在**

```powershell
go test ./internal/module/systemsetting -run 'TestListResponseUsesSharedPaginationPage' -count=1
```

Expected: FAIL，实际类型为 `systemsetting.Page`。

- [ ] **Step 3: 删除本地 Page 并保留业务响应名**

把 `response.go` imports 改为：

```go
import (
	"admin_back_go/internal/shared/dict"
	"admin_back_go/internal/shared/pagination"
)
```

删除本地 `type Page struct` 和原 `type ListResponse struct`，替换为：

```go
type ListResponse pagination.Result[ListItem]
```

保留 `PageInitResponse`、`PageInitDict`、`ListItem`、`CreateResponse` 和 `EmptyResponse` 原样。这里使用定义类型而不是类型别名：业务合同继续叫 `systemsetting.ListResponse`，但分页字段只有 `pagination.Page` 一个事实来源。

- [ ] **Step 4: 让 Service 构造公共 Page**

在 `service.go` imports 增加：

```go
"admin_back_go/internal/shared/pagination"
```

把列表返回改为：

```go
return &ListResponse{
	List: list,
	Page: pagination.Page{
		PageSize:    request.PageSize,
		CurrentPage: request.CurrentPage,
		TotalPage:   totalPage(total, request.PageSize),
		Total:       total,
	},
}, nil
```

保留 `totalPage` 在 `systemsetting`。分页结构可以公用，分页请求校验和计算规则仍属于业务模块。

- [ ] **Step 5: 同步两个测试 fake 的字段类型**

在 `handler_test.go` imports 增加：

```go
"admin_back_go/internal/shared/pagination"
```

把 fake Service 返回和 JSON 解码结构中的 Page 改为：

```go
return &ListResponse{
	List: []ListItem{{ID: 1, SettingKey: "user.default_avatar"}},
	Page: pagination.Page{
		CurrentPage: request.CurrentPage,
		PageSize:    request.PageSize,
		Total:       1,
		TotalPage:   1,
	},
}, nil
```

```go
Data struct {
	List []ListItem      `json:"list"`
	Page pagination.Page `json:"page"`
} `json:"data"`
```

在 `internal/server/router_test.go` imports 增加同一个 `pagination` 包，并把唯一的系统设置 fake 改为：

```go
return &systemsetting.ListResponse{
	List: []systemsetting.ListItem{{ID: 1, SettingKey: "user.default_avatar"}},
	Page: pagination.Page{
		CurrentPage: request.CurrentPage,
		PageSize:    request.PageSize,
		Total:       1,
		TotalPage:   1,
	},
}, nil
```

不要修改其他模块自己的 Page；它们只能在各自 Wave 中迁移。

- [ ] **Step 6: 运行定向回归并提交**

```powershell
go test ./internal/shared/pagination ./internal/module/systemsetting -count=1
go test ./internal/server -run 'TestRouterInstallsSystemSettingRESTRoutes' -count=1
git diff --check
git add internal/module/systemsetting/response.go internal/module/systemsetting/service.go internal/module/systemsetting/service_test.go internal/module/systemsetting/handler_test.go internal/server/router_test.go
git commit -m "refactor(systemsetting): use shared pagination"
```

Expected: 三个包的定向测试 PASS；API JSON 字段仍为 `list/page/page_size/current_page/total_page/total`。

### Task 3：把唯一前端 request 实现移到 utils

**Files:**

- Create: `E:/admin/admin_front_ts/src/utils/request.ts`（由现有实现原样迁入）
- Create: `E:/admin/admin_front_ts/tests/unit/http/request-entry.test.ts`
- Modify: `E:/admin/admin_front_ts/src/lib/http/index.ts`
- Modify: `E:/admin/admin_front_ts/src/lib/http/client.ts`
- Modify: `E:/admin/admin_front_ts/src/main.ts:24`
- Modify: `E:/admin/admin_front_ts/tests/helpers/api-client.ts:1-2`
- Modify: `E:/admin/admin_front_ts/tests/unit/http/architecture.test.ts:28-34`

- [ ] **Step 1: 写失败的单实例入口测试**

创建 `request-entry.test.ts`：

```ts
import { describe, expect, it } from 'vitest'
import legacyRequest, {
  executeAdminOperation as executeLegacyOperation,
  installApiClient as installLegacyApiClient,
} from '@/lib/http'
import request, {
  executeAdminOperation,
  installApiClient,
} from '@/utils/request'

describe('request entry', () => {
  it('keeps one request instance during the lib-to-utils migration', () => {
    expect(legacyRequest).toBe(request)
    expect(executeLegacyOperation).toBe(executeAdminOperation)
    expect(installLegacyApiClient).toBe(installApiClient)
  })
})
```

- [ ] **Step 2: 运行测试确认新入口尚不存在**

```powershell
npm test -- tests/unit/http/request-entry.test.ts
```

Expected: FAIL，模块解析明确指出 `@/utils/request` 不存在。

- [ ] **Step 3: 移动真实实现，不复制状态**

把当前实现移动到 `src/utils/request.ts`，目标文件完整内容固定为：

```ts
import { createApiError } from '@/modules/http/error'
import type { ApiClient, ExecuteOptions } from '@/modules/http/client'
import type { z } from 'zod'
import {
  defineOperation,
  type HttpMethod,
  type Operation,
  type QueryValue,
} from '@/modules/http/operations'

export interface RequestConfig<T = unknown, D = unknown> {
  readonly params?: object
  readonly data?: D
  readonly signal?: AbortSignal
  readonly idempotencyKey?: string
  readonly responseSchema?: z.ZodType<T>
}

export interface RequestClient {
  get<T = unknown>(url: string, config?: RequestConfig<T>): Promise<T>
  post<T = unknown, D = unknown>(url: string, data?: D, config?: RequestConfig<T, D>): Promise<T>
  put<T = unknown, D = unknown>(url: string, data?: D, config?: RequestConfig<T, D>): Promise<T>
  patch<T = unknown, D = unknown>(url: string, data?: D, config?: RequestConfig<T, D>): Promise<T>
  delete<T = unknown, D = unknown>(url: string, config?: RequestConfig<T, D>): Promise<T>
}

const publicAdminPaths = new Set([
  '/api/admin/v1/auth/captcha',
  '/api/admin/v1/auth/forgot-password',
  '/api/admin/v1/auth/login',
  '/api/admin/v1/auth/login-config',
  '/api/admin/v1/auth/refresh',
  '/api/admin/v1/auth/send-code',
])

let installedClient: ApiClient | null = null

export function installApiClient(client: ApiClient): () => void {
  installedClient = client
  return () => {
    if (installedClient === client) installedClient = null
  }
}

function requireClient(): ApiClient {
  if (!installedClient) {
    throw createApiError({
      kind: 'internal',
      code: 'http.client_not_installed',
      retryable: false,
      messageKey: 'http.clientNotInstalled',
    })
  }
  return installedClient
}

export function executeAdminOperation<TInput, TOutput>(
  operation: Operation<TInput, TOutput>,
  input: TInput,
  options: ExecuteOptions = {},
): Promise<TOutput> {
  return requireClient().execute(operation, input, options)
}

function queryFrom(params: object | undefined): Readonly<Record<string, QueryValue>> | undefined {
  if (!params) return undefined
  const query: Record<string, QueryValue> = {}
  for (const [name, value] of Object.entries(params)) {
    const validArray = Array.isArray(value)
      && value.every((item) => typeof item === 'string' || typeof item === 'number' || typeof item === 'boolean')
    if (
      value === undefined
      || value === null
      || typeof value === 'string'
      || typeof value === 'number'
      || typeof value === 'boolean'
      || validArray
    ) {
      query[name] = value as QueryValue
      continue
    }
    throw createApiError({
      kind: 'contract',
      code: 'http.query_value_invalid',
      retryable: false,
      messageKey: 'http.queryValueInvalid',
      messageData: { name },
    })
  }
  return query
}

function execute<T, D>(
  method: HttpMethod,
  url: string,
  body: D | undefined,
  config: RequestConfig<T, D> | undefined,
): Promise<T> {
  const idempotencyKey = config?.idempotencyKey?.trim()
  const operation = defineOperation<D | undefined, T>({
    id: `compat.${method.toLowerCase()}`,
    method,
    path: url,
    auth: publicAdminPaths.has(url) ? 'public' : 'required',
    timeout: typeof FormData !== 'undefined' && body instanceof FormData ? 'upload' : 'interactive',
    replay: method === 'GET' ? 'safe' : idempotencyKey ? 'idempotency-key' : 'never',
    responseSchema: config?.responseSchema,
    telemetryName: `compat.${method.toLowerCase()}`,
    encode: () => ({
      query: queryFrom(config?.params),
      body,
    }),
  })
  return requireClient().execute(operation, body, {
    signal: config?.signal,
    idempotencyKey,
  })
}

export const request: RequestClient = {
  get<T = unknown>(url: string, config?: RequestConfig<T>): Promise<T> {
    return execute<T, unknown>('GET', url, undefined, config)
  },
  post<T = unknown, D = unknown>(url: string, data?: D, config?: RequestConfig<T, D>): Promise<T> {
    return execute<T, D>('POST', url, data, config)
  },
  put<T = unknown, D = unknown>(url: string, data?: D, config?: RequestConfig<T, D>): Promise<T> {
    return execute<T, D>('PUT', url, data, config)
  },
  patch<T = unknown, D = unknown>(url: string, data?: D, config?: RequestConfig<T, D>): Promise<T> {
    return execute<T, D>('PATCH', url, data, config)
  },
  delete<T = unknown, D = unknown>(url: string, config?: RequestConfig<T, D>): Promise<T> {
    return execute<T, D>('DELETE', url, config?.data, config)
  },
}

export default request
```

不要新建第二个 `installedClient`，不要改变全局错误通知、鉴权、幂等或 schema 解析。

把 `src/lib/http/index.ts` 完整替换为临时兼容导出：

```ts
export {
  default,
  executeAdminOperation,
  installApiClient,
  request,
  type RequestClient,
  type RequestConfig,
} from '@/utils/request'
```

把 `src/lib/http/client.ts` 完整替换为直接指向同一实现的兼容导出：

```ts
export {
  installApiClient,
  request,
  type RequestClient,
  type RequestConfig,
} from '@/utils/request'
```

这两个文件只服务尚未迁移的调用者，删除期限固定为 Wave 07；禁止在这里重新增加实现。

- [ ] **Step 4: 让运行时装配和测试装配使用新入口**

在 `src/main.ts` 只改 import：

```ts
import { installApiClient } from './utils/request'
```

在 `tests/helpers/api-client.ts` 只改 import：

```ts
import { installApiClient } from '@/utils/request'
```

所有仍从 `@/lib/http` 导入的旧业务 API 保持不动；它们通过兼容导出使用同一个 `installedClient`。

- [ ] **Step 5: 删除保护旧实现位置的源码字符串测试**

从 `tests/unit/http/architecture.test.ts` 删除整个测试：

```ts
it('exports only the typed compatibility request facade, never a raw service', () => {
  const source = readFileSync(resolve('src/lib/http/index.ts'), 'utf8')

  expect(source).toContain('installApiClient')
  expect(source).toContain('requireClient().execute')
  expect(source).not.toMatch(/export\s+\{[^}]*\bservice\b/)
})
```

新 `request-entry.test.ts` 验证真实模块身份，避免继续用固定源码字符串保护已经退役的位置。

- [ ] **Step 6: 运行定向测试并提交**

```powershell
npm test -- tests/unit/http/request-entry.test.ts tests/unit/http/architecture.test.ts tests/shared/system/system-setting-api.test.ts
npx eslint src/utils/request.ts src/lib/http/index.ts src/lib/http/client.ts src/main.ts tests/helpers/api-client.ts tests/unit/http/request-entry.test.ts tests/unit/http/architecture.test.ts
git diff --check
git add src/utils/request.ts src/lib/http/index.ts src/lib/http/client.ts src/main.ts tests/helpers/api-client.ts tests/unit/http/request-entry.test.ts tests/unit/http/architecture.test.ts
git commit -m "refactor(http): move request entry to utils"
```

Expected: 三个 Vitest 文件和定向 ESLint PASS；旧 API 与新入口共享同一 client 实例。

### Task 4：建立前端公共分页并迁移 systemsetting

**Files:**

- Create: `E:/admin/admin_front_ts/src/utils/pagination.ts`
- Create: `E:/admin/admin_front_ts/tests/unit/utils/pagination.test.ts`
- Modify: `E:/admin/admin_front_ts/src/api/system/setting.ts:1-89`

- [ ] **Step 1: 写失败的严格分页 schema 测试**

创建 `pagination.test.ts`：

```ts
import { describe, expect, it } from 'vitest'
import { z } from 'zod'
import { pageSchema, paginatedSchema } from '@/utils/pagination'

describe('pagination contract', () => {
  const itemSchema = z.object({ id: z.number().int().positive() }).strict()
  const schema = paginatedSchema(itemSchema)

  it('accepts an empty list with every required page field', () => {
    const payload = {
      list: [],
      page: { page_size: 20, current_page: 1, total_page: 0, total: 0 },
    }
    expect(schema.parse(payload)).toEqual(payload)
  })

  it('rejects missing and unknown page fields instead of filling defaults', () => {
    expect(pageSchema.safeParse({ page_size: 20, current_page: 1, total: 0 }).success).toBe(false)
    expect(pageSchema.safeParse({
      page_size: 20,
      current_page: 1,
      total_page: 0,
      total: 0,
      next_page: 2,
    }).success).toBe(false)
  })
})
```

- [ ] **Step 2: 运行测试确认公共工具尚不存在**

```powershell
npm test -- tests/unit/utils/pagination.test.ts
```

Expected: FAIL，模块解析明确指出 `@/utils/pagination` 不存在。

- [ ] **Step 3: 创建最小分页工具**

创建 `src/utils/pagination.ts`：

```ts
import { z } from 'zod'

export const pageSchema = z.object({
  page_size: z.number().int().positive(),
  current_page: z.number().int().positive(),
  total_page: z.number().int().nonnegative(),
  total: z.number().int().nonnegative(),
}).strict()

export type PageInfo = z.infer<typeof pageSchema>

export interface PaginatedResponse<T> {
  list: T[]
  page: PageInfo
}

export function paginatedSchema<T extends z.ZodType>(itemSchema: T) {
  return z.object({
    list: z.array(itemSchema),
    page: pageSchema,
  }).strict()
}
```

不要在这里加入 `current_page=1`、`page_size=20`、最大页大小、query schema、`next_id`、远程下拉结构或 `catch` 后默认空列表。

- [ ] **Step 4: 让 systemsetting 消费公共分页和新 request 入口**

把 `src/api/system/setting.ts` imports 改为：

```ts
import { z } from 'zod'
import request from '@/utils/request'
import { paginatedSchema, type PaginatedResponse } from '@/utils/pagination'
import type { ExecuteOptions } from '@/modules/http/client'
import type { DictOption, Id } from '@/types/common'
```

把本地列表响应接口替换为：

```ts
export type SystemSettingListResponse = PaginatedResponse<SystemSettingItem>
```

完整删除本地 `pageSchema`：

```ts
const pageSchema = z.object({
  page_size: z.number().int().positive(),
  current_page: z.number().int().positive(),
  total_page: z.number().int().nonnegative(),
  total: z.number().int().nonnegative(),
}).strict()
```

把 `listSchema` 替换为：

```ts
const listSchema: z.ZodType<SystemSettingListResponse> = paginatedSchema(itemSchema)
```

其余业务 schema、请求参数、ID 校验和七个 API method 保持不变。

- [ ] **Step 5: 运行分页和系统设置定向测试并提交**

```powershell
npm test -- tests/unit/utils/pagination.test.ts tests/shared/system/system-setting-api.test.ts tests/unit/http/request-entry.test.ts
npx eslint src/utils/pagination.ts src/api/system/setting.ts tests/unit/utils/pagination.test.ts
git diff --check
git add src/utils/pagination.ts src/api/system/setting.ts tests/unit/utils/pagination.test.ts
git commit -m "refactor(systemsetting): use shared frontend pagination"
```

Expected: 三个 Vitest 文件与定向 ESLint PASS；缺 `total_page` 仍产生合同错误，不会被补默认值。

### Task 5：收紧 enums 边界而不改变通知 UI

**Files:**

- Modify: `E:/admin/admin_front_ts/src/enums/index.ts:33-50`
- Modify: `E:/admin/admin_front_ts/src/views/Main/notification/index.vue:6-12,76-96`
- Test: `E:/admin/admin_front_ts/tests/component/accessibility/notification.test.ts`

- [ ] **Step 1: 运行现有通知页面测试锁住当前行为**

```powershell
npm test -- tests/component/accessibility/notification.test.ts
```

Expected: PASS。该任务是纯职责搬移，不先制造用户可见失败。

- [ ] **Step 2: 确认公共枚举中存在 UI 映射**

```powershell
rg -n 'ColorMap|Color|LabelMap|TextMap' src/enums
```

Expected: 只命中 `NotificationTypeColorMap`；若还命中其他内容，停止并汇报，不在本任务扩大清理范围。

- [ ] **Step 3: 把颜色映射移回唯一业务页面**

从 `src/enums/index.ts` 删除：

```ts
export const NotificationTypeColorMap: Record<number, 'info' | 'success' | 'warning' | 'danger'> = {
  [NotificationTypeEnum.INFO]: 'info',
  [NotificationTypeEnum.SUCCESS]: 'success',
  [NotificationTypeEnum.WARNING]: 'warning',
  [NotificationTypeEnum.ERROR]: 'danger',
}
```

通知稳定值域 `NotificationTypeEnum` 和 `NotificationLevelEnum` 保留原值。

把通知页 enum import 改为：

```ts
import {
  CommonEnum,
  NotificationLevelEnum,
  NotificationTypeEnum,
} from '@/enums'
```

在 `isUnread` 前增加页面展示映射：

```ts
type NotificationTagType = 'info' | 'success' | 'warning' | 'danger'

const notificationTypeColorMap: Record<NotificationItem['type'], NotificationTagType> = {
  [NotificationTypeEnum.INFO]: 'info',
  [NotificationTypeEnum.SUCCESS]: 'success',
  [NotificationTypeEnum.WARNING]: 'warning',
  [NotificationTypeEnum.ERROR]: 'danger',
}

const getTypeColor = (type: NotificationItem['type']): NotificationTagType =>
  notificationTypeColorMap[type]
```

删除原来的：

```ts
const getTypeColor = (type: number) => NotificationTypeColorMap[type] || 'info'
```

`NotificationItem.type` 的生成合同已经是 `1 | 2 | 3 | 4`，因此四个分支完整，不需要用默认颜色掩盖非法后端值。

- [ ] **Step 4: 复跑边界、页面测试并提交**

```powershell
$hits = rg -n 'ColorMap|Color|LabelMap|TextMap' src/enums
if ($LASTEXITCODE -eq 0) { throw "UI display mapping remains in src/enums: $hits" }
if ($LASTEXITCODE -ne 1) { throw "rg failed with exit code $LASTEXITCODE" }
npm test -- tests/component/accessibility/notification.test.ts
npx eslint src/enums/index.ts src/views/Main/notification/index.vue
git diff --check
git add src/enums/index.ts src/views/Main/notification/index.vue
git commit -m "refactor(enums): keep only stable protocol values"
```

Expected: `src/enums` 无 UI 映射，通知组件测试和 ESLint PASS，四种通知标签颜色保持不变。

### Task 6：同步 Admin contract 兼容产物

后端分页字段的 Go 包路径从 `systemsetting.Page` 变为 `pagination.Page`。JSON 语义没变，但 Wave 07 前的 OpenAPI/前端 generated 质量门仍要求包路径 schema 和 manifest 自洽，因此必须机械同步。

**Files:**

- Modify generated backend bundle: `E:/admin/admin_back_go/contracts/admin/v1/**`
- Modify synchronized frontend bundle: `E:/admin/admin_front_ts/contracts/backend/admin/**`
- Modify generated frontend outputs: `E:/admin/admin_front_ts/src/modules/http/generated/**`
- Modify generated routing outputs only if generator changes them: `E:/admin/admin_front_ts/src/modules/routing/generated/**`

- [ ] **Step 1: 用后端业务提交 SHA 生成 bundle**

从 `E:/admin/admin_back_go` 执行：

```powershell
if (git status --short) { throw 'backend worktree must be clean before contract generation' }
$backendCommit = git rev-parse HEAD
if ($backendCommit -notmatch '^[0-9a-f]{40}$') { throw 'backend HEAD is not a full SHA' }
pwsh -NoProfile -File scripts/generate-admin-contract.ps1 -BackendCommit $backendCommit
go test ./internal/admincontract -run 'TestSystemAndCommunicationsRoutesPublishRuntimeModelContracts|TestOpenAPIContainsEveryRuntimeAdminOperation' -count=1
pwsh -NoProfile -File scripts/check-admin-contract.ps1 -BackendCommit $backendCommit
git diff --check
git add contracts/admin/v1
git commit -m "chore(contract): publish shared pagination schema"
```

Expected: contract 定向测试和 check PASS；`systemsetting.ListResponse` 的 JSON required 字段不变，Page schema 指向 `internal/shared/pagination`。

- [ ] **Step 2: 同步前端 bundle 和生成文件**

从 `E:/admin/admin_front_ts` 执行：

```powershell
if (git status --short) { throw 'frontend worktree must be clean before contract sync' }
$backendCommit = [string](Get-Content -Raw E:/admin/admin_back_go/contracts/admin/v1/manifest.json | ConvertFrom-Json).backend_commit
if ($backendCommit -notmatch '^[0-9a-f]{40}$') { throw 'contract manifest has no full backend SHA' }
node scripts/sync-admin-contract.mjs --backend E:/admin/admin_back_go --commit $backendCommit
npm run contract:generate
npm run contract:check
npm test -- tests/unit/http/generated-operations.test.ts tests/shared/system/system-setting-api.test.ts
git diff --check
git add contracts/backend/admin src/modules/http/generated src/modules/routing/generated
git commit -m "chore(contract): sync shared pagination schema"
```

Expected: sync、generate、check 和两个 Vitest 文件 PASS；`src/api/system/setting.ts` 不重新依赖 generated Page。

### Task 7：最终短验证与人工验收交接

**Files:**

- Read-only: `E:/admin/admin_back_go/internal/shared/pagination`
- Read-only: `E:/admin/admin_back_go/internal/module/systemsetting`
- Read-only: `E:/admin/admin_front_ts/src/utils/request.ts`
- Read-only: `E:/admin/admin_front_ts/src/utils/pagination.ts`
- Read-only: `E:/admin/admin_front_ts/src/enums/index.ts`
- Read-only: `E:/admin/admin_front_ts/src/api/system/setting.ts`

- [ ] **Step 1: 运行最终后端短测试**

```powershell
go test ./internal/shared/pagination ./internal/module/systemsetting -count=1
go test ./internal/server -run 'TestRouterInstallsSystemSettingRESTRoutes' -count=1
go test ./internal/admincontract -run 'TestSystemAndCommunicationsRoutesPublishRuntimeModelContracts|TestOpenAPIContainsEveryRuntimeAdminOperation' -count=1
```

Expected: 全部 PASS。不要扩展为 `go test ./...`。

- [ ] **Step 2: 运行最终前端短测试**

```powershell
npm test -- tests/unit/http/request-entry.test.ts tests/unit/utils/pagination.test.ts tests/shared/system/system-setting-api.test.ts tests/unit/http/architecture.test.ts tests/unit/http/generated-operations.test.ts tests/component/accessibility/notification.test.ts
npx eslint src/utils/request.ts src/utils/pagination.ts src/lib/http/index.ts src/lib/http/client.ts src/main.ts src/api/system/setting.ts src/enums/index.ts src/views/Main/notification/index.vue tests/helpers/api-client.ts tests/unit/http/request-entry.test.ts tests/unit/utils/pagination.test.ts tests/unit/http/architecture.test.ts
```

Expected: 六个 Vitest 文件与定向 ESLint PASS。不要运行全量 typecheck、`verify:frontend` 或 Playwright。

- [ ] **Step 3: 检查没有扩大迁移面**

```powershell
rg -n 'type Page struct' E:/admin/admin_back_go/internal/module/systemsetting
rg -n 'const pageSchema' E:/admin/admin_front_ts/src/api/system/setting.ts
rg -n "@/lib/http" E:/admin/admin_front_ts/src/api/system/setting.ts E:/admin/admin_front_ts/src/main.ts E:/admin/admin_front_ts/tests/helpers/api-client.ts
rg -n 'ColorMap|Color|LabelMap|TextMap' E:/admin/admin_front_ts/src/enums
git -C E:/admin/admin_back_go diff --check
git -C E:/admin/admin_back_go status --short
git -C E:/admin/admin_front_ts diff --check
git -C E:/admin/admin_front_ts status --short
```

Expected: 四个 `rg` 均无输出；两个仓库 `diff --check` 与 `status --short` 均干净。不要扫描或修改其他模块的重复 Page，它们留给各自 Wave。

- [ ] **Step 4: 输出人工验收清单并停止**

交接必须报告：

```text
- 后端与前端每个提交 SHA/标题；
- 实际运行的每条短测试及 PASS/FAIL；
- 明确写出未运行：admin-dev、全量 Go/Vue、typecheck、Playwright、verify:frontend；
- 人工验收：系统设置列表/翻页/新增/编辑/启停/删除/错误通知；
- 人工验收：通知页面四种类型标签颜色；
- 结构证据：systemsetting 已用公共分页，新 API 使用 utils/request，旧 lib/http 只是兼容桥；
- 下一入口：用户验收通过后，再单独盘点 Wave 03 的第一个业务模块，不自动继续。
```

## 2. 完成后的调用链

后端：

```text
systemsetting.Service
-> systemsetting.ListResponse
-> pagination.Result[systemsetting.ListItem]
-> pagination.Page
-> 原 JSON {list, page}
```

前端：

```text
views/Main/system/setting/index.vue
-> api/system/setting.ts
-> utils/request.ts
-> modules/http/ApiClient（迁移期底层）
-> backend

api/system/setting.ts
-> utils/pagination.ts
-> 严格校验 {list, page}
```

旧 `src/lib/http` 和 `src/modules/http` 仍是明确的迁移期边界，不是假装已经删除。后续每个业务模块只迁移自己的 API；所有消费者清零并人工验收后，Wave 07 才物理删除旧目录。
