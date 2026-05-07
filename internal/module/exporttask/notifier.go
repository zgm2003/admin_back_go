package exporttask

import (
	"context"
	"fmt"
	"strings"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/module/notificationtask"
)

type NotificationTaskCreator interface {
	Create(ctx context.Context, input notificationtask.CreateInput) (*notificationtask.CreateResponse, *apperror.Error)
}

type NotificationTaskNotifier struct {
	creator NotificationTaskCreator
}

func NewNotificationTaskNotifier(creator NotificationTaskCreator) *NotificationTaskNotifier {
	return &NotificationTaskNotifier{creator: creator}
}

func (n *NotificationTaskNotifier) NotifyExportSuccess(ctx context.Context, input NotifyInput) error {
	return n.create(ctx, input, enum.NotificationTypeSuccess, "导出任务已完成", fmt.Sprintf("%s 已生成，可前往导出任务下载。", exportTitle(input)))
}

func (n *NotificationTaskNotifier) NotifyExportFailed(ctx context.Context, input NotifyInput) error {
	content := fmt.Sprintf("%s 生成失败。", exportTitle(input))
	if msg := strings.TrimSpace(input.ErrorMsg); msg != "" {
		content = content + "原因：" + msg
	}
	return n.create(ctx, input, enum.NotificationTypeError, "导出任务失败", content)
}

func (n *NotificationTaskNotifier) create(ctx context.Context, input NotifyInput, notificationType int, title string, content string) error {
	if n == nil || n.creator == nil {
		return nil
	}
	if input.UserID <= 0 {
		return nil
	}
	platform := strings.TrimSpace(input.Platform)
	if platform == "" {
		platform = enum.PlatformAdmin
	}
	_, appErr := n.creator.Create(ctx, notificationtask.CreateInput{
		Title:      title,
		Content:    content,
		Type:       notificationType,
		Level:      enum.NotificationLevelUrgent,
		Link:       input.Link,
		Platform:   platform,
		TargetType: enum.NotificationTargetUsers,
		TargetIDs:  []int64{input.UserID},
		CreatedBy:  input.UserID,
	})
	if appErr != nil {
		return appErr
	}
	return nil
}

func exportTitle(input NotifyInput) string {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return "导出任务"
	}
	return title
}
