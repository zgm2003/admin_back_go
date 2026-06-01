package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"admin_back_go/internal/middleware"
	usermodule "admin_back_go/internal/module/user"
	"admin_back_go/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

type fakeUserService struct {
	input          usermodule.InitInput
	pageInitCalled bool
	profileUserID  int64
	profileViewer  int64
	listQuery      usermodule.ListQuery
	updateID       int64
	updateInput    usermodule.UpdateInput
	exportInput    usermodule.ExportInput
	deleteIDs      []int64
	batchInput     usermodule.BatchProfileUpdate
}

func (f *fakeUserService) Init(ctx context.Context, input usermodule.InitInput) (*usermodule.InitResponse, *apperror.Error) {
	f.input = input
	return &usermodule.InitResponse{UserID: input.UserID, Username: "admin"}, nil
}
func (f *fakeUserService) PageInit(ctx context.Context) (*usermodule.PageInitResponse, *apperror.Error) {
	f.pageInitCalled = true
	return &usermodule.PageInitResponse{}, nil
}
func (f *fakeUserService) Profile(ctx context.Context, userID int64, currentUserID int64) (*usermodule.ProfileResponse, *apperror.Error) {
	f.profileUserID = userID
	f.profileViewer = currentUserID
	return &usermodule.ProfileResponse{Profile: usermodule.ProfileDetail{UserID: userID}}, nil
}
func (f *fakeUserService) UpdateProfile(ctx context.Context, input usermodule.UpdateProfileInput) *apperror.Error {
	return nil
}
func (f *fakeUserService) UpdatePassword(ctx context.Context, input usermodule.UpdatePasswordInput) *apperror.Error {
	return nil
}
func (f *fakeUserService) UpdateEmail(ctx context.Context, input usermodule.UpdateEmailInput) *apperror.Error {
	return nil
}
func (f *fakeUserService) UpdatePhone(ctx context.Context, input usermodule.UpdatePhoneInput) *apperror.Error {
	return nil
}
func (f *fakeUserService) List(ctx context.Context, query usermodule.ListQuery) (*usermodule.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &usermodule.ListResponse{Page: usermodule.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize}}, nil
}
func (f *fakeUserService) Export(ctx context.Context, input usermodule.ExportInput) (*usermodule.ExportResponse, *apperror.Error) {
	f.exportInput = input
	return &usermodule.ExportResponse{ID: 1}, nil
}
func (f *fakeUserService) Update(ctx context.Context, id int64, input usermodule.UpdateInput) *apperror.Error {
	f.updateID = id
	f.updateInput = input
	return nil
}
func (f *fakeUserService) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	return nil
}
func (f *fakeUserService) Delete(ctx context.Context, ids []int64) *apperror.Error {
	f.deleteIDs = ids
	return nil
}
func (f *fakeUserService) BatchUpdateProfile(ctx context.Context, input usermodule.BatchProfileUpdate) *apperror.Error {
	f.batchInput = input
	return nil
}

func TestAdminUserTransportPreservesAdminUserRoutes(t *testing.T) {
	service := &fakeUserService{}
	router := newAdminUserTestRouter(service, &middleware.AuthIdentity{UserID: 7, Platform: "admin"})

	data := requestAdminUserData(t, router, http.MethodGet, "/api/admin/v1/users/me", "")
	if service.input.UserID != 7 || service.input.Platform != "admin" || data["username"] != "admin" {
		t.Fatalf("unexpected current-user input/data: input=%#v data=%#v", service.input, data)
	}
	for _, forbidden := range []string{"quick_entry", "quickEntry", "id", "nickname", "display_name", "avatar_url", "permissionCodes", "permission_codes", "button_codes"} {
		if _, ok := data[forbidden]; ok {
			t.Fatalf("admin users/me payload must not expose %q: %#v", forbidden, data)
		}
	}

	_ = requestAdminUserData(t, router, http.MethodGet, "/api/admin/v1/users/page-init", "")
	if !service.pageInitCalled {
		t.Fatalf("expected users page-init to call dedicated service method")
	}

	_ = requestAdminUserData(t, router, http.MethodGet, "/api/admin/v1/users/9/profile", "")
	if service.profileUserID != 9 || service.profileViewer != 7 {
		t.Fatalf("unexpected target profile input: user=%d viewer=%d", service.profileUserID, service.profileViewer)
	}

	_ = requestAdminUserData(t, router, http.MethodGet, "/api/admin/v1/users?current_page=1&page_size=20&address_id=3,4", "")
	if !reflect.DeepEqual(service.listQuery.AddressIDs, []int64{3, 4}) {
		t.Fatalf("unexpected address ids: %#v", service.listQuery.AddressIDs)
	}

	_ = requestAdminUserData(t, router, http.MethodPost, "/api/admin/v1/users/export", `{"ids":[3,2]}`)
	if service.exportInput.UserID != 7 || !reflect.DeepEqual(service.exportInput.IDs, []int64{3, 2}) {
		t.Fatalf("unexpected export input: %#v", service.exportInput)
	}
}

func TestAdminUserTransportRejectsLegacyAddressAlias(t *testing.T) {
	service := &fakeUserService{}
	router := newAdminUserTestRouter(service, &middleware.AuthIdentity{UserID: 7, Platform: "admin"})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/v1/users/9", bytes.NewBufferString(`{"username":"alice","avatar":"a.png","role_id":2,"sex":1,"address":3}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request for legacy address alias, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAdminUserTransportDoesNotMountInitRoute(t *testing.T) {
	router := newAdminUserTestRouter(&fakeUserService{}, &middleware.AuthIdentity{UserID: 7, Platform: "admin"})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/v1/users/init", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected users/init route to be removed, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func newAdminUserTestRouter(service usermodule.HTTPService, identity *middleware.AuthIdentity) *gin.Engine {
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

func requestAdminUserData(t *testing.T, router *gin.Engine, method string, path string, body string) map[string]any {
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
