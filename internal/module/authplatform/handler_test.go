package authplatform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"

	"github.com/gin-gonic/gin"
)

type fakeHTTPService struct {
	listQuery   ListQuery
	createInput CreateInput
	statusID    int64
	status      int
}

func (f *fakeHTTPService) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return (&Service{}).Init(ctx)
}

func (f *fakeHTTPService) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	f.listQuery = query
	return &ListResponse{List: []ListItem{}, Page: Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize}}, nil
}

func (f *fakeHTTPService) Create(ctx context.Context, input CreateInput) (int64, *apperror.Error) {
	f.createInput = input
	return 1, nil
}

func (f *fakeHTTPService) Update(ctx context.Context, id int64, input UpdateInput) *apperror.Error {
	return nil
}

func (f *fakeHTTPService) Delete(ctx context.Context, ids []int64) *apperror.Error {
	return nil
}

func (f *fakeHTTPService) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	f.statusID = id
	f.status = status
	return nil
}

func TestHandlerBindsListQueryWithValidator(t *testing.T) {
	router, service := newAuthPlatformHandlerRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/auth-platforms?current_page=1&page_size=50&status=1&name=PC", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if service.listQuery.CurrentPage != 1 || service.listQuery.PageSize != 50 || service.listQuery.Status == nil || *service.listQuery.Status != enum.CommonYes || service.listQuery.Name != "PC" {
		t.Fatalf("unexpected list query: %#v", service.listQuery)
	}
}

func TestHandlerRejectsUnsupportedCaptchaTypeBeforeService(t *testing.T) {
	router, service := newAuthPlatformHandlerRouter()

	body := `{"code":"mini","name":"小程序","login_types":["password"],"captcha_type":"click","access_ttl":3600,"refresh_ttl":86400,"bind_platform":1,"bind_device":2,"bind_ip":2,"single_session":1,"max_sessions":1,"allow_register":1}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth-platforms", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, recorder.Code, recorder.Body.String())
	}
	if service.createInput.Code != "" {
		t.Fatalf("service should not be called for invalid captcha_type: %#v", service.createInput)
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["code"] != float64(apperror.CodeBadRequest) {
		t.Fatalf("unexpected response payload: %#v", payload)
	}
}

func TestHandlerBindsStatusPatchBody(t *testing.T) {
	router, service := newAuthPlatformHandlerRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/auth-platforms/2/status", strings.NewReader(`{"status":2}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if service.statusID != 2 || service.status != enum.CommonNo {
		t.Fatalf("unexpected status input: id=%d status=%d", service.statusID, service.status)
	}
}

func newAuthPlatformHandlerRouter() (*gin.Engine, *fakeHTTPService) {
	gin.SetMode(gin.TestMode)
	service := &fakeHTTPService{}
	router := gin.New()
	RegisterRoutes(router, service)
	return router, service
}
