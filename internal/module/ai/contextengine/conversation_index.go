package contextengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/contextindex"
	"admin_back_go/internal/infra/taskqueue"
	"admin_back_go/internal/shared/enum"
	"gorm.io/gorm"
)

const TaskContextConversationIndexV1 = "ai:context-conversation-index:v1"

var ErrConversationTurnNotAuthoritative = errors.New("conversation turn is no longer authoritative")

type ContextConversationIndexV1 struct {
	ProfileID      uint64   `json:"profile_id"`
	ConversationID uint64   `json:"conversation_id"`
	UserMessageID  uint64   `json:"user_message_id"`
	SourceSHA256   [32]byte `json:"source_sha256"`
}

func (payload ContextConversationIndexV1) Validate() error {
	if payload.ProfileID == 0 || payload.ConversationID == 0 || payload.UserMessageID == 0 || payload.SourceSHA256 == ([32]byte{}) {
		return errors.New("conversation index identity is incomplete")
	}
	return nil
}

func ConversationIndexIdempotencyKey(payload ContextConversationIndexV1) (string, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	preimage := fmt.Sprintf("%s\x00%d\x00%d\x00%d\x00%s", TaskContextConversationIndexV1,
		payload.ProfileID, payload.ConversationID, payload.UserMessageID, hex.EncodeToString(payload.SourceSHA256[:]))
	digest := sha256.Sum256([]byte(preimage))
	return hex.EncodeToString(digest[:]), nil
}

type ConversationIndexWork struct {
	Profile         ContextProfile
	Platform        string
	IndexGeneration uint64
	Turn            ConversationTurn
}

func conversationTurnPoint(work ConversationIndexWork, dense []float32, sparse contextindex.SparseVector) (contextindex.IndexedPoint, error) {
	if work.Profile.ID == 0 || work.IndexGeneration == 0 || strings.TrimSpace(work.Platform) == "" {
		return contextindex.IndexedPoint{}, errors.New("conversation index work is incomplete")
	}
	if err := work.Turn.ComputeSourceSHA256(); err != nil {
		return contextindex.IndexedPoint{}, err
	}
	ref, err := PointID(work.Profile.ID, contextindex.SourceKindConversationTurn, work.Turn.UserMessage.ID, work.Turn.SourceSHA256)
	if err != nil {
		return contextindex.IndexedPoint{}, err
	}
	pointRef, err := contextindex.NewPointRef(ref, work.Profile.ID, work.IndexGeneration, contextindex.SourceKindConversationTurn, work.Turn.UserMessage.ID, work.Turn.SourceSHA256)
	if err != nil {
		return contextindex.IndexedPoint{}, err
	}
	return contextindex.NewIndexedPoint(contextindex.PointMetadata{
		Ref: pointRef, Platform: strings.ToLower(strings.TrimSpace(work.Platform)),
		ScopeKind: contextindex.ScopeKindConversation, ConversationID: work.Turn.ConversationID, UserID: work.Turn.UserID,
	}, dense, sparse)
}

type ConversationIndexRepository interface {
	LoadConversationIndexWork(context.Context, ContextConversationIndexV1) (ConversationIndexWork, error)
	BuildConversationIndexPayload(context.Context, uint64, uint64) (ContextConversationIndexV1, error)
}

type ConversationIndexRepairRepository interface {
	ListConversationIndexPayloads(context.Context, uint64, int) ([]ContextConversationIndexV1, uint64, error)
}

type GormConversationIndexRepository struct {
	db    *gorm.DB
	turns *GormConversationRepository
}

func NewConversationIndexRepository(db *gorm.DB) *GormConversationIndexRepository {
	if db == nil {
		return nil
	}
	return &GormConversationIndexRepository{db: db, turns: NewConversationRepositoryWithDB(db)}
}

type conversationIndexAuthorityRow struct {
	UserID          uint64  `gorm:"column:user_id"`
	AgentID         uint64  `gorm:"column:agent_id"`
	ProfileID       uint64  `gorm:"column:profile_id"`
	IndexGeneration *uint64 `gorm:"column:index_generation"`
}

