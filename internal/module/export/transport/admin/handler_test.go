package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/middleware"
	exporttaskmodule "admin_back_go/internal/module/export"
	"admin_back_go/internal/shared/apperror"
	projecti18n "admin_back_go/internal/shared/i18n"

	"github.com/gin-gonic/gin"
)

type fakeHTTPService struct {
	statusQuery exporttaskmodule.StatusCountQuery
	listQuery   exporttaskmodule.ListQuery
	deleteInput exporttaskmodule.DeleteInput
	err         *apperror.Error
}

func (f *fakeHTTPService) StatusCount(ctx context.Context, query exporttaskmodule.StatusCountQuery) ([]exporttaskmodule.StatusCountItem, *apperror.Error) {
	f.statusQuery = query
	return []exporttaskmodule.StatusCountItem{{Label: "处理中", Value: 1, Num: 1}}, f.err
}

func (f *fakeHTTPService) List(ctx context.Context, query exporttaskmodule.ListQuery) (*exporttaskmodule.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &exporttaskmodule.ListResponse{List: []exporttaskmodule.ListItem{}, Page: exporttaskmodule.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize}}, f.err
}

func (f *fakeHTTPService) Delete(ctx context.Context, input exporttaskmodule.DeleteInput) *apperror.Error {
	f.deleteInput = input
	return f.err
}

func TestHandlerStatusCountRequiresAuthIdentity(t *testing.T) {
	router := newExportTaskTestRouter(&fakeHTTPService{}, nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/export-tasks/status-count", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerStatusCountScopesCurrentUser(t *testing.T) {
	service := &fakeHTTPService{}
	router := newExportTaskTestRouter(service, &middleware.AuthIdentity{UserID: 9, Platform: "admin"})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/export-tasks/status-count?kind=user_list&title=%E7%94%A8%E6%88%B7&file_name=u.xlsx", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.statusQuery.UserID != 9 ||
		service.statusQuery.Platform != "admin" ||
		service.statusQuery.Kind != "user_list" ||
		service.statusQuery.Title != "用户" ||
		service.statusQuery.FileName != "u.xlsx" {
		t.Fatalf("unexpected status query: %#v", service.statusQuery)
	}
}

func TestHandlerListBindsQueryAndScopesCurrentUser(t *testing.T) {
	service := &fakeHTTPService{}
	router := newExportTaskTestRouter(service, &middleware.AuthIdentity{UserID: 9, Platform: "admin"})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/export-tasks?kind=user_list&current_page=2&page_size=10&status=2&file_name=u.xlsx", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.listQuery.UserID != 9 ||
		service.listQuery.Platform != "admin" ||
		service.listQuery.Kind != "user_list" ||
		service.listQuery.CurrentPage != 2 ||
		service.listQuery.PageSize != 10 ||
		service.listQuery.Status == nil ||
		*service.listQuery.Status != 2 ||
		service.listQuery.FileName != "u.xlsx" {
		t.Fatalf("unexpected list query: %#v", service.listQuery)
	}
}

func TestHandlerDeleteSupportsSingleAndBatch(t *testing.T) {
	service := &fakeHTTPService{}
	router := newExportTaskTestRouter(service, &middleware.AuthIdentity{UserID: 9, Platform: "admin"})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/admin/v1/export-tasks/7", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.deleteInput.UserID != 9 || service.deleteInput.Platform != "admin" || len(service.deleteInput.IDs) != 1 || service.deleteInput.IDs[0] != 7 {
		t.Fatalf("single delete mismatch: code=%d body=%s input=%#v", recorder.Code, recorder.Body.String(), service.deleteInput)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/admin/v1/export-tasks", strings.NewReader(`{"ids":[3,2]}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.deleteInput.UserID != 9 || service.deleteInput.Platform != "admin" || len(service.deleteInput.IDs) != 2 || service.deleteInput.IDs[0] != 3 || service.deleteInput.IDs[1] != 2 {
		t.Fatalf("batch delete mismatch: code=%d body=%s input=%#v", recorder.Code, recorder.Body.String(), service.deleteInput)
	}
}

func TestHandlerListLocalizesInvalidQuery(t *testing.T) {
	router := newExportTaskLocalizedTestRouter(&fakeHTTPService{}, &middleware.AuthIdentity{UserID: 9, Platform: "admin"})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/export-tasks?current_page=abc", nil)
	request.Header.Set("Accept-Language", "en-US")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeExportTaskBody(t, recorder)
	if body["msg"] != "Invalid export task list request" {
		t.Fatalf("expected localized list query error, got %#v", body["msg"])
	}
}

func TestHandlerListLocalizesMissingIdentity(t *testing.T) {
	router := newExportTaskLocalizedTestRouter(&fakeHTTPService{}, nil)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/export-tasks?current_page=1&page_size=20", nil)
	request.Header.Set("Accept-Language", "en-US")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeExportTaskBody(t, recorder)
	if body["msg"] != "Token is invalid or expired" {
		t.Fatalf("expected localized token error, got %#v", body["msg"])
	}
}

func newExportTaskTestRouter(service exporttaskmodule.HTTPService, identity *middleware.AuthIdentity) *gin.Engine {
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

func newExportTaskLocalizedTestRouter(service exporttaskmodule.HTTPService, identity *middleware.AuthIdentity) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(projecti18n.Localize())
	if identity != nil {
		router.Use(func(c *gin.Context) {
			c.Set(middleware.ContextAuthIdentity, identity)
			c.Next()
		})
	}
	RegisterRoutes(router, service)
	return router
}

func decodeExportTaskBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	return body
}
