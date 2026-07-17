package aiaudio

import (
	"context"
	"errors"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	airun "admin_back_go/internal/module/ai/run"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

type fakeRepository struct {
	agent *AgentRuntime
	err   error
}

func (f *fakeRepository) AgentForRuntime(ctx context.Context, agentID int64) (*AgentRuntime, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.agent, nil
}

type fakeSecretbox struct {
	plain string
	err   error
}

func (f fakeSecretbox) Decrypt(cipherText string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.plain, nil
}

type fakeEngineFactory struct {
	input  EngineConfig
	engine infraai.AudioEngine
	err    error
}

func (f *fakeEngineFactory) NewAudioEngine(ctx context.Context, input EngineConfig) (infraai.AudioEngine, error) {
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	return f.engine, nil
}

type fakeAudioEngine struct {
	input infraai.AudioInput
	body  []byte
	mime  string
	err   error
}

func (f *fakeAudioEngine) GenerateAudio(ctx context.Context, input infraai.AudioInput) (*infraai.AudioResult, error) {
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	return &infraai.AudioResult{Body: f.body, ContentType: f.mime}, nil
}

type fakeRunRecorder struct {
	nextID        int64
	startInput    airun.StartInput
	completeInput airun.CompleteInput
	failInput     airun.FailInput
}

func (f *fakeRunRecorder) Start(ctx context.Context, input airun.StartInput) (int64, error) {
	f.startInput = input
	if f.nextID == 0 {
		f.nextID = 99
	}
	return f.nextID, nil
}

func (f *fakeRunRecorder) Complete(ctx context.Context, input airun.CompleteInput) error {
	f.completeInput = input
	return nil
}

func (f *fakeRunRecorder) Fail(ctx context.Context, input airun.FailInput) error {
	f.failInput = input
	return nil
}

func (f *fakeRunRecorder) Cancel(ctx context.Context, input airun.CancelInput) error   { return nil }
func (f *fakeRunRecorder) Timeout(ctx context.Context, input airun.TimeoutInput) error { return nil }

func validCanvasAudioAgent() *AgentRuntime {
	return &AgentRuntime{
		AgentID:          8,
		ProviderID:       9,
		ModelID:          "tts-1",
		ModelDisplayName: "TTS 1",
		ScenesJSON:       `["canvas_audio_generate"]`,
		EngineType:       string(infraai.EngineTypeOpenAI),
		EngineBaseURL:    "https://api.openai.test/v1",
		EngineAPIKeyEnc:  "cipher-key",
		AgentStatus:      enum.CommonYes,
		EngineStatus:     enum.CommonYes,
	}
}

func TestGenerateUsesCanvasAudioAgentAndProviderOwnedModel(t *testing.T) {
	speed := 1.25
	engine := &fakeAudioEngine{body: []byte("audio"), mime: "audio/wav"}
	factory := &fakeEngineFactory{engine: engine}
	recorder := &fakeRunRecorder{nextID: 77}

	result, appErr := NewService(Dependencies{
		Repository:    &fakeRepository{agent: validCanvasAudioAgent()},
		Secretbox:     fakeSecretbox{plain: "plain-provider-key"},
		EngineFactory: factory,
		RunRecorder:   recorder,
		Now:           func() time.Time { return time.Unix(100, 0) },
	}).Generate(context.Background(), GenerateInput{
		UserID:         7,
		AgentID:        8,
		ModelID:        "client-model-must-be-ignored",
		Prompt:         "  hello canvas  ",
		Voice:          "nova",
		ResponseFormat: "wav",
		Speed:          &speed,
		Instructions:   "  warm narration  ",
	})

	if appErr != nil {
		t.Fatalf("Generate returned error: %#v", appErr)
	}
	if string(result.Body) != "audio" || result.ContentType != "audio/wav" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if factory.input.EngineType != infraai.EngineTypeOpenAI || factory.input.BaseURL != "https://api.openai.test/v1" || factory.input.APIKey != "plain-provider-key" {
		t.Fatalf("unexpected engine config: %#v", factory.input)
	}
	if engine.input.Model != "tts-1" || engine.input.Model == "client-model-must-be-ignored" || engine.input.Prompt != "hello canvas" || engine.input.Voice != "nova" || engine.input.ResponseFormat != "wav" || engine.input.Speed == nil || *engine.input.Speed != 1.25 || engine.input.Instructions != "warm narration" {
		t.Fatalf("unexpected audio input: %#v", engine.input)
	}
	if recorder.startInput.Platform != enum.PlatformCanvas || recorder.startInput.UserID != 7 || recorder.startInput.AgentID != 8 || recorder.startInput.ProviderID != 9 || recorder.startInput.ModelID != "tts-1" || recorder.startInput.InputSnapshot != "hello canvas" {
		t.Fatalf("unexpected run start: %#v", recorder.startInput)
	}
	if recorder.completeInput.RunID != 77 {
		t.Fatalf("run must be completed, got %#v", recorder.completeInput)
	}
}

func TestGenerateRejectsNonCanvasAudioScene(t *testing.T) {
	agent := validCanvasAudioAgent()
	agent.ScenesJSON = `["canvas_text_generate"]`

	_, appErr := NewService(Dependencies{
		Repository:    &fakeRepository{agent: agent},
		Secretbox:     fakeSecretbox{plain: "plain-provider-key"},
		EngineFactory: &fakeEngineFactory{engine: &fakeAudioEngine{body: []byte("audio"), mime: "audio/mpeg"}},
		RunRecorder:   &fakeRunRecorder{},
	}).Generate(context.Background(), GenerateInput{UserID: 7, AgentID: 8, Prompt: "hello"})

	if appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest || appErr.MessageID != "canvas.ai.audio.agent_unavailable" {
		t.Fatalf("expected agent_unavailable, got %#v", appErr)
	}
}

func TestGenerateValidatesAudioParameters(t *testing.T) {
	tooFast := 4.01
	tooSlow := 0.24
	tests := []struct {
		name  string
		input GenerateInput
	}{
		{name: "missing prompt", input: GenerateInput{UserID: 7, AgentID: 8, Prompt: " "}},
		{name: "invalid voice", input: GenerateInput{UserID: 7, AgentID: 8, Prompt: "hello", Voice: "client-voice"}},
		{name: "invalid format", input: GenerateInput{UserID: 7, AgentID: 8, Prompt: "hello", ResponseFormat: "json"}},
		{name: "speed too fast", input: GenerateInput{UserID: 7, AgentID: 8, Prompt: "hello", Speed: &tooFast}},
		{name: "speed too slow", input: GenerateInput{UserID: 7, AgentID: 8, Prompt: "hello", Speed: &tooSlow}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, appErr := NewService(Dependencies{
				Repository:    &fakeRepository{agent: validCanvasAudioAgent()},
				Secretbox:     fakeSecretbox{plain: "plain-provider-key"},
				EngineFactory: &fakeEngineFactory{engine: &fakeAudioEngine{body: []byte("audio"), mime: "audio/mpeg"}},
				RunRecorder:   &fakeRunRecorder{},
			}).Generate(context.Background(), tt.input)
			if appErr == nil || appErr.MessageID != "canvas.ai.audio.request.invalid" {
				t.Fatalf("expected invalid request, got %#v", appErr)
			}
		})
	}
}

func TestGenerateFailsClosedOnProviderError(t *testing.T) {
	_, appErr := NewService(Dependencies{
		Repository:    &fakeRepository{agent: validCanvasAudioAgent()},
		Secretbox:     fakeSecretbox{plain: "plain-provider-key"},
		EngineFactory: &fakeEngineFactory{engine: &fakeAudioEngine{err: errors.New("provider down")}},
		RunRecorder:   &fakeRunRecorder{nextID: 77},
	}).Generate(context.Background(), GenerateInput{UserID: 7, AgentID: 8, Prompt: "hello"})

	if appErr == nil || appErr.MessageID != "canvas.ai.audio.provider_failed" {
		t.Fatalf("expected provider_failed, got %#v", appErr)
	}
}
