package contextengine

import (
	"context"
	"crypto/sha256"
	"strings"

	"admin_back_go/internal/module/ai/officialmodel"
	aiprovider "admin_back_go/internal/module/ai/provider"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

func verifyFingerprintAuthority(ctx context.Context, tx *gorm.DB, fingerprint InputFingerprintHashInput) error {
	var agent struct {
		ID               uint64  `gorm:"column:id"`
		ProviderID       uint64  `gorm:"column:provider_id"`
		ModelID          string  `gorm:"column:model_id"`
		SystemPrompt     string  `gorm:"column:system_prompt"`
		ContextProfileID *uint64 `gorm:"column:context_profile_id"`
		Status           int     `gorm:"column:status"`
		IsDel            int     `gorm:"column:is_del"`
	}
	if err := tx.WithContext(ctx).Table("ai_agents").Where("id = ?", fingerprint.AgentID).Take(&agent).Error; err != nil {
		return err
	}
	agentHash, err := hashRuntimeFacts(struct {
		ID           uint64
		ProviderID   uint64
		ModelID      string
		SystemPrompt string
		ProfileID    *uint64
	}{agent.ID, agent.ProviderID, agent.ModelID, agent.SystemPrompt, agent.ContextProfileID})
	if err != nil || agentHash != fingerprint.AgentSHA256 || agent.ProviderID != fingerprint.ProviderID || agent.ModelID != fingerprint.ModelID ||
		agent.Status != enum.CommonYes || agent.IsDel != enum.CommonNo {
		return ErrInvalidContextPlan
	}
	var provider struct {
		ID       uint64 `gorm:"column:id"`
		Engine   string `gorm:"column:engine_type"`
		BaseURL  string `gorm:"column:base_url"`
		Protocol string `gorm:"column:api_protocol"`
		Status   int    `gorm:"column:status"`
		IsDel    int    `gorm:"column:is_del"`
	}
	if err := tx.WithContext(ctx).Table("ai_providers").Where("id = ?", fingerprint.ProviderID).Take(&provider).Error; err != nil {
		return err
	}
	providerHash, err := hashRuntimeFacts(struct {
		ID       uint64
		Engine   string
		BaseURL  string
		Protocol string
	}{provider.ID, provider.Engine, provider.BaseURL, provider.Protocol})
	if err != nil || providerHash != fingerprint.ProviderSHA256 || provider.Status != enum.CommonYes || provider.IsDel != enum.CommonNo {
		return ErrInvalidContextPlan
	}
	var providerModel struct {
		ID              uint64                      `gorm:"column:id"`
		ProviderID      uint64                      `gorm:"column:provider_id"`
		ModelID         string                      `gorm:"column:model_id"`
		ModelKind       string                      `gorm:"column:model_kind"`
		Status          int                         `gorm:"column:status"`
		OfficialModelID *string                     `gorm:"column:official_model_id"`
		OfficialCatalog *string                     `gorm:"column:official_catalog_version"`
		MappingStatus   officialmodel.MappingStatus `gorm:"column:mapping_status"`
	}
	if err := tx.WithContext(ctx).Table("ai_provider_models").Where("id = ?", fingerprint.ProviderModelID).Take(&providerModel).Error; err != nil {
		return err
	}
	if providerModel.ProviderID != fingerprint.ProviderID || providerModel.ModelID != fingerprint.ModelID ||
		providerModel.ModelKind != string(aiprovider.ModelKindChat) || providerModel.Status != enum.CommonYes ||
		providerModel.MappingStatus != officialmodel.MappingStatusMapped || providerModel.OfficialModelID == nil || providerModel.OfficialCatalog == nil {
		return ErrInvalidContextPlan
	}
	model, err := officialmodel.Default.ResolveIdentity(fingerprint.ModelID)
	if err != nil || model.ModelID != strings.TrimSpace(*providerModel.OfficialModelID) || model.CatalogVersion != strings.TrimSpace(*providerModel.OfficialCatalog) {
		return ErrInvalidContextPlan
	}
	capability, err := runtimeModelCapability(runtimeIdentityRow{
		ProviderID: fingerprint.ProviderID, ProviderModelID: fingerprint.ProviderModelID,
		AgentModelID: fingerprint.ModelID, ProviderAPIProtocol: provider.Protocol,
	}, officialmodel.ResolvedModel{Model: model})
	if err != nil {
		return err
	}
	capabilityHash, err := HashModelCapability(capability)
	if err != nil || capabilityHash != fingerprint.ModelCapabilitySHA256 || capability.MaxOutputTokens != fingerprint.Generation.EffectiveMaxOutputTokens {
		return ErrInvalidContextPlan
	}
	if fingerprint.Profile != nil {
		var profile ContextProfile
		if err := tx.WithContext(ctx).Where("id = ?", fingerprint.Profile.ID).Take(&profile).Error; err != nil {
			return err
		}
		current, err := runtimeProfileSnapshot(profile)
		if err != nil || current.ID != fingerprint.Profile.ID || current.SHA256 != fingerprint.Profile.SHA256 ||
			!equalGeneration(current.IndexGeneration, fingerprint.Profile.IndexGeneration) {
			return ErrInvalidContextPlan
		}
	}
	for _, message := range fingerprint.Messages {
		var row struct {
			Content  string  `gorm:"column:content"`
			MetaJSON *string `gorm:"column:meta_json"`
			Role     int     `gorm:"column:role"`
			IsDel    int     `gorm:"column:is_del"`
		}
		if err := tx.WithContext(ctx).Table("ai_messages").Where("id = ?", message.ID).Take(&row).Error; err != nil {
			return err
		}
		attachments, err := runtimeAttachments(row.MetaJSON)
		if err != nil || row.IsDel != enum.CommonNo || enum.AIMessageRoleLabels[row.Role] != string(message.Role) ||
			sha256.Sum256([]byte(row.Content)) != message.ContentSHA256 || !sameFingerprintAttachments(message.Attachments, attachments) {
			return ErrInvalidContextPlan
		}
	}
	for _, binding := range fingerprint.Bindings {
		var row runtimeBindingRow
		if err := tx.WithContext(ctx).Table("ai_context_bindings AS binding").Select("binding.id, binding.space_id, space.profile_id").
			Joins("JOIN ai_context_spaces AS space ON space.id = binding.space_id AND space.deleted_at IS NULL").
			Where("binding.id = ? AND binding.agent_id = ? AND binding.status = ? AND space.status = ?", binding.ID, fingerprint.AgentID, "enabled", SpaceEnabled).Take(&row).Error; err != nil {
			return err
		}
		hash, hashErr := hashRuntimeFacts(row)
		if hashErr != nil || hash != binding.SHA256 || row.SpaceID != binding.SpaceID {
			return ErrInvalidContextPlan
		}
	}
	for _, tool := range fingerprint.Tools {
		var row runtimeToolRow
		if err := tx.WithContext(ctx).Table("ai_agent_tools AS binding").
			Select("binding.id AS binding_id, tool.id AS tool_id, tool.code, tool.description, tool.parameters_json").
			Joins("JOIN ai_tools AS tool ON tool.id = binding.tool_id").
			Where("binding.agent_id = ? AND binding.tool_id = ? AND binding.status = ? AND tool.status = ? AND tool.is_del = ?",
				fingerprint.AgentID, tool.ID, enum.CommonYes, enum.CommonYes, enum.CommonNo).Take(&row).Error; err != nil {
			return err
		}
		hash, hashErr := hashRuntimeFacts(struct {
			ID          uint64
			Code        string
			Description string
			Parameters  string
		}{row.ToolID, row.Code, row.Description, row.ParametersJSON})
		if hashErr != nil || row.ToolID != tool.ID || row.Code != tool.Name || hash != tool.DefinitionSHA256 {
			return ErrInvalidContextPlan
		}
	}
	return nil
}

func sameFingerprintAttachments(expected []FingerprintAttachment, actual []runtimeAttachment) bool {
	if len(expected) != len(actual) {
		return false
	}
	for index := range expected {
		left, right := expected[index], actual[index]
		if left.Ordinal != uint32(index) || left.Kind != right.Kind || left.URL != right.URL || left.ObjectKey != right.ObjectKey ||
			left.ETag != right.ETag || left.Size != right.Size || left.MIMEType != right.MIMEType || left.Filename != right.Filename {
			return false
		}
	}
	return true
}
