package role

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/module/permission"

	"github.com/gin-gonic/gin"
)

type fakeHTTPService struct {
	initResult *InitResponse
	listResult *ListResponse
	err        *apperror.Error

	listQuery   ListQuery
	createInput MutationInput
	updateID    int64
	updateInput MutationInput
	deleteIDs   []int64
	defaultID   int64
}

func (f *fakeHTTPService) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return f.initResult, f.err
}

func (f *fakeHTTPService) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	f.listQuery = query
	return f.listResult, f.err
}

func (f *fakeHTTPService) Create(ctx context.Context, input MutationInput) (int64, *apperror.Error) {
	f.createInput = input
	return 12, f.err
}

func (f *fakeHTTPService) Update(ctx context.Context, id int64, input MutationInput) *apperror.Error {
	f.updateID = id
	f.updateInput = input
	return f.err
}

func (f *fakeHTTPService) Delete(ctx context.Context, ids []int64) *apperror.Error {
	f.deleteIDs = append([]int64{}, ids...)
	return f.err
}

func (f *fakeHTTPService) SetDefault(ctx context.Context, id int64) *apperror.Error {
	f.defaultID = id
	return f.err
}

func TestHandlerInstallsRoleRESTRoutes(t *testing.T) {
	service := &fakeHTTPService{
		initResult: &InitResponse{Dict: InitDict{PermissionPlatformArr: []permission.DictOption[string]{{Label: "admin", Value: "admin"}}}},
		listResult: &ListResponse{List: []ListItem{}, Page: Page{CurrentPage: 1, PageSize: 50, Total: 0, TotalPage: 0}},
	}
	router := newRoleTestRouter(service)

	assertRoleStatus(t, router, http.MethodGet, "/api/v1/roles/init", "", http.StatusOK)
	assertRoleStatus(t, router, http.MethodGet, "/api/v1/roles?current_page=1&page_size=50&name=运营", "", http.StatusOK)
	assertRoleStatus(t, router, http.MethodPost, "/api/v1/roles", `{"name":"运营","permission_id":[2,3]}`, http.StatusOK)
	assertRoleStatus(t, router, http.MethodPut, "/api/v1/roles/9", `{"name":"运营","permission_id":[2]}`, http.StatusOK)
	assertRoleStatus(t, router, http.MethodPatch, "/api/v1/roles/9/default", "", http.StatusOK)
	assertRoleStatus(t, router, http.MethodDelete, "/api/v1/roles/9", "", http.StatusOK)
	assertRoleStatus(t, router, http.MethodDelete, "/api/v1/roles", `{"ids":[7,8]}`, http.StatusOK)

	if service.listQuery.CurrentPage != 1 || service.listQuery.PageSize != 50 || service.listQuery.Name != "运营" {
		t.Fatalf("list query mismatch: %#v", service.listQuery)
	}
	if service.createInput.Name != "运营" || len(service.createInput.PermissionIDs) != 2 {
		t.Fatalf("create input mismatch: %#v", service.createInput)
	}
	if service.updateID != 9 || service.defaultID != 9 {
		t.Fatalf("route id mismatch: update=%d default=%d", service.updateID, service.defaultID)
	}
}

func TestHandlerRejectsInvalidRoleInput(t *testing.T) {
	router := newRoleTestRouter(&fakeHTTPService{})

	assertRoleStatus(t, router, http.MethodGet, "/api/v1/roles?current_page=0&page_size=50", "", http.StatusBadRequest)
	assertRoleStatus(t, router, http.MethodPut, "/api/v1/roles/bad", `{"name":"运营","permission_id":[]}`, http.StatusBadRequest)
	assertRoleStatus(t, router, http.MethodDelete, "/api/v1/roles", `{"ids":[]}`, http.StatusBadRequest)
}

func newRoleTestRouter(service HTTPService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, service)
	return router
}

func assertRoleStatus(t *testing.T, router *gin.Engine, method string, path string, body string, want int) {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, path, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != want {
		t.Fatalf("%s %s expected %d got %d body=%s", method, path, want, recorder.Code, recorder.Body.String())
	}
}
