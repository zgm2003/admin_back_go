package contextengine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	infraai "admin_back_go/internal/infra/ai"
)

type PlannerDependencies struct {
	Repository   PlanRepository
	GuardFactory PlanCommitGuardFactory
}

type Planner struct {
	repository   PlanRepository
	guardFactory PlanCommitGuardFactory
}

type BuildPlanInput struct {
	RunID            uint64
	ReplyCommandID   uint64
	LeaseOwner       string
	LeaseToken       uint64
	CurrentMessageID uint64
	AgentID          uint64
	UserID           uint64
	ConversationID   uint64
	ProviderID       uint64
	ModelID          string
	APIProtocol      string
	PolicyVersion    string

	Fingerprint      InputFingerprintHashInput
	ModelCapability  ModelCapabilityHashInput
	Budget           Budget
	Profile          *ProfileSnapshot
	RetrievalOutcome RetrievalOutcome
	PackGroups       []PackGroup
	Diagnostic       *PlanError
}

func NewPlanner(dependencies PlannerDependencies) *Planner {
	return &Planner{repository: dependencies.Repository, guardFactory: dependencies.GuardFactory}
}

func (planner *Planner) FindTerminalByRunID(ctx context.Context, runID uint64) (*ContextPlan, error) {
	if planner == nil || planner.repository == nil || runID == 0 {
		return nil, ErrPlanRepositoryNotConfigured
	}
	return planner.repository.FindTerminalByRunID(ctx, runID)
}

func (planner *Planner) BuildPlan(ctx context.Context, input BuildPlanInput) (ContextPlan, error) {
	if planner == nil || planner.repository == nil || planner.guardFactory == nil {
		return ContextPlan{}, ErrPlanRepositoryNotConfigured
	}
	if input.RunID == 0 {
		return ContextPlan{}, ErrInvalidContextPlan
	}
	existing, err := planner.repository.FindTerminalByRunID(ctx, input.RunID)
	if err != nil {
		return ContextPlan{}, err
	}
	if existing != nil {
		return *existing, nil
	}

	modelCapabilitySHA256, inputFingerprintSHA256, err := validateAndHashBuildPlanInput(input)
	if err != nil {
		return ContextPlan{}, err
	}
	budget := input.Budget
	budget.KnownInputUpperBound = 0
	if err := budget.Validate(); err != nil {
		return ContextPlan{}, err
	}

	plan := ContextPlan{
		RunID: input.RunID, Profile: cloneProfileSnapshot(input.Profile), PolicyVersion: input.PolicyVersion,
		InputFingerprintSHA256: inputFingerprintSHA256, ModelCapabilitySHA256: modelCapabilitySHA256,
		APIProtocol: input.APIProtocol, TokenCounterID: input.ModelCapability.TokenCounterID,
		Budget: budget, RetrievalOutcome: input.RetrievalOutcome, State: PlanReady, Error: clonePointer(input.Diagnostic),
		Metrics: ContextPlanMetricsV1{Schema: ContextPlanMetricsSchemaV1},
	}
	packed, packErr := Pack(PackInput{
		KnownInputBudget:             input.Budget.KnownInputBudget,
		ToolContinuationInputReserve: input.Budget.ToolContinuationInputReserve,
		Candidates:                   input.PackGroups,
	})
	if packErr != nil {
		code := ErrorCode(packErr.Code)
		if code.Validate() != nil {
			return ContextPlan{}, packErr
		}
		planError, err := NewPlanError("packing", code)
		if err != nil {
			return ContextPlan{}, err
		}
		plan.State = PlanFailed
		plan.RetrievalOutcome = RetrievalFailed
		plan.Error = &planError
	} else {
		plan.Budget.KnownInputUpperBound = packed.KnownInputUpperBound
		plan.Items = packed.Items
		if selectedPlanAttachment(plan.Items) {
			plan.Budget.Proof = BudgetOpaqueAttachment
		}
		planHash, err := HashPlan(plan)
		if err != nil {
			return ContextPlan{}, err
		}
		plan.PlanSHA256 = &planHash
	}
	if err := plan.Validate(); err != nil {
		return ContextPlan{}, err
	}

	authority := buildPlanAuthoritySnapshot(plan, input.Fingerprint)
	guard, authoritySHA256, err := planner.guardFactory.GuardFor(authority)
	if err != nil {
		return ContextPlan{}, err
	}
	token := PlanCommitToken{
		RunID: input.RunID, ReplyCommandID: input.ReplyCommandID, LeaseOwner: input.LeaseOwner, LeaseToken: input.LeaseToken,
		InputFingerprintSHA256: inputFingerprintSHA256, AuthoritySnapshotSHA256: authoritySHA256,
	}
	persisted, _, err := planner.repository.PersistTerminal(ctx, plan, guard, token)
	if err != nil {
		return ContextPlan{}, err
	}
	return persisted, nil
}

