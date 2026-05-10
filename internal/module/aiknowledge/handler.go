package aiknowledge

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

func (h *Handler) ListBases(c *gin.Context) {
	var req baseListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI知识库列表参数错误"))
		return
	}
	result, appErr := h.requireService().ListBases(c.Request.Context(), BaseListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, Name: req.Name, Code: req.Code, Status: req.Status})
	writeResult(c, result, appErr)
}

func (h *Handler) GetBase(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	result, appErr := h.requireService().GetBase(c.Request.Context(), id)
	writeResult(c, result, appErr)
}

func (h *Handler) CreateBase(c *gin.Context) {
	input, ok := bindBaseMutation(c)
	if !ok {
		return
	}
	id, appErr := h.requireService().CreateBase(c.Request.Context(), input)
	writeResult(c, gin.H{"id": id}, appErr)
}

func (h *Handler) UpdateBase(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	input, ok := bindBaseMutation(c)
	if !ok {
		return
	}
	appErr := h.requireService().UpdateBase(c.Request.Context(), id, input)
	writeResult(c, gin.H{}, appErr)
}

func (h *Handler) ChangeBaseStatus(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req statusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI知识库状态参数错误"))
		return
	}
	appErr := h.requireService().ChangeBaseStatus(c.Request.Context(), id, req.Status)
	writeResult(c, gin.H{}, appErr)
}

func (h *Handler) DeleteBase(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	appErr := h.requireService().DeleteBase(c.Request.Context(), id)
	writeResult(c, gin.H{}, appErr)
}

func (h *Handler) ListDocuments(c *gin.Context) {
	baseID, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req documentListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI知识库文档列表参数错误"))
		return
	}
	result, appErr := h.requireService().ListDocuments(c.Request.Context(), baseID, DocumentListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, Title: req.Title, Status: req.Status})
	writeResult(c, result, appErr)
}

func (h *Handler) CreateDocument(c *gin.Context) {
	baseID, ok := parseID(c, "id")
	if !ok {
		return
	}
	input, ok := bindDocumentMutation(c)
	if !ok {
		return
	}
	id, appErr := h.requireService().CreateDocument(c.Request.Context(), baseID, input)
	writeResult(c, gin.H{"id": id}, appErr)
}

func (h *Handler) GetDocument(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	result, appErr := h.requireService().GetDocument(c.Request.Context(), id)
	writeResult(c, result, appErr)
}

func (h *Handler) UpdateDocument(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	input, ok := bindDocumentMutation(c)
	if !ok {
		return
	}
	appErr := h.requireService().UpdateDocument(c.Request.Context(), id, input)
	writeResult(c, gin.H{}, appErr)
}

func (h *Handler) ChangeDocumentStatus(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req statusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI知识库文档状态参数错误"))
		return
	}
	appErr := h.requireService().ChangeDocumentStatus(c.Request.Context(), id, req.Status)
	writeResult(c, gin.H{}, appErr)
}

func (h *Handler) DeleteDocument(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	appErr := h.requireService().DeleteDocument(c.Request.Context(), id)
	writeResult(c, gin.H{}, appErr)
}

func (h *Handler) ReindexDocument(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	appErr := h.requireService().ReindexDocument(c.Request.Context(), id)
	writeResult(c, gin.H{}, appErr)
}

func (h *Handler) ListChunks(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	result, appErr := h.requireService().ListChunks(c.Request.Context(), id)
	writeResult(c, result, appErr)
}

func (h *Handler) RetrievalTest(c *gin.Context) {
	baseID, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req retrievalTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI知识库检索测试参数错误"))
		return
	}
	result, appErr := h.requireService().RetrievalTest(c.Request.Context(), baseID, RetrievalTestInput{Query: req.Query, TopK: req.TopK, MinScore: req.MinScore, MaxContextChars: req.MaxContextChars})
	writeResult(c, result, appErr)
}

func (h *Handler) AgentKnowledgeBases(c *gin.Context) {
	agentID, ok := parseID(c, "id")
	if !ok {
		return
	}
	result, appErr := h.requireService().AgentKnowledgeBases(c.Request.Context(), agentID)
	writeResult(c, result, appErr)
}

func (h *Handler) UpdateAgentKnowledgeBases(c *gin.Context) {
	agentID, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req updateAgentKnowledgeBindingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("智能体知识库绑定参数错误"))
		return
	}
	appErr := h.requireService().UpdateAgentKnowledgeBases(c.Request.Context(), agentID, UpdateAgentKnowledgeBindingsInput{Bindings: req.Bindings})
	writeResult(c, gin.H{}, appErr)
}

