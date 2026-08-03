package contextengine

import (
	"context"
	"testing"
	"time"
)

type cleanupRepositoryStub struct {
	profile     *ContextProfile
	visible     bool
	turnVisible bool
}

func (repository cleanupRepositoryStub) ConversationTurnVisible(context.Context, uint64, uint64, uint64, [32]byte) (bool, error) {
	return repository.turnVisible, nil
}

func (repository cleanupRepositoryStub) FindRebuildProfile(context.Context, uint64) (*ContextProfile, error) {
	return repository.profile, nil
}

func (repository cleanupRepositoryStub) DocumentVersionVisible(context.Context, uint64, uint64) (bool, error) {
	return repository.visible, nil
}

func TestCleanupKeepsVisibleDocumentVersionPoints(t *testing.T) {
	index := &consistencyIndexStub{}
	service := NewIndexCleanupService(cleanupRepositoryStub{visible: true}, index, "ctx", nil)
	err := service.CleanupIndex(t.Context(), ContextIndexCleanupV1{
		Kind: CleanupDocumentVersionPoints, ProfileID: 7, IndexGeneration: 2, DocumentVersionID: 9,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(index.deletedVersionPoint) != 0 {
		t.Fatalf("deleted visible document version points: %v", index.deletedVersionPoint)
	}
}

func TestCleanupEnforcesRetiredCollectionGraceAndPointers(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	one := uint64(1)
	profile := consistencyProfile(ProfileIndexReady, &one, nil)
	index := &consistencyIndexStub{aliases: map[string]string{"ctx_profile_7": "ctx_profile_7_g1"}}
	service := NewIndexCleanupService(cleanupRepositoryStub{profile: &profile}, index, "ctx", func() time.Time { return now })

	future := ContextIndexCleanupV1{Kind: CleanupRetiredCollection, ProfileID: 7, IndexGeneration: 2, NotBeforeUnixMS: now.Add(time.Minute).UnixMilli()}
	if err := service.CleanupIndex(t.Context(), future); err == nil {
		t.Fatal("cleanup before grace deadline succeeded")
	}

	past := future
	past.NotBeforeUnixMS = now.Add(-time.Minute).UnixMilli()
	if err := service.CleanupIndex(t.Context(), past); err != nil {
		t.Fatal(err)
	}
	if len(index.deletedCollections) != 1 || index.deletedCollections[0] != "ctx_profile_7_g2" {
		t.Fatalf("deleted collections=%v", index.deletedCollections)
	}

	past.IndexGeneration = one
	if err := service.CleanupIndex(t.Context(), past); err != nil {
		t.Fatal(err)
	}
	if len(index.deletedCollections) != 1 {
		t.Fatal("active collection was deleted")
	}
}

func TestCleanupKeepsVisibleConversationTurnAndDeletesInvisiblePoint(t *testing.T) {
	index := &consistencyIndexStub{}
	service := NewIndexCleanupService(cleanupRepositoryStub{turnVisible: true}, index, "ctx", nil)
	payload := ContextIndexCleanupV1{Kind: CleanupConversationPoints, ProfileID: 7, IndexGeneration: 2, ConversationID: 11, UserMessageID: 13, SourceSHA256: [32]byte{1}}
	if err := service.CleanupIndex(t.Context(), payload); err != nil {
		t.Fatal(err)
	}
	if len(index.deletedConversationPoint) != 0 {
		t.Fatal("visible conversation point was deleted")
	}
	service = NewIndexCleanupService(cleanupRepositoryStub{}, index, "ctx", nil)
	if err := service.CleanupIndex(t.Context(), payload); err != nil {
		t.Fatal(err)
	}
	if len(index.deletedConversationPoint) != 1 || index.deletedConversationPoint[0] != 13 {
		t.Fatalf("deleted conversation points=%v", index.deletedConversationPoint)
	}
}
