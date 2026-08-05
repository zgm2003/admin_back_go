package contextengine

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
	"gorm.io/gorm"
)

type runtimeFactsFixture struct{ facts RuntimeFacts }

func (fixture runtimeFactsFixture) LoadRuntimeFacts(context.Context, RuntimeInput) (RuntimeFacts, error) {
	return fixture.facts, nil
}

type runtimeEvidenceFixture struct{ evidence RuntimeEvidence }

func (fixture runtimeEvidenceFixture) ResolveRuntimeEvidence(context.Context, RuntimeInput, RuntimeFacts) (RuntimeEvidence, error) {
	return fixture.evidence, nil
}

type memoryContextFixture struct {
	memory *MemoryRecord
	err    error
}

func (fixture memoryContextFixture) LatestReadyMemory(context.Context, uint64, uint64, [sha256.Size]byte) (*MemoryRecord, error) {
	return fixture.memory, fixture.err
}

func TestContextPrecedenceComposesMemoryDirectAndRecalledTurnsOnce(t *testing.T) {
	newest := testRuntimeTurn(t, 8, true)
	direct := testRuntimeTurn(t, 7, false)
	covered := testRuntimeTurn(t, 3, false)
	summary := "user claims: stable preference"
	profileHash, memorySource, summaryHash := testSHA256("profile"), testSHA256("memory-source"), testSHA256(summary)
	memory := &MemoryRecord{ID: 9, ConversationID: 3, ProfileID: 7, ProfileSHA256: profileHash[:],
		FromMessageID: 1, ThroughMessageID: 4, SourceSHA256: memorySource[:], SummarySHA256: summaryHash[:],
		Summary: &summary, State: MemoryStateReady}
	facts := validBuildPlanInput()
	profile := &ProfileSnapshot{ID: 7, SHA256: testSHA256("profile")}
	materializer := NewPlanMaterializer(runtimeFactsFixture{facts: RuntimeFacts{
		Fingerprint: facts.Fingerprint, ModelCapability: facts.ModelCapability, Budget: facts.Budget, Profile: profile,
		CoreGroups: facts.PackGroups, Retrieval: &RuntimeRetrievalFacts{HasSources: true},
	}}, runtimeEvidenceFixture{evidence: RuntimeEvidence{Outcome: RetrievalHit, Groups: []PackGroup{
		testEvidenceGroup(BlockDocumentEvidence, "document_chunks:41", testSHA256("document"), 8),
		testEvidenceGroup(BlockRecalledTurn, "conversation_turn:7", direct.SourceSHA256, 8),
		testEvidenceGroup(BlockRecalledTurn, "conversation_turn:3", covered.SourceSHA256, 8),
	}}}, &historyPagerFixture{turns: []ConversationTurn{newest, direct, covered}}).
		WithMemoryReader(memoryContextFixture{memory: memory})

	input := RuntimeInput{RunID: facts.RunID, ReplyCommandID: facts.ReplyCommandID, LeaseOwner: facts.LeaseOwner, LeaseToken: facts.LeaseToken,
		CurrentMessageID: facts.CurrentMessageID, AgentID: facts.AgentID, UserID: facts.UserID, ConversationID: facts.ConversationID,
		ProviderID: facts.ProviderID, ModelID: facts.ModelID, APIProtocol: facts.APIProtocol}
	materialized, err := materializer.Materialize(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	packed, appErr := Pack(PackInput{KnownInputBudget: materialized.Budget.KnownInputBudget, Candidates: materialized.PackGroups})
	if appErr != nil {
		t.Fatal(appErr)
	}
	wantSelected := []BlockKind{BlockCurrentUserMessage, BlockRecentTurn, BlockHistoryAttachment, BlockDocumentEvidence, BlockConversationMemory, BlockRecentTurn}
	selected := make([]BlockKind, 0, len(wantSelected))
	exclusions := map[string]ExclusionReason{}
	for _, item := range packed.Items {
		if item.Decision == DecisionSelected {
			selected = append(selected, item.Block.Kind)
			continue
		}
		if item.ExclusionReason != nil {
			exclusions[item.Block.SourceRef] = *item.ExclusionReason
		}
	}
	if len(selected) != len(wantSelected) {
		t.Fatalf("selected=%v items=%+v", selected, packed.Items)
	}
	for index := range wantSelected {
		if selected[index] != wantSelected[index] {
			t.Fatalf("selected=%v want=%v", selected, wantSelected)
		}
	}
	if exclusions["conversation_turn:7"] != ExclusionDuplicateContent || exclusions["conversation_turn:3"] != ExclusionSupersededMemory {
		t.Fatalf("exclusions=%v", exclusions)
	}
}

func TestMemoryDiagnosticDoesNotFailCurrentPlanMaterialization(t *testing.T) {
	input := validBuildPlanInput()
	profile := &ProfileSnapshot{ID: 7, SHA256: testSHA256("profile")}
	materializer := NewPlanMaterializer(runtimeFactsFixture{facts: RuntimeFacts{
		Fingerprint: input.Fingerprint, ModelCapability: input.ModelCapability, Budget: input.Budget, Profile: profile,
		CoreGroups: input.PackGroups, Retrieval: &RuntimeRetrievalFacts{HasSources: false},
	}}, runtimeEvidenceFixture{evidence: RuntimeEvidence{Outcome: RetrievalSkipped}}, &historyPagerFixture{}).
		WithMemoryReader(memoryContextFixture{err: errors.New("failed memory diagnostic")})
	_, err := materializer.Materialize(t.Context(), RuntimeInput{RunID: input.RunID, ReplyCommandID: input.ReplyCommandID,
		LeaseOwner: input.LeaseOwner, LeaseToken: input.LeaseToken, CurrentMessageID: input.CurrentMessageID, AgentID: input.AgentID,
		UserID: input.UserID, ConversationID: input.ConversationID, ProviderID: input.ProviderID, ModelID: input.ModelID, APIProtocol: input.APIProtocol})
	if err != nil {
		t.Fatalf("memory diagnostic failed current plan: %v", err)
	}
}

func testRuntimeTurn(t *testing.T, anchor uint64, image bool) ConversationTurn {
	t.Helper()
	turn := ConversationTurn{ConversationID: 3, UserID: 7, AgentID: 5,
		UserMessage:      TurnMessage{ID: anchor, Role: "user", Content: "question"},
		AssistantMessage: TurnMessage{ID: anchor + 100, Role: "assistant", Content: "answer"}, AssistantDelivery: "completed"}
	if image {
		turn.UserMessage.Attachments = []TurnAttachment{{Index: 0, Type: "image", URL: "https://example.test/image.png", StorageProvider: "cos", ObjectKey: "image.png", ETag: "v1", Size: 10, MIMEType: "image/png", Name: "image.png"}}
	}
	if err := turn.ComputeSourceSHA256(); err != nil {
		t.Fatal(err)
	}
	return turn
}

func testEvidenceGroup(kind BlockKind, ref string, source [sha256.Size]byte, priority int32) PackGroup {
	content := "evidence"
	metadata := emptyBlockMetadata()
	if kind == BlockDocumentEvidence {
		paragraph := uint32(1)
		metadata.Document = &ContextDocumentEvidenceV1{Title: "document", DocumentID: 1, DocumentVersionID: 2, ChunkIDs: []uint64{41},
			Locators: []ContextLocatorV1{{Schema: ContextLocatorSchemaV1, Kind: "paragraph", Paragraph: &paragraph}}}
	}
	return PackGroup{Priority: priority, StableSourceID: ref, Blocks: []PackBlock{{Block: ContextBlock{Kind: kind, SourceType: map[bool]string{true: "document_chunk", false: "conversation_turn"}[kind == BlockDocumentEvidence],
		SourceRef: ref, SourceSHA256: source, AtomicGroupKey: ref, Priority: priority, TokenUpperBound: 8, ContentSnapshot: &content, Metadata: metadata}}}}
}

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

func TestCompileChatInputKeepsLegacyConversationTurnPlansReadable(t *testing.T) {
	plan := validReadyPlan()
	legacyTurn := "User: previous question\nAssistant[delivery=completed]: previous answer\n"
	plan.Items = append(plan.Items,
		ContextPlanItem{
			Ordinal: 2,
			Block: ContextBlock{
				Kind: BlockRecentTurn, SourceType: "conversation_turn", SourceRef: "conversation_turn:8",
				SourceSHA256: testSHA256("conversation_turn:8"), AtomicGroupKey: "conversation_turn:8",
				Priority: 4, TokenUpperBound: 20, ContentSnapshot: &legacyTurn, Metadata: emptyBlockMetadata(),
			},
			Decision: DecisionSelected,
		},
		ContextPlanItem{
			Ordinal: 3,
			Block: ContextBlock{
				Kind: BlockHistoryAttachment, SourceType: "attachment", SourceRef: "message:8/attachment:0",
				SourceSHA256: testSHA256("message:8/attachment:0"), AtomicGroupKey: "conversation_turn:8",
				Priority: 4, Metadata: ContextBlockMetadataV1{
					Schema: ContextBlockMetadataSchemaV1,
					Attachment: &ContextAttachmentV1{
						Kind: AttachmentImage, URL: "https://example.test/legacy.png", Size: 10,
						MIMEType: "image/png", Filename: "legacy.png",
					},
				},
			},
			Decision: DecisionSelected,
		},
	)
	plan.Budget.KnownInputUpperBound += 20
	plan.Budget.Proof = BudgetOpaqueAttachment
	hash, err := HashPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.PlanSHA256 = &hash

	compiled, err := CompileChatInput(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Messages) != 3 || compiled.Messages[0].Role != infraai.MessageRoleUser ||
		compiled.Messages[1].Role != infraai.MessageRoleSystem || compiled.Messages[1].Parts[0].Text != legacyTurn ||
		compiled.Messages[2].Role != infraai.MessageRoleUser || len(compiled.Messages[2].Parts) != 1 ||
		compiled.Messages[2].Parts[0].Attachment == nil || compiled.Messages[2].Parts[0].Attachment.Filename != "legacy.png" {
		t.Fatalf("compiled legacy plan=%+v", compiled.Messages)
	}
}

func TestCompileChatInputPreservesHistoricalTurnRolesBeforeCurrentUser(t *testing.T) {
	older := ConversationTurn{
		ConversationID: 3,
		UserID:         7,
		AgentID:        5,
		UserMessage:    TurnMessage{ID: 4, Role: "user", Content: "older question"},
		AssistantMessage: TurnMessage{
			ID: 104, Role: "assistant", Content: "older answer",
		},
		AssistantDelivery: "completed",
	}
	newer := ConversationTurn{
		ConversationID: 3,
		UserID:         7,
		AgentID:        5,
		UserMessage: TurnMessage{ID: 8, Role: "user", Content: "previous upload", Attachments: []TurnAttachment{{
			Index: 0, Type: "image", URL: "https://example.test/history.png", StorageProvider: "cos",
			ObjectKey: "ai_chat_attachments/history.png", ETag: "v1", Size: 10, MIMEType: "image/png", Name: "history.png",
		}}},
		ToolGroups:        []ToolGroup{{CallID: "call-1", Name: "lookup", Arguments: `{"id":1}`, Result: `{"ok":true}`}},
		AssistantMessage:  TurnMessage{ID: 108, Role: "assistant", Content: "previous answer"},
		AssistantDelivery: "completed",
	}
	for _, turn := range []*ConversationTurn{&older, &newer} {
		if err := turn.ComputeSourceSHA256(); err != nil {
			t.Fatal(err)
		}
	}

	input := validBuildPlanInput()
	currentAttachment := ContextAttachmentV1{
		Kind: AttachmentImage, URL: "https://example.test/current.png", ObjectKey: "ai_chat_attachments/current.png",
		ETag: "v2", Size: 11, MIMEType: "image/png", Filename: "current.png",
	}
	currentMetadata := emptyBlockMetadata()
	currentMetadata.Attachment = &currentAttachment
	currentGroupKey := input.PackGroups[0].Blocks[0].Block.AtomicGroupKey
	input.PackGroups[0].Blocks = append(input.PackGroups[0].Blocks, PackBlock{Block: ContextBlock{
		Kind: BlockCurrentAttachment, SourceType: "attachment", SourceRef: "message:9/attachment:0",
		SourceSHA256: testSHA256("message:9/attachment:0"), AtomicGroupKey: currentGroupKey,
		Required: true, Priority: 1, Metadata: currentMetadata,
	}})
	input.Fingerprint.Messages[0].Attachments = []FingerprintAttachment{{
		Ordinal: 0, Kind: currentAttachment.Kind, URL: currentAttachment.URL, ObjectKey: currentAttachment.ObjectKey,
		ETag: currentAttachment.ETag, Size: currentAttachment.Size, MIMEType: currentAttachment.MIMEType,
		Filename: currentAttachment.Filename,
	}}
	input.ModelCapability.InputModalities = []string{"text", "image"}
	input.Budget.Proof = BudgetOpaqueAttachment
	history, err := runtimeHistoryGroups(t.Context(), &historyPagerFixture{turns: []ConversationTurn{newer, older}}, nil, RuntimeInput{
		CurrentMessageID: input.CurrentMessageID,
		ConversationID:   input.ConversationID,
		UserID:           input.UserID,
	}, RuntimeFacts{
		ModelCapability: input.ModelCapability,
		Budget:          input.Budget,
		CoreGroups:      input.PackGroups,
	})
	if err != nil {
		t.Fatal(err)
	}
	input.PackGroups = append(input.PackGroups, history...)

	repository := &fakePlannerRepository{}
	plan, err := NewPlanner(PlannerDependencies{
		Repository: repository,
		GuardFactory: fixedGuardFactory{
			hash: testSHA256("authority"),
		},
	}).BuildPlan(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := CompileChatInput(plan)
	if err != nil {
		t.Fatal(err)
	}

	if len(compiled.Messages) != 5 {
		t.Fatalf("compiled messages=%+v", compiled.Messages)
	}
	wantRoles := []infraai.MessageRole{
		infraai.MessageRoleUser,
		infraai.MessageRoleAssistant,
		infraai.MessageRoleUser,
		infraai.MessageRoleAssistant,
		infraai.MessageRoleUser,
	}
	wantText := []string{"older question", "older answer", "", "", "hello"}
	for index := range wantRoles {
		message := compiled.Messages[index]
		if message.Role != wantRoles[index] || len(message.Parts) == 0 || message.Parts[0].Kind != infraai.ContentPartText ||
			(wantText[index] != "" && message.Parts[0].Text != wantText[index]) {
			t.Fatalf("message[%d]=%+v", index, message)
		}
	}
	if assistant := compiled.Messages[3].Parts[0].Text; !strings.Contains(assistant, "Tool[0] Call: id=call-1 name=lookup") ||
		!strings.Contains(assistant, "Tool[0] Result: id=call-1 result=") || !strings.HasSuffix(assistant, "previous answer") {
		t.Fatalf("historical assistant context=%q", assistant)
	}
	historyUser := compiled.Messages[2]
	if len(historyUser.Parts) != 2 || historyUser.Parts[1].Kind != infraai.ContentPartAttachment ||
		historyUser.Parts[1].Attachment == nil || historyUser.Parts[1].Attachment.Filename != "history.png" ||
		historyUser.Parts[0].Text != "previous upload" {
		t.Fatalf("historical user message=%+v", historyUser)
	}
	currentUser := compiled.Messages[len(compiled.Messages)-1]
	if currentUser.Role != infraai.MessageRoleUser || len(currentUser.Parts) != 2 ||
		currentUser.Parts[0].Kind != infraai.ContentPartText || currentUser.Parts[0].Text != "hello" ||
		currentUser.Parts[1].Kind != infraai.ContentPartAttachment || currentUser.Parts[1].Attachment == nil ||
		currentUser.Parts[1].Attachment.Filename != "current.png" {
		t.Fatalf("current user message=%+v", currentUser)
	}
}

func TestCompileChatInputKeepsOlderAttachmentOnlyTurnAsUserContext(t *testing.T) {
	turns := make([]ConversationTurn, 0, runtimeHistoryPageSize+1)
	for id := uint64(runtimeHistoryPageSize + 1); id > 0; id-- {
		turn := ConversationTurn{
			ConversationID: 3, UserID: 7, AgentID: 5,
			UserMessage:       TurnMessage{ID: id, Role: "user", Content: "question"},
			AssistantMessage:  TurnMessage{ID: id + 100, Role: "assistant", Content: "answer"},
			AssistantDelivery: "completed",
		}
		if id == 1 {
			turn.UserMessage.Content = ""
			turn.UserMessage.Attachments = []TurnAttachment{{
				Index: 0, Type: "file", StorageProvider: "cos", ObjectKey: "ai_chat_attachments/report.pdf",
				ETag: "v1", Size: 10, MIMEType: "application/pdf", Name: "report.pdf",
			}}
		}
		if err := turn.ComputeSourceSHA256(); err != nil {
			t.Fatal(err)
		}
		turns = append(turns, turn)
	}

	input := validBuildPlanInput()
	input.CurrentMessageID = 100
	input.Fingerprint.Messages[0].ID = 100
	input.PackGroups[0].SourceOrder = 100
	input.PackGroups[0].StableSourceID = "message:100"
	input.PackGroups[0].Blocks[0].Block.SourceRef = "message:100"
	input.PackGroups[0].Blocks[0].Block.SourceSHA256 = testSHA256("message:100")
	input.PackGroups[0].Blocks[0].Block.AtomicGroupKey = "message:100"
	input.ModelCapability.ContextWindowTokens = 100000
	input.Budget.ContextWindowTokens = 100000
	input.Budget.KnownInputBudget = 99800
	history, err := runtimeHistoryGroups(t.Context(), &historyPagerFixture{turns: turns}, historyAttachmentAvailabilityStub{ready: true}, RuntimeInput{
		CurrentMessageID: input.CurrentMessageID, ConversationID: input.ConversationID, UserID: input.UserID,
	}, RuntimeFacts{ModelCapability: input.ModelCapability, Budget: input.Budget, CoreGroups: input.PackGroups})
	if err != nil {
		t.Fatal(err)
	}
	input.PackGroups = append(input.PackGroups, history...)
	plan, err := NewPlanner(PlannerDependencies{
		Repository: &fakePlannerRepository{}, GuardFactory: fixedGuardFactory{hash: testSHA256("authority")},
	}).BuildPlan(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := CompileChatInput(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Messages) == 0 || compiled.Messages[0].Role != infraai.MessageRoleUser ||
		len(compiled.Messages[0].Parts) != 1 || compiled.Messages[0].Parts[0].Kind != infraai.ContentPartText ||
		!strings.Contains(compiled.Messages[0].Parts[0].Text, "Attachment[0]") ||
		!strings.Contains(compiled.Messages[0].Parts[0].Text, "report.pdf") {
		t.Fatalf("oldest attachment-only turn=%+v", compiled.Messages)
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
