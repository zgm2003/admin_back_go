package contextengine

import "context"

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
	Outcome RetrievalOutcome
	Groups  []PackGroup
	Failure *PlanError
}

type RuntimeEvidenceResolver interface {
	ResolveRuntimeEvidence(context.Context, RuntimeInput, RuntimeFacts) (RuntimeEvidence, error)
}

type PlanMaterializer struct {
	facts    RuntimeFactsReader
	evidence RuntimeEvidenceResolver
	history  ConversationTurnPager
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
		PackGroups: clonePackGroups(facts.CoreGroups),
	}
	historyGroups, err := runtimeHistoryGroups(ctx, materializer.history, input, facts)
	if err != nil {
		return BuildPlanInput{}, err
	}
	output.PackGroups = append(output.PackGroups, historyGroups...)
	if facts.Profile == nil {
		return output, nil
	}
	if materializer.evidence == nil {
		failure, failureErr := NewPlanError("profile", ErrCodeProfileUnavailable)
		if failureErr != nil {
			return BuildPlanInput{}, failureErr
		}
		output.RetrievalOutcome = RetrievalFailed
		output.Failure = &failure
		return output, nil
	}
	evidence, err := materializer.evidence.ResolveRuntimeEvidence(ctx, input, facts)
	if err != nil {
		return BuildPlanInput{}, err
	}
	if err := validateRuntimeEvidence(evidence); err != nil {
		return BuildPlanInput{}, err
	}
	output.RetrievalOutcome = evidence.Outcome
	output.Failure = clonePointer(evidence.Failure)
	if evidence.Failure == nil {
		output.PackGroups = append(output.PackGroups, clonePackGroups(evidence.Groups)...)
	}
	return output, nil
}

func validateRuntimeEvidence(evidence RuntimeEvidence) error {
	if evidence.Outcome.Validate() != nil || (evidence.Outcome == RetrievalFailed) != (evidence.Failure != nil) {
		return ErrInvalidContextPlan
	}
	if evidence.Failure != nil {
		if evidence.Failure.Validate() != nil || len(evidence.Groups) != 0 {
			return ErrInvalidContextPlan
		}
		return nil
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
