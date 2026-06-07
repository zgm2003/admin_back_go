package canvas

import (
	"context"

	promptmodule "admin_back_go/internal/module/ai/prompt"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type HTTPService interface {
	PublicList(ctx context.Context, query promptmodule.ListQuery) (*promptmodule.ListResponse, *apperror.Error)
}

type Handler struct{ service HTTPService }

func NewHandler(service HTTPService) *Handler { return &Handler{service: service} }

func (h *Handler) Prompts(c *gin.Context) {
	var req listPromptsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("ai.prompt.request.invalid", nil, "提示词列表参数错误"))
		return
	}
	result, appErr := h.requireService().PublicList(c.Request.Context(), promptmodule.ListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, Keyword: req.Keyword, Category: req.Category, Tags: req.Tag})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return failingService{}
	}
	return h.service
}

type failingService struct{}

func (failingService) PublicList(ctx context.Context, query promptmodule.ListQuery) (*promptmodule.ListResponse, *apperror.Error) {
	return nil, apperror.InternalKey("ai.prompt.service_missing", nil, "AI提示词服务未配置")
}
