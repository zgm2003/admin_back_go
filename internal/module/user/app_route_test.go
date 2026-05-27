package user

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/permission"

	"github.com/gin-gonic/gin"
)

type fakeAppUserService struct {
	initInput     InitInput
	profileUserID int64
	profileViewer int64
	updateInput   UpdateProfileInput
}

func (f *fakeAppUserService) Init(ctx context.Context, input InitInput) (*InitResponse, *apperror.Error) {
	f.initInput = input
	return &InitResponse{
		UserID:      input.UserID,
		Username:    "App User",
		Avatar:      "avatar.png",
		RoleName:    "管理员",
		Permissions: []permission.MenuItem{{Index: "1", Label: "系统"}},
		Router:      []permission.RouteItem{{Name: "admin", Path: "/system"}},
		ButtonCodes: []string{"user_add"},
	}, nil
}

func (f *fakeAppUserService) PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error) {
	return &PageInitResponse{}, nil
}

func (f *fakeAppUserService) Profile(ctx context.Context, userID int64, currentUserID int64) (*ProfileResponse, *apperror.Error) {
	f.profileUserID = userID
	f.profileViewer = currentUserID
	return &ProfileResponse{
		Profile: ProfileDetail{
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
		Dict: ProfileDict{
			AuthAddressTree: []AddressTreeNode{{ID: 3, Label: "武汉", Value: 3}},
			SexArr:          []SexOption{{Label: "男", Value: 1}},
		},
	}, nil
}

func (f *fakeAppUserService) UpdateProfile(ctx context.Context, input UpdateProfileInput) *apperror.Error {
	f.updateInput = input
	return nil
}

func (f *fakeAppUserService) UpdatePassword(ctx context.Context, input UpdatePasswordInput) *apperror.Error {
	return nil
}
func (f *fakeAppUserService) UpdateEmail(ctx context.Context, input UpdateEmailInput) *apperror.Error {
	return nil
}
func (f *fakeAppUserService) UpdatePhone(ctx context.Context, input UpdatePhoneInput) *apperror.Error {
	return nil
}
func (f *fakeAppUserService) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	return &ListResponse{}, nil
}
func (f *fakeAppUserService) Export(ctx context.Context, input ExportInput) (*ExportResponse, *apperror.Error) {
	return &ExportResponse{}, nil
}
func (f *fakeAppUserService) Update(ctx context.Context, id int64, input UpdateInput) *apperror.Error {
	return nil
}
func (f *fakeAppUserService) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	return nil
}
func (f *fakeAppUserService) Delete(ctx context.Context, ids []int64) *apperror.Error { return nil }
func (f *fakeAppUserService) BatchUpdateProfile(ctx context.Context, input BatchProfileUpdate) *apperror.Error {
	return nil
}

func TestUserModuleRegistersAppCurrentUserAndProfileRoutes(t *testing.T) {
	service := &fakeAppUserService{}
	router := newAppUserTestRouter(service, &middleware.AuthIdentity{UserID: 7, SessionID: 20, Platform: enum.PlatformApp})

	meData := requestAppUserData(t, router, http.MethodGet, "/api/app/v1/users/me", "")
	if service.initInput.UserID != 7 || service.initInput.Platform != enum.PlatformApp {
		t.Fatalf("unexpected init input: %#v", service.initInput)
	}
	if meData["id"] != float64(7) || meData["nickname"] != "App User" || meData["avatar"] != "avatar.png" {
		t.Fatalf("unexpected app users/me payload: %#v", meData)
	}
	for _, forbidden := range []string{"role_name", "permissions", "router", "buttonCodes", "quick_entry"} {
		if _, ok := meData[forbidden]; ok {
			t.Fatalf("app users/me must not include admin field %q: %#v", forbidden, meData)
		}
	}

	profileData := requestAppUserData(t, router, http.MethodGet, "/api/app/v1/profile", "")
	if service.profileUserID != 7 || service.profileViewer != 7 {
		t.Fatalf("unexpected profile input: user=%d viewer=%d", service.profileUserID, service.profileViewer)
	}
	profile := profileData["profile"].(map[string]any)
	if profile["nickname"] != "App User" || profile["email"] != "app@example.test" || profile["bio"] != "old bio" {
		t.Fatalf("unexpected app profile payload: %#v", profileData)
	}
	for _, forbidden := range []string{"role_id", "role_name", "is_self"} {
		if _, ok := profile[forbidden]; ok {
			t.Fatalf("app profile must not include admin field %q: %#v", forbidden, profile)
		}
	}
	dict := profileData["dict"].(map[string]any)
	if _, ok := dict["auth_address_tree"]; !ok {
		t.Fatalf("missing address tree in app profile dict: %#v", dict)
	}

	birthday := "2026-05-25"
	updateData := requestAppUserData(t, router, http.MethodPut, "/api/app/v1/profile", `{"nickname":"App User 2","avatar":"avatar2.png","sex":2,"birthday":"`+birthday+`","address_id":8,"detail_address":"湖北武汉光谷","bio":"new bio"}`)
	if service.updateInput.UserID != 7 || service.updateInput.Username != "App User 2" || service.updateInput.Avatar != "avatar2.png" || service.updateInput.Sex != 2 || service.updateInput.AddressID != 8 || service.updateInput.DetailAddress != "湖北武汉光谷" || service.updateInput.Bio != "new bio" || service.updateInput.Birthday == nil || *service.updateInput.Birthday != birthday {
		t.Fatalf("unexpected update input: %#v", service.updateInput)
	}
	userData := updateData["user"].(map[string]any)
	if userData["id"] != float64(7) || userData["nickname"] != "App User" {
		t.Fatalf("unexpected update response: %#v", updateData)
	}
}

func TestUserModuleAppRoutesRejectWrongPlatformScope(t *testing.T) {
	router := newAppUserTestRouter(&fakeAppUserService{}, &middleware.AuthIdentity{UserID: 7, SessionID: 20, Platform: enum.PlatformAdmin})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/app/v1/users/me", nil))

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected wrong app platform status 401, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func newAppUserTestRouter(service HTTPService, identity *middleware.AuthIdentity) *gin.Engine {
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

func requestAppUserData(t *testing.T, router *gin.Engine, method string, path string, body string) map[string]any {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("%s %s expected status 200, got %d body=%s", method, path, recorder.Code, recorder.Body.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	data, ok := decoded["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object data, got %#v", decoded)
	}
	if reflect.ValueOf(data).IsNil() {
		t.Fatalf("expected non-nil data")
	}
	return data
}
