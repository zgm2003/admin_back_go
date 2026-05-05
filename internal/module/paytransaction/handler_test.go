package paytransaction

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
	router, service := newPayTransactionHandlerRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/pay-transactions/page-init", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || service.called != "init" {
		t.Fatalf("unexpected init status=%d called=%s body=%s", recorder.Code, service.called, recorder.Body.String())
	}
}

func TestHandlerBindsListQuery(t *testing.T) {
	router, service := newPayTransactionHandlerRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/pay-transactions?current_page=2&page_size=30&order_no=R1&transaction_no=T1&user_id=7&channel=2&status=3&start_date=2026-04-01&end_date=2026-04-30", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	query := service.listQuery
	if service.called != "list" || query.CurrentPage != 2 || query.PageSize != 30 || query.OrderNo != "R1" || query.TransactionNo != "T1" || query.StartDate != "2026-04-01" || query.EndDate != "2026-04-30" {
		t.Fatalf("unexpected list query: %#v", query)
	}
	if query.UserID == nil || *query.UserID != 7 || query.Channel == nil || *query.Channel != enum.PayChannelAlipay || query.Status == nil || *query.Status != enum.PayTxnSuccess {
		t.Fatalf("unexpected optional list filters: %#v", query)
	}
}

func TestHandlerRejectsInvalidTxnStatusBeforeService(t *testing.T) {
	router, service := newPayTransactionHandlerRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/pay-transactions?status=999", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, recorder.Code, recorder.Body.String())
	}
	if service.called != "" {
		t.Fatalf("service should not be called for invalid status")
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
	router, service := newPayTransactionHandlerRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/pay-transactions/11", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || service.called != "detail" || service.detailID != 11 {
		t.Fatalf("unexpected detail status=%d called=%s id=%d body=%s", recorder.Code, service.called, service.detailID, recorder.Body.String())
	}
}

func TestHandlerRejectsInvalidDetailID(t *testing.T) {
	router, service := newPayTransactionHandlerRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/pay-transactions/bad", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, recorder.Code, recorder.Body.String())
	}
	if service.called != "" {
		t.Fatalf("service should not be called for invalid id")
	}
}

func newPayTransactionHandlerRouter() (*gin.Engine, *fakeHTTPService) {
	gin.SetMode(gin.TestMode)
	service := &fakeHTTPService{}
	router := gin.New()
	RegisterRoutes(router, service)
	return router, service
}
