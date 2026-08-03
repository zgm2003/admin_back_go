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
	aiprovider "admin_back_go/internal/module/ai/provider"
	"admin_back_go/internal/shared/enum"

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

type GormAdminRepository struct{ db *gorm.DB }

func NewAdminRepository(client *database.Client) *GormAdminRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormAdminRepository{db: client.Gorm}
}

func (repository *GormAdminRepository) FindProviderModelCapability(ctx context.Context, id uint64) (*ProviderModelCapability, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrPlanRepositoryNotConfigured
	}
	var row struct {
		ID              uint64  `gorm:"column:id"`
		Kind            string  `gorm:"column:model_kind"`
		ModelStatus     int     `gorm:"column:model_status"`
		ProviderStatus  int     `gorm:"column:provider_status"`
		ProviderDeleted int     `gorm:"column:provider_deleted"`
		OfficialModelID *string `gorm:"column:official_model_id"`
	}
	err := repository.db.WithContext(ctx).Table("ai_provider_models AS pm").
		Select("pm.id, pm.model_kind, pm.status AS model_status, pm.official_model_id, p.status AS provider_status, p.is_del AS provider_deleted").
		Joins("JOIN ai_providers AS p ON p.id = pm.provider_id").Where("pm.id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	officialID := ""
	if row.OfficialModelID != nil {
		officialID = strings.TrimSpace(*row.OfficialModelID)
	}
	return &ProviderModelCapability{ID: row.ID, Kind: aiprovider.ModelKind(row.Kind), Enabled: row.ModelStatus == enum.CommonYes, ProviderEnabled: row.ProviderStatus == enum.CommonYes && row.ProviderDeleted == enum.CommonNo, OfficialModelID: officialID}, nil
}

func (repository *GormAdminRepository) CreateProfile(ctx context.Context, profile ContextProfile) (ContextProfile, error) {
	if repository == nil || repository.db == nil {
		return ContextProfile{}, ErrPlanRepositoryNotConfigured
	}
	if err := repository.db.WithContext(ctx).Create(&profile).Error; err != nil {
		return ContextProfile{}, err
	}
	return profile, nil
}

