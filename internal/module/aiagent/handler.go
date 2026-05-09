package aiagent

import (
	"context"
	"strconv"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/middleware"
	platformai "admin_back_go/internal/platform/ai"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type Handler struct{ service HTTPService }

func NewHandler(service HTTPService) *Handler { return &Handler{service: service} }

func (h *Handler) Init(c *gin.Context) {
	result, appErr := h.requireService().Init(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) List(c *gin.Context) {
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI智能体列表参数错误"))
		return
	}
	result, appErr := h.requireService().List(c.Request.Context(), ListQuery{
		CurrentPage: req.CurrentPage,
		PageSize:    req.PageSize,
		Name:        req.Name,
		Scene:       req.Scene,
		ProviderID:  req.ProviderID,
		Status:      req.Status,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) Options(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return
	}
	result, appErr := h.requireService().Options(c.Request.Context(), OptionQuery{
		UserID: identity.UserID,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) ProviderModels(c *gin.Context) {
	providerID, ok := routeID(c, "无效的AI供应商ID")
	if !ok {
		return
	}
	result, appErr := h.requireService().ProviderModels(c.Request.Context(), providerID)
	writeResult(c, result, appErr)
}

func (h *Handler) Detail(c *gin.Context) {
	id, ok := routeID(c, "无效的AI智能体ID")
	if !ok {
		return
	}
	result, appErr := h.requireService().Detail(c.Request.Context(), id)
	writeResult(c, result, appErr)
}

func (h *Handler) Create(c *gin.Context) {
	var req mutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI智能体参数错误"))
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
	id, ok := routeID(c, "无效的AI智能体ID")
	if !ok {
		return
	}
	var req mutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI智能体参数错误"))
		return
	}
	if appErr := h.requireService().Update(c.Request.Context(), id, updateInput(req)); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) ChangeStatus(c *gin.Context) {
	id, ok := routeID(c, "无效的AI智能体ID")
	if !ok {
		return
	}
	var req statusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI智能体状态参数错误"))
		return
	}
	if appErr := h.requireService().ChangeStatus(c.Request.Context(), id, req.Status); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) Test(c *gin.Context) {
	id, ok := routeID(c, "无效的AI智能体ID")
	if !ok {
		return
	}
	result, appErr := h.requireService().Test(c.Request.Context(), id)
	writeResult(c, result, appErr)
}

func (h *Handler) Delete(c *gin.Context) {
	id, ok := routeID(c, "无效的AI智能体ID")
	if !ok {
		return
	}
	if appErr := h.requireService().Delete(c.Request.Context(), id); appErr != nil {
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

func routeID(c *gin.Context, message string) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, apperror.BadRequest(message))
		return 0, false
	}
	return id, true
}

func createInput(req mutationRequest) CreateInput {
	return CreateInput{
		ProviderID:   req.ProviderID,
		Name:         req.Name,
		ModelID:      req.ModelID,
		Scenes:       req.Scenes,
		SystemPrompt: req.SystemPrompt,
		Avatar:       req.Avatar,
		Status:       req.Status,
	}
}

func updateInput(req mutationRequest) UpdateInput { return UpdateInput(createInput(req)) }

func writeResult(c *gin.Context, result any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

type nilHTTPService struct{}

func (nilHTTPService) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return nil, apperror.Internal("AI智能体服务未配置")
}
func (nilHTTPService) ProviderModels(ctx context.Context, providerID uint64) (*ProviderModelsResponse, *apperror.Error) {
	return nil, apperror.Internal("AI智能体服务未配置")
}
func (nilHTTPService) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	return nil, apperror.Internal("AI智能体服务未配置")
}
func (nilHTTPService) Detail(ctx context.Context, id uint64) (*DetailResponse, *apperror.Error) {
	return nil, apperror.Internal("AI智能体服务未配置")
}
func (nilHTTPService) Create(ctx context.Context, input CreateInput) (uint64, *apperror.Error) {
	return 0, apperror.Internal("AI智能体服务未配置")
}
func (nilHTTPService) Update(ctx context.Context, id uint64, input UpdateInput) *apperror.Error {
	return apperror.Internal("AI智能体服务未配置")
}
func (nilHTTPService) ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	return apperror.Internal("AI智能体服务未配置")
}
func (nilHTTPService) Test(ctx context.Context, id uint64) (*platformai.TestConnectionResult, *apperror.Error) {
	return nil, apperror.Internal("AI智能体服务未配置")
}
func (nilHTTPService) Delete(ctx context.Context, id uint64) *apperror.Error {
	return apperror.Internal("AI智能体服务未配置")
}
func (nilHTTPService) Options(ctx context.Context, query OptionQuery) (*AgentOptionsResponse, *apperror.Error) {
	return nil, apperror.Internal("AI智能体服务未配置")
}
