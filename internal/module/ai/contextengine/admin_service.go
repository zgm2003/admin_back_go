package contextengine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/documentparser"
	"admin_back_go/internal/infra/storage"
	"admin_back_go/internal/module/ai/officialmodel"
	aiprovider "admin_back_go/internal/module/ai/provider"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

type AdminRepository interface {
	FindProviderModelCapability(context.Context, uint64) (*ProviderModelCapability, error)
	CreateProfile(context.Context, ContextProfile) (ContextProfile, error)
	FindProfile(context.Context, uint64) (*ContextProfile, error)
	UpdateProfileMetadata(context.Context, uint64, string, ProfileStatus) (ContextProfile, error)
	CompareAndSwapProfileIndex(context.Context, ProfileIndexCAS) (bool, error)
	CreateSpace(context.Context, ContextSpace) (ContextSpace, error)
	FindSpace(context.Context, string, uint64) (*ContextSpace, error)
	SpaceHasReferences(context.Context, uint64) (bool, error)
	UpdateSpace(context.Context, ContextSpace) (ContextSpace, error)
	SoftDeleteSpace(context.Context, string, uint64) error
	CreateDocumentWithVersion(context.Context, ContextDocument, ContextDocumentVersion) (DocumentAdminDTO, error)
	FindDocument(context.Context, string, uint64) (*DocumentAdminDTO, error)
	CreateDocumentVersion(context.Context, ContextDocumentVersion) (DocumentAdminDTO, error)
	AgentProfileChangeConflict(context.Context, uint64) (bool, error)
	ListProfiles(context.Context, ProfileStatus) ([]ContextProfile, error)
	ListSpaces(context.Context, string, uint64, string) ([]ContextSpace, error)
	ListDocuments(context.Context, string, uint64, string) ([]DocumentAdminDTO, error)
	ListDocumentVersions(context.Context, string, uint64) ([]DocumentVersionDTO, error)
	UpdateDocumentStatus(context.Context, string, uint64, string) (DocumentAdminDTO, error)
	SoftDeleteDocument(context.Context, string, uint64) error
	GetAgentContextProfile(context.Context, uint64) (*uint64, error)
	SetAgentContextProfile(context.Context, uint64, *uint64) error
	ListAgentContextSpaces(context.Context, uint64) ([]uint64, error)
	ReplaceAgentContextSpaces(context.Context, uint64, []uint64) error
}

type EvaluationRunner interface {
	RunEvaluation(context.Context, EvaluationRequest, EvaluationOptions) (ContextEvaluationResponse, error)
}

type DocumentVersionEnqueuer interface {
	EnqueueDocumentVersion(context.Context, uint64) error
}

type AgentBackfillEnqueuer interface {
	EnqueueAgentBackfill(context.Context, uint64, uint64) error
}

type AdminService struct {
	repository AdminRepository
	objects    storage.ConditionalObjectReader
	enqueuer   DocumentVersionEnqueuer
	models     officialmodel.Resolver
	backfill   AgentBackfillEnqueuer
	evaluator  EvaluationRunner
}

type AdminOption func(*AdminService)

func WithOfficialModelResolver(resolver officialmodel.Resolver) AdminOption {
	return func(service *AdminService) { service.models = resolver }
}

func WithAgentBackfillEnqueuer(enqueuer AgentBackfillEnqueuer) AdminOption {
	return func(service *AdminService) { service.backfill = enqueuer }
}

func WithEvaluationRunner(runner EvaluationRunner) AdminOption {
	return func(service *AdminService) { service.evaluator = runner }
}

