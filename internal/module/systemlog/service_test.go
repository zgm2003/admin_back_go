package systemlog

import (
	"context"
	"errors"
	"testing"

	"admin_back_go/internal/platform/logstore"
)

type fakeStore struct {
	files []logstore.FileItem
	tail  *logstore.TailResponse
	err   error
}

func (f *fakeStore) ListFiles(ctx context.Context) ([]logstore.FileItem, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.files, nil
}

func (f *fakeStore) Tail(ctx context.Context, query logstore.TailQuery) (*logstore.TailResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.tail, nil
}

func TestFilesWrapsStoreError(t *testing.T) {
	service := NewService(&fakeStore{err: errors.New("disk down")})

	if _, appErr := service.Files(context.Background()); appErr == nil || appErr.MessageID != "systemlog.files_read_failed" {
		t.Fatalf("expected keyed file list error, got %#v", appErr)
	}
}

func TestLinesMapsLogstoreErrors(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		messageID string
	}{
		{name: "invalid filename", err: logstore.ErrInvalidFilename, messageID: "systemlog.filename.invalid"},
		{name: "extension denied", err: logstore.ErrExtensionDenied, messageID: "systemlog.filename.invalid"},
		{name: "file not found", err: logstore.ErrFileNotFound, messageID: "systemlog.file_not_found"},
		{name: "read failed", err: errors.New("read failed"), messageID: "systemlog.lines_read_failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(&fakeStore{err: tt.err})

			if _, appErr := service.Lines(context.Background(), LinesQuery{Filename: "admin-api.log"}); appErr == nil || appErr.MessageID != tt.messageID {
				t.Fatalf("expected keyed lines error %q, got %#v", tt.messageID, appErr)
			}
		})
	}
}
