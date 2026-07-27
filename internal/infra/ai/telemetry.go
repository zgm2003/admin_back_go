package ai

import (
	"context"
	"sync"
	"time"

	"admin_back_go/internal/telemetry"
)

func InstrumentEngine(provider string, modality string, delegate Engine, recorder telemetry.Recorder) Engine {
	if delegate == nil {
		return nil
	}
	base := &instrumentedEngine{provider: provider, modality: modality, delegate: delegate, recorder: providerRecorder(recorder)}
	if prepared, ok := delegate.(PreparedChatEngine); ok {
		return &instrumentedPreparedEngine{instrumentedEngine: base, prepared: prepared}
	}
	return base
}

func InstrumentImageEngine(provider string, delegate ImageEngine, recorder telemetry.Recorder) ImageEngine {
	if delegate == nil {
		return nil
	}
	return &instrumentedImageEngine{provider: provider, delegate: delegate, recorder: providerRecorder(recorder)}
}

type instrumentedEngine struct {
	provider string
	modality string
	delegate Engine
	recorder telemetry.Recorder
}

type instrumentedPreparedEngine struct {
	*instrumentedEngine
	prepared PreparedChatEngine
}

func (engine *instrumentedPreparedEngine) PrepareChat(ctx context.Context, input ChatInput) ([]byte, error) {
	return engine.prepared.PrepareChat(ctx, input)
}

func (engine *instrumentedPreparedEngine) Capabilities() CapabilityMetadata {
	if engine == nil || engine.prepared == nil {
		return CapabilityMetadata{}
	}
	provider, ok := engine.prepared.(CapabilityProvider)
	if !ok {
		return CapabilityMetadata{}
	}
	return provider.Capabilities()
}

func (engine *instrumentedPreparedEngine) StreamPreparedChat(ctx context.Context, input PreparedChatRequest, sink EventSink) (result *ChatResult, err error) {
	startedAt := time.Now()
	if sink != nil {
		downstream := sink
		var firstByte sync.Once
		sink = providerEventSinkFunc(func(ctx context.Context, event Event) error {
			firstByte.Do(func() {
				engine.recorder.Observe("provider.first_byte_seconds", time.Since(startedAt).Seconds(), providerAttributes(engine.provider, engine.modality, "ok"))
			})
			return downstream.Emit(ctx, event)
		})
	}
	result, err = engine.prepared.StreamPreparedChat(ctx, input, sink)
	var tokens *providerTokens
	if result != nil {
		tokens = &providerTokens{prompt: result.PromptTokens, completion: result.CompletionTokens, total: result.TotalTokens}
	}
	recordProviderCompletion(engine.recorder, engine.provider, engine.modality, startedAt, err, tokens)
	return result, err
}

func (engine *instrumentedEngine) TestConnection(ctx context.Context, input TestConnectionInput) (result *TestConnectionResult, err error) {
	startedAt := time.Now()
	result, err = engine.delegate.TestConnection(ctx, input)
	recordProviderCompletion(engine.recorder, engine.provider, "connection", startedAt, err, nil)
	return result, err
}

func (engine *instrumentedEngine) StreamChat(ctx context.Context, input ChatInput, sink EventSink) (result *ChatResult, err error) {
	startedAt := time.Now()
	if sink != nil {
		downstream := sink
		var firstByte sync.Once
		sink = providerEventSinkFunc(func(ctx context.Context, event Event) error {
			firstByte.Do(func() {
				engine.recorder.Observe("provider.first_byte_seconds", time.Since(startedAt).Seconds(), providerAttributes(engine.provider, engine.modality, "ok"))
			})
			return downstream.Emit(ctx, event)
		})
	}
	result, err = engine.delegate.StreamChat(ctx, input, sink)
	var tokens *providerTokens
	if result != nil {
		tokens = &providerTokens{prompt: result.PromptTokens, completion: result.CompletionTokens, total: result.TotalTokens}
	}
	recordProviderCompletion(engine.recorder, engine.provider, engine.modality, startedAt, err, tokens)
	return result, err
}

type providerEventSinkFunc func(context.Context, Event) error

func (function providerEventSinkFunc) Emit(ctx context.Context, event Event) error {
	return function(ctx, event)
}

type instrumentedImageEngine struct {
	provider string
	delegate ImageEngine
	recorder telemetry.Recorder
}

func (engine *instrumentedImageEngine) GenerateImages(ctx context.Context, input ImageInput) (result *ImageResult, err error) {
	startedAt := time.Now()
	result, err = engine.delegate.GenerateImages(ctx, input)
	var tokens *providerTokens
	if result != nil {
		tokens = &providerTokens{prompt: result.PromptTokens, completion: result.CompletionTokens, total: result.TotalTokens}
	}
	recordProviderCompletion(engine.recorder, engine.provider, "image", startedAt, err, tokens)
	return result, err
}

type providerTokens struct {
	prompt     int
	completion int
	total      int
}

func recordProviderCompletion(recorder telemetry.Recorder, provider string, modality string, startedAt time.Time, err error, tokens *providerTokens) {
	status := "ok"
	if err != nil {
		status = "error"
	}
	attributes := providerAttributes(provider, modality, status)
	recorder.Count("provider.requests", 1, attributes)
	recorder.Observe("provider.total_seconds", time.Since(startedAt).Seconds(), attributes)
	if tokens == nil {
		return
	}
	recorder.Observe("provider.prompt_tokens", float64(tokens.prompt), attributes)
	recorder.Observe("provider.completion_tokens", float64(tokens.completion), attributes)
	recorder.Observe("provider.total_tokens", float64(tokens.total), attributes)
}

func providerAttributes(provider string, modality string, status string) telemetry.Attributes {
	return telemetry.Attributes{
		"provider.name":     provider,
		"provider.modality": modality,
		"provider.status":   status,
	}
}

func providerRecorder(recorder telemetry.Recorder) telemetry.Recorder {
	if recorder == nil {
		return telemetry.Noop()
	}
	return recorder
}

var (
	_ Engine             = (*instrumentedEngine)(nil)
	_ PreparedChatEngine = (*instrumentedPreparedEngine)(nil)
	_ CapabilityProvider = (*instrumentedPreparedEngine)(nil)
	_ ImageEngine        = (*instrumentedImageEngine)(nil)
)
