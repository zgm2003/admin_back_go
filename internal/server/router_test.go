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
	"admin_back_go/internal/module/notification"
	"admin_back_go/internal/module/notificationtask"
	"admin_back_go/internal/module/operationlog"
	"admin_back_go/internal/module/permission"
	"admin_back_go/internal/module/queuemonitor"
	realtimemodule "admin_back_go/internal/module/realtime"
	"admin_back_go/internal/module/role"
	"admin_back_go/internal/module/session"
	"admin_back_go/internal/module/systemlog"
	"admin_back_go/internal/module/systemsetting"
	"admin_back_go/internal/module/uploadtoken"
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
	profileUserID  int64
	profileViewer  int64
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

func (f *fakeRouterUserService) Profile(ctx context.Context, userID int64, currentUserID int64) (*user.ProfileResponse, *apperror.Error) {
	f.profileUserID = userID
	f.profileViewer = currentUserID
	return &user.ProfileResponse{Profile: user.ProfileDetail{UserID: userID, Username: "admin"}}, f.err
}

func (f *fakeRouterUserService) UpdateProfile(ctx context.Context, input user.UpdateProfileInput) *apperror.Error {
	f.profileUserID = input.UserID
	return f.err
}

func (f *fakeRouterUserService) UpdatePassword(ctx context.Context, input user.UpdatePasswordInput) *apperror.Error {
	f.profileUserID = input.UserID
	return f.err
}

func (f *fakeRouterUserService) UpdateEmail(ctx context.Context, input user.UpdateEmailInput) *apperror.Error {
	f.profileUserID = input.UserID
	return f.err
}

