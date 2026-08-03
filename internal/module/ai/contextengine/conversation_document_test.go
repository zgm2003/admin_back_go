package contextengine

import (
	"fmt"
	"testing"

	"admin_back_go/internal/infra/documentparser"
)

func TestConversationDocumentAttachmentIndexKeepsSameNameFilesDistinct(t *testing.T) {
	attachments := []conversationAttachment{
		{Type: "file", ObjectKey: "ai_chat_attachments/a/report.pdf", MIMEType: "application/pdf", Name: "report.pdf", Size: 10, ETag: "a"},
		{Type: "file", ObjectKey: "ai_chat_attachments/b/report.pdf", MIMEType: "application/pdf", Name: "report.pdf", Size: 11, ETag: "b"},
	}
	registry := documentparser.NewRegistry()
	identities := make(map[string]struct{}, len(attachments))
	for index, attachment := range attachments {
		if _, supported := supportedConversationAttachment(registry, attachment); !supported {
			t.Fatalf("attachment %d was rejected", index)
		}
		identities[fmt.Sprintf("conversation:11/message:13/attachment:%d", index)] = struct{}{}
	}
	if len(identities) != 2 {
		t.Fatalf("attachment identities collapsed: %v", identities)
	}
}

func TestConversationDocumentRejectsImagesAndUntrustedObjectKeys(t *testing.T) {
	registry := documentparser.NewRegistry()
	tests := []conversationAttachment{
		{Type: "image", ObjectKey: "ai_chat_images/a.png", MIMEType: "image/png", Name: "a.png", Size: 10, ETag: "a"},
		{Type: "file", ObjectKey: "ai_context_documents/report.pdf", MIMEType: "application/pdf", Name: "report.pdf", Size: 10, ETag: "a"},
	}
	for _, attachment := range tests {
		if _, supported := supportedConversationAttachment(registry, attachment); supported {
			t.Fatalf("unsupported attachment accepted: %+v", attachment)
		}
	}
}
