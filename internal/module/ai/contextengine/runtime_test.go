package contextengine

import (
	"context"
	"errors"
	"reflect"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
)

type runtimeMaterializerFixture struct {
	input BuildPlanInput
	err   error
	calls int
}

func (fixture *runtimeMaterializerFixture) Materialize(context.Context, RuntimeInput) (BuildPlanInput, error) {
	fixture.calls++
	return fixture.input, fixture.err
}

type runtimeEvidenceErrorFixture struct {
	evidence RuntimeEvidence
	err      error
	calls    int
}

func (fixture *runtimeEvidenceErrorFixture) ResolveRuntimeEvidence(context.Context, RuntimeInput, RuntimeFacts) (RuntimeEvidence, error) {
	fixture.calls++
	return fixture.evidence, fixture.err
}

func TestTerminalPlanReuseSkipsMaterialization(t *testing.T) {
	tests := map[string]ContextPlan{
		"ready hit":      runtimeHitPlan(),
		"ready degraded": degradedReadyPlan(t),
	}
	for name, plan := range tests {
		t.Run(name, func(t *testing.T) {
			plan.ID = 91
			rehashContextPlan(t, &plan)
			repository := &fakePlannerRepository{existing: &plan}
			materializer := &runtimeMaterializerFixture{err: errors.New("materializer must not run")}
			service := NewRuntimeService(materializer, NewPlanner(PlannerDependencies{
				Repository: repository, GuardFactory: fixedGuardFactory{hash: testSHA256("authority")},
			}))

			result, err := service.BuildPlan(t.Context(), RuntimeInput{RunID: plan.RunID})
			if err != nil {
				t.Fatal(err)
			}
			if materializer.calls != 0 || repository.persistCalls != 0 {
				t.Fatalf("materializer calls=%d persist calls=%d", materializer.calls, repository.persistCalls)
			}
			if result.Evidence.ID != plan.ID || result.Evidence.SHA256 != *plan.PlanSHA256 {
				t.Fatalf("evidence=%+v plan=%+v", result.Evidence, plan)
			}
		})
	}
}

func TestDegradedRuntimePersistsTypedEnhancementFailure(t *testing.T) {
	cause := errors.New("embedding provider unavailable")
	evidence := &runtimeEvidenceErrorFixture{evidence: RuntimeEvidence{Metrics: ContextPlanMetricsV1{Schema: ContextPlanMetricsSchemaV1}}, err: NewEnhancementFailure(EnhancementStageEmbedding, ErrCodeEmbeddingFailed, cause)}
	materializer, runtimeInput := runtimeFailureMaterializer(t, evidence)
	repository := &fakePlannerRepository{}
	service := NewRuntimeService(materializer, NewPlanner(PlannerDependencies{
		Repository: repository, GuardFactory: fixedGuardFactory{hash: testSHA256("authority")},
	}))

	result, err := service.BuildPlan(t.Context(), runtimeInput)
	if err != nil {
		t.Fatal(err)
	}
	plan := repository.persisted
	if plan.State != PlanReady || plan.RetrievalOutcome != RetrievalDegraded || plan.Error == nil ||
		plan.Error.Stage != string(EnhancementStageEmbedding) || plan.Error.Code != ErrCodeEmbeddingFailed || plan.PlanSHA256 == nil {
		t.Fatalf("plan=%+v", plan)
	}
	if result.Evidence.ID != plan.ID || result.Evidence.SHA256 != *plan.PlanSHA256 || len(result.ChatInput.Messages) == 0 {
		t.Fatalf("result=%+v", result)
	}
	if evidence.calls != 1 || repository.persistCalls != 1 {
		t.Fatalf("evidence calls=%d persist calls=%d", evidence.calls, repository.persistCalls)
	}
}

func TestDegradedRuntimeRejectsUnknownErrorWithoutPlan(t *testing.T) {
	cause := errors.New("mysql connection reset")
	evidence := &runtimeEvidenceErrorFixture{evidence: RuntimeEvidence{Metrics: ContextPlanMetricsV1{Schema: ContextPlanMetricsSchemaV1}}, err: cause}
	materializer, runtimeInput := runtimeFailureMaterializer(t, evidence)
	repository := &fakePlannerRepository{}
	service := NewRuntimeService(materializer, NewPlanner(PlannerDependencies{
		Repository: repository, GuardFactory: fixedGuardFactory{hash: testSHA256("authority")},
	}))

	result, err := service.BuildPlan(t.Context(), runtimeInput)
	if !errors.Is(err, cause) {
		t.Fatalf("unknown error was replaced: %v", err)
	}
	if !reflect.DeepEqual(result, RuntimeResult{}) || repository.persistCalls != 0 {
		t.Fatalf("result=%+v persist calls=%d", result, repository.persistCalls)
	}
}