func (repository *GormAdminRepository) FindProfile(ctx context.Context, id uint64) (*ContextProfile, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrPlanRepositoryNotConfigured
	}
	var profile ContextProfile
	err := repository.db.WithContext(ctx).Where("id = ?", id).Take(&profile).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func (repository *GormAdminRepository) ListProfiles(ctx context.Context, status ProfileStatus) ([]ContextProfile, error) {
	var items []ContextProfile
	query := repository.db.WithContext(ctx).Model(&ContextProfile{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	err := query.Order("id DESC").Find(&items).Error
	return items, err
}

func (repository *GormAdminRepository) UpdateProfileMetadata(ctx context.Context, id uint64, name string, status ProfileStatus) (ContextProfile, error) {
	if repository == nil || repository.db == nil {
		return ContextProfile{}, ErrPlanRepositoryNotConfigured
	}
	result := repository.db.WithContext(ctx).Model(&ContextProfile{}).Where("id = ?", id).Updates(map[string]any{"name": name, "status": status})
	if result.Error != nil {
		return ContextProfile{}, result.Error
	}
	if result.RowsAffected != 1 {
		return ContextProfile{}, gorm.ErrRecordNotFound
	}
	profile, err := repository.FindProfile(ctx, id)
	if err != nil {
		return ContextProfile{}, err
	}
	return *profile, nil
}

func (repository *GormAdminRepository) CompareAndSwapProfileIndex(ctx context.Context, input ProfileIndexCAS) (bool, error) {
	if repository == nil || repository.db == nil {
		return false, ErrPlanRepositoryNotConfigured
	}
	query := repository.db.WithContext(ctx).Model(&ContextProfile{}).Where("id = ? AND index_state = ?", input.ID, input.Expected.State)
	query = whereOptionalUint64(query, "active_index_generation", input.Expected.ActiveGeneration)
	query = whereOptionalUint64(query, "target_index_generation", input.Expected.TargetGeneration)
	fields := map[string]any{"index_state": input.Next.State, "active_index_generation": input.Next.ActiveGeneration, "target_index_generation": input.Next.TargetGeneration}
	if input.Next.ErrorCode == nil {
		fields["index_error_code"] = nil
	} else {
		fields["index_error_code"] = string(*input.Next.ErrorCode)
	}
	result := query.Updates(fields)
	return result.RowsAffected == 1, result.Error
}

func whereOptionalUint64(query *gorm.DB, column string, value *uint64) *gorm.DB {
	if value == nil {
		return query.Where(column + " IS NULL")
	}
	return query.Where(column+" = ?", *value)
}

func (repository *GormAdminRepository) CreateSpace(ctx context.Context, space ContextSpace) (ContextSpace, error) {
	if repository == nil || repository.db == nil {
		return ContextSpace{}, ErrPlanRepositoryNotConfigured
	}
	if err := repository.db.WithContext(ctx).Create(&space).Error; err != nil {
		return ContextSpace{}, err
	}
	return space, nil
}

func (repository *GormAdminRepository) FindSpace(ctx context.Context, platform string, id uint64) (*ContextSpace, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrPlanRepositoryNotConfigured
	}
	var space ContextSpace
	err := repository.db.WithContext(ctx).Where("id = ? AND platform = ? AND deleted_at IS NULL", id, platform).Take(&space).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &space, nil
}

func (repository *GormAdminRepository) ListSpaces(ctx context.Context, platform string, profileID uint64, status string) ([]ContextSpace, error) {
	var items []ContextSpace
	query := repository.db.WithContext(ctx).Where("platform = ? AND deleted_at IS NULL", platform)
	if profileID != 0 {
		query = query.Where("profile_id = ?", profileID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	err := query.Order("id DESC").Find(&items).Error
	return items, err
}

func (repository *GormAdminRepository) SpaceHasReferences(ctx context.Context, id uint64) (bool, error) {
	if repository == nil || repository.db == nil {
		return false, ErrPlanRepositoryNotConfigured
	}
	var count int64
	err := repository.db.WithContext(ctx).Raw(`SELECT
EXISTS(SELECT 1 FROM ai_context_documents WHERE space_id = ?) +
EXISTS(SELECT 1 FROM ai_context_bindings WHERE space_id = ?) AS ref_count`, id, id).Scan(&count).Error
	return count > 0, err
}

func (repository *GormAdminRepository) UpdateSpace(ctx context.Context, space ContextSpace) (ContextSpace, error) {
	if repository == nil || repository.db == nil {
		return ContextSpace{}, ErrPlanRepositoryNotConfigured
	}
	result := repository.db.WithContext(ctx).Model(&ContextSpace{}).Where("id = ? AND platform = ? AND deleted_at IS NULL", space.ID, space.Platform).
		Updates(map[string]any{"profile_id": space.ProfileID, "name": space.Name, "description": space.Description, "status": space.Status})
	if result.Error != nil {
		return ContextSpace{}, result.Error
	}
	if result.RowsAffected != 1 {
		return ContextSpace{}, gorm.ErrRecordNotFound
	}
	return space, nil
}

func (repository *GormAdminRepository) SoftDeleteSpace(ctx context.Context, platform string, id uint64) error {
	if repository == nil || repository.db == nil {
		return ErrPlanRepositoryNotConfigured
	}
	return repository.db.WithContext(ctx).Model(&ContextSpace{}).Where("id = ? AND platform = ? AND deleted_at IS NULL", id, platform).Update("deleted_at", gorm.Expr("CURRENT_TIMESTAMP(6)")).Error
}

func (repository *GormAdminRepository) CreateDocumentWithVersion(ctx context.Context, document ContextDocument, version ContextDocumentVersion) (DocumentAdminDTO, error) {
	if repository == nil || repository.db == nil {
		return DocumentAdminDTO{}, ErrPlanRepositoryNotConfigured
	}
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&document).Error; err != nil {
			return err
		}
		version.DocumentID = document.ID
		return tx.Create(&version).Error
	})
	if err != nil {
		return DocumentAdminDTO{}, err
	}
	return documentAdminDTO(document, version), nil
}

func (repository *GormAdminRepository) FindDocument(ctx context.Context, platform string, id uint64) (*DocumentAdminDTO, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrPlanRepositoryNotConfigured
	}
	var document ContextDocument
	err := repository.db.WithContext(ctx).Table("ai_context_documents AS d").Select("d.*").
		Joins("JOIN ai_context_spaces AS s ON s.id = d.space_id AND s.deleted_at IS NULL").
		Where("d.id = ? AND d.deleted_at IS NULL AND s.platform = ?", id, platform).Take(&document).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var version ContextDocumentVersion
	err = repository.db.WithContext(ctx).Where("document_id = ?", id).Order("id DESC").Take(&version).Error
	if err != nil {
		return nil, err
	}
	result := documentAdminDTO(document, version)
	return &result, nil
}

