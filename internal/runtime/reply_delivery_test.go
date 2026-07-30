package runtime

import (
	"context"
	"testing"
	"time"

	aichat "admin_back_go/internal/module/ai/chat"
	"admin_back_go/internal/module/ai/replycommand"
)

type fakeReplyDeliveryRepository struct {
	input  replycommand.AppendDeliveryChunkInput
	result replycommand.AppendDeliveryChunkResult
	err    error
}

func (f *fakeReplyDeliveryRepository) AppendDeliveryChunk(_ context.Context, input replycommand.AppendDeliveryChunkInput) (replycommand.AppendDeliveryChunkResult, error) {
	f.input = input
	return f.result, f.err
}

func TestReplyDeliveryCommitterMapsFencingIdentity(t *testing.T) {
	now := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	repository := &fakeReplyDeliveryRepository{result: replycommand.AppendDeliveryChunkResult{DeliverySeq: 4, Committed: true}}
	committer := replyDeliveryCommitter{repository: repository}

	sequence, committed, err := committer.CommitDelivery(context.Background(), aichat.DeliveryCommit{
		CommandID: 41,
		Owner:     "worker-a",
		Token:     7,
		Delta:     "  你\n",
		Now:       now,
	})
	if err != nil || !committed || sequence != 4 {
		t.Fatalf("sequence=%d committed=%v err=%v", sequence, committed, err)
	}
	if repository.input.CommandID != 41 || repository.input.Owner != "worker-a" || repository.input.Token != 7 ||
		repository.input.Delta != "  你\n" || !repository.input.Now.Equal(now) {
		t.Fatalf("input=%+v", repository.input)
	}
}
