package aichat

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	platformrealtime "admin_back_go/internal/platform/realtime"
)

const (
	EventAIResponseStart     = "ai.response.start.v1"
	EventAIResponseDelta     = "ai.response.delta.v1"
	EventAIResponseCompleted = "ai.response.completed.v1"
	EventAIResponseFailed    = "ai.response.failed.v1"
	EventAIResponseCancel    = "ai.response.cancel.v1"
)

type EnvelopeEvent struct {
	ID       string
	Event    string
	Envelope platformrealtime.Envelope
}

type StreamIDGenerator struct {
	mu     sync.Mutex
	lastMS int64
	seq    int64
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
	RunID          int64  `json:"run_id"`
	ConversationID int64  `json:"conversation_id"`
	RequestID      string `json:"request_id"`
	UserMessageID  int64  `json:"user_message_id"`
	AgentID        int64  `json:"agent_id"`
	IsNew          bool   `json:"is_new"`
}

type CompletedPayload struct {
	RunID              int64 `json:"run_id"`
	ConversationID     int64 `json:"conversation_id"`
	UserMessageID      int64 `json:"user_message_id"`
	AssistantMessageID int64 `json:"assistant_message_id"`
}

func BuildStartEvent(payload StartPayload) (EnvelopeEvent, error) {
	return buildEvent(EventAIResponseStart, payload)
}

func BuildDeltaEvent(runID int64, delta string) (EnvelopeEvent, error) {
	return buildEvent(EventAIResponseDelta, map[string]any{"run_id": runID, "delta": delta})
}

func BuildCompletedEvent(payload CompletedPayload) (EnvelopeEvent, error) {
	return buildEvent(EventAIResponseCompleted, payload)
}

func BuildFailedEvent(runID int64, message string) (EnvelopeEvent, error) {
	return buildEvent(EventAIResponseFailed, map[string]any{"run_id": runID, "msg": message})
}

func BuildCancelEvent(runID int64) (EnvelopeEvent, error) {
	return buildEvent(EventAIResponseCancel, map[string]any{"run_id": runID})
}

func buildEvent(eventType string, payload any) (EnvelopeEvent, error) {
	id := NewStreamIDGenerator().Next()
	envelope, err := platformrealtime.NewEnvelope(eventType, id, payload)
	if err != nil {
		return EnvelopeEvent{}, err
	}
	return EnvelopeEvent{ID: id, Event: eventType, Envelope: envelope}, nil
}

func isNewerStreamID(candidate string, current string) bool {
	candidateMS, candidateSeq, ok := parseStreamID(candidate)
	if !ok {
		return false
	}
	currentMS, currentSeq, ok := parseStreamID(current)
	if !ok {
		return true
	}
	return candidateMS > currentMS || (candidateMS == currentMS && candidateSeq > currentSeq)
}

func parseStreamID(value string) (int64, int64, bool) {
	parts := strings.Split(value, "-")
	if len(parts) != 2 {
		return 0, 0, false
	}
	ms, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	seq, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	return ms, seq, true
}
