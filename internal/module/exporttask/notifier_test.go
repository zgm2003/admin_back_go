package exporttask

import (
	"context"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/module/notificationtask"
)

type fakeNotificationCreator struct {
	inputs []notificationtask.CreateInput
	err    *apperror.Error
}

func (f *fakeNotificationCreator) Create(ctx context.Context, input notificationtask.CreateInput) (*notificationtask.CreateResponse, *apperror.Error) {
	f.inputs = append(f.inputs, input)
	return &notificationtask.CreateResponse{ID: 1, Queued: true}, f.err
}

func TestNotificationTaskNotifierCreatesSuccessAndFailedTasks(t *testing.T) {
	creator := &fakeNotificationCreator{}
	notifier := NewNotificationTaskNotifier(creator)
	if err := notifier.NotifyExportSuccess(context.Background(), NotifyInput{TaskID: 7, UserID: 9, Title: "用户列表导出", Platform: enum.PlatformAdmin, Link: "/system/exportTask?status=2"}); err != nil {
		t.Fatalf("NotifyExportSuccess returned error: %v", err)
	}
	if err := notifier.NotifyExportFailed(context.Background(), NotifyInput{TaskID: 8, UserID: 9, Title: "用户列表导出", Platform: enum.PlatformAdmin, Link: "/system/exportTask?status=3", ErrorMsg: "失败"}); err != nil {
		t.Fatalf("NotifyExportFailed returned error: %v", err)
	}
	if len(creator.inputs) != 2 {
		t.Fatalf("expected two notifications, got %#v", creator.inputs)
	}
	success := creator.inputs[0]
	if success.Type != enum.NotificationTypeSuccess || success.Level != enum.NotificationLevelUrgent || success.TargetType != enum.NotificationTargetUsers || success.TargetIDs[0] != 9 || success.Link != "/system/exportTask?status=2" || success.CreatedBy != 9 {
		t.Fatalf("unexpected success notification input: %#v", success)
	}
	failed := creator.inputs[1]
	if failed.Type != enum.NotificationTypeError || failed.Level != enum.NotificationLevelUrgent || failed.TargetIDs[0] != 9 || failed.Link != "/system/exportTask?status=3" || failed.Content == "" {
		t.Fatalf("unexpected failed notification input: %#v", failed)
	}
}
