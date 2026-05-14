package crontask

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	projecti18n "admin_back_go/internal/i18n"

	"github.com/gin-gonic/gin"
)

func TestCronTaskCatalogKeysMatch(t *testing.T) {
	want := []string{
		"crontask.service_missing",
		"crontask.repository_missing",
		"crontask.list.request.invalid",
		"crontask.save.request.invalid",
		"crontask.status.invalid",
		"crontask.delete.empty",
		"crontask.logs.request.invalid",
		"crontask.id.invalid",
		"crontask.query_failed",
		"crontask.name_check_failed",
		"crontask.name.duplicate",
		"crontask.create_failed",
		"crontask.name.immutable",
		"crontask.update_failed",
		"crontask.status_update_failed",
		"crontask.delete_failed",
		"crontask.logs_query_failed",
		"crontask.current_page.invalid",
		"crontask.page_size.invalid",
		"crontask.registry_status.invalid",
		"crontask.log_status.invalid",
		"crontask.name.invalid",
		"crontask.title.invalid",
		"crontask.description.too_long",
		"crontask.cron.invalid",
		"crontask.handler.too_long",
		"crontask.not_found",
	}

	for _, lang := range []string{"zh-CN", "en-US"} {
		keys, err := projecti18n.CatalogKeys(lang)
		if err != nil {
			t.Fatalf("load %s keys: %v", lang, err)
		}
		for _, key := range want {
			if _, ok := keys[key]; !ok {
				t.Fatalf("%s missing key %q", lang, key)
			}
		}
	}
}

func TestCronTaskServiceErrorsUseMessageIDs(t *testing.T) {
	ctx := context.Background()

	_, appErr := NewService(nil, NewDefaultRegistry()).List(ctx, ListQuery{CurrentPage: 1, PageSize: 20})
	assertCronTaskMessageID(t, appErr, "crontask.repository_missing")

	_, appErr = NewService(&fakeRepository{err: errors.New("db down")}, NewDefaultRegistry()).List(ctx, ListQuery{CurrentPage: 1, PageSize: 20})
	assertCronTaskMessageID(t, appErr, "crontask.query_failed")

	_, appErr = NewService(&fakeRepository{err: errors.New("db down")}, NewDefaultRegistry()).Create(ctx, validCronTaskInput())
	assertCronTaskMessageID(t, appErr, "crontask.name_check_failed")

	_, appErr = NewService(&fakeRepository{nameExists: true}, NewDefaultRegistry()).Create(ctx, validCronTaskInput())
	assertCronTaskMessageID(t, appErr, "crontask.name.duplicate")

	_, appErr = NewService(&failingCreateRepository{createErr: errors.New("insert failed")}, NewDefaultRegistry()).Create(ctx, validCronTaskInput())
	assertCronTaskMessageID(t, appErr, "crontask.create_failed")

	appErr = NewService(&fakeRepository{}, NewDefaultRegistry()).Update(ctx, 0, validCronTaskInput())
	assertCronTaskMessageID(t, appErr, "crontask.id.invalid")

	repo := &fakeRepository{tasks: []Task{{ID: 9, Name: "demo_task", Title: "Demo", Cron: "0 * * * * *", Status: enum.CommonYes}}}
	input := validCronTaskInput()
	input.Name = "other_task"
	appErr = NewService(repo, NewDefaultRegistry()).Update(ctx, 9, input)
	assertCronTaskMessageID(t, appErr, "crontask.name.immutable")

	appErr = NewService(&fakeRepository{}, NewDefaultRegistry()).Update(ctx, 99, validCronTaskInput())
	assertCronTaskMessageID(t, appErr, "crontask.not_found")

	appErr = NewService(&failingUpdateRepository{}, NewDefaultRegistry()).Update(ctx, 9, validCronTaskInput())
	assertCronTaskMessageID(t, appErr, "crontask.update_failed")

	appErr = NewService(&fakeRepository{}, NewDefaultRegistry()).ChangeStatus(ctx, 9, 99)
	assertCronTaskMessageID(t, appErr, "crontask.status.invalid")

	appErr = NewService(&failingStatusRepository{}, NewDefaultRegistry()).ChangeStatus(ctx, 9, enum.CommonNo)
	assertCronTaskMessageID(t, appErr, "crontask.status_update_failed")

	appErr = NewService(&fakeRepository{}, NewDefaultRegistry()).Delete(ctx, nil)
	assertCronTaskMessageID(t, appErr, "crontask.delete.empty")

	appErr = NewService(&failingDeleteRepository{}, NewDefaultRegistry()).Delete(ctx, []int64{9})
	assertCronTaskMessageID(t, appErr, "crontask.delete_failed")

	_, appErr = NewService(&fakeRepository{err: errors.New("db down")}, NewDefaultRegistry()).Logs(ctx, LogsQuery{TaskID: 1, CurrentPage: 1, PageSize: 20})
	assertCronTaskMessageID(t, appErr, "crontask.logs_query_failed")
}

