package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/config"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/auth"
	"admin_back_go/internal/module/authplatform"
	"admin_back_go/internal/module/captcha"
	"admin_back_go/internal/module/operationlog"
	"admin_back_go/internal/module/permission"
	"admin_back_go/internal/module/queuemonitor"
	realtimemodule "admin_back_go/internal/module/realtime"
	"admin_back_go/internal/module/role"
	"admin_back_go/internal/module/session"
	"admin_back_go/internal/module/systemlog"
	"admin_back_go/internal/module/user"
	platformrealtime "admin_back_go/internal/platform/realtime"
	"admin_back_go/internal/readiness"

	"github.com/gorilla/websocket"
)

type fakeReadinessChecker struct {
	report readiness.Report
}

func (f fakeReadinessChecker) Readiness(ctx context.Context) readiness.Report {
	return f.report
}

type fakeAuthService struct{}

func (fakeAuthService) Login(ctx context.Context, input auth.LoginInput) (*auth.LoginResponse, *apperror.Error) {
	return &auth.LoginResponse{
		AccessToken:      "access-token",
		RefreshToken:     "refresh-token",
		ExpiresIn:        14400,
		RefreshExpiresIn: 1209600,
	}, nil
}

func (fakeAuthService) SendCode(ctx context.Context, input auth.SendCodeInput) (string, *apperror.Error) {
	return "验证码发送成功(测试:123456)", nil
}

func (fakeAuthService) LoginConfig(ctx context.Context, platform string) (*auth.LoginConfigResponse, *apperror.Error) {
	return &auth.LoginConfigResponse{
		LoginTypeArr:   []auth.LoginTypeOption{{Label: "密码登录", Value: auth.LoginTypePassword}},
		CaptchaEnabled: true,
		CaptchaType:    captcha.TypeSlide,
	}, nil
}

func (fakeAuthService) Refresh(ctx context.Context, input session.RefreshInput) (*session.TokenResult, *apperror.Error) {
	return &session.TokenResult{
		AccessToken:      "new-access",
		RefreshToken:     "new-refresh",
		ExpiresIn:        14400,
		RefreshExpiresIn: 1209600,
	}, nil
}

func (fakeAuthService) Logout(ctx context.Context, accessToken string) *apperror.Error {
	return nil
}

type fakeCaptchaService struct{}

func (fakeCaptchaService) Generate(ctx context.Context) (*captcha.ChallengeResponse, *apperror.Error) {
	return &captcha.ChallengeResponse{
		CaptchaID:   "captcha-id",
		CaptchaType: captcha.TypeSlide,
		MasterImage: "data:image/jpeg;base64,master",
		TileImage:   "data:image/png;base64,tile",
		TileX:       7,
		TileY:       53,
		TileWidth:   62,
		TileHeight:  62,
		ImageWidth:  300,
		ImageHeight: 220,
		ExpiresIn:   120,
	}, nil
}

type fakeRouterUserService struct {
	input          user.InitInput
	result         *user.InitResponse
	err            *apperror.Error
	pageInitCalled bool
	listQuery      user.ListQuery
	listResult     *user.ListResponse
}

func (f *fakeRouterUserService) Init(ctx context.Context, input user.InitInput) (*user.InitResponse, *apperror.Error) {
	f.input = input
	return f.result, f.err
}

func (f *fakeRouterUserService) PageInit(ctx context.Context) (*user.PageInitResponse, *apperror.Error) {
	f.pageInitCalled = true
	return &user.PageInitResponse{}, f.err
}

func (f *fakeRouterUserService) List(ctx context.Context, query user.ListQuery) (*user.ListResponse, *apperror.Error) {
	f.listQuery = query
	if f.listResult != nil {
		return f.listResult, f.err
	}
	return &user.ListResponse{
		List: []user.ListItem{{ID: 1, Username: "admin"}},
		Page: user.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, f.err
}

func (f *fakeRouterUserService) Update(ctx context.Context, id int64, input user.UpdateInput) *apperror.Error {
	return f.err
}

func (f *fakeRouterUserService) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	return f.err
}

