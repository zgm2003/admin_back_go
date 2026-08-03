package contextengine

import (
	"context"
	"reflect"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/contextindex"
)

type rebuildRepositoryStub struct {
	profile ContextProfile
	events  *[]string
}

func (repository *rebuildRepositoryStub) FindRebuildProfile(context.Context, uint64) (*ContextProfile, error) {
	profile := repository.profile
	return &profile, nil
}

func (repository *rebuildRepositoryStub) CompareAndSwapRebuildIndex(_ context.Context, change ProfileIndexCAS) (bool, error) {
	*repository.events = append(*repository.events, "mysql:"+string(change.Next.State))
	repository.profile.IndexState = change.Next.State
	repository.profile.ActiveIndexGeneration = cloneUint64(change.Next.ActiveGeneration)
	repository.profile.TargetIndexGeneration = cloneUint64(change.Next.TargetGeneration)
	return true, nil
}

func (repository *rebuildRepositoryStub) LoadRebuildDocuments(context.Context, uint64) ([]RebuildDocument, error) {
	*repository.events = append(*repository.events, "mysql:documents")
	return nil, nil
}

func (repository *rebuildRepositoryStub) ActiveDocumentVersionIDs(context.Context, uint64) ([]uint64, error) {
	*repository.events = append(*repository.events, "mysql:snapshot")
	return nil, nil
}

type rebuildEmbeddingResolverStub struct{ events *[]string }

func (resolver rebuildEmbeddingResolverStub) ResolveEmbedding(context.Context, ContextProfile) (infraai.EmbeddingClient, error) {
	*resolver.events = append(*resolver.events, "embedding:resolve")
	return rebuildEmbeddingClientStub{}, nil
}

type rebuildEmbeddingClientStub struct{}

func (rebuildEmbeddingClientStub) Embed(context.Context, []string) (infraai.EmbeddingResult, error) {
	return infraai.EmbeddingResult{}, nil
}

type rebuildIndexStub struct {
	events    *[]string
	ensureErr error
}

func (index rebuildIndexStub) EnsureCollection(context.Context, contextindex.CollectionSpec) error {
	*index.events = append(*index.events, "qdrant:ensure")
	return index.ensureErr
}
func (index rebuildIndexStub) VerifyCollection(context.Context, string, contextindex.ActiveCollection) error {
	*index.events = append(*index.events, "qdrant:verify")
	return nil
}
func (index rebuildIndexStub) SwitchAlias(context.Context, string, string) error {
	*index.events = append(*index.events, "qdrant:alias")
	return nil
}
func (rebuildIndexStub) AliasTarget(context.Context, string) (string, bool, error) {
	return "", false, nil
}
func (index rebuildIndexStub) DeleteCollection(context.Context, string) error {
	*index.events = append(*index.events, "qdrant:delete")
	return nil
}
func (rebuildIndexStub) DeleteDocumentVersionPoints(context.Context, string, uint64, uint64, uint64) error {
	return nil
}
func (rebuildIndexStub) Upsert(context.Context, string, []contextindex.IndexedPoint) error {
	return nil
}
func (rebuildIndexStub) VerifyPoints(context.Context, string, []contextindex.PointRef, uint32) error {
	return nil
}

func TestRebuildSwitchesAliasBeforeMySQLGeneration(t *testing.T) {
	one := uint64(1)
	events := make([]string, 0, 8)
	repository := &rebuildRepositoryStub{profile: consistencyProfile(ProfileIndexReady, &one, nil), events: &events}
	service := NewProfileRebuildService(RebuildDependencies{
		Repository: repository, Embeddings: rebuildEmbeddingResolverStub{events: &events},
		Index: rebuildIndexStub{events: &events}, CollectionPrefix: "ctx",
	})
	if err := service.RebuildProfile(t.Context(), repository.profile.ID); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"mysql:rebuilding", "qdrant:ensure", "mysql:documents", "embedding:resolve",
		"mysql:snapshot", "qdrant:verify", "qdrant:alias", "mysql:ready",
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events=%v want=%v", events, want)
	}
}

func TestRebuildFailureKeepsHealthyActiveGeneration(t *testing.T) {
	one := uint64(1)
	events := make([]string, 0, 4)
	repository := &rebuildRepositoryStub{profile: consistencyProfile(ProfileIndexReady, &one, nil), events: &events}
	service := NewProfileRebuildService(RebuildDependencies{
		Repository: repository, Embeddings: rebuildEmbeddingResolverStub{events: &events},
		Index: rebuildIndexStub{events: &events, ensureErr: context.DeadlineExceeded}, CollectionPrefix: "ctx",
	})
	if err := service.RebuildProfile(t.Context(), repository.profile.ID); err == nil {
		t.Fatal("failed target collection was reported successful")
	}
	want := []string{"mysql:rebuilding", "qdrant:ensure", "qdrant:delete", "mysql:ready"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events=%v want=%v", events, want)
	}
	if repository.profile.ActiveIndexGeneration == nil || *repository.profile.ActiveIndexGeneration != one {
		t.Fatalf("active generation=%v", repository.profile.ActiveIndexGeneration)
	}
}
