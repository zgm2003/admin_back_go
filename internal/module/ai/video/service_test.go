package aivideo

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/secretbox"
	storagecos "admin_back_go/internal/infra/storage/cos"
	airun "admin_back_go/internal/module/ai/run"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type fakeRepository struct {
	agent         *AgentRuntime
	agentID       int64
	createdTask   VideoTask
	createdTaskID int64
	updates       []updateCall
	task          *VideoTask
	getUserID     int64
	getID         int64
	updateErr     error
	uploadConfig  *UploadConfig
}

type updateCall struct {
	userID int64
	id     int64
	fields map[string]any
}

func (f *fakeRepository) AgentForRuntime(ctx context.Context, agentID int64) (*AgentRuntime, error) {
	f.agentID = agentID
	return f.agent, nil
}

func (f *fakeRepository) CreateTask(ctx context.Context, task VideoTask) (int64, error) {
	f.createdTask = task
	if f.createdTaskID > 0 {
		return f.createdTaskID, nil
	}
	return 77, nil
}

func (f *fakeRepository) UpdateTask(ctx context.Context, userID int64, id int64, fields map[string]any) error {
	f.updates = append(f.updates, updateCall{userID: userID, id: id, fields: fields})
	return f.updateErr
}

func (f *fakeRepository) GetTask(ctx context.Context, userID int64, id int64) (*VideoTask, error) {
	f.getUserID = userID
	f.getID = id
	return f.task, nil
}

func (f *fakeRepository) LoadUploadConfig(context.Context) (*UploadConfig, error) {
	return f.uploadConfig, nil
}

type fakeEngineFactory struct {
	engine infraai.VideoEngine
	input  EngineConfig
	err    error
}

func (f *fakeEngineFactory) NewVideoEngine(ctx context.Context, input EngineConfig) (infraai.VideoEngine, error) {
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	return f.engine, nil
}

type fakeVideoEngine struct {
	createInput infraai.VideoInput
	createTask  *infraai.VideoTask
	createErr   error
	statusID    string
	statusTask  *infraai.VideoTask
	statusErr   error
	contentID   string
	body        []byte
	contentType string
	contentErr  error
}

type fakeRunRecorder struct {
	nextID    int64
	started   airun.StartInput
	completed airun.CompleteInput
	failed    airun.FailInput
	canceled  airun.CancelInput
	timeout   airun.TimeoutInput
}

func (f *fakeRunRecorder) Start(ctx context.Context, input airun.StartInput) (int64, error) {
	f.started = input
	if f.nextID == 0 {
		return 1, nil
	}
	return f.nextID, nil
}

func (f *fakeRunRecorder) Complete(ctx context.Context, input airun.CompleteInput) error {
	f.completed = input
	return nil
}

func (f *fakeRunRecorder) Fail(ctx context.Context, input airun.FailInput) error {
	f.failed = input
	return nil
}

func (f *fakeRunRecorder) Cancel(ctx context.Context, input airun.CancelInput) error {
	f.canceled = input
	return nil
}

func (f *fakeRunRecorder) Timeout(ctx context.Context, input airun.TimeoutInput) error {
	f.timeout = input
	return nil
}

func (f *fakeVideoEngine) CreateVideo(ctx context.Context, input infraai.VideoInput) (*infraai.VideoTask, error) {
	f.createInput = input
	return f.createTask, f.createErr
}

func (f *fakeVideoEngine) GetVideo(ctx context.Context, taskID string) (*infraai.VideoTask, error) {
	f.statusID = taskID
	return f.statusTask, f.statusErr
}

func (f *fakeVideoEngine) DownloadVideo(ctx context.Context, taskID string) ([]byte, string, error) {
	f.contentID = taskID
	if f.contentErr != nil {
		return nil, "", f.contentErr
	}
	return f.body, f.contentType, nil
}

type fakeReferenceObjectWriter struct{ input storagecos.PutInput }

