package aimessage

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/module/ai/replycommand"
	"admin_back_go/internal/module/ai/requestidentity"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/uploadpolicy"

	gomysql "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	HistoryOperationRevision     = "chat.revision"
	HistoryOperationRegeneration = "chat.regeneration"
)

var (
	ErrHistoryParticipantMissing = errors.New("message history reply participant is not configured")
	ErrHistoryAgentUnavailable   = errors.New("message history agent is unavailable")
	ErrHistorySourceInvalid      = errors.New("message history source snapshot is invalid")
	ErrHistorySourceChanged      = errors.New("message history source attachments changed")
	ErrHistoryRuntimeChanged     = errors.New("message history runtime changed")
	ErrHistoryUploadRuleChanged  = errors.New("message history upload rule changed")
)

type historySourceSnapshot struct {
	target Message
	user   Message
}

type historyRequestFacts struct {
	content       string
	attachments   []Attachment
	runtimeParams map[string]float64
	identity      requestidentity.Input
}

func (r *GormRepository) Revise(ctx context.Context, input EditInput) (HistoryAccepted, error) {
	return r.acceptHistoryReply(ctx, HistoryOperationRevision, input.UserID, input.ConversationID, input.MessageID, input.Content, input.RequestID, input.ValidatedAttachments, input.SourceAttachmentsSHA256, input.SourceRuntimeSHA256, input.UploadRuleToken)
}

func (r *GormRepository) Regenerate(ctx context.Context, input RegenerateInput) (HistoryAccepted, error) {
	return r.acceptHistoryReply(ctx, HistoryOperationRegeneration, input.UserID, input.ConversationID, input.AssistantMessageID, "", input.RequestID, input.ValidatedAttachments, input.SourceAttachmentsSHA256, input.SourceRuntimeSHA256, input.UploadRuleToken)
}

func (r *GormRepository) PrepareAction(ctx context.Context, input HistoryPrepareInput) (HistoryActionPreparation, error) {
	if r == nil || r.db == nil {
		return HistoryActionPreparation{}, ErrRepositoryNotConfigured
	}
	if input.Operation != HistoryOperationRevision && input.Operation != HistoryOperationRegeneration ||
		input.UserID <= 0 || input.ConversationID <= 0 || input.SourceMessageID <= 0 {
		return HistoryActionPreparation{}, ErrHistoryIDsInvalid
	}
	ctx = nonNilHistoryContext(ctx)
	runtime, err := r.historyRuntime(ctx, r.db, input.UserID, input.ConversationID, false)
	if err != nil {
		return HistoryActionPreparation{}, err
	}
	source, err := r.historySource(ctx, r.db, input.Operation, input.UserID, input.ConversationID, input.SourceMessageID, false)
	if err != nil {
		return HistoryActionPreparation{}, err
	}
	if err := validateVisibleHistorySource(input.Operation, source); err != nil {
		return HistoryActionPreparation{}, err
	}
	attachments, _, err := historyMetaInputs(source.user.MetaJSON)
	if err != nil {
		return HistoryActionPreparation{}, err
	}
	digest, err := historyAttachmentsDigest(attachments)
	if err != nil {
		return HistoryActionPreparation{}, ErrHistorySourceInvalid
	}
	return HistoryActionPreparation{
		Runtime: runtime, SourceAttachments: append([]Attachment(nil), attachments...), SourceAttachmentsSHA256: digest,
	}, nil
}