func TestCronTaskNormalizeErrorsUseMessageIDs(t *testing.T) {
	status := 99
	logStatus := 99

	listCases := []struct {
		name  string
		query ListQuery
		want  string
	}{
		{name: "current page", query: ListQuery{CurrentPage: 0, PageSize: 20}, want: "crontask.current_page.invalid"},
		{name: "page size", query: ListQuery{CurrentPage: 1, PageSize: 0}, want: "crontask.page_size.invalid"},
		{name: "status", query: ListQuery{CurrentPage: 1, PageSize: 20, Status: &status}, want: "crontask.status.invalid"},
		{name: "registry status", query: ListQuery{CurrentPage: 1, PageSize: 20, RegistryStatus: "lost"}, want: "crontask.registry_status.invalid"},
	}
	for _, tt := range listCases {
		t.Run("list "+tt.name, func(t *testing.T) {
			_, appErr := normalizeListQuery(tt.query)
			assertCronTaskMessageID(t, appErr, tt.want)
		})
	}

	logCases := []struct {
		name  string
		query LogsQuery
		want  string
	}{
		{name: "task id", query: LogsQuery{TaskID: 0, CurrentPage: 1, PageSize: 20}, want: "crontask.id.invalid"},
		{name: "current page", query: LogsQuery{TaskID: 1, CurrentPage: 0, PageSize: 20}, want: "crontask.current_page.invalid"},
		{name: "page size", query: LogsQuery{TaskID: 1, CurrentPage: 1, PageSize: 0}, want: "crontask.page_size.invalid"},
		{name: "log status", query: LogsQuery{TaskID: 1, CurrentPage: 1, PageSize: 20, Status: &logStatus}, want: "crontask.log_status.invalid"},
	}
	for _, tt := range logCases {
		t.Run("logs "+tt.name, func(t *testing.T) {
			_, appErr := normalizeLogsQuery(tt.query)
			assertCronTaskMessageID(t, appErr, tt.want)
		})
	}

	saveCases := []struct {
		name   string
		mutate func(*SaveInput)
		want   string
	}{
		{name: "name", mutate: func(input *SaveInput) { input.Name = "" }, want: "crontask.name.invalid"},
		{name: "title", mutate: func(input *SaveInput) { input.Title = "" }, want: "crontask.title.invalid"},
		{name: "description", mutate: func(input *SaveInput) { input.Description = strings.Repeat("字", maxDescriptionLen+1) }, want: "crontask.description.too_long"},
		{name: "cron", mutate: func(input *SaveInput) { input.Cron = "bad" }, want: "crontask.cron.invalid"},
		{name: "handler", mutate: func(input *SaveInput) { input.Handler = strings.Repeat("a", maxHandlerLen+1) }, want: "crontask.handler.too_long"},
		{name: "status", mutate: func(input *SaveInput) { input.Status = 99 }, want: "crontask.status.invalid"},
	}
	for _, tt := range saveCases {
		t.Run("save "+tt.name, func(t *testing.T) {
			input := validCronTaskInput()
			tt.mutate(&input)
			_, appErr := normalizeSaveInput(input)
			assertCronTaskMessageID(t, appErr, tt.want)
		})
	}
}

func TestCronTaskHandlerLocalizesListRequestError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(projecti18n.Localize())
	RegisterRoutes(router, &fakeCronHTTPService{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/cron-tasks?current_page=1&page_size=20&status=bad", nil)
	request.Header.Set("Accept-Language", "en-US")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, recorder.Code, recorder.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["msg"] != "Invalid cron task list request" {
		t.Fatalf("expected localized list request error, got %#v", payload["msg"])
	}
}

func validCronTaskInput() SaveInput {
	return SaveInput{Name: "demo_task", Title: "Demo", Cron: "0 * * * * *", Status: enum.CommonYes}
}

func assertCronTaskMessageID(t *testing.T, appErr *apperror.Error, want string) {
	t.Helper()
	if appErr == nil {
		t.Fatalf("expected error %q, got nil", want)
	}
	if appErr.MessageID != want {
		t.Fatalf("expected message id %q, got %#v", want, appErr)
	}
}

type failingCreateRepository struct {
	fakeRepository
	createErr error
}

func (f *failingCreateRepository) Create(ctx context.Context, row Task) (int64, error) {
	return 0, f.createErr
}

type failingUpdateRepository struct {
	fakeRepository
}

func (f *failingUpdateRepository) Get(ctx context.Context, id int64) (*Task, error) {
	return &Task{ID: id, Name: "demo_task", Title: "Demo", Cron: "0 * * * * *", Status: enum.CommonYes}, nil
}

func (f *failingUpdateRepository) Update(ctx context.Context, id int64, row Task) error {
	return errors.New("update failed")
}

type failingStatusRepository struct {
	fakeRepository
}

func (f *failingStatusRepository) Get(ctx context.Context, id int64) (*Task, error) {
	return &Task{ID: id, Name: "demo_task", Title: "Demo", Cron: "0 * * * * *", Status: enum.CommonYes}, nil
}

func (f *failingStatusRepository) UpdateStatus(ctx context.Context, id int64, status int) error {
	return errors.New("status update failed")
}

type failingDeleteRepository struct {
	fakeRepository
}

func (f *failingDeleteRepository) Get(ctx context.Context, id int64) (*Task, error) {
	return &Task{ID: id, Name: "demo_task", Title: "Demo", Cron: "0 * * * * *", Status: enum.CommonYes}, nil
}

func (f *failingDeleteRepository) Delete(ctx context.Context, ids []int64) error {
	return errors.New("delete failed")
}
