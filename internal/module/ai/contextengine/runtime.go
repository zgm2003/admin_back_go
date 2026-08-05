package contextengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/shared/apperror"
)

type Runtime interface {
	BuildPlan(context.Context, RuntimeInput) (RuntimeResult, error)
}

type ToolContinuationInput struct {
	PlanID      uint64
	PlanSHA256  [32]byte
	ToolCalls   []infraai.ToolCall
	ToolOutputs []infraai.ToolOutput
}

type ToolContinuationGuard interface {
	GuardToolContinuation(context.Context, ToolContinuationInput) *apperror.Error
}

type RuntimeMaterializer interface {
	Materialize(context.Context, RuntimeInput) (BuildPlanInput, error)
}

type RuntimeService struct {
	materializer RuntimeMaterializer
	planner      *Planner
	dispatch     *DispatchGuardFactory
}

func NewRuntimeService(materializer RuntimeMaterializer, planner *Planner, dispatch ...*DispatchGuardFactory) *RuntimeService {
	if materializer == nil || planner == nil {
		return nil
	}
	service := &RuntimeService{materializer: materializer, planner: planner}
	if len(dispatch) > 0 {
		service.dispatch = dispatch[0]
	}
	return service
}

func (service *RuntimeService) BindDispatchGuard(commandID uint64, owner string, token uint64) aigateway.DispatchGuard {
	if service == nil || service.dispatch == nil {
		return nil
	}
	return service.dispatch.Bind(commandID, owner, token)
}

func (service *RuntimeService) GuardToolContinuation(ctx context.Context, input ToolContinuationInput) *apperror.Error {
	if service == nil || service.dispatch == nil || service.dispatch.db == nil || input.PlanID == 0 || input.PlanSHA256 == ([32]byte{}) {
		return continuationAppError(ErrCodePlanConflict, ErrInvalidContextPlan)
	}
	var row contextPlanRow
	if err := service.dispatch.db.WithContext(ctx).Where("id = ?", input.PlanID).Take(&row).Error; err != nil {
		return continuationAppError(ErrCodePlanConflict, err)
	}
	var items []contextPlanItemRow
	if err := service.dispatch.db.WithContext(ctx).Where("plan_id = ?", input.PlanID).Order("ordinal ASC").Find(&items).Error; err != nil {
		return continuationAppError(ErrCodePlanConflict, err)
	}
	plan, err := contextPlanFromRows(row, items)
	if err != nil || plan.PlanSHA256 == nil || *plan.PlanSHA256 != input.PlanSHA256 || row.State != string(PlanReady) ||
		len(row.PlanSHA256) != 32 || !bytes.Equal(row.PlanSHA256, input.PlanSHA256[:]) || plan.Budget.ToolContinuationInputReserve == 0 {
		return continuationAppError(ErrCodePlanConflict, ErrInvalidContextPlan)
	}
	computed, err := HashPlan(plan)
	if err != nil || computed != input.PlanSHA256 {
		return continuationAppError(ErrCodePlanConflict, ErrInvalidContextPlan)
	}
	counter, err := infraai.ResolveTokenCounter(plan.TokenCounterID)
	if err != nil {
		return continuationAppError(ErrCodePlanConflict, err)
	}
	bound, err := toolContinuationUpperBound(input.ToolCalls, input.ToolOutputs, len(input.ToolOutputs) > 0)
	if err != nil {
		return continuationAppError(ErrCodeToolContinuationOverflow, err)
	}
	upper, err := counter.UpperBoundText(bound)
	if err != nil {
		return continuationAppError(ErrCodeToolContinuationOverflow, err)
	}
	if upper > plan.Budget.ToolContinuationInputReserve {
		return continuationAppError(ErrCodeToolContinuationOverflow, ErrInvalidBudget)
	}
	return nil
}

func toolContinuationUpperBound(calls []infraai.ToolCall, outputs []infraai.ToolOutput, complete bool) (string, error) {
	if len(calls) == 0 || (!complete && len(outputs) != 0) || (complete && len(calls) != len(outputs)) {
		return "", ErrInvalidContextPlan
	}
	type callFact struct {
		CallID    string          `json:"call_id"`
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
		Result    json.RawMessage `json:"result,omitempty"`
	}
	groups := make([]callFact, len(calls))
	seen := make(map[string]struct{}, len(calls))
	for index, call := range calls {
		id, name, args := strings.TrimSpace(call.ID), strings.TrimSpace(call.Name), strings.TrimSpace(call.Arguments)
		if args == "" {
			args = `{}`
		}
		canonicalArgs, err := canonicalToolJSON(args)
		if id == "" || name == "" || err != nil {
			return "", ErrInvalidContextPlan
		}
		if _, duplicate := seen[id]; duplicate {
			return "", ErrInvalidContextPlan
		}
		seen[id] = struct{}{}
		groups[index] = callFact{CallID: id, Name: name, Arguments: canonicalArgs}
	}
	if complete {
		for index, output := range outputs {
			id, name, result := strings.TrimSpace(output.CallID), strings.TrimSpace(output.Name), strings.TrimSpace(output.Output)
			canonicalResult, err := canonicalToolJSON(result)
			if id == "" || name == "" || result == "" || err != nil {
				return "", ErrInvalidContextPlan
			}
			if groups[index].CallID != id || groups[index].Name != name {
				return "", ErrInvalidContextPlan
			}
			groups[index].Result = canonicalResult
		}
	}
	raw, err := json.Marshal(struct {
		Schema string     `json:"schema"`
		Groups []callFact `json:"groups"`
	}{Schema: "tool_continuation_v1", Groups: groups})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func canonicalToolJSON(raw string) (json.RawMessage, error) {
	var value any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, ErrInvalidContextPlan
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(encoded), nil
}

func continuationAppError(code ErrorCode, cause error) *apperror.Error {
	appErr, err := NewContextAppError(code, cause)
	if err != nil {
		return apperror.Internal("工具续接校验失败")
	}
	return appErr
}

func (service *RuntimeService) BuildPlan(ctx context.Context, input RuntimeInput) (RuntimeResult, error) {
	if service == nil || service.materializer == nil || service.planner == nil {
		return RuntimeResult{}, ErrPlanRepositoryNotConfigured
	}
	existing, err := service.planner.FindTerminalByRunID(ctx, input.RunID)
	if err != nil {
		return RuntimeResult{}, err
	}
	if existing != nil {
		return RuntimeResultFromPlan(*existing)
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
