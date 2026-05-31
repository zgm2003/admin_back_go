package admin

import (
	"context"
	"strconv"

	rolemodule "admin_back_go/internal/module/role"

	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type HTTPService interface {
	PageInit(ctx context.Context) (*rolemodule.InitResponse, *apperror.Error)
	List(ctx context.Context, query rolemodule.ListQuery) (*rolemodule.ListResponse, *apperror.Error)
	Create(ctx context.Context, input rolemodule.MutationInput) (int64, *apperror.Error)
	Update(ctx context.Context, id int64, input rolemodule.MutationInput) *apperror.Error
	Delete(ctx context.Context, ids []int64) *apperror.Error
	SetDefault(ctx context.Context, id int64) *apperror.Error
}

type Handler struct {
	service HTTPService
}

func NewHandler(service HTTPService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) PageInit(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("角色服务未配置"))
		return
	}
	result, appErr := h.service.PageInit(c.Request.Context())
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) List(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("角色服务未配置"))
		return
	}

	query, ok := bindListQuery(c)
	if !ok {
		return
	}
	result, appErr := h.service.List(c.Request.Context(), query)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) Create(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("角色服务未配置"))
		return
	}

	input, ok := bindMutation(c)
	if !ok {
		return
	}
	id, appErr := h.service.Create(c.Request.Context(), input)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{"id": id})
}

func (h *Handler) Update(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("角色服务未配置"))
		return
	}

	id, ok := routeID(c)
	if !ok {
		return
	}
	input, ok := bindMutation(c)
	if !ok {
		return
	}
	if appErr := h.service.Update(c.Request.Context(), id, input); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) DeleteOne(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("角色服务未配置"))
		return
	}

	id, ok := routeID(c)
	if !ok {
		return
	}
	if appErr := h.service.Delete(c.Request.Context(), []int64{id}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) DeleteBatch(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("角色服务未配置"))
		return
	}

	var req deleteBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		response.Error(c, apperror.BadRequest("请选择要删除的角色"))
		return
	}
	if appErr := h.service.Delete(c.Request.Context(), req.IDs); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) SetDefault(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("角色服务未配置"))
		return
	}

	id, ok := routeID(c)
	if !ok {
		return
	}
	if appErr := h.service.SetDefault(c.Request.Context(), id); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func bindListQuery(c *gin.Context) (rolemodule.ListQuery, bool) {
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("列表参数错误"))
		return rolemodule.ListQuery{}, false
	}
	return rolemodule.ListQuery{
		CurrentPage: req.CurrentPage,
		PageSize:    req.PageSize,
		Name:        req.Name,
	}, true
}

func bindMutation(c *gin.Context) (rolemodule.MutationInput, bool) {
	var req mutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("参数错误"))
		return rolemodule.MutationInput{}, false
	}
	return rolemodule.MutationInput{
		Name:          req.Name,
		PermissionIDs: req.PermissionIDs,
	}, true
}

func routeID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequest("无效的角色ID"))
		return 0, false
	}
	return id, true
}
