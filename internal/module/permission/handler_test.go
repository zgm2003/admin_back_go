package permission

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin_back_go/internal/apperror"

	"github.com/gin-gonic/gin"
)

type fakeManagementService struct {
	initResult  *InitResponse
	listResult  []PermissionListItem
	createdID   int64
	err         *apperror.Error
	listQuery   PermissionListQuery
	createInput PermissionMutationInput
	updateID    int64
	updateInput PermissionMutationInput
	deleteIDs   []int64
	statusID    int64
	statusValue int
}

func (f *fakeManagementService) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return f.initResult, f.err
}

func (f *fakeManagementService) List(ctx context.Context, query PermissionListQuery) ([]PermissionListItem, *apperror.Error) {
	f.listQuery = query
	return f.listResult, f.err
}

func (f *fakeManagementService) Create(ctx context.Context, input PermissionMutationInput) (int64, *apperror.Error) {
	f.createInput = input
	return f.createdID, f.err
}

func (f *fakeManagementService) Update(ctx context.Context, id int64, input PermissionMutationInput) *apperror.Error {
	f.updateID = id
	f.updateInput = input
	return f.err
}

func (f *fakeManagementService) Delete(ctx context.Context, ids []int64) *apperror.Error {
	f.deleteIDs = ids
	return f.err
}

func (f *fakeManagementService) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	f.statusID = id
	f.statusValue = status
	return f.err
}

func TestPermissionRoutesUseRESTfulMethods(t *testing.T) {
	service := &fakeManagementService{
		initResult: &InitResponse{Dict: PermissionDict{PermissionTypeArr: permissionTypeOptions()}},
		listResult: []PermissionListItem{{ID: 1, Name: "系统"}},
		createdID:  9,
	}
	router := newPermissionTestRouter(service)

	assertPermissionStatus(t, router, http.MethodGet, "/api/admin/v1/permissions/init", nil, http.StatusOK)
	assertPermissionStatus(t, router, http.MethodGet, "/api/admin/v1/permissions?platform=admin&name=系统", nil, http.StatusOK)
	assertPermissionStatus(t, router, http.MethodPost, "/api/admin/v1/permissions", permissionRequestBody(TypeDir), http.StatusOK)
	assertPermissionStatus(t, router, http.MethodPut, "/api/admin/v1/permissions/9", permissionRequestBody(TypePage), http.StatusOK)
	assertPermissionStatus(t, router, http.MethodPatch, "/api/admin/v1/permissions/9/status", map[string]int{"status": CommonNo}, http.StatusOK)
	assertPermissionStatus(t, router, http.MethodDelete, "/api/admin/v1/permissions/9", nil, http.StatusOK)

	if service.listQuery.Platform != "admin" || service.listQuery.Name != "系统" {
		t.Fatalf("query mismatch: %#v", service.listQuery)
	}
	if service.createInput.Type != TypeDir || service.createInput.Name != "目录" {
		t.Fatalf("create input mismatch: %#v", service.createInput)
	}
	if service.updateID != 9 || service.updateInput.Type != TypePage {
		t.Fatalf("update input mismatch: id=%d input=%#v", service.updateID, service.updateInput)
	}
	if service.statusID != 9 || service.statusValue != CommonNo {
		t.Fatalf("status input mismatch: id=%d status=%d", service.statusID, service.statusValue)
	}
	if len(service.deleteIDs) != 1 || service.deleteIDs[0] != 9 {
		t.Fatalf("delete ids mismatch: %#v", service.deleteIDs)
	}
}

func TestPermissionBatchDeleteUsesDeleteBodyNotPostFallback(t *testing.T) {
	service := &fakeManagementService{}
	router := newPermissionTestRouter(service)

	assertPermissionStatus(t, router, http.MethodDelete, "/api/admin/v1/permissions", map[string][]int64{"ids": []int64{7, 8}}, http.StatusOK)

	if len(service.deleteIDs) != 2 || service.deleteIDs[0] != 7 || service.deleteIDs[1] != 8 {
		t.Fatalf("delete ids mismatch: %#v", service.deleteIDs)
	}
}

func TestPermissionHandlerRejectsInvalidRouteID(t *testing.T) {
	router := newPermissionTestRouter(&fakeManagementService{})

	assertPermissionStatus(t, router, http.MethodPut, "/api/admin/v1/permissions/bad", permissionRequestBody(TypeDir), http.StatusBadRequest)
}

func TestPermissionHandlerRejectsInvalidEnumInputsBeforeService(t *testing.T) {
	service := &fakeManagementService{}
	router := newPermissionTestRouter(service)

	assertPermissionStatus(t, router, http.MethodGet, "/api/admin/v1/permissions?platform=crm", nil, http.StatusBadRequest)
	assertPermissionStatus(t, router, http.MethodPost, "/api/admin/v1/permissions", permissionRequestBody(99), http.StatusBadRequest)
	assertPermissionStatus(t, router, http.MethodPatch, "/api/admin/v1/permissions/9/status", map[string]int{"status": 9}, http.StatusBadRequest)

	if service.listQuery.Platform != "" || service.createInput.Name != "" || service.statusID != 0 {
		t.Fatalf("service should not be called for invalid enum inputs: query=%#v create=%#v statusID=%d", service.listQuery, service.createInput, service.statusID)
	}
}

func newPermissionTestRouter(service ManagementService) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	RegisterRoutes(router, service)
	return router
}

func assertPermissionStatus(t *testing.T, router *gin.Engine, method string, path string, body any, wantStatus int) {
	t.Helper()
	var requestBody *bytes.Reader
	if body == nil {
		requestBody = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		requestBody = bytes.NewReader(payload)
	}

	request := httptest.NewRequest(method, path, requestBody)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != wantStatus {
		t.Fatalf("%s %s expected status %d, got %d body=%s", method, path, wantStatus, recorder.Code, recorder.Body.String())
	}
}

func permissionRequestBody(permissionType int) map[string]any {
	body := map[string]any{
		"platform":  "admin",
		"type":      permissionType,
		"name":      "目录",
		"parent_id": 0,
		"sort":      1,
	}
	switch permissionType {
	case TypeDir:
		body["i18n_key"] = "menu.system"
		body["show_menu"] = CommonYes
	case TypePage:
		body["name"] = "页面"
		body["path"] = "/system/user"
		body["component"] = "system/user/index"
		body["i18n_key"] = "menu.system_user"
		body["show_menu"] = CommonYes
	case TypeButton:
		body["name"] = "新增"
		body["parent_id"] = 2
		body["code"] = "permission_add"
	}
	return body
}
