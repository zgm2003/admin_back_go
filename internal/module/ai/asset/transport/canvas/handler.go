package canvas

import (
	"context"
	"strconv"

	assetmodule "admin_back_go/internal/module/ai/asset"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type HTTPService interface {
	PublicList(ctx context.Context, query assetmodule.ListQuery) (*assetmodule.ListResponse, *apperror.Error)
	Create(ctx context.Context, input assetmodule.Input) (int64, *apperror.Error)
	Update(ctx context.Context, id int64, input assetmodule.Input) *apperror.Error
	Delete(ctx context.Context, id int64) *apperror.Error
}

type Handler struct{ service HTTPService }

func NewHandler(service HTTPService) *Handler { return &Handler{service: service} }

func (h *Handler) Assets(c *gin.Context) {
	var req listAssetsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("ai.asset.request.invalid", nil, "素材列表参数错误"))
		return
	}
	result, appErr := h.requireService().PublicList(c.Request.Context(), assetmodule.ListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, Keyword: req.Keyword, Type: req.Type})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) Create(c *gin.Context) {
	var req assetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("ai.asset.request.invalid", nil, "素材参数错误"))
		return
	}
	id, appErr := h.requireService().Create(c.Request.Context(), input(req))
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OKWithMessageKey(c, gin.H{"id": id}, "ai.asset.create_success", nil, "创建AI素材成功")
}

func (h *Handler) Update(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req assetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("ai.asset.request.invalid", nil, "素材参数错误"))
		return
	}
	if appErr := h.requireService().Update(c.Request.Context(), id, input(req)); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OKWithMessageKey(c, gin.H{}, "ai.asset.update_success", nil, "更新AI素材成功")
}

func (h *Handler) Delete(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	if appErr := h.requireService().Delete(c.Request.Context(), id); appErr != nil {
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

func (failingService) PublicList(ctx context.Context, query assetmodule.ListQuery) (*assetmodule.ListResponse, *apperror.Error) {
	return nil, apperror.InternalKey("ai.asset.service_missing", nil, "AI素材服务未配置")
}
func (failingService) Create(ctx context.Context, input assetmodule.Input) (int64, *apperror.Error) {
	return 0, apperror.InternalKey("ai.asset.service_missing", nil, "AI素材服务未配置")
}
func (failingService) Update(ctx context.Context, id int64, input assetmodule.Input) *apperror.Error {
	return apperror.InternalKey("ai.asset.service_missing", nil, "AI素材服务未配置")
}
func (failingService) Delete(ctx context.Context, id int64) *apperror.Error {
	return apperror.InternalKey("ai.asset.service_missing", nil, "AI素材服务未配置")
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
