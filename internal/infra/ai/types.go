package ai

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type EngineType string

const (
	EngineTypeOpenAI EngineType = "openai"
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
	SupportedUsageIdentities    []UsageIdentity `json:"supported_usage_identities"`
	SafeInputUpperBoundStrategy string          `json:"safe_input_upper_bound_strategy"`
	SupportsIdempotencyHeader   bool            `json:"supports_idempotency_header"`
	SupportsCancelTask          bool            `json:"supports_cancel_task"`
}

type CapabilityProvider interface {
	Capabilities() CapabilityMetadata
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

type ChatInput struct {
	AttemptID            uint64
	IdempotencyKey       string
	AgentID              uint64
	RunID                uint64
	UserID               uint64
	UserKey              string
	Content              string
	ConversationEngineID string
	Inputs               map[string]any
	Tools                []ToolDefinition
	ToolCalls            []ToolCall
	ToolOutputs          []ToolOutput
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
}

type VideoInput struct {
	Model           string
	Prompt          string
	DurationSeconds int
	Size            string
	ResolutionName  string
	GenerateAudio   *bool
	Watermark       *bool
}

type VideoTask struct {
	ID           string
	Status       string
	ErrorMessage string
	RawResponse  map[string]any
}

type VideoEngine interface {
	CreateVideo(ctx context.Context, input VideoInput) (*VideoTask, error)
	GetVideo(ctx context.Context, taskID string) (*VideoTask, error)
	DownloadVideo(ctx context.Context, taskID string) ([]byte, string, error)
}

type AudioInput struct {
	Model          string
	Prompt         string
	Voice          string
	ResponseFormat string
	Speed          *float64
	Instructions   string
}

type AudioResult struct {
	Body        []byte
	ContentType string
}

type AudioEngine interface {
	GenerateAudio(ctx context.Context, input AudioInput) (*AudioResult, error)
}

type Event struct {
	Type      string
	DeltaText string
	Payload   map[string]any
}

type EventSink interface {
	Emit(ctx context.Context, event Event) error
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
