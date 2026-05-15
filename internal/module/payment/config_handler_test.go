package payment

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterRoutesInstallsPaymentConfigEndpointsOnly(t *testing.T) {
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
	} {
		if !routes[route.method+" "+route.path] {
			t.Fatalf("missing route %s %s", route.method, route.path)
		}
	}
	for _, retired := range []string{
		"GET /api/admin/v1/payment/channels",
		"GET /api/admin/v1/payment/orders",
		"GET /api/admin/v1/payment/events",
		"POST /api/payment/notify/alipay",
	} {
		if routes[retired] {
			t.Fatalf("retired payment route still registered: %s", retired)
		}
	}
}
