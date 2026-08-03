package contextengine

import (
	"crypto/sha256"
	"errors"
	"testing"
)

func TestMemoryIdentityKeyIncludesEverySourceFact(t *testing.T) {
	base := MemoryCandidate{ConversationID: 1, ProfileID: 2, ThroughMessageID: 3, SourceSHA256: sha256.Sum256([]byte("a"))}
	one, err := MemoryIdentityKey(base)
	if err != nil {
		t.Fatal(err)
	}
	base.SourceSHA256 = sha256.Sum256([]byte("b"))
	two, err := MemoryIdentityKey(base)
	if err != nil || one == two {
		t.Fatalf("source hash must be part of identity: %q %q %v", one, two, err)
	}
}

func TestMemoryInsertContractRejectsCrossConversationParent(t *testing.T) {
	parent := MemoryRecord{ID: 4, ConversationID: 99, ProfileID: 2, FromMessageID: 1, ThroughMessageID: 3, State: MemoryStateReady}
	candidate := MemoryCandidate{ConversationID: 1, ProfileID: 2, ParentMemoryID: &parent.ID, FromMessageID: 4, ThroughMessageID: 5, SourceSHA256: sha256.Sum256([]byte("s")), PolicyVersion: MemoryPolicyVersionV1, State: MemoryStateReady, Summary: "summary", SummarySHA256: sha256.Sum256([]byte("summary"))}
	if err := ValidateMemoryParent(candidate, parent); !errors.Is(err, ErrMemoryParentScope) {
		t.Fatalf("cross conversation parent error=%v", err)
	}
}
