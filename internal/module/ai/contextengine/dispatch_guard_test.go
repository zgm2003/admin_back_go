package contextengine

import (
	"errors"
	"testing"
)

func TestDispatchGuardMemoryRejectsChangedSummaryAndBrokenParent(t *testing.T) {
	summary := "stable summary"
	profileHash, sourceHash, summaryHash := testSHA256("profile"), testSHA256("source"), testSHA256(summary)
	row := MemoryRecord{ID: 7, ConversationID: 3, ProfileID: 5, ProfileSHA256: profileHash[:], FromMessageID: 1, ThroughMessageID: 9,
		SourceSHA256: sourceHash[:], SummarySHA256: summaryHash[:], Summary: &summary, State: MemoryStateReady}
	source := AuthoritySource{SourceType: "conversation_memory", SourceRef: "conversation_memory:7", SourceSHA256: sourceHash}
	if err := validateDispatchMemory(row, nil, 3, 5, profileHash, source); err != nil {
		t.Fatal(err)
	}

	changed := row
	changedSummary := "changed"
	changed.Summary = &changedSummary
	if err := validateDispatchMemory(changed, nil, 3, 5, profileHash, source); !errors.Is(err, errDispatchPermission) {
		t.Fatalf("changed summary error=%v", err)
	}

	parentID := uint64(6)
	row.ParentMemoryID = &parentID
	if err := validateDispatchMemory(row, nil, 3, 5, profileHash, source); !errors.Is(err, errDispatchPermission) {
		t.Fatalf("missing parent error=%v", err)
	}
}
