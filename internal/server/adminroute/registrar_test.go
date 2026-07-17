package adminroute

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegistrarAddsPolicyAndHandlerAtomically(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registry := NewRegistry()
	routes := NewRegistrar(engine, registry)
	routes.Handle(Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/widgets",
		Access: Permission("widget_list"),
		Audit:  NoAudit("read-only"),
	}, func(c *gin.Context) { c.Status(http.StatusNoContent) })

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/v1/widgets", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d", recorder.Code)
	}
	definitions := registry.Definitions()
	if len(definitions) != 1 || definitions[0].Access.PermissionCode != "widget_list" {
		t.Fatalf("definitions = %#v", definitions)
	}
}
