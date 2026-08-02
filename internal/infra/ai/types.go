package ai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

type EngineType string

const (
	EngineTypeOpenAI EngineType = "openai"
)

const (
	APIProtocolChatCompletions = "chat_completions"
	APIProtocolResponses       = "responses"
)

const (
	UsageStatusReported    = "reported"
	UsageStatusComplete    = "complete"
	UsageStatusUnavailable = "unavailable"
)

const (
	UsageCategoryInput      = "input"
	UsageCategoryOutput     = "output"
	UsageCategoryCacheRead  = "cache_read"
	UsageCategoryCacheWrite = "cache_write"
	UsageCategoryMedia      = "media"

	DispatchStateNotDispatched = "not_dispatched"
	DispatchStateDispatched    = "dispatched"
	DispatchStateUnknown       = "unknown"
)

// SafeInputUpperBoundStrategyUTF8RequestBytesV1 is the conservative strategy
// used by compatible chat transports. A provider request's UTF-8 byte length
// plus the fixed framing allowance is an upper bound for the input token
// count; the exact same function is used when quoting and when proving the
// prepared request before dispatch.
const SafeInputUpperBoundStrategyUTF8RequestBytesV1 = "utf8_request_bytes_plus_framing_v1"

const SafeInputUpperBoundStrategyNativeFileContextWindowV1 = "native_file_context_window_v1"

const safeInputUpperBoundFramingBytes int64 = 64

// SafeInputUpperBoundFromRequest returns the deterministic input-token ceiling
// for a serialized provider request. It intentionally over-reserves rather
// than guessing a tokenizer or silently charging an unbounded request.
func SafeInputUpperBoundFromRequest(body []byte) (int64, error) {
	if len(body) == 0 {
		return 0, errors.New("prepared request body is required")
	}
	if int64(len(body)) > int64(^uint64(0)>>1)-safeInputUpperBoundFramingBytes {
		return 0, errors.New("prepared request body is too large")
	}
	return int64(len(body)) + safeInputUpperBoundFramingBytes, nil
}

// UsageItem is the provider-neutral, integer usage unit consumed by pricing.
// Zero is valid only when the provider explicitly reports that item.
type UsageItem struct {
	Category string `json:"category"`
	Unit     string `json:"unit"`
	TierKey  string `json:"tier_key,omitempty"`
	Quantity int64  `json:"quantity"`
}

func (item UsageItem) Validate() error {
	if item.Category == "" || item.Unit == "" || item.Quantity < 0 {
		return errors.New("usage item category, unit and non-negative quantity are required")
	}
	return nil
}

type UsageSnapshot struct {
	Status          string          `json:"status"`
	RawProviderJSON json.RawMessage `json:"raw_provider_json,omitempty"`
	Items           []UsageItem     `json:"items,omitempty"`
	ResponseSHA256  [32]byte        `json:"response_sha256,omitempty"`
}

type usageSnapshotJSON struct {
	Status           string          `json:"status"`
	RawProviderJSON  json.RawMessage `json:"raw_provider_json,omitempty"`
	RawProviderBytes []byte          `json:"raw_provider_bytes,omitempty"`
	Items            []UsageItem     `json:"items,omitempty"`
	ResponseSHA256   [32]byte        `json:"response_sha256,omitempty"`
}

// MarshalJSON retains the historical JSON projection and an exact byte copy.
// encoding/json compacts RawMessage values, which otherwise breaks its hash.
func (s UsageSnapshot) MarshalJSON() ([]byte, error) {
	return json.Marshal(usageSnapshotJSON{
		Status:           s.Status,
		RawProviderJSON:  append(json.RawMessage(nil), s.RawProviderJSON...),
		RawProviderBytes: append([]byte(nil), s.RawProviderJSON...),
		Items:            s.Items,
		ResponseSHA256:   s.ResponseSHA256,
	})
}

func (s *UsageSnapshot) UnmarshalJSON(data []byte) error {
	var encoded usageSnapshotJSON
	if err := json.Unmarshal(data, &encoded); err != nil {
		return err
	}
	raw := append(json.RawMessage(nil), encoded.RawProviderJSON...)
	if len(encoded.RawProviderBytes) > 0 {
		if !json.Valid(encoded.RawProviderBytes) {
			return errors.New("raw provider bytes must contain valid JSON")
		}
		if len(raw) > 0 {
			var compactRaw bytes.Buffer
			if err := json.Compact(&compactRaw, raw); err != nil {
				return fmt.Errorf("raw provider JSON is invalid: %w", err)
			}
			projectedBytes, err := json.Marshal(json.RawMessage(encoded.RawProviderBytes))
			if err != nil {
				return fmt.Errorf("project raw provider bytes: %w", err)
			}
			if !bytes.Equal(compactRaw.Bytes(), projectedBytes) {
				return errors.New("raw provider JSON and exact bytes disagree")
			}
		}
		raw = append(json.RawMessage(nil), encoded.RawProviderBytes...)
	} else {
		raw = restoreHashVerifiedLegacyProviderBytes(raw, encoded.ResponseSHA256)
	}
	*s = UsageSnapshot{
		Status:          encoded.Status,
		RawProviderJSON: raw,
		Items:           encoded.Items,
		ResponseSHA256:  encoded.ResponseSHA256,
	}
	return nil
}

