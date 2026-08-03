package contextengine

import (
	"strings"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
)

func TestConversationTurnHashTracksFactsAndIgnoresUnmodeledURLs(t *testing.T) {
	turn := testConversationTurn()
	first, err := ConversationTurnSourceSHA256(turn)
	if err != nil {
		t.Fatal(err)
	}
	turn.AssistantDelivery = "stopped"
	second, err := ConversationTurnSourceSHA256(turn)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("delivery-state change did not change source hash")
	}
	turn = testConversationTurn()
	turn.UserMessage.Attachments[0].ObjectKey = "ai_chat_attachments/changed.pdf"
	third, err := ConversationTurnSourceSHA256(turn)
	if err != nil {
		t.Fatal(err)
	}
	if first == third {
		t.Fatal("attachment fact change did not change source hash")
	}
}

func TestTurnTextKeepsToolGroupAtomicAndExactBound(t *testing.T) {
	counter, err := infraai.ResolveTokenCounter(infraai.TokenCounterUTF8BytesV1)
	if err != nil {
		t.Fatal(err)
	}
	result, err := BuildConversationTurnText(testConversationTurn(), counter, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(result.Text, "Tool[0]") != 2 || !strings.Contains(result.Text, "Tool[0] Call:") || !strings.Contains(result.Text, "Tool[0] Result:") {
		t.Fatalf("tool group was split: %q", result.Text)
	}
	bound, err := counter.UpperBoundText(result.Text)
	if err != nil || bound != result.TokenUpperBound {
		t.Fatalf("bound=%d want=%d err=%v", result.TokenUpperBound, bound, err)
	}
}

func testConversationTurn() ConversationTurn {
	return ConversationTurn{
		ConversationID: 3, UserID: 7, AgentID: 5,
		UserMessage: TurnMessage{ID: 41, Role: "user", Content: "read this", Attachments: []TurnAttachment{{
			Index: 0, Type: "file", StorageProvider: "cos", ObjectKey: "ai_chat_attachments/report.pdf",
			ETag: "v1", Size: 12, MIMEType: "application/pdf", Name: "report.pdf",
		}}},
		ToolGroups:        []ToolGroup{{CallID: "call-1", Name: "lookup", Arguments: `{"id":1}`, Result: `{"ok":true}`}},
		AssistantMessage:  TurnMessage{ID: 42, Role: "assistant", Content: "done"},
		AssistantDelivery: "completed",
	}
}