func NewAdminService(repository AdminRepository, objects storage.ConditionalObjectReader, enqueuer DocumentVersionEnqueuer, options ...AdminOption) *AdminService {
	service := &AdminService{repository: repository, objects: objects, enqueuer: enqueuer}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (service *AdminService) CreateProfile(ctx context.Context, actorID uint32, input CreateProfileInput) (*ProfileDTO, *apperror.Error) {
	if service == nil || service.repository == nil {
		return nil, apperror.Internal("上下文仓储未配置")
	}
	input.Name = strings.TrimSpace(input.Name)
	if actorID == 0 || input.Name == "" || input.EmbeddingDimensions == 0 || input.EmbeddingMaxInputTokens <= 0 {
		return nil, apperror.BadRequest("上下文配置参数错误")
	}
	if _, err := infraai.ResolveTokenCounter(input.EmbeddingTokenCounterID); err != nil {
		return nil, apperror.BadRequest("Embedding Token Counter 无效")
	}
	distance := DenseDistance(input.DenseDistance)
	if distance.Validate() != nil {
		return nil, apperror.BadRequest("向量距离无效")
	}
	dense, err := ParseFixedScore(input.DenseMinScore)
	if err != nil {
		return nil, apperror.BadRequest("Dense 阈值无效")
	}
	if (input.RerankerProviderModelID == nil) != (input.RerankerMinScore == nil) {
		return nil, apperror.BadRequest("Reranker 模型与阈值必须成对配置")
	}
	if appErr := service.requireModel(ctx, input.EmbeddingProviderModelID, aiprovider.ModelKindEmbedding); appErr != nil {
		return nil, appErr
	}
	if input.RerankerProviderModelID != nil {
		if _, err := ParseFixedScore(*input.RerankerMinScore); err != nil {
			return nil, apperror.BadRequest("Reranker 阈值无效")
		}
		if appErr := service.requireModel(ctx, *input.RerankerProviderModelID, aiprovider.ModelKindRerank); appErr != nil {
			return nil, appErr
		}
	}
	if input.MemoryProviderModelID != nil {
		model, appErr := service.requireModelCapability(ctx, *input.MemoryProviderModelID, aiprovider.ModelKindChat)
		if appErr != nil {
			return nil, appErr
		}
		if service.models == nil || strings.TrimSpace(model.OfficialModelID) == "" {
			return nil, apperror.BadRequest("Memory 模型未映射到可信能力")
		}
		resolved, err := service.models.Resolve(ctx, model.OfficialModelID)
		if err != nil || resolved.Model.ContextWindowTokens <= 0 || resolved.Model.MaxOutputTokens <= 0 ||
			resolved.Model.MaxOutputTokens > resolved.Model.ContextWindowTokens || strings.TrimSpace(resolved.Model.TokenCounterID) == "" {
			return nil, apperror.BadRequest("Memory 模型能力不完整")
		}
	}
	target := uint64(1)
	profile := ContextProfile{Name: input.Name, EmbeddingProviderModelID: input.EmbeddingProviderModelID,
		EmbeddingDimensions: input.EmbeddingDimensions, EmbeddingMaxInputTokens: input.EmbeddingMaxInputTokens,
		EmbeddingTokenCounterID: strings.TrimSpace(input.EmbeddingTokenCounterID), DenseDistance: string(distance), DenseMinScore: dense.String(),
		SparseEncoder: SparseEncoderUnicodeLexicalV1, SparseEncoderVersion: SparseEncoderVersionV1,
		RerankerProviderModelID: cloneUint64(input.RerankerProviderModelID), RerankerMinScore: cloneString(input.RerankerMinScore),
		MemoryProviderModelID: cloneUint64(input.MemoryProviderModelID), Status: ProfileEnabled,
		TargetIndexGeneration: &target, IndexState: ProfileIndexProvisioning, CreatedBy: actorID}
	created, createErr := service.repository.CreateProfile(ctx, profile)
	if createErr != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "新增上下文配置失败", createErr)
	}
	return &created, nil
}

func (service *AdminService) ListProfiles(ctx context.Context, status ProfileStatus) (*ProfileListResponse, *apperror.Error) {
	if status != "" && ValidateContextAdminState("profile", string(status)) != nil {
		return nil, apperror.BadRequest("上下文配置状态无效")
	}
	items, err := service.repository.ListProfiles(ctx, status)
	if err != nil {
		return nil, internalAdminError("查询上下文配置失败", err)
	}
	return &ProfileListResponse{Items: items}, nil
}