func (f *fakeReferenceObjectWriter) Put(_ context.Context, input storagecos.PutInput) error {
	f.input = input
	return nil
}

func validCanvasVideoAgent(t *testing.T, box secretbox.Box) *AgentRuntime {
	t.Helper()
	cipher, err := box.Encrypt("provider-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	return &AgentRuntime{AgentID: 8, ProviderID: 9, ModelID: "grok-imagine-video", ScenesJSON: `["canvas_video_generate"]`, EngineType: string(infraai.EngineTypeOpenAI), EngineBaseURL: "https://api.openai.test/v1", EngineAPIKeyEnc: cipher, AgentStatus: enum.CommonYes, EngineStatus: enum.CommonYes}
}

func testVideoUploadConfig(t *testing.T, box secretbox.Box) *UploadConfig {
	t.Helper()
	secretID, err := box.Encrypt("cos-secret-id")
	if err != nil {
		t.Fatalf("encrypt COS secret id: %v", err)
	}
	secretKey, err := box.Encrypt("cos-secret-key")
	if err != nil {
		t.Fatalf("encrypt COS secret key: %v", err)
	}
	return &UploadConfig{
		SettingID:    1,
		Driver:       StorageProviderCOS,
		SecretIDEnc:  secretID,
		SecretKeyEnc: secretKey,
		Bucket:       "admin-test",
		Region:       "ap-guangzhou",
		BucketDomain: "https://cos.test",
	}
}

func fixedVideoNow() func() time.Time {
	return func() time.Time { return time.Date(2026, 6, 9, 10, 11, 12, 0, time.UTC) }
}

func fixedVideoRandom(buf []byte) (int, error) {
	for i := range buf {
		buf[i] = byte(i + 1)
	}
	return len(buf), nil
}

func TestVideoTaskTableNameKeepsCanvasTable(t *testing.T) {
	if got := (VideoTask{}).TableName(); got != "canvas_video_tasks" {
		t.Fatalf("expected canvas_video_tasks table, got %q", got)
	}
}

func TestUploadReferenceMediaStoresCOSObjectAndReturnsProviderURL(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	writer := &fakeReferenceObjectWriter{}
	repo := &fakeRepository{uploadConfig: testVideoUploadConfig(t, box)}
	service := NewService(Dependencies{
		Repository:   repo,
		Secretbox:    box,
		ObjectWriter: writer,
		Now:          fixedVideoNow(),
		Random:       fixedVideoRandom,
	})

	result, appErr := service.UploadReferenceMedia(context.Background(), ReferenceMediaUploadInput{
		UserID: 7, MediaKind: "video", FileName: " reference.mp4 ", MimeType: "application/octet-stream", Body: []byte("video-bytes"),
	})

	if appErr != nil {
		t.Fatalf("upload reference media error=%#v", appErr)
	}
	if result == nil || result.StorageProvider != StorageProviderCOS || result.MediaKind != "video" || result.MimeType != "video/mp4" || result.Bytes != 11 {
		t.Fatalf("unexpected upload result: %#v", result)
	}
	if result.URL != "https://cos.test/"+result.StorageKey {
		t.Fatalf("result URL must be public COS URL, result=%#v", result)
	}
	if !strings.HasPrefix(result.StorageKey, "ai-video-references/video/2026/06/09/7-") || !strings.HasSuffix(result.StorageKey, ".mp4") {
		t.Fatalf("unexpected storage key: %q", result.StorageKey)
	}
	if writer.input.SecretID != "cos-secret-id" || writer.input.SecretKey != "cos-secret-key" || writer.input.Bucket != "admin-test" || writer.input.Region != "ap-guangzhou" {
		t.Fatalf("COS credentials/config not passed to writer: %#v", writer.input)
	}
	if writer.input.Key != result.StorageKey || string(writer.input.Body) != "video-bytes" || writer.input.ContentType != "video/mp4" {
		t.Fatalf("COS put input mismatch: %#v", writer.input)
	}
}

func TestUploadReferenceMediaRejectsInvalidInput(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	service := NewService(Dependencies{Repository: &fakeRepository{uploadConfig: testVideoUploadConfig(t, box)}, Secretbox: box, ObjectWriter: &fakeReferenceObjectWriter{}, Random: fixedVideoRandom})

	cases := []ReferenceMediaUploadInput{
		{UserID: 0, MediaKind: "video", FileName: "reference.mp4", Body: []byte("video")},
		{UserID: 7, MediaKind: "text", FileName: "reference.txt", Body: []byte("text")},
		{UserID: 7, MediaKind: "video", FileName: "empty.mp4", Body: nil},
		{UserID: 7, MediaKind: "audio", FileName: "reference.txt", MimeType: "text/plain", Body: []byte("audio")},
	}

	for _, input := range cases {
		_, appErr := service.UploadReferenceMedia(context.Background(), input)
		if appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
			t.Fatalf("expected bad request for %#v, got %#v", input, appErr)
		}
	}
}