func (r *GormRepository) acceptHistoryReply(
	ctx context.Context,
	operation string,
	userID, conversationID, sourceMessageID int64,
	replacementContent, requestID string,
	validatedAttachments []Attachment,
	sourceAttachmentsSHA256 [32]byte,
	sourceRuntimeSHA256 [32]byte,
	uploadRuleToken uploadpolicy.ConsistencyToken,
) (HistoryAccepted, error) {
	if r == nil || r.db == nil {
		return HistoryAccepted{}, ErrRepositoryNotConfigured
	}
	if r.history == nil {
		return HistoryAccepted{}, ErrHistoryParticipantMissing
	}
	if userID <= 0 || conversationID <= 0 || sourceMessageID <= 0 || strings.TrimSpace(requestID) == "" {
		return HistoryAccepted{}, ErrHistoryIDsInvalid
	}
	ctx = nonNilHistoryContext(ctx)
	validatedAttachments, appErr := normalizeAttachments(validatedAttachments)
	if appErr != nil || !validatedHistoryAttachments(validatedAttachments) {
		return HistoryAccepted{}, ErrHistorySourceInvalid
	}
	replayRequest := r.historyReplayRequest(operation, userID, conversationID, sourceMessageID, replacementContent, requestID, validatedAttachments)

	var accepted HistoryAccepted
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if replay, replayErr := r.history.ReplayInTransaction(ctx, tx, replayRequest); replayErr != nil {
			return replayErr
		} else if replay != nil {
			accepted = HistoryAccepted{Reply: *replay, Replayed: true}
			return nil
		}
		if err := rejectActiveHistoryCommand(tx, userID, conversationID, false); err != nil {
			return err
		}
		if err := lockOwnedHistoryConversation(tx, userID, conversationID); err != nil {
			return err
		}
		if replay, replayErr := r.history.ReplayInTransaction(ctx, tx, replayRequest); replayErr != nil {
			return replayErr
		} else if replay != nil {
			accepted = HistoryAccepted{Reply: *replay, Replayed: true}
			return nil
		}
		if err := rejectActiveHistoryCommand(tx, userID, conversationID, true); err != nil {
			return err
		}
		lockedRuntime, err := r.historyRuntime(ctx, tx, userID, conversationID, true)
		if err != nil {
			return err
		}
		lockedRuntimeDigest, err := historyRuntimeDigest(lockedRuntime)
		if err != nil || subtle.ConstantTimeCompare(lockedRuntimeDigest[:], sourceRuntimeSHA256[:]) != 1 {
			return ErrHistoryRuntimeChanged
		}
		upperBound, err := visibleHistoryUpperBound(tx, conversationID)
		if err != nil {
			return err
		}
		lockedSource, err := r.historySource(ctx, tx, operation, userID, conversationID, sourceMessageID, true)
		if err != nil {
			return err
		}
		if err := validateVisibleHistorySource(operation, lockedSource); err != nil {
			return err
		}
		lockedAttachments, _, err := historyMetaInputs(lockedSource.user.MetaJSON)
		if err != nil {
			return err
		}
		lockedDigest, err := historyAttachmentsDigest(lockedAttachments)
		if err != nil || subtle.ConstantTimeCompare(lockedDigest[:], sourceAttachmentsSHA256[:]) != 1 {
			return ErrHistorySourceChanged
		}
		if len(validatedAttachments) > 0 {
			if uploadRuleToken == (uploadpolicy.ConsistencyToken{}) || r.uploadRuleGuard == nil {
				return ErrHistoryUploadRuleChanged
			}
			if guardErr := r.uploadRuleGuard.GuardActiveInTransaction(ctx, tx, uploadRuleToken); guardErr != nil {
				if errors.Is(guardErr, uploadpolicy.ErrRuleSnapshotChanged) {
					return ErrHistoryUploadRuleChanged
				}
				return guardErr
			}
		}
		createInput, err := r.buildHistoryCreateInput(ctx, operation, userID, conversationID, replacementContent, requestID, lockedSource, lockedRuntime, validatedAttachments)
		if err != nil {
			return err
		}
		if replay, replayErr := r.history.ReplayInTransaction(ctx, tx, createInput.HistoryRequest); replayErr != nil {
			return replayErr
		} else if replay != nil {
			accepted = HistoryAccepted{Reply: *replay, Replayed: true}
			return nil
		}
		cutFrom := lockedSource.user.ID
		if upperBound < cutFrom {
			return ErrHistorySourceNotFound
		}
		now := r.historyNow()
		result := tx.Model(&Message{}).
			Where("conversation_id = ? AND is_del = ? AND id >= ? AND id <= ?", conversationID, enum.CommonNo, cutFrom, upperBound).
			Updates(map[string]any{"is_del": enum.CommonYes, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrHistorySourceNotFound
		}
		createInput.AcceptedAt = now
		created, err := r.history.CreateInTransaction(ctx, tx, createInput)
		if err != nil {
			return err
		}
		if created.UserMessageID <= 0 || created.CommandID == 0 || created.RunID <= 0 || created.ChargeID <= 0 {
			return ErrHistorySourceInvalid
		}
		if err := tx.Table("ai_conversations").
			Where("id = ? AND user_id = ? AND is_del = ?", conversationID, userID, enum.CommonNo).
			Updates(map[string]any{"last_message_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		accepted = HistoryAccepted{Reply: created}
		return nil
	})
	if err != nil {
		if isHistoryDuplicateError(err) {
			return r.loadHistoryReplay(ctx, replayRequest, err)
		}
		return HistoryAccepted{}, err
	}
	return accepted, nil
}

func historyRuntimeDigest(runtime AgentRuntime) ([32]byte, error) {
	raw, err := json.Marshal(runtime)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(raw), nil
}

func (r *GormRepository) DeleteMessages(ctx context.Context, input DeleteInput) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	ids, ok := exactPositiveHistoryIDs(input.IDs)
	if input.UserID <= 0 || input.ConversationID <= 0 || !ok {
		return nil, ErrHistoryIDsInvalid
	}
	ctx = nonNilHistoryContext(ctx)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := rejectActiveHistoryCommand(tx, input.UserID, input.ConversationID, false); err != nil {
			return err
		}
		if err := lockOwnedHistoryConversation(tx, input.UserID, input.ConversationID); err != nil {
			return err
		}
		if err := rejectActiveHistoryCommand(tx, input.UserID, input.ConversationID, true); err != nil {
			return err
		}
		var rows []Message
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id").
			Where("conversation_id = ? AND is_del = ? AND id IN ?", input.ConversationID, enum.CommonNo, ids).
			Order("id ASC").Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(ids) {
			return ErrHistorySourceNotFound
		}
		for index, row := range rows {
			if row.ID != ids[index] {
				return ErrHistorySourceNotFound
			}
		}
		now := r.historyNow()
		result := tx.Model(&Message{}).
			Where("conversation_id = ? AND is_del = ? AND id IN ?", input.ConversationID, enum.CommonNo, ids).
			Updates(map[string]any{"is_del": enum.CommonYes, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(ids)) {
			return ErrHistorySourceNotFound
		}
		var aggregate struct {
			LastMessageAt *time.Time `gorm:"column:last_message_at"`
		}
		if err := tx.Table("ai_messages").Select("MAX(created_at) AS last_message_at").
			Where("conversation_id = ? AND is_del = ?", input.ConversationID, enum.CommonNo).
			Scan(&aggregate).Error; err != nil {
			return err
		}
		return tx.Table("ai_conversations").
			Where("id = ? AND user_id = ? AND is_del = ?", input.ConversationID, input.UserID, enum.CommonNo).
			Updates(map[string]any{"last_message_at": aggregate.LastMessageAt, "updated_at": now}).Error
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *GormRepository) historyReplayRequest(
	operation string,
	userID, conversationID, sourceMessageID int64,
	replacementContent, requestID string,
	validatedAttachments []Attachment,
) replycommand.HistoryRequest {
	return replycommand.HistoryRequest{
		UserID: userID, RequestID: strings.TrimSpace(requestID),
		ResolveIdentity: func(resolveCtx context.Context, db *gorm.DB) (requestidentity.Input, error) {
			runtime, err := r.historyIdentityRuntime(resolveCtx, db, userID, conversationID)
			if err != nil {
				return requestidentity.Input{}, err
			}
			if r.pricing == nil {
				return requestidentity.Input{}, ErrHistoryAgentUnavailable
			}
			model, err := r.pricing.Resolve(resolveCtx, strings.TrimSpace(runtime.ModelID))
			if err != nil || model.Model.MaxOutputTokens <= 0 {
				return requestidentity.Input{}, ErrHistoryAgentUnavailable
			}
			source, err := r.historySource(resolveCtx, db, operation, userID, conversationID, sourceMessageID, false)
			if err != nil {
				return requestidentity.Input{}, err
			}
			facts, err := buildHistoryRequestFacts(operation, userID, conversationID, replacementContent, source, runtime, model.Model.MaxOutputTokens, validatedAttachments)
			if err != nil {
				return requestidentity.Input{}, err
			}
			return facts.identity, nil
		},
	}
}

func (r *GormRepository) historyIdentityRuntime(ctx context.Context, db *gorm.DB, userID, conversationID int64) (AgentRuntime, error) {
	var runtime AgentRuntime
	err := db.WithContext(ctx).Table("ai_conversations").
		Select("ai_agents.id AS agent_id, ai_agents.model_id AS model_id").
		Joins("JOIN ai_agents ON ai_agents.id = ai_conversations.agent_id").
		Where("ai_conversations.id = ? AND ai_conversations.user_id = ? AND ai_conversations.is_del = ?", conversationID, userID, enum.CommonNo).
		Take(&runtime).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AgentRuntime{}, ErrHistorySourceNotFound
	}
	if err != nil {
		return AgentRuntime{}, err
	}
	if runtime.AgentID <= 0 || strings.TrimSpace(runtime.ModelID) == "" {
		return AgentRuntime{}, ErrHistorySourceInvalid
	}
	return runtime, nil
}

