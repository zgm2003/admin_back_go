package openaicompat

import (
	"context"
	"encoding/json"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
)

func TestChatMessagesPreserveTypedRoleAndPartOrder(t *testing.T) {
	prepared, err := New(Config{APIProtocol: infraai.APIProtocolChatCompletions}).PrepareChat(context.Background(), infraai.ChatInput{
		ModelID: "gpt-test",
		Messages: []infraai.Message{
			{Role: infraai.MessageRoleSystem, Parts: []infraai.ContentPart{{Kind: infraai.ContentPartText, Text: "system"}}},
			{Role: infraai.MessageRoleUser, Parts: []infraai.ContentPart{
				{Kind: infraai.ContentPartText, Text: "before"},
				{Kind: infraai.ContentPartAttachment, Attachment: &infraai.AttachmentRef{Kind: infraai.AttachmentImage, URL: "https://example.test/a.png"}},
				{Kind: infraai.ContentPartText, Text: "after"},
			}},
			{Role: infraai.MessageRoleAssistant, Parts: []infraai.ContentPart{{Kind: infraai.ContentPartText, Text: "answer"}}},
			{Role: infraai.MessageRoleUser, Parts: []infraai.ContentPart{{
				Kind: infraai.ContentPartAttachment, Attachment: &infraai.AttachmentRef{Kind: infraai.AttachmentImage, URL: "https://example.test/b.png"},
			}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(prepared, &request); err != nil {
		t.Fatal(err)
	}
	if request.Model != "gpt-test" || len(request.Messages) != 4 {
		t.Fatalf("request = %#v", request)
	}
	if request.Messages[0].Role != "system" || request.Messages[0].Content != "system" ||
		request.Messages[2].Role != "assistant" || request.Messages[2].Content != "answer" {
		t.Fatalf("text messages = %#v", request.Messages)
	}
	parts, ok := request.Messages[1].Content.([]any)
	if !ok || len(parts) != 3 {
		t.Fatalf("mixed user content = %#v", request.Messages[1].Content)
	}
	assertChatContentPart(t, parts[0], "text", "before")
	assertChatContentPart(t, parts[1], "image_url", "https://example.test/a.png")
	assertChatContentPart(t, parts[2], "text", "after")
	lastParts, ok := request.Messages[3].Content.([]any)
	if !ok || len(lastParts) != 1 {
		t.Fatalf("attachment-only user content = %#v", request.Messages[3].Content)
	}
	assertChatContentPart(t, lastParts[0], "image_url", "https://example.test/b.png")
}

func TestResponsesChatMessagesPreserveTypedRoleAndPartOrder(t *testing.T) {
	prepared, err := New(Config{
		APIProtocol: infraai.APIProtocolResponses,
		FileOpener:  testPreparedFileOpener(),
	}).PrepareChat(context.Background(), infraai.ChatInput{
		ModelID: "gpt-test",
		Messages: []infraai.Message{
			{Role: infraai.MessageRoleSystem, Parts: []infraai.ContentPart{{Kind: infraai.ContentPartText, Text: "system"}}},
			{Role: infraai.MessageRoleUser, Parts: []infraai.ContentPart{
				{Kind: infraai.ContentPartText, Text: "before"},
				{Kind: infraai.ContentPartAttachment, Attachment: &infraai.AttachmentRef{Kind: infraai.AttachmentImage, URL: "https://example.test/a.png"}},
				{Kind: infraai.ContentPartAttachment, Attachment: &infraai.AttachmentRef{
					Kind: infraai.AttachmentFile, ObjectKey: "ai_chat_attachments/a.txt", ETag: `"etag-v1"`,
					Size: 3, MIMEType: "text/plain", Filename: "a.txt",
				}},
				{Kind: infraai.ContentPartText, Text: "after"},
			}},
			{Role: infraai.MessageRoleAssistant, Parts: []infraai.ContentPart{{Kind: infraai.ContentPartText, Text: "answer"}}},
			{Role: infraai.MessageRoleUser, Parts: []infraai.ContentPart{{Kind: infraai.ContentPartText, Text: "next"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := infraai.ParsePreparedChatFileManifest(prepared)
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Model        string `json:"model"`
		Instructions string `json:"instructions"`
		Input        []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"input"`
	}
	if err := json.Unmarshal(manifest.Request, &request); err != nil {
		t.Fatal(err)
	}
	if request.Model != "gpt-test" || request.Instructions != "system" || len(request.Input) != 3 {
		t.Fatalf("request = %#v", request)
	}
	if request.Input[0].Role != "user" || request.Input[1].Role != "assistant" || request.Input[2].Role != "user" {
		t.Fatalf("roles = %#v", request.Input)
	}
	var parts []map[string]any
	if err := json.Unmarshal(request.Input[0].Content, &parts); err != nil {
		t.Fatalf("decode user content: %v", err)
	}
	if len(parts) != 4 || parts[0]["type"] != "input_text" || parts[0]["text"] != "before" ||
		parts[1]["type"] != "input_image" || parts[1]["image_url"] != "https://example.test/a.png" ||
		parts[2]["type"] != "file_ref" || parts[2]["ref"] != "file-1" ||
		parts[3]["type"] != "input_text" || parts[3]["text"] != "after" {
		t.Fatalf("ordered Responses parts = %#v", parts)
	}
	if len(manifest.Files) != 1 || manifest.Files[0].Ref != "file-1" || manifest.Files[0].ObjectKey != "ai_chat_attachments/a.txt" {
		t.Fatalf("manifest files = %#v", manifest.Files)
	}
	var assistantContent string
	if err := json.Unmarshal(request.Input[1].Content, &assistantContent); err != nil {
		t.Fatalf("decode assistant content: %v", err)
	}
	if assistantContent != "answer" {
		t.Fatalf("assistant content = %q", assistantContent)
	}
	var nextUserParts []map[string]any
	if err := json.Unmarshal(request.Input[2].Content, &nextUserParts); err != nil {
		t.Fatalf("decode next user content: %v", err)
	}
	if len(nextUserParts) != 1 || nextUserParts[0]["type"] != "input_text" || nextUserParts[0]["text"] != "next" {
		t.Fatalf("next user content = %#v", nextUserParts)
	}
}

func assertChatContentPart(t *testing.T, raw any, wantType, wantValue string) {
	t.Helper()
	part, ok := raw.(map[string]any)
	if !ok || part["type"] != wantType {
		t.Fatalf("content part = %#v, want type %q", raw, wantType)
	}
	if wantType == "text" {
		if part["text"] != wantValue {
			t.Fatalf("text part = %#v, want %q", part, wantValue)
		}
		return
	}
	image, ok := part["image_url"].(map[string]any)
	if !ok || image["url"] != wantValue {
		t.Fatalf("image part = %#v, want %q", part, wantValue)
	}
}