func TestCreateUsesAgentModelCreatesLocalTaskAndStoresProviderTask(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	engine := &fakeVideoEngine{createTask: &infraai.VideoTask{ID: "provider-task-1", Status: "running"}}
	factory := &fakeEngineFactory{engine: engine}
	repo := &fakeRepository{agent: validCanvasVideoAgent(t, box)}
	recorder := &fakeRunRecorder{nextID: 99}

	generateAudio := false
	watermark := true
	result, appErr := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: factory, RunRecorder: recorder}).Create(context.Background(), CreateInput{UserID: 7, AgentID: 8, ModelID: "client-model", Prompt: " clip ", DurationSeconds: 4, Size: "1280x720", ResolutionName: "720p", GenerateAudio: &generateAudio, Watermark: &watermark})

	if appErr != nil {
		t.Fatalf("Create error=%#v", appErr)
	}
	if result == nil || result.ID != 77 || result.Status != "running" {
		t.Fatalf("unexpected create result: %#v", result)
	}
	if repo.agentID != 8 {
		t.Fatalf("expected agent lookup id=8, got %d", repo.agentID)
	}
	if repo.createdTask.UserID != 7 || repo.createdTask.AgentID != 8 || repo.createdTask.ModelID != "grok-imagine-video" || repo.createdTask.Prompt != "clip" || repo.createdTask.Status != StatusPending || repo.createdTask.IsDel != IsDelActive {
		t.Fatalf("local task mismatch: %#v", repo.createdTask)
	}
	if factory.input.APIKey != "provider-key" || factory.input.EngineType != infraai.EngineTypeOpenAI {
		t.Fatalf("engine config mismatch: %#v", factory.input)
	}
	if engine.createInput.Model != "grok-imagine-video" || engine.createInput.Prompt != "clip" || engine.createInput.DurationSeconds != 4 || engine.createInput.Size != "1280x720" || engine.createInput.ResolutionName != "720p" {
		t.Fatalf("provider input mismatch: %#v", engine.createInput)
	}
	if engine.createInput.GenerateAudio == nil || *engine.createInput.GenerateAudio || engine.createInput.Watermark == nil || !*engine.createInput.Watermark {
		t.Fatalf("provider video switches mismatch: %#v", engine.createInput)
	}
	if len(repo.updates) != 2 || repo.updates[0].userID != 7 || repo.updates[0].id != 77 || repo.updates[0].fields["run_id"] != int64(99) || repo.updates[1].userID != 7 || repo.updates[1].id != 77 || repo.updates[1].fields["provider_task_id"] != "provider-task-1" || repo.updates[1].fields["status"] != StatusRunning {
		t.Fatalf("provider task update mismatch: %#v", repo.updates)
	}
	if recorder.started.Platform != enum.PlatformCanvas || recorder.started.RequestID != "canvas_video_task_77" || recorder.started.InputSnapshot != "clip" {
		t.Fatalf("video run was not started from local task: %#v", recorder.started)
	}
	if recorder.completed.RunID != 0 {
		t.Fatalf("running provider task must not complete run yet: %#v", recorder.completed)
	}
}