func (r *GormRepository) historyRuntime(ctx context.Context, db *gorm.DB, userID, conversationID int64, locked bool) (AgentRuntime, error) {
	query := db.WithContext(ctx).Table("ai_conversations").
		Select(`ai_agents.id AS agent_id, ai_agents.provider_id AS provider_id, ai_agents.model_id AS model_id,
			ai_agents.model_display_name AS model_display_name, ai_providers.engine_type AS engine_type,
			ai_providers.file_input_mode AS file_input_mode,
			ai_agents.billing_multiplier_ppm AS billing_multiplier_ppm,
			ai_agents.status AS status, ai_agents.scenes_json AS scenes_json,
			ai_provider_models.status AS provider_model_status,
			ai_provider_models.official_model_id AS official_model_id,
			ai_provider_models.official_catalog_version AS official_catalog_version,
			ai_provider_models.mapping_status AS mapping_status`).
		Joins("JOIN ai_agents ON ai_agents.id = ai_conversations.agent_id AND ai_agents.is_del = ?", enum.CommonNo).
		Joins("JOIN ai_providers ON ai_providers.id = ai_agents.provider_id AND ai_providers.is_del = ? AND ai_providers.status = ?", enum.CommonNo, enum.CommonYes).
		Joins("JOIN ai_provider_models ON ai_provider_models.provider_id = ai_agents.provider_id AND ai_provider_models.model_id = ai_agents.model_id AND ai_provider_models.status = ? AND ai_provider_models.mapping_status = ?", enum.CommonYes, officialmodel.MappingStatusMapped).
		Where("ai_conversations.id = ? AND ai_conversations.user_id = ? AND ai_conversations.is_del = ?", conversationID, userID, enum.CommonNo)
	if locked {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var runtime AgentRuntime
	err := query.Take(&runtime).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AgentRuntime{}, ErrHistorySourceNotFound
	}
	if err != nil {
		return AgentRuntime{}, err
	}
	if runtime.AgentID <= 0 || runtime.ProviderID <= 0 || strings.TrimSpace(runtime.ModelID) == "" ||
		runtime.Status != enum.CommonYes || runtime.ProviderModelStatus != enum.CommonYes || runtime.BillingMultiplierPPM <= 0 || !agentSupportsChat(runtime.ScenesJSON) {
		return AgentRuntime{}, ErrHistoryAgentUnavailable
	}
	return runtime, nil
}

