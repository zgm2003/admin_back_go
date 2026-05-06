package wallet

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/middleware"

	"github.com/gin-gonic/gin"
)

type fakeHTTPService struct {
	called          string
	listQuery       ListQuery
	txnQuery        TransactionListQuery
	adjustmentInput CreateAdjustmentInput
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

func (f *fakeHTTPService) Transactions(ctx context.Context, query TransactionListQuery) (*TransactionListResponse, *apperror.Error) {
	f.called = "transactions"
	f.txnQuery = query
	return &TransactionListResponse{List: []TransactionItem{}, Page: Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize}}, nil
}

func (f *fakeHTTPService) CreateAdjustment(ctx context.Context, input CreateAdjustmentInput) (*WalletAdjustmentCreateResponse, *apperror.Error) {
	f.called = "create_adjustment"
	f.adjustmentInput = input
	return &WalletAdjustmentCreateResponse{TransactionID: 8, BizActionNo: "WALLET:ADJUST:idem-0001", BalanceBefore: 1000, BalanceAfter: 1100}, nil
}

func TestHandlerRoutes(t *testing.T) {
	router, service := newWalletHandlerRouter()

	cases := []struct {
		path string
		want string
	}{
		{"/api/admin/v1/wallets/page-init", "init"},
		{"/api/admin/v1/wallets?current_page=2&page_size=30&user_id=7&start_date=2026-05-01&end_date=2026-05-06", "list"},
		{"/api/admin/v1/wallet-transactions?current_page=3&page_size=10&user_id=7&type=3&start_date=2026-05-01&end_date=2026-05-06", "transactions"},
	}
	for _, tc := range cases {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, tc.path, nil)
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || service.called != tc.want {
			t.Fatalf("%s expected called=%s status=%d, got called=%s status=%d body=%s", tc.path, tc.want, http.StatusOK, service.called, recorder.Code, recorder.Body.String())
		}
	}

	if service.listQuery.CurrentPage != 2 || service.listQuery.PageSize != 30 || service.listQuery.UserID == nil || *service.listQuery.UserID != 7 || service.listQuery.StartDate != "2026-05-01" || service.listQuery.EndDate != "2026-05-06" {
		t.Fatalf("unexpected wallet list query: %#v", service.listQuery)
	}
	if service.txnQuery.CurrentPage != 3 || service.txnQuery.PageSize != 10 || service.txnQuery.UserID == nil || *service.txnQuery.UserID != 7 || service.txnQuery.Type == nil || *service.txnQuery.Type != enum.WalletTypeAdjust {
		t.Fatalf("unexpected wallet transaction query: %#v", service.txnQuery)
	}
}

func TestHandlerRejectsInvalidWalletTypeBeforeService(t *testing.T) {
	router, service := newWalletHandlerRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/wallet-transactions?current_page=1&page_size=20&type=999", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, recorder.Code, recorder.Body.String())
	}
	if service.called != "" {
		t.Fatalf("service should not be called for invalid wallet type")
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["code"] != float64(apperror.CodeBadRequest) {
		t.Fatalf("unexpected response payload: %#v", payload)
	}
}

func TestHandlerCreateAdjustmentUsesAuthIdentity(t *testing.T) {
	router, service := newWalletHandlerRouterWithIdentity(&middleware.AuthIdentity{UserID: 3, SessionID: 9, Platform: "admin"})
	body := strings.NewReader(`{"user_id":7,"delta":100,"reason":"人工修正","idempotency_key":"idem-0001"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/wallet-adjustments", body)
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || service.called != "create_adjustment" {
		t.Fatalf("expected create adjustment, status=%d called=%s body=%s", recorder.Code, service.called, recorder.Body.String())
	}
	if service.adjustmentInput.UserID != 7 ||
		service.adjustmentInput.Delta != 100 ||
		service.adjustmentInput.OperatorID != 3 ||
		service.adjustmentInput.IdempotencyKey != "idem-0001" {
		t.Fatalf("unexpected input: %#v", service.adjustmentInput)
	}
}

func TestHandlerCreateAdjustmentRequiresAuthIdentity(t *testing.T) {
	router, service := newWalletHandlerRouterWithIdentity(nil)
	body := strings.NewReader(`{"user_id":7,"delta":100,"reason":"人工修正","idempotency_key":"idem-0001"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/wallet-adjustments", body)
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.called != "" {
		t.Fatalf("service should not be called")
	}
}

func newWalletHandlerRouter() (*gin.Engine, *fakeHTTPService) {
	gin.SetMode(gin.TestMode)
	service := &fakeHTTPService{}
	router := gin.New()
	RegisterRoutes(router, service)
	return router, service
}

func newWalletHandlerRouterWithIdentity(identity *middleware.AuthIdentity) (*gin.Engine, *fakeHTTPService) {
	gin.SetMode(gin.TestMode)
	service := &fakeHTTPService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if identity != nil {
			c.Set(middleware.ContextAuthIdentity, identity)
		}
		c.Next()
	})
	RegisterRoutes(router, service)
	return router, service
}
