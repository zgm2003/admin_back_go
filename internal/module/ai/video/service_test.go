package aivideo

import (
	"context"
	"errors"
	"regexp"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/secretbox"
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

func validCanvasVideoAgent(t *testing.T, box secretbox.Box) *AgentRuntime {
	t.Helper()
	cipher, err := box.Encrypt("provider-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	return &AgentRuntime{AgentID: 8, ProviderID: 9, ModelID: "grok-imagine-video", ScenesJSON: `["canvas_video_generate"]`, EngineType: string(infraai.EngineTypeOpenAI), EngineBaseURL: "https://api.openai.test/v1", EngineAPIKeyEnc: cipher, AgentStatus: enum.CommonYes, EngineStatus: enum.CommonYes}
}

func TestVideoTaskTableNameKeepsCanvasTable(t *testing.T) {
	if got := (VideoTask{}).TableName(); got != "canvas_video_tasks" {
		t.Fatalf("expected canvas_video_tasks table, got %q", got)
	}
}

func TestCreateUsesAgentModelCreatesLocalTaskAndStoresProviderTask(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	engine := &fakeVideoEngine{createTask: &infraai.VideoTask{ID: "provider-task-1", Status: "running"}}
	factory := &fakeEngineFactory{engine: engine}
	repo := &fakeRepository{agent: validCanvasVideoAgent(t, box)}

	result, appErr := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: factory}).Create(context.Background(), CreateInput{UserID: 7, AgentID: 8, ModelID: "client-model", Prompt: " clip ", DurationSeconds: 4, Size: "1280x720", ResolutionName: "720p"})

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
	if len(repo.updates) != 1 || repo.updates[0].userID != 7 || repo.updates[0].id != 77 || repo.updates[0].fields["provider_task_id"] != "provider-task-1" || repo.updates[0].fields["status"] != StatusRunning {
		t.Fatalf("provider task update mismatch: %#v", repo.updates)
	}
}

func TestCreateProviderFailureMarksLocalTaskFailed(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	repo := &fakeRepository{agent: validCanvasVideoAgent(t, box)}
	engine := &fakeVideoEngine{createErr: errors.New("provider down")}

	_, appErr := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: &fakeEngineFactory{engine: engine}}).Create(context.Background(), CreateInput{UserID: 7, AgentID: 8, Prompt: "clip"})

	if appErr == nil || appErr.MessageID != "canvas.ai.video.provider_failed" {
		t.Fatalf("expected provider failure error, got %#v", appErr)
	}
	if len(repo.updates) != 1 || repo.updates[0].fields["status"] != StatusFailed {
		t.Fatalf("provider failure must mark task failed, updates=%#v", repo.updates)
	}
}

func TestCreateReturnsTaskUpdateFailedWhenProviderFailureCannotMarkFailed(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	repo := &fakeRepository{agent: validCanvasVideoAgent(t, box), updateErr: errors.New("update failed")}
	engine := &fakeVideoEngine{createErr: errors.New("provider down")}

	_, appErr := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: &fakeEngineFactory{engine: engine}}).Create(context.Background(), CreateInput{UserID: 7, AgentID: 8, Prompt: "clip"})

	if appErr == nil || appErr.MessageID != "canvas.ai.video.task_update_failed" {
		t.Fatalf("expected task update failed error, got %#v", appErr)
	}
	if len(repo.updates) != 1 || repo.updates[0].fields["status"] != StatusFailed {
		t.Fatalf("provider failure must attempt failed marker, updates=%#v", repo.updates)
	}
}

func TestCreateRejectsEmptyProviderTaskID(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	repo := &fakeRepository{agent: validCanvasVideoAgent(t, box)}
	engine := &fakeVideoEngine{createTask: &infraai.VideoTask{ID: "  ", Status: "running"}}

	_, appErr := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: &fakeEngineFactory{engine: engine}}).Create(context.Background(), CreateInput{UserID: 7, AgentID: 8, Prompt: "clip"})

	if appErr == nil || appErr.MessageID != "canvas.ai.video.provider_task_invalid" {
		t.Fatalf("expected provider task invalid error, got %#v", appErr)
	}
	if len(repo.updates) != 1 || repo.updates[0].fields["status"] != StatusFailed {
		t.Fatalf("invalid provider task must mark task failed, updates=%#v", repo.updates)
	}
}

