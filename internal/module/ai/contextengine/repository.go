package contextengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"

	"admin_back_go/internal/infra/database"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

type PlanRepository interface {
	FindTerminalByRunID(context.Context, uint64) (*ContextPlan, error)
	PersistTerminal(context.Context, ContextPlan, PlanCommitTransactionGuard, PlanCommitToken) (ContextPlan, PersistDisposition, error)
}

type PlanCommitTransactionGuard interface {
	GuardPlanCommitInTransaction(context.Context, *gorm.DB, PlanCommitToken) (PlanCommitGuardResult, error)
}

type PlanCommitGuardResult struct {
	SnapshotConflict *PlanError
}

type PlanCommitToken struct {
	RunID                   uint64
	ReplyCommandID          uint64
	LeaseOwner              string
	LeaseToken              uint64
	InputFingerprintSHA256  [32]byte
	AuthoritySnapshotSHA256 [32]byte
}

func (token PlanCommitToken) Validate() error {
	if token.RunID == 0 || token.ReplyCommandID == 0 || token.LeaseToken == 0 ||
		!validIdentifier(token.LeaseOwner, 191) || isZeroSHA256(token.InputFingerprintSHA256) ||
		isZeroSHA256(token.AuthoritySnapshotSHA256) {
		return ErrInvalidPlanCommitToken
	}
	return nil
}

type PersistDisposition string

const (
	PersistCreated        PersistDisposition = "created"
	PersistLoadedExisting PersistDisposition = "loaded_existing"
)

func (disposition PersistDisposition) Validate() error {
	switch disposition {
	case PersistCreated, PersistLoadedExisting:
		return nil
	}
	return invalidValue("persist disposition", string(disposition))
}

type GormPlanRepository struct{ db *gorm.DB }

func NewPlanRepository(client *database.Client) *GormPlanRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormPlanRepository{db: client.Gorm}
}

func (repository *GormPlanRepository) FindTerminalByRunID(ctx context.Context, runID uint64) (*ContextPlan, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrPlanRepositoryNotConfigured
	}
	if runID == 0 {
		return nil, ErrInvalidContextPlan
	}
	var planRow contextPlanRow
	err := repository.db.WithContext(ctx).Where("run_id = ?", runID).Order("id ASC").Take(&planRow).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find terminal context plan: %w", err)
	}
	var itemRows []contextPlanItemRow
	if err := repository.db.WithContext(ctx).Where("plan_id = ?", planRow.ID).Order("ordinal ASC").Find(&itemRows).Error; err != nil {
		return nil, fmt.Errorf("find terminal context plan items: %w", err)
	}
	plan, err := contextPlanFromRows(planRow, itemRows)
	if err != nil {
		return nil, fmt.Errorf("decode terminal context plan: %w", err)
	}
	return &plan, nil
}