func selectedPlanAttachment(items []ContextPlanItem) bool {
	for _, item := range items {
		if item.Decision == DecisionSelected && item.Block.Kind.isAttachment() {
			return true
		}
	}
	return false
}

func validateAndHashBuildPlanInput(input BuildPlanInput) ([sha256.Size]byte, [sha256.Size]byte, error) {
	if input.RunID == 0 || input.ReplyCommandID == 0 || input.LeaseToken == 0 || input.CurrentMessageID == 0 ||
		input.AgentID == 0 || input.UserID == 0 || input.ConversationID == 0 || input.ProviderID == 0 ||
		!validIdentifier(input.LeaseOwner, 191) || input.PolicyVersion != input.Fingerprint.PolicyVersion ||
		input.AgentID != input.Fingerprint.AgentID || input.ProviderID != input.Fingerprint.ProviderID ||
		input.ProviderID != input.ModelCapability.ProviderID || input.Fingerprint.ProviderModelID != input.ModelCapability.ProviderModelID ||
		input.ModelID != input.Fingerprint.ModelID || input.ModelID != input.ModelCapability.RequestedModelID ||
		input.APIProtocol != input.ModelCapability.APIProtocol || input.Budget.ContextWindowTokens != input.ModelCapability.ContextWindowTokens ||
		input.Budget.EffectiveOutputTokens > input.ModelCapability.MaxOutputTokens || len(input.PackGroups) == 0 {
		return [sha256.Size]byte{}, [sha256.Size]byte{}, ErrInvalidContextPlan
	}
	if !equalProfileSnapshot(input.Profile, input.Fingerprint.Profile) {
		return [sha256.Size]byte{}, [sha256.Size]byte{}, ErrInvalidContextPlan
	}
	if input.RetrievalOutcome.Validate() != nil || input.RetrievalOutcome == RetrievalFailed {
		return [sha256.Size]byte{}, [sha256.Size]byte{}, ErrInvalidContextPlan
	}
	if input.RetrievalOutcome == RetrievalDegraded {
		if input.Diagnostic == nil || input.Diagnostic.Validate() != nil ||
			!validEnhancementFailurePair(EnhancementStage(input.Diagnostic.Stage), input.Diagnostic.Code) {
			return [sha256.Size]byte{}, [sha256.Size]byte{}, ErrInvalidContextPlan
		}
	} else if input.Diagnostic != nil {
		return [sha256.Size]byte{}, [sha256.Size]byte{}, ErrInvalidContextPlan
	}
	currentFound := false
	for _, message := range input.Fingerprint.Messages {
		if message.ID == input.CurrentMessageID && message.Role == infraai.MessageRoleUser {
			currentFound = true
			break
		}
	}
	if !currentFound {
		return [sha256.Size]byte{}, [sha256.Size]byte{}, ErrInvalidContextPlan
	}
	modelCapabilitySHA256, err := HashModelCapability(input.ModelCapability)
	if err != nil {
		return [sha256.Size]byte{}, [sha256.Size]byte{}, err
	}
	fingerprint := input.Fingerprint
	fingerprint.ModelCapabilitySHA256 = modelCapabilitySHA256
	inputFingerprintSHA256, err := HashInputFingerprint(fingerprint)
	if err != nil {
		return [sha256.Size]byte{}, [sha256.Size]byte{}, err
	}
	return modelCapabilitySHA256, inputFingerprintSHA256, nil
}

