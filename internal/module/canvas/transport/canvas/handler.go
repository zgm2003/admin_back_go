package canvas

import (
	"context"
	"net/http"
	"strconv"

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
	ChatCompletion(ctx context.Context, input canvasmodule.ChatCompletionInput) (*canvasmodule.ChatCompletionResponse, *apperror.Error)
	GenerateImage(ctx context.Context, input canvasmodule.ImageGenerationInput) (*canvasmodule.ImageGenerationResponse, *apperror.Error)
	GenerateVideo(ctx context.Context, input canvasmodule.VideoGenerationInput) (*canvasmodule.VideoGenerationResponse, *apperror.Error)
	VideoStatus(ctx context.Context, userID int64, id int64) (*canvasmodule.VideoStatusResponse, *apperror.Error)
	VideoContent(ctx context.Context, userID int64, id int64) ([]byte, string, *apperror.Error)
}

type Handler struct{ service HTTPService }

func NewHandler(service HTTPService) *Handler { return &Handler{service: service} }

func (h *Handler) Prompts(c *gin.Context) {
	var req listPromptsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("canvas.prompt.request.invalid", nil, "提示词列表参数错误"))
		return
	}
	result, appErr := h.requireService().PublicPrompts(c.Request.Context(), canvasmodule.PromptListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, Keyword: req.Keyword, Category: req.Category})
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

func (h *Handler) ChatCompletions(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req chatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("canvas.ai.chat.request.invalid", nil, "文本生成参数错误"))
		return
	}
	result, appErr := h.requireService().ChatCompletion(c.Request.Context(), canvasmodule.ChatCompletionInput{UserID: userID, AgentID: req.AgentID, ModelID: req.ModelID, Message: req.Message})
	writeResult(c, result, appErr)
}

func (h *Handler) ImageGenerations(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req imageGenerationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("canvas.ai.image.request.invalid", nil, "图片生成参数错误"))
		return
	}
	result, appErr := h.requireService().GenerateImage(c.Request.Context(), canvasmodule.ImageGenerationInput{
		UserID: userID, AgentID: req.AgentID, Prompt: req.Prompt, Size: req.Size, Quality: req.Quality,
		OutputFormat: req.OutputFormat, OutputCompression: req.OutputCompression, Moderation: req.Moderation,
		N: req.N, InputAssetIDs: req.InputAssetIDs, MaskAssetID: req.MaskAssetID, MaskTargetAssetID: req.MaskTargetAssetID,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) ImageEdits(c *gin.Context) {
	h.ImageGenerations(c)
}

func (h *Handler) VideoGenerations(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req videoGenerationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("canvas.ai.video.request.invalid", nil, "视频生成参数错误"))
		return
	}
	result, appErr := h.requireService().GenerateVideo(c.Request.Context(), canvasmodule.VideoGenerationInput{
		UserID: userID, AgentID: req.AgentID, ModelID: req.ModelID, Prompt: req.Prompt,
		DurationSeconds: req.DurationSeconds, Size: req.Size, ResolutionName: req.ResolutionName,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) VideoStatus(c *gin.Context) {
	userID, id, ok := currentUserIDAndRouteID(c)
	if !ok {
		return
	}
	result, appErr := h.requireService().VideoStatus(c.Request.Context(), userID, id)
	writeResult(c, result, appErr)
}

func (h *Handler) VideoContent(c *gin.Context) {
	userID, id, ok := currentUserIDAndRouteID(c)
	if !ok {
		return
	}
	body, contentType, appErr := h.requireService().VideoContent(c.Request.Context(), userID, id)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Data(http.StatusOK, contentType, body)
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
func (failingService) ChatCompletion(ctx context.Context, input canvasmodule.ChatCompletionInput) (*canvasmodule.ChatCompletionResponse, *apperror.Error) {
	return nil, apperror.InternalKey("canvas.service_missing", nil, "Canvas服务未配置")
}
func (failingService) GenerateImage(ctx context.Context, input canvasmodule.ImageGenerationInput) (*canvasmodule.ImageGenerationResponse, *apperror.Error) {
	return nil, apperror.InternalKey("canvas.service_missing", nil, "Canvas服务未配置")
}
func (failingService) GenerateVideo(ctx context.Context, input canvasmodule.VideoGenerationInput) (*canvasmodule.VideoGenerationResponse, *apperror.Error) {
	return nil, apperror.InternalKey("canvas.service_missing", nil, "Canvas服务未配置")
}
func (failingService) VideoStatus(ctx context.Context, userID int64, id int64) (*canvasmodule.VideoStatusResponse, *apperror.Error) {
	return nil, apperror.InternalKey("canvas.service_missing", nil, "Canvas服务未配置")
}
func (failingService) VideoContent(ctx context.Context, userID int64, id int64) ([]byte, string, *apperror.Error) {
	return nil, "", apperror.InternalKey("canvas.service_missing", nil, "Canvas服务未配置")
}

func currentUserID(c *gin.Context) (int64, bool) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return 0, false
	}
	return identity.UserID, true
}

func currentUserIDAndRouteID(c *gin.Context) (int64, int64, bool) {
	userID, ok := currentUserID(c)
	if !ok {
		return 0, 0, false
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequestKey("canvas.ai.video.id.invalid", nil, "视频任务ID无效"))
		return 0, 0, false
	}
	return userID, id, true
}

func writeResult(c *gin.Context, result any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}
