package airun

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin_back_go/internal/shared/enum"
)

func TestRecorderStartsImageRunWithoutPolymorphicSourceFields(t *testing.T) {
	repo := &fakeRecorderRepository{nextID: 9}
	svc := NewRecorder(repo, func() time.Time { return time.Date(2026, 6, 7, 1, 2, 3, 0, time.UTC) })
	id, err := svc.Start(context.Background(), StartInput{Platform: enum.PlatformCanvas, RequestID: "image-77", UserID: 5, AgentID: 8, ProviderID: 9, ModelID: "gpt-image-1", ModelDisplayName: "GPT Image", InputSnapshot: "cat"})
	if err != nil || id != 9 {
		t.Fatalf("start failed id=%d err=%v", id, err)
	}
	if repo.started.Platform != enum.PlatformCanvas || repo.started.RequestID != "image-77" {
		t.Fatalf("bad start record: %#v", repo.started)
	}
}

func TestRecorderRejectsMissingRequestID(t *testing.T) {
	svc := NewRecorder(&fakeRecorderRepository{}, time.Now)
	_, err := svc.Start(context.Background(), StartInput{Platform: enum.PlatformCanvas, UserID: 1, AgentID: 1, ProviderID: 1, ModelID: "m", InputSnapshot: "cat"})
	if err == nil {
		t.Fatalf("expected missing request id error")
	}
}

func TestRecorderCompleteStoresTokenCountsOnly(t *testing.T) {
	repo := &fakeRecorderRepository{}
	svc := NewRecorder(repo, func() time.Time { return time.Date(2026, 6, 7, 2, 0, 0, 0, time.UTC) })
	err := svc.Complete(context.Background(), CompleteInput{RunID: 9, PromptTokens: 3, CompletionTokens: 4, TotalTokens: 7, DurationMS: 1200})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if repo.completed.TotalTokens != 7 {
		t.Fatalf("bad complete record: %#v", repo.completed)
	}
}

type fakeRecorderRepository struct {
	nextID          int64
	started         StartRecord
	completed       CompleteRecord
	finished        FinishRecord
	startErr        error
}

func (f *fakeRecorderRepository) StartRun(ctx context.Context, input StartRecord) (int64, error) {
	f.started = input
	if f.startErr != nil {
		return 0, f.startErr
	}
	if f.nextID == 0 {
		return 1, nil
	}
	return f.nextID, nil
}

func (f *fakeRecorderRepository) CompleteRun(ctx context.Context, input CompleteRecord) error {
	f.completed = input
	return nil
}

func (f *fakeRecorderRepository) FinishRun(ctx context.Context, input FinishRecord) error {
	f.finished = input
	return nil
}

func TestRecorderReturnsRepositoryStartError(t *testing.T) {
	wantErr := errors.New("insert failed")
	svc := NewRecorder(&fakeRecorderRepository{startErr: wantErr}, time.Now)
	_, err := svc.Start(context.Background(), StartInput{Platform: enum.PlatformCanvas, RequestID: "image-7", UserID: 1, AgentID: 1, ProviderID: 1, ModelID: "m", InputSnapshot: "cat"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected repository error, got %v", err)
	}
}
