package replycommand

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const MaxDeliveryChunkBytes = 16 * 1024

var ErrDeliveryTransactionRequired = errors.New("delivery prefix requires an active transaction")

type DeliveryCleaner interface {
	DeleteDeliveryChunks(context.Context, uint64, int) (int64, error)
}

func CleanupDeliveryChunks(ctx context.Context, cleaner DeliveryCleaner, commandID uint64, maxBatches int) error {
	if cleaner == nil || commandID == 0 || maxBatches <= 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for batch := 0; batch < maxBatches; batch++ {
		deleted, err := cleaner.DeleteDeliveryChunks(ctx, commandID, 256)
		if err != nil {
			return err
		}
		if deleted < 256 {
			return nil
		}
	}
	return nil
}

type AppendDeliveryChunkInput struct {
	CommandID uint64
	Owner     string
	Token     uint64
	Delta     string
	Now       time.Time
}

type AppendDeliveryChunkResult struct {
	DeliverySeq uint32
	Committed   bool
}

type DeliveryPrefix struct {
	StopDeliverySeq uint32
	Content         string
	Consistent      bool
}

func (r *GormRepository) AppendDeliveryChunk(ctx context.Context, input AppendDeliveryChunkInput) (AppendDeliveryChunkResult, error) {
	if r == nil || r.db == nil {
		return AppendDeliveryChunkResult{}, ErrRepositoryNotConfigured
	}
	input.Owner = strings.TrimSpace(input.Owner)
	if input.CommandID == 0 || input.Owner == "" || input.Token == 0 || input.Now.IsZero() || !validDeliveryDelta(input.Delta) {
		return AppendDeliveryChunkResult{}, ErrCreateInputInvalid
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var result AppendDeliveryChunkResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var command Command
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "state", "lease_owner", "lease_token", "lease_expires_at", "cancel_requested_at", "delivery_seq").
			Where("id = ?", input.CommandID).
			First(&command).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if command.State != StateRunning || command.LeaseOwner == nil || *command.LeaseOwner != input.Owner ||
			command.LeaseToken != input.Token || command.LeaseExpiresAt == nil || !command.LeaseExpiresAt.After(input.Now) ||
			command.CancelRequestedAt != nil {
			return nil
		}

		next := command.DeliverySeq + 1
		if next == 0 {
			return ErrCreateInputInvalid
		}
		updated := tx.Model(&Command{}).
			Where("id = ?", command.ID).
			UpdateColumn("delivery_seq", next)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return errPublishLeaseLost
		}
		chunk := DeliveryChunk{
			CommandID:   command.ID,
			DeliverySeq: next,
			Delta:       input.Delta,
			CreatedAt:   input.Now,
		}
		if err := tx.Create(&chunk).Error; err != nil {
			return err
		}
		result = AppendDeliveryChunkResult{DeliverySeq: next, Committed: true}
		return nil
	})
	if err != nil {
		return AppendDeliveryChunkResult{}, err
	}
	return result, nil
}

func (r *GormRepository) ReadDeliveryPrefixTx(ctx context.Context, tx *gorm.DB, commandID uint64, deliveredSeq uint32) (DeliveryPrefix, error) {
	if r == nil || r.db == nil {
		return DeliveryPrefix{}, ErrRepositoryNotConfigured
	}
	return ReadDeliveryPrefixInTransaction(ctx, tx, commandID, deliveredSeq)
}

func ReadDeliveryPrefixInTransaction(ctx context.Context, tx *gorm.DB, commandID uint64, deliveredSeq uint32) (DeliveryPrefix, error) {
	if tx == nil {
		return DeliveryPrefix{}, ErrDeliveryTransactionRequired
	}
	if commandID == 0 {
		return DeliveryPrefix{}, ErrCreateInputInvalid
	}
	if deliveredSeq == 0 {
		return DeliveryPrefix{Consistent: true}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var chunks []DeliveryChunk
	err := tx.WithContext(ctx).
		Model(&DeliveryChunk{}).
		Select("delivery_seq", "delta").
		Where("command_id = ? AND delivery_seq <= ?", commandID, deliveredSeq).
		Order("delivery_seq ASC").
		Find(&chunks).Error
	if err != nil {
		return DeliveryPrefix{}, err
	}
	if len(chunks) != int(deliveredSeq) {
		return DeliveryPrefix{}, nil
	}
	var content strings.Builder
	for index, chunk := range chunks {
		if chunk.DeliverySeq != uint32(index+1) {
			return DeliveryPrefix{}, nil
		}
		content.WriteString(chunk.Delta)
	}
	return DeliveryPrefix{
		StopDeliverySeq: deliveredSeq,
		Content:         content.String(),
		Consistent:      true,
	}, nil
}

func (r *GormRepository) DeleteDeliveryChunks(ctx context.Context, commandID uint64, limit int) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if commandID == 0 {
		return 0, ErrCreateInputInvalid
	}
	if limit <= 0 || limit > 256 {
		limit = 256
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := r.db.WithContext(ctx).Exec(
		"DELETE FROM ai_reply_delivery_chunks WHERE command_id = ? ORDER BY delivery_seq ASC LIMIT ?",
		commandID,
		limit,
	)
	return result.RowsAffected, result.Error
}

func validDeliveryDelta(delta string) bool {
	return delta != "" && utf8.ValidString(delta) && len(delta) <= MaxDeliveryChunkBytes
}
