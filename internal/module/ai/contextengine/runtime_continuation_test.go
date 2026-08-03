package contextengine

import (
	"errors"
	"strings"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
)

func TestToolContinuationCanonicalizesCompleteAtomicGroups(t *testing.T) {
	encoded, err := toolContinuationUpperBound(
		[]infraai.ToolCall{{ID: " call-1 ", Name: " lookup ", Arguments: `{"b":2,"a":1}`}},
		[]infraai.ToolOutput{{CallID: "call-1", Name: "lookup", Output: ` { "ok": true } `}},
		true,
	)
	if err != nil {
		t.Fatalf("toolContinuationUpperBound: %v", err)
	}
	if !strings.Contains(encoded, `"arguments":{"a":1,"b":2}`) || !strings.Contains(encoded, `"result":{"ok":true}`) {
		t.Fatalf("continuation is not canonical: %s", encoded)
	}
}

func TestToolContinuationRejectsIncompleteAtomicGroups(t *testing.T) {
	_, err := toolContinuationUpperBound(
		[]infraai.ToolCall{{ID: "call-1", Name: "lookup", Arguments: `{}`}},
		[]infraai.ToolOutput{{CallID: "other", Name: "lookup", Output: `{}`}},
		true,
	)
	if !errors.Is(err, ErrInvalidContextPlan) {
		t.Fatalf("err=%v, want ErrInvalidContextPlan", err)
	}
}
