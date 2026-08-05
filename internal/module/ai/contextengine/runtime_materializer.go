package contextengine

import (
	"context"
	"crypto/sha256"
	"fmt"

	infraai "admin_back_go/internal/infra/ai"
)

const degradedContextInstruction = "Context enhancement is unavailable for this request. Use only the supplied core context and any explicitly supplied ready conversation memory. Do not claim that space documents or historical attachment retrieval was consulted. Do not emit [C<number>] citations."

const degradedContextStableSourceID = "context_policy:degraded:v1"

const degradedContextPolicySourceType = "context_policy"

type RuntimeFacts struct {
	Fingerprint     InputFingerprintHashInput
	ModelCapability ModelCapabilityHashInput
	Budget          Budget
	Profile         *ProfileSnapshot
	CoreGroups      []PackGroup
	Retrieval       *RuntimeRetrievalFacts
}

type RuntimeRetrievalFacts struct {
	Profile     ContextProfile
	SpaceIDs    []uint64
	CurrentText string
	HasSources  bool
}

type RuntimeFactsReader interface {
	LoadRuntimeFacts(context.Context, RuntimeInput) (RuntimeFacts, error)
}

type RuntimeEvidence struct {
	Outcome    RetrievalOutcome
	Groups     []PackGroup
	Diagnostic *PlanError
	Metrics    ContextPlanMetricsV1
}

type RuntimeEvidenceResolver interface {
	ResolveRuntimeEvidence(context.Context, RuntimeInput, RuntimeFacts) (RuntimeEvidence, error)
}

type PlanMaterializer struct {
	facts       RuntimeFactsReader
	evidence    RuntimeEvidenceResolver
	history     ConversationTurnPager
	attachments HistoricalAttachmentAvailability
	memory      MemoryContextReader
}

func NewPlanMaterializer(facts RuntimeFactsReader, evidence RuntimeEvidenceResolver, history ...ConversationTurnPager) *PlanMaterializer {
	if facts == nil {
		return nil
	}
	materializer := &PlanMaterializer{facts: facts, evidence: evidence}
	if len(history) > 0 {
		materializer.history = history[0]
	}
	return materializer
}

func (materializer *PlanMaterializer) WithMemoryReader(reader MemoryContextReader) *PlanMaterializer {
	if materializer != nil {
		materializer.memory = reader
	}
	return materializer
}

func (materializer *PlanMaterializer) Materialize(ctx context.Context, input RuntimeInput) (BuildPlanInput, error) {
	if materializer == nil || materializer.facts == nil {
		return BuildPlanInput{}, ErrPlanRepositoryNotConfigured
	}
	facts, err := materializer.facts.LoadRuntimeFacts(ctx, input)
	if err != nil {
		return BuildPlanInput{}, err
	}
	output := BuildPlanInput{
		RunID: input.RunID, ReplyCommandID: input.ReplyCommandID, LeaseOwner: input.LeaseOwner, LeaseToken: input.LeaseToken,
		CurrentMessageID: input.CurrentMessageID, AgentID: input.AgentID, UserID: input.UserID,
		ConversationID: input.ConversationID, ProviderID: input.ProviderID, ModelID: input.ModelID,
		APIProtocol: input.APIProtocol, PolicyVersion: facts.Fingerprint.PolicyVersion,
		Fingerprint: facts.Fingerprint, ModelCapability: facts.ModelCapability, Budget: facts.Budget,
		Profile: cloneProfileSnapshot(facts.Profile), RetrievalOutcome: RetrievalSkipped,
		PackGroups: clonePackGroups(facts.CoreGroups), Metrics: ContextPlanMetricsV1{Schema: ContextPlanMetricsSchemaV1},
	}
	memoryContext, err := materializer.runtimeMemory(ctx, input, facts)
	if err != nil {
		return BuildPlanInput{}, fmt.Errorf("load runtime memory: %w", err)
	}
	memory := memoryContext.Record
	memoryBoundary := uint64(0)
	if memory != nil {
		memoryBoundary = memory.ThroughMessageID
		group, err := memoryPackGroup(*memory, facts.ModelCapability.TokenCounterID)
		if err != nil {
			return BuildPlanInput{}, fmt.Errorf("build runtime memory group: %w", err)
		}
		output.PackGroups = append(output.PackGroups, group)
	}
	historyGroups, err := runtimeHistoryGroups(ctx, materializer.history, materializer.attachments, input, facts, memoryBoundary)
	if err != nil {
		return BuildPlanInput{}, fmt.Errorf("build runtime history groups: %w", err)
	}
	output.PackGroups = append(output.PackGroups, historyGroups...)
	if memoryContext.Expected && memory == nil {
		return materializer.degradedInput(output, EnhancementStageMemory, ErrCodeMemoryUnavailable)
	}
	if facts.Profile == nil {
		return output, nil
	}
	if materializer.evidence == nil {
		return materializer.degradedInput(output, EnhancementStageProfile, ErrCodeProfileUnavailable)
	}
	evidence, err := materializer.evidence.ResolveRuntimeEvidence(ctx, input, facts)
	if metricsErr := evidence.Metrics.Validate(); metricsErr != nil {
		return BuildPlanInput{}, fmt.Errorf("validate runtime evidence metrics: %w", metricsErr)
	}
	output.Metrics = evidence.Metrics
	if err != nil {
		failure, ok := AsEnhancementFailure(err)
		if !ok {
			return BuildPlanInput{}, err
		}
		return materializer.degradedInput(output, failure.Stage, failure.Code)
	}
	if err := validateRuntimeEvidence(evidence); err != nil {
		return BuildPlanInput{}, fmt.Errorf("validate runtime evidence: %w", err)
	}
	if evidence.Diagnostic != nil {
		return materializer.degradedInput(output, EnhancementStage(evidence.Diagnostic.Stage), evidence.Diagnostic.Code)
	}
	output.RetrievalOutcome = evidence.Outcome
	output.PackGroups = append(output.PackGroups, composeEvidenceGroups(historyGroups, evidence.Groups, memoryBoundary)...)
	return output, nil
}

