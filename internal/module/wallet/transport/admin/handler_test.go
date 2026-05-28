package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/middleware"
	walletmodule "admin_back_go/internal/module/wallet"
	"admin_back_go/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

func TestHandlerRoutesUseCurrentIdentityForWalletCenter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeHTTPService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 7, SessionID: 1, Platform: "admin"})
	})
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/wallet/summary", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.summaryUserID != 7 {
		t.Fatalf("expected current user id 7, got %d", service.summaryUserID)
	}
}

func TestHandlerConsumeValidatesRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeHTTPService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 7, SessionID: 1, Platform: "admin"})
	})
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/wallet/consumptions", strings.NewReader(`{"amount_cents":0,"source_id":1}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.consumeCalled {
		t.Fatalf("consume service should not be called for invalid request")
	}
}

func TestHandlerConsumeUsesCurrentIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeHTTPService{consumeResponse: &walletmodule.ConsumeResponse{Wallet: walletmodule.SummaryResponse{BalanceCents: 900}}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 7, SessionID: 1, Platform: "admin"})
	})
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/wallet/consumptions", strings.NewReader(`{"amount_cents":100,"source_id":3,"remark":" test "}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !service.consumeCalled || service.consumeInput.UserID != 7 || service.consumeInput.AmountCents != 100 || service.consumeInput.SourceID != 3 {
		t.Fatalf("unexpected consume input=%#v called=%v", service.consumeInput, service.consumeCalled)
	}
	var body struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body.Code != 0 {
		t.Fatalf("unexpected response body=%s err=%v", recorder.Body.String(), err)
	}
}

type fakeHTTPService struct {
	summaryUserID   int64
	consumeCalled   bool
	consumeInput    walletmodule.ConsumeInput
	consumeResponse *walletmodule.ConsumeResponse
}

func (f *fakeHTTPService) Summary(ctx context.Context, userID int64) (*walletmodule.SummaryResponse, *apperror.Error) {
	f.summaryUserID = userID
	return &walletmodule.SummaryResponse{BalanceCents: 0, BalanceText: "0.00"}, nil
}
func (f *fakeHTTPService) Transactions(ctx context.Context, query walletmodule.TransactionListQuery) (*walletmodule.TransactionListResponse, *apperror.Error) {
	return &walletmodule.TransactionListResponse{}, nil
}
func (f *fakeHTTPService) Consume(ctx context.Context, input walletmodule.ConsumeInput) (*walletmodule.ConsumeResponse, *apperror.Error) {
	f.consumeCalled = true
	f.consumeInput = input
	if f.consumeResponse != nil {
		return f.consumeResponse, nil
	}
	return &walletmodule.ConsumeResponse{}, nil
}
func (f *fakeHTTPService) WalletUsersPageInit(ctx context.Context) (*walletmodule.WalletUsersPageInitResponse, *apperror.Error) {
	return &walletmodule.WalletUsersPageInitResponse{}, nil
}
func (f *fakeHTTPService) WalletUsers(ctx context.Context, query walletmodule.WalletUserListQuery) (*walletmodule.WalletUserListResponse, *apperror.Error) {
	return &walletmodule.WalletUserListResponse{}, nil
}
func (f *fakeHTTPService) LedgerPageInit(ctx context.Context) (*walletmodule.LedgerPageInitResponse, *apperror.Error) {
	return &walletmodule.LedgerPageInitResponse{}, nil
}
func (f *fakeHTTPService) Ledger(ctx context.Context, query walletmodule.TransactionListQuery) (*walletmodule.TransactionListResponse, *apperror.Error) {
	return &walletmodule.TransactionListResponse{}, nil
}
