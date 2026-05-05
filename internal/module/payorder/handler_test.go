package payorder

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
	called           string
	statusCountQuery StatusCountQuery
	listQuery        ListQuery
	detailID         int64
	remarkID         int64
	remark           string
	closeID          int64
	closeReason      string
}

func (f *fakeHTTPService) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	f.called = "init"
	return &InitResponse{}, nil
}
func (f *fakeHTTPService) StatusCount(ctx context.Context, query StatusCountQuery) ([]StatusCountItem, *apperror.Error) {
	f.called = "status-count"
	f.statusCountQuery = query
	return []StatusCountItem{}, nil
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
func (f *fakeHTTPService) Remark(ctx context.Context, id int64, input RemarkInput) *apperror.Error {
	f.called = "remark"
	f.remarkID = id
	f.remark = input.Remark
	return nil
}
func (f *fakeHTTPService) Close(ctx context.Context, id int64, input CloseInput) *apperror.Error {
	f.called = "close"
	f.closeID = id
	f.closeReason = input.Reason
	return nil
}

func TestHandlerRoutes(t *testing.T) {
	router, service := newPayOrderHandlerRouter()

	cases := []struct {
		method string
		path   string
		body   string
		want   string
	}{
		{http.MethodGet, "/api/admin/v1/pay-orders/page-init", "", "init"},
		{http.MethodGet, "/api/admin/v1/pay-orders/status-count?order_no=R1&user_id=7", "", "status-count"},
		{http.MethodGet, "/api/admin/v1/pay-orders?current_page=2&page_size=30&order_no=R1&user_id=7&order_type=1&pay_status=3&start_date=2026-04-01&end_date=2026-04-30", "", "list"},
		{http.MethodGet, "/api/admin/v1/pay-orders/9", "", "detail"},
		{http.MethodPatch, "/api/admin/v1/pay-orders/9/remark", `{"remark":"ok"}`, "remark"},
		{http.MethodPatch, "/api/admin/v1/pay-orders/9/close", `{"reason":"admin"}`, "close"},
	}
	for _, tc := range cases {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || service.called != tc.want {
			t.Fatalf("%s %s expected called=%s status=%d, got called=%s status=%d body=%s", tc.method, tc.path, tc.want, http.StatusOK, service.called, recorder.Code, recorder.Body.String())
		}
	}

	if service.listQuery.CurrentPage != 2 || service.listQuery.PageSize != 30 || service.listQuery.OrderNo != "R1" || service.listQuery.StartDate != "2026-04-01" || service.listQuery.EndDate != "2026-04-30" {
		t.Fatalf("unexpected list query: %#v", service.listQuery)
	}
	if service.listQuery.UserID == nil || *service.listQuery.UserID != 7 || service.listQuery.OrderType == nil || *service.listQuery.OrderType != enum.PayOrderRecharge || service.listQuery.PayStatus == nil || *service.listQuery.PayStatus != enum.PayStatusPaid {
		t.Fatalf("unexpected optional list filters: %#v", service.listQuery)
	}
	if service.statusCountQuery.UserID == nil || *service.statusCountQuery.UserID != 7 || service.detailID != 9 || service.remarkID != 9 || service.closeID != 9 {
		t.Fatalf("unexpected handler captured values: %#v", service)
	}
}

func TestHandlerRejectsInvalidPayStatusBeforeService(t *testing.T) {
	router, service := newPayOrderHandlerRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/pay-orders?current_page=1&page_size=20&pay_status=999", nil)
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

func TestHandlerRejectsInvalidIDBeforeService(t *testing.T) {
	router, service := newPayOrderHandlerRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/pay-orders/bad", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, recorder.Code, recorder.Body.String())
	}
	if service.called != "" {
		t.Fatalf("service should not be called for invalid id")
	}
}

func newPayOrderHandlerRouter() (*gin.Engine, *fakeHTTPService) {
	gin.SetMode(gin.TestMode)
	service := &fakeHTTPService{}
	router := gin.New()
	RegisterRoutes(router, service)
	return router, service
}