func buildPlanAuthoritySnapshot(plan ContextPlan, fingerprint InputFingerprintHashInput) PlanAuthoritySnapshot {
	fingerprint.ModelCapabilitySHA256 = plan.ModelCapabilitySHA256
	snapshot := PlanAuthoritySnapshot{InputFingerprintSHA256: plan.InputFingerprintSHA256, Fingerprint: cloneInputFingerprint(fingerprint)}
	seen := make(map[string][sha256.Size]byte, len(plan.Items))
	for _, item := range plan.Items {
		if item.Decision != DecisionSelected {
			continue
		}
		key := item.Block.SourceType + "\x00" + item.Block.SourceRef
		if previous, exists := seen[key]; exists && previous == item.Block.SourceSHA256 {
			continue
		}
		seen[key] = item.Block.SourceSHA256
		snapshot.Sources = append(snapshot.Sources, AuthoritySource{
			SourceType: item.Block.SourceType, SourceRef: item.Block.SourceRef, SourceSHA256: item.Block.SourceSHA256,
		})
	}
	return snapshot
}

func CompileChatInput(plan ContextPlan) (infraai.ChatInput, error) {
	if err := plan.Validate(); err != nil || plan.State != PlanReady || plan.PlanSHA256 == nil {
		return infraai.ChatInput{}, ErrInvalidContextPlan
	}
	hash, err := HashPlan(plan)
	if err != nil || hash != *plan.PlanSHA256 {
		return infraai.ChatInput{}, fmt.Errorf("%w: plan hash mismatch", ErrInvalidContextPlan)
	}
	rolePreserving, err := usesRolePreservingConversationTurns(plan.Items)
	if err != nil {
		return infraai.ChatInput{}, err
	}
	var compiled infraai.ChatInput
	if rolePreserving {
		compiled, err = compileRolePreservingChatInput(plan)
	} else {
		compiled, err = compileLegacyChatInput(plan)
	}
	if err != nil {
		return infraai.ChatInput{}, err
	}
	if err := validateCompiledMessages(compiled.Messages); err != nil {
		return infraai.ChatInput{}, err
	}
	return compiled, nil
}

func usesRolePreservingConversationTurns(items []ContextPlanItem) (bool, error) {
	structured, legacy := false, false
	for _, item := range items {
		if item.Decision != DecisionSelected || item.Block.Kind != BlockRecentTurn {
			continue
		}
		if item.Block.Metadata.ConversationTurn == nil {
			legacy = true
		} else {
			structured = true
		}
	}
	if structured && legacy {
		return false, ErrInvalidContextPlan
	}
	return structured, nil
}

func compileLegacyChatInput(plan ContextPlan) (infraai.ChatInput, error) {
	compiled := infraai.ChatInput{RunID: plan.RunID}
	userGroups := make(map[string]int)
	for _, item := range plan.Items {
		if item.Decision != DecisionSelected {
			continue
		}
		block := item.Block
		switch block.Kind {
		case BlockCurrentUserMessage:
			index, exists := userGroups[block.AtomicGroupKey]
			if !exists {
				compiled.Messages = append(compiled.Messages, infraai.Message{Role: infraai.MessageRoleUser})
				index = len(compiled.Messages) - 1
				userGroups[block.AtomicGroupKey] = index
			}
			compiled.Messages[index].Parts = append(compiled.Messages[index].Parts, infraai.ContentPart{Kind: infraai.ContentPartText, Text: *block.ContentSnapshot})
		case BlockCurrentAttachment, BlockHistoryAttachment:
			index, exists := userGroups[block.AtomicGroupKey]
			if !exists {
				compiled.Messages = append(compiled.Messages, infraai.Message{Role: infraai.MessageRoleUser})
				index = len(compiled.Messages) - 1
				userGroups[block.AtomicGroupKey] = index
			}
			part, err := attachmentContentPart(block.Metadata.Attachment)
			if err != nil {
				return infraai.ChatInput{}, ErrInvalidContextPlan
			}
			compiled.Messages[index].Parts = append(compiled.Messages[index].Parts, part)
		case BlockDocumentEvidence:
			envelope, err := compileDocumentEvidence(item)
			if err != nil {
				return infraai.ChatInput{}, err
			}
			compiled.Messages = append(compiled.Messages, textMessage(infraai.MessageRoleSystem, envelope))
		case BlockSystemInstruction:
			compiled.Messages = append(compiled.Messages, textMessage(infraai.MessageRoleSystem, *block.ContentSnapshot))
		case BlockRecentTurn, BlockRecalledTurn, BlockConversationMemory, BlockToolDefinition, BlockToolCall, BlockToolResult:
			compiled.Messages = append(compiled.Messages, textMessage(infraai.MessageRoleSystem, *block.ContentSnapshot))
		default:
			return infraai.ChatInput{}, ErrInvalidContextPlan
		}
	}
	return compiled, nil
}

