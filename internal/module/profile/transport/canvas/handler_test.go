package canvas

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/profile"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"

	"github.com/gin-gonic/gin"
)

type fakeCanvasProfileService struct {
	profileResult *profile.ProfileResponse
	updateInput   profile.UpdateProfileInput
}

func (f *fakeCanvasProfileService) Profile(ctx context.Context, userID int64, currentUserID int64) (*profile.ProfileResponse, *apperror.Error) {
	if userID != 8 || currentUserID != 8 {
		return nil, apperror.BadRequest("unexpected user id")
	}
	return f.profileResult, nil
}

func (f *fakeCanvasProfileService) UpdateProfile(ctx context.Context, input profile.UpdateProfileInput) *apperror.Error {
	f.updateInput = input
	return nil
}

func TestCanvasProfileTransportReadsCurrentProfile(t *testing.T) {
	service := &fakeCanvasProfileService{profileResult: canvasProfileResult()}
	router := newCanvasProfileTestRouter(service, &middleware.AuthIdentity{UserID: 8, Platform: enum.PlatformCanvas})

	data := requestCanvasProfileData(t, router, http.MethodGet, "/api/canvas/v1/profile", "")
	profilePayload := data["profile"].(map[string]any)
	if profilePayload["username"] != "canvas-user" {
		t.Fatalf("unexpected profile payload: %#v", data)
	}
}

func TestCanvasProfileTransportUpdatesCurrentProfile(t *testing.T) {
	service := &fakeCanvasProfileService{profileResult: canvasProfileResult()}
	router := newCanvasProfileTestRouter(service, &middleware.AuthIdentity{UserID: 8, Platform: enum.PlatformCanvas})

	_ = requestCanvasProfileData(t, router, http.MethodPut, "/api/canvas/v1/profile", `{"username":"canvas-user-2","avatar":"avatar2.png","sex":2,"birthday":"2026-06-01","address_id":9,"detail_address":"湖北武汉光谷","bio":"new bio"}`)
	if service.updateInput.UserID != 8 || service.updateInput.Username != "canvas-user-2" || service.updateInput.AddressID != 9 {
		t.Fatalf("unexpected update input: %#v", service.updateInput)
	}
}

func TestCanvasProfileTransportRejectsWrongPlatformScope(t *testing.T) {
	router := newCanvasProfileTestRouter(&fakeCanvasProfileService{profileResult: canvasProfileResult()}, &middleware.AuthIdentity{UserID: 8, Platform: enum.PlatformAdmin})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/canvas/v1/profile", nil))
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
	RegisterRoutes(router, service)
	return router
}

func requestCanvasProfileData(t *testing.T, router *gin.Engine, method string, path string, body string) map[string]any {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(recorder, request)
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

func canvasProfileResult() *profile.ProfileResponse {
	return &profile.ProfileResponse{
		Profile: profile.ProfileDetail{
			UserID:        8,
			Username:      "canvas-user",
			Email:         "canvas@example.test",
			Phone:         "15671628271",
			Avatar:        "avatar.png",
			RoleID:        99,
			RoleName:      "画布用户",
			AddressID:     3,
			DetailAddress: "湖北武汉",
			Sex:           1,
			Birthday:      "2026-05-24",
			Bio:           "old bio",
			IsSelf:        1,
			HasPassword:   true,
		},
		Dict: profile.ProfileDict{
			AuthAddressTree: []profile.AddressTreeNode{{ID: 3, Label: "武汉", Value: 3}},
			SexArr:          []profile.SexOption{{Label: "男", Value: 1}},
		},
	}
}
