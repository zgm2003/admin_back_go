package aimessage

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	infraai "admin_back_go/internal/infra/ai"
	storagecos "admin_back_go/internal/infra/storage/cos"
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/module/ai/capability"
	"admin_back_go/internal/module/ai/contextengine"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/module/ai/replycommand"
	"admin_back_go/internal/module/ai/requestidentity"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/uploadpolicy"
)

const timeLayout = "2006-01-02 15:04:05"

const (
	defaultLimit = 20
	maxLimit     = 100
)

type Service struct {
	repository            Repository
	history               HistoryRepository
	replyWaker            ReplyWaker
	cancelPublisher       CancelPublisher
	pricingResolver       officialmodel.Resolver
	capabilities          infraai.TransportCapabilityResolver
	objectInspector       storagecos.ObjectInspector
	uploadRules           uploadpolicy.Resolver
	conversationDocuments contextengine.ConversationDocumentEnsurer
}

type Option func(*Service)

func WithReplyWaker(waker ReplyWaker) Option {
	return func(s *Service) { s.replyWaker = waker }
}

func WithCancelPublisher(publisher CancelPublisher) Option {
	return func(s *Service) { s.cancelPublisher = publisher }
}

func WithHistoryRepository(repository HistoryRepository) Option {
	return func(s *Service) { s.history = repository }
}

func WithPricingResolver(resolver officialmodel.Resolver) Option {
	return func(s *Service) { s.pricingResolver = resolver }
}

func WithTransportCapabilityResolver(resolver infraai.TransportCapabilityResolver) Option {
	return func(s *Service) {
		if resolver != nil {
			s.capabilities = resolver
		}
	}
}

func WithObjectInspector(inspector storagecos.ObjectInspector) Option {
	return func(s *Service) { s.objectInspector = inspector }
}

func WithUploadRuleResolver(resolver uploadpolicy.Resolver) Option {
	return func(s *Service) { s.uploadRules = resolver }
}

func WithConversationDocumentEnsurer(ensurer contextengine.ConversationDocumentEnsurer) Option {
	return func(s *Service) { s.conversationDocuments = ensurer }
}

func NewService(repository Repository, options ...Option) *Service {
	service := &Service{
		repository: repository,
		capabilities: infraai.TransportCapabilityResolverFunc(func(engineType infraai.EngineType) (infraai.CapabilityMetadata, bool) {
			return infraai.DefaultTransportCapabilities(engineType)
		}),
	}
	if history, ok := repository.(HistoryRepository); ok {
		service.history = history
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (s *Service) List(ctx context.Context, userID int64, query ListQuery) (*ListResponse, *apperror.Error) {
	if _, appErr := s.requireOwnedConversation(ctx, userID, query.ConversationID); appErr != nil {
		return nil, appErr
	}
	repo, _ := s.requireRepository()
	query.UserID = userID
	query = normalizeListQuery(query)
	rows, hasMore, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI消息失败", err)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	contexts, appErr := messageContexts(ctx, repo, rows)
	if appErr != nil {
		return nil, appErr
	}
	list := make([]MessageItem, 0, len(rows))
	for _, row := range rows {
		item := messageItem(row)
		item.Context = contexts[row.ID]
		list = append(list, item)
	}
	nextID := int64(0)
	if hasMore && len(rows) > 0 {
		nextID = rows[0].ID
	}
	return &ListResponse{List: list, NextID: nextID, HasMore: hasMore}, nil
}

func messageContexts(ctx context.Context, repository Repository, rows []MessageProjection) (map[int64]*contextengine.MessageContext, *apperror.Error) {
	runIDs := make([]uint64, 0)
	seen := make(map[uint64]struct{})
	for _, row := range rows {
		if !messageCanHaveContext(row) {
			continue
		}
		runID := uint64(*row.RunID)
		if _, exists := seen[runID]; exists {
			continue
		}
		seen[runID] = struct{}{}
		runIDs = append(runIDs, runID)
	}
	result := make(map[int64]*contextengine.MessageContext)
	if len(runIDs) == 0 {
		return result, nil
	}
	planRepository, ok := repository.(ContextPlanRepository)
	if !ok {
		return nil, apperror.Internal("AI消息上下文仓储未配置")
	}
	plans, err := planRepository.ContextPlans(ctx, runIDs)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI消息上下文失败", err)
	}
	for _, row := range rows {
		if !messageCanHaveContext(row) {
			continue
		}
		runID := uint64(*row.RunID)
		plan, exists := plans[runID]
		if !exists {
			continue
		}
		if plan.RunID != runID {
			return nil, apperror.Internal("AI消息上下文关联无效")
		}
		projection, err := contextengine.ProjectMessageContext(row.Content, plan)
		if err != nil {
			return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "AI消息上下文无效", err)
		}
		result[row.ID] = &projection
	}
	return result, nil
}

