package contextengine

import (
	"context"
	"errors"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/aigateway"
)

type Runtime interface {
	BuildPlan(context.Context, RuntimeInput) (RuntimeResult, error)
}

type RuntimeMaterializer interface {
	Materialize(context.Context, RuntimeInput) (BuildPlanInput, error)
}

type RuntimeService struct {
	materializer RuntimeMaterializer
	planner      *Planner
}

func NewRuntimeService(materializer RuntimeMaterializer, planner *Planner) *RuntimeService {
	if materializer == nil || planner == nil {
		return nil
	}
	return &RuntimeService{materializer: materializer, planner: planner}
}

func (service *RuntimeService) BuildPlan(ctx context.Context, input RuntimeInput) (RuntimeResult, error) {
	if service == nil || service.materializer == nil || service.planner == nil {
		return RuntimeResult{}, ErrPlanRepositoryNotConfigured
	}
	materialized, err := service.materializer.Materialize(ctx, input)
	if err != nil {
		return RuntimeResult{}, err
	}
	if !sameRuntimeIdentity(input, materialized) {
		return RuntimeResult{}, ErrInvalidContextPlan
	}
	plan, err := service.planner.BuildPlan(ctx, materialized)
	if err != nil {
		return RuntimeResult{}, err
	}
	return RuntimeResultFromPlan(plan)
}

type RuntimeInput struct {
	RunID            uint64
	ReplyCommandID   uint64
	LeaseOwner       string
	LeaseToken       uint64
	CurrentMessageID uint64
	AgentID          uint64
	UserID           uint64
	ConversationID   uint64
	ProviderID       uint64
	ModelID          string
	APIProtocol      string
	Messages         []infraai.Message
	Tools            []infraai.ToolDefinition
	Temperature      *float64
}

type RuntimeResult struct {
	Evidence  aigateway.ContextPlanEvidence
	ChatInput infraai.ChatInput
}

func sameRuntimeIdentity(runtime RuntimeInput, plan BuildPlanInput) bool {
	return runtime.RunID != 0 && runtime.RunID == plan.RunID &&
		runtime.ReplyCommandID == plan.ReplyCommandID && runtime.LeaseOwner == plan.LeaseOwner &&
		runtime.LeaseToken == plan.LeaseToken && runtime.CurrentMessageID == plan.CurrentMessageID &&
		runtime.AgentID == plan.AgentID && runtime.UserID == plan.UserID &&
		runtime.ConversationID == plan.ConversationID && runtime.ProviderID == plan.ProviderID &&
		runtime.ModelID == plan.ModelID && runtime.APIProtocol == plan.APIProtocol
}

func RuntimeResultFromPlan(plan ContextPlan) (RuntimeResult, error) {
	if plan.State == PlanFailed {
		if plan.Error == nil {
			return RuntimeResult{}, ErrInvalidContextPlan
		}
		appErr, err := NewContextAppError(plan.Error.Code, nil)
		if err != nil {
			return RuntimeResult{}, err
		}
		return RuntimeResult{}, appErr
	}
	if plan.ID == 0 || plan.PlanSHA256 == nil {
		return RuntimeResult{}, ErrInvalidContextPlan
	}
	input, err := CompileChatInput(plan)
	if err != nil {
		return RuntimeResult{}, err
	}
	evidence := aigateway.ContextPlanEvidence{ID: plan.ID, SHA256: *plan.PlanSHA256}
	if err := evidence.Validate(); err != nil {
		return RuntimeResult{}, errors.Join(ErrInvalidContextPlan, err)
	}
	return RuntimeResult{Evidence: evidence, ChatInput: input}, nil
}
