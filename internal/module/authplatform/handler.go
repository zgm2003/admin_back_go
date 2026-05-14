package authplatform

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
	Create(ctx context.Context, input CreateInput) (int64, *apperror.Error)
	Update(ctx context.Context, id int64, input UpdateInput) *apperror.Error
	Delete(ctx context.Context, ids []int64) *apperror.Error
	ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error
}

type Handler struct {
	service HTTPService
}

func NewHandler(service HTTPService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Init(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("authplatform.service_missing", nil, "认证平台服务未配置"))
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
		response.Error(c, apperror.InternalKey("authplatform.service_missing", nil, "认证平台服务未配置"))
		return
	}
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("authplatform.list.request.invalid", nil, "列表参数错误"))
		return
	}
	result, appErr := h.service.List(c.Request.Context(), ListQuery{
		CurrentPage: req.CurrentPage,
		PageSize:    req.PageSize,
		Name:        req.Name,
		Status:      req.Status,
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) Create(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("authplatform.service_missing", nil, "认证平台服务未配置"))
		return
	}
	input, ok := bindCreate(c)
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
		response.Error(c, apperror.InternalKey("authplatform.service_missing", nil, "认证平台服务未配置"))
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	input, ok := bindUpdate(c)
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
		response.Error(c, apperror.InternalKey("authplatform.service_missing", nil, "认证平台服务未配置"))
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
		response.Error(c, apperror.InternalKey("authplatform.service_missing", nil, "认证平台服务未配置"))
		return
	}
	var req deleteBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("authplatform.delete.empty", nil, "请选择要删除的平台"))
		return
	}
	if appErr := h.service.Delete(c.Request.Context(), req.IDs); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) ChangeStatus(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("authplatform.service_missing", nil, "认证平台服务未配置"))
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req statusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("authplatform.status.invalid", nil, "无效的状态"))
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
		response.Error(c, apperror.BadRequestKey("authplatform.id.invalid", nil, "无效的平台ID"))
		return 0, false
	}
	return id, true
}

func bindCreate(c *gin.Context) (CreateInput, bool) {
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("authplatform.create.request.invalid", nil, "参数错误"))
		return CreateInput{}, false
	}
	return CreateInput{
		Code: req.Code, Name: req.Name, LoginTypes: req.LoginTypes, CaptchaType: req.CaptchaType,
		AccessTTL: req.AccessTTL, RefreshTTL: req.RefreshTTL, BindPlatform: req.BindPlatform, BindDevice: req.BindDevice,
		BindIP: req.BindIP, SingleSession: req.SingleSession, MaxSessions: req.MaxSessions, AllowRegister: req.AllowRegister,
	}, true
}

func bindUpdate(c *gin.Context) (UpdateInput, bool) {
	var req updateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("authplatform.update.request.invalid", nil, "参数错误"))
		return UpdateInput{}, false
	}
	return UpdateInput{
		Name: req.Name, LoginTypes: req.LoginTypes, CaptchaType: req.CaptchaType,
		AccessTTL: req.AccessTTL, RefreshTTL: req.RefreshTTL, BindPlatform: req.BindPlatform, BindDevice: req.BindDevice,
		BindIP: req.BindIP, SingleSession: req.SingleSession, MaxSessions: req.MaxSessions, AllowRegister: req.AllowRegister,
	}, true
}
