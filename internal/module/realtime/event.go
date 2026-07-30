package realtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	infrarealtime "admin_back_go/internal/infra/realtime"
)

const (
	TypeAIResponseStartV1     = "ai.response.start.v1"
	TypeAIResponseDeltaV2     = "ai.response.delta.v2"
	TypeAIResponseCompletedV1 = "ai.response.completed.v1"
	TypeAIResponseFailedV1    = "ai.response.failed.v1"
	TypeAIResponseCanceledV2  = "ai.response.canceled.v2"
	TypeNotificationCreatedV1 = "notification.created.v1"
	TypeConnectedV1           = "realtime.connected.v1"
	TypePingV1                = "realtime.ping.v1"
	TypePongV1                = "realtime.pong.v1"
	TypeSubscribeV1           = "realtime.subscribe.v1"
	TypeSubscribedV1          = "realtime.subscribed.v1"
	TypeResumeV1              = "realtime.resume.v1"
	TypeResyncRequiredV1      = "realtime.resync_required.v1"
	TypeErrorV1               = "realtime.error.v1"
)

type Direction string

const (
	DirectionClient        Direction = "client"
	DirectionServer        Direction = "server"
	DirectionBidirectional Direction = "bidirectional"
)

var (
	ErrUnknownEventType       = errors.New("unknown realtime event type")
	ErrEventPayloadInvalid    = errors.New("realtime event payload is invalid")
	ErrEventDurabilityInvalid = errors.New("realtime event durability is invalid")
	ErrEventDirectionInvalid  = errors.New("realtime event direction is invalid")
)

type payloadValidator interface {
	Validate() error
}

type EventDefinition struct {
	Type       string
	Direction  Direction
	Durability infrarealtime.Durability
	NewPayload func() any
}

type EventRegistry struct {
	definitions map[string]EventDefinition
}

var sharedDefaultRegistry = newDefaultRegistry()

func DefaultRegistry() *EventRegistry {
	return sharedDefaultRegistry
}

func newDefaultRegistry() *EventRegistry {
	registry := &EventRegistry{definitions: make(map[string]EventDefinition)}
	for _, definition := range defaultEventDefinitions() {
		registry.definitions[definition.Type] = definition
	}
	return registry
}

func (r *EventRegistry) Definition(eventType string) (EventDefinition, bool) {
	if r == nil {
		return EventDefinition{}, false
	}
	definition, ok := r.definitions[strings.TrimSpace(eventType)]
	return definition, ok
}

func (r *EventRegistry) Definitions() []EventDefinition {
	if r == nil {
		return nil
	}
	result := make([]EventDefinition, 0, len(r.definitions))
	for _, definition := range r.definitions {
		result = append(result, definition)
	}
	sort.Slice(result, func(left int, right int) bool {
		return result[left].Type < result[right].Type
	})
	return result
}

func (r *EventRegistry) EncodePayload(eventType string, payload any) (json.RawMessage, error) {
	definition, ok := r.Definition(eventType)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownEventType, strings.TrimSpace(eventType))
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEventPayloadInvalid, err)
	}
	if _, err := decodePayload(definition, raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (r *EventRegistry) DecodePayload(eventType string, raw json.RawMessage) (any, error) {
	definition, ok := r.Definition(eventType)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownEventType, strings.TrimSpace(eventType))
	}
	return decodePayload(definition, raw)
}

func (r *EventRegistry) NewEphemeral(eventType string, requestID string, payload any, occurredAt time.Time) (infrarealtime.Envelope, error) {
	definition, ok := r.Definition(eventType)
	if !ok {
		return infrarealtime.Envelope{}, fmt.Errorf("%w: %s", ErrUnknownEventType, strings.TrimSpace(eventType))
	}
	if definition.Durability != infrarealtime.Ephemeral {
		return infrarealtime.Envelope{}, fmt.Errorf("%w: %s is %s", ErrEventDurabilityInvalid, definition.Type, definition.Durability)
	}
	if err := validateServerDirection(definition); err != nil {
		return infrarealtime.Envelope{}, err
	}
	raw, err := r.EncodePayload(eventType, payload)
	if err != nil {
		return infrarealtime.Envelope{}, err
	}
	return infrarealtime.NewEnvelopeAt(eventType, requestID, raw, occurredAt)
}

