package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin_back_go/internal/middleware"
	walletmodule "admin_back_go/internal/module/payment/wallet"
	"admin_back_go/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

func TestHandlerPaymentLedgerPageInitWorks(t *testing.T) {
	router, service := newWalletAdminTestRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/payment/ledger/page-init", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !service.ledgerPageInitCalled {
		t.Fatalf("ledger page-init service was not called")
	}
}

func TestHandlerPaymentLedgerPassesFilters(t *testing.T) {
	router, service := newWalletAdminTestRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/payment/ledger?current_page=2&page_size=25&user_id=42&direction=out&source_type=recharge&date_start=2026-05-01&date_end=2026-05-30", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	query := service.ledgerQuery
	if query.CurrentPage != 2 || query.PageSize != 25 || query.UserID != 42 || query.Direction != "out" || query.SourceType != "recharge" || query.DateStart != "2026-05-01" || query.DateEnd != "2026-05-30" {
		t.Fatalf("unexpected ledger query=%#v", query)
	}
}

func TestHandlerPaymentLedgerDoesNotRequireCurrentIdentity(t *testing.T) {
	router, service := newWalletAdminTestRouterWithoutIdentity()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/payment/ledger", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.ledgerQuery.UserID != 0 {
		t.Fatalf("admin ledger must not force a current user id, query=%#v", service.ledgerQuery)
	}
}

func TestHandlerPaymentWalletsPageInitWorks(t *testing.T) {
	router, service := newWalletAdminTestRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/payment/wallets/page-init", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !service.walletUsersPageInitCalled {
		t.Fatalf("wallet users page-init service was not called")
	}
}

func TestHandlerPaymentWalletsPassesFilters(t *testing.T) {
	router, service := newWalletAdminTestRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/payment/wallets?current_page=3&page_size=15&keyword=alice&user_id=99", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	query := service.walletUsersQuery
	if query.CurrentPage != 3 || query.PageSize != 15 || query.Keyword != "alice" || query.UserID != 99 {
		t.Fatalf("unexpected wallet users query=%#v", query)
	}
}

func TestHandlerPaymentWalletsDoesNotRequireCurrentIdentity(t *testing.T) {
	router, service := newWalletAdminTestRouterWithoutIdentity()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/payment/wallets", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.walletUsersQuery.UserID != 0 {
		t.Fatalf("admin wallet list must not force a current user id, query=%#v", service.walletUsersQuery)
	}
}

func TestHandlerWalletSummaryUsesCurrentIdentity(t *testing.T) {
	router, service := newWalletAdminTestRouter()

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

func TestHandlerWalletTransactionsForcesCurrentIdentity(t *testing.T) {
	router, service := newWalletAdminTestRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/wallet/transactions?current_page=4&page_size=10&user_id=999&keyword=ignored&direction=in&source_type=ai_generate&date_start=2026-05-01&date_end=2026-05-30", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	query := service.transactionsQuery
	if query.UserID != 7 || query.CurrentPage != 4 || query.PageSize != 10 || query.Keyword != "ignored" || query.Direction != "in" || query.SourceType != "ai_generate" || query.DateStart != "2026-05-01" || query.DateEnd != "2026-05-30" {
		t.Fatalf("unexpected transactions query=%#v", query)
	}
}

func TestHandlerWalletConsumptionsRouteRetired(t *testing.T) {
	router, _ := newWalletAdminTestRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/wallet/consumptions", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func newWalletAdminTestRouter() (*gin.Engine, *fakeHTTPService) {
	gin.SetMode(gin.TestMode)
	service := &fakeHTTPService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 7, SessionID: 1, Platform: "admin"})
	})
	RegisterRoutes(router, service)
	return router, service
}

func newWalletAdminTestRouterWithoutIdentity() (*gin.Engine, *fakeHTTPService) {
	gin.SetMode(gin.TestMode)
	service := &fakeHTTPService{}
	router := gin.New()
	RegisterRoutes(router, service)
	return router, service
}

type fakeHTTPService struct {
	summaryUserID             int64
	transactionsQuery         walletmodule.TransactionListQuery
	walletUsersPageInitCalled bool
	walletUsersQuery          walletmodule.WalletUserListQuery
	ledgerPageInitCalled      bool
	ledgerQuery               walletmodule.TransactionListQuery
}

func (f *fakeHTTPService) Summary(ctx context.Context, userID int64) (*walletmodule.SummaryResponse, *apperror.Error) {
	f.summaryUserID = userID
	return &walletmodule.SummaryResponse{Balance: "0"}, nil
}
func (f *fakeHTTPService) Transactions(ctx context.Context, query walletmodule.TransactionListQuery) (*walletmodule.TransactionListResponse, *apperror.Error) {
	f.transactionsQuery = query
	return &walletmodule.TransactionListResponse{}, nil
}
func (f *fakeHTTPService) WalletUsersPageInit(ctx context.Context) (*walletmodule.WalletUsersPageInitResponse, *apperror.Error) {
	f.walletUsersPageInitCalled = true
	return &walletmodule.WalletUsersPageInitResponse{}, nil
}
func (f *fakeHTTPService) WalletUsers(ctx context.Context, query walletmodule.WalletUserListQuery) (*walletmodule.WalletUserListResponse, *apperror.Error) {
	f.walletUsersQuery = query
	return &walletmodule.WalletUserListResponse{}, nil
}
func (f *fakeHTTPService) LedgerPageInit(ctx context.Context) (*walletmodule.LedgerPageInitResponse, *apperror.Error) {
	f.ledgerPageInitCalled = true
	return &walletmodule.LedgerPageInitResponse{}, nil
}
func (f *fakeHTTPService) Ledger(ctx context.Context, query walletmodule.TransactionListQuery) (*walletmodule.TransactionListResponse, *apperror.Error) {
	f.ledgerQuery = query
	return &walletmodule.TransactionListResponse{}, nil
}
