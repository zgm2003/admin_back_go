# RESTful User Session Adapter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `GET /api/admin/v1/users/me`, switch the frontend bootstrap call to it, and move token refresh to `POST /api/admin/v1/auth/refresh` without changing MySQL schema.

**Architecture:** Keep the existing Go shape: `route -> handler -> service -> repository -> model`. Reuse the existing user init service for both the legacy adapter and the new REST endpoint, with one private handler helper so the old and new paths cannot drift. On the frontend, change only the API boundary and session refresh URL; keep Pinia state assignment and router bootstrap behavior stable.

**Tech Stack:** Go 1.x, Gin, Go test, Vue 3, TypeScript, Pinia, Axios, Vitest, Vite 8.

---

## File structure

- Modify `internal/module/user/handler_test.go`: add a handler-level test for `GET /api/admin/v1/users/me` and prove it uses `AuthIdentity` exactly like legacy init.
- Modify `internal/module/user/handler.go`: add `Me(c *gin.Context)` and move shared current-user response logic into `respondWithCurrentUser(c *gin.Context)`.
- Modify `internal/module/user/route.go`: register `GET /api/admin/v1/users/me` while keeping `POST /api/Users/init`.
- Modify `internal/server/router_test.go`: add an integration test proving `/api/admin/v1/users/me` is protected by `AuthToken`, forwards headers into the authenticator, and returns user init payload through the router.
- Modify `src/api/user/users.ts`: define one `fetchCurrentUser()` function and bind both `UsersApi.me` and `UsersApi.init` to it.
- Modify `src/lib/http/auth-session.ts`: change refresh URL and 401 self-check from legacy refresh to `/api/admin/v1/auth/refresh`.
- Modify `tests/shared/user/users-api.test.ts`: fix the stale absolute `e:/admin` fixture path to `process.cwd()` and add static contract tests for the RESTful user/session URLs.

No table files or migration files are touched.

---

### Task 1: Backend user handler REST endpoint

**Files:**
- Modify: `internal/module/user/handler_test.go`
- Modify: `internal/module/user/handler.go`
- Modify: `internal/module/user/route.go`

- [ ] **Step 1: Write the failing handler test**

Append this test to `internal/module/user/handler_test.go` after `TestHandlerInitUsesAuthIdentityAndReturnsData`:

```go
func TestHandlerMeUsesAuthIdentityAndReturnsData(t *testing.T) {
	service := &fakeInitService{result: &InitResponse{
		UserID:      1,
		Username:    "admin",
		Avatar:      "avatar.png",
		RoleName:    "管理员",
		Permissions: []permission.MenuItem{{Index: "1", Label: "系统", Children: []permission.MenuItem{}}},
		Router:      []permission.RouteItem{{Name: "menu_2", Path: "/system/user", ViewKey: "system/user/index", Meta: map[string]string{"menuId": "2"}}},
		ButtonCodes: []string{"user_add"},
		QuickEntry:  []QuickEntry{{ID: 3, PermissionID: 2, Sort: 1}},
	}}
	router := newUserTestRouter(service, &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/users/me", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.input.UserID != 1 || service.input.Platform != "admin" {
		t.Fatalf("service input mismatch: %#v", service.input)
	}
	body := decodeUserBody(t, recorder)
	data := body["data"].(map[string]any)
	if data["username"] != "admin" || data["role_name"] != "管理员" {
		t.Fatalf("unexpected data: %#v", data)
	}
	if _, ok := data["quick_entry"]; !ok {
		t.Fatalf("missing quick_entry in response: %#v", data)
	}
}
```

- [ ] **Step 2: Run the focused test and verify it fails**

Run from `E:\admin_go\admin_back_go`:

```powershell
go test ./internal/module/user -run TestHandlerMeUsesAuthIdentityAndReturnsData -count=1
```

Expected failure:

```text
expected status 200, got 404
```

- [ ] **Step 3: Add the minimal handler implementation**

Replace `internal/module/user/handler.go` with this content:

```go
package user

import (
	"context"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type InitService interface {
	Init(ctx context.Context, input InitInput) (*InitResponse, *apperror.Error)
}

type Handler struct {
	service InitService
}

func NewHandler(service InitService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Init(c *gin.Context) {
	h.respondWithCurrentUser(c)
}

func (h *Handler) Me(c *gin.Context) {
	h.respondWithCurrentUser(c)
}

func (h *Handler) respondWithCurrentUser(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return
	}
	if h.service == nil {
		response.Error(c, apperror.Internal("用户初始化服务未配置"))
		return
	}

	result, appErr := h.service.Init(c.Request.Context(), InitInput{
		UserID:   identity.UserID,
		Platform: identity.Platform,
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}
```