func messageCanHaveContext(row MessageProjection) bool {
	if row.Role != enum.AIMessageRoleAssistant || row.RunID == nil || *row.RunID <= 0 || row.DeliveryState == nil {
		return false
	}
	return *row.DeliveryState == DeliveryStateCompleted || *row.DeliveryState == DeliveryStateStopped
}

func (s *Service) Send(ctx context.Context, userID int64, input SendInput) (*SendResponse, *apperror.Error) {
	if input.RequestReceivedAt.IsZero() {
		input.RequestReceivedAt = time.Now().UTC()
	}
	_, appErr := s.requireOwnedConversation(ctx, userID, input.ConversationID)
	if appErr != nil {
		return nil, appErr
	}
	content := strings.TrimSpace(input.Content)
	if content == "" && len(input.Attachments) == 0 {
		return nil, apperror.BadRequest("消息内容不能为空")
	}
	runtimeParams, appErr := normalizeRuntimeParams(input.RuntimeParams)
	if appErr != nil {
		return nil, appErr
	}
	requestID := strings.TrimSpace(input.RequestID)
	if requestID == "" || utf8.RuneCountInString(requestID) > 128 {
		return nil, apperror.BadRequest("request_id不能为空")
	}
	repo, _ := s.requireRepository()
	agent, err := repo.AgentForConversation(ctx, input.ConversationID, userID)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
	}
	if agent == nil || agent.Status != enum.CommonYes || agent.ProviderModelStatus != enum.CommonYes || agent.ProviderID <= 0 || strings.TrimSpace(agent.ModelID) == "" || strings.TrimSpace(agent.EngineType) == "" || agent.BillingMultiplierPPM <= 0 || !agentSupportsChat(agent.ScenesJSON) {
		return nil, apperror.BadRequest("该智能体不支持对话场景")
	}
	resolvedModel, err := resolveOfficialModelForSend(ctx, s.pricingResolver, *agent)
	if err != nil {
		if errors.Is(err, officialmodel.ErrModelRetired) {
			return nil, apperror.Wrap("ai.official_model.retired", apperror.CategoryConflict, 409, apperror.Permanent, "", nil, "该官方模型已退役", err)
		}
		return nil, apperror.Wrap("ai.billing.price_unavailable", apperror.CategoryConflict, 409, apperror.Permanent, "", nil, "该智能体缺少可用的模型价格", err)
	}
	effectiveCapabilities, err := s.effectiveChatCapabilities(*agent, resolvedModel.Model.Capabilities)
	if err != nil {
		return nil, apperror.Wrap("ai.capability.unavailable", apperror.CategoryInternal, 500, apperror.Retryable, "", nil, "AI模型能力不可用", err)
	}
	if !containsCapability(effectiveCapabilities.InputModalities, officialmodel.ModalityText) ||
		!containsCapability(effectiveCapabilities.OutputModalities, officialmodel.ModalityText) {
		return nil, apperror.BadRequest("当前模型不支持文本对话")
	}
	if _, overridden := runtimeParams["temperature"]; overridden && !containsCapability(effectiveCapabilities.SupportedParameters, officialmodel.ParameterTemperature) {
		return nil, apperror.BadRequest("当前模型不支持temperature")
	}
	attachments, uploadRuleToken, appErr := s.inspectAttachments(ctx, *agent, resolvedModel.Model.Capabilities, effectiveCapabilities, input.Attachments)
	if appErr != nil {
		return nil, appErr
	}
	pricingSnapshotJSON, effectiveMaxOutputTokens, err := pricingSnapshotFromResolved(*agent, runtimeParams, resolvedModel)
	if err != nil {
		return nil, apperror.Wrap("ai.billing.price_unavailable", apperror.CategoryConflict, 409, apperror.Permanent, "", nil, "该智能体缺少可用的模型价格", err)
	}
	fingerprint, err := buildSendFingerprint(userID, input.ConversationID, content, attachments, runtimeParams, *agent, effectiveMaxOutputTokens)
	if err != nil {
		return nil, apperror.BadRequest("AI消息请求身份无效")
	}
	inputSnapshot, err := sendInputSnapshot(content, attachments, runtimeParams)
	if err != nil {
		return nil, apperror.BadRequest("AI消息输入快照无效")
	}
	created, err := repo.CreateReply(ctx, replycommand.CreateReplyInput{
		ConversationID:        input.ConversationID,
		UserID:                userID,
		AgentID:               agent.AgentID,
		ProviderID:            agent.ProviderID,
		ModelID:               strings.TrimSpace(agent.ModelID),
		ModelDisplayName:      strings.TrimSpace(agent.ModelDisplayName),
		RequestID:             requestID,
		RequestReceivedAt:     input.RequestReceivedAt,
		Content:               content,
		MetaJSON:              metaJSONForSend(attachments, runtimeParams),
		InputSnapshot:         inputSnapshot,
		PricingSnapshotJSON:   pricingSnapshotJSON,
		EffectiveMaxTokens:    effectiveMaxOutputTokens,
		RequestFingerprint:    fingerprint,
		RequestIdentityStatus: requestidentity.IdentityStatusReplayable,
		UploadRuleToken:       uploadRuleToken,
	})
	if err != nil {
		if errors.Is(err, requestidentity.ErrRequestIdentityConflict) || errors.Is(err, requestidentity.ErrRequestIdentityNotReplayable) {
			return nil, apperror.Wrap(requestidentity.ErrorCodeFingerprintConflict, apperror.CategoryConflict, 409, apperror.Permanent, "", nil, "request_id与原请求内容冲突", err)
		}
		if errors.Is(err, replycommand.ErrUploadRuleChanged) {
			return nil, apperror.Wrap("ai.message.acceptance_changed", apperror.CategoryConflict, 409, apperror.Permanent, "aimessage.attachments.upload_rule_changed", nil, "当前上传规则已变化，请刷新后重试", err)
		}
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "提交AI回复任务失败", err)
	}
	if s.replyWaker != nil {
		_ = s.replyWaker.WakeReply(ctx, created.CommandID)
	}
	if agent.ContextProfileID != nil {
		s.ensureConversationDocuments(ctx, uint64(created.UserMessageID))
	}
	return &SendResponse{
		ConversationID: input.ConversationID,
		UserMessageID:  created.UserMessageID,
		CommandID:      created.CommandID,
		RequestID:      created.RequestID,
		State:          created.State,
	}, nil
}

