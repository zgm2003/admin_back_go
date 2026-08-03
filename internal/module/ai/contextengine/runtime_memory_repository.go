package contextengine

import (
	"context"
	"crypto/sha256"
	"strings"
)

func (repository *GormMemoryRepository) LatestReadyMemory(ctx context.Context, conversationID, profileID uint64, profileSHA256 [sha256.Size]byte) (*MemoryRecord, error) {
	if repository == nil || repository.db == nil || conversationID == 0 || profileID == 0 || profileSHA256 == ([sha256.Size]byte{}) {
		return nil, ErrMemoryInvalid
	}
	var rows []MemoryRecord
	if err := repository.db.WithContext(ctx).
		Where("conversation_id = ? AND context_profile_id_snapshot = ? AND state = ?", conversationID, profileID, MemoryStateReady).
		Order("through_message_id ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	valid := make(map[uint64]MemoryRecord, len(rows))
	var latest *MemoryRecord
	for _, row := range rows {
		if !readyMemoryRowValid(row, profileSHA256) {
			continue
		}
		if row.ParentMemoryID != nil {
			parent, ok := valid[*row.ParentMemoryID]
			candidate := MemoryCandidate{ID: row.ID, ConversationID: row.ConversationID, ProfileID: row.ProfileID,
				ParentMemoryID: row.ParentMemoryID, FromMessageID: row.FromMessageID, ThroughMessageID: row.ThroughMessageID}
			if !ok || ValidateMemoryParent(candidate, parent) != nil {
				continue
			}
		}
		valid[row.ID] = row
		copy := row
		latest = &copy
	}
	return latest, nil
}

func readyMemoryRowValid(row MemoryRecord, profileSHA256 [sha256.Size]byte) bool {
	if row.ID == 0 || row.ConversationID == 0 || row.ProfileID == 0 || row.State != MemoryStateReady || row.Summary == nil ||
		strings.TrimSpace(*row.Summary) == "" || row.FromMessageID == 0 || row.ThroughMessageID < row.FromMessageID {
		return false
	}
	profileHash, profileErr := SHA256FromBytes(row.ProfileSHA256)
	sourceHash, sourceErr := SHA256FromBytes(row.SourceSHA256)
	summaryHash, summaryErr := SHA256FromBytes(row.SummarySHA256)
	return profileErr == nil && sourceErr == nil && summaryErr == nil && profileHash == profileSHA256 &&
		sourceHash != ([sha256.Size]byte{}) && summaryHash == sha256.Sum256([]byte(*row.Summary))
}

var _ MemoryContextReader = (*GormMemoryRepository)(nil)