func (r *GormRepository) historySource(ctx context.Context, db *gorm.DB, operation string, userID, conversationID, sourceMessageID int64, locked bool) (historySourceSnapshot, error) {
	target, err := historyMessageByID(ctx, db, conversationID, sourceMessageID, locked)
	if err != nil {
		return historySourceSnapshot{}, err
	}
	if operation == HistoryOperationRevision {
		if target.Role != enum.AIMessageRoleUser {
			return historySourceSnapshot{}, ErrHistorySourceNotFound
		}
		return historySourceSnapshot{target: target, user: target}, nil
	}
	if operation != HistoryOperationRegeneration || target.Role != enum.AIMessageRoleAssistant {
		return historySourceSnapshot{}, ErrHistorySourceNotFound
	}
	commandQuery := db.WithContext(ctx).Where("user_id = ? AND conversation_id = ? AND assistant_message_id = ?", userID, conversationID, target.ID)
	if target.ReplyCommandID != nil {
		commandQuery = commandQuery.Where("id = ?", *target.ReplyCommandID)
	}
	if locked {
		commandQuery = commandQuery.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var command replycommand.Command
	if err := commandQuery.First(&command).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return historySourceSnapshot{}, ErrHistorySourceNotFound
		}
		return historySourceSnapshot{}, err
	}
	userMessage, err := historyMessageByID(ctx, db, conversationID, command.UserMessageID, locked)
	if err != nil || userMessage.Role != enum.AIMessageRoleUser {
		if err == nil {
			err = ErrHistorySourceNotFound
		}
		return historySourceSnapshot{}, err
	}
	return historySourceSnapshot{target: target, user: userMessage}, nil
}

