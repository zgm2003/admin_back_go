package contextengine

import (
	"context"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
	"gorm.io/gorm"
)

func TestBuildPlanPersistsOneReadyPlanWithCanonicalHash(t *testing.T) {
	repository := &fakePlannerRepository{}
	planner := NewPlanner(PlannerDependencies{Repository: repository, GuardFactory: fixedGuardFactory{hash: testSHA256("authority")}})
	plan, err := planner.BuildPlan(context.Background(), validBuildPlanInput())
	if err != nil {
		t.Fatal(err)
	}
	if plan.ID != 91 || plan.State != PlanReady || plan.PlanSHA256 == nil || repository.persistCalls != 1 {
		t.Fatalf("plan=%+v persist_calls=%d", plan, repository.persistCalls)
	}
	want, err := HashPlan(plan)
	if err != nil || want != *plan.PlanSHA256 {
		t.Fatalf("plan hash=%x want=%x err=%v", *plan.PlanSHA256, want, err)
	}
	if repository.token.InputFingerprintSHA256 != plan.InputFingerprintSHA256 || repository.token.AuthoritySnapshotSHA256 != testSHA256("authority") {
		t.Fatalf("token=%+v", repository.token)
	}
}

func TestBuildPlanReturnsExistingTerminalWithoutRebuilding(t *testing.T) {
	existing := validReadyPlan()
	repository := &fakePlannerRepository{existing: &existing}
	planner := NewPlanner(PlannerDependencies{Repository: repository, GuardFactory: fixedGuardFactory{hash: testSHA256("authority")}})
	plan, err := planner.BuildPlan(context.Background(), validBuildPlanInput())
	if err != nil || plan.RunID != existing.RunID || repository.persistCalls != 0 {
		t.Fatalf("plan=%+v err=%v persist_calls=%d", plan, err, repository.persistCalls)
	}
}