func (f *fakeRouterUserService) UpdatePhone(ctx context.Context, input user.UpdatePhoneInput) *apperror.Error {
	f.profileUserID = input.UserID
	return f.err
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

type fakeRouterNotificationService struct {
	listQuery      notification.ListQuery
	unreadIdentity notification.Identity
	markIdentity   notification.Identity
	markIDs        []int64
	deleteIdentity notification.Identity
	deleteIDs      []int64
}

func (f *fakeRouterNotificationService) Init(ctx context.Context) (*notification.InitResponse, *apperror.Error) {
	return notification.NewService(&fakeRepositoryForNotificationRouter{}).Init(ctx)
}

func (f *fakeRouterNotificationService) List(ctx context.Context, query notification.ListQuery) (*notification.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &notification.ListResponse{
		List: []notification.ListItem{{ID: 1, Title: "通知"}},
		Page: notification.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, nil
}

func (f *fakeRouterNotificationService) UnreadCount(ctx context.Context, identity notification.Identity) (*notification.UnreadCountResponse, *apperror.Error) {
	f.unreadIdentity = identity
	return &notification.UnreadCountResponse{Count: 2}, nil
}

func (f *fakeRouterNotificationService) MarkRead(ctx context.Context, identity notification.Identity, ids []int64) *apperror.Error {
	f.markIdentity = identity
	f.markIDs = append([]int64{}, ids...)
	return nil
}

func (f *fakeRouterNotificationService) Delete(ctx context.Context, identity notification.Identity, ids []int64) *apperror.Error {
	f.deleteIdentity = identity
	f.deleteIDs = append([]int64{}, ids...)
	return nil
}

type fakeRouterNotificationTaskService struct {
	statusCountQuery notificationtask.StatusCountQuery
	listQuery        notificationtask.ListQuery
	createInput      notificationtask.CreateInput
	cancelID         int64
	deleteID         int64
}

func (f *fakeRouterNotificationTaskService) Init(ctx context.Context) (*notificationtask.InitResponse, *apperror.Error) {
	return notificationtask.NewService(&fakeRepositoryForNotificationTaskRouter{}).Init(ctx)
}

func (f *fakeRouterNotificationTaskService) StatusCount(ctx context.Context, query notificationtask.StatusCountQuery) ([]notificationtask.StatusCountItem, *apperror.Error) {
	f.statusCountQuery = query
	return []notificationtask.StatusCountItem{{Label: "待发送", Value: 1, Num: 2}}, nil
}

func (f *fakeRouterNotificationTaskService) List(ctx context.Context, query notificationtask.ListQuery) (*notificationtask.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &notificationtask.ListResponse{
		List: []notificationtask.ListItem{{ID: 1, Title: "发布通知"}},
		Page: notificationtask.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, nil
}

func (f *fakeRouterNotificationTaskService) Create(ctx context.Context, input notificationtask.CreateInput) (*notificationtask.CreateResponse, *apperror.Error) {
	f.createInput = input
	return &notificationtask.CreateResponse{ID: 7, Queued: true}, nil
}

func (f *fakeRouterNotificationTaskService) Cancel(ctx context.Context, id int64) *apperror.Error {
	f.cancelID = id
	return nil
}

func (f *fakeRouterNotificationTaskService) Delete(ctx context.Context, id int64) *apperror.Error {
	f.deleteID = id
	return nil
}

type fakeRepositoryForNotificationTaskRouter struct{}

func (fakeRepositoryForNotificationTaskRouter) List(ctx context.Context, query notificationtask.ListQuery) ([]notificationtask.Task, int64, error) {
	return nil, 0, nil
}

func (fakeRepositoryForNotificationTaskRouter) CountByStatus(ctx context.Context, query notificationtask.StatusCountQuery) (map[int]int64, error) {
	return nil, nil
}

func (fakeRepositoryForNotificationTaskRouter) Create(ctx context.Context, row notificationtask.Task) (int64, error) {
	return 0, nil
}

func (fakeRepositoryForNotificationTaskRouter) Get(ctx context.Context, id int64) (*notificationtask.Task, error) {
	return nil, nil
}

func (fakeRepositoryForNotificationTaskRouter) CancelPending(ctx context.Context, id int64) (int64, error) {
	return 0, nil
}

func (fakeRepositoryForNotificationTaskRouter) Delete(ctx context.Context, id int64) (int64, error) {
	return 0, nil
}

func (fakeRepositoryForNotificationTaskRouter) CountTargetUsers(ctx context.Context, targetType int, targetIDs []int64) (int, error) {
	return 0, nil
}

func (fakeRepositoryForNotificationTaskRouter) ClaimDueTasks(ctx context.Context, now time.Time, limit int) ([]int64, error) {
	return nil, nil
}

func (fakeRepositoryForNotificationTaskRouter) ClaimSendTask(ctx context.Context, id int64) (*notificationtask.Task, bool, error) {
	return nil, false, nil
}

func (fakeRepositoryForNotificationTaskRouter) TargetUserIDs(ctx context.Context, task notificationtask.Task) ([]int64, error) {
	return nil, nil
}

func (fakeRepositoryForNotificationTaskRouter) InsertNotifications(ctx context.Context, rows []notificationtask.Notification) error {
	return nil
}

func (fakeRepositoryForNotificationTaskRouter) UpdateProgress(ctx context.Context, id int64, sentCount int, totalCount int) error {
	return nil
}

func (fakeRepositoryForNotificationTaskRouter) MarkSuccess(ctx context.Context, id int64, sentCount int, totalCount int) error {
	return nil
}

func (fakeRepositoryForNotificationTaskRouter) MarkFailed(ctx context.Context, id int64, errMsg string) error {
	return nil
}

type fakeRepositoryForNotificationRouter struct{}

func (fakeRepositoryForNotificationRouter) List(ctx context.Context, query notification.ListQuery) ([]notification.Notification, int64, error) {
	return nil, 0, nil
}

func (fakeRepositoryForNotificationRouter) UnreadCount(ctx context.Context, userID int64, platform string) (int64, error) {
	return 0, nil
}

func (fakeRepositoryForNotificationRouter) MarkRead(ctx context.Context, input notification.MarkReadInput) (int64, error) {
	return 0, nil
}

func (fakeRepositoryForNotificationRouter) Delete(ctx context.Context, input notification.DeleteInput) (int64, error) {
	return 0, nil
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

type fakeRouterSystemSettingService struct {
	listQuery systemsetting.ListQuery
	statusID  int64
	status    int
}

func (f *fakeRouterSystemSettingService) Init(ctx context.Context) (*systemsetting.InitResponse, *apperror.Error) {
	return systemsetting.NewService(nil).Init(ctx)
}

func (f *fakeRouterSystemSettingService) List(ctx context.Context, query systemsetting.ListQuery) (*systemsetting.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &systemsetting.ListResponse{
		List: []systemsetting.ListItem{{ID: 1, SettingKey: "user.default_avatar"}},
		Page: systemsetting.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: 1, TotalPage: 1},
	}, nil
}

func (f *fakeRouterSystemSettingService) Create(ctx context.Context, input systemsetting.CreateInput) (int64, *apperror.Error) {
	return 1, nil
}

func (f *fakeRouterSystemSettingService) Update(ctx context.Context, id int64, input systemsetting.UpdateInput) *apperror.Error {
	return nil
}

func (f *fakeRouterSystemSettingService) Delete(ctx context.Context, ids []int64) *apperror.Error {
	return nil
}

func (f *fakeRouterSystemSettingService) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	f.statusID = id
	f.status = status
	return nil
}

type fakeRouterUploadTokenService struct {
	input uploadtoken.CreateInput
}

func (f *fakeRouterUploadTokenService) Create(ctx context.Context, input uploadtoken.CreateInput) (*uploadtoken.CreateResponse, *apperror.Error) {
	f.input = input
	return &uploadtoken.CreateResponse{
		Provider: "cos",
		Bucket:   "bucket-a",
		Region:   "ap-nanjing",
		Key:      "images/2026/05/05/demo.png",
		Credentials: uploadtoken.CredentialsDTO{
			TmpSecretID:  "tmp-id",
			TmpSecretKey: "tmp-key",
			SessionToken: "session-token",
		},
		StartTime:   100,
		ExpiredTime: 200,
		Rule: uploadtoken.UploadRuleDTO{
			MaxSizeMB: 2,
			ImageExts: []string{
				"png",
			},
			FileExts: []string{
				"pdf",
			},
		},
	}, nil
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

func TestRouterRefreshEndpointIncludesCORSHeaders(t *testing.T) {
	router := newTestRouter(t, Dependencies{
		CORS:        config.DefaultCORSConfig(),
		AuthService: fakeAuthService{},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/refresh", strings.NewReader(`{"refresh_token":"refresh-token"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://127.0.0.1:5173")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:5173" {
		t.Fatalf("expected refresh CORS allow origin, got %q", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected refresh CORS credentials true, got %q", got)
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

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/profile", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || userService.profileUserID != 1 || userService.profileViewer != 1 {
		t.Fatalf("expected current profile route, code=%d body=%s service=%#v", recorder.Code, recorder.Body.String(), userService)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/users/9/profile", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || userService.profileUserID != 9 || userService.profileViewer != 1 {
		t.Fatalf("expected target profile route, code=%d body=%s service=%#v", recorder.Code, recorder.Body.String(), userService)
	}
}

func TestRouterInstallsNotificationListAsCurrentUserRESTPath(t *testing.T) {
	notificationService := &fakeRouterNotificationService{}
	var authInput middleware.TokenInput
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			authInput = input
			return &middleware.AuthIdentity{UserID: 12, SessionID: 10, Platform: input.Platform}, nil
		},
		NotificationService: notificationService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/notifications?current_page=1&page_size=5&keyword=%E5%AF%BC%E5%87%BA&type=2&level=2&is_read=2", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("platform", "admin")
	request.Header.Set("device-id", "desktop-1")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected notification list status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if authInput.AccessToken != "access-token" || authInput.Platform != "admin" || authInput.DeviceID != "desktop-1" {
		t.Fatalf("unexpected auth input: %#v", authInput)
	}
	query := notificationService.listQuery
	if query.UserID != 12 || query.Platform != "admin" || query.CurrentPage != 1 || query.PageSize != 5 || query.Keyword != "导出" {
		t.Fatalf("notification list query mismatch: %#v", query)
	}
	if query.Type == nil || *query.Type != 2 || query.Level == nil || *query.Level != 2 || query.IsRead == nil || *query.IsRead != 2 {
		t.Fatalf("notification list filters mismatch: %#v", query)
	}
	body := decodeRouterBody(t, recorder)
	data := mustRouterData(t, body)
	if _, ok := data["list"]; !ok {
		t.Fatalf("missing notification list in response: %#v", data)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/notifications/unread-count", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("platform", "admin")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected notification unread-count status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if notificationService.unreadIdentity.UserID != 12 || notificationService.unreadIdentity.Platform != "admin" {
		t.Fatalf("notification unread identity mismatch: %#v", notificationService.unreadIdentity)
	}
}

func TestRouterInstallsNotificationReadAndDeleteRoutes(t *testing.T) {
	notificationService := &fakeRouterNotificationService{}
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 12, SessionID: 10, Platform: "admin"}, nil
		},
		NotificationService: notificationService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/notifications/7/read", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected mark-one-read status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if notificationService.markIdentity.UserID != 12 || notificationService.markIdentity.Platform != "admin" || !reflect.DeepEqual(notificationService.markIDs, []int64{7}) {
		t.Fatalf("notification mark-one-read mismatch: identity=%#v ids=%#v", notificationService.markIdentity, notificationService.markIDs)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPatch, "/api/admin/v1/notifications/read", strings.NewReader(`{"ids":[3,4]}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected mark-batch-read status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if !reflect.DeepEqual(notificationService.markIDs, []int64{3, 4}) {
		t.Fatalf("notification mark-batch-read ids mismatch: %#v", notificationService.markIDs)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPatch, "/api/admin/v1/notifications/read", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected mark-all-read status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if len(notificationService.markIDs) != 0 {
		t.Fatalf("notification mark-all-read must pass empty ids, got %#v", notificationService.markIDs)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/admin/v1/notifications/9", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected delete-one status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if notificationService.deleteIdentity.UserID != 12 || notificationService.deleteIdentity.Platform != "admin" || !reflect.DeepEqual(notificationService.deleteIDs, []int64{9}) {
		t.Fatalf("notification delete-one mismatch: identity=%#v ids=%#v", notificationService.deleteIdentity, notificationService.deleteIDs)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/admin/v1/notifications", strings.NewReader(`{"ids":[1,2]}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected delete-batch status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if !reflect.DeepEqual(notificationService.deleteIDs, []int64{1, 2}) {
		t.Fatalf("notification delete-batch ids mismatch: %#v", notificationService.deleteIDs)
	}
}

func TestRouterInstallsNotificationTaskRESTRoutes(t *testing.T) {
	notificationTaskService := &fakeRouterNotificationTaskService{}
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 12, SessionID: 10, Platform: "admin"}, nil
		},
		NotificationTaskService: notificationTaskService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/notification-tasks/init", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected notification task init status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/notification-tasks/status-count?title=%E5%8F%91%E5%B8%83", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || notificationTaskService.statusCountQuery.Title != "发布" {
		t.Fatalf("expected notification task status-count route, code=%d body=%s query=%#v", recorder.Code, recorder.Body.String(), notificationTaskService.statusCountQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/notification-tasks?current_page=2&page_size=10&status=1&title=%E9%80%9A%E7%9F%A5", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected notification task list status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if notificationTaskService.listQuery.CurrentPage != 2 || notificationTaskService.listQuery.PageSize != 10 || notificationTaskService.listQuery.Title != "通知" {
		t.Fatalf("notification task list query mismatch: %#v", notificationTaskService.listQuery)
	}
	if notificationTaskService.listQuery.Status == nil || *notificationTaskService.listQuery.Status != 1 {
		t.Fatalf("notification task list status mismatch: %#v", notificationTaskService.listQuery.Status)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/v1/notification-tasks", strings.NewReader(`{"title":"发布通知","target_type":2,"target_ids":[3,4],"platform":"admin"}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected notification task create status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if notificationTaskService.createInput.CreatedBy != 12 || notificationTaskService.createInput.Title != "发布通知" || notificationTaskService.createInput.Platform != "admin" {
		t.Fatalf("notification task create input mismatch: %#v", notificationTaskService.createInput)
	}
	if !reflect.DeepEqual(notificationTaskService.createInput.TargetIDs, []int64{3, 4}) {
		t.Fatalf("notification task create target ids mismatch: %#v", notificationTaskService.createInput.TargetIDs)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPatch, "/api/admin/v1/notification-tasks/7/cancel", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || notificationTaskService.cancelID != 7 {
		t.Fatalf("expected notification task cancel route, code=%d body=%s id=%d", recorder.Code, recorder.Body.String(), notificationTaskService.cancelID)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/admin/v1/notification-tasks/8", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || notificationTaskService.deleteID != 8 {
		t.Fatalf("expected notification task delete route, code=%d body=%s id=%d", recorder.Code, recorder.Body.String(), notificationTaskService.deleteID)
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

func TestRouterInstallsSystemSettingRESTRoutes(t *testing.T) {
	systemSettingService := &fakeRouterSystemSettingService{}
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		PermissionRules: map[middleware.RouteKey]string{
			middleware.NewRouteKey(http.MethodPatch, "/api/admin/v1/system-settings/:id/status"): "system_setting_status",
		},
		PermissionChecker: func(ctx context.Context, input middleware.PermissionInput) *apperror.Error {
			return nil
		},
		SystemSettingService: systemSettingService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/system-settings?current_page=1&page_size=20&key=user.&status=1", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected system settings list status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if systemSettingService.listQuery.CurrentPage != 1 || systemSettingService.listQuery.PageSize != 20 || systemSettingService.listQuery.Key != "user." || systemSettingService.listQuery.Status == nil || *systemSettingService.listQuery.Status != 1 {
		t.Fatalf("system setting list query mismatch: %#v", systemSettingService.listQuery)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPatch, "/api/admin/v1/system-settings/2/status", strings.NewReader(`{"status":2}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status change status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if systemSettingService.statusID != 2 || systemSettingService.status != 2 {
		t.Fatalf("system setting status mismatch: id=%d status=%d", systemSettingService.statusID, systemSettingService.status)
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

func TestRouterInstallsUploadTokenCreateRoute(t *testing.T) {
	uploadTokenService := &fakeRouterUploadTokenService{}
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"}, nil
		},
		PermissionRules: map[middleware.RouteKey]string{
			middleware.NewRouteKey(http.MethodPost, "/api/admin/v1/upload-tokens"): "system_uploadToken_create",
		},
		PermissionChecker: func(ctx context.Context, input middleware.PermissionInput) *apperror.Error {
			if input.Code != "system_uploadToken_create" {
				t.Fatalf("unexpected permission code %q", input.Code)
			}
			return nil
		},
		UploadTokenService: uploadTokenService,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/upload-tokens", strings.NewReader(`{"folder":"images","file_name":"demo.png","file_size":1024,"file_kind":"image"}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected upload token status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if uploadTokenService.input.Folder != "images" || uploadTokenService.input.FileName != "demo.png" || uploadTokenService.input.FileSize != 1024 || uploadTokenService.input.FileKind != "image" {
		t.Fatalf("upload token input mismatch: %#v", uploadTokenService.input)
	}
	body := decodeRouterBody(t, recorder)
	data := mustRouterData(t, body)
	if data["provider"] != "cos" {
		t.Fatalf("expected cos provider, got %#v", data["provider"])
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

func TestRealtimeRouteAcceptsPathScopedCookieTokenForBrowserWebSocket(t *testing.T) {
	var gotInput middleware.TokenInput
	router := newTestRouter(t, Dependencies{
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			gotInput = input
			return &middleware.AuthIdentity{UserID: 7, SessionID: 9, Platform: input.Platform}, nil
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

	client, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):]+realtimemodule.WSPath, http.Header{
		"Cookie": []string{middleware.DefaultAccessTokenCookie + "=cookie-access-token"},
	})
	if err != nil {
		t.Fatalf("dial realtime with cookie token: %v", err)
	}
	defer client.Close()

	var connected map[string]any
	if err := client.ReadJSON(&connected); err != nil {
		t.Fatalf("read connected: %v", err)
	}
	if connected["type"] != realtimemodule.TypeConnectedV1 {
		t.Fatalf("expected connected event, got %#v", connected)
	}
	if gotInput.AccessToken != "cookie-access-token" {
		t.Fatalf("expected cookie access token, got %q", gotInput.AccessToken)
	}
	if gotInput.Platform != "admin" {
		t.Fatalf("expected cookie websocket auth to default platform admin, got %q", gotInput.Platform)
	}
}

func TestRealtimeRouteAllowsConfiguredBrowserOrigin(t *testing.T) {
	router := newTestRouter(t, Dependencies{
		CORS: config.CORSConfig{
			AllowOrigins:     []string{"http://127.0.0.1:5173"},
			AllowMethods:     []string{"GET", "OPTIONS"},
			AllowHeaders:     []string{"Authorization", "platform", "device-id"},
			AllowCredentials: true,
		},
		Authenticator: func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
			return &middleware.AuthIdentity{UserID: 7, SessionID: 9, Platform: input.Platform}, nil
		},
		RealtimeHandler: realtimemodule.NewHandler(
			realtimemodule.NewService(25*time.Second),
			platformrealtime.NewUpgrader(platformrealtime.NewAllowedOriginChecker([]string{"http://127.0.0.1:5173"})),
			platformrealtime.NewManager(),
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		),
	})
	server := httptest.NewServer(router)
	defer server.Close()

	client, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):]+realtimemodule.WSPath, http.Header{
		"Cookie": []string{middleware.DefaultAccessTokenCookie + "=cookie-access-token"},
		"Origin": []string{"http://127.0.0.1:5173"},
	})
	if err != nil {
		t.Fatalf("dial realtime from configured origin: %v", err)
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