func (s *Service) ensureConversationDocuments(ctx context.Context, messageID uint64) {
	if s == nil || s.conversationDocuments == nil || messageID == 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithoutCancel(ctx)
	if err := s.conversationDocuments.EnsureConversationDocuments(ctx, messageID); err != nil {
		slog.WarnContext(ctx, "AI conversation attachment ingestion deferred to reconciler", "message_id", messageID, "error", err)
	}
}

func (s *Service) pricingSnapshotForSend(ctx context.Context, agent AgentRuntime, runtimeParams map[string]float64) (string, int64, error) {
	if s == nil {
		return "", 0, officialmodel.ErrRepositoryNotConfigured
	}
	return resolvePricingSnapshotForSend(ctx, s.pricingResolver, agent, runtimeParams)
}

func resolvePricingSnapshotForSend(ctx context.Context, resolver officialmodel.Resolver, agent AgentRuntime, runtimeParams map[string]float64) (string, int64, error) {
	model, err := resolveOfficialModelForSend(ctx, resolver, agent)
	if err != nil {
		return "", 0, err
	}
	return pricingSnapshotFromResolved(agent, runtimeParams, model)
}

func resolveOfficialModelForSend(ctx context.Context, resolver officialmodel.Resolver, agent AgentRuntime) (officialmodel.ResolvedModel, error) {
	if resolver == nil {
		return officialmodel.ResolvedModel{}, officialmodel.ErrRepositoryNotConfigured
	}
	return officialmodel.ResolveMappedRoute(ctx, resolver, agent.ModelID, agent.OfficialModelID, agent.OfficialCatalogVersion, agent.MappingStatus)
}

func pricingSnapshotFromResolved(agent AgentRuntime, runtimeParams map[string]float64, model officialmodel.ResolvedModel) (string, int64, error) {
	effective := model.Model.MaxOutputTokens
	if _, forbidden := runtimeParams["max_tokens"]; forbidden || effective <= 0 || effective > int64(^uint(0)>>1) {
		return "", 0, pricing.ErrUnsafeTokenUpperBound
	}
	raw, err := aigateway.EncodePricingSnapshot(model, aigateway.PricingSnapshotInput{
		TransportEngine: strings.TrimSpace(agent.EngineType), RequestedModelID: strings.TrimSpace(agent.ModelID),
		EffectiveMaxOutputTokens: int(effective), MultiplierPPM: agent.BillingMultiplierPPM,
	})
	if err != nil {
		return "", 0, err
	}
	return raw, effective, nil
}

