package admin

import (
	"context"
	"strconv"

	"admin_back_go/internal/middleware"
	aitoolmodule "admin_back_go/internal/module/ai/tool"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type Handler struct{ service aitoolmodule.HTTPService }

func NewHandler(service aitoolmodule.HTTPService) *Handler { return &Handler{service: service} }

func (h *Handler) PageInit(c *gin.Context) {
	result, appErr := h.requireService().PageInit(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) List(c *gin.Context) {
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI工具列表参数错误"))
		return
	}
	result, appErr := h.requireService().List(c.Request.Context(), aitoolmodule.ListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, Name: req.Name, Code: req.Code, RiskLevel: req.RiskLevel, Status: req.Status})
	writeResult(c, result, appErr)
}

func (h *Handler) GeneratePageInit(c *gin.Context) {
	result, appErr := h.requireService().GeneratePageInit(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) GenerateDraft(c *gin.Context) {
	var req generateDraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI工具生成参数错误"))
		return
	}
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return
	}
	result, appErr := h.requireService().GenerateDraft(c.Request.Context(), aitoolmodule.GenerateDraftInput{AgentID: req.AgentID, UserID: uint64(identity.UserID), Requirement: req.Requirement, CodeHint: req.CodeHint})
	writeResult(c, result, appErr)
}

func (h *Handler) Create(c *gin.Context) {
	var req mutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI工具参数错误"))
		return
	}
	id, appErr := h.requireService().Create(c.Request.Context(), mutationInput(req))
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{"id": id})
}

func (h *Handler) Update(c *gin.Context) {
	id, ok := routeID(c, "无效的AI工具ID")
	if !ok {
		return
	}
	var req mutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI工具参数错误"))
		return
	}
	if appErr := h.requireService().Update(c.Request.Context(), id, mutationInput(req)); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) ChangeStatus(c *gin.Context) {
	id, ok := routeID(c, "无效的AI工具ID")
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
	id, ok := routeID(c, "无效的AI工具ID")
	if !ok {
		return
	}
	if appErr := h.requireService().Delete(c.Request.Context(), id); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) AgentTools(c *gin.Context) {
	agentID, ok := routeID(c, "无效的AI智能体ID")
	if !ok {
		return
	}
	result, appErr := h.requireService().AgentTools(c.Request.Context(), agentID)
	writeResult(c, result, appErr)
}

func (h *Handler) UpdateAgentTools(c *gin.Context) {
	agentID, ok := routeID(c, "无效的AI智能体ID")
	if !ok {
		return
	}
	var req updateAgentToolsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("智能体工具绑定参数错误"))
		return
	}
	if appErr := h.requireService().UpdateAgentTools(c.Request.Context(), agentID, aitoolmodule.UpdateAgentToolsInput{ToolIDs: req.ToolIDs}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) requireService() aitoolmodule.HTTPService {
	if h == nil || h.service == nil {
		return nilHTTPService{}
	}
	return h.service
}

func routeID(c *gin.Context, message string) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, apperror.BadRequest(message))
		return 0, false
	}
	return id, true
}

func mutationInput(req mutationRequest) aitoolmodule.MutationInput {
	return aitoolmodule.MutationInput{Name: req.Name, Code: req.Code, Description: req.Description, ParametersJSON: req.ParametersJSON, ResultSchemaJSON: req.ResultSchemaJSON, RiskLevel: req.RiskLevel, TimeoutMS: req.TimeoutMS, Status: req.Status}
}

func writeResult(c *gin.Context, result any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

type nilHTTPService struct{}

func (nilHTTPService) PageInit(ctx context.Context) (*aitoolmodule.InitResponse, *apperror.Error) {
	return nil, apperror.Internal("AI工具服务未配置")
}
func (nilHTTPService) List(ctx context.Context, query aitoolmodule.ListQuery) (*aitoolmodule.ListResponse, *apperror.Error) {
	return nil, apperror.Internal("AI工具服务未配置")
}
func (nilHTTPService) GeneratePageInit(ctx context.Context) (*aitoolmodule.GeneratePageInitResponse, *apperror.Error) {
	return nil, apperror.Internal("AI工具服务未配置")
}
func (nilHTTPService) GenerateDraft(ctx context.Context, input aitoolmodule.GenerateDraftInput) (*aitoolmodule.GenerateDraftResponse, *apperror.Error) {
	return nil, apperror.Internal("AI工具服务未配置")
}
func (nilHTTPService) Create(ctx context.Context, input aitoolmodule.MutationInput) (uint64, *apperror.Error) {
	return 0, apperror.Internal("AI工具服务未配置")
}
func (nilHTTPService) Update(ctx context.Context, id uint64, input aitoolmodule.MutationInput) *apperror.Error {
	return apperror.Internal("AI工具服务未配置")
}
func (nilHTTPService) ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	return apperror.Internal("AI工具服务未配置")
}
func (nilHTTPService) Delete(ctx context.Context, id uint64) *apperror.Error {
	return apperror.Internal("AI工具服务未配置")
}
func (nilHTTPService) AgentTools(ctx context.Context, agentID uint64) (*aitoolmodule.AgentToolsResponse, *apperror.Error) {
	return nil, apperror.Internal("AI工具服务未配置")
}
func (nilHTTPService) UpdateAgentTools(ctx context.Context, agentID uint64, input aitoolmodule.UpdateAgentToolsInput) *apperror.Error {
	return apperror.Internal("AI工具服务未配置")
}