func restoreHashVerifiedLegacyProviderBytes(raw json.RawMessage, expected [sha256.Size]byte) json.RawMessage {
	if len(raw) == 0 || expected == ([sha256.Size]byte{}) || sha256.Sum256(raw) == expected {
		return raw
	}
	restored := append(json.RawMessage(nil), raw...)
	for _, replacement := range []struct {
		from string
		to   string
	}{
		{`\u003c`, "<"},
		{`\u003e`, ">"},
		{`\u0026`, "&"},
		{`\u2028`, "\u2028"},
		{`\u2029`, "\u2029"},
	} {
		restored = bytes.ReplaceAll(restored, []byte(replacement.from), []byte(replacement.to))
	}
	if sha256.Sum256(restored) == expected {
		return restored
	}
	return raw
}

type UsageIdentity struct {
	Category string `json:"category"`
	Unit     string `json:"unit"`
	TierKey  string `json:"tier_key,omitempty"`
}

func (identity UsageIdentity) Normalized() (UsageIdentity, error) {
	identity.Category = strings.TrimSpace(identity.Category)
	identity.Unit = strings.TrimSpace(identity.Unit)
	identity.TierKey = strings.TrimSpace(identity.TierKey)
	if err := validateUsageItems([]UsageItem{{Category: identity.Category, Unit: identity.Unit, TierKey: identity.TierKey}}); err != nil {
		return UsageIdentity{}, err
	}
	return identity, nil
}

type CapabilityMetadata struct {
	SupportedUsageIdentities      []UsageIdentity `json:"supported_usage_identities"`
	SafeInputUpperBoundStrategy   string          `json:"safe_input_upper_bound_strategy"`
	SafeInputUpperBoundStrategies []string        `json:"safe_input_upper_bound_strategies"`
	SupportsIdempotencyHeader     bool            `json:"supports_idempotency_header"`
	SupportsCancelTask            bool            `json:"supports_cancel_task"`
	InputModalities               []string        `json:"input_modalities"`
	OutputModalities              []string        `json:"output_modalities"`
	SupportedParameters           []string        `json:"supported_parameters"`
	SupportsTools                 bool            `json:"supports_tools"`
	SupportsStreaming             bool            `json:"supports_streaming"`
	SupportsStructuredOutput      bool            `json:"supports_structured_output"`
}

type CapabilityProvider interface {
	Capabilities() CapabilityMetadata
}

type TransportCapabilityResolver interface {
	ResolveCapabilities(EngineType) (CapabilityMetadata, bool)
}

type TransportCapabilityResolverFunc func(EngineType) (CapabilityMetadata, bool)

func (resolve TransportCapabilityResolverFunc) ResolveCapabilities(engineType EngineType) (CapabilityMetadata, bool) {
	if resolve == nil {
		return CapabilityMetadata{}, false
	}
	return resolve(engineType)
}

func DefaultTransportCapabilities(engineType EngineType) (CapabilityMetadata, bool) {
	if engineType != EngineTypeOpenAI {
		return CapabilityMetadata{}, false
	}
	return CapabilityMetadata{
		SupportedUsageIdentities: []UsageIdentity{
			{Category: UsageCategoryInput, Unit: "token"},
			{Category: UsageCategoryOutput, Unit: "token"},
			{Category: UsageCategoryCacheRead, Unit: "token"},
			{Category: UsageCategoryCacheWrite, Unit: "token"},
		},
		SafeInputUpperBoundStrategy: SafeInputUpperBoundStrategyUTF8RequestBytesV1,
		SafeInputUpperBoundStrategies: []string{
			SafeInputUpperBoundStrategyUTF8RequestBytesV1,
			SafeInputUpperBoundStrategyNativeFileContextWindowV1,
		},
		SupportsIdempotencyHeader: true,
		InputModalities:           []string{"text", "image", "file"},
		OutputModalities:          []string{"text"},
		SupportedParameters:       []string{"temperature"},
		SupportsTools:             true,
		SupportsStreaming:         true,
		SupportsStructuredOutput:  true,
	}, true
}

