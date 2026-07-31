package requestidentity

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestBuildFingerprintNormalizesLineEndingsOptionsAndAttachmentOrder(t *testing.T) {
	firstInput := Input{
		UserID:         42,
		Operation:      " generate ",
		Modality:       " text ",
		AgentID:        7,
		ModelID:        " gpt-4o ",
		NormalizedText: "hello\r\nworld",
		Attachments: []AttachmentIdentity{
			{StorageProvider: " s3 ", StorageKey: " users/42/a.txt ", SHA256: strings.Repeat("c", 64)},
			{StorageProvider: " cos ", StorageKey: " users/42/z.txt ", SHA256: strings.Repeat("B", 64)},
			{StorageProvider: "cos", StorageKey: "users/42/m.txt", SHA256: strings.Repeat("a", 64)},
			{StorageProvider: "cos", StorageKey: "users/42/a.txt", SHA256: strings.Repeat("b", 64)},
		},
		Options: GenerationOptions{
			MaxOutputTokens: 100,
			Size:            " 1024x1024 ",
			DurationSeconds: 3,
			Count:           1,
			Extra:           map[string]string{" quality ": " high ", " seed ": " 7 "},
		},
	}
	secondInput := Input{
		UserID:         42,
		Operation:      "generate",
		Modality:       "text",
		AgentID:        7,
		ModelID:        "gpt-4o",
		NormalizedText: "hello\nworld",
		Attachments: []AttachmentIdentity{
			{StorageProvider: "cos", StorageKey: "users/42/a.txt", SHA256: strings.Repeat("b", 64)},
			{StorageProvider: "cos", StorageKey: "users/42/m.txt", SHA256: strings.Repeat("a", 64)},
			{StorageProvider: "cos", StorageKey: "users/42/z.txt", SHA256: strings.Repeat("b", 64)},
			{StorageProvider: "s3", StorageKey: "users/42/a.txt", SHA256: strings.Repeat("c", 64)},
		},
		Options: GenerationOptions{
			MaxOutputTokens: 100,
			Size:            "1024x1024",
			DurationSeconds: 3,
			Count:           1,
			Extra:           map[string]string{"quality": "high", "seed": "7"},
		},
	}

	first, err := Fingerprint(firstInput)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Fingerprint(secondInput)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("equivalent canonical inputs changed fingerprint: %x != %x", first, second)
	}
	carriageReturnInput := firstInput
	carriageReturnInput.NormalizedText = "hello\rworld"
	carriageReturnFingerprint, err := BuildFingerprint(carriageReturnInput)
	if err != nil {
		t.Fatal(err)
	}
	if carriageReturnFingerprint != second {
		t.Fatalf("bare carriage return was not normalized: %x != %x", carriageReturnFingerprint, second)
	}
	if firstInput.Attachments[0].StorageProvider != " s3 " || firstInput.Options.Extra[" quality "] != " high " {
		t.Fatal("fingerprint construction mutated caller-owned input")
	}
}

func TestBuildFingerprintPreservesAttachmentOrderWhenRequested(t *testing.T) {
	firstInput := validFingerprintInput()
	firstInput.PreserveAttachmentOrder = true
	firstInput.Attachments = []AttachmentIdentity{
		{StorageProvider: "cos", StorageKey: "objects/a", SHA256: strings.Repeat("a", 64)},
		{StorageProvider: "cos", StorageKey: "objects/b", SHA256: strings.Repeat("b", 64)},
	}
	secondInput := firstInput
	secondInput.Attachments = []AttachmentIdentity{firstInput.Attachments[1], firstInput.Attachments[0]}

	first, err := BuildFingerprint(firstInput)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildFingerprint(secondInput)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("ordered attachment swap must change the fingerprint")
	}
}

func TestBuildFingerprintRejectsNonAdjacentDuplicateAttachmentsWhenOrderIsPreserved(t *testing.T) {
	input := validFingerprintInput()
	input.PreserveAttachmentOrder = true
	input.Attachments = []AttachmentIdentity{
		{StorageProvider: "cos", StorageKey: "objects/a", SHA256: strings.Repeat("a", 64)},
		{StorageProvider: "cos", StorageKey: "objects/b", SHA256: strings.Repeat("b", 64)},
		{StorageProvider: "cos", StorageKey: "objects/a", SHA256: strings.Repeat("a", 64)},
	}

	if _, err := BuildFingerprint(input); err == nil {
		t.Fatal("non-adjacent duplicate attachment identity was accepted")
	}
}

