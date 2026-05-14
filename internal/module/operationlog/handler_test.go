package operationlog

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"admin_back_go/internal/apperror"
	projecti18n "admin_back_go/internal/i18n"

	"github.com/gin-gonic/gin"
)

type fakeHTTPService struct {
	listQuery  ListQuery
	listResult *ListResponse
	deleteIDs  []int64
	err        *apperror.Error
}

func (f *fakeHTTPService) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return &InitResponse{}, f.err
}

func (f *fakeHTTPService) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	f.listQuery = query
	if f.listResult != nil {
		return f.listResult, f.err
	}
	return &ListResponse{Page: Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize}}, f.err
}

func (f *fakeHTTPService) Delete(ctx context.Context, ids []int64) *apperror.Error {
	f.deleteIDs = ids
	return f.err
}

func TestHandlerListBindsRESTQuery(t *testing.T) {
	service := &fakeHTTPService{listResult: &ListResponse{
		List: []ListItem{{ID: 1, Action: "编辑用户"}},
		Page: Page{CurrentPage: 1, PageSize: 20, Total: 1, TotalPage: 1},
	}}
	router := newOperationLogTestRouter(service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/operation-logs?current_page=1&page_size=20&user_id=7&action=编辑&date=2026-05-01,2026-05-04", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.listQuery.CurrentPage != 1 || service.listQuery.PageSize != 20 || service.listQuery.UserID != 7 || service.listQuery.Action != "编辑" {
		t.Fatalf("query mismatch: %#v", service.listQuery)
	}
	if !reflect.DeepEqual(service.listQuery.DateRange, []string{"2026-05-01", "2026-05-04"}) {
		t.Fatalf("date range mismatch: %#v", service.listQuery.DateRange)
	}
	body := decodeOperationLogBody(t, recorder)
	data := body["data"].(map[string]any)
	if _, ok := data["list"]; !ok {
		t.Fatalf("missing list in response: %#v", data)
	}
}

func TestHandlerDeleteOneAndBatchUseRESTRoutes(t *testing.T) {
	service := &fakeHTTPService{}
	router := newOperationLogTestRouter(service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/admin/v1/operation-logs/9", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || !reflect.DeepEqual(service.deleteIDs, []int64{9}) {
		t.Fatalf("delete one mismatch: code=%d body=%s ids=%#v", recorder.Code, recorder.Body.String(), service.deleteIDs)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/admin/v1/operation-logs", bytes.NewReader([]byte(`{"ids":[9,10]}`)))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || !reflect.DeepEqual(service.deleteIDs, []int64{9, 10}) {
		t.Fatalf("delete batch mismatch: code=%d body=%s ids=%#v", recorder.Code, recorder.Body.String(), service.deleteIDs)
	}
}

func TestHandlerListLocalizesInvalidQuery(t *testing.T) {
	router := newOperationLogLocalizedTestRouter(&fakeHTTPService{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/operation-logs?current_page=abc", nil)
	request.Header.Set("Accept-Language", "en-US")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeOperationLogBody(t, recorder)
	if body["msg"] != "Invalid operation log list request" {
		t.Fatalf("expected localized query error, got %#v", body["msg"])
	}
}

func newOperationLogTestRouter(service HTTPService) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	RegisterRoutes(router, service)
	return router
}

func newOperationLogLocalizedTestRouter(service HTTPService) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(projecti18n.Localize())
	RegisterRoutes(router, service)
	return router
}

func decodeOperationLogBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	return body
}
