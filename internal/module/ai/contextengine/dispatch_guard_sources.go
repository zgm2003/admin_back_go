package contextengine

import (
	"context"
	"crypto/sha256"
	"errors"

	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

func verifyDispatchSources(ctx context.Context, tx *gorm.DB, platform string, facts dispatchGuardFacts) error {
	for _, item := range facts.SelectedItems {
		hash, err := SHA256FromBytes(item.SourceSHA256)
		if err != nil {
			return errDispatchPlanConflict
		}
		source := AuthoritySource{SourceType: item.SourceType, SourceRef: item.SourceRef, SourceSHA256: hash}
		switch source.SourceType {
		case "agent":
			err = verifyDispatchAgent(ctx, tx, facts.Run, source)
		case "message":
			err = verifyDispatchMessage(ctx, tx, facts.Run, source)
		case "attachment":
			err = verifyDispatchAttachment(ctx, tx, facts.Run, source)
		case "tool":
			err = verifyDispatchTool(ctx, tx, facts.Run, source)
		case "conversation_turn":
			err = verifyDispatchConversationTurn(ctx, tx, facts.Run, source)
		case "conversation_memory":
			err = verifyDispatchMemory(ctx, tx, facts, source)
		case "document_chunk":
			err = verifyDispatchDocumentChunks(ctx, tx, platform, facts, source)
		default:
			err = errDispatchPermission
		}
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, ErrInvalidContextPlan) || errors.Is(err, errTurnInvalid) {
				return errDispatchPermission
			}
			return err
		}
	}
	return nil
}

func verifyDispatchAgent(ctx context.Context, tx *gorm.DB, run dispatchRunRow, source AuthoritySource) error {
	id, err := parseAuthorityID(source.SourceRef, "agent:")
	if err != nil || id != run.AgentID {
		return errDispatchPermission
	}
	var row struct {
		ID               uint64  `gorm:"column:id"`
		ProviderID       uint64  `gorm:"column:provider_id"`
		ModelID          string  `gorm:"column:model_id"`
		SystemPrompt     string  `gorm:"column:system_prompt"`
		ContextProfileID *uint64 `gorm:"column:context_profile_id"`
		Status           int     `gorm:"column:status"`
		IsDel            int     `gorm:"column:is_del"`
	}
	if err := tx.WithContext(ctx).Table("ai_agents").Where("id = ?", id).Take(&row).Error; err != nil {
		return err
	}
	hash, err := hashRuntimeFacts(struct {
		ID           uint64
		ProviderID   uint64
		ModelID      string
		SystemPrompt string
		ProfileID    *uint64
	}{row.ID, row.ProviderID, row.ModelID, row.SystemPrompt, row.ContextProfileID})
	if err != nil {
		return err
	}
	if row.Status != enum.CommonYes || row.IsDel != enum.CommonNo || hash != source.SourceSHA256 {
		return errDispatchPermission
	}
	return nil
}

type dispatchMessageRow struct {
	ID             uint64  `gorm:"column:id"`
	ConversationID uint64  `gorm:"column:conversation_id"`
	Content        string  `gorm:"column:content"`
	MetaJSON       *string `gorm:"column:meta_json"`
	Role           int     `gorm:"column:role"`
	IsDel          int     `gorm:"column:is_del"`
}

func loadDispatchMessage(ctx context.Context, tx *gorm.DB, run dispatchRunRow, messageID uint64) (dispatchMessageRow, error) {
	if run.ConversationID == nil || run.UserMessageID == nil || messageID != *run.UserMessageID {
		return dispatchMessageRow{}, errDispatchPermission
	}
	var row dispatchMessageRow
	if err := tx.WithContext(ctx).Table("ai_messages").Where("id = ?", messageID).Take(&row).Error; err != nil {
		return dispatchMessageRow{}, err
	}
	if row.ConversationID != *run.ConversationID || row.Role != enum.AIMessageRoleUser || row.IsDel != enum.CommonNo {
		return dispatchMessageRow{}, errDispatchPermission
	}
	return row, nil
}

func verifyDispatchMessage(ctx context.Context, tx *gorm.DB, run dispatchRunRow, source AuthoritySource) error {
	id, err := parseAuthorityID(source.SourceRef, "message:")
	if err != nil {
		return errDispatchPermission
	}
	row, err := loadDispatchMessage(ctx, tx, run, id)
	if err != nil {
		return err
	}
	if sha256.Sum256([]byte(row.Content)) != source.SourceSHA256 {
		return errDispatchPermission
	}
	return nil
}