func TestAttachmentFactsSHA256IncludesEveryTrustedFact(t *testing.T) {
	base := AttachmentFacts{
		Type: "file", ObjectKey: "ai_chat_attachments/report.pdf", ETag: `"v1"`,
		Size: 4, MIMEType: "application/pdf", Name: "report.pdf",
	}
	want, err := AttachmentFactsSHA256(base)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*AttachmentFacts)
	}{
		{name: "type", mutate: func(value *AttachmentFacts) { value.Type = "image" }},
		{name: "object key", mutate: func(value *AttachmentFacts) { value.ObjectKey += ".bak" }},
		{name: "etag", mutate: func(value *AttachmentFacts) { value.ETag = `"v2"` }},
		{name: "size", mutate: func(value *AttachmentFacts) { value.Size++ }},
		{name: "mime", mutate: func(value *AttachmentFacts) { value.MIMEType = "text/plain" }},
		{name: "name", mutate: func(value *AttachmentFacts) { value.Name = "other.pdf" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := base
			test.mutate(&changed)
			got, err := AttachmentFactsSHA256(changed)
			if err != nil {
				t.Fatal(err)
			}
			if got == want {
				t.Fatal("trusted attachment fact change must change its digest")
			}
		})
	}
}

func TestBuildFingerprintPreservesLeadingAndTrailingTextWhitespace(t *testing.T) {
	base := validFingerprintInput()
	want, err := BuildFingerprint(base)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		text string
	}{
		{name: "leading ASCII space", text: " hello"},
		{name: "trailing ASCII space", text: "hello "},
		{name: "leading Unicode space", text: "\u2003hello"},
		{name: "trailing Unicode space", text: "hello\u00a0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := base
			input.NormalizedText = test.text
			got, err := BuildFingerprint(input)
			if err != nil {
				t.Fatal(err)
			}
			if got == want {
				t.Fatal("leading or trailing text whitespace did not change fingerprint")
			}
		})
	}
}

func TestBuildChatFingerprintIncludesConversationAndSourceMessage(t *testing.T) {
	input := Input{
		UserID:          42,
		Operation:       "reply",
		Modality:        "chat",
		AgentID:         7,
		ModelID:         "gpt-4o",
		NormalizedText:  "answer this",
		ConversationID:  19,
		SourceMessageID: 23,
	}

	first, err := BuildFingerprint(input)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := BuildFingerprint(input)
	if err != nil {
		t.Fatal(err)
	}
	if first != repeated {
		t.Fatalf("identical chat input is not deterministic: %x != %x", first, repeated)
	}

	changedConversation := input
	changedConversation.ConversationID++
	conversationFingerprint, err := BuildFingerprint(changedConversation)
	if err != nil {
		t.Fatal(err)
	}
	if first == conversationFingerprint {
		t.Fatal("conversation_id change must change fingerprint")
	}

	changedSource := input
	changedSource.SourceMessageID++
	sourceFingerprint, err := BuildFingerprint(changedSource)
	if err != nil {
		t.Fatal(err)
	}
	if first == sourceFingerprint {
		t.Fatal("source_message_id change must change fingerprint")
	}
}