func (repository *GormPlanRepository) PersistTerminal(
	ctx context.Context,
	candidate ContextPlan,
	guard PlanCommitTransactionGuard,
	token PlanCommitToken,
) (ContextPlan, PersistDisposition, error) {
	if repository == nil || repository.db == nil {
		return ContextPlan{}, "", ErrPlanRepositoryNotConfigured
	}
	if isNilPlanCommitGuard(guard) {
		return ContextPlan{}, "", ErrNilPlanCommitGuard
	}
	if candidate.ID != 0 {
		return ContextPlan{}, "", ErrInvalidContextPlan
	}
	if err := candidate.Validate(); err != nil {
		return ContextPlan{}, "", err
	}
	if err := token.Validate(); err != nil {
		return ContextPlan{}, "", err
	}
	if token.RunID != candidate.RunID || token.InputFingerprintSHA256 != candidate.InputFingerprintSHA256 {
		return ContextPlan{}, "", ErrInvalidPlanCommitToken
	}

	created := ContextPlan{}
	err := repository.db.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		guardResult, err := guard.GuardPlanCommitInTransaction(ctx, transaction, token)
		if err != nil {
			if errors.Is(err, ErrPlanCommitAborted) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("%w: %v", ErrPlanCommitAborted, err)
			}
			return fmt.Errorf("guard context plan commit: %w", err)
		}

		terminal := candidate
		if guardResult.SnapshotConflict != nil {
			if guardResult.SnapshotConflict.Code != ErrCodeSnapshotConflict {
				return fmt.Errorf("%w: snapshot conflict code", ErrInvalidContextPlan)
			}
			if err := guardResult.SnapshotConflict.Validate(); err != nil {
				return err
			}
			terminal.State = PlanFailed
			terminal.RetrievalOutcome = RetrievalFailed
			terminal.PlanSHA256 = nil
			terminal.Items = nil
			terminal.Error = clonePlanError(guardResult.SnapshotConflict)
			if err := terminal.Validate(); err != nil {
				return err
			}
		}

		row, err := contextPlanRowFromDomain(terminal)
		if err != nil {
			return err
		}
		if err := transaction.Create(&row).Error; err != nil {
			if isDuplicateRunPlan(err) {
				return errDuplicateRunPlan
			}
			return fmt.Errorf("insert terminal context plan: %w", err)
		}
		terminal.ID = row.ID

		itemRows, err := contextPlanItemRowsFromDomain(row.ID, terminal.Items)
		if err != nil {
			return err
		}
		if len(itemRows) != 0 {
			if err := transaction.Create(&itemRows).Error; err != nil {
				return fmt.Errorf("insert terminal context plan items: %w", err)
			}
		}
		created = terminal
		return nil
	})
	if err == nil {
		return created, PersistCreated, nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ContextPlan{}, "", fmt.Errorf("%w: %v", ErrPlanCommitAborted, err)
	}
	if !errors.Is(err, errDuplicateRunPlan) {
		return ContextPlan{}, "", err
	}

	winner, err := repository.FindTerminalByRunID(ctx, candidate.RunID)
	if err != nil {
		return ContextPlan{}, "", err
	}
	if winner == nil {
		return ContextPlan{}, "", errors.New("duplicate context plan has no committed winner")
	}
	return *winner, PersistLoadedExisting, nil
}

var errDuplicateRunPlan = errors.New("duplicate context plan run")

func contextPlanRowFromDomain(plan ContextPlan) (contextPlanRow, error) {
	metrics, err := json.Marshal(plan.Metrics)
	if err != nil {
		return contextPlanRow{}, fmt.Errorf("encode context plan metrics: %w", err)
	}
	row := contextPlanRow{
		RunID:                        plan.RunID,
		PolicyVersion:                plan.PolicyVersion,
		InputFingerprintSHA256:       append([]byte(nil), plan.InputFingerprintSHA256[:]...),
		ModelCapabilitySHA256:        append([]byte(nil), plan.ModelCapabilitySHA256[:]...),
		APIProtocolSnapshot:          plan.APIProtocol,
		TokenCounterIDSnapshot:       plan.TokenCounterID,
		ContextWindowTokens:          uint64(plan.Budget.ContextWindowTokens),
		EffectiveOutputTokens:        uint64(plan.Budget.EffectiveOutputTokens),
		ProviderProtocolUpperBound:   uint64(plan.Budget.ProviderProtocolUpperBound),
		ToolContinuationInputReserve: uint64(plan.Budget.ToolContinuationInputReserve),
		PolicySafetyMargin:           uint64(plan.Budget.PolicySafetyMargin),
		KnownInputBudget:             uint64(plan.Budget.KnownInputBudget),
		KnownInputUpperBound:         uint64(plan.Budget.KnownInputUpperBound),
		BudgetProof:                  string(plan.Budget.Proof),
		RetrievalOutcome:             string(plan.RetrievalOutcome),
		State:                        string(plan.State),
		MetricsJSON:                  string(metrics),
	}
	if plan.Profile != nil {
		profileID := plan.Profile.ID
		row.ContextProfileIDSnapshot = &profileID
		row.ContextProfileSHA256 = append([]byte(nil), plan.Profile.SHA256[:]...)
		row.ContextIndexGenerationSnapshot = cloneUint64(plan.Profile.IndexGeneration)
	}
	if plan.PlanSHA256 != nil {
		row.PlanSHA256 = append([]byte(nil), plan.PlanSHA256[:]...)
	}
	if plan.Error != nil {
		stage, code := plan.Error.Stage, string(plan.Error.Code)
		row.ErrorStage, row.ErrorCode = &stage, &code
		row.ErrorMessage = cloneString(plan.Error.Message)
	}
	return row, nil
}

