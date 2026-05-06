package payruntime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

type fakeHTTPService struct {
	createRechargeOrderCalled bool
	createPayAttemptCalled    bool
	listOrdersCalled          bool
	queryResultCalled         bool
	cancelOrderCalled         bool
	walletSummaryCalled       bool
	walletBillsCalled         bool
	notifyCalled              bool

	listOrdersUserID    int64
	listOrdersQuery     CurrentUserOrderListQuery
	queryResultUserID   int64
	queryResultOrderNo  string
	cancelOrderUserID   int64
	cancelOrderOrderNo  string
	cancelOrderInput    CancelOrderInput
	walletSummaryUserID int64
	walletBillsUserID   int64
	walletBillsQuery    WalletBillsQuery
}

func (f *fakeHTTPService) CreateRechargeOrder(ctx context.Context, userID int64, input RechargeOrderCreateInput) (*RechargeOrderCreateResponse, *apperror.Error) {
	f.createRechargeOrderCalled = true
	return &RechargeOrderCreateResponse{}, nil
}

func (f *fakeHTTPService) CreatePayAttempt(ctx context.Context, userID int64, orderNo string, input PayAttemptCreateInput) (*PayAttemptCreateResponse, *apperror.Error) {
	f.createPayAttemptCalled = true
	return &PayAttemptCreateResponse{}, nil
}

func (f *fakeHTTPService) ListCurrentUserRechargeOrders(ctx context.Context, userID int64, query CurrentUserOrderListQuery) (*CurrentUserOrderListResponse, *apperror.Error) {
	f.listOrdersCalled = true
	f.listOrdersUserID = userID
	f.listOrdersQuery = query
	return &CurrentUserOrderListResponse{List: []CurrentUserOrderItem{}, Page: Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize}}, nil
}

func (f *fakeHTTPService) QueryCurrentUserRechargeResult(ctx context.Context, userID int64, orderNo string) (*OrderQueryResultResponse, *apperror.Error) {
	f.queryResultCalled = true
	f.queryResultUserID = userID
	f.queryResultOrderNo = orderNo
	return &OrderQueryResultResponse{OrderNo: orderNo}, nil
}

func (f *fakeHTTPService) CancelCurrentUserRechargeOrder(ctx context.Context, userID int64, orderNo string, input CancelOrderInput) *apperror.Error {
	f.cancelOrderCalled = true
	f.cancelOrderUserID = userID
	f.cancelOrderOrderNo = orderNo
	f.cancelOrderInput = input
	return nil
}

func (f *fakeHTTPService) CurrentUserWalletSummary(ctx context.Context, userID int64) (*WalletSummaryResponse, *apperror.Error) {
	f.walletSummaryCalled = true
	f.walletSummaryUserID = userID
	return &WalletSummaryResponse{WalletExists: 1}, nil
}

func (f *fakeHTTPService) CurrentUserWalletBills(ctx context.Context, userID int64, query WalletBillsQuery) (*WalletBillsResponse, *apperror.Error) {
	f.walletBillsCalled = true
	f.walletBillsUserID = userID
	f.walletBillsQuery = query
	return &WalletBillsResponse{List: []WalletBillItem{}, Page: Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize}}, nil
}

func (f *fakeHTTPService) HandleAlipayNotify(ctx context.Context, input AlipayNotifyInput) (string, *apperror.Error) {
	f.notifyCalled = true
	return "success", nil
}

func TestHandlerRejectsInvalidPayMethodBeforeService(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
		want func(*fakeHTTPService) bool
	}{
		{
			name: "create recharge order",
			path: "/api/admin/v1/recharge-orders",
			body: `{"amount":1000,"pay_method":"bank","channel_id":1}`,
			want: func(service *fakeHTTPService) bool { return service.createRechargeOrderCalled },
		},
		{
			name: "create pay attempt",
			path: "/api/admin/v1/recharge-orders/R1/pay-attempts",
			body: `{"pay_method":"bank"}`,
			want: func(service *fakeHTTPService) bool { return service.createPayAttemptCalled },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &fakeHTTPService{}
			router := newHandlerTestRouter(service)

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, recorder.Code, recorder.Body.String())
			}
			if tt.want(service) {
				t.Fatalf("service must not be called for invalid pay_method")
			}
		})
	}
}

func TestHandlerInstallsCurrentUserWalletRuntimeRoutes(t *testing.T) {
	service := &fakeHTTPService{}
	router := newHandlerTestRouter(service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/recharge-orders?current_page=2&page_size=30", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !service.listOrdersCalled || service.listOrdersUserID != 1 || service.listOrdersQuery.CurrentPage != 2 || service.listOrdersQuery.PageSize != 30 {
		t.Fatalf("expected list current-user recharge orders route, code=%d body=%s service=%#v", recorder.Code, recorder.Body.String(), service)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/recharge-orders/R1/result", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !service.queryResultCalled || service.queryResultOrderNo != "R1" || service.queryResultUserID != 1 {
		t.Fatalf("expected query result route, code=%d body=%s service=%#v", recorder.Code, recorder.Body.String(), service)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPatch, "/api/admin/v1/recharge-orders/R1/cancel", strings.NewReader(`{"reason":"用户取消"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !service.cancelOrderCalled || service.cancelOrderOrderNo != "R1" || service.cancelOrderInput.Reason != "用户取消" {
		t.Fatalf("expected cancel order route, code=%d body=%s service=%#v", recorder.Code, recorder.Body.String(), service)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/wallet/summary", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !service.walletSummaryCalled || service.walletSummaryUserID != 1 {
		t.Fatalf("expected wallet summary route, code=%d body=%s service=%#v", recorder.Code, recorder.Body.String(), service)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/wallet/bills?current_page=3&page_size=10", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !service.walletBillsCalled || service.walletBillsUserID != 1 || service.walletBillsQuery.CurrentPage != 3 || service.walletBillsQuery.PageSize != 10 {
		t.Fatalf("expected wallet bills route, code=%d body=%s service=%#v", recorder.Code, recorder.Body.String(), service)
	}
}

func TestHandlerReturnsRawNotifyBodyEvenWhenServiceReportsError(t *testing.T) {
	service := &errorNotifyService{}
	router := newHandlerTestRouter(service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/pay/notify/alipay", strings.NewReader("out_trade_no=T1"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if strings.TrimSpace(recorder.Body.String()) != "fail" {
		t.Fatalf("expected raw fail body, got %q", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), `"code"`) {
		t.Fatalf("notify callback must not use JSON envelope: %s", recorder.Body.String())
	}
}

type errorNotifyService struct {
	fakeHTTPService
}

func (errorNotifyService) HandleAlipayNotify(ctx context.Context, input AlipayNotifyInput) (string, *apperror.Error) {
	return "fail", apperror.BadRequest("验签失败")
}

func newHandlerTestRouter(service HTTPService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	validate.MustRegister()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if c.Request.URL.Path != "/api/pay/notify/alipay" {
			c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 1, SessionID: 1, Platform: "admin"})
		}
		c.Next()
	})
	RegisterRoutes(router, service)
	return router
}
