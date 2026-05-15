package payment

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterRoutesInstallsPaymentConfigAndOrderEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, nil)

	routes := map[string]bool{}
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/admin/v1/payment/configs/page-init"},
		{http.MethodGet, "/api/admin/v1/payment/configs"},
		{http.MethodPost, "/api/admin/v1/payment/configs"},
		{http.MethodPut, "/api/admin/v1/payment/configs/:id"},
		{http.MethodPatch, "/api/admin/v1/payment/configs/:id/status"},
		{http.MethodDelete, "/api/admin/v1/payment/configs/:id"},
		{http.MethodPost, "/api/admin/v1/payment/configs/:id/test"},
		{http.MethodPost, "/api/admin/v1/payment/certificates"},
		{http.MethodGet, "/api/admin/v1/payment/orders/page-init"},
		{http.MethodGet, "/api/admin/v1/payment/orders"},
		{http.MethodGet, "/api/admin/v1/payment/orders/:id"},
		{http.MethodPost, "/api/admin/v1/payment/orders"},
		{http.MethodPost, "/api/admin/v1/payment/orders/:id/pay"},
		{http.MethodPost, "/api/admin/v1/payment/orders/:id/sync"},
		{http.MethodPatch, "/api/admin/v1/payment/orders/:id/close"},
		{http.MethodGet, "/api/admin/v1/payment/recharges/page-init"},
		{http.MethodGet, "/api/admin/v1/payment/recharges"},
		{http.MethodGet, "/api/admin/v1/payment/recharges/:id"},
		{http.MethodPost, "/api/admin/v1/payment/recharges"},
		{http.MethodPost, "/api/admin/v1/payment/recharges/:id/pay"},
		{http.MethodPost, "/api/admin/v1/payment/recharges/:id/sync"},
		{http.MethodPatch, "/api/admin/v1/payment/recharges/:id/close"},
	} {
		if !routes[route.method+" "+route.path] {
			t.Fatalf("missing route %s %s", route.method, route.path)
		}
	}
	for _, retired := range []string{
		"GET /api/admin/v1/payment/channels",
		"GET /api/admin/v1/payment/events",
		"GET /api/admin/v1/payment/" + "order",
		"POST /api/payment/notify/alipay",
	} {
		if routes[retired] {
			t.Fatalf("retired payment route still registered: %s", retired)
		}
	}
}