func TestBuildChatFingerprintOwnsCanonicalChatOperation(t *testing.T) {
	input := ChatFingerprintInput{
		UserID: 42, ConversationID: 19, AgentID: 7, ModelID: "gpt-4o", Text: "hello\r\nworld",
		Attachments: []AttachmentIdentity{{StorageProvider: "url", StorageKey: "https://example.test/b.png"}, {StorageProvider: "url", StorageKey: "https://example.test/a.png"}},
		Options:     GenerationOptions{MaxOutputTokens: 4096, Extra: map[string]string{"temperature": "0.7"}},
	}
	got, err := BuildChatFingerprint(input)
	if err != nil {
		t.Fatal(err)
	}
	want, err := BuildFingerprint(Input{
		UserID: 42, Operation: "chat.reply", Modality: "chat", AgentID: 7, ModelID: "gpt-4o", NormalizedText: "hello\nworld", ConversationID: 19,
		Attachments: []AttachmentIdentity{{StorageProvider: "url", StorageKey: "https://example.test/a.png"}, {StorageProvider: "url", StorageKey: "https://example.test/b.png"}},
		Options:     GenerationOptions{MaxOutputTokens: 4096, Extra: map[string]string{"temperature": "0.7"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("chat fingerprint=%x want=%x", got, want)
	}
}

func TestBuildFingerprintRejectsBlankOperationModalityAndInvalidIDs(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Input)
	}{
		{name: "zero user", mutate: func(input *Input) { input.UserID = 0 }},
		{name: "negative user", mutate: func(input *Input) { input.UserID = -1 }},
		{name: "zero agent", mutate: func(input *Input) { input.AgentID = 0 }},
		{name: "negative agent", mutate: func(input *Input) { input.AgentID = -1 }},
		{name: "negative conversation", mutate: func(input *Input) { input.ConversationID = -1 }},
		{name: "negative source message", mutate: func(input *Input) { input.SourceMessageID = -1 }},
		{name: "blank operation", mutate: func(input *Input) { input.Operation = " \t " }},
		{name: "non ascii operation", mutate: func(input *Input) { input.Operation = "r\u00e9ply" }},
		{name: "unstable operation", mutate: func(input *Input) { input.Operation = "reply now" }},
		{name: "blank modality", mutate: func(input *Input) { input.Modality = "\n" }},
		{name: "non ascii modality", mutate: func(input *Input) { input.Modality = "\u56fe\u50cf" }},
		{name: "blank model", mutate: func(input *Input) { input.ModelID = " \u2003 " }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validFingerprintInput()
			test.mutate(&input)
			if _, err := BuildFingerprint(input); err == nil {
				t.Fatal("invalid request identity was accepted")
			}
			if _, err := Fingerprint(input); err == nil {
				t.Fatal("Fingerprint accepted an identity rejected by BuildFingerprint")
			}
		})
	}
}

func TestBuildFingerprintRejectsNegativeGenerationOptions(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*GenerationOptions)
	}{
		{name: "max output tokens", mutate: func(options *GenerationOptions) { options.MaxOutputTokens = -1 }},
		{name: "duration seconds", mutate: func(options *GenerationOptions) { options.DurationSeconds = -1 }},
		{name: "count", mutate: func(options *GenerationOptions) { options.Count = -1 }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validFingerprintInput()
			test.mutate(&input.Options)
			if _, err := BuildFingerprint(input); err == nil {
				t.Fatal("negative generation option was accepted")
			}
		})
	}
}

func TestBuildFingerprintRejectsInvalidUTF8SemanticStrings(t *testing.T) {
	invalid := string([]byte{'x', 0xff})
	tests := []struct {
		name   string
		mutate func(*Input)
	}{
		{name: "model", mutate: func(input *Input) { input.ModelID = invalid }},
		{name: "text", mutate: func(input *Input) { input.NormalizedText = invalid }},
		{name: "attachment provider", mutate: func(input *Input) {
			input.Attachments = []AttachmentIdentity{{StorageProvider: invalid, StorageKey: "objects/a"}}
		}},
		{name: "attachment key", mutate: func(input *Input) {
			input.Attachments = []AttachmentIdentity{{StorageProvider: "cos", StorageKey: invalid}}
		}},
		{name: "option size", mutate: func(input *Input) { input.Options.Size = invalid }},
		{name: "option key", mutate: func(input *Input) { input.Options.Extra = map[string]string{invalid: "value"} }},
		{name: "option value", mutate: func(input *Input) { input.Options.Extra = map[string]string{"key": invalid} }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validFingerprintInput()
			test.mutate(&input)
			if _, err := BuildFingerprint(input); err == nil {
				t.Fatal("invalid UTF-8 semantic string was accepted")
			}
		})
	}
}

func TestBuildFingerprintRejectsInvalidAttachmentIdentity(t *testing.T) {
	tests := []struct {
		name       string
		attachment AttachmentIdentity
	}{
		{name: "blank provider", attachment: AttachmentIdentity{StorageProvider: " \t", StorageKey: "objects/a"}},
		{name: "blank key", attachment: AttachmentIdentity{StorageProvider: "cos", StorageKey: " \u2003 "}},
		{name: "short sha", attachment: AttachmentIdentity{StorageProvider: "cos", StorageKey: "objects/a", SHA256: "abc"}},
		{name: "non hexadecimal sha", attachment: AttachmentIdentity{StorageProvider: "cos", StorageKey: "objects/a", SHA256: strings.Repeat("g", 64)}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validFingerprintInput()
			input.Attachments = []AttachmentIdentity{test.attachment}
			if _, err := BuildFingerprint(input); err == nil {
				t.Fatal("invalid attachment identity was accepted")
			}
		})
	}
}

