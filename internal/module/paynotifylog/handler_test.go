package paynotifylog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"

	"github.com/gin-gonic/gin"
)

type fakeHTTPService struct {
	called    string
	listQuery ListQuery
	detailID  int64
}

func (f *fakeHTTPService) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	f.called = "init"
	return &InitResponse{}, nil
}

func (f *fakeHTTPService) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	f.called = "list"
	f.listQuery = query
	return &ListResponse{List: []ListItem{}, Page: Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize}}, nil
}

func (f *fakeHTTPService) Detail(ctx context.Context, id int64) (*DetailResponse, *apperror.Error) {
	f.called = "detail"
	f.detailID = id
	return &DetailResponse{}, nil
}

func TestHandlerCallsInit(t *testing.T) {
	router, service := newPayNotifyLogHandlerRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/pay-notify-logs/page-init", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || service.called != "init" {
		t.Fatalf("unexpected init status=%d called=%s body=%s", recorder.Code, service.called, recorder.Body.String())
	}
}

func TestHandlerBindsListQuery(t *testing.T) {
	router, service := newPayNotifyLogHandlerRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/pay-notify-logs?current_page=2&page_size=30&transaction_no=T1&channel=2&notify_type=1&process_status=2&start_date=2026-05-01&end_date=2026-05-07", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	query := service.listQuery
	if service.called != "list" || query.CurrentPage != 2 || query.PageSize != 30 || query.TransactionNo != "T1" || query.StartDate != "2026-05-01" || query.EndDate != "2026-05-07" {
		t.Fatalf("unexpected list query: %#v", query)
	}
	if query.Channel == nil || *query.Channel != enum.PayChannelAlipay || query.NotifyType == nil || *query.NotifyType != enum.NotifyPay || query.ProcessStatus == nil || *query.ProcessStatus != enum.NotifyProcessSuccess {
		t.Fatalf("unexpected optional list filters: %#v", query)
	}
}

func TestHandlerRejectsInvalidProcessStatusBeforeService(t *testing.T) {
	router, service := newPayNotifyLogHandlerRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/pay-notify-logs?process_status=999", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, recorder.Code, recorder.Body.String())
	}
	if service.called != "" {
		t.Fatalf("service should not be called for invalid process status")
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["code"] != float64(apperror.CodeBadRequest) {
		t.Fatalf("unexpected response payload: %#v", payload)
	}
}

func TestHandlerParsesDetailID(t *testing.T) {
	router, service := newPayNotifyLogHandlerRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/pay-notify-logs/11", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || service.called != "detail" || service.detailID != 11 {
		t.Fatalf("unexpected detail status=%d called=%s id=%d body=%s", recorder.Code, service.called, service.detailID, recorder.Body.String())
	}
}

func TestHandlerRejectsInvalidDetailID(t *testing.T) {
	router, service := newPayNotifyLogHandlerRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/pay-notify-logs/bad", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, recorder.Code, recorder.Body.String())
	}
	if service.called != "" {
		t.Fatalf("service should not be called for invalid id")
	}
}

func newPayNotifyLogHandlerRouter() (*gin.Engine, *fakeHTTPService) {
	gin.SetMode(gin.TestMode)
	service := &fakeHTTPService{}
	router := gin.New()
	RegisterRoutes(router, service)
	return router, service
}
