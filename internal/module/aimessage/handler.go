package aimessage

import (
	"context"
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

func (h *Handler) List(c *gin.Context) {
	identity, ok := authIdentity(c)
	if !ok {
		return
	}
	conversationID, ok := routeID(c, "id", "无效的AI会话ID")
	if !ok {
		return
	}
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI消息列表参数错误"))
		return
	}
	res, appErr := h.requireService().List(c.Request.Context(), identity.UserID, ListQuery{ConversationID: conversationID, CurrentPage: req.CurrentPage, PageSize: req.PageSize, Role: req.Role})
	writeResult(c, res, appErr)
}

func (h *Handler) EditContent(c *gin.Context) {
	identity, ok := authIdentity(c)
	if !ok {
		return
	}
	id, ok := routeID(c, "id", "无效的AI消息ID")
	if !ok {
		return
	}
	var req contentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI消息参数错误"))
		return
	}
	res, appErr := h.requireService().EditContent(c.Request.Context(), identity.UserID, id, req.Content)
	writeResult(c, res, appErr)
}

func (h *Handler) Feedback(c *gin.Context) {
	identity, ok := authIdentity(c)
	if !ok {
		return
	}
	id, ok := routeID(c, "id", "无效的AI消息ID")
	if !ok {
		return
	}
	var req feedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI消息反馈参数错误"))
		return
	}
	if appErr := h.requireService().Feedback(c.Request.Context(), identity.UserID, id, req.Feedback); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) Delete(c *gin.Context) {
	identity, ok := authIdentity(c)
	if !ok {
		return
	}
	id, ok := routeID(c, "id", "无效的AI消息ID")
	if !ok {
		return
	}
	res, appErr := h.requireService().Delete(c.Request.Context(), identity.UserID, []int64{id})
	writeResult(c, res, appErr)
}

func (h *Handler) BatchDelete(c *gin.Context) {
	identity, ok := authIdentity(c)
	if !ok {
		return
	}
	var req batchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI消息批量删除参数错误"))
		return
	}
	res, appErr := h.requireService().Delete(c.Request.Context(), identity.UserID, req.IDs)
	writeResult(c, res, appErr)
}

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return nilHTTPService{}
	}
	return h.service
}

func authIdentity(c *gin.Context) (*middleware.AuthIdentity, bool) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return nil, false
	}
	return identity, true
}

func routeID(c *gin.Context, name string, msg string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequest(msg))
		return 0, false
	}
	return id, true
}

func writeResult(c *gin.Context, res any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, res)
}

type nilHTTPService struct{}

func (nilHTTPService) List(ctx context.Context, userID int64, query ListQuery) (*ListResponse, *apperror.Error) {
	return nil, apperror.Internal("AI消息服务未配置")
}
func (nilHTTPService) EditContent(ctx context.Context, userID int64, id int64, content string) (*EditContentResponse, *apperror.Error) {
	return nil, apperror.Internal("AI消息服务未配置")
}
func (nilHTTPService) Feedback(ctx context.Context, userID int64, id int64, feedback *int) *apperror.Error {
	return apperror.Internal("AI消息服务未配置")
}
func (nilHTTPService) Delete(ctx context.Context, userID int64, ids []int64) (*DeleteResponse, *apperror.Error) {
	return nil, apperror.Internal("AI消息服务未配置")
}
