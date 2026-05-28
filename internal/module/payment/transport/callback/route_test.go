package callback

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterRoutesInstallsAlipayCallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, nil)

	routes := map[string]bool{}
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	if !routes[http.MethodPost+" /api/payment/callbacks/alipay"] {
		t.Fatalf("missing route POST /api/payment/callbacks/alipay")
	}
}