func verifyDispatchAttachment(ctx context.Context, tx *gorm.DB, run dispatchRunRow, source AuthoritySource) error {
	messageID, ordinal, err := parseAttachmentAuthorityRef(source.SourceRef)
	if err != nil {
		return errDispatchPermission
	}
	row, err := loadDispatchMessage(ctx, tx, run, messageID)
	if err != nil {
		return err
	}
	attachments, err := runtimeAttachments(row.MetaJSON)
	if err != nil || ordinal >= uint64(len(attachments)) {
		return errDispatchPermission
	}
	hash, err := hashRuntimeFacts(attachments[ordinal])
	if err != nil {
		return err
	}
	if hash != source.SourceSHA256 {
		return errDispatchPermission
	}
	return nil
}

func verifyDispatchTool(ctx context.Context, tx *gorm.DB, run dispatchRunRow, source AuthoritySource) error {
	id, err := parseAuthorityID(source.SourceRef, "tool:")
	if err != nil {
		return errDispatchPermission
	}
	var row runtimeToolRow
	if err := tx.WithContext(ctx).Table("ai_agent_tools AS binding").
		Select("binding.id AS binding_id, tool.id AS tool_id, tool.code, tool.description, tool.parameters_json").
		Joins("JOIN ai_tools AS tool ON tool.id = binding.tool_id").
		Where("binding.agent_id = ? AND binding.tool_id = ? AND binding.status = ? AND tool.status = ? AND tool.is_del = ?",
			run.AgentID, id, enum.CommonYes, enum.CommonYes, enum.CommonNo).Take(&row).Error; err != nil {
		return err
	}
	hash, err := hashRuntimeFacts(struct {
		ID          uint64
		Code        string
		Description string
		Parameters  string
	}{row.ToolID, row.Code, row.Description, row.ParametersJSON})
	if err != nil {
		return err
	}
	if row.ToolID != id || hash != source.SourceSHA256 {
		return errDispatchPermission
	}
	return nil
}

func verifyDispatchConversationTurn(ctx context.Context, tx *gorm.DB, run dispatchRunRow, source AuthoritySource) error {
	anchorID, err := parseAuthorityID(source.SourceRef, "conversation_turn:")
	if err != nil || run.ConversationID == nil {
		return errDispatchPermission
	}
	turns, err := NewConversationRepositoryWithDB(tx).CompleteByAnchors(ctx, *run.ConversationID, run.UserID, []uint64{anchorID})
	if err != nil {
		return err
	}
	if len(turns) != 1 || turns[0].UserMessage.ID != anchorID || turns[0].SourceSHA256 != source.SourceSHA256 {
		return errDispatchPermission
	}
	return nil
}

func verifyDispatchMemory(ctx context.Context, tx *gorm.DB, facts dispatchGuardFacts, source AuthoritySource) error {
	id, err := parseAuthorityID(source.SourceRef, "conversation_memory:")
	if err != nil || facts.Plan.ContextProfileIDSnapshot == nil || facts.Run.ConversationID == nil {
		return errDispatchPermission
	}
	profileHash, err := SHA256FromBytes(facts.Plan.ContextProfileSHA256)
	if err != nil {
		return errDispatchPlanConflict
	}
	var row MemoryRecord
	if err := tx.WithContext(ctx).Where("id = ?", id).Take(&row).Error; err != nil {
		return err
	}
	var parent *MemoryRecord
	if row.ParentMemoryID != nil {
		var parentRow MemoryRecord
		if err := tx.WithContext(ctx).Where("id = ?", *row.ParentMemoryID).Take(&parentRow).Error; err != nil {
			return err
		}
		parent = &parentRow
	}
	return validateDispatchMemory(row, parent, *facts.Run.ConversationID, *facts.Plan.ContextProfileIDSnapshot, profileHash, source)
}

func validateDispatchMemory(row MemoryRecord, parent *MemoryRecord, conversationID, profileID uint64, profileSHA256 [sha256.Size]byte, source AuthoritySource) error {
	id, err := parseAuthorityID(source.SourceRef, "conversation_memory:")
	if err != nil || source.SourceType != "conversation_memory" || row.ID != id || row.ConversationID != conversationID || row.ProfileID != profileID ||
		!readyMemoryRowValid(row, profileSHA256) {
		return errDispatchPermission
	}
	storedSource, err := SHA256FromBytes(row.SourceSHA256)
	if err != nil || storedSource != source.SourceSHA256 {
		return errDispatchPermission
	}
	if row.ParentMemoryID == nil {
		if parent != nil {
			return errDispatchPermission
		}
		return nil
	}
	if parent == nil || !readyMemoryRowValid(*parent, profileSHA256) {
		return errDispatchPermission
	}
	candidate := MemoryCandidate{ID: row.ID, ConversationID: row.ConversationID, ProfileID: row.ProfileID,
		ParentMemoryID: row.ParentMemoryID, FromMessageID: row.FromMessageID, ThroughMessageID: row.ThroughMessageID}
	if ValidateMemoryParent(candidate, *parent) != nil {
		return errDispatchPermission
	}
	return nil
}

