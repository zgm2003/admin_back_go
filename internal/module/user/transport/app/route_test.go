package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/permission"
	usermodule "admin_back_go/internal/module/user"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"

	"github.com/gin-gonic/gin"
)

type fakeAppUserService struct {
	initInput usermodule.InitInput
}

func (f *fakeAppUserService) Init(ctx context.Context, input usermodule.InitInput) (*usermodule.InitResponse, *apperror.Error) {
	f.initInput = input
	return &usermodule.InitResponse{
		UserID:   input.UserID,
		Username: "App User",
		Avatar:   "app.png",
		RoleName: "app_user",
		Permissions: []permission.MenuItem{{
			Index: "app_home",
			Label: "App Home",
			Path:  "/app/home",
		}},
		Router: []permission.RouteItem{{
			Name:    "AppHome",
			Path:    "/app/home",
			ViewKey: "app/home",
			Meta:    map[string]string{"type": "page"},
		}},
		ButtonCodes: []string{"app_access"},
	}, nil
}

func TestAppUserTransportCurrentUserUsesAppPlatformAndContractFields(t *testing.T) {
	service := &fakeAppUserService{}
	router := newAppUserTestRouter(service, &middleware.AuthIdentity{UserID: 7, Platform: enum.PlatformApp})

	data := requestAppUserData(t, router, http.MethodGet, "/api/app/v1/users/me")
	if service.initInput.UserID != 7 || service.initInput.Platform != enum.PlatformApp {
		t.Fatalf("unexpected init input: %#v", service.initInput)
	}

	assertAppUserField(t, data, "user_id")
	assertAppUserField(t, data, "username")
	assertAppUserField(t, data, "avatar")
	assertAppUserField(t, data, "role_name")
	assertAppUserField(t, data, "permissions")
	assertAppUserField(t, data, "router")
	assertAppUserField(t, data, "buttonCodes")

	if data["user_id"] != float64(7) || data["username"] != "App User" || data["avatar"] != "app.png" || data["role_name"] != "app_user" {
		t.Fatalf("unexpected app users/me payload: %#v", data)
	}
	if _, ok := data["permissions"].([]any); !ok {
		t.Fatalf("expected permissions array in app users/me payload, got %#v", data["permissions"])
	}
	if !appRouteSliceContains(data["router"], "/app/home") {
		t.Fatalf("expected app router path in users/me payload, got %#v", data["router"])
	}
	if !appStringSliceEqual(data["buttonCodes"], []string{"app_access"}) {
		t.Fatalf("expected buttonCodes in users/me payload, got %#v", data["buttonCodes"])
	}

	for _, forbidden := range []string{"id", "nickname", "display_name", "avatar_url", "permissionCodes", "permission_codes", "button_codes", "quick_entry", "quickEntry"} {
		if _, ok := data[forbidden]; ok {
			t.Fatalf("app users/me payload must not expose alias/admin-only field %q: %#v", forbidden, data)
		}
	}
}

func TestAppUserTransportRejectsWrongPlatformScope(t *testing.T) {
	router := newAppUserTestRouter(&fakeAppUserService{}, &middleware.AuthIdentity{UserID: 7, Platform: enum.PlatformAdmin})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/app/v1/users/me", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected wrong platform status 401, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func newAppUserTestRouter(service usermodule.InitService, identity *middleware.AuthIdentity) *gin.Engine {
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

func requestAppUserData(t *testing.T, router *gin.Engine, method string, path string) map[string]any {
	t.Helper()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("%s %s expected 200, got %d body=%s", method, path, recorder.Code, recorder.Body.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	data, ok := decoded["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing object data: %#v", decoded)
	}
	return data
}

func assertAppUserField(t *testing.T, data map[string]any, field string) {
	t.Helper()
	if _, ok := data[field]; !ok {
		t.Fatalf("missing required field %q in app users/me payload: %#v", field, data)
	}
}

func appStringSliceEqual(value any, want []string) bool {
	got, ok := value.([]any)
	if !ok || len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func appRouteSliceContains(value any, wantPath string) bool {
	got, ok := value.([]any)
	if !ok {
		return false
	}
	for _, raw := range got {
		item, ok := raw.(map[string]any)
		if ok && item["path"] == wantPath {
			return true
		}
	}
	return false
}
