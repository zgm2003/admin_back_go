package task

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
	projecti18n "admin_back_go/internal/shared/i18n"
)

func TestNotificationTaskCatalogKeysMatch(t *testing.T) {
	want := []string{
		"notificationtask.service_missing",
		"notificationtask.repository_missing",
		"notificationtask.status_count.request.invalid",
		"notificationtask.list.request.invalid",
		"notificationtask.create.request.invalid",
		"notificationtask.id.invalid",
		"notificationtask.status_count_failed",
		"notificationtask.query_failed",
		"notificationtask.target_count_failed",
		"notificationtask.target.invalid",
		"notificationtask.create_failed",
		"notificationtask.send_task_build_failed",
		"notificationtask.enqueue_failed",
		"notificationtask.not_found",
		"notificationtask.cancel.pending_only",
		"notificationtask.cancel_failed",
		"notificationtask.state_changed",
		"notificationtask.delete_failed",
		"notificationtask.current_page.invalid",
		"notificationtask.page_size.invalid",
		"notificationtask.status.invalid",
		"notificationtask.title.invalid",
		"notificationtask.link.too_long",
		"notificationtask.type.invalid",
		"notificationtask.level.invalid",
		"notificationtask.platform.invalid",
		"notificationtask.target_type.invalid",
		"notificationtask.target.required",
		"notificationtask.send_at.format_invalid",
		"notificationtask.send_at.past",
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

func TestNotificationTaskServiceErrorsUseMessageIDs(t *testing.T) {
	statusErrService := NewService(&fakeRepository{err: errors.New("db down")})
	_, appErr := statusErrService.StatusCount(context.Background(), StatusCountQuery{})
	assertNotificationTaskMessageID(t, appErr, "notificationtask.status_count_failed")

	listErrService := NewService(&fakeRepository{err: errors.New("db down")})
	_, appErr = listErrService.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20})
	assertNotificationTaskMessageID(t, appErr, "notificationtask.query_failed")

	_, appErr = NewService(&fakeRepository{}).List(context.Background(), ListQuery{CurrentPage: 0, PageSize: 20})
	assertNotificationTaskMessageID(t, appErr, "notificationtask.current_page.invalid")

	_, appErr = NewService(&fakeRepository{}).Create(context.Background(), CreateInput{Title: "通知", TargetType: enum.NotificationTargetRoles, CreatedBy: 1})
	assertNotificationTaskMessageID(t, appErr, "notificationtask.target.required")

	_, appErr = NewService(&fakeRepository{}).Create(context.Background(), CreateInput{Title: "通知", TargetType: enum.NotificationTargetAll})
	assertNotificationTaskMessageID(t, appErr, "auth.token.invalid_or_expired")

	enqueueErrService := NewService(
		&fakeRepository{targetCount: 1, createdID: 101},
		WithEnqueuer(&fakeEnqueuer{err: errors.New("redis down")}),
	)
	_, appErr = enqueueErrService.Create(context.Background(), CreateInput{Title: "通知", TargetType: enum.NotificationTargetAll, CreatedBy: 1})
	assertNotificationTaskMessageID(t, appErr, "notificationtask.enqueue_failed")

	appErr = NewService(nil).Cancel(context.Background(), 1)
	assertNotificationTaskMessageID(t, appErr, "notificationtask.repository_missing")

	appErr = NewService(&fakeRepository{}).Cancel(context.Background(), 0)
	assertNotificationTaskMessageID(t, appErr, "notificationtask.id.invalid")

	appErr = NewService(&fakeRepository{}).Cancel(context.Background(), 99)
	assertNotificationTaskMessageID(t, appErr, "notificationtask.not_found")

	appErr = NewService(&fakeRepository{getRow: &Task{ID: 1, Status: enum.NotificationTaskStatusSending}}).Cancel(context.Background(), 1)
	assertNotificationTaskMessageID(t, appErr, "notificationtask.cancel.pending_only")

	appErr = NewService(&fakeRepository{getRow: &Task{ID: 1, Status: enum.NotificationTaskStatusPending}}).Cancel(context.Background(), 1)
	assertNotificationTaskMessageID(t, appErr, "notificationtask.state_changed")

	appErr = NewService(&fakeRepository{getRow: &Task{ID: 2}}).Delete(context.Background(), 2)
	assertNotificationTaskMessageID(t, appErr, "notificationtask.state_changed")
}

func TestNotificationTaskNormalizeCreateInputErrorsUseMessageIDs(t *testing.T) {
	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.Local)
	base := func() CreateInput {
		return CreateInput{
			Title:      "通知",
			Type:       enum.NotificationTypeInfo,
			Level:      enum.NotificationLevelNormal,
			Platform:   enum.PlatformAll,
			TargetType: enum.NotificationTargetAll,
			CreatedBy:  1,
		}
	}
	cases := []struct {
		name   string
		mutate func(*CreateInput)
		want   string
	}{
		{name: "title", mutate: func(input *CreateInput) { input.Title = "" }, want: "notificationtask.title.invalid"},
		{name: "link", mutate: func(input *CreateInput) { input.Link = strings.Repeat("a", 501) }, want: "notificationtask.link.too_long"},
		{name: "type", mutate: func(input *CreateInput) { input.Type = 99 }, want: "notificationtask.type.invalid"},
		{name: "level", mutate: func(input *CreateInput) { input.Level = 99 }, want: "notificationtask.level.invalid"},
		{name: "platform", mutate: func(input *CreateInput) { input.Platform = "mini" }, want: "notificationtask.platform.invalid"},
		{name: "target type", mutate: func(input *CreateInput) { input.TargetType = 99 }, want: "notificationtask.target_type.invalid"},
		{name: "send_at format", mutate: func(input *CreateInput) { input.SendAt = "2026/05/14" }, want: "notificationtask.send_at.format_invalid"},
		{name: "send_at past", mutate: func(input *CreateInput) { input.SendAt = "2026-05-14 09:59:58" }, want: "notificationtask.send_at.past"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			input := base()
			tt.mutate(&input)
			_, _, appErr := normalizeCreateInput(input, now)
			assertNotificationTaskMessageID(t, appErr, tt.want)
		})
	}
}

func assertNotificationTaskMessageID(t *testing.T, appErr *apperror.Error, want string) {
	t.Helper()
	if appErr == nil {
		t.Fatalf("expected error %q, got nil", want)
	}
	if appErr.MessageID != want {
		t.Fatalf("expected message id %q, got %#v", want, appErr)
	}
}
