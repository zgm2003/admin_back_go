package canvas

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/permission"
	"admin_back_go/internal/module/profile"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"

	"github.com/gin-gonic/gin"
)

type fakeCanvasProfileService struct {
	initInput profile.InitInput
}

func (f *fakeCanvasProfileService) Init(ctx context.Context, input profile.InitInput) (*profile.InitResponse, *apperror.Error) {
	f.initInput = input
	return &profile.InitResponse{
		UserID: input.UserID, Username: "Canvas User", Avatar: "canvas.png", RoleName: "canvas",
		Permissions: []permission.MenuItem{},
		Router:      []permission.RouteItem{{Path: "/canvas", Meta: map[string]string{"code": "canvas_page"}}},
		ButtonCodes: []string{"canvas_access"},
	}, nil
}

func (f *fakeCanvasProfileService) Profile(ctx context.Context, userID int64, currentUserID int64) (*profile.ProfileResponse, *apperror.Error) {
	return nil, nil
}

func (f *fakeCanvasProfileService) UpdateProfile(ctx context.Context, input profile.UpdateProfileInput) *apperror.Error {
	return nil
}

func TestCanvasProfileTransportCurrentUserUsesCanvasPlatform(t *testing.T) {
	service := &fakeCanvasProfileService{}
	router := newCanvasProfileTestRouter(service, &middleware.AuthIdentity{UserID: 8, Platform: enum.PlatformCanvas})

	data := requestCanvasProfileData(t, router, http.MethodGet, "/api/canvas/v1/users/me")
	if service.initInput.UserID != 8 || service.initInput.Platform != enum.PlatformCanvas {
		t.Fatalf("unexpected init input: %#v", service.initInput)
	}
	if data["user_id"] != float64(8) || data["username"] != "Canvas User" || data["avatar"] != "canvas.png" || data["role_name"] != "canvas" {
		t.Fatalf("unexpected canvas users/me payload: %#v", data)
	}
	for _, forbidden := range []string{"id", "nickname", "display_name", "avatar_url", "permissionCodes", "permission_codes", "button_codes"} {
		if _, ok := data[forbidden]; ok {
			t.Fatalf("canvas users/me payload must not expose fallback/alias field %q: %#v", forbidden, data)
		}
	}
	if _, ok := data["permissions"].([]any); !ok {
		t.Fatalf("expected permissions in canvas users/me payload, got %#v", data["permissions"])
	}
	if !routeSliceEqual(data["router"], []string{"/canvas"}) {
		t.Fatalf("expected router in canvas users/me payload, got %#v", data["router"])
	}
	if !stringSliceEqual(data["buttonCodes"], []string{"canvas_access"}) {
		t.Fatalf("expected buttonCodes in canvas users/me payload, got %#v", data["buttonCodes"])
	}
}

func TestCanvasProfileTransportDoesNotMountProfileRoute(t *testing.T) {
	router := newCanvasProfileTestRouter(&fakeCanvasProfileService{}, &middleware.AuthIdentity{UserID: 8, Platform: enum.PlatformCanvas})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/canvas/v1/profile", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("canvas profile route must not be mounted, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestCanvasProfileTransportRejectsWrongPlatformScope(t *testing.T) {
	router := newCanvasProfileTestRouter(&fakeCanvasProfileService{}, &middleware.AuthIdentity{UserID: 8, Platform: enum.PlatformAdmin})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/canvas/v1/users/me", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected wrong platform status 401, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func newCanvasProfileTestRouter(service profile.AppService, identity *middleware.AuthIdentity) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	if identity != nil {
		router.Use(func(c *gin.Context) {
			c.Set(middleware.ContextAuthIdentity, identity)
			c.Next()
		})
	}
	RegisterRoutes(router, Dependencies{Service: service})
	return router
}

func requestCanvasProfileData(t *testing.T, router *gin.Engine, method string, path string) map[string]any {
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

func stringSliceEqual(value any, want []string) bool {
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

func routeSliceEqual(value any, wantPaths []string) bool {
	got, ok := value.([]any)
	if !ok || len(got) != len(wantPaths) {
		return false
	}
	for i := range wantPaths {
		item, ok := got[i].(map[string]any)
		if !ok || item["path"] != wantPaths[i] {
			return false
		}
	}
	return true
}
