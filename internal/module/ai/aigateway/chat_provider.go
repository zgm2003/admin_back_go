package aigateway

import (
	"context"
	"errors"
	"strings"
	"sync"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/billing"
)

// PreparedChatTransport is the minimum paid-chat provider surface. The
// prepared request API guarantees that recovery dispatches persisted bytes
// instead of rebuilding a mutable ChatInput.
type PreparedChatTransport interface {
	infraai.PreparedChatEngine
	infraai.CapabilityProvider
}

type ChatCandidateEncoder func(*infraai.ChatResult) (*string, error)

// PreparedChatProvider adapts a prepared chat transport to the Gateway
// provider contract. Create one instance per dispatch because it retains the
// rich chat result for the caller after the Gateway has persisted its billing
// evidence.
type PreparedChatProvider struct {
	transport        PreparedChatTransport
	sink             infraai.EventSink
	candidateEncoder ChatCandidateEncoder
	stopProbe        func() bool

	mu     sync.Mutex
	result *infraai.ChatResult
}

func (p *PreparedChatProvider) SetStopProbe(probe func() bool) {
	if p != nil {
		p.stopProbe = probe
	}
}

func NewPreparedChatProvider(transport PreparedChatTransport, sink infraai.EventSink, encoder ChatCandidateEncoder) *PreparedChatProvider {
	return &PreparedChatProvider{transport: transport, sink: sink, candidateEncoder: encoder}
}

func (p *PreparedChatProvider) Capabilities() infraai.CapabilityMetadata {
	if p == nil || p.transport == nil {
		return infraai.CapabilityMetadata{}
	}
	return p.transport.Capabilities()
}

func (p *PreparedChatProvider) ProvePreparedUpperBound(ctx context.Context, attempt ProviderAttempt) (PreparedUpperBoundProof, error) {
	if p == nil || p.transport == nil {
		return PreparedUpperBoundProof{}, ErrNotConfigured
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return PreparedUpperBoundProof{}, err
		}
	}
	capabilities := p.transport.Capabilities()
	if strings.TrimSpace(capabilities.SafeInputUpperBoundStrategy) != infraai.SafeInputUpperBoundStrategyUTF8RequestBytesV1 {
		return PreparedUpperBoundProof{}, gatewayError(ErrCodeInvalidPrepared, "provider safe input upper-bound strategy is unsupported", 409)
	}
	inputBound, err := infraai.SafeInputUpperBoundFromRequest(attempt.PreparedRequest)
	if err != nil {
		return PreparedUpperBoundProof{}, gatewayError(ErrCodeInvalidPrepared, err.Error(), 409)
	}
	items := make([]billing.UsageItem, len(attempt.Quote.UpperBoundItems))
	var inputItems, outputItems int
	for index, raw := range attempt.Quote.UpperBoundItems {
		item, normalizeErr := raw.Normalized()
		if normalizeErr != nil {
			return PreparedUpperBoundProof{}, gatewayError(ErrCodeInvalidPrepared, "quote contains invalid upper-bound usage", 409)
		}
		switch {
		case item.Category == billing.UsageCategoryInputText && item.Unit == "token":
			inputItems++
			if item.Quantity != inputBound {
				return PreparedUpperBoundProof{}, gatewayError(ErrCodeInvalidPrepared, "quoted input bound differs from prepared request proof", 409)
			}
		case item.Category == billing.UsageCategoryOutputText && item.Unit == "token":
			outputItems++
			if item.Quantity != int64(attempt.Quote.EffectiveMaxOutputTokens) {
				return PreparedUpperBoundProof{}, gatewayError(ErrCodeInvalidPrepared, "quoted output bound differs from effective output cap", 409)
			}
		default:
			return PreparedUpperBoundProof{}, gatewayError(ErrCodeInvalidPrepared, "prepared chat quote contains an unsupported upper-bound item", 409)
		}
		items[index] = item
	}
	if inputItems != 1 || outputItems != 1 {
		return PreparedUpperBoundProof{}, gatewayError(ErrCodeInvalidPrepared, "prepared chat quote requires one input and one output token bound", 409)
	}
	return PreparedUpperBoundProof{
		RequestSHA256: attempt.RequestSHA256,
		Strategy:      infraai.SafeInputUpperBoundStrategyUTF8RequestBytesV1,
		Items:         items,
	}, nil
}

func (p *PreparedChatProvider) Dispatch(ctx context.Context, attempt ProviderAttempt) (DispatchResult, error) {
	if p == nil || p.transport == nil {
		return DispatchResult{}, ErrNotConfigured
	}
	result, err := p.transport.StreamPreparedChat(ctx, infraai.PreparedChatRequest{
		Body:           append([]byte(nil), attempt.PreparedRequest...),
		IdempotencyKey: attempt.IdempotencyKey,
	}, p.sink)
	if err != nil {
		return DispatchResult{}, err
	}
	if result == nil {
		return DispatchResult{}, infraai.NewProviderError(infraai.ProviderOutcomeUnknown, "", errors.New("prepared chat provider returned no result"))
	}
	dispatchState := strings.TrimSpace(result.DispatchState)
	if dispatchState == "" {
		dispatchState = infraai.DispatchStateDispatched
	}
	var candidate *string
	if p.candidateEncoder != nil {
		candidate, err = p.candidateEncoder(result)
		if err != nil {
			return DispatchResult{}, err
		}
	}
	p.mu.Lock()
	p.result = cloneChatResult(result)
	p.mu.Unlock()
	terminalState := "succeeded"
	if p.stopProbe != nil && p.stopProbe() {
		terminalState = "canceled"
	}
	return DispatchResult{
		ProviderRequestID:   strings.TrimSpace(result.ProviderRequestID),
		ResponseSHA256:      result.ResponseSHA256,
		DispatchState:       dispatchState,
		TerminalState:       terminalState,
		Usage:               result.Usage,
		ResultCandidateJSON: candidate,
	}, nil
}

func (p *PreparedChatProvider) ChatResult() *infraai.ChatResult {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return cloneChatResult(p.result)
}

func cloneChatResult(result *infraai.ChatResult) *infraai.ChatResult {
	if result == nil {
		return nil
	}
	copy := *result
	copy.ToolCalls = append([]infraai.ToolCall(nil), result.ToolCalls...)
	copy.Usage.RawProviderJSON = append([]byte(nil), result.Usage.RawProviderJSON...)
	copy.Usage.Items = append([]infraai.UsageItem(nil), result.Usage.Items...)
	return &copy
}

var _ Provider = (*PreparedChatProvider)(nil)