func contextPlanItemRowsFromDomain(planID uint64, items []ContextPlanItem) ([]contextPlanItemRow, error) {
	rows := make([]contextPlanItemRow, 0, len(items))
	for _, item := range items {
		metadata, err := json.Marshal(item.Block.Metadata)
		if err != nil {
			return nil, fmt.Errorf("encode context plan item metadata: %w", err)
		}
		row := contextPlanItemRow{
			PlanID:          planID,
			Ordinal:         item.Ordinal,
			BlockKind:       string(item.Block.Kind),
			SourceType:      item.Block.SourceType,
			SourceRef:       item.Block.SourceRef,
			SourceSHA256:    append([]byte(nil), item.Block.SourceSHA256[:]...),
			AtomicGroupKey:  item.Block.AtomicGroupKey,
			Priority:        item.Block.Priority,
			Decision:        string(item.Decision),
			TokenUpperBound: uint64(item.Block.TokenUpperBound),
			CitationKey:     cloneString(item.CitationKey),
			ContentSnapshot: cloneString(item.Block.ContentSnapshot),
			MetadataJSON:    string(metadata),
		}
		if item.Block.Required {
			row.Required = 1
		}
		if item.ExclusionReason != nil {
			value := string(*item.ExclusionReason)
			row.ExclusionReason = &value
		}
		if item.FusionScore != nil {
			value := item.FusionScore.String()
			row.FusionScore = &value
		}
		if item.RerankScore != nil {
			value := item.RerankScore.String()
			row.RerankScore = &value
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func contextPlanFromRows(planRow contextPlanRow, itemRows []contextPlanItemRow) (ContextPlan, error) {
	inputHash, err := SHA256FromBytes(planRow.InputFingerprintSHA256)
	if err != nil {
		return ContextPlan{}, err
	}
	modelHash, err := SHA256FromBytes(planRow.ModelCapabilitySHA256)
	if err != nil {
		return ContextPlan{}, err
	}
	planHash, err := optionalSHA256FromBytes(planRow.PlanSHA256)
	if err != nil {
		return ContextPlan{}, err
	}
	budget, err := budgetFromRow(planRow)
	if err != nil {
		return ContextPlan{}, err
	}
	var metrics ContextPlanMetricsV1
	if err := strictJSONDecode(planRow.MetricsJSON, &metrics); err != nil {
		return ContextPlan{}, fmt.Errorf("decode context plan metrics: %w", err)
	}
	plan := ContextPlan{
		ID:                     planRow.ID,
		RunID:                  planRow.RunID,
		PolicyVersion:          planRow.PolicyVersion,
		InputFingerprintSHA256: inputHash,
		PlanSHA256:             planHash,
		ModelCapabilitySHA256:  modelHash,
		APIProtocol:            planRow.APIProtocolSnapshot,
		TokenCounterID:         planRow.TokenCounterIDSnapshot,
		Budget:                 budget,
		RetrievalOutcome:       RetrievalOutcome(planRow.RetrievalOutcome),
		State:                  PlanState(planRow.State),
		Metrics:                metrics,
	}
	if planRow.ContextProfileIDSnapshot != nil {
		profileHash, err := SHA256FromBytes(planRow.ContextProfileSHA256)
		if err != nil {
			return ContextPlan{}, err
		}
		plan.Profile = &ProfileSnapshot{
			ID: *planRow.ContextProfileIDSnapshot, SHA256: profileHash,
			IndexGeneration: cloneUint64(planRow.ContextIndexGenerationSnapshot),
		}
	} else if len(planRow.ContextProfileSHA256) != 0 || planRow.ContextIndexGenerationSnapshot != nil {
		return ContextPlan{}, ErrInvalidContextPlan
	}
	if planRow.ErrorStage != nil || planRow.ErrorCode != nil || planRow.ErrorMessage != nil {
		if planRow.ErrorStage == nil || planRow.ErrorCode == nil {
			return ContextPlan{}, ErrInvalidContextPlan
		}
		plan.Error = &PlanError{
			Stage: *planRow.ErrorStage, Code: ErrorCode(*planRow.ErrorCode), Message: cloneString(planRow.ErrorMessage),
		}
	}
	items, err := contextPlanItemsFromRows(itemRows)
	if err != nil {
		return ContextPlan{}, err
	}
	plan.Items = items
	if err := plan.Validate(); err != nil {
		return ContextPlan{}, err
	}
	return plan, nil
}

func contextPlanItemsFromRows(rows []contextPlanItemRow) ([]ContextPlanItem, error) {
	items := make([]ContextPlanItem, 0, len(rows))
	for _, row := range rows {
		hash, err := SHA256FromBytes(row.SourceSHA256)
		if err != nil {
			return nil, err
		}
		tokenUpperBound, err := checkedInt64(row.TokenUpperBound)
		if err != nil {
			return nil, err
		}
		var metadata ContextBlockMetadataV1
		if err := strictJSONDecode(row.MetadataJSON, &metadata); err != nil {
			return nil, fmt.Errorf("decode context plan item metadata: %w", err)
		}
		item := ContextPlanItem{
			Ordinal: row.Ordinal,
			Block: ContextBlock{
				Kind: BlockKind(row.BlockKind), SourceType: row.SourceType, SourceRef: row.SourceRef,
				SourceSHA256: hash, AtomicGroupKey: row.AtomicGroupKey, Required: row.Required == 1,
				Priority: row.Priority, TokenUpperBound: tokenUpperBound,
				ContentSnapshot: cloneString(row.ContentSnapshot), Metadata: metadata,
			},
			Decision: Decision(row.Decision), CitationKey: cloneString(row.CitationKey),
		}
		if row.Required > 1 {
			return nil, ErrInvalidContextPlan
		}
		if row.ExclusionReason != nil {
			value := ExclusionReason(*row.ExclusionReason)
			item.ExclusionReason = &value
		}
		if row.FusionScore != nil {
			value, err := ParseFixedScore(*row.FusionScore)
			if err != nil {
				return nil, err
			}
			item.FusionScore = &value
		}
		if row.RerankScore != nil {
			value, err := ParseFixedScore(*row.RerankScore)
			if err != nil {
				return nil, err
			}
			item.RerankScore = &value
		}
		items = append(items, item)
	}
	return items, nil
}

func budgetFromRow(row contextPlanRow) (Budget, error) {
	values := []uint64{
		row.ContextWindowTokens, row.EffectiveOutputTokens, row.ProviderProtocolUpperBound,
		row.ToolContinuationInputReserve, row.PolicySafetyMargin, row.KnownInputBudget, row.KnownInputUpperBound,
	}
	converted := make([]int64, len(values))
	for index, value := range values {
		var err error
		converted[index], err = checkedInt64(value)
		if err != nil {
			return Budget{}, err
		}
	}
	return Budget{
		ContextWindowTokens: converted[0], EffectiveOutputTokens: converted[1],
		ProviderProtocolUpperBound: converted[2], ToolContinuationInputReserve: converted[3],
		PolicySafetyMargin: converted[4], KnownInputBudget: converted[5], KnownInputUpperBound: converted[6],
		Proof: BudgetProof(row.BudgetProof),
	}, nil
}

func isDuplicateRunPlan(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) && strings.Contains(err.Error(), "uk_ai_context_plans_run") {
		return true
	}
	var mysqlError *mysqldriver.MySQLError
	if errors.As(err, &mysqlError) {
		return mysqlError.Number == 1062 && strings.Contains(mysqlError.Message, "uk_ai_context_plans_run")
	}
	return strings.Contains(err.Error(), "Duplicate entry") && strings.Contains(err.Error(), "uk_ai_context_plans_run")
}

func isNilPlanCommitGuard(guard PlanCommitTransactionGuard) bool {
	if guard == nil {
		return true
	}
	value := reflect.ValueOf(guard)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	}
	return false
}

func optionalSHA256FromBytes(raw []byte) (*[32]byte, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	value, err := SHA256FromBytes(raw)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func checkedInt64(value uint64) (int64, error) {
	if value > math.MaxInt64 {
		return 0, ErrInvalidContextPlan
	}
	return int64(value), nil
}

func clonePlanError(value *PlanError) *PlanError {
	if value == nil {
		return nil
	}
	return &PlanError{Stage: value.Stage, Code: value.Code, Message: cloneString(value.Message)}
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneUint64(value *uint64) *uint64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