func (repository *GormConversationIndexRepository) LoadConversationIndexWork(ctx context.Context, payload ContextConversationIndexV1) (ConversationIndexWork, error) {
	if repository == nil || repository.db == nil || repository.turns == nil {
		return ConversationIndexWork{}, errors.New("conversation index repository is not configured")
	}
	if err := payload.Validate(); err != nil {
		return ConversationIndexWork{}, err
	}
	var authority conversationIndexAuthorityRow
	err := repository.db.WithContext(ctx).Table("ai_conversations AS c").
		Select("c.user_id, c.agent_id, a.context_profile_id AS profile_id, p.active_index_generation AS index_generation").
		Joins("JOIN ai_agents AS a ON a.id = c.agent_id AND a.is_del = ?", enum.CommonNo).
		Joins("JOIN ai_context_profiles AS p ON p.id = a.context_profile_id AND p.status = ? AND p.index_state IN ?", ProfileEnabled, []ProfileIndexState{ProfileIndexReady, ProfileIndexRebuilding}).
		Where("c.id = ? AND c.is_del = ? AND a.context_profile_id = ?", payload.ConversationID, enum.CommonNo, payload.ProfileID).
		Scan(&authority).Error
	if err != nil {
		return ConversationIndexWork{}, err
	}
	if authority.UserID == 0 || authority.AgentID == 0 || authority.ProfileID != payload.ProfileID || authority.IndexGeneration == nil || *authority.IndexGeneration == 0 {
		return ConversationIndexWork{}, ErrConversationTurnNotAuthoritative
	}
	turns, err := repository.turns.CompleteByAnchors(ctx, payload.ConversationID, authority.UserID, []uint64{payload.UserMessageID})
	if err != nil {
		return ConversationIndexWork{}, err
	}
	if len(turns) != 1 || turns[0].SourceSHA256 != payload.SourceSHA256 {
		return ConversationIndexWork{}, ErrConversationTurnNotAuthoritative
	}
	var profile ContextProfile
	if err := repository.db.WithContext(ctx).
		Where("id = ? AND status = ? AND index_state IN ?", authority.ProfileID, ProfileEnabled, []ProfileIndexState{ProfileIndexReady, ProfileIndexRebuilding}).
		First(&profile).Error; err != nil {
		return ConversationIndexWork{}, err
	}
	return ConversationIndexWork{Profile: profile, Platform: "admin", IndexGeneration: *authority.IndexGeneration, Turn: turns[0]}, nil
}

func (repository *GormConversationIndexRepository) BuildConversationIndexPayload(ctx context.Context, conversationID, userMessageID uint64) (ContextConversationIndexV1, error) {
	if repository == nil || repository.db == nil || repository.turns == nil || conversationID == 0 || userMessageID == 0 {
		return ContextConversationIndexV1{}, errors.New("conversation index payload source is unavailable")
	}
	var authority struct {
		UserID    uint64 `gorm:"column:user_id"`
		ProfileID uint64 `gorm:"column:profile_id"`
	}
	if err := repository.db.WithContext(ctx).Table("ai_conversations AS c").
		Select("c.user_id, a.context_profile_id AS profile_id").
		Joins("JOIN ai_agents AS a ON a.id = c.agent_id AND a.is_del = ?", enum.CommonNo).
		Where("c.id = ? AND c.is_del = ?", conversationID, enum.CommonNo).Scan(&authority).Error; err != nil {
		return ContextConversationIndexV1{}, err
	}
	if authority.UserID == 0 || authority.ProfileID == 0 {
		return ContextConversationIndexV1{}, errors.New("conversation index profile is unavailable")
	}
	turns, err := repository.turns.CompleteByAnchors(ctx, conversationID, authority.UserID, []uint64{userMessageID})
	if err != nil || len(turns) != 1 {
		return ContextConversationIndexV1{}, errors.New("conversation turn is not complete")
	}
	return ContextConversationIndexV1{ProfileID: authority.ProfileID, ConversationID: conversationID, UserMessageID: userMessageID, SourceSHA256: turns[0].SourceSHA256}, nil
}

