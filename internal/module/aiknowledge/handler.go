package aiknowledge

import (
	"context"
	"strconv"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type Handler struct{ service HTTPService }

func NewHandler(service HTTPService) *Handler { return &Handler{service: service} }
func (h *Handler) Init(c *gin.Context) {
	res, err := h.requireService().Init(c.Request.Context())
	writeResult(c, res, err)
}
func (h *Handler) List(c *gin.Context) {
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("知识库列表参数错误"))
		return
	}
	res, appErr := h.requireService().List(c.Request.Context(), ListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, Name: req.Name, Visibility: req.Visibility, Status: req.Status})
	writeResult(c, res, appErr)
}
func (h *Handler) Detail(c *gin.Context) {
	id, ok := routeID(c, "id", "无效的知识库ID")
	if !ok {
		return
	}
	res, err := h.requireService().Detail(c.Request.Context(), id)
	writeResult(c, res, err)
}
func (h *Handler) Create(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	var req mutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("知识库参数错误"))
		return
	}
	id, appErr := h.requireService().Create(c.Request.Context(), userID(identity), kbInput(req))
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{"id": id})
}
func (h *Handler) Update(c *gin.Context) {
	id, ok := routeID(c, "id", "无效的知识库ID")
	if !ok {
		return
	}
	var req mutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("知识库参数错误"))
		return
	}
	if appErr := h.requireService().Update(c.Request.Context(), id, kbInput(req)); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}
func (h *Handler) ChangeStatus(c *gin.Context) {
	id, ok := routeID(c, "id", "无效的知识库ID")
	if !ok {
		return
	}
	var req statusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("知识库状态参数错误"))
		return
	}
	if appErr := h.requireService().ChangeStatus(c.Request.Context(), id, req.Status); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}
func (h *Handler) Delete(c *gin.Context) {
	id, ok := routeID(c, "id", "无效的知识库ID")
	if !ok {
		return
	}
	count, appErr := h.requireService().Delete(c.Request.Context(), []int64{id})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{"affected": count})
}
func (h *Handler) Documents(c *gin.Context) {
	kbID, ok := routeID(c, "id", "无效的知识库ID")
	if !ok {
		return
	}
	var req documentListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("知识库文档列表参数错误"))
		return
	}
	res, appErr := h.requireService().Documents(c.Request.Context(), DocumentListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, KnowledgeBaseID: kbID, Title: req.Title, Status: req.Status})
	writeResult(c, res, appErr)
}
func (h *Handler) DocumentDetail(c *gin.Context) {
	kbID, ok := routeID(c, "id", "无效的知识库ID")
	if !ok {
		return
	}
	id, ok := routeID(c, "document_id", "无效的文档ID")
	if !ok {
		return
	}
	res, appErr := h.requireService().DocumentDetail(c.Request.Context(), id, kbID)
	writeResult(c, res, appErr)
}
func (h *Handler) CreateDocument(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	kbID, ok := routeID(c, "id", "无效的知识库ID")
	if !ok {
		return
	}
	var req documentMutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("知识库文档参数错误"))
		return
	}
	input := documentInput(kbID, req)
	id, appErr := h.requireService().CreateDocument(c.Request.Context(), userID(identity), input)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{"id": id})
}
func (h *Handler) UpdateDocument(c *gin.Context) {
	kbID, ok := routeID(c, "id", "无效的知识库ID")
	if !ok {
		return
	}
	id, ok := routeID(c, "document_id", "无效的文档ID")
	if !ok {
		return
	}
	var req documentMutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("知识库文档参数错误"))
		return
	}
	if appErr := h.requireService().UpdateDocument(c.Request.Context(), id, documentInput(kbID, req)); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}
