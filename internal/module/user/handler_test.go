package user

import (
	"bytes"
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

	pageInitResult *PageInitResponse
	profileUserID  int64
	profileViewer  int64
	profileResult  *ProfileResponse
	updateProfile  UpdateProfileInput
	updatePassword UpdatePasswordInput
	updateEmail    UpdateEmailInput
	updatePhone    UpdatePhoneInput
	listQuery      ListQuery
	listResult     *ListResponse
	exportInput    ExportInput
	exportResult   *ExportResponse
	updateID       int64
	updateInput    UpdateInput
	statusID       int64
	statusValue    int
	deleteIDs      []int64
	batchInput     BatchProfileUpdate
}

func (f *fakeInitService) Init(ctx context.Context, input InitInput) (*InitResponse, *apperror.Error) {
	f.input = input
	f.inputs = append(f.inputs, input)
	return f.result, f.err
}

func (f *fakeInitService) PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error) {
	return f.pageInitResult, f.err
}

func (f *fakeInitService) Profile(ctx context.Context, userID int64, currentUserID int64) (*ProfileResponse, *apperror.Error) {
	f.profileUserID = userID
	f.profileViewer = currentUserID
	if f.profileResult != nil {
		return f.profileResult, f.err
	}
	return &ProfileResponse{Profile: ProfileDetail{UserID: userID, IsSelf: enumCommonNoForTest(currentUserID, userID)}}, f.err
}

func (f *fakeInitService) UpdateProfile(ctx context.Context, input UpdateProfileInput) *apperror.Error {
	f.updateProfile = input
	return f.err
}

func (f *fakeInitService) UpdatePassword(ctx context.Context, input UpdatePasswordInput) *apperror.Error {
	f.updatePassword = input
	return f.err
}

func (f *fakeInitService) UpdateEmail(ctx context.Context, input UpdateEmailInput) *apperror.Error {
	f.updateEmail = input
	return f.err
}

func (f *fakeInitService) UpdatePhone(ctx context.Context, input UpdatePhoneInput) *apperror.Error {
	f.updatePhone = input
	return f.err
}

func (f *fakeInitService) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	f.listQuery = query
	return f.listResult, f.err
}

func (f *fakeInitService) Export(ctx context.Context, input ExportInput) (*ExportResponse, *apperror.Error) {
	f.exportInput = input
	if f.exportResult != nil {
		return f.exportResult, f.err
	}
	return &ExportResponse{ID: 77, Message: "导出任务已提交，完成后将通知您"}, f.err
}

func (f *fakeInitService) Update(ctx context.Context, id int64, input UpdateInput) *apperror.Error {
	f.updateID = id
	f.updateInput = input
	return f.err
}

func (f *fakeInitService) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	f.statusID = id
	f.statusValue = status
	return f.err
}

func (f *fakeInitService) Delete(ctx context.Context, ids []int64) *apperror.Error {
	f.deleteIDs = ids
	return f.err
}

