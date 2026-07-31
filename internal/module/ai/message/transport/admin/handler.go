package admin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"time"

	"admin_back_go/internal/middleware"
	aimessagemodule "admin_back_go/internal/module/ai/message"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type Handler struct {
	service aimessagemodule.HTTPService
	history aimessagemodule.HistoryHTTPService
}

func NewHandler(service aimessagemodule.HTTPService) *Handler {
	history, _ := service.(aimessagemodule.HistoryHTTPService)
	return &Handler{service: service, history: history}
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
	res, appErr := h.requireService().List(c.Request.Context(), identity.UserID, aimessagemodule.ListQuery{ConversationID: conversationID, BeforeID: req.BeforeID, Limit: req.Limit})
	writeResult(c, res, appErr)
}

func (h *Handler) Send(c *gin.Context) {
	requestReceivedAt := time.Now().UTC()
	identity, ok := authIdentity(c)
	if !ok {
		return
	}
	conversationID, ok := routeID(c, "id", "无效的AI会话ID")
	if !ok {
		return
	}
	var req sendRequest
	if err := bindStrictJSON(c, &req); err != nil {
		response.Error(c, apperror.BadRequest("AI消息参数错误"))
		return
	}
	attachments := domainAttachments(req.Attachments)
	res, appErr := h.requireService().Send(c.Request.Context(), identity.UserID, aimessagemodule.SendInput{ConversationID: conversationID, Content: req.Content, RequestID: req.RequestID, RequestReceivedAt: requestReceivedAt, Attachments: attachments, RuntimeParams: req.RuntimeParams})
	writeAcceptedResult(c, res, appErr)
}

func bindStrictJSON(c *gin.Context, target any) error {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return binding.Validator.ValidateStruct(target)
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
	if err := bindStrictJSON(c, &req); err != nil {
		response.Error(c, apperror.BadRequest("AI消息参数错误"))
		return
	}
	res, appErr := h.requireService().Cancel(c.Request.Context(), identity.UserID, aimessagemodule.CancelInput{
		ConversationID: conversationID,
		RequestID:      req.RequestID,
		DeliveredSeq:   *req.DeliveredSeq,
	})
	writeResult(c, res, appErr)
}

func (h *Handler) Revise(c *gin.Context) {
	identity, ok := authIdentity(c)
	if !ok {
		return
	}
	conversationID, ok := routeID(c, "id", "无效的AI会话ID")
	if !ok {
		return
	}
	messageID, ok := routeID(c, "message_id", "无效的AI消息ID")
	if !ok {
		return
	}
	var req revisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("aimessage.revision.request.invalid", nil, "AI消息编辑参数错误"))
		return
	}
	var attachments *[]aimessagemodule.Attachment
	if req.Attachments != nil {
		values := domainAttachments(*req.Attachments)
		attachments = &values
	}
	res, appErr := h.requireHistoryService().Revise(c.Request.Context(), identity.UserID, aimessagemodule.EditInput{
		ConversationID: conversationID, MessageID: messageID, Content: req.Content, RequestID: req.RequestID,
		Attachments: attachments,
	})
	writeAcceptedResult(c, res, appErr)
}

func domainAttachments(input []attachmentRequest) []aimessagemodule.Attachment {
	attachments := make([]aimessagemodule.Attachment, len(input))
	for index, attachment := range input {
		attachments[index] = aimessagemodule.Attachment{
			Type: attachment.Type, ObjectKey: attachment.ObjectKey, MIMEType: attachment.MIMEType,
			URL: attachment.URL, Name: attachment.Name, Size: attachment.Size,
		}
	}
	return attachments
}

func (h *Handler) Regenerate(c *gin.Context) {
	identity, ok := authIdentity(c)
	if !ok {
		return
	}
	conversationID, ok := routeID(c, "id", "无效的AI会话ID")
	if !ok {
		return
	}
	messageID, ok := routeID(c, "message_id", "无效的AI消息ID")
	if !ok {
		return
	}
	var req regenerationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("aimessage.regeneration.request.invalid", nil, "AI回复重新生成参数错误"))
		return
	}
	res, appErr := h.requireHistoryService().Regenerate(c.Request.Context(), identity.UserID, aimessagemodule.RegenerateInput{
		ConversationID: conversationID, AssistantMessageID: messageID, RequestID: req.RequestID,
	})
	writeAcceptedResult(c, res, appErr)
}

func (h *Handler) DeleteMessages(c *gin.Context) {
	identity, ok := authIdentity(c)
	if !ok {
		return
	}
	conversationID, ok := routeID(c, "id", "无效的AI会话ID")
	if !ok {
		return
	}
	var req deleteMessagesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("aimessage.delete.request.invalid", nil, "AI消息删除参数错误"))
		return
	}
	res, appErr := h.requireHistoryService().DeleteMessages(c.Request.Context(), identity.UserID, aimessagemodule.DeleteInput{
		ConversationID: conversationID, IDs: req.IDs,
	})
	writeResult(c, res, appErr)
}

func (h *Handler) requireService() aimessagemodule.HTTPService {
	if h == nil || h.service == nil {
		return nilHTTPService{}
	}
	return h.service
}

func (h *Handler) requireHistoryService() aimessagemodule.HistoryHTTPService {
	if h == nil || h.history == nil {
		return nilHistoryHTTPService{}
	}
	return h.history
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

func writeAcceptedResult(c *gin.Context, res any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Accepted(c, res)
}

type nilHTTPService struct{}

type nilHistoryHTTPService struct{}

func (nilHTTPService) List(ctx context.Context, userID int64, query aimessagemodule.ListQuery) (*aimessagemodule.ListResponse, *apperror.Error) {
	return nil, apperror.Internal("AI消息服务未配置")
}
func (nilHTTPService) Send(ctx context.Context, userID int64, input aimessagemodule.SendInput) (*aimessagemodule.SendResponse, *apperror.Error) {
	return nil, apperror.Internal("AI消息服务未配置")
}
func (nilHTTPService) Cancel(ctx context.Context, userID int64, input aimessagemodule.CancelInput) (*aimessagemodule.CancelResponse, *apperror.Error) {
	return nil, apperror.Internal("AI消息服务未配置")
}

func (nilHistoryHTTPService) Revise(context.Context, int64, aimessagemodule.EditInput) (*aimessagemodule.SendResponse, *apperror.Error) {
	return nil, apperror.InternalKey("aimessage.service_missing", nil, "AI消息服务未配置")
}

func (nilHistoryHTTPService) Regenerate(context.Context, int64, aimessagemodule.RegenerateInput) (*aimessagemodule.SendResponse, *apperror.Error) {
	return nil, apperror.InternalKey("aimessage.service_missing", nil, "AI消息服务未配置")
}

func (nilHistoryHTTPService) DeleteMessages(context.Context, int64, aimessagemodule.DeleteInput) (*aimessagemodule.DeleteResponse, *apperror.Error) {
	return nil, apperror.InternalKey("aimessage.service_missing", nil, "AI消息服务未配置")
}