type compiledHistoryTurn struct {
	groupKey    string
	turn        ContextConversationTurnV1
	snapshot    string
	attachments []infraai.ContentPart
}

func compileRolePreservingChatInput(plan ContextPlan) (infraai.ChatInput, error) {
	historyByGroup := make(map[string]*compiledHistoryTurn)
	history := make([]*compiledHistoryTurn, 0)
	seenHistoryMessageIDs := make(map[uint64]struct{})
	for _, item := range plan.Items {
		if item.Decision != DecisionSelected || item.Block.Kind != BlockRecentTurn {
			continue
		}
		turn := item.Block.Metadata.ConversationTurn
		if turn == nil {
			return infraai.ChatInput{}, ErrInvalidContextPlan
		}
		if _, exists := historyByGroup[item.Block.AtomicGroupKey]; exists {
			return infraai.ChatInput{}, ErrInvalidContextPlan
		}
		if _, exists := seenHistoryMessageIDs[turn.UserMessageID]; exists {
			return infraai.ChatInput{}, ErrInvalidContextPlan
		}
		seenHistoryMessageIDs[turn.UserMessageID] = struct{}{}
		group := &compiledHistoryTurn{groupKey: item.Block.AtomicGroupKey, turn: *turn, snapshot: *item.Block.ContentSnapshot}
		historyByGroup[group.groupKey] = group
		history = append(history, group)
	}

	systemMessages := make([]infraai.Message, 0)
	currentMessages := make([]infraai.Message, 0, 1)
	currentGroups := make(map[string]int)
	currentMessage := func(groupKey string) int {
		if index, exists := currentGroups[groupKey]; exists {
			return index
		}
		currentMessages = append(currentMessages, infraai.Message{Role: infraai.MessageRoleUser})
		index := len(currentMessages) - 1
		currentGroups[groupKey] = index
		return index
	}

	for _, item := range plan.Items {
		if item.Decision != DecisionSelected {
			continue
		}
		block := item.Block
		switch block.Kind {
		case BlockCurrentUserMessage:
			index := currentMessage(block.AtomicGroupKey)
			currentMessages[index].Parts = append(currentMessages[index].Parts, infraai.ContentPart{Kind: infraai.ContentPartText, Text: *block.ContentSnapshot})
		case BlockCurrentAttachment:
			part, err := attachmentContentPart(block.Metadata.Attachment)
			if err != nil {
				return infraai.ChatInput{}, err
			}
			index := currentMessage(block.AtomicGroupKey)
			currentMessages[index].Parts = append(currentMessages[index].Parts, part)
		case BlockHistoryAttachment:
			group := historyByGroup[block.AtomicGroupKey]
			if group == nil {
				return infraai.ChatInput{}, ErrInvalidContextPlan
			}
			part, err := attachmentContentPart(block.Metadata.Attachment)
			if err != nil {
				return infraai.ChatInput{}, err
			}
			group.attachments = append(group.attachments, part)
		case BlockRecentTurn:
			continue
		case BlockDocumentEvidence:
			envelope, err := compileDocumentEvidence(item)
			if err != nil {
				return infraai.ChatInput{}, err
			}
			systemMessages = append(systemMessages, textMessage(infraai.MessageRoleSystem, envelope))
		case BlockSystemInstruction, BlockRecalledTurn, BlockConversationMemory, BlockToolDefinition, BlockToolCall, BlockToolResult:
			systemMessages = append(systemMessages, textMessage(infraai.MessageRoleSystem, *block.ContentSnapshot))
		default:
			return infraai.ChatInput{}, ErrInvalidContextPlan
		}
	}
	if len(currentMessages) != 1 {
		return infraai.ChatInput{}, ErrInvalidContextPlan
	}

	sort.Slice(history, func(left, right int) bool {
		return history[left].turn.UserMessageID < history[right].turn.UserMessageID
	})
	compiled := infraai.ChatInput{RunID: plan.RunID, Messages: systemMessages}
	for _, group := range history {
		userContent, attachmentContext, toolContext, assistantContent, err := group.turn.splitSnapshot(group.snapshot)
		if err != nil {
			return infraai.ChatInput{}, err
		}
		userContent = historicalUserText(userContent, attachmentContext, len(group.attachments) > 0)
		parts := make([]infraai.ContentPart, 0, 1+len(group.attachments))
		if strings.TrimSpace(userContent) != "" {
			parts = append(parts, infraai.ContentPart{Kind: infraai.ContentPartText, Text: userContent})
		}
		parts = append(parts, group.attachments...)
		if len(parts) == 0 {
			return infraai.ChatInput{}, ErrInvalidContextPlan
		}
		compiled.Messages = append(compiled.Messages, infraai.Message{Role: infraai.MessageRoleUser, Parts: parts})

		assistantMessage := toolContext
		if group.turn.AssistantDelivery == "stopped" && strings.TrimSpace(assistantContent) != "" {
			assistantMessage += conversationTurnAssistantPrefix(group.turn.AssistantDelivery)
		}
		assistantMessage += assistantContent
		if strings.TrimSpace(assistantMessage) != "" {
			compiled.Messages = append(compiled.Messages, textMessage(infraai.MessageRoleAssistant, assistantMessage))
		}
	}
	compiled.Messages = append(compiled.Messages, currentMessages[0])
	return compiled, nil
}