func (service *AdminService) GetProfile(ctx context.Context, id uint64) (*ProfileDTO, *apperror.Error) {
	if id == 0 {
		return nil, apperror.BadRequest("无效的上下文配置ID")
	}
	profile, err := service.repository.FindProfile(ctx, id)
	if err != nil {
		return nil, internalAdminError("查询上下文配置失败", err)
	}
	if profile == nil {
		return nil, apperror.NotFound("上下文配置不存在")
	}
	return profile, nil
}

func (service *AdminService) ChangeProfileStatus(ctx context.Context, id uint64, status ProfileStatus) (*ProfileDTO, *apperror.Error) {
	profile, appErr := service.GetProfile(ctx, id)
	if appErr != nil {
		return nil, appErr
	}
	return service.UpdateProfile(ctx, id, UpdateProfileInput{Name: profile.Name, Status: status})
}

func (service *AdminService) UpdateProfile(ctx context.Context, id uint64, input UpdateProfileInput) (*ProfileDTO, *apperror.Error) {
	if id == 0 || strings.TrimSpace(input.Name) == "" || (input.Status != ProfileEnabled && input.Status != ProfileRetired) {
		return nil, apperror.BadRequest("上下文配置参数错误")
	}
	profile, err := service.repository.FindProfile(ctx, id)
	if err != nil {
		return nil, internalAdminError("查询上下文配置失败", err)
	}
	if profile == nil {
		return nil, apperror.NotFound("上下文配置不存在")
	}
	updated, err := service.repository.UpdateProfileMetadata(ctx, id, strings.TrimSpace(input.Name), input.Status)
	if err != nil {
		return nil, internalAdminError("编辑上下文配置失败", err)
	}
	return &updated, nil
}

func (service *AdminService) CompareAndSwapProfileIndex(ctx context.Context, input ProfileIndexCAS) (bool, error) {
	if input.ID == 0 || input.Expected.Validate() != nil || input.Expected.ValidateTransition(input.Next) != nil {
		return false, ErrInvalidProfileIndex
	}
	return service.repository.CompareAndSwapProfileIndex(ctx, input)
}

func (service *AdminService) CreateSpace(ctx context.Context, platform string, actorID uint32, input CreateSpaceInput) (*SpaceDTO, *apperror.Error) {
	if !validSpaceMutation(platform, actorID, input) {
		return nil, apperror.BadRequest("上下文空间参数错误")
	}
	if appErr := service.requireAssignable(ctx, input.ProfileID); appErr != nil {
		return nil, appErr
	}
	created, err := service.repository.CreateSpace(ctx, ContextSpace{Platform: platform, ProfileID: input.ProfileID, Name: strings.TrimSpace(input.Name), Description: strings.TrimSpace(input.Description), Status: input.Status, CreatedBy: actorID})
	if err != nil {
		return nil, internalAdminError("新增上下文空间失败", err)
	}
	return &created, nil
}

func (service *AdminService) ListSpaces(ctx context.Context, platform string, profileID uint64, status string) (*SpaceListResponse, *apperror.Error) {
	if !enum.IsRegisteredPlatform(platform) || (status != "" && ValidateContextAdminState("space", status) != nil) {
		return nil, apperror.BadRequest("上下文空间参数错误")
	}
	items, err := service.repository.ListSpaces(ctx, platform, profileID, status)
	if err != nil {
		return nil, internalAdminError("查询上下文空间失败", err)
	}
	return &SpaceListResponse{Items: items}, nil
}

func (service *AdminService) GetSpace(ctx context.Context, platform string, id uint64) (*SpaceDTO, *apperror.Error) {
	if id == 0 || !enum.IsRegisteredPlatform(platform) {
		return nil, apperror.BadRequest("上下文空间参数错误")
	}
	space, err := service.repository.FindSpace(ctx, platform, id)
	if err != nil {
		return nil, internalAdminError("查询上下文空间失败", err)
	}
	if space == nil {
		return nil, apperror.NotFound("上下文空间不存在")
	}
	return space, nil
}

