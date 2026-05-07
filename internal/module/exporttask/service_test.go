package exporttask

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
)

type fakeRepository struct {
	cleanedAt     time.Time
	counts        map[int]int64
	gotCount      StatusCountQuery
	rows          []Task
	total         int64
	gotList       ListQuery
	created       Task
	createdID     int64
	deletedUserID int64
	deletedIDs    []int64
	getRow        *Task
	successID     int64
	successResult SuccessResult
	markFailedID  int64
	failedMessage string
	err           error
}

func (f *fakeRepository) CleanExpired(ctx context.Context, now time.Time) error {
	f.cleanedAt = now
	return f.err
}

func (f *fakeRepository) CountByStatus(ctx context.Context, query StatusCountQuery) (map[int]int64, error) {
	f.gotCount = query
	return f.counts, f.err
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]Task, int64, error) {
	f.gotList = query
	return f.rows, f.total, f.err
}

func (f *fakeRepository) Create(ctx context.Context, row Task) (int64, error) {
	f.created = row
	if f.createdID == 0 {
		f.createdID = 101
	}
	return f.createdID, f.err
}

func (f *fakeRepository) MarkSuccess(ctx context.Context, id int64, result SuccessResult) error {
	f.successID = id
	f.successResult = result
	return f.err
}

func (f *fakeRepository) MarkFailed(ctx context.Context, id int64, message string) error {
	f.markFailedID = id
	f.failedMessage = message
	return f.err
}

func (f *fakeRepository) DeleteByUser(ctx context.Context, userID int64, ids []int64) error {
	f.deletedUserID = userID
	f.deletedIDs = append([]int64{}, ids...)
	return f.err
}

func (f *fakeRepository) Get(ctx context.Context, id int64) (*Task, error) { return f.getRow, f.err }

func TestStatusCountReturnsFixedOrderAndScopesUser(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepository{counts: map[int]int64{enum.ExportTaskStatusSuccess: 2}}
	got, appErr := NewService(repo, WithNow(func() time.Time { return now })).StatusCount(context.Background(), StatusCountQuery{
		UserID: 9,
		Title:  " 用户 ",
	})
	if appErr != nil {
		t.Fatalf("StatusCount returned error: %v", appErr)
	}
	if repo.cleanedAt != now {
		t.Fatalf("expected CleanExpired with fixed now, got %v", repo.cleanedAt)
	}
	if repo.gotCount.UserID != 9 || repo.gotCount.Title != "用户" {
		t.Fatalf("expected scoped trimmed query, got %#v", repo.gotCount)
	}
	want := []StatusCountItem{
		{Label: "处理中", Value: enum.ExportTaskStatusPending, Num: 0},
		{Label: "已完成", Value: enum.ExportTaskStatusSuccess, Num: 2},
		{Label: "失败", Value: enum.ExportTaskStatusFailed, Num: 0},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected counts:\nwant=%#v\n got=%#v", want, got)
	}
}

func TestListScopesUserAndFormatsFileSize(t *testing.T) {
	createdAt := time.Date(2026, 5, 7, 12, 30, 0, 0, time.UTC)
	expireAt := createdAt.Add(7 * 24 * time.Hour)
	fileSize := int64(2048)
	rowCount := int64(3)
	repo := &fakeRepository{rows: []Task{{
		ID: 7, UserID: 9, Title: "用户列表导出", FileName: "u.xlsx", FileURL: "https://cos/u.xlsx",
		FileSize: &fileSize, RowCount: &rowCount, Status: enum.ExportTaskStatusSuccess, ExpireAt: &expireAt, CreatedAt: createdAt,
	}}, total: 1}
	got, appErr := NewService(repo).List(context.Background(), ListQuery{
		UserID: 9, CurrentPage: 1, PageSize: 20, Status: ptrInt(enum.ExportTaskStatusSuccess), FileName: " u ",
	})
	if appErr != nil {
		t.Fatalf("List returned error: %v", appErr)
	}
	if repo.gotList.UserID != 9 || repo.gotList.FileName != "u" {
		t.Fatalf("expected scoped trimmed list query, got %#v", repo.gotList)
	}
	if got.Page.Total != 1 || got.Page.TotalPage != 1 || len(got.List) != 1 {
		t.Fatalf("unexpected page/list: %#v", got)
	}
	item := got.List[0]
	if item.FileSizeText != "2 KB" || item.StatusText != "已完成" || item.ExpireAt == nil || *item.ExpireAt != "2026-05-14 12:30:00" {
		t.Fatalf("unexpected list item: %#v", item)
	}
}

func TestDeleteNormalizesIDsAndScopesUser(t *testing.T) {
	repo := &fakeRepository{}
	appErr := NewService(repo).Delete(context.Background(), DeleteInput{UserID: 9, IDs: []int64{3, 2, 3, 0, -1}})
	if appErr != nil {
		t.Fatalf("Delete returned error: %v", appErr)
	}
	if repo.deletedUserID != 9 || !reflect.DeepEqual(repo.deletedIDs, []int64{2, 3}) {
		t.Fatalf("unexpected delete call user=%d ids=%#v", repo.deletedUserID, repo.deletedIDs)
	}
}

func TestCreatePendingCreatesSevenDayPendingTask(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepository{createdID: 88}
	got, err := NewService(repo, WithNow(func() time.Time { return now })).CreatePending(context.Background(), CreatePendingInput{UserID: 9, Title: "用户列表导出"})
	if err != nil {
		t.Fatalf("CreatePending returned error: %v", err)
	}
	if got != 88 {
		t.Fatalf("expected id 88, got %d", got)
	}
	if repo.created.UserID != 9 || repo.created.Title != "用户列表导出" || repo.created.Status != enum.ExportTaskStatusPending || repo.created.IsDel != enum.CommonNo {
		t.Fatalf("unexpected created task: %#v", repo.created)
	}
	if repo.created.ExpireAt == nil || !repo.created.ExpireAt.Equal(now.Add(7*24*time.Hour)) {
		t.Fatalf("unexpected expire_at: %#v", repo.created.ExpireAt)
	}
}

func TestMarkFailedCapsMessageAtFiveHundredRunes(t *testing.T) {
	repo := &fakeRepository{}
	message := strings.Repeat("测", 600)
	if err := NewService(repo).MarkFailed(context.Background(), 77, message); err != nil {
		t.Fatalf("MarkFailed returned error: %v", err)
	}
	if repo.markFailedID != 77 || len([]rune(repo.failedMessage)) != 500 {
		t.Fatalf("expected capped message for task 77, got id=%d runes=%d", repo.markFailedID, len([]rune(repo.failedMessage)))
	}
}

func TestListRejectsInvalidStatus(t *testing.T) {
	_, appErr := NewService(&fakeRepository{}).List(context.Background(), ListQuery{UserID: 9, CurrentPage: 1, PageSize: 20, Status: ptrInt(99)})
	if appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected bad request for invalid status, got %#v", appErr)
	}
}

func ptrInt(value int) *int { return &value }
