package chat

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

func (h *Handler) ListConversations(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("聊天服务未配置"))
		return
	}
	identity, ok := identityFromContext(c)
	if !ok {
		return
	}
	var req conversationListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("会话列表参数错误"))
		return
	}
	if req.CurrentPage == 0 {
		req.CurrentPage = 1
	}
	if req.PageSize == 0 {
		req.PageSize = defaultPageSize
	}
	result, appErr := h.service.ListConversations(c.Request.Context(), identity, ConversationListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) CreatePrivate(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("聊天服务未配置"))
		return
	}
	identity, ok := identityFromContext(c)
	if !ok {
		return
	}
	var req createPrivateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("创建私聊参数错误"))
		return
	}
	result, appErr := h.service.CreatePrivate(c.Request.Context(), identity, CreatePrivateInput{UserID: req.UserID})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) ListMessages(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("聊天服务未配置"))
		return
	}
	identity, ok := identityFromContext(c)
	if !ok {
		return
	}
	conversationID, ok := routeID(c, "id", "无效的会话ID")
	if !ok {
		return
	}
	var req messageListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("消息列表参数错误"))
		return
	}
	if req.PageSize == 0 {
		req.PageSize = defaultPageSize
	}
	result, appErr := h.service.ListMessages(c.Request.Context(), identity, MessageListQuery{ConversationID: conversationID, CurrentPage: req.CurrentPage, PageSize: req.PageSize})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) SendMessage(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("聊天服务未配置"))
		return
	}
	identity, ok := identityFromContext(c)
	if !ok {
		return
	}
	conversationID, ok := routeID(c, "id", "无效的会话ID")
	if !ok {
		return
	}
	var req sendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("发送消息参数错误"))
		return
	}
	meta, ok := decodeMetaJSON(req.MetaJSON)
	if !ok {
		response.Error(c, apperror.BadRequest("消息元数据格式错误"))
		return
	}
	result, appErr := h.service.SendMessage(c.Request.Context(), identity, SendMessageInput{ConversationID: conversationID, Type: req.Type, Content: req.Content, MetaJSON: meta})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) MarkRead(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("聊天服务未配置"))
		return
	}
	identity, ok := identityFromContext(c)
	if !ok {
		return
	}
	conversationID, ok := routeID(c, "id", "无效的会话ID")
	if !ok {
		return
	}
	if appErr := h.service.MarkRead(c.Request.Context(), identity, MarkReadInput{ConversationID: conversationID}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) ListContacts(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("聊天服务未配置"))
		return
	}
	identity, ok := identityFromContext(c)
	if !ok {
		return
	}
	result, appErr := h.service.ListContacts(c.Request.Context(), identity)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) AddContact(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("聊天服务未配置"))
		return
	}
	identity, ok := identityFromContext(c)
	if !ok {
		return
	}
	userID, ok := routeID(c, "user_id", "无效的用户ID")
	if !ok {
		return
	}
	if appErr := h.service.AddContact(c.Request.Context(), identity, ContactInput{UserID: userID}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) ConfirmContact(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("聊天服务未配置"))
		return
	}
	identity, ok := identityFromContext(c)
	if !ok {
		return
	}
	userID, ok := routeID(c, "user_id", "无效的用户ID")
	if !ok {
		return
	}
	if appErr := h.service.ConfirmContact(c.Request.Context(), identity, ContactInput{UserID: userID}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) DeleteContact(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("聊天服务未配置"))
		return
	}
	identity, ok := identityFromContext(c)
	if !ok {
		return
	}
	userID, ok := routeID(c, "user_id", "无效的用户ID")
	if !ok {
		return
	}
	if appErr := h.service.DeleteContact(c.Request.Context(), identity, ContactInput{UserID: userID}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) DeleteConversation(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("聊天服务未配置"))
		return
	}
	identity, ok := identityFromContext(c)
	if !ok {
		return
	}
	conversationID, ok := routeID(c, "id", "无效的会话ID")
	if !ok {
		return
	}
	if appErr := h.service.DeleteConversation(c.Request.Context(), identity, conversationID); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) TogglePin(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("聊天服务未配置"))
		return
	}
	identity, ok := identityFromContext(c)
	if !ok {
		return
	}
	conversationID, ok := routeID(c, "id", "无效的会话ID")
	if !ok {
		return
	}
	if appErr := h.service.TogglePin(c.Request.Context(), identity, conversationID); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func identityFromContext(c *gin.Context) (Identity, bool) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return Identity{}, false
	}
	return Identity{UserID: identity.UserID, Platform: identity.Platform}, true
}

func routeID(c *gin.Context, name string, message string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequest(message))
		return 0, false
	}
	return id, true
}

var _ HTTPService = (*Service)(nil)