func (r *EventRegistry) NewDurable(eventID string, eventType string, requestID string, sequence uint64, payload any, occurredAt time.Time) (infrarealtime.Envelope, error) {
	definition, ok := r.Definition(eventType)
	if !ok {
		return infrarealtime.Envelope{}, fmt.Errorf("%w: %s", ErrUnknownEventType, strings.TrimSpace(eventType))
	}
	if definition.Durability != infrarealtime.Durable {
		return infrarealtime.Envelope{}, fmt.Errorf("%w: %s is %s", ErrEventDurabilityInvalid, definition.Type, definition.Durability)
	}
	if err := validateServerDirection(definition); err != nil {
		return infrarealtime.Envelope{}, err
	}
	raw, err := r.EncodePayload(eventType, payload)
	if err != nil {
		return infrarealtime.Envelope{}, err
	}
	return infrarealtime.NewDurableEnvelope(eventID, eventType, requestID, sequence, occurredAt, raw)
}

// ValidateServerEnvelope enforces the central event registry at every
// publication seam. Structural validity alone is insufficient: the event must
// be registered, server-directed, use its declared durability, and carry its
// exact typed payload.
func (r *EventRegistry) ValidateServerEnvelope(envelope infrarealtime.Envelope) error {
	if err := infrarealtime.ValidateServerEnvelope(envelope); err != nil {
		return err
	}
	definition, ok := r.Definition(envelope.Type)
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownEventType, strings.TrimSpace(envelope.Type))
	}
	if err := validateServerDirection(definition); err != nil {
		return err
	}
	if definition.Durability != envelope.Durability {
		return fmt.Errorf("%w: %s is %s, envelope is %s", ErrEventDurabilityInvalid, definition.Type, definition.Durability, envelope.Durability)
	}
	_, err := r.DecodePayload(definition.Type, envelope.Data)
	return err
}

func validateServerDirection(definition EventDefinition) error {
	if definition.Direction != DirectionServer && definition.Direction != DirectionBidirectional {
		return fmt.Errorf("%w: %s is %s", ErrEventDirectionInvalid, definition.Type, definition.Direction)
	}
	return nil
}

func decodePayload(definition EventDefinition, raw json.RawMessage) (any, error) {
	if definition.NewPayload == nil {
		return nil, fmt.Errorf("%w: %s has no codec", ErrEventPayloadInvalid, definition.Type)
	}
	payload := definition.NewPayload()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(payload); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrEventPayloadInvalid, definition.Type, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: %s has trailing content", ErrEventPayloadInvalid, definition.Type)
	}
	if validator, ok := payload.(payloadValidator); ok {
		if err := validator.Validate(); err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrEventPayloadInvalid, definition.Type, err)
		}
	}
	return payload, nil
}

type ConnectedPayload struct {
	UserID              int64  `json:"user_id"`
	Platform            string `json:"platform"`
	HeartbeatIntervalMS int64  `json:"heartbeat_interval_ms"`
}

func (p *ConnectedPayload) Validate() error {
	if p.UserID <= 0 || strings.TrimSpace(p.Platform) != "admin" || p.HeartbeatIntervalMS <= 0 {
		return errors.New("invalid connected payload")
	}
	return nil
}

type EmptyPayload struct{}

func (*EmptyPayload) Validate() error { return nil }

type PongPayload struct {
	ServerTime string `json:"server_time"`
}

func (p *PongPayload) Validate() error {
	if _, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(p.ServerTime)); err != nil {
		return errors.New("server_time must be RFC3339")
	}
	return nil
}

type SubscribePayload struct {
	Topics []string `json:"topics"`
}

