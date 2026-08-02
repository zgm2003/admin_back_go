package aichat

import (
	"testing"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/shared/enum"
)

func TestChatMessagesPreserveSystemHistoryCurrentAndAttachments(t *testing.T) {
	historyMeta := `{"attachments":[{"type":"file","object_key":"ai_chat_attachments/history.pdf","etag":"\"h1\"","mime_type":"application/pdf","name":"history.pdf","size":3}]}`
	currentMeta := `{"attachments":[{"type":"image","url":"https://example.test/current.png"}],"runtime_params":{"temperature":0}}`
	rows := []MessageHistory{
		{ID: 9, Role: enum.AIMessageRoleUser, Content: "", MetaJSON: &currentMeta},
		{ID: 2, Role: enum.AIMessageRoleAssistant, Content: "answer"},
		{ID: 1, Role: enum.AIMessageRoleUser, Content: "question", MetaJSON: &historyMeta},
	}
	selected := selectedChatContext(rows, 9, 0)
	messages, err := chatMessages(AgentEngineConfig{SystemPrompt: "  system  "}, selected, 9, "context\n\nquestion")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 4 || messages[0].Role != infraai.MessageRoleSystem || messages[0].Parts[0].Text != "system" ||
		messages[1].Role != infraai.MessageRoleUser || messages[2].Role != infraai.MessageRoleAssistant ||
		messages[3].Role != infraai.MessageRoleUser {
		t.Fatalf("messages = %#v", messages)
	}
	if len(messages[1].Parts) != 2 || messages[1].Parts[0].Text != "question" ||
		messages[1].Parts[1].Attachment == nil || messages[1].Parts[1].Attachment.Kind != infraai.AttachmentFile ||
		messages[1].Parts[1].Attachment.ObjectKey != "ai_chat_attachments/history.pdf" {
		t.Fatalf("historical user message = %#v", messages[1])
	}
	if len(messages[3].Parts) != 2 || messages[3].Parts[0].Text != "context\n\nquestion" ||
		messages[3].Parts[1].Attachment == nil || messages[3].Parts[1].Attachment.Kind != infraai.AttachmentImage ||
		messages[3].Parts[1].Attachment.URL != "https://example.test/current.png" {
		t.Fatalf("current user message = %#v", messages[3])
	}
	temperature := chatTemperature(rows, 9)
	if temperature == nil || *temperature != 0 {
		t.Fatalf("temperature = %#v", temperature)
	}
}

func TestChatMessagesRejectInvalidPersistedAttachmentFacts(t *testing.T) {
	meta := `{"attachments":[{"type":"file","object_key":"ai_chat_attachments/a.txt","mime_type":"text/plain","name":"a.txt","size":3}]}`
	rows := []MessageHistory{{ID: 9, Role: enum.AIMessageRoleUser, Content: "read", MetaJSON: &meta}}
	if _, err := chatMessages(AgentEngineConfig{}, rows, 9, "read"); err == nil {
		t.Fatal("invalid persisted file facts were accepted")
	}
}
