package aiknowledgemap

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
		response.Error(c, apperror.BadRequest("AI知识库列表参数错误"))
		return
	}
	result, appErr := h.requireService().List(c.Request.Context(), ListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, Name: req.Name, Code: req.Code, Visibility: req.Visibility, ProviderID: req.ProviderID, Status: req.Status})
	writeResult(c, result, appErr)
}

func (h *Handler) Detail(c *gin.Context) {
	id, ok := routeID(c, "无效的AI知识库ID")
	if !ok {
		return
	}
	result, appErr := h.requireService().Detail(c.Request.Context(), id)
	writeResult(c, result, appErr)
}

func (h *Handler) Create(c *gin.Context) {
	var req mapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI知识库参数错误"))
		return
	}
	id, appErr := h.requireService().Create(c.Request.Context(), mapInput(req))
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{"id": id})
}

func (h *Handler) Update(c *gin.Context) {
	id, ok := routeID(c, "无效的AI知识库ID")
	if !ok {
		return
	}
	var req mapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI知识库参数错误"))
		return
	}
	if appErr := h.requireService().Update(c.Request.Context(), id, mapInput(req)); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) ChangeStatus(c *gin.Context) {
	id, ok := routeID(c, "无效的AI知识库ID")
	if !ok {
		return
	}
	var req statusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI知识库状态参数错误"))
		return
	}
	if appErr := h.requireService().ChangeStatus(c.Request.Context(), id, req.Status); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) Sync(c *gin.Context) {
	id, ok := routeID(c, "无效的AI知识库ID")
	if !ok {
		return
	}
	if appErr := h.requireService().Sync(c.Request.Context(), id); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) Delete(c *gin.Context) {
	id, ok := routeID(c, "无效的AI知识库ID")
	if !ok {
		return
	}
	if appErr := h.requireService().Delete(c.Request.Context(), id); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) Documents(c *gin.Context) {
	id, ok := routeID(c, "无效的AI知识库ID")
	if !ok {
		return
	}
	result, appErr := h.requireService().Documents(c.Request.Context(), id)
	writeResult(c, result, appErr)
}

func (h *Handler) CreateDocument(c *gin.Context) {
	id, ok := routeID(c, "无效的AI知识库ID")
	if !ok {
		return
	}
	var req documentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI知识库文档参数错误"))
		return
	}
	docID, appErr := h.requireService().CreateDocument(c.Request.Context(), id, documentInput(req))
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{"id": docID})
}

func (h *Handler) ChangeDocumentStatus(c *gin.Context) {
	id, ok := routeID(c, "无效的AI知识库文档ID")
	if !ok {
		return
	}
	var req statusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI知识库文档状态参数错误"))
		return
	}
	if appErr := h.requireService().ChangeDocumentStatus(c.Request.Context(), id, req.Status); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) RefreshDocumentStatus(c *gin.Context) {
	id, ok := routeID(c, "无效的AI知识库文档ID")
	if !ok {
		return
	}
	if appErr := h.requireService().RefreshDocumentStatus(c.Request.Context(), id); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) DeleteDocument(c *gin.Context) {
	id, ok := routeID(c, "无效的AI知识库文档ID")
	if !ok {
		return
	}
	if appErr := h.requireService().DeleteDocument(c.Request.Context(), id); appErr != nil {
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

func mapInput(req mapRequest) MapInput {
	return MapInput{ProviderID: req.ProviderID, Name: req.Name, Code: req.Code, EngineDatasetID: req.EngineDatasetID, Visibility: req.Visibility, MetaJSON: req.MetaJSON, Status: req.Status}
}

func documentInput(req documentRequest) DocumentInput {
	return DocumentInput{Name: req.Name, SourceType: req.SourceType, SourceRef: req.SourceRef, Content: req.Content, MetaJSON: req.MetaJSON, Status: req.Status}
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
	return nil, apperror.Internal("AI知识库服务未配置")
}
func (nilHTTPService) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	return nil, apperror.Internal("AI知识库服务未配置")
}
func (nilHTTPService) Detail(ctx context.Context, id uint64) (*DetailResponse, *apperror.Error) {
	return nil, apperror.Internal("AI知识库服务未配置")
}
func (nilHTTPService) Create(ctx context.Context, input MapInput) (uint64, *apperror.Error) {
	return 0, apperror.Internal("AI知识库服务未配置")
}
func (nilHTTPService) Update(ctx context.Context, id uint64, input MapInput) *apperror.Error {
	return apperror.Internal("AI知识库服务未配置")
}
func (nilHTTPService) ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	return apperror.Internal("AI知识库服务未配置")
}
func (nilHTTPService) Sync(ctx context.Context, id uint64) *apperror.Error {
	return apperror.Internal("AI知识库服务未配置")
}
func (nilHTTPService) Delete(ctx context.Context, id uint64) *apperror.Error {
	return apperror.Internal("AI知识库服务未配置")
}
func (nilHTTPService) Documents(ctx context.Context, mapID uint64) (*DocumentListResponse, *apperror.Error) {
	return nil, apperror.Internal("AI知识库服务未配置")
}
func (nilHTTPService) CreateDocument(ctx context.Context, mapID uint64, input DocumentInput) (uint64, *apperror.Error) {
	return 0, apperror.Internal("AI知识库服务未配置")
}
func (nilHTTPService) ChangeDocumentStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	return apperror.Internal("AI知识库服务未配置")
}
func (nilHTTPService) RefreshDocumentStatus(ctx context.Context, id uint64) *apperror.Error {
	return apperror.Internal("AI知识库服务未配置")
}
func (nilHTTPService) DeleteDocument(ctx context.Context, id uint64) *apperror.Error {
	return apperror.Internal("AI知识库服务未配置")
}