func TestCreateProviderFailureMarksLocalTaskFailed(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	repo := &fakeRepository{agent: validCanvasVideoAgent(t, box)}
	engine := &fakeVideoEngine{createErr: errors.New("provider down")}
	recorder := &fakeRunRecorder{nextID: 99}

	_, appErr := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: &fakeEngineFactory{engine: engine}, RunRecorder: recorder}).Create(context.Background(), CreateInput{UserID: 7, AgentID: 8, Prompt: "clip"})

	if appErr == nil || appErr.MessageID != "canvas.ai.video.provider_failed" {
		t.Fatalf("expected provider failure error, got %#v", appErr)
	}
	if len(repo.updates) != 2 || repo.updates[0].fields["run_id"] != int64(99) || repo.updates[1].fields["status"] != StatusFailed {
		t.Fatalf("provider failure must mark task failed, updates=%#v", repo.updates)
	}
	if recorder.failed.RunID != 99 || recorder.failed.Message != "Canvas视频生成失败" {
		t.Fatalf("provider failure must fail video run: %#v", recorder.failed)
	}
}

func TestCreateReturnsTaskUpdateFailedWhenProviderFailureCannotMarkFailed(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	repo := &fakeRepository{agent: validCanvasVideoAgent(t, box), updateErr: errors.New("update failed")}
	engine := &fakeVideoEngine{createErr: errors.New("provider down")}

	_, appErr := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: &fakeEngineFactory{engine: engine}, RunRecorder: &fakeRunRecorder{nextID: 99}}).Create(context.Background(), CreateInput{UserID: 7, AgentID: 8, Prompt: "clip"})

	if appErr == nil || appErr.MessageID != "canvas.ai.video.task_update_failed" {
		t.Fatalf("expected task update failed error, got %#v", appErr)
	}
	if len(repo.updates) != 1 || repo.updates[0].fields["run_id"] != int64(99) {
		t.Fatalf("provider failure must attempt failed marker, updates=%#v", repo.updates)
	}
}

func TestCreateRejectsEmptyProviderTaskID(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	repo := &fakeRepository{agent: validCanvasVideoAgent(t, box)}
	engine := &fakeVideoEngine{createTask: &infraai.VideoTask{ID: "  ", Status: "running"}}

	_, appErr := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: &fakeEngineFactory{engine: engine}, RunRecorder: &fakeRunRecorder{nextID: 99}}).Create(context.Background(), CreateInput{UserID: 7, AgentID: 8, Prompt: "clip"})

	if appErr == nil || appErr.MessageID != "canvas.ai.video.provider_task_invalid" {
		t.Fatalf("expected provider task invalid error, got %#v", appErr)
	}
	if len(repo.updates) != 2 || repo.updates[0].fields["run_id"] != int64(99) || repo.updates[1].fields["status"] != StatusFailed {
		t.Fatalf("invalid provider task must mark task failed, updates=%#v", repo.updates)
	}
}

func TestCreateReturnsTaskUpdateFailedWhenInvalidProviderTaskCannotMarkFailed(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	repo := &fakeRepository{agent: validCanvasVideoAgent(t, box), updateErr: errors.New("update failed")}
	engine := &fakeVideoEngine{createTask: &infraai.VideoTask{ID: "  ", Status: "running"}}

	_, appErr := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: &fakeEngineFactory{engine: engine}, RunRecorder: &fakeRunRecorder{nextID: 99}}).Create(context.Background(), CreateInput{UserID: 7, AgentID: 8, Prompt: "clip"})

	if appErr == nil || appErr.MessageID != "canvas.ai.video.task_update_failed" {
		t.Fatalf("expected task update failed error, got %#v", appErr)
	}
	if len(repo.updates) != 1 || repo.updates[0].fields["run_id"] != int64(99) {
		t.Fatalf("invalid provider task must attempt failed marker, updates=%#v", repo.updates)
	}
}

