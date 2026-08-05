package admin

import (
	"context"
	"strconv"

	"admin_back_go/internal/middleware"
	contextengine "admin_back_go/internal/module/ai/contextengine"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type HTTPService interface {
	CreateProfile(context.Context, uint32, contextengine.CreateProfileInput) (*contextengine.ProfileDTO, *apperror.Error)
	UpdateProfile(context.Context, uint64, contextengine.UpdateProfileInput) (*contextengine.ProfileDTO, *apperror.Error)
	CreateSpace(context.Context, string, uint32, contextengine.CreateSpaceInput) (*contextengine.SpaceDTO, *apperror.Error)
	UpdateSpace(context.Context, string, uint64, contextengine.UpdateSpaceInput) (*contextengine.SpaceDTO, *apperror.Error)
	DeleteSpace(context.Context, string, uint64) *apperror.Error
	CreateDocument(context.Context, string, uint32, contextengine.CreateDocumentInput) (*contextengine.DocumentAdminDTO, *apperror.Error)
	ReindexDocument(context.Context, string, uint64) (*contextengine.DocumentAdminDTO, *apperror.Error)
}

type adminReadService interface {
	PageInit(context.Context) (*contextengine.ContextPageInitResponse, *apperror.Error)
	ListProfiles(context.Context, contextengine.ProfileStatus) (*contextengine.ProfileListResponse, *apperror.Error)
	GetProfile(context.Context, uint64) (*contextengine.ProfileDTO, *apperror.Error)
	UpdateProfileMetadata(context.Context, uint64, string) (*contextengine.ProfileDTO, *apperror.Error)
	ChangeProfileStatus(context.Context, uint64, contextengine.ProfileStatus) (*contextengine.ProfileDTO, *apperror.Error)
	ListSpaces(context.Context, string, uint64, string) (*contextengine.SpaceListResponse, *apperror.Error)
	GetSpace(context.Context, string, uint64) (*contextengine.SpaceDTO, *apperror.Error)
	ChangeSpaceStatus(context.Context, string, uint64, string) (*contextengine.SpaceDTO, *apperror.Error)
	ListDocuments(context.Context, string, uint64, string) (*contextengine.DocumentListResponse, *apperror.Error)
	GetDocument(context.Context, string, uint64) (*contextengine.DocumentAdminDTO, *apperror.Error)
	ListDocumentVersions(context.Context, string, uint64) (*contextengine.DocumentVersionListResponse, *apperror.Error)
	CreateDocumentVersion(context.Context, string, uint64, contextengine.CreateDocumentVersionInput) (*contextengine.DocumentAdminDTO, *apperror.Error)
	ChangeDocumentStatus(context.Context, string, uint64, string) (*contextengine.DocumentAdminDTO, *apperror.Error)
	DeleteDocument(context.Context, string, uint64) *apperror.Error
	GetAgentContextProfile(context.Context, uint64) (*contextengine.AgentContextProfileInput, *apperror.Error)
	UpdateAgentContextProfile(context.Context, uint64, *uint64) (*contextengine.AgentContextProfileInput, *apperror.Error)
	GetAgentContextSpaces(context.Context, uint64) (*contextengine.AgentContextSpacesInput, *apperror.Error)
	UpdateAgentContextSpaces(context.Context, uint64, []uint64) (*contextengine.AgentContextSpacesInput, *apperror.Error)
}

type evaluationService interface {
	Evaluate(context.Context, contextengine.EvaluationRequest) (*contextengine.ContextEvaluationResponse, *apperror.Error)
}

type Handler struct {
	platform string
	service  HTTPService
}

func NewHandler(platform string, service HTTPService) *Handler {
	return &Handler{platform: platform, service: service}
}

func (handler *Handler) PageInit(c *gin.Context) {
	service, ok := handler.service.(adminReadService)
	if !ok {
		unsupported(c)
		return
	}
	result, appErr := service.PageInit(c.Request.Context())
	write(c, result, appErr)
}

func (handler *Handler) CreateProfile(c *gin.Context) {
	var request profileCreateRequest
	if !bind(c, &request) {
		return
	}
	actor, ok := actorID(c)
	if !ok {
		return
	}
	result, appErr := handler.service.CreateProfile(c.Request.Context(), actor, profileCreateInput(request))
	write(c, result, appErr)
}
func (handler *Handler) UpdateProfile(c *gin.Context) {
	service, serviceOK := handler.service.(adminReadService)
	if !serviceOK {
		unsupported(c)
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	var request profileUpdateRequest
	if !bind(c, &request) {
		return
	}
	result, appErr := service.UpdateProfileMetadata(c.Request.Context(), id, request.Name)
	write(c, result, appErr)
}
func (handler *Handler) ListProfiles(c *gin.Context) {
	service, ok := handler.service.(adminReadService)
	if !ok {
		unsupported(c)
		return
	}
	var request profileListRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		response.Error(c, apperror.BadRequest("上下文配置查询参数错误"))
		return
	}
	result, appErr := service.ListProfiles(c.Request.Context(), request.Status)
	write(c, result, appErr)
}
func (handler *Handler) GetProfile(c *gin.Context) {
	service, ok := handler.service.(adminReadService)
	if !ok {
		unsupported(c)
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	result, appErr := service.GetProfile(c.Request.Context(), id)
	write(c, result, appErr)
}
func (handler *Handler) ChangeProfileStatus(c *gin.Context) {
	service, ok := handler.service.(adminReadService)
	if !ok {
		unsupported(c)
		return
	}
	id, valid := routeID(c)
	if !valid {
		return
	}
	var request contextStatusRequest
	if !bind(c, &request) {
		return
	}
	result, appErr := service.ChangeProfileStatus(c.Request.Context(), id, contextengine.ProfileStatus(request.Status))
	write(c, result, appErr)
}
func (handler *Handler) CreateSpace(c *gin.Context) {
	var request spaceRequest
	if !bind(c, &request) {
		return
	}
	actor, ok := actorID(c)
	if !ok {
		return
	}
	result, appErr := handler.service.CreateSpace(c.Request.Context(), handler.platform, actor, spaceInput(request))
	write(c, result, appErr)
}
func (handler *Handler) UpdateSpace(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	var request spaceRequest
	if !bind(c, &request) {
		return
	}
	input := spaceInput(request)
	result, appErr := handler.service.UpdateSpace(c.Request.Context(), handler.platform, id, contextengine.UpdateSpaceInput(input))
	write(c, result, appErr)
}
func (handler *Handler) ListSpaces(c *gin.Context) {
	service, ok := handler.service.(adminReadService)
	if !ok {
		unsupported(c)
		return
	}
	var request spaceListRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		response.Error(c, apperror.BadRequest("上下文空间查询参数错误"))
		return
	}
	result, appErr := service.ListSpaces(c.Request.Context(), handler.platform, request.ProfileID, request.Status)
	write(c, result, appErr)
}
func (handler *Handler) GetSpace(c *gin.Context) {
	service, ok := handler.service.(adminReadService)
	if !ok {
		unsupported(c)
		return
	}
	id, valid := routeID(c)
	if !valid {
		return
	}
	result, appErr := service.GetSpace(c.Request.Context(), handler.platform, id)
	write(c, result, appErr)
}
func (handler *Handler) ChangeSpaceStatus(c *gin.Context) {
	service, ok := handler.service.(adminReadService)
	if !ok {
		unsupported(c)
		return
	}
	id, valid := routeID(c)
	if !valid {
		return
	}
	var request contextStatusRequest
	if !bind(c, &request) {
		return
	}
	result, appErr := service.ChangeSpaceStatus(c.Request.Context(), handler.platform, id, request.Status)
	write(c, result, appErr)
}
func (handler *Handler) DeleteSpace(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	write(c, gin.H{}, handler.service.DeleteSpace(c.Request.Context(), handler.platform, id))
}
func (handler *Handler) CreateDocument(c *gin.Context) {
	var request documentRequest
	if !bind(c, &request) {
		return
	}
	actor, ok := actorID(c)
	if !ok {
		return
	}
	result, appErr := handler.service.CreateDocument(c.Request.Context(), handler.platform, actor, documentInput(request))
	write(c, result, appErr)
}
func (handler *Handler) ListSpaceDocuments(c *gin.Context) {
	service, ok := handler.service.(adminReadService)
	if !ok {
		unsupported(c)
		return
	}
	spaceID, valid := routeID(c)
	if !valid {
		return
	}
	var request documentListRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		response.Error(c, apperror.BadRequest("上下文文档查询参数错误"))
		return
	}
	result, appErr := service.ListDocuments(c.Request.Context(), handler.platform, spaceID, request.Status)
	write(c, result, appErr)
}
func (handler *Handler) CreateSpaceDocument(c *gin.Context) {
	var request spaceDocumentRequest
	if !bind(c, &request) {
		return
	}
	spaceID, valid := routeID(c)
	if !valid {
		return
	}
	actor, valid := actorID(c)
	if !valid {
		return
	}
	result, appErr := handler.service.CreateDocument(c.Request.Context(), handler.platform, actor, contextengine.CreateDocumentInput{SpaceID: &spaceID, Title: request.Title, SourceStorageProvider: request.SourceStorageProvider, SourceObjectKey: request.SourceObjectKey, SourceETag: request.SourceETag, SourceSize: request.SourceSize, SourceFilename: request.SourceFilename})
	write(c, result, appErr)
}
func (handler *Handler) GetDocument(c *gin.Context) {
	service, ok := handler.service.(adminReadService)
	if !ok {
		unsupported(c)
		return
	}
	id, valid := routeID(c)
	if !valid {
		return
	}
	result, appErr := service.GetDocument(c.Request.Context(), handler.platform, id)
	write(c, result, appErr)
}
func (handler *Handler) ListDocumentVersions(c *gin.Context) {
	service, ok := handler.service.(adminReadService)
	if !ok {
		unsupported(c)
		return
	}
	id, valid := routeID(c)
	if !valid {
		return
	}
	result, appErr := service.ListDocumentVersions(c.Request.Context(), handler.platform, id)
	write(c, result, appErr)
}
func (handler *Handler) CreateDocumentVersion(c *gin.Context) {
	service, ok := handler.service.(adminReadService)
	if !ok {
		unsupported(c)
		return
	}
	id, valid := routeID(c)
	if !valid {
		return
	}
	var request documentVersionRequest
	if !bind(c, &request) {
		return
	}
	result, appErr := service.CreateDocumentVersion(c.Request.Context(), handler.platform, id, contextengine.CreateDocumentVersionInput{SourceStorageProvider: request.SourceStorageProvider, SourceObjectKey: request.SourceObjectKey, SourceETag: request.SourceETag, SourceSize: request.SourceSize, SourceFilename: request.SourceFilename})
	write(c, result, appErr)
}
func (handler *Handler) ChangeDocumentStatus(c *gin.Context) {
	service, ok := handler.service.(adminReadService)
	if !ok {
		unsupported(c)
		return
	}
	id, valid := routeID(c)
	if !valid {
		return
	}
	var request contextStatusRequest
	if !bind(c, &request) {
		return
	}
	result, appErr := service.ChangeDocumentStatus(c.Request.Context(), handler.platform, id, request.Status)
	write(c, result, appErr)
}
func (handler *Handler) DeleteDocument(c *gin.Context) {
	service, ok := handler.service.(adminReadService)
	if !ok {
		unsupported(c)
		return
	}
	id, valid := routeID(c)
	if !valid {
		return
	}
	write(c, gin.H{}, service.DeleteDocument(c.Request.Context(), handler.platform, id))
}
func (handler *Handler) ReindexDocument(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	result, appErr := handler.service.ReindexDocument(c.Request.Context(), handler.platform, id)
	write(c, result, appErr)
}
func (handler *Handler) Evaluate(c *gin.Context) {
	service, ok := handler.service.(evaluationService)
	if !ok {
		unsupported(c)
		return
	}
	var request evaluationRequest
	if !bind(c, &request) {
		return
	}
	result, appErr := service.Evaluate(c.Request.Context(), request)
	write(c, result, appErr)
}
func (handler *Handler) GetAgentContextProfile(c *gin.Context) {
	service, ok := handler.service.(adminReadService)
	if !ok {
		unsupported(c)
		return
	}
	id, valid := routeID(c)
	if !valid {
		return
	}
	result, appErr := service.GetAgentContextProfile(c.Request.Context(), id)
	write(c, result, appErr)
}
func (handler *Handler) UpdateAgentContextProfile(c *gin.Context) {
	service, ok := handler.service.(adminReadService)
	if !ok {
		unsupported(c)
		return
	}
	id, valid := routeID(c)
	if !valid {
		return
	}
	var request agentContextProfileRequest
	if !bind(c, &request) {
		return
	}
	result, appErr := service.UpdateAgentContextProfile(c.Request.Context(), id, request.ProfileID)
	write(c, result, appErr)
}
func (handler *Handler) GetAgentContextSpaces(c *gin.Context) {
	service, ok := handler.service.(adminReadService)
	if !ok {
		unsupported(c)
		return
	}
	id, valid := routeID(c)
	if !valid {
		return
	}
	result, appErr := service.GetAgentContextSpaces(c.Request.Context(), id)
	write(c, result, appErr)
}
func (handler *Handler) UpdateAgentContextSpaces(c *gin.Context) {
	service, ok := handler.service.(adminReadService)
	if !ok {
		unsupported(c)
		return
	}
	id, valid := routeID(c)
	if !valid {
		return
	}
	var request agentContextSpacesRequest
	if !bind(c, &request) {
		return
	}
	result, appErr := service.UpdateAgentContextSpaces(c.Request.Context(), id, request.SpaceIDs)
	write(c, result, appErr)
}

func bind(c *gin.Context, request any) bool {
	if err := c.ShouldBindJSON(request); err != nil {
		response.Error(c, apperror.BadRequest("上下文管理参数错误"))
		return false
	}
	return true
}
func routeID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, apperror.BadRequest("无效的上下文资源ID"))
		return 0, false
	}
	return id, true
}
func actorID(c *gin.Context) (uint32, bool) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil {
		response.Error(c, apperror.Unauthorized("缺少管理员身份"))
		return 0, false
	}
	if identity.UserID <= 0 || uint64(identity.UserID) > uint64(^uint32(0)) {
		response.Error(c, apperror.Unauthorized("无效的管理员身份"))
		return 0, false
	}
	return uint32(identity.UserID), true
}
func write(c *gin.Context, result any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func unsupported(c *gin.Context) {
	response.Error(c, apperror.Internal("上下文管理服务未配置"))
}