func (service *AdminService) ChangeSpaceStatus(ctx context.Context, platform string, id uint64, status string) (*SpaceDTO, *apperror.Error) {
	space, appErr := service.GetSpace(ctx, platform, id)
	if appErr != nil {
		return nil, appErr
	}
	return service.UpdateSpace(ctx, platform, id, UpdateSpaceInput{ProfileID: space.ProfileID, Name: space.Name, Description: space.Description, Status: status})
}

func (service *AdminService) UpdateSpace(ctx context.Context, platform string, id uint64, input UpdateSpaceInput) (*SpaceDTO, *apperror.Error) {
	if id == 0 || !validSpaceMutation(platform, 1, input) {
		return nil, apperror.BadRequest("上下文空间参数错误")
	}
	space, err := service.repository.FindSpace(ctx, platform, id)
	if err != nil {
		return nil, internalAdminError("查询上下文空间失败", err)
	}
	if space == nil {
		return nil, apperror.NotFound("上下文空间不存在")
	}
	if space.ProfileID != input.ProfileID {
		referenced, err := service.repository.SpaceHasReferences(ctx, id)
		if err != nil {
			return nil, internalAdminError("检查上下文空间引用失败", err)
		}
		if referenced {
			return nil, conflict("上下文空间已有引用，不能切换配置")
		}
		if appErr := service.requireAssignable(ctx, input.ProfileID); appErr != nil {
			return nil, appErr
		}
	}
	space.ProfileID, space.Name, space.Description, space.Status = input.ProfileID, strings.TrimSpace(input.Name), strings.TrimSpace(input.Description), input.Status
	updated, err := service.repository.UpdateSpace(ctx, *space)
	if err != nil {
		return nil, internalAdminError("编辑上下文空间失败", err)
	}
	return &updated, nil
}

func (service *AdminService) DeleteSpace(ctx context.Context, platform string, id uint64) *apperror.Error {
	if id == 0 || !enum.IsRegisteredPlatform(platform) {
		return apperror.BadRequest("上下文空间参数错误")
	}
	if err := service.repository.SoftDeleteSpace(ctx, platform, id); err != nil {
		return internalAdminError("删除上下文空间失败", err)
	}
	return nil
}

func (service *AdminService) CreateDocument(ctx context.Context, platform string, actorID uint32, input CreateDocumentInput) (*DocumentAdminDTO, *apperror.Error) {
	if actorID == 0 || !enum.IsRegisteredPlatform(platform) || !validDocumentOwner(input) {
		return nil, apperror.BadRequest("上下文文档参数错误")
	}
	if input.SpaceID == nil {
		return nil, apperror.BadRequest("管理员只能创建空间文档")
	}
	space, err := service.repository.FindSpace(ctx, platform, *input.SpaceID)
	if err != nil {
		return nil, internalAdminError("查询上下文空间失败", err)
	}
	if space == nil || space.Status != SpaceEnabled {
		return nil, apperror.BadRequest("上下文空间不可用")
	}
	if appErr := service.requireAssignable(ctx, space.ProfileID); appErr != nil {
		return nil, appErr
	}
	if service.objects == nil {
		return nil, apperror.Internal("条件对象读取器未配置")
	}
	input.SourceObjectKey = strings.TrimSpace(input.SourceObjectKey)
	if !strings.HasPrefix(input.SourceObjectKey, "ai_context_documents/") || input.SourceObjectKey == "ai_context_documents/" {
		return nil, apperror.BadRequest("文档对象路径无效")
	}
	objectInput := storage.ConditionalObjectInput{StorageProvider: strings.TrimSpace(input.SourceStorageProvider), ObjectKey: input.SourceObjectKey, ETag: strings.TrimSpace(input.SourceETag), Size: input.SourceSize}
	metadata, err := service.objects.Head(ctx, objectInput)
	if err != nil {
		return nil, conditionalObjectAppError(err)
	}
	parser, err := documentparser.NewRegistry().Resolve(input.SourceFilename, metadata.MIMEType)
	if err != nil {
		return nil, apperror.BadRequest("文档格式不支持")
	}
	document := ContextDocument{SpaceID: cloneUint64(input.SpaceID), Title: strings.TrimSpace(input.Title), Status: DocumentEnabled, CreatedBy: actorID}
	version := newQueuedVersion(space.ProfileID, objectInput, metadata, strings.TrimSpace(input.SourceFilename), parser.Name(), parser.Version())
	created, err := service.repository.CreateDocumentWithVersion(ctx, document, version)
	if err != nil {
		return nil, internalAdminError("新增上下文文档失败", err)
	}
	if service.enqueuer != nil {
		_ = service.enqueuer.EnqueueDocumentVersion(ctx, created.Version.ID)
	}
	return &created, nil
}

