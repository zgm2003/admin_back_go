package aimessage

import (
	"context"
	"errors"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/module/ai/officialmodel"
	aiprovider "admin_back_go/internal/module/ai/provider"
	"admin_back_go/internal/module/ai/replycommand"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/uploadpolicy"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("aimessage repository not configured")

type GormRepository struct {
	db                 *gorm.DB
	replies            replycommand.Repository
	history            replycommand.HistoryTransactionParticipant
	pricing            officialmodel.Resolver
	uploadRuleGuard    UploadRuleTransactionGuard
	historyInvalidator HistoryDerivedInvalidator
	now                func() time.Time
}

type RepositoryOption func(*GormRepository)

type UploadRuleTransactionGuard = replycommand.UploadRuleTransactionGuard

type HistoryAfterCommit = func(context.Context)

type HistoryDerivedInvalidator interface {
	InvalidateSuffixInTransaction(context.Context, *gorm.DB, int64, int64, int64, int64) (HistoryAfterCommit, error)
	InvalidateMessagesInTransaction(context.Context, *gorm.DB, int64, int64, []int64) (HistoryAfterCommit, error)
}

func WithRepositoryPricingResolver(resolver officialmodel.Resolver) RepositoryOption {
	return func(repository *GormRepository) { repository.pricing = resolver }
}

func WithRepositoryUploadRuleGuard(guard UploadRuleTransactionGuard) RepositoryOption {
	return func(repository *GormRepository) { repository.uploadRuleGuard = guard }
}

func WithRepositoryHistoryDerivedInvalidator(invalidator HistoryDerivedInvalidator) RepositoryOption {
	return func(repository *GormRepository) { repository.historyInvalidator = invalidator }
}

func NewGormRepository(client *database.Client, replies replycommand.Repository, history replycommand.HistoryTransactionParticipant, options ...RepositoryOption) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	repository := &GormRepository{db: client.Gorm, replies: replies, history: history, now: time.Now}
	for _, option := range options {
		if option != nil {
			option(repository)
		}
	}
	return repository
}

func (r *GormRepository) CreateReply(ctx context.Context, input replycommand.CreateReplyInput) (replycommand.CreateReplyResult, error) {
	if r == nil || r.replies == nil {
		return replycommand.CreateReplyResult{}, ErrRepositoryNotConfigured
	}
	if input.UploadRuleToken != (uploadpolicy.ConsistencyToken{}) {
		guarded, ok := r.replies.(replycommand.UploadRuleGuardedRepository)
		if !ok || r.uploadRuleGuard == nil {
			return replycommand.CreateReplyResult{}, replycommand.ErrUploadRuleChanged
		}
		return guarded.CreateReplyWithUploadRuleGuard(ctx, input, r.uploadRuleGuard)
	}
	return r.replies.CreateReply(ctx, input)
}

func (r *GormRepository) RequestCancel(ctx context.Context, input replycommand.RequestCancelInput) (replycommand.RequestCancelResult, error) {
	if r == nil || r.replies == nil {
		return replycommand.RequestCancelResult{}, ErrRepositoryNotConfigured
	}
	return r.replies.RequestCancel(ctx, input)
}

func (r *GormRepository) Conversation(ctx context.Context, id int64) (*Conversation, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Conversation
	err := r.db.WithContext(ctx).Where("id = ?", id).Where("is_del = ?", enum.CommonNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) AgentForConversation(ctx context.Context, conversationID int64, userID int64) (*AgentRuntime, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row AgentRuntime
	err := r.db.WithContext(ctx).Table("ai_conversations c").
		Select(`a.id AS agent_id, a.context_profile_id AS context_profile_id, a.provider_id AS provider_id, a.model_id AS model_id,
			a.model_display_name AS model_display_name, e.engine_type AS engine_type,
			e.api_protocol AS api_protocol,
			a.billing_multiplier_ppm AS billing_multiplier_ppm,
			a.status AS status, a.scenes_json AS scenes_json,
			pm.status AS provider_model_status, pm.official_model_id AS official_model_id,
			pm.official_catalog_version AS official_catalog_version, pm.mapping_status AS mapping_status`).
		Joins("JOIN ai_agents a ON a.id = c.agent_id AND a.is_del = ?", enum.CommonNo).
		Joins("JOIN ai_providers e ON e.id = a.provider_id AND e.is_del = ? AND e.status = ?", enum.CommonNo, enum.CommonYes).
		Joins("JOIN ai_provider_models pm ON pm.provider_id = a.provider_id AND pm.model_id = a.model_id AND pm.model_kind = ? AND pm.status = ? AND pm.mapping_status = ?", aiprovider.ModelKindChat, enum.CommonYes, officialmodel.MappingStatusMapped).
		Where("c.id = ? AND c.user_id = ? AND c.is_del = ?", conversationID, userID, enum.CommonNo).
		Limit(1).
		Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.AgentID == 0 {
		return nil, nil
	}
	return &row, nil
}

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]MessageProjection, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, ErrRepositoryNotConfigured
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 20
	}
	db := r.db.WithContext(ctx).Table("ai_messages m").
		Select(`m.id, m.conversation_id, m.role, m.content_type, m.content, m.meta_json,
			m.reply_command_id, m.delivery_state, m.is_del, m.created_at, m.updated_at,
			paired_messages.id AS paired_message_id, ai_runs.id AS run_id,
			(ai_runs.id IS NOT NULL AND ai_runs.liked_at IS NOT NULL) AS liked,
			COALESCE(COALESCE(user_commands.state, assistant_commands.state) IN ('pending','claimed','running'), FALSE) AS settlement_pending`).
		Joins("JOIN ai_conversations c ON c.id = m.conversation_id AND c.user_id = ? AND c.is_del = ?", query.UserID, enum.CommonNo).
		Joins("LEFT JOIN ai_reply_commands user_commands ON user_commands.user_message_id = m.id AND m.role = ?", enum.AIMessageRoleUser).
		Joins("LEFT JOIN ai_reply_commands assistant_commands ON assistant_commands.id = m.reply_command_id AND assistant_commands.assistant_message_id = m.id AND m.role = ?", enum.AIMessageRoleAssistant).
		Joins("LEFT JOIN ai_messages paired_messages ON paired_messages.id = COALESCE(user_commands.assistant_message_id, assistant_commands.user_message_id) AND paired_messages.conversation_id = m.conversation_id AND paired_messages.is_del = ?", enum.CommonNo).
		Joins("LEFT JOIN ai_runs ON ai_runs.user_id = assistant_commands.user_id AND ai_runs.request_id = assistant_commands.request_id AND ai_runs.assistant_message_id = m.id AND ai_runs.conversation_id = m.conversation_id AND m.role = ?", enum.AIMessageRoleAssistant).
		Where("m.conversation_id = ?", query.ConversationID).
		Where("m.is_del = ?", enum.CommonNo)
	if query.BeforeID > 0 {
		db = db.Where("m.id < ?", query.BeforeID)
	}
	var rows []MessageProjection
	err := db.Order("m.id DESC").Limit(limit + 1).Find(&rows).Error
	if err != nil {
		return nil, false, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	return rows, hasMore, nil
}

func (r *GormRepository) ContextPlans(ctx context.Context, runIDs []uint64) (map[uint64]messageContextPlan, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	return loadMessageContextPlans(ctx, r.db, runIDs)
}
