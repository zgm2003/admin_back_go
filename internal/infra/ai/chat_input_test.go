package ai

import (
	"math"
	"testing"
)

func TestChatInputValidatesTypedMessages(t *testing.T) {
	zero := 0.0
	input := ChatInput{
		ModelID: "gpt-test",
		Messages: []Message{
			{Role: MessageRoleSystem, Parts: []ContentPart{{Kind: ContentPartText, Text: "system"}}},
			{Role: MessageRoleUser, Parts: []ContentPart{{Kind: ContentPartText, Text: "question"}}},
			{Role: MessageRoleAssistant, Parts: []ContentPart{{Kind: ContentPartText, Text: "answer"}}},
			{Role: MessageRoleUser, Parts: []ContentPart{{Kind: ContentPartAttachment, Attachment: &AttachmentRef{
				Kind: AttachmentImage, URL: "https://example.test/current.png",
			}}}},
		},
		Temperature: &zero,
	}

	if err := input.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestChatInputAllowsAttachmentOnlyCurrentUserMessage(t *testing.T) {
	input := ChatInput{
		ModelID: "gpt-test",
		Messages: []Message{{Role: MessageRoleUser, Parts: []ContentPart{{
			Kind:       ContentPartAttachment,
			Attachment: &AttachmentRef{Kind: AttachmentImage, URL: "https://example.test/current.png"},
		}}}},
	}
	if err := input.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestChatInputRejectsInvalidTypedMessageStates(t *testing.T) {
	valid := func() ChatInput {
		return ChatInput{ModelID: "gpt-test", Messages: []Message{{
			Role:  MessageRoleUser,
			Parts: []ContentPart{{Kind: ContentPartText, Text: "hello"}},
		}}}
	}
	tests := []struct {
		name   string
		mutate func(*ChatInput)
	}{
		{name: "missing model", mutate: func(input *ChatInput) { input.ModelID = "" }},
		{name: "missing messages", mutate: func(input *ChatInput) { input.Messages = nil }},
		{name: "unknown role", mutate: func(input *ChatInput) { input.Messages[0].Role = MessageRole("tool") }},
		{name: "missing parts", mutate: func(input *ChatInput) { input.Messages[0].Parts = nil }},
		{name: "unknown part kind", mutate: func(input *ChatInput) { input.Messages[0].Parts[0].Kind = ContentPartKind("unknown") }},
		{name: "empty text", mutate: func(input *ChatInput) { input.Messages[0].Parts[0].Text = "  " }},
		{name: "ambiguous text part", mutate: func(input *ChatInput) {
			input.Messages[0].Parts[0].Attachment = &AttachmentRef{Kind: AttachmentImage, URL: "https://example.test/a.png"}
		}},
		{name: "attachment part with text", mutate: func(input *ChatInput) {
			input.Messages[0].Parts[0] = ContentPart{Kind: ContentPartAttachment, Text: "hello", Attachment: &AttachmentRef{
				Kind: AttachmentImage, URL: "https://example.test/a.png",
			}}
		}},
		{name: "missing attachment", mutate: func(input *ChatInput) {
			input.Messages[0].Parts[0] = ContentPart{Kind: ContentPartAttachment}
		}},
		{name: "system attachment", mutate: func(input *ChatInput) {
			input.Messages[0] = Message{Role: MessageRoleSystem, Parts: []ContentPart{{
				Kind: ContentPartAttachment, Attachment: &AttachmentRef{Kind: AttachmentImage, URL: "https://example.test/a.png"},
			}}}
		}},
		{name: "ambiguous system parts", mutate: func(input *ChatInput) {
			input.Messages = append([]Message{{Role: MessageRoleSystem, Parts: []ContentPart{
				{Kind: ContentPartText, Text: "one"}, {Kind: ContentPartText, Text: "two"},
			}}}, input.Messages...)
		}},
		{name: "non finite temperature", mutate: func(input *ChatInput) {
			value := math.Inf(1)
			input.Temperature = &value
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid()
			test.mutate(&input)
			if err := input.Validate(); err == nil {
				t.Fatal("invalid typed chat input was accepted")
			}
		})
	}
}

func TestAttachmentRefValidatesImageAndNativeFileFacts(t *testing.T) {
	tests := []struct {
		name    string
		ref     AttachmentRef
		wantErr bool
	}{
		{name: "image URL without MIME", ref: AttachmentRef{Kind: AttachmentImage, URL: "https://example.test/a.png"}},
		{name: "image MIME", ref: AttachmentRef{Kind: AttachmentImage, URL: "https://example.test/a.png", MIMEType: "image/png"}},
		{name: "image missing URL", ref: AttachmentRef{Kind: AttachmentImage}, wantErr: true},
		{name: "image non image MIME", ref: AttachmentRef{Kind: AttachmentImage, URL: "https://example.test/a.png", MIMEType: "text/plain"}, wantErr: true},
		{name: "native file", ref: AttachmentRef{
			Kind: AttachmentFile, ObjectKey: "ai_chat_attachments/a.txt", ETag: "etag-v1", Size: 3,
			MIMEType: "text/plain", Filename: "a.txt",
		}},
		{name: "native file missing ETag", ref: AttachmentRef{
			Kind: AttachmentFile, ObjectKey: "ai_chat_attachments/a.txt", Size: 3, MIMEType: "text/plain", Filename: "a.txt",
		}, wantErr: true},
		{name: "unknown attachment kind", ref: AttachmentRef{Kind: AttachmentKind("unknown")}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.ref.Validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr = %v", err, test.wantErr)
			}
		})
	}
}
