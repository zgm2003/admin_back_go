package canvas

import (
	"context"
	"strconv"

	"admin_back_go/internal/middleware"
	assetmodule "admin_back_go/internal/module/ai/asset"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type HTTPService interface {
	UserList(ctx context.Context, userID uint64, query assetmodule.ListQuery) (*assetmodule.ListResponse, *apperror.Error)
	UserCreate(ctx context.Context, userID uint64, input assetmodule.Input) (int64, *apperror.Error)
	UserUpdate(ctx context.Context, userID uint64, id int64, input assetmodule.Input) *apperror.Error
	UserDelete(ctx context.Context, userID uint64, id int64) *apperror.Error
}

type Handler struct{ service HTTPService }

func NewHandler(service HTTPService) *Handler { return &Handler{service: service} }

func (h *Handler) Assets(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req listAssetsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("ai.asset.request.invalid", nil, "素材列表参数错误"))
		return
	}
	result, appErr := h.requireService().UserList(c.Request.Context(), userID, assetmodule.ListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, Keyword: req.Keyword, Type: req.Type})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) Create(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req assetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("ai.asset.request.invalid", nil, "素材参数错误"))
		return
	}
	id, appErr := h.requireService().UserCreate(c.Request.Context(), userID, input(req))
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OKWithMessageKey(c, gin.H{"id": id}, "ai.asset.create_success", nil, "创建AI素材成功")
}

func (h *Handler) Update(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req assetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("ai.asset.request.invalid", nil, "素材参数错误"))
		return
	}
	if appErr := h.requireService().UserUpdate(c.Request.Context(), userID, id, input(req)); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OKWithMessageKey(c, gin.H{}, "ai.asset.update_success", nil, "更新AI素材成功")
}

func (h *Handler) Delete(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	if appErr := h.requireService().UserDelete(c.Request.Context(), userID, id); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OKWithMessageKey(c, gin.H{}, "ai.asset.delete_success", nil, "删除AI素材成功")
}

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return failingService{}
	}
	return h.service
}

type failingService struct{}

func (failingService) UserList(ctx context.Context, userID uint64, query assetmodule.ListQuery) (*assetmodule.ListResponse, *apperror.Error) {
	return nil, apperror.InternalKey("ai.asset.service_missing", nil, "AI素材服务未配置")
}
func (failingService) UserCreate(ctx context.Context, userID uint64, input assetmodule.Input) (int64, *apperror.Error) {
	return 0, apperror.InternalKey("ai.asset.service_missing", nil, "AI素材服务未配置")
}
func (failingService) UserUpdate(ctx context.Context, userID uint64, id int64, input assetmodule.Input) *apperror.Error {
	return apperror.InternalKey("ai.asset.service_missing", nil, "AI素材服务未配置")
}
func (failingService) UserDelete(ctx context.Context, userID uint64, id int64) *apperror.Error {
	return apperror.InternalKey("ai.asset.service_missing", nil, "AI素材服务未配置")
}

func currentUserID(c *gin.Context) (uint64, bool) {
	raw, exists := c.Get(middleware.ContextAuthIdentity)
	if !exists {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return 0, false
	}
	identity, ok := raw.(*middleware.AuthIdentity)
	if !ok || identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return 0, false
	}
	if identity.Platform != enum.PlatformCanvas {
		response.Error(c, apperror.UnauthorizedKey("auth.platform.invalid", map[string]any{"platform": identity.Platform}, "Token平台不匹配"))
		return 0, false
	}
	return uint64(identity.UserID), true
}

func routeID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequestKey("ai.asset.id.invalid", nil, "素材ID无效"))
		return 0, false
	}
	return id, true
}

func input(req assetRequest) assetmodule.Input {
	return assetmodule.Input{
		Slug:        req.Slug,
		Type:        req.Type,
		Category:    req.Category,
		Title:       req.Title,
		CoverURL:    req.CoverURL,
		Description: req.Description,
		Content:     req.Content,
		URL:         req.URL,
		TagsJSON:    req.TagsJSON,
		Status:      req.Status,
	}
}