func (s *Service) effectiveChatCapabilities(agent AgentRuntime, official officialmodel.Capabilities) (officialmodel.Capabilities, error) {
	if s == nil || s.capabilities == nil {
		return officialmodel.Capabilities{}, errors.New("transport capability resolver is not configured")
	}
	metadata, ok := s.capabilities.ResolveCapabilities(infraai.EngineType(strings.TrimSpace(agent.EngineType)))
	if !ok {
		return officialmodel.Capabilities{}, errors.New("transport capabilities are unavailable")
	}
	return capability.EffectiveChatCapabilities(
		official,
		metadata,
		agent.ProviderModelStatus == enum.CommonYes && agent.MappingStatus == officialmodel.MappingStatusMapped,
	)
}

func (s *Service) inspectAttachments(
	ctx context.Context,
	runtime AgentRuntime,
	official officialmodel.Capabilities,
	effective officialmodel.Capabilities,
	attachments []Attachment,
) ([]Attachment, uploadpolicy.ConsistencyToken, *apperror.Error) {
	if len(attachments) == 0 {
		return []Attachment{}, uploadpolicy.ConsistencyToken{}, nil
	}
	if len(attachments) > capability.MaxAttachmentsPerMessage {
		return nil, uploadpolicy.ConsistencyToken{}, attachmentValidationError("ai.attachment.too_many", "aimessage.attachments.too_many", "每条消息最多只能添加5个附件")
	}
	if s == nil || s.uploadRules == nil {
		return nil, uploadpolicy.ConsistencyToken{}, apperror.BadRequestKey("aimessage.attachments.upload_rule_unavailable", nil, "当前上传规则不可用")
	}
	uploadRule, err := s.uploadRules.ResolveActive(ctx)
	if err != nil {
		return nil, uploadpolicy.ConsistencyToken{}, apperror.BadRequestKey("aimessage.attachments.upload_rule_unavailable", nil, "当前上传规则不可用")
	}
	if uploadRule.ConsistencyToken == (uploadpolicy.ConsistencyToken{}) {
		return nil, uploadpolicy.ConsistencyToken{}, apperror.BadRequestKey("aimessage.attachments.upload_rule_unavailable", nil, "当前上传规则不可用")
	}
	if s.objectInspector == nil {
		return nil, uploadpolicy.ConsistencyToken{}, apperror.InternalKey("aimessage.attachments.inspector_missing", nil, "附件检查服务未配置")
	}
	metadata, ok := s.capabilities.ResolveCapabilities(infraai.EngineType(strings.TrimSpace(runtime.EngineType)))
	if !ok {
		return nil, uploadpolicy.ConsistencyToken{}, apperror.InternalKey("aimessage.attachments.transport_unavailable", nil, "附件传输能力不可用")
	}
	acceptedFiles := capability.AllowedNativeFileExtensions(uploadRule.FileExtensions)
	nativeFile := capability.ResolveNativeFileCapability(capability.NativeFileCapabilityInput{
		OfficialEnabled:      official.NativeFileInput && containsCapability(official.InputModalities, officialmodel.ModalityFile),
		TransportEnabled:     containsCapability(metadata.InputModalities, officialmodel.ModalityFile),
		ProviderProtocol:     runtime.APIProtocol,
		ProviderRouteEnabled: runtime.ProviderModelStatus == enum.CommonYes && runtime.MappingStatus == officialmodel.MappingStatusMapped,
		PlatformReady:        len(acceptedFiles) > 0,
		AcceptedExtensions:   acceptedFiles,
	})

	locals := make([]Attachment, len(attachments))
	seenKeys := make(map[string]struct{}, len(attachments))
	imageCount := 0
	for index, raw := range attachments {
		item, appErr := normalizeLocalAttachment(raw, uploadRule, nativeFile, effective)
		if appErr != nil {
			return nil, uploadpolicy.ConsistencyToken{}, appErr
		}
		if _, duplicate := seenKeys[item.ObjectKey]; duplicate {
			return nil, uploadpolicy.ConsistencyToken{}, apperror.BadRequestKey("aimessage.attachments.duplicate", nil, "不能重复添加同一个附件")
		}
		seenKeys[item.ObjectKey] = struct{}{}
		if item.Type == "image" {
			imageCount++
		}
		locals[index] = item
	}
	if effective.ImageInput != nil && imageCount > effective.ImageInput.MaxFiles {
		return nil, uploadpolicy.ConsistencyToken{}, attachmentValidationError("ai.attachment.too_many", "aimessage.attachments.image_count_exceeded", "图片数量超过当前模型限制")
	}

	results := make([]Attachment, len(locals))
	groupCtx, cancelGroup := context.WithCancel(ctx)
	defer cancelGroup()
	var firstError error
	var firstErrorOnce sync.Once
	var wait sync.WaitGroup
	for index := range locals {
		index := index
		wait.Go(func() {
			objectMetadata, err := s.objectInspector.Head(groupCtx, locals[index].ObjectKey)
			if err != nil {
				firstErrorOnce.Do(func() {
					firstError = err
					cancelGroup()
				})
				return
			}
			item, err := normalizeTrustedAttachment(locals[index], objectMetadata, effective)
			if err != nil {
				firstErrorOnce.Do(func() {
					firstError = err
					cancelGroup()
				})
				return
			}
			results[index] = item
		})
	}
	wait.Wait()
	if firstError != nil {
		return nil, uploadpolicy.ConsistencyToken{}, attachmentInspectionError(firstError)
	}
	var total int64
	for _, item := range results {
		if uploadRule.MaxFileBytes <= 0 || item.Size > uploadRule.MaxFileBytes {
			return nil, uploadpolicy.ConsistencyToken{}, attachmentValidationError("ai.attachment.file_too_large", "aimessage.attachments.system_size_exceeded", "附件超过当前上传规则限制")
		}
		if item.Type == "file" && item.Size >= capability.MaxNativeFileBytesExclusive {
			return nil, uploadpolicy.ConsistencyToken{}, attachmentValidationError("ai.attachment.file_too_large", "aimessage.attachments.file_size_exceeded", "单个文件必须小于50 MiB")
		}
		if item.Size > capability.MaxMessageAttachmentBytes-total {
			return nil, uploadpolicy.ConsistencyToken{}, attachmentValidationError("ai.attachment.message_total_too_large", "aimessage.attachments.total_size_exceeded", "附件总大小不能超过50 MiB")
		}
		total += item.Size
	}
	return results, uploadRule.ConsistencyToken, nil
}

