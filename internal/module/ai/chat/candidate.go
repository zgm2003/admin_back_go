package aichat

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	infraai "admin_back_go/internal/infra/ai"
)

const (
	chatResultCandidateVersion       = "ai_chat_result_v2"
	legacyChatResultCandidateVersion = "ai_chat_result_v1"
	maxChatContinuationBytes         = 1 << 20
)

// FinalChatAnswerFromCandidate validates the immutable paid-chat result before
// the settlement transaction publishes it as an assistant message.
func FinalChatAnswerFromCandidate(raw string) (string, error) {
	candidate, err := parseChatResultCandidate(raw)
	if err != nil {
		return "", err
	}
	answer := strings.TrimSpace(candidate.Answer)
	if answer == "" || len(candidate.ToolCalls) != 0 {
		return "", errors.New("chat result candidate is not a final answer")
	}
	return answer, nil
}

// IsToolCallCandidate validates the persisted continuation shape without
// accepting malformed candidates as safe to redispatch.
func IsToolCallCandidate(raw string) (bool, error) {
	candidate, err := parseChatResultCandidate(raw)
	if err != nil {
		return false, err
	}
	return len(candidate.ToolCalls) > 0, nil
}

// ChatResultFromCandidate restores the strict, durable result shape used when
// a worker resumes an interrupted tool continuation.
func ChatResultFromCandidate(raw string) (*infraai.ChatResult, error) {
	candidate, err := parseChatResultCandidate(raw)
	if err != nil {
		return nil, err
	}
	return &infraai.ChatResult{
		Answer:       strings.TrimSpace(candidate.Answer),
		ToolCalls:    append([]infraai.ToolCall(nil), candidate.ToolCalls...),
		Continuation: cloneChatContinuation(candidate.Continuation),
	}, nil
}

func parseChatResultCandidate(raw string) (chatResultCandidate, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	var candidate chatResultCandidate
	if err := decoder.Decode(&candidate); err != nil {
		return chatResultCandidate{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return chatResultCandidate{}, errors.New("chat result candidate has trailing content")
	}
	if candidate.Version != chatResultCandidateVersion && candidate.Version != legacyChatResultCandidateVersion {
		return chatResultCandidate{}, errors.New("chat result candidate version is invalid")
	}
	if candidate.Version == legacyChatResultCandidateVersion && candidate.Continuation != nil {
		return chatResultCandidate{}, errors.New("legacy chat result candidate contains continuation")
	}
	if err := validateChatContinuation(candidate.Continuation); err != nil {
		return chatResultCandidate{}, err
	}
	return candidate, nil
}

func validateChatContinuation(continuation *infraai.ChatContinuation) error {
	if continuation == nil {
		return nil
	}
	if strings.TrimSpace(continuation.Protocol) != infraai.APIProtocolResponses ||
		len(continuation.Items) == 0 || len(continuation.Items) > maxChatContinuationBytes {
		return errors.New("chat result candidate continuation is invalid")
	}
	var items []json.RawMessage
	if err := json.Unmarshal(continuation.Items, &items); err != nil || len(items) == 0 {
		return errors.New("chat result candidate continuation is invalid")
	}
	return nil
}

func cloneChatContinuation(continuation *infraai.ChatContinuation) *infraai.ChatContinuation {
	if continuation == nil {
		return nil
	}
	copy := *continuation
	copy.Items = append(json.RawMessage(nil), continuation.Items...)
	return &copy
}