func (materializer *PlanMaterializer) degradedInput(output BuildPlanInput, stage EnhancementStage, code ErrorCode) (BuildPlanInput, error) {
	if !validEnhancementFailurePair(stage, code) {
		return BuildPlanInput{}, ErrInvalidContextPlan
	}
	diagnostic, err := NewPlanError(string(stage), code)
	if err != nil {
		return BuildPlanInput{}, err
	}
	instruction, err := degradedInstructionGroup(output)
	if err != nil {
		return BuildPlanInput{}, err
	}
	output.PackGroups = append(degradedPackGroups(output.PackGroups), instruction)
	output.RetrievalOutcome = RetrievalDegraded
	output.Diagnostic = &diagnostic
	return output, nil
}

func degradedInstructionGroup(output BuildPlanInput) (PackGroup, error) {
	counter, err := infraai.ResolveTokenCounter(output.ModelCapability.TokenCounterID)
	if err != nil {
		return PackGroup{}, err
	}
	bound, err := counter.UpperBoundText(degradedContextInstruction)
	if err != nil || bound <= 0 {
		return PackGroup{}, ErrInvalidContextPlan
	}
	source := degradedContextPolicySource()
	content := degradedContextInstruction
	return PackGroup{
		Required: true, Priority: 1, StableSourceID: degradedContextStableSourceID,
		Blocks: []PackBlock{{Block: ContextBlock{
			Kind: BlockSystemInstruction, SourceType: source.SourceType, SourceRef: source.SourceRef, SourceSHA256: source.SourceSHA256,
			AtomicGroupKey: degradedContextStableSourceID, Required: true, Priority: 1, TokenUpperBound: bound,
			ContentSnapshot: &content, Metadata: emptyBlockMetadata(),
		}}},
	}, nil
}

func degradedContextPolicySource() AuthoritySource {
	return AuthoritySource{
		SourceType:   degradedContextPolicySourceType,
		SourceRef:    degradedContextStableSourceID,
		SourceSHA256: sha256.Sum256([]byte(degradedContextInstruction)),
	}
}

func degradedPackGroups(groups []PackGroup) []PackGroup {
	kept := make([]PackGroup, 0, len(groups))
	for _, group := range groups {
		allowed := true
		for _, block := range group.Blocks {
			if block.FusionScore != nil || block.RerankScore != nil || block.Block.Metadata.Retrieval != nil || !degradedBlockKindAllowed(block.Block.Kind) {
				allowed = false
				break
			}
		}
		if allowed {
			kept = append(kept, group)
		}
	}
	return clonePackGroups(kept)
}

func degradedBlockKindAllowed(kind BlockKind) bool {
	switch kind {
	case BlockSystemInstruction, BlockCurrentUserMessage, BlockCurrentAttachment, BlockRecentTurn,
		BlockConversationMemory, BlockToolDefinition, BlockToolCall, BlockToolResult:
		return true
	default:
		return false
	}
}

func validateRuntimeEvidence(evidence RuntimeEvidence) error {
	if err := evidence.Metrics.Validate(); err != nil {
		return err
	}
	if evidence.Outcome.Validate() != nil || evidence.Outcome == RetrievalFailed {
		return ErrInvalidContextPlan
	}
	if evidence.Outcome == RetrievalDegraded {
		if evidence.Diagnostic == nil || evidence.Diagnostic.Validate() != nil || len(evidence.Groups) != 0 ||
			!validEnhancementFailurePair(EnhancementStage(evidence.Diagnostic.Stage), evidence.Diagnostic.Code) {
			return ErrInvalidContextPlan
		}
		return nil
	}
	if evidence.Diagnostic != nil {
		return ErrInvalidContextPlan
	}
	if evidence.Outcome == RetrievalHit && len(evidence.Groups) == 0 {
		return ErrInvalidContextPlan
	}
	if evidence.Outcome != RetrievalHit && len(evidence.Groups) != 0 {
		return ErrInvalidContextPlan
	}
	return nil
}

func clonePackGroups(groups []PackGroup) []PackGroup {
	cloned := make([]PackGroup, len(groups))
	for index, group := range groups {
		cloned[index] = group
		cloned[index].Relevance = clonePointer(group.Relevance)
		cloned[index].ExcludedReason = clonePointer(group.ExcludedReason)
		cloned[index].Blocks = make([]PackBlock, len(group.Blocks))
		for blockIndex, block := range group.Blocks {
			cloned[index].Blocks[blockIndex] = block
			cloned[index].Blocks[blockIndex].Block = cloneContextBlock(block.Block)
			cloned[index].Blocks[blockIndex].FusionScore = clonePointer(block.FusionScore)
			cloned[index].Blocks[blockIndex].RerankScore = clonePointer(block.RerankScore)
		}
	}
	return cloned
}