func TestCreateReturnsTaskUpdateFailedWhenInvalidProviderTaskCannotMarkFailed(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	repo := &fakeRepository{agent: validCanvasVideoAgent(t, box), updateErr: errors.New("update failed")}
	engine := &fakeVideoEngine{createTask: &infraai.VideoTask{ID: "  ", Status: "running"}}

	_, appErr := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: &fakeEngineFactory{engine: engine}}).Create(context.Background(), CreateInput{UserID: 7, AgentID: 8, Prompt: "clip"})

	if appErr == nil || appErr.MessageID != "canvas.ai.video.task_update_failed" {
		t.Fatalf("expected task update failed error, got %#v", appErr)
	}
	if len(repo.updates) != 1 || repo.updates[0].fields["status"] != StatusFailed {
		t.Fatalf("invalid provider task must attempt failed marker, updates=%#v", repo.updates)
	}
}

func TestCreateRejectsUnknownProviderStatus(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	for _, providerStatus := range []string{"paused", " "} {
		t.Run(providerStatus, func(t *testing.T) {
			repo := &fakeRepository{agent: validCanvasVideoAgent(t, box)}
			engine := &fakeVideoEngine{createTask: &infraai.VideoTask{ID: "provider-task-1", Status: providerStatus}}

			_, appErr := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: &fakeEngineFactory{engine: engine}}).Create(context.Background(), CreateInput{UserID: 7, AgentID: 8, Prompt: "clip"})

			if appErr == nil || appErr.MessageID != "canvas.ai.video.provider_status_invalid" {
				t.Fatalf("expected provider status invalid error for status %q, got %#v", providerStatus, appErr)
			}
			if len(repo.updates) != 0 {
				t.Fatalf("unknown provider status must not persist task update, updates=%#v", repo.updates)
			}
		})
	}
}

func TestStatusAndContentUseOwnedActiveTaskProviderTaskID(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	engine := &fakeVideoEngine{statusTask: &infraai.VideoTask{ID: "provider-task-1", Status: "completed"}, body: []byte("video"), contentType: "video/mp4"}
	repo := &fakeRepository{agent: validCanvasVideoAgent(t, box), task: &VideoTask{ID: 77, UserID: 7, AgentID: 8, ProviderTaskID: "provider-task-1", Status: StatusRunning, IsDel: IsDelActive}}
	svc := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: &fakeEngineFactory{engine: engine}})

	status, appErr := svc.Status(context.Background(), 7, 77)
	if appErr != nil || status == nil || status.ID != 77 || status.Status != StatusCompleted || engine.statusID != "provider-task-1" || repo.getUserID != 7 || repo.getID != 77 {
		t.Fatalf("status mismatch status=%#v id=%q repo=%#v err=%#v", status, engine.statusID, repo, appErr)
	}
	body, contentType, appErr := svc.Content(context.Background(), 7, 77)
	if appErr != nil || string(body) != "video" || contentType != "video/mp4" || engine.contentID != "provider-task-1" || repo.getUserID != 7 || repo.getID != 77 {
		t.Fatalf("content mismatch body=%q type=%q id=%q repo=%#v err=%#v", string(body), contentType, engine.contentID, repo, appErr)
	}
}

func TestStatusReturnsTaskUpdateFailedWhenPersistingProviderStatusFails(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	engine := &fakeVideoEngine{statusTask: &infraai.VideoTask{ID: "provider-task-1", Status: "completed"}}
	repo := &fakeRepository{agent: validCanvasVideoAgent(t, box), task: &VideoTask{ID: 77, UserID: 7, AgentID: 8, ProviderTaskID: "provider-task-1", Status: StatusRunning, IsDel: IsDelActive}, updateErr: errors.New("update failed")}

	_, appErr := NewService(Dependencies{Repository: repo, Secretbox: box, EngineFactory: &fakeEngineFactory{engine: engine}}).Status(context.Background(), 7, 77)

	if appErr == nil || appErr.MessageID != "canvas.ai.video.task_update_failed" {
		t.Fatalf("expected task update failed error, got %#v", appErr)
	}
	if len(repo.updates) != 1 || repo.updates[0].fields["status"] != StatusCompleted {
		t.Fatalf("status must attempt provider status persist, updates=%#v", repo.updates)
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
	if appErr.Code != apperror.CodeInternal {
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

	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.MessageID != "canvas.ai.video.agent_unavailable" {
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