func bindBaseMutation(c *gin.Context) (BaseMutationInput, bool) {
	var req baseMutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI知识库参数错误"))
		return BaseMutationInput{}, false
	}
	return BaseMutationInput{Name: req.Name, Code: req.Code, Description: req.Description, ChunkSizeChars: req.ChunkSizeChars, ChunkOverlapChars: req.ChunkOverlapChars, DefaultTopK: req.DefaultTopK, DefaultMinScore: req.DefaultMinScore, DefaultMaxContextChars: req.DefaultMaxContextChars, Status: req.Status}, true
}

func bindDocumentMutation(c *gin.Context) (DocumentMutationInput, bool) {
	var req documentMutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI知识库文档参数错误"))
		return DocumentMutationInput{}, false
	}
	return DocumentMutationInput{Title: req.Title, SourceType: req.SourceType, SourceRef: req.SourceRef, Content: req.Content, Status: req.Status}, true
}

func parseID(c *gin.Context, name string) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, apperror.BadRequest("ID参数错误"))
		return 0, false
	}
	return id, true
}

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return missingService{}
	}
	return h.service
}

type missingService struct{}

func (missingService) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return nil, apperror.Internal("AI知识库服务未配置")
}
func (missingService) ListBases(ctx context.Context, query BaseListQuery) (*BaseListResponse, *apperror.Error) {
	return nil, apperror.Internal("AI知识库服务未配置")
}
func (missingService) GetBase(ctx context.Context, id uint64) (*BaseDetailResponse, *apperror.Error) {
	return nil, apperror.Internal("AI知识库服务未配置")
}
func (missingService) CreateBase(ctx context.Context, input BaseMutationInput) (uint64, *apperror.Error) {
	return 0, apperror.Internal("AI知识库服务未配置")
}
func (missingService) UpdateBase(ctx context.Context, id uint64, input BaseMutationInput) *apperror.Error {
	return apperror.Internal("AI知识库服务未配置")
}
func (missingService) ChangeBaseStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	return apperror.Internal("AI知识库服务未配置")
}
func (missingService) DeleteBase(ctx context.Context, id uint64) *apperror.Error {
	return apperror.Internal("AI知识库服务未配置")
}
func (missingService) ListDocuments(ctx context.Context, baseID uint64, query DocumentListQuery) (*DocumentListResponse, *apperror.Error) {
	return nil, apperror.Internal("AI知识库服务未配置")
}
func (missingService) GetDocument(ctx context.Context, id uint64) (*DocumentDetailResponse, *apperror.Error) {
	return nil, apperror.Internal("AI知识库服务未配置")
}
func (missingService) CreateDocument(ctx context.Context, baseID uint64, input DocumentMutationInput) (uint64, *apperror.Error) {
	return 0, apperror.Internal("AI知识库服务未配置")
}
func (missingService) UpdateDocument(ctx context.Context, id uint64, input DocumentMutationInput) *apperror.Error {
	return apperror.Internal("AI知识库服务未配置")
}
func (missingService) ChangeDocumentStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	return apperror.Internal("AI知识库服务未配置")
}
func (missingService) DeleteDocument(ctx context.Context, id uint64) *apperror.Error {
	return apperror.Internal("AI知识库服务未配置")
}
func (missingService) ReindexDocument(ctx context.Context, id uint64) *apperror.Error {
	return apperror.Internal("AI知识库服务未配置")
}
func (missingService) ListChunks(ctx context.Context, documentID uint64) (*ChunkListResponse, *apperror.Error) {
	return nil, apperror.Internal("AI知识库服务未配置")
}
func (missingService) RetrievalTest(ctx context.Context, baseID uint64, input RetrievalTestInput) (*RetrievalResult, *apperror.Error) {
	return nil, apperror.Internal("AI知识库服务未配置")
}
func (missingService) AgentKnowledgeBases(ctx context.Context, agentID uint64) (*AgentKnowledgeBindingsResponse, *apperror.Error) {
	return nil, apperror.Internal("AI知识库服务未配置")
}
func (missingService) UpdateAgentKnowledgeBases(ctx context.Context, agentID uint64, input UpdateAgentKnowledgeBindingsInput) *apperror.Error {
	return apperror.Internal("AI知识库服务未配置")
}

func writeResult(c *gin.Context, data any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, data)
}
