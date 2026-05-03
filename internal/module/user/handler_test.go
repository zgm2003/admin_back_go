package user

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/permission"

	"github.com/gin-gonic/gin"
)

type fakeInitService struct {
	input  InitInput
	inputs []InitInput
	result *InitResponse
	err    *apperror.Error
}

func (f *fakeInitService) Init(ctx context.Context, input InitInput) (*InitResponse, *apperror.Error) {
	f.input = input
	f.inputs = append(f.inputs, input)
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

func TestHandlerRESTInitAndMeUseAuthIdentityAndMatchLegacyInitData(t *testing.T) {
	service := &fakeInitService{result: &InitResponse{
		UserID:   1,
		Username: "admin",
		Avatar:   "avatar.png",
		RoleName: "管理员",
		Permissions: []permission.MenuItem{{
			Index: "1",
			Label: "系统",
			Children: []permission.MenuItem{{
				Index: "2",
				Label: "用户",
				Path:  "/system/user",
			}},
		}},
		Router: []permission.RouteItem{{
			Name:    "menu_2",
			Path:    "/system/user",
			ViewKey: "system/user/index",
			Meta:    map[string]string{"menuId": "2"},
		}},
		ButtonCodes: []string{"user_add", "user_edit"},
		QuickEntry:  []QuickEntry{{ID: 3, PermissionID: 2, Sort: 1}},
	}}
	router := newUserTestRouter(service, &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"})

	legacyData := requestUserData(t, router, http.MethodPost, "/api/Users/init")
	restInitData := requestUserData(t, router, http.MethodGet, "/api/admin/v1/users/init")
	meData := requestUserData(t, router, http.MethodGet, "/api/admin/v1/users/me")

	if len(service.inputs) != 3 {
		t.Fatalf("expected service called three times, got %d inputs=%#v", len(service.inputs), service.inputs)
	}
	for _, input := range service.inputs {
		if input.UserID != 1 || input.Platform != "admin" {
			t.Fatalf("service input mismatch: %#v", input)
		}
	}
	if !reflect.DeepEqual(legacyData, restInitData) {
		t.Fatalf("REST init data payload mismatch with legacy init:\nlegacy=%#v\nrestInit=%#v", legacyData, restInitData)
	}
	if !reflect.DeepEqual(legacyData, meData) {
		t.Fatalf("REST me data payload mismatch with legacy init:\nlegacy=%#v\nme=%#v", legacyData, meData)
	}
	for _, key := range []string{"permissions", "router", "buttonCodes", "quick_entry"} {
		assertPayloadFieldEqual(t, key, legacyData, restInitData)
		assertPayloadFieldEqual(t, key, legacyData, meData)
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

func requestUserData(t *testing.T, router *gin.Engine, method, path string) map[string]any {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("%s %s expected status 200, got %d body=%s", method, path, recorder.Code, recorder.Body.String())
	}
	body := decodeUserBody(t, recorder)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing object data in response: %#v", body)
	}
	return data
}

func assertPayloadFieldEqual(t *testing.T, key string, legacyData, meData map[string]any) {
	t.Helper()

	legacyValue, ok := legacyData[key]
	if !ok {
		t.Fatalf("missing %s in legacy data: %#v", key, legacyData)
	}
	meValue, ok := meData[key]
	if !ok {
		t.Fatalf("missing %s in REST data: %#v", key, meData)
	}
	if !reflect.DeepEqual(legacyValue, meValue) {
		t.Fatalf("%s mismatch: legacy=%#v me=%#v", key, legacyValue, meValue)
	}
	items, ok := legacyValue.([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected non-empty %s payload, got %#v", key, legacyValue)
	}
}

func decodeUserBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	return body
}