func normalizeLocalAttachment(
	raw Attachment,
	uploadRule uploadpolicy.Rule,
	nativeFile capability.NativeFileCapability,
	effective officialmodel.Capabilities,
) (Attachment, *apperror.Error) {
	typ := strings.TrimSpace(raw.Type)
	if typ != "image" && typ != "file" {
		return Attachment{}, attachmentValidationError("ai.attachment.type_unsupported", "aimessage.attachments.type_invalid", "附件类型无效")
	}
	objectKey, err := storagecos.TrustedAIChatObjectKey(raw.ObjectKey, typ)
	if err != nil {
		return Attachment{}, apperror.BadRequestKey("aimessage.attachments.object_key_invalid", nil, "附件object_key无效")
	}
	name := strings.TrimSpace(raw.Name)
	if name == "" || strings.ContainsFunc(name, unicode.IsControl) {
		return Attachment{}, apperror.BadRequestKey("aimessage.attachments.name_invalid", nil, "附件名称无效")
	}
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "/" || name == "" {
		return Attachment{}, apperror.BadRequestKey("aimessage.attachments.name_invalid", nil, "附件名称无效")
	}
	nameExt := extensionOf(name)
	keyExt := extensionOf(objectKey)
	if nameExt == "" || nameExt != keyExt {
		return Attachment{}, apperror.BadRequestKey("aimessage.attachments.extension_mismatch", nil, "附件名称与对象扩展名不一致")
	}
	if typ == "image" {
		if effective.ImageInput == nil || !containsCapability(effective.InputModalities, officialmodel.ModalityImage) ||
			!containsCapability(uploadRule.ImageExtensions, nameExt) {
			return Attachment{}, attachmentValidationError("ai.attachment.type_unsupported", "aimessage.attachments.image_unsupported", "当前模型或上传规则不支持该图片")
		}
	} else if !nativeFile.Enabled || !containsCapability(nativeFile.AcceptedExtensions, nameExt) {
		return Attachment{}, nativeFileCapabilityError(nativeFile.DisabledReason)
	}
	return Attachment{Type: typ, ObjectKey: objectKey, Name: name, ETag: strings.TrimSpace(raw.ETag)}, nil
}

func normalizeTrustedAttachment(local Attachment, metadata storagecos.ObjectMetadata, effective officialmodel.Capabilities) (Attachment, error) {
	mimeType := strings.ToLower(strings.TrimSpace(metadata.MIMEType))
	if metadata.Key != local.ObjectKey || strings.TrimSpace(metadata.TrustedURL) == "" ||
		strings.TrimSpace(metadata.ETag) == "" || metadata.Size <= 0 || mimeType == "" {
		return Attachment{}, storagecos.ErrInvalidObjectMetadata
	}
	if local.ETag != "" && local.ETag != strings.TrimSpace(metadata.ETag) {
		return Attachment{}, storagecos.ErrObjectVersionChanged
	}
	extension := extensionOf(local.ObjectKey)
	if local.Type == "image" {
		if effective.ImageInput == nil || !containsCapability(effective.ImageInput.MIMETypes, mimeType) || metadata.Size > effective.ImageInput.MaxBytes ||
			!imageMIMECompatibleWithExtension(mimeType, extension) || mimeType == "image/gif" && !metadata.GIFStaticVerified {
			return Attachment{}, storagecos.ErrInvalidObjectMetadata
		}
	} else if mimeType != "application/octet-stream" && mimeTypeConflictsWithExtension(mimeType, extension) {
		return Attachment{}, storagecos.ErrInvalidObjectMetadata
	}
	return Attachment{
		Type: local.Type, ObjectKey: metadata.Key, MIMEType: mimeType, URL: strings.TrimSpace(metadata.TrustedURL),
		Name: local.Name, Size: metadata.Size, ETag: strings.TrimSpace(metadata.ETag),
	}, nil
}