func historyMessageByID(ctx context.Context, db *gorm.DB, conversationID, messageID int64, locked bool) (Message, error) {
	query := db.WithContext(ctx).Where("conversation_id = ? AND id = ?", conversationID, messageID)
	if locked {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var message Message
	if err := query.First(&message).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Message{}, ErrHistorySourceNotFound
		}
		return Message{}, err
	}
	return message, nil
}

func (r *GormRepository) buildHistoryCreateInput(
	ctx context.Context,
	operation string,
	userID, conversationID int64,
	replacementContent, requestID string,
	source historySourceSnapshot,
	runtime AgentRuntime,
	validatedAttachments []Attachment,
) (replycommand.HistoryCreateInput, error) {
	if r.pricing == nil {
		return replycommand.HistoryCreateInput{}, ErrHistoryAgentUnavailable
	}
	model, err := r.pricing.Resolve(ctx, strings.TrimSpace(runtime.ModelID))
	if err != nil || model.Model.MaxOutputTokens <= 0 {
		return replycommand.HistoryCreateInput{}, ErrHistoryAgentUnavailable
	}
	facts, err := buildHistoryRequestFacts(operation, userID, conversationID, replacementContent, source, runtime, model.Model.MaxOutputTokens, validatedAttachments)
	if err != nil {
		return replycommand.HistoryCreateInput{}, err
	}
	pricingJSON, effectiveMaxTokens, err := resolvePricingSnapshotForSend(ctx, r.pricing, runtime, facts.runtimeParams)
	if err != nil {
		return replycommand.HistoryCreateInput{}, ErrHistoryAgentUnavailable
	}
	inputSnapshot, err := historyInputSnapshot(facts.content, facts.attachments, facts.runtimeParams, facts.identity)
	if err != nil {
		return replycommand.HistoryCreateInput{}, ErrHistorySourceInvalid
	}
	return replycommand.HistoryCreateInput{
		HistoryRequest: replycommand.HistoryRequest{UserID: userID, RequestID: strings.TrimSpace(requestID), Identity: facts.identity},
		ConversationID: conversationID, AgentID: runtime.AgentID, ProviderID: runtime.ProviderID,
		ModelID: strings.TrimSpace(runtime.ModelID), ModelDisplayName: strings.TrimSpace(runtime.ModelDisplayName),
		Content: facts.content, MetaJSON: metaJSONForSend(facts.attachments, facts.runtimeParams), InputSnapshot: inputSnapshot,
		PricingSnapshotJSON: pricingJSON, EffectiveMaxTokens: effectiveMaxTokens,
	}, nil
}

