package exporttask

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/middleware"

	"github.com/gin-gonic/gin"
)

type fakeHTTPService struct {
	statusQuery StatusCountQuery
	listQuery   ListQuery
	deleteInput DeleteInput
	err         *apperror.Error
}

func (f *fakeHTTPService) StatusCount(ctx context.Context, query StatusCountQuery) ([]StatusCountItem, *apperror.Error) {
	f.statusQuery = query
	return []StatusCountItem{{Label: "处理中", Value: 1, Num: 1}}, f.err
}

func (f *fakeHTTPService) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	f.listQuery = query
	return &ListResponse{List: []ListItem{}, Page: Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize}}, f.err
}

func (f *fakeHTTPService) Delete(ctx context.Context, input DeleteInput) *apperror.Error {
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
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/export-tasks/status-count?title=%E7%94%A8%E6%88%B7", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.statusQuery.UserID != 9 || service.statusQuery.Title != "用户" {
		t.Fatalf("unexpected status query: %#v", service.statusQuery)
	}
}

func TestHandlerListBindsQueryAndScopesCurrentUser(t *testing.T) {
	service := &fakeHTTPService{}
	router := newExportTaskTestRouter(service, &middleware.AuthIdentity{UserID: 9, Platform: "admin"})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/export-tasks?current_page=2&page_size=10&status=2&file_name=u.xlsx", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.listQuery.UserID != 9 || service.listQuery.CurrentPage != 2 || service.listQuery.PageSize != 10 || service.listQuery.Status == nil || *service.listQuery.Status != 2 || service.listQuery.FileName != "u.xlsx" {
		t.Fatalf("unexpected list query: %#v", service.listQuery)
	}
}

func TestHandlerDeleteSupportsSingleAndBatch(t *testing.T) {
	service := &fakeHTTPService{}
	router := newExportTaskTestRouter(service, &middleware.AuthIdentity{UserID: 9, Platform: "admin"})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/admin/v1/export-tasks/7", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.deleteInput.UserID != 9 || len(service.deleteInput.IDs) != 1 || service.deleteInput.IDs[0] != 7 {
		t.Fatalf("single delete mismatch: code=%d body=%s input=%#v", recorder.Code, recorder.Body.String(), service.deleteInput)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/admin/v1/export-tasks", strings.NewReader(`{"ids":[3,2]}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.deleteInput.UserID != 9 || len(service.deleteInput.IDs) != 2 || service.deleteInput.IDs[0] != 3 || service.deleteInput.IDs[1] != 2 {
		t.Fatalf("batch delete mismatch: code=%d body=%s input=%#v", recorder.Code, recorder.Body.String(), service.deleteInput)
	}
}

func newExportTaskTestRouter(service HTTPService, identity *middleware.AuthIdentity) *gin.Engine {
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

func decodeExportTaskBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	return body
}
