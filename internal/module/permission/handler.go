package permission

import (
	"context"
	"strconv"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type ManagementService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	List(ctx context.Context, query PermissionListQuery) ([]PermissionListItem, *apperror.Error)
	Create(ctx context.Context, input PermissionMutationInput) (int64, *apperror.Error)
	Update(ctx context.Context, id int64, input PermissionMutationInput) *apperror.Error
	Delete(ctx context.Context, ids []int64) *apperror.Error
	ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error
}

type ManagementHandler struct {
	service ManagementService
}

func NewManagementHandler(service ManagementService) *ManagementHandler {
	return &ManagementHandler{service: service}
}

func (h *ManagementHandler) Init(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("权限服务未配置"))
		return
	}
	result, appErr := h.service.Init(c.Request.Context())
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *ManagementHandler) List(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("权限服务未配置"))
		return
	}

	query := PermissionListQuery{
		Platform: c.Query("platform"),
		Name:     c.Query("name"),
		Path:     c.Query("path"),
	}
	if typeValue := c.Query("type"); typeValue != "" {
		parsed, err := strconv.Atoi(typeValue)
		if err != nil {
			response.Error(c, apperror.BadRequest("无效的权限类型"))
			return
		}
		query.Type = parsed
	}

	result, appErr := h.service.List(c.Request.Context(), query)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *ManagementHandler) Create(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("权限服务未配置"))
		return
	}

	input, ok := bindPermissionMutation(c)
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

func (h *ManagementHandler) Update(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("权限服务未配置"))
		return
	}

	id, ok := routeID(c)
	if !ok {
		return
	}
	input, ok := bindPermissionMutation(c)
	if !ok {
		return
	}

	if appErr := h.service.Update(c.Request.Context(), id, input); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *ManagementHandler) DeleteOne(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("权限服务未配置"))
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

func (h *ManagementHandler) DeleteBatch(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("权限服务未配置"))
		return
	}

	var req deleteBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("请选择要删除的权限"))
		return
	}
	if appErr := h.service.Delete(c.Request.Context(), req.IDs); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *ManagementHandler) ChangeStatus(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("权限服务未配置"))
		return
	}

	id, ok := routeID(c)
	if !ok {
		return
	}
	var req statusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("无效的状态"))
		return
	}

	if appErr := h.service.ChangeStatus(c.Request.Context(), id, req.Status); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func routeID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequest("无效的权限ID"))
		return 0, false
	}
	return id, true
}

func bindPermissionMutation(c *gin.Context) (PermissionMutationInput, bool) {
	var req permissionMutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("参数错误"))
		return PermissionMutationInput{}, false
	}
	return PermissionMutationInput{
		Platform:  req.Platform,
		Type:      req.Type,
		Name:      req.Name,
		ParentID:  req.ParentID,
		Icon:      req.Icon,
		Path:      req.Path,
		Component: req.Component,
		I18nKey:   req.I18nKey,
		Code:      req.Code,
		Sort:      req.Sort,
		ShowMenu:  req.ShowMenu,
	}, true
}

type permissionMutationRequest struct {
	Platform  string `json:"platform"`
	Type      int    `json:"type"`
	Name      string `json:"name"`
	ParentID  int64  `json:"parent_id"`
	Icon      string `json:"icon"`
	Path      string `json:"path"`
	Component string `json:"component"`
	I18nKey   string `json:"i18n_key"`
	Code      string `json:"code"`
	Sort      int    `json:"sort"`
	ShowMenu  int    `json:"show_menu"`
}

type deleteBatchRequest struct {
	IDs []int64 `json:"ids"`
}

type statusRequest struct {
	Status int `json:"status"`
}
