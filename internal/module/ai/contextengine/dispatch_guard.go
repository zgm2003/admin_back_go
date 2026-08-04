package contextengine

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

var (
	errDispatchPlanConflict = errors.New("context dispatch plan conflict")
	errDispatchPermission   = errors.New("context dispatch source is no longer authorized")
)

type DispatchGuardFactory struct {
	db       *gorm.DB
	platform string
	now      func() time.Time
}

func NewDispatchGuardFactory(client *database.Client, platform string, now func() time.Time) *DispatchGuardFactory {
	platform = strings.ToLower(strings.TrimSpace(platform))
	if client == nil || client.Gorm == nil || platform == "" {
		return nil
	}
	if now == nil {
		now = time.Now
	}
	return &DispatchGuardFactory{db: client.Gorm, platform: platform, now: now}
}

func (factory *DispatchGuardFactory) Bind(commandID uint64, owner string, token uint64) aigateway.DispatchGuard {
	owner = strings.TrimSpace(owner)
	if factory == nil || factory.db == nil || factory.now == nil || commandID == 0 || owner == "" || token == 0 {
		return nil
	}
	return &gormDispatchGuard{factory: factory, commandID: commandID, owner: owner, token: token}
}

type gormDispatchGuard struct {
	factory   *DispatchGuardFactory
	commandID uint64
	owner     string
	token     uint64
}

func (guard *gormDispatchGuard) GuardDispatch(ctx context.Context, input aigateway.DispatchGuardInput) *apperror.Error {
	if guard == nil || guard.factory == nil || guard.factory.db == nil || input.RunID <= 0 || input.AttemptNo == 0 ||
		input.ContextPlan.Validate() != nil || input.PreparedRequestSHA256 == ([32]byte{}) {
		return dispatchGuardAppError(ErrCodePlanConflict, ErrInvalidContextPlan)
	}
	err := guard.factory.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		facts, err := loadDispatchFacts(ctx, tx, guard, input)
		if err != nil {
			return err
		}
		return verifyDispatchSources(ctx, tx, guard.factory.platform, facts)
	})
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, errDispatchPlanConflict):
		return dispatchGuardAppError(ErrCodePlanConflict, err)
	case errors.Is(err, errDispatchPermission):
		return dispatchGuardAppError(ErrCodePermissionDenied, err)
	default:
		return dispatchGuardAppError(ErrCodeRetrievalFailed, err)
	}
}

type dispatchGuardFacts struct {
	Plan          contextPlanRow
	Items         []contextPlanItemRow
	SelectedItems []contextPlanItemRow
	Run           dispatchRunRow
}

type dispatchRunRow struct {
	ID             int64   `gorm:"column:id"`
	UserID         uint64  `gorm:"column:user_id"`
	AgentID        uint64  `gorm:"column:agent_id"`
	ConversationID *uint64 `gorm:"column:conversation_id"`
	UserMessageID  *uint64 `gorm:"column:user_message_id"`
	Status         string  `gorm:"column:status"`
}

type dispatchAttemptRow struct {
	RunID                 int64   `gorm:"column:run_id"`
	CommandID             *uint64 `gorm:"column:command_id"`
	AttemptNo             uint32  `gorm:"column:attempt_no"`
	State                 string  `gorm:"column:state"`
	PreparedRequestSHA256 []byte  `gorm:"column:prepared_request_sha256"`
	ContextPlanID         *uint64 `gorm:"column:context_plan_id"`
	ContextPlanSHA256     []byte  `gorm:"column:context_plan_sha256"`
}

