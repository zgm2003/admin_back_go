package usersession

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/middleware"

	"github.com/gin-gonic/gin"
)

type fakeHTTPService struct {
	pageInitResult *PageInitResponse
	listQuery      ListQuery
	listResult     *ListResponse
	statsResult    *StatsResponse
	revokeID       int64
	batchInput     BatchRevokeInput
	currentSession int64
	revokeResult   *RevokeResponse
	batchResult    *BatchRevokeResponse
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

func (f *fakeHTTPService) Revoke(ctx context.Context, id int64, currentSessionID int64) (*RevokeResponse, *apperror.Error) {
	f.revokeID = id
	f.currentSession = currentSessionID
	if f.revokeResult != nil {
		return f.revokeResult, f.err
	}
	return &RevokeResponse{ID: id, Revoked: true}, f.err
}

func (f *fakeHTTPService) BatchRevoke(ctx context.Context, input BatchRevokeInput, currentSessionID int64) (*BatchRevokeResponse, *apperror.Error) {
	f.batchInput = input
	f.currentSession = currentSessionID
	if f.batchResult != nil {
		return f.batchResult, f.err
	}
	return &BatchRevokeResponse{Count: int64(len(input.IDs))}, f.err
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

func TestHandlerRevokeUsesCurrentSessionIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeHTTPService{revokeResult: &RevokeResponse{ID: 77, Revoked: true}}
	router := newUserSessionTestRouter(service, &middleware.AuthIdentity{UserID: 1, SessionID: 55, Platform: "admin"})

	req := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/user-sessions/77/revoke", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("revoke status=%d body=%s", resp.Code, resp.Body.String())
	}
	if service.revokeID != 77 || service.currentSession != 55 {
		t.Fatalf("revoke service input mismatch: id=%d current=%d", service.revokeID, service.currentSession)
	}
}

func TestHandlerBatchRevokeUsesCurrentSessionIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeHTTPService{batchResult: &BatchRevokeResponse{Count: 2, SkippedCurrent: 1}}
	router := newUserSessionTestRouter(service, &middleware.AuthIdentity{UserID: 1, SessionID: 55, Platform: "admin"})

	req := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/user-sessions/revoke", bytes.NewBufferString(`{"ids":[77,55,78]}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("batch revoke status=%d body=%s", resp.Code, resp.Body.String())
	}
	if service.currentSession != 55 || len(service.batchInput.IDs) != 3 || service.batchInput.IDs[1] != 55 {
		t.Fatalf("batch revoke service input mismatch: current=%d input=%#v", service.currentSession, service.batchInput)
	}
}

func newUserSessionTestRouter(service HTTPService, identity *middleware.AuthIdentity) *gin.Engine {
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