func (p *SubscribePayload) Validate() error {
	if len(p.Topics) == 0 {
		return errors.New("topics is required")
	}
	seen := make(map[string]struct{}, len(p.Topics))
	for _, topic := range p.Topics {
		topic = strings.TrimSpace(topic)
		if topic == "" || utf8.RuneCountInString(topic) > 128 {
			return errors.New("topic is invalid")
		}
		if _, exists := seen[topic]; exists {
			return errors.New("topics must be unique")
		}
		seen[topic] = struct{}{}
	}
	return nil
}

type SubscribedPayload = SubscribePayload

type ResumePayload struct {
	AfterSequence *uint64 `json:"after_sequence"`
}

func (p *ResumePayload) Validate() error {
	if p.AfterSequence == nil {
		return errors.New("after_sequence is required")
	}
	return nil
}

type ResyncRequiredPayload struct {
	LatestSequence uint64 `json:"latest_sequence"`
}

func (*ResyncRequiredPayload) Validate() error { return nil }

type ErrorPayload struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func (p *ErrorPayload) Validate() error {
	if p.Code <= 0 || strings.TrimSpace(p.Msg) == "" || len([]rune(p.Msg)) > 1024 {
		return errors.New("invalid error payload")
	}
	return nil
}

type NotificationCreatedPayload struct {
	TaskID           int64  `json:"task_id"`
	Title            string `json:"title"`
	Content          string `json:"content"`
	Link             string `json:"link"`
	Level            string `json:"level"`
	NotificationType string `json:"notification_type"`
}

func (p *NotificationCreatedPayload) Validate() error {
	if p.TaskID <= 0 || len([]rune(p.Title)) > 255 || len([]rune(p.Content)) > 65536 || len([]rune(p.Link)) > 2048 {
		return errors.New("invalid notification payload")
	}
	if p.Level != "normal" && p.Level != "urgent" {
		return errors.New("invalid notification level")
	}
	switch p.NotificationType {
	case "info", "success", "warning", "error":
		return nil
	default:
		return errors.New("invalid notification type")
	}
}

type AIResponseStartPayload struct {
	ConversationID int64  `json:"conversation_id"`
	RequestID      string `json:"request_id"`
	UserMessageID  int64  `json:"user_message_id"`
	AgentID        int64  `json:"agent_id"`
}

func (p *AIResponseStartPayload) Validate() error {
	if p.ConversationID <= 0 || p.UserMessageID <= 0 || p.AgentID <= 0 || !validRequestID(p.RequestID) {
		return errors.New("invalid AI start payload")
	}
	return nil
}

type AIResponseDeltaPayload struct {
	ConversationID int64  `json:"conversation_id"`
	RequestID      string `json:"request_id"`
	DeliverySeq    uint32 `json:"delivery_seq"`
	Delta          string `json:"delta"`
}

func (p *AIResponseDeltaPayload) Validate() error {
	if p.ConversationID <= 0 || !validRequestID(p.RequestID) || p.DeliverySeq == 0 || p.Delta == "" ||
		!utf8.ValidString(p.Delta) || len(p.Delta) > 16*1024 {
		return errors.New("invalid AI delta payload")
	}
	return nil
}

type AIResponseCompletedPayload struct {
	ConversationID     int64  `json:"conversation_id"`
	RequestID          string `json:"request_id"`
	AssistantMessageID int64  `json:"assistant_message_id"`
}

func (p *AIResponseCompletedPayload) Validate() error {
	if p.ConversationID <= 0 || p.AssistantMessageID <= 0 || !validRequestID(p.RequestID) {
		return errors.New("invalid AI completed payload")
	}
	return nil
}

type AIResponseFailedPayload struct {
	ConversationID int64   `json:"conversation_id"`
	RequestID      string  `json:"request_id"`
	Msg            string  `json:"msg"`
	ErrorCode      string  `json:"error_code"`
	WalletPath     *string `json:"wallet_path"`
	RechargePath   *string `json:"recharge_path"`
}