func TestCreateRejectsUnknownProviderStatus(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	for _, providerStatus := range []string{"paused", " "} {
		t.Run(providerStatus, func(t *testing.T) {
			repo := &fakeRepository{agent: validCanvasVideoAgent(t, box)}
			engine := &fakeVideoEngine{createTask: &infraai.VideoTask{ID: "provider-task-1", Status: providerStatus}}

			_, appErr := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: &fakeEngineFactory{engine: engine}, RunRecorder: &fakeRunRecorder{nextID: 99}}).Create(context.Background(), CreateInput{UserID: 7, AgentID: 8, Prompt: "clip"})

			if appErr == nil || appErr.MessageID != "canvas.ai.video.provider_status_invalid" {
				t.Fatalf("expected provider status invalid error for status %q, got %#v", providerStatus, appErr)
			}
			if len(repo.updates) != 1 || repo.updates[0].fields["run_id"] != int64(99) {
				t.Fatalf("unknown provider status must not persist task update, updates=%#v", repo.updates)
			}
		})
	}
}

func TestStatusAndContentUseOwnedActiveTaskProviderTaskID(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	engine := &fakeVideoEngine{statusTask: &infraai.VideoTask{ID: "provider-task-1", Status: "completed"}, body: []byte("video"), contentType: "video/mp4"}
	repo := &fakeRepository{agent: validCanvasVideoAgent(t, box), task: &VideoTask{ID: 77, UserID: 7, AgentID: 8, ProviderTaskID: "provider-task-1", RunID: 99, Status: StatusRunning, IsDel: IsDelActive}}
	recorder := &fakeRunRecorder{}
	svc := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: &fakeEngineFactory{engine: engine}, RunRecorder: recorder})

	status, appErr := svc.Status(context.Background(), 7, 77)
	if appErr != nil || status == nil || status.ID != 77 || status.Status != StatusCompleted || engine.statusID != "provider-task-1" || repo.getUserID != 7 || repo.getID != 77 {
		t.Fatalf("status mismatch status=%#v id=%q repo=%#v err=%#v", status, engine.statusID, repo, appErr)
	}
	if recorder.completed.RunID != 99 {
		t.Fatalf("completed video status must complete bound run: %#v", recorder.completed)
	}
	body, contentType, appErr := svc.Content(context.Background(), 7, 77)
	if appErr != nil || string(body) != "video" || contentType != "video/mp4" || engine.contentID != "provider-task-1" || repo.getUserID != 7 || repo.getID != 77 {
		t.Fatalf("content mismatch body=%q type=%q id=%q repo=%#v err=%#v", string(body), contentType, engine.contentID, repo, appErr)
	}
}

func TestStatusReturnsTaskUpdateFailedWhenPersistingProviderStatusFails(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	engine := &fakeVideoEngine{statusTask: &infraai.VideoTask{ID: "provider-task-1", Status: "completed"}}
	repo := &fakeRepository{agent: validCanvasVideoAgent(t, box), task: &VideoTask{ID: 77, UserID: 7, AgentID: 8, ProviderTaskID: "provider-task-1", RunID: 99, Status: StatusRunning, IsDel: IsDelActive}, updateErr: errors.New("update failed")}

	_, appErr := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: &fakeEngineFactory{engine: engine}}).Status(context.Background(), 7, 77)

	if appErr == nil || appErr.MessageID != "canvas.ai.video.task_update_failed" {
		t.Fatalf("expected task update failed error, got %#v", appErr)
	}
	if len(repo.updates) != 1 || repo.updates[0].fields["status"] != StatusCompleted {
		t.Fatalf("status must attempt provider status persist, updates=%#v", repo.updates)
	}
}

