package admin

import (
	"context"
	"strconv"

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
	res, appErr := h.requireService().PageInit(c.Request.Context())
	writeResult(c, res, appErr)
}

func (h *Handler) List(c *gin.Context) {
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI运行列表参数错误"))
		return
	}
	res, appErr := h.requireService().List(c.Request.Context(), airunmodule.ListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, Status: req.Status, UserID: req.UserID, RequestID: req.RequestID, AgentID: req.AgentID, ProviderID: req.ProviderID, DateStart: req.DateStart, DateEnd: req.DateEnd})
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

func (h *Handler) Stats(c *gin.Context) {
	var req statsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI运行统计参数错误"))
		return
	}
	res, appErr := h.requireService().Stats(c.Request.Context(), airunmodule.StatsFilter{DateStart: req.DateStart, DateEnd: req.DateEnd, AgentID: req.AgentID, ProviderID: req.ProviderID, UserID: req.UserID})
	writeResult(c, res, appErr)
}

func (h *Handler) StatsByDate(c *gin.Context) {
	query, ok := bindStatsList(c)
	if !ok {
		return
	}
	res, appErr := h.requireService().StatsByDate(c.Request.Context(), query)
	writeResult(c, res, appErr)
}

func (h *Handler) StatsByAgent(c *gin.Context) {
	query, ok := bindStatsList(c)
	if !ok {
		return
	}
	res, appErr := h.requireService().StatsByAgent(c.Request.Context(), query)
	writeResult(c, res, appErr)
}

func (h *Handler) StatsByUser(c *gin.Context) {
	query, ok := bindStatsList(c)
	if !ok {
		return
	}
	res, appErr := h.requireService().StatsByUser(c.Request.Context(), query)
	writeResult(c, res, appErr)
}

func (h *Handler) requireService() airunmodule.HTTPService {
	if h == nil || h.service == nil {
		return nilHTTPService{}
	}
	return h.service
}

func bindStatsList(c *gin.Context) (airunmodule.StatsListQuery, bool) {
	var req statsListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI运行统计列表参数错误"))
		return airunmodule.StatsListQuery{}, false
	}
	return airunmodule.StatsListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, DateStart: req.DateStart, DateEnd: req.DateEnd, AgentID: req.AgentID, ProviderID: req.ProviderID, UserID: req.UserID}, true
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

func (nilHTTPService) PageInit(ctx context.Context) (*airunmodule.InitResponse, *apperror.Error) {
	return nil, apperror.Internal("AI运行服务未配置")
}
func (nilHTTPService) List(ctx context.Context, query airunmodule.ListQuery) (*airunmodule.ListResponse, *apperror.Error) {
	return nil, apperror.Internal("AI运行服务未配置")
}
func (nilHTTPService) Detail(ctx context.Context, id int64) (*airunmodule.DetailResponse, *apperror.Error) {
	return nil, apperror.Internal("AI运行服务未配置")
}
func (nilHTTPService) Stats(ctx context.Context, query airunmodule.StatsFilter) (*airunmodule.StatsResponse, *apperror.Error) {
	return nil, apperror.Internal("AI运行服务未配置")
}
func (nilHTTPService) StatsByDate(ctx context.Context, query airunmodule.StatsListQuery) (*airunmodule.StatsByDateResponse, *apperror.Error) {
	return nil, apperror.Internal("AI运行服务未配置")
}
func (nilHTTPService) StatsByAgent(ctx context.Context, query airunmodule.StatsListQuery) (*airunmodule.StatsByAgentResponse, *apperror.Error) {
	return nil, apperror.Internal("AI运行服务未配置")
}
func (nilHTTPService) StatsByUser(ctx context.Context, query airunmodule.StatsListQuery) (*airunmodule.StatsByUserResponse, *apperror.Error) {
	return nil, apperror.Internal("AI运行服务未配置")
}
