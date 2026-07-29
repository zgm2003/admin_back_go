package admin

import (
	"net/http"

	airunmodule "admin_back_go/internal/module/ai/run"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func Register(router *gin.Engine, service airunmodule.HTTPService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service)
	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-runs/page-init",
		Access: adminroute.Permission("ai_run_list"),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.PageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-runs",
		Access: adminroute.Permission("ai_run_list"),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.List)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-runs/stats",
		Access: adminroute.Permission("ai_run_list"),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.Stats)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-runs/stats/latency",
		Access: adminroute.Permission("ai_run_list"),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.LatencyStats)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-runs/stats/by-date",
		Access: adminroute.Permission("ai_run_list"),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.StatsByDate)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-runs/stats/by-agent",
		Access: adminroute.Permission("ai_run_list"),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.StatsByAgent)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-runs/stats/by-user",
		Access: adminroute.Permission("ai_run_list"),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.StatsByUser)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/ai-runs/:id",
		Access: adminroute.Permission("ai_run_list"),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.Detail)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/ai-runs/:id/user-feedback",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("self-service AI run feedback"),
	}, handler.SetUserFeedback)
}
