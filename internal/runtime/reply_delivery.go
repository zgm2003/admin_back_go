package runtime

import (
	"context"

	aichat "admin_back_go/internal/module/ai/chat"
	"admin_back_go/internal/module/ai/replycommand"
)

type replyDeliveryRepository interface {
	AppendDeliveryChunk(context.Context, replycommand.AppendDeliveryChunkInput) (replycommand.AppendDeliveryChunkResult, error)
}

type replyDeliveryCommitter struct {
	repository replyDeliveryRepository
}

func (p replyDeliveryCommitter) CommitDelivery(ctx context.Context, input aichat.DeliveryCommit) (uint32, bool, error) {
	result, err := p.repository.AppendDeliveryChunk(ctx, replycommand.AppendDeliveryChunkInput{
		CommandID: input.CommandID,
		Owner:     input.Owner,
		Token:     input.Token,
		Delta:     input.Delta,
		Now:       input.Now,
	})
	return result.DeliverySeq, result.Committed, err
}

var _ aichat.DeliveryCommitter = replyDeliveryCommitter{}