func (f *fakeRouterUserService) Delete(ctx context.Context, ids []int64) *apperror.Error {
	return f.err
}

func (f *fakeRouterUserService) BatchUpdateProfile(ctx context.Context, input user.BatchProfileUpdate) *apperror.Error {
	return f.err
}

type fakeRouterPermissionService struct {
	listQuery permission.PermissionListQuery
}

func (f *fakeRouterPermissionService) Init(ctx context.Context) (*permission.InitResponse, *apperror.Error) {
	return &permission.InitResponse{Dict: permission.PermissionDict{}}, nil
}

func (f *fakeRouterPermissionService) List(ctx context.Context, query permission.PermissionListQuery) ([]permission.PermissionListItem, *apperror.Error) {
	f.listQuery = query
	return []permission.PermissionListItem{{ID: 1, Name: "系统"}}, nil
}

func (f *fakeRouterPermissionService) Create(ctx context.Context, input permission.PermissionMutationInput) (int64, *apperror.Error) {
	return 1, nil
}

func (f *fakeRouterPermissionService) Update(ctx context.Context, id int64, input permission.PermissionMutationInput) *apperror.Error {
	return nil
}

func (f *fakeRouterPermissionService) Delete(ctx context.Context, ids []int64) *apperror.Error {
	return nil
}

func (f *fakeRouterPermissionService) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	return nil
}

type fakeRouterRoleService struct {
	listQuery role.ListQuery
}

func (f *fakeRouterRoleService) Init(ctx context.Context) (*role.InitResponse, *apperror.Error) {
	return &role.InitResponse{}, nil
}

