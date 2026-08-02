package contextengine

import (
	"crypto/sha256"
	"math"
	"testing"
)

func TestContextProfileHashUsesOnlyImmutableConfiguration(t *testing.T) {
	rerankerID, memoryID := uint64(22), uint64(33)
	dense := mustFixedScore("0.2")
	rerank := mustFixedScore("0.3")
	input := ContextProfileHashInput{
		ID: 7, Name: "primary", Status: "enabled", IndexState: ProfileIndexReady,
		ActiveGeneration: uint64Pointer(4), TargetGeneration: nil, VerifiedUnixMS: 123,
		EmbeddingProviderModelID: 11, EmbeddingDimensions: 1536, EmbeddingMaxInputTokens: 8192,
		EmbeddingTokenCounterID: "utf8_bytes_v1", DenseDistance: DenseDistanceCosine,
		DenseMinScore: dense, SparseEncoder: "unicode_lexical_v1", SparseEncoderVersion: "1",
		RerankerProviderModelID: &rerankerID, RerankerMinScore: &rerank, MemoryProviderModelID: &memoryID,
	}
	got, err := HashContextProfile(input)
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256([]byte(`{"schema":"context_profile_v1","embedding_provider_model_id":11,"embedding_dimensions":1536,"embedding_max_input_tokens":8192,"embedding_token_counter_id":"utf8_bytes_v1","dense_distance":"cosine","dense_min_score":"0.200000","sparse_encoder":"unicode_lexical_v1","sparse_encoder_version":"1","reranker_provider_model_id":22,"reranker_min_score":"0.300000","memory_provider_model_id":33}`))
	if got != want {
		t.Fatalf("profile hash = %x, want %x", got, want)
	}

	changedRuntime := input
	changedRuntime.ID = 99
	changedRuntime.Name = "renamed"
	changedRuntime.Status = "retired"
	changedRuntime.ActiveGeneration = uint64Pointer(8)
	changedRuntime.IndexState = ProfileIndexFailed
	changedRuntime.VerifiedUnixMS = 999
	if hash, err := HashContextProfile(changedRuntime); err != nil || hash != got {
		t.Fatalf("runtime profile fields changed hash: %x, %v", hash, err)
	}
	changedConfig := input
	changedConfig.EmbeddingDimensions++
	if hash, err := HashContextProfile(changedConfig); err != nil || hash == got {
		t.Fatalf("immutable profile config did not change hash: %x, %v", hash, err)
	}
	invalidEncoder := input
	invalidEncoder.SparseEncoder = "unknown_encoder"
	if _, err := HashContextProfile(invalidEncoder); err == nil {
		t.Fatal("unknown sparse encoder was accepted")
	}
}