func (service *AdminService) ListDocuments(ctx context.Context, platform string, spaceID uint64, status string) (*DocumentListResponse, *apperror.Error) {
	if spaceID == 0 || !enum.IsRegisteredPlatform(platform) || (status != "" && ValidateContextAdminState("document", status) != nil) {
		return nil, apperror.BadRequest("上下文文档参数错误")
	}
	items, err := service.repository.ListDocuments(ctx, platform, spaceID, status)
	if err != nil {
		return nil, internalAdminError("查询上下文文档失败", err)
	}
	return &DocumentListResponse{Items: items}, nil
}

func (service *AdminService) GetDocument(ctx context.Context, platform string, id uint64) (*DocumentAdminDTO, *apperror.Error) {
	if id == 0 || !enum.IsRegisteredPlatform(platform) {
		return nil, apperror.BadRequest("上下文文档参数错误")
	}
	item, err := service.repository.FindDocument(ctx, platform, id)
	if err != nil {
		return nil, internalAdminError("查询上下文文档失败", err)
	}
	if item == nil {
		return nil, apperror.NotFound("上下文文档不存在")
	}
	return item, nil
}

func (service *AdminService) ListDocumentVersions(ctx context.Context, platform string, id uint64) (*DocumentVersionListResponse, *apperror.Error) {
	if id == 0 || !enum.IsRegisteredPlatform(platform) {
		return nil, apperror.BadRequest("上下文文档参数错误")
	}
	items, err := service.repository.ListDocumentVersions(ctx, platform, id)
	if err != nil {
		return nil, internalAdminError("查询上下文文档版本失败", err)
	}
	return &DocumentVersionListResponse{Items: items}, nil
}

func (service *AdminService) CreateDocumentVersion(ctx context.Context, platform string, id uint64, input CreateDocumentVersionInput) (*DocumentAdminDTO, *apperror.Error) {
	document, appErr := service.GetDocument(ctx, platform, id)
	if appErr != nil {
		return nil, appErr
	}
	if service.objects == nil || strings.TrimSpace(input.SourceObjectKey) == "" || input.SourceSize <= 0 || strings.TrimSpace(input.SourceFilename) == "" {
		return nil, apperror.BadRequest("上下文文档版本参数错误")
	}
	objectInput := storage.ConditionalObjectInput{StorageProvider: strings.TrimSpace(input.SourceStorageProvider), ObjectKey: strings.TrimSpace(input.SourceObjectKey), ETag: strings.TrimSpace(input.SourceETag), Size: input.SourceSize}
	metadata, err := service.objects.Head(ctx, objectInput)
	if err != nil {
		return nil, conditionalObjectAppError(err)
	}
	parser, err := documentparser.NewRegistry().Resolve(input.SourceFilename, metadata.MIMEType)
	if err != nil {
		return nil, apperror.BadRequest("文档格式不支持")
	}
	version := newQueuedVersion(document.ProfileID, objectInput, metadata, strings.TrimSpace(input.SourceFilename), parser.Name(), parser.Version())
	version.DocumentID = id
	created, err := service.repository.CreateDocumentVersion(ctx, version)
	if err != nil {
		return nil, internalAdminError("创建上下文文档版本失败", err)
	}
	if service.enqueuer != nil {
		_ = service.enqueuer.EnqueueDocumentVersion(ctx, created.Version.ID)
	}
	return &created, nil
}

