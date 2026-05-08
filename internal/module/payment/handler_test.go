package payment

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterRoutesInstallsPaymentEndpoints(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	RegisterRoutes(router, nil)

	routes := map[string]struct{}{}
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}

	expected := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/admin/v1/payment/channels/page-init"},
		{http.MethodGet, "/api/admin/v1/payment/channels"},
		{http.MethodPost, "/api/admin/v1/payment/channels"},
		{http.MethodPut, "/api/admin/v1/payment/channels/:id"},
		{http.MethodPatch, "/api/admin/v1/payment/channels/:id/status"},
		{http.MethodDelete, "/api/admin/v1/payment/channels/:id"},
		{http.MethodGet, "/api/admin/v1/payment/orders/page-init"},
		{http.MethodGet, "/api/admin/v1/payment/orders"},
		{http.MethodPost, "/api/admin/v1/payment/orders"},
		{http.MethodGet, "/api/admin/v1/payment/orders/:order_no/result"},
		{http.MethodPost, "/api/admin/v1/payment/orders/:order_no/pay"},
		{http.MethodPatch, "/api/admin/v1/payment/orders/:order_no/cancel"},
		{http.MethodGet, "/api/admin/v1/payment/orders/:order_no"},
		{http.MethodPatch, "/api/admin/v1/payment/orders/:order_no/close"},
		{http.MethodGet, "/api/admin/v1/payment/events"},
		{http.MethodGet, "/api/admin/v1/payment/events/:id"},
		{http.MethodPost, "/api/payment/notify/alipay"},
	}

	for _, tt := range expected {
		key := tt.method + " " + tt.path
		if _, ok := routes[key]; !ok {
			t.Fatalf("missing route %s; got %#v", key, routes)
		}
	}
}
