package exporttask

import (
	"context"
	"errors"
	"testing"

	"admin_back_go/internal/enum"
)

type fakeDataProvider struct {
	kind string
	ids  []int64
	data *FileData
	err  error
}

func (f *fakeDataProvider) BuildExportData(ctx context.Context, kind string, ids []int64) (*FileData, error) {
	f.kind = kind
	f.ids = append([]int64{}, ids...)
	return f.data, f.err
}

type fakeFileWriter struct {
	data FileData
	body []byte
	err  error
}

func (f *fakeFileWriter) Write(data FileData) ([]byte, error) {
	f.data = data
	return f.body, f.err
}

type fakeUploader struct {
	input  UploadInput
	result *UploadResult
	err    error
}

func (f *fakeUploader) Upload(ctx context.Context, input UploadInput) (*UploadResult, error) {
	f.input = input
	return f.result, f.err
}

type fakeNotifier struct {
	success NotifyInput
	failed  NotifyInput
}

func (f *fakeNotifier) NotifyExportSuccess(ctx context.Context, input NotifyInput) error {
	f.success = input
	return nil
}

func (f *fakeNotifier) NotifyExportFailed(ctx context.Context, input NotifyInput) error {
	f.failed = input
	return nil
}

func TestRunGeneratesUploadsMarksSuccessAndNotifies(t *testing.T) {
	repo := &fakeRepository{getRow: &Task{ID: 7, UserID: 9, Title: "用户列表导出", Status: enum.ExportTaskStatusPending, IsDel: enum.CommonNo}}
	provider := &fakeDataProvider{data: &FileData{Prefix: "用户列表导出", Headers: []Column{{Key: "id", Title: "ID"}}, Rows: []map[string]string{{"id": "3"}}}}
	writer := &fakeFileWriter{body: []byte("xlsx")}
	uploader := &fakeUploader{result: &UploadResult{FileName: "u.xlsx", FileURL: "https://cos/u.xlsx", FileSize: 4, RowCount: 1}}
	notifier := &fakeNotifier{}

	err := NewService(repo, WithExportDataProvider(provider), WithFileWriter(writer), WithFileUploader(uploader), WithNotifier(notifier)).Run(context.Background(), RunInput{TaskID: 7, Kind: KindUserList, UserID: 9, Platform: "admin", IDs: []int64{3}})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if provider.kind != KindUserList || len(provider.ids) != 1 || provider.ids[0] != 3 {
		t.Fatalf("unexpected provider call: kind=%s ids=%#v", provider.kind, provider.ids)
	}
	if uploader.input.TaskID != 7 || uploader.input.Prefix != "用户列表导出" || string(uploader.input.Body) != "xlsx" || uploader.input.RowCount != 1 {
		t.Fatalf("unexpected upload input: %#v", uploader.input)
	}
	if repo.successResult.FileName != "u.xlsx" || repo.successResult.FileURL != "https://cos/u.xlsx" || repo.successID != 7 {
		t.Fatalf("unexpected success mark id=%d result=%#v", repo.successID, repo.successResult)
	}
	if notifier.success.TaskID != 7 || notifier.success.UserID != 9 || notifier.success.Link != "/system/exportTask?status=2" {
		t.Fatalf("unexpected success notification: %#v", notifier.success)
	}
}

func TestRunMarksFailedAndNotifiesWhenGenerationFails(t *testing.T) {
	repo := &fakeRepository{getRow: &Task{ID: 7, UserID: 9, Title: "用户列表导出", Status: enum.ExportTaskStatusPending, IsDel: enum.CommonNo}}
	provider := &fakeDataProvider{err: errors.New("provider failed")}
	notifier := &fakeNotifier{}

	err := NewService(repo, WithExportDataProvider(provider), WithFileWriter(&fakeFileWriter{}), WithFileUploader(&fakeUploader{}), WithNotifier(notifier)).Run(context.Background(), RunInput{TaskID: 7, Kind: KindUserList, UserID: 9, IDs: []int64{3}})
	if err == nil {
		t.Fatalf("expected Run error")
	}
	if repo.markFailedID != 7 || repo.failedMessage == "" {
		t.Fatalf("expected task failure mark, got id=%d msg=%q", repo.markFailedID, repo.failedMessage)
	}
	if notifier.failed.TaskID != 7 || notifier.failed.UserID != 9 || notifier.failed.Link != "/system/exportTask?status=3" {
		t.Fatalf("unexpected failed notification: %#v", notifier.failed)
	}
}