func TestDegradedPackingKeepsCoreMemoryAndCurrentAttachmentOnly(t *testing.T) {
	input := validBuildPlanInput()
	memoryModelID := uint64(33)
	generation := uint64(1)
	profile := &ProfileSnapshot{ID: 7, SHA256: testSHA256("profile"), IndexGeneration: &generation}
	input.Fingerprint.Profile = cloneProfileSnapshot(profile)
	attachment := ContextAttachmentV1{
		Kind: AttachmentFile, ObjectKey: "ai_chat_attachments/report.pdf", ETag: "v1", Size: 12,
		MIMEType: "application/pdf", Filename: "report.pdf",
	}
	metadata := emptyBlockMetadata()
	metadata.Attachment = &attachment
	input.PackGroups[0].Blocks = append(input.PackGroups[0].Blocks, PackBlock{Block: ContextBlock{
		Kind: BlockCurrentAttachment, SourceType: "attachment", SourceRef: "message:9/attachment:0",
		SourceSHA256: testSHA256("current-attachment"), AtomicGroupKey: input.PackGroups[0].StableSourceID,
		Required: true, Priority: 1, Metadata: metadata,
	}})
	input.Fingerprint.Messages[0].Attachments = []FingerprintAttachment{{
		Ordinal: 0, Kind: attachment.Kind, ObjectKey: attachment.ObjectKey, ETag: attachment.ETag,
		Size: attachment.Size, MIMEType: attachment.MIMEType, Filename: attachment.Filename,
	}}
	input.ModelCapability.NativeFileInput = true
	input.Budget.Proof = BudgetOpaqueAttachment

	memory := validRuntimeMemoryRecord()
	history := testRuntimeTurn(t, 8, true)
	evidence := &runtimeEvidenceErrorFixture{
		evidence: RuntimeEvidence{Outcome: RetrievalHit, Groups: []PackGroup{
			testEvidenceGroup(BlockDocumentEvidence, "document_chunks:41", testSHA256("document"), 8),
		}, Metrics: ContextPlanMetricsV1{Schema: ContextPlanMetricsSchemaV1, QueryEmbeddingMS: 12, QueryEmbeddingRequestCount: 1}},
		err: NewEnhancementFailure(EnhancementStageRerank, ErrCodeRerankFailed, errors.New("rerank unavailable")),
	}
	materializer := NewPlanMaterializer(runtimeFactsFixture{facts: RuntimeFacts{
		Fingerprint: input.Fingerprint, ModelCapability: input.ModelCapability, Budget: input.Budget, Profile: profile,
		CoreGroups: input.PackGroups, Retrieval: &RuntimeRetrievalFacts{Profile: ContextProfile{
			ID: 7, Status: ProfileEnabled, IndexState: ProfileIndexReady, ActiveIndexGeneration: &generation,
			MemoryProviderModelID: &memoryModelID, EmbeddingTokenCounterID: infraai.TokenCounterUTF8BytesV1,
			EmbeddingMaxInputTokens: 4096, DenseMinScore: "0.100000",
		}, CurrentText: "query", HasSources: true},
	}}, evidence, &historyPagerFixture{turns: []ConversationTurn{history}}).WithMemoryReader(memoryContextFixture{memory: &memory})
	materialized, err := materializer.Materialize(t.Context(), runtimeInputFromBuild(input))
	if err != nil {
		t.Fatal(err)
	}
	repository := &fakePlannerRepository{}
	plan, err := NewPlanner(PlannerDependencies{Repository: repository, GuardFactory: fixedGuardFactory{hash: testSHA256("authority")}}).
		BuildPlan(t.Context(), materialized)
	if err != nil {
		t.Fatal(err)
	}

	wantKinds := map[BlockKind]bool{
		BlockCurrentUserMessage: true,
		BlockCurrentAttachment:  true,
		BlockConversationMemory: true,
		BlockSystemInstruction:  true,
	}
	seen := make(map[BlockKind]int)
	for _, item := range plan.Items {
		if item.Decision != DecisionSelected {
			continue
		}
		seen[item.Block.Kind]++
		if !wantKinds[item.Block.Kind] || item.CitationKey != nil {
			t.Fatalf("unexpected degraded item=%+v", item)
		}
	}
	for kind := range wantKinds {
		if seen[kind] != 1 {
			t.Fatalf("selected kinds=%v", seen)
		}
	}
	if plan.RetrievalOutcome != RetrievalDegraded || plan.Error == nil || plan.Error.Code != ErrCodeRerankFailed {
		t.Fatalf("plan=%+v", plan)
	}
	if plan.Metrics.QueryEmbeddingMS != 12 || plan.Metrics.QueryEmbeddingRequestCount != 1 {
		t.Fatalf("degraded plan lost completed stage metrics: %+v", plan.Metrics)
	}

	compiled, err := CompileChatInput(plan)
	if err != nil {
		t.Fatal(err)
	}
	var foundAttachment, foundInstruction bool
	for _, message := range compiled.Messages {
		for _, part := range message.Parts {
			if part.Attachment != nil && part.Attachment.Filename == attachment.Filename {
				foundAttachment = true
			}
			if part.Text == degradedContextInstruction {
				foundInstruction = true
			}
		}
	}
	if !foundAttachment || !foundInstruction {
		t.Fatalf("compiled=%+v", compiled.Messages)
	}
}