- [ ] **Step 4: Register the REST route**

Replace `internal/module/user/route.go` with this content:

```go
package user

import "github.com/gin-gonic/gin"

func RegisterRoutes(router *gin.Engine, service InitService) {
	handler := NewHandler(service)

	v1 := router.Group("/api/admin/v1/users")
	v1.GET("/me", handler.Me)

	legacy := router.Group("/api/Users")
	legacy.POST("/init", handler.Init)
}
```

- [ ] **Step 5: Run handler tests and verify they pass**

Run from `E:\admin_go\admin_back_go`:

```powershell
go test ./internal/module/user -run "TestHandler(Init|Me)" -count=1
```

Expected output:

```text
ok  	admin_back_go/internal/module/user
```

- [ ] **Step 6: Commit backend handler route change**

Run from `E:\admin_go\admin_back_go`:

```powershell
git add internal/module/user/handler.go internal/module/user/handler_test.go internal/module/user/route.go
git commit -m "feat: add current user REST endpoint"
```

Expected output includes:

```text
feat: add current user REST endpoint
```

---

### Task 2: Backend router integration guard

**Files:**
- Modify: `internal/server/router_test.go`

- [ ] **Step 1: Add user and permission imports**

In `internal/server/router_test.go`, extend the import block by adding these imports:

```go
	"admin_back_go/internal/module/permission"
	"admin_back_go/internal/module/user"
```

The local import section should include:

```go
	"admin_back_go/internal/apperror"
	"admin_back_go/internal/config"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/permission"
	"admin_back_go/internal/module/session"
	"admin_back_go/internal/module/user"
	"admin_back_go/internal/readiness"
```

- [ ] **Step 2: Add fake user service**

Add this fake below `fakeAuthService` in `internal/server/router_test.go`:

```go
type fakeRouterUserService struct {
	input  user.InitInput
	result *user.InitResponse
	err    *apperror.Error
}

func (f *fakeRouterUserService) Init(ctx context.Context, input user.InitInput) (*user.InitResponse, *apperror.Error) {
	f.input = input
	return f.result, f.err
}
```

- [ ] **Step 3: Write the router integration test**

Add this test after `TestRouterInstallsRefreshEndpointAsPublicPath`:

```go
func TestRouterInstallsUsersMeAsProtectedPath(t *testing.T) {
	var authInput middleware.TokenInput
	userService := &fakeRouterUserService{result: &user.InitResponse{
		UserID:      1,
		Username:    "admin",
		Avatar:      "avatar.png",
		RoleName:    "管理员",
		Permissions: []permission.MenuItem{{Index: "1", Label: "系统", Children: []permission.MenuItem{}}},
		Router:      []permission.RouteItem{{Name: "menu_2", Path: "/system/user", ViewKey: "system/user/index"}},
		ButtonCodes: []string{"user_add"},
		QuickEntry:  []user.QuickEntry{{ID: 3, PermissionID: 2, Sort: 1}},
	}}
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			authInput = input
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: input.Platform}, nil
		},
		UserService: userService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/users/me", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("platform", "admin")
	request.Header.Set("device-id", "desktop-1")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if authInput.AccessToken != "access-token" || authInput.Platform != "admin" || authInput.DeviceID != "desktop-1" {
		t.Fatalf("unexpected auth input: %#v", authInput)
	}
	if userService.input.UserID != 1 || userService.input.Platform != "admin" {
		t.Fatalf("unexpected user service input: %#v", userService.input)
	}
	body := decodeRouterBody(t, recorder)
	data := mustRouterData(t, body)
	if data["username"] != "admin" || data["role_name"] != "管理员" {
		t.Fatalf("unexpected users/me payload: %#v", data)
	}
	if _, ok := data["buttonCodes"]; !ok {
		t.Fatalf("missing buttonCodes in users/me payload: %#v", data)
	}
}
```

- [ ] **Step 4: Run the router test and verify it passes after Task 1**

Run from `E:\admin_go\admin_back_go`:

```powershell
go test ./internal/server -run TestRouterInstallsUsersMeAsProtectedPath -count=1
```

Expected output:

```text
ok  	admin_back_go/internal/server
```

- [ ] **Step 5: Commit router integration guard**

Run from `E:\admin_go\admin_back_go`:

```powershell
git add internal/server/router_test.go
git commit -m "test: cover current user REST route"
```

Expected output includes:

```text
test: cover current user REST route
```

---