func (repository *GormAdminRepository) ListDocuments(ctx context.Context, platform string, spaceID uint64, status string) ([]DocumentAdminDTO, error) {
	var documents []ContextDocument
	query := repository.db.WithContext(ctx).Table("ai_context_documents AS d").Select("d.*").Joins("JOIN ai_context_spaces AS s ON s.id = d.space_id AND s.deleted_at IS NULL").Where("d.space_id = ? AND d.deleted_at IS NULL AND s.platform = ?", spaceID, platform)
	if status != "" {
		query = query.Where("d.status = ?", status)
	}
	if err := query.Order("d.id DESC").Find(&documents).Error; err != nil {
		return nil, err
	}
	items := make([]DocumentAdminDTO, 0, len(documents))
	for _, document := range documents {
		var version ContextDocumentVersion
		if err := repository.db.WithContext(ctx).Where("document_id = ?", document.ID).Order("id DESC").Take(&version).Error; err != nil {
			return nil, err
		}
		items = append(items, documentAdminDTO(document, version))
	}
	return items, nil
}

func (repository *GormAdminRepository) ListDocumentVersions(ctx context.Context, platform string, id uint64) ([]DocumentVersionDTO, error) {
	if document, err := repository.FindDocument(ctx, platform, id); err != nil || document == nil {
		return nil, err
	}
	var rows []ContextDocumentVersion
	if err := repository.db.WithContext(ctx).Where("document_id = ?", id).Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]DocumentVersionDTO, len(rows))
	for index, row := range rows {
		items[index] = documentVersionDTO(row)
	}
	return items, nil
}

func (repository *GormAdminRepository) UpdateDocumentStatus(ctx context.Context, platform string, id uint64, status string) (DocumentAdminDTO, error) {
	result := repository.db.WithContext(ctx).Model(&ContextDocument{}).Where("id = ? AND deleted_at IS NULL AND EXISTS (SELECT 1 FROM ai_context_spaces s WHERE s.id = ai_context_documents.space_id AND s.platform = ? AND s.deleted_at IS NULL)", id, platform).Update("status", status)
	if result.Error != nil {
		return DocumentAdminDTO{}, result.Error
	}
	if result.RowsAffected != 1 {
		return DocumentAdminDTO{}, gorm.ErrRecordNotFound
	}
	item, err := repository.FindDocument(ctx, platform, id)
	if err != nil {
		return DocumentAdminDTO{}, err
	}
	return *item, nil
}

func (repository *GormAdminRepository) SoftDeleteDocument(ctx context.Context, platform string, id uint64) error {
	return repository.db.WithContext(ctx).Model(&ContextDocument{}).Where("id = ? AND deleted_at IS NULL AND EXISTS (SELECT 1 FROM ai_context_spaces s WHERE s.id = ai_context_documents.space_id AND s.platform = ? AND s.deleted_at IS NULL)", id, platform).Update("deleted_at", gorm.Expr("CURRENT_TIMESTAMP(6)")).Error
}

func (repository *GormAdminRepository) GetAgentContextProfile(ctx context.Context, agentID uint64) (*uint64, error) {
	var row struct {
		ProfileID *uint64 `gorm:"column:context_profile_id"`
	}
	err := repository.db.WithContext(ctx).Table("ai_agents").Select("context_profile_id").Where("id = ? AND is_del = ?", agentID, enum.CommonNo).Take(&row).Error
	return cloneUint64(row.ProfileID), err
}

func (repository *GormAdminRepository) SetAgentContextProfile(ctx context.Context, agentID uint64, profileID *uint64) error {
	return repository.db.WithContext(ctx).Table("ai_agents").Where("id = ? AND is_del = ?", agentID, enum.CommonNo).Update("context_profile_id", profileID).Error
}

func (repository *GormAdminRepository) ListAgentContextSpaces(ctx context.Context, agentID uint64) ([]uint64, error) {
	var ids []uint64
	err := repository.db.WithContext(ctx).Table("ai_context_bindings").Where("agent_id = ? AND status = ?", agentID, SpaceEnabled).Order("id ASC").Pluck("space_id", &ids).Error
	return ids, err
}

func (repository *GormAdminRepository) ReplaceAgentContextSpaces(ctx context.Context, agentID uint64, ids []uint64) error {
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var agent struct {
			ProfileID *uint64 `gorm:"column:context_profile_id"`
		}
		if err := tx.Table("ai_agents").Select("context_profile_id").Where("id = ? AND status = ? AND is_del = ?", agentID, enum.CommonYes, enum.CommonNo).Take(&agent).Error; err != nil {
			return err
		}
		if len(ids) != 0 {
			if agent.ProfileID == nil {
				return errors.New("agent context profile is required before assigning spaces")
			}
			var compatible int64
			if err := tx.Table("ai_context_spaces").Where("id IN ? AND platform = ? AND profile_id = ? AND status = ? AND deleted_at IS NULL", ids, enum.PlatformAdmin, *agent.ProfileID, SpaceEnabled).Count(&compatible).Error; err != nil {
				return err
			}
			if compatible != int64(len(ids)) {
				return errors.New("context spaces are missing, disabled, or use a different profile")
			}
		}
		if err := tx.Exec("DELETE FROM ai_context_bindings WHERE agent_id = ?", agentID).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		rows := make([]map[string]any, len(ids))
		for index, id := range ids {
			rows[index] = map[string]any{"agent_id": agentID, "space_id": id, "status": SpaceEnabled}
		}
		return tx.Table("ai_context_bindings").Create(&rows).Error
	})
}

