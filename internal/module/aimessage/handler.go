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
	res, appErr := h.requireService().List(c.Request.Context(), identity.UserID, ListQuery{ConversationID: conversationID, BeforeID: req.BeforeID, Limit: req.Limit})
	writeResult(c, res, appErr)
}

func (h *Handler) Send(c *gin.Context) {
	identity, ok := authIdentity(c)
	if !ok {
		return
	}
	conversationID, ok := routeID(c, "id", "无效的AI会话ID")
	if !ok {
		return
	}
	var req sendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI消息参数错误"))
		return
	}
	res, appErr := h.requireService().Send(c.Request.Context(), identity.UserID, SendInput{ConversationID: conversationID, Content: req.Content, RequestID: req.RequestID, Attachments: req.Attachments, RuntimeParams: req.RuntimeParams})
	writeResult(c, res, appErr)
}

func (h *Handler) Cancel(c *gin.Context) {
	identity, ok := authIdentity(c)
	if !ok {
		return
	}
	conversationID, ok := routeID(c, "id", "无效的AI会话ID")
	if !ok {
		return
	}
	var req cancelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI消息参数错误"))
		return
	}
	res, appErr := h.requireService().Cancel(c.Request.Context(), identity.UserID, CancelInput{ConversationID: conversationID, RequestID: req.RequestID})
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
func (nilHTTPService) Send(ctx context.Context, userID int64, input SendInput) (*SendResponse, *apperror.Error) {
	return nil, apperror.Internal("AI消息服务未配置")
}
func (nilHTTPService) Cancel(ctx context.Context, userID int64, input CancelInput) (*CancelResponse, *apperror.Error) {
	return nil, apperror.Internal("AI消息服务未配置")
}