func TestStatusCancelsVideoRunWhenProviderCancelled(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	engine := &fakeVideoEngine{statusTask: &infraai.VideoTask{ID: "provider-task-1", Status: "cancelled", ErrorMessage: "user cancelled"}}
	repo := &fakeRepository{agent: validCanvasVideoAgent(t, box), task: &VideoTask{ID: 77, UserID: 7, AgentID: 8, ProviderTaskID: "provider-task-1", RunID: 99, Status: StatusRunning, IsDel: IsDelActive}}
	recorder := &fakeRunRecorder{}

	status, appErr := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: &fakeEngineFactory{engine: engine}, RunRecorder: recorder}).Status(context.Background(), 7, 77)

	if appErr != nil || status == nil || status.Status != StatusCancelled {
		t.Fatalf("status mismatch status=%#v err=%#v", status, appErr)
	}
	if recorder.canceled.RunID != 99 || recorder.canceled.Message != "user cancelled" {
		t.Fatalf("cancelled provider status must cancel bound run: %#v", recorder.canceled)
	}
}

func TestStatusRejectsUnknownProviderStatus(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	for _, providerStatus := range []string{"paused", " "} {
		t.Run(providerStatus, func(t *testing.T) {
			engine := &fakeVideoEngine{statusTask: &infraai.VideoTask{ID: "provider-task-1", Status: providerStatus}}
			repo := &fakeRepository{agent: validCanvasVideoAgent(t, box), task: &VideoTask{ID: 77, UserID: 7, AgentID: 8, ProviderTaskID: "provider-task-1", Status: StatusRunning, IsDel: IsDelActive}}

			_, appErr := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: &fakeEngineFactory{engine: engine}}).Status(context.Background(), 7, 77)

			if appErr == nil || appErr.MessageID != "canvas.ai.video.provider_status_invalid" {
				t.Fatalf("expected provider status invalid error for status %q, got %#v", providerStatus, appErr)
			}
			if len(repo.updates) != 0 {
				t.Fatalf("unknown provider status must not persist task update, updates=%#v", repo.updates)
			}
		})
	}
}

func TestStatusRejectsTaskFromDifferentUser(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	repo := &fakeRepository{agent: validCanvasVideoAgent(t, box), task: &VideoTask{ID: 77, UserID: 99, AgentID: 8, ProviderTaskID: "provider-task-1", IsDel: IsDelActive}}

	_, appErr := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: &fakeEngineFactory{engine: &fakeVideoEngine{}}}).Status(context.Background(), 7, 77)

	if appErr == nil || appErr.MessageID != "canvas.ai.video.not_found" {
		t.Fatalf("expected ownership not_found error, got %#v", appErr)
	}
}

func TestContentRejectsMissingProviderTaskID(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	repo := &fakeRepository{agent: validCanvasVideoAgent(t, box), task: &VideoTask{ID: 77, UserID: 7, AgentID: 8, ProviderTaskID: " ", IsDel: IsDelActive}}

	_, _, appErr := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: &fakeEngineFactory{engine: &fakeVideoEngine{}}}).Content(context.Background(), 7, 77)

	if appErr == nil || appErr.MessageID != "canvas.ai.video.provider_task_missing" {
		t.Fatalf("expected missing provider task error, got %#v", appErr)
	}
	if appErr.LegacyCode != apperror.CodeInternal {
		t.Fatalf("provider task missing is stored-state inconsistency, expected code=500, got %#v", appErr)
	}
}

