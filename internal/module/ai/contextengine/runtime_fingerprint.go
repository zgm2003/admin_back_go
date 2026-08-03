package contextengine

import (
	"context"
	"crypto/sha256"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/shared/enum"
)

func (reader *GormRuntimeFactsReader) runtimeFingerprint(
	ctx context.Context,
	input RuntimeInput,
	identity runtimeIdentityRow,
	message runtimeMessageRow,
	attachments []runtimeAttachment,
	bindings []runtimeBindingRow,
	tools []runtimeToolRow,
	model ModelCapabilityHashInput,
	resolved officialmodel.ResolvedModel,
) (InputFingerprintHashInput, *ProfileSnapshot, *ContextProfile, *RuntimeRetrievalFacts, error) {
	agentSHA, err := hashRuntimeFacts(struct {
		ID           uint64
		ProviderID   uint64
		ModelID      string
		SystemPrompt string
		ProfileID    *uint64
	}{identity.AgentID, identity.AgentProviderID, identity.AgentModelID, identity.AgentSystemPrompt, identity.AgentContextProfileID})
	if err != nil {
		return InputFingerprintHashInput{}, nil, nil, nil, err
	}
	providerSHA, err := hashRuntimeFacts(struct {
		ID       uint64
		Engine   string
		BaseURL  string
		Protocol string
	}{identity.ProviderID, identity.ProviderEngineType, identity.ProviderBaseURL, identity.ProviderAPIProtocol})
	if err != nil {
		return InputFingerprintHashInput{}, nil, nil, nil, err
	}
	modelSHA, err := HashModelCapability(model)
	if err != nil {
		return InputFingerprintHashInput{}, nil, nil, nil, err
	}
	profile, profileSnapshot, retrieval, err := reader.loadProfileFacts(ctx, identity.AgentContextProfileID, input.AgentID, input.ConversationID, input.UserID)
	if err != nil {
		return InputFingerprintHashInput{}, nil, nil, nil, err
	}
	messageFacts := FingerprintMessage{ID: message.ID, Role: infraai.MessageRoleUser, ContentSHA256: sha256.Sum256([]byte(message.Content))}
	for index, attachment := range attachments {
		messageFacts.Attachments = append(messageFacts.Attachments, FingerprintAttachment{
			Ordinal: uint32(index), Kind: attachment.Kind, URL: attachment.URL, ObjectKey: attachment.ObjectKey,
			ETag: attachment.ETag, Size: attachment.Size, MIMEType: attachment.MIMEType, Filename: attachment.Filename,
		})
	}
	var temperature *FixedScore
	if input.Temperature != nil {
		value, temperatureErr := FixedScoreFromFloat64(*input.Temperature)
		if temperatureErr != nil {
			return InputFingerprintHashInput{}, nil, nil, nil, temperatureErr
		}
		temperature = &value
	}
	fingerprint := InputFingerprintHashInput{
		PolicyVersion: ContextPolicyV1, AgentID: identity.AgentID, AgentSHA256: agentSHA, ProviderID: identity.ProviderID,
		ProviderSHA256: providerSHA, ProviderModelID: identity.ProviderModelID, ModelID: identity.AgentModelID,
		ModelCapabilitySHA256: modelSHA, Profile: profileSnapshot, Messages: []FingerprintMessage{messageFacts},
		Generation: FingerprintGeneration{Temperature: temperature, EffectiveMaxOutputTokens: resolved.Model.MaxOutputTokens},
	}
	for _, binding := range bindings {
		hash, hashErr := hashRuntimeFacts(binding)
		if hashErr != nil {
			return InputFingerprintHashInput{}, nil, nil, nil, hashErr
		}
		fingerprint.Bindings = append(fingerprint.Bindings, FingerprintBinding{ID: binding.ID, SpaceID: binding.SpaceID, SHA256: hash})
	}
	for _, tool := range tools {
		hash, hashErr := hashRuntimeFacts(struct {
			ID          uint64
			Code        string
			Description string
			Parameters  string
		}{tool.ToolID, tool.Code, tool.Description, tool.ParametersJSON})
		if hashErr != nil {
			return InputFingerprintHashInput{}, nil, nil, nil, hashErr
		}
		fingerprint.Tools = append(fingerprint.Tools, FingerprintTool{ID: tool.ToolID, Name: tool.Code, DefinitionSHA256: hash})
	}
	return fingerprint, profileSnapshot, profile, retrieval, nil
}

