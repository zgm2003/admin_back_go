package airun

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"admin_back_go/internal/infra/storage"
)

type fakeInputAttachmentPreviewer struct {
	input  storage.ImagePreviewInput
	result storage.ImagePreview
	err    error
	calls  int
}

func (previewer *fakeInputAttachmentPreviewer) Preview(_ context.Context, input storage.ImagePreviewInput) (storage.ImagePreview, error) {
	previewer.calls++
	previewer.input = input
	return previewer.result, previewer.err
}

func TestInputAttachmentPreviewUsesPersistedNestedImageEvidence(t *testing.T) {
	repository := &fakeRepository{inputSnapshot: &InputSnapshotRow{
		RunID:         44,
		InputSnapshot: `{"content":"describe","meta_json":"{\"attachments\":[{\"type\":\"file\",\"object_key\":\"ai_chat_attachments/report.pdf\",\"mime_type\":\"application/pdf\",\"name\":\"report.pdf\",\"size\":12,\"etag\":\"\\\"file-v1\\\"\"},{\"type\":\"image\",\"object_key\":\"ai_chat_images/2026/08/reference.png\",\"mime_type\":\"image/png\",\"url\":\"https://private.example/reference.png\",\"name\":\"reference.png\",\"size\":342460,\"etag\":\"\\\"image-v1\\\"\"}]}"}`,
	}}
	previewer := &fakeInputAttachmentPreviewer{result: storage.ImagePreview{
		URL: "https://signed.example/reference.png?q-signature=proof", ExpiresIn: 300,
	}}

	response, appErr := NewService(repository, WithInputAttachmentPreviewer(previewer)).InputAttachmentPreview(context.Background(), 44, 2)
	if appErr != nil {
		t.Fatalf("InputAttachmentPreview returned error: %v", appErr)
	}
	if !reflect.DeepEqual(repository.inputSnapshotIDs, []int64{44}) {
		t.Fatalf("input snapshot run IDs=%v", repository.inputSnapshotIDs)
	}
	wantInput := storage.ImagePreviewInput{
		StorageProvider: "cos",
		ObjectKey:       "ai_chat_images/2026/08/reference.png",
		ETag:            `"image-v1"`,
		Size:            342460,
		MIMEType:        "image/png",
	}
	if previewer.calls != 1 || !reflect.DeepEqual(previewer.input, wantInput) {
		t.Fatalf("preview call=%d input=%+v want=%+v", previewer.calls, previewer.input, wantInput)
	}
	if response.URL != previewer.result.URL || response.ExpiresIn != 300 {
		t.Fatalf("response=%+v", response)
	}
}

func TestInputAttachmentPreviewRejectsInvalidSelection(t *testing.T) {
	for _, test := range []struct {
		name     string
		runID    int64
		ordinal  int64
		snapshot string
		status   int
	}{
		{name: "invalid run ID", runID: 0, ordinal: 1, status: http.StatusBadRequest},
		{name: "invalid ordinal", runID: 44, ordinal: 0, status: http.StatusBadRequest},
		{name: "ordinal out of range", runID: 44, ordinal: 2, snapshot: `{"attachments":[{"type":"image","object_key":"ai_chat_images/a.png","mime_type":"image/png","name":"a.png","size":1,"etag":"v1"}]}`, status: http.StatusBadRequest},
		{name: "non image attachment", runID: 44, ordinal: 1, snapshot: `{"attachments":[{"type":"file","object_key":"ai_chat_attachments/a.pdf","mime_type":"application/pdf","name":"a.pdf","size":1,"etag":"v1"}]}`, status: http.StatusBadRequest},
		{name: "missing image evidence", runID: 44, ordinal: 1, snapshot: `{"attachments":[{"type":"image","mime_type":"image/png","name":"legacy.png","size":1}]}`, status: http.StatusConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeRepository{}
			if test.snapshot != "" {
				repository.inputSnapshot = &InputSnapshotRow{RunID: 44, InputSnapshot: test.snapshot}
			}
			previewer := &fakeInputAttachmentPreviewer{}
			_, appErr := NewService(repository, WithInputAttachmentPreviewer(previewer)).InputAttachmentPreview(context.Background(), test.runID, test.ordinal)
			if appErr == nil || appErr.HTTPStatus != test.status {
				t.Fatalf("error=%v want status=%d", appErr, test.status)
			}
			if previewer.calls != 0 {
				t.Fatalf("previewer called %d times", previewer.calls)
			}
		})
	}
}

func TestInputAttachmentPreviewReportsMissingOrChangedObjectAsUnavailable(t *testing.T) {
	for _, previewErr := range []error{
		storage.ErrInvalidImagePreviewInput,
		storage.ErrConditionalObjectUnavailable,
		storage.ErrConditionalObjectVersionChanged,
	} {
		repository := &fakeRepository{inputSnapshot: &InputSnapshotRow{
			RunID:         44,
			InputSnapshot: `{"attachments":[{"type":"image","object_key":"ai_chat_images/a.png","mime_type":"image/png","name":"a.png","size":1,"etag":"v1"}]}`,
		}}
		previewer := &fakeInputAttachmentPreviewer{err: previewErr}

		_, appErr := NewService(repository, WithInputAttachmentPreviewer(previewer)).InputAttachmentPreview(context.Background(), 44, 1)
		if appErr == nil || appErr.HTTPStatus != http.StatusConflict || appErr.Code != "airun.input_attachment.preview_unavailable" || !errors.Is(appErr, previewErr) {
			t.Fatalf("error=%+v want unavailable conflict wrapping %v", appErr, previewErr)
		}
	}
}
