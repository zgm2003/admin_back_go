package systemlog

import (
	"context"
	"strings"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	Files(ctx context.Context) (*FilesResponse, *apperror.Error)
	Lines(ctx context.Context, query LinesQuery) (*LinesResponse, *apperror.Error)
}

type Handler struct {
	service HTTPService
}

func NewHandler(service HTTPService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Init(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("systemlog.service_missing", nil, "系统日志服务未配置"))
		return
	}
	result, appErr := h.service.Init(c.Request.Context())
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) Files(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("systemlog.service_missing", nil, "系统日志服务未配置"))
		return
	}
	result, appErr := h.service.Files(c.Request.Context())
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) Lines(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("systemlog.service_missing", nil, "系统日志服务未配置"))
		return
	}
	var req linesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("systemlog.query.invalid", nil, "日志查询参数错误"))
		return
	}
	result, appErr := h.service.Lines(c.Request.Context(), LinesQuery{
		Filename: strings.TrimSpace(c.Param("name")),
		Tail:     req.Tail,
		Level:    strings.ToUpper(strings.TrimSpace(req.Level)),
		Keyword:  strings.TrimSpace(req.Keyword),
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}
