package aiconversation

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
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI会话列表参数错误"))
		return
	}
	res, appErr := h.requireService().List(c.Request.Context(), identity.UserID, ListQuery{AgentID: req.AgentID, BeforeID: req.BeforeID, Limit: req.Limit})
	writeResult(c, res, appErr)
}

func (h *Handler) Detail(c *gin.Context) {
	identity, ok := authIdentity(c)
	if !ok {
		return
	}
	id, ok := routeID(c, "id", "无效的AI会话ID")
	if !ok {
		return
	}
	res, appErr := h.requireService().Detail(c.Request.Context(), identity.UserID, id)
	writeResult(c, res, appErr)
}

func (h *Handler) Create(c *gin.Context) {
	identity, ok := authIdentity(c)
	if !ok {
		return
	}
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI会话参数错误"))
		return
	}
	id, appErr := h.requireService().Create(c.Request.Context(), identity.UserID, CreateInput{AgentID: req.AgentID, Title: req.Title})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, CreateResponse{ID: id})
}

func (h *Handler) Update(c *gin.Context) {
	identity, ok := authIdentity(c)
	if !ok {
		return
	}
	id, ok := routeID(c, "id", "无效的AI会话ID")
	if !ok {
		return
	}
	var req updateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI会话参数错误"))
		return
	}
	if appErr := h.requireService().Update(c.Request.Context(), identity.UserID, id, UpdateInput{Title: req.Title}); appErr != nil {
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
	id, ok := routeID(c, "id", "无效的AI会话ID")
	if !ok {
		return
	}
	if appErr := h.requireService().Delete(c.Request.Context(), identity.UserID, id); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
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
	return nil, apperror.Internal("AI会话服务未配置")
}
func (nilHTTPService) Detail(ctx context.Context, userID int64, id int64) (*ConversationDetail, *apperror.Error) {
	return nil, apperror.Internal("AI会话服务未配置")
}
func (nilHTTPService) Create(ctx context.Context, userID int64, input CreateInput) (int64, *apperror.Error) {
	return 0, apperror.Internal("AI会话服务未配置")
}
func (nilHTTPService) Update(ctx context.Context, userID int64, id int64, input UpdateInput) *apperror.Error {
	return apperror.Internal("AI会话服务未配置")
}
func (nilHTTPService) Delete(ctx context.Context, userID int64, id int64) *apperror.Error {
	return apperror.Internal("AI会话服务未配置")
}
