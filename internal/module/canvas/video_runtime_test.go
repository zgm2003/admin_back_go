package canvas

import (
	"context"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/secretbox"
	aibilling "admin_back_go/internal/module/ai/billing"
	"admin_back_go/internal/shared/enum"
)

type fakeVideoRepository struct{ agent *VideoAgentRuntime }

func (f *fakeVideoRepository) AgentForVideoRuntime(ctx context.Context, agentID int64) (*VideoAgentRuntime, error) {
	return f.agent, nil
}

type fakeVideoEngineFactory struct {
	engine *fakeVideoEngine
	input  VideoEngineConfig
}

func (f *fakeVideoEngineFactory) NewVideoEngine(ctx context.Context, input VideoEngineConfig) (infraai.VideoEngine, error) {
	f.input = input
	return f.engine, nil
}

type fakeVideoEngine struct {
	createInput infraai.VideoInput
	createTask  *infraai.VideoTask
	statusTask  *infraai.VideoTask
	statusID    string
	contentID   string
}

func (f *fakeVideoEngine) CreateVideo(ctx context.Context, input infraai.VideoInput) (*infraai.VideoTask, error) {
	f.createInput = input
	return f.createTask, nil
}
func (f *fakeVideoEngine) GetVideo(ctx context.Context, taskID string) (*infraai.VideoTask, error) {
	f.statusID = taskID
	return f.statusTask, nil
}
func (f *fakeVideoEngine) DownloadVideo(ctx context.Context, taskID string) ([]byte, string, error) {
	f.contentID = taskID
	return []byte("video"), "video/mp4", nil
}

func validVideoRuntimeAgent(t *testing.T, box secretbox.Box) *VideoAgentRuntime {
	t.Helper()
	cipher, err := box.Encrypt("provider-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	return &VideoAgentRuntime{AgentID: 8, ProviderID: 9, ModelID: "grok-imagine-video", ScenesJSON: `["image_generate"]`, EngineType: string(infraai.EngineTypeOpenAI), EngineBaseURL: "https://api.openai.test/v1", EngineAPIKeyEnc: cipher, AgentStatus: enum.CommonYes, EngineStatus: enum.CommonYes}
}

func TestVideoRuntimeCreateUsesAgentModelAndReturnsProviderTask(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	engine := &fakeVideoEngine{createTask: &infraai.VideoTask{ID: "task-1", Status: "running"}}
	factory := &fakeVideoEngineFactory{engine: engine}
	svc := NewVideoRuntimeService(VideoRuntimeDependencies{Repository: &fakeVideoRepository{agent: validVideoRuntimeAgent(t, box)}, Secretbox: box, EngineFactory: factory})

	result, appErr := svc.Create(context.Background(), VideoCreateInput{UserID: 7, AgentID: 8, ModelID: "override-video", Prompt: "clip", DurationSeconds: 4, Size: "1280x720", ResolutionName: "720p"})

	if appErr != nil {
		t.Fatalf("Create error=%#v", appErr)
	}
	if result.ProviderTaskID != "task-1" || result.ProviderID != 9 || result.ModelID != "grok-imagine-video" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if factory.input.APIKey != "provider-key" || engine.createInput.Model != "grok-imagine-video" || engine.createInput.DurationSeconds != 4 || engine.createInput.Size != "1280x720" {
		t.Fatalf("engine input mismatch factory=%#v create=%#v", factory.input, engine.createInput)
	}
}

func TestVideoRuntimeStatusAndContentUseBillingRecordProviderTaskID(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	engine := &fakeVideoEngine{statusTask: &infraai.VideoTask{ID: "provider-task-1", Status: "completed"}}
	svc := NewVideoRuntimeService(VideoRuntimeDependencies{Repository: &fakeVideoRepository{agent: validVideoRuntimeAgent(t, box)}, Secretbox: box, EngineFactory: &fakeVideoEngineFactory{engine: engine}})
	record := &aibilling.BillingRecord{ID: 77, UserID: 7, AgentID: 8, ProviderTaskID: "provider-task-1"}

	status, appErr := svc.Status(context.Background(), VideoStatusInput{UserID: 7, BillingRecord: record})
	if appErr != nil || status.Status != "completed" || engine.statusID != "provider-task-1" {
		t.Fatalf("status mismatch status=%#v id=%q err=%#v", status, engine.statusID, appErr)
	}

	body, contentType, appErr := svc.Content(context.Background(), VideoContentInput{UserID: 7, BillingRecord: record})
	if appErr != nil || string(body) != "video" || contentType != "video/mp4" || engine.contentID != "provider-task-1" {
		t.Fatalf("content mismatch body=%q type=%q id=%q err=%#v", string(body), contentType, engine.contentID, appErr)
	}
}