func (f *fakeRouterRoleService) List(ctx context.Context, query role.ListQuery) (*role.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &role.ListResponse{
		List: []role.ListItem{{ID: 1, Name: "管理员"}},
		Page: role.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, nil
}

func (f *fakeRouterRoleService) Create(ctx context.Context, input role.MutationInput) (int64, *apperror.Error) {
	return 1, nil
}

func (f *fakeRouterRoleService) Update(ctx context.Context, id int64, input role.MutationInput) *apperror.Error {
	return nil
}

func (f *fakeRouterRoleService) Delete(ctx context.Context, ids []int64) *apperror.Error {
	return nil
}

func (f *fakeRouterRoleService) SetDefault(ctx context.Context, id int64) *apperror.Error {
	return nil
}

type fakeRouterAuthPlatformService struct {
	listQuery authplatform.ListQuery
}

func (f *fakeRouterAuthPlatformService) Init(ctx context.Context) (*authplatform.InitResponse, *apperror.Error) {
	return (&authplatform.Service{}).Init(ctx)
}

func (f *fakeRouterAuthPlatformService) List(ctx context.Context, query authplatform.ListQuery) (*authplatform.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &authplatform.ListResponse{
		List: []authplatform.ListItem{{ID: 1, Code: "admin", Name: "PC后台", CaptchaType: captcha.TypeSlide}},
		Page: authplatform.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, nil
}

func (f *fakeRouterAuthPlatformService) Create(ctx context.Context, input authplatform.CreateInput) (int64, *apperror.Error) {
	return 1, nil
}

func (f *fakeRouterAuthPlatformService) Update(ctx context.Context, id int64, input authplatform.UpdateInput) *apperror.Error {
	return nil
}

func (f *fakeRouterAuthPlatformService) Delete(ctx context.Context, ids []int64) *apperror.Error {
	return nil
}

func (f *fakeRouterAuthPlatformService) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	return nil
}

type fakeRouterOperationLogService struct {
	initCalled bool
	listQuery  operationlog.ListQuery
	deleteIDs  []int64
	listResult *operationlog.ListResponse
}

func (f *fakeRouterOperationLogService) Init(ctx context.Context) (*operationlog.InitResponse, *apperror.Error) {
	f.initCalled = true
	return &operationlog.InitResponse{}, nil
}

func (f *fakeRouterOperationLogService) List(ctx context.Context, query operationlog.ListQuery) (*operationlog.ListResponse, *apperror.Error) {
	f.listQuery = query
	if f.listResult != nil {
		return f.listResult, nil
	}
	return &operationlog.ListResponse{
		Page: operationlog.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, nil
}

func (f *fakeRouterOperationLogService) Delete(ctx context.Context, ids []int64) *apperror.Error {
	f.deleteIDs = ids
	return nil
}

type fakeRouterSystemLogService struct {
	filesCalled bool
	linesQuery  systemlog.LinesQuery
}

func (f *fakeRouterSystemLogService) Init(ctx context.Context) (*systemlog.InitResponse, *apperror.Error) {
	return systemlog.NewService(nil).Init(ctx)
}

func (f *fakeRouterSystemLogService) Files(ctx context.Context) (*systemlog.FilesResponse, *apperror.Error) {
	f.filesCalled = true
	return &systemlog.FilesResponse{List: []systemlog.FileItem{{Name: "admin-api.log", Size: 1, SizeHuman: "1 B", MTime: "2026-05-04 10:00:00"}}}, nil
}

func (f *fakeRouterSystemLogService) Lines(ctx context.Context, query systemlog.LinesQuery) (*systemlog.LinesResponse, *apperror.Error) {
	f.linesQuery = query
	return &systemlog.LinesResponse{Filename: query.Filename, Total: 1, Lines: []systemlog.LineItem{{Number: 1, Level: "ERROR", Content: "ERROR boom"}}}, nil
}

type fakeRouterQueueMonitorService struct {
	listCalled      bool
	failedListQuery queuemonitor.FailedListQuery
}

type fakeQueueMonitorUI struct {
	called bool
	path   string
	method string
}

func (f *fakeQueueMonitorUI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.called = true
	f.path = r.URL.Path
	f.method = r.Method
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("queue monitor ui"))
}

func (f *fakeRouterQueueMonitorService) List(ctx context.Context) ([]queuemonitor.QueueItem, *apperror.Error) {
	f.listCalled = true
	return []queuemonitor.QueueItem{{Name: "critical", Label: "高优先级队列", Group: "critical"}}, nil
}

func (f *fakeRouterQueueMonitorService) FailedList(ctx context.Context, query queuemonitor.FailedListQuery) (*queuemonitor.FailedListResponse, *apperror.Error) {
	f.failedListQuery = query
	return &queuemonitor.FailedListResponse{
		List: []queuemonitor.FailedTaskItem{{ID: "task-1", State: "retry"}},
		Page: queuemonitor.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, nil
}

func TestHealthEndpointReturnsOK(t *testing.T) {
	router := newTestRouter(t, Dependencies{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	router.ServeHTTP(recorder, request)

	assertRequestID(t, recorder)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	body := decodeRouterBody(t, recorder)
	if body["code"] != float64(0) {
		t.Fatalf("expected code 0, got %#v", body["code"])
	}
	if body["msg"] != "ok" {
		t.Fatalf("expected msg ok, got %#v", body["msg"])
	}

	data := mustRouterData(t, body)
	if data["status"] != "ok" {
		t.Fatalf("expected data.status ok, got %#v", data["status"])
	}
}

func TestPingEndpointReturnsPong(t *testing.T) {
	router := newTestRouter(t, Dependencies{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/ping", nil)
	router.ServeHTTP(recorder, request)

	assertRequestID(t, recorder)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	body := decodeRouterBody(t, recorder)
	data := mustRouterData(t, body)
	if data["message"] != "pong" {
		t.Fatalf("expected data.message pong, got %#v", data["message"])
	}
}

func TestReadyEndpointReturnsReadyWhenResourcesAreDisabled(t *testing.T) {
	router := newTestRouter(t, Dependencies{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	router.ServeHTTP(recorder, request)

	assertRequestID(t, recorder)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	body := decodeRouterBody(t, recorder)
	if body["code"] != float64(0) {
		t.Fatalf("expected code 0, got %#v", body["code"])
	}

	data := mustRouterData(t, body)
	if data["status"] != readiness.StatusReady {
		t.Fatalf("expected ready status, got %#v", data["status"])
	}
	checks, ok := data["checks"].(map[string]any)
	if !ok {
		t.Fatalf("expected checks object, got %#v", data["checks"])
	}
	database, ok := checks["database"].(map[string]any)
	if !ok || database["status"] != readiness.StatusDisabled {
		t.Fatalf("expected disabled database check, got %#v", checks["database"])
	}
	queueRedis, ok := checks["queue_redis"].(map[string]any)
	if !ok || queueRedis["status"] != readiness.StatusDisabled {
		t.Fatalf("expected disabled queue_redis check, got %#v", checks["queue_redis"])
	}
	realtimeCheck, ok := checks["realtime"].(map[string]any)
	if !ok || realtimeCheck["status"] != readiness.StatusDisabled {
		t.Fatalf("expected disabled realtime check, got %#v", checks["realtime"])
	}
}

func TestReadyEndpointReturnsErrorWithDetailsWhenResourceIsDown(t *testing.T) {
	router := newTestRouter(t, Dependencies{Readiness: fakeReadinessChecker{report: readiness.NewReport(map[string]readiness.Check{
		"database": {Status: readiness.StatusDown, Message: "connection refused"},
		"redis":    {Status: readiness.StatusDisabled},
	})}})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	router.ServeHTTP(recorder, request)

	assertRequestID(t, recorder)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, recorder.Code)
	}

	body := decodeRouterBody(t, recorder)
	if body["code"] != float64(500) {
		t.Fatalf("expected code 500, got %#v", body["code"])
	}
	if body["msg"] != "service not ready" {
		t.Fatalf("expected service not ready message, got %#v", body["msg"])
	}

	data := mustRouterData(t, body)
	if data["status"] != readiness.StatusNotReady {
		t.Fatalf("expected not_ready status, got %#v", data["status"])
	}
}

func TestRouterInstallsAccessLogAfterRequestID(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))
	router := NewRouter(Dependencies{Logger: logger})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Header.Set(middleware.HeaderRequestID, "rid-router")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	entry := decodeRouterLogEntry(t, buffer.Bytes())
	if entry["msg"] != "http request" {
		t.Fatalf("expected http request log message, got %#v", entry["msg"])
	}
	if entry["request_id"] != "rid-router" {
		t.Fatalf("expected request_id rid-router, got %#v", entry["request_id"])
	}
	if entry["method"] != http.MethodGet {
		t.Fatalf("expected method GET, got %#v", entry["method"])
	}
	if entry["path"] != "/health" {
		t.Fatalf("expected path /health, got %#v", entry["path"])
	}
	if entry["status"] != float64(http.StatusOK) {
		t.Fatalf("expected status 200, got %#v", entry["status"])
	}
}

func TestRouterInstallsCORSAfterAccessLog(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))
	router := NewRouter(Dependencies{
		Logger: logger,
		CORS: config.CORSConfig{
			AllowOrigins:  []string{"http://localhost:5173"},
			AllowMethods:  []string{http.MethodGet, http.MethodOptions},
			AllowHeaders:  []string{"Content-Type", "Authorization", "platform", "device-id", "X-Trace-Id", middleware.HeaderRequestID},
			ExposeHeaders: []string{middleware.HeaderRequestID},
			MaxAge:        12 * time.Hour,
		},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/health", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("Access-Control-Request-Method", http.MethodGet)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("expected allowed origin, got %q", got)
	}

	entry := decodeRouterLogEntry(t, buffer.Bytes())
	if entry["msg"] != "http request" {
		t.Fatalf("expected http request log message, got %#v", entry["msg"])
	}
	if entry["status"] != float64(http.StatusNoContent) {
		t.Fatalf("expected access log status 204, got %#v", entry["status"])
	}
}

func TestRouterInstallsAuthTokenForNonPublicPaths(t *testing.T) {
	router := newTestRouter(t, Dependencies{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/private", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusUnauthorized, recorder.Code, recorder.Body.String())
	}
	body := decodeRouterBody(t, recorder)
	if body["code"] != float64(401) {
		t.Fatalf("expected code 401, got %#v", body["code"])
	}
	if body["msg"] != "缺少Token" {
		t.Fatalf("expected missing token message, got %#v", body["msg"])
	}
}

func TestRouterInstallsRefreshEndpointAsPublicPath(t *testing.T) {
	router := newTestRouter(t, Dependencies{AuthService: fakeAuthService{}})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/refresh", strings.NewReader(`{"refresh_token":"refresh-token"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	body := decodeRouterBody(t, recorder)
	data := mustRouterData(t, body)
	if data["access_token"] != "new-access" {
		t.Fatalf("expected refresh endpoint response, got %#v", data)
	}
}

func TestRouterInstallsLoginEndpointsAsPublicPaths(t *testing.T) {
	router := newTestRouter(t, Dependencies{AuthService: fakeAuthService{}})

	configRecorder := httptest.NewRecorder()
	configRequest := httptest.NewRequest(http.MethodGet, "/api/admin/v1/auth/login-config", nil)
	configRequest.Header.Set("platform", "admin")
	router.ServeHTTP(configRecorder, configRequest)
	if configRecorder.Code != http.StatusOK {
		t.Fatalf("expected login config status %d, got %d body=%s", http.StatusOK, configRecorder.Code, configRecorder.Body.String())
	}

	loginRecorder := httptest.NewRecorder()
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/login", strings.NewReader(`{"login_account":"15671628271","login_type":"password","password":"123456","captcha_id":"captcha-id","captcha_answer":{"x":120,"y":80}}`))
	loginRequest.Header.Set("Content-Type", "application/json")
	loginRequest.Header.Set("platform", "admin")
	router.ServeHTTP(loginRecorder, loginRequest)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("expected login status %d, got %d body=%s", http.StatusOK, loginRecorder.Code, loginRecorder.Body.String())
	}
}

func TestRouterInstallsCaptchaEndpointAsPublicPath(t *testing.T) {
	router := newTestRouter(t, Dependencies{CaptchaService: fakeCaptchaService{}})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/auth/captcha", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected captcha status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	body := decodeRouterBody(t, recorder)
	data := mustRouterData(t, body)
	if data["captcha_id"] != "captcha-id" || data["captcha_type"] != captcha.TypeSlide {
		t.Fatalf("unexpected captcha response: %#v", data)
	}
}

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

func TestRouterInstallsUsersInitAsProtectedRESTPath(t *testing.T) {
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
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/users/init", nil)
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
		t.Fatalf("unexpected users/init payload: %#v", data)
	}
	if _, ok := data["buttonCodes"]; !ok {
		t.Fatalf("missing buttonCodes in users/init payload: %#v", data)
	}
}

func TestRouterInstallsUserManagementRESTRoutes(t *testing.T) {
	userService := &fakeRouterUserService{}
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		UserService: userService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/users?current_page=1&page_size=20&username=admin", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if userService.listQuery.CurrentPage != 1 || userService.listQuery.PageSize != 20 || userService.listQuery.Username != "admin" {
		t.Fatalf("user list query mismatch: %#v", userService.listQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/users/page-init", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !userService.pageInitCalled {
		t.Fatalf("expected users page-init route, code=%d body=%s called=%v", recorder.Code, recorder.Body.String(), userService.pageInitCalled)
	}
}

func TestRouterInstallsPermissionRESTRoutes(t *testing.T) {
	permissionService := &fakeRouterPermissionService{}
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		PermissionService: permissionService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/permissions?platform=admin", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if permissionService.listQuery.Platform != "admin" {
		t.Fatalf("permission list query mismatch: %#v", permissionService.listQuery)
	}
}

func TestRouterInstallsRoleRESTRoutes(t *testing.T) {
	roleService := &fakeRouterRoleService{}
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		RoleService: roleService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/roles?current_page=1&page_size=50&name=管理", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if roleService.listQuery.CurrentPage != 1 || roleService.listQuery.PageSize != 50 || roleService.listQuery.Name != "管理" {
		t.Fatalf("role list query mismatch: %#v", roleService.listQuery)
	}
}

