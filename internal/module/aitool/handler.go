package aitool

import (
	"context"
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
	result, appErr := h.requireService().Init(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) List(c *gin.Context) {
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI工具列表参数错误"))
		return
	}
	result, appErr := h.requireService().List(c.Request.Context(), ListQuery{
		CurrentPage:  req.CurrentPage,
		PageSize:     req.PageSize,
		Name:         req.Name,
		Status:       req.Status,
		ExecutorType: req.ExecutorType,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) Create(c *gin.Context) {
	var req mutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI工具参数错误"))
		return
	}
	id, appErr := h.requireService().Create(c.Request.Context(), createInput(req))
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{"id": id})
}

func (h *Handler) Update(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req mutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI工具参数错误"))
		return
	}
	if appErr := h.requireService().Update(c.Request.Context(), id, updateInput(req)); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) ChangeStatus(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req statusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI工具状态参数错误"))
		return
	}
	if appErr := h.requireService().ChangeStatus(c.Request.Context(), id, req.Status); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) Delete(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	if appErr := h.requireService().Delete(c.Request.Context(), id); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) AgentOptions(c *gin.Context) {
	var req agentOptionsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("智能体工具选项参数错误"))
		return
	}
	result, appErr := h.requireService().AgentOptions(c.Request.Context(), req.AgentID)
	writeResult(c, result, appErr)
}

func (h *Handler) SyncAgentBindings(c *gin.Context) {
	agentID, ok := agentID(c)
	if !ok {
		return
	}
	var req bindingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("智能体工具绑定参数错误"))
		return
	}
	if appErr := h.requireService().SyncAgentBindings(c.Request.Context(), agentID, req.ToolIDs); appErr != nil {
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

func routeID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequest("无效的AI工具ID"))
		return 0, false
	}
	return id, true
}

func agentID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("agent_id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequest("无效的智能体ID"))
		return 0, false
	}
	return id, true
}

func createInput(req mutationRequest) CreateInput {
	return CreateInput{
		Name: req.Name, Code: req.Code, Description: req.Description, SchemaJSON: req.SchemaJSON,
		ExecutorType: req.ExecutorType, ExecutorConfig: req.ExecutorConfig, Status: req.Status,
	}
}

func updateInput(req mutationRequest) UpdateInput {
	return UpdateInput(createInput(req))
}

func writeResult(c *gin.Context, result any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

type nilHTTPService struct{}

func (nilHTTPService) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return nil, apperror.Internal("AI工具服务未配置")
}
func (nilHTTPService) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	return nil, apperror.Internal("AI工具服务未配置")
}
func (nilHTTPService) Create(ctx context.Context, input CreateInput) (int64, *apperror.Error) {
	return 0, apperror.Internal("AI工具服务未配置")
}
func (nilHTTPService) Update(ctx context.Context, id int64, input UpdateInput) *apperror.Error {
	return apperror.Internal("AI工具服务未配置")
}
func (nilHTTPService) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	return apperror.Internal("AI工具服务未配置")
}
func (nilHTTPService) Delete(ctx context.Context, id int64) *apperror.Error {
	return apperror.Internal("AI工具服务未配置")
}
func (nilHTTPService) AgentOptions(ctx context.Context, agentID int64) (*AgentToolsResponse, *apperror.Error) {
	return nil, apperror.Internal("AI工具服务未配置")
}
func (nilHTTPService) SyncAgentBindings(ctx context.Context, agentID int64, toolIDs []int64) *apperror.Error {
	return apperror.Internal("AI工具服务未配置")
}