func (s UsageSnapshot) Complete() bool {
	if s.Status != UsageStatusReported && s.Status != UsageStatusComplete {
		return false
	}
	return validateUsageItems(s.Items) == nil
}

func (s UsageSnapshot) Validate() error {
	if s.Status == UsageStatusUnavailable {
		return nil
	}
	if s.Status != UsageStatusReported && s.Status != UsageStatusComplete {
		return fmt.Errorf("unknown usage status %q", s.Status)
	}
	return validateUsageItems(s.Items)
}

func validateUsageItems(items []UsageItem) error {
	if len(items) == 0 {
		return errors.New("complete usage requires at least one item")
	}
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if err := item.Validate(); err != nil {
			return err
		}
		if item.Category != UsageCategoryInput && item.Category != UsageCategoryOutput && item.Category != UsageCategoryCacheRead && item.Category != UsageCategoryCacheWrite && item.Category != UsageCategoryMedia {
			return fmt.Errorf("unsupported usage category %q", item.Category)
		}
		key := item.Category + "|" + item.Unit + "|" + item.TierKey
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate usage item %q", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func NewUsageSnapshot(status string, raw []byte, items []UsageItem) (UsageSnapshot, error) {
	s := UsageSnapshot{Status: status, RawProviderJSON: append([]byte(nil), raw...), Items: append([]UsageItem(nil), items...)}
	s.ResponseSHA256 = sha256.Sum256(raw)
	return s, s.Validate()
}

type TestConnectionInput struct {
	EngineType EngineType
	BaseURL    string
	APIKey     string
	TimeoutMs  int
}

type TestConnectionResult struct {
	OK        bool   `json:"ok"`
	Status    string `json:"status"`
	LatencyMs int    `json:"latency_ms"`
	Message   string `json:"message"`
}

type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

type ToolOutput struct {
	CallID string
	Name   string
	Output string
}

type MessageRole string

const (
	MessageRoleSystem    MessageRole = "system"
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
)

func (role MessageRole) Validate() error {
	switch role {
	case MessageRoleSystem, MessageRoleUser, MessageRoleAssistant:
		return nil
	}
	return fmt.Errorf("%w: unsupported chat message role %q", ErrInvalidConfig, role)
}

type ContentPartKind string

const (
	ContentPartText       ContentPartKind = "text"
	ContentPartAttachment ContentPartKind = "attachment"
)

func (kind ContentPartKind) Validate() error {
	switch kind {
	case ContentPartText, ContentPartAttachment:
		return nil
	}
	return fmt.Errorf("%w: unsupported chat content part kind %q", ErrInvalidConfig, kind)
}

type AttachmentKind string

const (
	AttachmentImage AttachmentKind = "image"
	AttachmentFile  AttachmentKind = "file"
)

func (kind AttachmentKind) Validate() error {
	switch kind {
	case AttachmentImage, AttachmentFile:
		return nil
	}
	return fmt.Errorf("%w: unsupported chat attachment kind %q", ErrInvalidConfig, kind)
}

type AttachmentRef struct {
	Kind      AttachmentKind
	URL       string
	ObjectKey string
	ETag      string
	Size      int64
	MIMEType  string
	Filename  string
}

func (ref AttachmentRef) Validate() error {
	if err := ref.Kind.Validate(); err != nil {
		return err
	}
	switch ref.Kind {
	case AttachmentImage:
		if strings.TrimSpace(ref.URL) == "" ||
			(strings.TrimSpace(ref.MIMEType) != "" && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(ref.MIMEType)), "image/")) {
			return fmt.Errorf("%w: image attachment URL or MIME type is invalid", ErrInvalidConfig)
		}
	case AttachmentFile:
		if strings.TrimSpace(ref.ObjectKey) == "" || strings.TrimSpace(ref.ETag) == "" || ref.Size <= 0 ||
			strings.TrimSpace(ref.MIMEType) == "" || strings.TrimSpace(ref.Filename) == "" {
			return fmt.Errorf("%w: native file attachment facts are incomplete", ErrInvalidConfig)
		}
	}
	return nil
}

type ContentPart struct {
	Kind       ContentPartKind
	Text       string
	Attachment *AttachmentRef
}