func imageMIMECompatibleWithExtension(mimeType, extension string) bool {
	switch mimeType {
	case "image/jpeg":
		return extension == "jpeg" || extension == "jpg" || extension == "jfif" || extension == "pjpeg"
	case "image/png":
		return extension == "png"
	case "image/webp":
		return extension == "webp"
	case "image/gif":
		return extension == "gif"
	default:
		return false
	}
}

func nativeFileCapabilityError(reason string) *apperror.Error {
	switch reason {
	case capability.NativeFileDisabledOfficialModel:
		return attachmentValidationError("ai.attachment.model_unsupported", "aimessage.attachments.official_model_unsupported", "当前模型不支持文件输入")
	case capability.NativeFileDisabledProviderProtocol:
		return attachmentValidationError("ai.attachment.provider_api_protocol_unsupported", "aimessage.attachments.provider_api_protocol_unsupported", "当前渠道请求协议不支持文件，请切换为 Responses API")
	case capability.NativeFileDisabledTransport:
		return attachmentValidationError("ai.attachment.transport_unsupported", "aimessage.attachments.transport_unsupported", "当前渠道传输协议不支持文件")
	default:
		return attachmentValidationError("ai.attachment.type_unsupported", "aimessage.attachments.platform_unsupported", "当前平台暂不支持文件输入")
	}
}

func attachmentValidationError(code, messageID, fallback string) *apperror.Error {
	return apperror.New(code, apperror.CategoryValidation, http.StatusBadRequest, apperror.Permanent, messageID, nil, fallback)
}

func attachmentInspectionError(cause error) *apperror.Error {
	switch {
	case errors.Is(cause, storagecos.ErrObjectVersionChanged):
		return apperror.Wrap("ai.attachment.object_version_changed", apperror.CategoryConflict, http.StatusConflict, apperror.Permanent,
			"aimessage.attachments.invalid", nil, "附件版本已变化，请重新上传后重试", cause)
	case errors.Is(cause, storagecos.ErrInvalidObjectMetadata), errors.Is(cause, storagecos.ErrUntrustedObjectKey):
		return apperror.Wrap("ai.attachment.type_unsupported", apperror.CategoryValidation, http.StatusBadRequest, apperror.Permanent,
			"aimessage.attachments.invalid", nil, "附件类型或元数据不受支持", cause)
	default:
		return apperror.Wrap("ai.attachment.object_unavailable", apperror.CategoryConflict, http.StatusConflict, apperror.Permanent,
			"aimessage.attachments.invalid", nil, "附件对象当前不可用，请重新上传后重试", cause)
	}
}

func extensionOf(value string) string {
	return strings.TrimPrefix(strings.ToLower(path.Ext(value)), ".")
}

func mimeTypeConflictsWithExtension(mimeType, extension string) bool {
	if mimeType == "text/plain" {
		expected, _, _ := mime.ParseMediaType(mime.TypeByExtension("." + extension))
		if expected == "" || strings.HasPrefix(expected, "text/") || isTextLikeApplicationMIME(expected) {
			return false
		}
		return true
	}
	extensions, err := mime.ExtensionsByType(mimeType)
	if err != nil || len(extensions) == 0 {
		return strings.HasPrefix(mimeType, "image/") || strings.HasPrefix(mimeType, "audio/") || strings.HasPrefix(mimeType, "video/")
	}
	for _, candidate := range extensions {
		if strings.TrimPrefix(strings.ToLower(candidate), ".") == extension {
			return false
		}
	}
	return true
}

func isTextLikeApplicationMIME(mimeType string) bool {
	switch mimeType {
	case "application/json", "application/ld+json", "application/xml", "application/javascript", "application/sql", "application/yaml", "application/toml":
		return true
	default:
		return strings.HasSuffix(mimeType, "+json") || strings.HasSuffix(mimeType, "+xml")
	}
}