type dispatchDocumentChunkRow struct {
	ChunkID                uint64  `gorm:"column:chunk_id"`
	ChunkFactsSHA256       []byte  `gorm:"column:chunk_facts_sha256"`
	VersionProfileID       uint64  `gorm:"column:version_profile_id"`
	VersionState           string  `gorm:"column:version_state"`
	DocumentStatus         string  `gorm:"column:document_status"`
	DocumentSpaceID        *uint64 `gorm:"column:document_space_id"`
	DocumentConversationID *uint64 `gorm:"column:document_conversation_id"`
	SpaceProfileID         *uint64 `gorm:"column:space_profile_id"`
	SpacePlatform          *string `gorm:"column:space_platform"`
	SpaceStatus            *string `gorm:"column:space_status"`
	BindingStatus          *string `gorm:"column:binding_status"`
	ConversationUserID     *uint64 `gorm:"column:conversation_user_id"`
}

func verifyDispatchDocumentChunks(ctx context.Context, tx *gorm.DB, platform string, facts dispatchGuardFacts, source AuthoritySource) error {
	chunkIDs, err := parseDocumentChunkAuthorityRef(source.SourceRef)
	if err != nil || facts.Plan.ContextProfileIDSnapshot == nil || runConversationID(facts.Run) == 0 {
		return errDispatchPermission
	}
	var rows []dispatchDocumentChunkRow
	err = tx.WithContext(ctx).Table("ai_context_chunks AS chunk").Select(`
		chunk.id AS chunk_id, chunk.chunk_facts_sha256,
		version.profile_id AS version_profile_id, version.state AS version_state,
		document.status AS document_status, document.space_id AS document_space_id,
		document.conversation_id AS document_conversation_id,
		space.profile_id AS space_profile_id, space.platform AS space_platform, space.status AS space_status,
		binding.status AS binding_status, conversation.user_id AS conversation_user_id`).
		Joins("JOIN ai_context_document_versions AS version ON version.id = chunk.document_version_id").
		Joins("JOIN ai_context_documents AS document ON document.id = version.document_id AND document.deleted_at IS NULL").
		Joins("LEFT JOIN ai_context_spaces AS space ON space.id = document.space_id AND space.deleted_at IS NULL").
		Joins("LEFT JOIN ai_context_bindings AS binding ON binding.space_id = space.id AND binding.agent_id = ?", facts.Run.AgentID).
		Joins("LEFT JOIN ai_conversations AS conversation ON conversation.id = document.conversation_id AND conversation.is_del = ?", enum.CommonNo).
		Where("chunk.id IN ?", chunkIDs).Find(&rows).Error
	if err != nil {
		return err
	}
	byID := make(map[uint64]dispatchDocumentChunkRow, len(rows))
	for _, row := range rows {
		if _, duplicate := byID[row.ChunkID]; duplicate {
			return errDispatchPlanConflict
		}
		byID[row.ChunkID] = row
	}
	hashes := make([][sha256.Size]byte, len(chunkIDs))
	for index, chunkID := range chunkIDs {
		row, ok := byID[chunkID]
		if !ok || row.VersionProfileID != *facts.Plan.ContextProfileIDSnapshot || row.VersionState != DocumentVersionReady ||
			row.DocumentStatus != DocumentEnabled {
			return errDispatchPermission
		}
		spaceAuthorized := row.DocumentSpaceID != nil && row.SpaceProfileID != nil && *row.SpaceProfileID == *facts.Plan.ContextProfileIDSnapshot &&
			row.SpacePlatform != nil && *row.SpacePlatform == platform && row.SpaceStatus != nil && *row.SpaceStatus == SpaceEnabled &&
			row.BindingStatus != nil && *row.BindingStatus == "enabled"
		conversationAuthorized := row.DocumentConversationID != nil && *row.DocumentConversationID == runConversationID(facts.Run) &&
			row.ConversationUserID != nil && *row.ConversationUserID == facts.Run.UserID
		if !spaceAuthorized && !conversationAuthorized {
			return errDispatchPermission
		}
		hashes[index], err = SHA256FromBytes(row.ChunkFactsSHA256)
		if err != nil {
			return errDispatchPlanConflict
		}
	}
	combined, err := documentChunkSourceSHA256(chunkIDs, hashes)
	if err != nil || combined != source.SourceSHA256 {
		return errDispatchPermission
	}
	return nil
}

func runConversationID(run dispatchRunRow) uint64 {
	if run.ConversationID == nil {
		return 0
	}
	return *run.ConversationID
}
