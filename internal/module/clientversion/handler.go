package clientversion

import (
	"strconv"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service HTTPService
}

func NewHandler(service HTTPService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Init(c *gin.Context) {
	service, ok := h.requireService(c)
	if !ok {
		return
	}
	result, appErr := service.Init(c.Request.Context())
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
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("clientversion.list.request.invalid", nil, "列表参数错误"))
		return
	}
	result, appErr := service.List(c.Request.Context(), ListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, Platform: req.Platform})
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
	var req saveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("clientversion.create.request.invalid", nil, "参数错误"))
		return
	}
	id, appErr := service.Create(c.Request.Context(), createInput(req))
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{"id": id})
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
	var req saveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("clientversion.update.request.invalid", nil, "参数错误"))
		return
	}
	if appErr := service.Update(c.Request.Context(), id, UpdateInput(createInput(req))); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) SetLatest(c *gin.Context) {
	service, ok := h.requireService(c)
	if !ok {
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	if appErr := service.SetLatest(c.Request.Context(), id); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) ForceUpdate(c *gin.Context) {
	service, ok := h.requireService(c)
	if !ok {
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req forceUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("clientversion.force_update.invalid", nil, "无效的强制更新状态"))
		return
	}
	if appErr := service.ForceUpdate(c.Request.Context(), id, req.ForceUpdate); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) Delete(c *gin.Context) {
	service, ok := h.requireService(c)
	if !ok {
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	if appErr := service.Delete(c.Request.Context(), id); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) UpdateJSON(c *gin.Context) {
	service, ok := h.requireService(c)
	if !ok {
		return
	}
	var req updateJSONRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("clientversion.platform.invalid", nil, "无效的客户端平台"))
		return
	}
	result, appErr := service.UpdateJSON(c.Request.Context(), req.Platform)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) CurrentCheck(c *gin.Context) {
	service, ok := h.requireService(c)
	if !ok {
		return
	}
	var req currentCheckRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("clientversion.current_check.request.invalid", nil, "参数错误"))
		return
	}
	result, appErr := service.CurrentCheck(c.Request.Context(), CurrentCheckQuery{Version: req.Version, Platform: req.Platform})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) requireService(c *gin.Context) (HTTPService, bool) {
	if h == nil || h.service == nil {
		response.Error(c, apperror.InternalKey("clientversion.service_missing", nil, "客户端版本服务未配置"))
		return nil, false
	}
	return h.service, true
}

func routeID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequestKey("clientversion.id.invalid", nil, "无效的版本ID"))
		return 0, false
	}
	return id, true
}

func createInput(req saveRequest) CreateInput {
	return CreateInput{
		Version:     req.Version,
		Notes:       req.Notes,
		FileURL:     req.FileURL,
		Signature:   req.Signature,
		Platform:    req.Platform,
		FileSize:    req.FileSize,
		ForceUpdate: req.ForceUpdate,
	}
}
