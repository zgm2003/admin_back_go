package canvas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin_back_go/internal/middleware"
	walletmodule "admin_back_go/internal/module/payment/wallet"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"

	"github.com/gin-gonic/gin"
)

func TestCanvasWalletSummaryUsesCurrentCanvasUser(t *testing.T) {
	router, service := newCanvasWalletTestRouter(&middleware.AuthIdentity{UserID: 8, Platform: enum.PlatformCanvas})

	requestCanvasWallet(t, router, http.MethodGet, "/api/canvas/v1/wallet/summary")

	if service.summaryUserID != 8 {
		t.Fatalf("expected current canvas user id 8, got %d", service.summaryUserID)
	}
}

func TestCanvasWalletTransactionsForcesCurrentCanvasUser(t *testing.T) {
	router, service := newCanvasWalletTestRouter(&middleware.AuthIdentity{UserID: 8, Platform: enum.PlatformCanvas})

	requestCanvasWallet(t, router, http.MethodGet, "/api/canvas/v1/wallet/transactions?current_page=3&page_size=20&user_id=999&keyword=WLT&direction=in&source_type=ai_generate")

	query := service.transactionsQuery
	if query.UserID != 8 || query.CurrentPage != 3 || query.PageSize != 20 || query.Keyword != "WLT" || query.Direction != "in" || query.SourceType != "ai_generate" {
		t.Fatalf("unexpected wallet transactions query: %#v", query)
	}
}

func TestCanvasWalletRejectsAdminPlatformIdentity(t *testing.T) {
	router, _ := newCanvasWalletTestRouter(&middleware.AuthIdentity{UserID: 8, Platform: enum.PlatformAdmin})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/canvas/v1/wallet/summary", nil))

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for admin token on canvas wallet route, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func newCanvasWalletTestRouter(identity *middleware.AuthIdentity) (*gin.Engine, *fakeCanvasWalletService) {
	gin.SetMode(gin.TestMode)
	service := &fakeCanvasWalletService{}
	router := gin.New()
	if identity != nil {
		router.Use(func(c *gin.Context) {
			c.Set(middleware.ContextAuthIdentity, identity)
			c.Next()
		})
	}
	RegisterRoutes(router, service)
	return router, service
}

func requestCanvasWallet(t *testing.T, router *gin.Engine, method string, path string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("%s %s expected 200, got %d body=%s", method, path, recorder.Code, recorder.Body.String())
	}
}

type fakeCanvasWalletService struct {
	summaryUserID     int64
	transactionsQuery walletmodule.TransactionListQuery
}

func (f *fakeCanvasWalletService) Summary(ctx context.Context, userID int64) (*walletmodule.SummaryResponse, *apperror.Error) {
	f.summaryUserID = userID
	return &walletmodule.SummaryResponse{}, nil
}

func (f *fakeCanvasWalletService) Transactions(ctx context.Context, query walletmodule.TransactionListQuery) (*walletmodule.TransactionListResponse, *apperror.Error) {
	f.transactionsQuery = query
	return &walletmodule.TransactionListResponse{}, nil
}