func (reader *GormRuntimeFactsReader) loadProfileFacts(ctx context.Context, profileID *uint64, agentID, conversationID, userID uint64) (*ContextProfile, *ProfileSnapshot, *RuntimeRetrievalFacts, error) {
	if profileID == nil {
		return nil, nil, nil, nil
	}
	var profile ContextProfile
	if err := reader.db.WithContext(ctx).Where("id = ?", *profileID).Take(&profile).Error; err != nil {
		return nil, nil, nil, err
	}
	snapshot, err := runtimeProfileSnapshot(profile)
	if err != nil {
		return nil, nil, nil, err
	}
	var sourceCount int64
	query := reader.db.WithContext(ctx).Table("ai_context_documents AS document").
		Joins("JOIN ai_context_document_versions AS version ON version.id = document.active_version_id AND version.state = ? AND version.profile_id = ?", DocumentVersionReady, profile.ID).
		Where("document.status = ? AND document.deleted_at IS NULL", DocumentEnabled)
	query = query.Where("((document.conversation_id = ? AND EXISTS (SELECT 1 FROM ai_conversations WHERE id = document.conversation_id AND user_id = ? AND is_del = ?)) OR document.space_id IN (SELECT space_id FROM ai_context_bindings WHERE agent_id = ? AND status = ?))", conversationID, userID, enum.CommonNo, agentID, "enabled")
	if err := query.Count(&sourceCount).Error; err != nil {
		return nil, nil, nil, err
	}
	return &profile, snapshot, &RuntimeRetrievalFacts{Profile: profile, SpaceIDs: nil, CurrentText: "", HasSources: sourceCount > 0}, nil
}

func runtimeProfileSnapshot(profile ContextProfile) (*ProfileSnapshot, error) {
	denseMinScore, err := ParseFixedScore(profile.DenseMinScore)
	if err != nil {
		return nil, err
	}
	var rerankerMinScore *FixedScore
	if profile.RerankerMinScore != nil {
		value, parseErr := ParseFixedScore(*profile.RerankerMinScore)
		if parseErr != nil {
			return nil, parseErr
		}
		rerankerMinScore = &value
	}
	profileHash, err := HashContextProfile(ContextProfileHashInput{
		ID: profile.ID, Name: profile.Name, Status: string(profile.Status), IndexState: profile.IndexState,
		ActiveGeneration: profile.ActiveIndexGeneration, TargetGeneration: profile.TargetIndexGeneration,
		EmbeddingProviderModelID: profile.EmbeddingProviderModelID, EmbeddingDimensions: profile.EmbeddingDimensions,
		EmbeddingMaxInputTokens: profile.EmbeddingMaxInputTokens, EmbeddingTokenCounterID: profile.EmbeddingTokenCounterID,
		DenseDistance: DenseDistance(profile.DenseDistance), DenseMinScore: denseMinScore,
		SparseEncoder: profile.SparseEncoder, SparseEncoderVersion: profile.SparseEncoderVersion,
		RerankerProviderModelID: profile.RerankerProviderModelID, RerankerMinScore: rerankerMinScore, MemoryProviderModelID: profile.MemoryProviderModelID,
		VerifiedUnixMS: optionalUnixMilli(profile.IndexVerifiedAt),
	})
	if err != nil {
		return nil, err
	}
	return &ProfileSnapshot{ID: profile.ID, SHA256: profileHash, IndexGeneration: clonePointer(profile.ActiveIndexGeneration)}, nil
}

func runtimeModelCapability(identity runtimeIdentityRow, resolved officialmodel.ResolvedModel) (ModelCapabilityHashInput, error) {
	model := resolved.Model
	return ModelCapabilityHashInput{
		ProviderID: identity.ProviderID, ProviderModelID: identity.ProviderModelID, RequestedModelID: identity.AgentModelID,
		CanonicalModelID: model.ModelID, APIProtocol: identity.ProviderAPIProtocol, ContextWindowTokens: model.ContextWindowTokens,
		MaxOutputTokens: model.MaxOutputTokens, TokenCounterID: model.TokenCounterID, InputModalities: model.Capabilities.InputModalities,
		OutputModalities: model.Capabilities.OutputModalities, SupportedParameters: model.Capabilities.SupportedParameters,
		SupportsTools: model.Capabilities.SupportsTools, NativeFileInput: model.Capabilities.NativeFileInput,
		ImageInput: model.Capabilities.ImageInput != nil || model.Capabilities.NativeFileInput,
	}, nil
}

func runtimeBudget(model officialmodel.Model, hasTools bool, attachments []runtimeAttachment) (Budget, error) {
	protocol := contextProtocolBaseReserve
	toolReserve := int64(0)
	if hasTools {
		toolReserve = contextToolReserve
		protocol += toolReserve
	}
	safety := contextSafetyMargin
	known := model.ContextWindowTokens - model.MaxOutputTokens - protocol - safety
	if known < 0 {
		return Budget{}, ErrInvalidBudget
	}
	proof := BudgetConservative
	if len(attachments) > 0 {
		proof = BudgetOpaqueAttachment
	}
	return Budget{ContextWindowTokens: model.ContextWindowTokens, EffectiveOutputTokens: model.MaxOutputTokens,
		ProviderProtocolUpperBound: protocol, ToolContinuationInputReserve: toolReserve, PolicySafetyMargin: safety,
		KnownInputBudget: known, Proof: proof}, nil
}