func containsCapability(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func sendInputSnapshot(content string, attachments []Attachment, runtimeParams map[string]float64) (string, error) {
	if len(attachments) == 0 && len(runtimeParams) == 0 {
		return content, nil
	}
	return encodeChatInputSnapshot(content, attachments, runtimeParams, nil)
}

type historyRequestIdentitySnapshot struct {
	Operation       string `json:"operation"`
	SourceMessageID int64  `json:"source_message_id"`
}

func historyInputSnapshot(content string, attachments []Attachment, runtimeParams map[string]float64, identity requestidentity.Input) (string, error) {
	operation := strings.TrimSpace(identity.Operation)
	if (operation != HistoryOperationRevision && operation != HistoryOperationRegeneration) || identity.SourceMessageID <= 0 {
		return "", errors.New("invalid history request identity snapshot")
	}
	return encodeChatInputSnapshot(content, attachments, runtimeParams, &historyRequestIdentitySnapshot{
		Operation: operation, SourceMessageID: identity.SourceMessageID,
	})
}

func encodeChatInputSnapshot(content string, attachments []Attachment, runtimeParams map[string]float64, identity *historyRequestIdentitySnapshot) (string, error) {
	raw, err := json.Marshal(struct {
		Content         string                          `json:"content"`
		Attachments     []Attachment                    `json:"attachments,omitempty"`
		RuntimeParams   map[string]float64              `json:"runtime_params,omitempty"`
		RequestIdentity *historyRequestIdentitySnapshot `json:"request_identity,omitempty"`
	}{Content: content, Attachments: attachments, RuntimeParams: runtimeParams, RequestIdentity: identity})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (s *Service) Cancel(ctx context.Context, userID int64, input CancelInput) (*CancelResponse, *apperror.Error) {
	if _, appErr := s.requireOwnedConversation(ctx, userID, input.ConversationID); appErr != nil {
		return nil, appErr
	}
	requestID := strings.TrimSpace(input.RequestID)
	if requestID == "" || utf8.RuneCountInString(requestID) > 128 {
		return nil, apperror.BadRequest("request_id不能为空")
	}
	repo, _ := s.requireRepository()
	result, err := repo.RequestCancel(ctx, replycommand.RequestCancelInput{
		ConversationID: input.ConversationID,
		UserID:         userID,
		RequestID:      requestID,
		DeliveredSeq:   input.DeliveredSeq,
		Now:            time.Now(),
	})
	if err != nil {
		if errors.Is(err, replycommand.ErrReplyCommandNotFound) {
			return nil, apperror.NotFound("AI回复任务不存在")
		}
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "取消AI回复失败", err)
	}
	if result.CommandID == 0 {
		return nil, apperror.NotFound("AI回复任务不存在")
	}
	if !result.DeliveryConsistent && result.Status == replycommand.CancelStatusStopped {
		slog.Default().WarnContext(context.WithoutCancel(ctx), "AI stopped delivery prefix was inconsistent",
			"command_id", result.CommandID,
			"request_id", requestID,
			"requested_delivery_seq", input.DeliveredSeq,
			"stop_delivery_seq", result.StopDeliverySeq,
		)
	}
	if s.cancelPublisher != nil && result.Status == replycommand.CancelStatusStopped && result.SettlementPending {
		_ = s.cancelPublisher.PublishCancel(context.WithoutCancel(ctx), result.CommandID)
	}
	var assistantMessageID *int64
	if result.AssistantMessageID > 0 {
		assistantID := result.AssistantMessageID
		assistantMessageID = &assistantID
	}
	return &CancelResponse{
		ConversationID:     input.ConversationID,
		RequestID:          requestID,
		Status:             string(result.Status),
		AssistantMessageID: assistantMessageID,
		SettlementPending:  result.SettlementPending,
	}, nil
}

func buildSendFingerprint(userID, conversationID int64, content string, attachments []Attachment, runtimeParams map[string]float64, agent AgentRuntime, effectiveMaxOutputTokens int64) ([32]byte, error) {
	identities := make([]requestidentity.AttachmentIdentity, 0, len(attachments))
	for _, attachment := range attachments {
		digest, err := attachmentIdentitySHA256(attachment)
		if err != nil {
			return [32]byte{}, err
		}
		identities = append(identities, requestidentity.AttachmentIdentity{StorageProvider: "cos", StorageKey: attachment.ObjectKey, SHA256: digest})
	}
	options := requestidentity.GenerationOptions{MaxOutputTokens: effectiveMaxOutputTokens, Extra: map[string]string{}}
	for key, value := range runtimeParams {
		options.Extra[key] = strconv.FormatFloat(value, 'f', -1, 64)
	}
	if len(options.Extra) == 0 {
		options.Extra = nil
	}
	return requestidentity.BuildChatFingerprint(requestidentity.ChatFingerprintInput{
		UserID: userID, ConversationID: conversationID, AgentID: agent.AgentID, ModelID: agent.ModelID, Text: content,
		Attachments: identities, Options: options, PreserveAttachmentOrder: true,
	})
}

func metaJSONForSend(attachments []Attachment, runtimeParams map[string]float64) *string {
	meta := map[string]any{}
	if len(attachments) > 0 {
		meta["attachments"] = attachments
	}
	if len(runtimeParams) > 0 {
		meta["runtime_params"] = runtimeParams
	}
	if len(meta) == 0 {
		return nil
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return nil
	}
	value := string(data)
	return &value
}

func normalizeAttachments(input []Attachment) ([]Attachment, *apperror.Error) {
	if len(input) == 0 {
		return []Attachment{}, nil
	}
	if len(input) > capability.MaxAttachmentsPerMessage {
		return nil, apperror.BadRequest("附件数量不能超过5个")
	}
	out := make([]Attachment, 0, len(input))
	for _, item := range input {
		typ := strings.TrimSpace(item.Type)
		objectKey := strings.TrimSpace(item.ObjectKey)
		if typ != "image" && typ != "file" {
			return nil, apperror.BadRequest("附件类型无效")
		}
		if objectKey == "" {
			return nil, apperror.BadRequest("附件object_key不能为空")
		}
		if item.Size < 0 {
			return nil, apperror.BadRequest("附件大小非法")
		}
		out = append(out, Attachment{
			Type: typ, ObjectKey: objectKey, MIMEType: strings.ToLower(strings.TrimSpace(item.MIMEType)),
			URL: strings.TrimSpace(item.URL), Name: strings.TrimSpace(item.Name), Size: item.Size, ETag: strings.TrimSpace(item.ETag),
		})
	}
	return out, nil
}

func attachmentIdentitySHA256(attachment Attachment) (string, error) {
	return requestidentity.AttachmentFactsSHA256(requestidentity.AttachmentFacts{
		Type: attachment.Type, ObjectKey: attachment.ObjectKey, ETag: attachment.ETag,
		Size: attachment.Size, MIMEType: attachment.MIMEType, Name: attachment.Name,
	})
}

func normalizeRuntimeParams(input map[string]float64) (map[string]float64, *apperror.Error) {
	if len(input) == 0 {
		return nil, nil
	}
	out := map[string]float64{}
	for key, value := range input {
		switch key {
		case "temperature":
			if value < 0 || value > 2 {
				return nil, apperror.BadRequest("temperature必须在0到2之间")
			}
			out[key] = value
		case "max_tokens":
			return nil, apperror.BadRequest("max_tokens由官方模型上限统一控制，不能设置")
		case "max_history":
			if value < 1 || value > 50 || value != float64(int64(value)) {
				return nil, apperror.BadRequest("max_history必须是1到50之间的整数")
			}
			out[key] = value
		default:
			return nil, apperror.BadRequest("不支持的AI运行参数")
		}
	}
	return out, nil
}

func decodeMetaJSON(raw string) *MessageMeta {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var decoded MessageMeta
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil
	}
	return &decoded
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("AI消息仓储未配置")
	}
	return s.repository, nil
}