func TestBuildFingerprintAcceptsAttachmentWithoutSHA(t *testing.T) {
	input := validFingerprintInput()
	input.Attachments = []AttachmentIdentity{{StorageProvider: "cos", StorageKey: "objects/a"}}
	if _, err := BuildFingerprint(input); err != nil {
		t.Fatalf("attachment without optional sha rejected: %v", err)
	}
}

func TestBuildFingerprintRejectsNormalizedDuplicateAttachmentsAndOptionKeys(t *testing.T) {
	t.Run("attachment tuple", func(t *testing.T) {
		input := validFingerprintInput()
		input.Attachments = []AttachmentIdentity{
			{StorageProvider: " cos ", StorageKey: " objects/a ", SHA256: strings.Repeat("A", 64)},
			{StorageProvider: "cos", StorageKey: "objects/a", SHA256: strings.Repeat("a", 64)},
		}
		if _, err := BuildFingerprint(input); err == nil {
			t.Fatal("duplicate normalized attachment tuple was accepted")
		}
	})

	t.Run("option key", func(t *testing.T) {
		input := validFingerprintInput()
		input.Options.Extra = map[string]string{" quality ": "high", "quality": "low"}
		if _, err := BuildFingerprint(input); err == nil {
			t.Fatal("duplicate normalized option key was accepted")
		}
	})

	t.Run("blank option key", func(t *testing.T) {
		input := validFingerprintInput()
		input.Options.Extra = map[string]string{" \u2003 ": "value"}
		if _, err := BuildFingerprint(input); err == nil {
			t.Fatal("blank normalized option key was accepted")
		}
	})

	t.Run("same storage identity with distinct sha", func(t *testing.T) {
		input := validFingerprintInput()
		input.Attachments = []AttachmentIdentity{
			{StorageProvider: "cos", StorageKey: "objects/a", SHA256: strings.Repeat("a", 64)},
			{StorageProvider: "cos", StorageKey: "objects/a", SHA256: strings.Repeat("b", 64)},
		}
		if _, err := BuildFingerprint(input); err == nil {
			t.Fatal("conflicting attachment object identity was accepted")
		}
	})
}

func TestBuildFingerprintUsesVersionedCanonicalPayload(t *testing.T) {
	if FingerprintVersion != "v1" {
		t.Fatalf("fingerprint version=%q", FingerprintVersion)
	}

	got, err := BuildFingerprint(validFingerprintInput())
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256([]byte(`{"fingerprint_version":"v1","user_id":42,"operation":"generate-v1","modality":"text","agent_id":7,"model_id":"gpt-4o","normalized_text":"hello","generation_options":{}}`))
	if got != want {
		t.Fatalf("fingerprint does not use the v1 canonical payload: got %x want %x", got, want)
	}
}

func TestInputCannotBeMarshaledAsCanonicalPayload(t *testing.T) {
	if _, err := json.Marshal(validFingerprintInput()); err == nil {
		t.Fatal("raw Input was serialized outside the canonical builder")
	}
}

func TestBuildFingerprintCanonicalizesNilAndEmptyCollections(t *testing.T) {
	nilCollections := validFingerprintInput()
	emptyCollections := validFingerprintInput()
	emptyCollections.Attachments = []AttachmentIdentity{}
	emptyCollections.Options.Extra = map[string]string{}

	first, err := BuildFingerprint(nilCollections)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildFingerprint(emptyCollections)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("nil and empty collections changed fingerprint: %x != %x", first, second)
	}
}

