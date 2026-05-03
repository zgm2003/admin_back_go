package role

import (
	"context"
	"strconv"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Create(ctx context.Context, input MutationInput) (int64, *apperror.Error)
	Update(ctx context.Context, id int64, input MutationInput) *apperror.Error
	Delete(ctx context.Context, ids []int64) *apperror.Error
	SetDefault(ctx context.Context, id int64) *apperror.Error
}

type Handler struct {
	service HTTPService
}

func NewHandler(service HTTPService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Init(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("角色服务未配置"))
		return
	}
	result, appErr := h.service.Init(c.Request.Context())
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
	if err := c.ShouldBindJSON(&req); err != nil || len(normalizeIDs(req.IDs)) == 0 {
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

func bindListQuery(c *gin.Context) (ListQuery, bool) {
	currentPage, ok := parseRequiredPositiveInt(c, "current_page", "当前页无效")
	if !ok {
		return ListQuery{}, false
	}
	pageSize, ok := parseRequiredPositiveInt(c, "page_size", "每页数量无效")
	if !ok {
		return ListQuery{}, false
	}
	return ListQuery{
		CurrentPage: currentPage,
		PageSize:    pageSize,
		Name:        c.Query("name"),
	}, true
}

func parseRequiredPositiveInt(c *gin.Context, key string, message string) (int, bool) {
	value := c.Query(key)
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		response.Error(c, apperror.BadRequest(message))
		return 0, false
	}
	return parsed, true
}

func bindMutation(c *gin.Context) (MutationInput, bool) {
	var req mutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("参数错误"))
		return MutationInput{}, false
	}
	return MutationInput{
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

type mutationRequest struct {
	Name          string  `json:"name"`
	PermissionIDs []int64 `json:"permission_id"`
}

type deleteBatchRequest struct {
	IDs []int64 `json:"ids"`
}