func loadDispatchFacts(ctx context.Context, tx *gorm.DB, guard *gormDispatchGuard, input aigateway.DispatchGuardInput) (dispatchGuardFacts, error) {
	var plan contextPlanRow
	if err := tx.WithContext(ctx).Where("id = ?", input.ContextPlan.ID).Take(&plan).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return dispatchGuardFacts{}, errDispatchPlanConflict
		}
		return dispatchGuardFacts{}, err
	}
	if plan.RunID != uint64(input.RunID) || plan.State != string(PlanReady) || len(plan.PlanSHA256) != 32 ||
		!bytes.Equal(plan.PlanSHA256, input.ContextPlan.SHA256[:]) {
		return dispatchGuardFacts{}, errDispatchPlanConflict
	}
	var attempt dispatchAttemptRow
	if err := tx.WithContext(ctx).Table("ai_provider_attempts").
		Where("run_id = ? AND attempt_no = ?", input.RunID, input.AttemptNo).Take(&attempt).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return dispatchGuardFacts{}, errDispatchPlanConflict
		}
		return dispatchGuardFacts{}, err
	}
	if attempt.RunID != input.RunID || attempt.AttemptNo != input.AttemptNo || attempt.State != "prepared" ||
		attempt.CommandID == nil || *attempt.CommandID != guard.commandID || attempt.ContextPlanID == nil ||
		*attempt.ContextPlanID != input.ContextPlan.ID || !bytes.Equal(attempt.ContextPlanSHA256, input.ContextPlan.SHA256[:]) ||
		!bytes.Equal(attempt.PreparedRequestSHA256, input.PreparedRequestSHA256[:]) {
		return dispatchGuardFacts{}, errDispatchPlanConflict
	}
	var command lockedReplyCommand
	if err := tx.WithContext(ctx).Where("id = ?", guard.commandID).Take(&command).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return dispatchGuardFacts{}, errDispatchPlanConflict
		}
		return dispatchGuardFacts{}, err
	}
	if command.State != "running" || command.LeaseOwner == nil || *command.LeaseOwner != guard.owner ||
		command.LeaseToken != guard.token || command.LeaseExpiresAt == nil || !command.LeaseExpiresAt.After(guard.factory.now().UTC()) ||
		command.CancelRequestedAt != nil {
		return dispatchGuardFacts{}, errDispatchPlanConflict
	}
	if err := requireActiveDispatchConversation(ctx, tx, command); err != nil {
		return dispatchGuardFacts{}, err
	}
	var run dispatchRunRow
	if err := tx.WithContext(ctx).Table("ai_runs").Where("id = ?", input.RunID).Take(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return dispatchGuardFacts{}, errDispatchPlanConflict
		}
		return dispatchGuardFacts{}, err
	}
	if run.ID != input.RunID || run.Status != "running" || run.UserID != uint64(command.UserID) ||
		run.ConversationID == nil || *run.ConversationID != uint64(command.ConversationID) ||
		run.UserMessageID == nil || *run.UserMessageID != uint64(command.UserMessageID) {
		return dispatchGuardFacts{}, errDispatchPlanConflict
	}
	var items []contextPlanItemRow
	if err := tx.WithContext(ctx).Where("plan_id = ?", plan.ID).Order("ordinal ASC").Find(&items).Error; err != nil {
		return dispatchGuardFacts{}, err
	}
	if len(items) == 0 {
		return dispatchGuardFacts{}, errDispatchPlanConflict
	}
	domain, err := contextPlanFromRows(plan, items)
	if err != nil || domain.PlanSHA256 == nil || *domain.PlanSHA256 != input.ContextPlan.SHA256 {
		return dispatchGuardFacts{}, errDispatchPlanConflict
	}
	computed, err := HashPlan(domain)
	if err != nil || computed != input.ContextPlan.SHA256 {
		return dispatchGuardFacts{}, errDispatchPlanConflict
	}
	selected := make([]contextPlanItemRow, 0, len(items))
	for _, item := range items {
		if item.Decision == string(DecisionSelected) {
			selected = append(selected, item)
		}
	}
	if len(selected) == 0 {
		return dispatchGuardFacts{}, errDispatchPlanConflict
	}
	return dispatchGuardFacts{Plan: plan, Items: items, SelectedItems: selected, Run: run}, nil
}

func requireActiveDispatchConversation(ctx context.Context, tx *gorm.DB, command lockedReplyCommand) error {
	if tx == nil || command.ConversationID <= 0 || command.UserID <= 0 {
		return errDispatchPlanConflict
	}
	var conversation struct {
		ID int64 `gorm:"column:id"`
	}
	if err := tx.WithContext(ctx).Table("ai_conversations").
		Where("id = ? AND user_id = ? AND is_del = ?", command.ConversationID, command.UserID, enum.CommonNo).
		First(&conversation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errDispatchPlanConflict
		}
		return err
	}
	if conversation.ID != command.ConversationID {
		return errDispatchPlanConflict
	}
	return nil
}

func dispatchGuardAppError(code ErrorCode, cause error) *apperror.Error {
	appErr, err := NewContextAppError(code, cause)
	if err != nil {
		return apperror.Internal("上下文派发校验失败")
	}
	return appErr
}

var _ aigateway.DispatchGuard = (*gormDispatchGuard)(nil)