func (service *AdminService) ChangeDocumentStatus(ctx context.Context, platform string, id uint64, status string) (*DocumentAdminDTO, *apperror.Error) {
	if id == 0 || !enum.IsRegisteredPlatform(platform) || ValidateContextAdminState("document", status) != nil {
		return nil, apperror.BadRequest("上下文文档状态无效")
	}
	item, err := service.repository.UpdateDocumentStatus(ctx, platform, id, status)
	if err != nil {
		return nil, internalAdminError("修改上下文文档状态失败", err)
	}
	return &item, nil
}

func (service *AdminService) DeleteDocument(ctx context.Context, platform string, id uint64) *apperror.Error {
	if id == 0 || !enum.IsRegisteredPlatform(platform) {
		return apperror.BadRequest("上下文文档参数错误")
	}
	if err := service.repository.SoftDeleteDocument(ctx, platform, id); err != nil {
		return internalAdminError("删除上下文文档失败", err)
	}
	return nil
}

func (service *AdminService) ReindexDocument(ctx context.Context, platform string, id uint64) (*DocumentAdminDTO, *apperror.Error) {
	if id == 0 || !enum.IsRegisteredPlatform(platform) {
		return nil, apperror.BadRequest("上下文文档参数错误")
	}
	document, err := service.repository.FindDocument(ctx, platform, id)
	if err != nil {
		return nil, internalAdminError("查询上下文文档失败", err)
	}
	if document == nil {
		return nil, apperror.NotFound("上下文文档不存在")
	}
	parser, resolveErr := documentparser.NewRegistry().Resolve(document.Version.SourceFilename, document.Version.SourceMIMEType)
	if resolveErr != nil {
		return nil, apperror.BadRequest("文档格式不支持")
	}
	version := ContextDocumentVersion{DocumentID: document.ID, ProfileID: document.ProfileID, SourceStorageProvider: document.Version.SourceStorageProvider,
		SourceObjectKey: document.Version.SourceObjectKey, SourceETag: document.Version.SourceETag, SourceSize: document.Version.SourceSize,
		SourceMIMEType: document.Version.SourceMIMEType, SourceFilename: document.Version.SourceFilename, ParserName: parser.Name(),
		ParserVersion: parser.Version(), ChunkerVersion: ChunkerVersionV1, State: DocumentVersionQueued}
	version.SourceFactsSHA256 = sourceFactsHash(version)
	created, err := service.repository.CreateDocumentVersion(ctx, version)
	if err != nil {
		return nil, internalAdminError("重建上下文文档失败", err)
	}
	if service.enqueuer != nil {
		_ = service.enqueuer.EnqueueDocumentVersion(ctx, created.Version.ID)
	}
	return &created, nil
}

func (service *AdminService) RequireAssignable(ctx context.Context, profileID uint64) error {
	if appErr := service.requireAssignable(ctx, profileID); appErr != nil {
		return appErr
	}
	return nil
}

func (service *AdminService) RequireAgentProfileChangeAllowed(ctx context.Context, agentID uint64, profileID *uint64) error {
	if agentID == 0 {
		return apperror.BadRequest("无效的AI智能体ID")
	}
	conflictFound, err := service.repository.AgentProfileChangeConflict(ctx, agentID)
	if err != nil {
		return internalAdminError("检查AI智能体上下文引用失败", err)
	}
	if conflictFound {
		return conflict("AI智能体已有上下文数据，不能修改配置")
	}
	if profileID != nil {
		return service.RequireAssignable(ctx, *profileID)
	}
	return nil
}

