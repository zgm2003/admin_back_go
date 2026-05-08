package dify

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	platformai "admin_back_go/internal/platform/ai"
)

type streamEvent struct {
	Event          string         `json:"event"`
	TaskID         string         `json:"task_id"`
	ID             string         `json:"id"`
	MessageID      string         `json:"message_id"`
	ConversationID string         `json:"conversation_id"`
	Answer         string         `json:"answer"`
	Metadata       streamMetadata `json:"metadata"`
	Code           string         `json:"code"`
	Message        string         `json:"message"`
}

type streamMetadata struct {
	Usage usage `json:"usage"`
}

type usage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	TotalPrice       string  `json:"total_price"`
	Latency          float64 `json:"latency"`
}

func parseStreamEvents(r io.Reader) ([]streamEvent, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	events := []streamEvent{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var event streamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil, fmt.Errorf("decode dify stream event: %w", err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read dify stream: %w", err)
	}
	return events, nil
}

func streamResult(events []streamEvent) (*platformai.ChatResult, error) {
	result := &platformai.ChatResult{}
	var answer bytes.Buffer
	for _, event := range events {
		if event.TaskID != "" {
			result.EngineTaskID = event.TaskID
		}
		if event.MessageID != "" {
			result.EngineMessageID = event.MessageID
		} else if event.ID != "" && result.EngineMessageID == "" && event.Event == "message" {
			result.EngineMessageID = event.ID
		}
		if event.ConversationID != "" {
			result.EngineConversationID = event.ConversationID
		}
		if event.Answer != "" {
			answer.WriteString(event.Answer)
		}
		if event.Metadata.Usage.TotalTokens > 0 || event.Metadata.Usage.PromptTokens > 0 || event.Metadata.Usage.CompletionTokens > 0 {
			result.PromptTokens = event.Metadata.Usage.PromptTokens
			result.CompletionTokens = event.Metadata.Usage.CompletionTokens
			result.TotalTokens = event.Metadata.Usage.TotalTokens
			result.Cost = parsePrice(event.Metadata.Usage.TotalPrice)
			result.LatencyMs = int(event.Metadata.Usage.Latency * 1000)
		}
		if event.Event == "error" {
			msg := strings.TrimSpace(event.Message)
			if msg == "" {
				msg = strings.TrimSpace(event.Code)
			}
			if msg == "" {
				msg = "dify stream error"
			}
			return nil, fmt.Errorf("%w: %s", platformai.ErrUpstreamFailed, msg)
		}
	}
	result.Answer = answer.String()
	return result, nil
}

func eventForSink(event streamEvent) (platformai.Event, bool) {
	switch event.Event {
	case "message", "agent_message":
		if event.Answer == "" {
			return platformai.Event{}, false
		}
		return platformai.Event{Type: "delta", DeltaText: event.Answer, Payload: payloadForEvent(event)}, true
	case "message_end":
		return platformai.Event{Type: "completed", Payload: payloadForEvent(event)}, true
	case "error":
		return platformai.Event{Type: "failed", Payload: payloadForEvent(event)}, true
	default:
		return platformai.Event{Type: event.Event, Payload: payloadForEvent(event)}, event.Event != ""
	}
}

func payloadForEvent(event streamEvent) map[string]any {
	payload := map[string]any{
		"event": event.Event,
	}
	if event.TaskID != "" {
		payload["task_id"] = event.TaskID
	}
	if event.ID != "" {
		payload["id"] = event.ID
	}
	if event.MessageID != "" {
		payload["message_id"] = event.MessageID
	}
	if event.ConversationID != "" {
		payload["conversation_id"] = event.ConversationID
	}
	if event.Answer != "" {
		payload["answer"] = event.Answer
	}
	if event.Code != "" {
		payload["code"] = event.Code
	}
	if event.Message != "" {
		payload["message"] = event.Message
	}
	if event.Metadata.Usage.TotalTokens > 0 || event.Metadata.Usage.PromptTokens > 0 || event.Metadata.Usage.CompletionTokens > 0 {
		payload["usage"] = map[string]any{
			"prompt_tokens":     event.Metadata.Usage.PromptTokens,
			"completion_tokens": event.Metadata.Usage.CompletionTokens,
			"total_tokens":      event.Metadata.Usage.TotalTokens,
			"total_price":       event.Metadata.Usage.TotalPrice,
			"latency":           event.Metadata.Usage.Latency,
		}
	}
	return payload
}
