package systemsetting

import (
	"context"
	"strconv"

	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type HTTPService interface {
	PageInit(context.Context) (*PageInitResponse, *apperror.Error)
	List(context.Context, ListRequest) (*ListResponse, *apperror.Error)
	Create(context.Context, CreateRequest) (int64, *apperror.Error)
	Update(context.Context, int64, UpdateRequest) *apperror.Error
	Delete(context.Context, []int64) *apperror.Error
	ChangeStatus(context.Context, int64, int) *apperror.Error
}

type Handler struct {
	service HTTPService
}

func NewHandler(service HTTPService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) requireService(c *gin.Context) (HTTPService, bool) {
	if h == nil || h.service == nil {
		response.Error(c, apperror.InternalKey(
			"systemsetting.service_missing",
			nil,
			"系统设置服务未配置",
		))
		return nil, false
	}
	return h.service, true
}

func (h *Handler) PageInit(c *gin.Context) {
	service, ok := h.requireService(c)
	if !ok {
		return
	}
	result, appErr := service.PageInit(c.Request.Context())
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) List(c *gin.Context) {
	service, ok := h.requireService(c)
	if !ok {
		return
	}
	var request ListRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		response.Error(c, apperror.BadRequestKey("systemsetting.list.request.invalid", nil, "列表参数错误"))
		return
	}
	result, appErr := service.List(c.Request.Context(), request)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) Create(c *gin.Context) {
	service, ok := h.requireService(c)
	if !ok {
		return
	}
	var request CreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, apperror.BadRequestKey("systemsetting.create.request.invalid", nil, "参数错误"))
		return
	}
	id, appErr := service.Create(c.Request.Context(), request)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, CreateResponse{ID: id})
}

func (h *Handler) Update(c *gin.Context) {
	service, ok := h.requireService(c)
	if !ok {
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	var request UpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, apperror.BadRequestKey("systemsetting.update.request.invalid", nil, "参数错误"))
		return
	}
	if appErr := service.Update(c.Request.Context(), id, request); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, EmptyResponse{})
}

func (h *Handler) DeleteOne(c *gin.Context) {
	service, ok := h.requireService(c)
	if !ok {
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	if appErr := service.Delete(c.Request.Context(), []int64{id}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, EmptyResponse{})
}

func (h *Handler) DeleteBatch(c *gin.Context) {
	service, ok := h.requireService(c)
	if !ok {
		return
	}
	var request DeleteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, apperror.BadRequestKey("systemsetting.delete.empty", nil, "请选择要删除的配置"))
		return
	}
	if appErr := service.Delete(c.Request.Context(), request.IDs); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, EmptyResponse{})
}

func (h *Handler) ChangeStatus(c *gin.Context) {
	service, ok := h.requireService(c)
	if !ok {
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	var request StatusRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, apperror.BadRequestKey("systemsetting.status.invalid", nil, "无效的状态"))
		return
	}
	if appErr := service.ChangeStatus(c.Request.Context(), id, request.Status); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, EmptyResponse{})
}

func routeID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequestKey("systemsetting.id.invalid", nil, "无效的配置ID"))
		return 0, false
	}
	return id, true
}
