package exporttask

import (
	"strconv"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service HTTPService
}

func NewHandler(service HTTPService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) StatusCount(c *gin.Context) {
	identity, ok := currentIdentity(c)
	if !ok {
		return
	}
	if h.service == nil {
		response.Error(c, apperror.Internal("导出任务服务未配置"))
		return
	}
	var req statusCountRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("状态统计参数错误"))
		return
	}
	result, appErr := h.service.StatusCount(c.Request.Context(), StatusCountQuery{UserID: identity.UserID, Title: req.Title, FileName: req.FileName})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) List(c *gin.Context) {
	identity, ok := currentIdentity(c)
	if !ok {
		return
	}
	if h.service == nil {
		response.Error(c, apperror.Internal("导出任务服务未配置"))
		return
	}
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("列表参数错误"))
		return
	}
	result, appErr := h.service.List(c.Request.Context(), ListQuery{
		UserID:      identity.UserID,
		CurrentPage: req.CurrentPage,
		PageSize:    req.PageSize,
		Status:      req.Status,
		Title:       req.Title,
		FileName:    req.FileName,
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) DeleteOne(c *gin.Context) {
	identity, ok := currentIdentity(c)
	if !ok {
		return
	}
	if h.service == nil {
		response.Error(c, apperror.Internal("导出任务服务未配置"))
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	if appErr := h.service.Delete(c.Request.Context(), DeleteInput{UserID: identity.UserID, IDs: []int64{id}}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) DeleteBatch(c *gin.Context) {
	identity, ok := currentIdentity(c)
	if !ok {
		return
	}
	if h.service == nil {
		response.Error(c, apperror.Internal("导出任务服务未配置"))
		return
	}
	var req deleteBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("参数错误"))
		return
	}
	if appErr := h.service.Delete(c.Request.Context(), DeleteInput{UserID: identity.UserID, IDs: req.IDs}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func currentIdentity(c *gin.Context) (*middleware.AuthIdentity, bool) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return nil, false
	}
	return identity, true
}

func routeID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequest("无效的导出任务ID"))
		return 0, false
	}
	return id, true
}

var _ HTTPService = (*Service)(nil)
