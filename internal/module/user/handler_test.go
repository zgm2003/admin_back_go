package user

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/permission"

	"github.com/gin-gonic/gin"
)

type fakeInitService struct {
	input  InitInput
	result *InitResponse
	err    *apperror.Error
}

func (f *fakeInitService) Init(ctx context.Context, input InitInput) (*InitResponse, *apperror.Error) {
	f.input = input
	return f.result, f.err
}

func TestHandlerInitRequiresAuthIdentity(t *testing.T) {
	router := newUserTestRouter(&fakeInitService{}, nil)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/Users/init", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeUserBody(t, recorder)
	if body["msg"] != "Token无效或已过期" {
		t.Fatalf("unexpected message: %#v", body["msg"])
	}
}

func TestHandlerInitUsesAuthIdentityAndReturnsData(t *testing.T) {
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
	request := httptest.NewRequest(http.MethodPost, "/api/Users/init", nil)
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
	if _, ok := data["buttonCodes"]; !ok {
		t.Fatalf("missing buttonCodes in response: %#v", data)
	}
}

func newUserTestRouter(service InitService, identity *middleware.AuthIdentity) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	if identity != nil {
		router.Use(func(c *gin.Context) {
			c.Set(middleware.ContextAuthIdentity, identity)
			c.Next()
		})
	}
	RegisterRoutes(router, service)
	return router
}

func decodeUserBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	return body
}
