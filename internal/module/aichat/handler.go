package aichat

import (
	"context"
	"strconv"
	"time"

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

func (h *Handler) CreateRun(c *gin.Context) {
	identity, ok := authIdentity(c)
	if !ok {
		return
	}
	var req createRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI对话参数错误"))
		return
	}
	res, appErr := h.requireService().CreateRun(c.Request.Context(), identity.UserID, createInput(req))
	writeResult(c, res, appErr)
}

func (h *Handler) SendMessage(c *gin.Context) {
	h.CreateRun(c)
}

func (h *Handler) Events(c *gin.Context) {
	identity, ok := authIdentity(c)
	if !ok {
		return
	}
	runID, ok := routeID(c, "run_id", "无效的AI运行ID")
	if !ok {
		return
	}
	var req eventsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI运行事件参数错误"))
		return
	}
	res, appErr := h.requireService().Events(c.Request.Context(), identity.UserID, runID, req.LastID, time.Duration(req.TimeoutMS)*time.Millisecond)
	writeResult(c, res, appErr)
}

func (h *Handler) Cancel(c *gin.Context) {
	identity, ok := authIdentity(c)
	if !ok {
		return
	}
	runID, ok := routeID(c, "run_id", "无效的AI运行ID")
	if !ok {
		return
	}
	res, appErr := h.requireService().Cancel(c.Request.Context(), identity.UserID, runID)
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

func createInput(req createRunRequest) CreateRunInput {
	return CreateRunInput{Content: req.Content, ConversationID: req.ConversationID, AgentID: req.AgentID, MaxHistory: req.MaxHistory, Attachments: req.Attachments, Temperature: req.Temperature, MaxTokens: req.MaxTokens}
}

func writeResult(c *gin.Context, res any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, res)
}

type nilHTTPService struct{}

func (nilHTTPService) CreateRun(ctx context.Context, userID int64, input CreateRunInput) (*CreateRunResponse, *apperror.Error) {
	return nil, apperror.Internal("AI对话服务未配置")
}
func (nilHTTPService) Events(ctx context.Context, userID int64, runID int64, lastID string, timeout time.Duration) (*EventsResponse, *apperror.Error) {
	return nil, apperror.Internal("AI对话服务未配置")
}
func (nilHTTPService) Cancel(ctx context.Context, userID int64, runID int64) (*CancelResponse, *apperror.Error) {
	return nil, apperror.Internal("AI对话服务未配置")
}
