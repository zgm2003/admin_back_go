package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/profile"

	"github.com/gin-gonic/gin"
)

type fakeAppProfileService struct {
	initInput     profile.InitInput
	profileUserID int64
	profileViewer int64
	updateInput   profile.UpdateProfileInput
}

func (f *fakeAppProfileService) Init(ctx context.Context, input profile.InitInput) (*profile.InitResponse, *apperror.Error) {
	f.initInput = input
	return &profile.InitResponse{UserID: input.UserID, Username: "App User", Avatar: "avatar.png"}, nil
}

func (f *fakeAppProfileService) Profile(ctx context.Context, userID int64, currentUserID int64) (*profile.ProfileResponse, *apperror.Error) {
	f.profileUserID = userID
	f.profileViewer = currentUserID
	return &profile.ProfileResponse{
		Profile: profile.ProfileDetail{
			UserID:        userID,
			Username:      "App User",
			Email:         "app@example.test",
			Phone:         "15671628271",
			Avatar:        "avatar.png",
			RoleID:        99,
			RoleName:      "管理员",
			AddressID:     3,
			DetailAddress: "湖北武汉",
			Sex:           1,
			Birthday:      "2026-05-24",
			Bio:           "old bio",
			HasPassword:   true,
		},
		Dict: profile.ProfileDict{
			AuthAddressTree: []profile.AddressTreeNode{{ID: 3, Label: "武汉", Value: 3}},
			SexArr:          []profile.SexOption{{Label: "男", Value: 1}},
		},
	}, nil
}

func (f *fakeAppProfileService) UpdateProfile(ctx context.Context, input profile.UpdateProfileInput) *apperror.Error {
	f.updateInput = input
	return nil
}

func TestAppProfileTransportPreservesAppCurrentUserAndProfileRoutes(t *testing.T) {
	service := &fakeAppProfileService{}
	router := newAppProfileTestRouter(service, &middleware.AuthIdentity{UserID: 7, Platform: enum.PlatformApp})

	meData := requestAppProfileData(t, router, http.MethodGet, "/api/app/v1/users/me", "")
	if service.initInput.UserID != 7 || service.initInput.Platform != enum.PlatformApp {
		t.Fatalf("unexpected init input: %#v", service.initInput)
	}
	if meData["id"] != float64(7) || meData["nickname"] != "App User" {
		t.Fatalf("unexpected users/me payload: %#v", meData)
	}

	profileData := requestAppProfileData(t, router, http.MethodGet, "/api/app/v1/profile", "")
	if service.profileUserID != 7 || service.profileViewer != 7 {
		t.Fatalf("unexpected profile input: user=%d viewer=%d", service.profileUserID, service.profileViewer)
	}
	profilePayload := profileData["profile"].(map[string]any)
	if profilePayload["nickname"] != "App User" || profilePayload["bio"] != "old bio" {
		t.Fatalf("unexpected profile payload: %#v", profileData)
	}
	for _, forbidden := range []string{"role_id", "role_name", "is_self"} {
		if _, ok := profilePayload[forbidden]; ok {
			t.Fatalf("app profile must not include admin field %q: %#v", forbidden, profilePayload)
		}
	}

	_ = requestAppProfileData(t, router, http.MethodPut, "/api/app/v1/profile", `{"nickname":"App User 2","avatar":"avatar2.png","sex":2,"birthday":"2026-05-25","address_id":8,"detail_address":"湖北武汉光谷","bio":"new bio"}`)
	if service.updateInput.UserID != 7 || service.updateInput.Username != "App User 2" || service.updateInput.AddressID != 8 {
		t.Fatalf("unexpected update input: %#v", service.updateInput)
	}
}

func TestAppProfileTransportRejectsWrongPlatformScope(t *testing.T) {
	router := newAppProfileTestRouter(&fakeAppProfileService{}, &middleware.AuthIdentity{UserID: 7, Platform: enum.PlatformAdmin})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/app/v1/users/me", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected wrong platform status 401, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func newAppProfileTestRouter(service profile.AppService, identity *middleware.AuthIdentity) *gin.Engine {
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

func requestAppProfileData(t *testing.T, router *gin.Engine, method string, path string, body string) map[string]any {
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
