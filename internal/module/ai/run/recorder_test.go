package airun

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin_back_go/internal/shared/enum"
)

func TestRecorderStartsUnifiedImageRun(t *testing.T) {
	repo := &fakeRecorderRepository{nextID: 9}
	svc := NewRecorder(repo, func() time.Time { return time.Date(2026, 6, 7, 1, 2, 3, 0, time.UTC) })
	sourceID := uint64(77)
	id, err := svc.Start(context.Background(), StartInput{Platform: enum.PlatformCanvas, Modality: enum.AIRunModalityImage, SourceType: enum.AIRunSourceImageTask, SourceID: sourceID, RequestID: "image-77", UserID: 5, AgentID: 8, ProviderID: 9, ModelID: "gpt-image-1", ModelDisplayName: "GPT Image", InputSnapshot: "cat"})
	if err != nil || id != 9 {
		t.Fatalf("start failed id=%d err=%v", id, err)
	}
	if repo.started.Platform != enum.PlatformCanvas || repo.started.SourceID != 77 || repo.started.UsageStatus != enum.AIRunUsagePending {
		t.Fatalf("bad start record: %#v", repo.started)
	}
}

func TestRecorderRejectsMissingSourceType(t *testing.T) {
	svc := NewRecorder(&fakeRecorderRepository{}, time.Now)
	_, err := svc.Start(context.Background(), StartInput{Platform: enum.PlatformCanvas, Modality: enum.AIRunModalityImage, UserID: 1, AgentID: 1, ProviderID: 1, ModelID: "m", InputSnapshot: "cat"})
	if err == nil {
		t.Fatalf("expected missing source type error")
	}
}

func TestRecorderRejectsZeroSourceID(t *testing.T) {
	svc := NewRecorder(&fakeRecorderRepository{}, time.Now)
	_, err := svc.Start(context.Background(), StartInput{Platform: enum.PlatformCanvas, Modality: enum.AIRunModalityImage, SourceType: enum.AIRunSourceImageTask, SourceID: 0, UserID: 1, AgentID: 1, ProviderID: 1, ModelID: "m", InputSnapshot: "cat"})
	if err == nil {
		t.Fatalf("expected zero source id error")
	}
}

func TestRecorderCompleteKeepsTokenUsageFlag(t *testing.T) {
	repo := &fakeRecorderRepository{}
	svc := NewRecorder(repo, func() time.Time { return time.Date(2026, 6, 7, 2, 0, 0, 0, time.UTC) })
	err := svc.Complete(context.Background(), CompleteInput{RunID: 9, PromptTokens: 3, CompletionTokens: 4, TotalTokens: 7, UsageStatus: enum.AIRunUsageReported, DurationMS: 1200})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if repo.completed.TotalTokens != 7 || repo.completed.UsageStatus != enum.AIRunUsageReported {
		t.Fatalf("bad complete record: %#v", repo.completed)
	}
}

func TestRecorderRejectsTerminalPendingUsage(t *testing.T) {
	svc := NewRecorder(&fakeRecorderRepository{}, time.Now)
	err := svc.Complete(context.Background(), CompleteInput{RunID: 9, UsageStatus: enum.AIRunUsagePending})
	if err == nil {
		t.Fatalf("expected pending terminal usage error")
	}
}

func TestRecorderCompletesSourceRun(t *testing.T) {
	repo := &fakeRecorderRepository{}
	svc := NewRecorder(repo, func() time.Time { return time.Date(2026, 6, 7, 3, 0, 0, 0, time.UTC) })

	err := svc.CompleteSource(context.Background(), CompleteSourceInput{SourceType: enum.AIRunSourceCanvasVideoTask, SourceID: 77, UsageStatus: enum.AIRunUsageUnavailable, DurationMS: 1200})

	if err != nil {
		t.Fatalf("complete source failed: %v", err)
	}
	if repo.completedSource.SourceType != enum.AIRunSourceCanvasVideoTask || repo.completedSource.SourceID != 77 || repo.completedSource.UsageStatus != enum.AIRunUsageUnavailable {
		t.Fatalf("bad source complete record: %#v", repo.completedSource)
	}
}

type fakeRecorderRepository struct {
	nextID          int64
	started         StartRecord
	completed       CompleteRecord
	completedSource CompleteSourceRecord
	finished        FinishRecord
	finishedSource  FinishSourceRecord
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

func (f *fakeRecorderRepository) CompleteRunBySource(ctx context.Context, input CompleteSourceRecord) error {
	f.completedSource = input
	return nil
}

func (f *fakeRecorderRepository) FinishRunBySource(ctx context.Context, input FinishSourceRecord) error {
	f.finishedSource = input
	return nil
}

func TestRecorderReturnsRepositoryStartError(t *testing.T) {
	wantErr := errors.New("insert failed")
	svc := NewRecorder(&fakeRecorderRepository{startErr: wantErr}, time.Now)
	_, err := svc.Start(context.Background(), StartInput{Platform: enum.PlatformCanvas, Modality: enum.AIRunModalityImage, SourceType: enum.AIRunSourceImageTask, SourceID: 7, UserID: 1, AgentID: 1, ProviderID: 1, ModelID: "m", InputSnapshot: "cat"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected repository error, got %v", err)
	}
}
