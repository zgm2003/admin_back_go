package usersession

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin_back_go/internal/apperror"

	"github.com/gin-gonic/gin"
)

type fakeHTTPService struct {
	pageInitResult *PageInitResponse
	listQuery      ListQuery
	listResult     *ListResponse
	statsResult    *StatsResponse
	err            *apperror.Error
}

func (f *fakeHTTPService) PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error) {
	return f.pageInitResult, f.err
}

func (f *fakeHTTPService) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	f.listQuery = query
	return f.listResult, f.err
}

func (f *fakeHTTPService) Stats(ctx context.Context) (*StatsResponse, *apperror.Error) {
	return f.statsResult, f.err
}

func TestHandlerRoutesUserSessionReadOnlyEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeHTTPService{
		pageInitResult: &PageInitResponse{},
		listResult:     &ListResponse{Page: Page{PageSize: 30, CurrentPage: 2, Total: 1, TotalPage: 1}},
		statsResult:    &StatsResponse{TotalActive: 1, PlatformDistribution: map[string]int64{"admin": 1, "app": 0}},
	}
	router := gin.New()
	RegisterRoutes(router, service)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/v1/user-sessions?current_page=2&page_size=30&username=test&platform=admin&status=active", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", resp.Code, resp.Body.String())
	}
	if service.listQuery.CurrentPage != 2 || service.listQuery.PageSize != 30 || service.listQuery.Username != "test" || service.listQuery.Platform != "admin" || service.listQuery.Status != "active" {
		t.Fatalf("list query mismatch: %#v", service.listQuery)
	}

	for _, path := range []string{"/api/admin/v1/user-sessions/page-init", "/api/admin/v1/user-sessions/stats"} {
		req = httptest.NewRequest(http.MethodGet, path, nil)
		resp = httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, resp.Code, resp.Body.String())
		}
	}
}