func TestProviderRetryReusesIdenticalPlanAndPreparedInput(t *testing.T) {
	input := validBuildPlanInput()
	materializer := &runtimeMaterializerFixture{input: input}
	repository := &fakePlannerRepository{}
	service := NewRuntimeService(materializer, NewPlanner(PlannerDependencies{
		Repository: repository, GuardFactory: fixedGuardFactory{hash: testSHA256("authority")},
	}))
	runtimeInput := runtimeInputFromBuild(input)

	first, err := service.BuildPlan(t.Context(), runtimeInput)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.BuildPlan(t.Context(), runtimeInput)
	if err != nil {
		t.Fatal(err)
	}
	if materializer.calls != 1 || repository.persistCalls != 1 {
		t.Fatalf("materializer calls=%d persist calls=%d", materializer.calls, repository.persistCalls)
	}
	if first.Evidence != second.Evidence || !reflect.DeepEqual(first.ChatInput, second.ChatInput) {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
}

func runtimeFailureMaterializer(t *testing.T, evidence RuntimeEvidenceResolver) (*PlanMaterializer, RuntimeInput) {
	t.Helper()
	input := validBuildPlanInput()
	profile := &ProfileSnapshot{ID: 7, SHA256: testSHA256("profile")}
	input.Fingerprint.Profile = cloneProfileSnapshot(profile)
	materializer := NewPlanMaterializer(runtimeFactsFixture{facts: RuntimeFacts{
		Fingerprint: input.Fingerprint, ModelCapability: input.ModelCapability, Budget: input.Budget, Profile: profile,
		CoreGroups: input.PackGroups, Retrieval: &RuntimeRetrievalFacts{Profile: ContextProfile{ID: 7}, HasSources: true},
	}}, evidence, &historyPagerFixture{})
	return materializer, runtimeInputFromBuild(input)
}

func runtimeInputFromBuild(input BuildPlanInput) RuntimeInput {
	return RuntimeInput{
		RunID: input.RunID, ReplyCommandID: input.ReplyCommandID, LeaseOwner: input.LeaseOwner, LeaseToken: input.LeaseToken,
		CurrentMessageID: input.CurrentMessageID, AgentID: input.AgentID, UserID: input.UserID, ConversationID: input.ConversationID,
		ProviderID: input.ProviderID, ModelID: input.ModelID, APIProtocol: input.APIProtocol,
	}
}

func rehashContextPlan(t *testing.T, plan *ContextPlan) {
	t.Helper()
	plan.PlanSHA256 = nil
	hash, err := HashPlan(*plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.PlanSHA256 = &hash
}

func runtimeHitPlan() ContextPlan {
	plan := validReadyPlan()
	plan.RetrievalOutcome = RetrievalHit
	plan.Items = append(plan.Items, citationDocumentItem(2, "C1", "Refund policy", 10, 11, 20, 1, "Refunds take three days.", 11))
	plan.Budget.KnownInputUpperBound += 11
	return plan
}

var _ RuntimeMaterializer = (*runtimeMaterializerFixture)(nil)
