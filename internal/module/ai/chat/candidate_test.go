package aichat

import "testing"

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
