package contextengine

import (
	"context"
	"errors"
	"log/slog"
	"sort"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

type HistoryInvalidationService struct {
	db      *gorm.DB
	cleanup *QueueIndexCleanupEnqueuer
}

func NewHistoryInvalidationService(client *database.Client, cleanup *QueueIndexCleanupEnqueuer) *HistoryInvalidationService {
	if client == nil || cleanup == nil {
		return nil
	}
	return &HistoryInvalidationService{db: client.Gorm, cleanup: cleanup}
}

type historyInvalidationSelector struct {
	fromMessageID    uint64
	throughMessageID uint64
	messageIDs       []uint64
}

type historyInvalidationAuthority struct {
	ProfileID       uint64  `gorm:"column:profile_id"`
	IndexGeneration *uint64 `gorm:"column:index_generation"`
}

type historyCleanupBatch struct {
	conversation []ContextIndexCleanupV1
	documents    []ContextIndexCleanupV1
}

func (service *HistoryInvalidationService) InvalidateSuffixInTransaction(
	ctx context.Context, tx *gorm.DB, userID, conversationID, fromMessageID, throughMessageID int64,
) (func(context.Context), error) {
	if fromMessageID <= 0 || throughMessageID < fromMessageID {
		return nil, errors.New("history invalidation suffix is invalid")
	}
	return service.invalidateInTransaction(ctx, tx, uint64(userID), uint64(conversationID), historyInvalidationSelector{
		fromMessageID: uint64(fromMessageID), throughMessageID: uint64(throughMessageID),
	})
}

func (service *HistoryInvalidationService) InvalidateMessagesInTransaction(
	ctx context.Context, tx *gorm.DB, userID, conversationID int64, messageIDs []int64,
) (func(context.Context), error) {
	ids := make([]uint64, len(messageIDs))
	for index, id := range messageIDs {
		if id <= 0 {
			return nil, errors.New("history invalidation message identity is invalid")
		}
		ids[index] = uint64(id)
	}
	return service.invalidateInTransaction(ctx, tx, uint64(userID), uint64(conversationID), historyInvalidationSelector{messageIDs: ids})
}

func (service *HistoryInvalidationService) invalidateInTransaction(
	ctx context.Context, tx *gorm.DB, userID, conversationID uint64, selector historyInvalidationSelector,
) (func(context.Context), error) {
	if service == nil || service.db == nil || service.cleanup == nil || tx == nil || userID == 0 || conversationID == 0 {
		return nil, errors.New("history invalidation service is not configured")
	}
	var authority historyInvalidationAuthority
	err := tx.WithContext(ctx).Table("ai_conversations AS c").
		Select("a.context_profile_id AS profile_id, p.active_index_generation AS index_generation").
		Joins("JOIN ai_agents AS a ON a.id = c.agent_id AND a.is_del = ? AND a.context_profile_id IS NOT NULL", enum.CommonNo).
		Joins("JOIN ai_context_profiles AS p ON p.id = a.context_profile_id AND p.status = ?", ProfileEnabled).
		Where("c.id = ? AND c.user_id = ? AND c.is_del = ?", conversationID, userID, enum.CommonNo).
		Take(&authority).Error
	if err != nil {
		return nil, err
	}
	messageRows, err := selectedHistoryMessageRows(ctx, tx, conversationID, selector)
	if err != nil {
		return nil, err
	}
	selectedIDs := make([]uint64, 0, len(messageRows))
	hiddenUserIDs := make([]uint64, 0, len(messageRows))
	for _, row := range messageRows {
		selectedIDs = append(selectedIDs, row.ID)
		if row.Role == enum.AIMessageRoleUser {
			hiddenUserIDs = append(hiddenUserIDs, row.ID)
		}
	}
	anchors, err := affectedHistoryTurnAnchors(ctx, tx, conversationID, selectedIDs, hiddenUserIDs)
	if err != nil {
		return nil, err
	}
	cleanup := historyCleanupBatch{}
	if authority.IndexGeneration != nil && *authority.IndexGeneration > 0 && len(anchors) > 0 {
		turns, err := NewConversationRepositoryWithDB(tx).CompleteByAnchors(ctx, conversationID, userID, anchors)
		if err != nil {
			return nil, err
		}
		for _, turn := range turns {
			cleanup.conversation = append(cleanup.conversation, ContextIndexCleanupV1{
				Kind: CleanupConversationPoints, ProfileID: authority.ProfileID, IndexGeneration: *authority.IndexGeneration,
				ConversationID: conversationID, UserMessageID: turn.UserMessage.ID, SourceSHA256: turn.SourceSHA256,
			})
		}
	}
	if len(hiddenUserIDs) > 0 {
		documentCleanup, err := disableConversationDocuments(ctx, tx, authority, conversationID, hiddenUserIDs)
		if err != nil {
			return nil, err
		}
		cleanup.documents = append(cleanup.documents, documentCleanup...)
	}
	if len(anchors) > 0 {
		earliest := anchors[0]
		if err := tx.WithContext(ctx).Table("ai_conversation_memories").
			Where("conversation_id = ? AND state = ? AND through_message_id >= ?", conversationID, "ready", earliest).
			Update("state", "invalidated").Error; err != nil {
			return nil, err
		}
	}
	return func(callbackCtx context.Context) {
		service.enqueueHistoryCleanup(callbackCtx, cleanup)
	}, nil
}

type historyMessageRoleRow struct {
	ID   uint64 `gorm:"column:id"`
	Role int    `gorm:"column:role"`
}

func selectedHistoryMessageRows(ctx context.Context, tx *gorm.DB, conversationID uint64, selector historyInvalidationSelector) ([]historyMessageRoleRow, error) {
	query := tx.WithContext(ctx).Table("ai_messages").Select("id, role").
		Where("conversation_id = ? AND is_del = ?", conversationID, enum.CommonNo)
	if len(selector.messageIDs) > 0 {
		query = query.Where("id IN ?", selector.messageIDs)
	} else {
		query = query.Where("id >= ? AND id <= ?", selector.fromMessageID, selector.throughMessageID)
	}
	var rows []historyMessageRoleRow
	err := query.Order("id ASC").Find(&rows).Error
	return rows, err
}

func affectedHistoryTurnAnchors(ctx context.Context, tx *gorm.DB, conversationID uint64, selectedIDs, hiddenUserIDs []uint64) ([]uint64, error) {
	anchors := append([]uint64(nil), hiddenUserIDs...)
	if len(selectedIDs) > 0 {
		var runAnchors []uint64
		if err := tx.WithContext(ctx).Table("ai_runs").Distinct("user_message_id").
			Where("conversation_id = ? AND assistant_message_id IN ?", conversationID, selectedIDs).
			Order("user_message_id ASC").Scan(&runAnchors).Error; err != nil {
			return nil, err
		}
		anchors = append(anchors, runAnchors...)
	}
	return dedupeHistoryAnchors(anchors), nil
}

func dedupeHistoryAnchors(anchors []uint64) []uint64 {
	values := append([]uint64(nil), anchors...)
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	result := values[:0]
	for _, anchor := range values {
		if anchor == 0 || len(result) > 0 && result[len(result)-1] == anchor {
			continue
		}
		result = append(result, anchor)
	}
	return result
}

func disableConversationDocuments(ctx context.Context, tx *gorm.DB, authority historyInvalidationAuthority, conversationID uint64, sourceMessageIDs []uint64) ([]ContextIndexCleanupV1, error) {
	var rows []struct {
		VersionID uint64 `gorm:"column:version_id"`
		ProfileID uint64 `gorm:"column:profile_id"`
	}
	err := tx.WithContext(ctx).Table("ai_context_documents AS d").
		Select("v.id AS version_id, v.profile_id").
		Joins("LEFT JOIN ai_context_document_versions AS v ON v.id = d.active_version_id").
		Where("d.conversation_id = ? AND d.source_message_id IN ? AND d.status = ? AND d.deleted_at IS NULL", conversationID, sourceMessageIDs, DocumentEnabled).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	if err := tx.WithContext(ctx).Model(&ContextDocument{}).
		Where("conversation_id = ? AND source_message_id IN ? AND status = ? AND deleted_at IS NULL", conversationID, sourceMessageIDs, DocumentEnabled).
		Update("status", DocumentDisabled).Error; err != nil {
		return nil, err
	}
	if authority.IndexGeneration == nil || *authority.IndexGeneration == 0 {
		return nil, nil
	}
	cleanup := make([]ContextIndexCleanupV1, 0, len(rows))
	for _, row := range rows {
		if row.VersionID == 0 || row.ProfileID == 0 {
			continue
		}
		cleanup = append(cleanup, ContextIndexCleanupV1{
			Kind: CleanupDocumentVersionPoints, ProfileID: row.ProfileID,
			IndexGeneration: *authority.IndexGeneration, DocumentVersionID: row.VersionID,
		})
	}
	return cleanup, nil
}

func (service *HistoryInvalidationService) enqueueHistoryCleanup(ctx context.Context, cleanup historyCleanupBatch) {
	if ctx == nil {
		ctx = context.Background()
	}
	for _, payload := range append(cleanup.conversation, cleanup.documents...) {
		if err := service.cleanup.EnqueueIndexCleanup(ctx, payload); err != nil {
			slog.WarnContext(ctx, "AI history index cleanup deferred to reconciler", "cleanup_kind", payload.Kind, "profile_id", payload.ProfileID, "error", err)
		}
	}
}
