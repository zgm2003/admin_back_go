package queuemonitor

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
	listCalled       bool
	failedListQuery  FailedListQuery
	failedListResult *FailedListResponse
	err              *apperror.Error
}

func (f *fakeHTTPService) List(ctx context.Context) ([]QueueItem, *apperror.Error) {
	f.listCalled = true
	return []QueueItem{{Name: "critical", Label: "高优先级队列", Group: "critical"}}, f.err
}

func (f *fakeHTTPService) FailedList(ctx context.Context, query FailedListQuery) (*FailedListResponse, *apperror.Error) {
	f.failedListQuery = query
	if f.failedListResult != nil {
		return f.failedListResult, f.err
	}
	return &FailedListResponse{Page: Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize}}, f.err
}

func TestHandlerListUsesRESTReadOnlyRoute(t *testing.T) {
	service := &fakeHTTPService{}
	router := newQueueMonitorTestRouter(service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/queue-monitor", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !service.listCalled {
		t.Fatalf("expected list service call")
	}
	data := decodeQueueMonitorBody(t, recorder)["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("expected one queue item, got %#v", data)
	}
}

func TestHandlerFailedListBindsQueueAndPagination(t *testing.T) {
	service := &fakeHTTPService{failedListResult: &FailedListResponse{
		List: []FailedTaskItem{{ID: "task-1", State: "retry"}},
		Page: Page{CurrentPage: 2, PageSize: 10, Total: 1, TotalPage: 1},
	}}
	router := newQueueMonitorTestRouter(service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/queue-monitor/failed?queue=critical&current_page=2&page_size=10", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.failedListQuery.Queue != "critical" || service.failedListQuery.CurrentPage != 2 || service.failedListQuery.PageSize != 10 {
		t.Fatalf("query mismatch: %#v", service.failedListQuery)
	}
}

func newQueueMonitorTestRouter(service HTTPService) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	RegisterRoutes(router, service, nil)
	return router
}

func decodeQueueMonitorBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	return body
}
