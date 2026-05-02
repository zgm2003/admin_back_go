package system

import (
	"admin_back_go/internal/apperror"
	"admin_back_go/internal/readiness"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
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
		response.ErrorWithData(c, apperror.Internal("service not ready"), report)
		return
	}
	response.OK(c, report)
}