func (repository *GormAdminRepository) CreateDocumentVersion(ctx context.Context, version ContextDocumentVersion) (DocumentAdminDTO, error) {
	if repository == nil || repository.db == nil {
		return DocumentAdminDTO{}, ErrPlanRepositoryNotConfigured
	}
	if err := repository.db.WithContext(ctx).Create(&version).Error; err != nil {
		return DocumentAdminDTO{}, err
	}
	var document ContextDocument
	if err := repository.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", version.DocumentID).Take(&document).Error; err != nil {
		return DocumentAdminDTO{}, err
	}
	return documentAdminDTO(document, version), nil
}

func (repository *GormAdminRepository) AgentProfileChangeConflict(ctx context.Context, agentID uint64) (bool, error) {
	if repository == nil || repository.db == nil {
		return false, ErrPlanRepositoryNotConfigured
	}
	var count int64
	err := repository.db.WithContext(ctx).Raw(`SELECT
EXISTS(SELECT 1 FROM ai_context_bindings WHERE agent_id = ? AND status = 'enabled') +
EXISTS(SELECT 1 FROM ai_context_documents d JOIN ai_conversations c ON c.id=d.conversation_id WHERE c.agent_id=? AND d.active_version_id IS NOT NULL) +
EXISTS(SELECT 1 FROM ai_conversation_memories WHERE agent_id = ?) AS ref_count`, agentID, agentID, agentID).Scan(&count).Error
	return count > 0, err
}

func NewPlanRepository(client *database.Client) *GormPlanRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormPlanRepository{db: client.Gorm}
}

func NewPlanRepositoryFromGorm(db *gorm.DB) *GormPlanRepository {
	if db == nil {
		return nil
	}
	return &GormPlanRepository{db: db}
}

func (repository *GormPlanRepository) FindTerminalByRunIDs(ctx context.Context, runIDs []uint64) (map[uint64]ContextPlan, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrPlanRepositoryNotConfigured
	}
	uniqueRunIDs := make([]uint64, 0, len(runIDs))
	seenRunIDs := make(map[uint64]struct{}, len(runIDs))
	for _, runID := range runIDs {
		if runID == 0 {
			return nil, ErrInvalidContextPlan
		}
		if _, exists := seenRunIDs[runID]; exists {
			continue
		}
		seenRunIDs[runID] = struct{}{}
		uniqueRunIDs = append(uniqueRunIDs, runID)
	}
	if len(uniqueRunIDs) == 0 {
		return map[uint64]ContextPlan{}, nil
	}

	var planRows []contextPlanRow
	if err := repository.db.WithContext(ctx).Where("run_id IN ?", uniqueRunIDs).Order("id ASC").Find(&planRows).Error; err != nil {
		return nil, fmt.Errorf("find terminal context plans: %w", err)
	}
	if len(planRows) == 0 {
		return map[uint64]ContextPlan{}, nil
	}
	planIDs := make([]uint64, 0, len(planRows))
	for _, row := range planRows {
		planIDs = append(planIDs, row.ID)
	}
	var itemRows []contextPlanItemRow
	if err := repository.db.WithContext(ctx).Where("plan_id IN ?", planIDs).Order("plan_id ASC, ordinal ASC").Find(&itemRows).Error; err != nil {
		return nil, fmt.Errorf("find terminal context plan items: %w", err)
	}
	itemsByPlanID := make(map[uint64][]contextPlanItemRow, len(planRows))
	for _, row := range itemRows {
		itemsByPlanID[row.PlanID] = append(itemsByPlanID[row.PlanID], row)
	}
	result := make(map[uint64]ContextPlan, len(planRows))
	for _, row := range planRows {
		if _, duplicate := result[row.RunID]; duplicate {
			return nil, ErrInvalidContextPlan
		}
		plan, err := contextPlanFromRows(row, itemsByPlanID[row.ID])
		if err != nil {
			return nil, fmt.Errorf("decode terminal context plan: %w", err)
		}
		result[row.RunID] = plan
	}
	return result, nil
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