func TestModelCapabilityAndInputFingerprintHashesAreCanonical(t *testing.T) {
	modelInput := ModelCapabilityHashInput{
		ProviderID: 7, ProviderModelID: 8, RequestedModelID: "gpt-test", CanonicalModelID: "gpt-5.6",
		APIProtocol: APIProtocolResponses, ContextWindowTokens: 1000, MaxOutputTokens: 100,
		TokenCounterID: "utf8_bytes_v1", InputModalities: []string{"text", "file", "image"},
		SupportedParameters: []string{"temperature"}, SupportsTools: true, NativeFileInput: true, ImageInput: true,
	}
	modelHash, err := HashModelCapability(modelInput)
	if err != nil {
		t.Fatal(err)
	}
	reorderedModel := modelInput
	reorderedModel.InputModalities = []string{"image", "text", "file"}
	if hash, err := HashModelCapability(reorderedModel); err != nil || hash != modelHash {
		t.Fatalf("set ordering changed model capability hash: %x, %v", hash, err)
	}

	profileHash := testSHA256("profile")
	modelCapabilityHash := modelHash
	input := InputFingerprintHashInput{
		PolicyVersion: "context_policy_v1",
		AgentID:       5, AgentSHA256: testSHA256("agent"),
		ProviderID: 7, ProviderSHA256: testSHA256("provider"), ProviderModelID: 8,
		ModelID: "gpt-5.6", ModelCapabilitySHA256: modelCapabilityHash,
		Profile: &ProfileSnapshot{ID: 1, SHA256: profileHash, IndexGeneration: uint64Pointer(4)},
		Messages: []FingerprintMessage{{
			ID: 9, Role: "user", ContentSHA256: testSHA256("question"),
			Attachments: []FingerprintAttachment{{
				Ordinal: 1, Kind: AttachmentFile, ObjectKey: "ai_chat_attachments/a.txt", ETag: "etag-1",
				Size: 3, MIMEType: "text/plain", Filename: "a.txt",
			}, {
				Ordinal: 2, Kind: AttachmentFile, ObjectKey: "ai_chat_attachments/b.txt", ETag: "etag-2",
				Size: 4, MIMEType: "text/plain", Filename: "b.txt",
			}},
		}, {
			ID: 10, Role: "assistant", ContentSHA256: testSHA256("answer"),
		}},
		Bindings: []FingerprintBinding{
			{ID: 2, SpaceID: 20, SHA256: testSHA256("binding-2")},
			{ID: 1, SpaceID: 10, SHA256: testSHA256("binding-1")},
		},
		Tools: []FingerprintTool{
			{ID: 2, Name: "second", DefinitionSHA256: testSHA256("tool-2")},
			{ID: 1, Name: "first", DefinitionSHA256: testSHA256("tool-1")},
		},
		Generation: FingerprintGeneration{Temperature: fixedScorePointer("0")},
	}
	fingerprint, err := HashInputFingerprint(input)
	if err != nil {
		t.Fatal(err)
	}
	reorderedInput := input
	reorderedInput.Bindings = []FingerprintBinding{input.Bindings[1], input.Bindings[0]}
	reorderedInput.Tools = []FingerprintTool{input.Tools[1], input.Tools[0]}
	if hash, err := HashInputFingerprint(reorderedInput); err != nil || hash != fingerprint {
		t.Fatalf("set ordering changed input fingerprint: %x, %v", hash, err)
	}
	reorderedMessages := input
	reorderedMessages.Messages = []FingerprintMessage{input.Messages[1], input.Messages[0]}
	if hash, err := HashInputFingerprint(reorderedMessages); err != nil || hash == fingerprint {
		t.Fatalf("message ordering did not change input fingerprint: %x, %v", hash, err)
	}
	reorderedAttachments := input
	reorderedAttachments.Messages = append([]FingerprintMessage(nil), input.Messages...)
	firstAttachment := input.Messages[0].Attachments[1]
	firstAttachment.Ordinal = 1
	secondAttachment := input.Messages[0].Attachments[0]
	secondAttachment.Ordinal = 2
	reorderedAttachments.Messages[0].Attachments = []FingerprintAttachment{firstAttachment, secondAttachment}
	if hash, err := HashInputFingerprint(reorderedAttachments); err != nil || hash == fingerprint {
		t.Fatalf("attachment ordering did not change input fingerprint: %x, %v", hash, err)
	}
	changed := input
	changed.Messages = append([]FingerprintMessage(nil), input.Messages...)
	changed.Messages[0].ContentSHA256 = testSHA256("changed")
	if hash, err := HashInputFingerprint(changed); err != nil || hash == fingerprint {
		t.Fatalf("message content did not change fingerprint: %x, %v", hash, err)
	}
}

func TestModelCapabilityHashRejectsUnvalidatedEnums(t *testing.T) {
	valid := ModelCapabilityHashInput{
		ProviderID: 1, ProviderModelID: 2, RequestedModelID: "gpt-test", CanonicalModelID: "gpt-test",
		APIProtocol: APIProtocolResponses, ContextWindowTokens: 1000, MaxOutputTokens: 100,
		TokenCounterID: "utf8_bytes_v1", InputModalities: []string{"text"},
	}
	unknownModality := valid
	unknownModality.InputModalities = []string{"unknown"}
	if _, err := HashModelCapability(unknownModality); err == nil {
		t.Fatal("unknown modality was accepted")
	}
	unknownParameter := valid
	unknownParameter.SupportedParameters = []string{"unknown"}
	if _, err := HashModelCapability(unknownParameter); err == nil {
		t.Fatal("unknown supported parameter was accepted")
	}
}