func TestRouterInstallsAuthPlatformRESTRoutes(t *testing.T) {
	authPlatformService := &fakeRouterAuthPlatformService{}
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		AuthPlatformService: authPlatformService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/auth-platforms?current_page=1&page_size=50&status=1", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if authPlatformService.listQuery.CurrentPage != 1 || authPlatformService.listQuery.PageSize != 50 || authPlatformService.listQuery.Status == nil || *authPlatformService.listQuery.Status != 1 {
		t.Fatalf("auth platform list query mismatch: %#v", authPlatformService.listQuery)
	}
	body := decodeRouterBody(t, recorder)
	data := mustRouterData(t, body)
	if _, ok := data["list"]; !ok {
		t.Fatalf("missing list in auth-platforms response: %#v", data)
	}
}

func TestRouterInstallsOperationLogRESTRoutes(t *testing.T) {
	operationLogService := &fakeRouterOperationLogService{}
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		PermissionRules: map[middleware.RouteKey]string{
			middleware.NewRouteKey(http.MethodDelete, "/api/admin/v1/operation-logs/:id"): "devTools_operationLog_del",
		},
		PermissionChecker: func(ctx context.Context, input middleware.PermissionInput) *apperror.Error {
			return nil
		},
		OperationLogService: operationLogService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/operation-logs?current_page=1&page_size=20&action=编辑&date=2026-05-01,2026-05-04", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if operationLogService.listQuery.CurrentPage != 1 || operationLogService.listQuery.PageSize != 20 || operationLogService.listQuery.Action != "编辑" {
		t.Fatalf("operation log list query mismatch: %#v", operationLogService.listQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/admin/v1/operation-logs/9", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected delete status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if !reflect.DeepEqual(operationLogService.deleteIDs, []int64{9}) {
		t.Fatalf("operation log delete mismatch: %#v", operationLogService.deleteIDs)
	}
}