func (f *fakeInitService) BatchUpdateProfile(ctx context.Context, input BatchProfileUpdate) *apperror.Error {
	f.batchInput = input
	return f.err
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

func TestHandlerPageInitUsesDedicatedRouteWithoutOverloadingUsersInit(t *testing.T) {
	service := &fakeInitService{pageInitResult: &PageInitResponse{
		Dict: PageInitDict{
			RoleArr: []RoleOption{{Label: "管理员", Value: 1}},
			AuthAddressTree: []AddressTreeNode{{
				Label: "中国",
				Value: 1,
			}},
			SexArr:      []SexOption{{Label: "未知", Value: 0}},
			PlatformArr: []PlatformOption{{Label: "admin", Value: "admin"}},
		},
	}}
	router := newUserTestRouter(service, &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"})

	data := requestUserData(t, router, http.MethodGet, "/api/admin/v1/users/page-init")

	dict := data["dict"].(map[string]any)
	if _, ok := dict["roleArr"]; !ok {
		t.Fatalf("missing roleArr in page-init: %#v", data)
	}
	if len(service.inputs) != 0 {
		t.Fatalf("page-init must not call current-user Init, inputs=%#v", service.inputs)
	}
}

func TestHandlerProfileRoutesUseAuthIdentityAndExplicitAddressIDContract(t *testing.T) {
	service := &fakeInitService{profileResult: &ProfileResponse{
		Profile: ProfileDetail{UserID: 1, Username: "admin", IsSelf: 1},
		Dict:    ProfileDict{SexArr: []SexOption{{Label: "未知", Value: 0}}},
	}}
	router := newUserTestRouter(service, &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"})

	data := requestUserData(t, router, http.MethodGet, "/api/admin/v1/profile")
	if service.profileUserID != 1 || service.profileViewer != 1 {
		t.Fatalf("current profile service input mismatch: userID=%d viewer=%d", service.profileUserID, service.profileViewer)
	}
	if _, ok := data["profile"]; !ok {
		t.Fatalf("missing profile payload: %#v", data)
	}

	_ = requestUserData(t, router, http.MethodGet, "/api/admin/v1/users/9/profile")
	if service.profileUserID != 9 || service.profileViewer != 1 {
		t.Fatalf("target profile service input mismatch: userID=%d viewer=%d", service.profileUserID, service.profileViewer)
	}

	birthday := "2026-05-05"
	validBody := []byte(`{"username":"alice","avatar":"a.png","sex":1,"birthday":"` + birthday + `","address_id":3,"detail_address":"玄武区","bio":"bio"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/v1/profile", bytes.NewReader(validBody))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected valid update 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.updateProfile.UserID != 1 || service.updateProfile.AddressID != 3 || service.updateProfile.Birthday == nil || *service.updateProfile.Birthday != birthday {
		t.Fatalf("update profile input mismatch: %#v", service.updateProfile)
	}

	legacyAliasBody := []byte(`{"username":"alice","avatar":"a.png","sex":1,"address":3}`)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, "/api/admin/v1/profile", bytes.NewReader(legacyAliasBody))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected legacy address alias to be rejected, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerProfileSecurityRoutesUseCurrentIdentity(t *testing.T) {
	service := &fakeInitService{}
	router := newUserTestRouter(service, &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"})

	passwordBody := []byte(`{"verify_type":"code","account":"alice@example.com","code":"123456","new_password":"new-secret","confirm_password":"new-secret"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/v1/profile/security/password", bytes.NewReader(passwordBody))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected password update 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.updatePassword.UserID != 1 || service.updatePassword.VerifyType != "code" || service.updatePassword.Account != "alice@example.com" {
		t.Fatalf("password input mismatch: %#v", service.updatePassword)
	}

	emailBody := []byte(`{"email":"new@example.com","code":"123456"}`)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, "/api/admin/v1/profile/security/email", bytes.NewReader(emailBody))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected email update 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.updateEmail.UserID != 1 || service.updateEmail.Email != "new@example.com" || service.updateEmail.Code != "123456" {
		t.Fatalf("email input mismatch: %#v", service.updateEmail)
	}

	phoneBody := []byte(`{"phone":"15671628271","code":"123456"}`)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, "/api/admin/v1/profile/security/phone", bytes.NewReader(phoneBody))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected phone update 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.updatePhone.UserID != 1 || service.updatePhone.Phone != "15671628271" || service.updatePhone.Code != "123456" {
		t.Fatalf("phone input mismatch: %#v", service.updatePhone)
	}
}

func TestHandlerListBindsRESTQueryAndCommaAddressIDs(t *testing.T) {
	service := &fakeInitService{listResult: &ListResponse{
		List: []ListItem{{ID: 1, Username: "alice"}},
		Page: Page{CurrentPage: 1, PageSize: 20, Total: 1, TotalPage: 1},
	}}
	router := newUserTestRouter(service, &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"})

	data := requestUserData(t, router, http.MethodGet, "/api/admin/v1/users?current_page=1&page_size=20&username=alice&address_id=3,4&sex=1")

	if service.listQuery.CurrentPage != 1 || service.listQuery.PageSize != 20 || service.listQuery.Username != "alice" {
		t.Fatalf("list query mismatch: %#v", service.listQuery)
	}
	if !reflect.DeepEqual(service.listQuery.AddressIDs, []int64{3, 4}) {
		t.Fatalf("address ids mismatch: %#v", service.listQuery.AddressIDs)
	}
	if service.listQuery.Sex == nil || *service.listQuery.Sex != 1 {
		t.Fatalf("sex query mismatch: %#v", service.listQuery.Sex)
	}
	if _, ok := data["list"]; !ok {
		t.Fatalf("missing list in response: %#v", data)
	}
}

func TestHandlerUpdateRequiresAddressIDNotLegacyAddressAlias(t *testing.T) {
	service := &fakeInitService{}
	router := newUserTestRouter(service, &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"})

	validBody := []byte(`{"username":"alice","avatar":"a.png","role_id":2,"sex":1,"address_id":3,"detail_address":"玄武区","bio":"bio"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/v1/users/9", bytes.NewReader(validBody))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected valid update 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.updateID != 9 || service.updateInput.AddressID != 3 || service.updateInput.Username != "alice" {
		t.Fatalf("update input mismatch: id=%d input=%#v", service.updateID, service.updateInput)
	}

	legacyAliasBody := []byte(`{"username":"alice","avatar":"a.png","role_id":2,"sex":1,"address":3}`)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, "/api/admin/v1/users/9", bytes.NewReader(legacyAliasBody))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected legacy address alias to be rejected, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerStatusDeleteAndBatchProfileRoutes(t *testing.T) {
	service := &fakeInitService{}
	router := newUserTestRouter(service, &middleware.AuthIdentity{UserID: 1, SessionID: 10, Platform: "admin"})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/users/9/status", bytes.NewReader([]byte(`{"status":2}`)))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.statusID != 9 || service.statusValue != 2 {
		t.Fatalf("status route mismatch: code=%d body=%s service=%#v", recorder.Code, recorder.Body.String(), service)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/admin/v1/users/9", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !reflect.DeepEqual(service.deleteIDs, []int64{9}) {
		t.Fatalf("delete one route mismatch: code=%d body=%s ids=%#v", recorder.Code, recorder.Body.String(), service.deleteIDs)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/admin/v1/users", bytes.NewReader([]byte(`{"ids":[8,9]}`)))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !reflect.DeepEqual(service.deleteIDs, []int64{8, 9}) {
		t.Fatalf("delete batch route mismatch: code=%d body=%s ids=%#v", recorder.Code, recorder.Body.String(), service.deleteIDs)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPatch, "/api/admin/v1/users", bytes.NewReader([]byte(`{"ids":[8,9],"field":"address_id","address_id":3}`)))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.batchInput.Field != BatchProfileFieldAddressID || service.batchInput.AddressID != 3 {
		t.Fatalf("batch profile route mismatch: code=%d body=%s input=%#v", recorder.Code, recorder.Body.String(), service.batchInput)
	}
}

func newUserTestRouter(service HTTPService, identity *middleware.AuthIdentity) *gin.Engine {
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

func enumCommonNoForTest(currentUserID int64, userID int64) int {
	if currentUserID > 0 && currentUserID == userID {
		return 1
	}
	return 2
}

func TestHandlerExportUsesAuthIdentityAndBodyIDs(t *testing.T) {
	service := &fakeInitService{exportResult: &ExportResponse{ID: 88, Message: "导出任务已提交，完成后将通知您"}}
	router := newUserTestRouter(service, &middleware.AuthIdentity{UserID: 9, SessionID: 10, Platform: "admin"})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/users/export", bytes.NewBufferString(`{"ids":[3,2]}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.exportInput.UserID != 9 || service.exportInput.Platform != "admin" || !reflect.DeepEqual(service.exportInput.IDs, []int64{3, 2}) {
		t.Fatalf("unexpected export input: %#v", service.exportInput)
	}
	body := decodeUserBody(t, recorder)
	data := body["data"].(map[string]any)
	if data["message"] != "导出任务已提交，完成后将通知您" {
		t.Fatalf("unexpected response: %#v", data)
	}
}
