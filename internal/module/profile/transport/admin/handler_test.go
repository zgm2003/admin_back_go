package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"admin_back_go/internal/apperror"
	projecti18n "admin_back_go/internal/i18n"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/profile"
	"admin_back_go/internal/module/userquickentry"

	"github.com/gin-gonic/gin"
)

type fakeProfileService struct {
	profileUserID int64
	profileViewer int64
	updateInput   profile.UpdateProfileInput
	passwordInput profile.UpdatePasswordInput
	emailInput    profile.UpdateEmailInput
	phoneInput    profile.UpdatePhoneInput
}

func (f *fakeProfileService) Profile(ctx context.Context, userID int64, currentUserID int64) (*profile.ProfileResponse, *apperror.Error) {
	f.profileUserID = userID
	f.profileViewer = currentUserID
	return &profile.ProfileResponse{Profile: profile.ProfileDetail{UserID: userID, Username: "admin"}}, nil
}
func (f *fakeProfileService) UpdateProfile(ctx context.Context, input profile.UpdateProfileInput) *apperror.Error {
	f.updateInput = input
	return nil
}
func (f *fakeProfileService) UpdatePassword(ctx context.Context, input profile.UpdatePasswordInput) *apperror.Error {
	f.passwordInput = input
	return nil
}
func (f *fakeProfileService) UpdateEmail(ctx context.Context, input profile.UpdateEmailInput) *apperror.Error {
	f.emailInput = input
	return nil
}
func (f *fakeProfileService) UpdatePhone(ctx context.Context, input profile.UpdatePhoneInput) *apperror.Error {
	f.phoneInput = input
	return nil
}

type fakeQuickEntryService struct {
	userID int64
	input  userquickentry.SaveInput
}

func (f *fakeQuickEntryService) Save(ctx context.Context, userID int64, input userquickentry.SaveInput) (*userquickentry.SaveResponse, *apperror.Error) {
	f.userID = userID
	f.input = input
	return &userquickentry.SaveResponse{QuickEntry: []userquickentry.QuickEntry{{ID: 7, PermissionID: 3, Sort: 1}}}, nil
}

func TestAdminProfileTransportPreservesCurrentUserProfileAndQuickEntryRoutes(t *testing.T) {
	profileService := &fakeProfileService{}
	quickEntryService := &fakeQuickEntryService{}
	router := newAdminProfileTestRouter(profileService, quickEntryService, &middleware.AuthIdentity{UserID: 44, Platform: "admin"})

	data := requestAdminProfileData(t, router, http.MethodGet, "/api/admin/v1/profile", "")
	if profileService.profileUserID != 44 || profileService.profileViewer != 44 {
		t.Fatalf("unexpected current profile input: user=%d viewer=%d", profileService.profileUserID, profileService.profileViewer)
	}
	if _, ok := data["profile"]; !ok {
		t.Fatalf("missing profile payload: %#v", data)
	}

	_ = requestAdminProfileData(t, router, http.MethodPut, "/api/admin/v1/profile", `{"username":"alice","avatar":"a.png","sex":1,"birthday":"2026-05-05","address_id":3,"detail_address":"玄武区","bio":"bio"}`)
	if profileService.updateInput.UserID != 44 || profileService.updateInput.Username != "alice" || profileService.updateInput.AddressID != 3 {
		t.Fatalf("unexpected update input: %#v", profileService.updateInput)
	}

	_ = requestAdminProfileData(t, router, http.MethodPut, "/api/admin/v1/profile/security/password", `{"verify_type":"code","account":"alice@example.com","code":"123456","new_password":"secret1","confirm_password":"secret1"}`)
	if profileService.passwordInput.UserID != 44 || profileService.passwordInput.Account != "alice@example.com" {
		t.Fatalf("unexpected password input: %#v", profileService.passwordInput)
	}

	_ = requestAdminProfileData(t, router, http.MethodPut, "/api/admin/v1/profile/security/email", `{"email":"new@example.com","code":"123456"}`)
	if profileService.emailInput.UserID != 44 || profileService.emailInput.Email != "new@example.com" {
		t.Fatalf("unexpected email input: %#v", profileService.emailInput)
	}

	_ = requestAdminProfileData(t, router, http.MethodPut, "/api/admin/v1/profile/security/phone", `{"phone":"15671628271","code":"123456"}`)
	if profileService.phoneInput.UserID != 44 || profileService.phoneInput.Phone != "15671628271" {
		t.Fatalf("unexpected phone input: %#v", profileService.phoneInput)
	}

	_ = requestAdminProfileData(t, router, http.MethodPut, "/api/admin/v1/users/me/quick-entries", `{"permission_ids":[3,1,3]}`)
	if quickEntryService.userID != 44 || !reflect.DeepEqual(quickEntryService.input.PermissionIDs, []int64{3, 1, 3}) {
		t.Fatalf("unexpected quick-entry input: user=%d input=%#v", quickEntryService.userID, quickEntryService.input)
	}
}

func TestAdminProfileTransportLocalizesInvalidQuickEntryRequest(t *testing.T) {
	router := newAdminProfileLocalizedTestRouter(&fakeProfileService{}, &fakeQuickEntryService{}, &middleware.AuthIdentity{UserID: 44, Platform: "admin"})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/v1/users/me/quick-entries", bytes.NewBufferString(`{`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept-Language", "en-US")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if decoded["msg"] != "Invalid quick entry request" {
		t.Fatalf("expected localized quick-entry message, got %#v", decoded["msg"])
	}
}

func newAdminProfileTestRouter(service profile.HTTPService, quickEntryService profile.QuickEntryService, identity *middleware.AuthIdentity) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	if identity != nil {
		router.Use(func(c *gin.Context) {
			c.Set(middleware.ContextAuthIdentity, identity)
			c.Next()
		})
	}
	RegisterRoutes(router, service, quickEntryService)
	return router
}

func newAdminProfileLocalizedTestRouter(service profile.HTTPService, quickEntryService profile.QuickEntryService, identity *middleware.AuthIdentity) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(projecti18n.Localize())
	if identity != nil {
		router.Use(func(c *gin.Context) {
			c.Set(middleware.ContextAuthIdentity, identity)
			c.Next()
		})
	}
	RegisterRoutes(router, service, quickEntryService)
	return router
}

func requestAdminProfileData(t *testing.T, router *gin.Engine, method string, path string, body string) map[string]any {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
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