func TestRouterInstallsSystemLogReadOnlyRESTRoutes(t *testing.T) {
	systemLogService := &fakeRouterSystemLogService{}
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		SystemLogService: systemLogService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/system-logs/files", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected system log files status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if !systemLogService.filesCalled {
		t.Fatalf("expected system log files service call")
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/system-logs/files/admin-api.log/lines?tail=500&level=ERROR&keyword=boom", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected system log lines status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if systemLogService.linesQuery.Filename != "admin-api.log" || systemLogService.linesQuery.Tail != 500 || systemLogService.linesQuery.Level != "ERROR" || systemLogService.linesQuery.Keyword != "boom" {
		t.Fatalf("system log lines query mismatch: %#v", systemLogService.linesQuery)
	}
}

func TestRouterInstallsQueueMonitorReadOnlyRESTRoutes(t *testing.T) {
	queueMonitorService := &fakeRouterQueueMonitorService{}
	queueMonitorUI := &fakeQueueMonitorUI{}
	var uiAuthToken string
	var uiAuthPlatform string
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			if strings.HasPrefix(input.AccessToken, "cookie-") {
				uiAuthToken = input.AccessToken
				uiAuthPlatform = input.Platform
			}
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		QueueMonitorService: queueMonitorService,
		QueueMonitorUI:      queueMonitorUI,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/queue-monitor", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if !queueMonitorService.listCalled {
		t.Fatalf("expected queue monitor list call")
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/queue-monitor/failed?queue=critical&current_page=2&page_size=10", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if queueMonitorService.failedListQuery.Queue != "critical" || queueMonitorService.failedListQuery.CurrentPage != 2 || queueMonitorService.failedListQuery.PageSize != 10 {
		t.Fatalf("queue monitor failed query mismatch: %#v", queueMonitorService.failedListQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, queuemonitor.UIPath+"/api/queues", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected queue monitor UI status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if !queueMonitorUI.called || queueMonitorUI.path != queuemonitor.UIPath+"/api/queues" || queueMonitorUI.method != http.MethodGet {
		t.Fatalf("queue monitor UI handler not called as expected: %#v", queueMonitorUI)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, queuemonitor.UIPath, nil)
	request.AddCookie(&http.Cookie{Name: middleware.DefaultAccessTokenCookie, Value: "cookie-access-token"})
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected queue monitor UI cookie status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if uiAuthToken != "cookie-access-token" {
		t.Fatalf("expected queue monitor UI to authenticate with cookie token, got %q", uiAuthToken)
	}
	if uiAuthPlatform != "admin" {
		t.Fatalf("expected queue monitor UI cookie auth to use admin platform, got %q", uiAuthPlatform)
	}
}

func TestRouterInstallsPermissionCheckAfterAuthToken(t *testing.T) {
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		PermissionRules: map[middleware.RouteKey]string{
			middleware.NewRouteKey(http.MethodGet, "/api/admin/v1/users/me"): "user:me",
		},
		PermissionChecker: func(ctx context.Context, input middleware.PermissionInput) *apperror.Error {
			if input.UserID != 1 || input.Code != "user:me" {
				t.Fatalf("unexpected permission input: %#v", input)
			}
			return apperror.Forbidden("无接口权限")
		},
		UserService: &fakeRouterUserService{},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/users/me", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusForbidden, recorder.Code, recorder.Body.String())
	}
	body := decodeRouterBody(t, recorder)
	if body["msg"] != "无接口权限" {
		t.Fatalf("expected permission denial, got %#v", body)
	}
}

