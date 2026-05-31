package canvas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/middleware"
	paymentmodule "admin_back_go/internal/module/payment"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"

	"github.com/gin-gonic/gin"
)

func TestCanvasRechargePageInitUsesCurrentCanvasUser(t *testing.T) {
	router, service := newCanvasRechargeTestRouter(&middleware.AuthIdentity{UserID: 8, Platform: enum.PlatformCanvas})

	requestCanvasRecharge(t, router, http.MethodGet, "/api/canvas/v1/payment/recharges/page-init", "")

	if service.pageInitUserID != 8 {
		t.Fatalf("expected current canvas user id 8, got %d", service.pageInitUserID)
	}
}

func TestCanvasRechargeListForcesCurrentCanvasUser(t *testing.T) {
	router, service := newCanvasRechargeTestRouter(&middleware.AuthIdentity{UserID: 8, Platform: enum.PlatformCanvas})

	requestCanvasRecharge(t, router, http.MethodGet, "/api/canvas/v1/payment/recharges?current_page=2&page_size=10&user_id=999&status=paying", "")

	query := service.listQuery
	if query.UserID != 8 || query.CurrentPage != 2 || query.PageSize != 10 || query.Status != "paying" {
		t.Fatalf("unexpected recharge list query: %#v", query)
	}
}

func TestCanvasRechargeCreateForcesCurrentCanvasUser(t *testing.T) {
	router, service := newCanvasRechargeTestRouter(&middleware.AuthIdentity{UserID: 8, Platform: enum.PlatformCanvas})

	requestCanvasRecharge(t, router, http.MethodPost, "/api/canvas/v1/payment/recharges", `{"user_id":999,"package_code":"recharge_10","pay_method":"web","return_url":"https://canvas.example.test/recharge"}`)

	input := service.createInput
	if input.UserID != 8 || input.PackageCode != "recharge_10" || input.PayMethod != "web" || input.ReturnURL != "https://canvas.example.test/recharge" {
		t.Fatalf("unexpected recharge create input: %#v", input)
	}
}

func TestCanvasRechargePayUsesCurrentCanvasUserAndRouteID(t *testing.T) {
	router, service := newCanvasRechargeTestRouter(&middleware.AuthIdentity{UserID: 8, Platform: enum.PlatformCanvas})

	requestCanvasRecharge(t, router, http.MethodPost, "/api/canvas/v1/payment/recharges/23/pay", "")

	if service.payUserID != 8 || service.payID != 23 {
		t.Fatalf("unexpected recharge pay args: userID=%d id=%d", service.payUserID, service.payID)
	}
}

func TestCanvasRechargeRejectsAdminPlatformIdentity(t *testing.T) {
	router, _ := newCanvasRechargeTestRouter(&middleware.AuthIdentity{UserID: 8, Platform: enum.PlatformAdmin})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/canvas/v1/payment/recharges/page-init", nil))

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for admin token on canvas recharge route, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func newCanvasRechargeTestRouter(identity *middleware.AuthIdentity) (*gin.Engine, *fakeCanvasRechargeService) {
	gin.SetMode(gin.TestMode)
	service := &fakeCanvasRechargeService{}
	router := gin.New()
	if identity != nil {
		router.Use(func(c *gin.Context) {
			c.Set(middleware.ContextAuthIdentity, identity)
			c.Next()
		})
	}
	RegisterRechargeRoutes(router, service)
	return router, service
}

func requestCanvasRecharge(t *testing.T, router *gin.Engine, method string, path string, body string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, path, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("%s %s expected 200, got %d body=%s", method, path, recorder.Code, recorder.Body.String())
	}
}

type fakeCanvasRechargeService struct {
	pageInitUserID int64
	listQuery      paymentmodule.RechargeListQuery
	createInput    paymentmodule.RechargeCreateInput
	payUserID      int64
	payID          int64
}

func (f *fakeCanvasRechargeService) RechargePageInit(ctx context.Context, userID int64) (*paymentmodule.RechargePageInitResponse, *apperror.Error) {
	f.pageInitUserID = userID
	return &paymentmodule.RechargePageInitResponse{}, nil
}

func (f *fakeCanvasRechargeService) ListRecharges(ctx context.Context, query paymentmodule.RechargeListQuery) (*paymentmodule.RechargeListResponse, *apperror.Error) {
	f.listQuery = query
	return &paymentmodule.RechargeListResponse{}, nil
}

func (f *fakeCanvasRechargeService) CreateRecharge(ctx context.Context, input paymentmodule.RechargeCreateInput) (*paymentmodule.RechargePayResponse, *apperror.Error) {
	f.createInput = input
	return &paymentmodule.RechargePayResponse{ID: 1, Status: "paying"}, nil
}

func (f *fakeCanvasRechargeService) PayRecharge(ctx context.Context, userID int64, id int64) (*paymentmodule.RechargePayResponse, *apperror.Error) {
	f.payUserID = userID
	f.payID = id
	return &paymentmodule.RechargePayResponse{ID: id, Status: "paying"}, nil
}
