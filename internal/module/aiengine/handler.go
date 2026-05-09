package aiengine

import (
	"context"
	"strconv"

	"admin_back_go/internal/apperror"
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
		response.Error(c, apperror.BadRequest("AI供应商列表参数错误"))
		return
	}
	result, appErr := h.requireService().List(c.Request.Context(), ListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, Name: req.Name, EngineType: req.EngineType, Status: req.Status})
	writeResult(c, result, appErr)
}

func (h *Handler) PreviewModels(c *gin.Context) {
	var req modelOptionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI供应商模型拉取参数错误"))
		return
	}
	result, appErr := h.requireService().PreviewModels(c.Request.Context(), modelOptionsInput(req))
	writeResult(c, result, appErr)
}

func (h *Handler) PreviewStoredModels(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	result, appErr := h.requireService().PreviewStoredModels(c.Request.Context(), id)
	writeResult(c, result, appErr)
}

func (h *Handler) Create(c *gin.Context) {
	var req mutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI供应商参数错误"))
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
		response.Error(c, apperror.BadRequest("AI供应商参数错误"))
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
		response.Error(c, apperror.BadRequest("AI供应商状态参数错误"))
		return
	}
	if appErr := h.requireService().ChangeStatus(c.Request.Context(), id, req.Status); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) TestConnection(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	result, appErr := h.requireService().TestConnection(c.Request.Context(), id)
	writeResult(c, result, appErr)
}

func (h *Handler) SyncModels(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	result, appErr := h.requireService().SyncModels(c.Request.Context(), id)
	writeResult(c, result, appErr)
}

func (h *Handler) ListProviderModels(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	result, appErr := h.requireService().ListProviderModels(c.Request.Context(), id)
	writeResult(c, result, appErr)
}

func (h *Handler) UpdateProviderModels(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req updateModelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI供应商模型参数错误"))
		return
	}
	if appErr := h.requireService().UpdateProviderModels(c.Request.Context(), id, updateModelsInput(req)); appErr != nil {
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

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return nilHTTPService{}
	}
	return h.service
}

func routeID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, apperror.BadRequest("无效的AI供应商ID"))
		return 0, false
	}
	return id, true
}

func createInput(req mutationRequest) CreateInput {
	return CreateInput{Name: req.Name, EngineType: req.EngineType, Driver: req.Driver, BaseURL: req.BaseURL, APIKey: req.APIKey, ModelIDs: req.ModelIDs, ModelDisplayNames: req.ModelDisplayNames, Status: req.Status}
}
func updateInput(req mutationRequest) UpdateInput { return UpdateInput(createInput(req)) }
func modelOptionsInput(req modelOptionsRequest) ModelOptionsInput {
	return ModelOptionsInput{EngineType: req.EngineType, Driver: req.Driver, BaseURL: req.BaseURL, APIKey: req.APIKey}
}
func updateModelsInput(req updateModelsRequest) UpdateModelsInput {
	return UpdateModelsInput{ModelIDs: req.ModelIDs, ModelDisplayNames: req.ModelDisplayNames, Statuses: req.Statuses}
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
	return nil, apperror.Internal("AI供应商服务未配置")
}
func (nilHTTPService) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	return nil, apperror.Internal("AI供应商服务未配置")
}
func (nilHTTPService) Create(ctx context.Context, input CreateInput) (uint64, *apperror.Error) {
	return 0, apperror.Internal("AI供应商服务未配置")
}
func (nilHTTPService) Update(ctx context.Context, id uint64, input UpdateInput) *apperror.Error {
	return apperror.Internal("AI供应商服务未配置")
}
func (nilHTTPService) ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	return apperror.Internal("AI供应商服务未配置")
}
func (nilHTTPService) TestConnection(ctx context.Context, id uint64) (*platformai.TestConnectionResult, *apperror.Error) {
	return nil, apperror.Internal("AI供应商服务未配置")
}
func (nilHTTPService) PreviewModels(ctx context.Context, input ModelOptionsInput) (*ModelOptionsResponse, *apperror.Error) {
	return nil, apperror.Internal("AI供应商服务未配置")
}
func (nilHTTPService) PreviewStoredModels(ctx context.Context, id uint64) (*ModelOptionsResponse, *apperror.Error) {
	return nil, apperror.Internal("AI供应商服务未配置")
}
func (nilHTTPService) SyncModels(ctx context.Context, id uint64) (*ModelOptionsResponse, *apperror.Error) {
	return nil, apperror.Internal("AI供应商服务未配置")
}
func (nilHTTPService) ListProviderModels(ctx context.Context, id uint64) (*ProviderModelsResponse, *apperror.Error) {
	return nil, apperror.Internal("AI供应商服务未配置")
}
func (nilHTTPService) UpdateProviderModels(ctx context.Context, id uint64, input UpdateModelsInput) *apperror.Error {
	return apperror.Internal("AI供应商服务未配置")
}
func (nilHTTPService) Delete(ctx context.Context, id uint64) *apperror.Error {
	return apperror.Internal("AI供应商服务未配置")
}