func TestRouterInstallsOperationLogAfterPermissionCheck(t *testing.T) {
	var got middleware.OperationInput
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		OperationRules: map[middleware.RouteKey]middleware.OperationRule{
			middleware.NewRouteKey(http.MethodGet, "/api/admin/v1/users/me"): {Module: "user", Action: "me", Title: "查看当前用户"},
		},
		OperationRecorder: func(ctx context.Context, input middleware.OperationInput) error {
			got = input
			return nil
		},
		UserService: &fakeRouterUserService{result: &user.InitResponse{UserID: 1, Username: "admin"}},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/users/me", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if got.UserID != 1 || got.Module != "user" || got.Action != "me" || got.Status != http.StatusOK || !got.Success {
		t.Fatalf("unexpected operation input: %#v", got)
	}
}

func TestRealtimeRouteRequiresAuthAndUpgradesWebSocket(t *testing.T) {
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 7, SessionID: 9, Platform: "admin"}, nil
		},
		RealtimeHandler: realtimemodule.NewHandler(
			realtimemodule.NewService(25*time.Second),
			platformrealtime.NewUpgrader(func(*http.Request) bool { return true }),
			platformrealtime.NewManager(),
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		),
	})
	server := httptest.NewServer(router)
	defer server.Close()

	client, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):]+"/api/admin/v1/realtime/ws", http.Header{
		"Authorization": []string{"Bearer access-token"},
		"platform":      []string{"admin"},
		"device-id":     []string{"codex-test"},
	})
	if err != nil {
		t.Fatalf("dial realtime: %v", err)
	}
	defer client.Close()

	var connected map[string]any
	if err := client.ReadJSON(&connected); err != nil {
		t.Fatalf("read connected: %v", err)
	}
	if connected["type"] != realtimemodule.TypeConnectedV1 {
		t.Fatalf("expected connected event, got %#v", connected)
	}
}

func newTestRouter(t *testing.T, deps Dependencies) http.Handler {
	t.Helper()
	if deps.Logger == nil {
		deps.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return NewRouter(deps)
}

func decodeRouterBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	return body
}

func decodeRouterLogEntry(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(data), &entry); err != nil {
		t.Fatalf("invalid json log entry: %v\n%s", err, data)
	}
	return entry
}

func mustRouterData(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %#v", body["data"])
	}
	return data
}

func assertRequestID(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Header().Get("X-Request-Id") == "" {
		t.Fatalf("expected X-Request-Id header")
	}
}
