package exporttask

import (
	"context"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/enum"
	storagecos "admin_back_go/internal/platform/storage/cos"
)

type fakeUploadConfigRepository struct {
	config *UploadConfig
	err    error
}

func (f fakeUploadConfigRepository) GetEnabledConfig(ctx context.Context) (*UploadConfig, error) {
	return f.config, f.err
}

type fakeCOSWriter struct {
	input storagecos.PutInput
	err   error
}

func (f *fakeCOSWriter) Put(ctx context.Context, input storagecos.PutInput) error {
	f.input = input
	return f.err
}

type plainSecretbox struct{}

func (plainSecretbox) Decrypt(ciphertext string) (string, error) { return ciphertext, nil }

func TestCOSUploaderUploadsXLSXToExportsFolder(t *testing.T) {
	writer := &fakeCOSWriter{}
	now := time.Date(2026, 5, 7, 12, 13, 14, 0, time.UTC)
	uploader := NewCOSUploader(fakeUploadConfigRepository{config: &UploadConfig{
		Driver: enum.UploadDriverCOS, SecretIDEnc: "sid", SecretKeyEnc: "skey", Bucket: "bucket", Region: "ap-guangzhou", BucketDomain: "https://cdn.example.com",
	}}, plainSecretbox{}, writer, WithUploadNow(func() time.Time { return now }))

	got, err := uploader.Upload(context.Background(), UploadInput{TaskID: 88, Prefix: "用户列表导出", Body: []byte("xlsx"), RowCount: 3})
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}
	if got.FileName != "用户列表导出_20260507_121314_88.xlsx" || got.FileURL != "https://cdn.example.com/exports/20260507/用户列表导出_20260507_121314_88.xlsx" || got.FileSize != 4 || got.RowCount != 3 {
		t.Fatalf("unexpected upload result: %#v", got)
	}
	if !strings.HasPrefix(writer.input.Key, "exports/20260507/") {
		t.Fatalf("expected exports date key, got %q", writer.input.Key)
	}
	if writer.input.ContentType != XLSXContentType || string(writer.input.Body) != "xlsx" || writer.input.SecretID != "sid" || writer.input.SecretKey != "skey" {
		t.Fatalf("unexpected put input: %#v", writer.input)
	}
}

func TestCOSUploaderBuildsDefaultCOSURL(t *testing.T) {
	writer := &fakeCOSWriter{}
	uploader := NewCOSUploader(fakeUploadConfigRepository{config: &UploadConfig{
		Driver: enum.UploadDriverCOS, SecretIDEnc: "sid", SecretKeyEnc: "skey", Bucket: "bucket", Region: "ap-guangzhou",
	}}, plainSecretbox{}, writer, WithUploadNow(func() time.Time { return time.Date(2026, 5, 7, 1, 2, 3, 0, time.UTC) }))
	got, err := uploader.Upload(context.Background(), UploadInput{TaskID: 5, Prefix: "export", Body: []byte("xlsx"), RowCount: 1})
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}
	if got.FileURL != "https://bucket.cos.ap-guangzhou.myqcloud.com/exports/20260507/export_20260507_010203_5.xlsx" {
		t.Fatalf("unexpected default url: %q", got.FileURL)
	}
}

func TestCOSUploaderFailsWithoutEnabledConfig(t *testing.T) {
	uploader := NewCOSUploader(fakeUploadConfigRepository{}, plainSecretbox{}, &fakeCOSWriter{})
	_, err := uploader.Upload(context.Background(), UploadInput{TaskID: 1, Prefix: "export", Body: []byte("xlsx"), RowCount: 1})
	if err == nil || !strings.Contains(err.Error(), "upload config") {
		t.Fatalf("expected missing config error, got %v", err)
	}
}

func TestCOSUploaderRejectsNonCOSDriver(t *testing.T) {
	uploader := NewCOSUploader(fakeUploadConfigRepository{config: &UploadConfig{Driver: "oss"}}, plainSecretbox{}, &fakeCOSWriter{})
	_, err := uploader.Upload(context.Background(), UploadInput{TaskID: 1, Prefix: "export", Body: []byte("xlsx"), RowCount: 1})
	if err == nil || !strings.Contains(err.Error(), "only supports cos") {
		t.Fatalf("expected non-cos error, got %v", err)
	}
}