func historicalUserText(userContent, attachmentContext string, nativeAttachments bool) string {
	if nativeAttachments || attachmentContext == "" {
		return userContent
	}
	attachmentContext = strings.TrimSuffix(attachmentContext, "\n")
	if userContent == "" || strings.HasSuffix(userContent, "\n") {
		return userContent + attachmentContext
	}
	return userContent + "\n" + attachmentContext
}

func attachmentContentPart(attachment *ContextAttachmentV1) (infraai.ContentPart, error) {
	if attachment == nil {
		return infraai.ContentPart{}, ErrInvalidContextPlan
	}
	ref := infraai.AttachmentRef{
		Kind: attachment.Kind, URL: attachment.URL, ObjectKey: attachment.ObjectKey, ETag: attachment.ETag,
		Size: attachment.Size, MIMEType: attachment.MIMEType, Filename: attachment.Filename,
	}
	return infraai.ContentPart{Kind: infraai.ContentPartAttachment, Attachment: &ref}, nil
}

func compileDocumentEvidence(item ContextPlanItem) (string, error) {
	if item.CitationKey == nil || !citationPattern.MatchString(*item.CitationKey) || item.Block.ContentSnapshot == nil ||
		item.Block.Metadata.Document == nil {
		return "", ErrInvalidContextPlan
	}
	document := item.Block.Metadata.Document
	if err := document.Validate(); err != nil {
		return "", err
	}
	locators, err := json.Marshal(document.Locators)
	if err != nil {
		return "", err
	}
	key := *item.CitationKey
	return "[UNTRUSTED_CONTEXT " + key + "]\nsource: " + document.Title + " | locator: " + string(locators) +
		"\ncontent:\n" + *item.Block.ContentSnapshot + "\n[/UNTRUSTED_CONTEXT " + key + "]", nil
}

func textMessage(role infraai.MessageRole, content string) infraai.Message {
	return infraai.Message{Role: role, Parts: []infraai.ContentPart{{Kind: infraai.ContentPartText, Text: content}}}
}

func validateCompiledMessages(messages []infraai.Message) error {
	hasUser := false
	for _, message := range messages {
		if err := message.Validate(); err != nil {
			return err
		}
		if message.Role == infraai.MessageRoleUser {
			hasUser = true
		}
	}
	if !hasUser {
		return errors.New("compiled context plan has no user message")
	}
	return nil
}

func cloneProfileSnapshot(snapshot *ProfileSnapshot) *ProfileSnapshot {
	if snapshot == nil {
		return nil
	}
	cloned := *snapshot
	cloned.IndexGeneration = clonePointer(snapshot.IndexGeneration)
	return &cloned
}

func equalProfileSnapshot(left, right *ProfileSnapshot) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	if left.ID != right.ID || left.SHA256 != right.SHA256 {
		return false
	}
	if left.IndexGeneration == nil || right.IndexGeneration == nil {
		return left.IndexGeneration == nil && right.IndexGeneration == nil
	}
	return *left.IndexGeneration == *right.IndexGeneration
}