func (repository *GormConversationIndexRepository) ListConversationIndexPayloads(ctx context.Context, afterRunID uint64, limit int) ([]ContextConversationIndexV1, uint64, error) {
	if repository == nil || repository.db == nil || limit <= 0 {
		return nil, 0, errors.New("conversation index repair repository is not configured")
	}
	var rows []struct {
		RunID          uint64 `gorm:"column:run_id"`
		ConversationID uint64 `gorm:"column:conversation_id"`
		UserMessageID  uint64 `gorm:"column:user_message_id"`
		ProfileID      uint64 `gorm:"column:profile_id"`
	}
	err := repository.db.WithContext(ctx).Table("ai_runs AS r").
		Select("r.id AS run_id, r.conversation_id, r.user_message_id, a.context_profile_id AS profile_id").
		Joins("JOIN ai_conversations AS c ON c.id = r.conversation_id AND c.user_id = r.user_id AND c.is_del = ?", enum.CommonNo).
		Joins("JOIN ai_agents AS a ON a.id = r.agent_id AND a.id = c.agent_id AND a.is_del = ? AND a.context_profile_id IS NOT NULL", enum.CommonNo).
		Where("r.id > ? AND r.status IN ? AND r.assistant_message_id IS NOT NULL", afterRunID, []string{enum.AIRunStatusSuccess, enum.AIRunStatusCanceled}).
		Order("r.id ASC").Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, afterRunID, err
	}
	payloads := make([]ContextConversationIndexV1, 0, len(rows))
	next := afterRunID
	for _, row := range rows {
		next = row.RunID
		payload, err := repository.BuildConversationIndexPayload(ctx, row.ConversationID, row.UserMessageID)
		if err != nil {
			continue
		}
		if payload.ProfileID != row.ProfileID {
			continue
		}
		payloads = append(payloads, payload)
	}
	if len(rows) < limit {
		next = 0
	}
	return payloads, next, nil
}

type ConversationIndexDependencies struct {
	Repository       ConversationIndexRepository
	Embeddings       EmbeddingResolver
	Index            IndexWriter
	CollectionPrefix string
}

type ConversationIndexService struct{ deps ConversationIndexDependencies }

func NewConversationIndexService(deps ConversationIndexDependencies) *ConversationIndexService {
	return &ConversationIndexService{deps: deps}
}

type ConversationIndexJobService interface {
	IndexConversationTurn(context.Context, ContextConversationIndexV1) error
}

func (service *ConversationIndexService) IndexConversationTurn(ctx context.Context, payload ContextConversationIndexV1) error {
	if service == nil || service.deps.Repository == nil || service.deps.Embeddings == nil || service.deps.Index == nil || strings.TrimSpace(service.deps.CollectionPrefix) == "" {
		return errors.New("conversation index service is not configured")
	}
	if err := payload.Validate(); err != nil {
		return err
	}
	work, err := service.deps.Repository.LoadConversationIndexWork(ctx, payload)
	if err != nil {
		if errors.Is(err, ErrConversationTurnNotAuthoritative) {
			return nil
		}
		return err
	}
	if work.Profile.ID != payload.ProfileID || work.Turn.ConversationID != payload.ConversationID || work.Turn.UserMessage.ID != payload.UserMessageID {
		return errors.New("conversation index authority changed")
	}
	if work.Turn.SourceSHA256 != payload.SourceSHA256 {
		return nil
	}
	counter, err := infraai.ResolveTokenCounter(work.Profile.EmbeddingTokenCounterID)
	if err != nil {
		return err
	}
	text, err := BuildConversationTurnText(work.Turn, counter, work.Profile.EmbeddingMaxInputTokens)
	if err != nil {
		return err
	}
	embedding, err := service.deps.Embeddings.ResolveEmbedding(ctx, work.Profile)
	if err != nil {
		return err
	}
	result, err := embedding.Embed(ctx, []string{text.Text})
	if err != nil {
		return err
	}
	if len(result.Vectors) != 1 {
		return errors.New("conversation embedding result count mismatch")
	}
	sparse, err := EncodeSparse(text.Text)
	if err != nil {
		return err
	}
	point, err := conversationTurnPoint(work, result.Vectors[0], sparse)
	if err != nil {
		return err
	}
	collection := physicalCollectionName(service.deps.CollectionPrefix, work.Profile.ID, work.IndexGeneration)
	return service.deps.Index.Upsert(ctx, collection, []contextindex.IndexedPoint{point})
}

type QueueConversationTurnEnqueuer struct{ queue taskqueue.Enqueuer }

func NewConversationTurnEnqueuer(queue taskqueue.Enqueuer) *QueueConversationTurnEnqueuer {
	return &QueueConversationTurnEnqueuer{queue: queue}
}

func (enqueuer *QueueConversationTurnEnqueuer) EnqueueConversationTurn(ctx context.Context, payload ContextConversationIndexV1) error {
	if enqueuer == nil || enqueuer.queue == nil {
		return errors.New("conversation index enqueuer is not configured")
	}
	key, err := ConversationIndexIdempotencyKey(payload)
	if err != nil {
		return err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = enqueuer.queue.Enqueue(ctx, taskqueue.Task{ID: key, Type: TaskContextConversationIndexV1, Payload: data})
	if taskqueue.IsDuplicateTask(err) {
		return nil
	}
	return err
}
