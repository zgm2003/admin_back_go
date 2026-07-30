package aimessage

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	infraai "admin_back_go/internal/infra/ai"
	storagecos "admin_back_go/internal/infra/storage/cos"
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/module/ai/capability"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/module/ai/replycommand"
	"admin_back_go/internal/module/ai/requestidentity"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

const timeLayout = "2006-01-02 15:04:05"

const (
	defaultLimit = 20
	maxLimit     = 100
)

type Service struct {
	repository      Repository
	history         HistoryRepository
	replyWaker      ReplyWaker
	cancelPublisher CancelPublisher
	pricingResolver officialmodel.Resolver
	capabilities    infraai.TransportCapabilityResolver
	objectInspector storagecos.ObjectInspector
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
	list := make([]MessageItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, messageItem(row))
	}
	nextID := int64(0)
	if hasMore && len(rows) > 0 {
		nextID = rows[0].ID
	}
	return &ListResponse{List: list, NextID: nextID, HasMore: hasMore}, nil
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
	attachments, appErr := normalizeAttachments(input.Attachments)
	if appErr != nil {
		return nil, appErr
	}
	if content == "" && len(attachments) == 0 {
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
	attachments, appErr = s.inspectImageAttachments(ctx, attachments, effectiveCapabilities)
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
	})
	if err != nil {
		if errors.Is(err, requestidentity.ErrRequestIdentityConflict) || errors.Is(err, requestidentity.ErrRequestIdentityNotReplayable) {
			return nil, apperror.Wrap(requestidentity.ErrorCodeFingerprintConflict, apperror.CategoryConflict, 409, apperror.Permanent, "", nil, "request_id与原请求内容冲突", err)
		}
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "提交AI回复任务失败", err)
	}
	if s.replyWaker != nil {
		_ = s.replyWaker.WakeReply(ctx, created.CommandID)
	}
	return &SendResponse{
		ConversationID: input.ConversationID,
		UserMessageID:  created.UserMessageID,
		CommandID:      created.CommandID,
		RequestID:      created.RequestID,
		State:          created.State,
	}, nil
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

func (s *Service) inspectImageAttachments(ctx context.Context, attachments []Attachment, capabilities officialmodel.Capabilities) ([]Attachment, *apperror.Error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	image := capabilities.ImageInput
	if image == nil || !containsCapability(capabilities.InputModalities, officialmodel.ModalityImage) {
		return nil, apperror.BadRequest("当前模型不支持图片输入")
	}
	if len(attachments) > image.MaxFiles {
		return nil, apperror.BadRequest("图片数量超过当前模型限制")
	}
	if s == nil || s.objectInspector == nil {
		return nil, apperror.Internal("图片附件检查服务未配置")
	}
	results := make([]Attachment, len(attachments))
	errorsByIndex := make([]error, len(attachments))
	var wait sync.WaitGroup
	for index := range attachments {
		index := index
		wait.Go(func() {
			metadata, err := s.objectInspector.Head(ctx, attachments[index].ObjectKey)
			if err != nil {
				errorsByIndex[index] = err
				return
			}
			if metadata.Key != attachments[index].ObjectKey || strings.TrimSpace(metadata.TrustedURL) == "" ||
				!containsCapability(image.MIMETypes, metadata.MIMEType) || metadata.Size <= 0 || metadata.Size > image.MaxBytes {
				errorsByIndex[index] = errors.New("image attachment metadata violates effective capability")
				return
			}
			results[index] = Attachment{
				Type: "image", ObjectKey: metadata.Key, MIMEType: metadata.MIMEType, URL: metadata.TrustedURL,
				Name: attachments[index].Name, Size: metadata.Size,
			}
		})
	}
	wait.Wait()
	for _, err := range errorsByIndex {
		if err != nil {
			return nil, apperror.BadRequest("图片附件无效或超出当前模型限制")
		}
	}
	return results, nil
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
		identities = append(identities, requestidentity.AttachmentIdentity{StorageProvider: "cos", StorageKey: attachment.ObjectKey})
	}
	options := requestidentity.GenerationOptions{MaxOutputTokens: effectiveMaxOutputTokens, Extra: map[string]string{}}
	for key, value := range runtimeParams {
		options.Extra[key] = strconv.FormatFloat(value, 'f', -1, 64)
	}
	if len(options.Extra) == 0 {
		options.Extra = nil
	}
	return requestidentity.BuildChatFingerprint(requestidentity.ChatFingerprintInput{
		UserID: userID, ConversationID: conversationID, AgentID: agent.AgentID, ModelID: agent.ModelID, Text: content, Attachments: identities, Options: options,
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
		return nil, nil
	}
	if len(input) > 5 {
		return nil, apperror.BadRequest("图片数量不能超过5张")
	}
	out := make([]Attachment, 0, len(input))
	for _, item := range input {
		typ := strings.TrimSpace(item.Type)
		objectKey := strings.TrimSpace(item.ObjectKey)
		if typ != "image" {
			return nil, apperror.BadRequest("仅支持图片附件")
		}
		if objectKey == "" {
			return nil, apperror.BadRequest("图片object_key不能为空")
		}
		if item.Size < 0 {
			return nil, apperror.BadRequest("图片大小非法")
		}
		out = append(out, Attachment{
			Type: "image", ObjectKey: objectKey, MIMEType: strings.TrimSpace(item.MIMEType),
			URL: strings.TrimSpace(item.URL), Name: strings.TrimSpace(item.Name), Size: item.Size,
		})
	}
	return out, nil
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

func decodeMetaJSON(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil
	}
	return decoded
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
