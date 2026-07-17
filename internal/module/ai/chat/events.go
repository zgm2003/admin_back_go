package aichat

import (
	"fmt"
	"sync"
	"time"

	infrarealtime "admin_back_go/internal/infra/realtime"
)

const (
	EventAIResponseStart     = "ai.response.start.v1"
	EventAIResponseDelta     = "ai.response.delta.v1"
	EventAIResponseCompleted = "ai.response.completed.v1"
	EventAIResponseFailed    = "ai.response.failed.v1"
)

type EnvelopeEvent struct {
	ID       string
	Event    string
	Envelope infrarealtime.Envelope
}

type StreamIDGenerator struct {
	mu     sync.Mutex
	lastMS int64
	seq    int64
}

func StreamIDFromSeq(seq uint64) string {
	if seq == 0 {
		return "0-0"
	}
	return fmt.Sprintf("%d-0", seq)
}

func NewStreamIDGenerator() *StreamIDGenerator {
	return &StreamIDGenerator{}
}

func (g *StreamIDGenerator) Next() string {
	if g == nil {
		g = NewStreamIDGenerator()
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now().UnixMilli()
	if now == g.lastMS {
		g.seq++
	} else {
		g.lastMS = now
		g.seq = 0
	}
	return fmt.Sprintf("%d-%d", g.lastMS, g.seq)
}

type StartPayload struct {
	ConversationID int64  `json:"conversation_id"`
	RequestID      string `json:"request_id"`
	UserMessageID  int64  `json:"user_message_id"`
	AgentID        int64  `json:"agent_id"`
}

type DeltaPayload struct {
	ConversationID int64  `json:"conversation_id"`
	RequestID      string `json:"request_id"`
	Delta          string `json:"delta"`
}

type CompletedPayload struct {
	ConversationID     int64  `json:"conversation_id"`
	RequestID          string `json:"request_id"`
	AssistantMessageID int64  `json:"assistant_message_id"`
}

type FailedPayload struct {
	ConversationID int64  `json:"conversation_id"`
	RequestID      string `json:"request_id"`
	Msg            string `json:"msg"`
}

func BuildStartEvent(payload StartPayload) (EnvelopeEvent, error) {
	return buildEvent(EventAIResponseStart, payload)
}

func BuildDeltaEvent(payload DeltaPayload) (EnvelopeEvent, error) {
	return buildEvent(EventAIResponseDelta, payload)
}

func BuildCompletedEvent(payload CompletedPayload) (EnvelopeEvent, error) {
	return buildEvent(EventAIResponseCompleted, payload)
}

func BuildFailedEvent(payload FailedPayload) (EnvelopeEvent, error) {
	return buildEvent(EventAIResponseFailed, payload)
}

func BuildEventFromPayload(eventType string, payload any) (EnvelopeEvent, error) {
	return buildEvent(eventType, payload)
}

func buildEvent(eventType string, payload any) (EnvelopeEvent, error) {
	id := NewStreamIDGenerator().Next()
	envelope, err := infrarealtime.NewEnvelope(eventType, id, payload)
	if err != nil {
		return EnvelopeEvent{}, err
	}
	return EnvelopeEvent{ID: id, Event: eventType, Envelope: envelope}, nil
}