func buildHistoryRequestFacts(
	operation string,
	userID, conversationID int64,
	replacementContent string,
	source historySourceSnapshot,
	runtime AgentRuntime,
	effectiveMaxOutputTokens int64,
	validatedAttachments []Attachment,
) (historyRequestFacts, error) {
	content := source.user.Content
	if operation == HistoryOperationRevision {
		content = strings.TrimSpace(replacementContent)
		if content == "" {
			return historyRequestFacts{}, ErrHistorySourceInvalid
		}
	}
	_, runtimeParams, err := historyMetaInputs(source.user.MetaJSON)
	if err != nil {
		return historyRequestFacts{}, err
	}
	attachments := append([]Attachment(nil), validatedAttachments...)
	if strings.TrimSpace(content) == "" && len(attachments) == 0 {
		return historyRequestFacts{}, ErrHistorySourceInvalid
	}
	identity := historyRequestIdentity(operation, userID, conversationID, source.target.ID, content, attachments, runtimeParams, runtime, effectiveMaxOutputTokens)
	if _, err := requestidentity.BuildFingerprint(identity); err != nil {
		return historyRequestFacts{}, ErrHistorySourceInvalid
	}
	return historyRequestFacts{content: content, attachments: attachments, runtimeParams: runtimeParams, identity: identity}, nil
}

func historyRequestIdentity(operation string, userID, conversationID, sourceMessageID int64, content string, attachments []Attachment, runtimeParams map[string]float64, runtime AgentRuntime, effectiveMaxOutputTokens int64) requestidentity.Input {
	attachmentIdentities := make([]requestidentity.AttachmentIdentity, 0, len(attachments))
	for _, attachment := range attachments {
		digest, err := attachmentIdentitySHA256(attachment)
		if err != nil {
			return requestidentity.Input{}
		}
		attachmentIdentities = append(attachmentIdentities, requestidentity.AttachmentIdentity{StorageProvider: "cos", StorageKey: attachment.ObjectKey, SHA256: digest})
	}
	options := requestidentity.GenerationOptions{MaxOutputTokens: effectiveMaxOutputTokens, Extra: map[string]string{}}
	for key, value := range runtimeParams {
		options.Extra[key] = strconv.FormatFloat(value, 'f', -1, 64)
	}
	if len(options.Extra) == 0 {
		options.Extra = nil
	}
	return requestidentity.Input{
		UserID: userID, Operation: operation, Modality: "chat", AgentID: runtime.AgentID, ModelID: strings.TrimSpace(runtime.ModelID),
		NormalizedText: content, Attachments: attachmentIdentities, Options: options,
		ConversationID: conversationID, SourceMessageID: sourceMessageID, PreserveAttachmentOrder: true,
	}
}

func historyMetaInputs(raw *string) ([]Attachment, map[string]float64, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil, nil
	}
	var meta struct {
		Attachments   []Attachment       `json:"attachments"`
		RuntimeParams map[string]float64 `json:"runtime_params"`
	}
	if err := json.Unmarshal([]byte(*raw), &meta); err != nil {
		return nil, nil, ErrHistorySourceInvalid
	}
	// Legacy messages may contain the former user-controlled output cap. It is
	// read for display elsewhere but never carried into a new history request.
	delete(meta.RuntimeParams, "max_tokens")
	attachments, appErr := normalizeAttachments(meta.Attachments)
	if appErr != nil {
		return nil, nil, ErrHistorySourceInvalid
	}
	runtimeParams, appErr := normalizeRuntimeParams(meta.RuntimeParams)
	if appErr != nil {
		return nil, nil, ErrHistorySourceInvalid
	}
	return attachments, runtimeParams, nil
}