type AIResponseCanceledPayload struct {
	ConversationID     int64  `json:"conversation_id"`
	RequestID          string `json:"request_id"`
	AssistantMessageID int64  `json:"assistant_message_id"`
}

func (p *AIResponseCanceledPayload) Validate() error {
	if p.ConversationID <= 0 || p.AssistantMessageID <= 0 || !validRequestID(p.RequestID) {
		return errors.New("invalid AI canceled payload")
	}
	return nil
}

func (p *AIResponseFailedPayload) Validate() error {
	errorCode := strings.TrimSpace(p.ErrorCode)
	if p.ConversationID <= 0 || !validRequestID(p.RequestID) || strings.TrimSpace(p.Msg) == "" || len([]rune(p.Msg)) > 1024 || errorCode == "" || p.ErrorCode != errorCode || len([]rune(p.ErrorCode)) > 128 {
		return errors.New("invalid AI failed payload")
	}
	if p.ErrorCode == "ai.billing.insufficient_balance" {
		if p.WalletPath == nil || *p.WalletPath != "/profile/wallet" || p.RechargePath == nil || *p.RechargePath != "/payment/recharge" {
			return errors.New("invalid AI billing failure paths")
		}
	} else if p.WalletPath != nil || p.RechargePath != nil {
		return errors.New("non-billing AI failure must not expose wallet paths")
	}
	return nil
}

func validRequestID(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && utf8.RuneCountInString(value) <= 128
}

func defaultEventDefinitions() []EventDefinition {
	return []EventDefinition{
		{Type: TypeAIResponseStartV1, Direction: DirectionServer, Durability: infrarealtime.Ephemeral, NewPayload: func() any { return &AIResponseStartPayload{} }},
		{Type: TypeAIResponseDeltaV2, Direction: DirectionServer, Durability: infrarealtime.Ephemeral, NewPayload: func() any { return &AIResponseDeltaPayload{} }},
		{Type: TypeAIResponseCompletedV1, Direction: DirectionServer, Durability: infrarealtime.Durable, NewPayload: func() any { return &AIResponseCompletedPayload{} }},
		{Type: TypeAIResponseFailedV1, Direction: DirectionServer, Durability: infrarealtime.Durable, NewPayload: func() any { return &AIResponseFailedPayload{} }},
		{Type: TypeAIResponseCanceledV2, Direction: DirectionServer, Durability: infrarealtime.Durable, NewPayload: func() any { return &AIResponseCanceledPayload{} }},
		{Type: TypeNotificationCreatedV1, Direction: DirectionServer, Durability: infrarealtime.Durable, NewPayload: func() any { return &NotificationCreatedPayload{} }},
		{Type: TypeConnectedV1, Direction: DirectionServer, Durability: infrarealtime.Ephemeral, NewPayload: func() any { return &ConnectedPayload{} }},
		{Type: TypePingV1, Direction: DirectionBidirectional, Durability: infrarealtime.Ephemeral, NewPayload: func() any { return &EmptyPayload{} }},
		{Type: TypePongV1, Direction: DirectionServer, Durability: infrarealtime.Ephemeral, NewPayload: func() any { return &PongPayload{} }},
		{Type: TypeSubscribeV1, Direction: DirectionClient, Durability: infrarealtime.Ephemeral, NewPayload: func() any { return &SubscribePayload{} }},
		{Type: TypeSubscribedV1, Direction: DirectionServer, Durability: infrarealtime.Ephemeral, NewPayload: func() any { return &SubscribedPayload{} }},
		{Type: TypeResumeV1, Direction: DirectionClient, Durability: infrarealtime.Ephemeral, NewPayload: func() any { return &ResumePayload{} }},
		{Type: TypeResyncRequiredV1, Direction: DirectionServer, Durability: infrarealtime.Ephemeral, NewPayload: func() any { return &ResyncRequiredPayload{} }},
		{Type: TypeErrorV1, Direction: DirectionServer, Durability: infrarealtime.Ephemeral, NewPayload: func() any { return &ErrorPayload{} }},
	}
}