func TestBuildFingerprintChangesWhenSemanticFieldsChange(t *testing.T) {
	base := validFingerprintInput()
	base.ConversationID = 11
	base.SourceMessageID = 13
	base.NormalizedText = "hello world"
	base.Attachments = []AttachmentIdentity{{StorageProvider: "cos", StorageKey: "objects/a", SHA256: strings.Repeat("a", 64)}}
	base.Options = GenerationOptions{
		MaxOutputTokens: 100,
		Size:            "1024x1024",
		DurationSeconds: 2,
		Count:           1,
		Extra:           map[string]string{"quality": "high"},
	}
	want, err := BuildFingerprint(base)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*Input)
	}{
		{name: "user", mutate: func(input *Input) { input.UserID++ }},
		{name: "operation", mutate: func(input *Input) { input.Operation = "regenerate-v1" }},
		{name: "modality", mutate: func(input *Input) { input.Modality = "image" }},
		{name: "agent", mutate: func(input *Input) { input.AgentID++ }},
		{name: "model", mutate: func(input *Input) { input.ModelID = "gpt-4.1" }},
		{name: "internal text whitespace", mutate: func(input *Input) { input.NormalizedText = "hello  world" }},
		{name: "attachment provider", mutate: func(input *Input) { input.Attachments[0].StorageProvider = "s3" }},
		{name: "attachment key", mutate: func(input *Input) { input.Attachments[0].StorageKey = "objects/b" }},
		{name: "attachment sha", mutate: func(input *Input) { input.Attachments[0].SHA256 = strings.Repeat("b", 64) }},
		{name: "max output tokens", mutate: func(input *Input) { input.Options.MaxOutputTokens++ }},
		{name: "size", mutate: func(input *Input) { input.Options.Size = "512x512" }},
		{name: "duration", mutate: func(input *Input) { input.Options.DurationSeconds++ }},
		{name: "count", mutate: func(input *Input) { input.Options.Count++ }},
		{name: "extra value", mutate: func(input *Input) { input.Options.Extra["quality"] = "standard" }},
		{name: "conversation", mutate: func(input *Input) { input.ConversationID++ }},
		{name: "source message", mutate: func(input *Input) { input.SourceMessageID++ }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := cloneFingerprintInput(base)
			test.mutate(&input)
			got, err := BuildFingerprint(input)
			if err != nil {
				t.Fatal(err)
			}
			if got == want {
				t.Fatal("semantic field change did not change fingerprint")
			}
		})
	}
}

func cloneFingerprintInput(input Input) Input {
	cloned := input
	cloned.Attachments = append([]AttachmentIdentity(nil), input.Attachments...)
	cloned.Options.Extra = make(map[string]string, len(input.Options.Extra))
	for key, value := range input.Options.Extra {
		cloned.Options.Extra[key] = value
	}
	return cloned
}

func validFingerprintInput() Input {
	return Input{
		UserID:         42,
		Operation:      "generate-v1",
		Modality:       "text",
		AgentID:        7,
		ModelID:        "gpt-4o",
		NormalizedText: "hello",
	}
}

func TestFingerprintIsStableForIdenticalTypedInput(t *testing.T) {
	input := Input{
		UserID:         42,
		Operation:      "generate",
		Modality:       "image",
		AgentID:        7,
		ModelID:        "image-v1",
		NormalizedText: "draw a cat",
		Attachments:    []AttachmentIdentity{{StorageProvider: "cos", StorageKey: "users/42/cat.png", SHA256: strings.Repeat("a", 64)}},
		Options:        GenerationOptions{Size: "1024x1024", Count: 1, Extra: map[string]string{"quality": "high"}},
	}
	first, err := Fingerprint(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildFingerprint(input)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("identical input changed fingerprint: %x != %x", first, second)
	}
	if len(first) != 32 {
		t.Fatalf("fingerprint length=%d", len(first))
	}
}

func TestFingerprintChangesWithContentOrOptions(t *testing.T) {
	base := Input{UserID: 42, Operation: "generate", Modality: "text", AgentID: 7, ModelID: "m", NormalizedText: "hello", Options: GenerationOptions{MaxOutputTokens: 100}}
	first, err := Fingerprint(base)
	if err != nil {
		t.Fatal(err)
	}
	changedText := base
	changedText.NormalizedText = "hello!"
	second, _ := Fingerprint(changedText)
	if first == second {
		t.Fatal("content change must change fingerprint")
	}
	changedOptions := base
	changedOptions.Options.MaxOutputTokens = 101
	third, _ := Fingerprint(changedOptions)
	if first == third {
		t.Fatal("option change must change fingerprint")
	}
}

func TestCompareReplayRejectsDifferentFingerprint(t *testing.T) {
	want := [32]byte{1}
	if err := CompareForReplay(IdentityStatusReplayable, want, want); err != nil {
		t.Fatalf("equal fingerprint rejected: %v", err)
	}
	if err := CompareForReplay(IdentityStatusReplayable, want, [32]byte{2}); !errors.Is(err, ErrRequestIdentityConflict) {
		t.Fatalf("different fingerprint error=%v", err)
	}
}

func TestCompareReplayRejectsLegacyNonReplayableIdentity(t *testing.T) {
	legacyMarkerHash := [32]byte{1}
	if err := CompareForReplay(IdentityStatusLegacyNonReplayable, legacyMarkerHash, legacyMarkerHash); !errors.Is(err, ErrRequestIdentityNotReplayable) {
		t.Fatalf("legacy marker replay error=%v", err)
	}
}