func (service *AdminService) ContextProfileAssignmentCommitted(ctx context.Context, agentID uint64, profileID uint64) error {
	if service == nil || service.backfill == nil {
		return nil
	}
	return service.backfill.EnqueueAgentBackfill(ctx, agentID, profileID)
}

func (service *AdminService) Evaluate(ctx context.Context, input EvaluationRequest) (*ContextEvaluationResponse, *apperror.Error) {
	input.Query = strings.TrimSpace(input.Query)
	if input.AgentID == 0 || input.Query == "" || len(input.Query) > 20000 {
		return nil, apperror.BadRequest("上下文评测参数错误")
	}
	if service.evaluator == nil {
		return nil, apperror.Internal("上下文评测未配置")
	}
	result, err := service.evaluator.RunEvaluation(ctx, input, EvaluationOptions{Persist: false})
	if err != nil {
		return nil, internalAdminError("上下文评测失败", err)
	}
	return &result, nil
}

func (service *AdminService) GetAgentContextProfile(ctx context.Context, agentID uint64) (*AgentContextProfileInput, *apperror.Error) {
	if agentID == 0 {
		return nil, apperror.BadRequest("无效的AI智能体ID")
	}
	profileID, err := service.repository.GetAgentContextProfile(ctx, agentID)
	if err != nil {
		return nil, internalAdminError("查询AI智能体上下文配置失败", err)
	}
	return &AgentContextProfileInput{ProfileID: profileID}, nil
}

func (service *AdminService) UpdateAgentContextProfile(ctx context.Context, agentID uint64, profileID *uint64) (*AgentContextProfileInput, *apperror.Error) {
	if err := service.RequireAgentProfileChangeAllowed(ctx, agentID, profileID); err != nil {
		if appErr, ok := err.(*apperror.Error); ok {
			return nil, appErr
		}
		return nil, internalAdminError("校验AI智能体上下文配置失败", err)
	}
	if err := service.repository.SetAgentContextProfile(ctx, agentID, profileID); err != nil {
		return nil, internalAdminError("修改AI智能体上下文配置失败", err)
	}
	if profileID != nil {
		_ = service.ContextProfileAssignmentCommitted(ctx, agentID, *profileID)
	}
	return &AgentContextProfileInput{ProfileID: cloneUint64(profileID)}, nil
}

func (service *AdminService) GetAgentContextSpaces(ctx context.Context, agentID uint64) (*AgentContextSpacesInput, *apperror.Error) {
	if agentID == 0 {
		return nil, apperror.BadRequest("无效的AI智能体ID")
	}
	ids, err := service.repository.ListAgentContextSpaces(ctx, agentID)
	if err != nil {
		return nil, internalAdminError("查询AI智能体上下文空间失败", err)
	}
	return &AgentContextSpacesInput{SpaceIDs: ids}, nil
}

func (service *AdminService) UpdateAgentContextSpaces(ctx context.Context, agentID uint64, ids []uint64) (*AgentContextSpacesInput, *apperror.Error) {
	if agentID == 0 {
		return nil, apperror.BadRequest("无效的AI智能体ID")
	}
	seen := make(map[uint64]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			return nil, apperror.BadRequest("上下文空间ID无效")
		}
		if _, exists := seen[id]; exists {
			return nil, apperror.BadRequest("上下文空间ID重复")
		}
		seen[id] = struct{}{}
	}
	if err := service.repository.ReplaceAgentContextSpaces(ctx, agentID, ids); err != nil {
		return nil, internalAdminError("修改AI智能体上下文空间失败", err)
	}
	return &AgentContextSpacesInput{SpaceIDs: append([]uint64(nil), ids...)}, nil
}

func (service *AdminService) requireAssignable(ctx context.Context, id uint64) *apperror.Error {
	profile, err := service.repository.FindProfile(ctx, id)
	if err != nil {
		return internalAdminError("查询上下文配置失败", err)
	}
	if profile == nil || profile.Status != ProfileEnabled || profile.IndexState != ProfileIndexReady || profile.ActiveIndexGeneration == nil {
		return profileUnavailable(nil)
	}
	return nil
}

