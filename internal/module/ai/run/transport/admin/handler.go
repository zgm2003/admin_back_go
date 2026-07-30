package admin

import (
	"context"
	"strconv"

	"admin_back_go/internal/middleware"
	airunmodule "admin_back_go/internal/module/ai/run"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service airunmodule.HTTPService
}

func NewHandler(service airunmodule.HTTPService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) PageInit(c *gin.Context) {
	var req pageInitRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI运行初始化参数错误"))
		return
	}
	res, appErr := h.requireService().PageInit(c.Request.Context(), airunmodule.PageInitFilter{DateStart: req.DateStart, DateEnd: req.DateEnd})
	writeResult(c, res, appErr)
}

func (h *Handler) List(c *gin.Context) {
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI运行列表参数错误"))
		return
	}
	res, appErr := h.requireService().List(c.Request.Context(), airunmodule.ListQuery{
		CurrentPage: req.CurrentPage, PageSize: req.PageSize, Platform: req.Platform, Status: req.Status,
		UserID: req.UserID, RequestID: req.RequestID, AgentID: req.AgentID, ProviderID: req.ProviderID,
		ModelID: req.ModelID, BillingStatus: req.BillingStatus, BillingReason: req.BillingReason,
		ErrorCode: req.ErrorCode, ToolCode: req.ToolCode, RunAnomaly: req.RunAnomaly,
		BillingAnomaly: req.BillingAnomaly, UserFeedback: req.UserFeedback, AnomalyAsOf: req.AnomalyAsOf,
		DateStart: req.DateStart, DateEnd: req.DateEnd,
	})
	writeResult(c, res, appErr)
}

func (h *Handler) Detail(c *gin.Context) {
	id, ok := routeID(c, "id", "无效的AI运行ID")
	if !ok {
		return
	}
	res, appErr := h.requireService().Detail(c.Request.Context(), id)
	writeResult(c, res, appErr)
}

func (h *Handler) SetUserFeedback(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.UnauthorizedKey("airun.feedback.unauthorized", nil, "Token无效或已过期"))
		return
	}
	id, ok := routeID(c, "id", "无效的AI运行ID")
	if !ok {
		return
	}
	var req feedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Liked == nil {
		response.Error(c, apperror.BadRequestKey("airun.feedback.request_invalid", nil, "AI运行反馈参数错误"))
		return
	}
	res, appErr := h.requireFeedbackService().SetUserFeedback(c.Request.Context(), identity.UserID, id, *req.Liked)
	writeResult(c, res, appErr)
}

func (h *Handler) Dashboard(c *gin.Context) {
	var req dashboardRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI运行驾驶舱参数错误"))
		return
	}
	res, appErr := h.requireService().Dashboard(c.Request.Context(), airunmodule.DashboardFilter{
		RequestID: middleware.GetRequestID(c),
		DateStart: req.DateStart, DateEnd: req.DateEnd, Platform: req.Platform, ModelID: req.ModelID,
		AgentID: req.AgentID, ProviderID: req.ProviderID, UserID: req.UserID,
	})
	writeResult(c, res, appErr)
}

func (h *Handler) requireService() airunmodule.HTTPService {
	if h == nil || h.service == nil {
		return nilHTTPService{}
	}
	return h.service
}

func (h *Handler) requireFeedbackService() airunmodule.FeedbackHTTPService {
	if h != nil && h.service != nil {
		if service, ok := h.service.(airunmodule.FeedbackHTTPService); ok {
			return service
		}
	}
	return nilHTTPService{}
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

func (nilHTTPService) PageInit(ctx context.Context, filter airunmodule.PageInitFilter) (*airunmodule.InitResponse, *apperror.Error) {
	return nil, apperror.Internal("AI运行服务未配置")
}
func (nilHTTPService) List(ctx context.Context, query airunmodule.ListQuery) (*airunmodule.ListResponse, *apperror.Error) {
	return nil, apperror.Internal("AI运行服务未配置")
}
func (nilHTTPService) Detail(ctx context.Context, id int64) (*airunmodule.DetailResponse, *apperror.Error) {
	return nil, apperror.Internal("AI运行服务未配置")
}
func (nilHTTPService) Dashboard(ctx context.Context, filter airunmodule.DashboardFilter) (*airunmodule.DashboardResponse, *apperror.Error) {
	return nil, apperror.Internal("AI运行服务未配置")
}
func (nilHTTPService) SetUserFeedback(ctx context.Context, userID int64, id int64, liked bool) (*airunmodule.FeedbackResponse, *apperror.Error) {
	return nil, apperror.Internal("AI运行服务未配置")
}
