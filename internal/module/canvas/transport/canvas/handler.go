package canvas

import (
	"context"

	"admin_back_go/internal/middleware"
	canvasmodule "admin_back_go/internal/module/canvas"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type HTTPService interface {
	PublicPrompts(ctx context.Context, query canvasmodule.PromptListQuery) (*canvasmodule.PromptListResponse, *apperror.Error)
	PublicAssets(ctx context.Context, query canvasmodule.AssetListQuery) (*canvasmodule.AssetListResponse, *apperror.Error)
	PublicSettings(ctx context.Context, input canvasmodule.SettingsInput) (*canvasmodule.SettingsResponse, *apperror.Error)
}

type Handler struct{ service HTTPService }

func NewHandler(service HTTPService) *Handler { return &Handler{service: service} }

func (h *Handler) Prompts(c *gin.Context) {
	var req listPromptsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("canvas.prompt.request.invalid", nil, "提示词列表参数错误"))
		return
	}
	result, appErr := h.requireService().PublicPrompts(c.Request.Context(), canvasmodule.PromptListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, Keyword: req.Keyword, Category: req.Category, Tags: req.Tag})
	writeResult(c, result, appErr)
}

func (h *Handler) Assets(c *gin.Context) {
	var req listAssetsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("canvas.asset.request.invalid", nil, "素材列表参数错误"))
		return
	}
	result, appErr := h.requireService().PublicAssets(c.Request.Context(), canvasmodule.AssetListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, Keyword: req.Keyword, Type: req.Type})
	writeResult(c, result, appErr)
}

func (h *Handler) Settings(c *gin.Context) {
	var userID int64
	if identity := middleware.GetAuthIdentity(c); identity != nil {
		userID = identity.UserID
	}
	result, appErr := h.requireService().PublicSettings(c.Request.Context(), canvasmodule.SettingsInput{UserID: userID})
	writeResult(c, result, appErr)
}

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return failingService{}
	}
	return h.service
}

type failingService struct{}

func (failingService) PublicPrompts(ctx context.Context, query canvasmodule.PromptListQuery) (*canvasmodule.PromptListResponse, *apperror.Error) {
	return nil, apperror.InternalKey("canvas.service_missing", nil, "Canvas服务未配置")
}
func (failingService) PublicAssets(ctx context.Context, query canvasmodule.AssetListQuery) (*canvasmodule.AssetListResponse, *apperror.Error) {
	return nil, apperror.InternalKey("canvas.service_missing", nil, "Canvas服务未配置")
}
func (failingService) PublicSettings(ctx context.Context, input canvasmodule.SettingsInput) (*canvasmodule.SettingsResponse, *apperror.Error) {
	return nil, apperror.InternalKey("canvas.service_missing", nil, "Canvas服务未配置")
}

func writeResult(c *gin.Context, result any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}