func (service *AdminService) requireModel(ctx context.Context, id uint64, kind aiprovider.ModelKind) *apperror.Error {
	_, appErr := service.requireModelCapability(ctx, id, kind)
	return appErr
}
func (service *AdminService) requireModelCapability(ctx context.Context, id uint64, kind aiprovider.ModelKind) (*ProviderModelCapability, *apperror.Error) {
	model, err := service.repository.FindProviderModelCapability(ctx, id)
	if err != nil {
		return nil, internalAdminError("查询AI模型能力失败", err)
	}
	if model == nil || !model.Enabled || !model.ProviderEnabled || model.Kind != kind {
		return nil, apperror.BadRequest("AI模型用途不匹配或渠道不可用")
	}
	return model, nil
}

func validSpaceMutation(platform string, actor uint32, input CreateSpaceInput) bool {
	return enum.IsRegisteredPlatform(platform) && actor > 0 && input.ProfileID > 0 && strings.TrimSpace(input.Name) != "" && (input.Status == SpaceEnabled || input.Status == SpaceDisabled)
}
func validDocumentOwner(input CreateDocumentInput) bool {
	return (input.SpaceID != nil) != (input.ConversationID != nil) && strings.TrimSpace(input.Title) != "" && strings.TrimSpace(input.SourceFilename) != "" && input.SourceSize > 0 && (input.SpaceID == nil || (input.SourceMessageID == nil && input.SourceAttachmentIndex == nil)) && (input.ConversationID == nil || (input.SourceMessageID != nil && input.SourceAttachmentIndex != nil))
}

func newQueuedVersion(profileID uint64, input storage.ConditionalObjectInput, metadata storage.ConditionalObjectMetadata, filename, parserName, parserVersion string) ContextDocumentVersion {
	version := ContextDocumentVersion{ProfileID: profileID, SourceStorageProvider: input.StorageProvider, SourceObjectKey: input.ObjectKey,
		SourceETag: metadata.ETag, SourceSize: metadata.Size, SourceMIMEType: metadata.MIMEType, SourceFilename: filename,
		ParserName: parserName, ParserVersion: parserVersion, ChunkerVersion: ChunkerVersionV1, State: DocumentVersionQueued}
	version.SourceFactsSHA256 = sourceFactsHash(version)
	return version
}
func sourceFactsHash(version ContextDocumentVersion) []byte {
	body, _ := json.Marshal([]any{"context_document_source_v1", version.SourceStorageProvider, version.SourceObjectKey, version.SourceETag, version.SourceSize, version.SourceMIMEType, version.SourceFilename})
	sum := sha256.Sum256(body)
	return sum[:]
}
func internalAdminError(message string, err error) *apperror.Error {
	return apperror.LegacyWrap(apperror.CodeInternal, 500, message, err)
}
func conflict(message string) *apperror.Error {
	return apperror.New("ai.context.conflict", apperror.CategoryConflict, 409, apperror.Permanent, "", nil, message)
}
func profileUnavailable(err error) *apperror.Error {
	if err != nil {
		return apperror.Wrap("ai.context.profile_unavailable", apperror.CategoryDependency, 503, apperror.Permanent, "", nil, "上下文配置当前不可用", err)
	}
	return apperror.New("ai.context.profile_unavailable", apperror.CategoryDependency, 503, apperror.Permanent, "", nil, "上下文配置当前不可用")
}
func conditionalObjectAppError(err error) *apperror.Error {
	if errors.Is(err, storage.ErrConditionalObjectUnavailable) || errors.Is(err, storage.ErrConditionalObjectVersionChanged) {
		return apperror.New("ai.context.document_object_changed", apperror.CategoryConflict, 409, apperror.Permanent, "", nil, "文档对象不存在或已变化")
	}
	return internalAdminError("校验文档对象失败", err)
}