func historyAttachmentsDigest(attachments []Attachment) ([32]byte, error) {
	normalized, appErr := normalizeAttachments(attachments)
	if appErr != nil {
		return [32]byte{}, ErrHistorySourceInvalid
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(raw), nil
}

func validatedHistoryAttachments(attachments []Attachment) bool {
	for _, attachment := range attachments {
		if strings.TrimSpace(attachment.ObjectKey) == "" || strings.TrimSpace(attachment.MIMEType) == "" ||
			strings.TrimSpace(attachment.URL) == "" || strings.TrimSpace(attachment.Name) == "" ||
			strings.TrimSpace(attachment.ETag) == "" || attachment.Size <= 0 {
			return false
		}
	}
	return true
}

func validateVisibleHistorySource(operation string, source historySourceSnapshot) error {
	if source.target.IsDel != enum.CommonNo || source.user.IsDel != enum.CommonNo {
		return ErrHistorySourceNotFound
	}
	if operation == HistoryOperationRevision && (source.target.ID != source.user.ID || source.user.Role != enum.AIMessageRoleUser) {
		return ErrHistorySourceNotFound
	}
	if operation == HistoryOperationRegeneration && (source.target.Role != enum.AIMessageRoleAssistant || source.user.Role != enum.AIMessageRoleUser) {
		return ErrHistorySourceNotFound
	}
	return nil
}

func lockOwnedHistoryConversation(tx *gorm.DB, userID, conversationID int64) error {
	var conversation Conversation
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND user_id = ? AND is_del = ?", conversationID, userID, enum.CommonNo).
		First(&conversation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrHistorySourceNotFound
	}
	return err
}

func rejectActiveHistoryCommand(tx *gorm.DB, userID, conversationID int64, locked bool) error {
	var commands []replycommand.Command
	query := tx.Select("id").
		Where("user_id = ? AND conversation_id = ? AND state IN ?", userID, conversationID,
			[]replycommand.State{replycommand.StatePending, replycommand.StateClaimed, replycommand.StateRunning}).
		Order("id ASC").Limit(1)
	if locked {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := query.Find(&commands).Error
	if err != nil {
		return err
	}
	if len(commands) > 0 {
		return ErrHistoryActiveCommand
	}
	return nil
}

func visibleHistoryUpperBound(tx *gorm.DB, conversationID int64) (int64, error) {
	var aggregate struct {
		MaxID int64 `gorm:"column:max_id"`
	}
	err := tx.Table("ai_messages").Select("COALESCE(MAX(id), 0) AS max_id").
		Where("conversation_id = ? AND is_del = ?", conversationID, enum.CommonNo).Scan(&aggregate).Error
	return aggregate.MaxID, err
}

func exactPositiveHistoryIDs(input []int64) ([]int64, bool) {
	if len(input) == 0 {
		return nil, false
	}
	ids := append([]int64(nil), input...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for index, id := range ids {
		if id <= 0 || index > 0 && ids[index-1] == id {
			return nil, false
		}
	}
	return ids, true
}

func (r *GormRepository) loadHistoryReplay(ctx context.Context, request replycommand.HistoryRequest, original error) (HistoryAccepted, error) {
	var replay *replycommand.CreateReplyResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var replayErr error
		replay, replayErr = r.history.ReplayInTransaction(ctx, tx, request)
		return replayErr
	})
	if err != nil {
		return HistoryAccepted{}, err
	}
	if replay != nil {
		return HistoryAccepted{Reply: *replay, Replayed: true}, nil
	}
	return HistoryAccepted{}, original
}

func isHistoryDuplicateError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var mysqlErr *gomysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

func (r *GormRepository) historyNow() time.Time {
	if r != nil && r.now != nil {
		return r.now()
	}
	return time.Now()
}

func nonNilHistoryContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

var _ HistoryRepository = (*GormRepository)(nil)
