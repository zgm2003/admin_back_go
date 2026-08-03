package contextengine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

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
}

func NewPlanner(dependencies PlannerDependencies) *Planner {
	return &Planner{repository: dependencies.Repository, guardFactory: dependencies.GuardFactory}
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
		Budget: budget, RetrievalOutcome: input.RetrievalOutcome, State: PlanReady,
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
	if input.RetrievalOutcome == RetrievalFailed || input.RetrievalOutcome.Validate() != nil {
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
			attachment := block.Metadata.Attachment
			if attachment == nil {
				return infraai.ChatInput{}, ErrInvalidContextPlan
			}
			ref := infraai.AttachmentRef{
				Kind: attachment.Kind, URL: attachment.URL, ObjectKey: attachment.ObjectKey, ETag: attachment.ETag,
				Size: attachment.Size, MIMEType: attachment.MIMEType, Filename: attachment.Filename,
			}
			compiled.Messages[index].Parts = append(compiled.Messages[index].Parts, infraai.ContentPart{Kind: infraai.ContentPartAttachment, Attachment: &ref})
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
	if err := validateCompiledMessages(compiled.Messages); err != nil {
		return infraai.ChatInput{}, err
	}
	return compiled, nil
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
