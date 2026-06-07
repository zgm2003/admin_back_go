package admin

import (
	"context"
	"strconv"

	"admin_back_go/internal/middleware"
	aiimagemodule "admin_back_go/internal/module/ai/image"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type Handler struct{ service aiimagemodule.HTTPService }

func NewHandler(service aiimagemodule.HTTPService) *Handler { return &Handler{service: service} }

func (h *Handler) PageInit(c *gin.Context) {
	result, appErr := h.requireService().PageInit(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) List(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("aiimage.task.list.request.invalid", nil, "图片任务列表参数错误"))
		return
	}
	result, appErr := h.requireService().List(c.Request.Context(), userID, aiimagemodule.ListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, Status: req.Status, IsFavorite: req.IsFavorite})
	writeResult(c, result, appErr)
}

func (h *Handler) Detail(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := routeID(c, "aiimage.task.id.invalid", "无效的图片任务ID")
	if !ok {
		return
	}
	result, appErr := h.requireService().Detail(c.Request.Context(), userID, id)
	writeResult(c, result, appErr)
}

func (h *Handler) RegisterAsset(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req registerAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("aiimage.asset.request.invalid", nil, "图片资产参数错误"))
		return
	}
	result, appErr := h.requireService().RegisterAsset(c.Request.Context(), aiimagemodule.RegisterAssetInput{UserID: userID, StorageProvider: req.StorageProvider, StorageKey: req.StorageKey, StorageURL: req.StorageURL, MimeType: req.MimeType, Width: req.Width, Height: req.Height, SizeBytes: req.SizeBytes, SourceType: req.SourceType})
	writeResult(c, result, appErr)
}

func (h *Handler) Create(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req createTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("aiimage.task.request.invalid", nil, "图片任务参数错误"))
		return
	}
	result, appErr := h.requireService().Create(c.Request.Context(), aiimagemodule.CreateInput{UserID: userID, AgentID: req.AgentID, Platform: enum.PlatformAdmin, Prompt: req.Prompt, Size: req.Size, Quality: req.Quality, OutputFormat: req.OutputFormat, OutputCompression: req.OutputCompression, Moderation: req.Moderation, N: req.N, InputAssetIDs: req.InputAssetIDs, MaskAssetID: req.MaskAssetID, MaskTargetAssetID: req.MaskTargetAssetID})
	writeResult(c, result, appErr)
}

func (h *Handler) Favorite(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := routeID(c, "aiimage.task.id.invalid", "无效的图片任务ID")
	if !ok {
		return
	}
	var req favoriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("aiimage.favorite.request.invalid", nil, "图片收藏参数错误"))
		return
	}
	result, appErr := h.requireService().Favorite(c.Request.Context(), aiimagemodule.FavoriteInput{UserID: userID, TaskID: id, IsFavorite: req.IsFavorite})
	writeResult(c, result, appErr)
}

func (h *Handler) Delete(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := routeID(c, "aiimage.task.id.invalid", "无效的图片任务ID")
	if !ok {
		return
	}
	if appErr := h.requireService().Delete(c.Request.Context(), userID, id); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) requireService() aiimagemodule.HTTPService {
	if h == nil || h.service == nil {
		return nilHTTPService{}
	}
	return h.service
}

func currentUserID(c *gin.Context) (uint64, bool) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return 0, false
	}
	return uint64(identity.UserID), true
}

func routeID(c *gin.Context, messageID string, fallback string) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, apperror.BadRequestKey(messageID, nil, fallback))
		return 0, false
	}
	return id, true
}

func writeResult(c *gin.Context, result any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

type nilHTTPService struct{}

func (nilHTTPService) PageInit(ctx context.Context) (*aiimagemodule.PageInitResponse, *apperror.Error) {
	return nil, apperror.InternalKey("aiimage.service_missing", nil, "AI图片服务未配置")
}
func (nilHTTPService) List(ctx context.Context, userID uint64, query aiimagemodule.ListQuery) (*aiimagemodule.ListResponse, *apperror.Error) {
	return nil, apperror.InternalKey("aiimage.service_missing", nil, "AI图片服务未配置")
}
func (nilHTTPService) Detail(ctx context.Context, userID uint64, taskID uint64) (*aiimagemodule.DetailResponse, *apperror.Error) {
	return nil, apperror.InternalKey("aiimage.service_missing", nil, "AI图片服务未配置")
}
func (nilHTTPService) RegisterAsset(ctx context.Context, input aiimagemodule.RegisterAssetInput) (*aiimagemodule.AssetDTO, *apperror.Error) {
	return nil, apperror.InternalKey("aiimage.service_missing", nil, "AI图片服务未配置")
}
func (nilHTTPService) Create(ctx context.Context, input aiimagemodule.CreateInput) (*aiimagemodule.CreateTaskResponse, *apperror.Error) {
	return nil, apperror.InternalKey("aiimage.service_missing", nil, "AI图片服务未配置")
}
func (nilHTTPService) CreateWithUploadedAssets(ctx context.Context, input aiimagemodule.CreateWithUploadedAssetsInput) (*aiimagemodule.CreateTaskResponse, *apperror.Error) {
	return nil, apperror.InternalKey("aiimage.service_missing", nil, "AI图片服务未配置")
}
func (nilHTTPService) Favorite(ctx context.Context, input aiimagemodule.FavoriteInput) (*aiimagemodule.TaskDTO, *apperror.Error) {
	return nil, apperror.InternalKey("aiimage.service_missing", nil, "AI图片服务未配置")
}
func (nilHTTPService) Delete(ctx context.Context, userID uint64, taskID uint64) *apperror.Error {
	return apperror.InternalKey("aiimage.service_missing", nil, "AI图片服务未配置")
}