func (part ContentPart) Validate() error {
	if err := part.Kind.Validate(); err != nil {
		return err
	}
	switch part.Kind {
	case ContentPartText:
		if strings.TrimSpace(part.Text) == "" || part.Attachment != nil {
			return fmt.Errorf("%w: text content part is invalid", ErrInvalidConfig)
		}
	case ContentPartAttachment:
		if part.Text != "" || part.Attachment == nil {
			return fmt.Errorf("%w: attachment content part is invalid", ErrInvalidConfig)
		}
		if err := part.Attachment.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type Message struct {
	Role  MessageRole
	Parts []ContentPart
}

func (message Message) Validate() error {
	if err := message.Role.Validate(); err != nil {
		return err
	}
	if len(message.Parts) == 0 {
		return fmt.Errorf("%w: chat message parts are required", ErrInvalidConfig)
	}
	if message.Role == MessageRoleSystem && (len(message.Parts) != 1 || message.Parts[0].Kind != ContentPartText) {
		return fmt.Errorf("%w: system messages require exactly one text part", ErrInvalidConfig)
	}
	for _, part := range message.Parts {
		if err := part.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// ChatContinuation carries provider-owned output items that must be replayed
// on the next Responses turn, notably encrypted reasoning and function calls.
// Items is an opaque JSON array validated by the compatible transport.
type ChatContinuation struct {
	Protocol string          `json:"protocol"`
	Items    json.RawMessage `json:"items"`
}

type ChatInput struct {
	AttemptID                uint64
	IdempotencyKey           string
	AgentID                  uint64
	RunID                    uint64
	UserID                   uint64
	UserKey                  string
	ModelID                  string
	Messages                 []Message
	Temperature              *float64
	ConversationEngineID     string
	EffectiveMaxOutputTokens int
	Tools                    []ToolDefinition
	ToolCalls                []ToolCall
	ToolOutputs              []ToolOutput
	Continuation             *ChatContinuation
}

func (input ChatInput) Validate() error {
	if strings.TrimSpace(input.ModelID) == "" || len(input.Messages) == 0 {
		return fmt.Errorf("%w: chat model and messages are required", ErrInvalidConfig)
	}
	hasUser := false
	for _, message := range input.Messages {
		if err := message.Validate(); err != nil {
			return err
		}
		if message.Role == MessageRoleUser {
			hasUser = true
		}
	}
	if !hasUser {
		return fmt.Errorf("%w: chat input requires a user message", ErrInvalidConfig)
	}
	if input.Temperature != nil && (math.IsNaN(*input.Temperature) || math.IsInf(*input.Temperature, 0)) {
		return fmt.Errorf("%w: chat temperature must be finite", ErrInvalidConfig)
	}
	return nil
}

type ChatResult struct {
	ProviderRequestID    string
	EngineConversationID string
	EngineMessageID      string
	EngineTaskID         string
	Answer               string
	ToolCalls            []ToolCall
	PromptTokens         int
	CompletionTokens     int
	TotalTokens          int
	UsageStatus          string
	Usage                UsageSnapshot `json:"usage"`
	DispatchState        string        `json:"dispatch_state"`
	ResponseSHA256       [32]byte      `json:"response_sha256,omitempty"`
	LatencyMs            int
	FileInputMetrics     *FileInputMetrics `json:"file_input_metrics,omitempty"`
	Continuation         *ChatContinuation `json:"continuation,omitempty"`
}

type Event struct {
	Type      string
	DeltaText string
	Payload   map[string]any
}

type EventSink interface {
	Emit(ctx context.Context, event Event) error
}

type FatalEventSinkError struct{ Err error }

func (err *FatalEventSinkError) Error() string {
	if err == nil || err.Err == nil {
		return "fatal AI event sink error"
	}
	return err.Err.Error()
}

func (err *FatalEventSinkError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

func FatalEventSink(err error) error {
	if err == nil {
		return nil
	}
	return &FatalEventSinkError{Err: err}
}

func IsFatalEventSinkError(err error) bool {
	var fatal *FatalEventSinkError
	return errors.As(err, &fatal)
}

type Engine interface {
	TestConnection(ctx context.Context, input TestConnectionInput) (*TestConnectionResult, error)
	StreamChat(ctx context.Context, input ChatInput, sink EventSink) (*ChatResult, error)
}

// PreparedChatRequest is the exact credential-free request body persisted by
// the billing gateway before a paid provider call is dispatched.
type PreparedChatRequest struct {
	Body           []byte
	IdempotencyKey string
}

// PreparedChatEngine extends Engine for paid dispatch. Implementations must
// send Body verbatim; they must not rebuild it from ChatInput during recovery.
type PreparedChatEngine interface {
	Engine
	PrepareChat(context.Context, ChatInput) ([]byte, error)
	StreamPreparedChat(context.Context, PreparedChatRequest, EventSink) (*ChatResult, error)
}

type PreparedChatPreflighter interface {
	PreflightPreparedChat(context.Context, []byte) (*FileInputMetrics, error)
}