func (s *Service) requireOwnedConversation(ctx context.Context, userID int64, id int64) (*Conversation, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.Unauthorized("Token无效或已过期")
	}
	if id <= 0 {
		return nil, apperror.BadRequest("无效的AI会话ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.Conversation(ctx, id)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI会话失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("AI会话不存在")
	}
	if row.UserID != userID {
		return nil, apperror.Forbidden("无权访问该AI会话")
	}
	return row, nil
}

func normalizeListQuery(query ListQuery) ListQuery {
	if query.Limit <= 0 {
		query.Limit = defaultLimit
	}
	if query.Limit > maxLimit {
		query.Limit = maxLimit
	}
	return query
}

func messageItem(row MessageProjection) MessageItem {
	contentType := strings.TrimSpace(row.ContentType)
	if contentType == "" {
		contentType = "text"
	}
	metaJSON := ""
	if row.MetaJSON != nil {
		metaJSON = *row.MetaJSON
	}
	return MessageItem{
		ID: row.ID, Role: row.Role, ContentType: contentType, Content: row.Content, MetaJSON: decodeMetaJSON(metaJSON),
		PairedMessageID: row.PairedMessageID, RunID: row.RunID, Liked: row.Liked,
		DeliveryState: row.DeliveryState, SettlementPending: row.SettlementPending,
		CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt),
	}
}

func agentSupportsChat(raw string) bool {
	var scenes []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &scenes); err != nil || len(scenes) == 0 {
		return false
	}
	for _, scene := range scenes {
		if strings.TrimSpace(scene) == "chat" {
			return true
		}
	}
	return false
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(timeLayout)
}