func TestPlanHashIncludesDecisionsScoresAndSourceOrderButNotAuditRuntime(t *testing.T) {
	plan := validReadyPlan()
	plan.ID = 91
	plan.Items[0].Block.Kind = BlockDocumentEvidence
	plan.Items[0].CitationKey = stringPointer("C1")
	plan.Items[0].FusionScore = fixedScorePointer("0.7")
	base, err := HashPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	pending := plan
	pending.PlanSHA256 = nil
	if pendingHash, err := HashPlan(pending); err != nil || pendingHash != base {
		t.Fatalf("pending plan hash = %x, %v; want %x", pendingHash, err, base)
	}

	runtimeOnly := plan
	runtimeOnly.ID = 999
	runtimeOnly.Metrics.PackingMS = 999
	runtimeHash, err := HashPlan(runtimeOnly)
	if err != nil || runtimeHash != base {
		t.Fatalf("audit runtime changed plan hash: %x, %v", runtimeHash, err)
	}

	mutations := []struct {
		name   string
		mutate func(*ContextPlan)
	}{
		{name: "source", mutate: func(value *ContextPlan) { value.Items[0].Block.SourceRef = "message:2" }},
		{name: "priority", mutate: func(value *ContextPlan) { value.Items[0].Block.Priority++ }},
		{name: "score", mutate: func(value *ContextPlan) { value.Items[0].FusionScore = fixedScorePointer("0.8") }},
		{name: "additional cited evidence", mutate: func(value *ContextPlan) {
			second := value.Items[0]
			second.Ordinal = 2
			second.Block.SourceRef = "message:2"
			second.Block.SourceSHA256 = testSHA256("message:2")
			second.CitationKey = stringPointer("C2")
			value.Items = append(value.Items, second)
			value.Budget.KnownInputUpperBound += second.Block.TokenUpperBound
		}},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			changed := plan
			changed.Items = append([]ContextPlanItem(nil), plan.Items...)
			test.mutate(&changed)
			hash, err := HashPlan(changed)
			if err != nil || hash == base {
				t.Fatalf("mutation did not change plan hash: %x, %v", hash, err)
			}
		})
	}
}

func TestPlanHashIncludesTypedAttachmentFacts(t *testing.T) {
	plan := validReadyPlan()
	plan.Budget.Proof = BudgetOpaqueAttachment
	plan.Items[0].Block.Kind = BlockCurrentAttachment
	plan.Items[0].Block.ContentSnapshot = nil
	plan.Items[0].Block.Metadata.Attachment = &ContextAttachmentV1{
		Kind: AttachmentFile, ObjectKey: "ai_chat_attachments/a.txt", ETag: "etag-1",
		Size: 3, MIMEType: "text/plain", Filename: "a.txt",
	}
	base, err := HashPlan(plan)
	if err != nil {
		t.Fatal(err)
	}

	changed := plan
	changed.Items = append([]ContextPlanItem(nil), plan.Items...)
	metadata := *plan.Items[0].Block.Metadata.Attachment
	metadata.ETag = "etag-2"
	changed.Items[0].Block.Metadata.Attachment = &metadata
	if hash, err := HashPlan(changed); err != nil || hash == base {
		t.Fatalf("attachment facts did not change plan hash: %x, %v", hash, err)
	}
}

func TestInputFingerprintPreservesLegacyImageURLFacts(t *testing.T) {
	input := InputFingerprintHashInput{
		PolicyVersion:         "context_policy_v1",
		AgentID:               5,
		AgentSHA256:           testSHA256("agent"),
		ProviderID:            7,
		ProviderSHA256:        testSHA256("provider"),
		ProviderModelID:       8,
		ModelID:               "gpt-5.6",
		ModelCapabilitySHA256: testSHA256("capability"),
		Messages: []FingerprintMessage{{
			ID: 9, Role: "user", ContentSHA256: testSHA256("question"),
			Attachments: []FingerprintAttachment{{
				Ordinal: 1,
				Kind:    AttachmentImage,
				URL:     "https://example.test/history.png",
			}},
		}},
	}
	base, err := HashInputFingerprint(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Messages[0].Attachments[0].URL = "https://example.test/changed.png"
	if changed, err := HashInputFingerprint(input); err != nil || changed == base {
		t.Fatalf("image URL did not change fingerprint: %x, %v", changed, err)
	}
}

func TestCanonicalHashRejectsMapsAndFloats(t *testing.T) {
	if _, err := canonicalSHA256(map[string]any{"a": 1}); err == nil {
		t.Fatal("map was accepted by canonical hash")
	}
	if _, err := canonicalSHA256(struct{ Score float64 }{Score: math.NaN()}); err == nil {
		t.Fatal("float was accepted by canonical hash")
	}
	if _, err := canonicalSHA256(struct{ Score float32 }{}); err == nil {
		t.Fatal("float32 was accepted by canonical hash")
	}
}

func testSHA256(value string) [sha256.Size]byte { return sha256.Sum256([]byte(value)) }

func mustFixedScore(value string) FixedScore {
	score, err := ParseFixedScore(value)
	if err != nil {
		panic(err)
	}
	return score
}

func fixedScorePointer(value string) *FixedScore {
	score := mustFixedScore(value)
	return &score
}

func uint64Pointer(value uint64) *uint64 { return &value }
