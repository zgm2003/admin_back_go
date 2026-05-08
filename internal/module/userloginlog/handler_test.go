package userloginlog

import (
	"context"
	"encoding/json"
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
	err            *apperror.Error
}

func (f *fakeHTTPService) PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error) {
	return f.pageInitResult, f.err
}

func (f *fakeHTTPService) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	f.listQuery = query
	return f.listResult, f.err
}

func TestHandlerPageInitReturnsDict(t *testing.T) {
	service := &fakeHTTPService{pageInitResult: &PageInitResponse{Dict: PageInitDict{
		PlatformArr:  []Option[string]{{Label: "admin", Value: "admin"}},
		LoginTypeArr: []Option[string]{{Label: "密码登录", Value: "password"}},
	}}}
	router := newLoginLogTestRouter(service)

	data := requestLoginLogData(t, router, http.MethodGet, "/api/admin/v1/users/login-logs/page-init")
	if _, ok := data["dict"]; !ok {
		t.Fatalf("missing dict payload: %#v", data)
	}
}

func TestHandlerListBindsQuery(t *testing.T) {
	service := &fakeHTTPService{listResult: &ListResponse{List: []ListItem{{ID: 1}}, Page: Page{PageSize: 20, CurrentPage: 1, Total: 1, TotalPage: 1}}}
	router := newLoginLogTestRouter(service)

	_ = requestLoginLogData(t, router, http.MethodGet, "/api/admin/v1/users/login-logs?current_page=2&page_size=30&user_id=44&login_account=adm&login_type=password&ip=127&platform=admin&is_success=1&date_start=2026-05-01&date_end=2026-05-08")

	if service.listQuery.CurrentPage != 2 || service.listQuery.PageSize != 30 || service.listQuery.UserID != 44 {
		t.Fatalf("pagination/user filters mismatch: %#v", service.listQuery)
	}
	if service.listQuery.LoginAccount != "adm" || service.listQuery.LoginType != "password" || service.listQuery.IP != "127" || service.listQuery.Platform != "admin" {
		t.Fatalf("string filters mismatch: %#v", service.listQuery)
	}
	if service.listQuery.IsSuccess == nil || *service.listQuery.IsSuccess != 1 {
		t.Fatalf("is_success mismatch: %#v", service.listQuery)
	}
	if service.listQuery.DateStart != "2026-05-01" || service.listQuery.DateEnd != "2026-05-08" {
		t.Fatalf("date filters mismatch: %#v", service.listQuery)
	}
}

func newLoginLogTestRouter(service HTTPService) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	RegisterRoutes(router, service)
	return router
}

func requestLoginLogData(t *testing.T, router *gin.Engine, method, path string) map[string]any {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("%s %s expected status 200, got %d body=%s", method, path, recorder.Code, recorder.Body.String())
	}
	body := decodeLoginLogBody(t, recorder)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing object data in response: %#v", body)
	}
	return data
}

func decodeLoginLogBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	return body
}