func (h *Handler) DeleteDocument(c *gin.Context) {
	kbID, ok := routeID(c, "id", "无效的知识库ID")
	if !ok {
		return
	}
	id, ok := routeID(c, "document_id", "无效的文档ID")
	if !ok {
		return
	}
	count, appErr := h.requireService().DeleteDocument(c.Request.Context(), id, kbID)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{"affected": count})
}
func (h *Handler) ReindexDocument(c *gin.Context) {
	kbID, ok := routeID(c, "id", "无效的知识库ID")
	if !ok {
		return
	}
	id, ok := routeID(c, "document_id", "无效的文档ID")
	if !ok {
		return
	}
	count, appErr := h.requireService().ReindexDocument(c.Request.Context(), id, kbID)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{"chunk_count": count})
}
func (h *Handler) Chunks(c *gin.Context) {
	kbID, ok := routeID(c, "id", "无效的知识库ID")
	if !ok {
		return
	}
	var req chunkListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("知识库切片参数错误"))
		return
	}
	res, appErr := h.requireService().Chunks(c.Request.Context(), ChunkListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, KnowledgeBaseID: kbID, DocumentID: req.DocumentID})
	writeResult(c, res, appErr)
}
func (h *Handler) RetrievalTest(c *gin.Context) {
	kbID, ok := routeID(c, "id", "无效的知识库ID")
	if !ok {
		return
	}
	var req retrievalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("知识库召回参数错误"))
		return
	}
	res, appErr := h.requireService().RetrievalTest(c.Request.Context(), RetrievalInput{KnowledgeBaseID: kbID, Query: req.Query, TopK: req.TopK})
	writeResult(c, res, appErr)
}
func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return nilHTTPService{}
	}
	return h.service
}
func routeID(c *gin.Context, name, msg string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequest(msg))
		return 0, false
	}
	return id, true
}
func userID(identity *middleware.AuthIdentity) int64 {
	if identity == nil {
		return 0
	}
	return identity.UserID
}
func kbInput(req mutationRequest) KnowledgeBaseMutationInput {
	return KnowledgeBaseMutationInput{Name: req.Name, Description: req.Description, Visibility: req.Visibility, PermissionJSON: req.PermissionJSON, ChunkSize: req.ChunkSize, ChunkOverlap: req.ChunkOverlap, TopK: req.TopK, ScoreThreshold: req.ScoreThreshold, Status: req.Status}
}
func documentInput(kbID int64, req documentMutationRequest) DocumentMutationInput {
	return DocumentMutationInput{KnowledgeBaseID: kbID, Title: req.Title, SourceType: req.SourceType, Content: req.Content, Status: req.Status}
}
func writeResult(c *gin.Context, res any, err *apperror.Error) {
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, res)
}

type nilHTTPService struct{}

func (nilHTTPService) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return nil, apperror.Internal("知识库服务未配置")
}
func (nilHTTPService) List(ctx context.Context, q ListQuery) (*ListResponse, *apperror.Error) {
	return nil, apperror.Internal("知识库服务未配置")
}
func (nilHTTPService) Detail(ctx context.Context, id int64) (*KnowledgeBaseItem, *apperror.Error) {
	return nil, apperror.Internal("知识库服务未配置")
}
func (nilHTTPService) Create(ctx context.Context, u int64, i KnowledgeBaseMutationInput) (int64, *apperror.Error) {
	return 0, apperror.Internal("知识库服务未配置")
}
func (nilHTTPService) Update(ctx context.Context, id int64, i KnowledgeBaseMutationInput) *apperror.Error {
	return apperror.Internal("知识库服务未配置")
}
func (nilHTTPService) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	return apperror.Internal("知识库服务未配置")
}
func (nilHTTPService) Delete(ctx context.Context, ids []int64) (int64, *apperror.Error) {
	return 0, apperror.Internal("知识库服务未配置")
}
func (nilHTTPService) Documents(ctx context.Context, q DocumentListQuery) (*DocumentListResponse, *apperror.Error) {
	return nil, apperror.Internal("知识库服务未配置")
}
func (nilHTTPService) DocumentDetail(ctx context.Context, id, kbID int64) (*DocumentItem, *apperror.Error) {
	return nil, apperror.Internal("知识库服务未配置")
}
func (nilHTTPService) CreateDocument(ctx context.Context, u int64, i DocumentMutationInput) (int64, *apperror.Error) {
	return 0, apperror.Internal("知识库服务未配置")
}
func (nilHTTPService) UpdateDocument(ctx context.Context, id int64, i DocumentMutationInput) *apperror.Error {
	return apperror.Internal("知识库服务未配置")
}
func (nilHTTPService) DeleteDocument(ctx context.Context, id, kbID int64) (int64, *apperror.Error) {
	return 0, apperror.Internal("知识库服务未配置")
}
func (nilHTTPService) ReindexDocument(ctx context.Context, id, kbID int64) (int, *apperror.Error) {
	return 0, apperror.Internal("知识库服务未配置")
}
func (nilHTTPService) Chunks(ctx context.Context, q ChunkListQuery) (*ChunkListResponse, *apperror.Error) {
	return nil, apperror.Internal("知识库服务未配置")
}
func (nilHTTPService) RetrievalTest(ctx context.Context, i RetrievalInput) (*RetrievalResponse, *apperror.Error) {
	return nil, apperror.Internal("知识库服务未配置")
}
