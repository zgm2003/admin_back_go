package aitoolmap

import (
	"context"
	"strconv"

	"admin_back_go/internal/apperror"
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
		response.Error(c, apperror.BadRequest("AI工具映射列表参数错误"))
		return
	}
	result, appErr := h.requireService().List(c.Request.Context(), ListQuery{
		CurrentPage: req.CurrentPage,
		PageSize:    req.PageSize,
		Name:        req.Name,
		Code:        req.Code,
		ToolType:    req.ToolType,
		RiskLevel:   req.RiskLevel,
		ProviderID:  req.ProviderID,
		AgentID:     req.AgentID,
		Status:      req.Status,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) Create(c *gin.Context) {
	var req mutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI工具映射参数错误"))
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
	id, ok := routeID(c, "无效的AI工具映射ID")
	if !ok {
		return
	}
	var req mutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI工具映射参数错误"))
		return
	}
	if appErr := h.requireService().Update(c.Request.Context(), id, mutationInput(req)); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) ChangeStatus(c *gin.Context) {
	id, ok := routeID(c, "无效的AI工具映射ID")
	if !ok {
		return
	}
	var req statusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI工具映射状态参数错误"))
		return
	}
	if appErr := h.requireService().ChangeStatus(c.Request.Context(), id, req.Status); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) Delete(c *gin.Context) {
	id, ok := routeID(c, "无效的AI工具映射ID")
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

func mutationInput(req mutationRequest) MutationInput {
	return MutationInput{
		ProviderID:     req.ProviderID,
		AgentID:        req.AgentID,
		Name:           req.Name,
		Code:           req.Code,
		ToolType:       req.ToolType,
		EngineToolID:   req.EngineToolID,
		PermissionCode: req.PermissionCode,
		RiskLevel:      req.RiskLevel,
		ConfigJSON:     req.ConfigJSON,
		Status:         req.Status,
	}
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
	return nil, apperror.Internal("AI工具映射服务未配置")
}

func (nilHTTPService) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	return nil, apperror.Internal("AI工具映射服务未配置")
}

func (nilHTTPService) Create(ctx context.Context, input MutationInput) (uint64, *apperror.Error) {
	return 0, apperror.Internal("AI工具映射服务未配置")
}

func (nilHTTPService) Update(ctx context.Context, id uint64, input MutationInput) *apperror.Error {
	return apperror.Internal("AI工具映射服务未配置")
}

func (nilHTTPService) ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	return apperror.Internal("AI工具映射服务未配置")
}

func (nilHTTPService) Delete(ctx context.Context, id uint64) *apperror.Error {
	return apperror.Internal("AI工具映射服务未配置")
}
