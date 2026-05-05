package notification

import (
	"context"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
)

type fakeRepository struct {
	rows              []Notification
	total             int64
	gotList           ListQuery
	gotUnreadUserID   int64
	gotUnreadPlatform string
	unreadCount       int64
	marked            MarkReadInput
	deleted           DeleteInput
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]Notification, int64, error) {
	f.gotList = query
	return f.rows, f.total, nil
}

func (f *fakeRepository) UnreadCount(ctx context.Context, userID int64, platform string) (int64, error) {
	f.gotUnreadUserID = userID
	f.gotUnreadPlatform = platform
	return f.unreadCount, nil
}

func (f *fakeRepository) MarkRead(ctx context.Context, input MarkReadInput) (int64, error) {
	f.marked = input
	return 1, nil
}

func (f *fakeRepository) Delete(ctx context.Context, input DeleteInput) (int64, error) {
	f.deleted = input
	return int64(len(input.IDs)), nil
}

func TestInitReturnsEnumBackedDict(t *testing.T) {
	service := NewService(&fakeRepository{})

	got, appErr := service.Init(context.Background())
	if appErr != nil {
		t.Fatalf("expected init to succeed, got %v", appErr)
	}
	if len(got.Dict.NotificationTypeArr) != 4 || got.Dict.NotificationTypeArr[0].Value != enum.NotificationTypeInfo || got.Dict.NotificationTypeArr[3].Value != enum.NotificationTypeError {
		t.Fatalf("unexpected type dict: %#v", got.Dict.NotificationTypeArr)
	}
	if len(got.Dict.NotificationLevelArr) != 2 || got.Dict.NotificationLevelArr[1].Value != enum.NotificationLevelUrgent {
		t.Fatalf("unexpected level dict: %#v", got.Dict.NotificationLevelArr)
	}
	if len(got.Dict.NotificationReadStatusArr) != 2 || got.Dict.NotificationReadStatusArr[0].Value != enum.CommonYes || got.Dict.NotificationReadStatusArr[1].Value != enum.CommonNo {
		t.Fatalf("unexpected read dict: %#v", got.Dict.NotificationReadStatusArr)
	}
}

func TestListNormalizesFiltersAndReturnsLabels(t *testing.T) {
	createdAt := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepository{
		rows: []Notification{{
			ID: 7, UserID: 12, Title: "导出完成", Content: "点击查看", Type: enum.NotificationTypeSuccess,
			Level: enum.NotificationLevelUrgent, Link: "/exports", Platform: "admin", IsRead: enum.CommonNo, CreatedAt: createdAt,
		}},
		total: 1,
	}
	service := NewService(repo)

	got, appErr := service.List(context.Background(), ListQuery{
		CurrentPage: 1,
		PageSize:    20,
		UserID:      12,
		Platform:    " admin ",
		Keyword:     " 导出 ",
		Type:        ptrInt(enum.NotificationTypeSuccess),
		Level:       ptrInt(enum.NotificationLevelUrgent),
		IsRead:      ptrInt(enum.CommonNo),
	})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	if repo.gotList.Platform != "admin" || repo.gotList.Keyword != "导出" || repo.gotList.UserID != 12 {
		t.Fatalf("unexpected normalized query: %#v", repo.gotList)
	}
	if got.Page.Total != 1 || got.Page.TotalPage != 1 || len(got.List) != 1 {
		t.Fatalf("unexpected list response: %#v", got)
	}
	item := got.List[0]
	if item.TypeText != "成功" || item.LevelText != "紧急" || item.CreatedAt != "2026-05-05 12:00:00" {
		t.Fatalf("unexpected item labels: %#v", item)
	}
}

func TestListRejectsInvalidEnumFilters(t *testing.T) {
	service := NewService(&fakeRepository{})

	_, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20, UserID: 1, Platform: "admin", Type: ptrInt(99)})
	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "无效的通知类型" {
		t.Fatalf("expected invalid type error, got %#v", appErr)
	}

	_, appErr = service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20, UserID: 1, Platform: "admin", Level: ptrInt(99)})
	if appErr == nil || appErr.Message != "无效的通知级别" {
		t.Fatalf("expected invalid level error, got %#v", appErr)
	}

	_, appErr = service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20, UserID: 1, Platform: "admin", IsRead: ptrInt(99)})
	if appErr == nil || appErr.Message != "无效的已读状态" {
		t.Fatalf("expected invalid read status error, got %#v", appErr)
	}
}

func TestUnreadCountRequiresIdentityAndScopesRepository(t *testing.T) {
	repo := &fakeRepository{unreadCount: 5}
	service := NewService(repo)

	got, appErr := service.UnreadCount(context.Background(), Identity{UserID: 12, Platform: "admin"})
	if appErr != nil {
		t.Fatalf("expected unread count to succeed, got %v", appErr)
	}
	if got.Count != 5 || repo.gotUnreadUserID != 12 || repo.gotUnreadPlatform != "admin" {
		t.Fatalf("unexpected unread count result=%#v repo=%#v", got, repo)
	}
}

func TestMarkReadNormalizesIDsAndAllowsAllWhenEmpty(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	appErr := service.MarkRead(context.Background(), Identity{UserID: 12, Platform: "admin"}, []int64{3, 3, 2, 0})
	if appErr != nil {
		t.Fatalf("expected mark read to succeed, got %v", appErr)
	}
	if repo.marked.UserID != 12 || repo.marked.Platform != "admin" || len(repo.marked.IDs) != 2 || repo.marked.IDs[0] != 2 || repo.marked.IDs[1] != 3 {
		t.Fatalf("unexpected mark input: %#v", repo.marked)
	}

	appErr = service.MarkRead(context.Background(), Identity{UserID: 12, Platform: "admin"}, nil)
	if appErr != nil {
		t.Fatalf("expected mark all read to succeed, got %v", appErr)
	}
	if len(repo.marked.IDs) != 0 {
		t.Fatalf("expected empty IDs to mean mark all, got %#v", repo.marked.IDs)
	}
}

func TestDeleteRequiresAtLeastOneIDAndNormalizes(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	appErr := service.Delete(context.Background(), Identity{UserID: 12, Platform: "admin"}, []int64{5, 5, 4, -1})
	if appErr != nil {
		t.Fatalf("expected delete to succeed, got %v", appErr)
	}
	if repo.deleted.UserID != 12 || repo.deleted.Platform != "admin" || len(repo.deleted.IDs) != 2 || repo.deleted.IDs[0] != 4 || repo.deleted.IDs[1] != 5 {
		t.Fatalf("unexpected delete input: %#v", repo.deleted)
	}

	appErr = service.Delete(context.Background(), Identity{UserID: 12, Platform: "admin"}, nil)
	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "请选择要删除的通知" {
		t.Fatalf("expected empty delete error, got %#v", appErr)
	}
}

func ptrInt(value int) *int { return &value }