func TestBuildPlanPersistsRequiredOverflowAsFailed(t *testing.T) {
	repository := &fakePlannerRepository{}
	planner := NewPlanner(PlannerDependencies{Repository: repository, GuardFactory: fixedGuardFactory{hash: testSHA256("authority")}})
	input := validBuildPlanInput()
	input.Budget.KnownInputBudget = 1
	input.Budget.ContextWindowTokens = input.Budget.EffectiveOutputTokens + input.Budget.ProviderProtocolUpperBound + input.Budget.PolicySafetyMargin + 1
	input.ModelCapability.ContextWindowTokens = input.Budget.ContextWindowTokens
	plan, err := planner.BuildPlan(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if plan.State != PlanFailed || plan.Error == nil || plan.Error.Code != ErrCodeRequiredOverflow || plan.PlanSHA256 != nil || len(plan.Items) != 0 {
		t.Fatalf("plan=%+v", plan)
	}
}

func TestCompileChatInputKeepsCurrentUserTextByteStable(t *testing.T) {
	plan := validReadyPlan()
	hash, err := HashPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.PlanSHA256 = &hash
	compiled, err := CompileChatInput(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Messages) != 1 || compiled.Messages[0].Role != infraai.MessageRoleUser || compiled.Messages[0].Parts[0].Text != "hello" {
		t.Fatalf("compiled=%+v", compiled)
	}
}

func TestCompileChatInputWrapsDocumentEvidenceAsUntrustedSystemContext(t *testing.T) {
	plan := validReadyPlan()
	paragraph := uint32(2)
	content := "refunds take three business days"
	citation := "C1"
	plan.Items = append(plan.Items, ContextPlanItem{
		Ordinal: 2,
		Block: ContextBlock{
			Kind: BlockDocumentEvidence, SourceType: "document_chunk", SourceRef: "chunk:20",
			SourceSHA256: testSHA256("chunk:20"), AtomicGroupKey: "document:10", Priority: 200,
			TokenUpperBound: 33, ContentSnapshot: &content,
			Metadata: ContextBlockMetadataV1{Schema: ContextBlockMetadataSchemaV1, Document: &ContextDocumentEvidenceV1{
				Title: "Refund policy", DocumentID: 10, DocumentVersionID: 11, ChunkIDs: []uint64{20},
				Locators: []ContextLocatorV1{{Schema: ContextLocatorSchemaV1, Kind: "paragraph", Paragraph: &paragraph}},
			}},
		},
		Decision: DecisionSelected, CitationKey: &citation,
	})
	plan.Budget.KnownInputUpperBound += 33
	hash, err := HashPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.PlanSHA256 = &hash

	compiled, err := CompileChatInput(plan)
	if err != nil {
		t.Fatal(err)
	}
	want := "[UNTRUSTED_CONTEXT C1]\nsource: Refund policy | locator: [{\"schema\":\"context_locator_v1\",\"kind\":\"paragraph\",\"paragraph\":2}]\ncontent:\nrefunds take three business days\n[/UNTRUSTED_CONTEXT C1]"
	if len(compiled.Messages) != 2 || compiled.Messages[1].Role != infraai.MessageRoleSystem || compiled.Messages[1].Parts[0].Text != want {
		t.Fatalf("compiled=%+v", compiled)
	}
}

func TestCompileChatInputRejectsChangedPlanAfterHash(t *testing.T) {
	plan := validReadyPlan()
	hash, err := HashPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.PlanSHA256 = &hash
	changed := "changed"
	plan.Items[0].Block.ContentSnapshot = &changed
	if _, err := CompileChatInput(plan); err == nil {
		t.Fatal("changed plan compiled with a stale hash")
	}
}

type fakePlannerRepository struct {
	existing     *ContextPlan
	persisted    ContextPlan
	token        PlanCommitToken
	persistCalls int
}

func (repository *fakePlannerRepository) FindTerminalByRunID(context.Context, uint64) (*ContextPlan, error) {
	return repository.existing, nil
}

func (repository *fakePlannerRepository) PersistTerminal(_ context.Context, plan ContextPlan, _ PlanCommitTransactionGuard, token PlanCommitToken) (ContextPlan, PersistDisposition, error) {
	repository.persistCalls++
	repository.persisted = plan
	repository.token = token
	if err := plan.Validate(); err != nil {
		return ContextPlan{}, "", err
	}
	plan.ID = 91
	return plan, PersistCreated, nil
}

type fixedGuardFactory struct {
	hash [32]byte
	err  error
}

func (factory fixedGuardFactory) GuardFor(PlanAuthoritySnapshot) (PlanCommitTransactionGuard, [32]byte, error) {
	return noopPlanGuard{}, factory.hash, factory.err
}

type noopPlanGuard struct{}

func (noopPlanGuard) GuardPlanCommitInTransaction(context.Context, *gorm.DB, PlanCommitToken) (PlanCommitGuardResult, error) {
	return PlanCommitGuardResult{}, nil
}

func validBuildPlanInput() BuildPlanInput {
	block := testPackBlock(BlockCurrentUserMessage, "message:9", 10, "hello")
	fingerprint := InputFingerprintHashInput{
		PolicyVersion: "context_policy_v1", AgentID: 5, AgentSHA256: testSHA256("agent"),
		ProviderID: 7, ProviderSHA256: testSHA256("provider"), ProviderModelID: 8, ModelID: "gpt-5.6",
		ModelCapabilitySHA256: testSHA256("capability"),
		Messages:              []FingerprintMessage{{ID: 9, Role: infraai.MessageRoleUser, ContentSHA256: testSHA256("hello")}},
	}
	return BuildPlanInput{
		RunID: 44, ReplyCommandID: 77, LeaseOwner: "worker-a", LeaseToken: 3,
		CurrentMessageID: 9, AgentID: 5, UserID: 7, ConversationID: 3, ProviderID: 7, ModelID: "gpt-5.6", APIProtocol: APIProtocolResponses,
		PolicyVersion: "context_policy_v1", Fingerprint: fingerprint,
		ModelCapability: ModelCapabilityHashInput{
			ProviderID: 7, ProviderModelID: 8, RequestedModelID: "gpt-5.6", CanonicalModelID: "gpt-5.6",
			APIProtocol: APIProtocolResponses, ContextWindowTokens: 1000, MaxOutputTokens: 100,
			TokenCounterID: "utf8_bytes_v1", InputModalities: []string{"text"},
		},
		Budget:           Budget{ContextWindowTokens: 1000, EffectiveOutputTokens: 100, ProviderProtocolUpperBound: 50, ToolContinuationInputReserve: 25, PolicySafetyMargin: 50, KnownInputBudget: 800, Proof: BudgetConservative},
		RetrievalOutcome: RetrievalSkipped,
		PackGroups:       []PackGroup{{Required: true, Priority: 1, SourceOrder: 9, StableSourceID: "message:9", Blocks: []PackBlock{block}}},
	}
}
