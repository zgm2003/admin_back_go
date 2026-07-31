package aichat

import (
	"bytes"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
)

func TestFinalChatAnswerFromCandidateAcceptsOnlyFinalVersionedAnswer(t *testing.T) {
	answer, err := FinalChatAnswerFromCandidate(`{"version":"ai_chat_result_v1","answer":"  hello  "}`)
	if err != nil || answer != "hello" {
		t.Fatalf("answer=%q err=%v", answer, err)
	}

	for _, raw := range []string{
		`{"version":"unknown","answer":"hello"}`,
		`{"version":"ai_chat_result_v1","answer":"hello","tool_calls":[{"id":"call-1","name":"lookup"}]}`,
		`{"version":"ai_chat_result_v1","answer":""}`,
		`{"version":"ai_chat_result_v1","answer":"hello","extra":true}`,
	} {
		if answer, err := FinalChatAnswerFromCandidate(raw); err == nil || answer != "" {
			t.Fatalf("invalid candidate accepted: raw=%s answer=%q err=%v", raw, answer, err)
		}
	}
}

func TestChatResultCandidatePreservesResponsesContinuation(t *testing.T) {
	want := &infraai.ChatContinuation{
		Protocol: infraai.APIProtocolResponses,
		Items:    []byte(`[{"id":"rs_1","type":"reasoning","encrypted_content":"opaque"},{"id":"fc_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"}]`),
	}
	raw, err := MarshalChatResultCandidate(&infraai.ChatResult{
		ToolCalls:    []infraai.ToolCall{{ID: "call_1", Name: "lookup", Arguments: "{}"}},
		Continuation: want,
	})
	if err != nil || raw == nil {
		t.Fatalf("marshal candidate: raw=%v err=%v", raw, err)
	}
	restored, err := ChatResultFromCandidate(*raw)
	if err != nil {
		t.Fatalf("restore candidate: %v", err)
	}
	if restored.Continuation == nil || restored.Continuation.Protocol != want.Protocol ||
		!bytes.Equal(restored.Continuation.Items, want.Items) {
		t.Fatalf("restored continuation=%#v want=%#v", restored.Continuation, want)
	}
}
