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
	notifyCalled              bool
}

func (f *fakeHTTPService) CreateRechargeOrder(ctx context.Context, userID int64, input RechargeOrderCreateInput) (*RechargeOrderCreateResponse, *apperror.Error) {
	f.createRechargeOrderCalled = true
	return &RechargeOrderCreateResponse{}, nil
}

func (f *fakeHTTPService) CreatePayAttempt(ctx context.Context, userID int64, orderNo string, input PayAttemptCreateInput) (*PayAttemptCreateResponse, *apperror.Error) {
	f.createPayAttemptCalled = true
	return &PayAttemptCreateResponse{}, nil
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
