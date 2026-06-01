package canvas

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

type fakeCanvasUserService struct {
	initInput usermodule.InitInput
}

func (f *fakeCanvasUserService) Init(ctx context.Context, input usermodule.InitInput) (*usermodule.InitResponse, *apperror.Error) {
	f.initInput = input
	return &usermodule.InitResponse{
		UserID:   input.UserID,
		Username: "Canvas User",
		Avatar:   "canvas.png",
		RoleName: "canvas_user",
		Permissions: []permission.MenuItem{{
			Index: "canvas_home",
			Label: "Canvas Home",
			Path:  "/canvas/home",
		}},
		Router: []permission.RouteItem{{
			Name:    "CanvasHome",
			Path:    "/canvas/home",
			ViewKey: "canvas/home",
			Meta:    map[string]string{"type": "page"},
		}},
		ButtonCodes: []string{"canvas_access"},
	}, nil
}

func TestCanvasUserTransportCurrentUserUsesCanvasPlatformAndContractFields(t *testing.T) {
	service := &fakeCanvasUserService{}
	router := newCanvasUserTestRouter(service, &middleware.AuthIdentity{UserID: 8, Platform: enum.PlatformCanvas})

	data := requestCanvasUserData(t, router, http.MethodGet, "/api/canvas/v1/users/me")
	if service.initInput.UserID != 8 || service.initInput.Platform != enum.PlatformCanvas {
		t.Fatalf("unexpected init input: %#v", service.initInput)
	}

	assertCanvasUserField(t, data, "user_id")
	assertCanvasUserField(t, data, "username")
	assertCanvasUserField(t, data, "avatar")
	assertCanvasUserField(t, data, "role_name")
	assertCanvasUserField(t, data, "permissions")
	assertCanvasUserField(t, data, "router")
	assertCanvasUserField(t, data, "buttonCodes")

	if data["user_id"] != float64(8) || data["username"] != "Canvas User" || data["avatar"] != "canvas.png" || data["role_name"] != "canvas_user" {
		t.Fatalf("unexpected canvas users/me payload: %#v", data)
	}
	if _, ok := data["permissions"].([]any); !ok {
		t.Fatalf("expected permissions array in canvas users/me payload, got %#v", data["permissions"])
	}
	if !canvasRouteSliceContains(data["router"], "/canvas/home") {
		t.Fatalf("expected canvas PAGE route path in users/me payload, got %#v", data["router"])
	}
	if !canvasStringSliceEqual(data["buttonCodes"], []string{"canvas_access"}) {
		t.Fatalf("expected buttonCodes in users/me payload, got %#v", data["buttonCodes"])
	}

	for _, forbidden := range []string{"id", "nickname", "display_name", "avatar_url", "permissionCodes", "permission_codes", "button_codes", "quick_entry", "quickEntry"} {
		if _, ok := data[forbidden]; ok {
			t.Fatalf("canvas users/me payload must not expose alias/admin-only field %q: %#v", forbidden, data)
		}
	}
}

func TestCanvasUserTransportRejectsWrongPlatformScope(t *testing.T) {
	router := newCanvasUserTestRouter(&fakeCanvasUserService{}, &middleware.AuthIdentity{UserID: 8, Platform: enum.PlatformAdmin})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/canvas/v1/users/me", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected wrong platform status 401, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func newCanvasUserTestRouter(service usermodule.InitService, identity *middleware.AuthIdentity) *gin.Engine {
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

func requestCanvasUserData(t *testing.T, router *gin.Engine, method string, path string) map[string]any {
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

func assertCanvasUserField(t *testing.T, data map[string]any, field string) {
	t.Helper()
	if _, ok := data[field]; !ok {
		t.Fatalf("missing required field %q in canvas users/me payload: %#v", field, data)
	}
}

func canvasStringSliceEqual(value any, want []string) bool {
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

func canvasRouteSliceContains(value any, wantPath string) bool {
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