func TestContentRejectsEmptyProviderBody(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	repo := &fakeRepository{agent: validCanvasVideoAgent(t, box), task: &VideoTask{ID: 77, UserID: 7, AgentID: 8, ProviderTaskID: "provider-task-1", IsDel: IsDelActive}}

	_, _, appErr := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: &fakeEngineFactory{engine: &fakeVideoEngine{body: []byte{}, contentType: "video/mp4"}}}).Content(context.Background(), 7, 77)

	if appErr == nil || appErr.MessageID != "canvas.ai.video.content_empty" {
		t.Fatalf("expected empty content error, got %#v", appErr)
	}
}

func TestCreateRejectsNonCanvasVideoScene(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	agent := validCanvasVideoAgent(t, box)
	agent.ScenesJSON = `["chat"]`

	_, appErr := NewService(Dependencies{Repository: &fakeRepository{agent: agent}, Secretbox: box, EngineFactory: &fakeEngineFactory{engine: &fakeVideoEngine{}}}).Create(context.Background(), CreateInput{UserID: 7, AgentID: 8, Prompt: "clip"})

	if appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest || appErr.MessageID != "canvas.ai.video.agent_unavailable" {
		t.Fatalf("expected canvas video scene rejection, got %#v", appErr)
	}
}

func TestCreateRejectsDisabledAgentAsUnavailable(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	cipher, err := box.Encrypt("provider-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	repo, mock, closeDB := newVideoMockRepository(t)
	defer closeDB()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT a.id AS agent_id,\n\t\t\ta.provider_id AS provider_id,\n\t\t\ta.model_id AS model_id,\n\t\t\ta.model_display_name AS model_display_name,\n\t\t\ta.system_prompt AS system_prompt,\n\t\t\ta.scenes_json AS scenes_json,\n\t\t\ta.status AS agent_status,\n\t\t\te.engine_type AS engine_type,\n\t\t\te.base_url AS engine_base_url,\n\t\t\te.api_key_enc AS engine_api_key_enc,\n\t\t\te.status AS engine_status FROM ai_agents AS a JOIN ai_providers e ON e.id = a.provider_id AND e.is_del = ? WHERE a.id = ? AND a.is_del = ? LIMIT ?")).
		WithArgs(enum.CommonNo, int64(8), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"agent_id", "provider_id", "model_id", "model_display_name", "system_prompt", "scenes_json", "agent_status", "engine_type", "engine_base_url", "engine_api_key_enc", "engine_status"}).
			AddRow(int64(8), int64(9), "grok-imagine-video", "Video", "", `["canvas_video_generate"]`, enum.CommonNo, string(infraai.EngineTypeOpenAI), "https://api.openai.test/v1", cipher, enum.CommonYes))

	_, appErr := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: &fakeEngineFactory{engine: &fakeVideoEngine{}}}).Create(context.Background(), CreateInput{UserID: 7, AgentID: 8, Prompt: "clip"})

	if appErr == nil || appErr.MessageID != "canvas.ai.video.agent_unavailable" {
		t.Fatalf("expected disabled agent unavailable error, got %#v", appErr)
	}
	assertVideoMockExpectations(t, mock)
}

func TestCreateRejectsInvalidRequest(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	cases := []CreateInput{
		{UserID: 0, AgentID: 8, Prompt: "clip"},
		{UserID: 7, AgentID: 0, Prompt: "clip"},
		{UserID: 7, AgentID: 8, Prompt: "   "},
	}
	for _, input := range cases {
		_, appErr := NewService(Dependencies{Repository: &fakeRepository{}, Secretbox: box, EngineFactory: &fakeEngineFactory{}}).Create(context.Background(), input)
		if appErr == nil || appErr.MessageID != "canvas.ai.video.request.invalid" {
			t.Fatalf("expected invalid request error for %#v, got %#v", input, appErr)
		}
	}
}

func newVideoMockRepository(t *testing.T) (*GormRepository, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, SkipDefaultTransaction: false, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	return &GormRepository{db: db}, mock, func() { _ = sqlDB.Close() }
}

func assertVideoMockExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