### Task 3: Frontend users API contract switch

**Files:**
- Modify: `tests/shared/user/users-api.test.ts`
- Modify: `src/api/user/users.ts`

- [ ] **Step 1: Fix frontend users API test fixture paths**

Replace `tests/shared/user/users-api.test.ts` with this content:

```ts
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

function readFrontendSource(relativePath: string) {
  return readFileSync(resolve(process.cwd(), relativePath), 'utf8')
}

function readUsersApiSource() {
  return readFrontendSource('src/api/user/users.ts')
}

function readUserTypeSource() {
  return readFrontendSource('src/types/user.ts')
}

function readAuthSessionSource() {
  return readFrontendSource('src/lib/http/auth-session.ts')
}

describe('users api auth contract', () => {
  it('does not expose a standalone register api anymore', () => {
    const source = readUsersApiSource()
    const typeSource = readUserTypeSource()

    expect(source).not.toContain('export interface UserRegisterParams')
    expect(source).not.toContain('register: (params:')
    expect(source).not.toContain('/api/Users/register')
    expect(source).toContain('/api/Users/login')
    expect(typeSource).not.toContain("| 'register'")
  })

  it('uses the RESTful current-user endpoint for session bootstrap', () => {
    const source = readUsersApiSource()

    expect(source).toContain("request.get<UserInitResponse>('/api/admin/v1/users/me')")
    expect(source).toContain('me: fetchCurrentUser')
    expect(source).toContain('init: fetchCurrentUser')
    expect(source).not.toContain("request.post<UserInitResponse>('/api/Users/init'")
  })

  it('uses the RESTful auth refresh endpoint', () => {
    const source = readAuthSessionSource()

    expect(source).toContain('`${baseURL}/api/admin/v1/auth/refresh`')
    expect(source).toContain("originalRequest.url?.includes('/api/admin/v1/auth/refresh')")
    expect(source).not.toContain('`${baseURL}/api/Users/refresh`')
    expect(source).not.toContain("originalRequest.url?.includes('/api/Users/refresh')")
  })
})
```

- [ ] **Step 2: Run the frontend contract test and verify it fails**

Run from `E:\admin_go\admin_front_ts`:

```powershell
npm run test -- tests/shared/user/users-api.test.ts
```

Expected failure lines include:

```text
expected "..." to contain "request.get<UserInitResponse>('/api/admin/v1/users/me')"
expected "..." to contain "`${baseURL}/api/admin/v1/auth/refresh`"
```

- [ ] **Step 3: Switch users API bootstrap to RESTful GET**

In `src/api/user/users.ts`, replace the current `UsersApi` opening with this code:

```ts
const fetchCurrentUser = () =>
  request.get<UserInitResponse>('/api/admin/v1/users/me')

export const UsersApi = {
  me: fetchCurrentUser,
  init: fetchCurrentUser,

  getLoginConfig: () =>
    request.post<LoginConfigResponse>('/api/Users/getLoginConfig', {}),
```

Keep the remaining `login`, `refresh`, `logout`, `sendCode`, `forgetPassword`, `initPersonal`, `editPersonal`, `EditPassword`, `updatePhone`, `updateEmail`, and `updatePassword` entries exactly as they are in this task. This task only changes the bootstrap read contract.

- [ ] **Step 4: Run the focused frontend contract test and verify only refresh is still failing**

Run from `E:\admin_go\admin_front_ts`:

```powershell
npm run test -- tests/shared/user/users-api.test.ts
```

Expected failure lines now mention only `/api/admin/v1/auth/refresh` because Task 4 has not changed `auth-session.ts` yet.

- [ ] **Step 5: Commit frontend users API switch**

Run from `E:\admin_go\admin_front_ts`:

```powershell
git add src/api/user/users.ts tests/shared/user/users-api.test.ts
git commit -m "feat: use REST current user endpoint"
```

Expected output includes:

```text
feat: use REST current user endpoint
```

---

### Task 4: Frontend refresh endpoint switch

**Files:**
- Modify: `src/lib/http/auth-session.ts`
- Modify: `tests/shared/user/users-api.test.ts`

- [ ] **Step 1: Change refresh request URL**

In `src/lib/http/auth-session.ts`, replace:

```ts
      `${baseURL}/api/Users/refresh`,
```

with:

```ts
      `${baseURL}/api/admin/v1/auth/refresh`,
```

- [ ] **Step 2: Change refresh self-check URL**

In `src/lib/http/auth-session.ts`, replace:

```ts
    if (originalRequest.url?.includes('/api/Users/refresh') || originalRequest._retry) {
```

