package admin

import (
	systemmodule "admin_back_go/internal/module/system"
	"admin_back_go/internal/readiness"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *systemmodule.Service
}

func NewHandler(service *systemmodule.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Health(c *gin.Context) {
	response.OK(c, h.service.Health())
}

func (h *Handler) Ping(c *gin.Context) {
	response.OK(c, h.service.Ping())
}

func (h *Handler) Ready(c *gin.Context) {
	report := h.service.Ready(c.Request.Context())
	if report.Status != readiness.StatusReady {
		response.ErrorWithData(c, apperror.InternalKey("system.ready.not_ready", nil, "service not ready"), report)
		return
	}
	response.OK(c, report)
}
