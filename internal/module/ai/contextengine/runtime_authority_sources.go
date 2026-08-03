package contextengine

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"

	"admin_back_go/internal/infra/contextindex"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

func verifySelectedSource(ctx context.Context, tx *gorm.DB, platform string, fingerprint InputFingerprintHashInput, source AuthoritySource) error {
	if handled, err := verifySelectedFingerprintSource(fingerprint, source); handled {
		return err
	}
	switch source.SourceType {
	case "document_chunk":
		if fingerprint.Profile == nil || fingerprint.Profile.IndexGeneration == nil {
			return ErrInvalidContextPlan
		}
		chunkIDs, err := parseDocumentChunkAuthorityRef(source.SourceRef)
		if err != nil {
			return err
		}
		var rows []struct {
			ID     uint64 `gorm:"column:id"`
			SHA256 []byte `gorm:"column:chunk_facts_sha256"`
		}
		if err := tx.WithContext(ctx).Table("ai_context_chunks").Select("id, chunk_facts_sha256").Where("id IN ?", chunkIDs).Find(&rows).Error; err != nil {
			return err
		}
		byID := make(map[uint64][sha256.Size]byte, len(rows))
		for _, row := range rows {
			hash, hashErr := SHA256FromBytes(row.SHA256)
			if hashErr != nil {
				return ErrInvalidContextPlan
			}
			byID[row.ID] = hash
		}
		hashes := make([][sha256.Size]byte, len(chunkIDs))
		for index, chunkID := range chunkIDs {
			var exists bool
			hashes[index], exists = byID[chunkID]
			if !exists {
				return ErrInvalidContextPlan
			}
		}
		scope, err := loadFingerprintAuthorityScope(ctx, tx, fingerprint)
		if err != nil {
			return err
		}
		candidates := make([]Candidate, len(chunkIDs))
		for index, chunkID := range chunkIDs {
			candidates[index].Point = contextindex.PointRef{
				ProfileID: fingerprint.Profile.ID, IndexGeneration: *fingerprint.Profile.IndexGeneration,
				SourceKind: contextindex.SourceKindDocumentChunk, SourceID: chunkID, SourceSHA256: hashes[index],
			}
		}
		repository := NewCandidateRepositoryWithDB(tx, NewConversationRepositoryWithDB(tx))
		verification, err := repository.VerifyCandidates(ctx, CandidateAuthoritySnapshot{
			ProfileID: fingerprint.Profile.ID, IndexGeneration: *fingerprint.Profile.IndexGeneration,
			AgentID: fingerprint.AgentID, UserID: scope.UserID, ConversationID: scope.ConversationID, Platform: platform,
		}, candidates)
		if err != nil {
			return err
		}
		if len(verification.Authorized) != len(chunkIDs) || len(verification.Excluded) != 0 {
			return ErrInvalidContextPlan
		}
		hash, err := documentChunkSourceSHA256(chunkIDs, hashes)
		if err != nil || hash != source.SourceSHA256 {
			return ErrInvalidContextPlan
		}
	case "conversation_turn":
		id, err := parseAuthorityID(source.SourceRef, "conversation_turn:")
		if err != nil {
			return err
		}
		scope, err := loadFingerprintAuthorityScope(ctx, tx, fingerprint)
		if err != nil {
			return err
		}
		turns, err := NewConversationRepositoryWithDB(tx).CompleteByAnchors(ctx, scope.ConversationID, scope.UserID, []uint64{id})
		if err != nil {
			return err
		}
		if len(turns) != 1 || turns[0].UserMessage.ID != id || turns[0].SourceSHA256 != source.SourceSHA256 {
			return ErrInvalidContextPlan
		}
	case "conversation_memory":
		if fingerprint.Profile == nil {
			return ErrInvalidContextPlan
		}
		id, err := parseAuthorityID(source.SourceRef, "conversation_memory:")
		if err != nil {
			return err
		}
		scope, err := loadFingerprintAuthorityScope(ctx, tx, fingerprint)
		if err != nil {
			return err
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
		if err := validateDispatchMemory(row, parent, scope.ConversationID, fingerprint.Profile.ID, fingerprint.Profile.SHA256, source); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown authority source %q", source.SourceType)
	}
	return nil
}

type fingerprintAuthorityScope struct {
	ConversationID uint64 `gorm:"column:conversation_id"`
	UserID         uint64 `gorm:"column:user_id"`
}

func loadFingerprintAuthorityScope(ctx context.Context, tx *gorm.DB, fingerprint InputFingerprintHashInput) (fingerprintAuthorityScope, error) {
	if len(fingerprint.Messages) == 0 {
		return fingerprintAuthorityScope{}, ErrInvalidContextPlan
	}
	var scope fingerprintAuthorityScope
	if err := tx.WithContext(ctx).Table("ai_messages AS message").Select("message.conversation_id, conversation.user_id").
		Joins("JOIN ai_conversations AS conversation ON conversation.id = message.conversation_id AND conversation.is_del = ?", enum.CommonNo).
		Where("message.id = ? AND message.is_del = ?", fingerprint.Messages[0].ID, enum.CommonNo).Take(&scope).Error; err != nil {
		return fingerprintAuthorityScope{}, err
	}
	if scope.ConversationID == 0 || scope.UserID == 0 {
		return fingerprintAuthorityScope{}, ErrInvalidContextPlan
	}
	return scope, nil
}

func verifySelectedFingerprintSource(fingerprint InputFingerprintHashInput, source AuthoritySource) (bool, error) {
	switch source.SourceType {
	case "agent":
		id, err := parseAuthorityID(source.SourceRef, "agent:")
		if err != nil || id != fingerprint.AgentID || source.SourceSHA256 != fingerprint.AgentSHA256 {
			return true, ErrInvalidContextPlan
		}
		return true, nil
	case "message":
		id, err := parseAuthorityID(source.SourceRef, "message:")
		if err != nil {
			return true, ErrInvalidContextPlan
		}
		for _, message := range fingerprint.Messages {
			if message.ID == id && message.ContentSHA256 == source.SourceSHA256 {
				return true, nil
			}
		}
		return true, ErrInvalidContextPlan
	case "tool":
		id, err := parseAuthorityID(source.SourceRef, "tool:")
		if err != nil {
			return true, ErrInvalidContextPlan
		}
		for _, tool := range fingerprint.Tools {
			if tool.ID == id && tool.DefinitionSHA256 == source.SourceSHA256 {
				return true, nil
			}
		}
		return true, ErrInvalidContextPlan
	case "attachment":
		messageID, ordinal, err := parseAttachmentAuthorityRef(source.SourceRef)
		if err != nil {
			return true, ErrInvalidContextPlan
		}
		for _, message := range fingerprint.Messages {
			if message.ID != messageID || ordinal >= uint64(len(message.Attachments)) {
				continue
			}
			attachment := message.Attachments[ordinal]
			if attachment.Ordinal != uint32(ordinal) {
				return true, ErrInvalidContextPlan
			}
			hash, hashErr := hashRuntimeFacts(runtimeAttachment{
				Kind: attachment.Kind, URL: attachment.URL, ObjectKey: attachment.ObjectKey, ETag: attachment.ETag,
				Size: attachment.Size, MIMEType: attachment.MIMEType, Filename: attachment.Filename,
			})
			if hashErr != nil || hash != source.SourceSHA256 {
				return true, ErrInvalidContextPlan
			}
			return true, nil
		}
		return true, ErrInvalidContextPlan
	default:
		return false, nil
	}
}

func parseAuthorityID(ref, prefix string) (uint64, error) {
	if !strings.HasPrefix(ref, prefix) || strings.Contains(strings.TrimPrefix(ref, prefix), "/") || strings.Contains(strings.TrimPrefix(ref, prefix), ",") {
		return 0, ErrInvalidContextPlan
	}
	id, err := strconv.ParseUint(strings.TrimPrefix(ref, prefix), 10, 64)
	if err != nil || id == 0 {
		return 0, ErrInvalidContextPlan
	}
	return id, nil
}

func parseAttachmentAuthorityRef(ref string) (uint64, uint64, error) {
	parts := strings.Split(ref, "/attachment:")
	if len(parts) != 2 {
		return 0, 0, ErrInvalidContextPlan
	}
	messageID, err := parseAuthorityID(parts[0], "message:")
	if err != nil {
		return 0, 0, err
	}
	ordinal, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return 0, 0, ErrInvalidContextPlan
	}
	return messageID, ordinal, nil
}

func parseDocumentChunkAuthorityRef(ref string) ([]uint64, error) {
	if !strings.HasPrefix(ref, "document_chunks:") {
		return nil, ErrInvalidContextPlan
	}
	parts := strings.Split(strings.TrimPrefix(ref, "document_chunks:"), ",")
	ids := make([]uint64, len(parts))
	seen := make(map[uint64]struct{}, len(parts))
	for index, part := range parts {
		id, err := strconv.ParseUint(part, 10, 64)
		if err != nil || id == 0 {
			return nil, ErrInvalidContextPlan
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, ErrInvalidContextPlan
		}
		seen[id] = struct{}{}
		ids[index] = id
	}
	return ids, nil
}