with:

```ts
    if (originalRequest.url?.includes('/api/admin/v1/auth/refresh') || originalRequest._retry) {
```

- [ ] **Step 3: Run the focused frontend contract test and verify it passes**

Run from `E:\admin_go\admin_front_ts`:

```powershell
npm run test -- tests/shared/user/users-api.test.ts
```

Expected output includes:

```text
PASS  tests/shared/user/users-api.test.ts
```

- [ ] **Step 4: Commit frontend refresh switch**

Run from `E:\admin_go\admin_front_ts`:

```powershell
git add src/lib/http/auth-session.ts tests/shared/user/users-api.test.ts
git commit -m "feat: use REST auth refresh endpoint"
```

Expected output includes:

```text
feat: use REST auth refresh endpoint
```

---

### Task 5: Full verification and migration evidence

**Files:**
- No required file changes.

- [ ] **Step 1: Run all backend tests**

Run from `E:\admin_go\admin_back_go`:

```powershell
go test ./...
```

Expected output:

```text
ok  	admin_back_go/...
```

The exact package list may include multiple `ok` lines. Any `FAIL` line blocks completion.

- [ ] **Step 2: Run focused frontend contract tests**

Run from `E:\admin_go\admin_front_ts`:

```powershell
npm run test -- tests/shared/user/users-api.test.ts
```

Expected output includes:

```text
PASS  tests/shared/user/users-api.test.ts
```

- [ ] **Step 3: Run frontend type/build verification**

Run from `E:\admin_go\admin_front_ts`:

```powershell
npm run build:check
```

Expected output includes:

```text
vue-tsc -b && vite build
```

and exits with code `0`. If Node/Vite crashes with a Windows memory or spawn error, stop and report the exact error text instead of claiming frontend verification passed.

- [ ] **Step 4: Prove the legacy bootstrap call is gone from the frontend session path**

Run from `E:\admin_go`:

```powershell
rg -n "/api/Users/init|/api/Users/refresh" admin_front_ts/src/api/user/users.ts admin_front_ts/src/lib/http/auth-session.ts admin_front_ts/src/store/user.ts
```

Expected output:

```text
admin_front_ts/src/api/user/users.ts:<line>:    request.post<UserLoginSession>('/api/Users/refresh', params),
```

This remaining `UsersApi.refresh` entry is allowed because login and direct legacy refresh API cleanup are not part of this slice. There must be no match for `/api/Users/init` and no match for `/api/Users/refresh` inside `src/lib/http/auth-session.ts`.

- [ ] **Step 5: Prove the new RESTful paths exist**

Run from `E:\admin_go`:

```powershell
rg -n "/api/admin/v1/users/me|/api/admin/v1/auth/refresh" admin_back_go/internal admin_front_ts/src admin_front_ts/tests/shared/user/users-api.test.ts
```

Expected output includes matches in:

```text
admin_back_go/internal/module/user/route.go
admin_back_go/internal/module/user/handler_test.go
admin_back_go/internal/server/router_test.go
admin_front_ts/src/api/user/users.ts
admin_front_ts/src/lib/http/auth-session.ts
admin_front_ts/tests/shared/user/users-api.test.ts
```

- [ ] **Step 6: Check repo status without staging unrelated old work**

Run from `E:\admin_go`:

```powershell
git -C admin_back_go status --short
git -C admin_front_ts status --short
```

Expected result:

```text
admin_front_ts has no uncommitted files from this slice.
admin_back_go may still show pre-existing scaffold files from earlier Go skeleton work; this slice must not leave handler, route, router test, or plan files uncommitted.
```

---

## Self-review result

Spec coverage:

- `GET /api/admin/v1/users/me`: Task 1 and Task 2.
- Keep `POST /api/Users/init`: Task 1 route keeps it.
- Frontend bootstrap switch: Task 3.
- Refresh switch to `/api/admin/v1/auth/refresh`: Task 4.
- No MySQL schema change: file structure and Task 5 status check.
- No frontend fallback fields: Task 3 uses the existing `UserInitResponse` shape and keeps store assignment unchanged.
- Verification: Task 5.

Red-flag scan:

- No unfinished markers.
- No deferred implementation slots.
- Every code-changing task includes exact code or exact replacement text.

Type consistency:

- Backend uses existing `InitService`, `InitInput`, `InitResponse`, `QuickEntry`, `middleware.AuthIdentity`, and `permission` DTOs.
- Frontend keeps `UserInitResponse` and introduces `fetchCurrentUser` once, then binds `me` and `init` to the same function.
